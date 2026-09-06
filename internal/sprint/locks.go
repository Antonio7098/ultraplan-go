package sprint

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Antonio7098/ultraplan-go/internal/project"
)

// The mutation lease is product-owned and shared by flow, execute, review,
// QA, smoke, and verify. A context marker permits composite workflows to invoke
// child stages without reacquiring the same cross-process file lock.
type mutationLeaseContextKey struct{}

type mutationLeaseContext struct {
	path string
}

// ReconcileInterruptedMutation converts durable running records left by a
// dead owner into explicit interrupted evidence. A live cross-process lease is
// never rewritten.
func (s Service) ReconcileInterruptedMutation(ctx context.Context, projectRef, sprintRef string) (bool, error) {
	lockedCtx, release, err := s.acquireMutationContext(ctx, projectRef, sprintRef)
	if err != nil {
		if errors.Is(err, ErrVerificationConflict) {
			return false, nil
		}
		return false, err
	}
	defer release()
	_ = lockedCtx
	sp, err := s.resolveMutationSprint(projectRef, sprintRef)
	if err != nil {
		return false, err
	}
	now := s.now().UTC()
	_, uncertaintyErr := loadCleanupUncertain(sp)
	hasUncertainty := uncertaintyErr == nil
	if uncertaintyErr != nil && !errors.Is(uncertaintyErr, os.ErrNotExist) {
		return false, uncertaintyErr
	}
	changed := false
	state, err := LoadExecuteRunState(s.root, sp)
	if err == nil {
		for i := range state.Tasks {
			if state.Tasks[i].Status != ExecuteTaskRunning {
				continue
			}
			state.Tasks[i].Status = ExecuteTaskFailed
			state.Tasks[i].UpdatedAt = now
			state.Tasks[i].CompletedAt = &now
			state.Tasks[i].Diagnostics = append(state.Tasks[i].Diagnostics, ExecuteDiagnostic{Code: "recovery-interrupted", Message: "running task belonged to a stopped process; inspect durable target state before resuming", At: now})
			changed = true
		}
		if changed {
			state.UpdatedAt = now
			if err := SaveExecuteRunState(s.root, sp, state); err != nil {
				return false, err
			}
		}
	} else if !errors.Is(err, ErrExecuteRunStateMissing) && !legacyTerminalExecuteRunState(s.root, sp) {
		return false, err
	}
	flow, flowErr := LoadFlowState(s.root, sp)
	if flowErr == nil {
		if reconcileExpiredAttempts(&flow, now) {
			flow.UpdatedAt = now
			if err := SaveFlowState(s.root, sp, flow); err != nil {
				return false, err
			}
			changed = true
		}
		qaStore := NewQAStore(s.root, sp)
		if qaPath, pathErr := qaStore.StatePath(); pathErr == nil {
			if _, statErr := os.Lstat(qaPath); statErr == nil {
				qaState, qaErr := qaStore.LoadState()
				if qaErr != nil {
					return false, qaErr
				}
				if reconcileInterruptedQAState(&qaState) {
					qaState.UpdatedAt = now
					if err := qaStore.SaveRecoveredState(qaState, flow); err != nil {
						return false, err
					}
					changed = true
				}
			}
		}
	} else if !errors.Is(flowErr, ErrFlowStateMissing) && !legacyFlowState(s.root, sp) {
		return false, flowErr
	}
	if hasUncertainty && changed {
		if err := removeCleanupUncertain(sp); err != nil {
			return false, err
		}
	}
	if hasUncertainty && !changed {
		return false, fmt.Errorf("%w: %s", ErrCleanupUncertain, filepath.Join(sp.Path, cleanupUncertainFileName))
	}
	return changed, nil
}

func (s Service) acquireMutationContext(ctx context.Context, projectRef, sprintRef string) (context.Context, func(), error) {
	sp, err := s.resolveMutationSprint(projectRef, sprintRef)
	if err != nil {
		return ctx, nil, err
	}
	path := filepath.Clean(sp.Path)
	if held, ok := ctx.Value(mutationLeaseContextKey{}).(mutationLeaseContext); ok && held.path == path {
		return ctx, func() {}, nil
	}
	release, err := s.acquireMutation(projectRef, sprintRef)
	if err != nil {
		return ctx, nil, err
	}
	return context.WithValue(ctx, mutationLeaseContextKey{}, mutationLeaseContext{path: path}), release, nil
}

func (s Service) resolveMutationSprint(projectRef, sprintRef string) (Sprint, error) {
	projects, err := project.DiscoverProjects(s.root)
	if err != nil {
		return Sprint{}, err
	}
	p, err := project.ResolveProject(projects, projectRef)
	if err != nil {
		return Sprint{}, err
	}
	sprints, err := DiscoverSprints(s.root, p)
	if err != nil {
		return Sprint{}, err
	}
	sp, err := ResolveSprint(sprints, sprintRef)
	return sp, err
}
