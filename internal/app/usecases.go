package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Antonio7098/ultraplan-go/internal/platform/config"
	"github.com/Antonio7098/ultraplan-go/internal/project"
	"github.com/Antonio7098/ultraplan-go/internal/sprint"
	"github.com/Antonio7098/ultraplan-go/internal/study"
	"github.com/Antonio7098/ultraplan-go/internal/workspace"
)

const PreviewByteLimit = 32 * 1024

type ReadOnlyUseCases interface {
	Dashboard(ctx context.Context) (DashboardResult, error)
	PreviewArtifact(ctx context.Context, path string) (ArtifactPreviewResult, error)
}

type DashboardResult struct {
	Workspace string
	Projects  []ProjectSummary
	Studies   []StudySummary
	Sprints   []SprintSummary
}

type DisplayFinding struct {
	Severity   string
	Section    string
	Path       string
	Problem    string
	Cause      string
	Suggestion string
}

type DisplayArtifact struct {
	Label string
	Path  string
	Kind  string
}

type ArtifactPreviewResult struct {
	Path      string
	Kind      string
	Content   string
	Truncated bool
	Missing   bool
	Invalid   bool
	Error     string
}

type dashboardUseCases struct {
	root              string
	runner            func(context.Context, OperationRequest, func(OperationEvent)) (OperationResult, error)
	stageRuntime      map[sprint.PlanningStage]sprint.StageRuntime
	reviewConcurrency int
	smokeSettings     sprint.SmokeSettings
	readOnly          bool
}

func (u dashboardUseCases) sprintService() sprint.Service {
	service := sprint.NewService(u.root).WithStageRuntime(u.stageRuntime).WithReviewConcurrency(u.reviewConcurrency)
	if u.smokeSettings.DiscoveryTimeout > 0 {
		service = service.WithSmokeSettings(u.smokeSettings)
	}
	if u.readOnly {
		service = service.WithoutStatusWrites()
	}
	return service
}

func NewReadOnlyUseCases(root string) ReadOnlyUseCases {
	return dashboardUseCases{root: root}
}

func (u dashboardUseCases) Dashboard(ctx context.Context) (DashboardResult, error) {
	if err := ctx.Err(); err != nil {
		return DashboardResult{}, err
	}
	projects, err := u.ProjectSummaries(ctx)
	if err != nil {
		return DashboardResult{}, err
	}
	studies, err := u.StudySummaries(ctx)
	if err != nil {
		return DashboardResult{}, err
	}
	sprints, err := u.SprintSummaries(ctx)
	if err != nil {
		return DashboardResult{}, err
	}
	return DashboardResult{Workspace: u.root, Projects: projects, Studies: studies, Sprints: sprints}, nil
}

func (u dashboardUseCases) PreviewArtifact(ctx context.Context, rel string) (ArtifactPreviewResult, error) {
	if err := ctx.Err(); err != nil {
		return ArtifactPreviewResult{}, err
	}
	rel = filepath.ToSlash(filepath.Clean(filepath.FromSlash(strings.TrimSpace(rel))))
	if !supportedPreviewPath(rel) {
		return ArtifactPreviewResult{Path: rel, Kind: previewKind(rel), Error: "unsupported artifact path"}, nil
	}
	path, err := workspace.ResolveInside(u.root, rel)
	if err != nil {
		return ArtifactPreviewResult{Path: rel, Error: err.Error()}, nil
	}
	if err := rejectPreviewSymlink(u.root, rel); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return ArtifactPreviewResult{Path: rel, Kind: previewKind(rel), Error: "artifact path contains a symbolic link"}, nil
	}
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return ArtifactPreviewResult{Path: rel, Kind: previewKind(rel), Missing: true, Error: "artifact is missing"}, nil
		}
		return ArtifactPreviewResult{Path: rel, Kind: previewKind(rel), Error: fmt.Sprintf("read artifact: %v", err)}, nil
	}
	defer f.Close()
	limited := io.LimitReader(f, PreviewByteLimit+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return ArtifactPreviewResult{Path: rel, Kind: previewKind(rel), Error: fmt.Sprintf("read artifact: %v", err)}, nil
	}
	truncated := len(data) > PreviewByteLimit
	if truncated {
		data = data[:PreviewByteLimit]
	}
	result := ArtifactPreviewResult{Path: rel, Kind: previewKind(rel), Content: string(data), Truncated: truncated}
	if result.Kind == "json" && json.Valid(data) == false {
		result.Invalid = true
		result.Error = "invalid JSON preview"
	}
	return result, nil
}

func rejectPreviewSymlink(root, rel string) error {
	path := root
	for _, part := range strings.Split(filepath.Clean(filepath.FromSlash(rel)), string(filepath.Separator)) {
		path = filepath.Join(path, part)
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symbolic link component: %s", part)
		}
	}
	return nil
}

func supportedPreviewPath(rel string) bool {
	if rel == "." || rel == ".." || strings.HasPrefix(rel, "../") || filepath.IsAbs(rel) {
		return false
	}
	base := filepath.Base(rel)
	if base == "project-index.md" || base == "roadmap.md" || base == "requirements.md" || base == "code-context.md" || base == "sprint-index.md" ||
		base == "technical-handbook.md" || base == "reasoning.md" || base == "plan.md" || base == "execute.md" || base == "review.md" || base == "smoke.md" ||
		base == "flow-state.json" || base == ".run-state.json" {
		return true
	}
	if strings.HasPrefix(rel, "projects/") && strings.Contains(rel, "/docs/") && strings.HasSuffix(rel, ".md") {
		return true
	}
	if strings.HasPrefix(rel, "studies/") {
		if strings.Contains(rel, "/dimensions/") && strings.HasSuffix(rel, ".md") {
			return true
		}
		if strings.Contains(rel, "/sources/") && strings.HasSuffix(rel, ".md") {
			return true
		}
		if strings.HasSuffix(rel, "/.ultraplan/run-state.json") {
			return true
		}
	}
	return false
}

func previewKind(rel string) string {
	if strings.HasSuffix(rel, ".json") {
		return "json"
	}
	return "markdown"
}

func displaySafe(value string) string {
	return config.RedactValue("tui.display", value)
}

func sortArtifacts(items []DisplayArtifact) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].Path == items[j].Path {
			return items[i].Label < items[j].Label
		}
		return items[i].Path < items[j].Path
	})
}

func projectFinding(f project.ValidationFinding) DisplayFinding {
	return DisplayFinding{
		Severity:   string(f.Severity),
		Section:    string(f.Section),
		Path:       f.Path,
		Problem:    displaySafe(f.Problem),
		Cause:      displaySafe(f.Cause),
		Suggestion: displaySafe(f.Suggestion),
	}
}

func sprintFinding(f sprint.ValidationFinding) DisplayFinding {
	return DisplayFinding{
		Severity:   "error",
		Section:    f.Section,
		Path:       f.Path,
		Problem:    displaySafe(f.Problem),
		Cause:      displaySafe(f.Cause),
		Suggestion: displaySafe(f.Suggestion),
	}
}

func studyFinding(c study.ValidationCheck) DisplayFinding {
	return DisplayFinding{
		Severity:   string(c.Severity),
		Section:    c.Name,
		Path:       c.Path,
		Problem:    displaySafe(string(c.Status)),
		Cause:      displaySafe(c.Observed),
		Suggestion: displaySafe(c.Guidance),
	}
}
