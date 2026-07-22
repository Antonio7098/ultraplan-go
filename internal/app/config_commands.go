package app

import (
	"fmt"
	"strings"

	"github.com/Antonio7098/ultraplan-go/internal/platform/config"
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
	fmt.Fprintf(deps.stdout, "planning.requirements_model: %s\n", redacted.Planning.RequirementsModel)
	fmt.Fprintf(deps.stdout, "planning.requirements_variant: %s\n", redacted.Planning.RequirementsVariant)
	fmt.Fprintf(deps.stdout, "planning.sprint_index_model: %s\n", redacted.Planning.SprintIndexModel)
	fmt.Fprintf(deps.stdout, "planning.sprint_index_variant: %s\n", redacted.Planning.SprintIndexVariant)
	fmt.Fprintf(deps.stdout, "planning.technical_handbook_model: %s\n", redacted.Planning.TechnicalHandbookModel)
	fmt.Fprintf(deps.stdout, "planning.technical_handbook_variant: %s\n", redacted.Planning.TechnicalHandbookVariant)
	fmt.Fprintf(deps.stdout, "planning.area_reasoning_model: %s\n", redacted.Planning.AreaReasoningModel)
	fmt.Fprintf(deps.stdout, "planning.area_reasoning_variant: %s\n", redacted.Planning.AreaReasoningVariant)
	fmt.Fprintf(deps.stdout, "planning.reasoning_model: %s\n", redacted.Planning.ReasoningModel)
	fmt.Fprintf(deps.stdout, "planning.reasoning_variant: %s\n", redacted.Planning.ReasoningVariant)
	fmt.Fprintf(deps.stdout, "planning.plan_model: %s\n", redacted.Planning.PlanModel)
	fmt.Fprintf(deps.stdout, "planning.plan_variant: %s\n", redacted.Planning.PlanVariant)
	fmt.Fprintf(deps.stdout, "planning.execute_model: %s\n", redacted.Planning.ExecuteModel)
	fmt.Fprintf(deps.stdout, "planning.execute_variant: %s\n", redacted.Planning.ExecuteVariant)
	fmt.Fprintf(deps.stdout, "planning.review_model: %s\n", redacted.Planning.ReviewModel)
	fmt.Fprintf(deps.stdout, "planning.review_variant: %s\n", redacted.Planning.ReviewVariant)
	fmt.Fprintf(deps.stdout, "smoke.discovery_timeout: %s\n", redacted.Smoke.DiscoveryTimeout)
	fmt.Fprintf(deps.stdout, "smoke.run_timeout: %s\n", redacted.Smoke.RunTimeout)
	fmt.Fprintf(deps.stdout, "smoke.stdout_limit: %d\n", redacted.Smoke.StdoutLimit)
	fmt.Fprintf(deps.stdout, "smoke.stderr_limit: %d\n", redacted.Smoke.StderrLimit)
	fmt.Fprintf(deps.stdout, "smoke.cleanup_grace: %s\n", redacted.Smoke.CleanupGrace)
	fmt.Fprintf(deps.stdout, "smoke.environment: %s\n", strings.Join(redacted.Smoke.Environment, ", "))
	fmt.Fprintf(deps.stdout, "logging.format: %s\n", redacted.Logging.Format)
	fmt.Fprintf(deps.stdout, "logging.level: %s\n", redacted.Logging.Level)
	fmt.Fprintf(deps.stdout, "agentwrap.executable: %s\n", redacted.Agentwrap.Executable)
	fmt.Fprintf(deps.stdout, "agentwrap.required_health: %s\n", strings.Join(redacted.Agentwrap.RequiredHealth, ", "))
	fmt.Fprintf(deps.stdout, "agentwrap.required_capabilities: %s\n", strings.Join(redacted.Agentwrap.RequiredCapabilities, ", "))
	fmt.Fprintf(deps.stdout, "agentwrap.stderr_limit: %d\n", redacted.Agentwrap.StderrLimit)
	fmt.Fprintf(deps.stdout, "agentwrap.sandbox: %s\n", redacted.Agentwrap.Sandbox)
	fmt.Fprintf(deps.stdout, "agentwrap.permission_mode: %s\n", redacted.Agentwrap.PermissionMode)
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
