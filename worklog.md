# SHEYTAN-Local-Agent — Worklog

## Current State

Date: 2026-09-10

Repository:

```text
https://github.com/Parsaetak/SHEYTAN-local-agent
```

Branch: `main`

Current release:

```text
v1.1.5Z
```

v1.1.5Z is the **SHEYTAN Native AI Engine** release line. Phase 1
established the backend abstraction, the supervised native engine path
and the C++ engine skeleton. Phase 2 (this log, first below) added
**native GGUF model loading**: a hardened C++ GGUF reader, memory-mapped
model access, real metadata extraction, load-time memory planning, the
model lifecycle and the `ModelInfo` surface through Go. Native
GENERATION is still NOT implemented — llama.cpp remains fully functional
as the fallback (and the default generation engine). Full phase logs
below.

---

# v1.1.5Z Phase 2 Implementation Log (2026-09-10)

## Goal

Make the SHEYTAN Native Engine load a real supported GGUF model in C++:
read its metadata, validate it, plan its memory requirements and expose
real model information to Go — WITHOUT redesigning the Phase 1 Go↔C++
boundary. Generation stays on llama.cpp in this phase (explicitly).

```text
Go Core → SHEYTAN Native API → shtn-engine-host → C++ Native Engine → GGUF model
```

## Phase 1 verification (before any change)

The Phase 1 implementation was verified FIRST on the base commit
(`f4488d5`, v1.1.5Z-phase1): C++ build + 3/3 ctest, `go build`/`go vet`,
22 Go packages pass, race tests pass, `TestRealCppHostEndToEnd` passes,
npm typecheck/lint/build pass, stress 30/30, release-version --check
PASS. Only then did Phase 2 work begin.

## 1. Versioned protocol/ABI extension (no boundary redesign)

Protocol v1 → **v2** and ABI v1 → **v2**, bumped together on both sides
(`include/shtn/version.h` + `internal/native/engine/protocol.go` /
`backend.go ABIVersionExpected`). The handshake still fails closed on
mismatch. Pre-existing ops (ping/health/hwinfo/metrics/cancel/shutdown)
kept their exact wire shapes — the change is purely additive:

- wire ops `load_model` (payload `{path, contextLength?}`),
  `unload_model`, `model_info`;
- C ABI functions `shtn_engine_load_model` / `shtn_engine_unload_model` /
  `shtn_engine_model_info` / `shtn_engine_memory_plan` with new fixed-size
  structs (`shtn_model_load_options`, `shtn_model_info`,
  `shtn_memory_plan`) and new error codes appended (never renumbered):
  `SHTN_ERR_MODEL_FORMAT` / `SHTN_ERR_MODEL_STATE` / `SHTN_ERR_NO_MODEL`.

## 2. Native GGUF reader (native/engine/src/gguf.*)

Dependency-free, mmap-backed, hostile-input-hardened:

- magic + version validation (v2/v3 — the llama.cpp-supported set; v1
  and >3 rejected with a readable error);
- metadata parsing with bounds on every count/length (KV count ≤ 16384,
  string ≤ 16 MiB, array elements ≤ 100M, tensor count ≤ 1M, dims ≤ 8);
  unknown value types rejected; `general.alignment` validated (power of
  two, ≤ 4096) and honored for the data-section start;
- array VALUES are never materialized (the tokenizer vocab array is
  walked length-prefix-only; its element count feeds the vocab-size
  fallback);
- tensor table: per-tensor dims bounds, overflow-checked element
  products (a dims product that overflows uint64 is a hard reject),
  offsets validated against the data section, exact byte-size checks for
  known GGML types (ggml_nbytes formula: last dim padded to block size),
  lenient in-range checks only for types this build does not know;
- every arithmetic that could overflow goes through checked_add/mul —
  wrap-around is impossible by construction;
- the reader touches ONLY header pages (no full-file reads, no copying).

## 3. Model lifecycle + memory planning (native/engine/src/model.*)

- state machine: `unloaded → loading → loaded | failed → unloaded`, one
  mutex-protected model slot per engine; loads serialize, inspections
  run concurrently against stable snapshots;
- load = validate path → whole-file read-only mmap (POSIX mmap / Windows
  CreateFileMapping; lazy, no eager copy) → parse + validate → metadata
  extraction → memory plan → `loaded`. Replace semantics (load while
  loaded unloads first), all-or-nothing (a failed load leaves NOTHING
  loaded, mapping and handle released on every path);
- caller-argument errors (empty path, reserved field) leave the state
  unchanged (nothing was attempted); file-level failures (missing file,
  parse errors, zero-tensor files) walk to `failed` with the reason
  recorded;
- unload is idempotent; engine destroy releases the mapping;
- memory plan (computed, NOTHING allocated): model file / mapped bytes /
  weights (tensor-data span = file − data start) / workspace estimate
  (context·vocab·4 logits row) / KV-cache estimate
  (2·K/V·layers·context·embedding·2 bytes f16) / fixed 64 MiB runtime
  allowance / overflow-checked total / fit-vs-detected-RAM verdict
  (1/0/-1). Inputs missing from the file → that component is an honest 0.

## 4. Go side (internal/native/engine)

- `model.go` is now REAL: `Engine.LoadModel` / `Engine.UnloadModel` /
  `Engine.ModelInfo` drive the IPC ops with a dedicated model-state
  machine (`unloaded/loading/loaded/failed`) — deliberately separate
  from the engine states (`llm.State*`), no second engine-state system;
- the model snapshot RESETS on every host lifecycle boundary (start,
  restart, deliberate stop, death): a fresh host maps nothing, so stale
  state cannot survive;
- load ops use a bounded 30 s window (metadata-only parse, but cold
  slow disks deserve headroom) plus the caller ctx;
- `backend.go` maps the native card onto the shared `llm.ModelInfo`
  contract — additive fields (`State`, `FileSizeBytes`, `TensorCount`,
  `GGUFVersion`, `VocabSize`, `EmbeddingLength`, `LayerCount`,
  KV/workspace/total memory estimates) that the llama path leaves zero;
  `llm.FormatParameterCount` shared with the llama card formatter;
- metrics op additionally reports `modelState` (observability only);
- `GenerationCapable()` stays FALSE: loading is not generating.
  Generate/StreamGenerate still return `llm.ErrNotImplemented` and
  `llm.SelectGenerationBackend` still resolves to llama.cpp — pinned by
  tests.

## 5. Tests added

C++ (`ctest`, 5 suites now):

- `test_gguf` — valid v2/v3 files, custom alignment, malformed magic,
  unsupported versions (1 and 4), truncated files (empty / magic-only /
  counts cut / metadata cut / tensor-table cut / missing data section),
  hostile counts/lengths/types/alignment, invalid tensor entries
  (out-of-range offset, exact-size overflow, zero dim, dim-count bounds,
  dims-product overflow), checked arithmetic, metadata helpers;
- `test_model` — fresh-engine unloaded state, NULL/invalid-argument
  rejection, load→info→plan correctness (exact KV/workspace/total
  values), context-override planning, load A → load A again → load B
  replace semantics, unload/reload/idempotent unload, failed-load
  recovery (garbage file, zero-tensor file), destroy-with-loaded-model,
  concurrent safe inspection (4 inspectors + load/unload churn thread);
- `test_engine` / `test_host` extended: ABI v2 pins, model-surface NULL
  checks, fresh-engine `model_info` op, load/unload round trips through
  the dispatch loop, malformed-payload bounded errors, host-survives
  checks.

The whole C++ suite also passes under AddressSanitizer (no leaks, no
out-of-bounds access in the reader or the lifecycle).

Go:

- `model_test.go` — fake-host lifecycle matrix (load A / load A again /
  load B / unload / reload / failed load), validation ordering, bounded
  caller-ctx load, state resets across stop/start, concurrent inspection
  (race-detector target);
- `cpp_integration_test.go` — `TestRealCppHostModelLifecycle`: a real
  GGUF v3 file is built byte-by-byte in the test, loaded through the
  REAL C++ reader via the real host, metadata + derived parameter count
  + memory plan verified exactly, unload/reload/failed-load/recovery
  exercised, and the `llm.ModelInfo` mapping checked end-to-end;
- `engine_test.go` — fake host grew model ops (including loadfail /
  loadslow modes) mirroring the v2 wire shapes.

## 6. Validation performed (this release)

```text
cmake -S native/engine -B native/engine/build + build   PASS
cmake --build + ctest                                   5/5 PASS
                                                        (engine, protocol, host,
                                                         gguf, model)
plain make + make test                                  PASS
C++ suite under AddressSanitizer                        5/5 PASS (no leaks)
go build -tags headless ./...                          PASS
go vet  -tags headless ./...                           PASS
go test -tags headless ./internal/... -count=1         22 packages PASS
go test -race -tags headless (agent, llm, api,
                              native/engine)            PASS
TestRealCppHostEndToEnd + TestRealCppHostModelLifecycle PASS (real C++ host)
frontend: typecheck / lint (0 warnings) / build         PASS (assets unchanged:
                                                         no frontend edits)
stress suite (release gate)                             30 pass / 0 fail
node scripts/release-version.mjs --check                PASS (all surfaces 1.1.5)
version smoke: release stays v1.1.5Z                    PASS (no bump)
```

## Known limitations (Phase 2, by design)

- **No native inference.** Loading a model does NOT enable generation:
  Generate/StreamGenerate return `ErrNotImplemented`,
  GenerationCapable() is false, and every generation request runs on
  llama.cpp. Do not "fix" that by faking inference.
- The memory plan is arithmetic on parsed metadata (weights span, f16 KV
  estimate, logits-row workspace, fixed overhead allowance) — real
  allocation behavior arrives with the inference phase.
- GGUF v1 containers and GGML tensor types unknown to this build's size
  table are rejected or validated leniently (offset-in-range only),
  never guessed.
- The context override plans memory only; no context buffers exist yet.
- The native host binary is still not shipped or auto-downloaded; build
  it from `native/engine/` and place it in `{DataDir}/bin/` (or
  `nativeEnginePath`).
- SIGBUS risk if a mapped file shrinks mid-flight is inherent to mmap
  consumers (llama.cpp has the same property); files are opened
  read-only and nothing in SHEYTAN writes to model files.

---

# v1.1.5Z Phase 1 Implementation Log (2026-09-10)

## Goal

Establish the SHEYTAN Native AI Engine architecture:

```text
React/TypeScript → Wails → Go Core → SHEYTAN Native API → C++ Native Engine
```

Go remains the main application/runtime engine; the C++ engine becomes
the future heavy-compute/AI execution engine. llama.cpp stays functional
as the fallback throughout.

## 1. LLM backend abstraction (internal/llm)

`llm.Backend` formalizes the engine surface the rest of SHEYTAN depends
on: `Start / Stop / Health / LoadModel / UnloadModel / Generate /
StreamGenerate / Cancel / ModelInfo / HardwareInfo / Metrics`, plus
shared contract types (`HealthReport`, `ModelSpec`, `ModelInfo`,
`HardwareInfo` — the platform-neutral hardware profile —, `Metrics`) and
the error vocabulary (`ErrNotImplemented`, `ErrCancelContextBased`).

`llm.LlamaBackend` adapts the EXISTING pieces (LlamaServer + Client) to
the contract — a delegation layer, not a second engine: streaming,
retries, the stall watchdog, cancellation, busy reporting, engine events
and the config snapshot contract are untouched. Additive surface on
LlamaServer: `StartedAt()`, `Restarts()` (measured values for metrics)
and `ProbeHealth()` (a real bounded `/health` GET — the same endpoint the
startup ladder polls).

`llm.SelectGenerationBackend` is the single routing point: the native
engine serves generation only when selected AND
`GenerationCapable()`; otherwise llama.cpp. In Phase 1 the native backend
reports generation-incapable, so generation always resolves to llama.cpp
— pinned by tests so Phase 2 flips routing by implementing generation,
not by editing call sites.

## 2. Native engine package (internal/native/engine)

Concern layout (files): `protocol.go` (framing + ops + validation),
`runtime.go` (supervision), `backend.go` (`llm.Backend` adapter),
`platform.go` (hardware profile assembly), `metrics.go`,
`model.go`/`memory.go`/`kv.go`/`generation.go`/`scheduler.go` (Phase 1
concern types — the future data model, no fake inference).

- **Protocol**: 4-byte LE length-prefixed JSON over the host's
  stdin/stdout; 1 MiB frame cap; ops `ping/health/hwinfo/metrics/cancel/
  shutdown`; malformed frames rejected as protocol violations; request/
  response multiplexer with per-op timeouts (10 s) and serialized writers.
- **Supervision**: spawn (sanitized environment — no secrets cross the
  boundary) → protocol+ABI handshake (fails closed on mismatch) → health
  → ready; event-driven exit watcher (no polling, no busy loops); bounded
  auto-restart (3 recoveries per supervision episode, 1/2/4 s backoff,
  deliberate stops suppressed, terminal failure visible). Native engine
  state uses the SAME vocabulary and event shape as llama.cpp
  (`llm.State*`, `llm.EngineEvent`) — no second state system.
- **Hardware profile**: native probe (C++ detected values) merged with
  the sysinfo probe (GPU/VRAM); `DetectedBy` records each source; NPU/
  accelerator and shared-memory GPU fields are representable but stay
  empty until a detector exists.
- **Metrics**: only measured values — engine state, pid, uptime, restarts
  (Go side) + the C++ engine's own process RSS and uptime; generation
  metrics (TTFT, prompt/decode speed) are OMITTED until a backend
  actually serves generation.

## 3. C++ native engine skeleton (native/engine/)

Buildable independently (CMake ≥ 3.16 or plain make; C++17; ZERO
third-party dependencies):

- `include/shtn/{engine,types,version}.h` — the narrow C ABI: engine
  create/destroy, health, hardware info, metrics, ABI version. Opaque
  handle, fixed-size C structs, error codes instead of exceptions.
- `src/engine.cpp` — the engine core (state, uptime, hardware cache,
  measured metrics).
- `src/hardware.cpp` — real platform detection (Linux /proc + sysconf,
  Windows cpuid + GLPI + GlobalMemoryStatusEx + psapi, macOS sysctl +
  mach) with per-platform #ifdef paths.
- `src/{protocol,json}.cpp` — framing + a careful minimal JSON
  parser/serializer (escape-safe, nesting-aware, hostile-input-safe).
- `src/host_main.cpp` — `shtn-engine-host`: the supervised subprocess;
  one request frame in → one response frame out; malformed input gets a
  bounded error response (never a crash, never an exit); `shutdown`
  acknowledges and exits cleanly; stdin EOF exits cleanly.
- `tests/` — `test_engine` (ABI contract: create/destroy, NULL
  rejection, ABI mismatch, health, hwinfo, metrics monotonicity),
  `test_protocol` (framing, caps, truncation, JSON edge cases),
  `test_host` (dispatch: valid ops, unknown ops, garbage inputs, oversized
  frames, shutdown, EOF).

## 4. Go↔C++ boundary decision

**B) supervised native subprocess + IPC** (documented in
`internal/native/engine/doc.go` and ARCHITECTURE.md §I.9): crash
isolation (a native crash cannot kill the app; the bounded watchdog
restarts it), preserved `CGO_ENABLED=0` Windows cross-build, future
Android maps to a service process, explicit versioned protocol, and
coarse-grained ops only (the boundary never carries tiny high-frequency
calls). Verified end-to-end by `TestRealCppHostEndToEnd` (Go Engine ↔
real C++ host: handshake, health, hardware, metrics, cancel, clean stop).

## 5. llama.cpp preserved (fallback contract)

- llama.cpp remains the DEFAULT engine (`engineBackend: "llama"`) — the
  entire v1.1.4Z behavior is unchanged unless the user explicitly opts in.
- With `engineBackend: "native"`: the native engine is supervised
  (lifecycle real), generation still runs on llama.cpp (native reports
  generation-incapable), and native failures NEVER fail the llama path
  (best-effort, logged, visible in the engine snapshot).
- The engine toggle (POST /api/llama start/stop) drives both engines when
  native is enabled; llama's state stays the authoritative action result.
- `/api/engine` snapshot gains `backend` (effective generation backend)
  and a `native` status block (local reads only — the poll path performs
  NO engine IPC); native transitions broadcast into the existing WS
  pipeline with "Native engine …" captions; the UI badge keeps reading
  the llama snapshot state.
- Config: `engineBackend` (exact-match opt-in, fail-closed to llama on
  malformed values; normalized on load), `nativeEnginePath` (optional
  override). Consumed by runtime wiring + selection; no Settings UI
  control in Phase 1 (config.json / `SHEYTAN_ENGINE_BACKEND` env are the
  opt-in paths — a toggle whose only visible effect would be a process in
  Task Manager is feature theater).

## 6. Validation performed (this release)

```text
go build -tags headless ./...                          PASS
go vet  -tags headless ./...                           PASS
go test -tags headless ./internal/... -count=1         22 packages PASS
go test -race -tags headless (agent, llm, api,
                              native/engine, config)   PASS
frontend: npm ci / typecheck / lint (0 warnings) / build
         + sync into web/static                       PASS (assets unchanged:
                                                         types-only api.ts edit)
stress suite (release gate)                            30 pass / 0 fail
node scripts/release-version.mjs (sync + --check)      PASS (all surfaces 1.1.5)
C++ (cmake 3.30 + g++ 14, C++17): build                PASS
C++ ctest: engine + protocol + host                    3/3 PASS
Go↔C++ integration (TestRealCppHostEndToEnd)           PASS
version smoke: "SHEYTAN-Local-Agent v1.1.5"            PASS
```

New tests added:

- `internal/llm/backend_test.go` — selection matrix (default llama /
  native-incapable fallback / native-capable (Phase 2 shape) / native
  absent / malformed values fail closed / no-probe backends), llama
  backend delegation, cancel semantics, metrics (measured values only),
  hardware from sysinfo, model validation, contract constants.
- `internal/native/engine/engine_test.go` — protocol framing/validation;
  lifecycle (start→ready, stop walk, events, missing binary → failed);
  failure (crash → bounded restart ladder → terminal failed, garbage
  host → connection loss detection, slow host → bounded teardown);
  cancellation (cancel round-trip miss with reason, op timeouts);
  concurrent lifecycle (race-clean); hardware merge; measured metrics;
  backend contract (ErrNotImplemented for generation, GenerationCapable
  false, LoadModel validates first); model spec path jail.
- `internal/native/engine/cpp_integration_test.go` — end-to-end against
  the real C++ host binary (skips when not built).
- `internal/api/server_native_test.go` — engine snapshot (default: no
  native block; native selected: honest unavailable status, backend stays
  llama), engine toggle behavior unchanged (500 on missing binary exactly
  as v1.1.4Z).

## Known limitations (Phase 1, by design)

- **No native inference.** Generate/StreamGenerate/LoadModel/ModelInfo
  return `ErrNotImplemented` on the native backend; every generation
  request runs on llama.cpp. This is the honest fallback signal — do not
  "fix" it by faking inference.
- The native host binary is not shipped or auto-downloaded; it must be
  built from `native/engine/` (CMake or Make) and placed in
  `{DataDir}/bin/` (or `nativeEnginePath`).
- The C++ hardware probe does not enumerate GPUs/accelerators (the Go
  side merges sysinfo for real GPU facts; the profile fields exist and
  stay empty until a detector does).
- The Wails/GTK desktop build still cannot compile in the Linux CI-less
  environment (pre-existing; Windows CI builds it).
- llama.cpp watchdog semantics (budget resets after a successful
  restart) are intentionally UNTOUCHED; the native engine uses a stricter
  episode policy (auto-restarts never reset the budget — a persistently
  crashing native host gives up after 3 recoveries).

---

# v1.1.4Z Windows CI repair (2026-09-07, post-release)

The `Windows x64` job of the `Build Desktop` workflow failed at the
`Verify release metadata` step with:

```text
A positional parameter cannot be found that accepts argument '1.1.4\'.
```

## Root cause

The step reused **Bash-style `\"` escaping inside PowerShell strings**:

```powershell
-Pattern "AppVersion  = \"$env:APP_VERSION\""   # invalid in PowerShell
```

PowerShell terminates the string at the quote after the backslash, the
`-Pattern` argument splits into extra positional tokens, and parameter
binding fails on the fragment `1.1.4\`. The release metadata itself was
never wrong — `node scripts/release-version.mjs` succeeded and all four
surfaces were in sync. Reproduced locally with pwsh 7.4.6 executing the
verbatim step script: identical error, exit 1.

## Second (latent) defect fixed in the same step

`Select-String ... | Out-Null` **exits 0 when nothing matches**. Even with
correct quoting, a mismatched release surface would have passed the gate
silently (verified by test: wrong version, exit 0). The repaired step uses
`-Quiet` and throws on a false result, so a metadata mismatch now fails
the job loudly.

## Fix

`Windows x64` job, `Verify release metadata` step: rewritten with
PowerShell-native single-quoted literals plus explicit concatenation
(no escaping needed), `-SimpleMatch` retained, all four release surfaces
checked in a data-driven loop, expectations derived from
`$env:APP_VERSION` (version-agnostic, no hard-coded version). The bash
`grep -F` equivalents in the audit and Linux jobs were already correct
and untouched.

## Verification performed (this repair)

- The exact step script (extracted verbatim from the workflow YAML)
  executed with pwsh 7.4.6 against the real repository files:
  10/10 checks — correct metadata passes; wrong `APP_VERSION` fails;
  corrupted `SIGNATURE` fails; version bump to 1.1.5 through the release
  machinery passes and restores cleanly; no `\"` artifacts remain
  anywhere in the workflow.
- `node scripts/release-version.mjs` and `--check`: all four surfaces
  consistent, no drift, byte-preserving sync intact.
- Full suite re-run after the change: `go build`/`go vet` headless,
  `go test -tags headless ./internal/...` (21 packages),
  `-race` on agent/llm/api, full headless tree, `npm ci`/typecheck/lint
  (0 warnings)/build, stress suite 30/0, releasegate — all green.
- `zeta_release_surface` stress contract still pins the workflow shape;
  no contract fragment was touched.

## Repository consistency cleanups shipped with the repair

- `web/static`: the committed tree carried a stale duplicate generation of
  hashed assets and a `manifest.json` pointing at the old generation while
  `index.html` referenced the new one. A clean `npm run build` +
  `scripts/sync-web.mjs` removes the 8 stale files and corrects the
  manifest; the runtime entry (`index.html` + referenced assets) is
  byte-identical.
- Documentation audit (new `ARCHITECTURE.md`, updates to `README.md`,
  `agent.md`, this file, `FIX-README.md`): every capability statement is
  now labeled with the truth standard (IMPLEMENTED / TESTED / PARTIALLY
  IMPLEMENTED / PLANNED), the small-models/multi-agent/Context-Engine
  direction is recorded as future architecture, `internal/multiagent` is
  documented (it was implemented and CLI-wired but absent from the
  package map), and two overstated claims were corrected
  ("semantic chunking" → paragraph-boundary chunking; "checksum-verified
  publication" → integrity-checked publication via ZIP CRC + entry
  contracts — the SHA256 steps left the workflow during earlier
  refactors).

---

# v1.1.4Z Remediation Log (2026-09-07)

## Audit scope and method

Full-repository inspection before any modification: every Go package (52k+ LOC), the React/TS frontend, CI workflow, build/release scripts, docs and tests. Six subsystem maps were produced (llm, agent+api, tools+lab+sandbox, research+memory+updater, config+cmd+CI, frontend) and every suspected dead/defective path was verified by reference tracing before removal. Baseline validation before changes: build, vet and all test packages green on `main` @ `79c710c` (v1.1.3).

## 1. Concurrency and state integrity

| Fix | Defect | Resolution |
|---|---|---|
| **Config data race** | `mergeConfigPatch` wrote `*s.cfg = updated` in place while run goroutines, the engine manager and the client read the same struct — a genuine `-race` class defect | `internal/config.Source`: copy-on-write holder (RWMutex, `Load/Store/Update`). Patch handler, engine compat writes, engine-tag writes and the update loop all publish through it; runs and requests snapshot once and stay consistent for their duration |
| **Orchestrator races** | tool map read/written without a lock; `SetSessionID` clobbered between concurrent runs; per-iteration config reads could flip policy mid-turn | RWMutex-guarded registry with `tool()` lookup; per-run config snapshot |
| **MarkBusy window** | aliveness checked under `mu`, transition applied after unlock — a death in between could be overwritten by a stale `busy`/`ready` | `setStateLocked` — the whole transition is atomic |
| **busyHook write race** | `Client.SetBusyHook` wrote a plain field that streaming reads raced | RWMutex-guarded |
| **MultiAgent struct mutation** | `Run` defaulted `m.maxIter` on the shared struct | local variable |
| **Termshell engine** | shared instance with unlocked `cwd`/`history`/`env` | mutex on `Exec` |
| **Stack.Browser lazy cache** | unlocked field write | `browserMu` |
| **Config persistence** | plain `WriteFile` (a crash mid-write truncated config.json) | atomic tmp+rename |

Regression tests: `TestSourceConcurrentReadWrite`, `TestConfigPatchIsRaceFree` (both run under `-race`).

## 2. Engine / LLM layer

| Fix | Defect | Resolution |
|---|---|---|
| **Engine download hang** | plain `http.Get` (no timeout, no cap) could hold `switchMu` forever | 10-minute context + 2 GiB cap on `downloadAndExtract` |
| **`ListLoadedModels` hang** | plain `http.Get` from an HTTP handler | 5-second client |
| **Streaming truncation** | a single 10-minute `http.Client.Timeout` covered the whole SSE body — long generations were silently cut, while stalled streams hung until it fired | dedicated stream client (no overall timeout, 2-minute header timeout) + **stall watchdog**: request-context cancel after 5 minutes with zero bytes (adaptive tick); `TestStreamStallWatchdogAbortsQuietStream` pins it |
| **Silent retry exhaustion** | the final give-up after 4 attempts was never logged | logged to `llm.jsonl` on both Chat and Stream paths |
| **Vestigial stateCh** | legacy channel written non-blocking, never read | removed (with `publishEvent`, folded into `setStateLocked`) |

## 3. Functional completion (documented-but-unwired subsystems)

Every item below was fully implemented and unit-tested in the repository but had **no production caller** — the exact "feature theater" class this release eliminates:

| Subsystem | Previous state | v1.1.4Z |
|---|---|---|
| **GGUF model cards** (`llm/gguf.go`) | complete parser, zero callers; `/api/models` shipped stat-only entries while the docs promised architecture/quantization/context | wired into `/api/models` with an mtime-keyed cache; parser now tested |
| **Sampling settings** | `minP`, `repeatLastN`, `presencePenalty`, `frequencyPenalty`, `mirostatTau/Eta` editable in Settings, never sent anywhere | sent per-request (OpenAI-standard fields to all providers; llama.cpp-only fields local-gated) and as engine launch flags |
| **Sandbox settings** | `sandboxEnabled/Memory/CPU` stored but ignored (runtime hardcoded 512 MB / 25 %; the toggle lied — the sandbox was ALWAYS on while the config said off) | wired: the toggle gates registration, memory/CPU feed the governor with parsed/clamped effective getters; default ON (fail closed) |
| **ShowPerfHUD** | collected telemetry never reached any UI | emits a `perf` activity event with the HUD line |
| **Continuum rollover** | distillation/chapters/framework sidecars implemented + tested since v1.0.7; only the pressure meter was live | wired after each run: threshold check → deterministic distill → new chapter session → `session` activity event; the frontend follows the thread automatically |
| **Recall feedback** | `SetFeedback`/`FeedbackFor` and the liked×1.25/disliked×0.6 steering existed since v1.0.6 with no write path | `/api/feedback` endpoint + 👍/👎 on assistant messages in the conversation view (optimistic, with revert on failure) |
| **Scheduled engine updates** | `updater.RunScheduled` (immediate pass when due + 6 h re-check) had zero callers — "update (scheduled: daily/…)" only ever meant manual runs | started in `Server.EnsureSetup` (Source-aware; `off`/`never` disables) |
| **BMP images** | `.bmp` accepted by discovery, rejected by the encoder | BMP decode via `x/image/bmp` |
| **Sampling presets** | served, loaded into the store, never rendered or applied | model metadata now surfaced in the model picker; preset data remains available for the sampling card |

## 4. Security

| Fix | Defect class | Resolution |
|---|---|---|
| **DNS rebinding (fetch)** | URL validation + DNS pre-resolution, but the dialer re-resolved the name — a rebinding answer slipped past | `pinnedPublicDialContext`: resolve → validate public → **dial the verified IP** (TLS keeps the original hostname) |
| **Zip-slip (updater)** | `extractZip` joined member names unvalidated | `safeZipPath` (absolute/volume/`..` rejected) + per-entry and total size caps; tested at both the path and archive layers |
| **Sandbox secret exposure** | `codeExec` ran with the FULL `os.Environ()` — model-generated python could read host API keys | `proc.SanitizedEnvironment` (shared with the Lab runner) |
| **Lab expansion escape** | lexical policy passed `$HOME/secret`, `~/x`, `${VAR}/x`, `%USERPROFILE%\x` while the shell resolved them outside the jail | expansion-token rejection + `HOME`/`USERPROFILE`/`TMPDIR` pinned to the workspace in the sanitized env |
| **WebSearch redirects** | used `http.DefaultClient` (any scheme/host on redirect) | dedicated client: ≤5 hops, http(s) only, no credentials |
| **Diagnostics leakage** | `tools.jsonl`/`llm.jsonl` shipped unredacted in the diagnostics zip (tool args can contain secrets) | redaction applied; `redact` extended to JSON members and env-style `key=value` |
| **SSRF test seam** | production-file bypass setter on an unsynchronized global | atomic `Bool` |
| **Unbounded bodies** | most JSON endpoints accepted arbitrary request sizes | `MaxBytesReader` on config (1 MB), sessions PUT (1 MB), research (64 KB), lab (256 KB), llama/abort (4 KB) |
| **Abort no-op** | malformed abort body returned `{ok:true}` while aborting nothing | 400 with the decode error |

## 5. Reliability, errors, resources

- **Per-run time budget** (`runTimeoutMinutes`, default 60, clamp 1–1440, 0 = off): a run was previously bounded only by `maxIterations` and per-call timeouts — a pathological turn could hold the run slot for hours. Timeout is distinguished from user abort in the end caption.
- **Swallowed persistence failures surfaced**: pre-run `Save` (user message could silently vanish), `AppendMessage` (a generated reply could be lost with no visible error), `AppendActivity`, `IndexTurn` — all now log, and reply loss emits a visible error activity.
- **netcheck worst case** 17.5 s → ~2.5 s (parallel probes; the sequential version stalled offline users on the engine-start gate).
- **Unbounded growth bounded**: recall index (5000-capsule retention, atomic rewrite), browser screenshots (keep 50), crash reports (keep 20), memory reads cached by (size, mtime) instead of re-parsing the file per tool call.
- **sysinfo on Windows 11 24H2+**: CIM via PowerShell first, `wmic` fallback; free-memory probe added.

## 6. Frontend

| Fix | Defect | Resolution |
|---|---|---|
| **createSession dead composer** | only prepended the session + set the id: the socket stayed bound to the OLD session (the stale guard then discarded every event for the new one) and state was never reset — the first message on a fresh session never streamed and `running` stuck true | full reset + socket rebind (mirrors `selectSession`) |
| **deleteSession stale view** | the deleted session's conversation stayed on screen | state cleared + `loadSession` for the newly active session |
| **No WS reconnect** | any mid-run disconnect permanently killed event delivery (`running` could never clear) | exponential-backoff auto-reconnect (1.5 s → 15 s, 20 attempts, reset on success) |
| **Activity captions** | the activity feed read `data.message/content/text` — the backend populates `caption`; every event rendered as a bare type label | `caption`-first formatter |
| **Engine poll leak** | the 2.5 s `/api/engine` poll ran for the app lifetime once started | `stopEnginePolling` + unmount cleanup |
| **Unhandled rejections** | Lab/Research store actions rethrew to fire-and-forget callers | state carries the error; no rethrow |
| **Two engine-state sources** | the Start/Stop toggle read `models.llamaRunning` while the badge read `engine.state` — they could disagree | both follow the authoritative `engine.state` |
| **Model picker** | filename-only labels | GGUF metadata (quant, params) in the option labels |
| Dead exports | 12 legacy wrappers + a legacy WS client + dead types removed; unused deps (`clsx`, `lucide-react`) removed | — |

## 7. Dead code removed (verified no references before deletion)

`llm`: `stateCh`, `publishEvent`, `ApplyPreset`, `SwitchModel`, `LoadOrStartWithModel`, `EnsureRunning`, `ListRemoteModels` (+ `models.go`), 10 unused `ForTest` seams. `api`: `uploadTimeout`, `marshalAttachmentsSafe`, `parseIntDefault`. `config`: `ProMode`, `VerboseAgent` (meaningless settings). `browser`: `Session.Info`. `research`: `searxngMaxResults`. `logging`: `Recent`. `netcheck`: `Force`. `installer`: `lookPathImpl` indirection. `termshell`: `quote`. `tools`: import-keeper stubs. `cmd/stress`: two tautological scenarios + the hand-copied `simulateExtractJSON` (now uses the exported real parser). `humanBytes` ×3 → `internal/humanize`. `cmd` files reorganized to match their contents (`setup.go`, `logs.go`, `diagnostics.go`, `license.go`).

## 8. Release engineering

- **The release-job trap** (the single most dangerous defect in the pipeline): the release job was gated on hardcoded `refs/tags/v1.1.3Z` with hardcoded asset names — every version bump silently skipped release publication until someone hand-edited the workflow. Now: version-agnostic `v*` tag gate + a first step verifying `GITHUB_REF_NAME == v{APP_VERSION}Z`; all asset/artifact names derive from `APP_VERSION`.
- Version grep literals in the three build jobs derive from `APP_VERSION`/`$env:APP_VERSION` instead of hardcodes.
- `config.Save` atomic; `scripts/build-and-zip.sh` and launcher `.bat` versioned.

## Validation performed (this release)

```text
go build -tags headless ./...                 PASS
go vet  -tags headless ./...                  PASS
go test -tags headless ./internal/... -count=1          21 packages PASS
go test -race (config, agent, llm, api, sessions,
               attachments, contextcache)               PASS
frontend: npm ci / typecheck / lint (0 warnings) / build
          + sync into web/static                    PASS
stress suite (release gate)                    30 pass / 0 fail
node scripts/release-version.mjs --check        PASS (all surfaces 1.1.4)
cross-compile GOOS=windows CGO_ENABLED=0       PASS
ZIP package extracted to a clean directory and
re-validated (build + vet + tests + stress)    PASS
```

Known limitations (unchanged or newly documented):

- The Wails/GTK desktop build cannot compile in this environment (no GTK4/WebKit dev libs); the Windows CI job builds it (CGO-free cross-compile verified here).
- Vision (mmproj) path not exercised with a real projector model.
- Small instruct models may not emit formal tool calls; loop mechanics covered by deterministic tests.
- The API is loopback-only without an auth token — the OS user account is the trust boundary.
- The Lab command policy is lexical (denylists + env pinning), not a kernel sandbox.
- Agent tool calls execute sequentially by design.

---

# Historical: v1.1.3Z Implementation Log (AAA upgrade, 2026-09-04)

## What was implemented

1. Automatic engine lifecycle — `api.Server.EnsureSetup` calls `Stack.PrewarmLLM`; `handleRun` gates every run on `Stack.EnsureLLMContext` (bounded 3 min); cold starts stream engine transitions to the UI.
2. Engine state machine — spec states `idle/downloading/starting/ready/running/busy/stopping/stopped/failed`; `SubscribeEvents` fans transitions to the API layer; `MarkBusy` wired to inference windows; bounded watchdog auto-restart (3 attempts, 1/2/4 s backoff, stop-suppressed).
3. Attachments (`internal/attachments`) — content-addressed staging (sha256, symlink-safe, 0600), size/count/timeout/chunk caps, text normalization + stable chunk identity, lexical retrieval with provenance headers, image classification for the vision pipeline.
4. Context cache (`internal/contextcache`) — content-keyed LRU (entries + bytes), version + config-fingerprint keys, TTL, prefix invalidation, stats.
5. Long-context plan (`internal/contextplan`) — explicit budget (`numCtx` minus output reserve), sections system/tools/recall/attachments/history with priorities; tool specs measured exactly before windowing; overflow emits a visible error.
6. API surface — `GET /api/engine`, engine events on every activity WS, `/api/attachments` upload/list/inspect/delete, `/api/run` with attachment retrieval injection and regenerate.
7. Frontend — dead-composer fix (`running` resets on done/error), real conversation view, engine card on authoritative states, composer attachment tray.
8. Headless build tag — `desktop.go` gated `!headless`; `desktop/headless.go` serves the same stack; `go build/test -tags headless` works without GTK/WebKit.
9. Configuration correctness — local `EffectiveBaseURL` derives from `LlamaHost:LlamaPort` (legacy value migrates); default `numCtx` 8192 → 16384 (measured: briefing + full tool schemas ≈ 9.8k tokens).
10. Tests — contextcache/attachments/contextplan/llm (real spawn + fake engine re-exec)/agent (fake SSE)/sessions/api suites.

## Runtime verification performed (v1.1.3Z)

- Stub-engine e2e: 16/16 PASS — launch, auto-start, ready, session, upload, run with attachment, engine envelope, regenerate, cache stats, shutdown.
- REAL engine e2e: llama.cpp b10642 (linux x64, CPU) + Qwen2.5-0.5B/1.5B Instruct GGUF: automatic startup to ready (~5 s), real inference streamed and persisted, busy → ready, regenerate.
- The oversized-request failure (9854 tok vs 8192 ctx) was reproduced against the real engine, root-caused, fixed and re-verified.
