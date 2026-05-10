package apiadapter

import (
	"encoding/json"

	"github.com/vex9z7/llama.cpp-stack/gateway/internal/openaiapi"
)

// AdaptResponsesRequestBody normalizes OpenAI Responses request shorthand into
// the explicit message-item shape expected by the pinned llama.cpp Responses
// implementation. In particular, OpenAI accepts previous assistant history as
// {"role":"assistant","content":"..."}; llama.cpp b8840 requires an item type
// and assistant output content as output_text.
func AdaptResponsesRequestBody(body []byte) ([]byte, error) {
	var req openaiapi.ResponseCreateRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return body, err
	}
	if req.Input == nil {
		return body, nil
	}
	items, ok := responseInputItems(req.Input)
	if !ok {
		return body, nil
	}
	changed := false
	for i := range items {
		item, itemChanged, err := normalizeResponseInputItem(items[i])
		if err != nil {
			return body, err
		}
		if itemChanged {
			items[i] = item
			changed = true
		}
	}
	if !changed {
		return body, nil
	}
	if err := req.Input.FromResponseInput1(items); err != nil {
		return body, err
	}
	return json.Marshal(req)
}

func responseInputItems(input *openaiapi.ResponseInput) ([]openaiapi.ResponseInputItem, bool) {
	items, err := input.AsResponseInput1()
	return items, err == nil
}

func responseInputMessage(item openaiapi.ResponseInputItem) (openaiapi.EasyInputMessage, bool) {
	message, err := item.AsEasyInputMessage()
	return message, err == nil && message.Role.Valid()
}

func normalizeResponseInputItem(item openaiapi.ResponseInputItem) (openaiapi.ResponseInputItem, bool, error) {
	message, ok := responseInputMessage(item)
	if !ok {
		return item, false, nil
	}

	changed := false
	if message.Type == nil {
		messageType := openaiapi.EasyInputMessageTypeMessage
		message.Type = &messageType
		changed = true
	}

	text, err := message.Content.AsEasyInputMessageContent0()
	if err == nil {
		contentType := contentTypeForRole(message.Role)
		message.Content = openaiapi.EasyInputMessageContent{}
		if err := message.Content.FromEasyInputMessageContent1([]openaiapi.InputMessageContent{{Type: contentType, Text: &text}}); err != nil {
			return item, false, err
		}
		changed = true
	}

	if !changed {
		return item, false, nil
	}
	var out openaiapi.ResponseInputItem
	if err := out.FromEasyInputMessage(message); err != nil {
		return item, false, err
	}
	return out, true, nil
}

func contentTypeForRole(role openaiapi.EasyInputMessageRole) openaiapi.InputMessageContentType {
	if role == openaiapi.EasyInputMessageRoleAssistant {
		return openaiapi.InputMessageContentTypeOutputText
	}
	return openaiapi.InputMessageContentTypeInputText
}
