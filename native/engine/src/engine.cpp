// engine.cpp — SHEYTAN Native Engine core implementation (Phase 2).
//
// Phase 2 adds the model concern: shtn_engine_load_model /
// shtn_engine_unload_model / shtn_engine_model_info /
// shtn_engine_memory_plan (see model.h). The lifecycle, health,
// hardware and metrics surfaces are unchanged from Phase 1; health's
// detail string now summarizes the model concern.

#include "shtn/engine.h"

#include "hardware.h"
#include "model.h"
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

    // Model concern (guarded by its own mutex inside Model).
    shtn::model::Model model;
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

} // extern "C"
