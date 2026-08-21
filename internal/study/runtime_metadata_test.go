package study

import (
	"strings"
	"testing"
	"time"

	runtimepkg "github.com/Antonio7098/ultraplan-go/internal/platform/runtime"
)

func TestAgentMetadataIncludesRuntimeObservability(t *testing.T) {
	started := time.Now().Add(-3 * time.Second).UTC()
	finished := time.Now().UTC()
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
		StartedAt: started, FinishedAt: finished,
		Usage: runtimepkg.Usage{TurnsKnown: true, Turns: 5, TotalTokensKnown: true, TotalTokens: 100, CacheReadTokensKnown: true, CacheReadTokens: 80},
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
	if !meta.Usage.TurnsKnown || meta.Usage.Turns != 5 || meta.DurationMS < 2900 || meta.StartedAt == nil || meta.FinishedAt == nil {
		t.Fatalf("runtime metrics=%+v", meta)
	}
}

func TestExecutionTaskErrorPreservesAttemptDetail(t *testing.T) {
	err := executionTaskError("runtime.failed", ExecutionResult{
		Status:       ExecutionStatusRuntimeFailed,
		RuntimeError: "opencode event: runtime_exit: OpenCode reported a fatal session error",
		Agent: AgentMetadata{Attempts: []AttemptMetadata{{
			Provider: "openrouter", Model: "stealth/ox-alpha", ErrorCategory: "runtime_exit", ErrorDetail: "Unexpected server error ref=err-1",
		}}},
	})
	if err.Detail != "Unexpected server error ref=err-1" || !strings.Contains(err.Message, "Unexpected server error ref=err-1") {
		t.Fatalf("task error detail not preserved: %+v", err)
	}
}
