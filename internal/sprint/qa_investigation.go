package sprint

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"sort"
	"strings"
	"time"

	pprocess "github.com/Antonio7098/ultraplan-go/internal/platform/process"
)

type QAInvestigationRequest struct {
	Project          string
	Sprint           string
	TargetRoot       string
	WorkspaceParent  string
	ProtectedRoots   []string
	Plan             QAEvidencePlan
	Budgets          QABudgets
	Environment      map[string]string
	Runner           pprocess.Runner
	ExpectedTargetID string
	Now              func() time.Time
}

// RunQAInvestigation executes one already frozen plan in one disposable copy.
// It deliberately blocks when the host cannot prove native protected-root
// denial. A runtime policy or prompt is not accepted as a substitute.
func RunQAInvestigation(ctx context.Context, req QAInvestigationRequest) (QAEvidenceRecord, error) {
	if err := ValidateQAEvidencePlan(req.Plan, req.Budgets); err != nil {
		return QAEvidenceRecord{}, NewQAError(QAErrorMalformedEvidence, "investigate", err.Error(), err)
	}
	if req.Plan.AttemptID == "" || req.ExpectedTargetID == "" || !validFingerprint(req.ExpectedTargetID) {
		return QAEvidenceRecord{}, NewQAError(QAErrorStaleInput, "investigate", "a current target identity is required", nil)
	}
	if req.Runner == nil {
		req.Runner = pprocess.DirectRunner{}
	}
	if req.Now == nil {
		req.Now = func() time.Time { return time.Now().UTC() }
	}
	limits := pprocess.IsolationLimits{MaxFiles: req.Budgets.TreeFiles, MaxBytes: req.Budgets.TreeBytes, MaxFileSize: req.Budgets.FileBytes, Timeout: req.Budgets.ShardTimeout}
	targetBefore, err := pprocess.IdentifyTree(ctx, req.TargetRoot, limits)
	if err != nil {
		return QAEvidenceRecord{}, NewQAError(QAErrorPermissionDenied, "investigate", "cannot identify the protected target", err)
	}
	if targetBefore.Digest != req.ExpectedTargetID {
		return QAEvidenceRecord{}, NewQAError(QAErrorStaleInput, "investigate", "protected target identity does not match the frozen plan", nil)
	}
	workspace, err := pprocess.CreateIsolation(ctx, pprocess.IsolationRequest{SourceRoot: req.TargetRoot, ParentDir: req.WorkspaceParent, Prefix: req.Plan.ShardID, ProtectedRoots: req.ProtectedRoots, Limits: limits})
	if err != nil {
		return QAEvidenceRecord{}, NewQAError(QAErrorPermissionDenied, "investigate", "cannot create a contained QA workspace", err)
	}
	cleanup := QACleanupFacts{Attempted: true}
	defer func() {
		if !cleanup.Complete {
			_ = workspace.Cleanup()
		}
	}()
	if workspace.Source.Digest != targetBefore.Digest {
		result := workspace.Cleanup()
		cleanup.WorkspaceRemoved, cleanup.Complete, cleanup.Diagnostic = result.Complete, result.Complete, result.Error
		return QAEvidenceRecord{}, NewQAError(QAErrorStaleInput, "investigate", "protected target changed while creating the isolated workspace", nil)
	}
	capabilities := workspace.Capabilities
	if !capabilities.PrivateWorkspace || !capabilities.ContainedCopy || !capabilities.ProcessGroup || !capabilities.DescendantCleanup || !capabilities.WorkspaceRemoval || !capabilities.NativeProtectedRootDeny {
		result := workspace.Cleanup()
		cleanup.DescendantsTerminated = capabilities.DescendantCleanup
		cleanup.WorkspaceRemoved, cleanup.Complete, cleanup.Diagnostic = result.Complete, result.Complete, result.Error
		return QAEvidenceRecord{}, NewQAError(QAErrorAdmissionBlocked, "investigate", "host isolation cannot prove protected-root denial and descendant cleanup", nil)
	}
	if strings.Contains(strings.Join(append(append([]string(nil), req.Plan.Args...), valuesOnly(req.Environment)...), "\x00"), req.TargetRoot) {
		result := workspace.Cleanup()
		cleanup.WorkspaceRemoved, cleanup.Complete, cleanup.Diagnostic = result.Complete, result.Complete, result.Error
		return QAEvidenceRecord{}, NewQAError(QAErrorPermissionDenied, "investigate", "the original target path leaked into a child request", nil)
	}

	before, err := workspace.Identity(ctx, limits)
	if err != nil {
		return QAEvidenceRecord{}, NewQAError(QAErrorPermissionDenied, "investigate", "cannot identify isolated workspace", err)
	}
	commandResults := make([]QACommandResult, 0, 1)
	outcome := QAEvidencePass
	reason := "check_passed"
	if req.Plan.Executable != "" {
		started := req.Now().UTC()
		result, runErr := workspace.Run(ctx, req.Runner, ".", pprocess.Request{Executable: req.Plan.Executable, Args: append([]string(nil), req.Plan.Args...), Env: pprocess.SortedEnvironment(req.Environment), Timeout: req.Plan.Timeout, StdoutLimit: req.Plan.OutputLimit, StderrLimit: req.Plan.OutputLimit, CleanupGrace: req.Budgets.CleanupTimeout})
		argsDigest := sha256.Sum256([]byte(strings.Join(req.Plan.Args, "\x00")))
		stdoutDigest := sha256.Sum256([]byte(result.Stdout))
		stderrDigest := sha256.Sum256([]byte(result.Stderr))
		commandResults = append(commandResults, QACommandResult{Executable: filepath.Base(req.Plan.Executable), ArgsDigest: hex.EncodeToString(argsDigest[:]), ExitCode: result.ExitCode, Duration: req.Now().UTC().Sub(started), StdoutDigest: hex.EncodeToString(stdoutDigest[:]), StderrDigest: hex.EncodeToString(stderrDigest[:]), OutputBytes: len(result.Stdout) + len(result.Stderr), Truncated: result.StdoutTruncated || result.StderrTruncated, TimedOut: result.TimedOut, Cancelled: result.Cancelled, CleanupAttempted: result.CleanupAttempted, CleanupComplete: result.CleanupComplete})
		if runErr != nil {
			outcome, reason = QAEvidenceFail, "command_failed"
			if result.Cancelled || result.TimedOut || !result.CleanupComplete {
				outcome, reason = QAEvidenceBlocked, "command_incomplete"
			}
		}
		if result.StdoutTruncated || result.StderrTruncated {
			outcome, reason = QAEvidenceBlocked, "output_truncated"
		}
	}
	after, identityErr := workspace.Identity(context.WithoutCancel(ctx), limits)
	if identityErr != nil {
		outcome, reason = QAEvidenceBlocked, "workspace_identity_failed"
	}
	changedPaths, changesErr := workspace.ChangedPaths(context.WithoutCancel(ctx), limits)
	if changesErr != nil {
		outcome, reason = QAEvidenceBlocked, "workspace_changes_incomplete"
	}
	for _, path := range changedPaths {
		if !qaPathApproved(path, req.Plan.ApprovedPaths) {
			outcome, reason = QAEvidenceBlocked, "path_not_approved"
		}
	}
	cleanupResult := workspace.Cleanup()
	cleanup = QACleanupFacts{Attempted: true, DescendantsTerminated: true, WorkspaceRemoved: cleanupResult.Complete, Complete: cleanupResult.Complete, Diagnostic: cleanupResult.Error}
	if !cleanup.Complete {
		outcome, reason = QAEvidenceBlocked, "cleanup_uncertain"
	}
	targetAfter, targetErr := pprocess.IdentifyTree(context.WithoutCancel(ctx), req.TargetRoot, limits)
	if targetErr != nil || targetAfter.Digest != targetBefore.Digest {
		outcome, reason = QAEvidenceBlocked, "target_drift"
	}
	identity := before.Digest
	if after.Digest != "" {
		identity = after.Digest
	}
	id, idErr := NewQAV2ID("evidence", req.Project, req.Sprint, req.Plan.ID, struct {
		Workspace string
		Commands  []QACommandResult
		Changes   []string
		Outcome   QAEvidenceOutcome
	}{workspace.Path, commandResults, changedPaths, outcome})
	if idErr != nil {
		return QAEvidenceRecord{}, idErr
	}
	targetAfterDigest := ""
	if targetErr == nil {
		targetAfterDigest = targetAfter.Digest
	}
	return QAEvidenceRecord{SchemaVersion: QAEvidenceSchemaVersion, ID: id, PlanID: req.Plan.ID, AttemptID: req.Plan.AttemptID, ShardID: req.Plan.ShardID, WorkspaceID: hashOpaque(workspace.Path), WorkspaceIdentity: identity, TargetIdentityBefore: targetBefore.Digest, TargetIdentityAfter: targetAfterDigest, GovernedInputFingerprint: req.Plan.GovernedInputFingerprint, ImplementationFingerprint: req.Plan.ImplementationFingerprint, MapFingerprint: req.Plan.MapFingerprint, Commands: commandResults, ChangedPaths: changedPaths, Outcome: outcome, ReasonCode: reason, Repeatable: outcome == QAEvidencePass, Contained: true, Cleanup: cleanup, CompletedAt: req.Now().UTC()}, nil
}

func FreezeQAEvidencePlan(project, sprint string, plan QAEvidencePlan, budgets QABudgets, now time.Time) (QAEvidencePlan, error) {
	plan.SchemaVersion = QAEvidenceSchemaVersion
	plan.TheoryIDs = normalizeQAStrings(plan.TheoryIDs)
	plan.ExpectationRefs = normalizeQAStrings(plan.ExpectationRefs)
	plan.ApprovedPaths = normalizeQAStrings(plan.ApprovedPaths)
	plan.EnvironmentNames = normalizeQAStrings(plan.EnvironmentNames)
	plan.FrozenAt = now.UTC()
	id, err := NewQAV2ID("plan", project, sprint, plan.ShardID, struct {
		Kind                QACheckKind
		TheoryIDs           []string
		Expectations        []string
		Conditions          []string
		Paths               []string
		Executable          string
		Args                []string
		Environment         []string
		Timeout             time.Duration
		Output              int
		Analyzers           int
		Governed, Impl, Map string
	}{plan.Kind, plan.TheoryIDs, plan.ExpectationRefs, []string{plan.ConfirmationCondition, plan.RefutationCondition, plan.InconclusiveCondition}, plan.ApprovedPaths, plan.Executable, plan.Args, plan.EnvironmentNames, plan.Timeout, plan.OutputLimit, plan.AnalyzerCalls, plan.GovernedInputFingerprint, plan.ImplementationFingerprint, plan.MapFingerprint})
	if err != nil {
		return QAEvidencePlan{}, err
	}
	plan.ID = id
	if err := ValidateQAEvidencePlan(plan, budgets); err != nil {
		return QAEvidencePlan{}, err
	}
	return plan, nil
}

func valuesOnly(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		out = append(out, values[key])
	}
	return out
}

func hashOpaque(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}
