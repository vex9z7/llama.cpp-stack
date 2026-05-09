# Multi-instance deployment

The single-instance stack is still the default. For multiple loaded models, use generated Compose from `configs/instances.toml`.

Why generated Compose instead of many env vars:

- one config file can describe many models;
- each instance has explicit port, model file, alias, backend, context, and parallelism;
- the generated `docker-compose.instances.yml` can be inspected before startup;
- the model/router layer can later consume the same config.

## Setup

```bash
cp configs/instances.example.toml configs/instances.toml
$EDITOR configs/instances.toml
```

Each enabled instance becomes one service/container:

```toml
[[instances]]
name = "qwen3-4b"
enabled = true
port = 8081
model_file = "Qwen3-4B-Q4_K_M.gguf"
alias = "qwen3-4b-local"
backend = "vulkan"
parallel = 2
```

## Commands

```bash
make instances-render   # generate docker-compose.instances.yml
make instances-check    # verify model files and backend devices
make instances-config   # render then show docker compose config
make instances-up       # start all enabled instances
make instances-logs     # follow logs for all instances
make instances-ps       # show instance containers
make instances-down     # stop all instances
```

The generated file is ignored by git:

```text
docker-compose.instances.yml
configs/instances.toml
```

## Ports and model names

Each instance exposes the same internal llama-server port `8080`, mapped to a unique host port:

```text
qwen3-4b   -> http://127.0.0.1:8081/v1/chat/completions
coder-7b   -> http://127.0.0.1:8082/v1/chat/completions
```

Each instance should also have a distinct `alias`, because OpenAI-compatible clients send a `model` field.

## Resource notes

Every instance loads its own model into RAM/VRAM. Multiple Vulkan instances on one AMD GPU are simple and debuggable, but not free. Start with small `parallel` values and only enable the models you need.

For production-style access, put a router/reverse proxy in front of these instances instead of exposing all ports publicly.
