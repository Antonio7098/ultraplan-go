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
	To       PlanningStage
	DryRun   bool
	Review   ReviewRequest
	Smoke    SmokeRequest
	Progress func(FlowProgress)
}

type FlowProgress struct {
	Stage   PlanningStage
	State   string
	Message string
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

// Flow owns the ordered sprint state machine. Surfaces map requests and render
// this result; they do not schedule stages or duplicate verification policy.
func (s Service) Flow(ctx context.Context, projectRef, sprintRef string, req FlowRequest) (FlowResult, error) {
	stages, err := flowStages(req.To)
	if err != nil {
		return FlowResult{}, err
	}
	if req.DryRun {
		if req.To == StageReview || req.To == StageSmoke {
			verified, verifyErr := s.Verify(ctx, projectRef, sprintRef, VerifyRequest{To: req.To, DryRun: true, Review: req.Review, Smoke: req.Smoke, Progress: req.Progress})
			message := "verification dry run"
			if verified.ReviewResult != nil {
				message = firstNonEmptyString(verified.ReviewResult.Prompt, verified.ReviewResult.Message, message)
			}
			return FlowResult{Project: verified.Project, Sprint: verified.Sprint, To: req.To, DryRun: true, Message: message}, verifyErr
		}
		stages = []PlanningStage{req.To}
	} else {
		// A non-dry flow is the materialization entrypoint for a roadmap sprint.
		// Create its safe skeleton before acquiring the sprint-scoped mutation
		// lease; lease resolution deliberately accepts existing sprints only.
		sp, _, _, resolveErr := s.resolveSprintForRequirements(projectRef, sprintRef, true)
		if resolveErr != nil {
			return FlowResult{}, resolveErr
		}
		sprintRef = sp.Slug
		var release func()
		ctx, release, err = s.acquireMutationContext(ctx, projectRef, sprintRef)
		if err != nil {
			return FlowResult{}, err
		}
		defer release()
	}
	var result FlowResult
	for _, stage := range stages {
		emitFlow(req.Progress, FlowProgress{Stage: stage, State: "checking", Message: "checking prerequisites and existing artifact"})
		stageReq := FlowRequest{To: stage, DryRun: req.DryRun}
		if !req.DryRun {
			valid, validateErr := s.flowStageAlreadyValid(projectRef, sprintRef, stage)
			if validateErr != nil {
				return FlowResult{}, validateErr
			}
			if valid {
				result = FlowResult{Project: projectRef, Sprint: sprintRef, To: stage, Message: string(stage) + " already complete"}
				emitFlow(req.Progress, FlowProgress{Stage: stage, State: "skipped", Message: "already complete"})
				continue
			}
			emitFlow(req.Progress, FlowProgress{Stage: stage, State: "running", Message: "starting runtime-backed stage"})
		}
		var stageErr error
		result, stageErr = s.runFlowStage(ctx, projectRef, sprintRef, stageReq)
		if stageErr != nil {
			emitFlow(req.Progress, FlowProgress{Stage: stage, State: "failed", Message: stageErr.Error()})
			return result, stageErr
		}
		emitFlow(req.Progress, FlowProgress{Stage: stage, State: "complete", Message: firstNonEmptyString(result.Message, "stage complete")})
	}
	if req.To == StageReview || req.To == StageSmoke {
		verified, verifyErr := s.Verify(ctx, projectRef, sprintRef, VerifyRequest{To: req.To, Review: req.Review, Smoke: req.Smoke, Progress: req.Progress})
		message := fmt.Sprintf("verification assessment=%s next=%s", verified.Verification.Assessment, verified.Verification.NextAction)
		return FlowResult{Project: verified.Project, Sprint: verified.Sprint, To: req.To, Message: message}, verifyErr
	}
	result.To = req.To
	return result, nil
}

// FlowStage runs exactly one planning stage. It preserves the stage's normal
// prerequisite validation while deliberately not scheduling earlier stages.
func (s Service) FlowStage(ctx context.Context, projectRef, sprintRef string, req FlowRequest) (FlowResult, error) {
	if err := validatePlanningStageTarget(req.To); err != nil {
		return FlowResult{}, err
	}
	if !req.DryRun {
		sp, _, _, resolveErr := s.resolveSprintForRequirements(projectRef, sprintRef, true)
		if resolveErr != nil {
			return FlowResult{}, resolveErr
		}
		sprintRef = sp.Slug
		var release func()
		var err error
		ctx, release, err = s.acquireMutationContext(ctx, projectRef, sprintRef)
		if err != nil {
			return FlowResult{}, err
		}
		defer release()
	}
	emitFlow(req.Progress, FlowProgress{Stage: req.To, State: "running", Message: "running selected stage only"})
	result, err := s.runFlowStage(ctx, projectRef, sprintRef, req)
	if err != nil {
		emitFlow(req.Progress, FlowProgress{Stage: req.To, State: "failed", Message: err.Error()})
		return result, err
	}
	emitFlow(req.Progress, FlowProgress{Stage: req.To, State: "complete", Message: firstNonEmptyString(result.Message, "stage complete")})
	return result, nil
}

func (s Service) runFlowStage(ctx context.Context, projectRef, sprintRef string, req FlowRequest) (FlowResult, error) {
	switch req.To {
	case StageRequirements:
		return s.FlowRequirements(ctx, projectRef, sprintRef, req)
	case StageSprintIndex:
		return s.FlowSprintIndex(ctx, projectRef, sprintRef, req)
	case StageTechnicalHandbook:
		return s.FlowTechnicalHandbook(ctx, projectRef, sprintRef, req)
	case StageAreaReasoning, StageReasoning:
		return s.FlowReasoning(ctx, projectRef, sprintRef, req)
	case StagePlan:
		return s.FlowPlan(ctx, projectRef, sprintRef, req)
	case StageExecute:
		execute, err := s.Execute(ctx, projectRef, sprintRef, ExecuteRequest{DryRun: req.DryRun, Resume: true})
		return FlowResult{Project: execute.Project, Sprint: execute.Sprint, To: StageExecute, DryRun: execute.DryRun, Message: firstNonEmptyString(execute.Prompt, execute.Message), Findings: execute.Findings}, err
	default:
		return FlowResult{}, fmt.Errorf("unsupported flow stage %q", req.To)
	}
}

func validatePlanningStageTarget(stage PlanningStage) error {
	switch stage {
	case StageRequirements, StageSprintIndex, StageTechnicalHandbook, StageAreaReasoning, StageReasoning, StagePlan:
		return nil
	default:
		return fmt.Errorf("unsupported single planning stage %q", stage)
	}
}

func flowStages(target PlanningStage) ([]PlanningStage, error) {
	if err := validateFlowTarget(target); err != nil {
		return nil, err
	}
	ordered := []PlanningStage{StageRequirements, StageSprintIndex, StageTechnicalHandbook, StageAreaReasoning, StageReasoning, StagePlan, StageExecute}
	end := 0
	switch target {
	case StageRequirements:
		end = 1
	case StageSprintIndex:
		end = 2
	case StageTechnicalHandbook:
		end = 3
	case StageAreaReasoning:
		end = 4
	case StageReasoning:
		end = 5
	case StagePlan:
		end = 6
	case StageExecute, StageReview, StageSmoke:
		end = 7
	}
	return append([]PlanningStage(nil), ordered[:end]...), nil
}

func (s Service) flowStageAlreadyValid(projectRef, sprintRef string, stage PlanningStage) (bool, error) {
	var result ValidationResult
	var err error
	switch stage {
	case StageRequirements:
		result, err = s.ValidateRequirements(projectRef, sprintRef)
	case StageSprintIndex:
		result, err = s.ValidateSprintIndex(projectRef, sprintRef)
	case StageTechnicalHandbook:
		result, err = s.ValidateTechnicalHandbook(projectRef, sprintRef)
	case StageAreaReasoning:
		result, err = s.ValidateAreaReasoning(projectRef, sprintRef)
	case StageReasoning:
		result, err = s.ValidateReasoning(projectRef, sprintRef)
	case StagePlan:
		result, err = s.ValidatePlan(projectRef, sprintRef)
	case StageExecute:
		return s.ExecuteComplete(projectRef, sprintRef)
	default:
		return false, fmt.Errorf("unsupported flow stage %q", stage)
	}
	if err != nil {
		return false, nil
	}
	return result.Valid(), nil
}

func emitFlow(progress func(FlowProgress), event FlowProgress) {
	if progress != nil {
		progress(event)
	}
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
	if stage != StageRequirements && stage != StageSprintIndex && stage != StageTechnicalHandbook && stage != StageAreaReasoning && stage != StageReasoning && stage != StagePlan && stage != StageExecute && stage != StageReview && stage != StageSmoke {
		return fmt.Errorf("unsupported sprint flow target %q; supports requirements, sprint-index, technical-handbook, area-reasoning, reasoning, plan, execute, review, and smoke", stage)
	}
	return nil
}
