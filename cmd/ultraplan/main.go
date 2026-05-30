package main

import (
	"os"

	"ultraplan-go/internal/app"
)

func main() {
	os.Exit(app.Run(app.Config{
		Args:    os.Args[1:],
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
		Version: app.DefaultVersion(),
	}))
}
