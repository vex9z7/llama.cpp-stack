#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DEST="$ROOT/openai-openapi"
TMP="${TMPDIR:-/tmp}/openai-openapi-snapshot.$$"
REPO_URL="https://github.com/openai/openai-openapi.git"
SPEC_URL="https://app.stainless.com/api/spec/documented/openai/openapi.documented.yml"

trap 'rm -rf "$TMP"' EXIT

git clone --depth 1 "$REPO_URL" "$TMP"
commit="$(git -C "$TMP" rev-parse HEAD)"
branch="$(git -C "$TMP" branch --show-current || true)"

mkdir -p "$DEST"
rsync -a --delete --exclude .git "$TMP/" "$DEST/"
mkdir -p "$DEST/spec"
curl -L --fail --silent --show-error "$SPEC_URL" -o "$DEST/spec/openapi.documented.yml"

cat > "$DEST/SNAPSHOT" <<SNAPSHOT
source = https://github.com/openai/openai-openapi
branch = ${branch:-unknown}
commit = ${commit}
spec_url = ${SPEC_URL}
fetched_at_utc = $(date -u +%Y-%m-%dT%H:%M:%SZ)
update_command = scripts/update_openai_openapi_snapshot.sh
SNAPSHOT

echo "Updated OpenAI OpenAPI snapshot: $commit"
