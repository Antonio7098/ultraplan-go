package app

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Antonio7098/ultraplan-go/internal/platform/config"
	"github.com/Antonio7098/ultraplan-go/internal/sprint"
	"github.com/Antonio7098/ultraplan-go/internal/study"
)

type TUIRunOptions struct {
	UseCases OperationalUseCases
	Stdout   io.Writer
	Width    int
}

type TUIRunner func(context.Context, TUIRunOptions) error

var tuiRunner TUIRunner = func(context.Context, TUIRunOptions) error {
	return fmt.Errorf("tui runner is not configured")
}

func SetTUIRunner(runner TUIRunner) {
	if runner == nil {
		tuiRunner = func(context.Context, TUIRunOptions) error {
			return fmt.Errorf("tui runner is not configured")
		}
		return
	}
	tuiRunner = runner
}

func runTUI(deps dependencies, args []string) error {
	if len(args) > 0 {
		if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
			_, err := deps.stdout.Write([]byte(tuiHelp()))
			return err
		}
		return classified(ExitUsage, "tui: unknown argument %q", args[0])
	}
	root, err := discoverWorkspace(deps)
	if err != nil {
		return err
	}
	if _, err := loadEffectiveConfig(root, deps, config.CLIOverrides{}); err != nil {
		return err
	}
	useCases := dashboardUseCases{root: root.Path}
	useCases.runner = func(ctx context.Context, req OperationRequest, emit func(OperationEvent)) (OperationResult, error) {
		result := OperationResult{State: OperationComplete, Subject: operationFirstNonEmpty(req.Project+"/"+req.Sprint, req.Study)}
		switch req.Kind {
		case OperationFlow:
			service, e := sprintRuntimeService(deps, root)
			if e != nil {
				return failedOperation(result, e)
			}
			r, e := runSprintFlow(ctx, service, req.Project, req.Sprint, sprint.FlowRequest{To: sprint.PlanningStage(req.Stage)})
			result.Message = r.Message
			if e != nil {
				return failedOperation(result, e)
			}
		case OperationExecuteStart, OperationExecuteResume:
			service, e := sprintRuntimeService(deps, root)
			if e != nil {
				return failedOperation(result, e)
			}
			r, e := service.Execute(ctx, req.Project, req.Sprint, sprint.ExecuteRequest{TaskID: req.Task, Resume: req.Kind == OperationExecuteResume})
			result.Message = r.Message
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
			r, e := service.RunLoop(ctx, study.RunLoopRequest{StudyRef: req.Study, DimensionRefs: req.Dimensions, SourceRefs: req.Sources, Parallelism: parallel, Config: summary, Continue: req.Kind == OperationStudyResume, Command: []string{"ultraplan", "tui"}, Progress: func(p study.RunLoopProgress) {
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
	if err := tuiRunner(deps.ctx, TUIRunOptions{UseCases: useCases, Stdout: deps.stdout, Width: 100}); err != nil {
		return classified(ExitError, "tui.start: %w", err)
	}
	return nil
}

func operationTaskStats(task study.TaskState, now time.Time) RunTaskSummary {
	return runTaskSummary(task, now)
}

func tuiHelp() string {
	return `ultraplan tui

Usage:
  ultraplan [--workspace <path>] tui

Starts a read-only terminal dashboard with validation controls for workspace,
project, study, and sprint state. Validation actions and artifact previews do not
run workflows. Refresh may
recompute deterministic sprint flow-state.json status.
`
}
