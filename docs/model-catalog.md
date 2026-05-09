# Model catalog

`models/catalog.toml` is intentionally small and Hugging Face CLI friendly.

It answers only one question:

> Where do we download this GGUF model from?

It does not describe runtime parameters, aliases, ports, routes, context size, parallel slots, or request overrides.

## Format

```toml
[[models]]
repo = "Qwen/Qwen3-4B-GGUF"
quant = "Q4_K_M"
```

The local model id is derived directly from repo and quant:

```text
Qwen/Qwen3-4B-GGUF/Q4_K_M
```

The `repo` field maps directly to Hugging Face CLI:

```bash
hf download Qwen/Qwen3-4B-GGUF ...
```

The `quant` field derives the Hugging Face include glob:

```text
quant = "Q4_K_M" -> --include "*Q4_K_M*.gguf"
```

This matters because many GGUF repos contain multiple quantizations. Without filtering, a download may pull tens of GB.

## Optional override

If a repo uses unusual filenames, add an explicit pattern or exact file:

```toml
[[models]]
repo = "org/repo"
quant = "Q4_K_M"
pattern = "*Q4_K_M*Instruct*.gguf"

[[models]]
repo = "org/another-repo"
quant = "Q4_K_M"
file = "exact-file-name.gguf"
```

## Stable local filename

After download, the script creates a stable symlink:

```text
models/hf/<owner>/<repo>/<quant>.gguf -> <actual-hugging-face-filename>.gguf
```

Example:

```text
models/hf/Qwen/Qwen3-4B-GGUF/Q4_K_M.gguf -> Qwen3-4B-Q4_K_M.gguf
```

This lets deployment use a filename derived from the model id while preserving the original downloaded file.

## Deployment-time download

Set a catalog model id in `.env`:

```env
LLAMA_MODEL=Qwen/Qwen3-4B-GGUF/Q4_K_M
```

Then:

```bash
make up
```

`make up` runs the catalog downloader first, then starts `llama-server` with:

```text
LLAMA_MODEL_FILE=hf/Qwen/Qwen3-4B-GGUF/Q4_K_M.gguf
```

If `LLAMA_MODEL` is empty, deployment falls back to `LLAMA_MODEL_FILE` / `model.gguf` behavior.
