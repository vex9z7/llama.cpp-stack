#!/usr/bin/env bash
set -euo pipefail
BASE_URL="${BASE_URL:-http://127.0.0.1:${HOST_PORT:-8080}/v1}"
ROOT_URL="${ROOT_URL:-http://127.0.0.1:${HOST_PORT:-8080}}"
MODEL="${MODEL:-${LLAMA_ALIAS:-local-llm}}"
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
  "stream": true
}
JSON

printf 'Starting streaming request, then cancelling after %ss...\n' "$SECONDS_BEFORE_CANCEL"
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

printf '\nSlots after cancellation, if endpoint is enabled:\n'
curl -fsS "${ROOT_URL}/slots" || true
printf '\n\nCheck llama-server logs for slot cancellation/freeing messages:\n'
printf '  docker compose logs --tail=100 llama-server\n'
