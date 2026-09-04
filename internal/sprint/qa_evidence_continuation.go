package sprint

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	pprocess "github.com/Antonio7098/ultraplan-go/internal/platform/process"
	pruntime "github.com/Antonio7098/ultraplan-go/internal/platform/runtime"
)

// continueQAInvestigatorForEvidence grants one bounded write turn to the
// original investigator session in its original private workspace. It returns
// only product-snapshotted test files; agent output is never treated as proof.
func (s Service) continueQAInvestigatorForEvidence(ctx context.Context, qaMap QAMap, shard QAShard, initial pruntime.Request, original QAInvestigatorAttempt, evidenceRequest QAArbiterEvidenceRequest, spec QAReproductionSpec, previous *QAReproductionRun, round int) (pruntime.Result, []QATestFile, QAInvestigatorAttempt, error) {
	if round < 1 || round > qaMap.Budgets.EvidenceRoundsPerShard {
		return pruntime.Result{}, nil, QAInvestigatorAttempt{}, NewQAError(QAErrorBudgetExhausted, "continue investigator for evidence", "evidence round budget is exhausted", nil)
	}
	if evidenceRequest.OriginShardID != shard.ID || spec.ShardID != shard.ID || spec.AttemptID != qaMap.SemanticAttemptID {
		return pruntime.Result{}, nil, QAInvestigatorAttempt{}, NewQAError(QAErrorInvalidState, "continue investigator for evidence", "evidence request is routed to the wrong shard", nil)
	}
	workspace := qaInvestigatorWorkspacePath(s.root, qaMap.SemanticAttemptID, shard.ID)
	if err := validateRetainedRuntimeIdentity(original, initial.Provider, initial.Model, initial.Metadata["variant"], initial.RuntimeStorePath, hashOpaque(workspace), original.SessionID); err != nil {
		return pruntime.Result{}, nil, QAInvestigatorAttempt{}, NewQAError(QAErrorRuntimeUnavailable, "continue investigator for evidence", "original_session_unavailable", err)
	}
	if info, err := os.Lstat(workspace); err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return pruntime.Result{}, nil, QAInvestigatorAttempt{}, NewQAError(QAErrorRuntimeUnavailable, "continue investigator for evidence", "original_session_unavailable", err)
	}
	limits := pprocess.IsolationLimits{MaxFiles: qaMap.Budgets.TreeFiles, MaxBytes: qaMap.Budgets.TreeBytes, MaxFileSize: qaMap.Budgets.FileBytes, Timeout: qaMap.Budgets.AuthoringWallTime}
	snapshotParent, err := os.MkdirTemp("", "ultraplan-qa-authoring-snapshot-")
	if err != nil {
		return pruntime.Result{}, nil, QAInvestigatorAttempt{}, err
	}
	defer os.RemoveAll(snapshotParent)
	snapshot, err := pprocess.CreateIsolation(ctx, pprocess.IsolationRequest{SourceRoot: workspace, ParentDir: snapshotParent, Prefix: shard.ID, ProtectedRoots: []string{s.root}, Limits: limits})
	if err != nil {
		return pruntime.Result{}, nil, QAInvestigatorAttempt{}, NewQAError(QAErrorPermissionDenied, "continue investigator for evidence", "cannot snapshot the private investigator workspace", err)
	}
	defer snapshot.Cleanup()
	packet := struct {
		SchemaVersion int                      `json:"schema_version"`
		Round         int                      `json:"round"`
		Request       QAArbiterEvidenceRequest `json:"evidence_request"`
		Theories      []QATheory               `json:"theories"`
		Spec          QAReproductionSpec       `json:"reproduction_spec"`
		Previous      *QAReproductionRun       `json:"previous_run,omitempty"`
		Remaining     struct {
			Rounds       int    `json:"rounds"`
			Files        int    `json:"files"`
			Bytes        int    `json:"bytes"`
			Commands     int    `json:"commands"`
			RuntimeTurns int    `json:"runtime_turns"`
			WallTime     string `json:"wall_time"`
		} `json:"remaining"`
	}{SchemaVersion: QAEvidenceSchemaVersion, Round: round, Request: evidenceRequest, Theories: append([]QATheory(nil), shard.Theories...), Spec: spec, Previous: previous}
	packet.Remaining.Rounds = qaMap.Budgets.EvidenceRoundsPerShard - round + 1
	packet.Remaining.Files = qaMap.Budgets.AuthoredTestFiles
	packet.Remaining.Bytes = qaMap.Budgets.AuthoredTestBytes
	packet.Remaining.Commands = qaMap.Budgets.TestCommandsPerRound
	packet.Remaining.RuntimeTurns = qaMap.Budgets.AuthoringRuntimeTurns
	packet.Remaining.WallTime = qaMap.Budgets.AuthoringWallTime.String()
	data, err := canonicalQAJSON(packet)
	if err != nil {
		return pruntime.Result{}, nil, QAInvestigatorAttempt{}, err
	}
	prompt := "The arbiter needs stronger executable evidence. Continue your original investigation in the same private workspace. Create or strengthen only the approved _test.go files. The failing test output must contain the reproduction_spec.predicted_failure.test_name, assertion, and exact single-line output_matcher so UltraPlan can distinguish the predicted defect from unrelated failures. Include a passing control in the same test. Do not modify product source, repository control files, generated binaries, or any other path. Do not classify the result or approve the test. UltraPlan will snapshot and execute it independently.\n\n" + string(data) + "\n"
	if len(prompt) > qaMap.Budgets.PromptBytes {
		return pruntime.Result{}, nil, QAInvestigatorAttempt{}, NewQAError(QAErrorBudgetExhausted, "continue investigator for evidence", "evidence continuation prompt exceeds its frozen limit", nil)
	}
	req := initial
	req.Prompt, req.SessionID, req.SessionAction, req.Cache = prompt, original.SessionID, "continue", pruntime.CacheDirective{}
	req.WorkDir, req.RuntimeStorePath = workspace, original.RuntimeStoreRef
	req.Timeout, req.Sandbox, req.Permissions = qaMap.Budgets.AuthoringWallTime, "workspace_write", "restricted"
	req.Metadata = cloneMetadata(initial.Metadata, map[string]string{"operation": "qa-investigate-evidence-continuation", "evidence_request": evidenceRequest.ID, "evidence_round": fmt.Sprintf("%d", round)})
	req.Policy = pruntime.PermissionPolicy{Default: "deny", Tools: map[string]string{"read": "allow", "list": "allow", "search": "allow", "glob": "allow", "write": "allow", "edit": "allow", "patch": "allow", "bash": "deny", "shell": "deny"}}
	for _, rel := range spec.ApprovedTestPaths {
		path := filepath.Join(workspace, filepath.FromSlash(rel))
		if !inside(workspace, path) {
			return pruntime.Result{}, nil, QAInvestigatorAttempt{}, NewQAError(QAErrorPermissionDenied, "continue investigator for evidence", "approved test path escapes the private workspace", nil)
		}
		req.Policy.PathRules = append(req.Policy.PathRules, pruntime.PermissionPathRule{Path: path, Action: "allow"})
	}
	sort.Slice(req.Policy.PathRules, func(i, j int) bool { return req.Policy.PathRules[i].Path < req.Policy.PathRules[j].Path })
	started := s.now().UTC()
	result, runErr := s.startQARuntime(ctx, qaMap, req)
	completed := s.now().UTC()
	attempt := QAInvestigatorAttempt{ID: fmt.Sprintf("%s/evidence/%d", original.ID, round), Number: round + 1, SessionID: result.SessionID, Provider: req.Provider, Model: req.Model, Variant: req.Metadata["variant"], RuntimeStoreRef: result.RuntimeStorePath, WorkspaceID: hashOpaque(workspace), StartedAt: started, CompletedAt: &completed, ImplementationBefore: spec.ImplementationFingerprint, ImplementationAfter: spec.ImplementationFingerprint, Usage: qaUsageSummary(result.Usage), RuntimeEvents: result.EventStats.Total, RetainedEvents: len(result.Events), ObservedToolCalls: qaObservedToolCalls(result.Events), ContextMetrics: qaAttemptContextMetrics(req, result.Events, completed.Sub(started))}
	if attempt.RuntimeStoreRef == "" {
		attempt.RuntimeStoreRef = req.RuntimeStorePath
	}
	if runErr != nil || result.SessionID != original.SessionID || result.RuntimeStorePath != "" && result.RuntimeStorePath != original.RuntimeStoreRef || !qaRuntimePermissionsRestricted(result) {
		attempt.FailureKind, attempt.Retryable, attempt.StopReason = "original_session_unavailable", false, "original_session_unavailable"
		return result, nil, attempt, NewQAError(QAErrorRuntimeUnavailable, "continue investigator for evidence", "original_session_unavailable", runErr)
	}
	if result.Usage.TurnsKnown && result.Usage.Turns > int64(qaMap.Budgets.AuthoringRuntimeTurns) {
		attempt.StopReason = "authoring runtime turn budget exhausted"
		return result, nil, attempt, NewQAError(QAErrorBudgetExhausted, "continue investigator for evidence", attempt.StopReason, nil)
	}
	changed, err := pprocess.CompareTrees(context.WithoutCancel(ctx), snapshot.Path, workspace, limits)
	if err != nil {
		attempt.StopReason = "workspace snapshot comparison failed"
		return result, nil, attempt, NewQAError(QAErrorPermissionDenied, "continue investigator for evidence", attempt.StopReason, err)
	}
	if len(changed) == 0 {
		correction := req
		correction.Prompt = "No approved test file was created in the previous turn. This evidence request cannot be completed with prose alone. Use the available write or edit tool now to create exactly one approved _test.go path from the frozen reproduction spec. The failing assertion must emit the exact frozen test_name, assertion, and output_matcher, and the test must include a passing control. Do not modify any other path.\n"
		correction.SessionID, correction.SessionAction, correction.Cache = result.SessionID, "continue", pruntime.CacheDirective{}
		correction.Metadata = cloneMetadata(req.Metadata, map[string]string{"authoring_correction": "missing_test_file", "repair_of": result.RunID})
		corrected, correctionErr := s.startQARuntime(ctx, qaMap, correction)
		completed = s.now().UTC()
		attempt.CompletedAt = &completed
		attempt.RuntimeEvents += corrected.EventStats.Total
		attempt.RetainedEvents += len(corrected.Events)
		attempt.ObservedToolCalls += qaObservedToolCalls(corrected.Events)
		attempt.Usage = addQAUsageSummaries(attempt.Usage, qaUsageSummary(corrected.Usage))
		if correctionErr != nil || corrected.SessionID != original.SessionID || corrected.RuntimeStorePath != "" && corrected.RuntimeStorePath != original.RuntimeStoreRef || !qaRuntimePermissionsRestricted(corrected) {
			attempt.FailureKind, attempt.Retryable, attempt.StopReason = "original_session_unavailable", false, "original_session_unavailable"
			return corrected, nil, attempt, NewQAError(QAErrorRuntimeUnavailable, "continue investigator for evidence", "original_session_unavailable", correctionErr)
		}
		result = corrected
		changed, err = pprocess.CompareTrees(context.WithoutCancel(ctx), snapshot.Path, workspace, limits)
		if err != nil {
			attempt.StopReason = "workspace snapshot comparison failed"
			return result, nil, attempt, NewQAError(QAErrorPermissionDenied, "continue investigator for evidence", attempt.StopReason, err)
		}
	}
	approved := map[string]bool{}
	for _, path := range spec.ApprovedTestPaths {
		approved[filepath.ToSlash(filepath.Clean(filepath.FromSlash(path)))] = true
	}
	for _, path := range changed {
		if !approved[path] || !strings.HasSuffix(path, "_test.go") {
			attempt.StopReason = "test authoring changed a non-approved path"
			return result, nil, attempt, NewQAError(QAErrorPermissionDenied, "continue investigator for evidence", attempt.StopReason, nil)
		}
	}
	files := make([]QATestFile, 0, len(changed))
	total := 0
	for _, path := range changed {
		full := filepath.Join(workspace, filepath.FromSlash(path))
		info, statErr := os.Lstat(full)
		if statErr != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return result, nil, attempt, NewQAError(QAErrorPermissionDenied, "continue investigator for evidence", "authored test is not a regular file", statErr)
		}
		content, readErr := os.ReadFile(full)
		if readErr != nil {
			return result, nil, attempt, readErr
		}
		total += len(content)
		files = append(files, QATestFile{Path: path, Content: string(content)})
	}
	if len(files) == 0 {
		attempt.StopReason = "investigator did not create an approved test file"
		return result, nil, attempt, NewQAError(QAErrorMalformedEvidence, "continue investigator for evidence", attempt.StopReason, nil)
	}
	if len(files) > qaMap.Budgets.AuthoredTestFiles || total > qaMap.Budgets.AuthoredTestBytes {
		attempt.StopReason = "authored test file or byte budget exhausted"
		return result, nil, attempt, NewQAError(QAErrorBudgetExhausted, "continue investigator for evidence", attempt.StopReason, nil)
	}
	attempt.StopReason = "investigator-authored test snapshotted"
	return result, files, attempt, nil
}

func addQAUsageSummaries(left, right QAUsageSummary) QAUsageSummary {
	return QAUsageSummary{
		InputTokensKnown: left.InputTokensKnown && right.InputTokensKnown, InputTokens: left.InputTokens + right.InputTokens,
		OutputTokensKnown: left.OutputTokensKnown && right.OutputTokensKnown, OutputTokens: left.OutputTokens + right.OutputTokens,
		TotalTokensKnown: left.TotalTokensKnown && right.TotalTokensKnown, TotalTokens: left.TotalTokens + right.TotalTokens,
		ReasoningTokensKnown: left.ReasoningTokensKnown && right.ReasoningTokensKnown, ReasoningTokens: left.ReasoningTokens + right.ReasoningTokens,
		CacheReadTokensKnown: left.CacheReadTokensKnown && right.CacheReadTokensKnown, CacheReadTokens: left.CacheReadTokens + right.CacheReadTokens,
		CacheWriteTokensKnown: left.CacheWriteTokensKnown && right.CacheWriteTokensKnown, CacheWriteTokens: left.CacheWriteTokens + right.CacheWriteTokens,
		TurnsKnown: left.TurnsKnown && right.TurnsKnown, Turns: left.Turns + right.Turns,
	}
}
