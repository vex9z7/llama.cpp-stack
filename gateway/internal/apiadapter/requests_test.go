package apiadapter

import (
	"encoding/json"
	"testing"
)

func TestAdaptResponsesRequestBodyNormalizesAssistantStringHistory(t *testing.T) {
	body := []byte(`{"model":"m","input":[{"role":"user","content":"Remember the number is 42."},{"role":"assistant","content":"Okay."},{"role":"user","content":"What number did I give you?"}]}`)
	got, err := AdaptResponsesRequestBody(body)
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		Input []struct {
			Type    string `json:"type"`
			Role    string `json:"role"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"input"`
	}
	if err := json.Unmarshal(got, &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Input) != 3 {
		t.Fatalf("input length=%d", len(out.Input))
	}
	if out.Input[1].Type != "message" || out.Input[1].Role != "assistant" {
		t.Fatalf("assistant item not normalized as message: %+v", out.Input[1])
	}
	if len(out.Input[1].Content) != 1 || out.Input[1].Content[0].Type != "output_text" || out.Input[1].Content[0].Text != "Okay." {
		t.Fatalf("assistant content not normalized to output_text: %+v", out.Input[1].Content)
	}
	if out.Input[0].Content[0].Type != "input_text" || out.Input[2].Content[0].Type != "input_text" {
		t.Fatalf("user content should normalize to input_text: first=%+v third=%+v", out.Input[0].Content, out.Input[2].Content)
	}
}

func TestAdaptResponsesRequestBodyPreservesTypedFunctionHistory(t *testing.T) {
	body := []byte(`{"model":"m","input":[{"type":"function_call","call_id":"call_1","name":"lookup","arguments":"{}"},{"type":"function_call_output","call_id":"call_1","output":"ok"}]}`)
	got, err := AdaptResponsesRequestBody(body)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(body) {
		t.Fatalf("typed function history should pass through unchanged\n got: %s\nwant: %s", got, body)
	}
}
