# SHEYTAN-Local-Agent — Agent Context

## Project

Repository:

https://github.com/Parsaetak/SHEYTAN-local-agent

Product:

**SHEYTAN-Local-Agent**

Current release:

**1.1.0 — Zeta**

Core principle:

> The model proposes. The tools execute. The laboratory verifies.

This repository is a local-first autonomous AI software-engineering laboratory and agent.

The system is expected to become a practical desktop application, not a demonstration UI.

---

# 1. Non-negotiable development rules

When working on this repository:

1. Inspect the live GitHub repository before making assumptions.
2. Inspect the relevant GitHub Actions workflow and its latest run.
3. Inspect actual workflow logs when diagnosing failures.
4. Never assume a pushed change worked just because a commit exists.
5. After changes, run the relevant checks and verify the resulting state.
6. Preserve the current architecture unless there is a demonstrated reason to change it.
7. Prefer small, stable, testable changes over unnecessary rewrites.
8. Do not reintroduce deleted legacy architecture.
9. Keep frontend rendering efficient and avoid unnecessary dependencies.
10. Provide complete replacement files when asking a human to replace code.
11. Never put Markdown code fences inside YAML workflow files.
12. Never perform repository formatting writes inside CI merely to make checks pass.
13. Secrets must never be exposed in frontend responses or committed configuration.
14. Do not claim success without evidence.

---

# 2. Current architecture

## Backend

Language:

Go 1.26

Desktop runtime:

Wails v3

Main runtime:

`internal/runtime`

API:

`internal/api`

Configuration:

`internal/config`

LLM integration:

`internal/llm`

Agent:

`internal/agent`

Tools:

`internal/tools`

Memory / recall:

`internal/memory`

Coding Lab:

`internal/lab`

Research:

`internal/research`

Browser / vision integrations:

as implemented under the corresponding runtime/tool packages.

---

# 3. Frontend

The current production desktop UI is:

- React
- TypeScript
- Vite
- Zustand
- Lucide / lightweight icon usage
- embedded under `web/static`
- served by the Go/Wails runtime

The active desktop path is:

`internal/desktop`

The old Fyne UI:

`internal/ui`

was deliberately removed.

Do not recreate it.

Do not import it.

Do not restore stale Fyne dependencies.

---

# 4. Frontend architecture

Main application:

`web/src/App.tsx`

Current workspace layers:

- Agent
- Coding Lab
- Research
- Settings

Workspace routing is hash-based.

Settings must remain a lazy-loaded workspace so the main Agent surface stays fast.

---

# 5. User experience contract

Version 1.1.0 must not behave like a minimal proof-of-concept.

The desktop user must be able to control the major runtime behavior from the application itself.

At minimum the application must expose:

## Model control

Users must be able to:

- see detected local GGUF models;
- choose the active local model;
- see which model is currently active;
- start the local inference engine;
- stop the local inference engine;
- see whether llama.cpp is running;
- see loaded models;
- refresh model state.

A fake model dropdown is unacceptable.

Model selection must actually change the backend configuration and runtime.

For the current architecture, global runtime model selection is intentional because the agent orchestrator currently resolves the active model from shared configuration.

Do not mutate global configuration on a per-session basis unless concurrency is designed and verified first.

Future per-session models must be implemented through an explicit concurrency-safe mechanism.

---

# 6. Settings workspace

Settings must be a real product surface, not a placeholder.

The Settings workspace should expose the major runtime categories.

Required categories include:

## Provider

- Local provider
- Remote provider
- local model
- remote base URL
- remote model
- local LLM base URL
- model directory

Remote API secrets must remain backend-owned and redacted.

Never render an API key received from a backend configuration response.

---

## Sampling

Expose the important LLM controls including:

- temperature
- top-p
- top-k
- min-p
- max tokens
- repeat penalty
- seed
- context size
- batch size
- GPU layers
- CPU threads
- preset

---

## Presets

Expose the backend presets.

Preset data comes from:

`GET /api/presets`

Preset selection must update the actual runtime configuration.

---

## Agent behavior

Expose:

- maximum iterations
- parallel tools
- verbose agent events
- thinking mode
- memory recall
- recall top-k
- history window
- continuum context

---

## Performance

Expose:

- target FPS
- smooth stream
- performance HUD
- GPU auto offload
- flash attention
- batch size
- GPU layers
- threads
- relevant engine tuning controls

The frontend must not implement an expensive frame loop merely to display a target FPS.

Target FPS is a runtime/UI scheduling target, not permission to waste CPU.

---

## Tools

The user must be able to see available tools and enable/disable them.

Tool names and descriptions must come from:

`GET /api/tools`

Do not hard-code a fake tool inventory in the frontend.

---

## Browser

Expose:

- browser executable
- headless mode
- slow-motion timing

---

## Vision

Expose:

- vision enable/disable
- multimodal projector path

---

## Sandbox

Expose:

- sandbox enabled
- memory limit
- CPU limit

---

## Coding Lab

Expose:

- lab enabled
- workspace root
- command timeout
- lab iteration limit
- keep workspaces
- network permission

---

## Research

Expose:

- research enabled
- backend
- SearXNG URL
- maximum results
- timeout
- GitHub research
- Reddit research
- general web research

---

## Hardware diagnostics

The Settings workspace should display available system information from:

`GET /api/sysinfo`

Where available, show:

- CPU
- logical threads
- RAM
- free disk
- GPUs
- recommended context
- recommended batch
- hardware warnings

---

# 7. Configuration API

The backend already owns runtime configuration.

Frontend configuration code must use:

`GET /api/config`

and:

`PUT /api/config`

Configuration writes must be merge patches, not destructive full-object replacements.

Secrets must be preserved by backend redaction logic.

When changing model or engine-sensitive settings, the UI may stop/start llama.cpp so the new configuration becomes active.

Do not silently fake a runtime change.

---

# 8. llama.cpp model switching

The runtime starts the configured model using the configured model path.

Changing:

`cfg.Model`

and restarting llama.cpp is the correct current mechanism for global local-model switching.

Expected UI flow:

1. User selects model.
2. Frontend sends `/api/config` with `{ "model": "..." }`.
3. Frontend stops llama.cpp when necessary.
4. Frontend starts llama.cpp.
5. Frontend refreshes `/api/models`.
6. UI reflects the new active state.

Errors must be visible to the user.

---

# 9. Sessions

Sessions are persistent.

The UI should continue to support:

- create session
- delete session
- active session
- session list
- session metadata

Future improvements should include:

- rename
- archive
- search
- richer context controls
- attachment management
- per-session preferences where concurrency-safe

Do not break existing session persistence.

---

# 10. Release packaging

The primary user distribution must be **portable ZIP packages**.

There must be exactly two primary desktop packages:

1. Windows x64
2. Linux x64

Expected names:

`SHEYTAN-Local-Agent-Windows-x64-v1.1.0Z.zip`

`SHEYTAN-Local-Agent-Linux-x64-v1.1.0Z.zip`

Each archive must contain a single top-level directory:

`SHEYTAN-Local-Agent/`

The executable, runtime data, models directory, sessions, workspace, logs, configuration, and supporting files must remain together.

The user should be able to:

1. extract the ZIP;
2. open the `SHEYTAN-Local-Agent` folder;
3. run the application;
4. continue using the same folder later.

Do not make raw standalone executables the primary release assets.

Raw executables may exist temporarily as workflow outputs when useful for verification, but releases should publish the ZIP packages as the primary downloadable artifacts.

Portable data must be relative to the application root unless an explicit user configuration overrides it.

---

# 11. Portable directories

The portable distribution should preserve directories such as:

- `models/`
- `sessions/`
- `workspace/`
- logs/runtime data as implemented by the backend

Documentation inside the package should explain where models and workspaces belong.

Never require the user to reconstruct the application directory layout manually.

---

# 12. CI requirements

The main desktop workflow should build:

- Windows x64
- Linux x64

Frontend validation:

- `npm install`
- `npm run typecheck`
- `npm run lint`
- `npm run format:check`
- `npm run build`

Then verify:

`web/static/index.html`

exists.

Backend validation:

- Go tests
- Go vet
- platform executable smoke test

CI must fail if the executable was not actually produced.

CI must fail if the generated frontend was not embedded.

CI must fail if the ZIP was not produced.

---

# 13. GitHub Actions safety

Never commit:

inside `.github/workflows/*.yml`.

YAML files contain YAML directly.

Do not ask CI to run Prettier in write mode.

Use:

`npm run format:check`

for verification.

Formatting changes should be created by development tooling and committed intentionally.

---

# 14. Frontend performance

Performance is a product feature.

Target smooth UI behavior around 120 FPS where hardware permits.

Rules:

- avoid unnecessary React state updates;
- avoid giant global rerenders;
- use Zustand selectors carefully;
- lazy-load workspace-specific surfaces;
- do not poll aggressively;
- do not recreate expensive objects every render;
- batch updates where possible;
- keep activity-stream rendering bounded;
- avoid heavy animation libraries when CSS can do the work;
- do not add dependencies without a measurable reason;
- do not sacrifice application responsiveness for decorative effects.

The Agent workspace must remain usable while long-running model/tool activity is occurring.

---

# 15. Activity stream

Activity events arrive through the WebSocket API.

The client should:

- connect once for the active session/runtime;
- reconnect when appropriate;
- disconnect on unmount;
- avoid duplicated sockets;
- avoid unbounded DOM growth;
- render long-running activity efficiently.

---

# 16. API correctness

Frontend TypeScript types must mirror backend JSON field names.

Do not invent a camelCase API schema when the backend exposes another shape.

When changing backend JSON structures:

1. inspect the Go structs/handlers;
2. update frontend interfaces;
3. update API methods;
4. update UI behavior;
5. run typecheck/build;
6. verify the live API.

---

# 17. Security

Do not expose:

- remote API keys
- secrets
- private credentials
- internal tokens

The `/api/config` endpoint intentionally redacts the remote API key.

Preserve this behavior.

Do not add localStorage persistence for secrets.

Do not place secrets in frontend build-time environment variables unless explicitly designed and documented.

---

# 18. Agent runtime

The agent runtime is authoritative.

Do not implement frontend behavior that claims an agent capability which the backend does not actually execute.

The UI should expose real capabilities already implemented by:

- orchestrator
- tools
- LLM runtime
- Coding Lab
- Research
- memory
- recall
- browser
- vision

When adding a new UI control:

1. identify its backend configuration/runtime behavior;
2. verify the API path;
3. connect the control to the real implementation;
4. verify the end-to-end effect.

---

# 19. Coding Lab

The Coding Lab is an execution and verification environment.

The laboratory must preserve the principle:

> Execute, verify, and repair.

Do not turn Coding Lab into a simple command console.

Workspace isolation and verification state are important.

---

# 20. Research

Research is evidence-oriented.

The UI should distinguish:

- query
- provider
- source
- authority
- result
- published date
- match score

Research settings must not imply that an unavailable provider is operational.

---

# 21. Testing workflow

For every development cycle:

## Before

Inspect:

- live repository
- relevant files
- current branch
- Actions
- latest successful/failed run
- release state when packaging is involved

## During

Make the smallest coherent change.

Prefer complete file replacements for manually applied frontend changes.

## After

Run:


npm install
npm run typecheck
npm run lint
npm run format:check
npm run build
go test ./...
go vet ./...

using platform-appropriate environment requirements.

Then verify:

generated frontend exists;
desktop executable exists;
application smoke test works;
ZIP contains the expected top-level folder;
ZIP opens without corruption;
release assets are correct;
no unwanted raw executable is being presented as the primary release artifact.
22. Standard completion phrase

When the user says:

done, check verify and continue

Do not merely acknowledge it.

Inspect the actual current GitHub state.

Verify the code.

Verify Actions.

Verify artifacts/releases.

Identify remaining problems.

Then continue with the next highest-value improvement.

Never assume the previous change worked.

23. Current product priorities

Priority order:

reliable portable Windows/Linux ZIP distribution;
usable Settings workspace;
real model selection and engine control;
richer Agent interaction;
session management improvements;
attachments/file staging;
Coding Lab maturity;
Research maturity;
diagnostics/performance telemetry;
broader integration and end-to-end testing.

The application should progressively become a practical autonomous engineering workstation rather than a minimal chat shell.

24. Definition of done for user-facing features

A user-facing feature is not complete merely because:

a button exists;
a setting appears;
TypeScript compiles;
a commit exists.

It is complete only when:

the UI exposes the feature clearly;
the frontend calls the real API;
the backend performs the intended action;
state is refreshed;
errors are visible;
the relevant tests/checks pass;
CI passes;
the resulting artifact contains the feature.
25. Design principle

SHEYTAN-Local-Agent should feel like a serious local AI engineering environment.

The user should not need to edit config.json manually for normal operation.

Common tasks should be possible directly inside the application:

choose a model;
start/stop the engine;
adjust inference;
configure tools;
configure agent behavior;
configure browser/vision;
configure Coding Lab;
configure Research;
inspect hardware;
manage sessions;
run tasks;
inspect execution;
verify results.

The application should make the correct action easy and the dangerous action explicit.

using platform-appropriate environment requirements.

Then verify:

generated frontend exists;
desktop executable exists;
application smoke test works;
ZIP contains the expected top-level folder;
ZIP opens without corruption;
release assets are correct;
no unwanted raw executable is being presented as the primary release artifact.
22. Standard completion phrase

When the user says:

done, check verify and continue

Do not merely acknowledge it.

Inspect the actual current GitHub state.

Verify the code.

Verify Actions.

Verify artifacts/releases.

Identify remaining problems.

Then continue with the next highest-value improvement.

Never assume the previous change worked.

23. Current product priorities

Priority order:

reliable portable Windows/Linux ZIP distribution;
usable Settings workspace;
real model selection and engine control;
richer Agent interaction;
session management improvements;
attachments/file staging;
Coding Lab maturity;
Research maturity;
diagnostics/performance telemetry;
broader integration and end-to-end testing.

The application should progressively become a practical autonomous engineering workstation rather than a minimal chat shell.

24. Definition of done for user-facing features

A user-facing feature is not complete merely because:

a button exists;
a setting appears;
TypeScript compiles;
a commit exists.

It is complete only when:

the UI exposes the feature clearly;
the frontend calls the real API;
the backend performs the intended action;
state is refreshed;
errors are visible;
the relevant tests/checks pass;
CI passes;
the resulting artifact contains the feature.
25. Design principle

SHEYTAN-Local-Agent should feel like a serious local AI engineering environment.

The user should not need to edit config.json manually for normal operation.

Common tasks should be possible directly inside the application:

choose a model;
start/stop the engine;
adjust inference;
configure tools;
configure agent behavior;
configure browser/vision;
configure Coding Lab;
configure Research;
inspect hardware;
manage sessions;
run tasks;
inspect execution;
verify results.

The application should make the correct action easy and the dangerous action explicit.

One important correction before you replace the workflow: the repository structure currently places the frontend under `web`, so the workflow above intentionally uses `web/package.json` rather than assuming the frontend package lives at the repository root. That distinction should remain consistent with the live repo.
