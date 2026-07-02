# llama.cpp Vulkan Inference Stack

本仓库用于部署一个轻量、可控、可调试的本地 LLM 推理服务：基于 `llama.cpp` / `llama-server` router mode、GGUF 模型、Docker Compose、Go gateway，以及 AMD GPU 的 Vulkan 后端。

## 目标

- 提供 OpenAI-compatible API，便于接入 agent orchestration 和后续工具生态。
- 支持流式输出和客户端取消，避免断开的请求继续占用 GPU。
- 使用 Docker Compose 部署，依赖隔离、可迁移、易重启。
- 支持 AMD GPU Vulkan 加速和 GGUF 模型。
- 支持 catalog 模型按需下载、按需加载。

## Quick start

Requirements: Linux, Docker with Compose plugin, Bash, `curl`.

```bash
cp .env.example .env
make up        # starts the Go gateway + llama.cpp router mode
make logs      # follow gateway/router logs
make down      # stop the stack
```

`make up` deploys the Go gateway. The gateway listens on container port `8090`; the host bind is controlled by `LLAMA_HOST` / `LLAMA_PORT`, or by optional `GATEWAY_HOST` / `GATEWAY_PORT` overrides.

## Manual reachability test

健康检查：

```bash
curl http://127.0.0.1:8080/health
curl http://127.0.0.1:8080/v1/models
```

非流式 OpenAI chat completion：

```bash
curl http://127.0.0.1:8080/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -d '{
    "model":"Open4bits/Qwen3-0.6b-gguf/Q4_K_M",
    "messages":[{"role":"user","content":"Say hello in one sentence."}],
    "max_tokens":64,
    "stream":false
  }'
```

流式检查（会按需下载 `GATEWAY_SMOKE_MODEL`）：

```bash
make smoke
```

取消检查（会按需下载 `GATEWAY_SMOKE_MODEL`）：

```bash
make stream-cancel
```

## Architecture

```text
Client / Apps / Agents
        |
        v
model-permissions init container
  - prepares /models ownership for the non-root gateway
        |
        v
Go Gateway container
  - OpenAI-compatible public API
  - catalog parsing / allowlist
  - lazy Hugging Face downloader
  - router preset generation
  - typed OpenAI compatibility adapters where needed
  - streaming/cancellation-aware proxy
        |
        v
llama-server router mode container
  - dynamic model load/unload
  - child llama-server instances
  - GGUF/Vulkan inference
```

Public gateway endpoints:

```text
GET  /health
GET  /v1/models
POST /v1/chat/completions
POST /v1/completions
POST /v1/responses
POST /v1/embeddings
```

The gateway does not expose backend URLs or llama.cpp router management endpoints such as `/models/load`, `/models/unload`, `/slots`, `/props`, or `/metrics` publicly.

## Runtime responsibility boundaries

```text
Gateway
  owns: public API, OpenAPI, catalog allowlist, lazy download, startup preset generation,
        capability checks, cancellation-aware proxy, public error shape
  does not own: inference execution, child llama-server processes, public slots

Gateway model manager
  owns: in-process catalog/download/preset state and same-model locking
  does not own: scheduling, LRU, model load/unload, Docker container lifecycle,
                or a public admin API

llama.cpp router mode
  owns: child llama-server lifecycle, model load/unload, models-max capacity,
        LRU/idle behavior, llama.cpp-native streaming and cancellation
  does not own: Hugging Face download or public catalog allowlisting

Tooling
  owns: catalog files, Docker Compose profiles, probes, schema checks, docs
```

## Docker deployment notes

The Compose stack uses service names instead of fixed `container_name` values so multiple checkouts can coexist when `COMPOSE_PROJECT_NAME` differs. The gateway image build copies only `go.mod`, `go.sum`, and `gateway/` into the build stage; vendored schema/upstream snapshots and GGUF files stay outside the image build context.

Runtime hardening defaults:

- `model-permissions` is a one-shot root init service that prepares `/models` ownership before runtime services start;
- gateway runs as non-root `${APP_UID}:${APP_GID}` and has a read-only root filesystem; `/models` and `/tmp` remain writable;
- llama-router intentionally remains root for simpler Vulkan `/dev/dri` access on Fedora Atomic/Bazzite-style hosts;
- gateway and router use `no-new-privileges`;
- `HF_TOKEN_FILE` can point at a mounted secret file and takes precedence over `HF_TOKEN`.

The default application identity is:

```env
APP_UID=10001
APP_GID=10001
```

Change it only if those IDs conflict with host ownership policy.

## Logging

The stack logs to stdout/stderr only. It does not write or rotate log files.
Production should use structured JSON logs:

```env
LOG_FORMAT=json
LOG_LEVEL=info
```

Local debugging may use:

```env
LOG_FORMAT=text
```

Stable log/event keywords are grouped by runtime role:

```text
gateway.*  client-facing HTTP/API lifecycle
model.*    model policy, catalog, lazy download, preset, scheduling
backend.*  calls and proxy traffic to the internal inference backend
```

Probe scripts may print `probe.*` messages in CI/operator output, but `probe.*`
is not a runtime service role. See `docs/llama-router-mode-design.md` for the
detailed keyword list.

## Model catalog

`configs/models.catalog.toml` lists Hugging Face GGUF sources. It is source-only and does not contain ports.

```toml
[[models]]
repo = "Qwen/Qwen3-4B-GGUF"
quant = "Q4_K_M"

# Optional for vision-language models in llama.cpp/GGUF repos.
# The gateway downloads this alongside the main GGUF and writes it to the router preset.
mmproj = "mmproj-F16.gguf"
```

Model refs are derived as `<repo>/<quant>`, for example:

```text
Qwen/Qwen3-4B-GGUF/Q4_K_M
```

The catalog also includes MoE test targets. `allenai/OLMoE-1B-7B-0125-GGUF/Q3_K_M` is the recommended first MoE model for the current 8GB UMA host. Larger Qwen A3B MoE entries are included for a future 64GB UMA host and are lazy-loaded only when requested.

Requesting a catalog model lazily downloads the selected GGUF, plus optional `mmproj` projector files for multimodal-capable models, into:

```text
models/hf/<repo>/<quant>.gguf
```

The stack generates a full-catalog router preset before `llama-router` starts. At request time the gateway downloads the GGUF if missing, verifies the router registry contains the model, and proxies the request. Router mode owns actual model load/unload.

## Capacity semantics

Runtime model residency is delegated to llama.cpp router mode:

- `LLAMA_MODELS_MAX` controls the maximum number of loaded model instances; default `4`, matching llama.cpp;
- router mode autoloads requested models when enabled;
- when capacity is reached, router mode may unload the least-recently-used model;
- same-model concurrency is handled by llama.cpp slots/queue inside the child instance; `LLAMA_ROUTER_PARALLEL=-1` keeps llama.cpp automatic slot selection;
- custom gateway scheduling such as pinning, priority, or VRAM-aware placement can be added later only when needed.

## Repository layout

```text
.
├── gateway/
│   ├── Dockerfile                    # Go gateway container image
│   ├── cmd/gateway/                  # gateway HTTP entrypoint
│   └── internal/                     # gateway-only Go packages
│       ├── catalog/
│       ├── config/
│       ├── hf/
│       ├── openai/
│       ├── preset/
│       ├── proxy/
│       ├── routerclient/
│       └── routermanager/
├── openai-openapi/                   # vendored OpenAI OpenAPI snapshot
├── llamacpp-upstream/                # vendored llama.cpp upstream snapshot
├── openai-api-schema.yaml            # generated gateway OpenAI subset schema
├── llamacpp-api-schema/              # pinned llama.cpp upstream schema subset
├── docker-compose.dynamic.yml        # init + Go gateway + llama-server router mode
├── docker-compose.dynamic.vulkan.yml # Vulkan override for router
├── docker-compose.dynamic.cuda.yml   # CUDA override for router
├── Makefile                          # validated compose workflow
├── docs/
├── models/
└── scripts/
```

## Key configuration

编辑 `.env`：

- `LLAMA_BACKEND`：`cpu` / `vulkan` / `cuda`，默认 `vulkan`。
- `LLAMA_HOST` / `LLAMA_PORT`：gateway 宿主机监听地址和端口。
- `GATEWAY_HOST` / `GATEWAY_PORT`：可选覆盖；留空则继承 `LLAMA_HOST` / `LLAMA_PORT`。
- `LLAMA_ROUTER_URL`：gateway 访问内部 llama.cpp router mode 的地址。
- `LLAMA_MODELS_MAX`：router mode 最多同时加载的模型实例数；默认 `4`，跟随 llama.cpp。
- `LLAMA_MODELS_AUTOLOAD`：是否允许 router mode 按请求自动加载模型。
- `LLAMA_SLEEP_IDLE_SECONDS`：空闲模型自动释放时间；`0` 表示关闭。
- `LLAMA_ROUTER_CTX_SIZE` / `LLAMA_ROUTER_PARALLEL`：生成 router preset 时的上下文和 slot 默认值；`LLAMA_ROUTER_PARALLEL=-1` 表示使用 llama.cpp 自动选择。
- `GATEWAY_PROXY_RESPONSE_HEADER_TIMEOUT_SECONDS`：gateway 等待 llama-router 返回响应头的时间；长 prefill 或长 tool-call 非 streaming 请求建议调高，默认 300 秒。
- `LLAMA_ROUTER_N_GPU_LAYERS`：GPU offload 层数；`999` 表示尽量全部 offload。
- `HF_ENDPOINT` / `HF_TOKEN` / `HF_TOKEN_FILE`：Hugging Face 下载配置；`HF_TOKEN_FILE` 指向挂载的 secret 文件时优先于 `HF_TOKEN`。
- `GATEWAY_SMOKE_MODEL`：`make smoke` / `make probe-api` 使用的模型。
- `APP_UID` / `APP_GID`：gateway 非 root 运行身份和 `/models` 所有权；默认 `10001:10001`。
- catalog `kind`：默认 `chat`；embedding 模型设置 `kind = "embedding"`，gateway 会在 router preset 中标记 embeddings，并只允许该模型走 `/v1/embeddings`。

## Gateway framework

The gateway uses `chi` as the low-level `net/http` router and Huma on top of the chi adapter for API boundary/OpenAPI generation. Core catalog, downloader, preset, router-client, and streaming proxy logic stay in plain gateway-internal Go packages so they can be reused by CLI/probes/tests and keep cancellation behavior explicit.

## Commands

```bash
make up
make down
make restart
make logs
make ps
make config
make models
make probe-gateway
make probe-errors
make smoke
make stream-cancel
```

Backend selection:

```bash
make up BACKEND=vulkan
make up BACKEND=cpu
make up BACKEND=cuda
```

## API schemas

The vendored OpenAI OpenAPI snapshot lives under `openai-openapi/` and is documented in `docs/api-schemas.md`. Router-mode architecture is documented in `docs/llama-router-mode-design.md`.

```bash
make schemas
make check-openai-openapi
make probe-api BASE_URL=https://llamacpp-stack.vex9z7.com
```

## Vulkan/AMD 排障

```bash
ls -l /dev/dri
getent group render video
# 如果宿主机安装了 vulkan-tools：
vulkaninfo --summary
```

如果容器无法看到 GPU：

1. 确认宿主机存在 `/dev/dri/renderD*`。
2. 确认 Docker 容器拥有 render node 访问权限。
3. 查看 `make logs` 中是否出现 Vulkan device / layer offload 信息。
4. 必要时 pin 一个已验证的 llama.cpp Vulkan 镜像 tag。

## 暴露范围与安全

默认只绑定 localhost。若需要在局域网或 VPN 内访问，设置：

```env
LLAMA_HOST=0.0.0.0
```

公网暴露前应增加认证代理、访问控制和日志策略。
