# Typed Gateway Boundary Plan

## Status

Implemented in the gateway layer only.

This plan does **not** change llama.cpp router mode, router scheduling, model
loading flags, compose router wiring, or the llama.cpp container behavior. The
router remains the upstream inference backend. The gateway becomes responsible
for making the public API boundary typed and OpenAI-compatible.

## Problem

The project treats the vendored OpenAI OpenAPI snapshot as the public API ground
truth, but the gateway historically proxied inference responses as raw bytes.
That made the schema a reference document rather than an implementation
constraint.

A concrete example is the OpenAI Responses API usage object. OpenAI's
`ResponseUsage` requires:

```json
{
  "output_tokens_details": {
    "reasoning_tokens": 0
  }
}
```

llama.cpp may return `output_tokens_details: null` or omit it. If the gateway raw
copies that response, the public endpoint is not OpenAI-compatible even though
`/v1/responses` exists and basic text generation works.

## Goals

1. Keep llama.cpp router mode untouched.
2. Make the gateway's public OpenAI boundary typed.
3. Make the gateway's internal llama.cpp boundary typed where the gateway must
   adapt upstream responses.
4. Keep streaming cancellation behavior and SSE flushing intact.
5. Preserve unknown upstream fields when possible, while normalizing fields that
   are part of the OpenAI contract.
6. Add static/local checks that do not require a deployed endpoint.

## Non-goals

- Do not reimplement llama.cpp router scheduling.
- Do not expose llama.cpp internal endpoints.
- Do not add Pipecat-specific behavior.
- Do not add Qwen-specific request fields or thinking controls.
- Do not fully type every OpenAI schema in one pass.

## Design

### Package boundaries

```text
gateway/internal/openaiapi
  Public OpenAI-compatible structs and contract helpers.


gateway/internal/llamacppapi
  Typed representations of the llama.cpp upstream fields that need adaptation.


gateway/internal/apiadapter
  Conversion functions from llama.cpp typed responses to OpenAI typed responses.


gateway/internal/server
  HTTP/Huma orchestration, model availability checks, proxying, and streaming
  framing. It should call typed adapters instead of editing arbitrary response
  maps inline.
```

### Implemented typed coverage

The gateway now has typed coverage for all public gateway-owned responses and
all non-streaming inference responses exposed by the current public API:

- `GET /health`: gateway-owned typed health payload.
- `GET /v1/models`: OpenAI-style typed model list.
- gateway-originated errors: OpenAI-style typed error object.
- `POST /v1/responses`: typed llama.cpp response -> typed OpenAI response.
- `POST /v1/chat/completions`: typed llama.cpp completion usage -> typed OpenAI completion usage.
- `POST /v1/completions`: typed llama.cpp completion usage -> typed OpenAI completion usage.
- `POST /v1/embeddings`: typed llama.cpp embedding usage -> typed OpenAI embedding usage.

Streaming inference remains framed as SSE. The gateway types and adapts the
Responses API `response.completed` event, because it contains the final OpenAI
`ResponseUsage` contract. Other streaming events are passed through unchanged
unless/until a concrete OpenAI contract mismatch is identified.

### Responses API flow

For non-streaming responses:

```text
llama.cpp JSON body
  -> llamacppapi.Response.UnmarshalJSON
  -> apiadapter.OpenAIResponseFromLlama
  -> openaiapi.Response.MarshalJSON
  -> client
```

For streaming responses:

```text
SSE frame
  -> typed SSE parser
  -> if event is response.completed:
       llamacppapi.ResponseCompletedEvent
       -> apiadapter.OpenAIResponseCompletedEventFromLlama
       -> openaiapi.ResponseCompletedEvent
     else:
       pass through unchanged
  -> client
```

Unknown fields are preserved through raw JSON maps. Typed fields that are part of
the OpenAI contract override upstream raw fields at the public boundary.

### Usage normalization rule

For OpenAI Responses usage:

- `input_tokens_details.cached_tokens` must exist; default to `0` when upstream
  omits/nulls details.
- `output_tokens_details.reasoning_tokens` must exist; default to `0` when
  upstream omits/nulls details.
- token counts default to `0` only when upstream omits them.

This makes the public response satisfy the OpenAI Responses usage contract while
not changing llama.cpp router behavior.

## Static checks

Static/local checks should verify:

1. The vendored OpenAI OpenAPI snapshot still declares `ResponseUsage` with
   required `output_tokens_details.reasoning_tokens`.
2. Static valid fixtures pass the OpenAI Responses usage contract.
3. Static invalid fixtures with `output_tokens_details: null` fail the contract.
4. Go typed OpenAI usage JSON never emits `output_tokens_details: null`.
5. llama.cpp typed usage with nil details converts to OpenAI typed usage with
   `reasoning_tokens: 0`.
6. SSE `response.completed` events are adapted without breaking event framing.

## Future phases

Future work should deepen typing for streaming event variants and request-body
validation. Each phase must keep llama.cpp router mode untouched and introduce
typed adapters at the gateway boundary rather than changing router internals.
