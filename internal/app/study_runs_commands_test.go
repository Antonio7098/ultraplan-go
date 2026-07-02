package app

import (
	"path/filepath"
	"testing"

	runtimepkg "github.com/Antonio7098/ultraplan-go/internal/platform/runtime"
)

func TestStudyRunsSummaryCommand(t *testing.T) {
	dir, studyRoot := promptCommandFixture(t)
	fake := &commandFakeRuntime{
		write:  validCommandSourceReport,
		result: runtimepkg.Result{RunID: "fake-run", Status: "completed"},
	}
	restore := stubStudyRuntime(t, fake)
	defer restore()

	stdout, stderr, status := runForTest([]string{"--workspace", dir, "study", "demo", "runs", "--help"})
	if status != ExitOK {
		t.Fatalf("status = %d stdout = %q stderr = %q", status, stdout, stderr)
	}
	assertContains(t, stdout, "ultraplan study <study> runs summary")

	stdout, stderr, status = runForTest([]string{"--workspace", dir, "study", "demo", "run-loop", "--dimension", "01", "--source", "repo", "--parallel", "1"})
	if status != ExitOK {
		t.Fatalf("run-loop status = %d stdout = %q stderr = %q", status, stdout, stderr)
	}

	stdout, stderr, status = runForTest([]string{"--workspace", dir, "study", "demo", "runs", "summary"})
	if status != ExitOK {
		t.Fatalf("summary status = %d stdout = %q stderr = %q", status, stdout, stderr)
	}
	assertContains(t, stdout, "Run summary: "+filepath.Join("studies", "demo", ".ultraplan", "runs", "summary.md"))
	assertContains(t, stdout, "Run ledger: "+filepath.Join("studies", "demo", ".ultraplan", "runs", "tasks.jsonl"))
	assertNotContains(t, stdout, studyRoot)
}
