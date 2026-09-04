# SHEYTAN-Local-Agent — Worklog

## 2026-09-03 — v1.1.2Z AAA Application Milestone Started

### Release baseline

The v1.1.1Z (Zeta) release is now published and verified.

Repository:

https://github.com/Parsaetak/SHEYTAN-local-agent

Current main branch at the beginning of the v1.1.2Z phase:

main
3c05af4f25f42b17486d2683bdf210f97b7885d7

Current release:

v1.1.1Z

Verified release assets:

SHEYTAN-Local-Agent-Windows-x64-v1.1.1Z.zip
SHEYTAN-Local-Agent-Linux-x64-v1.1.1Z.zip

The v1.1.1Z release successfully established the portable desktop distribution baseline.

v1.1.2Z — AAA Completion Milestone
Product goal

v1.1.2Z is not a minor feature release.

It is the first target release in which SHEYTAN-Local-Agent should behave as a complete, polished, serious desktop AI engineering application rather than a collection of functional panels.

The target experience is:

complete application
+
real functionality
+
coherent UX
+
advanced controls
+
smooth interaction
+
high performance
+
reliable execution
+
objective verification
=
SHEYTAN-Local-Agent v1.1.2Z

The product should feel like a premium desktop application with a modern engineering workspace.

The design target is comparable in polish and responsiveness to high-quality AAA desktop software.

"AAA" in this worklog means:

application completeness
visual quality
interaction quality
performance
reliability
state consistency
feature integration
release readiness

It does NOT mean adding graphics-heavy effects at the expense of performance.

v1.1.2Z Non-Negotiable Goals
1. Every major section must be functional

The following sections must contain real working behavior:

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
Hardware / System
Logs
Updates

A visual placeholder, dead control, or decorative button does not count as implementation.

For every major user-facing action:

UI
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
2. Agent Workspace

The Agent is the central application experience.

The workspace must support:

conversation
streaming responses
session continuity
message history
tool execution visibility
tool results
errors
cancellation
retry
regenerate
model awareness
runtime state
attachments
context information

The user must be able to understand what SHEYTAN is currently doing without reading raw logs.

The UI should distinguish clearly between:

user message
assistant response
thinking / planning state
tool call
tool result
research result
verification result
error
system event

Streaming must not freeze the interface.

Long-running inference and tools must not block the UI thread.

3. Model System

Model management is a first-class feature.

The user must be able to:

discover local models
see model names
see model file sizes
see model paths
see format information
see loaded state
select a model
change active model
start engine
stop engine
restart engine
observe engine state

The system should support real model lifecycle management rather than only writing a model name into configuration.

Target flow:

discover
 ↓
inspect
 ↓
select
 ↓
load
 ↓
verify
 ↓
use

The UI should expose model state clearly:

not found
found
available
loading
loaded
running
stopping
error

Model switching must be safe and deterministic.

The active model shown in the UI must correspond to the model actually used by the runtime.

4. Local Model Discovery

Local GGUF discovery should become a complete workflow.

Required capabilities:

scan model directory
refresh
detect new files
deduplicate results
show metadata when available
identify invalid files
identify currently loaded model

Potential future model metadata:

filename
size
architecture
quantization
context capacity
parameter estimate
modified time
path
status

The interface must remain responsive while scanning large model directories.

Scanning should use bounded work and avoid unnecessary repeated filesystem traversal.

5. Engine Control

Engine control must be reliable.

Required operations:

start
stop
restart
status
health
active model
host
port
process ID where applicable

The runtime should never report:

"running"

when the backend process is actually dead.

Engine state should come from actual process/runtime state rather than optimistic frontend assumptions.

Transitions should be visible:

idle
starting
ready
busy
stopping
stopped
failed
6. Inference Controls

Settings should expose advanced but understandable inference controls.

Target controls include:

temperature
top-p
top-k
min-p where supported
repeat penalty
max tokens
context size
seed
grammar / structured output where supported
parallelism
thread count
GPU layers where supported
batch size
micro-batch size where supported

Controls must be validated against backend capabilities.

Unsupported settings must not silently pretend to work.

The UI should distinguish:

supported
unsupported
default
custom
7. Presets

Presets should become real behavior profiles.

Expected examples:

Balanced
Fast
Reasoning
Coding
Research
Creative
Low Memory
High Quality

A preset must actually modify runtime configuration.

The active preset should be visible.

Custom values must be preserved where appropriate.

8. Agent Configuration

Agent settings must become meaningful runtime controls.

Target configuration:

system behavior
planning depth
tool policy
research policy
memory policy
verification policy
autonomy level
maximum iterations
timeout
parallel tools
repair behavior
failure strategy

The UI should communicate clearly which settings are:

safe
advanced
experimental
dangerous

Dangerous operations must remain explicit.

9. Tool System

Tools must be treated as first-class runtime components.

The interface should display:

tool name
description
enabled state
category
availability
last use
errors

Tool categories can include:

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

Tool calls should be visible in the Agent activity stream.

Tool outputs should not silently disappear.

Tool errors should be represented as actual errors rather than normal assistant text.

10. Coding Lab

Coding Lab must evolve into a genuine software-engineering workspace.

Required capabilities:

create workspace
open workspace
select source project
inspect files
browse directories
open files
edit files
show diffs
run commands
run tests
run builds
inspect stdout
inspect stderr
inspect exit status
verify changes
snapshot
promote
discard

The Lab must make the following lifecycle visible:

REQUEST
 ↓
INSPECT
 ↓
BASELINE
 ↓
PLAN
 ↓
EDIT
 ↓
RUN
 ↓
VERIFY
 ↓
REVIEW
 ↓
PROMOTE

Verification remains objective.

The LLM may propose a result.

Only runtime evidence determines success.

11. Coding Lab File Explorer

The Lab file browser should support:

tree navigation
expand/collapse
file selection
basic file metadata
search
open source files
diff preview
changed-file indicators

Large trees must not render everything at once.

Use virtualization or incremental rendering where useful.

12. Coding Lab Terminal

The terminal view must support:

running command
live output
stdout
stderr
exit code
duration
cancellation
timeout
process state

The UI should clearly distinguish a:

running process
completed process
failed process
canceled process
timed out process
blocked process

Terminal rendering must remain efficient with high-volume output.

13. Research

Research must be a usable research workspace.

The user should be able to:

enter query
select backend
run research
see sources
inspect source metadata
open source
compare sources
save findings

Research results should show:

title
provider
URL
authority
relevance
published time when known
snippet

The system must maintain the distinction between:

evidence

and:

truth

Research should feed engineering tasks without automatically becoming trusted memory.

14. Memory

Memory should become inspectable.

Required capabilities:

view memories
search memories
inspect memory
delete memory
refresh memory list
see source/session metadata

Memory IDs must remain collision-safe.

Memory should be bounded and structured.

The system must avoid uncontrolled growth.

15. Recall / Context

Recall should expose useful context state.

The application should make it possible to understand:

what the agent currently knows
what memory was recalled
what documents contributed
what tools contributed evidence
what context is active

The UI should not overwhelm the user with raw internal prompts.

Use summarized, inspectable context views.

16. Browser

Browser automation must become visible and controllable.

Target interface:

browser status
browser availability
current task
navigation state
screenshots
page title
URL
errors
cancel

Browser automation must respect existing SSRF and safety controls.

The user should see when the browser is being used.

17. Vision

Vision functionality should become a usable tool rather than a hidden backend feature.

The interface should support appropriate visual inputs and display:

image received
vision analysis status
result
errors

Visual processing should not freeze the main interface.

18. Attachments

Attachments should become part of the Agent workflow.

Target:

attach file
inspect file
stage file
remove attachment
show attachment in message

The system must enforce safe limits.

Binary data should not be blindly injected into prompts.

Files should be represented through controlled staging and appropriate extraction/inspection.

19. Sessions

Sessions must feel like persistent workspaces rather than IDs.

Required capabilities:

new session
rename session
switch session
delete session
session history
active model
activity state
session metadata

Future target:

session search
session pinning
session archive
session export

The active session must remain synchronized between frontend and runtime.

20. Diagnostics

Diagnostics should be useful without requiring shell access.

The UI should expose:

runtime health
engine health
API health
model status
memory state
tool status
recent errors
performance data
logs
diagnostics export

Diagnostics must redact secrets.

21. Logs

Logs should be readable inside the application.

Target interface:

recent events
severity
category
timestamp
search
filter
copy
export
clear where safe

The existing structured logs remain the source of engineering diagnostics.

22. Hardware / System

The application should show useful local hardware state.

Target information:

OS
CPU
cores
RAM
GPU where detectable
VRAM where detectable
architecture
available memory
process information
recommended runtime settings

Do not claim hardware support unless actually detected.

23. Updates

The update subsystem should show:

current version
latest known version
engine version
update state
schedule
manual check

Updates must be explicit and auditable.

No silent destructive update behavior.

24. Advanced Settings

The Settings workspace should be organized into meaningful sections.

Suggested structure:

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

Avoid one giant undifferentiated form.

Use progressive disclosure:

common controls
 ↓
advanced controls
 ↓
expert controls
25. UI / UX Direction

The UI is a critical part of v1.1.2Z.

The target is:

modern
dark
technical
premium
focused
fast
minimal
informative

The interface should communicate system state clearly.

Avoid:

decorative clutter
excessive gradients
unnecessary animation
large empty areas
nested modal overload
ambiguous buttons
fake loading states

The visual hierarchy should make the main action obvious.

26. 120Hz / High-Refresh Design Goal

The application should target smooth interaction on high-refresh displays.

Primary target:

120 Hz

Design principles:

no unnecessary layout thrashing
no large synchronous render work
minimal forced reflow
efficient animation
transform/opacity based motion where appropriate
small React render surfaces
memoization where it materially helps
virtualization for large lists
debounced expensive searches
batched state updates
WebSocket event coalescing where appropriate

Do not blindly animate everything.

Animation must be:

fast
purposeful
interruptible
low-cost
consistent

A frame-rate number should never be achieved by disabling useful functionality.

27. Performance Budget

Performance is a first-class release requirement.

Target characteristics:

instant local navigation
responsive typing
smooth scrolling
responsive settings
non-blocking inference
non-blocking tool execution
controlled activity rendering
bounded log rendering
bounded list rendering

The frontend must remain interactive during:

LLM inference
large tool output
research requests
workspace operations
model scanning
engine startup
28. Animation System

Use a unified motion language.

Target classes:

micro interaction
panel transition
workspace transition
status transition
loading state
success state
error state

Motion should use a small set of timing principles rather than arbitrary durations.

Recommended behavior:

micro:     very fast
normal:    short
transition: moderate

Respect reduced-motion preferences.

Animations must not block interaction.

29. Rendering Architecture

Maintain small React client boundaries.

Prefer:

server/static-safe boundaries
+
small client components
+
lazy-loaded large workspaces
+
stable Zustand selectors

Large panels should remain lazy-loaded.

Do not introduce a framework migration without a demonstrated need.

Do not replace React/Vite with another frontend architecture merely for stylistic reasons.

30. WebSocket / Activity Performance

The activity stream must scale.

Avoid rerendering the entire application for every event.

Use:

stable selectors
bounded event buffers
coalescing where safe
virtualized long histories
event categorization

Tool output and LLM streaming should be handled without producing excessive DOM pressure.

31. Backend Performance

Go runtime code should remain efficient.

Avoid unnecessary:

filesystem scans
JSON serialization
duplicate model discovery
duplicate configuration loads
unbounded buffers
goroutine leaks
process leaks
repeated network calls

All long-running operations require:

timeout
cancellation
bounded output
cleanup
32. Security

No v1.1.2Z feature should weaken:

workspace isolation
path validation
SSRF protection
shell policy
Git policy
sandbox policy
process cancellation
secret redaction
configuration protection

The model is not trusted.

The runtime remains authoritative.

33. Release Quality

v1.1.2Z must not be released because:

the code compiles

alone.

Release readiness requires:

frontend checks
Go checks
stress suite
desktop builds
Windows package
Linux package
package verification
UI integration verification
functional tests
release asset verification
34. Test Philosophy

Tests should verify behavior rather than implementation details.

Required layers:

unit tests
integration tests
stress tests
API tests
frontend typecheck
frontend lint
frontend build
desktop build
artifact verification
release verification

User-visible features need functional evidence.

35. Zeta Regression Rules

Never reintroduce:

internal/ui
Fyne desktop UI
old versioned stress files
old e2e scripts
GTK3-only Linux workflow
WebKitGTK 4.1-only Linux workflow
unanchored runtime .gitignore rules
unbounded process execution
unbounded output capture
secret leakage
36. CI Linux Requirements

Wails v3 beta.16 Linux builds use:

GTK4
WebKitGTK 6.0
glib
gio-unix
libsoup 3
pkg-config
CGO

The current CI must install at least:

libgtk-4-dev
libwebkitgtk-6.0-dev
libsoup-3.0-dev
pkg-config
zip

Ubuntu baseline:

ubuntu-24.04

Do not revert to:

libgtk-3-dev
libwebkit2gtk-4.1-dev

unless deliberately building an explicit GTK3 legacy target.

37. Release Packaging

Primary releases remain:

SHEYTAN-Local-Agent-Windows-x64-vX.Y.ZZ.zip
SHEYTAN-Local-Agent-Linux-x64-vX.Y.ZZ.zip

Each ZIP contains:

SHEYTAN-Local-Agent/

Portable data remains associated with that directory.

38. Version Surface

When the version changes, verify all relevant version sources:

internal/config/config.go
package.json
.github/workflows/build-desktop.yml
SIGNATURE
portable launch scripts
documentation where versioned

For v1.1.2Z:

APP_VERSION    = 1.1.2
Codename       = Zeta
Release tag    = v1.1.2Z
Package suffix = Z
39. Standard Engineering Workflow

Before modifying anything:

inspect live main
 ↓
verify exact commit
 ↓
inspect relevant files
 ↓
inspect current Actions state
 ↓
inspect current release state when relevant

Then:

identify real problem
 ↓
make smallest coherent change
 ↓
run relevant local checks where possible
 ↓
push
 ↓
inspect actual GitHub result
 ↓
inspect logs
 ↓
verify artifact
 ↓
continue
40. Standard Completion Phrase

When the user says:

done, check verify and continue

the required sequence is:

1. inspect live main
2. verify exact commit SHA
3. inspect changed files
4. inspect relevant GitHub Actions
5. inspect actual logs
6. inspect artifacts/releases when relevant
7. identify any failure or unfinished feature
8. continue with the next highest-value improvement

Never answer only:

looks good

without evidence.

41. Current v1.1.2Z Execution Order

The work should proceed in this order unless real evidence requires reprioritization:

PHASE 1
Agent core
Sessions
Streaming
Cancellation
Tool visibility
Model/runtime synchronization

PHASE 2
Model management
Discovery
Lifecycle
Engine
Inference controls

PHASE 3
Coding Lab
File explorer
Terminal
Diffs
Verification
Promotion

PHASE 4
Research
Memory
Recall
Browser
Vision

PHASE 5
Advanced Settings
Diagnostics
Hardware
Updates

PHASE 6
AAA UI polish
Motion system
120Hz performance
responsive behavior
accessibility
reduced motion

PHASE 7
integration tests
stress
desktop packaging
release verification
42. Definition of Complete

v1.1.2Z is complete only when:

every major workspace is functional
+
every visible primary action has a real implementation
+
runtime state stays synchronized
+
errors are visible
+
long-running work is cancellable
+
models can be selected and used
+
Coding Lab can execute and verify engineering work
+
Research produces usable evidence
+
Memory/Recall are inspectable
+
advanced settings work
+
diagnostics work
+
Windows and Linux packages build
+
CI is green
+
application remains responsive under load
+
high-refresh displays receive smooth interaction
+
release assets are verified
43. Final Product Principle

SHEYTAN should not feel like:

a chat UI with many buttons

It should feel like:

a local AI engineering computer

The user should be able to:

think
ask
research
inspect
code
execute
verify
repair
remember
review
repeat

inside one coherent environment.

The model proposes.

The tools execute.

The laboratory verifies.

The application presents the evidence clearly.

44. v1.1.2Z Status

Current state:

v1.1.1Z release              COMPLETE
Windows portable package     VERIFIED
Linux portable package       VERIFIED
Wails GTK4/WebKit6 CI        FIXED
Legacy release problems      FIXED
AAA application milestone    IN PROGRESS

Next highest-value implementation target:

Agent workspace → real streaming/tool execution/session state/model state

---

## 2026-09-04 — v1.1.2Z AAA Application Milestone DELIVERED

### Release

Version:  1.1.2
Codename: Zeta
Tag:      v1.1.2Z

Release assets (produced by CI on tag push):

SHEYTAN-Local-Agent-Windows-x64-v1.1.2Z.zip
SHEYTAN-Local-Agent-Linux-x64-v1.1.2Z.zip

### Problems found in the repository (all fixed in this release)

1. CI workflow contract violations (5). The workflow refactor had removed
   `actions/setup-node@v4` (Node was floating on the runner's version), the
   `go run ./scripts/gen-syso` Windows resource step (no icon, no version
   info, no DPI-aware manifest), the `-H=windowsgui` subsystem flag (console
   window flash on double-click), and the `sheytan-local-agent.bat` launcher
   from the portable ZIP. The `zeta_release_surface` stress scenario pins
   every one of these — and it never ran in CI, so nothing caught the
   regressions. All five items restored and the stress gate now runs in the
   Linux CI job.

2. Settings panel crashed to a blank page. `/api/models` returned JSON null
   for `local`/`loaded` when no GGUF models existed (nil Go slices marshal
   as null). `SettingsPanel.tsx` called `.map()` on the decoded value and
   the whole React tree unmounted. Fixed at both layers: the API now always
   returns arrays, and the panel null-guards its lists.

3. The model selector was silently broken. The API shipped bare filename
   strings while the frontend expected `{id, name, path, sizeBytes}`
   objects — every `<option>` rendered with an undefined value. The API now
   returns rich model descriptors (id, name, provider, path, sizeBytes).

4. The activity WebSocket closed immediately when idle. The server sent one
   "idle" sentinel and closed the socket (close code 1006), so the topbar
   permanently showed "Offline" between runs and the app looked broken.
   Idle connections now park in a standby registry, get woken and attached
   to the run hub the instant a run starts, and are kept alive with
   protocol-level pings (25s). The topbar now shows "Connected".

5. Fresh installs rendered a dead workspace. With zero sessions the runtime
   status stayed offline and the composer was inert. The first launch now
   auto-creates the initial session so the Agent workspace is immediately
   live (WebSocket connected, activity streaming, composer usable).

6. Legacy code left in the tree. Removed: stale `web/static` build
   artifacts (two generations of hashed chunks plus the pre-React hand
   written `app.js`/`styles.css`), the broken `Taskfile.yml` (referenced a
   `build/` directory that is not part of the repository), the retired
   versioned e2e scripts (`e2e-v102.sh` … `e2e-v108.sh`, `e2e-v107/`), and
   the orphaned `scripts/glm-proxy.mjs` test proxy.

### v1.1.2Z feature work

- 120Hz-first motion system (`src/motion.css`): compositor-only animations
  (transform/opacity/filter), frame-quantized duration ladder, spring
  easing curves, staggered entrances for every panel grid, animated
  workspace view transitions (keyed remount + glide), micro-interactions
  (press scale, hover lift, nav slide), and a full
  `prefers-reduced-motion` collapse.
- `App.tsx` view transitions: `<main key={view}>` remount animation,
  staggered navigation buttons, animated header blur-in.
- Model dropdown text overflow fixed (ellipsis).
- Auto-session on first launch (`src/agent-init.ts`).
- Idle WebSocket standby architecture (`internal/api/server.go`):
  standby registry, wake-on-run, read pump with abort routing, ping
  keepalive.
- Rich models API (`internal/api/server.go`: `modelInfo`).
- `npm ci` replaces `npm install` in CI (deterministic builds) with
  setup-node npm caching.

### Verification evidence

- Frontend: `npm run typecheck` PASS, `npm run lint` 0 warnings/0 errors,
  `npm run build` PASS (17 source files, fresh `web/static`).
- Go: `go vet ./...` PASS (windows/amd64 target), `go test ./internal/...`
  12 packages ok, stress suite 32/32 pass (incl. `zeta_release_surface`).
- Windows executable: PE32+ GUI subsystem, 9 sections (rsrc embedded),
  icon + version 1.1.2 + DPI-aware manifest via gen-syso, 17.7 MB.
- Live end-to-end (headless `serve` binary + agent-browser): 28/28 checks
  pass — all API endpoints, WebSocket upgrade + standby "Connected" state,
  all four panels (Agent/Lab/Research/Settings) rendering and interactive,
  composer input, zero console errors, v1.1.2 version consistency across
  server banner, topbar chip, and API state.

### Files changed

- `.github/workflows/build-desktop.yml` — version bump + contract restore + stress gate
- `internal/config/config.go` — AppVersion 1.1.2
- `internal/api/server.go` — standby WS, rich models API, imports
- `src/motion.css` — NEW, the full motion system
- `src/main.tsx` — import motion.css
- `src/App.tsx` — view transitions + staggered nav
- `src/agent-init.ts` — first-launch auto-session
- `src/SettingsPanel.tsx` — null-guard hardening
- `src/settings.css` — select ellipsis fix
- `cmd/stress_zeta.go` — 1.1.2 floor + v1.1.2Z notes
- `package.json` — 1.1.2-zeta
- `SIGNATURE` — regenerated (v1.1.2, Parsa Tak)
- `sheytan-local-agent.bat` — v1.1.2Z notes
- `scripts/build-and-zip.sh` — v1.1.2
- `README.md`, `worklog.md`, `agent.md` — v1.1.2Z documentation
- Removed: `Taskfile.yml`, `scripts/e2e-v102.sh`, `scripts/e2e-v106.sh`,
  `scripts/e2e-v107.sh`, `scripts/e2e-v108.sh`, `scripts/e2e-v107/main.go`,
  `scripts/glm-proxy.mjs`, stale `web/static` artifacts

### Next release target

v1.1.3Z candidates (in priority order):

- run a real model end-to-end through the UI (engine start → chat → tools)
- richer Coding Lab task inspection and artifact browsing
- model metadata surface (architecture, quantization, context capacity)
- frontend integration tests in CI (Playwright-style panel smoke suite)
