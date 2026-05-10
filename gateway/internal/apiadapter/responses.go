package apiadapter

import (
	"encoding/json"

	"github.com/vex9z7/llama.cpp-stack/gateway/internal/llamacppapi"
	"github.com/vex9z7/llama.cpp-stack/gateway/internal/openaiapi"
)

func OpenAIResponseFromLlama(in llamacppapi.Response) openaiapi.Response {
	return openaiapi.Response{Fields: in.Fields, Usage: OpenAIUsageFromLlama(in.Usage)}
}

func OpenAIResponseCompletedEventFromLlama(in llamacppapi.ResponseCompletedEvent) openaiapi.ResponseCompletedEvent {
	return openaiapi.ResponseCompletedEvent{Fields: in.Fields, Response: OpenAIResponseFromLlama(in.Response)}
}

func OpenAIUsageFromLlama(in *llamacppapi.ResponseUsage) *openaiapi.ResponseUsage {
	if in == nil {
		return nil
	}
	out := &openaiapi.ResponseUsage{
		InputTokens:  in.InputTokens,
		OutputTokens: in.OutputTokens,
		TotalTokens:  in.TotalTokens,
	}
	if in.InputTokensDetails != nil {
		out.InputTokensDetails.CachedTokens = in.InputTokensDetails.CachedTokens
	}
	if in.OutputTokensDetails != nil {
		out.OutputTokensDetails.ReasoningTokens = in.OutputTokensDetails.ReasoningTokens
	}
	return out
}

func AdaptResponsesBody(body []byte) ([]byte, error) {
	var upstream llamacppapi.Response
	if err := json.Unmarshal(body, &upstream); err != nil {
		return body, err
	}
	return json.Marshal(OpenAIResponseFromLlama(upstream))
}

func AdaptResponsesSSEPayload(payload []byte) ([]byte, bool, error) {
	var envelope struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return payload, false, err
	}
	if envelope.Type != "response.completed" {
		return payload, false, nil
	}
	var upstream llamacppapi.ResponseCompletedEvent
	if err := json.Unmarshal(payload, &upstream); err != nil {
		return payload, false, err
	}
	out, err := json.Marshal(OpenAIResponseCompletedEventFromLlama(upstream))
	return out, true, err
}
