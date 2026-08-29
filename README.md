# SHEYTAN™ Local-Agent v1.0.10

> Native Windows desktop AI agent. Go binary + Fyne UI. **SHEYTAN™ is a
> trademark of Parsaetak · © 2024–2026 Parsaetak. All rights reserved.
> Licensed under the Parsaetak Proprietary License v1.1
> (https://github.com/Parsaetak).**
> **No browser UI, no WebView, no Chrome dependency. No LM Studio. No
> Docker. No Linux dependency.** A single .exe launches a native window
> with the fire-themed chat UI, **vision (images + screenshots)**, browser
> automation, data analysis + SVG charts, a built-in Linux-like terminal,
> log catcher, sandbox, multi-agent orchestrator, persistent memory with
> 👍/👎 feedback steering, local GGUF inference — or any remote
> OpenAI-compatible endpoint. **Everything lives inside the app folder —
> fully portable.**

## What's new in v1.0.10 — PRISM: the build fix, four new tools, and leaner session I/O

### 🔧 The v1.0.9 GitHub build error — fixed at the root

The v1.0.9 tag failed CI with `no required module provides package
github.com/sheytan/local-agent/internal/sessions` (and the same for
`internal/sandbox`): both packages existed in the dev tree but were never
committed. v1.0.10 ships both in the repository, and the build script now
compiles the GitHub-zip source tree itself before packaging, so a
source-incomplete release can never ship again.

### 🧰 Four new agent tools

- **`json`** — query & transform JSON/JSONL: dot/bracket path extraction
  with `[*]` array wildcards (`items[*].price`), JSONL row filtering
  (`where` with eq/ne/contains/gt/lt/…), object stats (key frequency,
  value types, depth), recursive key inventory, pretty-print and
  flatten-to-dot-keys. Every action is size-capped and line-oriented.
- **`archive`** — zip/tar/gzip without the shell: create, list, extract.
  All copies are chunk-streamed (1 MB buffers), extraction is
  zip-slip-guarded (`../` entries are rejected), with entry-count and
  total-size caps.
- **`fetch`** — one bounded GET returning readable text: scripts/styles
  stripped, entities decoded, paragraph structure preserved, byte-capped
  (512 KB default, 4 MB max) — the fast middle ground between webSearch
  snippets and the full browser. `mode:"raw"` returns the untouched body.
- **`diff`** — line-level verification (Myers O(ND)): changed regions with
  `-`/`+` lines, context control, similarity summary, and a bounded
  fallback when files differ beyond the edit-distance cap. The agent can
  now PROVE it changed exactly one line.

All four register in the GUI and serve mode alike (same agent surface),
and AI-CONTEXT v10 teaches every model when to reach for them
(webSearch → fetch → files → dataAnalysis is now a first-class recipe).

### ⚡ Leaner session I/O + recall corpus cache

- **The activity feed moved to an append-only sidecar**
  (`sessions/<id>.activities.jsonl`). v1.0.9 appended every milestone
  tool event by rewriting the ENTIRE session JSON — on a long session
  that was the single most expensive write in serve mode. One append now
  costs one small write, O(1) in session size; old inline activities
  migrate on first read; Delete cleans the sidecar too.
- **The recall engine caches its BM25 corpus statistics** (document
  frequencies, average length, per-capsule term vectors) instead of
  re-tokenizing every capsule on every user turn; the cache invalidates
  the moment a new capsule is indexed. Same results, a fraction of the
  per-turn cost.
- **Numeric version assertions**: the stress suite's release checks now
  compare versions numerically — the lexicographic `"1.0.10" < "1.0.9"`
  bug that would have broken every future release-surface test is gone.

### 🎨 Web UI (serve mode) polish

Session message counts render from the meta-index stubs (no full load),
message rendering batches through a document fragment (one reflow
instead of N), long histories skip offscreen layout work
(`content-visibility`), scrolling is smooth, and users who disable
animations get them honored (`prefers-reduced-motion`).

### 📦 Two zips, as always

1. **`sheytan-local-agent-1.0.10.zip`** — the ready-to-run portable app
   (exe + bundled llama.cpp engine + docs + worklog).
2. **`sheytan-local-agent-1.0.10-github.zip`** — the complete source tree,
   no binaries, with `.gitignore` and a GitHub Actions workflow.

## What's new in v1.0.9 — TURBINE: smooth 120fps streaming, the file studio, and a rewritten data engine


### ⚡ Smooth 120fps streaming — the frame-paced pump

Streaming used to poke the widget tree once per coalesced snapshot (about
12 updates a second). v1.0.9 rebuilds the whole streaming path around a
**frame-paced pump** (`internal/ui/pacer.go`): the agent forwards snapshots
at your target frame rate (default **120 FPS** — one update every ~8.3 ms),
and the pacer coalesces them into **at most ONE UI batch per display
frame** — labels, status, refresh and scroll in a single pass, and zero UI
work on frames where nothing changed. Long replies now render at the
monitor's cadence instead of flooding the widget tree, and the status line
shows a **live tokens/sec readout while the text pours in** (the speed HUD
no longer waits until the reply ends). New Settings → **Streaming** section:
toggle smooth streaming and pick the target (60 / 90 / 120 / 144 / 240).

### 🗂 The file studio — create, read, write, combine, and everything between

The `files` tool grew from read/write/list/delete into a complete studio:

- **`combine`** — merge many files into one, ordered, through a chunked
  1 MB stream (constant memory — a multi-GB merge never stalls the app),
  with an optional separator between files.
- **`read` with chunk windows** — `offsetLine` + `maxLines` paginate huge
  files (byte-capped per call), so the agent reads a 100k-line log in
  slices instead of flooding its context.
- **`search`** — regex content search across a file or a folder tree with
  line numbers and a hard hit cap.
- **`replace`** — literal or regex, dry-run counts matches first, the real
  write goes through a temp file + atomic rename.
- **`append`, `copy`, `move`, `mkdir`, `tree`, `info`** — the full daily
  set, all chunk-streamed internally.

### 📊 The data engine, rewritten — parse once, analyze many

- **Zero-copy CSV engine** — RFC-4180 fields are subslices of the source
  (the old parser routed every byte of every cell through a
  strings.Builder); CRLF is handled during the scan (no more full-file
  `\r\n` rewrite copy); rows are presized from a newline census.
- **Parse-once numeric caches** — every numeric column parses at most once
  per dataset; stats, correlation, regression, group-by, histograms and
  outlier scans now read the cached floats. Chained analysis on one file
  is dramatically faster.
- **LRU dataset cache** — the hottest datasets stay resident (recency +
  byte budget) instead of the old wipe-everything-at-16-entries map.
- **Seven new analysis actions** — `regression` (least squares + R²/RMSE +
  prediction), `valueCounts` (frequency tables), `pivot` (2-D group-by
  grids), `dedupe` (with optional cleaned-file write-out), `sample`
  (head/tail/random), `outliers` (IQR + z-score fences), `movingavg`
  (windowed smoothing with series write-out).

### 🧹 Under the hood — every hot path re-audited

- **History windowing is O(n)** — the compactor used to re-prepend the
  whole kept slice per message (O(n²)); a 400-message turn paid ~80k
  struct copies per iteration. It is now one backward pass and exactly
  one slice copy (verified with a 20k-message stress scenario).
- **The SSE pump scans bytes** — no per-line string allocations while
  tokens stream (comments and keep-alives cost nothing).
- **Streaming coalescing tracks the frame target** — emit interval
  derives from the FPS setting (8 ms at 120).
- **Headless `serve` gains the dataAnalysis tool** — the API server now
  exposes the same agent surface as the GUI.
- **Reconstructed packages** — v1.0.9 re-adds `internal/sessions` and
  `internal/sandbox` (the meta-indexed session store and the Windows
  Job-Object sandbox) with the exact contracts the app expects, verified
  by 12 new stress scenarios.
- **+12 stress scenarios (162 total)** covering CSV parity against hostile
  inputs, parse-number fast paths, files-v2 round trips, every new
  analysis action, the numeric cache, O(n) windowing on 20k messages,
  byte-level SSE decoding (content + reasoning + streamed tool-call
  fragments), the session store and sandbox contracts, and the v9 AI
  context. New headless pacer tests cover coalescing, tail delivery and
  live tok/s.

### 📦 Two zips, as always

1. **`sheytan-local-agent-1.0.10.zip`** — the ready-to-run portable app
   (exe + bundled llama.cpp engine + docs + worklog).
2. **`sheytan-local-agent-1.0.10-github.zip`** — the complete source tree,
   no binaries, with `.gitignore` and a GitHub Actions workflow.

## What's new in v1.0.8 — AURORA: the attachment fix, the z.ai/Codex-grade UI, and Parsa Tak's signature

### 🩹 The attachment crash — found and killed

v1.0.7 could close entirely when you clicked **attach**. The root cause:
the built-in Fyne file dialog *walks the filesystem in Go* to list
folders, and on real Windows machines that walker panics on special
locations (network drives, OneDrive virtual folders, empty card readers)
— an uncaught panic in any goroutine terminates the whole app. v1.0.8
replaces the walker with the **OS's own dialog via a raw `comdlg32`
syscall** (`internal/native`): no filesystem walking, instant open, real
multi-select, and it cannot panic — there is no interpreted layer between
the app and the OS. On top of that, the whole UI now runs behind
**panic guards** (`internal/ui/safety.go`): any callback panic is
recovered into a crash log + status note instead of a vanished window.
The same low-level treatment is the v1.0.8 speed story: direct Win32
syscalls, GC tuned for streaming renders, and icon caches.

### 🎨 Aurora Luxe — the UI rebuilt to the z.ai / OpenAI Codex bar

- **The unified pill composer** — one rounded surface holding the input,
  the staged attachment tiles, and the action row (attach · camera ·
  tools · thinking · context meter · send) INSIDE it, the idiom the top
  AI chat platforms converged on. Quiet tiles at rest, warm on hover,
  molten gradient when active.
- **Every stock button is gone** — the ~30 flat gray `widget.Button`s
  across dialogs and toolbars are now **Aurora pills**: primary CTAs with
  painted gradients, layered elevation, glow rings and press motion;
  quiet ghost secondaries with hairline ember edges; a danger variant for
  destructive actions.
- **Modern iconography** — stroke weight raised to the 2025/26 platform
  standard; the send arrow is the ChatGPT-school up-arrow, the paperclip
  and wrench are the clean Lucide geometries, the camera is softer.
- **The app is signed** — under the name **Parsa Tak**. The exe's version
  resource carries the signer (right-click → Properties → Details), the
  About dialog shows the signature line, and a `SIGNATURE` file ships in
  both zips. The licensor remains Parsaetak.

### 📦 Two zips from now on

1. **`sheytan-local-agent-1.0.8.zip`** — the ready-to-run portable app
   (exe + bundled llama.cpp engine + docs + worklog), exactly as before.
2. **`sheytan-local-agent-1.0.8-github.zip`** — the complete source tree,
   no .exe, no engine binaries, no generated artifacts — with `.gitignore`
   and a GitHub Actions workflow so pushing it to a repo rebuilds the exe
   automatically.

---

## What's new in v1.0.7 — CONTINUUM: almost unlimited context, and the Ember Luxe UI

### ♾ Continuum — the context window is no longer a wall

Long conversations used to degrade and die at the context limit. v1.0.7
makes them **chapters**:

- A **live context meter** sits above the composer — you SEE the pressure
  fill (ember → gold → hot), the chapter count, and the token estimate.
  Tap it for the full context panel: usage stats, framework memory size,
  threshold, and a manual **“Extend now”**.
- At the threshold (default 75% of the history budget, configurable in
  Settings → Continuum), the app ends the turn by **distilling the whole
  chapter into a state Framework** — mission, durable facts, decisions,
  open threads, files involved, user preferences, and a rolling summary —
  then **silently creates the next chapter session in the background**,
  seeds it with the Framework briefing plus the most recent messages, and
  swaps it in before you type the next word. You see one slim
  **“Chapter 2 — context extended · memory carried forward”** card and keep
  talking. Every chapter stays small enough to prefill instantly; the
  thread itself effectively never runs out of context.
- **Distillation is layered**: a deterministic, offline, pure-Go extractor
  runs first (instant, zero-cost); when the engine is available a
  background LLM pass *refines* the Framework (rewrites the summary,
  prunes stale threads) before the next turn — a slow or failed pass can
  never block or degrade the rollover.
- **The model is taught to use it**: AI-CONTEXT v7 explains the
  `[CONTINUUM FRAMEWORK]` block to every plugged-in model — treat it as
  your own memory, never re-ask, pick up the open threads. The **Memory
  view** shows the live thread state (mission, facts, decisions, open
  threads) as the first card.
- Chapters chain in the sidebar (**CH 2 · 3h ago · 12 msg**) and stay
  individually browsable; 👍/👎 feedback and recall stay pinned to the
  messages that earned them, even across a rollover.
- Settings → Continuum: enable/disable, threshold slider (50–95%),
  carried-messages slider (0–16), briefing budget. The palette gains
  **“Extend context now”**.

### 🔥 Ember Luxe — every control re-crafted

The whole control surface was rebuilt on a real design system (layered
elevation shadows, glass surfaces with edge light, top-lit hairline
bevels, animated glow rings):

- **True rounded-gradient buttons** — raster-painted (Fyne has no rounded
  gradient primitive, so SHEYTAN paints its own), with press physics:
  the gradient darkens, a soft light flashes, the glow ring lights on
  hover. The send button is now a 38px gradient disc with its own shadow
  and ember ring.
- **Glass chips and cards** — the model chip, composer, suggestion cards,
  and message bubbles are translucent glass with ember edge light;
  suggestion cards carry tinted icon badges from a warm brand rotation.
- **The composer floats** — elevation shadow + hairline bevel make the
  input slab sit ABOVE the page. Thinking/tools toggles become “molten”
  discs when active.
- Consistent radius scale (8/12/16/20) and one material system across
  every panel, dialog, list and card.

### What's new in v1.0.6 — VISION: the model can see, the app runs its own terminal

### 👁️ Multimodal vision — images, screenshots, understood

Drop an `mmproj-*.gguf` projector (e.g. `mmproj-gemma-4-E2B-it-BF16.gguf`)
next to your chat model (e.g. `gemma-4-E2B-it-Q4_K_M.gguf`) and SHEYTAN
**pairs them automatically** — the engine launches with `--mmproj`, the
footer lights a **VISION** badge, and:

- **Attach images** to any message (.png/.jpg/.webp/.gif): they travel as
  real OpenAI-style `image_url` parts the vision encoder actually sees.
  Thumbnails render in the chat; tap one to zoom.
- **The `screenshot` tool** — the agent captures your primary display
  (pure-syscall GDI, no console flash) and *looks at it*: “what's wrong
  with my screen?” is now one question away. The capture lands in
  `logs/screenshots/` and the Files view.
- **The 📷 camera button** in the composer captures your screen straight
  into the message you're typing.
- Smart degradation: without a projector, images become text notes and the
  screenshot tool *explains how to enable vision* instead of failing; with
  a remote vision endpoint (GLM-4V-class), user-attached images still work.
- Performance discipline: images are downscaled to ≤2048px, base64-encoded
  **once** (cached), and only the current turn's images ride the wire —
  older ones degrade to notes so the agent loop never stalls.

### 🖥️ The app's own terminal — a Linux simulator, jailed to the app folder

A real interactive console inside the app (new **Terminal** view): 34
busybox-style commands (`ls -l`, `cd`, `cat`, `grep`, pipes —
`cat file | grep -i error | wc -l` — `find`, `du`, `df`, `tree`, `ps`,
`stat`, `neofetch`…), command history chips, ember prompt coloring. It is
**jailed to the app folder** (`~` = the portable root, nothing outside it
is reachable) and spawns **no real processes** — safe by construction.
The agent gets the *same* environment as the new `linux` tool (shared
working directory), so it always has a dependable scratch shell.

### 👍/👎 Feedback that actually learns

Every assistant reply carries **like / dislike** buttons. Verdicts are
persisted per message *and* folded into the recall engine: answers you
liked rank **+25% higher** in future memory retrieval; disliked ones sink
40%. Your ratings shape how SHEYTAN remembers — without ever growing the
prompt.

### 📊 Resources — a professional panel for space & processing

The new **Resources** view: live **disk usage bars** per folder (models,
sessions, logs, engine, workspace…), **live RAM** of the llama.cpp engine
and the app itself, **engine allocation** controls (generation threads, GPU
layers, context window, multi-agent pipeline depth — applied with one
restart) and **budgets + one-click cleanup** (trim old sessions, rotate
logs, clear the workspace).

### ⌨️ Ctrl+K command palette + UI refresh

The modern navigation surface: **Ctrl+K** opens a searchable palette with
every action (new chat, capture screen, switch model, terminal, resources,
toggles…). Plus: message **timestamps**, image thumbnails with zoom, a
VISION badge, a “See my screen” hero card, and new professional icons
(camera, terminal, gauge, thumbs).

### 🧠 Smarter under the hood

- `mmproj` files are **excluded from the model picker** (they are not chat
  models — picking one was a guaranteed exit-code-1).
- The engine ladder retries **once without the projector** if a broken
  mmproj is the crash cause — text-only beats no engine at all.
- `AI-CONTEXT.md` v6 teaches every model about vision, the terminal, the
  linux tool and feedback semantics.
- `worklog.md` now ships **inside the zip** — the next agent (or
  developer) starts exactly where this release ended.

## What's new in v1.0.5 — Reliability: every model loads, dialogs that fit, a real .exe icon

### 🔧 The compatibility ladder — “exit code 1” model failures are fixed

Fresh model releases (like gemma E2B/E4B-class GGUFs) can make a stock
llama.cpp launch die instantly with a bare “exit status 1” — a chat
template the default parser rejects, a flag the model's attention type
disagrees with, a GPU backend the model crashes on, or an architecture the
bundled engine predates. v1.0.5 replaces the single-shot launch with a
**four-profile compatibility ladder**:

1. **Full speed** — the complete Speed Pack + GPU offload (as before).
2. **Template compat** — adds `--jinja` (a real Jinja parser for modern
   GGUF chat templates).
3. **No speed flags** — drops flash-attention / cache-reuse / batch tuning
   (the historically crashy flags), keeps GPU.
4. **Safe mode** — bare CPU launch that runs on anything.

The winning profile is **remembered per install** (next boots start there
directly), resets automatically after an engine update, and shows as
`compat mode N` in the footer. And when the engine itself says it does not
know the model's architecture, SHEYTAN **auto-updates llama.cpp once and
retries** — brand-new model families become usable without waiting for the
next app release. Failures now surface the engine's **actual stderr** (the
real reason) in a proper dialog instead of a mysterious exit code.

### 🖥️ Panels, tabs and dialogs actually fit now

Two real bugs fixed:

- **Dialog collapse** — every dialog whose content scrolls (Settings, the
  model picker, System info, About, License, Provider…) sized itself to the
  scroller's ~30px minimum and opened as an unusable sliver. All custom
  dialogs now carry explicit generous sizes (clamped to the window), plus a
  minimum-size floor on the scroll content so the class of bug cannot
  return.
- **DPI awareness** — the .exe now ships a **PerMonitorV2 manifest**, so on
  125–150%-scaled laptop displays Fyne finally sees the real scale and the
  whole interface renders at native size instead of a miniaturized 1x. The
  window also auto-fits the actual screen on open, and the Pro dock
  (Context/Params/System/Tools) has a guaranteed 380px minimum width.

### 🔥 A real app icon (and version info) on the .exe

The brand flame — the exact mark the UI uses — is now rendered into the
.exe's resources: a multi-size Windows icon (16→256px) for Explorer, the
taskbar and window listings, plus proper **version info** (product name,
company, copyright, file version) in the Details tab. The manifest also
enables modern Common Controls v6 and long-path support.

## What's new in v1.0.4 — Velocity: no extra terminal, the Speed Pack & the Files view

### 🖥 No more extra terminal window

The #1 annoyance is gone. Every subprocess the app launches — the
llama.cpp engine, `cmd.exe` for the shell tool, `wmic`/`powershell` for
hardware probes, git, python, node, tar — now runs with
`CREATE_NO_WINDOW`, so **no console window ever opens or flashes**. The
launcher `.bat` starts the GUI **detached** and closes itself instantly
(the .exe itself is a pure GUI app — you can double-click it directly).

### ⚡ Speed Pack — the fastest local AI experience (LM Studio competitor)

Research-backed llama.cpp tuning, on by default:

- **Flash Attention kernels** (`--flash-attn`) — straight throughput win
  on every backend.
- **Prompt-cache reuse** (`--cache-reuse 32` + `cache_prompt`) — agent
  turns share a long stable prefix (AI context + tool schemas), so the
  repeated prefill **collapses to near zero after the first turn**:
  dramatically faster time-to-first-token on every follow-up.
- **Split thread pools** — generation on **physical cores** (SMT costs
  5–15% tok/s), prefill on all logical cores (`--threads` /
  `--threads-batch`).
- **Tuned HTTP transport** — pooled keep-alive connections between the
  app and the engine.
- **Speed HUD** — live **tokens/sec + time-to-first-token** in the
  footer after every reply, exactly the telemetry local-LLM users watch
  first.
- Optional (Speed settings dialog): **KV-cache quantization** (q8_0
  halves context memory at <5% speed cost), **mlock** (pin weights in
  RAM), and **speculative decoding** with a draft model (20–50% more
  tokens/sec for same-family pairs).

Direct llama.cpp already beats wrapper apps by 5–20% on identical
hardware — SHEYTAN *is* the engine host, with zero wrapper overhead.

### 🗂 Data management — real model cards

The new **Models** view parses GGUF headers directly (pure Go) and shows
**real model cards**: parameter count, quantization, trained context
length, file size — plus a **memory-fit verdict** against your detected
VRAM/RAM ("Fits GPU VRAM — full GPU speed" / "GPU/RAM split" / "too
big"), per-model actions (Use, Reveal, Copy path, Delete), a folder
storage breakdown, and the Speed Pack switches. The model picker in the
header shows the same cards.

### 📁 Files view — every created file, in the app

Every file the agent creates (reports, charts, scripts, downloads,
screenshots, diagnostics) is detected automatically — tool-agnostic via
turn snapshot diffing — and shown **inside the app**: compact
"Created N files" chips under the chat reply, and a full **Files** view
with big type icons and one-tap actions: **Preview in-app** (text, code,
markdown, images, SVG), **Open with default app**, **Reveal in
Explorer** (file pre-selected), **Copy path**.

## What's new in v1.0.3 — bundled engine, instant start & the polish release

### 🔥 llama.cpp ships inside the app folder — runs clean out of the box

The portable folder now **bundles a current llama.cpp engine build**
(`bin\`, Vulkan + CPU backends). No first-run download, no missing-engine
errors, no internet needed to get started — unzip, drop a model in
`models\`, chat. When a GPU is present, layers are **auto-offloaded via
Vulkan** (NVIDIA, AMD, and Intel all work); without one the engine runs on
CPU. The scheduled updater now correctly tracks upstream builds and swaps
the engine in place (stopping it first — Windows locks running exes).

### ⚡ The engine runs the moment you start chatting

Pick a model (or just type): the engine **boots automatically** with a
visible “Starting engine…” status — the first message no longer fails when
the server wasn't started by hand. Selecting a model **pre-warms** the
engine immediately, so the first reply lands without a cold-start wait.
Cold boots get a generous 3-minute load window (7–8B models on slow disks
genuinely need it), and a port collision or an orphaned engine from a
crashed run is detected and explained (or adopted) instead of failing with
a cryptic bind error.

### 🩹 The fixes you asked for

- **Engine updates actually work.** The updater now walks the upstream
  release list and picks the newest tag that really ships a prebuilt
  Windows binary (the “latest” release can be a source-only milestone tag,
  which 404'd every previous update), falls back to the release Atom feed
  when the GitHub API is rate-limited, and “Check for engine update now”
  ALWAYS checks (it used to silently no-op between scheduled checks).
- **OFFLINE → ONLINE flips fast.** The connectivity probe is now
  multi-strategy (TCP anycast + HTTP connectivity checks + DNS), so
  proxied/filtered networks — where the old raw-socket probe reported
  offline forever — report correctly, and a reconnect is detected within
  seconds (10 s retry cadence while offline).
- **No more phantom “gemma-4”.** The model chip only ever shows a model
  that actually exists in your `models\` folder: the picker lists local
  `.gguf` files only, the first one is auto-selected at boot, and the chip
  reads clean (no `.gguf` suffix). An empty folder shows “Choose model”
  guidance instead of a model you don't have.

### ✨ A more professional, animated UI

- **Hover states everywhere** — nav rows, the model chip, suggestion
  cards, model rows, attachment tiles, and buttons glow with smooth
  120 ms color transitions; the sidebar accent bar **grows** into place.
- **Press feedback** — the send button and composer controls pop on tap;
  the new gradient **“New chat” CTA** brightens on hover and darkens on
  press.
- **Message entrance animation** — new bubbles fade in with a soft
  reveal; the first-run cards stagger in with rhythm; the hero flame sits
  on a warm radial glow.
- **Attachment tiles with big icons** — staged files render as icon-first
  cards: a large type-specific glyph (image, audio, video, archive, code,
  doc, gguf), the name, and a type hint — hoverable, with an inline remove
  ✕.

## What's new in v1.0.2 — attachments, thinking mode & persistent recall

### 📎 Attach files to any chat

A paperclip button in the composer opens the native file picker.
**.txt and .md are 100% supported**, plus every common text/code format
(csv, json, yaml, xml, html, go, py, rs, js, ts, sh, ps1, log, sql, …).
Attached files ride along with your message:

- Text files are **inlined into the message** inside a 256 KB budget
  (configurable) — long files keep their head and tail with an explicit
  elision marker so beginnings and endings always arrive.
- Binary files (pdf, images, archives, …) are attached as metadata notes
  with their path — the agent reads exactly the parts it needs with the
  `files` tool instead of you pasting anything.
- Up to 8 files per message, each shown as a removable chip; sent messages
  display their attachments as chips in the bubble.

### 🧠 Thinking mode (toggle in the composer)

Flip the brain toggle and the agent reasons step by step before answering:

- Works with **any** model: a THINKING MODE instruction makes it externalize
  reasoning in `<think>…</think>` tags; endpoints that stream native
  `reasoning_content` (DeepSeek/GLM style) are captured automatically.
- The reasoning streams live in the status line ("Thinking…") and the Pro
  activity stream, then lands in a collapsible **Thought process** section
  above the answer — kept in the session file, never re-sent to the model.

### 🛠 Tool selection

The composer tools popover (and the Pro dock's Tools tab) lets you choose
exactly which tools the agent may use — per-tool toggles plus presets
(**All** / **Local only** for offline work / **None**). Disabled tools are
never advertised to the model, and if it tries one anyway it receives a
clear "disabled by the user" instruction to re-plan. CLI:
`sheytan ask --tools files,shell "..."`.

### 💾 Persistent recall — past chats without the context bloat

The new recall engine indexes a tiny digest of **every completed exchange**
(~300 bytes: what you asked, the outcome, tools used) into
`recall/index.jsonl`. On each new message it retrieves the most relevant
past exchanges via **BM25 + recency scoring** and injects them as one
bounded block — so weeks of history stay usable *without* re-feeding old
conversations into the context window or RAM:

- The in-memory index holds digests only; session files are never opened by
  the retrieval path (10,000 exchanges ≈ 3 MB).
- One-time **backfill** imports existing sessions on first launch.
- The agent can also search past chats explicitly:
  `memory action=history query="the sales report"`.
- Duplicate exchanges are deduped automatically.

### ⚡ Chunking engine — smooth at any size

All data that crosses a size boundary now flows through one chunking layer:

- **History windowing:** the prompt keeps the most recent turns that fit a
  configurable share of `num_ctx` (default 60%); older turns are compacted
  into a single elision note while their facts remain recallable. Your
  latest message is never dropped.
- **Session meta-index:** the sidebar list reads a compact `sessions/index.json`
  instead of loading every full session file — hundreds of sessions open
  instantly; full histories load only when you click one.
- Attachment head/tail windows and bounded recall blocks (above).

CLI additions: `sheytan ask --think "..."` for a one-shot thinking turn,
`--tools a,b` to restrict tools. 92 stress tests (was 74) + all unit +
headless UI suites green.

## What's new in v1.0.1 — the AI instruction file & performance release

### AI-CONTEXT.md — a complete briefing for every plugged-in AI

Any model you connect — local GGUF or remote API — now receives a full
operating manual as the first system message of every conversation:

- **`AI-CONTEXT.md`** ships in the app folder. It tells the AI where it is
  (your machine, the portable folder map), how the agent loop works, and
  teaches every tool with argument shapes, examples, worked recipes, and a
  failure playbook.
- A **LIVE ENVIRONMENT** block is appended per conversation: OS, CPU/RAM/GPU,
  active provider + model, working directory, connectivity, date, and the
  tool list — so the model never guesses its own surroundings.
- **You can edit it.** Your edits are preserved across restarts; an app
  upgrade that ships newer instructions regenerates the file (version
  marker inside). `sheytan context` prints the effective briefing,
  `sheytan context --reset` restores the canonical file.

### Performance cycle — found & fixed

- **Streaming is coalesced at the source.** v1.0.0 emitted one activity per
  token delta, each carrying the ENTIRE accumulated reply — O(n²) copying
  for an n-token answer, plus a UI refresh and (on the API path) a full
  session-file rewrite per token. Now: at most ~12 updates/second with a
  guaranteed final flush. Long answers stream dramatically lighter.
- **The API server no longer persists streaming deltas** — only milestone
  events (tool start/end, plan, error, done) hit the session file; the
  final text is stored as the assistant message as before.
- **Deterministic tool order.** The tool list sent to the model was built
  from map iteration — a DIFFERENT random order every turn, which silently
  invalidated llama.cpp's prompt-prefix cache and forced a full re-prefill
  of the system prompt each turn. Tools are now sent in stable sorted
  order, so the KV cache (and your first-token latency) survives turns.
- **Pro-mode activity stream renders one live row** for streaming text
  instead of one widget row per token (hundreds of rows per reply before).
- **Sessions save as compact JSON** — roughly half the file size and
  marshal cost of the old indented format (it rewrites on every message).
- **Search regex hoisted** — the DuckDuckGo lite-page parser recompiled its
  regex on every call.

### Libraries updated to latest releases

Fyne v2.8.1, chromedp/cdproto (2026-08-04), golang.org/x/image v0.45.0,
x/net v0.58.0, x/text v0.41.0, fsnotify v1.10.1, go-runewidth v0.0.28,
goldmark v1.8.5, and the rest of the dependency tree — built and re-tested
(74 stress + all unit + headless UI suites) on the new versions.

## What's new in v1.0.0 — the minimal pro release

### The model picker finally works

v0.9 showed your `.gguf` files but tapping one did nothing visible. Three
root causes, all fixed:

- **The engine never reloaded.** llama.cpp loads the model at boot; writing
  `model` into config.json changed nothing for the running server. Picking
  a model now stops and reboots the engine with the new GGUF (or arms it
  for the next start when the engine is off).
- **The dialog never closed and gave no feedback.** The picker now closes
  the instant you tap a model, marks the active one with a check, and the
  status line confirms the switch (`Reloading engine with …` → `Model
  ready`).
- **Bare filenames could not be resolved.** v0.9 passed `model.gguf`
  straight to `--model`, which the subprocess resolved against *its* working
  directory — not the models folder. Model names now resolve robustly:
  exact file, substring, fuzzy tokens ("qwen 7b" finds
  `qwen2.5-7b-instruct-q5.gguf`), or an absolute path.

The model selector is also a **real clickable chip in the header** now —
with a live status dot (green = engine ready, amber = loading, red =
error, gray = off) fed by an engine watcher.

### A dead-click audit across the whole app

- Sessions and charts **deselect after activation** — clicking the same
  item again re-fires instead of silently doing nothing.
- **Provider and preset dialogs close on save/apply.**
- **Attached files can be removed** (previously add-only).
- Start/stop engine report through the status line, not just popups.
- With no models installed, the picker opens a get-started card (open the
  models folder / connect a provider) instead of a dead file list.

### chat.z.ai-style UI — minimal by default, Pro on demand

- **New layout**: sessions sidebar + centered ~780px conversation measure +
  composer with a circular ember send button. User messages are right-set
  tinted bubbles; assistant replies render as clean full-width text under a
  small SHEYTAN marker — the calm reading pattern of chat.z.ai, in the fire
  theme.
- **The main screen is the hero**: flame logo, "What shall we forge
  today?", and four suggestion cards that pre-fill the composer (analyze
  data, research the web, automate the browser, write & run code). First
  run without a model shows a get-started card instead.
- **Minimal mode (default)**: one status line summarizes background work
  (`Thinking…`, `Calling tool: files…`, red **Stop**). Logs, memory, and
  the dock are hidden — the log catcher still writes everything to disk.
- **Pro mode** (sidebar toggle): right dock with Context / Params / System
  / Tools, Memory + Logs views, and the full activity stream. The
  preference persists.
- **Markdown tables render safely** — they are rewritten to monospace
  blocks, because the RichText table renderer paints cells over
  neighboring paragraphs (found by pixel-level visual audit).

### Scheduled library updates — daily / weekly / monthly

The bundled llama.cpp engine now updates itself on a schedule
(`updateSchedule` in config.json, default `daily`):

- Checks the upstream GitHub release, downloads to a staging dir, and
  swaps the binary only after a clean extract — a bad download can never
  break a working engine.
- If the engine is running it is restarted around the swap.
- Fully offline-safe: checks are skipped with a log line and retried on
  the next due tick.
- Visible in Settings → *Engine updates* (schedule radio + last check +
  *Check now*), in the footer status, and via the CLI:

```
sheytan-local-agent update --status
sheytan-local-agent update --force
```

### Engine & internals

- The subprocess cwd is pinned to the app folder (never inherits an
  arbitrary launcher directory).
- The llama.cpp download URL builder now produces the canonical asset name
  (v0.9 had a latent double-`b` that would have failed fresh installs).
- Engine lineage is recorded in `installed.json` (`engineTag`), which the
  updater compares against upstream.

## What's new in v0.9.1 — quality & offline release

### UI overlap bugs — all fixed

The v0.9.0 GUI could visually overwrite itself in several situations. Every
root cause was found, fixed, and locked in with regression tests:

- **Long chat messages no longer paint over each other.** Messages now live
  in a vertical scroll of real bubble widgets, each sized to its own
  content — the old fixed-height list rows let tall bubbles bleed into the
  messages below them.
- **Cross-goroutine UI mutations eliminated.** Every widget update from the
  agent goroutine now runs on the main thread (`fyne.Do`). The old direct
  calls raced the render loop — the real cause of text and panels randomly
  overwriting each other mid-run.
- **The animation fade bug.** Opacity animation compounded alpha
  multiplicatively on every tick, exponentially fading pulsing elements
  (typing dots, view-transition veil, splash text) to invisible. Opacity is
  now applied absolutely; the typing dots and cross-fades stay alive.
- **Pasting a wall of text into the input no longer pushes the layout
  around.** The input sits in a capped scroll area with a fixed height;
  huge pastes scroll inside instead of inflating the panel over the chat.
- **Narrow/minimized windows are safe.** The window has a minimum size, the
  right panel wraps its long labels instead of squeezing the chat column
  shut, and every list row, status label, chart name, and activity caption
  truncates cleanly at its panel edge.
- **The live-agent strip is complete**: flame + typing dots on the left,
  truncated caption, red **Abort** on the right — previously the strip could
  render stale (missing Abort) because Fyne does not re-layout a container
  when only a child's visibility changes.

### Fully functional offline

The app now detects connectivity (an ONLINE / OFFLINE pill in the status
bar) and degrades gracefully instead of hanging:

- **webSearch** fails instantly with a teaching message instead of crawling
  through three engines and a 25s timeout.
- **browser automation** refuses remote URLs while offline (local `file://`
  pages still work).
- **Remote LLM endpoints** skip the whole retry ladder with a clear message
  that suggests switching to the local provider. **Local endpoints on
  127.0.0.1 (llama.cpp, Ollama, LM Studio) are exempt** — they keep working
  offline.
- **llama.cpp bootstrap** explains what to do when the server binary is
  missing and there is no internet (reconnect once, or drop a prebuilt
  `llama-server.exe` into `bin\`).
- **The LLM itself is told** the machine is offline: the orchestrator
  prepends an environment note so the model answers from local tools and its
  own knowledge instead of burning iterations on web calls. The planner /
  critic / summarizer prompts in the multi-agent pipeline get the same note.
- Everything local keeps working with zero network: chat with a local GGUF
  model, files, shell, codeExec sandbox, git, data analysis + charts,
  memory, sessions, logs, diagnostics.

### Hardening

- 58 stress tests (was 52) including six new offline-mode tests, plus new
  headless-GUI regression tests for the layout engine, strip composition,
  and animation opacity.
- Full headless screenshot verification at default (1340×840) and minimum
  (980×620) window sizes, VLM-audited clean.

## What's new in v0.9.0 — the finalization release


### Fully portable — everything in the SHEYTAN-Local-Agent folder

All data now lives **next to the .exe**, so the whole app is one movable,
copyable, backup-able folder:

```
sheytan-local-agent\
├── sheytan-local-agent.exe   ← the app
├── sheytan-local-agent.bat   ← double-click launcher
├── config.json               ← all settings
├── AI-CONTEXT.md             ← the AI's operating manual (editable)
├── worklog.md                ← the full development worklog (v1.0.7)
├── models\                   ← drop .gguf files here (+ mmproj-*.gguf projectors for vision)
├── sessions\                 ← chat history (one JSON per session + index.json)
├── memory\ (memory.jsonl)    ← agent long-term memory
├── recall\                   ← persistent recall: digests of past exchanges + feedback.jsonl
├── logs\                     ← log catcher: app.log, tools.jsonl, llm.jsonl, crashes\, screenshots\
├── charts\                   ← SVG charts rendered by the dataAnalysis tool
├── browser-profile\          ← persistent Chromium profile (logins survive restarts)
├── sandbox\                  ← isolated code-execution workdirs
├── workspace\                ← agent scratch space
└── bin\                      ← bundled llama.cpp server (Vulkan + CPU)
```

- Nothing is written to `%USERPROFILE%`, the registry, or AppData anymore.
- First run automatically **migrates** an old `~/.sheytan` folder (sessions,
  memory, logs, models, config) into the app folder.
- Relative paths used by the agent (files, shell, git, dataAnalysis) all
  resolve against the app folder — chained workflows never break.

### Data analysis tools (new)

A pure-Go data engine — no Python required, runs in-process:

- **Loads** CSV, TSV, and JSON/JSONL datasets with automatic type
  inference (number / string / bool) and missing-value detection.
- **11 actions**: `profile` (shape, types, missing, examples),
  `stats` (count/mean/std/min/q1/median/q3/max/sum),
  `correlation` (Pearson matrix, pairwise-complete),
  `groupby` (count/sum/mean/min/max),
  `filter` (=, !=, >, <, >=, <=, contains, startswith, endswith, in, empty),
  `sort`, `query` (combined select+filter+sort),
  `histogram`, `missing`, `convert` (csv ↔ json ↔ tsv), and `chart`.
- **Fire-themed SVG charts**: bar, line, scatter (with Pearson r), and pie
  with legend — rendered into `charts\` and previewed in the GUI **Data**
  view. Any browser can open the SVGs at full quality.
- Datasets are cached (path+mtime) so chained calls don't re-parse.
- Friendly flat-JSON arg errors teach the model the correct format.

### A GUI that stands out — "Forge Dark"

- **Icon rail navigation**: Chat · Data · Memory · Logs + Provider ·
  License · Settings — 30 hand-drawn, fire-styled SVG icons in one design
  language (1.7px rounded strokes, ember orange), active state animates.
- **Smooth animations everywhere**: boot splash (the flame grows, glows,
  and burns away), pulsing flame + typing-dots indicator while the agent
  runs, breathing ember line under the activity strip, and cross-fade
  transitions between views.
- **Styled chat bubbles**: user vs SHEYTAN cards with ember accent bars,
  markdown-rendered assistant replies (tables, code, bold).
- **Live activity strip**: every tool call streams as a caption with
  timestamps; errors glow red.
- **Data view**: chart gallery with live SVG previews, refresh, and
  open-in-browser.
- **Memory view**: searchable long-term memory browser.
- **Status bar**: provider + model pills, session info, version.
- Right panel tabs with icons: Context · Params · System · Tools.

### Reliable web search — multi-engine

The webSearch tool now tries **DuckDuckGo → DuckDuckGo Lite → Bing** with
bot-block detection (DDG anomaly challenges return HTTP 202 and are
skipped automatically) and decodes Bing redirect links to real URLs. If
every engine is blocked it says so and suggests the browser tool. Verified
live: returns real results for Go 1.26 and example.com queries.

### Correct legal structure

- Copyright holder and licensor: **Parsaetak** (https://github.com/Parsaetak)
- **SHEYTAN™** is the trademark for this app, owned by Parsaetak.
- The shipped `LICENSE` file is generated from the in-app license text
  (they can never drift — enforced by a stress test).

### Tool interoperability hardening

- One canonical base directory for every tool — `files` writes
  `sales.csv`, `dataAnalysis` profiles it, `shell` cats it, `git` commits
  it, all with the same relative path.
- Tool descriptions teach chaining (write CSV → analyze → chart → open).
- Browser session startup is now timeout-bounded (30s) and
  cancellation-aware — a hung Chrome can never block the agent loop.
- End-to-end verified with **GLM as the LLM engine**: 13/13 scenarios
  pass, including files → dataAnalysis → convert chains and
  webSearch + browser combined workflows.

## The full agent stack

| Capability | Details |
| ---------- | ------- |
| **Orchestrator** | Plan → execute → critic loop with streaming activity captions, abort support, max-iteration guard |
| **Multi-agent pipeline** | Planner → executor → critic → summarizer (Agent menu → Multi-agent pipeline) |
| **Tools** | shell · files · codeExec · webSearch · git · browser · **dataAnalysis** · memory |
| **Sandbox** | Windows Job Object: memory cap, kill-on-close, isolated workdirs under `sandbox\` |
| **Memory** | JSONL long-term store with token search; append/delete/clear from GUI |
| **Browser automation** | Persistent profile, 17 human-like actions, structured page understanding, auto-restart, screenshots |
| **Local inference** | llama.cpp server auto-downloaded to `bin\` on first run, GGUF models from `models\` |
| **Remote providers** | Any OpenAI-compatible endpoint (z.ai, OpenAI, OpenRouter, vLLM…) with retry + backoff |
| **Log catcher** | app.log, tools.jsonl, llm.jsonl, crash reports, screenshots, diagnostics zip (redacted) |

## Quick start

1. Unzip anywhere (e.g. `C:\Tools\sheytan-local-agent\`).
2. Double-click `sheytan-local-agent.bat` (or the .exe).
   - The whole folder is the app — move it wherever you like.
3. Pick your engine:
   - **Local**: drop a `.gguf` model into `models\` — the bundled
     llama.cpp engine (already in `bin\`) starts automatically the moment
     you pick a model or send your first message. GPU? It offloads by
     itself. No downloads, no setup.
   - **Remote**: Agent → LLM provider… → enter any OpenAI-compatible
     base URL + API key + model. Test connection lists available models.
4. Ask. Watch the flame.

### Remote provider via environment variables

```bat
set SHEYTAN_PROVIDER=remote
set SHEYTAN_REMOTE_BASE_URL=https://api.example.com/v1
set SHEYTAN_REMOTE_API_KEY=sk-...
set SHEYTAN_REMOTE_MODEL=glm-4.6
sheytan-local-agent.exe
```

### CLI subcommands

| Command | Purpose |
| ------- | ------- |
| `ask "do something"` | Headless agent turn with live activity captions (`--new`, `--session`, `--multi`, `--no-llm-start`, `--think`, `--tools a,b`) |
| `context [--reset\|--path]` | Show / regenerate the AI instruction file (AI-CONTEXT.md) |
| `update [--force\|--status]` | Check / apply llama.cpp engine updates (daily / weekly / monthly / off schedule) |
| `stress` | 100 hostile tests across every subsystem |
| `logs` | Log tail + per-tool stats |
| `diagnostics [out.zip]` | Redacted diagnostics bundle for bug reports |
| `license` | Trademark + full license text |
| `sysinfo`, `doctor`, `serve`, `version` | System probe, health check, legacy web UI, version |

## Where things live (portable layout)

| Data | Path (inside the app folder) |
| ---- | ---- |
| Settings | `config.json` |
| AI instructions | `AI-CONTEXT.md` (editable — prepended to every model's system prompt) |
| Models | `models\` |
| Sessions | `sessions\` |
| Memory | `memory.jsonl` |
| Past-chat recall index | `recall\` |
| Logs & crashes & screenshots | `logs\` |
| Charts | `charts\` |
| Browser profile | `browser-profile\` |
| Sandbox workdirs | `sandbox\` |
| llama.cpp engine (bundled, Vulkan + CPU) | `bin\` |

`SHEYTAN_DATA_DIR` still overrides the root for advanced setups.

## Sampling presets

`precise · balanced · creative · coding · speedrun · exhaustive` — Agent →
Sampling preset…, or `SHEYTAN_LLM_PRESET` env var. Full LM Studio-style
knobs (temperature, top-k/p, min-p, repeat penalty, mirostat, num_ctx,
num_gpu, …) in the Params tab.

## Log catcher (useful data for updates)

- `logs\app.log` (2 MB rotation, 5 generations) — lifecycle + warnings
- `logs\tools.jsonl` — one record per tool call: args, result, error,
  duration, session
- `logs\llm.jsonl` — provider, model, latency, finish reason per call
- `logs\crashes\` — one file per recovered panic with stack trace
- `logs\screenshots\` — browser-tool captures
- GUI **Logs view**: live viewer, per-tool stats, diagnostics export
- Panics are caught, logged, and written to a crash file — never a silent
  exit.

## System requirements (Windows)

- Windows 10/11 x64
- For local inference: ~8 GB RAM (a 4–8B GGUF model), any GPU optional
  (num_gpu layers auto-recommended in the System tab)
- For browser automation: Microsoft Edge (preinstalled) or Chrome — no
  extra install needed on Windows

## License & trademark

```
SHEYTAN™ is a trademark of Parsaetak.
Copyright © 2024–2026 Parsaetak (https://github.com/Parsaetak).
All rights reserved. Licensed under the Parsaetak Proprietary
License v1.1 — see the LICENSE file or Help → License in the app.
```

---

*Built with Go 1.27, Fyne 2.8, chromedp. Stress-tested with 100 hostile
scenarios; capability-tested end-to-end with GLM as the LLM engine.*
