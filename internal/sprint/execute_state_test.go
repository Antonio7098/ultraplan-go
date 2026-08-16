package sprint

import (
	"errors"
	"os"
	"path/filepath"
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
