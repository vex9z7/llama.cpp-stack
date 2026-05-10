# API schemas

This repo keeps a local schema snapshot for the public Go gateway API surface we intend to integrate against.

## Source of truth

There is no single OpenAPI document emitted by the deployed gateway. The useful sources are:

1. llama.cpp server docs and README:
   - <https://github.com/ggml-org/llama.cpp/blob/master/tools/server/README.md>
   - <https://www.mintlify.com/ggml-org/llama.cpp/api/rest/overview>
   - <https://www.mintlify.com/ggml-org/llama.cpp/inference/server>
2. OpenAI API shape for the `/v1/*` compatibility layer:
   - <https://github.com/openai/openai-openapi>
   - <https://platform.openai.com/docs/api-reference>
3. Black-box probes against our deployed gateway:
   - `GET /health`
   - `GET /v1/models`
   - `POST /v1/chat/completions`
   - `POST /v1/completions`
   - `POST /v1/responses`
   - `POST /v1/embeddings`

## Files

- `schemas/openapi/gateway.openapi.yaml`
  - OpenAPI 3.1 subset for the public gateway endpoints.
- `schemas/json/chat-completion-request.schema.json`
- `schemas/json/chat-completion-response.schema.json`
- `schemas/json/completion-request.schema.json`
- `schemas/json/completion-response.schema.json`
- `schemas/json/responses-request.schema.json`
- `schemas/json/responses-response.schema.json`
- `schemas/json/models-response.schema.json`
- `schemas/json/embeddings-request.schema.json`
- `schemas/json/embeddings-response.schema.json`
- `schemas/json/health-response.schema.json`
- `schemas/json/error-response.schema.json`

Historical/internal schemas for llama.cpp-native endpoints may still exist under `schemas/json/`, but they are not part of the public gateway contract unless referenced by `schemas/openapi/gateway.openapi.yaml`.

## Important compatibility notes

`/v1/chat/completions`, `/v1/completions`, `/v1/responses`, `/v1/models`, and `/v1/embeddings` are OpenAI-compatible endpoints, not the official OpenAI service.

`/slots`, `/metrics`, `/completion`, `/worker/*`, and `/control/*` are intentionally not public gateway endpoints.

For Qwen3, thinking/reasoning is controlled per request with:

```json
{
  "chat_template_kwargs": {
    "enable_thinking": false
  }
}
```

Do not use `reasoning_format: "none"`, top-level `enable_thinking: false`, `jinja.enable_thinking: false`, `template_kwargs.enable_thinking: false`, or `reasoning: {"effort": "none"}` as thinking-off switches for the current llama.cpp/Qwen3 deployment. Live tests showed only top-level `chat_template_kwargs.enable_thinking=false` reliably turns Qwen3 thinking off for both `/v1/chat/completions` and `/v1/responses`.

`/v1/embeddings` is routed only to catalog models with `kind = "embedding"`; chat models return `model_capability_mismatch`.

## Maintenance policy

Treat these schemas as an integration contract for this stack, not a complete upstream spec. When upgrading llama.cpp or changing models, run the API probe again and update the schemas if observed behavior changes.

## Schema coverage

The schema set now includes request and response schemas for the core integration endpoints:

| Endpoint | Request schema | Response schema |
| --- | --- | --- |
| `GET /health` | n/a | `health-response.schema.json` |
| `GET /v1/models` | n/a | `models-response.schema.json` |
| `POST /v1/chat/completions` | `chat-completion-request.schema.json` | `chat-completion-response.schema.json` |
| `POST /v1/completions` | `completion-request.schema.json` | `completion-response.schema.json` |
| `POST /v1/responses` | `responses-request.schema.json` | `responses-response.schema.json` |
| `POST /v1/embeddings` | `embeddings-request.schema.json` | `embeddings-response.schema.json` / `error-response.schema.json` for capability mismatch |

The top-level request/response schemas are intentionally exact (`additionalProperties: false`) to avoid false positives. Nested implementation-specific objects such as timings, metadata, generation settings, and slot params remain permissive because llama.cpp evolves quickly and exposes backend-specific details there.

## Runtime validation

Use the probe script to validate both outbound requests and inbound responses against these schemas:

```bash
python3 -m pip install -r requirements-dev.txt
python3 scripts/probe_api_schemas.py --base-url https://llamacpp-stack.vex9z7.com --model Open4bits/Qwen3-0.6b-gguf/Q4_K_M
```

Or via Make:

```bash
make probe-gateway BASE_URL=https://llamacpp-stack.vex9z7.com
```

This catches false positives in permissive schemas and false negatives caused by schema drift.


## Gateway error codes

The Go gateway returns OpenAI-shaped error envelopes:

```json
{
  "error": {
    "message": "...",
    "type": "invalid_request_error",
    "code": "model_not_found"
  }
}
```

Stable gateway error codes:

- `invalid_json`
- `missing_model`
- `model_not_found`
- `model_capability_mismatch`
- `no_idle_worker`
- `download_failed`
- `worker_load_failed`
- `ensure_running_failed`

Probe helpers:

```bash
make probe-errors BASE_URL=https://llamacpp-stack.vex9z7.com
make probe-capacity BASE_URL=https://llamacpp-stack.vex9z7.com
make probe-cancel BASE_URL=https://llamacpp-stack.vex9z7.com
```
