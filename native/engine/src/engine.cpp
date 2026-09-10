// engine.cpp — SHEYTAN Native Engine core implementation (Phase 4).
//
// Phase 4 adds the tokenizer / KV-cache / scheduler foundation:
// shtn_engine_tokenizer_init / tokenizer_info / tokenizer_encode /
// tokenizer_decode / kv_cache_info / scheduler_info. These are REAL:
// the tokenizer reads GGUF arrays, the KV cache is a real allocation
// sized from model dims, the scheduler is a real bounded queue. They
// are NOT inference — no forward pass exists, no token is generated.
// The llama.cpp fallback remains the generation backend.
//
// Phase 2 added the model concern: shtn_engine_load_model /
// shtn_engine_unload_model / shtn_engine_model_info /
// shtn_engine_memory_plan (see model.h). The lifecycle, health,
// hardware and metrics surfaces are unchanged from Phase 1; health's
// detail string now summarizes the model concern.

#include "shtn/engine.h"

#include "hardware.h"
#include "kv_cache.h"
#include "model.h"
#include "scheduler.h"
#include "tokenizer.h"
#include "util.h"

#include <chrono>
#include <cstring>
#include <memory>
#include <mutex>
#include <new>
#include <string>

struct shtn_engine {
    std::mutex mu;
    std::chrono::steady_clock::time_point started_at;
    shtn_hardware_info hardware;
    bool hardware_cached;

    // Model concern (guarded by its own mutex inside Model). The Model
    // owns the tokenizer vocab too (Phase 4).
    shtn::model::Model model;

    // Phase 4: KV cache + scheduler owned by the engine. The KV cache
    // is NOT auto-allocated on model load — it exists as a real
    // data structure but is only measured (and the host reports the
    // honest zero-state until a forward pass exists).
    shtn::kv::Cache kv_cache;
    shtn::sched::Scheduler scheduler;
};

namespace {

using shtn::copy_cstr;

void summarize_model(const shtn::model::Model& model, char* dst, size_t cap) {
    shtn_model_info info{};
    model.fill_info(&info);

    if (info.state[0] == '\0' ||
        std::strcmp(info.state, SHTN_MODEL_STATE_UNLOADED) == 0) {
        copy_cstr(dst, cap, "no model loaded");
        return;
    }

    if (std::strcmp(info.state, SHTN_MODEL_STATE_LOADED) == 0) {
        std::string s = "model loaded: ";
        s += info.architecture;
        s += " (";
        s += info.quantization[0] != '\0' ? info.quantization : "unknown quant";
        s += ")";
        copy_cstr(dst, cap, s.c_str());
        return;
    }

    if (std::strcmp(info.state, SHTN_MODEL_STATE_FAILED) == 0) {
        std::string s = "model load failed: ";
        s += info.error;
        copy_cstr(dst, cap, s.c_str());
        return;
    }

    copy_cstr(dst, cap, "model loading");
}

} // namespace

extern "C" {

uint32_t shtn_abi_version(void) {
    return SHTN_ABI_VERSION;
}

int32_t shtn_engine_create(const shtn_engine_options* opts, shtn_engine** out) {
    if (opts == nullptr || out == nullptr) {
        return SHTN_ERR_INVALID_ARG;
    }

    if (opts->abi_version != SHTN_ABI_VERSION) {
        return SHTN_ERR_ABI;
    }

    if (opts->reserved != 0) {
        return SHTN_ERR_INVALID_ARG;
    }

    auto* engine = new (std::nothrow) shtn_engine();
    if (engine == nullptr) {
        return SHTN_ERR_INTERNAL;
    }

    engine->started_at = std::chrono::steady_clock::now();
    engine->hardware_cached = false;

    *out = engine;
    return SHTN_OK;
}

void shtn_engine_destroy(shtn_engine* engine) {
    // The Model destructor releases the mapping via its members; the
    // explicit unload keeps the release path identical to the API path.
    if (engine != nullptr) {
        engine->model.unload();
    }
    delete engine;
}

int32_t shtn_engine_health(const shtn_engine* engine, shtn_health_status* out) {
    if (engine == nullptr || out == nullptr) {
        return SHTN_ERR_INVALID_ARG;
    }

    std::memset(out, 0, sizeof(*out));

    // The engine is healthy while it exists; a FAILED model load is a
    // model-concern failure (visible in the detail), not an engine fault.
    out->healthy = 1;
    copy_cstr(out->state, sizeof(out->state), "ready");
    summarize_model(engine->model, out->detail, sizeof(out->detail));

    return SHTN_OK;
}

int32_t shtn_engine_hardware_info(shtn_engine* engine, shtn_hardware_info* out) {
    if (engine == nullptr || out == nullptr) {
        return SHTN_ERR_INVALID_ARG;
    }

    std::lock_guard<std::mutex> lock(engine->mu);

    if (!engine->hardware_cached) {
        shtn_hardware_info& hw = engine->hardware;
        std::memset(&hw, 0, sizeof(hw));

        copy_cstr(hw.architecture, sizeof(hw.architecture), shtn::architecture_name());
        shtn::detect_cpu(hw.cpu);
        shtn::detect_ram(hw.ram);

        // No GPU / accelerator detection on the C++ side (the Go core
        // merges the sysinfo probe for real GPU facts). Counts stay
        // zero — represented, not invented.
        hw.gpu_count = 0;
        hw.accelerator_count = 0;

        copy_cstr(hw.detected_by, sizeof(hw.detected_by), "shtn-engine-cpp");

        engine->hardware_cached = true;
    }

    *out = engine->hardware;
    return SHTN_OK;
}

int32_t shtn_engine_metrics(shtn_engine* engine, shtn_metrics* out) {
    if (engine == nullptr || out == nullptr) {
        return SHTN_ERR_INVALID_ARG;
    }

    std::lock_guard<std::mutex> lock(engine->mu);

    std::memset(out, 0, sizeof(*out));

    const auto uptime = std::chrono::steady_clock::now() - engine->started_at;
    out->uptime_seconds = std::chrono::duration<double>(uptime).count();
    out->process_rss_bytes = shtn::current_process_rss();
    out->active_requests = 0; // measured: no generation exists yet
    copy_cstr(out->state, sizeof(out->state), "ready");

    return SHTN_OK;
}

// --- Phase 2: model surface ----------------------------------------------

int32_t shtn_engine_load_model(shtn_engine* engine, const char* path,
                               const shtn_model_load_options* opts) {
    if (engine == nullptr || path == nullptr) {
        return SHTN_ERR_INVALID_ARG;
    }

    shtn_model_load_options defaults{};
    const shtn_model_load_options& options =
        opts != nullptr ? *opts : defaults;

    std::string error;
    const int32_t rc = engine->model.load(path, options, error);
    if (rc != SHTN_OK && error.empty()) {
        error = "model load failed with error code " + std::to_string(rc);
    }
    return rc;
}

int32_t shtn_engine_unload_model(shtn_engine* engine) {
    if (engine == nullptr) {
        return SHTN_ERR_INVALID_ARG;
    }

    return engine->model.unload();
}

int32_t shtn_engine_model_info(const shtn_engine* engine,
                               shtn_model_info* out) {
    if (engine == nullptr || out == nullptr) {
        return SHTN_ERR_INVALID_ARG;
    }

    engine->model.fill_info(out);
    return SHTN_OK;
}

int32_t shtn_engine_memory_plan(const shtn_engine* engine,
                                shtn_memory_plan* out) {
    if (engine == nullptr || out == nullptr) {
        return SHTN_ERR_INVALID_ARG;
    }

    engine->model.fill_plan(out);
    return SHTN_OK;
}

// --- Phase 4: tokenizer / KV / scheduler surface --------------------------

// fill_tokenizer_info copies the materialized vocab snapshot (or the
// zero-state) into the ABI struct.
namespace {

void fill_tokenizer_info(const shtn_engine* engine,
                         shtn_tokenizer_info* out) {
    std::memset(out, 0, sizeof(*out));

    const shtn::tokenizer::Vocab* v = engine->model.tokenizer_vocab();
    if (v == nullptr) {
        // Not initialized — leave everything zero. The host reports
        // initialized=0 honestly.
        return;
    }

    shtn::tokenizer::InfoSnapshot snap;
    shtn::tokenizer::fill_info(*v, snap);

    out->initialized = snap.initialized ? 1 : 0;
    out->vocab_size = snap.vocab_size;
    out->merge_count = snap.merge_count;
    out->has_bos = snap.has_bos ? 1 : 0;
    out->has_eos = snap.has_eos ? 1 : 0;
    out->has_unknown = snap.has_unknown ? 1 : 0;
    out->bos_id = snap.bos_id;
    out->eos_id = snap.eos_id;
    out->unknown_id = snap.unknown_id;

    const char* model_str = "unsupported";
    switch (snap.model) {
    case shtn::tokenizer::Model::kBPE:     model_str = "bpe"; break;
    case shtn::tokenizer::Model::kUnigram: model_str = "unigram"; break;
    case shtn::tokenizer::Model::kWPM:     model_str = "wpm"; break;
    default: break;
    }
    shtn::copy_cstr(out->model, sizeof(out->model), model_str);
    shtn::copy_cstr(out->model_name, sizeof(out->model_name),
                    snap.model_name.c_str());
}

void fill_kv_cache_info(const shtn_engine* engine,
                        shtn_kv_cache_info* out) {
    std::memset(out, 0, sizeof(*out));

    shtn::kv::Stats s = engine->kv_cache.stats();
    out->allocated = s.allocated ? 1 : 0;
    out->capacity_bytes = s.capacity_bytes;
    out->used_bytes = s.used_bytes;
    out->capacity_positions = s.capacity_positions;
    out->used_positions = s.used_positions;
    out->layer_count = s.layer_count;
    out->kv_dim = s.kv_dim;
    shtn::copy_cstr(out->quantization, sizeof(out->quantization),
                    s.quantization.c_str());
}

void fill_scheduler_info(const shtn_engine* engine,
                         shtn_scheduler_info* out) {
    std::memset(out, 0, sizeof(*out));

    shtn::sched::Stats s = engine->scheduler.stats();
    out->active_requests = s.active_requests;
    out->queued_requests = s.queued_requests;
    out->max_concurrent = s.max_concurrent;
    out->queue_depth_limit = s.queue_depth_limit;
    out->total_submitted = s.total_submitted;
    out->total_completed = s.total_completed;
    out->total_cancelled = s.total_cancelled;
    out->total_failed = s.total_failed;
    out->shutting_down = s.shutting_down ? 1 : 0;
}

} // namespace

int32_t shtn_engine_tokenizer_init(shtn_engine* engine,
                                   shtn_tokenizer_info* out) {
    if (engine == nullptr || out == nullptr) {
        return SHTN_ERR_INVALID_ARG;
    }

    std::string error;
    const int32_t rc = engine->model.init_tokenizer(error);

    // Always fill the info struct so the caller sees the result.
    fill_tokenizer_info(engine, out);
    if (rc != SHTN_OK && !error.empty()) {
        shtn::copy_cstr(out->error, sizeof(out->error), error.c_str());
    }
    return rc;
}

int32_t shtn_engine_tokenizer_info(const shtn_engine* engine,
                                   shtn_tokenizer_info* out) {
    if (engine == nullptr || out == nullptr) {
        return SHTN_ERR_INVALID_ARG;
    }
    fill_tokenizer_info(engine, out);
    return SHTN_OK;
}

int32_t shtn_engine_tokenizer_encode(const shtn_engine* engine,
                                     const char* text, uint64_t text_len,
                                     const shtn_encode_options* opts,
                                     shtn_encode_result* result) {
    if (engine == nullptr || text == nullptr || opts == nullptr ||
        result == nullptr || result->ids == nullptr) {
        return SHTN_ERR_INVALID_ARG;
    }
    if (opts->reserved != 0) {
        return SHTN_ERR_INVALID_ARG;
    }
    if (opts->max_tokens == 0) {
        return SHTN_ERR_INVALID_ARG;
    }

    result->ids_count = 0;
    result->truncated = 0;

    const shtn::tokenizer::Vocab* v = engine->model.tokenizer_vocab();
    if (v == nullptr) {
        return SHTN_ERR_NO_MODEL;
    }

    shtn::tokenizer::EncodeOptions eopts;
    eopts.add_bos = opts->add_bos != 0;
    eopts.add_eos = opts->add_eos != 0;
    eopts.max_tokens = opts->max_tokens;

    std::string input(text, static_cast<size_t>(text_len));

    shtn::tokenizer::EncodeResult er;
    std::string error;
    const int32_t rc = shtn::tokenizer::encode(*v, input, eopts, er, error);
    if (rc != SHTN_OK) {
        return rc;
    }

    // Copy into the caller's buffer (bounded by max_tokens).
    uint32_t cap = opts->max_tokens;
    uint32_t n = static_cast<uint32_t>(er.ids.size());
    if (n > cap) {
        n = cap;
        result->truncated = 1;
    } else if (er.truncated) {
        result->truncated = 1;
    }
    for (uint32_t i = 0; i < n; ++i) {
        result->ids[i] = er.ids[i];
    }
    result->ids_count = n;

    return SHTN_OK;
}

int32_t shtn_engine_tokenizer_decode(const shtn_engine* engine,
                                     const uint32_t* ids, uint64_t ids_count,
                                     const shtn_decode_options* opts,
                                     shtn_decode_result* result) {
    if (engine == nullptr || opts == nullptr || result == nullptr ||
        result->text == nullptr) {
        return SHTN_ERR_INVALID_ARG;
    }
    if (opts->reserved != 0) {
        return SHTN_ERR_INVALID_ARG;
    }
    if (opts->max_bytes == 0) {
        return SHTN_ERR_INVALID_ARG;
    }
    if (ids == nullptr && ids_count > 0) {
        return SHTN_ERR_INVALID_ARG;
    }

    result->text_count = 0;
    result->truncated = 0;
    result->text[0] = '\0';

    const shtn::tokenizer::Vocab* v = engine->model.tokenizer_vocab();
    if (v == nullptr) {
        return SHTN_ERR_NO_MODEL;
    }

    shtn::tokenizer::DecodeOptions dopts;
    dopts.skip_special = opts->skip_special != 0;
    dopts.max_bytes = opts->max_bytes;

    shtn::tokenizer::DecodeResult dr;
    std::string error;
    const int32_t rc = shtn::tokenizer::decode(*v, ids,
        static_cast<size_t>(ids_count), dopts, dr, error);
    if (rc != SHTN_OK) {
        return rc;
    }

    uint32_t cap = opts->max_bytes;
    uint32_t n = static_cast<uint32_t>(dr.text.size());
    if (n >= cap) {
        n = cap - 1; // leave room for NUL
        result->truncated = 1;
    } else if (dr.truncated) {
        result->truncated = 1;
    }
    std::memcpy(result->text, dr.text.data(), n);
    result->text[n] = '\0';
    result->text_count = n;

    return SHTN_OK;
}

int32_t shtn_engine_kv_cache_info(const shtn_engine* engine,
                                  shtn_kv_cache_info* out) {
    if (engine == nullptr || out == nullptr) {
        return SHTN_ERR_INVALID_ARG;
    }
    fill_kv_cache_info(engine, out);
    return SHTN_OK;
}

int32_t shtn_engine_scheduler_info(const shtn_engine* engine,
                                   shtn_scheduler_info* out) {
    if (engine == nullptr || out == nullptr) {
        return SHTN_ERR_INVALID_ARG;
    }
    fill_scheduler_info(engine, out);
    return SHTN_OK;
}

} // extern "C"
