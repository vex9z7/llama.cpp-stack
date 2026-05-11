#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DEST="$ROOT/llamacpp-upstream"
SNAPSHOT="$DEST/SNAPSHOT"
TMP="${TMPDIR:-/tmp}/llamacpp-upstream-snapshot.$$"
REPO_URL="https://github.com/ggml-org/llama.cpp.git"

trap 'rm -rf "$TMP"' EXIT

if [[ ! -f "$SNAPSHOT" ]]; then
  echo "missing $SNAPSHOT" >&2
  exit 1
fi

commit="${LLAMACPP_COMMIT:-$(awk -F': *' '$1 == "git_commit" {print $2}' "$SNAPSHOT")}"
commit="${commit//[[:space:]]/}"
if [[ -z "$commit" ]]; then
  echo "missing git_commit in $SNAPSHOT" >&2
  exit 1
fi

mapfile -t vendored_files < <(awk '
  /^vendored_files:/ {in_list=1; next}
  in_list && /^  - / {sub(/^  - /, ""); print; next}
  in_list && NF && $0 !~ /^  - / {in_list=0}
' "$SNAPSHOT")
if [[ ${#vendored_files[@]} -eq 0 ]]; then
  echo "missing vendored_files in $SNAPSHOT" >&2
  exit 1
fi

git clone --filter=blob:none --no-checkout "$REPO_URL" "$TMP"
git -C "$TMP" checkout --detach "$commit"

# Remove the previously vendored source files/directories while preserving
# metadata such as SNAPSHOT and SHA256SUMS.
rm -rf "$DEST/tools"

for rel in "${vendored_files[@]}"; do
  src="$TMP/$rel"
  dst="$DEST/$rel"
  if [[ ! -f "$src" ]]; then
    echo "missing upstream vendored file at $commit: $rel" >&2
    exit 1
  fi
  mkdir -p "$(dirname "$dst")"
  cp "$src" "$dst"
done

python3 "$ROOT/scripts/check_vendored_integrity.py" --write

echo "Updated llama.cpp upstream snapshot from $commit"
