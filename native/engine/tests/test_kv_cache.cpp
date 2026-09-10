// test_kv_cache.cpp — Phase 4 KV cache tests (dependency-free asserts).
//
// Verifies the KV cache allocates correctly from real model dims,
// reports measured bytes, resets, releases, and rejects hostile layouts.

#include "shtn/engine.h"
#include "shtn/types.h"

#include "kv_cache.h"

#include <cstdio>
#include <cstring>
#include <string>

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
    using namespace shtn::kv;

    // --- basic allocate / stats / release -------------------------------
    {
        Cache c;
        Layout l;
        l.layer_count = 4;
        l.embedding_length = 64;
        l.head_count = 8;
        l.head_dim = 8;
        l.kv_head_count = 8; // MHA (no GQA)
        l.kv_dim = 8 * 8;    // 64
        l.context_length = 128;

        std::string err;
        CHECK(c.allocate(l, 0, err) == SHTN_OK);

        Stats s = c.stats();
        CHECK(s.allocated == true);
        CHECK(s.layer_count == 4);
        CHECK(s.kv_dim == 64);
        CHECK(s.capacity_positions == 128);
        CHECK(s.used_positions == 0); // honest: no forward pass
        CHECK(s.used_bytes == 0);     // honest: nothing written
        CHECK(s.capacity_bytes > 0);
        // 2 (K+V) * 4 layers * 128 ctx * 64 kv_dim * 2 bytes (f16) = 524288
        CHECK(s.capacity_bytes == 2ull * 4 * 128 * 64 * 2);
        CHECK(s.quantization == "f16");
    }

    // --- advance updates used_positions honestly ------------------------
    {
        Cache c;
        Layout l;
        l.layer_count = 2;
        l.embedding_length = 32;
        l.head_count = 4;
        l.head_dim = 8;
        l.kv_head_count = 4;
        l.kv_dim = 32;
        l.context_length = 64;

        std::string err;
        CHECK(c.allocate(l, 0, err) == SHTN_OK);

        c.advance(10);
        Stats s = c.stats();
        CHECK(s.used_positions == 10);
        CHECK(s.used_bytes > 0); // proportional to used_positions

        c.advance(100); // overshoots capacity — clamps
        s = c.stats();
        CHECK(s.used_positions == 64);

        c.reset();
        s = c.stats();
        CHECK(s.used_positions == 0);
        CHECK(s.used_bytes == 0);
    }

    // --- GQA: kv_head_count < head_count -------------------------------
    {
        Cache c;
        Layout l;
        l.layer_count = 4;
        l.embedding_length = 64;
        l.head_count = 8;
        l.head_dim = 8;
        l.kv_head_count = 2; // GQA 4:1
        l.kv_dim = 2 * 8;    // 16 (NOT 64)
        l.context_length = 128;

        std::string err;
        CHECK(c.allocate(l, 0, err) == SHTN_OK);

        Stats s = c.stats();
        CHECK(s.kv_dim == 16);
        // 2 (K+V) * 4 layers * 128 ctx * 16 kv_dim * 2 bytes = 32768
        CHECK(s.capacity_bytes == 2ull * 4 * 128 * 16 * 2);
    }

    // --- invalid layout rejected ----------------------------------------
    {
        Cache c;
        Layout l; // all zeros
        std::string err;
        CHECK(c.allocate(l, 0, err) == SHTN_ERR_INVALID_ARG);
    }

    // --- context length cap enforced ------------------------------------
    {
        Cache c;
        Layout l;
        l.layer_count = 1;
        l.embedding_length = 8;
        l.head_count = 1;
        l.head_dim = 8;
        l.kv_head_count = 1;
        l.kv_dim = 8;
        l.context_length = (1ull << 21); // > kMaxContextLength

        std::string err;
        CHECK(c.allocate(l, 0, err) == SHTN_ERR_UNSUPPORTED);
    }

    // --- available RAM check honored -----------------------------------
    {
        Cache c;
        Layout l;
        l.layer_count = 4;
        l.embedding_length = 64;
        l.head_count = 8;
        l.head_dim = 8;
        l.kv_head_count = 8;
        l.kv_dim = 64;
        l.context_length = 128;

        std::string err;
        // Need 524288 bytes; give 1024 → reject.
        CHECK(c.allocate(l, 1024, err) == SHTN_ERR_UNSUPPORTED);
        // Give enough → succeed.
        CHECK(c.allocate(l, 1ull << 20, err) == SHTN_OK);
    }

    // --- release clears state ------------------------------------------
    {
        Cache c;
        Layout l;
        l.layer_count = 1;
        l.embedding_length = 8;
        l.head_count = 1;
        l.head_dim = 8;
        l.kv_head_count = 1;
        l.kv_dim = 8;
        l.context_length = 16;

        std::string err;
        CHECK(c.allocate(l, 0, err) == SHTN_OK);
        CHECK(c.allocated() == true);
        c.release();
        CHECK(c.allocated() == false);
        Stats s = c.stats();
        CHECK(s.allocated == false);
        CHECK(s.capacity_bytes == 0);
    }

    // --- move semantics -------------------------------------------------
    {
        Cache a;
        Layout l;
        l.layer_count = 1;
        l.embedding_length = 8;
        l.head_count = 1;
        l.head_dim = 8;
        l.kv_head_count = 1;
        l.kv_dim = 8;
        l.context_length = 16;

        std::string err;
        CHECK(a.allocate(l, 0, err) == SHTN_OK);

        Cache b = std::move(a);
        CHECK(b.allocated() == true);
        CHECK(a.allocated() == false); // NOLINT(bugprone-use-after-move)
        Stats s = b.stats();
        CHECK(s.capacity_positions == 16);
    }

    // --- k_layer / v_layer return non-null after allocate --------------
    {
        Cache c;
        Layout l;
        l.layer_count = 2;
        l.embedding_length = 16;
        l.head_count = 2;
        l.head_dim = 8;
        l.kv_head_count = 2;
        l.kv_dim = 16;
        l.context_length = 32;

        std::string err;
        CHECK(c.allocate(l, 0, err) == SHTN_OK);

        CHECK(c.k_layer(0) != nullptr);
        CHECK(c.k_layer(1) != nullptr);
        CHECK(c.v_layer(0) != nullptr);
        CHECK(c.v_layer(1) != nullptr);
        // K and V for the same layer must be different pointers.
        CHECK(c.k_layer(0) != c.v_layer(0));
        // Out-of-range layer returns nullptr.
        CHECK(c.k_layer(99) == nullptr);
    }

    if (failures > 0) {
        std::fprintf(stderr, "test_kv_cache: %d failure(s)\n", failures);
        return 1;
    }

    std::printf("test_kv_cache: all checks passed\n");
    return 0;
}
