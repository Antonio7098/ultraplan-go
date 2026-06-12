# UltraPlan Go

UltraPlan Go is a local-first Go CLI for durable architecture studies over source repositories. The product direction is to make agent-assisted research reproducible, resumable, inspectable, and backed by concrete code references.

This repository is currently in an active implementation stage. The implemented CLI surface covers workspace setup, configuration inspection, health checks, version metadata, study/source/dimension discovery, study initialization, prompt previews, single analysis execution, synthesis, run-all batch execution, durable resumable run-loop execution, run-state status inspection, and deterministic summary generation. Code-reference extraction is described in the product and technical docs but is not fully implemented yet.

## What It Does Today

- Initializes an UltraPlan workspace with the expected directory and file scaffold.
- Discovers a workspace from `--workspace`, `ULTRAPLAN_WORKSPACE`, or the current directory ancestry.
- Loads and validates `ultraplan.yml` configuration.
- Applies supported `ULTRAPLAN_` environment overrides.
- Prints effective configuration in text or JSON.
- Runs basic health checks for workspace, config, filesystem, and environment state.
- Lists discovered studies.
- Lists a study's source directories and Markdown dimensions.
- Initializes studies from YAML.
- Renders analysis and synthesis prompt previews.
- Runs one analysis task through the configured runtime.
- Synthesizes final reports from valid per-source reports.
- Runs selected applicable study tasks with bounded parallelism using `study <study> run-all`.
- Resumes durable study execution with `study <study> run-loop`, backed by per-study locks and atomic run-state persistence.
- Supports explicit selected-study `--force-unlock` for operator-confirmed stale lock recovery.
- Writes deterministic `studies/<study>/summary.csv`.
- Shows persisted run-state status, lock diagnostics, retry state, task sections, and safe runtime metadata where run-state exists.

## Repository Layout

```text
cmd/ultraplan/                 CLI entrypoint
internal/app/                  command parsing and application wiring
internal/workspace/            workspace discovery, initialization, validation, paths
internal/study/                study workflows, prompts, validation, execution, summary, state
internal/platform/config/      config defaults, file loading, env overrides, validation
internal/platform/logging/     logging primitives
internal/platform/runtime/     runtime integration placeholder
internal/codeextract/          code extraction placeholder
ARCHITECTURE.md                architecture guidance
PRD.md                         product requirements
TRD.md                         technical requirements
```

## Requirements

- Go 1.26 or newer, as declared in `go.mod`.

## Build

```bash
go build -o bin/ultraplan ./cmd/ultraplan
```

Run the built binary:

```bash
./bin/ultraplan --help
```

You can also run directly from source:

```bash
go run ./cmd/ultraplan --help
```

## Quick Start

Initialize a workspace:

```bash
go run ./cmd/ultraplan init-workspace --path .
```

Preview the scaffold without writing files:

```bash
go run ./cmd/ultraplan init-workspace --path . --dry-run
```

Check the workspace:

```bash
go run ./cmd/ultraplan health
```

Inspect effective configuration:

```bash
go run ./cmd/ultraplan config show
go run ./cmd/ultraplan config show --json
```

List studies:

```bash
go run ./cmd/ultraplan study list
```

List one study's sources and dimensions:

```bash
go run ./cmd/ultraplan study <study-name> list
```

## Workspace Structure

`init-workspace` creates the following baseline structure:

```text
ultraplan.yml
prompts/
  base.md
  synthesize.md
templates/
  repo-analysis.md
  report.md
studies/
```

Studies are discovered under `studies/`. A study may contain:

```text
studies/<study-name>/
  sources/
    <source-name>/
  dimensions/
    01-some-topic.md
    02-another-topic.md
```

Dimension files must be Markdown files named with a positive numeric prefix, optionally followed by a slug:

```text
01.md
01-runtime-adapters.md
2 workspace validation.md
```

The CLI normalizes dimension numbers to two digits when listing them.

## Configuration

The default generated `ultraplan.yml` is:

```yaml
version: 1
runtime:
  default: opencode
models:
  default: provider/model
  primary: provider/model
  backup: provider/model
execution:
  default_variant: high
  default_parallel: 3
  default_timeout: 30m
  default_retries: 3
logging:
  format: text
  level: info
agentwrap:
  executable: opencode
  required_health:
    - runtime_available
    - structured_output
    - workdir
```

Supported environment overrides:

```text
ULTRAPLAN_WORKSPACE
ULTRAPLAN_RUNTIME_DEFAULT
ULTRAPLAN_MODEL_DEFAULT
ULTRAPLAN_MODEL_PRIMARY
ULTRAPLAN_MODEL_BACKUP
ULTRAPLAN_DEFAULT_VARIANT
ULTRAPLAN_DEFAULT_PARALLEL
ULTRAPLAN_DEFAULT_TIMEOUT
ULTRAPLAN_DEFAULT_RETRIES
ULTRAPLAN_LOG_FORMAT
ULTRAPLAN_LOG_LEVEL
ULTRAPLAN_AGENTWRAP_EXECUTABLE
```

Validation currently requires:

- `version` is `1`.
- `runtime.default` is `opencode`.
- model names, default variant, and agent wrapper executable are non-empty.
- `execution.default_parallel` is positive.
- `execution.default_timeout` is a positive Go duration, such as `30m`.
- `execution.default_retries` is not negative.
- `logging.format` is `text` or `json`.
- `logging.level` is `debug`, `info`, `warn`, or `error`.
- `agentwrap.required_health` entries are `runtime_available`, `structured_output`, or `workdir`.

## Commands

```text
ultraplan [--workspace <path>] [command]

Commands:
  init-workspace   Initialize an UltraPlan workspace.
  config           Inspect effective configuration.
  health           Check workspace, config, filesystem, and environment basics.
  study            Inspect studies, sources, and dimensions.
  version          Print build metadata.
```

Global workspace selection:

```bash
ultraplan --workspace /path/to/workspace health
ULTRAPLAN_WORKSPACE=/path/to/workspace ultraplan health
```

## Testing

Run the full test suite:

```bash
go test ./...
```

## Development Notes

The architecture intentionally keeps product behavior inside product modules:

- `internal/workspace` owns workspace rules and filesystem safety.
- `internal/study` owns study discovery and study-domain behavior.
- `internal/platform/*` is reserved for cross-cutting infrastructure.
- `internal/app` is the composition and CLI layer.

See `ARCHITECTURE.md`, `PRD.md`, and `TRD.md` for the fuller planned system, including study execution, synthesis, resumability, runtime adapters, and code-reference extraction.
