# QA context-engineering dogfood campaign

Campaign dates: 2026-08-31 to 2026-09-01

## 1. Verdict

`pass_with_findings`

The campaign is complete. The real QA path passed semantic mapping, investigation, arbitration, reconciliation, isolated evidence, adjudication, cleanup, the four-value arbiter matrix, same-path cache measurement, and live browser-ledger inspection. An authorized disposable fixture then supplied two independent deterministic failures and exactly two repair-eligible issues. Both the sequential and parallel live campaigns completed 2 of 2 repairs with exit code 0 and full verification ladders.

The findings are the fixed product defects listed in Section 14 and retained failed attempts used to harden fixture admission. None leaves a mandatory lane incomplete.

## 2. Scope and source identities

| Item | Identity |
|---|---|
| UltraPlan base commit | `c48babecfb892f86502abac07a3e5ced4549370b` |
| Final source diff SHA-256 | `b0be5aec79ef027f1327d68c3722eda1d85a14279263ef696f2470f2a8d8b7b7` |
| Agentwrap commit | `511d34d252db5f5dcb40dad6a4a5502f9836f8a1` |
| Final binary SHA-256 | `96b7d7e508dd9451c67f0ce8571705442425c0938c2b1704038d54587111db54` |
| Target branch | `ultraplan/ultraplan-go/38-bounded-repair` |
| Target commit | `accda5dff06d9b5debac9f0e3782a7dab6ea32f6` |

The source worktree remains intentionally dirty with the campaign fixes and this report. The protected original target remained Git-clean. All injected defects and repair mutations were confined to `repair-fixture-target`.

## 3. Fixture and model routes

The evidence root is `/home/antonioborgerees/coding/.ultraplan-qa-context-dogfood-20260831/evidence`. Required evidence is not stored in `/tmp`.

All QA roles used `openrouter/minimax/minimax-m3:free`, variant `high`, with `fallback: false`. OpenCode 1.18.25 supplied provider-managed caching. UltraPlan did not claim a caller routing key or native intra-message byte breakpoint.

The review and smoke files used for fixture admission are controlled prerequisites, not campaign review evidence. A live review attempt and a live smoke attempt were retained, including their failures. The fixture uses the previously validated passing prerequisites with current target fingerprints.

## 4. Workflow exercised

The real runs covered schema-1 upgrades, map preview, semantic mapping, parallel investigators, context continuations, limits 1, 2, 4, and 24, arbiter output repair, strict reference closure, cross-group reconciliation, isolated deterministic checks, cleanup, adjudication, same-path cache comparison, and live schema-2 metrics rendering.

Controlled tests covered repair assignment, provisional checks, serial apply, stale proposals, cancellation, journals, target drift, cleanup, smoke gates, recovery, replay, and reconnect behavior. Live repair covered two sequential issue runs and two parallel proposal runs with serialized production apply.

## 5. Mandatory gate results

| Gate | Result | Evidence |
|---|---|---|
| UltraPlan tests | pass | `lane-00-offline/ultraplan-test-campaign-final.log` |
| Race tests | pass | `lane-00-offline/ultraplan-race-campaign-final.log` |
| Vet and diff check | pass | final Lane 00 logs |
| Agentwrap tests, vet, diff | pass | final Lane 00 logs |
| Semantic mapper and investigators | pass | Lane 01 and matrix metrics |
| Arbiter limits | pass | `lane-02-arbiter-matrix/matrix-summary.json` |
| Reconciliation | pass | real limit-1 and limit-4 collapses plus negatives |
| Evidence authority | pass | no candidate promoted after passing evidence |
| Direct inputs and cache | pass | Lane 04 summaries |
| Web ledger | pass | live HTML and screenshots under `web-ledger/` |
| Controlled repair and recovery | pass | Lane 05, 06, and 07 logs |
| Live sequential repair | pass, 2/2 | `lane-08-live-repair-fixture/sequential-summary.json` |
| Live parallel repair | pass, 2/2 | `lane-09-live-parallel-repair/parallel-summary.json` |

## 6. Semantic mapper findings

Lane 01 completed with attempt `qa-v1-attempt-f72ed77d08b54947f004b3bc` and map `qa-v1-map-3fd27eb5af3a296cfbc0bf7d`. Five shards terminated. The run completed arbitration, reconciliation, ten isolated checks, and adjudication with assessment `pass` and zero repair-eligible issues.

The foundation had 178 blocks and 343,706 bytes. All seven changed paths had one primary owner. Runnable packs recorded nonzero prompt and prefix sizes and complete block sets.

Mapper and investigator output varied between fixed-route runs. The report does not misattribute every matrix difference to the grouping limit.

## 7. Arbiter-limit comparison

| Limit | Theories | Groups | Largest | Provisional | Reconciled | Assessment |
|---:|---:|---:|---:|---:|---:|---|
| 1 | 11 | 11 | 1 | 6 | 4 | pass after bounded rerun |
| 2 | 2 | 1 | 2 | 0 | 0 | pass |
| 4 | 5 | 2 | 4 | 2 | 1 | pass |
| 24 | 12 | 1 | 12 | 5 | 5 | pass |

No group exceeded its limit. All groups recorded the configured strong route and no fallback. The deterministic grouping regression invokes grouping twice on identical theories and asserts identical IDs and membership.

The matrix contains 75 new call rows: 6 mapper, 46 investigator, 19 arbiter, and 4 reconciler. Repairs and continuations remain separate rows.

## 8. Issue reconciliation findings

At limit 1, six provisional issues became four reconciled issues without losing confirmed theory coverage. At limit 4, two became one. No issue became repair-eligible because its deterministic evidence did not fail.

Controlled negatives reject unknown theory IDs, duplicate membership, omitted membership, invented evidence, malformed JSON, unknown fields, and multiple JSON values. A bad reconciler response has no deterministic semantic fallback.

## 9. Direct-input and cache measurements

All five initial limit-24 investigators recorded one identical 12,584-byte prefix with SHA-256 `9df2dac9ff61ecfc8604d5cd73c4e37b0bbcd0024dbabbe20a2f506270c1a358`, exact zero tool calls, read-only sandboxing, and deny-by-default permissions.

The cache experiment recreated the same absolute `cache-workspace` path from the frozen seed. The first workspace remains archived beside it.

| Mapper field | Trial 1 | Trial 2 |
|---|---:|---:|
| Prompt bytes | 378,321 | 378,321 |
| Prefix bytes | 342,675 | 342,675 |
| Prefix digest | identical | identical |
| Input tokens | 108,195 | 106,382 |
| Output tokens | 8,526 | 5,292 |
| Cache-read tokens | 128 | 1,941 |
| Cache-write tokens | 0, known | 0, known |

Both trials later encountered semantic validation failures. That does not invalidate prompt and cache construction. Cache reads are provider observations, not a guaranteed product contract.

## 10. Sequential repair proof

Fixture attempt `qa-v1-attempt-31b5d03cd183b5c873b57549` admitted `Retry limit is disabled` and `Lease duration is disabled` from separate repeatable failing checks. Campaign `repair-campaign-v1-78bceb1e83405050885f4900` used `per_issue` assignment and `sequential` execution. It completed 2 of 2. Repair runs `repair-v1-run-f430946bcdfceae7501ec599` and `repair-v1-run-642c0e1f6c144da5213e53d3` both reached verified outcomes. The post-run fixture checks passed.

## 11. Parallel repair proof

Fixture attempt `qa-v1-attempt-e260681e830347ac31277956` admitted the same two defects under grouped, two-per-agent, parallel policy. Campaign `repair-campaign-v1-edd420d70474ac34e10c0026` completed 2 of 2 with exit code 0. Repair runs `repair-v1-run-2b406413663ed7cbb7aa215a` and `repair-v1-run-7f65c041a2da0d08f6104e01` both verified. Proposals ran through the parallel campaign path, while production apply and verification remained ordered. The final fixture checks passed.

## 12. Failure and recovery results

| Case | Result |
|---|---|
| Mapper malformed twice | real fail-closed evidence retained |
| Arbiter malformed or fenced | real fail-closed evidence retained |
| Arbiter unknown references | reproduced and fixed with `allowed_refs` |
| Reconciler invalid membership or references | controlled fail-closed pass |
| Provisional check failure | controlled pass, target unchanged |
| QA cancellation and interrupted phases | controlled durable-state pass |
| Stale parallel proposal | controlled rejection pass |
| Journaled apply and exact postimage | controlled recovery pass |
| Unknown drift | controlled escalation pass |
| Intermediate and final smoke rules | controlled pass |
| Web replay and reconnect | controlled pass |

Live cancellation was not injected into the successful campaigns. Controlled cancellation, recovery, and stale-proposal tests remain the authority for those failure paths.

## 13. Usage, cost, timing, and cleanup

Lane 01 retained 28 QA calls. The limit matrix retained 75 new QA calls across its clones. Successful provider calls recorded known input, output, cache-read, cache-write, and total-token fields. Required no-tool roles recorded exact zero tools. OpenRouter did not report cost, so no amount is invented.

The per-call QA matrix is in [qa-context-engineering-dogfood-runtime-matrix.md](qa-context-engineering-dogfood-runtime-matrix.md). It contains the 38 QA and repair calls retained in the final fixture ledger, with agent role, operation, task, status, input tokens, output tokens, cache reads, cache writes, total tokens, tool calls, and tool-count exactness. Unknown provider values remain `unknown`; known zero values remain `0`. Review, planning, smoke, and merge agents are excluded because this result set measures QA.

| QA result stage | Retained calls |
|---|---:|
| QA | 30 |
| Repair | 8 |
| Total | 38 |

The web ledger displayed 285 retained rows in the limit-24 clone. Desktop and narrow screenshots show 100 complete-token rows, 1,110 explicitly unknown values, 41 exact-tool rows, zero tool calls, and 471 runtime events. Known zero and unknown render separately.

Every run that reached evidence execution recorded complete isolated-workspace cleanup. Successful repair runs accepted and released their leases, applied only the authorized fixture file, and completed their ladders. All retained evidence lives under the persistent campaign root.

## 14. Bugs found and fixed

### Complete context incorrectly disabled discovery tools

QA prompts and permission policies treated a complete direct-input packet as a reason to prohibit tool use. That reversed the intended optimization: supplied context should make discovery unnecessary, not make it impossible. The semantic mapper, investigator, challenger, arbiter, reconciler, output-repair continuations, and failed-evidence evaluator now retain bounded read, list, search, and glob access under a read-only sandbox and default-deny mutation policy. Prompts say the packet should normally be sufficient while leaving read-only verification available. Focused contract tests cover both the available discovery tools and the denied write, edit, patch, bash, and shell tools. The retained campaign's zero-tool rows remain historical evidence from the pre-fix runtime and are not rewritten.

### Legacy maps rejected after adding `arbiter_max_theories`

An absent legacy field decoded as zero. The loader now supplies historical default 24 only for legacy schema-1 maps. `TestQAStoreUpgradesMapFromBeforeArbiterTheoryBudget` covers it.

### Conflict detected after a paid mapper call

New-start conflict checks ran after mapping, and resume remapped. `qaMapForRun` now checks retained state first. `TestQAMapForRunRejectsConflictAndReusesRetainedMapOnResume` covers both paths.

### Arbiter wire values and repair diagnostics were incomplete

The prompt omitted accepted strings, leading MiniMax to return unsupported `retain`. Repair also discarded its second failure. The shared contract now lists exact enums and coverage rules, while errors retain both attempts. A focused contract test prevents drift.

### Arbiter reference closure was implicit

Nested theory text exposed block-like IDs that were not delivered as context. Packs now carry canonical `allowed_refs`, and every output reference must come from it.

### Arbiter repair could regress valid coverage

One repair fixed a missing issue but dropped another. Repair turns now restate the full contract and require preservation of other valid coverage.

### Arbiter framing was underspecified

MiniMax fenced both attempts at limit 1. The contract now requires `{` as the first byte, `}` as the last, and forbids Markdown fences, backticks, prose, or commentary. Strict decoding remains unchanged.

### Evidence isolation rejected contained symlinks

Target identity accepted contained relative links while isolation rejected all links. Isolation now validates containment, copies link text without following it, and hashes type and target. Escapes, hard links, and special files still fail closed. A focused copy and identity test covers this.

### Ephemeral serving contradicted the documented command

`serve --listen 127.0.0.1:0` failed before binding. Port zero is now allowed only for numeric loopback addresses, and the resolved nonzero listener is revalidated. CLI and server tests cover IPv4 and IPv6.

### Repair help omitted a required admission term

The Sprint 38 live smoke contract requires repair help to name the conformance review gate. The help described the mechanics but omitted that term. It now states that repair requires a current acceptable conformance review and passing containing smoke. `TestSprintHelpIsRegistered` pins the wording.

### Repair budget sources exposed an internal label

Environment overrides were projected as `env`, while the public smoke contract requires `environment`. `repairBudgetsFor` now normalizes the public source label without changing config-layer precedence. `TestRepairBudgetsForUsesTypedConfigAndReportsSources` covers the lower environment override.

## 15. What is proven

- Real QA completes through evidence-backed adjudication.
- Every tested group respects its configured limit.
- Reconciliation collapses duplicates without granting repair authority.
- Direct packs have exact shared prefixes and no discovery-tool dependency.
- Per-call telemetry distinguishes continuations, known zero, unknown values, routes, permissions, tokens, and events.
- Same-path requests preserve exact prompt and prefix construction.
- The browser ledger matches its clone ledger and is readable at 390 pixels.
- Controlled repair and recovery boundaries fail closed.
- Contained symlinks work without permitting escapes.
- A live two-issue sequential campaign completes both repairs and every verification ladder.
- A live grouped parallel campaign completes both isolated proposals with ordered production application.
- Both repaired fixture checks pass after each successful campaign.

## 16. What remains unproven

- Live stale-proposal regeneration after an overlapping mutation.
- Live cancellation during proposal or verification.
- Live final-smoke failure after an earlier pending success.

These are failure-injection extensions, not missing campaign goals. Controlled deterministic tests cover all three. The required real sequential and parallel success lanes are complete.

## 17. Efficiency compared with prior dogfood

The directly comparable baseline is the two-issue grouped repair proof in [qa-dogfood-campaign.md](qa-dogfood-campaign.md). That campaign and the two current lanes used the same MiniMax M3 free route and completed two repairs. Older planning and code-context campaigns did not retain the same complete per-call fields, so they are not used for numeric repair comparisons.

| Metric | Prior grouped campaign | Current sequential | Change vs prior | Current grouped parallel | Change vs prior |
|---|---:|---:|---:|---:|---:|
| Issues completed | 2/2 | 2/2 | same | 2/2 | same |
| Proposal calls | 2 | 2 | same | 2 | same |
| Fresh input tokens | 168 | 225 | +33.9% | 202 | +20.2% |
| Output tokens | 67 | 47 | -29.9% | 59 | -11.9% |
| Cache-read tokens | 52,837 | 14,486 | -72.6% | 17,880 | -66.2% |
| Cache-write tokens | not stated | 0 | not comparable | 0 | not comparable |
| Total reported tokens | 53,072 | 14,758 | -72.2% | 18,141 | -65.8% |
| Proposal runtime sum | 51.714 s | 15.203 s | -70.6% | 80.698 s | +56.0% |
| Campaign wall time | about 110 s | 68.620 s | about -37.6% | 142.278 s | about +29.3% |
| Tool calls | unavailable | 4 exact | not comparable | 4 exact | not comparable |
| Model sessions | 1 shared | 2 separate | different topology | 1 shared | same topology |

The current sequential lane is the most efficient observed two-issue repair run by wall time and reported tokens. Its lower cache count is not automatically a cache improvement. It submitted 33.9% more fresh input but reused far less cached context, while still finishing faster and emitting fewer output tokens.

The grouped parallel lane did not produce a latency win. Grouped assignment placed both issues in one worker queue to preserve session continuation. Parallel execution can overlap workers, not items inside one worker, so this fixture created no cross-worker overlap. The lane proves the parallel campaign path and ordered apply behavior, but it is not evidence of parallel speedup. A two-assignment fixture is required for that measurement.

The prior report did not retain tool-call totals, and its referenced OpenCode session is no longer present in the local runtime database. The current count comes from explicit per-call counters: both successful lanes used four tools, one read and one edit for each of two proposals. A numeric tool-call delta would be fabricated, so the table marks it unavailable.

The broader context-engineering campaign has no honest one-number efficiency comparison with the prior repair-only campaign. The QA scope includes semantic mapping, cache trials, limit variations, recovery attempts, and two live repair lanes. The per-call QA matrix is the audit record for that scope.

### Paid MiniMax M3 cost estimate

OpenRouter listed the routed `minimax/minimax-m3` price on 2026-09-01 as $0.23 per million input tokens, $0.96 per million output tokens, and $0.05 per million cached-read tokens. Common standard providers listed $0.30, $1.20, and $0.06 respectively. Prices vary by provider and routing choice. Sources: [OpenRouter MiniMax M3 pricing](https://openrouter.ai/minimax/minimax-m3/pricing) and [provider table](https://openrouter.ai/minimax/minimax-m3/providers).

The formula is `(input x input rate + output x output rate + cache read x cache-read rate) / 1,000,000`. Cache writes were known zero in the current successful runs.

| Run | Routed estimate | Standard-provider estimate |
|---|---:|---:|
| Prior grouped repair | $0.002745 | $0.003301 |
| Current sequential repair | $0.000821 | $0.000993 |
| Current grouped parallel repair | $0.000997 | $0.001204 |
| Current QA-stage M3 calls | $0.346606 | $0.443566 |
| Current repair-stage M3 calls, including failed attempts | $0.003443 | $0.004162 |

The successful current sequential repair would cost about 0.082 cents on the routed price, versus 0.274 cents for the prior grouped proof. That is about 70.1% cheaper. The grouped parallel repair would cost about 0.100 cents, about 63.7% cheaper than the prior proof.

The QA and repair estimates exclude calls made with Muse Spark, GPT-5.6, and other models because MiniMax M3 pricing does not apply to them. They also exclude non-QA stages from the results.

## 18. Record-retention note

The persistent campaign root contains manifests, command ledgers, rebuilt binaries, failed and successful envelopes, bounded stderr, private diagnostics with mode `0600`, per-clone metrics, call rows, matrix and cache summaries, controlled-test logs, live metrics HTML, desktop and narrow screenshots, both fixture publications, repair packets, campaign summaries, and post-repair checks. Failed admission, review, smoke, and campaign attempts were not overwritten. Archived workspaces and targets remain under the same persistent root. Nothing required for the verdict is stored under `/tmp`.
