# Technical Requirements Document: UltraPlan Go

**Version:** 1.1.0
**Status:** Draft
**Owner:** Engineering
**Last Updated:** 2026-06-15

## 1. Purpose

This TRD defines the technical requirements for UltraPlan Go, a production-grade Go CLI that implements the proven UltraPlan workflow: study initialization, source analysis, report synthesis, code-reference extraction, resumable orchestration, validation, operational diagnostics, and governed project/sprint planning through `plan.md`. Sprint implementation execution is intentionally deferred.

This document is implementation-oriented. It defines boundaries, modules, data models, state machines, validators, runtime contracts, error handling, and testing requirements. It does not prescribe every package name or third-party library, but it should be specific enough to guide implementation.

UltraPlan Go must use `github.com/Antonio7098/agentwrap` and its `opencode` adapter for agentic runtime supervision. UltraPlan should not reimplement agentwrap's runtime contract, OpenCode process handling, canonical events, policy execution, output validation, repair, permission translation, health checks, observability records, or run inspection primitives.

## 2. System Scope

UltraPlan Go is responsible for:

- Managing a local UltraPlan workspace.
- Initializing and validating study definitions.
- Running source/dimension analysis tasks through runtime adapters.
- Synthesizing final reports from per-source reports.
- Maintaining durable run state for long-running batches.
- Validating generated artifacts.
- Extracting code snippets from report citations.
- Managing project planning roots under `projects/<project>`.
- Validating project indexes that catalog contracts, evidence, reasoning templates, and review protocols.
- Creating and validating sprint planning artifacts through `plan.md`.
- Providing human-readable and structured operational output.

UltraPlan Go is not responsible for:

- Hosting a multi-user service.
- Providing a browser application.
- Managing team permissions.
- Replacing source control, issue trackers, or project management systems.
- Owning AI provider billing.
- Guaranteeing semantic correctness of generated prose beyond validation rules.
- Reimplementing agentwrap SDK features that already exist as runtime-neutral primitives.
- Executing implementation tasks from sprint plans.
- Running smoke investigations, review automation, issue tracking, or Git mutation.

## 3. Architecture Overview

UltraPlan Go should be module-driven, not global-layer-driven. Product behavior should live with the module that owns the relevant state and workflow. Shared platform packages should be limited to genuinely cross-cutting infrastructure such as configuration, logging, filesystem helpers, and runtime execution.

The detailed architecture is defined in [ARCHITECTURE.md](ARCHITECTURE.md). The initial implementation should follow this high-level shape:

```text
cmd/
  ultraplan/
    main.go

internal/
  app/                  # composition root and dependency wiring
  platform/             # generic infrastructure only
    config/
    logging/
    filesystem/
    runtime/
  workspace/            # workspace discovery, paths, and validation
  study/                # study lifecycle, prompts, scheduling, validation, reports, state
  project/              # project docs, project-index cataloging and validation
  sprint/               # planning artifacts, flow state, prompts, and validators through plan
  codeextract/          # citation parsing, file resolution, snippet extraction
```

The core rule is:

```text
Platform owns generic capabilities.
Modules own product behavior.
Logic stays near the state it transforms.
Interfaces appear only at external or volatile boundaries.
```

Do not create global technical-layer packages such as `internal/validation`, `internal/scheduler`, `internal/reports`, or `internal/prompts` unless the behavior is genuinely reusable across multiple product modules. Prefer keeping behavior inside the module that owns the state and workflow, for example `internal/study/validation.go`, `internal/study/scheduler.go`, and `internal/study/reports.go`.

Runtime supervision is delegated to `agentwrap`. The platform runtime package must stay generic: it may know about prompts, working directories, models, timeouts, permissions, events, and execution results, but it must not know about studies, dimensions, sources, synthesis gating, report semantics, project catalogs, sprint stages, or product state machines.

## 4. Design Principles

- Keep the domain model independent of CLI parsing.
- Keep runtime adapters independent of study semantics.
- Treat filesystem artifacts as durable product state.
- Validate outputs before marking tasks successful.
- Make state transitions explicit and testable.
- Prefer deterministic fixtures for tests.
- Do not require real OpenCode for normal unit tests.
- Use agentwrap for runtime execution, retries, validation wrapping, permission policy translation, health/preflight checks, observability, metadata, and OpenCode structured-output adaptation.
- Keep UltraPlan-specific study, source, report, project, and sprint behavior outside agentwrap adapters and wrappers.
- Avoid global mutable state.
- Avoid package cycles.
- Make dry-run behavior available before expensive operations.
- Keep generated Markdown and YAML readable by humans.

## 5. Workspace Requirements

### 5.1 Workspace Discovery

The CLI must resolve a workspace root using this precedence:

1. Explicit `--workspace <path>`.
2. `ULTRAPLAN_WORKSPACE` environment variable.
3. Current directory if it contains a workspace marker or required structure.
4. Nearest parent directory containing a workspace marker or required structure.

Workspace discovery must not traverse indefinitely. It must stop at filesystem root.

### 5.2 Workspace Structure

Required top-level structure:

```text
.
  ultraplan.yml
  prompts/
    base.md
    synthesize.md
  templates/
    repo-analysis.md
    report.md
  studies/
```

Optional structure:

```text
.
  .ultraplan/
    cache/
    logs/
    locks/
    tmp/
```

### 5.3 Path Handling

- All workspace-managed paths must be normalized.
- Commands must reject paths that escape the workspace when the command is expected to operate only inside the workspace.
- Source directories may be local paths inside a study.
- Absolute paths may be accepted in config only when explicitly allowed.
- Generated artifacts should prefer workspace-relative paths.

## 6. Configuration Requirements

### 6.1 Config File

Primary config file: `ultraplan.yml`.

Required fields:

```yaml
version: 1
runtime:
  default: opencode
models:
  default: provider/model
  primary: provider/model
  backup: provider/model
execution:
  default_variant: high
  default_parallel: 3
  default_timeout: 30m
  default_retries: 3
logging:
  format: text
  level: info
agentwrap:
  executable: opencode
  required_health:
    - runtime_available
    - structured_output
    - workdir
```

### 6.2 Config Precedence

Effective config must be resolved in this order:

1. Built-in defaults.
2. Workspace config.
3. Environment variables.
4. Command-specific flags.

The effective config object must record which values were defaulted and which were user-provided when relevant for diagnostics.

### 6.3 Secret Handling

- Secrets must not be required in `ultraplan.yml`.
- Runtime/provider secrets should be inherited from the environment or runtime-native config.
- `config show` must redact fields marked sensitive.
- Logs must not print full environment variables by default.

### 6.4 Config Validation

Config validation must check:

- Schema version.
- Required fields.
- Valid duration syntax.
- Positive parallelism, retries, and timeouts.
- Known runtime names.
- Known logging formats.
- Model values are non-empty when required.
- Agentwrap required health check names map to `agentwrap.HealthCheckID` values.
- Agentwrap executable path is non-empty when configured.

Validation failures must include field path and corrective guidance.

### 6.5 Agentwrap Configuration Mapping

UltraPlan config must map into agentwrap concepts instead of inventing parallel runtime concepts:

- Runtime executable maps to `opencode.WithExecutable`.
- Extra runtime args map to `opencode.WithExtraArgs`.
- Runtime environment additions map to `opencode.WithEnv`, with secrets redacted from diagnostics.
- Provider and model map to `agentwrap.RunRequest.Provider` and `agentwrap.RunRequest.Model`.
- Timeout maps to `agentwrap.RunRequest.Timeout`.
- Permission mode maps to `agentwrap.RunRequest.Permissions`.
- Structured permission policy maps to `agentwrap.RunRequest.PermissionPolicy`.
- Sandbox mode maps to `agentwrap.RunRequest.Sandbox`.
- Required health checks map to `agentwrap.RunRequest.RequireHealth`.
- Required capabilities map to `agentwrap.RunRequest.RequireCaps`.
- Output expectations and repair policy map to `agentwrap.RunRequest.Validation`.

UltraPlan should use agentwrap's `EffectiveConfig`, source-aware config layers, and redaction helpers where practical for runtime-facing configuration summaries.

## 7. CLI Requirements

### 7.1 CLI Behavior

- Every command must return a meaningful exit code.
- `--help` must be available globally and per command.
- `--json` must be supported for status and validation commands.
- `--dry-run` must be supported for expensive or mutating operations where useful.
- `--workspace` must be available globally.
- Commands must be script-friendly and avoid interactive prompts unless explicitly requested.

### 7.2 Exit Codes

Suggested exit code classes:

- `0`: Success.
- `1`: General failure.
- `2`: Usage or argument error.
- `3`: Config error.
- `4`: Workspace or filesystem error.
- `5`: Validation failure.
- `6`: Runtime failure.
- `7`: Cancellation.
- `8`: Partial completion.

### 7.3 Output Modes

Text mode:

- Default for humans.
- Shows progress and actionable summaries.

JSON mode:

- Stable structure for automation.
- No ANSI formatting.
- Includes command name, workspace, status, timestamps, and result payload.

## 8. Domain Model

### 8.1 Study

```go
type Study struct {
    Name        string
    Description string
    Root        string
    Sources     []Source
    Dimensions  []Dimension
}
```

Requirements:

- `Name` must be filesystem-safe.
- `Root` must be under `studies/` unless a custom output root is supplied.
- Sources and dimensions must be sorted deterministically for listing and task creation.

### 8.2 Source

```go
type SourceKind string

const (
    SourceKindDirectory SourceKind = "directory"
    SourceKindMarkdown  SourceKind = "markdown"
)

type Source struct {
    Name                 string
    Path                 string
    URL                  string
    Description          string
    Kind                 SourceKind
    ApplicableDimensions []string
    Frontmatter          map[string]any
}
```

Requirements:

- `Name` must be unique inside a study.
- Directory source `Path` must resolve to a directory for execution.
- Markdown source `Path` must resolve to a `.md` file directly under the study's `sources/` directory.
- Markdown source `ApplicableDimensions` must contain normalized two-digit dimension numbers.
- Empty `ApplicableDimensions` means the source applies to all dimensions.
- Source lookup by prefix is allowed only when unambiguous.
- Source discovery must distinguish directory sources from Markdown document sources.

### 8.3 Dimension

```go
type Dimension struct {
    Number      string
    Slug        string
    Title       string
    File        string
    Purpose     string
    Steps       []string
    Citations   []string
    Questions   []string
}
```

Requirements:

- `Number` must be zero-padded.
- `Slug` must be kebab-case.
- Dimension lookup by number, slug, full filename, or unambiguous prefix must be supported.

### 8.4 Report

```go
type Report struct {
    Kind       ReportKind
    Study      string
    Dimension  string
    Source     string
    Path       string
    Status     ValidationStatus
    Score      *float64
}
```

Report kinds:

- Per-source.
- Final synthesis.
- Code extraction bundle.
- Sprint reasoning.
- Sprint tracker.

### 8.5 Target

```go
type Target struct {
    Name       string
    Root       string
    PRDPath    string
    TRDPath    string
    BriefPath  string
    RoadmapPath string
    DecisionsPath string
    Sprints    []Sprint
}
```

### 8.6 Sprint

```go
type Sprint struct {
    Target       string
    Slug         string
    Root         string
    ReasoningPath string
    PlanPath     string
    Status       SprintStatus
}
```

## 9. Study Initialization

### 9.1 Input Schema

The study initialization YAML must support:

- Study name.
- Description.
- Repository/source target count.
- Repository/source items.
- Dimension target count.
- Dimension items.
- Optional source clone behavior.
- Optional output directory.

The parser must reject unknown required structures only when they prevent safe execution. It may ignore unknown extension fields with a warning if version-compatible.

### 9.2 Assisted Completion

If `repos.count` is greater than the number of explicit repo items, study initialization may request additional source suggestions from the configured runtime.

If `dimensions.count` is greater than the number of explicit dimension items, study initialization may request additional dimension suggestions from the configured runtime.

Runtime suggestions must use structured JSON outputs.

Suggested source schema:

```go
type SuggestedSource struct {
    Name        string `json:"name"`
    URL         string `json:"url"`
    Description string `json:"description"`
}
```

Suggested dimension schema:

```go
type SuggestedDimension struct {
    Number    string   `json:"number"`
    Slug      string   `json:"name"`
    Title     string   `json:"title"`
    Purpose   string   `json:"purpose"`
    Steps     []string `json:"steps"`
    Citations []string `json:"citations"`
    Questions []string `json:"questions"`
}
```

Requirements:

- Assisted completion must be disabled by `--no-assist`.
- Dry-run mode must report the shortage without invoking the runtime.
- Suggestions must be cached under `.ultraplan/cache/study-init/<study>/`.
- Suggestions must be validated before merge.
- Suggestions must be deduplicated by name and URL for sources.
- Suggestions must be deduplicated by number and slug for dimensions.
- Invalid suggestions must be ignored with warnings unless no valid suggestions remain.
- The normalized `study-init.yml` must include accepted suggestions.
- The cache must record runtime, model, prompt hash, created time, and raw response path.

### 9.3 Generated Files

Study initialization must create:

```text
studies/<study>/
  study-init.yml
  README.md
  dimensions/
    NN-slug.md
  sources/
  reports/
    source/
    final/
```

### 9.4 Source Cloning

When cloning is enabled:

- Use `git clone --depth 1`.
- Clone each source into `sources/<source-name>`.
- Skip existing source directories unless `--force` or a specific clean option is used.
- Record clone failures without hiding successful clones.
- Return partial-completion status when some clones fail.
- Optionally verify source URLs before cloning when `--verify-sources` is enabled.
- If URL verification fails and assisted completion is enabled, the CLI may request one replacement source for the same study purpose.
- Replacement sources must pass the same validation and deduplication as other suggestions.

### 9.5 Force Behavior

`--force` may overwrite generated study structure. It must not delete unrelated paths outside the selected study directory.

If destructive replacement is needed, implementation must clearly scope the deletion and should prefer moving old content to a backup or requiring a more explicit flag for deletion.

## 9A. Source Discovery and Applicability

### 9A.1 Source Discovery

Source discovery must inspect `studies/<study>/sources/` and return:

- One directory source for each non-hidden directory.
- One Markdown document source for each top-level `.md` file.

Source discovery must ignore:

- Hidden entries.
- Non-directory, non-`.md` files.
- Nested Markdown files inside directory sources, because those belong to the directory source's repository content.

### 9A.2 Markdown Frontmatter

Markdown document sources may begin with YAML frontmatter:

```markdown
---
applicable_dimensions:
  - 1
  - 3
  - 5
---
# My Guide
```

Required helper behavior:

- `parseFrontmatter(content string) (map[string]any, error)` parses leading `---` frontmatter only.
- `stripFrontmatter(content string) string` removes leading frontmatter and returns the document body.
- Files without leading frontmatter return an empty frontmatter map and the original content.
- Malformed frontmatter should produce a source validation warning or error according to command context.

`applicable_dimensions` rules:

- Accepted values may be numbers or strings.
- Values must normalize to two-digit dimension numbers.
- `1`, `"1"`, and `"01"` all match dimension `01`.
- Empty or absent `applicable_dimensions` means all dimensions are applicable.
- Invalid values must be reported with the source path and offending value.

### 9A.3 Applicability Filtering

The implementation must provide a helper equivalent to:

```go
func GetApplicableSources(sources []Source, dimension Dimension) []Source
```

Requirements:

- Directory sources are always applicable.
- Markdown sources with no applicability filter are applicable to all dimensions.
- Markdown sources with `ApplicableDimensions` are applicable only when the selected dimension number matches.
- Filtering must be used everywhere source/dimension task pairs are created, validated, summarized, or synthesized.

The following flows must respect applicability:

- Single analysis command.
- Full study run.
- Stateful run-loop initial state creation.
- Stateful run-loop resume validation.
- Completed source detection.
- Synthesis gating.
- Summary generation.
- Status calculations.
- Artifact validation for expected reports.

Inapplicable pairs must be skipped, not failed.

## 10. Prompt Composition

### 10.1 Prompt Inputs

Directory source analysis prompt inputs:

- Shared base prompt.
- Dimension Markdown.
- Report template.
- Study name.
- Source name and path.
- Output path.
- Hard rules for source isolation and citation format.

Markdown document source analysis prompt inputs:

- Shared base prompt, with code-exploration rules replaced or scoped for document analysis.
- Dimension Markdown.
- Report template.
- Study name.
- Source name and path.
- Stripped Markdown document content.
- Output path.
- Explicit instruction that all source material is embedded in the prompt.
- Explicit instruction not to access external files, repositories, or code.
- Explicit instruction that code citation requirements do not apply unless the dimension states otherwise.

Synthesis prompt inputs:

- Synthesis prompt.
- Dimension Markdown.
- Final report template.
- List of per-source report paths.
- Output path.

Sprint planning prompt inputs are deferred; current prompt composition covers analysis and synthesis only.

### 8.1 Study

```go
type Study struct {
    Name        string
    Description string
    Root        string
    Sources     []Source
    Dimensions  []Dimension
}
```

Requirements:

- `Name` must be filesystem-safe.
- `Root` must be under `studies/` unless a custom output root is supplied.
- Sources and dimensions must be sorted deterministically for listing and task creation.

### 8.2 Source

```go
type SourceKind string

const (
    SourceKindDirectory SourceKind = "directory"
    SourceKindMarkdown  SourceKind = "markdown"
)

type Source struct {
    Name                 string
    Path                 string
    URL                  string
    Description          string
    Kind                 SourceKind
    ApplicableDimensions []string
    Frontmatter          map[string]any
}
```

Requirements:

- `Name` must be unique inside a study.
- Directory source `Path` must resolve to a directory for execution.
- Markdown source `Path` must resolve to a `.md` file directly under the study's `sources/` directory.
- Markdown source `ApplicableDimensions` must contain normalized two-digit dimension numbers.
- Empty `ApplicableDimensions` means the source applies to all dimensions.
- Source lookup by prefix is allowed only when unambiguous.
- Source discovery must distinguish directory sources from Markdown document sources.

### 8.3 Dimension

```go
type Dimension struct {
    Number      string
    Slug        string
    Title       string
    File        string
    Purpose     string
    Steps       []string
    Citations   []string
    Questions   []string
}
```

Requirements:

- `Number` must be zero-padded.
- `Slug` must be kebab-case.
- Dimension lookup by number, slug, full filename, or unambiguous prefix must be supported.

### 8.4 Report

```go
type Report struct {
    Kind       ReportKind
    Study      string
    Dimension  string
    Source     string
    Path       string
    Status     ValidationStatus
    Score      *float64
}
```

Report kinds:

- Per-source.
- Final synthesis.
- Code extraction bundle.
- Sprint reasoning.
- Sprint tracker.

### 8.5 Target

```go
type Target struct {
    Name       string
    Root       string
    PRDPath    string
    TRDPath    string
    BriefPath  string
    RoadmapPath string
    DecisionsPath string
    Sprints    []Sprint
}
```

### 8.6 Sprint

```go
type Sprint struct {
    Target       string
    Slug         string
    Root         string
    ReasoningPath string
    PlanPath     string
    Status       SprintStatus
}
```

## 9. Study Initialization

### 9.1 Input Schema

The study initialization YAML must support:

- Study name.
- Description.
- Repository/source target count.
- Repository/source items.
- Dimension target count.
- Dimension items.
- Optional source clone behavior.
- Optional output directory.

The parser must reject unknown required structures only when they prevent safe execution. It may ignore unknown extension fields with a warning if version-compatible.

### 9.2 Assisted Completion

If `repos.count` is greater than the number of explicit repo items, study initialization may request additional source suggestions from the configured runtime.

If `dimensions.count` is greater than the number of explicit dimension items, study initialization may request additional dimension suggestions from the configured runtime.

Runtime suggestions must use structured JSON outputs.

Suggested source schema:

```go
type SuggestedSource struct {
    Name        string `json:"name"`
    URL         string `json:"url"`
    Description string `json:"description"`
}
```

Suggested dimension schema:

```go
type SuggestedDimension struct {
    Number    string   `json:"number"`
    Slug      string   `json:"name"`
    Title     string   `json:"title"`
    Purpose   string   `json:"purpose"`
    Steps     []string `json:"steps"`
    Citations []string `json:"citations"`
    Questions []string `json:"questions"`
}
```

Requirements:

- Assisted completion must be disabled by `--no-assist`.
- Dry-run mode must report the shortage without invoking the runtime.
- Suggestions must be cached under `.ultraplan/cache/study-init/<study>/`.
- Suggestions must be validated before merge.
- Suggestions must be deduplicated by name and URL for sources.
- Suggestions must be deduplicated by number and slug for dimensions.
- Invalid suggestions must be ignored with warnings unless no valid suggestions remain.
- The normalized `study-init.yml` must include accepted suggestions.
- The cache must record runtime, model, prompt hash, created time, and raw response path.

### 9.3 Generated Files

Study initialization must create:

```text
studies/<study>/
  study-init.yml
  README.md
  dimensions/
    NN-slug.md
  sources/
  reports/
    source/
    final/
```

### 9.4 Source Cloning

When cloning is enabled:

- Use `git clone --depth 1`.
- Clone each source into `sources/<source-name>`.
- Skip existing source directories unless `--force` or a specific clean option is used.
- Record clone failures without hiding successful clones.
- Return partial-completion status when some clones fail.
- Optionally verify source URLs before cloning when `--verify-sources` is enabled.
- If URL verification fails and assisted completion is enabled, the CLI may request one replacement source for the same study purpose.
- Replacement sources must pass the same validation and deduplication as other suggestions.

### 9.5 Force Behavior

`--force` may overwrite generated study structure. It must not delete unrelated paths outside the selected study directory.

If destructive replacement is needed, implementation must clearly scope the deletion and should prefer moving old content to a backup or requiring a more explicit flag for deletion.

## 9A. Source Discovery and Applicability

### 9A.1 Source Discovery

Source discovery must inspect `studies/<study>/sources/` and return:

- One directory source for each non-hidden directory.
- One Markdown document source for each top-level `.md` file.

Source discovery must ignore:

- Hidden entries.
- Non-directory, non-`.md` files.
- Nested Markdown files inside directory sources, because those belong to the directory source's repository content.

### 9A.2 Markdown Frontmatter

Markdown document sources may begin with YAML frontmatter:

```markdown
---
applicable_dimensions:
  - 1
  - 3
  - 5
---
# My Guide
```

Required helper behavior:

- `parseFrontmatter(content string) (map[string]any, error)` parses leading `---` frontmatter only.
- `stripFrontmatter(content string) string` removes leading frontmatter and returns the document body.
- Files without leading frontmatter return an empty frontmatter map and the original content.
- Malformed frontmatter should produce a source validation warning or error according to command context.

`applicable_dimensions` rules:

- Accepted values may be numbers or strings.
- Values must normalize to two-digit dimension numbers.
- `1`, `"1"`, and `"01"` all match dimension `01`.
- Empty or absent `applicable_dimensions` means all dimensions are applicable.
- Invalid values must be reported with the source path and offending value.

### 9A.3 Applicability Filtering

The implementation must provide a helper equivalent to:

```go
func GetApplicableSources(sources []Source, dimension Dimension) []Source
```

Requirements:

- Directory sources are always applicable.
- Markdown sources with no applicability filter are applicable to all dimensions.
- Markdown sources with `ApplicableDimensions` are applicable only when the selected dimension number matches.
- Filtering must be used everywhere source/dimension task pairs are created, validated, summarized, or synthesized.

The following flows must respect applicability:

- Single analysis command.
- Full study run.
- Stateful run-loop initial state creation.
- Stateful run-loop resume validation.
- Completed source detection.
- Synthesis gating.
- Summary generation.
- Status calculations.
- Artifact validation for expected reports.

Inapplicable pairs must be skipped, not failed.

## 10. Prompt Composition

### 10.1 Prompt Inputs

Directory source analysis prompt inputs:

- Shared base prompt.
- Dimension Markdown.
- Report template.
- Study name.
- Source name and path.
- Output path.
- Hard rules for source isolation and citation format.

Markdown document source analysis prompt inputs:

- Shared base prompt, with code-exploration rules replaced or scoped for document analysis.
- Dimension Markdown.
- Report template.
- Study name.
- Source name and path.
- Stripped Markdown document content.
- Output path.
- Explicit instruction that all source material is embedded in the prompt.
- Explicit instruction not to access external files, repositories, or code.
- Explicit instruction that code citation requirements do not apply unless the dimension states otherwise.

Synthesis prompt inputs:

- Synthesis prompt.
- Dimension Markdown.
- Final report template.
- List of per-source report paths.
- Output path.

Sprint planning prompt inputs are deferred; current prompt composition covers analysis and synthesis only.

### 10.2 Prompt Builder Requirements

- Prompt builders must be deterministic.
- Prompt builders must be unit-tested with golden fixtures.
- Prompt builders must return both prompt text and input manifest.
- Dry-run mode must expose the input manifest.
- Missing required prompt files must fail before runtime execution.
- Directory source prompts must preserve source-isolation and file-line citation rules.
- Markdown document source prompts must embed stripped document content and use document-analysis instructions.
- Prompt builders must not embed YAML frontmatter from Markdown document sources.

## 11. Runtime Adapter Requirements

### 11.1 Agentwrap Dependency

UltraPlan Go must use `github.com/Antonio7098/agentwrap` as the runtime SDK and `github.com/Antonio7098/agentwrap/opencode` as the first concrete runtime adapter.

UltraPlan must not define a competing public runtime contract. Product code may define thin internal interfaces for testing and dependency injection, but the implementation boundary to agentic runtimes must be agentwrap's root package API:

```go
type Runtime interface {
    StartRun(context.Context, RunRequest) (Run, error)
    Capabilities(context.Context) (Capabilities, error)
}
```

UltraPlan runtime integration must use:

- `agentwrap.Runtime`
- `agentwrap.Run`
- `agentwrap.RunRequest`
- `agentwrap.RunResult`
- `agentwrap.Capabilities`
- `agentwrap.SDKError`
- `agentwrap.HealthChecker` where the concrete runtime supports it
- `agentwrap.PolicyRunner`
- `agentwrap.ValidatingRuntime`
- `agentwrap.ObservingRuntime`
- `agentwrap.RunStore` or an UltraPlan implementation of that interface

### 11.2 Required Runtime Composition

The default production composition must be:

```text
UltraPlan task runner
  -> agentwrap.ObservingRuntime
  -> agentwrap.ValidatingRuntime
  -> agentwrap.PolicyRunner
  -> opencode.Runtime
```

Rationale and requirements:

- `ObservingRuntime` should wrap the logical run so UltraPlan can inspect active/completed runs, ordered events, final merged metadata, sink failures, and store-backed records.
- `ValidatingRuntime` must be used so runtime exit success is insufficient without UltraPlan artifact validation.
- `PolicyRunner` must be used for bounded retry, wait, rate-limit handling, and fallback decisions.
- `opencode.Runtime` must remain the adapter that owns OpenCode CLI invocation and native event projection.
- UltraPlan may adjust wrapper order only for an explicit reason, such as allowing validation failures to participate in policy fallback for a specific task type.

### 11.3 Run Request Mapping

For each UltraPlan task, build an `agentwrap.RunRequest`:

- `Prompt`: composed UltraPlan prompt.
- `WorkDir`: source directory, study root, or workspace root according to task type.
- `Provider` and `Model`: resolved model configuration.
- `Timeout`: task timeout.
- `Metadata`: study, dimension, source, source kind, output path, task kind, and UltraPlan task ID.
- `RequireHealth`: configured health checks, at least runtime availability, structured output, and working directory for OpenCode-backed tasks.
- `RequireCaps`: structured events and cancellation for normal tasks; permissions, artifacts, usage, or validation events when task policy requires them.
- `PermissionPolicy`: task-specific agent permissions.
- `Sandbox`: configured sandbox mode.
- `WantSession`, `SessionID`, and `SessionAction`: only when UltraPlan intentionally continues or repairs a related run.
- `Validation`: agentwrap validation expectations and custom validators for required UltraPlan artifacts.

### 11.4 OpenCode Adapter Usage

UltraPlan must use `opencode.NewRuntime` and options documented by agentwrap:

- `opencode.WithExecutable`
- `opencode.WithExtraArgs`
- `opencode.WithEnv`
- `opencode.WithStderrLimit`

UltraPlan must rely on the adapter to:

- Launch `opencode run --format json`.
- Pass `--dir`, `--model`, `--session`, extra args, and prompt.
- Decode newline-delimited JSON records.
- Project native OpenCode records into `agentwrap.Event` values.
- Attach native raw payloads as unsafe diagnostics by default.
- Classify malformed output, runtime exit, timeout, cancellation, rate limits, permission issues, and health failures into `agentwrap.SDKError`.
- Translate supported `agentwrap.PermissionPolicy` fields into `OPENCODE_CONFIG_CONTENT`.
- Enforce required health checks before process launch.
- Report best-effort session continuation metadata for supported session actions.

UltraPlan must not parse OpenCode stdout/stderr directly except through agentwrap diagnostics already exposed in `RunResult`, `SDKError`, events, or metadata.

## 12. Canonical Events

### 12.1 Event Envelope

UltraPlan must consume agentwrap canonical events. The source event shape is:

```go
type Event struct {
    ID        agentwrap.EventID
    RunID     agentwrap.RunID
    SessionID agentwrap.SessionID
    Time      time.Time
    Type      string
    Payload   agentwrap.EventPayload
    Raw       *agentwrap.RawPayload
}
```

`Event.Type` preserves the native or adapter-defined event name. UltraPlan should use `Event.Kind()` or `Payload["event_kind"]` for canonical classification.

### 12.2 Required Event Kinds

UltraPlan must handle agentwrap's current event kinds:

- `lifecycle`
- `session`
- `message`
- `progress`
- `tool`
- `artifact`
- `permission`
- `blocking`
- `usage`
- `warning`
- `fatal_error`
- `rate_limit`
- `validation`
- `retry`
- `fallback`
- `final_result`
- `native_extension`

UltraPlan task state and logs should map these event kinds into product-facing status, diagnostics, and run history without losing the original event kind.

### 12.3 Event Compatibility

- Unknown native events projected as `native_extension` must not fail tasks by themselves.
- Unsafe raw native payload bytes must not be persisted by UltraPlan unless an explicit debug retention option is enabled.
- Persisted event records should preserve raw payload presence, source, encoding, safety, and omission reason when bytes are omitted.
- UltraPlan should implement or configure an agentwrap `EventSink` and `RunStore` for durable local inspection.

### 12.4 Agentwrap Metadata

UltraPlan must consume and persist relevant `agentwrap.RunMetadata` fields:

- Runtime context.
- Parent run ID.
- Attempts.
- Policy decisions.
- Status and timing.
- Session metadata.
- Permission metadata.
- Cleanup metadata.
- Validation and repair metadata.
- Artifact references.
- Warnings and errors.
- Usage, estimated cost, and throughput where available.
- Native metadata safe for persistence.

Unknown usage token values must remain unknown. UltraPlan must not convert unknown usage to zero.

## 13. Scheduler and Run State

### 13.1 Task State

```go
type TaskState struct {
    ID              string
    Kind            TaskKind
    Study           string
    DimensionNumber string
    DimensionSlug   string
    Source          string
    SourceKind      SourceKind
    OutputPath      string
    Status          TaskStatus
    Attempts        int
    LastError       *UltraPlanError
    LastAttemptAt   *time.Time
    NextRetryAt     *time.Time
    StartedAt       *time.Time
    CompletedAt     *time.Time
    Validation      *ValidationResult
    AgentRunID      agentwrap.RunID
    SessionID       agentwrap.SessionID
    TurnID          agentwrap.TurnID
}
```

Task kinds:

- Analysis.
- Synthesis.
- Sprint execution.

### 13.2 Run State File

Each stateful study run must persist to:

```text
studies/<study>/.ultraplan/run-state.json
```

State file fields:

- Schema version.
- Run ID.
- Created time.
- Updated time.
- Batch size.
- Filters.
- Config summary.
- Task list.
- Agentwrap run metadata summary for each started task.
- Agentwrap validation, policy, permission, session, cleanup, artifact, and usage summaries where available.
- Completion flag.

### 13.3 Atomic Writes

State writes must be atomic:

1. Write to temporary file in same directory.
2. Flush and close.
3. Rename over previous state file.

If atomic rename is unavailable on a platform, the implementation must use the safest available strategy and document residual risk.

### 13.4 Resume Logic

On resume:

- Load state file.
- Validate schema version.
- Reset stale running tasks to pending or failed according to policy.
- Revalidate completed task outputs.
- Queue missing synthesis tasks when all inputs are valid.
- Exclude inapplicable Markdown source/dimension pairs from required input checks.
- Reconcile task state with agentwrap completed run records when an `agentwrap.RunStore` is available.
- Preserve attempt history.
- Continue respecting filters from the original run unless user explicitly overrides them.

### 13.5 Scheduling

- Use bounded worker pools.
- Never spawn more runtime processes than configured.
- Build the task matrix from applicable source/dimension pairs only.
- Prefer synthesis tasks when they become unblocked, but do not starve analysis tasks.
- Stop scheduling new work on cancellation.
- Save state after each meaningful transition.
- Drain `agentwrap.Run.Events()` while waiting so runtime event channels cannot block.
- Always call `Run.Wait` to obtain final `agentwrap.RunResult` and classified terminal errors.

## 14. Retry, Fallback, and Backoff

### 14.1 Agentwrap Policy Requirements

UltraPlan must use `agentwrap.PolicyRunner` and `agentwrap.BasicPolicy` for default retry, wait, rate-limit, and fallback behavior.

Requirements:

- Configure `MaxAttemptsPerTarget` from UltraPlan execution config.
- Configure `RetryRateLimits` according to workspace policy.
- Use agentwrap backoff implementations such as `agentwrap.ExponentialBackoff` or a compatible configured policy.
- Configure fallback alternatives as `agentwrap.FallbackAlternative` values.
- Preserve original prompt, workdir, timeout, permission policy, sandbox, metadata, and validation expectations when constructing fallback requests unless a specific fallback intentionally overrides them.
- Record policy decisions from `RunMetadata.Policy.Decisions` in UltraPlan task state.
- Surface agentwrap `rate_limit`, `retry`, and `fallback` events in status/log output.
- Do not implement retry/fallback logic inside the OpenCode adapter.

### 14.2 Error Classifications

UltraPlan product errors may include workspace, filesystem, source, dimension, and ambiguity categories. Runtime-facing errors must use or wrap `agentwrap.SDKError` categories.

Agentwrap categories that UltraPlan must handle:

- `configuration`
- `health`
- `runtime_unavailable`
- `provider_unavailable`
- `model_unavailable`
- `authentication`
- `permission`
- `rate_limit`
- `timeout`
- `cancellation`
- `malformed_event`
- `runtime_exit`
- `validation`
- `repair_exhausted`
- `cleanup`
- `unknown`

UltraPlan-specific categories:

- Usage.
- Workspace.
- Filesystem.
- SourceNotFound.
- DimensionNotFound.
- AmbiguousReference.
- MissingOutput.

### 14.3 Error Attributes

UltraPlan must inspect runtime errors with `errors.As` or `agentwrap.ErrorAs`, not by parsing error strings.

For runtime errors, persist or display safe `agentwrap.SDKError` fields:

- Category.
- Operation.
- UserDetail.
- DebugDetail when debug output is enabled.
- Provider.
- Model.
- RuntimeKind.
- ExitCode.
- Signal.
- NativeType.
- RetryAfter.
- Redacted metadata.

### 14.4 Backoff State

Backoff state must be derived from agentwrap policy attempts and persisted in UltraPlan task state:

- Attempt count.
- Target index.
- Attempt number on current task.
- Last classified error.
- Next retry time when known.
- Fallback model/runtime when selected.

### 14.5 Fallback Policy

Fallback may support:

- Backup model through an agentwrap fallback request.
- Backup provider through an agentwrap fallback request.
- Backup runtime in future versions through a different `agentwrap.Runtime`.

Fallback decisions must be visible in task state, agentwrap metadata, and events.
## 15. Validation Requirements

### 15.1 Agentwrap Validation

UltraPlan must use `agentwrap.ValidatingRuntime` and `agentwrap.ValidationSpec` for runtime output validation.

Agentwrap expectation kinds available to UltraPlan:

- `file`
- `directory`
- `artifact`
- `markdown_template`
- `json`
- `metadata`
- `custom`

UltraPlan-specific validation rules must be implemented as `agentwrap.Validator` or `agentwrap.ValidatorFunc` checks where they validate runtime-produced artifacts.

### 15.2 Validation Result Mapping

UltraPlan task state must map from `agentwrap.ValidationResult`:

- `Passed`
- `Skipped`
- `PassedCount`
- `FailedCount`
- `SkippedCount`
- `Checks`
- `Failures`
- `Errors`
- safe native details

Required validation failures must make the logical run fail through agentwrap. Optional validation failures may remain warnings.

### 15.3 Report Validators

Per-source report validator checks:

- File exists.
- File is non-empty.
- Heading exists.
- Source info section exists.
- Summary section exists.
- Rating section exists.
- Rating can be parsed.
- Required question/answer section exists.
- Citation shape appears where required.

Directory source report validation:

- Must enforce code citation shape unless the dimension explicitly disables that requirement.
- Code citations should use file paths and line numbers.

Markdown document source report validation:

- Must not require code citations by default.
- Must still require a rating, summary, and answers to dimension questions.
- May validate document-specific citations or section references if a dimension explicitly requires them.
- Must record the source kind in validation diagnostics.

Final report validator checks:

- File exists.
- File is non-empty.
- Study parameters exist.
- Sources studied table exists.
- Executive summary exists.
- Rating summary exists.
- Pattern or synthesis sections exist.
- Open questions or notable absences section exists.

Sprint reasoning validator checks:

- File exists.
- Requirement mapping exists.
- Decisions and tradeoffs exist.
- Risks and assumptions exist.
- Exit criteria exist.

Sprint tracker validator checks:

- File exists.
- Sprint overview exists.
- Scope and non-scope exist.
- Execution checklist exists.
- Testing checklist exists.
- Risks and blockers exist.
- Success criteria exist.
- Execution notes exist.

### 15.4 Validation Failure Behavior

Validation failure must:

- Mark the task failed or repairing according to agentwrap validation and repair policy.
- Include failed check names.
- Include expected vs observed details where safe.
- Include path to invalid artifact.
- Be visible in status output.
- Be recorded in `RunMetadata.Validation`.
- Emit agentwrap validation events.

### 15.5 Repair Requirements

UltraPlan may enable bounded repair through `agentwrap.RepairConfig`.

Requirements:

- Repair attempts must be bounded by `MaxAttempts`.
- Repair prompts must be built from safe expected/observed facts and artifact references, not large raw report content by default.
- Repair should use `SessionActionContinue` when same-session repair is useful and supported.
- Repair must inherit original workdir, provider/model, sandbox, permission mode, and permission policy unless UltraPlan explicitly overrides them.
- Exhausted repair must surface `agentwrap.ErrorRepairExhausted`.
- Permission denial during repair remains `agentwrap.ErrorPermission`.

## 16. Code Reference Extraction

### 16.1 Citation Syntax

Supported citation forms:

- `` `path/to/file.go:42` ``
- `` `path/to/file.go:42-58` ``
- `` `path/to/file.go:42,47,53` ``

The extractor should also tolerate an en dash in old reports but should emit normalized output.

### 16.2 Source Table Parsing

Reports must contain a sources table with source names and paths. The extractor must parse rows shaped like:

```markdown
| 1 | source-name | `sources/source-name` |
```

The parser must support workspace-relative and report-relative source paths.

### 16.3 Resolution Algorithm

For each citation:

1. Try path relative to each source root.
2. If citation includes a source-prefixed path, try stripping first path segment.
3. If unresolved, search by basename within source roots, excluding `.git`, `node_modules`, and known ignored directories.
4. Record unresolved reference if no match exists.

### 16.4 Output

Output must include:

- Source name.
- File path.
- Requested line spec.
- Resolved absolute or workspace-relative path.
- Rendered code with line numbers.
- Unresolved reference summary.

JSON output must expose structured refs and resolution status.

## 17. Summary Generation

Summary generator must:

- Discover dimensions in deterministic order.
- Discover sources in deterministic order.
- Parse ratings from per-source reports.
- Write `summary.csv`.
- Include columns for source, each dimension, and total.
- Sort by total descending.
- Represent missing ratings as empty cells.
- Represent inapplicable Markdown source/dimension pairs distinctly from missing expected reports, either as an empty non-error cell or a documented sentinel value.
- Exclude inapplicable pairs from missing-report warnings.

Rating parser must handle:

- `**8 / 10**`
- `8/10`
- `Rating: 8`

Ambiguous ratings should create warnings, not invented values.

## 18. Deferred Target and Sprint Technical Requirements

The TypeScript prototype includes target scaffolding, sprint planning, and sprint execution. UltraPlan Go does not implement those workflows in the current study-side build. No target/sprint commands, packages, validators, persisted schemas, runtime task kinds, or acceptance criteria are required now.

## 19. Logging and Diagnostics

UltraPlan logging and diagnostics must consume agentwrap observability records rather than directly inspecting native runtime process streams.

### 19.1 Logger Requirements

- Context-aware logging.
- Text and JSON output.
- Log levels: debug, info, warn, error.
- Redaction support.
- Stable fields for structured logs.

Structured log fields:

- Timestamp.
- Level.
- Command.
- Workspace.
- Study.
- Target.
- Sprint.
- Run ID.
- Task ID.
- Attempt.
- Runtime.
- Model.
- Event type.
- Message.
- Agentwrap run ID.
- Agentwrap session ID.
- Agentwrap event kind.
- Agentwrap policy decision when applicable.

### 19.2 Diagnostics

Diagnostics must include:

- Runtime executable path.
- Runtime version when available.
- Working directory.
- Timeout.
- Exit code.
- Safe stderr excerpt.
- Validation failures.
- State file path.
- Agentwrap `RunMetadata` summaries for attempts, policy, validation, repair, sessions, permissions, cleanup, artifacts, usage, warnings, and errors.
- Agentwrap raw payload omission/safety facts when relevant.

Diagnostics must not include:

- API keys.
- Full sensitive environment.
- Secret config values.

### 19.3 Agentwrap Stores and Sinks

UltraPlan must use agentwrap observability hooks:

- `agentwrap.ObservingRuntime` for active/completed run projections.
- `agentwrap.EventSink` for event fan-out to logs and local event records.
- `agentwrap.RunStore` for run inspection.

First release may use `agentwrap.MemoryRunStore` in tests and small local runs, but production local durability should use an UltraPlan store backed by workspace files if run records need to survive process exit.

Required sink failures must be treated according to agentwrap semantics: returned from `Wait` when the primary runtime outcome succeeded. Best-effort sink failures must be recorded as warnings without replacing the primary outcome.

## 20. Concurrency and Cancellation

### 20.1 Concurrency

- Use `context.Context` for cancellation.
- Use bounded goroutine pools for tasks.
- Avoid unbounded channels.
- Drain `agentwrap.Run.Events()` or deliberately discard through a draining goroutine until the channel closes.
- Ensure `agentwrap.Run.Wait()` is called for every started run.
- Let the agentwrap adapter own subprocess waiting and cleanup.
- Ensure no goroutine leaks in tests.

### 20.2 Cancellation

Cancellation sources:

- User interrupt.
- Context cancellation.
- Timeout.
- Runtime failure.

Cancellation behavior:

- Stop scheduling new tasks.
- Cancel active task contexts and call `agentwrap.Run.Cancel`.
- Let the agentwrap adapter attempt runtime process termination and cleanup.
- Save state.
- Mark active tasks cancelled or retryable based on policy.
- Return cancellation exit code when user initiated.
- Preserve agentwrap cleanup metadata separately from the primary run result.

## 21. Persistence and File Writes

### 21.1 File Write Policy

- Generated files must be written with explicit paths.
- State files must be atomic.
- Report files written by runtimes are validated after runtime exit.
- UltraPlan-created helper files should use `0644` permissions by default.
- Directories should use `0755` by default.

### 21.2 Locks

The first release should prevent accidental concurrent mutation of the same study run state.

Lock requirements:

- Per-study run lock for `run-loop`.
- Lock file includes PID, command, and timestamp.
- Stale lock detection should be conservative.
- Users can force unlock with explicit command or flag.

## 22. Security Requirements

- Redact secrets from logs and config output.
- Use agentwrap redaction helpers and `SDKError` safe fields for runtime diagnostics where practical.
- Validate workspace paths.
- Avoid shell interpolation for runtime commands where possible.
- UltraPlan product code must not invoke OpenCode directly; `agentwrap/opencode` owns `exec.CommandContext` process execution.
- Treat source repository content as untrusted input.
- Do not execute commands from source repositories unless explicitly configured.
- Disable post-run hooks by default.
- Require explicit opt-in for commands that mutate Git state.
- Runtime permission posture must be expressed through `agentwrap.PermissionPolicy` where possible.
- Unsupported required permission policy features must fail before runtime launch unless explicitly configured as best-effort.

### 22.1 Permission Policy Requirements

UltraPlan must support task-level permission policies using agentwrap:

- Read-only study analysis should default to allowing read/search/list-like tools and denying or asking for edit/shell behavior unless a command explicitly needs mutation.
- Study-side commands must not edit source repositories unless explicitly configured for generated artifacts.
- Shell behavior should default to ask or deny unless explicitly enabled.
- Permission policy metadata must be persisted from `RunMetadata.Permissions`.
- Permission events must appear in logs/status where relevant.
- Path-level permission rules must respect agentwrap OpenCode adapter limitations: unsupported required path rules fail before launch unless best-effort behavior is configured.

## 23. Testing Requirements

### 23.1 Unit Tests

Required unit coverage:

- Workspace discovery.
- Path normalization.
- Config loading and precedence.
- Study YAML parsing.
- Dimension generation.
- Source and dimension resolution.
- Markdown source discovery.
- Markdown frontmatter parsing.
- Markdown frontmatter stripping.
- Applicable dimension normalization and filtering.
- Prompt composition.
- State transitions.
- Backoff calculation.
- Error classification and mapping from `agentwrap.SDKError`.
- Report validation.
- Rating parsing.
- Code citation parsing.
- Code reference resolution.
- Summary generation.
- Target initialization.
- Sprint file validation.
- Agentwrap `RunRequest` construction for analysis and synthesis tasks. Sprint planning and execution tasks are deferred.
- Agentwrap wrapper composition.
- Permission policy construction.

### 23.2 Fixture Tests

Fixtures required:

- Minimal valid study.
- Study with missing source.
- Study with ambiguous dimension reference.
- Study with directory sources and Markdown document sources.
- Markdown document source with `applicable_dimensions`.
- Markdown document source with malformed frontmatter.
- Valid per-source report.
- Invalid per-source report.
- Valid final report.
- Invalid final report.
- Code citations with single lines, ranges, lists, missing files, and source-prefixed paths.
- OpenCode structured event stream.
- Malformed runtime event stream.
- Rate limit event or diagnostic.
- Timeout fixture through fake runtime.
- Agentwrap event records for lifecycle, validation, retry, fallback, permission, and cleanup.

### 23.3 Fake Runtime Tests

Fake runtime tests should use agentwrap-compatible fakes. If the public agentwrap package does not expose a reusable fake, UltraPlan should define local test fakes that implement `agentwrap.Runtime` and `agentwrap.Run`.

Fake runtime must support:

- Successful run that writes expected artifact.
- Successful runtime exit without artifact.
- Runtime failure.
- Timeout.
- Cancellation.
- Malformed events.
- Rate limit.
- Delayed events for concurrency tests.
- Validation failure and repair paths through `agentwrap.ValidatingRuntime`.
- Retry and fallback paths through `agentwrap.PolicyRunner`.
- Observability records through `agentwrap.ObservingRuntime`.

### 23.4 Integration Tests

Gated integration tests may require:

- OpenCode installed.
- Provider/model configured.
- Network access if runtime requires it.

These tests must be opt-in and skipped by default unless required environment variables are present.

OpenCode integration tests must go through `agentwrap/opencode`, not direct process invocation.

### 23.5 Golden Tests

Golden tests should cover:

- Generated dimension Markdown.
- Generated study README.
- Directory source prompt composition.
- Markdown document source prompt composition with stripped frontmatter.
- Summary CSV.
- Code extraction output.
- Target starter files.

Golden updates must be explicit.

## 24. Build and Release Requirements

### 24.1 Build

- Go module builds with `go build ./...`.
- Tests run with `go test ./...`.
- CLI binary is produced by `go build ./cmd/ultraplan`.
- Go module must depend on `github.com/Antonio7098/agentwrap`.
- Local development may use a `replace` directive or workspace file to point at the sibling `agentwrap` checkout.
- Release builds must pin an explicit agentwrap version or commit.
- Agentwrap documentation under `agentwrap/docs` is the canonical reference for runtime integration behavior.

### 24.2 Version Command

CLI must support:

```bash
ultraplan version
```

Version output:

- Version.
- Commit SHA when available.
- Build date when available.
- Go version.

### 24.3 Distribution

First release should support:

- Source build.
- Linux binary.
- macOS binary.

Release artifacts should include checksums.

## 25. Migration From Prototype

The Go implementation must preserve the important prototype workflows:

- Study listing.
- Study initialization.
- Single dimension/source run.
- Full study run.
- Stateful run-loop.
- Status.
- Synthesis.
- Summary.
- Code extraction.
- Sprint execution.
- Markdown document sources with `applicable_dimensions`.

The Go implementation may intentionally change:

- Command grouping for clarity.
- Config file format.
- Internal state schema.
- Runtime adapter implementation.
- Validation strictness.
- Logging format.

The Go implementation must intentionally change runtime execution from prototype-owned OpenCode process management to agentwrap-owned runtime supervision.

Migration support should include:

- Ability to read existing study directory layouts where practical.
- Validation command that reports incompatible or missing files.
- Documentation for converting prototype config into `ultraplan.yml`.

## 26. Acceptance Criteria

UltraPlan Go is technically acceptable when:

- `go test ./...` passes.
- CLI builds as a single binary.
- `ultraplan --help` and command help are complete.
- Workspace init and validation work.
- Study init from YAML creates expected structure.
- Source discovery finds directories and top-level Markdown document sources.
- Markdown source `applicable_dimensions` filters source/dimension task creation and validation.
- Study list commands work.
- Single fake-runtime analysis writes and validates a report.
- Runtime execution uses `github.com/Antonio7098/agentwrap` and the `agentwrap/opencode` adapter.
- Default runtime composition includes `ObservingRuntime`, `ValidatingRuntime`, and `PolicyRunner`.
- Fake-runtime run-loop resumes after interruption.
- Synthesis task runs only after valid per-source reports exist.
- Summary generation is deterministic.
- Code extraction resolves valid citations and reports unresolved ones.
- OpenCode adapter passes gated smoke tests.
- Runtime health gates use agentwrap health checks.
- Runtime validation uses agentwrap validation expectations and custom validators.
- Runtime retry/fallback uses agentwrap policy metadata and events.
- Runtime permission policies use agentwrap permission metadata and OpenCode translation.
- Target init creates required files.
- Sprint execution loads required files and updates tracker validation.
- Logs redact secrets.
- State writes are atomic.
- Cancellation saves state.
- Documentation explains normal workflows and recovery workflows.

## 27. Open Technical Questions

- Should the workspace marker be `ultraplan.yml`, `.ultraplan/`, or both?
- Should report validators be configurable per study, or fixed for the first release?
- Should retries be per task only, or should there also be run-level retry budgets?
- How should UltraPlan persist agentwrap `RunStore` records durably across process exits?
- Which agentwrap wrapper order should be used when validation failures should be eligible for fallback?
- Should lock files be mandatory for all mutating commands or only long-running run loops?
- Should code extraction support non-local source paths in the first release?
- Should generated report templates be versioned independently from the CLI binary?
- What is the minimum stable JSON schema for status output?

## 28. Changelog

| Version | Date | Change | Rationale |
|---------|------|--------|-----------|
| 1.0.0 | 2026-05-25 | Initial TRD | Define production technical requirements for UltraPlan Go. |
| 1.1.0 | 2026-05-25 | Ground runtime supervision in agentwrap | Require UltraPlan Go to use agentwrap runtime contracts, wrappers, OpenCode adapter, validation, policy, health, observability, metadata, and permissions. |


## Current Scope Clarification

UltraPlan has two product sides: (1) studying source repositories/documents and producing validated research artifacts, and (2) applying those study findings to targets, roadmaps, sprint plans, and execution. This TRD covers only the study side for the current build. Any target, sprint, or implementation-execution details retained from the prototype are non-goals until a later requirements revision.
