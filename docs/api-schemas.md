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
3. Black-box probes against our deployed endpoint:
   - `GET /health`
   - `GET /v1/models`
   - `POST /v1/chat/completions`
   - `POST /v1/completions`
   - `POST /v1/responses`
   - `GET /slots`
   - `GET /metrics`
   - `POST /v1/embeddings`

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
- `schemas/json/slots-response.schema.json`
- `schemas/json/native-completion-request.schema.json`
- `schemas/json/native-completion-response.schema.json`
- `schemas/json/metrics-response.schema.json`
- `schemas/json/error-response.schema.json`

## Important compatibility notes

`/v1/chat/completions`, `/v1/completions`, `/v1/responses`, `/v1/models`, and `/v1/embeddings` are OpenAI-compatible endpoints, not the official OpenAI service.

`/health`, `/slots`, `/metrics`, and `/completion` are llama.cpp-native endpoints.

For Qwen3, thinking/reasoning is controlled per request with:

```json
{
  "chat_template_kwargs": {
    "enable_thinking": false
  }
}
```

Do not use `reasoning_format: "none"` as a thinking-off switch. In testing, that caused raw `<think>` text to move into `content` rather than preventing thinking.

`/metrics` returns `501` unless the server starts with `--metrics`.

`/v1/embeddings` returns `501` unless the server starts with `--embeddings`; embedding quality also depends on model and pooling configuration.

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
| `POST /v1/embeddings` | `embeddings-request.schema.json` | `embeddings-response.schema.json` / `error-response.schema.json` when disabled |
| `POST /completion` | `native-completion-request.schema.json` | `native-completion-response.schema.json` |
| `GET /slots` | n/a | `slots-response.schema.json` |
| `GET /metrics` | n/a | `metrics-response.schema.json` / `error-response.schema.json` when disabled |

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
