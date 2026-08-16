package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestWebOperationPreparationNormalizesFingerprintsAndHasNoSideEffects(t *testing.T) {
	root := t.TempDir()
	sprintRoot := filepath.Join(root, "projects", "alpha", "sprints", "31-web")
	if err := os.MkdirAll(sprintRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sprintRoot, "plan.md"), []byte("# Plan\n\n- task\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	u := dashboardUseCases{root: root, reviewConcurrency: 3}
	before := operationTree(t, root)
	first, err := u.PrepareOperation(context.Background(), OperationRequest{
		Kind: OperationFlow, Project: " alpha ", Sprint: "31-web", Stage: "plan",
		ReviewFocus: []string{"security", "architecture", "security"},
	})
	if err != nil {
		t.Fatal(err)
	}
	after := operationTree(t, root)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("preparation mutated workspace: before=%v after=%v", before, after)
	}
	if first.Request.Project != "alpha" || !reflect.DeepEqual(first.Request.ReviewFocus, []string{"architecture", "security"}) {
		t.Fatalf("normalized request=%+v", first.Request)
	}
	if first.InputFingerprint == "" || first.CanonicalRequest == "" || first.MutationClass != "sprint_mutation" || first.Request.ExpectedFingerprint != first.InputFingerprint {
		t.Fatalf("preparation=%+v", first)
	}
	second, err := u.PrepareOperation(context.Background(), first.Request)
	if err != nil || second.InputFingerprint != first.InputFingerprint {
		t.Fatalf("stable fingerprint first=%q second=%q err=%v", first.InputFingerprint, second.InputFingerprint, err)
	}
	if err := os.WriteFile(filepath.Join(sprintRoot, "plan.md"), []byte("# Plan\n\n- changed task\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	changed, err := u.PrepareOperation(context.Background(), first.Request)
	if err != nil {
		t.Fatal(err)
	}
	if changed.InputFingerprint == first.InputFingerprint {
		t.Fatal("governed input mutation did not change fingerprint")
	}
}

func TestWebOperationExecutionRejectsStalePreparationBeforeRunner(t *testing.T) {
	root := t.TempDir()
	sprintRoot := filepath.Join(root, "projects", "alpha", "sprints", "31-web")
	if err := os.MkdirAll(sprintRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	plan := filepath.Join(sprintRoot, "plan.md")
	if err := os.WriteFile(plan, []byte("# Plan\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	called := false
	u := dashboardUseCases{root: root, runner: func(context.Context, OperationRequest, func(OperationEvent)) (OperationResult, error) {
		called = true
		return OperationResult{State: OperationComplete}, nil
	}}
	prepared, err := u.PrepareOperation(context.Background(), OperationRequest{Kind: OperationFlow, Project: "alpha", Sprint: "31-web", Stage: "plan"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(plan, []byte("# Changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = u.RunOperation(context.Background(), prepared.Request, nil)
	if !errors.Is(err, ErrStaleOperation) {
		t.Fatalf("error=%v", err)
	}
	if called {
		t.Fatal("runner called for stale preparation")
	}
}

func operationTree(t *testing.T, root string) []string {
	t.Helper()
	var paths []string
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		paths = append(paths, rel)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return paths
}
