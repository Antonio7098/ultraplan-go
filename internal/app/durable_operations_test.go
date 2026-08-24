package app

import (
	"context"
	"errors"
	"testing"

	"github.com/Antonio7098/ultraplan-go/internal/runcontrol"
)

func TestDurableOperationAcceptsBeforeExecutionRecordsEventsAndFinishes(t *testing.T) {
	ctx := context.Background()
	repository, err := runcontrol.OpenSQLite(ctx, t.TempDir(), runcontrol.SQLiteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })
	owner, err := runcontrol.NewProcessOwner()
	if err != nil {
		t.Fatal(err)
	}
	manager := newDurableOperationManager(repository, owner)
	confirmation := Confirmation{Request: OperationRequest{Kind: OperationExecuteStart, Project: "alpha", Sprint: "35", Task: "task-1"}}
	accepted, err := manager.AcceptOperation(ctx, confirmation, "confirmation-digest")
	if err != nil {
		t.Fatal(err)
	}
	runID := runcontrol.RunID(accepted.RunID)
	if err := runID.Validate(); err != nil || accepted.Existing || accepted.Context == nil {
		t.Fatalf("accepted=%+v err=%v", accepted, err)
	}
	if got := runcontrol.ParentRun(accepted.Context); got != runID {
		t.Fatalf("operation context parent = %q, want %q", got, runID)
	}
	snapshot, err := repository.Snapshot(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Lifecycle != runcontrol.LifecycleRunning || snapshot.Target.Operation != string(OperationExecuteStart) || snapshot.CurrentAttemptID == "" {
		t.Fatalf("accepted snapshot=%+v", snapshot)
	}
	committed, err := manager.RecordOperationEvent(ctx, accepted.RunID, OperationEvent{State: OperationRunning, Stage: "execute", Task: "task-1", Message: "not stored", PhaseState: "checking", SafeSummary: "Checking prerequisites", Completed: 1, Total: 2, EventType: "tool.completed", EventKind: "tool", Tool: "bash"})
	if err != nil || !committed {
		t.Fatalf("committed=%v err=%v", committed, err)
	}
	if err := manager.FinishOperation(ctx, accepted.RunID, OperationComplete, nil); err != nil {
		t.Fatal(err)
	}
	snapshot, err = repository.Snapshot(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Lifecycle != runcontrol.LifecycleSucceeded || snapshot.LastSequence != 3 {
		t.Fatalf("terminal snapshot=%+v", snapshot)
	}
	events, err := repository.Events(ctx, runID, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 || events[1].Payload["message"] != "" || events[1].Omission == nil {
		t.Fatalf("durable events=%+v", events)
	}
	if events[1].Payload["scope"] != "operation" || events[1].Payload["kind"] != "tool" || events[1].Payload["tool"] != "bash" {
		t.Fatalf("tool call details were not retained: %+v", events[1].Payload)
	}
	if events[1].Payload["phase_state"] != "checking" || events[1].Payload["summary"] != "Checking prerequisites" {
		t.Fatalf("safe phase progress was not retained: %+v", events[1].Payload)
	}
}

func TestDurableOperationDeduplicatesAcrossManagersAndFailsClosed(t *testing.T) {
	ctx := context.Background()
	repository, err := runcontrol.OpenSQLite(ctx, t.TempDir(), runcontrol.SQLiteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	owner, err := runcontrol.NewProcessOwner()
	if err != nil {
		t.Fatal(err)
	}
	manager := newDurableOperationManager(repository, owner)
	confirmation := Confirmation{Request: OperationRequest{Kind: OperationStudyStart, Study: "research"}}
	first, err := manager.AcceptOperation(ctx, confirmation, "same-confirmation-digest")
	if err != nil {
		t.Fatal(err)
	}
	other := newDurableOperationManager(repository, owner)
	second, err := other.AcceptOperation(ctx, confirmation, "same-confirmation-digest")
	if err != nil || !second.Existing || second.RunID != first.RunID {
		t.Fatalf("first=%+v second=%+v err=%v", first, second, err)
	}
	if err := manager.FinishOperation(ctx, first.RunID, OperationCancelled, context.Canceled); err != nil {
		t.Fatal(err)
	}
	if err := repository.Close(); err != nil {
		t.Fatal(err)
	}

	closed := newDurableOperationManager(repository, owner)
	if _, err := closed.AcceptOperation(ctx, confirmation, "new-digest"); err == nil || errors.Is(err, runcontrol.ErrConflict) {
		t.Fatalf("closed repository acceptance error=%v", err)
	}
}
