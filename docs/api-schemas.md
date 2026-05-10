# API schemas

This repo keeps a local schema snapshot for the llama.cpp API surface we intend to integrate against.

## Source of truth

There is no single OpenAPI document emitted by the deployed `llama-server` instance. The useful sources are:

1. llama.cpp server docs and README:
   - <https://github.com/ggml-org/llama.cpp/blob/master/tools/server/README.md>
   - <https://www.mintlify.com/ggml-org/llama.cpp/api/rest/overview>
   - <https://www.mintlify.com/ggml-org/llama.cpp/inference/server>
2. OpenAI API shape for the `/v1/*` compatibility layer:
   - <https://github.com/openai/openai-openapi>
   - <https://platform.openai.com/docs/api-reference>
3. Black-box probes against the public gateway endpoint:
   - `GET /health`
   - `GET /v1/models`
   - `POST /v1/chat/completions`
   - `POST /v1/completions`
   - `POST /v1/responses`
   - `POST /v1/embeddings`

Internal llama.cpp-native schemas are retained only for direct backend probes.
The gateway intentionally hides `/slots`, `/metrics`, and `/completion`.

## Files

- `schemas/openapi/llama-server.openapi.yaml`
  - OpenAPI 3.1 subset for the endpoints this stack uses.
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
- `schemas/json/slots-response.schema.json` — internal backend probe only
- `schemas/json/native-completion-request.schema.json` — internal backend probe only
- `schemas/json/native-completion-response.schema.json` — internal backend probe only
- `schemas/json/metrics-response.schema.json` — internal backend probe only
- `schemas/json/error-response.schema.json`

## Important compatibility notes

`/v1/chat/completions`, `/v1/completions`, `/v1/responses`, `/v1/models`, and `/v1/embeddings` are OpenAI-compatible endpoints, not the official OpenAI service.

The public gateway also exposes `/health`. Llama.cpp-native `/slots`, `/metrics`, and `/completion` are internal backend endpoints and are not part of the public gateway surface.

OpenAI compatibility adapter work is planned in `docs/openai-compat-adapter-plan.md`; Pipecat-specific findings are tracked in `docs/pipecat-responses-compat-notes.md`. Until that lands, known llama.cpp extensions such as `chat_template_kwargs.enable_thinking` may still be needed for some model-specific behavior.

For Qwen3, thinking/reasoning is controlled per request with:

```json
{
  "chat_template_kwargs": {
    "enable_thinking": false
  }
}
```

Do not use `reasoning_format: "none"` as a thinking-off switch. In testing, that caused raw `<think>` text to move into `content` rather than preventing thinking.

Backend `/metrics` returns `501` unless llama.cpp starts with `--metrics`.

`/v1/embeddings` is exposed by the gateway, but the gateway first checks catalog `kind = "embedding"`; embedding quality also depends on model and pooling configuration.

## Maintenance policy

Treat these schemas as an integration contract for this stack, not a complete upstream spec. When upgrading llama.cpp or changing models, run the API probe again and update the schemas if observed behavior changes.

## Schema coverage

The schema set includes request and response schemas for the public gateway endpoints, plus a few internal backend-only schemas used by direct llama.cpp probes:

| Endpoint | Request schema | Response schema | Scope |
| --- | --- | --- | --- |
| `GET /health` | n/a | `health-response.schema.json` | public gateway |
| `GET /v1/models` | n/a | `models-response.schema.json` | public gateway |
| `POST /v1/chat/completions` | `chat-completion-request.schema.json` | `chat-completion-response.schema.json` | public gateway |
| `POST /v1/completions` | `completion-request.schema.json` | `completion-response.schema.json` | public gateway |
| `POST /v1/responses` | `responses-request.schema.json` | `responses-response.schema.json` | public gateway |
| `POST /v1/embeddings` | `embeddings-request.schema.json` | `embeddings-response.schema.json` / `error-response.schema.json` when disabled | public gateway |
| `POST /completion` | `native-completion-request.schema.json` | `native-completion-response.schema.json` | internal backend only |
| `GET /slots` | n/a | `slots-response.schema.json` | internal backend only |
| `GET /metrics` | n/a | `metrics-response.schema.json` / `error-response.schema.json` when disabled | internal backend only |

The top-level request/response schemas are intentionally exact (`additionalProperties: false`) to avoid false positives. Nested implementation-specific objects such as timings, metadata, generation settings, and slot params remain permissive because llama.cpp evolves quickly and exposes backend-specific details there.

## Runtime validation

Use the probe script to validate both outbound requests and inbound responses against these schemas:

```bash
python3 -m pip install -r requirements-dev.txt
python3 scripts/probe_api_schemas.py --base-url https://llamacpp-stack.vex9z7.com --model local-llm
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
- `download_failed`
- `router_reload_failed`
- `router_unavailable`
- `ensure_available_failed`

Probe helpers:

```bash
make probe-errors BASE_URL=https://llamacpp-stack.vex9z7.com
make probe-capacity BASE_URL=https://llamacpp-stack.vex9z7.com
make probe-cancel BASE_URL=https://llamacpp-stack.vex9z7.com
```
