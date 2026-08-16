package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Antonio7098/ultraplan-go/internal/platform/config"
	"github.com/Antonio7098/ultraplan-go/internal/sprint"
	"github.com/Antonio7098/ultraplan-go/internal/study"
	"github.com/Antonio7098/ultraplan-go/internal/workspace"
)

// sharedOperationRunner is the single runtime-backed implementation used by
// terminal and browser adapters. Surface code owns presentation and
// confirmation; workflow semantics remain here and in the product services.
func sharedOperationRunner(deps dependencies, root workspace.Root, effective config.Effective, useCases dashboardUseCases) func(context.Context, OperationRequest, func(OperationEvent)) (OperationResult, error) {
	return func(ctx context.Context, req OperationRequest, emit func(OperationEvent)) (OperationResult, error) {
		result := OperationResult{State: OperationComplete, Subject: operationFirstNonEmpty(req.Project+"/"+req.Sprint, req.Study)}
		switch req.Kind {
		case OperationFlow:
			service, e := sprintRuntimeService(deps, root, tuiSprintRuntimeProgress(emit))
			if e != nil {
				return failedOperation(result, e)
			}
			r, e := runSprintFlow(ctx, service, req.Project, req.Sprint, sprint.FlowRequest{To: sprint.PlanningStage(req.Stage), Review: sprint.ReviewRequest{Restart: req.RestartReview}, Smoke: sprint.SmokeRequest{NonInteractive: true, OverrideConfirmed: req.ForceReview, ForceReview: req.ForceReview, OverrideRationale: req.OverrideRationale}, Progress: func(progress sprint.FlowProgress) {
				emit(OperationEvent{State: OperationRunning, Stage: string(progress.Stage), Message: progress.State + ": " + displaySafe(progress.Message)})
			}})
			result.Message = r.Message
			result = operationWithSprintFindings(result, r.Findings)
			if e != nil {
				return failedOperation(result, e)
			}
		case OperationExecuteStart, OperationExecuteResume:
			service, e := sprintRuntimeService(deps, root, tuiSprintRuntimeProgress(emit))
			if e != nil {
				return failedOperation(result, e)
			}
			r, e := service.Execute(ctx, req.Project, req.Sprint, sprint.ExecuteRequest{TaskID: req.Task, Resume: req.Kind == OperationExecuteResume})
			result.Message = r.Message
			result = operationWithSprintFindings(result, r.Findings)
			if e != nil {
				return failedOperation(result, e)
			}
		case OperationReviewStart:
			service, e := sprintRuntimeService(deps, root, tuiSprintRuntimeProgress(emit))
			if e != nil {
				return failedOperation(result, e)
			}
			r, e := service.Review(ctx, req.Project, req.Sprint, sprint.ReviewRequest{Concurrency: req.Parallelism, Focus: req.ReviewFocus, Restart: req.RestartReview, Progress: func(p sprint.ReviewProgress) {
				emit(OperationEvent{State: OperationRunning, Stage: "review", Task: p.CoverageID, Message: p.Message, Completed: p.Completed, Total: p.Total})
			}})
			result.Message = fmt.Sprintf("%s verdict=%s", r.Status, r.Verdict)
			for _, f := range r.Findings {
				result.Findings = append(result.Findings, DisplayFinding{Severity: f.Severity, Section: "review", Problem: f.Title, Cause: f.Detail, Suggestion: f.Action})
			}
			if e != nil {
				return failedOperation(result, e)
			}
		case OperationSmokeStart:
			service := sprint.NewService(root.Path).WithSmokeSettings(smokeSettings(effective, envLookup(deps.env)))
			var timeout time.Duration
			if req.Timeout != "" {
				timeout, _ = time.ParseDuration(req.Timeout)
			}
			r, e := service.RunSmoke(ctx, req.Project, req.Sprint, sprint.SmokeRequest{Level: req.Level, Suite: req.Suite, Test: req.Test, Timeout: timeout, ForceReview: req.ForceReview, OverrideConfirmed: req.ForceReview, OverrideRationale: req.OverrideRationale, Progress: func(p sprint.SmokeProgress) {
				emit(OperationEvent{State: OperationRunning, Stage: string(p.Phase), Task: operationFirstNonEmpty(p.Test, p.Suite), Message: p.Message, Completed: p.Completed, Total: p.Total})
			}})
			result.Message = fmt.Sprintf("%s verdict=%s run=%s next=%s", r.Status, r.Verdict, r.RunID, r.NextAction)
			if r.Artifact != "" {
				if preview, readErr := useCases.PreviewArtifact(ctx, r.Artifact); readErr == nil {
					result.Content, result.Truncated = boundContent(preview.Content)
				}
			}
			if e != nil {
				return failedOperation(result, e)
			}
		case OperationVerifyStart:
			service, e := sprintRuntimeService(deps, root, tuiSprintRuntimeProgress(emit))
			if e != nil {
				return failedOperation(result, e)
			}
			var timeout time.Duration
			if req.Timeout != "" {
				timeout, _ = time.ParseDuration(req.Timeout)
			}
			r, e := service.Verify(ctx, req.Project, req.Sprint, sprint.VerifyRequest{To: sprint.PlanningStage(req.Stage), Review: sprint.ReviewRequest{Focus: req.ReviewFocus, Restart: req.RestartReview}, Smoke: sprint.SmokeRequest{Level: req.Level, Suite: req.Suite, Test: req.Test, Timeout: timeout, ForceReview: req.ForceReview, OverrideConfirmed: req.ForceReview, OverrideRationale: req.OverrideRationale}, Progress: func(p sprint.FlowProgress) {
				emit(OperationEvent{State: OperationRunning, Stage: string(p.Stage), Message: p.State + ": " + p.Message})
			}})
			result.Message = fmt.Sprintf("assessment=%s next=%s", r.Verification.Assessment, r.Verification.NextAction)
			if e != nil {
				return failedOperation(result, e)
			}
		case OperationStudyStart, OperationStudyResume:
			flags := runAllFlags{}
			flags.parallelism = &req.Parallelism
			service, parallel, summary, e := runLoopService(deps, root, flags)
			if e != nil {
				return failedOperation(result, e)
			}
			r, e := service.RunLoop(ctx, study.RunLoopRequest{StudyRef: req.Study, DimensionRefs: req.Dimensions, SourceRefs: req.Sources, Parallelism: parallel, Config: summary, Continue: req.Kind == OperationStudyResume, Command: []string{"ultraplan", "operation"}, Progress: func(p study.RunLoopProgress) {
				stats := operationTaskStats(p.Task, time.Now().UTC())
				emit(OperationEvent{State: OperationRunning, Task: p.Task.ID, Stage: string(p.Event), Message: strings.TrimSpace(p.Task.DimensionRef + " " + p.Task.Source), Completed: p.ScopeCounts.Completed, Total: p.ScopeCounts.Total, Attempt: p.Task.Attempts, RuntimeAttempts: stats.RuntimeAttempts, Turns: stats.Turns, TurnsKnown: stats.TurnsKnown, Tokens: stats.Tokens, TokensKnown: stats.TokensKnown, InputTokens: stats.InputTokens, OutputTokens: stats.OutputTokens, ReasoningTokens: stats.ReasoningTokens, CacheReadTokens: stats.CacheReadTokens, CacheWriteTokens: stats.CacheWriteTokens, Duration: stats.Duration, Provider: stats.Provider, Model: stats.Model, Cost: stats.Cost, RuntimeEvents: stats.Events})
			}})
			result.Message = string(r.Status)
			if e != nil {
				return failedOperation(result, e)
			}
		case OperationStudyCancel:
			service := study.NewService(root.Path)
			listing, e := service.ListStudy(req.Study)
			if e != nil {
				return failedOperation(result, e)
			}
			info, e := study.CancelRunLoop(listing.Study)
			if e != nil {
				return failedOperation(result, e)
			}
			result.Message = fmt.Sprintf("cancellation requested from run-loop process %d", info.PID)
		default:
			return failedOperation(result, fmt.Errorf("unsupported runtime operation %q", req.Kind))
		}
		emit(OperationEvent{State: OperationComplete, Message: "operation complete"})
		return result, nil
	}
}
