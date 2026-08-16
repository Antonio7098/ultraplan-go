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

type repairReviewRuntime struct{ calls atomic.Int32 }

type rejectedReviewRuntime struct {
	calls       atomic.Int32
	permissions pruntime.PermissionSummary
	malformed   bool
}

type resumableReviewRuntime struct {
	mu              sync.Mutex
	cancel          context.CancelFunc
	interruptOnCall int
	calls           []pruntime.Request
}

func (r *resumableReviewRuntime) StartRun(ctx context.Context, req pruntime.Request) (pruntime.Result, error) {
	r.mu.Lock()
	r.calls = append(r.calls, req)
	call := len(r.calls)
	r.mu.Unlock()
	sessionID := "session-" + req.Metadata["coverage"]
	if req.OnEvent != nil {
		req.OnEvent(pruntime.Event{SessionID: sessionID, Kind: "session", Payload: map[string]any{"session_id": sessionID}})
	}
	if call == r.interruptOnCall {
		r.cancel()
		<-ctx.Done()
		return pruntime.Result{SessionID: sessionID, Permissions: pruntime.PermissionSummary{Mode: "restricted"}}, ctx.Err()
	}
	data, _ := json.Marshal(ReviewCoverageResult{SchemaVersion: 1, CoverageID: req.Metadata["coverage"], Applicability: "direct", Summary: "checked"})
	return pruntime.Result{SessionID: sessionID, TerminalOutput: string(data), Permissions: pruntime.PermissionSummary{Mode: "restricted"}}, nil
}

func (r *repairReviewRuntime) StartRun(_ context.Context, req pruntime.Request) (pruntime.Result, error) {
	r.calls.Add(1)
	if req.SessionAction != "continue" {
		return pruntime.Result{SessionID: "review-session", Events: []pruntime.Event{{Payload: map[string]any{"text": "analysis without the required object"}}}, Permissions: pruntime.PermissionSummary{Mode: "restricted"}}, nil
	}
	data, _ := json.Marshal(ReviewCoverageResult{SchemaVersion: 1, CoverageID: req.Metadata["coverage"], Applicability: "direct", Summary: "repaired"})
	return pruntime.Result{Events: []pruntime.Event{{Payload: map[string]any{"text": string(data)}}}, Permissions: pruntime.PermissionSummary{Mode: "restricted"}}, nil
}

func (r *rejectedReviewRuntime) StartRun(_ context.Context, req pruntime.Request) (pruntime.Result, error) {
	r.calls.Add(1)
	data, _ := json.Marshal(ReviewCoverageResult{SchemaVersion: 1, CoverageID: req.Metadata["coverage"], Applicability: "direct", Summary: "checked"})
	if r.malformed {
		data = []byte("analysis without the required structured result")
	}
	return pruntime.Result{
		SessionID:      "review-session",
		TerminalOutput: string(data),
		Permissions:    r.permissions,
	}, nil
}

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
	if len(first.ChangedPaths) != 1 || first.ChangedPaths[0] != "internal/sprint/review.go" {
		t.Fatalf("target changed paths include governed workspace artifacts: %#v", first.ChangedPaths)
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
	state, stateErr := LoadFlowState(root, sp)
	if stateErr != nil || state.Review.LastComplete == nil || state.Review.LastComplete.Verdict != ReviewPass || state.Review.LastAttempt == nil || state.Review.LastAttempt.Status != AttemptFailed {
		t.Fatalf("failed attempt did not preserve last complete review: state=%+v err=%v", state.Review, stateErr)
	}
	for _, req := range rt.requests {
		if req.WorkDir == "" || req.Policy.Default != "deny" || req.Policy.Tools["external_directory"] != "" || req.Sandbox != "read_only" {
			t.Fatalf("unsafe reviewer request: %+v", req)
		}
		if len(req.Prompt) > reviewPromptMaxBytes || strings.Contains(req.Prompt, "# Requirements\n\nReview this sprint.") {
			t.Fatalf("reviewer prompt was not compact and path-backed: bytes=%d", len(req.Prompt))
		}
		if req.SessionAction == "" && (strings.Contains(req.Prompt, filepath.Join(root, filepath.FromSlash(ArtifactRelPath(sp, StageRequirements)))) || !strings.Contains(req.Prompt, filepath.Join(req.WorkDir, "workspace", filepath.FromSlash(ArtifactRelPath(sp, StageRequirements))))) {
			t.Fatalf("reviewer prompt omitted governed input path: %s", req.Prompt)
		}
	}
}

func TestReviewResumesValidatedCoverageAndOpenCodeSession(t *testing.T) {
	root, sp := reviewFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	runtime := &resumableReviewRuntime{cancel: cancel, interruptOnCall: 2}
	service := NewService(root).WithRuntime(runtime).WithStageRuntime(map[PlanningStage]StageRuntime{StageReview: {Model: "minimax-coding-plan/MiniMax-M3"}}).WithReviewConcurrency(1)

	first, err := service.Review(ctx, "proj", "01", ReviewRequest{Concurrency: 1})
	if err == nil || first.Status != ReviewCancelled {
		t.Fatalf("interrupted result=%+v err=%v", first, err)
	}
	state, err := LoadFlowState(root, sp)
	if err != nil || state.Review == nil || state.Review.Resume == nil {
		t.Fatalf("missing resume state: review=%+v err=%v", state.Review, err)
	}
	completed, retainedSession := 0, ""
	for _, checkpoint := range state.Review.Resume.Coverage {
		if checkpoint.Status == AttemptCompleted {
			completed++
		} else if checkpoint.SessionID != "" {
			retainedSession = checkpoint.SessionID
		}
	}
	if completed != 1 || retainedSession == "" {
		t.Fatalf("resume checkpoints=%+v", state.Review.Resume.Coverage)
	}

	runtime.interruptOnCall = 0
	resumed, err := service.Review(context.Background(), "proj", "01", ReviewRequest{Concurrency: 1})
	if err != nil || resumed.Status != ReviewCompleted || !resumed.Resumed || resumed.Reused != 1 {
		t.Fatalf("resumed result=%+v err=%v", resumed, err)
	}
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if len(runtime.calls) != 3 {
		t.Fatalf("runtime calls=%d want 3", len(runtime.calls))
	}
	last := runtime.calls[2]
	if last.SessionID != retainedSession || last.SessionAction != "continue" {
		t.Fatalf("resume request session=%q action=%q want %q/continue", last.SessionID, last.SessionAction, retainedSession)
	}
}

func TestReviewRestartDiscardsCoverageAndSessions(t *testing.T) {
	root, _ := reviewFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	runtime := &resumableReviewRuntime{cancel: cancel, interruptOnCall: 2}
	service := NewService(root).WithRuntime(runtime).WithStageRuntime(map[PlanningStage]StageRuntime{StageReview: {Model: "minimax-coding-plan/MiniMax-M3"}}).WithReviewConcurrency(1)
	if _, err := service.Review(ctx, "proj", "01", ReviewRequest{Concurrency: 1}); err == nil {
		t.Fatal("expected interrupted review")
	}

	runtime.interruptOnCall = 0
	restarted, err := service.Review(context.Background(), "proj", "01", ReviewRequest{Concurrency: 1, Restart: true})
	if err != nil || restarted.Status != ReviewCompleted || !restarted.Restarted || restarted.Resumed || restarted.Reused != 0 {
		t.Fatalf("restarted result=%+v err=%v", restarted, err)
	}
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if len(runtime.calls) != 4 {
		t.Fatalf("runtime calls=%d want 4", len(runtime.calls))
	}
	for _, req := range runtime.calls[2:] {
		if req.SessionID != "" || req.SessionAction != "" {
			t.Fatalf("restart reused session: id=%q action=%q", req.SessionID, req.SessionAction)
		}
	}
}

func TestReviewerPromptStaysBelowSubprocessArgumentBudget(t *testing.T) {
	root, sp := reviewFixture(t)
	large := strings.Repeat("governed evidence line\n", 10_000)
	writeFileContent(t, sp.Path, "# Requirements\n\n"+large, "requirements.md")
	service := NewService(root).WithStageRuntime(map[PlanningStage]StageRuntime{StageReview: {Model: "openai/gpt-5.6"}})
	manifest, findings, err := service.PrepareReview("proj", "01", ReviewRequest{})
	if err != nil || len(findings) != 0 {
		t.Fatalf("prepare: err=%v findings=%+v", err, findings)
	}
	prompt := renderReviewerPrompt(manifest, manifest.Coverage[0])
	if len(prompt) > reviewPromptMaxBytes {
		t.Fatalf("prompt bytes=%d budget=%d", len(prompt), reviewPromptMaxBytes)
	}
	if strings.Contains(prompt, large) || !strings.Contains(prompt, filepath.Join(root, filepath.FromSlash(ArtifactRelPath(sp, StageRequirements)))) {
		t.Fatal("prompt did not replace governed content with its readable frozen path")
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
	results[0].Findings[0].Citations[0].Path = "go.mod"
	_, ds, verdict = validateReviewCoverage(root, m, results)
	if verdict != ReviewVerdictBlocked || len(ds) == 0 {
		t.Fatalf("unfrozen citation verdict=%s diagnostics=%+v", verdict, ds)
	}
}

func TestReviewResultSchemaCanonicalizesLegacyDeferredAndRejectsDuplicateFindingIDs(t *testing.T) {
	root, _ := reviewFixture(t)
	service := NewService(root).WithStageRuntime(map[PlanningStage]StageRuntime{StageReview: {Model: "openai/gpt-5.6"}})
	manifest, findings, err := service.PrepareReview("proj", "01", ReviewRequest{})
	if err != nil || len(findings) != 0 {
		t.Fatalf("prepare: err=%v findings=%+v", err, findings)
	}
	coverage := manifest.Coverage[0]
	legacy := ReviewCoverageResult{SchemaVersion: 1, CoverageID: coverage.ID, Applicability: "deferred", Summary: "Deferred by the governing contract."}
	normalizeReviewResult(&legacy)
	if legacy.Applicability != "explicitly_deferred" || len(reviewResultProblems(root, manifest, coverage.ID, legacy)) != 0 {
		t.Fatalf("legacy result not canonicalized: %+v problems=%+v", legacy, reviewResultProblems(root, manifest, coverage.ID, legacy))
	}
	citation := ReviewCitation{Path: coverage.Path, StartLine: 1, EndLine: 1}
	duplicate := ReviewFinding{ID: "DUP-1", Severity: "low", Applicability: "direct", Title: "Issue", Detail: "Actionable deviation.", Citations: []ReviewCitation{citation}}
	results := make([]ReviewCoverageResult, len(manifest.Coverage))
	for i, item := range manifest.Coverage {
		results[i] = ReviewCoverageResult{SchemaVersion: 1, CoverageID: item.ID, Applicability: "direct", Summary: "Checked."}
	}
	results[0].Findings = []ReviewFinding{duplicate}
	results[1].Findings = []ReviewFinding{duplicate}
	_, diagnostics, verdict := validateReviewCoverage(root, manifest, results)
	if verdict != ReviewVerdictBlocked || !hasReviewDiagnostic(diagnostics, "duplicate-finding-id") {
		t.Fatalf("verdict=%s diagnostics=%+v", verdict, diagnostics)
	}
}

func TestReviewResultCanonicalizesFrozenReadPathCitation(t *testing.T) {
	root, _ := reviewFixture(t)
	service := NewService(root).WithStageRuntime(map[PlanningStage]StageRuntime{StageReview: {Model: "openai/gpt-5.6"}})
	manifest, findings, err := service.PrepareReview("proj", "01", ReviewRequest{})
	if err != nil || len(findings) != 0 {
		t.Fatalf("prepare: err=%v findings=%+v", err, findings)
	}
	manifest.ReviewerRoot = filepath.Join(root, "frozen-review")
	input := manifest.Inputs[0]
	result := ReviewCoverageResult{
		SchemaVersion: 1,
		CoverageID:    manifest.Coverage[0].ID,
		Applicability: "direct",
		Summary:       "Checked.",
		Findings: []ReviewFinding{{
			ID: "READ-PATH-1", Severity: "low", Applicability: "direct", Title: "Issue", Detail: "Actionable deviation.",
			Citations: []ReviewCitation{{Path: reviewInputReadPath(manifest, input), StartLine: 1, EndLine: 1}},
		}},
	}
	normalizeReviewResultForManifest(manifest, &result)
	if got := result.Findings[0].Citations[0].Path; got != input.Path {
		t.Fatalf("citation path=%q want logical %q", got, input.Path)
	}
	if problems := reviewResultProblems(root, manifest, result.CoverageID, result); len(problems) != 0 {
		t.Fatalf("canonical citation rejected: %+v", problems)
	}
}

func TestReviewRepairPromptRetainsPriorOutputAndFrozenCitationMap(t *testing.T) {
	root, _ := reviewFixture(t)
	service := NewService(root).WithStageRuntime(map[PlanningStage]StageRuntime{StageReview: {Model: "openai/gpt-5.6"}})
	manifest, findings, err := service.PrepareReview("proj", "01", ReviewRequest{})
	if err != nil || len(findings) != 0 {
		t.Fatalf("prepare: err=%v findings=%+v", err, findings)
	}
	manifest.ReviewerRoot = filepath.Join(root, "frozen-review")
	coverage := manifest.Coverage[0]
	prompt := buildReviewRepairPrompt(manifest, coverage, []string{"citation is invalid"}, `{"coverageId":"prior"}`)
	for _, want := range []string{coverage.ID, "citation is invalid", manifest.Inputs[0].Path, reviewInputReadPath(manifest, manifest.Inputs[0]), `{"coverageId":"prior"}`} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("repair prompt missing %q: %s", want, prompt)
		}
	}
}

func hasReviewDiagnostic(values []ReviewDiagnostic, code string) bool {
	for _, value := range values {
		if value.Code == code {
			return true
		}
	}
	return false
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

func TestExtractReviewResultUsesUntruncatedTerminalOutput(t *testing.T) {
	summary := strings.Repeat("review evidence ", 400)
	data, _ := json.Marshal(ReviewCoverageResult{SchemaVersion: 1, CoverageID: "contract-testing", Applicability: "direct", Summary: summary})
	r := pruntime.Result{
		TerminalOutput: string(data),
		Events:         []pruntime.Event{{Type: "text", Payload: map[string]any{"text": string(data[:4096]) + "... [truncated]"}}},
	}
	var out ReviewCoverageResult
	if !extractReviewResult(r, &out) {
		t.Fatal("expected review result from untruncated terminal output")
	}
	if out.CoverageID != "contract-testing" || out.Summary != summary {
		t.Fatalf("unexpected result: coverage=%q summary-bytes=%d", out.CoverageID, len(out.Summary))
	}
}

func TestReviewerGetsOneStructuredOutputRepair(t *testing.T) {
	root, _ := reviewFixture(t)
	runtime := &repairReviewRuntime{}
	service := NewService(root).WithRuntime(runtime).WithStageRuntime(map[PlanningStage]StageRuntime{StageReview: {Model: "openai/gpt-5.6"}})
	result, err := service.Review(context.Background(), "proj", "01", ReviewRequest{Concurrency: 2})
	if err != nil || result.Verdict != ReviewPass {
		t.Fatalf("repair review result=%+v err=%v", result, err)
	}
	if runtime.calls.Load() != int32(len(result.Coverage)*2) {
		t.Fatalf("runtime calls=%d coverage=%d", runtime.calls.Load(), len(result.Coverage))
	}
}

func TestReviewerBlocksUnsupportedPermissions(t *testing.T) {
	root, _ := reviewFixture(t)
	runtime := &rejectedReviewRuntime{permissions: pruntime.PermissionSummary{Mode: "restricted", UnsupportedCount: 1}}
	service := NewService(root).WithRuntime(runtime).WithStageRuntime(map[PlanningStage]StageRuntime{StageReview: {Model: "openai/gpt-5.6"}})
	result, err := service.Review(context.Background(), "proj", "01", ReviewRequest{Concurrency: 2})
	if err == nil || result.Verdict != ReviewVerdictBlocked {
		t.Fatalf("permission review result=%+v err=%v", result, err)
	}
	if runtime.calls.Load() != int32(len(result.Coverage)) {
		t.Fatalf("runtime calls=%d coverage=%d", runtime.calls.Load(), len(result.Coverage))
	}
}

func TestReviewerBlocksAfterStructuredOutputRepairIsExhausted(t *testing.T) {
	root, _ := reviewFixture(t)
	runtime := &rejectedReviewRuntime{permissions: pruntime.PermissionSummary{Mode: "restricted"}, malformed: true}
	service := NewService(root).WithRuntime(runtime).WithStageRuntime(map[PlanningStage]StageRuntime{StageReview: {Model: "openai/gpt-5.6"}})
	result, err := service.Review(context.Background(), "proj", "01", ReviewRequest{Concurrency: 2})
	if err == nil || result.Verdict != ReviewVerdictBlocked {
		t.Fatalf("exhausted repair result=%+v err=%v", result, err)
	}
	if runtime.calls.Load() != int32(len(result.Coverage)*3) {
		t.Fatalf("runtime calls=%d coverage=%d", runtime.calls.Load(), len(result.Coverage))
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
	if result.Verdict != ReviewVerdictBlocked || result.ProvisionalVerdict != ReviewPass || !reviewDiagnosticsContain(result.Diagnostics, "requirements.md") {
		t.Fatalf("drift outcome did not separate authoritative/provisional verdict or identify input: %+v", result)
	}
	if _, err := os.Stat(filepath.Join(sp.Path, "review.md")); !os.IsNotExist(err) {
		t.Fatalf("stale review wrote artifact: %v", err)
	}
}

func reviewDiagnosticsContain(values []ReviewDiagnostic, text string) bool {
	for _, value := range values {
		if strings.Contains(value.Message, text) {
			return true
		}
	}
	return false
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
	writeFileContent(t, sp.Path, `{"files":["internal/sprint/review.go","projects/proj/sprints/01-alpha/plan.md"]}`+"\n", ".run-state.json")
	return root, sp
}
