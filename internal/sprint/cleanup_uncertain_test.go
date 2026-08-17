package sprint

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRecordCleanupUncertainIsDurableAndReconciliationConsumesIt(t *testing.T) {
	root := t.TempDir()
	projectRoot := filepath.Join(root, "projects", "alpha")
	sprintRoot := filepath.Join(projectRoot, "sprints", "31-web")
	if err := os.MkdirAll(sprintRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, "project-index.md"), []byte("# Project Index: alpha\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sprintRoot, "sprint-index.md"), []byte("# Sprint Index: web\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	service := NewService(root)
	service.now = func() time.Time { return time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC) }
	err := service.RecordCleanupUncertain(context.Background(), "alpha", "31-web", CleanupUncertainRecord{
		OperationID: "op_deadline", Kind: "sprint-flow", Reason: "server_shutdown",
	})
	if err != nil {
		t.Fatal(err)
	}
	sp := Sprint{Project: "alpha", Slug: "31-web", Path: sprintRoot}
	record, err := loadCleanupUncertain(sp)
	if err != nil || record.OperationID != "op_deadline" || record.OwnerPID < 1 || record.RecordedAt.IsZero() {
		t.Fatalf("record=%+v err=%v", record, err)
	}
	changed, err := service.ReconcileInterruptedMutation(context.Background(), "alpha", "31-web")
	if !errors.Is(err, ErrCleanupUncertain) || changed {
		t.Fatalf("reconcile without canonical running state changed=%t err=%v", changed, err)
	}
	if _, err := os.Stat(filepath.Join(sprintRoot, cleanupUncertainFileName)); err != nil {
		t.Fatalf("unreconciled uncertainty marker was removed: %v", err)
	}
}
