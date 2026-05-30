package logging

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestLoggerRedactsTextAndJSON(t *testing.T) {
	var text bytes.Buffer
	if err := New(&text, "text", "info").Info("hello", map[string]string{"token": "secret-token"}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(text.String(), "secret-token") || !strings.Contains(text.String(), "[REDACTED]") {
		t.Fatalf("text log not redacted: %s", text.String())
	}

	var js bytes.Buffer
	if err := New(&js, "json", "info").Error("bad", map[string]string{"api_key": "secret-key"}); err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(js.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["api_key"] != "[REDACTED]" {
		t.Fatalf("json log not redacted: %+v", payload)
	}
}
