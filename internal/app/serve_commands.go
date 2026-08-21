package app

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/Antonio7098/ultraplan-go/internal/platform/config"
)

const DefaultServeListen = "127.0.0.1:8080"

func runServe(deps dependencies, args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(deps.stderr)
	listen := fs.String("listen", DefaultServeListen, "numeric loopback listen address")
	openBrowser := fs.Bool("open-browser", false, "open the dashboard in the default browser")
	fs.Usage = func() { _, _ = deps.stdout.Write([]byte(serveHelp())) }
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return classified(ExitUsage, "serve: %w", err)
	}
	if fs.NArg() != 0 {
		return classified(ExitUsage, "serve: unexpected argument %q", fs.Arg(0))
	}
	if err := ValidateLoopbackListen(*listen); err != nil {
		return classified(ExitUsage, "serve.listen: %w", err)
	}

	root, err := discoverWorkspace(deps)
	if err != nil {
		return err
	}
	effective, err := loadEffectiveConfig(root, deps, configOverridesForServe())
	if err != nil {
		return err
	}
	if deps.webRunner == nil {
		return classified(ExitError, "serve.start: web runner is not configured")
	}
	dashboard := dashboardUseCases{
		root: root.Path, stageRuntime: planningStageRuntime(effective.Config),
		reviewConcurrency: effective.Config.Execution.DefaultParallel,
		smokeSettings:     smokeSettings(effective, envLookup(deps.env)), readOnly: true,
	}
	if errors.Is(deps.ctx.Err(), context.Canceled) {
		return nil
	}
	repository, _, err := runRepository(deps)
	if err != nil {
		return err
	}
	useCases := NewWebUseCases(root.Path, WebUseCaseOptions{
		StageRuntime:      planningStageRuntime(effective.Config),
		ReviewConcurrency: effective.Config.Execution.DefaultParallel,
		SmokeSettings:     smokeSettings(effective, envLookup(deps.env)),
		Runner:            sharedOperationRunner(deps, root, effective, dashboard),
		RunControl:        repositoryRunUseCases{repository: repository},
		DurableOperations: newDurableOperationManager(repository, deps.runControl.owner),
	})
	err = deps.webRunner(deps.ctx, ServeRunOptions{
		Listen:      *listen,
		OpenBrowser: *openBrowser,
		UseCases:    useCases,
		Stdout:      deps.stdout,
		Diagnostics: deps.stderr,
	})
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) && errors.Is(deps.ctx.Err(), context.Canceled) {
		return nil
	}
	return classifiedCause(ExitError, err, "serve.start")
}

// configOverridesForServe exists to make it explicit that serve uses the
// normal workspace/environment config validation without adding hidden web
// configuration sources in this sprint.
func configOverridesForServe() config.CLIOverrides {
	return config.CLIOverrides{}
}

// ValidateLoopbackListen accepts numeric IPv4 loopback literals or bracketed
// IPv6 loopback literals with an explicit non-zero port.
func ValidateLoopbackListen(value string) error {
	if strings.TrimSpace(value) != value || value == "" {
		return errors.New("listen address is required")
	}
	host, port, err := net.SplitHostPort(value)
	if err != nil {
		return fmt.Errorf("use a numeric loopback IP and explicit port: %w", err)
	}
	if strings.Contains(host, "%") {
		return errors.New("IPv6 zones are not supported")
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return errors.New("listen address must use a numeric loopback IP such as 127.0.0.1 or [::1]")
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return errors.New("listen port must be between 1 and 65535")
	}
	return nil
}

func serveHelp() string {
	return `ultraplan serve

Usage:
  ultraplan [--workspace <path>] serve [--listen <address>] [--open-browser]

Starts the guarded local browser dashboard. The server accepts only numeric
loopback addresses, exposes allowlisted app operations with current
confirmation and bounded SSE progress, and shuts down owned operations
gracefully on interrupt or process cancellation.

Options:
  --listen <address>   Loopback IP and port (default 127.0.0.1:8080).
  --open-browser       Open the canonical dashboard URL after listening.
  --workspace <path>   Select the workspace (global flag).
  -h, --help           Show help without starting a listener.
`
}
