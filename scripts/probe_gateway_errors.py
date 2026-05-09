#!/usr/bin/env python3
"""Probe gateway OpenAI-shaped error responses."""
from __future__ import annotations

import argparse
import json
import urllib.error
import urllib.request


def post(base: str, path: str, payload: dict):
    req = urllib.request.Request(
        base.rstrip("/") + path,
        data=json.dumps(payload).encode("utf-8"),
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    try:
        with urllib.request.urlopen(req, timeout=30) as resp:
            return resp.status, json.loads(resp.read().decode("utf-8"))
    except urllib.error.HTTPError as exc:
        return exc.code, json.loads(exc.read().decode("utf-8"))


def assert_error(base: str, path: str, payload: dict, status: int, code: str):
    got_status, data = post(base, path, payload)
    assert got_status == status, (got_status, data)
    err = data.get("error", {})
    assert err.get("code") == code, data
    assert err.get("message"), data
    print(f"error probe ok: {path} status={status} code={code}")


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--base-url", default="http://127.0.0.1:8090")
    parser.add_argument("--embedding-model", default="n24q02m/Qwen3-Embedding-0.6B-GGUF/Q4_K_M")
    args = parser.parse_args()

    assert_error(args.base_url, "/v1/chat/completions", {"messages": []}, 400, "missing_model")
    assert_error(
        args.base_url,
        "/v1/chat/completions",
        {"model": "not-a-real/model/Q4", "messages": [{"role": "user", "content": "hi"}]},
        404,
        "model_not_found",
    )
    assert_error(
        args.base_url,
        "/v1/chat/completions",
        {"model": args.embedding_model, "messages": [{"role": "user", "content": "hi"}], "max_tokens": 4},
        400,
        "model_capability_mismatch",
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
