package web

import (
	"bytes"
	"strings"
	"testing"
)

func TestSSEFrameUsesStableDecimalIDAndEventName(t *testing.T) {
	var out bytes.Buffer
	event := operationEvent{ID: 18, Name: "progress", Data: []byte(`{"operation_id":"op_example","sequence":18}`)}
	if err := writeSSEEvent(&out, event); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"id: 18\n", "event: progress\n", "data: {\"operation_id\":\"op_example\",\"sequence\":18}\n\n"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("missing %q in %q", want, out.String())
		}
	}
}

func TestEventIDValidation(t *testing.T) {
	if id, err := parseEventID("42"); err != nil || id != 42 {
		t.Fatalf("id=%d err=%v", id, err)
	}
	if _, err := parseEventID("not-decimal"); err == nil {
		t.Fatal("expected invalid event id")
	}
}
