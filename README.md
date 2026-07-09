# UltraPlan Go

UltraPlan Go is a local-first CLI for durable architecture studies and governed planning artifacts. It initializes study workspaces, runs source and dimension analyses through agentwrap/OpenCode, synthesizes reports, validates artifacts, regenerates summaries, extracts cited code snippets for review, and manages planning projects and sprints through `plan.md`.

This release includes the study workflow and the planning workflow from `study -> select -> distill -> reason -> plan`. Sprint implementation execution, smoke investigation execution, review automation, issue tracking, hosted SaaS, browser UI, multi-user collaboration, automatic Git mutation, signing, notarization, tags, and artifact upload are deferred.

## Install

Install to your user bin directory, which keeps the `ultraplan` command in the same place for future upgrades:

```bash
./scripts/install-ultraplan.sh
```

That script installs `ultraplan` to `~/.local/bin` by default. If you prefer a different bin directory, set `GOBIN` first:

```bash
GOBIN="$HOME/bin" ./scripts/install-ultraplan.sh
```

Build from source:

```bash
go build -o bin/ultraplan ./cmd/ultraplan
```

Run the built binary:

```bash
./bin/ultraplan --help
```

For local release artifacts, see [docs/release-checklist.md](docs/release-checklist.md). This repository packages Linux and macOS binaries under `dist/` when the release checklist is run.

## Quick Start

Initialize a workspace:

```bash
ultraplan init-workspace --path .
```

Check workspace, config, and runtime health:

```bash
ultraplan health
```

Inspect effective configuration:

```bash
ultraplan config show
ultraplan config show --json
```

Initialize a study from YAML:

```bash
ultraplan study init study-init.yml --no-clone
```

List studies, sources, and dimensions:

```bash
ultraplan study list
ultraplan study <study> list
```

Preview prompts without runtime execution:

```bash
ultraplan study <study> prompt analysis 01 <source>
ultraplan study <study> prompt synthesis 01 --output previews/synthesis-01.txt
```

Install editable copies of the built-in prompts and templates only when you want to customize them:

```bash
ultraplan defaults install --dry-run
ultraplan defaults install
```

Run study work:

```bash
ultraplan study <study> run 01 <source>
ultraplan study <study> synthesize 01
ultraplan study <study> run-all --parallel 3
ultraplan study <study> run-loop --parallel 3
ultraplan study <study> run-loop --dimension 01 --source <source> --parallel 1
```

`run-loop` resumes shared study progress by default. Dimension/source filters advance only that slice of the study graph while progress is still stored in `studies/<study>/.ultraplan/run-state.json`. Use `--reset` only when you intentionally want to archive and rebuild study progress.

Validate, inspect status, summarize, and extract code references:

```bash
ultraplan study <study> validate --json
ultraplan study <study> status
ultraplan study <study> summary
ultraplan code studies/<study>/reports/final/01-topic.md --json
```

Inspect planning projects and validate governed sprint artifacts:

```bash
ultraplan project list
ultraplan project <project> status
ultraplan project <project> validate
ultraplan sprint <project> <sprint> status
ultraplan sprint <project> <sprint> validate requirements
ultraplan sprint <project> <sprint> validate sprint-index
ultraplan sprint <project> <sprint> validate execute
ultraplan sprint <project> <sprint> flow --to requirements --dry-run
ultraplan sprint <project> <sprint> flow --to plan --dry-run
ultraplan sprint <project> <sprint> flow --to execute --dry-run
ultraplan sprint <project> <sprint> execute --resume
```

## Documentation

- [User guide](docs/user-guide.md): end-to-end study workflow.
- [CLI reference](docs/cli-reference.md): public commands, flags, exit classes, and stable JSON surfaces.
- [Configuration](docs/configuration.md): `ultraplan.yml`, environment overrides, precedence, redaction, and runtime mapping.
- [Recovery runbook](docs/recovery.md): validation failures, stale locks, cancellation, partial runs, and safe retry.
- [OpenCode smoke](docs/opencode-smoke.md): gated real-runtime smoke procedure outside default tests.
- [Planning smoke](docs/planning-smoke.md): gated planning flow smoke procedure.
- [Migration from `.ultra/cli`](docs/migration-from-ultra-cli.md): planning artifact migration notes.
- [Release checklist](docs/release-checklist.md): local release gates, packaging, checksums, and security review.

## Workspace Model

`init-workspace` creates the minimal required workspace:

```text
README.md
ultraplan.yml
studies/
```

Prompts and templates are built into the CLI. A workspace does not need `prompts/` or `templates/` to run. If a workspace file exists at the same relative path, it overrides the built-in default. Use `ultraplan defaults install` to materialize editable copies:

```text
prompts/
  base.md
  synthesize.md
  create-sprint-index.md
  create-technical-handbook.md
  create-area-reasoning.md
  create-sprint-reasoning.md
  plan-sprint.md
  ...
templates/
  repo-analysis.md
  report.md
  sprint-index.md
  technical-handbook.md
  sprint-reasoning.md
  sprint-plan.md
  ...
```

If an existing workspace prompt or template differs from the built-in default, `defaults install` lists the customized file and asks before overwriting it. Use `--force` only when you intentionally want to replace customized files without confirmation.

Studies live under `studies/<study>/` with editable source, dimension, report, run-state, and summary artifacts. Directory sources are analyzed by path. Live directory-source metadata is stored in `sources/<source>.ultraplan-source.yml` or `sources/<source>/.ultraplan-source.yml`; `applicable_dimensions` there limits which dimensions apply. Top-level Markdown sources can declare the same filter in frontmatter. `study-init.yml` is retained as initialization provenance, not the live applicability contract.

Study reports are dimension-scoped. Per-source reports are written to `studies/<study>/reports/source/<dimension-ref>/<source>.md`, and synthesis writes `studies/<study>/reports/final/<dimension-ref>.md`.

Projects live under `projects/<project>/` with `docs/`, `roadmap.md`, `project-index.md`, and `sprints/<sprint>/`. Planning sprints are editable Markdown/JSON artifact chains through `requirements.md`, `sprint-index.md`, `technical-handbook.md`, optional `reasoning/*.md`, `reasoning.md`, `plan.md`, and `flow-state.json`.

## Runtime Boundary

UltraPlan owns study behavior and artifact validation. Runtime execution is delegated through `github.com/Antonio7098/agentwrap` and its OpenCode adapter. UltraPlan does not claim direct OpenCode process supervision, provider billing ownership, or provider-agnostic guarantees that bypass the configured runtime.

Default tests are offline and fake-first. Real OpenCode smoke is gated by local OpenCode, provider configuration, network availability, and a prepared workspace.

## Development

Run the offline test suite:

```bash
go test ./...
```

Run the race suite:

```bash
go test -race ./...
```

Build the CLI:

```bash
go build ./cmd/ultraplan
```

The architecture keeps product behavior inside product modules:

- `internal/workspace` owns workspace discovery, path safety, and workspace validation.
- `internal/study` owns study workflows, prompts, validation, execution, summaries, and durable state.
- `internal/project` owns project discovery, project-index catalog validation, and project status.
- `internal/sprint` owns planning artifacts, flow state, stage validation, prompt previews, and flow execution through `plan.md`.
- `internal/codeextract` owns citation parsing and snippet extraction.
- `internal/platform/*` owns cross-cutting infrastructure and generic runtime integration.
- `internal/app` owns CLI wiring and process exit behavior.
