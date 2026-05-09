# Model Router / Orchestration Layer Design

> Update: dynamic lifecycle should be implemented through a manager + fixed worker-agent pool, with no Docker socket mounted into application containers. See `docs/dynamic-model-manager-design.md` for the worker-agent design with `LLAMA_MAX_INSTANCES` and reject-on-full v1 policy.


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
Model Router / Orchestration Layer
        |
        +--> llama-server qwen3-4b
        +--> llama-server qwen3-8b
        +--> llama-server coder
        +--> llama-server embedding-model
```

The router exposes one stable external API surface:

```text
/v1/chat/completions
/v1/completions
/v1/responses
/v1/models
/v1/embeddings
/health
```

Internally, each backend remains a normal `llama-server` instance with its own:

- model file;
- alias;
- port;
- context size;
- parallel slot count;
- backend type, e.g. Vulkan/CPU/CUDA;
- lifecycle state.

## 3. Design principles

### 3.1 Keep llama-server as the inference primitive

Each loaded model should still be served by a direct `llama-server` process/container.

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
- expose model/instance metadata;
- optionally start/stop backend instances.

### 3.3 Prefer explicit model metadata over env var sprawl

The current single-instance `.env` is acceptable for one model. Multi-model routing needs structured metadata.

Use a catalog file rather than many environment variables.

### 3.4 Separate routing from lifecycle management, but allow one process initially

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

For early implementation, these can live in one small service or CLI package. The internal interfaces should remain separate so they can be split later.

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

## 7. Dynamic lifecycle options

There are two implementation paths.

### 7.1 CLI-first runtime manager

Start with a CLI:

```bash
llamactl models download Qwen/Qwen3-4B-GGUF/Q4_K_M
llamactl instances start Qwen/Qwen3-4B-GGUF/Q4_K_M --port 8081
llamactl instances list
llamactl router config
```

Pros:

- simple;
- no daemon required;
- works well for local experiments;
- easy to debug Docker commands.

Cons:

- router cannot automatically start missing models unless it shells out or shares library code;
- less convenient for remote management.

### 7.2 Daemon/control API

Run a small manager service:

```http
GET  /control/models
POST /control/models/{id}/download
GET  /control/instances
POST /control/instances
DELETE /control/instances/{id}
POST /control/instances/{id}/probe
```

Pros:

- dynamic model start/stop via API;
- router and manager can coordinate;
- foundation for UI/automation.

Cons:

- more security concerns;
- must protect Docker socket access;
- more moving parts.

## 8. Recommended implementation sequence

### Phase A: Router-only, static backends

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

### Phase C: CLI runtime manager

Add `llamactl`:

```bash
llamactl models list
llamactl models download <model-id>
llamactl instances start <model-id>
llamactl instances stop <instance-id>
llamactl instances list
```

Use Docker directly for dynamic instance lifecycle.

### Phase D: Router + manager integration

Router can read running instance state and update routes dynamically.

Possible modes:

- reload route file on SIGHUP;
- poll `.runtime/instances.json`;
- query manager API;
- receive manager events.

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
2. Should model lifecycle be local-only CLI first, or HTTP control API first?
3. Should Docker socket access live inside the router container, or only on the host CLI?
4. Should the router auto-start models on first request, or only route to already-running instances?
5. How should model aliases be standardized?
6. Should Qwen3 thinking default to off globally, with opt-in thinking routes?
7. How much state should be stored in files vs discovered from Docker?

## 11. Current recommendation

Build in this order:

1. Router-only, static backend routes.
2. Add schema probe for router API compatibility.
3. Add dynamic `llamactl` lifecycle manager.
4. Connect router to runtime state.
5. Add policy/load routing.

This keeps the critical serving path testable before adding dynamic orchestration complexity.
