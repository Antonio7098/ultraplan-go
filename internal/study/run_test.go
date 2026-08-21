package study

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	runtimepkg "github.com/Antonio7098/ultraplan-go/internal/platform/runtime"
)

type fakeRuntime struct {
	requests []runtimepkg.Request
	result   runtimepkg.Result
	err      error
	write    string
	mutate   map[string]string
}

func (f *fakeRuntime) StartRun(ctx context.Context, req runtimepkg.Request) (runtimepkg.Result, error) {
	if ctx == nil {
		panic("nil context")
	}
	f.requests = append(f.requests, req)
	if f.result.SessionID != "" && req.OnEvent != nil {
		req.OnEvent(runtimepkg.Event{SessionID: f.result.SessionID, Kind: "session"})
	}
	if f.write != "" && req.Validation != nil && len(req.Validation.Expectations) > 0 {
		path := req.Validation.Expectations[0].Path
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			panic(err)
		}
		if err := os.WriteFile(path, []byte(f.write), 0o644); err != nil {
			panic(err)
		}
	}
	for path, content := range f.mutate {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			panic(err)
		}
	}
	return f.result, f.err
}

func TestRunAnalysisContinuesCompatibleInterruptedSession(t *testing.T) {
	root, _ := executionFixture(t)
	first := &fakeRuntime{result: runtimepkg.Result{RunID: "run-1", SessionID: "study-session", Status: "failed", Error: &runtimepkg.Error{Category: "rate_limit"}}, err: errors.New("rate limited")}
	service := NewService(root, WithRuntime(first, runtimeRequest()))
	var checkpoint TaskSession
	if _, err := service.RunAnalysis(context.Background(), ExecutionRequest{StudyRef: "demo", DimensionRef: "01", SourceRef: "repo", OnSession: func(value TaskSession) { checkpoint = value }}); err != nil {
		t.Fatal(err)
	}
	if checkpoint.SessionID != "study-session" || checkpoint.InputFingerprint == "" {
		t.Fatalf("checkpoint = %+v", checkpoint)
	}

	second := &fakeRuntime{result: runtimepkg.Result{RunID: "run-2", SessionID: "study-session", Status: "completed"}, write: validSourceReport}
	service = NewService(root, WithRuntime(second, runtimeRequest()))
	result, err := service.RunAnalysis(context.Background(), ExecutionRequest{StudyRef: "demo", DimensionRef: "01", SourceRef: "repo", ResumeSession: &checkpoint})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != ExecutionStatusCompleted || len(second.requests) != 1 {
		t.Fatalf("result=%+v requests=%d", result, len(second.requests))
	}
	request := second.requests[0]
	if request.SessionID != "study-session" || request.SessionAction != "continue" || !strings.HasPrefix(request.Prompt, "Continue the interrupted study task") {
		t.Fatalf("continuation request = %+v", request)
	}
}

func TestRunAnalysisStartsFreshWhenStudyInputChanged(t *testing.T) {
	root, st := executionFixture(t)
	first := &fakeRuntime{result: runtimepkg.Result{SessionID: "old-session", Status: "failed", Error: &runtimepkg.Error{Category: "rate_limit"}}, err: errors.New("rate limited")}
	service := NewService(root, WithRuntime(first, runtimeRequest()))
	var checkpoint TaskSession
	_, _ = service.RunAnalysis(context.Background(), ExecutionRequest{StudyRef: "demo", DimensionRef: "01", SourceRef: "repo", OnSession: func(value TaskSession) { checkpoint = value }})
	writeReport(t, filepath.Join(st.Path, "dimensions", "01-structure.md"), "# Changed structure\n")

	second := &fakeRuntime{result: runtimepkg.Result{Status: "completed"}, write: validSourceReport}
	service = NewService(root, WithRuntime(second, runtimeRequest()))
	if _, err := service.RunAnalysis(context.Background(), ExecutionRequest{StudyRef: "demo", DimensionRef: "01", SourceRef: "repo", ResumeSession: &checkpoint}); err != nil {
		t.Fatal(err)
	}
	if len(second.requests) != 1 || second.requests[0].SessionID != "" || second.requests[0].SessionAction != "" {
		t.Fatalf("changed input reused session: %+v", second.requests)
	}
}

type continuationFallbackRuntime struct {
	requests []runtimepkg.Request
	path     string
}

func (r *continuationFallbackRuntime) StartRun(_ context.Context, req runtimepkg.Request) (runtimepkg.Result, error) {
	r.requests = append(r.requests, req)
	if len(r.requests) == 1 {
		return runtimepkg.Result{SessionID: req.SessionID, Status: "failed", Error: &runtimepkg.Error{Category: "runtime_exit"}}, errors.New("session not found")
	}
	if err := os.MkdirAll(filepath.Dir(r.path), 0o755); err != nil {
		return runtimepkg.Result{}, err
	}
	if err := os.WriteFile(r.path, []byte(validSourceReport), 0o644); err != nil {
		return runtimepkg.Result{}, err
	}
	return runtimepkg.Result{SessionID: "fresh-session", Status: "completed"}, nil
}

func TestRunAnalysisFallsBackOnceWhenContinuationFails(t *testing.T) {
	root, st := executionFixture(t)
	requestConfig := runtimeRequest()
	first := &fakeRuntime{result: runtimepkg.Result{SessionID: "missing-session", Status: "failed", Error: &runtimepkg.Error{Category: "rate_limit"}}, err: errors.New("rate limited")}
	service := NewService(root, WithRuntime(first, requestConfig))
	var checkpoint TaskSession
	_, _ = service.RunAnalysis(context.Background(), ExecutionRequest{StudyRef: "demo", DimensionRef: "01", SourceRef: "repo", OnSession: func(value TaskSession) { checkpoint = value }})
	output := SourceReportPath(st, Source{Name: "repo", Kind: SourceKindDirectory}, Dimension{Number: "01", Slug: "structure"})
	runtime := &continuationFallbackRuntime{path: output}
	service = NewService(root, WithRuntime(runtime, requestConfig))
	result, err := service.RunAnalysis(context.Background(), ExecutionRequest{StudyRef: "demo", DimensionRef: "01", SourceRef: "repo", ResumeSession: &checkpoint})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != ExecutionStatusCompleted || len(runtime.requests) != 2 || runtime.requests[0].SessionAction != "continue" || runtime.requests[1].SessionAction != "fresh" || runtime.requests[1].SessionID != "" {
		t.Fatalf("result=%+v requests=%+v", result, runtime.requests)
	}
}

func TestRunAnalysisSuccessMapsRuntimeRequestAndValidates(t *testing.T) {
	root, st := executionFixture(t)
	rt := &fakeRuntime{result: runtimepkg.Result{RunID: "run-1", Status: "completed"}, write: validSourceReport}
	service := NewService(root, WithRuntime(rt, runtimeRequest()))

	result, err := service.RunAnalysis(context.Background(), ExecutionRequest{StudyRef: "demo", DimensionRef: "01", SourceRef: "repo"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != ExecutionStatusCompleted || result.RuntimeRunID != "run-1" || result.Validation.Status != ValidationStatusPassed {
		t.Fatalf("result = %+v", result)
	}
	if len(rt.requests) != 1 {
		t.Fatalf("runtime calls = %d", len(rt.requests))
	}
	req := rt.requests[0]
	if req.WorkDir != st.Path {
		t.Fatalf("WorkDir = %q", req.WorkDir)
	}
	if req.Policy.Tools["external_directory"] != "deny" {
		t.Fatalf("external_directory policy = %q, want deny", req.Policy.Tools["external_directory"])
	}
	if req.Provider != "anthropic" || req.Model != "claude" || req.Timeout != time.Minute {
		t.Fatalf("runtime config not mapped: %+v", req)
	}
	if req.Metadata["task.kind"] != "analysis" || req.Metadata["source.name"] != "repo" || req.Metadata["dimension.ref"] != "01-structure" {
		t.Fatalf("metadata = %+v", req.Metadata)
	}
	if req.Validation == nil || len(req.Validation.Expectations) != 1 || req.Validation.Expectations[0].Path != SourceReportPath(st, Source{Name: "repo", Kind: SourceKindDirectory}, Dimension{Number: "01", Slug: "structure"}) {
		t.Fatalf("validation expectation = %+v", req.Validation)
	}
	if req.Prompt == "" || !strings.Contains(req.Prompt, "Inspect only the selected source directory") {
		t.Fatalf("prompt not built correctly")
	}
}

func TestRunAnalysisRuntimeFailureAndValidationFailures(t *testing.T) {
	root, _ := executionFixture(t)
	rt := &fakeRuntime{result: runtimepkg.Result{RunID: "run-2", Status: "failed", Error: &runtimepkg.Error{Category: "rate_limit"}}, err: errors.New("rate limited")}
	service := NewService(root, WithRuntime(rt, runtimeRequest()))
	result, err := service.RunAnalysis(context.Background(), ExecutionRequest{StudyRef: "demo", DimensionRef: "01", SourceRef: "repo"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != ExecutionStatusRuntimeFailed || result.RuntimeCategory != "rate_limit" || !errors.Is(result.RuntimeErr, rt.err) {
		t.Fatalf("result = %+v", result)
	}

	rt = &fakeRuntime{result: runtimepkg.Result{RunID: "run-3", Status: "completed"}}
	service = NewService(root, WithRuntime(rt, runtimeRequest()))
	result, err = service.RunAnalysis(context.Background(), ExecutionRequest{StudyRef: "demo", DimensionRef: "01", SourceRef: "repo"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != ExecutionStatusValidationFailed || !hasCheck(result.Validation.Checks, "content.read", ValidationStatusFailed) {
		t.Fatalf("missing output result = %+v", result)
	}

	rt = &fakeRuntime{result: runtimepkg.Result{RunID: "run-4", Status: "completed"}, write: "# Invalid\n"}
	service = NewService(root, WithRuntime(rt, runtimeRequest()))
	result, err = service.RunAnalysis(context.Background(), ExecutionRequest{StudyRef: "demo", DimensionRef: "01", SourceRef: "repo"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != ExecutionStatusValidationFailed || !hasCheck(result.Validation.Checks, "section.summary", ValidationStatusFailed) {
		t.Fatalf("invalid output result = %+v", result)
	}
}

func TestRunAnalysisRecoversCleanRuntimeExitWhenReportValidates(t *testing.T) {
	root, _ := executionFixture(t)
	cause := errors.New("missing final event")
	rt := &fakeRuntime{
		result: runtimepkg.Result{RunID: "run-exit", Status: "failed", Error: &runtimepkg.Error{Category: "runtime_exit"}},
		err:    cause,
		write:  validSourceReport,
	}
	service := NewService(root, WithRuntime(rt, runtimeRequest()))

	result, err := service.RunAnalysis(context.Background(), ExecutionRequest{StudyRef: "demo", DimensionRef: "01", SourceRef: "repo"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != ExecutionStatusCompleted || !errors.Is(result.RuntimeErr, cause) || result.Validation.Status != ValidationStatusPassed {
		t.Fatalf("result = %+v", result)
	}
}

func TestRunAnalysisWarnsWhenRuntimeEditsSourceFiles(t *testing.T) {
	root, st := executionFixture(t)
	sourcePath := filepath.Join(st.Path, "sources", "repo", "main.go")
	writeReport(t, sourcePath, "package main\n")
	rt := &fakeRuntime{
		result: runtimepkg.Result{RunID: "run-edit", Status: "completed"},
		write:  validSourceReport,
		mutate: map[string]string{sourcePath: "package main\n\n// changed by runtime\n"},
	}
	service := NewService(root, WithRuntime(rt, runtimeRequest()))

	result, err := service.RunAnalysis(context.Background(), ExecutionRequest{StudyRef: "demo", DimensionRef: "01", SourceRef: "repo"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != ExecutionStatusCompleted {
		t.Fatalf("Status = %q, want completed", result.Status)
	}
	if len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0], "modified sources/repo/main.go") {
		t.Fatalf("Warnings = %+v", result.Warnings)
	}
}

func TestRunAnalysisSkipsInapplicableMarkdownWithoutRuntime(t *testing.T) {
	root, _ := executionFixture(t)
	rt := &fakeRuntime{}
	service := NewService(root, WithRuntime(rt, runtimeRequest()))
	result, err := service.RunAnalysis(context.Background(), ExecutionRequest{StudyRef: "demo", DimensionRef: "01", SourceRef: "other.md"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != ExecutionStatusSkipped || len(rt.requests) != 0 {
		t.Fatalf("result = %+v calls = %d", result, len(rt.requests))
	}
}

func TestSynthesizeSuccessPreflightBlockAndFinalValidation(t *testing.T) {
	root, st := executionFixture(t)
	writeReport(t, SourceReportPath(st, Source{Name: "repo", Kind: SourceKindDirectory}, Dimension{Number: "01", Slug: "structure"}), validSourceReport)
	writeReport(t, SourceReportPath(st, Source{Name: "doc.md", Kind: SourceKindMarkdown}, Dimension{Number: "01", Slug: "structure"}), validMarkdownReport)
	rt := &fakeRuntime{result: runtimepkg.Result{RunID: "run-s", Status: "completed"}, write: validFinalReport}
	service := NewService(root, WithRuntime(rt, runtimeRequest()))
	result, err := service.Synthesize(context.Background(), SynthesisRequest{StudyRef: "demo", DimensionRef: "01"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != ExecutionStatusCompleted || result.Validation.Status != ValidationStatusPassed {
		t.Fatalf("result = %+v", result)
	}
	if len(rt.requests) != 1 || rt.requests[0].WorkDir != st.Path || rt.requests[0].Metadata["task.kind"] != "synthesis" {
		t.Fatalf("request = %+v", rt.requests)
	}

	os.Remove(SourceReportPath(st, Source{Name: "repo", Kind: SourceKindDirectory}, Dimension{Number: "01", Slug: "structure"}))
	rt = &fakeRuntime{}
	service = NewService(root, WithRuntime(rt, runtimeRequest()))
	result, err = service.Synthesize(context.Background(), SynthesisRequest{StudyRef: "demo", DimensionRef: "01"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != ExecutionStatusPreflightBlocked || len(rt.requests) != 0 || len(result.Blockers) == 0 {
		t.Fatalf("blocked result = %+v calls = %d", result, len(rt.requests))
	}

	writeReport(t, SourceReportPath(st, Source{Name: "repo", Kind: SourceKindDirectory}, Dimension{Number: "01", Slug: "structure"}), validSourceReport)
	rt = &fakeRuntime{result: runtimepkg.Result{RunID: "run-s2", Status: "completed"}, write: "# Invalid final\n"}
	service = NewService(root, WithRuntime(rt, runtimeRequest()))
	result, err = service.Synthesize(context.Background(), SynthesisRequest{StudyRef: "demo", DimensionRef: "01"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != ExecutionStatusValidationFailed || !hasCheck(result.Validation.Checks, "section.sources_table", ValidationStatusFailed) {
		t.Fatalf("invalid final result = %+v", result)
	}
}

func TestSynthesizePreservesRuntimeFailureCause(t *testing.T) {
	root, st := executionFixture(t)
	writeReport(t, SourceReportPath(st, Source{Name: "repo", Kind: SourceKindDirectory}, Dimension{Number: "01", Slug: "structure"}), validSourceReport)
	writeReport(t, SourceReportPath(st, Source{Name: "doc.md", Kind: SourceKindMarkdown}, Dimension{Number: "01", Slug: "structure"}), validMarkdownReport)
	cause := errors.New("runtime unavailable")
	rt := &fakeRuntime{result: runtimepkg.Result{RunID: "run-s", Status: "failed", Error: &runtimepkg.Error{Category: "runtime"}}, err: cause}
	service := NewService(root, WithRuntime(rt, runtimeRequest()))

	result, err := service.Synthesize(context.Background(), SynthesisRequest{StudyRef: "demo", DimensionRef: "01"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != ExecutionStatusRuntimeFailed || !errors.Is(result.RuntimeErr, cause) {
		t.Fatalf("result = %+v", result)
	}
}

func TestSynthesizeRecoversCleanRuntimeExitWhenFinalReportValidates(t *testing.T) {
	root, st := executionFixture(t)
	writeReport(t, SourceReportPath(st, Source{Name: "repo", Kind: SourceKindDirectory}, Dimension{Number: "01", Slug: "structure"}), validSourceReport)
	writeReport(t, SourceReportPath(st, Source{Name: "doc.md", Kind: SourceKindMarkdown}, Dimension{Number: "01", Slug: "structure"}), validMarkdownReport)
	cause := errors.New("missing final event")
	rt := &fakeRuntime{
		result: runtimepkg.Result{RunID: "run-s", Status: "failed", Error: &runtimepkg.Error{Category: "runtime_exit"}},
		err:    cause,
		write:  validFinalReport,
	}
	service := NewService(root, WithRuntime(rt, runtimeRequest()))

	result, err := service.Synthesize(context.Background(), SynthesisRequest{StudyRef: "demo", DimensionRef: "01"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != ExecutionStatusCompleted || !errors.Is(result.RuntimeErr, cause) || result.Validation.Status != ValidationStatusPassed {
		t.Fatalf("result = %+v", result)
	}
}

func executionFixture(t *testing.T) (string, Study) {
	t.Helper()
	root := t.TempDir()
	for _, rel := range []string{"prompts", "templates", "studies/demo/dimensions", "studies/demo/sources/repo", "studies/demo/reports/source", "studies/demo/reports/final"} {
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(rel)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeReport(t, filepath.Join(root, "prompts", "base.md"), "# Base Prompt\n")
	writeReport(t, filepath.Join(root, "prompts", "synthesize.md"), "# Synthesis Prompt\n")
	writeReport(t, filepath.Join(root, "templates", "repo-analysis.md"), "# Repository Analysis\n")
	writeReport(t, filepath.Join(root, "templates", "report.md"), "# Report\n")
	writeReport(t, filepath.Join(root, "studies", "demo", "dimensions", "01-structure.md"), "# Structure\n")
	writeReport(t, filepath.Join(root, "studies", "demo", "sources", "doc.md"), "---\napplicable_dimensions: [1]\n---\n# Doc\n")
	writeReport(t, filepath.Join(root, "studies", "demo", "sources", "other.md"), "---\napplicable_dimensions: [2]\n---\n# Other\n")
	return root, Study{Name: "demo", Path: filepath.Join(root, "studies", "demo")}
}

func runtimeRequest() runtimepkg.Request {
	return runtimepkg.Request{
		Provider:      "anthropic",
		Model:         "claude",
		Timeout:       time.Minute,
		RequireHealth: []string{"runtime_available"},
		RequireCaps:   []string{"structured_events"},
		Permissions:   "restricted",
		Policy:        runtimepkg.PermissionPolicy{Default: "ask"},
	}
}

const validSourceReport = `# Report

## Source Information

## Summary

## Rating

Rating: 8

## Questions and Answers

- Q: one?
  A: answer

code.go:42
`

const validMarkdownReport = `# Report

## Source Information

## Summary

## Rating

Rating: 8

## Questions and Answers

- Q: one?
  A: answer
`

const validFinalReport = `# Final Report

## Study Parameters

## Sources Studied

| Source | Path |
| --- | --- |
| repo | source |

## Executive Summary

## Rating Summary

## Pattern Synthesis

## Open Questions
`
