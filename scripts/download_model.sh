#!/usr/bin/env bash
set -euo pipefail

CATALOG="${CATALOG:-models/catalog.tsv}"
MODELS_DIR="${MODELS_DIR:-models}"
MODEL_NAME="${MODEL_NAME:-${MODEL:-}}"
MODEL_REPO="${MODEL_REPO:-}"
MODEL_INCLUDE="${MODEL_INCLUDE:-${INCLUDE:-}}"
MODEL_ALIAS="${MODEL_ALIAS:-}"
WRITE_ENV="${WRITE_ENV:-0}"
ENV_FILE="${ENV_FILE:-.env}"

usage() {
  cat <<USAGE
Usage:
  MODEL=<catalog-name> $0
  MODEL_REPO=<hf-repo> MODEL_INCLUDE='<glob>' $0

Examples:
  MODEL=qwen3-4b-q4 make download
  MODEL_REPO=Qwen/Qwen3-8B-GGUF MODEL_INCLUDE='*Q4_K_M*.gguf' make download
  MODEL=qwen3-8b-q4 WRITE_ENV=1 make download

Catalog:
USAGE
  awk -F '\t' 'BEGIN { printf "  %-24s %-36s %-18s %s\n", "NAME", "REPO", "INCLUDE", "DESCRIPTION" }
    $0 !~ /^#/ && NF >= 5 { printf "  %-24s %-36s %-18s %s\n", $1, $2, $3, $5 }' "$CATALOG" 2>/dev/null || true
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  usage
  exit 0
fi

if [[ -n "$MODEL_NAME" && -z "$MODEL_REPO" ]]; then
  if [[ ! -f "$CATALOG" ]]; then
    echo "Missing catalog: $CATALOG" >&2
    exit 2
  fi
  row=$(awk -F '\t' -v name="$MODEL_NAME" '$0 !~ /^#/ && $1 == name { print; found=1; exit } END { if (!found) exit 1 }' "$CATALOG") || {
    echo "Unknown MODEL=$MODEL_NAME" >&2
    usage >&2
    exit 2
  }
  IFS=$'\t' read -r _name MODEL_REPO MODEL_INCLUDE MODEL_ALIAS _description <<<"$row"
fi

if [[ -z "$MODEL_REPO" || -z "$MODEL_INCLUDE" ]]; then
  echo "MODEL_REPO and MODEL_INCLUDE are required unless MODEL names a catalog entry." >&2
  usage >&2
  exit 2
fi

mkdir -p "$MODELS_DIR"

echo "Downloading GGUF model"
echo "  repo:    $MODEL_REPO"
echo "  include: $MODEL_INCLUDE"
echo "  dir:     $MODELS_DIR"

before=$(mktemp)
after=$(mktemp)
trap 'rm -f "$before" "$after"' EXIT
find "$MODELS_DIR" -maxdepth 1 -type f -name '*.gguf' -printf '%f\n' | sort >"$before"

if command -v hf >/dev/null 2>&1; then
  hf download "$MODEL_REPO" \
    --include "$MODEL_INCLUDE" \
    --local-dir "$MODELS_DIR"
elif [[ -x .venv/bin/hf ]]; then
  .venv/bin/hf download "$MODEL_REPO" \
    --include "$MODEL_INCLUDE" \
    --local-dir "$MODELS_DIR"
elif command -v huggingface-cli >/dev/null 2>&1; then
  huggingface-cli download "$MODEL_REPO" \
    --include "$MODEL_INCLUDE" \
    --local-dir "$MODELS_DIR"
elif [[ -x .venv/bin/huggingface-cli ]]; then
  .venv/bin/huggingface-cli download "$MODEL_REPO" \
    --include "$MODEL_INCLUDE" \
    --local-dir "$MODELS_DIR"
elif command -v docker >/dev/null 2>&1; then
  docker run --rm \
    -v "$(pwd)/$MODELS_DIR:/models:Z" \
    python:3.12-slim \
    bash -lc "pip install --no-cache-dir huggingface_hub >/dev/null && hf download '$MODEL_REPO' --include '$MODEL_INCLUDE' --local-dir /models"
else
  echo "Need hf/huggingface-cli or docker." >&2
  echo "Install with: pip install -U huggingface_hub" >&2
  exit 2
fi

find "$MODELS_DIR" -maxdepth 1 -type f -name '*.gguf' -printf '%f\n' | sort >"$after"
new_file=$(comm -13 "$before" "$after" | head -n 1 || true)
if [[ -z "$new_file" ]]; then
  new_file=$(find "$MODELS_DIR" -maxdepth 1 -type f -name '*.gguf' -printf '%f\n' | sort -r | head -n 1 || true)
fi

if [[ -n "$new_file" ]]; then
  echo "Selected model file: $new_file"
  if [[ "$WRITE_ENV" == "1" ]]; then
    touch "$ENV_FILE"
    python3 - "$ENV_FILE" "$new_file" "$MODEL_ALIAS" <<'PY'
from pathlib import Path
import sys
path = Path(sys.argv[1])
model_file = sys.argv[2]
alias = sys.argv[3]
lines = path.read_text().splitlines() if path.exists() else []

def set_key(lines, key, value):
    out=[]
    done=False
    for line in lines:
        if line.startswith(key + '='):
            out.append(f'{key}={value}')
            done=True
        else:
            out.append(line)
    if not done:
        out.append(f'{key}={value}')
    return out

lines = set_key(lines, 'LLAMA_MODEL_FILE', model_file)
if alias:
    lines = set_key(lines, 'LLAMA_ALIAS', alias)
path.write_text('\n'.join(lines).rstrip() + '\n')
PY
    echo "Updated $ENV_FILE with LLAMA_MODEL_FILE=$new_file"
  else
    echo "To use it, set in .env:"
    echo "  LLAMA_MODEL_FILE=$new_file"
    if [[ -n "$MODEL_ALIAS" ]]; then
      echo "  LLAMA_ALIAS=$MODEL_ALIAS"
    fi
  fi
else
  echo "No .gguf file found after download. Check MODEL_INCLUDE pattern." >&2
  exit 1
fi

ls -lh "$MODELS_DIR"/*.gguf 2>/dev/null || true
