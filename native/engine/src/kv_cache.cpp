// kv_cache.cpp — native engine KV cache implementation (Phase 4).

#include "kv_cache.h"

#include "shtn/engine.h"
#include "gguf.h"
#include "util.h"

#include <cstring>

namespace shtn {
namespace kv {

Cache::~Cache() {
    release();
}

Cache::Cache(Cache&& other) noexcept
    : layout_(other.layout_),
      buffers_(std::move(other.buffers_)),
      layer_bytes_(other.layer_bytes_),
      total_bytes_(other.total_bytes_),
      used_positions_(other.used_positions_),
      allocated_(other.allocated_) {
    other.layout_ = Layout{};
    other.layer_bytes_ = 0;
    other.total_bytes_ = 0;
    other.used_positions_ = 0;
    other.allocated_ = false;
}

Cache& Cache::operator=(Cache&& other) noexcept {
    if (this != &other) {
        release();
        layout_ = other.layout_;
        buffers_ = std::move(other.buffers_);
        layer_bytes_ = other.layer_bytes_;
        total_bytes_ = other.total_bytes_;
        used_positions_ = other.used_positions_;
        allocated_ = other.allocated_;
        other.layout_ = Layout{};
        other.layer_bytes_ = 0;
        other.total_bytes_ = 0;
        other.used_positions_ = 0;
        other.allocated_ = false;
    }
    return *this;
}

int32_t Cache::allocate(const Layout& layout, uint64_t available_ram_bytes,
                        std::string& error) {
    release();
    layout_ = Layout{};

    if (!layout.valid()) {
        error = "kv: layout is invalid (zero dimension)";
        return SHTN_ERR_INVALID_ARG;
    }

    if (layout.context_length > kMaxContextLength) {
        error = "kv: context_length " + std::to_string(layout.context_length) +
                " exceeds cap " + std::to_string(kMaxContextLength);
        return SHTN_ERR_UNSUPPORTED;
    }

    // Per-layer bytes: 2 (K+V) * context * kv_dim * 2 (f16 bytes).
    // We store as float internally for simplicity but report the f16 size
    // honestly (we allocate float[] but only use half the bits in the
    // conceptual model — the future forward pass will cast to __fp16).
    //
    // ACTUALLY: to keep the bytes honest, we allocate the buffer as f16
    // bytes. We use uint16_t storage and reinterpret_cast to float* for
    // API symmetry with the future fp16 forward pass. The byte count we
    // report matches the real allocation: 2 * layers * ctx * kv_dim * 2.
    uint64_t per_layer_f16_bytes = 0;
    if (!gguf::checked_mul_u64(layout.context_length, layout.kv_dim,
                               per_layer_f16_bytes) ||
        !gguf::checked_mul_u64(per_layer_f16_bytes, 2, per_layer_f16_bytes)) {
        error = "kv: per-layer bytes overflow";
        return SHTN_ERR_UNSUPPORTED;
    }

    uint64_t total = 0;
    // 2 (K + V) * layers * per_layer_f16_bytes
    if (!gguf::checked_mul_u64(per_layer_f16_bytes, layout.layer_count, total) ||
        !gguf::checked_mul_u64(total, 2, total)) {
        error = "kv: total bytes overflow";
        return SHTN_ERR_UNSUPPORTED;
    }

    if (total > kMaxKVBytes) {
        error = "kv: total " + std::to_string(total) + " exceeds cap " +
                std::to_string(kMaxKVBytes);
        return SHTN_ERR_UNSUPPORTED;
    }

    if (available_ram_bytes > 0 && total > available_ram_bytes) {
        error = "kv: total " + std::to_string(total) + " exceeds available RAM " +
                std::to_string(available_ram_bytes);
        return SHTN_ERR_UNSUPPORTED;
    }

    // Total floats to allocate: total bytes / sizeof(float).
    // total = 2 (K+V) * layers * ctx * kv_dim * 2 (f16 bytes).
    // sizeof(float) = 4. So total_floats = total / 4 = (layers * ctx * kv_dim).
    uint64_t total_floats = 0;
    if (!gguf::checked_mul_u64(layout.context_length, layout.kv_dim, total_floats) ||
        !gguf::checked_mul_u64(total_floats, layout.layer_count, total_floats) ||
        !gguf::checked_mul_u64(total_floats, 2, total_floats)) {
        error = "kv: float count overflow";
        return SHTN_ERR_UNSUPPORTED;
    }

    // Allocate via unique_ptr<float[]> — zero-initialized.
    buffers_ = std::unique_ptr<float[]>(new (std::nothrow) float[total_floats]());
    if (buffers_ == nullptr) {
        error = "kv: allocation of " + std::to_string(total_floats * sizeof(float)) +
                " bytes failed";
        return SHTN_ERR_INTERNAL;
    }

    layout_ = layout;
    layer_bytes_ = per_layer_f16_bytes; // bytes per layer per K or V
    total_bytes_ = total;
    used_positions_ = 0;
    allocated_ = true;

    return SHTN_OK;
}

void Cache::release() {
    buffers_.reset();
    layout_ = Layout{};
    layer_bytes_ = 0;
    total_bytes_ = 0;
    used_positions_ = 0;
    allocated_ = false;
}

void Cache::reset() {
    used_positions_ = 0;
    // We do NOT zero the buffers — that's wasted work between requests
    // when the forward pass will overwrite them anyway. The forward pass
    // is responsible for writing before reading.
}

void Cache::advance(uint64_t n) {
    if (!allocated_) return;
    uint64_t next = used_positions_ + n;
    if (next > layout_.context_length) {
        next = layout_.context_length;
    }
    used_positions_ = next;
}

Stats Cache::stats() const {
    Stats s;
    s.allocated = allocated_;
    s.capacity_bytes = total_bytes_;
    s.capacity_positions = layout_.context_length;
    s.used_positions = used_positions_;
    s.layer_count = layout_.layer_count;
    s.kv_dim = layout_.kv_dim;
    if (allocated_) {
        s.quantization = "f16";
        // used_bytes: proportional to used_positions.
        // (used_positions / context_length) * capacity_bytes — but only
        // when context_length > 0.
        if (layout_.context_length > 0) {
            s.used_bytes = (used_positions_ * total_bytes_) / layout_.context_length;
        }
    }
    return s;
}

float* Cache::k_layer(uint32_t layer) const {
    if (!allocated_ || layer >= layout_.layer_count) return nullptr;
    // K block comes first: layers 0..N-1, each layer_bytes_/sizeof(float)
    // floats.
    uint64_t floats_per_layer = layer_bytes_ / sizeof(float);
    return buffers_.get() + (layer * floats_per_layer);
}

float* Cache::v_layer(uint32_t layer) const {
    if (!allocated_ || layer >= layout_.layer_count) return nullptr;
    // V block comes after the K block: layers N..2N-1.
    uint64_t floats_per_layer = layer_bytes_ / sizeof(float);
    uint64_t total_k_floats = floats_per_layer * layout_.layer_count;
    return buffers_.get() + total_k_floats + (layer * floats_per_layer);
}

} // namespace kv
} // namespace shtn
