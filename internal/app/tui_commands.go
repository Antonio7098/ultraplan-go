package app

import (
	"context"
	"fmt"
	"io"

	"github.com/Antonio7098/ultraplan-go/internal/platform/config"
)

type TUIRunOptions struct {
	UseCases ReadOnlyUseCases
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
	useCases := NewReadOnlyUseCases(root.Path)
	if err := tuiRunner(deps.ctx, TUIRunOptions{UseCases: useCases, Stdout: deps.stdout, Width: 100}); err != nil {
		return classified(ExitError, "tui.start: %w", err)
	}
	return nil
}

func tuiHelp() string {
	return `ultraplan tui

Usage:
  ultraplan [--workspace <path>] tui

Starts a read-only terminal dashboard for workspace, project, study, and sprint
state. Navigation and preview actions do not run workflows. Refresh may
recompute deterministic sprint flow-state.json status.
`
}
