package apiadapter

import (
	"encoding/json"

	"github.com/vex9z7/llama.cpp-stack/gateway/internal/llamacppapi"
	"github.com/vex9z7/llama.cpp-stack/gateway/internal/openaiapi"
)

func OpenAICompletionUsageFromLlama(in *llamacppapi.CompletionUsage) *openaiapi.CompletionUsage {
	if in == nil {
		return nil
	}
	return &openaiapi.CompletionUsage{
		PromptTokens:     in.PromptTokens,
		CompletionTokens: in.CompletionTokens,
		TotalTokens:      in.TotalTokens,
		PromptTokensDetails: openaiapi.PromptTokensDetails{
			CachedTokens: in.PromptTokensDetails.CachedTokens,
		},
		CompletionTokensDetails: openaiapi.CompletionTokensDetails{ReasoningTokens: 0},
	}
}

func AdaptChatCompletionBody(body []byte) ([]byte, error) {
	var upstream llamacppapi.ChatCompletion
	if err := json.Unmarshal(body, &upstream); err != nil {
		return body, err
	}
	return json.Marshal(openaiapi.ChatCompletion{AdditionalProperties: upstream.AdditionalProperties, Usage: OpenAICompletionUsageFromLlama(upstream.Usage)})
}

func AdaptCompletionBody(body []byte) ([]byte, error) {
	var upstream llamacppapi.Completion
	if err := json.Unmarshal(body, &upstream); err != nil {
		return body, err
	}
	return json.Marshal(openaiapi.Completion{AdditionalProperties: upstream.AdditionalProperties, Usage: OpenAICompletionUsageFromLlama(upstream.Usage)})
}

func AdaptEmbeddingBody(body []byte) ([]byte, error) {
	var upstream llamacppapi.EmbeddingResponse
	if err := json.Unmarshal(body, &upstream); err != nil {
		return body, err
	}
	var usage *openaiapi.EmbeddingUsage
	if upstream.Usage != nil {
		promptTokens := 0
		if upstream.Usage.PromptTokens != nil {
			promptTokens = *upstream.Usage.PromptTokens
		}
		totalTokens := 0
		if upstream.Usage.TotalTokens != nil {
			totalTokens = *upstream.Usage.TotalTokens
		}
		usage = &openaiapi.EmbeddingUsage{PromptTokens: promptTokens, TotalTokens: totalTokens}
	}
	return json.Marshal(openaiapi.EmbeddingResponse{AdditionalProperties: upstream.AdditionalProperties, Usage: usage})
}
