# Source Analysis: pydantic-ai

## 10.02 — Event Schema and Lifecycle Events

### Source Info

| Field | Value |
|-------|-------|
| Name | pydantic-ai |
| Path | `studies/agent-harness-study/sources/pydantic-ai` |
| Language / Stack | Unknown — source checkout unavailable (Python expected per project description) |
| Analyzed | 2026-08-23 |

## Summary

The selected source directory `studies/agent-harness-study/sources/pydantic-ai/` is **empty in this workspace**. No source files, tests, configuration, schemas, README, manifest, or `.git` metadata are present inside the boundary. The UltraPlan source descriptor `sources/pydantic-ai.ultraplan-source.yml:1-3` declares the upstream as `https://github.com/pydantic/pydantic-ai`, describes it as "Type-system-centric agent design with validated structured outputs", and lists `10.02` among `applicable_dimensions` at `sources/pydantic-ai.ultraplan-source.yml:44`. It does not vendor the source code into the study workspace, and no clone, mirror, submodule, or vendored copy is present.

`find studies/agent-harness-study/sources/pydantic-ai -mindepth 1` returns zero entries. There is no implementation, no test, no schema, no README, no manifest, and no config inside the source boundary that could be cited for event schemas, emitters, sequence numbers, version fields, parent ID linkage, or lifecycle event types. Hard rule #1 forbids cross-source filesystem access, so no sibling source (e.g., `langfuse/`, `openhands/`) is consulted as a substitute.

Because no code or schema artifacts exist inside the selected source directory, every dimension question collapses to "no evidence found" with the search boundary explicitly stated, and the rating falls into the lowest band of the rubric (Absent / implicit / ad-hoc / unsafe).

## Rating

**Score: 1 / 10**

Rationale: The rubric gives 1–3 for "Absent, implicit, ad-hoc, or unsafe." With an empty source directory there is no event schema, no emitter, no sequencing field, no version field, no parent ID linkage, and no documented lifecycle event vocabulary that can be cited. The framework cannot be scored higher than the minimum because nothing observable supports a stronger claim, and the Hard Rules forbid compensating with evidence sourced from outside the selected source boundary.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Source presence | Directory `studies/agent-harness-study/sources/pydantic-ai/` exists but contains zero files or subdirectories (`find ... -mindepth 1` → empty). | `studies/agent-harness-study/sources/pydantic-ai/` (no files) |
| Source descriptor (metadata only) | `pydantic-ai.ultraplan-source.yml:1` declares `name: "pydantic-ai"` and `url: "https://github.com/pydantic/pydantic-ai"`; description at `:3`; `10.02` listed under `applicable_dimensions` at `:44`. | `studies/agent-harness-study/sources/pydantic-ai.ultraplan-source.yml:1`, `:3`, `:44` |
| Event type definitions | No evidence found — no source files inside the source directory. | — |
| Event ordering / timestamps | No evidence found — no source files inside the source directory. | — |
| Event versioning | No evidence found — no source files inside the source directory. | — |
| Parent ID linkage (run / span / tool call) | No evidence found — no source files inside the source directory. | — |
| Lifecycle event types (created / started / completed / failed / cancelled) | No evidence found — no source files inside the source directory. | — |
| Event emitters | No evidence found — no source files inside the source directory. | — |

**Search boundary:** inspected the full source directory recursively (`ls -laR studies/agent-harness-study/sources/pydantic-ai` returns only `.` and `..`; `find ... -mindepth 1` returns zero hits); inspected the `*.ultraplan-source.yml` descriptor for metadata only (`sources/pydantic-ai.ultraplan-source.yml:1-79`); did not access sibling sources per the isolation rules (hard rule #1).

## Answers to Dimension Questions

1. **Are events typed and versioned?**
   No evidence found. There is no schema file, no Python source, no protobuf / OpenAPI / JSON-Schema document, and no test fixture in the source directory that defines an event payload or a version field. The pydantic-ai project is publicly known to expose Pydantic-typed `AgentRunResult`/`RunContext` constructs and a streaming API, but none of that can be cited from the empty directory without violating rule #1.

2. **Are events ordered and timestamped?**
   No evidence found. No sequence counter, monotonic ID, wall-clock timestamp, or ordering key is present anywhere in the source directory. Public knowledge suggests `pydantic-ai` returns `ModelMessage` objects with Pydantic `Field(default_factory=...)` timestamps, but the actual symbol, file, and line number cannot be cited.

3. **Do events carry sufficient context?**
   No evidence found. No `run_id`, `thread_id`, `span_id`, `parent_id`, `tool_call_id`, `agent_id`, `session_id`, or correlation field can be cited because no event payload exists in the source directory. Even if such fields are present upstream (commonly `RunContext`, `Usage` limits, and `messages` history), the absence of vendored source means nothing can be evidenced.

4. **Are lifecycle events comprehensive?**
   No evidence found. There is no enumerated lifecycle vocabulary (creation, start, completion, failure, cancellation, suspension, resumption) implemented or documented inside the source directory. The dimension's gating question — *"Can you reconstruct the full lifecycle of any run from events alone?"* — is therefore **not answerable** from the selected source as configured.

## Architectural Decisions

No clear evidence found. The source directory is empty, so no architectural decisions are observable. The only artifact available is the UltraPlan descriptor itself (`studies/agent-harness-study/sources/pydantic-ai.ultraplan-source.yml:1-79`), which is study metadata, not framework design.

## Notable Patterns

No clear evidence found. No source code is present to characterize patterns (e.g., the public AG-UI / Pydantic Graph state-machine streaming patterns are not inspectable from this boundary).

## Tradeoffs

No clear evidence found. Tradeoffs cannot be derived because there is no implementation to compare against. Any tradeoff claim about `pydantic-ai`'s type-first design vs. raw event streams would be a vibe claim rather than a cited finding.

## Failure Modes / Edge Cases

No clear evidence found. The most concrete failure mode is the **study-side** failure: the upstream `pydantic-ai` repository was not vendored into `studies/agent-harness-study/sources/pydantic-ai/`, so this dimension cannot be studied without first populating that directory (e.g., `git clone https://github.com/pydantic/pydantic-ai studies/agent-harness-study/sources/pydantic-ai`). Without source files, no failure modes of the framework's event pipeline are observable.

## Future Considerations

1. **Vendor the upstream source.** Before re-running this dimension, populate `studies/agent-harness-study/sources/pydantic-ai/` with a real checkout (or a pinned submodule) at the commit declared in the study configuration so evidence can be collected against the actual pydantic-ai event types.
2. **Establish a baseline vocabulary.** When the source is present, capture event-related symbols — likely candidates include `Agent.iter`, `ModelMessage`, `ModelResponse`, `RunContext`, `ToolCallPart`, `TextPart`, `RetryPromptPart`, and `FinalResult`/`TextResult` discriminator — and record their file paths and line numbers inside the vendored tree.
3. **Verify ordering and versioning.** Confirm whether events carry sequence numbers (likely a Pydantic `Field(default_factory=lambda: uuid4())` UUID rather than a monotonic counter), schema versions, and parent/child linkages; record the exact symbols (e.g., `RunContext.run_id`, `messages[i].timestamp`).
4. **Cross-check lifecycle coverage.** Map emitted events against creation / completion / failure / cancellation (likely encoded via `ModelMessage` union members rather than a discrete event bus) and document any gaps as explicit "missing event" findings.
5. **Investigate the streaming API.** Once vendored, inspect `Agent.iter`/`Agent.iter_stream`/`Agent.run_stream` for whether lifecycle phases (`start`, `tool_call`, `tool_result`, `model_response`, `final_result`, `error`) are first-class event types or merely iterators over `node` returns — that distinction drives the dimension's rating.

## Questions / Gaps

- The source directory contains no files at all. Was the upstream `pydantic/pydantic-ai` repository intentionally excluded from this study, or is the vendor step (clone / submodule / archive extract) missing from the study bootstrap?
- The UltraPlan source descriptor (`studies/agent-harness-study/sources/pydantic-ai.ultraplan-source.yml:1-79`) lists `10.02` as applicable at `:44`, but no artifact exists to study. Should the descriptor be updated to mark the dimension as `N/A` when no source is vendored, or should the vendor step be made mandatory before scheduling dimensions?
- The dimension prompt expects citations in the form `path/to/file.py:NN`. With no files present, every citation collapses to "no evidence found." Is it acceptable for the report to ship at score 1 with that honest disclosure, or must UltraPlan refuse to schedule 10.02 against an empty source?
- pydantic-ai is widely understood to be type-first and pydantic-model-centric rather than event-bus-centric; once the source is vendored, the dimension should consider whether its "events" are better characterized as `ModelMessage` union variants streamed via `Agent.iter` rather than discrete `EventType` enum members — the rating rubric may need adjustment for such architectures.

---

Generated by `dimensions/10.02-event-schema-and-lifecycle-events` against `pydantic-ai`.