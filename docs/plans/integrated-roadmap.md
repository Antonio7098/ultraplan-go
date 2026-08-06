# UltraPlan Integrated Product Roadmap

**Status:** Proposed  
**Sequence:** Web foundations first  
**Scope:** Consolidates the current plans in `docs/plans` into one delivery order  
**Primary principle:** Build the observable product surface before adding the next major workflows

## 1. Summary

UltraPlan should now evolve along one integrated roadmap rather than treating its current plans as separate initiatives.

The immediate priority is the local web foundation.

The browser should become the most accessible place to understand:

- what projects, studies, and sprints exist;
- which stages are ready, running, complete, stale, blocked, or failed;
- what an operation is currently doing;
- what evidence and artifacts it produced;
- why an operation stopped;
- what the next valid action is;
- whether cancellation, recovery, or intervention is required.

That surface should exist before adding code-context generation, richer QA, repair loops, retrieval, or alternate persistence. Each later capability can then extend an already proven application boundary, event stream, operation model, and browser interface instead of adding another opaque CLI workflow that must be surfaced retrospectively.

The recommended sequence is:

```text
1. Observable local web foundation
2. Guarded web operations and live progress
3. Web hardening and release
4. Sprint code-context stage
5. Minimal content identity and provenance
6. Read-only QA decomposition and synthesis
7. Evidence-producing QA and smoke integration
8. Manual repair and bounded automatic repair
9. Joint dogfooding and schema revision
10. Derived retrieval and lexical search
11. Product persistence boundary and SQLite
12. Optional knowledge graph
13. Authority decision, cloud, and Aren
```

The browser comes first, but the filesystem remains the source of truth. The first server is an adapter over shared application use cases, not a database product and not an independent implementation of UltraPlan semantics.

## 2. Source plans

This roadmap integrates:

- `docs/plans/ultraplan-local-server-experiment-plan.md`
- `docs/plans/sprint-code-context-stage.md`
- `docs/plans/retrieval-ready-content-plan.md`
- `docs/plans/post-execution-qa-and-repair-loop.md`
- `docs/plans/retrieval-ready-content-knowledge-graph-addendum.md`

Those documents remain the detailed design references for their respective capabilities. This roadmap owns their relative priority, dependency order, and stop/go gates.

## 3. Why web foundations come first

UltraPlan already has a broad operational surface:

- study task graphs and synthesis;
- resumable run-loop state;
- planning stages;
- execution;
- review and smoke verification;
- attempts, retries, rate limits, cancellation, locks, and recovery;
- CLI and TUI operations;
- canonical Markdown artifacts and durable JSON state.

The next plans add considerably more:

- a new planning stage;
- source-context artifacts;
- stable content identity and provenance;
- QA maps, shards, theories, evidence, and adjudication;
- repair cycles;
- retrieval records and indexes;
- optional database-backed persistence;
- eventually knowledge traversal.

Adding those capabilities before creating a strong observable interface would make the product increasingly difficult to understand and operate. The web foundation should therefore establish the common product surface into which later features fit.

The web-first choice does not mean building a large frontend platform before useful product work. It means delivering the smallest browser and HTTP/SSE boundary that makes current and future workflows visible, navigable, and controllable.

## 4. Cross-roadmap architectural rules

These rules apply throughout the roadmap.

### 4.1 The filesystem remains authoritative initially

Markdown artifacts, JSON state, study outputs, project files, and sprint files remain the product source of truth until the explicit SQLite phase.

The initial browser must not introduce:

- a shadow database;
- a browser-only workflow state;
- duplicate validation rules;
- independent directory discovery;
- direct Markdown parsing in HTTP handlers;
- subprocess invocation of the CLI for ordinary operations.

### 4.2 Interfaces call shared application use cases

The intended shape is:

```text
CLI -----\
TUI ------> shared application queries and commands -> product modules -> filesystem
HTTP ----/
```

CLI, TUI, and HTTP should differ in presentation and interaction, not in product semantics.

### 4.3 Observability must be truthful

A runtime process exiting successfully is not equivalent to a successful UltraPlan stage.

The web surface should distinguish:

- operation lifecycle;
- runtime lifecycle;
- artifact existence;
- artifact validation;
- canonical stage outcome;
- stale or superseded evidence;
- cleanup or reconciliation uncertainty.

Unknown, stale, interrupted, or partially validated work must never appear as complete.

### 4.4 Markdown remains authoritative content

Future metadata, retrieval records, graph projections, and database revisions must preserve the distinction between authoritative content and derived views.

Until the persistence decision changes:

```text
Markdown artifact = authoritative authored content
flow and run JSON = authoritative machine state for its owned concern
search record = derived and rebuildable
knowledge graph = derived and rebuildable
browser view = projection through application queries
```

### 4.5 Product persistence is separate from source and execution workspaces

A future SQLite or Postgres store may own UltraPlan artifacts, revisions, workflow state, and run metadata.

It must not replace:

- the real Git checkout;
- source-code discovery;
- code edits;
- builds and tests;
- temporary agent workspaces.

### 4.6 Add abstractions only when the next phase requires them

Do not build the final persistence, retrieval, graph, or workflow abstraction during the web phase.

The web foundation should expose existing use cases cleanly and reveal where future boundaries are genuinely needed.

## 5. Phase 1 — Observable local web foundation

**Goal:** Make current UltraPlan state understandable from a browser without changing persistence or mutation semantics.

This phase corresponds to the read-only foundation of the local server plan and should be the next major implementation effort.

### 5.1 Server foundation

Add a loopback-only Go HTTP server, exposed through a command such as:

```bash
ultraplan serve
```

The server should provide:

- explicit bind address and port handling;
- loopback-only defaults;
- health and readiness endpoints;
- graceful startup and shutdown;
- embedded templates and static assets;
- bounded request and response handling;
- typed HTML and JSON error responses;
- no Node.js application server requirement.

### 5.2 Shared application query boundary

HTTP handlers should receive application-facing queries rather than constructing product services themselves.

A possible conceptual shape is:

```go
type Application struct {
    Projects ProjectQueries
    Sprints  SprintQueries
    Studies  StudyQueries
    Runs     RunQueries
}
```

The exact structure should follow existing package ownership. The important rule is that the server uses the same state derivation, discovery, validation, and status logic as the CLI and TUI.

### 5.3 Read-only dashboard

The first browser should allow users to navigate:

- workspace summary;
- projects;
- studies;
- sprints;
- stage status and readiness;
- current and recent runs;
- validation findings;
- lock and recovery status;
- bounded artifact previews;
- next valid actions.

The dashboard should prioritize operational clarity over visual complexity.

### 5.4 Run and stage observability model

Establish a reusable read model for:

- operation identity;
- operation kind;
- project, study, or sprint scope;
- current lifecycle state;
- current stage or task;
- start and finish time;
- latest progress summary;
- attempt count;
- retry or rate-limit state;
- cancellation state;
- terminal outcome;
- canonical artifact pointers;
- validation summary;
- recovery recommendation.

This is a read model over current domain and runtime state. It is not yet a new generic workflow engine or database schema.

### 5.5 Safe artifact viewing

Artifact previews must be:

- bounded in size;
- escaped safely;
- path-safe;
- explicit about truncation;
- able to show validation and freshness state;
- unable to execute embedded scripts or unsafe HTML.

Where practical, previews should link back to the artifact identity and owning project, study, sprint, stage, or run.

### 5.6 Phase 1 exit criteria

- `ultraplan serve` starts and stops reliably.
- The browser can inspect all current projects, studies, sprints, stage state, validation, and recent operations.
- Browser status agrees with CLI and TUI status for shared fixtures.
- HTTP handlers do not parse product files or duplicate workflow rules.
- The filesystem remains the sole product source of truth.
- No database or speculative persistence port is introduced.
- The UI is already useful for observing real current work.

## 6. Phase 2 — Guarded operations and live progress

**Goal:** Make the browser a safe operational interface, not merely a report viewer.

### 6.1 Shared application commands

Expose already-supported mutations through typed application commands.

Initial candidates include:

- starting a study run-loop;
- starting a sprint flow to a selected stage;
- running one explicit stage;
- validating an artifact or stage;
- running review or smoke;
- cancelling an active operation;
- approved recovery actions already supported by the CLI or TUI.

The browser should not gain product capabilities that bypass or differ from the existing application behavior.

### 6.2 Confirmation and stale-confirmation semantics

Destructive, replacing, or expensive operations should use reusable confirmation packets that include:

- operation description;
- affected scope;
- current state or version;
- consequences;
- confirmation expiry or stale-state detection where needed.

A confirmation must fail if the underlying state changes enough to invalidate the user's decision.

### 6.3 Server-sent progress events

Add SSE for live operation progress.

The event surface should support:

- operation started;
- stage or task started;
- progress summary;
- runtime attempt information;
- retry and rate-limit waiting;
- validation result;
- artifact produced;
- cancellation requested and accepted;
- operation completed, failed, blocked, cancelled, or interrupted;
- recovery or next-action recommendation.

Reuse current runtime progress concepts rather than creating a browser-only event model. Events must remain bounded and redacted.

### 6.4 Reconnect and current-state recovery

The browser must not rely exclusively on an in-memory event stream.

After reconnect or server restart, it should be able to load:

- current durable operation state;
- current run-loop or verification state;
- current locks;
- latest known stage outcome;
- recent bounded event summaries where available.

SSE is a live transport, not the source of truth.

### 6.5 Cancellation and locking

Browser cancellation must reach the same context and cleanup path as CLI or TUI cancellation.

Conflicting operations must be rejected through the existing or shared scope-lock semantics. The server should not add an unrelated lock system.

### 6.6 Phase 2 exit criteria

- The browser can safely start and cancel representative current workflows.
- Live progress is truthful and reconnectable.
- CLI, TUI, and browser operations produce equivalent artifacts and state transitions.
- Confirmation and stale-confirmation behavior is shared.
- Conflicting mutations are rejected consistently.
- Cancellation, timeout, cleanup uncertainty, and validation failure are never rendered as success.

## 7. Phase 3 — Web hardening and observable-product release

**Goal:** Make the filesystem-backed browser a supported interface before building the next major workflow.

### 7.1 Hardening

Complete:

- route and input validation;
- path traversal and symlink defenses;
- HTML and script safety;
- CSRF protections appropriate to the local product;
- secret and environment redaction;
- bounded logs and event payloads;
- race and concurrency tests;
- graceful shutdown under active operations;
- orphaned-operation and lock recovery;
- browser refresh and reconnect behavior;
- accessibility and keyboard navigation for core operations.

### 7.2 Documentation and diagnostics

Document:

- local security assumptions;
- bind and access behavior;
- operation confirmation;
- cancellation semantics;
- reconnect and recovery;
- parity with CLI and TUI;
- artifact preview limitations;
- how to diagnose server and SSE failures.

### 7.3 Extensibility gate

Before leaving this phase, prove that a new stage or verification phase can add:

- status;
- artifact preview;
- an operation command;
- progress events;
- recovery information;

without adding a parallel route-specific product implementation.

### 7.4 Phase 3 exit criteria

- The filesystem-backed web product is stable enough for daily UltraPlan use.
- One substantial study and one sprint workflow have been observed and operated through the browser.
- The application query/command boundary is stable enough for later features.
- Remaining web limitations are recorded explicitly.
- The product can now add code-context and QA with immediate browser visibility.

## 8. Phase 4 — Sprint code-context stage

**Goal:** Gather implementation evidence once per sprint and reuse it across all later stages.

Introduce the planning chain:

```text
requirements
-> code-context
-> sprint-index
-> technical-handbook
-> area-reasoning
-> reasoning
-> plan
```

Execution and verification stages should also receive the stored context where they invoke an agent.

The authoritative artifact is:

```text
projects/<project>/sprints/<sprint>/code-context.md
```

The stage should:

- inspect the implementation repository broadly;
- select exact source excerpts relevant to the sprint;
- record repository-relative paths, ranges, symbols, and rationale;
- describe important relationships and open questions;
- write only the sprint artifact;
- avoid making final design or planning decisions;
- permit downstream agents to inspect more source whenever useful.

The exact requirements and code-context content should form a stable shared prompt prefix for downstream agents, improving consistency and allowing provider prompt-prefix caching where available.

The browser should expose:

- code-context readiness and progress;
- the resulting artifact;
- selected source references;
- validation findings;
- explicit rerun actions;
- staleness information when it later becomes available.

### Phase 4 exit criteria

- `code-context` is a first-class stage after requirements.
- It runs once in cumulative planning.
- Downstream prompts receive the exact stored pack.
- The browser observes and controls the stage through existing application and event surfaces.
- No repository index, RAG system, cache subsystem, or JSON context manifest is added.

## 9. Phase 5 — Minimal content identity and provenance

**Goal:** Improve content structure enough to support reliable evidence, QA, and later retrieval without prematurely building a retrieval system.

### 9.1 Inventory and retrieval-question corpus

Before changing all templates:

- inspect representative study, project, sprint, code-context, review, and smoke artifacts;
- define at least twenty real retrieval and traceability questions;
- classify whether each needs metadata, lexical search, relationship traversal, or source lookup;
- measure citation precision, heading consistency, section size, decision grounding, and supersession representation.

### 9.2 Optional artifact metadata envelope

Add an optional, versioned YAML frontmatter parser and validator.

Initial required fields for opted-in artifacts should remain minimal:

```text
schema
id
type
title
status
authority
```

Legacy artifacts without frontmatter remain valid.

### 9.3 Pilot order

Apply structure gradually:

1. source reports;
2. final study reports;
3. sprint requirements;
4. sprint decisions;
5. review and QA findings;
6. project and sprint indexes;
7. technical handbooks and area reasoning;
8. plans only where useful.

Introduce revision-aware evidence before relying heavily on relationships.

### 9.4 Shared identity distinctions

Keep these concepts distinct:

- **artifact ID:** stable semantic identity;
- **block ID:** stable semantic unit within or across artifacts;
- **artifact revision:** immutable content version, needed later by database persistence;
- **input fingerprint:** exact governed inputs to an operation;
- **source snapshot:** repository revision and dirty-state/content identity;
- **derived record ID:** rebuildable search or graph projection identity.

### Phase 5 exit criteria

- New pilot artifacts have stable identity and clear authority.
- Material evidence references repository revision and precise source location.
- Requirements, evidence, patterns, decisions, and findings can be referenced explicitly.
- Legacy workspaces remain usable.
- No search index or graph exists yet.

## 10. Phase 6 — Read-only QA decomposition and synthesis

**Goal:** Establish safe, observable QA architecture without generated tests or production repair.

### 10.1 Terminology and phase model

Keep the existing analytical review capability and present it as **Conformance Review** while preserving command and artifact compatibility.

Introduce a verification-phase concept distinct from planning stages:

```text
conformance-review
qa
repair
```

### 10.2 Deterministic QA map

Map changed behavior into bounded verification surfaces using:

- requirements;
- code-context;
- sprint reasoning and plan;
- execute evidence and changed paths;
- selected contracts and protocols;
- Conformance Review findings;
- adjacent tests and known checks;
- risk tags and package or interface boundaries.

Shard by coherent behavior rather than mechanically by file.

### 10.3 Read-only investigators

Run bounded investigators that may:

- inspect assigned and approved context;
- run safe non-mutating checks;
- formulate falsifiable theories;
- record confirmation, refutation, and inconclusive conditions;
- produce static evidence and recommended checks;
- request bounded cross-shard follow-up.

They may not create tests, mutate production code, or promote their own theories into repairable issues.

### 10.4 Global synthesis

Add central synthesis that:

- deduplicates theories;
- combines related evidence;
- identifies cross-shard interactions;
- retains refuted and invalid theories;
- requests focused follow-up;
- does not yet promote issues or repair code.

The web UI should immediately expose QA maps, shard progress, theory outcomes, synthesis status, and blocking reasons through the existing observability foundation.

### Phase 6 exit criteria

- QA mapping is deterministic for unchanged inputs.
- Every changed path belongs to a bounded verification surface.
- Investigation state is durable and resumable.
- Theory outcomes are visible and inspectable in the browser.
- No investigator mutates production or verification code.
- No automatic repair exists.

## 11. Phase 7 — Evidence-producing QA and smoke integration

**Goal:** Allow investigators to gather discriminating empirical evidence safely.

### 11.1 Isolated investigation workspaces

Create one validated isolated workspace per writable shard attempt.

Investigators may then:

- write targeted tests;
- create fixtures and probes;
- generate smoke scenarios;
- run bounded experiments;
- preserve generated patches and evidence.

They still may not repair production code.

Until isolation is proven, writable investigation remains sequential or disabled. Isolation uncertainty is a blocked outcome.

### 11.2 Evidence adjudication

Add a global adjudicator responsible for:

- expectation grounding;
- evidence validity and freshness;
- flaky or invalid check rejection;
- cross-shard reasoning;
- root-cause grouping;
- issue promotion;
- repair eligibility;
- regression-candidate classification.

A failing check is not automatically a confirmed issue.

### 11.3 Canonical QA

Add:

```text
qa.md
verification/state.json
verification/attempts/...
```

Keep detailed QA attempt state out of `flow-state.json`; flow state should retain canonical summaries, freshness, verdicts, and pointers.

### 11.4 Smoke absorption

Wrap the current smoke protocol as a QA suite/executor while preserving:

- discovery and protocol rules;
- process containment;
- environment allowlisting;
- timeout and cancellation;
- cleanup and reconciliation;
- evidence validation;
- canonical versus narrow evidence distinctions;
- diagnostic-only behavior.

Keep `smoke` and `smoke.md` compatibility until QA parity is proven.

### Phase 7 exit criteria

- QA can safely move from theory to evidence.
- Evidence-backed issues are distinct from suspicions and failed setups.
- Smoke runs through QA without losing guarantees.
- The browser exposes shard evidence, adjudication, issues, and current canonical QA status.
- Stale, malformed, diagnostic, narrow, or uncontained evidence cannot produce a pass.

## 12. Phase 8 — Repair and bounded convergence

**Goal:** Repair only adjudicated, bounded issues and make non-convergence explicit.

### 12.1 Manual single-issue repair

Introduce:

```bash
ultraplan sprint <project> <sprint> repair --issue <id>
```

A repair agent receives a frozen issue packet containing:

- confirmed claim;
- supporting evidence;
- violated expectations;
- allowed paths and scope;
- acceptance criteria;
- exact reproducer;
- affected and containing checks.

Repair may modify production code within scope. It may not weaken evidence, tests, requirements, or acceptance criteria.

### 12.2 Progressive reverification

After a repair:

```text
exact reproducer
-> affected shard
-> linked theories
-> neighbouring or boundary shards
-> containing QA suites
-> Conformance Review delta
```

### 12.3 Bounded automatic cycles

Later add:

```bash
ultraplan sprint <project> <sprint> verify --repair --max-cycles 3
```

Stop when:

- the issue set does not shrink;
- severity, scope, or uncertainty increases;
- a design or requirement decision is needed;
- required evidence is unavailable;
- workspace identity changes;
- cleanup or isolation is uncertain;
- configured cycle or reopening limits are reached.

Expose `verified`, `verified_with_findings`, `failed`, `blocked`, `escalated`, and `stalled` distinctly.

### Phase 8 exit criteria

- One issue can be repaired and reverified end to end.
- Automatic repair is bounded and resumable.
- Repair cannot manufacture a pass by weakening evidence.
- The browser shows cycle history, issue changes, repair scope, reverification, and convergence decisions.

## 13. Phase 9 — Joint dogfooding and schema revision

**Goal:** Evaluate the integrated product before committing to retrieval infrastructure or alternate persistence.

Dogfood together:

- the filesystem-backed browser;
- code-context;
- content identity and provenance;
- Conformance Review;
- QA and repair.

Required cases should include:

- one substantial multi-repository study;
- one study using document sources;
- one full requirements-to-verification sprint;
- one carried-forward decision;
- one superseded decision;
- one finding that contradicts an assumption;
- one browser-managed long-running operation;
- one cancellation or recovery case;
- one repair cycle.

Evaluate:

- whether the browser makes operation state genuinely easier to understand;
- whether later phases fit the shared application and event boundary cleanly;
- metadata accuracy and authoring burden;
- source evidence trustworthiness;
- QA shard quality and investigation cost;
- false-positive, inconclusive, and blocked rates;
- repair convergence;
- repeated filesystem discovery cost;
- need for drafts, immutable revisions, approvals, and cross-project queries;
- whether lexical search would solve real discovery problems;
- whether multi-hop relationship traversal is genuinely needed.

This phase produces explicit stop/go decisions for:

- retrieval infrastructure;
- automatic repair expansion;
- persistence-boundary extraction;
- SQLite;
- knowledge-graph experiments.

## 14. Phase 10 — Derived retrieval baseline

**Goal:** Add retrieval only after the content contract has survived real use.

### 14.1 Derived retrieval records

Create deterministic, disposable records from authoritative artifacts.

Chunk along semantic boundaries:

- one requirement;
- one evidence record;
- one observation;
- one pattern;
- one decision;
- one risk or assumption;
- one review or QA finding;
- one plan task;
- one code-context excerpt;
- one meaningful narrative subsection.

Avoid blind fixed-token windows as the primary strategy.

### 14.2 Narrow lexical prototype

Start with:

- local-only operation;
- read-only queries;
- study reports first;
- metadata filtering;
- lexical search;
- exact evidence output;
- explicit index version and freshness;
- query and result logging for evaluation.

Do not add embeddings before lexical retrieval has a measured baseline. Do not silently inject retrieval results into governed sprint context.

### 14.3 Browser integration

The existing browser should expose:

- index status and freshness;
- filtered search;
- exact matched artifacts and semantic blocks;
- provenance and source references;
- explanation of why a result is current, historical, or superseded.

### Phase 10 exit criteria

- Retrieval answers a defined question corpus measurably better than direct navigation alone.
- Results preserve authority, status, provenance, and exact source text.
- The index is safe to delete and rebuild.
- Explicit project and sprint selections still govern agent context.

## 15. Phase 11 — Product persistence boundary and SQLite

**Goal:** Introduce alternate product persistence only after the web product demonstrates concrete need.

### 15.1 Classify current state

Classify each file or output as:

- durable authored artifact;
- portable workflow checkpoint;
- derived output;
- operational server state;
- run evidence;
- repository source state.

Repository source state remains outside product persistence.

### 15.2 Extract package-owned persistence contracts

Introduce focused repositories only where replacement is required:

```text
project.Repository
sprint.Repository
study.Repository
run.Repository
```

Do not create a generic virtual filesystem or universal storage interface.

Represent:

- semantic atomic stage commits;
- immutable artifact revisions;
- optimistic concurrency;
- stable product identity;
- validation tied to exact revisions.

Move existing filesystem behavior behind adapters one representative workflow at a time and prove it with shared contract tests.

### 15.3 SQLite mode

Add:

- embedded migrations;
- platform SQLite helpers;
- product-specific SQLite adapters;
- explicit `filesystem` or `sqlite` authority selection;
- filesystem-to-SQLite migration with dry-run;
- immutable revisions and history;
- browser draft, comparison, and concurrency workflows where justified;
- no dual writes or silent synchronization.

### 15.4 Agent execution projection

In SQLite mode:

```text
database revisions
-> temporary filesystem workspace
-> agent execution
-> validated output collection
-> atomic new revisions
```

Source code remains in Git. OpenCode or a later runtime remains unaware of SQLite.

### Phase 11 exit criteria

- A representative project journey works in both filesystem and SQLite modes.
- Both implementations pass shared repository contracts.
- The browser and application services remain substantially unchanged across modes.
- Failed operations cannot leave partial canonical state.
- The filesystem-backed mode remains available for comparison.
- No continuous bidirectional synchronization exists.

## 16. Phase 12 — Optional knowledge graph

**Goal:** Add bounded relationship traversal only if retrieval and direct artifact inspection are insufficient.

Prerequisites:

- retrieval-ready content has been dogfooded;
- stable IDs and explicit relationships exist in real artifacts;
- a lexical retrieval baseline exists;
- real questions require multi-hop provenance or traceability;
- direct inspection does not answer them adequately.

Begin with an in-memory, read-only graph built from authoritative artifacts and explicit or deterministic relationships.

Initial capabilities may include:

```bash
ultraplan knowledge status
ultraplan knowledge inspect <entity>
ultraplan knowledge trace <entity>
ultraplan knowledge validate
ultraplan knowledge explain-path <from> <to>
```

Do not initially introduce:

- a graph database;
- graph-authored state;
- unrestricted inferred edges;
- a universal ontology;
- comprehensive source-code topology.

Only after proven value should the roadmap consider:

```text
in-memory graph
-> diagnostic commands
-> optional derived SQLite projection
-> hybrid text and graph retrieval
-> delivery traceability
-> optional source topology
```

The graph remains derived and disposable.

## 17. Phase 13 — Authority decision, cloud, and Aren

After both filesystem and SQLite modes have been used on real work, choose explicitly:

### Option A — SQLite/server canonical

```text
SQLite = artifact and workflow authority
filesystem = source, execution, export, and publication
```

### Option B — Both modes remain first-class

Only then design explicit revision-aware pull, push, diff, and conflict resolution. Do not use last-write-wins synchronization.

### Option C — Hybrid publication

```text
SQLite = drafts, revisions, operations, approvals, intermediate outputs
Git/filesystem = selected accepted artifacts and reports
```

Cloud migration follows proven local semantics:

```text
SQLite -> Postgres
loopback HTTP -> authenticated API
single local process -> durable scheduler and workers
local temp workspace -> isolated sandbox
local attachments -> object storage where appropriate
```

Aren should expose typed artifact tools over the same application services. Sandboxes continue to own repository discovery, code edits, builds, tests, and temporary work.

## 18. Recommended immediate delivery sequence

The next implementation order should be:

1. Add the loopback server, application query boundary, and read-only dashboard.
2. Add run and stage observability, safe artifact previews, validation, locks, and recovery status.
3. Add guarded browser operations, shared confirmation, SSE progress, cancellation, and reconnect.
4. Harden and dogfood the filesystem-backed browser until it is a supported interface.
5. Implement the sprint `code-context` stage and surface it through the existing web operation and observability model.
6. Complete the content inventory and add the optional minimal metadata parser.
7. Pilot revision-aware evidence on study reports, then requirement and decision IDs.
8. Add read-only QA mapping, investigators, and synthesis with full browser visibility.
9. Add isolated evidence-producing QA and absorb smoke as a QA executor.
10. Add manual repair, then bounded automatic repair.
11. Dogfood the integrated system and revise schemas and workflows.
12. Build lexical retrieval only if the question corpus demonstrates value.
13. Extract persistence boundaries and add SQLite only if browser usage demonstrates concrete product needs.
14. Treat knowledge graphs, synchronization, cloud, and Aren as separately gated later phases.

## 19. Stop conditions

Pause or simplify a roadmap branch when its evidence gate is not met.

### Web

Pause interface expansion if routes are duplicating product logic or if parity cannot be maintained. Fix the application boundary first.

### Content structure

Pause if metadata becomes inaccurate bureaucracy, agents invent relationships, or structured output materially worsens analysis.

### QA

Pause automatic expansion if theory quality is poor, evidence is frequently invalid, isolation is unreliable, or repair does not converge.

### Retrieval

Stop before embeddings or agent-facing retrieval if metadata and lexical search already answer the real question corpus adequately.

### SQLite

Do not proceed merely because a database seems cleaner. Require concrete needs such as drafts, immutable revisions, cross-project queries, approvals, provenance, or performance.

### Knowledge graph

Do not persist or expand the graph unless bounded in-memory traversal answers valuable real questions more reliably than direct artifact inspection and retrieval.

## 20. Final recommendation

Build the observable local product first.

The first major milestone should be a filesystem-backed browser that truthfully exposes UltraPlan's current state, artifacts, operations, progress, failures, cancellation, and recovery through shared application use cases.

That gives every later capability an accessible operational home:

```text
web observability foundation
  -> code-context visibility
  -> structured evidence visibility
  -> QA and repair visibility
  -> retrieval visibility
  -> persistence and revision workflows
  -> optional knowledge traversal
```

The web-first sequence improves the development process as well as the product. Later stages can be dogfooded, inspected, debugged, and compared through the browser from their first implementation rather than becoming another layer of hidden state that must be surfaced afterward.

The browser should therefore come first, while the filesystem, existing product modules, and shared application semantics remain firmly in control.