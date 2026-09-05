````markdown
# SHEYTAN™ Local-Agent

> **A local-first AI software-engineering laboratory.**
>
> The model proposes. The tools execute. The laboratory verifies.

SHEYTAN™ Local-Agent is a local-first desktop AI engineering environment built around Go, React/TypeScript, Wails, local LLM inference, controlled tools, isolated coding workspaces, research, memory, recall, and objective verification.

**SHEYTAN™ is a trademark of Parsaetak · © 2024–2026 Parsaetak. All rights reserved.**

Licensed under the **Parsaetak Proprietary License v1.1**.

---

# Project Identity

```text
Application:      SHEYTAN-Local-Agent
Current release:  v1.1.2Z
Codename:         Zeta
Current branch:   main
Development:      v1.1.3Z
````

The repository currently contains the desktop application shell and a substantial runtime foundation. The current development phase is focused on making the application **actually functional end-to-end**, especially the local inference path and long-context agent workflow.

The central execution invariant is:

```text
LLM proposal
     ↓
tool execution
     ↓
objective evidence
     ↓
verification
     ↓
accept / reject
```

The model is never the authority on whether an engineering task succeeded.

---

# Current State

Status after the v1.1.3Z AAA upgrade (2026-09-04). The application is no
longer a shell around missing runtime: the full acceptance chain is
implemented and was executed against a REAL llama.cpp engine (b10642) with
a real Qwen2.5 model.

Evidence-based status (VERIFIED / PARTIALLY VERIFIED / NOT VERIFIED):

```text
automatic llama.cpp startup (launch -> healthy)   VERIFIED  (real engine + stub, e2e)
model resolve / load / health / ready states       VERIFIED  (real engine, e2e + unit)
first real inference + streaming + persistence     VERIFIED  (real Qwen2.5, e2e)
engine states (idle/starting/ready/busy/failed...) VERIFIED  (unit + e2e)
bounded auto-restart after engine death            VERIFIED  (unit, crash test)
tool loop (call -> registry -> exec -> follow-up)  VERIFIED  (deterministic tests)
tool schemas advertised to engine                  VERIFIED  (e2e envelope)
attachments (upload/stage/chunk/retrieve)          VERIFIED  (unit + e2e)
context cache (hit/miss/version/bounds/concurrent) VERIFIED  (unit)
long-context plan + windowing + overflow warning   VERIFIED  (unit + real-engine finding)
sessions (CRUD, concurrency, sidecar bounds)       VERIFIED  (unit + e2e)
regenerate                                         VERIFIED  (e2e)
frontend build (typecheck/lint/vite/embed)         VERIFIED  (npm run build)
conversation history view + composer running fix   VERIFIED  (code + typecheck; manual UI NOT VERIFIED)
desktop (Wails/GTK) build                          NOT VERIFIED in sandbox (no GTK dev libs; CI builds it)
vision pipeline with real mmproj                   NOT VERIFIED (no mmproj model tested)
```

Key product finding from real-engine testing: with the FULL tool registry,
the AI-context briefing + tool schemas cost ~9.8k tokens, so the previous
default 8k context window could not fit even the FIRST request. The default
numCtx is now 16384 and the context plan reports overflow visibly instead of
letting the engine fail silently.

The remaining priority is keeping that full path verified while polishing UX.

The priority is:

```text
START
  ↓
ENGINE READY
  ↓
MODEL READY
  ↓
MESSAGE SENT
  ↓
LLM RESPONSE
  ↓
TOOLS EXECUTE
  ↓
FILES / ATTACHMENTS
  ↓
LONG CONTEXT
  ↓
CACHE / RECALL
  ↓
VERIFICATION
  ↓
VISIBLE RESULT
```

A feature is considered complete only when that full path works in the real desktop application.

---

# Development Priorities

The immediate development order is:

```text
1. Automatic llama.cpp lifecycle
2. Reliable model discovery / selection / startup
3. End-to-end agent inference
4. Real attachment ingestion
5. Context chunking
6. Context caching
7. Long-context retrieval / recall
8. Tool execution reliability
9. Error propagation and cancellation
10. Functional integration testing
11. Performance optimization
12. UX refinement
```

Do not reverse this order merely because a UI task is easier.

---

# Architecture

```text
┌─────────────────────────────────────────────┐
│              React / TypeScript             │
│                  Vite UI                    │
└──────────────────────┬──────────────────────┘
                       │
                 REST + WebSocket
                       │
                       ▼
┌─────────────────────────────────────────────┐
│                   Go API                    │
└──────────────────────┬──────────────────────┘
                       │
                       ▼
┌─────────────────────────────────────────────┐
│                 Go Runtime                  │
│                                             │
│ agent / orchestrator                       │
│ LLM / llama.cpp                             │
│ tools                                       │
│ context / chunking                          │
│ cache / recall                              │
│ memory                                      │
│ research                                    │
│ Coding Lab                                  │
│ sandbox                                     │
│ filesystem                                  │
│ browser                                     │
│ vision                                      │
│ sessions                                    │
│ process control                             │
│ diagnostics                                │
└──────────────────────┬──────────────────────┘
                       │
                       ▼
┌─────────────────────────────────────────────┐
│             llama.cpp server                │
│          local model inference              │
└─────────────────────────────────────────────┘
```

Critical execution logic belongs in Go.

Presentation and interaction logic belongs in React.

---

# Desktop Application

The production desktop application uses the Wails v3 shell.

```text
sheytan-local-agent.exe
├── embedded React/Vite assets
├── Go runtime
├── Go HTTP API
├── WebSocket activity
└── Wails desktop shell
```

The production application does not require a separately running frontend development server.

Frontend source:

```text
src/
```

Embedded production assets:

```text
web/static/
```

Build synchronization:

```text
npm run build
      ↓
Vite dist/
      ↓
scripts/sync-web.mjs
      ↓
web/static/
      ↓
Go embed
```

Whenever frontend source changes, `web/static` must be rebuilt and synchronized.

---

# Repository Structure

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
├── desktop/
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

src/
scripts/
web/
```

Important LLM files currently include:

```text
internal/llm/client.go
internal/llm/gguf.go
internal/llm/llama.go
internal/llm/models.go
internal/llm/presets.go
internal/llm/speed.go
```

Context infrastructure includes:

```text
internal/chunking/chunking.go
internal/chunking/chunking_test.go
```

---

# Local llama.cpp Engine

The local llama.cpp server is a core dependency of the application.

The application must treat llama.cpp as an **application-managed runtime service**, not as an optional manually launched dependency.

The required behavior is:

```text
Application start
      ↓
initialize config
      ↓
discover / resolve model
      ↓
ensure llama.cpp binary
      ↓
start llama.cpp automatically
      ↓
wait for actual readiness
      ↓
verify HTTP health
      ↓
mark engine ready
      ↓
enable agent inference
```

The application must not require the user to manually launch llama.cpp for normal operation.

## Required engine behavior

On application startup:

```text
missing binary
    ↓
download automatically when network is available
    ↓
install locally
    ↓
start
```

or:

```text
binary already installed
    ↓
start automatically
```

The system must also:

```text
detect dead engine
restart when appropriate
detect startup failure
surface the actual error
track active model
track PID where applicable
expose health
expose lifecycle state
support stop
support restart
```

Valid engine states include:

```text
idle
starting
downloading
ready
running
busy
stopping
stopped
failed
```

Never report `running` or `ready` based solely on an optimistic frontend state.

The state must come from actual engine/process/HTTP health.

---

# Model Lifecycle

The model lifecycle should be:

```text
discover
   ↓
inspect
   ↓
select
   ↓
resolve path
   ↓
start llama.cpp
   ↓
load model
   ↓
verify health
   ↓
use
```

GGUF discovery must remain bounded and responsive.

Model information should include, when available:

```text
id
name
path
size
format
architecture
quantization
context capacity
status
loaded state
```

The active model shown by the UI must be the model actually used by the runtime.

---

# Agent Inference

The primary application workflow is:

```text
User message
    ↓
Session state
    ↓
Context assembly
    ↓
Cached context reuse
    ↓
Chunk retrieval
    ↓
LLM request
    ↓
Streaming response
    ↓
Tool call(s)
    ↓
Tool result(s)
    ↓
Follow-up inference
    ↓
Final answer
```

The Agent must support:

```text
conversation
streaming
message history
tool calls
tool results
errors
cancellation
retry
regeneration
model awareness
runtime awareness
attachments
long context
context provenance
```

An inference request that silently fails, produces no usable choice, or leaves the UI waiting indefinitely is an error.

---

# Attachments

Attachments are part of the Agent workflow, not a cosmetic UI feature.

The target workflow is:

```text
select file
   ↓
validate
   ↓
stage safely
   ↓
inspect metadata
   ↓
extract usable content
   ↓
chunk if necessary
   ↓
cache processed representation
   ↓
attach to context
   ↓
send to model
```

Required attachment capabilities:

```text
attach
inspect
stage
remove
reuse
show in conversation
```

The runtime must never blindly inject arbitrary binary data into a prompt.

Attachments must have bounded:

```text
file size
extraction size
processing time
chunk count
memory use
```

The implementation must distinguish between:

```text
original file
staged file
extracted text
metadata
chunks
cached representation
```

These are not interchangeable objects.

---

# Context Chunking

Long-context support must be based on structured chunking rather than concatenating every available input into a single prompt.

The chunking pipeline should be:

```text
raw input
   ↓
normalize
   ↓
segment
   ↓
chunk
   ↓
metadata
   ↓
hash
   ↓
cache
   ↓
retrieve required chunks
   ↓
assemble bounded context
```

Chunk metadata should be stable enough to support reuse:

```text
chunk ID
source ID
source type
source path
content hash
sequence
character / byte range
token estimate
created time
version
```

Chunking must preserve semantic boundaries where practical.

Preferred boundaries include:

```text
document sections
paragraphs
code blocks
function / class boundaries
headings
sentences
```

Do not split structured code or documents arbitrarily when a better boundary is available.

---

# Context Caching

Context caching is a first-class runtime requirement.

The system should avoid recomputing identical context repeatedly.

Cacheable stages include:

```text
file extraction
document normalization
chunking
chunk metadata
content hashing
retrieval results
context assembly
```

Cache keys must be deterministic and content-sensitive.

Recommended key structure:

```text
source identity
+
content hash
+
processing version
+
chunking configuration
+
relevant runtime configuration
```

A stale cache entry must never be treated as current merely because its path is unchanged.

Content hash or equivalent invalidation must be used.

The cache should be:

```text
bounded
versioned
recoverable
observable
invalidatable
safe under concurrent access
```

---

# Long-Context Strategy

The application should not attempt to place every historical message into every prompt.

Preferred strategy:

```text
recent conversation
        +
relevant recalled context
        +
retrieved attachment/document chunks
        +
high-value memory
        +
current task state
        +
tool evidence
```

The system should maintain a context budget.

When the budget is exceeded:

```text
compress
retrieve
prune low-value history
retain important state
reassemble
```

Do not solve context pressure by blindly increasing the prompt size.

---

# Memory and Recall

Memory and context are related but distinct.

Memory stores durable information.

Recall selects useful information for a specific task.

The conceptual pipeline is:

```text
memory
   ↓
recall candidates
   ↓
relevance filtering
   ↓
context budget
   ↓
prompt assembly
```

Memory classes include:

```text
M1 trusted user facts
M2 preferences
M3 project state
M4 decisions
M5 learned procedures
M6 conversation summaries
M7 observations / provisional knowledge
```

External research must retain provenance and must not silently become trusted memory.

---

# Tool Architecture

Tools implement the runtime tool contract and must be exposed according to actual availability.

Conceptually:

```go
type Tool interface {
    Name() string
    Description() string
    Parameters() any
    Run(ctx context.Context, args json.RawMessage) (string, error)
}
```

Useful tool categories include:

```text
filesystem
attachments
shell
git
browser
research
vision
memory
recall
sandbox
process
diagnostics
```

The model should never be told a tool exists unless that tool is actually registered and usable.

---

# Coding Lab

Coding Lab is the engineering execution environment.

```text
project
  ↓
isolated workspace
  ↓
inspect
  ↓
edit
  ↓
run
  ↓
verify
  ↓
review
  ↓
promote / discard
```

The Lab must preserve:

```text
workspace isolation
path validation
symlink protections
shell policy
Git policy
network policy
timeouts
cancellation
bounded output
process-tree cleanup
environment sanitization
```

A task is not successful merely because a command exits with status 0.

Objective verification is required.

---

# Verification

Verification remains authoritative.

```text
EDIT / RUN
     ↓
verification stale
     ↓
VERIFY
     ↓
PASS ──► continue
FAIL ──► diagnose / repair
```

Examples such as:

```text
echo
printf
true
exit 0
```

are not meaningful proof of project correctness.

Meaningful verification should prefer project-level checks such as:

```text
Go tests
Node tests
TypeScript checks
builds
linting
Rust tests
Python tests
functional integration checks
desktop smoke tests
```

---

# Research

Research is an evidence source.

Supported architecture may include:

```text
Auto
GitHub
Reddit
DuckDuckGo
SearXNG
Web compatibility alias
```

Research results should carry:

```text
title
provider
URL
snippet
relevance
authority
published information when available
```

Research is never automatically authoritative.

The final engineering proof remains:

```text
local state
+
actual execution
+
objective verification
```

---

# Sessions

Sessions are persistent units of work.

Required behavior:

```text
create
rename
switch
delete
restore history
active model
activity
metadata
```

Session state must remain synchronized between frontend and backend.

Long-running sessions must not produce unbounded in-memory history.

---

# Browser and Vision

Browser and vision are runtime capabilities, not decorative panels.

Browser state should expose:

```text
availability
task
navigation
URL
page title
screenshots
errors
cancellation
```

Vision should expose:

```text
input
processing state
result
errors
```

Both must preserve existing security controls.

---

# Security

The LLM is an untrusted proposal source.

Runtime policy remains authoritative.

Do not weaken:

```text
workspace isolation
filesystem boundaries
path validation
symlink protection
SSRF controls
DNS validation
redirect validation
shell policy
Git policy
sandbox policy
process cancellation
secret redaction
safe archive extraction
```

Long-running operations require:

```text
timeout
cancellation
bounded resources
cleanup
```

---

# Performance

The application targets smooth interaction on high-refresh displays, including 120 Hz environments.

Performance priorities are:

```text
input latency
scrolling
workspace switching
stream rendering
WebSocket processing
tool output
filesystem operations
context assembly
```

Avoid:

```text
whole-app rerenders
unbounded DOM growth
unbounded logs
repeated filesystem scans
duplicate extraction
duplicate chunking
duplicate network requests
blocking UI work
```

Prefer:

```text
bounded histories
memoization where useful
stable selectors
virtualized lists
lazy loading
debouncing
batched updates
event coalescing
incremental processing
```

Do not add visual effects that materially reduce application responsiveness.

Respect:

```text
prefers-reduced-motion
```

---

# Frontend

Frontend stack:

```text
Node.js
TypeScript
React
Vite
Zustand
REST
WebSocket
```

Root-level frontend configuration:

```text
package.json
package-lock.json
src/
```

Do not create:

```text
web/package.json
```

Primary commands:

```bash
npm install
npm run typecheck
npm run lint
npm run build
npm run build:web
npm run sync:web
npm run preview
npm run format
npm run format:check
```

Large workspaces should remain lazy-loaded.

Keep React client boundaries small.

---

# Backend

Go is the systems/runtime core.

Target runtime:

```text
Go 1.26
Wails v3
```

The backend owns:

```text
process lifecycle
llama.cpp lifecycle
model lifecycle
tool execution
filesystem access
context processing
chunking
cache
sessions
memory
recall
research
sandbox
verification
```

Do not move security-critical runtime behavior into the frontend.

---

# CI and Release

Primary CI workflow:

```text
.github/workflows/build-desktop.yml
```

Windows:

```text
windows-latest
CGO_ENABLED=0
```

Linux:

```text
ubuntu-24.04
CGO_ENABLED=1
```

Linux Wails dependencies include:

```text
libgtk-4-dev
libwebkitgtk-6.0-dev
libsoup-3.0-dev
pkg-config
zip
```

Do not regress to the obsolete GTK3/WebKitGTK 4.1 dependency path unless deliberately building a legacy target.

Primary release assets:

```text
SHEYTAN-Local-Agent-Windows-x64-vX.Y.ZZ.zip
SHEYTAN-Local-Agent-Linux-x64-vX.Y.ZZ.zip
```

Each package contains:

```text
SHEYTAN-Local-Agent/
```

---

# Version Discipline

When the application version changes, verify every relevant version surface:

```text
internal/config/config.go
package.json
.github/workflows/build-desktop.yml
SIGNATURE
portable launch scripts
documentation
release metadata
```

Current development target:

```text
APP_VERSION = 1.1.3
Codename    = Zeta
Release tag = v1.1.3Z
```

Do not describe unfinished functionality as released functionality.

---

# Definition of Done

A feature is complete only when:

```text
UI exists
   +
API exists
   +
runtime implementation exists
   +
real operation occurs
   +
state updates
   +
errors are surfaced
   +
cancellation works
   +
relevant tests pass
   +
desktop build passes
   +
functional behavior is verified
```

A button is not a feature.

An endpoint is not a feature.

A successful compile is not a feature.

A commit is not proof.

A desktop window opening is not proof.

---

# Immediate v1.1.3Z Goal

The next milestone is **live model end-to-end functionality**.

Priority:

```text
Application launch
    ↓
Automatic llama.cpp startup
    ↓
Actual model readiness
    ↓
First inference
    ↓
Streaming response
    ↓
Tool execution
    ↓
Attachment ingestion
    ↓
Chunking
    ↓
Context cache
    ↓
Long-context recall
    ↓
Reliable multi-turn interaction
    ↓
Objective verification
```

The system should feel functional before additional visual expansion is attempted.

---

# Development Rule

Always inspect the live repository before modifying architecture.

Always verify:

```text
current commit
relevant source
runtime behavior
Actions
logs
artifacts
```

Never assume that a pushed commit means the feature works.

Never claim a feature is complete without evidence.

When the user says:

```text
done, check verify and continue
```

perform:

```text
inspect
  ↓
verify
  ↓
diagnose
  ↓
fix
  ↓
retest
  ↓
continue
```

---

# License

SHEYTAN™ Local-Agent is proprietary software.

See `LICENSE` for the governing terms.

````

### `worklog.md`

```markdown
# SHEYTAN-Local-Agent — Worklog

## Current State

Date: 2026-09-04

Repository:

```text
https://github.com/Parsaetak/SHEYTAN-local-agent
````

Branch:

```text
main
```

Current verified main commit:

```text
0d8aff61eaa3cbdcdf723254f03883d33c403e51
```

Latest published release:

```text
v1.1.2Z
```

Published release assets:

```text
SHEYTAN-Local-Agent-Windows-x64-v1.1.2Z.zip
SHEYTAN-Local-Agent-Linux-x64-v1.1.2Z.zip
```

The v1.1.2Z release and its packaging pipeline are established.

The current development problem is different:

> The desktop application starts, but the end-to-end application is still not reliably functional.

The next phase is therefore a functionality milestone, not another UI milestone.

---

# v1.1.3Z — Live Runtime Functionality

## Primary Objective

Make the application operate as a real local AI agent from startup to completed task.

Target:

```text
launch application
      ↓
llama.cpp starts automatically
      ↓
model is resolved
      ↓
engine becomes actually ready
      ↓
agent sends inference request
      ↓
response streams
      ↓
tools execute
      ↓
attachments are processed
      ↓
long context is chunked / cached
      ↓
relevant context is recalled
      ↓
agent continues reasoning
      ↓
final result appears
```

No manual llama.cpp launch should be required during normal operation.

---

# 1. Automatic llama.cpp Lifecycle

## Requirement

The application must automatically manage llama.cpp.

Startup must perform:

```text
application startup
    ↓
load configuration
    ↓
resolve engine path
    ↓
ensure llama.cpp binary
    ↓
resolve model
    ↓
start process
    ↓
wait for readiness
    ↓
health check
    ↓
publish actual engine state
```

Required states:

```text
idle
downloading
starting
ready
running
busy
stopping
stopped
failed
```

The UI must not invent these states.

Backend/process state is authoritative.

## Acceptance

A clean desktop launch must result in:

```text
SHEYTAN starts
→ llama.cpp starts
→ model becomes ready
→ agent can send inference
```

without requiring the user to run a separate command.

---

# 2. Engine Failure Recovery

The engine must detect:

```text
process death
startup failure
HTTP failure
model load failure
port conflict
invalid executable
invalid model
```

Recovery policy:

```text
detect
  ↓
diagnose
  ↓
retry/restart when safe
  ↓
surface actual error
```

Do not report:

```text
ready
```

after a failed launch attempt.

---

# 3. Model Lifecycle

Required path:

```text
discover
 ↓
inspect
 ↓
select
 ↓
resolve
 ↓
load
 ↓
verify
 ↓
use
```

Model state must represent reality.

Required observability:

```text
model name
path
size
format
active state
engine state
load state
error
```

The frontend model selection and backend active model must never diverge.

---

# 4. End-to-End Agent

The Agent is the highest-priority user workflow.

Required:

```text
new session
message input
actual inference
streaming
conversation history
tool calls
tool results
errors
abort
retry
regenerate
runtime state
model state
```

The most important acceptance test is:

```text
open app
→ create/use session
→ enter message
→ submit
→ llama.cpp is used
→ output streams
→ final assistant message appears
```

---

# 5. Tool Execution

Verify that tool execution is actually connected end-to-end.

For every tool:

```text
model tool call
    ↓
tool registry
    ↓
argument validation
    ↓
runtime execution
    ↓
actual result
    ↓
activity event
    ↓
follow-up LLM turn
```

A visible tool definition without actual execution is incomplete.

Tool failures must surface as errors.

---

# 6. Attachments

Add dedicated Agent attachment tooling.

Required workflow:

```text
select file
 ↓
validate
 ↓
stage
 ↓
inspect
 ↓
extract
 ↓
chunk
 ↓
cache
 ↓
associate with message/session
 ↓
provide relevant representation to model
```

Required capabilities:

```text
attach
inspect
remove
reuse
show in conversation
```

Do not put raw arbitrary binary data into prompts.

The system must distinguish:

```text
source file
staged file
metadata
extracted content
chunks
cache entry
```

---

# 7. Context Chunking

The repository already contains:

```text
internal/chunking/
```

The next goal is to connect chunking to the real Agent/context pipeline.

Required:

```text
raw content
 ↓
normalize
 ↓
chunk
 ↓
hash
 ↓
store metadata
 ↓
retrieve relevant chunks
 ↓
assemble bounded prompt context
```

The chunking system must support:

```text
large text files
documents
conversation summaries
attachment content
project source
research material
```

Prefer semantic boundaries.

Do not unnecessarily split:

```text
functions
classes
code blocks
document sections
```

---

# 8. Context Cache

Implement a real cache around processed context.

At minimum cache:

```text
file extraction
normalized content
chunking
chunk metadata
content hash
retrieval results
```

Cache identity must depend on content.

Recommended:

```text
source identity
+
content hash
+
processing version
+
configuration
```

Required properties:

```text
bounded
versioned
concurrency-safe
invalidatable
observable
```

Never reuse stale content merely because the source path is unchanged.

---

# 9. Long Context

Do not construct prompts by blindly concatenating the entire session.

Preferred context:

```text
recent messages
+
relevant historical messages
+
memory
+
recalled chunks
+
attachment chunks
+
tool evidence
+
current task state
```

When context exceeds the available budget:

```text
measure
 ↓
prioritize
 ↓
compress
 ↓
retrieve
 ↓
prune low-value material
 ↓
assemble
```

The current task, recent messages, tool evidence, and important state must survive context pressure.

---

# 10. Context Provenance

Every injected external/context item should be traceable.

Useful provenance:

```text
source
source type
source path / URL
session
chunk ID
content hash
retrieval reason
timestamp
```

The UI should be able to present a concise context summary without exposing unnecessary internal prompt details.

---

# 11. Sessions

Verify:

```text
create
switch
rename
delete
history
active model
activity
```

Long sessions must not create uncontrolled in-memory growth.

History handling should be bounded and compatible with the new chunk/cache system.

---

# 12. Performance

The long-context path must optimize:

```text
deduplication
incremental processing
cache hits
bounded allocations
bounded history
stream rendering
WebSocket event volume
```

Avoid:

```text
re-extracting unchanged files
re-chunking unchanged files
rebuilding identical context repeatedly
serializing huge histories unnecessarily
rerendering the whole UI for every token
```

The target remains smooth interactive behavior, including 120 Hz-capable displays.

---

# 13. Reliability

Every long-running operation must have:

```text
context cancellation
timeout
bounded output
cleanup
error propagation
```

No process leaks.

No goroutine leaks.

No unbounded buffers.

No fake success states.

---

# 14. Security

Do not regress:

```text
filesystem boundaries
workspace isolation
symlink protection
shell policy
Git policy
network restrictions
SSRF protection
process-tree cancellation
secret redaction
safe archive extraction
```

Attachments must be processed through controlled staging.

Extracted content must inherit the same resource limits as other model-controlled inputs.

---

# 15. Testing

Required tests for this phase:

## Engine

```text
automatic startup
already-running engine adoption
startup failure
dead-process detection
restart
port conflict
invalid model
```

## Agent

```text
first inference
streaming
tool call
tool result
multi-turn conversation
abort
retry
error propagation
```

## Attachments

```text
small text file
large text file
binary rejection/inspection
duplicate file
modified file
remove attachment
multiple attachments
```

## Chunking

```text
small input
large input
structured code
document sections
stable chunk IDs
hash changes
```

## Cache

```text
cache hit
cache miss
content invalidation
version invalidation
concurrent access
bounded growth
```

## Long Context

```text
history pressure
attachment pressure
mixed memory + documents
retrieval
context truncation
context rebuild
```

---

# 16. Release Gate

v1.1.3Z is not complete because:

```text
the UI looks good
```

or:

```text
the executable launches
```

or:

```text
the build passes
```

The release gate is:

```text
desktop launch
+
automatic engine startup
+
actual model inference
+
working agent
+
working tools
+
working attachments
+
working chunking
+
working cache
+
working long-context recall
+
functional tests
+
CI
```

---

# 17. Development Order

Work in this order unless a verified dependency requires otherwise:

```text
1. llama.cpp lifecycle
2. model lifecycle
3. end-to-end inference
4. tool loop
5. attachments
6. chunking integration
7. context cache
8. long-context recall
9. session/history pressure
10. performance
11. integration tests
12. UI polish
```

Do not spend the next phase primarily on decorative UI work while the core runtime path remains broken.

---

# 18. Engineering Rules

Always:

```text
inspect live repository
verify current commit
inspect relevant files
inspect Actions when CI matters
inspect logs when failures exist
test actual runtime behavior
verify artifacts where relevant
```

Never:

```text
assume a commit works
claim startup means functionality
claim a button is a feature
claim an endpoint is a feature
claim a compile proves correctness
```

The authoritative sequence is:

```text
source
+
runtime
+
test
+
verification
```

---

# 19. Working Definition of Done

A user-facing feature is done only when:

```text
frontend
 ↓
API
 ↓
runtime
 ↓
actual operation
 ↓
state update
 ↓
visible result
 ↓
error path
 ↓
cancellation
 ↓
test evidence
```

All of those stages must work.

---

# 20. Current Next Task

The next implementation task is:

> **Make llama.cpp startup fully automatic and prove a clean desktop launch can reach a real, healthy model endpoint without manual engine intervention.**

After that:

```text
first real inference
→ tool loop
→ attachments
→ chunking
→ cache
→ long context
```

````

### `agent.md`

```markdown
# SHEYTAN-Local-Agent — Agent Context

> Persistent engineering handoff for the next agent working on this repository.

Repository:

```text
https://github.com/Parsaetak/SHEYTAN-local-agent
````

Current branch:

```text
main
```

Current verified main commit:

```text
0d8aff61eaa3cbdcdf723254f03883d33c403e51
```

Current published release:

```text
v1.1.2Z
```

Current development target:

```text
v1.1.3Z
```

---

# 1. Mission

SHEYTAN-Local-Agent is a local-first AI software-engineering environment.

Core principle:

> **The model proposes. The tools execute. The laboratory verifies.**

The current mission is not to add more visual surface area.

The current mission is to make the existing desktop application **actually functional end-to-end**.

Primary target:

```text
desktop launch
    ↓
automatic llama.cpp startup
    ↓
actual model readiness
    ↓
real inference
    ↓
tool execution
    ↓
attachments
    ↓
chunking
    ↓
context cache
    ↓
long-context recall
    ↓
reliable multi-turn agent
```

---

# 2. Important Current Reality

The application can launch as a desktop application.

That does **not** mean the complete runtime workflow is finished.

The current development phase must therefore assume:

```text
desktop shell != functional agent
```

The next agent must prioritize runtime integration and real behavior.

Do not treat UI presence as proof of functionality.

---

# 3. Current Architecture

Backend:

```text
Go 1.26
Wails v3
Go HTTP API
WebSocket activity
```

Frontend:

```text
React
TypeScript
Vite
Zustand
```

Primary runtime packages:

```text
internal/agent
internal/aicontext
internal/api
internal/browser
internal/chunking
internal/config
internal/desktop
internal/lab
internal/llm
internal/memory
internal/multiagent
internal/native
internal/netcheck
internal/proc
internal/recall
internal/research
internal/runtime
internal/sandbox
internal/sessions
internal/sysinfo
internal/tools
internal/updater
internal/vision
```

---

# 4. Desktop Shell

Production architecture:

```text
React/Vite
    ↓
web/static
    ↓
Go embed
    ↓
Wails
    ↓
Go runtime
```

No separate production frontend server should be required.

Do not reintroduce:

```text
Fyne
internal/ui
legacy desktop UI
```

---

# 5. Absolute Development Rules

Before modifying anything:

```text
inspect live repository
verify exact main commit
inspect relevant source
inspect current Actions when relevant
inspect logs when relevant
verify actual runtime behavior
```

Never:

```text
assume a commit works
assume a successful build means functionality
assume a UI control is wired
assume an endpoint is wired
assume engine state is true
claim success without evidence
```

When the user says:

```text
done, check verify and continue
```

the required loop is:

```text
inspect
 ↓
verify
 ↓
diagnose
 ↓
fix
 ↓
retest
 ↓
continue
```

Do not merely acknowledge.

---

# 6. Current Highest Priority — llama.cpp

`internal/llm/llama.go` contains the application-managed `LlamaServer`.

The intended architecture is:

```text
SHEYTAN startup
    ↓
ensure llama.cpp binary
    ↓
resolve model
    ↓
spawn llama.cpp
    ↓
wait for health
    ↓
publish actual engine state
    ↓
allow agent inference
```

The application must manage the engine automatically.

## Non-negotiable requirement

The user should not need to manually start llama.cpp.

Normal behavior:

```text
launch SHEYTAN
→ engine starts automatically
→ model is loaded
→ health is verified
→ agent can use it
```

---

# 7. Engine State

Engine state must come from actual runtime/process state.

Valid states:

```text
idle
downloading
starting
ready
running
busy
stopping
stopped
failed
```

Never allow the frontend to claim:

```text
ready
running
loaded
```

without backend evidence.

Engine health should verify the real llama.cpp endpoint.

---

# 8. Engine Recovery

Handle:

```text
missing binary
download failure
invalid executable
invalid model
model load failure
port conflict
process death
HTTP failure
startup timeout
```

Expected strategy:

```text
detect
 ↓
diagnose
 ↓
retry/restart where safe
 ↓
surface actual failure
```

Do not leave stale `running` state after process death.

---

# 9. Model Lifecycle

Required lifecycle:

```text
discover
 ↓
inspect
 ↓
select
 ↓
resolve path
 ↓
start engine
 ↓
load model
 ↓
verify
 ↓
use
```

Model state must be authoritative.

Expose where practical:

```text
name
path
size
format
architecture
quantization
context capacity
selected
loaded
running
error
```

The active model displayed in the UI must match the model actually used by the engine.

---

# 10. End-to-End Agent

The most important acceptance test is:

```text
open desktop application
 ↓
session exists
 ↓
enter message
 ↓
submit
 ↓
llama.cpp receives inference request
 ↓
assistant output streams
 ↓
final response appears
```

The Agent must support:

```text
conversation
streaming
history
tool calls
tool results
errors
abort
retry
regenerate
runtime visibility
model visibility
attachments
context state
```

A response that never reaches a model is a runtime failure.

---

# 11. Tool Loop

Required execution:

```text
LLM
 ↓
tool call
 ↓
registry
 ↓
validation
 ↓
tool execution
 ↓
result
 ↓
activity event
 ↓
follow-up LLM request
 ↓
final response
```

Tool definitions exposed to the model must match the real runtime registry.

Do not advertise unavailable tools.

Do not silently convert tool failures into successful assistant prose.

---

# 12. Attachments

Attachment support is now a priority requirement.

The repository should gain dedicated attachment functionality rather than relying on ad-hoc frontend file handling.

Required flow:

```text
select
 ↓
validate
 ↓
stage
 ↓
inspect
 ↓
extract
 ↓
chunk
 ↓
cache
 ↓
associate with session/message
 ↓
retrieve relevant content
 ↓
send to model
```

Required operations:

```text
attach
inspect
remove
reuse
show in conversation
```

Do not inject arbitrary binary data into prompts.

Use controlled staging.

Bound:

```text
file size
extraction size
processing time
memory
chunk count
```

---

# 13. Context Chunking

`internal/chunking/` already exists.

The next task is not merely to have a chunking package.

The next task is to connect it to the actual Agent context pipeline.

Required:

```text
source
 ↓
normalize
 ↓
chunk
 ↓
hash
 ↓
metadata
 ↓
store
 ↓
retrieve
 ↓
assemble context
```

Potential chunk sources:

```text
attachments
documents
source files
conversation history
research
memory-derived context
tool output
```

Prefer semantic boundaries:

```text
headings
sections
paragraphs
functions
classes
code blocks
sentences
```

Avoid destructive arbitrary splits when better boundaries exist.

---

# 14. Chunk Identity

Chunks should have stable identifiers and provenance.

Recommended metadata:

```text
chunk ID
source ID
source type
path / URL
content hash
sequence
range
token estimate
processing version
timestamp
```

The same content processed with the same configuration should produce reusable chunk identities.

---

# 15. Context Cache

Context caching is a first-class requirement.

Cache suitable expensive transformations:

```text
file extraction
normalization
chunking
chunk metadata
content hashes
retrieval results
context assembly
```

Cache keys must be content-sensitive.

Recommended:

```text
source identity
+
content hash
+
processing version
+
relevant configuration
```

Never assume:

```text
same path == same content
```

The cache must support:

```text
hit
miss
invalidate
version change
concurrent access
bounded growth
inspection
```

---

# 16. Long Context

Never construct enormous prompts by blindly appending all history.

Preferred context:

```text
recent conversation
+
relevant recalled history
+
memory
+
attachment chunks
+
project context
+
tool evidence
+
current task state
```

When context pressure occurs:

```text
measure budget
 ↓
retain high-value state
 ↓
retrieve relevant chunks
 ↓
compress older history
 ↓
drop low-value material
 ↓
assemble final context
```

The system must know why a context item was included.

---

# 17. Context Budget

Every inference request should have an explicit context budget.

Account for:

```text
system instructions
tool schemas
recent messages
retrieved chunks
memory
attachments
tool results
requested output tokens
```

Do not exceed the actual model/runtime context limit.

Do not rely on frontend estimates if the backend can determine the real budget.

---

# 18. Context Provenance

The runtime should be able to identify:

```text
where context came from
why it was included
which chunk was used
which version/hash was processed
which session supplied it
```

Useful provenance:

```text
source
source type
path / URL
session
chunk ID
content hash
retrieval reason
timestamp
```

The UI should expose a compact useful summary, not raw internal prompts.

---

# 19. Memory / Recall

Memory and recall are distinct:

```text
memory = durable information
recall = task-specific selection
```

Recall should feed the context pipeline.

Conceptually:

```text
memory
 ↓
candidate recall
 ↓
relevance
 ↓
context budget
 ↓
prompt
```

Memory classes:

```text
M1 trusted facts
M2 preferences
M3 project state
M4 decisions
M5 procedures / learned fixes
M6 summaries
M7 provisional observations
```

External research must retain provenance and trust boundaries.

---

# 20. Sessions

Required:

```text
create
rename
switch
delete
history
active model
activity
metadata
```

The session should not duplicate massive context unnecessarily.

Use:

```text
summaries
chunks
cache
recall
```

to control history growth.

---

# 21. Performance

The application should remain responsive during:

```text
LLM streaming
engine startup
attachment extraction
chunking
cache operations
research
tool execution
large filesystem operations
```

Avoid:

```text
whole-app rerenders
unbounded history
unbounded logs
duplicate scans
duplicate parsing
duplicate chunking
duplicate extraction
synchronous heavy frontend work
```

Prefer:

```text
lazy loading
stable selectors
memoization where useful
virtualization
debouncing
bounded buffers
event coalescing
incremental processing
```

120 Hz responsiveness remains a target, but correctness comes first.

---

# 22. Runtime Safety

Preserve all existing:

```text
path validation
workspace boundaries
symlink protection
shell policy
Git policy
network restrictions
SSRF controls
DNS validation
redirect validation
process-tree cancellation
timeouts
bounded output
secret redaction
archive safety
```

Never weaken a safety control merely to unblock a feature.

---

# 23. Testing Requirements

A runtime feature is incomplete until tested at the correct layer.

## Engine

```text
automatic start
health readiness
startup timeout
failure
restart
port conflict
dead process
invalid model
```

## Agent

```text
first inference
streaming
multi-turn
tool call
tool result
abort
retry
error
```

## Attachments

```text
small text
large text
multiple files
duplicate file
modified file
invalid input
remove
reuse
```

## Chunking

```text
small input
large input
code structure
document structure
stable IDs
hash invalidation
```

## Cache

```text
hit
miss
invalidation
version invalidation
concurrency
bounds
```

## Long Context

```text
history pressure
attachment pressure
memory + attachment
retrieval
compression
reassembly
```

---

# 24. CI

Primary workflow:

```text
.github/workflows/build-desktop.yml
```

Windows:

```text
windows-latest
CGO_ENABLED=0
```

Linux:

```text
ubuntu-24.04
CGO_ENABLED=1
```

Linux dependencies:

```text
libgtk-4-dev
libwebkitgtk-6.0-dev
libsoup-3.0-dev
pkg-config
zip
```

Do not restore:

```text
libgtk-3-dev
libwebkit2gtk-4.1-dev
```

except for an explicitly intentional legacy target.

---

# 25. Frontend Build

Root-level package:

```text
package.json
```

Never create:

```text
web/package.json
```

Required checks:

```bash
npm install
npm run typecheck
npm run lint
npm run build
```

After frontend changes:

```text
Vite build
 ↓
scripts/sync-web.mjs
 ↓
web/static
```

Verify the embedded application contains the new assets.

---

# 26. Release Discipline

When changing versions, synchronize:

```text
internal/config/config.go
package.json
.github/workflows/build-desktop.yml
SIGNATURE
portable scripts
release documentation
```

Target:

```text
APP_VERSION = 1.1.3
Codename    = Zeta
Tag         = v1.1.3Z
```

Do not label unfinished functionality as complete.

---

# 27. Regression Rules

Never reintroduce:

```text
internal/ui
Fyne UI
old versioned stress programs
old e2e scripts
obsolete GTK3-only Linux workflow
unbounded execution
unbounded output
secret leakage
```

Preserve the current Go + React + Wails architecture unless a demonstrated technical requirement justifies a change.

---

# 28. Definition of Done

A feature is done only when:

```text
frontend action
 ↓
API
 ↓
backend runtime
 ↓
real operation
 ↓
actual state change
 ↓
visible result
 ↓
error path
 ↓
cancellation
 ↓
tests
 ↓
runtime verification
```

A button is not a feature.

A compile is not a feature.

An endpoint is not a feature.

A desktop window is not a feature.

A commit is not proof.

---

# 29. Required Development Sequence

Use this order:

```text
1. Automatic llama.cpp startup
2. Model lifecycle
3. First real inference
4. Tool loop
5. Attachment tools
6. Chunking integration
7. Context cache
8. Long-context recall
9. Session/history optimization
10. Functional tests
11. Performance
12. UI polish
```

Do not reverse this order just because visual changes are easier.

---

# 30. Immediate Next Task

The next implementation task is:

> **Make llama.cpp start automatically on desktop launch and prove that a newly launched application can reach a real healthy model endpoint without manual engine intervention.**

Acceptance:

```text
start application
 ↓
llama.cpp starts automatically
 ↓
model resolves
 ↓
engine becomes healthy
 ↓
UI reflects actual ready state
 ↓
agent can send a real inference request
```

Once that is verified, continue directly to:

```text
first real inference
→ tool loop
→ attachments
→ chunking
→ cache
→ long context
```

---

# 31. Final Rule

The application must become **functional before it becomes larger**.

Prefer:

```text
real behavior
+
verification
+
reliability
+
performance
```

over:

```text
more panels
+
more settings
+
more visual features
```

The next agent must work from evidence, not assumptions.

```

These replacements intentionally correct the documentation drift: **v1.1.2Z remains the published release, while v1.1.3Z is treated as the live-runtime functionality phase rather than pretending the application is already feature-complete.**
```
