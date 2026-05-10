#!/usr/bin/env bash
set -euo pipefail
BASE_URL="${BASE_URL:-http://127.0.0.1:${HOST_PORT:-8090}/v1}"
MODEL="${MODEL:-${GATEWAY_SMOKE_MODEL:-Open4bits/Qwen3-0.6b-gguf/Q4_K_M}}"
API_KEY="${LLAMA_API_KEY:-sk-no-key-required}"
SECONDS_BEFORE_CANCEL="${SECONDS_BEFORE_CANCEL:-2}"

payload=$(mktemp)
out=$(mktemp)
trap 'rm -f "$payload" "$out"' EXIT
cat >"$payload" <<JSON
{
  "model": "${MODEL}",
  "messages": [{"role":"user","content":"Write a very long numbered essay about request cancellation in streaming inference. Continue until stopped."}],
  "max_tokens": 2048,
  "temperature": 0.8,
  "stream": true,
  "chat_template_kwargs": {"enable_thinking": false}
}
JSON

printf 'Starting streaming request through gateway, then cancelling after %ss...\n' "$SECONDS_BEFORE_CANCEL"
curl -NfsS "${BASE_URL}/chat/completions" \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer ${API_KEY}" \
  -d @"$payload" >"$out" &
pid=$!
sleep "$SECONDS_BEFORE_CANCEL"
kill -INT "$pid" 2>/dev/null || true
wait "$pid" 2>/dev/null || true

printf 'Cancelled client request. Bytes received before cancel: '
wc -c <"$out"
printf '\nGateway intentionally does not expose /slots publicly. Check backend logs for upstream cancellation/freeing messages if needed.\n'
