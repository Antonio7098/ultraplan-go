# UltraPlan Filesystem-First Server, Persistence Boundary, SQLite Migration, and Cloud Plan

**A staged plan to extend the existing local-first product without prematurely changing its source of truth**  
**Version:** Planning artefact v1.3  
**Prepared:** 31 July 2026  
**Repository:** https://github.com/Antonio7098/ultraplan-go  
**Authoritative roadmap:** https://github.com/Antonio7098/ultraplan-workspace/blob/main/projects/ultraplan-go/roadmap.md

> **Recommended sequence:** Complete the already-planned local Go server and browser UI directly over the existing filesystem-backed application. Dogfood that interface. Then extract product-level persistence boundaries, keeping repository and execution workspaces filesystem-native. Add SQLite as a second implementation selected through the composition root, migrate existing artefacts explicitly, and only then decide whether filesystem authorship remains first-class, becomes an import/export workflow, or survives primarily as an execution projection and Git publication surface.

## 1. Executive summary

UltraPlan already has the correct immediate next step in its roadmap: Product Phase 4 adds a loopback-only Go HTTP server, browser UI, guarded operations, and SSE progress over the same shared application use cases and filesystem workspace used by the CLI and TUI.

That phase should be completed before introducing a database. It proves the server boundary, frontend workflows, typed application operations, progress streaming, cancellation, locking, security, and CLI/TUI/web parity without changing persistence semantics at the same time.

The broader evolution should be:

```text
Existing filesystem CLI/TUI
        |
        v
Filesystem-backed local server and browser UI
        |
        v
Real-use evaluation and product learning
        |
        v
Product-level persistence boundaries
        |
        +----------------------+
        |                      |
        v                      v
Filesystem adapters       SQLite adapters
(reference mode)          (database mode)
        \                      /
         \                    /
          +-> shared application use cases
                      |
                      v
        Filesystem execution projection
                      |
                      v
           OpenCode / later Aren
```

This sequence separates four questions:

1. **Is a server and browser UI useful?** Test this while preserving the filesystem source of truth.
2. **Where is persistence genuinely part of product semantics?** Learn this from the filesystem-backed server rather than guessing.
3. **Is database-backed artefact management better?** Test SQLite after the server interaction model and application use cases are proven.
4. **Does local filesystem authorship deserve to remain first-class?** Decide this only after using both modes.

The central architectural rule is:

> **Inject product persistence, not a pretend filesystem.**

UltraPlan should not abstract all file access behind a generic `ReadFile`/`WriteFile` interface and then force SQLite to emulate paths. It should define product-level repository contracts around projects, studies, sprints, artefacts, revisions, workflow transitions, and runs. Filesystem and SQLite adapters implement those contracts. Real source repositories and agent execution workspaces remain normal filesystems.

## 2. Alignment with the existing roadmap

The existing roadmap defines Product Phase 4 as:

```text
browser -> local HTTP/SSE adapter -> shared app use cases -> existing product modules
```

The server and browser are explicitly local interfaces over the existing filesystem-backed product. They do not introduce a database-backed alternate source of truth.

The existing sequence remains authoritative:

- **Sprint 30:** local web foundation and read-only dashboard.
- **Sprint 31:** guarded web operations and SSE progress.
- **Sprint 32:** local web hardening, documentation, and release.

This plan begins after, and builds on, that work. It does not move SQLite or a generic storage layer into Product Phase 4.

The Phase 4 constraint should be:

```text
HTTP handlers
    -> shared application queries and commands
    -> existing project, sprint, study, and runtime services
    -> current filesystem workspace
```

HTTP handlers must not:

- scrape CLI output;
- invoke `ultraplan` as a subprocess for ordinary product operations;
- parse Markdown directly;
- walk workspace directories independently;
- duplicate validation or transition rules;
- own filesystem-specific workflow logic.

This establishes the top half of the eventual boundary before persistence becomes replaceable.

## 3. Guiding principles

1. **One major architectural change at a time.** First add the interface; then change persistence.
2. **Preserve current behaviour before migrating it.** The filesystem-backed CLI, TUI, and server establish the reference semantics.
3. **Shared application use cases remain central.** CLI, TUI, browser, and later Aren tools call the same product operations.
4. **Inject product capabilities, not storage mechanics.** Repositories expose UltraPlan semantics rather than `ReadFile`, SQL, or transaction handles.
5. **Keep interfaces owned by product modules.** Project, sprint, study, and run packages define the persistence they require.
6. **Avoid a premature universal artefact abstraction.** Consolidate only after multiple modules prove the same lifecycle semantics.
7. **Keep repository discovery filesystem-native.** Source code, Git, builds, tests, and code search continue to use a real checkout.
8. **Keep execution workspaces filesystem-native.** OpenCode and shell tooling continue to operate in ordinary directories.
9. **Select persistence once at composition.** A server/workspace uses one authoritative product store at a time.
10. **Do not dual-write.** Migration, import, export, and later synchronization are explicit operations.
11. **Use semantic atomic operations.** Stage completion should commit artefact revisions, validation, and flow state as one product action.
12. **Make concurrency part of the contract.** Writes identify their expected base revision and reject stale updates.
13. **Use shared repository contract tests.** Filesystem and SQLite implementations must preserve the same observable semantics.
14. **Use real work as evidence.** The authority decision follows dogfooding, not architectural preference alone.
15. **Treat Aren as the long-term execution substrate, not a prerequisite.** The local server and SQLite work should remain useful when Aren arrives.

## 4. Three distinct storage boundaries

UltraPlan should distinguish three different concerns that must not be collapsed into one `Storage` interface.

### 4.1 Product persistence

This is the replaceable boundary:

```text
Filesystem now
SQLite later
Postgres in the cloud
```

It contains UltraPlan-owned product data:

- project and study identity;
- sprint identity and lifecycle;
- requirements, indexes, handbooks, reasoning, plans, reviews, smoke reports;
- study dimensions, source reports, final reports, and summaries where retained;
- artefact revisions and validation results;
- stage state and stage executions;
- run metadata and events;
- approvals, proposals, and publication state when introduced.

### 4.2 Repository workspace

This remains filesystem and Git based:

```text
/workspace/repo
```

It contains:

- source code;
- tests;
- dependency files;
- repository documentation;
- project configuration;
- Git history and working-tree state;
- code changes produced during execute work.

SQLite should not emulate this workspace. A narrow source-workspace capability may expose safe root resolution and Git revision metadata, but the underlying implementation remains a real checkout.

### 4.3 Execution workspace

This remains a temporary filesystem or sandbox:

```text
/tmp/ultraplan/runs/<run-id>/workspace
```

It is used for:

- OpenCode;
- AgentWrap;
- prompts and temporary context;
- database artefact projections;
- shell commands;
- source-code edits;
- builds and tests;
- recovery after interrupted runs.

In SQLite mode, this filesystem is a projection and mutable run environment, not the product source of truth.

## 5. Evolution of the architecture

### Stage A — Existing filesystem product

```text
CLI / TUI
    |
    v
shared app use cases
    |
    v
workspace, study, project, and sprint modules
    |
    v
filesystem workspace
```

The workspace Markdown and JSON files remain authoritative.

### Stage B — Filesystem-backed local server

```text
CLI command ----\
TUI action ------> shared app use cases -> product modules -> filesystem workspace
HTTP action -----/
                       ^
                       |
                 browser + SSE
```

The browser is another adapter. No database projection, import, capture, or dual-write logic is required.

### Stage C — Explicit persistence boundary

```text
CLI / TUI / HTTP
        |
        v
shared application use cases
        |
        v
project / sprint / study repositories
        |
        v
filesystem adapters
```

The filesystem remains authoritative, but product services stop depending directly on path layouts where a proven persistence seam exists.

### Stage D — SQLite-backed local server mode

```text
CLI / TUI / HTTP
        |
        v
shared application use cases
        |
        v
product persistence ports
      /   \
filesystem  SQLite
adapters    adapters
```

A workspace or server instance has one authority at a time. The first SQLite version does not continuously synchronize both stores.

### Stage E — Cloud and Aren

```text
Cloud UI / API
      |
      v
UltraPlan application services -> Postgres artefacts and workflow state
      |
      v
Aren run lifecycle -> sandboxed Git checkout
                         |
                         +-> repository discovery, code edits, tests

Agent -> typed UltraPlan artefact tools -> application services -> database
```

## 6. Phase A — Complete the planned filesystem-backed web product

This phase is the existing Product Phase 4 and should remain unchanged in purpose.

### A1. Read-only web foundation

Deliver the planned `ultraplan serve` loopback server and read-only dashboard over shared app queries.

Key outcomes:

- server lifecycle, address binding, graceful shutdown, and health are proven;
- templates and static assets are served from the Go product;
- projects, studies, sprints, status, validation, and bounded artefact previews can be inspected;
- routes return typed HTML or JSON errors correctly;
- path safety and script/HTML preview risks are controlled;
- no Node.js application server or database is required.

### A2. Guarded operations and SSE

Expose already-supported operations through shared application commands rather than subprocess invocation or CLI scraping.

Key outcomes:

- confirmation and stale-confirmation semantics are reusable across TUI and web;
- run progress is streamed truthfully through SSE;
- cancellation reaches the shared operation context;
- concurrent conflicting mutations are rejected through existing scope locks;
- browser state can reconnect to current durable filesystem-backed operation state;
- CLI, TUI, and browser produce equivalent workflow outcomes.

### A3. Application object and adapter wiring

The server should receive the same application-facing operations as the CLI and TUI.

A possible shape is:

```go
type Application struct {
    Projects ProjectUseCases
    Sprints  SprintUseCases
    Studies  StudyUseCases
    Runs     RunUseCases
}
```

The exact structure should follow existing package ownership. The important property is that CLI, TUI, and HTTP adapters do not construct their own product services or bypass application commands.

The current `internal/app` package already performs manual constructor-style dependency wiring for runtime and TUI concerns. Product composition should evolve through the same explicit Go approach rather than introducing a dependency-injection framework.

### A4. Hardening and release

Complete security, recovery, documentation, race testing, shutdown, redaction, and parity gates.

**Exit criteria for Phase A:**

- the browser is a supported local interface;
- the filesystem remains the sole product source of truth;
- every web mutation passes through typed shared application use cases;
- the server is sufficiently stable to use for real UltraPlan work;
- no database repository or speculative universal storage interface is required.

## 7. Phase B — Dogfood the filesystem-backed server

Use the released local server and frontend for real studies and governed sprint workflows before defining the SQLite product model.

### Questions to answer

- Which artefacts benefit from a richer editor rather than direct Markdown editing?
- Is revision history valuable beyond Git history?
- Which screens require cross-project or cross-sprint queries that are awkward on files?
- Which operations feel slow because the server repeatedly discovers and parses the workspace?
- Which workflow state is genuinely operational rather than portable project content?
- Do users want drafts that are not immediately represented as Git/filesystem changes?
- Are comments, approvals, proposals, and revision comparison important?
- Which reports are useful in the UI but undesirable in repository history?
- How often does the user leave the browser and edit files directly?
- Which groups of writes need to be atomic from the product's perspective?
- Which path-based operations are merely representation details, and which are meaningful user-facing identities?

### Evidence to collect

- representative navigation and operation traces;
- filesystem reads and writes per workflow;
- latency of project/study/sprint discovery;
- examples of desired autosave, draft, history, diff, and approval behaviour;
- incidents where local file editing is easier than the browser;
- incidents where filesystem state makes the browser experience awkward;
- data that should remain portable versus server-only;
- operations that currently coordinate multiple Markdown/JSON writes;
- stale-write or concurrent-run cases that need optimistic concurrency;
- areas where project, sprint, and study persistence genuinely share semantics.

**Exit criteria for Phase B:**

- at least one substantial study and one substantial planning/execute/review/smoke workflow have been managed through the browser;
- the desired database entities are grounded in observed workflows;
- a written storage classification exists for every managed artefact and state file;
- candidate persistence ports are tied to actual use cases;
- there is evidence that SQLite solves concrete product problems rather than merely changing technology.

## 8. Phase C — Design and extract the persistence boundary

The filesystem remains authoritative throughout this phase. The purpose is to define a clean replacement seam without changing observable behaviour.

### C1. Classify current data

Classify each current file or output as one of:

- **durable authored artefact:** requirements, handbooks, reasoning, plans, final reports;
- **portable workflow checkpoint:** validated stage completion or resumable study state;
- **derived output:** summaries, indexes, cached previews;
- **operational server state:** active requests, subscribers, confirmations, leases;
- **run evidence:** logs, diagnostics, transcripts, test output;
- **repository source state:** code, tests, configuration, Git history.

Only the first five categories are candidates for product persistence. Repository source state remains outside the database persistence boundary.

### C2. Introduce stable product identities only where required

Add stable concepts where database storage or concurrency genuinely requires them:

- Repository
- Project
- Study
- Sprint
- Artefact
- ArtefactKind
- ArtefactRevision
- StageExecution
- Run
- RunEvent

Do not replace useful human references and filesystem paths. Stable IDs supplement those representations.

### C3. Keep interfaces owned by product modules

Do not start with one giant `Storage`, `DataStore`, or universal `ArtifactRepository`.

Prefer package-owned ports such as:

```text
project.Repository
sprint.Repository
study.Repository
run.Repository
```

Each interface should be defined in the package that consumes it, using domain/application types rather than filesystem paths or SQL records.

Illustrative sprint persistence:

```go
type Repository interface {
    GetSprint(ctx context.Context, id SprintID) (Sprint, error)
    ListSprints(ctx context.Context, projectID ProjectID) ([]SprintSummary, error)

    GetArtifact(
        ctx context.Context,
        sprintID SprintID,
        kind ArtifactKind,
    ) (Artifact, error)

    SaveArtifactRevision(
        ctx context.Context,
        input SaveArtifactRevisionInput,
    ) (ArtifactRevision, error)

    GetFlowState(
        ctx context.Context,
        sprintID SprintID,
    ) (FlowState, error)

    CommitStageResult(
        ctx context.Context,
        result StageResult,
    ) error
}
```

Illustrative study persistence may have different operations because source/dimension task graphs, synthesis, and run-loop state are not identical to sprint artefact lifecycles.

A shared artefact subsystem should be extracted later only if project, sprint, and study modules prove that they need the same revision, validation, proposal, and publication behaviour.

### C4. Do not inject a generic virtual filesystem

Avoid:

```go
type Storage interface {
    ReadFile(path string) ([]byte, error)
    WriteFile(path string, content []byte) error
    Walk(root string) ([]Entry, error)
}
```

This would force SQLite to emulate the current path layout and preserve representation assumptions inside product services.

Generic filesystem mechanics may remain in `internal/platform/filesystem`:

- atomic replacement;
- path safety;
- directory creation;
- locking;
- hashing;
- temporary directories.

Those helpers should not know what a sprint plan or study report is.

### C5. Define semantic atomic operations

Database-backed writes will often need to update several records together:

```text
create artefact revision
-> attach validation result
-> mark stage execution complete
-> advance flow state
-> record run provenance
```

Do not expose a generic database transaction to product code.

Prefer a semantic operation:

```go
type StageResult struct {
    SprintID        SprintID
    Stage           Stage
    ExpectedVersion int64
    ArtifactChanges []ArtifactChange
    Validation      ValidationResult
    RunID           RunID
}

type Repository interface {
    CommitStageResult(
        ctx context.Context,
        result StageResult,
    ) error
}
```

The SQLite adapter implements one SQL transaction. The filesystem adapter implements the closest safe equivalent through validation, temporary files, atomic renames, lock ownership, and writing flow state last.

### C6. Make optimistic concurrency part of the contract

Writes should identify the current revision or version they were based on:

```go
type SaveArtifactRevisionInput struct {
    ArtifactID              ArtifactID
    ExpectedCurrentRevision RevisionID
    Content                 string
    ProducedByRun           *RunID
}
```

A stale write returns a stable conflict error instead of overwriting newer work.

The filesystem implementation may use revision manifests or normalized content hashes. SQLite may use guarded updates such as:

```sql
UPDATE artifacts
SET current_revision_id = ?
WHERE id = ?
  AND current_revision_id = ?;
```

These semantics later support browser editing, multiple runs, local synchronization, and Aren tools.

### C7. Make filesystem adapters explicit

The reference implementation should become visible as product adapters, for example:

```text
internal/
  project/
    repository.go
    service.go
    fsstore/

  sprint/
    repository.go
    service.go
    fsstore/

  study/
    repository.go
    service.go
    fsstore/

  run/
    repository.go
    service.go

  platform/
    filesystem/
```

Exact package names should follow Go import-cycle constraints and existing package organisation. The key is that filesystem mapping lives behind product contracts rather than in HTTP handlers or duplicated application flows.

### C8. Introduce the composition root

Persistence selection should happen once during application composition.

```go
type StorageMode string

const (
    StorageFilesystem StorageMode = "filesystem"
    StorageSQLite     StorageMode = "sqlite"
)

type ComposeOptions struct {
    WorkspaceRoot string
    StorageMode   StorageMode
    SQLitePath    string
    Runtime       runtime.Runner
}

func Compose(opts ComposeOptions) (*Application, error) {
    switch opts.StorageMode {
    case StorageFilesystem:
        return composeFilesystemApplication(opts)
    case StorageSQLite:
        return composeSQLiteApplication(opts)
    default:
        return nil, fmt.Errorf("unsupported storage mode %q", opts.StorageMode)
    }
}
```

Use ordinary constructor injection and explicit Go wiring. Do not add a DI framework.

Filesystem composition constructs filesystem repositories. SQLite composition opens the database and constructs SQLite repositories. CLI, TUI, and HTTP adapters receive the resulting application use cases without knowing which persistence mode was selected.

### C9. Add shared repository contract tests

Each repository boundary should have a behaviour suite run against every implementation.

```go
func RepositoryContract(
    t *testing.T,
    newRepository func(t *testing.T) sprint.Repository,
) {
    t.Run("saves and reloads an artefact", ...)
    t.Run("preserves immutable revisions", ...)
    t.Run("rejects stale revisions", ...)
    t.Run("commits stage results atomically", ...)
}
```

Run it against both implementations:

```go
func TestFilesystemRepositoryContract(t *testing.T) {
    RepositoryContract(t, newFilesystemRepository)
}

func TestSQLiteRepositoryContract(t *testing.T) {
    RepositoryContract(t, newSQLiteRepository)
}
```

Also retain end-to-end CLI/TUI/web parity tests. Repository contract tests prove persistence equivalence; adapter parity tests prove the user interfaces still expose the same workflow behaviour.

### C10. Extract incrementally

Do not refactor the entire product before SQLite work begins.

Recommended order:

1. choose one representative browser workflow, likely a sprint artefact read/edit/validate transition;
2. define the smallest product repository required by that workflow;
3. move current filesystem behaviour behind the interface;
4. run existing tests and add repository contract tests;
5. repeat for the next workflow;
6. leave unrelated direct filesystem operations concrete until a second implementation or product need exists.

**Exit criteria for Phase C:**

- no public workflow behaviour changes;
- filesystem mode remains authoritative and production-capable;
- product services depend on focused package-owned repositories where replacement is required;
- repository source and execution workspace access remain explicitly filesystem-based;
- persistence is selected once in the composition root;
- semantic atomicity and optimistic concurrency are represented in contracts;
- filesystem repository implementations pass shared contract suites;
- no universal low-level storage abstraction has been introduced.

## 9. Phase D — Add SQLite-backed local server mode

SQLite is introduced only after the server and persistence boundaries are proven.

### D1. Local database foundation

Add:

- embedded schema migrations;
- transaction helpers;
- foreign-key enforcement;
- busy timeout and bounded write handling;
- health and schema-version reporting;
- backup/export safeguards;
- deterministic test database creation.

Recommended initial location:

```text
~/.local/share/ultraplan/ultraplan.db
```

or an explicitly configured per-server data directory.

Generic SQLite mechanics belong in `internal/platform/sqlite` or an equivalent infrastructure package. Product queries and mappings remain in product-owned SQLite adapters.

### D2. Initial schema

A compact first schema should include:

```text
repositories
projects
studies
sprints
artifacts
artifact_revisions
stage_executions
runs
run_events
validation_results
```

Artefact revisions should be immutable. The artefact record points to its current accepted revision. Draft, recovery, rejected, and approved states should be added only where required by proven frontend workflows.

### D3. SQLite product adapters

Add product-specific implementations:

```text
project/sqlitestore
sprint/sqlitestore
study/sqlitestore
run/sqlitestore
```

The implementation packages may vary to avoid import cycles, but SQL must not leak into application services or HTTP handlers.

Each SQLite repository must pass the same contract suite as its filesystem counterpart before the mode is exposed.

### D4. Explicit storage mode

Support explicit authority rather than silent dual persistence:

```yaml
storage:
  mode: filesystem
```

or:

```yaml
storage:
  mode: sqlite
  database: ~/.local/share/ultraplan/ultraplan.db
```

The browser, CLI, TUI, and application services should remain substantially unchanged because they operate through shared use cases.

One application instance must not mix filesystem-backed projects and SQLite-backed projects accidentally. Per-project mixed authority can be considered later only if a clear product need emerges.

### D5. Import filesystem workspaces into SQLite

Implement a deliberate migration/import operation:

```bash
ultraplan storage migrate \
  --from filesystem \
  --to sqlite \
  --workspace ./workspace \
  --dry-run

ultraplan storage migrate \
  --from filesystem \
  --to sqlite \
  --workspace ./workspace
```

The importer should:

1. discover and validate the workspace using existing public semantics;
2. show the mapping before mutation;
3. assign stable IDs;
4. create initial artefact revisions with source paths and normalized hashes;
5. derive stage state only from valid artefacts and portable checkpoints;
6. record import provenance and source Git revision when available;
7. commit atomically;
8. produce a migration report;
9. leave the source workspace unchanged;
10. switch configured authority only after validation succeeds.

### D6. Frontend revision workflows

Once SQLite is authoritative for a server instance, add the capabilities that justify it:

- draft editing and autosave;
- immutable accepted revisions;
- revision history and comparison;
- optimistic concurrency;
- validation attached to exact revisions;
- run input/output provenance;
- optional proposal, review, and approval states;
- fast cross-project querying.

**Exit criteria for Phase D:**

- one imported project can complete the requirements-to-plan journey entirely in SQLite-backed mode;
- the same application validators and stage rules apply in both modes;
- filesystem and SQLite repositories pass common contract tests;
- SQLite mode survives restart and failed operations without partial state;
- the existing filesystem-backed server remains available for comparison;
- no continuous filesystem/SQLite synchronization exists.

## 10. Phase E — OpenCode execution in SQLite-backed mode

OpenCode remains filesystem-native in the first database-backed implementation.

### E1. Project database state into a run workspace

For each agent-backed operation:

1. create a durable run record with exact input revision IDs;
2. create a unique temporary execution workspace;
3. clone or attach the relevant Git repository when source discovery is required;
4. materialise required UltraPlan artefacts into canonical paths;
5. write a projection manifest containing artefact IDs, revision IDs, paths, and hashes;
6. invoke the existing UltraPlan/AgentWrap/OpenCode workflow in that workspace.

### E2. Collect managed outputs after execution

After completion, cancellation, or failure:

1. compare the workspace with the projection manifest;
2. classify recognised UltraPlan artefacts, source-code changes, diagnostics, and unknown files;
3. validate changed managed artefacts;
4. create new immutable revisions through semantic repository operations;
5. commit multi-artefact stage results atomically;
6. update stage state only when the complete output set is valid;
7. retain partial or invalid work as non-canonical recovery drafts;
8. preserve Git changes as a patch, branch, or commit rather than database file rows;
9. remove the temporary workspace only after capture succeeds.

The initial model remains:

```text
materialise once -> run OpenCode -> collect once
```

It does not require filesystem watchers, per-save database writes, an OpenCode plugin, or a virtual filesystem.

### E3. Recovery

Run records should retain the temporary workspace location and capture status so that server restart can:

- finish output collection;
- preserve recovery drafts;
- report orphaned or interrupted execution;
- avoid losing a long generated document because a later step failed.

**Exit criteria for Phase E:**

- one real reasoning or planning run consumes SQLite revisions and produces validated new revisions;
- OpenCode remains unaware of SQLite;
- stage state and outputs update atomically;
- source-code changes remain ordinary Git workspace changes;
- the execution projection uses the same application validation and repository contracts as direct browser edits.

## 11. Phase F — Compare authority models

Use filesystem-backed server mode and SQLite-backed server mode on real work.

Evaluate:

- editing quality;
- speed and discoverability;
- revision history and recovery;
- Git diff usefulness;
- offline operation;
- local agent compatibility;
- import/export friction;
- complexity of maintaining both repository implementations;
- confidence in database-backed workflow truth;
- value of keeping reports and planning artefacts alongside source code.

Choose one of three outcomes.

### Outcome A — SQLite/server canonical

```text
SQLite = artefact and workflow authority
filesystem = repository source, temporary execution projection, and export format
```

Keep import and export, but do not promise bidirectional live synchronization.

### Outcome B — Both modes remain first-class

Implement revision-aware synchronization only after this decision.

Required concepts:

- stable artefact IDs;
- `.ultraplan/artifact-manifest.json`;
- base revision IDs and normalized content hashes;
- explicit `sync status`, `pull`, `push`, `diff`, and `resolve`;
- conflict detection rather than last-write-wins;
- proposed deletion and rename semantics;
- cloud/server-only operational state.

The optimistic concurrency rules in the repository contracts should be reused by synchronization rather than replaced with a separate conflict model.

### Outcome C — Hybrid publication model

```text
SQLite = drafts, revisions, operations, approvals, and intermediate reports
Git/filesystem = selected accepted requirements, reasoning, plans, and final reports
```

Publishing creates a branch or commit and records the exact database revision-to-Git relationship. Repository changes are explicit imports or proposals, not silent overwrites.

## 12. Phase G — Cloud migration

Only after the local SQLite model and authority choice are proven should the server move to the cloud.

| Proven local component | Cloud evolution |
|---|---|
| SQLite | Postgres |
| Explicit Go composition | Control-plane composition and worker clients |
| Loopback HTTP | Authenticated API |
| Single-user server | Tenant and permission model |
| Temporary local directory | Isolated sandbox/workspace session |
| In-process execution queue | Durable scheduler and worker leasing |
| Local attachments | Object storage |
| Local provider credentials | Short-lived secret broker or provider proxy |
| Local Git checkout | Repository connection, clone, branch, and patch lifecycle |

The control plane owns durable identity, artefacts, workflow state, runs, policies, and sandbox lifecycle. Sandboxes own mutable repository execution state.

The product repository interfaces should remain application-facing. Postgres adapters can replace SQLite adapters without exposing SQL or cloud infrastructure to project, sprint, or study services.

## 13. Phase H — Aren integration and direct artefact tools

The filesystem projection is a compatibility bridge, not the intended final write model for UltraPlan artefacts.

With Aren, the mature path becomes:

```text
Agent
  -> typed UltraPlan artefact tool
  -> Aren tool boundary
  -> UltraPlan application service
  -> validation, policy, and concurrency checks
  -> immutable database revision
```

### Typed read tools

- `get_project_context`
- `list_artifacts`
- `get_artifact`
- `get_artifact_revision`
- `search_artifacts`
- `get_sprint_state`

### Typed write and lifecycle tools

- `save_artifact_draft`
- `propose_artifact_revision`
- `validate_artifact`
- `link_artifacts`
- `submit_artifact_for_review`
- `approve_artifact_revision`

Every write should:

- declare the artefact and expected base revision;
- be scoped to the active run and permissions;
- validate before promotion;
- record agent/run provenance;
- emit a lifecycle event;
- support checkpointing before process termination.

The tool calls should invoke application services, not repository implementations directly. This preserves validation, transition, permission, and atomicity rules across browser, CLI, and agent operations.

The sandbox filesystem remains useful for:

- repository discovery and code search;
- source-code edits;
- builds and tests;
- generated code and configuration;
- temporary scratch work.

A mature sandbox layout may be:

```text
/workspace/repo/       writable Git checkout
/workspace/context/    optional read-only artefact projection
/workspace/scratch/    temporary agent files
```

A final filesystem collection pass should remain as a compatibility and safety mechanism, even when typed tools are preferred.

## 14. Proposed package and composition direction

The exact package layout should be earned during implementation, but the intended ownership is:

```text
cmd/ultraplan/
  main.go

internal/app/
  composition.go
  application.go
  cli wiring

internal/server/
  HTTP routes
  templates
  SSE adapter

internal/project/
  service.go
  repository.go
  fsstore/
  sqlitestore/

internal/sprint/
  service.go
  repository.go
  fsstore/
  sqlitestore/

internal/study/
  service.go
  repository.go
  fsstore/
  sqlitestore/

internal/run/
  service.go
  repository.go
  sqlitestore/

internal/platform/filesystem/
  path safety
  atomic writes
  locks
  hashing
  temp workspaces

internal/platform/sqlite/
  opening
  migrations
  pragmas
  transaction helpers
```

This is directional, not a mandate to create every package immediately. Avoid empty interfaces, one-line wrappers, and single-implementation ports until a real second implementation or test seam exists.

## 15. Testing strategy

### Filesystem-backed server

- CLI/TUI/web parity tests;
- route and template tests;
- path safety and preview tests;
- confirmation, cancellation, locking, SSE reconnect, and shutdown tests;
- race tests and browser security tests.

### Persistence boundary

- package-owned repository contract tests;
- semantic atomicity tests;
- optimistic-concurrency and stale-write tests;
- validator and stage-transition tests remain storage-independent;
- golden workspace fixtures preserve current semantics;
- composition tests prove filesystem and SQLite modes select the correct adapters;
- no product service depends on SQL or generic path APIs through the persistence port.

### SQLite mode

- migration and schema compatibility tests;
- transaction rollback and crash-boundary tests;
- foreign-key and busy-timeout tests;
- import dry-run and atomicity tests;
- revision provenance and validation tests;
- backup and recovery tests;
- every SQLite repository runs the same contract suite as its filesystem counterpart.

### Execution projection

- exact input revision capture;
- deterministic materialisation;
- changed/new/deleted/unknown file classification;
- atomic multi-artefact capture;
- cancellation and failed-run recovery drafts;
- cleanup only after successful capture;
- source-code changes excluded from database artefact rows.

## 16. Key risks and mitigations

### Risk: SQLite work delays the already-planned web release

**Mitigation:** Product Phase 4 remains filesystem-backed and independently releasable. SQLite begins only after its release and dogfood gates.

### Risk: dependency injection becomes framework-heavy

**Mitigation:** use ordinary Go constructors and one explicit composition root. Do not introduce a DI container.

### Risk: a generic storage abstraction preserves filesystem assumptions

**Mitigation:** define package-owned product repositories and keep repository/execution filesystem capabilities separate.

### Risk: too many tiny interfaces and adapters appear before they are needed

**Mitigation:** extract one representative workflow at a time and add ports only where a second implementation or meaningful test seam exists.

### Risk: a universal artefact model flattens real module differences

**Mitigation:** begin with project, sprint, study, and run repositories. Consolidate shared artefact lifecycle behaviour only after repetition is proven.

### Risk: filesystem and SQLite modes diverge

**Mitigation:** use shared repository contract suites plus CLI/TUI/web parity tests.

### Risk: two modes create duplicated product logic

**Mitigation:** share application services, validation, transitions, and operation contracts; vary only persistence and execution adapters.

### Risk: premature synchronization becomes the project

**Mitigation:** use one authority per server/workspace and explicit migration/import/export until the authority decision.

### Risk: database mode loses portability

**Mitigation:** preserve canonical Markdown mappings, deterministic export, provenance manifests, and public CLI validation.

### Risk: OpenCode produces partially valid output

**Mitigation:** capture complete change sets, validate before promotion, retain recovery drafts, and update stage state transactionally.

### Risk: the database becomes a second Git implementation

**Mitigation:** keep source code and source history in Git. Database revisions model UltraPlan artefacts and workflow provenance, not arbitrary repository files.

## 17. Decision gates

### Gate 1 — Filesystem web release

Proceed to persistence-boundary extraction only when:

- Product Phase 4 is released or remaining exceptions are recorded;
- CLI/TUI/web parity is demonstrated;
- real browser dogfood evidence exists;
- the shared application boundary is stable.

### Gate 2 — Persistence boundary readiness

Proceed to SQLite implementation only when:

- product data has been classified;
- one representative workflow runs through package-owned repository contracts;
- filesystem adapters preserve current behaviour;
- semantic atomicity and optimistic concurrency requirements are documented;
- contract tests exist for the extracted repository boundaries;
- repository and execution filesystem access remain outside product persistence.

### Gate 3 — SQLite value proposition

Expose SQLite mode only when concrete needs are documented for several of:

- drafts/autosave;
- immutable revisions;
- cross-project queries;
- workflow provenance;
- approvals/proposals;
- faster navigation;
- server-only intermediate outputs;
- durable run/event history.

### Gate 4 — Authority choice

Do not build bidirectional sync until both filesystem-backed and SQLite-backed modes have been used on real work and Outcome A, B, or C is explicitly selected.

### Gate 5 — Cloud migration

Do not introduce remote sandboxes, tenancy, or Postgres until the local database schema, repository contracts, execution projection, recovery semantics, and authority model are proven.

## 18. Recommended implementation order after Product Phase 4

1. Dogfood the filesystem-backed browser on real studies and sprint workflows.
2. Classify current artefacts, checkpoints, derived data, operational state, run evidence, and repository state.
3. Select one representative sprint or study workflow for boundary extraction.
4. Define a package-owned repository interface in the consuming module.
5. Move existing filesystem behaviour behind a filesystem adapter.
6. Add shared repository contract tests and preserve CLI/TUI/web parity.
7. Repeat only for persistence areas required by the SQLite mode.
8. Add the explicit composition root and `filesystem` storage selection.
9. Add SQLite platform mechanics and product-specific SQLite adapters.
10. Run every repository contract suite against both implementations.
11. Add explicit filesystem-to-SQLite migration with dry-run and validation.
12. Add SQLite-backed browser editing and revision features.
13. Add OpenCode materialisation and post-run capture.
14. Dogfood both authority modes.
15. Choose server-canonical, dual first-class, or hybrid publication.
16. Build synchronization only if dual authorship is explicitly selected.
17. Move the proven architecture to Postgres and sandboxed execution.
18. Integrate Aren typed artefact tools over the same application services.

## 19. Final recommendation

The first server should be the server UltraPlan already plans: a loopback-only HTTP/SSE and browser adapter over shared use cases and the existing filesystem workspace.

After that server is proven, introduce dependency injection at the **product persistence boundary**:

```text
application services
    -> project / sprint / study / run repositories
    -> filesystem or SQLite adapters
```

Do not inject a universal filesystem, and do not abstract the real Git checkout or execution workspace into the database model.

Use explicit constructor injection and one composition root. Keep one authoritative mode at a time. Make atomic stage commits, immutable revisions, and optimistic concurrency part of the repository contracts. Verify filesystem and SQLite behaviour through the same contract tests.

This creates the cleanest path from the existing local product to SQLite, cloud Postgres, sandboxed execution, and eventually Aren tools without turning UltraPlan into a storage-abstraction project before the product has earned it.
