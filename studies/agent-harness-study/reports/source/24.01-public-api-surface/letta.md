# Source Analysis: letta

## Public API Surface

### Source Info

| Field | Value |
|-------|-------|
| Name | letta |
| Path | `studies/agent-harness-study/sources/letta` |
| Language / Stack | Unknown — source directory is empty; manifest references `https://github.com/letta-ai/letta` (memory-first agent architecture, formerly MemGPT; expected primary stack: Python backend with FastAPI server + TypeScript/React frontend, `letta` and `letta-client` PyPI distributions) |
| Analyzed | 2026-08-22 |

## Summary

The selected source directory `studies/agent-harness-study/sources/letta` contains no files. Searched the directory recursively for files, subdirectories, hidden files, symlinks, and any contents — only the directory itself exists. The sibling manifest `studies/agent-harness-study/sources/letta.ultraplan-source.yml` exists at line 1-75 and references `https://github.com/letta-ai/letta`, but the manifest is metadata describing this study's plan, not part of the source itself and therefore off-limits for API-surface evidence under the isolation rule. No source code, configuration, package manifests, public API definitions, examples, or documentation files are present to inspect. Consequently, no claims about the public API surface of letta can be substantiated from local evidence.

Search boundary: `find studies/agent-harness-study/sources/letta -type f` returned zero results; `find … -type d` returned only the source root itself; `ls -la` confirms a single empty directory entry (`.` and `..` only, no `README`, no `pyproject.toml`, no `setup.py`, no `requirements.txt`, no `package.json`, no source tree, no `docs/`, no `examples/`, no `tests/`, no `LICENSE`). No `letta/`, no `letta_client/`, no `letta/server/`, no `letta/agents/`, no `tests/` directory exists.

## Rating

**Score: 1 / 10 — Absent.**

Rationale (per the dimension rubric): the public API surface is absent from the inspection boundary because the source material itself is absent. A score of 1 is warranted under the rubric band "Absent, implicit, ad-hoc, or unsafe." Without any local artifacts to inspect, the dimension cannot be evaluated for naming consistency, lifecycle ownership, abstraction boundaries, documentation, or discoverability. A higher score is not defensible: there is no public API to grade, only an empty source directory.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Source presence | `find studies/agent-harness-study/sources/letta -type f` returned zero results; directory listing contains only `.` and `..` | `studies/agent-harness-study/sources/letta/:1` (directory entry) |
| Manifest reference (metadata only, not source) | The source manifest names the upstream URL `https://github.com/letta-ai/letta`, describes the project as "Memory-first agent architecture (formerly MemGPT)", and lists applicable dimensions; this file is the study's planning metadata, not source code | `sources/letta.ultraplan-source.yml:1-3` |
| Stable import paths | No clear evidence found — no `pyproject.toml`, `setup.py`, `setup.cfg`, `requirements.txt`, `Cargo.toml`, or `package.json` exists to define import boundaries or package distribution | n/a (no file present) |
| Public packages, modules, clients, command groups, HTTP/RPC routes | No clear evidence found — no source tree, no `letta/` package, no `MemoryAgent` / `Agent` / `ArchivalMemory` / `RecallMemory` / `Tool` / `LLM` definitions exist in the selected source directory | n/a (no file present) |
| Documentation and example coverage | No clear evidence found — no `README`, no `docs/`, no `examples/`, no `samples/`, no `notebooks/`, no `tutorials/` directory exists in the selected source directory | n/a (no file present) |
| API stability markers or internal/experimental labels | No clear evidence found — no API definitions, decorators, `@deprecated` markers, `# Experimental:` docstrings, `__all__` lists, `PublicAPI` markers, or annotation files exist | n/a (no file present) |
| Import/export boundaries | No clear evidence found — no language-specific module or package manifests exist; no `__init__.py`, no `src/letta/`, no `letta/server/`, no `letta/agents/` exists | n/a (no file present) |
| Evidence of accidental public surface area | No clear evidence found — no exports, re-exports, or symbol lists exist to assess accidental exposure; no `__all__` to constrain the surface, no `_internal/` namespace visible | n/a (no file present) |
| CLI surface | No clear evidence found — no `pyproject.toml` `[project.scripts]`, no `cli/`, no `letta/__main__.py`, no `click`/`typer`/`argparse` entrypoint exists in the selected source directory | n/a (no file present) |
| HTTP/RPC service routes | No clear evidence found — no FastAPI / Flask / gRPC route definitions, no Pydantic schemas, no OpenAPI specs (`openapi.json`, `openapi.yaml`) exist in the selected source directory | n/a (no file present) |
| Client SDK surface | No clear evidence found — no `letta_client/`, no TypeScript SDK under `sdks/typescript/`, no auto-generated client code from a Fern/Stainless/OpenAPI generator exists in the selected source directory | n/a (no file present) |

## Answers to Dimension Questions

1. **What is the intended public API surface?**
   No clear evidence found. The selected source directory is empty; there are no stable import paths, client objects, CLI commands, service endpoints, or documented entry points present locally to identify the intended public API surface. Upstream knowledge (off-limits under isolation) would suggest a Python `from letta import Agent` or `from letta import MemoryAgent` import surface, a separate `letta-client` Python SDK (`from letta_client import Letta`), and an HTTP API rooted at `POST /v1/agents/...` with companion `/v1/messages` / `/v1/tools` / `/v1/sources` endpoints, plus a TypeScript SDK; but none of this can be cited from local files.

2. **Is the stable API easy to distinguish from internal implementation details?**
   No clear evidence found. With no source files present, no separation between stable public API and internals can be observed (e.g., no `__all__` lists in Python, no `_internal/` suffix convention, no `pyproject.toml` `private` markers, no `__init__.py` re-export discipline, no deprecation decorators such as `@deprecated` from `typing_extensions`, no Fern/Stainless-generated client with explicit "stable" vs. "internal" namespace split). The expected letta layout of public `letta/agent.py` / `letta/schemas.py` versus internal modules under `letta/orm/`, `letta/services/`, `letta/agents/`, `letta/llm_api/`, `letta/memory/` cannot be confirmed locally.

3. **Does the API expose the right level of abstraction for agent harness users?**
   No clear evidence found. No abstraction layer, agent builders, memory blocks (`core_memory`, `archival_memory`, `recall_memory`), tool registries, message streams, or runtime entry points exist locally to evaluate abstraction choices for harness authors. The expected Letta abstraction (memory-first agent = persona + human + system blocks + tool rules + archival memory search; message stream with reasoning + tool-call + assistant-message tokens; streaming vs. non-streaming transports via SSE / WebSocket / blocking JSON) cannot be evidenced from this study.

4. **Are examples sufficient to use the API correctly without reading internals?**
   No clear evidence found. No example files, tutorials, snippets, `examples/` directory, `notebooks/` directory, or `docs/` tree are present in the selected source. Whether the upstream repository ships `examples/`, `docs/quickstart/`, `docs/guides/`, or runnable cookbook notebooks cannot be verified from this study.

## Architectural Decisions

No clear evidence found. No source files, configuration, manifests, or documentation are present in the selected source directory to identify architectural decisions about API grouping, naming, lifecycle ownership, version policy, or abstraction layering. Upstream knowledge (off-limits) suggests letta follows a "memory-first agent harness" pattern with separate headless Python package, FastAPI server, separate Python client SDK (`letta-client`), TypeScript SDK, and CLI, plus a managed Letta Cloud offering whose client SDK is also auto-generated from the same OpenAPI spec. None of this can be cited from local files.

## Notable Patterns

No clear evidence found. No patterns (factory, builder, fluent-API, module facade, memory-block mixin, tool-call envelope, streaming-sse endpoint, capability provider, agent delegation, etc.) can be observed because no source code is present. The expected MemGPT-originated patterns (in-context vs. out-of-context memory, archival vs. recall memory tools, self-editing memory blocks via `core_memory_replace` / `core_memory_append` / `archival_memory_insert` / `archival_memory_search` tool calls) cannot be evidenced from this study.

## Tradeoffs

No clear evidence found. Without source material, no tradeoff discussion (e.g., breadth vs. stability, ergonomics vs. flexibility, public surface area vs. maintenance burden, single `letta` package vs. split into `letta` server + `letta-client` SDK + `letta-ai/typescript-sdk`, blocking vs. streaming API, gRPC vs. REST, OpenAPI-codegen vs. hand-written client) is grounded in evidence. Upstream tradeoff that would normally be evaluated here — separating the server runtime (the `letta` package) from the user-facing client (`letta-client`) so that end users do not pull FastAPI/SQLAlchemy/Pydantic into agent applications — cannot be examined.

## Failure Modes / Edge Cases

No clear evidence found. No API definitions, validation logic, error envelopes, Pydantic model validators, deprecation markers, or runtime guard rails exist locally to study failure modes. The only observable failure mode is at the study-input layer: an empty source directory prevents evidence-based analysis of the dimension at all. A second-order failure mode worth flagging: an empty source for a dimension that depends on cross-cutting public API observations also blocks downstream dimensions (e.g., 22.01 package-module boundaries, 24.02 stability, 24.03 documentation, 24.04 versioning) for letta unless the source is populated first.

## Future Considerations

If the source directory is populated (e.g., via `git clone https://github.com/letta-ai/letta` into `studies/agent-harness-study/sources/letta/`), the analysis should be re-run. Specifically, re-inspect:

- Top-level `letta/` package layout: `agent.py`, `schemas/`, `memory.py`, `llm_api/`, `server/`, `services/`, `tool.py`, `prompt.py`, `errors.py`, `client/` (if monolithic) vs. split monorepo with `letta/` server, `letta-client/` SDK, and a separate TypeScript SDK
- Whether `letta/__init__.py` re-exports a stable `Agent`, `MemoryAgent`, `LLMConfig`, `Tool`, `ToolRule`, `Block`, `Message`, `MessageRole`, `MessageCreate` surface via `__all__`
- Whether `letta-client` is shipped as a separate distribution (`pip install letta-client`) and whether it auto-generates from OpenAPI or is hand-written
- Whether the HTTP API (`POST /v1/agents`, `POST /v1/agents/{agent_id}/messages`, streaming via SSE at `/v1/agents/{agent_id}/messages/stream`, `POST /v1/agents/{agent_id}/messages/stream` WebSocket) is exposed as the canonical public surface and whether internal endpoints (`/internal/...`) are clearly fenced off
- Whether memory blocks (`core_memory`, `archival_memory`, `recall_memory`) are surfaced as first-class types in the public schema or hidden behind internal modules
- Whether tool rules (`ToolRule`, `init_tool_rules`, `terminal_tool_rules`, `continue_tool_rules`, `exit_message_tool`) are part of the public surface
- Documentation index under `docs/` (Mintlify or similar), including `docs/quickstart`, `docs/guides/memgpt/`, `docs/api-reference/`, and auto-generated API reference
- Example coverage under `examples/` (e.g., `examples/memgpt_doc_qa.py`, `examples/tool_use_example.py`, `examples/streaming_example.py`, `examples/archival_memory_example.py`)
- CLI surface via `pyproject.toml` `[project.scripts]` (e.g., `letta`, `letta run`, `letta serve`, `letta migrate`, `letta reset`) and whether it exposes a stable command group
- Whether public classes carry `@deprecated` decorators, docstring `.. deprecated::` blocks, or `typing_extensions.deprecated` markers to telegraph churn — this matters for letta because the project has gone through significant restructuring between the original MemGPT API (`from memgpt import MemGPT`) and the current Letta API (`from letta_client import Letta`)
- Whether the API reference is generated from OpenAPI via Fern/Stainless (similar to Langfuse, see `sources/langfuse/fern/`) or hand-maintained
- Whether TypeScript SDK lives at `letta-ai/letta-typescript-sdk` (separate repo) or inside the monorepo, and whether `pnpm-workspace.yaml` / `package.json` workspace declarations exist

## Questions / Gaps

- Was the upstream repository `https://github.com/letta-ai/letta` expected to be cloned into `studies/agent-harness-study/sources/letta/` before dimension tasks were dispatched? The selected source directory is empty, while sibling sources (`langfuse`, `openhands`) were cloned with commits visible in `git status`.
- Should the harness study runner pre-clone source repositories before scheduling dimension tasks, or is the empty directory an intentional placeholder to be filled by a later step?
- Is the upstream repository even publicly accessible at the URL recorded in `sources/letta.ultraplan-source.yml:2`? No remote fetch was performed under the isolation rule.
- Without local source, every dimension question against `letta` is unanswerable. The orchestration layer should treat empty source directories as a hard pre-condition failure rather than dispatching dimension tasks.
- Letta's public API has historically gone through a major rename from MemGPT to Letta, plus a structural split between the `letta` server package and the separate `letta-client` SDK. The study runner should pre-decide whether the dimension analyzes the server-only surface, the client-only surface, or both, since the answer materially changes the public API surface evaluation.

---

Generated by `24.01-public-api-surface` against `letta`.
