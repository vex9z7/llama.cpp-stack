#!/usr/bin/env bash
set -euo pipefail

CATALOG="${CATALOG:-models/catalog.toml}"
MODELS_DIR="${MODELS_DIR:-models}"
MODEL_REF="${MODEL_REF:-${MODEL:-${LLAMA_MODEL:-}}}"
HF_REPO="${HF_REPO:-${MODEL_REPO:-}}"
QUANT="${QUANT:-}"
FILE_PATTERN="${FILE_PATTERN:-${MODEL_INCLUDE:-${INCLUDE:-}}}"
WRITE_ENV="${WRITE_ENV:-0}"
ENV_FILE="${ENV_FILE:-.env}"

usage() {
  cat <<USAGE
Usage:
  MODEL='<repo>/<quant>' $0
  HF_REPO='<hf-repo>' QUANT='Q4_K_M' $0
  HF_REPO='<hf-repo>' FILE_PATTERN='<glob>' $0

Examples:
  MODEL='Qwen/Qwen3-4B-GGUF/Q4_K_M' make download
  MODEL='Qwen/Qwen3-4B-GGUF/Q4_K_M' WRITE_ENV=1 make download
  HF_REPO='Qwen/Qwen3-8B-GGUF' QUANT='Q4_K_M' make download

Catalog:
USAGE
  python3 - "$CATALOG" <<'PY' 2>/dev/null || true
import sys, tomllib
from pathlib import Path
path = Path(sys.argv[1])
data = tomllib.loads(path.read_text())
print(f"  {'MODEL':<54} {'PATTERN'}")
for item in data.get('models', []):
    repo = item.get('repo', '')
    quant = item.get('quant', '')
    model = f"{repo}/{quant}" if repo and quant else repo
    pattern = item.get('pattern') or item.get('file') or (f"*{quant}*.gguf" if quant else '')
    print(f"  {model:<54} {pattern}")
PY
}

quote_assignments_from_catalog() {
  python3 - "$CATALOG" "$MODEL_REF" <<'PY'
import shlex, sys, tomllib
from pathlib import Path
catalog = Path(sys.argv[1])
model_ref = sys.argv[2]
data = tomllib.loads(catalog.read_text())
for item in data.get('models', []):
    repo = item.get('repo')
    quant = item.get('quant')
    if repo and quant and f"{repo}/{quant}" == model_ref:
        pattern = item.get('pattern') or item.get('file') or f"*{quant}*.gguf"
        for key, value in {
            'HF_REPO': repo,
            'QUANT': quant,
            'FILE_PATTERN': pattern,
            'MODEL_REF': model_ref,
        }.items():
            print(f"{key}={shlex.quote(str(value))}")
        raise SystemExit(0)
print(f"echo 'Unknown MODEL={model_ref}' >&2")
print("exit 2")
PY
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  usage
  exit 0
fi

if [[ -n "$MODEL_REF" && -z "$HF_REPO" ]]; then
  if [[ ! -f "$CATALOG" ]]; then
    echo "Missing catalog: $CATALOG" >&2
    exit 2
  fi
  eval "$(quote_assignments_from_catalog)"
fi

if [[ -z "$FILE_PATTERN" ]]; then
  if [[ -n "$QUANT" ]]; then
    FILE_PATTERN="*${QUANT}*.gguf"
  else
    echo "FILE_PATTERN or QUANT is required." >&2
    usage >&2
    exit 2
  fi
fi

if [[ -z "$HF_REPO" ]]; then
  echo "HF_REPO is required unless MODEL names a catalog entry." >&2
  usage >&2
  exit 2
fi

if [[ -z "$QUANT" && -n "$MODEL_REF" ]]; then
  QUANT="${MODEL_REF##*/}"
fi
if [[ -z "$MODEL_REF" && -n "$QUANT" ]]; then
  MODEL_REF="${HF_REPO}/${QUANT}"
fi
if [[ -z "$QUANT" ]]; then
  echo "QUANT is required to create the stable local path." >&2
  exit 2
fi

repo_dir="$MODELS_DIR/hf/$HF_REPO"
stable_rel="hf/$HF_REPO/${QUANT}.gguf"
stable_path="$MODELS_DIR/$stable_rel"
mkdir -p "$repo_dir"

printf 'Downloading GGUF model\n'
printf '  hf repo:      %s\n' "$HF_REPO"
printf '  pattern:      %s\n' "$FILE_PATTERN"
printf '  local dir:    %s\n' "$repo_dir"
printf '  stable path:  %s\n' "$stable_path"

if command -v hf >/dev/null 2>&1; then
  hf download "$HF_REPO" --include "$FILE_PATTERN" --local-dir "$repo_dir"
elif [[ -x .venv/bin/hf ]]; then
  .venv/bin/hf download "$HF_REPO" --include "$FILE_PATTERN" --local-dir "$repo_dir"
elif command -v huggingface-cli >/dev/null 2>&1; then
  huggingface-cli download "$HF_REPO" --include "$FILE_PATTERN" --local-dir "$repo_dir"
elif [[ -x .venv/bin/huggingface-cli ]]; then
  .venv/bin/huggingface-cli download "$HF_REPO" --include "$FILE_PATTERN" --local-dir "$repo_dir"
elif command -v docker >/dev/null 2>&1; then
  docker run --rm \
    -v "$(pwd)/$MODELS_DIR:/models:Z" \
    python:3.12-slim \
    bash -lc "pip install --no-cache-dir huggingface_hub >/dev/null && hf download '$HF_REPO' --include '$FILE_PATTERN' --local-dir '/models/hf/$HF_REPO'"
else
  echo "Need hf/huggingface-cli or docker." >&2
  echo "Install with: pip install -U huggingface_hub" >&2
  exit 2
fi

selected=$(python3 - "$repo_dir" "$FILE_PATTERN" <<'PY'
import sys
from pathlib import Path
repo_dir = Path(sys.argv[1])
pattern = sys.argv[2]
matches = sorted(p for p in repo_dir.glob(pattern) if p.is_file() or p.is_symlink())
real_matches = [p for p in matches if not p.is_symlink()]
if len(real_matches) == 1:
    print(real_matches[0].name)
elif len(matches) == 1:
    print(matches[0].name)
elif not matches:
    print(f"No .gguf file matches pattern: {pattern}", file=sys.stderr)
    raise SystemExit(1)
else:
    print("Multiple files match pattern; add pattern or file to catalog:", file=sys.stderr)
    for p in matches:
        print(f"  {p.name}", file=sys.stderr)
    raise SystemExit(1)
PY
)

printf 'Selected model file: %s\n' "$selected"
ln -sfn "$selected" "$stable_path"
printf 'Linked stable model path: %s -> %s\n' "$stable_rel" "$selected"

if [[ "$WRITE_ENV" == "1" ]]; then
  touch "$ENV_FILE"
  python3 - "$ENV_FILE" "$MODEL_REF" "$stable_rel" <<'PY'
from pathlib import Path
import sys
path = Path(sys.argv[1])
model_ref = sys.argv[2]
model_file = sys.argv[3]
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

if model_ref:
    lines = set_key(lines, 'LLAMA_MODEL', model_ref)
lines = set_key(lines, 'LLAMA_MODEL_FILE', model_file)
path.write_text('\n'.join(lines).rstrip() + '\n')
PY
  printf 'Updated %s with LLAMA_MODEL_FILE=%s\n' "$ENV_FILE" "$stable_rel"
else
  printf 'To use it, set in .env:\n'
  if [[ -n "$MODEL_REF" ]]; then
    printf '  LLAMA_MODEL=%s\n' "$MODEL_REF"
  fi
  printf '  LLAMA_MODEL_FILE=%s\n' "$stable_rel"
fi

find "$MODELS_DIR/hf" -type l -name '*.gguf' -print 2>/dev/null | sort || true
