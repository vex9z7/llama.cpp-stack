# Model catalog

`models/catalog.toml` is intentionally small and Hugging Face CLI friendly.

It answers only one question:

> Which Hugging Face GGUF models are allowed to be lazy-downloaded by the manager?

It does not describe runtime parameters, aliases, ports, routes, context size, parallel slots, or request overrides.

## Format

```toml
[[models]]
repo = "Qwen/Qwen3-4B-GGUF"
quant = "Q4_K_M"
```

The local model ref is derived directly from repo and quant:

```text
Qwen/Qwen3-4B-GGUF/Q4_K_M
```

The `repo` field maps directly to Hugging Face CLI semantics:

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

The manager should download into:

```text
models/hf/<owner>/<repo>/
```

and create a stable symlink:

```text
models/hf/<owner>/<repo>/<quant>.gguf -> <actual-hugging-face-filename>.gguf
```

Example:

```text
models/hf/Qwen/Qwen3-4B-GGUF/Q4_K_M.gguf -> Qwen3-4B-Q4_K_M.gguf
```

This lets runtime code use a deterministic path while preserving the original downloaded filename.

## Lazy download

There is intentionally no `make download` path. The planned manager backend owns lazy download:

```text
router request -> manager ensure-running(model_ref) -> download if missing -> load into idle worker
```

Manual prefetch can be reintroduced later as a `llamactl` command that calls the manager API, not as a host-side deployment prerequisite.

## Included starter catalog

The starter catalog intentionally mixes small smoke-test models, general chat models, coder models, and one embedding model so deployment and router behavior can be tested across model families.

Examples:

```text
Open4bits/Qwen3-0.6b-gguf/Q4_K_M
ggml-org/Qwen3-1.7B-GGUF/Q4_K_M
Qwen/Qwen3-4B-GGUF/Q4_K_M
Qwen/Qwen2.5-Coder-3B-Instruct-GGUF/Q4_K_M
n24q02m/Qwen3-Embedding-0.6B-GGUF/Q4_K_M
```

Some models may have license or runtime requirements beyond this stack. For example, embedding models should run in a separate instance with `--embeddings`, and Gemma models have their own license terms.
