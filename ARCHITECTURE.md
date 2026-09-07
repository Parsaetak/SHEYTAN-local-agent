# SHEYTAN-Local-Agent — Architecture Truth Table & Future Direction

> **Status vocabulary used in this document** (see Part III for the rules):
>
> `IMPLEMENTED` · `TESTED` · `PARTIALLY IMPLEMENTED` · `EXPERIMENTAL` ·
> `PLANNED` · `OPTIONAL FUTURE`
>
> A capability is either wired end-to-end and verifiable, or it is clearly
> marked as future/planned. Nothing in Part II of this document is
> implemented today.

This document has two jobs:

1. State **what SHEYTAN actually implements today**, with evidence — so no
   reader (human or AI agent) mistakes a roadmap for a runtime.
2. Record the **validated architectural direction** for SHEYTAN's future AI
   runtime — so future development does not lose the design intent.

Related documents:

| File | Role |
|---|---|
| `README.md` | user-facing overview of the shipped product |
| `agent.md` | engineering handoff for the next AI agent working on this repo |
| `worklog.md` | dated remediation / release history (evidence log) |
| `internal/aicontext/AI-CONTEXT.md` | the runtime briefing prepended to the model's system prompt (a shipped product file, not developer docs) |
| this file | implementation truth table + future architecture |

The core principle that governs both current and future work:

> **The model proposes. The tools execute. The laboratory verifies.**

---

# Part I — What is implemented today

Every row below was verified against source code, tests and the stress
suite at release `v1.1.4Z` (see `worklog.md` for the audit method).
"TESTED" means covered by `go test` packages and/or the 30-scenario
stress suite; see the exact commands in `agent.md` §10.

| Subsystem | Package(s) | Status | Notes |
|---|---|---|---|
| Managed llama.cpp engine lifecycle (download → launch → health → supervise → bounded auto-restart → scheduled updates) | `internal/llm`, `internal/updater` | IMPLEMENTED + TESTED | 3 restarts/episode with 1/2/4 s backoff; downloads context-bounded (10 min) and size-capped (2 GiB) |
| LLM client: streaming, retries, stall watchdog, sampling wire format | `internal/llm` | IMPLEMENTED + TESTED | no overall stream timeout by design; 5-min zero-byte stall watchdog pinned by `TestStreamStallWatchdogAbortsQuietStream` |
| Agent loop (plan → tool calls → observations → verify → answer) | `internal/agent` | IMPLEMENTED + TESTED | **sequential** tool execution; iteration cap (default 25) + per-run time budget (default 60 min) |
| Tool registry (17 tools when all features enabled) | `internal/tools`, `internal/lab`, `internal/research`, `internal/memory`, `internal/sandbox` | IMPLEMENTED + TESTED | `shell`, `files`, `codeExec`, `webSearch`, `git`, `browser`, `dataAnalysis`, `json`, `archive`, `fetch`, `diff`, `screenshot`, `linux`, `coding_lab`, `research`, `memory` + sandbox override |
| Coding Lab (isolated workspace, lexical command policy, verification gates, bounded repair loop, snapshot-before-promote) | `internal/lab` | IMPLEMENTED + TESTED | policy is lexical + env-pinned, not a kernel sandbox (documented limitation) |
| Code-exec sandbox governor (Windows Job Objects) | `internal/sandbox` | IMPLEMENTED + TESTED | memory/CPU configurable, fail-closed default ON |
| Attachments (content-addressed staging, caps, normalization, retrieval with provenance headers) | `internal/attachments` | IMPLEMENTED + TESTED | sha256 staging, symlink-safe |
| Chunking (paragraph-boundary splitting, byte budgets, head+tail windowing, history windowing) | `internal/chunking` | IMPLEMENTED + TESTED | **not** structural/semantic repository chunking — see Part II §4 |
| Context plan (explicit budget: system / tools / recall / attachments / history sections with priorities and pressure) | `internal/contextplan` | IMPLEMENTED + TESTED | the seed of the future budget taxonomy — see Part II §5 |
| Context cache (content-keyed LRU, TTL, bounds) | `internal/contextcache` | IMPLEMENTED + TESTED | |
| Memory (M1–M7 trust classes, persistent JSONL store, search) | `internal/memory` | IMPLEMENTED + TESTED | external material quarantined |
| Recall (BM25 over past turns, recency boost, 👍/👎 feedback steering) | `internal/recall` | IMPLEMENTED + TESTED | feedback wired via `/api/feedback` since v1.1.4Z |
| Continuum chapter rollover | `internal/continuum` | IMPLEMENTED + TESTED (wired post-run) | deterministic `Distill` runs in production; the LLM `Enhance` pass is implemented and unit-tested but has **no production caller** (deliberate future option) |
| Research (auto/GitHub/Reddit/DuckDuckGo/SearXNG, TTL cache, provenance) | `internal/research` | IMPLEMENTED + TESTED | SSRF/alias contracts tested |
| Vision (mmproj projector pairing, image classification, screenshot capture) | `internal/vision`, `internal/screen` | PARTIALLY IMPLEMENTED | pairing logic implemented + unit-tested; not yet exercised with a real projector model (known limitation) |
| Multi-agent pipeline (planner → executor → critic → summarizer) | `internal/multiagent` | PARTIALLY IMPLEMENTED | wired via CLI `ask --multi` only; **not** exposed through the HTTP API or UI; runs **sequentially** on one model |
| Config source (copy-on-write live configuration) | `internal/config` | IMPLEMENTED + TESTED | race-detector-clean; `Source` is the only sanctioned mutation path |
| Observability (logs, rotation, crash reports, diagnostics zip with redaction, perf HUD) | `internal/logging`, `internal/resources` | IMPLEMENTED + TESTED | |
| Sessions (persistence, concurrency, sidecars) | `internal/sessions` | IMPLEMENTED + TESTED | |
| Release engineering (single-source version sync, CI gates, zip-slip-safe updater) | `scripts/release-version.mjs`, `.github/workflows/build-desktop.yml`, `internal/updater` | IMPLEMENTED + TESTED | `package.json` is the single source of truth for the version |
| Frontend (React 19 + TS + Vite, embedded via `go:embed`) | `src/`, `web/static` | IMPLEMENTED + TESTED | `npm run build` must be re-run after frontend changes |

Explicit non-goals of the **current** runtime (do not mistake these for
missing features):

- The agent loop executes tools **sequentially**. Parallel tool execution
  is a deliberate non-goal today.
- The HTTP/WS API is loopback-only with no auth token; the OS user account
  is the trust boundary.
- The Lab command policy is lexical (denylists + env pinning), not a
  kernel-level sandbox.

---

# Part II — The validated architectural direction (PLANNED — NOT IMPLEMENTED)

> **Read this banner literally.** Nothing in Part II exists in the code
> today. These sections record design intent that has been reviewed and
> agreed as the direction for future development. Any statement here must
> never be copied into user-facing capability claims. When a Part II item
> becomes real, move its row into Part I with evidence, and note the
> release in `worklog.md`.

## II.1 — Small, fast, local models are the foundation

SHEYTAN should be designed so a user with modest hardware can run a
capable multi-agent system. The architecture must not assume one giant
model.

Preferred philosophy:

```text
many efficient agents
+ orchestration
+ tools
+ external memory
+ objective verification
```

rather than:

```text
one enormous model
```

Rationale:

- **Lower memory requirements.** Several small quantized models can share
  a machine that could not hold one very large model.
- **Higher concurrency.** Small models iterate quickly, so multiple agents
  can work in parallel within the same compute envelope.
- **Faster agent iteration.** Shorter time-per-token means planner/critic
  style loops stay interactive instead of minutes-per-pass.
- **Practical low-end hardware support.** This is the audience SHEYTAN is
  for: local-first, privacy-first, no datacenter GPU required.
- **Model specialization.** Different agents can use different models —
  a coding-tuned model for the coder role, a cheap fast model for routing
  and classification, a multimodal model only when vision is needed.
- **Better parallel workloads.** N small models × M concurrent agents
  scales on consumer hardware; one large model serializes everything.
- **Reserve strength for exceptional tasks.** A larger or multimodal
  model can be invoked selectively when a task genuinely requires it,
  instead of paying its cost on every turn.

The key architectural claim: **model size is not the sole source of
system intelligence.** Structure (tools, memory, retrieval, verification,
orchestration) contributes at least as much as raw model capacity.
SHEYTAN's intelligence should come from the *system*, not from a single
overwhelming model.

Current state (honest): the runtime loads **one** model per session
(`model` config key) and every run uses that model. There is no
small-models-first composition, no per-agent model assignment, and no
model pool today.

## II.2 — Model tiers and hardware-adaptive routing (PLANNED)

Intended ladder, expressed generically. Tiers describe *capability and
resource cost*, not specific products; the runtime should discover what
is available locally and map models to tiers by measured properties
(parameter count, context length, modality, quantization, tokens/sec).

| Tier | Conceptual class | Intended role |
|---|---|---|
| Tier 0 | smallest local model | ultra-low-resource tasks: routing, classification, extraction, formatting |
| Tier 1 | small local model | default agent turns, cheap tool loops, summarization |
| Tier 2 | medium local model | complex reasoning, planning, code editing |
| Tier 3 | larger local model | difficult local tasks: multi-file refactors, deep debugging |
| Tier 4 | optional high-end (possibly multimodal) model | exceptional reasoning, visual workloads, specialized tasks |

Routing factors the future orchestrator should weigh:

```text
task complexity
modality (text / image / structured)
latency requirement
available hardware (RAM, VRAM, tokens/sec)
context requirement
confidence / retry state
resource budget (battery, thermal, concurrent runs)
```

The architectural principle is **model-agnostic, capability-based
routing**: the system must not hard-depend on any specific model brand or
version. Model names change every few months; the tier abstraction and
routing rules are the durable part.

**Candidate model examples** (examples only — **none of these are
integrated with SHEYTAN**; specs verified against public sources in
2026-09 and will change):

- **Gemma-class** (Google open models): the E2B/E4B "edge" size class
  runs in roughly 2–4 GB of memory and suits Tier 0/1; the 12B–27B class
  suits Tier 2/3; the line offers up to 128K (edge) / 256K (large)
  context windows and multimodal variants, which is interesting for a
  Tier 4 vision agent.
- **GLM-class** (Z.ai open weights): the "Air" class couples a large
  total parameter count with a small *active* parameter count
  (MoE), giving high capability per active FLOP — a candidate for
  Tier 3/4 when hardware allows; recent generations offer 128K–200K
  context windows and strong agentic/tool-use behavior.

These families are named **only as concrete illustrations of the tier
ladder**. SHEYTAN must not make any of them mandatory, and any future
default model choice must be re-verified against authoritative specs at
the time of integration. Small instruct-tuned models from other families
can fill Tier 0–2 equally well.

Current state (honest): there is no tier system, no router, and no
automatic model selection. The user picks one model in the UI.

## II.3 — The Context Engine: external context and chunking (PLANNED)

SHEYTAN's future context architecture must not depend on repeatedly
placing an entire repository or a huge Markdown document into the model
context window. The intended pipeline:

```text
Repository
   ↓
Structural index
   ↓
Semantic index
   ↓
Hierarchical retrieval
   ↓
Context builder
   ↓
Small local model
   ↓
Action
   ↓
Verification
   ↓
Memory update
```

The core mental model:

- **Model context = working memory.** Small, expensive, rebuilt per turn,
  holds only what the current reasoning step needs.
- **External structured storage = project memory.** Large, cheap,
  persistent, queryable, versioned.

The system should be designed around:

- **structural chunking** — chunks derived from document/source structure
  (headings, symbols, blocks), not fixed-size token windows
- **semantic chunking** — chunk boundaries respecting meaning, with
  embedding-assisted grouping where available
- **hierarchical retrieval** — drill down from repo → file → section →
  chunk only as needed
- **metadata-aware retrieval** — filter/rank chunks by path, type, mtime,
  symbol, heading
- **provenance** — every retrieved fragment carries file path + span, so
  edits can be located and audited
- **neighboring context** — retrieval returns parents/siblings, because a
  chunk in isolation is often misleading
- **cross-reference retrieval** — follow links, imports, includes and
  definitions on demand
- **persistent memory** — distilled facts/decisions outlive individual
  sessions (today's `memory`/`recall`/`continuum` are the seed)
- **context budgeting** — a hard allowance per section (Part II §5)
- **artifact-based agent communication** — agents exchange compact
  structured artifacts instead of transcript dumps (Part II §7)

Current state (honest): SHEYTAN has the *seeds* — an explicit context
plan (`internal/contextplan`), a content-keyed cache
(`internal/contextcache`), paragraph-boundary chunking for attachments
(`internal/chunking`), provenance-tagged attachment retrieval, BM25 recall
and continuum rollover. There is **no** repository structural or semantic
index, no hierarchical retrieval, and no context builder that composes
retrieved chunks for a model. "Context Engine" is the name for the future
system that unifies these.

## II.4 — Hierarchical chunk model (PLANNED)

Intended hierarchy:

```text
Repository
  ↓
File
  ↓
Section / Symbol
  ↓
Chunk
  ↓
Exact edit region
```

For Markdown specifically, the chunker should understand: headings and
nested headings, lists, tables, code blocks, links and references, and
definitions — so that a "section" is a real structural section, not an
approximate byte window. For source code, the equivalent is symbols
(functions, types, methods) and their spans.

A chunk should conceptually retain metadata such as:

```text
file path
section path (e.g.  "README.md > Architecture > Context Engine")
chunk ID
parent section ID
previous chunk ID / next chunk ID
token estimate
content hash (for cache keys and change detection)
provenance (how the chunk was produced / last verified)
```

This must never be reduced to naive fixed-token splitting: fixed windows
cut semantic units in half, destroy provenance, and make retrieval
matching noisy.

Current state (honest): `internal/chunking` splits text at paragraph
boundaries under byte budgets, windows long content head+tail, and gives
attachment chunks a stable identity. Chunks carry **no** structural
metadata (no section path, no parent/sibling links, no content hash
index). The hierarchy above is the target Context Engine data model.

## II.5 — Context budgeting (PLANNED extension of an implemented seed)

The future agent should treat context as an explicitly budgeted resource
allocated among:

```text
system instructions
task description
memory (persistent facts, distilled frameworks)
retrieved source (chunks + provenance)
tool output
conversation history
reasoning scratch space
response allowance
```

The goal is not "use as much context as possible" but
**"use the smallest sufficient working set."** Smaller working sets mean
faster inference, more concurrency, less drift, and deterministic
overflow behavior on low-resource machines.

Current state (honest): `internal/contextplan` already implements a real
budget — `numCtx` minus an output reserve, sections for system / tools /
recall / attachments / history with priorities, exact measurement of tool
schemas before windowing, and a visible error on overflow. This is the
implemented seed. The future work is: finer-grained sections (task,
retrieved source, tool output, reasoning), budget policies per agent
role, and budget-aware retrieval (fetch fewer/smaller chunks when the
budget is tight). Extend the existing package; do not invent a parallel
mechanism.

## II.6 — Multi-agent architecture (PLANNED)

Intended shape: a set of specialized, low-cost agents coordinated by an
orchestrator. Example conceptual roles:

```text
planner          decomposes the task
coder            produces edits
researcher       gathers external/internal context
tester           writes and runs checks
debugger         diagnoses failures
documentation    updates docs to match code
reviewer         audits diffs
verifier         runs objective verification gates
```

Agents should not all use the same model. The orchestrator should
eventually assign models per agent according to:

```text
task complexity
modality
latency requirement
hardware
context requirement
confidence in prior attempts
resource budget
failure / retry state
```

Current state (honest):

- The **primary runtime is a single-agent sequential loop**
  (`internal/agent`): one model, tools executed one at a time, iteration
  cap + time budget. Parallel execution is a **deliberate non-goal
  today**.
- A **sequential** planner → executor → critic → summarizer pipeline
  exists (`internal/multiagent`) and is reachable from the CLI
  (`sheytan ask --multi`). It uses a single model and is not exposed via
  the HTTP API or the UI.

Do not claim true parallel multi-agent execution, per-agent model
assignment, or a persistent agent society. Those are Part II futures.
The truth is: sequential single-model pipelines today; specialized,
model-diverse, eventually-parallel agents as the target.

## II.7 — Agent-to-agent communication via structured artifacts (PLANNED)

Future agents should communicate primarily through **structured
artifacts** — files with schemas — rather than by forwarding entire
conversation histories:

```text
Agent A
  ↓ writes
analysis.json   patch.diff   findings.md   test-results.json
  ↓ read + verified by
Agent B
  ↓
verification result
```

Why:

- **Reproducibility** — artifacts are inspectable inputs/outputs; a run
  can be replayed and diffed.
- **Auditability** — the artifact chain is the evidence trail.
- **Token efficiency** — a consumer reads the distilled artifact, not the
  producer's whole transcript.
- **Long-running workflows** — artifacts persist across restarts; agents
  resume from files, not from live RAM.
- **Low-resource operation** — small models can consume compact,
  well-shaped artifacts far more reliably than huge raw histories.

Current state (honest): there is no production artifact protocol between
agents. The closest existing primitives are the Lab's patch export, the
`diff` tool, and JSONL sidecars (sessions, recall, memory). A future
implementation should define the artifact schemas first and treat every
artifact as untrusted input (validate before consume), consistent with
the security invariants in `agent.md` §7.

## II.8 — Document editing architecture (PLANNED)

Markdown and documentation editing are core SHEYTAN workloads. The
intended future workflow:

```text
request
 ↓
intent analysis
 ↓
document search / index
 ↓
relevant section retrieval
 ↓
dependency / cross-reference retrieval
 ↓
context construction (budgeted)
 ↓
model edit proposal
 ↓
structured patch
 ↓
diff inspection
 ↓
Markdown / document validation
 ↓
tests / semantic checks
 ↓
final verification
```

The model is **never the authority** on whether the document was
correctly modified. The repository/tooling must validate:

- syntax (well-formed Markdown / target format)
- structure (heading tree intact, no accidental section deletion)
- references (internal links resolve where testable)
- required sections still present
- forbidden deletions (protected content untouched)
- duplicate headings where they matter
- malformed tables
- unbalanced code fences
- expected architecture statements (when the doc is contract-bearing)
- version metadata consistency (release surfaces)
- generated assets in sync (e.g. `web/static` after frontend changes)

Current state (honest): the primitives exist — `files` tool (read/write
with jail), `diff` tool, Lab verification gates, `release-version.mjs`
for release-surface metadata, and CI checks for embedded frontend and
release identity. There is **no** section-aware retrieval, no structured
document patch format, and no Markdown-structure validator today.

---

# Part III — Documentation truth standard

Every significant capability statement in this repository must fit one
of these categories:

| Label | Meaning |
|---|---|
| `IMPLEMENTED` | wired end-to-end in production code paths |
| `TESTED` | covered by automated tests (unit / HTTP-level / stress) |
| `PARTIALLY IMPLEMENTED` | exists but not wired to every surface (e.g. CLI-only, no UI) |
| `EXPERIMENTAL` | behind a flag / unstable path, may change |
| `PLANNED` | agreed direction, **not** built — must say so explicitly |
| `OPTIONAL FUTURE` | idea kept on record, no commitment |

Rules:

1. Never let a planned capability read as an implemented one. Bad:
   "SHEYTAN runs hierarchical multi-agent memory." Good: "The current
   runtime provides measured context budgets and BM25 recall. The planned
   Context Engine (Part II §3) is intended to add hierarchical retrieval."
2. Resolve contradictions with **source code as the authority**, not by
   copying a claim into more documents.
3. When implementation status changes, update Part I of this file, the
   relevant section of `README.md` / `agent.md`, and add a dated entry in
   `worklog.md` — in the same change.
4. Model names and versions are examples, never foundations. Verify specs
   against authoritative sources at integration time, and label them as
   candidates otherwise.
