package runcontrol

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"
)

func TestProcessConcurrentWritersShareOneDurableSequence(t *testing.T) {
	if os.Getenv("ULTRAPLAN_RUNCONTROL_HELPER") != "" {
		t.Skip("parent-only test")
	}
	ctx := context.Background()
	root := t.TempDir()
	repository, err := OpenSQLite(ctx, root, SQLiteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := repository.Accept(ctx, Acceptance{Target: Target{Kind: "process-test", Operation: "concurrent-write"}})
	if err != nil {
		t.Fatal(err)
	}
	owner := Owner{ID: "process-shared-owner", Process: ProcessIdentity{PID: os.Getpid()}}
	attempt, _, err := repository.Claim(ctx, Claim{RunID: snapshot.RunID, Owner: owner, Lease: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.Close(); err != nil {
		t.Fatal(err)
	}

	environment := append(os.Environ(),
		"ULTRAPLAN_RUNCONTROL_HELPER=1", "ULTRAPLAN_RUNCONTROL_ROOT="+root,
		"ULTRAPLAN_RUNCONTROL_RUN="+string(snapshot.RunID), "ULTRAPLAN_RUNCONTROL_ATTEMPT="+string(attempt.ID),
		"ULTRAPLAN_RUNCONTROL_OWNER="+owner.ID, "ULTRAPLAN_RUNCONTROL_FENCE="+strconv.FormatUint(attempt.FencingGeneration, 10),
	)
	commands := []*exec.Cmd{
		exec.Command(os.Args[0], "-test.run=^TestProcessWriterHelper$", "-test.count=1"),
		exec.Command(os.Args[0], "-test.run=^TestProcessWriterHelper$", "-test.count=1"),
	}
	outputs := make([]bytes.Buffer, len(commands))
	for index, command := range commands {
		command.Env = environment
		command.Stdout = &outputs[index]
		command.Stderr = &outputs[index]
	}
	for _, command := range commands {
		if err := command.Start(); err != nil {
			t.Fatal(err)
		}
	}
	for index, command := range commands {
		if err := command.Wait(); err != nil {
			t.Fatalf("helper: %v\n%s", err, outputs[index].String())
		}
	}

	repository, err = OpenSQLite(ctx, root, SQLiteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	events, err := repository.Events(ctx, snapshot.RunID, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 40 {
		t.Fatalf("events=%d, want 40", len(events))
	}
	for index, event := range events {
		if event.Sequence != uint64(index+1) {
			t.Fatalf("event[%d].sequence=%d", index, event.Sequence)
		}
	}
}

func TestProcessIndependentObserverPersistsCancellation(t *testing.T) {
	if os.Getenv("ULTRAPLAN_RUNCONTROL_HELPER") != "" {
		t.Skip("parent-only test")
	}
	ctx := context.Background()
	root := t.TempDir()
	repository, err := OpenSQLite(ctx, root, SQLiteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := repository.Accept(ctx, Acceptance{Target: Target{Kind: "operation", Operation: "process-cancel"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.Close(); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(os.Args[0], "-test.run=^TestProcessWriterHelper$", "-test.count=1")
	command.Env = append(os.Environ(),
		"ULTRAPLAN_RUNCONTROL_HELPER=1", "ULTRAPLAN_RUNCONTROL_ACTION=cancel",
		"ULTRAPLAN_RUNCONTROL_ROOT="+root, "ULTRAPLAN_RUNCONTROL_RUN="+string(snapshot.RunID),
	)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("cancellation helper: %v\n%s", err, output)
	}
	repository, err = OpenSQLite(ctx, root, SQLiteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	observed, err := repository.Snapshot(ctx, snapshot.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if observed.Cancellation.State != CancellationRequested || observed.Lifecycle != LifecycleCancelling || observed.LastSequence != 1 {
		t.Fatalf("cross-process cancellation snapshot = %+v", observed)
	}
	events, err := repository.Events(ctx, snapshot.RunID, 0, 10)
	if err != nil || len(events) != 1 || events[0].Type != EventCancellation {
		t.Fatalf("cross-process cancellation events=%+v err=%v", events, err)
	}
}

func TestProcessUnclaimedAcceptanceIsReconciledAfterOwnerExit(t *testing.T) {
	if os.Getenv("ULTRAPLAN_RUNCONTROL_HELPER") != "" {
		t.Skip("parent-only test")
	}
	ctx := context.Background()
	root := t.TempDir()
	command := exec.Command(os.Args[0], "-test.run=^TestProcessWriterHelper$", "-test.count=1")
	command.Env = append(os.Environ(),
		"ULTRAPLAN_RUNCONTROL_HELPER=1", "ULTRAPLAN_RUNCONTROL_ACTION=accept",
		"ULTRAPLAN_RUNCONTROL_ROOT="+root,
	)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("acceptance helper: %v\n%s", err, output)
	}
	fields := bytes.Fields(output)
	if len(fields) == 0 {
		t.Fatalf("acceptance helper returned no run ID: %q", output)
	}
	runID := RunID(fields[0])
	if err := runID.Validate(); err != nil {
		t.Fatalf("helper run ID %q: %v", output, err)
	}
	repository, err := OpenSQLite(ctx, root, SQLiteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := repository.Snapshot(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.Close(); err != nil {
		t.Fatal(err)
	}
	clock := &mutableClock{at: accepted.AcceptedAt.Add(ReconciliationGrace + time.Second)}
	repository, err = OpenSQLite(ctx, root, SQLiteOptions{Clock: clock})
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	report, err := repository.Reconcile(ctx, &staticProcessProbe{err: errors.New("unclaimed run must not use a PID probe")}, ReconcileOptions{})
	if err != nil || report.Terminal != 1 {
		t.Fatalf("unclaimed process reconciliation report=%+v err=%v", report, err)
	}
	observed, err := repository.Snapshot(ctx, runID)
	if err != nil || observed.Lifecycle != LifecycleInterrupted || observed.CurrentAttemptID != "" {
		t.Fatalf("unclaimed process snapshot=%+v err=%v", observed, err)
	}
}

func TestProcessClaimedOwnerExitIsInterruptedAndRepeatedReconciliationIsIdempotent(t *testing.T) {
	if os.Getenv("ULTRAPLAN_RUNCONTROL_HELPER") != "" {
		t.Skip("parent-only test")
	}
	ctx := context.Background()
	root := t.TempDir()
	command := exec.Command(os.Args[0], "-test.run=^TestProcessWriterHelper$", "-test.count=1")
	command.Env = append(os.Environ(),
		"ULTRAPLAN_RUNCONTROL_HELPER=1", "ULTRAPLAN_RUNCONTROL_ACTION=accept-claim",
		"ULTRAPLAN_RUNCONTROL_ROOT="+root,
	)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("claimed-owner helper: %v\n%s", err, output)
	}
	fields := bytes.Fields(output)
	if len(fields) == 0 {
		t.Fatalf("claimed-owner helper returned no run ID: %q", output)
	}
	runID := RunID(fields[0])
	repository, err := OpenSQLite(ctx, root, SQLiteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := repository.Snapshot(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	var leaseExpires string
	if err := repository.db.QueryRowContext(ctx, `SELECT lease_expires_at FROM attempts WHERE attempt_id = ?`, claimed.CurrentAttemptID).Scan(&leaseExpires); err != nil {
		t.Fatal(err)
	}
	if err := repository.Close(); err != nil {
		t.Fatal(err)
	}
	expiresAt, err := parseTime(leaseExpires)
	if err != nil {
		t.Fatal(err)
	}
	repository, err = OpenSQLite(ctx, root, SQLiteOptions{Clock: &mutableClock{at: expiresAt.Add(ReconciliationGrace + time.Second)}})
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	report, err := repository.Reconcile(ctx, NativeProcessProbe{}, ReconcileOptions{})
	if err != nil || report.Terminal != 1 || report.Scanned != 1 {
		t.Fatalf("owner-exit reconciliation report=%+v err=%v", report, err)
	}
	observed, err := repository.Snapshot(ctx, runID)
	if err != nil || observed.Lifecycle != LifecycleInterrupted || observed.Terminal == nil {
		t.Fatalf("owner-exit snapshot=%+v err=%v", observed, err)
	}
	again, err := repository.Reconcile(ctx, NativeProcessProbe{}, ReconcileOptions{})
	if err != nil || again.Scanned != 0 || again.Terminal != 0 {
		t.Fatalf("repeated reconciliation report=%+v err=%v", again, err)
	}
}

func TestProcessWriterHelper(t *testing.T) {
	if os.Getenv("ULTRAPLAN_RUNCONTROL_HELPER") == "" {
		t.Skip("helper-only test")
	}
	repository, err := OpenSQLite(context.Background(), os.Getenv("ULTRAPLAN_RUNCONTROL_ROOT"), SQLiteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	if os.Getenv("ULTRAPLAN_RUNCONTROL_ACTION") == "accept" {
		snapshot, err := repository.Accept(context.Background(), Acceptance{Target: Target{Kind: "process-test", Operation: "accept-before-claim"}})
		if err != nil {
			t.Fatal(err)
		}
		fmt.Fprintln(os.Stdout, snapshot.RunID)
		return
	}
	if os.Getenv("ULTRAPLAN_RUNCONTROL_ACTION") == "accept-claim" {
		owner, err := NewProcessOwner()
		if err != nil {
			t.Fatal(err)
		}
		snapshot, err := repository.Accept(context.Background(), Acceptance{Target: Target{Kind: "process-test", Operation: "claimed-owner-exit"}})
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := repository.Claim(context.Background(), Claim{RunID: snapshot.RunID, Owner: owner, Lease: OwnerLeaseDuration}); err != nil {
			t.Fatal(err)
		}
		fmt.Fprintln(os.Stdout, snapshot.RunID)
		return
	}
	if os.Getenv("ULTRAPLAN_RUNCONTROL_ACTION") == "cancel" {
		if _, changed, err := repository.RequestCancellation(context.Background(), RunID(os.Getenv("ULTRAPLAN_RUNCONTROL_RUN")), "user_requested"); err != nil || !changed {
			t.Fatalf("cross-process cancellation changed=%t err=%v", changed, err)
		}
		return
	}
	generation, err := strconv.ParseUint(os.Getenv("ULTRAPLAN_RUNCONTROL_FENCE"), 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	fence := Fence{RunID: RunID(os.Getenv("ULTRAPLAN_RUNCONTROL_RUN")), AttemptID: AttemptID(os.Getenv("ULTRAPLAN_RUNCONTROL_ATTEMPT")), OwnerID: os.Getenv("ULTRAPLAN_RUNCONTROL_OWNER"), FencingGeneration: generation}
	for index := 0; index < 20; index++ {
		if _, _, err := repository.Append(context.Background(), fence, EventDraft{Type: EventProgress, Payload: map[string]string{"index": fmt.Sprint(index)}}); err != nil {
			t.Fatal(err)
		}
	}
}
