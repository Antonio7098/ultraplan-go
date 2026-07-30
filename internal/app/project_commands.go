package app

import (
	"errors"
	"fmt"

	"github.com/Antonio7098/ultraplan-go/internal/project"
)

func runProject(deps dependencies, args []string) error {
	if len(args) == 0 {
		return classified(ExitUsage, "project requires a subcommand\n\nRun 'ultraplan project --help' for usage.")
	}
	if args[0] == "--help" || args[0] == "-h" {
		_, err := deps.stdout.Write([]byte(projectHelp()))
		return err
	}
	if len(args) >= 2 && args[0] == "list" && (args[1] == "--help" || args[1] == "-h") {
		_, err := deps.stdout.Write([]byte(projectListHelp()))
		return err
	}
	if len(args) >= 3 && (args[2] == "--help" || args[2] == "-h") {
		switch args[1] {
		case "status":
			_, err := deps.stdout.Write([]byte(projectStatusHelp()))
			return err
		case "validate":
			_, err := deps.stdout.Write([]byte(projectValidateHelp()))
			return err
		}
	}
	root, err := discoverWorkspace(deps)
	if err != nil {
		return err
	}
	service := project.NewService(root.Path)
	switch {
	case len(args) == 1 && args[0] == "list":
		projects, err := service.ListProjects()
		if err != nil {
			return mapProjectError("project.list", err)
		}
		fmt.Fprintf(deps.stdout, "Workspace: %s\n", root.Path)
		fmt.Fprintln(deps.stdout, "Projects:")
		if len(projects) == 0 {
			fmt.Fprintln(deps.stdout, "  (none)")
			return nil
		}
		for _, item := range projects {
			fmt.Fprintf(deps.stdout, "  %s\n", item.Name)
		}
		return nil
	case len(args) == 2 && args[1] == "status":
		status, err := service.Status(args[0])
		if err != nil {
			return mapProjectError("project.status", err)
		}
		renderProjectStatus(deps, status)
		return nil
	case len(args) == 2 && args[1] == "validate":
		result, err := service.Validate(args[0])
		if err != nil {
			return mapProjectError("project.validate", err)
		}
		renderProjectValidation(deps, result)
		if len(result.Findings) > 0 {
			return classedError{class: ExitValidation, code: errorCode(ExitValidation), err: fmt.Errorf("project.validate: validation failed")}
		}
		return nil
	case args[0] == "list":
		return classified(ExitUsage, "project list: unknown argument %q", args[1])
	default:
		return classified(ExitUsage, "project: expected 'list', '<project> status', or '<project> validate'")
	}
}

func mapProjectError(prefix string, err error) error {
	var refErr project.RefError
	if errors.As(err, &refErr) {
		return classified(ExitValidation, "%s: %w", prefix, err)
	}
	return classified(ExitWorkspace, "%s: %w", prefix, err)
}

func renderProjectStatus(deps dependencies, status project.ProjectStatus) {
	fmt.Fprintf(deps.stdout, "Project: %s\n", status.Project.Name)
	fmt.Fprintf(deps.stdout, "Docs: %s\n", status.DocsDir)
	fmt.Fprintf(deps.stdout, "Markdown docs: %d\n", len(status.MarkdownDocs))
	for _, doc := range status.MarkdownDocs {
		fmt.Fprintf(deps.stdout, "  %s\n", doc)
	}
	fmt.Fprintf(deps.stdout, "Roadmap: %s\n", status.Roadmap)
	fmt.Fprintf(deps.stdout, "Project index: %s\n", status.ProjectIndex)
	fmt.Fprintf(deps.stdout, "Sprints: %s\n", status.SprintsDir)
	fmt.Fprintf(deps.stdout, "Sprint directories: %d\n", len(status.SprintDirs))
	for _, sprint := range status.SprintDirs {
		fmt.Fprintf(deps.stdout, "  %s\n", sprint)
	}
	fmt.Fprintln(deps.stdout, "Reasoning defaults:")
	for _, item := range status.ReasoningDefaults {
		fmt.Fprintf(deps.stdout, "  %s: %s\n", item.RelativePath, item.Source)
	}
	fmt.Fprintf(deps.stdout, "Project area reasoning documents: %d\n", len(status.AreaReasoningDocuments))
	for _, path := range status.AreaReasoningDocuments {
		fmt.Fprintf(deps.stdout, "  %s\n", path)
	}
	fmt.Fprintf(deps.stdout, "Catalog: %s\n", status.Catalog)
	if len(status.ValidationFinds) > 0 {
		fmt.Fprintf(deps.stdout, "Findings: %d\n", len(status.ValidationFinds))
	}
}

func renderProjectValidation(deps dependencies, result project.ValidationResult) {
	fmt.Fprintf(deps.stdout, "Project: %s\n", result.Project.Name)
	fmt.Fprintf(deps.stdout, "Validation: %s\n", result.Status)
	fmt.Fprintf(deps.stdout, "Findings: %d\n", len(result.Findings))
	for _, finding := range result.Findings {
		fmt.Fprintf(deps.stderr, "%s", finding.Severity)
		if finding.Section != "" {
			fmt.Fprintf(deps.stderr, " section=%q", finding.Section)
		}
		if finding.EntryName != "" {
			fmt.Fprintf(deps.stderr, " entry=%q", finding.EntryName)
		}
		if finding.Path != "" {
			fmt.Fprintf(deps.stderr, " path=%q", finding.Path)
		}
		fmt.Fprintf(deps.stderr, " problem=%q cause=%q suggestion=%q\n", finding.Problem, finding.Cause, finding.Suggestion)
	}
}

func projectHelp() string {
	return `ultraplan project

Usage:
  ultraplan project list
  ultraplan project <project> status
  ultraplan project <project> validate

Commands:
  list                List discovered project roots.
  <project> status    Show project docs, roadmap, index, sprints, and catalog health.
  <project> validate  Validate project files and project-index.md catalog references.
`
}

func projectListHelp() string {
	return `ultraplan project list

Usage:
  ultraplan project list

Lists direct non-hidden project roots under projects/.
`
}

func projectStatusHelp() string {
	return `ultraplan project <project> status

Usage:
  ultraplan project <project> status

Shows read-only project planning root health without runtime execution.
`
}

func projectValidateHelp() string {
	return `ultraplan project <project> validate

Usage:
  ultraplan project <project> validate

Validates required project files and project-index.md catalog paths.
`
}
