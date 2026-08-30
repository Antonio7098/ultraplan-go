# QA dogfood campaign

Date: 2026-08-28 to 2026-08-30  
Target: QA and bounded repair through Sprint 38  
Runtime model: `openrouter/minimax/minimax-m3:free`, high variant  
Repository: `ultraplan-go`

## Verdict

The read-only QA path, evidence production, adjudication, one-issue repair, grouped repair assignment, two-issue queue execution, model-session reuse, serial production mutation, issue-scoped cleanup, and final containing-smoke gate all worked in real runs.

The repair path is proven for the exercised two-issue grouped case. General grouped campaigns remain bounded by the cases tested. Cancellation, recovery during mutation, queues larger than two issues, multiple worker queues, and failures on intermediate gates still need deliberate fault-injection runs before they deserve the same confidence.

The campaign found implementation and observability defects. The fixes were committed, pushed to `origin/main`, and installed. Test workspaces, runtime databases, transcripts, and generated QA attempts stayed outside the repository as requested. The original temporary report copies were lost when their `/tmp` directory expired; this document reconstructs the retained findings and measured evidence.

## What “campaign” means

“Campaign” was added with grouped repair execution. It has a narrow meaning:

- A repair assignment is an adjudication-time ordered queue of issue IDs.
- A repair run handles exactly one issue.
- A repair campaign is the durable coordinator that executes all current assignments.

One worker owns each assignment. The worker reuses one model session across its queue. Issues never share a packet, isolated mutation copy, apply journal, or repair result. Production mutations run serially.

For a two-issue queue:

```text
adjudication
  -> assignment [issue A, issue B]
  -> campaign worker, session X
       -> issue A packet -> isolated repair -> scoped gates -> cleanup
       -> issue B packet -> isolated repair -> full gates -> cleanup
  -> campaign completed
```

An intermediate issue can finish as `verified_pending_campaign`. That outcome means the issue-scoped gates passed, production apply completed, and cleanup succeeded. The containing-smoke gate is explicitly deferred. The last issue must pass the full ladder, including containing smoke, before the campaign can complete. A pending outcome is not standalone repair success.

## Scope and method

The work concentrated on QA behavior, not Conformance Review or the TUI. Stale review and smoke prerequisites in the disposable workspace were reconciled or replaced with controlled proof when they only blocked access to the QA path under test.

The campaign used:

- A disposable clone of the workspace and a disposable target repository.
- OpenRouter’s free MiniMax M3 route for model-backed investigators and repair proposals.
- Real UltraPlan QA and repair commands against the rebuilt binary.
- Durable operation, QA attempt, repair result, campaign, and OpenCode runtime records for inspection.
- Existing Go tests, race tests, vet, and the external Sprint 38 smoke fixture after source fixes.

No test result or provider database was committed. The source fixes and durable reports are committed.

## Workflow exercised

```text
admission
  -> deterministic QA map
  -> read-only investigators
  -> approved checks in isolated evidence copies
  -> synthesis and theory deduplication
  -> adjudication and issue promotion
  -> repair assignments
  -> frozen issue packet
  -> model proposal in isolated repair copy
  -> product-owned patch validation and serial apply
  -> issue-scoped verification
  -> final containing smoke
  -> durable campaign result
```

## Sprint coverage

| Sprint | Capability | Evidence status |
|---|---|---|
| 36, `36-read-only-qa` | Read-only QA foundation | Previously proven end to end: 11/11 shards, 34 theories, 11 accepted evidence records |
| 37, `37-evidence-qa-smoke` | Evidence workspaces and smoke integration | Previously proven end to end: 10/10 shards, 46 theories, 18 accepted evidence records |
| 38, `38-bounded-repair` | Bounded repair and observability | Exercised during this campaign, including a real two-issue grouped queue |

The prior Sprint 36 run ended `pass_with_findings` with 12 confirmed, 5 refuted, 14 inconclusive, and 3 blocked theories. The prior Sprint 37 run ended `pass_with_findings` with 32 confirmed, 13 inconclusive, and 1 refuted theory. Those runs established the investigator and evidence base used by the Sprint 38 work.

## Proven grouped repair run

The final proof used campaign `repair-campaign-v1-25669934257fb8cc03a87524`.

| Fact | Result |
|---|---|
| Campaign status | `completed` |
| Queue completion | 2 of 2 issues verified |
| Shared model session | `ses_fb5ea385cffeCvqzbMvihJJumE` |
| Repository mutation | Serial, one issue at a time |
| Issue isolation | Fresh copy per issue at a stable private worker path |
| Final global gate | Containing smoke passed on issue 2 |
| Campaign wall time | About 110 seconds |

### Issue 1

Repair run `repair-v1-run-1d4566d5b18fb2a026f326ee`:

| Measurement | Value |
|---|---:|
| Outcome | `verified_pending_campaign` |
| Scoped gates | 5 passed |
| Containing smoke | `deferred` |
| Runtime duration | 24.593 s |
| Verification commands | 5 |
| Input tokens | 84 |
| Output tokens | 46 |
| Cache-read tokens | 25,074 |
| Total reported tokens | 25,204 |
| Cleanup | Proven |
| Complete ladder | No, intentionally deferred |

### Issue 2

Repair run `repair-v1-run-ba83f9ffa31986df3696d335`:

| Measurement | Value |
|---|---:|
| Outcome | `verified_with_findings` |
| Verification gates | All 6 passed |
| Containing smoke | Passed in 19.917 s |
| Runtime duration | 27.121 s |
| Verification commands | 6 |
| Input tokens | 84 |
| Output tokens | 21 |
| Cache-read tokens | 27,763 |
| Total reported tokens | 27,868 |
| Cleanup | Proven |
| Complete ladder | Yes |

Both issue runs recorded the same model session. The second issue therefore proves session continuation rather than two coincidentally separate workers. Each issue still used a fresh isolated copy and removed it after completion.

## Usage and observability

The repaired usage path is:

```text
agentwrap.Usage
  -> QAInvestigatorAttempt or repair runtime observation
  -> shard, cycle, result, assessment, and campaign projections
  -> persisted JSON and web status
```

The provider’s flat-versus-nested usage mismatch had previously hidden token counts even though timings were present. Updating `agentwrap` to `cc51e26` restored input, output, cache, timing, and reported-cost propagation without a configuration change.

The grouped proof recorded 168 fresh input tokens, 67 output tokens, and 52,837 cache-read tokens across the two repair proposals. Provider cost was zero on the selected free OpenRouter route. “Zero” here is the provider-reported price for that route, not an estimate that the work consumed no compute.

Durable records expose:

- Provider, model, variant, and retained session ID.
- Start, completion, and duration facts.
- Input, output, cache-read, and cache-write tokens when reported.
- Provider cost or local rate-table estimate when available.
- Per-command duration, output bytes, truncation, and gate result.
- Repair counters, cleanup, mutation, and final outcome.
- Campaign worker queues, current item, completed count, and per-item result.

The OpenCode database also retained the full model session transcript by default until runtime-store cleanup policy expires it. It was intentionally not copied into this repository because the user requested that test results remain disposable.

## Bugs found and fixed

### Usage projection mismatch

OpenCode returned token data in a shape the projector did not recognize. Timings survived, but token and cache counters remained empty. `agentwrap@cc51e26` fixed the projection. A rebuilt UltraPlan binary recorded all fields on the next run.

### Duplicate repair findings

Synthesis and adjudication could promote equivalent findings more than once. The fix deduplicates on normalized claim, issue class, and location. It unions evidence, retains the strongest severity, and preserves repair eligibility if any equivalent finding is eligible. Adjudication now validates group-to-issue relationships strictly.

Commit: `324b4a97`, `fix: deduplicate QA repair findings`.

### Real-QA evidence identity comparison

Two identity checks compared the wrong values. The Git implementation fingerprint was compared with the map fingerprint, and the bounded evidence tree’s before/after check could compare a value with itself. Both comparisons now use the intended independently observed identities.

### Grouped queue could not finish safely

The first implementation reran broad QA between every issue and did not have a truthful result for a repaired intermediate item whose global smoke gate had not run. That made session reuse and a complete two-issue queue impractical to prove.

The fix added:

- `verified_pending_campaign` for intermediate issue success.
- A `deferred` containing-smoke gate with a recorded reason and next action.
- A full containing-smoke requirement on the final issue.
- Rebase of the next packet to the observed current target under the internal writer-fenced campaign authority.
- A stable private worker path. Each issue removes and recreates a fresh isolated copy at that path, allowing model-session continuation without sharing mutation state.
- Strict validation that the pending outcome appears only in an authorized intermediate campaign item.

Commit: `ffac3594`, `fix: complete grouped repair queues safely`.

### External smoke fixture admitted stale identities

The Sprint 38 external fixture used a fixed nonexistent issue identifier. Repeated runs could collide with retained state and fail for a valid but unintended reason. The fixture now creates a unique valid nonexistent ID and accepts any nonzero rejection where the assertion concerns denied automatic authority rather than one exact rejection branch. This fixture lives outside the `ultraplan-go` Git repository and was not included in its commits.

### Vocabulary was undefined

“Campaign” appeared in the CLI and configuration before the architecture defined it. The docs now distinguish assignment, repair run, and campaign and explain `verified_pending_campaign`.

Commit: `4caf1c34`, `docs: define repair campaign vocabulary`.

## Source verification

The final source changes passed:

```text
go test ./internal/platform/process ./internal/sprint ./internal/web
go test ./internal/app -run 'Repair|QAAdjudication|SprintQA'
go test -race ./internal/platform/process ./internal/sprint
go vet ./internal/platform/process ./internal/sprint ./internal/app ./internal/web
git diff --check
```

The broad application suite still encounters the known unrelated timeout in the existing study run-loop test. Repair-focused application tests passed. This limitation was not reclassified as repair proof.

The final campaign source commit was pushed and installed. At the time of the proof, the installed binary SHA-256 was:

```text
bbc32f958148c22a6000d100390fa4125eb4791b014a1112dccf0819591c9817
```

The later documentation-only commit did not require reinstalling the binary.

## What is proven

- A grouped assignment can contain two repair-eligible issues.
- One worker can retain and reuse the same model session across both issues.
- Every issue still gets a separate packet, isolated copy, repair run, apply, result, and cleanup.
- Production mutation remains serial.
- An intermediate issue can finish truthfully without claiming the global ladder passed.
- The final issue runs and passes containing smoke before campaign completion.
- Token, cache, timing, command, cleanup, and outcome facts persist and appear in projections.
- The source survives focused tests, race detection, vet, an external smoke fixture, and real dogfood execution.

## What remains unproven

- A queue with three or more issues.
- Multiple non-empty worker queues in one campaign.
- `per_issue` and `grouped` comparison under the same fixture.
- Cancellation during proposal, journal apply, verification, and the handoff between issues.
- Recovery from a dead runtime after production apply but before result publication.
- Compensation after exact postimage detection and escalation after target drift.
- A failed intermediate scoped gate and a failed final containing-smoke gate.
- Session loss between queued issues.
- Web observation under a long-running, multi-worker campaign with reconnect and event replay.
- Runtime-store cleanup while a retained transcript is still needed for audit.

These are the next fault-injection targets. The exercised path is real and coherent, but “fully proven” would be too strong until these terminal boundaries have run.

## Recommended next test matrix

| Test | Expected result |
|---|---|
| Three issues in one queue | First two pending, third full verification, one session ID |
| Two queues of two issues | One session per queue, four serial production applies |
| Cancel during proposal | No production mutation, durable cancelled terminal |
| Kill after journaled apply | Recovery verifies or compensates exact known bytes |
| Change target before recovery | Escalation, never inferred success or blind rollback |
| Fail issue 2 scoped gate | Campaign stops, issue 3 stays queued, failure is durable |
| Fail final smoke | Campaign fails despite earlier pending successes |
| Remove retained session | Explicit runtime-unavailable failure before next mutation |
| Disconnect and reconnect web client | Canonical progress resumes without duplicated events |

## Record-retention note

The original campaign artifacts were written under `/tmp/ultraplan-repair-loop-dogfood-20260828` and were not committed. That directory expired, which removed the first Markdown and HTML report copies along with the disposable evidence. This reconstructed report lives under `docs/reports` so future repository cleanup does not erase the baseline.

