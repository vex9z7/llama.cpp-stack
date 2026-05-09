# Dynamic Model Manager Design

## 1. Purpose

This document defines the dynamic model management architecture for `llama.cpp-stack`.

The goal is to support many cataloged models while only running a bounded number of loaded `llama-server` processes at a time.

Important distinction:

```text
cataloged model  != downloaded model != loaded worker process
```

A model in `models/catalog.toml` is only known/allowed. It should not automatically consume RAM/VRAM. A model becomes expensive only when a worker-agent starts a `llama-server` subprocess for it.

## 2. Key decision: no Docker socket

The manager and router must not mount `/var/run/docker.sock`.

Reasons:

- Docker socket is usually host-root privileged;
- rootless/atomic systems may not expose a compatible socket;
- exposing the socket to an application container is too broad a security boundary;
- the target deployment should work with all services containerized and minimal host mutation.

Therefore dynamic model management is not implemented by dynamically creating Docker containers.

Instead:

```text
Compose starts a fixed pool of worker-agent containers.
Manager controls those worker agents over HTTP.
Each worker agent dynamically starts/stops a llama-server subprocess inside its own container.
```

## 3. High-level architecture

```text
Client / Pipecat / Agents
        |
        v
Router container
  - public OpenAI-compatible API
  - no Docker socket
  - proxies inference requests
        |
        | internal HTTP
        v
Manager container
  - catalog and local model state
  - worker-slot allocation
  - model download
  - no Docker socket
        |
        | internal HTTP
        +--> Worker Agent 0 container
        |      - idle OR llama-server subprocess for model A
        |
        +--> Worker Agent 1 container
               - idle OR llama-server subprocess for model B
```

`LLAMA_WORKER_POOL_SIZE` corresponds to the number of worker-agent containers available, not the number of cataloged models.

## 4. Core constraints

## 4.1 One llama-server process runs one model

A `llama-server` process starts with one model path:

```bash
llama-server --model /models/hf/Qwen/Qwen3-4B-GGUF/Q4_K_M.gguf
```

It can handle multiple concurrent requests for that same model via slots, but it is not a multi-model runtime.

Therefore dynamic model management means dynamically starting/stopping `llama-server` subprocesses inside pre-created worker-agent containers.

## 4.2 Worker-agent containers are fixed at deployment time

Compose owns container lifecycle:

```text
router
manager
worker-0
worker-1
...
```

The manager owns model process lifecycle inside the workers:

```text
worker idle -> load model -> llama-server running
worker running -> unload model -> idle
```

## 4.3 Capacity policy v1: loaded-model residency

In router semantics, one loaded worker instance corresponds to one routable model.

```text
worker instance = one worker-agent container with one loaded llama-server subprocess
router model    = the public model name routed to that worker instance
```

Capacity is about how many different models can be resident at once, not how many requests can run at once.

Initial policy:

```text
LLAMA_EVICTION_POLICY=reject
```

Rules:

1. If requested model is already loaded in a worker, route to that worker.
2. If that worker's internal `llama-server` request slots are busy, the request may wait/queue inside that worker. Router does not reject based on `llama-server` slots.
3. If requested model is not loaded and an idle worker exists, load it into the idle worker.
4. If requested model is not loaded and no idle worker exists, return HTTP 429.
5. Do not evict or unload running models automatically in v1.

## 5. Component responsibilities

## 5.1 Model catalog

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

The catalog only defines what may be downloaded/loaded. It does not define OpenAI model names, ports, context size, parallelism, or routes.

## 5.2 Worker agent

A worker-agent container wraps one possible `llama-server` subprocess.

Responsibilities:

- expose a small internal control API;
- start `llama-server` with a requested model path;
- stop `llama-server` on unload;
- report current state;
- proxy or expose the child `llama-server` inference port;
- prevent loading a second model while already running one;
- terminate the child process cleanly on container shutdown.

Worker agent does not:

- download models;
- choose routing policy;
- access Docker socket.

## 5.3 Manager

The manager is an internal control-plane service.

Responsibilities:

- read `models/catalog.toml`;
- report catalog/download/running state;
- download models into `models/hf/...`;
- track worker-agent states;
- allocate idle workers;
- call worker `/worker/load` and `/worker/unload`;
- return 429 when no worker is available;
- provide a stable API for router and `llamactl`.

Manager does not access Docker socket.

## 5.4 Router

The router is the public request gateway.

Responsibilities:

- expose OpenAI-compatible endpoints;
- call manager to ensure a model is loaded;
- proxy requests to the selected worker's `llama-server` port;
- proxy streaming SSE responses;
- close upstream connections on client disconnect;
- return capacity/startup errors from manager to clients.

Router does not:

- access Docker socket;
- download models directly;
- start/stop `llama-server` processes directly.

## 5.5 llamactl

`llamactl` is a CLI wrapper around the manager API.

Example commands:

```bash
llamactl models list
llamactl models download Qwen/Qwen3-4B-GGUF/Q4_K_M
llamactl workers list
llamactl workers unload worker-0
```

## 6. Worker agent API v1

Worker API is internal-only. It should not be exposed publicly.

Suggested control URL:

```text
http://worker-0:8092
```

The child `llama-server` listens inside the same container on:

```text
http://worker-0:8080
```

## 6.1 `GET /worker/status`

Idle response:

```json
{
  "id": "worker-0",
  "state": "idle",
  "model_ref": null,
  "model_path": null,
  "inference_url": null
}
```

Running response:

```json
{
  "id": "worker-0",
  "state": "running",
  "model_ref": "Qwen/Qwen3-4B-GGUF/Q4_K_M",
  "model_path": "hf/Qwen/Qwen3-4B-GGUF/Q4_K_M.gguf",
  "model_name": "Qwen/Qwen3-4B-GGUF/Q4_K_M",
  "inference_url": "http://worker-0:8080",
  "pid": 123
}
```

## 6.2 `POST /worker/load`

Starts `llama-server` inside the worker container.

Request:

```json
{
  "model_ref": "Qwen/Qwen3-4B-GGUF/Q4_K_M",
  "model_path": "hf/Qwen/Qwen3-4B-GGUF/Q4_K_M.gguf",
  "model_name": "Qwen/Qwen3-4B-GGUF/Q4_K_M",
  "ctx_size": 8192,
  "parallel": 1,
  "threads_http": -1,
  "n_gpu_layers": 999,
  "extra_args": ""
}
```

Worker command shape:

```bash
/app/llama-server \
  --host 0.0.0.0 \
  --port 8080 \
  --model /models/hf/Qwen/Qwen3-4B-GGUF/Q4_K_M.gguf \
  --alias Qwen/Qwen3-4B-GGUF/Q4_K_M \
  --ctx-size 8192 \
  --parallel 1 \
  --threads-http -1 \
  --n-gpu-layers 999 \
  --cont-batching \
  --cache-prompt
```

Success response:

```json
{
  "status": "loaded",
  "worker": "worker-0",
  "inference_url": "http://worker-0:8080"
}
```

If already running another model:

```http
HTTP/1.1 409 Conflict
```

```json
{
  "error": {
    "type": "worker_busy",
    "message": "Worker already has a loaded model"
  }
}
```

## 6.3 `POST /worker/unload`

Stops the child `llama-server` process.

Default behavior should refuse unload if active requests are present. Worker can check local `llama-server` `/slots` before terminating.

Force request:

```json
{
  "force": true
}
```

Success response:

```json
{
  "status": "unloaded",
  "worker": "worker-0"
}
```

## 6.4 `GET /worker/health`

Reports worker-agent health, not necessarily model readiness.

```json
{
  "status": "ok",
  "state": "idle"
}
```

When running, it may include child health:

```json
{
  "status": "ok",
  "state": "running",
  "llama_server": {
    "status": "ok"
  }
}
```

## 7. Manager API v1

Manager API is internal-only. Router and `llamactl` call this API.

Suggested URL:

```text
http://manager:8091
```

## 7.1 `GET /control/models`

Returns catalog models with downloaded/running status.

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

## 7.2 `GET /control/workers`

Returns current worker-agent states.

Example response:

```json
{
  "max_instances": 2,
  "workers": [
    {
      "id": "worker-0",
      "state": "running",
      "model_ref": "Qwen/Qwen3-4B-GGUF/Q4_K_M",
      "model_name": "Qwen/Qwen3-4B-GGUF/Q4_K_M",
      "model_path": "hf/Qwen/Qwen3-4B-GGUF/Q4_K_M.gguf",
      "inference_url": "http://worker-0:8080"
    },
    {
      "id": "worker-1",
      "state": "idle"
    }
  ]
}
```

## 7.3 `POST /control/ensure-running`

Ensures a model has a loaded worker.

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
  "worker": "worker-0",
  "inference_url": "http://worker-0:8080"
}
```

If cold-started:

```json
{
  "status": "loaded",
  "worker": "worker-1",
  "inference_url": "http://worker-1:8080"
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
    "code": "no_idle_worker",
    "message": "Requested model is not loaded and no idle worker instance is available"
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
    "code": "worker_load_failed",
    "message": "Worker failed to load llama-server model"
  }
}
```

## 7.4 `POST /control/models/{model_ref}/download`

Downloads a catalog model if missing.

Because `model_ref` contains `/`, this endpoint may be easier to implement as a JSON body instead of a path parameter:

```http
POST /control/download
```

```json
{
  "model_ref": "Qwen/Qwen3-4B-GGUF/Q4_K_M"
}
```

## 7.5 `POST /control/workers/{id}/unload`

Unloads a worker.

```json
{
  "force": false
}
```

## 8. Router request flow

## 8.1 Model already loaded

```text
POST /v1/chat/completions model=X
        |
        v
router -> manager ensure-running(X)
        |
        v
manager returns existing worker inference_url
        |
        v
router proxies to inference_url/v1/chat/completions
```

## 8.2 Model cold, idle worker exists

```text
router -> manager ensure-running(X)
        |
        v
manager downloads if needed
manager calls idle worker /worker/load
worker starts llama-server subprocess
worker waits /health
        |
        v
manager returns inference_url
        |
        v
router proxies request
```

## 8.3 Model already loaded but busy

```text
router -> manager ensure-running(X)
        |
        v
manager returns existing worker inference_url
        |
        v
router proxies to the same worker
        |
        v
worker llama-server handles internal request slots / queueing
```

Router hides `llama-server` slots and does not reject just because the loaded worker is busy.

## 8.4 Model cold, all workers occupied

```text
router -> manager ensure-running(X)
        |
        v
manager returns 429 capacity_error
        |
        v
router returns 429 to client
```

This rejection only means there is no idle worker instance to load a new model. It does not mean per-model request slots are full.

No eviction in v1.

## 9. Router model semantics

A loaded worker instance corresponds to one router model.

```text
router model name -> manager loaded worker -> worker llama-server
```

Multiple requests for the same router model are forwarded to the same worker instance. If that worker is busy, the request may wait in the worker's `llama-server` scheduler/queue. Router does not expose worker ids, worker topology, or `llama-server` slots.

## 9.1 `/v1/models` semantics

The router may return catalog models, not only loaded models.

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

## 10. Compose topology

Initial service set:

```text
router
manager
worker-0
worker-1
```

Example shape:

```yaml
services:
  router:
    build: ./router
    ports:
      - "8090:8090"
    environment:
      MANAGER_BASE_URL: http://manager:8091
    depends_on:
      - manager

  manager:
    build: ./manager
    expose:
      - "8091"
    volumes:
      - ./models:/models:rw,Z
    environment:
      WORKER_BASE_URLS: http://worker-0:8092,http://worker-1:8092
      LLAMA_EVICTION_POLICY: reject

  worker-0:
    build: ./worker
    expose:
      - "8080"
      - "8092"
    volumes:
      - ./models:/models:ro,Z
    devices:
      - /dev/dri:/dev/dri

  worker-1:
    build: ./worker
    expose:
      - "8080"
      - "8092"
    volumes:
      - ./models:/models:ro,Z
    devices:
      - /dev/dri:/dev/dri
```

No service mounts Docker socket.

## 11. Environment variables

Suggested manager/router env:

```env
LLAMA_WORKER_POOL_SIZE=2
LLAMA_EVICTION_POLICY=reject
LLAMA_INSTANCE_START_TIMEOUT_SECONDS=120
WORKER_BASE_URLS=http://worker-0:8092,http://worker-1:8092
LLAMA_WORKER_CTX_SIZE=8192
LLAMA_WORKER_PARALLEL=1
LLAMA_WORKER_THREADS_HTTP=-1
LLAMA_WORKER_N_GPU_LAYERS=999
```

Router-only:

```env
MANAGER_BASE_URL=http://manager:8091
```

Worker-only:

```env
WORKER_ID=worker-0
LLAMA_SERVER_PORT=8080
WORKER_AGENT_PORT=8092
```

## 12. Downloader integration

Manager can download models directly using `huggingface_hub` into mounted `/models`.

Recommended v1:

```text
manager includes huggingface_hub
manager lazy-downloads directly into mounted /models
```

Manual prefetch should later be exposed through `llamactl`, which calls the manager API. There should be no host-side `make download` deployment prerequisite.

This keeps download ownership inside the manager and avoids a separate downloader service.

## 13. Cancellation requirements

Router must propagate disconnects.

For streaming requests:

1. client opens stream to router;
2. router opens stream to worker's `llama-server`;
3. router forwards SSE chunks;
4. client disconnects;
5. router closes upstream stream;
6. worker's `llama-server` releases slot.

Probe requirement:

```text
start long stream through router
cancel after 1-2 seconds
query worker llama-server /slots
assert no slot is_processing=true
```

## 14. Implementation phases

### Phase 1: Worker agent

Implement worker-agent API:

- `GET /worker/status`
- `GET /worker/health`
- `POST /worker/load`
- `POST /worker/unload`

It should start/stop `llama-server` subprocesses inside its own container.

### Phase 2: Manager backend

Implement manager API:

- `GET /control/models`
- `GET /control/workers`
- `POST /control/ensure-running`
- `POST /control/download`
- `POST /control/workers/{id}/unload`

### Phase 3: Router

Implement OpenAI-compatible router:

- `/v1/models`
- `/v1/chat/completions`
- `/v1/responses`
- `/health`

Router calls manager for `ensure-running`.

### Phase 4: llamactl

Implement CLI wrapper around manager:

```bash
llamactl models list
llamactl models download <model-ref>
llamactl workers list
llamactl workers unload <worker-id>
```

### Phase 5: Probes and schemas

Extend schema/API probes to cover:

- worker API;
- manager API;
- router `/v1/models` catalog semantics;
- cold start;
- capacity rejection;
- cancellation through router.

### Phase 6: Advanced policies

Only after v1 is stable:

- idle TTL;
- LRU eviction;
- load-aware routing from `/slots`;
- task-aware routing;
- model aliases;
- embeddings-specific workers.
