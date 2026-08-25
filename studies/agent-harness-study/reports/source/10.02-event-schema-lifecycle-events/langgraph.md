# Source Analysis: langgraph

## Dimension 10.02 — Event Schema and Lifecycle Events

### Source Info

| Field | Value |
|-------|-------|
| Name | langgraph |
| Path | `studies/agent-harness-study/sources/langgraph` |
| Language / Stack | Unknown — source not present in workspace |
| Analyzed | 2026-08-23 |

## Summary

The selected source directory `studies/agent-harness-study/sources/langgraph` is empty in this workspace. No source files, tests, configuration, or schemas are available to inspect. The `langgraph.ultraplan-source.yml` metadata (lines 1-103) declares the upstream as `https://github.com/langchain-ai/langgraph` and lists this dimension (`10.02`) among `applicable_dimensions`, but no checkout, mirror, or vendored copy was produced into the source directory before this study was dispatched.

Because hard rule #1 forbids cross-source filesystem access, no sibling sources (e.g., `langfuse/`, `openhands/`, `pydantic-ai/`, `temporal/`) may be inspected to substitute evidence. The dimension therefore has no observable evidence inside the selected source boundary, and every question in the dimension is answered as "no evidence found" with the search boundary explicitly stated.

## Rating

**Score: 1 / 10**

Rationale: The dimension requires inspecting event type definitions, ordering, versioning, and lifecycle coverage (creation, completion, failure, cancellation). With the source directory empty, none of these can be observed, verified, or refuted. A score of 1 (Absent) is the only defensible rating under the rubric: there is no implemented, implicit, or ad-hoc model to evaluate within the isolated source.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Event schemas | No evidence found — source directory is empty | `studies/agent-harness-study/sources/langgraph/` (no files) |
| Event emitters | No evidence found — source directory is empty | `studies/agent-harness-study/sources/langgraph/` (no files) |
| Sequence numbers | No evidence found — source directory is empty | `studies/agent-harness-study/sources/langgraph/` (no files) |
| Event version fields | No evidence found — source directory is empty | `studies/agent-harness-study/sources/langgraph/` (no files) |
| Lifecycle event types | No evidence found — source directory is empty | `studies/agent-harness-study/sources/langgraph/` (no files) |
| Source metadata | YAML declares upstream URL and applicable dimensions | `sources/langgraph.ultraplan-source.yml:1-103` |

Search boundary: `ls -laR studies/agent-harness-study/sources/langgraph` returns only `.` and `..`; `find` / `grep` over the directory yield zero hits. Per hard rule #1, sibling sources and other workspace trees outside the selected directory were not consulted.

## Answers to Dimension Questions

1. **Are events typed and versioned?** — No evidence found. No event type definitions, schemas, or version fields are present in the selected source directory.
2. **Are events ordered and timestamped?** — No evidence found. No sequence numbers, monotonic counters, or timestamp fields are available for inspection.
3. **Do events carry sufficient context?** — No evidence found. Without event payloads we cannot confirm whether run IDs, span IDs, or tool call IDs are propagated.
4. **Are lifecycle events comprehensive?** — No evidence found. Creation, completion, failure, and cancellation event types cannot be observed; whether the run lifecycle is fully reconstructible from events alone is undecidable from this source as configured.

## Architectural Decisions

No clear evidence found. No architectural decisions are observable in the empty source directory.

## Notable Patterns

No clear evidence found. No code is available to characterize patterns.

## Tradeoffs

No clear evidence found. Tradeoffs cannot be inferred from an empty source boundary.

## Failure Modes / Edge Cases

No clear evidence found. Failure-mode handling, retry semantics, and edge-case coverage are not inspectable here.

## Future Considerations

- Provision the source: clone or vendor `https://github.com/langchain-ai/langgraph` into `studies/agent-harness-study/sources/langgraph/` (matching the upstream URL declared in `sources/langgraph.ultraplan-source.yml:2`) so this dimension can be re-run with real evidence.
- Once the checkout exists, re-dispatch this study and prioritize inspection of any `events`, `schema`, or `lifecycle` modules, plus the streaming API surface (`stream_mode`/`astream_events`) which is the most likely carrier for lifecycle events in langgraph-style runtimes.
- Cross-check whether langgraph exposes `StreamWriter` events, node-enter/node-exit hooks, or durable-event checkpoint tuples — these are the candidate constructs the dimension would need to cite.

## Questions / Gaps

- Why is the source directory empty? Was the clone step skipped, filtered out by `.gitignore`, or never executed by the study driver?
- Should the study driver fall back to fetching upstream sources on demand when a directory is empty, while still respecting rule #1 by writing the fetch artifacts inside the source boundary?
- Once content is available, are there sub-projects (e.g., `libs/langgraph`, `libs/checkpoint`, `libs/sdk-js`) that should be the primary inspection surface for event schemas?

---

Generated by `dimensions/10.02-event-schema-and-lifecycle-events` against `langgraph`.
