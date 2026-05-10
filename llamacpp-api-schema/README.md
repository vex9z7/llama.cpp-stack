# llama.cpp API Schema

This directory contains the llama.cpp upstream API contract used by
`llama.cpp-stack`.

The schema is **not** an official llama.cpp OpenAPI document. It is a reviewed
schema for the pinned llama.cpp server snapshot recorded in:

```text
../llamacpp-upstream/SNAPSHOT
```

Current pin:

```text
llama.cpp git tag: b8840
llama.cpp commit: 9e5647affa54ea724196db15ec9b76c4abd16d4a
CPU image:    ghcr.io/ggml-org/llama.cpp:server-b8840
Vulkan image: ghcr.io/ggml-org/llama.cpp:server-vulkan-b8840
CUDA image:   ghcr.io/ggml-org/llama.cpp:server-cuda-b8840
```

## Files

```text
openapi.yaml
```

`openapi.yaml` covers the upstream endpoints consumed by the gateway:

```text
GET  /health
GET  /models
POST /models/load
POST /models/unload
POST /v1/chat/completions
POST /v1/completions
POST /v1/responses
POST /v1/embeddings
```

## Checks

Run:

```bash
make check-llamacpp-upstream
```

or as part of all schema checks:

```bash
make schemas
```
