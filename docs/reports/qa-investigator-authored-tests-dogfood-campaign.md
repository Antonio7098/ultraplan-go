# Investigator-authored QA tests dogfood campaign

## Verdict

Complete. All campaign acceptance goals were achieved.

The live QA flow retained original investigator and arbiter session identity, authored persistent executable evidence, reproduced the predicted product failure, promoted it to an issue, carried the regression into the production repository, repaired the implementation, and completed a bounded repair campaign with every gate passing. This campaign verdict is separate from historical sprint assessments shown by the workspace.

## Proved flow

| Goal | Result | Persistent evidence |
|---|---|---|
| Original investigator authors evidence | Passed | `evidence/lane-01-live/live-verification-before-repair-fixture` |
| Reproduced failure from a theory | Passed: `predicted_failure_reproduced` | `evidence/lane-01-live/evidence-rerun-classifier-fixed-qa-v2-test-5eacc9020568d035ab33d2de.json` |
| Evidence promoted to an issue | Passed: `qa-v2-issue-6461fb6e2f79834be178c8a5` | `evidence/lane-01-live/qa-resume-promote-retained-failure-v3.json` |
| Regression enters production | Passed | `internal/sprint/qa_repair_test.go`, `TestApplyRepairFilesRejectsInvalidUTF8WithoutMutation` |
| Product repair enters production | Passed | `internal/sprint/qa_repair.go`, invalid UTF-8 rejection before mutation |
| Exact fail-before and pass-after evidence | Passed | authored-test run records plus `evidence/lane-01-live/promoted-regression-test-after-fix.log` |
| Issue packets and journaled application | Passed | `evidence/lane-05-complete-final/verification` |
| Complete containing ladder | Passed: six of six gates | final `reverification.json` in the retained repair run |
| Final campaign outcome | Passed: `verified`, coordinator `completed` | `evidence/lane-05-repair-campaign-complete-v2.json` |
| Persistent evidence | Passed | campaign root and SHA-256 manifests listed below |

The campaign root is `/home/antonioborgerees/coding/.ultraplan-qa-authored-tests-dogfood-20260902`. Evidence, runtime metrics, fixture tools, failed envelopes, snapshots, checksums, and the persistent runtime directory all live under that root, outside an operating-system temporary workspace.

## Product and harness fixes found during dogfood

1. Replaced natural-language failure matching with a deterministic authored-test marker and test-name classifier.
2. Kept legacy mismatch records readable after classifier upgrades.
3. Stabilized evidence-plan `FrozenAt` values across resume.
4. Corrected immutable-record error kinds and detailed validation reporting.
5. Allowed recovery to load state with broken optional artifact references and clear them safely.
6. Filtered invented semantic mapper path and expectation suggestions.
7. Validated required investigator draft fields before accepting model output, enabling same-session correction.
8. Reloaded persisted test bundles, authoring attempts, runs, evidence requests, and arbiter rounds on resume.
9. Applied the latest authored-test result back to its theories before adjudication.
10. Made adjudication, issue, assessment, and evidence-request lifecycle projections replaceable at their canonical paths.
11. Limited resumed arbitration to theory groups affected by new authored evidence.
12. Added strict UTF-8 validation before journaled replacement bytes can mutate production.
13. Isolated smoke subprocess temporary directories to prevent parallel IPC collisions.
14. Replaced the smoke protocol's nested tsx launcher with `node --import tsx` to prevent parent-child IPC collision.
15. Corrected the dogfood fixture so healthy defects are not re-promoted.
16. Restored the retained arbiter runtime-store path before exact-session identity validation.
17. Allowed a current blocked QA assessment to admit individually eligible promoted issues while keeping incomplete or stale QA ineligible.
18. Derived production repair authority from the immutable shard when the promoted issue location is an investigator-authored `_test.go` file.
19. Added deterministic retry aliases for terminal failed or cancelled durable operations while preserving deduplication for active and successful operations.
20. Bound reconciled issues to authored bundles through immutable evidence identity and recovered missing packet coverage from persisted evidence.
21. Preserved the investigator command working directory in evidence plans and repair checks, including backward-compatible recovery for existing bundles.
22. Gave repair checks a private Go home, module cache, build cache, and temporary directory outside the checked target.
23. Made repair-check cleanup remove read-only Go module-cache trees and report the real cleanup result on pre-apply terminal runs.

Earlier live iterations also corrected arbiter evidence-request limits, evidence command constraints, detailed terminal errors, contradictory resolved requests, issue theory normalization, repair priority schema, Linux `/dev` isolation, read-only cleanup permissions, mapper contract identity, and unresolved-evidence assessment blocking. `BUG_LOG.md` records the grouped defects; the source diff and every failed run remain in campaign evidence.

## Final repair proof

Final campaign: `repair-campaign-v1-16b39893bef0f9ef6433ea1f`.

- Outcome: `verified`
- Coordinator status: `completed`
- Production applied: true
- Cleanup complete: true
- Complete ladder: true
- Repair run: `repair-v1-run-8cc084b9e6759201695a66f2`
- Repair model session: `ses_f94b8626cffeajm2iyno0B4iUH`
- Repair tool calls: 2
- Containing smoke: 6 passed, 0 failed, 0 errors
- Fixture checks: alpha and beta both pass

The pre-fix `EADDRINUSE`, missing-run, stale-fixture, and failed repair records were retained rather than overwritten. The final snapshot contains 28 files with a SHA-256 manifest at `evidence/lane-05-complete-final/SHA256SUMS`.

## Investigator test promotion proof

Lane 07 proves the production mechanism itself, using live model-backed QA and repair rather than a hand-applied fix.

| Boundary | Retained result |
|---|---|
| QA attempt | `qa-v1-attempt-77bf26b1fcbff4b449d4e784` |
| Original theory | `qa-v1-theory-bcc20cc1ca4c589ca55560c8` |
| Authored bundle | `qa-v2-test-00c7eecd3eb2cebcdbdcd3b0` |
| Reproduction spec | `qa-v2-spec-be0e2d51ef9a59fc19324f0e` |
| Fail-before evidence | `qa-v2-evidence-7eaaf6005366cf35fbb1677f`, exit 1 with the exact predicted-failure marker |
| Promoted issue | `qa-v2-issue-0ac7dcc692bd709ddd383310` |
| Successful repair run | `repair-v1-run-d04767998970569ffa6c0af0` |
| Repair model activity | 397 input, 59 output, 41,314 cache-read, 41,770 total tokens, 11 tool calls |
| Paired production apply | `internal/sprint/qa_investigator_e5a5dc2aa66b_test.go` created and `internal/sprint/qa_repair_state.go` changed in one apply journal |
| Verification | provisional reproducer passed, immutable primary reproducer passed, six of six post-apply gates passed |
| Terminal facts | outcome `verified`, production applied `true`, complete ladder `true`, cleanup complete `true`, unresolved issue count 0 |

The apply journal records both paths as applied and neither as restored. The canonical target then passed `go test . -run '^TestQAInvestigator_e5a5dc2aa66b$' -count=1`. The complete packet, confirmation, split patches, preimages, journal, scope, reverification, cleanup, result, command envelopes, model stores, and focused test output live under `lane-07-e2e-promotion`. Failed attempts remain beside the successful run and show the mechanism defects fixed during dogfood.

## Real branch merge proof

The promoted pair was also tested through Git in a persistent disposable clone. The pre-promotion target became baseline commit `67bce1f109dc8cdedae8a088192b39e65da388de` on `dogfood/qa-promotion-baseline`. A source branch, `dogfood/qa-promotion-source`, committed only the authored test and implementation repair as `634721bd50dac48193db17b1c533065920021662`. Git then merged that source branch with `--no-ff` into `dogfood/qa-promotion-integration`, producing two-parent merge commit `4dd5f41a39892304beb3612a3c1f32eba983d94e`.

The merged branch had exactly the expected added test and modified implementation paths. The focused regression and `go test ./internal/sprint` passed, `git fsck` passed, and the final worktree was clean. The clone had no local `main` branch; `origin/main` remained at `c48babecfb892f86502abac07a3e5ced4549370b`. The repository, refs, commit graph, merge output, tests, and checksum manifest live at `lane-07-e2e-promotion/git-merge-proof`.

The manual merge above was followed by a separate proof using UltraPlan's governed merge command. `ultraplan sprint ultraplan-go 38-bounded-repair merge inspect --json` reported `ready: true` for source branch `dogfood/ultraplan-command-source` and integration branch `dogfood/ultraplan-command-integration`, with exactly the same two promoted paths. `ultraplan ... merge --yes --json` then completed and created merge commit `95dd1568fce71a40d3cd139c7958b99eb818fa4f` with parents `67bce1f109dc8cdedae8a088192b39e65da388de` and `634721bd50dac48193db17b1c533065920021662`.

The command's own retained checks passed: no unmerged paths, the expected `MERGE_HEAD`, `git diff --check`, and `go test ./cmd/... ./internal/...`. `merge status --json` returned `completed`, `git fsck` passed, and the integration worktree remained clean. Its persistent repository, copied workspace, JSON envelopes, merge state, `merge.md`, commit graph, and checks live at `lane-07-e2e-promotion/merge-command-live-proof`.

## Runtime efficiency and cost

The complete per-call matrix is in `docs/reports/qa-context-engineering-dogfood-runtime-matrix.md`. It excludes Conformance Review agents and includes all 233 retained QA and repair calls: input, output, cache read, cache write, total tokens, tool calls, and exactness.

Campaign totals were 7,695,962 uncached input tokens, 620,480 output tokens, 8,433,471 cache-read tokens, 16,749,913 reported total tokens, and 256 tool calls across 301 retained QA and repair calls. The final live QA-and-promotion slice used 68 calls, 3,565,311 total tokens, and 148 tool calls, versus 32 calls, 1,620,695 total tokens, and zero tool calls in the previous post-policy rerun. The extra work covered executable evidence authoring, output correction, evidence continuation, affected-group re-arbitration, failed repair diagnostics, and the successful paired-patch promotion.

OpenRouter listed paid MiniMax M3 at $0.23 per million uncached input tokens, $0.96 per million output tokens, and $0.05 per million cache-read tokens on 2026-09-04. Applied to the full retained campaign, the counterfactual cost is $2.787406: $1.770071 input, $0.595661 output, and $0.421674 cache read. The actual calls used the free model route, so no paid spend is claimed. Source: https://openrouter.ai/minimax/minimax-m3/pricing

## Validation

- `go test ./internal/... ./cmd/...`: passed
- `go vet ./internal/... ./cmd/...`: passed
- `go test -race ./internal/sprint ./internal/app ./internal/platform/config`: passed
- `git diff --check`: passed
- Sprint 38 external containing smoke: six of six passed
- Live investigator promotion: `repair-v1-run-d04767998970569ffa6c0af0` verified, paired apply journal complete, canonical focused regression passed
- Browser dashboard and Sprint 38 detail: rendered successfully at 1280 by 800; Sprint 38 showed `complete`, `pass`, current review `pass`, and smoke `pass`
- Browser recording: `evidence/lane-06-ui/sprint-38-pass-browser-recording.mp4`

`go test ./...` still fails at test discovery for `studies/agent-harness-study/sources/letta/tests/data/data_structures.cpp` because that imported study fixture is not a cgo or SWIG package. All product packages pass; this pre-existing repository-wide fixture limitation is not a campaign acceptance failure.

## Durable evidence index

- Final metrics: `evidence/final-runtime-metrics.json`
- Final repair envelope: `evidence/lane-05-repair-campaign-complete-v2.json`
- Final repair tree and checksums: `evidence/lane-05-complete-final/`
- Live investigator-to-production promotion: `lane-07-e2e-promotion/`
- Real non-main Git merge: `lane-07-e2e-promotion/git-merge-proof/`
- Governed UltraPlan merge command: `lane-07-e2e-promotion/merge-command-live-proof/`
- Authored failure and promotion evidence: `evidence/lane-01-live/`
- Failed repair and recovery snapshots: `evidence/lane-05-before-smoke-isolation-rerun/`, `evidence/lane-05-failed-manual-and-stale-issue-fixture/`, `evidence/lane-05-filtered-fixture-target-mutated/`, and `evidence/lane-05-pre-harness-fix-failure/`
- Offline validation: `evidence/lane-00-offline/`
- Persistent fixture generator: `tools/fixture-source/`
- Matrix generator: `tools/render-runtime-matrix.sh`

No required campaign evidence depends on `/tmp`.
