package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/Antonio7098/ultraplan-go/internal/app"
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
	}))
}
