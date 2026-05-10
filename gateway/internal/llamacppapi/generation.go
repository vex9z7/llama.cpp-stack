package llamacppapi

import "encoding/json"

type PromptTokensDetails struct {
	CachedTokens int `json:"cached_tokens"`
	AudioTokens  int `json:"audio_tokens"`
}

type CompletionTokensDetails struct {
	ReasoningTokens          int `json:"reasoning_tokens"`
	AudioTokens              int `json:"audio_tokens"`
	AcceptedPredictionTokens int `json:"accepted_prediction_tokens"`
	RejectedPredictionTokens int `json:"rejected_prediction_tokens"`
}

type CompletionUsage struct {
	PromptTokens            int                      `json:"prompt_tokens"`
	CompletionTokens        int                      `json:"completion_tokens"`
	TotalTokens             int                      `json:"total_tokens"`
	PromptTokensDetails     *PromptTokensDetails     `json:"prompt_tokens_details"`
	CompletionTokensDetails *CompletionTokensDetails `json:"completion_tokens_details"`
}

type ChatCompletion struct {
	Fields map[string]json.RawMessage
	Usage  *CompletionUsage
}

func (c *ChatCompletion) UnmarshalJSON(data []byte) error {
	return unmarshalFieldsWithUsage(data, &c.Fields, &c.Usage)
}

type Completion struct {
	Fields map[string]json.RawMessage
	Usage  *CompletionUsage
}

func (c *Completion) UnmarshalJSON(data []byte) error {
	return unmarshalFieldsWithUsage(data, &c.Fields, &c.Usage)
}

type EmbeddingUsage struct {
	PromptTokens int `json:"prompt_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

type EmbeddingResponse struct {
	Fields map[string]json.RawMessage
	Usage  *EmbeddingUsage
}

func (r *EmbeddingResponse) UnmarshalJSON(data []byte) error {
	return unmarshalFieldsWithUsage(data, &r.Fields, &r.Usage)
}

func unmarshalFieldsWithUsage[T any](data []byte, fields *map[string]json.RawMessage, usage **T) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*fields = raw
	if body, ok := raw["usage"]; ok && string(body) != "null" {
		var parsed T
		if err := json.Unmarshal(body, &parsed); err != nil {
			return err
		}
		*usage = &parsed
	}
	return nil
}
