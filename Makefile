SHELL := /usr/bin/env bash

-include .env
export

BACKEND ?= $(or $(LLAMA_BACKEND),vulkan)
GATEWAY_SMOKE_MODEL ?= Open4bits/Qwen3-0.6b-gguf/Q4_K_M
COMPOSE_CMD ?= docker compose

COMPOSE_FILES_cpu := -f docker-compose.dynamic.yml
COMPOSE_FILES_vulkan := -f docker-compose.dynamic.yml -f docker-compose.dynamic.vulkan.yml
COMPOSE_FILES_cuda := -f docker-compose.dynamic.yml -f docker-compose.dynamic.cuda.yml

SUPPORTED_BACKENDS := cpu vulkan cuda
ifeq ($(filter $(BACKEND),$(SUPPORTED_BACKENDS)),)
$(error Unsupported BACKEND=$(BACKEND). Use one of: $(SUPPORTED_BACKENDS))
endif

COMPOSE_FILES := $(COMPOSE_FILES_$(BACKEND))
COMPOSE := $(COMPOSE_CMD) $(COMPOSE_FILES)

.PHONY: check go-test schemas probe-api probe-gateway probe-cancel probe-capacity models up down restart logs ps config build smoke stream-cancel

go-test:
	go test ./...

schemas:
	@python3 -c "import json, pathlib; [json.loads(p.read_text()) for p in pathlib.Path('schemas/json').glob('*.json')]; print('json schemas ok')"
	@python3 -c "from pathlib import Path; t=Path('schemas/openapi/llama-server.openapi.yaml').read_text(); assert 'openapi: 3.1.0' in t and '/v1/chat/completions:' in t; print('openapi schema smoke ok')"

probe-api:
	python3 scripts/probe_api_schemas.py --base-url "$${BASE_URL:-http://127.0.0.1:$${GATEWAY_PORT:-$${LLAMA_PORT:-8090}}}" --model "$${GATEWAY_SMOKE_MODEL:-$(GATEWAY_SMOKE_MODEL)}"

probe-gateway:
	python3 scripts/probe_gateway.py --base-url "$${BASE_URL:-http://127.0.0.1:$${GATEWAY_PORT:-$${LLAMA_PORT:-8090}}}"

probe-cancel:
	python3 scripts/probe_gateway_cancel.py --base-url "$${BASE_URL:-http://127.0.0.1:$${GATEWAY_PORT:-$${LLAMA_PORT:-8090}}}" --model "$${GATEWAY_SMOKE_MODEL:-$(GATEWAY_SMOKE_MODEL)}"

probe-capacity:
	python3 scripts/probe_gateway_capacity.py --base-url "$${BASE_URL:-http://127.0.0.1:$${GATEWAY_PORT:-$${LLAMA_PORT:-8090}}}"

models:
	@python3 -c "import tomllib; rows=tomllib.load(open('models/catalog.toml','rb')).get('models',[]); print(f'{\"MODEL\":<54} {\"PATTERN\"}'); [print(f'{(r.get(\"repo\",\"\") + \"/\" + r.get(\"quant\",\"\")):<54} {r.get(\"pattern\") or r.get(\"file\") or (\"*\" + r.get(\"quant\",\"\") + \"*.gguf\")}') for r in rows]"

check:
	@$(COMPOSE_CMD) version >/dev/null 2>&1 || (echo "Compose command failed: $(COMPOSE_CMD). Install Docker Compose plugin or run with COMPOSE_CMD=docker-compose" >&2; exit 2)
	@test -f "models/catalog.toml" || (echo "Missing models/catalog.toml" >&2; exit 2)
	@case "$(BACKEND)" in \
		cpu) echo "Backend cpu: no GPU device required" ;; \
		vulkan) test -e /dev/dri || (echo "Missing /dev/dri for Vulkan backend" >&2; exit 2); echo "Backend vulkan: /dev/dri found" ;; \
		cuda) command -v nvidia-smi >/dev/null || (echo "nvidia-smi not found; install NVIDIA driver/container toolkit for CUDA backend" >&2; exit 2); nvidia-smi -L ;; \
	esac

build:
	$(COMPOSE) build

up: check
	$(COMPOSE) up -d --build --force-recreate --remove-orphans

down:
	$(COMPOSE) down --remove-orphans

restart: down up

logs:
	$(COMPOSE) logs -f

ps:
	$(COMPOSE) ps

config:
	$(COMPOSE) config

smoke:
	HOST_PORT=$${GATEWAY_PORT:-$${LLAMA_PORT:-8090}} MODEL=$${GATEWAY_SMOKE_MODEL:-$(GATEWAY_SMOKE_MODEL)} ./scripts/smoke_stream.sh

stream-cancel:
	HOST_PORT=$${GATEWAY_PORT:-$${LLAMA_PORT:-8090}} MODEL=$${GATEWAY_SMOKE_MODEL:-$(GATEWAY_SMOKE_MODEL)} ./scripts/test_cancel.sh
