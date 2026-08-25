# Source Analysis: langgraph

## Resource Locking and Isolation

### Source Info

| Field | Value |
|-------|-------|
| Name | langgraph |
| Path | `studies/agent-harness-study/sources/langgraph` |
| Language / Stack | Unknown — source directory is empty; manifest declares a Python monorepo (`langchain-ai/langgraph`), but no source files are present locally |
| Analyzed | 2026-08-23 |

## Summary

No code-level evidence could be gathered for this dimension. The selected source directory `studies/agent-harness-study/sources/langgraph/` is empty: it contains zero files, zero subdirectories, and no VCS metadata (no `.git`, no `pyproject.toml`, no `README.md`, no manifest of its own). The sibling file `sources/langgraph.ultraplan-source.yml` describes the upstream as the GitHub repo `langchain-ai/langgraph` (`studies/agent-harness-study/sources/langgraph.ultraplan-source.yml:1-2`) — the Python reference implementation of LangGraph's stateful graph runtime — and explicitly declares this dimension in scope (`studies/agent-harness-study/sources/langgraph.ultraplan-source.yml:56`, within the `applicable_dimensions` list starting at `studies/agent-harness-study/sources/langgraph.ultraplan-source.yml:4`). The workspace, however, does not currently hold a checkout of that repository, so nothing about its resource-locking or isolation behavior can be inspected locally.

Because no source files are present inside the selected source directory, none of the dimension steps could be executed:

1. **Identify resources (files, shell, browser, database, network, workspace, secrets)** — no files to search.
2. **Inspect lock managers** — no files to search.
3. **Check lock granularity** — no files to search.
4. **Check deadlock prevention** — no files to search.
5. **Inspect sandbox boundaries** — no files to search.

Per the dimension Quality Bar, every claim must cite a `path/to/file.py:NN` evidence entry. No such entries exist inside the source root for this run, so all dimension questions are answered "No evidence found" with the search boundary explicitly stated. The dimension's guiding question — *can two tools edit the same file safely?* — is unanswerable from this checkout: there is not a single tool implementation present. The rating is therefore the minimum of the rubric (1/10): the source offers no analyzable evidence for resource locking or isolation in its current local state.

## Rating

**1 / 10** — Absent at the local source; the dimension cannot be evaluated because the source directory is empty.

- Searched `studies/agent-harness-study/sources/langgraph/` (recursive): 0 files, 0 subdirectories.
- Searched `studies/agent-harness-study/sources/` siblings: out of scope per Source Isolation Rules.
- No `.git/`, no `pyproject.toml`, no `requirements*.txt`, no `README.md`, no `LICENSE`, no lock-manager or sandbox modules inside the source root.
- Rating driven entirely by absence of local evidence rather than by a negative finding about an implemented upstream feature.

## Evidence Collected

Every entry includes file path with line number. Format: `path/to/file.py:NN`. Only two artifacts exist at or above the selected source boundary, so only they can be cited:

| Area | Evidence | File:Line |
|------|----------|-----------|
| Source root | Directory exists but contains no files | `studies/agent-harness-study/sources/langgraph/` (empty) |
| Manifest | Upstream identity: `name: "langgraph"`, `url: https://github.com/langchain-ai/langgraph` | `studies/agent-harness-study/sources/langgraph.ultraplan-source.yml:1-2` |
| Manifest | Description: "Best reference for durable execution, checkpoints, interrupts, state graphs" | `studies/agent-harness-study/sources/langgraph.ultraplan-source.yml:3` |
| Manifest | Dimension declared in scope for this source | `studies/agent-harness-study/sources/langgraph.ultraplan-source.yml:56` |
| Resource lock manager | No evidence found | n/a |
| Workspace locks | No evidence found | n/a |
| File locks | No evidence found | n/a |
| Database transactions | No evidence found | n/a |
| Sandbox config | No evidence found | n/a |
| Deadlock handling | No evidence found | n/a |

## Answers to Dimension Questions

1. **Which resources are shared?** — No evidence found. The selected source directory contains no implementation files; the resource surface (files, shell, browser, database, network, workspace, secrets) cannot be observed. Any statement about upstream LangGraph's shared state channels or checkpointer backends would be public knowledge, not a cited source-file finding, and is recorded as unverified here.
2. **What protects them?** — No evidence found. No lock managers, mutex/stripe implementations, per-thread/per-loop primitives, or ownership contracts exist locally to inspect.
3. **Are locks coarse or fine-grained?** — No evidence found. Lock granularity cannot be characterized without any synchronization code in the checkout.
4. **Can deadlocks occur?** — No evidence found. No lock-ordering discipline, acquisition-timeout code, single-owner-task marshalling, or deadlock-related comments/tests are present locally.
5. **Are resource conflicts visible?** — No evidence found. No logging/metrics around contention, no conflict-detection code, and no tests exist in the source root to examine.

## Architectural Decisions

No clear evidence found. The source directory is empty; no Pregel runtime, no StateGraph compilation pipeline, no channel/reducer primitives, no checkpointer interfaces, and no tool-binding layer are available locally from which isolation architecture could be characterized. Manifest-stated design intent ("durable execution, checkpoints, interrupts, state graphs", `studies/agent-harness-study/sources/langgraph.ultraplan-source.yml:3`) cannot be matched to concrete symbols without source files.

## Notable Patterns

No clear evidence found. No files exist in the selected source directory to surface lock stripes, per-key locks, sandbox configurations, transaction wrappers, or concurrency tests for this dimension.

## Tradeoffs

No clear evidence found. Tradeoff analysis cannot be performed because there is no code, configuration, or documentation present inside `studies/agent-harness-study/sources/langgraph/` to compare against alternatives. Public-knowledge framings (e.g., that LangGraph centralizes mutable state in channels guarded by a single-writer superstep model rather than per-resource locking) are deliberately not cited because they cannot be tied to a specific file:line.

## Failure Modes / Edge Cases

No clear evidence found. The dimension's central question — *can two tools edit the same file safely?* — cannot be answered from the local source: there are no tool implementations, no file-write paths, and no lock managers to reason about. Potential failure modes the framework *might* exhibit (concurrent node writes racing on shared state keys; parallel supersteps contending on a checkpointer backend; unsynchronized side effects from tool nodes touching external resources) are not observable here and remain unverified hypotheses.

## Future Considerations

- **Re-fetch the source.** `studies/agent-harness-study/sources/langgraph/` needs to be populated with a checkout of `https://github.com/langchain-ai/langgraph` before this dimension can be studied. The manifest already points at the upstream URL (`studies/agent-harness-study/sources/langgraph.ultraplan-source.yml:2`) and confirms scope (`studies/agent-harness-study/sources/langgraph.ultraplan-source.yml:56`), so materializing the checkout would unblock this and the remaining empty-source dimensions.
- **Focus areas once materialized.** Upstream, resource-isolation behavior would most plausibly live in the checkpoint/persistence layer (backend transactionality for Postgres/SQLite savers), the channel write-path arbitration inside `libs/langgraph/langgraph/pregel/`, and any executor/tool-node concurrency controls. These are upstream-knowledge pointers only and must be re-validated against file:line evidence after materialization.
- **Cross-check sibling reports at evaluation time.** If re-fetch is not possible, the validator should treat this report as a no-evidence gap rather than a negative finding, since absence of local files is not equivalent to absence of upstream behavior.
- **Operational note for study infrastructure.** Multiple sources are currently empty in this study (`crewai`, `langgraph`, `letta`, `opa`, `openai-agents-sdk`, `pydantic-ai`, `temporal`); the study runner should ensure source materialization before dispatching dimension tasks for these repositories.

## Questions / Gaps

- The source directory is empty; what was the intended source location and how is materialization triggered?
- The manifest at `studies/agent-harness-study/sources/langgraph.ultraplan-source.yml:2` references a public GitHub URL — was the upstream intended to be cloned per-task or pre-checked into the workspace?
- Should this dimension be re-run after the source is populated, or is this gap acceptable given the broader empty-source pattern in the study?
- If a single upstream repo is too large to materialize fully, is a curated subset (e.g., the pregel loop plus the checkpointer backends) an acceptable scoped checkout for this dimension?

---

Generated by `studies/agent-harness-study/dimensions/07.05-resource-locking-and-isolation.md` against `langgraph`.
