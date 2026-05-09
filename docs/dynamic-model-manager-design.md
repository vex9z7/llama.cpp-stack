# Dynamic Model Manager Design

## 1. Purpose

This document defines the dynamic model management architecture for `llama.cpp-stack`.

The goal is to support many cataloged models while only running a bounded number of `llama-server` workers at a time.

Important distinction:

```text
cataloged model  != downloaded model != running instance
```

A model in `models/catalog.toml` is only allowed/known. It should not automatically consume RAM/VRAM. A model becomes expensive only when the manager starts a `llama-server` worker for it.

## 2. High-level architecture

```text
Client / Pipecat / Agents
        |
        v
Router
  - OpenAI-compatible API
  - no Docker socket
  - proxies requests
        |
        | internal HTTP control API
        v
Manager Backend
  - catalog and local model state
  - max instance slots
  - model download
  - worker lifecycle
  - Docker socket access
        |
        v
llama-server worker containers
  - one loaded model per worker
```

The router must not mount `/var/run/docker.sock`. Docker control is isolated to the manager backend.

## 3. Core constraints

## 3.1 One llama-server worker runs one model

A `llama-server` process is started with a single model path:

```bash
llama-server --model /models/hf/Qwen/Qwen3-4B-GGUF/Q4_K_M.gguf
```

It can handle multiple concurrent requests for that same model via slots, but it is not a multi-model runtime.

Therefore dynamic model management means dynamically creating/stopping worker containers, not hot-swapping multiple GGUF models inside one `llama-server` process.

## 3.2 Instance slots are bounded

The system has a fixed maximum number of running workers:

```env
LLAMA_MAX_INSTANCES=2
```

This acts like a memory/VRAM residency limit.

If all slots are occupied and a request asks for a model that is not already running, v1 rejects the request. No eviction or queueing in v1.

## 4. Component responsibilities

## 4.1 Model catalog

Source file:

```text
models/catalog.toml
```

Example:

```toml
[[models]]
repo = "Qwen/Qwen3-4B-GGUF"
quant = "Q4_K_M"
```

Derived identity:

```text
model_ref = Qwen/Qwen3-4B-GGUF/Q4_K_M
model_path = hf/Qwen/Qwen3-4B-GGUF/Q4_K_M.gguf
```

The catalog only defines what may be downloaded/started. It does not define OpenAI model names, ports, runtime parameters, or routes.

## 4.2 Manager backend

The manager is the only service that controls Docker.

Responsibilities:

- read `models/catalog.toml`;
- report catalog/download/running state;
- download models into `models/hf/...`;
- allocate worker slots;
- start `llama-server` worker containers;
- stop/remove worker containers;
- wait for worker `/health`;
- reject when no free slots exist;
- reconstruct state from Docker labels after restart.

## 4.3 Router

The router is the public request gateway.

Responsibilities:

- expose OpenAI-compatible endpoints;
- call manager to ensure a model is running;
- proxy requests to the selected worker;
- proxy streaming SSE responses;
- close upstream connections on client disconnect;
- return capacity/startup errors from manager to clients.

Router does not:

- access Docker socket;
- download models directly;
- start/stop containers directly.

## 4.4 llamactl

`llamactl` is a CLI wrapper around the manager API.

It does not directly call Docker in the target architecture.

Example commands:

```bash
llamactl models list
llamactl models download Qwen/Qwen3-4B-GGUF/Q4_K_M
llamactl instances list
llamactl instances stop 0
```

## 5. Manager API v1

The manager API is internal-only. It should not be exposed publicly.

Suggested base URL inside Docker network:

```text
http://manager:8091
```

## 5.1 `GET /control/models`

Returns catalog models with local/running status.

Example response:

```json
{
  "models": [
    {
      "model_ref": "Qwen/Qwen3-4B-GGUF/Q4_K_M",
      "repo": "Qwen/Qwen3-4B-GGUF",
      "quant": "Q4_K_M",
      "model_path": "hf/Qwen/Qwen3-4B-GGUF/Q4_K_M.gguf",
      "downloaded": true,
      "running": false
    }
  ]
}
```

## 5.2 `GET /control/instances`

Returns current slot state.

Example response:

```json
{
  "max_instances": 2,
  "instances": [
    {
      "slot": 0,
      "state": "running",
      "model_ref": "Qwen/Qwen3-4B-GGUF/Q4_K_M",
      "model_name": "Qwen/Qwen3-4B-GGUF/Q4_K_M",
      "model_path": "hf/Qwen/Qwen3-4B-GGUF/Q4_K_M.gguf",
      "container": "llama-slot-0",
      "backend_url": "http://llama-slot-0:8080"
    },
    {
      "slot": 1,
      "state": "empty",
      "container": "llama-slot-1"
    }
  ]
}
```

## 5.3 `POST /control/ensure-running`

Ensures a model has a running worker.

Request:

```json
{
  "model_ref": "Qwen/Qwen3-4B-GGUF/Q4_K_M"
}
```

If already running:

```json
{
  "status": "running",
  "slot": 0,
  "backend_url": "http://llama-slot-0:8080"
}
```

If cold-started:

```json
{
  "status": "started",
  "slot": 1,
  "backend_url": "http://llama-slot-1:8080"
}
```

If capacity is full:

```http
HTTP/1.1 429 Too Many Requests
```

```json
{
  "error": {
    "type": "capacity_error",
    "code": "max_instances_reached",
    "message": "No free llama-server instance slots"
  }
}
```

If startup fails or times out:

```http
HTTP/1.1 503 Service Unavailable
```

```json
{
  "error": {
    "type": "startup_error",
    "code": "worker_start_failed",
    "message": "llama-server worker failed health check"
  }
}
```

## 5.4 `DELETE /control/instances/{slot}`

Stops and removes a worker in a slot.

Default behavior should refuse to stop active workers unless forced.

Before stopping, manager should check backend `/slots` and ensure no slot has:

```json
"is_processing": true
```

Force option:

```http
DELETE /control/instances/0?force=true
```

## 6. Worker container model

Each running model uses one worker container.

Slot-derived values:

```text
slot 0 -> container llama-slot-0 -> backend_url http://llama-slot-0:8080
slot 1 -> container llama-slot-1 -> backend_url http://llama-slot-1:8080
```

Worker start command shape:

```bash
docker run -d \
  --name llama-slot-0 \
  --network "$LLAMA_DOCKER_NETWORK" \
  -v "$PWD/models:/models:ro,Z" \
  --device /dev/dri:/dev/dri \
  --label com.llamacpp-stack.role=worker \
  --label com.llamacpp-stack.slot=0 \
  --label com.llamacpp-stack.model_ref="Qwen/Qwen3-4B-GGUF/Q4_K_M" \
  ghcr.io/ggml-org/llama.cpp:server-vulkan \
  --host 0.0.0.0 \
  --port 8080 \
  --model /models/hf/Qwen/Qwen3-4B-GGUF/Q4_K_M.gguf \
  --alias "Qwen/Qwen3-4B-GGUF/Q4_K_M" \
  --ctx-size 8192 \
  --parallel 1 \
  --n-gpu-layers 999 \
  --cont-batching \
  --cache-prompt
```

No host port publishing is required for workers in the normal router path. Router talks to workers over Docker network by container name.

Optional debug mode may publish host ports later.

## 7. Docker labels

Worker containers should be labeled so manager can reconstruct state after restart.

Suggested labels:

```text
com.llamacpp-stack.role=worker
com.llamacpp-stack.slot=0
com.llamacpp-stack.model_ref=Qwen/Qwen3-4B-GGUF/Q4_K_M
com.llamacpp-stack.model_path=hf/Qwen/Qwen3-4B-GGUF/Q4_K_M.gguf
com.llamacpp-stack.model_name=Qwen/Qwen3-4B-GGUF/Q4_K_M
```

Manager can list workers using:

```bash
docker ps --filter label=com.llamacpp-stack.role=worker
```

This avoids relying only on a mutable state file.

## 8. Router request flow

## 8.1 Model already running

```text
POST /v1/chat/completions model=X
        |
        v
router -> manager ensure-running(X)
        |
        v
manager returns backend_url for existing worker
        |
        v
router proxies to backend_url/v1/chat/completions
```

## 8.2 Model cold, free slot exists

```text
router -> manager ensure-running(X)
        |
        v
manager downloads if needed
manager starts worker in free slot
manager waits /health
        |
        v
router proxies request
```

## 8.3 Model cold, all slots full

```text
router -> manager ensure-running(X)
        |
        v
manager returns 429 capacity_error
        |
        v
router returns 429 to client
```

No eviction in v1.

## 9. `/v1/models` semantics

The router may return catalog models, not only running models.

Example:

```json
{
  "object": "list",
  "data": [
    {
      "id": "Qwen/Qwen3-4B-GGUF/Q4_K_M",
      "object": "model",
      "owned_by": "llama.cpp-stack",
      "meta": {
        "downloaded": true,
        "running": false,
        "cold_start": true
      }
    }
  ]
}
```

This makes `/v1/models` a catalog of available cold-startable models. This differs from conservative OpenAI-compatible semantics where only warm/running models are listed. We should document this extension clearly.

## 10. Capacity policy v1

Initial policy:

```text
LLAMA_EVICTION_POLICY=reject
```

Rules:

1. If requested model is already running, use it.
2. If requested model is not running and a slot is free, start it.
3. If requested model is not running and no slot is free, return HTTP 429.
4. Do not stop or evict running models automatically.

Future policies:

- LRU eviction of idle workers;
- idle TTL shutdown;
- queue until slot frees;
- priority-based eviction;
- per-model pinning.

## 11. Environment variables

Suggested manager/router env:

```env
LLAMA_MAX_INSTANCES=2
LLAMA_EVICTION_POLICY=reject
LLAMA_INSTANCE_START_TIMEOUT_SECONDS=120
LLAMA_DOCKER_NETWORK=llama-cpp-stack_default
LLAMA_WORKER_IMAGE=ghcr.io/ggml-org/llama.cpp:server-vulkan
LLAMA_WORKER_BACKEND=vulkan
LLAMA_WORKER_CTX_SIZE=8192
LLAMA_WORKER_PARALLEL=1
LLAMA_WORKER_THREADS_HTTP=-1
LLAMA_WORKER_N_GPU_LAYERS=999
```

Manager-only:

```env
DOCKER_HOST=unix:///var/run/docker.sock
HF_TOKEN=
```

Router-only:

```env
MANAGER_BASE_URL=http://manager:8091
```

## 12. Downloader integration

Manager can download models directly using `huggingface_hub`, or it can invoke the dedicated downloader container.

Recommended v1 for simplicity:

```text
manager image includes huggingface_hub
manager downloads directly into mounted /models
```

The existing `docker/hf-downloader` remains useful for manual `make download` and deployment-time model prefetching.

## 13. Cancellation requirements

Router must propagate disconnects.

For streaming requests:

1. client opens stream to router;
2. router opens stream to worker;
3. router forwards SSE chunks;
4. client disconnects;
5. router closes upstream worker stream;
6. worker releases slot.

Probe requirement:

```text
start long stream through router
cancel after 1-2 seconds
query worker /slots
assert no slot is_processing=true
```

## 14. Implementation phases

### Phase 1: Manager backend

Implement internal manager API:

- `GET /control/models`
- `GET /control/instances`
- `POST /control/ensure-running`
- `DELETE /control/instances/{slot}`

No public router changes yet.

### Phase 2: Router

Implement OpenAI-compatible router:

- `/v1/models`
- `/v1/chat/completions`
- `/v1/responses`
- `/health`

Router calls manager for `ensure-running`.

### Phase 3: llamactl

Implement CLI wrapper around manager:

```bash
llamactl models list
llamactl models download <model-ref>
llamactl instances list
llamactl instances stop <slot>
```

### Phase 4: Probes and schemas

Extend schema/API probes to cover:

- manager API;
- router `/v1/models` catalog semantics;
- cold start;
- capacity rejection;
- cancellation through router.

### Phase 5: Advanced policies

Only after v1 is stable:

- idle TTL;
- LRU eviction;
- load-aware routing from `/slots`;
- task-aware routing;
- model aliases;
- embeddings-specific workers.
