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

func flowSuccessStages(sp Sprint, noTemplates bool, now time.Time) []StageState {
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

func flowFailedStages(sp Sprint, err error, now time.Time) []StageState {
	msg := safeError(err)
	return []StageState{
		{Stage: StageRequirements, Status: StatusComplete, Path: ArtifactRelPath(sp, StageRequirements)},
		{Stage: StageSprintIndex, Status: StatusFailed, Path: ArtifactRelPath(sp, StageSprintIndex), LastRunAt: &now, Error: msg},
		{Stage: StageTechnicalHandbook, Status: StatusMissing, Path: ArtifactRelPath(sp, StageTechnicalHandbook)},
		{Stage: StageAreaReasoning, Status: StatusMissing, Path: ArtifactRelPath(sp, StageAreaReasoning)},
		{Stage: StageReasoning, Status: StatusMissing, Path: ArtifactRelPath(sp, StageReasoning)},
		{Stage: StagePlan, Status: StatusMissing, Path: ArtifactRelPath(sp, StagePlan)},
	}
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
	if stage != StageSprintIndex {
		return fmt.Errorf("unsupported sprint flow target %q; Sprint 18 supports only sprint-index", stage)
	}
	return nil
}
