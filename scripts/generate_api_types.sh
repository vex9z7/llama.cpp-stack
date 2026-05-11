#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OAPI_CODEGEN="${OAPI_CODEGEN:-$(go env GOPATH)/bin/oapi-codegen}"
VERSION="v2.7.0"

if [[ ! -x "${OAPI_CODEGEN}" ]]; then
  echo "Installing oapi-codegen ${VERSION}..." >&2
  go install "github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@${VERSION}"
fi

python3 "${ROOT}/scripts/generate_openai_gateway_schema.py" \
  --source "${ROOT}/openai-openapi/spec/openapi.documented.yml" \
  --output "${ROOT}/openai-api-schema.yaml"

mkdir -p \
  "${ROOT}/gateway/internal/llamacppapi/generated" \
  "${ROOT}/gateway/internal/openaiapi/generated"

"${OAPI_CODEGEN}" \
  -generate types,skip-prune \
  -package generated \
  -o "${ROOT}/gateway/internal/llamacppapi/generated/types.gen.go" \
  "${ROOT}/llamacpp-api-schema/openapi.yaml"

"${OAPI_CODEGEN}" \
  -generate types,skip-prune \
  -package generated \
  -o "${ROOT}/gateway/internal/openaiapi/generated/types.gen.go" \
  "${ROOT}/openai-api-schema.yaml"

gofmt -w \
  "${ROOT}/gateway/internal/llamacppapi/generated/types.gen.go" \
  "${ROOT}/gateway/internal/openaiapi/generated/types.gen.go"
