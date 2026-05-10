package server

import (
	"encoding/json"
	"testing"
)

func TestAdaptRequestBodyNormalizesResponsesAssistantHistory(t *testing.T) {
	body := []byte(`{"model":"m","input":[{"role":"assistant","content":"Okay."}]}`)
	got, err := adaptRequestBody("/v1/responses", body)
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
	if out.Input[0].Type != "message" || out.Input[0].Content[0].Type != "output_text" {
		t.Fatalf("request was not normalized: %s", got)
	}
}
