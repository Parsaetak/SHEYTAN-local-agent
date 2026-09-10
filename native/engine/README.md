# SHEYTAN Native AI Engine — C++ Engine Core (v1.1.5Z Phase 1)

This is the C++ side of the **SHEYTAN Native AI Engine architecture**:

```text
React/TypeScript
      ↓
    Wails
      ↓
   Go Core  (internal/ — application + runtime engine)
      ↓
SHEYTAN Native API  (internal/native/engine — supervised host + IPC)
      ↓
C++ Native Engine   (THIS tree — future heavy-compute/AI execution engine)
```

## Status (read literally — ARCHITECTURE.md Part III truth standard)

**IMPLEMENTED + TESTED in Phase 1:**

- a narrow, stable **C ABI** (`include/shtn/engine.h`):
  `shtn_engine_create` / `shtn_engine_destroy` / `shtn_engine_health` /
  `shtn_engine_hardware_info` / `shtn_engine_metrics` / `shtn_abi_version`
- real platform hardware detection (`src/hardware.cpp`): CPU
  (name/cores/frequency), RAM (total/available), compile-time
  architecture — detected values only, never guessed, no hardcoded vendor
  assumptions
- real process metrics: engine uptime and its own RSS
- `shtn-engine-host`: the supervised subprocess the Go core spawns — a
  length-prefixed JSON IPC protocol over stdin/stdout (4-byte
  little-endian frame length, 1 MiB cap, protocol+ABI handshake, bounded
  error responses for malformed input — the host never crashes on bad
  input)
- a dependency-free test suite (CMake/CTest or plain make)

**NOT implemented (future phases):** inference. No model loading, no
generation, no KV cache, no scheduler activity — the concern types exist
on the Go side (`internal/native/engine`), the wire op set reserves
`cancel` for generation requests, and llama.cpp remains the only
generation engine. The host's `cancel` op honestly answers "no active
generation requests".

## Build

CMake (preferred):

```bash
cmake -S native/engine -B native/engine/build
cmake --build native/engine/build
ctest --test-dir native/engine/build          # engine + protocol + host suites
```

Plain make (no CMake required):

```bash
make -C native/engine
make -C native/engine test
```

Artifacts:

- `build/shtn-engine-host` — the supervised host executable the Go core
  spawns
- `build/libshtn_engine.a` — the engine core behind the C ABI

## Runtime use

Copy `shtn-engine-host` to `{DataDir}/bin/` (or configure
`nativeEnginePath`), set `engineBackend: "native"` in `config.json`, and
the Go core will supervise it. Without the binary the native path reports
unavailable and llama.cpp remains the engine (the default anyway).

## Protocol (v1)

Frame: `[4-byte LE length][JSON payload]` (cap 1 MiB), over the host's
stdin/stdout. Ops: `ping`, `health`, `hwinfo`, `metrics`, `cancel`,
`shutdown`. Bump `SHTN_PROTOCOL_VERSION` / `SHTN_ABI_VERSION`
(`include/shtn/version.h`) whenever the wire or ABI changes — the Go core
fails closed on mismatch.
