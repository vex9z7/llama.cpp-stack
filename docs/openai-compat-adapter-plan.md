# OpenAI Compatibility Adapter Plan

## Status

Policy. The gateway should expose OpenAI-compatible HTTP APIs using the vendored OpenAI OpenAPI snapshot as the public contract. It should otherwise remain a thin proxy to llama.cpp router mode.

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
- Use `openai-openapi/spec/openapi.documented.yml` as the public API ground truth.
- Keep response mutation minimal.
- Keep backend scheduling delegated to llama.cpp router mode.

## Non-goals

- Do not reimplement OpenAI's full API surface.
- Do not fake unsupported OpenAI features with misleading behavior.
- Do not add gateway-side model load/unload scheduling in this adapter work.
- Do not add model-vendor-specific request fields or translations as public compatibility behavior.

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
3. If the caller provides an explicit llama.cpp extension, do not overwrite it.
4. Only translate fields whose mapping is deterministic and tested.
5. Pass through unknown fields unless they are known to break compatibility.
6. Keep endpoint-specific behavior explicit: Chat Completions and Responses may need different adapters.

## Current adapter policy

The gateway currently does not implement model-specific request adapters. Requests are forwarded to llama.cpp router mode after gateway-level catalog/capability checks.

If future OpenAI compatibility issues are found, fixes should be justified against one of:

1. vendored OpenAI OpenAPI snapshot;
2. OpenAI SDK generated types;
3. generic OpenAI-compatible client behavior.

Do not add compatibility behavior solely for a specific application framework or a specific model family.

## Response adapter policy

The gateway should not eagerly rewrite Responses API output. It should proxy llama.cpp `/v1/responses` response shape as-is unless the vendored OpenAI schema or SDK types require a generic normalization.

## Proposed code boundary for future generic adapters

If generic adapters become necessary, keep them inside the gateway container source tree. The adapter should not know how to download models, render presets, or call the router.

## Tests and probes

Use behavior probes that assert public OpenAI-compatible semantics, not local hand-written schemas.
