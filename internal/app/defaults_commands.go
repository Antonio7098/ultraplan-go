package app

import (
	"fmt"
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
  --force        Overwrite existing customized prompt/template files.
  -h, --help     Show help.
`
}
