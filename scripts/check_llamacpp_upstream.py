#!/usr/bin/env python3
"""Static checks for the pinned llama.cpp upstream snapshot."""
from __future__ import annotations

import argparse
import re
import sys
from pathlib import Path


def parse_snapshot(path: Path) -> dict[str, str]:
    out: dict[str, str] = {}
    for line in path.read_text(encoding="utf-8").splitlines():
        if not line or line.startswith(" ") or line.startswith("#") or ":" not in line:
            continue
        key, value = line.split(":", 1)
        out[key.strip()] = value.strip()
    return out


def require(condition: bool, message: str) -> None:
    if not condition:
        raise AssertionError(message)


def check_snapshot(root: Path) -> dict[str, str]:
    snapshot_path = root / "SNAPSHOT"
    require(snapshot_path.exists(), "missing llamacpp-upstream/SNAPSHOT")
    snapshot = parse_snapshot(snapshot_path)
    expected = {
        "git_tag": "b8840",
        "git_commit": "9e5647affa54ea724196db15ec9b76c4abd16d4a",
        "image_tag_cpu": "ghcr.io/ggml-org/llama.cpp:server-b8840",
        "image_tag_vulkan": "ghcr.io/ggml-org/llama.cpp:server-vulkan-b8840",
        "image_tag_cuda": "ghcr.io/ggml-org/llama.cpp:server-cuda-b8840",
    }
    for key, value in expected.items():
        require(snapshot.get(key) == value, f"SNAPSHOT {key}={snapshot.get(key)!r}, want {value!r}")
    for key in ["image_manifest_digest_cpu", "image_manifest_digest_vulkan", "image_manifest_digest_cuda"]:
        require(snapshot.get(key, "").startswith("sha256:"), f"SNAPSHOT missing digest {key}")
    return snapshot


def check_vendored_files(root: Path, snapshot: dict[str, str]) -> None:
    required = [
        "tools/server/README.md",
        "tools/server/README-dev.md",
        "tools/server/server.cpp",
        "tools/server/server-models.cpp",
        "tools/server/server-context.cpp",
        "tools/server/server-http.cpp",
    ]
    for rel in required:
        path = root / rel
        require(path.exists(), f"missing vendored llama.cpp file: {rel}")
        require(path.stat().st_size > 0, f"empty vendored llama.cpp file: {rel}")


def extract_routes(server_cpp: Path) -> set[tuple[str, str]]:
    routes: set[tuple[str, str]] = set()
    pattern = re.compile(r'ctx_http\.(get|post)\s*\(\s*"([^"]+)"')
    for match in pattern.finditer(server_cpp.read_text(encoding="utf-8")):
        routes.add((match.group(1).upper(), match.group(2)))
    return routes


def check_routes(root: Path) -> None:
    routes = extract_routes(root / "tools/server/server.cpp")
    required = {
        ("GET", "/health"),
        ("GET", "/models"),
        ("GET", "/v1/models"),
        ("POST", "/models/load"),
        ("POST", "/models/unload"),
        ("POST", "/v1/chat/completions"),
        ("POST", "/v1/completions"),
        ("POST", "/v1/responses"),
        ("POST", "/v1/embeddings"),
    }
    missing = sorted(required - routes)
    require(not missing, f"vendored llama.cpp server.cpp missing required routes: {missing}")


def check_compose(snapshot: dict[str, str]) -> None:
    checks = {
        Path("docker-compose.dynamic.yml"): snapshot["image_tag_cpu"],
        Path("docker-compose.dynamic.vulkan.yml"): snapshot["image_tag_vulkan"],
        Path("docker-compose.dynamic.cuda.yml"): snapshot["image_tag_cuda"],
    }
    for path, image in checks.items():
        text = path.read_text(encoding="utf-8")
        require(image in text, f"{path} does not default to pinned image {image}")


def check_schema(schema_path: Path, snapshot: dict[str, str]) -> None:
    text = schema_path.read_text(encoding="utf-8")
    for needle in [
        f"git_tag: {snapshot['git_tag']}",
        f"git_commit: {snapshot['git_commit']}",
        snapshot["image_tag_cpu"],
        snapshot["image_tag_vulkan"],
        snapshot["image_tag_cuda"],
        "/models/load:",
        "/models/unload:",
        "/v1/models:",
        "/v1/responses:",
        "ResponseUsage:",
        "CompletionUsage:",
        "required: [input_tokens, output_tokens, total_tokens, input_tokens_details]",
        "required: [completion_tokens, prompt_tokens, total_tokens, prompt_tokens_details]",
    ]:
        require(needle in text, f"schema missing {needle!r}")


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--root", default="llamacpp-upstream")
    parser.add_argument("--schema", default="llamacpp-api-schema/openapi.yaml")
    args = parser.parse_args()
    root = Path(args.root)
    snapshot = check_snapshot(root)
    check_vendored_files(root, snapshot)
    check_routes(root)
    check_compose(snapshot)
    check_schema(Path(args.schema), snapshot)
    print("llama.cpp upstream snapshot checks passed.")
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except Exception as exc:
        print(f"[FAIL] {exc}", file=sys.stderr)
        raise
