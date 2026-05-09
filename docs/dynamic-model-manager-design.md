# Dynamic Model Management Design

## 1. Purpose

This document defines the dynamic model management architecture for `llama.cpp-stack`.

The goal is to support many cataloged models while only running a bounded number of loaded `llama-server` processes at a time.

Important distinction:

```text
cataloged model  != downloaded model != loaded worker process
```

A model in `models/catalog.toml` is only known/allowed. It should not automatically consume RAM/VRAM. A model becomes expensive only when a worker-agent starts a `llama-server` subprocess for it.

## 2. Key decisions

### 2.1 No Docker socket

Application containers must not mount `/var/run/docker.sock`.

Reasons:

- Docker socket is usually host-root privileged;
- rootless/atomic systems may not expose a compatible socket;
- exposing the socket to an application container is too broad a security boundary;
- the target deployment should work with all services containerized and minimal host mutation.

Therefore dynamic model management is not implemented by dynamically creating Docker containers.

Instead:

```text
Compose starts a fixed pool of worker-agent containers.
Gateway controls those worker agents over internal HTTP.
Each worker agent dynamically starts/stops one llama-server subprocess inside its own container.
```

### 2.2 One public service: gateway

There is no separate public control-plane service in v1.

Externally, clients see one service:

```text
Gateway service
  - OpenAI-compatible API
  - /health
```

Internally, the gateway still has clear module boundaries:

```text
gateway process/container
  router module
    - OpenAI request parsing
    - request proxying
    - streaming and cancellation

  manager module
    - catalog state
    - lazy model download
    - worker allocation
    - load/unload decisions

  catalog module
    - models/catalog.toml parsing
    - model_ref -> local path mapping

  worker client module
    - calls worker-agent internal API

  proxy module
    - forwards HTTP/SSE to llama-server
```

The router and manager are logical boundaries, not separately exposed network services. This keeps deployment simple while preserving the option to split the control plane later if needed.

## 3. High-level architecture

```text
Client / Pipecat / Agents
        |
        v
Gateway container
  - public OpenAI-compatible API
  - internal router module
  - internal manager module
  - internal catalog/downloader module
  - no Docker socket
        |
        | internal Docker network HTTP
        +--> Worker Agent 0 container
        |      - idle OR llama-server subprocess for model A
        |
        +--> Worker Agent 1 container
               - idle OR llama-server subprocess for model B
```

`LLAMA_WORKER_POOL_SIZE` corresponds to the number of worker-agent containers available, not the number of cataloged models.

In Compose, the pool size is materialized as explicit services:

```text
gateway
worker-0
worker-1
...
```

The gateway does not create or delete containers. It only controls the process inside each pre-created worker container.

## 4. Core constraints

### 4.1 One llama-server process runs one model

A `llama-server` process starts with one model path:

```bash
llama-server --model /models/hf/Qwen/Qwen3-4B-GGUF/Q4_K_M.gguf
```

It can handle multiple concurrent requests for that same model via llama.cpp slots, but it is not a multi-model runtime.

Therefore dynamic model management means dynamically starting/stopping `llama-server` subprocesses inside pre-created worker-agent containers.

### 4.2 Worker-agent containers are fixed at deployment time

Compose owns container lifecycle:

```text
gateway
worker-0
worker-1
...
```

The gateway owns model process lifecycle inside the workers:

```text
worker idle -> load model -> llama-server running
worker running -> unload model -> idle
```

### 4.3 Capacity policy v1: loaded-model residency

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
2. If that worker's internal `llama-server` request slots are busy, the request may wait/queue inside that worker. Gateway does not reject based on `llama-server` slots.
3. If requested model is not loaded and an idle worker exists, lazily download it if needed, then load it into the idle worker.
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

The catalog defines what may be downloaded/loaded. It does not define ports or worker assignment.

## 5.2 Gateway router module

The router module is the public request path.

Responsibilities:

- expose OpenAI-compatible endpoints;
- parse request bodies and read `model`;
- call the manager module to ensure the requested model is loaded;
- proxy requests to the selected worker's `llama-server` port;
- proxy streaming SSE responses;
- close upstream connections on client disconnect;
- translate capacity/startup errors into OpenAI-shaped HTTP errors where possible.

Router module does not:

- download models directly;
- decide worker allocation details;
- start/stop `llama-server` processes directly;
- expose worker ids, worker topology, or llama.cpp `/slots` to public clients.

## 5.3 Gateway manager module

The manager module is internal library/application logic inside the gateway process. It is not a public HTTP service in v1.

Responsibilities:

- read `models/catalog.toml`;
- track catalog/download/running state;
- lazily download models into `models/hf/...` when first requested;
- poll worker-agent states;
- allocate idle workers;
- call worker-agent `/worker/load` and `/worker/unload`;
- return an internal capacity error when no worker is available;
- provide a stable in-process interface for router code and future `llamactl` code.

Manager module does not:

- access Docker socket;
- expose public `/control/*` endpoints;
- proxy user inference requests.

Suggested internal interface, expressed as functions rather than public HTTP endpoints:

```python
list_models() -> list[ModelStatus]
ensure_running(model_ref: str) -> RunningBackend
list_workers() -> list[WorkerStatus]
unload_worker(worker_id: str, force: bool = False) -> None
```

## 5.4 Worker agent

A worker-agent container wraps one possible `llama-server` subprocess.

Responsibilities:

- expose a small internal-only worker API on the Docker network;
- start `llama-server` with a requested model path;
- stop `llama-server` on unload;
- report current state;
- expose the child `llama-server` inference port to the gateway;
- prevent loading a second model while already running one;
- terminate the child process cleanly on container shutdown.

Worker agent does not:

- download models;
- choose routing policy;
- access Docker socket;
- expose public APIs outside the Compose network.

## 5.5 llamactl

`llamactl` remains optional.

Since v1 does not expose a public control API, `llamactl` can be implemented later in one of two ways:

1. `docker compose exec gateway llamactl ...`, calling the gateway manager module locally; or
2. a private/admin-only gateway endpoint bound to localhost or disabled by default.

Do not make a public `/control/*` API part of the default deployment.

## 6. Worker agent API v1

Worker API is internal-only. It should not be published through the gateway or reverse proxy.

Suggested control URL:

```text
http://worker-0:8092
```

The child `llama-server` listens inside the same container on:

```text
http://worker-0:8080
```

### 6.1 `GET /worker/status`

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

### 6.2 `POST /worker/load`

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

### 6.3 `POST /worker/unload`

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

### 6.4 `GET /worker/health`

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

## 7. Gateway request flow

### 7.1 Model already loaded

```text
POST /v1/chat/completions model=X
        |
        v
gateway router -> manager.ensure_running(X)
        |
        v
manager returns existing worker inference_url
        |
        v
gateway proxies to inference_url/v1/chat/completions
```

### 7.2 Model cold, idle worker exists

```text
gateway router -> manager.ensure_running(X)
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
gateway proxies request
```

### 7.3 Model already loaded but busy

```text
gateway router -> manager.ensure_running(X)
        |
        v
manager returns existing worker inference_url
        |
        v
gateway proxies to the same worker
        |
        v
worker llama-server handles internal request slots / queueing
```

Gateway hides `llama-server` slots and does not reject just because the loaded worker is busy.

### 7.4 Model cold, all workers occupied

```text
gateway router -> manager.ensure_running(X)
        |
        v
manager returns internal capacity error
        |
        v
gateway returns HTTP 429 to client
```

This rejection only means there is no idle worker instance to load a new model. It does not mean per-model request slots are full.

No eviction in v1.

## 8. Public API surface

Default public API:

```text
GET  /health
GET  /v1/models
POST /v1/chat/completions
POST /v1/completions
POST /v1/responses
POST /v1/embeddings  # only when embedding-capable workers exist
```

Do not expose:

```text
/control/*
/worker/*
/slots
/backend worker URLs
```

`/v1/models` may return catalog models, not only loaded models:

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

This is an intentional extension: the model may be cold-startable even if not currently loaded.

## 9. Compose topology

Initial dynamic service set:

```text
gateway
worker-0
worker-1
```

Example shape:

```yaml
services:
  gateway:
    build: ./gateway
    ports:
      - "8090:8090"
    volumes:
      - ./models:/models:rw,Z
    environment:
      WORKER_BASE_URLS: http://worker-0:8092,http://worker-1:8092
      LLAMA_EVICTION_POLICY: reject
    depends_on:
      - worker-0
      - worker-1

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

No service mounts Docker socket. Worker APIs are only reachable on the Compose network.

## 10. Environment variables

Suggested gateway env:

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

Worker-only env:

```env
WORKER_ID=worker-0
LLAMA_SERVER_PORT=8080
WORKER_AGENT_PORT=8092
```

`LLAMA_WORKER_POOL_SIZE` is documentation/validation for the gateway. In plain Docker Compose, the actual pool is still the number of declared worker services.

## 11. Downloader integration

Gateway manager module owns lazy download.

Recommended v1:

```text
gateway includes huggingface_hub
gateway downloads directly into mounted /models
worker containers mount /models read-only
gateway loads worker only after model file exists
```

There should be no host-side `make download` deployment prerequisite.

A manual prefetch command can be added later through `docker compose exec gateway llamactl ...`, but that should call the same manager code path as request-time lazy download.

## 12. Cancellation requirements

Gateway must propagate disconnects.

For streaming requests:

1. client opens stream to gateway;
2. gateway opens stream to worker's `llama-server`;
3. gateway forwards SSE chunks;
4. client disconnects;
5. gateway closes upstream stream;
6. worker's `llama-server` releases slot.

Probe requirement:

```text
start long stream through gateway
cancel after 1-2 seconds
query worker llama-server /slots internally
assert no slot is_processing=true
```

The probe may use internal worker URLs from the test environment, but the production gateway should not expose `/slots` publicly.

## 13. Implementation phases

### Phase 1: Gateway router module

Implement OpenAI-compatible gateway endpoints:

- `/v1/models`
- `/v1/chat/completions`
- `/v1/responses`
- `/health`

Start with one static backend if needed to validate proxying and cancellation.

### Phase 2: Worker agent

Implement worker-agent API:

- `GET /worker/status`
- `GET /worker/health`
- `POST /worker/load`
- `POST /worker/unload`

It should start/stop `llama-server` subprocesses inside its own container.

### Phase 3: Gateway manager module

Implement manager code inside the gateway process:

- catalog parsing;
- local file detection;
- lazy Hugging Face download;
- worker discovery;
- `ensure_running(model_ref)`;
- capacity rejection when no idle worker exists.

### Phase 4: Probes and schemas

Extend schema/API probes to cover:

- gateway `/v1/models` catalog semantics;
- cold start;
- capacity rejection;
- cancellation through gateway;
- internal worker health/load/unload in test-only mode.

### Phase 5: Optional operations CLI

Add `llamactl` only after the internal manager interface is stable.

Preferred operator shape:

```bash
docker compose exec gateway llamactl models list
docker compose exec gateway llamactl workers list
docker compose exec gateway llamactl workers unload worker-0
```

### Phase 6: Advanced policies

Only after v1 is stable:

- idle TTL;
- LRU eviction;
- load-aware routing from internal `/slots` probes;
- task-aware routing;
- model aliases;
- embeddings-specific workers.
