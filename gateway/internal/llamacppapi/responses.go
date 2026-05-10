package llamacppapi

import "encoding/json"

type InputTokensDetails struct {
	CachedTokens int `json:"cached_tokens"`
}

type OutputTokensDetails struct {
	ReasoningTokens int `json:"reasoning_tokens"`
}

type ResponseUsage struct {
	InputTokens         int                  `json:"input_tokens"`
	InputTokensDetails  *InputTokensDetails  `json:"input_tokens_details"`
	OutputTokens        int                  `json:"output_tokens"`
	OutputTokensDetails *OutputTokensDetails `json:"output_tokens_details"`
	TotalTokens         int                  `json:"total_tokens"`
}

type Response struct {
	Fields map[string]json.RawMessage
	Usage  *ResponseUsage
}

func (r *Response) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	r.Fields = fields
	if raw, ok := fields["usage"]; ok && string(raw) != "null" {
		var usage ResponseUsage
		if err := json.Unmarshal(raw, &usage); err != nil {
			return err
		}
		r.Usage = &usage
	}
	return nil
}

type ResponseCompletedEvent struct {
	Fields   map[string]json.RawMessage
	Response Response
}

func (e *ResponseCompletedEvent) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	e.Fields = fields
	if raw, ok := fields["response"]; ok && string(raw) != "null" {
		if err := json.Unmarshal(raw, &e.Response); err != nil {
			return err
		}
	}
	return nil
}
