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
	out := &openaiapi.CompletionUsage{
		PromptTokens:     in.PromptTokens,
		CompletionTokens: in.CompletionTokens,
		TotalTokens:      in.TotalTokens,
	}
	if in.PromptTokensDetails != nil {
		out.PromptTokensDetails.CachedTokens = in.PromptTokensDetails.CachedTokens
		out.PromptTokensDetails.AudioTokens = in.PromptTokensDetails.AudioTokens
	}
	if in.CompletionTokensDetails != nil {
		out.CompletionTokensDetails.ReasoningTokens = in.CompletionTokensDetails.ReasoningTokens
		out.CompletionTokensDetails.AudioTokens = in.CompletionTokensDetails.AudioTokens
		out.CompletionTokensDetails.AcceptedPredictionTokens = in.CompletionTokensDetails.AcceptedPredictionTokens
		out.CompletionTokensDetails.RejectedPredictionTokens = in.CompletionTokensDetails.RejectedPredictionTokens
	}
	return out
}

func AdaptChatCompletionBody(body []byte) ([]byte, error) {
	var upstream llamacppapi.ChatCompletion
	if err := json.Unmarshal(body, &upstream); err != nil {
		return body, err
	}
	return json.Marshal(openaiapi.ChatCompletion{Fields: upstream.Fields, Usage: OpenAICompletionUsageFromLlama(upstream.Usage)})
}

func AdaptCompletionBody(body []byte) ([]byte, error) {
	var upstream llamacppapi.Completion
	if err := json.Unmarshal(body, &upstream); err != nil {
		return body, err
	}
	return json.Marshal(openaiapi.Completion{Fields: upstream.Fields, Usage: OpenAICompletionUsageFromLlama(upstream.Usage)})
}

func AdaptEmbeddingBody(body []byte) ([]byte, error) {
	var upstream llamacppapi.EmbeddingResponse
	if err := json.Unmarshal(body, &upstream); err != nil {
		return body, err
	}
	var usage *openaiapi.EmbeddingUsage
	if upstream.Usage != nil {
		usage = &openaiapi.EmbeddingUsage{PromptTokens: upstream.Usage.PromptTokens, TotalTokens: upstream.Usage.TotalTokens}
	}
	return json.Marshal(openaiapi.EmbeddingResponse{Fields: upstream.Fields, Usage: usage})
}
