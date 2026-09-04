# QA investigator-authored tests

## Goal

Let an investigator turn a falsifiable theory into executable evidence, let the arbiter request stronger evidence, and always send that request back to the original investigator session. When the evidence promotes an issue, carry the validated regression test into the production repository together with the repair.

The evidence chain must remain explicit:

```text
theory
  -> original investigator session
  -> immutable reproduction specification
  -> investigator-authored test
  -> matching failure on the frozen implementation
  -> arbitration
  -> promoted issue
  -> unchanged test passes with repair
  -> test and repair applied atomically
```

## Current implementation anchors

- [`internal/sprint/qa.go`](../../internal/sprint/qa.go) runs investigators, records attempts, handles same-session context continuation in `continueQAInvestigator`, then starts synthesis, arbitration, evidence publication, and adjudication.
- [`internal/sprint/qa_prompt.go`](../../internal/sprint/qa_prompt.go) builds the investigator packet and permission policy. Investigator calls currently use the protected target as their working directory and remain read-only.
- [`internal/sprint/qa_types.go`](../../internal/sprint/qa_types.go) defines `QAInvestigatorAttempt`, `QATheory`, `QAArbiterOverride`, `QAArbiterIssue`, `QAArbiterGroup`, and `QAArbitration`.
- [`internal/sprint/qa_arbiter.go`](../../internal/sprint/qa_arbiter.go) groups theories, validates arbiter output, creates provisional issues, and reconciles issues across groups. The current wire contract can override theories and propose issues, but cannot request more evidence.
- [`internal/sprint/qa_evidence.go`](../../internal/sprint/qa_evidence.go) defines frozen evidence plans, command results, evidence records, adjudicated issues, and theory-to-evidence repair inputs.
- [`internal/sprint/qa_investigation.go`](../../internal/sprint/qa_investigation.go) creates a protected disposable copy, executes a frozen command without a shell, validates changed paths and identities, and proves cleanup.
- [`internal/sprint/qa_state.go`](../../internal/sprint/qa_state.go) owns immutable paths and publication for maps, shards, evidence plans, evidence records, patches, adjudication, and assessment under one QA attempt.
- [`internal/sprint/qa_adjudication.go`](../../internal/sprint/qa_adjudication.go) promotes accepted failing evidence into root-cause groups, issues, and repair assignments.
- [`internal/sprint/qa_repair.go`](../../internal/sprint/qa_repair.go) loads the issue's theories, evidence, plans, arbiter overrides, exact reproducer, checks, and allowed paths into `RepairIssuePacket`.
- [`internal/sprint/qa_repair_state.go`](../../internal/sprint/qa_repair_state.go) persists repair packets, proposal patches, scope records, reverification, cleanup, and final results.

## Required state changes

### Retain original investigator identity

Add these fields to `QAInvestigatorAttempt`:

```go
SessionID       string
Provider        string
Model           string
Variant         string
RuntimeStoreRef string
WorkspaceID     string
```

`runOneQAShard` already receives `result.SessionID`, but currently uses it only for immediate context continuation. Persist it before arbitration. Validate that every later evidence request uses the same session ID, provider, model, runtime store, and private workspace.

If the retained session cannot continue, publish `original_session_unavailable` and leave the theory inconclusive. Do not silently create a replacement investigator.

### Add reproduction records

Add immutable types alongside `QAEvidencePlan` and `QAEvidenceRecord`:

```go
type QAReproductionSpec struct {
    ID                    string
    AttemptID             string
    ShardID               string
    TheoryIDs             []string
    Claim                 string
    Preconditions         []string
    ExpectedBehavior      string
    PredictedFailure      QAFailureSignature
    InconclusiveConditions []string
    ApprovedTestPaths     []string
    Command               QACheckDescriptor
    ImplementationFingerprint string
    FrozenAt              time.Time
}

type QATestBundle struct {
    ID             string
    SpecID         string
    Files          []QATestFile
    ContentDigest  string
    DerivedFrom    string
}

type QAReproductionRun struct {
    ID             string
    SpecID         string
    TestBundleID   string
    TargetIdentity string
    Result         QACommandResult
    Signature      QAFailureSignature
    Outcome        QAEvidenceOutcome
    Cleanup        QACleanupFacts
}
```

The failure signature must distinguish a reproduced claim from an unrelated failure. Record the expected test name, assertion or structured error code, exit class, and bounded output matcher. Compile errors, unrelated panics, timeouts, truncated output, infrastructure errors, and mismatched assertions are `inconclusive`.

### Add arbiter evidence requests

Extend the arbiter wire contract with `evidence_requests`:

```go
type QAArbiterEvidenceRequest struct {
    ID                  string
    TheoryIDs           []string
    OriginShardID       string
    Gap                 string
    RequestedEvidence   string
    RequiredObservation string
    ControlRequirement  string
    Priority            string
}
```

Each request must reference theories from one original shard. If an arbiter group contains theories from several shards, it emits one request per origin shard. This guarantees that UltraPlan can route every request back to the investigator that authored those theories.

Validation in `validateQAArbiterGroupOutput` must reject unknown theories, a mismatched origin shard, empty discrimination criteria, duplicate requests, requests for already sufficient evidence, and requests above the evidence-round budget.

## Investigator workspace lifecycle

The original investigator cannot safely begin in the protected target if later turns need test-writing tools. Change the initial shard lifecycle:

1. Create one private isolated target copy per shard before the first investigator call.
2. Start the investigator session with that copy as its fixed working directory.
3. Keep writes denied during initial theory generation.
4. Retain the workspace through arbitration and bounded evidence-strengthening rounds.
5. When the arbiter requests evidence, continue the same session in the same workspace and allow write and edit only for validated `_test.go` paths owned by the shard.
6. Snapshot the workspace before and after every continuation. Reject source changes, repository-control changes, generated binaries, symlinks, and test paths outside the allowlist.
7. Clean the workspace only after the shard and every arbiter request reach a terminal state. Persist cleanup facts.

Use the native protected-root and descendant-cleanup checks already required by `RunQAInvestigation`. The model never receives write authority over the production checkout.

For interruption recovery, use a stable private path derived from attempt ID and shard ID, similar to the stable private worker path used by repair campaigns. The path is operational state, not durable evidence. Test bundles and run records must remain sufficient after the workspace is removed.

## Same-session evidence strengthening

Add `continueQAInvestigatorForEvidence` beside `continueQAInvestigator` in `qa.go`.

Its request must contain:

- The arbiter evidence request.
- The original theories and their current evidence.
- The frozen reproduction conditions.
- Approved test paths and commands.
- Bounded compiler or test output from the preceding attempt, if any.
- The remaining round, file, byte, command, and time budgets.

Set `SessionID` to the retained original investigator session and `SessionAction` to `continue`. Require the returned session ID to match exactly. Record the continuation as another `QAInvestigatorAttempt` in the same shard history.

The original investigator may create or strengthen a test, but it cannot approve the test or classify the result. UltraPlan snapshots the files, builds the immutable bundle, executes the frozen command, and compares the observed failure with the frozen signature.

## Iterative arbitration state machine

Replace the one-way investigation-to-arbitration transition with a bounded loop:

```text
initial investigation
  -> execute proposed reproducers
  -> arbitrate
      -> issue or terminal theory outcome
      -> evidence request
          -> continue original investigator session
          -> execute strengthened reproducer
          -> re-arbitrate affected theory group
```

Only affected groups rerun. Cross-group reconciliation waits until no group has pending evidence requests.

Add lower-only budgets for:

- Evidence rounds per shard.
- Tests per theory and per issue.
- Authored test files and bytes.
- Test commands per round.
- Authoring runtime turns and wall time.

The same normalized evidence request cannot be issued twice without new evidence. Budget exhaustion produces an inconclusive theory and no repair-eligible issue.

## Multiple theories per issue

Keep `QAArbiterIssue.TheoryIDs`, but persist an explicit coverage graph:

```go
type QAIssueEvidenceCoverage struct {
    IssueID       string
    TheoryIDs     []string
    TestBundleIDs []string
    EvidenceIDs   []string
    Coverage      map[string][]string // theory ID to evidence or test IDs
    PrimaryReproducers []string
}
```

One test may cover several theories only when its reproduction spec names each theory and its failure signature discriminates the shared claim. The arbiter may combine theories into one issue while requesting separate tests for distinct observable behavior.

Repair verification must pass every primary reproducer covering a confirmed theory. It may not discard a reproducer merely because another test covers the same issue title.

## Persistent layout

Extend the existing attempt layout owned by `qa_state.go`:

```text
verification/attempts/<attempt-id>/
  investigator-tests/<test-id>/
    reproduction-spec.json
    bundle.json
    files/
    authoring-attempts/<attempt-number>.json
    runs/<run-id>/
      result.json
      failure-signature.json
      stdout.log
      stderr.log
      cleanup.json
  arbiter-evidence-requests/<request-id>.json
  issue-evidence-coverage.json
```

Add path constructors and load/publish validation in `qa_state.go`. Publication must be atomic with the updated shard, synthesis, and QA state. Full OpenCode transcripts can remain in the sprint runtime store, but session transcripts are not the authority for rerunning evidence.

Persist bounded stdout and stderr, not only digests, because signature review and reproduction diagnosis need the actual retained output. Continue recording digests, byte counts, truncation, and redaction facts.

## Production promotion

When adjudication promotes an issue, mark validated test bundles as regression candidates. `PrepareRepair` must load them with the issue's theories and evidence and freeze them into the repair packet.

Split the repair proposal into two recorded components:

```text
test.patch
implementation.patch
```

Verification order:

1. Apply the exact test patch to the frozen broken implementation and require the expected failure signature.
2. Apply the implementation patch without changing the test patch.
3. Require the same test digest to pass.
4. Run every other primary reproducer for the issue.
5. Run package tests, linked QA checks, containing QA, and containing smoke.
6. Apply both patches to production in one journaled transaction.

If repository conventions require editing the investigator test, create a derived bundle with `DerivedFrom` set to the original bundle. The derived bundle must repeat the complete fail-before and pass-after proof. Never rewrite the original evidence bundle.

Extend repair scope validation so approved regression-test paths are allowed only when they come from the frozen issue coverage record. Preserve existing forbidden paths and byte limits.

## CLI and status

Add read and rerun commands:

```text
ultraplan sprint <project> <sprint> qa evidence tests --json
ultraplan sprint <project> <sprint> qa evidence inspect --test <test-id> --json
ultraplan sprint <project> <sprint> qa evidence rerun --test <test-id> --target current --json
```

`rerun` creates a fresh isolated copy, materializes the immutable bundle, verifies every digest, executes the frozen descriptor, publishes a new run record, and cleans the copy. It never depends on the original authoring workspace.

QA status should expose pending arbiter evidence requests, their original investigator session, evidence round, test bundle, latest result, and next action.

## Implementation sequence

1. Persist investigator runtime identity and original session IDs in shard attempts.
2. Move initial investigator execution into stable private per-shard workspaces.
3. Add reproduction specifications, test bundles, failure signatures, run records, path constructors, and atomic persistence.
4. Add bounded test-authoring permissions and independent execution without shell access.
5. Extend arbiter output and validation with evidence requests.
6. Add same-session evidence continuation routed strictly to the original shard investigator.
7. Add bounded re-arbitration and delay reconciliation until requests terminate.
8. Add the issue-to-theory-to-test coverage graph.
9. Freeze regression bundles into repair packets and implement paired test and implementation patches.
10. Add persistent rerun commands, status projection, and web/TUI rendering.

## Required tests

- An arbiter request resumes the exact original session ID, provider, model, runtime store, and workspace.
- Session loss blocks the theory as inconclusive and never starts a replacement investigator.
- Requests spanning several origin shards are split and routed correctly.
- A test confirming several theories records complete theory coverage.
- Compile errors, unrelated panics, timeouts, and mismatched assertions do not confirm a theory.
- The same request cannot loop without new evidence.
- Test authoring cannot change product source or escape approved `_test.go` paths.
- The immutable test fails on the frozen implementation and passes after the repair.
- Editing a promoted test creates a derived bundle and repeats both sides of the proof.
- The test patch and implementation patch apply atomically or compensate together.
- Rerun works after the original authoring workspace and model transcript are removed.
- Cancellation and recovery retain test bundles, run records, session identity, and cleanup facts.

## Completion criteria

The feature is complete only when a live campaign proves all of the following:

- An investigator authors a test that reproduces its theory with the expected failure signature.
- The arbiter requests stronger evidence and UltraPlan resumes the original investigator session.
- The strengthened test changes the arbiter result without changing prior immutable evidence.
- One promoted issue maps to multiple theories with explicit evidence coverage.
- The exact regression test fails before the fix, passes after it, and lands atomically with the implementation repair.
- The retained test bundle reruns successfully after the authoring workspace is gone.
- Every artifact needed for reproduction survives outside temporary storage.
