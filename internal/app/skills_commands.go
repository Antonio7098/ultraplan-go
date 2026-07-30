package app

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/Antonio7098/ultraplan-go/internal/workspace"
)

func runSkills(deps dependencies, args []string) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		_, err := deps.stdout.Write([]byte(skillsHelp()))
		return err
	}
	switch args[0] {
	case "materialise", "materialize":
		return runSkillsMaterialise(deps, args[1:])
	default:
		return classified(ExitUsage, "skills: unknown subcommand %q", args[0])
	}
}

func runSkillsMaterialise(deps dependencies, args []string) error {
	path := deps.workDir
	if deps.workspaceFlag != "" {
		path = deps.workspaceFlag
	}
	selection := "all"
	dryRun := false
	force := false
	selectionSet := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			_, err := deps.stdout.Write([]byte(skillsMaterialiseHelp()))
			return err
		case "--dry-run":
			dryRun = true
		case "--force":
			force = true
		case "--path":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
				return classified(ExitUsage, "skills materialise --path requires a directory")
			}
			path = args[i+1]
			i++
		default:
			if strings.HasPrefix(args[i], "-") {
				return classified(ExitUsage, "skills materialise: unknown argument %q", args[i])
			}
			if selectionSet {
				return classified(ExitUsage, "skills materialise: expected at most one stage selection")
			}
			selection = args[i]
			selectionSet = true
		}
	}
	if path == "" {
		path = "."
	}
	opts := workspace.SkillsOptions{Force: force}
	var plan workspace.SkillsPlan
	var err error
	if dryRun {
		plan, err = workspace.PlanSkills(path, selection, opts)
	} else {
		if !force {
			initial, planErr := workspace.PlanSkills(path, selection, opts)
			if planErr != nil {
				return classified(ExitWorkspace, "skills.materialise: %w", planErr)
			}
			conflicts := skippedSkillFiles(initial.Operations)
			if len(conflicts) > 0 && confirmOverwriteSkills(deps.stdout, deps.stdin, conflicts) {
				opts.Force = true
			}
		}
		plan, err = workspace.MaterialiseSkills(path, selection, opts)
	}
	if err != nil {
		return classified(ExitWorkspace, "skills.materialise: %w", err)
	}
	fmt.Fprintf(deps.stdout, "Workspace: %s\n", plan.Root)
	fmt.Fprintf(deps.stdout, "Selection: %s\n", selection)
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
  ultraplan skills materialise [all|stage] [--path <dir>] [--dry-run] [--force]

Commands:
  materialise   Write manually invoked UltraPlan stage skills into .agents/skills.

The American spelling "materialize" is accepted as an alias.
`
}

func skillsMaterialiseHelp() string {
	return `ultraplan skills materialise

Usage:
  ultraplan skills materialise [all|stage] [--path <dir>] [--dry-run] [--force]

Stages:
  requirements
  sprint-index
  technical-handbook
  area-reasoning
  reasoning
  plan
  execute
  review
  smoke

Flags:
  --path <dir>   Workspace directory to receive .agents/skills.
  --dry-run      Print planned operations without writing files.
  --force        Overwrite existing customized stage skill files without asking.
  -h, --help     Show help.
`
}

func skippedSkillFiles(ops []workspace.Operation) []string {
	var out []string
	for _, op := range ops {
		if op.Type == "file" && op.Action == "skip" {
			out = append(out, op.Path)
		}
	}
	return out
}

func confirmOverwriteSkills(stdout io.Writer, stdin io.Reader, paths []string) bool {
	fmt.Fprintln(stdout, "The following stage skill files already exist and differ from the built-in versions:")
	for _, path := range paths {
		fmt.Fprintf(stdout, "- %s\n", path)
	}
	fmt.Fprint(stdout, "Overwrite these files with the built-in stage skills? Type yes to overwrite: ")
	answer, err := bufio.NewReader(stdin).ReadString('\n')
	if err != nil && err != io.EOF {
		fmt.Fprintf(stdout, "\nCould not read confirmation: %v\n", err)
		return false
	}
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "yes", "y":
		fmt.Fprintln(stdout, "Overwriting customized stage skills.")
		return true
	default:
		fmt.Fprintln(stdout, "Keeping customized stage skills. Use --force to overwrite without prompting.")
		return false
	}
}
