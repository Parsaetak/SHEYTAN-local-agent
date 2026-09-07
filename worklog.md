# SHEYTAN-Local-Agent — Worklog

## Current State

Date: 2026-09-07

Repository:

```text
https://github.com/Parsaetak/SHEYTAN-local-agent
```

Branch: `main`

Current release:

```text
v1.1.4Z
```

v1.1.4Z is a **functional-maturity and remediation release**: a full-repository audit followed by targeted fixes for every defect class the audit surfaced — concurrency, functional completion of documented-but-unwired subsystems, security hardening, error visibility, dead-code removal and release-engineering traps.

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
