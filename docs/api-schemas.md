# API contract and OpenAI schema source

This stack treats the vendored OpenAI OpenAPI snapshot as the ground truth for the public OpenAI-compatible API shape.

Snapshot location:

```text
openai-openapi/spec/openapi.documented.yml
openai-openapi/SNAPSHOT
```

Update/check commands:

```bash
make update-openai-openapi
make check-openai-openapi
make schemas
```

`make schemas` validates the full static contract pipeline: vendored snapshot integrity, the OpenAI upstream snapshot, OpenAI response fixtures, the pinned llama.cpp upstream snapshot, the local llama.cpp schema/source comparison, gateway typed-boundary rules, and generated API type drift. The previous hand-written `schemas/json` and `schemas/openapi` files were removed to avoid maintaining a parallel, drifting schema copy.

## Source-of-truth order

1. OpenAI API Reference and vendored OpenAI OpenAPI snapshot.
2. OpenAI SDK generated types when practical client compatibility differs from raw schema text.
3. llama.cpp backend behavior.
4. Public gateway behavior probes.

The gateway should adapt llama.cpp backend reality toward the OpenAI-compatible public contract where the mapping is deterministic and safe.

## Public gateway surface

```text
GET  /health
GET  /v1/models
POST /v1/chat/completions
POST /v1/completions
POST /v1/responses
POST /v1/embeddings
```

The public gateway intentionally hides llama.cpp-native or router-management endpoints:

```text
/slots
/metrics
/props
/completion
/models/load
/models/unload
```

## Adapter obligations

The gateway should not define a project-specific schema dialect. Public request and response shapes should follow the vendored OpenAI OpenAPI snapshot. Backend-specific behavior should pass through unless a generic OpenAI compatibility issue is identified from the official schema or SDK types.

## Probes

Use static schema/type checks plus behavior probes instead of local hand-written request/response schema copies:

```bash
make probe-gateway
make probe-api
make probe-errors
make probe-capacity
make probe-cancel
```

`make probe-api` runs `scripts/probe_openai_compat.py` and checks basic OpenAI-compatible HTTP behavior without local hand-written schemas.

## Vendored integrity

The upstream snapshots under `openai-openapi/` and `llamacpp-upstream/` are
external source snapshots. They should not be edited in-place. Each directory
contains a `SHA256SUMS` manifest, and CI runs:

```bash
make check-vendored-integrity
```

To intentionally refresh a snapshot, use the documented update command, review
the upstream diff, then regenerate the manifests with:

```bash
make update-vendored-integrity
```
