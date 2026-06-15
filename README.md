# UltraPlan Go

UltraPlan Go is a local-first CLI for durable architecture studies and governed planning artifacts. It initializes study workspaces, runs source and dimension analyses through agentwrap/OpenCode, synthesizes reports, validates artifacts, regenerates summaries, extracts cited code snippets for review, and manages planning projects and sprints through `plan.md`.

This release includes the study workflow and the planning workflow from `study -> select -> distill -> reason -> plan`. Sprint implementation execution, smoke investigation execution, review automation, issue tracking, hosted SaaS, browser UI, multi-user collaboration, automatic Git mutation, signing, notarization, tags, and artifact upload are deferred.

## Install

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

Run study work:

```bash
ultraplan study <study> run 01 <source>
ultraplan study <study> synthesize 01
ultraplan study <study> run-all --parallel 3
ultraplan study <study> run-loop --parallel 3
```

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
ultraplan sprint <project> <sprint> validate sprint-index
ultraplan sprint <project> <sprint> flow --to plan --dry-run
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

`init-workspace` creates:

```text
ultraplan.yml
prompts/
  base.md
  synthesize.md
templates/
  repo-analysis.md
  report.md
studies/
projects/
```

Studies live under `studies/<study>/` with editable source, dimension, report, run-state, and summary artifacts. Directory sources are analyzed by path. Top-level Markdown sources can use `applicable_dimensions` frontmatter to limit which dimensions apply.

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
