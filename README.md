# SHEYTAN™ Local-Agent

> **A local-first AI software-engineering laboratory.**
>
> The model proposes. The tools execute. The laboratory verifies.

**SHEYTAN™** is a local-first AI agent/runtime for software engineering. It combines local or OpenAI-compatible LLMs with controlled tools, isolated coding workspaces, objective verification, bounded autonomous repair, technical research, persistent engineering memory, and a modern web UI.

**SHEYTAN™ is a trademark of Parsaetak · © 2024–2026 Parsaetak. All rights reserved.**

Licensed under the **Parsaetak Proprietary License v1.1**.

---

# Project Identity

```text
Application:  SHEYTAN-Local-Agent
Version:     1.1.2
Codename:    Zeta
```

The numeric application version and human-facing codename are separate runtime values.

The defining architectural principle is:

```text
LLM claim
   ↓
tool execution
   ↓
objective evidence
   ↓
verification
   ↓
accept / reject
```

A language model is never treated as the final authority on whether a code change works.

---

# What SHEYTAN Is

SHEYTAN-Local-Agent is intended to behave as a bounded AI software engineer rather than a conventional chatbot.

A typical engineering task can follow this lifecycle:

```text
REQUEST
   ↓
UNDERSTAND
   ↓
INSPECT
   ↓
BASELINE
   ↓
PLAN
   ↓
EDIT
   ↓
RUN
   ↓
VERIFY
   │
   ├── PASS ──► REVIEW ──► COMPLETE
   │
   └── FAIL
          ↓
       DIAGNOSE
          ↓
       RESEARCH
          ↓
        REPAIR ↺
```

The system is designed so that every stage can be constrained by policies, timeouts, workspace boundaries, output limits, cancellation, and verification rules.

---

# Architecture

The project uses Go as the systems/runtime core and a modern TypeScript frontend for the interactive UI.

```text
                    ┌────────────────────────┐
                    │   React / TypeScript   │
                    │        Vite UI         │
                    └───────────┬────────────┘
                                │
                         REST + WebSocket
                                │
                                ▼
                    ┌────────────────────────┐
                    │       Go HTTP API      │
                    └───────────┬────────────┘
                                │
                                ▼
                    ┌────────────────────────┐
                    │      Go Runtime        │
                    ├────────────────────────┤
                    │ orchestrator           │
                    │ LLM                    │
                    │ tools                  │
                    │ Coding Lab             │
                    │ research               │
                    │ memory / recall        │
                    │ sandbox                │
                    │ process control        │
                    │ sessions               │
                    └────────────────────────┘
```

The runtime owns system-level operations.

The frontend owns presentation, interaction, session navigation, activity display, and future project/Lab visualizations.

Critical execution logic remains in Go.

## Desktop Shell

The shipping desktop application is a single self-contained executable built on the Wails v3 shell (`internal/desktop`):

```text
sheytan-local-agent.exe
├── React production assets (embedded from web/static)
├── Go HTTP API + WebSocket (same process, same handler)
└── WebView2 window (Windows) / WebKitGTK (Linux)
```

One in-process HTTP handler routes `/api/` and `/ws/` to the Go backend and everything else to the embedded asset server. No external browser, localhost listener, or frontend server is required in production.

The legacy Fyne desktop UI (`internal/ui`, `fyne.io/fyne/v2`) was fully removed in the Zeta release — the React/TypeScript UI is the only application UI.

---

# Runtime Components

```text
cmd/
internal/
├── agent/
├── aicontext/
├── api/
├── artifacts/
├── brand/
├── browser/
├── chunking/
├── config/
├── continuum/
├── desktop/      (Wails v3 shell)
├── installer/
├── lab/
├── llm/
├── logging/
├── memory/
├── multiagent/
├── native/
├── netcheck/
├── proc/
├── recall/
├── releasegate/
├── research/
├── resources/
├── runtime/
├── sandbox/
├── screen/
├── sessions/
├── sysinfo/
├── termshell/
├── tools/
├── updater/
└── vision/
src/              (React/TypeScript frontend)
scripts/
web/              (embedded production assets)
```

Primary subsystems:

```text
LLM
Orchestrator
Tool Registry
Coding Lab
Research
Memory
Recall
Sandbox
Process Control
HTTP API
WebSocket Activity
React UI
```

---

# Coding Lab

The Coding Lab is the execution and verification core for autonomous software changes.

A source project is treated as the original authority:

```text
project/
├── source files
├── tests
├── dependencies
└── .git/
```

The Lab operates on a disposable workspace:

```text
lab/
└── workspaces/
    └── <task-id>/
        ├── source files
        ├── tests
        └── build/test state
```

The workspace is intentionally separated from the original source tree during autonomous modification.

## Lab capabilities

```text
workspace creation
workspace isolation
source/workspace overlap protection
workspace path validation
symlink exclusion
.git handling
snapshots
patch export
promotion
bounded command execution
timeouts
cancellation
combined output limits
stdin isolation
environment sanitization
command policy
task lifecycle
session registry
objective verification
repair iteration limits
```

---

# Verification Invariant

The most important Lab invariant is:

> **A task cannot be considered successfully completed without current passing verification.**

Verification becomes stale whenever the workspace is mutated.

```text
RUN / EDIT
     ↓
verification stale
     ↓
VERIFY
     ↓
PASS ──► continue
FAIL ──► diagnose / repair
```

Trivial successful shell commands are not accepted as proof of correctness.

Examples explicitly rejected as meaningful verification include:

```text
echo
printf
true
false
exit 0
```

The verifier can discover native project checks for common project types such as:

```text
Go
Node.js
Python
Rust
```

The verification model is based on meaningful project-level checks rather than arbitrary command success.

---

# Autonomous Repair

The Lab contains a bounded repair controller.

The controller is designed around:

```text
baseline
   ↓
diagnose
   ↓
repair
   ↓
verify
   ↓
repeat only when justified
```

The repair budget is bounded:

```text
default: 25 iterations
absolute maximum: 100 iterations
```

The controller also avoids blindly retrying the same normalized command indefinitely.

Every repair action remains subject to the normal Lab policy and runner controls.

Autonomous repair is therefore bounded rather than unconstrained.

---

# Promotion and Recovery

Changes are not written directly into the original source tree during normal Lab execution.

Promotion follows this model:

```text
Lab workspace
     ↓
snapshot
     ↓
current verification
     ↓
promotion
     ↓
source tree
```

Snapshots are created before source mutation.

The Lab workspace is intentionally retained after promotion so that evidence, diagnostics, and recovery information are not immediately destroyed.

The Lab can also export a binary-safe patch representation.

---

# Controlled Execution

Agent-controlled commands pass through multiple layers:

```text
LLM
 ↓
Tool API
 ↓
Policy
 ↓
Lab Runner
 ↓
Workspace
 ↓
Process
```

The execution stack provides defense in depth.

Controls include:

```text
workspace boundary
path validation
command policy
interactive-command restrictions
network restrictions
Git restrictions
timeouts
output limits
stdin isolation
environment sanitization
process-tree cancellation
```

These controls reduce risk but are **not equivalent to a dedicated operating-system or hardware security boundary**.

---

# Filesystem Security

The filesystem path layer validates tool paths relative to a configured base.

Important protections include:

```text
absolute-path rejection where required
relative traversal rejection
base-directory validation
symlink-prefix escape detection
safe path joining
existing-path checks
workspace restriction
```

Tools are expected to use the checked path layer instead of constructing unrestricted filesystem paths themselves.

---

# Shell and Git Security

Shell and Git execution are constrained by both path validation and command policy.

The Git policy rejects common escape mechanisms such as:

```text
-C
--git-dir
--work-tree
absolute external paths
parent traversal
file:// repository targets
```

Workspace-local Git workflows such as committing are permitted where policy allows them.

Git publishing is blocked by the Lab policy.

---

# Process Control

The process layer is designed to cancel entire process trees rather than only the direct child process.

Unix-like systems use process groups.

Windows uses:

```text
CREATE_NEW_PROCESS_GROUP
taskkill /PID /T /F
```

This reduces the risk of timed-out or canceled commands leaving child processes alive.

The Lab runner also avoids inheriting the host's complete environment.

Sensitive host environment values are removed before Lab execution.

Standard input is not inherited from the host terminal for Lab commands.

---

# Network and Fetch Security

Model-controlled HTTP access is bounded and validated.

The fetch layer includes:

```text
request timeout
response-size limits
redirect limits
redirect re-validation
localhost blocking
loopback blocking
link-local blocking
private-network blocking
DNS/network validation
```

The purpose is to prevent a model-controlled fetch path from being used to access internal/private services.

---

# Research Engine

Research is a first-class runtime capability.

Current research backends include:

```text
Auto
GitHub
Reddit
DuckDuckGo
SearXNG
Web compatibility alias
```

The agent-facing research tool is:

```text
research
```

The `web` backend name is retained as a compatibility alias for DuckDuckGo. It is **not** a generic unrestricted web provider.

Research requests and results are normalized.

The service supports:

```text
bounded result counts
bounded timeouts
provider validation
result normalization
deduplication
relevance ranking
authority ranking
content hashing
metadata
```

---

# Research Evidence

Research is evidence, not automatic truth.

The intended authority ordering is approximately:

```text
official documentation
        ↓
maintainer / project material
        ↓
project source / release information
        ↓
high-quality technical discussion
        ↓
community reports
        ↓
unknown web content
```

This ordering is advisory.

A high-ranked result does not become a confirmed fix merely because it ranks highly.

The final acceptance criterion remains:

```text
local project state
+
local execution
+
objective verification
```

---

# GitHub Research

The GitHub provider is intended to surface engineering evidence such as:

```text
exact error messages
package/module names
symbols
known regressions
maintainer explanations
issues
pull requests
workarounds
```

Responses are bounded and endpoint construction is validated.

GitHub research remains evidence that must be checked against the local project and version context.

---

# Reddit Research

Reddit provides practical community evidence such as:

```text
installation failures
hardware-specific issues
configuration problems
regressions
practical workarounds
```

Reddit evidence is treated as experiential rather than authoritative.

OAuth/token requirements are enforced where applicable.

---

# Web Research

The current architecture supports:

```text
DuckDuckGo
SearXNG
```

with:

```text
timeouts
result limits
body limits
normalization
ranking
deduplication
```

External web content must not silently become authoritative engineering knowledge.

---

# Memory and Recall

The memory layer separates trusted engineering knowledge from provisional observations.

Current memory classes:

```text
M1 = trusted user facts
M2 = preferences
M3 = project state
M4 = decisions
M5 = procedures / learned fixes
M6 = conversation summaries
M7 = observations / untrusted or provisional knowledge
```

Memory entries can carry provenance and trust information.

External research provenance is quarantined by default rather than being silently promoted into trusted memory.

The system distinguishes:

```text
what was observed
what was reported
what was inferred
what was verified
```

This distinction is essential to reliable autonomous engineering.

---

# AI Context

The project contains an operational AI constitution in:

```text
internal/aicontext/AI-CONTEXT.md
```

The AI context is responsible for communicating:

```text
runtime identity
system information
LLM/provider state
capabilities
tool availability
research availability
memory expectations
verification rules
safety constraints
failure discipline
answer discipline
```

The runtime can generate context using the actual registered tool set.

This is important because the model should see capabilities that match the tools that actually exist at runtime.

---

# Agent Tool Architecture

Tools implement the common interface:

```go
type Tool interface {
    Name() string
    Description() string
    Parameters() any
    Run(ctx context.Context, args json.RawMessage) (string, error)
}
```

The orchestrator exposes the tools actually registered in the runtime.

Tool ordering is deterministic.

Configured tool restrictions are applied before capability information is exposed to the model.

This prevents divergence between:

```text
advertised capabilities
```

and:

```text
actual capabilities
```

---

# LLM Runtime

SHEYTAN supports local and OpenAI-compatible LLM endpoints.

Conceptually:

```text
SHEYTAN
   ↓
LLM Client
   ├── local llama.cpp
   └── OpenAI-compatible endpoint
```

The runtime supports:

```text
streaming
tool calls
history management
iteration limits
abort handling
telemetry
context-pressure management
dynamic tool exposure
```

The LLM remains a replaceable component.

The engineering infrastructure around it is application-owned.

An LLM response without usable choices is treated as an error rather than being silently accepted.

---

# API

The Go server provides HTTP endpoints for the frontend/runtime boundary.

Core API areas include:

```text
state
sysinfo
models
presets
tools
sessions
session details
session updates
run
abort
activity WebSocket
```

Session operations are session-scoped.

The WebSocket activity channel is also session-scoped.

API configuration responses redact sensitive credentials.

Configuration updates behave as patches against the current configuration rather than blindly replacing unrelated settings.

---

# WebSocket Activity

Live activity is delivered over:

```text
/ws/activity?sessionId=<session-id>
```

Activity connections are scoped to a session.

The server enforces an origin policy rather than accepting arbitrary cross-origin WebSocket requests.

The React frontend connects to the active session's activity channel.

---

# React / TypeScript UI

The current frontend stack is:

```text
Node.js
TypeScript
React
Vite
Zustand
REST
WebSocket
```

The package currently targets:

```text
Node >= 22.12.0
npm  >= 10.0.0
```

The main scripts are:

```bash
npm run dev
npm run build
npm run build:web
npm run sync:web
npm run preview
npm run typecheck
npm run lint
npm run format
npm run format:check
```

The React UI currently includes the core runtime shell:

```text
session navigation
session creation
session deletion
active-session selection
live activity
connection state
runtime metrics
model state
agent composer
abort support
```

The frontend communicates with the Go runtime through the HTTP API and WebSocket activity channel.

The legacy Fyne desktop UI was removed in the Zeta release; the React UI is the application UI, embedded into the Wails desktop shell and served from `web/static`. It remains active development work and should not be described as feature-complete.

---

# Frontend Design Direction

The intended UI evolution is:

```text
Agent
 ├── conversation / activity
 ├── runtime state
 ├── models
 └── execution status

Coding Lab
 ├── task
 ├── workspace
 ├── commands
 ├── verification
 ├── repair iterations
 ├── snapshots
 └── promotion

Research
 ├── query
 ├── providers
 ├── evidence
 ├── ranking
 └── source details

Memory
 ├── recall
 ├── provenance
 ├── trust
 └── engineering knowledge

Artifacts
 ├── files
 ├── diffs
 └── future SVG workspace
```

Lab and Research UI panels shipped in v1.1.1Z (`LabPanel.tsx`, `ResearchPanel.tsx`,
`SettingsPanel.tsx` — all embedded in the release binary).

The v1.1.2Z release adds a 120Hz-first motion system (`src/motion.css`):
compositor-only animations (transform/opacity/filter), frame-quantized
durations, spring easing curves, staggered panel entrances, animated
workspace view transitions, and full `prefers-reduced-motion` support.

The SVG editor/preview remains later work.

---

# Configuration

Important Version Zeta configuration areas include:

```text
LabEnabled
LabWorkspaceRoot
LabCommandTimeoutSec
LabMaxIterations
LabKeepWorkspaces
LabAllowNetwork

ResearchEnabled
ResearchBackend
ResearchSearXNGURL
ResearchMaxResults
ResearchTimeoutSec
ResearchCacheTTLMin
ResearchGitHub
ResearchReddit
ResearchWeb
ResearchUserAgent
```

Lab iteration policy:

```text
default = 25
maximum = 100
```

The effective value is bounded even when configuration is malformed or absent.

---

# Security Posture

The project follows a defense-in-depth model.

Important security properties include:

```text
workspace boundaries
checked filesystem paths
symlink avoidance
ZIP extraction validation
SSRF protection
bounded HTTP responses
bounded redirects
command policy
Git restrictions
process-tree cancellation
environment sanitization
stdin isolation
configuration secret redaction
WebSocket origin checks
verification gating
research provenance quarantine
```

Security-sensitive changes should preserve these invariants rather than bypassing them for convenience.

---

# Development Rules

## For developers

Changes should preserve the separation:

```text
UI
 ↓
API
 ↓
runtime
 ↓
controlled execution
```

Do not move security-critical execution logic into the browser merely to simplify UI implementation.

When changing an API contract:

```text
backend contract
↓
TypeScript types
↓
store/runtime usage
↓
UI consumers
```

must remain synchronized.

When changing Lab behavior:

```text
policy
runner
task lifecycle
verification
promotion
tests
```

must be considered together.

When changing research behavior:

```text
provider
normalization
ranking
deduplication
trust/provenance
tests
```

must remain consistent.

---

# Rules for Coding Agents

Agents working on this repository should follow these rules.

### 1. Inspect before modifying

Never assume a file, endpoint, capability, or architecture exists.

Check the live repository first.

### 2. Preserve verified boundaries

Do not weaken:

```text
workspace isolation
path checks
process cancellation
command policy
SSRF protection
secret redaction
verification gates
memory quarantine
```

to make a feature easier.

### 3. Treat external research as evidence

A GitHub issue, Reddit post, web page, or model-generated claim is not automatically true.

Use research to form hypotheses.

Use the local project to test them.

### 4. Never fake verification

Do not turn:

```text
echo
true
exit 0
```

into a success signal.

Do not report a task as fixed without current meaningful verification.

### 5. Re-verify after mutation

Any workspace mutation can invalidate previous verification.

### 6. Prefer bounded behavior

Loops, retries, commands, network access, output, and repair iterations must have explicit limits.

### 7. Keep API contracts explicit

JSON names are part of the API contract.

Use the backend's actual field names.

### 8. Keep documentation truthful

Do not document planned functionality as implemented.

Use these states:

```text
Implemented
In progress
Planned
Experimental
```

### 9. Read AI-CONTEXT.md

The operational constitution in:

```text
internal/aicontext/AI-CONTEXT.md
```

is part of the agent/runtime design and should be treated as an engineering contract.

### 10. Verify the actual repository state

A requested change is not considered complete merely because a patch was written locally.

The authoritative state is the committed repository state plus successful verification.

---

# Verification Commands

After Go changes:

```bash
go mod tidy
go test ./internal/... -tags headless -count=1
go test ./internal/lab/ -count=1
go test ./internal/research/ -count=1
go vet ./internal/...
go build -o /tmp/sheytan .
```

The desktop shell (`internal/desktop`) compiles against Wails v3. On Windows — the CI runner — no cgo is required. On Linux, building or testing any package that imports it needs GTK4/WebKitGTK development headers (`pkg-config --exists gtk4 webkitgtk-6.0`); without them, verify with the Windows cross-build instead:

```bash
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build ./...
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go vet ./...
```

The stress suite builds headless on any host via the dedicated entry point:

```bash
go build -o /tmp/sheytan-stress ./scripts/stress-main
SHEYTAN_DATA_DIR=/tmp/sheytan-stress-root /tmp/sheytan-stress stress
```

After frontend changes:

```bash
npm ci
npm run typecheck
npm run lint
npm run format:check
npm run build:web
```

`npm ci` installs exactly the committed `package-lock.json` (CI uses it too);
`format:check` is the honest CI gate — never commit a tree that needs
`npm run format` to pass it.

Full frontend/runtime synchronization:

```bash
npm run build
```

The exact commands that are appropriate for a change should be selected from the actual project structure rather than copied mechanically.

A test command itself is not sufficient evidence when the test suite does not cover the changed behavior.

---

# Releases

CI is `.github/workflows/build-desktop.yml`. Every push to `main` builds
both platforms; pushing a tag publishes a GitHub Release:

```bash
git tag v1.1.2Z
git push origin v1.1.2Z
```

The release assets are produced by the workflow itself:

```text
SHEYTAN-Local-Agent-Windows-x64-v1.1.2Z.zip
SHEYTAN-Local-Agent-Linux-x64-v1.1.2Z.zip
```

Both zips are portable: extract, double-click `SHEYTAN-Local-Agent.exe`
(Windows) or run `SHEYTAN-Local-Agent` (Linux). Models, sessions, logs, and
Coding Lab workspaces live next to the executable. The Windows zip also
ships the `sheytan-local-agent.bat` launcher, and the exe carries the brand
icon, the Parsa Tak signature, and a DPI-aware manifest.

---

# Repository Hygiene

Before committing:

```text
no stale module paths
no dead imports
no hardcoded secrets
no unrestricted filesystem paths
no unbounded network operations
no accidental host-environment inheritance
no fake verification paths
no stale documentation claims
no generated build artifacts unless intentionally tracked
```

Format and typecheck frontend changes.

Run relevant Go tests and vet checks.

---

# Current Status

## Implemented

```text
✓ canonical Go module path
✓ TOML dependency normalization
✓ runtime version/codename separation
✓ dynamic runtime tool exposure
✓ AI operating context
✓ persistent sessions
✓ memory classes and provenance
✓ memory quarantine for external research
✓ bounded Coding Lab
✓ workspace isolation
✓ task/session lifecycle
✓ command policy
✓ path security
✓ Git restrictions
✓ process-tree cancellation
✓ environment sanitization
✓ stdin isolation
✓ objective verification
✓ verification invalidation
✓ native verification discovery
✓ snapshot support
✓ patch export
✓ promotion gating
✓ bounded autonomous repair
✓ GitHub research
✓ Reddit research
✓ DuckDuckGo research
✓ SearXNG research
✓ research ranking
✓ research deduplication
✓ research caching
✓ SSRF-aware fetch path
✓ API hardening
✓ WebSocket origin restrictions
✓ per-session abort
✓ LLM empty-response rejection
✓ llama startup deadlock fix
✓ ZIP extraction protection
✓ React/Vite frontend foundation
✓ session-aware React activity WebSocket
✓ REST API integration
✓ Wails v3 desktop shell (single self-contained exe)
✓ legacy Fyne UI fully removed (Zeta)
✓ Coding Lab frontend panel
✓ Research frontend panel
✓ Settings frontend panel
✓ repaired Linux CI (GTK4 + WebKitGTK-6.0 for Wails v3)
✓ pinned Node 24 / Go 1.26 toolchains in CI
✓ versioned legacy sources retired (v1.1.1Z)
✓ 120Hz-first motion system with spring easing + staggered entrances (v1.1.2Z)
✓ animated workspace view transitions (v1.1.2Z)
✓ idle WebSocket standby: activity stream stays connected between runs (v1.1.2Z)
✓ models API returns rich {id, name, path, sizeBytes} descriptors (v1.1.2Z)
✓ Settings panel null-crash fixed: fresh installs no longer render a blank page (v1.1.2Z)
✓ first-launch auto-session: Agent workspace is immediately live (v1.1.2Z)
✓ Windows CI restored: gen-syso icon/resources + -H=windowsgui + .bat launcher (v1.1.2Z)
✓ stress release-surface gate now runs in CI (v1.1.2Z)
✓ stale web/static build artifacts + broken Taskfile + old e2e scripts removed (v1.1.2Z)
```

## In Progress

```text
□ complete React feature parity polish with the pre-migration desktop UI
□ richer project/task inspection
□ end-to-end autonomous coding workflows
□ broader verification coverage
□ frontend integration tests
```

## Later

```text
□ live SVG editor
□ advanced artifact workspace
□ richer visual debugging
□ additional research providers
□ more sophisticated engineering-memory workflows
```

---

# Engineering Philosophy

SHEYTAN is built around one principle:

> **Intelligence is not only the model.**

A useful engineering agent combines:

```text
Model
× Reasoning
× Tools
× Memory
× Planning
× Verification
× Runtime
× Compute
```

A stronger model can improve reasoning.

It cannot replace:

```text
safe execution
real project state
persistent evidence
objective verification
bounded control
```

The final system should therefore behave less like:

```text
"the AI says the code is fixed"
```

and more like:

```text
"the system inspected the project,
changed the isolated workspace,
ran the relevant checks,
verified the result,
preserved the evidence,
and only then accepted the change."
```

---

# Final Objective

The long-term target is a local AI software engineer capable of receiving a request such as:

```text
Fix every failing test in this repository.
Research unfamiliar errors.
Do not modify files outside the project.
Keep iterating until the build and tests pass.
```

and executing a bounded, auditable engineering loop:

```text
REQUEST
   ↓
INSPECT
   ↓
BASELINE
   ↓
DIAGNOSE
   ↓
RESEARCH
   ↓
PLAN
   ↓
PATCH
   ↓
RUN
   ↓
VERIFY
   │
   ├── PASS → REVIEW → COMPLETE
   │
   └── FAIL → RESEARCH → REPAIR ↺
```

The objective is not to make the model appear certain.

The objective is to make the system produce **evidence-backed engineering results**.
