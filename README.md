# SHEYTAN™ Local-Agent — Version Zeta

> A local-first autonomous AI software-engineering environment.
>
> **SHEYTAN™ is a trademark of Parsaetak · © 2024–2026 Parsaetak. All rights reserved.**
> Licensed under the Parsaetak Proprietary License v1.1.

SHEYTAN-Local-Agent is a local-first AI agent built to reason about software, use controlled tools, operate inside isolated coding workspaces, research technical problems, and verify whether proposed changes actually work.

The defining rule of Version Zeta is:

> **The AI may propose a solution. The laboratory decides whether the solution actually works.**

---

## Version Zeta

Version Zeta moves the project toward an autonomous coding laboratory rather than a conventional chat assistant.

```text
                         SHEYTAN — VERSION ZETA
                                  │
                    ┌─────────────┴─────────────┐
                    │                           │
                LOCAL AI                   ENGINEERING STATE
                    │                    memory / recall / sessions
                    ▼                           │
              ORCHESTRATOR ◄───────────────────┘
                    │
          ┌─────────┼──────────┐
          │         │          │
        TOOLS      LAB      RESEARCH
          │         │          │
          │         │     GitHub / Reddit / Web
          │         │          │
          │    ┌────┴─────┐    │
          │    │ WORKSPACE│    │
          │    │ isolated │    │
          │    └────┬─────┘    │
          │         │          │
          │      RUN / EDIT ◄──┘
          │         │
          │      VERIFY
          │         │
          └─────────┴──────────────► PASS / FAIL
                                         │
                                FAIL ────┴────► REPAIR ↺
```

The model remains replaceable. The orchestration, tooling, isolation, verification, research, memory, and UI are application-owned systems.

---

# Current Architecture

The project currently contains a Go backend/runtime with web and Windows desktop surfaces.

```text
Go runtime
 ├── local/remote LLM
 ├── orchestrator
 ├── tools
 ├── memory / recall
 ├── browser
 ├── sandbox
 ├── Coding Lab
 └── HTTP + WebSocket API
```

The current web frontend is a static UI embedded into the Go server, while Windows retains the existing native Fyne entry point.

The next UI stage is a **Node.js + React + TypeScript + Vite frontend**, while keeping the Go runtime as the execution and systems backend.

This separation keeps process execution, filesystem access, sandboxing, local LLM management, and orchestration in Go while giving the UI a modern application stack.

---

# Coding Lab

The Coding Lab is the central Version Zeta execution system.

A task begins from a source project:

```text
project/
├── source files
├── tests
├── dependencies
└── .git/
```

The Lab creates an isolated workspace:

```text
lab/
└── workspaces/
    └── task-YYYYMMDD-HHMMSS-random/
        ├── source files
        ├── tests
        └── build/test state
```

The autonomous task operates on the disposable workspace instead of directly modifying the original source tree.

## Implemented Lab layers

```text
internal/lab/
├── workspace.go
├── runner.go
├── policy.go
├── task.go
├── session.go
├── verifier.go
├── tool.go
├── lab_test.go
└── runner_test.go
```

Current foundation includes:

* isolated workspace creation
* source/workspace overlap protection
* `.git` exclusion
* symlink exclusion
* workspace path traversal protection
* bounded process execution
* context cancellation
* command timeouts
* shared stdout/stderr output limits
* conservative command policy
* task lifecycle management
* session registry
* objective verification
* verification invalidation after commands
* successful-task verification gating
* orchestrator tool integration
* regression tests for critical Lab behavior

---

# Verification Invariant

A successful Coding Lab task cannot be completed without a current passing verification.

```text
RUN / EDIT
     │
     ▼
verification becomes stale
     │
     ▼
VERIFY
     │
     ├── FAIL ──► repair / research
     │
     └── PASS
           │
           ▼
        FINISH
```

This prevents an older successful verification result from being reused after a later workspace mutation.

---

# Controlled Execution

The intended execution path is:

```text
LLM
 │
 ▼
Tool API
 │
 ▼
Command Policy
 │
 ▼
Coding Lab Runner
 │
 ▼
Isolated Workspace
 │
 ▼
Process
```

Current controls include:

```text
workspace boundary
command timeout
combined output limit
network policy
interactive-command policy
destructive-command policy
tool allow-list
abort support
structured execution results
```

The policy layer is defense-in-depth and is not a replacement for operating-system isolation.

---

# Autonomous Repair Loop

The intended Zeta loop is:

```text
USER TASK
   │
   ▼
ANALYZE PROJECT
   │
   ▼
BASELINE
   │
   ├── build
   ├── tests
   └── diagnostics
   │
   ▼
PLAN
   │
   ▼
EDIT
   │
   ▼
RUN
   │
   ▼
VERIFY
   │
   ├── PASS ──► REVIEW ──► COMPLETE
   │
   └── FAIL
          │
          ▼
       DIAGNOSE
          │
          ▼
       RESEARCH
          │
          ▼
        REPAIR ↺
```

The autonomous repair controller is still under active implementation.

---

# Research Engine

Version Zeta is designed to research technical failures instead of blindly guessing.

Planned research sources include:

```text
GitHub
Reddit
general web
official documentation
SearXNG
bounded HTTP fetches
```

Research should extract evidence such as:

```text
exact error strings
package/module names
symbols
version information
maintainer explanations
known regressions
candidate patches
workarounds
```

A discovered solution is treated as a **candidate**, then checked against the local project and version context.

---

# Evidence Model

The intended authority order is approximately:

```text
official documentation
        ↓
maintainer-authored issue / PR
        ↓
project source / release information
        ↓
high-quality technical discussion
        ↓
community reports / Reddit
        ↓
unknown web content
```

This ranking is guidance, not proof.

Local execution and objective verification remain the final acceptance criterion.

---

# AI Runtime

SHEYTAN supports local and remote OpenAI-compatible providers.

```text
SHEYTAN
   │
   ▼
LLM Client
   │
   ├── local llama.cpp
   └── remote OpenAI-compatible endpoint
```

The orchestrator provides:

* deterministic tool ordering
* tool allow-listing
* iterative tool calls
* streaming activity
* abort support
* history windowing
* persistent recall
* context-pressure tracking
* configurable iteration limits

---

# Memory and Recall

Persistent sessions and recall are already part of the runtime.

Zeta is extending that foundation toward engineering memory:

```text
project
├── architecture knowledge
├── dependency information
├── known failures
├── successful commands
├── verified fixes
├── rejected approaches
└── research evidence
```

The long-term goal is to make previously verified engineering knowledge reusable across tasks.

---

# User Interface — Zeta UI Migration

The existing UI is being replaced incrementally by a modern Node.js frontend while the Go runtime remains the systems core.

Target stack:

```text
Node.js
TypeScript
React
Vite
REST
WebSocket
```

Target architecture:

```text
             Node / React UI
                    │
             REST + WebSocket
                    │
                    ▼
              Go HTTP API
                    │
                    ▼
             Shared Go Runtime
                    │
       ┌────────────┼─────────────┐
       │            │             │
      LLM          LAB         RESEARCH
```

The migration is intended to improve:

* UI development speed
* component reuse
* maintainability
* interactive rendering
* live project visualization
* editor integration
* responsive application behavior

Critical process, filesystem, sandbox, LLM, and orchestration logic remains in Go.

The migration is **planned/in progress**, not yet complete.

---

# Live SVG Workspace

SVG becomes a first-class project artifact in the new UI.

```text
.svg file
   │
   ├── live SVG renderer
   │      ├── zoom
   │      ├── pan
   │      ├── selection
   │      └── instant refresh
   │
   └── source editor
          ├── XML editing
          ├── formatting
          ├── validation
          └── save
```

The intended workflow is:

```text
AI creates SVG
      ↓
Coding Lab writes file
      ↓
UI renders SVG live
      ↓
user edits SVG source
      ↓
preview updates immediately
      ↓
Lab verifies resulting artifact
```

The new interface is intended to allow both **visual inspection** and **direct SVG source editing** without leaving the application.

The SVG editor/preview is a target of the Node/React UI stage and is not yet a completed product feature.

---

# Tool Architecture

Agent tools implement the common interface:

```go
type Tool interface {
    Name() string
    Description() string
    Parameters() any
    Run(ctx context.Context, args json.RawMessage) (string, error)
}
```

Current tool families include:

```text
Core
├── files
├── shell
├── code execution
├── git
├── browser
├── web search
├── JSON
├── archive
├── bounded fetch
├── diff
└── data analysis

Coding Lab
└── coding_lab

Runtime
├── memory
├── recall
├── screenshot
└── linux terminal
```

The Coding Lab is registered through the shared runtime stack so the main runtime can use the same implementation.

---

# Configuration

Version Zeta adds dedicated Lab and research settings:

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

Default safety posture:

```text
Coding Lab           enabled
Lab network          disabled
workspace retention  disabled
command timeout     300 seconds
research             enabled
GitHub research      enabled
Reddit research      enabled
web research         enabled
```

---

# Project Structure

```text
SHEYTAN-local-agent/
├── cmd/
├── internal/
│   ├── agent/
│   ├── aicontext/
│   ├── api/
│   ├── artifacts/
│   ├── browser/
│   ├── chunking/
│   ├── config/
│   ├── continuum/
│   ├── lab/
│   ├── llm/
│   ├── logging/
│   ├── memory/
│   ├── multiagent/
│   ├── native/
│   ├── recall/
│   ├── runtime/
│   ├── sandbox/
│   ├── sessions/
│   └── tools/
├── web/
│   ├── embed.go
│   └── static/
├── models/
├── sessions/
├── logs/
├── sandbox/
├── workspace/
├── lab/
├── charts/
├── go.mod
├── go.sum
├── README.md
└── worklog.md
```

The future Node/React frontend will be added as a dedicated application surface rather than replacing the Go runtime.

---

# Running

Headless agent:

```bash
sheytan-local-agent ask "Analyze this project"
```

HTTP/web server:

```bash
sheytan-local-agent serve
```

Diagnostics:

```bash
sheytan-local-agent doctor
```

System information:

```bash
sheytan-local-agent sysinfo
```

Stress suite:

```bash
sheytan-local-agent stress
```

Version:

```bash
sheytan-local-agent version
```

Windows currently retains the native desktop entry point while the Node/React UI migration is developed.

---

# Version Naming

The user-facing numbered release identity is retired.

The project identity is:

```text
SHEYTAN-Local-Agent
Version Zeta
```

Configuration exposes:

```go
const (
    AppName    = "SHEYTAN-Local-Agent"
    AppVersion = "Zeta"
)
```

Internal engineering milestones may still be tracked in `worklog.md`, but the public application identity remains **Version Zeta**.

---

# Current Development Status

## Implemented

```text
✓ Go runtime and application architecture
✓ local and remote LLM support
✓ orchestrator/tool registry
✓ persistent sessions and recall
✓ streaming / abort / context management
✓ browser and Git tooling
✓ diagnostics and stress infrastructure
✓ portable application data
✓ Coding Lab workspace isolation
✓ Coding Lab runner
✓ conservative command policy
✓ Coding Lab task lifecycle
✓ Coding Lab session registry
✓ objective verification
✓ verification invalidation/gating
✓ Coding Lab orchestrator integration
✓ Lab regression tests
✓ shared stdout/stderr output limiting
```

## In progress

```text
□ unified API/runtime construction
□ autonomous repair controller
□ GitHub research provider
□ Reddit research provider
□ general web / SearXNG research
□ research cache and evidence ranking
□ engineering/coding memory integration
□ Node.js + React + TypeScript + Vite UI
□ live SVG editor/preview
□ full GUI/Lab integration
□ end-to-end autonomous coding verification
```

---

# Core Objective

The finished Version Zeta system should turn a request such as:

```text
Fix every failing test in this repository.
Research unfamiliar errors.
Do not modify files outside the project.
Keep iterating until the build and tests pass.
```

into a bounded engineering loop:

```text
REQUEST
  ↓
ANALYZE
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

The defining objective remains:

> **Build an AI software engineer whose claims are backed by executable evidence.**
