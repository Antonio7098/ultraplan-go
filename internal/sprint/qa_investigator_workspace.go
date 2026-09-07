package sprint

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
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

// Restore a cleaned workspace at the same path for a retained session. Only
// the frozen target and validated immutable test bundles may be materialized.
func restoreQAInvestigatorEvidenceWorkspace(ctx context.Context, root, target string, qaMap QAMap, shard QAShard, tests []QATestPublication) error {
	path := qaInvestigatorWorkspacePath(root, qaMap.SemanticAttemptID, shard.ID)
	if info, err := os.Lstat(path); err == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("unsafe investigator workspace")
		}
		return nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	identity, err := targetIdentity(target)
	if err != nil || identity != qaMap.ImplementationFingerprint {
		return NewQAError(QAErrorStaleInput, "restore investigator workspace", "target no longer matches the frozen implementation", err)
	}
	if _, err := prepareQAInvestigatorWorkspace(ctx, root, target, qaMap, shard); err != nil {
		return err
	}
	ordered := append([]QATestPublication(nil), tests...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i].Spec.FrozenAt.Before(ordered[j].Spec.FrozenAt)
	})
	for _, test := range ordered {
		if test.Spec.ShardID != shard.ID || test.Spec.AttemptID != qaMap.SemanticAttemptID {
			continue
		}
		if test.Spec.ImplementationFingerprint != qaMap.ImplementationFingerprint {
			return errors.New("stale retained investigator test")
		}
		if err := ValidateQAReproductionSpec(test.Spec, qaMap.Budgets); err != nil {
			return err
		}
		if err := ValidateQATestBundle(test.Bundle, test.Spec, qaMap.Budgets); err != nil {
			return err
		}
		for _, file := range test.Bundle.Files {
			full := filepath.Join(path, filepath.FromSlash(file.Path))
			for parent := full; parent != path; parent = filepath.Dir(parent) {
				if !inside(path, parent) {
					return errors.New("retained test escapes investigator workspace")
				}
				if info, err := os.Lstat(parent); err == nil && info.Mode()&os.ModeSymlink != 0 {
					return errors.New("retained test path contains a symlink")
				} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
					return err
				}
			}
			if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
				return err
			}
			if err := os.WriteFile(full, []byte(file.Content), 0o600); err != nil {
				return err
			}
		}
	}
	return nil
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
		category := QAErrorPermissionDenied
		detail := "cannot create the private per-shard target copy"
		if errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "exceeds declared limits") {
			category = QAErrorBudgetExhausted
			detail = "the target exceeds the private per-shard copy limits"
		}
		return "", NewQAError(category, "prepare investigator workspace", detail, err)
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
