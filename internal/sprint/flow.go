package sprint

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Antonio7098/ultraplan-go/internal/platform/config"
	"github.com/Antonio7098/ultraplan-go/internal/platform/runtime"
)

type Runtime interface {
	StartRun(context.Context, runtime.Request) (runtime.Result, error)
}

type sessionDeleter interface {
	DeleteSession(context.Context, string) error
}

func (s Service) deleteCompletedSession(ctx context.Context, sessionID string) error {
	if strings.TrimSpace(sessionID) == "" {
		return nil
	}
	deleter, ok := s.runtime.(sessionDeleter)
	if !ok {
		return nil
	}
	return deleter.DeleteSession(ctx, sessionID)
}

func (s Service) deleteCompletedSessions(ctx context.Context, result runtime.Result) error {
	ids := append([]string(nil), result.SessionIDs...)
	ids = append(ids, result.SessionID)
	seen := map[string]bool{}
	for _, id := range ids {
		if strings.TrimSpace(id) == "" || seen[id] {
			continue
		}
		seen[id] = true
		if err := s.deleteCompletedSession(ctx, id); err != nil {
			return err
		}
	}
	return nil
}

type FlowRequest struct {
	To              PlanningStage
	DryRun          bool
	ModelOverride   string
	VariantOverride string
	// StageOverrides optionally sets a per-stage runtime model and/or variant
	// for this flow invocation. Stages without an entry keep their configured
	// defaults unchanged.
	StageOverrides map[PlanningStage]StageRuntime
	Review         ReviewRequest
	Smoke          SmokeRequest
	Progress       func(FlowProgress)
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
		stageReq := FlowRequest{To: stage, DryRun: req.DryRun, ModelOverride: req.ModelOverride, VariantOverride: req.VariantOverride}
		if !req.DryRun {
			valid, validateErr := s.flowStageAlreadyValid(projectRef, sprintRef, stage)
			if validateErr != nil {
				return FlowResult{}, validateErr
			}
			if valid {
				if stage == StageCodeContext {
					sp, inputs, _, resolveErr := s.resolveSprintInputs(projectRef, sprintRef)
					if resolveErr != nil {
						return FlowResult{}, resolveErr
					}
					if _, findings := s.resolveSprintTarget(sp, inputs.ProjectIndex, true); len(findings) > 0 {
						return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: stage, Findings: findings}, fmt.Errorf("code-context sprint workspace creation failed")
					}
				}
				if stage != StageExecute {
					if sp, _, _, resolveErr := s.resolveSprintInputs(projectRef, sprintRef); resolveErr == nil {
						_ = clearPlanningStageSession(sp, stage)
					}
				}
				result = FlowResult{Project: projectRef, Sprint: sprintRef, To: stage, Message: string(stage) + " already complete"}
				emitFlow(req.Progress, FlowProgress{Stage: stage, State: "skipped", Message: "already complete"})
				continue
			}
			emitFlow(req.Progress, FlowProgress{Stage: stage, State: "running", Message: "starting runtime-backed stage"})
		}
		var stageErr error
		result, stageErr = s.withStageOverrides(req.StageOverrides).runFlowStage(ctx, projectRef, sprintRef, stageReq)
		if stageErr != nil {
			emitFlow(req.Progress, FlowProgress{Stage: stage, State: "failed", Message: "stage failed; inspect validation findings and durable state"})
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
	result, err := s.withStageOverrides(req.StageOverrides).runFlowStage(ctx, projectRef, sprintRef, req)
	if err != nil {
		emitFlow(req.Progress, FlowProgress{Stage: req.To, State: "failed", Message: "stage failed; inspect validation findings and durable state"})
		return result, err
	}
	emitFlow(req.Progress, FlowProgress{Stage: req.To, State: "complete", Message: firstNonEmptyString(result.Message, "stage complete")})
	return result, nil
}

func (s Service) runFlowStage(ctx context.Context, projectRef, sprintRef string, req FlowRequest) (FlowResult, error) {
	switch req.To {
	case StageRequirements:
		return s.FlowRequirements(ctx, projectRef, sprintRef, req)
	case StageCodeContext:
		return s.FlowCodeContext(ctx, projectRef, sprintRef, req)
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
	case StageRequirements, StageCodeContext, StageSprintIndex, StageTechnicalHandbook, StageAreaReasoning, StageReasoning, StagePlan:
		return nil
	default:
		return fmt.Errorf("unsupported single planning stage %q", stage)
	}
}

func flowStages(target PlanningStage) ([]PlanningStage, error) {
	if err := validateFlowTarget(target); err != nil {
		return nil, err
	}
	ordered := append(PlanningStages(), StageExecute)
	end := 0
	switch target {
	case StageRequirements:
		end = 1
	case StageCodeContext:
		end = 2
	case StageSprintIndex:
		end = 3
	case StageTechnicalHandbook:
		end = 4
	case StageAreaReasoning:
		end = 5
	case StageReasoning:
		end = 6
	case StagePlan:
		end = 7
	case StageExecute, StageReview, StageSmoke:
		end = 8
	}
	return append([]PlanningStage(nil), ordered[:end]...), nil
}

func (s Service) flowStageAlreadyValid(projectRef, sprintRef string, stage PlanningStage) (bool, error) {
	var result ValidationResult
	var err error
	switch stage {
	case StageRequirements:
		result, err = s.ValidateRequirements(projectRef, sprintRef)
	case StageCodeContext:
		sp, _, _, resolveErr := s.resolveSprintInputs(projectRef, sprintRef)
		if resolveErr != nil {
			return false, resolveErr
		}
		_, prerequisiteErr := s.codeContextPrerequisite(sp)
		return prerequisiteErr == nil, nil
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
	stages := emptyPlanningStageStates(sp)
	setFlowStage(stages, StageRequirements, StatusComplete, &now, "")
	setFlowStage(stages, StageCodeContext, StatusReady, nil, "")
	return stages
}

func flowCodeContextSuccessStages(sp Sprint, now time.Time) []StageState {
	stages := flowRequirementsSuccessStages(sp, now)
	setFlowStage(stages, StageCodeContext, StatusComplete, &now, "")
	setFlowStage(stages, StageSprintIndex, StatusReady, nil, "")
	return stages
}

func flowSprintIndexSuccessStages(sp Sprint, noTemplates bool, now time.Time) []StageState {
	stages := flowCodeContextSuccessStages(sp, now)
	setFlowStage(stages, StageSprintIndex, StatusComplete, &now, "")
	setFlowStage(stages, StageTechnicalHandbook, StatusReady, nil, "")
	if noTemplates {
		setFlowStage(stages, StageAreaReasoning, StatusSkipped, nil, "")
		setFlowStage(stages, StageReasoning, StatusReady, nil, "")
	}
	return stages
}

func flowTechnicalHandbookSuccessStages(sp Sprint, noTemplates bool, now time.Time) []StageState {
	stages := flowSprintIndexSuccessStages(sp, false, now)
	setFlowStage(stages, StageTechnicalHandbook, StatusComplete, &now, "")
	setFlowStage(stages, StageAreaReasoning, StatusReady, nil, "")
	if noTemplates {
		setFlowStage(stages, StageAreaReasoning, StatusSkipped, nil, "")
		setFlowStage(stages, StageReasoning, StatusReady, nil, "")
	}
	return stages
}

func flowAreaReasoningSuccessStages(sp Sprint, noTemplates bool, now time.Time) []StageState {
	stages := flowTechnicalHandbookSuccessStages(sp, noTemplates, now)
	if noTemplates {
		setFlowStage(stages, StageAreaReasoning, StatusSkipped, &now, "")
		setFlowStage(stages, StageReasoning, StatusReady, nil, "")
		return stages
	}
	setFlowStage(stages, StageAreaReasoning, StatusComplete, &now, "")
	setFlowStage(stages, StageReasoning, StatusReady, nil, "")
	return stages
}

func flowReasoningSuccessStages(sp Sprint, noTemplates bool, now time.Time) []StageState {
	stages := flowAreaReasoningSuccessStages(sp, noTemplates, now)
	setFlowStage(stages, StageReasoning, StatusComplete, &now, "")
	setFlowStage(stages, StagePlan, StatusReady, nil, "")
	return stages
}

func flowPlanSuccessStages(sp Sprint, noTemplates bool, now time.Time) []StageState {
	stages := flowReasoningSuccessStages(sp, noTemplates, now)
	setFlowStage(stages, StagePlan, StatusComplete, &now, "")
	return stages
}

func flowFailedStages(sp Sprint, target PlanningStage, err error, now time.Time) []StageState {
	msg := safeError(err)
	stages := emptyPlanningStageStates(sp)
	for _, stage := range PlanningStages() {
		if stage == target {
			setFlowStage(stages, stage, StatusFailed, &now, msg)
			break
		}
		setFlowStage(stages, stage, StatusComplete, nil, "")
	}
	return stages
}

func emptyPlanningStageStates(sp Sprint) []StageState {
	stages := make([]StageState, 0, len(PlanningStages()))
	for _, stage := range PlanningStages() {
		stages = append(stages, StageState{Stage: stage, Status: StatusMissing, Path: ArtifactRelPath(sp, stage)})
	}
	return stages
}

func setFlowStage(stages []StageState, target PlanningStage, status StageStatus, at *time.Time, detail string) {
	for i := range stages {
		if stages[i].Stage == target {
			stages[i].Status, stages[i].LastRunAt, stages[i].Error = status, at, detail
			return
		}
	}
}

func safeError(err error) string {
	if err == nil {
		return ""
	}
	msg := config.RedactValue("sprint.stage_error", err.Error())
	msg = strings.ReplaceAll(msg, "\n", " ")
	msg = strings.ReplaceAll(msg, "\r", " ")
	msg = strings.ReplaceAll(msg, "\x00", "")
	if len(msg) > 180 {
		msg = msg[:180]
	}
	return msg
}

func validateFlowTarget(stage PlanningStage) error {
	if stage != StageRequirements && stage != StageCodeContext && stage != StageSprintIndex && stage != StageTechnicalHandbook && stage != StageAreaReasoning && stage != StageReasoning && stage != StagePlan && stage != StageExecute && stage != StageReview && stage != StageSmoke {
		return fmt.Errorf("unsupported sprint flow target %q; supports requirements, code-context, sprint-index, technical-handbook, area-reasoning, reasoning, plan, execute, review, and smoke", stage)
	}
	return nil
}
