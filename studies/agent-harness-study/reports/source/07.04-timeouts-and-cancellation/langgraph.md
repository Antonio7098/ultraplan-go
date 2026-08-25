# Source Analysis: langgraph

## Timeouts and Cancellation

### Source Info

| Field | Value |
|-------|-------|
| Name | langgraph |
| Path | `studies/agent-harness-study/sources/langgraph` |
| Language / Stack | Unknown — source directory is empty; manifest declares a Python monorepo (`langchain-ai/langgraph`), but no source files are present locally |
| Analyzed | 2026-08-23 |

## Summary

No code-level evidence could be gathered for this dimension. The selected source directory `studies/agent-harness-study/sources/langgraph/` is empty: it contains zero files, zero subdirectories, and no VCS metadata (no `.git`, no `pyproject.toml`, no `README.md`, no manifest). The sibling file `sources/langgraph.ultraplan-source.yml` describes the upstream as the GitHub repo `langchain-ai/langgraph` (the Python reference implementation of LangGraph's stateful graph runtime, with documented durable execution, checkpointing, interrupts, and resumable subgraphs), but the workspace does not currently hold a checkout of that repository.

Because no source files are present inside the selected source directory, none of the dimension steps could be executed:

1. **Timeout config** — no files to search.
2. **Per-tool vs global timeouts** — no files to search.
3. **Cancellation tokens / signals** — no files to search.
4. **Cleanup behavior** — no files to search.
5. **Cancelled structured results** — no files to search.
6. **User-initiated cancellation** — no files to search.

Per the dimension Quality Bar, every claim must cite a `path/to/file.ts:NN` evidence entry. No such entries exist for this run, so all dimension questions are answered "No evidence found" with the search boundary explicitly stated. The rating is therefore the minimum of the rubric (1/10): the source offers no analyzable evidence for timeouts or cancellation in its current local state.

## Rating

**1 / 10** — Absent at the local source; the dimension cannot be evaluated because the source directory is empty.

- Searched `studies/agent-harness-study/sources/langgraph/` (recursive): 0 files, 0 subdirectories.
- Searched `studies/agent-harness-study/sources/` siblings: out of scope per Source Isolation Rules.
- Searched the wider filesystem (`find / -type d -name "langgraph"`): only the empty target directory matches; no checkout, no vendored copy, no cache.
- No `pyproject.toml`, no `requirements*.txt`, no `README.md`, no `LICENSE`, no `.git/` inside the source root.
- Rating driven entirely by absence of evidence rather than by a negative finding about an implemented feature.

## Evidence Collected

Every entry includes file path with line number. Format: `path/to/file.py:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Source root | Directory exists but contains no files | `studies/agent-harness-study/sources/langgraph/` (empty) |
| Manifest | Upstream URL & scope declared | `studies/agent-harness-study/sources/langgraph.ultraplan-source.yml:2-3` |
| Manifest | Description: "Best reference for durable execution, checkpoints, interrupts, state graphs" | `studies/agent-harness-study/sources/langgraph.ultraplan-source.yml:3` |
| Manifest | Applicable dimensions list (includes `07.04`) | `studies/agent-harness-study/sources/langgraph.ultraplan-source.yml:55` |
| Timeout wrappers | No evidence found | n/a |
| Cancellation tokens | No evidence found | n/a |
| Abort controllers | No evidence found | n/a |
| Cleanup handlers | No evidence found | n/a |
| Cancelled statuses | No evidence found | n/a |
| Timeout tests | No evidence found | n/a |

## Answers to Dimension Questions

1. **Can a tool hang forever?** — No evidence found. The selected source directory contains no implementation files; the timeout/cancellation surface cannot be observed. Upstream LangGraph is widely known to expose `GraphRecursionError` and recursion-limit guards rather than per-tool wall-clock timeouts, but that is a public-knowledge claim, not a cited source-file finding, so it is recorded as unverified here.
2. **Are timeouts configurable?** — No evidence found. No config files, no `Pregel`/`StateGraph` runtime modules, no tool wrapper nodes, no `RunnableConfig` definitions exist in the source root to inspect.
3. **Can users cancel?** — No evidence found. No CLI entry points, no `interrupt` mechanism code, no `Command` primitives, no signal handlers, no `KeyboardInterrupt` plumbing is accessible in the local copy.
4. **Is cancellation cooperative or forced?** — No evidence found. No `asyncio.CancelledError` handling, no `signal.SIGINT`/`SIGTERM` handlers, no abort-controller wiring, no thread-flag cancel protocol is present locally.
5. **Does cancellation leave resources dirty?** — No evidence found. No `finally` blocks, no `contextlib`/`async with` lifecycle code, no checkpoint close handlers, no resource teardown logic can be examined in this empty checkout.

## Architectural Decisions

No clear evidence found. The source directory is empty; no `Pregel` runtime, no `StateGraph` compilation pipeline, no node/wrapper primitives, no checkpointer interface, and no tool-binding layer is available locally to characterize the architecture for this dimension. Manifest-stated design intent (durable execution, interrupts, state graphs — `studies/agent-harness-study/sources/langgraph.ultraplan-source.yml:3`) cannot be matched to concrete symbols without source files.

## Notable Patterns

No clear evidence found. No files exist in the selected source directory to surface timeout wrappers, cancellation tokens, abort controllers, cleanup handlers, cancelled-status enums, `GraphRecursionError`/`NodeInterrupt`/`GraphInterrupt` types, or timeout/recursion-limit tests.

## Tradeoffs

No clear evidence found. Tradeoff analysis cannot be performed because there is no code, configuration, or documentation present inside `studies/agent-harness-study/sources/langgraph/` to compare against alternatives. Public-knowledge framings (LangGraph trades wall-clock timeouts for step/recursion bounds and durable interrupts via checkpoint resume) are not cited because they cannot be tied to a specific file:line.

## Failure Modes / Edge Cases

No clear evidence found. The dimension's central question — "Can a stuck shell command or API call be stopped without killing the whole run?" — cannot be answered from the local source. Possible failure modes the framework *might* exhibit (a tool node invoking an LLM HTTP call that hangs indefinitely; a subgraph node blocking on a custom callable; a checkpointer awaiting a slow backend write; a long-running `Pregel` step exceeding user patience) are not observable here.

## Future Considerations

- **Re-fetch the source.** `studies/agent-harness-study/sources/langgraph/` needs to be populated with a checkout of `https://github.com/langchain-ai/langgraph` before this dimension can be studied. The manifest already points at the upstream URL (`studies/agent-harness-study/sources/langgraph.ultraplan-source.yml:2`), so a `git clone` into the source directory would unblock this and the remaining empty-source dimensions in the study.
- **Focus areas once materialized.** The dimension is likely richest in the upstream `libs/langgraph/langgraph/pregel/` and `libs/langgraph/langgraph/graph/` trees, where `Pregel` enforces `recursion_limit` / `step_timeout` style step-bounds; node tools would come from `langgraph.prebuilt.ToolNode`; cancellation primitives live in the `interrupt`/`Command` surface. These are upstream-knowledge pointers only and must be re-validated against file:line evidence after materialization.
- **Cross-check sibling reports at evaluation time.** If re-fetch is not possible, the validator should treat this report as a no-evidence gap rather than a negative finding, since absence of local files is not equivalent to absence of upstream behavior.
- **Operational note for study infrastructure.** Multiple sources are currently empty in this study (`crewai`, `langgraph`, `letta`, `opa`, `openai-agents-sdk`, `pydantic-ai`, `temporal`); the study runner should ensure source materialization before dispatching dimension tasks for these repositories.

## Questions / Gaps

- The source directory is empty; what was the intended source location and how is materialization triggered?
- The manifest at `studies/agent-harness-study/sources/langgraph.ultraplan-source.yml:2` references a public GitHub URL — was the upstream intended to be cloned per-task or pre-checked into the workspace?
- Should this dimension be re-run after the source is populated, or is this gap acceptable given the broader empty-source pattern in the study?
- If a single upstream repo is too large to materialize fully, is a curated subset (e.g., `libs/langgraph/langgraph/pregel/` plus the `prebuilt/` tool-node module) an acceptable scoped checkout for this dimension?

---

Generated by `studies/agent-harness-study/dimensions/07.04-timeouts-and-cancellation.md` against `langgraph`.
