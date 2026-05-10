#!/usr/bin/env bash
set -euo pipefail
BASE_URL="${BASE_URL:-http://127.0.0.1:${HOST_PORT:-8090}/v1}"
MODEL="${MODEL:-${GATEWAY_SMOKE_MODEL:-Open4bits/Qwen3-0.6b-gguf/Q4_K_M}}"
API_KEY="${LLAMA_API_KEY:-sk-no-key-required}"

curl -NfsS "${BASE_URL}/chat/completions" \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer ${API_KEY}" \
  -d @- <<JSON
{
  "model": "${MODEL}",
  "messages": [{"role":"user","content":"Count from 1 to 20, separated by commas."}],
  "max_tokens": 128,
  "temperature": 0.2,
  "stream": true
}
JSON
printf '\n'
