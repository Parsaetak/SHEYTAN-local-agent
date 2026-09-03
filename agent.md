# AGENT.md — Handover Notes for the Next Agent

> Read this file FIRST, then `worklog.md` (session history), then
> `internal/aicontext/AI-CONTEXT.md` (the app's own operating context).

## Repository Identity

```text
Application:  SHEYTAN-Local-Agent
Version:      1.1.0
Codename:     Zeta
Module path:  github.com/Parsaetak/SHEYTAN-local-agent
Author/signer: Parsa Tak (brand.SignedBy) — licensor: Parsaetak
```

Zeta is the consolidated release: one Go runtime, one React frontend, one
desktop shell. The legacy Fyne desktop UI (`internal/ui`, `fyne.io/*`) was
**fully removed** on 2026-09-03. Do not reintroduce it; there is nothing to
migrate back.

## Architecture (current truth)

```text
src/                  React/TypeScript frontend (Vite, Zustand, oxlint)
  │  npm run build → dist/ → scripts/sync-web.mjs → web/static/
  ▼
web/static/           EMBEDDED production assets (committed — CI verifies them)
  │
internal/desktop/     Wails v3 shell: one in-process handler routes
  │                   /api/ + /ws/ to the Go API, everything else to assets
  ▼
internal/api/         Go HTTP API + WebSocket activity
internal/runtime/     runtime stack construction (tools.SetBaseDir lives here)
internal/agent/       orchestrator (LLM loop, tools, activity stream)
internal/tools/       shell/files/fetch/browser/data/lab tools + SSRF guards
internal/lab/         Coding Lab: isolated workspaces, verification, repair
internal/research/    GitHub/Reddit/DuckDuckGo/SearXNG providers + cache
```

Entry points: `main.go` → `main_windows.go` / `main_other.go` →
`internal/desktop.Run`. CLI subcommands live in `cmd/` (serve, ask, stress,
diagnostics, update, license, context).

## Non-negotiable rules

1. **Module path** is `github.com/Parsaetak/SHEYTAN-local-agent`. The old
   `github.com/sheytan/local-agent/...` imports caused a CI failure once;
   `rg "github.com/sheytan/local-agent"` must return only worklog history.
2. **No Fyne.** No `fyne.io/*`, no `internal/ui`. The icon pipeline embeds
   `scripts/gen-syso/logo-512.png` — regenerate that PNG from
   `brand.LogoSVG` only if the brand mark changes.
3. **`web/static` is committed build output.** After frontend changes run
   `npm run build` (tsc -b + vite build + sync:web) and commit the synced
   assets. CI fails if `web/static/index.html` does not carry a
   `type="module"` script.
4. **`.gitignore` patterns that match directories MUST be root-anchored**
   (`/sessions/`, not `sessions/`). `internal/releasegate` fails CI on
   violations — this bug broke v1.0.9/v1.0.10.
5. **Version bumps**: update `config.AppVersion`; the stress check
   `stressV111ReleaseSurface` enforces a `>= 1.1.0` floor and CI workflow
   hygiene (pinned `go-version: "1.26"`, actions pins). SIGNATURE and the
   exe resource metadata derive from `internal/brand` at build time.
6. **Tool jail**: any code that runs tools outside the API server must call
   `tools.SetBaseDir(dir)` first (see `internal/runtime/runtime.go:79` and
   `scripts/stress-main`).

## Build & verify (exact commands)

```bash
# Frontend (Node >= 22.12, npm >= 10)
npm install
npm run typecheck && npm run lint && npm run format:check
npm run build                    # also syncs web/static

# Go — Windows target is the release target (cgo-free)
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build ./...
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go vet ./...

# Go tests (CI: windows-latest; on Linux use the cross-build above for
# internal/desktop, which needs GTK4/WebKitGTK headers natively)
go test ./internal/... -tags headless -count=1
go test ./... -run Test -count=1

# Stress suite (174 scenarios, must end "0 fail")
go build -o /tmp/sheytan-stress ./scripts/stress-main
SHEYTAN_DATA_DIR=/tmp/sheytan-stress-root /tmp/sheytan-stress stress

# Windows resources (icon + version + DPI manifest) — any host
go run ./scripts/gen-syso
```

CI (`.github/workflows/build-windows.yml`) runs: npm install → typecheck →
lint → format → build → embedded-asset verification → gen-syso →
`go test ./internal/... -tags headless` → `go test ./... -run Test` →
`go vet ./...` → exe build (`-H=windowsgui`) → signature probe
(UTF-16 "Parsa Tak" must appear in the exe) → artifact upload; tag pushes
attach the exe to a GitHub release.

## Known state & next steps

- GitHub Actions was red because `internal/ui` survived while its go.mod
  entries did not. Fixed 2026-09-03 (see the last worklog section). The
  next push to `main` should be green end to end; watch run
  "Build SHEYTAN Local Agent — Windows".
- `internal/research` has one inherently network-flaky httptest scenario
  (reddit_test loopback write); it passed 5 consecutive `-count=5` runs.
  If CI ever flakes there, re-run before investigating.
- React UI parity with the pre-migration desktop UI is the main remaining
  feature track (see README "In Progress"): Lab panel maturity, Research
  panel maturity, frontend integration tests.
- `scripts/build-and-zip.sh` produces the portable zip + GitHub source zip
  with integrity gates; it expects `/home/z/my-project/download` and an
  optional llama.cpp engine folder, and degrades gracefully when the engine
  is absent (the app auto-downloads it on first run).

## Where to look when something fails

| Symptom | First place to look |
|---|---|
| CI fails at "Run internal unit tests" | compile errors → stale imports/module path; run the cross-build above |
| exe missing icon/wrong version | `scripts/gen-syso`, `internal/brand`, `config.AppVersion` |
| signature probe fails | `brand.SignedBy`, `gen-syso` CompanyName, winres metadata |
| tool calls error "base directory is not configured" | missing `tools.SetBaseDir` in the embedding host |
| fetch blocked with "not a public IP address" | SSRF guard working as designed; test code may use the tools testhook |
| frontend asset verification fails | re-run `npm run build`, commit `web/static` |
| releasegate fails | unanchored `.gitignore` pattern or missing source package |
