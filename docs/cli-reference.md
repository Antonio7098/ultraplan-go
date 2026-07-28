# CLI Reference

This release includes study commands and governed sprint planning, execute, resumable automated review, integrated `verify`, focused review reruns, and review-gated deep smoke. Issue management, Git mutation, hosted services, and browser UI remain deferred.

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

Human-readable errors are printed to stderr. JSON commands use documented envelopes or deterministic command-specific JSON described below. Runtime-backed study and sprint commands also stream sanitized progress while preserving final result output. Standalone, run-all, and sprint progress uses stderr so stdout remains machine-readable; the durable study run-loop retains its task-progress stream on stdout. Progress includes lifecycle, provider progress, tool, validation, retry/fallback, permission, warning, and terminal events; message bodies and raw provider payloads are not printed.

## Commands

### `ultraplan init-workspace`

```text
ultraplan init-workspace [--path <dir>] [--dry-run]
```

Creates the minimal required workspace scaffold: `README.md`, `ultraplan.yml`, and `studies/`. The README includes common workspace commands. `--dry-run` prints planned operations without writing files.

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

Validates required project files and `project-index.md` catalog references for contracts, evidence reports, reasoning templates, review protocols, and the external smoke harness manifest.

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
ultraplan sprint <project> <sprint> status [--json]
```

Inspects planning, execute, review, and smoke state and refreshes `projects/<project>/sprints/<sprint>/flow-state.json`. Static smoke readiness validates the catalog, manifest, review gate, artifact, fingerprint, and evidence paths without launching discovery or a run.

### `ultraplan sprint <project> <sprint> validate`

```text
ultraplan sprint <project> <sprint> validate sprint-index
ultraplan sprint <project> <sprint> validate technical-handbook
ultraplan sprint <project> <sprint> validate area-reasoning
ultraplan sprint <project> <sprint> validate reasoning
ultraplan sprint <project> <sprint> validate plan
ultraplan sprint <project> <sprint> validate execute
ultraplan sprint <project> <sprint> validate review
ultraplan sprint <project> <sprint> validate smoke
```

Validates one planning or execute stage artifact without invoking runtime. `sprint-index` references must be a subset of `project-index.md`. Plan validation checks traceability to `reasoning.md` and task/evidence checklist structure. Execute validation checks plan task extraction and target safety.

### `ultraplan sprint <project> <sprint> prompt`

```text
ultraplan sprint <project> <sprint> prompt sprint-index
ultraplan sprint <project> <sprint> prompt technical-handbook
ultraplan sprint <project> <sprint> prompt area-reasoning
ultraplan sprint <project> <sprint> prompt reasoning
ultraplan sprint <project> <sprint> prompt plan
ultraplan sprint <project> <sprint> prompt execute
```

Prints runtime-free prompt previews for planning and execute stages. Prompt previews are for inspection and do not call agentwrap, OpenCode, providers, subprocesses, or the network.

Planning prompts use the same default/override model as study prompts. The prototype markdown prompt is the instruction source; UltraPlan appends a runtime manifest with concrete project, sprint, path, and selection data.

### `ultraplan sprint <project> <sprint> flow`

```text
ultraplan sprint <project> <sprint> flow --to requirements [--dry-run]
ultraplan sprint <project> <sprint> flow --to sprint-index [--dry-run]
ultraplan sprint <project> <sprint> flow --to technical-handbook [--dry-run]
ultraplan sprint <project> <sprint> flow --to area-reasoning [--dry-run]
ultraplan sprint <project> <sprint> flow --to reasoning [--dry-run]
ultraplan sprint <project> <sprint> flow --to plan [--dry-run]
ultraplan sprint <project> <sprint> flow --to execute [--dry-run]
ultraplan sprint <project> <sprint> flow --to review [--restart-review] [--dry-run]
ultraplan sprint <project> <sprint> flow --to smoke [--restart-review] [--dry-run] [--yes]
```

Runs or previews the governed stage flow through smoke. A non-dry-run flow reports each stage as it is checked, started, skipped, completed, or failed and interleaves sanitized runtime progress. Review and smoke use the same sprint-owned transition as `verify`. Compatible interrupted reviews resume by default; `--restart-review` discards retained review progress. A non-dry-run smoke transition requires `--yes`.

### `ultraplan sprint <project> <sprint> execute`

```text
ultraplan sprint <project> <sprint> execute [--task <id>] [--dry-run] [--resume] [--model <provider/model>]
```

Executes validated top-level `plan.md` task checkboxes through the generic runtime boundary. It writes `.run-state.json` and `execute.md`, requires runtime evidence or a safe diagnostic before marking a task complete, and constrains work to the project index target implementation directory.

### `ultraplan sprint <project> <sprint> review`

```text
ultraplan sprint <project> <sprint> review [--focus <coverage-id>] [--restart] [--dry-run] [--model <provider/model>] [--parallel <n>] [--json]
```

Runs bounded read-only conformance reviewers and atomically writes `review.md`. Compatible interrupted attempts resume validated coverage and retained OpenCode sessions. Use `--restart` to discard the resumable attempt and start fresh. A focused rerun promotes only when all other coverage can be retained from the same current fingerprint.

### `ultraplan sprint <project> <sprint> smoke`

```text
ultraplan sprint <project> <sprint> smoke [--level <id>|--suite <id>|--test <id>] [--timeout <duration>] [--force-review --override-reason <text>] [--dry-run] [--yes] [--json]
```

Discovers the cataloged protocol-v1 harness, gates on the current review fingerprint, selects sufficient scope, invokes direct bounded argv, validates external evidence, and atomically writes `smoke.md` before smoke flow state. `--force-review` additionally requires `--override-reason`; the resulting run is diagnostic and cannot promote the review or overall assessment. Raw streams and run/issue evidence remain external. Timeout, cancellation, malformed evidence, path escape, and uncertain cleanup never replace a valid summary.

### `ultraplan sprint <project> <sprint> verify`

```text
ultraplan sprint <project> <sprint> verify [--to review|smoke] [--focus-review <coverage-id>] [--restart-review] [--level <id>|--suite <id>|--test <id>] [--timeout <duration>] [--force-review --override-reason <text>] [--dry-run] [--yes] [--json]
```

Runs the shared execute-evidence → review → smoke transition. It requires complete execute evidence, reuses a current review or resumes compatible unfinished review coverage, and applies the review gate before smoke. `--restart-review` starts all reviewers in fresh sessions. Focused review and narrow smoke selections remain diagnostic unless complete retained or containing coverage proves they can promote canonical evidence.

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

Sprint `status --json`, `review --json`, and `smoke --json` also expose schema-versioned envelopes. Other text output is intended for humans unless explicitly promoted to stable JSON.
