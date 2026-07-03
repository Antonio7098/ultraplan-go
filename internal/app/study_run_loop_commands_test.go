package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	runtimepkg "github.com/Antonio7098/ultraplan-go/internal/platform/runtime"
	"github.com/Antonio7098/ultraplan-go/internal/study"
)

func TestStudyRunLoopCommandHelpInvalidFlagsAndSuccess(t *testing.T) {
	dir, studyRoot := promptCommandFixture(t)
	fake := &commandFakeRuntime{
		write: validCommandSourceReport,
		result: runtimepkg.Result{
			RunID:  "fake-run",
			Status: "completed",
			Usage:  runtimepkg.Usage{TotalTokensKnown: true, TotalTokens: 42},
			Policy: runtimepkg.PolicySummary{FinalAttempt: 1, Decisions: []runtimepkg.PolicyDecision{{
				Attempt: 1,
				Kind:    "stop",
				Reason:  "completed",
			}}},
			Permissions: runtimepkg.PermissionSummary{Mode: "restricted", PolicyID: "perm-1", Default: "ask"},
			Cleanup:     runtimepkg.CleanupSummary{Attempted: true, Completed: true},
			Repair:      runtimepkg.RepairSummary{Configured: true},
		},
	}
	restore := stubStudyRuntime(t, fake)
	defer restore()

	stdout, stderr, status := runForTest([]string{"--workspace", dir, "study", "demo", "run-loop", "--help"})
	if status != ExitOK {
		t.Fatalf("status = %d stdout = %q stderr = %q", status, stdout, stderr)
	}
	assertContains(t, stdout, "--force-unlock")
	assertContains(t, stdout, "--continue")
	assertContains(t, stdout, "--reset")
	assertContains(t, stdout, "run-state.json")

	_, stderr, status = runForTest([]string{"--workspace", dir, "study", "demo", "run-loop", "--parallel", "0"})
	if status != ExitUsage {
		t.Fatalf("status = %d stderr = %q", status, stderr)
	}
	if fake.calls != 0 {
		t.Fatalf("runtime calls = %d", fake.calls)
	}

	stdout, stderr, status = runForTest([]string{"--workspace", dir, "study", "demo", "run-loop", "--dimension", "01", "--source", "repo", "--parallel", "1"})
	if status != ExitOK {
		t.Fatalf("status = %d stdout = %q stderr = %q", status, stdout, stderr)
	}
	assertContains(t, stdout, "Study progress: completed")
	assertContains(t, stdout, "[run-loop]")
	assertContains(t, stdout, "Study progress state: "+filepath.Join("studies", "demo", ".ultraplan", "run-state.json"))
	assertContains(t, stdout, "Lock: "+filepath.Join("studies", "demo", ".ultraplan", "run-loop.lock"))
	assertNotContains(t, stdout, studyRoot)
	assertContains(t, stdout, "Study completed: 1")
	assertContains(t, stdout, "Scope completed: 1")
	assertContains(t, stderr, "analysis pending 01-structure doc.md")
	loaded, err := study.LoadRunState(study.Study{Name: "demo", Path: studyRoot})
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Config.DefaultParallel != 3 || loaded.Config.Model == "" {
		t.Fatalf("config summary = %#v", loaded.Config)
	}
	repoTask := findTaskBySource(loaded.Tasks, "repo")
	if repoTask.Agent.Usage.TotalTokens != 42 || repoTask.Agent.Permissions.PolicyID != "perm-1" || !repoTask.Agent.Cleanup.Completed {
		t.Fatalf("agent metadata = %#v", repoTask.Agent)
	}
}

func TestStudyRunLoopCommandLockConflictForceUnlockAndStatusMetadata(t *testing.T) {
	dir, studyRoot := promptCommandFixture(t)
	lockPath := filepath.Join(studyRoot, ".ultraplan", "run-loop.lock")
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		t.Fatal(err)
	}
	liveLock := fmt.Sprintf(`{"study":"demo","pid":%d,"command":"existing","acquired_at":"2026-06-03T12:00:00Z"}`, os.Getpid())
	if err := os.WriteFile(lockPath, []byte(liveLock), 0o644); err != nil {
		t.Fatal(err)
	}
	fake := &commandFakeRuntime{write: validCommandSourceReport}
	restore := stubStudyRuntime(t, fake)
	defer restore()

	_, stderr, status := runForTest([]string{"--workspace", dir, "study", "demo", "run-loop", "--dimension", "01", "--source", "repo"})
	if status != ExitPartial {
		t.Fatalf("status = %d stderr = %q", status, stderr)
	}
	assertContains(t, stderr, "study run-loop locked")
	if fake.calls != 0 {
		t.Fatalf("runtime calls = %d", fake.calls)
	}

	stdout, stderr, status := runForTest([]string{"--workspace", dir, "study", "demo", "run-loop", "--dimension", "01", "--source", "repo", "--force-unlock"})
	if status != ExitOK {
		t.Fatalf("status = %d stdout = %q stderr = %q", status, stdout, stderr)
	}
	assertContains(t, stdout, "Study progress: completed")

	state, err := study.LoadRunState(study.Study{Name: "demo", Path: studyRoot})
	if err != nil {
		t.Fatal(err)
	}
	state.Tasks[0].Status = study.TaskStatusRetrying
	retry := time.Date(2026, 6, 3, 13, 0, 0, 0, time.UTC)
	state.Tasks[0].RetryAfter = &retry
	state.Tasks[0].Agent.Policy.Decisions = []study.PolicyDecisionMetadata{{Kind: "retry", Reason: "rate limit", Delay: "1h"}}
	state.Tasks[0].Agent.Omissions = []study.MetadataOmission{{Field: "events.event-1.raw", Reason: "unsafe raw payload bytes omitted by default"}}
	if err := study.SaveRunState(study.Study{Name: "demo", Path: studyRoot}, state); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lockPath, []byte(`{"study":"demo","pid":456,"command":"ultraplan study demo run-loop --api-key=secret-value","acquired_at":"2026-06-03T12:30:00Z"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, status = runForTest([]string{"--workspace", dir, "study", "demo", "status"})
	if status != ExitOK {
		t.Fatalf("status = %d stdout = %q stderr = %q", status, stdout, stderr)
	}
	assertContains(t, stdout, "Study progress state: "+filepath.Join("studies", "demo", ".ultraplan", "run-state.json"))
	assertContains(t, stdout, "Lock: "+filepath.Join("studies", "demo", ".ultraplan", "run-loop.lock"))
	assertContains(t, stdout, "Lock command: [REDACTED]")
	assertNotContains(t, stdout, "secret-value")
	assertNotContains(t, stdout, studyRoot)
	assertContains(t, stdout, "Active tasks:")
	assertContains(t, stdout, "policy: final_attempt")
	assertContains(t, stdout, "omitted: events.event-1.raw")
}

func TestStudyRunLoopCommandConfirmsBeforeReplacingExistingState(t *testing.T) {
	dir, studyRoot := promptCommandFixture(t)
	fake := &commandFakeRuntime{write: validCommandSourceReport, result: runtimepkg.Result{RunID: "fake-run", Status: "completed"}}
	restore := stubStudyRuntime(t, fake)
	defer restore()

	stdout, stderr, status := runForTest([]string{"--workspace", dir, "study", "demo", "run-loop", "--dimension", "01", "--source", "repo", "--parallel", "1"})
	if status != ExitOK {
		t.Fatalf("initial status = %d stdout = %q stderr = %q", status, stdout, stderr)
	}
	initialCalls := fake.calls
	loaded, err := study.LoadRunState(study.Study{Name: "demo", Path: studyRoot})
	if err != nil {
		t.Fatal(err)
	}
	initialTaskCount := len(loaded.Tasks)

	stdout, stderr, status = runForTest([]string{"--workspace", dir, "study", "demo", "run-loop", "--dimension", "01", "--source", "doc.md", "--parallel", "1"})
	if status != ExitOK {
		t.Fatalf("resume status = %d stdout = %q stderr = %q", status, stdout, stderr)
	}
	assertContains(t, stdout, "Study progress: completed")
	if fake.calls != initialCalls+1 {
		t.Fatalf("runtime calls after resume = %d, want %d", fake.calls, initialCalls+1)
	}
	loaded, err = study.LoadRunState(study.Study{Name: "demo", Path: studyRoot})
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Tasks) != initialTaskCount || findTaskBySource(loaded.Tasks, "repo").Status != study.TaskStatusCompleted || findTaskBySource(loaded.Tasks, "doc.md").Status != study.TaskStatusCompleted {
		t.Fatalf("state did not resume shared progress: %#v", loaded.Tasks)
	}

	stdout, stderr, status = runForTestWithInput([]string{"--workspace", dir, "study", "demo", "run-loop", "--dimension", "01", "--source", "doc.md", "--parallel", "1", "--reset"}, nil, "no\n")
	if status != ExitPartial {
		t.Fatalf("unconfirmed reset status = %d stdout = %q stderr = %q", status, stdout, stderr)
	}
	assertContains(t, stdout, "Existing study progress is present")
	assertContains(t, stderr, "replacement not confirmed")

	stdout, stderr, status = runForTestWithInput([]string{"--workspace", dir, "study", "demo", "run-loop", "--dimension", "01", "--source", "doc.md", "--parallel", "1", "--reset"}, nil, "yes\n")
	if status != ExitOK {
		t.Fatalf("confirmed status = %d stdout = %q stderr = %q", status, stdout, stderr)
	}
	assertContains(t, stdout, "Replacing existing study progress.")
	loaded, err = study.LoadRunState(study.Study{Name: "demo", Path: studyRoot})
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Tasks) != initialTaskCount || findTaskBySource(loaded.Tasks, "doc.md").Status != study.TaskStatusCompleted || findTaskBySource(loaded.Tasks, "repo").Status != study.TaskStatusPending {
		t.Fatalf("state was not replaced after confirmation: %#v", loaded.Tasks)
	}
}

func findTaskBySource(tasks []study.TaskState, source string) study.TaskState {
	for _, task := range tasks {
		if task.Source == source {
			return task
		}
	}
	return study.TaskState{}
}

func TestStudyRunLoopCommandCancellationExit(t *testing.T) {
	dir, _ := promptCommandFixture(t)
	fake := &commandFakeRuntime{
		err:    context.Canceled,
		result: runtimepkg.Result{RunID: "cancel-run", Status: "cancelled", Error: &runtimepkg.Error{Category: "cancellation", UserDetail: "cancelled"}},
	}
	restore := stubStudyRuntime(t, fake)
	defer restore()

	stdout, stderr, status := runForTest([]string{"--workspace", dir, "study", "demo", "run-loop", "--dimension", "01", "--source", "repo"})
	if status != ExitCancel {
		t.Fatalf("status = %d stdout = %q stderr = %q", status, stdout, stderr)
	}
	assertContains(t, stderr, "cancelled")
}
