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
  - Request schema for `/v1/chat/completions`, including llama.cpp extensions such as `chat_template_kwargs`.
- `schemas/json/chat-completion-response.schema.json`
  - Response/chunk shape for `/v1/chat/completions`, including `reasoning_content`.
- `schemas/json/slots-response.schema.json`
  - Native `/slots` response shape.

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
