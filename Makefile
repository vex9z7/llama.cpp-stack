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

.PHONY: check up down restart logs ps config smoke stream-cancel

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
