package sprint

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	pprocess "github.com/Antonio7098/ultraplan-go/internal/platform/process"
)

func qaInvestigatorWorkspaceParent(root, attemptID string) string {
	scope := hashOpaque(filepath.Clean(root))[:24]
	return filepath.Join(os.TempDir(), "ultraplan-qa-investigators", scope, attemptID)
}

func qaInvestigatorWorkspacePath(root, attemptID, shardID string) string {
	return filepath.Join(qaInvestigatorWorkspaceParent(root, attemptID), shardID)
}

func prepareQAInvestigatorWorkspace(ctx context.Context, root, target string, qaMap QAMap, shard QAShard) (string, error) {
	path := qaInvestigatorWorkspacePath(root, qaMap.SemanticAttemptID, shard.ID)
	limits := pprocess.IsolationLimits{MaxFiles: qaMap.Budgets.TreeFiles, MaxBytes: qaMap.Budgets.TreeBytes, MaxFileSize: qaMap.Budgets.FileBytes, Timeout: qaMap.Budgets.ShardTimeout}
	if info, err := os.Lstat(path); err == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return "", NewQAError(QAErrorPermissionDenied, "prepare investigator workspace", "retained shard workspace is unsafe", nil)
		}
		targetIdentity, targetErr := pprocess.IdentifyTree(ctx, target, limits)
		workspaceIdentity, workspaceErr := pprocess.IdentifyTree(ctx, path, limits)
		if targetErr != nil || workspaceErr != nil || targetIdentity.Digest != workspaceIdentity.Digest {
			return "", NewQAError(QAErrorStaleInput, "prepare investigator workspace", "retained shard workspace no longer matches its frozen implementation", errors.Join(targetErr, workspaceErr))
		}
		return path, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	workspace, err := pprocess.CreateIsolation(ctx, pprocess.IsolationRequest{SourceRoot: target, ParentDir: qaInvestigatorWorkspaceParent(root, qaMap.SemanticAttemptID), Destination: path, Prefix: shard.ID, ProtectedRoots: []string{root, target}, Limits: limits})
	if err != nil {
		return "", NewQAError(QAErrorPermissionDenied, "prepare investigator workspace", "cannot create the private per-shard target copy", err)
	}
	capabilities := workspace.Capabilities
	if !capabilities.PrivateWorkspace || !capabilities.ContainedCopy || !capabilities.DescendantCleanup || !capabilities.WorkspaceRemoval || !capabilities.NativeProtectedRootDeny {
		cleanup := workspace.Cleanup()
		return "", NewQAError(QAErrorAdmissionBlocked, "prepare investigator workspace", "host isolation cannot protect the production target", errors.New(cleanup.Error))
	}
	return workspace.Path, nil
}

func cleanupQAInvestigatorWorkspaces(root, attemptID string) QACleanupFacts {
	path := qaInvestigatorWorkspaceParent(root, attemptID)
	facts := QACleanupFacts{Attempted: true, DescendantsTerminated: true}
	base := filepath.Base(path)
	if !validQAIDKind(base, "attempt") || !strings.Contains(filepath.ToSlash(path), "/ultraplan-qa-investigators/") {
		facts.Diagnostic = "refused unsafe investigator workspace cleanup path"
		return facts
	}
	if err := os.RemoveAll(path); err != nil {
		facts.Diagnostic = err.Error()
		return facts
	}
	_, err := os.Lstat(path)
	facts.WorkspaceRemoved = errors.Is(err, fs.ErrNotExist)
	facts.Complete = facts.WorkspaceRemoved
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		facts.Diagnostic = err.Error()
	}
	return facts
}
