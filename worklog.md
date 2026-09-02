# Worklog

# SHEYTAN-Local-Agent — Version Zeta

---

## Project Direction

Version Zeta establishes the next architectural stage of SHEYTAN-Local-Agent:

> A local-first AI software engineer with an isolated coding laboratory, automated verification, persistent engineering memory, and external research across GitHub, Reddit, and the web.

The project remains implemented in Go and continues to preserve the existing local-AI, tool, GUI, server, memory, diagnostics, browser, and orchestration infrastructure.

---

## Historical Foundation

### Original local-agent foundation

The project began as a local-first AI agent capable of interacting with the operating system through tools.

The original architecture included:

* local LLM integration
* shell execution
* file operations
* code execution
* web search
* browser automation
* Git operations
* multi-agent orchestration
* persistent memory
* sandboxing
* CLI/TUI

The original implementation was progressively evolved from an early TypeScript/Node prototype into the current Go implementation.

---

## Go Architecture

The project was migrated to a native Go implementation to provide:

* a single compiled executable
* better subprocess control
* stronger concurrency primitives
* native Windows support
* native filesystem APIs
* lower runtime overhead
* simpler distribution
* easier long-running service behavior

The repository now uses a modular internal package structure.

---

## Agent Runtime

The current agent runtime is centered around the orchestrator.

Its responsibilities include:

* planning
* LLM execution
* tool calls
* tool results
* iterative reasoning
* streaming activity
* abort support
* tool allow-listing
* history management
* persistent recall
* context pressure tracking

The orchestrator is the foundation on which the Zeta Coding Lab is being built.

---

## Memory and Recall

Persistent memory and recall were introduced so the agent can retain useful information between interactions.

The current system provides:

* persistent session information
* recall capsules
* bounded context injection
* history windowing
* context rollover support

Zeta extends this idea toward engineering memory:

* known project architecture
* previously encountered failures
* verified fixes
* successful commands
* rejected approaches
* research references

---

## Performance Work

The project has accumulated several performance-oriented improvements.

These include:

* deterministic tool ordering
* context window management
* persistent recall optimization
* streaming coalescing
* frame-oriented rendering
* LLM performance telemetry
* prompt-prefix stability
* engine configuration controls
* batch tuning
* optional KV-cache reuse
* optional FlashAttention
* configurable thread counts
* resource retention controls

The long-term goal is to keep the local agent responsive while performing long autonomous operations.

---

## Current Agent Tool Model

All agent tools implement a common interface conceptually equivalent to:

```go
type Tool interface {
    Name() string
    Description() string
    Parameters() any
    Run(ctx context.Context, args json.RawMessage) (string, error)
}
```

This allows the orchestrator to expose a stable tool surface to different local models.

---

# Version Zeta

## Zeta naming transition

The numbered user-facing release identity has been retired.

The application identity is now:

```text
SHEYTAN-Local-Agent
Version Zeta
```

The configuration uses:

```go
const (
    AppName    = "SHEYTAN-Local-Agent"
    AppVersion = "Zeta"
)
```

---

# Coding Lab

## Zeta Coding Lab goal

The Coding Lab changes the agent from:

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

The laboratory is intended to provide the controlled environment where that loop can operate.

---

## Workspace foundation

### File created

```text
internal/lab/workspace.go
```

### Purpose

The workspace package creates isolated task copies from an existing source repository.

The implementation:

* resolves absolute paths
* validates the source directory
* prevents source/workspace overlap
* generates cryptographically random task IDs
* creates disposable workspace directories
* recursively copies regular files
* skips `.git`
* skips symbolic links
* supports context cancellation
* deletes only workspaces below the configured laboratory root
* prevents path traversal through workspace-relative paths

### Current workspace format

```text
lab/workspaces/
└── task-YYYYMMDD-HHMMSS-random/
```

The workspace itself intentionally does not preserve the source `.git` directory.

Git state will be handled separately by the future patch/verification layer.

---

# Next Coding Lab Components

## Runner

Planned file:

```text
internal/lab/runner.go
```

Responsibilities:

* execute commands in a workspace
* capture stdout
* capture stderr
* capture exit status
* enforce context cancellation
* enforce timeouts
* record duration
* return structured execution results
* prevent execution outside the workspace policy

---

## Verifier

Planned file:

```text
internal/lab/verifier.go
```

Responsibilities:

* detect project language/toolchain
* identify test commands
* identify build commands
* identify lint/static-analysis commands
* execute verification stages
* aggregate results
* report PASS/FAIL evidence

---

## Lab Policy

Planned file:

```text
internal/lab/policy.go
```

Responsibilities:

* command allow/deny decisions
* network policy
* workspace boundary policy
* destructive operation restrictions
* resource controls
* execution mode

---

## Lab Task

Planned file:

```text
internal/lab/task.go
```

Responsibilities:

* task metadata
* task lifecycle
* task state
* iteration count
* failure history
* research references
* verification history

---

## Lab Session

Planned file:

```text
internal/lab/session.go
```

Responsibilities:

* persist autonomous coding sessions
* preserve task state
* retain execution logs
* retain verification results
* connect research evidence to changes

---

# Research Engine

Zeta adds dedicated external research instead of relying on a single generic web-search tool.

Planned package:

```text
internal/research/
```

Planned components:

```text
provider.go
github.go
reddit.go
web.go
searxng.go
rank.go
cache.go
```

---

## GitHub Research

GitHub is the primary source for code-level troubleshooting.

Planned capabilities:

```text
search code
search issues
search pull requests
read issue
read pull request
read repository file
inspect candidate patches
```

The agent should prefer exact error strings, package names, symbols, and version information when researching software failures.

---

## Reddit Research

Reddit is intended primarily for practical troubleshooting.

Typical research:

```text
real-world reports
hardware-specific workarounds
configuration issues
regressions
installation problems
community fixes
```

Reddit results should be treated as experience evidence rather than authoritative documentation.

---

## Web Research

The web layer should support:

* general search
* bounded HTTP fetch
* documentation lookup
* official project documentation
* tutorials
* technical articles

A self-hosted SearXNG installation is an intended preferred backend for unrestricted local use where practical.

---

# Evidence Model

The future research engine should distinguish source authority.

Conceptual ranking:

```text
official documentation
        ↓
maintainer-authored issue / PR
        ↓
high-quality project source
        ↓
technical community discussion
        ↓
Reddit/community reports
        ↓
unknown web content
```

This ranking is a decision aid, not an absolute truth system.

The final acceptance criterion remains local verification.

---

# Autonomous Repair

The final Zeta repair loop is planned as:

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
                               ↺
```

The loop must be bounded by:

```text
maximum iterations
maximum command duration
maximum task duration
workspace/resource limits
abort signal
```

---

# Engineering Principle

The most important Zeta principle is:

> The language model does not declare its own code correct.

The laboratory produces the evidence.

---

# Configuration

Version Zeta adds:

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

Default intentions:

```text
Coding Lab              enabled
Lab network             disabled
workspace preservation  disabled
command timeout        300 seconds
research                enabled
GitHub research         enabled
Reddit research         enabled
web research            enabled
```

---

# Safety Direction

The eventual execution path is:

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

The model should not receive unrestricted direct access to the host.

The future policy layer will be responsible for controlling:

* arbitrary paths
* destructive shell commands
* network access
* package installation
* privilege escalation
* process limits
* workspace boundaries

---

# Repository Documentation

The Zeta documentation set consists of:

```text
README.md
worklog.md
internal/aicontext/AI-CONTEXT.md
```

The README describes the architecture and operating philosophy.

The worklog records implementation history.

AI-CONTEXT describes the runtime environment and agent-facing operating context.

---

# Current State

## Foundation completed

```text
✓ Go application
✓ local LLM support
✓ remote OpenAI-compatible provider
✓ agent orchestrator
✓ tool registry
✓ persistent memory
✓ recall
✓ context management
✓ streaming
✓ browser automation
✓ Git tooling
✓ diagnostics
✓ stress testing
✓ native GUI
✓ headless server
✓ portable application layout
✓ Coding Lab workspace foundation
```

## Zeta work in progress

```text
□ Lab runner
□ Lab verifier
□ Lab policy
□ Lab task lifecycle
□ Research provider abstraction
□ GitHub research tools
□ Reddit research tools
□ general web research tools
□ research cache
□ evidence ranking
□ autonomous repair controller
□ engineering memory integration
□ Zeta GUI integration
□ full end-to-end autonomous coding tests
```

---

# Current Change

## Zeta workspace foundation

Added:

```text
internal/lab/workspace.go
```

The workspace manager currently supports:

```text
Create()
Remove()
PathFor()
```

and enforces the initial filesystem safety boundary.

---

# Immediate Next Stage

The next implementation file is:

```text
internal/lab/runner.go
```

The runner will become the controlled execution engine used by:

```text
run command
run tests
run build
run lint
run verifier
```

That component will then be connected to the existing agent tool system.

---

# Long-Term Target

The finished Zeta system should make a task like:

```text
Fix every failing test in this repository.
Use GitHub and Reddit to research unfamiliar errors.
Do not modify files outside the project.
Keep iterating until the test suite and build pass.
```

operate as:

```text
USER REQUEST
     ↓
LOCAL AI
     ↓
REPOSITORY ANALYSIS
     ↓
BASELINE TEST
     ↓
FAILURE EXTRACTION
     ↓
GITHUB / REDDIT / WEB RESEARCH
     ↓
CANDIDATE FIX
     ↓
PATCH
     ↓
TEST
     ↓
FAIL?
  YES ────────────────► RESEARCH AGAIN
   │
   NO
   ↓
BUILD
   ↓
LINT
   ↓
REGRESSION
   ↓
DIFF REVIEW
   ↓
VERIFIED RESULT
```

That is the defining objective of **SHEYTAN-Local-Agent — Version Zeta**.
