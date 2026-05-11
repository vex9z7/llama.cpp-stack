package server

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/vex9z7/llama.cpp-stack/gateway/internal/apiadapter"
	"github.com/vex9z7/llama.cpp-stack/gateway/internal/openaiapi"
	"github.com/vex9z7/llama.cpp-stack/gateway/internal/proxy"
)

func (a *App) writeUpstreamResponse(ctx huma.Context, resp *http.Response, model string) {
	appendResponseHeaders(ctx, resp.Header)
	if resp.Header.Get("Content-Type") == "" {
		ctx.SetHeader("Content-Type", "application/json; charset=utf-8")
	}
	ctx.SetStatus(resp.StatusCode)

	if shouldAdapt(ctx.URL().Path, resp) {
		if isEventStream(resp.Header.Get("Content-Type")) {
			if ctx.URL().Path == "/v1/responses" {
				if err := copyResponsesSSE(ctx.BodyWriter(), resp.Body); err != nil {
					a.log.Warn("responses SSE copy failed", "model", model, "error", err)
				}
				return
			}
			if err := proxy.CopyFlush(ctx.BodyWriter(), resp.Body); err != nil {
				a.log.Warn("stream proxy copy failed", "model", model, "error", err)
			}
			return
		}
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			a.log.Warn("upstream body read failed", "model", model, "path", ctx.URL().Path, "error", err)
			return
		}
		body, err = adaptBody(ctx.URL().Path, body)
		if err != nil {
			a.log.Warn("typed adaptation failed", "model", model, "path", ctx.URL().Path, "error", err)
		}
		_, _ = ctx.BodyWriter().Write(body)
		return
	}

	if err := proxy.CopyFlush(ctx.BodyWriter(), resp.Body); err != nil {
		a.log.Warn("proxy copy failed", "model", model, "error", err)
	}
}

func shouldAdapt(path string, resp *http.Response) bool {
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false
	}
	switch path {
	case "/v1/chat/completions", "/v1/completions", "/v1/responses", "/v1/embeddings":
		return true
	default:
		return false
	}
}

func adaptBody(path string, body []byte) ([]byte, error) {
	switch path {
	case "/v1/chat/completions":
		return apiadapter.AdaptChatCompletionBody(body)
	case "/v1/completions":
		return apiadapter.AdaptCompletionBody(body)
	case "/v1/responses":
		return apiadapter.AdaptResponsesBody(body)
	case "/v1/embeddings":
		return apiadapter.AdaptEmbeddingBody(body)
	default:
		return body, nil
	}
}

func isEventStream(contentType string) bool {
	return strings.Contains(strings.ToLower(contentType), "text/event-stream")
}

func copyResponsesSSE(w io.Writer, src io.Reader) error {
	flusher, _ := w.(http.Flusher)
	adapter := newResponsesSSEAdapter()
	r := bufio.NewReader(src)
	var block [][]byte
	for {
		line, err := r.ReadBytes('\n')
		if len(line) > 0 {
			block = append(block, line)
			if isBlankSSELine(line) {
				out := adapter.adaptBlock(block)
				if _, werr := w.Write(out); werr != nil {
					return werr
				}
				if flusher != nil {
					flusher.Flush()
				}
				block = nil
			}
		}
		if err != nil {
			if err == io.EOF {
				if len(block) > 0 {
					out := adapter.adaptBlock(block)
					if _, werr := w.Write(out); werr != nil {
						return werr
					}
				}
				return nil
			}
			return fmt.Errorf("responses SSE read: %w", err)
		}
	}
}

func isBlankSSELine(line []byte) bool {
	return len(bytes.TrimRight(line, "\r\n")) == 0
}

func adaptSSEDataLine(line []byte) []byte {
	return newResponsesSSEAdapter().adaptLine(line)
}

type responsesSSEAdapter struct {
	toolCalls map[string]functionCallState
}

type functionCallState struct {
	ItemID        string
	Name          string
	Arguments     string
	OutputIndex   int
	Sequence      *int
	ArgumentsDone bool
}

func newResponsesSSEAdapter() *responsesSSEAdapter {
	return &responsesSSEAdapter{toolCalls: map[string]functionCallState{}}
}

func (a *responsesSSEAdapter) adaptBlock(block [][]byte) []byte {
	for i, line := range block {
		trimmed := bytes.TrimRight(line, "\r\n")
		if !bytes.HasPrefix(trimmed, []byte("data: ")) {
			continue
		}
		adapted := a.adaptLine(line)
		if bytes.Contains(adapted, []byte("\n\nevent: ")) {
			out := make([]byte, 0, len(adapted)+1)
			out = append(out, adapted...)
			if !bytes.HasSuffix(out, []byte("\n\n")) {
				out = append(out, '\n')
			}
			return out
		}
		out := make([]byte, 0, len(adapted)+len(block)*32)
		for j, original := range block {
			if j == i {
				out = append(out, adapted...)
			} else {
				out = append(out, original...)
			}
		}
		return out
	}
	return bytes.Join(block, nil)
}

func (a *responsesSSEAdapter) adaptLine(line []byte) []byte {
	trimmed := bytes.TrimRight(line, "\r\n")
	newline := line[len(trimmed):]
	if !bytes.HasPrefix(trimmed, []byte("data: ")) {
		return line
	}
	payload := bytes.TrimPrefix(trimmed, []byte("data: "))
	if bytes.Equal(payload, []byte("[DONE]")) {
		return line
	}
	adaptedPayload, prefixEvent := a.adaptPayload(payload)
	if prefixEvent == "" {
		out := make([]byte, 0, len("data: ")+len(adaptedPayload)+len(newline))
		out = append(out, "data: "...)
		out = append(out, adaptedPayload...)
		out = append(out, newline...)
		return out
	}
	out := make([]byte, 0, len("event: ")+len(prefixEvent)+len("\ndata: ")+len(adaptedPayload)+len(newline))
	out = append(out, "event: "...)
	out = append(out, prefixEvent...)
	out = append(out, '\n')
	out = append(out, "data: "...)
	out = append(out, adaptedPayload...)
	out = append(out, newline...)
	return out
}

func (a *responsesSSEAdapter) adaptPayload(payload []byte) ([]byte, string) {
	adapted, changed, err := apiadapter.AdaptResponsesSSEPayload(payload)
	if err == nil && changed {
		payload = adapted
	}

	var envelope struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return payload, ""
	}
	switch envelope.Type {
	case "response.function_call_arguments.delta":
		a.observeArgumentsDelta(payload)
		return payload, ""
	case "response.function_call_arguments.done":
		a.observeArgumentsDone(payload)
		return payload, ""
	case "response.output_item.done":
		return a.injectArgumentsDoneIfNeeded(payload)
	default:
		return payload, ""
	}
}

func (a *responsesSSEAdapter) observeArgumentsDelta(payload []byte) {
	var event struct {
		ItemID      string `json:"item_id"`
		OutputIndex int    `json:"output_index"`
		Sequence    *int   `json:"sequence_number,omitempty"`
		Delta       string `json:"delta"`
	}
	if err := json.Unmarshal(payload, &event); err != nil || event.ItemID == "" {
		return
	}
	state := a.toolCalls[event.ItemID]
	state.ItemID = event.ItemID
	state.OutputIndex = event.OutputIndex
	state.Sequence = event.Sequence
	state.Arguments += event.Delta
	a.toolCalls[event.ItemID] = state
}

func (a *responsesSSEAdapter) observeArgumentsDone(payload []byte) {
	var event struct {
		ItemID      string `json:"item_id"`
		Name        string `json:"name"`
		OutputIndex int    `json:"output_index"`
		Sequence    *int   `json:"sequence_number,omitempty"`
		Arguments   string `json:"arguments"`
	}
	if err := json.Unmarshal(payload, &event); err != nil || event.ItemID == "" {
		return
	}
	state := a.toolCalls[event.ItemID]
	state.ItemID = event.ItemID
	state.Name = event.Name
	state.OutputIndex = event.OutputIndex
	state.Sequence = event.Sequence
	state.Arguments = event.Arguments
	state.ArgumentsDone = true
	a.toolCalls[event.ItemID] = state
}

func (a *responsesSSEAdapter) injectArgumentsDoneIfNeeded(payload []byte) ([]byte, string) {
	var event openaiapi.ResponseOutputItemDoneEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return payload, ""
	}
	item, itemOK := responseOutputFunctionCallItem(event.Item)
	if !itemOK {
		return payload, ""
	}
	itemID := firstNonEmpty(stringValue(item.Id), item.CallId)
	if itemID == "" {
		return payload, ""
	}
	state := a.toolCalls[itemID]
	if state.ArgumentsDone {
		return payload, ""
	}
	state.ItemID = itemID
	state.Name = firstNonEmpty(state.Name, item.Name)
	state.Arguments = firstNonEmpty(state.Arguments, item.Arguments)
	if event.OutputIndex != nil {
		state.OutputIndex = *event.OutputIndex
	}
	state.Sequence = event.SequenceNumber
	state.ArgumentsDone = true
	a.toolCalls[itemID] = state

	done, err := json.Marshal(openaiapi.ResponseFunctionCallArgumentsDoneEvent{
		Type:           openaiapi.ResponseFunctionCallArgumentsDoneEventTypeDone,
		ItemId:         state.ItemID,
		Name:           state.Name,
		OutputIndex:    state.OutputIndex,
		SequenceNumber: state.Sequence,
		Arguments:      state.Arguments,
	})
	if err != nil {
		return payload, ""
	}
	out := make([]byte, 0, len(done)+len("\n\nevent: response.output_item.done\ndata: ")+len(payload))
	out = append(out, done...)
	out = append(out, "\n\nevent: response.output_item.done\ndata: "...)
	out = append(out, payload...)
	return out, "response.function_call_arguments.done"
}

func responseOutputFunctionCallItem(item openaiapi.ResponseOutputItem) (openaiapi.ResponseOutputFunctionCallItem, bool) {
	functionCall, err := item.AsResponseOutputFunctionCallItem()
	if err != nil || functionCall.Type != openaiapi.ResponseOutputFunctionCallItemTypeFunctionCall {
		return openaiapi.ResponseOutputFunctionCallItem{}, false
	}
	return functionCall, true
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
