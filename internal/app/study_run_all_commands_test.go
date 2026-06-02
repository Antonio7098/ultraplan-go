package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStudyRunAllCommandHelpSuccessAndSummary(t *testing.T) {
	dir, studyRoot := promptCommandFixture(t)
	fake := &commandFakeRuntime{write: validCommandSourceReport}
	restore := stubStudyRuntime(t, fake)
	defer restore()

	stdout, stderr, status := runForTest([]string{"--workspace", dir, "study", "demo", "run-all", "--help"})
	if status != ExitOK {
		t.Fatalf("status = %d stdout = %q stderr = %q", status, stdout, stderr)
	}
	assertContains(t, stdout, "--dimension <ref>")
	assertContains(t, stdout, "--source <ref>")
	assertContains(t, stdout, "--parallel <n>")

	stdout, stderr, status = runForTest([]string{"--workspace", dir, "study", "demo", "run-all", "--dimension", "01", "--source", "repo", "--parallel", "1"})
	if status != ExitOK {
		t.Fatalf("status = %d stdout = %q stderr = %q", status, stdout, stderr)
	}
	assertContains(t, stdout, "Run-all: completed")
	assertContains(t, stdout, "Completed: 2")
	assertContains(t, stdout, "Summary: summary.csv")
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	if fake.calls != 2 {
		t.Fatalf("runtime calls = %d, want analysis + synthesis", fake.calls)
	}
	if _, err := os.Stat(filepath.Join(studyRoot, "summary.csv")); err != nil {
		t.Fatal(err)
	}
}

func TestStudyRunAllCommandPreflightAndFlagErrorsStartNoRuntime(t *testing.T) {
	dir, _ := promptCommandFixture(t)
	fake := &commandFakeRuntime{write: validCommandSourceReport}
	restore := stubStudyRuntime(t, fake)
	defer restore()

	_, stderr, status := runForTest([]string{"--workspace", dir, "study", "demo", "run-all", "--parallel", "0"})
	if status != ExitUsage {
		t.Fatalf("status = %d stderr = %q", status, stderr)
	}
	if fake.calls != 0 {
		t.Fatalf("runtime calls = %d", fake.calls)
	}

	_, stderr, status = runForTest([]string{"--workspace", dir, "study", "demo", "run-all", "--dimension", "missing"})
	if status != ExitValidation {
		t.Fatalf("status = %d stderr = %q", status, stderr)
	}
	assertContains(t, stderr, "dimension reference")
	if fake.calls != 0 {
		t.Fatalf("runtime calls = %d", fake.calls)
	}
}

func TestStudyRunAllCommandPartialRuntimeFailureRedactsOutput(t *testing.T) {
	dir, _ := promptCommandFixture(t)
	fake := &commandFakeRuntime{err: os.ErrPermission}
	restore := stubStudyRuntime(t, fake)
	defer restore()

	stdout, stderr, status := runForTest([]string{"--workspace", dir, "study", "demo", "run-all", "--dimension", "01", "--source", "repo"})
	if status != ExitPartial && status != ExitRuntime {
		t.Fatalf("status = %d stdout = %q stderr = %q", status, stdout, stderr)
	}
	assertContains(t, stdout, "Run-all:")
	assertContains(t, stderr, "analysis runtime_failed")
	assertNotContains(t, stdout, "Base Prompt")
	assertNotContains(t, stderr, "Embedded Document")
}
