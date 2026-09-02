# Bug log

This file tracks defects found during UltraPlan QA dogfood campaigns. Campaign test gaps are listed separately because an unexercised failure path is not evidence of a product defect.

## Open bugs

No open product bugs were identified by the two campaigns covered here.

## Fixed bugs

### BUG-018: Ignored fixture binary blocked evidence admission

- Found: 2026-09-02
- Status: Fixed
- Area: QA dogfood fixture
- Symptom: The post-permission-fix QA rerun completed mapping, investigation, arbitration, and reconciliation, then failed to freeze the evidence tree. An ignored 39,099,697-byte `ultraplan` binary in the fixture target exceeded the 32 MiB per-file isolation limit even though Git-based freshness did not report it.
- Resolution: The binary was moved out of the fixture target into the persistent campaign evidence directory and retained with its SHA-256. No isolation limit was raised.
- Verification: The next resume completed eight of eight shards, published ten evidence records, and produced assessment `pass`.

### BUG-017: Repair budget source exposed an internal label

- Found: 2026-09-01
- Status: Fixed
- Area: Repair configuration
- Symptom: Environment overrides appeared as `env`, but the public smoke contract requires `environment`.
- Resolution: `repairBudgetsFor` now normalizes the public source label without changing configuration precedence.
- Verification: `TestRepairBudgetsForUsesTypedConfigAndReportsSources`.

### BUG-016: Repair help omitted the conformance-review requirement

- Found: 2026-09-01
- Status: Fixed
- Area: CLI documentation
- Symptom: Repair help described admission mechanics without naming the required conformance-review gate.
- Resolution: The help now requires a current acceptable conformance review and passing containing smoke.
- Verification: `TestSprintHelpIsRegistered`.

### BUG-015: Ephemeral loopback serving failed before bind

- Found: 2026-09-01
- Status: Fixed
- Area: Web server
- Symptom: `serve --listen 127.0.0.1:0` failed even though the documented workflow used port `0` to request an ephemeral listener.
- Resolution: Port `0` is accepted for numeric loopback addresses, and the resolved nonzero listener is revalidated after bind.
- Verification: CLI and server tests cover IPv4 and IPv6.

### BUG-014: Evidence isolation rejected safe symlinks

- Found: 2026-09-01
- Status: Fixed
- Area: Evidence isolation
- Symptom: Target identity accepted contained relative symlinks, but evidence isolation rejected every symlink.
- Resolution: Isolation validates containment, copies link text without following it, and hashes the entry type and target. Escaping links, hard links, and special files still fail closed.
- Verification: Focused copy and identity tests.

### BUG-013: Arbiter output framing was underspecified

- Found: 2026-09-01
- Status: Fixed
- Area: QA arbitration
- Symptom: MiniMax wrapped both limit-1 arbiter responses in Markdown fences, causing strict decoding to fail.
- Resolution: The contract now requires `{` as the first byte and `}` as the last and forbids fences, backticks, prose, and commentary.
- Verification: The four-value arbiter matrix passed at limits 1, 2, 4, and 24.

### BUG-012: Arbiter repair could discard valid coverage

- Found: 2026-09-01
- Status: Fixed
- Area: QA arbitration
- Symptom: A repair response corrected one missing issue while dropping another valid issue.
- Resolution: Repair turns restate the full contract and require preservation of all other valid coverage.
- Verification: Focused arbiter contract coverage and the passing limit matrix.

### BUG-011: Arbiter references were not closed over delivered context

- Found: 2026-09-01
- Status: Fixed
- Area: QA arbitration
- Symptom: Nested theory text exposed block-like IDs that were not included in the arbiter context, allowing invalid references in output.
- Resolution: Arbiter packs now contain canonical `allowed_refs`, and every output reference must belong to that set.
- Verification: Controlled unknown-reference failures and the passing arbiter matrix.

### BUG-010: Arbiter contract and repair diagnostics were incomplete

- Found: 2026-09-01
- Status: Fixed
- Area: QA arbitration
- Symptom: The prompt omitted accepted wire values, and the repair path discarded its second failure. MiniMax returned the unsupported value `retain`.
- Resolution: The shared contract lists exact enum values and coverage rules. Errors retain both failed attempts.
- Verification: Focused contract tests.

### BUG-009: Resume remapped and conflict checks ran after mapping

- Found: 2026-09-01
- Status: Fixed
- Area: QA mapping
- Symptom: A conflicting new run could incur a paid mapper call before rejection, while a resumed run performed mapping again instead of reusing retained state.
- Resolution: `qaMapForRun` checks retained state before invoking the mapper.
- Verification: `TestQAMapForRunRejectsConflictAndReusesRetainedMapOnResume`.

### BUG-008: Legacy maps failed after the arbiter budget was added

- Found: 2026-09-01
- Status: Fixed
- Area: QA persistence
- Symptom: Legacy schema-1 maps decoded an absent `arbiter_max_theories` field as zero and were rejected.
- Resolution: The loader supplies the historical default of 24 only when upgrading legacy schema-1 maps.
- Verification: `TestQAStoreUpgradesMapFromBeforeArbiterTheoryBudget`.

### BUG-007: Complete context disabled read-only discovery

- Found: 2026-09-01
- Status: Fixed
- Area: QA permissions
- Symptom: A complete direct-input packet prohibited all discovery tools. Agents could not perform bounded verification when supplied context was incomplete or ambiguous.
- Resolution: QA roles retain read, list, search, and glob access in a read-only sandbox with mutation denied by default. Prompts still direct agents to use the supplied packet first.
- Verification: Contract tests assert available discovery tools and denied write, edit, patch, bash, and shell tools. A fresh live rerun retained 32 calls with restricted permission enforcement, permission audits, and zero unsupported tools. Agentwrap translated the explicit allows to OpenCode's native read, list, grep, and glob permissions. The model made no tool calls, so a successful end-to-end read invocation remains unproven.

### BUG-006: Repair campaign vocabulary was undefined

- Found: 2026-08-30
- Status: Fixed
- Area: Documentation
- Symptom: The CLI and configuration used "campaign" without defining its relationship to assignments and repair runs.
- Resolution: Documentation now distinguishes repair assignments, repair runs, and campaigns and defines `verified_pending_campaign`.
- Commit: `4caf1c34` (`docs: define repair campaign vocabulary`).

### BUG-005: External smoke fixture reused stale identities

- Found: 2026-08-30
- Status: Fixed
- Area: Test fixture
- Symptom: The Sprint 38 fixture used a fixed nonexistent issue ID. Repeated runs could collide with retained state and fail for an unrelated reason.
- Resolution: The fixture generates a unique valid nonexistent ID and accepts the intended class of denied-authority rejection.
- Note: The fixture is maintained outside this repository.

### BUG-004: Grouped repair queues could not finish safely

- Found: 2026-08-30
- Status: Fixed
- Area: Repair campaigns
- Symptom: Broad QA reran between every issue, and no truthful outcome represented an intermediate repair whose scoped gates passed while final containing smoke remained pending.
- Resolution: Added `verified_pending_campaign`, explicit deferred-smoke records, mandatory final containing smoke, packet rebasing, stable private worker paths, and strict authorization of intermediate pending outcomes.
- Verification: A real two-issue grouped campaign reused one model session, isolated both issues, applied mutations serially, and passed final containing smoke.
- Commit: `ffac3594` (`fix: complete grouped repair queues safely`).

### BUG-003: Real-QA evidence identity checks compared the wrong values

- Found: 2026-08-30
- Status: Fixed
- Area: Evidence integrity
- Symptom: One check compared the Git implementation fingerprint with the map fingerprint. Another could compare a bounded evidence tree identity with itself.
- Resolution: Both checks now compare independently observed identities from the intended sources.
- Verification: Real QA evidence production and adjudication passed.

### BUG-002: Equivalent repair findings were promoted more than once

- Found: 2026-08-30
- Status: Fixed
- Area: QA synthesis and adjudication
- Symptom: Equivalent findings could produce duplicate repair issues.
- Resolution: Findings are deduplicated by normalized claim, issue class, and location. The merge unions evidence, keeps the strongest severity, and preserves repair eligibility.
- Verification: Adjudication also validates group-to-issue relationships strictly.
- Commit: `324b4a97` (`fix: deduplicate QA repair findings`).

### BUG-001: Usage projection dropped token and cache counters

- Found: 2026-08-30
- Status: Fixed
- Area: Runtime telemetry
- Symptom: OpenCode returned usage in a shape the projector did not recognize. Timings persisted, but token and cache fields remained empty.
- Resolution: Agentwrap updated its usage projection to handle the returned shape.
- Verification: A rebuilt UltraPlan binary recorded input, output, cache, timing, and reported-cost fields in the next campaign.
- Dependency revision: `agentwrap@cc51e26`.

## Unverified failure paths

These are coverage gaps, not confirmed bugs.

- Live stale-proposal regeneration after an overlapping mutation.
- Live cancellation during proposal or verification.
- Live final-smoke failure after an earlier pending success.
- Queues with three or more issues.
- Campaigns with multiple non-empty worker queues.
- Session loss between queued issues.
- Runtime-store cleanup while a retained transcript is still needed for audit.

Controlled deterministic tests cover stale proposals, cancellation, journal recovery, target drift, intermediate and final smoke rules, cleanup, replay, and reconnect behavior. Live fault injection remains outstanding where noted above.
