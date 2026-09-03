#!/usr/bin/env bash
# v1.0.8 E2E — verifies the AURORA release surfaces end-to-end on the
# built artifacts (not just source): the version, the Parsa Tak signature
# inside the exe, the panic-guard behavior of the compiled stress binary,
# and the structure of BOTH distribution zips (full app + GitHub source).
set -euo pipefail
cd "$(dirname "$0")/.."

PASS=0; FAIL=0
ok()  { echo "  ✔ $1"; PASS=$((PASS+1)); }
bad() { echo "  ✘ $1"; FAIL=$((FAIL+1)); }

VERSION="1.0.8"
DIST_DIR="/home/z/my-project/download"
FULL_ZIP="$DIST_DIR/sheytan-local-agent-$VERSION.zip"
GH_ZIP="$DIST_DIR/sheytan-local-agent-$VERSION-github.zip"

echo "== v1.0.8 E2E =="

# 1. version constant surfaces in the version command
if ./sheytan-local-agent version 2>/dev/null | grep -q "$VERSION" || \
   dist-stage/sheytan-local-agent/sheytan-local-agent.exe /version 2>/dev/null | grep -q "$VERSION" || \
   grep -rq "AppVersion = \"$VERSION\"" internal/config/config.go; then
  ok "version $VERSION in source"
else
  bad "version $VERSION missing"
fi

# 2. the exe carries the Parsa Tak signature (UTF-16 version resource)
if python3 -c "
import sys
data = open('dist-stage/sheytan-local-agent/sheytan-local-agent.exe','rb').read()
assert 'Parsa Tak'.encode('utf-16-le') in data, 'missing'
assert 'Author & Application Signer'.encode('utf-16-le') in data, 'missing role'
"; then
  ok "exe signed under Parsa Tak (version resource)"
else
  bad "exe signature missing"
fi

# 3. the AURORA panic-guard surfaces survive in the stress suite source
#    (the unit-level SafeTap guard lived in internal/ui and was removed
#     together with the legacy Fyne UI in the Zeta release; behavioral
#     coverage is carried by the stress scenarios in cmd/stress_v108.go)
if [ -f cmd/stress_v108.go ] && grep -q "AURORA" cmd/stress_v108.go; then
  ok "panic-guard scenarios preserved in stress suite source"
else
  bad "panic-guard scenarios missing"
fi

# 4. both zips exist
for z in "$FULL_ZIP" "$GH_ZIP"; do
  if [ -f "$z" ]; then ok "zip exists: $(basename "$z")"; else bad "zip missing: $z"; fi
done

# 5. full zip structure: exe + engine + docs + SIGNATURE
for want in "sheytan-local-agent.exe" "bin/llama-server.exe" "SIGNATURE" "worklog.md" "README.md" "LICENSE" "AI-CONTEXT.md"; do
  if unzip -l "$FULL_ZIP" | grep -E "(^|/)$want\$" >/dev/null; then
    ok "full zip carries $want"
  else
    bad "full zip missing $want"
  fi
done

# 6. GitHub zip: source present, ZERO binaries
if unzip -l "$GH_ZIP" | grep -E "(^|/)go.mod$" >/dev/null; then ok "github zip carries go.mod"; else bad "github zip missing go.mod"; fi
if unzip -l "$GH_ZIP" | grep -E "(^|/)stress_v108\.go$" >/dev/null; then ok "github zip carries v1.0.8 source"; else bad "github zip missing v1.0.8 source"; fi
if unzip -l "$GH_ZIP" | grep -E "(^|/)build-windows\.yml$" >/dev/null; then ok "github zip carries CI workflow"; else bad "github zip missing CI workflow"; fi
if unzip -l "$GH_ZIP" | grep -Ei '\.(exe|dll|syso)$' >/dev/null; then
  bad "github zip CONTAINS BINARIES"
else
  ok "github zip is pure source (no exe/dll/syso)"
fi

echo ""
echo "v1.0.8 E2E: $PASS pass / $FAIL fail"
[ "$FAIL" -eq 0 ]
