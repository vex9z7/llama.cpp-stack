# llama.cpp Upstream Contract

## Purpose

The gateway has two API contracts:

- OpenAI OpenAPI snapshot: public client-facing contract.
- Pinned llama.cpp upstream snapshot: private upstream contract consumed by the gateway.

This document describes the pinned llama.cpp snapshot. It exists so the gateway's
`llamacppapi` types and adapters are reviewed against the same llama.cpp version
that the Docker deployment runs.

## Pinned version

The current upstream snapshot is pinned to llama.cpp build tag `b8840`:

```text
repo: https://github.com/ggml-org/llama.cpp
git_tag: b8840
git_commit: 9e5647affa54ea724196db15ec9b76c4abd16d4a
```

Default router images are pinned to the matching image tags:

```text
CPU:    ghcr.io/ggml-org/llama.cpp:server-b8840
Vulkan: ghcr.io/ggml-org/llama.cpp:server-vulkan-b8840
CUDA:   ghcr.io/ggml-org/llama.cpp:server-cuda-b8840
```

The exact image digests are recorded in `llamacpp-upstream/SNAPSHOT`.

## Vendored files

The project vendors the relevant `tools/server` docs and source subset under:

```text
llamacpp-upstream/tools/server/
```

This is not used to build llama.cpp. It is a reviewed source snapshot for API
contract inspection and drift checks.

## Reviewed schema

The gateway-maintained upstream schema lives at:

```text
llamacpp-api-schema/openapi.yaml
```

It is not an official llama.cpp schema. It is a reviewed schema for the pinned
llama.cpp version and only covers upstream endpoints consumed by the gateway.

## Checks

Run:

```bash
make check-llamacpp-upstream
make compare-llamacpp-schema
```

This verifies:

- the pinned tag/commit/image metadata in `SNAPSHOT`,
- the default compose images use the pinned tags,
- the vendored server files exist,
- the vendored `server.cpp` still contains required gateway upstream routes,
- the reviewed schema references the same pinned tag/commit/images.

`make schemas` also runs this check.
