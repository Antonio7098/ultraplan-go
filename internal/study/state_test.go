package study

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewRunStateBuildsDeterministicApplicableTasks(t *testing.T) {
	root, study := testStudyRoot(t)
	now := time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC)
	dimensions := []Dimension{
		{Number: "02", Slug: "runtime", File: "02-runtime.md", Path: filepath.Join(study.Path, "dimensions", "02-runtime.md")},
		{Number: "01", Slug: "structure", File: "01-structure.md", Path: filepath.Join(study.Path, "dimensions", "01-structure.md")},
	}
	sources := []Source{
		{Name: "repo", Kind: SourceKindDirectory, Path: filepath.Join(study.Path, "sources", "repo")},
		{Name: "filtered.md", Kind: SourceKindMarkdown, Path: filepath.Join(study.Path, "sources", "filtered.md"), ApplicableDimensions: []string{"02"}},
	}

	state, err := NewRunState(NewRunStateRequest{WorkspaceRoot: root, Study: study, Sources: sources, Dimensions: dimensions, RunID: "fixed", Now: now, Config: ConfigSummary{Runtime: "opencode"}})
	if err != nil {
		t.Fatal(err)
	}
	if state.SchemaVersion != RunStateSchemaVersion || state.RunID != "fixed" || state.Config.Runtime != "opencode" {
		t.Fatalf("unexpected state header: %#v", state)
	}
	if len(state.Tasks) != 5 {
		t.Fatalf("tasks = %d, want 5: %#v", len(state.Tasks), state.Tasks)
	}
	assertTaskIDs(t, state.Tasks, []string{
		"analysis:sample:01:structure:repo:directory",
		"analysis:sample:02:runtime:filtered:markdown",
		"analysis:sample:02:runtime:repo:directory",
		"synthesis:sample:01:structure",
		"synthesis:sample:02:runtime",
	})
	for _, task := range state.Tasks {
		if filepath.IsAbs(task.OutputPath) {
			t.Fatalf("output path is absolute: %s", task.OutputPath)
		}
		if task.Kind == TaskKindAnalysis && task.Dimension == "01" && task.Source == "filtered.md" {
			t.Fatalf("inapplicable markdown source was planned: %#v", task)
		}
	}
	synthesis := findTask(t, state.Tasks, "synthesis:sample:02:runtime")
	if len(synthesis.Dependencies) != 2 {
		t.Fatalf("synthesis deps = %d, want 2", len(synthesis.Dependencies))
	}
}

func TestSaveLoadRunStateAndErrorCategories(t *testing.T) {
	root, sample := testStudyRoot(t)
	now := time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC)
	state, err := NewRunState(NewRunStateRequest{
		WorkspaceRoot: root,
		Study:         sample,
		Sources:       []Source{{Name: "repo", Kind: SourceKindDirectory}},
		Dimensions:    []Dimension{{Number: "01", Slug: "structure"}},
		RunID:         "fixed",
		Now:           now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveRunState(sample, state); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadRunState(sample)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.RunID != "fixed" || len(loaded.Tasks) != 2 {
		t.Fatalf("loaded state mismatch: %#v", loaded)
	}

	missing := Study{Name: "missing", Path: filepath.Join(root, "studies", "missing")}
	if _, err := LoadRunState(missing); !errors.Is(err, ErrRunStateMissing) {
		t.Fatalf("missing error = %v", err)
	}
	path := RunStatePath(sample)
	if err := os.WriteFile(path, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadRunState(sample); !errors.Is(err, ErrRunStateMalformed) {
		t.Fatalf("malformed error = %v", err)
	}
	raw, _ := json.Marshal(map[string]any{"schema_version": 999, "run_id": "x", "study": "sample", "created_at": now, "updated_at": now})
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadRunState(sample); !errors.Is(err, ErrRunStateUnsupported) {
		t.Fatalf("unsupported error = %v", err)
	}
}

func TestSaveRunStateRenameFailurePreservesPriorState(t *testing.T) {
	root, sample := testStudyRoot(t)
	now := time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC)
	state, err := NewRunState(NewRunStateRequest{WorkspaceRoot: root, Study: sample, RunID: "first", Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveRunState(sample, state); err != nil {
		t.Fatal(err)
	}
	state.RunID = "second"
	runStateWriteHooks.BeforeRename = func(path string) error { return errors.New("injected failure") }
	err = SaveRunState(sample, state)
	runStateWriteHooks.BeforeRename = nil
	if err == nil {
		t.Fatal("expected injected failure")
	}
	loaded, err := LoadRunState(sample)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.RunID != "first" {
		t.Fatalf("run id = %q, want first", loaded.RunID)
	}
}

func TestResumeValidateRunStateResetsAndRevalidates(t *testing.T) {
	root, sample := testStudyRoot(t)
	source := Source{Name: "repo", Kind: SourceKindDirectory, Path: filepath.Join(sample.Path, "sources", "repo")}
	dimension := Dimension{Number: "01", Slug: "structure"}
	now := time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC)
	state, err := NewRunState(NewRunStateRequest{WorkspaceRoot: root, Study: sample, Sources: []Source{source}, Dimensions: []Dimension{dimension}, RunID: "fixed", Now: now})
	if err != nil {
		t.Fatal(err)
	}
	state.Tasks[0].Status = TaskStatusRunning
	state.Tasks[0].Attempts = 2
	state.Tasks[0].LastError = &TaskError{Message: "kept"}
	state.Tasks[1].Status = TaskStatusCompleted
	writeStateTestReport(t, SourceReportPath(sample, source, dimension), true)
	ResumeValidateRunState(&state, sample, []Source{source}, []Dimension{dimension}, now.Add(time.Hour))
	if state.Tasks[0].Status != TaskStatusPending || state.Tasks[0].Attempts != 2 || state.Tasks[0].LastError.Message != "kept" {
		t.Fatalf("stale active task not preserved/reset: %#v", state.Tasks[0])
	}
	if state.Tasks[1].Status != TaskStatusFailed || state.Tasks[1].Validation == nil {
		t.Fatalf("synthesis completed task should fail missing final report validation: %#v", state.Tasks[1])
	}
}

func TestStatusSummaryCountsStatusesAndRetry(t *testing.T) {
	retry := time.Date(2026, 5, 31, 13, 0, 0, 0, time.UTC)
	state := RunState{RunID: "fixed", Tasks: []TaskState{
		{Status: TaskStatusPending},
		{Status: TaskStatusRunning},
		{Status: TaskStatusValidating},
		{Status: TaskStatusCompleted},
		{Status: TaskStatusFailed},
		{Status: TaskStatusCancelled},
		{Status: TaskStatusSkipped},
		{Status: TaskStatusWaiting},
		{Status: TaskStatusRetrying, RetryAfter: &retry},
	}}
	summary := SummarizeRunState(state, "state.json")
	if summary.Total != 9 || summary.Active != 4 || summary.RetryCount != 1 || summary.NextRetryAt == nil {
		t.Fatalf("bad summary: %#v", summary)
	}
}

func testStudyRoot(t *testing.T) (string, Study) {
	t.Helper()
	root := t.TempDir()
	path := filepath.Join(root, "studies", "sample")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	return root, Study{Name: "sample", Path: path}
}

func assertTaskIDs(t *testing.T, tasks []TaskState, want []string) {
	t.Helper()
	if len(tasks) != len(want) {
		t.Fatalf("task count = %d, want %d", len(tasks), len(want))
	}
	for i := range want {
		if tasks[i].ID != want[i] {
			t.Fatalf("task[%d] = %q, want %q", i, tasks[i].ID, want[i])
		}
	}
}

func findTask(t *testing.T, tasks []TaskState, id string) TaskState {
	t.Helper()
	for _, task := range tasks {
		if task.ID == id {
			return task
		}
	}
	t.Fatalf("task %q not found", id)
	return TaskState{}
}

func writeStateTestReport(t *testing.T, path string, source bool) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	content := "# Report\n\n## Source Info\n\n## Summary\n\n## Rating\n\nRating: 5/5\n\n## Question and Answer\n\nfile.go:1\n"
	if !source {
		content = "# Report\n\n## Study Context\n\n## Sources Studied\n\n| Source | Rating |\n| --- | --- |\n\n## Executive Summary\n\n## Rating Summary\n\n## Synthesis\n\n## Open Questions\n"
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
