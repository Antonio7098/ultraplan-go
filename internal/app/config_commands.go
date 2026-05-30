package app

import (
	"fmt"
	"strings"

	"ultraplan-go/internal/platform/config"
)

func runConfig(deps dependencies, args []string) error {
	if len(args) == 0 {
		return classified(ExitUsage, "config requires a subcommand\n\nRun 'ultraplan config show --help' for usage.")
	}
	if args[0] == "--help" || args[0] == "-h" {
		_, err := deps.stdout.Write([]byte(configHelp()))
		return err
	}
	if args[0] != "show" {
		return classified(ExitUsage, "config: unknown subcommand %q", args[0])
	}
	jsonOut := false
	for _, arg := range args[1:] {
		switch arg {
		case "--help", "-h":
			_, err := deps.stdout.Write([]byte(configShowHelp()))
			return err
		case "--json":
			jsonOut = true
		default:
			return classified(ExitUsage, "config show: unknown argument %q", arg)
		}
	}
	root, err := discoverWorkspace(deps)
	if err != nil {
		return err
	}
	effective, err := loadEffectiveConfig(root, deps, config.CLIOverrides{JSON: jsonOut})
	if err != nil {
		return err
	}
	redacted := config.Redact(effective)
	if jsonOut {
		return writeJSON(deps.stdout, "config show", root.Path, "ok", redacted)
	}
	fmt.Fprintf(deps.stdout, "Workspace: %s\n", root.Path)
	fmt.Fprintf(deps.stdout, "version: %d\n", redacted.Version)
	fmt.Fprintf(deps.stdout, "runtime.default: %s\n", redacted.Runtime.Default)
	fmt.Fprintf(deps.stdout, "models.default: %s\n", redacted.Models.Default)
	fmt.Fprintf(deps.stdout, "models.primary: %s\n", redacted.Models.Primary)
	fmt.Fprintf(deps.stdout, "models.backup: %s\n", redacted.Models.Backup)
	fmt.Fprintf(deps.stdout, "execution.default_variant: %s\n", redacted.Execution.DefaultVariant)
	fmt.Fprintf(deps.stdout, "execution.default_parallel: %d\n", redacted.Execution.DefaultParallel)
	fmt.Fprintf(deps.stdout, "execution.default_timeout: %s\n", redacted.Execution.DefaultTimeout)
	fmt.Fprintf(deps.stdout, "execution.default_retries: %d\n", redacted.Execution.DefaultRetries)
	fmt.Fprintf(deps.stdout, "logging.format: %s\n", redacted.Logging.Format)
	fmt.Fprintf(deps.stdout, "logging.level: %s\n", redacted.Logging.Level)
	fmt.Fprintf(deps.stdout, "agentwrap.executable: %s\n", redacted.Agentwrap.Executable)
	fmt.Fprintf(deps.stdout, "agentwrap.required_health: %s\n", strings.Join(redacted.Agentwrap.RequiredHealth, ", "))
	return nil
}

func configHelp() string {
	return `ultraplan config

Usage:
  ultraplan config show [--json]

Commands:
  show   Print effective configuration.
`
}

func configShowHelp() string {
	return `ultraplan config show

Usage:
  ultraplan config show [--json]

Flags:
  --json      Print JSON output.
  -h, --help  Show help.
`
}
