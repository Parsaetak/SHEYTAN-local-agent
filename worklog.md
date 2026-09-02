# SHEYTAN-Local-Agent — Worklog

## 2026-09-02 — Version Zeta Stabilization

### Project direction

The repository is being consolidated into a local-first AI software-engineering runtime built around:

```text
Go runtime
+ controlled tools
+ Coding Lab
+ objective verification
+ bounded repair
+ external research
+ engineering memory
+ React frontend
```

The governing principle is:

> **The language model proposes. The system verifies.**

---

# Runtime Identity

The application identity is now separated into:

```go
const (
    AppName     = "SHEYTAN-Local-Agent"
    AppVersion  = "1.1.0"
    AppCodename = "Zeta"
)
```

The runtime reports the actual Go runtime version instead of a hardcoded Go version.

---

# Repository / Module Normalization

The canonical Go module path is:

```text
github.com/Parsaetak/SHEYTAN-local-agent
```

The obsolete module path:

```text
github.com/sheytan/local-agent
```

was removed from Go imports.

The TOML dependency was aligned to:

```text
github.com/BurntSushi/toml v1.6.0
```

The module was tidied after the dependency correction.

---

# Agent / AI Context

The AI context system was upgraded so runtime instructions can use the actual registered tool set.

Implemented:

```text
SystemMessage()
SystemMessageWithTools()
Briefing()
BriefingWithTools()
```

The runtime now derives exposed tool capabilities from the actual orchestrator registry.

This prevents:

```text
advertised tools
```

from drifting away from:

```text
registered tools
```

The operational AI constitution is maintained in:

```text
internal/aicontext/AI-CONTEXT.md
```

The constitution defines:

```text
truth/evidence rules
tool-use rules
research rules
memory rules
verification rules
safety rules
failure handling
final-answer discipline
```

---

# Configuration

Version Zeta configuration includes Coding Lab and research controls.

Important settings:

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

Lab iteration bounds are:

```text
default = 25
maximum = 100
```

The effective configuration is bounded even when no explicit value is supplied.

---

# API Hardening

The HTTP API was hardened around:

```text
per-session abort
WebSocket lifecycle
origin validation
restricted CORS
configuration patching
configuration secret redaction
```

Configuration GET responses do not expose sensitive remote API credentials.

Configuration PUT/POST updates merge patches into the current configuration so unrelated fields are preserved.

WebSocket origins are checked against the local/same-origin policy.

Activity connections are session-specific.

---

# LLM Runtime Hardening

The LLM client now rejects successful HTTP responses that contain no usable choices.

An empty choices array is treated as an error.

This prevents silent acceptance of structurally invalid LLM responses.

---

# llama.cpp Hardening

The llama startup deadlock was removed.

Archive extraction now rejects unsafe ZIP entries involving:

```text
absolute paths
parent traversal
Windows volume paths
backslash traversal
```

This prevents extracted files from escaping the target directory.

---

# Sandbox Timeout

Sandbox execution now propagates timeout context into the actual command execution path.

The timeout therefore applies to the running process instead of only measuring the caller.

---

# Filesystem Security

The base-directory layer now provides checked path resolution.

Important primitives include:

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

Protection covers:

```text
absolute paths
relative traversal
unset base directories
tilde expansion hazards
symlink-prefix escapes
workspace restrictions
```

---

# Fetch / SSRF Protection

The controlled fetch path now contains:

```text
bounded response bodies
bounded redirects
redirect re-validation
localhost blocking
loopback blocking
link-local blocking
private-network blocking
DNS resolution validation
request timeout
```

Default and maximum response limits are bounded.

The objective is to prevent model-controlled HTTP access from reaching internal/private services.

---

# Shell and Git Policy

Shell and Git working directories are checked against the canonical base directory.

Git escape mechanisms are blocked, including:

```text
-C
--git-dir
--work-tree
absolute external paths
parent traversal
file:// repository targets
```

Legitimate workspace-local Git workflows such as committing remain possible.

Git publishing is blocked.

Destructive command patterns are rejected through the Lab policy layer.

The policy is defense in depth and is not considered a substitute for OS-level sandboxing.

---

# Process Control

Process execution now supports process-tree cancellation.

Unix-like systems use process groups.

Windows uses:

```text
CREATE_NEW_PROCESS_GROUP
taskkill /PID /T /F
```

This is used to prevent timed-out or canceled commands from leaving child processes running.

---

# Coding Lab Runner

The Lab runner now handles:

```text
working-directory validation
stdout capture
stderr capture
combined output limits
exit status
duration
timeouts
context cancellation
stdin isolation
environment sanitization
process-tree cleanup
```

The runner does not inherit the host environment wholesale.

Sensitive environment values are removed before Lab execution.

---

# Coding Lab Policy

The Lab policy is token-aware rather than relying exclusively on naive substring matching.

It controls:

```text
dangerous commands
interactive commands
network operations
absolute paths
path traversal
Git publishing
destructive Git operations
workspace escapes
dangerous arguments
```

The security principle is:

```text
allow normal engineering work
block obvious escape/destructive paths
```

while still relying on the wider runtime for defense in depth.

---

# Coding Lab Workspace

The Lab workspace manager now supports:

```text
Create
Remove
PathFor
Snapshot
Promote
```

The workspace layer provides:

```text
source validation
workspace isolation
random workspace IDs
safe regular-file copying
.git handling
symlink exclusion
path traversal protection
root-restricted cleanup
source/workspace overlap checks
```

The workspace remains disposable and isolated while autonomous modifications are being evaluated.

---

# Task Lifecycle

Lab tasks use:

```text
pending
running
succeeded
failed
canceled
blocked
```

Typical flow:

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

Task and workspace runtime IDs are aligned.

---

# Verification Gate

The core Lab rule is:

> **A successful task requires current passing verification.**

Verification is invalidated whenever a new command mutates the workspace.

The system explicitly rejects trivial proof commands such as:

```text
echo
printf
true
false
exit 0
```

The verifier instead seeks meaningful project checks.

---

# Native Verification

Automatic verification discovery currently covers common project families:

```text
Go
Node.js
Python
Rust
```

Representative checks include:

```text
go build ./...
go test ./...
npm test
npm run build
Python compilation checks
pytest
cargo check
cargo test
```

The verifier does not accept arbitrary successful commands as proof.

---

# Promotion / Snapshot Safety

Lab promotion now follows:

```text
workspace
   ↓
snapshot
   ↓
current verification
   ↓
promote
   ↓
source
```

A snapshot is created before source mutation.

The Lab workspace is preserved after promotion rather than immediately deleted.

This keeps evidence available for:

```text
recovery
debugging
comparison
post-failure investigation
```

---

# Patch Export

The Lab can export a binary-safe Git patch representing the difference between:

```text
source tree
```

and:

```text
Lab workspace
```

Patch artifacts are stored beneath the Lab root.

---

# Autonomous Repair

The Lab contains a bounded repair controller.

The controller performs:

```text
baseline verification
diagnosis
bounded repair action
re-verification
final verification
```

Iteration policy:

```text
default = 25
maximum = 100
```

The controller rejects empty repair actions.

Identical normalized actions are not allowed to repeat forever.

All repair commands still pass through the normal:

```text
policy
runner
workspace
verification
```

layers.

---

# Lab Session Registry

The Lab session registry provides:

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

Each Lab session owns synchronization state.

Registry locking and per-session task locking are structured to avoid lock-order deadlocks.

---

# Research Engine

The research service was consolidated into a unified provider architecture.

Current providers:

```text
Auto
GitHub
Reddit
SearXNG
DuckDuckGo
Web compatibility alias
```

The agent-facing tool is:

```text
research
```

Research requests and results are normalized.

The service supports:

```text
result limits
timeouts
normalization
deduplication
authority classification
relevance ranking
content hashes
provider metadata
```

The `web` backend is explicitly treated as a compatibility alias for DuckDuckGo rather than as an unrestricted generic web provider.

---

# Research Result Model

Research results retain fields such as:

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

Deduplication can use:

```text
content hash
normalized URL
normalized content
```

This reduces repeated evidence from multiple providers.

---

# Research Ranking

The research layer distinguishes:

```text
relevance
```

from:

```text
authority
```

Authority classes include concepts such as:

```text
official documentation
project / maintainer material
technical discussion
community material
unknown material
```

Ranking is advisory.

A highly ranked result is still only evidence.

The local project and executable verification remain the final acceptance mechanism.

---

# GitHub Research

The GitHub provider is bounded and validates endpoint construction.

Its purpose is to locate engineering evidence such as:

```text
exact errors
issues
pull requests
regressions
symbols
module names
maintainer explanations
workarounds
```

Response bodies are bounded.

GitHub content remains research evidence rather than automatically trusted memory.

---

# Reddit Research

The Reddit provider is designed for practical/community evidence:

```text
installation failures
hardware issues
configuration problems
regressions
community workarounds
```

OAuth/token requirements are enforced where needed.

Reddit remains lower-authority experiential evidence.

---

# Web Research

The web layer currently supports:

```text
DuckDuckGo
SearXNG
```

with:

```text
bounded timeouts
bounded results
bounded bodies
normalization
ranking
deduplication
```

No external result should be promoted to authoritative engineering knowledge without appropriate evidence.

---

# Research Cache

Research caching was added with bounded storage.

The cache uses:

```text
TTL
bounded entry count
normalized request keys
copy-on-read/write behavior
```

Only successful non-empty research results are cached.

This avoids caching provider failures as if they were useful evidence.

---

# Engineering Memory

Memory now has explicit classes:

```text
M1 = trusted user facts
M2 = preferences
M3 = project state
M4 = decisions
M5 = procedures / learned fixes
M6 = conversation summaries
M7 = observations / untrusted or provisional knowledge
```

Entries can retain:

```text
trust level
provenance
source
URI/reference information
```

External provenance is automatically quarantined.

External research cannot silently become trusted M1–M6 knowledge merely because a caller supplied a trusted-looking label.

---

# Memory Trust Model

The memory system distinguishes:

```text
trusted user information
verified local information
generated/provisional information
external research
```

External sources such as:

```text
web
research
GitHub
Reddit
```

are quarantined as provisional knowledge by default.

Default recall excludes quarantined entries.

Explicit requests can include them when appropriate.

---

# Multi-Agent / Critic Behavior

The critic path now treats malformed or failed structured responses as unsatisfied rather than accidentally passing them.

Planner fallback behavior remains graceful where possible.

The runtime therefore follows:

```text
valid evidence
+
valid structured response
=
accepted state
```

rather than treating parser failure as success.

---

# Frontend Migration

The frontend is being migrated toward:

```text
Node.js
TypeScript
React
Vite
Zustand
REST
WebSocket
```

The current React shell already integrates with the actual Go API.

Current frontend/runtime integration includes:

```text
application state
system information
models
presets
tools
sessions
session creation
session deletion
active session selection
agent runs
abort
activity WebSocket
runtime status
```

The React activity connection is session-aware.

Initialization now loads the session state first and connects to the active session afterward.

---

# Frontend Next Stage

The next frontend stage is:

```text
React UI parity
      ↓
Coding Lab panel
      ↓
Research panel
      ↓
project/task inspection
      ↓
artifact workflows
```

The SVG editor/preview is intentionally later.

The frontend should not duplicate Go execution logic.

---

# Documentation Corrections

The previous documentation contained several stale statements describing already-implemented systems as planned.

The documentation is being normalized around four states:

```text
Implemented
In progress
Planned
Experimental
```

This worklog records actual implementation state rather than aspirational architecture.

---

# Developer Operating Rules

Developers working on the repository should:

```text
inspect current code before editing
preserve API contracts
preserve security boundaries
keep execution logic in Go
keep UI concerns in React
test changed behavior
avoid unnecessary rewrites
prefer bounded behavior
document actual capabilities
```

When changing one subsystem, check neighboring contracts.

Examples:

```text
API change
→ TypeScript API types
→ Zustand store
→ UI consumers

Lab change
→ policy
→ runner
→ task
→ verifier
→ promotion
→ tests

Research change
→ provider
→ normalization
→ ranking
→ deduplication
→ memory provenance
→ tests
```

---

# Coding-Agent Operating Rules

Agents operating on this repository should treat the repository itself as authoritative.

Required behavior:

```text
1. Inspect before modifying.
2. Do not assume a capability exists.
3. Preserve security invariants.
4. Treat research as evidence.
5. Never fake verification.
6. Re-verify after mutation.
7. Bound retries, loops, output, and network access.
8. Keep API field names synchronized.
9. Keep memory provenance intact.
10. Keep documentation truthful.
```

Before making architectural changes, read:

```text
README.md
worklog.md
internal/aicontext/AI-CONTEXT.md
```

When modifying an existing file, inspect its current live contents rather than relying on an old copy.

When a task claims to be complete, verify the actual repository state and the behavior relevant to the change.

---

# Verification Discipline

The intended Go acceptance baseline is:

```bash
go mod tidy
go test ./internal/... -tags headless -count=1
go test ./internal/lab/ -count=1
go test ./internal/research/ -count=1
go vet ./internal/...
go build -o /tmp/sheytan .
```

Frontend acceptance includes:

```bash
npm run typecheck
npm run lint
npm run build:web
```

The integrated build is:

```bash
npm run build
```

The exact relevant checks should depend on what changed.

A passing command is not proof unless the command actually tests the behavior under change.

---

# Current State — 2026-09-02

## Stable implemented foundations

```text
✓ Go runtime architecture
✓ canonical module path
✓ version/codename separation
✓ LLM client
✓ llama runtime hardening
✓ orchestrator
✓ dynamic tool exposure
✓ AI context
✓ persistent sessions
✓ memory and recall
✓ Coding Lab
✓ Lab isolation
✓ Lab policy
✓ Lab runner
✓ process control
✓ path security
✓ fetch/SSRF protection
✓ verification gate
✓ native verification
✓ snapshots
✓ promotion
✓ patch export
✓ bounded repair
✓ GitHub research
✓ Reddit research
✓ DuckDuckGo research
✓ SearXNG research
✓ research ranking
✓ research deduplication
✓ research cache
✓ API hardening
✓ WebSocket security
✓ React/Vite frontend foundation
```

## Active work

```text
□ complete React feature parity
□ Coding Lab React panel
□ Research React panel
□ richer task/project inspection
□ frontend integration testing
□ broader end-to-end autonomous workflows
□ SVG workspace/editor
```

---

# Working Principle

The system is intentionally built around:

```text
MODEL
  +
REASONING
  +
TOOLS
  +
MEMORY
  +
PLANNING
  +
VERIFICATION
  +
RUNTIME
  +
COMPUTE
```

A stronger model improves the reasoning component.

It does not remove the need for:

```text
real project state
controlled execution
persistent evidence
verification
security boundaries
bounded autonomy
```

The architectural target remains:

> **An AI software engineer whose important claims are backed by executable evidence.**
