package study

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Antonio7098/agentwrap"

	runtimepkg "github.com/Antonio7098/ultraplan-go/internal/platform/runtime"
)

func (s Service) RunAnalysis(ctx context.Context, req ExecutionRequest) (ExecutionResult, error) {
	listing, err := s.ListStudy(req.StudyRef)
	if err != nil {
		return ExecutionResult{}, err
	}
	dimension, err := ResolveDimension(listing.Dimensions, req.DimensionRef)
	if err != nil {
		return ExecutionResult{}, err
	}
	source, err := ResolveSource(listing.Sources, req.SourceRef)
	if err != nil {
		return ExecutionResult{}, err
	}
	result := ExecutionResult{
		Status:     ExecutionStatusCompleted,
		TaskKind:   TaskKindAnalysis,
		Study:      listing.Study,
		Dimension:  dimension,
		Source:     source,
		OutputPath: SourceReportPath(listing.Study, source, dimension),
	}
	if !SourceAppliesToDimension(source, dimension) {
		result.Status = ExecutionStatusSkipped
		result.SkippedReason = fmt.Sprintf("source %q does not apply to dimension %s", source.Name, dimension.Ref())
		return result, nil
	}
	prompt, err := BuildAnalysisPrompt(PromptRequest{WorkspaceRoot: s.workspaceRoot, Study: listing.Study, Dimension: dimension, Source: source})
	if err != nil {
		return ExecutionResult{}, err
	}
	workDir := listing.Study.Path
	beforeFiles, snapshotErr := snapshotFiles(listing.Study.Path)
	runtimeResult, runErr := s.startRuntime(ctx, prompt, TaskKindAnalysis, listing.Study, dimension, source, workDir, result.OutputPath)
	if snapshotErr != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("edit monitoring skipped before runtime: %v", snapshotErr))
	} else if afterFiles, err := snapshotFilesSettled(listing.Study.Path); err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("edit monitoring skipped after runtime: %v", err))
	} else {
		result.Warnings = append(result.Warnings, unexpectedEditWarnings(listing.Study.Path, beforeFiles, afterFiles, []string{result.OutputPath})...)
	}
	result.RuntimeRunID = runtimeResult.RunID
	result.RuntimeStatus = runtimeResult.Status
	result.Agent = agentMetadata(runtimeResult, s.runtimeConfig)
	if runErr != nil {
		result.RuntimeError = runErr.Error()
		result.RuntimeErr = runErr
		if runtimeResult.Error != nil {
			result.RuntimeCategory = runtimeResult.Error.Category
		}
		result.Validation = ValidateSourceReport(listing.Study, source, dimension)
		if result.Validation.Status == ValidationStatusPassed {
			result.Status = ExecutionStatusCompleted
			return result, nil
		}
		if recoverableRuntimeOutputFailure(runtimeResult) {
			result.Status = ExecutionStatusValidationFailed
			return result, nil
		}
		result.Status = statusForRuntimeFailure(runtimeResult)
		return result, nil
	}
	result.Validation = ValidateSourceReport(listing.Study, source, dimension)
	if result.Validation.Status != ValidationStatusPassed {
		result.Status = ExecutionStatusValidationFailed
	}
	return result, nil
}

func (s Service) startRuntime(ctx context.Context, prompt PromptResult, kind TaskKind, study Study, dimension Dimension, source Source, workDir, outputPath string) (runtimepkg.Result, error) {
	if s.runtime == nil {
		return runtimepkg.Result{}, fmt.Errorf("runtime is required")
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return runtimepkg.Result{}, fmt.Errorf("create output directory: %w", err)
	}
	req := s.runtimeConfig
	req.Prompt = prompt.Text
	req.WorkDir = workDir
	req = withStudyRuntimeIsolation(req)
	req.Metadata = executionMetadata(req, kind, study, dimension, source, outputPath)
	req.Validation = &agentwrap.ValidationSpec{Expectations: []agentwrap.ValidationExpectation{{
		ID:       "expected_output",
		Kind:     agentwrap.ExpectationFile,
		Severity: agentwrap.ExpectationRequired,
		Path:     outputPath,
	}}}
	return s.runtime.StartRun(ctx, req)
}

func withStudyRuntimeIsolation(req runtimepkg.Request) runtimepkg.Request {
	if req.Policy.Tools == nil {
		req.Policy.Tools = map[string]string{}
	}
	req.Policy.Tools["external_directory"] = "deny"
	return req
}

func executionMetadata(req runtimepkg.Request, kind TaskKind, study Study, dimension Dimension, source Source, outputPath string) map[string]string {
	meta := map[string]string{
		"task.kind":        string(kind),
		"study":            study.Name,
		"dimension.number": dimension.Number,
		"dimension.slug":   dimension.Slug,
		"dimension.ref":    dimension.Ref(),
		"output.path":      outputPath,
		"runtime.provider": req.Provider,
		"runtime.model":    req.Model,
	}
	if source.Name != "" {
		meta["source.name"] = source.Name
		meta["source.kind"] = string(source.Kind)
	}
	if req.Permissions != "" {
		meta["runtime.permissions"] = req.Permissions
	}
	return meta
}

func statusForRuntimeFailure(result runtimepkg.Result) ExecutionStatus {
	if result.Status == "cancelled" || (result.Error != nil && result.Error.Category == "cancellation") {
		return ExecutionStatusCancelled
	}
	return ExecutionStatusRuntimeFailed
}

func recoverableRuntimeOutputFailure(result runtimepkg.Result) bool {
	return result.Error != nil && result.Error.Category == "runtime_exit"
}
