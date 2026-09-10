// memory_phase3_test.go — Phase 3 store tests: append-aware incremental
// cache (no full re-parse after writes), copy-free search correctness
// after append/update/clear, and bounded-resource behavior.
package memory

import (
	"strings"
	"sync"
	"testing"
)

func TestAppendDoesNotForceFullReparses(t *testing.T) {
	path := t.TempDir() + "/mem.jsonl"
	s := New(path)

	// Create the store, then warm the cache: the first Search parses the
	// file once. (A search against a NOT-YET-EXISTING file correctly stays
	// cold — there is nothing to cache yet.)
	if err := s.Append([]string{"seed"}, "seed entry for warming", "agent"); err != nil {
		t.Fatalf("seed append: %v", err)
	}
	if _, err := s.Search("warmup", 5); err != nil {
		t.Fatalf("warmup search: %v", err)
	}

	parses0, inc0 := s.ParseStats()

	for i := 0; i < 50; i++ {
		if err := s.Append([]string{"tag"}, strings.Repeat("entry number ", 20)+string(rune('a'+i%26)), "agent"); err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}

	// Every append must have been folded in incrementally — no full parse
	// may have happened after the warmup.
	parses1, inc1 := s.ParseStats()
	if parses1 != parses0 {
		t.Fatalf("appends triggered full re-parses: %d -> %d", parses0, parses1)
	}
	if inc1-inc0 != 50 {
		t.Fatalf("expected 50 incremental appends, got %d", inc1-inc0)
	}

	// And the data must actually be searchable.
	hits, err := s.Search("entry number", 100)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) != 50 {
		t.Fatalf("expected 50 hits after incremental appends, got %d", len(hits))
	}
}

func TestSearchCorrectAfterClearAndReappend(t *testing.T) {
	path := t.TempDir() + "/mem.jsonl"
	s := New(path)

	_ = s.Append([]string{"old"}, "pre-clear memory about keyboards", "agent")
	if err := s.Clear(); err != nil {
		t.Fatalf("clear: %v", err)
	}

	hits, err := s.Search("keyboards", 10)
	if err != nil {
		t.Fatalf("post-clear search: %v", err)
	}
	if len(hits) != 0 {
		t.Fatalf("cleared store returned hits: %d", len(hits))
	}

	if err := s.Append([]string{"new"}, "post-clear memory about keyboards", "agent"); err != nil {
		t.Fatalf("re-append: %v", err)
	}

	hits, err = s.Search("keyboards", 10)
	if err != nil {
		t.Fatalf("post-reappend search: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("expected exactly 1 hit after clear+reappend, got %d", len(hits))
	}
	if hits[0].Content != "post-clear memory about keyboards" {
		t.Fatalf("wrong hit: %q", hits[0].Content)
	}
}

func TestDeleteByIDUpdatesCacheIncrementally(t *testing.T) {
	path := t.TempDir() + "/mem.jsonl"
	s := New(path)

	var ids []string
	for i := 0; i < 10; i++ {
		e := Entry{Tags: []string{"t"}, Content: "delete target number", Class: ClassM7, Trust: TrustProvisional}
		e = NormalizeEntry(e)
		if err := s.AppendEntry(e); err != nil {
			t.Fatalf("append: %v", err)
		}
		ids = append(ids, e.ID)
	}

	// Warm the parsed cache — a cold cache parses lazily on first need
	// (correct); the incremental guarantee is about NOT re-parsing on
	// every write afterwards.
	if _, err := s.Search("delete target", 100); err != nil {
		t.Fatalf("warmup search: %v", err)
	}

	parses0, _ := s.ParseStats()

	if err := s.DeleteByID(ids[3]); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := s.DeleteByID(ids[7]); err != nil {
		t.Fatalf("delete: %v", err)
	}

	parses1, _ := s.ParseStats()
	if parses1 != parses0 {
		t.Fatalf("delete forced a full re-parse on next read: %d -> %d", parses0, parses1)
	}

	if got := s.Count(); got != 8 {
		t.Fatalf("expected 8 entries after deletes, got %d", got)
	}

	hits, err := s.Search("delete target", 100)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) != 8 {
		t.Fatalf("deleted entries still searchable: %d", len(hits))
	}
}

func TestExternalWriterStillDetected(t *testing.T) {
	// The cache key remains stat-based, so a store written by ANOTHER
	// Store instance (or process) must still be picked up — the
	// incremental path must never hide external writes.
	path := t.TempDir() + "/mem.jsonl"
	s1 := New(path)
	s2 := New(path)

	_ = s1.Append([]string{"a"}, "first writer content", "agent")
	_ = s2.Append([]string{"b"}, "second writer content", "agent")

	hits, err := s1.Search("second writer", 10)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("external write not visible: %d hits", len(hits))
	}
}

func TestSearchDoesNotMutateCachedEntries(t *testing.T) {
	path := t.TempDir() + "/mem.jsonl"
	s := New(path)

	_ = s.Append([]string{"score"}, "scored content alpha", "agent")
	_ = s.Append([]string{"score"}, "scored content beta", "agent")

	// Score is search-only and must not leak into the cached entries.
	if _, err := s.Search("alpha", 10); err != nil {
		t.Fatalf("search: %v", err)
	}

	all, err := s.All()
	if err != nil {
		t.Fatalf("all: %v", err)
	}
	for _, e := range all {
		if e.Score != 0 {
			t.Fatalf("cached entry carried a search score: %+v", e)
		}
	}
}

func TestConcurrentSearchAppendRace(t *testing.T) {
	path := t.TempDir() + "/mem.jsonl"
	s := New(path)

	_ = s.Append([]string{"seed"}, "seed entry", "agent")

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				_ = s.Append([]string{"c"}, "concurrent content", "agent")
				_, _ = s.Search("concurrent", 5)
			}
		}(i)
	}
	wg.Wait()

	if got := s.Count(); got != 81 {
		t.Fatalf("expected 81 entries after concurrent appends, got %d", got)
	}
}

func BenchmarkMemorySearchLargeStore(b *testing.B) {
	path := b.TempDir() + "/mem.jsonl"
	s := New(path)

	for i := 0; i < 5000; i++ {
		_ = s.Append([]string{"bench"}, "benchmark entry with some searchable content about pipelines "+string(rune('a'+i%26)), "agent")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := s.Search("searchable content", 10); err != nil {
			b.Fatal(err)
		}
	}
}
