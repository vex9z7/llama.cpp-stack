# llama.cpp Vulkan Inference Stack

本仓库用于部署一个轻量、可控、可调试的本地 LLM 推理服务：基于 [`llama.cpp`](https://github.com/ggml-org/llama.cpp) / `llama-server`、GGUF 模型、Docker Compose，以及 AMD GPU 的 Vulkan 后端。

## 目标

- 提供 OpenAI-compatible API，便于接入 Pipecat、agent orchestration 和后续工具生态。
- 支持流式输出和客户端取消，避免断开的请求继续占用 GPU。
- 使用 Docker Compose 部署，依赖隔离、可迁移、易重启。
- 支持 AMD GPU Vulkan 加速和 GGUF 模型。
- 支持多并发请求；当前优先级是稳定性、隔离性和取消响应，而不是极致吞吐。

## Quick start

Requirements: Linux, Docker with Compose plugin, Bash, `curl`.

```bash
cp .env.example .env
make up        # starts the Go gateway + worker-agent pool; removes old single-instance orphans
make logs      # follow gateway/worker logs
make down      # stop the dynamic stack
```

`make up` now deploys the dynamic gateway on host port `8080` by default, so an existing reverse proxy that used to point at the old `llama-server` can keep the same upstream port after the old container is removed. The old single-instance deployment is still available as `make legacy-up`.

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



## Dynamic gateway mode

The Go dynamic gateway/worker mode is the first implementation of the router design in `docs/dynamic-model-manager-design.md`. It exposes one OpenAI-compatible gateway and keeps worker/slot details internal.

```bash
cp .env.example .env
make up BACKEND=vulkan
make logs
make probe-gateway BASE_URL=http://127.0.0.1:8080
```

Gateway endpoints:

```text
GET  /health
GET  /v1/models
POST /v1/chat/completions
POST /v1/completions
POST /v1/responses
POST /v1/embeddings
```

`/v1/models` returns catalog models from `models/catalog.toml`. Requesting one of those model ids lazily downloads the GGUF into `models/hf/<repo>/<quant>.gguf`, loads it into an idle worker-agent container, and proxies the request to that worker's `llama-server`.

Example dynamic request:

```bash
curl http://127.0.0.1:8080/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -d '{
    "model":"Qwen/Qwen3-4B-GGUF/Q4_K_M",
    "messages":[{"role":"user","content":"Say hello in one sentence."}],
    "max_tokens":64,
    "stream":true,
    "chat_template_kwargs":{"enable_thinking":false}
  }'
```

Capacity semantics for v1:

- one worker-agent container can load one model;
- requests for an already-loaded model are forwarded to that same worker;
- same-model concurrency is handled by that worker's `llama-server` slots/queue;
- if a cold model is requested and all workers already hold other models, gateway returns HTTP 429;
- no Docker socket is mounted, and no `/control/*`, `/worker/*`, or `/slots` endpoints are exposed publicly.

Dynamic mode uses these files:

```text
Dockerfile.gateway
Dockerfile.worker
docker-compose.dynamic.yml
docker-compose.dynamic.vulkan.yml
docker-compose.dynamic.cuda.yml
cmd/gateway
cmd/worker
internal/
```

## Model catalog

`models/catalog.toml` lists Hugging Face models that the future manager may lazy-download on demand. It is source-only and does not contain runtime parameters.

```toml
[[models]]
repo = "Qwen/Qwen3-4B-GGUF"
quant = "Q4_K_M"
```

Model refs are derived as `<repo>/<quant>`, for example:

```text
Qwen/Qwen3-4B-GGUF/Q4_K_M
```

The default deployment uses the Go gateway and lazy-downloads catalog models on demand. The legacy single-instance Compose path still exists behind `make legacy-up` and expects an already-local GGUF file via `LLAMA_MODEL_FILE`.

## Models

模型文件放在 `./models` 下。动态 gateway 会按需下载到 `models/hf/<repo>/<quant>.gguf`。旧单实例模式会加载：

```text
models/model.gguf
```

如需使用其它模型文件：

```env
LLAMA_MODEL_FILE=hf/Qwen/Qwen3-8B-GGUF/Q4_K_M.gguf
LLAMA_ALIAS=qwen3-8b-local
```

`models/**/*.gguf` 不入 git，只有 `models/.gitkeep` 用于保留目录。

## 目录结构

```text
.
├── docker-compose.dynamic.yml        # default Go gateway + worker-agent stack
├── docker-compose.dynamic.vulkan.yml # dynamic Vulkan override
├── docker-compose.dynamic.cuda.yml   # dynamic CUDA override
├── docker-compose.yml                # legacy llama-server base service
├── docker-compose.vulkan.yml         # legacy Vulkan override
├── docker-compose.cuda.yml           # legacy CUDA override
├── Makefile                  # validated compose workflow
├── .env.example              # 可复制的本地配置模板
├── docs/
│   └── deployment-plan.md    # 部署方案和工程说明
├── models/                   # 放置本地 GGUF 模型；模型文件不入 git
└── scripts/
    ├── smoke_stream.sh       # 流式输出 smoke test
    └── test_cancel.sh        # 客户端取消验证脚本
```

## 关键配置

编辑 `.env`：

动态 gateway：

- `GATEWAY_HOST` / `GATEWAY_PORT`：gateway 宿主机监听地址和端口；默认 `127.0.0.1:8080`。
- `WORKER_BASE_URLS`：Compose 内部 worker-agent 地址列表。
- `LLAMA_WORKER_POOL_SIZE`：文档/校验用的 worker 数量；实际数量由 Compose worker 服务决定。
- `LLAMA_WORKER_CTX_SIZE` / `LLAMA_WORKER_PARALLEL`：每个 worker 启动 llama-server 时的上下文和并发。
- `HF_ENDPOINT` / `HF_TOKEN`：Hugging Face 下载配置。

通用/legacy：

- `LLAMA_BACKEND`：`cpu` / `vulkan` / `cuda`，默认 `vulkan`。
- `LLAMA_HOST` / `LLAMA_PORT`：宿主机监听地址和端口；默认只绑定 `127.0.0.1:8080`。
- `LLAMA_MODEL_FILE`：`./models` 下的 GGUF 路径，例如 `model.gguf` 或 `hf/Qwen/Qwen3-4B-GGUF/Q4_K_M.gguf`。
- `LLAMA_ALIAS`：客户端请求时使用的 model 名称。
- `LLAMA_CTX_SIZE` / `LLAMA_N_PARALLEL`：上下文总量与并发 slot 数。有效单请求上下文大约是 `CTX_SIZE / N_PARALLEL`。
- `LLAMA_N_GPU_LAYERS`：GPU offload 层数；`999` 表示尽量全部 offload，VRAM 不足时调低。
- `LLAMA_EXTRA_ARGS`：额外传给 `llama-server` 的参数，例如某些版本支持的 metrics/slots/API-key 参数。

## API schemas

Integration schemas live under `schemas/` and are documented in `docs/api-schemas.md`. They cover the OpenAI-compatible `/v1/*` subset plus llama.cpp-native endpoints such as `/health` and `/slots`.

```bash
make schemas
make probe-gateway BASE_URL=https://llamacpp-stack.vex9z7.com
```

## Multi-instance

For future dynamic multi-model serving, see `docs/dynamic-model-manager-design.md`. The older static generated-compose workflow is documented in `docs/multi-instance.md`. The workflow uses `configs/instances.toml` to generate `docker-compose.instances.yml`.

```bash
cp configs/instances.example.toml configs/instances.toml
make instances-render
make instances-up
```

## Backend selection

默认 `make up` 使用 dynamic gateway。默认使用 Vulkan：

```bash
make up
```

CPU debug：

```bash
make up BACKEND=cpu
```

CUDA portability option：

```bash
make up BACKEND=cuda
```

旧单实例模式：

```bash
make legacy-up BACKEND=vulkan
make legacy-logs
make legacy-down
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
# 如果你的 llama-server 版本支持 API key，可通过 LLAMA_EXTRA_ARGS 传入对应参数。
```

公网暴露前应增加认证代理、访问控制和日志策略。
