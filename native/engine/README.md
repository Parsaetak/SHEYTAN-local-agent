# SHEYTAN Native AI Engine — C++ Engine Core (v1.1.5Z Phase 2)

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

**IMPLEMENTED + TESTED in Phase 2 (native GGUF model loading):**

- a bounds-checked, overflow-safe **GGUF reader** (`src/gguf.*`): magic +
  version validation (v2/v3), metadata parsing with hostile-input bounds
  (counts, string lengths, array element counts), tensor-table parsing
  (dims bounds, overflow-checked element products, per-tensor offsets
  validated against the data section, exact byte-size checks for known
  GGML types), alignment handling — model metadata is NEVER trusted
- **memory-mapped model access** (`MappedFile`): whole-file read-only
  mapping on POSIX (`mmap`) and Windows (`CreateFileMapping`); loading a
  multi-GB model does NOT copy it into RAM — only the header pages are
  touched
- **`shtn_engine_load_model` / `shtn_engine_unload_model` /
  `shtn_engine_model_info` / `shtn_engine_memory_plan`** (ABI v2):
  all-or-nothing loading with replace semantics, idempotent unload,
  clean release of the mapping/handle on every path (verified under
  AddressSanitizer)
- real **metadata extraction**: architecture, name, parameter count
  (explicit `general.parameter_count` or derived from the tensor
  table), context length, vocabulary size, embedding length, layer
  count, quantization (from `general.file_type`), tensor count, file
  size — only values actually read or derived; absent keys stay zero
- a load-time **memory plan** (computed, never allocated): model file /
  mapped bytes / weights (tensor-data span) / workspace estimate
  (context·vocab·4 logits row) / KV-cache estimate
  (2·K/V·layers·context·embedding·2 bytes f16) / fixed 64 MiB runtime
  allowance / overflow-checked total / fit-vs-detected-RAM verdict
- the model lifecycle states `unloaded` / `loading` / `loaded` /
  `failed`, mirrored on the Go side (protocol v2 ops: `load_model`,
  `unload_model`, `model_info`)

**NOT implemented (future phases):** inference. No generation, no KV
cache allocation, no compute buffers, no scheduler activity — the
concern types exist on the Go side (`internal/native/engine`), the wire
op set reserves `cancel` for generation requests, and llama.cpp remains
the only generation engine. The host's `cancel` op honestly answers "no
active generation requests".

## Build

CMake (preferred):

```bash
cmake -S native/engine -B native/engine/build
cmake --build native/engine/build
ctest --test-dir native/engine/build          # engine + protocol + host + gguf + model suites
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

## Protocol (v2)

Frame: `[4-byte LE length][JSON payload]` (cap 1 MiB), over the host's
stdin/stdout. Ops: `ping`, `health`, `hwinfo`, `metrics`, `cancel`,
`load_model` (payload `{"path": "...", "contextLength": 0}`),
`unload_model`, `model_info`, `shutdown`. Bump `SHTN_PROTOCOL_VERSION` /
`SHTN_ABI_VERSION` (`include/shtn/version.h`) whenever the wire or ABI
changes — the Go core fails closed on mismatch (v2 was bumped together
on both sides in Phase 2; pre-existing op shapes are unchanged).
