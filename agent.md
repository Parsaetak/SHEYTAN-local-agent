# SHEYTAN-Local-Agent — Agent Context

> Persistent engineering handoff for the next agent working on this repository.
>
> Current stable release: **v1.1.2Z (Zeta)** — AAA application milestone delivered
>
> Current development target: **v1.1.3Z — Live model end-to-end polish**

Repository:

https://github.com/Parsaetak/SHEYTAN-local-agent

---

# 1. Mission

SHEYTAN-Local-Agent is a local-first autonomous AI software-engineering environment.

The product principle is:

> **The model proposes. The tools execute. The laboratory verifies.**

SHEYTAN is not intended to remain a chatbot with engineering tools attached.

The v1.1.2Z target is a complete premium desktop engineering application.

The target experience is:


AI agent
+
local models
+
tools
+
research
+
memory
+
coding laboratory
+
verification
+
desktop UX
+
advanced runtime control

All major sections must be functional.

2. Current Release State

Current stable release:

Version:  1.1.2
Codename: Zeta
Tag:      v1.1.2Z

Verified release assets:

SHEYTAN-Local-Agent-Windows-x64-v1.1.2Z.zip
SHEYTAN-Local-Agent-Linux-x64-v1.1.2Z.zip

The v1.1.2Z release delivered the AAA application milestone: the 120Hz
motion system, idle WebSocket standby (topbar now shows "Connected"), rich
models API ({id, name, path, sizeBytes}), the Settings null-crash fix,
first-launch auto-session, and the fully restored Windows CI pipeline
(gen-syso icon/resources, -H=windowsgui, .bat launcher, stress gate in CI).

All legacy code has been removed: stale web/static artifacts, the broken
Taskfile.yml, the versioned e2e scripts, and the orphaned glm-proxy.

Next release target:

Version:  1.1.3
Codename: Zeta
Tag:      v1.1.3Z

v1.1.3Z is the live-model end-to-end polish release.

3. Non-Negotiable Development Rules
Inspect the live GitHub repository before making assumptions.
Verify the exact current main commit SHA.
Inspect the relevant files before editing.
Inspect current GitHub Actions when CI/build behavior is involved.
Inspect actual workflow logs when diagnosing failures.
Verify release assets when release packaging is involved.
Never claim success without evidence.
Never assume a pushed change worked because a commit exists.
Preserve the current architecture unless there is a demonstrated engineering reason to change it.
Prefer coherent incremental improvements over unnecessary rewrites.
Provide complete replacement files when the user needs to manually replace code.
Do not reintroduce deleted legacy architecture.
Do not recreate the Fyne UI.
Do not recreate internal/ui.
Do not accumulate old cmd/stress_vNNN.go files.
Do not restore retired e2e scripts.
Keep frontend package configuration at repository root.
Do not create web/package.json.
Keep secrets out of frontend responses.
Keep secrets out of committed configuration.
Do not silently weaken security policy to make a feature easier.
Long-running operations require cancellation, timeout, cleanup, and bounded output.
User-visible functionality is not complete just because a button or endpoint exists.
Never use fake success states.
Prefer actual runtime state over optimistic UI state.
Do not add animation that materially damages responsiveness.
Do not trade 120Hz responsiveness for decorative visual effects.
Respect reduced-motion preferences.
Keep critical execution logic in Go.
Keep presentation logic in React.
Maintain static-export/embed compatibility.
Do not introduce a major framework migration without clear evidence.
When the user says done, check verify and continue, inspect, verify, diagnose, then continue.
4. Current Architecture
Backend
Go 1.26
Wails v3
Go HTTP API
WebSocket activity

Primary packages:

internal/runtime
internal/api
internal/config
internal/llm
internal/agent
internal/tools
internal/memory
internal/recall
internal/lab
internal/research
internal/browser
internal/vision
internal/sandbox
internal/sessions
internal/logging
internal/desktop
internal/diagnostics
internal/sysinfo
internal/updater
5. Desktop Shell

The shipping desktop application uses Wails v3.

Windows
 └── WebView2

Linux
 └── WebKitGTK 6.0

The frontend is embedded into the desktop executable.

Production architecture:

React/Vite assets
      ↓
web/static
      ↓
Go embed
      ↓
Wails desktop shell
      ↓
Go API + WebSocket

There is no separate production frontend server.

6. Frontend

The frontend uses:

React
TypeScript
Vite
Zustand
lightweight icon library

Frontend package lives at:

package.json
package-lock.json
src/

There is intentionally no:

web/package.json

Build path:

npm install
npm run typecheck
npm run lint
npm run build

Build synchronization:

Vite dist
 ↓
scripts/sync-web.mjs
 ↓
web/static

web/static is committed because the Go application embeds it.

Whenever src/ changes:

rebuild frontend
+
update web/static
7. Current UI Workspaces

Current frontend workspaces include:

AgentBody
AgentSidebar
AgentHeader
LabPanel
ResearchPanel
SettingsPanel

Workspace routing is controlled through:

src/App.tsx
src/workspace.ts

Large workspace components should remain lazy-loaded.

8. v1.1.2Z Product Definition

v1.1.2Z is the AAA application milestone.

It must complete all major user-facing areas:

Agent
Sessions
Models
Engine
Settings
Coding Lab
Research
Memory
Recall
Browser
Vision
Tools
Diagnostics
Logs
Hardware
Updates

Every major section must be usable.

A visible control must map to:

real frontend action
 ↓
real API call
 ↓
real backend operation
 ↓
real state refresh

No fake controls.

No dead-end flows.

No buttons that exist only visually.

9. Agent Workspace Requirements

The Agent is the primary interface.

Required:

new session
message input
streaming assistant output
conversation history
tool calls
tool results
research events
verification events
errors
cancellation
retry
regenerate
model visibility
runtime state
attachments

The user must always be able to understand:

what SHEYTAN is doing
what tool is running
whether the engine is ready
whether the task succeeded
why it failed
10. Model Requirements

Model management must be real.

Required:

discover local GGUF models
refresh models
show metadata where available
show size
show path
show loaded state
select model
load
unload/stop engine
restart
health state

Model state must distinguish:

missing
available
loading
loaded
running
stopping
stopped
error

The frontend must not claim a model is active until the runtime confirms it.

11. Engine Requirements

Required engine operations:

start
stop
restart
status
health
active model
endpoint
process state

State should represent actual runtime/process state.

Target state machine:

idle
starting
ready
busy
stopping
stopped
failed
12. LLM / Inference Requirements

Advanced inference controls should be available where supported:

temperature
top-p
top-k
min-p
repeat penalty
max tokens
context size
seed
grammar/structured output where supported
threads
parallelism
GPU layers where supported
batch parameters where supported

Unsupported parameters must not silently appear functional.

13. Presets

Presets must modify actual runtime behavior.

Suggested built-ins:

Balanced
Fast
Reasoning
Coding
Research
Creative
Low Memory
High Quality

The active preset must be visible.

Custom configuration should remain possible.

14. Agent Behavior

Advanced agent configuration should include:

planning
autonomy
tool use
research
memory
verification
repair
iteration limits
timeouts
parallelism

Dangerous controls require explicit UI treatment.

15. Tool Registry

Tools are first-class components.

UI should expose:

name
category
description
enabled state
availability
recent activity
errors

Categories include:

filesystem
shell
git
browser
research
vision
memory
sandbox
process
diagnostics

Tool execution should appear in the activity stream.

16. Coding Lab

Coding Lab is the engineering execution core.

Required workflow:

select project
 ↓
create isolated workspace
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
promote/discard

Required UI:

project selector
workspace selector
file tree
file view/editor
diff viewer
terminal
test/build actions
verification panel
workspace state
promotion controls

Verification is authoritative.

The model does not decide whether a task passed.

17. Coding Lab Security

Preserve:

path validation
base directory restrictions
workspace isolation
symlink protection
Git policy
shell policy
network policy
destructive command policy
process-tree cancellation
timeouts
bounded output
environment sanitization

Never weaken these controls to make UI behavior easier.

18. Research

Research must provide:

query
provider
results
authority
relevance
URL
snippet
source metadata

Research is evidence.

Research does not become unquestioned truth.

Existing providers may include:

Auto
GitHub
Reddit
SearXNG
DuckDuckGo
Web compatibility alias

Maintain bounded requests and response bodies.

Maintain SSRF protections.

19. Memory / Recall

Memory must be inspectable.

Required:

list
search
inspect
delete
refresh
metadata

Recall should show useful context provenance without exposing unnecessary internal prompt details.

Memory IDs must remain collision-safe.

20. Browser / Vision

Browser activity must be visible.

Show:

browser state
task
URL
page title
screenshots
errors
cancellation

Vision must expose actual processing state and results.

Both must preserve existing safety boundaries.

21. Attachments

Agent attachments should support:

select
stage
inspect
remove
show in conversation

Use controlled file staging.

Do not blindly insert arbitrary binary content into prompts.

Enforce file-size and processing bounds.

22. Sessions

Sessions are persistent units of interaction.

Required:

create
rename
switch
delete
history
active model
activity
metadata

Future candidates:

search
pin
archive
export

Frontend and backend session state must remain synchronized.

23. Settings

Organize Settings into:

General
Models
Inference
Agent
Tools
Browser
Vision
Memory
Coding Lab
Research
Engine
Network
Security
Performance
Diagnostics
Updates
About

Prefer progressive disclosure.

Do not force users to manually edit configuration files for common operations.

24. Diagnostics / Logs

Diagnostics should include:

runtime health
engine health
API health
model state
tool state
memory state
recent errors
performance
logs
export

Logs UI should support:

filter
search
severity
category
timestamp
copy
export

Keep secret redaction active.

25. Hardware / System

Expose detected information such as:

OS
CPU
architecture
cores
RAM
GPU
VRAM
available memory
runtime state

Never fabricate unsupported hardware data.

26. Update System

Expose:

application version
latest known version
engine version
manual check
schedule
update status

Updates must be explicit and safe.

27. AAA UI Requirements

The application should feel like a polished desktop product.

Target qualities:

premium
modern
technical
dense where useful
clear
responsive
fast
consistent

Avoid:

visual noise
gratuitous animation
fake loading
over-large controls
excessive empty space
ambiguous actions
unnecessary modal dialogs

Use strong state communication:

ready
busy
warning
error
success
offline
28. 120Hz / High-Refresh Target

The application should target smooth interaction at:

120 Hz

The target is not merely a benchmark.

It means:

smooth scrolling
responsive typing
fast workspace switching
stable activity stream
responsive controls
no visible stalls during inference

Use:

CSS transforms/opacity for motion
small render surfaces
memoization where useful
stable Zustand selectors
virtualized large lists
debounced expensive operations
bounded event buffers
lazy loading

Avoid:

forced layout loops
large synchronous work in render
whole-app rerenders
unnecessary DOM growth
unbounded activity histories
29. Motion System

Use a consistent animation language.

Animation categories:

micro
panel
workspace
status
loading
success
error

Motion should be:

short
purposeful
interruptible
cheap
consistent

Respect:

prefers-reduced-motion

Never block user input with animation.

30. Performance Principles

Performance is a feature.

Protect:

input latency
scrolling
workspace navigation
WebSocket processing
inference rendering
tool output rendering
filesystem interaction

Large datasets should be:

bounded
virtualized
paged
incrementally rendered

Long-running operations must never monopolize the UI.

31. API / WebSocket Requirements

API behavior must remain:

session-aware
bounded
cancelable
safe
serializable

WebSocket handling must support:

connect
disconnect
reconnect where appropriate
per-session activity
bounded history

Do not rerender the entire application for every event.

32. Security Requirements

Never regress:

sandbox path validation
workspace isolation
SSRF controls
DNS validation
redirect validation
shell policy
Git policy
process-tree cleanup
secret redaction
safe archive extraction

The LLM is an untrusted proposal source.

Runtime policy remains authoritative.

33. CI Requirements

Workflow:

.github/workflows/build-desktop.yml

Windows:

windows-latest
CGO_ENABLED=0

Linux:

ubuntu-24.04
CGO_ENABLED=1

Linux Wails v3 dependencies:

libgtk-4-dev
libwebkitgtk-6.0-dev
libsoup-3.0-dev
pkg-config
zip

Do not regress to:

libgtk-3-dev
libwebkit2gtk-4.1-dev

Node must be pinned in CI.

Current intended Node:

Node 24

Go:

Go 1.26
34. Frontend CI

Required:

npm install
npm run typecheck
npm run lint
npm run build

Verify:

web/static/index.html

exists after build.

Do not reintroduce the old formatting gate merely to create another CI check.

35. Backend CI

Required:

go test ./internal/... -tags headless -count=1
go test ./... -run Test -count=1
go vet ./...

Desktop build must be tested.

Executable smoke test must validate actual output.

36. Release Packaging

Primary release assets remain exactly:

SHEYTAN-Local-Agent-Windows-x64-vX.Y.ZZ.zip
SHEYTAN-Local-Agent-Linux-x64-vX.Y.ZZ.zip

Each contains:

SHEYTAN-Local-Agent/

The application, models directory, workspace, runtime/supporting files remain together.

Raw executables are not the primary release distribution.

37. Version Discipline

For every release, synchronize:

internal/config/config.go
package.json
.github/workflows/build-desktop.yml
SIGNATURE
portable scripts
documentation where versioned

For v1.1.2Z:

APP_VERSION = 1.1.2
Codename = Zeta
Tag = v1.1.2Z
38. Testing Philosophy

A feature is complete only when:

UI exists
+
real API call exists
+
backend behavior exists
+
state refresh exists
+
error path exists
+
relevant tests pass
+
CI passes
+
built artifact contains it

A button is not a feature.

A setting is not a feature.

A TypeScript compile is not a feature.

A commit is not proof.

39. Stress Testing

The current stress architecture uses the consolidated Zeta suite.

Do not recreate:

cmd/stress_v08.go
cmd/stress_v09.go
...

Extend:

cmd/stress.go
cmd/stress_zeta.go

or add focused unit/integration tests under:

internal/*

Stress areas should include:

memory uniqueness
log rotation
large inputs
tool failures
concurrency
Unicode
null/invalid arguments
sandbox boundaries
workspace security
session behavior
runtime lifecycle
40. Standard User Workflow

When user says:

done, check verify and continue

perform:

live repository inspection
 ↓
exact commit verification
 ↓
relevant file inspection
 ↓
Actions inspection
 ↓
logs inspection
 ↓
artifacts/release inspection
 ↓
identify defects
 ↓
implement next improvement

Never merely acknowledge.

41. Development Priorities

Unless evidence requires another order:

P0
Agent core
streaming
tool visibility
session state
model/engine synchronization

P0
Model lifecycle
discovery
selection
loading
engine control

P0
Coding Lab execution
terminal
files
diffs
verification

P1
Research
Memory
Recall
Browser
Vision

P1
Advanced settings
Diagnostics
Hardware
Updates

P1
AAA UI polish
120Hz motion
accessibility
responsive design

P0
Integration tests
stress
Windows/Linux packaging
release verification
42. Current Known-Fixed Problems

Do not regress:

Linux GTK mismatch
unanchored .gitignore rules
legacy Fyne UI
old versioned stress files
old e2e tooling
stale embedded frontend
unsafe archive extraction
sandbox timeout propagation
process-tree leaks
SSRF protections
Git escape paths
memory ID collisions
log rotation issues
43. Current Repository Baseline

At the start of v1.1.2Z, the architecture includes:

cmd/
internal/
src/
scripts/
web/
.github/
build/

Important production files include:

main.go
main_windows.go
main_other.go

.github/workflows/build-desktop.yml

src/App.tsx
src/AgentBody.tsx
src/SettingsPanel.tsx
src/workspace.ts
src/api.ts
src/main.tsx

internal/desktop
internal/runtime
internal/api
internal/agent
internal/llm
internal/tools
internal/lab
internal/research
internal/memory
internal/recall
internal/browser
internal/vision
44. Product Standard

SHEYTAN should become:

a local AI engineering workstation

not:

a chatbot with settings

The core workflow is:

ask
 ↓
understand
 ↓
inspect
 ↓
plan
 ↓
execute
 ↓
observe
 ↓
verify
 ↓
repair
 ↓
review
 ↓
finish

Every step should be observable and trustworthy.

45. Final v1.1.2Z Acceptance Criteria

The release is considered complete only when all of the following are true:

[ ] Agent is fully usable
[ ] streaming works
[ ] cancellation works
[ ] retry works
[ ] tool execution is visible
[ ] sessions are usable
[ ] models are discoverable
[ ] models are selectable
[ ] engine lifecycle works
[ ] inference settings actually apply
[ ] presets actually apply
[ ] Coding Lab can execute work
[ ] Coding Lab shows diffs
[ ] Coding Lab runs verification
[ ] Research works
[ ] Memory is inspectable
[ ] Recall is useful
[ ] Browser is visible and functional
[ ] Vision is functional
[ ] attachments work
[ ] diagnostics work
[ ] logs are inspectable
[ ] hardware information works
[ ] updates are visible
[ ] advanced settings work
[ ] security remains enforced
[ ] application stays responsive during long operations
[ ] UI is optimized for high-refresh displays
[ ] animations are smooth and purposeful
[ ] reduced motion is respected
[ ] Windows build passes
[ ] Linux build passes
[ ] Windows ZIP verified
[ ] Linux ZIP verified
[ ] stress suite passes
[ ] integration tests pass
[ ] release assets verified
46. Guiding Rule

The most important development rule for v1.1.2Z is:

Do not build more surface area until the existing surface actually works.

Prioritize:

functional depth
>
state correctness
>
reliability
>
performance
>
polish
>
additional features

The product must earn its visual sophistication through real functionality.

47. Next Target

The v1.1.2Z milestone is complete. The next implementation target is:

LIVE MODEL END-TO-END

Specifically:

real model download or user-provided GGUF flow
+
engine start/stop through the UI (already wired — verify against a real model)
+
streaming chat responses in the activity stream
+
tool execution visibility during a live run
+
cancellation mid-run
+
retry / regenerate

The API, WebSocket standby, session, and model-selector plumbing all work
end-to-end (verified by the v1.1.2Z live test suite, 28/28). What remains
is exercising the full loop against a real llama.cpp engine + GGUF model
and polishing everything the loop surfaces.
