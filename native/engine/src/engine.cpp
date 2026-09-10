// engine.cpp — SHEYTAN Native Engine core implementation (Phase 1).

#include "shtn/engine.h"

#include "hardware.h"

#include <chrono>
#include <cstring>
#include <mutex>
#include <new>

struct shtn_engine {
    std::mutex mu;
    std::chrono::steady_clock::time_point started_at;
    shtn_hardware_info hardware;
    bool hardware_cached;
};

namespace {

void copy_cstr(char* dst, size_t cap, const char* src) {
    if (cap == 0 || src == nullptr) {
        return;
    }
    const size_t n = std::strlen(src);
    const size_t m = n < cap - 1 ? n : cap - 1;
    std::memcpy(dst, src, m);
    dst[m] = '\0';
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
    delete engine;
}

int32_t shtn_engine_health(const shtn_engine* engine, shtn_health_status* out) {
    if (engine == nullptr || out == nullptr) {
        return SHTN_ERR_INVALID_ARG;
    }

    std::memset(out, 0, sizeof(*out));

    // Phase 1: a created engine is healthy and ready — there is no model
    // loading step yet, so health is derived from instance validity.
    // Later phases will probe the real execution state here.
    out->healthy = 1;
    copy_cstr(out->state, sizeof(out->state), "ready");
    copy_cstr(out->detail, sizeof(out->detail),
              "phase 1 skeleton: lifecycle only, no inference");

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

        // Phase 1: no GPU / accelerator detection on the C++ side (the Go
        // core merges the sysinfo probe for real GPU facts). Counts stay
        // zero — represented but not invented.
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
    out->active_requests = 0; // measured: no generation exists in Phase 1
    copy_cstr(out->state, sizeof(out->state), "ready");

    return SHTN_OK;
}

} // extern "C"
