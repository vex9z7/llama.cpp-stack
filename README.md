# llama.cpp Vulkan Inference Stack

本仓库用于部署一个轻量、可控、可调试的本地 LLM 推理服务：基于 `llama.cpp` / `llama-server`、GGUF 模型、Docker Compose、Go gateway/worker-agent，以及 AMD GPU 的 Vulkan 后端。

## 目标

- 提供 OpenAI-compatible API，便于接入 Pipecat、agent orchestration 和后续工具生态。
- 支持流式输出和客户端取消，避免断开的请求继续占用 GPU。
- 使用 Docker Compose 部署，依赖隔离、可迁移、易重启。
- 支持 AMD GPU Vulkan 加速和 GGUF 模型。
- 支持 catalog 模型按需下载、按需加载。

## Quick start

Requirements: Linux, Docker with Compose plugin, Bash, `curl`.

```bash
cp .env.example .env
make up        # starts the Go gateway + worker-agent pool
make logs      # follow gateway/worker logs
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
    "stream":false,
    "chat_template_kwargs":{"enable_thinking":false}
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
Client / Pipecat / Agents
        |
        v
Go Gateway container
  - OpenAI-compatible public API
  - catalog parsing
  - lazy Hugging Face downloader
  - worker allocation
  - streaming/cancellation-aware proxy
        |
        +--> worker-agent-0 container -> llama-server subprocess
        +--> worker-agent-1 container -> llama-server subprocess
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

The gateway does not expose `/control/*`, `/worker/*`, backend URLs, or llama.cpp `/slots` publicly.

## Model catalog

`models/catalog.toml` lists Hugging Face GGUF sources. It is source-only and does not contain ports or worker assignments.

```toml
[[models]]
repo = "Qwen/Qwen3-4B-GGUF"
quant = "Q4_K_M"
```

Model refs are derived as `<repo>/<quant>`, for example:

```text
Qwen/Qwen3-4B-GGUF/Q4_K_M
```

Requesting a catalog model lazily downloads the selected GGUF into:

```text
models/hf/<repo>/<quant>.gguf
```

Then the gateway loads that model into an idle worker-agent container and proxies the request to that worker's `llama-server`.

## Capacity semantics

V1 capacity policy is simple and explicit:

- one worker-agent container can load one model;
- requests for an already-loaded model are forwarded to that same worker;
- same-model concurrency is handled by that worker's `llama-server` slots/queue;
- if a cold model is requested and all workers already hold other models, gateway returns HTTP 429;
- no eviction/LRU policy in v1.

## Repository layout

```text
.
├── Dockerfile.gateway
├── Dockerfile.worker
├── docker-compose.dynamic.yml        # Go gateway + worker-agent stack
├── docker-compose.dynamic.vulkan.yml # Vulkan override
├── docker-compose.dynamic.cuda.yml   # CUDA override
├── Makefile                          # validated compose workflow
├── cmd/
│   ├── gateway/
│   └── worker/
├── internal/
│   ├── catalog/
│   ├── hf/
│   ├── manager/
│   ├── proxy/
│   ├── workerclient/
│   └── llamaprocess/
├── docs/
├── models/
└── scripts/
```

## Key configuration

编辑 `.env`：

- `LLAMA_BACKEND`：`cpu` / `vulkan` / `cuda`，默认 `vulkan`。
- `LLAMA_HOST` / `LLAMA_PORT`：gateway 宿主机监听地址和端口。
- `GATEWAY_HOST` / `GATEWAY_PORT`：可选覆盖；留空则继承 `LLAMA_HOST` / `LLAMA_PORT`。
- `WORKER_BASE_URLS`：Compose 内部 worker-agent 地址列表。
- `LLAMA_WORKER_POOL_SIZE`：文档/校验用的 worker 数量；实际数量由 Compose worker 服务决定。
- `LLAMA_WORKER_CTX_SIZE` / `LLAMA_WORKER_PARALLEL`：每个 worker 启动 llama-server 时的上下文和并发。
- `LLAMA_WORKER_N_GPU_LAYERS`：GPU offload 层数；`999` 表示尽量全部 offload。
- `HF_ENDPOINT` / `HF_TOKEN`：Hugging Face 下载配置。
- `GATEWAY_SMOKE_MODEL`：`make smoke` / `make probe-api` 使用的模型。

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

Integration schemas live under `schemas/` and are documented in `docs/api-schemas.md`.

```bash
make schemas
make probe-gateway BASE_URL=https://llamacpp-stack.vex9z7.com
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
