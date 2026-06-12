package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	runtimepkg "ultraplan-go/internal/platform/runtime"
	"ultraplan-go/internal/study"
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
	assertContains(t, stdout, "Run-loop: completed")
	assertContains(t, stdout, "Completed: 2")
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	loaded, err := study.LoadRunState(study.Study{Name: "demo", Path: studyRoot})
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Config.DefaultParallel != 3 || loaded.Config.Model == "" {
		t.Fatalf("config summary = %#v", loaded.Config)
	}
	if loaded.Tasks[0].Agent.Usage.TotalTokens != 42 || loaded.Tasks[0].Agent.Permissions.PolicyID != "perm-1" || !loaded.Tasks[0].Agent.Cleanup.Completed {
		t.Fatalf("agent metadata = %#v", loaded.Tasks[0].Agent)
	}
}

func TestStudyRunLoopCommandLockConflictForceUnlockAndStatusMetadata(t *testing.T) {
	dir, studyRoot := promptCommandFixture(t)
	lockPath := filepath.Join(studyRoot, ".ultraplan", "run-loop.lock")
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lockPath, []byte(`{"study":"demo","pid":123,"command":"existing","acquired_at":"2026-06-03T12:00:00Z"}`), 0o644); err != nil {
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
	assertContains(t, stdout, "Run-loop: completed")

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
	stdout, stderr, status = runForTest([]string{"--workspace", dir, "study", "demo", "status"})
	if status != ExitOK {
		t.Fatalf("status = %d stdout = %q stderr = %q", status, stdout, stderr)
	}
	assertContains(t, stdout, "Active tasks:")
	assertContains(t, stdout, "policy: final_attempt")
	assertContains(t, stdout, "omitted: events.event-1.raw")
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
