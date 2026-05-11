package apiadapter

import (
	"encoding/json"

	"github.com/vex9z7/llama.cpp-stack/gateway/internal/llamacppapi"
	"github.com/vex9z7/llama.cpp-stack/gateway/internal/openaiapi"
)

func OpenAIResponseFromLlama(in llamacppapi.Response) openaiapi.Response {
	out := openaiapi.Response{
		Id:                   in.Id,
		Model:                in.Model,
		AdditionalProperties: in.AdditionalProperties,
		Usage:                OpenAIUsageFromLlama(in.Usage),
	}
	if in.Object != nil {
		object := openaiapi.ResponseObject(*in.Object)
		out.Object = &object
	}
	if in.Status != nil {
		status := openaiapi.ResponseStatus(*in.Status)
		out.Status = &status
	}
	if in.Output != nil {
		output, err := OpenAIResponseOutputFromLlama(*in.Output)
		if err == nil {
			out.Output = &output
		}
	}
	return out
}

func OpenAIResponseOutputFromLlama(in []llamacppapi.ResponseOutputItem) ([]openaiapi.ResponseOutputItem, error) {
	body, err := json.Marshal(in)
	if err != nil {
		return nil, err
	}
	var out []openaiapi.ResponseOutputItem
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func OpenAIResponseCompletedEventFromLlama(in llamacppapi.ResponseCompletedEvent) openaiapi.ResponseCompletedEvent {
	return openaiapi.ResponseCompletedEvent{AdditionalProperties: in.AdditionalProperties, Type: in.Type, Response: OpenAIResponseFromLlama(in.Response)}
}

func OpenAIUsageFromLlama(in *llamacppapi.ResponseUsage) *openaiapi.ResponseUsage {
	if in == nil {
		return nil
	}
	return &openaiapi.ResponseUsage{
		InputTokens:  in.InputTokens,
		OutputTokens: in.OutputTokens,
		TotalTokens:  in.TotalTokens,
		InputTokensDetails: openaiapi.ResponseInputTokensDetails{
			CachedTokens: in.InputTokensDetails.CachedTokens,
		},
		OutputTokensDetails: openaiapi.ResponseOutputTokensDetails{ReasoningTokens: 0},
	}
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
