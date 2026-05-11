#!/usr/bin/env python3
"""Generate the gateway OpenAI schema from the vendored OpenAI snapshot.

The full OpenAI OpenAPI document is intentionally not fed directly to
`oapi-codegen`: the vendored snapshot is OpenAPI 3.1 and contains constructs that
v2.7.0 cannot generate today (for example `anyOf: [{type: ...}, {type: null}]`).
This script is the production contract bridge for the gateway-supported subset:

1. Verify the vendored OpenAI snapshot still contains the official components and
   fields the gateway exposes/adapts.
2. Emit a deterministic, codegen-compatible schema subset whose public types are
   named for the gateway packages but are anchored to the official components.

If the upstream OpenAI shapes change, this script fails before type generation,
forcing an explicit schema review instead of silently drifting.
"""
from __future__ import annotations

import argparse
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
DEFAULT_SOURCE = ROOT / "openai-openapi/spec/openapi.documented.yml"
DEFAULT_OUTPUT = ROOT / "openai-api-schema.yaml"


def require(condition: bool, message: str) -> None:
    if not condition:
        raise AssertionError(message)


def component_block(source: str, name: str) -> str:
    pattern = re.compile(rf"^    {re.escape(name)}:\n(?P<body>(?:      .*\n|\n)+?)(?=^    [A-Za-z0-9_].*:\n|\Z)", re.MULTILINE)
    match = pattern.search(source)
    require(match is not None, f"OpenAI snapshot missing component {name}")
    return match.group(0)


def check_official_snapshot(source: str) -> None:
    require("openapi: 3.1.0" in source, "OpenAI snapshot is not OpenAPI 3.1")
    require("operationId: createResponse" in source, "OpenAI snapshot missing createResponse operation")

    checks: dict[str, list[str]] = {
        "CreateResponse": [
            "input:",
            "$ref: '#/components/schemas/InputParam'",
            "stream:",
        ],
        "EasyInputMessage": [
            "enum:",
            "- user",
            "- assistant",
            "- system",
            "- developer",
            "- role",
            "- content",
        ],
        "FunctionToolCall": [
            "- function_call",
            "call_id:",
            "name:",
            "arguments:",
            "- type\n        - call_id\n        - name\n        - arguments",
        ],
        "FunctionToolCallOutput": [
            "- function_call_output",
            "call_id:",
            "output:",
            "- type\n        - call_id\n        - output",
        ],
        "OutputItem": [
            "$ref: '#/components/schemas/OutputMessage'",
            "$ref: '#/components/schemas/FunctionToolCall'",
            "$ref: '#/components/schemas/ReasoningItem'",
            "discriminator:",
        ],
        "OutputMessage": [
            "- message",
            "role:",
            "- assistant",
            "content:",
            "- id\n        - type\n        - role\n        - content\n        - status",
        ],
        "ReasoningItem": [
            "- reasoning",
            "summary:",
            "$ref: '#/components/schemas/SummaryTextContent'",
            "$ref: '#/components/schemas/ReasoningTextContent'",
            "- id\n        - summary\n        - type",
        ],
        "SummaryTextContent": [
            "- summary_text",
            "text:",
            "- type\n        - text",
        ],
        "ReasoningTextContent": [
            "- reasoning_text",
            "text:",
            "- type\n        - text",
        ],
        "OutputTextContent": [
            "- output_text",
            "text:",
        ],
        "RefusalContent": [
            "- refusal",
            "refusal:",
        ],
        "ResponseCompletedEvent": [
            "response.completed",
            "$ref: '#/components/schemas/Response'",
            "sequence_number:",
            "- type\n        - response\n        - sequence_number",
        ],
        "ResponseFunctionCallArgumentsDeltaEvent": [
            "response.function_call_arguments.delta",
            "item_id:",
            "output_index:",
            "delta:",
        ],
        "ResponseFunctionCallArgumentsDoneEvent": [
            "response.function_call_arguments.done",
            "item_id:",
            "name:",
            "output_index:",
            "arguments:",
        ],
        "ResponseOutputItemDoneEvent": [
            "response.output_item.done",
            "$ref: '#/components/schemas/OutputItem'",
            "output_index:",
        ],
        "ResponseUsage": [
            "input_tokens_details:",
            "output_tokens_details:",
            "cached_tokens:",
            "reasoning_tokens:",
        ],
    }
    for name, needles in checks.items():
        block = component_block(source, name)
        for needle in needles:
            require(needle in block, f"OpenAI component {name} missing expected contract text: {needle!r}")


SCHEMA = r'''openapi: 3.1.0
info:
  title: OpenAI gateway contract subset for llama.cpp-stack
  version: 2026-05-10
  description: >
    Generated OpenAI-compatible public contract subset implemented by the
    llama.cpp-stack gateway. The upstream ground truth is the vendored OpenAI
    OpenAPI snapshot in openai-openapi/spec/openapi.documented.yml. This file is
    generated by scripts/generate_openai_gateway_schema.py and should not be
    edited by hand.
x-llamacpp-stack:
  source: openai-openapi/spec/openapi.documented.yml
  generated_by: scripts/generate_openai_gateway_schema.py
paths:
  /v1/models:
    get:
      operationId: listModels
      responses:
        '200':
          description: Model list
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ModelList'
        default:
          description: Gateway-originated OpenAI-compatible error
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ErrorBody'
  /v1/chat/completions:
    post:
      operationId: createChatCompletion
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/ModelRequest'
      responses:
        '200':
          description: Chat completion response
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ChatCompletion'
            text/event-stream:
              schema:
                type: string
        default:
          description: Gateway-originated OpenAI-compatible error
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ErrorBody'
  /v1/completions:
    post:
      operationId: createCompletion
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/ModelRequest'
      responses:
        '200':
          description: Completion response
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/Completion'
            text/event-stream:
              schema:
                type: string
        default:
          description: Gateway-originated OpenAI-compatible error
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ErrorBody'
  /v1/responses:
    post:
      operationId: createResponse
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/ResponseCreateRequest'
      responses:
        '200':
          description: Responses API response
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/Response'
            text/event-stream:
              schema:
                type: string
        default:
          description: Gateway-originated OpenAI-compatible error
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ErrorBody'
  /v1/embeddings:
    post:
      operationId: createEmbedding
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/ModelRequest'
      responses:
        '200':
          description: Embedding response
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/EmbeddingResponse'
        default:
          description: Gateway-originated OpenAI-compatible error
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ErrorBody'
components:
  schemas:
    ModelRequest:
      type: object
      required: [model]
      additionalProperties: true
      properties:
        model:
          type: string
    ResponseCreateRequest:
      x-oai-source: CreateResponse
      type: object
      required: [model]
      additionalProperties: true
      properties:
        model:
          type: string
        input:
          $ref: '#/components/schemas/ResponseInput'
        stream:
          type: boolean
    ResponseInput:
      x-oai-source: ResponseInput
      oneOf:
        - type: string
        - type: array
          items:
            $ref: '#/components/schemas/ResponseInputItem'
    ResponseInputItem:
      x-oai-source: Item
      oneOf:
        - $ref: '#/components/schemas/EasyInputMessage'
        - $ref: '#/components/schemas/ResponseFunctionCall'
        - $ref: '#/components/schemas/ResponseFunctionCallOutput'
    EasyInputMessage:
      x-oai-source: EasyInputMessage
      type: object
      required: [role, content]
      additionalProperties: true
      properties:
        type:
          type: string
          enum: [message]
        role:
          type: string
          enum: [user, assistant, system, developer]
        content:
          $ref: '#/components/schemas/EasyInputMessageContent'
    EasyInputMessageContent:
      x-oai-source: EasyInputMessage.content
      oneOf:
        - type: string
        - type: array
          items:
            $ref: '#/components/schemas/InputMessageContent'
    InputMessageContent:
      x-oai-source: InputMessageContentList
      type: object
      required: [type]
      additionalProperties: true
      properties:
        type:
          type: string
          enum: [input_text, output_text, input_image, input_file, input_audio]
        text:
          type: string
    ResponseFunctionCall:
      x-oai-source: FunctionToolCall
      type: object
      required: [type, call_id, name, arguments]
      additionalProperties: true
      properties:
        type:
          type: string
          enum: [function_call]
        call_id:
          type: string
        name:
          type: string
        arguments:
          type: string
    ResponseFunctionCallOutput:
      x-oai-source: FunctionToolCallOutput
      type: object
      required: [type, call_id, output]
      additionalProperties: true
      properties:
        type:
          type: string
          enum: [function_call_output]
        call_id:
          type: string
        output:
          type: string
    ErrorBody:
      type: object
      required: [error]
      properties:
        error:
          $ref: '#/components/schemas/ErrorObject'
    ErrorObject:
      type: object
      required: [message, type]
      properties:
        message:
          type: string
        type:
          type: string
        code:
          type: string
    ModelList:
      type: object
      required: [object, data]
      properties:
        object:
          type: string
          const: list
        data:
          type: array
          items:
            $ref: '#/components/schemas/Model'
    Model:
      type: object
      required: [id, object, owned_by, meta]
      properties:
        id:
          type: string
        object:
          type: string
        owned_by:
          type: string
        meta:
          $ref: '#/components/schemas/ModelMeta'
    ModelMeta:
      type: object
      required: [downloaded, running, cold_start, repo, quant]
      properties:
        downloaded:
          type: boolean
        router_status:
          type: string
        running:
          type: boolean
        cold_start:
          type: boolean
        repo:
          type: string
        quant:
          type: string
        kind:
          type: string
    PromptTokensDetails:
      x-oai-source: PromptTokensDetails
      type: object
      required: [cached_tokens]
      properties:
        cached_tokens:
          type: integer
        audio_tokens:
          type: integer
    CompletionTokensDetails:
      x-oai-source: CompletionTokensDetails
      type: object
      required: [reasoning_tokens]
      properties:
        reasoning_tokens:
          type: integer
        audio_tokens:
          type: integer
        accepted_prediction_tokens:
          type: integer
        rejected_prediction_tokens:
          type: integer
    CompletionUsage:
      x-oai-source: CompletionUsage
      type: object
      required: [prompt_tokens, completion_tokens, total_tokens, prompt_tokens_details, completion_tokens_details]
      properties:
        prompt_tokens:
          type: integer
        completion_tokens:
          type: integer
        total_tokens:
          type: integer
        prompt_tokens_details:
          $ref: '#/components/schemas/PromptTokensDetails'
        completion_tokens_details:
          $ref: '#/components/schemas/CompletionTokensDetails'
    ResponseCompletedEvent:
      x-oai-source: ResponseCompletedEvent
      type: object
      required: [type, response, sequence_number]
      additionalProperties: true
      properties:
        type:
          type: string
          enum: [response.completed]
        response:
          $ref: '#/components/schemas/Response'
        sequence_number:
          type: integer
    ResponseFunctionCallArgumentsDeltaEvent:
      x-oai-source: ResponseFunctionCallArgumentsDeltaEvent
      type: object
      required: [type, item_id, output_index, delta]
      additionalProperties: true
      properties:
        type:
          type: string
          enum: [response.function_call_arguments.delta]
        item_id:
          type: string
        output_index:
          type: integer
        sequence_number:
          type: integer
        delta:
          type: string
    ResponseFunctionCallArgumentsDoneEvent:
      x-oai-source: ResponseFunctionCallArgumentsDoneEvent
      type: object
      required: [type, item_id, name, output_index, arguments]
      additionalProperties: true
      properties:
        type:
          type: string
          enum: [response.function_call_arguments.done]
        item_id:
          type: string
        name:
          type: string
        output_index:
          type: integer
        sequence_number:
          type: integer
        arguments:
          type: string
    ResponseOutputItemDoneEvent:
      x-oai-source: ResponseOutputItemDoneEvent
      type: object
      required: [type, item]
      additionalProperties: true
      properties:
        type:
          type: string
          enum: [response.output_item.done]
        output_index:
          type: integer
        sequence_number:
          type: integer
        item:
          $ref: '#/components/schemas/ResponseOutputItem'
    ResponseOutputItem:
      x-oai-source: OutputItem
      oneOf:
        - $ref: '#/components/schemas/ResponseOutputFunctionCallItem'
        - $ref: '#/components/schemas/ResponseOutputMessageItem'
        - $ref: '#/components/schemas/ResponseOutputReasoningItem'
    ResponseOutputFunctionCallItem:
      x-oai-source: FunctionToolCall
      type: object
      required: [type, call_id, name, arguments]
      additionalProperties: true
      properties:
        type:
          type: string
          enum: [function_call]
        id:
          type: string
        call_id:
          type: string
        name:
          type: string
        arguments:
          type: string
        status:
          type: string
          enum: [in_progress, completed, incomplete]
    ResponseOutputMessageItem:
      x-oai-source: OutputMessage
      type: object
      required: [type, id, role, content]
      additionalProperties: true
      properties:
        type:
          type: string
          enum: [message]
        id:
          type: string
        role:
          type: string
          enum: [assistant]
        status:
          type: string
          enum: [in_progress, completed, incomplete]
        content:
          type: array
          items:
            $ref: '#/components/schemas/ResponseOutputMessageContent'
    ResponseOutputMessageContent:
      x-oai-source: OutputMessageContent
      oneOf:
        - $ref: '#/components/schemas/ResponseOutputTextContent'
        - $ref: '#/components/schemas/ResponseRefusalContent'
    ResponseOutputTextContent:
      x-oai-source: OutputTextContent
      type: object
      required: [type, text]
      additionalProperties: true
      properties:
        type:
          type: string
          enum: [output_text]
        text:
          type: string
    ResponseRefusalContent:
      x-oai-source: RefusalContent
      type: object
      required: [type, refusal]
      additionalProperties: true
      properties:
        type:
          type: string
          enum: [refusal]
        refusal:
          type: string
    ResponseOutputReasoningItem:
      x-oai-source: ReasoningItem
      type: object
      required: [type, id, summary]
      additionalProperties: true
      properties:
        type:
          type: string
          enum: [reasoning]
        id:
          type: string
        summary:
          type: array
          items:
            $ref: '#/components/schemas/ResponseOutputSummaryTextContent'
        content:
          type: array
          items:
            $ref: '#/components/schemas/ResponseOutputReasoningContent'
        encrypted_content:
          type: string
        status:
          type: string
          enum: [in_progress, completed, incomplete]
    ResponseOutputReasoningContent:
      x-oai-source: ReasoningTextContent
      type: object
      required: [type, text]
      additionalProperties: true
      properties:
        type:
          type: string
          enum: [reasoning_text]
        text:
          type: string
    ResponseOutputSummaryTextContent:
      x-oai-source: SummaryTextContent
      type: object
      required: [type, text]
      additionalProperties: true
      properties:
        type:
          type: string
          enum: [summary_text]
        text:
          type: string
    ResponseInputTokensDetails:
      x-oai-source: ResponseUsage.input_tokens_details
      type: object
      required: [cached_tokens]
      properties:
        cached_tokens:
          type: integer
    ResponseOutputTokensDetails:
      x-oai-source: ResponseUsage.output_tokens_details
      type: object
      required: [reasoning_tokens]
      properties:
        reasoning_tokens:
          type: integer
    ResponseUsage:
      x-oai-source: ResponseUsage
      type: object
      required: [input_tokens, input_tokens_details, output_tokens, output_tokens_details, total_tokens]
      properties:
        input_tokens:
          type: integer
        input_tokens_details:
          $ref: '#/components/schemas/ResponseInputTokensDetails'
        output_tokens:
          type: integer
        output_tokens_details:
          $ref: '#/components/schemas/ResponseOutputTokensDetails'
        total_tokens:
          type: integer
    ChatCompletion:
      type: object
      additionalProperties: true
      properties:
        usage:
          $ref: '#/components/schemas/CompletionUsage'
    Completion:
      type: object
      additionalProperties: true
      properties:
        usage:
          $ref: '#/components/schemas/CompletionUsage'
    Response:
      x-oai-source: Response
      type: object
      additionalProperties: true
      properties:
        id:
          type: string
        object:
          type: string
          enum: [response]
        status:
          type: string
          enum: [completed, failed, in_progress, cancelled, queued, incomplete]
        model:
          type: string
        output:
          type: array
          items:
            $ref: '#/components/schemas/ResponseOutputItem'
        usage:
          $ref: '#/components/schemas/ResponseUsage'
    EmbeddingUsage:
      type: object
      required: [prompt_tokens, total_tokens]
      properties:
        prompt_tokens:
          type: integer
        total_tokens:
          type: integer
    EmbeddingResponse:
      type: object
      additionalProperties: true
      properties:
        usage:
          $ref: '#/components/schemas/EmbeddingUsage'
'''


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--source", default=str(DEFAULT_SOURCE))
    parser.add_argument("--output", default=str(DEFAULT_OUTPUT))
    args = parser.parse_args()

    source = Path(args.source).read_text(encoding="utf-8")
    check_official_snapshot(source)
    output = Path(args.output)
    output.write_text(SCHEMA, encoding="utf-8")
    print(f"generated {output} from {args.source}")
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except Exception as exc:
        print(f"[FAIL] {exc}", file=sys.stderr)
        raise
