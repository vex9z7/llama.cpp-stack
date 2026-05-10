#!/usr/bin/env python3
"""Probe gateway streaming cancellation behavior.

This public probe cannot inspect backend /slots because the gateway intentionally
hides backend internals. It verifies the externally important behavior instead:
start a long stream, close the client connection, then immediately complete a
short request against the same model. If cancellation leaves the backend slot
stuck, the follow-up request should time out or fail.
"""
from __future__ import annotations

import argparse
import json
import time
import urllib.request


def post_json(base: str, path: str, payload: dict, timeout: float):
    req = urllib.request.Request(
        base.rstrip("/") + path,
        data=json.dumps(payload).encode("utf-8"),
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    return urllib.request.urlopen(req, timeout=timeout)


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--base-url", default="http://127.0.0.1:8090")
    parser.add_argument("--model", default="Open4bits/Qwen3-0.6b-gguf/Q4_K_M")
    parser.add_argument("--cancel-after", type=float, default=2.0)
    parser.add_argument("--followup-timeout", type=float, default=20.0)
    args = parser.parse_args()

    long_payload = {
        "model": args.model,
        "messages": [
            {
                "role": "user",
                "content": "Write a very long numbered essay about cancellation propagation in streaming LLM inference. Continue until stopped.",
            }
        ],
        "max_tokens": 2048,
        "temperature": 0.8,
        "stream": True,
        "chat_template_kwargs": {"enable_thinking": False},
    }

    started = time.monotonic()
    resp = post_json(args.base_url, "/v1/chat/completions", long_payload, timeout=60)
    received = 0
    deadline = started + args.cancel_after
    while time.monotonic() < deadline:
        chunk = resp.read(4096)
        if not chunk:
            break
        received += len(chunk)
    resp.close()

    short_payload = {
        "model": args.model,
        "messages": [{"role": "user", "content": "Reply with exactly OK."}],
        "max_tokens": 8,
        "stream": False,
        "chat_template_kwargs": {"enable_thinking": False},
    }
    follow_started = time.monotonic()
    with post_json(args.base_url, "/v1/chat/completions", short_payload, timeout=args.followup_timeout) as follow:
        body = follow.read().decode("utf-8")
        data = json.loads(body)
    elapsed = time.monotonic() - follow_started
    content = data["choices"][0]["message"].get("content", "")
    assert follow.status == 200, follow.status
    assert content, data
    print(
        f"gateway cancellation probe ok: cancelled_stream_bytes={received} "
        f"followup_seconds={elapsed:.3f} followup_content={content!r}"
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
