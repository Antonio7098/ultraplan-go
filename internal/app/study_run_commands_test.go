package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"ultraplan-go/internal/platform/config"
	runtimepkg "ultraplan-go/internal/platform/runtime"
	"ultraplan-go/internal/study"
)

type commandFakeRuntime struct {
	err   error
	write string
	calls int
}

func (f *commandFakeRuntime) StartRun(ctx context.Context, req runtimepkg.Request) (runtimepkg.Result, error) {
	f.calls++
	if f.write != "" && req.Validation != nil && len(req.Validation.Expectations) > 0 {
		path := req.Validation.Expectations[0].Path
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			panic(err)
		}
		if err := os.WriteFile(path, []byte(f.write), 0o644); err != nil {
			panic(err)
		}
	}
	result := runtimepkg.Result{RunID: "fake-run", Status: "completed"}
	if f.err != nil {
		result.Status = "failed"
		result.Error = &runtimepkg.Error{Category: "runtime"}
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
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	if fake.calls != 1 {
		t.Fatalf("runtime calls = %d", fake.calls)
	}
	if _, err := os.Stat(filepath.Join(studyRoot, "reports", "source", "repo-01-structure.md")); err != nil {
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
	if err := os.Remove(filepath.Join(studyRoot, "reports", "source", "repo-01-structure.md")); err != nil {
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

func TestStudySynthesizeCommandPreflightAndValidationExit(t *testing.T) {
	dir, studyRoot := promptCommandFixture(t)
	writeFixtureFileContent(t, studyRoot, validCommandSourceReport, "reports", "source", "repo-01-structure.md")
	writeFixtureFileContent(t, studyRoot, validCommandMarkdownReport, "reports", "source", "doc.md-01-structure.md")
	fake := &commandFakeRuntime{write: validCommandFinalReport}
	restore := stubStudyRuntime(t, fake)
	defer restore()

	stdout, stderr, status := runForTest([]string{"--workspace", dir, "study", "demo", "synthesize", "01"})
	if status != ExitOK {
		t.Fatalf("status = %d stdout = %q stderr = %q", status, stdout, stderr)
	}
	assertContains(t, stdout, "Completed synthesis: demo 01-structure")
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}

	if err := os.Remove(filepath.Join(studyRoot, "reports", "source", "repo-01-structure.md")); err != nil {
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

	writeFixtureFileContent(t, studyRoot, validCommandSourceReport, "reports", "source", "repo-01-structure.md")
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
