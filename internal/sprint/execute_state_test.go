package sprint

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	pruntime "github.com/Antonio7098/ultraplan-go/internal/platform/runtime"
)

type recordingExecuteRuntime struct {
	calls int
}

func (r *recordingExecuteRuntime) StartRun(_ context.Context, _ pruntime.Request) (pruntime.Result, error) {
	r.calls++
	return pruntime.Result{RunID: "execute-run", Status: "success", Artifacts: []pruntime.Artifact{{ID: "evidence", Kind: "test", Description: "verified"}}}, nil
}

type checkpointSaveFailureRuntime struct {
	statePath string
}

func (r checkpointSaveFailureRuntime) StartRun(_ context.Context, req pruntime.Request) (pruntime.Result, error) {
	if err := os.Remove(r.statePath); err != nil {
		return pruntime.Result{}, err
	}
	if err := os.Mkdir(r.statePath, 0o755); err != nil {
		return pruntime.Result{}, err
	}
	if req.OnEvent != nil {
		req.OnEvent(pruntime.Event{SessionID: "execute-session"})
	}
	if err := os.Remove(r.statePath); err != nil {
		return pruntime.Result{}, err
	}
	return pruntime.Result{RunID: "execute-run", SessionID: "execute-session", Status: "success", Artifacts: []pruntime.Artifact{{ID: "evidence", Kind: "test", Description: "verified"}}}, nil
}

func TestExecuteRunStateStrictLoadingAndAtomicWritePreservesPrior(t *testing.T) {
	root := workspaceFixture(t)
	sp := sprintFixture(t, root, "proj", "01-alpha")
	now := time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC)
	state := validExecuteRunState(sp, now)
	if err := SaveExecuteRunState(root, sp, state); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadExecuteRunState(root, sp)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.SchemaVersion != ExecuteRunStateSchemaVersion || loaded.Tasks[0].Status != ExecuteTaskPending {
		t.Fatalf("loaded = %+v", loaded)
	}

	path, err := ExecuteRunStatePath(root, sp)
	if err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	bad := state
	bad.Tasks[0].Status = "done"
	writeJSON(t, path, bad)
	if _, err := LoadExecuteRunState(root, sp); !errors.Is(err, ErrExecuteRunStateMalformed) {
		t.Fatalf("unsupported status err = %v", err)
	}
	writeFileContent(t, filepath.Dir(path), string(original), filepath.Base(path))

	err = saveExecuteRunStateWithHooks(root, sp, state, atomicWriteHooks{BeforeRename: func(string) error {
		return errors.New("rename blocked")
	}})
	if err == nil {
		t.Fatalf("expected write failure")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(original) {
		t.Fatalf("prior state was not preserved")
	}
}

func TestExecuteRunStateValidationFailures(t *testing.T) {
	root := workspaceFixture(t)
	sp := sprintFixture(t, root, "proj", "01-alpha")
	now := time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC)
	path, err := ExecuteRunStatePath(root, sp)
	if err != nil {
		t.Fatal(err)
	}

	cases := map[string]func(ExecuteRunState) ExecuteRunState{
		"missing schema": func(s ExecuteRunState) ExecuteRunState {
			s.SchemaVersion = 0
			return s
		},
		"unsupported schema": func(s ExecuteRunState) ExecuteRunState {
			s.SchemaVersion = 99
			return s
		},
		"project mismatch": func(s ExecuteRunState) ExecuteRunState {
			s.Project = "other"
			return s
		},
		"unsafe plan path": func(s ExecuteRunState) ExecuteRunState {
			s.PlanPath = "../plan.md"
			return s
		},
		"missing tasks": func(s ExecuteRunState) ExecuteRunState {
			s.Tasks = nil
			return s
		},
		"duplicate task id": func(s ExecuteRunState) ExecuteRunState {
			s.Tasks = append(s.Tasks, s.Tasks[0])
			return s
		},
		"missing required task fields": func(s ExecuteRunState) ExecuteRunState {
			s.Tasks[0].Identity.Name = ""
			return s
		},
		"negative attempts": func(s ExecuteRunState) ExecuteRunState {
			s.Tasks[0].Attempts = -1
			return s
		},
		"running without startedAt": func(s ExecuteRunState) ExecuteRunState {
			s.Tasks[0].Status = ExecuteTaskRunning
			s.Tasks[0].StartedAt = nil
			return s
		},
		"terminal without completedAt": func(s ExecuteRunState) ExecuteRunState {
			s.Tasks[0].Status = ExecuteTaskComplete
			s.Tasks[0].CompletedAt = nil
			return s
		},
		"unsafe diagnostic": func(s ExecuteRunState) ExecuteRunState {
			s.Tasks[0].Diagnostics = []ExecuteDiagnostic{{Code: "runtime\nfailed", Message: "bad", At: now}}
			return s
		},
		"unsafe evidence path": func(s ExecuteRunState) ExecuteRunState {
			s.Tasks[0].Evidence = []ExecuteEvidence{{Kind: "file", Summary: "created", Path: "../outside"}}
			return s
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			err := ValidateExecuteRunState(root, sp, mutate(validExecuteRunState(sp, now)), path)
			if err == nil {
				t.Fatalf("expected validation error")
			}
		})
	}
}

func TestExecuteRunStateLoadMissingAndMalformed(t *testing.T) {
	root := workspaceFixture(t)
	sp := sprintFixture(t, root, "proj", "01-alpha")
	if _, err := LoadExecuteRunState(root, sp); !errors.Is(err, ErrExecuteRunStateMissing) {
		t.Fatalf("missing err = %v", err)
	}
	writeFileContent(t, sp.Path, "{not json", ".run-state.json")
	if _, err := LoadExecuteRunState(root, sp); !errors.Is(err, ErrExecuteRunStateMalformed) {
		t.Fatalf("malformed err = %v", err)
	}
}

func TestLegacyTerminalExecuteStatusPreservesHistoricalCompletion(t *testing.T) {
	root := workspaceFixture(t)
	sp := sprintFixture(t, root, "proj", "01-alpha")
	writeFileContent(t, sp.Path, `{"status":"complete","completedAt":"2026-05-30T10:07:22Z","files":[],"testsRun":[],"blockers":[]}`, ".run-state.json")
	status, ok := LegacyTerminalExecuteStatus(root, sp)
	if !ok || status != "complete" {
		t.Fatalf("legacy status = %q, %t", status, ok)
	}
	if _, err := LoadExecuteRunState(root, sp); !errors.Is(err, ErrExecuteRunStateMalformed) {
		t.Fatalf("legacy state unexpectedly became resumable: %v", err)
	}
}

func TestDeferredExecuteTaskRequiresRationaleAndIsResolved(t *testing.T) {
	now := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	sp := Sprint{Project: "proj", Slug: "01-alpha", Path: "/workspace/projects/proj/sprints/01-alpha"}
	state := validExecuteRunState(sp, now)
	state.Tasks[0].Status = ExecuteTaskDeferred
	state.Tasks[0].CompletedAt = &now
	if err := ValidateExecuteRunState("/workspace", sp, state, "state.json"); err == nil {
		t.Fatal("deferred task without rationale passed validation")
	}
	state.Tasks[0].Diagnostics = []ExecuteDiagnostic{{Code: "deferred", Message: "Accepted follow-up work", At: now}}
	if err := ValidateExecuteRunState("/workspace", sp, state, "state.json"); err != nil {
		t.Fatalf("deferred task with rationale failed validation: %v", err)
	}
	if hasFailedExecuteTask(state.Tasks) {
		t.Fatal("deferred task was treated as failed")
	}
}

func TestDeferExecuteTaskPersistsRationaleAndSummary(t *testing.T) {
	root := workspaceFixture(t)
	sp := sprintFixture(t, root, "proj", "01-alpha")
	writeFixtureProjectIndex(t, root, "proj")
	now := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	if err := SaveExecuteRunState(root, sp, validExecuteRunState(sp, now)); err != nil {
		t.Fatal(err)
	}
	result, err := NewService(root).DeferExecuteTask(context.Background(), "proj", "01", "task-abc123", "Accepted for Sprint 32")
	if err != nil {
		t.Fatal(err)
	}
	state, err := LoadExecuteRunState(root, sp)
	if err != nil {
		t.Fatal(err)
	}
	if state.Tasks[0].Status != ExecuteTaskDeferred || state.Tasks[0].Diagnostics[len(state.Tasks[0].Diagnostics)-1].Message != "Accepted for Sprint 32" {
		t.Fatalf("state = %+v", state.Tasks[0])
	}
	if result.Message != "execute task deferred" {
		t.Fatalf("result = %+v", result)
	}
	data, err := os.ReadFile(filepath.Join(sp.Path, "execute.md"))
	if err != nil || !strings.Contains(string(data), "deferred") || !strings.Contains(string(data), "Accepted for Sprint 32") {
		t.Fatalf("summary=%q err=%v", data, err)
	}
}

func TestExecuteFailsBeforeRuntimeWhenRunningStateCannotBePersisted(t *testing.T) {
	runtime := &recordingExecuteRuntime{}
	root, sp, service := executePersistenceFixture(t, runtime)
	statePath, err := ExecuteRunStatePath(root, sp)
	if err != nil {
		t.Fatal(err)
	}
	clockCalls := 0
	var sabotageErr error
	service = service.WithClock(func() time.Time {
		clockCalls++
		if clockCalls == 3 {
			sabotageErr = os.Remove(statePath)
			if sabotageErr == nil {
				sabotageErr = os.Mkdir(statePath, 0o755)
			}
		}
		return time.Date(2026, 8, 21, 12, 0, clockCalls, 0, time.UTC)
	})
	_, err = service.Execute(context.Background(), "proj", "01", ExecuteRequest{})
	if sabotageErr != nil {
		t.Fatalf("test setup failed: %v", sabotageErr)
	}
	if err == nil || !strings.Contains(err.Error(), "persist running execute task") {
		t.Fatalf("running-state persistence err=%v", err)
	}
	if runtime.calls != 0 {
		t.Fatalf("runtime called after running-state persistence failed: %d", runtime.calls)
	}
}

func TestExecuteFailsAndRecordsFailedTaskWhenSessionCheckpointCannotBePersisted(t *testing.T) {
	root, sp, _ := executePersistenceFixture(t, &recordingExecuteRuntime{})
	statePath, err := ExecuteRunStatePath(root, sp)
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(root).WithRuntime(checkpointSaveFailureRuntime{statePath: statePath}).WithStageRuntime(map[PlanningStage]StageRuntime{StageExecute: {Model: "test/model"}})
	_, err = service.Execute(context.Background(), "proj", "01", ExecuteRequest{})
	if err == nil || !strings.Contains(err.Error(), "persist runtime session") {
		t.Fatalf("session checkpoint persistence err=%v", err)
	}
	state, loadErr := LoadExecuteRunState(root, sp)
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if state.Tasks[0].Status != ExecuteTaskFailed || len(state.Tasks[0].Diagnostics) == 0 || state.Tasks[0].Diagnostics[len(state.Tasks[0].Diagnostics)-1].Code != "state-save-failed" {
		t.Fatalf("checkpoint failure was not persisted as a failed task: %+v", state.Tasks[0])
	}
}

func executePersistenceFixture(t *testing.T, runtime Runtime) (string, Sprint, Service) {
	t.Helper()
	root := workspaceFixture(t)
	sp := sprintFixture(t, root, "proj", "01-alpha")
	writeFixtureProjectIndex(t, root, "proj")
	writeFileContent(t, root, "# Architecture Template\n", "system", "reasoning", "architecture_reasoning_template.md")
	writeEvidenceFile(t, root)
	writeFileContent(t, sp.Path, "# Requirements\n\nExecute persistence.\n", "requirements.md")
	writeCompletedCodeContext(t, root, sp)
	writeFileContent(t, sp.Path, validSprintIndex(), "sprint-index.md")
	writeFileContent(t, sp.Path, validReasoningTechnicalHandbook(), "technical-handbook.md")
	writeFileContent(t, sp.Path, validAreaReasoning(), "reasoning", "architecture.md")
	writeFileContent(t, sp.Path, validPlanFinalReasoning(), "reasoning.md")
	writeFileContent(t, sp.Path, validPlan(), "plan.md")
	service := NewService(root).WithRuntime(runtime).WithStageRuntime(map[PlanningStage]StageRuntime{StageExecute: {Model: "test/model"}})
	return root, sp, service
}

func validExecuteRunState(sp Sprint, now time.Time) ExecuteRunState {
	return NewExecuteRunState(
		sp,
		ExecuteTargetRef{Path: "/home/antonioborgerees/coding/ultraplan/ultraplan-go", Source: "project-index.md"},
		ArtifactRelPath(sp, StagePlan),
		"sha256:abc123",
		[]ExecuteTaskRecord{{
			ID:        "task-abc123",
			Identity:  ExecuteTaskIdentity{Name: "Task 1: Add execute state", PlanLine: 42, Decisions: []string{"Decision 3"}, Requirements: []string{"REQ-23-46"}},
			Status:    ExecuteTaskPending,
			Attempts:  0,
			CreatedAt: now,
			UpdatedAt: now,
		}},
		now,
	)
}
