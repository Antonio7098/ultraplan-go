package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Antonio7098/ultraplan-go/internal/runcontrol"
)

func TestRunCommandsListShowFollowCancelDiagnosticsAndSupport(t *testing.T) {
	root := initializedWorkspace(t)
	runID := seedCLIActiveRun(t, root)

	stdout, stderr, status := runForTest([]string{"--workspace", root, "run", "list", "--project", "alpha"})
	if status != ExitOK || stderr != "" {
		t.Fatalf("list status=%d stderr=%q", status, stderr)
	}
	assertContains(t, stdout, string(runID))
	assertContains(t, stdout, "running")

	stdout, stderr, status = runForTest([]string{"--workspace", root, "run", "show", string(runID), "--json"})
	if status != ExitOK || stderr != "" {
		t.Fatalf("show status=%d stderr=%q", status, stderr)
	}
	var envelope map[string]any
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatalf("show JSON: %v\n%s", err, stdout)
	}
	assertContains(t, stdout, `"lifecycle": "running"`)

	stdout, stderr, status = runForTest([]string{"--workspace", root, "run", "cancel", string(runID), "--reason", "operator_requested", "--json"})
	if status != ExitOK || stderr != "" {
		t.Fatalf("cancel status=%d stderr=%q", status, stderr)
	}
	assertContains(t, stdout, `"state": "requested"`)

	support := filepath.Join(t.TempDir(), "support.json")
	stdout, stderr, status = runForTest([]string{"--workspace", root, "run", "diagnostics", "--support-export", support, "--json"})
	if status != ExitOK || stderr != "" {
		t.Fatalf("diagnostics status=%d stderr=%q", status, stderr)
	}
	info, err := os.Stat(support)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 || info.Size() <= 0 || info.Size() > 1<<20 {
		t.Fatalf("support mode=%04o size=%d", info.Mode().Perm(), info.Size())
	}
	data, err := os.ReadFile(support)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "committed safe summary") || strings.Contains(string(data), root) {
		t.Fatalf("support export leaked event content or workspace path: %s", data)
	}
	for _, want := range []string{`"config"`, `"full_history_source": "default"`, `"workspace_quota_source": "default"`, `"reconciliation"`, `"logs"`, `"run reconciliation completed"`} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("support export missing %s: %s", want, data)
		}
	}
}

func TestRunFollowReplaysTerminalJournalAndStopsWithoutCancelling(t *testing.T) {
	root := initializedWorkspace(t)
	runID := seedCLITerminalRun(t, root)
	stdout, stderr, status := runForTest([]string{"--workspace", root, "run", "follow", string(runID), "--after", "0", "--json"})
	if status != ExitOK || stderr != "" {
		t.Fatalf("follow status=%d stderr=%q", status, stderr)
	}
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if len(lines) != 2 {
		t.Fatalf("follow lines=%d output=%q", len(lines), stdout)
	}
	assertContains(t, lines[0], `"sequence":1`)
	assertContains(t, lines[1], `"sequence":2`)
}

func TestRunHelpDoesNotOpenRepository(t *testing.T) {
	root := initializedWorkspace(t)
	stdout, stderr, status := runForTest([]string{"--workspace", root, "run", "--help"})
	if status != ExitOK || stderr != "" || !strings.Contains(stdout, "List newest durable workspace runs") {
		t.Fatalf("status=%d stdout=%q stderr=%q", status, stdout, stderr)
	}
	if _, err := os.Stat(filepath.Join(root, runcontrol.DatabaseRelativePath)); !os.IsNotExist(err) {
		t.Fatalf("help opened run-control repository: %v", err)
	}
}

func seedCLIActiveRun(t *testing.T, root string) runcontrol.RunID {
	t.Helper()
	repository, runID, _, _ := seedCLIRun(t, root)
	if err := repository.Close(); err != nil {
		t.Fatal(err)
	}
	return runID
}

func seedCLITerminalRun(t *testing.T, root string) runcontrol.RunID {
	t.Helper()
	repository, runID, fence, _ := seedCLIRun(t, root)
	if _, _, err := repository.ProposeTerminal(context.Background(), fence, runcontrol.TerminalProposal{Outcome: runcontrol.TerminalSucceeded, Reason: "completed", ProposedBy: "test"}); err != nil {
		t.Fatal(err)
	}
	if err := repository.Close(); err != nil {
		t.Fatal(err)
	}
	return runID
}

func seedCLIRun(t *testing.T, root string) (*runcontrol.SQLiteRepository, runcontrol.RunID, runcontrol.Fence, runcontrol.Owner) {
	t.Helper()
	ctx := context.Background()
	repository, err := runcontrol.OpenSQLite(ctx, root, runcontrol.SQLiteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := repository.Accept(ctx, runcontrol.Acceptance{Target: runcontrol.Target{Kind: "sprint", Operation: "execute", Project: "alpha", Sprint: "35"}, ProductStatus: "task_running"})
	if err != nil {
		t.Fatal(err)
	}
	owner := runcontrol.Owner{ID: "cli-test-owner", Process: runcontrol.ProcessIdentity{PID: os.Getpid()}}
	attempt, _, err := repository.Claim(ctx, runcontrol.Claim{RunID: snapshot.RunID, Owner: owner, Lease: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	fence := runcontrol.Fence{RunID: snapshot.RunID, AttemptID: attempt.ID, OwnerID: owner.ID, FencingGeneration: attempt.FencingGeneration}
	if _, _, err := repository.Append(ctx, fence, runcontrol.EventDraft{Type: runcontrol.EventMessage, Payload: map[string]string{"message": "committed safe summary"}}); err != nil {
		t.Fatal(err)
	}
	return repository, snapshot.RunID, fence, owner
}
