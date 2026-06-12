package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"ultraplan-go/internal/study"
)

func TestStudyStatusShowsPersistedRunState(t *testing.T) {
	dir := initializedWorkspace(t)
	studyRoot := filepath.Join(dir, "studies", "platform")
	mkdirAll(t, studyRoot, "sources", "repo")
	writeFixtureFile(t, studyRoot, "dimensions", "01-structure.md")
	now := time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC)
	retry := time.Date(2026, 5, 31, 13, 0, 0, 0, time.UTC)
	state := study.RunState{
		SchemaVersion: study.RunStateSchemaVersion,
		RunID:         "run-fixed",
		Study:         "platform",
		CreatedAt:     now,
		UpdatedAt:     now,
		Tasks: []study.TaskState{
			{ID: "a", Kind: study.TaskKindAnalysis, Status: study.TaskStatusFailed, Study: "platform", Dimension: "01", Source: "repo", SourceKind: study.SourceKindDirectory, OutputPath: "out-a", CreatedAt: now, UpdatedAt: now},
			{ID: "b", Kind: study.TaskKindSynthesis, Status: study.TaskStatusRetrying, Study: "platform", OutputPath: "out-b", RetryAfter: &retry, CreatedAt: now, UpdatedAt: now},
		},
	}
	if err := study.SaveRunState(study.Study{Name: "platform", Path: studyRoot}, state); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, status := runForTest([]string{"--workspace", dir, "study", "platform", "status"})
	if status != ExitOK {
		t.Fatalf("status = %d, stdout = %q stderr = %q", status, stdout, stderr)
	}
	assertContains(t, stdout, "Run state: "+filepath.Join("studies", "platform", ".ultraplan", "run-state.json"))
	assertNotContains(t, stdout, studyRoot)
	assertContains(t, stdout, "Run ID: run-fixed")
	assertContains(t, stdout, "Complete: false")
	assertContains(t, stdout, "Tasks: 2")
	assertContains(t, stdout, "Failed: 1")
	assertContains(t, stdout, "Active: 1")
	assertContains(t, stdout, "Retries: 1")
	assertContains(t, stdout, "Next retry: 2026-05-31T13:00:00Z")
}

func TestStudyStatusMissingAndMalformedStateAreDistinct(t *testing.T) {
	dir := initializedWorkspace(t)
	studyRoot := filepath.Join(dir, "studies", "platform")
	mkdirAll(t, studyRoot, "sources", "repo")
	writeFixtureFile(t, studyRoot, "dimensions", "01-structure.md")

	_, stderr, status := runForTest([]string{"--workspace", dir, "study", "platform", "status"})
	if status != ExitValidation {
		t.Fatalf("status = %d stderr = %q", status, stderr)
	}
	assertContains(t, stderr, "run state missing")
	assertContains(t, stderr, filepath.Join(studyRoot, ".ultraplan", "run-state.json"))

	path := filepath.Join(studyRoot, ".ultraplan", "run-state.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, stderr, status = runForTest([]string{"--workspace", dir, "study", "platform", "status"})
	if status != ExitValidation {
		t.Fatalf("status = %d stderr = %q", status, stderr)
	}
	assertContains(t, stderr, "run state malformed")
	assertContains(t, stderr, path)
}

func TestStudyStatusUnsupportedStateIsDistinct(t *testing.T) {
	dir := initializedWorkspace(t)
	studyRoot := filepath.Join(dir, "studies", "platform")
	mkdirAll(t, studyRoot, "sources", "repo")
	writeFixtureFile(t, studyRoot, "dimensions", "01-structure.md")
	now := time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC)
	path := filepath.Join(studyRoot, ".ultraplan", "run-state.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(map[string]any{
		"schema_version": 999,
		"run_id":         "run-fixed",
		"study":          "platform",
		"created_at":     now,
		"updated_at":     now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		t.Fatal(err)
	}

	_, stderr, status := runForTest([]string{"--workspace", dir, "study", "platform", "status"})
	if status != ExitValidation {
		t.Fatalf("status = %d stderr = %q", status, stderr)
	}
	assertContains(t, stderr, "run state unsupported")
	assertContains(t, stderr, path)
	assertContains(t, stderr, "schema_version 999")
}
