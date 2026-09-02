# SHEYTAN™ Local-Agent — Version Zeta

> A local-first autonomous AI coding laboratory.
>
> **SHEYTAN™ is a trademark of Parsaetak · © 2024–2026 Parsaetak. All rights reserved.**
> Licensed under the Parsaetak Proprietary License v1.1.

SHEYTAN-Local-Agent is a native Go application designed to make a local AI model function as a practical software-engineering agent.

The **Version Zeta** architecture combines:

* a local LLM mind
* persistent memory and recall
* deterministic tool calling
* isolated coding workspaces
* controlled command execution
* automated tests/builds/linting
* autonomous repair loops
* Git inspection
* GitHub research
* Reddit research
* general web research
* browser automation
* vision
* multi-agent orchestration
* diagnostics and telemetry
* native Windows UI
* headless server operation
* portable application data

The central design principle is:

> **The AI may propose a solution. The laboratory decides whether the solution actually works.**

---

## Version Zeta

Version Zeta changes the project's center of gravity from a general-purpose local AI assistant into an **autonomous coding laboratory**.

The existing agent runtime already provides the core reasoning/tool loop, persistent recall, streaming activity, context management, tool allow-listing, iteration limits, and abort support.

Zeta adds the surrounding engineering system that allows those capabilities to operate on real software projects safely.

### Zeta architecture

```text
                         SHEYTAN-LOCAL-AGENT
                                  │
                 ┌────────────────┴────────────────┐
                 │                                 │
             LOCAL AI                           MEMORY
                 │                                 │
          LM Studio / LLM                 recall / sessions
                 │
                 ▼
          ┌───────────────┐
          │  ORCHESTRATOR │
          │ plan / act    │
          │ observe / fix │
          └───────┬───────┘
                  │
       ┌──────────┼───────────┐
       │          │           │
       ▼          ▼           ▼
     TOOLS       LAB       RESEARCH
       │          │           │
       │          │      ┌────┼─────────┐
       │          │      │    │         │
       │          │    GitHub Reddit   Web
       │          │
       │    ┌─────┴──────────┐
       │    │    WORKSPACE   │
       │    │ isolated copy  │
       │    └─────┬──────────┘
       │          │
       │     edit / execute
       │          │
       │    ┌─────▼──────────┐
       │    │  VERIFICATION  │
       │    │ test/build/lint│
       │    └─────┬──────────┘
       │          │
       │       PASS / FAIL
       │          │
       └──────────┴───────────────↺
```

---

# Coding Lab

The Coding Lab is the core Version Zeta subsystem.

A task starts with a source repository:

```text
project/
├── source files
├── tests
├── dependencies
└── .git/
```

The Lab creates a disposable copy:

```text
lab/
└── workspaces/
    └── task-YYYYMMDD-HHMMSS-random/
        ├── source files
        ├── tests
        └── build environment
```

The source repository is never supposed to be the direct execution target of the autonomous loop.

The current workspace layer:

* creates unique task directories
* copies regular files
* excludes `.git`
* ignores symbolic links
* prevents source/workspace overlap
* prevents workspace path traversal
* restricts cleanup to the laboratory root
* supports cancellation through `context.Context`

Current file:

```text
internal/lab/workspace.go
```

---

# Autonomous coding loop

The intended Zeta repair loop is:

```text
USER TASK
   │
   ▼
ANALYZE REPOSITORY
   │
   ▼
RUN BASELINE
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
   ├──────── PASS ────────► REVIEW ──► COMPLETE
   │
   └──────── FAIL
                 │
                 ▼
              DIAGNOSE
                 │
                 ▼
             RESEARCH
                 │
                 ▼
                EDIT
                 │
                 └───────────────► VERIFY
```

The agent should not stop merely because code "looks correct."

A successful autonomous task should be supported by executable evidence such as:

```text
✓ tests passed
✓ build passed
✓ lint passed
✓ regression checks passed
✓ requested behavior reproduced
✓ final diff reviewed
```

---

# Research Engine

Zeta gives the coding agent external research capabilities.

Research sources are intentionally separated by purpose.

## GitHub

GitHub is the primary code-repair source.

The future research interface is designed around:

```text
github_search_code()
github_search_issues()
github_search_prs()
github_read_issue()
github_read_pr()
github_read_file()
```

Typical use:

```text
compiler error
      ↓
GitHub code search
      ↓
GitHub issues
      ↓
pull requests
      ↓
maintainer discussion
      ↓
candidate solution
      ↓
local verification
```

A discovered GitHub solution is treated as a **candidate**, not automatically as truth.

---

## Reddit

Reddit is intended for practical troubleshooting and real-world experience.

Typical questions:

```text
"Has anyone encountered this?"
"What fixed this?"
"Is this bug still present?"
"Does this work on Intel hardware?"
"What workaround is currently used?"
```

Reddit evidence is useful for experience and troubleshooting, but should normally receive less authority than official documentation or maintainer-authored changes.

---

## General web

General web research fills gaps between repositories, documentation, discussions, and tutorials.

The system may use:

```text
SearXNG
DuckDuckGo
direct bounded HTTP fetches
official documentation
project documentation
```

A self-hosted SearXNG instance is particularly useful because it can act as a unified local search gateway.

---

# Evidence-driven repair

Zeta follows this rule:

```text
SEARCH RESULT
      ↓
EXTRACT CLAIM
      ↓
CHECK VERSION / CONTEXT
      ↓
COMPARE SOURCES
      ↓
APPLY CANDIDATE
      ↓
RUN LOCALLY
      ↓
PASS → ACCEPT
FAIL → REJECT / RESEARCH AGAIN
```

This is important because language models can produce technically convincing but incorrect fixes.

The laboratory is the final authority for whether a candidate repair is operationally successful.

---

# Local AI

SHEYTAN-Local-Agent supports local and remote OpenAI-compatible LLM providers.

The local architecture is designed around:

```text
SHEYTAN Agent
     │
     ▼
LLM client
     │
     ├── bundled llama.cpp
     │
     └── compatible local endpoint
```

The project also maintains support for remote OpenAI-compatible providers.

The model remains replaceable. The orchestration, tools, research system, workspace, verification, and memory are owned by SHEYTAN.

---

# Persistent memory

SHEYTAN already contains persistent session and recall infrastructure.

The Zeta direction extends that concept to coding work:

```text
project knowledge
    │
    ├── architecture
    ├── dependencies
    ├── known failures
    ├── previous fixes
    ├── successful commands
    ├── rejected approaches
    └── verified repairs
```

This allows future tasks to benefit from previous verified work instead of rediscovering the same problem repeatedly.

---

# Tool architecture

Every agent capability should implement a controlled tool interface.

Conceptually:

```go
type Tool interface {
    Name() string
    Description() string
    Parameters() any
    Run(ctx context.Context, args json.RawMessage) (string, error)
}
```

The current orchestrator already uses this registry-style architecture.

Zeta extends the tool surface toward:

```text
LOCAL
├── files
├── shell
├── code execution
├── git
├── browser
├── JSON
├── archive
├── diff
└── data analysis

LAB
├── create workspace
├── inspect workspace
├── run command
├── run tests
├── build
├── lint
├── verify
└── cleanup

RESEARCH
├── web search
├── web fetch
├── GitHub code
├── GitHub issues
├── GitHub PRs
├── Reddit search
└── Reddit fetch
```

---

# Security model

The autonomous agent should not receive unrestricted access to the entire operating system.

The intended execution path is:

```text
LLM
 │
 ▼
Tool API
 │
 ▼
Policy
 │
 ▼
Lab Runner
 │
 ▼
Isolated Workspace
 │
 ▼
Process
```

Important controls include:

```text
workspace boundary
command timeout
iteration limit
network policy
tool allow-list
abort support
resource quota
structured logging
```

The current `workspace.go` already establishes the filesystem-boundary foundation for this model.

---

# Project structure

Current architecture includes:

```text
SHEYTAN-local-agent/
│
├── cmd/
│   ├── ask.go
│   ├── context.go
│   ├── diagnostics.go
│   ├── license.go
│   ├── root.go
│   ├── serve.go
│   ├── stress.go
│   └── ...
│
├── internal/
│   ├── agent/
│   │   └── orchestrator.go
│   │
│   ├── aicontext/
│   │   └── AI-CONTEXT.md
│   │
│   ├── api/
│   ├── artifacts/
│   ├── browser/
│   ├── chunking/
│   ├── config/
│   ├── continuum/
│   ├── llm/
│   ├── logging/
│   ├── memory/
│   ├── multiagent/
│   │
│   └── lab/
│       └── workspace.go
│
├── models/
├── sessions/
├── logs/
├── browser-profile/
├── sandbox/
├── workspace/
├── lab/
│   └── workspaces/
│
├── charts/
├── go.mod
├── go.sum
├── README.md
└── worklog.md
```

The exact tree will expand as the Zeta Lab and Research engines are implemented.

---

# Configuration

Version Zeta adds dedicated configuration for the Coding Lab and research system.

Important settings include:

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

The defaults are intentionally conservative.

For example:

```text
Lab network access      OFF
workspace preservation  OFF
command timeout         300 seconds
research enabled        ON
GitHub research         ON
Reddit research         ON
web research            ON
```

---

# Version naming

The numbered release system is retired for the main project identity.

The current project identity is:

```text
SHEYTAN-Local-Agent
Version Zeta
```

The configuration exposes:

```go
const (
    AppName    = "SHEYTAN-Local-Agent"
    AppVersion = "Zeta"
)
```

Future internal implementation milestones may still be documented separately, but the user-facing release identity remains **Version Zeta**.

---

# Running

Interactive GUI:

```bash
sheytan-local-agent
```

Headless agent:

```bash
sheytan-local-agent ask "Analyze this project"
```

Server:

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

The eventual Coding Lab interface is intended to expose commands such as:

```bash
sheytan-local-agent lab run "Fix all failing tests"
```

and:

```bash
sheytan-local-agent lab run \
    --repo ./project \
    "Implement the requested feature and make the complete test suite pass"
```

---

# Design principles

## Local-first

Local computation should be preferred whenever practical.

## Evidence over confidence

A plausible AI answer is not equivalent to a verified software change.

## Isolate before executing

Autonomous code should run in a controlled laboratory workspace.

## Research before guessing

Known bugs, issues, patches, documentation, and real-world reports should be searched before reinventing solutions.

## Verify before accepting

Tests, builds, linting, regression checks, and diffs are part of the reasoning loop.

## Keep the model replaceable

The architecture should work with multiple local models and compatible endpoints.

## Keep state portable

Application state belongs inside the application data root whenever practical.

## Make failures observable

Every major action should produce useful logs and structured activity events.

---

# Current Zeta development status

### Completed foundation

* Go application architecture
* native GUI/server surface
* local/remote LLM support
* agent orchestration
* tool registry
* persistent memory/recall
* context management
* streaming activity
* browser automation
* Git tooling
* diagnostics
* stress testing
* portable application data
* isolated Lab workspace foundation

### Current development sequence

```text
Zeta
 │
 ├── 1. Workspace        ← current foundation
 ├── 2. Runner
 ├── 3. Verifier
 ├── 4. Lab tool adapter
 ├── 5. Research provider
 ├── 6. GitHub research
 ├── 7. Reddit research
 ├── 8. Web research
 ├── 9. Autonomous repair loop
 ├── 10. Research cache
 ├── 11. Coding memory
 └── 12. Zeta integration/UI
```

---

# Core objective

The long-term objective of Version Zeta is not simply:

```text
"AI that writes code"
```

It is:

```text
                HUMAN
                  │
                  ▼
               TASK
                  │
                  ▼
             LOCAL AI MIND
                  │
        ┌─────────┼──────────┐
        │         │          │
        ▼         ▼          ▼
      CODE      RESEARCH    TOOLS
        │         │          │
        └─────────┼──────────┘
                  │
                  ▼
             CODING LAB
                  │
                  ▼
              EXECUTE
                  │
                  ▼
              VERIFY
             /       \
          PASS       FAIL
           │           │
           ▼           ▼
        REVIEW      RESEARCH
           │           │
           ▼           └──────► REPAIR
        COMPLETE              ↺
```

The laboratory, not the language model, determines whether the software is actually fixed.
