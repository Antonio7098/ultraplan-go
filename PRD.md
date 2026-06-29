# Product Requirements Document: UltraPlan Go

**Version:** 1.1.0
**Status:** Draft
**Owner:** Product and Engineering
**Last Updated:** 2026-06-15

## Stage 1: Product Brief

### 1.1 Executive Summary

UltraPlan Go is a production-grade command-line planning and research system that runs architecture studies across source repositories, synthesizes structured reports, extracts cited code references, and creates governed planning artifacts through `plan.md`. Sprint implementation execution remains deferred.

- **Problem Statement:** The current prototype proves the workflow, but it is script-like, tightly coupled to a local Bun/TypeScript environment, and not hardened for durable runs, reproducible outputs, reliable retries, clear configuration, or long-term extensibility.
- **Proposed Solution:** Rebuild UltraPlan as a Go CLI with a well-defined domain model, deterministic filesystem layout, resumable orchestration, structured runtime adapters, robust validation, and production-quality testing.
- **Target Users:** Engineers, technical leads, AI workflow builders, and product teams using agentic coding runtimes to study codebases, compare architectural patterns, and plan future implementation work from separately reviewed study outputs.
- **Expected Outcome:** Users can initialize studies, run large batches of source analyses, synthesize final reports, extract code citations, inspect study status with durable state, and use selected evidence to produce governed planning artifacts through `plan.md`.

### 1.2 Product Context

The prototype in `ultraplan/cli` demonstrates the core product shape:

- A global `study` CLI.
- Study directories with sources, dimensions, per-source reports, final reports, and summaries.
- YAML-based study initialization.
- Agent runtime execution through OpenCode.
- Parallel and resumable study runs.
- Structured prompts and output templates.
- Code-reference extraction from generated reports.
- Project documents for PRDs, TRDs, roadmaps, project indexes, sprint requirements, sprint indexes, technical handbooks, reasoning, and plans in the prototype.
- Sprint planning and sprint execution workflows in the prototype.

For UltraPlan Go, the planning artifact chain is included through `plan.md`. Sprint implementation execution, smoke investigation execution, review automation, issue tracking, automatic Git mutation, hosted services, and browser UI remain future scope.

The Go product keeps those core capabilities but makes them reliable, portable, testable, and extensible.

### 1.3 The Opportunity

- **Why Now:** Teams are increasingly using autonomous coding agents for research and implementation, but durable planning workflows still depend on ad hoc scripts, one-off prompts, manual citation chasing, and fragile long-running terminal sessions.
- **Problem Archetype:** Hair on Fire and Hard Fact. Long-running AI research/planning workflows are valuable, but failures, missing outputs, weak citations, and non-resumable execution create immediate operational pain.
- **Strategic Fit:** UltraPlan Go first becomes the durable study backbone for architecture research. Applying study findings to target definition, sprint planning, and implementation execution is a later product phase.
- **North Star Metric:** Percentage of planned study runs that complete with valid artifacts, valid citations, and no manual recovery.
- **Strategic Priority:** High. The product converts exploratory AI planning into repeatable engineering infrastructure.

### 1.4 Product Principles

- **Cited claims over unsupported summaries:** Every architectural claim in generated reports should trace to a concrete source reference where applicable.
- **Runtime success is not product success:** A run is not complete until required outputs exist and pass validation.
- **Resumability by default:** Long-running work must survive interruption, process exit, and partial failures.
- **Explicit state and explicit errors:** Users must be able to inspect what is pending, running, failed, retrying, completed, and why.
- **Small composable primitives:** Studies, dimensions, sources, runs, and reports must remain clear concepts. Targets and sprints are acknowledged future concepts, not current build requirements.
- **Product workflows stay product-owned:** Runtime adapters execute agent work; UltraPlan owns study behavior. Target planning behavior is deferred.
- **Portable local-first operation:** The primary product is a CLI that works from a repository checkout without requiring a server.

### 1.5 Target Personas

**Architecture Researcher**

- Role: Senior engineer or technical lead.
- Goals: Compare mature open-source implementations across dimensions and extract usable design guidance.
- Pain Points: Manual research does not scale, citations are hard to verify, and synthesis loses source detail.
- Key Behaviors: Creates study definitions, reviews generated reports, inspects code citations, and uses findings as input to later design decisions.

**Agent Workflow Operator**

- Role: Developer running long batches of autonomous agent work.
- Goals: Start, monitor, pause, resume, and recover many agent tasks.
- Pain Points: Runtime failures, rate limits, missing artifacts, and process interruptions require manual babysitting.
- Key Behaviors: Uses `run-loop`, status views, retry policies, validation reports, and logs.

**Tool Extender**

- Role: Engineer adding new runtime adapters, validators, output formats, or product commands.
- Goals: Extend UltraPlan without rewriting the study model or breaking existing workflows.
- Pain Points: Prototype code combines CLI parsing, prompts, state, runtime calls, filesystem writes, and validation in one layer.
- Key Behaviors: Works in Go packages with stable interfaces, tests, fixtures, and clear extension points.

### 1.6 User Scenarios

1. **Initialize a comparative architecture study**
   - As an architecture researcher, I want to create a new study from a YAML definition so that sources, dimensions, report folders, and usage docs are generated consistently.

2. **Run one source against one dimension**
   - As a researcher, I want to run a single dimension/source analysis so that I can test prompts, validate a dimension, and inspect output before launching a batch.

3. **Run a complete study**
   - As an operator, I want to run all dimensions against all sources with controlled parallelism so that large research batches complete without hand scheduling.

4. **Resume interrupted work**
   - As an operator, I want a stateful loop to resume incomplete analyses and syntheses so that terminal interruption or runtime failure does not restart completed work.

5. **Synthesize per-source reports**
   - As a researcher, I want all per-source reports for a dimension synthesized into a final comparative report so that I can understand cross-source patterns and tradeoffs.

6. **Extract code references**
   - As a reviewer, I want a command that resolves cited file and line references to source snippets so that claims in a report can be audited quickly.

7. **Inspect status**
    - As an operator, I want clear status for studies, active tasks, failed tasks, retry times, outputs, and summaries so that I can decide whether to wait, retry, inspect, or intervene.

### 1.7 Business and Product Goals

- Provide a durable Go implementation of the proven prototype workflow.
- Reduce manual recovery in large study runs.
- Improve confidence in generated reports through citation validation and report validation.
- Keep the design compatible with later target planning and sprint execution without implementing those workflows now.
- Allow future runtime support without changing the study workflow model.
- Keep the CLI understandable enough that users can inspect and edit generated artifacts directly.

### 1.8 Non-Goals

- Do not build a hosted SaaS product in the first production release.
- Do not build a web UI in the first production release.
- Do not require users to adopt one particular AI provider.
- Do not hide generated files behind an opaque database-only workflow.
- Do not make study dimensions immutable; users must be able to edit Markdown/YAML artifacts.
- Do not turn UltraPlan into a general-purpose workflow engine.
- Do not implement project management features such as assignment, scheduling, burndown charts, or issue tracker replacement.
- Do not depend on free-form terminal text when a runtime offers structured output.

## Stage 2: Solution Specification

### 2.1 Product Surface

UltraPlan Go ships as a single CLI binary, tentatively named `ultraplan`.

The CLI must support these top-level areas:

- Study discovery and inspection.
- Study initialization from YAML.
- Single analysis runs.
- Full study runs.
- Stateful resumable run loops.
- Run status reporting.
- Final report synthesis.
- Summary generation.
- Code-reference extraction.
- Target initialization and inspection.
- Configuration inspection and validation.
- Health checks for runtime dependencies.

The command surface should be stable, scriptable, and documented through help output.

### 2.2 Workspace Model

UltraPlan operates inside a workspace root. The workspace root contains shared configuration and studies. Prompt and report templates are embedded in the CLI as built-in defaults; workspace `prompts/` and `templates/` files are optional overrides. Target directories may exist from the prototype but are future scope for UltraPlan Go.

Required workspace concepts:

- **Workspace:** The root directory that contains UltraPlan-managed artifacts.
- **Study:** A named comparative research project.
- **Source:** A local source input studied against dimensions. A source may be a directory, usually a code repository, or a Markdown document file placed directly under a study's `sources/` directory.
- **Dimension:** A Markdown-defined architectural concern or analysis prompt.
- **Per-source report:** Output for one dimension/source pair.
- **Final report:** Synthesis for one dimension across all sources.
- **Summary:** CSV or structured summary of source scores across dimensions.

### 2.3 Functional Requirements

#### Must Have

1. **CLI bootstrap and help**
   - Users can run `ultraplan --help`.
   - Users can discover available commands and command-specific options.
   - Invalid commands return non-zero exit codes and actionable errors.

2. **Workspace discovery**
   - CLI can locate the workspace root from the current directory or explicit `--workspace`.
   - CLI can validate required workspace folders and config files.
   - CLI errors clearly when run outside a workspace.

3. **Configuration loading**
   - CLI reads workspace configuration from a structured config file.
   - CLI supports command-line overrides for model, runtime, variant, timeout, parallelism, batch size, and output path.
   - CLI exposes effective configuration for debugging.
   - Sensitive values must not be printed by default.

4. **Study listing**
   - Users can list all studies.
   - Users can list a study's sources and dimensions.
   - Study source listing must show whether each source is a directory source or Markdown document source.
   - Markdown document sources with `applicable_dimensions` frontmatter must show or expose their dimension filter.
   - Listing output is stable enough for humans and scripts.

5. **Study initialization**
   - Users can initialize a study from a YAML file.
   - YAML supports study name, description, repository/source items, dimension items, desired counts, and optional dimension content.
   - CLI creates study folders, dimension Markdown files, source folder, report folders, study README, and normalized `study-init.yml`.
   - CLI supports dry run, force overwrite, no-clone, custom output directory, and field overrides.
   - Source cloning, when enabled, uses shallow clones and reports per-source success/failure.

6. **Assisted study completion**
   - When a YAML file declares a desired source count greater than the explicit source items, the CLI can ask the configured runtime to suggest additional sources.
   - When a YAML file declares a desired dimension count greater than the explicit dimension items, the CLI can ask the configured runtime to suggest additional dimensions.
   - Suggested additions must be written to a cache artifact before they are merged into the normalized study definition.
   - Runtime-generated source and dimension suggestions must be validated before use.
   - Dry-run mode must show how many sources or dimensions would be requested without invoking the runtime.
   - Users must be able to disable assisted completion and proceed only with explicit YAML entries.

7. **Dimension file generation**
   - Generated dimension files include purpose, steps, expected citations, questions, rating rubric, and output guidance.
   - Generated files are human-editable Markdown.
   - Dimension numbering is stable and zero-padded.

8. **Prompt composition**
   - CLI composes analysis prompts from shared base instructions, selected dimension file, selected source path, and report template.
   - CLI composes synthesis prompts from synthesis instructions, selected dimension file, per-source report manifest, and final report template.
   - CLI uses built-in prompt and template defaults when workspace overrides are absent.
   - CLI can install editable prompt and template defaults into a workspace.
   - CLI asks before overwriting existing customized prompt/template files unless an explicit force flag is used.
   - Directory source prompts instruct the runtime to explore only the source directory and cite code paths with line numbers.
   - Markdown document source prompts embed the stripped document body directly and instruct the runtime to analyze only the embedded document content without external code or filesystem exploration.
   - Dry-run mode prints or writes prompt previews without executing runtime work.

9. **Single analysis run**
   - Users can run one dimension against one source.
   - If a Markdown document source declares that the selected dimension is not applicable, the command must skip the analysis with a clear message instead of invoking the runtime.
   - CLI creates the output folder if needed.
   - Runtime is invoked with configured working directory, model, variant, timeout, and permission mode.
   - CLI validates that the expected per-source report was written.
   - CLI returns non-zero status if runtime execution or output validation fails.

10. **Full study run**
   - Users can run all matching dimension/source pairs with controlled parallelism.
   - Users can filter by dimensions and sources.
   - The task matrix must exclude Markdown document sources for dimensions not listed in their `applicable_dimensions` frontmatter.
   - CLI synthesizes final reports after per-source analyses complete.
   - CLI generates or updates a summary after analyses and final reports.

11. **Stateful run loop**
   - Users can start or resume a long-running batch.
   - CLI stores durable run state in the study directory.
   - Run state includes task status, attempts, errors, timestamps, retry times, and completion markers.
   - CLI detects completed output files and does not redo valid completed tasks unless requested.
   - CLI validates that state marked complete still has the required files.
   - CLI handles interrupted runs by saving state before exit when possible.
   - CLI state creation, resume validation, completed-source detection, and synthesis gating must ignore inapplicable Markdown document source/dimension pairs.
   - CLI schedules synthesis only when all applicable source reports for a dimension exist and pass validation.

12. **Retry and backoff**
    - CLI supports retry with bounded backoff.
    - CLI classifies failures enough to decide whether retry is allowed.
    - Rate limits are classified separately from generic runtime failures.
    - Users can configure retry count, backoff schedule, timeout, and fallback model/runtime options.

13. **Runtime health checks**
    - CLI can check whether required runtime executables are available.
    - CLI can check configured provider/model availability where runtime support allows it.
    - CLI can fail fast before expensive runs when required setup is missing.

14. **Runtime adapter for OpenCode**
    - First production runtime adapter supports OpenCode.
    - Adapter uses structured output when available.
    - Adapter preserves native event payloads for diagnostics.
    - Adapter maps native events into canonical UltraPlan events.
    - Adapter captures stderr diagnostics, exit status, timeout, cancellation, usage metadata when available, and final status.

15. **Canonical event stream**
    - CLI and internal packages share a canonical event model for lifecycle, messages, tool activity, artifacts, warnings, errors, rate limits, usage, validation, retry, fallback, and final result.
    - Unknown future event types must not crash the system by default.

16. **Report validation**
    - Per-source reports must be checked for existence, non-empty content, required headings, rating, and citation shape where applicable.
    - Final reports must be checked for existence, non-empty content, required headings, rating summary, source table, and citation discipline where applicable.
    - Validation failures must include clear repair context.

17. **Code-reference extraction**
    - Users can run a command against one or more reports to resolve inline code references.
    - The extractor parses a sources table from a report.
    - The extractor resolves references such as `path/to/file.go:42`, ranges, and selected line lists.
    - Output includes source name, path, line numbers, and code snippets.
    - Unresolved references are reported with enough detail to fix the report.
    - Users can write extraction output to a file.

18. **Summary generation**
    - CLI generates a summary of source scores across dimensions.
    - Summary is deterministic.
    - Missing scores are represented distinctly from zero.
    - Inapplicable Markdown document source/dimension pairs are represented distinctly from missing expected reports.
    - Sources are sorted by total score by default.

19. **Markdown document sources**
    - Users can place Markdown files directly in `studies/<study>/sources/` alongside source directories.
    - Markdown document sources may include YAML frontmatter.
    - Supported frontmatter must include `applicable_dimensions`, a list of dimension numbers that the document should be analyzed against.
    - Dimension numbers in `applicable_dimensions` must be normalized so `1` and `01` match the same dimension.
    - Markdown document sources without `applicable_dimensions` are applicable to every dimension by default.
    - Frontmatter must be stripped before document content is embedded in analysis prompts.
    - Markdown document analysis must not require code citations or code exploration.

20. **Logging and diagnostics**
    - CLI exposes human-readable progress logs by default.
    - CLI supports structured JSON logs for automation.
    - Logs include run IDs, task IDs, dimension/source identifiers, attempt numbers, runtime, model, duration, and status.
    - Debug logs include safe diagnostics without leaking secrets.

21. **Cancellation**
    - Users can interrupt active runs.
    - CLI attempts to cancel owned runtime processes.
    - State is saved before exit when possible.
    - Cancellation is visible in run state and logs.

22. **Tests and fixtures**
    - The product includes unit tests for domain models, config, path resolution, state transitions, report validation, code extraction, and prompt composition.
    - The product includes fake runtime fixtures for deterministic runtime behavior.
    - OpenCode integration tests are gated so normal test runs do not require OpenCode.

#### Should Have

1. **Machine-readable output modes**
   - Commands that list, status, validate, or inspect state support JSON output.

2. **Repair prompts**
   - Validation failures can trigger bounded repair attempts when configured.
   - Repair prompts include missing output details and previous run context.

3. **Run history inspection**
   - Users can inspect prior run attempts and validation results.

4. **Config initialization**
   - Users can generate a starter workspace config.

5. **Template validation**
   - CLI can validate prompt and report template overrides for required placeholders.

6. **Git integration hooks**
   - CLI can optionally run configured post-write commands such as formatting, tests, commit, or push.
   - These hooks are disabled by default and must be explicit.

7. **Runtime usage metadata**
   - CLI records tokens, cost estimates, duration, and model/provider metadata where available.

8. **Report path normalization**
   - CLI normalizes report paths in generated prompts and outputs so workspaces remain relocatable.

9. **Schema versioning**
   - Run state, config, and structured artifacts include schema versions.

10. **Workspace migration checks**
    - CLI can detect old schema versions and recommend or run migrations.

#### Could Have

1. Additional runtime adapters beyond OpenCode.
2. Direct provider/model workers for non-coding document generation.
3. Watch mode for status dashboards.
4. Terminal UI for active run monitoring.
5. Remote artifact storage.
6. Embedded local API server for future UI integration.
7. Pluggable report output formats beyond Markdown and CSV.
8. Advanced source acquisition beyond Git clone, such as local tarballs or registries.

#### Won't Have In First Production Release

1. Hosted multi-user service.
2. Browser UI.
3. Organization-level permissions.
4. Built-in issue tracker integration.
5. Full workflow DAG engine.
6. Sprint implementation execution, smoke investigation execution, review automation, or issue tracking.
7. Silent auto-commit or auto-push by default.

### 2.4 Command Requirements

The exact command names may be refined during implementation, but the production CLI must cover this surface.

#### Global Commands

- `ultraplan list`
  - Lists studies. Target listing is deferred.

- `ultraplan init-workspace`
  - Creates the minimal workspace structure and starter config.

- `ultraplan defaults install`
  - Installs editable copies of built-in prompts and templates.
  - Lists customized files and asks before overwriting them unless forced.

- `ultraplan config show`
  - Shows effective config with secrets redacted.

- `ultraplan health`
  - Runs workspace and runtime health checks.

- `ultraplan code <report>...`
  - Extracts cited code snippets from reports.

#### Study Commands

- `ultraplan study list`
  - Lists studies.

- `ultraplan study init <study-init.yml>`
  - Initializes a study from YAML.

- `ultraplan study <study> list`
  - Lists sources and dimensions for one study.

- `ultraplan study <study> run <dimension-ref> <source-ref>`
  - Runs one analysis.

- `ultraplan study <study> run-all`
  - Runs all selected analyses and synthesis.

- `ultraplan study <study> run-loop`
  - Runs or resumes stateful batch execution.

- `ultraplan study <study> status`
  - Prints run state and progress.

- `ultraplan study <study> synthesize <dimension-ref>`
  - Synthesizes one dimension from existing per-source reports.

- `ultraplan study <study> summary`
  - Regenerates score summary.

- `ultraplan study <study> validate`
  - Validates study structure and generated artifacts.

#### Project And Sprint Planning Commands

UltraPlan Go supports governed planning artifacts through `plan.md`:

- `ultraplan project list`
  - Lists discovered project roots.

- `ultraplan project <project> status`
  - Shows docs, roadmap, project index, sprints, and catalog health.

- `ultraplan project <project> validate`
  - Validates project files and `project-index.md` catalog references.

- `ultraplan sprint <project> <sprint> status`
  - Inspects planning artifacts and refreshes `flow-state.json`.

- `ultraplan sprint <project> <sprint> validate <stage>`
  - Validates planning stages through `plan`.

- `ultraplan sprint <project> <sprint> prompt <stage>`
  - Prints runtime-free stage prompt previews.

- `ultraplan sprint <project> <sprint> flow --to <stage> [--dry-run]`
  - Runs or previews planning flow through `plan.md`.

The following command families remain deferred: `ultraplan target ...`, sprint implementation execution, smoke investigation execution, review automation, issue tracking, and Git mutation.

### 2.5 Study Initialization Requirements

Input YAML must support:

```yaml
name: go-cli-study
description: Comparative architecture study for Go CLIs
repos:
  count: 3
  items:
    - name: example
      url: https://github.com/org/example
      description: Why this source matters
    - name: guide
      path: sources/guide.md
      description: Markdown guide source analyzed only for dimensions declared in frontmatter
dimensions:
  count: 2
  items:
    - number: "01"
      name: project-structure
      title: Project Structure
      description: Boundaries and package layout
      purpose: What this dimension analyzes
      steps:
        - Inspect module and package layout
      citations:
        - Source files implementing key boundaries
      questions:
        - How are responsibilities separated?
```

Acceptance criteria:

- Missing required fields produce actionable validation errors.
- Counts cannot be less than item counts unless explicitly allowed.
- Dimension numbers are normalized to two digits.
- Generated dimension files preserve supplied steps, citations, and questions.
- Runtime-suggested missing sources and dimensions are cached, validated, deduplicated, and included in the normalized `study-init.yml`.
- Users can opt out of runtime-suggested additions.
- Existing study directories are protected unless `--force` is set.
- Dry run shows planned directories and files.

### 2.5.1 Markdown Document Source Requirements

A Markdown document source is a `.md` file placed directly under a study's `sources/` directory, for example:

```text
studies/<study>/sources/my-guide.md
```

Markdown document sources may declare dimension applicability with YAML frontmatter:

```markdown
---
applicable_dimensions:
  - 1
  - 3
  - 5
---
# My Guide

Content relevant to dimensions 1, 3, and 5.
```

Requirements:

- Source discovery must discover both source directories and top-level `.md` files under `sources/`.
- Directory sources keep the existing behavior: the runtime explores source files and cites code file paths with line numbers.
- Markdown document sources are read as documents, not repositories.
- Frontmatter is parsed before scheduling.
- Frontmatter is stripped before document content is embedded into prompts.
- `applicable_dimensions` values are normalized to two-digit dimension numbers for matching.
- A Markdown document source with `applicable_dimensions: [1, 3, 5]` is analyzed only against dimensions `01`, `03`, and `05`.
- A Markdown document source without `applicable_dimensions` is analyzed against all dimensions.
- Inapplicable document source/dimension pairs are skipped in single runs, batch runs, run-loop state creation, resume validation, completed-source detection, synthesis gating, and summary generation.
- Skipped inapplicable pairs are not failures.
- Markdown document prompts must say that all material is in the embedded document and that the runtime must not access external files or code.
- Markdown document report validation must not require code citations unless a dimension explicitly requires document section citations or another non-code citation rule.

### 2.6 Study Execution Requirements

Each analysis task must have:

- Stable task ID.
- Study name.
- Dimension number and slug.
- Source name.
- Source kind: directory or Markdown document.
- Output path.
- Runtime config.
- Attempt number.
- Status.
- Start/end timestamps.
- Error classification.
- Validation result.

Lifecycle states:

- Pending.
- Ready.
- Running.
- Waiting.
- Retrying.
- Validating.
- Repairing.
- Completed.
- Failed.
- Cancelled.
- Skipped.

Per-source analysis success requires:

- Runtime exits successfully.
- Expected report file exists.
- Report is non-empty.
- Required sections are present.
- Rating can be parsed.
- Required citation format is present for directory sources unless the dimension explicitly permits otherwise.
- Markdown document source reports satisfy document-analysis validation rules and are not failed for lack of code citations.

Synthesis success requires:

- All required applicable per-source reports are valid.
- Runtime exits successfully.
- Final report file exists.
- Final report is non-empty.
- Required sections are present.
- Source summary table exists.
- Rating summary exists.

### 2.7 Planning Scope

The TypeScript prototype demonstrates a second side of UltraPlan: applying study findings to project requirements, roadmaps, decisions, sprint planning, and sprint execution. UltraPlan Go includes the governed planning artifact chain through `plan.md`.

Current planning requirements stop at producing and validating project and sprint planning artifacts. Sprint implementation execution, smoke investigation execution, review automation, issue tracking, automatic Git mutation, hosted services, and browser UI should be considered future product work unless this PRD is revised.

### 2.8 User Experience Requirements

- Default output should be calm, concise, and useful during long runs.
- Status output should show totals, completed counts, failed tasks, pending tasks, retry times, and current active work.
- Errors should include what failed, why it failed, where to inspect state, and a suggested next command.
- Destructive operations such as overwrite should require explicit flags or interactive confirmation.
- Dry-run modes should be available for initialization, prompt execution and batch runs.
- Commands should work from nested directories inside a workspace.
- Generated files should be readable and editable without proprietary tooling.

### 2.9 Data and Artifact Requirements

UltraPlan must create deterministic, reviewable artifacts.

Required generated artifacts:

- Study dimension Markdown files.
- Per-source analysis Markdown files.
- Final synthesis Markdown files.
- Study summary CSV.
- Study README.
- Normalized study YAML.
- Run state JSON.
- Run logs or event records.
- Optional code extraction bundles.
- Project indexes.
- Sprint planning artifacts through `plan.md`.
- Sprint flow-state JSON.

Artifact requirements:

- Use stable paths.
- Avoid absolute paths in generated files unless necessary for local diagnostics.
- Include schema/version metadata where the artifact is machine-read.
- Be safe to review in Git.
- Avoid secret values.

## Stage 3: Execution Plan

### 3.1 Technical Implications

- **Language:** Go.
- **Distribution:** Single CLI binary.
- **Storage:** Local filesystem artifacts in the workspace.
- **Runtime Integration:** Adapter-based execution with OpenCode as the first adapter.
- **Concurrency:** Bounded worker pools with durable task state.
- **Validation:** First-class validators for config, study structure, reports, code references, and run state.
- **Testing:** Unit tests, fixture tests, fake runtime tests, and gated real-runtime integration tests.
- **Compatibility:** Linux and macOS are required for first release; Windows support should not be intentionally blocked but may be later-release.

### 3.2 System Architecture

UltraPlan Go should be organized around these responsibilities:

- CLI command layer.
- Workspace discovery and path resolution.
- Configuration loading and validation.
- Domain model for studies, runs, reports, and runtime events.
- Study initialization service.
- Prompt composition service.
- Runtime adapter service.
- Run scheduler and state store.
- Report validation service.
- Synthesis service.
- Code-reference extraction service.
- Project catalog and sprint planning services through `plan.md`.
- Logging and diagnostics.

Data flow for a per-source analysis:

```text
CLI command
  -> workspace discovery
  -> config resolution
  -> study/dimension/source resolution
  -> prompt composition
  -> runtime adapter execution
  -> event stream and logs
  -> expected artifact validation
  -> run state update
  -> summary update
```

Data flow for a study batch:

```text
CLI command
  -> task graph creation
  -> durable run state
  -> bounded worker pool
  -> per-source analysis tasks
  -> validation
  -> synthesis task scheduling
  -> final report generation
  -> summary generation
  -> terminal and structured status
```

Deferred future data flow: applying study findings to targets and sprints is intentionally excluded from the current execution plan.

### 3.3 Dependencies

Required external dependencies:

- Git executable for cloning sources when enabled.
- OpenCode executable for the first runtime adapter.
- One or more configured AI providers, depending on runtime configuration.

Optional dependencies:

- User-configured post-run commands.
- Additional runtime executables in future releases.

Internal dependencies:

- Stable prompt templates.
- Stable report templates.
- Deterministic built-in defaults with workspace override support.
- Workspace config schema.
- Validator definitions.

### 3.4 Performance Requirements

- CLI startup for simple commands should complete in under 250 ms on a normal developer machine, excluding filesystem scans of large source trees.
- Listing studies should avoid recursively scanning source repositories.
- Run-loop status should load state in under 1 second for thousands of tasks.
- Code-reference extraction should cache file lookup work within one command invocation.
- Bounded parallelism must prevent unbounded process creation.
- Large reports should be streamed or read intentionally; avoid accidental loading of entire source repositories into memory.
- Markdown document sources should be read once per prompt build or validation pass and should not trigger repository-style recursive scans.

### 3.5 Reliability Requirements

- Run state must be written atomically.
- Partial writes must not corrupt the last known good state.
- Interrupted runs must be resumable.
- Completed tasks must be revalidated before being trusted.
- Runtime exit code 0 must not bypass output validation.
- Cleanup failures must be visible.
- Failed tasks must retain error details and next action.
- Retry loops must be bounded.

### 3.6 Security Requirements

- Secrets must not be logged by default.
- Config display must redact sensitive fields.
- Runtime environment construction must separate safe and sensitive values.
- Generated artifacts must not include API keys or secret env values.
- Source analysis must respect source isolation requirements.
- Markdown document source analysis must respect document isolation requirements: no external filesystem or code access should be requested by the prompt.
- Commands that overwrite or delete user files must require explicit flags or interactive confirmation.
- User-provided paths must be normalized and checked against the workspace where appropriate.

### 3.7 Observability Requirements

UltraPlan must expose:

- Human-readable progress.
- Structured JSON logs when requested.
- Per-run event records.
- Attempt history.
- Runtime/model/provider metadata.
- Durations.
- Validation results.
- Retry/fallback decisions.
- Warnings and diagnostics.

Status output must answer:

- What is running?
- What is done?
- What failed?
- What is waiting?
- What will retry and when?
- Which artifacts were produced?
- Which validations failed?

### 3.8 Non-Goals and Icebox

The first production release excludes:

- Hosted service mode.
- Browser UI.
- Multi-user auth.
- Team collaboration features.
- Full remote execution service.
- Plugin marketplace.
- Complex workflow DAG authoring.
- Automatic source selection from a vague natural-language goal.

Deferred possibilities:

- TUI dashboard.
- Local API server.
- Remote worker mode.
- Additional runtime adapters.
- Rich HTML report rendering.
- Integration with GitHub issues or pull requests.

### 3.9 Risks and Mitigations

- **Risk:** Runtime structured output changes.
  - **Mitigation:** Preserve raw events, version native decoders, fixture-test unknown and malformed events.

- **Risk:** Generated reports omit required files or sections.
  - **Mitigation:** Treat validators as product gates, not optional checks.

- **Risk:** Long runs create corrupted state after interruption.
  - **Mitigation:** Atomic state writes and startup revalidation.

- **Risk:** CLI surface grows confusing.
  - **Mitigation:** Keep commands grouped by workspace, study, and config.

- **Risk:** Too much workflow logic enters runtime adapters.
  - **Mitigation:** Keep adapters limited to execution and event projection; product services own study orchestration and validation.

- **Risk:** Users rely on non-deterministic generated content without review.
  - **Mitigation:** Generated files remain Markdown/YAML and validation is explicit.

- **Risk:** Source repositories are huge.
  - **Mitigation:** Avoid recursive scans except in targeted code-reference lookup, cache lookups within commands, and allow explicit source paths.

## Stage 4: Validation and Launch Plan

### 4.1 Success Metrics

**Primary KPI: Completed Valid Runs**

- Definition: Percentage of requested study analysis and synthesis tasks that complete with valid required artifacts.
- Target: 95% for deterministic fake-runtime test suites; 85% for real OpenCode smoke batches in controlled environments.
- Measurement: Run state records and validation results.

**Secondary KPI: Manual Recovery Rate**

- Definition: Percentage of batch runs requiring manual file repair, state editing, or task rescheduling.
- Target: Less than 10% for supported runtime configurations.
- Measurement: Failed task classifications, repair attempts, and user-reported interventions.

**Secondary KPI: Citation Resolution Rate**

- Definition: Percentage of code references in reports that resolve to local source files.
- Target: 95% or higher for reports generated from local source repositories.
- Measurement: `ultraplan code` resolution summary.

**Guardrail Metric: Invalid Success Rate**

- Definition: Runs marked completed despite missing or invalid required artifacts.
- Target: 0 known cases.
- Measurement: Validator failures, test fixtures, and manual audit.

### 4.2 Definition of Done

The first production release is done when:

- CLI builds as a Go binary.
- Workspace initialization and validation work.
- Study initialization works from YAML.
- Study listing works.
- Single analysis run works with fake runtime and OpenCode adapter.
- Full study run works with controlled parallelism.
- Stateful run-loop persists and resumes state.
- Synthesis works from validated per-source reports.
- Summary generation works.
- Code-reference extraction works.
- Config and health commands work.
- Unit and fixture tests pass.
- Gated OpenCode smoke test path is documented.
- User-facing docs describe the supported workflow.

### 4.3 Instrumentation Requirements

Track these event categories:

- Command started.
- Command completed.
- Command failed.
- Runtime health check started/completed/failed.
- Task queued.
- Task started.
- Runtime event received.
- Artifact detected.
- Validation started/completed/failed.
- Retry scheduled.
- Fallback selected.
- Task completed.
- Task failed.
- Task cancelled.
- Summary generated.

Metrics to monitor locally:

- Task duration.
- Runtime duration.
- Validation duration.
- Attempts per task.
- Failure category counts.
- Citation resolution counts.
- Generated artifact counts.
- Pending/running/completed/failed task counts.

### 4.4 Rollout Strategy

**Phase 1: Internal CLI parity**

- Implement the core prototype behavior in Go.
- Use fake runtime tests and fixture reports.
- Validate generated directory structures against prototype expectations.

**Phase 2: Real runtime smoke**

- Run small OpenCode-backed studies.
- Verify structured events, report writes, validation, status, and retry behavior.
- Compare outputs to prototype workflows where applicable.

**Phase 3: Production hardening**

- Add migration checks, richer validation, docs, error polish, and release packaging.

### 4.5 Review History

| Date | Reviewer | Status | Notes |
|------|----------|--------|-------|
| 2026-05-25 | Product/Engineering | Draft | Initial production PRD based on the prototype workflow. |

### 4.6 Changelog

| Version | Date | Change | Rationale |
|---------|------|--------|-----------|
| 1.0.0 | 2026-05-25 | Initial full PRD | Establish production requirements for UltraPlan Go. |
