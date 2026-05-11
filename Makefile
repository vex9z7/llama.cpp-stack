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

.PHONY: check fmt fmt-check go-test go-vet lint check-go schemas check-openai-openapi check-openai-response-contract check-llamacpp-upstream compare-llamacpp-schema check-gateway-typed-boundary check-api-types-generated update-openai-openapi generate-openai-gateway-schema generate-api-types probe-api probe-gateway probe-cancel probe-capacity probe-errors models up down restart logs ps config build smoke stream-cancel

fmt:
	gofmt -w gateway

fmt-check:
	@test -z "$$(gofmt -l gateway)" || (gofmt -l gateway; exit 1)

go-test:
	go test ./...

go-vet:
	go vet ./...

lint:
	@command -v golangci-lint >/dev/null || (echo "golangci-lint not found. Install from https://golangci-lint.run/usage/install/" >&2; exit 127)
	golangci-lint run

check-go: fmt-check go-vet go-test lint

schemas: check-openai-openapi check-openai-response-contract check-llamacpp-upstream compare-llamacpp-schema check-gateway-typed-boundary check-api-types-generated
	@echo "openai schema snapshot ok"

check-openai-openapi:
	@python3 -c "from pathlib import Path; t=Path('openai-openapi/spec/openapi.documented.yml').read_text(); assert 'openapi: 3.1.0' in t and '/responses:' in t and 'output_tokens_details' in t; print('openai upstream openapi snapshot ok')"

check-openai-response-contract:
	python3 scripts/check_openai_response_contract.py

check-llamacpp-upstream:
	python3 scripts/check_llamacpp_upstream.py

compare-llamacpp-schema:
	python3 scripts/compare_llamacpp_schema_to_source.py

check-gateway-typed-boundary:
	python3 scripts/check_gateway_typed_boundary.py

generate-openai-gateway-schema:
	python3 scripts/generate_openai_gateway_schema.py

generate-api-types:
	./scripts/generate_api_types.sh

check-api-types-generated:
	./scripts/check_api_types_generated.sh

update-openai-openapi:
	./scripts/update_openai_openapi_snapshot.sh

probe-api:
	python3 scripts/probe_openai_compat.py --base-url "$${BASE_URL:-http://127.0.0.1:$${GATEWAY_PORT:-$${LLAMA_PORT:-8090}}}" --model "$${GATEWAY_SMOKE_MODEL:-$(GATEWAY_SMOKE_MODEL)}"

probe-gateway:
	python3 scripts/probe_gateway.py --base-url "$${BASE_URL:-http://127.0.0.1:$${GATEWAY_PORT:-$${LLAMA_PORT:-8090}}}"

probe-cancel:
	python3 scripts/probe_gateway_cancel.py --base-url "$${BASE_URL:-http://127.0.0.1:$${GATEWAY_PORT:-$${LLAMA_PORT:-8090}}}" --model "$${GATEWAY_SMOKE_MODEL:-$(GATEWAY_SMOKE_MODEL)}"

probe-capacity:
	python3 scripts/probe_gateway_capacity.py --base-url "$${BASE_URL:-http://127.0.0.1:$${GATEWAY_PORT:-$${LLAMA_PORT:-8090}}}"

probe-errors:
	python3 scripts/probe_gateway_errors.py --base-url "$${BASE_URL:-http://127.0.0.1:$${GATEWAY_PORT:-$${LLAMA_PORT:-8090}}}"

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
