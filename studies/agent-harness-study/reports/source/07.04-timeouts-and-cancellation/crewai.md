# Source Analysis: crewai

## Timeouts and Cancellation

### Source Info

| Field | Value |
|-------|-------|
| Name | crewai |
| Path | `studies/agent-harness-study/sources/crewai` |
| Language / Stack | Unknown — source directory is empty; manifest declares a Python monorepo (`crewAIInc/crewAI`), but no source files are present locally |
| Analyzed | 2026-08-23 |

## Summary

No code-level evidence could be gathered for this dimension. The selected source directory `studies/agent-harness-study/sources/crewai/` is empty: it contains zero files, zero subdirectories, and no VCS metadata (no `.git`, no `pyproject.toml`, no `README.md`, no manifest). The sibling file `sources/crewai.ultraplan-source.yml` describes the upstream as the GitHub repo `crewAIInc/crewAI` (a Python monorepo covering `crewai`, `crewai-core`, `crewai-tools`, `crewai-files`), but the workspace does not currently hold a checkout of that repository.

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

- Searched `studies/agent-harness-study/sources/crewai/` (recursive): 0 files, 0 subdirectories.
- Searched `studies/agent-harness-study/sources/` siblings: out of scope per Source Isolation Rules.
- Searched the wider filesystem (`find / -name "crewai" -type d`): only the empty target directory matches; no checkout, no vendored copy, no cache.
- No `pyproject.toml`, no `requirements*.txt`, no `README.md`, no `LICENSE`, no `.git/` inside the source root.
- Rating driven entirely by absence of evidence rather than by a negative finding about an implemented feature.

## Evidence Collected

Every entry includes file path with line number. Format: `path/to/file.py:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Source root | Directory exists but contains no files | `studies/agent-harness-study/sources/crewai/` (empty) |
| Manifest | Upstream URL & scope declared | `studies/agent-harness-study/sources/crewai.ultraplan-source.yml:2` |
| Manifest | Applicable dimensions list (includes `07.04`) | `studies/agent-harness-study/sources/crewai.ultraplan-source.yml:45` |
| Timeout wrappers | No evidence found | n/a |
| Cancellation tokens | No evidence found | n/a |
| Abort controllers | No evidence found | n/a |
| Cleanup handlers | No evidence found | n/a |
| Cancelled statuses | No evidence found | n/a |
| Timeout tests | No evidence found | n/a |

## Answers to Dimension Questions

1. **Can a tool hang forever?** — No evidence found. The selected source directory contains no implementation files; the timeout/cancellation surface cannot be observed.
2. **Are timeouts configurable?** — No evidence found. No config files, no LLM/tool wrapper modules, no schema files exist in the source root to inspect.
3. **Can users cancel?** — No evidence found. No CLI entry points, no Flow runtime, no signal handlers, no `KeyboardInterrupt` plumbing is accessible in the local copy.
4. **Is cancellation cooperative or forced?** — No evidence found. No `asyncio.CancelledError` handling, no `signal.SIGINT`/`SIGTERM` handlers, no abort-controller wiring is present locally.
5. **Does cancellation leave resources dirty?** — No evidence found. No `finally` blocks, no context managers, no resource lifecycle code can be examined in this empty checkout.

## Architectural Decisions

No clear evidence found. The source directory is empty; no `Flow`, `Crew`, `AgentExecutor`, tool wrapper, or LLM call site is available locally to characterize the architecture for this dimension.

## Notable Patterns

No clear evidence found. No files exist in the selected source directory to surface timeout wrappers, cancellation tokens, abort controllers, cleanup handlers, cancelled-status enums, or timeout tests.

## Tradeoffs

No clear evidence found. Tradeoff analysis cannot be performed because there is no code, configuration, or documentation present inside `studies/agent-harness-study/sources/crewai/` to compare against alternatives.

## Failure Modes / Edge Cases

No clear evidence found. The dimension's central question — "Can a stuck shell command or API call be stopped without killing the whole run?" — cannot be answered from the local source. Possible failure modes the framework *might* exhibit (LLM HTTP call hanging indefinitely; tool subprocess never returning; Flow listener awaiting forever) are not observable here.

## Future Considerations

- **Re-fetch the source.** `studies/agent-harness-study/sources/crewai/` needs to be populated with a checkout of `https://github.com/crewAIInc/crewAI` before this dimension can be studied. The manifest already points at the upstream URL (`studies/agent-harness-study/sources/crewai.ultraplan-source.yml:2`), so a `git clone` into the source directory would unblock this and the remaining empty-source dimensions in the study.
- **Cross-check sibling reports at evaluation time.** If re-fetch is not possible, the validator should treat this report as a no-evidence gap rather than a negative finding, since absence of local files is not equivalent to absence of upstream behavior.
- **Operational note for study infrastructure.** Five of the eleven sources are currently empty (`crewai`, `langgraph`, `letta`, `opa`, `openai-agents-sdk`, `pydantic-ai`, `temporal`); the study runner should ensure source materialization before dispatching dimension tasks for these repositories.

## Questions / Gaps

- The source directory is empty; what was the intended source location and how is materialization triggered?
- The manifest at `studies/agent-harness-study/sources/crewai.ultraplan-source.yml:2` references a public GitHub URL — was the upstream intended to be cloned per-task or pre-checked into the workspace?
- Should this dimension be re-run after the source is populated, or is this gap acceptable given the broader empty-source pattern in the study?

---

Generated by `studies/agent-harness-study/dimensions/07.04-timeouts-and-cancellation.md` against `crewai`.
