package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"ultraplan-go/internal/platform/config"
	"ultraplan-go/internal/workspace"
)

const (
	ExitOK         = 0
	ExitError      = 1
	ExitUsage      = 2
	ExitConfig     = 3
	ExitWorkspace  = 4
	ExitValidation = 5
	ExitRuntime    = 6
	ExitCancel     = 7
	ExitPartial    = 8
)

type Config struct {
	Args    []string
	Stdout  io.Writer
	Stderr  io.Writer
	Version Version
	WorkDir string
	Env     map[string]string
}

type classedError struct {
	class int
	err   error
}

func (e classedError) Error() string { return e.err.Error() }
func (e classedError) Unwrap() error { return e.err }

func classified(class int, format string, args ...any) error {
	return classedError{class: class, err: fmt.Errorf(format, args...)}
}

func Run(cfg Config) int {
	stdout := cfg.Stdout
	if stdout == nil {
		stdout = io.Discard
	}

	stderr := cfg.Stderr
	if stderr == nil {
		stderr = io.Discard
	}

	version := cfg.Version
	if version.IsZero() {
		version = DefaultVersion()
	}

	deps := dependencies{
		stdout:  stdout,
		stderr:  stderr,
		workDir: cfg.WorkDir,
		env:     cfg.Env,
	}
	if deps.workDir == "" {
		if wd, err := os.Getwd(); err == nil {
			deps.workDir = wd
		}
	}

	args, global, err := parseGlobalFlags(cfg.Args)
	if err != nil {
		return fail(stderr, classified(ExitUsage, "%s", err.Error()))
	}
	deps.workspaceFlag = global.workspace

	if len(args) == 0 {
		return writeStatus(stdout, renderHelp())
	}

	switch args[0] {
	case "--help", "-h":
		return writeStatus(stdout, renderHelp())
	case "version":
		return writeStatus(stdout, renderVersion(version))
	case "init-workspace":
		return failOrOK(stderr, runInitWorkspace(deps, args[1:]))
	case "config":
		return failOrOK(stderr, runConfig(deps, args[1:]))
	case "health":
		return failOrOK(stderr, runHealth(deps, args[1:]))
	case "study":
		return failOrOK(stderr, runStudy(deps, args[1:]))
	default:
		return fail(stderr, classified(ExitUsage, "unknown command %q\n\nRun 'ultraplan --help' to see available commands.", args[0]))
	}
}

type dependencies struct {
	stdout        io.Writer
	stderr        io.Writer
	workDir       string
	workspaceFlag string
	env           map[string]string
}

type globalFlags struct {
	workspace string
}

func parseGlobalFlags(args []string) ([]string, globalFlags, error) {
	var out []string
	var flags globalFlags
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--workspace":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
				return nil, flags, errors.New("--workspace requires a path")
			}
			flags.workspace = args[i+1]
			i++
		case strings.HasPrefix(arg, "--workspace="):
			flags.workspace = strings.TrimPrefix(arg, "--workspace=")
			if flags.workspace == "" {
				return nil, flags, errors.New("--workspace requires a path")
			}
		default:
			out = append(out, arg)
		}
	}
	return out, flags, nil
}

func failOrOK(stderr io.Writer, err error) int {
	if err == nil {
		return ExitOK
	}
	return fail(stderr, err)
}

func fail(stderr io.Writer, err error) int {
	if err == nil {
		return ExitOK
	}
	if _, writeErr := fmt.Fprintln(stderr, err.Error()); writeErr != nil {
		return ExitError
	}
	var classifiedErr classedError
	if errors.As(err, &classifiedErr) {
		return classifiedErr.class
	}
	return ExitError
}

func writeStatus(w io.Writer, text string) int {
	if _, err := io.WriteString(w, text); err != nil {
		return ExitError
	}
	return ExitOK
}

func renderHelp() string {
	return `ultraplan

Usage:
  ultraplan [--workspace <path>] [command]

Commands:
  init-workspace   Initialize an UltraPlan workspace.
  config           Inspect effective configuration.
  health           Check workspace, config, filesystem, and environment basics.
  study            Inspect studies, sources, and dimensions.
  version          Print build metadata.

Flags:
  --workspace <path>   Use a workspace path.
  -h, --help          Show help.
`
}

func renderVersion(version Version) string {
	return fmt.Sprintf("Version: %s\nCommit: %s\nBuildDate: %s\nGoVersion: %s\n",
		version.Version,
		version.Commit,
		version.BuildDate,
		version.GoVersion,
	)
}

func writeJSON(w io.Writer, command, workspacePath, status string, result any) error {
	payload := struct {
		Command   string `json:"command"`
		Workspace string `json:"workspace,omitempty"`
		Status    string `json:"status"`
		Result    any    `json:"result"`
	}{
		Command:   command,
		Workspace: workspacePath,
		Status:    status,
		Result:    result,
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(payload)
}

func discoverWorkspace(deps dependencies) (workspace.Root, error) {
	env := envLookup(deps.env)
	root, err := workspace.Discover(workspace.DiscoverOptions{
		ExplicitPath: deps.workspaceFlag,
		EnvWorkspace: env("ULTRAPLAN_WORKSPACE"),
		StartDir:     deps.workDir,
	})
	if err != nil {
		return workspace.Root{}, classified(ExitWorkspace, "%s", err.Error())
	}
	return root, nil
}

func envLookup(env map[string]string) func(string) string {
	return func(key string) string {
		if env != nil {
			return env[key]
		}
		return os.Getenv(key)
	}
}

func loadEffectiveConfig(root workspace.Root, deps dependencies, cli config.CLIOverrides) (config.Effective, error) {
	effective, err := config.Load(config.LoadOptions{
		WorkspaceRoot: root.Path,
		Env:           envLookup(deps.env),
		CLI:           cli,
	})
	if err != nil {
		return config.Effective{}, classified(ExitConfig, "%s", err.Error())
	}
	return effective, nil
}
