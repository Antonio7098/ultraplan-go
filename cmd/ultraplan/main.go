package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/Antonio7098/ultraplan-go/internal/app"
	"github.com/Antonio7098/ultraplan-go/internal/tui"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	app.SetTUIRunner(func(ctx context.Context, opts app.TUIRunOptions) error {
		return tui.Run(ctx, tui.Options{UseCases: opts.UseCases, Stdout: opts.Stdout, Width: opts.Width})
	})
	os.Exit(app.Run(app.Config{
		Args:    os.Args[1:],
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
		Context: ctx,
		Version: app.DefaultVersion(),
	}))
}
