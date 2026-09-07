# SHEYTAN™ Local-Agent

> **A local-first AI software-engineering laboratory.**
>
> The model proposes. The tools execute. The laboratory verifies.

SHEYTAN™ Local-Agent is a local-first desktop AI engineering environment built around Go, React/TypeScript, Wails v3, managed llama.cpp inference, controlled tools, isolated coding workspaces, research, memory, recall, and objective verification.

**SHEYTAN™ is a trademark of Parsaetak · © 2024–2026 Parsaetak. All rights reserved.**

Licensed under the **Parsaetak Proprietary License v1.1** (see `LICENSE`).

```text
Application:      SHEYTAN-Local-Agent
Current release:  v1.1.4Z
Codename:         Zeta
Branch:           main
```

---

# What SHEYTAN is

A single Windows-first desktop application that:

1. **Manages its own inference engine** — a llama.cpp server is downloaded, launched, health-checked, supervised (bounded auto-restart) and updated automatically. No manual engine babysitting.
2. **Runs a real agent loop** — plan → tool calls → observations → verification → final answer, with streaming, cancellation, retries, per-run time budgets and loop prevention.
3. **Executes engineering work in an isolated Coding Lab** — workspace copies, shell/network/dangerous-command policy, objective verification gates, repair loops, snapshot-before-promote.
4. **Treats long context as an engineering problem** — measured context budgets, structured chunking, a content-keyed cache, provenance-tagged retrieval, recall of past exchanges, and automatic chapter rollover for sessions that outgrow their window.

The model is never the authority on whether an engineering task succeeded — objective verification is.

# Architecture

```text
┌─────────────────────────────────────────────┐
│              React / TypeScript             │
│                  Vite UI                    │
└──────────────────────┬──────────────────────┘
                       │ REST + WebSocket (127.0.0.1)
                       ▼
┌─────────────────────────────────────────────┐
│                   Go API                    │
└──────────────────────┬──────────────────────┘
                       ▼
┌─────────────────────────────────────────────┐
│                 Go Runtime                  │
│  agent orchestrator · tool registry (17)   │
│  llama.cpp lifecycle · sandbox governor    │
│  attachments · chunking · context cache     │
│  context plan · memory · recall · continuum │
│  research · sessions · browser · vision     │
└──────────────────────┬──────────────────────┘
                       ▼
┌─────────────────────────────────────────────┐
│             llama.cpp server                │
│          local model inference              │
└─────────────────────────────────────────────┘
```

Critical execution logic belongs to Go. Presentation and interaction logic belong to React. The production desktop app embeds the built frontend (`web/static/`) via `go:embed` — no separate frontend server is needed.

# Major capabilities (all implemented and tested)

| Area | What works |
|---|---|
| **Engine lifecycle** | Automatic download → launch → `/health` readiness → `ready`; bounded auto-restart (3 attempts, exponential backoff); deliberate stop suppression; port-conflict adoption; compat ladder (4 launch profiles); one-shot self-update when a model needs a newer engine; scheduled update loop (daily/weekly/monthly) |
| **Engine states** | `idle / downloading / starting / ready / running / busy / stopping / stopped / failed` — backend-authoritative, fanned out over every activity WebSocket; the UI never invents them |
| **Agent loop** | Streaming responses with think-tag splitting and native `reasoning_content`; tool calls with argument validation; tool-result follow-up turns; iteration cap (default 25); per-run time budget (default 60 min); abort with partial-result preservation; regenerate; timeout-vs-abort distinguished in the UI |
| **Tools** | 17 registered tools: `shell`, `files`, `codeExec` (Job-Object sandboxed), `webSearch`, `git`, `browser`, `dataAnalysis`, `json`, `archive`, `fetch`, `diff`, `screenshot`, `linux`, `coding_lab`, `research`, `memory` (+ sandbox override). Tool schemas are measured exactly before windowing |
| **Coding Lab** | Isolated workspace copies (symlinks skipped, `.git` excluded), lexical command policy (dangerous/network/interactive/escape denylists + expansion-token hardening), 2 MiB bounded output, sanitized environment (secrets scrubbed, `HOME` pinned to the workspace), objective verification (trivial `echo`-style checks rejected), bounded repair loop with repeat-command detection, patch export, snapshot-before-promote |
| **Attachments** | Content-addressed staging (sha256, symlink-safe, no exec bits), size/count/processing/chunk caps, text normalization + semantic chunking, cached retrieval with provenance headers, image classification into the vision pipeline |
| **Context** | Measured context plan (system / tools / recall / attachments / history sections with priorities); history windowing to the budget; content-keyed LRU cache (entries + bytes + TTL bounds); overflow surfaces as a visible error instead of an engine rejection |
| **Long context** | Continuum chapter rollover: when a session crosses the pressure threshold, facts/decisions/threads are distilled into a framework and the conversation continues in a fresh chapter session (the UI follows automatically) |
| **Memory & recall** | Trust-classed memory (M1–M7, external material quarantined), BM25 recall with recency boost and 👍/👎 feedback steering (persistent sidecar) |
| **Research** | Auto/GitHub/Reddit/DuckDuckGo/SearXNG providers, TTL-cached, authority-ranked, provenance-tagged |
| **Observability** | `app.log` + `tools.jsonl` + `llm.jsonl` with rotation and bounds, crash reports (pruned), diagnostics zip with secret redaction, perf HUD (TTFT / tok/s), engine logs ring |
| **Concurrency** | Copy-on-write live configuration (no data races between Settings patches and active runs), mutex-guarded registries, per-run config snapshots, race-detector-clean core |

# Supported environment

- **Windows 10/11 x64** — the primary target (GUI subsystem exe, console-less, DPI-aware, process-tree kills via `taskkill /T`).
- **Linux x64** — desktop build requires GTK4 / WebKitGTK 6.0 dev libraries; the `headless` build tag runs the same stack without them.
- Hardware probing (CPU/RAM/GPU) uses CIM via PowerShell with a `wmic` fallback (wmic is removed on Windows 11 24H2+).

# Model / runtime integration

- **Local engine**: managed llama.cpp server (bundled CPU build by default; Vulkan offload detected automatically). Models are discovered from the `models/` folder, with GGUF header metadata (architecture, quantization, context length, parameter count) surfaced in the UI.
- **Remote providers**: any OpenAI-compatible endpoint (`SHEYTAN_PROVIDER=remote`). llama.cpp-only request fields (`top_k`, `min_p`, `n_ctx`, `repeat_last_n`) are automatically omitted for remotes; OpenAI-standard sampling fields are sent to both.

# Installation

```text
SHEYTAN-Local-Agent-Windows-x64-v1.1.4Z.zip
└── SHEYTAN-Local-Agent/
    ├── SHEYTAN-Local-Agent.exe   (GUI app + embedded UI + HTTP/WS API)
    ├── sheytan-local-agent.bat   (portable launcher)
    ├── AI-CONTEXT.md             (the model's operating manual)
    └── README / LICENSE / SIGNATURE / worklog
```

Unzip anywhere and run `SHEYTAN-Local-Agent.exe`. On first launch the app creates its portable data layout next to the executable:

```text
SHEYTAN-Local-Agent/
├── models/         (drop .gguf files here; mmproj-*.gguf pairs as vision projectors)
├── sessions/       (one JSON per session + activity sidecars)
├── logs/           (app.log, tools.jsonl, llm.jsonl, crashes/, screenshots/)
├── bin/            (auto-downloaded llama-server)
├── attachments/    (content-addressed staged uploads)
├── lab/workspaces/ (isolated coding-lab copies)
├── sandbox/        (governed code execution)
├── recall/         (index + feedback sidecars)
└── config.json
```

The engine binary downloads automatically when the machine is online; drop a prebuilt `llama-server(.exe)` into `bin/` for offline installs.

# Configuration

Settings are edited in the UI (`Settings` view) or by patching `config.json` (the API accepts partial JSON objects). Selected keys:

| Key | Default | Meaning |
|---|---|---|
| `provider` | `local` | `local` (managed llama.cpp) or `remote` (OpenAI-compatible endpoint) |
| `model` | first `.gguf` | active local model |
| `llamaPort` | 8080 | managed engine port |
| `llamaAutoStart` | true | prewarm engine at launch |
| `llm.numCtx` | 16384 | context window (measured minimum for the full tool schema is ~9.8k tokens) |
| `llm.*` | — | sampling: temperature, top-p, top-k, min-p, penalties, mirostat, seed, stop |
| `maxIterations` | 25 | agent-loop iteration cap |
| `runTimeoutMinutes` | 60 | per-turn budget (0 = unbounded) |
| `sandboxEnabled` | true | Job-Object resource governor for `codeExec` |
| `thinkingMode` | false | externalized reasoning blocks |
| `recallEnabled` | true | inject relevant past exchanges |
| `continuumEnabled` | true | automatic chapter rollover |
| `labEnabled` | true | Coding Lab tool |
| `researchEnabled` | true | research tool + providers |
| `updateSchedule` | daily | engine update cadence (`off` disables) |

Environment overrides (`SHEYTAN_*`) are documented in `sheytan help`.

# Usage

```bash
# desktop app (default on Windows/Linux)
SHEYTAN-Local-Agent.exe

# headless server + UI in a browser
sheytan-local-agent serve --port 8765

# one-shot headless agent turn
sheytan-local-agent ask "summarize ./notes" --session work

# multi-agent planner/executor/critic pipeline (CLI)
sheytan-local-agent ask "..." --multi

# health, diagnostics, engine update
sheytan-local-agent doctor
sheytan-local-agent diagnostics
sheytan-local-agent update --status
```

REST/WS surface (loopback only): `/api/state`, `/api/engine`, `/api/models`, `/api/sessions`, `/api/config`, `/api/llama`, `/api/run`, `/api/abort`, `/api/attachments`, `/api/tools`, `/api/lab`, `/api/research`, `/api/feedback`, `/ws/activity?sessionId=`.

# Agent / tool capabilities and limits

- The agent loop executes tools **sequentially** (parallel execution is a deliberate non-goal for now).
- Small instruct models may not emit formal tool calls even when tools are advertised — the loop mechanics are covered by deterministic tests.
- The `linux` tool is an honest in-process shell simulator (its description tells the model so); real shell work goes through `shell`/`codeExec`/the Lab.
- Vision requires an `mmproj-*.gguf` projector paired with the active model; screenshots capture the primary display (Windows).
- The Coding Lab's command policy is lexical — it blocks known-dangerous, network, interactive and escape tokens, and the runtime additionally pins `HOME` and scrubs secrets, but it is not a kernel-level sandbox.

# Security model

- The LLM is an **untrusted proposal source**. Runtime policy is authoritative.
- The HTTP/WS API binds `127.0.0.1` and rejects non-approved origins; it has no auth token by design (loopback-only desktop app), so any local process can reach it — treat the machine's user account as the trust boundary.
- Filesystem tools are jailed to the portable data root with traversal + symlink resolution checks; archive extraction is zip-slip-safe with entry/total caps.
- `fetch` enforces public-destination SSRF controls end-to-end: URL validation, DNS pre-resolution, **and** dialed-IP pinning (DNS-rebinding window closed).
- Lab and sandbox processes run with a sanitized environment (API keys/tokens/credentials scrubbed) and a workspace-pinned `HOME`.
- Secrets never appear in API responses (redacted on read), and diagnostics zips redact config, crash logs and structured logs.

# Development

```bash
# backend (no GTK needed)
go build -tags headless ./...
go test -tags headless ./internal/... -count=1
go test -race -tags headless ./internal/agent/ ./internal/llm/ ./internal/api/ ...
go vet -tags headless ./...

# frontend
npm install
npm run typecheck
npm run lint
npm run build        # tsc + vite + sync into web/static

# release stress suite (gate used by CI and build-and-zip.sh)
go run ./scripts/stress-main stress

# release consistency (package.json -> config.go / config.yml / SIGNATURE / workflow)
node scripts/release-version.mjs --check
```

The desktop (Wails) build for Linux needs `libgtk-4-dev`, `libwebkitgtk-6.0-dev`, `libsoup-3.0-dev`, `pkg-config`; the Windows build is CGO-free and cross-compilable.

After any frontend change, `npm run build` must be run so `web/static` (the embedded assets) stays in sync.

# Testing

- **19+ Go test packages** — engine lifecycle (real process spawn/kill via a fake llama.cpp re-exec), agent loop (fake SSE engine: streaming, tool calls, abort, error propagation), API surface (HTTP-level session/attachment/config/feedback contracts), attachments, chunking, context cache, context plan, continuum, lab (policy, repair loop, verification), memory, recall, research (SSRF/alias contracts), sessions (concurrency, sidecar bounds), termshell, tools, vision, releasegate, plus v1.1.4Z regression tests for the config source race, sampling wire format, GGUF parser, stream stall watchdog, zip-slip and escape tokens.
- **Stress suite** — 30 scenarios (hostile prompts, garbage tool args, shell injection, memory/session contracts, release-surface pinning) run in CI and as a release gate.
- **CI** (`.github/workflows/build-desktop.yml`) — audit job (version sync + frontend verify), Windows job (tests + GUI exe + console probe + package + zip verification), Linux job (tests + stress suite + package), release job (version-agnostic tag gate, checksum-verified publication).

# Version

`v1.1.4Z` — see `worklog.md` for the complete remediation history and `agent.md` for the engineering handoff context.

# License

SHEYTAN™ Local-Agent is proprietary software. See `LICENSE` for the governing terms.
