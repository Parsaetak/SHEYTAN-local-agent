#!/usr/bin/env bash
# Build + package SHEYTAN-Local-Agent v1.1.0 (Zeta) (Windows-only native desktop GUI)
#
# DUAL OUTPUT (v1.0.8):
#   1. /home/z/my-project/download/sheytan-local-agent-1.1.0.zip
#      The ready-to-run portable app: exe (icon + Parsa Tak-signed version
#      info + DPI manifest), bundled llama.cpp engine, docs, worklog.
#
#   2. /home/z/my-project/download/sheytan-local-agent-1.1.0-github.zip
#      The GitHub-ready SOURCE tree: every line of code, no .exe, no engine
#      binaries, no generated .syso — plus .gitignore and a CI workflow so
#      `git init && push` produces a building repository whose Actions
#      rebuild the exe automatically.
#
# v1.1.0 (Zeta) (GRANITE) — the release that actually builds on GitHub:
#   - ROOT CAUSE of the v1.0.9/v1.0.10 CI failures fixed: unanchored
#     .gitignore patterns (sessions/, sandbox/, ...) matched
#     internal/sessions + internal/sandbox at any depth, so git silently
#     refused to commit them. All runtime-dir patterns are now root-anchored
#     (/sessions/, /sandbox/, ...) and internal/releasegate fails CI if a
#     pattern ever swallows source again.
#   - Memory IDs are collision-proof (timestamp + atomic counter + 4 random
#     bytes) — the old time-only IDs collided on Windows clock granularity
#     and made DeleteByID wipe two entries at once.
#   - TrimLogs/rotateTail no longer renames over an open file (Windows
#     refused the rename; trimming silently freed 0 bytes).
#   - CI workflow: Go pinned to 1.26 (no floating 'stable'), branch triggers
#     repaired, actions upgraded to Node-24 runtimes.
#   - The application remains SIGNED UNDER THE NAME PARSA TAK (exe
#     CompanyName, About dialog, SIGNATURE file in both zips).
set -euo pipefail

cd "$(dirname "$0")/.."

VERSION="1.1.0"
APP_NAME="sheytan-local-agent"
STAGE_DIR="dist-stage/$APP_NAME"
GH_STAGE_DIR="dist-stage/$APP_NAME-github"
DIST_DIR="/home/z/my-project/download"
ENGINE_SRC="/home/z/my-project/engine-dl/vulkan"
ENGINE_TAG="b10642"

# Toolchain paths (v1.1.0 (Zeta) session — see /home/z/my-project/env.sh)
export GOROOT=/home/z/.local/go
export PATH=$GOROOT/bin:/home/z/mingw-root/usr/bin:$PATH
export GOPATH=/home/z/go
export GOFLAGS=-mod=mod
export GOPROXY=https://goproxy.cn,https://proxy.golang.org,direct

# Linux GUI cgo headers (fyne/glfw) from extracted dev debs
export XORG_ROOT=/home/z/xorg-root
export CPATH=$XORG_ROOT/usr/include
export PKG_CONFIG_PATH=$XORG_ROOT/usr/lib/x86_64-linux-gnu/pkgconfig:/usr/lib/x86_64-linux-gnu/pkgconfig
export CGO_LDFLAGS="-L$XORG_ROOT/usr/lib/x86_64-linux-gnu -L/usr/lib/x86_64-linux-gnu"

# Clean + recreate stage
rm -rf "$STAGE_DIR" "$GH_STAGE_DIR"
mkdir -p "$STAGE_DIR"

# Regenerate LICENSE from the in-app brand text (they can never drift).
echo ">> Regenerating LICENSE from brand.LicenseText..."
go run scripts/gen-license.go

# Generate the SIGNATURE block (signed under: Parsa Tak).
echo ">> Generating SIGNATURE (signed by Parsa Tak)..."
go run scripts/gen-signature.go "$VERSION"

# Run stress tests first — don't ship if any test fails (Linux build, no CGO)
echo ">> Building Linux stress-test binary..."
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /tmp/sheytan-stress .
echo ">> Running stress suite (174 tests)..."
if ! SHEYTAN_DATA_DIR=/tmp/sheytan-stress-root /tmp/sheytan-stress stress 2>&1 | tail -3 | grep -q "0 fail"; then
  echo "!! Stress tests failed; aborting."
  exit 1
fi
rm -rf /tmp/sheytan-stress-root
echo ">> All stress tests pass."

echo ">> Running unit tests (Linux, no CGO)..."
if ! CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go test ./internal/sessions ./internal/memory ./internal/chunking ./internal/recall ./internal/tools ./internal/vision ./internal/termshell ./internal/resources ./internal/continuum ./internal/native ./internal/releasegate > /tmp/unittest.log 2>&1; then
  cat /tmp/unittest.log
  echo "!! Unit tests failed; aborting."
  exit 1
fi
echo ">> All unit tests pass."

echo ">> Rendering GUI screenshots (headless verification)..."
if ! CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go test -tags headless ./internal/ui > /tmp/shot.log 2>&1; then
  cat /tmp/shot.log
  echo "!! Headless UI suite failed; aborting."
  exit 1
fi
echo ">> GUI screenshots rendered OK (see internal/ui/shots/)."

# Now switch to Windows cross-compile mode (mingw from extracted debs)
export CC="x86_64-w64-mingw32-gcc --sysroot=/home/z/mingw-root"
export CXX="x86_64-w64-mingw32-g++ --sysroot=/home/z/mingw-root"
export CGO_ENABLED=1
export GOOS=windows
export GOARCH=amd64
export CPATH=
export CGO_LDFLAGS=

# Regenerate the Windows resource object — the app icon (brand flame,
# multi-size), version info (SIGNED BY PARSAT TAK as CompanyName + the
# signature line in Comments + LegalTrademarks), and the PerMonitorV2
# DPI-awareness manifest.
echo ">> Generating Windows resources (icon + Parsa Tak signature + DPI manifest)..."
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go run ./scripts/gen-syso
if [ ! -f rsrc_windows_amd64.syso ]; then
  echo "!! rsrc_windows_amd64.syso was not generated."
  exit 1
fi

echo ">> Building Windows .exe (native GUI)..."
go build -ldflags="-s -w -H=windowsgui" -o "$STAGE_DIR/$APP_NAME.exe" .

# Sanity: the exe must carry the brand strings AND the signature.
if ! grep -q "SHEYTAN" "$STAGE_DIR/$APP_NAME.exe" 2>/dev/null; then
  echo "!! exe looks wrong (brand strings missing)"
  exit 1
fi
if ! python3 -c "
data = open('$STAGE_DIR/$APP_NAME.exe','rb').read()
assert 'Parsa Tak'.encode('utf-16-le') in data, 'signature missing'
assert '$VERSION'.encode('utf-16-le') in data, 'version missing'
"; then
  echo "!! exe signature/version metadata verification failed"
  exit 1
fi
echo ">> exe verified: brand + Parsa Tak signature + v$VERSION present."

echo ">> Vet (windows)..."
go vet ./internal/ui/ ./internal/tools/ ./internal/config/ ./internal/native/ ./internal/sessions/ ./internal/sandbox/ > /dev/null

# Copy launcher + docs + license + AI instruction file + worklog + signature
cp sheytan-local-agent.bat "$STAGE_DIR/"
cp README.md "$STAGE_DIR/"
cp LICENSE "$STAGE_DIR/"
cp SIGNATURE "$STAGE_DIR/"
cp internal/aicontext/AI-CONTEXT.md "$STAGE_DIR/AI-CONTEXT.md"
# v1.0.6+: the worklog ships in the zip so the next agent (or developer)
# picks up EXACTLY where this session ended.
cp /home/z/my-project/worklog.md "$STAGE_DIR/worklog.md"

# --- bundle the llama.cpp engine (Vulkan + CPU backends) into bin/ ---
if [ -d "$ENGINE_SRC" ]; then
  echo ">> Bundling llama.cpp engine ($ENGINE_TAG, Vulkan + CPU) into bin/..."
  mkdir -p "$STAGE_DIR/bin"
  ENGINE_FILES=(
    llama-server.exe
    llama-server-impl.dll
    llama-common.dll
    llama.dll
    ggml.dll
    ggml-base.dll
    ggml-rpc.dll
    ggml-vulkan.dll
    libomp.dll
    mtmd.dll
    LICENSE-LLVM-OpenMP
  )
  # every CPU variant dll
  for f in "$ENGINE_SRC"/ggml-cpu-*.dll; do
    ENGINE_FILES+=("$(basename "$f")")
  done
  for f in "${ENGINE_FILES[@]}"; do
    if [ -f "$ENGINE_SRC/$f" ]; then
      cp "$ENGINE_SRC/$f" "$STAGE_DIR/bin/"
    else
      echo "!! Engine file missing: $ENGINE_SRC/$f"
      exit 1
    fi
  done
  echo ">> Engine bundled: $(ls "$STAGE_DIR/bin" | wc -l) files, $(du -sh "$STAGE_DIR/bin" | cut -f1)."
else
  echo "!! Engine source $ENGINE_SRC not found — download it first:"
  echo "   curl -L -o /home/z/my-project/engine-dl/vulkan.zip https://github.com/ggml-org/llama.cpp/releases/download/$ENGINE_TAG/llama-$ENGINE_TAG-bin-win-vulkan-x64.zip"
  echo "   (extract to /home/z/my-project/engine-dl/vulkan)"
  exit 1
fi

# Pre-create the portable folder skeleton so users see where things go.
mkdir -p "$STAGE_DIR/models" "$STAGE_DIR/workspace" "$STAGE_DIR/charts"

# --- ZIP 1: the full portable app ---
ZIP="$DIST_DIR/$APP_NAME-$VERSION.zip"
mkdir -p "$DIST_DIR"
rm -f "$ZIP"
( cd dist-stage && zip -r -q "$ZIP" "$APP_NAME" )
echo ""
echo ">> ZIP 1 (full app): $ZIP"
ls -lh "$ZIP"

# --- v1.1.0 (Zeta) GATE: the GitHub zip must COMPILE on its own ---
# (v1.0.9 shipped without internal/sessions + internal/sandbox — the CI
# error the user hit. v1.0.10's gate compiled the staged tree but the user's
# `git add` still silently dropped the packages because an UNANCHORED
# .gitignore pattern matched them. This gate now also asserts the two
# packages are physically staged AND that git itself does not ignore them.)
echo ">> Staging GitHub source tree..."
mkdir -p "$GH_STAGE_DIR"
rsync -a --delete \
  --exclude 'dist-stage/' \
  --exclude 'build/' \
  --exclude '*.exe' \
  --exclude '*.syso' \
  --exclude '*.zip' \
  --exclude '/local-agent' \
  --exclude '/build.log' \
  --exclude '/build1.log' \
  --exclude 'rsrc_windows_amd64.syso' \
  --exclude '.git/' \
  --exclude 'internal/ui/shots/' \
  --exclude 'scripts/build-and-zip*.sh.bak' \
  ./ "$GH_STAGE_DIR/"
# Source zips carry docs + signature too.
cp SIGNATURE "$GH_STAGE_DIR/SIGNATURE" 2>/dev/null || true
cp /home/z/my-project/worklog.md "$GH_STAGE_DIR/worklog.md"

# GATE 1 — the critical packages must be physically present in the staged tree.
for critical in internal/sessions/sessions.go internal/sandbox/sandbox.go internal/releasegate/releasegate_test.go; do
  if [ ! -f "$GH_STAGE_DIR/$critical" ]; then
    echo "!! CRITICAL SOURCE MISSING from the staged GitHub tree: $critical"
    exit 1
  fi
done
echo ">> GATE 1 passed: internal/sessions + internal/sandbox + internal/releasegate staged."

# GATE 2 — git itself must neither ignore nor refuse to track the critical
# packages in the tree we ship (mirrors internal/releasegate; this is the
# exact user-side failure mode of v1.0.10: the zips were complete, but the
# user's `git add` silently dropped the packages because the .gitignore
# that shipped INSIDE the zip matched them).
# NOTE: no `grep -q` pipes here — under `set -o pipefail`, grep's early
# exit SIGPIPEs git and turns a PASS into a false FAIL.
if command -v git >/dev/null 2>&1; then
  gate2_ok=1
  ( cd "$GH_STAGE_DIR" && git init -q ) || gate2_ok=0
  if [ "$gate2_ok" -eq 1 ]; then
    # (a) a scratch `git add -A` dry-run must LIST the critical files…
    listed=$( cd "$GH_STAGE_DIR" && git add -A -n 2>/dev/null || true )
    if [ -z "$listed" ]; then
      echo "!! GATE 2 FAILED: git add dry-run listed nothing in the staged tree"
      gate2_ok=0
    fi
    if [ "$gate2_ok" -eq 1 ]; then
      for critical in internal/sessions/sessions.go internal/sandbox/sandbox.go internal/releasegate/releasegate_test.go; do
        case "$listed" in
          *"$critical"*) ;; # git would track it — good
          *)
            echo "!! GATE 2 FAILED: git would NOT track $critical in the shipped tree"
            gate2_ok=0
            break
            ;;
        esac
        # (b) …and check-ignore must not flag it as ignored.
        if ( cd "$GH_STAGE_DIR" && git check-ignore -q -- "$critical" ); then
          echo "!! GATE 2 FAILED: the shipped .gitignore ignores $critical"
          gate2_ok=0
          break
        fi
      done
    fi
  fi
  rm -rf "$GH_STAGE_DIR/.git"
  if [ "$gate2_ok" -ne 1 ]; then
    echo "   (this is the exact v1.0.9/v1.0.10 bug — anchor runtime dirs with a leading '/')"
    exit 1
  fi
  echo ">> GATE 2 passed: git tracks and does not ignore the critical packages."
fi

# GATE 3 — the staged tree must COMPILE on its own.
echo ">> GATE 3: compiling the GitHub source tree (Linux, full GUI build with cgo)..."
# The gate must mirror a real user/CI build: EVERY package, including the
# Fyne GUI (which needs cgo + the X11/GL headers). The mingw cross-compile
# env is overridden back to the native Linux toolchain for this step.
if ! ( cd "$GH_STAGE_DIR" && \
    GOOS=linux GOARCH=amd64 CGO_ENABLED=1 \
    CC=gcc CXX=g++ \
    CPATH="$XORG_ROOT/usr/include" \
    PKG_CONFIG_PATH="$XORG_ROOT/usr/lib/x86_64-linux-gnu/pkgconfig:/usr/lib/x86_64-linux-gnu/pkgconfig" \
    CGO_LDFLAGS="-L$XORG_ROOT/usr/lib/x86_64-linux-gnu -L/usr/lib/x86_64-linux-gnu" \
    go build ./... 2> /tmp/gh-compile.log ); then
  cat /tmp/gh-compile.log
  echo "!! The GitHub source tree does not compile — refusing to ship."
  exit 1
fi
echo ">> GATE 3 passed: the source tree builds clean (all packages, GUI included)."

# GATE 4 — the sealed zip itself must contain the critical packages.
# (No grep -q pipes: under pipefail, grep's early exit SIGPIPEs unzip and
# a successful match would look like a failure.)
GH_ZIP="$DIST_DIR/$APP_NAME-$VERSION-github.zip"
rm -f "$GH_ZIP"
( cd dist-stage && zip -r -q "$GH_ZIP" "$APP_NAME-github" )
zip_listing=$( unzip -l "$GH_ZIP" 2>/dev/null || true )
for critical in "internal/sessions/sessions.go" "internal/sandbox/sandbox.go" "internal/releasegate/releasegate_test.go"; do
  case "$zip_listing" in
    *"$critical"*) ;; # present in the sealed zip — good
    *)
      echo "!! GATE 4 FAILED: $critical is not inside the sealed GitHub zip."
      exit 1
      ;;
  esac
done
echo ">> GATE 4 passed: the sealed zip carries the critical packages."
echo ""
echo ">> ZIP 2 (GitHub source): $GH_ZIP"
ls -lh "$GH_ZIP"

# Guard: the GitHub zip must NOT contain any exe/dll/syso (or the
# extensionless Linux build binary).
if unzip -l "$GH_ZIP" | grep -Ei '\.(exe|dll|syso)$|/local-agent$' | grep -q .; then
  echo "!! GitHub zip contains binaries — refusing to ship:"
  unzip -l "$GH_ZIP" | grep -Ei '\.(exe|dll|syso)$|/local-agent$'
  exit 1
fi
echo ">> GitHub zip verified: pure source, zero binaries."

echo ""
echo ">> Done. Outputs:"
ls -lh "$ZIP" "$GH_ZIP" | awk '{print "   " $NF " (" $5 ")"}'
