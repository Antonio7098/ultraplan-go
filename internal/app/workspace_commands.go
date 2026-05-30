package app

import (
	"fmt"
	"strings"

	"ultraplan-go/internal/workspace"
)

func runInitWorkspace(deps dependencies, args []string) error {
	path := deps.workDir
	dryRun := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			_, err := deps.stdout.Write([]byte(initWorkspaceHelp()))
			return err
		case "--dry-run":
			dryRun = true
		case "--path":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
				return classified(ExitUsage, "init-workspace --path requires a directory")
			}
			path = args[i+1]
			i++
		default:
			return classified(ExitUsage, "init-workspace: unknown argument %q", args[i])
		}
	}
	if path == "" {
		path = "."
	}
	var plan workspace.InitPlan
	var err error
	if dryRun {
		plan, err = workspace.PlanInit(path)
	} else {
		plan, err = workspace.Init(path)
	}
	if err != nil {
		return classified(ExitWorkspace, "%s", err.Error())
	}
	action := "created"
	if dryRun {
		action = "would create"
	}
	fmt.Fprintf(deps.stdout, "Workspace: %s\n", plan.Root)
	if len(plan.Operations) == 0 {
		fmt.Fprintln(deps.stdout, "No changes needed.")
		return nil
	}
	for _, op := range plan.Operations {
		fmt.Fprintf(deps.stdout, "%s %s %s\n", action, op.Type, op.Path)
	}
	return nil
}

func initWorkspaceHelp() string {
	return `ultraplan init-workspace

Usage:
  ultraplan init-workspace [--path <dir>] [--dry-run]

Flags:
  --path <dir>   Workspace directory to initialize.
  --dry-run      Print planned operations without writing files.
  -h, --help     Show help.
`
}
