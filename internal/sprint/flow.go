package sprint

import (
	"context"
	"fmt"
	"strings"
	"time"

	"ultraplan-go/internal/platform/runtime"
)

type Runtime interface {
	StartRun(context.Context, runtime.Request) (runtime.Result, error)
}

type FlowRequest struct {
	To     PlanningStage
	DryRun bool
}

type FlowResult struct {
	Project  string
	Sprint   string
	To       PlanningStage
	DryRun   bool
	Message  string
	Runtime  runtime.Result
	Stages   []StageState
	Findings []ValidationFinding
}

func flowSprintIndexSuccessStages(sp Sprint, noTemplates bool, now time.Time) []StageState {
	stages := []StageState{
		{Stage: StageRequirements, Status: StatusComplete, Path: ArtifactRelPath(sp, StageRequirements), LastRunAt: &now},
		{Stage: StageSprintIndex, Status: StatusComplete, Path: ArtifactRelPath(sp, StageSprintIndex), LastRunAt: &now},
		{Stage: StageTechnicalHandbook, Status: StatusReady, Path: ArtifactRelPath(sp, StageTechnicalHandbook)},
		{Stage: StageAreaReasoning, Status: StatusMissing, Path: ArtifactRelPath(sp, StageAreaReasoning)},
		{Stage: StageReasoning, Status: StatusMissing, Path: ArtifactRelPath(sp, StageReasoning)},
		{Stage: StagePlan, Status: StatusMissing, Path: ArtifactRelPath(sp, StagePlan)},
	}
	if noTemplates {
		stages[3].Status = StatusSkipped
		stages[4].Status = StatusReady
	}
	return stages
}

func flowTechnicalHandbookSuccessStages(sp Sprint, noTemplates bool, now time.Time) []StageState {
	stages := []StageState{
		{Stage: StageRequirements, Status: StatusComplete, Path: ArtifactRelPath(sp, StageRequirements), LastRunAt: &now},
		{Stage: StageSprintIndex, Status: StatusComplete, Path: ArtifactRelPath(sp, StageSprintIndex), LastRunAt: &now},
		{Stage: StageTechnicalHandbook, Status: StatusComplete, Path: ArtifactRelPath(sp, StageTechnicalHandbook), LastRunAt: &now},
		{Stage: StageAreaReasoning, Status: StatusReady, Path: ArtifactRelPath(sp, StageAreaReasoning)},
		{Stage: StageReasoning, Status: StatusMissing, Path: ArtifactRelPath(sp, StageReasoning)},
		{Stage: StagePlan, Status: StatusMissing, Path: ArtifactRelPath(sp, StagePlan)},
	}
	if noTemplates {
		stages[3].Status = StatusSkipped
		stages[4].Status = StatusReady
	}
	return stages
}

func flowFailedStages(sp Sprint, target PlanningStage, err error, now time.Time) []StageState {
	msg := safeError(err)
	stages := []StageState{
		{Stage: StageRequirements, Status: StatusComplete, Path: ArtifactRelPath(sp, StageRequirements)},
		{Stage: StageSprintIndex, Status: StatusFailed, Path: ArtifactRelPath(sp, StageSprintIndex), LastRunAt: &now, Error: msg},
		{Stage: StageTechnicalHandbook, Status: StatusMissing, Path: ArtifactRelPath(sp, StageTechnicalHandbook)},
		{Stage: StageAreaReasoning, Status: StatusMissing, Path: ArtifactRelPath(sp, StageAreaReasoning)},
		{Stage: StageReasoning, Status: StatusMissing, Path: ArtifactRelPath(sp, StageReasoning)},
		{Stage: StagePlan, Status: StatusMissing, Path: ArtifactRelPath(sp, StagePlan)},
	}
	if target == StageTechnicalHandbook {
		stages[1] = StageState{Stage: StageSprintIndex, Status: StatusComplete, Path: ArtifactRelPath(sp, StageSprintIndex)}
		stages[2] = StageState{Stage: StageTechnicalHandbook, Status: StatusFailed, Path: ArtifactRelPath(sp, StageTechnicalHandbook), LastRunAt: &now, Error: msg}
	}
	return stages
}

func safeError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	msg = strings.ReplaceAll(msg, "\n", " ")
	msg = strings.ReplaceAll(msg, "\r", " ")
	msg = strings.ReplaceAll(msg, "\x00", "")
	if len(msg) > 180 {
		msg = msg[:180]
	}
	return msg
}

func validateFlowTarget(stage PlanningStage) error {
	if stage != StageSprintIndex && stage != StageTechnicalHandbook {
		return fmt.Errorf("unsupported sprint flow target %q; supports sprint-index and technical-handbook", stage)
	}
	return nil
}
