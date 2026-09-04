package sprint

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Antonio7098/ultraplan-go/internal/platform/config"
	pprocess "github.com/Antonio7098/ultraplan-go/internal/platform/process"
)

type QAReproductionRequest struct {
	Project          string
	Sprint           string
	TargetRoot       string
	WorkspaceParent  string
	ProtectedRoots   []string
	Spec             QAReproductionSpec
	Bundle           QATestBundle
	Budgets          QABudgets
	Runner           pprocess.Runner
	ExpectedTargetID string
	// AllowDifferentImplementation is used only by the explicit `--target current`
	// rerun path. Evidence admission on the original broken implementation keeps
	// this false and must match the immutable spec fingerprint.
	AllowDifferentImplementation bool
	Now                          func() time.Time
}

// RunQAReproduction materializes an immutable authored-test bundle in a fresh
// isolated copy and runs the frozen command without a shell.
func RunQAReproduction(ctx context.Context, req QAReproductionRequest) (QAReproductionRun, error) {
	if err := ValidateQAReproductionSpec(req.Spec, req.Budgets); err != nil {
		return QAReproductionRun{}, NewQAError(QAErrorMalformedEvidence, "run reproduction", err.Error(), err)
	}
	if err := ValidateQATestBundle(req.Bundle, req.Spec, req.Budgets); err != nil {
		return QAReproductionRun{}, NewQAError(QAErrorMalformedEvidence, "run reproduction", err.Error(), err)
	}
	if req.Runner == nil {
		req.Runner = pprocess.DirectRunner{}
	}
	if req.Now == nil {
		req.Now = func() time.Time { return time.Now().UTC() }
	}
	limits := pprocess.IsolationLimits{MaxFiles: req.Budgets.TreeFiles, MaxBytes: req.Budgets.TreeBytes, MaxFileSize: req.Budgets.FileBytes, Timeout: req.Budgets.ShardTimeout}
	targetBefore, err := pprocess.IdentifyTree(ctx, req.TargetRoot, limits)
	currentTargetID, identityErr := targetIdentity(req.TargetRoot)
	if err != nil || identityErr != nil || currentTargetID != req.ExpectedTargetID || !req.AllowDifferentImplementation && currentTargetID != req.Spec.ImplementationFingerprint {
		return QAReproductionRun{}, NewQAError(QAErrorStaleInput, "run reproduction", "target identity does not match the frozen reproduction", err)
	}
	workspace, err := pprocess.CreateIsolation(ctx, pprocess.IsolationRequest{SourceRoot: req.TargetRoot, ParentDir: req.WorkspaceParent, Prefix: req.Bundle.ID, ProtectedRoots: append(req.ProtectedRoots, req.TargetRoot), Limits: limits})
	if err != nil {
		return QAReproductionRun{}, NewQAError(QAErrorPermissionDenied, "run reproduction", "cannot create a contained reproduction workspace", err)
	}
	cleanup := QACleanupFacts{Attempted: true}
	defer func() {
		if !cleanup.Complete {
			_ = workspace.Cleanup()
		}
	}()
	if !workspace.Capabilities.PrivateWorkspace || !workspace.Capabilities.ContainedCopy || !workspace.Capabilities.DescendantCleanup || !workspace.Capabilities.WorkspaceRemoval || !workspace.Capabilities.NativeProtectedRootDeny {
		result := workspace.Cleanup()
		return QAReproductionRun{}, NewQAError(QAErrorAdmissionBlocked, "run reproduction", "host isolation cannot prove required containment", fmt.Errorf("cleanup complete: %t", result.Complete))
	}
	for _, file := range req.Bundle.Files {
		path, resolveErr := workspace.Resolve(file.Path)
		if resolveErr != nil {
			return QAReproductionRun{}, NewQAError(QAErrorPermissionDenied, "run reproduction", "authored test path escapes the workspace", resolveErr)
		}
		if mkdirErr := os.MkdirAll(filepath.Dir(path), 0o700); mkdirErr != nil {
			return QAReproductionRun{}, mkdirErr
		}
		if writeErr := os.WriteFile(path, []byte(file.Content), 0o600); writeErr != nil {
			return QAReproductionRun{}, writeErr
		}
	}
	changed, err := workspace.ChangedPaths(ctx, limits)
	if err != nil {
		return QAReproductionRun{}, NewQAError(QAErrorPermissionDenied, "run reproduction", "cannot verify authored test scope", err)
	}
	for _, path := range changed {
		if !qaPathApproved(path, req.Spec.ApprovedTestPaths) {
			return QAReproductionRun{}, NewQAError(QAErrorPermissionDenied, "run reproduction", "authored test materialization changed a non-test path", nil)
		}
	}
	executable, err := exec.LookPath(req.Spec.Command.Executable)
	if err != nil {
		return QAReproductionRun{}, NewQAError(QAErrorAdmissionBlocked, "run reproduction", "frozen test executable is unavailable", err)
	}
	workdir := req.Spec.Command.WorkingDirectory
	if workdir == "" {
		workdir = "."
	}
	environment := make(map[string]string, len(req.Spec.Command.Environment))
	for _, name := range req.Spec.Command.Environment {
		if strings.ContainsAny(name, "=\x00\r\n") {
			return QAReproductionRun{}, NewQAError(QAErrorPermissionDenied, "run reproduction", "frozen environment name is invalid", nil)
		}
		if value, ok := os.LookupEnv(name); ok {
			environment[name] = value
		}
	}
	runtimeRoot, err := workspace.Resolve(".ultraplan-test-runtime")
	if err != nil {
		return QAReproductionRun{}, err
	}
	for _, path := range []string{runtimeRoot, filepath.Join(runtimeRoot, "cache"), filepath.Join(runtimeRoot, "tmp"), filepath.Join(runtimeRoot, "home"), filepath.Join(runtimeRoot, "gopath")} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			return QAReproductionRun{}, err
		}
	}
	environment["HOME"] = filepath.Join(runtimeRoot, "home")
	environment["GOCACHE"] = filepath.Join(runtimeRoot, "cache")
	environment["GOPATH"] = filepath.Join(runtimeRoot, "gopath")
	environment["GOTMPDIR"] = filepath.Join(runtimeRoot, "tmp")
	environment["TMPDIR"] = filepath.Join(runtimeRoot, "tmp")
	started := req.Now().UTC()
	result, runErr := workspace.Run(ctx, req.Runner, workdir, pprocess.Request{Executable: executable, Args: append([]string(nil), req.Spec.Command.Args...), Env: pprocess.SortedEnvironment(environment), Timeout: req.Spec.Command.Timeout, StdoutLimit: req.Spec.Command.OutputLimit, StderrLimit: req.Spec.Command.OutputLimit, CleanupGrace: req.Budgets.CleanupTimeout})
	argsDigest := sha256.Sum256([]byte(strings.Join(req.Spec.Command.Args, "\x00")))
	stdout, stderr := config.RedactValue("qa.reproduction.stdout", result.Stdout), config.RedactValue("qa.reproduction.stderr", result.Stderr)
	redactions := 0
	if stdout != result.Stdout {
		redactions++
	}
	if stderr != result.Stderr {
		redactions++
	}
	stdoutDigest := sha256.Sum256([]byte(stdout))
	stderrDigest := sha256.Sum256([]byte(stderr))
	command := QACommandResult{Executable: filepath.Base(executable), ArgsDigest: hex.EncodeToString(argsDigest[:]), ExitCode: result.ExitCode, Duration: req.Now().UTC().Sub(started), StdoutDigest: hex.EncodeToString(stdoutDigest[:]), StderrDigest: hex.EncodeToString(stderrDigest[:]), Stdout: stdout, Stderr: stderr, OutputBytes: len(stdout) + len(stderr), Truncated: result.StdoutTruncated || result.StderrTruncated, Redacted: redactions > 0, RedactionCount: redactions, TimedOut: result.TimedOut, Cancelled: result.Cancelled, CleanupAttempted: result.CleanupAttempted, CleanupComplete: result.CleanupComplete}
	if runErr != nil && !command.TimedOut && !command.Cancelled && command.ExitCode == 0 {
		command.ExitCode = 1
	}
	outcome, reason := ClassifyQAReproductionResult(command, req.Spec.PredictedFailure)
	if removeErr := removeQAReproductionRuntime(runtimeRoot); removeErr != nil {
		outcome, reason = QAEvidenceInconclusive, "runtime_cleanup_incomplete"
	}
	postChanges, changeErr := workspace.ChangedPaths(context.WithoutCancel(ctx), limits)
	if changeErr != nil {
		outcome, reason = QAEvidenceInconclusive, "workspace_changes_incomplete"
	}
	for _, path := range postChanges {
		if !qaPathApproved(path, req.Spec.ApprovedTestPaths) {
			outcome, reason = QAEvidenceInconclusive, "source_change_detected"
		}
	}
	cleanupResult := workspace.Cleanup()
	cleanup = QACleanupFacts{Attempted: true, DescendantsTerminated: command.CleanupComplete, WorkspaceRemoved: cleanupResult.Complete, Complete: command.CleanupComplete && cleanupResult.Complete, Diagnostic: cleanupResult.Error}
	if !cleanup.Complete {
		outcome, reason = QAEvidenceInconclusive, "cleanup_uncertain"
	}
	targetAfter, targetErr := pprocess.IdentifyTree(context.WithoutCancel(ctx), req.TargetRoot, limits)
	currentTargetAfter, currentTargetErr := targetIdentity(req.TargetRoot)
	if targetErr != nil || currentTargetErr != nil || targetAfter.Digest != targetBefore.Digest || currentTargetAfter != currentTargetID {
		outcome, reason = QAEvidenceInconclusive, "target_drift"
	}
	completed := req.Now().UTC()
	run := QAReproductionRun{SchemaVersion: QAEvidenceSchemaVersion, SpecID: req.Spec.ID, TestBundleID: req.Bundle.ID, TargetIdentity: currentTargetID, Result: command, Signature: req.Spec.PredictedFailure, Outcome: outcome, ReasonCode: reason, Cleanup: cleanup, CompletedAt: completed}
	run.ID, err = NewQAV2ID("run", req.Project, req.Sprint, req.Bundle.ID, struct {
		Target, Args, Stdout, Stderr, Reason string
		Exit                                 int
		Completed                            time.Time
	}{run.TargetIdentity, command.ArgsDigest, command.StdoutDigest, command.StderrDigest, reason, command.ExitCode, completed})
	if err != nil {
		return QAReproductionRun{}, err
	}
	if err := ValidateQAReproductionRun(run, req.Spec, req.Bundle); err != nil {
		return QAReproductionRun{}, NewQAError(QAErrorMalformedEvidence, "run reproduction", err.Error(), err)
	}
	return run, nil
}

func removeQAReproductionRuntime(root string) error {
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return os.Chmod(path, 0o700)
		}
		return nil
	}); err != nil {
		return err
	}
	return os.RemoveAll(root)
}

func (s Service) RerunQATest(ctx context.Context, projectRef, sprintRef, testID string, token QAWriterToken) (QAReproductionRun, error) {
	if err := token.Validate(); err != nil {
		return QAReproductionRun{}, NewQAError(QAErrorConflict, "rerun authored test", err.Error(), err)
	}
	sp, err := s.resolveMutationSprint(projectRef, sprintRef)
	if err != nil {
		return QAReproductionRun{}, err
	}
	store := NewQAStore(s.root, sp)
	state, err := store.LoadState()
	if err != nil {
		return QAReproductionRun{}, err
	}
	qaMap, err := store.LoadMap(state.CurrentAttemptID)
	if err != nil {
		return QAReproductionRun{}, err
	}
	spec, bundle, err := store.LoadTestBundle(state.CurrentAttemptID, testID, qaMap.Budgets)
	if err != nil {
		return QAReproductionRun{}, err
	}
	manifest, findings, err := s.PrepareReview(projectRef, sprintRef, ReviewRequest{})
	if err != nil || len(findings) > 0 {
		return QAReproductionRun{}, NewQAError(QAErrorStaleInput, "rerun authored test", "current implementation target is unavailable", err)
	}
	workspaceParent, err := os.MkdirTemp("", "ultraplan-qa-rerun-")
	if err != nil {
		return QAReproductionRun{}, err
	}
	defer os.RemoveAll(workspaceParent)
	targetID, err := targetIdentity(manifest.Target)
	if err != nil {
		return QAReproductionRun{}, err
	}
	// A persistent rerun may target the current implementation after repair. The
	// immutable spec remains unchanged; the run records the selected target.
	run, err := RunQAReproduction(ctx, QAReproductionRequest{Project: sp.Project, Sprint: sp.Slug, TargetRoot: manifest.Target, WorkspaceParent: workspaceParent, ProtectedRoots: []string{s.root, manifest.Target}, Spec: spec, Bundle: bundle, Budgets: qaMap.Budgets, ExpectedTargetID: targetID, AllowDifferentImplementation: true, Now: s.now})
	if err != nil {
		return QAReproductionRun{}, err
	}
	// Restore the immutable signature/spec identity for validation and storage.
	run.SpecID = spec.ID
	fence := s.qaWriterFence
	if fence == nil {
		expected := token
		fence = func(got QAWriterToken) error {
			if got != expected {
				return fmt.Errorf("writer token does not own this rerun")
			}
			return nil
		}
	}
	store = store.WithWriterFence(fence)
	if err := store.PublishReproductionRun(state.CurrentAttemptID, spec, bundle, run, token); err != nil {
		return QAReproductionRun{}, err
	}
	return run, nil
}
