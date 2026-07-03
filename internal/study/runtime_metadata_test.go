package study

import (
	"strings"
	"testing"

	runtimepkg "github.com/Antonio7098/ultraplan-go/internal/platform/runtime"
)

func TestAgentMetadataIncludesRuntimeObservability(t *testing.T) {
	meta := agentMetadata(runtimepkg.Result{
		RunID:  "run-1",
		Status: "completed",
		EventStats: runtimepkg.EventStats{
			Total:    205,
			Retained: 200,
			Dropped:  5,
			Limit:    200,
		},
		Memory: runtimepkg.MemoryStats{
			StartAllocBytes: 100,
			PeakAllocBytes:  500,
			EndAllocBytes:   300,
			Samples:         206,
		},
	}, runtimepkg.Request{Provider: "openrouter", Model: "model"})

	if meta.Events == nil || meta.Events.Total != 205 || meta.Events.Retained != 200 || meta.Events.Dropped != 5 || meta.Events.Limit != 200 {
		t.Fatalf("events metadata = %+v", meta.Events)
	}
	if meta.Memory == nil || meta.Memory.StartAllocBytes != 100 || meta.Memory.PeakAllocBytes != 500 || meta.Memory.EndAllocBytes != 300 || meta.Memory.Samples != 206 {
		t.Fatalf("memory metadata = %+v", meta.Memory)
	}
	if len(meta.Omissions) != 1 || meta.Omissions[0].Field != "events" || !strings.Contains(meta.Omissions[0].Reason, "5 runtime events omitted") {
		t.Fatalf("omissions = %+v", meta.Omissions)
	}
}
