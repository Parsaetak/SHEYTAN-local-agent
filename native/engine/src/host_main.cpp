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
//
// Phase 2 ops: load_model / unload_model / model_info — the native GGUF
// loading surface (validate + memory-map + metadata + memory plan; no
// inference in this phase).

#include "shtn/engine.h"
#include "shtn/types.h"
#include "shtn/version.h"

#include "json.h"
#include "protocol.h"

#include <cstdio>
#include <cstring>
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

    // The model concern rides along as an honest snapshot (state only;
    // full metadata is the model_info op's job).
    shtn_model_info mi{};
    shtn_engine_model_info(engine, &mi);

    std::ostringstream oss;
    oss << "{"
        << "\"engineState\":" << quote(m.state)
        << ",\"uptimeSeconds\":" << json_num(m.uptime_seconds)
        << ",\"processRssBytes\":" << json_uint(m.process_rss_bytes)
        << ",\"modelState\":" << quote(mi.state)
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
    // No generation requests exist in this phase, so a cancel is
    // honestly reported as a miss (the Go side surfaces the reason).
    std::ostringstream oss;
    oss << "{"
        << "\"cancelled\":false"
        << ",\"reason\":\"no active generation requests (generation is a later phase)\""
        << "}";
    return oss.str();
}

// model_info_json renders one shtn_model_info snapshot (only fields the
// engine actually read or derived; zeros/empties stay zero/empty).
std::string model_info_json(const shtn_model_info& mi) {
    std::ostringstream oss;
    oss << "{"
        << "\"path\":" << quote(mi.path)
        << ",\"architecture\":" << quote(mi.architecture)
        << ",\"name\":" << quote(mi.name)
        << ",\"quantization\":" << quote(mi.quantization)
        << ",\"state\":" << quote(mi.state)
        << ",\"error\":" << quote(mi.error)
        << ",\"fileSizeBytes\":" << json_uint(mi.file_size_bytes)
        << ",\"parameterCount\":" << json_uint(mi.parameter_count)
        << ",\"contextLength\":" << json_uint(mi.context_length)
        << ",\"vocabularySize\":" << json_uint(mi.vocabulary_size)
        << ",\"embeddingLength\":" << json_uint(mi.embedding_length)
        << ",\"layerCount\":" << json_uint(mi.layer_count)
        << ",\"tensorCount\":" << mi.tensor_count
        << ",\"ggufVersion\":" << mi.gguf_version
        << ",\"fileType\":" << mi.general_file_type
        << ",\"hasFileType\":" << (mi.has_file_type ? "true" : "false")
        << ",\"kvCacheBytes\":" << json_uint(mi.kv_cache_bytes)
        << ",\"workspaceBytes\":" << json_uint(mi.workspace_bytes)
        << ",\"totalPlanBytes\":" << json_uint(mi.total_plan_bytes)
        << "}";
    return oss.str();
}

std::string memory_plan_json(const shtn_memory_plan& mp) {
    std::ostringstream oss;
    oss << "{"
        << "\"modelFileBytes\":" << json_uint(mp.model_file_bytes)
        << ",\"mappedBytes\":" << json_uint(mp.mapped_bytes)
        << ",\"weightsBytes\":" << json_uint(mp.weights_bytes)
        << ",\"workspaceBytes\":" << json_uint(mp.workspace_bytes)
        << ",\"kvCacheBytes\":" << json_uint(mp.kv_cache_bytes)
        << ",\"runtimeOverheadBytes\":" << json_uint(mp.runtime_overhead_bytes)
        << ",\"totalBytes\":" << json_uint(mp.total_bytes)
        << ",\"availableRamBytes\":" << json_uint(mp.available_ram_bytes)
        << ",\"fitsInRam\":" << mp.fits_in_ram
        << "}";
    return oss.str();
}

// op_load_model validates the payload, runs the native load and renders
// the post-load model snapshot. Errors are thrown as readable strings
// (the dispatch turns them into bounded error frames).
std::string op_load_model(shtn_engine* engine, const std::string& payload_raw,
                          bool has_payload) {
    std::string path;
    uint32_t context_length = 0;
    std::string err;

    if (!has_payload) {
        throw std::string("load_model requires a payload");
    }

    if (!shtn::json::extract_load_payload(payload_raw, path, context_length,
                                          err)) {
        throw std::string("malformed load_model payload: ") + err;
    }

    shtn_model_load_options opts{};
    opts.context_length = context_length;
    opts.reserved = 0;

    const int32_t rc = shtn_engine_load_model(engine, path.c_str(), &opts);

    if (rc != SHTN_OK) {
        // The model snapshot carries the failure detail — surface it in
        // the error response instead of a bare code.
        shtn_model_info mi{};
        shtn_engine_model_info(engine, &mi);

        std::string msg = "load failed";
        if (mi.error[0] != '\0') {
            msg = mi.error;
        } else {
            msg += " (error code " + std::to_string(rc) + ")";
        }
        throw msg;
    }

    shtn_model_info mi{};
    shtn_memory_plan mp{};

    shtn_engine_model_info(engine, &mi);
    shtn_engine_memory_plan(engine, &mp);

    std::ostringstream oss;
    oss << "{"
        << "\"loaded\":true"
        << ",\"state\":" << quote(mi.state)
        << ",\"model\":" << model_info_json(mi)
        << ",\"memory\":" << memory_plan_json(mp)
        << "}";
    return oss.str();
}

std::string op_unload_model(shtn_engine* engine) {
    const int32_t rc = shtn_engine_unload_model(engine);

    if (rc != SHTN_OK) {
        throw std::string("unload failed (error code ") + std::to_string(rc) + ")";
    }

    std::ostringstream oss;
    oss << "{"
        << "\"loaded\":false"
        << ",\"state\":" << quote(SHTN_MODEL_STATE_UNLOADED)
        << "}";
    return oss.str();
}

std::string op_model_info(shtn_engine* engine) {
    shtn_model_info mi{};
    shtn_memory_plan mp{};

    const int32_t rc = shtn_engine_model_info(engine, &mi);
    if (rc != SHTN_OK) {
        throw std::string("model_info failed (error code ") +
            std::to_string(rc) + ")";
    }

    shtn_engine_memory_plan(engine, &mp);

    const bool loaded = std::strcmp(mi.state, SHTN_MODEL_STATE_LOADED) == 0;

    std::ostringstream oss;
    oss << "{"
        << "\"loaded\":" << (loaded ? "true" : "false")
        << ",\"state\":" << quote(mi.state);

    // With nothing attempted, model/memory stay absent — the snapshot is
    // honest about "nothing to report".
    if (mi.state[0] != '\0' &&
        std::strcmp(mi.state, SHTN_MODEL_STATE_UNLOADED) != 0) {
        oss << ",\"model\":" << model_info_json(mi)
            << ",\"memory\":" << memory_plan_json(mp);
    }

    oss << "}";
    return oss.str();
}

// --- dispatch ----------------------------------------------------------------

// handle_request processes one parsed request and returns the RESULT JSON
// (success) or throws a std::string error message (failure).
std::string handle_request(shtn_engine* engine, const shtn::json::Parsed& req) {
    if (req.op == "ping") {
        return op_ping();
    }
    if (req.op == "health") {
        return op_health(engine);
    }
    if (req.op == "hwinfo") {
        return op_hardware(engine);
    }
    if (req.op == "metrics") {
        return op_metrics(engine);
    }
    if (req.op == "cancel") {
        return op_cancel();
    }
    if (req.op == "load_model") {
        return op_load_model(engine, req.payload_raw, req.has_payload);
    }
    if (req.op == "unload_model") {
        return op_unload_model(engine);
    }
    if (req.op == "model_info") {
        return op_model_info(engine);
    }
    if (req.op == "shutdown") {
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
            const std::string result = handle_request(engine, parsed);

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
              << " (phase 2: lifecycle, health, hardware, metrics, GGUF model loading)"
              << std::endl;

    return shtn_host_run(std::cin, std::cout);
}

#endif /* SHTN_HOST_NO_MAIN */
