package openaiapi

import "encoding/json"

type PromptTokensDetails struct {
	CachedTokens int `json:"cached_tokens"`
	AudioTokens  int `json:"audio_tokens,omitempty"`
}

type CompletionTokensDetails struct {
	ReasoningTokens          int `json:"reasoning_tokens"`
	AudioTokens              int `json:"audio_tokens,omitempty"`
	AcceptedPredictionTokens int `json:"accepted_prediction_tokens,omitempty"`
	RejectedPredictionTokens int `json:"rejected_prediction_tokens,omitempty"`
}

type CompletionUsage struct {
	PromptTokens            int                     `json:"prompt_tokens"`
	CompletionTokens        int                     `json:"completion_tokens"`
	TotalTokens             int                     `json:"total_tokens"`
	PromptTokensDetails     PromptTokensDetails     `json:"prompt_tokens_details"`
	CompletionTokensDetails CompletionTokensDetails `json:"completion_tokens_details"`
}

type ChatCompletion struct {
	Fields map[string]json.RawMessage
	Usage  *CompletionUsage
}

func (c ChatCompletion) MarshalJSON() ([]byte, error) {
	return marshalFieldsWithUsage(c.Fields, c.Usage)
}

type Completion struct {
	Fields map[string]json.RawMessage
	Usage  *CompletionUsage
}

func (c Completion) MarshalJSON() ([]byte, error) {
	return marshalFieldsWithUsage(c.Fields, c.Usage)
}

type EmbeddingUsage struct {
	PromptTokens int `json:"prompt_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

type EmbeddingResponse struct {
	Fields map[string]json.RawMessage
	Usage  *EmbeddingUsage
}

func (r EmbeddingResponse) MarshalJSON() ([]byte, error) {
	return marshalFieldsWithUsage(r.Fields, r.Usage)
}

func marshalFieldsWithUsage(fields map[string]json.RawMessage, usage any) ([]byte, error) {
	out := cloneFields(fields)
	if usage != nil {
		body, err := json.Marshal(usage)
		if err != nil {
			return nil, err
		}
		out["usage"] = body
	}
	if out == nil {
		out = map[string]json.RawMessage{}
	}
	return json.Marshal(out)
}
