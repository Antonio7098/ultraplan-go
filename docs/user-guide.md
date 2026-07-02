# UltraPlan User Guide

This guide covers the current study and planning release. Planning supports project and sprint artifacts through `plan.md`. Sprint implementation execution, smoke investigation execution, review automation, issue tracking, hosted services, browser UI, multi-user collaboration, and automatic Git mutation are not part of this release.

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

`init-workspace` creates only the required workspace files: `ultraplan.yml` and `studies/`. Prompts and templates are built into the CLI, so a workspace can run without local `prompts/` or `templates/` directories.

If you want editable copies of the built-in defaults, install them:

```bash
ultraplan defaults install --dry-run
ultraplan defaults install
```

Workspace files at the same relative paths override built-ins. For example, `prompts/base.md` overrides the built-in base prompt, and `templates/report.md` overrides the built-in report template.

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

Generated study artifacts are human-editable Markdown and YAML.

## 6. List Studies, Sources, And Dimensions

```bash
ultraplan study list
ultraplan study <study> list
```

Source listing reports directory sources and Markdown document sources. Source entries can declare `applicable_dimensions` in `study-init.yml`, and Markdown document sources can also declare it in frontmatter; if present, UltraPlan skips non-matching dimensions instead of invoking the runtime.

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

`run-all` executes applicable analysis tasks with bounded parallelism, runs synthesis where possible, and writes `studies/<study>/summary.csv`.

## 11. Resume With Run Loop

```bash
ultraplan study <study> run-loop --parallel 3
```

`run-loop` persists `studies/<study>/.ultraplan/run-state.json` after meaningful task transitions, prints compact task progress as it runs, and refuses concurrent runs through a per-study lock. By default, it archives any existing run-state and starts a fresh durable run for the selected filters.

Use `--continue` to resume and revalidate the current run-state:

```bash
ultraplan study <study> run-loop --continue --parallel 3
```

Use `--force-unlock` only after confirming no active process owns the lock:

```bash
ultraplan study <study> run-loop --force-unlock
```

## 12. Inspect Status

```bash
ultraplan study <study> status
ultraplan study <study> status --json
```

Status shows run-state path, task counts, active/failed/cancelled/recent tasks, retry timing, lock diagnostics, safe runtime metadata, usage/cost when known, policy decisions, cleanup, repair, and omitted unsafe payload notes.

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

Planning sprints live under `projects/<project>/sprints/<sprint>/`. The supported chain is `sprint-index`, `technical-handbook`, optional `area-reasoning`, `reasoning`, and `plan`.

```bash
ultraplan sprint <project> <sprint> status
ultraplan sprint <project> <sprint> validate sprint-index
ultraplan sprint <project> <sprint> prompt plan
ultraplan sprint <project> <sprint> flow --to plan --dry-run
```

Use `prompt <stage>` before runtime-backed flow to inspect the stage input. Use `flow --to <stage> --dry-run` to preview planned stage execution. Non-dry-run flow can generate planning artifacts when runtime prerequisites are available.

The planning flow stops at `plan.md`; it does not execute the plan or create smoke, review, issue, or Git artifacts.

Sprint planning prompts are markdown defaults embedded in the CLI, not hand-built Go checklist strings. A workspace can override them by installing defaults and editing files such as `prompts/create-sprint-index.md`, `prompts/create-technical-handbook.md`, `prompts/create-sprint-reasoning.md`, or `prompts/plan-sprint.md`.
