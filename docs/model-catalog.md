# Model catalog

`configs/models.catalog.toml` is intentionally small and Hugging Face CLI friendly.

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
gateway request -> ensure model available -> download if missing -> verify router registry -> proxy to backend
```

Manual prefetch can be reintroduced later as an operator command, not as a host-side deployment prerequisite.

## Included starter catalog

The starter catalog is intentionally curated rather than exhaustive: it keeps one smoke-test model, a few recommended chat/coding models, a small set of new-architecture compatibility candidates, a clearly marked experimental low-refusal/abliterated set, and one embedding model.

Examples:

```text
ggml-org/Qwen3-0.6B-GGUF/Q8_0
ggml-org/Qwen3-1.7B-GGUF/Q4_K_M
unsloth/Qwen3.5-2B-GGUF/Q4_K_M
Qwen/Qwen3-4B-GGUF/Q4_K_M
Qwen/Qwen3-8B-GGUF/Q4_K_M
Qwen/Qwen2.5-Coder-3B-Instruct-GGUF/Q4_K_M
Qwen/Qwen2.5-Coder-7B-Instruct-GGUF/Q4_K_M
LiquidAI/LFM2-700M-GGUF/Q4_K_M
unsloth/Qwen3.5-9B-GGUF/Q4_K_M
lukey03/Qwen3.5-9B-abliterated-GGUF/Q4_K_M
mradermacher/DeepSeek-R1-Distill-Qwen-7B-abliterated-v2-GGUF/Q4_K_M
QuantFactory/Mistral-Nemo-Instruct-2407-abliterated-GGUF/Q4_K_M
Qwen/Qwen3-Embedding-0.6B-GGUF/Q8_0
```

Some models may have license or runtime requirements beyond this stack. For embedding models, set `kind = "embedding"`; the gateway will mark the generated router preset with `embeddings = true` and only allow that model on `/v1/embeddings`. Newly released model families may require a newer llama.cpp than the pinned image, so validate them with the smoke/probe commands before production use. Official GGUF sources are preferred. Community-only experimental entries must be clearly grouped and should be added only when there is no suitable official source and the model is needed for a specific test. Low-refusal/abliterated models are for behavior testing, not trusted default baselines.



## MoE model policy

The catalog includes two MoE tiers:

- **Current 8GB UMA candidates**: `allenai/OLMoE-1B-7B-0125-GGUF` in `Q3_K_M` and `Q4_K_M`. OLMoE is a 1B-active / 7B-total MoE, so it is the realistic first MoE target for the current NucBox-class machine.
- **Future 64GB UMA candidates**: Qwen 30B/35B A3B MoE models from Unsloth. These entries are intentionally source-only and lazy-loaded; do not request them on the current 8GB machine unless testing memory failure behavior.

For Qwen3.5 35B A3B, the catalog does not pin `mmproj` initially. This avoids downloading the extra projector for text-only MoE benchmarking; add it later if vision testing is explicitly needed.

## Multimodal projector files

Some GGUF repos include a separate llama.cpp multimodal projector (`mmproj`) used to enable image inputs for vision-language models. Catalog entries can pin this optional file:

```toml
[[models]]
repo = "unsloth/Qwen3.5-4B-GGUF"
quant = "Q4_K_M"
mmproj = "mmproj-F16.gguf"
```

The model ref stays `<repo>/<quant>`. The gateway downloads the main GGUF and the projector into `models/hf/<repo>/`, and the generated router preset includes both `model = ...` and `mmproj = ...`. Text-only requests still use the same model ref.
