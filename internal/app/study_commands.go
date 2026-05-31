package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ultraplan-go/internal/study"
	"ultraplan-go/internal/workspace"
)

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
	case len(args) >= 3 && args[1] == "prompt":
		return runStudyPrompt(deps, root.Path, service, args[0], args[2:])
	case len(args) == 2 && args[1] == "status":
		return runStudyStatus(deps, service, args[0])
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
			if source.Kind == study.SourceKindMarkdown {
				applicability := "all"
				if len(source.ApplicableDimensions) > 0 {
					applicability = strings.Join(source.ApplicableDimensions, ",")
				}
				fmt.Fprintf(deps.stdout, "  %s %s %s\n", source.Name, source.Kind, applicability)
				continue
			}
			fmt.Fprintf(deps.stdout, "  %s %s\n", source.Name, source.Kind)
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
		return classified(ExitUsage, "study: expected 'init', 'list', '<study> list', '<study> prompt', or '<study> status'")
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
  ultraplan study <study> status
  ultraplan study <study> prompt analysis <dimension> <source> [--output <file>]
  ultraplan study <study> prompt synthesis <dimension> [--output <file>]

Commands:
  init              Initialize a study from YAML.
  list              List discovered studies.
  <study> list      List sources and dimensions for one study.
  <study> status    Show persisted run-state status without runtime execution.
  <study> prompt    Render prompt previews without runtime execution.
`
}

func runStudyStatus(deps dependencies, service study.Service, studyRef string) error {
	listing, err := service.ListStudy(studyRef)
	if err != nil {
		return mapStudyError(err)
	}
	state, err := study.LoadRunState(listing.Study)
	if err != nil {
		return mapStudyStatusError(err)
	}
	study.ResumeValidateRunState(&state, listing.Study, listing.Sources, listing.Dimensions, timeNow())
	summary := study.SummarizeRunState(state, study.RunStatePath(listing.Study))
	renderStudyStatus(deps.stdout, summary)
	return nil
}

var timeNow = func() time.Time { return time.Now().UTC() }

func mapStudyStatusError(err error) error {
	switch {
	case errors.Is(err, study.ErrRunStateMissing):
		return classified(ExitValidation, "study.status: %w", err)
	case errors.Is(err, study.ErrRunStateMalformed), errors.Is(err, study.ErrRunStateUnsupported):
		return classified(ExitValidation, "study.status: %w", err)
	default:
		return classified(ExitWorkspace, "study.status: %w", err)
	}
}

func renderStudyStatus(w io.Writer, summary study.StatusSummary) {
	fmt.Fprintf(w, "Run state: %s\n", summary.StatePath)
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
