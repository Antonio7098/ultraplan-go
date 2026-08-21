# CLI Reference

This release includes study commands and governed sprint planning, execute, resumable automated review, integrated `verify`, focused review reruns, review-gated deep smoke, the terminal dashboard, and a loopback-only browser dashboard with guarded operations and SSE progress. Issue management, Git mutation, and hosted services remain deferred.

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

### `ultraplan skills materialise`

```text
ultraplan skills materialise [all|stage] [--path <dir>] [--dry-run] [--force]
```

Writes manually invoked sprint-stage skills to
`.agents/skills/ultraplan-<stage>/`. With no selection, all eleven skills are
materialised. Supported stages are `reconcile`, `requirements`, `code-context`, `sprint-index`,
`technical-handbook`, `area-reasoning`, `reasoning`, `plan`, `execute`,
`review`, and `smoke`.

Each generated skill includes `SKILL.md` and `agents/openai.yaml`. Implicit
invocation is disabled. The `SKILL.md` embeds the canonical stage prompt and
adds interactive prerequisite checks, explicit proposal-only behavior,
validation, and flow-state reconciliation.

The manual-only `code-context` skill delegates to the canonical `sprint ... flow --to code-context` operation. It does not reproduce repository selection, prompt construction, artifact validation, atomic replacement, or state transitions.

Behavior matches `defaults install`: missing files are created, identical
files are unchanged, customized files require confirmation, `--force`
overwrites them, and `--dry-run` makes no writes. `materialize` is accepted as
an alias.

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

Shows project docs, roadmap, `project-index.md`, sprints, catalog health,
project-owned area reasoning documents, and the effective project/workspace/built-in
source for each reasoning default without runtime execution.

### `ultraplan project <project> validate`

```text
ultraplan project <project> validate
```

Validates required project files and `project-index.md` catalog references for contracts, evidence reports, reasoning templates, review protocols, and the external smoke harness manifest. It also validates project reasoning overrides and rejects reasoning templates owned by a different project.

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

Lists sources and dimensions for one study. Markdown sources show their applicability filter or `all`. The configured dimension priority from `studies/<study>/study.json` is shown after the natural dimension listing.

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

Runs selected applicable analysis tasks, synthesis tasks, and summary generation with bounded parallelism. `--dimension` and `--source` are repeatable. Configured `dimension_order` entries are strict priority tiers: each listed dimension reaches synthesis or another terminal state before the next tier starts, followed by all unlisted dimensions.

### `ultraplan study <study> run-loop`

```text
ultraplan study <study> run-loop [--dimension <ref>] [--source <ref>] [--parallel <n>] [--force-unlock] [--reset] [--yes]
```

Advances shared durable study progress with per-study locking and `studies/<study>/.ultraplan/run-state.json`. By default, existing progress is resumed, reconciled against current source/dimension applicability metadata, and revalidated. `--dimension` and `--source` select the eligible slice to advance; `study.json` priority applies within that slice. Terminal progress shows both selected-scope and whole-study counts. Use `--reset` to archive and rebuild progress, with confirmation unless `--yes` is provided. Use `--force-unlock` only for operator-confirmed stale locks.

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
ultraplan sprint <project> <sprint> validate requirements
ultraplan sprint <project> <sprint> validate code-context
ultraplan sprint <project> <sprint> validate sprint-index
ultraplan sprint <project> <sprint> validate technical-handbook
ultraplan sprint <project> <sprint> validate area-reasoning
ultraplan sprint <project> <sprint> validate reasoning
ultraplan sprint <project> <sprint> validate plan
ultraplan sprint <project> <sprint> validate execute
ultraplan sprint <project> <sprint> validate review
ultraplan sprint <project> <sprint> validate smoke
```

Validates one planning or execute stage artifact without invoking runtime. `code-context` requires the canonical reference-only Markdown shape with safe repository-relative paths, exact positive line ranges, optional symbols, rationale, and no fenced source. `sprint-index` references must be a subset of `project-index.md`. Plan validation checks traceability to `reasoning.md` and task/evidence checklist structure. Execute validation checks plan task extraction and target safety.

### `ultraplan sprint <project> <sprint> prompt`

```text
ultraplan sprint <project> <sprint> prompt requirements
ultraplan sprint <project> <sprint> prompt code-context
ultraplan sprint <project> <sprint> prompt sprint-index
ultraplan sprint <project> <sprint> prompt technical-handbook
ultraplan sprint <project> <sprint> prompt area-reasoning
ultraplan sprint <project> <sprint> prompt reasoning
ultraplan sprint <project> <sprint> prompt plan
ultraplan sprint <project> <sprint> prompt execute
```

Prints runtime-free prompt previews for planning and execute stages. Prompt previews are for inspection and do not call agentwrap, OpenCode, providers, subprocesses, or the network. Once a completed valid code-context artifact exists, compatible downstream previews use the same shared composition path as runtime requests: exact stored requirements/context bytes, bounded transient contained source evidence, then the stage boundary and stage-specific suffix.

Planning prompts use the same default/override model as study prompts. The prototype markdown prompt is the instruction source; UltraPlan appends a runtime manifest with concrete project, sprint, path, and selection data.

### `ultraplan sprint <project> <sprint> flow`

```text
ultraplan sprint <project> <sprint> flow --to requirements [--dry-run]
ultraplan sprint <project> <sprint> flow --to code-context [--dry-run]
ultraplan sprint <project> <sprint> flow --to sprint-index [--dry-run]
ultraplan sprint <project> <sprint> flow --to technical-handbook [--dry-run]
ultraplan sprint <project> <sprint> flow --to area-reasoning [--dry-run]
ultraplan sprint <project> <sprint> flow --to reasoning [--dry-run]
ultraplan sprint <project> <sprint> flow --to plan [--dry-run]
ultraplan sprint <project> <sprint> flow --to execute [--dry-run]
ultraplan sprint <project> <sprint> flow --to review [--restart-review] [--dry-run]
ultraplan sprint <project> <sprint> flow --to smoke [--restart-review] [--dry-run] [--yes]
```

Runs or previews the governed stage flow through smoke. Cumulative planning order is `requirements -> code-context -> sprint-index -> technical-handbook -> area-reasoning -> reasoning -> plan`; `flow --to plan` dispatches code-context exactly once when it is not already complete and valid. A code-context rerun reads the configured implementation target with restricted permissions and atomically replaces only `code-context.md`. A non-dry-run flow reports each stage as it is checked, started, skipped, completed, or failed and interleaves sanitized runtime progress. Review and smoke use the same sprint-owned transition as `verify`. Compatible interrupted reviews resume by default; `--restart-review` discards retained review progress. A non-dry-run smoke transition requires `--yes`.

### `ultraplan sprint <project> <sprint> execute`

```text
ultraplan sprint <project> <sprint> execute [--task <id>] [--dry-run] [--resume] [--model <provider/model>]
```

Executes validated top-level `plan.md` task checkboxes through the generic runtime boundary. It writes `.run-state.json` and `execute.md`, requires runtime evidence or a safe diagnostic before marking a task complete, and constrains work to the project index target implementation directory.

Explicitly defer accepted follow-up work with a required rationale:

```text
ultraplan sprint <project> <sprint> execute --task <id> --defer --reason "accepted rationale and follow-up owner"
```

Deferral is a durable terminal outcome shown in `execute.md` and status counts. The plan checkbox remains unchecked so deferred work is not represented as implemented. Review accepts it only when the unchecked task's stable ID has a matching governed `deferred` outcome; arbitrary unchecked or manually checked tasks cannot bypass execution.

The normal agent-owned path uses the plan itself. During an active execute attempt, the execute agent may replace the task's top-level `[ ]` marker with `[/]` and append an inline rationale:

```markdown
- [/] **Task 6: Persist shutdown uncertainty** — Deferred: requires a separately governed owner capability in Sprint 32
```

After the runtime returns, UltraPlan validates the reason, preserves the task's stable ID, records the durable `deferred` outcome, and continues with the next task. A `[/]` marker without `— Deferred: <reason>` is invalid. A pre-existing marker cannot bypass a new execution or review unless the matching run-state already records that deferral.

### `ultraplan sprint <project> <sprint> review`

```text
ultraplan sprint <project> <sprint> review [--focus <coverage-id>] [--restart] [--dry-run] [--model <provider/model>] [--parallel <n>] [--json]
```

Runs bounded read-only conformance reviewers and atomically writes `review.md`. Compatible interrupted attempts resume validated coverage and retained OpenCode sessions. Use `--restart` to discard the resumable attempt and start fresh. A focused rerun promotes only when all other coverage can be retained from the same current fingerprint.

Example:

```bash
ultraplan sprint ultraplan-go 28-review-to-smoke-flow review --focus architecture --json
```

### `ultraplan sprint <project> <sprint> smoke`

```text
ultraplan sprint <project> <sprint> smoke [--level <id>|--suite <id>|--test <id>] [--timeout <duration>] [--force-review --override-reason <text>] [--dry-run] [--yes] [--json]
```

Gates on the current review fingerprint, then uses `planning.smoke_model` to create or update a durable sprint-specific suite in the cataloged protocol-v1 harness. Authoring is restricted to manifest-declared paths and targets non-deterministic real boundaries not already settled by normal tests. UltraPlan then discovers enumerated coverage/tests, selects sufficient scope, invokes direct bounded argv, verifies the exact executed test identities and external evidence, and atomically writes `smoke.md` before smoke flow state. `--force-review` additionally requires `--override-reason`; the resulting run is diagnostic and cannot promote the review or overall assessment. Raw streams and run/issue evidence remain external. Timeout, cancellation, missing coverage, out-of-scope author changes, malformed evidence, path escape, and uncertain cleanup never replace a valid summary.

Example:

```bash
ultraplan sprint ultraplan-go 28-review-to-smoke-flow smoke --dry-run --json
```

### `ultraplan sprint <project> <sprint> verify`

```text
ultraplan sprint <project> <sprint> verify [--to review|smoke] [--focus-review <coverage-id>] [--restart-review] [--level <id>|--suite <id>|--test <id>] [--timeout <duration>] [--force-review --override-reason <text>] [--dry-run] [--yes] [--json]
```

Runs the shared execute-evidence → review → smoke transition. It requires complete execute evidence, reuses a current review or resumes compatible unfinished review coverage, and applies the review gate before smoke. `--restart-review` starts all reviewers in fresh sessions. Focused review and narrow smoke selections remain diagnostic unless complete retained or containing coverage proves they can promote canonical evidence.

Example:

```bash
ultraplan sprint ultraplan-go 28-review-to-smoke-flow verify --to smoke --yes
```

### `ultraplan code`

```text
ultraplan code <report>... [--json] [--output <path>]
```

Extracts cited code snippets from one or more reports. Text output is human-oriented. `--json` renders deterministic code extraction JSON with reports, sources, references, diagnostics, unresolved entries, and status.

### `ultraplan serve`

```text
ultraplan [--workspace <path>] serve [--listen <address>] [--open-browser]
```

Starts the guarded local browser dashboard. Workspace selection uses the
normal precedence: global `--workspace`, `ULTRAPLAN_WORKSPACE`, then current
directory ancestry. Configuration is loaded and validated before a listener is
opened.

- `--listen` defaults to `127.0.0.1:8080`. It must contain a numeric loopback
  IP and an explicit port. Canonical examples are `127.0.0.1:8080` and
  `[::1]:8080`; `localhost`, port zero, wildcard, LAN, and public addresses are
  rejected before server startup.
- `--open-browser` asks the platform browser launcher to open the canonical
  bound URL after listening succeeds. A launcher failure is a redacted warning;
  the healthy server continues and the printed URL can be opened manually.
- Startup/listen failures use the existing error exit class. Interrupt or
  process-context cancellation first drains new commands, cancels every active
  server-owned operation through its canonical app context, waits for bounded
  durable reconciliation, then shuts down HTTP. Cleanup uncertainty is not
  reported as success.

`serve --help` never opens a listener or parses the embedded templates.
Existing CLI commands, `ultraplan tui`, and the browser remain independent
adapters over the same typed app operation services. Browser commands use a
two-step current confirmation, bounded ephemeral status/SSE, and explicit
`DELETE` cancellation. They do not change CLI flags or automation semantics;
the browser cannot edit arbitrary files, submit arbitrary commands, or mutate
Git. See
[Local Web Dashboard](local-web.md) for routes, limits, trust boundaries, and
troubleshooting.

### `ultraplan version`

Prints version, commit, build date, and Go version metadata.

## Stable JSON Surfaces

The compatibility-sensitive JSON surfaces in this release are:

- `config show --json`
- `health --json`
- `study <study> validate --json`
- `study <study> status --json`
- `code --json` deterministic extraction result
- `sprint <project> <sprint> status --json`
- `sprint <project> <sprint> review --json`
- `sprint <project> <sprint> smoke --json`
- `sprint <project> <sprint> verify --json`

The Phase 3 field-level compatibility contract is documented in [Phase 3 JSON Schemas](phase3-json-schemas.md).

Sprint `status --json`, `review --json`, and `smoke --json` also expose schema-versioned envelopes. Other text output is intended for humans unless explicitly promoted to stable JSON.
## Durable run commands

All commands operate on the selected workspace (`--workspace` or normal
discovery) and do not contact a provider.

```text
ultraplan run list [--project <ref>] [--sprint <ref>] [--study <ref>]
                   [--lifecycle <comma-list>] [--limit <1..200>]
                   [--after <cursor>] [--json]
ultraplan run show <run-id> [--json]
ultraplan run follow <run-id> [--after <sequence>] [--json]
ultraplan run cancel <run-id> [--reason user_requested] [--json]
ultraplan run diagnostics [--json] [--support-export <file>]
```

`list` is newest-first, defaults to 50 records, and returns an opaque pagination
cursor. Active means only `accepted`, `queued`, `running`, or `cancelling`.
`follow` replays committed events, polls quickly while catching up and at most
once per second while idle. Interrupting follow stops observation only; use
`cancel` for an explicit durable cancellation command. Cancellation is
idempotent and does not overwrite an already-terminal winner.

Diagnostics reports schema/WAL, quota, retention, active/stalled ownership, and
reconciliation backlog. `--support-export` creates a private, bounded file (at
most 1 MiB) containing safe snapshots, event headers/omission facts, health,
config source classes, reconciliation decisions, and the newest sanitized
records from `.ultraplan/run-control.log`; it excludes event payloads, prompts,
provider data, source content, credentials, arbitrary output, and the absolute
workspace path.
