package sprint

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/Antonio7098/ultraplan-go/internal/platform/gitpublish"
)

func (s Service) publishPlanningStage(ctx context.Context, projectRef, sprintRef string, stage PlanningStage) ([]gitpublish.Result, error) {
	if s.publisher == nil {
		return nil, nil
	}
	sp, err := s.resolveMutationSprint(projectRef, sprintRef)
	if err != nil {
		return nil, err
	}
	paths := []string{filepath.Join(sp.Path, "flow-state.json")}
	if stage == StageAreaReasoning {
		inputs, readErr := s.store.ReadPlanningInputs(sp)
		if readErr != nil {
			return nil, readErr
		}
		_, _, catalog, resolveErr := s.resolveSprintInputs(projectRef, sprintRef)
		if resolveErr != nil {
			return nil, resolveErr
		}
		manifest, findings := BuildReasoningManifest(s.root, sp, inputs, catalog)
		if len(findings) > 0 {
			return nil, fmt.Errorf("git publish: area-reasoning manifest is invalid")
		}
		for _, entry := range manifest.ReasoningTemplates {
			paths = append(paths, filepath.Join(s.root, filepath.FromSlash(normalizeWorkspacePath(entry.OutputPath))))
		}
	} else {
		path, pathErr := ArtifactPath(s.root, sp, stage)
		if pathErr != nil {
			return nil, pathErr
		}
		paths = append(paths, path)
	}
	if stage == StageCodeContext {
		paths = append(paths, sprintWorkspacePath(sp))
	}
	result, err := s.publisher.Publish(ctx, gitpublish.Request{
		Root: s.root, Paths: paths,
		Message:  fmt.Sprintf("ultraplan: sprint %s/%s complete %s", sp.Project, sp.Slug, stage),
		Identity: fmt.Sprintf("sprint/%s/%s/%s", sp.Project, sp.Slug, stage),
	})
	if result.Repository == "" && result.Skipped {
		return nil, err
	}
	return []gitpublish.Result{result}, err
}

func (s Service) publishExecuteStage(ctx context.Context, sp Sprint, target ExecuteTargetRef) ([]gitpublish.Result, error) {
	if s.publisher == nil {
		return nil, nil
	}
	identity := fmt.Sprintf("sprint/%s/%s/%s", sp.Project, sp.Slug, StageExecute)
	message := fmt.Sprintf("ultraplan: sprint %s/%s complete %s", sp.Project, sp.Slug, StageExecute)
	implementation, err := s.publisher.Publish(ctx, gitpublish.Request{Root: target.Path, All: true, Message: message, Identity: identity + "/implementation"})
	results := visiblePublication(implementation)
	if err != nil {
		return results, err
	}
	workspaceResult, err := s.publisher.Publish(ctx, gitpublish.Request{
		Root: s.root,
		Paths: []string{
			filepath.Join(sp.Path, "execute.md"),
			filepath.Join(sp.Path, ".run-state.json"),
		},
		Message: message, Identity: identity + "/workspace",
	})
	results = append(results, visiblePublication(workspaceResult)...)
	return results, err
}

func (s Service) publishReviewStage(ctx context.Context, sp Sprint) ([]gitpublish.Result, error) {
	if s.publisher == nil {
		return nil, nil
	}
	result, err := s.publisher.Publish(ctx, gitpublish.Request{
		Root:     s.root,
		Paths:    []string{filepath.Join(sp.Path, "review.md"), filepath.Join(sp.Path, "flow-state.json")},
		Message:  fmt.Sprintf("ultraplan: sprint %s/%s complete %s", sp.Project, sp.Slug, StageReview),
		Identity: fmt.Sprintf("sprint/%s/%s/%s", sp.Project, sp.Slug, StageReview),
	})
	return visiblePublication(result), err
}

func (s Service) publishSmokeStage(ctx context.Context, prepared smokePrepared, result SmokeResult) ([]gitpublish.Result, error) {
	if s.publisher == nil {
		return nil, nil
	}
	identity := fmt.Sprintf("sprint/%s/%s/%s", prepared.Sprint.Project, prepared.Sprint.Slug, StageSmoke)
	message := fmt.Sprintf("ultraplan: sprint %s/%s complete %s", prepared.Sprint.Project, prepared.Sprint.Slug, StageSmoke)
	var results []gitpublish.Result
	if len(result.AuthorChangedPaths) > 0 {
		paths := make([]string, 0, len(result.AuthorChangedPaths))
		for _, rel := range result.AuthorChangedPaths {
			paths = append(paths, filepath.Join(prepared.HarnessRoot, filepath.FromSlash(rel)))
		}
		harness, err := s.publisher.Publish(ctx, gitpublish.Request{Root: prepared.HarnessRoot, Paths: paths, Message: message, Identity: identity + "/harness"})
		results = append(results, visiblePublication(harness)...)
		if err != nil {
			return results, err
		}
	}
	workspacePaths := []string{
		filepath.Join(prepared.Sprint.Path, "smoke.md"),
		filepath.Join(prepared.Sprint.Path, "flow-state.json"),
	}
	if result.Verdict == SmokePass && !result.DiagnosticOnly {
		workspacePaths = append(workspacePaths, filepath.Join(filepath.Dir(filepath.Dir(prepared.Sprint.Path)), "roadmap.md"))
	}
	workspaceResult, err := s.publisher.Publish(ctx, gitpublish.Request{Root: s.root, Paths: workspacePaths, Message: message, Identity: identity + "/workspace"})
	results = append(results, visiblePublication(workspaceResult)...)
	return results, err
}

func visiblePublication(result gitpublish.Result) []gitpublish.Result {
	if result.Repository == "" && result.Skipped {
		return nil
	}
	return []gitpublish.Result{result}
}
