package apiadapter

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestAdaptRequestReasoningEffort(t *testing.T) {
	out, err := AdaptRequest(PathChatCompletions, []byte(`{"model":"m","reasoning_effort":"none"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !out.Changed {
		t.Fatal("expected changed request")
	}
	var obj map[string]any
	if err := json.Unmarshal(out.Body, &obj); err != nil {
		t.Fatal(err)
	}
	kwargs := obj["chat_template_kwargs"].(map[string]any)
	if kwargs["enable_thinking"] != false {
		t.Fatalf("enable_thinking = %#v", kwargs["enable_thinking"])
	}
}

func TestAdaptRequestReasoningObject(t *testing.T) {
	out, err := AdaptRequest(PathResponses, []byte(`{"model":"m","input":"x","reasoning":{"effort":"off"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if !out.Changed || !strings.Contains(string(out.Body), `"enable_thinking":false`) {
		t.Fatalf("request not adapted: %s", out.Body)
	}
}

func TestAdaptRequestDoesNotOverrideExplicitExtension(t *testing.T) {
	body := []byte(`{"model":"m","reasoning_effort":"none","chat_template_kwargs":{"enable_thinking":true}}`)
	out, err := AdaptRequest(PathChatCompletions, body)
	if err != nil {
		t.Fatal(err)
	}
	if out.Changed {
		t.Fatalf("explicit extension should win: %s", out.Body)
	}
}

func TestAdaptRequestPassesThroughNonDisableEffort(t *testing.T) {
	body := []byte(`{"model":"m","reasoning":{"effort":"low"}}`)
	out, err := AdaptRequest(PathResponses, body)
	if err != nil {
		t.Fatal(err)
	}
	if out.Changed {
		t.Fatalf("low effort should pass through: %s", out.Body)
	}
}

func TestNormalizeResponsesJSONUsageDetails(t *testing.T) {
	body := []byte(`{"id":"r","object":"response","status":"completed","usage":{"input_tokens":3,"output_tokens":4,"output_tokens_details":null}}`)
	out, changed, err := NormalizeResponsesJSON(body)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected normalized response")
	}
	var obj map[string]any
	if err := json.Unmarshal(out, &obj); err != nil {
		t.Fatal(err)
	}
	usage := obj["usage"].(map[string]any)
	if usage["total_tokens"].(float64) != 7 {
		t.Fatalf("total_tokens = %#v", usage["total_tokens"])
	}
	outDetails := usage["output_tokens_details"].(map[string]any)
	if outDetails["reasoning_tokens"].(float64) != 0 {
		t.Fatalf("reasoning_tokens = %#v", outDetails["reasoning_tokens"])
	}
	inDetails := usage["input_tokens_details"].(map[string]any)
	if inDetails["cached_tokens"].(float64) != 0 {
		t.Fatalf("cached_tokens = %#v", inDetails["cached_tokens"])
	}
}

func TestNormalizeResponsesSSECompletedEvent(t *testing.T) {
	in := strings.NewReader("event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":1,\"output_tokens\":2,\"output_tokens_details\":null}}}\n\n")
	var out bytes.Buffer
	if err := NormalizeResponsesSSE(&out, in); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, `"reasoning_tokens":0`) || !strings.Contains(got, `"cached_tokens":0`) || !strings.Contains(got, `"total_tokens":3`) {
		t.Fatalf("unexpected SSE output: %s", got)
	}
}
