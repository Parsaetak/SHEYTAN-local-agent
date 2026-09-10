// test_host.cpp — dispatch loop tests: malformed native requests never
// crash the host; every valid op round-trips; shutdown exits cleanly.

#define SHTN_HOST_NO_MAIN
#include "../src/host_main.cpp"

#include "gguf_writer.h"

#include <cstdio>
#include <fstream>
#include <sstream>
#include <vector>
#include <string>

#ifdef _WIN32
#include <direct.h>
#include <windows.h>
#define S_MKDIR(p) _mkdir(p)
#else
#include <sys/stat.h>
#include <sys/types.h>
#include <unistd.h>
#include <cstdlib>
#define S_MKDIR(p) mkdir((p), 0755)
#endif

static int failures = 0;

#define CHECK(cond)                                                        \
    do {                                                                   \
        if (!(cond)) {                                                     \
            std::fprintf(stderr, "FAIL %s:%d: %s\n", __FILE__, __LINE__,    \
                         #cond);                                           \
            ++failures;                                                    \
        }                                                                  \
    } while (0)

// gguf_test_quote wraps a path as a JSON string (paths are plain ASCII in
// tests; the host's quote() is available via host_main.cpp's includes).
static std::string gguf_test_quote(const std::string& s) {
    std::string out = "\"";
    for (char c : s) {
        if (c == '\\' || c == '"') {
            out.push_back('\\');
        }
        out.push_back(c);
    }
    out.push_back('"');
    return out;
}

// run feeds raw bytes to the host loop and returns its output + exit code.
static std::pair<std::string, int> run(const std::string& input) {
    std::istringstream in(input);
    std::ostringstream out;

    const int rc = shtn_host_run(in, out);

    return {out.str(), rc};
}

// last_frame decodes the final frame of a framed stream.
static std::string last_frame(const std::string& framed) {
    if (framed.size() < 4) {
        return {};
    }

    size_t off = 0;
    std::string last;

    while (off + 4 <= framed.size()) {
        const uint32_t size =
            static_cast<uint32_t>(static_cast<unsigned char>(framed[off])) |
            (static_cast<uint32_t>(static_cast<unsigned char>(framed[off + 1])) << 8) |
            (static_cast<uint32_t>(static_cast<unsigned char>(framed[off + 2])) << 16) |
            (static_cast<uint32_t>(static_cast<unsigned char>(framed[off + 3])) << 24);

        if (off + 4 + size > framed.size()) {
            break; // truncated tail
        }

        last = framed.substr(off + 4, size);
        off += 4 + size;
    }

    return last;
}

static std::string frame(const std::string& payload) {
    const uint32_t size = static_cast<uint32_t>(payload.size());

    std::string out;
    out.push_back(static_cast<char>(size & 0xFF));
    out.push_back(static_cast<char>((size >> 8) & 0xFF));
    out.push_back(static_cast<char>((size >> 16) & 0xFF));
    out.push_back(static_cast<char>((size >> 24) & 0xFF));
    out += payload;

    return out;
}

int main() {
    // --- valid ops round-trip -------------------------------------------------
    {
        const auto [out, rc] = run(frame("{\"id\":1,\"op\":\"ping\"}"));

        CHECK(rc == 0);

        const std::string resp = last_frame(out);
        CHECK(resp.find("\"id\":1") != std::string::npos);
        CHECK(resp.find("\"ok\":true") != std::string::npos);
        CHECK(resp.find("\"protocolVersion\":2") != std::string::npos);
        CHECK(resp.find("\"abiVersion\":2") != std::string::npos);
    }

    {
        const auto [out, rc] = run(frame("{\"id\":2,\"op\":\"health\"}"));

        CHECK(rc == 0);

        const std::string resp = last_frame(out);
        CHECK(resp.find("\"healthy\":true") != std::string::npos);
        CHECK(resp.find("\"state\":\"ready\"") != std::string::npos);
    }

    {
        const auto [out, rc] = run(frame("{\"id\":3,\"op\":\"hwinfo\"}"));

        CHECK(rc == 0);

        const std::string resp = last_frame(out);
        CHECK(resp.find("\"architecture\":") != std::string::npos);
        CHECK(resp.find("\"totalBytes\":") != std::string::npos);
    }

    {
        const auto [out, rc] = run(frame("{\"id\":4,\"op\":\"metrics\"}"));

        CHECK(rc == 0);

        const std::string resp = last_frame(out);
        CHECK(resp.find("\"engineState\":\"ready\"") != std::string::npos);
        CHECK(resp.find("\"scheduler\"") != std::string::npos);
        CHECK(resp.find("\"maxConcurrentRequests\":1") != std::string::npos);
        CHECK(resp.find("\"nativeRssBytes\":") != std::string::npos);
    }

    {
        const auto [out, rc] = run(frame("{\"id\":5,\"op\":\"cancel\",\"payload\":{\"requestId\":\"req-9\"}}"));

        CHECK(rc == 0);

        const std::string resp = last_frame(out);
        CHECK(resp.find("\"cancelled\":false") != std::string::npos);
        CHECK(resp.find("\"reason\"") != std::string::npos);
    }

    // --- unknown op: bounded error response, loop keeps serving ---------------
    {
        std::string input;
        input += frame("{\"id\":10,\"op\":\"make-coffee\"}");
        input += frame("{\"id\":11,\"op\":\"ping\"}");

        const auto [out, rc] = run(input);

        CHECK(rc == 0);

        // First: error for the unknown op.
        const std::string err = last_frame(out.substr(0, out.size()));
        // (last_frame returns the LAST frame; decode both by splitting.)
        std::istringstream stream(out);
        std::string first;
        std::string second;

        {
            std::string framed(out);
            size_t off = 0;
            std::vector<std::string> frames;
            while (off + 4 <= framed.size()) {
                const uint32_t size =
                    static_cast<uint32_t>(static_cast<unsigned char>(framed[off])) |
                    (static_cast<uint32_t>(static_cast<unsigned char>(framed[off + 1])) << 8) |
                    (static_cast<uint32_t>(static_cast<unsigned char>(framed[off + 2])) << 16) |
                    (static_cast<uint32_t>(static_cast<unsigned char>(framed[off + 3])) << 24);
                if (off + 4 + size > framed.size()) {
                    break;
                }
                frames.push_back(framed.substr(off + 4, size));
                off += 4 + size;
            }

            CHECK(frames.size() == 2);
            if (frames.size() == 2) {
                first = frames[0];
                second = frames[1];
            }
        }

        CHECK(first.find("\"ok\":false") != std::string::npos);
        CHECK(first.find("unknown op") != std::string::npos);
        CHECK(first.find("\"id\":10") != std::string::npos);

        CHECK(second.find("\"ok\":true") != std::string::npos);
        CHECK(second.find("\"id\":11") != std::string::npos);
    }

    // --- malformed requests: error response, host survives ---------------------
    {
        const char* garbage[] = {
            "not json at all",
            "{",
            "{\"id\":\"NaN\"}",
            "{\"op\":true}",
            "[]",
            "{\"id\":1,\"op\":\"ping\",\"payload\":{broken}}",
            "\x01\x02\x03",
        };

        for (const char* g : garbage) {
            std::string input;
            input += frame(g);
            input += frame("{\"id\":99,\"op\":\"ping\"}");

            const auto [out, rc] = run(input);

            CHECK(rc == 0); // never crashes, never exits

            // Final frame must be the healthy ping response.
            const std::string resp = last_frame(out);
            CHECK(resp.find("\"ok\":true") != std::string::npos);
            CHECK(resp.find("\"id\":99") != std::string::npos);
        }
    }

    // --- model ops: load / info / unload round trip ----------------------------
    {
#ifdef _WIN32
        char base[MAX_PATH];
        GetTempPathA(MAX_PATH, base);
        std::string dir = std::string(base) + "shtn-host-test-" +
                          std::to_string(GetCurrentProcessId());
#else
        const char* env = std::getenv("TMPDIR");
        std::string base = env != nullptr ? env : "/tmp";
        std::string dir = base + "/shtn-host-test-" +
                          std::to_string(static_cast<unsigned long>(::getpid()));
#endif
        S_MKDIR(dir.c_str());

        const auto tiny = gguf_test::make_tiny_model(dir + "/tiny.gguf");
        {
            std::ofstream f(tiny.path, std::ios::binary | std::ios::trunc);
            f.write(reinterpret_cast<const char*>(tiny.image.data()),
                    static_cast<std::streamsize>(tiny.image.size()));
            CHECK(static_cast<bool>(f));
        }

        // Fresh host: model_info reports unloaded, no model object.
        {
            const auto [out, rc] = run(frame("{\"id\":20,\"op\":\"model_info\"}"));
            CHECK(rc == 0);
            const std::string resp = last_frame(out);
            CHECK(resp.find("\"loaded\":false") != std::string::npos);
            CHECK(resp.find("\"state\":\"unloaded\"") != std::string::npos);
            CHECK(resp.find("\"model\"") == std::string::npos);
        }

        // load_model without payload → bounded error, host survives.
        {
            std::string input;
            input += frame("{\"id\":21,\"op\":\"load_model\"}");
            input += frame("{\"id\":22,\"op\":\"ping\"}");

            const auto [out, rc] = run(input);
            CHECK(rc == 0);

            std::istringstream stream(out);
            // Two frames out: error + ping-ok.
            const std::string framed = out;
            size_t off = 0;
            std::vector<std::string> frames;
            while (off + 4 <= framed.size()) {
                const uint32_t size =
                    static_cast<uint32_t>(static_cast<unsigned char>(framed[off])) |
                    (static_cast<uint32_t>(static_cast<unsigned char>(framed[off + 1])) << 8) |
                    (static_cast<uint32_t>(static_cast<unsigned char>(framed[off + 2])) << 16) |
                    (static_cast<uint32_t>(static_cast<unsigned char>(framed[off + 3])) << 24);
                if (off + 4 + size > framed.size()) {
                    break;
                }
                frames.push_back(framed.substr(off + 4, size));
                off += 4 + size;
            }
            CHECK(frames.size() == 2);
            if (frames.size() == 2) {
                CHECK(frames[0].find("\"ok\":false") != std::string::npos);
                CHECK(frames[0].find("payload") != std::string::npos);
                CHECK(frames[1].find("\"ok\":true") != std::string::npos);
            }
        }

        // load_model with malformed payload → bounded error.
        {
            std::string input;
            input += frame("{\"id\":23,\"op\":\"load_model\",\"payload\":{\"bad\":1}}");
            input += frame("{\"id\":24,\"op\":\"load_model\",\"payload\":{}}");
            input += frame("{\"id\":25,\"op\":\"ping\"}");

            const auto [out, rc] = run(input);
            CHECK(rc == 0);

            const std::string resp = last_frame(out);
            CHECK(resp.find("\"id\":25") != std::string::npos); // still serving
        }

        // load_model → model_info → unload_model → model_info.
        {
            std::string input;
            input += frame("{\"id\":30,\"op\":\"load_model\",\"payload\":{\"path\":" +
                           gguf_test_quote(tiny.path) + "}}");
            input += frame("{\"id\":31,\"op\":\"model_info\"}");
            input += frame("{\"id\":32,\"op\":\"unload_model\"}");
            input += frame("{\"id\":33,\"op\":\"model_info\"}");

            const auto [out, rc] = run(input);
            CHECK(rc == 0);

            // Decode all four frames.
            std::vector<std::string> frames;
            size_t off = 0;
            while (off + 4 <= out.size()) {
                const uint32_t size =
                    static_cast<uint32_t>(static_cast<unsigned char>(out[off])) |
                    (static_cast<uint32_t>(static_cast<unsigned char>(out[off + 1])) << 8) |
                    (static_cast<uint32_t>(static_cast<unsigned char>(out[off + 2])) << 16) |
                    (static_cast<uint32_t>(static_cast<unsigned char>(out[off + 3])) << 24);
                if (off + 4 + size > out.size()) {
                    break;
                }
                frames.push_back(out.substr(off + 4, size));
                off += 4 + size;
            }

            CHECK(frames.size() == 4);
            if (frames.size() == 4) {
                CHECK(frames[0].find("\"id\":30") != std::string::npos);
                CHECK(frames[0].find("\"ok\":true") != std::string::npos);
                CHECK(frames[0].find("\"loaded\":true") != std::string::npos);
                CHECK(frames[0].find("\"state\":\"loaded\"") != std::string::npos);
                CHECK(frames[0].find("\"architecture\":\"llama\"") != std::string::npos);
                CHECK(frames[0].find("\"contextLength\":256") != std::string::npos);
                CHECK(frames[0].find("\"vocabularySize\":96") != std::string::npos);
                CHECK(frames[0].find("\"tensorCount\":3") != std::string::npos);
                CHECK(frames[0].find("\"kvCacheBytes\":") != std::string::npos);
                CHECK(frames[0].find("\"weightsBytes\":") != std::string::npos);

                CHECK(frames[1].find("\"id\":31") != std::string::npos);
                CHECK(frames[1].find("\"loaded\":true") != std::string::npos);
                CHECK(frames[1].find("\"ggufVersion\":3") != std::string::npos);

                CHECK(frames[2].find("\"id\":32") != std::string::npos);
                CHECK(frames[2].find("\"loaded\":false") != std::string::npos);
                CHECK(frames[2].find("\"state\":\"unloaded\"") != std::string::npos);

                CHECK(frames[3].find("\"id\":33") != std::string::npos);
                CHECK(frames[3].find("\"state\":\"unloaded\"") != std::string::npos);
            }
        }

        // load_model with a garbage model → error response naming the failure.
        {
            std::vector<uint8_t> garbage(256, 0x42);
            const std::string bad = dir + "/garbage.gguf";
            {
                std::ofstream f(bad, std::ios::binary | std::ios::trunc);
                f.write(reinterpret_cast<const char*>(garbage.data()),
                        static_cast<std::streamsize>(garbage.size()));
            }

            std::string input;
            input += frame("{\"id\":40,\"op\":\"load_model\",\"payload\":{\"path\":" +
                           gguf_test_quote(bad) + "}}");
            input += frame("{\"id\":41,\"op\":\"ping\"}");

            const auto [out, rc] = run(input);
            CHECK(rc == 0);

            const std::string resp = last_frame(out);
            CHECK(resp.find("\"id\":41") != std::string::npos); // host alive
        }
    }

    // --- oversized frame: protocol violation exit (supervisor restarts) -------
    {
        const uint32_t big = (1u << 20) + 1;

        std::string input;
        input.push_back(static_cast<char>(big & 0xFF));
        input.push_back(static_cast<char>((big >> 8) & 0xFF));
        input.push_back(static_cast<char>((big >> 16) & 0xFF));
        input.push_back(static_cast<char>((big >> 24) & 0xFF));

        const auto [out, rc] = run(input);

        CHECK(rc == 2); // protocol violation
    }

    // --- shutdown: acknowledge and clean exit ---------------------------------
    {
        std::string input;
        input += frame("{\"id\":50,\"op\":\"health\"}");
        input += frame("{\"id\":51,\"op\":\"shutdown\"}");
        input += frame("{\"id\":52,\"op\":\"ping\"}"); // must NOT be served

        const auto [out, rc] = run(input);

        CHECK(rc == 0);

        const std::string resp = last_frame(out);
        CHECK(resp.find("\"id\":51") != std::string::npos);
        CHECK(resp.find("\"ok\":true") != std::string::npos);
    }

    // --- EOF: clean exit -------------------------------------------------------
    {
        const auto [out, rc] = run(frame("{\"id\":60,\"op\":\"ping\"}"));

        CHECK(rc == 0);
    }

    if (failures > 0) {
        std::fprintf(stderr, "test_host: %d failure(s)\n", failures);
        return 1;
    }

    std::printf("test_host: all checks passed\n");
    return 0;
}
