package openaiapi

import (
	"encoding/json"
)

type InputTokensDetails struct {
	CachedTokens int `json:"cached_tokens"`
}

type OutputTokensDetails struct {
	ReasoningTokens int `json:"reasoning_tokens"`
}

type ResponseUsage struct {
	InputTokens         int                 `json:"input_tokens"`
	InputTokensDetails  InputTokensDetails  `json:"input_tokens_details"`
	OutputTokens        int                 `json:"output_tokens"`
	OutputTokensDetails OutputTokensDetails `json:"output_tokens_details"`
	TotalTokens         int                 `json:"total_tokens"`
}

type Response struct {
	Fields map[string]json.RawMessage
	Usage  *ResponseUsage
}

func (r Response) MarshalJSON() ([]byte, error) {
	fields := cloneFields(r.Fields)
	if r.Usage != nil {
		usage, err := json.Marshal(r.Usage)
		if err != nil {
			return nil, err
		}
		fields["usage"] = usage
	}
	if fields == nil {
		fields = map[string]json.RawMessage{}
	}
	return json.Marshal(fields)
}

type ResponseCompletedEvent struct {
	Fields   map[string]json.RawMessage
	Response Response
}

func (e ResponseCompletedEvent) MarshalJSON() ([]byte, error) {
	fields := cloneFields(e.Fields)
	response, err := json.Marshal(e.Response)
	if err != nil {
		return nil, err
	}
	fields["response"] = response
	if _, ok := fields["type"]; !ok {
		typeValue, err := json.Marshal("response.completed")
		if err != nil {
			return nil, err
		}
		fields["type"] = typeValue
	}
	return json.Marshal(fields)
}

func cloneFields(in map[string]json.RawMessage) map[string]json.RawMessage {
	out := make(map[string]json.RawMessage, len(in))
	for k, v := range in {
		out[k] = append(json.RawMessage(nil), v...)
	}
	return out
}
