package apiadapter

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/vex9z7/llama.cpp-stack/gateway/internal/llamacppapi"
)

func TestOpenAICompletionUsageFromLlamaFillsNilDetails(t *testing.T) {
	got := OpenAICompletionUsageFromLlama(&llamacppapi.CompletionUsage{PromptTokens: 1, CompletionTokens: 2, TotalTokens: 3})
	if got == nil {
		t.Fatal("usage is nil")
	}
	if got.PromptTokensDetails.CachedTokens != 0 {
		t.Fatalf("cached_tokens=%d", got.PromptTokensDetails.CachedTokens)
	}
	if got.CompletionTokensDetails.ReasoningTokens != 0 {
		t.Fatalf("reasoning_tokens=%d", got.CompletionTokensDetails.ReasoningTokens)
	}
}

func TestAdaptChatCompletionBodyFillsCompletionDetails(t *testing.T) {
	body := []byte(`{"id":"chat_1","object":"chat.completion","usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3,"prompt_tokens_details":null,"completion_tokens_details":null}}`)
	got, err := AdaptChatCompletionBody(body)
	if err != nil {
		t.Fatal(err)
	}
	text := string(got)
	if !strings.Contains(text, `"completion_tokens_details":{"reasoning_tokens":0}`) {
		t.Fatalf("missing completion details: %s", text)
	}
	if !strings.Contains(text, `"prompt_tokens_details":{"cached_tokens":0}`) {
		t.Fatalf("missing prompt details: %s", text)
	}
}

func TestAdaptCompletionBodyFillsCompletionDetails(t *testing.T) {
	body := []byte(`{"id":"cmpl_1","object":"text_completion","usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3}}`)
	got, err := AdaptCompletionBody(body)
	if err != nil {
		t.Fatal(err)
	}
	text := string(got)
	if !strings.Contains(text, `"completion_tokens_details":{"reasoning_tokens":0}`) {
		t.Fatalf("missing completion details: %s", text)
	}
}

func TestAdaptEmbeddingBodyKeepsEmbeddingUsageTyped(t *testing.T) {
	body := []byte(`{"object":"list","data":[],"usage":{"prompt_tokens":4,"total_tokens":4}}`)
	got, err := AdaptEmbeddingBody(body)
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		Usage struct {
			PromptTokens int `json:"prompt_tokens"`
			TotalTokens  int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(got, &out); err != nil {
		t.Fatal(err)
	}
	if out.Usage.PromptTokens != 4 || out.Usage.TotalTokens != 4 {
		t.Fatalf("usage=%+v", out.Usage)
	}
}
