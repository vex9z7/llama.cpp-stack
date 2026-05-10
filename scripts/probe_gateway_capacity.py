#!/usr/bin/env python3
"""Probe router-mode gateway model switching behavior.

The gateway now delegates load/unload capacity to llama.cpp router mode. This
probe loads several catalog models in sequence and accepts either successful
router-mode LRU switching or an explicit strict-capacity 429 if that policy is
enabled later. It may trigger lazy model downloads on first run.
"""
from __future__ import annotations

import argparse
import json
import sys
import urllib.error
import urllib.request

DEFAULT_MODELS = [
    "Open4bits/Qwen3-0.6b-gguf/Q4_K_M",
    "ggml-org/Qwen3-1.7B-GGUF/Q4_K_M",
    "ggml-org/SmolLM3-3B-GGUF/Q4_K_M",
]


def post(base: str, model: str, timeout: float):
    payload = {
        "model": model,
        "messages": [{"role": "user", "content": "Reply with exactly OK."}],
        "max_tokens": 8,
        "stream": False,
        "chat_template_kwargs": {"enable_thinking": False},
    }
    req = urllib.request.Request(
        base.rstrip("/") + "/v1/chat/completions",
        data=json.dumps(payload).encode("utf-8"),
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            return resp.status, json.loads(resp.read().decode("utf-8"))
    except urllib.error.HTTPError as exc:
        body = exc.read().decode("utf-8")
        try:
            data = json.loads(body)
        except json.JSONDecodeError:
            data = {"raw": body}
        return exc.code, data


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--base-url", default="http://127.0.0.1:8090")
    parser.add_argument("--models", default=",".join(DEFAULT_MODELS), help="comma-separated: model_a,model_b,model_c")
    parser.add_argument("--timeout", type=float, default=360.0)
    args = parser.parse_args()

    models = [m.strip() for m in args.models.split(",") if m.strip()]
    if len(models) != 3:
        print("--models must contain exactly three model refs", file=sys.stderr)
        return 2

    for model in models[:2]:
        status, data = post(args.base_url, model, args.timeout)
        if status != 200:
            print(json.dumps(data, indent=2), file=sys.stderr)
            raise AssertionError(f"expected {model} to load with 200, got {status}")
        print(f"requested {model}: 200")

    status, data = post(args.base_url, models[2], args.timeout)
    if status == 200:
        print(f"gateway router-mode switching probe ok: third_model={models[2]} status=200")
        return 0
    if status == 429:
        err = data.get("error", {})
        assert err.get("code") in {"no_idle_model_slot", "no_idle_worker"}, data
        print(f"gateway strict capacity probe ok: third_model={models[2]} status=429 code={err.get('code')}")
        return 0
    print(json.dumps(data, indent=2), file=sys.stderr)
    raise AssertionError(f"expected third model to either succeed or return strict-capacity 429, got {status}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
