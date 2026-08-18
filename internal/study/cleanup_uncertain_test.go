package study

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"
)

func TestCleanupUncertaintyIsDurableAndReconciliationConsumesIt(t *testing.T) {
	root, st := executionFixture(t)
	listing, err := NewService(root).ListStudy(st.Name)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	state, err := NewRunState(NewRunStateRequest{
		WorkspaceRoot: root,
		Study:         listing.Study,
		Sources:       listing.Sources,
		Dimensions:    listing.Dimensions,
		RunID:         "run-cleanup",
		Now:           now,
	})
	if err != nil {
		t.Fatal(err)
	}
	state.Tasks[0].Status = TaskStatusRunning
	state.Tasks[0].StartedAt = &now
	if err := SaveRunState(st, state); err != nil {
		t.Fatal(err)
	}
	service := NewService(root)
	if err := service.RecordCleanupUncertain(context.Background(), st.Name, CleanupUncertainRecord{
		OperationID: "op-study-deadline",
		Kind:        "study-start",
		Reason:      "server_shutdown",
		RecordedAt:  now,
	}); err != nil {
		t.Fatal(err)
	}
	record, err := loadCleanupUncertain(st)
	if err != nil || record.OperationID != "op-study-deadline" || record.OwnerPID < 1 {
		t.Fatalf("record=%+v err=%v", record, err)
	}
	changed, err := service.ReconcileInterruptedRun(context.Background(), st.Name)
	if err != nil || !changed {
		t.Fatalf("changed=%t err=%v", changed, err)
	}
	loaded, err := LoadRunState(st)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Tasks[0].Status != TaskStatusCancelled || loaded.Tasks[0].LastError == nil || loaded.Tasks[0].LastError.Code != "workflow.interrupted" || loaded.Tasks[0].CompletedAt == nil {
		t.Fatalf("task was not reconciled: %+v", loaded.Tasks[0])
	}
	if _, err := os.Stat(cleanupUncertainPath(st)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("consumed marker still exists: %v", err)
	}
}

func TestCleanupUncertaintyFailsClosedWithoutActiveCanonicalState(t *testing.T) {
	root, st := executionFixture(t)
	service := NewService(root)
	if err := service.RecordCleanupUncertain(context.Background(), st.Name, CleanupUncertainRecord{
		OperationID: "op-study-deadline",
		Kind:        "study-resume",
		Reason:      "server_shutdown",
	}); err != nil {
		t.Fatal(err)
	}
	changed, err := service.ReconcileInterruptedRun(context.Background(), st.Name)
	if changed || !errors.Is(err, ErrCleanupUncertain) {
		t.Fatalf("changed=%t err=%v", changed, err)
	}
	if _, err := os.Stat(cleanupUncertainPath(st)); err != nil {
		t.Fatalf("unresolved marker was removed: %v", err)
	}
}
