# Typed Gateway Boundary Plan

## Status

Planned and implemented in the gateway layer only.

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

### Responses API phase

The first typed boundary covers `/v1/responses`, because this is where the
current OpenAI contract mismatch exists.

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

After Responses is typed and stable, apply the same pattern to:

1. Chat Completions usage and message shape.
2. Embeddings responses.
3. Legacy completions.
4. Request validation for supported OpenAI request subsets.

Each phase should keep llama.cpp router mode untouched and should introduce typed
adapters at the gateway boundary rather than changing router internals.
