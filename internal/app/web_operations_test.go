package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/Antonio7098/ultraplan-go/internal/sprint"
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

func TestWebOperationCodeContextUsesGenericStageAndGovernedFingerprint(t *testing.T) {
	root := t.TempDir()
	sprintRoot := filepath.Join(root, "projects", "alpha", "sprints", "33-context")
	if err := os.MkdirAll(sprintRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	requirements := filepath.Join(sprintRoot, "requirements.md")
	contextPath := filepath.Join(sprintRoot, "code-context.md")
	if err := os.WriteFile(requirements, []byte("# Requirements\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(contextPath, []byte("# Context\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	u := dashboardUseCases{root: root, stageRuntime: map[sprint.PlanningStage]sprint.StageRuntime{sprint.StageCodeContext: {Model: "vendor/context", Variant: "high"}}}
	prepared, err := u.PrepareOperation(context.Background(), OperationRequest{Kind: OperationStage, Project: "alpha", Sprint: "33-context", Stage: "code-context"})
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Request.Kind != OperationStage || prepared.Request.Stage != "code-context" || prepared.ModelSource != "vendor/context variant=high" || prepared.MutationClass != "sprint_mutation" {
		t.Fatalf("code-context preparation did not reuse generic stage operation: %+v", prepared)
	}
	if err := os.WriteFile(contextPath, []byte("# Changed context\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	changed, err := u.PrepareOperation(context.Background(), prepared.Request)
	if err != nil {
		t.Fatal(err)
	}
	if changed.InputFingerprint == prepared.InputFingerprint {
		t.Fatal("code-context governed artifact mutation did not invalidate confirmation")
	}
}

func TestProjectReasoningOperationPreparationUsesProjectScope(t *testing.T) {
	root := t.TempDir()
	projectRoot := filepath.Join(root, "projects", "alpha")
	if err := os.MkdirAll(filepath.Join(projectRoot, "project-reasoning"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, "project-index.md"), []byte("# Project index\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	u := dashboardUseCases{root: root}
	prepared, err := u.PrepareOperation(context.Background(), OperationRequest{Kind: OperationProjectReasoningFlow, Project: "alpha", Stage: "review"})
	if err != nil {
		t.Fatal(err)
	}
	if !prepared.Runtime || !prepared.Mutates || prepared.MutationClass != "project_mutation" || prepared.DurableRefreshPath != "/api/v1/projects/alpha" {
		t.Fatalf("project reasoning preparation = %+v", prepared)
	}
	if !reflect.DeepEqual(prepared.Paths, []string{"projects/alpha"}) || prepared.Request.Sprint != "" {
		t.Fatalf("project reasoning scope = %+v", prepared)
	}
}

func TestWebCleanupUncertaintyDelegatesToStudyOwner(t *testing.T) {
	root := t.TempDir()
	studyRoot := filepath.Join(root, "studies", "demo")
	if err := os.MkdirAll(studyRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	useCases := NewWebUseCases(root, WebUseCaseOptions{})
	recorder, ok := useCases.(OperationCleanupRecorder)
	if !ok {
		t.Fatal("web use cases do not expose cleanup recording")
	}
	if err := recorder.RecordOperationCleanupUncertain(context.Background(), OperationCleanupUncertain{
		OperationID: "op-study",
		Request:     OperationRequest{Kind: OperationStudyStart, Study: "demo"},
		Reason:      "server_shutdown",
		RecordedAt:  time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(studyRoot, ".ultraplan", "cleanup-uncertain.json")); err != nil {
		t.Fatalf("study-owned cleanup marker missing: %v", err)
	}
}

func TestQAOperationPreparationRejectsEveryCallerOwnedControl(t *testing.T) {
	u := dashboardUseCases{}
	base := OperationRequest{Kind: OperationQAStart, Project: "alpha", Sprint: "36-read-only-qa"}
	for name, mutate := range map[string]func(*OperationRequest){
		"stage":       func(req *OperationRequest) { req.Stage = "qa" },
		"model":       func(req *OperationRequest) { req.Model = "caller/model" },
		"timeout":     func(req *OperationRequest) { req.Timeout = "1h" },
		"parallelism": func(req *OperationRequest) { req.Parallelism = 2 },
		"sources":     func(req *OperationRequest) { req.Sources = []string{"caller"} },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := base
			mutate(&candidate)
			if _, err := u.PrepareOperation(context.Background(), candidate); err == nil {
				t.Fatal("caller-owned QA control was accepted")
			}
		})
	}
	if _, err := u.PrepareOperation(context.Background(), OperationRequest{Kind: OperationQAStatus, Project: "alpha", Sprint: "36-read-only-qa", Task: "qa-v1-shard-aaaaaaaaaaaaaaaaaaaaaaaa"}); err == nil {
		t.Fatal("QA status accepted a focused shard")
	}
	for _, req := range []OperationRequest{
		{Kind: OperationQAResume, Project: "alpha", Sprint: "37-evidence", Suite: "smoke"},
		{Kind: OperationQAStart, Project: "alpha", Sprint: "37-evidence", Suite: "other"},
		{Kind: OperationQAStart, Project: "alpha", Sprint: "37-evidence", Suite: "smoke", Task: "shard"},
	} {
		if err := validateQAOperationRequest(req); err == nil {
			t.Fatalf("invalid QA suite request accepted: %+v", req)
		}
	}
	for _, req := range []OperationRequest{
		{Kind: OperationQAStart, Project: "alpha", Sprint: "37-evidence", Suite: "smoke"},
		{Kind: OperationQADryRun, Project: "alpha", Sprint: "37-evidence", Suite: "smoke"},
	} {
		if err := validateQAOperationRequest(req); err != nil {
			t.Fatalf("valid QA suite request rejected: %+v: %v", req, err)
		}
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
