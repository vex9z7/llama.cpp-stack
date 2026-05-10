# OpenAI Compatibility Adapter Plan

## Status

In progress. The gateway mostly proxies OpenAI-compatible llama.cpp routes, with small deterministic request/response adapters where llama.cpp output differs from OpenAI/Pipecat client expectations.

Pipecat-specific observed gaps and priorities are tracked in `docs/pipecat-responses-compat-notes.md`.

The public endpoint remains:

```text
https://llamacpp-stack.vex9z7.com/v1
```

Primary target APIs:

```text
GET  /v1/models
POST /v1/chat/completions
POST /v1/responses
POST /v1/embeddings
```

`/v1/responses` is a first-class integration target. New adapter work should test it at the same level as `/v1/chat/completions`.

## Goals

- Let application callers use normal OpenAI SDK/client patterns wherever possible.
- Hide common llama.cpp/model-template quirks behind the gateway.
- Preserve advanced llama.cpp extension escape hatches for operators and experiments.
- Keep response mutation minimal and schema-tested.
- Keep backend scheduling delegated to llama.cpp router mode.

## Non-goals

- Do not reimplement OpenAI's full API surface.
- Do not fake unsupported OpenAI features with misleading behavior.
- Do not rewrite llama.cpp Responses output unless a concrete SDK compatibility issue requires it.
- Do not add gateway-side model load/unload scheduling in this adapter work.

## Compatibility policy

The gateway should be OpenAI-compatible at the public boundary and llama.cpp-aware internally.

```text
External client
  -> OpenAI-style request
  -> gateway request adapter
  -> llama.cpp/router-compatible request
  -> backend response
  -> gateway response adapter only when needed
  -> OpenAI-compatible response contract
```

Rules:

1. Prefer OpenAI-compatible request fields in public examples and docs.
2. Continue accepting known llama.cpp extensions such as `chat_template_kwargs`.
3. If the caller provides an explicit llama.cpp extension, do not overwrite it.
4. Only translate fields whose mapping is deterministic and tested.
5. Pass through unknown fields unless they are known to break compatibility.
6. Keep endpoint-specific behavior explicit: Chat Completions and Responses may need different adapters.

## First response adapter: Responses usage details

Priority: P0. Status: implemented in `gateway/internal/apiadapter`.

Pipecat HTTP Responses currently streams text successfully but can crash when `usage.output_tokens_details` is `null` or missing. The gateway should normalize `/v1/responses` usage objects without rewriting unrelated response fields.

Required normalized shape:

```json
{
  "usage": {
    "input_tokens": 123,
    "input_tokens_details": {
      "cached_tokens": 0
    },
    "output_tokens": 45,
    "output_tokens_details": {
      "reasoning_tokens": 0
    },
    "total_tokens": 168
  }
}
```

Apply to:

```text
/v1/responses non-stream JSON response
/v1/responses stream response.completed event
```

If token counts are missing, only fill the details objects; do not invent token totals that cannot be derived.

## First adapter: reasoning/thinking disable

Status: implemented in `gateway/internal/apiadapter`.

Qwen3-style models may emit thinking unless the llama.cpp template receives:

```json
{
  "chat_template_kwargs": {
    "enable_thinking": false
  }
}
```

That is not an OpenAI-standard public parameter. The gateway should accept OpenAI-ish reasoning controls and adapt them for llama.cpp.

### Chat Completions request adapter

Endpoint:

```text
POST /v1/chat/completions
```

Accept:

```json
{
  "reasoning_effort": "none"
}
```

Also accept a Responses-like object if a client sends it:

```json
{
  "reasoning": {
    "effort": "none"
  }
}
```

or:

```json
{
  "reasoning": {
    "enabled": false
  }
}
```

Translate to llama.cpp only when thinking is explicitly disabled:

```json
{
  "chat_template_kwargs": {
    "enable_thinking": false
  }
}
```

### Responses request adapter

Endpoint:

```text
POST /v1/responses
```

Accept:

```json
{
  "reasoning": {
    "effort": "none"
  }
}
```

or:

```json
{
  "reasoning": {
    "enabled": false
  }
}
```

Translate to:

```json
{
  "chat_template_kwargs": {
    "enable_thinking": false
  }
}
```

Also accept `reasoning_effort = "none"` on `/v1/responses` as a convenience alias, but document `reasoning.effort` as the preferred Responses shape.

### Override precedence

If the request already includes:

```json
{
  "chat_template_kwargs": {
    "enable_thinking": true
  }
}
```

or:

```json
{
  "chat_template_kwargs": {
    "enable_thinking": false
  }
}
```

then the adapter must not override it.

Explicit backend extension wins over OpenAI-ish adapter fields.

### Effort values

First implementation should only map clear disable values:

```text
none
off
disabled
disable
false
```

Do not map `minimal`, `low`, `medium`, `high`, or `xhigh` to llama.cpp-specific budgets yet. Pass those fields through without injecting `enable_thinking` until a model-specific, tested mapping exists.

## Response adapter policy

The gateway should not eagerly rewrite Responses API output.

Current policy:

- Proxy llama.cpp `/v1/responses` response shape as-is except for deterministic OpenAI compatibility normalization such as usage details.
- Ensure HTTP headers are compatible, especially `Content-Type`.
- Add minimal response normalization only when an OpenAI SDK/client compatibility issue is reproduced.
- Any response normalization must have schema tests and deployed endpoint probes.

## Proposed code boundary

Adapter package:

```text
gateway/internal/apiadapter
```

Suggested public function:

```go
func AdaptRequest(endpoint string, body []byte) (AdaptedRequest, error)
```

Where:

```go
type AdaptedRequest struct {
    Body []byte
    Changed bool
    Notes []string
}
```

Gateway flow:

```text
read request body
parse model for catalog/capability validation
manager.EnsureAvailable(model, kind)
adapter.AdaptRequest(path, body)
proxy adapted body to llama.cpp router
```

The adapter should not know how to download models, render presets, or call the router.

## Tests and probes

Unit tests:

- Chat `reasoning_effort: none` injects `chat_template_kwargs.enable_thinking=false`.
- Chat `reasoning.effort: none` injects the same.
- Chat `reasoning.enabled: false` injects the same.
- Responses `reasoning.effort: none` injects the same.
- Responses `reasoning.enabled: false` injects the same.
- Existing `chat_template_kwargs.enable_thinking` is not overwritten.
- Non-disable efforts pass through unchanged.
- Non-chat/non-responses endpoints pass through unchanged.

Runtime probes:

- `/v1/chat/completions` with `reasoning_effort=none` returns no raw `<think>` content for Qwen3 test model.
- `/v1/responses` with `reasoning.effort=none` returns no raw `<think>` content for Qwen3 test model.
- Both responses pass OpenAI compatibility behavior probes.

## Documentation updates after implementation

After the adapter lands:

- Update `docs/api-schemas.md` compatibility notes.
- Update README public endpoint examples to prefer OpenAI-style reasoning fields.
- Keep `chat_template_kwargs` documented as an advanced llama.cpp escape hatch.
- Add adapter behavior to probe docs and schema probe coverage.
