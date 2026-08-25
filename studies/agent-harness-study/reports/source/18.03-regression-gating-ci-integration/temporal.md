# Source Analysis: temporal

## 18.03 Regression Gating and CI Integration

### Source Info

| Field | Value |
|-------|-------|
| Name | temporal |
| Path | `studies/agent-harness-study/sources/temporal` |
| Language / Stack | Unknown (source directory is empty; metadata declares upstream `https://github.com/temporalio/temporal`, a Go-based workflow engine) |
| Analyzed | 2026-08-23 |

## Summary

The selected source directory `studies/agent-harness-study/sources/temporal/` is empty: it contains no files (neither regular files nor dotfiles) and no subdirectories. Only the `.` and `..` directory entries are present. Because the hard rules of this study forbid cross-source filesystem access, no other source directories were inspected and no external references (e.g., the upstream `temporalio/temporal` repository on GitHub) were fetched.

With zero files in scope, there is no code, configuration, CI definition, regression gate, baseline comparison script, drift dashboard, or eval report generator to cite. Every dimension question for 18.03 therefore resolves to "no evidence found" inside the in-scope boundary. The findings below state this explicitly per the template's "no finding" guidance.

Search boundary used:
- `ls -la studies/agent-harness-study/sources/temporal/` — confirms directory is empty (only `.` and `..`).
- `find studies/agent-harness-study/sources/temporal/ -type f` — returns no results.
- The companion `sources/temporal.ultraplan-source.yml:1-3` is the only artifact associated with the source and is metadata only (name, URL, description); it does not contain code or CI evidence.

## Rating

**Score: 1 / 10**

Rationale: A score of 1 ("Absent, implicit, ad-hoc, or unsafe") is the only honest rating because the evaluation has no in-scope artifacts to evaluate. Nothing is "present but inconsistent" — nothing is present at all within the source isolation boundary. There is no CI YAML, no regression-gate script, no baseline comparator, no drift dashboard, and no report generator in the selected directory. The rating cannot be raised above 1 without evidence, and the rules forbid reaching outside the source directory to gather it.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| CI pipeline config | No clear evidence found — directory `studies/agent-harness-study/sources/temporal/` contains zero files; no `.github/workflows/`, `.circleci/`, `.gitlab-ci.yml`, `Makefile` CI target, or equivalent was found within the source boundary. | n/a |
| Regression gate code | No clear evidence found — no executable, library, or script files exist in the source directory, so no gate logic is reachable for citation. | n/a |
| Baseline comparison scripts | No clear evidence found — directory is empty; no comparison, snapshot, or golden-file harness exists in scope. | n/a |
| Drift tracking dashboards | No clear evidence found — no config (e.g., Grafana, Datadog), schema, or query file exists in the source directory. | n/a |
| Eval report generation | No clear evidence found — no template, formatter, or report-writer code exists in the source directory. | n/a |
| Source metadata only (context, not code) | `sources/temporal.ultraplan-source.yml:1-3` declares `name: "temporal"`, upstream URL `https://github.com/temporalio/temporal`, and description "Gold standard for workflow durability and replay." This file declares the source but is not itself implementation evidence. | `sources/temporal.ultraplan-source.yml:1-3` |

## Answers to Dimension Questions

1. **Do evals run in CI?** — No clear evidence found. No CI configuration file of any kind (GitHub Actions, CircleCI, GitLab, Buildkite, Jenkins, etc.) exists in `studies/agent-harness-study/sources/temporal/`. The directory contains zero files, so there is no in-scope artifact to inspect. The rule against cross-source filesystem access prevents borrowing an answer from sibling sources.
2. **Do regressions block deployments?** — No clear evidence found. No merge gate, branch protection config, status-check definition, or deployment script exists in the source directory. With no files, no blocking logic can be cited.
3. **Are results compared to baselines?** — No clear evidence found. No baseline, golden-output, snapshot, or threshold-comparison code exists in the source directory.
4. **Is pass-rate drift tracked?** — No clear evidence found. No time-series storage, metric definition, alert rule, or drift-detection code exists in the source directory.

## Architectural Decisions

No clear evidence found. No architectural artifacts (code, configuration, design documents) exist within the source directory to cite. The only architectural signal in scope is metadata in `sources/temporal.ultraplan-source.yml:1-3`, which describes the source as a "Gold standard for workflow durability and replay" but provides no implementation detail relevant to CI gating.

## Notable Patterns

No clear evidence found. No source files exist in the selected directory, so no patterns (eval harness layout, gate-threshold data structures, regression reporting idioms, etc.) can be observed. Per the cross-source isolation rule, patterns cannot be inferred from sibling sources.

## Tradeoffs

No clear evidence found. Tradeoffs for 18.03 normally involve choices such as "fail-fast vs. soft-warn," "absolute threshold vs. statistical control," or "PR-time gating vs. nightly gating." None of these can be characterized because there is no implementation to compare against. The only observable tradeoff relevant to this study is between breadth of evidence (which would require leaving the source boundary) and source isolation compliance (which forbids that). This study chooses compliance.

## Failure Modes / Edge Cases

No clear evidence found. The only failure mode observable within the source boundary is the absence of any source material itself: if a downstream consumer (another study dimension, a future contributor, or a CI consumer) expects to find a Temporal checkout under `studies/agent-harness-study/sources/temporal/`, every dimension that depends on in-scope files will fail the same way this one does. No code-level failure modes can be enumerated because no code is in scope.

## Future Considerations

- Populate the source directory before running this dimension. The minimum useful contents would be a Temporal clone (or a curated subset), so subsequent studies can cite concrete files such as `.github/workflows/*.yml`, `Makefile`, and `tools/` regression scripts.
- If only a subset of the upstream repository is in scope for this study, document that subset (e.g., via a manifest or a subdirectory) so analysts can search a non-empty boundary and cite file paths from within it.
- Once files are present, the dimension checklist is well-formed and can be re-run unchanged: the four questions (CI integration, deployment blocking, baseline comparison, drift tracking) are the right axes for 18.03 regardless of how the source is populated.

## Questions / Gaps

1. Why is `studies/agent-harness-study/sources/temporal/` empty? Was the upstream clone step skipped, deduplicated against another source, or intentionally left for later? `sources/temporal.ultraplan-source.yml:1-3` implies a populated checkout was expected.
2. Should the source-isolation rules allow fetching the declared upstream URL (`https://github.com/temporalio/temporal`) as a fallback when the local source directory is empty? The current prompt explicitly forbids this, which is the correct call for normal studies but produces a no-evidence report here.
3. Is `temporal.ultraplan-source.yml` itself in scope as evidence, or strictly out-of-scope as metadata? This report treats it as in-scope metadata only (cited at `sources/temporal.ultraplan-source.yml:1-3`) and not as implementation evidence.
4. Should empty-source studies be skipped at scheduling time rather than producing a 1/10 report? That is a workflow-level decision outside the dimension's scope, but it is worth surfacing.

---

Generated by `18.03-regression-gating-and-ci-integration` against `temporal`.
