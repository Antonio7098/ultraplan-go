package app

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"ultraplan-go/internal/codeextract"
)

type codeFlags struct {
	json    bool
	output  string
	reports []string
}

func runCode(deps dependencies, args []string) error {
	if len(args) == 0 {
		return classified(ExitUsage, "code requires at least one report\n\nRun 'ultraplan code --help' for usage.")
	}
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		_, err := deps.stdout.Write([]byte(codeHelp()))
		return err
	}
	flags, err := parseCodeArgs(args)
	if err != nil {
		return classified(ExitUsage, "code: %w", err)
	}
	root, err := discoverWorkspace(deps)
	if err != nil {
		return err
	}
	reports := make([]string, 0, len(flags.reports))
	for _, report := range flags.reports {
		path := report
		if !filepath.IsAbs(path) {
			path = filepath.Join(deps.workDir, path)
		}
		reports = append(reports, path)
	}
	result, err := codeextract.Extract(codeextract.Request{WorkspaceRoot: root.Path, Reports: reports})
	if err != nil {
		return classified(ExitWorkspace, "code.extract: %w", err)
	}
	var buf bytes.Buffer
	if flags.json {
		err = codeextract.RenderJSON(&buf, result)
	} else {
		err = codeextract.RenderText(&buf, root.Path, result)
	}
	if err != nil {
		return classified(ExitError, "code.render: %w", err)
	}
	if flags.output != "" {
		output := flags.output
		if !filepath.IsAbs(output) {
			output = filepath.Join(deps.workDir, output)
		}
		if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
			return classified(ExitWorkspace, "code.output: %w", err)
		}
		if err := os.WriteFile(output, buf.Bytes(), 0o644); err != nil {
			return classified(ExitWorkspace, "code.output: %w", err)
		}
	} else if _, err := deps.stdout.Write(buf.Bytes()); err != nil {
		return err
	}
	switch result.Status {
	case codeextract.StatusValidation:
		return classedError{class: ExitValidation, code: errorCode(ExitValidation), err: fmt.Errorf("code.extract: validation failed")}
	case codeextract.StatusPartial:
		return classedError{class: ExitPartial, code: errorCode(ExitPartial), err: fmt.Errorf("code.extract: unresolved references")}
	default:
		return nil
	}
}

func parseCodeArgs(args []string) (codeFlags, error) {
	var flags codeFlags
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--json":
			flags.json = true
		case arg == "--output":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
				return flags, fmt.Errorf("--output requires a path")
			}
			i++
			flags.output = args[i]
		case strings.HasPrefix(arg, "--output="):
			flags.output = strings.TrimPrefix(arg, "--output=")
			if flags.output == "" {
				return flags, fmt.Errorf("--output requires a path")
			}
		case strings.HasPrefix(arg, "-"):
			return flags, fmt.Errorf("unknown argument %q", arg)
		default:
			flags.reports = append(flags.reports, arg)
		}
	}
	if len(flags.reports) == 0 {
		return flags, fmt.Errorf("requires at least one report")
	}
	return flags, nil
}

func codeHelp() string {
	return `ultraplan code

Usage:
  ultraplan code <report>... [--json] [--output <path>]

Flags:
  --json            Render deterministic JSON output.
  --output <path>   Write extraction output to a file instead of stdout.
`
}
