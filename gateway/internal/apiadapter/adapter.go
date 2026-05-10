package apiadapter

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"strings"
)

const (
	PathChatCompletions = "/v1/chat/completions"
	PathResponses       = "/v1/responses"
)

type AdaptedRequest struct {
	Body    []byte
	Changed bool
}

func AdaptRequest(path string, body []byte) (AdaptedRequest, error) {
	if path != PathChatCompletions && path != PathResponses {
		return AdaptedRequest{Body: body}, nil
	}
	var obj map[string]any
	if err := json.Unmarshal(body, &obj); err != nil {
		return AdaptedRequest{}, err
	}
	if !reasoningDisabled(obj) || hasExplicitEnableThinking(obj) {
		return AdaptedRequest{Body: body}, nil
	}
	kwargs, _ := obj["chat_template_kwargs"].(map[string]any)
	if kwargs == nil {
		kwargs = map[string]any{}
		obj["chat_template_kwargs"] = kwargs
	}
	kwargs["enable_thinking"] = false
	out, err := json.Marshal(obj)
	if err != nil {
		return AdaptedRequest{}, err
	}
	return AdaptedRequest{Body: out, Changed: true}, nil
}

func NormalizeResponsesJSON(body []byte) ([]byte, bool, error) {
	var obj map[string]any
	if err := json.Unmarshal(body, &obj); err != nil {
		return nil, false, err
	}
	changed := normalizeResponseObject(obj)
	if !changed {
		return body, false, nil
	}
	out, err := json.Marshal(obj)
	if err != nil {
		return nil, false, err
	}
	return out, true, nil
}

func NormalizeResponsesSSE(w io.Writer, r io.Reader) error {
	s := bufio.NewScanner(r)
	s.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for s.Scan() {
		line := s.Text()
		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			if data != "" && data != "[DONE]" {
				if out, ok := normalizeResponsesEventData([]byte(data)); ok {
					line = "data: " + string(out)
				}
			}
		}
		if _, err := io.WriteString(w, line+"\n"); err != nil {
			return err
		}
		if f, ok := w.(interface{ Flush() }); ok {
			f.Flush()
		}
	}
	return s.Err()
}

func normalizeResponsesEventData(data []byte) ([]byte, bool) {
	var obj map[string]any
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(&obj); err != nil {
		return nil, false
	}
	changed := false
	if response, ok := obj["response"].(map[string]any); ok {
		changed = normalizeResponseObject(response) || changed
	} else {
		changed = normalizeResponseObject(obj) || changed
	}
	if !changed {
		return nil, false
	}
	out, err := json.Marshal(obj)
	if err != nil {
		return nil, false
	}
	return out, true
}

func normalizeResponseObject(obj map[string]any) bool {
	usage, ok := obj["usage"].(map[string]any)
	if !ok || usage == nil {
		return false
	}
	changed := false
	if details, ok := usage["input_tokens_details"].(map[string]any); !ok || details == nil {
		usage["input_tokens_details"] = map[string]any{"cached_tokens": 0}
		changed = true
	} else if _, ok := details["cached_tokens"]; !ok {
		details["cached_tokens"] = 0
		changed = true
	}
	if details, ok := usage["output_tokens_details"].(map[string]any); !ok || details == nil {
		usage["output_tokens_details"] = map[string]any{"reasoning_tokens": 0}
		changed = true
	} else if _, ok := details["reasoning_tokens"]; !ok {
		details["reasoning_tokens"] = 0
		changed = true
	}
	if _, ok := usage["total_tokens"]; !ok {
		if in, okIn := numberValue(usage["input_tokens"]); okIn {
			if out, okOut := numberValue(usage["output_tokens"]); okOut {
				usage["total_tokens"] = in + out
				changed = true
			}
		}
	}
	return changed
}

func hasExplicitEnableThinking(obj map[string]any) bool {
	kwargs, ok := obj["chat_template_kwargs"].(map[string]any)
	if !ok || kwargs == nil {
		return false
	}
	_, ok = kwargs["enable_thinking"]
	return ok
}

func reasoningDisabled(obj map[string]any) bool {
	if disabledString(obj["reasoning_effort"]) {
		return true
	}
	reasoning, ok := obj["reasoning"].(map[string]any)
	if !ok || reasoning == nil {
		return false
	}
	if enabled, ok := reasoning["enabled"].(bool); ok && !enabled {
		return true
	}
	return disabledString(reasoning["effort"])
}

func disabledString(v any) bool {
	s, ok := v.(string)
	if !ok {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "none", "off", "disabled", "disable", "false":
		return true
	default:
		return false
	}
}

func numberValue(v any) (int64, bool) {
	switch n := v.(type) {
	case int:
		return int64(n), true
	case int64:
		return n, true
	case float64:
		return int64(n), true
	case json.Number:
		i, err := n.Int64()
		return i, err == nil
	default:
		return 0, false
	}
}
