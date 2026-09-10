// contextcache_phase3_test.go — Phase 3 cache tests: single-flight
// coalescing (GetOrCompute), oversized-entry rejection, new counters.
package contextcache

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestGetOrComputeComputesOnce(t *testing.T) {
	c := New()

	calls := 0
	v, ok := GetOrCompute(c, "k", 0, func(s string) int64 { return int64(len(s)) }, func() string {
		calls++
		return "computed"
	})

	if !ok || v != "computed" || calls != 1 {
		t.Fatalf("first call: ok=%t v=%q calls=%d", ok, v, calls)
	}

	v, ok = GetOrCompute(c, "k", 0, func(s string) int64 { return int64(len(s)) }, func() string {
		calls++
		return "should-not-run"
	})

	if !ok || v != "computed" || calls != 1 {
		t.Fatalf("cached call recomputed: ok=%t v=%q calls=%d", ok, v, calls)
	}

	if c.Stats().Hits != 1 {
		t.Fatalf("expected 1 hit, got %d", c.Stats().Hits)
	}
}

func TestGetOrComputeCoalescesConcurrentSameKey(t *testing.T) {
	c := New()

	var calls atomic.Int64
	release := make(chan struct{})
	computeStarted := make(chan struct{})

	compute := func() string {
		if calls.Add(1) == 1 {
			close(computeStarted)
		}
		<-release
		return "shared"
	}

	const workers = 16
	var wg sync.WaitGroup
	results := make([]string, workers)

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			v, ok := GetOrCompute(c, "hot", 0, func(s string) int64 { return int64(len(s)) }, compute)
			if !ok {
				panic("coalesced call lost")
			}
			results[idx] = v
		}(i)
	}

	<-computeStarted
	// Give the other goroutines a moment to pile onto the in-flight call.
	time.Sleep(20 * time.Millisecond)
	close(release)
	wg.Wait()

	if got := calls.Load(); got != 1 {
		t.Fatalf("compute ran %d times, want exactly 1", got)
	}

	for i, r := range results {
		if r != "shared" {
			t.Fatalf("worker %d got %q", i, r)
		}
	}

	if got := c.Stats().Coalesced; got == 0 {
		t.Fatalf("expected coalesced joins to be counted, got 0")
	}
}

func TestGetOrComputePropagatesPanic(t *testing.T) {
	c := New()

	boom := func() string {
		panic("boom")
	}

	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Fatalf("expected panic to propagate to the owner")
			}
		}()
		GetOrCompute(c, "p", 0, nil, boom)
	}()

	// The in-flight slot must be released: the next caller recomputes
	// instead of deadlocking/seeing a stale call.
	ran := false
	func() {
		defer func() {
			_ = recover()
		}()
		GetOrCompute(c, "p", 0, nil, boom)
		ran = true // unreachable when boom panics again — fine
	}()

	if ran {
		// boom panics every time; reaching here means the panic vanished.
		t.Fatalf("panic was swallowed on recompute")
	}

	// A healthy value can now be stored under the same key.
	v, ok := GetOrCompute(c, "p", 0, nil, func() string { return "ok" })
	if !ok || v != "ok" {
		t.Fatalf("recovery path broken: ok=%t v=%q", ok, v)
	}
}

func TestPutRejectsOversizedEntries(t *testing.T) {
	c := New(WithMaxBytes(1024), WithMaxEntryBytes(512))

	if !c.Put("small", "v", 100, 0) {
		t.Fatalf("small entry must be accepted")
	}

	if c.Put("huge", "v", 4096, 0) {
		t.Fatalf("entry above the max-entry bound must be rejected")
	}

	st := c.Stats()
	if st.Oversized != 1 {
		t.Fatalf("expected 1 oversized rejection, got %d", st.Oversized)
	}

	// The cache must still hold exactly the small entry — the rejection
	// cannot have evicted anything.
	if st.Entries != 1 || st.Bytes != 100 {
		t.Fatalf("cache state after rejection: %+v", st)
	}

	if _, ok := c.Get("small"); !ok {
		t.Fatalf("small entry vanished")
	}

	// GetOrCompute with an oversized result shares the value with the
	// caller but must not cache it.
	v, ok := GetOrCompute(c, "big", 0, func(s string) int64 { return int64(len(s)) }, func() string {
		return string(make([]byte, 4096))
	})
	if !ok || len(v) != 4096 {
		t.Fatalf("oversized GetOrCompute result lost")
	}
	if _, ok := c.Get("big"); ok {
		t.Fatalf("oversized GetOrCompute value must not be cached")
	}
}

func TestGetOrComputeDifferentKeysComputeConcurrently(t *testing.T) {
	c := New()

	release := make(chan struct{})
	started := make(chan struct{}, 2)

	compute := func() string {
		started <- struct{}{}
		<-release
		return "v"
	}

	var wg sync.WaitGroup

	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			GetOrCompute(c, "a", 0, nil, compute)
		}()
		break // only one pair needed
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		GetOrCompute(c, "b", 0, nil, compute)
	}()

	<-started
	<-started // both keys compute in parallel — no cross-key serialization
	close(release)
	wg.Wait()
}

func TestInsertsCountedExactly(t *testing.T) {
	c := New()

	c.Put("a", 1, 10, 0)
	c.Put("a", 2, 10, 0) // update, not an insert
	c.Put("b", 3, 20, 0)

	st := c.Stats()
	if st.Inserts != 2 || st.Bytes != 30 {
		t.Fatalf("accounting wrong: inserts=%d bytes=%d", st.Inserts, st.Bytes)
	}

	c.Invalidate("a")
	c.Put("a", 4, 10, 0) // re-insert after invalidation

	if st := c.Stats(); st.Inserts != 3 {
		t.Fatalf("re-insert not counted: inserts=%d", st.Inserts)
	}
}

func BenchmarkCacheGetOrComputeHit(b *testing.B) {
	c := New()
	GetOrCompute(c, "k", 0, nil, func() string { return "value" })

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, ok := GetOrCompute(c, "k", 0, nil, func() string {
			b.Fatal("must not recompute")
			return ""
		}); !ok {
			b.Fatal("hit lost")
		}
	}
}
