# Addendum: Speculative Knowledge Graph for Retrieval-Ready UltraPlan Content

## Status

This document extends `docs/plans/retrieval-ready-content-plan.md` with a deliberately speculative, post-retrieval knowledge-graph direction.

It is not an implementation commitment and does not change the priority or sequencing of the original plan. The retrieval-ready content work remains the prerequisite. A graph should be considered only after stable artifact identity, semantic blocks, provenance, controlled relationships, revision-aware evidence, and a useful retrieval baseline have been demonstrated on real studies and sprint workflows.

## Summary

A knowledge graph could eventually help UltraPlan represent and traverse the lineage between:

```text
external evidence
  -> observations and patterns
  -> project interpretation
  -> accepted decisions
  -> planned work
  -> implementation surfaces
  -> tests and review findings
  -> later learning or supersession
```

The graph should not replace Markdown artifacts, source repositories, Git history, or future product persistence. It should be a derived, disposable projection over authoritative UltraPlan content and repository evidence.

The most valuable end state is not a generic document graph. It is a governed evidence-and-decision graph that can answer questions such as:

- Which evidence supports this accepted decision?
- Which requirements does this decision implement?
- Which plan tasks and code surfaces implement it?
- Which tests, review findings, or smoke evidence verify it?
- Which later decision superseded it?
- Where do source observations disagree?
- Which current decisions depend on stale evidence or historical assumptions?

A future graph should be introduced in small stages: explicit content relationships first, then an in-memory artifact graph, then optional derived persistence, then hybrid graph-and-text retrieval, and only much later source-topology integration if real use demonstrates the need.

## Relationship to the retrieval-ready content plan

The original plan already introduces the essential prerequisites:

- stable artifact IDs;
- typed semantic blocks;
- controlled relationships;
- source and revision provenance;
- status, authority, and supersession;
- deterministic chunking;
- derived retrieval records;
- an optional narrow retrieval prototype.

Those features should remain useful even if no graph is ever built.

This addendum begins conceptually after the original plan's retrieval prototype. It does not require the graph phases to follow immediately, and each phase must independently justify the next.

```text
retrieval-ready content
  -> dogfooded schema
  -> derived retrieval records
  -> narrow retrieval baseline
  -> optional in-memory graph experiment
  -> optional persistent graph projection
  -> optional hybrid retrieval
  -> optional delivery and source topology expansion
```

## Central rule

> The graph points to authoritative knowledge and explains its relationships. It does not become the authoritative knowledge itself.

Deleting the graph must not lose an accepted decision, requirement, study finding, citation, plan, review result, or source artifact. Rebuilding from the current authoritative sources should restore the same deterministic graph wherever the source content has not changed.

## Goals

- Make evidence and decision lineage traversable across UltraPlan stages.
- Preserve explicit provenance for every material node and relationship.
- Distinguish current, historical, superseded, and contradictory knowledge.
- Combine semantic retrieval with deterministic relationship expansion.
- Enable structural validation across requirements, decisions, plans, implementation, and verification.
- Keep the graph read-only and derived in its earliest forms.
- Avoid requiring users or agents to author a second graph-specific representation.
- Add graph concepts only when real queries or validations justify them.
- Keep source-code topology separate from the artifact and reasoning graph until integration is earned.

## Non-goals

The initial graph work must not introduce:

- Neo4j, a hosted graph service, or another operational dependency;
- a universal enterprise ontology;
- a graph database as UltraPlan's product source of truth;
- graph-only edits that do not update authoritative artifacts;
- automatic conversion of every sentence into a node;
- unrestricted LLM-inferred edges treated as fact;
- ingestion of every source symbol, call edge, and import from every repository;
- mandatory graph query languages for users or agents;
- replacement of lexical or semantic retrieval;
- automatic architectural decisions from graph structure;
- an obligation to migrate every historical workspace before graph experiments begin.

## Why a graph may be valuable

Text retrieval answers a relevance question:

> Which pieces of content are likely to help with this query?

A graph can answer relationship and lineage questions:

> How did this evidence affect the project, what was chosen, what implemented it, and what later verified or superseded it?

For example:

```text
REQ-007
  implemented_by
DEC-SPRINT-004
  grounded_by
PAT-OUTCOME-001
  supported_by
EVD-TEMPORAL-014
```

Later delivery evidence may extend the chain:

```text
DEC-SPRINT-004
  implemented_by
CODE-ultraplan-go-internal-run-outcome-Arbiter

DEC-SPRINT-004
  verified_by
TEST-OUTCOME-RACE-003

DEC-SPRINT-004
  reviewed_by
FIND-REVIEW-018
```

This enables end-to-end traceability without forcing UltraPlan to become a heavyweight requirements-management system.

## Graph boundaries

A future implementation should distinguish at least three related but separate projections.

### 1. Artifact and governance graph

Represents projects, studies, sprints, managed artifacts, contracts, protocols, requirements, and explicit selections.

This graph answers:

- What exists?
- Which artifacts were selected for this sprint?
- Which requirements and contracts constrain the work?
- Which artifacts were derived from others?

### 2. Evidence and decision graph

Represents observations, evidence, patterns, trade-offs, assumptions, risks, decisions, rejected alternatives, questions, review findings, and supersession.

This graph answers:

- What supports or contradicts a claim?
- Which evidence materially grounded a decision?
- Which assumptions remain unresolved?
- Which decision is current?

### 3. Source topology graph

Represents repositories, revisions, packages, files, symbols, tests, configuration, and selected structural relationships.

This graph answers:

- Where is a decision implemented?
- Which tests cover a source surface?
- Which callers or interfaces may be affected?

The source topology graph should be deferred. The initial graph should refer only to source files, symbols, and ranges already cited by UltraPlan artifacts. A comprehensive code graph should be designed separately if later evidence shows that it materially improves discovery, planning, review, or change-impact analysis.

## Layered conceptual model

The graph may be understood as four linked layers.

### Governance layer

```text
Requirement
Contract
Protocol
Constraint
```

This layer states what must be true.

### Knowledge layer

```text
Observation
Evidence
Pattern
TradeOff
Recommendation
NotableAbsence
```

This layer records what has been observed or learned.

### Decision layer

```text
Decision
RejectedAlternative
Risk
Assumption
OpenQuestion
RevisitTrigger
```

This layer records what was chosen, why, and under which conditions it should be reconsidered.

### Delivery layer

```text
PlanTask
CodeSurface
Change
TestEvidence
ReviewFinding
SmokeEvidence
```

This layer records how a decision was implemented and verified.

A high-value traversal is:

```text
Requirement
  -> Decision
  -> PlanTask
  -> CodeSurface
  -> TestEvidence
  -> ReviewFinding
```

## Initial node vocabulary

The first graph experiment should use only node kinds already grounded in UltraPlan content.

### Artifact-level nodes

```text
Workspace
Project
Sprint
Study
Dimension
Artifact
Repository
RepositorySnapshot
```

### Semantic nodes

```text
Requirement
Contract
Protocol
Observation
Evidence
Pattern
TradeOff
Decision
RejectedAlternative
Risk
Assumption
OpenQuestion
ReviewFinding
TestEvidence
SmokeEvidence
```

### Lightweight source-reference nodes

```text
SourceFile
SourceSymbol
SourceRange
Commit
```

Do not add a node kind merely because it may someday be useful. Add it only when it has distinct identity, validation rules, traversal behavior, or display needs.

## Initial relationship vocabulary

Begin with the controlled relationship vocabulary already proposed by the retrieval-ready content plan, then extend it cautiously where graph behavior genuinely differs.

### Structural and provenance relationships

```text
contains
derived_from
selects
depends_on
produces
references
```

### Governance and delivery relationships

```text
constrained_by
implements
satisfies
validated_by
verified_by
reviewed_by
```

### Evidence and reasoning relationships

```text
supports
contradicts
informs
rejects
raises
resolves
applies_to
```

### Lifecycle relationships

```text
supersedes
superseded_by
revises
historical_version_of
```

Avoid vague edges such as `related_to`, `mentions`, or `is_relevant_to` in the initial schema. They are difficult to validate and provide little reliable traversal behavior.

A new edge type should be introduced only when UltraPlan needs to query, validate, render, or rank it differently from existing relationships.

## Relationship provenance

A graph relationship must not appear as an unexplained fact.

Each material edge should carry enough provenance to identify why it exists.

A candidate edge record is:

```json
{
  "from": "pattern.outcome.single-terminal-owner",
  "type": "supports",
  "to": "sprint.ultraplan-go.12.decision.004",
  "origin": "explicit",
  "asserted_by": "sprint.ultraplan-go.12.reasoning",
  "location": "Final Decisions > Decision 4",
  "status": "accepted",
  "confidence": "high",
  "source_revision": "sha256:..."
}
```

### Edge origins

Use a controlled origin classification:

```text
explicit
structural
deterministic
inferred
```

- `explicit`: directly declared in authoritative metadata or a semantic block.
- `structural`: derived from containment, artifact paths, or known stage ownership.
- `deterministic`: produced by a defined parser or resolver rule.
- `inferred`: proposed by an LLM or heuristic and not explicitly asserted by the source.

Inferred edges must remain visibly weaker. They should not satisfy governance validation, mark work complete, supersede accepted decisions, or silently enter prompts as authoritative context.

An inferred relationship may be offered for confirmation or used as a low-confidence retrieval lead, but it should not be promoted without an explicit source update or user confirmation.

## Time, revision, and supersession

Temporal state is essential. Historical and current decisions must not be returned as equally authoritative merely because their text is similar.

Material nodes should expose appropriate fields such as:

```text
created_at
valid_from
valid_until
status
artifact_revision
source_snapshot
superseded_by
```

For example:

```text
DEC-004 status = historical
DEC-018 status = accepted
DEC-018 supersedes DEC-004
```

Default retrieval and traversal should prefer current or accepted nodes while preserving the ability to inspect historical reasoning.

A graph build should report unresolved lifecycle problems, including:

- two accepted decisions that claim to supersede the same predecessor incompatibly;
- a current decision depending on a superseded decision without an explicit carry-forward relationship;
- an artifact marked historical but still selected as current governance;
- a relationship whose asserted source revision no longer matches the authoritative artifact.

## Contradiction as a first-class relationship

UltraPlan studies should preserve disagreement rather than flattening it into artificial consensus.

Examples:

```text
OBS-TEMPORAL-003
  contradicts
OBS-OTHER-RUNTIME-011
```

```text
PAT-CENTRAL-SCHEDULER
  rejected_by
DEC-SPRINT-014
```

A contradiction edge should preserve scope. Two observations may disagree about ownership while agreeing about durability or observability.

A candidate contradiction record should include:

```text
scope
reason
asserted_by
confidence
resolution_status
resolved_by
```

The graph must not assume that every contradiction requires a universal resolution. A sprint may choose one approach for its own constraints while both external observations remain valid in their original contexts.

## Representing notable absence cautiously

Absence is not equivalent to non-existence.

A study may report that a capability was not found within the inspected scope. The graph must not convert that into an unconditional negative fact.

Represent notable absence as an observation node with bounded provenance:

```text
NotableAbsence
  subject: temporal
  expected_capability: explicit terminal arbitration component
  observation: not identified
  inspected_scope: [...]
  repository_revision: ...
  confidence: medium
```

The accurate claim is:

> The capability was not identified in this bounded investigation.

It is not:

> The capability does not exist.

Absence nodes should be excluded from high-confidence negative answers unless their scope and confidence travel with the result.

## Hybrid graph and text retrieval

The graph should complement lexical and semantic retrieval rather than replace them.

### When graph traversal works best

- The starting entity is known.
- The relationship type is explicit.
- The user asks for provenance, dependency, implementation, validation, or supersession.

Examples:

```text
Show all evidence supporting DEC-SPRINT-004.
Trace REQ-007 through implementation and review.
Which current decisions depend on this contract?
```

### When text retrieval works best

- The query is conceptual.
- The user does not know the entity name.
- Relevant artifacts use varied terminology.
- The answer may be contained in narrative analysis.

Examples:

```text
How should competing cancellation and completion events resolve?
What patterns exist for durable review resumption?
```

### Candidate hybrid pipeline

```text
1. Parse query intent and available filters.
2. Run metadata and lexical retrieval.
3. Add semantic retrieval only when justified by the retrieval baseline.
4. Resolve candidate chunks to graph nodes.
5. Expand through explicitly allowed edge types and depth limits.
6. Fetch authoritative Markdown and source excerpts for selected nodes.
7. Rerank, diversify, and remove redundant evidence.
8. Return evidence with relationship paths and provenance.
```

Graph expansion must be bounded. It should use query-specific edge allowlists, depth limits, status filters, and authority filters rather than recursively returning an entire connected component.

## Candidate user and agent surfaces

The earliest surface should be diagnostic and read only.

Possible commands:

```bash
ultraplan knowledge inspect <entity-ref>
ultraplan knowledge trace <entity-ref>
ultraplan knowledge validate
ultraplan knowledge explain-path <from> <to>
ultraplan knowledge status
```

Examples:

```bash
ultraplan knowledge trace REQ-007
ultraplan knowledge inspect DEC-SPRINT-004
ultraplan knowledge explain-path REQ-007 FIND-REVIEW-018
```

Potential machine-readable output should include:

- matched entity;
- node type and status;
- authoritative source path and revision;
- traversed edge type;
- edge origin and confidence;
- destination entity;
- concise explanation;
- unresolved or stale references.

Agents should not need to write graph queries. A future agent-facing tool should expose typed operations such as entity lookup, relationship expansion, provenance tracing, and bounded path discovery.

## Structural validation opportunities

A graph projection can enable useful cross-artifact validation that is difficult when artifacts are inspected independently.

Candidate findings include:

- accepted decision without supporting evidence or explicit rationale;
- requirement without an implementing decision;
- decision without a corresponding plan task;
- plan task not linked to a decision or requirement;
- decision marked implemented without a code or artifact reference;
- implementation reference without planned validation;
- review finding that maps to no requirement, decision, or plan task;
- current decision grounded only in superseded or stale evidence;
- source evidence tied to a repository revision different from the declared study snapshot;
- risk with no mitigation, acceptance, or follow-up;
- open question treated as resolved without a resolving decision;
- contradiction that a sprint must resolve but has not addressed;
- verification evidence attached to a historical decision instead of its replacement.

Initial graph validation must report findings without mutating artifacts. Content fixes remain edits to the authoritative Markdown or source metadata.

## Cautious incremental phases

## Graph Phase 0: Explicit relationships only

This is already part of the retrieval-ready content plan.

### Scope

- stable IDs;
- controlled relation vocabulary;
- provenance fields;
- status and supersession;
- revision-aware evidence;
- resolver validation.

### Constraints

- no graph data structure exposed publicly;
- no graph persistence;
- no graph CLI;
- no inferred edges;
- no new operational dependency.

### Exit criteria

- real studies and sprints produce useful explicit relationships;
- relationship authoring burden is acceptable;
- unresolved references can be diagnosed reliably;
- enough real questions require traversal to justify an experiment.

## Graph Phase 1: In-memory artifact graph experiment

Build a workspace graph in memory during a diagnostic command or validation run.

### Scope

- parse existing artifact metadata and semantic blocks;
- create nodes for artifacts and explicitly identified semantic objects;
- create structural, explicit, and deterministic edges;
- support lookup and bounded traversal;
- output stable JSON for evaluation;
- discard the graph at process exit.

### Candidate implementation boundary

A focused package might live under:

```text
internal/knowledgegraph
```

It may own:

- graph node and edge projections;
- deterministic graph building;
- bounded traversal;
- path explanation;
- graph-specific findings.

It must not own:

- Markdown authoring;
- project or sprint persistence;
- source repository mutation;
- semantic retrieval;
- product decisions;
- automatic promotion of inferred relationships.

### Evaluation

Run the graph against the existing retrieval-question corpus and additional graph-specific questions.

Measure:

- graph build time and memory;
- unresolved edge rates;
- duplicate identity rates;
- usefulness of provenance paths;
- whether the graph finds information not easily available through artifact inspection;
- false confidence caused by incomplete relationships;
- authoring changes needed to improve graph quality.

### Exit criteria

- the graph rebuild is deterministic for unchanged content;
- bounded traces answer useful real questions;
- graph findings reveal actionable content-quality problems;
- the in-memory implementation remains simple enough to delete or replace;
- there is no requirement for a graph database.

## Graph Phase 2: Read-only knowledge commands

Expose the validated in-memory projection through diagnostic CLI commands.

### Initial commands

```bash
ultraplan knowledge status
ultraplan knowledge inspect <entity-ref>
ultraplan knowledge trace <entity-ref>
ultraplan knowledge validate
```

### Requirements

- JSON output is stable and versioned;
- human output explains source paths and relationship provenance;
- traversal defaults are bounded and status-aware;
- stale or unresolved references are explicit;
- historical entities are excluded by default unless requested;
- inferred edges remain excluded because none are required yet.

### Exit criteria

- commands are useful during real study and sprint work;
- graph rebuild cost is acceptable for ordinary local workspaces;
- users can understand why every returned relationship exists;
- no persistent graph is needed for acceptable interaction.

## Graph Phase 3: Optional derived graph persistence

Persist the graph only if repeated rebuild cost, query latency, or server usage demonstrates a real need.

### Principles

- persistence is derived cache data;
- the graph is safe to delete and rebuild;
- authoritative artifacts retain ownership of graph meaning;
- index versioning records parser and schema versions;
- changed artifacts rebuild only affected nodes and edges where practical;
- full rebuild remains available and is the correctness baseline;
- one local process must not observe partially updated graph state.

### Candidate storage

SQLite may be sufficient because the early graph is local, bounded, and primarily queried through known traversals. Do not introduce a dedicated graph database without evidence that SQLite or in-memory adjacency structures are inadequate.

The stored projection may use ordinary tables such as:

```text
nodes
edges
edge_provenance
artifact_revisions
build_metadata
findings
```

This is an implementation detail of the derived index, not a product persistence model.

### Exit criteria

- persistent projection materially improves measured behavior;
- rebuild and incremental update semantics are tested;
- corruption can be recovered through deletion and rebuild;
- no user-authored graph state exists only in the index.

## Graph Phase 4: Hybrid retrieval integration

Integrate graph expansion with the narrow retrieval prototype only after both systems have useful independent baselines.

### Initial scope

- retrieve candidate semantic blocks;
- resolve them to graph entities;
- expand through a small query-specific edge allowlist;
- fetch authoritative text for graph neighbors;
- display the relationship path that caused expansion;
- compare retrieval quality with and without graph expansion.

### Initial edge allowlists

Examples:

```text
Decision query:
  grounded_by
  implements
  supersedes
  verified_by

Requirement query:
  implemented_by
  satisfied_by
  validated_by

Evidence query:
  supports
  contradicts
  derived_from
```

### Evaluation

Measure:

- expected evidence recall;
- unsupported-answer rate;
- stale or historical result rate;
- redundant-result rate;
- prompt size;
- path explainability;
- whether graph expansion improves agent outcomes rather than merely returning more context.

### Exit criteria

- graph expansion improves a defined set of retrieval questions;
- every expanded result has a clear provenance path;
- context growth remains bounded;
- explicit governance selections still outrank similarity or graph proximity;
- retrieval remains useful when graph expansion is disabled.

## Graph Phase 5: Delivery traceability

Connect accepted decisions to planned work, implementation surfaces, tests, reviews, and smoke evidence.

### Candidate relationships

```text
Requirement implements Decision
Decision produces PlanTask
PlanTask changes CodeSurface
CodeSurface covered_by TestEvidence
Decision verified_by ReviewFinding
Sprint validated_by SmokeEvidence
```

Use direction names consistently in the final schema. The examples above are conceptual and should be normalized before implementation.

### Uses

- identify requirements with no implementation path;
- identify decisions with no verification evidence;
- explain why a source file changed;
- determine which decisions a failed test may affect;
- prepare focused review context;
- trace a review finding back to requirements and evidence;
- identify historical code surfaces linked only to superseded decisions.

### Constraints

- relationships must come from plan, execution, review, and smoke artifacts or deterministic change evidence;
- Git diff proximity alone must not establish semantic implementation;
- an LLM may suggest links for confirmation but may not silently assert delivery completion;
- review and smoke stages remain authoritative for their own evidence.

### Exit criteria

- traceability reveals real gaps in completed sprints;
- links are maintainable without excessive agent boilerplate;
- review agents can use the graph without treating it as proof;
- the delivery graph improves focused verification or impact analysis.

## Graph Phase 6: Optional source topology integration

Consider a deeper code graph only after delivery traceability has been used successfully.

### Possible node kinds

```text
Package
Module
File
Type
Interface
Function
Method
Test
ConfigKey
CLICommand
EventType
Schema
```

### Possible relationships

```text
imports
calls
implements
constructs
reads
writes
tests
configures
exposes
publishes
consumes
```

### Constraints

- source topology remains revision-specific;
- parsers should be deterministic and language-aware where possible;
- the graph should distinguish static evidence from runtime evidence;
- symbol identities must tolerate line movement;
- large repository indexing must be incremental and bounded;
- source topology must remain a separate projection linked to the artifact graph;
- a full multi-language code graph is not a prerequisite for useful knowledge traversal.

### Exit criteria

- a measured use case cannot be solved adequately through lexical search, semantic retrieval, and cited source references;
- the target languages and repositories are clearly scoped;
- symbol extraction quality is sufficient for trusted navigation;
- the implementation does not force every study source repository into a heavyweight indexing lifecycle.

## Deferred decisions

Do not decide these until a phase requires them:

- property graph versus RDF-style representation;
- SQLite adjacency tables versus another derived store;
- graph query language;
- graph visualization UI;
- automatic entity resolution across workspaces;
- cross-workspace or organization-wide graph federation;
- inference model or confidence-calibration strategy;
- source graph parser framework;
- hosted graph service;
- graph synchronization in a multi-user cloud environment;
- graph embeddings or graph neural networks.

## Risks and mitigations

### Risk: ontology expansion becomes the product

Mitigation:

- use a small vocabulary;
- require a real query, validation, or rendering need for new types;
- version schema changes;
- remove unused node and edge kinds before they become compatibility commitments.

### Risk: graph completeness creates false confidence

Mitigation:

- treat the graph as a projection, not a complete worldview;
- display provenance and unresolved references;
- distinguish explicit from inferred edges;
- preserve authoritative source text in all outputs;
- report coverage and build findings.

### Risk: stale decisions outrank current knowledge

Mitigation:

- model status and supersession explicitly;
- default queries to current entities;
- attach artifact and repository revisions;
- validate lifecycle contradictions.

### Risk: relationship authoring adds excessive boilerplate

Mitigation:

- derive structural edges automatically;
- require explicit edges only for material semantic relationships;
- reuse IDs already needed for retrieval and validation;
- dogfood before making fields mandatory.

### Risk: graph expansion bloats prompts

Mitigation:

- use edge allowlists, depth limits, authority filters, and result budgets;
- rerank after expansion;
- fetch authoritative text only for selected nodes;
- compare against retrieval without graph expansion.

### Risk: inferred relationships are mistaken for facts

Mitigation:

- label origin and confidence;
- exclude inferred edges from governance validation;
- require confirmation before promotion;
- never allow inference alone to mark implementation or verification complete.

### Risk: source topology scope becomes unbounded

Mitigation:

- begin with cited source references only;
- keep source topology separate;
- index only selected implementation repositories and revisions;
- add language support incrementally;
- prove value before broad ingestion.

## Testing strategy

### Unit tests

- stable node identity;
- stable edge identity;
- relationship vocabulary validation;
- explicit, structural, deterministic, and inferred origin handling;
- status and supersession filtering;
- contradiction representation;
- bounded notable-absence behavior;
- revision and content-hash handling;
- deterministic traversal ordering;
- depth and result limits.

### Integration tests

- build a graph from representative workspace fixtures;
- trace requirement to decision to plan to review;
- resolve evidence and source references;
- report broken relationships;
- exclude superseded nodes by default;
- include historical nodes when requested;
- rebuild unchanged content to the same projection;
- update one artifact and rebuild only affected graph portions where incremental indexing exists.

### Evaluation fixtures

Add graph-oriented questions to the retrieval-question corpus, including:

- What evidence supports this decision?
- Which requirement does this plan task implement?
- Which current decision superseded the filesystem-only persistence choice?
- Which review finding contradicted an implementation assumption?
- Which accepted decisions have no verification evidence?
- Which studies materially influenced this sprint?
- Which source references are tied to stale repository revisions?
- Where do two studied repositories disagree about cancellation ownership?

Each question should declare expected nodes, edges, authoritative source artifacts, and acceptable historical variants.

## Documentation expectations

If graph work begins, document:

- the graph is derived and disposable;
- authoritative sources remain Markdown and source repositories;
- node and edge vocabulary;
- edge origin and confidence;
- status and supersession behavior;
- notable-absence limitations;
- rebuild and staleness behavior;
- command output contracts;
- graph coverage limitations;
- how retrieval uses bounded graph expansion;
- how users repair graph findings through source artifact edits.

## Overall exit criteria for pursuing a knowledge graph

Proceed beyond an in-memory experiment only when all of the following are true:

- retrieval-ready content has been dogfooded successfully;
- stable IDs and relationships are present in real artifacts;
- a narrow retrieval baseline exists;
- real questions require multi-hop provenance or traceability;
- the graph answers those questions more reliably than direct artifact inspection alone;
- provenance prevents relationships from becoming unsupported assertions;
- rebuild behavior is deterministic;
- authoring burden is acceptable;
- the graph remains useful without becoming authoritative product storage;
- a dedicated graph database is still optional rather than assumed.

## Long-term possibility

The most compelling eventual shape is a living evidence-and-decision graph for software delivery:

```text
external evidence
      -> project interpretation
      -> accepted decision
      -> planned work
      -> implementation
      -> verification
      -> later learning
```

Such a graph could allow UltraPlan to explain not only what should be done, but why the software is the way it is, which evidence shaped it, how that reasoning was implemented, and whether later work confirmed, contradicted, or superseded it.

That possibility is valuable, but it should remain downstream of the immediate work: improve the content, establish trustworthy retrieval, observe real usage, and earn each additional graph capability through demonstrated need.