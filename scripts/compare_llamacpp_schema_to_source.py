#!/usr/bin/env python3
"""Compare reviewed llama.cpp schema against vendored llama.cpp server source.

This is a lightweight static source/schema consistency check for the pinned
llama.cpp snapshot. It intentionally avoids runtime and does not try to infer a
full OpenAPI document from C++; it checks the reviewed schema against concrete
route and JSON-output facts present in the vendored source.
"""
from __future__ import annotations

import argparse
import re
import sys
from pathlib import Path


def require(condition: bool, message: str) -> None:
    if not condition:
        raise AssertionError(message)


def extract_server_routes(server_cpp: Path) -> set[tuple[str, str]]:
    text = server_cpp.read_text(encoding="utf-8")
    routes: set[tuple[str, str]] = set()
    pattern = re.compile(r'ctx_http\.(get|post)\s*\(\s*"([^"]+)"')
    for match in pattern.finditer(text):
        routes.add((match.group(1).upper(), match.group(2)))
    return routes


def extract_schema_paths(schema_text: str) -> set[str]:
    return set(re.findall(r"^  (/[^:]+):", schema_text, flags=re.MULTILINE))


def check_paths(schema_text: str, routes: set[tuple[str, str]]) -> None:
    schema_paths = extract_schema_paths(schema_text)
    gateway_required = {
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
    missing_source = sorted(gateway_required - routes)
    require(not missing_source, f"vendored source missing gateway-required routes: {missing_source}")
    missing_schema = sorted(path for _method, path in gateway_required if path not in schema_paths)
    require(not missing_schema, f"schema missing gateway-required paths: {missing_schema}")


def check_source_facts(schema_text: str, source_root: Path) -> None:
    task_cpp = (source_root / "server-task.cpp").read_text(encoding="utf-8")
    common_cpp = (source_root / "server-common.cpp").read_text(encoding="utf-8")
    models_cpp = (source_root / "server-models.cpp").read_text(encoding="utf-8")

    # Completion/chat usage in b8840 source has prompt_tokens_details.cached_tokens
    # but does not include completion_tokens_details.
    require('"prompt_tokens_details", json { {"cached_tokens", n_prompt_tokens_cache} }' in task_cpp,
            "source no longer shows completion prompt_tokens_details.cached_tokens")
    require('"completion_tokens_details"' not in task_cpp,
            "source now mentions completion_tokens_details; review schema and adapter")
    require("required: [completion_tokens, prompt_tokens, total_tokens, prompt_tokens_details]" in schema_text,
            "schema does not require source-backed completion usage fields")
    require("completion_tokens_details:" not in schema_text,
            "schema should not claim llama.cpp b8840 emits completion_tokens_details")

    # Responses usage in b8840 source has input_tokens_details.cached_tokens but
    # does not include output_tokens_details.
    require('"input_tokens_details", json { {"cached_tokens", n_prompt_tokens_cache} }' in task_cpp,
            "source no longer shows responses input_tokens_details.cached_tokens")
    require('"output_tokens_details"' not in task_cpp,
            "source now mentions output_tokens_details; review schema and adapter")
    require("required: [input_tokens, output_tokens, total_tokens, input_tokens_details]" in schema_text,
            "schema does not require source-backed response usage fields")
    require("output_tokens_details:" not in schema_text,
            "schema should not claim llama.cpp b8840 emits output_tokens_details")

    # Embeddings response source emits model/object/usage/data with prompt/total tokens.
    for needle in ['{"model", json_value(request, "model", model_name)}', '{"object", "list"}',
                   '{"prompt_tokens", n_tokens}', '{"total_tokens", n_tokens}', '{"data", data}']:
        require(needle in common_cpp, f"source missing embeddings response fact: {needle}")
    require("EmbeddingUsage:" in schema_text and "prompt_tokens:" in schema_text and "total_tokens:" in schema_text,
            "schema missing embedding usage fields")

    # Router model list source emits object=list and model records with id/object/owned_by/status.
    for needle in ['{"data", models_json}', '{"object", "list"}', '{"id",       meta.name}',
                   '{"object",   "model"}', '{"owned_by", "llamacpp"}', '{"status",   status}']:
        require(needle in models_cpp, f"source missing router models fact: {needle}")
    require("ModelList:" in schema_text and "ModelRecord:" in schema_text,
            "schema missing router model components")


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--source-root", default="llamacpp-upstream/tools/server")
    parser.add_argument("--schema", default="llamacpp-api-schema/openapi.yaml")
    args = parser.parse_args()

    source_root = Path(args.source_root)
    schema_text = Path(args.schema).read_text(encoding="utf-8")
    routes = extract_server_routes(source_root / "server.cpp")
    check_paths(schema_text, routes)
    check_source_facts(schema_text, source_root)
    print("llama.cpp schema/source comparison passed.")
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except Exception as exc:
        print(f"[FAIL] {exc}", file=sys.stderr)
        raise
