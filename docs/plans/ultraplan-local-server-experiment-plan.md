# UltraPlan Filesystem-First Server, SQLite Migration, and Cloud Plan

**A staged plan to extend the existing local-first product without prematurely changing its source of truth**  
**Version:** Planning artefact v1.2  
**Prepared:** 31 July 2026  
**Repository:** https://github.com/Antonio7098/ultraplan-go  
**Authoritative roadmap:** https://github.com/Antonio7098/ultraplan-workspace/blob/main/projects/ultraplan-go/roadmap.md

> **Recommended sequence:** Complete the already-planned local Go server and browser UI directly over the existing filesystem-backed application. Use that interface on real work. Only then introduce SQLite as a second persistence implementation, migrate server-managed artefacts into it, and decide whether the filesystem remains first-class, becomes an import/export format, or is retained only as an execution projection and Git publication surface.

## 1. Executive summary

UltraPlan already has the correct immediate next step in its roadmap: Product Phase 4 adds a loopback-only Go HTTP server, browser UI, guarded operations, and SSE progress over the same shared application use cases and filesystem workspace used by the CLI and TUI.

That phase should be completed before introducing a database. It proves the server boundary, frontend workflows, typed application operations, progress streaming, cancellation, locking, security, and CLI/TUI/web parity without changing persistence semantics at the same time.

The broader evolution should therefore be:

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
Explicit storage boundary and SQLite-backed server mode
        |
        v
Filesystem import/export and optional synchronization decision
        |
        v
Cloud control plane, sandboxed repository execution, and Aren tools
```

This sequence separates three different questions:

1. **Is a server and browser UI useful?** Test this while preserving the filesystem source of truth.
2. **Is database-backed artefact management better?** Test this after the server interaction model is proven.
3. **Does local filesystem authorship deserve to remain first-class?** Decide this only after using both modes.

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

This plan begins after, and builds on, that work. It does not move SQLite into Product Phase 4.

## 3. Guiding principles

1. **One major architectural change at a time.** First add the interface; then change persistence.
2. **Preserve current behaviour before migrating it.** The filesystem-backed CLI, TUI, and server establish the reference semantics.
3. **Shared application use cases remain central.** CLI, TUI, browser, and later Aren tools call the same product operations rather than reimplementing workflows.
4. **Do not force a storage abstraction across every file access.** Add persistence seams around meaningful product operations and artefact lifecycles.
5. **Use real work as evidence.** The authority decision follows dogfooding, not architectural preference alone.
6. **Keep OpenCode filesystem-native in the short term.** Database integration surrounds execution rather than requiring an OpenCode plugin.
7. **Keep SQLite migration local and single-user first.** Do not mix it with authentication, tenancy, remote workers, or cloud sandboxing.
8. **Treat Aren as the long-term execution substrate, not a prerequisite.** The local server and SQLite work should remain useful when Aren arrives.

## 4. Evolution of the architecture

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

### Stage C — SQLite-backed local server mode

```text
Browser / server API
        |
        v
shared app use cases
        |
        v
product persistence boundary
      /   \
filesystem  SQLite
mode        server mode
```

A workspace or server instance has one authority at a time. The first SQLite version should not continuously synchronize both stores.

### Stage D — Cloud and Aren

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

## 5. Phase A — Complete the planned filesystem-backed web product

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

Expose the already-supported operations through shared application commands rather than subprocess invocation or CLI scraping.

Key outcomes:

- confirmation and stale-confirmation semantics are reusable across TUI and web;
- run progress is streamed truthfully through SSE;
- cancellation reaches the shared operation context;
- concurrent conflicting mutations are rejected through existing scope locks;
- browser state can reconnect to current durable filesystem-backed operation state;
- CLI, TUI, and browser produce equivalent workflow outcomes.

### A3. Hardening and release

Complete security, recovery, documentation, race testing, shutdown, redaction, and parity gates.

**Exit criteria for Phase A:**

- the browser is a supported local interface;
- the filesystem remains the sole product source of truth;
- every web mutation passes through typed shared application use cases;
- the server is sufficiently stable to use for real UltraPlan work;
- database work has not leaked into the Phase 4 architecture.

## 6. Phase B — Dogfood the filesystem-backed server

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

### Evidence to collect

- representative navigation and operation traces;
- filesystem reads and writes per workflow;
- latency of project/study/sprint discovery;
- examples of desired autosave, draft, history, diff, and approval behaviour;
- incidents where local file editing is easier than the browser;
- incidents where filesystem state makes the browser experience awkward;
- data that should remain portable versus server-only.

**Exit criteria for Phase B:**

- at least one substantial study and one substantial planning/execute/review/smoke workflow have been managed through the browser;
- the desired database entities are grounded in observed workflows;
- a written storage classification exists for every managed artefact and state file;
- there is evidence that SQLite solves concrete product problems rather than merely changing technology.

## 7. Phase C — Define the storage boundary without changing authority

Introduce typed persistence contracts while the filesystem remains authoritative.

### C1. Classify current data

Classify each current file or output as one of:

- **durable authored artefact:** requirements, handbooks, reasoning, plans, final reports;
- **portable workflow checkpoint:** validated stage completion or resumable study state;
- **derived output:** summaries, indexes, cached previews;
- **operational server state:** active requests, subscribers, confirmations, leases;
- **run evidence:** logs, diagnostics, transcripts, test output;
- **repository source state:** code, tests, configuration, Git history.

### C2. Introduce product identities

Add stable concepts where they are genuinely required by database storage:

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

### C3. Extract focused persistence seams

Prefer boundaries such as:

```go
type ArtifactRepository interface {
    GetCurrent(ctx context.Context, ref ArtifactRef) (ArtifactRevision, error)
    List(ctx context.Context, scope ArtifactScope) ([]ArtifactSummary, error)
    SaveRevision(ctx context.Context, input SaveRevisionInput) (ArtifactRevision, error)
}

type WorkflowStateRepository interface {
    LoadSprint(ctx context.Context, ref SprintRef) (SprintState, error)
    ApplyTransition(ctx context.Context, transition StageTransition) error
}
```

Avoid a lowest-common-denominator virtual filesystem interface that simply recreates `ReadFile`, `WriteFile`, and `WalkDir` remotely.

### C4. Keep filesystem adapters as the reference implementation

The CLI, TUI, and filesystem-backed web mode should continue passing the existing regression and parity suites through the new seams.

**Exit criteria for Phase C:**

- no public workflow behaviour changes;
- filesystem mode remains authoritative and production-capable;
- one representative end-to-end workflow is expressed through typed product persistence operations;
- the seams reflect observed product needs rather than speculative cloud requirements.

## 8. Phase D — Add SQLite-backed local server mode

SQLite is introduced only after the server and application boundaries are proven.

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

Artefact revisions should be immutable. The artefact record points to its current accepted revision. Draft, recovery, rejected, and approved states can be added only where required by the frontend workflow.

### D3. Explicit server storage mode

Support an explicit mode rather than silent dual persistence:

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

The browser and API should remain substantially unchanged because they operate through shared application services.

### D4. Import filesystem workspaces into SQLite

Implement a deliberate import operation:

```bash
ultraplan server import-workspace ./workspace --dry-run
ultraplan server import-workspace ./workspace
```

The importer should:

1. discover and validate the workspace using existing public semantics;
2. show the mapping before mutation;
3. assign stable IDs;
4. create initial artefact revisions with source paths and hashes;
5. derive stage state only from valid artefacts and portable checkpoints;
6. record import provenance and source Git revision when available;
7. commit atomically;
8. leave the source workspace unchanged.

### D5. Frontend revision workflows

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

- one imported project can complete the requirements-to-plan journey entirely in SQLite-backed server mode;
- the same application validators and stage rules apply in both modes;
- SQLite mode survives restart and failed operations without partial state;
- the existing filesystem-backed server remains available for comparison;
- no continuous filesystem/SQLite synchronization exists yet.

## 9. Phase E — OpenCode execution in SQLite-backed mode

OpenCode remains filesystem-native in the first database-backed implementation.

### E1. Project database state into a run workspace

For each agent-backed operation:

1. create a durable run record with exact input revision IDs;
2. create a unique temporary workspace;
3. clone or attach the relevant Git repository when source discovery is required;
4. materialise required UltraPlan artefacts into canonical paths;
5. write a projection manifest containing artefact IDs, revision IDs, paths, and hashes;
6. invoke the existing UltraPlan/AgentWrap/OpenCode workflow in that workspace.

### E2. Collect managed outputs after execution

After completion, cancellation, or failure:

1. compare the workspace with the projection manifest;
2. classify recognised UltraPlan artefacts, source-code changes, diagnostics, and unknown files;
3. validate changed managed artefacts;
4. create new immutable revisions in one transaction;
5. update stage state only when the complete output set is valid;
6. retain partial or invalid work as non-canonical recovery drafts;
7. preserve Git changes as a patch, branch, or commit rather than database file rows;
8. remove the temporary workspace only after capture succeeds.

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
- source-code changes remain ordinary Git workspace changes.

## 10. Phase F — Compare authority models

Use filesystem-backed server mode and SQLite-backed server mode on real work.

Evaluate:

- editing quality;
- speed and discoverability;
- revision history and recovery;
- Git diff usefulness;
- offline operation;
- local agent compatibility;
- import/export friction;
- complexity of maintaining both modes;
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

### Outcome C — Hybrid publication model

```text
SQLite = drafts, revisions, operations, approvals, and intermediate reports
Git/filesystem = selected accepted requirements, reasoning, plans, and final reports
```

Publishing creates a branch or commit and records the exact database revision-to-Git relationship. Repository changes are explicit imports or proposals, not silent overwrites.

## 11. Phase G — Cloud migration

Only after the local SQLite model and authority choice are proven should the server move to the cloud.

| Proven local component | Cloud evolution |
|---|---|
| SQLite | Postgres |
| Loopback HTTP | Authenticated API |
| Single-user server | Tenant and permission model |
| Temporary local directory | Isolated sandbox/workspace session |
| In-process execution queue | Durable scheduler and worker leasing |
| Local attachments | Object storage |
| Local provider credentials | Short-lived secret broker or provider proxy |
| Local Git checkout | Repository connection, clone, branch, and patch lifecycle |

The control plane owns durable identity, artefacts, workflow state, runs, policies, and sandbox lifecycle. Sandboxes own mutable repository execution state.

## 12. Phase H — Aren integration and direct artefact tools

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

## 13. Testing strategy

### Filesystem-backed server

- CLI/TUI/web parity tests;
- route and template tests;
- path safety and preview tests;
- confirmation, cancellation, locking, SSE reconnect, and shutdown tests;
- race tests and browser security tests.

### Persistence seams

- contract tests run against filesystem and SQLite implementations;
- validator and stage-transition tests remain storage-independent;
- golden workspace fixtures preserve current semantics.

### SQLite mode

- migration and schema compatibility tests;
- transaction rollback and crash-boundary tests;
- optimistic-concurrency tests;
- import dry-run and atomicity tests;
- revision provenance and validation tests;
- backup and recovery tests.

### Execution projection

- exact input revision capture;
- deterministic materialisation;
- changed/new/deleted/unknown file classification;
- atomic multi-artefact capture;
- cancellation and failed-run recovery drafts;
- cleanup only after successful capture;
- source-code changes excluded from database artefact rows.

## 14. Key risks and mitigations

### Risk: SQLite work delays the already-planned web release

**Mitigation:** Product Phase 4 remains filesystem-backed and independently releasable. SQLite begins only after its release gate.

### Risk: storage abstractions distort simple filesystem behaviour

**Mitigation:** extract seams from observed server workflows and preserve the filesystem implementation as the reference contract.

### Risk: two modes create duplicated product logic

**Mitigation:** share application services, validation, transitions, and operation contracts; vary only persistence and execution adapters.

### Risk: premature synchronization becomes the project

**Mitigation:** use one authority per server/workspace and explicit import/export until the authority decision.

### Risk: database mode loses portability

**Mitigation:** preserve canonical Markdown mappings, deterministic export, provenance manifests, and public CLI validation.

### Risk: OpenCode produces partially valid output

**Mitigation:** capture complete change sets, validate before promotion, retain recovery drafts, and update stage state transactionally.

### Risk: the database becomes a second Git implementation

**Mitigation:** keep source code and source history in Git. Database revisions model UltraPlan artefacts and workflow provenance, not arbitrary repository files.

## 15. Decision gates

### Gate 1 — Filesystem web release

Proceed to SQLite design only when:

- Product Phase 4 is released or remaining exceptions are recorded;
- CLI/TUI/web parity is demonstrated;
- real browser dogfood evidence exists;
- the shared application boundary is stable.

### Gate 2 — SQLite value proposition

Proceed to implementation only when concrete needs are documented for several of:

- drafts/autosave;
- immutable revisions;
- cross-project queries;
- workflow provenance;
- approvals/proposals;
- faster navigation;
- server-only intermediate outputs;
- durable run/event history.

### Gate 3 — Authority choice

Do not build bidirectional sync until both filesystem-backed and SQLite-backed modes have been used on real work and Outcome A, B, or C is explicitly selected.

### Gate 4 — Cloud migration

Do not introduce remote sandboxes, tenancy, or Postgres until the local database schema, execution projection, recovery semantics, and authority model are proven.

## 16. Recommended immediate action

Continue with the roadmap’s existing Product Phase 4 exactly as a filesystem-backed server and browser UI.

After Sprint 32:

1. dogfood the filesystem-backed browser;
2. document persistence pain and desired database capabilities;
3. define focused product persistence seams;
4. introduce SQLite as an explicit alternative server mode;
5. import existing workspaces and run real OpenCode flows through projection/capture;
6. decide whether the filesystem remains first-class;
7. only then implement synchronization or simplify around the server;
8. move the proven model into the cloud and integrate Aren.

## 17. Final recommendation

The first server should not be database-backed. It should be the server UltraPlan already plans: a loopback-only HTTP/SSE and browser adapter over shared use cases and the existing filesystem workspace.

That gives you a useful frontend sooner and provides the behavioural baseline against which SQLite can be judged. SQLite then becomes a deliberate migration of a proven local server rather than a simultaneous rewrite of persistence, execution, and user experience.
