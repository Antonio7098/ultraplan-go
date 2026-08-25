# Source Analysis: langfuse

## Regression Gating and CI Integration (Dimension 18.03)

### Source Info

| Field | Value |
|-------|-------|
| Name | langfuse |
| Path | `studies/agent-harness-study/sources/langfuse` |
| Language / Stack | TypeScript / Node.js (Next.js + Express worker, pnpm monorepo, BullMQ, Prisma/Postgres, ClickHouse, Redis) |
| Analyzed | 2026-08-23 |

## Summary

Langfuse ships a large multi-package monorepo with a single `CI/CD` workflow
(`.github/workflows/pipeline.yml:5`) that fans out into lint/typecheck,
Storybook, ESLint-plugin, sandbox-runtime, server-shared, web (server + client
+ e2e), worker (with optional LLM-connections matrix), docker-build, and
golden-SQL jobs. Every job is required by the aggregator job
`all-ci-passed` (`.github/workflows/pipeline.yml:1170-1212`); its
`success=false` output is the only gate that lets
`build-docker-image-release` run
(`.github/workflows/pipeline.yml:1230-1263`), which in turn feeds the release
tag → image pipeline. So CI failure does block the only release-image path.

What is **not** present is anything that is specifically eval-shaped:

- There is **no CI eval pass-rate baseline**. Eval results in this codebase are
  user-facing LLM-as-judge or user-written code-based scores persisted as
  `Score` rows in ClickHouse
  (`worker/src/features/evaluation/evalCompletion.ts:21-94`,
  `packages/shared/src/server/evals/codeEvalExecution.ts:206-313`). Nothing
  in the repo compares aggregate pass-rates between runs or against a saved
  baseline.
- There is **no drift dashboard** for eval scores. The only "drift" vocabulary
  found is in the **CI-runtime** analyst agent
  (`.github/workflows/ci-runtime-analyst.md:351-369`), which tracks CI step
  drift, flaky-test counts, and timing regressions — not eval quality
  metrics. The custom Vitest reporter at
  `scripts/vitest/ci-reporter.ts:42-170` prints slowest/retried/flaky tests
  but does not aggregate pass-rates.
- The only **baseline-comparison logic** is a user-facing experiment UI:
  `web/src/features/experiments/components/ExperimentBaselineControls.tsx:7-56`
  + `packages/shared/src/server/repositories/experiments.ts:849-920` (the
  `requireBaselinePresence` and `isBaselineEnforced` paths) lets a user mark
  one dataset run as the "baseline" against which to display other runs in
  the experiments table. It is a presentational feature, not a CI gate.
- The closest thing to **regression baseline tests** is the
  golden-SQL harness at
  `packages/shared/src/server/query-ast/goldenHarness.ts:1-233` +
  `environments.golden.test.ts:1-76` + the snapshot file
  `packages/shared/src/server/query-ast/__snapshots__/environments.golden.test.ts.snap:1-50`.
  It captures ClickHouse SQL emitted by repositories at the exec seam,
  normalizes it via `clickhouse format`, and compares against committed
  snapshots. This catches silent SQL-shape regressions during the
  query-builder AST refactor. It is a *code-level* regression gate, not an
  eval-quality gate.

So evals themselves are exercised in CI (the `tests-worker` job runs them
against a Floci Lambda mock, see below) and any CI failure blocks releases,
but there is no concept of an "eval pass rate" being measured, compared to
a baseline, or tracked across runs.

## Rating

**4 / 10 — Present but inconsistent, weakly documented, or fragile.**

Rationale:

- CI infrastructure that gates release images is real and operational
  (`pipeline.yml:1170-1212`, `pipeline.yml:1230-1263`), so a green-build gate
  for the software exists.
- Code-based evals run in CI against an in-cluster Lambda mock
  (`pipeline.yml:827-862` + `docker-compose.dev.yml:53-72` +
  `scripts/code-eval-runners/bootstrap-floci.py:19-89`).
- But the dimension asks specifically about *eval* gating, baseline
  comparison, and pass-rate drift. None of those exist as first-class
  concepts. The only baseline logic is a user-facing experiment diff view,
  and the only drift tracking is CI *runtime* drift. The golden-SQL snapshot
  is a one-off safety net for a specific refactor, not an eval-quality gate.
- No eval regression report is generated, no eval dashboard tracks trends,
  and the entire repo surfaces no notion of "an eval got worse between two
  commits" outside the manual UX comparison.

## Evidence Collected

Every entry includes a file path with line numbers. Format `path:line`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| CI workflow fan-out (test jobs required by aggregator) | `all-ci-passed` job `needs:` every test job | `.github/workflows/pipeline.yml:1170-1189` |
| CI aggregator failure handling (Slack notify on push failures only) | `Notify Slack` step conditioned on `failure() && push` | `.github/workflows/pipeline.yml:1217-1228` |
| Release gate: `build-docker-image-release` only runs after `all-ci-passed.success==true` | `needs: all-ci-passed` + `if` clause | `.github/workflows/pipeline.yml:1230-1233` |
| Code-eval dispatcher configured in worker CI tests (Floci mock at :4566) | `LANGFUSE_CODE_EVAL_AWS_LAMBDA_ENDPOINT: http://localhost:4566` | `.github/workflows/pipeline.yml:862` |
| Code-eval runners sourced into the dev CI stack (Floci Lambda mock) | `worker-tests` profile wires Floci | `docker-compose.dev.yml:53-72` |
| Code-eval Python handler (CI Lambda payload) | `handler(event, context)` returns scores or error envelope | `scripts/code-eval-runners/python/code_based_eval_handler.py:100-128` |
| Code-eval Node/TS handler (CI Lambda payload) | `handler(event)` strips TS, runs `evaluate(ctx)` | `scripts/code-eval-runners/node/code-based-eval-handler.mjs:9-46` |
| Code-eval runner bootstrap script (uploads ZIPs to Floci Lambda) | `main()` upserts `code-based-eval-executor-{node,python}` lambdas | `scripts/code-eval-runners/bootstrap-floci.py:19-89` |
| Worker exec path that consumes the eval dispatcher | `executeCodeBasedEvaluation` -> `runCodeBasedEvaluationDispatch` | `worker/src/features/evaluation/codeBased/executeCodeBasedEvaluation.ts:30-95` |
| Eval result persistence (no regression compare) | `completeEvalExecution` uploads scores via IngestionQueue, marks JobExecution COMPLETED | `worker/src/features/evaluation/evalCompletion.ts:21-94` |
| Eval dispatch wrapper (no aggregate eval) | `runCodeBasedEvaluationDispatch` returns scores or `success:false` error | `packages/shared/src/server/evals/codeEvalExecution.ts:206-313` |
| Dispatcher selection in prod vs dev/test | `resolveConfiguredCodeEvalDispatcher` defaults to `insecure-local` in dev/test, else requires explicit config | `packages/shared/src/server/evals/codeEvalDispatchers.ts:13-43` |
| Local TS code-eval dispatcher | `LocalCodeEvalDispatcher.dispatch` runs TS evaluator in `node:vm` with timeout | `packages/shared/src/server/evals/localCodeEvalDispatcher.ts:25-109` |
| Test env flag used by CI to skip live LLM call validation | `LANGFUSE_SKIP_EVALUATOR_MODEL_CALL_VALIDATION=true` injected by pipeline | `.github/workflows/pipeline.yml:627` and `.github/workflows/warm-caches.yml:90` |
| Code-eval preflight validates schema/model but does not compare scores | `getEvaluatorDefinitionPreflightError` runs a model test only | `web/src/features/evals/server/evaluator-preflight.ts:73-114` |
| CI reporter prints slow/retried/flaky tests, not eval pass-rate | `VitestCiReporter.onTestRunEnd` summarizes slowestTests + retriedTests | `scripts/vitest/ci-reporter.ts:42-170` |
| CI-runtime drift agent (not eval drift) — calls out regressions >~10% on medians, persistent flaky tests | Weekly analyst scans vitest JSONL | `.github/workflows/ci-runtime-analyst.md:254-378` |
| Tree-skip duplicate CI on pushes to main | `pre-job` matches `head_commit.tree_id` against prior successful runs | `.github/workflows/pipeline.yml:42-77` |
| User-facing experiment baseline selector | `ExperimentBaselineControls` lets a user pick one run as baseline | `web/src/features/experiments/components/ExperimentBaselineControls.tsx:7-56` |
| Experiment baseline SQL path (presence enforcement only) | `isBaselineEnforced` requires baseline row in `having` | `packages/shared/src/server/repositories/experiments.ts:849-920` |
| Golden-SQL regression harness (ClickHouse query shape, not eval) | `goldenHarness.ts` records exec seam; test `toMatchSnapshot` | `packages/shared/src/server/query-ast/goldenHarness.ts:1-233` |
| Golden-SQL harness wired into CI test suite (vitest snapshot) | `environments.golden.test.ts` skips loudly if `clickhouse format` absent | `packages/shared/src/server/query-ast/environments.golden.test.ts:29-76` |
| Golden snapshot sample (committed baseline) | Snapshot file holds canonical SQL for `events_only` and `legacy` modes | `packages/shared/src/server/query-ast/__snapshots__/environments.golden.test.ts.snap:1-50` |
| Deploy workflow does NOT reference CI results (manual workflow_dispatch, separate from CI/CD) | `deploy.yml` has no `needs:` from `pipeline.yml` | `.github/workflows/deploy.yml:1-136` |

## Answers to Dimension Questions

1. **Do evals run in CI?**
   Partially. Code-based evals run in CI: the `tests-worker` job spins up
   Floci (a local Lambda mock), points
   `LANGFUSE_CODE_EVAL_AWS_LAMBDA_ENDPOINT` at it
   (`.github/workflows/pipeline.yml:862`), and the test suite executes the
   dispatcher end-to-end (`scripts/code-eval-runners/bootstrap-floci.py:19-89`,
   `worker/src/features/evaluation/codeBased/awsLambdaCodeEvalDispatcher.integration.test.ts`).
   LLM-as-judge evals, however, are explicitly excluded from default worker
   tests via `pnpm --filter=worker run test:exclude-llm-connections`
   (`.github/workflows/pipeline.yml:860`) and only run in a separate
   `test-worker-llm-connections` job gated by a paths-filter
   (`.github/workflows/pipeline.yml:863-869`,
   `pipeline.yml:78-102`). So LLM eval coverage in CI is selective.
2. **Do regressions block deployments?**
   Yes for release-image builds: any failure in the CI fan-out sets
   `all-ci-passed.outputs.success=false`
   (`.github/workflows/pipeline.yml:1197-1212`), and the release-image job
   has `if: always() && needs.all-ci-passed.outputs.success == 'true'`
   (`.github/workflows/pipeline.yml:1233`). But the `deploy.yml` workflow
   (ECS service deploys) is a separate workflow triggered on push to
   `main`/`production` or `workflow_dispatch`
   (`.github/workflows/deploy.yml:1-29`) and does **not** depend on the CI
   workflow results, so a green build is not strictly required to ship.
3. **Are results compared to baselines?**
   No eval baseline comparison. The only baseline concept in the codebase
   is the user-facing experiment run comparison: a user picks one
   `dataset_run_id` as baseline and the experiments table computes a diff
   (e.g. `requireBaselinePresence` /
   `countIf(e.experiment_id = {baseExperimentId}) > 0`,
   `packages/shared/src/server/repositories/experiments.ts:849-920`,
   `web/src/features/experiments/components/ExperimentBaselineControls.tsx:7-56`).
   This is not CI gating and not aggregate pass-rate. The golden-SQL
   snapshots (`packages/shared/src/server/query-ast/__snapshots__/environments.golden.test.ts.snap`)
   are baseline-comparison tests for SQL shape, not eval scores.
4. **Is pass-rate drift tracked?**
   No. Search for `drift`/`pass-rate`/`regress` across
   `packages/shared/src/server/evals`,
   `worker/src/features/evaluation`, `web/src/features/evals`,
   `web/src/features/experiments`, and all `.github/workflows` files finds
   only:
   - CI *runtime* drift tracking in the weekly analyst agent
     (`.github/workflows/ci-runtime-analyst.md:351-369`).
   - Slow/flaky test counts in `scripts/vitest/ci-reporter.ts:151-167`.
   - User-facing experiment diff vs. a chosen baseline run.
   - Generic comments like "cannot drift" that document design intent
     (e.g. `packages/shared/src/server/queries/clickhouse-sql/query-fragments.ts:136`).
   No score-aggregate time series, no eval dashboard, no eval report file.

## Architectural Decisions

- **Tests gate release images, not all deploys.** `pipeline.yml:1230-1233`
  enforces that the docker-release pipeline only runs on a successful CI run;
  however, `.github/workflows/deploy.yml:1-29` is a separate workflow with
  no `needs:` link to `CI/CD`, so it can run on a fresh push to
  `main`/`production` without waiting for CI. This makes the gating model
  "release images are safe; production ECS deploys may lead or follow."
- **Tree-skip pre-job** (`pipeline.yml:42-77`) deduplicates CI runs when the
  same git tree was already validated; useful for push events on `main` but
  intentionally a no-op for `pull_request` and `merge_group` so reviewers
  always see fresh runs.
- **Dispatchers, not monolithic eval execution.** Eval execution is split
  behind a `CodeEvalDispatcher` interface with two production impls:
  `LocalCodeEvalDispatcher` (vm-isolated TS only, marked
  `insecure-local` for dev/test only,
  `packages/shared/src/server/evals/localCodeEvalDispatcher.ts:16-110`) and
  `AwsLambdaCodeEvalDispatcher`
  (`packages/shared/src/server/evals/codeEvalDispatchers.ts:32-40`,
  `packages/shared/src/server/evals/awsLambdaCodeEvalDispatcher.ts`). The
  same worker code path
  (`worker/src/features/evaluation/codeBased/executeCodeBasedEvaluation.ts:30-95`)
  drives both, which is why CI can swap the endpoint to Floci without
  touching the calling code.
- **Eval results are first-class Score rows.** Eval outcomes flow through
  `completeEvalExecution` (`worker/src/features/evaluation/evalCompletion.ts:21-94`)
  into the same ingestion pipeline as user-provided scores. That makes
  scores queryable for ad-hoc dashboards but does not by itself create any
  pass-rate aggregation.

## Notable Patterns

- **Dispatcher abstraction with two backends.** Single interface, two
  concrete dispatchers, plus a config-driven resolver
  (`packages/shared/src/server/evals/codeEvalDispatchers.ts:13-43`).
- **Code-eval runs as packaged Lambdas.** CI/local both rely on
  `scripts/code-eval-runners/{python,node}/*.py|mjs` handlers and
  `bootstrap-floci.py` to register them with a local Lambda mock
  (`docker-compose.dev.yml:53-72`).
- **Golden-SQL snapshot pattern.** Capture-on-mock, `clickhouse format`
  normalize, param-token rewrite, snapshot diff
  (`packages/shared/src/server/query-ast/goldenHarness.ts:160-233`).
- **CI health telemetry in log output.** Custom Vitest reporter prints
  slowest / retried / flaky tests to stdout
  (`scripts/vitest/ci-reporter.ts:42-170`), then an external analyst agent
  parses the JSONL for weekly trend reports
  (`.github/workflows/ci-runtime-analyst.md:254-369`).

## Tradeoffs

- **Release images are gated; deploys are not.** Strict gating for tagged
  releases (`.github/workflows/pipeline.yml:1233`) gives auditable image
  provenance, but the ECS deploy workflow runs without CI evidence, so a
  failing test on `main` can still be deployed via `deploy.yml` until the
  release tag is cut.
- **Worker tests exclude LLM-connections by default.** Reduces cost and
  flakiness (`.github/workflows/pipeline.yml:860`) but means
  `test-worker-llm-connections` only runs when its paths-filter flags a
  change or a tag is pushed (`.github/workflows/pipeline.yml:863-869`), so
  the full LLM-eval path is rarely exercised on a regular PR.
- **Golden-SQL is a refactor safety net, not a contract test.** It catches
  the query-ast refactor breaking SQL shape, but it is limited to one
  repository family so far and is gated by `clickhouse format` availability
  (`packages/shared/src/server/query-ast/environments.golden.test.ts:29-37`),
  which is fine for CI but is a soft contract.
- **No structured eval report.** Eval outcomes go through the standard
  score-write path (`worker/src/features/evaluation/evalCompletion.ts:54-77`),
  which is correct for users querying scores, but means no off-the-shelf
  pass-rate artifact is emitted from CI to compare run-over-run.

## Failure Modes / Edge Cases

- **CI green != deploy safe.** `deploy.yml` runs independently of the
  `CI/CD` workflow; a regression merged to `main` could deploy before CI
  fails the release tag, depending on branch ordering.
- **LLM-eval drift hides between releases.** Because LLM-connection tests
  only run on tag pushes or path-filter changes
  (`.github/workflows/pipeline.yml:863-869`), an LLM-eval regression that
  only manifests for a new prompt template or model may pass on the PR and
  surface only later.
- **Code-eval runtime mismatch.** Worker tests use the Floci in-cluster
  Lambda (`docker-compose.dev.yml:53-72`,
  `.github/workflows/pipeline.yml:862`), but production uses real
  AWS Lambda. Behavior differences (concurrency, payload limits, runtime
  versions, network egress) are not regression-tested.
- **`clickhouse format` absent on a runner.** Golden-SQL tests skip with a
  warning (`packages/shared/src/server/query-ast/environments.golden.test.ts:33-37`),
  which silently disables the only baseline-snapshot gate.
- **Flaky eval tests can mask regression.** Vitest retries plus the CI
  reporter's `flaky: true` flag (`scripts/vitest/ci-reporter.ts:68-70`) mean
  a flaky test that eventually passes is not surfaced as a failure; there
  is no policy to fail the build on N consecutive flaky runs in a week
  (only the weekly analyst notes them,
  `.github/workflows/ci-runtime-analyst.md:304-306`).

## Future Considerations

- **Promote eval-pass-rate to a CI artifact.** Persist eval-job outcomes
  per `JobExecution` and emit an aggregate in
  `completeEvalExecution` (`worker/src/features/evaluation/evalCompletion.ts:21-94`)
  or in `runCodeBasedEvaluationDispatch`
  (`packages/shared/src/server/evals/codeEvalExecution.ts:206-313`) that
  CI could compare against a stored baseline snapshot.
- **Wire `deploy.yml` to `CI/CD`.** Adding `needs: [CI/CD]` from the
  `ecs-deploy` job would close the gap between test gating and ECS
  promotion.
- **Always-on LLM eval job.** A scheduled job that runs a fixture
  eval-dataset against `LANGFUSE_SKIP_EVALUATOR_MODEL_CALL_VALIDATION=false`
  and emits a pass-rate artifact would give a real eval-drift dashboard.
- **Golden harness expansion.** The pattern at
  `packages/shared/src/server/query-ast/goldenHarness.ts:1-233` is
  server-only and test-scoped. Extending it to eval SQL paths or to the
  `Score` write payload would give an eval-payload regression gate.
- **First-class flake budget.** The CI reporter's `[flaky]` output
  (`scripts/vitest/ci-reporter.ts:97-99`) is human-readable only. Promote
  it to a counted metric that fails the build when a test is flaky N
  times in a row.

## Questions / Gaps

- Does the team intentionally exclude `deploy.yml` from CI gating, or is
  this an oversight? The current setup means a green CI badge does not
  imply a safe ECS deploy.
- Is there a separate Langfuse Cloud or SaaS-side CI run that adds eval
  pass-rate checks not present in this OSS repo? No evidence found in
  this source tree.
- Where would an "eval baseline" file live if introduced? The closest
  precedent is `packages/shared/src/server/query-ast/__snapshots__/`,
  suggesting a `__eval-baselines__/` directory under the shared package
  would fit conventions.
- Is the golden-SQL harness intended to become a long-term contract test,
  or is it throwaway scaffolding for the query-ast refactor? No README
  comment in `packages/shared/src/server/query-ast/README.md:1-27`
  clarifies durability intent.
- No evidence found for: a dedicated eval CI report file, eval pass-rate
  thresholds, eval score time-series aggregation in CI, or eval-specific
  Slack/alerting on drift.

---

Generated by `18.03-regression-gating-and-ci-integration` against `langfuse`.