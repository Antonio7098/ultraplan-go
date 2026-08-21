package sprint

import (
	"context"
	"errors"
	"os"
	"testing"

	pruntime "github.com/Antonio7098/ultraplan-go/internal/platform/runtime"
)

type checkpointRuntime struct {
	calls []pruntime.Request
}

func (r *checkpointRuntime) StartRun(_ context.Context, req pruntime.Request) (pruntime.Result, error) {
	r.calls = append(r.calls, req)
	if len(r.calls) == 1 {
		if req.OnEvent != nil {
			req.OnEvent(pruntime.Event{SessionID: "retained-session"})
		}
		return pruntime.Result{}, context.Canceled
	}
	return pruntime.Result{SessionID: "retained-session", Status: "success"}, nil
}

func TestPlanningStageRunContinuesCheckpointedSession(t *testing.T) {
	root := workspaceFixture(t)
	sp := sprintFixture(t, root, "proj", "01-session")
	runtime := &checkpointRuntime{}
	service := NewService(root).WithRuntime(runtime)
	req := pruntime.Request{Prompt: "first prompt", Provider: "opencode", Model: "model", WorkDir: root, PromptRef: pruntime.PromptReference{Checksum: "one"}}

	if _, err := service.startPlanningStageRun(context.Background(), sp, StageRequirements, req); !errors.Is(err, context.Canceled) {
		t.Fatalf("first run error=%v", err)
	}
	state, err := loadStageSessions(sp)
	if err != nil || state.Sessions[string(StageRequirements)].SessionID != "retained-session" {
		t.Fatalf("checkpoint=%+v err=%v", state, err)
	}

	req.Prompt = "refreshed prompt"
	req.PromptRef.Checksum = "one"
	if _, err := service.startPlanningStageRun(context.Background(), sp, StageRequirements, req); err != nil {
		t.Fatal(err)
	}
	if len(runtime.calls) != 2 || runtime.calls[1].SessionID != "retained-session" || runtime.calls[1].SessionAction != "continue" {
		t.Fatalf("continuation request=%+v", runtime.calls)
	}
	if err := clearPlanningStageSession(sp, StageRequirements); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stageSessionPath(sp)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("session checkpoint was not cleared: %v", err)
	}
}

func TestPlanningStageRunToleratesPromptChangesWithoutExactMatchGate(t *testing.T) {
	root := workspaceFixture(t)
	sp := sprintFixture(t, root, "proj", "01-session-change")
	runtime := &checkpointRuntime{}
	service := NewService(root).WithRuntime(runtime)
	req := pruntime.Request{Prompt: "first prompt", Provider: "opencode", Model: "model", WorkDir: root, PromptRef: pruntime.PromptReference{Checksum: "one"}}
	if _, err := service.startPlanningStageRun(context.Background(), sp, StageRequirements, req); !errors.Is(err, context.Canceled) {
		t.Fatalf("first run error=%v", err)
	}
	req.Prompt, req.PromptRef.Checksum = "changed prompt", "two"
	if _, err := service.startPlanningStageRun(context.Background(), sp, StageRequirements, req); err != nil {
		t.Fatal(err)
	}
	if runtime.calls[1].SessionID != "retained-session" || runtime.calls[1].SessionAction != "continue" {
		t.Fatalf("changed prompt did not reuse compatible interrupted session: %+v", runtime.calls[1])
	}
}

func TestMergeRuntimeSummaryRetainsInterruptedExecuteSession(t *testing.T) {
	previous := &ExecuteRuntimeSummary{SessionID: "execute-session", Model: "model"}
	merged := mergeRuntimeSummary(previous, &ExecuteRuntimeSummary{Model: "model"})
	if merged.SessionID != "execute-session" {
		t.Fatalf("session=%q", merged.SessionID)
	}
}
