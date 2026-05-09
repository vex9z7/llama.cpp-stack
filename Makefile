SHELL := /usr/bin/env bash

-include .env
export

BACKEND ?= $(or $(LLAMA_BACKEND),vulkan)
MODEL_FILE ?= $(or $(LLAMA_MODEL_FILE),model.gguf)
GATEWAY_SMOKE_MODEL ?= Open4bits/Qwen3-0.6b-gguf/Q4_K_M
COMPOSE_CMD ?= docker compose

LEGACY_COMPOSE_FILES_cpu := -f docker-compose.yml
LEGACY_COMPOSE_FILES_vulkan := -f docker-compose.yml -f docker-compose.vulkan.yml
LEGACY_COMPOSE_FILES_cuda := -f docker-compose.yml -f docker-compose.cuda.yml

DYNAMIC_COMPOSE_FILES_cpu := -f docker-compose.dynamic.yml
DYNAMIC_COMPOSE_FILES_vulkan := -f docker-compose.dynamic.yml -f docker-compose.dynamic.vulkan.yml
DYNAMIC_COMPOSE_FILES_cuda := -f docker-compose.dynamic.yml -f docker-compose.dynamic.cuda.yml

SUPPORTED_BACKENDS := cpu vulkan cuda
ifeq ($(filter $(BACKEND),$(SUPPORTED_BACKENDS)),)
$(error Unsupported BACKEND=$(BACKEND). Use one of: $(SUPPORTED_BACKENDS))
endif

LEGACY_COMPOSE_FILES := $(LEGACY_COMPOSE_FILES_$(BACKEND))
DYNAMIC_COMPOSE_FILES := $(DYNAMIC_COMPOSE_FILES_$(BACKEND))
LEGACY_COMPOSE := LLAMA_MODEL_FILE=$(MODEL_FILE) $(COMPOSE_CMD) $(LEGACY_COMPOSE_FILES)
DYNAMIC_COMPOSE := $(COMPOSE_CMD) $(DYNAMIC_COMPOSE_FILES)

.PHONY: check go-test schemas probe-api probe-gateway models instances-render instances-check instances-up instances-down instances-logs instances-ps instances-config dynamic-check dynamic-build dynamic-up dynamic-down dynamic-restart dynamic-logs dynamic-ps dynamic-config remove-legacy legacy-check legacy-up legacy-down legacy-restart legacy-logs legacy-ps legacy-config up down restart logs ps config smoke stream-cancel

go-test:
	go test ./...

schemas:
	@python3 -c "import json, pathlib; [json.loads(p.read_text()) for p in pathlib.Path('schemas/json').glob('*.json')]; print('json schemas ok')"
	@python3 -c "from pathlib import Path; t=Path('schemas/openapi/llama-server.openapi.yaml').read_text(); assert 'openapi: 3.1.0' in t and '/v1/chat/completions:' in t; print('openapi schema smoke ok')"

probe-api:
	python3 scripts/probe_api_schemas.py --base-url "$${BASE_URL:-http://127.0.0.1:$${GATEWAY_PORT:-$${LLAMA_PORT:-8090}}}" --model "$${GATEWAY_SMOKE_MODEL:-$(GATEWAY_SMOKE_MODEL)}"

probe-gateway:
	python3 scripts/probe_gateway.py --base-url "$${BASE_URL:-http://127.0.0.1:$${GATEWAY_PORT:-$${LLAMA_PORT:-8090}}}"

models:
	@python3 -c "import tomllib; rows=tomllib.load(open('models/catalog.toml','rb')).get('models',[]); print(f'{\"MODEL\":<54} {\"PATTERN\"}'); [print(f'{(r.get(\"repo\",\"\") + \"/\" + r.get(\"quant\",\"\")):<54} {r.get(\"pattern\") or r.get(\"file\") or (\"*\" + r.get(\"quant\",\"\") + \"*.gguf\")}') for r in rows]"

instances-render:
	python3 scripts/render_instances.py render --config "$${INSTANCES_CONFIG:-configs/instances.toml}" --output "$${INSTANCES_COMPOSE:-docker-compose.instances.yml}"

instances-check:
	python3 scripts/render_instances.py check --config "$${INSTANCES_CONFIG:-configs/instances.toml}"

instances-up: instances-render instances-check
	$(COMPOSE_CMD) -f "$${INSTANCES_COMPOSE:-docker-compose.instances.yml}" up -d --force-recreate

instances-down:
	$(COMPOSE_CMD) -f "$${INSTANCES_COMPOSE:-docker-compose.instances.yml}" down

instances-logs:
	$(COMPOSE_CMD) -f "$${INSTANCES_COMPOSE:-docker-compose.instances.yml}" logs -f

instances-ps:
	$(COMPOSE_CMD) -f "$${INSTANCES_COMPOSE:-docker-compose.instances.yml}" ps

instances-config: instances-render
	$(COMPOSE_CMD) -f "$${INSTANCES_COMPOSE:-docker-compose.instances.yml}" config

check: dynamic-check

dynamic-check:
	@$(COMPOSE_CMD) version >/dev/null 2>&1 || (echo "Compose command failed: $(COMPOSE_CMD). Install Docker Compose plugin or run with COMPOSE_CMD=docker-compose" >&2; exit 2)
	@test -f "models/catalog.toml" || (echo "Missing models/catalog.toml" >&2; exit 2)
	@case "$(BACKEND)" in \
		cpu) echo "Dynamic backend cpu: no GPU device required" ;; \
		vulkan) test -e /dev/dri || (echo "Missing /dev/dri for Vulkan backend" >&2; exit 2); echo "Dynamic backend vulkan: /dev/dri found" ;; \
		cuda) command -v nvidia-smi >/dev/null || (echo "nvidia-smi not found; install NVIDIA driver/container toolkit for CUDA backend" >&2; exit 2); nvidia-smi -L ;; \
	esac

dynamic-build:
	$(DYNAMIC_COMPOSE) build

remove-legacy:
	-$(DYNAMIC_COMPOSE) down --remove-orphans
	-$(LEGACY_COMPOSE) down --remove-orphans
	-$(COMPOSE_CMD) rm -f -s llama gateway worker-0 worker-1 2>/dev/null || true
	-docker rm -f llama-cpp llama-gateway llama-worker-0 llama-worker-1 2>/dev/null || true

dynamic-up: dynamic-check remove-legacy
	$(DYNAMIC_COMPOSE) up -d --build --force-recreate --remove-orphans

dynamic-down:
	$(DYNAMIC_COMPOSE) down --remove-orphans

dynamic-restart: dynamic-down dynamic-up

dynamic-logs:
	$(DYNAMIC_COMPOSE) logs -f

dynamic-ps:
	$(DYNAMIC_COMPOSE) ps

dynamic-config:
	$(DYNAMIC_COMPOSE) config

up: dynamic-up

down: dynamic-down

restart: dynamic-restart

logs: dynamic-logs

ps: dynamic-ps

config: dynamic-config

legacy-check:
	@$(COMPOSE_CMD) version >/dev/null 2>&1 || (echo "Compose command failed: $(COMPOSE_CMD). Install Docker Compose plugin or run with COMPOSE_CMD=docker-compose" >&2; exit 2)
	@test -n "$(MODEL_FILE)" || (echo "MODEL_FILE is empty. Set LLAMA_MODEL_FILE in .env or pass MODEL_FILE=<name>.gguf" >&2; exit 2)
	@test -f "models/$(MODEL_FILE)" || (echo "Missing model: models/$(MODEL_FILE). Place a GGUF model there or set LLAMA_MODEL_FILE." >&2; exit 2)
	@case "$(BACKEND)" in \
		cpu) echo "Legacy backend cpu: no GPU device required" ;; \
		vulkan) test -e /dev/dri || (echo "Missing /dev/dri for Vulkan backend" >&2; exit 2); echo "Legacy backend vulkan: /dev/dri found" ;; \
		cuda) command -v nvidia-smi >/dev/null || (echo "nvidia-smi not found; install NVIDIA driver/container toolkit for CUDA backend" >&2; exit 2); nvidia-smi -L ;; \
	esac

legacy-up: legacy-check
	$(LEGACY_COMPOSE) up -d --force-recreate --remove-orphans

legacy-down:
	$(LEGACY_COMPOSE) down --remove-orphans

legacy-restart: legacy-down legacy-up

legacy-logs:
	$(LEGACY_COMPOSE) logs -f llama

legacy-ps:
	$(LEGACY_COMPOSE) ps

legacy-config:
	$(LEGACY_COMPOSE) config

smoke:
	HOST_PORT=$${GATEWAY_PORT:-$${LLAMA_PORT:-8090}} LLAMA_ALIAS=$${GATEWAY_SMOKE_MODEL:-$(GATEWAY_SMOKE_MODEL)} ./scripts/smoke_stream.sh

stream-cancel:
	HOST_PORT=$${GATEWAY_PORT:-$${LLAMA_PORT:-8090}} LLAMA_ALIAS=$${GATEWAY_SMOKE_MODEL:-$(GATEWAY_SMOKE_MODEL)} ./scripts/test_cancel.sh
