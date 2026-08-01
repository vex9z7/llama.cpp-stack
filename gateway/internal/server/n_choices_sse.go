package server

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

func copyNChoiceSSE(dst io.Writer, src io.Reader, choiceIndex int, emitDone bool) error {
	reader := bufio.NewReader(src)
	var block []byte
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			block = append(block, line...)
			if isSSEBlockEnd(line) {
				if err := writeNChoiceSSEBlock(dst, block, choiceIndex, emitDone); err != nil {
					return err
				}
				block = block[:0]
			}
		}
		if err != nil {
			if err == io.EOF {
				if len(bytes.TrimSpace(block)) > 0 {
					return writeNChoiceSSEBlock(dst, block, choiceIndex, emitDone)
				}
				return nil
			}
			return fmt.Errorf("read SSE: %w", err)
		}
	}
}

func isSSEBlockEnd(line []byte) bool {
	trimmed := strings.TrimRight(string(line), "\r\n")
	return trimmed == ""
}

func writeNChoiceSSEBlock(dst io.Writer, block []byte, choiceIndex int, emitDone bool) error {
	lines := strings.Split(strings.ReplaceAll(string(block), "\r\n", "\n"), "\n")
	written := false
	for _, line := range lines {
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" {
			continue
		}
		if data == "[DONE]" {
			if emitDone {
				_, err := io.WriteString(dst, "data: [DONE]\n\n")
				return err
			}
			return nil
		}
		rewritten, ok, err := reindexSSEData([]byte(data), choiceIndex)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		if _, err := dst.Write([]byte("data: ")); err != nil {
			return err
		}
		if _, err := dst.Write(rewritten); err != nil {
			return err
		}
		if _, err := dst.Write([]byte("\n\n")); err != nil {
			return err
		}
		written = true
	}
	_ = written
	return nil
}

func reindexSSEData(data []byte, choiceIndex int) ([]byte, bool, error) {
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, false, err
	}
	rawChoices, ok := payload["choices"].([]any)
	if !ok || len(rawChoices) == 0 {
		// Streaming usage-only chunks from individual upstream calls would be
		// misleading for an emulated n-choice response, so omit them.
		return nil, false, nil
	}
	for _, choice := range rawChoices {
		if choiceMap, ok := choice.(map[string]any); ok {
			choiceMap["index"] = choiceIndex
		}
	}
	out, err := json.Marshal(payload)
	return out, true, err
}
