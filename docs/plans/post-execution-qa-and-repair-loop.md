# Post-Execution QA and Repair Loop Plan

**Status:** Proposed  
**Scope:** UltraPlan Go post-execution verification  
**Target flow:** `execute -> conformance review -> QA -> bounded repair loop -> verified`  
**Compatibility baseline:** the current `execute -> review -> smoke` flow, resumable review coverage, smoke protocol v1, `verify`, fingerprint-based freshness, canonical Markdown artifacts, and durable `flow-state.json`

## 1. Summary

UltraPlan currently has two different post-execution mechanisms:

1. `review` performs an independent, read-only conformance assessment against the sprint's governed contracts, handbook, reasoning, plan, execute evidence, project documents, and selected review protocols.
2. `smoke` invokes a separately cataloged external harness, runs bounded discovered checks, validates external evidence, records linked issues, and writes the canonical `smoke.md` summary.

These mechanisms are useful, but their current names and sequencing hide an important distinction:

- the current review is primarily **analytical conformance checking**;
- the proposed QA stage is **empirical investigation and evidence gathering**;
- smoke is one QA mechanism rather than the entire empirical stage;
- repair must consume only adjudicated, evidence-backed issues;
- repeated repair requires an explicit, bounded convergence loop.

The target model is:

```text
Execute
  -> Conformance Review
  -> QA
       -> map changed behaviour into bounded verification shards
       -> one QA investigator per shard
       -> inspect, theorise, test, refine, and gather evidence locally
       -> synthesize and adjudicate evidence globally
  -> Repair
       -> one repair agent per confirmed issue or root-cause cluster
       -> targeted re-QA
       -> affected-neighbour re-QA
       -> containing QA suites
       -> conformance delta review
  -> Verified, Blocked, Failed, or Escalated
```

The central design decision is that a shard agent owns the tight local investigation loop:

```text
inspect -> form falsifiable theory -> design check -> run check -> refine -> report evidence
```

A separate global adjudicator remains responsible for evidence quality, deduplication, cross-shard reasoning, issue promotion, repair grouping, and escalation. A QA investigator may create or modify verification code in an isolated workspace, but it must not repair production code. A repair agent may change production code, but it must not weaken or rewrite the evidence that authorised the repair.

## 2. Goals

### 2.1 Primary goals

- Rename the existing conceptual review stage to **Conformance Review** while preserving compatibility for the existing `review` command and artifacts during migration.
- Introduce **QA** as the empirical, evidence-producing post-execution stage.
- Absorb the existing smoke capability into QA as a suite/executor type without discarding its protocol, containment, safety, evidence, and cleanup guarantees.
- Avoid sending the whole change set to one agent by decomposing it into bounded logical verification surfaces.
- Let each shard investigator retain the context needed to move quickly from suspicion to discriminating evidence.
- Preserve a strong boundary between investigation, adjudication, and repair.
- Promote only evidence-backed theories into repairable issues.
- Make repair loops durable, resumable, fingerprinted, bounded, and convergent.
- Preserve current canonical-evidence rules: stale, malformed, diagnostic-only, narrow, or uncontained evidence must not silently become a passing assessment.

### 2.2 Secondary goals

- Reuse current runtime progress, attempt, fingerprint, artifact digest, state validation, and recovery concepts.
- Keep the user-facing default simple while exposing focused manual controls for investigation and recovery.
- Preserve negative evidence so refuted theories are not repeatedly rediscovered.
- Support both existing tests and newly generated tests, probes, traces, smoke scenarios, and external/manual evidence.
- Allow local findings to trigger new cross-shard investigations when the original decomposition proves incomplete.

## 3. Non-goals

This plan does not initially require:

- a general workflow DAG engine;
- automatic Git commits, pushes, branches, pull requests, or issue tracker integration;
- unrestricted autonomous mutation of the target repository;
- one permanent generated test for every theory;
- proving semantic correctness beyond the available contracts and evidence;
- fully parallel mutating investigations without workspace isolation;
- indefinite repair until all checks happen to turn green;
- replacing project-owned test systems with a universal UltraPlan test runner;
- deleting smoke protocol v1 before QA has equivalent or stronger coverage.

## 4. Terminology and responsibilities

### 4.1 Conformance Review

Conformance Review is the current `review` capability under a more precise name.

It asks:

- Does the implementation conform to requirements, reasoning, plan, handbook, selected contracts, project documents, and review protocols?
- Are required behaviours missing or contradicted?
- Are important invariants or decisions apparently violated?
- Which suspicious observations should QA attempt to confirm or refute?

It remains read-only against the target implementation.

Its outputs include:

- conformance findings;
- risks;
- static evidence;
- potential violations;
- QA theories or verification recommendations;
- a canonical verdict that remains independent of QA.

A failed or blocked Conformance Review may permit explicitly diagnostic QA, but QA must not convert the overall assessment into pass while the current Conformance Review remains failed or blocked.

### 4.2 QA

QA is the user-facing name for evidence-based quality verification.

It asks:

- Does the implementation build, run, and behave as intended?
- Do existing tests and acceptance checks pass?
- Can suspected failures be reproduced under controlled conditions?
- Can a theory be refuted?
- Are failure, concurrency, cancellation, recovery, persistence, API, and integration paths supported by evidence?
- Is the gathered evidence sufficient to create a repairable issue?

QA contains multiple executor types:

```text
QA
  - existing build/lint/type/static checks
  - existing unit tests
  - existing integration tests
  - generated targeted tests
  - temporary diagnostic probes
  - property/fuzz/concurrency checks
  - smoke suites
  - containing/regression suites
  - external/manual evidence
```

### 4.3 Smoke

Smoke becomes a QA suite or executor category.

The existing smoke protocol remains valuable because it already owns:

- cataloged harness discovery;
- explicit protocol and argv forms;
- bounded process execution;
- environment allowlisting;
- selection and containing-suite semantics;
- external evidence validation;
- issue references;
- timeout, cancellation, cleanup, and reconciliation behaviour;
- diagnostic-only review overrides;
- canonical-versus-narrow evidence rules.

The migration should wrap this capability under QA before attempting to redesign it.

### 4.4 Theory

A theory is a falsifiable claim about a possible defect or quality failure.

A useful theory contains:

- the claimed behaviour;
- its basis;
- affected locations or behavioural surfaces;
- the contract, requirement, invariant, or established behaviour it may violate;
- severity if confirmed;
- a confirmation condition;
- a refutation condition;
- an inconclusive condition;
- a proposed evidence strategy.

A vague concern is not a theory. For example:

```text
Weak: cancellation may be buggy.
Strong: when cancellation is accepted before process completion publishes, the completion path can still become the externally observed terminal outcome.
```

### 4.5 Evidence-backed issue

A theory becomes an issue only when adjudication concludes that:

- the expectation is grounded;
- the evidence is valid and current;
- the observed result matches the confirmation condition;
- the test or probe did not merely fail because of invalid setup;
- the issue concerns the current implementation fingerprint;
- repair scope and acceptance criteria are sufficiently clear.

### 4.6 Repair

Repair is the only stage allowed to modify production behaviour.

A repair task consumes a frozen issue packet containing:

- the confirmed claim;
- supporting evidence;
- violated contracts or expectations;
- likely affected paths;
- generated regression candidate, where applicable;
- allowed scope;
- acceptance criteria;
- required targeted and containing checks.

## 5. Target lifecycle

```text
execute evidence complete
        |
        v
conformance review
        |
        v
QA map
        |
        v
parallel shard investigations
  inspect -> theorise -> test -> refine -> evidence
        |
        v
synthesis and adjudication
   |          |             |
   |          |             +-> blocked/inconclusive -> follow-up or escalation
   |          +-> refuted/invalid -> retain negative evidence
   +-> confirmed issue
              |
              v
          repair agent
              |
              v
      exact reproducer / regression
              |
              v
       affected shard re-QA
              |
              v
      neighbouring shard re-QA
              |
              v
      containing QA suites
              |
              v
    conformance delta review
              |
      +-------+--------+
      |                |
    clean         issue remains/new issue
      |                |
   verified        next bounded cycle or escalation
```

## 6. QA decomposition

### 6.1 Do not shard mechanically by file

One agent per file is a useful fallback for isolated files, but files are packaging boundaries rather than reliable behavioural boundaries.

The default unit should be a **verification surface**: the smallest bounded set of changed and contextual code needed to reason about one coherent behaviour.

Examples:

```text
Terminal outcome arbitration
  - executor.go
  - cancellation.go
  - outcome.go
  - relevant tests

Configuration loading
  - config.go
  - defaults.go
  - validation.go

CLI verification transition
  - sprint_commands.go
  - verify.go
  - state.go
  - related command tests
```

### 6.2 QA map inputs

The mapper should consume:

- execute run-state and execute evidence;
- changed paths and, when available, changed symbols;
- target identity/fingerprint;
- sprint requirements and acceptance criteria;
- reasoning, plan, handbook, and selected contracts;
- Conformance Review findings and recommended checks;
- project test configuration and known QA commands;
- existing tests adjacent to changed code;
- package/module/import relationships where cheaply available;
- risk tags such as concurrency, cancellation, persistence, public API, migration, external I/O, security, and state transition.

### 6.3 Mapping rules

The mapper should:

- assign every changed path to one primary shard;
- create boundary shards when behaviour crosses packages, interfaces, producer/consumer pairs, state transitions, or public APIs;
- allow explicit overlap for high-risk boundaries;
- avoid duplicate generic investigation of the same surface;
- identify pre-existing containing test suites;
- identify surfaces for which automated evidence is unavailable;
- produce stable shard identities and an input fingerprint.

Suggested initial limits:

- maximum changed files per ordinary shard;
- maximum contextual files;
- maximum changed lines or symbols;
- maximum distinct behavioural concerns;
- maximum context expansion depth;
- maximum shard runtime and investigation iterations.

When a surface exceeds a limit, split it by behaviour rather than arbitrary file count.

### 6.4 Shard types

Initial shard kinds may include:

- `file`: isolated file or pure utility;
- `package`: cohesive package/module behaviour;
- `feature`: one logical feature across files;
- `interface`: interface plus changed provider/consumer behaviour;
- `state-transition`: lifecycle or state-machine path;
- `cross-cutting`: cancellation, logging, security, observability, compatibility, persistence;
- `boundary`: producer/consumer, CLI/domain, protocol/adapter, storage/domain;
- `acceptance`: requirement-driven end-to-end behaviour.

These kinds are descriptive rather than a closed ontology. The important contract is the bounded behavioural scope.

## 7. Shard investigator contract

### 7.1 Inputs

Each investigator receives:

- shard identity and title;
- primary changed paths;
- allowed contextual paths;
- relevant requirements, decisions, invariants, and acceptance criteria;
- related Conformance Review findings;
- existing tests and known commands;
- implementation and shard fingerprints;
- writable verification workspace policy;
- budgets and stop conditions.

It should not receive the entire repository by default. It may request bounded context expansion, which is recorded in state.

### 7.2 Allowed actions

A shard investigator may:

- inspect assigned and approved contextual code;
- run safe existing checks;
- write targeted tests in an isolated workspace;
- create temporary probes or fixtures;
- create smoke scenarios compatible with the QA executor contract;
- capture bounded logs, traces, exit results, timing, and structured evidence;
- refine, refute, invalidate, or split its own theories;
- escalate a cross-shard theory.

It may not:

- modify shared production code;
- modify governed planning inputs;
- weaken existing tests or acceptance conditions;
- change test expectations merely to obtain a pass;
- silently broaden its shard beyond configured limits;
- mark a production issue repaired;
- directly promote its own claim into an automatically repairable issue.

### 7.3 Tight investigation loop

Each shard runs a bounded loop:

```text
inspect
  -> identify risk or suspicious behaviour
  -> write a falsifiable theory
  -> define confirmation/refutation/inconclusive outcomes
  -> choose the cheapest discriminating evidence method
  -> execute the check
  -> interpret the result
  -> refine, split, refute, confirm, block, or escalate
```

Suggested limits for the first implementation:

- no more than three investigation iterations per theory;
- no more than a small bounded number of generated checks per shard;
- explicit command and wall-clock budgets;
- explicit context expansion budget;
- fail closed on uncertain cleanup or workspace isolation;
- preserve the final check plan before execution to reduce confirmation bias.

### 7.4 Theory outcomes

Each theory ends as one of:

- `confirmed`: evidence supports the claim;
- `refuted`: evidence directly contradicts the claim under the stated conditions;
- `invalid`: the claim was based on an incorrect interpretation or unsupported expectation;
- `inconclusive`: the check could not discriminate reliably;
- `blocked`: prerequisites or environment were unavailable;
- `cross_shard`: local evidence suggests an interaction outside the shard;
- `not_applicable`: the expected behaviour does not apply to this implementation.

Negative outcomes are retained as useful evidence.

### 7.5 Example shard result

```json
{
  "shard_id": "SHARD-terminal-outcome",
  "fingerprint": "...",
  "surface": "terminal outcome arbitration",
  "status": "completed",
  "investigations": [
    {
      "id": "THEORY-017",
      "claim": "accepted cancellation can be overwritten by late successful completion",
      "expectation_refs": ["INV-TERMINAL-001"],
      "status": "confirmed",
      "confirmation_condition": "success is externally observed after cancellation was accepted",
      "refutation_condition": "controlled ordering proves cancellation remains terminal",
      "inconclusive_condition": "ordering cannot be controlled",
      "evidence": [
        {
          "kind": "generated_test",
          "path": "internal/runtime/executor_test.go",
          "command": "go test ./internal/runtime -run TestCancellationWinsTerminalOutcome",
          "result": "failed",
          "summary": "success was observed after cancellation"
        }
      ]
    }
  ]
}
```

## 8. Investigation workspace isolation

Parallel investigators that may write tests must not share a mutable target tree.

The required model is one isolated investigation workspace per shard attempt. The isolation mechanism may be:

- a runtime-provided sandbox;
- a temporary copy of the target tree;
- a copy-on-write filesystem;
- a temporary Git worktree when the target state can be represented safely;
- another explicitly validated local isolation mechanism.

UltraPlan must not assume that the target is clean, committed, or even a Git repository. Therefore Git worktrees should be an optimisation, not the only design.

Until isolated writable workspaces exist:

- investigators that only read and run non-mutating existing commands may run in parallel;
- investigators that create tests or probes must run sequentially in a protected temporary copy, or remain disabled;
- inability to prove isolation is `blocked`, not permission to write into the shared target.

Generated verification patches should be preserved separately from production repairs. A later adjudicator or repair workflow decides whether a regression candidate becomes permanent.

## 9. Synthesis and adjudication

### 9.1 Why a global stage remains necessary

Local shard evidence does not remove the need for global reasoning. The synthesizer/adjudicator must:

- deduplicate equivalent theories;
- combine complementary evidence;
- detect contradictions;
- identify manifestations of one root cause;
- identify cross-shard interactions;
- validate expectation grounding;
- challenge flaky, invalid, or non-discriminating checks;
- classify evidence sufficiency;
- decide which generated tests are regression candidates;
- decide whether issues should be repaired separately or as one root-cause cluster;
- request additional focused investigations where necessary.

### 9.2 Evidence quality checks

Before issue promotion, adjudication should verify:

- the evidence concerns the current implementation and shard fingerprint;
- the setup is valid and contained;
- the confirmation condition was established before interpretation;
- the failure corresponds to the theory rather than unrelated setup failure;
- repeated or deterministic execution requirements were met;
- external evidence paths, hashes, and identities remain valid;
- a subjective preference has not been mistaken for a contract violation;
- severity and repair eligibility are distinct decisions.

### 9.3 Cross-shard follow-up

A cross-shard theory creates a new bounded investigation rather than silently expanding an existing shard.

Example:

```text
publisher shard: locally emitted event appears valid
consumer shard: locally parsed event appears valid
synthesis: versions differ across the boundary
new shard: event schema producer/consumer compatibility
```

The new shard receives only the relevant prior evidence and bounded code context.

### 9.4 Issue promotion rules

A confirmed theory may be promoted when:

- a governed or established expectation is identified;
- evidence is current and sufficient;
- the issue is reproducible or otherwise statically proven;
- impact and scope are understood enough to define acceptance;
- the issue is not merely an unchosen design alternative;
- no unresolved evidence contradiction remains.

A promoted issue should include:

- stable ID;
- source theory IDs and shard IDs;
- severity and confidence;
- violated expectations;
- evidence references;
- likely affected paths;
- allowed repair scope;
- regression candidate;
- targeted verification commands;
- containing suites;
- repair eligibility and escalation reason.

## 10. Repair model

### 10.1 Repair eligibility

Automatic repair is allowed only when:

- the issue is evidence-backed;
- repair does not require changing requirements, reasoning, plan, or foundational design;
- repair scope is bounded;
- acceptance criteria are explicit;
- the repair does not require destructive migration or policy judgment;
- verification can determine success;
- required workspace and mutation safety are available.

Issues requiring design choice, requirement revision, broad migration, destructive action, secrets, external approvals, or unclear acceptance are escalated.

### 10.2 Repair granularity

Default to one repair agent per issue.

Group multiple issues only when adjudication establishes a shared root cause and one repair is safer than competing changes. Grouping must be explicit and recorded.

### 10.3 Repair agent restrictions

A repair agent may:

- modify production code within the approved scope;
- promote an adjudicated regression test candidate;
- add additional focused tests needed to express the accepted fix;
- run the specified targeted checks.

It may not:

- delete or weaken the evidence that authorised the repair;
- modify the canonical theory/issue record;
- broaden requirements to redefine the failure away;
- modify the smoke/QA harness merely to suppress a failure;
- suppress checks without explicit adjudicator approval;
- mark its own repair globally verified.

### 10.4 Reverification order

After repair, widen evidence progressively:

1. Run the exact reproducer or regression candidate.
2. Rerun the affected shard.
3. Rerun linked theories touching the same paths or invariants.
4. Rerun neighbouring/boundary shards identified by dependency impact.
5. Run the containing QA suite.
6. Run containing smoke against the repaired target.

Conformance Review runs once before repair admission. Repair does not trigger a second review.

## 11. Bounded convergence loop

The system must optimise for convergence, not indefinite retries.

Initial automatic limits:

- maximum three repair cycles per verification run;
- maximum two reopenings of the same issue;
- stop when the confirmed issue set does not decrease across a cycle;
- stop when severity, affected scope, or uncertainty increases;
- stop when repair requires governed design changes;
- stop when required evidence remains unavailable;
- stop when new issues exceed a configured threshold;
- stop on uncertain cleanup, target drift, or workspace identity change.

Each cycle records:

```text
cycle ID
input implementation fingerprint
issues entering cycle
repairs attempted
issues resolved
issues reopened
new issues
blocked/inconclusive checks
output implementation fingerprint
convergence decision
```

Terminal outcomes:

- `verified`: current Conformance Review and required QA evidence pass;
- `verified_with_findings`: only accepted non-blocking findings remain;
- `failed`: current evidence establishes an unresolved defect or conformance failure;
- `blocked`: required evidence, environment, cleanup, or isolation is unavailable;
- `escalated`: design or human decision is required;
- `stalled`: bounded repair made no acceptable progress.

## 12. Artifact and state model

### 12.1 Canonical artifacts

The intended canonical user-facing artifacts are:

```text
projects/<project>/sprints/<sprint>/
  execute.md
  conformance-review.md
  qa.md
  verification/
    state.json
    attempts/
      <attempt-id>/
        map.json
        shards/
          <shard-id>.json
        synthesis.json
        issues.json
        repairs.json
```

The exact detailed layout can be refined, but canonical summaries and durable machine state must remain separate.

### 12.2 Compatibility migration

Do not rename everything in one release.

Recommended transition:

1. Keep internal `StageReview`, `review.md`, and `review` command initially.
2. Change human-facing labels and documentation to **Conformance Review**.
3. Add `conformance-review` as an alias.
4. Add explicit metadata such as `review_kind: conformance` to new JSON/state.
5. Introduce `qa.md` and QA state while retaining `smoke.md` as a compatibility summary generated by the smoke QA executor.
6. Make `smoke` an alias for `qa --suite smoke` after QA owns the same guarantees.
7. Only rename the canonical review artifact in a later migration with an explicit compatibility guide.

### 12.3 Detailed state ownership

Avoid expanding `flow-state.json` into an unbounded investigation database.

Recommended ownership:

- `flow-state.json`: canonical stage summaries, freshness, current verdicts, next action, and pointers/digests;
- QA durable state: shards, theories, evidence, synthesis, issues, repair cycles, budgets, and resumability;
- external harness: raw smoke streams, per-test artifacts, external issue files, and large evidence;
- canonical Markdown: current human-readable conformance and QA summaries.

The QA state must be schema-versioned and fail closed on unknown major versions.

### 12.4 Fingerprints

Track at least:

- governed Conformance Review input fingerprint;
- implementation fingerprint;
- QA map fingerprint;
- shard input fingerprint;
- theory/evidence implementation fingerprint;
- QA canonical evidence fingerprint;
- repair input and output fingerprints;
- containing-suite evidence fingerprint.

A repair invalidates evidence according to affected dependency surfaces. Unaffected negative evidence may be retained only when the dependency model and fingerprints prove it remains applicable.

## 13. CLI design

### 13.1 Compatibility-first command surface

```bash
ultraplan sprint <project> <sprint> review
ultraplan sprint <project> <sprint> conformance-review
ultraplan sprint <project> <sprint> qa
ultraplan sprint <project> <sprint> repair --issue <id>
ultraplan sprint <project> <sprint> verify
```

`review` and `conformance-review` initially invoke the same capability.

### 13.2 QA controls

Potential focused controls:

```bash
ultraplan sprint <project> <sprint> qa --dry-run
ultraplan sprint <project> <sprint> qa --restart
ultraplan sprint <project> <sprint> qa --parallel <n>
ultraplan sprint <project> <sprint> qa --shard <id>
ultraplan sprint <project> <sprint> qa --theory <id>
ultraplan sprint <project> <sprint> qa --suite smoke
ultraplan sprint <project> <sprint> qa --diagnostic
ultraplan sprint <project> <sprint> qa --json
```

The public command need not expose internal phases such as map, investigate, synthesize, and adjudicate as separate mandatory commands. They should be durable internal phases, with focused controls added only where recovery requires them.

### 13.3 Smoke compatibility

```bash
ultraplan sprint <project> <sprint> smoke ...
```

should eventually behave as a compatibility alias for:

```bash
ultraplan sprint <project> <sprint> qa --suite smoke ...
```

Existing flags, selection, confirmation, timeout, diagnostic override, JSON, and containment semantics must be preserved.

### 13.4 Verify orchestration

Recommended target:

```bash
ultraplan sprint <project> <sprint> verify --to conformance-review
ultraplan sprint <project> <sprint> verify --to qa
ultraplan sprint <project> <sprint> verify --to qa --repair --max-cycles 3 --yes
```

`verify` remains the orchestrator. It should not create a third competing canonical assessment artifact.

## 14. Internal architecture direction

### 14.1 Separate planning stages from verification phases

The current code models review and smoke as `PlanningStage` values in several places. QA and repair loops will make that increasingly misleading.

Introduce a distinct verification type before the new lifecycle becomes deeply embedded:

```go
type VerificationPhase string

const (
    PhaseConformanceReview VerificationPhase = "conformance-review"
    PhaseQA                VerificationPhase = "qa"
    PhaseRepair            VerificationPhase = "repair"
)
```

Compatibility adapters can continue accepting existing `StageReview` and `StageSmoke` values during migration.

### 14.2 Keep behaviour in the sprint module

Follow the existing module-driven architecture. Start with focused files in `internal/sprint`, for example:

```text
qa.go
qa_types.go
qa_state.go
qa_map.go
qa_shards.go
qa_prompt.go
qa_evidence.go
qa_adjudication.go
qa_repair.go
qa_verify.go
qa_smoke.go
```

Do not introduce a large global `internal/qa` layer unless the behaviour becomes genuinely reusable outside sprint verification.

### 14.3 Reuse current foundations

Reuse or extend:

- `VerificationAttempt` lifecycle;
- artifact digest validation;
- fingerprint freshness;
- bounded diagnostics;
- atomic writes and reconciliation;
- runtime prompt references and trace IDs;
- progress events;
- review concurrency and resumability concepts;
- process cleanup guarantees;
- smoke evidence references;
- JSON schema versioning;
- CLI/TUI operation preparation and confirmation.

## 15. Implementation phases

### Phase 0: terminology and compatibility design

- Document Conformance Review versus QA.
- Add `conformance-review` CLI/help alias without changing underlying state.
- Label existing review output as Conformance Review in text and TUI.
- Add `review_kind: conformance` to new JSON where additive compatibility permits.
- Define the verification phase type and compatibility mapping.
- Define QA artifact and state schemas before runtime implementation.

**Exit criteria:** terminology is unambiguous; no existing workflow breaks.

### Phase 1: QA map and read-only shard investigation

- Add deterministic QA mapping from execute evidence and changed paths.
- Add stable shard IDs, fingerprints, budgets, and map validation.
- Run shard agents read-only against isolated or protected contexts.
- Permit existing non-mutating checks only.
- Produce theories, static evidence, recommended checks, refutations, and cross-shard escalations.
- Add synthesis without issue promotion or repair.

**Exit criteria:** a change set is decomposed reproducibly; agents investigate bounded surfaces; results are resumable and inspectable.

### Phase 2: isolated evidence-writing investigators

- Add isolated investigation workspace creation and validation.
- Permit generated tests, fixtures, probes, and smoke scenarios inside the isolated workspace.
- Require predeclared confirmation/refutation/inconclusive conditions.
- Capture generated patches, commands, bounded output, and evidence identity.
- Preserve permanent-regression candidates separately from temporary diagnostics.

**Exit criteria:** shard agents can safely move from theory to discriminating evidence without modifying shared production code.

### Phase 3: adjudication and canonical QA

- Add evidence quality validation.
- Add theory deduplication and cross-shard follow-up.
- Add issue promotion rules and repair eligibility.
- Write canonical `qa.md` and QA state.
- Wrap existing smoke protocol as a QA suite/executor.
- Preserve `smoke.md` compatibility and external evidence layout.
- Derive overall assessment from Conformance Review plus QA.

**Exit criteria:** QA produces current canonical evidence and evidence-backed issues; smoke is available through QA without lost guarantees.

### Phase 4: single-issue manual repair

- Add `repair --issue <id>`.
- Freeze issue packet and allowed scope.
- Apply repair in a protected workspace.
- Run exact reproducer, affected shard, containing QA, and repaired-target smoke.
- Persist repair and reverification state.
- Require explicit confirmation before production mutation.

**Exit criteria:** one confirmed issue can be repaired and reverified end to end without speculative production changes.

### Phase 5: bounded automatic repair cycles

- Add repair grouping by adjudicated root cause.
- Add `verify --repair --max-cycles`.
- Add stall, reopening, severity-growth, and scope-growth detection.
- Add progressively widening re-QA.
- Add terminal escalated/stalled states.
- Add restart/resume and cancellation recovery.

**Exit criteria:** automatic repair is bounded, durable, convergent, and cannot manufacture a pass by weakening evidence.

### Phase 6: optimisation and advanced coverage

Only after the earlier phases are proven:

- symbol-aware mapping;
- dependency graph integration;
- cost-aware shard scheduling;
- selective evidence retention after repair;
- property/fuzz/concurrency executor plugins;
- cross-sprint regression knowledge;
- reusable refuted-theory cache;
- richer TUI investigation views;
- optional external issue/PR publication.

## 16. Testing strategy

### 16.1 Deterministic offline tests

Require fake-runtime and fake-workspace coverage for:

- stable QA mapping;
- shard overlap and boundary creation;
- budget enforcement;
- theory status transitions;
- invalid confirmation conditions;
- state schema migration/rejection;
- fingerprint invalidation;
- resumable shard attempts;
- synthesis deduplication;
- cross-shard follow-up;
- evidence quality rejection;
- issue promotion and non-promotion;
- repair eligibility;
- repair loop stall detection;
- canonical artifact atomicity;
- cancellation and reconciliation;
- smoke-as-QA compatibility;
- overall assessment derivation.

### 16.2 Isolation tests

Inject failures for:

- copy/worktree creation;
- target drift during investigation;
- path escape;
- symlink escape;
- concurrent shard writes;
- cleanup timeout;
- leftover process descendants;
- failed temporary workspace deletion;
- generated patch outside allowed verification paths.

Uncertain isolation must block mutation.

### 16.3 Evidence tests

Cover:

- confirmed, refuted, invalid, inconclusive, blocked, cross-shard, and not-applicable outcomes;
- flaky generated tests;
- unrelated setup failure;
- stale evidence;
- malformed evidence;
- incorrect expectation grounding;
- evidence that proves a symptom but not the stated claim;
- generated regression candidate promotion and rejection.

### 16.4 Repair tests

Cover:

- repair attempts to weaken tests;
- repair attempts outside allowed scope;
- issue reopening;
- one root-cause repair resolving multiple issues;
- repair causing new higher-severity issue;
- exact reproducer pass but containing suite fail;
- delta review requiring full review;
- cycle limit and stalled outcome.

## 17. Safety and trust rules

- QA evidence and repair operate against explicit target fingerprints.
- Runtime success is not QA success.
- A failing check is not automatically a confirmed issue.
- A passing narrow check is not automatically containing evidence.
- Diagnostic QA after failed Conformance Review cannot improve the canonical overall assessment.
- Shard investigators cannot modify shared production code.
- Repair agents cannot modify evidence records or weaken accepted expectations.
- Generated tests must be reviewed for validity before becoming permanent.
- Raw provider/runtime/harness payloads remain bounded and external where appropriate.
- Secrets and full unsafe environment values must not enter state, Markdown, JSON, prompts, or progress events.
- Cancellation, timeout, cleanup uncertainty, stale evidence, or isolation uncertainty must never be represented as pass.

## 18. Risks and mitigations

### Excessive shard fragmentation

**Risk:** too many tiny shards miss behavioural interactions and waste runtime.

**Mitigation:** verification-surface mapping, explicit boundary shards, overlap limits, and global synthesis.

### Shards become too broad

**Risk:** agents revert to shallow whole-change review.

**Mitigation:** file/context/line/concern budgets and explicit cross-shard escalation.

### Confirmation bias

**Risk:** the investigator writes a test intended only to prove its own theory.

**Mitigation:** require confirmation, refutation, and inconclusive conditions before execution; central evidence adjudication; optional independent challenge for high-severity findings.

### Generated-test pollution

**Risk:** diagnostic tests are committed permanently despite being brittle or artificial.

**Mitigation:** preserve generated checks as candidates; adjudication classifies permanent regression, diagnostic probe, smoke scenario, or invalid experiment.

### Parallel mutation conflicts

**Risk:** investigators overwrite each other's tests or alter the target during inspection.

**Mitigation:** isolated shard workspaces; read-only parallelism until isolation is proven.

### Repair oscillation

**Risk:** repeated agents alternate between competing fixes.

**Mitigation:** frozen issue packets, cycle history, reopen limits, root-cause grouping, and stall detection.

### State explosion

**Risk:** `flow-state.json` becomes large and fragile.

**Mitigation:** keep canonical summary state separate from detailed QA attempt state and external evidence.

### Smoke regression during absorption

**Risk:** replacing smoke too early loses process safety or evidence guarantees.

**Mitigation:** initially wrap protocol v1 unchanged as a QA executor; deprecate only after parity tests.

## 19. Decisions captured by this plan

- Keep the existing analytical review capability and call it Conformance Review.
- Introduce QA as a distinct empirical stage.
- Absorb smoke into QA as a suite/executor, not as the whole QA model.
- Do not send the complete change set to one QA agent by default.
- Shard by logical verification surface, with per-file shards only where appropriate.
- Let each shard investigator own theory generation and evidence gathering for a tight feedback loop.
- Keep global synthesis/adjudication separate.
- Keep production repair separate from QA investigation.
- Promote only evidence-backed, grounded theories into repairable issues.
- Reverify progressively after repair.
- Bound repair loops and make non-convergence explicit.
- Preserve compatibility and migrate incrementally rather than renaming and replacing all current review/smoke state in one release.

## 20. Recommended first implementation sprint

The first sprint should not attempt repair. It should establish the architecture safely:

1. Add Conformance Review terminology and aliasing.
2. Introduce `VerificationPhase` separately from `PlanningStage`.
3. Define QA state, shard, theory, evidence, and synthesis schemas.
4. Implement deterministic QA mapping from execute changed paths.
5. Run bounded read-only investigators per logical verification surface.
6. Persist theories, static evidence, refutations, and cross-shard requests.
7. Add central synthesis without issue promotion.
8. Expose `qa --dry-run`, `qa`, `qa --shard`, status, JSON, and TUI summaries.
9. Keep existing smoke and verify behaviour unchanged in this sprint.
10. Prove cancellation, resume, fingerprint invalidation, atomic state, and deterministic mapping before enabling test-writing investigators.

This sequence earns the next abstraction: isolated writable investigations and evidence adjudication. It avoids combining decomposition, generated tests, smoke migration, issue promotion, production repair, and looping into one unsafe release.
