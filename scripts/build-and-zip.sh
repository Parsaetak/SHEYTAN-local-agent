#!/usr/bin/env bash
# Build + package SHEYTAN-Local-Agent v1.0.9 (Windows-only native desktop GUI)
#
# DUAL OUTPUT (v1.0.8):
#   1. /home/z/my-project/download/sheytan-local-agent-1.0.9.zip
#      The ready-to-run portable app: exe (icon + Parsa Tak-signed version
#      info + DPI manifest), bundled llama.cpp engine, docs, worklog.
#
#   2. /home/z/my-project/download/sheytan-local-agent-1.0.9-github.zip
#      The GitHub-ready SOURCE tree: every line of code, no .exe, no engine
#      binaries, no generated .syso — plus .gitignore and a CI workflow so
#      `git init && push` produces a building repository whose Actions
#      rebuild the exe automatically.
#
# v1.0.9 (TURBINE) highlights baked into this build:
#   - Smooth 120fps streaming: the frame-paced pump coalesces UI updates to
#     the display cadence, with a live tok/s readout while tokens stream.
#   - The file studio: combine / chunked read / search / replace / tree /
#     info / append / copy / move / mkdir — all chunk-streamed internally.
#   - The data engine rewrite: zero-copy CSV parsing, parse-once numeric
#     caches, LRU dataset cache, O(n) history windowing, byte-level SSE
#     pump, plus seven new data-analysis actions.
#   - Reconstructed internal/sessions + internal/sandbox packages.
#   - The application remains SIGNED UNDER THE NAME PARSA TAK (exe
#     CompanyName, About dialog, SIGNATURE file in both zips).
set -euo pipefail

cd "$(dirname "$0")/.."

VERSION="1.0.9"
APP_NAME="sheytan-local-agent"
STAGE_DIR="dist-stage/$APP_NAME"
GH_STAGE_DIR="dist-stage/$APP_NAME-github"
DIST_DIR="/home/z/my-project/download"
ENGINE_SRC="/home/z/my-project/engine-dl/vulkan"
ENGINE_TAG="b10642"

# Toolchain paths
export GOROOT=/home/z/go-root/go
export PATH=$GOROOT/bin:/home/z/mingw32/extracted/usr/bin:$PATH
export GOPATH=/home/z/go
export GOFLAGS=-mod=mod

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
echo ">> Running stress suite (162 tests)..."
if ! SHEYTAN_DATA_DIR=/tmp/sheytan-stress-root /tmp/sheytan-stress stress 2>&1 | tail -3 | grep -q "0 fail"; then
  echo "!! Stress tests failed; aborting."
  exit 1
fi
rm -rf /tmp/sheytan-stress-root
echo ">> All stress tests pass."

echo ">> Running unit tests (Linux, no CGO)..."
if ! CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go test ./internal/sessions ./internal/memory ./internal/chunking ./internal/recall ./internal/tools ./internal/vision ./internal/termshell ./internal/resources ./internal/continuum ./internal/native > /tmp/unittest.log 2>&1; then
  cat /tmp/unittest.log
  echo "!! Unit tests failed; aborting."
  exit 1
fi
echo ">> All unit tests pass."

echo ">> Rendering GUI screenshots (headless verification)..."
if ! CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go test -tags headless ./internal/ui > /tmp/shot.log 2>&1; then
  cat /tmp/shot.log
  echo "!! Headless UI suite failed; aborting."
  exit 1
fi
echo ">> GUI screenshots rendered OK (see internal/ui/shots/)."

# Now switch to Windows cross-compile mode
export CC=x86_64-w64-mingw32-gcc
export CXX=x86_64-w64-mingw32-g++
export CGO_ENABLED=1
export GOOS=windows
export GOARCH=amd64

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
assert '1.0.9'.encode('utf-16-le') in data, 'version missing'
"; then
  echo "!! exe signature/version metadata verification failed"
  exit 1
fi
echo ">> exe verified: brand + Parsa Tak signature + v1.0.9 present."

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

# --- ZIP 2: GitHub-ready source (no .exe, no engine, no generated syso) ---
echo ">> Staging GitHub source tree..."
mkdir -p "$GH_STAGE_DIR"
rsync -a --delete \
  --exclude 'dist-stage/' \
  --exclude 'build/' \
  --exclude '*.exe' \
  --exclude '*.syso' \
  --exclude '*.zip' \
  --exclude 'rsrc_windows_amd64.syso' \
  --exclude '.git/' \
  --exclude 'internal/ui/shots/' \
  --exclude 'scripts/build-and-zip*.sh.bak' \
  ./ "$GH_STAGE_DIR/"
# Source zips carry docs + signature too.
cp SIGNATURE "$GH_STAGE_DIR/SIGNATURE" 2>/dev/null || true
cp /home/z/my-project/worklog.md "$GH_STAGE_DIR/worklog.md"

GH_ZIP="$DIST_DIR/$APP_NAME-$VERSION-github.zip"
rm -f "$GH_ZIP"
( cd dist-stage && zip -r -q "$GH_ZIP" "$APP_NAME-github" )
echo ""
echo ">> ZIP 2 (GitHub source): $GH_ZIP"
ls -lh "$GH_ZIP"

# Guard: the GitHub zip must NOT contain any exe/dll.
if unzip -l "$GH_ZIP" | grep -Ei '\.(exe|dll|syso)$' | grep -q .; then
  echo "!! GitHub zip contains binaries — refusing to ship:"
  unzip -l "$GH_ZIP" | grep -Ei '\.(exe|dll|syso)$'
  exit 1
fi
echo ">> GitHub zip verified: pure source, zero binaries."

echo ""
echo ">> Done. Outputs:"
ls -lh "$ZIP" "$GH_ZIP" | awk '{print "   " $NF " (" $5 ")"}'
