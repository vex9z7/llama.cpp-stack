# llama.cpp Router Mode Architecture

## 1. Status

Decision: use an application gateway as the default public entrypoint, backed by `llama-server` router mode as the dynamic model lifecycle layer.

The gateway is not a replacement for llama.cpp router mode. It is a thin product/control layer that fills the gaps router mode does not currently own for this project: catalog allowlisting, Hugging Face lazy download, preset generation/reload, public API shaping, and contract checks. If upstream router mode later implements some of these features well enough, the gateway should delegate to upstream rather than duplicate them.

Rationale:

- `llama-server` router mode is implemented inside upstream llama.cpp;
- it already manages child model instances, dynamic load/unload, autoload, LRU, and idle sleep;
- it preserves llama.cpp-native OpenAI-compatible API behavior, streaming, cancellation, Vulkan, and GGUF support;
- maintaining our own child-process lifecycle would duplicate fast-moving upstream behavior.

Router mode is currently marked experimental upstream, so the stack should keep probes and clear contract tests. However, it is still a better maintenance boundary than reimplementing the process manager ourselves.

## 2. Target architecture

```text
Client / Pipecat / Agents
        |
        v
Gateway
  - public OpenAI-compatible API
  - catalog allowlist
  - lazy Hugging Face download
  - generated router preset management
  - public endpoint shaping / hiding
  - OpenAPI / schema contract checks
  - cancellation-aware proxy
        |
        v
llama-server router mode
  - OpenAI-compatible llama.cpp API
  - dynamic model load/unload
  - model child process lifecycle
  - streaming and cancellation behavior
        |
        +--> child llama-server: model A
        +--> child llama-server: model B
        +--> child llama-server: embedding model
```

The gateway is the default public service. The llama.cpp router should normally be reachable only on the Compose/internal network or localhost for operator probes. This lets us use router mode for lifecycle management while avoiding direct public exposure of experimental management endpoints.

OpenAI compatibility adapter details are tracked in `docs/openai-compat-adapter-plan.md`. In particular, `/v1/responses` is treated as a first-class public API and should receive the same adapter/probe coverage as `/v1/chat/completions`.

## 3. Responsibility split

The deployed stack has two network services in the default path:

```text
public client -> Go gateway -> internal llama.cpp router mode -> child llama-server
```

The gateway and router are deliberately separate responsibility boundaries even
though both participate in request handling. The gateway owns product policy and
public API shape. The upstream router owns actual model process lifecycle and
llama.cpp request execution.

## 3.1 Gateway owns

The gateway is the public service and policy boundary.

Responsibilities:

- expose the public OpenAI-compatible endpoint surface;
- publish `/openapi.json` for the public gateway contract;
- enforce the catalog allowlist before any request reaches llama.cpp;
- enforce endpoint/model capability checks, for example chat vs embedding;
- resolve Hugging Face GGUF files from `models/catalog.toml`;
- lazily download missing model files;
- maintain the stable local model path layout;
- generate `models-preset.generated.ini` from downloaded catalog models;
- call router `/models?reload=1` after catalog/download/preset changes;
- leave model load/unload scheduling to llama.cpp router mode by default;
- proxy allowed inference requests to llama.cpp router mode;
- adapt known OpenAI-compatible request fields to llama.cpp/model-template
  extensions when the mapping is deterministic and tested;
- preserve request bodies and llama.cpp/OpenAI-compatible response shapes as much
  as possible;
- propagate client disconnect/cancellation through the upstream HTTP request;
- hide router internals from public clients;
- normalize gateway-originated errors into OpenAI-shaped error objects;
- emit structured logs to stdout/stderr.

Non-responsibilities:

- do not run inference;
- do not directly create child `llama-server` processes;
- do not expose `/slots`, `/props`, `/metrics`, `/models/load`, or
  `/models/unload` publicly;
- do not store logs in files;
- do not become a replacement implementation of llama.cpp router mode.

## 3.2 Gateway model manager owns

The model manager is in-process gateway logic, not a separate public control
API. It prepares models for the backend but does not schedule loaded instances.

Responsibilities:

- track catalog/download/router metadata needed by request handling;
- serialize same-model lazy downloads and preset reloads;
- ensure a requested catalog model exists locally before proxying;
- render presets and call `/models?reload=1` when the downloaded model set changes.

Non-responsibilities:

- do not implement LRU or loaded-instance scheduling in v1;
- do not call `/models/load` or `/models/unload` as part of the normal request path;
- do not expose a public admin plane in v1;
- do not mount Docker socket or manage containers directly;
- do not persist model-manager state in a database initially.

## 3.3 Upstream llama.cpp router mode owns

- discovering local GGUF models from `--models-dir`;
- reading model presets from `--models-preset`;
- starting child `llama-server` instances;
- unloading child instances;
- `--models-max` capacity limit;
- LRU unload when capacity is reached;
- `--models-autoload` on first request;
- `--sleep-idle-seconds` idle memory release;
- proxying requests to model child instances;
- llama.cpp-native OpenAI-compatible endpoints;
- streaming response behavior;
- request cancellation when the client disconnects.

Non-responsibilities:

- do not define the public product API by itself;
- do not own the Hugging Face catalog;
- do not download missing models from the internet;
- do not decide which catalog entries are allowed for public use.

## 3.4 llama.cpp-stack tooling owns

- curated `models/catalog.toml`;
- Docker Compose profiles for Vulkan/CUDA/CPU;
- vendored OpenAI OpenAPI snapshot checks plus public gateway behavior probes;
- documentation and operational probes.

## 3.5 Logging roles and event keywords

Logging policy:

- application logs go only to stdout/stderr;
- production format should be JSON (`LOG_FORMAT=json`);
- local development may use text (`LOG_FORMAT=text`);
- the stack does not write log files, rotate log files, or own log retention;
- Docker, journald, Loki, Vector, Fluent Bit, or another collector should own
  ingestion and storage.

Use stable event keywords in the `msg` field and add structured attributes for
filtering. Suggested attributes across roles:

```text
request_id
method
path
status
duration_ms
model
endpoint
stream
error
```

Runtime log keywords are grouped into three roles:

```text
gateway.*  client-facing HTTP/API boundary
model.*    model policy, catalog, lazy download, preset, scheduling
backend.*  calls and proxy traffic to the internal inference backend
```

Gateway keywords:

```text
gateway.start
gateway.shutdown
gateway.request
gateway.response
gateway.error
gateway.health
gateway.openapi
```

Model keywords:

```text
model.catalog_load
model.catalog_reload
model.ensure
model.download
model.preset
model.hit
model.miss
model.load
model.unload
model.capacity_full
model.reject
model.lru_select
model.idle_select
```

Backend keywords:

```text
backend.health
backend.models
backend.reload
backend.load
backend.unload
backend.forward
backend.response
backend.stream
backend.cancel
backend.error
```

`backend.*` means the gateway is talking to the internal inference backend. The
current backend is llama.cpp router mode, but the keyword intentionally leaves
room for future backends.

Probe scripts may print their own `probe.*` messages in CI or operator output,
but `probe.*` is not a runtime service role.

## 4. Model identity and local paths

The source catalog remains simple and Hugging Face friendly:

```toml
[[models]]
repo = "Qwen/Qwen3-4B-GGUF"
quant = "Q4_K_M"
kind = "chat"

[[models]]
repo = "n24q02m/Qwen3-Embedding-0.6B-GGUF"
quant = "Q4_K_M"
kind = "embedding"
```

Derived identity:

```text
model_ref = <repo>/<quant>
example   = Qwen/Qwen3-4B-GGUF/Q4_K_M
```

Stable local path:

```text
/models/hf/<repo>/<quant>.gguf
example: /models/hf/Qwen/Qwen3-4B-GGUF/Q4_K_M.gguf
```

This path layout intentionally allows `/` in model ids. The id naturally maps to a directory path and avoids lossy escaping.

## 5. Runtime preset generation

`llama-server` router mode supports `--models-preset` INI files. We should generate this file from catalog + runtime defaults instead of manually maintaining per-model command lines.

Example generated file:

```ini
version = 1

[*]
ctx-size = 8192
parallel = -1
threads-http = -1
n-gpu-layers = 999
jinja = true

[Qwen/Qwen3-4B-GGUF/Q4_K_M]
model = /models/hf/Qwen/Qwen3-4B-GGUF/Q4_K_M.gguf
alias = Qwen/Qwen3-4B-GGUF/Q4_K_M

[n24q02m/Qwen3-Embedding-0.6B-GGUF/Q4_K_M]
model = /models/hf/n24q02m/Qwen3-Embedding-0.6B-GGUF/Q4_K_M.gguf
alias = n24q02m/Qwen3-Embedding-0.6B-GGUF/Q4_K_M
embeddings = true
pooling = mean
```

Open questions to verify per upstream behavior:

- exact preset key for embedding enablement;
- whether model aliases may contain `/` in every target client;
- whether `kind = "embedding"` needs a separate router process if runtime flags conflict;
- how `load-on-startup`, `stop-timeout`, and `sleep-idle-seconds` should be exposed in our config.

## 6. Download and reload flow

Router mode discovers models from cache, `--models-dir`, or `--models-preset`. It does not replace our need for a curated downloader.

Default lazy flow with gateway:

```text
request model=X
  |
  v
gateway validates X exists in catalog
  |
  +-- if stable file missing:
  |      download GGUF from Hugging Face
  |      publish /models/hf/<repo>/<quant>.gguf atomically
  |      regenerate models-preset.ini if needed
  |      call llama router GET /models?reload=1
  |
  v
proxy original request to llama router
  |
  v
llama router autoloads X and proxies to child instance
```

Optional deploy-time prefetch flow:

```text
operator runs prefetch command for selected models
gateway/tool generates models-preset.ini
llama-server --models-preset /models/models-preset.ini --models-max N ...
```

Deploy-time prefetch is only an optimization. Request-time lazy download should use the same catalog/downloader/preset code path.

Important note: if a new model file appears after router startup, the gateway should call `GET /models?reload=1` before forwarding the first request for that model.

## 7. Public API surface

The gateway should initially expose only:

```text
GET  /health
GET  /v1/models
POST /v1/chat/completions
POST /v1/completions
POST /v1/responses
POST /v1/embeddings
```

The gateway should deliberately hide experimental/router management routes from normal public clients:

```text
/models/load
/models/unload
/slots
/metrics
/props
```

Those can remain available only on the internal Compose network or localhost for operator probes. If a future admin API is needed, it should be explicit, authenticated, and disabled by default.

## 8. Capacity and unload policy

Use upstream llama.cpp router controls first:

```text
--models-max N             maximum loaded model instances
--models-autoload          load requested model automatically
--no-models-autoload       require explicit load
--sleep-idle-seconds N     unload idle model memory after inactivity
POST /models/unload        explicit unload
```

Default recommendation follows llama.cpp defaults unless explicitly overridden:

```text
models-max = 4             # llama.cpp default, 0 = unlimited
models-autoload = enabled  # llama.cpp default
parallel = -1              # llama.cpp automatic slot selection
sleep-idle-seconds = 0     # disabled in this stack until tuned
```

Default v1 policy accepts upstream router behavior: when a cold model is
requested and the loaded model count has reached `--models-max`, llama.cpp router
mode performs LRU unload and then loads the requested model. The gateway should
not duplicate that behavior unless a concrete product requirement appears, such
as pinned models, priority, or VRAM-aware placement.

Slot count is not dynamically adjusted by the gateway. `LLAMA_ROUTER_PARALLEL`
controls the preset `parallel` value passed to child model instances; `-1` means
llama.cpp chooses automatically. Changing slot count for a loaded model requires
changing the preset and reloading that model instance, not a hot slot resize.

## 9. Cancellation and streaming

Cancellation remains a hard requirement.

Validated behavior in local spike:

```text
client starts long streaming request
client disconnects / timeout occurs
router proxy reports upstream cancellation
child llama-server logs cancel task
follow-up request succeeds
```

Required probe:

1. start a long streaming request through the deployed endpoint;
2. cancel after a short delay;
3. send a follow-up request to the same model;
4. optionally inspect internal `/slots` or logs when available;
5. assert the server is not still consuming generation resources for the abandoned request.

## 10. Gateway API behavior

The gateway is intentionally thin. It should not implement inference and should not manage child model processes. Its request path is:

```text
public request
  -> parse enough JSON to read model
  -> validate model exists in catalog
  -> ensure local GGUF exists, downloading if needed
  -> ensure generated preset includes the model
  -> call router /models?reload=1 when new files/presets appear
  -> proxy the original request to llama.cpp router mode
  -> stream response back to client
```

For endpoints without a model field, the gateway either serves them directly (`/health`, `/v1/models`) or rejects/hides them unless explicitly supported.

### 10.1 Public endpoints

```text
GET  /health
GET  /v1/models
POST /v1/chat/completions
POST /v1/completions
POST /v1/responses
POST /v1/embeddings
```

`/v1/models` should return catalog entries, enriched with local/router state when available:

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
        "router_status": "unloaded",
        "kind": "chat"
      }
    }
  ]
}
```

### 10.2 Request preservation

The gateway should preserve request bodies as much as possible. It may parse JSON to read `model` and optionally apply explicit policy, but it should not drop unknown llama.cpp/OpenAI extension fields.

### 10.3 Error shaping

Gateway-originated errors should use OpenAI-shaped errors:

```json
{
  "error": {
    "message": "requested model is not in catalog",
    "type": "invalid_request_error",
    "code": "model_not_found"
  }
}
```

Initial gateway error codes:

```text
invalid_json
missing_model
model_not_found
model_capability_mismatch
download_failed
router_reload_failed
router_unavailable
```

Upstream llama.cpp errors may be passed through unless they need normalization for a known integration.

## 11. Framework choice for our code

Even though llama.cpp owns model lifecycle, the gateway still has real API responsibilities: model allowlisting, lazy download, router reload, endpoint hiding, and contract documentation. This is enough structure to justify a small framework boundary.

Selected stack:

```text
HTTP router           chi
API boundary/OpenAPI  Huma on the chi adapter
Core library code     plain Go packages
CLI / generators      Go flag package first; Cobra only if CLI grows
HTTP client/proxy     net/http, explicit context-aware streaming code
Schema source        vendored OpenAI OpenAPI snapshot + behavior probes
```

### 11.1 Why chi

`chi` is the right low-level router for this service:

- it is small and idiomatic Go;
- it keeps standard `net/http` request/response semantics;
- it does not hide `http.Request.Context()`, which we need for client disconnect cancellation;
- it composes naturally with middleware;
- it avoids the heavier framework context abstractions used by Gin/Fiber;
- it is easy to bypass for exact streaming/proxy behavior when needed.

If we only needed routing, chi alone would be enough. The downside is that chi does not force typed API boundaries or generate OpenAPI. Over time, a pure-chi gateway can drift into manually parsed handlers with unclear request/response contracts.

### 11.2 Why Huma on top of chi

Huma is used for the public gateway boundary, not for model lifecycle.

Benefits:

- explicit operation registration for the public API surface;
- generated OpenAPI document from the registered gateway routes;
- a clearer place to document request/response/error shapes;
- generated OpenAPI and behavior probes can be compared against the vendored OpenAI snapshot in CI;
- Huma can run on top of chi, so we still keep standard `net/http` semantics.

We should not use Huma as a deep application framework. It should define the boundary; plain Go packages should implement the behavior.

### 11.3 Why not Gin/Fiber/Echo

- Gin is popular but introduces its own context style and does not solve our OpenAPI contract problem without annotation tooling.
- Fiber is fast but uses fasthttp semantics, which is a poor fit for standard Go reverse proxy/cancellation behavior.
- Echo is usable, but does not provide a better fit than chi for this gateway.

For our constraints, `chi + Huma` is a better balance than a larger web framework.

### 11.4 Practical boundary rule

```text
Huma defines and documents the public gateway surface.
chi provides the actual HTTP routing substrate.
Gateway-internal catalog/download/preset/router clients stay framework-independent.
Streaming proxy remains explicit and context-aware via net/http.
```

Do not introduce Huma or chi dependencies into non-HTTP packages. Catalog parsing, downloader, preset generation, and router clients should remain plain packages under `gateway/internal` so they can be reused by gateway tests and future gateway-local tools.

## 12. Implementation phases

### Phase 1: Router-mode compose behind gateway

- add a `llama-router` service using `llama-server` without `--model`;
- add a `gateway` service as the only public HTTP service;
- mount `/models` read-write into gateway and read-only or read-write as needed into router;
- pass `--models-preset /models/models-preset.generated.ini` to router;
- preserve Vulkan/CUDA profiles on router;
- gateway proxies allowed `/v1/*` requests to router;
- add smoke probes for `/health`, `/v1/models`, `/v1/chat/completions`, `/v1/responses`, streaming cancellation.

### Phase 2: Catalog to preset generator

- read `models/catalog.toml`;
- derive `model_ref` and stable local paths;
- emit `models/models-preset.generated.ini`;
- fail clearly if required files are missing when running in deploy-time mode.

### Phase 3: Downloader integration

Status: implemented in the gateway request path.

- implement catalog-aware Hugging Face download using the existing downloader logic;
- publish stable paths atomically;
- render `models-preset.generated.ini` after downloads;
- call `/models?reload=1` after new files appear;
- add per-model locking so concurrent requests do not download the same model twice.

### Phase 4: Huma/chi gateway boundary

Status: implemented as the default gateway boundary.

- route registration uses Huma on the chi adapter;
- gateway exposes OpenAI-compatible endpoints only;
- gateway validates the catalog allowlist;
- gateway triggers lazy download/reload;
- gateway proxies to llama router with cancellation propagation;
- gateway hides router management endpoints from public clients;
- gateway exposes generated OpenAPI at `/openapi.json`;
- CI should compare the generated OpenAPI surface against fixed contracts.

### Phase 5: Optional platform layer

Only after a concrete product requirement appears:

- pinned or priority models;
- VRAM-aware placement;
- auth;
- metrics;
- task-aware routing;
- edge node routing;
- Pipecat-specific defaults.

## 12. Risks and mitigations

| Risk | Mitigation |
|---|---|
| router mode is experimental | keep smoke/cancellation/schema probes; pin image/tag; keep the gateway/backend boundary small |
| upstream API changes | treat OpenAI snapshot checks and behavior probes as contract tests; review llama.cpp release notes before bumping |
| model id/path mismatch | derive ids and paths from catalog in one package; avoid duplicate naming logic |
| public exposure of management endpoints | gateway is default public entrypoint; do not publish router service directly |
| lazy download race | per-model file lock and atomic rename |
| LRU behavior surprises users | document `--models-max`; add gateway policy later only if pinning, priority, or VRAM-aware placement is needed |

## 13. Current recommendation

Move implementation toward:

```text
Docker Compose + Huma-based thin gateway
+ llama-server router mode
+ catalog/preset/download tooling
```

Keep the gateway as a thin policy/download/proxy layer, not as a second model process manager.
