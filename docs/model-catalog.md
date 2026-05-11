# Model catalog

`models/catalog.toml` is intentionally small and Hugging Face CLI friendly.

It answers only one question:

> Which Hugging Face GGUF models are allowed to be lazy-downloaded by the gateway?

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

The gateway downloads the selected remote GGUF directly into a deterministic stable path:

```text
models/hf/<owner>/<repo>/<quant>.gguf
```

Example:

```text
models/hf/Qwen/Qwen3-4B-GGUF/Q4_K_M.gguf
```

The original Hugging Face filename is used only while selecting which remote file to download; runtime code always uses the stable path derived from `repo` and `quant`.

## Lazy download

There is intentionally no `make download` path. The gateway owns lazy download:

```text
gateway request -> ensure model available -> download if missing -> render preset -> router reload -> proxy to backend
```

Manual prefetch can be reintroduced later as an operator command, not as a host-side deployment prerequisite.

## Included starter catalog

The starter catalog intentionally mixes small smoke-test models, general chat models, coder models, and one embedding model so deployment and router behavior can be tested across model families.

Examples:

```text
Open4bits/Qwen3-0.6b-gguf/Q4_K_M
ggml-org/Qwen3-1.7B-GGUF/Q4_K_M
Qwen/Qwen3-4B-GGUF/Q4_K_M
AaryanK/Qwen3.5-0.8B-GGUF/Q4_K_M
unsloth/Qwen3.5-2B-GGUF/Q4_K_M
ggml-org/gemma-3-1b-it-GGUF/Q4_K_M
ibm-granite/granite-3.3-2b-instruct-GGUF/Q4_K_M
LiquidAI/LFM2-700M-GGUF/Q4_K_M
jc-builds/Qwen3.5-4B-Q4_K_M-GGUF/Q4_K_M
unsloth/Qwen3.5-9B-GGUF/Q4_K_M
worthdoing/Phi-4-mini-GGUF/Q4_K_M
openbmb/MiniCPM4-8B-GGUF/Q4_K_M
Qwen/Qwen2.5-Coder-3B-Instruct-GGUF/Q4_K_M
n24q02m/Qwen3-Embedding-0.6B-GGUF/Q4_K_M
```

Some models may have license or runtime requirements beyond this stack. For embedding models, set `kind = "embedding"`; the gateway will mark the generated router preset with `embeddings = true` and only allow that model on `/v1/embeddings`. Newly released model families may require a newer llama.cpp than the pinned image, so validate them with the smoke/probe commands before production use. Gemma models and other gated/community models may have their own license terms.
