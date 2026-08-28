package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Antonio7098/ultraplan-go/internal/platform/config"
	runtimepkg "github.com/Antonio7098/ultraplan-go/internal/platform/runtime"
	"github.com/Antonio7098/ultraplan-go/internal/study"
)

type commandFakeRuntime struct {
	err    error
	write  string
	calls  int
	mutate map[string]string
	result runtimepkg.Result
}

func (f *commandFakeRuntime) StartRun(ctx context.Context, req runtimepkg.Request) (runtimepkg.Result, error) {
	f.calls++
	if req.OnEvent != nil {
		req.OnEvent(runtimepkg.Event{Type: "lifecycle.transition", Kind: "lifecycle", Payload: map[string]any{"state": "running"}})
	}
	if f.write != "" && req.Validation != nil && len(req.Validation.Expectations) > 0 {
		path := req.Validation.Expectations[0].Path
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			panic(err)
		}
		content := f.write
		if req.Metadata["task.kind"] == string(study.TaskKindSynthesis) && content == validCommandSourceReport {
			content = validCommandFinalReport
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			panic(err)
		}
	}
	for path, content := range f.mutate {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			panic(err)
		}
	}
	result := f.result
	if result.RunID == "" {
		result = runtimepkg.Result{RunID: "fake-run", Status: "completed"}
	}
	if f.err != nil {
		if result.Status == "" || result.Status == "completed" {
			result.Status = "failed"
		}
		if result.Error == nil {
			result.Error = &runtimepkg.Error{Category: "runtime"}
		}
	}
	return result, f.err
}

func TestStudyRunCommandSuccessSkipAndDiagnostics(t *testing.T) {
	dir, studyRoot := promptCommandFixture(t)
	fake := &commandFakeRuntime{write: validCommandSourceReport}
	restore := stubStudyRuntime(t, fake)
	defer restore()

	stdout, stderr, status := runForTest([]string{"--workspace", dir, "study", "demo", "run", "01", "repo"})
	if status != ExitOK {
		t.Fatalf("status = %d stdout = %q stderr = %q", status, stdout, stderr)
	}
	assertContains(t, stdout, "Completed analysis: demo 01-structure repo")
	assertContains(t, stderr, "[runtime] analysis")
	assertContains(t, stderr, "lifecycle.transition state=running")
	if fake.calls != 1 {
		t.Fatalf("runtime calls = %d", fake.calls)
	}
	if _, err := os.Stat(filepath.Join(studyRoot, "reports", "source", "01-structure", "repo.md")); err != nil {
		t.Fatal(err)
	}

	fake.calls = 0
	stdout, stderr, status = runForTest([]string{"--workspace", dir, "study", "demo", "run", "01", "other.md"})
	if status != ExitOK {
		t.Fatalf("skip status = %d stdout = %q stderr = %q", status, stdout, stderr)
	}
	assertContains(t, stdout, "Skipped analysis:")
	if fake.calls != 0 {
		t.Fatalf("skip runtime calls = %d", fake.calls)
	}

	fake.err = errors.New("provider secret should be redacted")
	fake.write = ""
	if err := os.Remove(filepath.Join(studyRoot, "reports", "source", "01-structure", "repo.md")); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, status = runForTest([]string{"--workspace", dir, "study", "demo", "run", "01", "repo"})
	if status != ExitRuntime {
		t.Fatalf("runtime status = %d stdout = %q stderr = %q", status, stdout, stderr)
	}
	assertContains(t, stderr, "Runtime failed")
	assertNotContains(t, stdout, "Base Prompt")
	assertNotContains(t, stderr, "Embedded Document")
}

func TestStudyRunCommandWarnsForUnexpectedRuntimeEdits(t *testing.T) {
	dir, studyRoot := promptCommandFixture(t)
	sourcePath := filepath.Join(studyRoot, "sources", "repo", "main.go")
	writeFixtureFileContent(t, studyRoot, "package main\n", "sources", "repo", "main.go")
	fake := &commandFakeRuntime{
		write:  validCommandSourceReport,
		mutate: map[string]string{sourcePath: "package main\n\n// unexpected edit\n"},
	}
	restore := stubStudyRuntime(t, fake)
	defer restore()

	stdout, stderr, status := runForTest([]string{"--workspace", dir, "study", "demo", "run", "01", "repo"})
	if status != ExitOK {
		t.Fatalf("status = %d stdout = %q stderr = %q", status, stdout, stderr)
	}
	assertContains(t, stdout, "Completed analysis: demo 01-structure repo")
	assertContains(t, stderr, "Warning: unexpected edit outside allowed paths: modified sources/repo/main.go")
}

func TestStudySynthesizeCommandPreflightAndValidationExit(t *testing.T) {
	dir, studyRoot := promptCommandFixture(t)
	writeFixtureFileContent(t, studyRoot, validCommandSourceReport, "reports", "source", "01-structure", "repo.md")
	writeFixtureFileContent(t, studyRoot, validCommandMarkdownReport, "reports", "source", "01-structure", "doc.md")
	fake := &commandFakeRuntime{write: validCommandFinalReport}
	restore := stubStudyRuntime(t, fake)
	defer restore()

	stdout, stderr, status := runForTest([]string{"--workspace", dir, "study", "demo", "synthesize", "01"})
	if status != ExitOK {
		t.Fatalf("status = %d stdout = %q stderr = %q", status, stdout, stderr)
	}
	assertContains(t, stdout, "Completed synthesis: demo 01-structure")
	assertContains(t, stderr, "[runtime] synthesis")
	assertContains(t, stderr, "lifecycle.transition state=running")

	if err := os.Remove(filepath.Join(studyRoot, "reports", "source", "01-structure", "repo.md")); err != nil {
		t.Fatal(err)
	}
	fake.calls = 0
	stdout, stderr, status = runForTest([]string{"--workspace", dir, "study", "demo", "synthesize", "01"})
	if status != ExitValidation {
		t.Fatalf("preflight status = %d stdout = %q stderr = %q", status, stdout, stderr)
	}
	assertContains(t, stderr, "Synthesis blocked")
	if fake.calls != 0 {
		t.Fatalf("preflight runtime calls = %d", fake.calls)
	}

	writeFixtureFileContent(t, studyRoot, validCommandSourceReport, "reports", "source", "01-structure", "repo.md")
	fake.write = "# Invalid final\n"
	stdout, stderr, status = runForTest([]string{"--workspace", dir, "study", "demo", "synthesize", "01"})
	if status != ExitValidation {
		t.Fatalf("validation status = %d stdout = %q stderr = %q", status, stdout, stderr)
	}
	assertContains(t, stderr, "Validation failed:")
	assertContains(t, stderr, "section.sources_table")
}

func TestStudyRunCommandUsage(t *testing.T) {
	dir, _ := promptCommandFixture(t)
	stdout, stderr, status := runForTest([]string{"--workspace", dir, "study", "demo", "run", "01"})
	if status != ExitUsage {
		t.Fatalf("status = %d stdout = %q stderr = %q", status, stdout, stderr)
	}
	assertContains(t, stderr, "requires <dimension> <source>")

	stdout, stderr, status = runForTest([]string{"--workspace", dir, "study", "demo", "run", "--help"})
	if status != ExitOK {
		t.Fatalf("help status = %d stdout = %q stderr = %q", status, stdout, stderr)
	}
	assertContains(t, stdout, "ultraplan study <study> run <dimension> <source>")
}

func TestRuntimeProgressSummaryIsUsefulAndOmitsMessages(t *testing.T) {
	event := runtimepkg.Event{Type: "tool.completed", Kind: "tool", Payload: map[string]any{"tool": "read", "status": "completed"}}
	if !runtimeEventIsProgress(event) {
		t.Fatal("tool event should be progress")
	}
	summary := runtimeProgressSummary(event)
	for _, want := range []string{"tool.completed", "status=completed", "tool=read"} {
		assertContains(t, summary, want)
	}
	if runtimeEventIsProgress(runtimepkg.Event{Type: "message.part", Kind: "message"}) {
		t.Fatal("message bodies must not be streamed as progress")
	}
	leaky := runtimeProgressSummary(runtimepkg.Event{Type: "opencode.log.rate_limit", Kind: "warning", Payload: map[string]any{"detail": "rate limited\nunrelated session log\x00tail"}})
	if strings.ContainsAny(leaky, "\r\n\x00") || strings.Contains(leaky, "  ") {
		t.Fatalf("multiline progress was not bounded to one line: %q", leaky)
	}
}

func stubStudyRuntime(t *testing.T, rt study.Runtime) func() {
	t.Helper()
	old := studyRuntimeFactory
	studyRuntimeFactory = func(config.Config) (study.Runtime, error) {
		return rt, nil
	}
	return func() { studyRuntimeFactory = old }
}

const validCommandSourceReport = `# Report

## Source Information

## Summary

## Rating

Rating: 8

## Questions and Answers

- Q: one?
  A: answer

code.go:42
`

const validCommandMarkdownReport = `# Report

## Source Information

## Summary

## Rating

Rating: 8

## Questions and Answers

- Q: one?
  A: answer
`

const validCommandFinalReport = `# Final Report

## Study Parameters

## Sources Studied

| Source | Path |
| --- | --- |
| repo | source |

## Executive Summary

## Rating Summary

## Pattern Synthesis

## Open Questions
`
