package app

import (
	"context"
	"fmt"

	"github.com/Antonio7098/ultraplan-go/internal/project"
	"github.com/Antonio7098/ultraplan-go/internal/sprint"
)

type SprintSummary struct {
	Project           string
	Slug              string
	Status            string
	FlowStatePath     string
	ExecutePath       string
	RunStatePath      string
	ReviewPath        string
	SmokePath         string
	Stages            []StageSummary
	Execute           ExecuteSummary
	Review            ReviewSummary
	Smoke             SmokeSummary
	Findings          []DisplayFinding
	Artifacts         []DisplayArtifact
	RefreshMayWrite   bool
	RefreshActionNote string
	Assessment        string
	NextAction        string
}

type ReviewSummary struct {
	Available                bool
	Status, Verdict          string
	Stale                    bool
	Completed, Total         int
	Pending, Running, Failed int
	Artifact, Digest         string
	FreshnessReasons         []string
	Reviewers                []ReviewItemSummary
}

type ReviewItemSummary struct {
	ID, Name, Kind, Path, Status, Summary string
}

type SmokeSummary struct {
	Available                    bool
	Status, Verdict, RunID       string
	Stale, Reconciliation        bool
	Artifact, Digest, NextAction string
	FreshnessReasons             []string
	Issues                       []sprint.SmokeIssue
	Override                     *sprint.DiagnosticOverride
}

type StageSummary struct {
	Name   string
	Status string
	Path   string
	Error  string
}

type ExecuteSummary struct {
	Available bool
	Total     int
	Pending   int
	Running   int
	Complete  int
	Deferred  int
	Failed    int
	Cancelled int
	Message   string
}

func (u dashboardUseCases) SprintSummaries(ctx context.Context) ([]SprintSummary, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	projects, err := project.DiscoverProjects(u.root)
	if err != nil {
		return nil, mapProjectError("project.list", err)
	}
	service := u.sprintService()
	var out []SprintSummary
	for _, p := range projects {
		sprints, err := sprint.DiscoverSprints(u.root, p)
		if err != nil {
			return nil, mapSprintError("sprint.list", err)
		}
		for _, sp := range sprints {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			status, err := service.Status(p.Name, sp.Slug)
			if err != nil {
				out = append(out, SprintSummary{
					Project:           p.Name,
					Slug:              sp.Slug,
					Status:            "status unavailable",
					RefreshMayWrite:   !u.readOnly,
					RefreshActionNote: "refresh recomputes deterministic flow-state.json status when existing state is valid",
					Findings:          []DisplayFinding{{Severity: "error", Section: "sprint.status", Problem: displaySafe(err.Error()), Suggestion: "Inspect or regenerate sprint flow-state.json outside the read-only TUI."}},
					Artifacts: []DisplayArtifact{
						{Label: "requirements", Path: sprint.ArtifactRelPath(sp, sprint.StageRequirements), Kind: "markdown"},
						{Label: "sprint-index", Path: sprint.ArtifactRelPath(sp, sprint.StageSprintIndex), Kind: "markdown"},
						{Label: "technical-handbook", Path: sprint.ArtifactRelPath(sp, sprint.StageTechnicalHandbook), Kind: "markdown"},
						{Label: "reasoning", Path: sprint.ArtifactRelPath(sp, sprint.StageReasoning), Kind: "markdown"},
						{Label: "plan", Path: sprint.ArtifactRelPath(sp, sprint.StagePlan), Kind: "markdown"},
						{Label: "execute", Path: sprint.ArtifactRelPath(sp, sprint.StageExecute), Kind: "markdown"},
						{Label: "review", Path: sprint.ArtifactRelPath(sp, sprint.StageReview), Kind: "markdown"},
						{Label: "smoke", Path: sprint.ArtifactRelPath(sp, sprint.StageSmoke), Kind: "markdown"},
						{Label: "flow-state", Path: sprint.FlowStateRelPath(sp), Kind: "json"},
						{Label: "run-state", Path: sprint.ExecuteRunStateRelPath(sp), Kind: "json"},
					},
					Execute: ExecuteSummary{Message: "execute status unavailable because sprint status failed"},
				})
				continue
			}
			review := summarizeReview(status.Review)
			manifest, _, manifestErr := service.PrepareReview(p.Name, sp.Slug, sprint.ReviewRequest{})
			if manifestErr == nil {
				review.Reviewers = summarizeReviewers(status.Review, manifest.Coverage)
			} else {
				review.Reviewers = summarizeReviewers(status.Review, nil)
			}
			for _, reviewer := range review.Reviewers {
				switch reviewer.Status {
				case "pending":
					review.Pending++
				case "running":
					review.Running++
				case "failed", "cancelled", "timed_out", "blocked":
					review.Failed++
				}
			}
			summary := SprintSummary{
				Project:           p.Name,
				Slug:              sp.Slug,
				Status:            "available",
				FlowStatePath:     status.FlowStatePath,
				ExecutePath:       status.ExecutePath,
				RunStatePath:      status.RunStatePath,
				ReviewPath:        status.ReviewPath,
				SmokePath:         status.SmokePath,
				RefreshMayWrite:   !u.readOnly,
				RefreshActionNote: "refresh derives verification freshness and assessment from current evidence without caching them as authoritative state",
				Execute:           summarizeExecute(status.ExecuteState),
				Review:            review,
				Smoke:             summarizeSmoke(status.Smoke),
				Assessment:        string(status.Verification.Assessment),
				NextAction:        status.Verification.NextAction,
			}
			if status.HistoricalExecutionStatus != "" {
				summary.Status = status.HistoricalExecutionStatus
				summary.Execute = summarizeHistoricalExecute(status.HistoricalExecutionStatus)
			}
			summary.Review.Artifact, summary.Review.Digest, summary.Review.FreshnessReasons = status.Verification.Review.Artifact, status.Verification.Review.ArtifactDigest, append([]string(nil), status.Verification.Review.FreshnessReasons...)
			summary.Review.Stale = !status.Verification.Review.Fresh
			summary.Smoke.Artifact, summary.Smoke.Digest, summary.Smoke.NextAction = status.Verification.Smoke.Artifact, status.Verification.Smoke.ArtifactDigest, status.Verification.Smoke.NextAction
			summary.Smoke.FreshnessReasons, summary.Smoke.Issues, summary.Smoke.Override = append([]string(nil), status.Verification.Smoke.FreshnessReasons...), append([]sprint.SmokeIssue(nil), status.Verification.Smoke.Issues...), status.Verification.Smoke.Override
			summary.Smoke.Stale = !status.Verification.Smoke.Fresh
			for _, stage := range status.Stages {
				summary.Stages = append(summary.Stages, StageSummary{Name: string(stage.Stage), Status: string(stage.Status), Path: stage.Path, Error: displaySafe(stage.Error)})
			}
			summary.Artifacts = append(summary.Artifacts,
				DisplayArtifact{Label: "requirements", Path: sprint.ArtifactRelPath(sp, sprint.StageRequirements), Kind: "markdown"},
				DisplayArtifact{Label: "sprint-index", Path: sprint.ArtifactRelPath(sp, sprint.StageSprintIndex), Kind: "markdown"},
				DisplayArtifact{Label: "technical-handbook", Path: sprint.ArtifactRelPath(sp, sprint.StageTechnicalHandbook), Kind: "markdown"},
				DisplayArtifact{Label: "reasoning", Path: sprint.ArtifactRelPath(sp, sprint.StageReasoning), Kind: "markdown"},
				DisplayArtifact{Label: "plan", Path: sprint.ArtifactRelPath(sp, sprint.StagePlan), Kind: "markdown"},
				DisplayArtifact{Label: "execute", Path: sprint.ArtifactRelPath(sp, sprint.StageExecute), Kind: "markdown"},
				DisplayArtifact{Label: "review", Path: sprint.ArtifactRelPath(sp, sprint.StageReview), Kind: "markdown"},
				DisplayArtifact{Label: "smoke", Path: sprint.ArtifactRelPath(sp, sprint.StageSmoke), Kind: "markdown"},
				DisplayArtifact{Label: "flow-state", Path: sprint.FlowStateRelPath(sp), Kind: "json"},
				DisplayArtifact{Label: "run-state", Path: sprint.ExecuteRunStateRelPath(sp), Kind: "json"},
			)
			for _, stage := range []sprint.PlanningStage{sprint.StageRequirements, sprint.StageSprintIndex, sprint.StageTechnicalHandbook, sprint.StageReasoning, sprint.StagePlan, sprint.StageExecute, sprint.StageReview, sprint.StageSmoke} {
				result, err := validateSprintStage(service, p.Name, sp.Slug, stage)
				if err != nil {
					continue
				}
				for _, finding := range result.Findings {
					summary.Findings = append(summary.Findings, sprintFinding(finding))
				}
			}
			sortArtifacts(summary.Artifacts)
			out = append(out, summary)
		}
	}
	return out, nil
}

func validateSprintStage(service sprint.Service, projectRef, sprintRef string, stage sprint.PlanningStage) (sprint.ValidationResult, error) {
	switch stage {
	case sprint.StageRequirements:
		return service.ValidateRequirements(projectRef, sprintRef)
	case sprint.StageSprintIndex:
		return service.ValidateSprintIndex(projectRef, sprintRef)
	case sprint.StageTechnicalHandbook:
		return service.ValidateTechnicalHandbook(projectRef, sprintRef)
	case sprint.StageAreaReasoning:
		return service.ValidateAreaReasoning(projectRef, sprintRef)
	case sprint.StageReasoning:
		return service.ValidateReasoning(projectRef, sprintRef)
	case sprint.StagePlan:
		return service.ValidatePlan(projectRef, sprintRef)
	case sprint.StageExecute:
		return service.ValidateExecute(projectRef, sprintRef)
	case sprint.StageReview:
		return service.ValidateReview(projectRef, sprintRef)
	case sprint.StageSmoke:
		return service.ValidateSmoke(projectRef, sprintRef)
	default:
		return sprint.ValidationResult{}, fmt.Errorf("unsupported validation stage %q", stage)
	}
}

func summarizeSmoke(state *sprint.SmokeStageState) SmokeSummary {
	if state == nil {
		return SmokeSummary{}
	}
	return SmokeSummary{Available: true, Status: string(state.Status), Verdict: string(state.Verdict), RunID: state.RunID, Stale: state.Stale, Reconciliation: state.Reconciliation}
}

func summarizeReview(state *sprint.ReviewStageState) ReviewSummary {
	if state == nil {
		return ReviewSummary{}
	}
	return ReviewSummary{Available: true, Status: string(state.Status), Verdict: string(state.Verdict), Stale: state.Stale, Completed: state.Completed, Total: state.Total}
}

func summarizeReviewers(state *sprint.ReviewStageState, coverage []sprint.ReviewInput) []ReviewItemSummary {
	items := make([]ReviewItemSummary, 0, len(coverage))
	positions := make(map[string]int, len(coverage))
	for _, input := range coverage {
		if input.ID == "" {
			continue
		}
		positions[input.ID] = len(items)
		items = append(items, ReviewItemSummary{ID: input.ID, Name: input.Name, Kind: input.Kind, Path: input.Path, Status: "pending"})
	}
	ensure := func(id string) int {
		if index, ok := positions[id]; ok {
			return index
		}
		positions[id] = len(items)
		items = append(items, ReviewItemSummary{ID: id, Name: id, Status: "pending"})
		return len(items) - 1
	}
	if state == nil {
		return items
	}
	if state.LastComplete != nil {
		for _, result := range state.LastComplete.Coverage {
			index := ensure(result.CoverageID)
			items[index].Status = "completed"
			items[index].Summary = displaySafe(result.Summary)
		}
	}
	if state.Resume != nil {
		for _, checkpoint := range state.Resume.Coverage {
			index := ensure(checkpoint.CoverageID)
			items[index].Status = string(checkpoint.Status)
			if checkpoint.Result != nil {
				items[index].Summary = displaySafe(checkpoint.Result.Summary)
			}
		}
	}
	return items
}

func summarizeExecute(state *sprint.ExecuteRunState) ExecuteSummary {
	if state == nil {
		return ExecuteSummary{Message: "execute run-state unavailable"}
	}
	summary := ExecuteSummary{Available: true, Total: len(state.Tasks)}
	for _, task := range state.Tasks {
		switch task.Status {
		case sprint.ExecuteTaskPending:
			summary.Pending++
		case sprint.ExecuteTaskRunning:
			summary.Running++
		case sprint.ExecuteTaskComplete:
			summary.Complete++
		case sprint.ExecuteTaskDeferred:
			summary.Deferred++
		case sprint.ExecuteTaskFailed:
			summary.Failed++
		case sprint.ExecuteTaskCancelled:
			summary.Cancelled++
		}
	}
	return summary
}

func summarizeHistoricalExecute(status string) ExecuteSummary {
	summary := ExecuteSummary{Available: true, Total: 1, Message: "historical terminal execution evidence"}
	switch status {
	case "complete":
		summary.Complete = 1
	case "failed":
		summary.Failed = 1
	case "cancelled":
		summary.Cancelled = 1
	}
	return summary
}
