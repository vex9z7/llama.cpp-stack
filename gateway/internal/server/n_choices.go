package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
)

type nChoiceRequest struct {
	Bodies [][]byte
	N      int
	Stream bool
}

func buildNChoiceRequest(path string, body []byte) (nChoiceRequest, error) {
	if path != "/v1/chat/completions" && path != "/v1/completions" {
		return nChoiceRequest{Bodies: [][]byte{body}, N: 1}, nil
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nChoiceRequest{}, err
	}
	n, ok, err := jsonInt(payload["n"])
	if err != nil {
		return nChoiceRequest{}, fmt.Errorf("field 'n' must be an integer")
	}
	if !ok || n <= 1 {
		return nChoiceRequest{Bodies: [][]byte{body}, N: 1}, nil
	}
	stream, _ := payload["stream"].(bool)
	payload["n"] = 1
	upstreamBody, err := json.Marshal(payload)
	if err != nil {
		return nChoiceRequest{}, err
	}
	bodies := make([][]byte, n)
	for i := range bodies {
		bodies[i] = upstreamBody
	}
	return nChoiceRequest{Bodies: bodies, N: n, Stream: stream}, nil
}

func jsonInt(value any) (int, bool, error) {
	if value == nil {
		return 0, false, nil
	}
	switch v := value.(type) {
	case float64:
		if math.Trunc(v) != v {
			return 0, true, errors.New("not an integer")
		}
		return int(v), true, nil
	case int:
		return v, true, nil
	case json.Number:
		i, err := v.Int64()
		return int(i), true, err
	default:
		return 0, true, errors.New("not an integer")
	}
}

func aggregateNChoiceBodies(bodies [][]byte) ([]byte, error) {
	if len(bodies) == 0 {
		return nil, errors.New("no response bodies to aggregate")
	}
	var out map[string]any
	choices := make([]any, 0, len(bodies))
	var usage map[string]any
	for _, body := range bodies {
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			return nil, err
		}
		if out == nil {
			out = payload
		}
		rawChoices, ok := payload["choices"].([]any)
		if !ok {
			return nil, errors.New("upstream response is missing choices")
		}
		for _, choice := range rawChoices {
			if choiceMap, ok := choice.(map[string]any); ok {
				choiceMap["index"] = len(choices)
			}
			choices = append(choices, choice)
		}
		if rawUsage, ok := payload["usage"].(map[string]any); ok {
			if usage == nil {
				usage = map[string]any{}
			}
			mergeUsage(usage, rawUsage)
		}
	}
	out["choices"] = choices
	if usage != nil {
		out["usage"] = usage
	}
	return json.Marshal(out)
}

func mergeUsage(dst, src map[string]any) {
	for key, value := range src {
		switch v := value.(type) {
		case float64:
			dst[key] = numeric(dst[key]) + v
		case map[string]any:
			nested, _ := dst[key].(map[string]any)
			if nested == nil {
				nested = map[string]any{}
				dst[key] = nested
			}
			mergeUsage(nested, v)
		default:
			if _, exists := dst[key]; !exists {
				dst[key] = value
			}
		}
	}
}

func numeric(value any) float64 {
	v, _ := value.(float64)
	return v
}
