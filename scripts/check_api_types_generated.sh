#!/usr/bin/env bash
set -euo pipefail

./scripts/generate_api_types.sh >/tmp/llamacpp-stack-generate-api-types.log 2>&1 || {
  cat /tmp/llamacpp-stack-generate-api-types.log >&2
  exit 1
}

if ! git diff --quiet -- \
  gateway/internal/llamacppapi/generated/types.gen.go \
  gateway/internal/openaiapi/generated/types.gen.go; then
  cat /tmp/llamacpp-stack-generate-api-types.log >&2
  echo "generated API types are out of date; run ./scripts/generate_api_types.sh" >&2
  git diff -- gateway/internal/llamacppapi/generated/types.gen.go gateway/internal/openaiapi/generated/types.gen.go >&2
  exit 1
fi
