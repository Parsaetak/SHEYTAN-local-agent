// model.h — the native engine's model concern (Phase 2).
//
// Owns the model lifecycle: unloaded → loading → loaded | failed →
// unloaded. A "load" validates the GGUF file, memory-maps it (lazy —
// tensor data is NOT copied into RAM) and computes the memory plan; an
// "unload" releases the mapping and every cached fact. No inference, no
// KV-cache allocation, no workspace allocation — those are later phases;
// the plan below is arithmetic on metadata only.

#ifndef SHTN_MODEL_H
#define SHTN_MODEL_H

#include "shtn/types.h"

#include "gguf.h"

#include <mutex>
#include <string>

namespace shtn {
namespace model {

// Fixed engine bookkeeping allowance factored into the memory plan:
// thread stacks, allocator arenas, mmap page-table pressure. It is a
// documented planning constant — nothing is pre-allocated for it.
constexpr uint64_t kRuntimeOverheadBytes = 64u << 20; // 64 MiB

// Model is the single model slot of one engine instance. Every public
// call is thread-safe (internal mutex).
class Model {
public:
    Model() = default;
    ~Model() = default;

    Model(const Model&) = delete;
    Model& operator=(const Model&) = delete;

    // Load `path` (GGUF): validate, map, extract metadata, plan. Replace
    // semantics: a currently loaded model is unloaded first, and a
    // failed load leaves NOTHING loaded. Returns SHTN_OK, or a negative
    // error code with `error` filled for the caller's response.
    int32_t load(const std::string& path,
                 const shtn_model_load_options& opts, std::string& error);

    // Unload and release everything. Idempotent. Returns SHTN_OK.
    int32_t unload();

    // Snapshot into the ABI structs. Never fails (NULL out is guarded by
    // the caller layer); reflects the current state exactly.
    void fill_info(shtn_model_info* out) const;
    void fill_plan(shtn_memory_plan* out) const;

    // Current state string (SHTN_MODEL_STATE_*).
    std::string state() const;

private:
    // Guards every field below. `load` holds the lock for the whole
    // attempt: loads are serialized (a load in flight rejects a second
    // load with SHTN_ERR_MODEL_STATE), while inspections may run
    // concurrently against the last stable snapshot.
    mutable std::mutex mu_;

    std::string state_ = SHTN_MODEL_STATE_UNLOADED;
    std::string error_;        // last load failure detail
    std::string path_;

    gguf::MappedFile mapping_; // owns the mmap + fd
    gguf::GgufHeader header_;  // parsed + validated header facts

    shtn_memory_plan plan_{};  // last computed plan (0s before load)
};

} // namespace model
} // namespace shtn

#endif /* SHTN_MODEL_H */
