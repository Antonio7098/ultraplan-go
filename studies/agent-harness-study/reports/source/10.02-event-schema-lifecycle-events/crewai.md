# Source Analysis: crewai

## 10.02 — Event Schema and Lifecycle Events

### Source Info

| Field | Value |
|-------|-------|
| Name | crewai |
| Path | `studies/agent-harness-study/sources/crewai` |
| Language / Stack | Unknown (source checkout unavailable) |
| Analyzed | 2026-08-23 |

## Summary

The selected source directory `studies/agent-harness-study/sources/crewai/` is **empty in the local checkout**. No files, no subdirectories, and no `.git` metadata are present. The UltraPlan source descriptor (`sources/crewai.ultraplan-source.yml`) only names the upstream URL (`https://github.com/crewAIInc/crewAI`) and the set of applicable dimensions; it does not vendor the code into this study workspace. As a result, the analysis is bounded to what can be observed about the artifact set inside the isolated source directory, which is nothing concrete.

`find studies/agent-harness-study/sources/crewai/ -mindepth 1` returns zero entries. There is no implementation, no test, no schema, no README, no manifest, and no config inside the source boundary that could be cited for event schemas, emitters, sequence numbers, version fields, or lifecycle event types. Per the Hard Rules, cross-source filesystem access is banned, so no sibling source (e.g., `langfuse/`, `openhands/`) is inspected as a substitute.

Because no code or schema artifacts exist inside the source directory, every dimension question collapses to "no evidence found" and the rating falls into the lowest band of the rubric (Absent / implicit / ad-hoc / unsafe).

## Rating

**Score: 1 / 10**

Rationale: The rubric gives 1–3 for "Absent, implicit, ad-hoc, or unsafe." With an empty source directory there is no event schema, no emitter, no sequencing field, no version field, no parent ID linkage, and no documented lifecycle event vocabulary. The framework cannot be scored higher than the minimum because nothing observable supports a stronger claim, and the Hard Rules forbid compensating with evidence from outside the source.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Source presence | Directory `studies/agent-harness-study/sources/crewai/` exists but contains zero files or subdirectories (`find ... -mindepth 1` → empty). | `studies/agent-harness-study/sources/crewai/` (no files) |
| Source descriptor | `crewai.ultraplan-source.yml:1` declares `name: "crewai"` and `url: "https://github.com/crewAIInc/crewAI"`; lists `10.02` among `applicable_dimensions` (`:50`). | `studies/agent-harness-study/sources/crewai.ultraplan-source.yml:1` and `:50` |
| Event type definitions | No evidence found — no source files inside the source directory. | — |
| Event ordering / timestamps | No evidence found — no source files inside the source directory. | — |
| Event versioning | No evidence found — no source files inside the source directory. | — |
| Parent ID linkage (run / span / tool call) | No evidence found — no source files inside the source directory. | — |
| Lifecycle event types (created / completed / failed / cancelled) | No evidence found — no source files inside the source directory. | — |
| Event emitters | No evidence found — no source files inside the source directory. | — |

**Search boundary:** inspected the full source directory recursively with `find … -mindepth 1`; inspected the `*.ultraplan-source.yml` descriptor for metadata only; did not access sibling sources per the isolation rules.

## Answers to Dimension Questions

1. **Are events typed and versioned?**
   No evidence found. There is no schema file, no Python source, no protobuf / OpenAPI / JSON-Schema document, and no test fixture in the source directory that defines an event payload or a version field.

2. **Are events ordered and timestamped?**
   No evidence found. No sequence counter, monotonic ID, wall-clock timestamp, or ordering key is present anywhere in the source directory.

3. **Do events carry sufficient context?**
   No evidence found. No `run_id`, `thread_id`, `span_id`, `parent_id`, `tool_call_id`, `agent_id`, `session_id`, or correlation field can be cited because no event payload exists in the source directory.

4. **Are lifecycle events comprehensive?**
   No evidence found. There is no enumerated lifecycle vocabulary (creation, start, completion, failure, cancellation, suspension, resumption) implemented or documented inside the source directory. The dimension's gating question — *"Can you reconstruct the full lifecycle of any run from events alone?"* — is therefore **not answerable** from the selected source.

## Architectural Decisions

No clear evidence found. The source directory is empty, so no architectural decisions are observable. The only artifact available is the UltraPlan descriptor itself (`studies/agent-harness-study/sources/crewai.ultraplan-source.yml:1-87`), which is study metadata, not framework design.

## Notable Patterns

No clear evidence found.

## Tradeoffs

No clear evidence found. Tradeoffs cannot be derived because there is no implementation to compare against.

## Failure Modes / Edge Cases

No clear evidence found. The most concrete failure mode is the study-side failure: the upstream repository was not vendored into `studies/agent-harness-study/sources/crewai/`, so this dimension cannot be studied without first populating that directory (e.g., `git clone https://github.com/crewAIInc/crewAI studies/agent-harness-study/sources/crewai`). Without source files, no failure modes of the framework's event pipeline are observable.

## Future Considerations

1. **Vendor the upstream source.** Before re-running this dimension, populate `studies/agent-harness-study/sources/crewai/` with a real checkout (or a pinned submodule) so evidence can be collected against the actual CrewAI event types.
2. **Establish a baseline vocabulary.** When the source is present, capture event type names (`CrewKickoffStartedEvent`, `CrewKickoffCompletedEvent`, `CrewKickoffFailedEvent`, `AgentExecutionStartedEvent`, `ToolUsageStartedEvent`, etc.) from whichever Python package the framework ships, and record their schema files with line numbers.
3. **Verify ordering and versioning.** Confirm whether events carry sequence numbers, monotonic IDs, schema versions, and parent/child linkages; record the exact symbols (e.g., `CrewEvent.seq`, `CrewEvent.schema_version`).
4. **Cross-check lifecycle coverage.** Map emitted events against creation / completion / failure / cancellation and document any gaps as explicit "missing event" findings.

## Questions / Gaps

- The source directory contains no files at all. Is the upstream `crewAIInc/crewAI` repository intentionally excluded from this study, or is the vendor step (clone / submodule / archive extract) missing from the study bootstrap?
- The UltraPlan source descriptor (`studies/agent-harness-study/sources/crewai.ultraplan-source.yml:1-87`) lists `10.02` as applicable, but no artifact exists to study. Should the descriptor be updated to mark the dimension as `N/A` when no source is vendored, or should the vendor step be made mandatory before scheduling dimensions?
- The dimension prompt expects citations in the form `path/to/file.ts:NN`. With no files present, every citation collapses to "no evidence found." Is it acceptable for the report to ship at score 1 with that honest disclosure, or must UltraPlan refuse to schedule 10.02 against an empty source?
- CrewAI is widely described as a multi-agent orchestration framework with role-based delegation. Once vendored, the analyst should specifically inspect `crewai/crew.py`, `crewai/agent.py`, and any `crewai/events/` or `crewai/utilities/events.py` modules for typed event emitters and lifecycle coverage; that inspection is impossible today.

---

Generated by `dimensions/10.02-event-schema-and-lifecycle-events` against `crewai`.
