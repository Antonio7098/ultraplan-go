# CLI Reference

This release includes study commands and planning commands through `plan.md`. Sprint implementation execution, smoke investigation execution, review automation, issue tracking, Git mutation, hosted services, and browser UI are deferred.

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

Creates the minimal required workspace scaffold: `ultraplan.yml` and `studies/`. `--dry-run` prints planned operations without writing files.

Built-in prompts and templates are embedded in the CLI and are not required in the workspace.

### `ultraplan defaults install`

```text
ultraplan defaults install [--path <dir>] [--dry-run] [--force]
```

Writes editable copies of the built-in prompts and templates into a workspace. If `--path` is omitted, the command uses global `--workspace` when present, otherwise the current working directory.

Behavior:

- Missing prompt/template files are created.
- Existing files that exactly match the built-in default are left unchanged.
- Existing files that differ are listed before overwrite.
- Without `--force`, the command asks for confirmation before overwriting customized files.
- A negative or empty answer keeps customized files and creates only non-conflicting missing files.
- `--force` overwrites customized files without asking.
- `--dry-run` prints planned operations and never writes files or asks for confirmation.

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

### `ultraplan project list`

```text
ultraplan project list
```

Lists discovered project roots under `projects/`.

### `ultraplan project <project> status`

```text
ultraplan project <project> status
```

Shows project docs, roadmap, `project-index.md`, sprints, and catalog health without runtime execution.

### `ultraplan project <project> validate`

```text
ultraplan project <project> validate
```

Validates required project files and `project-index.md` catalog references for contracts, evidence reports, reasoning templates, and review protocols.

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

Study prompt rendering first checks workspace overrides such as `prompts/base.md` and `templates/report.md`. If no workspace file exists, it uses the built-in default embedded in the CLI.

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
ultraplan study <study> run-loop [--dimension <ref>] [--source <ref>] [--parallel <n>] [--force-unlock] [--reset] [--yes]
```

Advances shared durable study progress with per-study locking and `studies/<study>/.ultraplan/run-state.json`. By default, existing progress is resumed, reconciled against current source/dimension applicability metadata, and revalidated. `--dimension` and `--source` select the eligible slice to advance; terminal progress shows both selected-scope and whole-study counts. Use `--reset` to archive and rebuild progress, with confirmation unless `--yes` is provided. Use `--force-unlock` only for operator-confirmed stale locks.

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

Shows persisted run-state status without runtime execution. Counts are reconciled against the current discovered source/dimension applicability before output, so edited source metadata is reflected without requiring `run-loop --reset`.

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

### `ultraplan sprint <project> <sprint> status`

```text
ultraplan sprint <project> <sprint> status
```

Inspects planning artifacts and refreshes `projects/<project>/sprints/<sprint>/flow-state.json`.

### `ultraplan sprint <project> <sprint> validate`

```text
ultraplan sprint <project> <sprint> validate sprint-index
ultraplan sprint <project> <sprint> validate technical-handbook
ultraplan sprint <project> <sprint> validate area-reasoning
ultraplan sprint <project> <sprint> validate reasoning
ultraplan sprint <project> <sprint> validate plan
```

Validates one planning stage artifact without executing implementation work. `sprint-index` references must be a subset of `project-index.md`. Plan validation checks traceability to `reasoning.md` and task/evidence checklist structure.

### `ultraplan sprint <project> <sprint> prompt`

```text
ultraplan sprint <project> <sprint> prompt sprint-index
ultraplan sprint <project> <sprint> prompt technical-handbook
ultraplan sprint <project> <sprint> prompt area-reasoning
ultraplan sprint <project> <sprint> prompt reasoning
ultraplan sprint <project> <sprint> prompt plan
```

Prints runtime-free prompt previews for planning stages. Prompt previews are for inspection and do not call agentwrap, OpenCode, providers, subprocesses, or the network.

Planning prompts use the same default/override model as study prompts. The prototype markdown prompt is the instruction source; UltraPlan appends a runtime manifest with concrete project, sprint, path, and selection data.

### `ultraplan sprint <project> <sprint> flow`

```text
ultraplan sprint <project> <sprint> flow --to sprint-index [--dry-run]
ultraplan sprint <project> <sprint> flow --to technical-handbook [--dry-run]
ultraplan sprint <project> <sprint> flow --to area-reasoning [--dry-run]
ultraplan sprint <project> <sprint> flow --to reasoning [--dry-run]
ultraplan sprint <project> <sprint> flow --to plan [--dry-run]
```

Runs or previews the planning artifact flow through the requested stage. The supported stage chain is `sprint-index`, `technical-handbook`, `area-reasoning`, `reasoning`, and `plan`. The command does not execute implementation, smoke, review, issue, Git, prompt-generation, or hosted workflows.

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

Project and sprint planning commands currently expose text output only. Other text output is intended for humans unless a future release explicitly promotes it to stable JSON.
