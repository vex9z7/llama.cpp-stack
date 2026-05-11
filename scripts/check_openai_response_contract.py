#!/usr/bin/env python3
"""Static OpenAI Responses contract checks.

This script intentionally does not call a live endpoint. It verifies that the
vendored OpenAI OpenAPI snapshot defines the Responses usage contract we depend
on, then checks static response/event fixtures against that contract. The
invalid fixtures are expected to fail so regressions in the checker itself are
caught without requiring a deployed server.
"""
from __future__ import annotations

import argparse
import json
from pathlib import Path
from typing import Any


class ContractError(AssertionError):
    pass


def assert_openai_snapshot_contract(spec_path: Path) -> None:
    text = spec_path.read_text(encoding="utf-8")
    required_needles = [
        "ResponseUsage:",
        "ResponseCompletedEvent:",
        "response.completed",
        "sequence_number:",
        "output_tokens_details:",
        "completion_tokens_details:",
        "prompt_tokens_details:",
        "reasoning_tokens:",
        "- output_tokens_details",
        "- reasoning_tokens",
    ]
    missing = [needle for needle in required_needles if needle not in text]
    if missing:
        raise ContractError(f"OpenAI OpenAPI snapshot missing required Responses usage markers: {missing}")


def usage_from_payload(payload: dict[str, Any]) -> dict[str, Any]:
    if isinstance(payload.get("usage"), dict):
        return payload["usage"]
    response = payload.get("response")
    if isinstance(response, dict) and isinstance(response.get("usage"), dict):
        return response["usage"]
    raise ContractError("payload does not contain a Responses usage object")


def assert_generation_usage_contract(payload: dict[str, Any]) -> None:
    usage = usage_from_payload(payload)
    for field in ["prompt_tokens", "prompt_tokens_details", "completion_tokens", "completion_tokens_details", "total_tokens"]:
        if field not in usage:
            raise ContractError(f"usage missing required field: {field}")
    if not isinstance(usage["prompt_tokens"], int):
        raise ContractError("usage.prompt_tokens must be an integer")
    if not isinstance(usage["completion_tokens"], int):
        raise ContractError("usage.completion_tokens must be an integer")
    if not isinstance(usage["total_tokens"], int):
        raise ContractError("usage.total_tokens must be an integer")
    prompt_details = usage["prompt_tokens_details"]
    if not isinstance(prompt_details, dict):
        raise ContractError("usage.prompt_tokens_details must be an object")
    if not isinstance(prompt_details.get("cached_tokens"), int):
        raise ContractError("usage.prompt_tokens_details.cached_tokens must be an integer")
    completion_details = usage["completion_tokens_details"]
    if not isinstance(completion_details, dict):
        raise ContractError("usage.completion_tokens_details must be an object")
    if not isinstance(completion_details.get("reasoning_tokens"), int):
        raise ContractError("usage.completion_tokens_details.reasoning_tokens must be an integer")


def assert_response_event_contract(payload: dict[str, Any]) -> None:
    if payload.get("type") == "response.completed":
        if not isinstance(payload.get("sequence_number"), int):
            raise ContractError("response.completed sequence_number must be an integer")
        response = payload.get("response")
        if not isinstance(response, dict):
            raise ContractError("response.completed response must be an object")


def assert_response_usage_contract(payload: dict[str, Any]) -> None:
    assert_response_event_contract(payload)
    usage = usage_from_payload(payload)
    for field in ["input_tokens", "input_tokens_details", "output_tokens", "output_tokens_details", "total_tokens"]:
        if field not in usage:
            raise ContractError(f"usage missing required field: {field}")

    if not isinstance(usage["input_tokens"], int):
        raise ContractError("usage.input_tokens must be an integer")
    if not isinstance(usage["output_tokens"], int):
        raise ContractError("usage.output_tokens must be an integer")
    if not isinstance(usage["total_tokens"], int):
        raise ContractError("usage.total_tokens must be an integer")

    input_details = usage["input_tokens_details"]
    if not isinstance(input_details, dict):
        raise ContractError("usage.input_tokens_details must be an object")
    if not isinstance(input_details.get("cached_tokens"), int):
        raise ContractError("usage.input_tokens_details.cached_tokens must be an integer")

    output_details = usage["output_tokens_details"]
    if not isinstance(output_details, dict):
        raise ContractError("usage.output_tokens_details must be an object")
    if not isinstance(output_details.get("reasoning_tokens"), int):
        raise ContractError("usage.output_tokens_details.reasoning_tokens must be an integer")


def check_fixture(path: Path, expect_valid: bool, contract: str) -> None:
    payload = json.loads(path.read_text(encoding="utf-8"))
    try:
        if contract == "responses":
            assert_response_usage_contract(payload)
        elif contract == "generation":
            assert_generation_usage_contract(payload)
        else:
            raise ContractError(f"unknown contract: {contract}")
    except ContractError:
        if expect_valid:
            raise
        print(f"[ok] expected invalid fixture failed contract: {path}")
        return
    if not expect_valid:
        raise ContractError(f"invalid fixture unexpectedly passed contract: {path}")
    print(f"[ok] valid fixture passed contract: {path}")


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--spec", default="openai-openapi/spec/openapi.documented.yml")
    parser.add_argument("--response-fixtures", default="scripts/fixtures/openai/responses")
    parser.add_argument("--chat-fixtures", default="scripts/fixtures/openai/chat")
    args = parser.parse_args()

    assert_openai_snapshot_contract(Path(args.spec))
    for fixture_dir, contract in [(Path(args.response_fixtures), "responses"), (Path(args.chat_fixtures), "generation")]:
        valid = sorted(fixture_dir.glob("*.valid.json"))
        invalid = sorted(fixture_dir.glob("*.invalid.json"))
        if not valid or not invalid:
            raise ContractError(f"expected both valid and invalid static fixtures in {fixture_dir}")
        for path in valid:
            check_fixture(path, expect_valid=True, contract=contract)
        for path in invalid:
            check_fixture(path, expect_valid=False, contract=contract)
    print("OpenAI static contract checks passed.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
