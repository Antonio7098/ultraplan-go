# Source Analysis: reports

## 18.03 — Regression Gating and CI Integration

### Source Info

| Field | Value |
|-------|-------|
| Name | reports |
| Path | `studies/agent-harness-study/sources/reports` |
| Language / Stack | None — the source contains no files (no code, CI config, or docs) |
| Analyzed | 2026-08-23 |

**Citation note.** The selected source contains zero files, so no source-side symbol can be cited as `path/to/file.yml:NN`. To keep every claim traceable at line granularity, this report cites its own audited lines using the workspace-relative form `studies/agent-harness-study/reports/source/18.03-regression-gating-ci-integration/reports.md:NN`. Each such anchor resolves either to the Search Audit Record (`studies/agent-harness-study/reports/source/18.03-regression-gating-ci-integration/reports.md:106`) or to a finding section that restates its result. Bare directory paths elsewhere in this report describe what was inspected; they are context, not code citations, because directories carry no line numbers.

## Summary

The selected source is a directory snapshot that is effectively empty. A full recursive inspection of `studies/agent-harness-study/sources/reports` found exactly two directories and zero files:

- `studies/agent-harness-study/sources/reports/source` (directory, no files)
- `studies/agent-harness-study/sources/reports/source/07.04-timeouts-and-cancellation` (empty directory)

That result is anchored three ways: a directory-tree listing produced exactly those two entries and no files (`studies/agent-harness-study/reports/source/18.03-regression-gating-ci-integration/reports.md:110`); a recursive file-and-symlink scan returned zero matches (`studies/agent-harness-study/reports/source/18.03-regression-gating-ci-integration/reports.md:111`); and hidden-entry checks at every level listed nothing but those two directories (`studies/agent-harness-study/reports/source/18.03-regression-gating-ci-integration/reports.md:112`).

Because there are no source files, there is nothing to study for this dimension: no CI pipeline config, no regression gate code, no baseline comparison logic, no drift tracking, and no eval report generation. All four dimension questions resolve to "No clear evidence found"; none can be answered within this source's boundary. The rating reflects the absence of any observable gating mechanism rather than a judgment about the quality of an existing one.

Per the study isolation rules, no sibling sources or workspace files outside `studies/agent-harness-study/sources/reports` were inspected.

## Rating

**1 / 10 — Absent.**

Rationale against the rubric:

| Rubric band | Applicability |
|-------------|---------------|
| 1-3 (Absent, implicit, ad-hoc, unsafe) | **Matches.** There is no regression gating or CI integration of any kind in this source: zero workflow files, zero gate code, zero baselines, zero dashboards. Nothing exists to be implicit or ad-hoc; the property is simply absent. |
| 4-6 (Present but inconsistent) | Not applicable — nothing is present. |
| 7-8 (Clear model with tests/interfaces) | Not applicable. |
| 9-10 (Mature, durable, extensible) | Not applicable. |

Score of 1 (rather than 2-3) because even weak gating setups leave some artifact (a CI manifest, a threshold constant, a report template); this source leaves none (`studies/agent-harness-study/reports/source/18.03-regression-gating-ci-integration/reports.md:111`). The only structural observation available is the layout convention itself (`source/<NN.NN-dimension-slug>/`), which encodes expected per-dimension placement but carries no executable behavior (`studies/agent-harness-study/reports/source/18.03-regression-gating-ci-integration/reports.md:110`).

> Rubric probe — *"Can a code change that degrades quality be blocked before deployment?"*: **Not answerable here.** This source defines no deployment path, no checks, and no gates, so it can block nothing; the question cannot be evaluated within this boundary.

## Evidence Collected

Every entry includes a line-anchored citation. Because the source holds no files, each anchor points into this report's Search Audit Record instead of into a source file; the inspected location is preserved in the Evidence column.

| Area | Evidence | File:Line |
|------|----------|-----------|
| CI pipeline config | No clear evidence found. Recursive scan returned zero files; no `.yml`/`.yaml`/`.toml` workflow manifests exist anywhere under `studies/agent-harness-study/sources/reports`. | `studies/agent-harness-study/reports/source/18.03-regression-gating-ci-integration/reports.md:111` |
| Regression gate code | No clear evidence found. Zero files means zero branch-protection configs, required-check definitions, or gate scripts to inspect. | `studies/agent-harness-study/reports/source/18.03-regression-gating-ci-integration/reports.md:111` |
| Baseline comparison scripts | No clear evidence found. No baseline/golden/snapshot files exist; the recursive scan would have matched them regardless of extension. | `studies/agent-harness-study/reports/source/18.03-regression-gating-ci-integration/reports.md:111` |
| Drift tracking dashboards | No clear evidence found. Zero files excludes any dashboard-as-code artifact (Grafana JSON, GH Pages source, metric tables). Hidden-entry checks confirm nothing is masked. | `studies/agent-harness-study/reports/source/18.03-regression-gating-ci-integration/reports.md:112` |
| Eval report generation | No clear evidence found. No templates, renderers, or report writers exist under the source root. | `studies/agent-harness-study/reports/source/18.03-regression-gating-ci-integration/reports.md:111` |
| Structural convention (only observable artifact) | Directory naming follows `<NN.NN>-<dimension-slug>/` (e.g., `studies/agent-harness-study/sources/reports/source/07.04-timeouts-and-cancellation`), implying per-dimension staging — but the sole slot is empty, so intent is inferred, not implemented. | `studies/agent-harness-study/reports/source/18.03-regression-gating-ci-integration/reports.md:110` |

## Answers to Dimension Questions

1. **Do evals run in CI?**
   `No clear evidence found.` The source contains zero files (`studies/agent-harness-study/reports/source/18.03-regression-gating-ci-integration/reports.md:111`), so no CI pipeline definition exists to inspect.

2. **Do regressions block deployments?**
   `No clear evidence found.` With no gates, checks, or deployment configuration present, blocking behavior cannot be observed. This question cannot be answered within the source boundary.

3. **Are results compared to baselines?**
   `No clear evidence found.` No baseline artifacts exist anywhere under the source root (`studies/agent-harness-study/reports/source/18.03-regression-gating-ci-integration/reports.md:111`); there is nothing to compare.

4. **Is pass-rate drift tracked?**
   `No clear evidence found.` No historical result storage, dashboards, or trend metrics exist (`studies/agent-harness-study/reports/source/18.03-regression-gating-ci-integration/reports.md:112`); drift tracking cannot be assessed.

## Architectural Decisions

`No clear evidence found.` No implementation files exist from which architectural decisions could be inferred (`studies/agent-harness-study/reports/source/18.03-regression-gating-ci-integration/reports.md:111`). The single observable decision is organizational: the nested `source/<dimension-id>-<slug>/` layout (`studies/agent-harness-study/reports/source/18.03-regression-gating-ci-integration/reports.md:110`) suggests content was expected to be staged per study dimension. Whether that convention is load-bearing (consumed by tooling) or purely cosmetic cannot be determined from this source alone.

## Notable Patterns

`No clear evidence found.` No code patterns exist (`studies/agent-harness-study/reports/source/18.03-regression-gating-ci-integration/reports.md:111`). The only pattern-like signal is the empty placeholder-directory structure noted above; it is classified here as scaffolding, not as a gating mechanism.

## Tradeoffs

Nothing to trade off — the source has no implementation. Two boundary observations instead:

- **Snapshot vs. live tree:** If this directory was meant to snapshot generated evaluation reports, the isolation/reproducibility tradeoff was paid for but the payload never landed — consumers get isolation guarantees over an empty set (`studies/agent-harness-study/reports/source/18.03-regression-gating-ci-integration/reports.md:110`).
- **Convention-over-content risk:** A rigid `<NN.NN>-slug/` layout communicates expected structure even when content is missing, which can mask emptiness downstream (a well-formed path may be mistaken for substance).

## Failure Modes / Edge Cases

- **Empty-input failure mode (observed):** Any pipeline expecting eval reports at `studies/agent-harness-study/sources/reports/source/<dimension-slug>/` would fail or silently produce empty output, since the sole slot holds no files (`studies/agent-harness-study/reports/source/18.03-regression-gating-ci-integration/reports.md:112`).
- **Silent-absence hazard:** Because directories exist, naive existence checks succeed while content reads fail — presence-of-path does not imply availability-of-data.
- **Gate semantics unobservable:** The sole named slot references dimension 07.04 (timeouts and cancellation); whether any regression gate ever consumed content from it is unknowable from an empty directory.

## Future Considerations

- Populate the source directory before re-running this dimension, then re-score; the current rating applies strictly to the empty state.
- Add a harness-level preflight check that fails fast when a selected source directory contains zero files, converting silent-empty snapshots into explicit errors.
- If the per-dimension staging convention is meaningful, document its contract (who writes into it, what filenames are expected) once content exists.

## Questions / Gaps

- Why is `studies/agent-harness-study/sources/reports` empty — was content generation skipped, moved elsewhere, or deleted? Unanswerable within the source boundary.
- Is `sources/reports/source/07.04-timeouts-and-cancellation/` produced by the same harness that stages prior dimensions' outputs as inputs? Unanswerable without inspecting sibling sources, which is banned by the study's isolation rules.
- All four dimension questions remain open pending a non-empty source; see the Search Audit Record below for the complete command-level evidence (`studies/agent-harness-study/reports/source/18.03-regression-gating-ci-integration/reports.md:106`).

### Search Audit Record

All commands were run from the workspace root against the selected source only:

1. Directory-tree listing (`find studies/agent-harness-study/sources/reports -mindepth 1 | sort`) → exactly two entries: `source` and `source/07.04-timeouts-and-cancellation`; no files.
2. Recursive file-and-symlink count (`find studies/agent-harness-study/sources/reports \( -type f -o -type l \) | wc -l`) → `0`; exit status 0.
3. Hidden-entry checks (`ls -A` at `studies/agent-harness-study/sources/reports`, at `source`, and at `source/07.04-timeouts-and-cancellation`) → only the directory entries above; the leaf directory is completely empty.

---

Generated by `18.03-regression-gating-ci-integration` against `reports`.
