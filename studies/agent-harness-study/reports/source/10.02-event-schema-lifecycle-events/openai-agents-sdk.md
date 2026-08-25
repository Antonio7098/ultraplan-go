# Source Analysis: openai-agents-sdk

## Dimension 10.02: Event Schema and Lifecycle Events

### Source Info

| Field | Value |
|-------|-------|
| Name | openai-agents-sdk |
| Path | `studies/agent-harness-study/sources/openai-agents-sdk` |
| Language / Stack | Unknown — source directory is empty |
| Analyzed | 2026-08-23 |

## Summary

The selected source directory `studies/agent-harness-study/sources/openai-agents-sdk/` is empty. The companion metadata file at `studies/agent-harness-study/sources/openai-agents-sdk.ultraplan-source.yml:1` identifies the intended upstream as `https://github.com/openai/openai-agents-python` (a Python project), but no checkout, snapshot, vendored copy, or partial extraction is present locally. As a result, there is no implementation, no test suite, no configuration, no documentation, and no schema files inside the source boundary to inspect.

The search boundary was the directory itself: `ls -la studies/agent-harness-study/sources/openai-agents-sdk/` returns only `.` and `..`, and `find studies/agent-harness-study/sources/openai-agents-sdk -type f` returns zero results. No Python, Markdown, TOML, YAML, or JSON artifacts were discovered. Because the source isolation rules forbid inspecting other workspace sources to compensate, this study cannot answer the dimension questions from first-party evidence.

## Rating

**Score: 1 / 10**

Rationale: the dimension requires evidence of event type definitions, ordering/timestamping, versioning strategy, parent ID linkage, and lifecycle event coverage. With zero files in the source directory, none of these can be observed. The score of 1 (rather than 0) reflects that the source *is* declared and registered against this dimension in the metadata (`openai-agents-sdk.ultraplan-source.yml`), so the *intent* to evaluate it exists — but no material is available to evaluate.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Source directory contents | Directory listing shows only `.` and `..`; no files present | `studies/agent-harness-study/sources/openai-agents-sdk/.`:1 |
| Recursive file search | `find` returns no files under the source root | `studies/agent-harness-study/sources/openai-agents-sdk/`:1 |
| Source metadata declaring intent | Source registered against dimension 10.02, upstream URL points to `openai/openai-agents-python` | `studies/agent-harness-study/sources/openai-agents-sdk.ultraplan-source.yml:1-7` |
| Event schemas | No evidence found | n/a |
| Event emitters | No evidence found | n/a |
| Sequence numbers | No evidence found | n/a |
| Event version fields | No evidence found | n/a |
| Lifecycle event types | No evidence found | n/a |
| Tests covering lifecycle events | No evidence found | n/a |

## Answers to Dimension Questions

1. **Are events typed and versioned?** — No evidence found. No event schema, TypedDict, dataclass, Pydantic model, or enum is present in the source directory. Per the upstream URL metadata, `openai-agents-python` does ship a tracing/events module, but this study cannot verify it from local files.
2. **Are events ordered and timestamped?** — No evidence found. No sequence-number field, monotonic counter, or timestamp field can be inspected locally.
3. **Do events carry sufficient context?** — No evidence found. No `run_id`, `span_id`, `tool_call_id`, parent reference, or correlation ID can be cited from source.
4. **Are lifecycle events comprehensive?** — No evidence found. Cannot confirm presence of creation/completion/failure/cancellation lifecycle hooks without local artifacts.

The dimension's signature question — "Can you reconstruct the full lifecycle of any run from events alone?" — cannot be answered from this source in its current state.

## Architectural Decisions

No clear evidence found. Search boundary: the entire contents of `studies/agent-harness-study/sources/openai-agents-sdk/` (zero files). Per source isolation rules, no peer source directory was inspected to infer design intent.

## Notable Patterns

No clear evidence found. Search boundary as above.

## Tradeoffs

No clear evidence found. Without a populated source tree, it is impossible to observe whether the design favors (a) wire-format stability vs. in-process convenience, (b) strict Pydantic schemas vs. duck-typed dicts, or (c) explicit lifecycle hooks vs. inferred state transitions. The upstream repository's public docs (out of scope here) describe a tracing subsystem built on OpenAI's trace spans, which would likely impose some structure — but that is external knowledge, not local evidence.

## Failure Modes / Edge Cases

No clear evidence found. A common failure mode for an empty source directory in this study pipeline is a missed `git clone` or sparse-checkout step upstream of the analysis run. If the source were populated, candidate failure modes to investigate would include: dropped events on emitter exceptions, version-skew between span producers and consumers, missing cancellation events when an agent loop is interrupted mid-tool-call, and reconstruction gaps when parent IDs are absent on leaf spans.

## Future Considerations

- **Populate the source.** Run the fetch step (e.g., `git clone https://github.com/openai/openai-agents-python studies/agent-harness-study/sources/openai-agents-sdk`) and re-execute this dimension. The upstream is public and the dimension is registered, so the gap is operational, not analytical.
- **Pin a commit.** Once populated, record the commit SHA in the source metadata so this report can be regenerated deterministically and compared across versions.
- **Prioritize the tracing module.** Dimension 10.02 maps most directly to `src/agents/tracing/` (likely path; unverified) — that is the first directory to inspect after population.

## Questions / Gaps

1. Why is `studies/agent-harness-study/sources/openai-agents-sdk/` empty despite the metadata declaring it as a dimension target? Was the fetch step skipped, failed, or filtered out?
2. Is there a sibling artifact (e.g., a `.git` directory, archive, or lockfile) outside the source directory that should be merged in? Per isolation rules, this study did not search for one.
3. Should the pipeline produce an explicit "source unavailable" marker rather than scoring 1/10 on missing evidence, to distinguish "low-quality source" from "missing source"?

---

Generated by `10.02-event-schema-and-lifecycle-events` against `openai-agents-sdk`.
