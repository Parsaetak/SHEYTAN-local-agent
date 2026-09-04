# SHEYTAN-Local-Agent v1.1.2Z — CI Failure Diagnosis & Complete Fix

This package fixes the failed `Build Desktop` runs for **v1.1.2Z** (tag run
33833819590), the first fix attempt **v1.1.2Z-fix** (runs 33834866456 and
33885579070), and a latent Linux-job failure that would have surfaced next.

---

## 1. Why the v1.1.2Z tag build failed

**Failed job:** `Windows x64` — step `Verify Windows executable`
("Process completed with exit code 1"). The Linux job was green.

**Root cause:** v1.1.2Z introduced `-ldflags="-s -w -H=windowsgui"` for the
Windows exe (v1.1.1Z built a console-subsystem exe with just `-s -w`). The
verify step then launched this **GUI-subsystem** binary and tried to capture
its stdout to smoke-test the version banner:

    SHEYTAN-Local-Agent v1.1.2
      ...
      os: windows/amd64

A `windowsgui` process gets no console and its stdout cannot be captured
reliably by the GitHub Actions pwsh step, so the captured output was empty,
the expected-version and `windows/amd64` assertions threw, and the step
exited 1. Evidence:

- v1.1.1Z used the **identical** verify step with a console-subsystem build
  and passed.
- Linux (console subsystem) passes the same check on every run.

**Fix (already on main):** the workflow now builds a CI-only
`-H=windowsconsole` probe from the exact same source and runs the version
smoke test against the probe instead of the GUI exe. The shipped GUI exe
stays console-less for end users. Verified locally: the probe prints
`SHEYTAN-Local-Agent v1.1.2` + `os: windows/amd64` and exits 0.

## 2. Why v1.1.2Z-fix still failed (the missing piece)

Job `Source & frontend audit`, step `Verify repository shape and release
identity`, 9 seconds in — `node scripts/release-version.mjs` exits 1:

    [sheytan-release] ERROR: unable to read build/config.yml:
    ENOENT: no such file or directory

**The v1.1.2Z cleanup deleted the whole `build/` directory** — including
`build/config.yml`, the Wails project metadata — but the new audit step and
the release-version gate both require it. The v1.1.2Z-fix commit added the
workflow and `scripts/release-version.mjs` but **`build/config.yml` never
landed in the repo**.

### Why it never landed: the .gitignore trap

`.gitignore` contained a blanket `/build/` pattern (meant for generated
artifacts like `build/icon-preview.png`). Anyone who copies
`build/config.yml` into their working tree and runs `git add .` gets it
**silently skipped** — git refuses ignored paths without `-f`, and the
plumbing `git add .` just moves on. Local builds pass (the file is on disk),
CI fails (GitHub never received the file). This is the same failure class
that dropped `internal/sessions/` in v1.0.9/v1.0.10 — which is why the repo
has the `internal/releasegate` gate in the first place; that gate simply
never walked `build/`.

## 3. The third bug (latent): stress suite drifted from the workflow

`/tmp/sheytan-stress stress` (the `Stress suite` step of the Linux job)
exited 1 — the `zeta_release_surface` contract still demanded the
**pre-v1.1.2Z** workflow shape:

| Stress expectation (stale)           | Workflow reality (v1.1.2Z)                    |
| ------------------------------------ | --------------------------------------------- |
| `actions/checkout@v4` … `@v2` set    | `checkout@v7`, `setup-node@v7`, `setup-go@v7`, `upload-artifact@v6`, `download-artifact@v7`, `gh-release@v3` (Node 20 deprecation fix) |
| literal `node-version: "24"`         | `NODE_VERSION: "24"` env + `${{ env.NODE_VERSION }}` (single source of truth, same pattern as `GO_VERSION`) |
| `Upload Windows ZIP` step names      | `Upload Windows package` / `Upload Linux package` |

The Linux job never got that far in the failed runs (audit fails first), so
this was waiting to break the **next** green attempt.

## 4. What this package changes

| File | Change | Purpose |
|------|--------|---------|
| `build/config.yml` | **Add** | Restored Wails project metadata with `productVersion: "1.1.2-zeta"`, satisfying the audit grep and `release-version.mjs`. |
| `.gitignore` | **Edit** | `/build/` → `/build/*` + `!/build/config.yml`. Generated artifacts stay ignored; the source file becomes committable. Without this, the file vanishes again on the next `git add .`. |
| `internal/releasegate/releasegate_test.go` | **Edit** | The gate now walks `build/` too, so a future `.gitignore` pattern that swallows `build/config.yml` fails `go test ./internal/...` **before** a broken release is pushed. |
| `cmd/stress_zeta.go` | **Edit** | `zeta_release_surface` contract updated to the v1.1.2Z workflow: current action versions, `NODE_VERSION: "24"` env pin, renamed upload steps. |

Everything else is the repository as-is (the workflow itself was already
correct — console probe, tag-agnostic release job, no `format:check`).

## 5. Verified locally (all green)

- `node scripts/release-version.mjs` and `--check` — all four release
  surfaces consistent (`config.go`, `build/config.yml`, `SIGNATURE`,
  workflow `APP_VERSION`).
- Every `test -f` / `grep` in `Verify repository shape and release identity`.
- `npm ci`, `npm run typecheck`, `npm run lint`, `npm run build` — clean;
  rebuilt assets are byte-identical to the committed `web/static/`.
- `web/static/index.html` embed check (`<div id="root"></div>`).
- `go test ./internal/... -tags headless -count=1` — all packages pass
  (releasegate included, with the new `build/` walk).
- `go test ./... -run Test -count=1` — 13 test packages pass.
- `go vet ./...` and full cross-compile for `GOOS=windows CGO_ENABLED=0`
  (the Windows CI path) — clean.
- `go run ./scripts/gen-syso` — writes `rsrc_windows_amd64.syso` with
  version 1.1.2 + DPI-aware manifest.
- Stress suite: **32 pass / 0 fail**, exit 0.
- Version smoke probe (`version` subcommand): prints
  `SHEYTAN-Local-Agent v1.1.2`, `os: linux/amd64`, exit 0 — the Windows
  console probe produces the same output with `os: windows/amd64`.

Only environmental caveat: `internal/desktop` + root `main` need
GTK4/WebKitGTK cgo headers to **compile on Linux** — the workflow's
`Install Linux build dependencies` step provides exactly that on the runner
(and the Windows job compiles them with `CGO_ENABLED=0`, verified here by
cross-compilation).

## 6. How to apply

```bash
# from your repository root, after extracting this zip over it:
git add .gitignore build/config.yml internal/releasegate/releasegate_test.go cmd/stress_zeta.go
git commit -m "Fix v1.1.2Z CI: restore build/config.yml, un-trap .gitignore, sync stress contract"
git push origin main
```

`build/config.yml` is now safely trackable — `git check-ignore
build/config.yml` returns "not ignored" (verified).

### Re-point the v1.1.2Z tag

A tag push runs the workflow file **at the tag commit** — the existing
v1.1.2Z tag still points to the buggy workflow, so re-create it on the fixed
commit:

```bash
git fetch origin
git tag -d v1.1.2Z
git push origin :refs/tags/v1.1.2Z     # deletes the remote tag
git tag v1.1.2Z origin/main            # re-tag the fixed commit
git push origin v1.1.2Z
```

The re-run will publish the Windows ZIP to the existing v1.1.2Z release
(currently it only has the Linux asset); the `release` job verifies both
SHA256 checksums before publishing.

## 7. Expected result

- `Source & frontend audit` — green (config.yml exists and is trackable,
  greps satisfied, no format:check).
- `Windows x64` — green (console probe used for the version smoke test; GUI
  exe still shipped without a console window).
- `Linux x64` — green (unchanged behavior, stress contract back in sync).
- `Publish and verify <tag>` — green; Windows + Linux ZIPs published to the
  GitHub release with checksum verification.

## 8. Maintenance note (next release)

To cut v1.1.3Z later: bump `version` in `package.json` (e.g. `1.1.3-zeta`),
run `node scripts/release-version.mjs` to propagate it everywhere (including
`build/config.yml` now), commit, tag `v1.1.3Z`, push. The release job picks
up any `v*` tag automatically — no workflow edits needed. Keep `APP_VERSION`
in sync (the script checks it).
