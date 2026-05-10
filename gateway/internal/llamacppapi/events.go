package llamacppapi

import (
	"encoding/json"
	"fmt"
)

type ResponseCompletedEvent struct {
	Response             Response               `json:"response"`
	Type                 string                 `json:"type"`
	AdditionalProperties map[string]interface{} `json:"-"`
}

func (e *ResponseCompletedEvent) UnmarshalJSON(b []byte) error {
	object := make(map[string]json.RawMessage)
	if err := json.Unmarshal(b, &object); err != nil {
		return err
	}
	if raw, found := object["response"]; found {
		if err := json.Unmarshal(raw, &e.Response); err != nil {
			return fmt.Errorf("error reading 'response': %w", err)
		}
		delete(object, "response")
	}
	if raw, found := object["type"]; found {
		if err := json.Unmarshal(raw, &e.Type); err != nil {
			return fmt.Errorf("error reading 'type': %w", err)
		}
		delete(object, "type")
	}
	if len(object) != 0 {
		e.AdditionalProperties = make(map[string]interface{})
		for fieldName, fieldBuf := range object {
			var fieldVal interface{}
			if err := json.Unmarshal(fieldBuf, &fieldVal); err != nil {
				return fmt.Errorf("error unmarshaling field %s: %w", fieldName, err)
			}
			e.AdditionalProperties[fieldName] = fieldVal
		}
	}
	return nil
}
