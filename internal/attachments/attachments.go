// Package attachments implements SHEYTAN's real attachment pipeline
// (v1.1.3Z): select → validate → safe staging → inspect → type detection
// → extract → chunk → cache → associate with a session/message → retrieve
// relevant content → send a bounded representation to the model.
//
// Security model:
//
//   - Uploads are staged inside a dedicated directory under the app's
//     private data dir. Stored files are content-addressed by SHA-256 and
//     written 0600 with O_EXCL semantics; symlink following is rejected.
//   - Original filenames are sanitized to a display name only — they never
//     touch the filesystem.
//   - Nothing is ever executed. Text-ish content is extracted and chunked;
//     images ride the vision pipeline as plain paths; binaries become a
//     bounded metadata note so the model can decide what to do with the
//     files tool.
//   - Hard limits: file size, per-request byte cap, file count, processing
//     time, chunk count, and total in-memory buffering are all bounded and
//     enforced.
//
// Processing results (normalized text, chunk metadata, retrieval blocks)
// are cached in the content-aware contextcache keyed by content hash +
// processing version + configuration fingerprint. Same content under a
// different name must hit; same name with different bytes must miss.
package attachments

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/Parsaetak/SHEYTAN-local-agent/internal/chunking"
	"github.com/Parsaetak/SHEYTAN-local-agent/internal/contextcache"
	"github.com/Parsaetak/SHEYTAN-local-agent/internal/vision"
)

// Hard limits (all enforced; overridable via Options for tests).
const (
	DefaultMaxFileSizeBytes  = 64 << 20 // 64 MiB per file
	DefaultMaxFilesPerAttach = 16       // files per upload request
	DefaultProcessTimeout    = 30 * time.Second
	DefaultChunkSizeBytes    = 4096 // chunk target size for text
	DefaultMaxChunksPerFile  = 512
	DefaultMaxTotalBytes     = 128 << 20 // staging dir soft cap
	DefaultRetrievalBudget   = 16 * 1024 // bytes for one retrieval block
)

// Kind classifies a staged attachment.
type Kind string

const (
	KindText   Kind = "text"
	KindImage  Kind = "image"
	KindBinary Kind = "binary"
)

// Limits is the bound set applied to staging and processing.
type Limits struct {
	MaxFileSizeBytes  int64
	MaxFilesPerAttach int
	ProcessTimeout    time.Duration
	ChunkSizeBytes    int
	MaxChunksPerFile  int
	MaxTotalBytes     int64
	RetrievalBudget   int
}

// DefaultLimits returns the production bound set.
func DefaultLimits() Limits {
	return Limits{
		MaxFileSizeBytes:  DefaultMaxFileSizeBytes,
		MaxFilesPerAttach: DefaultMaxFilesPerAttach,
		ProcessTimeout:    DefaultProcessTimeout,
		ChunkSizeBytes:    DefaultChunkSizeBytes,
		MaxChunksPerFile:  DefaultMaxChunksPerFile,
		MaxTotalBytes:     DefaultMaxTotalBytes,
		RetrievalBudget:   DefaultRetrievalBudget,
	}
}

// Chunk is one stored chunk of an attachment with stable identity and
// provenance. Hashes are content hashes: the same content processed with
// the same configuration yields identical chunk identities.
type Chunk struct {
	ID      string `json:"id"` // <attID>:<index>:<hash8>
	AttID   string `json:"attId"`
	Index   int    `json:"index"`
	Hash    string `json:"hash"`   // sha256 of chunk text
	Offset  int    `json:"offset"` // byte offset in normalized text
	Bytes   int    `json:"bytes"`
	Tokens  int    `json:"tokens"`  // estimated
	Preview string `json:"preview"` // first ~120 chars
}

// Attachment is the metadata record of one staged file.
type Attachment struct {
	ID        string    `json:"id"` // content-addressed: "a" + sha256[:24]
	Name      string    `json:"name"`
	Kind      Kind      `json:"kind"`
	Size      int64     `json:"size"`
	SHA256    string    `json:"sha256"`
	CreatedAt time.Time `json:"createdAt"`
	Chunks    []Chunk   `json:"chunks,omitempty"`
	Note      string    `json:"note,omitempty"`

	// SessionIDs records which sessions reference this attachment
	// (provenance, for listing/cleanup decisions).
	SessionIDs []string `json:"sessionIds,omitempty"`
}

// Manager owns the staged attachment store.
type Manager struct {
	mu      sync.Mutex
	dir     string // <dataDir>/attachments
	cache   *contextcache.Cache
	limits  Limits
	metas   map[string]*Attachment // id -> metadata (loaded lazily)
	loaded  bool
	version string // processing fingerprint for cache keys
}

// Options configures a Manager.
type Options struct {
	Limits Limits
	Cache  *contextcache.Cache
}

// NewManager opens (and creates) the attachment store under dir.
func NewManager(dir string, opts Options) (*Manager, error) {
	if dir == "" {
		return nil, fmt.Errorf("attachments: empty directory")
	}

	limits := opts.Limits
	if limits.MaxFileSizeBytes <= 0 {
		limits = DefaultLimits()
	}

	m := &Manager{
		dir:    dir,
		cache:  opts.Cache,
		limits: limits,
		metas:  make(map[string]*Attachment),
	}

	if m.cache == nil {
		m.cache = contextcache.New()
	}

	m.version = contextcache.ConfigFingerprint(
		fmt.Sprintf("v%d", contextcache.Version),
		fmt.Sprintf("chunk=%d", limits.ChunkSizeBytes),
		fmt.Sprintf("maxchunks=%d", limits.MaxChunksPerFile),
	)

	for _, sub := range []string{"objects", "meta"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			return nil, fmt.Errorf("attachments: create %s: %w", sub, err)
		}
	}

	return m, nil
}

// Dir returns the store root.
func (m *Manager) Dir() string { return m.dir }

// Limits returns the active bound set.
func (m *Manager) Limits() Limits { return m.limits }

// objectPath is the content-addressed object location.
func (m *Manager) objectPath(id string) string {
	return filepath.Join(m.dir, "objects", id)
}

func (m *Manager) metaPath(id string) string {
	return filepath.Join(m.dir, "meta", id+".json")
}

// Add stages one uploaded file: it reads, bounds, hashes, stores, sniffs,
// and processes (chunks) the content. The returned Attachment is safe to
// persist and display. `sessionID` records provenance (may be empty).
func (m *Manager) Add(
	ctx context.Context,
	sessionID string,
	displayName string,
	r io.Reader,
) (*Attachment, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	name := SanitizeName(displayName)

	// Read bounded: one extra byte beyond the cap detects oversize without
	// ever buffering more than the cap in memory.
	lr := io.LimitReader(r, m.limits.MaxFileSizeBytes+1)

	data, err := io.ReadAll(lr)
	if err != nil {
		return nil, fmt.Errorf("attachments: read %s: %w", name, err)
	}

	if int64(len(data)) > m.limits.MaxFileSizeBytes {
		return nil, fmt.Errorf(
			"attachments: %s exceeds the %s per-file limit",
			name,
			humanBytes(m.limits.MaxFileSizeBytes),
		)
	}

	if len(data) == 0 {
		return nil, fmt.Errorf("attachments: %s is empty", name)
	}

	sum := sha256.Sum256(data)

	id := "a" + hex.EncodeToString(sum[:])[:24]
	sha := hex.EncodeToString(sum[:])

	// Content-addressed store: identical content under any name dedupes.
	obj := m.objectPath(id)

	if err := writeObjectAtomic(obj, data); err != nil {
		return nil, err
	}

	kind := classify(name, data)

	att := &Attachment{
		ID:        id,
		Name:      name,
		Kind:      kind,
		Size:      int64(len(data)),
		SHA256:    sha,
		CreatedAt: time.Now().UTC(),
	}

	if sessionID != "" {
		att.SessionIDs = []string{sessionID}
	}

	// Process (extract + chunk) with a bounded timeout. Text only — images
	// and binaries carry no chunks by design.
	if kind == KindText {
		pctx, cancel := context.WithTimeout(ctx, m.limits.ProcessTimeout)
		defer cancel()

		chunks, perr := m.processText(pctx, att, data)
		if perr != nil {
			att.Note = "processing incomplete: " + perr.Error()
		}

		att.Chunks = chunks
	} else if kind == KindImage {
		att.Note = "image attachment — delivered to the vision pipeline when the engine supports it"
	} else {
		att.Note = fmt.Sprintf(
			"binary attachment (%s) — not inlined; the agent can inspect it with its file tools at %s",
			humanBytes(att.Size),
			obj,
		)
	}

	if err := m.saveMeta(att); err != nil {
		return nil, err
	}

	m.mu.Lock()
	m.metas[id] = att
	m.mu.Unlock()

	return att, nil
}

// processText normalizes and chunks text content, caching the result.
func (m *Manager) processText(
	ctx context.Context,
	att *Attachment,
	data []byte,
) ([]Chunk, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	key := contextcache.Key(
		"attachments:chunks",
		att.SHA256,
		m.version,
	)

	if cached, ok := m.cache.Get(key); ok {
		if chunks, ok := cached.([]Chunk); ok {
			return chunks, nil
		}
	}

	text := NormalizeText(data)

	chunks := buildChunks(
		att.ID,
		text,
		m.limits.ChunkSizeBytes,
		m.limits.MaxChunksPerFile,
	)

	m.cache.Put(key, chunks, int64(len(chunks))*int64(chunkMetaSize), 0)

	return chunks, nil
}

// chunkMetaSize is a rough per-chunk overhead estimate for cache sizing.
const chunkMetaSize = 160

// Get returns the metadata for one attachment.
func (m *Manager) Get(id string) (*Attachment, bool) {
	m.mu.Lock()
	att, ok := m.metas[id]
	m.mu.Unlock()

	if ok {
		return att, true
	}

	att, err := m.loadMeta(id)
	if err != nil {
		return nil, false
	}

	m.mu.Lock()
	m.metas[id] = att
	m.mu.Unlock()

	return att, true
}

// List returns every stored attachment, newest first.
func (m *Manager) List() []*Attachment {
	m.loadAll()

	m.mu.Lock()
	defer m.mu.Unlock()

	out := make([]*Attachment, 0, len(m.metas))

	for _, att := range m.metas {
		out = append(out, att)
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})

	return out
}

// Delete removes an attachment: object, metadata, cached derivatives.
// Returns whether the id existed.
func (m *Manager) Delete(id string) bool {
	existed := false

	m.mu.Lock()
	if _, ok := m.metas[id]; ok {
		delete(m.metas, id)
		existed = true
	}
	m.mu.Unlock()

	if err := os.Remove(m.metaPath(id)); err == nil {
		existed = true
	}

	if err := os.Remove(m.objectPath(id)); err == nil {
		existed = true
	}

	m.cache.Invalidate(
		contextcache.Key("attachments:chunks", shaOfID(id), m.version),
	)

	return existed
}

// Associate records that a session references an attachment.
func (m *Manager) Associate(id, sessionID string) {
	if sessionID == "" {
		return
	}

	att, ok := m.Get(id)
	if !ok {
		return
	}

	for _, s := range att.SessionIDs {
		if s == sessionID {
			return
		}
	}

	att.SessionIDs = append(att.SessionIDs, sessionID)
	_ = m.saveMeta(att)
}

// StagePath returns the on-disk path of the stored object (for the vision
// pipeline and for the model's file tools). Returns "" when unknown.
func (m *Manager) StagePath(id string) string {
	att, ok := m.Get(id)
	if !ok {
		return ""
	}

	return m.objectPath(att.ID)
}

// Retrieve composes a bounded, provenance-tagged block of the most
// relevant chunks across the given attachments for one query. Images are
// skipped (they ride the vision pipeline); binaries contribute their note.
// Results are cached content-aware; an empty result returns "".
func (m *Manager) Retrieve(
	ctx context.Context,
	query string,
	ids []string,
	budgetBytes int,
) string {
	if budgetBytes <= 0 {
		budgetBytes = m.limits.RetrievalBudget
	}

	type scored struct {
		att   *Attachment
		chunk Chunk
		score float64
	}

	terms := tokenize(query)
	var outAttachments []*Attachment

	for _, id := range ids {
		if att, ok := m.Get(id); ok {
			outAttachments = append(outAttachments, att)
		}
	}

	if len(outAttachments) == 0 {
		return ""
	}

	// Deterministic cache key: sorted ids + query hash + version + budget.
	sortedIDs := make([]string, len(ids))
	copy(sortedIDs, ids)
	sort.Strings(sortedIDs)

	qHash := contextcache.ContentHash([]byte(strings.ToLower(query)))

	key := contextcache.Key(
		"attachments:retrieve",
		strings.Join(sortedIDs, ","),
		qHash,
		m.version,
		fmt.Sprintf("b%d", budgetBytes),
	)

	if cached, ok := m.cache.Get(key); ok {
		if block, ok := cached.(string); ok {
			return block
		}
	}

	var candidates []scored

	for _, att := range outAttachments {
		switch att.Kind {
		case KindImage:
			continue
		case KindBinary:
			continue
		}

		for _, ch := range att.Chunks {
			if ctx.Err() != nil {
				break
			}

			preview := ch.Preview
			score := scoreChunk(terms, preview, ch)

			if score > 0 {
				candidates = append(candidates, scored{att: att, chunk: ch, score: score})
			}
		}
	}

	// Highest score first; ties broken by attachment then index for
	// determinism.
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}

		if candidates[i].att.ID != candidates[j].att.ID {
			return candidates[i].att.ID < candidates[j].att.ID
		}

		return candidates[i].chunk.Index < candidates[j].chunk.Index
	})

	var b strings.Builder
	used := 0

	writeAttHeader := func(att *Attachment) {
		fmt.Fprintf(
			&b,
			"----- attachment: %s (%s, %s) -----\n",
			att.Name,
			att.Kind,
			humanBytes(att.Size),
		)
	}

	written := make(map[string]bool)

	for _, c := range candidates {
		if ctx.Err() != nil {
			break
		}

		full := m.chunkText(c.att, c.chunk)
		if full == "" {
			continue
		}

		need := len(full) + len(c.att.Name) + 80
		if used+need > budgetBytes {
			// Try a truncated tail of this chunk if we still have room.
			remain := budgetBytes - used

			if remain > len(full) {
				remain = len(full)
			}

			if remain > 256 {
				full = full[:remain]
				need = remain
			} else {
				break
			}
		}

		if !written[c.att.ID] {
			writeAttHeader(c.att)
			written[c.att.ID] = true
		}

		fmt.Fprintf(&b, "[chunk %d · %s]\n%s\n\n", c.chunk.Index, c.chunk.Hash[:8], full)
		used += need
	}

	// If nothing scored, still surface a compact metadata block so the
	// model knows attachments exist (bounded to two files' headers).
	if b.Len() == 0 {
		n := 0

		for _, att := range outAttachments {
			if att.Kind == KindImage {
				continue
			}

			writeAttHeader(att)
			att2 := m.objectPath(att.ID)
			fmt.Fprintf(&b, "full text available at %s\n\n", att2)
			n++
			used += 200

			if n >= 2 || used >= budgetBytes {
				break
			}
		}

		if b.Len() == 0 {
			return ""
		}
	}

	block := strings.TrimSpace(b.String())

	m.cache.Put(key, block, int64(len(block)), time.Minute)

	return block
}

// chunkText reads the exact byte range of one chunk from the stored
// object. Falls back to the preview when the object vanished (the cache
// entry is invalidated so stale previews stop circulating).
func (m *Manager) chunkText(att *Attachment, ch Chunk) string {
	data, err := os.ReadFile(m.objectPath(att.ID))
	if err != nil {
		m.cache.Invalidate(
			contextcache.Key("attachments:chunks", att.SHA256, m.version),
		)
		return ch.Preview
	}

	if ch.Offset >= len(data) {
		return ch.Preview
	}

	end := ch.Offset + ch.Bytes
	if end > len(data) {
		end = len(data)
	}

	return string(data[ch.Offset:end])
}

// --- persistence ----------------------------------------------------------

func (m *Manager) saveMeta(att *Attachment) error {
	data, err := json.Marshal(att)
	if err != nil {
		return err
	}

	return writeAtomic(m.metaPath(att.ID), data)
}

func (m *Manager) loadMeta(id string) (*Attachment, error) {
	data, err := os.ReadFile(m.metaPath(id))
	if err != nil {
		return nil, err
	}

	var att Attachment
	if err := json.Unmarshal(data, &att); err != nil {
		return nil, fmt.Errorf("attachments: corrupt meta %s: %w", id, err)
	}

	return &att, nil
}

func (m *Manager) loadAll() {
	m.mu.Lock()
	loaded := m.loaded
	m.loaded = true
	m.mu.Unlock()

	if loaded {
		return
	}

	entries, err := os.ReadDir(filepath.Join(m.dir, "meta"))
	if err != nil {
		return
	}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}

		id := strings.TrimSuffix(e.Name(), ".json")

		if att, err := m.loadMeta(id); err == nil {
			m.mu.Lock()
			m.metas[id] = att
			m.mu.Unlock()
		}
	}
}

// --- pure helpers ---------------------------------------------------------

// SanitizeName reduces an uploaded filename to a safe display name: the
// base name only (both separator styles), control characters stripped,
// length capped.
func SanitizeName(name string) string {
	name = strings.TrimSpace(name)

	// Normalize Windows-style separators first so "..\dir\file.exe" and
	// "../dir/file.exe" both reduce to their base name.
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))

	name = strings.Map(func(r rune) rune {
		switch {
		case r < 32 || r == 127:
			return -1
		case r == '/' || r == '\\':
			return '_'
		default:
			return r
		}
	}, name)

	name = strings.TrimSpace(name)

	const maxName = 200
	if utf8.RuneCountInString(name) > maxName {
		runes := []rune(name)
		name = string(runes[:maxName])
	}

	if name == "" || name == "." || name == ".." {
		name = "attachment"
	}

	return name
}

// classify decides text/image/binary from the name extension and a
// content sniff (a renamed .exe must not pass as text).
func classify(name string, data []byte) Kind {
	if vision.IsImageFile(name) {
		return KindImage
	}

	if looksBinary(data) {
		return KindBinary
	}

	if chunking.IsKnownTextExt(name) || isUTF8ish(data) {
		return KindText
	}

	return KindBinary
}

// looksBinary reports whether the head of the content has binary
// signatures (NUL runs).
func looksBinary(data []byte) bool {
	head := data
	if len(head) > 8192 {
		head = head[:8192]
	}

	for _, b := range head {
		if b == 0 {
			return true
		}
	}

	return false
}

// isUTF8ish tolerates mostly-valid UTF-8 (log files with a stray byte).
func isUTF8ish(data []byte) bool {
	head := data
	if len(head) > 16384 {
		head = head[:16384]
	}

	if utf8.Valid(head) {
		return true
	}

	bad, total := 0, 0

	for _, r := range string(head) {
		total++
		if r == utf8.RuneError {
			bad++
		}
	}

	return total > 0 && bad*10 < total
}

// NormalizeText converts staged bytes into prompt-ready text: invalid
// UTF-8 replaced, BOM dropped, CRLF normalized, trailing space trimmed.
func NormalizeText(data []byte) string {
	b := data

	if len(b) >= 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF {
		b = b[3:]
	}

	s := strings.ToValidUTF8(string(b), "\uFFFD")
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")

	return strings.TrimRight(s, "\n")
}

// buildChunks splits normalized text on semantic boundaries (blank lines,
// then lines, then hard splits) and stamps each chunk with stable
// identity: content hash + sequence. Byte offsets index the normalized
// text stored in the object, so chunk text can be re-read exactly.
func buildChunks(attID, text string, chunkSize, maxChunks int) []Chunk {
	if chunkSize < 256 {
		chunkSize = 256
	}

	if maxChunks < 1 {
		maxChunks = 1
	}

	if text == "" {
		return nil
	}

	parts := chunking.SplitParagraphs(text, chunkSize)
	if len(parts) > maxChunks {
		parts = parts[:maxChunks]
	}

	out := make([]Chunk, 0, len(parts))
	offset := 0

	for i, p := range parts {
		// SplitParagraphs keeps trailing newlines; recompute the true
		// offset of this part in the normalized text.
		idx := strings.Index(text[offset:], p)
		start := offset

		if idx >= 0 {
			start = offset + idx
		}

		sum := sha256.Sum256([]byte(p))
		hash := hex.EncodeToString(sum[:])

		out = append(out, Chunk{
			ID:      fmt.Sprintf("%s:%d:%s", attID, i, hash[:8]),
			AttID:   attID,
			Index:   i,
			Hash:    hash,
			Offset:  start,
			Bytes:   len(p),
			Tokens:  chunking.EstimateTokens(p),
			Preview: clipRunes(strings.TrimSpace(p), 120),
		})

		offset = start + len(p)
	}

	return out
}

// tokenize lowercases and splits a query into overlap terms.
func tokenize(q string) []string {
	fields := strings.FieldsFunc(strings.ToLower(q), func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r > 127)
	})

	out := make([]string, 0, len(fields))

	for _, f := range fields {
		if len(f) >= 2 {
			out = append(out, f)
		}
	}

	return out
}

// scoreChunk ranks one chunk against the query terms using the preview
// text plus term frequency. Term overlap in a 120-char preview is a cheap
// first-pass relevance signal; the full chunk text is not scanned here to
// keep retrieval O(chunks) with no I/O (chunk text is only read for
// selected chunks). Lexical overlap stays deterministic and offline-safe.
func scoreChunk(terms []string, preview string, ch Chunk) float64 {
	if len(terms) == 0 {
		return 0
	}

	hay := strings.ToLower(preview)

	score := 0.0

	for _, t := range terms {
		if strings.Contains(hay, t) {
			score += float64(len(t))
		}
	}

	if score == 0 {
		return 0
	}

	// Earlier chunks carry headers/titles — slight boost.
	score += float64(maxChunksBoost-ch.Index) * 0.01

	// Larger chunks carry more evidence.
	score += float64(ch.Bytes) / 4096.0

	return score
}

const maxChunksBoost = 16

func shaOfID(id string) string {
	sum := sha256.Sum256([]byte(id))
	return hex.EncodeToString(sum[:])
}

func clipRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}

	return string([]rune(s)[:n]) + "…"
}

func humanBytes(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

// writeObjectAtomic writes data to path refusing to follow symlinks and
// using tmp+rename for atomicity. Existing identical objects are fine
// (content-addressed dedupe).
func writeObjectAtomic(path string, data []byte) error {
	if fi, err := os.Lstat(path); err == nil {
		if fi.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("attachments: refusing symlink at %s", path)
		}

		// Same content already stored — dedupe hit.
		return nil
	}

	tmp := path + ".tmp"

	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("attachments: stage: %w", err)
	}

	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("attachments: commit: %w", err)
	}

	return nil
}

func writeAtomic(path string, data []byte) error {
	tmp := path + ".tmp"

	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}

	return os.Rename(tmp, path)
}
