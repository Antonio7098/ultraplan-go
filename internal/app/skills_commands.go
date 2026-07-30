package app

import (
	"fmt"
	"strings"

	"github.com/Antonio7098/ultraplan-go/internal/workspace"
)

func runSkills(deps dependencies, args []string) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		_, err := deps.stdout.Write([]byte(skillsHelp()))
		return err
	}
	switch args[0] {
	case "list":
		for _, name := range workspace.SkillNames() {
			fmt.Fprintln(deps.stdout, name)
		}
		return nil
	case "materialize":
		return runSkillsMaterialize(deps, args[1:])
	default:
		return classified(ExitUsage, "skills: unknown subcommand %q", args[0])
	}
}

func runSkillsMaterialize(deps dependencies, args []string) error {
	path := deps.workDir
	if deps.workspaceFlag != "" {
		path = deps.workspaceFlag
	}
	name := "all"
	dryRun := false
	force := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			_, err := deps.stdout.Write([]byte(skillsMaterializeHelp()))
			return err
		case "--dry-run":
			dryRun = true
		case "--force":
			force = true
		case "--path":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
				return classified(ExitUsage, "skills materialize --path requires a directory")
			}
			path = args[i+1]
			i++
		default:
			if strings.HasPrefix(args[i], "-") {
				return classified(ExitUsage, "skills materialize: unknown argument %q", args[i])
			}
			if name != "all" {
				return classified(ExitUsage, "skills materialize accepts one skill name or all")
			}
			name = args[i]
		}
	}
	if path == "" {
		path = "."
	}
	opts := workspace.SkillMaterializeOptions{Force: force}
	var plan workspace.SkillMaterializePlan
	var err error
	if dryRun {
		plan, err = workspace.PlanSkillMaterialize(path, name, opts)
	} else {
		plan, err = workspace.MaterializeSkills(path, name, opts)
	}
	if err != nil {
		return classified(ExitWorkspace, "skills.materialize: %w", err)
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

func skillsHelp() string {
	return `ultraplan skills

Usage:
  ultraplan skills list
  ultraplan skills materialize [all|<skill>] [--path <dir>] [--dry-run] [--force]

Commands:
  list          List embedded stage skills.
  materialize   Write one or all stage skills into .opencode/skills.
`
}

func skillsMaterializeHelp() string {
	return `ultraplan skills materialize

Usage:
  ultraplan skills materialize [all|<skill>] [--path <dir>] [--dry-run] [--force]

The default target is all embedded skills.
`
}
