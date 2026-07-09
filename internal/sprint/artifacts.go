package sprint

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Antonio7098/ultraplan-go/internal/workspace"
)

func ArtifactRelPath(s Sprint, stage PlanningStage) string {
	base := filepath.ToSlash(filepath.Join("projects", s.Project, "sprints", s.Slug))
	switch stage {
	case StageRequirements:
		return base + "/requirements.md"
	case StageSprintIndex:
		return base + "/sprint-index.md"
	case StageTechnicalHandbook:
		return base + "/technical-handbook.md"
	case StageAreaReasoning:
		return base + "/reasoning"
	case StageReasoning:
		return base + "/reasoning.md"
	case StagePlan:
		return base + "/plan.md"
	case StageExecute:
		return base + "/execute.md"
	default:
		return base
	}
}

func FlowStateRelPath(s Sprint) string {
	return filepath.ToSlash(filepath.Join("projects", s.Project, "sprints", s.Slug, "flow-state.json"))
}

func ExecuteRunStateRelPath(s Sprint) string {
	return filepath.ToSlash(filepath.Join("projects", s.Project, "sprints", s.Slug, ".run-state.json"))
}

func ArtifactPath(root string, s Sprint, stage PlanningStage) (string, error) {
	return resolveSprintContained(root, s, ArtifactRelPath(s, stage))
}

func FlowStatePath(root string, s Sprint) (string, error) {
	return resolveSprintContained(root, s, FlowStateRelPath(s))
}

func resolveSprintContained(root string, s Sprint, rel string) (string, error) {
	if filepath.IsAbs(rel) {
		return "", fmt.Errorf("artifact path %q must be workspace-relative", rel)
	}
	cleanRel := filepath.ToSlash(filepath.Clean(filepath.FromSlash(rel)))
	if cleanRel == "." || strings.HasPrefix(cleanRel, "../") || cleanRel == ".." {
		return "", fmt.Errorf("artifact path %q escapes workspace", rel)
	}
	full, err := workspace.ResolveInside(root, cleanRel)
	if err != nil {
		return "", err
	}
	rootPath, err := workspace.ResolveInside(root, filepath.ToSlash(filepath.Join("projects", s.Project, "sprints", s.Slug)))
	if err != nil {
		return "", err
	}
	if !inside(rootPath, full) {
		return "", fmt.Errorf("artifact path %q escapes sprint root", rel)
	}
	return full, nil
}

func inside(root, candidate string) bool {
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(candidate))
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}
