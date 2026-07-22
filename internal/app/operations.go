package app

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/Antonio7098/ultraplan-go/internal/project"
	"github.com/Antonio7098/ultraplan-go/internal/sprint"
	"github.com/Antonio7098/ultraplan-go/internal/study"
)

// OperationalUseCases is the terminal-adapter boundary for interactive actions.
// Its values deliberately contain no CLI, runtime-provider, or TUI framework types.
type OperationalUseCases interface {
	ReadOnlyUseCases
	Validate(context.Context, ValidationRequest) (ValidationOperationResult, error)
	PrepareOperation(context.Context, OperationRequest) (Confirmation, error)
	RunOperation(context.Context, OperationRequest, func(OperationEvent)) (OperationResult, error)
}

type OperationKind string

const (
	OperationSprintStatus  OperationKind = "sprint-status"
	OperationPrompt        OperationKind = "sprint-prompt"
	OperationFlowDryRun    OperationKind = "sprint-flow-dry-run"
	OperationFlow          OperationKind = "sprint-flow"
	OperationExecuteStatus OperationKind = "execute-status"
	OperationExecuteDryRun OperationKind = "execute-dry-run"
	OperationExecuteStart  OperationKind = "execute-start"
	OperationExecuteResume OperationKind = "execute-resume"
	OperationReviewStatus  OperationKind = "review-status"
	OperationReviewDryRun  OperationKind = "review-dry-run"
	OperationReviewStart   OperationKind = "review-start"
	OperationSmokeStatus   OperationKind = "smoke-status"
	OperationSmokeDryRun   OperationKind = "smoke-dry-run"
	OperationSmokeStart    OperationKind = "smoke-start"
	OperationStudyStart    OperationKind = "study-start"
	OperationStudyResume   OperationKind = "study-resume"
	OperationStudyCancel   OperationKind = "study-cancel"
)

type OperationState string

const (
	OperationPreparing OperationState = "preparing"
	OperationRunning   OperationState = "running"
	OperationComplete  OperationState = "complete"
	OperationFailed    OperationState = "failed"
	OperationCancelled OperationState = "cancelled"
	OperationPartial   OperationState = "partial"
)

type OperationRequest struct {
	Kind                                OperationKind
	Project, Sprint, Study, Stage, Task string
	Level, Suite, Test                  string
	Timeout                             string
	ForceReview                         bool
	Sources, Dimensions                 []string
	Parallelism                         int
}
type Confirmation struct {
	Request                          OperationRequest
	Subject                          string
	Paths, Scope                     []string
	Runtime, Mutates                 bool
	Warning, ModelSource, Permission string
}
type OperationEvent struct {
	State                                                                         OperationState
	Stage, Task, Message                                                          string
	Completed, Total, Attempt                                                     int
	RuntimeAttempts                                                               int
	Turns                                                                         int64
	TurnsKnown                                                                    bool
	Tokens                                                                        int64
	TokensKnown                                                                   bool
	InputTokens, OutputTokens, ReasoningTokens, CacheReadTokens, CacheWriteTokens int64
	Duration, Provider, Model, Cost                                               string
	RuntimeEvents                                                                 int64
}
type OperationResult struct {
	State                     OperationState
	Subject, Message, Content string
	Truncated                 bool
	Findings                  []DisplayFinding
	Error                     *OperationError
}
type OperationError struct {
	Code, Category, Operation, Component, Message, Cause, Guidance string
	Retryable                                                      bool
}

func (u dashboardUseCases) PrepareOperation(ctx context.Context, req OperationRequest) (Confirmation, error) {
	if err := ctx.Err(); err != nil {
		return Confirmation{}, err
	}
	if (req.Kind == OperationReviewStart || req.Kind == OperationReviewDryRun) && req.Parallelism <= 0 {
		req.Parallelism = u.reviewConcurrency
	}
	c := Confirmation{Request: req, Subject: operationFirstNonEmpty(req.Project+"/"+req.Sprint, req.Study), Permission: "workspace policy enforced"}
	switch req.Kind {
	case OperationPrompt, OperationFlowDryRun, OperationExecuteDryRun, OperationExecuteStatus, OperationReviewDryRun, OperationReviewStatus, OperationSmokeStatus:
		c.Scope = []string{req.Stage}
		c.Warning = "runtime-free; no runtime-backed writes"
	case OperationSprintStatus:
		c.Mutates = true
		c.Scope = []string{"all sprint stages", "execute and review state"}
		c.Warning = "RUNTIME-FREE; MAY REFRESH FLOW-STATE.JSON"
	case OperationFlow:
		c.Runtime = true
		c.Mutates = true
		c.Scope = []string{"planning stages through " + req.Stage}
		c.Warning = "RUNTIME + WORKSPACE MUTATION"
	case OperationExecuteStart, OperationExecuteResume:
		c.Runtime = true
		c.Mutates = true
		c.Scope = []string{"execute tasks"}
		c.Warning = "RUNTIME + APPROVED TARGET MUTATION"
	case OperationReviewStart:
		c.Runtime = true
		c.Mutates = true
		c.Scope = []string{"one read-only reviewer per selected contract plus handbook", fmt.Sprintf("bounded parallelism: %d", req.Parallelism)}
		c.Warning = "RUNTIME + REVIEW ARTIFACT WRITE (TARGET READ-ONLY)"
	case OperationSmokeDryRun, OperationSmokeStart:
		preview, err := u.sprintService().RunSmoke(ctx, req.Project, req.Sprint, sprint.SmokeRequest{Level: req.Level, Suite: req.Suite, Test: req.Test, ForceReview: req.ForceReview, DryRun: true})
		if err != nil {
			return c, err
		}
		c.Runtime = true
		c.Mutates = req.Kind == OperationSmokeStart
		c.Scope = []string{fmt.Sprintf("%s %s", preview.ScopeKind, preview.Scope), preview.ScopeRationale, "prerequisites: " + strings.Join(preview.Prerequisites, ", "), "duration/cost: " + preview.DurationClass + "/" + preview.CostClass, "safe command: " + preview.SafeArgv, "evidence roots: " + strings.Join(preview.EvidenceRoots, ", ")}
		c.Warning = "EXTERNAL HARNESS + SMOKE ARTIFACT WRITE; RAW EVIDENCE REMAINS EXTERNAL"
	case OperationStudyStart, OperationStudyResume:
		if req.Parallelism < 1 || req.Parallelism > 64 {
			return c, fmt.Errorf("parallelism must be between 1 and 64")
		}
		c.Runtime = true
		c.Mutates = true
		c.Scope = []string{fmt.Sprintf("study run-loop with %d parallel workers", req.Parallelism)}
		c.Warning = "RUNTIME + STUDY STATE MUTATION"
	case OperationStudyCancel:
		c.Mutates = true
		c.Scope = []string{"active study run-loop"}
		c.Warning = "CANCEL ACTIVE RUN LOOP"
	default:
		return c, fmt.Errorf("unsupported operation %q", req.Kind)
	}
	if req.Project != "" {
		c.Paths = []string{"projects/" + req.Project + "/sprints/" + req.Sprint}
	} else {
		c.Paths = []string{"studies/" + req.Study}
	}
	return c, nil
}

func (u dashboardUseCases) RunOperation(ctx context.Context, req OperationRequest, emit func(OperationEvent)) (OperationResult, error) {
	if emit == nil {
		emit = func(OperationEvent) {}
	}
	emit(OperationEvent{State: OperationRunning, Stage: req.Stage, Message: "operation started"})
	if err := ctx.Err(); err != nil {
		return OperationResult{State: OperationCancelled}, err
	}
	ss := u.sprintService()
	result := OperationResult{State: OperationComplete, Subject: operationFirstNonEmpty(req.Project+"/"+req.Sprint, req.Study)}
	stage := sprint.PlanningStage(req.Stage)
	switch req.Kind {
	case OperationSprintStatus:
		r, err := ss.Status(req.Project, req.Sprint)
		if err != nil {
			return failedOperation(result, err)
		}
		result.Message = summarizeSprintStatus(r)
	case OperationPrompt:
		p, err := promptSprintStage(ss, req.Project, req.Sprint, stage)
		if err != nil {
			return failedOperation(result, err)
		}
		result.Content, result.Truncated = boundContent(p.Prompt)
	case OperationFlowDryRun:
		r, err := runSprintFlow(ctx, ss, req.Project, req.Sprint, sprint.FlowRequest{To: stage, DryRun: true})
		if err != nil {
			result = operationWithSprintFindings(result, r.Findings)
			return failedOperation(result, err)
		}
		result.Content, result.Truncated = boundContent(r.Message)
		result.Message = fmt.Sprintf("flow to %s dry run", stage)
	case OperationExecuteDryRun:
		r, err := ss.Execute(ctx, req.Project, req.Sprint, sprint.ExecuteRequest{DryRun: true, TaskID: req.Task})
		if err != nil {
			result = operationWithSprintFindings(result, r.Findings)
			return failedOperation(result, err)
		}
		result.Content, result.Truncated = boundContent(r.Prompt)
		result.Message = r.Message
	case OperationExecuteStatus:
		r, err := ss.Status(req.Project, req.Sprint)
		if err != nil {
			return failedOperation(result, err)
		}
		execute := summarizeExecute(r.ExecuteState)
		if !execute.Available {
			result.Message = execute.Message
		} else {
			result.Message = fmt.Sprintf("%d total: %d pending, %d running, %d complete, %d failed, %d cancelled", execute.Total, execute.Pending, execute.Running, execute.Complete, execute.Failed, execute.Cancelled)
		}
	case OperationReviewDryRun:
		r, err := ss.Review(ctx, req.Project, req.Sprint, sprint.ReviewRequest{DryRun: true, Concurrency: req.Parallelism})
		if err != nil {
			return failedOperation(result, err)
		}
		result.Content, result.Truncated = boundContent(r.Prompt)
		result.Message = r.Message
	case OperationReviewStatus:
		r, err := ss.Status(req.Project, req.Sprint)
		if err != nil {
			return failedOperation(result, err)
		}
		if r.Review == nil {
			result.Message = "review not started"
		} else {
			result.Message = fmt.Sprintf("%s verdict=%s stale=%t progress=%d/%d", r.Review.Status, r.Review.Verdict, r.Review.Stale, r.Review.Completed, r.Review.Total)
		}
	case OperationSmokeStatus:
		r, err := ss.SmokeStatus(req.Project, req.Sprint)
		if err != nil {
			return failedOperation(result, err)
		}
		result.Message = fmt.Sprintf("%s verdict=%s stale=%t run=%s next=%s", r.Status, r.Verdict, r.Stale, r.RunID, r.NextAction)
	case OperationSmokeDryRun:
		r, err := ss.RunSmoke(ctx, req.Project, req.Sprint, sprint.SmokeRequest{Level: req.Level, Suite: req.Suite, Test: req.Test, ForceReview: req.ForceReview, DryRun: true})
		if err != nil {
			return failedOperation(result, err)
		}
		result.Message = fmt.Sprintf("%s %s: %s", r.ScopeKind, r.Scope, r.ScopeRationale)
		result.Content, result.Truncated = boundContent(sprint.RenderSmoke(r))
	default:
		if u.runner == nil {
			return failedOperation(result, fmt.Errorf("runtime-backed operation is unavailable without configured composition"))
		}
		return u.runner(ctx, req, emit)
	}
	emit(OperationEvent{State: OperationComplete, Stage: req.Stage, Message: operationFirstNonEmpty(result.Message, "operation complete")})
	return result, nil
}

func summarizeSprintStatus(status sprint.StatusSummary) string {
	parts := make([]string, 0, len(status.Stages)+2)
	for _, stage := range status.Stages {
		parts = append(parts, fmt.Sprintf("%s=%s", stage.Stage, stage.Status))
	}
	execute := summarizeExecute(status.ExecuteState)
	if execute.Available {
		parts = append(parts, fmt.Sprintf("execute=%d/%d complete (%d failed, %d cancelled)", execute.Complete, execute.Total, execute.Failed, execute.Cancelled))
	} else {
		parts = append(parts, "execute=not started")
	}
	if status.Review == nil {
		parts = append(parts, "review=not started")
	} else {
		parts = append(parts, fmt.Sprintf("review=%s verdict=%s stale=%t", status.Review.Status, status.Review.Verdict, status.Review.Stale))
	}
	if status.Smoke == nil {
		parts = append(parts, "smoke=not started")
	} else {
		parts = append(parts, fmt.Sprintf("smoke=%s verdict=%s stale=%t", status.Smoke.Status, status.Smoke.Verdict, status.Smoke.Stale))
	}
	return strings.Join(parts, "\n")
}

func operationWithSprintFindings(result OperationResult, findings []sprint.ValidationFinding) OperationResult {
	for _, finding := range findings {
		result.Findings = append(result.Findings, sprintFinding(finding))
	}
	return result
}
func failedOperation(r OperationResult, err error) (OperationResult, error) {
	if errors.Is(err, context.Canceled) {
		r.State = OperationCancelled
	} else {
		r.State = OperationFailed
	}
	r.Message = displaySafe(err.Error())
	code, category, guidance := "internal.error", "internal", "Inspect durable state and retry after correcting the reported cause."
	if smokeErr, ok := sprint.AsSmokeError(err); ok {
		code, category, guidance = smokeErr.Code, smokeErr.Category, smokeErr.Guidance
		r.Error = &OperationError{Code: code, Category: category, Operation: "smoke", Component: "sprint", Message: r.Message, Cause: r.Message, Guidance: guidance, Retryable: category == "process" || category == "timeout"}
		return r, err
	}
	s := strings.ToLower(err.Error())
	switch {
	case errors.Is(err, context.Canceled):
		code, category, guidance = "workflow.cancelled", "cancellation", "Resume the operation when ready."
	case strings.Contains(s, "validation") || strings.Contains(s, "unsupported") || strings.Contains(s, "parallelism"):
		code, category, guidance = "validation.reference", "validation", "Correct the selected scope or artifacts and retry."
	case strings.Contains(s, "runtime") || strings.Contains(s, "provider"):
		code, category, guidance = "provider.runtime", "runtime", "Check runtime configuration and provider availability."
	case strings.Contains(s, "lock"):
		code, category, guidance = "workflow.locked", "concurrency", "Inspect the active or stale study lock before retrying."
	}
	r.Error = &OperationError{Code: code, Category: category, Operation: "tui.operation", Component: "app", Message: r.Message, Cause: r.Message, Guidance: guidance, Retryable: category == "runtime" || category == "concurrency"}
	return r, err
}
func boundContent(s string) (string, bool) {
	if len(s) > PreviewByteLimit {
		return s[:PreviewByteLimit], true
	}
	return s, false
}
func operationFirstNonEmpty(a, b string) string {
	if strings.Trim(a, "/") != "" {
		return strings.Trim(a, "/")
	}
	return b
}
func promptSprintStage(s sprint.Service, p, sp string, stage sprint.PlanningStage) (sprint.PromptPreview, error) {
	switch stage {
	case sprint.StageRequirements:
		return s.PromptRequirements(p, sp)
	case sprint.StageSprintIndex:
		return s.PromptSprintIndex(p, sp)
	case sprint.StageTechnicalHandbook:
		return s.PromptTechnicalHandbook(p, sp)
	case sprint.StageAreaReasoning:
		return s.PromptAreaReasoning(p, sp)
	case sprint.StageReasoning:
		return s.PromptReasoning(p, sp)
	case sprint.StagePlan:
		return s.PromptPlan(p, sp)
	case sprint.StageExecute:
		return s.PromptExecute(p, sp, sprint.ExecuteRequest{})
	case sprint.StageReview:
		return s.PromptReview(p, sp, sprint.ReviewRequest{})
	default:
		return sprint.PromptPreview{}, fmt.Errorf("unsupported prompt stage %q", stage)
	}
}

type ValidationSubject string

const (
	ValidationProject ValidationSubject = "project"
	ValidationStudy   ValidationSubject = "study"
	ValidationSprint  ValidationSubject = "sprint"
)

type ValidationRequest struct {
	Subject ValidationSubject
	Project string
	Study   string
	Sprint  string
	Stage   string
}

type ValidationOperationResult struct {
	Operation string
	Subject   string
	Status    string
	Findings  []DisplayFinding
}

func NewOperationalUseCases(root string) OperationalUseCases { return dashboardUseCases{root: root} }

func (u dashboardUseCases) Validate(ctx context.Context, req ValidationRequest) (ValidationOperationResult, error) {
	if err := ctx.Err(); err != nil {
		return ValidationOperationResult{}, err
	}
	result := ValidationOperationResult{Operation: "validate", Status: "valid"}
	switch req.Subject {
	case ValidationProject:
		ref := strings.TrimSpace(req.Project)
		validation, err := project.NewService(u.root).Validate(ref)
		if err != nil {
			return result, mapProjectError("project.validate", err)
		}
		result.Subject = ref
		result.Status = string(validation.Status)
		for _, finding := range validation.Findings {
			result.Findings = append(result.Findings, projectFinding(finding))
		}
	case ValidationStudy:
		ref := strings.TrimSpace(req.Study)
		validation, err := study.NewService(u.root).ValidateStudy(ref)
		if err != nil {
			return result, mapStudyError(err)
		}
		result.Subject = ref
		result.Status = string(validation.Status)
		for _, check := range validation.Checks {
			if check.Status != study.ValidationStatusPassed {
				result.Findings = append(result.Findings, studyFinding(check))
			}
		}
		for _, report := range validation.Reports {
			for _, check := range report.Checks {
				if check.Status != study.ValidationStatusPassed {
					result.Findings = append(result.Findings, studyFinding(check))
				}
			}
		}
	case ValidationSprint:
		stage := sprint.PlanningStage(strings.TrimSpace(req.Stage))
		validation, err := validateSprintStage(u.sprintService(), req.Project, req.Sprint, stage)
		if err != nil {
			return result, mapSprintError("sprint.validate", err)
		}
		result.Subject = strings.TrimSpace(req.Project) + "/" + strings.TrimSpace(req.Sprint) + "/" + string(stage)
		if !validation.Valid() {
			result.Status = "invalid"
		}
		for _, finding := range validation.Findings {
			result.Findings = append(result.Findings, sprintFinding(finding))
		}
	default:
		return result, fmt.Errorf("validate: unsupported subject %q", req.Subject)
	}
	sort.SliceStable(result.Findings, func(i, j int) bool {
		a, b := result.Findings[i], result.Findings[j]
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		if a.Section != b.Section {
			return a.Section < b.Section
		}
		return a.Problem < b.Problem
	})
	return result, nil
}
