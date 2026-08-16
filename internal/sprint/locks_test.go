package sprint

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSprintMutationLeaseIsSharedAndCompositeSafe(t *testing.T) {
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
	one := NewService(root)
	two := NewService(root)
	ctx, release, err := one.acquireMutationContext(context.Background(), "alpha", "31-web")
	if err != nil {
		t.Fatal(err)
	}
	if _, nestedRelease, err := one.acquireMutationContext(ctx, "alpha", "31-web"); err != nil {
		t.Fatalf("nested lease: %v", err)
	} else {
		nestedRelease()
	}
	if _, _, err := two.acquireMutationContext(context.Background(), "alpha", "31-web"); !errors.Is(err, ErrVerificationConflict) {
		t.Fatalf("cross-service conflict error=%v", err)
	}
	release()
	ctx, secondRelease, err := two.acquireMutationContext(context.Background(), "alpha", "31-web")
	if err != nil || ctx == nil {
		t.Fatalf("reacquire err=%v", err)
	}
	secondRelease()
}

func TestReconcileInterruptedMutationLeavesLegacyTerminalRunStateUntouched(t *testing.T) {
	root := t.TempDir()
	projectRoot := filepath.Join(root, "projects", "alpha")
	sprintRoot := filepath.Join(projectRoot, "sprints", "01-legacy")
	if err := os.MkdirAll(sprintRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, "project-index.md"), []byte("# Project Index: alpha\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sprintRoot, "sprint-index.md"), []byte("# Sprint Index: legacy\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	legacy := []byte("{\n  \"status\": \"complete\",\n  \"completedAt\": \"2026-01-01T00:00:00Z\"\n}\n")
	statePath := filepath.Join(sprintRoot, ".run-state.json")
	if err := os.WriteFile(statePath, legacy, 0o644); err != nil {
		t.Fatal(err)
	}

	changed, err := NewService(root).ReconcileInterruptedMutation(context.Background(), "alpha", "01-legacy")
	if err != nil {
		t.Fatalf("reconcile legacy terminal state: %v", err)
	}
	if changed {
		t.Fatal("legacy terminal state unexpectedly changed")
	}
	got, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(legacy) {
		t.Fatalf("legacy state was rewritten:\n%s", got)
	}
}

func TestReconcileInterruptedMutationLeavesLegacyFlowStateUntouched(t *testing.T) {
	root := t.TempDir()
	projectRoot := filepath.Join(root, "projects", "alpha")
	sprintRoot := filepath.Join(projectRoot, "sprints", "01-legacy")
	if err := os.MkdirAll(sprintRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, "project-index.md"), []byte("# Project Index: alpha\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sprintRoot, "sprint-index.md"), []byte("# Sprint Index: legacy\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	legacy := []byte("{\n  \"version\": 1,\n  \"project\": \"alpha\",\n  \"sprint\": \"01-legacy\",\n  \"stages\": {}\n}\n")
	statePath := filepath.Join(sprintRoot, "flow-state.json")
	if err := os.WriteFile(statePath, legacy, 0o644); err != nil {
		t.Fatal(err)
	}

	changed, err := NewService(root).ReconcileInterruptedMutation(context.Background(), "alpha", "01-legacy")
	if err != nil {
		t.Fatalf("reconcile legacy flow state: %v", err)
	}
	if changed {
		t.Fatal("legacy flow state unexpectedly changed")
	}
	got, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(legacy) {
		t.Fatalf("legacy state was rewritten:\n%s", got)
	}
}

func TestReconcileInterruptedMutationRejectsUnrecognizedMalformedRunState(t *testing.T) {
	root := t.TempDir()
	projectRoot := filepath.Join(root, "projects", "alpha")
	sprintRoot := filepath.Join(projectRoot, "sprints", "01-broken")
	if err := os.MkdirAll(sprintRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, "project-index.md"), []byte("# Project Index: alpha\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sprintRoot, "sprint-index.md"), []byte("# Sprint Index: broken\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sprintRoot, ".run-state.json"), []byte("{not json\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := NewService(root).ReconcileInterruptedMutation(context.Background(), "alpha", "01-broken")
	if !errors.Is(err, ErrExecuteRunStateMalformed) {
		t.Fatalf("error=%v, want ErrExecuteRunStateMalformed", err)
	}
}
