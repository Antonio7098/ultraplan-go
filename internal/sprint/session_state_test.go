package sprint

import (
	"context"
	"errors"
	"os"
	"testing"

	pruntime "github.com/Antonio7098/ultraplan-go/internal/platform/runtime"
)

type checkpointRuntime struct {
	calls   []pruntime.Request
	deleted []string
}

type missingSessionRuntime struct {
	calls []pruntime.Request
}

func (r *missingSessionRuntime) StartRun(_ context.Context, req pruntime.Request) (pruntime.Result, error) {
	r.calls = append(r.calls, req)
	if req.SessionID != "" {
		return pruntime.Result{Status: "failed", Error: &pruntime.Error{Category: "runtime_exit", DebugDetail: "Error: Session not found"}}, errors.New("OpenCode exited before a successful final result")
	}
	if req.OnEvent != nil {
		req.OnEvent(pruntime.Event{SessionID: "fresh-session"})
	}
	return pruntime.Result{SessionID: "fresh-session", Status: "success"}, nil
}

func (r *missingSessionRuntime) DeleteSession(context.Context, string) error { return nil }

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

func (r *checkpointRuntime) DeleteSession(_ context.Context, sessionID string) error {
	r.deleted = append(r.deleted, sessionID)
	return nil
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

func TestCompletedPlanningStageDeletesRuntimeSessionAndCheckpoint(t *testing.T) {
	root := workspaceFixture(t)
	sp := sprintFixture(t, root, "proj", "01-cleanup")
	runtime := &checkpointRuntime{}
	service := NewService(root).WithRuntime(runtime)
	state := stageSessionState{SchemaVersion: stageSessionSchemaVersion, Sessions: map[string]stageSessionRecord{
		string(StageRequirements): {SessionID: "retained-session"},
	}}
	if err := saveStageSessions(sp, state); err != nil {
		t.Fatal(err)
	}
	if err := service.cleanupPlanningStageSession(context.Background(), sp, StageRequirements, "retained-session"); err != nil {
		t.Fatal(err)
	}
	if len(runtime.deleted) != 1 || runtime.deleted[0] != "retained-session" {
		t.Fatalf("deleted sessions=%v", runtime.deleted)
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

func TestPlanningStageSessionsAreScopedByRuntimeDimensions(t *testing.T) {
	root := workspaceFixture(t)
	sp := sprintFixture(t, root, "proj", "01-area-sessions")
	runtime := &checkpointRuntime{}
	service := NewService(root).WithRuntime(runtime)
	base := pruntime.Request{Prompt: "prompt", Provider: "opencode", Model: "model", WorkDir: root}
	base.Metadata = map[string]string{"area": "API Design"}
	if _, err := service.startPlanningStageRun(context.Background(), sp, StageAreaReasoning, base); !errors.Is(err, context.Canceled) {
		t.Fatalf("first run error=%v", err)
	}
	base.Metadata = map[string]string{"area": "Architecture"}
	if _, err := service.startPlanningStageRun(context.Background(), sp, StageAreaReasoning, base); err != nil {
		t.Fatal(err)
	}
	if runtime.calls[1].SessionID != "" || runtime.calls[1].SessionAction != "" {
		t.Fatalf("different area reused session: %+v", runtime.calls[1])
	}
	state, err := loadStageSessions(sp)
	if err != nil {
		t.Fatal(err)
	}
	if state.Sessions["area-reasoning:::API Design"].SessionID != "retained-session" {
		t.Fatalf("area checkpoint missing: %+v", state.Sessions)
	}
}

func TestPlanningStageRunRestartsFreshWhenSessionIsMissing(t *testing.T) {
	root := workspaceFixture(t)
	sp := sprintFixture(t, root, "proj", "01-missing-session")
	state := stageSessionState{SchemaVersion: stageSessionSchemaVersion, Sessions: map[string]stageSessionRecord{
		string(StageRequirements): {SessionID: "stale-session", Provider: "opencode", Model: "model", WorkDir: root},
	}}
	if err := saveStageSessions(sp, state); err != nil {
		t.Fatal(err)
	}
	runtime := &missingSessionRuntime{}
	service := NewService(root).WithRuntime(runtime)
	req := pruntime.Request{Prompt: "original prompt", Provider: "opencode", Model: "model", WorkDir: root}
	result, err := service.startPlanningStageRun(context.Background(), sp, StageRequirements, req)
	if err != nil || result.SessionID != "fresh-session" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if len(runtime.calls) != 2 || runtime.calls[0].SessionAction != "continue" || runtime.calls[1].SessionID != "" || runtime.calls[1].SessionAction != "fresh" || runtime.calls[1].Prompt != "original prompt" {
		t.Fatalf("calls=%+v", runtime.calls)
	}
	state, err = loadStageSessions(sp)
	if err != nil || state.Sessions[string(StageRequirements)].SessionID != "fresh-session" {
		t.Fatalf("checkpoint=%+v err=%v", state, err)
	}
}

func TestMergeRuntimeSummaryRetainsInterruptedExecuteSession(t *testing.T) {
	previous := &ExecuteRuntimeSummary{SessionID: "execute-session", Model: "model"}
	merged := mergeRuntimeSummary(previous, &ExecuteRuntimeSummary{Model: "model"})
	if merged.SessionID != "execute-session" {
		t.Fatalf("session=%q", merged.SessionID)
	}
}
