package sprint

import (
	"testing"

	pruntime "github.com/Antonio7098/ultraplan-go/internal/platform/runtime"
)

func TestRuntimeRequestCombinesConfiguredAndSprintProgressObservers(t *testing.T) {
	configuredCalls := 0
	var observed RuntimeProgress
	service := NewService(t.TempDir()).WithRuntime(nil, pruntime.Request{OnEvent: func(pruntime.Event) {
		configuredCalls++
	}}).WithRuntimeProgress(func(progress RuntimeProgress) {
		observed = progress
	})

	req := service.runtimeRequest("prompt", map[string]string{
		"project":  "proj",
		"sprint":   "29-release",
		"stage":    string(StageReview),
		"task":     "task-1",
		"coverage": "coverage-1",
	})
	event := pruntime.Event{Type: "progress", Kind: "progress"}
	req.OnEvent(event)

	if configuredCalls != 1 {
		t.Fatalf("configured observer calls=%d, want 1", configuredCalls)
	}
	if observed.Stage != StageReview || observed.Task != "task-1" || observed.CoverageID != "coverage-1" || observed.Event.Type != "progress" {
		t.Fatalf("sprint progress = %+v", observed)
	}
	if req.TraceID == "" || req.Metadata["trace_id"] != req.TraceID {
		t.Fatalf("trace identity not propagated: %+v", req)
	}
	if req.PromptRef.ID != "sprint.review" || req.PromptRef.Version != "1" || req.PromptRef.OwnerKind != "sprint" || req.PromptRef.OwnerID != "proj/29-release" || req.PromptRef.Purpose != "review" || req.PromptRef.Checksum == "" {
		t.Fatalf("prompt identity not propagated: %+v", req.PromptRef)
	}
	if req.Metadata["prompt_checksum"] != req.PromptRef.Checksum {
		t.Fatalf("prompt checksum metadata mismatch: %+v", req.Metadata)
	}
}
