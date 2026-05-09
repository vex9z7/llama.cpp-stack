# Model Router / Orchestration Layer Design

> Update: dynamic lifecycle should be implemented through one public gateway service plus a fixed worker-agent pool. The gateway contains separate router and manager modules, but does not expose a public control API and does not mount the Docker socket. See `docs/dynamic-model-manager-design.md`.


## 1. Purpose

This document defines the next architecture direction for `llama.cpp-stack`: a lightweight model router and orchestration layer in front of multiple `llama-server` backends.

The goal is not to turn this project into a large inference platform. The goal is to keep the system local, debuggable, and controllable while adding the minimum control plane needed for:

- multiple simultaneously available models;
- OpenAI-compatible routing by `model` field;
- dynamic model lifecycle management;
- request cancellation propagation;
- future Pipecat / voice pipeline integration;
- future task-aware routing and edge-node experiments.

Current phase remains single-model capable. This design describes the next phase.

## 2. Target architecture

```text
Client / Pipecat / Agents
        |
        v
Gateway service
  - OpenAI-compatible router module
  - internal manager module
  - lazy downloader/catalog module
        |
        +--> worker-agent 0 -> llama-server qwen3-4b
        +--> worker-agent 1 -> llama-server qwen3-8b
        +--> worker-agent 2 -> llama-server coder
```

The gateway exposes one stable external API surface:

```text
/v1/chat/completions
/v1/completions
/v1/responses
/v1/models
/v1/embeddings
/health
```

Internally, each loaded model remains a normal `llama-server` process managed by a worker-agent container, with its own:

- model file;
- alias;
- port;
- context size;
- parallel slot count;
- backend type, e.g. Vulkan/CPU/CUDA;
- lifecycle state.

## 3. Design principles

### 3.1 Keep llama-server as the inference primitive

Each loaded model should still be served by a direct `llama-server` process inside a worker-agent container.

Reasons:

- request lifecycle behavior is known and testable;
- client disconnect cancellation works;
- `/slots` exposes useful operational state;
- backend-specific settings remain close to the backend;
- failures are isolated per model.

### 3.2 Router should be thin

The router should not implement inference. It should:

- parse OpenAI-compatible requests;
- choose a backend;
- proxy HTTP/SSE traffic;
- propagate cancellation;
- expose OpenAI-compatible model metadata;
- delegate lifecycle decisions to the internal manager module.

### 3.3 Prefer explicit model metadata over env var sprawl

The gateway uses structured catalog metadata rather than per-model environment variables.

### 3.4 Separate routing from lifecycle management inside one service

Conceptually there are two subsystems:

```text
Router
  - request path
  - proxying
  - streaming
  - cancellation

Runtime Manager
  - models
  - downloads
  - instance start/stop
  - health probes
  - state reconciliation
```

For v1 implementation, these live in one gateway service/container. The boundary is an in-process interface, not a public `/control/*` API. Keeping the modules separate lets us split them later without exposing control-plane details now.

## 4. Components

## 4.1 Model catalog

The catalog should stay deliberately simple and Hugging Face CLI friendly. It describes model sources only, not runtime parameters, aliases, routes, ports, or request overrides.

Suggested file:

```text
models/catalog.toml
```

Example:

```toml
[[models]]
repo = "Qwen/Qwen3-4B-GGUF"
quant = "Q4_K_M"

[[models]]
repo = "Qwen/Qwen3-8B-GGUF"
quant = "Q4_K_M"

[[models]]
repo = "Qwen/Qwen2.5-Coder-7B-Instruct-GGUF"
quant = "Q4_K_M"
```

Rules:

- `repo` maps directly to `hf download <repo>`;
- `quant` derives the file filter `*<quant>*.gguf`;
- the model id is derived as `<repo>/<quant>`, for example `Qwen/Qwen3-4B-GGUF/Q4_K_M`;
- optional `pattern` can override the derived glob for unusual repos;
- optional `file` can pin an exact filename;
- download creates a stable symlink `models/hf/<repo>/<quant>.gguf` to the actual HF filename.

Runtime concerns live elsewhere. For example, `alias`, `port`, `ctx_size`, and `parallel` belong to runtime/router config, not the source catalog.

## 4.2 Local model store

Model files remain under:

```text
models/*.gguf
```

The catalog describes expected files. The model manager can report:

```text
available: file exists locally
missing: catalog entry exists but file not downloaded
running: one or more instances currently serve it
```

Possible commands:

```bash
llamactl models list
llamactl models local
llamactl models download Qwen/Qwen3-4B-GGUF/Q4_K_M
llamactl models inspect Qwen/Qwen3-4B-GGUF/Q4_K_M
```

## 4.3 Runtime instance

An instance is a live `llama-server` process/container.

Instance fields:

```json
{
  "id": "qwen3-4b-a",
  "model_id": "Qwen/Qwen3-4B-GGUF/Q4_K_M",
  "model_file": "Qwen3-4B-Q4_K_M.gguf",
  "alias": "qwen3-4b-local",
  "backend": "vulkan",
  "host": "127.0.0.1",
  "port": 8081,
  "container": "llama-qwen3-4b-a",
  "ctx_size": 8192,
  "parallel": 2,
  "n_gpu_layers": 999,
  "state": "running"
}
```

State file:

```text
.runtime/instances.json
```

This state file should be treated as a cache. Docker/container state is source-of-truth for running processes.

Possible commands:

```bash
llamactl instances start Qwen/Qwen3-4B-GGUF/Q4_K_M --port 8081
llamactl instances stop qwen3-4b-a
llamactl instances list
llamactl instances logs qwen3-4b-a
llamactl instances probe qwen3-4b-a
```

## 4.4 Router config

The router needs to map public model names to backends.

Initial config can be generated from running instances:

```json
{
  "routes": [
    {
      "model": "qwen3-4b-local",
      "backend_url": "http://127.0.0.1:8081",
      "model_id": "Qwen/Qwen3-4B-GGUF/Q4_K_M",
      "kind": "chat",
      "default_request_overrides": {
        "chat_template_kwargs": {
          "enable_thinking": false
        }
      }
    }
  ]
}
```

Later, route selection can become policy-based.

## 4.5 Router model-to-instance semantics

One loaded worker instance corresponds to one routable model in the router.

```text
router model name -> one loaded worker instance -> one llama-server subprocess
```

Requests for the same router model are always forwarded to the same loaded worker instance. If that worker's internal `llama-server` slots are busy, requests may wait inside the worker's `llama-server`; router does not expose or reject based on slots.

Router only rejects for capacity when the requested model is not currently loaded and no idle worker instance is available to load it.

## 5. Router behavior

## 5.1 `/v1/models`

Router returns an aggregate model list.

For OpenAI compatibility, response should include:

```json
{
  "object": "list",
  "data": [
    {
      "id": "qwen3-4b-local",
      "object": "model",
      "owned_by": "llama.cpp-stack"
    }
  ]
}
```

It may also include internal metadata under extension fields, but the OpenAI-compatible shape should be preserved.

## 5.2 `/v1/chat/completions`

Routing algorithm v1:

1. Parse JSON body.
2. Read `model`.
3. Find a running backend whose alias/model route matches.
4. Apply route default request overrides, unless already provided.
5. Proxy request to backend `/v1/chat/completions`.
6. Stream response back if `stream: true`.
7. If client disconnects, close upstream request immediately.

Important default override for Qwen3:

```json
{
  "chat_template_kwargs": {
    "enable_thinking": false
  }
}
```

This avoids wasting tokens on reasoning for short agent/tool/JSON requests.

## 5.3 `/v1/responses`

Same routing as chat completions, proxying to backend `/v1/responses`.

Default overrides should also be applied.

## 5.4 `/v1/completions`

Legacy completions can be routed by `model`, but should be considered lower priority for agent integrations.

## 5.5 `/v1/embeddings`

Embeddings should probably be a separate instance type.

Reason:

- requires `llama-server --embeddings`;
- embedding models may need different pooling settings;
- chat models are not necessarily useful embedding models.

Routing should check `kind = "embedding"` or capability metadata.

## 5.6 `/health`

Router health should report router status and backend status.

Example:

```json
{
  "status": "ok",
  "backends": [
    {
      "model": "qwen3-4b-local",
      "url": "http://127.0.0.1:8081",
      "status": "ok"
    }
  ]
}
```

## 6. Request cancellation

This is a hard requirement.

For streaming requests:

- client connects to router;
- router opens streaming request to backend;
- router forwards SSE chunks;
- if client disconnects, router must close the upstream HTTP response/session;
- backend `llama-server` should then release the slot.

Validation:

1. Start long streaming request through router.
2. Cancel client after 1-2 seconds.
3. Query backend `/slots`.
4. Assert no slot is still processing.

This should become part of `probe_api_schemas.py` once router exists.

## 7. Dynamic lifecycle model

V1 dynamic lifecycle is implemented inside the gateway service, not through a separately exposed control API.

```text
public client -> gateway OpenAI API
                 |
                 +-> router module
                 +-> manager module
                 +-> worker clients
                         |
                         +-> worker-agent internal APIs
```

The public surface remains OpenAI-compatible. Internal lifecycle operations are represented as code interfaces such as:

```python
ensure_running(model_ref)
list_models()
list_workers()
unload_worker(worker_id, force=False)
```

A future `llamactl` may call these interfaces via `docker compose exec gateway ...` or a disabled-by-default admin endpoint, but `/control/*` should not be part of the default public deployment.

## 8. Recommended implementation sequence

### Phase A: Gateway router with static backends

Implement a small router that reads a static route config:

```toml
[[routes]]
model = "qwen3-4b-local"
backend_url = "http://127.0.0.1:8081"

[routes.default_request_overrides]
chat_template_kwargs = { enable_thinking = false }
```

It supports:

- `/v1/models`
- `/v1/chat/completions`
- streaming proxy
- cancellation propagation
- `/health`

This validates the critical request path before lifecycle management.

### Phase B: Model catalog migration

Use `models/catalog.toml` as the source catalog.

Keep TSV temporarily or generate it from TOML for compatibility.

### Phase C: Gateway manager module + worker agents

Add a fixed worker-agent pool in Compose and implement the gateway manager module:

- discover workers from `WORKER_BASE_URLS`;
- lazily download catalog models;
- load a model into an idle worker on first request;
- reject with 429 when no idle worker exists;
- do not expose `/control/*` publicly.

### Phase D: Optional llamactl

Add `llamactl` only as an operator convenience, preferably run inside the gateway container:

```bash
docker compose exec gateway llamactl models list
docker compose exec gateway llamactl workers list
docker compose exec gateway llamactl workers unload worker-0
```

### Phase E: Policy routing

Add routing policy:

- exact `model` match;
- fallback default model;
- task kind, e.g. `chat`, `code`, `embedding`;
- load-aware routing using `/slots`;
- latency-aware routing;
- edge-node aware routing.

## 9. First router implementation sketch

Use Python initially for speed and clarity.

Candidate stack:

- FastAPI or Starlette;
- httpx for async upstream proxy;
- uvicorn;
- TOML route config.

Why not Nginx/Caddy only:

- need to inspect JSON `model` field;
- need per-model request overrides;
- need cancellation-aware streaming proxy;
- future lifecycle management needs application logic.

Minimal files:

```text
router/
  app.py
  config.py
  proxy.py
  schemas.py
configs/router.example.toml
scripts/llamactl.py
```

Initial router config:

```toml
[router]
host = "0.0.0.0"
port = 8090

[[routes]]
model = "qwen3-4b-local"
backend_url = "http://127.0.0.1:8081"
kind = "chat"

[routes.default_request_overrides]
chat_template_kwargs = { enable_thinking = false }
```

## 10. Open questions

1. Should router expose only OpenAI-compatible endpoints, or also proxy llama.cpp-native endpoints per backend?
2. Should `llamactl` use `docker compose exec gateway` only, or should there be an opt-in localhost/admin endpoint?
3. How should worker-agent images locate the llama-server binary across upstream image variants?
4. Which models should be eligible for lazy auto-start by default?
5. How should model aliases be standardized?
6. Should Qwen3 thinking default to off globally, with opt-in thinking routes?
7. How much state should be stored in files vs discovered from Docker?

## 11. Current recommendation

Build in this order:

1. Router-only, static backend routes.
2. Add schema probe for gateway API compatibility.
3. Add fixed worker-agent pool.
4. Add gateway manager module with lazy download/load.
5. Add optional `llamactl` and later policy/load routing.

This keeps the critical serving path testable before adding dynamic orchestration complexity.

## 12. Implementation stack decision

Use Go for the gateway and worker-agent implementation. The default stack is:

```text
HTTP server/client  net/http
Streaming proxy     explicit context-aware proxy code
Catalog TOML        github.com/pelletier/go-toml/v2
Logging             log/slog
Config              env vars + models/catalog.toml
HF download         minimal Go downloader in gateway manager
Worker lifecycle    os/exec + syscall
State               in-memory, rebuilt from worker status
```

The detailed implementation stack and readiness checklist live in `docs/dynamic-model-manager-design.md`.
