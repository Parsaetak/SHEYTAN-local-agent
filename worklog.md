````markdown
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

```
```
