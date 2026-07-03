package app

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Antonio7098/ultraplan-go/internal/platform/config"
	runtimepkg "github.com/Antonio7098/ultraplan-go/internal/platform/runtime"
	"github.com/Antonio7098/ultraplan-go/internal/study"
	"github.com/Antonio7098/ultraplan-go/internal/workspace"
)

var studyRuntimeFactory = func(c config.Config) (study.Runtime, error) {
	return runtimepkg.NewOpenCode(c)
}

func runStudy(deps dependencies, args []string) error {
	if len(args) == 0 {
		return classified(ExitUsage, "study requires a subcommand\n\nRun 'ultraplan study --help' for usage.")
	}
	if args[0] == "--help" || args[0] == "-h" {
		_, err := deps.stdout.Write([]byte(studyHelp()))
		return err
	}
	if len(args) >= 2 && args[0] == "init" && (args[1] == "--help" || args[1] == "-h") {
		_, err := deps.stdout.Write([]byte(studyInitHelp()))
		return err
	}

	root, err := discoverWorkspace(deps)
	if err != nil {
		return err
	}
	service := study.NewService(root.Path)

	switch {
	case len(args) >= 1 && args[0] == "init":
		return runStudyInit(deps, root.Path, args[1:])
	case len(args) >= 2 && args[1] == "run-loop":
		return runStudyRunLoop(deps, root, args[0], args[2:])
	case len(args) >= 2 && args[1] == "run-all":
		return runStudyRunAll(deps, root, args[0], args[2:])
	case len(args) >= 3 && args[1] == "run":
		return runStudyRun(deps, root, args[0], args[2:])
	case len(args) >= 3 && args[1] == "synthesize":
		return runStudySynthesize(deps, root, args[0], args[2:])
	case len(args) >= 3 && args[1] == "prompt":
		return runStudyPrompt(deps, root.Path, service, args[0], args[2:])
	case len(args) >= 2 && args[1] == "summary":
		return runStudySummary(deps, root.Path, service, args[0], args[2:])
	case len(args) >= 2 && args[1] == "validate":
		return runStudyValidate(deps, root.Path, service, args[0], args[2:])
	case len(args) >= 2 && args[1] == "status":
		return runStudyStatus(deps, root.Path, service, args[0], args[2:])
	case len(args) >= 2 && args[1] == "runs":
		return runStudyRuns(deps, root.Path, service, args[0], args[2:])
	case len(args) == 1 && args[0] == "list":
		studies, err := service.ListStudies()
		if err != nil {
			return mapStudyError(err)
		}
		fmt.Fprintf(deps.stdout, "Workspace: %s\n", root.Path)
		fmt.Fprintln(deps.stdout, "Studies:")
		if len(studies) == 0 {
			fmt.Fprintln(deps.stdout, "  (none)")
			return nil
		}
		for _, item := range studies {
			fmt.Fprintf(deps.stdout, "  %s\n", item.Name)
		}
		return nil
	case len(args) == 2 && args[1] == "list":
		listing, err := service.ListStudy(args[0])
		if err != nil {
			return mapStudyError(err)
		}
		fmt.Fprintf(deps.stdout, "Study: %s\n", listing.Study.Name)
		fmt.Fprintln(deps.stdout, "Sources:")
		if len(listing.Sources) == 0 {
			fmt.Fprintln(deps.stdout, "  (none)")
		}
		for _, source := range listing.Sources {
			applicability := "all"
			if len(source.ApplicableDimensions) > 0 {
				applicability = strings.Join(source.ApplicableDimensions, ",")
			}
			fmt.Fprintf(deps.stdout, "  %s %s %s\n", source.Name, source.Kind, applicability)
		}
		fmt.Fprintln(deps.stdout, "Dimensions:")
		if len(listing.Dimensions) == 0 {
			fmt.Fprintln(deps.stdout, "  (none)")
		}
		for _, dimension := range listing.Dimensions {
			fmt.Fprintf(deps.stdout, "  %s %s %s\n", dimension.Number, dimension.Slug, dimension.File)
		}
		return nil
	case args[0] == "list":
		return classified(ExitUsage, "study list: unknown argument %q", args[1])
	default:
		return classified(ExitUsage, "study: expected 'init', 'list', '<study> list', '<study> summary', '<study> validate', '<study> status', '<study> runs', '<study> run-loop', '<study> run-all', '<study> run', '<study> synthesize', or '<study> prompt'")
	}
}

func mapStudyError(err error) error {
	var refErr study.RefError
	if errors.As(err, &refErr) {
		return classified(ExitValidation, "study.resolve: %w", err)
	}
	if errors.Is(err, study.ErrPromptInapplicable) {
		return classified(ExitValidation, "study.prompt: %w", err)
	}
	return classified(ExitWorkspace, "study.list: %w", err)
}

func studyHelp() string {
	return `ultraplan study

Usage:
  ultraplan study init <study-init.yml> [--dry-run] [--force] [--no-clone] [--output <dir>]
  ultraplan study list
  ultraplan study <study> list
  ultraplan study <study> summary
  ultraplan study <study> validate [--json]
  ultraplan study <study> status [--json]
  ultraplan study <study> runs summary
  ultraplan study <study> run-loop [--dimension <ref>] [--source <ref>] [--parallel <n>] [--force-unlock]
  ultraplan study <study> run-all [--dimension <ref>] [--source <ref>] [--parallel <n>]
  ultraplan study <study> run <dimension> <source>
  ultraplan study <study> synthesize <dimension>
  ultraplan study <study> prompt analysis <dimension> <source> [--output <file>]
  ultraplan study <study> prompt synthesis <dimension> [--output <file>]

Commands:
  init              Initialize a study from YAML.
  list              List discovered studies.
  <study> list      List sources and dimensions for one study.
  <study> summary   Regenerate deterministic studies/<study>/summary.csv without runtime execution.
  <study> validate  Validate study artifacts without runtime execution.
  <study> status    Show persisted run-state status without runtime execution.
  <study> runs      Inspect or refresh run history artifacts without runtime execution.
  <study> run-loop  Resume durable study execution with per-study locking and persisted task state.
  <study> run-all   Execute selected applicable study analysis tasks, synthesize, and write summary.csv.
  <study> run       Execute one analysis task through the configured runtime.
  <study> synthesize Execute one synthesis task through the configured runtime.
  <study> prompt    Render prompt previews without runtime execution.
`
}

type runAllFlags struct {
	dimensions  []string
	sources     []string
	parallelism *int
	forceUnlock bool
	continueRun bool
	reset       bool
	yes         bool
}

func runStudyRunLoop(deps dependencies, root workspace.Root, studyRef string, args []string) error {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		_, err := deps.stdout.Write([]byte(studyRunLoopHelp()))
		return err
	}
	flags, err := parseRunLoopArgs(args)
	if err != nil {
		return classified(ExitUsage, "study run-loop: %w", err)
	}
	if flags.reset {
		if err := confirmRunLoopReplacement(deps, root.Path, studyRef, flags); err != nil {
			return err
		}
	}
	service, parallelism, summary, err := runLoopService(deps, root, flags)
	if err != nil {
		return err
	}
	command := append([]string{"ultraplan", "study", studyRef, "run-loop"}, args...)
	result, err := service.RunLoop(deps.ctx, study.RunLoopRequest{
		StudyRef:      studyRef,
		DimensionRefs: flags.dimensions,
		SourceRefs:    flags.sources,
		Parallelism:   parallelism,
		Config:        summary,
		Command:       command,
		ForceUnlock:   flags.forceUnlock,
		Continue:      flags.continueRun,
		Reset:         flags.reset,
		Progress:      renderRunLoopProgress(deps, root.Path),
	})
	if err != nil {
		return mapStudyRunLoopError(err)
	}
	renderRunLoopResult(deps, root.Path, result)
	return classifyRunAllResult(study.RunAllResult{Status: result.Status})
}

func parseRunLoopArgs(args []string) (runAllFlags, error) {
	var filtered []string
	forceUnlock := false
	continueRun := false
	reset := false
	yes := false
	for _, arg := range args {
		if arg == "--force-unlock" {
			forceUnlock = true
			continue
		}
		if arg == "--continue" {
			continueRun = true
			continue
		}
		if arg == "--reset" {
			reset = true
			continue
		}
		if arg == "--yes" || arg == "-y" {
			yes = true
			continue
		}
		filtered = append(filtered, arg)
	}
	parsed, err := parseRunAllArgs(filtered)
	if err != nil {
		return parsed, err
	}
	parsed.forceUnlock = forceUnlock
	parsed.continueRun = continueRun
	parsed.reset = reset
	parsed.yes = yes
	return parsed, nil
}

func confirmRunLoopReplacement(deps dependencies, root string, studyRef string, flags runAllFlags) error {
	service := study.NewService(root)
	listing, err := service.ListStudy(studyRef)
	if err != nil {
		return mapStudyError(err)
	}
	path := study.RunStatePath(listing.Study)
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return classified(ExitWorkspace, "study.run-loop: inspect existing run-state: %w", err)
	}
	if flags.yes {
		return nil
	}
	state, stateErr := study.LoadRunState(listing.Study)
	fmt.Fprintln(deps.stdout, "Existing study progress is present and would be archived/replaced.")
	fmt.Fprintf(deps.stdout, "Study progress state: %s\n", workspace.Rel(root, path))
	if stateErr == nil {
		counts := study.SummarizeRunState(state, path)
		fmt.Fprintf(deps.stdout, "Tasks: %d completed, %d failed, %d cancelled, %d pending/running/waiting\n", counts.Completed, counts.Failed, counts.Cancelled, counts.Pending+counts.Running+counts.Validating+counts.Waiting+counts.Retrying)
	} else {
		fmt.Fprintf(deps.stdout, "Could not summarize current study progress: %v\n", stateErr)
	}
	fmt.Fprintln(deps.stdout, "Omit --reset to resume existing study progress instead.")
	fmt.Fprint(deps.stdout, "Archive and replace existing study progress? Type yes to replace: ")
	answer, err := bufio.NewReader(deps.stdin).ReadString('\n')
	if err != nil && err != io.EOF {
		return classified(ExitPartial, "study.run-loop: read replacement confirmation: %w", err)
	}
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "yes", "y":
		fmt.Fprintln(deps.stdout, "Replacing existing study progress.")
		return nil
	default:
		fmt.Fprintln(deps.stdout, "Keeping existing study progress. Re-run without --reset to resume it, or use --reset --yes to replace without prompting.")
		return classified(ExitPartial, "study.run-loop: replacement not confirmed")
	}
}

func runLoopService(deps dependencies, root workspace.Root, flags runAllFlags) (study.Service, int, study.ConfigSummary, error) {
	effective, err := loadEffectiveConfig(root, deps, config.CLIOverrides{})
	if err != nil {
		return study.Service{}, 0, study.ConfigSummary{}, err
	}
	parallelism := effective.Config.Execution.DefaultParallel
	if flags.parallelism != nil {
		parallelism = *flags.parallelism
	}
	if parallelism < 1 {
		return study.Service{}, 0, study.ConfigSummary{}, classified(ExitConfig, "study run-loop: parallelism must be at least 1")
	}
	req, err := runtimepkg.RequestFromConfig(effective.Config, root.Path)
	if err != nil {
		return study.Service{}, 0, study.ConfigSummary{}, classified(ExitConfig, "runtime.config: %w", err)
	}
	rt, err := studyRuntimeFactory(effective.Config)
	if err != nil {
		return study.Service{}, 0, study.ConfigSummary{}, classified(ExitRuntime, "runtime.init: %w", err)
	}
	summary := study.ConfigSummary{
		Runtime:          effective.Config.Runtime.Default,
		Model:            effective.Config.Models.Default,
		Variant:          effective.Config.Execution.DefaultVariant,
		DefaultParallel:  effective.Config.Execution.DefaultParallel,
		DefaultTimeout:   effective.Config.Execution.DefaultTimeout,
		DefaultRetries:   effective.Config.Execution.DefaultRetries,
		WorkspaceVersion: strconv.Itoa(effective.Config.Version),
	}
	return study.NewService(root.Path, study.WithRuntime(rt, req)), parallelism, summary, nil
}

func mapStudyRunLoopError(err error) error {
	switch {
	case errors.Is(err, study.ErrStudyLocked):
		return classified(ExitPartial, "study.run-loop: %w", err)
	case errors.Is(err, study.ErrRunStateMalformed), errors.Is(err, study.ErrRunStateUnsupported):
		return classified(ExitValidation, "study.run-loop: %w", err)
	default:
		return mapStudyExecutionError("study.run-loop", err)
	}
}

func renderRunLoopResult(deps dependencies, root string, result study.RunLoopResult) {
	fmt.Fprintf(deps.stdout, "Study progress: %s\n", result.Status)
	fmt.Fprintf(deps.stdout, "Study: %s\n", result.Study.Name)
	fmt.Fprintf(deps.stdout, "Parallelism: %d\n", result.Parallelism)
	fmt.Fprintf(deps.stdout, "Study progress state: %s\n", workspace.Rel(root, result.StatePath))
	fmt.Fprintf(deps.stdout, "Study progress summary: %s\n", workspace.Rel(root, study.RunHistorySummaryPath(result.Study)))
	fmt.Fprintf(deps.stdout, "Lock: %s\n", workspace.Rel(root, result.LockPath))
	fmt.Fprintf(deps.stdout, "Study completed: %d\n", result.Counts.Completed)
	fmt.Fprintf(deps.stdout, "Study failed: %d\n", result.Counts.Failed)
	fmt.Fprintf(deps.stdout, "Study skipped: %d\n", result.Counts.Skipped)
	fmt.Fprintf(deps.stdout, "Study pending: %d\n", result.Counts.Pending)
	if result.ScopeCounts != result.Counts {
		fmt.Fprintf(deps.stdout, "Scope completed: %d\n", result.ScopeCounts.Completed)
		fmt.Fprintf(deps.stdout, "Scope failed: %d\n", result.ScopeCounts.Failed)
		fmt.Fprintf(deps.stdout, "Scope skipped: %d\n", result.ScopeCounts.Skipped)
		fmt.Fprintf(deps.stdout, "Scope pending: %d\n", result.ScopeCounts.Pending)
	}
	for _, task := range result.State.Tasks {
		if task.Status == study.TaskStatusCompleted {
			continue
		}
		fmt.Fprintf(deps.stderr, "%s %s %s", task.Kind, task.Status, task.DimensionRef)
		if task.Source != "" {
			fmt.Fprintf(deps.stderr, " %s", task.Source)
		}
		if task.LastError != nil {
			fmt.Fprintf(deps.stderr, ": %s", config.RedactValue("task.error", task.LastError.Message))
		}
		fmt.Fprintln(deps.stderr)
	}
}

func renderRunLoopProgress(deps dependencies, root string) func(study.RunLoopProgress) {
	return func(progress study.RunLoopProgress) {
		task := progress.Task
		counts := progress.Counts
		scope := progress.ScopeCounts
		if progress.Event == study.RunLoopProgressRuntime && progress.RuntimeEvent != nil {
			fmt.Fprintf(deps.stdout, "[run-loop] runtime  %-9s %s", task.Kind, task.DimensionRef)
			if task.Source != "" {
				fmt.Fprintf(deps.stdout, " %s", task.Source)
			}
			fmt.Fprintf(deps.stdout, " | %s", runtimeProgressSummary(*progress.RuntimeEvent))
			fmt.Fprintf(deps.stdout, " | scope done %d/%d study done %d/%d active %d pending %d retrying %d failed %d\n", scope.Completed, scope.Total, counts.Completed, counts.Total, counts.Active, counts.Pending, counts.Retrying, counts.Failed+counts.Cancelled)
			return
		}
		fmt.Fprintf(deps.stdout, "[run-loop] %-9s %-9s %s", progress.Event, task.Kind, task.DimensionRef)
		if task.Source != "" {
			fmt.Fprintf(deps.stdout, " %s", task.Source)
		}
		if task.OutputPath != "" {
			fmt.Fprintf(deps.stdout, " -> %s", runLoopProgressPath(root, task.OutputPath))
		}
		fmt.Fprintf(deps.stdout, " | scope done %d/%d study done %d/%d active %d pending %d failed %d\n", scope.Completed, scope.Total, counts.Completed, counts.Total, counts.Active, counts.Pending, counts.Failed+counts.Cancelled)
		if task.LastError != nil {
			fmt.Fprintf(deps.stdout, "[run-loop] error     %s: %s\n", task.ID, config.RedactValue("task.error", task.LastError.Message))
		}
	}
}

func runtimeProgressSummary(event runtimepkg.Event) string {
	parts := []string{}
	if event.Kind != "" {
		parts = append(parts, event.Kind)
	} else if event.Type != "" {
		parts = append(parts, event.Type)
	}
	for _, key := range []string{"provider", "model", "retry_after", "delay", "reason", "detail", "error", "state"} {
		value, ok := event.Payload[key]
		if !ok {
			continue
		}
		text := strings.TrimSpace(fmt.Sprint(value))
		if text == "" || text == "<nil>" {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s=%s", key, config.RedactValue("runtime."+key, text)))
	}
	if len(parts) == 0 {
		return "event"
	}
	return strings.Join(parts, " ")
}

func runLoopProgressPath(root, path string) string {
	if filepath.IsAbs(path) {
		return workspace.Rel(root, path)
	}
	return filepath.ToSlash(filepath.Clean(path))
}

func runStudyRunAll(deps dependencies, root workspace.Root, studyRef string, args []string) error {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		_, err := deps.stdout.Write([]byte(studyRunAllHelp()))
		return err
	}
	flags, err := parseRunAllArgs(args)
	if err != nil {
		return classified(ExitUsage, "study run-all: %w", err)
	}
	service, parallelism, err := runAllService(deps, root, flags)
	if err != nil {
		return err
	}
	result, err := service.RunAll(deps.ctx, study.RunAllRequest{
		StudyRef:      studyRef,
		DimensionRefs: flags.dimensions,
		SourceRefs:    flags.sources,
		Parallelism:   parallelism,
	})
	if err != nil {
		return mapStudyExecutionError("study.run-all", err)
	}
	renderRunAllResult(deps, result)
	return classifyRunAllResult(result)
}

func parseRunAllArgs(args []string) (runAllFlags, error) {
	var flags runAllFlags
	for i := 0; i < len(args); i++ {
		arg := args[i]
		next := func(name string) (string, error) {
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
				return "", fmt.Errorf("%s requires a value", name)
			}
			i++
			return args[i], nil
		}
		switch {
		case arg == "--dimension":
			value, err := next(arg)
			if err != nil {
				return flags, err
			}
			flags.dimensions = append(flags.dimensions, value)
		case strings.HasPrefix(arg, "--dimension="):
			flags.dimensions = append(flags.dimensions, strings.TrimPrefix(arg, "--dimension="))
		case arg == "--source":
			value, err := next(arg)
			if err != nil {
				return flags, err
			}
			flags.sources = append(flags.sources, value)
		case strings.HasPrefix(arg, "--source="):
			flags.sources = append(flags.sources, strings.TrimPrefix(arg, "--source="))
		case arg == "--parallel":
			value, err := next(arg)
			if err != nil {
				return flags, err
			}
			n, err := parsePositiveIntFlag("--parallel", value)
			if err != nil {
				return flags, err
			}
			flags.parallelism = &n
		case strings.HasPrefix(arg, "--parallel="):
			n, err := parsePositiveIntFlag("--parallel", strings.TrimPrefix(arg, "--parallel="))
			if err != nil {
				return flags, err
			}
			flags.parallelism = &n
		default:
			return flags, fmt.Errorf("unknown argument %q", arg)
		}
	}
	return flags, nil
}

func studyRunLoopHelp() string {
	return `ultraplan study <study> run-loop

Usage:
  ultraplan study <study> run-loop [--dimension <ref>] [--source <ref>] [--parallel <n>] [--force-unlock] [--reset] [--yes]

Flags:
  --dimension <ref>   Advance only this dimension in the shared study progress. Repeatable.
  --source <ref>      Advance only this source in the shared study progress. Repeatable.
  --parallel <n>      Override configured default parallelism. Must be at least 1.
  --force-unlock      Remove this study's existing run-loop lock before starting.
  --continue          Deprecated compatibility no-op; continuing is the default.
  --reset             Archive existing study progress and rebuild it from the current study graph.
  --yes, -y           Replace existing study progress without prompting. Only applies with --reset.

By default, run-loop resumes shared study progress and filters only choose which slice is eligible to advance. The run-loop persists studies/<study>/.ultraplan/run-state.json after each meaningful task transition, appends execution history to studies/<study>/.ultraplan/runs/tasks.jsonl, refreshes studies/<study>/.ultraplan/runs/summary.md, cancels through the runtime boundary on interrupt, and refuses concurrent invocations unless --force-unlock is used.
`
}

func parsePositiveIntFlag(name, value string) (int, error) {
	n, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer", name)
	}
	if n < 1 {
		return 0, fmt.Errorf("%s must be at least 1", name)
	}
	return n, nil
}

func runAllService(deps dependencies, root workspace.Root, flags runAllFlags) (study.Service, int, error) {
	effective, err := loadEffectiveConfig(root, deps, config.CLIOverrides{})
	if err != nil {
		return study.Service{}, 0, err
	}
	parallelism := effective.Config.Execution.DefaultParallel
	if flags.parallelism != nil {
		parallelism = *flags.parallelism
	}
	if parallelism < 1 {
		return study.Service{}, 0, classified(ExitConfig, "study run-all: parallelism must be at least 1")
	}
	req, err := runtimepkg.RequestFromConfig(effective.Config, root.Path)
	if err != nil {
		return study.Service{}, 0, classified(ExitConfig, "runtime.config: %w", err)
	}
	rt, err := studyRuntimeFactory(effective.Config)
	if err != nil {
		return study.Service{}, 0, classified(ExitRuntime, "runtime.init: %w", err)
	}
	return study.NewService(root.Path, study.WithRuntime(rt, req)), parallelism, nil
}

func renderRunAllResult(deps dependencies, result study.RunAllResult) {
	fmt.Fprintf(deps.stdout, "Run-all: %s\n", result.Status)
	fmt.Fprintf(deps.stdout, "Study: %s\n", result.Study.Name)
	fmt.Fprintf(deps.stdout, "Parallelism: %d\n", result.Parallelism)
	fmt.Fprintf(deps.stdout, "Completed: %d\n", result.Counts.Completed)
	fmt.Fprintf(deps.stdout, "Failed: %d\n", result.Counts.Failed)
	fmt.Fprintf(deps.stdout, "Skipped: %d\n", result.Counts.Skipped)
	fmt.Fprintf(deps.stdout, "Pending: %d\n", result.Counts.Pending)
	fmt.Fprintf(deps.stdout, "Summary: %s\n", workspace.Rel(result.Study.Path, result.SummaryPath))
	for _, item := range append(append([]study.ExecutionResult{}, result.Analysis...), result.Synthesis...) {
		if item.Status == study.ExecutionStatusCompleted {
			continue
		}
		fmt.Fprintf(deps.stderr, "%s %s", item.TaskKind, item.Status)
		if item.Dimension.Ref() != "" {
			fmt.Fprintf(deps.stderr, " %s", item.Dimension.Ref())
		}
		if item.Source.Name != "" {
			fmt.Fprintf(deps.stderr, " %s", item.Source.Name)
		}
		if item.RuntimeCategory != "" {
			fmt.Fprintf(deps.stderr, ": %s", item.RuntimeCategory)
		}
		fmt.Fprintln(deps.stderr)
	}
	for _, warning := range result.Warnings {
		fmt.Fprintf(deps.stderr, "Warning: %s: %s\n", workspace.Rel(result.Study.Path, warning.Path), warning.Message)
	}
}

func classifyRunAllResult(result study.RunAllResult) error {
	switch result.Status {
	case study.RunAllStatusCompleted:
		return nil
	case study.RunAllStatusCancelled:
		return classedError{class: ExitCancel, code: errorCode(ExitCancel), err: fmt.Errorf("study.run-all: cancelled")}
	case study.RunAllStatusPartial:
		return classedError{class: ExitPartial, code: errorCode(ExitPartial), err: fmt.Errorf("study.run-all: partial completion")}
	case study.RunAllStatusRuntimeFailed:
		return classedError{class: ExitRuntime, code: errorCode(ExitRuntime), err: fmt.Errorf("study.run-all: runtime failed")}
	default:
		return classedError{class: ExitValidation, code: errorCode(ExitValidation), err: fmt.Errorf("study.run-all: validation failed")}
	}
}

func studyRunAllHelp() string {
	return `ultraplan study <study> run-all

Usage:
  ultraplan study <study> run-all [--dimension <ref>] [--source <ref>] [--parallel <n>]

Flags:
  --dimension <ref>   Limit execution to one dimension. Repeatable.
  --source <ref>      Limit execution to one source. Repeatable.
  --parallel <n>      Override configured default parallelism. Must be at least 1.
`
}

func runStudySummary(deps dependencies, root string, service study.Service, studyRef string, args []string) error {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		_, err := deps.stdout.Write([]byte(studySummaryHelp()))
		return err
	}
	if len(args) != 0 {
		return classified(ExitUsage, "study summary: unknown argument %q", args[0])
	}
	result, err := service.WriteSummary(studyRef)
	if err != nil {
		return mapStudySummaryError(err)
	}
	fmt.Fprintf(deps.stdout, "Summary: %s\n", workspace.Rel(root, result.Path))
	for _, warning := range result.Warnings {
		fmt.Fprintf(deps.stderr, "Warning: source=%s dimension=%s", warning.Source, warning.Dimension)
		if warning.Path != "" {
			fmt.Fprintf(deps.stderr, " path=%s", workspace.Rel(root, warning.Path))
		}
		fmt.Fprintf(deps.stderr, ": %s\n", warning.Message)
	}
	return nil
}

func mapStudySummaryError(err error) error {
	var refErr study.RefError
	if errors.As(err, &refErr) {
		return classified(ExitValidation, "study.summary: %w", err)
	}
	return classified(ExitWorkspace, "study.summary: %w", err)
}

func studySummaryHelp() string {
	return `ultraplan study <study> summary

Usage:
  ultraplan study <study> summary

Regenerates studies/<study>/summary.csv from existing reports without runtime execution.
`
}

func runStudyRun(deps dependencies, root workspace.Root, studyRef string, args []string) error {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		_, err := deps.stdout.Write([]byte(studyRunHelp()))
		return err
	}
	if len(args) != 2 {
		return classified(ExitUsage, "study run: requires <dimension> <source>")
	}
	service, err := executionService(deps, root)
	if err != nil {
		return err
	}
	result, err := service.RunAnalysis(deps.ctx, study.ExecutionRequest{StudyRef: studyRef, DimensionRef: args[0], SourceRef: args[1]})
	if err != nil {
		return mapStudyExecutionError("study.run", err)
	}
	renderExecutionResult(deps, result)
	return classifyExecutionResult("study.run", result)
}

func runStudySynthesize(deps dependencies, root workspace.Root, studyRef string, args []string) error {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		_, err := deps.stdout.Write([]byte(studySynthesizeHelp()))
		return err
	}
	if len(args) != 1 {
		return classified(ExitUsage, "study synthesize: requires <dimension>")
	}
	service, err := executionService(deps, root)
	if err != nil {
		return err
	}
	result, err := service.Synthesize(deps.ctx, study.SynthesisRequest{StudyRef: studyRef, DimensionRef: args[0]})
	if err != nil {
		return mapStudyExecutionError("study.synthesize", err)
	}
	renderExecutionResult(deps, result)
	return classifyExecutionResult("study.synthesize", result)
}

func executionService(deps dependencies, root workspace.Root) (study.Service, error) {
	effective, err := loadEffectiveConfig(root, deps, config.CLIOverrides{})
	if err != nil {
		return study.Service{}, err
	}
	req, err := runtimepkg.RequestFromConfig(effective.Config, root.Path)
	if err != nil {
		return study.Service{}, classified(ExitConfig, "runtime.config: %w", err)
	}
	rt, err := studyRuntimeFactory(effective.Config)
	if err != nil {
		return study.Service{}, classified(ExitRuntime, "runtime.init: %w", err)
	}
	return study.NewService(root.Path, study.WithRuntime(rt, req)), nil
}

func renderExecutionResult(deps dependencies, result study.ExecutionResult) {
	relOutput := workspace.Rel(result.Study.Path, result.OutputPath)
	writeWarnings := func() {
		for _, warning := range result.Warnings {
			fmt.Fprintf(deps.stderr, "Warning: %s\n", warning)
		}
	}
	switch result.Status {
	case study.ExecutionStatusCompleted:
		if result.TaskKind == study.TaskKindSynthesis {
			fmt.Fprintf(deps.stdout, "Completed synthesis: %s %s -> %s\n", result.Study.Name, result.Dimension.Ref(), relOutput)
			writeWarnings()
			return
		}
		fmt.Fprintf(deps.stdout, "Completed analysis: %s %s %s -> %s\n", result.Study.Name, result.Dimension.Ref(), result.Source.Name, relOutput)
	case study.ExecutionStatusSkipped:
		fmt.Fprintf(deps.stdout, "Skipped analysis: %s\n", result.SkippedReason)
	case study.ExecutionStatusRuntimeFailed, study.ExecutionStatusCancelled:
		fmt.Fprintf(deps.stderr, "Runtime failed for %s %s", result.TaskKind, result.Dimension.Ref())
		if result.Source.Name != "" {
			fmt.Fprintf(deps.stderr, " %s", result.Source.Name)
		}
		if result.RuntimeCategory != "" {
			fmt.Fprintf(deps.stderr, ": %s", result.RuntimeCategory)
		}
		if result.RuntimeError != "" {
			fmt.Fprintf(deps.stderr, ": %s", config.RedactValue("runtime.error", result.RuntimeError))
		}
		fmt.Fprintln(deps.stderr)
	case study.ExecutionStatusValidationFailed:
		fmt.Fprintf(deps.stderr, "Validation failed: %s\n", result.Validation.Path)
		for _, check := range result.Validation.Checks {
			if check.Status == study.ValidationStatusFailed {
				fmt.Fprintf(deps.stderr, "  %s: %s\n", check.Name, check.Observed)
			}
		}
	case study.ExecutionStatusPreflightBlocked:
		fmt.Fprintln(deps.stderr, "Synthesis blocked by invalid or missing source reports:")
		for _, validation := range result.PreflightResults {
			if validation.Status == study.ValidationStatusPassed {
				continue
			}
			fmt.Fprintf(deps.stderr, "  %s\n", validation.Path)
			for _, check := range validation.Checks {
				if check.Status == study.ValidationStatusFailed {
					fmt.Fprintf(deps.stderr, "    %s: %s\n", check.Name, check.Observed)
				}
			}
		}
	}
	writeWarnings()
}

func classifyExecutionResult(prefix string, result study.ExecutionResult) error {
	switch result.Status {
	case study.ExecutionStatusCompleted, study.ExecutionStatusSkipped:
		return nil
	case study.ExecutionStatusRuntimeFailed:
		return classedError{class: ExitRuntime, code: errorCode(ExitRuntime), err: fmt.Errorf("%s: runtime failed", prefix)}
	case study.ExecutionStatusCancelled:
		return classedError{class: ExitCancel, code: errorCode(ExitCancel), err: fmt.Errorf("%s: cancelled", prefix)}
	case study.ExecutionStatusValidationFailed:
		return classedError{class: ExitValidation, code: errorCode(ExitValidation), err: fmt.Errorf("%s: validation failed", prefix)}
	case study.ExecutionStatusPreflightBlocked:
		return classedError{class: ExitValidation, code: errorCode(ExitValidation), err: fmt.Errorf("%s: preflight blocked", prefix)}
	default:
		return classedError{class: ExitError, code: errorCode(ExitError), err: fmt.Errorf("%s: unknown result status %q", prefix, result.Status)}
	}
}

func mapStudyExecutionError(prefix string, err error) error {
	var refErr study.RefError
	if errors.As(err, &refErr) {
		return classified(ExitValidation, "%s: %w", prefix, err)
	}
	return classified(ExitWorkspace, "%s: %w", prefix, err)
}

func studyRunHelp() string {
	return `ultraplan study <study> run

Usage:
  ultraplan study <study> run <dimension> <source>
`
}

func studySynthesizeHelp() string {
	return `ultraplan study <study> synthesize

Usage:
  ultraplan study <study> synthesize <dimension>
`
}

func runStudyValidate(deps dependencies, root string, service study.Service, studyRef string, args []string) error {
	jsonOut := false
	for _, arg := range args {
		switch arg {
		case "--help", "-h":
			_, err := deps.stdout.Write([]byte(studyValidateHelp()))
			return err
		case "--json":
			jsonOut = true
		default:
			return classified(ExitUsage, "study validate: unknown argument %q", arg)
		}
	}
	result, err := service.ValidateStudy(studyRef)
	if err != nil {
		return mapStudyError(err)
	}
	status := "ok"
	if result.Status == study.ValidationStatusFailed {
		status = "fail"
	}
	if jsonOut {
		if err := writeJSON(deps.stdout, "study.validate", root, status, redactedStudyValidationResult(root, result)); err != nil {
			return err
		}
	} else {
		renderStudyValidate(deps.stdout, root, result)
	}
	if result.Status == study.ValidationStatusFailed {
		return classedError{class: ExitValidation, code: errorCode(ExitValidation), err: errors.New("study.validate: validation failed")}
	}
	return nil
}

func studyValidateHelp() string {
	return `ultraplan study <study> validate

Usage:
  ultraplan study <study> validate [--json]

Validates study artifacts without runtime execution.
`
}

func renderStudyValidate(w io.Writer, root string, result study.StudyValidationResult) {
	fmt.Fprintf(w, "Validation: %s\n", result.Status)
	fmt.Fprintf(w, "Study: %s\n", result.Study)
	fmt.Fprintf(w, "Checks: %d passed, %d failed, %d skipped, %d inapplicable\n", result.Summary.Passed, result.Summary.Failed, result.Summary.Skipped, result.Summary.Inapplicable)
	for _, check := range result.Checks {
		if check.Status != study.ValidationStatusFailed {
			continue
		}
		fmt.Fprintf(w, "  %s: %s", check.Name, config.RedactValue(check.Name, check.Observed))
		if check.Path != "" {
			fmt.Fprintf(w, " (%s)", workspace.Rel(root, check.Path))
		}
		if check.Guidance != "" {
			fmt.Fprintf(w, " - %s", config.RedactValue(check.Name+".guidance", check.Guidance))
		}
		fmt.Fprintln(w)
	}
	for _, report := range result.Reports {
		if report.Status != study.ValidationStatusFailed {
			continue
		}
		fmt.Fprintf(w, "  report: %s\n", workspace.Rel(root, report.Path))
		for _, check := range report.Checks {
			if check.Status == study.ValidationStatusFailed {
				fmt.Fprintf(w, "    %s: %s\n", check.Name, config.RedactValue(check.Name, check.Observed))
			}
		}
	}
}

func redactedStudyValidationResult(root string, result study.StudyValidationResult) study.StudyValidationResult {
	result.Checks = append([]study.ValidationCheck(nil), result.Checks...)
	for i := range result.Checks {
		result.Checks[i] = redactedValidationCheck(root, result.Checks[i])
	}
	result.Reports = append([]study.ValidationResult(nil), result.Reports...)
	for i := range result.Reports {
		result.Reports[i].Path = workspace.Rel(root, result.Reports[i].Path)
		result.Reports[i].Checks = append([]study.ValidationCheck(nil), result.Reports[i].Checks...)
		for j := range result.Reports[i].Checks {
			result.Reports[i].Checks[j] = redactedValidationCheck(root, result.Reports[i].Checks[j])
		}
	}
	return result
}

func redactedValidationCheck(root string, check study.ValidationCheck) study.ValidationCheck {
	if check.Path != "" {
		check.Path = workspace.Rel(root, check.Path)
	}
	check.Expected = config.RedactValue(check.Name+".expected", check.Expected)
	check.Observed = config.RedactValue(check.Name+".observed", check.Observed)
	check.Guidance = config.RedactValue(check.Name+".guidance", check.Guidance)
	return check
}

func runStudyStatus(deps dependencies, root string, service study.Service, studyRef string, args []string) error {
	jsonOut := false
	for _, arg := range args {
		switch arg {
		case "--help", "-h":
			_, err := deps.stdout.Write([]byte(studyStatusHelp()))
			return err
		case "--json":
			jsonOut = true
		default:
			return classified(ExitUsage, "study status: unknown argument %q", arg)
		}
	}
	listing, err := service.ListStudy(studyRef)
	if err != nil {
		return mapStudyError(err)
	}
	state, err := study.LoadRunState(listing.Study)
	if err != nil {
		return mapStudyStatusError(root, listing.Study, err)
	}
	study.ReconcileRunState(&state, root, listing.Study, listing.Sources, listing.Dimensions, time.Now().UTC())
	summary := study.SummarizeRunState(state, study.RunStatePath(listing.Study))
	lock, err := study.LockInfoForStatus(listing.Study)
	if err != nil {
		return classified(ExitWorkspace, "study.status lock: %w", err)
	}
	summary.Lock = lock
	if jsonOut {
		if err := writeJSON(deps.stdout, "study.status", root, "ok", statusJSONResult(root, state, summary)); err != nil {
			return err
		}
	} else {
		renderStudyStatus(deps.stdout, root, summary)
	}
	return nil
}

func studyStatusHelp() string {
	return `ultraplan study <study> status

Usage:
  ultraplan study <study> status [--json]

Shows persisted run-state status without runtime execution.
`
}

var timeNow = func() time.Time { return time.Now().UTC() }

func mapStudyStatusError(root string, st study.Study, err error) error {
	display := statusRunStateErrorDisplay(root, st, err)
	switch {
	case errors.Is(err, study.ErrRunStateMissing):
		return classedError{class: ExitValidation, code: errorCode(ExitValidation), err: displayError{display: "study.status: " + display, cause: err}}
	case errors.Is(err, study.ErrRunStateMalformed), errors.Is(err, study.ErrRunStateUnsupported):
		return classedError{class: ExitValidation, code: errorCode(ExitValidation), err: displayError{display: "study.status: " + display, cause: err}}
	default:
		return classedError{class: ExitWorkspace, code: errorCode(ExitWorkspace), err: displayError{display: "study.status: " + display, cause: err}}
	}
}

func statusRunStateErrorDisplay(root string, st study.Study, err error) string {
	abs := study.RunStatePath(st)
	rel := workspace.Rel(root, abs)
	return strings.ReplaceAll(err.Error(), abs, rel)
}

func renderStudyStatus(w io.Writer, root string, summary study.StatusSummary) {
	fmt.Fprintf(w, "Study progress state: %s\n", workspace.Rel(root, summary.StatePath))
	fmt.Fprintf(w, "Run ID: %s\n", summary.RunID)
	fmt.Fprintf(w, "Complete: %t\n", summary.Complete)
	fmt.Fprintf(w, "Tasks: %d\n", summary.Total)
	fmt.Fprintf(w, "Completed: %d\n", summary.Completed)
	fmt.Fprintf(w, "Failed: %d\n", summary.Failed)
	fmt.Fprintf(w, "Active: %d\n", summary.Active)
	fmt.Fprintf(w, "Retries: %d\n", summary.RetryCount)
	if summary.NextRetryAt != nil {
		fmt.Fprintf(w, "Next retry: %s\n", summary.NextRetryAt.UTC().Format(time.RFC3339))
	}
	if summary.Lock != nil {
		fmt.Fprintf(w, "Lock: %s\n", workspace.Rel(root, summary.Lock.Path))
		fmt.Fprintf(w, "Lock PID: %d\n", summary.Lock.PID)
		fmt.Fprintf(w, "Lock command: %s\n", config.RedactValue("lock.command", summary.Lock.Command))
		fmt.Fprintf(w, "Lock acquired: %s\n", summary.Lock.AcquiredAt.UTC().Format(time.RFC3339))
	}
	renderStatusTaskSection(w, "Active", summary.Tasks, func(task study.TaskState) bool {
		return task.Status == study.TaskStatusRunning || task.Status == study.TaskStatusValidating || task.Status == study.TaskStatusWaiting || task.Status == study.TaskStatusRetrying
	})
	renderStatusTaskSection(w, "Failed", summary.Tasks, func(task study.TaskState) bool {
		return task.Status == study.TaskStatusFailed
	})
	renderStatusTaskSection(w, "Cancelled", summary.Tasks, func(task study.TaskState) bool {
		return task.Status == study.TaskStatusCancelled
	})
	renderStatusTaskSection(w, "Recent", summary.Tasks, func(task study.TaskState) bool {
		return task.Status == study.TaskStatusCompleted || task.Status == study.TaskStatusSkipped
	})
}

func renderStatusTaskSection(w io.Writer, title string, tasks []study.TaskState, include func(study.TaskState) bool) {
	wrote := false
	count := 0
	for _, task := range tasks {
		if !include(task) {
			continue
		}
		if !wrote {
			fmt.Fprintf(w, "%s tasks:\n", title)
			wrote = true
		}
		count++
		if count > 5 && title == "Recent" {
			fmt.Fprintln(w, "  ...")
			return
		}
		fmt.Fprintf(w, "  %s %s", task.Status, task.Kind)
		if task.DimensionRef != "" {
			fmt.Fprintf(w, " %s", task.DimensionRef)
		}
		if task.Source != "" {
			fmt.Fprintf(w, " %s", task.Source)
		}
		if task.OutputPath != "" {
			fmt.Fprintf(w, " -> %s", task.OutputPath)
		}
		fmt.Fprintln(w)
		if task.RetryAfter != nil {
			fmt.Fprintf(w, "    retry: %s\n", task.RetryAfter.UTC().Format(time.RFC3339))
		}
		if task.LastError != nil {
			fmt.Fprintf(w, "    error: %s\n", config.RedactValue("task.error", task.LastError.Message))
		}
		if task.Agent.Provider != "" || task.Agent.Model != "" || task.Agent.Status != "" {
			fmt.Fprintf(w, "    runtime: provider=%s model=%s status=%s run=%s\n", task.Agent.Provider, task.Agent.Model, task.Agent.Status, task.Agent.RunID)
		}
		if task.Agent.Usage.TotalTokensKnown {
			fmt.Fprintf(w, "    usage: total_tokens=%d\n", task.Agent.Usage.TotalTokens)
		} else if task.Agent.Usage.InputTokensKnown || task.Agent.Usage.OutputTokensKnown {
			fmt.Fprintf(w, "    usage: input_known=%t output_known=%t\n", task.Agent.Usage.InputTokensKnown, task.Agent.Usage.OutputTokensKnown)
		} else if task.Agent.RunID != "" {
			fmt.Fprintln(w, "    usage: unknown")
		}
		if task.Agent.Cost != nil {
			fmt.Fprintf(w, "    cost: %.6f %s estimate=%t\n", task.Agent.Cost.Amount, task.Agent.Cost.Currency, task.Agent.Cost.Estimate)
		}
		if len(task.Agent.Policy.Decisions) > 0 {
			last := task.Agent.Policy.Decisions[len(task.Agent.Policy.Decisions)-1]
			fmt.Fprintf(w, "    policy: final_attempt=%d last_decision=%s reason=%s\n", task.Agent.Policy.FinalAttempt, last.Kind, config.RedactValue("policy.reason", last.Reason))
		}
		if task.Agent.Permissions.Mode != "" || task.Agent.Permissions.PolicyID != "" {
			fmt.Fprintf(w, "    permissions: mode=%s policy=%s unsupported=%d\n", task.Agent.Permissions.Mode, task.Agent.Permissions.PolicyID, task.Agent.Permissions.UnsupportedCount)
		}
		if task.Agent.Cleanup.Attempted {
			fmt.Fprintf(w, "    cleanup: completed=%t failed=%t\n", task.Agent.Cleanup.Completed, task.Agent.Cleanup.Failed)
		}
		if task.Agent.Repair.Attempted || task.Agent.Repair.Configured {
			fmt.Fprintf(w, "    repair: attempts=%d exhausted=%t permission_denied=%t\n", task.Agent.Repair.AttemptCount, task.Agent.Repair.Exhausted, task.Agent.Repair.PermissionDenied)
		}
		for _, omission := range task.Agent.Omissions {
			fmt.Fprintf(w, "    omitted: %s (%s)\n", omission.Field, omission.Reason)
		}
	}
}

type studyPromptFlags struct {
	output string
}

func runStudyPrompt(deps dependencies, root string, service study.Service, studyRef string, args []string) error {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		_, err := deps.stdout.Write([]byte(studyPromptHelp()))
		return err
	}
	if len(args) == 0 {
		return classified(ExitUsage, "study prompt: requires analysis or synthesis")
	}
	listing, err := service.ListStudy(studyRef)
	if err != nil {
		return mapStudyPromptError(err)
	}
	switch args[0] {
	case "analysis":
		dimRef, sourceRef, flags, err := parsePromptAnalysisArgs(args[1:])
		if err != nil {
			return classified(ExitUsage, "study prompt analysis: %w", err)
		}
		dimension, err := study.ResolveDimension(listing.Dimensions, dimRef)
		if err != nil {
			return mapStudyPromptError(err)
		}
		source, err := study.ResolveSource(listing.Sources, sourceRef)
		if err != nil {
			return mapStudyPromptError(err)
		}
		result, err := study.BuildAnalysisPrompt(study.PromptRequest{WorkspaceRoot: root, Study: listing.Study, Dimension: dimension, Source: source})
		if err != nil {
			return mapStudyPromptError(err)
		}
		return writePromptPreview(root, deps.stdout, result, flags.output)
	case "synthesis":
		dimRef, flags, err := parsePromptSynthesisArgs(args[1:])
		if err != nil {
			return classified(ExitUsage, "study prompt synthesis: %w", err)
		}
		dimension, err := study.ResolveDimension(listing.Dimensions, dimRef)
		if err != nil {
			return mapStudyPromptError(err)
		}
		result, err := study.BuildSynthesisPrompt(study.PromptRequest{WorkspaceRoot: root, Study: listing.Study, Dimension: dimension})
		if err != nil {
			return mapStudyPromptError(err)
		}
		return writePromptPreview(root, deps.stdout, result, flags.output)
	default:
		return classified(ExitUsage, "study prompt: expected analysis or synthesis")
	}
}

func mapStudyPromptError(err error) error {
	var refErr study.RefError
	if errors.As(err, &refErr) {
		return classified(ExitValidation, "study.resolve: %w", err)
	}
	if errors.Is(err, study.ErrPromptInapplicable) {
		return classified(ExitValidation, "study.prompt: %w", err)
	}
	return classified(ExitWorkspace, "study.prompt: %w", err)
}

func parsePromptAnalysisArgs(args []string) (string, string, studyPromptFlags, error) {
	var positional []string
	flags, err := parseStudyPromptFlags(args, &positional)
	if err != nil {
		return "", "", flags, err
	}
	if len(positional) != 2 {
		return "", "", flags, fmt.Errorf("requires <dimension> <source>")
	}
	return positional[0], positional[1], flags, nil
}

func parsePromptSynthesisArgs(args []string) (string, studyPromptFlags, error) {
	var positional []string
	flags, err := parseStudyPromptFlags(args, &positional)
	if err != nil {
		return "", flags, err
	}
	if len(positional) != 1 {
		return "", flags, fmt.Errorf("requires <dimension>")
	}
	return positional[0], flags, nil
}

func parseStudyPromptFlags(args []string, positional *[]string) (studyPromptFlags, error) {
	var flags studyPromptFlags
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--output":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
				return flags, fmt.Errorf("--output requires a path")
			}
			flags.output = args[i+1]
			i++
		case strings.HasPrefix(arg, "--output="):
			flags.output = strings.TrimPrefix(arg, "--output=")
			if flags.output == "" {
				return flags, fmt.Errorf("--output requires a path")
			}
		case strings.HasPrefix(arg, "-"):
			return flags, fmt.Errorf("unknown flag %s", arg)
		default:
			*positional = append(*positional, arg)
		}
	}
	return flags, nil
}

func writePromptPreview(root string, stdout io.Writer, result study.PromptResult, output string) error {
	rendered, err := renderPromptPreview(result)
	if err != nil {
		return classified(ExitError, "study.prompt: %w", err)
	}
	if output == "" {
		_, err := io.WriteString(stdout, rendered)
		return err
	}
	path, err := workspace.ResolveInside(root, output)
	if err != nil {
		return classified(ExitValidation, "study.prompt output: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return classified(ExitWorkspace, "study.prompt output: create parent %s: %w", workspace.Rel(root, filepath.Dir(path)), err)
	}
	if err := os.WriteFile(path, []byte(rendered), 0o644); err != nil {
		return classified(ExitWorkspace, "study.prompt output: write %s: %w", workspace.Rel(root, path), err)
	}
	fmt.Fprintf(stdout, "Wrote prompt preview: %s\n", workspace.Rel(root, path))
	return nil
}

func renderPromptPreview(result study.PromptResult) (string, error) {
	manifest, err := json.MarshalIndent(result.Manifest, "", "  ")
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("--- manifest ---\n%s\n--- prompt ---\n%s", manifest, result.Text), nil
}

func studyPromptHelp() string {
	return `ultraplan study <study> prompt

Usage:
  ultraplan study <study> prompt analysis <dimension> <source> [--output <file>]
  ultraplan study <study> prompt synthesis <dimension> [--output <file>]

Flags:
  --output <file>  Write the rendered prompt preview to a workspace-relative file.

This command renders prompt text and a deterministic input manifest only. It does not execute runtime analysis, synthesis, agentwrap, OpenCode, providers, network calls, or subprocesses.
`
}

type studyInitFlags struct {
	dryRun  bool
	force   bool
	noClone bool
	output  string
}

func runStudyInit(deps dependencies, root string, args []string) error {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		_, err := deps.stdout.Write([]byte(studyInitHelp()))
		return err
	}
	input, flags, err := parseStudyInitArgs(args)
	if err != nil {
		return classified(ExitUsage, "study init: %w", err)
	}
	result, err := study.Init(study.InitRequest{
		WorkspaceRoot: root,
		InputPath:     input,
		OutputDir:     flags.output,
		DryRun:        flags.dryRun,
		Force:         flags.force,
		NoClone:       flags.noClone,
	})
	printStudyInitResult(deps.stdout, root, result)
	if err == nil {
		return nil
	}
	var partial study.ClonePartialError
	switch {
	case errors.As(err, &partial):
		for _, failure := range partial.Failures {
			fmt.Fprintf(deps.stderr, "clone failed for %s [%s]: %v\n", failure.Action.Name, failure.Code, failure.Err)
		}
		return classified(ExitPartial, "study.init: %w", err)
	case errors.Is(err, study.ErrInitValidation):
		return classified(ExitValidation, "study.init: %w", err)
	case errors.Is(err, study.ErrInitOverwrite):
		return classified(ExitValidation, "study.init: %w", err)
	default:
		return mapStudyError(err)
	}
}

func parseStudyInitArgs(args []string) (string, studyInitFlags, error) {
	var input string
	var flags studyInitFlags
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--dry-run":
			flags.dryRun = true
		case arg == "--force":
			flags.force = true
		case arg == "--no-clone":
			flags.noClone = true
		case arg == "--output":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
				return "", flags, fmt.Errorf("--output requires a path")
			}
			flags.output = args[i+1]
			i++
		case strings.HasPrefix(arg, "--output="):
			flags.output = strings.TrimPrefix(arg, "--output=")
			if flags.output == "" {
				return "", flags, fmt.Errorf("--output requires a path")
			}
		case strings.HasPrefix(arg, "-"):
			return "", flags, fmt.Errorf("unknown flag %s", arg)
		default:
			if input != "" {
				return "", flags, fmt.Errorf("unexpected argument %q", arg)
			}
			input = arg
		}
	}
	if input == "" {
		return "", flags, fmt.Errorf("requires <study-init.yml>")
	}
	return input, flags, nil
}

func printStudyInitResult(w interface{ Write([]byte) (int, error) }, root string, result study.InitResult) {
	if result.StudyName == "" {
		return
	}
	action := "Initialized"
	if result.DryRun {
		action = "Would initialize"
	}
	fmt.Fprintf(w, "%s study: %s\n", action, result.StudyName)
	fmt.Fprintf(w, "Output: %s\n", workspace.Rel(root, result.StudyDir))
	fmt.Fprintln(w, "Directories:")
	for _, dir := range result.Directories {
		fmt.Fprintf(w, "  %s\n", workspace.Rel(root, dir))
	}
	fmt.Fprintln(w, "Files:")
	for _, file := range result.Files {
		fmt.Fprintf(w, "  %s\n", workspace.Rel(root, file))
	}
	fmt.Fprintln(w, "Clone actions:")
	if len(result.CloneActions) == 0 {
		fmt.Fprintln(w, "  (none)")
	} else {
		for _, clone := range result.CloneActions {
			fmt.Fprintf(w, "  %s -> %s\n", clone.Name, filepath.ToSlash(workspace.Rel(root, clone.Dest)))
		}
	}
	if len(result.SkippedClones) > 0 {
		fmt.Fprintln(w, "Skipped clone actions due to --no-clone:")
		for _, clone := range result.SkippedClones {
			fmt.Fprintf(w, "  %s\n", clone.Name)
		}
	}
}

func studyInitHelp() string {
	return `ultraplan study init

Usage:
  ultraplan study init <study-init.yml> [--dry-run] [--force] [--no-clone] [--output <dir>]

Flags:
  --dry-run       Print planned directories, files, and clone actions without writing.
  --force         Overwrite known generated files inside the selected study directory.
  --no-clone      Create artifacts but skip URL-backed git clone actions.
  --output <dir>  Write the study to a workspace-relative directory instead of studies/<study>.
`
}
