// Package contextcache is SHEYTAN's content-aware cache for expensive
// context processing: file extraction, normalization, chunking, chunk
// metadata, retrieval results, and deterministic context transformations.
//
// Design rules (v1.1.3Z):
//
//   - Keys are composed by callers from CONTENT identity — never from
//     paths alone. The same path with different bytes must miss; the same
//     bytes under different paths must hit.
//   - Every key embeds a processing version and the relevant configuration
//     fingerprint, so upgrading the chunker or changing a budget naturally
//     invalidates old entries (version invalidation).
//   - Growth is bounded by both entry count and total byte size; eviction
//     is least-recently-used. Hit/miss/eviction counters expose behavior.
//   - The cache is safe for concurrent use.
//
// The cache stores derived, reproducible data only. Nothing here is the
// source of truth: dropping the whole cache must never lose information,
// only recomputation time.
package contextcache

import (
	"container/list"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash/fnv"
	"sort"
	"strings"
	"sync"
	"time"
)

// Version is the global processing version baked into every key the
// helpers build. Bump it when the processing pipeline changes shape so
// stale entries can never be served to new code.
const Version = 3

// entry is one cached value.
type entry struct {
	key       string
	value     any
	bytes     int64
	expiresAt time.Time // zero = no expiry
}

// Cache is a bounded LRU cache with hit/miss statistics.
type Cache struct {
	mu sync.Mutex

	maxEntries int
	maxBytes   int64

	ll    *list.List               // front = most recent
	items map[string]*list.Element // key -> element holding *entry

	hits      uint64
	misses    uint64
	evictions uint64

	bytes int64
}

// Option configures a Cache at construction time.
type Option func(*Cache)

// WithMaxEntries caps the number of live entries (default 512).
func WithMaxEntries(n int) Option {
	return func(c *Cache) {
		if n > 0 {
			c.maxEntries = n
		}
	}
}

// WithMaxBytes caps the total size of cached values (default 256 MiB).
func WithMaxBytes(n int64) Option {
	return func(c *Cache) {
		if n > 0 {
			c.maxBytes = n
		}
	}
}

// New returns a Cache with the given options.
func New(opts ...Option) *Cache {
	c := &Cache{
		maxEntries: 512,
		maxBytes:   256 << 20,
		ll:         list.New(),
		items:      make(map[string]*list.Element),
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// Key composes a stable cache key from ordered parts. Parts are joined
// with a separator that cannot appear in hex hashes; callers should pass
// hashes/versions/typed values, never raw user text.
func Key(parts ...string) string {
	return strings.Join(parts, "\x1f")
}

// ContentHash returns the hex SHA-256 of data — the canonical content
// identity used in keys. Same bytes → same hash, whatever the path.
func ContentHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// ConfigFingerprint hashes a compact configuration description into the
// key space so a budget/parameter change invalidates derived entries.
func ConfigFingerprint(parts ...string) string {
	h := fnv.New64a()

	for _, p := range parts {
		_, _ = h.Write([]byte(p))
		_, _ = h.Write([]byte{0})
	}

	return hex.EncodeToString(h.Sum(nil))
}

// Get returns the cached value for key and reports whether it hit.
// Expired entries are treated as misses and dropped.
func (c *Cache) Get(key string) (any, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	el, ok := c.items[key]
	if !ok {
		c.misses++
		return nil, false
	}

	e := el.Value.(*entry)

	if !e.expiresAt.IsZero() && time.Now().After(e.expiresAt) {
		c.removeElement(el)
		c.misses++
		return nil, false
	}

	c.ll.MoveToFront(el)
	c.hits++
	return e.value, true
}

// Put stores value under key with an optional TTL (ttl <= 0 = no expiry).
// The size hint lets the bounds track heterogeneous values; callers that
// cannot estimate cheaply may pass 0 (then only the entry count applies).
func (c *Cache) Put(key string, value any, sizeHint int64, ttl time.Duration) {
	if sizeHint < 0 {
		sizeHint = 0
	}

	var expiresAt time.Time

	if ttl > 0 {
		expiresAt = time.Now().Add(ttl)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if el, ok := c.items[key]; ok {
		e := el.Value.(*entry)
		c.bytes -= e.bytes
		e.value = value
		e.bytes = sizeHint
		e.expiresAt = expiresAt
		c.bytes += sizeHint
		c.ll.MoveToFront(el)
		c.evictLocked()
		return
	}

	e := &entry{
		key:       key,
		value:     value,
		bytes:     sizeHint,
		expiresAt: expiresAt,
	}

	c.items[key] = c.ll.PushFront(e)
	c.bytes += sizeHint
	c.evictLocked()
}

// Invalidate drops one key. Returns whether it existed.
func (c *Cache) Invalidate(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	el, ok := c.items[key]
	if !ok {
		return false
	}

	c.removeElement(el)
	return true
}

// InvalidatePrefix drops every key that starts with the given prefix —
// e.g. all chunks derived from one source, or everything produced by an
// old processing version. Returns the number of dropped entries.
func (c *Cache) InvalidatePrefix(prefix string) int {
	c.mu.Lock()
	defer c.mu.Unlock()

	var doomed []*list.Element

	for key, el := range c.items {
		if strings.HasPrefix(key, prefix) {
			doomed = append(doomed, el)
		}
	}

	for _, el := range doomed {
		c.removeElement(el)
	}

	return len(doomed)
}

// Clear drops every entry (corruption recovery: callers that suspect bad
// derived data simply wipe the cache — all contents are reproducible).
func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.ll.Init()
	c.items = make(map[string]*list.Element)
	c.bytes = 0
}

// Stats is a point-in-time snapshot of cache behavior.
type Stats struct {
	Entries   int     `json:"entries"`
	Bytes     int64   `json:"bytes"`
	MaxBytes  int64   `json:"maxBytes"`
	Hits      uint64  `json:"hits"`
	Misses    uint64  `json:"misses"`
	Evictions uint64  `json:"evictions"`
	HitRatio  float64 `json:"hitRatio"`
}

// Stats returns the counters and current occupancy.
func (c *Cache) Stats() Stats {
	c.mu.Lock()
	defer c.mu.Unlock()

	s := Stats{
		Entries:   len(c.items),
		Bytes:     c.bytes,
		MaxBytes:  c.maxBytes,
		Hits:      c.hits,
		Misses:    c.misses,
		Evictions: c.evictions,
	}

	if total := s.Hits + s.Misses; total > 0 {
		s.HitRatio = float64(s.Hits) / float64(total)
	}

	return s
}

// Keys returns the live keys sorted lexicographically (inspection aid;
// used by tests and the debug endpoint).
func (c *Cache) Keys() []string {
	c.mu.Lock()
	defer c.mu.Unlock()

	out := make([]string, 0, len(c.items))
	for k := range c.items {
		out = append(out, k)
	}

	sort.Strings(out)
	return out
}

// removeElement drops one element; caller holds mu.
func (c *Cache) removeElement(el *list.Element) {
	e := el.Value.(*entry)
	c.bytes -= e.bytes
	_ = c.ll.Remove(el)
	delete(c.items, e.key)
}

// evictLocked enforces both bounds; caller holds mu.
func (c *Cache) evictLocked() {
	for len(c.items) > c.maxEntries {
		c.evictOldestLocked()
	}

	for c.bytes > c.maxBytes && len(c.items) > 1 {
		c.evictOldestLocked()
	}
}

func (c *Cache) evictOldestLocked() {
	el := c.ll.Back()
	if el == nil {
		return
	}

	c.removeElement(el)
	c.evictions++
}

// String renders a compact human-readable summary for logs.
func (s Stats) String() string {
	return fmt.Sprintf(
		"entries=%d bytes=%d hits=%d misses=%d evictions=%d hitRatio=%.2f",
		s.Entries, s.Bytes, s.Hits, s.Misses, s.Evictions, s.HitRatio,
	)
}
