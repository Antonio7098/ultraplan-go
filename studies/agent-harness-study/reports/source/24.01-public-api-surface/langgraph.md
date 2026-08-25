# Source Analysis: langgraph

## Public API Surface

### Source Info

| Field | Value |
|-------|-------|
| Name | langgraph |
| Path | `studies/agent-harness-study/sources/langgraph` |
| Language / Stack | unknown (source not materialised) |
| Analyzed | 2026-08-22 |

## Summary

The selected source directory is empty: `studies/agent-harness-study/sources/langgraph/` contains no files. A recursive enumeration (`ls -laR`, glob `**/*`) and the source manifest show zero Python/TypeScript modules, no `pyproject.toml` / `setup.py` / `package.json`, no documentation, no tests, and no exported symbols. Because the dimension definition requires evidence drawn from import paths, client objects, CLI commands, service endpoints, and documented entry points, and the rules prohibit inspecting sibling sources, this study cannot inspect any concrete public API surface for `langgraph`. The score reflects the absence of inspectable evidence rather than a judgment about the upstream `langchain-ai/langgraph` project itself.

Search boundary executed:

- Recursive listing of `studies/agent-harness-study/sources/langgraph/` — no files (`studies/agent-harness-study/sources/langgraph/.`).
- Glob `**/*` against the selected source path — zero matches.
- Source isolation rule (per task prompt) forbids reading other source directories, the dimension inputs/manifest aside, so no substitute evidence is admissible in this study.

## Rating

**Rating: 1 / 10** — Tier: Absent (no inspectable evidence)

**Score:** 1
**Score (out of 10):** 1/10
**Tier:** Absent (rubric band 1-3)

Rationale: the rubric maps scores `1-3` to "Absent, implicit, ad-hoc, or unsafe" public APIs. With zero files in the selected source path there are no stable import paths, no clients, no CLI commands, no service endpoints, no type definitions, no tests, and no documented examples to evaluate. None of the four dimension questions can be answered with code-cited evidence, which itself is the strongest indicator that a public API surface study is not feasible against this materialised source.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Package manifest (`pyproject.toml` / `setup.py`) | No evidence found — no manifest present in the selected source directory. | `studies/agent-harness-study/sources/langgraph/.` (directory empty) |
| Top-level `__init__.py` / package index modules | No evidence found. | `studies/agent-harness-study/sources/langgraph/.` (directory empty) |
| Client objects (`StateGraph`, `Pregel`, `CompiledStateGraph`, etc.) | No evidence found — no Python or TypeScript modules file present. | `studies/agent-harness-study/sources/langgraph/.` (directory empty) |
| CLI command groups (e.g. `langgraph dev`, `langgraph up`) | No evidence found — no CLI entry points or script wrappers. | `studies/agent-harness-study/sources/langgraph/.` (directory empty) |
| HTTP/RPC service routes (e.g. `RemoteGraph` transports) | No evidence found — no route definitions. | `studies/agent-harness-study/sources/langgraph/.` (directory empty) |
| Public type definitions / interfaces (`State`, `Send`, `Command`, `interrupt`, etc.) | No evidence found. | `studies/agent-harness-study/sources/langgraph/.` (directory empty) |
| Documentation pages / runnable examples (`README.md`, `docs/`, `examples/`) | No evidence found. | `studies/agent-harness-study/sources/langgraph/.` (directory empty) |
| Test suites for public API contract (`tests/`) | No evidence found. | `studies/agent-harness-study/sources/langgraph/.` (directory empty) |
| API stability markers (`__all__`, `PUBLIC`, deprecation notes, version pins) | No evidence found. | `studies/agent-harness-study/sources/langgraph/.` (directory empty) |
| Internal/experimental labels (`_internal/`, `experimental/`) | No evidence found. | `studies/agent-harness-study/sources/langgraph/.` (directory empty) |
| Import/export boundaries (e.g. `index.ts`, re-export barrels) | No evidence found. | `studies/agent-harness-study/sources/langgraph/.` (directory empty) |
| Evidence of accidental public surface area (loose star exports, wildcard `__init__` re-exports) | No evidence found. | `studies/agent-harness-study/sources/langgraph/.` (directory empty) |

## Answers to Dimension Questions

1. **What is the intended public API surface?** — No clear evidence found. The intended surface cannot be enumerated from the empty source; per the source manifest at `sources/langgraph.ultraplan-source.yml:2-3`, the upstream project is the `langchain-ai/langgraph` repository, but the materialised snapshot at `studies/agent-harness-study/sources/langgraph/` contains nothing to read.
2. **Is the stable API easy to distinguish from internal implementation details?** — No clear evidence found. There are no `__all__` lists, no `PUBLIC` markers, no deprecation decorators, and no internal/experimental labels present to inspect.
3. **Does the API expose the right level of abstraction for agent harness users?** — No clear evidence found. No state-graph or checkpoint abstractions are introspectable in the selected directory.
4. **Are examples sufficient to use the API correctly without reading internals?** — No clear evidence found. No `examples/`, `docs/`, or README exists inside the selected source directory.

## Architectural Decisions

No clear evidence found. The selected source contains no implementation files, configuration, or documentation; therefore no architectural decisions about import-path grouping, namespace separation, client-object ownership, or lifecycle hooks can be cited.

## Notable Patterns

No clear evidence found. A pattern search (e.g., `__all__`, `PublicApi`, namespace layering, re-export barrels, command-group registration) returned no candidates because the directory contains no files.

## Tradeoffs

No clear evidence found. Tradeoffs only become nameable once stable import paths, clients, and endpoints exist; here the absence of any surface precludes that analysis.

## Failure Modes / Edge Cases

No clear evidence found. Failure modes of accidental exports, shadowed names, or unguarded internal access require at least one public module to study. None is present.

## Future Considerations

- The materialised source snapshot at `studies/agent-harness-study/sources/langgraph/` needs to be populated (e.g., via a fetch of the upstream `langchain-ai/langgraph` repository, see `sources/langgraph.ultraplan-source.yml:2`) before any dimension anchored on code can produce evidence-grade findings.
- Once materialised, a re-run of this dimension should specifically surface:
  - The contents of the top-level package `__init__.py` and any re-export barrels that define the `__all__` for `langgraph` (`StateGraph`, `Pregel`, `CompiledStateGraph`, `RemoteGraph`, `MessageGraph`, `Send`, `Command`, `interrupt`, `get_state`, `update_state`).
  - The CLI entry points registered under the `langgraph-cli` package and the command groups they expose (`dev`, `up`, `build`, `dockerfile`, `deploy`).
  - The `langgraph-sdk` TypeScript client surface and the HTTP routes consumed by `RemoteGraph` (e.g. `/threads`, `/threads/{thread_id}/runs`, `/assistants`).
  - Stability markers such as `PublicAPI`, `_internal` namespaces, or deprecation decorators used to separate stable API from internals.
  - Runnable examples (`examples/`, `docs/docs/` snippets) that demonstrate the stable surface without requiring internals.

## Questions / Gaps

- Why is the `langgraph` source directory empty while sibling sources such as `agent-framework`, `crewai`, `langfuse`, `letta`, `opa`, `openai-agents-sdk`, `pydantic-ai`, and `temporal` (see `sources/`) all contain materialised code? This is the single most important question for the study, because the gap determines whether the dimension is reported as "no evidence" or rewritten once the snapshot is populated.
- Without violating source isolation, there is no admissible way to infer what the upstream `langgraph` public API looks like; downstream re-runs of this dimension must rely on the materialised snapshot rather than on out-of-scope cross-source reads.

---

Generated by `24.01-public-api-surface` against `langgraph`.
