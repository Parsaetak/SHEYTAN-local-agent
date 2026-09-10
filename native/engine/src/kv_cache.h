// kv_cache.h — the SHEYTAN native engine KV cache (Phase 4).
//
// A real K/V cache sized from the model's actual dimensions
// (layer_count, embedding_length, head_count, head_dim, context_length).
// The cache stores per-layer K and V tensors as contiguous float16
// buffers, indexed by position [0, capacity).
//
// What this is:
//   - real allocations, sized from real model dims;
//   - explicit capacity (context length);
//   - measured bytes (returned to the host and reported through metrics);
//   - clear/reset on model unload and at request boundaries;
//   - reuse across decode steps (the same buffer is overwritten per
//     position by the future transformer forward pass);
//   - explicit ownership: the cache OWNS its buffers and frees them on
//     destruction.
//
// What this is NOT:
//   - this is NOT a paged cache (no virtual memory mapping);
//   - this is NOT a quantized cache (f16 only in Phase 4);
//   - this is NOT wired into a transformer forward pass (no forward pass
//     exists yet — the cache exists, is allocated, is measurable, but no
//     inference populates it);
//   - the cache never FABRICATES utilization — usage is the count of
//     positions actually written, which is 0 until a forward pass
//     exists.
//
// SECURITY / BOUNDS:
//   - every dimension comes from the parsed GGUF header (already
//     validated by gguf.cpp);
//   - capacity is bounded by kMaxContextLength (1M positions);
//   - total bytes is checked against kMaxKVBytes (16 GiB) and against
//     the available RAM passed in by the host;
//   - allocation failure returns SHTN_ERR_INTERNAL with a clear error
//     (never a nullptr dereference).

#ifndef SHTN_KV_CACHE_H
#define SHTN_KV_CACHE_H

#include "shtn/types.h"

#include <cstdint>
#include <memory>
#include <string>
#include <vector>

namespace shtn {
namespace kv {

// Hard bounds (defensive — real models are far below these).
constexpr uint64_t kMaxContextLength = 1u << 20;  // 1M positions
constexpr uint64_t kMaxKVBytes       = 16ull << 30; // 16 GiB total

// Layout describes the per-layer K/V tensor shape derived from the model.
struct Layout {
    uint32_t layer_count = 0;       // <arch>.block_count
    uint32_t embedding_length = 0;  // <arch>.embedding_length
    uint32_t head_count = 0;        // <arch>.attention.head_count
    uint32_t head_dim = 0;          // embedding_length / head_count
    uint32_t kv_head_count = 0;     // <arch>.attention.head_count_kv (GQA)
    uint32_t kv_dim = 0;            // kv_head_count * head_dim
    uint64_t context_length = 0;    // effective context (planned)

    bool valid() const {
        return layer_count > 0 && embedding_length > 0 &&
               head_count > 0 && head_dim > 0 && kv_head_count > 0 &&
               kv_dim > 0 && context_length > 0;
    }
};

// Stats is the measurable snapshot reported through the metrics op.
// Every field is a real measured value — never a guess.
struct Stats {
    bool allocated = false;          // true after a successful alloc
    uint64_t capacity_bytes = 0;     // K + V total allocated bytes
    uint64_t used_bytes = 0;         // bytes for positions actually written
    uint64_t capacity_positions = 0; // context_length
    uint64_t used_positions = 0;     // 0 until a forward pass writes
    uint32_t layer_count = 0;
    uint32_t kv_dim = 0;
    std::string quantization;        // "f16" in Phase 4
};

// Cache is the owned KV cache of one model. Not copyable; move-only.
class Cache {
public:
    Cache() = default;
    ~Cache();

    Cache(const Cache&) = delete;
    Cache& operator=(const Cache&) = delete;
    Cache(Cache&& other) noexcept;
    Cache& operator=(Cache&& other) noexcept;

    // Allocate the K and V buffers from the given layout. Returns:
    //   SHTN_OK              allocated;
    //   SHTN_ERR_INVALID_ARG layout invalid;
    //   SHTN_ERR_NO_MODEL    no layout set;
    //   SHTN_ERR_UNSUPPORTED layout exceeds kMaxContextLength or
    //                         kMaxKVBytes, or exceeds available_ram;
    //   SHTN_ERR_INTERNAL    allocation failed.
    //
    // available_ram_bytes is the host's view of free RAM (0 = unknown;
    // the check is skipped when unknown).
    int32_t allocate(const Layout& layout, uint64_t available_ram_bytes,
                     std::string& error);

    // Release the buffers. Idempotent.
    void release();

    // Reset marks all positions as unused (used_positions_ = 0). The
    // buffers stay allocated; this is the per-request-boundary reset.
    void reset();

    // Advance records that `n` decode positions have been written. Used
    // by the future forward pass to update usage. Bounded by capacity.
    void advance(uint64_t n);

    // Stats returns the current measurable snapshot.
    Stats stats() const;

    // Direct buffer access (for the future forward pass). Returns nullptr
    // when not allocated.
    //
    // The K and V buffers are laid out as:
    //   k[layer][position * kv_dim]   — layer-major, position-contiguous;
    //   v[layer][position * kv_dim]   — same.
    //
    // Each element is a 16-bit float (IEEE 754 binary16). The future
    // forward pass reads/writes through these pointers; the cache itself
    // does no computation.
    float* k_layer(uint32_t layer) const;
    float* v_layer(uint32_t layer) const;

    bool allocated() const { return buffers_ != nullptr; }
    uint64_t capacity_positions() const { return layout_.context_length; }
    uint64_t used_positions() const { return used_positions_; }

private:
    Layout layout_{};
    // One contiguous allocation holding all layers' K then all layers' V.
    // Per-layer pointers are computed as base + layer * layer_bytes.
    std::unique_ptr<float[]> buffers_;
    uint64_t layer_bytes_ = 0;   // bytes per layer per K or V
    uint64_t total_bytes_ = 0;   // capacity_bytes
    uint64_t used_positions_ = 0;
    bool allocated_ = false;
};

} // namespace kv
} // namespace shtn

#endif /* SHTN_KV_CACHE_H */
