package sprint

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

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
