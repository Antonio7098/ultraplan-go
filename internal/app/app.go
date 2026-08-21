package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Antonio7098/ultraplan-go/internal/platform/config"
	"github.com/Antonio7098/ultraplan-go/internal/workspace"
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
	Args                 []string
	Stdin                io.Reader
	Stdout               io.Writer
	Stderr               io.Writer
	Context              context.Context
	Version              Version
	WorkDir              string
	Env                  map[string]string
	TUIRunner            TUIRunner
	WebRunner            WebRunner
	SprintRuntimeFactory SprintRuntimeFactory
}

type classedError struct {
	class int
	code  string
	err   error
}

func (e classedError) Error() string { return e.err.Error() }
func (e classedError) Unwrap() error { return e.err }
func (e classedError) Code() string  { return e.code }

type displayError struct {
	display string
	cause   error
}

func (e displayError) Error() string { return e.display }
func (e displayError) Unwrap() error { return e.cause }

func classified(class int, format string, args ...any) error {
	return classedError{class: class, code: errorCode(class), err: fmt.Errorf(format, args...)}
}

func classifiedCause(class int, cause error, format string, args ...any) error {
	return classedError{class: class, code: errorCode(class), err: fmt.Errorf(format+": %w", append(args, cause)...)}
}

func errorCode(class int) string {
	switch class {
	case ExitUsage:
		return "validation.usage"
	case ExitConfig:
		return "config.invalid"
	case ExitWorkspace:
		return "validation.workspace"
	case ExitValidation:
		return "validation.reference"
	case ExitRuntime:
		return "provider.runtime"
	case ExitCancel:
		return "workflow.cancelled"
	case ExitPartial:
		return "workflow.partial"
	default:
		return "internal.error"
	}
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
	stdin := cfg.Stdin
	if stdin == nil {
		stdin = os.Stdin
	}

	version := cfg.Version
	if version.IsZero() {
		version = DefaultVersion()
	}

	runControl := newRunControlState()
	defer runControl.Close()
	deps := dependencies{
		stdout:               stdout,
		stderr:               stderr,
		stdin:                stdin,
		ctx:                  cfg.Context,
		workDir:              cfg.WorkDir,
		env:                  cfg.Env,
		tuiRunner:            cfg.TUIRunner,
		webRunner:            cfg.WebRunner,
		sprintRuntimeFactory: cfg.SprintRuntimeFactory,
		runControl:           runControl,
	}
	if deps.sprintRuntimeFactory == nil {
		deps.sprintRuntimeFactory = defaultSprintRuntimeFactory
	}
	if deps.ctx == nil {
		deps.ctx = context.Background()
	}
	if deps.workDir == "" {
		if wd, err := os.Getwd(); err == nil {
			deps.workDir = wd
		}
	}

	args, global, err := parseGlobalFlags(cfg.Args)
	if err != nil {
		return fail(stderr, classified(ExitUsage, "parse global flags: %w", err))
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
	case "defaults":
		return failOrOK(stderr, runDefaults(deps, args[1:]))
	case "skills":
		return failOrOK(stderr, runSkills(deps, args[1:]))
	case "config":
		return failOrOK(stderr, runConfig(deps, args[1:]))
	case "health":
		return failOrOK(stderr, runHealth(deps, args[1:]))
	case "run":
		return failOrOK(stderr, runRun(deps, args[1:]))
	case "project":
		return failOrOK(stderr, runProject(deps, args[1:]))
	case "sprint":
		return failOrOK(stderr, runSprint(deps, args[1:]))
	case "study":
		return failOrOK(stderr, runStudy(deps, args[1:]))
	case "tui":
		return failOrOK(stderr, runTUI(deps, args[1:]))
	case "serve":
		return failOrOK(stderr, runServe(deps, args[1:]))
	case "code":
		return failOrOK(stderr, runCode(deps, args[1:]))
	default:
		return fail(stderr, classified(ExitUsage, "unknown command %q\n\nRun 'ultraplan --help' to see available commands.", args[0]))
	}
}

type dependencies struct {
	stdout               io.Writer
	stderr               io.Writer
	stdin                io.Reader
	ctx                  context.Context
	workDir              string
	workspaceFlag        string
	env                  map[string]string
	tuiRunner            TUIRunner
	webRunner            WebRunner
	sprintRuntimeFactory SprintRuntimeFactory
	runControl           *runControlState
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
  defaults         Install editable built-in prompts and templates.
  skills           Materialise manually invoked stage skills.
  config           Inspect effective configuration.
  code             Extract cited code snippets from reports.
  health           Check workspace, config, filesystem, and environment basics.
  run              Inspect, follow, cancel, and diagnose durable runs.
  project          Inspect projects and validate project indexes.
  sprint           Inspect planning sprint artifact flow state.
  study            Inspect studies, sources, and dimensions.
  tui              Open a read-only terminal dashboard.
  serve            Start the read-only local browser dashboard.
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

func discoverWorkspace(deps dependencies) (workspace.Root, error) {
	env := envLookup(deps.env)
	root, err := workspace.Discover(workspace.DiscoverOptions{
		ExplicitPath: deps.workspaceFlag,
		EnvWorkspace: env("ULTRAPLAN_WORKSPACE"),
		StartDir:     deps.workDir,
	})
	if err != nil {
		return workspace.Root{}, classified(ExitWorkspace, "workspace.discover: %w", err)
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
		return config.Effective{}, classified(ExitConfig, "config.load: %w", err)
	}
	return effective, nil
}
