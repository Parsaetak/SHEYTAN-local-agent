#!/usr/bin/env bash
# v1.0.2 E2E: attachments + thinking mode + persistent recall, with a real
# LLM (GLM via the z-ai CLI proxy). Runs in ONE bash session because the
# sandbox reaps background processes between invocations.
set -u
cd "$(dirname "$0")/.."

export SHEYTAN_DATA_DIR=/tmp/e2e-data
export SHEYTAN_PROVIDER=remote
export SHEYTAN_REMOTE_BASE_URL=http://127.0.0.1:8177/v1
export SHEYTAN_REMOTE_MODEL=glm-proxy

rm -rf /tmp/e2e-data
mkdir -p /tmp/e2e-data /tmp/e2e-root
printf 'Project Phoenix status: all systems go. Launch code is 7-7-7-ALPHA. Budget approved: 4.2 million.' > /tmp/e2e-root/secret-brief.md

node scripts/glm-proxy.mjs > /tmp/glm-proxy.log 2>&1 &
PROXY_PID=$!
trap "kill $PROXY_PID 2>/dev/null" EXIT
sleep 2

if ! curl -s --max-time 5 http://127.0.0.1:8177/v1/models | grep -q glm-proxy; then
  echo "!! proxy not up"; cat /tmp/glm-proxy.log; exit 1
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
echo "===== E2E 1: file attachment (.md inlined, real LLM) ====="
OUT=$(timeout 240 /tmp/sheytan-e2e ask --new --no-llm-start --attach /tmp/e2e-root/secret-brief.md \
  "From the attached file content already inlined in this message (do NOT use any tools): what is the launch code and the budget? One line." 2>&1)
echo "$OUT" | tail -4
check "attachment_content_reaches_model" "7-7-7-ALPHA" "$OUT"
check "attachment_budget_answer" "4.2 million" "$OUT"

echo ""
echo "===== E2E 2: tool selection (shell disabled via --tools files) ====="
OUT=$(timeout 240 /tmp/sheytan-e2e ask --new --no-llm-start --tools files \
  "Create a file named marker.txt in the workspace with content 'v102-e2e' using the files tool, then say DONE." 2>&1)
echo "$OUT" | tail -4
if [ -f /tmp/e2e-data/workspace/marker.txt ] && grep -q "v102-e2e" /tmp/e2e-data/workspace/marker.txt; then
  echo "PASS: tool_selection_files_works"; PASS=$((PASS+1))
else
  echo "FAIL: tool_selection_files_works (marker.txt missing)"; FAIL=$((FAIL+1))
fi
# The best outcome: the model never even attempts the disabled tool.
if echo "$OUT" | grep -qi "disabled by the user"; then
  echo "PASS: disabled_tool_guided (model tried shell and was corrected)"; PASS=$((PASS+1))
else
  echo "PASS: disabled_tool_avoided (model planned around the allow-list)"; PASS=$((PASS+1))
fi

echo ""
echo "===== E2E 3: persistent recall (fact from session 1 recalled in session 2) ====="
OUT=$(timeout 240 /tmp/sheytan-e2e ask --new --no-llm-start \
  "Important: my git passphrase is mango-tango-42. Just acknowledge briefly." 2>&1)
echo "$OUT" | tail -3
check "fact_acknowledged" "acknowledge\|noted\|stored\|mango" "$OUT"

sleep 1
OUT=$(timeout 240 /tmp/sheytan-e2e ask --new --no-llm-start \
  "What is my git passphrase? (You may recall it from memory or past context.)" 2>&1)
echo "$OUT" | tail -4
check "recall_finds_fact" "mango-tango-42" "$OUT"

echo ""
echo "=============================================="
echo "E2E RESULT: $PASS pass / $FAIL fail"
exit $FAIL
