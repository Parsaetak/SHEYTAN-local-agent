````markdown
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
```
