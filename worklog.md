# Worklog

# SHEYTAN-Local-Agent — Version Zeta

---

## Project Direction

Version Zeta establishes the next architectural stage of SHEYTAN-Local-Agent:

> A local-first AI software engineer with an isolated coding laboratory, controlled execution, objective verification, persistent engineering memory, external research, and a modern interactive UI.

The project keeps the Go implementation as the systems/runtime foundation while moving the user interface toward a Node.js + React + TypeScript + Vite architecture.

---

# Historical Foundation

The project began as a local-first AI agent capable of interacting with the operating system through tools.

The original architecture included:

```text
local LLM
shell execution
file operations
code execution
web search
browser automation
Git
multi-agent orchestration
persistent memory
sandboxing
CLI/TUI
```

The implementation evolved from an earlier TypeScript/Node prototype into the current Go runtime.

---

# Go Runtime

The Go architecture remains responsible for system-level operations:

```text
LLM management
orchestration
tool execution
filesystem operations
process execution
sandboxing
Coding Lab
research services
memory
sessions
HTTP API
WebSocket activity
```

The Go runtime provides:

* one native backend
* native process control
* native filesystem APIs
* strong concurrency primitives
* native Windows integration
* portable application data
* predictable long-running service behavior

---

# Agent Runtime

The current agent runtime is centered around the orchestrator.

Responsibilities include:

```text
planning
LLM execution
tool calls
tool results
iterative reasoning
streaming activity
abort support
tool allow-listing
history management
persistent recall
context pressure tracking
```

Tools are exposed through a stable interface:

```go
type Tool interface {
    Name() string
    Description() string
    Parameters() any
    Run(ctx context.Context, args json.RawMessage) (string, error)
}
```

---

# Performance Work

The project already contains several performance-oriented mechanisms:

```text
deterministic tool ordering
history windowing
persistent recall
stream coalescing
frame-oriented UI pacing
LLM telemetry
prompt-prefix stability
batch tuning
KV-cache reuse controls
FlashAttention controls
configurable thread counts
resource retention controls
```

The UI migration will continue this direction by moving interactive presentation into a dedicated Node/React frontend while retaining the optimized Go runtime.

The target is responsive interaction during long-running local inference and autonomous coding tasks.

---

# Version Zeta Naming

The numbered user-facing release identity has been retired.

Current identity:

```text
SHEYTAN-Local-Agent
Version Zeta
```

Configuration:

```go
const (
    AppName    = "SHEYTAN-Local-Agent"
    AppVersion = "Zeta"
)
```

---

# Coding Lab

## Goal

The Coding Lab changes the operating model from:

```text
AI → answer
```

into:

```text
AI
 ↓
inspect
 ↓
plan
 ↓
edit
 ↓
execute
 ↓
test
 ↓
diagnose
 ↓
research
 ↓
repair
 ↓
verify
```

---

# Workspace Foundation

Implemented:

```text
internal/lab/workspace.go
```

The workspace manager provides:

```text
Create()
Remove()
PathFor()
```

Capabilities include:

```text
absolute-path normalization
source validation
source/workspace overlap protection
cryptographically random task IDs
isolated workspace creation
regular-file copying
.git exclusion
symbolic-link exclusion
context cancellation
root-restricted cleanup
workspace path traversal protection
```

Workspace format:

```text
lab/workspaces/
└── task-YYYYMMDD-HHMMSS-random/
```

---

# Lab Runner

Implemented:

```text
internal/lab/runner.go
```

Responsibilities:

```text
execute commands
validate working directories
capture stdout
capture stderr
capture exit status
measure execution duration
enforce context cancellation
enforce command timeout
bound combined output
return structured results
```

### Important hardening

stdout and stderr now share one output budget.

```text
configured limit
      │
      ▼
┌───────────────┐
│ shared budget │
└───────┬───────┘
        │
   ┌────┴────┐
   ▼         ▼
stdout     stderr
```

This prevents the effective retained output from becoming roughly twice the configured limit.

Regression coverage was added in:

```text
internal/lab/runner_test.go
```

---

# Lab Policy

Implemented:

```text
internal/lab/policy.go
```

The default policy is intentionally conservative.

Controls include:

```text
network access
dangerous commands
interactive commands
workspace escape patterns
command length
```

The policy blocks high-confidence destructive/system-level operations such as:

```text
disk formatting
filesystem destruction
registry deletion
system shutdown/reboot
destructive Git operations
Git publishing
privilege escalation
recognized network download commands
```

The policy is defense-in-depth and does not replace OS-level isolation.

---

# Lab Task Lifecycle

Implemented:

```text
internal/lab/task.go
```

Task states:

```text
pending
running
succeeded
failed
canceled
blocked
```

Lifecycle:

```text
NewTask
   ↓
Start
   ↓
RunCommand
   ↓
Verify
   ↓
Finish
```

Failure/control paths include:

```text
Fail
Cancel
Block
Close
```

---

# Verification Gate

The task lifecycle now enforces a critical invariant:

> A task cannot become successfully completed without a current passing verification.

A command execution invalidates previous verification.

```text
VERIFY PASS
     │
     ▼
new command
     │
     ▼
verification stale
     │
     ▼
VERIFY AGAIN
```

This prevents stale verification evidence from being reused after workspace changes.

---

# Verifier

Implemented:

```text
internal/lab/verifier.go
```

Verification supports:

```text
required checks
optional checks
build commands
test commands
explicit command checks
structured result aggregation
cancellation handling
failure reporting
task verification recording
```

Verification is recorded only after the complete verification sequence finishes.

This is important because individual verification commands themselves pass through task execution, which invalidates previous verification state.

---

# Coding Lab Tool

Implemented:

```text
internal/lab/tool.go
```

The orchestrator-facing tool is:

```text
coding_lab
```

Supported lifecycle actions:

```text
start_task
run
verify
finish
fail
cancel
block
close
```

The model therefore interacts with one controlled Lab interface rather than receiving unrestricted process/filesystem primitives.

---

# Lab Session Registry

Implemented:

```text
internal/lab/session.go
```

The registry provides:

```text
Create
Get
GetTask
Touch
Delete
List
Count
Active
RemoveCompleted
SnapshotTasks
```

This gives the Lab a runtime-level association between an autonomous session and its task.

---

# Lab Tests

Implemented:

```text
internal/lab/lab_test.go
internal/lab/runner_test.go
```

Coverage includes:

```text
workspace isolation
.git exclusion
path escape prevention
runner success
runner failure
runner timeout
policy rejection
verification gate
verification invalidation
failed verification recording
session lifecycle
Coding Lab end-to-end tool flow
combined output limiting
```

The repository's Windows CI workflow already runs:

```text
go test ./internal/... -tags headless
go vet ./internal/...
go build ...
```

so these tests are part of the existing validation path.

---

# Shared Runtime Integration

The Coding Lab was integrated into:

```text
internal/runtime/runtime.go
```

The runtime now initializes and registers:

```text
Coding Lab
TaskManager
WorkspaceManager
Runner
Policy
Verifier
SessionRegistry
```

The shared runtime exposes the Lab through:

```text
Stack.Lab
```

The orchestrator registers:

```text
coding_lab
```

when the Lab is enabled.

---

# Current Backend Integration Issue

The HTTP server currently constructs its own orchestrator independently.

Current architecture:

```text
CLI / desktop
      ↓
runtime.NewStack()
      ↓
shared orchestrator

HTTP server
      ↓
api.New()
      ↓
separate orchestrator
```

This creates a duplicated runtime surface.

The next backend integration task is therefore:

```text
unify API server with runtime.Stack
```

The objective is for CLI, desktop, and web UI to operate against the same runtime/tool registry.

---

# Research Engine

Version Zeta introduces configuration for dedicated technical research.

Current configuration surface:

```text
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

Planned structure:

```text
internal/research/
├── provider.go
├── github.go
├── reddit.go
├── web.go
├── searxng.go
├── rank.go
└── cache.go
```

---

# GitHub Research

Planned capabilities:

```text
search code
search issues
search pull requests
read issues
read pull requests
read repository files
inspect candidate patches
```

Research should prefer:

```text
exact error text
package/module names
symbols
version numbers
maintainer explanations
known regressions
```

GitHub evidence remains a candidate until locally verified.

---

# Reddit Research

Planned capabilities:

```text
community reports
hardware-specific workarounds
configuration issues
regressions
installation problems
practical fixes
```

Reddit is treated as experience evidence rather than authoritative documentation.

---

# General Web Research

Planned capabilities:

```text
general search
bounded HTTP fetch
official documentation lookup
technical documentation
tutorial lookup
project documentation
SearXNG integration
```

The research layer should preserve evidence and source context so the autonomous repair controller can make decisions from traceable information.

---

# Evidence Ranking

The intended authority order is:

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

The ranking is a decision aid.

Verification remains the final authority.

---

# Autonomous Repair Controller

The final repair loop is planned as:

```text
CREATE TASK
    ↓
CREATE WORKSPACE
    ↓
INSPECT PROJECT
    ↓
RUN BASELINE
    ↓
ANALYZE FAILURE
    ↓
RESEARCH
    ↓
PLAN FIX
    ↓
EDIT
    ↓
RUN
    ↓
VERIFY
    │
    ├── PASS → REVIEW → COMPLETE
    │
    └── FAIL → DIAGNOSE → RESEARCH → EDIT
```

The loop must be bounded by:

```text
maximum iterations
maximum command duration
task timeout
workspace/resource limits
abort signal
```

The repair controller is not yet implemented.

---

# Engineering Memory

Existing recall infrastructure stores useful past interactions.

The Zeta extension should connect coding tasks to engineering memory:

```text
project architecture
dependency versions
known errors
successful commands
failed approaches
verified patches
research references
verification evidence
```

The long-term objective is to prevent repeated rediscovery of already solved engineering problems.

---

# UI / UX Migration

## Current State

The current application includes:

```text
Go/Fyne native Windows UI
embedded static web UI
Go HTTP API
WebSocket activity stream
```

## Target State

The UI is being migrated toward:

```text
Node.js
React
TypeScript
Vite
```

with:

```text
REST
WebSocket
```

connecting to the Go backend.

Target:

```text
                Node / React
                    │
             REST + WebSocket
                    │
                    ▼
                 Go API
                    │
                    ▼
              Runtime Stack
                    │
        ┌───────────┼───────────┐
        ▼           ▼           ▼
       LLM         LAB       RESEARCH
```

The Go backend remains the authority for execution and system access.

---

# SVG Workspace

SVG is being promoted to a first-class UI artifact.

Target experience:

```text
SVG file
   │
   ├── visual canvas
   │      ├── zoom
   │      ├── pan
   │      ├── selection
   │      └── live refresh
   │
   └── source editor
          ├── XML
          ├── formatting
          ├── validation
          └── save
```

Target workflow:

```text
AI generation
     ↓
Lab file write
     ↓
live SVG render
     ↓
human visual inspection
     ↓
source edit
     ↓
instant preview
     ↓
verification
```

The feature is planned, not yet complete.

---

# Security Direction

The intended execution path remains:

```text
LLM
 ↓
Tool API
 ↓
Policy
 ↓
Runner
 ↓
Workspace
 ↓
Process
```

Future hardening should address:

```text
stronger OS isolation
resource limits
network isolation
package-install restrictions
process trees
workspace quotas
artifact controls
```

The existing Lab policy is not treated as a complete security boundary.

---

# Current Status

## Completed

```text
✓ Go runtime
✓ local LLM support
✓ remote OpenAI-compatible provider
✓ orchestrator
✓ tool registry
✓ persistent memory
✓ recall
✓ context management
✓ streaming
✓ abort handling
✓ browser automation
✓ Git tooling
✓ diagnostics
✓ stress infrastructure
✓ portable application layout
✓ Lab workspace isolation
✓ Lab runner
✓ Lab policy
✓ Lab task lifecycle
✓ Lab session registry
✓ Lab verifier
✓ Lab tool
✓ shared stdout/stderr output budgeting
✓ Lab regression tests
✓ shared runtime Lab registration
```

## In progress

```text
□ unify API server with shared runtime
□ autonomous repair controller
□ research provider abstraction
□ GitHub research
□ Reddit research
□ general web / SearXNG research
□ research cache
□ evidence ranking
□ engineering memory integration
□ Node.js + React + TypeScript + Vite UI
□ live SVG editor/preview
□ full Lab UI
□ end-to-end autonomous coding validation
```

---

# Immediate Engineering Sequence

The current sequence is:

```text
1. Unify API + runtime
        ↓
2. Harden existing backend functions
        ↓
3. Build research provider abstraction
        ↓
4. Implement GitHub research
        ↓
5. Implement Reddit research
        ↓
6. Implement web/SearXNG research
        ↓
7. Add research cache + evidence ranking
        ↓
8. Build autonomous repair controller
        ↓
9. Connect engineering memory
        ↓
10. Build Node/React/Vite UI
        ↓
11. Build live SVG workspace
        ↓
12. Integrate full GUI/Lab flow
        ↓
13. Run end-to-end autonomous verification
```

---

# Core Principle

The defining Version Zeta principle remains:

> **The language model does not declare its own code correct.**

The system must produce executable evidence.

```text
AI proposal
    ↓
controlled execution
    ↓
objective verification
    ↓
evidence
    ↓
accepted result
```

That principle defines **SHEYTAN-Local-Agent — Version Zeta**.
