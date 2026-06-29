package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Antonio7098/ultraplan-go/internal/platform/config"
	"github.com/Antonio7098/ultraplan-go/internal/study"
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

func TestStudyStatusJSONShapeAndNoRuntime(t *testing.T) {
	dir := initializedWorkspace(t)
	studyRoot := filepath.Join(dir, "studies", "platform")
	mkdirAll(t, studyRoot, "sources", "repo")
	writeFixtureFile(t, studyRoot, "dimensions", "01-structure.md")
	now := time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC)
	state := study.RunState{
		SchemaVersion: study.RunStateSchemaVersion,
		RunID:         "run-fixed",
		Study:         "platform",
		CreatedAt:     now,
		UpdatedAt:     now,
		Tasks: []study.TaskState{
			{
				ID: "a", Kind: study.TaskKindAnalysis, Status: study.TaskStatusCompleted,
				Study: "platform", Dimension: "01", DimensionRef: "01-structure", Source: "repo",
				SourceKind: study.SourceKindDirectory, OutputPath: filepath.Join(studyRoot, "reports", "source", "repo-01-structure.md"),
				CreatedAt: now, UpdatedAt: now,
				Validation: &study.ValidationSummary{Status: study.ValidationStatusPassed, CheckedAt: now, Path: filepath.Join(studyRoot, "reports", "source", "repo-01-structure.md"), PassedChecks: 3},
			},
		},
	}
	if err := study.SaveRunState(study.Study{Name: "platform", Path: studyRoot}, state); err != nil {
		t.Fatal(err)
	}
	called := false
	orig := studyRuntimeFactory
	studyRuntimeFactory = func(config.Config) (study.Runtime, error) {
		called = true
		return nil, nil
	}
	defer func() { studyRuntimeFactory = orig }()

	stdout, stderr, status := runForTest([]string{"--workspace", dir, "study", "platform", "status", "--json"})
	if status != ExitOK {
		t.Fatalf("status = %d stdout = %q stderr = %q", status, stdout, stderr)
	}
	if called {
		t.Fatalf("status initialized runtime")
	}
	var payload struct {
		SchemaVersion int    `json:"schema_version"`
		Command       string `json:"command"`
		Status        string `json:"status"`
		Result        struct {
			SchemaVersion int    `json:"schema_version"`
			RunID         string `json:"run_id"`
			StatePath     string `json:"state_path"`
			Counts        struct {
				Completed int `json:"completed"`
				Total     int `json:"total"`
			} `json:"counts"`
			Tasks []struct {
				OutputPath string `json:"output_path"`
				Usage      struct {
					Known bool `json:"known"`
				} `json:"usage"`
				Validation *study.ValidationSummary `json:"validation"`
			} `json:"tasks"`
			Usage struct {
				Known bool `json:"known"`
			} `json:"usage"`
			Cost struct {
				Known bool `json:"known"`
			} `json:"cost"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, stdout)
	}
	if payload.SchemaVersion != 1 || payload.Command != "study.status" || payload.Status != "ok" {
		t.Fatalf("unexpected envelope: %+v", payload)
	}
	if payload.Result.SchemaVersion != 1 || payload.Result.RunID != "run-fixed" || payload.Result.Counts.Total != 1 || payload.Result.Counts.Completed != 1 {
		t.Fatalf("unexpected result: %+v", payload.Result)
	}
	if payload.Result.Usage.Known || payload.Result.Cost.Known {
		t.Fatalf("usage/cost should be unknown: %+v", payload.Result)
	}
	if len(payload.Result.Tasks) != 1 || payload.Result.Tasks[0].Validation == nil {
		t.Fatalf("missing task validation: %+v", payload.Result.Tasks)
	}
	assertNotContains(t, stdout, studyRoot)
	assertNotContains(t, stdout, "\x1b[")
}

func TestStudyStatusHelp(t *testing.T) {
	dir := initializedWorkspace(t)
	mkdirAll(t, dir, "studies", "platform")

	for _, flag := range []string{"--help", "-h"} {
		t.Run(flag, func(t *testing.T) {
			stdout, stderr, status := runForTest([]string{"--workspace", dir, "study", "platform", "status", flag})
			if status != ExitOK {
				t.Fatalf("status = %d stdout = %q stderr = %q", status, stdout, stderr)
			}
			assertContains(t, stdout, "ultraplan study <study> status")
			assertContains(t, stdout, "Shows persisted run-state status")
			if stderr != "" {
				t.Fatalf("stderr = %q, want empty", stderr)
			}
		})
	}
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
	assertContains(t, stderr, filepath.Join("studies", "platform", ".ultraplan", "run-state.json"))
	assertNotContains(t, stderr, studyRoot)

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
	assertContains(t, stderr, filepath.Join("studies", "platform", ".ultraplan", "run-state.json"))
	assertNotContains(t, stderr, studyRoot)
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
	assertContains(t, stderr, filepath.Join("studies", "platform", ".ultraplan", "run-state.json"))
	assertNotContains(t, stderr, studyRoot)
	assertContains(t, stderr, "schema_version 999")
}
