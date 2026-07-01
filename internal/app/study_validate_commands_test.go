package app

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/Antonio7098/ultraplan-go/internal/platform/config"
	"github.com/Antonio7098/ultraplan-go/internal/study"
)

func TestStudyValidateTextAndJSON(t *testing.T) {
	dir := validValidationWorkspace(t)

	stdout, stderr, status := runForTest([]string{"--workspace", dir, "study", "platform", "validate"})
	if status != ExitOK {
		t.Fatalf("status = %d stdout = %q stderr = %q", status, stdout, stderr)
	}
	assertContains(t, stdout, "Validation: passed")
	assertContains(t, stdout, "Checks:")
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}

	stdout, stderr, status = runForTest([]string{"--workspace", dir, "study", "platform", "validate", "--json"})
	if status != ExitOK {
		t.Fatalf("status = %d stdout = %q stderr = %q", status, stdout, stderr)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	var payload struct {
		SchemaVersion int    `json:"schema_version"`
		Command       string `json:"command"`
		Workspace     string `json:"workspace"`
		Status        string `json:"status"`
		GeneratedAt   string `json:"generated_at"`
		Result        struct {
			SchemaVersion int    `json:"schema_version"`
			Study         string `json:"study"`
			Status        string `json:"status"`
			Summary       struct {
				Failed       int `json:"failed"`
				Inapplicable int `json:"inapplicable"`
			} `json:"summary"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, stdout)
	}
	if payload.SchemaVersion != 1 || payload.Command != "study.validate" || payload.Workspace != dir || payload.Status != "ok" || payload.GeneratedAt == "" {
		t.Fatalf("unexpected envelope: %+v", payload)
	}
	if payload.Result.SchemaVersion != 1 || payload.Result.Study != "platform" || payload.Result.Status != "passed" || payload.Result.Summary.Failed != 0 {
		t.Fatalf("unexpected result: %+v", payload.Result)
	}
	assertNotContains(t, stdout, "\x1b[")
}

func TestStudyValidateFailuresExitFiveAndRedactJSON(t *testing.T) {
	dir := initializedWorkspace(t)
	studyRoot := filepath.Join(dir, "studies", "platform")
	mkdirAll(t, studyRoot, "sources", "repo", "dimensions")
	writeFixtureFile(t, studyRoot, "dimensions", "01-structure.md")
	writeFixtureFile(t, studyRoot, "summary.csv")

	stdout, stderr, status := runForTest([]string{"--workspace", dir, "study", "platform", "validate", "--json"})
	if status != ExitValidation {
		t.Fatalf("status = %d stdout = %q stderr = %q", status, stdout, stderr)
	}
	assertContains(t, stdout, `"status": "fail"`)
	assertContains(t, stderr, "study.validate: validation failed")
	assertNotContains(t, stdout, "\x1b[")
	assertNotContains(t, stdout, "sk-test")
}

func TestStudyValidateDoesNotInitializeRuntime(t *testing.T) {
	dir := validValidationWorkspace(t)
	called := false
	orig := studyRuntimeFactory
	studyRuntimeFactory = func(config.Config) (study.Runtime, error) {
		called = true
		return nil, nil
	}
	defer func() { studyRuntimeFactory = orig }()

	_, stderr, status := runForTest([]string{"--workspace", dir, "study", "platform", "validate"})
	if status != ExitOK {
		t.Fatalf("status = %d stderr = %q", status, stderr)
	}
	if called {
		t.Fatalf("validate initialized runtime")
	}
}

func validValidationWorkspace(t *testing.T) string {
	t.Helper()
	dir := initializedWorkspace(t)
	studyRoot := filepath.Join(dir, "studies", "platform")
	mkdirAll(t, studyRoot, "sources", "repo", "dimensions", "reports", "source", "reports", "final")
	writeFixtureFile(t, studyRoot, "dimensions", "01-structure.md")
	writeFixtureFile(t, studyRoot, "summary.csv")
	writeFixtureFileContent(t, studyRoot, validSourceReport(), "reports", "source", "01-structure", "repo.md")
	writeFixtureFileContent(t, studyRoot, validFinalReport(), "reports", "final", "01-structure.md")
	return dir
}

func validSourceReport() string {
	return `# Report

## Source Information

## Summary

## Rating

Rating: 8

## Questions and Answers

- Q: one?
  A: answer

code.go:42
`
}

func validFinalReport() string {
	return `# Final Report

## Study Parameters

## Sources Studied

| Source | Path |
| --- | --- |
| repo | sources/repo |

## Executive Summary

## Rating Summary

## Pattern Synthesis

## Open Questions
`
}
