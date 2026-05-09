# llama.cpp Vulkan Inference Stack

本仓库用于部署一个轻量、可控、可调试的本地 LLM 推理服务：基于 `llama.cpp` / `llama-server`、GGUF 模型、Docker Compose，以及 AMD GPU 的 Vulkan 后端。

## 目标

- 提供 OpenAI-compatible API，便于接入 Pipecat、agent orchestration 和后续工具生态。
- 支持流式输出和客户端取消，避免断开的请求继续占用 GPU。
- 使用 Docker Compose 部署，依赖隔离、可迁移、易重启。
- 支持 AMD GPU Vulkan 加速和 GGUF 模型。
- 支持多并发请求；当前优先级是稳定性、隔离性和取消响应，而不是极致吞吐。

## 快速开始

```bash
cp .env.example .env
mkdir -p models
# 将你的 GGUF 模型放到 ./models/model.gguf，或修改 .env 里的 LLAMA_MODEL。
docker compose up -d
docker compose logs -f llama-server
```

健康检查：

```bash
curl http://127.0.0.1:8080/health
curl http://127.0.0.1:8080/v1/models
```

非流式 OpenAI chat completion：

```bash
curl http://127.0.0.1:8080/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer sk-no-key-required' \
  -d '{
    "model":"local-llm",
    "messages":[{"role":"user","content":"Say hello in one sentence."}],
    "max_tokens":64,
    "stream":false
  }'
```

流式检查：

```bash
./scripts/smoke_stream.sh
```

取消检查：

```bash
./scripts/test_cancel.sh
```

## 目录结构

```text
.
├── docker-compose.yml        # llama-server Vulkan 服务
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

- `LLAMA_IMAGE`：默认 `ghcr.io/ggml-org/llama.cpp:server-vulkan`。验证通过后建议 pin 到明确 tag。
- `LLAMA_MODEL`：容器内模型路径，通常是 `/models/<file>.gguf`。
- `LLAMA_ALIAS`：客户端请求时使用的 model 名称。
- `LLAMA_CTX_SIZE` / `LLAMA_N_PARALLEL`：上下文总量与并发 slot 数。有效单请求上下文大约是 `CTX_SIZE / N_PARALLEL`。
- `LLAMA_N_GPU_LAYERS`：GPU offload 层数；`999` 表示尽量全部 offload，VRAM 不足时调低。
- `HOST_BIND`：默认 `127.0.0.1`，避免误暴露到局域网。

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
3. 查看 `docker compose logs llama-server` 中是否出现 Vulkan device / layer offload 信息。
4. 必要时 pin 一个已验证的 llama.cpp Vulkan 镜像 tag。

## 暴露范围与安全

默认只绑定 localhost。若需要在局域网或 VPN 内访问，设置：

```env
HOST_BIND=0.0.0.0
LLAMA_API_KEY=<your-key>
```

公网暴露前应增加认证代理、访问控制和日志策略。
