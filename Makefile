SHELL := /usr/bin/env bash

-include .env
export

BACKEND ?= $(or $(LLAMA_BACKEND),vulkan)
MODEL_FILE ?= $(or $(LLAMA_MODEL_FILE),model.gguf)
COMPOSE_CMD ?= docker compose

COMPOSE_FILES_cpu := -f docker-compose.yml
COMPOSE_FILES_vulkan := -f docker-compose.yml -f docker-compose.vulkan.yml
COMPOSE_FILES_cuda := -f docker-compose.yml -f docker-compose.cuda.yml

SUPPORTED_BACKENDS := cpu vulkan cuda
ifeq ($(filter $(BACKEND),$(SUPPORTED_BACKENDS)),)
$(error Unsupported BACKEND=$(BACKEND). Use one of: $(SUPPORTED_BACKENDS))
endif

COMPOSE_FILES := $(COMPOSE_FILES_$(BACKEND))
COMPOSE := LLAMA_MODEL_FILE=$(MODEL_FILE) $(COMPOSE_CMD) $(COMPOSE_FILES)

.PHONY: check schemas probe-api models instances-render instances-check instances-up instances-down instances-logs instances-ps instances-config up down restart logs ps config smoke stream-cancel

schemas:
	@python3 -c "import json, pathlib; [json.loads(p.read_text()) for p in pathlib.Path('schemas/json').glob('*.json')]; print('json schemas ok')"
	@python3 -c "from pathlib import Path; t=Path('schemas/openapi/llama-server.openapi.yaml').read_text(); assert 'openapi: 3.1.0' in t and '/v1/chat/completions:' in t; print('openapi schema smoke ok')"

probe-api:
	python3 scripts/probe_api_schemas.py --base-url "$${BASE_URL:-http://127.0.0.1:$${LLAMA_PORT:-8080}}" --model "$${LLAMA_ALIAS:-local-llm}"

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

check:
	@$(COMPOSE_CMD) version >/dev/null 2>&1 || (echo "Compose command failed: $(COMPOSE_CMD). Install Docker Compose plugin or run with COMPOSE_CMD=docker-compose" >&2; exit 2)
	@test -n "$(MODEL_FILE)" || (echo "MODEL_FILE is empty. Set LLAMA_MODEL_FILE in .env or pass MODEL_FILE=<name>.gguf" >&2; exit 2)
	@test -f "models/$(MODEL_FILE)" || (echo "Missing model: models/$(MODEL_FILE). Place a GGUF model there or set LLAMA_MODEL_FILE." >&2; exit 2)
	@case "$(BACKEND)" in \
		cpu) echo "Backend cpu: no GPU device required" ;; \
		vulkan) test -e /dev/dri || (echo "Missing /dev/dri for Vulkan backend" >&2; exit 2); echo "Backend vulkan: /dev/dri found" ;; \
		cuda) command -v nvidia-smi >/dev/null || (echo "nvidia-smi not found; install NVIDIA driver/container toolkit for CUDA backend" >&2; exit 2); nvidia-smi -L ;; \
	esac

up: check
	$(COMPOSE) up -d --force-recreate

down:
	$(COMPOSE) down

restart: down up

logs:
	$(COMPOSE) logs -f llama

ps:
	$(COMPOSE) ps

config:
	$(COMPOSE) config

smoke:
	HOST_PORT=$${LLAMA_PORT:-8080} LLAMA_ALIAS=$${LLAMA_ALIAS:-local-llm} ./scripts/smoke_stream.sh

stream-cancel:
	HOST_PORT=$${LLAMA_PORT:-8080} LLAMA_ALIAS=$${LLAMA_ALIAS:-local-llm} ./scripts/test_cancel.sh
