# Source Analysis: temporal

## 13.01 Error Taxonomy

### Source Info

| Field | Value |
|-------|-------|
| Name | temporal |
| Path | `studies/agent-harness-study/sources/temporal` |
| Language / Stack | Unknown (source payload missing) |
| Analyzed | 2026-08-22 |

## Summary

The selected source directory `studies/agent-harness-study/sources/temporal/` is empty on disk. No implementation files, tests, configuration, manifests, or documentation were present at the time of this study. The only artifact in the sources folder for this target is the source descriptor `studies/agent-harness-study/sources/temporal.ultraplan-source.yml:1-64`, which declares the upstream URL as `https://github.com/temporalio/temporal` and the framing "Gold standard for workflow durability and replay", but does not include any code, error definitions, or taxonomy material.

Because the source payload is missing, there is no implementation, no public API, no error type enums, no error classification code, no error handling dispatch, and no error documentation to inspect. All dimension steps, evidence requirements, and questions are therefore answered against a null evidence base. The analysis below follows the template, but every section explicitly records "No clear evidence found" together with the search boundary that was actually executed.

Search boundary executed for this task:

- `ls -la studies/agent-harness-study/sources/temporal/` → empty directory (`.` and `..` only).
- `find studies/agent-harness-study/sources/temporal -mindepth 1 -maxdepth 5` → zero entries.
- `find studies/agent-harness-study/sources/temporal -type f` → zero files.
- Read of `studies/agent-harness-study/sources/temporal.ultraplan-source.yml:1-64` → metadata-only.
- No cross-source inspection was performed; sibling sources under `studies/agent-harness-study/sources/` were intentionally not read, per the isolation rules in the base prompt.

## Rating

**1 / 10** — Absent.

The source contains no implementation or tests from which an error taxonomy, retry/escalation/stop policy, extensibility strategy, or operational safeguard can be established. The score reflects the absence of analyzable implementation rather than a claim about a particular taxonomy in an unavailable dependency.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`. No source files exist, so no line-citable implementation evidence is available.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Source inventory | No evidence found; the selected source directory is empty. | `studies/agent-harness-study/sources/temporal/` (no file or line available) |
| Source metadata | Descriptor pointing at upstream repository; no code payload. | `studies/agent-harness-study/sources/temporal.ultraplan-source.yml:1-64` |
| Error type definitions | No evidence found. | `studies/agent-harness-study/sources/temporal/` (no file or line available) |
| Error classification | No evidence found. | `studies/agent-harness-study/sources/temporal/` (no file or line available) |
| Error handling dispatch | No evidence found. | `studies/agent-harness-study/sources/temporal/` (no file or line available) |
| Tests | No evidence found. | `studies/agent-harness-study/sources/temporal/` (no file or line available) |
| Documentation | No evidence found. | `studies/agent-harness-study/sources/temporal/` (no file or line available) |

## Answers to Dimension Questions

1. **Are errors classified by source?** No evidence found. There are no files under `studies/agent-harness-study/sources/temporal/` from which a category such as model, provider, tool, validation, policy, context, user, infrastructure, or timeout could be inspected. The descriptor at `studies/agent-harness-study/sources/temporal.ultraplan-source.yml:3` only states a generic "Gold standard for workflow durability and replay" framing.
2. **Is the taxonomy used for handling?** No evidence found. No dispatch or handling code exists in the selected source, so routing, retries, escalation, and stopping behavior cannot be determined.
3. **Are error categories documented?** No evidence found. No documentation files exist under the selected source; the `.ultraplan-source.yml` file is metadata only.
4. **Can new error types be added without breaking existing handling?** Cannot be determined. The empty source provides no error interfaces, dispatch contract, tests, or extension points to evaluate.

## Architectural Decisions

No evidence found. The selected source exposes no observable architectural decisions, error interfaces, classification rules, or handling policy. The descriptor at `studies/agent-harness-study/sources/temporal.ultraplan-source.yml:1-64` does not encode any taxonomy decisions.

## Notable Patterns

No evidence found. No implementation or test files exist under `studies/agent-harness-study/sources/temporal/`.

## Tradeoffs

The empty source provides no evidence for assessing tradeoffs. In particular, it cannot establish whether errors are extensible, whether callers receive actionable classification, or whether retry behavior avoids duplicate side effects. Any conclusion beyond the absence of source material would be unsupported.

## Failure Modes / Edge Cases

No evidence found for timeout behavior, retries, escalation, stop conditions, partial failures, validation failures, provider failures, or infrastructure failures. Because no code or tests are present, the source cannot support a claim about how any of these cases are handled.

## Future Considerations

If the source is populated, add and document an explicit error category model covering the requested source categories (model, provider, tool, validation, policy, context, user, infrastructure, timeout), define the retry/escalate/stop policy for each category, and centralize dispatch so classification drives handling. Add tests for new categories without changing existing dispatch contracts, plus failure and observability coverage.

## Questions / Gaps

- The selected source directory contains zero files, so this study is bounded to an empty input.
- The language, stack, public interfaces, error taxonomy, handling behavior, tests, and documentation are all unknown.
- A representative implementation or populated source is required before a meaningful rating above the absence rating can be assigned.

---

Generated by `13.01-error-taxonomy` against `temporal`.
