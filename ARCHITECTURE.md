# UltraPlan Go Architecture

## Architecture Philosophy

UltraPlan Go is module-driven, not global-layer-driven.

The guiding question is not:

> What technical category does this file belong to?

It is:

> Which module owns this behavior and state?

A product module should encapsulate a complete slice of behavior:

```text
module = state + logic + workflows + validation + persistence adapters + CLI/API surface
```

This means study behavior should stay with the study module. Code extraction behavior should stay with the code extraction module. Workspace behavior should stay with the workspace module. Shared platform packages should exist only for genuinely cross-cutting infrastructure.

## Core Rule

Prefer this:

```text
internal/study owns its validation, scheduling, reports, prompts, state, and persistence
internal/codeextract owns its parsing, resolution, extraction, and validation behavior
internal/workspace owns workspace discovery, path rules, and workspace validation
internal/platform/runtime owns generic execution only
```

Avoid this as the default shape:

```text
internal/validation
internal/scheduler
internal/reports
internal/prompts
internal/study
```

Global technical-layer packages look clean at first, but they tend to fracture product context and create cross-module coupling.

## Initial Layout

Use a pragmatic Go layout: one package per product module, split by focused files first, and introduce subpackages only when the module grows enough that the boundary improves comprehension.

```text
cmd/
  ultraplan/
    main.go

internal/
  app/
    app.go                  # composition root and dependency wiring

  platform/
    config/
    logging/
    filesystem/
    runtime/
      runtime.go            # generic execution interface
      agentwrap.go          # agentwrap integration
      opencode.go           # opencode-specific adapter, if needed

  workspace/
    domain.go
    discovery.go
    validation.go
    paths.go
    store.go

  study/
    domain.go               # Study, Source, Dimension, Report, RunState
    service.go              # use-case coordination
    init.go
    run.go
    run_all.go
    synthesize.go
    scheduler.go
    state.go
    prompts.go
    validation.go
    reports.go
    summary.go
    store_fs.go
    cli.go

  codeextract/
    domain.go
    service.go
    parser.go
    resolver.go
    validation.go
    cli.go
```

Do not immediately create a large clean-architecture tree such as:

```text
internal/study/domain/
internal/study/app/
internal/study/ports/
internal/study/adapters/
internal/study/validation/
internal/study/reports/
internal/study/prompts/
```

That can recreate the same abstraction problem as global layers. Start with one package per module and multiple focused files. Split into subpackages later when there is a concrete readability or dependency benefit.

## Module Ownership

### `study`

`study` is the main product module for the current build. It owns the full study lifecycle:

```text
Study definitions
Sources
Dimensions
Source applicability
Prompt creation
Single analysis runs
Full study runs
Run-loop state
Scheduling
Per-source report paths
Final synthesis report paths
Study validation
Report parsing
Summary generation
Filesystem persistence for study artifacts
CLI commands for study workflows
```

Prefer:

```text
internal/study/validation.go
internal/study/scheduler.go
internal/study/reports.go
internal/study/prompts.go
```

Instead of:

```text
internal/validation/study.go
internal/scheduler/study.go
internal/reports/study.go
internal/prompts/study.go
```

The study module may call platform runtime services, workspace path services, and config/logging infrastructure. Runtime and platform packages must not import `study`.

### `platform/runtime`

Runtime is platform-level because it is generic execution infrastructure, not study behavior.

It may understand:

```text
Prompt
Working directory
Model
Timeout
Environment
Permissions
Expected output path
Execution events
Execution result
```

It must not understand:

```text
Study
Dimension
Source
Synthesis gating
Report semantics
Study state machines
Summary generation
```

The dependency direction is:

```text
study -> platform/runtime
platform/runtime -> no product modules
```

Runtime supervision is delegated to `agentwrap`. UltraPlan's runtime integration should translate generic execution requests into agentwrap requests and translate generic results/events back to UltraPlan's platform-facing runtime model.

### `workspace`

`workspace` owns where UltraPlan is operating and how workspace paths are resolved.

It owns:

```text
Root discovery
Workspace marker/config lookup
Canonical workspace paths
Workspace-level validation
Path safety rules
```

It should not know:

```text
Which dimensions exist
How study synthesis works
How report summaries are generated
How code extraction parses citations
```

Boundary:

```text
workspace = where am I and where are things?
study = what study work happens here?
```

### `codeextract`

`codeextract` owns code-reference extraction as a distinct product capability.

It owns:

```text
Parsing report citations
Resolving referenced source files
Extracting source snippets
Producing extraction output
Validating extraction requests
CLI commands for extraction workflows
```

It may consume report paths or metadata produced by `study`, but it should not become a generic `reports` package unless the behavior is genuinely shared by multiple modules.

## Dependency Rules

Use these rules to keep module boundaries clear:

```text
Product modules may depend on platform packages.
Product modules may depend on workspace when they need workspace paths.
Product modules should not depend on other product modules unless there is a clear product relationship.
Platform packages must not depend on product modules.
Shared helpers must not become a dumping ground for product behavior.
Runtime must not import study.
```

Expected dependency direction for the current build:

```text
cmd/ultraplan -> internal/app
internal/app -> product modules + platform modules
study -> workspace
study -> platform/runtime
study -> platform/config/logging/filesystem as needed
codeextract -> workspace
codeextract -> platform/filesystem/logging as needed
workspace -> platform/filesystem as needed
platform/* -> no product modules
```

## Encapsulation in Practice

A module should expose a small use-case-oriented surface and hide internal helpers.

Example shape:

```go
type Service struct {
    store   Store
    runtime Runtime
    clock   Clock
}

func (s *Service) InitStudy(ctx context.Context, req InitStudyRequest) error
func (s *Service) RunDimension(ctx context.Context, req RunDimensionRequest) error
func (s *Service) RunAll(ctx context.Context, req RunAllRequest) error
func (s *Service) Synthesize(ctx context.Context, req SynthesizeRequest) error
```

Internally, the module can call helpers such as:

```go
validateStudy(...)
buildAnalysisPrompt(...)
resolveSources(...)
scheduleRuns(...)
parseReports(...)
writeRunState(...)
```

Callers should not need to know those helpers exist.

## Interface Guidance

Interfaces should appear at external or volatile boundaries, not everywhere by default.

Good interface boundaries:

```text
Runtime execution
Filesystem persistence where tests need fakes
Clock/time
External process execution
```

Avoid introducing interfaces for every internal helper. If a function is purely internal to a module and not volatile, keep it concrete.

## Final Principle

```text
Platform owns generic capabilities.
Modules own product behavior.
Logic stays near the state it transforms.
Interfaces appear only at external or volatile boundaries.
```
