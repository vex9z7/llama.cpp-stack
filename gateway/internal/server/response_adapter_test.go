package server

import (
	"bytes"
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

func TestCopyResponsesSSEInjectsFunctionCallArgumentsDone(t *testing.T) {
	stream := strings.Join([]string{
		`event: response.function_call_arguments.delta`,
		`data: {"type":"response.function_call_arguments.delta","item_id":"fc_1","output_index":0,"delta":"{\"city\":"}`,
		``,
		`event: response.function_call_arguments.delta`,
		`data: {"type":"response.function_call_arguments.delta","item_id":"fc_1","output_index":0,"delta":"\"Berlin\"}","sequence_number":2}`,
		``,
		`event: response.output_item.done`,
		`data: {"type":"response.output_item.done","output_index":0,"item":{"id":"fc_1","type":"function_call","name":"lookup_weather","arguments":"{\"city\":\"Berlin\"}"},"sequence_number":3}`,
		``,
		`event: response.completed`,
		`data: {"type":"response.completed","response":{"usage":{"input_tokens":1,"input_tokens_details":{"cached_tokens":0},"output_tokens":2,"output_tokens_details":null,"total_tokens":3}}}`,
		``,
	}, "\n")

	var out bytes.Buffer
	if err := copyResponsesSSE(&out, strings.NewReader(stream)); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "event: response.function_call_arguments.done") {
		t.Fatalf("missing arguments.done event:\n%s", got)
	}
	if !strings.Contains(got, `"type":"response.function_call_arguments.done"`) {
		t.Fatalf("missing arguments.done payload:\n%s", got)
	}
	if !strings.Contains(got, `"name":"lookup_weather"`) || !strings.Contains(got, `"arguments":"{\"city\":\"Berlin\"}"`) {
		t.Fatalf("arguments.done missing final function call data:\n%s", got)
	}
	if strings.Index(got, "response.function_call_arguments.done") > strings.Index(got, "response.output_item.done") {
		t.Fatalf("arguments.done must be emitted before output_item.done:\n%s", got)
	}
	if !strings.Contains(got, `"output_tokens_details":{"reasoning_tokens":0}`) {
		t.Fatalf("completed usage adaptation regressed:\n%s", got)
	}
}

func TestCopyResponsesSSEDoesNotDuplicateFunctionCallArgumentsDone(t *testing.T) {
	stream := strings.Join([]string{
		`event: response.function_call_arguments.done`,
		`data: {"type":"response.function_call_arguments.done","item_id":"fc_1","name":"lookup_weather","output_index":0,"arguments":"{}","sequence_number":2}`,
		``,
		`event: response.output_item.done`,
		`data: {"type":"response.output_item.done","output_index":0,"item":{"id":"fc_1","type":"function_call","name":"lookup_weather","arguments":"{}"},"sequence_number":3}`,
		``,
	}, "\n")
	var out bytes.Buffer
	if err := copyResponsesSSE(&out, strings.NewReader(stream)); err != nil {
		t.Fatal(err)
	}
	if strings.Count(out.String(), "response.function_call_arguments.done") != 2 { // event line + JSON payload
		t.Fatalf("arguments.done duplicated or lost:\n%s", out.String())
	}
}
