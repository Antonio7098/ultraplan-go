# Bug log

This file tracks defects found during UltraPlan QA dogfood campaigns. Campaign test gaps are listed separately because an unexercised failure path is not evidence of a product defect.

## Open bugs

No open product bugs were identified by the two campaigns covered here.

## Fixed bugs

### BUG-034: Project review verdicts could contradict their findings

- Found: 2026-09-04
- Status: Fixed
- Area: Project reasoning review
- Symptom: The reviewer stated that no actionable contract defect remained but returned `pass_with_findings`, which kept an otherwise acceptable project blocked under a `pass` policy.
- Resolution: Review output now includes an `Actionable Findings` count. Validation requires zero findings for `pass` and at least one for `pass_with_findings` or `fail`. The prompt distinguishes actionable defects from editorial observations and future proof obligations.
- Verification: `TestReviewVerdictRequiresConsistentActionableFindingCount`; the Aren Phase 1 rerun returned `Actionable Findings: 0` and `Verdict: pass`.

### BUG-033: Stale project-reasoning reruns reused the previous artifact

- Found: 2026-09-04
- Status: Fixed
- Area: Project reasoning promotion
- Symptom: When an output file already existed, a rerun ignored the runtime's new terminal response and validated the old file. Prompt changes therefore appeared to have no effect.
- Resolution: A non-empty terminal response always becomes the new candidate. UltraPlan retains the previous artifact only for rollback when the new candidate fails validation or promotion.
- Verification: `TestProjectReasoningTerminalCandidateReplacesExistingOutput`; the corrected Aren review replaced the prior verdict while every upstream artifact resumed.

### BUG-032: Oversized project-reasoning prompts displaced stage instructions

- Found: 2026-09-04
- Status: Fixed
- Area: Project reasoning prompts
- Symptom: Aren's evidence-assessment prompt reached about 606,000 input tokens. MiniMax answered an embedded downstream template instead of the selected evidence-assessment area because the controlling prefix fell outside effective context.
- Resolution: Direct project inputs now use canonical ordering, deduplication, a deterministic 512 KiB total budget, a 64 KiB per-input limit, fair allocation, and head-and-tail excerpts. Resolved references share the same bounded allocation.
- Verification: `TestProjectReasoningDirectInputsHaveDeterministicBudget`; all six Aren areas completed without topic drift at about 110,000 input tokens for the largest later stages.

### BUG-031: Project reasoning discarded runtime configuration and terminal Markdown

- Found: 2026-09-04
- Status: Fixed
- Area: Project reasoning runtime
- Symptom: The CLI resolved the requested model but built a bare runtime request for project reasoning. It also required the model to write the target file even though the stage contract should let UltraPlan validate and promote returned Markdown.
- Resolution: Project reasoning now inherits provider, model, timeout, sandbox, permissions, and health settings from effective configuration. It accepts plain or fenced terminal Markdown while running the model read-only.
- Verification: `TestAreaFlowDirectlyInjectsAssignedStudyReport`, `TestProjectReasoningResultContentAcceptsMarkdownFence`, and the complete Aren Phase 1 run with `openrouter/minimax/minimax-m3:free`.

### BUG-030: Merge descriptions could contradict the non-fast-forward policy

- Found: 2026-09-04
- Status: Fixed
- Area: Governed sprint merge
- Symptom: A successful `ultraplan sprint ... merge --yes` created the correct two-parent merge commit, but the model-authored verification text called the relationship a "clean fast-forward."
- Resolution: The merge-description contract now states that UltraPlan always creates a non-fast-forward two-parent commit. Validation rejects claims that the target is fast-forwarded while accepting accurate `non-fast-forward` wording.
- Verification: `TestValidateMergeDescriptionRejectsFastForwardClaim`; the original contradictory description remains in the live command evidence.

### BUG-029: Pre-apply repair results always reported cleanup as incomplete

- Found: 2026-09-04
- Status: Fixed
- Area: Repair terminal publication
- Symptom: A rejected isolated proposal could remove its workspace successfully but still publish `cleanup_complete: false`.
- Resolution: Every pre-apply terminal path now passes the observed workspace and parent cleanup result into terminal publication.
- Verification: Focused repair tests pass, and the successful live promotion records complete cycle cleanup.

### BUG-028: Repair-check cleanup could not remove Go's read-only module cache

- Found: 2026-09-04
- Status: Fixed
- Area: Repair verification isolation
- Symptom: The exact reproducer passed, but cleanup failed with `permission denied` on a read-only module-cache directory, so the repair correctly stopped before apply.
- Resolution: Repair runtime cleanup makes cache directories owner-writable before removal, matching the main isolation cleanup contract.
- Verification: `TestRepairCheckEnvironmentProvidesPrivateGoRuntimeAndCleansIt` covers a read-only module tree. The next live repair passed cleanup.

### BUG-027: Repair checks omitted the private Go runtime environment

- Found: 2026-09-04
- Status: Fixed
- Area: Repair verification
- Symptom: A valid proposed fix failed with `module cache not found: neither GOMODCACHE nor GOPATH is set` because frozen checks passed only `PATH`.
- Resolution: Provisional checks, post-apply checks, and immutable reproducer reruns now receive private `HOME`, `GOCACHE`, `GOPATH`, `GOTMPDIR`, and `TMPDIR` directories outside the checked target.
- Verification: The live promotion's provisional and post-apply Go checks passed without changing target identity.

### BUG-026: Repair packets dropped the investigator test working directory

- Found: 2026-09-04
- Status: Fixed
- Area: Evidence and repair checks
- Symptom: The immutable spec required `internal/sprint`, but the repair packet ran `go test .` at the repository root.
- Resolution: Evidence plans now retain `working_directory`, repair checks copy it, and legacy authored bundles recover it from the approved test path.
- Verification: `TestFreezeRepairChecksPreservesAndRecoversWorkingDirectory`; live packet `repair-v1-run-d04767998970569ffa6c0af0` retained `workdir: internal/sprint`.

### BUG-025: Reconciled issue identity could sever authored-test coverage

- Found: 2026-09-04
- Status: Fixed
- Area: QA promotion and repair packets
- Symptom: Title or location reconciliation prevented an authored bundle from reaching the promoted issue, and missing regression coverage could be silently omitted from a repair packet.
- Resolution: Coverage follows immutable evidence identity. Repair preparation can recover bundle and spec ownership from persisted evidence, and packet validation rejects a regression candidate without signed coverage.
- Verification: Multi-theory coverage and regression-packet validation tests; the live packet contains the theory, evidence, bundle, spec, and primary reproducer.

### BUG-024: Failed durable operations blocked a corrected retry

- Found: 2026-09-04
- Status: Fixed
- Area: Durable operation admission
- Symptom: Repeating the same repair preparation after fixing a mechanism defect deduplicated to the terminal failed operation instead of creating a retry.
- Resolution: Failed and cancelled aliases now receive bounded deterministic retry aliases. Active and successful operations still deduplicate.
- Verification: `TestDurableOperationDeduplicatesAcrossManagersAndFailsClosed`; repeated live repair preparations produced new run identities.

### BUG-023: Authored-test issues lacked production repair authority

- Found: 2026-09-04
- Status: Fixed
- Area: Repair admission
- Symptom: The issue location named the private `_test.go` file, so the packet omitted the production file needed to repair the reproduced defect.
- Resolution: For investigator-authored test issues, production authority comes from the immutable originating shard's changed paths.
- Verification: `TestRepairAllowedPathsUsesImmutableShardForAuthoredTestIssue`; the live packet allowed both the test and `internal/sprint/qa_repair_state.go`.

### BUG-022: Exact-session evidence continuation and blocked QA repair admission failed

- Found: 2026-09-04
- Status: Fixed
- Area: QA continuation and repair admission
- Symptom: Arbiter continuation validated against the wrong current runtime store, and a current blocked assessment prevented an individually eligible promoted issue from reaching repair.
- Resolution: Arbiter continuation restores the retained store before identity validation. Repair admission now evaluates eligible issues from a current blocked assessment while still rejecting stale and incomplete QA.
- Verification: `TestEvidenceReturnRestoresRetainedArbiterStoreBeforeIdentityValidation`, `TestRepairAdmissionAllowsEligibleIssueFromBlockedCurrentQA`, and the completed live promotion.

### BUG-021: Nested and parallel smoke processes collided on the tsx IPC socket

- Found: 2026-09-04
- Status: Fixed
- Area: Repair reverification and external smoke harness
- Symptom: A repaired target passed every scoped gate, but containing smoke failed with `EADDRINUSE` during discovery or exited before producing run evidence. Concurrent or nested `tsx` processes inherited one `TMPDIR` and selected the same IPC socket.
- Resolution: UltraPlan now gives each smoke invocation a private temporary directory and cleans it after discovery and execution. The harness protocol now launches its nested TypeScript CLI through `node --import tsx`, which does not start a second competing tsx IPC server.
- Verification: The retained final repair run passed `containing_smoke` in 10,460 ms and ended `verified` with complete cleanup.

### BUG-020: Dogfood fixture promoted already-fixed defects

- Found: 2026-09-04
- Status: Fixed
- Area: QA dogfood fixture
- Symptom: Rebuilding the repair fixture after one successful repair recreated an issue for the healthy file, so the repair model correctly produced no diff and the campaign failed.
- Resolution: The persistent external fixture generator reads each target file and creates evidence and candidates only for defects that still reproduce. The fixture tool lives outside the target so it cannot change the target fingerprint.
- Verification: The filtered fixture created exactly one beta issue after alpha was healthy; the resulting one-issue campaign completed.

### BUG-019: Authored-test and evidence continuation state could not reliably reach promotion

- Found: 2026-09-02 through 2026-09-04
- Status: Fixed
- Area: Evidence-producing QA
- Symptom: Live runs exposed unstable plan timestamps, incomplete resume hydration, invented mapper references, weak investigator draft validation, stale evidence-request projections, over-broad re-arbitration, and a failure classifier that matched prose instead of the test marker.
- Resolution: Stabilized frozen plans; validated authored-test drafts; persisted and reloaded bundles, attempts, runs, requests, and arbiter rounds; constrained semantic references; refreshed only affected arbiter groups; and classified failures using the test name plus deterministic output marker. Recovery now tolerates and clears broken optional artifact references.
- Verification: The retained authored test reproduced invalid UTF-8 acceptance as `predicted_failure_reproduced`, promoted issue `qa-v2-issue-6461fb6e2f79834be178c8a5`, and the production regression passes after the UTF-8 guard was added.

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
