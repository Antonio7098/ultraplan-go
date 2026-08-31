# QA context-engineering dogfood campaign handoff

Prepared: 2026-08-31
Target: semantic QA mapping, bounded arbitration, issue reconciliation, direct context injection, cache-prefix construction, and sequential and parallel repair
Repositories: `ultraplan-go` and `agentwrap`
Status: handoff, not execution evidence

Baseline: [QA dogfood campaign](qa-dogfood-campaign.md)

Fixed fixture: project `ultraplan-go`, sprint `38-bounded-repair`
Expected preserved target ref: `ultraplan/ultraplan-go/38-bounded-repair`
Expected preserved target commit: `accda5dff06d9b5debac9f0e3782a7dab6ea32f6`
Runtime route: `openrouter/minimax/minimax-m3:free`, high variant

The archived fixture manifest from the previous campaign is authoritative. If
it records a different target commit, use that exact commit and record the
discrepancy. Do not silently substitute current `main`.

## Verdict this campaign must earn

The campaign should answer one question: can the new QA path find, reconcile,
repair, and verify real defects without hiding missing context, duplicate issues,
stale patches, failed checks, or uncertain cleanup?

A green campaign proves the exercised fixtures and failure boundaries. It does
not prove every repository, model, provider, or repair conflict. The final
report must keep that distinction, as the previous QA campaign did.

## What changed under test

```text
frozen sprint diff and governed inputs
  -> semantic mapper agent
  -> canonical shard context packs
  -> parallel investigator agents
  -> deterministic theory grouping by shared code context
  -> bounded arbiter agents
  -> issue reconciliation agent
  -> deterministic evidence admission and adjudication
  -> one isolated repair proposal per issue
       -> sequential proposal path
       -> parallel proposal path
  -> ordered single-writer integration
  -> authoritative post-integration verification
```

The semantic mapper, investigators, arbiters, reconciler, evaluator, and repair
workers are model-backed. Theory grouping, pack materialization, reference
validation, evidence execution, adjudication, patch admission, integration,
and final verification are product-owned code.

## Campaign rules

- Use a disposable workspace and a disposable target repository for every
  model-backed or mutation test.
- Do not point a campaign workspace at either source checkout.
- Do not run repair against the production planning workspace or production
  implementation checkout.
- Do not run Conformance Review. This campaign tests QA, not review quality.
- Reconcile review and smoke admission only inside each disposable workspace
  with clearly marked campaign-only reports and current fixture fingerprints.
- Freeze the source identities, source dirty-state digest, binary digest,
  effective config, model routes, and fixture identities before the first run.
- Keep provider credentials in the runtime credential store or process
  environment. Do not copy them into the campaign directory.
- Do not retain full prompts, raw provider payloads, unrestricted transcripts,
  full environment dumps, production file contents, or private preimages in the
  report.
- Treat `qa status`, product JSON, immutable QA artifacts, repair journals, and
  target Git identities as evidence. Terminal text alone is not evidence.
- Stop on uncertain cleanup, target drift, an unexplained production mutation,
  or a malformed durable record. Do not continue to collect a more favorable
  result.
- Do not weaken a failing acceptance criterion to make the campaign pass.

## Required inputs

The operator needs:

- A source ref containing the new UltraPlan QA implementation.
- A source ref containing the new `agentwrap` cache contract.
- The same disposable Sprint 38 workspace seed used by the previous campaign.
- The same `ultraplan-go` implementation snapshot used by that campaign.
- The `openrouter/minimax/minimax-m3:free` provider route.
- A qualifying manual repair proof before automatic campaign lanes.
- The prior Sprint 38 bounded containing-smoke fixture.

Do not add new seeded defects to the real-run snapshot. Compare the new QA
results with the previous campaign's retained findings and two-issue repair
proof. Use offline fixtures for malformed output, invented references, stale
preimages, and other forced failures that the preserved snapshot does not
naturally produce.

## Review and smoke prerequisite shortcut

No lane may invoke `sprint ... review`, `flow --to review`, or any command that
implicitly starts review. A review runtime call is a campaign failure.

Start from the previous campaign's disposable Sprint 38 state when available.
If its review or smoke prerequisite is stale under the copied fixture:

1. Run only runtime-free status and fingerprint inspection.
2. Place minimal `review.md` and `smoke.md` reports in the disposable sprint.
3. Mark both reports `DOGFOOD PREREQUISITE ONLY, NOT REVIEW EVIDENCE`.
4. Reconcile only the disposable `flow-state.json` review and smoke fields to
   the exact current target and governed-input fingerprints.
5. Validate the state through normal read-only status commands.
6. Keep the before and after prerequisite files in the private campaign
   evidence directory, but do not commit or publish them.

The reports exist only to satisfy QA admission. Do not score their contents as
review or smoke proof. If admission remains blocked, fix the disposable state
or the campaign preparation script. Do not run review to get past it. Repair's
final containing-smoke gate should use the small preserved Sprint 38 fixture,
not a broad planning or review workflow.

## Fixed model routing

Configure every QA model role to the same route and high variant:

```yaml
qa:
  model: openrouter/minimax/minimax-m3:free
  variant: high
  mapper_model: openrouter/minimax/minimax-m3:free
  mapper_variant: high
  investigator_model: openrouter/minimax/minimax-m3:free
  investigator_variant: high
  challenger_model: openrouter/minimax/minimax-m3:free
  challenger_variant: high
  arbiter_model: openrouter/minimax/minimax-m3:free
  arbiter_variant: high
  reconciler_model: openrouter/minimax/minimax-m3:free
  reconciler_variant: high
  evaluator_model: openrouter/minimax/minimax-m3:free
  evaluator_variant: high
  repair_model: openrouter/minimax/minimax-m3:free
  repair_variant: high
```

Do not use a stronger arbiter route in this campaign. Do not accept fallback to
another provider or model. Every successful model call must record
`openrouter/minimax/minimax-m3:free` and variant `high`; otherwise the run is
not comparable with the previous campaign.

## Build and source freeze

Do not add a local `replace` directive to `ultraplan-go/go.mod`. Build the
candidate through a temporary Go workspace so both candidate modules are used
without changing either repository.

```bash
export UP_SOURCE=/absolute/path/to/ultraplan-go
export AGENTWRAP_SOURCE=/absolute/path/to/agentwrap
export CAMPAIGN_ROOT="$(mktemp -d /tmp/ultraplan-qa-context-dogfood-XXXXXXXX)"
export CAMPAIGN_EVIDENCE="$CAMPAIGN_ROOT/evidence"
mkdir -p "$CAMPAIGN_EVIDENCE" "$CAMPAIGN_ROOT/bin"
cd "$CAMPAIGN_ROOT"
go work init "$UP_SOURCE" "$AGENTWRAP_SOURCE"
cd "$UP_SOURCE"
GOWORK="$CAMPAIGN_ROOT/go.work" go build -trimpath -o "$CAMPAIGN_ROOT/bin/ultraplan" ./cmd/ultraplan
sha256sum "$CAMPAIGN_ROOT/bin/ultraplan"
git -C "$UP_SOURCE" rev-parse HEAD
git -C "$AGENTWRAP_SOURCE" rev-parse HEAD
git -C "$UP_SOURCE" status --short
git -C "$AGENTWRAP_SOURCE" status --short
git -C "$UP_SOURCE" diff --binary | sha256sum
git -C "$AGENTWRAP_SOURCE" diff --binary | sha256sum
GOWORK="$CAMPAIGN_ROOT/go.work" go list -m all
```

Record the outputs in a small source manifest. A dirty source tree is allowed
only when its binary diff digest is recorded. A later rebuild after a fix gets
a new manifest and binary digest.

## Evidence layout

Keep the working evidence outside both repositories:

```text
evidence/
  source-manifest.md
  fixture-manifest.md
  command-ledger.tsv
  runtime-calls.jsonl
  runtime-call-totals.json
  lane-00-offline/
  lane-01-semantic-map/
  lane-02-arbiter-limits/
  lane-03-reconciliation/
  lane-04-cache/
  lane-05-repair-sequential/
  lane-06-repair-parallel/
  lane-07-faults/
  final-measurements.json
```

For every command, record the lane, start and finish time, sanitized argv,
exit code, binary digest, workspace path, target commit, operation run ID, QA
attempt ID, and repair or campaign ID where applicable. Store bounded stdout
and stderr separately. Hash any large artifact instead of copying its body.

## Per-call runtime telemetry is a hard gate

Record one durable telemetry row for every runtime invocation, not one row per
top-level QA command. Aggregates are useful, but they cannot replace the call
rows from which they were calculated.

The call ledger must distinguish:

- Semantic mapper call and any mapper output-repair continuation.
- Every investigator initial call and context-expansion continuation.
- Every challenger call and output-repair continuation.
- Every arbiter group call and output-repair continuation.
- Issue reconciler call and output-repair continuation.
- Each of the three failed-evidence evaluator calls.
- Every repair proposal call, including a stale-proposal regeneration.
- Any model-backed containing-smoke call that the repair path cannot avoid.

Each row in `runtime-calls.jsonl` must contain:

| Field group | Required values |
|---|---|
| Identity | operation run ID, QA attempt ID, call ID, global sequence, stage sequence, stage, role, shard, group, evidence or issue ID |
| Route | provider, model, variant, fallback status, final target index |
| Session | session ID, turn ID where available, fresh or continuation action |
| Timing | started time, completed time, duration milliseconds, terminal status |
| Prompt | total bytes, stable-prefix bytes, prefix SHA-256, cache cohort, cache mode, cache transport |
| Tokens | input, output, reasoning, cache-read, cache-write, and total token values plus a `known` flag for each |
| Tools | total observed tool calls, exactness flag, calls by tool kind, distinct files read, read bytes, repeated reads, and search calls |
| Events | total runtime events, retained events, dropped events, warnings, and bounded error category |
| Cost | amount, currency, estimate flag, and source when available |
| Permissions | sandbox, permission mode, default action, unsupported count, and audit count |

For successful provider calls, input, output, cache-read, cache-write, and total
tokens must all be known. A known zero is valid. An unknown value is not zero.
If OpenRouter omits one of those fields, preserve `known: false`, record the
provider omission, and mark the campaign incomplete until projection or route
handling is fixed. Reasoning tokens follow the same representation; if the
route does not expose them, the row must say so explicitly.

Tool counts are mandatory even for no-tool roles. Mapper, arbiter, reconciler,
and evaluator calls with denied tools should record a known total of zero.
Investigator and repair calls should record both total calls and counts by tool
kind. Output-repair and continuation calls get their own rows. Do not fold them
into the original call and lose their token or tool cost.

Deterministic stages also need rows in the final measurement table, but their
runtime fields should be `not_applicable`, not zero. Record command counts,
durations, output bytes, and results for evidence execution, provisional repair
checks, integration, and authoritative verification.

UltraPlan persists this shape in the sprint's schema-2
`.runtime-metrics.json`. QA and repair fail if their metric row cannot be
written. Schema-1 ledgers from the previous campaign are upgraded without
discarding their token records. Lane 00 must prove the shape before any
provider call. Do not proceed with a campaign whose evidence cannot answer how
many tokens or tool calls each model-backed stage consumed.

At the start of each lane, record the existing call count. After the lane,
export only the new rows through the read-only command:

```bash
METRICS_BEFORE="$($ULTRAPLAN_BIN --workspace "$DOGFOOD_WORKSPACE" \
  sprint "$DOGFOOD_PROJECT" "$DOGFOOD_SPRINT" metrics --json | jq '.runs | length')"

# Run the lane here.

$ULTRAPLAN_BIN --workspace "$DOGFOOD_WORKSPACE" \
  sprint "$DOGFOOD_PROJECT" "$DOGFOOD_SPRINT" metrics --json \
  > "$LANE_EVIDENCE/runtime-metrics.json"
jq -c --argjson first "$METRICS_BEFORE" '.runs[$first:][]' \
  "$LANE_EVIDENCE/runtime-metrics.json" \
  >> "$CAMPAIGN_EVIDENCE/runtime-calls.jsonl"
```

Never copy `.runtime-metrics.json` from one clone into another. It belongs to
the sprint clone that produced it.

### Web ledger verification

Start the dashboard against the same disposable workspace; do not create a
second metrics fixture for this check:

```bash
"$ULTRAPLAN_BIN" --workspace "$DOGFOOD_WORKSPACE" serve --listen 127.0.0.1:0
```

Open
`/projects/ultraplan-go/sprints/38-bounded-repair/metrics`. Save one desktop
and one narrow-viewport screenshot in the lane evidence. The page passes only
when:

- Its call count equals `.runs | length` from `metrics --json`.
- Its complete-token, unknown-token, exact-tool, tool-call, event, stage, and
  cost totals agree with the exported ledger.
- Expanding a mapper, investigator, arbiter, reconciler, evaluator, repair,
  and output-repair row exposes the matching identity, route, session,
  timestamp, prompt/cache, token, tool, event, permission, and outcome fields.
- Known zero values render as zero and unknown provider values render as
  `not reported`; neither is silently substituted for the other.
- Output-repair and context continuations are separate ordered rows with their
  session action and `repair_of` lineage visible.
- The ledger remains readable without horizontal page overflow at both tested
  viewports. Wide stage tables may use their bounded local scroll container.

The page is an observation surface over the schema-2 sprint ledger. A UI row
is not independent evidence when the underlying JSON row is absent or wrong.

## Release gates

All gates in this table are mandatory unless the final report marks the
campaign failed or incomplete.

| Area | Required evidence |
|---|---|
| Source | Focused tests, race tests, vet, and clean diff checks pass |
| Telemetry | Every runtime call has a durable per-call row with route, timing, token, cache, tool, event, permission, and cost facts |
| Web ledger | The dedicated sprint metrics page exposes every retained row and its totals at desktop and narrow viewports |
| Mapper | Real model execution, no fallback, valid mapper record, every changed path owned exactly once |
| Packs | Every runnable shard records `pack_complete`, nonzero prompt bytes, and a valid context-block set |
| Investigators | All selected shards terminate truthfully and retain usage, context, and output diagnostics |
| Grouping | Every arbiter group stays within the configured theory limit and grouping is deterministic for identical inputs |
| Arbiter | Strong route is recorded, invalid references fail closed, false theory is not promoted |
| Reconciliation | Every retained confirmed theory occurs in exactly one reconciled issue and cross-group duplicates collapse |
| Evidence | No issue becomes repair-eligible without admitted failing evidence |
| Direct injection | Delivered block IDs and content digests match the frozen foundation and shard pack |
| Cache construction | Sibling investigators have byte-identical recorded prefixes and exact digest and byte counts |
| Sequential repair | Separate issue packets and copies, ordered applies, required provisional and final gates |
| Parallel repair | Proposal time overlaps, production apply remains serial, stale proposal is rejected or regenerated |
| Recovery | Cancellation and interrupted apply settle to a truthful durable terminal state |
| Cleanup | Every isolated workspace and mutation lease has recorded complete cleanup |

Provider cache efficiency is not a correctness gate, but complete cache token
accounting is a telemetry gate.
OpenCode 1.18.25 does not expose a caller-defined provider routing key or an
intra-message byte breakpoint. The campaign must not report native breakpoint
support. It should report provider-managed message caching and the observed
cache-read and cache-write token fields when the provider supplies them.

## Lane 00: offline contract proof

Run this before spending provider tokens:

```bash
cd "$UP_SOURCE"
GOWORK="$CAMPAIGN_ROOT/go.work" go test ./cmd/... ./internal/...
GOWORK="$CAMPAIGN_ROOT/go.work" go test -race ./internal/platform/runtime ./internal/sprint
GOWORK="$CAMPAIGN_ROOT/go.work" go vet ./cmd/... ./internal/...
git diff --check

cd "$AGENTWRAP_SOURCE"
GOWORK="$CAMPAIGN_ROOT/go.work" go test ./...
GOWORK="$CAMPAIGN_ROOT/go.work" go vet ./...
git diff --check
```

Retain the known repository-wide UltraPlan fixture limitation separately if it
still exists: `studies/agent-harness-study/sources/letta/tests/data` contains a
C++ fixture that `go test ./...` rejects when cgo and SWIG are not active. Do
not relabel that package setup failure as a QA failure or a green broad suite.

The focused proof must include tests for:

- Semantic mapper failure stopping evidence-producing QA.
- Deterministic grouping stability and maximum size.
- Reconciler rejection of repeated or missing theory membership.
- Exact prompt-prefix digest validation.
- Identical investigator prefixes for sibling shards.
- Sequential and parallel campaign state validation.
- Rejection of stale parallel proposal preimages.
- Provisional checks running before canonical target mutation.
- Schema-1 runtime metrics upgrading without losing prior token rows.
- QA and repair stopping when required runtime telemetry cannot be persisted.
- Evaluator and both repair proposal paths writing call rows.

## Lane 01: real semantic mapping and investigation

Create a fresh campaign workspace from the seed. Point its project index at a
fresh disposable target clone. Record both Git identities before admission.

```bash
export ULTRAPLAN_BIN="$CAMPAIGN_ROOT/bin/ultraplan"
export DOGFOOD_WORKSPACE=/absolute/path/to/disposable/workspace
export DOGFOOD_PROJECT=ultraplan-go
export DOGFOOD_SPRINT=38-bounded-repair

"$ULTRAPLAN_BIN" --workspace "$DOGFOOD_WORKSPACE" config show --json
"$ULTRAPLAN_BIN" --workspace "$DOGFOOD_WORKSPACE" health --json
"$ULTRAPLAN_BIN" --workspace "$DOGFOOD_WORKSPACE" sprint "$DOGFOOD_PROJECT" "$DOGFOOD_SPRINT" qa --dry-run --json
"$ULTRAPLAN_BIN" --workspace "$DOGFOOD_WORKSPACE" sprint "$DOGFOOD_PROJECT" "$DOGFOOD_SPRINT" qa --json
"$ULTRAPLAN_BIN" --workspace "$DOGFOOD_WORKSPACE" sprint "$DOGFOOD_PROJECT" "$DOGFOOD_SPRINT" qa status --json
```

Inspect `verification/state.json`, the current attempt `map.json`, each shard
record, `synthesis.json`, `adjudication.json`, `issues.json`, and
`assessment.json`.

Record:

- Mapper executor, model, prompt bytes, prefix bytes, prefix digest, and
  fallback flag.
- Foundation fingerprint, block count, block kinds, total bytes, and omissions.
- Changed-path ownership across primary shards.
- Mapper-selected context paths, context block IDs, risk tags, and concerns.
- Per-shard pack completeness, prompt bytes, prefix bytes, attempt count,
  theories, context requests, tool calls, usage, and terminal reason.
- Evidence outcomes and admitted issue count.
- Target identity before and after read-only QA. They must match.

Manually compare the semantic map with the retained Sprint 38 report, archived
fixture manifest, and prior issue evidence. A map is not good merely because it
is valid JSON. It should keep coupled code together, identify boundary
behavior, and avoid irrelevant context. Record misses and overbroad shards as
findings even if investigators later recover through tools.

## Lane 02: arbiter theory-limit matrix

Run identical fixture clones with `arbiter_max_theories` set to `1`, `2`, `4`,
and the default `24`. Use the same mapper, investigator, arbiter, reconciler,
provider, model, variant, target bytes, and governed inputs.

```bash
ULTRAPLAN_QA_ARBITER_MAX_THEORIES=2 \
  "$ULTRAPLAN_BIN" --workspace "$DOGFOOD_WORKSPACE" \
  sprint "$DOGFOOD_PROJECT" "$DOGFOOD_SPRINT" qa --json
```

Use a fresh workspace clone for each value. Do not delete or rewrite a prior
attempt to force a rerun.

For each run, record:

- Total theories and arbiter groups.
- Theory count and context block count per group.
- Group model, overrides, provisional issues, and fallback status.
- Reconciled issue count and theory membership.
- Wall time, prompt bytes where retained, input tokens, cache tokens, output
  tokens, and cost where known.

No group may exceed the configured limit. Re-running deterministic grouping on
the same retained theories must produce the same group IDs and memberships.
Model wording and exact issue text need not be byte-identical.

Compare the `1` and `24` runs closely. If the smaller groups produce duplicate
provisional issues, reconciliation should collapse them without losing theory
or evidence references. A material verdict change needs explanation and a
reproduction, not dismissal as model variance.

## Lane 03: issue reconciliation

This lane needs at least one defect described by theories from separate arbiter
groups. Force the boundary with `arbiter_max_theories: 1` if needed.

The reconciler passes only when:

- Its configured model and variant are recorded.
- `fallback` is false.
- Every provisional theory appears exactly once in the final reconciled set.
- Equivalent defects become one issue.
- Distinct root causes remain separate.
- Evidence references are the union of supplied valid references.
- The strongest justified severity survives deduplication.
- No reconciled issue becomes repair-eligible without deterministic admitted
  failing evidence.

Run controlled negative tests with fake runtime output for unknown theory IDs,
duplicate membership, omitted membership, invented evidence IDs, malformed
JSON, and over-budget output. Each case must stop or fail closed. The product
must not fall back to a deterministic semantic guess after a bad reconciler
response.

## Lane 04: direct inputs and cache behavior

Use at least two sibling investigator shards. Their common frozen foundation
must be byte-identical and precede shard-specific context.

From the mapper and shard records, verify:

- `prefix_bytes` is positive and no greater than `prompt_bytes`.
- `prefix_digest` is 64 hexadecimal characters.
- Sibling attempts in the same cohort record the same prefix bytes, digest,
  model, variant, sandbox, permission policy, and target work directory.
- The bytes described by the breakpoint hash to the recorded digest in an
  instrumented offline test.
- Every cited context block exists in the frozen foundation or the supplied
  shard-specific block set.
- The model does not need to reread directly supplied governed inputs merely to
  discover their contents. Additional repository reads must have a stated gap.

Run the same unchanged QA fixture twice at the same absolute workspace and
target paths. Archive the first disposable workspace by renaming it, recreate
the original path from the frozen seed, then run again. This keeps cache cohort
inputs stable without manually deleting product state.

Record cache-read and cache-write tokens by role where available. Also record
unknown fields as unknown. Do not turn unknown values into zero.

The cache report must say:

- UltraPlan produced exact stable-prefix metadata.
- `agentwrap` validated the prefix and preserved prompt bytes.
- OpenCode used provider-managed message caching.
- Caller routing-key and native byte-breakpoint application remained false.
- Provider-reported cache hits, if any, were observations rather than a
  guaranteed contract.

If role-level attribution is missing for mapper, arbiter, or reconciler calls,
record an observability gap. Do not infer those values from aggregate process
usage.

## Lane 05: sequential repair

Use a fresh clone of the preserved two-issue Sprint 38 fixture. Set:

```yaml
qa:
  repair_assignment_mode: grouped
  issues_per_repair_agent: 2
  repair_execution_mode: sequential
```

Create the required real manual proof first. Then run the automatic campaign:

```bash
"$ULTRAPLAN_BIN" --workspace "$DOGFOOD_WORKSPACE" \
  sprint "$DOGFOOD_PROJECT" "$DOGFOOD_SPRINT" \
  repair campaign --confirmer dogfood-operator --yes --json
```

Required evidence:

- One assignment with the same two ordered issue IDs retained by the previous
  campaign.
- One retained model session for the queue.
- Two separate issue packets, isolated workspaces, patch records, apply
  journals, cycle records, and results.
- Issue 1 may finish `verified_pending_campaign` only after scoped verification
  and cleanup.
- Issue 2 must run the complete ladder, including containing smoke.
- Every later packet observes the integrated target identity from the prior
  issue.
- Production writes are serial and journaled.
- The final target contains only admitted paths and bytes.
- The campaign cannot complete if the final containing smoke fails.

## Lane 06: parallel repair

Start from an identical fresh fixture, then set:

```yaml
qa:
  concurrent_investigators: 2
  repair_assignment_mode: per_issue
  issues_per_repair_agent: 1
  repair_execution_mode: parallel
```

The same two retained issues must create two non-empty worker queues. Do not add
a third defect or change acceptance criteria merely to manufacture contention.

Required evidence:

- At least two proposal runtime intervals overlap in wall-clock time.
- Each proposal runs in its own private copied workspace.
- Each distinct frozen provisional check passes before that proposal reaches
  integration.
- Proposal work never mutates the canonical target.
- Integration remains ordered and single-writer.
- Apply journals form a continuous before and after target-identity chain.
- If the retained issues naturally overlap, the stale proposal fails its
  preimage check after the earlier apply and is regenerated against the current
  target. Otherwise prove this boundary in Lane 07 with an offline controlled
  proposal fixture.
- No blind patch application or Git merge occurs.
- Each issue still has its own packet, repair run, cycle, result, and cleanup.
- Final containing smoke passes on the fully integrated target.

Compare sequential and parallel results on the identical fixture:

- Both should resolve the seeded issues and pass the same authoritative tests.
- Final source bytes may differ if both patches are valid.
- Parallel mode should reduce proposal wall time when proposal intervals
  overlap. Do not claim a speedup from process start times alone.
- Token and cache comparisons must use known provider values only.

## Lane 07: failure and recovery matrix

Run every case against a disposable clone. Validate a process identity before
sending a signal. Never use a broad process-name kill.

| Fault | Expected result |
|---|---|
| Mapper runtime unavailable | QA fails before investigators and publishes no semantic fallback map |
| Mapper malformed output twice | QA fails closed with bounded diagnostics |
| Arbiter unknown references | Group fails closed and no issues are promoted |
| Reconciler duplicate theory membership | Reconciliation fails closed |
| Provisional repair check fails | Proposal rejected, canonical target unchanged |
| Cancel during parallel proposal | No production mutation, durable cancelled terminal, cleanup complete |
| Cancel during ordered verification | Known apply state retained, no inferred success |
| Runtime dies after journaled apply | Recovery verifies exact postimage or compensates exact known bytes |
| Target drifts before recovery | Escalation, no blind rollback or second apply |
| Earlier repair stales later proposal | Controlled offline proposal is discarded and regenerated |
| Intermediate scoped gate fails | Campaign stops and later queued issues remain unrun |
| Final containing smoke fails | Campaign fails despite earlier pending successes |
| Retained queue session disappears | Explicit failure before the next unsafe mutation |
| Web client disconnects and reconnects | Canonical progress resumes without duplicate semantic events |

For interruption cases, retain the operation run ID, writer generation,
campaign ID, repair run ID, journal digest, target fingerprint, cleanup record,
and recovery output. A process exit without these facts is not enough.

## Measurements to publish

The final report should contain one table per real run with:

- Source and binary digests.
- Workspace and target fixture IDs.
- Model and variant for every role.
- Mapper prompt and prefix bytes.
- Foundation and shard counts.
- Theory totals and outcomes.
- Arbiter group sizes and context-block counts.
- Provisional and reconciled issue counts.
- Evidence outcomes and repair-eligible issue count.
- Input, output, reasoning, cache-read, cache-write, and total tokens when known.
- Provider cost or local estimate, with its source.
- Proposal and campaign wall time.
- Session IDs by worker queue.
- Changed paths and byte counts without patch bodies.
- Verification gates, duration, output bytes, and result.
- Cleanup and recovery result.

Publish a small comparison table for the arbiter limits and another for
sequential versus parallel repair.

## Bug handling during the campaign

When the campaign finds a defect:

1. Stop the affected lane and preserve bounded evidence.
2. Reproduce it on a fresh clone with the same source, binary, config, and
   fixture identities.
3. State the violated invariant before changing code.
4. Add the narrowest offline regression test that would have caught it.
5. Fix the source without editing retained campaign evidence.
6. Rebuild and record new source and binary identities.
7. Re-run the failed lane, its adjacent boundary, and the offline release gate.
8. Record the defect, cause, fix ref, and before and after evidence in the final
   report.

Do not overwrite failed runs with successful reruns. They are separate evidence
records.

## Final report template

Write the completed report beside the previous campaign report under
`docs/reports`. Use this order:

1. Verdict.
2. Scope and source identities.
3. Fixture and model routes.
4. Workflow exercised.
5. Mandatory gate results.
6. Semantic mapper findings.
7. Arbiter-limit comparison.
8. Issue reconciliation findings.
9. Direct-input and cache measurements.
10. Sequential repair proof.
11. Parallel repair proof.
12. Failure and recovery results.
13. Usage, cost, timing, and cleanup tables.
14. Bugs found and fixed.
15. What is proven.
16. What remains unproven.
17. Record-retention note.

The verdict should be `pass`, `pass_with_findings`, `incomplete`, or `failed`.
`pass_with_findings` requires every mandatory safety and correctness gate to
pass. Missing evidence for a mandatory gate is `incomplete`, not a pass.

## Completion checklist

- [ ] Source manifest and binary digest recorded.
- [ ] Disposable workspace and target identities recorded.
- [ ] Offline tests, race tests, vet, and diff checks recorded.
- [ ] No review command or implicit review runtime call occurred.
- [ ] Real semantic mapper run inspected against prior Sprint 38 evidence.
- [ ] Every QA and repair runtime call exported from schema-2 metrics.
- [ ] Token known flags and provider omissions retained without zero-filling.
- [ ] Tool counts and exactness flags recorded for every model call.
- [ ] Web ledger row count, totals, disclosures, and narrow layout match the exported ledger.
- [ ] Every changed path owned exactly once.
- [ ] Direct shard packs and stable prefixes verified.
- [ ] Arbiter limits `1`, `2`, `4`, and `24` exercised.
- [ ] Cross-group issue reconciliation exercised.
- [ ] False and duplicate theories handled correctly.
- [ ] Provider cache behavior measured without claiming native breakpoints.
- [ ] Preserved two-issue grouped sequential campaign completed.
- [ ] Two-queue parallel campaign completed.
- [ ] Stale proposal regeneration proven in the live fixture or controlled offline lane.
- [ ] Provisional checks shown to precede integration.
- [ ] Cancellation, interrupted apply, target drift, and failed final smoke tested.
- [ ] Cleanup complete for every isolated workspace.
- [ ] Failed and successful reruns retained separately.
- [ ] Final report states both proven and unproven boundaries.
