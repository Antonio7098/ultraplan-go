# Source Analysis: pydantic-ai

## Public API Surface

### Source Info

| Field | Value |
|-------|-------|
| Name | pydantic-ai |
| Path | `studies/agent-harness-study/sources/pydantic-ai` |
| Language / Stack | Unknown (Python expected, source not present on disk) |
| Analyzed | 2026-08-22 |

## Summary

The selected source directory `studies/agent-harness-study/sources/pydantic-ai` is empty on the local filesystem. A recursive listing with `ls -la` and `find -type f` returns no files, no hidden files, no subdirectories, no manifest (`pyproject.toml`, `setup.py`, `pdm.lock`, `uv.lock`, `src/`, `pydantic_ai/`, `pydantic_ai_slim/`), and no README. The source declared at `sources/pydantic-ai.ultraplan-source.yml:2` (`https://github.com/pydantic/pydantic-ai`) has not been materialised into the study workspace, so per the source-isolation rule ("You are studying exactly one selected source. You may ONLY access files inside that source's directory") no inspection of code, configuration, tests, or docs inside the project was possible. The accompanying manifest file `pydantic-ai.ultraplan-source.yml` (`sources/pydantic-ai.ultraplan-source.yml:1-79`) only declares metadata (name, URL, description, applicable dimensions) — it is not part of the source tree and cannot substitute for code evidence.

Public API surface cannot be evaluated when no importable package, no module surface, no CLI entry point, no HTTP/RPC handler, no `__init__.py` re-export list, no `__all__`, no `py.typed` marker, no Sphinx/MkDocs site, and no examples directory exists in the inspected directory. The rubric anchor for 1–3 ("Absent, implicit, ad-hoc, or unsafe") applies because there is literally no code to evaluate. The rating below is not a judgment on pydantic-ai itself; it is a judgment on the evidence available under the source-isolation constraint.

The analysis below therefore records the absence of evidence rather than fabricating findings. The search boundary was the directory itself plus the adjacent manifest file at `sources/pydantic-ai.ultraplan-source.yml:1-79`. Every path cited as missing is at the root of `studies/agent-harness-study/sources/pydantic-ai/`.

## Rating

**1 / 10** — Absent.

Rationale: Public API surface — stable import paths, client objects, CLI commands, service endpoints, documented entry points, examples, naming conventions, lifecycle ownership, discoverability, abstraction boundaries — cannot be evaluated when no package, no modules, no `__init__.py`, no entry-point declarations (`[project.scripts]`), no `py.typed` marker, no type stubs, no examples folder, and no documentation site exist in the inspected directory. There is no `Agent`, no `Tool`, no `Model`, no `RunContext`, no `UsageLimits`, no `Run` class, no `__main__` block, no `pydantic_ai` namespace to inspect. Without a manifest or module tree, none of the four rubric dimensions (intended surface, separation from internals, abstraction level, example coverage) can be cited with evidence. The 1/10 score reflects the rubric floor under the source-isolation constraint.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Top-level import surface | No `pyproject.toml`, `setup.py`, `setup.cfg`, `src/`, `pydantic_ai/`, `pydantic_ai_slim/`, or namespace package declaration present. Searched: `studies/agent-harness-study/sources/pydantic-ai/*` — zero matches. | `studies/agent-harness-study/sources/pydantic-ai/:1` (directory empty) |
| Stable import paths | No `__init__.py`, no `__all__` lists, no re-export surface to inspect. | `studies/agent-harness-study/sources/pydantic-ai/:1` (directory empty) |
| Client objects / entry points | No `[project.scripts]`, no `[project.entry-points]`, no `console_scripts`, no `__main__.py`, no CLI command group to enumerate. | `studies/agent-harness-study/sources/pydantic-ai/:1` (directory empty) |
| HTTP / RPC service endpoints | No `fastapi`, `starlette`, `aiohttp`, `flask`, `grpc`, `django` route surface — no handlers, no routers, no middleware to list. | `studies/agent-harness-study/sources/pydantic-ai/:1` (directory empty) |
| API stability markers | No `py.typed` marker, no `*.pyi` stubs, no `deprecated` decorators, no `__deprecated__` attributes, no stability labels (`@public_api`, `@internal`, `@experimental`) could be located. | `studies/agent-harness-study/sources/pydantic-ai/:1` (directory empty) |
| Import/export boundaries | No `__all__`, no `_`-prefix convention enforcement, no `typing.TYPE_CHECKING` import-block discipline to assess. | `studies/agent-harness-study/sources/pydantic-ai/:1` (directory empty) |
| Documentation site & examples | No `docs/`, no `examples/`, no `README.md`, no MkDocs/Sphinx configuration, no doctest fixtures. | `studies/agent-harness-study/sources/pydantic-ai/:1` (directory empty) |
| Tests demonstrating public API | No `tests/`, no `test_*.py`, no `conftest.py`, no pytest configuration to anchor intended behaviour. | `studies/agent-harness-study/sources/pydantic-ai/:1` (directory empty) |
| Build / packaging config | No `pyproject.toml`, `pdm.lock`, `uv.lock`, `poetry.lock`, `setup.py`, `requirements*.txt`, `MANIFEST.in`. | `studies/agent-harness-study/sources/pydantic-ai/:1` (directory empty) |
| Source manifest pointer | URL `https://github.com/pydantic/pydantic-ai` declared but not fetched into the source directory. | `sources/pydantic-ai.ultraplan-source.yml:2` |
| Dimension scope | This dimension (`24.01`) is listed as applicable (line 76), confirming the study intent to evaluate the public surface of pydantic-ai. | `sources/pydantic-ai.ultraplan-source.yml:76` |
| Description anchor | Manifest describes pydantic-ai as "Type-system-centric agent design with validated structured outputs", which is the only available framing signal. | `sources/pydantic-ai.ultraplan-source.yml:3` |

## Answers to Dimension Questions

1. **What is the intended public API surface?** — No clear evidence found. Search boundary: the entire `studies/agent-harness-study/sources/pydantic-ai/` directory, which contains zero files. No `Agent`, `AgentRun`, `Tool`, `ToolDefinition`, `Model`, `ModelSettings`, `RunContext`, `Run`, `UsageLimits`, `Message`, or `Result` symbols could be located. The manifest at `sources/pydantic-ai.ultraplan-source.yml:3` advertises "validated structured outputs" but no source files back this with importable symbols.
2. **Is the stable API easy to distinguish from internal implementation details?** — No clear evidence found. There is no `__all__`, no `py.typed` marker, no `*.pyi` stubs, no `_`-prefix or `_`-suffix naming discipline to evaluate, no `public/` vs `internal/` directory split, no `# noqa: F401` re-export markers, and no reST/Sphinx `:public-api:` directive. The boundary is invisible at the filesystem level.
3. **Does the API expose the right level of abstraction for agent harness users?** — No clear evidence found. There is no `Agent` constructor, no `agent.run_sync(...)` / `agent.run(...)` / `agent.iter(...)` entry surface, no dependency-injection boundary (e.g., `RunContext[Deps]`), no tool-registration decorator (`@agent.tool`, `@agent.tool_plain`), and no provider abstraction (`OpenAIModel`, `AnthropicModel`, `BedrockModel`, etc.) to assess.
4. **Are examples sufficient to use the API correctly without reading internals?** — No clear evidence found. There is no `examples/`, no `docs/`, no `README.md`, no doctest, no `mkdocs.yml`, no tutorial-style code comments, and no runnable snippets anywhere in the inspected directory. Without a source tree, the answer cannot differ from "no evidence".

## Architectural Decisions

No clear evidence found. The directory contains no files from which architectural decisions about the public API could be derived. The only available signal is the manifest description at `sources/pydantic-ai.ultraplan-source.yml:3` ("Type-system-centric agent design with validated structured outputs"), which is a marketing-level statement and cannot be cited as an implemented architectural decision.

The following decisions cannot be inspected because the source is absent:

- Whether `Agent` is the primary user-facing construct or one of several.
- Whether providers live in sub-packages (`pydantic_ai.models.openai`, `pydantic_ai.models.anthropic`, `pydantic_ai.models.bedrock`, `pydantic_ai.models.gemini`, etc.) or are flattened.
- Whether `pydantic_ai` and `pydantic_ai_slim` are co-shipped distributions and which symbols belong to which.
- Whether tools are registered via decorator, via explicit list, or via class.
- Whether streaming, structured output, and async iteration each have a dedicated public type or share one.

## Notable Patterns

No clear evidence found.

Patterns that cannot be cited because the source is absent:

- Type-driven output validation (whether Pydantic models are passed directly to `output_type=`).
- Provider-agnostic model interface (whether models implement a common `Model` ABC).
- Dependency-injection via `RunContext[Deps]` generic.
- `@agent.tool` / `@agent.tool_plain` decorator-based registration vs. explicit registration.
- Agent-as-tool composition (`some_agent.as_tool("name")` style).
- Structured streaming via `agent.iter(...)` async iterator yielding partial messages.

## Tradeoffs

No clear evidence found. Without a manifest, a module tree, or a public-API declaration, no tradeoffs can be cited. The following tradeoff axes cannot be evaluated:

- Single distribution (`pydantic-ai`) vs. split distribution (`pydantic-ai` + `pydantic-ai-slim`).
- Flat namespace (`Agent`, `Tool`, `Model` at root) vs. sub-packages (`agent.Agent`, `tool.Tool`, `models.Model`).
- Lazy provider loading vs. eager provider loading.
- Decorator-first vs. imperative-first tool registration.
- Type-first output (`output_type=MyModel`) vs. function-first output (`output_type=MyModel.model_validate_json`).
- Public `py.typed` marker vs. untyped surface.

## Failure Modes / Edge Cases

- **Source not materialised.** The study workflow assumes the source has been cloned/copied into `studies/agent-harness-study/sources/pydantic-ai/`. In this run it has not. Any dimension that depends on this source will hit the same gap and must either (a) request materialisation of the source or (b) record "no clear evidence found".
- **Cross-source isolation blocks workaround.** Hard rule #1 forbids reading sibling sources (`../langfuse/`, `../openhands/`, etc.) to compensate, so the analysis must terminate at the empty-directory boundary.
- **No public-API evidence possible at 24.01.** Unlike 22.01 (package/module boundaries), where the absence of files can sometimes be partially inferred from build configuration, 24.01 strictly requires inspecting the importable surface — `__init__.py`, `__all__`, type stubs, entry-point declarations, decorators, CLI handlers. None of those artefacts are present, so the report cannot move beyond "absent".
- **Manifest description is not evidence.** The line at `sources/pydantic-ai.ultraplan-source.yml:3` ("Type-system-centric agent design with validated structured outputs") is a one-line marketing summary; treating it as architectural evidence would violate rule #3 ("Cite evidence, not vibes").

## Future Considerations

- Materialise `pydantic-ai` into `studies/agent-harness-study/sources/pydantic-ai/` (e.g., `git clone https://github.com/pydantic/pydantic-ai`) before running any dimension that requires code inspection.
- Once materialised, re-run this dimension to evaluate:
  - Top-level re-exports in `pydantic_ai/__init__.py` and any `__all__` declaration.
  - Sub-package split (`agent/`, `models/`, `tools/`, `messages/`, `settings/`, `exceptions/`, `result/`, `formatting/`).
  - Provider adapters under `pydantic_ai/models/<provider>/` (OpenAI, Anthropic, Bedrock, Gemini, Groq, Mistral, Cohere, etc.).
  - Tool-registration decorators (`@agent.tool`, `@agent.tool_plain`) and tool-as-agent composition (`agent.as_tool`).
  - Streaming primitives (`agent.iter(...)`, `AgentRun`, `PartDeltaEvent`).
  - CLI entry points (e.g., `pydantic-ai` console script, `pydantic_ai eval`, `pydantic_ai chat`).
  - `py.typed` marker, `*.pyi` stubs, deprecation markers (`warnings.warn(..., DeprecationWarning, stacklevel=2)`).
  - Documentation site (`docs/`, MkDocs config, example gallery).
- Consider a study-level pre-flight check that fails fast when a source directory is empty, instead of producing N "no evidence" reports across dimensions.
- Consider extending the manifest schema with a `materialisation_command` or `git_ref` field so the runner can clone on demand without violating source isolation.

## Questions / Gaps

- Why is the `pydantic-ai` source directory empty while sibling `langfuse/` and `openhands/` are populated? Is there a fetch step missing from the study bootstrap, or a per-source allowlist that excludes it?
- Is there an out-of-band mechanism (git submodule, archive download, monorepo path) the study expects the analyst to use? If so, it must be documented in the prompt because rule #1 forbids reaching outside the source directory.
- Should future prompts allow a "source unavailable — abort" exit code instead of forcing a low-score, no-evidence report across all dimensions that depend on the source?
- Is the dimension scope at `sources/pydantic-ai.ultraplan-source.yml:5-79` (which lists `24.01` as applicable) accurate given the source has not been fetched — i.e., was the source ever actually available for earlier dimensions, and was it lost between runs, or was it never fetched at all?
- Does the runner expect to read public pydantic-ai files from an upstream cache (e.g., `~/.cache/agent-harness-study/pydantic-ai`) that the prompt does not mention?

---

Generated by `dimensions/24.01-public-api-surface` against `pydantic-ai`.
