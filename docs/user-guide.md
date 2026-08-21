# UltraPlan User Guide

This guide covers study workflows and governed sprint delivery through execute, resumable automated review, integrated verification, review-gated deep smoke, and the local loopback browser dashboard. Issue management, hosted services, remote/multi-user browser access, and automatic Git mutation are not part of this release.

## Local Browser Dashboard

Run `ultraplan serve` inside a workspace, then open the exact numeric-loopback
URL printed by the command. Use `--listen 127.0.0.1:<port>` or `[::1]:<port>`
to choose a different local address and `--open-browser` to request automatic
launch. The browser reads the same app-owned project, sprint, study, validation,
artifact, review, and smoke projections used by CLI/TUI surfaces.

Guarded actions use prepare -> review normalized scope -> confirm -> observe.
Closing or refreshing a tab cancels only observation; use the visible cancel
action or interrupt the owning process to cancel work. After an event gap,
expired operation, interruption, or restart, refresh the linked durable project,
sprint, or study status before acting. JavaScript improves live progress but all
inspection, confirmation, operation status, cancellation, and recovery pages
retain server-rendered fallbacks.

## 1. Build Or Install

From the repository root:

```bash
go build -o bin/ultraplan ./cmd/ultraplan
```

Use `bin/ultraplan` directly or put it on `PATH`.

## 2. Create A Workspace

Initialize the current directory:

```bash
ultraplan init-workspace --path .
```

Preview without writing:

```bash
ultraplan init-workspace --path . --dry-run
```

`init-workspace` creates only the required workspace files: `README.md`, `ultraplan.yml`, and `studies/`. The README includes common health, config, study, planning, and defaults commands. Prompts and templates are built into the CLI, so a workspace can run without local `prompts/` or `templates/` directories.

Materialise the optional, manually invoked sprint-stage skills when the
workspace will be used directly by an agent:

```bash
ultraplan skills materialise --dry-run
ultraplan skills materialise
```

Use `ultraplan skills materialise <stage>` to install just one. Skills are
written under `.agents/skills`, are explicit-invocation-only, and preserve
customized local copies unless overwrite is confirmed.

If you want editable copies of the built-in defaults, install them:

```bash
ultraplan defaults install --dry-run
ultraplan defaults install
```

Workspace files at the same relative paths override built-ins. For example, `prompts/base.md` overrides the built-in base prompt, and `templates/report.md` overrides the built-in report template.

Reasoning prompts and the final reasoning template may be specialised for one
project under `projects/<project>/prompts/` and
`projects/<project>/templates/sprint-reasoning.md`. Their precedence is project,
workspace, then built-in. Area-specific reasoning source documents belong under
`projects/<project>/reasoning/` and must be listed in that project's
`project-index.md`.

If `defaults install` finds an existing prompt or template that differs from the built-in default, it lists the file and asks before overwriting it. Answering anything other than `yes` keeps the customized file. Use `--force` only when you intentionally want to overwrite customized prompt/template files without confirmation.

Study reports are grouped by dimension. Analysis writes `studies/<study>/reports/source/<dimension-ref>/<source>.md`; synthesis writes `studies/<study>/reports/final/<dimension-ref>.md`.

Workspace discovery uses this order:

1. `--workspace <path>`
2. `ULTRAPLAN_WORKSPACE`
3. current directory ancestry containing `ultraplan.yml`

## 3. Configure Runtime Defaults

Edit `ultraplan.yml`, then inspect the effective config:

```bash
ultraplan config show
ultraplan config show --json
```

The default runtime is `opencode` through agentwrap. Runtime/provider secrets should stay in runtime-native environment or provider config, not in `ultraplan.yml`.

## 4. Check Health

Run:

```bash
ultraplan health
ultraplan health --json
```

Health checks workspace discovery, required workspace files, config validation, environment override presence, and configured runtime health/capabilities when config is valid.

## 5. Initialize A Study

Create a `study-init.yml`, then run:

```bash
ultraplan study init study-init.yml --no-clone
```

Useful flags:

- `--dry-run`: print planned directories, files, and clone actions.
- `--force`: allow overwriting an existing study output.
- `--no-clone`: create source directories without cloning repositories.
- `--output <dir>`: choose a workspace-relative output directory.

Generated study artifacts are human-editable Markdown, YAML, and JSON.

Initialization creates `studies/<study>/study.json` for live study execution settings:

```json
{
  "version": 1,
  "dimension_order": [
    "04.03",
    "01-execution-semantics"
  ]
}
```

The list is optional and may contain the same unambiguous dimension references accepted by study commands. Listed dimensions execute first in the configured order. UltraPlan completes the applicable analyses and synthesis for each listed dimension before admitting the next configured dimension. Afterward, unlisted dimensions use their natural order and existing bounded parallelism. A missing file or empty list preserves the default behavior. Unknown, ambiguous, and duplicate references are rejected.

## 6. List Studies, Sources, And Dimensions

```bash
ultraplan study list
ultraplan study <study> list
```

Source listing reports directory sources and Markdown document sources. Directory sources can declare `applicable_dimensions` in `sources/<source>.ultraplan-source.yml` or `sources/<source>/.ultraplan-source.yml`; Markdown document sources can declare it in frontmatter. If present, UltraPlan skips non-matching dimensions instead of invoking the runtime. `study-init.yml` remains the initialization input/provenance file and is not used as the live applicability source after initialization.

## 7. Preview Prompts

Preview analysis:

```bash
ultraplan study <study> prompt analysis <dimension> <source>
```

Preview synthesis:

```bash
ultraplan study <study> prompt synthesis <dimension> --output previews/synthesis.txt
```

Prompt preview renders a deterministic manifest plus prompt text. It does not execute agentwrap, OpenCode, providers, subprocesses, or network calls.

The preview also shows whether prompt/template content came from a workspace override or a built-in default. Built-in sources are shown with a `builtin:` prefix.

## 8. Run One Analysis

```bash
ultraplan study <study> run <dimension> <source>
```

The command composes the prompt, invokes the configured runtime, expects the per-source report to be written, validates the report, and exits non-zero if runtime execution or validation fails. Inapplicable Markdown source/dimension pairs are skipped with a clear message.

## 9. Synthesize A Final Report

```bash
ultraplan study <study> synthesize <dimension>
```

Synthesis checks required per-source reports first. Missing or invalid inputs block synthesis instead of producing a misleading final report.

## 10. Run A Batch

```bash
ultraplan study <study> run-all --parallel 3
```

Optional filters:

```bash
ultraplan study <study> run-all --dimension 01 --source <source>
```

`run-all` executes applicable analysis tasks with bounded parallelism, runs synthesis where possible, and writes `studies/<study>/summary.csv`. When `study.json` defines `dimension_order`, configured dimensions run as strict priority tiers before the remaining dimensions.

## 11. Resume With Run Loop

```bash
ultraplan study <study> run-loop --parallel 3
```

`run-loop` persists shared study progress in `studies/<study>/.ultraplan/run-state.json` after meaningful task transitions, prints compact task progress as it runs, and refuses concurrent invocations through a per-study lock. By default, it resumes existing progress; dimension/source filters only choose which slice of the study graph is eligible to advance. On each start, it reconciles the persisted task graph against the current discovered source/dimension applicability so status totals and scheduling follow source metadata updates. It reads `dimension_order` on every invocation, so editing `study.json` changes the next scheduling decision without resetting durable progress.

Memory diagnostics are appended to `studies/<study>/.ultraplan/diagnostics/run-loop-memory.jsonl`. Samples are written at state load/save and runtime boundaries and every five seconds, and include Go heap usage, process RSS/high-water/swap, state-file size, task ID, and phase duration. The file rotates to `.1` at 8 MiB so diagnostics cannot grow without bound.

Use filters to advance a specific slice without creating a separate run:

```bash
ultraplan study <study> run-loop --dimension 01 --source temporal --parallel 1
```

Use `--reset` only when you intentionally want to archive and rebuild study progress. The command asks for confirmation unless `--yes` is also provided.

Use `--force-unlock` only after confirming no active process owns the lock:

```bash
ultraplan study <study> run-loop --force-unlock
```

## 12. Inspect Status

```bash
ultraplan study <study> status
ultraplan study <study> status --json
```

Status shows run-state path, task counts, active/failed/cancelled/recent tasks, retry timing, lock diagnostics, safe runtime metadata, usage/cost when known, policy decisions, cleanup, repair, and omitted unsafe payload notes. Status reconciles counts against the current discovered source/dimension applicability before rendering.

## 13. Validate Artifacts

```bash
ultraplan study <study> validate
ultraplan study <study> validate --json
```

Validation checks study artifacts without runtime execution. Treat a validation failure as a product failure even if the runtime reported success.

## 14. Regenerate Summary

```bash
ultraplan study <study> summary
```

This regenerates `studies/<study>/summary.csv` from existing reports without runtime execution.

## 15. Extract Code References

```bash
ultraplan code studies/<study>/reports/final/01-topic.md
ultraplan code studies/<study>/reports/final/01-topic.md --json --output evidence/code-refs.json
```

Code extraction resolves cited file and line references from reports back to source snippets. Unresolved citations are reported and return a partial/validation exit class depending on the failure.

## 16. Inspect Planning Projects

Projects live under `projects/<project>/` and contain `docs/`, `roadmap.md`, `project-index.md`, and sprint directories.

```bash
ultraplan project list
ultraplan project <project> status
ultraplan project <project> validate
```

Project validation checks that the project catalog resolves selected contracts, evidence reports, reasoning templates, review protocols, and source documents.

## 17. Work Through Sprint Planning

Planning sprints live under `projects/<project>/sprints/<sprint>/`. The supported chain is `requirements`, `code-context`, `sprint-index`, `technical-handbook`, optional `area-reasoning`, `reasoning`, `plan`, controlled `execute`, automated `review`, and review-gated `smoke`.

```bash
ultraplan sprint <project> <sprint> status
ultraplan sprint <project> <sprint> validate requirements
ultraplan sprint <project> <sprint> prompt code-context
ultraplan sprint <project> <sprint> validate code-context
ultraplan sprint <project> <sprint> flow --to code-context
ultraplan sprint <project> <sprint> validate sprint-index
ultraplan sprint <project> <sprint> validate execute
ultraplan sprint <project> <sprint> prompt requirements
ultraplan sprint <project> <sprint> prompt plan
ultraplan sprint <project> <sprint> prompt execute
ultraplan sprint <project> <sprint> flow --to plan --dry-run
ultraplan sprint <project> <sprint> flow --to execute --dry-run
ultraplan sprint <project> <sprint> execute --resume
```

Use `prompt <stage>` before runtime-backed flow to inspect the stage input. Use `flow --to <stage> --dry-run` to preview planned stage execution. Non-dry-run flow can generate planning artifacts when runtime prerequisites are available. While it runs, the CLI prints stage transitions and sanitized runtime progress to stderr, leaving the final result on stdout. The TUI shows the same stage and runtime events in the active operation view for sprint flow, execute, and review.

`code-context` reads the configured implementation target under a restricted read-only runtime policy and atomically replaces only the sprint's `code-context.md` after structural validation. The artifact records paths, exact line ranges, optional symbols, and rationale—not copied source. Downstream agent-backed prompts share the exact requirements and code-context bytes plus transient bounded source excerpts, clearly marked untrusted; the context pack never prevents further live repository inspection. A bad path, range, encoding, containment check, changed read, or 256 KiB shared-prefix budget overflow stops before runtime invocation and leaves the last valid artifacts/state intact.

The planning flow continues through controlled execute from validated `plan.md` tasks. One reusable agent session owns the ordered pending-task queue: its first turn receives shared sprint context and the queue, later tasks use compact continuation turns, and UltraPlan checkpoints task status and evidence between turns. Execute writes `.run-state.json` and `execute.md`; automated review then writes the current `review.md`.

Use the integrated transition after execute:

```bash
ultraplan sprint <project> <sprint> verify --to review --dry-run
ultraplan sprint <project> <sprint> verify --to smoke --dry-run
ultraplan sprint <project> <sprint> verify --to smoke --yes
ultraplan sprint <project> <sprint> validate smoke
```

`verify` requires complete execute evidence, obtains or reuses a current review, then applies the smoke gate. Interrupted reviews resume validated coverage and retained OpenCode sessions by default. Use `review --restart` or `verify --restart-review` when you intentionally want fresh sessions; a restart cannot be combined with focused review.

The smoke preview shows the review gate, sufficient scope, prerequisites, duration/cost class where supplied, safe argv, and external evidence roots. `--force-review` additionally requires `--override-reason` and is diagnostic-only after a current failed/blocked review; it cannot override stale or malformed review evidence or improve the overall assessment. Raw harness evidence stays in its `runs/` and `issues/` directories, while the sprint stores only `smoke.md` and flow state.

Sprint planning prompts are markdown defaults embedded in the CLI, not hand-built Go checklist strings. A workspace can override them by installing defaults and editing files such as `prompts/create-requirements.md`, `prompts/create-sprint-index.md`, `prompts/create-technical-handbook.md`, `prompts/create-sprint-reasoning.md`, or `prompts/plan-sprint.md`. A project may override only the area-reasoning prompt, final-reasoning prompt, and final-reasoning template. Project status reports which source is effective.

The materialised stage skills are interactive forms of those prompts. Invoke
them explicitly as `$ultraplan-requirements`, `$ultraplan-code-context`,
`$ultraplan-sprint-index`, `$ultraplan-technical-handbook`,
`$ultraplan-area-reasoning`, `$ultraplan-reasoning`, `$ultraplan-plan`,
`$ultraplan-execute`, `$ultraplan-review`, or `$ultraplan-smoke`. A skill
checks status and validates prerequisites first. If gaps exist, it must ask
before filling them. Unless the user requests a proposal or discussion only,
the skill performs the selected stage and reconciles status afterward.

For area and final reasoning, an explicit deep-dive request should become an
interactive design discussion covering evidence, alternatives, trade-offs,
risks, technical debt, and future consequences before conclusions are written.
See [Manually Invoked Stage Skills](stage-skills.md) for the complete contract.
## Watching and controlling durable runs

Every runtime-backed CLI command, confirmed web/TUI operation, and external
smoke execution receives a durable run before work starts. Use `ultraplan run
list` or the web/TUI Runs view to find it. The same ID, lifecycle, liveness,
cancellation state, product status, retention state, and event cursor are shown
on every surface.

Lifecycle answers what the operation has durably decided; liveness answers
whether its current owner is known to be running. Product status is separate.
For example, `running / stalled / product unknown` is meaningful and must not
be displayed as success. Likewise, a cancellation request can lose the
immutable terminal race to a completion that had already committed.

Use `run follow` for observation and `run cancel` for control. Closing a tab,
leaving a TUI view, quitting the TUI, or interrupting `run follow` does not
cancel work. A compacted or tombstone record remains truthful about its current
snapshot even when full event history is no longer available.
