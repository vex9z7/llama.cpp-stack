package server

import (
	"encoding/json"
	"testing"
)

func TestBuildNChoiceRequestSplitsChatRequest(t *testing.T) {
	got, err := buildNChoiceRequest("/v1/chat/completions", []byte(`{"model":"m","n":3,"stream":false,"messages":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	if got.N != 3 || len(got.Bodies) != 3 {
		t.Fatalf("N=%d len=%d", got.N, len(got.Bodies))
	}
	for _, body := range got.Bodies {
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatal(err)
		}
		if payload["n"] != float64(1) {
			t.Fatalf("upstream n=%v body=%s", payload["n"], body)
		}
	}
}

func TestBuildNChoiceRequestAllowsStreamingN(t *testing.T) {
	got, err := buildNChoiceRequest("/v1/chat/completions", []byte(`{"model":"m","n":2,"stream":true,"messages":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	if got.N != 2 || !got.Stream {
		t.Fatalf("N=%d stream=%v", got.N, got.Stream)
	}
}

func TestAggregateNChoiceBodiesIndexesChoicesAndSumsUsage(t *testing.T) {
	got, err := aggregateNChoiceBodies([][]byte{
		[]byte(`{"id":"a","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"one"}}],"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12,"prompt_tokens_details":{"cached_tokens":1}}}`),
		[]byte(`{"id":"b","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"two"}}],"usage":{"prompt_tokens":10,"completion_tokens":3,"total_tokens":13,"prompt_tokens_details":{"cached_tokens":2}}}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		Choices []struct {
			Index   int `json:"index"`
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens        int `json:"prompt_tokens"`
			CompletionTokens    int `json:"completion_tokens"`
			TotalTokens         int `json:"total_tokens"`
			PromptTokensDetails struct {
				CachedTokens int `json:"cached_tokens"`
			} `json:"prompt_tokens_details"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(got, &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Choices) != 2 || out.Choices[0].Index != 0 || out.Choices[1].Index != 1 {
		t.Fatalf("choices=%+v body=%s", out.Choices, got)
	}
	if out.Usage.PromptTokens != 20 || out.Usage.CompletionTokens != 5 || out.Usage.TotalTokens != 25 || out.Usage.PromptTokensDetails.CachedTokens != 3 {
		t.Fatalf("usage=%+v body=%s", out.Usage, got)
	}
}
