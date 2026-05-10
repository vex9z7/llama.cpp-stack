package openaiapi

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestResponseUsageMarshalNeverNullsRequiredDetails(t *testing.T) {
	body, err := json.Marshal(ResponseUsage{})
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if strings.Contains(text, `"output_tokens_details":null`) {
		t.Fatalf("output details must not be null: %s", text)
	}
	if !strings.Contains(text, `"output_tokens_details":{"reasoning_tokens":0}`) {
		t.Fatalf("missing required reasoning_tokens: %s", text)
	}
	if !strings.Contains(text, `"input_tokens_details":{"cached_tokens":0}`) {
		t.Fatalf("missing required cached_tokens: %s", text)
	}
}
