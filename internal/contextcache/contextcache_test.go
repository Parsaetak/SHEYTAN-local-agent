package contextcache

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestHitMissAndStats(t *testing.T) {
	c := New()

	if _, ok := c.Get("missing"); ok {
		t.Fatal("expected miss for absent key")
	}

	c.Put("k1", "value1", 10, 0)

	v, ok := c.Get("k1")
	if !ok || v != "value1" {
		t.Fatalf("expected hit with value1, got ok=%v v=%v", ok, v)
	}

	stats := c.Stats()

	if stats.Hits != 1 || stats.Misses != 1 {
		t.Fatalf("unexpected stats: %+v", stats)
	}

	if stats.HitRatio != 0.5 {
		t.Fatalf("expected hit ratio 0.5, got %v", stats.HitRatio)
	}
}

func TestSamePathDifferentContentMustMiss(t *testing.T) {
	// The cache contract: keys are content-derived. Two "files" at the same
	// path with different bytes must produce different keys.
	c := New()

	path := "/workspace/notes.md"

	keyV1 := Key("chunks", path, ContentHash([]byte("version one")))
	keyV2 := Key("chunks", path, ContentHash([]byte("version two — edited")))

	c.Put(keyV1, "old chunks", 10, 0)

	if _, ok := c.Get(keyV2); ok {
		t.Fatal("edited content must not be served from the old content's cache entry")
	}

	if v, ok := c.Get(keyV1); !ok || v != "old chunks" {
		t.Fatal("unchanged content should still hit")
	}
}

func TestLRUEvictionByEntries(t *testing.T) {
	c := New(WithMaxEntries(3), WithMaxBytes(1<<30))

	for i := 0; i < 5; i++ {
		c.Put(fmt.Sprintf("k%d", i), i, 1, 0)
	}

	stats := c.Stats()

	if stats.Entries != 3 {
		t.Fatalf("expected 3 entries after eviction, got %d", stats.Entries)
	}

	if stats.Evictions != 2 {
		t.Fatalf("expected 2 evictions, got %d", stats.Evictions)
	}

	// Oldest entries must be gone; newest must remain.
	if _, ok := c.Get("k0"); ok {
		t.Fatal("k0 should have been evicted (oldest)")
	}

	if _, ok := c.Get("k4"); !ok {
		t.Fatal("k4 should remain (newest)")
	}
}

func TestLRUEvictionByBytes(t *testing.T) {
	c := New(WithMaxEntries(100), WithMaxBytes(100))

	c.Put("big1", "x", 60, 0)
	c.Put("big2", "y", 60, 0)

	stats := c.Stats()

	if stats.Bytes > 100 {
		t.Fatalf("byte bound violated: %d > 100", stats.Bytes)
	}

	if stats.Entries > 1 {
		t.Fatalf("expected eviction down to <=1 entry, got %d", stats.Entries)
	}
}

func TestLRURecencyProtectsHotEntries(t *testing.T) {
	c := New(WithMaxEntries(2))

	c.Put("a", 1, 1, 0)
	c.Put("b", 2, 1, 0)

	// Touch "a" so it becomes most-recently-used.
	if _, ok := c.Get("a"); !ok {
		t.Fatal("a should be present")
	}

	c.Put("c", 3, 1, 0)

	if _, ok := c.Get("a"); !ok {
		t.Fatal("recently-used 'a' must survive; 'b' should be evicted instead")
	}

	if _, ok := c.Get("b"); ok {
		t.Fatal("least-recently-used 'b' should have been evicted")
	}
}

func TestTTLExpiry(t *testing.T) {
	c := New()

	c.Put("ttl", "value", 1, 20*time.Millisecond)

	if _, ok := c.Get("ttl"); !ok {
		t.Fatal("fresh TTL entry should hit")
	}

	time.Sleep(40 * time.Millisecond)

	if _, ok := c.Get("ttl"); ok {
		t.Fatal("expired entry must miss")
	}
}

func TestInvalidatePrefix(t *testing.T) {
	c := New()

	c.Put(Key("attachments:chunks", "aaa", "v1"), 1, 1, 0)
	c.Put(Key("attachments:chunks", "bbb", "v1"), 2, 1, 0)
	c.Put(Key("attachments:retrieve", "aaa"), 3, 1, 0)

	dropped := c.InvalidatePrefix("attachments:chunks")

	if dropped != 2 {
		t.Fatalf("expected 2 dropped, got %d", dropped)
	}

	if _, ok := c.Get(Key("attachments:retrieve", "aaa")); !ok {
		t.Fatal("unrelated entry must survive prefix invalidation")
	}
}

func TestClearAndConfigFingerprint(t *testing.T) {
	c := New()

	c.Put("a", 1, 1, 0)
	c.Put("b", 2, 1, 0)

	c.Clear()

	if got := c.Stats().Entries; got != 0 {
		t.Fatalf("clear must empty the cache, got %d entries", got)
	}

	// Content-sensitive fingerprints: different config → different key.
	f1 := ConfigFingerprint("chunk=4096")
	f2 := ConfigFingerprint("chunk=8192")

	if f1 == f2 {
		t.Fatal("different configuration must produce different fingerprints")
	}
}

func TestConcurrentAccess(t *testing.T) {
	c := New(WithMaxEntries(64))

	var wg sync.WaitGroup

	for worker := 0; worker < 8; worker++ {
		wg.Add(1)

		go func(seed int) {
			defer wg.Done()

			for i := 0; i < 500; i++ {
				key := fmt.Sprintf("k%d", (seed+i)%100)

				if i%3 == 0 {
					c.Put(key, i, 1, 0)
					continue
				}

				c.Get(key)

				if i%97 == 0 {
					c.InvalidatePrefix("k")
				}
			}
		}(worker)
	}

	wg.Wait()

	if got := c.Stats().Entries; got > 64 {
		t.Fatalf("bound violated under concurrency: %d entries", got)
	}
}
