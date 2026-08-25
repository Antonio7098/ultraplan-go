# Source Analysis: opa

## 18.03 — Regression Gating and CI Integration

### Source Info

| Field | Value |
|-------|-------|
| Name | opa |
| Path | `studies/agent-harness-study/sources/opa` |
| Language / Stack | Unknown (source not present locally) |
| Analyzed | 2026-08-23 |

## Summary

No evidence found. The selected source directory `studies/agent-harness-study/sources/opa/` is empty in this workspace — `ls -la studies/agent-harness-study/sources/opa/` returns only `.` and `..`, and `find studies/agent-harness-study/sources/opa -mindepth 1` returns no files. Per the source-isolation rule, no sibling sources or upstream GitHub content was inspected.

Because there is no source code, configuration, CI workflow, or test harness present, none of the dimension's steps (CI integration, regression gating, baseline comparison, drift tracking, result reporting) can be evaluated against concrete artifacts. The study is reported with explicit "No evidence found" markers so that downstream readers can recognize the gap rather than infer conclusions from absence. The only related files in the workspace that are in scope for this report are the dimension definition at `dimensions/18.03-regression-gating-and-ci-integration.md:1` and the source manifest at `sources/opa.ultraplan-source.yml:1` that declares this dimension applicable to `opa` (`sources/opa.ultraplan-source.yml:25`).

## Rating

**Score: 1 / 10** — Absent.

Rationale: The rubric's 1–3 band is defined as "Absent, implicit, ad-hoc, or unsafe" at `dimensions/18.03-regression-gating-and-ci-integration.md:34`. With zero files in the source directory there is no CI pipeline config, no regression gate code, no baseline comparison script, no drift tracking dashboard, and no eval report generator to inspect. Any score above 1 would require evidence of a CI workflow or eval harness, which does not exist in the selected source. The rating cannot be raised without re-running the analysis once the source is materialized (e.g., a fresh clone of `https://github.com/open-policy-agent/opa` per `sources/opa.ultraplan-source.yml:2` into `studies/agent-harness-study/sources/opa/`).

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| CI pipeline config | No evidence found. Directory `studies/agent-harness-study/sources/opa` is empty; no `.github/workflows`, `.circleci`, `.gitlab-ci.yml`, `Makefile` CI targets, or analogous config files are present to inspect. Rubric definition: `dimensions/18.03-regression-gating-and-ci-integration.md:9` (step 1) and `dimensions/18.03-regression-gating-and-ci-integration.md:17` (evidence to capture). Scope confirmed: `sources/opa.ultraplan-source.yml:25` lists `18.03` as applicable to `opa`. | `dimensions/18.03-regression-gating-and-ci-integration.md:9`; `dimensions/18.03-regression-gating-and-ci-integration.md:17`; `sources/opa.ultraplan-source.yml:25` |
| Regression gate code | No evidence found. No source files exist to contain gate logic, threshold checks, or pre-merge hooks. Rubric definition: `dimensions/18.03-regression-gating-and-ci-integration.md:10` (step 2) and `dimensions/18.03-regression-gating-and-ci-integration.md:18` (evidence to capture). | `dimensions/18.03-regression-gating-and-ci-integration.md:10`; `dimensions/18.03-regression-gating-and-ci-integration.md:18` |
| Baseline comparison scripts | No evidence found. No scripts or binaries exist under the source directory for diffing eval runs against a stored baseline. Rubric definition: `dimensions/18.03-regression-gating-and-ci-integration.md:11` (step 3) and `dimensions/18.03-regression-gating-and-ci-integration.md:19` (evidence to capture). | `dimensions/18.03-regression-gating-and-ci-integration.md:11`; `dimensions/18.03-regression-gating-and-ci-integration.md:19` |
| Drift tracking dashboards | No evidence found. No telemetry, metrics export, or dashboard definition is present in the source tree. Rubric definition: `dimensions/18.03-regression-gating-and-ci-integration.md:12` (step 4) and `dimensions/18.03-regression-gating-and-ci-integration.md:20` (evidence to capture). | `dimensions/18.03-regression-gating-and-ci-integration.md:12`; `dimensions/18.03-regression-gating-and-ci-integration.md:20` |
| Eval report generation | No evidence found. No reporter, formatter, or report template exists to emit eval summaries. Rubric definition: `dimensions/18.03-regression-gating-and-ci-integration.md:13` (step 5) and `dimensions/18.03-regression-gating-and-ci-integration.md:21` (evidence to capture). | `dimensions/18.03-regression-gating-and-ci-integration.md:13`; `dimensions/18.03-regression-gating-and-ci-integration.md:21` |

Search boundary: `find studies/agent-harness-study/sources/opa -mindepth 1` returns 0 entries; `ls -la studies/agent-harness-study/sources/opa/` shows only `.` and `..`.

## Answers to Dimension Questions

1. **Do evals run in CI?** No evidence found. No CI workflow files are present in the selected source directory, so integration of evals into CI cannot be confirmed or denied from local artifacts. Question defined at `dimensions/18.03-regression-gating-and-ci-integration.md:25`.
2. **Do regressions block deployments?** No evidence found. With no regression gate code, deploy-blocking thresholds, or required-status-check definitions available locally, the blocking behavior cannot be characterized. Question defined at `dimensions/18.03-regression-gating-and-ci-integration.md:26`.
3. **Are results compared to baselines?** No evidence found. No baseline comparison logic, fixture store, or golden-file diff harness exists in the empty source tree. Question defined at `dimensions/18.03-regression-gating-and-ci-integration.md:27`.
4. **Is pass-rate drift tracked?** No evidence found. No drift metrics, trend storage, or alerting configuration is present locally to support a determination. Question defined at `dimensions/18.03-regression-gating-and-ci-integration.md:28`.

## Architectural Decisions

No clear evidence found. The source tree contains no files from which architectural decisions on CI, regression gating, baselines, or drift tracking can be extracted. The only in-workspace reference to the source's intended scope is `sources/opa.ultraplan-source.yml:1` (name) and `sources/opa.ultraplan-source.yml:4` (applicable_dimensions list).

## Notable Patterns

No clear evidence found. No implementation, configuration, or test files are present in the selected source directory, so no reusable patterns can be cited. The dimension's intended pattern of inquiry is enumerated at `dimensions/18.03-regression-gating-and-ci-integration.md:9-13`.

## Tradeoffs

No clear evidence found. Without source artifacts, tradeoffs between gating strictness and developer velocity, baseline freshness vs. flakiness, or drift sensitivity vs. noise cannot be analyzed. The dimension's central design question is recorded at `dimensions/18.03-regression-gating-and-ci-integration.md:39`.

## Failure Modes / Edge Cases

No clear evidence found. Potential failure modes (flaky evals gating merges, baseline rot, missed drift signals, CI runner capacity exhaustion) cannot be grounded in implementation evidence because no implementation is present locally. The dimension's rating-scale rubric that defines what counts as "unsafe" is at `dimensions/18.03-regression-gating-and-ci-integration.md:34-37`.

## Future Considerations

- Re-materialize the source: clone `https://github.com/open-policy-agent/opa` (declared at `sources/opa.ultraplan-source.yml:2`) into `studies/agent-harness-study/sources/opa/` so that this dimension, and the other dimensions listed for `opa` in the source manifest (`sources/opa.ultraplan-source.yml:4-37`), can be studied against actual code.
- Once the source is populated, prioritize inspection of `.github/workflows/`, `Makefile` CI targets, any `tester` / `regression` packages, and reporting utilities when re-running this dimension (see evidence-to-capture list at `dimensions/18.03-regression-gating-and-ci-integration.md:17-21`).
- Confirm whether OPA's own CI model is in scope for this agent-harness study, or whether the dimension is meant to apply only to evals specific to agent workflows — `sources/opa.ultraplan-source.yml:25` applies this dimension to OPA but the dimension prompt at `dimensions/18.03-regression-gating-and-ci-integration.md:5` references "evals", so scope alignment should be revisited.

## Questions / Gaps

- Why is `studies/agent-harness-study/sources/opa/` empty while sibling sources such as `langfuse/`, `langgraph/`, `openhands/` appear populated? This blocks the study for any of the dimensions listed at `sources/opa.ultraplan-source.yml:4-37`, not only 18.03.
- Does the UltraPlan harness expect sources to be cloned on demand, or is the missing materialization a configuration error that should be fixed before subsequent dimension studies?
- If OPA is intentionally out of scope for this dimension (it is a policy engine, not an eval harness), should the dimension manifest be revised to drop 18.03 from `opa`'s `applicable_dimensions` at `sources/opa.ultraplan-source.yml:25`?
- Without access to the upstream repository, even a re-run would require network access or a pre-staged clone; clarify the expected provisioning.

---

Generated by `dimensions/18.03-regression-gating-and-ci-integration.md:1` against `opa` (`sources/opa.ultraplan-source.yml:1`).