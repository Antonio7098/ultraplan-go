package app

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/Antonio7098/ultraplan-go/internal/workspace"
)

func runDefaults(deps dependencies, args []string) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		_, err := deps.stdout.Write([]byte(defaultsHelp()))
		return err
	}
	switch args[0] {
	case "install":
		return runDefaultsInstall(deps, args[1:])
	default:
		return classified(ExitUsage, "defaults: unknown subcommand %q", args[0])
	}
}

func runDefaultsInstall(deps dependencies, args []string) error {
	path := deps.workDir
	if deps.workspaceFlag != "" {
		path = deps.workspaceFlag
	}
	dryRun := false
	force := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			_, err := deps.stdout.Write([]byte(defaultsInstallHelp()))
			return err
		case "--dry-run":
			dryRun = true
		case "--force":
			force = true
		case "--path":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
				return classified(ExitUsage, "defaults install --path requires a directory")
			}
			path = args[i+1]
			i++
		default:
			return classified(ExitUsage, "defaults install: unknown argument %q", args[i])
		}
	}
	if path == "" {
		path = "."
	}
	opts := workspace.DefaultsOptions{Force: force}
	var plan workspace.DefaultsPlan
	var err error
	if dryRun {
		plan, err = workspace.PlanDefaults(path, opts)
	} else {
		if !force {
			initial, planErr := workspace.PlanDefaults(path, opts)
			if planErr != nil {
				return classified(ExitWorkspace, "defaults.install: %w", planErr)
			}
			conflicts := skippedDefaultFiles(initial.Operations)
			if len(conflicts) > 0 && confirmOverwriteDefaults(deps.stdout, deps.stdin, conflicts) {
				opts.Force = true
			}
		}
		plan, err = workspace.InstallDefaults(path, opts)
	}
	if err != nil {
		return classified(ExitWorkspace, "defaults.install: %w", err)
	}
	fmt.Fprintf(deps.stdout, "Workspace: %s\n", plan.Root)
	if len(plan.Operations) == 0 {
		fmt.Fprintln(deps.stdout, "No changes needed.")
		return nil
	}
	for _, op := range plan.Operations {
		action := op.Action
		if dryRun {
			action = "would " + action
		}
		fmt.Fprintf(deps.stdout, "%s %s %s\n", action, op.Type, op.Path)
	}
	return nil
}

func defaultsHelp() string {
	return `ultraplan defaults

Usage:
  ultraplan defaults install [--path <dir>] [--dry-run] [--force]

Commands:
  install   Write built-in prompts and templates into a workspace for editing.
`
}

func defaultsInstallHelp() string {
	return `ultraplan defaults install

Usage:
  ultraplan defaults install [--path <dir>] [--dry-run] [--force]

Flags:
  --path <dir>   Workspace directory to receive defaults.
  --dry-run      Print planned operations without writing files.
  --force        Overwrite existing customized prompt/template files without asking.
  -h, --help     Show help.
`
}

func skippedDefaultFiles(ops []workspace.Operation) []string {
	var out []string
	for _, op := range ops {
		if op.Type == "file" && op.Action == "skip" {
			out = append(out, op.Path)
		}
	}
	return out
}

func confirmOverwriteDefaults(stdout io.Writer, stdin io.Reader, paths []string) bool {
	fmt.Fprintln(stdout, "The following prompt/template files already exist and differ from the built-in defaults:")
	for _, path := range paths {
		fmt.Fprintf(stdout, "- %s\n", path)
	}
	fmt.Fprint(stdout, "Overwrite these files with the built-in defaults? Type yes to overwrite: ")
	answer, err := bufio.NewReader(stdin).ReadString('\n')
	if err != nil && err != io.EOF {
		fmt.Fprintf(stdout, "\nCould not read confirmation: %v\n", err)
		return false
	}
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "yes", "y":
		fmt.Fprintln(stdout, "Overwriting customized defaults.")
		return true
	default:
		fmt.Fprintln(stdout, "Keeping customized files. Use --force to overwrite without prompting.")
		return false
	}
}
