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
	fmt.Fprintf(deps.stdout, "planning.code_context_model: %s (source: %s)\n", redacted.Planning.CodeContextModel, redacted.Sources["planning.code_context_model"])
	fmt.Fprintf(deps.stdout, "planning.code_context_variant: %s (source: %s)\n", redacted.Planning.CodeContextVariant, redacted.Sources["planning.code_context_variant"])
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
	fmt.Fprintf(deps.stdout, "qa.model: %s (source: %s)\n", redacted.QA.Model, redacted.Sources["qa.model"])
	fmt.Fprintf(deps.stdout, "qa.variant: %s (source: %s)\n", redacted.QA.Variant, redacted.Sources["qa.variant"])
	for _, item := range []struct{ field, value string }{
		{"qa.mapper_model", redacted.QA.MapperModel}, {"qa.mapper_variant", redacted.QA.MapperVariant},
		{"qa.investigator_model", redacted.QA.InvestigatorModel}, {"qa.investigator_variant", redacted.QA.InvestigatorVariant},
		{"qa.challenger_model", redacted.QA.ChallengerModel}, {"qa.challenger_variant", redacted.QA.ChallengerVariant},
		{"qa.arbiter_model", redacted.QA.ArbiterModel}, {"qa.arbiter_variant", redacted.QA.ArbiterVariant},
		{"qa.reconciler_model", redacted.QA.ReconcilerModel}, {"qa.reconciler_variant", redacted.QA.ReconcilerVariant},
		{"qa.evaluator_model", redacted.QA.EvaluatorModel}, {"qa.evaluator_variant", redacted.QA.EvaluatorVariant},
		{"qa.repair_model", redacted.QA.RepairModel}, {"qa.repair_variant", redacted.QA.RepairVariant},
	} {
		fmt.Fprintf(deps.stdout, "%s: %s (source: %s)\n", item.field, item.value, redacted.Sources[item.field])
	}
	fmt.Fprintf(deps.stdout, "qa.repair_assignment_mode: %s (source: %s)\n", redacted.QA.RepairAssignmentMode, redacted.Sources["qa.repair_assignment_mode"])
	fmt.Fprintf(deps.stdout, "qa.issues_per_repair_agent: %d (source: %s)\n", redacted.QA.IssuesPerRepairAgent, redacted.Sources["qa.issues_per_repair_agent"])
	fmt.Fprintf(deps.stdout, "qa.arbiter_max_theories: %d (source: %s)\n", redacted.QA.ArbiterMaxTheories, redacted.Sources["qa.arbiter_max_theories"])
	fmt.Fprintf(deps.stdout, "qa.repair_execution_mode: %s (source: %s)\n", redacted.QA.RepairExecutionMode, redacted.Sources["qa.repair_execution_mode"])
	fmt.Fprintf(deps.stdout, "qa.changed_paths: %d\n", redacted.QA.ChangedPaths)
	fmt.Fprintf(deps.stdout, "qa.primary_shards: %d\n", redacted.QA.PrimaryShards)
	fmt.Fprintf(deps.stdout, "qa.boundary_shards: %d\n", redacted.QA.BoundaryShards)
	fmt.Fprintf(deps.stdout, "qa.follow_up_shards: %d\n", redacted.QA.FollowUpShards)
	fmt.Fprintf(deps.stdout, "qa.total_shards: %d\n", redacted.QA.TotalShards)
	fmt.Fprintf(deps.stdout, "qa.pending_entries: %d\n", redacted.QA.PendingEntries)
	fmt.Fprintf(deps.stdout, "qa.changed_paths_per_shard: %d\n", redacted.QA.ChangedPathsPerShard)
	fmt.Fprintf(deps.stdout, "qa.context_paths_per_shard: %d\n", redacted.QA.ContextPathsPerShard)
	fmt.Fprintf(deps.stdout, "qa.context_expansions: %d\n", redacted.QA.ContextExpansions)
	fmt.Fprintf(deps.stdout, "qa.paths_per_expansion: %d\n", redacted.QA.PathsPerExpansion)
	fmt.Fprintf(deps.stdout, "qa.behavioral_concerns_per_shard: %d\n", redacted.QA.BehavioralConcernsPerShard)
	fmt.Fprintf(deps.stdout, "qa.theories_per_shard: %d\n", redacted.QA.TheoriesPerShard)
	fmt.Fprintf(deps.stdout, "qa.iterations_per_attempt: %d\n", redacted.QA.IterationsPerAttempt)
	fmt.Fprintf(deps.stdout, "qa.commands_per_attempt: %d\n", redacted.QA.CommandsPerAttempt)
	fmt.Fprintf(deps.stdout, "qa.output_repair_attempts: %d\n", redacted.QA.OutputRepairAttempts)
	fmt.Fprintf(deps.stdout, "qa.concurrent_investigators: %d\n", redacted.QA.ConcurrentInvestigators)
	fmt.Fprintf(deps.stdout, "qa.tree_files: %d\n", redacted.QA.TreeFiles)
	fmt.Fprintf(deps.stdout, "qa.tree_bytes: %d\n", redacted.QA.TreeBytes)
	fmt.Fprintf(deps.stdout, "qa.file_bytes: %d\n", redacted.QA.FileBytes)
	fmt.Fprintf(deps.stdout, "qa.generated_checks: %d\n", redacted.QA.GeneratedChecks)
	fmt.Fprintf(deps.stdout, "qa.generated_patch_bytes: %d\n", redacted.QA.GeneratedPatchBytes)
	fmt.Fprintf(deps.stdout, "qa.evidence_records: %d\n", redacted.QA.EvidenceRecords)
	fmt.Fprintf(deps.stdout, "qa.issues: %d\n", redacted.QA.Issues)
	fmt.Fprintf(deps.stdout, "qa.command_timeout: %s\n", redacted.QA.CommandTimeout)
	fmt.Fprintf(deps.stdout, "qa.shard_timeout: %s\n", redacted.QA.ShardTimeout)
	fmt.Fprintf(deps.stdout, "qa.run_timeout: %s\n", redacted.QA.RunTimeout)
	fmt.Fprintf(deps.stdout, "qa.cleanup_timeout: %s\n", redacted.QA.CleanupTimeout)
	fmt.Fprintf(deps.stdout, "qa.command_output_bytes: %d\n", redacted.QA.CommandOutputBytes)
	fmt.Fprintf(deps.stdout, "qa.shard_output_bytes: %d\n", redacted.QA.ShardOutputBytes)
	fmt.Fprintf(deps.stdout, "qa.prompt_bytes: %d\n", redacted.QA.PromptBytes)
	fmt.Fprintf(deps.stdout, "qa.recent_progress: %d\n", redacted.QA.RecentProgress)
	fmt.Fprintf(deps.stdout, "qa.retained_attempts: %d\n", redacted.QA.RetainedAttempts)
	fmt.Fprintf(deps.stdout, "qa.state_bytes: %d\n", redacted.QA.StateBytes)
	fmt.Fprintf(deps.stdout, "smoke.discovery_timeout: %s\n", redacted.Smoke.DiscoveryTimeout)
	fmt.Fprintf(deps.stdout, "smoke.run_timeout: %s\n", redacted.Smoke.RunTimeout)
	fmt.Fprintf(deps.stdout, "smoke.stdout_limit: %d\n", redacted.Smoke.StdoutLimit)
	fmt.Fprintf(deps.stdout, "smoke.stderr_limit: %d\n", redacted.Smoke.StderrLimit)
	fmt.Fprintf(deps.stdout, "smoke.cleanup_grace: %s\n", redacted.Smoke.CleanupGrace)
	fmt.Fprintf(deps.stdout, "smoke.environment: %s\n", strings.Join(redacted.Smoke.Environment, ", "))
	fmt.Fprintf(deps.stdout, "run_control.full_history: %s (source: %s)\n", redacted.RunControl.FullHistory, redacted.Sources["run_control.full_history"])
	fmt.Fprintf(deps.stdout, "run_control.tombstone_history: %s (source: %s)\n", redacted.RunControl.TombstoneHistory, redacted.Sources["run_control.tombstone_history"])
	fmt.Fprintf(deps.stdout, "run_control.workspace_quota_bytes: %d (source: %s)\n", redacted.RunControl.WorkspaceQuota, redacted.Sources["run_control.workspace_quota_bytes"])
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
