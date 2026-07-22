package sprint

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	pruntime "github.com/Antonio7098/ultraplan-go/internal/platform/runtime"
)

type reviewRuntime struct {
	mu        sync.Mutex
	calls     int
	malformed bool
	requests  []pruntime.Request
}

type concurrentReviewRuntime struct{ active, max atomic.Int32 }

type mutateReviewRuntime struct {
	once sync.Once
	path string
}

type contextReviewRuntime struct{}

func (contextReviewRuntime) StartRun(ctx context.Context, _ pruntime.Request) (pruntime.Result, error) {
	<-ctx.Done()
	return pruntime.Result{}, ctx.Err()
}

func (r *mutateReviewRuntime) StartRun(_ context.Context, req pruntime.Request) (pruntime.Result, error) {
	r.once.Do(func() { _ = os.WriteFile(r.path, []byte("# Requirements\n\nChanged while reviewing.\n"), 0644) })
	data, _ := json.Marshal(ReviewCoverageResult{SchemaVersion: 1, CoverageID: req.Metadata["coverage"], Applicability: "direct", Summary: "checked"})
	return pruntime.Result{Events: []pruntime.Event{{Payload: map[string]any{"content": string(data)}}}, Permissions: pruntime.PermissionSummary{Mode: "restricted"}}, nil
}

func (r *concurrentReviewRuntime) StartRun(_ context.Context, req pruntime.Request) (pruntime.Result, error) {
	n := r.active.Add(1)
	for {
		old := r.max.Load()
		if n <= old || r.max.CompareAndSwap(old, n) {
			break
		}
	}
	time.Sleep(5 * time.Millisecond)
	r.active.Add(-1)
	data, _ := json.Marshal(ReviewCoverageResult{SchemaVersion: 1, CoverageID: req.Metadata["coverage"], Applicability: "direct", Summary: "checked"})
	return pruntime.Result{Events: []pruntime.Event{{Payload: map[string]any{"content": string(data)}}}, Permissions: pruntime.PermissionSummary{Mode: "restricted"}}, nil
}

func (r *reviewRuntime) StartRun(_ context.Context, req pruntime.Request) (pruntime.Result, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	r.requests = append(r.requests, req)
	content := "not-json"
	if !r.malformed {
		data, _ := json.Marshal(ReviewCoverageResult{SchemaVersion: 1, CoverageID: req.Metadata["coverage"], Applicability: "direct", Summary: "Conforms to the selected scope."})
		content = string(data)
	}
	return pruntime.Result{Status: "success", Events: []pruntime.Event{{Type: "message", Payload: map[string]any{"content": content}}}, Permissions: pruntime.PermissionSummary{Mode: "restricted", Default: "deny"}}, nil
}

func TestReviewManifestExecutionAndArtifactPreservation(t *testing.T) {
	root, sp := reviewFixture(t)
	rt := &reviewRuntime{}
	service := NewService(root).WithRuntime(rt).WithStageRuntime(map[PlanningStage]StageRuntime{StageReview: {Model: "openai/gpt-5.6", Variant: "medium"}})
	first, findings, err := service.PrepareReview("proj", "01", ReviewRequest{Concurrency: 2})
	if err != nil || len(findings) != 0 {
		t.Fatalf("prepare: err=%v findings=%+v", err, findings)
	}
	second, findings, err := service.PrepareReview("proj", "01", ReviewRequest{Concurrency: 2})
	if err != nil || len(findings) != 0 || first.Fingerprint != second.Fingerprint {
		t.Fatalf("manifest is not deterministic: first=%s second=%s findings=%+v err=%v", first.Fingerprint, second.Fingerprint, findings, err)
	}
	result, err := service.Review(context.Background(), "proj", "01", ReviewRequest{Concurrency: 2})
	if err != nil {
		t.Fatalf("review: %v result=%+v", err, result)
	}
	if result.Status != ReviewCompleted || result.Verdict != ReviewPass || rt.calls != len(first.Coverage) {
		t.Fatalf("review result=%+v calls=%d coverage=%d", result, rt.calls, len(first.Coverage))
	}
	artifact := filepath.Join(sp.Path, "review.md")
	prior, err := os.ReadFile(artifact)
	if err != nil || !strings.Contains(string(prior), "Verdict: `pass`") || !strings.Contains(string(prior), "Review status: `completed`") {
		t.Fatalf("review artifact: err=%v content=%s", err, prior)
	}
	if validation, err := service.ValidateReview("proj", "01"); err != nil || !validation.Valid() {
		t.Fatalf("validation: err=%v findings=%+v", err, validation.Findings)
	}
	rt.malformed = true
	failed, err := service.Review(context.Background(), "proj", "01", ReviewRequest{Concurrency: 2})
	if err == nil || failed.Verdict != ReviewVerdictBlocked {
		t.Fatalf("malformed review result=%+v err=%v", failed, err)
	}
	after, _ := os.ReadFile(artifact)
	if string(after) != string(prior) {
		t.Fatal("failed review replaced the last valid review.md")
	}
	for _, req := range rt.requests {
		if req.WorkDir == "" || req.Policy.Default != "deny" || req.Sandbox != "read_only" {
			t.Fatalf("unsafe reviewer request: %+v", req)
		}
	}
}

func TestReviewVerdictAndCitationValidation(t *testing.T) {
	root, _ := reviewFixture(t)
	service := NewService(root).WithStageRuntime(map[PlanningStage]StageRuntime{StageReview: {Model: "openai/gpt-5.6"}})
	m, findings, err := service.PrepareReview("proj", "01", ReviewRequest{})
	if err != nil || len(findings) != 0 {
		t.Fatalf("prepare: %v %+v", err, findings)
	}
	results := make([]ReviewCoverageResult, 0, len(m.Coverage))
	for _, coverage := range m.Coverage {
		results = append(results, ReviewCoverageResult{SchemaVersion: 1, CoverageID: coverage.ID, Applicability: "direct", Summary: "checked"})
	}
	results[0].Findings = []ReviewFinding{{ID: "F-1", Severity: "medium", Applicability: "direct", Title: "Follow-up", Detail: "Small issue", Citations: []ReviewCitation{{Path: ArtifactRelPath(Sprint{Project: "proj", Slug: "01-alpha"}, StageRequirements), StartLine: 1, EndLine: 1}}}}
	fs, ds, verdict := validateReviewCoverage(root, m, results)
	if len(fs) != 1 || len(ds) != 0 || verdict != ReviewPassWithFindings {
		t.Fatalf("warning verdict=%s findings=%+v diagnostics=%+v", verdict, fs, ds)
	}
	results[0].Findings[0].Severity = "blocker"
	_, _, verdict = validateReviewCoverage(root, m, results)
	if verdict != ReviewFail {
		t.Fatalf("critical verdict=%s", verdict)
	}
	results[0].Findings[0].Citations[0].EndLine = 999
	_, ds, verdict = validateReviewCoverage(root, m, results)
	if verdict != ReviewVerdictBlocked || len(ds) == 0 {
		t.Fatalf("invalid line verdict=%s diagnostics=%+v", verdict, ds)
	}
	results[0].Findings[0].Citations[0].EndLine = 1
	results[0].Findings[0].Citations[0].Path = "../../etc/passwd"
	_, ds, verdict = validateReviewCoverage(root, m, results)
	if verdict != ReviewVerdictBlocked || len(ds) == 0 {
		t.Fatalf("unsafe citation verdict=%s diagnostics=%+v", verdict, ds)
	}
}

func TestExtractReviewResultReadsOpenCodeTextPart(t *testing.T) {
	data, _ := json.Marshal(ReviewCoverageResult{SchemaVersion: 1, CoverageID: "contract-testing", Applicability: "direct", Summary: "checked"})
	r := pruntime.Result{Events: []pruntime.Event{{Type: "text", Payload: map[string]any{"part": map[string]any{"type": "text", "text": string(data)}}}}}
	var out ReviewCoverageResult
	if !extractReviewResult(r, &out) {
		t.Fatal("expected review result from OpenCode part.text")
	}
	if out.CoverageID != "contract-testing" || out.Summary != "checked" {
		t.Fatalf("unexpected result: %+v", out)
	}
}

func TestAtomicReviewWritePreservesPriorArtifactOnRenameFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "review.md")
	if err := os.WriteFile(path, []byte("prior\n"), 0644); err != nil {
		t.Fatal(err)
	}
	err := atomicWriteReviewWithHooks(path, []byte("next\n"), reviewWriteHooks{BeforeRename: func(string) error { return context.Canceled }})
	if err == nil {
		t.Fatal("expected injected failure")
	}
	data, _ := os.ReadFile(path)
	if string(data) != "prior\n" {
		t.Fatalf("artifact changed: %q", data)
	}
}

func TestReviewFanOutUsesConfiguredBound(t *testing.T) {
	root, _ := reviewFixture(t)
	rt := &concurrentReviewRuntime{}
	service := NewService(root).WithRuntime(rt).WithStageRuntime(map[PlanningStage]StageRuntime{StageReview: {Model: "openai/gpt-5.6"}})
	result, err := service.Review(context.Background(), "proj", "01", ReviewRequest{Concurrency: 2})
	if err != nil {
		t.Fatal(err)
	}
	if got := rt.max.Load(); got < 2 || got > 2 {
		t.Fatalf("peak concurrency=%d", got)
	}
	if len(result.Coverage) != 2 {
		t.Fatalf("coverage=%d", len(result.Coverage))
	}
}

func TestReviewDetectsInputDriftBeforePersistence(t *testing.T) {
	root, sp := reviewFixture(t)
	rt := &mutateReviewRuntime{path: filepath.Join(sp.Path, "requirements.md")}
	service := NewService(root).WithRuntime(rt).WithStageRuntime(map[PlanningStage]StageRuntime{StageReview: {Model: "openai/gpt-5.6"}})
	result, err := service.Review(context.Background(), "proj", "01", ReviewRequest{Concurrency: 2})
	if err == nil || result.Status != ReviewFailed {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if _, err := os.Stat(filepath.Join(sp.Path, "review.md")); !os.IsNotExist(err) {
		t.Fatalf("stale review wrote artifact: %v", err)
	}
}

func TestReviewCancellationAndBlockedPreflightDoNotPass(t *testing.T) {
	root, sp := reviewFixture(t)
	service := NewService(root).WithRuntime(contextReviewRuntime{}).WithStageRuntime(map[PlanningStage]StageRuntime{StageReview: {Model: "openai/gpt-5.6"}})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := service.Review(ctx, "proj", "01", ReviewRequest{Concurrency: 2})
	if err == nil || result.Status != ReviewCancelled || result.Verdict == ReviewPass {
		t.Fatalf("cancel result=%+v err=%v", result, err)
	}
	rt := &reviewRuntime{}
	writeFileContent(t, sp.Path, validPlan(), "plan.md")
	service = NewService(root).WithRuntime(rt).WithStageRuntime(map[PlanningStage]StageRuntime{StageReview: {Model: "openai/gpt-5.6"}})
	result, err = service.Review(context.Background(), "proj", "01", ReviewRequest{})
	if err == nil || result.Status != ReviewBlocked || rt.calls != 0 {
		t.Fatalf("blocked result=%+v calls=%d err=%v", result, rt.calls, err)
	}
	if got := safeReviewText("/workspace", "token=secret /workspace/file"); strings.Contains(got, "secret") || strings.Contains(got, "/workspace") {
		t.Fatalf("unsafe diagnostic %q", got)
	}
}

func reviewFixture(t *testing.T) (string, Sprint) {
	t.Helper()
	root := workspaceFixture(t)
	sp := sprintFixture(t, root, "proj", "01-alpha")
	writeFileContent(t, filepath.Join(root, "projects", "proj"), testProjectIndex(), "project-index.md")
	writeFileContent(t, root, "# Architecture\n", ".ultra", "system", "contracts", "core", "architecture.md")
	writeFileContent(t, root, "# Review Protocol\n", ".ultra", "system", "protocols", "sprint-review-protocol.md")
	writeFileContent(t, root, "# Architecture Template\n", "system", "reasoning", "architecture_reasoning_template.md")
	writeFileContent(t, root, "# Evidence\n", "studies", "go-cli-study", "reports", "final", "01-project-structure.md")
	writeFileContent(t, sp.Path, "# Requirements\n\nReview this sprint.\n", "requirements.md")
	writeFileContent(t, sp.Path, validSprintIndex(), "sprint-index.md")
	writeFileContent(t, sp.Path, validReasoningTechnicalHandbook(), "technical-handbook.md")
	writeFileContent(t, sp.Path, validAreaReasoning(), "reasoning", "architecture.md")
	writeFileContent(t, sp.Path, validPlanFinalReasoning(), "reasoning.md")
	writeFileContent(t, sp.Path, strings.ReplaceAll(validPlan(), "- [ ]", "- [x]"), "plan.md")
	writeFileContent(t, sp.Path, "# Execute Summary\n\nAll tasks complete.\n\n- `go test ./...`: pass\n", "execute.md")
	writeFileContent(t, sp.Path, `{"files":["internal/sprint/review.go"]}`+"\n", ".run-state.json")
	return root, sp
}
