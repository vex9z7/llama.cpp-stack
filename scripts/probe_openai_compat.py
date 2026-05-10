#!/usr/bin/env python3
"""Probe public gateway OpenAI-compatible behavior without local hand-written schemas."""
from __future__ import annotations

import argparse
import json
import sys
import urllib.error
import urllib.request


def post_json(base: str, path: str, payload: dict, timeout: float = 120.0):
    req = urllib.request.Request(
        base.rstrip("/") + path,
        data=json.dumps(payload).encode("utf-8"),
        headers={"Content-Type": "application/json", "Authorization": "Bearer sk-no-key-required"},
        method="POST",
    )
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            return resp.status, resp.headers.get("Content-Type", ""), resp.read().decode("utf-8")
    except urllib.error.HTTPError as exc:
        body = exc.read().decode("utf-8")
        return exc.code, exc.headers.get("Content-Type", ""), body


def assistant_text(message: dict) -> str:
    return (message.get("content") or message.get("reasoning_content") or "").strip()


def response_output_text(data: dict) -> str:
    parts = []
    for item in data.get("output", []) or []:
        for content in item.get("content", []) or []:
            text = content.get("text") or content.get("delta") or ""
            if text:
                parts.append(text)
    return "".join(parts).strip()


def assert_usage(usage: dict):
    assert isinstance(usage, dict), usage
    # OpenAI Responses usage should include token counts when provided by backend.
    for field in ["input_tokens", "input_tokens_details", "output_tokens", "output_tokens_details", "total_tokens"]:
        assert field in usage, usage
    assert isinstance(usage.get("input_tokens"), int), usage
    assert isinstance(usage.get("output_tokens"), int), usage
    assert isinstance(usage.get("total_tokens"), int), usage
    input_details = usage.get("input_tokens_details")
    assert isinstance(input_details, dict), usage
    assert isinstance(input_details.get("cached_tokens"), int), usage
    output_details = usage.get("output_tokens_details")
    assert isinstance(output_details, dict), usage
    assert isinstance(output_details.get("reasoning_tokens"), int), usage


def probe_chat(base: str, model: str):
    status, _ctype, raw = post_json(
        base,
        "/v1/chat/completions",
        {
            "model": model,
            "messages": [{"role": "user", "content": "Reply exactly: OK"}],
            "max_tokens": 256,
            "temperature": 0,
        },
    )
    if status >= 400:
        raise AssertionError(f"chat returned {status}: {raw}")
    data = json.loads(raw)
    message = data.get("choices", [{}])[0].get("message", {})
    assert assistant_text(message), data
    print("[ok] chat completions")


def probe_responses_json(base: str, model: str):
    status, _ctype, raw = post_json(
        base,
        "/v1/responses",
        {
            "model": model,
            "input": "Reply exactly: OK",
            "max_output_tokens": 256,
            "temperature": 0,
        },
    )
    if status >= 400:
        raise AssertionError(f"responses returned {status}: {raw}")
    data = json.loads(raw)
    assert response_output_text(data), data
    assert_usage(data.get("usage"))
    print("[ok] responses")


def probe_responses_stream(base: str, model: str):
    req = urllib.request.Request(
        base.rstrip("/") + "/v1/responses",
        data=json.dumps(
            {
                "model": model,
                "input": "Reply exactly: OK",
                "max_output_tokens": 256,
                "temperature": 0,
                "stream": True,
            }
        ).encode("utf-8"),
        headers={"Content-Type": "application/json", "Authorization": "Bearer sk-no-key-required"},
        method="POST",
    )
    with urllib.request.urlopen(req, timeout=120) as resp:
        completed = None
        saw_text = False
        for raw_line in resp:
            line = raw_line.decode("utf-8").strip()
            if not line.startswith("data: "):
                continue
            payload = line[6:]
            if payload == "[DONE]":
                break
            event = json.loads(payload)
            event_type = event.get("type", "")
            if event_type.endswith(".delta") and event.get("delta"):
                saw_text = True
            if event.get("type") == "response.completed":
                completed = event
                break
    if completed is None:
        raise AssertionError("stream did not produce response.completed")
    assert saw_text, completed
    assert_usage(completed.get("response", {}).get("usage"))
    print("[ok] responses stream")


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--base-url", default="http://127.0.0.1:8090")
    parser.add_argument("--model", default="Open4bits/Qwen3-0.6b-gguf/Q4_K_M")
    args = parser.parse_args()
    base = args.base_url.rstrip("/")
    probe_chat(base, args.model)
    probe_responses_json(base, args.model)
    probe_responses_stream(base, args.model)
    print("OpenAI compatibility probe passed.")
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except Exception as exc:
        print(f"[FAIL] {exc}", file=sys.stderr)
        raise
