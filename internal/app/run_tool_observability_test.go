package app

import (
	"strings"
	"testing"

	runtimepkg "github.com/Antonio7098/ultraplan-go/internal/platform/runtime"
)

func TestRuntimeEventDraftPreservesRedactedToolArgumentsAndResult(t *testing.T) {
	draft := runtimeEventDraft(runtimepkg.Request{Metadata: map[string]string{"task": "analysis:01:source"}}, runtimepkg.Event{
		ID: "event-1", Kind: "tool", Type: "tool_use",
		Payload: map[string]any{"part": map[string]any{
			"callID": "call-1", "tool": "bash",
			"state": map[string]any{"status": "completed", "input": map[string]any{"command": "go test ./...", "api_token": "private"}, "output": "ok"},
		}},
	})
	if draft.Tool != "bash" || draft.Payload["tool_call_id"] != "call-1" || draft.Payload["tool_status"] != "completed" {
		t.Fatalf("tool identity = %#v", draft.Payload)
	}
	if !strings.Contains(draft.Payload["tool_arguments"], `"command":"go test ./..."`) || !strings.Contains(draft.Payload["tool_arguments"], `"api_token":"[REDACTED]"`) {
		t.Fatalf("arguments = %q", draft.Payload["tool_arguments"])
	}
	if draft.Payload["tool_result"] != `"ok"` {
		t.Fatalf("result = %q", draft.Payload["tool_result"])
	}
}
