package sprint

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Antonio7098/ultraplan-go/internal/platform/runtime"
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

func flowRequirementsSuccessStages(sp Sprint, now time.Time) []StageState {
	return []StageState{
		{Stage: StageRequirements, Status: StatusComplete, Path: ArtifactRelPath(sp, StageRequirements), LastRunAt: &now},
		{Stage: StageSprintIndex, Status: StatusReady, Path: ArtifactRelPath(sp, StageSprintIndex)},
		{Stage: StageTechnicalHandbook, Status: StatusMissing, Path: ArtifactRelPath(sp, StageTechnicalHandbook)},
		{Stage: StageAreaReasoning, Status: StatusMissing, Path: ArtifactRelPath(sp, StageAreaReasoning)},
		{Stage: StageReasoning, Status: StatusMissing, Path: ArtifactRelPath(sp, StageReasoning)},
		{Stage: StagePlan, Status: StatusMissing, Path: ArtifactRelPath(sp, StagePlan)},
	}
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

func flowAreaReasoningSuccessStages(sp Sprint, noTemplates bool, now time.Time) []StageState {
	stages := flowTechnicalHandbookSuccessStages(sp, noTemplates, now)
	if noTemplates {
		stages[3] = StageState{Stage: StageAreaReasoning, Status: StatusSkipped, Path: ArtifactRelPath(sp, StageAreaReasoning), LastRunAt: &now}
		stages[4] = StageState{Stage: StageReasoning, Status: StatusReady, Path: ArtifactRelPath(sp, StageReasoning)}
		return stages
	}
	stages[3] = StageState{Stage: StageAreaReasoning, Status: StatusComplete, Path: ArtifactRelPath(sp, StageAreaReasoning), LastRunAt: &now}
	stages[4] = StageState{Stage: StageReasoning, Status: StatusReady, Path: ArtifactRelPath(sp, StageReasoning)}
	return stages
}

func flowReasoningSuccessStages(sp Sprint, noTemplates bool, now time.Time) []StageState {
	stages := flowAreaReasoningSuccessStages(sp, noTemplates, now)
	stages[4] = StageState{Stage: StageReasoning, Status: StatusComplete, Path: ArtifactRelPath(sp, StageReasoning), LastRunAt: &now}
	stages[5] = StageState{Stage: StagePlan, Status: StatusMissing, Path: ArtifactRelPath(sp, StagePlan)}
	return stages
}

func flowPlanSuccessStages(sp Sprint, noTemplates bool, now time.Time) []StageState {
	stages := flowReasoningSuccessStages(sp, noTemplates, now)
	stages[5] = StageState{Stage: StagePlan, Status: StatusComplete, Path: ArtifactRelPath(sp, StagePlan), LastRunAt: &now}
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
	if target == StageRequirements {
		stages[0] = StageState{Stage: StageRequirements, Status: StatusFailed, Path: ArtifactRelPath(sp, StageRequirements), LastRunAt: &now, Error: msg}
		stages[1] = StageState{Stage: StageSprintIndex, Status: StatusMissing, Path: ArtifactRelPath(sp, StageSprintIndex)}
	}
	if target == StageTechnicalHandbook {
		stages[1] = StageState{Stage: StageSprintIndex, Status: StatusComplete, Path: ArtifactRelPath(sp, StageSprintIndex)}
		stages[2] = StageState{Stage: StageTechnicalHandbook, Status: StatusFailed, Path: ArtifactRelPath(sp, StageTechnicalHandbook), LastRunAt: &now, Error: msg}
	}
	if target == StageAreaReasoning {
		stages[1] = StageState{Stage: StageSprintIndex, Status: StatusComplete, Path: ArtifactRelPath(sp, StageSprintIndex)}
		stages[2] = StageState{Stage: StageTechnicalHandbook, Status: StatusComplete, Path: ArtifactRelPath(sp, StageTechnicalHandbook)}
		stages[3] = StageState{Stage: StageAreaReasoning, Status: StatusFailed, Path: ArtifactRelPath(sp, StageAreaReasoning), LastRunAt: &now, Error: msg}
	}
	if target == StageReasoning {
		stages[1] = StageState{Stage: StageSprintIndex, Status: StatusComplete, Path: ArtifactRelPath(sp, StageSprintIndex)}
		stages[2] = StageState{Stage: StageTechnicalHandbook, Status: StatusComplete, Path: ArtifactRelPath(sp, StageTechnicalHandbook)}
		stages[3] = StageState{Stage: StageAreaReasoning, Status: StatusComplete, Path: ArtifactRelPath(sp, StageAreaReasoning)}
		stages[4] = StageState{Stage: StageReasoning, Status: StatusFailed, Path: ArtifactRelPath(sp, StageReasoning), LastRunAt: &now, Error: msg}
	}
	if target == StagePlan {
		stages[1] = StageState{Stage: StageSprintIndex, Status: StatusComplete, Path: ArtifactRelPath(sp, StageSprintIndex)}
		stages[2] = StageState{Stage: StageTechnicalHandbook, Status: StatusComplete, Path: ArtifactRelPath(sp, StageTechnicalHandbook)}
		stages[3] = StageState{Stage: StageAreaReasoning, Status: StatusComplete, Path: ArtifactRelPath(sp, StageAreaReasoning)}
		stages[4] = StageState{Stage: StageReasoning, Status: StatusComplete, Path: ArtifactRelPath(sp, StageReasoning)}
		stages[5] = StageState{Stage: StagePlan, Status: StatusFailed, Path: ArtifactRelPath(sp, StagePlan), LastRunAt: &now, Error: msg}
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
	if stage != StageRequirements && stage != StageSprintIndex && stage != StageTechnicalHandbook && stage != StageAreaReasoning && stage != StageReasoning && stage != StagePlan && stage != StageExecute {
		return fmt.Errorf("unsupported sprint flow target %q; supports requirements, sprint-index, technical-handbook, area-reasoning, reasoning, plan, and execute", stage)
	}
	return nil
}
