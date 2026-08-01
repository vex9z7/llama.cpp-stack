package server

import (
	"bytes"
	"strings"
	"testing"
)

func TestCopyNChoiceSSEReindexesAndDelaysDone(t *testing.T) {
	input := strings.NewReader("data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"hello\"}}]}\n\n" +
		"data: [DONE]\n\n")
	var out bytes.Buffer
	if err := copyNChoiceSSE(&out, input, 2, false); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	if !strings.Contains(text, `"index":2`) {
		t.Fatalf("missing reindexed choice: %s", text)
	}
	if strings.Contains(text, `[DONE]`) {
		t.Fatalf("unexpected done: %s", text)
	}
}

func TestCopyNChoiceSSEEmitsFinalDone(t *testing.T) {
	input := strings.NewReader("data: {\"choices\":[{\"index\":0,\"delta\":{}}]}\n\n" +
		"data: [DONE]\n\n")
	var out bytes.Buffer
	if err := copyNChoiceSSE(&out, input, 1, true); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	if !strings.Contains(text, `"index":1`) || !strings.Contains(text, `data: [DONE]`) {
		t.Fatalf("bad output: %s", text)
	}
}

func TestCopyNChoiceSSEOmitUsageOnlyChunk(t *testing.T) {
	input := strings.NewReader("data: {\"choices\":[],\"usage\":{\"total_tokens\":3}}\n\n" +
		"data: [DONE]\n\n")
	var out bytes.Buffer
	if err := copyNChoiceSSE(&out, input, 0, true); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "usage") {
		t.Fatalf("usage-only chunk leaked: %s", out.String())
	}
	if !strings.Contains(out.String(), `[DONE]`) {
		t.Fatalf("missing done: %s", out.String())
	}
}
