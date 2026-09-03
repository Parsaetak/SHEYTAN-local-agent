#!/usr/bin/env bash
# v1.0.7 E2E: Continuum chapter rollover with a REAL LLM (GLM via the z-ai
# CLI proxy). Runs in ONE bash session because the sandbox reaps background
# processes between invocations.
#
#  1. Chapter 1: a real exchange plants a durable fact (company + code).
#  2. Context pressure crosses the threshold → Manager.Rollover creates
#     chapter 2 seeded with the distilled Framework briefing.
#  3. A REAL model answers in chapter 2 — the fact is only reachable through
#     the CONTINUUM FRAMEWORK block (the raw history is drowned by the work
#     log; the carried tail does not contain the fact).
#  4. Enhance() best-effort refinement against the live endpoint.
set -u
cd "$(dirname "$0")/.."

# Portable toolchain: use the go binary from PATH (no machine-specific
# GOROOT/GOPATH/mingw hacks — the Zeta tree cross-builds with stock Go).
export GOFLAGS=-mod=mod

export SHEYTAN_DATA_DIR=/tmp/e2e-v107-data

rm -rf /tmp/e2e-v107-data

node scripts/glm-proxy.mjs > /tmp/glm-proxy-v107.log 2>&1 &
PROXY_PID=$!
trap "kill $PROXY_PID 2>/dev/null" EXIT
sleep 2

if ! curl -s --max-time 5 http://127.0.0.1:8177/v1/models | grep -q glm-proxy; then
  echo "!! proxy not up"; cat /tmp/glm-proxy-v107.log; exit 1
fi
echo "== proxy up =="

echo ">> building e2e binary..."
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /tmp/sheytan-e2e-v107 ./scripts/e2e-v107 || exit 1

echo ">> running continuum e2e (real LLM, ~2-4 min)..."
timeout 420 /tmp/sheytan-e2e-v107
RC=$?

kill $PROXY_PID 2>/dev/null
echo "== e2e exit code: $RC =="
exit $RC
