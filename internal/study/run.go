package study

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	runtimepkg "github.com/Antonio7098/ultraplan-go/internal/platform/runtime"
	"github.com/Antonio7098/ultraplan-go/internal/workspace"
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
	runtimeResult, runErr := s.startRuntime(ctx, prompt, TaskKindAnalysis, listing.Study, dimension, source, workDir, result.OutputPath, beforeFiles, snapshotErr, req.ResumeSession, req.OnSession, req.OnEvent)
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

func (s Service) startRuntime(ctx context.Context, prompt PromptResult, kind TaskKind, study Study, dimension Dimension, source Source, workDir, outputPath string, inputs fileSnapshot, inputsErr error, resume *TaskSession, onSession func(TaskSession), onEvent func(runtimepkg.Event)) (runtimepkg.Result, error) {
	if s.runtime == nil {
		return runtimepkg.Result{}, fmt.Errorf("runtime is required")
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return runtimepkg.Result{}, fmt.Errorf("create output directory: %w", err)
	}
	req := s.runtimeConfig
	req.WorkDir = workDir
	req = withStudyRuntimeIsolation(req)
	req.Metadata = executionMetadata(req, kind, study, dimension, source, outputPath)
	req.Validation = studyReportValidationSpec(kind, study, source, dimension, outputPath)
	var fingerprint string
	var fingerprintErr error
	var fingerprintOnce sync.Once
	loadFingerprint := func() (string, error) {
		fingerprintOnce.Do(func() {
			fingerprint, fingerprintErr = studySessionFingerprint(s.workspaceRoot, prompt, req, kind, study, dimension, source, outputPath, inputs, inputsErr)
		})
		return fingerprint, fingerprintErr
	}
	continuing := false
	if resume != nil {
		fingerprint, fingerprintErr = loadFingerprint()
		continuing = fingerprintErr == nil && studySessionCompatible(resume, req, fingerprint)
	}
	req.Prompt = prompt.Text
	if continuing {
		req.SessionID = resume.SessionID
		req.SessionAction = "continue"
		req.Prompt = studyContinuationPrompt(req.Prompt)
	}
	checkpoint := func(sessionID string) {
		if onSession == nil || sessionID == "" {
			return
		}
		fingerprint, fingerprintErr := loadFingerprint()
		if fingerprintErr != nil {
			return
		}
		onSession(TaskSession{SessionID: sessionID, Provider: req.Provider, Model: req.Model, WorkDir: req.WorkDir, InputFingerprint: fingerprint, UpdatedAt: time.Now().UTC()})
	}
	req.OnEvent = func(event runtimepkg.Event) {
		if onEvent != nil {
			onEvent(event)
		}
		checkpoint(event.SessionID)
	}
	result, err := s.runtime.StartRun(ctx, req)
	checkpoint(result.SessionID)
	if continuing && studyContinuationNeedsFreshFallback(ctx, result, err) {
		failed := *resume
		failed.ContinueFailures++
		failed.UpdatedAt = time.Now().UTC()
		if onSession != nil {
			onSession(failed)
		}
		req.SessionID = ""
		req.SessionAction = "fresh"
		req.Prompt = prompt.Text
		result, err = s.runtime.StartRun(ctx, req)
		checkpoint(result.SessionID)
	}
	return result, err
}

func studyContinuationNeedsFreshFallback(ctx context.Context, result runtimepkg.Result, err error) bool {
	if err == nil || ctx.Err() != nil {
		return false
	}
	category := ""
	if result.Error != nil {
		category = result.Error.Category
	}
	switch category {
	case "cancellation", "timeout", "permission", "authentication", "rate_limit", "provider_unavailable", "runtime_unavailable", "model_unavailable", "cleanup":
		return false
	default:
		return true
	}
}

func studySessionFingerprint(workspaceRoot string, prompt PromptResult, req runtimepkg.Request, kind TaskKind, study Study, dimension Dimension, source Source, outputPath string, inputs fileSnapshot, inputsErr error) (string, error) {
	inputDigest, err := studyMutableInputDigest(workspaceRoot, prompt, kind, source, inputs, inputsErr)
	value := strings.Join([]string{prompt.Text, inputDigest, req.Provider, req.Model, req.WorkDir, string(kind), study.Name, dimension.Ref(), source.Name, string(source.Kind), outputPath}, "\x00")
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:]), err
}

func studyMutableInputDigest(workspaceRoot string, prompt PromptResult, kind TaskKind, source Source, inputs fileSnapshot, inputsErr error) (string, error) {
	if inputsErr != nil {
		return "input-unavailable", inputsErr
	}
	entries := map[string]string{}
	switch kind {
	case TaskKindAnalysis:
		if source.Kind != SourceKindDirectory {
			break
		}
		for path, digest := range inputs {
			rel, err := filepath.Rel(source.Path, path)
			if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				if err == nil {
					continue
				}
				return "input-unavailable", err
			}
			entries[filepath.ToSlash(rel)] = digest
		}
	case TaskKindSynthesis:
		for _, rel := range prompt.Manifest.InputReportPaths {
			path, err := workspace.ResolveInside(workspaceRoot, rel)
			if err != nil {
				return "input-unavailable", err
			}
			digest, ok := inputs[filepath.Clean(path)]
			if !ok {
				return "input-unavailable", fmt.Errorf("session input missing from snapshot: %s", rel)
			}
			entries[filepath.ToSlash(rel)] = digest
		}
	}
	paths := make([]string, 0, len(entries))
	for path := range entries {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	hash := sha256.New()
	for _, path := range paths {
		fmt.Fprintf(hash, "%s\x00%s\n", path, entries[path])
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func studySessionCompatible(session *TaskSession, req runtimepkg.Request, fingerprint string) bool {
	return session != nil && session.SessionID != "" && session.ContinueFailures == 0 && session.Provider == req.Provider && session.Model == req.Model && session.WorkDir == req.WorkDir && session.InputFingerprint == fingerprint
}

func studyContinuationPrompt(prompt string) string {
	return "Continue the interrupted study task from this session. Re-read the current task prompt and filesystem state. Finish or repair only the requested report. Do not repeat completed investigation unnecessarily.\n\n" + prompt
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
