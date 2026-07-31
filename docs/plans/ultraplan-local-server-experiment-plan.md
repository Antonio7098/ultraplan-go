# UltraPlan Local Server Experiment and Cloud Migration Plan

**A staged plan to evaluate a database-backed server and frontend while preserving the existing local-first CLI**  
**Version:** Planning artefact v1.1  
**Prepared:** 31 July 2026  
**Repository:** https://github.com/Antonio7098/ultraplan-go

> **Recommended strategy:** Build a database-backed local server and browser frontend as a parallel operating mode. Keep the existing filesystem CLI intact, use explicit import/export during the experiment, and postpone bidirectional synchronization until real use proves that both modes deserve to remain first-class.

## 1. Executive summary

UltraPlan is currently a local-first Go CLI whose product workflows are expressed through filesystem workspaces, Markdown artefacts, JSON state, embedded prompts and templates, and delegated agent execution. The proposed experiment adds a local database-backed server and frontend without prematurely replacing the working CLI or solving cloud infrastructure.

The experiment answers one product question: **does UltraPlan become materially better when artefacts, revisions, workflow state, and runs are managed through a server and visual interface?**

### Expected end state

- Existing CLI and filesystem workspaces continue to work independently.
- A local server persists projects, studies, sprints, artefacts, revisions, stage state, runs, and events in SQLite.
- A frontend supports navigation, editing, revision history, validation, stage progression, and run observation.
- Agent execution uses temporary filesystem projections.
- Import/export provides portability without premature live sync.
- A formal decision gate selects server-only, revision-aware dual mode, or hybrid Git publication.

## 2. Guiding principles

1. Preserve local-first until disproven.
2. One storage authority per workspace during the experiment.
3. Earn abstractions around real product operations.
4. Keep files as the execution and portability format.
5. Let the database own server workflow truth.
6. Make the local architecture resemble the future cloud control plane.
7. Avoid distributed-systems machinery before the local model is proven.

## 3. Target architecture

```text
Browser frontend
      |
      | HTTP + Server-Sent Events
      v
UltraPlan local server
  - API and application services
  - artifact/revision management
  - workflow and validation orchestration
  - run lifecycle and event persistence
      |
      +--> SQLite database
      |
      +--> temporary workspace projection
              +--> existing UltraPlan operations
              +--> AgentWrap / OpenCode
              +--> Git and language tooling

Existing CLI --------------------> filesystem workspace
```

## 4. Core data model

- **Repository**: external Git identity and source context.
- **Project**: long-lived planning scope.
- **Study**: sources, dimensions, analyses, and final reports.
- **Sprint**: governed delivery scope and stage chain.
- **Artifact**: stable identity for one logical document.
- **ArtifactRevision**: immutable content snapshot with provenance.
- **StageExecution**: persistent state for a workflow stage.
- **Run**: one execution attempt with exact inputs and outputs.
- **RunEvent**: ordered structured execution record.
- **ImportExportJob**: auditable movement between filesystem and server.

Artifact revisions are immutable. Editing creates a new revision whose parent is the version edited. The artifact points to its current revision.

## 5. Suggested storage

| Data | Canonical store in server mode |
|---|---|
| Projects, studies, sprints | SQLite |
| Draft and current artefacts | SQLite immutable revisions |
| Stage state, validation, runs, events | SQLite |
| Source code | Git checkout when required |
| Active run workspace | Temporary filesystem projection |
| Large diagnostics/evidence | Local attachment directory initially; object storage in cloud |
| Exported portable workspace | Filesystem/Git |

## 6. API and frontend

Use versioned JSON HTTP endpoints plus SSE for run events. Initial surfaces:

- projects and project overview
- sprints and stage transitions
- studies and operations
- artefacts and immutable revisions
- validation
- runs, cancellation, and events
- import/export
- health, version, and capabilities

Frontend vertical slice:

1. Home/project list
2. Project overview
3. Sprint stage workspace
4. Markdown editor and preview
5. Revision history and diff
6. Run detail and live events
7. Study progress/report view
8. Import/export preview

## 7. Execution projection

1. Persist the run and exact input revision set.
2. Create a unique temporary workspace.
3. Clone source context when required.
4. Materialise DB artefacts into canonical UltraPlan paths.
5. Write a projection manifest containing IDs, paths, revisions, and hashes.
6. Invoke current UltraPlan/AgentWrap execution.
7. Persist normalized events and bounded diagnostics.
8. Capture only declared outputs.
9. Validate candidates.
10. Create revisions and update stage state atomically.
11. Clean up or retain the projection according to policy.

### Short-term OpenCode write path

OpenCode remains completely filesystem-native during the local-server experiment. It is started in the temporary projected workspace and continues to read, create, and edit normal files. It does not need database awareness or a special UltraPlan tool.

The server surrounds each run with two adapters:

- **Workspace projector:** writes the exact database revision set into the canonical UltraPlan directory layout and records a baseline manifest.
- **Workspace collector:** compares the completed workspace with the baseline, classifies changes, validates managed outputs, and persists new immutable revisions.

The collector must not copy the entire workspace into the database. It imports only recognised UltraPlan artefacts, including requirements, indexes, handbooks, reasoning documents, plans, study dimensions, and reports. Source-code changes remain Git changes; logs and large diagnostics remain run artefacts; temporary and unknown files are retained only for inspection or discarded according to policy.

Collection occurs after successful completion and on a best-effort basis after cancellation, failure, or server recovery. Valid expected outputs are promoted atomically with stage state. Invalid or partial outputs are retained as non-canonical recovery drafts so that useful work is not lost. The temporary workspace is not removed until capture has completed successfully.

This first implementation is deliberately snapshot-based:

```text
materialise once
-> execute OpenCode
-> collect once
```

It does not require filesystem watchers, per-save database writes, a virtual filesystem, or an OpenCode plugin.

## 8. Import/export before synchronization

### Import

- Discover and validate a filesystem workspace.
- Produce a dry-run mapping report.
- Map paths to logical artefact kinds.
- Create initial revisions with import provenance.
- Derive portable stage state from validated artefacts.
- Commit transactionally and leave the source untouched.

### Export

- Freeze a revision set.
- Materialise canonical paths and configuration.
- Generate portable checkpoints and a traceability manifest.
- Validate through the public CLI.
- Never silently overwrite an existing destination.

## 9. Implementation phases

### Phase 0 - Baseline and characterization

- Classify durable artefacts, portable checkpoints, derived outputs, and transient state.
- Add representative fixture workspaces and golden regression tests.
- Characterize reads and writes for key workflows.

**Exit:** current behaviour is protected and every initial artefact kind has a documented filesystem mapping.

### Phase 1 - Product-level persistence seams

- Extract pure validators, transitions, prompt construction, and report operations.
- Add stable IDs and artifact kinds.
- Wrap path mapping in filesystem adapters.
- Return structured application results from commands.

**Exit:** CLI is unchanged; one representative stage operates from structured inputs without HTTP or SQLite knowledge.

### Phase 2 - Local server foundation

- Add server entrypoint, configuration, graceful shutdown, logging, errors, migrations, SQLite, and SSE.
- Persist runs and events before enabling execution.

**Exit:** runs and ordered events survive restart.

### Phase 3 - Database-backed product storage

- Implement projects, studies, sprints, artefacts, revisions, stages, and validation.
- Add optimistic concurrency and revision comparison.

**Exit:** a complete documentation journey works without a permanent workspace.

### Phase 4 - Frontend vertical slice

- Build project, sprint, artefact editor, history, diff, validation, and study views.

**Exit:** requirements-to-plan documentation can be performed through the frontend, excluding agent generation.

### Phase 5 - Run orchestration and projections

- Add typed operations, durable lifecycle, cancellation, projection manifests, capture, validation, and recovery.

**Exit:** one real agent-backed workflow produces validated DB revisions without partial stage advancement.

### Phase 6 - Import/export and real-use evaluation

- Import substantial existing work.
- Export and validate it through the current CLI.
- Use the frontend over multiple real sessions and record friction.

**Exit:** an evidence-backed authority decision can be made.

### Phase 7 - Authority-model decision

Choose:

- **A. Server canonical:** files are projections and exports.
- **B. Dual first-class modes:** implement revision-aware synchronization.
- **C. Hybrid:** server owns drafts and operations; selected approved revisions publish to Git.

## 10. Optional sync phase

Only if Outcome B is selected:

- commit `.ultraplan/artifact-manifest.json`
- track stable IDs, base revisions, paths, and hashes
- implement `sync status`, `pull`, `push`, `diff`, and `resolve`
- require declared parent revisions
- detect conflicts instead of overwriting
- add three-way Markdown merge only after manual conflict handling is reliable
- keep live run state, leases, approvals, and usage data server-only

## 11. Cloud migration

| Local | Cloud |
|---|---|
| SQLite | Postgres |
| Temporary directory | Isolated sandbox/workspace session |
| In-process queue | Durable scheduler and worker leasing |
| Local diagnostics | Object storage |
| Local credentials | Short-lived secret broker/provider proxy |
| Loopback API | Authenticated multi-tenant API |

**UltraPlan owns** study and sprint semantics, artefacts, validation, and governed stages.  
**Aren owns** generic run lifecycle, sandboxing, processes, cancellation, resource policy, events, credentials, and scheduling.

### Mature Aren artefact path

The filesystem projection is a compatibility bridge, not the intended final write model. Once Aren provides a custom harness and typed tools, agents should write durable UltraPlan outputs through an artefact service rather than by creating Markdown files and waiting for post-run inference.

```text
Agent
  -> create or revise artefact tool
  -> Aren tool boundary
  -> UltraPlan application service
  -> validation and optimistic-concurrency checks
  -> immutable database revision
```

The agent may continue to use the repository filesystem for source discovery, code search, builds, tests, and source-code modification. Planning context can be retrieved through typed read tools or exposed as a read-only projection. A mature sandbox can therefore separate:

```text
/workspace/repo/       writable Git checkout for code
/workspace/context/    optional read-only artefact projection
/workspace/scratch/    temporary working files
```

Aren tools should support explicit operations such as `get_artifact`, `search_artifacts`, `save_artifact_draft`, `propose_artifact_revision`, `validate_artifact`, and `submit_artifact_for_review`. Every write supplies the base revision, records run provenance, enforces policy, emits an event, and can checkpoint long-running work before the agent exits.

The final filesystem collection pass should remain available as a safety and compatibility mechanism because coding agents and shell tools may still modify managed files without using the typed tool.

## 12. Recommended first vertical slice

Import one project into SQLite; display one sprint; edit requirements as immutable revisions; validate them; trigger one reasoning or planning operation through a temporary projection; stream the run; capture the result; and export the project back to a valid filesystem workspace.

This slice exercises identity, revisions, lifecycle, validation, API, frontend, execution, events, projection, and portability without building every feature.

## 13. Acceptance criteria

- Existing filesystem CLI remains green and independent.
- Server mode supports project, sprint, study, artefact, revision, and validation workflows.
- One agent-backed operation produces validated immutable outputs atomically.
- Runs, events, cancellation intent, and terminal state survive restart.
- Import has a dry-run preview and strict transaction.
- Export passes public CLI validation.
- Frontend exposes validation, conflicts, and run state clearly.
- A written decision selects the long-term authority model.

## 14. Final recommendation

Proceed with the local server and frontend as an additive independent mode. Preserve filesystem UltraPlan, but do not continuously synchronize the two during the experiment. Use explicit import/export and temporary execution projections. Decide whether sync is worth building only after the database-backed product has been used for real studies and sprint planning.
