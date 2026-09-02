# Worklog

# SHEYTAN-Local-Agent — Version Zeta

---

## Project Direction

Version Zeta establishes the next architectural stage of SHEYTAN-Local-Agent:

> A local-first AI software engineer with an isolated coding laboratory, controlled execution, objective verification, persistent engineering memory, external research, autonomous bounded repair, and a modern interactive UI.

The project keeps the Go implementation as the systems/runtime foundation while moving the user interface toward a Node.js + React + TypeScript + Vite architecture.

The defining engineering principle is:

> The language model does not declare its own code correct. The system must produce executable evidence.

---

# Current Identity

The application identity is split into a numeric release version and a human-facing codename.

```go
const (
    AppName     = "SHEYTAN-Local-Agent"
    AppVersion  = "1.1.0"
    AppCodename = "Zeta"
)
```

Runtime Go version reporting uses the actual Go runtime rather than a hardcoded version string.

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

```text
native backend
native process control
native filesystem APIs
strong concurrency primitives
native Windows integration
portable application data
predictable long-running service behavior
```

---

# Agent Runtime

The agent runtime is centered around the orchestrator.

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
deterministic tool ordering
dynamic tool discovery
```

Tools use the common interface:

```go
type Tool interface {
    Name() string
    Description() string
    Parameters() any
    Run(ctx context.Context, args json.RawMessage) (string, error)
}
```

The orchestrator exposes its actual runtime registry rather than maintaining a separate execution-only list.

The AI context now receives the enabled registered tools from the orchestrator so the model's advertised capabilities match the tools actually available to it.

---

# Module Path Normalization

The Go module path was standardized across the repository.

Canonical module path:

```text
github.com/Parsaetak/SHEYTAN-local-agent
```

The obsolete path:

```text
github.com/sheytan/local-agent
```

was removed from Go imports.

This eliminates mixed-module resolution and allows the repository to build against one consistent module identity.

---

# Dependency Maintenance

The TOML dependency was aligned with the supported release:

```text
github.com/BurntSushi/toml v1.6.0
```

The module file was tidied after dependency normalization.

---

# AI Context

Implemented:

```text
internal/aicontext/aicontext.go
internal/aicontext/AI-CONTEXT.md
```

Capabilities include:

```text
embedded AI operating instructions
materialized AI-CONTEXT.md
version markers
user-customizable context files
live environment briefing
OS/runtime information
CPU/RAM/GPU information
provider/model information
connectivity status
vision capability status
Continuum status
tool capability briefing
```

The AI context now supports:

```text
SystemMessage()
SystemMessageWithTools()
Briefing()
BriefingWithTools()
```

The compatibility API remains available, while the live runtime path uses the actual registered tool list.

---

# Tool Capability Exposure

The live AI context now derives available tools from the orchestrator registry.

This prevents a divergence between:

```text
tools advertised to the model
```

and:

```text
tools actually registered in the runtime
```

The configured allow-list is applied before the tool list is exposed.

Dynamic tools such as:

```text
coding_lab
research
future runtime tools
```

can therefore appear automatically without maintaining another hardcoded list.

---

# Configuration

The configuration layer now contains the Version Zeta runtime controls required for the Coding Lab and research stack.

Important controls include:

```text
LabEnabled
LabWorkspaceRoot
LabAllowNetwork
LabMaxIterations
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

The Lab iteration limit is bounded:

```text
default = 25
maximum = 100
```

The effective configuration falls back to the bounded default when no explicit Lab value is supplied.

---

# Performance Work

The project contains multiple performance-oriented mechanisms:

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

The orchestrator sorts tool definitions before building LLM requests.

This makes prompt construction deterministic and avoids unnecessary prompt-prefix churn caused by Go map iteration order.

---

# API and WebSocket Hardening

Implemented in:

```text
internal/api/server.go
internal/api/ws.go
```

The API layer now includes:

```text
per-session cancellation
WebSocket lifecycle hardening
origin checking
restricted CORS handling
configuration patching
configuration secret redaction
```

Remote API credentials are not returned through normal configuration GET responses.

Configuration updates are applied as patches against the current configuration rather than blindly replacing the complete stored configuration.

WebSocket origins are checked against the configured/local development policy.

---

# LLM / llama.cpp Hardening

Implemented:

```text
internal/llm/llama.go
```

The llama startup deadlock was removed.

Archive extraction now rejects unsafe ZIP paths including:

```text
absolute paths
parent-directory traversal
Windows volume paths
backslash traversal
```

This prevents archive entries from escaping the intended extraction directory.

---

# Sandbox

Implemented:

```text
internal/sandbox/sandbox.go
```

Sandbox execution now propagates the timeout context into the actual command execution path.

The timeout therefore applies to the running process rather than merely timing the caller.

---

# Filesystem Path Security

Implemented:

```text
internal/tools/basedir.go
```

The path layer now provides:

```text
BaseDir
SetBaseDir
ResolvePath
ResolvePathChecked
ResolvePathWithinBase
SafeJoin
SafeExistingPath
UnsafeAbsolutePath
```

Protection includes:

```text
empty/unset base handling
absolute path rejection
relative traversal rejection
tilde path handling
symlink-prefix escape detection
canonical workspace restriction
```

Tools can therefore operate relative to the application/workspace root without implicitly accepting arbitrary host paths.

---

# Fetch / SSRF Protection

Implemented:

```text
internal/tools/fetch.go
```

The fetch layer includes:

```text
bounded response bodies
bounded redirects
redirect re-validation
localhost blocking
loopback blocking
link-local blocking
private-network blocking
DNS resolution checks
request timeout
```

Default response size is bounded and the maximum configurable response remains capped.

The objective is to prevent a model-controlled fetch operation from being used to reach internal/private network resources.

---

# Shell / Git Jail

Implemented in:

```text
internal/tools/tools.go
```

The shell and Git tools now resolve working directories against the canonical base directory.

Git argument handling rejects escape mechanisms such as:

```text
-C
--git-dir
--work-tree
absolute external paths
path traversal
file:// repository targets
```

The shell uses a non-login shell invocation.

---

# Process Control

Implemented:

```text
internal/proc/proc.go
internal/proc/proc_other.go
internal/proc/proc_windows.go
```

Process execution now supports process-tree-aware cancellation.

Unix-like systems use process groups.

Windows uses:

```text
CREATE_NEW_PROCESS_GROUP
taskkill /PID /T /F
```

This prevents timed-out or canceled commands from leaving child processes running behind.

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
sanitize inherited environment
avoid stdin inheritance
```

stdout and stderr share one combined output budget.

The runner does not inherit the host environment wholesale.

Sensitive host environment values are removed before Lab execution.

---

# Lab Policy

Implemented:

```text
internal/lab/policy.go
```

The policy layer is tokenized rather than based only on naive substring matching.

Controls include:

```text
dangerous commands
interactive commands
network operations
filesystem escape
absolute paths
parent traversal
Git publishing
destructive Git operations
command-specific dangerous arguments
```

Allowed workspace Git workflows include legitimate operations such as commits.

Git publishing is blocked.

Destructive operations such as root-level removal are rejected.

The policy remains defense-in-depth and is not treated as a complete OS isolation boundary.

---

# Coding Lab Workspace

Implemented:

```text
internal/lab/workspace.go
```

The workspace manager provides:

```text
Create()
Remove()
PathFor()
Snapshot()
Promote()
```

Capabilities include:

```text
source validation
workspace isolation
cryptographically random workspace IDs
regular-file copying
.git preservation for promotion/snapshots
symbolic-link exclusion
path traversal protection
root-restricted cleanup
source/workspace overlap protection
context cancellation
```

The Lab workspace is intentionally isolated from the source tree during autonomous modification.

---

# Lab Task Lifecycle

Implemented:

```text
internal/lab/task.go
```

Task lifecycle:

```text
pending
running
succeeded
failed
canceled
blocked
```

Flow:

```text
NewTask
   ↓
Start
   ↓
RunCommand
   ↓
Verify
   ↓
Promote
   ↓
Finish
```

A task's runtime ID is aligned with the shared workspace ID when execution starts.

---

# Verification Gate

Implemented:

```text
internal/lab/verifier.go
```

Core invariant:

> A task cannot become successfully completed without a current passing verification.

Verification is invalidated whenever a new command mutates the workspace.

Therefore:

```text
VERIFY PASS
     │
     ▼
workspace mutation
     │
     ▼
verification stale
     │
     ▼
VERIFY AGAIN
```

The system rejects trivial commands as proof of correctness, including:

```text
echo
printf
true
false
exit 0
```

---

# Native Verification

The verifier can discover project checks automatically.

Supported project families include:

```text
Go
Node.js
Python
Rust
```

Examples include:

```text
go build ./...
go test ./...
npm test
npm run build
Python compile checks
pytest
cargo check
cargo test
```

Verification requires meaningful passing checks rather than accepting an arbitrary successful shell command.

---

# Lab Promotion

Implemented:

```text
export_patch
promote
```

The promotion flow now:

```text
workspace
   ↓
create snapshot
   ↓
verify current state
   ↓
mirror workspace into source
   ↓
preserve .git
```

A snapshot is created before source mutation.

The Lab workspace is not deleted immediately after promotion, which preserves the evidence and allows recovery/debugging.

The final task completion gate requires promotion evidence corresponding to the current verification.

---

# Lab Export Patch

The Lab can export a binary-safe Git patch comparing:

```text
source tree
```

against:

```text
Lab workspace
```

The patch is stored under the Lab root's patch directory.

---

# Autonomous Repair Controller

Implemented:

```text
internal/lab/repair.go
```

The bounded repair controller performs:

```text
baseline verification
diagnosis
bounded repair commands
re-verification
final verification
```

The controller is limited by:

```text
maximum repair iterations
```

with:

```text
default = 25
absolute cap = 100
```

The controller rejects empty repair actions.

Identical normalized commands are not retried forever.

A repeated identical action is rejected after the configured repetition threshold.

Every repair command runs through the normal Lab task/policy/runner controls.

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

Each session owns synchronization state so concurrent actions within the same Lab session do not race.

Registry operations avoid lock-order deadlocks with the per-session task mutex.

---

# Shared IDs

The Lab now uses one shared random identity across the task/workspace runtime.

The task ID becomes the workspace ID when execution begins.

This simplifies:

```text
task tracking
workspace lookup
patch naming
snapshot naming
repair bookkeeping
```

---

# Research Engine

Implemented:

```text
internal/research/provider.go
internal/research/service.go
internal/research/github.go
internal/research/reddit.go
internal/research/tool.go
```

The research layer exposes one unified service with multiple backends:

```text
Auto
GitHub
Reddit
Web
SearXNG
DuckDuckGo
```

The unified agent-facing interface is:

```text
research
```

The service supports:

```text
bounded result counts
bounded timeouts
provider validation
request normalization
result normalization
deduplication
provider ranking
authority ranking
```

---

# Research Provider Model

Research requests are normalized before execution.

Results preserve:

```text
title
URL
snippet
source
provider
published time
authority
match score
content hash
metadata
```

Results are deduplicated using:

```text
content hash
URL
normalized content
```

---

# Research Ranking

The research layer distinguishes technical authority from raw relevance.

Current authority classes include:

```text
official documentation
project/maintainer material
technical discussion
community material
unknown material
```

Ranking combines:

```text
relevance
authority
provider evidence
```

The ranking is advisory.

Local execution and verification remain the final authority.

---

# GitHub Research

Implemented:

```text
internal/research/github.go
```

The GitHub provider supports bounded issue/pull-request research using GitHub's search endpoint.

It is designed to surface:

```text
exact error messages
package/module names
symbols
regressions
maintainer explanations
known workarounds
```

Protection includes:

```text
4 MiB maximum response body
validated endpoint construction
scheme/host validation
safe path joining
normalized ranking scores
NaN/Inf score rejection
```

GitHub evidence is treated as research evidence and must be independently verified before being considered a confirmed fix.

---

# Reddit Research

Implemented:

```text
internal/research/reddit.go
```

Reddit research supports practical evidence such as:

```text
hardware-specific problems
installation issues
configuration problems
regressions
community workarounds
```

OAuth/token requirements and bounded response handling are enforced where applicable.

Reddit is treated as experiential evidence rather than authoritative documentation.

---

# Web Research Direction

The unified service has explicit support for:

```text
Web
SearXNG
DuckDuckGo
```

The architecture is prepared for free external research through configurable backends while retaining:

```text
timeouts
result limits
response caps
normalization
ranking
deduplication
```

Untrusted external content must remain evidence rather than being silently elevated to authoritative memory.

---

# Engineering Memory

Implemented:

```text
internal/memory/memory.go
```

Memory is now structured into seven classes:

```text
M1 = user facts
M2 = preferences
M3 = project state
M4 = decisions
M5 = procedures / learned fixes
M6 = conversation summaries
M7 = observations / untrusted or provisional knowledge
```

Every memory entry can carry:

```text
class
trust level
provenance
source
URI
reference
observation time
quarantine state
authority state
```

---

# Memory Trust Model

Trust levels include:

```text
unknown
untrusted
provisional
trusted
verified
```

M1 is restricted to explicit trusted/verified user-originated facts.

External research is automatically treated as:

```text
M7
provisional
quarantined
non-authoritative
```

This prevents material copied from the public web from silently becoming a persistent user fact.

---

# Memory Recall

Normal recall excludes quarantined entries.

Memory results expose:

```text
class
trust
authoritative status
source
provenance
quarantine status
```

This makes recalled information distinguishable from verified system facts.

Persistent conversation recall remains available separately from curated memory.

---

# Memory Compatibility

The memory store retains compatibility with the existing:

```text
Append
All
Search
DeleteByID
Clear
Count
```

interfaces.

The newer structured path is:

```text
AppendEntry
SearchWithOptions
NormalizeEntry
```

---

# AI Context + Memory Boundary

The AI context and memory architecture now enforce two separate principles:

```text
AI context = operational instructions
memory     = evidence with provenance
```

A retrieved memory item does not automatically become a trusted system instruction.

An external research result does not automatically become an authoritative memory fact.

---

# Recall Infrastructure

Persistent recall continues to index completed conversations as bounded capsules.

The orchestrator can inject relevant previous exchanges into the current prompt while respecting:

```text
recall limits
history limits
context budget
```

This allows long-running engineering tasks to reuse previous work without replaying the complete conversation every turn.

---

# Runtime Integration

Implemented:

```text
internal/runtime/runtime.go
```

The runtime stack wires:

```text
LLM
orchestrator
memory
recall
Coding Lab
research
sandbox
browser
filesystem tools
shell
Git
Linux simulation
vision
```

The runtime registers Coding Lab when enabled.

The runtime also registers the unified Research tool when research is enabled.

---

# Current API/Runtime Boundary

The architecture still contains an important integration gap:

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
separate runtime surface
```

The next major backend integration task is:

```text
unify API server with runtime.Stack
```

The goal is for:

```text
CLI
desktop
React UI
HTTP API
WebSocket
```

to operate against one authoritative runtime/tool registry.

---

# UI / UX Migration

## Current State

The application currently includes:

```text
Go/Fyne native UI
Go HTTP API
WebSocket activity stream
embedded/static web interface
```

## Target State

The UI is being moved toward:

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

Target architecture:

```text
             React / TypeScript
                     │
              REST + WebSocket
                     │
                     ▼
                  Go API
                     │
                     ▼
                Runtime Stack
                     │
        ┌────────────┼────────────┐
        ▼            ▼            ▼
       LLM          LAB        RESEARCH
```

The Go backend remains authoritative for execution and system access.

---

# UI Capability Parity

The React interface must expose the capabilities already present in Go rather than inventing unsupported backend behavior.

The intended UI surface includes:

```text
chat
streaming activity
session management
tool activity
Coding Lab
research
memory
configuration
diagnostics
verification
artifacts
```

---

# SVG Workspace

The SVG editor remains a planned first-class workspace feature.

Target:

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
human inspection
     ↓
source edit
     ↓
instant preview
     ↓
verification
```

This remains downstream of the backend stabilization work.

---

# Documentation Accuracy

Documentation must reflect actual capabilities.

The project should not claim:

```text
implemented
```

for components that only exist as architectural plans.

Likewise, completed components must no longer be described as merely planned.

The worklog itself is maintained against the verified repository state.

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

Defense-in-depth includes:

```text
path restriction
SSRF protection
process-tree cancellation
environment sanitization
stdin isolation
shell restriction
Git restriction
archive path validation
workspace isolation
verification gates
promotion snapshots
memory quarantine
```

Future strengthening may add:

```text
stronger OS isolation
resource quotas
network namespaces/isolation
package-install controls
process limits
workspace quotas
artifact policies
```

---

# Batch Repair Status

## Batch A — Build Foundation

```text
✓ P0-1 unified Go module path
✓ P0-2 TOML dependency aligned to v1.6.0
✓ P0-8 numeric version + codename separation
✓ runtime Go version reporting
□ P2-2 .prettierignore
□ P2-3 releasegate coverage expansion
```

## Batch B — Runtime Safety

```text
✓ P0-6 WebSocket race/drop handling
✓ P0-7 camelCase LLM JSON tags
✓ P0-9 per-session abort
✓ P0-16 CORS/WS origin restrictions
✓ configuration patch semantics
✓ API secret redaction
✓ P1-11 full-session UI fetch behavior
```

## Batch C — Execution Security

```text
✓ P0-10 llama startup deadlock
✓ P0-11 sandbox timeout propagation
✓ P0-12 ZIP slip protection
✓ P0-13 filesystem/shell/Git/fetch jail
✓ SSRF protection
✓ P1-2 tokenized Lab policy
✓ sanitized Lab environment
✓ process-group cancellation
```

## Batch D — Coding Lab

```text
✓ P0-14 independent native verification
✓ trivial verification rejection
✓ automatic project-check discovery
✓ P0-15 promotion snapshots
✓ promotion without immediate Lab deletion
✓ P1-1 bounded autonomous repair controller
✓ P1-3 shared task/workspace ID
✓ P1-4 per-session mutex
✓ P1-5 required checks default
```

## Batch E — Research / Memory / Context

```text
✓ research request/result normalization
✓ backend abstraction
✓ GitHub provider
✓ Reddit provider
✓ bounded research responses
✓ authority/relevance ranking
✓ research deduplication
✓ M1-M7 memory classes
✓ provenance
✓ trust levels
✓ untrusted-web quarantine
✓ non-authoritative normal recall
✓ dynamic AI-context tool exposure
□ deeper research-source validation
□ cache architecture refinement
□ full engineering-memory integration
□ operational AI-CONTEXT constitution expansion
□ critic/research strict-failure semantics
```

## Batch F — UI

```text
□ React UI parity
□ Lab panel
□ Research panel
□ memory panel
□ runtime/session integration
□ SVG editor
```

## Batch G — Documentation

```text
□ final capability audit
□ README synchronization
□ worklog synchronization
□ remove stale planned-state claims
```

---

# Current Immediate Sequence

The active engineering sequence is now:

```text
1. finish Batch-E backend semantics
        ↓
2. unify HTTP/API runtime with shared Stack
        ↓
3. complete research/cache/evidence hardening
        ↓
4. connect engineering memory to verified fixes
        ↓
5. complete React/TypeScript/Vite UI parity
        ↓
6. implement SVG workspace
        ↓
7. run complete end-to-end autonomous coding tests
        ↓
8. synchronize README/documentation with verified capabilities
```

---

# Verification Commands

The acceptance sequence remains:

```bash
grep -R 'github.com/sheytan/local-agent' --include='*.go'
go mod tidy
go test ./internal/... -tags headless -count=1
go test ./internal/lab/ -count=1
go test ./internal/research/ -count=1
go vet ./internal/...
go build -o /tmp/sheytan .
npm run typecheck
```

The desired result is:

```text
no obsolete module imports
clean module state
passing internal tests
passing Lab tests
passing research tests
clean go vet
successful Go build
successful frontend typecheck
```

---

# Critical Invariants

The following invariants define the repaired architecture.

```text
A model cannot declare its own code correct.

A successful command is not automatically verification.

A workspace mutation invalidates previous verification.

Promotion requires current verification.

Finish requires current promotion evidence.

Lab commands operate inside the allowed workspace.

Git push is blocked by Lab policy.

Absolute external working directories are rejected.

Traversal outside the configured base is rejected.

Loopback/private-network fetches are blocked.

Timed-out process trees are terminated.

Lab commands do not inherit host secrets.

Untrusted web material cannot silently become M1 memory.

Quarantined memory is excluded from ordinary recall.

The AI context advertises the actual registered toolset.

Per-session abort does not cancel unrelated sessions.

Tool ordering is deterministic.

Snapshots are created before promotion mutates source.

```

---

# Core Architecture Principle

The defining Version Zeta principle remains:

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

And the long-term autonomous coding loop is:

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
    ├── PASS → REVIEW → PROMOTE → COMPLETE
    │
    └── FAIL → DIAGNOSE → RESEARCH → EDIT
```

The system is therefore designed around one rule:

> **SHEYTAN-Local-Agent must produce evidence, not merely confidence.**
