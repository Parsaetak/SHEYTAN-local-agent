// Package memory is the agent's persistent memory layer. It's a JSONL file
// with full-text search over tags + content. Memory survives across runs
// and across sessions — the agent can recall "facts" from previous runs.
package memory

import (
        "bufio"
        "context"
        crand "crypto/rand"
        "encoding/json"
        "fmt"
        "os"
        "path/filepath"
        "sort"
        "strings"
        "sync"
        "sync/atomic"
        "time"
)

// Entry is one persisted memory fact.
type Entry struct {
        ID        string    `json:"id"`
        Tags      []string  `json:"tags"`
        Content   string    `json:"content"`
        Source    string    `json:"source,omitempty"` // session ID or "system"
        CreatedAt time.Time `json:"createdAt"`
        Score     float64   `json:"-"` // search-only
}

// Store is the JSONL-backed memory store.
type Store struct {
        path string
        mu   sync.Mutex
}

// New opens (or creates) a memory store at `path`.
func New(path string) *Store {
        _ = os.MkdirAll(filepath.Dir(path), 0o755)
        return &Store{path: path}
}

// Append writes a new memory entry.
func (s *Store) Append(tags []string, content, source string) error {
        s.mu.Lock()
        defer s.mu.Unlock()
        e := Entry{
                ID:        uniqueID(),
                Tags:      tags,
                Content:   content,
                Source:    source,
                CreatedAt: time.Now().UTC(),
        }
        f, err := os.OpenFile(s.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
        if err != nil {
                return err
        }
        defer f.Close()
        enc := json.NewEncoder(f)
        enc.SetEscapeHTML(false)
        return enc.Encode(e)
}

// All reads every entry from disk.
func (s *Store) All() ([]Entry, error) {
        s.mu.Lock()
        defer s.mu.Unlock()
        return s.allLocked()
}

// allLocked is the inner All without re-acquiring the mutex. Callers must
// already hold s.mu.
func (s *Store) allLocked() ([]Entry, error) {
        f, err := os.Open(s.path)
        if err != nil {
                if os.IsNotExist(err) {
                        return nil, nil
                }
                return nil, err
        }
        defer f.Close()
        var out []Entry
        sc := bufio.NewScanner(f)
        sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
        for sc.Scan() {
                var e Entry
                if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
                        continue
                }
                out = append(out, e)
        }
        return out, sc.Err()
}

// Search returns entries whose tags or content match `query` (case-insensitive).
func (s *Store) Search(query string, limit int) ([]Entry, error) {
        all, err := s.All()
        if err != nil {
                return nil, err
        }
        q := strings.ToLower(query)
        if q == "" {
                sort.Slice(all, func(i, j int) bool {
                        return all[i].CreatedAt.After(all[j].CreatedAt)
                })
                if limit > 0 && len(all) > limit {
                        all = all[:limit]
                }
                return all, nil
        }
        for i := range all {
                var score float64
                for _, t := range all[i].Tags {
                        if strings.Contains(strings.ToLower(t), q) {
                                score += 2.0
                        }
                }
                if strings.Contains(strings.ToLower(all[i].Content), q) {
                        score += 1.0
                }
                all[i].Score = score
        }
        var hits []Entry
        for _, e := range all {
                if e.Score > 0 {
                        hits = append(hits, e)
                }
        }
        sort.Slice(hits, func(i, j int) bool {
                return hits[i].Score > hits[j].Score
        })
        if limit > 0 && len(hits) > limit {
                hits = hits[:limit]
        }
        return hits, nil
}

// DeleteByID removes one entry by ID (rewrites the file).
func (s *Store) DeleteByID(id string) error {
        s.mu.Lock()
        defer s.mu.Unlock()
        entries, err := s.allLocked()
        if err != nil {
                return err
        }
        var kept []Entry
        for _, e := range entries {
                if e.ID != id {
                        kept = append(kept, e)
                }
        }
        tmp := s.path + ".tmp"
        f, err := os.Create(tmp)
        if err != nil {
                return err
        }
        enc := json.NewEncoder(f)
        enc.SetEscapeHTML(false)
        for _, e := range kept {
                if err := enc.Encode(e); err != nil {
                        f.Close()
                        _ = os.Remove(tmp)
                        return err
                }
        }
        if err := f.Close(); err != nil {
                _ = os.Remove(tmp)
                return err
        }
        return os.Rename(tmp, s.path)
}

// Clear empties the memory file.
func (s *Store) Clear() error {
        s.mu.Lock()
        defer s.mu.Unlock()
        return os.WriteFile(s.path, []byte{}, 0o644)
}

// Count returns the number of stored entries.
func (s *Store) Count() int {
        entries, _ := s.All()
        return len(entries)
}

// Tool wraps Store as an agent.Tool so the LLM can read/write memory.
type Tool struct {
        Store *Store

        // RecallSearch optionally searches past-conversation digests (the
        // persistent recall engine). Wired by the runtime stack; nil keeps the
        // tool memory-only.
        RecallSearch func(query string, k int) []string
}

func (t Tool) Name() string { return "memory" }
func (Tool) Description() string {
        return "Persist + recall facts across sessions. action=recall+query | action=remember+tags+content | action=list | action=forget+id | action=history+query (search past conversations)"
}
func (Tool) Parameters() any {
        return struct {
                Action  string   `json:"action"`
                Query   string   `json:"query,omitempty"`
                Tags    []string `json:"tags,omitempty"`
                Content string   `json:"content,omitempty"`
                ID      string   `json:"id,omitempty"`
                Limit   int      `json:"limit,omitempty"`
        }{}
}

func (t Tool) Run(ctx context.Context, args json.RawMessage) (string, error) {
        var p struct {
                Action  string   `json:"action"`
                Query   string   `json:"query"`
                Tags    []string `json:"tags"`
                Content string   `json:"content"`
                ID      string   `json:"id"`
                Limit   int      `json:"limit"`
        }
        if err := json.Unmarshal(args, &p); err != nil {
                // Friendly hint for the classic mistake: tags passed as a string.
                var probe map[string]json.RawMessage
                if json.Unmarshal(args, &probe) == nil {
                        if rawTags, ok := probe["tags"]; ok && len(rawTags) > 0 && rawTags[0] == '"' {
                                return "", fmt.Errorf(`bad args: "tags" must be a JSON ARRAY of strings, e.g. {"tags":["project","fact"]} — not a plain string`)
                        }
                }
                return "", fmt.Errorf("bad args: %w", err)
        }
        if p.Limit == 0 {
                p.Limit = 10
        }
        switch strings.ToLower(p.Action) {
        case "recall", "search":
                hits, err := t.Store.Search(p.Query, p.Limit)
                if err != nil {
                        return "", err
                }
                if len(hits) == 0 {
                        return "No matching memories found.", nil
                }
                var b strings.Builder
                for i, e := range hits {
                        fmt.Fprintf(&b, "[%d] %s tags=%v source=%s\n    %s\n\n",
                                i+1, e.ID, e.Tags, e.Source, truncate(e.Content, 200))
                }
                return b.String(), nil
        case "history", "past":
                // v1.0.2: search digests of PAST CONVERSATIONS (what the user asked
                // and what happened across sessions), not just curated facts.
                if t.RecallSearch == nil {
                        return "Past-conversation search is not available in this build.", nil
                }
                if p.Query == "" {
                        return "", fmt.Errorf("query is required for history search")
                }
                lines := t.RecallSearch(p.Query, p.Limit)
                if len(lines) == 0 {
                        return "No matching past conversations found.", nil
                }
                return strings.Join(lines, "\n"), nil
        case "remember", "save", "add":
                if p.Content == "" {
                        return "", fmt.Errorf("content is required to remember")
                }
                if err := t.Store.Append(p.Tags, p.Content, "agent"); err != nil {
                        return "", err
                }
                return "remembered", nil
        case "list":
                all, err := t.Store.All()
                if err != nil {
                        return "", err
                }
                var b strings.Builder
                for _, e := range all {
                        fmt.Fprintf(&b, "%s tags=%v %s\n", e.ID, e.Tags, truncate(e.Content, 80))
                }
                return b.String(), nil
        case "forget", "delete":
                if p.ID == "" {
                        return "", fmt.Errorf("id is required to forget")
                }
                return fmt.Sprintf("forgotten %s", p.ID), t.Store.DeleteByID(p.ID)
        case "clear":
                return "cleared", t.Store.Clear()
        default:
                return "", fmt.Errorf("unknown action %q", p.Action)
        }
}

func truncate(s string, n int) string {
        if len(s) <= n {
                return s
        }
        return s[:n] + "..."
}

// idSeq guarantees uniqueness inside one process even when the OS clock
// granularity makes consecutive time.Now() calls return the same instant
// (Windows ticks are coarse — this is what let two Appends share an ID and
// made DeleteByID wipe both entries in the v1.0.10 CI run).
var idSeq atomic.Uint64

// uniqueID is collision-safe: a UTC timestamp prefix (IDs stay
// human-sortable chronologically) + a per-process counter + 4 random bytes.
// Two Appends can only collide if they land in the same second, wrap the
// 6-digit counter to the same value AND draw identical random bytes.
func uniqueID() string {
        var rnd [4]byte
        if _, err := crand.Read(rnd[:]); err != nil {
                // crypto/rand only fails if the OS entropy source is broken; fall
                // back to wider counter digits rather than ever returning a
                // timestamp-only (collidable) ID.
                return time.Now().UTC().Format("20060102150405") +
                        fmt.Sprintf("-%09d", idSeq.Add(1))
        }
        return time.Now().UTC().Format("20060102150405") +
                fmt.Sprintf("-%06d-%08x", idSeq.Add(1)%1_000_000, rnd)
}
