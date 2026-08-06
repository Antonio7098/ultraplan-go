# Plan: Retrieval-Ready UltraPlan Content

## Summary

Evolve UltraPlan's Markdown artifacts so they are easier to search, filter, retrieve, cite, and combine in future agent workflows without prematurely building a RAG subsystem, embedding store, retrieval database, or universal artifact model.

The immediate goal is not to add semantic search. It is to make the content UltraPlan already produces more consistently identifiable, self-contained, provenance-aware, and divisible into meaningful semantic units.

The work should proceed cautiously:

1. measure the current content before changing it;
2. define a small content contract around existing artifacts;
3. introduce optional metadata and stable semantic blocks;
4. validate new structure initially as warnings;
5. apply the contract to newly generated artifacts before migrating old ones;
6. dogfood the structure on real studies and sprints;
7. only then design a derived retrieval index around observed needs.

Existing Markdown remains the authoritative, human-editable representation throughout. Any future retrieval index must be derived, disposable, and rebuildable.

## Motivation

UltraPlan already produces valuable structured content:

- study source reports;
- final dimension reports;
- project indexes;
- sprint indexes;
- technical handbooks;
- area-specific reasoning;
- final sprint reasoning;
- sprint plans;
- reviews and smoke reports;
- contracts, protocols, and project documentation;
- curated source excerpts such as the planned `code-context.md` artifact.

The current templates already use stable headings, evidence tables, confidence fields, trade-offs, decisions, risks, and handoff sections. This gives UltraPlan a strong base for retrieval.

However, future retrieval would still face several avoidable problems:

- document identity is mostly inferred from paths and headings;
- artifact status and authority are not represented consistently;
- citations do not always identify repository revision and precise range;
- long sections may combine several unrelated claims;
- facts, interpretations, and recommendations can be mixed together;
- relationships between requirements, evidence, decisions, plans, and reviews are mostly implicit;
- superseded material may remain lexically or semantically similar to current decisions;
- chunking would otherwise depend on arbitrary token windows;
- an indexer would need artifact-specific heuristics for every template.

Improving these properties now makes the content more useful even without retrieval. It also avoids designing a future index around inconsistent historical output.

## Central rule

> Optimize UltraPlan artifacts for semantic units, provenance, and governed relationships, not for a particular vector database, embedding model, or RAG framework.

## Goals

- Preserve human-readable, Git-friendly Markdown as the canonical content format.
- Give managed artifacts stable identity independent of file path and title changes.
- Make artifact type, lifecycle status, authority, scope, and provenance explicit.
- Make important claims and decisions retrievable as self-contained semantic units.
- Improve source evidence so it is repository- and revision-aware.
- Make relationships between requirements, evidence, decisions, implementation, and validation explicit.
- Allow future retrieval to filter deterministically before lexical or semantic ranking.
- Support deterministic, structure-aware chunking without arbitrary fixed-token splits.
- Introduce changes incrementally without invalidating existing workspaces.
- Gather evidence from real studies and sprints before committing to retrieval infrastructure.

## Non-goals

This plan does not introduce:

- a vector database;
- embeddings;
- semantic search;
- a retrieval CLI;
- a source-code index;
- a symbol graph;
- automated question answering;
- agent tool calls for retrieval;
- a universal `Artifact` domain abstraction across all product modules;
- a second authoritative JSON representation for every Markdown file;
- mandatory migration of all existing workspace artifacts;
- automatic rewriting of customized workspace templates;
- strict content-size limits;
- a large ontology or unrestricted tagging system;
- generated metadata that users must maintain manually.

Those capabilities should be considered separately after the content contract has been used and evaluated.

## Design principles

### 1. Markdown remains authoritative

UltraPlan should continue to treat the human-editable Markdown artifact as the source of truth. Metadata belongs in a small frontmatter envelope and important structure belongs in ordinary Markdown headings, fields, tables, and lists.

Do not require users or agents to maintain a parallel JSON document containing the same content.

### 2. Add structure only where it earns its cost

Not every paragraph needs an ID or typed schema. Stable identifiers should be reserved for semantic objects that are likely to be cited, related, superseded, validated, or retrieved independently:

- requirements;
- contracts;
- evidence records;
- observations;
- patterns;
- decisions;
- risks and assumptions;
- review findings;
- open questions where they are carried across stages.

Ordinary narrative prose can remain ordinary prose.

### 3. Compatibility before purity

Existing artifacts without metadata must remain readable and valid. New validators should begin with advisory findings. Strict requirements should be introduced only after templates, prompts, documentation, and real workspaces have demonstrated that the structure is stable and useful.

### 4. Retrieval metadata is not product persistence

Artifact metadata describes content for discovery and provenance. It must not force project, sprint, study, and run packages behind one generic storage abstraction.

Future retrieval records are derived cache data and should remain separate from both product persistence and repository source state.

### 5. Explicit governance outranks similarity

A future retrieval system may suggest relevant reports, contracts, decisions, or protocols, but explicit selections in `project-index.md` and `sprint-index.md` remain authoritative for governed sprint context.

### 6. Provenance travels with the claim

A retrieved claim should not require loading an entire report merely to discover whether it is an observation, inference, decision, or recommendation and what evidence supports it.

### 7. Do not design the final vocabulary up front

Start with a small controlled vocabulary, observe real usage, and extend it through versioned schema changes. Avoid a broad ontology before retrieval questions and failure cases are known.

## Proposed content model

The content model has three levels.

```text
Artifact
  metadata envelope
  retrieval summary
  semantic sections
    typed blocks where useful
      explicit evidence and relationships
```

A future indexer may convert these into derived retrieval records:

```text
Markdown artifact
  -> parse
  -> identify semantic blocks
  -> inherit artifact metadata
  -> resolve relationships
  -> emit derived chunks
```

The derived records are not part of the first implementation.

## Phase 0: Inventory and retrieval-question corpus

Before changing templates, inspect representative current artifacts and define what future retrieval must answer.

### Scope

Sample at least:

- several source reports from different study dimensions;
- several final reports;
- one small and one large project index;
- multiple sprint indexes;
- technical handbooks;
- area reasoning documents;
- final reasoning documents;
- plans, reviews, and smoke reports;
- contracts and protocols;
- any existing decision logs;
- the first real `code-context.md` artifacts once that stage exists.

### Deliverables

Add a planning-only fixture or document containing representative retrieval questions, for example:

- Which reports discuss terminal outcome arbitration?
- What accepted decision currently governs persistence boundaries?
- Which source evidence supports cancellation being distinct from failure?
- What requirements are implemented by a named sprint decision?
- Which review finding validated or contradicted that decision?
- What studies are relevant to public API compatibility?
- Which current implementation files own flow-state transitions?
- Which earlier decision has been superseded?

Classify each question by the expected answer source:

- exact metadata filtering;
- lexical search;
- semantic similarity;
- relationship traversal;
- source-code lookup;
- a combination of these.

### Measurements

Record:

- section sizes;
- heading consistency;
- duplicate titles;
- common citation formats;
- how often one section contains multiple independent claims;
- how often paths or titles change;
- how decisions refer to requirements and evidence;
- how superseded content is currently represented;
- the percentage of reports with precise file ranges;
- the percentage of source evidence tied to a revision.

### Exit criteria

- A representative artifact sample has been reviewed.
- At least twenty real retrieval questions have expected source artifacts.
- The team can identify which problems require content changes and which belong to a later retrieval engine.
- No production template or validator has changed yet.

## Phase 1: Define the minimal artifact envelope

Introduce a versioned metadata contract, but do not require every artifact to use it immediately.

### Proposed frontmatter

Use YAML frontmatter because UltraPlan already depends on YAML and the representation remains readable and editable.

```yaml
---
schema: ultraplan.artifact/v1
id: study.agent-harness-study.dimension.01.11.final
type: study.final-report
title: Terminal Outcome Arbitration
status: final
authority: synthesized

study: agent-harness-study
dimension: "01.11"
project: null
sprint: null

topics:
  - lifecycle
  - terminal-outcomes
  - cancellation

created_at: 2026-08-06
derived_from:
  - study.agent-harness-study.dimension.01.11.source.temporal
supersedes: []
---
```

### Required initial fields

For artifacts that opt into `ultraplan.artifact/v1`, require only:

- `schema`;
- `id`;
- `type`;
- `title`;
- `status`;
- `authority`.

All other fields should be optional and type-specific.

Do not require users or agents to write `content_revision`. A future indexer can compute content hashes deterministically.

### Initial artifact types

Start with only types already produced or planned by UltraPlan:

```text
study.source-report
study.final-report
project.index
sprint.requirements
sprint.code-context
sprint.index
sprint.technical-handbook
sprint.area-reasoning
sprint.reasoning
sprint.plan
sprint.review
sprint.smoke
project.contract
project.reasoning-template
protocol
source-document
decision-log
```

Keep the vocabulary in one versioned package or schema definition rather than duplicating string validation across modules.

Do not model every artifact through a shared runtime interface. Shared schema parsing may live in a focused content package while each product module remains responsible for its own artifacts.

### Initial statuses

Use a small set:

```text
draft
current
final
superseded
archived
```

Artifact-specific validators may restrict which statuses are meaningful.

### Initial authorities

Use a compact set:

```text
normative
observational
synthesized
decisive
procedural
advisory
historical
```

Document the meaning clearly:

- `normative`: requirement, contract, or protocol;
- `observational`: source or runtime evidence;
- `synthesized`: cross-source analysis or handbook;
- `decisive`: accepted project or sprint decision;
- `procedural`: plan or operating procedure;
- `advisory`: recommendation or exploratory reasoning;
- `historical`: superseded material kept for provenance.

### Stable ID rules

IDs should be:

- workspace-unique;
- deterministic from stable domain references where possible;
- lowercase ASCII;
- dot-separated;
- independent of the current absolute filesystem location;
- preserved when a title changes;
- changed only when the semantic identity changes.

Examples:

```text
study.agent-harness-study.dimension.01.11.source.temporal
study.agent-harness-study.dimension.01.11.final
project.ultraplan-go.index
sprint.ultraplan-go.12.requirements
sprint.ultraplan-go.12.reasoning
contract.runtime.cancellation
protocol.review.public-api
```

Do not introduce opaque UUIDs unless later database-backed identity or collision evidence requires them.

### Implementation boundary

Add a small parser and validator, likely under a focused package such as:

```text
internal/contentmeta
```

Responsibilities:

- parse optional frontmatter;
- preserve body content exactly;
- validate the versioned envelope;
- expose normalized metadata to callers;
- return typed findings rather than terminating processes;
- reject unsafe YAML constructs or unsupported schema versions cleanly.

It should not:

- discover every workspace artifact;
- own project or sprint validation;
- write artifacts;
- build an index;
- know retrieval ranking;
- define product persistence.

### Compatibility behavior

- Files without frontmatter continue to parse as legacy artifacts.
- Existing validators continue to validate their current body structure.
- Metadata validation findings are warnings by default.
- Unknown frontmatter keys are preserved and ignored unless they violate YAML safety or conflict with reserved fields.
- Unsupported future schema versions produce a clear advisory or validation failure depending on the command context.

### Exit criteria

- The metadata parser has focused unit tests.
- Existing artifacts without frontmatter still validate exactly as before.
- A small set of newly generated fixture artifacts includes valid frontmatter.
- No existing customized workspace template is overwritten.
- No retrieval index exists.

## Phase 2: Add retrieval summaries to high-value artifacts

Add a short, structured retrieval summary near the top of artifacts whose purpose is broader than a single obvious requirement or task.

### Target artifacts

Begin with:

- final study reports;
- source reports;
- technical handbooks;
- area reasoning;
- final sprint reasoning;
- project indexes;
- contracts and protocols.

Do not add it automatically to every plan task or tiny artifact.

### Proposed structure

```markdown
## Retrieval Summary

**Answers**

- How should competing terminal outcomes be arbitrated?
- Which component should publish the final run state?

**Use When**

- Designing lifecycle state machines.
- Reviewing cancellation and timeout behavior.

**Not Intended For**

- General provider retry policy.
- Workflow scheduling.

**Core Terms**

`terminal outcome`, `outcome arbitration`, `cancellation race`, `final state`
```

### Rules

- Keep it concise.
- Describe questions the artifact actually answers.
- Include applicability and exclusions where confusion is likely.
- Do not create large generated keyword lists.
- Prefer canonical domain language plus a few genuine aliases.
- Do not repeat the executive summary verbatim.

### Rollout

1. Add retrieval summaries to embedded templates.
2. Update corresponding prompts to generate them.
3. Update template validation to warn when the section is absent in new-schema artifacts.
4. Leave legacy artifacts untouched.
5. Evaluate usefulness over several real study and sprint runs.

### Exit criteria

- Retrieval summaries are consistently concise and useful in dogfooded artifacts.
- They improve manual discovery without adding substantial document noise.
- Agents do not fill the section with generic or invented keywords.
- Absence remains non-fatal during this phase.

## Phase 3: Introduce typed semantic blocks selectively

Convert the highest-value repeated concepts from large prose sections into stable, self-contained blocks.

### Candidate block kinds

Begin with:

```text
requirement
evidence
observation
pattern
decision
risk
assumption
review-finding
open-question
```

Do not introduce a generic syntax for every paragraph.

### Block representation

Use normal Markdown headings and labeled fields.

```markdown
### PAT-OUTCOME-001: Single Terminal Outcome Owner

**Type:** pattern
**Claim:** One component owns publication of the final execution outcome.
**Confidence:** high

#### Applies When

Multiple asynchronous paths can complete, fail, cancel, or time out the same run.

#### Mechanism

Terminal candidates pass through one arbitration boundary.

#### Trade-Offs

A central coordination point is introduced and precedence rules must be explicit.

#### Evidence

- `EVD-TEMPORAL-014`
- `EVD-OPENHANDS-021`

#### Related

- `DEC-SPRINT-004`
- `RISK-LATE-CANCELLATION`
```

### ID scope

Use IDs only where cross-reference is valuable.

Recommended prefixes:

```text
REQ-
CON-
EVD-
OBS-
PAT-
DEC-
RISK-
ASM-
FIND-
Q-
```

The prefix communicates block kind, but the parsed `Type` remains authoritative.

IDs should be unique within the workspace when practical. At minimum they must be unique within the artifact and unambiguous when qualified by artifact ID.

### Separate observation, interpretation, and implication

Source analysis should avoid mixing these in one paragraph.

```markdown
### OBS-TEMPORAL-008: Cancellation Does Not Publish Completion Directly

**Type:** observation
**Evidence:** `EVD-TEMPORAL-014`
**Confidence:** medium

#### Observation

[What the source verifiably does.]

#### Interpretation

[What the analyst infers from that implementation.]

#### Potential UltraPlan Implication

[What may be relevant, without turning the observation into a decision.]
```

This structure should be applied where the distinction matters, not mechanically to every source note.

### Initial template changes

Prioritize:

1. requirements;
2. source-report evidence and observations;
3. final-report patterns;
4. final sprint decisions;
5. review findings.

Delay large-scale restructuring of plans, handbooks, and all narrative sections until the first blocks have been evaluated.

### Exit criteria

- Important semantic units can be understood when retrieved independently.
- Evidence and applicability remain in the same block as the claim.
- The documents remain pleasant to read as Markdown.
- Agents do not create IDs for trivial prose.
- Validators can detect duplicate or malformed IDs.

## Phase 4: Make source evidence revision-aware

Improve evidence references before relying on them for retrieval.

### Current limitation

A path and line number alone does not identify:

- the repository;
- the revision;
- whether the line number is a range;
- the symbol or logical unit;
- the evidence kind;
- which claim the evidence supports.

### Proposed evidence block

```markdown
### EVD-TEMPORAL-014: Workflow Completion Arbitration

**Type:** evidence
**Repository:** `temporalio/temporal`
**Revision:** `4e73bc7`
**Path:** `service/history/workflow/context.go`
**Lines:** `412-486`
**Symbol:** `ContextImpl.UpdateWorkflowExecutionAsActive`
**Evidence Kind:** implementation
**Supports:** `OBS-TEMPORAL-008`, `PAT-OUTCOME-001`
**Why Relevant:** Coordinates final state mutation and persistence.
```

### Revision rules

For cloned study repositories:

- record the resolved commit SHA used during analysis;
- record repository identity separately from local path;
- use repository-relative paths;
- use inclusive line ranges;
- include a symbol where reliably available;
- do not claim a symbol if it was inferred unreliably.

For a live project implementation repository:

- record the current commit SHA;
- record whether the working tree was clean or dirty;
- if dirty evidence must be durable, compute a source snapshot or content hash later rather than pretending the commit identifies local changes;
- surface staleness rather than silently accepting it.

For non-repository document sources:

- use source artifact ID and content revision or file hash when available;
- preserve page, heading, line, or section locators appropriate to that format.

### Evidence normalization

Do not require agents to hand-format every locator perfectly in the first version. Prefer one of these incremental approaches:

1. prompts generate the richer evidence block;
2. validators normalize simple legacy `path:line` references where context makes repository and revision unambiguous;
3. a future evidence extraction command can emit canonical records from artifact references.

### Compatibility

- Legacy `path:line` remains supported.
- New-schema source reports should warn when revision metadata is missing.
- Final reports may continue to reference source-report evidence IDs rather than repeating every repository locator.
- Do not duplicate large source snippets merely to satisfy metadata.

### Exit criteria

- New study source reports identify repository revision.
- Evidence IDs resolve to precise source locations or return explicit unresolved findings.
- Final reports can trace material claims through source-report evidence records.
- Dirty project repository evidence cannot be mistaken for clean commit evidence.

## Phase 5: Add controlled relationships

Make the existing UltraPlan artifact chain explicit without building a general-purpose knowledge graph.

### Initial relation vocabulary

```text
derived_from
selects
depends_on
constrained_by
implements
satisfies
supports
contradicts
supersedes
validated_by
produces
references
```

Keep the vocabulary versioned and documented. Add new relations only when real artifacts cannot express a useful relationship with the existing set.

### Artifact-level relationships

Represent broad provenance in frontmatter:

```yaml
relations:
  selects:
    - study.agent-harness-study.dimension.01.11.final
  constrained_by:
    - contract.runtime.cancellation
  produces:
    - sprint.ultraplan-go.12.technical-handbook
```

### Block-level relationships

Represent fine-grained links in block fields:

```markdown
**Implements:** `REQ-LIFECYCLE-007`
**Supported By:** `EVD-TEMPORAL-014`
**Validated By:** `FIND-REVIEW-006`
**Supersedes:** `DEC-SPRINT-002`
```

### Index ownership

- `project-index.md` remains the available context pool.
- `sprint-index.md` remains the governed selected context.
- Retrieval-assisted discovery may eventually propose candidates but cannot silently modify these selections.
- A selection should record why the artifact was selected and which question it is expected to answer.

### Reference resolution

Add a workspace-level reference resolver only after IDs are present in enough artifacts to justify it.

The resolver should:

- discover managed artifact IDs through existing project and study services where possible;
- detect duplicates;
- resolve qualified block references;
- report missing targets;
- avoid independently reimplementing project or sprint discovery rules.

### Exit criteria

- Decisions can be traced to requirements and evidence.
- Review findings can validate or contradict decisions.
- Superseded decisions are clearly distinguishable from current decisions.
- Project and sprint indexes preserve their governance role.
- No graph database is required.

## Phase 6: Retrieval-oriented validation and authoring feedback

Introduce validation gradually, with a clear distinction between structural errors and content-quality warnings.

### Initial errors

For artifacts that declare `ultraplan.artifact/v1`:

- malformed YAML frontmatter;
- unsupported or invalid schema syntax;
- missing required metadata fields;
- invalid artifact type, status, or authority;
- duplicate artifact ID in the same validation scope;
- duplicate block ID within an artifact;
- malformed explicit line ranges;
- unsafe absolute or escaping repository paths where repository-relative paths are required.

### Initial warnings

- missing retrieval summary on a high-value artifact;
- excessively large section with no semantic subheadings;
- vague headings such as `Other`, `Notes`, or `Analysis`;
- accepted decision without rationale;
- decision without requirement or evidence grounding where grounding is expected;
- evidence without repository revision;
- unresolved relationship target;
- superseded artifact not marked historical or superseded;
- observation mixing recommendation language without a clear distinction;
- block too dependent on unexplained pronouns or previous context;
- generated core terms that do not appear in or accurately describe the artifact.

### Warning rollout

Use staged enforcement:

1. report warnings only in explicit validation output;
2. include warning counts in status;
3. dogfood and tune false positives;
4. make selected warnings blocking only for newly generated artifacts where the rule has proven stable;
5. preserve a compatibility mode for legacy workspaces.

Do not make stylistic heuristics hard failures.

### Prompt support

Prompts should explain the semantic purpose of the structure rather than merely demand fields. Agents should be instructed to:

- write one principal claim per typed block;
- keep applicability, limitations, and evidence with the claim;
- avoid inventing relationships;
- use stable IDs sparingly;
- distinguish observation from inference and decision;
- preserve explicit uncertainty.

### Exit criteria

- Validators catch broken identity and provenance reliably.
- Warning false-positive rates are acceptable on real artifacts.
- Validation output explains how to repair findings.
- Existing legacy workspaces remain usable.

## Phase 7: Dogfood and revise the schema

Use the content contract on real work before committing to retrieval infrastructure.

### Required dogfooding

At minimum:

- one substantial multi-repository study;
- one study with document sources as well as code repositories;
- one full project sprint through requirements, code context, index, handbook, reasoning, plan, execute, review, and smoke;
- one sprint that carries forward a prior decision;
- one case where a decision is superseded;
- one case where review evidence contradicts an assumption or decision.

### Evaluate

- Does metadata remain accurate after files move or titles change?
- Do agents create stable IDs consistently?
- Are typed blocks too verbose?
- Can a block be understood independently?
- Does revision-aware evidence materially improve trust?
- Are relationships useful or burdensome?
- Which metadata fields are never queried?
- Which missing fields repeatedly require inference?
- Do retrieval summaries improve manual discovery?
- How much prompt and token overhead has been added?
- Does the additional structure reduce or increase report quality?

### Schema revision

If changes are necessary:

- update the schema version deliberately;
- document compatibility;
- provide a mechanical migration only for fields that can be transformed safely;
- do not use an LLM to silently rewrite accepted decisions or evidence;
- preserve unknown fields and human customizations.

### Exit criteria

- The content contract has survived real studies and sprint workflows.
- The stable fields and relations are justified by observed use.
- Major authoring pain points are resolved.
- There is evidence that retrieval infrastructure would now solve concrete discovery problems.

## Phase 8: Design the derived retrieval record format

Only after dogfooding should UltraPlan define how artifacts become retrieval records.

### Derived-record principles

- Records are generated from authoritative artifacts and source repositories.
- Records are safe to delete and rebuild.
- Chunk identity derives from artifact ID, semantic block ID or heading path, schema version, and content hash.
- Metadata is inherited from the artifact and augmented with block-specific fields.
- Original text is preserved exactly in the record.
- Generated search context is separate from the original text.
- An index version records parser, chunker, and later embedding model versions.

### Semantic chunking rules

Prefer:

| Content | Retrieval unit |
| --- | --- |
| Requirement | one requirement block |
| Evidence | one evidence record |
| Observation | one observation block |
| Pattern | one pattern block |
| Decision | one complete decision |
| Risk or assumption | one record |
| Review | one finding |
| Plan | one task or work package |
| Code context | one selected symbol or logical range |
| Narrative report prose | one meaningful subsection |
| Table | one row plus table and heading context |

Avoid blind fixed-token windows as the primary chunking strategy. Token limits may be used only to split unusually large semantic blocks while preserving parent relationships.

### Candidate record

```json
{
  "artifact_id": "study.agent-harness-study.dimension.01.11.final",
  "chunk_id": "PAT-OUTCOME-001",
  "artifact_type": "study.final-report",
  "status": "final",
  "authority": "synthesized",
  "heading_path": [
    "Pattern Catalog",
    "Single Terminal Outcome Owner"
  ],
  "block_type": "pattern",
  "topics": [
    "terminal-outcomes",
    "cancellation"
  ],
  "evidence_refs": [
    "EVD-TEMPORAL-014"
  ],
  "text": "...",
  "content_hash": "sha256:..."
}
```

### Deferred decisions

Do not choose in this phase unless required by a concrete prototype:

- SQLite versus a dedicated search engine;
- local versus hosted embeddings;
- embedding model;
- approximate nearest-neighbor implementation;
- graph database;
- reranker model;
- query-generation agent;
- index synchronization protocol.

### Exit criteria

- A derived-record schema can represent all dogfooded artifact types.
- Chunking is deterministic and structure-aware.
- Rebuilding produces stable IDs for unchanged content.
- The record format does not become a second authoring surface.

## Phase 9: Optional retrieval prototype

A retrieval prototype may be planned separately after Phase 8.

The likely first prototype should be deliberately narrow:

- local only;
- read only;
- study final reports and source reports first;
- metadata filtering plus lexical search;
- exact evidence output rather than generated answers;
- no embeddings until lexical retrieval has a measured baseline;
- no automatic prompt injection into planning or execution;
- query and result logging for evaluation;
- explicit index status and staleness reporting.

This phase is intentionally not specified in detail here. It should be shaped by the retrieval-question corpus and dogfooding evidence rather than by assumptions made during content restructuring.

## Template-by-template migration order

Apply changes in this order to limit blast radius.

### 1. Source report

Files:

```text
internal/workspace/scaffold/templates/repo-analysis.md
internal/workspace/scaffold/prompts/base.md
```

Changes:

- optional metadata envelope;
- retrieval summary;
- revision-aware source metadata;
- evidence IDs;
- clearer observation versus interpretation structure where material.

Why first:

Source reports provide the provenance foundation for final reports.

### 2. Final study report

Files:

```text
internal/workspace/scaffold/templates/report.md
internal/workspace/scaffold/prompts/synthesize.md
```

Changes:

- optional metadata envelope;
- retrieval summary;
- typed patterns;
- explicit links to source-report evidence IDs;
- status and authority.

### 3. Requirements

Changes:

- stable requirement IDs;
- optional artifact metadata;
- explicit non-goals and acceptance evidence;
- no attempt to turn every sentence into a requirement object.

### 4. Sprint reasoning

Files:

```text
internal/workspace/scaffold/templates/sprint-reasoning.md
internal/workspace/scaffold/prompts/create-sprint-reasoning.md
```

Changes:

- stable decision IDs;
- explicit implementation and evidence relationships;
- accepted, deferred, or superseded status;
- revisit triggers;
- artifact metadata.

### 5. Review and smoke

Changes:

- stable finding IDs;
- explicit decision or requirement targets;
- supports, contradicts, or validates relationships;
- evidence locators.

### 6. Project and sprint indexes

Files:

```text
internal/workspace/scaffold/templates/project-index.md
internal/workspace/scaffold/templates/sprint-index.md
```

Changes:

- selected artifact IDs alongside paths;
- questions each selection should answer;
- selection reason;
- authority and status;
- explicit relationship to selected outputs.

Keep paths because they remain useful and human-readable. IDs supplement paths rather than replacing them.

### 7. Technical handbook and area reasoning

Apply only after source and final reports have stable references. Avoid generating handbook IDs that point to unstable evidence structures.

### 8. Plans and procedural artifacts

Plans should remain task-oriented. Add artifact metadata and decision/requirement references, but do not over-structure implementation prose unless retrieval testing shows a clear benefit.

## Migration strategy for existing workspaces

### Default behavior

- Existing artifacts remain legacy-valid.
- New embedded templates generate the newer structure after the relevant phase lands.
- Customized workspace templates are never overwritten automatically.
- `defaults install` continues to preserve customized files unless overwrite is explicitly confirmed.

### Suggested commands later

A future migration command may provide:

```bash
ultraplan content inspect
ultraplan content validate
ultraplan content migrate --dry-run
```

Do not implement these commands until the metadata parser and template rollout prove that users need them.

### Safe mechanical migration

Mechanical migration may safely add:

- schema version;
- deterministic artifact ID inferred from unambiguous workspace location;
- artifact type;
- current status inferred from known artifact role;
- title copied from the first heading.

Mechanical migration must not invent:

- topics;
- confidence;
- evidence relationships;
- supersession;
- accepted decisions;
- observation or inference boundaries;
- repository revision when it was not recorded.

Those require explicit review or regeneration.

## Testing strategy

### Metadata parsing

- no-frontmatter legacy files;
- valid v1 frontmatter;
- malformed YAML;
- unsupported schema version;
- unknown keys preserved;
- frontmatter body boundary preserved exactly;
- duplicate required fields rejected;
- safe parsing limits.

### Artifact validation

- required fields by artifact type;
- valid and invalid status/authority combinations;
- stable ID syntax;
- duplicate artifact and block IDs;
- malformed relations;
- missing relation targets;
- superseded status handling.

### Template and prompt tests

- generated templates contain the expected envelope placeholders;
- prompts explain the purpose of metadata and typed blocks;
- legacy template overrides still win over embedded defaults;
- prompt previews remain deterministic;
- metadata does not add timestamps or other unstable content to shared prompt-cache prefixes unless already part of the artifact.

### Study flow tests

- source reports record repository revision;
- final reports preserve source evidence traceability;
- existing studies without metadata still run and synthesize;
- summary and code extraction continue to work.

### Sprint flow tests

- existing legacy sprint artifacts validate;
- new requirements and reasoning IDs resolve;
- selected project/sprint index entries resolve by ID and path;
- review findings can target decisions and requirements;
- superseded decisions are not treated as current by status queries.

### Golden fixtures

Maintain a small set of representative artifacts for:

- legacy format;
- minimal v1 metadata;
- fully structured v1 report;
- invalid metadata;
- unresolved relations;
- superseded decision chain;
- dirty-repository source evidence.

Avoid huge golden files that make intentional template evolution difficult to review.

## Documentation changes

Update documentation incrementally with each phase rather than documenting the final envisioned system in advance.

Likely files:

```text
README.md
docs/user-guide.md
docs/cli-reference.md
docs/configuration.md
docs/recovery.md
docs/stage-skills.md
```

Add a focused content contract document only when Phase 1 begins, for example:

```text
docs/content-contract.md
```

It should cover:

- artifact metadata;
- type, status, and authority vocabularies;
- stable ID rules;
- typed block conventions;
- evidence locators;
- relationships;
- compatibility and migration.

Do not describe an embeddings or vector-store design in that document.

## Operational and security considerations

- Treat frontmatter and artifact text as untrusted workspace input.
- Bound YAML alias expansion and input size.
- Never execute content found in metadata.
- Redact secrets before any future index captures source or artifact text.
- Respect existing path-safety rules when resolving evidence.
- Do not index runtime payloads or source directories by default merely because metadata references them.
- Preserve source licensing and access boundaries in any future retrieval design.
- A future shared or cloud index must not mix private repositories or tenants without explicit isolation.

These concerns should influence the content contract, but they do not require a retrieval subsystem now.

## Risks and mitigations

### Risk: Metadata becomes bureaucracy

**Mitigation:** Keep required fields minimal, generate deterministic fields where safe, and add structure only to high-value semantic objects.

### Risk: Agents produce superficially valid but inaccurate metadata

**Mitigation:** Prefer fields derived from known workflow context, validate references, and avoid requiring agents to invent broad topics or relationships.

### Risk: Stable IDs are not stable

**Mitigation:** Define deterministic domain-based rules, preserve IDs across title changes, and dogfood before enforcing workspace-wide uniqueness.

### Risk: Documents become repetitive and unpleasant

**Mitigation:** Keep retrieval summaries short, avoid duplicating full provenance in final reports, and use IDs only where cross-reference is valuable.

### Risk: Schema work delays useful product work

**Mitigation:** Implement phases independently, stop after any phase that has not demonstrated value, and do not make retrieval readiness a prerequisite for unrelated roadmap work.

### Risk: A premature ontology grows without limit

**Mitigation:** Start with small controlled vocabularies, version them, and require real query evidence before adding fields or relations.

### Risk: Legacy workspaces become second-class

**Mitigation:** Preserve legacy validation and discovery, use warning-first rollout, and provide migration only after the new contract is proven.

### Risk: Retrieval-oriented writing distorts analysis

**Mitigation:** Preserve nuanced narrative sections, keep uncertainty explicit, and treat semantic blocks as anchors rather than replacements for deep reasoning.

## Recommended delivery slices

This plan should not be implemented as one large sprint.

### Slice A: Evidence gathering and schema proposal

- complete Phase 0;
- write the initial schema and vocabulary proposal;
- add no production behavior.

### Slice B: Optional metadata parser

- implement Phase 1 parser and validation;
- add fixtures;
- preserve all legacy behavior.

### Slice C: Source-report pilot

- add metadata, retrieval summary, and revision-aware evidence to source reports only;
- run one study;
- evaluate output quality.

### Slice D: Final-report pilot

- add final-report metadata and typed patterns;
- retain existing report sections;
- test evidence traceability.

### Slice E: Requirements and decisions

- introduce requirement and decision IDs;
- add relationship validation within one sprint;
- do not yet require workspace-wide resolution.

### Slice F: Review traceability

- allow review findings to reference decisions and requirements;
- test validation and supersession behavior.

### Slice G: Workspace relationship resolution

- add cross-artifact ID discovery and relation validation only after enough artifacts use IDs.

### Slice H: Dogfood and schema revision

- complete Phase 7;
- decide whether to continue toward derived retrieval records.

Each slice should have its own sprint requirements, reasoning, plan, tests, and explicit stop/go decision.

## Stop conditions

Pause or simplify the effort if:

- authors repeatedly remove or ignore the metadata;
- agents generate unreliable relationships;
- the structure materially worsens report quality;
- template and validation complexity grows faster than demonstrated retrieval value;
- stable IDs cannot be maintained without a database or central registry;
- retrieval questions are answered adequately by current paths, headings, and lexical search;
- a proposed field is not used in real queries or governance workflows.

The correct outcome may be a much smaller content contract than the one proposed here.

## Success criteria

The content evolution is successful when:

- new managed artifacts have stable identity and clear authority;
- high-value reports explain when they should and should not be retrieved;
- important requirements, evidence, decisions, and review findings are self-contained;
- material claims trace to revision-aware evidence;
- current and superseded decisions are distinguishable;
- project and sprint indexes retain explicit governance over selected context;
- existing legacy workspaces remain usable;
- future chunking can follow semantic structure rather than arbitrary token windows;
- real retrieval questions can be answered from deterministic metadata, lexical content, and explicit relationships;
- the team can design a retrieval prototype from observed evidence rather than speculation.

## Final recommendation

Implement only Phases 0 and 1 initially, then pilot Phases 2 through 4 on source and final study reports before changing the full sprint artifact chain.

Do not begin embeddings, vector search, or agent-facing retrieval until:

- the content contract has been dogfooded;
- revision-aware evidence is reliable;
- stable IDs and relationships have proven manageable;
- a retrieval-question corpus demonstrates where lexical and metadata search are insufficient;
- the derived index can be clearly treated as a disposable cache rather than a new source of truth.

This sequence improves UltraPlan's content immediately while preserving the option to stop before a RAG system if simpler search and curated evidence packs prove sufficient.
