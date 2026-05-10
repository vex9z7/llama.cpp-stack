package apiadapter

import (
	"encoding/json"
	"testing"

	"github.com/vex9z7/llama.cpp-stack/gateway/internal/llamacppapi"
)

func TestOpenAIUsageFromLlamaFillsNilDetails(t *testing.T) {
	got := OpenAIUsageFromLlama(&llamacppapi.ResponseUsage{InputTokens: 1, OutputTokens: 2, TotalTokens: 3})
	if got == nil {
		t.Fatal("usage is nil")
	}
	if got.InputTokensDetails.CachedTokens != 0 {
		t.Fatalf("cached_tokens=%d", got.InputTokensDetails.CachedTokens)
	}
	if got.OutputTokensDetails.ReasoningTokens != 0 {
		t.Fatalf("reasoning_tokens=%d", got.OutputTokensDetails.ReasoningTokens)
	}
}

func TestAdaptResponsesBodyPreservesFieldsAndFillsOutputDetails(t *testing.T) {
	body := []byte(`{"id":"resp_1","object":"response","usage":{"input_tokens":1,"input_tokens_details":{"cached_tokens":0},"output_tokens":2,"output_tokens_details":null,"total_tokens":3}}`)
	got, err := AdaptResponsesBody(body)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(got, &out); err != nil {
		t.Fatal(err)
	}
	if out["id"] != "resp_1" {
		t.Fatalf("id not preserved: %#v", out["id"])
	}
	usage := out["usage"].(map[string]any)
	details := usage["output_tokens_details"].(map[string]any)
	if details["reasoning_tokens"] != float64(0) {
		t.Fatalf("reasoning_tokens=%v", details["reasoning_tokens"])
	}
}

func TestAdaptResponsesSSEPayloadOnlyChangesCompletedEvent(t *testing.T) {
	other := []byte(`{"type":"response.output_text.delta","delta":"hi"}`)
	got, changed, err := AdaptResponsesSSEPayload(other)
	if err != nil {
		t.Fatal(err)
	}
	if changed || string(got) != string(other) {
		t.Fatalf("unexpected non-completed adaptation changed=%v got=%s", changed, got)
	}

	completed := []byte(`{"type":"response.completed","response":{"id":"resp_1","usage":{"input_tokens":1,"input_tokens_details":{"cached_tokens":0},"output_tokens":2,"output_tokens_details":null,"total_tokens":3}}}`)
	got, changed, err = AdaptResponsesSSEPayload(completed)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected completed event to change")
	}
	var out map[string]any
	if err := json.Unmarshal(got, &out); err != nil {
		t.Fatal(err)
	}
	response := out["response"].(map[string]any)
	usage := response["usage"].(map[string]any)
	details := usage["output_tokens_details"].(map[string]any)
	if details["reasoning_tokens"] != float64(0) {
		t.Fatalf("reasoning_tokens=%v", details["reasoning_tokens"])
	}
}
