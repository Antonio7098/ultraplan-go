package app

import (
	"context"
	"io"
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
	effective, err := loadEffectiveConfig(root, deps, config.CLIOverrides{})
	if err != nil {
		return err
	}
	useCases := dashboardUseCases{
		root: root.Path, stageRuntime: planningStageRuntime(effective.Config),
		reviewConcurrency: effective.Config.Execution.DefaultParallel,
		smokeSettings:     smokeSettings(effective, envLookup(deps.env)),
	}
	useCases.runner = sharedOperationRunner(deps, root, effective, useCases)
	if deps.tuiRunner == nil {
		return classified(ExitError, "tui.start: tui runner is not configured")
	}
	if err := deps.tuiRunner(deps.ctx, TUIRunOptions{UseCases: useCases, Stdout: deps.stdout, Width: 100}); err != nil {
		return classified(ExitError, "tui.start: %w", err)
	}
	return nil
}

func tuiSprintRuntimeProgress(emit func(OperationEvent)) func(sprint.RuntimeProgress) {
	return func(progress sprint.RuntimeProgress) {
		if !runtimeEventIsProgress(progress.Event) {
			return
		}
		task := operationFirstNonEmpty(progress.Task, progress.CoverageID)
		emit(OperationEvent{State: OperationRunning, Stage: string(progress.Stage), Task: task, Message: runtimeProgressSummary(progress.Event), RuntimeEvents: 1})
	}
}

func operationTaskStats(task study.TaskState, now time.Time) RunTaskSummary {
	return runTaskSummary(task, now)
}

func tuiHelp() string {
	return `ultraplan tui

Usage:
  ultraplan [--workspace <path>] tui

Starts an operational terminal dashboard for workspace, project, study, and
sprint state. Every sprint status, validation, prompt, flow, execute, and review,
plus review-gated smoke,
operation is available. Runtime-backed or mutating actions require confirmation;
validation, prompt previews, and dry runs do not invoke the runtime. Refresh and
sprint status may recompute deterministic sprint flow-state.json status.
`
}
