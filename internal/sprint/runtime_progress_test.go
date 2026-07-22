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
}
