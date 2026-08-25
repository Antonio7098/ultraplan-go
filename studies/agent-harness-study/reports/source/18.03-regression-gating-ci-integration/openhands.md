# Source Analysis: openhands

## 18.03 — Regression Gating and CI Integration

### Source Info

| Field | Value |
|-------|-------|
| Name | openhands (OpenHands `agent-canvas` frontend) |
| Path | `studies/agent-harness-study/sources/openhands` |
| Language / Stack | TypeScript / React 19 / React-Router 7 / Vite 8 / Vitest 4 / Playwright 1.62 / Stryker 9.6 |
| Analyzed | 2026-08-23 |

> Scope note: this source is the `agent-canvas` UI frontend only (per `AGENTS.md` line 12: *"This repository is the OpenHands frontend"*). There is **no SWE-bench / agent-benchmark harness** in this repo. The closest analogues to "evals" for a UI codebase are the Playwright E2E suites (mock-LLM and live-LLM), the Vitest unit suite, and the Stryker mutation suite. The dimension is interpreted in that frame.

## Summary

CI integration is **mature, multi-tiered, and explicitly gated**. Every PR runs a deterministic unit + lint + build pipeline that the release process explicitly waits on; PR-level Playwright E2E (mock-LLM and Docker) are required checks; live-LLM-backed E2E is opt-in via the `live-e2e` label; release-please refuses to tag a release until `test-and-build (ubuntu)` is green on the release commit.

What is **not present** is any baseline comparison, pass-rate drift tracking, or historical test-result dashboard. Each CI run is an independent snapshot. Reports are rendered per-run into PR comments and uploaded as artifacts, but nothing compares them across runs, no pass-rate is computed against a stored baseline, and no thresholds gate the merge. Stryker mutation scores are explicitly "report-only initially; establish a stable baseline before adding a failing threshold" (`docs/DEVELOPMENT.md:144-146`).

Net assessment: a **strong gating pipeline with weak historical observability**. A code change that breaks a unit test, lint rule, type-check, build, or any mock-LLM E2E is blocked from merge and from tagging a release. A code change that silently degrades a metric (e.g., slower Playwright run, dropped mutation coverage, lower pass-rate of an unstable test) is not blocked because no metric threshold exists.

## Rating

**5 / 10** — Present but inconsistent on the observability side; strong on the blocking side; no baseline/drift layer.

| Sub-criterion | Score | Rationale |
|---|---|---|
| Evals run in CI | 9/10 | Three E2E workflows + unit + lint + build + typecheck on every PR |
| Regressions block deployments | 8/10 | Required checks gate PR merge and release tag; live E2E is opt-in, not required |
| Baseline comparison | 1/10 | No baseline files, no comparison logic, no snapshot-vs-current diff |
| Pass-rate drift tracking | 1/10 | No history, no dashboard, no metric threshold, no trend tracking |

Aggregated to **5/10** because the rubric is "Can a code change that degrades quality be blocked before deployment?" — yes for hard regressions (test failures, lint, build), no for soft metrics (mutation score, latency, pass-rate drift).

## Evidence Collected

Every entry includes a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| PR pipeline runs lint + unit + build (the "test-and-build" gate) | `npm ci`, `npm run lint`, `npm test`, `npm run build`, `npm run build:lib`, `npm pack --dry-run` in a matrix over ubuntu/windows | `.github/workflows/ci.yml:39-81` |
| Test matrix built dynamically and gates `full_checks` on ubuntu | Matrix JSON hard-coded to `{"name":"ubuntu","full_checks":true}` | `.github/workflows/ci.yml:28-37` |
| Concurrency policy keeps the release-required check green | Push runs are NOT cancelled in-progress to keep release checks stable | `.github/workflows/ci.yml:16-22` |
| Release pipeline **waits** for `test-and-build (ubuntu)` before release-please tags | Job `await-required-checks` polls check-runs API for `test-and-build (ubuntu)` and refuses to release if not `success` | `.github/workflows/release.yml:21-76` |
| Release-please is a reusable external workflow with App-token auth | Reuses `OpenHands/release-actions/.github/workflows/release-please.yml@main` | `.github/workflows/release.yml:78-87` |
| Mock-LLM E2E runs on every PR commit, then posts report + fails job on failure | Steps: build matrix → start mock LLM → build frontend → resolve affected dirs → run Playwright → upload artifacts → render report → upsert PR comment → `exit $exit_code` | `.github/workflows/mock-llm-e2e.yml:48-345` |
| Affected-test selector + "always run `regressions`" rule for selective runs | `tests/e2e/mock-llm/test-mapping.json` declares `alwaysRun: ["regressions"]`; resolver fails closed → full suite | `tests/e2e/mock-llm/test-mapping.json:106`; `tests/e2e/mock-llm/scripts/resolve-affected-tests.mjs` |
| Mock-LLM Docker E2E runs the **same** specs against the published Docker image | Two triggers: `workflow_run` after Docker build, plus `pull_request` | `.github/workflows/mock-llm-docker-e2e.yml:17-100` |
| Mock-LLM Docker E2E polls Docker workflow until image is built, fails if Docker fails | Polls `actions/workflows/docker.yml/runs?head_sha=SHA` with 15-min timeout | `.github/workflows/mock-llm-docker-e2e.yml:178-223` |
| Live LLM-backed E2E is label-gated (opt-in, not required) | Trigger requires `live-e2e` label on PR or manual `workflow_dispatch` | `.github/workflows/ci.yml:84-89` |
| Live E2E skips fork PRs before checking out PR code so secrets never leak | `is_fork` detection + downstream step `if: is_fork != 'true'` everywhere | `.github/workflows/ci.yml:147-281` |
| Live E2E renders a Markdown report with screenshot/video URLs, posts/upserts to PR | Renders via `tests/e2e/live/scripts/render-live-e2e-report.mjs` and `upsert-pr-comment.mjs`; attaches to step summary | `.github/workflows/ci.yml:454-487` |
| Mock-LLM E2E posts per-run PR comment with pass/fail counts | `**${passed}/${total} passed**` summary, fails-on-exit step at end | `tests/e2e/mock-llm/scripts/render-mock-llm-report.mjs:129-204`; `.github/workflows/mock-llm-e2e.yml:299-345` |
| Custom reporter writes per-test results + `all-passed` marker for CI coordination | `DoneMarkerReporter` writes `.results.json` after every test, `.tests-done` + `.all-passed` only when complete | `tests/e2e/mock-llm/reporters/done-marker-reporter.ts:14-50` |
| Mock-LLM E2E handles teardown hangs by killing Playwright but using marker as ground truth | CI wrapper uses `.all-passed` to override non-zero exit caused by teardown hang | `.github/workflows/mock-llm-e2e.yml:239-267` |
| Dependabot groups dependency bumps with 7-day cooldown | Weekly schedule, grouped bumps, cooldown, npm + github-actions ecosystems | `.github/dependabot.yml:1-110` |
| PR title lint + type labels via reusable workflow | `pr.yml` calls `OpenHands/release-actions/.github/workflows/pr-title.yml@main` | `.github/workflows/pr.yml:17-21` |
| PR description validation: HUMAN note + linked ready-for-dev issue + screenshot for frontend PRs | Python checker rejects missing template fields; bot PRs exempted | `.github/scripts/check_pr_description.py:37-59`; `.github/workflows/pr-description-check.yml:23-44` |
| Issue readiness check: `ready-for-dev` label based on body sections + screenshot | Python checker with idempotent label management + upserted comment | `.github/scripts/check_issue_readiness.py`; `.github/workflows/issue-readiness-check.yml:36-108` |
| SDK version sync (cron + PR + repository_dispatch) catches upstream version drift | Hourly schedule + PR/path triggers + external `sdk-version-check` event | `.github/workflows/sdk-version-sync.yml:1-83` |
| CI script tests run pytest over `.github/scripts/` on PRs that touch them | Self-tests the Python checkers | `.github/workflows/ci-script-tests.yml:9-37` |
| Bump chart image tag from successful Docker release tag | Filter to release-please semver tags only | `.github/workflows/bump-chart.yml:21-63` |
| Mutation testing (Stryker) configured but **no threshold enforced** | `stryker.config.mjs` configures Vitest runner; DEV doc says "report-only initially; establish a stable baseline before adding a failing threshold" | `stryker.config.mjs:1-22`; `docs/DEVELOPMENT.md:144-146` |
| Coverage reporters enabled, but no coverage threshold configured | `vite.config.ts` defines reporters only; `test:coverage` script runs Vitest with `--coverage` | `vite.config.ts:490-494`; `package.json:89` |
| PR artifacts cleanup removes `.pr/` after approval | Workflow `cleanup-on-approval` triggered by `pull_request_review` state `approved` | `.github/workflows/pr-artifacts.yml:16-213` |
| Release-please bumps versions in `config/defaults.json` + `helm/agent-canvas/Chart.yaml` + README | Release manifest tracks per-package versions | `release-please-config.json:6-25`; `.release-please-manifest.json:1-3` |
| `scripts/stryker-diff.mjs` mutates only files changed since a base ref | Local mutation-on-diff utility; not wired into CI | `scripts/stryker-diff.mjs:1-137`; `package.json:91-92` |
| Custom code-review skill flags "eval / benchmark risk" PRs for human review | Reviewers required to call out memory/condenser/tool-harness changes that could affect eval performance | `.agents/skills/custom-codereview-guide.md:18-35` |
| npm publish workflow runs `npm test` before publishing | Tests are required before npm publish; no explicit coverage gate | `.github/workflows/npm-publish.yml:59-60` |

## Answers to Dimension Questions

### 1. Do evals run in CI?

**Yes — for every PR, multiple tiers.**

- **Unit + lint + build**: `.github/workflows/ci.yml:39-81` runs `npm ci`, `npm run lint`, `npm test`, `npm run build` on a `test-and-build` matrix (ubuntu + windows). This is the always-on required check.
- **Mock-LLM E2E**: `.github/workflows/mock-llm-e2e.yml:48-345` runs the full Playwright suite (selectively, by changed files) on every PR commit and `pull_request [opened, synchronize, reopened]`.
- **Mock-LLM Docker E2E**: `.github/workflows/mock-llm-docker-e2e.yml:82-469` runs the same specs against the Docker image, also on every PR commit.
- **Live-LLM E2E**: `.github/workflows/ci.yml:83-281` runs live E2E only when a PR carries the `live-e2e` label or via manual `workflow_dispatch` — opt-in, not required.
- **Mutation testing**: not wired into CI; only invoked locally via `npm run test:mutation` / `:diff` / `:incremental`.
- **Script tests**: `.github/workflows/ci-script-tests.yml` pytest over `.github/scripts/` when those files change.
- **SDK version sync**: `.github/workflows/sdk-version-sync.yml` cron + path-triggered version-drift check.

### 2. Do regressions block deployments?

**Yes for the always-on checks; partially for opt-in live E2E.**

- `test-and-build (ubuntu)` is the explicit gate the release workflow waits on: `.github/workflows/release.yml:21-76` polls the check-runs API and refuses to release-please if the check is not `success`. This is the load-bearing regression gate for releases.
- Mock-LLM E2E and Mock-LLM Docker E2E both have a final `exit $exit_code` step that fails the job on any test failure (`.github/workflows/mock-llm-e2e.yml:341-345`, analogous step in docker workflow), so they appear as required checks on the PR.
- `live-e2e` does NOT block merges by default — it only runs when the PR carries the `live-e2e` label (`.github/workflows/ci.yml:84-89`).
- Mutation testing has no enforcement (`docs/DEVELOPMENT.md:144-146` — explicitly report-only).
- Coverage has no threshold (`vite.config.ts:490-494` — reporters only).
- Lint and typecheck both run inside the unit step and thus block merges.

### 3. Are results compared to baselines?

**No — no baseline comparison exists.**

- The mock-LLM report renders only the current run's pass/fail counts (`tests/e2e/mock-llm/scripts/render-mock-llm-report.mjs:129-204`).
- The live-E2E report renders only current-run summary (`tests/e2e/live/scripts/render-live-e2e-report.mjs:218-240`).
- No file under `.mock-llm-markers/` is checked in (`.gitignore` lines 23–32 ignore the whole dir).
- No `baseline.json` / `expected.json` / `golden.json` for test results anywhere in the tree.
- The only "drift" comparison in the repo is on **version pins** (`.github/workflows/sdk-version-sync.yml`) — that's a dependency-drift check, not a result-drift check.
- The "always run `regressions`" rule in `test-mapping.json:106` is a fixed inclusion list, not a baseline.

### 4. Is pass-rate drift tracked?

**No — no historical storage, no dashboard, no trend metric.**

- Each CI run uploads per-run artifacts (`test-results-mock-llm/`, `playwright-report-mock-llm/`) but they are not aggregated.
- No GH Pages / dashboards-as-code exists for tracking pass-rates.
- No scheduled job stores results over time.
- No PR comment compares current vs. previous run.
- The `custom-codereview-guide.md` mentions "eval / benchmark risk" PR review but does not reference any historical metric.

## Architectural Decisions

- **Multi-tier separation of E2E concerns**: mock-LLM tests run on every PR; live-LLM tests are label-gated because they require real LLM credentials and are non-deterministic (`.github/workflows/ci.yml:83-89`; `AGENTS.md` lines "Live End-to-End Test Framework" + "Mock-LLM E2E Test Framework").
- **Required-check gating via GitHub Actions matrix**: the `test-and-build (ubuntu)` matrix entry is the single load-bearing PR/release gate (`.github/workflows/release.yml:30`).
- **Selective test execution by file mapping**: `tests/e2e/mock-llm/test-mapping.json` maps source paths to test subdirectories; the resolver fails closed to full suite if mapping breaks — keeping required checks reliable while limiting CI cost (`.github/workflows/mock-llm-e2e.yml:170-191`).
- **Self-tests for CI infrastructure**: `ci-script-tests.yml` runs pytest over the Python checkers when they change, so the gating logic itself has regression coverage (`.github/workflows/ci-script-tests.yml:9-37`).
- **No coverage / mutation threshold on purpose**: the project deliberately postpones thresholds until "a stable baseline" is established — preferring a fast iteration loop first (`docs/DEVELOPMENT.md:144-146`).
- **PR artifacts are ephemeral**: `.pr/` directory contains QA screenshots/videos that are pushed to the PR branch but removed on approval so they never enter the squash merge (`.github/workflows/pr-artifacts.yml:74-123`).
- **Fork-PR safety**: live E2E workflow checks for `is_fork == 'true'` and short-circuits before any secret-handling step (`.github/workflows/ci.yml:147-281`).

## Notable Patterns

- **Path-triggered "detect-pr-changes" gate**: `.github/workflows/mock-llm-e2e.yml:17-47` and `mock-llm-docker-e2e.yml:51-80` use a lightweight `detect-pr-changes` job that calls `gh api pulls/{n}/files` and outputs `should_run: true/false`. This keeps required checks "completed" (not "pending") for docs-only PRs while skipping heavy jobs.
- **Reusable workflow composition for release actions**: `OpenHands/release-actions/.github/workflows/release-please.yml`, `pr-title.yml`, and `release-ready.yml` are imported across `release.yml`, `pr.yml`, `release-ready.yml`. Centralizes versioning policy.
- **Marker-based Playwright → CI handoff**: `DoneMarkerReporter` writes `.mock-llm-markers/.all-passed` only when all tests pass, so the CI wrapper can disambiguate "Playwright killed during teardown" from "tests failed" (`.github/workflows/mock-llm-e2e.yml:239-267`).
- **PR comment upsert by hidden marker**: both `tests/e2e/live/scripts/upsert-pr-comment.mjs` and `tests/e2e/mock-llm/scripts/upsert-pr-comment.mjs` use HTML-comment markers (`<!-- agent-canvas-live-e2e-report -->`, `<!-- agent-canvas-mock-llm-e2e-report -->`) to find and replace previous reports rather than spamming new comments.
- **Per-spec "� new" badge in mock-LLM report**: the workflow queries GitHub's API for files with `status == "added"` matching `tests/e2e/mock-llm/**/*.spec.ts` and badges them in the report (`tests/e2e/mock-llm/scripts/render-mock-llm-report.mjs`; `.github/workflows/mock-llm-e2e.yml:283-298`).
- **Required-check orchestration via `concurrency`**: `ci.yml` uses per-commit push concurrency groups and never cancels push runs, to avoid losing the green check that release-please depends on (`.github/workflows/ci.yml:16-22`).
- **Live E2E artifact-on-approval pattern**: `.github/workflows/pr-artifacts.yml:16-213` removes `.pr/` directory after PR approval — a one-way ratchet that prevents ephemeral QA content from polluting main.

## Tradeoffs

- **Mock-LLM E2E is the practical eval tier for this UI repo**: by design it uses scripted trajectories rather than real LLMs, so its pass-rate is deterministic. That makes it a strong merge gate but a poor signal for production LLM-behavior regressions (those would need `live-e2e`, which is opt-in).
- **Coverage and mutation testing are off the gate**: keeps the iteration loop fast (`docs/DEVELOPMENT.md:144-146`) but means silent drops in test quality are not blocked.
- **No run-history storage**: simpler infra, no dashboard infra to maintain, but no observability into "test X has been flaky for 3 weeks" or "pass-rate dropped from 95% to 80% after PR Y".
- **Live E2E is opt-in to keep CI cost low**: but real LLM regressions only surface when someone remembers to apply the `live-e2e` label — a process risk, not a tooling gap.
- **Release gate is on the simple `test-and-build` matrix entry, not the heavier E2E jobs**: keeps release latency low but means a passing release tag is not necessarily a guarantee of E2E green (E2E jobs run as separate required checks rather than release blockers).
- **Required-check detection of "no run" vs "passed" handled via API polling**: `.github/workflows/release.yml:50-67` polls the check-runs API rather than depending on a workflow_call/dependency graph — works across multiple workflows but adds 40 minutes of wait budget.

## Failure Modes / Edge Cases

- **Fork PR live-E2E leak**: explicitly guarded by `is_fork != 'true'` checks; if a future contributor removes a guard, the LLM credential would be exposed. Mitigated by the `Skip live E2E for fork PRs` step (`.github/workflows/ci.yml:147-149`).
- **PR-artifact-only commits**: live E2E detects commits that change only `.pr/*` and skips Playwright — protects against infinite loops when the artifact-push workflow itself adds commits (`.github/workflows/ci.yml:181-203`).
- **Playwright teardown hang**: the mock-LLM workflow gives a 5-second grace then `kill -9`s Playwright, then trusts the `.all-passed` marker rather than the (non-zero) Playwright exit code (`.github/workflows/mock-llm-e2e.yml:239-267`). Without the marker, a teardown hang would falsely fail the job.
- **Affected-test resolver failure**: fails closed to running the full suite (`.github/workflows/mock-llm-e2e.yml:170-176`), preventing a malformed `test-mapping.json` from silently skipping tests.
- **Docker workflow still building when E2E starts**: Docker-E2E polls `docker.yml` runs by `head_sha` with a 15-minute timeout (`.github/workflows/mock-llm-docker-e2e.yml:178-223`); longer Docker builds would time out and fail.
- **`LLM_API_KEY` missing in live E2E**: explicit skip path (`Skip live Agent Server E2E because LLM_API_KEY is not configured.`, `.github/workflows/ci.yml:275-277`) — without it, every PR without a configured secret would fail.
- **Mock-LLM server cold start**: `tests/e2e/mock-llm/scripts/mock-llm-server.py` is verified by a 30-attempt retry loop because `openhands-sdk`'s litellm import is slow (`.github/workflows/mock-llm-e2e.yml:130-147`).
- **Test mapping file drift**: cross-cutting source files (e.g. `src/api/agent-server-adapter.ts`) trigger the full suite via `runAllSources`, so the resolver never silently skips a needed test (`.github/workflows/mock-llm-e2e.yml:182-189`).
- **No evidence of "no flake tracking"**: tests can be flaky (mock-LLM retries are documented at `tests/e2e/mock-llm/utils/mock-llm-helpers.ts` for individual specs), but no aggregate flake metric is exposed in PR reports.

## Future Considerations

- **Wire mutation testing into CI on PR diffs**: `scripts/stryker-diff.mjs` already computes the file list and runs Stryker locally; adding a `mutation-diff` job to `.github/workflows/mock-llm-e2e.yml` would close the "soft metric" gap once the project is ready to set thresholds (`docs/DEVELOPMENT.md:144-146`).
- **Add coverage threshold to `vite.config.ts:490-494`**: a `coverage.thresholds` block would make coverage a merge gate rather than report-only. Today's setup ships the report but never enforces it.
- **Persist per-run results in a GH-Pages dashboard or store them in an artifact-tracked history table**: would unlock pass-rate drift tracking. Today the artifacts are uploaded (`.github/workflows/mock-llm-e2e.yml:271-281`) but no job consumes them across runs.
- **Promote `live-e2e` from opt-in to required on label-only or scheduled nightly**: would catch real-LLM regressions that mock-LLM cannot simulate. Cost is the main blocker (LLM API calls per PR).
- **Track flake-rate per spec across runs**: requires the persistence change above; would catch the "test X has been intermittent for a month" class of regressions that currently hide in PR comments.
- **Make the mock-LLM Docker E2E a release gate**: today the Docker image can ship even if Docker E2E fails (the workflow is on `pull_request` and `workflow_run`, but `docker.yml` does not depend on it). Adding a `needs:` from `docker.yml::merge-manifests` to the E2E workflow would harden the release image.
- **Snapshot the "all tests passing" manifest per release**: a checked-in `known-good-baseline.json` (or similar) would give the project its first concrete baseline to compare against; combined with the existing `affected_tests` mechanism, it would power pass-rate drift detection.

## Questions / Gaps

- **Where does pass-rate actually live over time?** No evidence found. There is no scheduled job that aggregates `test-results-*/results.json` across runs, no BigQuery export, no JSON-lines log. Search boundary: `tests/`, `scripts/`, `.github/workflows/`, `package.json` scripts, `vitest.setup.ts`, `tests/e2e/mock-llm/scripts/*`, `tests/e2e/live/scripts/*`.
- **Is the `live-e2e` PR check enforced via GitHub branch protection, or only advisory?** No evidence found in the repo. The workflow itself is a required-check shape, but the repo's branch-protection settings live in the GitHub UI and are not encoded here. Search boundary: `.github/`, `docs/`.
- **Are there any dashboards-as-code (e.g. Grafana JSON, Datadog monitors, GH Pages status page)?** No evidence found. Search boundary: `*.json`, `*.md` outside `node_modules`, `.github/`.
- **Is the "always-run `regressions`" directory the closest thing to a baseline, or is there a stored expected-results file?** No evidence found beyond the directory list at `tests/e2e/mock-llm/test-mapping.json:106`. Search boundary: `tests/e2e/`, `__tests__/`, `src/`.
- **What happens when a flaky test passes via retry on PR but fails on merge?** No evidence found. The reporter tracks `retryCount` (`tests/e2e/mock-llm/scripts/render-mock-llm-report.mjs:73`) but no metric gates on it.
- **Why is mutation testing deliberately not in CI?** Evidence: `docs/DEVELOPMENT.md:144-146` — explicit deferral until a stable baseline exists. The baseline does not yet exist.
- **Is there a documented "how to add a regression test" flow tied to this dimension?** Yes for the suite itself (see `AGENTS.md` "Testing Rules"), but not for the gating/observability story.

---

Generated by `18.03-regression-gating-and-ci-integration` against `openhands`.
