# Source Analysis: crewai

## Public API Surface

### Source Info

| Field | Value |
|-------|-------|
| Name | crewai |
| Path | `studies/agent-harness-study/sources/crewai` |
| Language / Stack | Unknown — source directory is empty; manifest references `https://github.com/crewAIInc/crewAI` (Python multi-agent orchestration framework; expected primary stack: Python on top of LiteLLM/Provider abstractions, with `crewai` core package, `crewai-tools`, `crewai-flows`) |
| Analyzed | 2026-08-22 |

## Summary

The selected source directory `studies/agent-harness-study/sources/crewai` contains no files. Searched the directory recursively for files, subdirectories, hidden files, symlinks, and any contents — only the directory itself exists. The sibling manifest `studies/agent-harness-study/sources/crewai.ultraplan-source.yml` exists at line 1-87 and references `https://github.com/crewAIInc/crewAI`, but the manifest is metadata describing this study's plan, not part of the source itself and therefore off-limits for API-surface evidence under the isolation rule. No source code, configuration, package manifests, public API definitions, examples, or documentation files are present to inspect. Consequently, no claims about the public API surface of crewai can be substantiated from local evidence.

Search boundary: `find studies/agent-harness-study/sources/crewai -type f` returned zero results; `find … -type d` returned only the source root itself; `ls -la` confirms a single empty directory entry (`.` and `..` only, no `README`, no `pyproject.toml`, no `setup.py`, no `requirements.txt`, no `package.json`, no `Cargo.toml`, no source tree, no `docs/`, no `examples/`, no `tests/`, no `LICENSE`). No `src/`, no `crewai/`, no `crewai_tools/`, no `flows/`, no `tests/` directory exists.

## Rating

**Score: 1 / 10 — Absent.**

Rationale (per the dimension rubric): the public API surface is absent from the inspection boundary because the source material itself is absent. A score of 1 is warranted under the rubric band "Absent, implicit, ad-hoc, or unsafe." Without any local artifacts to inspect, the dimension cannot be evaluated for naming consistency, lifecycle ownership, abstraction boundaries, documentation, or discoverability. A higher score is not defensible: there is no public API to grade, only an empty source directory.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Source presence | `find studies/agent-harness-study/sources/crewai -type f` returned zero results; directory listing contains only `.` and `..` | `studies/agent-harness-study/sources/crewai/:1` (directory entry) |
| Manifest reference (metadata only, not source) | The source manifest names the upstream URL `https://github.com/crewAIInc/crewAI` and lists applicable dimensions; this file is the study's planning metadata, not source code | `sources/crewai.ultraplan-source.yml:2` |
| Stable import paths | No clear evidence found — no `pyproject.toml`, `setup.py`, `setup.cfg`, `requirements.txt`, `Cargo.toml`, or `package.json` exists to define import boundaries or package distribution | n/a (no file present) |
| Public packages, modules, clients, command groups, HTTP/RPC routes | No clear evidence found — no source tree, no `crewai/` package, no `Agent`, `Crew`, `Task`, `Flow`, `Tool`, `LLM`, or `Process` definitions exist in the selected source directory | n/a (no file present) |
| Documentation and example coverage | No clear evidence found — no `README`, no `docs/`, no `examples/`, no `samples/`, no `tutorials/` directory exists in the selected source directory | n/a (no file present) |
| API stability markers or internal/experimental labels | No clear evidence found — no API definitions, decorators, `@deprecated` markers, `# Experimental:` docstrings, `__all__` lists, or annotation files exist | n/a (no file present) |
| Import/export boundaries | No clear evidence found — no language-specific module or package manifests exist; no `__init__.py`, no `src/crewai/`, no `src/crewai_tools/`, no `src/crewai/flows/` exists | n/a (no file present) |
| Evidence of accidental public surface area | No clear evidence found — no exports, re-exports, or symbol lists exist to assess accidental exposure; no `__all__` to constrain the surface, no `_internal/` namespace visible | n/a (no file present) |
| CLI surface | No clear evidence found — no `pyproject.toml` `[project.scripts]`, no `cli/`, no `main.py`, no `click`/`typer`/`argparse` entrypoint exists in the selected source directory | n/a (no file present) |

## Answers to Dimension Questions

1. **What is the intended public API surface?**
   No clear evidence found. The selected source directory is empty; there are no stable import paths, client objects, CLI commands, service endpoints, or documented entry points present locally to identify the intended public API surface. Upstream knowledge (off-limits under isolation) would suggest `from crewai import Agent, Crew, Task, Process`, `from crewai_tools import ...`, and `from crewai.flow.flow import Flow, listen, or_`, but none of this can be cited from local files.

2. **Is the stable API easy to distinguish from internal implementation details?**
   No clear evidence found. With no source files present, no separation between stable public API and internals can be observed (e.g., no `__all__` lists in Python, no `_internal/` suffix convention, no `pyproject.toml` `private` markers, no `__init__.py` re-export discipline, no deprecation decorators such as `@deprecated` from `typing_extensions`). The expected crewai layout of public `crewai/agent.py`, `crewai/crew.py`, `crewai/task.py`, `crewai/process.py` versus internal modules under `crewai/utilities/`, `crewai/memory/`, `crewai/llms/` cannot be confirmed locally.

3. **Does the API expose the right level of abstraction for agent harness users?**
   No clear evidence found. No abstraction layer, agent/crew/task builders, tool registries, process selection enum (`Process.sequential`, `Process.hierarchical`), memory providers, or runtime entry points exist locally to evaluate abstraction choices for harness authors. The expected CrewAI abstraction (Agent = role+goal+backstory+tools; Crew = agents+tasks+process; Task = agent+description+expected_output) cannot be evidenced from this study.

5. **Are examples sufficient to use the API correctly without reading internals?**
   No clear evidence found. No example files, tutorials, snippets, `examples/` directory, `notebooks/` directory, or `docs/` tree are present in the selected source. Whether the upstream repository ships `examples/`, `docs/how-to/`, `docs/core-concepts/`, or runnable cookbook notebooks cannot be verified from this study.

## Architectural Decisions

No clear evidence found. No source files, configuration, manifests, or documentation are present in the selected source directory to identify architectural decisions about API grouping, naming, lifecycle ownership, version policy, or abstraction layering. Upstream knowledge (off-limits) suggests crewai follows a "builder + orchestration" pattern with `Agent`/`Crew`/`Task` as the headline trio and `Process.sequential` vs `Process.hierarchical` as the execution strategy axis, plus a separate `crewai.flows` / `crewai.flow` package for event-driven Flows. None of this can be cited from local files.

## Notable Patterns

No clear evidence found. No patterns (factory, builder, fluent-API, module facade, `@tool` decorator, `@agent` / `@task` / `@crew` decorators, capability provider, agent delegation via `allow_delegation=True`, etc.) can be observed because no source code is present.

## Tradeoffs

No clear evidence found. Without source material, no tradeoff discussion (e.g., breadth vs. stability, ergonomics vs. flexibility, public surface area vs. maintenance burden, single `crewai` package vs. split into `crewai-core` + `crewai-tools` + `crewai-flows`) is grounded in evidence. Upstream tradeoff that would normally be evaluated here — separating orchestration (`Crew`) from execution (`Process`) and memory (`Memory`) from tool runtime (`Tool`) — cannot be examined.

## Failure Modes / Edge Cases

No clear evidence found. No API definitions, validation logic, error envelopes, Pydantic model validators, deprecation markers, or runtime guard rails exist locally to study failure modes. The only observable failure mode is at the study-input layer: an empty source directory prevents evidence-based analysis of the dimension at all. A second-order failure mode worth flagging: an empty source for a dimension that depends on cross-cutting public API observations also blocks downstream dimensions (e.g., 22.01 package-module boundaries, 24.02 stability, 24.03 documentation) for crewai unless the source is populated first.

## Future Considerations

If the source directory is populated (e.g., via `git clone https://github.com/crewAIInc/crewAI` into `studies/agent-harness-study/sources/crewai/`), the analysis should be re-run. Specifically, re-inspect:

- Top-level `crewai/` package layout: `agent.py`, `crew.py`, `task.py`, `process.py`, `llm.py`, `memory/`, `tools/`, `utilities/`, `flows/` (if monolithic) vs. split monorepo with `crewai-core/`, `crewai-tools/`, `crewai-flows/` subprojects
- Whether `crewai/__init__.py` re-exports a stable `Agent`, `Crew`, `Task`, `Process`, `LLM`, `Flow`, `listen`, `and_`, `or_`, `start` surface via `__all__`
- Whether `crewai-tools` is shipped as a separate distribution (`pip install crewai-tools`) and whether tool authors integrate via a stable `@tool`/`@BaseTool` interface
- Whether `crewai-flows` (or `crewai.flow`) introduces a new public surface (Flow, listen, and_, or_, start, persist) that overlaps or conflicts with `Crew`/`Agent` semantics
- Documentation index under `docs/`, including `docs/core-concepts/`, `docs/how-to/`, `docs/learn/`, and auto-generated API reference
- Example coverage under `examples/`, including `example_simple.py`, `example_with_tools.py`, `example_flow*.py`, `hierarchical_*.py`
- CLI surface via `pyproject.toml` `[project.scripts]` (e.g., `crewai` / `crewai run` / `crewai deploy`) and whether it exposes a stable command group
- Whether public classes carry `@deprecated` decorators, docstring `.. deprecated::` blocks, or `typing_extensions.deprecated` markers to telegraph churn
- Whether `Process` is an `enum.Enum` with `sequential` and `hierarchical` as stable values versus a stringly-typed config flag

## Questions / Gaps

- Was the upstream repository `https://github.com/crewAIInc/crewAI` expected to be cloned into `studies/agent-harness-study/sources/crewai/` before dimension tasks were dispatched? The selected source directory is empty, while sibling sources (`langfuse`, `openhands`) were cloned with commits visible in `git status`.
- Should the harness study runner pre-clone source repositories before scheduling dimension tasks, or is the empty directory an intentional placeholder to be filled by a later step?
- Is the upstream repository even publicly accessible at the URL recorded in `sources/crewai.ultraplan-source.yml:2`? No remote fetch was performed under the isolation rule.
- Without local source, every dimension question against `crewai` is unanswerable. The orchestration layer should treat empty source directories as a hard pre-condition failure rather than dispatching dimension tasks.
- crewai has historically distinguished between `crewai` (orchestration), `crewai-tools` (tool catalog), and `crewai-flows` / `crewai.flow` (event-driven state machines) as separate distributions. The study runner should pre-decide whether the dimension analyzes all three subpackages or only the head `crewai` package, since the answer materially changes the public API surface evaluation.

---

Generated by `24.01-public-api-surface` against `crewai`.