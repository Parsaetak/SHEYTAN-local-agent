#!/usr/bin/env bash
# v1.0.6 E2E: multimodal wire format + attachment/recall regressions with a
# REAL LLM (GLM via the z-ai CLI proxy). Runs in ONE bash session because the
# sandbox reaps background processes between invocations.
#
#  1. Text attachment regression (the MarshalJSON rewrite must not break the
#     classic text path) — same checks as the v1.0.2 suite.
#  2. IMAGE attachment: a real PNG rides the message as OpenAI content parts.
#     Success = the endpoint accepts the request and answers (the proxy model
#     may or may not be vision-capable — the wire format must never break).
#  3. Persistent recall regression across sessions.
set -u
cd "$(dirname "$0")/.."

export SHEYTAN_DATA_DIR=/tmp/e2e-v106-data
export SHEYTAN_PROVIDER=remote
export SHEYTAN_REMOTE_BASE_URL=http://127.0.0.1:8177/v1
export SHEYTAN_REMOTE_MODEL=glm-proxy

rm -rf /tmp/e2e-v106-data
mkdir -p /tmp/e2e-v106-data /tmp/e2e-v106-root
printf 'Project Phoenix status: all systems go. Launch code is 7-7-7-ALPHA. Budget approved: 4.2 million.' > /tmp/e2e-v106-root/secret-brief.md

# A real PNG for the image test.
python3 - << 'PYEOF'
import struct, zlib
def chunk(t, d):
    c = struct.pack('>I', len(d)) + t + d
    return c + struct.pack('>I', zlib.crc32(t + d) & 0xffffffff)
w, h = 64, 48
raw = b''
for y in range(h):
    raw += b'\x00'
    for x in range(w):
        raw += bytes([255, 90, 38])
ihdr = struct.pack('>IIBBBBB', w, h, 8, 2, 0, 0, 0)
png = b'\x89PNG\r\n\x1a\n' + chunk(b'IHDR', ihdr) + chunk(b'IDAT', zlib.compress(raw)) + chunk(b'IEND', b'')
open('/tmp/e2e-v106-root/ember.png', 'wb').write(png)
print('png written:', len(png), 'bytes')
PYEOF

node scripts/glm-proxy.mjs > /tmp/glm-proxy-v106.log 2>&1 &
PROXY_PID=$!
trap "kill $PROXY_PID 2>/dev/null" EXIT
sleep 2

if ! curl -s --max-time 5 http://127.0.0.1:8177/v1/models | grep -q glm-proxy; then
  echo "!! proxy not up"; cat /tmp/glm-proxy-v106.log; exit 1
fi
echo "== proxy up =="

PASS=0; FAIL=0
check() { # name, needle, output
  if echo "$3" | grep -qi "$2"; then
    echo "PASS: $1"; PASS=$((PASS+1))
  else
    echo "FAIL: $1 (missing: $2)"; echo "--- got: ---"; echo "$3" | tail -6; echo "---"; FAIL=$((FAIL+1))
  fi
}

echo ""
echo "===== E2E 1: text attachment regression (.md inlined, real LLM) ====="
OUT=$(timeout 240 /tmp/sheytan-e2e ask --new --no-llm-start --attach /tmp/e2e-v106-root/secret-brief.md \
  "From the attached file content already inlined in this message (do NOT use any tools): what is the launch code and the budget? One line." 2>&1)
echo "$OUT" | tail -4
check "attachment_content_reaches_model" "7-7-7-ALPHA" "$OUT"
check "attachment_budget_answer" "4.2 million" "$OUT"

echo ""
echo "===== E2E 2: IMAGE attachment rides the multimodal wire (real endpoint) ====="
OUT=$(timeout 240 /tmp/sheytan-e2e ask --new --no-llm-start --attach /tmp/e2e-v106-root/ember.png \
  "I attached an image. Reply in ONE short line: first, the single word IMAGE, then describe anything you can about the attachment (format or content). Do not use tools." 2>&1)
echo "$OUT" | tail -4
check "image_request_accepted" "IMAGE" "$OUT"
# The request must not fail with a wire/parse error — any answer at all proves
# the content-parts body is valid JSON the endpoint accepts.
if echo "$OUT" | grep -qi "error\|400\|invalid"; then
  echo "FAIL: image_request_error_free (endpoint complained)"; echo "$OUT" | tail -4; FAIL=$((FAIL+1))
else
  echo "PASS: image_request_error_free"; PASS=$((PASS+1))
fi

echo ""
echo "===== E2E 3: persistent recall regression (session 1 → session 2) ====="
OUT=$(timeout 240 /tmp/sheytan-e2e ask --new --no-llm-start \
  "Important: my wifi password is cherry-ember-88. Just acknowledge briefly." 2>&1)
echo "$OUT" | tail -3
check "fact_acknowledged" "acknowledge\|noted\|stored\|cherry" "$OUT"

sleep 1
OUT=$(timeout 240 /tmp/sheytan-e2e ask --new --no-llm-start \
  "What is my wifi password? (You may recall it from memory or past context.)" 2>&1)
echo "$OUT" | tail -4
check "recall_finds_fact" "cherry-ember-88" "$OUT"

echo ""
echo "=============================================="
echo "E2E RESULT: $PASS pass / $FAIL fail"
exit $FAIL
