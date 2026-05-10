package openaiapi

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCompletionUsageMarshalNeverNullsRequiredDetails(t *testing.T) {
	body, err := json.Marshal(CompletionUsage{})
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if strings.Contains(text, `"completion_tokens_details":null`) || strings.Contains(text, `"prompt_tokens_details":null`) {
		t.Fatalf("details must not be null: %s", text)
	}
	if !strings.Contains(text, `"completion_tokens_details":{"reasoning_tokens":0}`) {
		t.Fatalf("missing reasoning_tokens: %s", text)
	}
	if !strings.Contains(text, `"prompt_tokens_details":{"cached_tokens":0}`) {
		t.Fatalf("missing cached_tokens: %s", text)
	}
}
