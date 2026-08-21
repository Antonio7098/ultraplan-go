package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/Antonio7098/ultraplan-go/internal/app"
	"github.com/Antonio7098/ultraplan-go/internal/tui"
	"github.com/Antonio7098/ultraplan-go/internal/web"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(app.Run(app.Config{
		Args:    os.Args[1:],
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
		Context: ctx,
		Version: app.DefaultVersion(),
		TUIRunner: func(ctx context.Context, opts app.TUIRunOptions) error {
			return tui.Run(ctx, tui.Options{UseCases: opts.UseCases, Stdout: opts.Stdout, Width: opts.Width})
		},
		WebRunner: func(ctx context.Context, opts app.ServeRunOptions) error {
			return web.Run(ctx, web.Options{
				Listen:        opts.Listen,
				OpenBrowser:   opts.OpenBrowser,
				Queries:       opts.UseCases,
				Operations:    opts.UseCases,
				Runs:          opts.UseCases,
				Stdout:        opts.Stdout,
				Diagnostics:   opts.Diagnostics,
				LaunchBrowser: openBrowser,
			})
		},
	}))
}

func openBrowser(ctx context.Context, target string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var command string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		command, args = "open", []string{target}
	case "windows":
		command, args = "rundll32", []string{"url.dll,FileProtocolHandler", target}
	default:
		command, args = "xdg-open", []string{target}
	}
	if err := exec.CommandContext(ctx, command, args...).Run(); err != nil {
		return fmt.Errorf("launch browser: %w", err)
	}
	return nil
}
