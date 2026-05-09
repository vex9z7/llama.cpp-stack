# Local LLM Inference Service Deployment Plan

## 1. 背景

本项目部署一个轻量、私有、自托管的 LLM 推理服务，基于 `llama.cpp` 和 GGUF 模型。目标环境是 Linux + AMD GPU + Vulkan + Docker，面向未来 Pipecat、语音流式 pipeline、agent 编排和边缘推理节点实验。

该系统不是 GUI 桌面应用，也不是黑盒式一键 AI 产品。当前阶段更重视：

- 可控性
- 可调试性
- API 兼容性
- 请求生命周期管理
- 后端灵活性

## 2. 当前阶段目标

### 2.1 OpenAI-compatible API

推理服务应暴露接近 OpenAI chat/completions 的 API，以便：

- 接入 Pipecat
- 接入后续 orchestration layer
- 降低 backend 替换成本
- 复用现有工具生态

### 2.2 请求取消与中断

后端必须正确处理：

- client disconnect
- streaming cancellation
- request interruption

语音 UI 和低延迟对话场景下，用户打断或客户端断开后，废弃请求应尽快释放 GPU/slot 资源。

### 2.3 Dockerized deployment

部署使用 Docker Compose，保持：

- 依赖隔离
- 环境可复现
- 易迁移
- 易重启
- 不引入 Kubernetes 复杂度

### 2.4 Vulkan backend

AMD GPU 通过 Vulkan 后端加速。模型格式使用 GGUF，以获得较好的本地生态、量化灵活性和边缘设备友好性。

### 2.5 并发请求

`llama-server` 通过 slot / parallel 配置支持多并发请求。当前不追求最高吞吐 batching，而是优先：

- 可预测延迟
- request isolation
- cancellation responsiveness
- 稳定性

## 3. 技术选择

### Runtime

选择：`llama.cpp` / `llama-server`

原因：

- 轻量，依赖少
- 支持 GGUF
- 支持 Vulkan
- 提供 OpenAI-compatible endpoint
- 比高层 wrapper 更直接地暴露请求生命周期与运行时行为

### Model format

选择：GGUF

原因：

- llama.cpp 原生生态
- 量化选择丰富
- 模型文件易迁移
- 适合本地和边缘部署

## 4. 架构

```text
Client / Pipecat / Agents
        |
        v
Future Orchestration Layer
        |
        v
llama-server OpenAI-compatible API
        |
        v
llama.cpp Vulkan Backend
        |
        v
GGUF Models
```

当前仓库只实现 `llama-server` 层和 Docker 部署基础。orchestration、routing、metrics stack、auth proxy 后续再加。

## 5. 部署设计

### Container responsibilities

单容器负责：

- 加载 GGUF 模型
- 暴露 HTTP API
- 管理 slot / parallel 请求
- 使用 Vulkan 后端执行推理

### Persistent storage

- `./models:/models:ro`：宿主机放置 GGUF 模型，容器只读挂载。
- `hf-cache` named volume：预留给后续 HF 拉取/缓存场景。

### GPU access

Compose 挂载：

```yaml
devices:
  - /dev/dri:/dev/dri
```

AMD Vulkan 主要依赖 render node，例如 `/dev/dri/renderD128`。

## 6. 运行配置

关键参数：

- `LLAMA_ARG_MODEL`：模型路径
- `LLAMA_ARG_ALIAS`：OpenAI API 中暴露的模型名
- `LLAMA_ARG_CTX_SIZE`：总上下文窗口
- `LLAMA_ARG_N_PARALLEL`：并发 slot 数
- `LLAMA_ARG_N_GPU_LAYERS`：GPU offload 层数
- `LLAMA_ARG_CONT_BATCHING`：连续 batching，适合多客户端
- `LLAMA_ARG_CACHE_PROMPT`：prompt cache，提高重复上下文效率
- `LLAMA_ARG_ENDPOINT_METRICS`：metrics endpoint
- `LLAMA_ARG_ENDPOINT_SLOTS`：slot inspection endpoint

注意：`CTX_SIZE` 通常会在并发 slot 之间分配；例如 `8192 / 2` 时，每个并发请求可用上下文大约为 4096 tokens。

## 7. 验证计划

### 7.1 基础启动

```bash
docker compose up -d
docker compose logs -f llama-server
curl http://127.0.0.1:8080/health
curl http://127.0.0.1:8080/v1/models
```

### 7.2 Chat completion

验证 `/v1/chat/completions` 非流式请求。

### 7.3 Streaming

运行：

```bash
./scripts/smoke_stream.sh
```

预期：客户端能持续收到 SSE token/chunk。

### 7.4 Cancellation

运行：

```bash
./scripts/test_cancel.sh
```

预期：脚本主动中断 streaming client 后，服务端 slot 应释放；可通过 `/slots` 和 logs 观察。

### 7.5 并发

启动多个 streaming/non-streaming 请求，观察：

- 是否按 `LLAMA_N_PARALLEL` 接收并发
- 超出并发时是否排队或等待
- cancellation 后 slot 是否恢复
- GPU/VRAM 是否稳定

## 8. 运维与迁移

迁移到新机器需要：

1. 安装 Docker / Compose
2. 确认 AMD Vulkan 驱动与 `/dev/dri`
3. 复制仓库
4. 复制 `models/*.gguf`
5. 复制或重建 `.env`
6. `docker compose up -d`

推荐在验证稳定后 pin 镜像 tag，避免上游 latest/tag 变化导致不可复现。

## 9. 当前非目标

当前阶段不实现：

- Kubernetes
- autoscaling
- distributed inference
- enterprise auth
- multi-node scheduling
- advanced batching optimization
- GUI frontend

## 10. 后续演进

后续可以逐步增加：

- Pipecat streaming integration test
- 轻量 orchestration layer
- 多模型 routing
- request queue / timeout policy
- Prometheus metrics + Grafana dashboard
- auth reverse proxy
- edge node profile presets
