package openaiapi

import (
	"encoding/json"
	"fmt"
)

type ResponseCompletedEvent struct {
	Response             Response               `json:"response"`
	Type                 string                 `json:"type"`
	AdditionalProperties map[string]interface{} `json:"-"`
}

func (e ResponseCompletedEvent) MarshalJSON() ([]byte, error) {
	object := make(map[string]json.RawMessage)
	response, err := json.Marshal(e.Response)
	if err != nil {
		return nil, fmt.Errorf("error marshaling 'response': %w", err)
	}
	object["response"] = response
	typeValue := e.Type
	if typeValue == "" {
		typeValue = "response.completed"
	}
	object["type"], err = json.Marshal(typeValue)
	if err != nil {
		return nil, fmt.Errorf("error marshaling 'type': %w", err)
	}
	for fieldName, field := range e.AdditionalProperties {
		object[fieldName], err = json.Marshal(field)
		if err != nil {
			return nil, fmt.Errorf("error marshaling '%s': %w", fieldName, err)
		}
	}
	return json.Marshal(object)
}
