# SHEYTAN-Local-Agent — Agent Context

> Persistent engineering handoff for the next agent working on this repository.

Repository: https://github.com/Parsaetak/SHEYTAN-local-agent

Branch: `main`

Current release: `v1.1.5Z` (SHEYTAN Native AI Engine architecture foundation; see `worklog.md` for the full Phase 1 log).

**Read `worklog.md` before working.** It records the audit findings and the fixes this release shipped, including which subsystems were previously unwired and why.

**Read `ARCHITECTURE.md` for the implementation truth table and the validated future direction.** Its Part II records the planned architecture (small local models, tiered model routing, the Context Engine, context budgeting, multi-agent, artifact-based communication, document editing). None of that is implemented — never present it as current capability. Its Part III defines the documentation truth standard every change must follow.

---

# 1. Mission

SHEYTAN-Local-Agent is a local-first AI software-engineering environment.

Core principle:

> **The model proposes. The tools execute. The laboratory verifies.**

The current mission is maintaining and deepening **verified runtime behavior** — not adding visual surface area. Every change must keep the full path working:

```text
desktop launch
  → automatic llama.cpp startup
  → real model readiness
  → inference (streaming, tools, budget)
  → attachments / chunking / cache / recall
  → continuum rollover on pressure
  → objective verification (Lab)
  → visible, honest result
```

# 2. Development rules

Before modifying anything:

```text
inspect live repository
verify exact main commit
inspect relevant source
verify actual runtime behavior
```

Never:

```text
assume a commit works
assume a successful build means functionality
assume a UI control is wired
assume engine state is true
claim success without evidence
```

When the user says "done, check verify and continue":

```text
inspect → verify → diagnose → fix → retest → continue
```

# 3. Architecture (v1.1.5Z)

Backend: Go 1.26, Wails v3 (desktop shell), Go HTTP API + WebSocket on `127.0.0.1:8765`.

Engine stack (v1.1.5Z Phase 1 — the native engine is a supervised FOUNDATION, llama.cpp is still the only generation engine):

```text
React/TypeScript → Wails → Go Core → llm.Backend contract → llama.cpp (default + fallback)
                                              ↘ internal/native/engine → shtn-engine-host (C++, lifecycle/metrics only)
```

Frontend: React 19, TypeScript, Vite, Zustand; embedded via `web/static` (go:embed) — **`npm run build` must be re-run after any frontend change** so the embedded assets stay in sync.

Primary packages:

```text
internal/agent       orchestrator (per-run config snapshot, tool registry)
internal/llm         LlamaServer (engine lifecycle) + OpenAI-compatible client
                     + Backend contract + LlamaBackend + selection (v1.1.5Z)
internal/native/engine  SHEYTAN native engine: protocol, supervised runtime,
                     Backend adapter, hardware profile, metrics, concern types
                     (Phase 1: NO inference — see doc.go for the boundary decision)
internal/api         REST/WS surface, run registry, engine event bus
internal/runtime     Stack wiring (single source for every subsystem)
internal/config      Config + Source (copy-on-write live config)  ← READ THIS
internal/attachments staged uploads, chunking retrieval
internal/contextplan context budget authority
internal/contextcache content-keyed LRU cache
internal/continuum   chapter rollover (wired post-run since v1.1.4Z)
internal/lab         Coding Lab (policy, runner, verifier, repair)
internal/sandbox     Job-Object code-exec governor
internal/proc        process spawn/kill-tree + environment sanitization
internal/tools       17 agent tools
internal/memory      M1–M7 trust-classed store
internal/recall      BM25 recall + feedback steering
internal/research    multi-provider search
internal/multiagent  planner→executor→critic pipeline (CLI `ask --multi` ONLY — sequential, single model, no HTTP/UI surface)
internal/updater     engine download/update (zip-slip hardened)
internal/logging     log catcher + redaction
internal/sysinfo     hardware probe (CIM-first on Windows)
internal/netcheck    parallel connectivity probes
native/engine/       C++ native engine (CMake + Makefile): C ABI core,
                     shtn-engine-host subprocess, protocol + host tests
```

# 4. Configuration: the Source contract

`config.Source` (internal/config/source.go) is the **only** sanctioned way to mutate live configuration:

- Values obtained from `Load()` are **immutable by contract** — never mutate them.
- Writers go through `Update`/`UpdateErr`/`Store` (copy → mutate → publish).
- Runs, requests and engine starts take ONE snapshot per operation.
- `updater.RunScheduled` runs `CheckAndApply` on a private copy and publishes back.

Violating this contract reintroduces the v1.1.3Z data race (`*s.cfg = updated` in the patch handler). `TestSourceConcurrentReadWrite` and `TestConfigPatchIsRaceFree` guard it under `-race` — keep them passing.

# 5. Engine rules

- Engine state is backend-authoritative: `idle/downloading/starting/ready/running/busy/stopping/stopped/failed`. `ready`/`busy` are the reachable alive states; the UI must never invent any state. The native engine uses the SAME vocabulary and event shape (`llm.State*`, `llm.EngineEvent`) — there is no second state system; each engine owns its authoritative state and the API exposes one snapshot per engine.
- The engine start captures one config snapshot (a Settings PATCH mid-boot can no longer produce half-old/half-new launch flags).
- `MarkBusy` performs the whole transition under one lock — do not split it again (see `setStateLocked`).
- Streaming has NO overall client timeout by design; the stall watchdog (5 min zero-byte) provides the hang bound. Do not reintroduce a blanket `http.Client.Timeout` on the stream client.
- Engine downloads are context-bounded (10 min) and size-capped (2 GiB).
- v1.1.5Z backend rules: generation is routed by `llm.SelectGenerationBackend` — the native engine only when selected (`engineBackend: "native"`) AND `GenerationCapable()`; otherwise llama.cpp. In Phase 1 native generation is not implemented, so generation ALWAYS resolves to llama.cpp. The native backend's Generate/StreamGenerate/LoadModel return `llm.ErrNotImplemented` — that is the fallback signal, never a bug to "fix" by faking inference. Native engine failures never fail the llama path (best-effort, logged, visible in `native.state`).
- The native engine host (`shtn-engine-host`) runs with a sanitized environment, bounded op timeouts (10 s), a 1 MiB frame cap and a protocol/ABI handshake that fails closed. Build it from `native/engine/` (CMake or Make); Phase 1 does not ship or auto-download it.

# 6. Bounded-resource invariants

Every long-running operation must have: context cancellation, timeout, bounded output, cleanup. Current bounds to preserve:

```text
run time budget       runTimeoutMinutes (default 60, clamp 1..1440, 0=off)
engine watchdog       3 restarts/episode, 1/2/4s backoff
LLM retries           4 attempts (no retry after first emitted token)
stream stall          5 min zero-byte abort
lab output            2 MiB shared stdout+stderr
lab command timeout   ≤ 3600s     repair iterations ≤ 100
shell output          tool-level caps (64 KB simulator, 2 MB file reads)
attachments           manager-enforced size/count/chunk/processing caps
recall index          5000 capsules (compacted on load)
screenshots           50 kept     crash reports: 20 kept
WS hubs               128-event buffers, drop-on-slow (never block runs)
```

# 7. Security invariants (do not regress)

```text
loopback-only API + origin allow-list
path jail (traversal + symlink resolution) on every file tool
zip-slip-safe extraction with caps (llama.go AND updater)
fetch: URL validation + DNS pre-resolution + pinned dial IP
sandbox + lab: sanitized environment, HOME pinned to workspace
lab policy: dangerous/network/interactive/escape denylists
   (incl. $VAR/ ~/ %VAR% expansion tokens — see isExpandedPathToken)
secrets redacted: config GET, diagnostics zip, logs
```

Fail closed. Never weaken a control to unblock a feature.

# 8. Wired-surface contract (the v1.1.4Z lesson)

Before this release, several subsystems were fully implemented but had **zero production callers** (GGUF cards, continuum rollover, recall feedback, RunScheduled, sandbox settings, parts of sampling). The rule going forward:

> **A capability is either wired end-to-end (backend + API + UI + tests) or deleted. A stored-but-ignored setting is a defect.**

When adding a config field, grep for a consumer in the same change. When adding an endpoint, verify the frontend calls it (and vice versa).

# 9. Frontend contract

- The activity WebSocket reconnects automatically (exponential backoff); `done`/`error` always release the composer. Session create/delete rebind the socket and reset conversation state. If you touch session lifecycle, keep those invariants.
- `ActivityEvent.data.caption` is the display text (the backend `agent.Activity` contract) — formatters read caption first.
- The engine toggle AND the badge both read `engine.state` (never `models.llamaRunning`).
- The engine poll is stopped on view unmount.
- Recall feedback buttons send the exchange query (the user message preceding the reply) — the backend derives the same capsule id as `IndexTurn`.

# 10. Testing requirements

```bash
go test -tags headless ./internal/... -count=1
go test -race  -tags headless ./internal/agent/ ./internal/llm/ ./internal/api/
go vet -tags headless ./...
npm run typecheck && npm run lint && npm run build
go run ./scripts/stress-main stress          # release gate (0 fail required)
node scripts/release-version.mjs --check     # version surfaces consistent
# C++ native engine (when toolchain available):
cmake -S native/engine -B native/engine/build && cmake --build native/engine/build
ctest --test-dir native/engine/build         # 3 suites: engine, protocol, host
# Go↔C++ integration (skips when the host binary is not built):
go test -tags headless ./internal/native/engine/ -run TestRealCppHostEndToEnd
```

New runtime features need a regression test at the level where a real user would notice the failure (HTTP-level for API changes, request-shape tests for wire fields, behavioral tests for loop mechanics).

# 11. CI / release discipline

`.github/workflows/build-desktop.yml` is version-agnostic: the release job runs for any `v*` tag and verifies `GITHUB_REF_NAME == v{APP_VERSION}Z`. Version bumps flow from `package.json` via `node scripts/release-version.mjs` (syncs `config.go`, `build/config.yml`, `SIGNATURE`, workflow `APP_VERSION`). Hand-edit nothing else for a bump — then re-run the `--check`.

Do not reintroduce hardcoded version literals in the workflow (grep literals derive from `APP_VERSION`; the release gate is `startsWith(github.ref, 'refs/tags/v')`).

# 12. Definition of done

```text
frontend action → API → runtime → real operation → state update
→ visible result → error path → cancellation → tests → verification
```

A button is not a feature. An endpoint is not a feature. A compile is not a feature. A commit is not proof.

# 13. Immediate next tasks (priority order)

```text
1. Native engine Phase 2: real generation — implement Generate/
   StreamGenerate in the C++ core + host protocol (coarse-grained:
   whole requests, streamed chunks), then flip GenerationCapable().
   Extend internal/native/engine/{model,memory,kv,generation,scheduler}
   from types to implementations. Do NOT change the wire types or the
   ABI without bumping SHTN_PROTOCOL_VERSION / SHTN_ABI_VERSION.
2. Native model loading: GGUF parsing + weights/KV memory planning
   (memory.go MemoryPlan) behind LoadModel; then ModelInfo.
3. Native engine packaging: build + ship shtn-engine-host in the
   portable layout (bin/) with an update path (updater pattern).
4. Vision pipeline verification with a real mmproj projector
5. Tool-calling reliability tuning with larger instruct models
6. Continuum rollover exercise under real long sessions (it is wired +
   unit-tested; it has not yet been observed in a real multi-hour thread)
7. Context Engine foundations (PLANNED work — see ARCHITECTURE.md Part II)
8. Model tier discovery + capability-based routing (PLANNED — see
   ARCHITECTURE.md §II.2)
```

# 14. Architectural direction (PLANNED — read `ARCHITECTURE.md` Part II)

The validated direction for SHEYTAN's future AI runtime, in one paragraph:
small fast local models are the foundation (`many efficient agents +
orchestration + tools + external memory + verification`, never one
giant model); a tier ladder (Tier 0 smallest → Tier 4 optional
high-end multimodal) routes work by capability with model-agnostic,
hardware-adaptive rules; the Context Engine treats model context as
working memory backed by external project memory (structural index →
semantic index → hierarchical retrieval → budgeted context builder);
context budgeting enforces the smallest sufficient working set;
specialized agents (planner, coder, researcher, tester, debugger,
documentation, reviewer, verifier) communicate through structured
artifacts (`analysis.json`, `patch.diff`, `findings.md`,
`test-results.json`); document editing flows through section-aware
retrieval, structured patches and objective validation gates where the
model is never the authority on correctness.

Every clause above is **future architecture**. The current runtime is a
sequential single-agent loop with one model per session (plus the
CLI-only sequential multiagent pipeline). Full details, current-state
notes and the candidate model examples (verified 2026-09, labeled
non-integrated) are in `ARCHITECTURE.md`.

# 15. Final rule

Prefer real behavior + verification + reliability over more panels, more settings, more visual features. The next agent must work from evidence, not assumptions.
