// host_main.cpp — the supervised native engine host (shtn-engine-host).
//
// The Go core spawns this process and speaks the length-prefixed JSON
// protocol over stdin/stdout (see internal/native/engine/protocol.go).
// stderr carries human-readable diagnostics (ring-buffered by Go).
//
// Dispatch loop rules:
//   - one request frame in → exactly one response frame out;
//   - malformed input (bad JSON, unknown op, oversized frame) gets a
//     bounded ERROR response — the host NEVER crashes and never exits
//     on bad input;
//   - "shutdown" acknowledges and exits cleanly;
//   - stdin EOF exits cleanly (the Go side closes stdin on stop);
//   - every op is coarse-grained; there is no per-token traffic.

#include "shtn/engine.h"
#include "shtn/types.h"
#include "shtn/version.h"

#include "json.h"
#include "protocol.h"

#include <cstdio>
#include <iostream>
#include <sstream>
#include <string>

#if defined(_WIN32)
#include <fcntl.h>
#include <io.h>
#endif

namespace {

using shtn::json::quote;

// json_num renders a double without trailing garbage for integral values.
std::string json_num(double v) {
    std::ostringstream oss;
    oss << v;
    return oss.str();
}

std::string json_uint(unsigned long long v) {
    std::ostringstream oss;
    oss << v;
    return oss.str();
}

// --- op handlers ------------------------------------------------------------

std::string op_ping() {
    std::ostringstream oss;
    oss << "{"
        << "\"protocolVersion\":" << SHTN_PROTOCOL_VERSION
        << ",\"abiVersion\":" << SHTN_ABI_VERSION
        << ",\"engine\":" << quote(SHTN_ENGINE_NAME)
        << "}";
    return oss.str();
}

std::string op_health(shtn_engine* engine) {
    shtn_health_status health{};

    const int32_t rc = shtn_engine_health(engine, &health);

    if (rc != SHTN_OK) {
        throw std::string("health failed with error code ") + std::to_string(rc);
    }

    std::ostringstream oss;
    oss << "{"
        << "\"healthy\":" << (health.healthy ? "true" : "false")
        << ",\"state\":" << quote(health.state)
        << ",\"detail\":" << quote(health.detail)
        << "}";
    return oss.str();
}

std::string op_hardware(shtn_engine* engine) {
    shtn_hardware_info hw{};

    const int32_t rc = shtn_engine_hardware_info(engine, &hw);

    if (rc != SHTN_OK) {
        throw std::string("hwinfo failed with error code ") + std::to_string(rc);
    }

    std::ostringstream oss;
    oss << "{"
        << "\"architecture\":" << quote(hw.architecture)
        << ",\"cpu\":{"
        << "\"name\":" << quote(hw.cpu.name)
        << ",\"physicalCores\":" << hw.cpu.physical_cores
        << ",\"logicalCores\":" << hw.cpu.logical_cores
        << ",\"frequencyMHz\":" << (hw.cpu.frequency_hz / 1000000)
        << "},\"ram\":{"
        << "\"totalBytes\":" << json_uint(hw.ram.total_bytes)
        << ",\"availableBytes\":" << json_uint(hw.ram.available_bytes)
        << "},\"gpus\":[],\"accelerators\":[]"
        << "}";
    return oss.str();
}

std::string op_metrics(shtn_engine* engine) {
    shtn_metrics m{};

    const int32_t rc = shtn_engine_metrics(engine, &m);

    if (rc != SHTN_OK) {
        throw std::string("metrics failed with error code ") + std::to_string(rc);
    }

    std::ostringstream oss;
    oss << "{"
        << "\"engineState\":" << quote(m.state)
        << ",\"uptimeSeconds\":" << json_num(m.uptime_seconds)
        << ",\"processRssBytes\":" << json_uint(m.process_rss_bytes)
        << ",\"scheduler\":{"
        << "\"activeRequests\":" << m.active_requests
        << ",\"maxConcurrentRequests\":1"
        << "},\"memory\":{"
        << "\"nativeRssBytes\":" << json_uint(m.process_rss_bytes)
        << "},\"kv\":{"
        << "\"strategy\":\"none\""
        << "},\"generation\":{"
        << "\"activeRequests\":" << m.active_requests
        << "}"
        << "}";
    return oss.str();
}

std::string op_cancel() {
    // Phase 1: no generation requests exist, so a cancel is honestly
    // reported as a miss (the Go side surfaces the reason).
    std::ostringstream oss;
    oss << "{"
        << "\"cancelled\":false"
        << ",\"reason\":\"no active generation requests (phase 1 skeleton)\""
        << "}";
    return oss.str();
}

// --- dispatch ----------------------------------------------------------------

// handle_request processes one parsed request and returns the RESULT JSON
// (success) or throws a std::string error message (failure).
std::string handle_request(shtn_engine* engine, const std::string& op) {
    if (op == "ping") {
        return op_ping();
    }
    if (op == "health") {
        return op_health(engine);
    }
    if (op == "hwinfo") {
        return op_hardware(engine);
    }
    if (op == "metrics") {
        return op_metrics(engine);
    }
    if (op == "cancel") {
        return op_cancel();
    }
    if (op == "shutdown") {
        // Acknowledged by the caller specially (respond, then exit).
        return "{}";
    }

    throw std::string("unknown op");
}

// error_response builds the bounded failure frame for a request id.
std::string error_response(int64_t id, const std::string& message) {
    std::ostringstream oss;
    oss << "{"
        << "\"id\":" << id
        << ",\"ok\":false"
        << ",\"error\":" << quote(message)
        << "}";
    return oss.str();
}

// success_response wraps a result payload.
std::string success_response(int64_t id, const std::string& result) {
    std::ostringstream oss;
    oss << "{"
        << "\"id\":" << id
        << ",\"ok\":true"
        << ",\"result\":" << result
        << "}";
    return oss.str();
}

// host_loop is the testable core: read frames from in, write response
// frames to out, until EOF or shutdown. Returns the process exit code.
int host_loop(std::istream& in, std::ostream& out, shtn_engine* engine) {
    bool shutting_down = false;

    for (;;) {
        std::string payload;

        switch (shtn::protocol::read_frame(in, payload)) {
        case shtn::protocol::FrameStatus::Ok:
            break;
        case shtn::protocol::FrameStatus::Eof:
            return shutting_down ? 0 : 0; // clean close either way
        case shtn::protocol::FrameStatus::TooLarge:
            // Cannot correlate to an id (frame unreadable): log and exit —
            // the supervisor restarts us. This is the one non-clean exit:
            // a peer sending >1 MiB frames is broken beyond recovery.
            std::fprintf(stderr, "[shtn-host] protocol violation: frame exceeds cap\n");
            return 2;
        case shtn::protocol::FrameStatus::Truncated:
            std::fprintf(stderr, "[shtn-host] protocol violation: truncated frame\n");
            return 2;
        case shtn::protocol::FrameStatus::Invalid:
            std::fprintf(stderr, "[shtn-host] protocol violation: invalid frame\n");
            return 2;
        }

        const shtn::json::Parsed parsed = shtn::json::parse_request(payload);

        if (!parsed.valid) {
            // Malformed request: bounded error response, keep serving.
            shtn::protocol::write_frame(
                out, error_response(0, "malformed request: " + parsed.error));
            continue;
        }

        try {
            const std::string result = handle_request(engine, parsed.op);

            if (parsed.op == "shutdown") {
                // Respond first, then exit cleanly.
                shtn::protocol::write_frame(
                    out, success_response(parsed.id, result));
                return 0;
            }

            shtn::protocol::write_frame(
                out, success_response(parsed.id, result));
        } catch (const std::string& err) {
            shtn::protocol::write_frame(
                out, error_response(parsed.id, err));
        }
    }
}

} // namespace

// Exposed for tests (tests/test_host.cpp).
int shtn_host_run(std::istream& in, std::ostream& out) {
    shtn_engine* engine = nullptr;

    shtn_engine_options opts{};
    opts.abi_version = SHTN_ABI_VERSION;
    opts.reserved = 0;

    const int32_t rc = shtn_engine_create(&opts, &engine);

    if (rc != SHTN_OK || engine == nullptr) {
        std::fprintf(stderr, "[shtn-host] engine create failed: %d\n", rc);
        return 1;
    }

    const int exit_code = host_loop(in, out, engine);

    shtn_engine_destroy(engine);

    return exit_code;
}

#ifndef SHTN_HOST_NO_MAIN

int main() {
#if defined(_WIN32)
    // Binary-safe stdio on Windows.
    _setmode(_fileno(stdin), _O_BINARY);
    _setmode(_fileno(stdout), _O_BINARY);
    _setmode(_fileno(stderr), _O_BINARY);
#endif

    std::ios::sync_with_stdio(false);

    std::cerr << "[shtn-host] "
              << "engine=" << SHTN_ENGINE_NAME
              << " abi=" << SHTN_ABI_VERSION
              << " protocol=" << SHTN_PROTOCOL_VERSION
              << " (phase 1 skeleton: lifecycle, health, hardware, metrics)"
              << std::endl;

    return shtn_host_run(std::cin, std::cout);
}

#endif /* SHTN_HOST_NO_MAIN */
