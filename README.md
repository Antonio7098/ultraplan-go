# UltraPlan Go

UltraPlan Go is a local-first CLI for durable architecture studies and governed sprint delivery. It initializes study workspaces, runs source and dimension analyses through agentwrap/OpenCode, synthesizes reports, validates artifacts, executes sprint plans, runs resumable automated reviews, drives review-gated smoke verification, and embeds manually invoked skills for every sprint stage.

This release includes study workflows and the grounded governed sprint chain `requirements -> code-context -> sprint-index -> technical-handbook -> area-reasoning -> reasoning -> plan -> execute -> review -> smoke`, including integrated `sprint verify`, durable review resume, focused review reruns, and explicit diagnostic smoke overrides. `code-context.md` stores repository-relative references only; later agent-backed stages receive one byte-stable shared prefix containing the exact requirements and context-pack bytes plus bounded transient live source evidence. Agents may still inspect additional repository files. Issue management, hosted SaaS, multi-user collaboration, automatic Git mutation, retrieval/indexing, UltraPlan cache ownership, signing, notarization, tags, and artifact upload remain deferred.

Phase 3 operators should start with the [CLI reference](docs/cli-reference.md), [recovery runbook](docs/recovery.md), [JSON schema contract](docs/phase3-json-schemas.md), and [legacy verification migration guide](docs/phase3-migration.md). The authoritative product requirements, technical requirements, architecture, roadmap, and sprint plans live in the adjacent `ultraplan-go-workspace/projects/ultraplan-go/` planning workspace; this repository does not duplicate them.

## Install

Install to your user bin directory, which keeps the `ultraplan` command in the same place for future upgrades:

```bash
./scripts/install-ultraplan.sh
```

That script installs `ultraplan` from the current checkout to `~/.local/bin` by default. It can be run from any directory. If you prefer a different bin directory, set `GOBIN` first:

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

Materialise all manually invoked stage skills, or just one:

```bash
ultraplan skills materialise
ultraplan skills materialise reasoning
ultraplan skills materialise code-context
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
- [Stage skills](docs/stage-skills.md): manual invocation, prerequisite interaction, materialisation, and state ownership.
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

Reasoning defaults may also be specialised per project. UltraPlan resolves
`create-area-reasoning.md`, `create-sprint-reasoning.md`, and
`sprint-reasoning.md` in this order: project override, workspace override,
then built-in default. Project overrides live at:

```text
projects/<project>/prompts/create-area-reasoning.md
projects/<project>/prompts/create-sprint-reasoning.md
projects/<project>/templates/sprint-reasoning.md
```

Studies live under `studies/<study>/` with editable source, dimension, report, run-state, and summary artifacts. Directory sources are analyzed by path. Live directory-source metadata is stored in `sources/<source>.ultraplan-source.yml` or `sources/<source>/.ultraplan-source.yml`; `applicable_dimensions` there limits which dimensions apply. Top-level Markdown sources can declare the same filter in frontmatter. `study-init.yml` is retained as initialization provenance, not the live applicability contract.

Each initialized study also has an editable `studies/<study>/study.json`. Its optional `dimension_order` list runs the referenced dimensions first, in order, before the remaining dimensions. Each listed dimension completes its applicable analyses and synthesis before the next tier starts; unlisted dimensions retain natural ordering and bounded parallelism. Existing studies without `study.json` retain their current behavior.

Study reports are dimension-scoped. Per-source reports are written to `studies/<study>/reports/source/<dimension-ref>/<source>.md`, and synthesis writes `studies/<study>/reports/final/<dimension-ref>.md`.

Projects live under `projects/<project>/` with `docs/`, `roadmap.md`, `project-index.md`, and `sprints/<sprint>/`. A project can keep specialised area reasoning documents under `projects/<project>/reasoning/` and list them in `project-index.md`. Planning sprints are editable Markdown/JSON artifact chains through `requirements.md`, reference-only `code-context.md`, `sprint-index.md`, `technical-handbook.md`, optional `reasoning/*.md`, `reasoning.md`, `plan.md`, and `flow-state.json`. `flow --to plan` runs code-context exactly once after requirements.

Manually invoked stage skills are materialised under `.agents/skills/`. Each
skill contains the corresponding canonical prompt plus interactive
prerequisite, proposal, validation, and reconciliation rules. They are marked
manual-only and therefore require explicit `$ultraplan-<stage>` invocation.
`$ultraplan-code-context` is the narrow canonical-delegation exception: it invokes `ultraplan sprint "$PROJECT" "$SPRINT" flow --to code-context`, leaving target resolution, read-only runtime policy, validation, atomic promotion, and state transitions in the product operation.

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

- `internal/workspace` owns workspace discovery, path safety, workspace validation, and embedded stage-skill materialisation.
- `internal/study` owns study workflows, prompts, validation, execution, summaries, and durable state.
- `internal/project` owns project discovery, project-index catalog validation, and project status.
- `internal/sprint` owns planning artifacts, flow state, stage validation, prompt previews, and flow execution through `plan.md`.
- `internal/codeextract` owns citation parsing and snippet extraction.
- `internal/platform/*` owns cross-cutting infrastructure and generic runtime integration.
- `internal/app` owns CLI wiring and process exit behavior.
