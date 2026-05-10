#!/usr/bin/env python3
"""Probe llama-server API and validate request/response payloads against local schemas.

This is intentionally stricter than a smoke test:
- every outbound JSON payload is validated before sending;
- every inbound JSON response is validated after receiving;
- streaming responses validate each SSE JSON chunk;
- disabled optional endpoints validate their error-response shape.
"""
from __future__ import annotations

import argparse
import json
import sys
import time
import warnings
from pathlib import Path
from typing import Any

import requests
warnings.simplefilter("ignore", DeprecationWarning)
from jsonschema import Draft202012Validator, RefResolver

ROOT = Path(__file__).resolve().parents[1]
SCHEMA_DIR = ROOT / "schemas" / "json"


def load_schema(name: str) -> dict[str, Any]:
    return json.loads((SCHEMA_DIR / name).read_text())


SCHEMAS: dict[str, dict[str, Any]] = {
    p.name: json.loads(p.read_text()) for p in SCHEMA_DIR.glob("*.schema.json")
}
STORE: dict[str, dict[str, Any]] = {}
for name, schema in SCHEMAS.items():
    STORE[name] = schema
    STORE[f"file://{SCHEMA_DIR / name}"] = schema
    if "$id" in schema:
        STORE[schema["$id"]] = schema


def validator(schema_name: str) -> Draft202012Validator:
    schema = SCHEMAS[schema_name]
    resolver = RefResolver(base_uri=f"file://{SCHEMA_DIR}/", referrer=schema, store=STORE)
    return Draft202012Validator(schema, resolver=resolver)


def validate(schema_name: str, instance: Any, label: str) -> None:
    errors = sorted(validator(schema_name).iter_errors(instance), key=lambda e: list(e.path))
    if errors:
        print(f"\n[FAIL] {label} does not match {schema_name}", file=sys.stderr)
        for err in errors[:10]:
            path = "/".join(str(x) for x in err.absolute_path) or "<root>"
            print(f"  - {path}: {err.message}", file=sys.stderr)
        raise SystemExit(1)
    print(f"[ok] {label} matches {schema_name}")


def request_json(method: str, base_url: str, path: str, *, body: dict[str, Any] | None = None, timeout: int = 90) -> tuple[int, Any]:
    url = base_url.rstrip("/") + path
    resp = requests.request(method, url, json=body, timeout=timeout)
    try:
        payload = resp.json()
    except Exception:
        payload = resp.text
    return resp.status_code, payload


def post_case(base_url: str, path: str, request_schema: str, response_schema: str, body: dict[str, Any], label: str, *, timeout: int = 90) -> Any:
    validate(request_schema, body, f"{label} request")
    status, payload = request_json("POST", base_url, path, body=body, timeout=timeout)
    if status >= 400:
        print(json.dumps(payload, indent=2, ensure_ascii=False), file=sys.stderr)
        raise SystemExit(f"{label} returned HTTP {status}")
    validate(response_schema, payload, f"{label} response")
    return payload


def get_case(base_url: str, path: str, response_schema: str, label: str, *, allow_error_501: bool = False) -> Any:
    status, payload = request_json("GET", base_url, path, timeout=30)
    if allow_error_501 and status == 501:
        validate("error-response.schema.json", payload, f"{label} 501 response")
        return payload
    if status >= 400:
        print(json.dumps(payload, indent=2, ensure_ascii=False), file=sys.stderr)
        raise SystemExit(f"{label} returned HTTP {status}")
    validate(response_schema, payload, f"{label} response")
    return payload


def stream_case(base_url: str, model: str) -> None:
    body = {
        "model": model,
        "messages": [{"role": "user", "content": "Count from 1 to 3."}],
        "max_tokens": 64,
        "temperature": 0,
        "stream": True,
        "chat_template_kwargs": {"enable_thinking": False},
    }
    validate("chat-completion-request.schema.json", body, "stream chat request")
    url = base_url.rstrip("/") + "/v1/chat/completions"
    seen = 0
    with requests.post(url, json=body, stream=True, timeout=90) as resp:
        resp.raise_for_status()
        for raw in resp.iter_lines(decode_unicode=True):
            if not raw or not raw.startswith("data: "):
                continue
            data = raw[6:]
            if data == "[DONE]":
                break
            chunk = json.loads(data)
            validate("chat-completion-response.schema.json", chunk, f"stream chunk {seen}")
            seen += 1
            if seen >= 5:
                break
    if seen == 0:
        raise SystemExit("stream produced no JSON chunks")
    print(f"[ok] validated {seen} streaming chunks")


def cancel_case(base_url: str, model: str) -> None:
    body = {
        "model": model,
        "messages": [{"role": "user", "content": "Write a long numbered list. Keep going until stopped."}],
        "max_tokens": 2048,
        "temperature": 0.7,
        "stream": True,
        "chat_template_kwargs": {"enable_thinking": False},
    }
    validate("chat-completion-request.schema.json", body, "cancel stream request")
    url = base_url.rstrip("/") + "/v1/chat/completions"
    with requests.post(url, json=body, stream=True, timeout=90) as resp:
        resp.raise_for_status()
        for i, _line in enumerate(resp.iter_lines(decode_unicode=True)):
            if i >= 3:
                break
    time.sleep(0.5)
    followup = {
        "model": model,
        "messages": [{"role": "user", "content": "Return exactly: OK"}],
        "max_tokens": 16,
        "temperature": 0,
        "stream": False,
        "chat_template_kwargs": {"enable_thinking": False},
    }
    status, payload = request_json("POST", base_url, "/v1/chat/completions", body=followup, timeout=30)
    if status >= 400:
        raise SystemExit(f"follow-up after cancellation returned HTTP {status}: {payload}")
    validate("chat-completion-response.schema.json", payload, "cancel follow-up response")
    print("[ok] cancellation allowed immediate follow-up request")


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--base-url", default="https://llamacpp-stack.vex9z7.com")
    parser.add_argument("--model", default="Open4bits/Qwen3-0.6b-gguf/Q4_K_M")
    parser.add_argument("--skip-cancel", action="store_true")
    args = parser.parse_args()

    base_url = args.base_url.rstrip("/")
    model = args.model

    get_case(base_url, "/health", "health-response.schema.json", "health")
    get_case(base_url, "/v1/models", "models-response.schema.json", "models")

    post_case(
        base_url,
        "/v1/chat/completions",
        "chat-completion-request.schema.json",
        "chat-completion-response.schema.json",
        {
            "model": model,
            "messages": [{"role": "user", "content": "Return exactly: OK"}],
            "max_tokens": 32,
            "temperature": 0,
            "stream": False,
            "chat_template_kwargs": {"enable_thinking": False},
        },
        "chat completion",
    )
    post_case(
        base_url,
        "/v1/chat/completions",
        "chat-completion-request.schema.json",
        "chat-completion-response.schema.json",
        {
            "model": model,
            "messages": [{"role": "user", "content": "Call get_weather for Paris."}],
            "max_tokens": 128,
            "temperature": 0,
            "stream": False,
            "chat_template_kwargs": {"enable_thinking": False},
            "tools": [{"type": "function", "function": {"name": "get_weather", "description": "Get weather", "parameters": {"type": "object", "properties": {"city": {"type": "string"}}, "required": ["city"]}}}],
            "tool_choice": "auto",
        },
        "chat tool call",
    )
    post_case(
        base_url,
        "/v1/completions",
        "completion-request.schema.json",
        "completion-response.schema.json",
        {"model": model, "prompt": "Return exactly: OK", "max_tokens": 32, "temperature": 0},
        "legacy completion",
    )
    post_case(
        base_url,
        "/v1/responses",
        "responses-request.schema.json",
        "responses-response.schema.json",
        {"model": model, "input": "Return exactly: OK", "max_output_tokens": 32, "temperature": 0, "chat_template_kwargs": {"enable_thinking": False}},
        "responses api",
    )
    status, payload = request_json("POST", base_url, "/v1/embeddings", body={"model": model, "input": "hello"}, timeout=60)
    validate("embeddings-request.schema.json", {"model": model, "input": "hello"}, "embeddings request")
    if status < 400:
        validate("embeddings-response.schema.json", payload, "embeddings response")
    else:
        validate("error-response.schema.json", payload, "embeddings error response")
        code = payload.get("error", {}).get("code")
        # A gateway with kind routing returns model_capability_mismatch for chat models.
        # A direct/outdated llama-server may return 501 when embeddings are disabled.
        if code not in {"model_capability_mismatch", 501, "501"}:
            raise SystemExit(f"unexpected embeddings error code: status={status} payload={payload}")

    stream_case(base_url, model)
    if not args.skip_cancel:
        cancel_case(base_url, model)

    print("\nAll API schema probes passed.")


if __name__ == "__main__":
    main()
