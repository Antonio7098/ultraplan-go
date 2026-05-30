package app

import (
	"errors"
	"fmt"

	"ultraplan-go/internal/study"
)

func runStudy(deps dependencies, args []string) error {
	if len(args) == 0 {
		return classified(ExitUsage, "study requires a subcommand\n\nRun 'ultraplan study --help' for usage.")
	}
	if args[0] == "--help" || args[0] == "-h" {
		_, err := deps.stdout.Write([]byte(studyHelp()))
		return err
	}

	root, err := discoverWorkspace(deps)
	if err != nil {
		return err
	}
	service := study.NewService(root.Path)

	switch {
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
		return classified(ExitUsage, "study: expected 'list' or '<study> list'")
	}
}

func mapStudyError(err error) error {
	var refErr study.RefError
	if errors.As(err, &refErr) {
		return classified(ExitValidation, "%s", refErr.Error())
	}
	return classified(ExitWorkspace, "%s", err.Error())
}

func studyHelp() string {
	return `ultraplan study

Usage:
  ultraplan study list
  ultraplan study <study> list

Commands:
  list             List discovered studies.
  <study> list     List sources and dimensions for one study.
`
}
