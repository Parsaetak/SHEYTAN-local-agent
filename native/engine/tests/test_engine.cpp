// test_engine.cpp — C ABI contract tests (dependency-free asserts).

#include "shtn/engine.h"

#include <cmath>
#include <cstdio>
#include <cstring>

static int failures = 0;

#define CHECK(cond)                                                        \
    do {                                                                   \
        if (!(cond)) {                                                     \
            std::fprintf(stderr, "FAIL %s:%d: %s\n", __FILE__, __LINE__,    \
                         #cond);                                           \
            ++failures;                                                    \
        }                                                                  \
    } while (0)

int main() {
    // ABI version sanity: must match the version the Go core pins.
    CHECK(shtn_abi_version() == SHTN_ABI_VERSION);
    CHECK(shtn_abi_version() == 1u); // Phase 1

    // --- create/destroy round trip --------------------------------------
    {
        shtn_engine* engine = nullptr;
        shtn_engine_options opts{};
        opts.abi_version = SHTN_ABI_VERSION;
        opts.reserved = 0;

        CHECK(shtn_engine_create(&opts, &engine) == SHTN_OK);
        CHECK(engine != nullptr);
        shtn_engine_destroy(engine);
    }

    // --- NULL-argument rejection (never dereferenced) --------------------
    {
        CHECK(shtn_engine_create(nullptr, nullptr) == SHTN_ERR_INVALID_ARG);

        shtn_engine* engine = nullptr;
        shtn_engine_options opts{};
        opts.abi_version = SHTN_ABI_VERSION;

        CHECK(shtn_engine_create(&opts, nullptr) == SHTN_ERR_INVALID_ARG);
        CHECK(shtn_engine_create(nullptr, &engine) == SHTN_ERR_INVALID_ARG);

        shtn_engine_destroy(nullptr); // must be a safe no-op

        CHECK(shtn_engine_health(nullptr, nullptr) == SHTN_ERR_INVALID_ARG);
        CHECK(shtn_engine_hardware_info(nullptr, nullptr) == SHTN_ERR_INVALID_ARG);
        CHECK(shtn_engine_metrics(nullptr, nullptr) == SHTN_ERR_INVALID_ARG);
    }

    // --- ABI mismatch fails closed ---------------------------------------
    {
        shtn_engine* engine = nullptr;
        shtn_engine_options opts{};
        opts.abi_version = SHTN_ABI_VERSION + 1u; // wrong on purpose
        opts.reserved = 0;

        CHECK(shtn_engine_create(&opts, &engine) == SHTN_ERR_ABI);
        CHECK(engine == nullptr);
    }

    // --- health -----------------------------------------------------------
    {
        shtn_engine* engine = nullptr;
        shtn_engine_options opts{};
        opts.abi_version = SHTN_ABI_VERSION;

        CHECK(shtn_engine_create(&opts, &engine) == SHTN_OK);

        shtn_health_status health{};
        CHECK(shtn_engine_health(engine, &health) == SHTN_OK);
        CHECK(health.healthy == 1);
        CHECK(std::strcmp(health.state, "ready") == 0);

        shtn_engine_destroy(engine);
    }

    // --- hardware info: detected values only ------------------------------
    {
        shtn_engine* engine = nullptr;
        shtn_engine_options opts{};
        opts.abi_version = SHTN_ABI_VERSION;

        CHECK(shtn_engine_create(&opts, &engine) == SHTN_OK);

        shtn_hardware_info hw{};
        CHECK(shtn_engine_hardware_info(engine, &hw) == SHTN_OK);

        // Architecture is compile-time knowledge: always present.
        CHECK(hw.architecture[0] != '\0');

        // Logical cores are detectable on every supported platform.
        CHECK(hw.cpu.logical_cores > 0);

        // Total RAM is detectable on every supported platform.
        CHECK(hw.ram.total_bytes > 0);

        // Phase 1: no C++ GPU/accelerator detection — counts must be
        // exactly zero (represented, not invented).
        CHECK(hw.gpu_count == 0);
        CHECK(hw.accelerator_count == 0);

        // The detection source must be recorded.
        CHECK(hw.detected_by[0] != '\0');

        shtn_engine_destroy(engine);
    }

    // --- metrics: measured values only ------------------------------------
    {
        shtn_engine* engine = nullptr;
        shtn_engine_options opts{};
        opts.abi_version = SHTN_ABI_VERSION;

        CHECK(shtn_engine_create(&opts, &engine) == SHTN_OK);

        shtn_metrics m{};
        CHECK(shtn_engine_metrics(engine, &m) == SHTN_OK);

        CHECK(m.uptime_seconds >= 0.0);
        CHECK(m.process_rss_bytes > 0);     // RSS is really measured
        CHECK(m.active_requests == 0);      // no generation exists
        CHECK(std::strcmp(m.state, "ready") == 0);

        // Repeated reads must be monotonically non-decreasing in uptime.
        shtn_metrics m2{};
        CHECK(shtn_engine_metrics(engine, &m2) == SHTN_OK);
        CHECK(m2.uptime_seconds >= m.uptime_seconds);

        shtn_engine_destroy(engine);
    }

    if (failures > 0) {
        std::fprintf(stderr, "test_engine: %d failure(s)\n", failures);
        return 1;
    }

    std::printf("test_engine: all checks passed\n");
    return 0;
}
