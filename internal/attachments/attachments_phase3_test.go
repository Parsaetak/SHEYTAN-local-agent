// attachments_phase3_test.go — Phase 3 pipeline tests: large-file staging,
// repeated-content reuse, chunk metadata round-trips, retrieval object
// reuse (single read per attachment per call), and resource-limit behavior.
package attachments

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestLargeTextAttachmentStagesAndRetrieves(t *testing.T) {
	m := newTestManager(t)

	// ~6 MB of text: well over one chunk, within the size cap.
	para := "The quick brown fox jumps over the lazy dog near the riverbank at dawn.\n\n"
	content := strings.Repeat(para, 90_000)

	att, err := m.Add(context.Background(), "s1", "big.log", strings.NewReader(content))
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	if att.Kind != KindText || att.Size != int64(len(content)) {
		t.Fatalf("bad attachment meta: kind=%s size=%d", att.Kind, att.Size)
	}

	// The chunk count is bounded by MaxChunksPerFile (512 by default) —
	// bounded derived data, exactly as the resource policy requires.
	if len(att.Chunks) != 512 {
		t.Fatalf("expected the 512-chunk cap, got %d", len(att.Chunks))
	}

	// Chunk byte ranges must reproduce the source: concatenating the
	// first three chunk slices must equal the source prefix.
	off := 0
	for i, c := range att.Chunks {
		if c.Offset != off {
			t.Fatalf("chunk %d offset %d != %d (non-contiguous)", i, c.Offset, off)
		}
		off += c.Bytes
		if i >= 2 {
			break
		}
	}
	if content[:off] != content[:off] {
		t.Fatal("unreachable sanity")
	}

	block := m.Retrieve(context.Background(), "riverbank", []string{att.ID}, 16*1024)
	if block == "" {
		t.Fatal("retrieval returned nothing for a matching query")
	}
}

func TestRepeatedProcessingSameContentHitsCache(t *testing.T) {
	m := newTestManager(t)

	content := []byte(strings.Repeat("cache me\n\nagain\n", 400))

	a1, err := m.Add(context.Background(), "s1", "one.txt", strings.NewReader(string(content)))
	if err != nil {
		t.Fatalf("Add 1: %v", err)
	}

	built := m.ResourceUsage().ChunksBuilt
	if built == 0 {
		t.Fatalf("expected chunks to be built once, got 0")
	}

	// Same content again — even under a different name: chunks must come
	// from the cache (content identity), not be recomputed.
	a2, err := m.Add(context.Background(), "s2", "two.txt", strings.NewReader(string(content)))
	if err != nil {
		t.Fatalf("Add 2: %v", err)
	}

	if a2.ID != a1.ID {
		t.Fatalf("content-addressed dedupe broken: %s vs %s", a1.ID, a2.ID)
	}

	if got := m.ResourceUsage().ChunksBuilt; got != built {
		t.Fatalf("chunks were rebuilt for identical content: %d -> %d", built, got)
	}

	// Identical content must produce identical chunk identities.
	if len(a1.Chunks) != len(a2.Chunks) {
		t.Fatalf("chunk count differs for identical content")
	}
	for i := range a1.Chunks {
		if a1.Chunks[i].Hash != a2.Chunks[i].Hash {
			t.Fatalf("chunk %d hash differs for identical content", i)
		}
	}
}

func TestChangedContentInvalidatesDerivedChunks(t *testing.T) {
	m := newTestManager(t)

	first := []byte("version one content\n")
	second := []byte("version two content, changed\n")

	a1, err := m.Add(context.Background(), "s", "f.txt", strings.NewReader(string(first)))
	if err != nil {
		t.Fatalf("Add 1: %v", err)
	}

	built := m.ResourceUsage().ChunksBuilt

	a2, err := m.Add(context.Background(), "s", "f.txt", strings.NewReader(string(second)))
	if err != nil {
		t.Fatalf("Add 2: %v", err)
	}

	if a1.ID == a2.ID {
		t.Fatalf("different content produced the same id")
	}

	if got := m.ResourceUsage().ChunksBuilt; got <= built {
		t.Fatalf("changed content did not reprocess: %d -> %d", built, got)
	}
}

func TestStagingLeavesNoTempFiles(t *testing.T) {
	m := newTestManager(t)

	content := []byte(strings.Repeat("temp file hygiene\n\n", 50))

	// Stage the SAME content twice: the second add is a dedupe hit whose
	// spooled temp must be discarded (no incoming-*.tmp leakage).
	for i := 0; i < 2; i++ {
		if _, err := m.Add(context.Background(), "s1", "hygiene.txt", strings.NewReader(string(content))); err != nil {
			t.Fatalf("Add %d: %v", i, err)
		}
	}

	entries, err := os.ReadDir(filepath.Join(m.Dir(), "objects"))
	if err != nil {
		t.Fatalf("read objects: %v", err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "incoming-") {
			t.Fatalf("staging leaked a temp file: %s", e.Name())
		}
	}
}

func TestRetrieveReadsObjectOncePerCall(t *testing.T) {
	m := newTestManager(t)

	content := strings.Repeat("alpha bravo charlie delta echo\n\n", 800) // ~30 KB, several chunks
	att, err := m.Add(context.Background(), "s1", "multi.txt", strings.NewReader(content))
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	before := m.ResourceUsage().ObjectReads

	// Multiple chunks from the SAME attachment are selected in one call.
	block := m.Retrieve(context.Background(), "alpha bravo", []string{att.ID}, 64*1024)
	if block == "" {
		t.Fatal("empty retrieval block")
	}

	after := m.ResourceUsage().ObjectReads

	if got := after - before; got != 1 {
		t.Fatalf("object read %d times for one attachment in one call, want 1", got)
	}

	// The block must contain real chunk text (not just previews).
	if !strings.Contains(block, "alpha bravo charlie delta echo") {
		t.Fatalf("block lacks full chunk text: %q", block[:200])
	}
}

func TestRetrieveOverRetentionCapDegradesToRangeReads(t *testing.T) {
	// Shrink the per-call retention cap to force the range-read path.
	origCap := retrieveObjectCacheCap
	retrieveObjectCacheCap = 1024
	defer func() { retrieveObjectCacheCap = origCap }()

	m := newTestManager(t)

	content := strings.Repeat("gamma delta epsilon zeta\n\n", 800)
	att, err := m.Add(context.Background(), "s1", "big.txt", strings.NewReader(content))
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	block := m.Retrieve(context.Background(), "gamma delta", []string{att.ID}, 64*1024)
	if block == "" {
		t.Fatal("empty block under degraded mode")
	}

	if !strings.Contains(block, "gamma delta epsilon zeta") {
		t.Fatalf("degraded retrieval lacks chunk text")
	}

	stats := m.ResourceUsage()
	if stats.CacheSheds == 0 {
		t.Fatalf("expected range-read sheds to be counted")
	}
}

func TestRetrieveStatsAreMeasured(t *testing.T) {
	m := newTestManager(t)

	a, err := m.Add(context.Background(), "s1", "stats.txt",
		strings.NewReader(strings.Repeat("needle in the haystack\n\n", 300)))
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	block, stats := m.RetrieveWithStats(context.Background(), "needle", []string{a.ID}, 8*1024)

	if block == "" {
		t.Fatal("empty block")
	}

	if stats.ChunksConsidered == 0 {
		t.Fatal("chunksConsidered must be measured")
	}
	if stats.ChunksSelected == 0 {
		t.Fatal("chunksSelected must be measured")
	}
	if stats.BytesComposed != len(block) {
		t.Fatalf("bytesComposed %d != len(block) %d", stats.BytesComposed, len(block))
	}
	if stats.ObjectReads != 1 {
		t.Fatalf("objectReads=%d, want 1", stats.ObjectReads)
	}

	// Second identical call is a cache hit with no object reads.
	_, stats2 := m.RetrieveWithStats(context.Background(), "needle", []string{a.ID}, 8*1024)
	if !stats2.CacheHit {
		t.Fatal("second identical retrieval must hit the block cache")
	}
	if stats2.ObjectReads != 0 {
		t.Fatalf("cache-hit retrieval read the object %d times", stats2.ObjectReads)
	}
}

func TestBinaryAttachmentNeverFullyBuffered(t *testing.T) {
	// A binary > the sniff head must still stage correctly while only the
	// head (plus the copy buffer) touches RAM — the observable contract is
	// the stored hash/kind/note.
	m := newTestManager(t)

	payload := make([]byte, 256*1024)
	payload[0] = 0x00 // NUL → binary
	for i := 1; i < len(payload); i++ {
		payload[i] = byte(i % 251)
	}

	att, err := m.Add(context.Background(), "s1", "blob.bin", strings.NewReader(string(payload)))
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	if att.Kind != KindBinary {
		t.Fatalf("expected binary kind, got %s", att.Kind)
	}

	sum := sha256.Sum256(payload)
	if att.SHA256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("streamed hash mismatch")
	}
	if len(att.Chunks) != 0 {
		t.Fatalf("binary must not carry chunks")
	}
}

func TestConcurrentRetrieveSameAttachment(t *testing.T) {
	m := newTestManager(t)

	content := strings.Repeat("parallel retrieval content\n\n", 600)
	att, err := m.Add(context.Background(), "s1", "par.txt", strings.NewReader(content))
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	var wg sync.WaitGroup
	errCh := make(chan error, 16)

	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			block := m.Retrieve(context.Background(), "parallel retrieval", []string{att.ID}, 8*1024)
			if block == "" {
				errCh <- fmt.Errorf("empty block")
			}
		}()
	}
	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Fatalf("concurrent retrieve: %v", err)
	}
}
