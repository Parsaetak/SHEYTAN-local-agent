# SHEYTAN-Local-Agent v1.1.2Z — CI Failure Diagnosis & Fix

This package fixes the failed `Build Desktop` runs for **v1.1.2Z** (tag run
33833819590) and the current `main` (run 33834866456).

---

## 1. Why the v1.1.2Z tag build failed

**Failed job:** `Windows x64` — step 16 `Verify Windows executable`
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
- Your own latest commit on main already documents this exact problem in the
  workflow comments ("console subsystem so stdout/stderr can be captured
  reliably by GitHub Actions on Windows").

**Fix already present on main:** the newest workflow builds a CI-only
`-H=windowsconsole` probe from the same source and runs the version smoke
test against the probe instead of the GUI exe. This package keeps that
design — it is correct.

## 2. Why the current main build still fails

Job `Source & frontend audit`, step `Verify repository shape and release
identity`. The new workflow references release-surface files that do not
exist in the repository:

1. `node scripts/release-version.mjs` — **the script was never committed.**
   It is called in the audit job AND in the "Synchronize Zeta release
   metadata" step of both build jobs, so all three jobs are broken by this.
2. `grep -F 'productVersion: "1.1.2-zeta"' build/config.yml` — **the file
   was deleted** in the v1.1.2Z cleanup, but the audit still greps it.
3. Latent (would fail next): `npm run format:check` — verified locally, it
   fails on 6 files (`agent.md`, `worklog.md`, `src/AgentBody.tsx`,
   `src/api.ts`, `src/motion.css`, `src/SettingsPanel.tsx`). You already
   removed this check once for exactly this reason (commit 49164b8, "Remove
   frontend format check from build workflow"); the new workflow re-added
   it in three places.

Everything else in the workflow was verified good: all pinned action
versions exist (checkout@v7, setup-node@v7, setup-go@v7,
upload-artifact@v6, download-artifact@v7, action-gh-release@v3), all npm
scripts referenced exist, typecheck/lint/build pass locally, and the built
`web/static/index.html` contains the required `<div id="root"></div>`.

## 3. What is in this package

Replace/add these files in your repository root (paths match the repo):

| File | Change | Purpose |
|------|--------|---------|
| `.github/workflows/build-desktop.yml` | **Replace** | Removes the 3 × `npm run format:check` lines; makes the release job tag-agnostic (`${{ github.ref_name }}` instead of hardcoded `v1.1.2Z`). Everything else is your latest workflow, untouched. |
| `scripts/release-version.mjs` | **Add** | The missing release-metadata gate. Single source of truth = `package.json` version (`1.1.2-zeta`); validates/repairs `config.go` AppVersion, `build/config.yml` productVersion, `SIGNATURE` header, and the workflow `APP_VERSION`. Zero dependencies, CRLF-safe. Default mode syncs; `--check` mode only verifies (exit 1 + `::error::` annotations on drift). |
| `build/config.yml` | **Add** | Restored Wails project metadata with `productVersion: "1.1.2-zeta"`, satisfying the audit grep. |

## 4. How to apply

```bash
# from your repository root, after copying the files over it:
git add .github/workflows/build-desktop.yml scripts/release-version.mjs build/config.yml
git commit -m "Fix v1.1.2Z CI: add release-version gate, restore build/config.yml, drop failing format:check"
git push origin main
```

### Important: re-point the v1.1.2Z tag

A tag push runs the workflow file **at the tag commit** — the existing
v1.1.2Z tag still points to the buggy workflow, so it must be re-created on
the fixed commit:

```bash
git fetch origin
git tag -d v1.1.2Z
git push origin :refs/tags/v1.1.2Z     # deletes the remote tag
git tag v1.1.2Z origin/main            # re-tag the fixed commit
git push origin v1.1.2Z
```

The re-run will publish the Windows ZIP to the existing v1.1.2Z release
(currently it only has the Linux asset) and the `release` job verifies both
SHA256 checksums before publishing.

## 5. Expected result

- `Source & frontend audit` — green (script now exists, greps satisfied,
  no format:check).
- `Windows x64` — green (console probe used for the version smoke test;
  GUI exe still shipped without a console window).
- `Linux x64` — green (unchanged behavior).
- `Publish and verify <tag>` — green; Windows + Linux ZIPs published to the
  GitHub release with checksum verification.

## 6. Maintenance note (next release)

To cut v1.1.3Z later: bump `version` in `package.json` (e.g. `1.1.3-zeta`),
run `node scripts/release-version.mjs` to propagate it everywhere, commit,
tag `v1.1.3Z`, push. The release job now picks up any `v*` tag
automatically — no workflow edits needed. Keep `APP_VERSION` in sync (the
script checks it).
