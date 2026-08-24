package web

import (
	"testing"

	"github.com/Antonio7098/ultraplan-go/internal/app"
)

func TestRunEventViewExposesPrettyToolObservation(t *testing.T) {
	view := newRunEventView(app.RunEvent{Payload: map[string]string{
		"kind": "tool", "tool_name": "bash", "tool_call_id": "call-1", "tool_status": "completed",
		"tool_arguments": `{"command":"go test ./..."}`, "tool_result": `{"exit_code":0,"output":"ok"}`,
	}})
	if view.DetailTool != "bash" || view.ToolCallID != "call-1" || view.ToolStatus != "completed" {
		t.Fatalf("identity = %#v", view)
	}
	if view.ToolArguments != "{\n  \"command\": \"go test ./...\"\n}" || view.ToolResult == "" {
		t.Fatalf("details = %#v", view)
	}
}
