package server

import (
	"strings"
	"testing"
)

func TestAdaptSSEDataLineFillsCompletedResponseUsage(t *testing.T) {
	line := []byte(`data: {"type":"response.completed","response":{"usage":{"input_tokens":1,"input_tokens_details":{"cached_tokens":0},"output_tokens":2,"output_tokens_details":null,"total_tokens":3}}}` + "\n")
	got := string(adaptSSEDataLine(line))
	if !strings.Contains(got, `"output_tokens_details":{"reasoning_tokens":0}`) {
		t.Fatalf("missing normalized output details: %s", got)
	}
}

func TestAdaptSSEDataLinePassesThroughUnknownEvents(t *testing.T) {
	line := []byte(`data: {"type":"response.output_text.delta","delta":"hi"}` + "\n")
	if got := string(adaptSSEDataLine(line)); got != string(line) {
		t.Fatalf("unknown event changed: %s", got)
	}
}
