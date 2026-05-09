#!/usr/bin/env python3
"""Small readiness probe for the dynamic Go gateway.

This intentionally avoids chat completion by default because that may trigger a
large lazy model download. Use make probe-api against the gateway when a model
request should be tested end-to-end.
"""
from __future__ import annotations

import argparse
import json
import urllib.request


def get_json(base: str, path: str):
    with urllib.request.urlopen(base.rstrip("/") + path, timeout=15) as resp:
        body = resp.read().decode("utf-8")
        return resp.status, json.loads(body)


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--base-url", default="http://127.0.0.1:8090")
    args = parser.parse_args()

    status, health = get_json(args.base_url, "/health")
    assert status == 200, status
    assert health.get("status") == "ok", health

    status, models = get_json(args.base_url, "/v1/models")
    assert status == 200, status
    assert models.get("object") == "list", models
    data = models.get("data")
    assert isinstance(data, list) and data, models
    first = data[0]
    assert first.get("id") and first.get("object") == "model", first
    assert isinstance(first.get("meta"), dict), first
    print(f"gateway probe ok: {len(data)} catalog models")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
