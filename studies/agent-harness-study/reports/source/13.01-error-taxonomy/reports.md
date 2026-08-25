# Source Analysis: reports

## 13.01: Error Taxonomy

### Source Info

| Field | Value |
|-------|-------|
| Name | reports |
| Path | `studies/agent-harness-study/sources/reports` |
| Language / Stack | None — the source contains no files (no code, config, or docs) |
| Analyzed | 2026-08-23 |

**Citation note.** The selected source contains zero files, so no source-side symbol can be cited as `path/to/file.ts:NN`. To keep every claim traceable at line granularity, this report cites its own audited lines using the workspace-relative form `studies/agent-harness-study/reports/source/13.01-error-taxonomy/reports.md:NN`. Each such anchor resolves either to the Search Audit Record (`studies/agent-harness-study/reports/source/13.01-error-taxonomy/reports.md:106`) or to a finding section that restates its result. Bare directory paths elsewhere in this report describe what was inspected; they are context, not code citations, because directories carry no line numbers.

## Summary

The selected source is a directory snapshot that is effectively empty. A full recursive inspection of `studies/agent-harness-study/sources/reports` found exactly two directories and zero files:

- `studies/agent-harness-study/sources/reports/source` (directory, no files)
- `studies/agent-harness-study/sources/reports/source/07.04-timeouts-and-cancellation` (empty directory)

That result is anchored three ways: a directory-tree listing produced exactly two entries and no files (`studies/agent-harness-study/reports/source/13.01-error-taxonomy/reports.md:110`); a recursive file-and-symlink scan returned zero matches (`studies/agent-harness-study/reports/source/13.01-error-taxonomy/reports.md:111`); and hidden-file checks at every level listed nothing but those two directories (`studies/agent-harness-study/reports/source/13.01-error-taxonomy/reports.md:113`).

Because there are no source files, there is no error taxonomy to study: no error type definitions (`No clear evidence found`), no classification-by-source code (`No clear evidence found`), no handling dispatch (`No clear evidence found`), and no error documentation (`No clear evidence found`). Every dimension question resolves to "not answerable within this source's boundary." The rating reflects absence of any observable error model rather than a judgment about quality of an existing one.

Per the study isolation rules, no sibling sources or workspace files outside `studies/agent-harness-study/sources/reports` were inspected.

## Rating

**1 / 10 — Absent.**

Rationale against the rubric:

| Rubric band | Applicability |
|-------------|---------------|
| 1-3 (Absent, implicit, ad-hoc, unsafe) | **Matches.** There is no error taxonomy at all in this source: zero error enums, zero classification code, zero handlers, zero docs. Nothing exists to be implicit or ad-hoc; the dimension is simply absent. |
| 4-6 (Present but inconsistent) | Not applicable — nothing is present. |
| 7-8 (Clear model with tests/interfaces) | Not applicable. |
| 9-10 (Mature, durable, extensible) | Not applicable. |

Score of 1 (rather than 2-3) because even weak taxonomies leave some artifact (a string constant, a catch-all branch); this source leaves none (`studies/agent-harness-study/reports/source/13.01-error-taxonomy/reports.md:111`). The only structural observation available is that the directory layout itself encodes a dimension-based organization convention (`source/<NN.NN-dimension-name>/`, e.g. `studies/agent-harness-study/sources/reports/source/07.04-timeouts-and-cancellation`), but with no files inside, this cannot be treated as an error-taxonomy mechanism (`studies/agent-harness-study/reports/source/13.01-error-taxonomy/reports.md:110`).

> Rubric probe — *"Can you tell from the error type whether to retry, escalate, or stop?"*: **No.** No error types exist in this source, so no retry/escalate/stop decision can be derived from them.

## Evidence Collected

Every entry includes a line-anchored citation. Because the source holds no files, each anchor points into this report's Search Audit Record instead of into a source file; the inspected location is preserved in the Evidence column.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Error type definitions / enums | No clear evidence found. Recursive listing of the source returned zero files; no `.ts/.go/.py/.rs/.json/.yaml` artifacts exist under `studies/agent-harness-study/sources/reports/source/07.04-timeouts-and-cancellation` to define error types. | `studies/agent-harness-study/reports/source/13.01-error-taxonomy/reports.md:111` |
| Error classification by source category (model/provider/tool/validation/policy/context/user/infrastructure/timeout) | No clear evidence found. No code files exist anywhere under `studies/agent-harness-study/sources/reports`, confirmed by hidden-file checks at every level. | `studies/agent-harness-study/reports/source/13.01-error-taxonomy/reports.md:113` |
| Error handling dispatch (routing on error kind) | No clear evidence found. Zero files means zero switch/match/if-chains on error categories; the size audit shows only two directory rows and no file rows. | `studies/agent-harness-study/reports/source/13.01-error-taxonomy/reports.md:112` |
| Error documentation (docs describing taxonomy) | No clear evidence found. No README, markdown, or comment-bearing files exist anywhere under `studies/agent-harness-study/sources/reports`. | `studies/agent-harness-study/reports/source/13.01-error-taxonomy/reports.md:111` |
| Tests demonstrating intended error behavior | No clear evidence found. The recursive file-and-symlink scan would have matched test files of any extension; it returned nothing. | `studies/agent-harness-study/reports/source/13.01-error-taxonomy/reports.md:111` |
| Structural convention (only observable artifact) | Directory naming follows `<NN.NN>-<dimension-slug>` (e.g., `studies/agent-harness-study/sources/reports/source/07.04-timeouts-and-cancellation`), implying per-dimension report placement — but it is empty, so intent is inferred, not implemented. | `studies/agent-harness-study/reports/source/13.01-error-taxonomy/reports.md:110` |

## Answers to Dimension Questions

1. **Are errors classified by source?**
   `No clear evidence found.` The source contains zero files (recursive scan: `studies/agent-harness-study/reports/source/13.01-error-taxonomy/reports.md:111`), so no classification code exists to inspect.

2. **Is the taxonomy used for handling?**
   `No clear evidence found.` With no error types and no handler code present, no dispatch behavior can be observed or tested. This question cannot be answered within the source boundary.

3. **Are error categories documented?**
   `No clear evidence found.` No markdown, README, or doc files exist under `studies/agent-harness-study/sources/reports`; both scans confirm an empty file set (`studies/agent-harness-study/reports/source/13.01-error-taxonomy/reports.md:111`, `studies/agent-harness-study/reports/source/13.01-error-taxonomy/reports.md:113`).

4. **Can new error types be added without breaking existing handling?**
   `No clear evidence found.` Extensibility cannot be assessed: there are no existing types, interfaces, or handlers whose compatibility could be evaluated. Trivially, "adding" anything breaks nothing because nothing exists — but that is vacuous, not extensible design.

## Architectural Decisions

`No clear evidence found.` No implementation files exist from which architectural decisions could be inferred (`studies/agent-harness-study/reports/source/13.01-error-taxonomy/reports.md:111`). The single observable decision is organizational: the source uses a nested `source/<dimension-id>-<slug>/` layout (`studies/agent-harness-study/sources/reports/source/07.04-timeouts-and-cancellation`), suggesting content was expected to be organized per study dimension. Whether that convention is load-bearing (parsed by tooling) or purely cosmetic cannot be determined from this source alone.

## Notable Patterns

`No clear evidence found.` No code patterns exist (`studies/agent-harness-study/reports/source/13.01-error-taxonomy/reports.md:111`). The only pattern-like signal is the empty placeholder-directory structure noted above; it is classified here as scaffolding, not as an error-handling pattern.

## Tradeoffs

Nothing to trade off — the source has no implementation. Two boundary observations instead:

- **Snapshot vs. live tree:** If this directory was meant to be a snapshot of generated reports, the tradeoff of snapshotting (isolation, reproducibility) was paid for but the payload never landed — the consumer gets isolation guarantees over an empty set (`studies/agent-harness-study/reports/source/13.01-error-taxonomy/reports.md:112`).
- **Convention-over-content risk:** A rigid `NN.NN-slug/` layout communicates expected structure even when content is missing, which can mask emptiness downstream (consumers see a well-formed path and may assume substance).

## Failure Modes / Edge Cases

- **Empty-input failure mode (observed):** Any pipeline expecting report content at `studies/agent-harness-study/sources/reports/source/07.04-timeouts-and-cancellation` would fail or silently produce empty output, since the directory holds no files (`studies/agent-harness-study/reports/source/13.01-error-taxonomy/reports.md:113`).
- **Silent-absence hazard:** Because directories exist, naive existence checks (`os.path.isdir`) succeed while content reads fail — a classic edge case where presence-of-path does not imply availability-of-data.
- **Timeout/cancellation linkage unobservable:** The sole named subdirectory references dimension 07.04 (timeouts and cancellation), which typically correlates with timeout-class errors; whether such errors were ever recorded here is unknowable from the empty directory.

## Future Considerations

- Populate the source directory before re-running this dimension, then re-score; the current rating applies strictly to the empty state.
- Add a harness-level preflight check that fails fast when a selected source directory contains zero files, converting silent-empty snapshots into explicit errors.
- If the `NN.NN-slug/` convention is meaningful, document its contract (who writes into it, what file names are expected) once content exists.

## Questions / Gaps

- Why is `studies/agent-harness-study/sources/reports` empty — was content generation skipped, moved elsewhere, or deleted? Unanswerable within the source boundary.
- Is the nested `source/07.04-timeouts-and-cancellation/` path produced by the same harness that renders this prompt (i.e., a prior dimension's output staged as input)? Unanswerable without inspecting sibling sources, which is banned.
- All four dimension questions remain open pending a non-empty source; see the Search Audit Record below for the complete command-level evidence.

### Search Audit Record

All commands were run from the workspace root against the selected source only:

1. Directory-tree listing (`find studies/agent-harness-study/sources/reports -mindepth 1 | sort`) → exactly two entries: `source` and `source/07.04-timeouts-and-cancellation`; no files.
2. Recursive file-and-symlink scan (`find studies/agent-harness-study/sources/reports -type f -o -type l`) → zero results, exit status 0.
3. Size audit (`du -a studies/agent-harness-study/sources/reports`) → 12 blocks total across two directory rows; no file rows.
4. Hidden-file checks (`ls -A` at `studies/agent-harness-study/sources/reports`, at `source`, and at `source/07.04-timeouts-and-cancellation`) → only the directory entries above; the leaf directory is completely empty.

---

Generated by `Dimension 13.01: Error Taxonomy` against `reports`.
