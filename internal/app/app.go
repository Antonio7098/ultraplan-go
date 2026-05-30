package app

import (
	"fmt"
	"io"
)

const (
	ExitOK    = 0
	ExitError = 1
	ExitUsage = 2
)

type Config struct {
	Args    []string
	Stdout  io.Writer
	Stderr  io.Writer
	Version Version
}

func Run(cfg Config) int {
	stdout := cfg.Stdout
	if stdout == nil {
		stdout = io.Discard
	}

	stderr := cfg.Stderr
	if stderr == nil {
		stderr = io.Discard
	}

	version := cfg.Version
	if version.IsZero() {
		version = DefaultVersion()
	}

	if len(cfg.Args) == 0 {
		return writeStatus(stdout, renderHelp())
	}

	switch cfg.Args[0] {
	case "--help", "-h":
		return writeStatus(stdout, renderHelp())
	case "version":
		return writeStatus(stdout, renderVersion(version))
	default:
		if _, err := fmt.Fprintf(stderr, "unknown command %q\n\nRun 'ultraplan --help' to see available commands.\n", cfg.Args[0]); err != nil {
			return ExitError
		}
		return ExitUsage
	}
}

func writeStatus(w io.Writer, text string) int {
	if _, err := io.WriteString(w, text); err != nil {
		return ExitError
	}
	return ExitOK
}

func renderHelp() string {
	return `ultraplan

Usage:
  ultraplan [command]

Commands:
  version   Print build metadata.

Flags:
  -h, --help   Show help.
`
}

func renderVersion(version Version) string {
	return fmt.Sprintf("Version: %s\nCommit: %s\nBuildDate: %s\nGoVersion: %s\n",
		version.Version,
		version.Commit,
		version.BuildDate,
		version.GoVersion,
	)
}
