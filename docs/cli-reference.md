# CLI Reference

All commands are study-side commands. Target and sprint workflows are deferred and are not part of this public release surface.

## Global Usage

```text
ultraplan [--workspace <path>] [command]
```

Global flags:

- `--workspace <path>`: use an explicit workspace path.
- `-h`, `--help`: show help.

Workspace discovery order is explicit flag, `ULTRAPLAN_WORKSPACE`, then current directory ancestry containing `ultraplan.yml`.

## Exit Classes

The CLI uses numeric process statuses:

- `0`: success.
- `1`: internal or write error.
- `2`: usage error.
- `3`: configuration error.
- `4`: workspace or filesystem error.
- `5`: validation/reference error.
- `6`: runtime/provider error.
- `7`: cancellation.
- `8`: partial completion.

Human-readable errors are printed to stderr. JSON commands use documented envelopes or deterministic command-specific JSON described below.

## Commands

### `ultraplan init-workspace`

```text
ultraplan init-workspace [--path <dir>] [--dry-run]
```

Creates the baseline workspace scaffold. `--dry-run` prints planned operations without writing files.

### `ultraplan config show`

```text
ultraplan config show [--json]
```

Prints effective configuration after defaults, workspace config, environment overrides, and supported command flags. Sensitive values are redacted.

`--json` uses the stable JSON envelope:

```json
{
  "schema_version": 1,
  "command": "config show",
  "workspace": "/path/to/workspace",
  "status": "ok",
  "generated_at": "2026-06-13T00:00:00Z",
  "result": {}
}
```

### `ultraplan health`

```text
ultraplan health [--json]
```

Checks workspace discovery, workspace structure, config validation, filesystem readability, environment override presence, and configured runtime health/capability checks when possible.

`--json` uses the stable JSON envelope with `result.schema_version: 1` and a `checks` array.

### `ultraplan study init`

```text
ultraplan study init <study-init.yml> [--dry-run] [--force] [--no-clone] [--output <dir>]
```

Initializes a study from YAML. Clone failures can return partial completion while still reporting created artifacts.

### `ultraplan study list`

```text
ultraplan study list
```

Lists discovered studies under `studies/`.

### `ultraplan study <study> list`

```text
ultraplan study <study> list
```

Lists sources and dimensions for one study. Markdown sources show their applicability filter or `all`.

### `ultraplan study <study> prompt`

```text
ultraplan study <study> prompt analysis <dimension> <source> [--output <file>]
ultraplan study <study> prompt synthesis <dimension> [--output <file>]
```

Renders a deterministic manifest and prompt text. It does not invoke runtime execution.

### `ultraplan study <study> run`

```text
ultraplan study <study> run <dimension> <source>
```

Runs one analysis task through configured agentwrap/OpenCode runtime and validates the expected per-source report.

### `ultraplan study <study> synthesize`

```text
ultraplan study <study> synthesize <dimension>
```

Runs one synthesis task after validating required per-source reports.

### `ultraplan study <study> run-all`

```text
ultraplan study <study> run-all [--dimension <ref>] [--source <ref>] [--parallel <n>]
```

Runs selected applicable analysis tasks, synthesis tasks, and summary generation with bounded parallelism. `--dimension` and `--source` are repeatable.

### `ultraplan study <study> run-loop`

```text
ultraplan study <study> run-loop [--dimension <ref>] [--source <ref>] [--parallel <n>] [--force-unlock]
```

Runs or resumes durable study execution with per-study locking and `studies/<study>/.ultraplan/run-state.json`. Use `--force-unlock` only for operator-confirmed stale locks.

### `ultraplan study <study> validate`

```text
ultraplan study <study> validate [--json]
```

Validates study artifacts without runtime execution.

`--json` uses the stable JSON envelope with `command: "study.validate"`. The result contains redacted validation checks and report checks.

### `ultraplan study <study> status`

```text
ultraplan study <study> status [--json]
```

Shows persisted run-state status without runtime execution.

`--json` uses the stable JSON envelope with `command: "study.status"` and `result.schema_version: 1`. The stable result includes:

- `run_id`, `complete`, and `state_path`.
- `counts` for pending, running, validating, completed, failed, cancelled, skipped, waiting, retrying, active, and retries.
- optional redacted `lock`.
- `run_metadata` with timestamps, filters, and config summary.
- `tasks` with IDs, kind, status, dimension/source, output path, attempts, retry timing, redacted errors, validation summary, agent status, usage, and cost.
- aggregate `usage` and `cost` where known.

Debug/runtime raw payloads are not a stable public JSON surface.

### `ultraplan study <study> summary`

```text
ultraplan study <study> summary
```

Regenerates deterministic `studies/<study>/summary.csv` from existing reports without runtime execution.

### `ultraplan code`

```text
ultraplan code <report>... [--json] [--output <path>]
```

Extracts cited code snippets from one or more reports. Text output is human-oriented. `--json` renders deterministic code extraction JSON with reports, sources, references, diagnostics, unresolved entries, and status.

### `ultraplan version`

Prints version, commit, build date, and Go version metadata.

## Stable JSON Surfaces

The compatibility-sensitive JSON surfaces in this release are:

- `config show --json`
- `health --json`
- `study <study> validate --json`
- `study <study> status --json`
- `code --json` deterministic extraction result

Other text output is intended for humans unless a future release explicitly promotes it to stable JSON.
