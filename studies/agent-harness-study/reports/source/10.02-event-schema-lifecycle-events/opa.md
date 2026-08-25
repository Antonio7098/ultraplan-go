# Source Analysis: opa

## Event Schema and Lifecycle Events

### Source Info

| Field | Value |
|-------|-------|
| Name | opa |
| Path | `studies/agent-harness-study/sources/opa` |
| Language / Stack | Unknown — source material not present locally |
| Analyzed | 2026-08-23 |

## Summary

The selected source directory `studies/agent-harness-study/sources/opa/` is empty. It contains no files, no subdirectories, and no hidden files. The accompanying source-metadata file `sources/opa.ultraplan-source.yml:2` lists the upstream URL `https://github.com/open-policy-agent/opa`, but per the source isolation rules in this prompt only the contents of `studies/agent-harness-study/sources/opa/` may be inspected, and that directory holds zero artifacts.

Search boundary (verified at analysis time):
- `ls -la studies/agent-harness-study/sources/opa/` → only `.` and `..` entries.
- `find studies/agent-harness-study/sources/opa -type f` → no results.
- `find studies/agent-harness-study/sources/opa -type d` → only the root directory itself.

With no implementation, schema, test, config, or doc inside the selected source, none of the dimension artifacts (event type definitions, ordering fields, version fields, parent-ID linkage, lifecycle event types) can be observed. The full lifecycle of any run cannot be reconstructed from events alone — there are no events to reconstruct from.

## Rating

**1 / 10 — Absent.**

Rationale: The dimension rubric requires evidence of typed, ordered, timestamped, versioned events that link to parent runtime objects (run/span/tool-call IDs) and cover creation, completion, failure, and cancellation. None of those artifacts exist inside the selected source directory. With nothing to inspect, no run lifecycle is reconstructable from events alone, and there is no schema, emitter, or test against which to award partial credit.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`. Where the source contains no files, the citation points at the empty source root.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Source contents | Directory exists but is empty (no files, no subdirectories, no hidden files) | `studies/agent-harness-study/sources/opa/` (directory listing) |
| Event type definitions | No clear evidence found — directory contains zero files | `studies/agent-harness-study/sources/opa/` (empty) |
| Event emitters | No clear evidence found — no source files present | `studies/agent-harness-study/sources/opa/` (empty) |
| Sequence numbers | No clear evidence found — no source files present | `studies/agent-harness-study/sources/opa/` (empty) |
| Event version fields | No clear evidence found — no source files present | `studies/agent-harness-study/sources/opa/` (empty) |
| Parent-ID linkage (run / span / tool call) | No clear evidence found — no source files present | `studies/agent-harness-study/sources/opa/` (empty) |
| Lifecycle event types (create / complete / fail / cancel) | No clear evidence found — no source files present | `studies/agent-harness-study/sources/opa/` (empty) |
| Tests covering event behavior | No clear evidence found — no test files present | `studies/agent-harness-study/sources/opa/` (empty) |
| Documentation of event model | No clear evidence found — no docs present inside the source directory | `studies/agent-harness-study/sources/opa/` (empty) |

## Answers to Dimension Questions

1. **Are events typed and versioned?** No clear evidence found. The source directory contains no event type definitions, schemas, or version fields. The metadata at `sources/opa.ultraplan-source.yml:1-2` declares this source as `opa` referencing `https://github.com/open-policy-agent/opa`, but that file is source metadata, not implementation, and is outside the selectable code surface for this dimension.
2. **Are events ordered and timestamped?** No clear evidence found. No sequence-number or timestamp fields exist in any file because no files exist in the selected source directory.
3. **Do events carry sufficient context?** No clear evidence found. There are no run/span/tool-call ID fields, payloads, or correlation keys to evaluate.
4. **Are lifecycle events comprehensive?** No clear evidence found. There are no creation, completion, failure, or cancellation event definitions in the selected source directory.
5. **Can you reconstruct the full lifecycle of any run from events alone?** No. With zero artifacts in the selected source, no lifecycle is reconstructable from this source.

## Architectural Decisions

No clear evidence found. No implementation files are present in `studies/agent-harness-study/sources/opa/` from which to infer architectural decisions about event modeling. The only adjacent artifact, `sources/opa.ultraplan-source.yml:2`, lists the upstream URL, but per source isolation rules that external repository cannot be inspected for this study.

## Notable Patterns

No clear evidence found. No code, schemas, or tests exist inside the selected source directory.

## Tradeoffs

No clear evidence found. Tradeoffs (e.g., typed-vs-untyped, push-vs-pull, single-writer-vs-multiple-emitters) cannot be assessed when no implementation is available in the selected source directory.

## Failure Modes / Edge Cases

No clear evidence found. Failure modes of the event schema (dropped events, replay correctness, ordering under partition, schema migration, clock skew, idempotency) cannot be characterized from an empty source directory.

## Future Considerations

- Provision the opa source material under `studies/agent-harness-study/sources/opa/` (clone, vendor, or snapshot the upstream `open-policy-agent/opa` repository referenced at `sources/opa.ultraplan-source.yml:2`) so this dimension can be re-evaluated against real evidence.
- Once populated, re-run Dimension 10.02 against the vendored layout. Candidate starting points in the upstream OPA codebase to verify locally would be the decision-log/audit pipeline (e.g., decision log types, HTTP handlers, plugin audit-event interfaces) — but no upstream file paths are claimed here, and any concrete paths must be confirmed against the vendored checkout before citing.

## Questions / Gaps

- Why is `studies/agent-harness-study/sources/opa/` empty when the metadata at `sources/opa.ultraplan-source.yml:1` declares `opa` as an active source applicable to dimension `10.02`?
- Is the source expected to be fetched on demand by the analysis runner, or was the checkout/snapshot step skipped?
- Without local artifacts, no claim about OPA's actual event schema can be made in this report. The study for this source on this dimension must be marked incomplete until material is provided under the selected source directory.

---

Generated by `10.02-event-schema-and-lifecycle-events` against `opa`.