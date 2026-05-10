# Pipecat Responses Compatibility Notes

## Status

Observed against the public gateway endpoint:

```text
https://llamacpp-stack.vex9z7.com
```

OpenAI-compatible base URL:

```text
https://llamacpp-stack.vex9z7.com/v1
```

These notes classify observed Pipecat/OpenAI Responses compatibility gaps into:

```text
A. Already supported by llama.cpp/gateway; may need small shape normalization
B. Backend capability exists; gateway request/response adapter can make it more OpenAI-compatible
C. Not supported by current HTTP gateway/backend; requires a new protocol bridge or larger feature
```

## Observed results

### 1. Pipecat WebSocket Responses

Pipecat service:

```python
OpenAIResponsesLLMService
```

Default transport tested:

```text
wss://llamacpp-stack.vex9z7.com/v1/responses
```

Observed result:

```text
server rejected WebSocket connection: HTTP 405
```

Classification: **C — not supported by current gateway/backend**.

Reason:

- The current stack exposes HTTP endpoints.
- `/v1/responses` is served as HTTP/SSE, not WebSocket.
- Supporting this would require a WebSocket protocol bridge:

```text
Pipecat/OpenAI-style WebSocket
  -> gateway WS handler
  -> HTTP/SSE /v1/responses or chat request
  -> translate backend chunks/events back into WS messages
```

This is not a JSON field adapter issue.

Short-term recommendation:

```python
OpenAIResponsesHttpLLMService
```

### 2. Pipecat HTTP Responses streaming

Pipecat service:

```python
OpenAIResponsesHttpLLMService
```

Observed result:

- Pipeline runs.
- Text streams out.
- Pipecat receives frames like:

```text
LLMFullResponseStartFrame
LLMTextFrame
LLMFullResponseEndFrame
```

Classification: **A/B — core HTTP Responses streaming works, but shape compatibility needs adapter work**.

### 3. Responses usage details crash

Observed Pipecat error:

```text
'NoneType' object has no attribute 'reasoning_tokens'
```

Observed endpoint response shape:

```json
"usage": {
  "input_tokens": 152,
  "output_tokens": 20,
  "output_tokens_details": null
}
```

Pipecat expects OpenAI-style details object:

```json
"usage": {
  "input_tokens": 152,
  "input_tokens_details": {
    "cached_tokens": 0
  },
  "output_tokens": 20,
  "output_tokens_details": {
    "reasoning_tokens": 0
  },
  "total_tokens": 172
}
```

Classification: **A — gateway can fix with response adapter**.

This should be the highest-priority compatibility fix because it currently interrupts Pipecat post-processing and can block tool-call frame handling.

Required normalization:

- If `usage.input_tokens_details` is missing or null, set:

```json
{"cached_tokens": 0}
```

- If `usage.output_tokens_details` is missing or null, set:

```json
{"reasoning_tokens": 0}
```

- If `usage.total_tokens` is missing and both input/output token counts exist, set:

```text
total_tokens = input_tokens + output_tokens
```

Apply this to:

```text
POST /v1/responses       non-stream JSON response
POST /v1/responses       streaming response.completed event
```

Do not rewrite unrelated response fields.

### 4. Responses tool call

Observed direct OpenAI SDK test output:

```json
{
  "type": "function_call",
  "name": "lookup_weather",
  "arguments": "{\"city\": \"Paris\"}"
}
```

Classification: **A — endpoint capability exists**.

Pipecat tool-call frames did not complete because the usage details crash interrupted Pipecat processing. Retest after the Responses usage adapter is implemented.

### 5. Qwen thinking / reasoning disable

Current llama.cpp/Qwen-specific parameter:

```json
{
  "chat_template_kwargs": {
    "enable_thinking": false
  }
}
```

This is not a standard OpenAI public parameter.

Classification: **B — gateway request adapter can make this more OpenAI-compatible**.

The gateway should accept OpenAI-ish public fields such as:

```json
{
  "reasoning": {
    "effort": "none"
  }
}
```

or:

```json
{
  "reasoning": {
    "enabled": false
  }
}
```

or, for Chat Completions compatibility:

```json
{
  "reasoning_effort": "none"
}
```

and translate to:

```json
{
  "chat_template_kwargs": {
    "enable_thinking": false
  }
}
```

Only map clear disable values initially:

```text
none
off
disabled
disable
false
```

Do not map `minimal`, `low`, `medium`, `high`, or `xhigh` until model-specific behavior is tested.

If the request already contains `chat_template_kwargs.enable_thinking`, do not overwrite it.

## Priority order

### P0: Responses usage response adapter

Status: implemented. Usage details normalization applies to:

```text
/v1/responses non-stream
/v1/responses stream response.completed event
```

This should fix the observed Pipecat crash and allow tool-call frame handling to proceed.

### P1: Reasoning request adapter

Status: implemented. OpenAI-ish reasoning disable mapping applies to:

```text
/v1/chat/completions
/v1/responses
```

### P2: Tests and probes

Add tests/probes for:

- Responses non-stream usage details object exists.
- Responses streaming `response.completed` usage details object exists.
- Pipecat-style Responses tool call is not blocked by usage parsing.
- Reasoning disable adapter does not emit raw `<think>` content for Qwen3 test models.

### P3: Optional WebSocket bridge

Only implement if Pipecat's WebSocket Responses service becomes a hard requirement.

This is a separate feature, not a small compatibility adapter.

## Recommended Pipecat HTTP configuration today

Use Pipecat HTTP Responses service rather than WebSocket Responses service:

```python
from pipecat.services.openai.responses.llm import OpenAIResponsesHttpLLMService

llm = OpenAIResponsesHttpLLMService(
    api_key="dummy",
    base_url="https://llamacpp-stack.vex9z7.com/v1",
    settings=OpenAIResponsesHttpLLMService.Settings(
        model="Open4bits/Qwen3-0.6b-gguf/Q4_K_M",
        temperature=0.0,
        max_completion_tokens=512,
        extra={
            "extra_body": {
                "chat_template_kwargs": {
                    "enable_thinking": False,
                }
            }
        },
    ),
)
```

After P1 lands, the recommended request shape should move toward OpenAI-style reasoning fields instead of direct `chat_template_kwargs`.
