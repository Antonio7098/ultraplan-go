# Source Analysis: pydantic-ai

## Interface Contract Design

### Source Info

| Field | Value |
|-------|-------|
| Name | pydantic-ai |
| Path | `studies/agent-harness-study/sources/pydantic-ai` |
| Language / Stack | Unknown (Python expected, source not present on disk) |
| Analyzed | 2026-08-23 |

## Summary

The selected source directory `studies/agent-harness-study/sources/pydantic-ai` is empty on the local filesystem. A recursive listing with `ls -la` and `find -type f` returns no files, no hidden files, no subdirectories, no manifest (`pyproject.toml`, `setup.py`, `pdm.lock`, `uv.lock`, `src/`, `pydantic_ai/`, `pydantic_ai_slim/`), no tests, and no README. The upstream repository declared at `sources/pydantic-ai.ultraplan-source.yml:2` (`https://github.com/pydantic/pydantic-ai`) has not been materialised into the study workspace, so per the source-isolation rule ("You are studying exactly one selected source. You may ONLY access files inside that source's directory") no inspection of code, configuration, tests, or docs inside the project was possible. The accompanying manifest file `sources/pydantic-ai.ultraplan-source.yml:1-79` only declares metadata (name, URL, description, applicable dimensions) — it is not part of the source tree and cannot substitute for code evidence.

Interface contract design — central interfaces, protocols, abstract base classes, schemas, service contracts, dependency direction, error envelopes, cancellation, lifecycle methods, conformance suites, substitutability, and runtime/schema validation (`dimensions/24.02-interface-contract-design.md:9-13`) — cannot be evaluated when no package, no modules, no `__init__.py`, no `Protocol`/`ABC` declarations, no Pydantic model schemas, no type stubs, no tests, and no documentation exist in the inspected directory. There is no `Model` protocol (e.g. `pydantic_ai.models.Model`), no tool registration protocol (`pydantic_ai.tools.Tool`), no `RunContext` dependency-injection contract, no `AgentRun` lifecycle, no error/exception contract (`pydantic_ai.exceptions`), no provider adapter implementations (`pydantic_ai.models.openai`, `pydantic_ai.models.anthropic`, `pydantic_ai.models.bedrock`, `pydantic_ai.models.gemini`), and no conformance suite to inspect.

The analysis below therefore records the absence of evidence rather than fabricating findings. The search boundary was the directory itself plus the adjacent manifest file at `sources/pydantic-ai.ultraplan-source.yml:1-79`. Every path cited as missing is at the root of `studies/agent-harness-study/sources/pydantic-ai/`. Per the source-isolation rule, sibling sources (`sources/agent-framework`, `sources/crewai`, `sources/langfuse`, `sources/langgraph`, `sources/letta`, `sources/openhands`, `sources/openai-agents-sdk`, `sources/opa`, `sources/temporal`) were not consulted to substitute for evidence.

## Rating

**1 / 10 — Absent, implicit, ad-hoc, or unsafe.**

Rationale:
- The rubric band 1–3 covers "Absent, implicit, ad-hoc, or unsafe" contracts (`dimensions/24.02-interface-contract-design.md:32`). With no source files in the inspected directory there are no contracts to evaluate; the absence itself prevents assigning any credit for explicit interfaces, conformance tests, or validation.
- The dimension's central question — "Can two independent implementations satisfy the same contract without relying on undocumented behavior?" (`dimensions/24.02-interface-contract-design.md:39`) — cannot be answered affirmatively when no contract surface is observable. It can only be answered negatively: with no contracts visible, no two implementations can demonstrably satisfy them inside this study's evidence boundary.
- The description in `sources/pydantic-ai.ultraplan-source.yml:3` advertises "Type-system-centric agent design with validated structured outputs", which strongly implies a contract-rich surface (provider protocol, tool schema, structured-output validator, dependency-injection boundary). None of those contracts are visible in the inspected path.
- The 1/10 score reflects the rubric floor under the source-isolation constraint and is not a judgment on pydantic-ai itself; it is a judgment on the evidence available in `studies/agent-harness-study/sources/pydantic-ai/`.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Top-level contract surface | No `pyproject.toml`, `setup.py`, `setup.cfg`, `src/`, `pydantic_ai/`, `pydantic_ai_slim/`, or namespace package declaration present. Recursive `find` on the directory returns zero entries. | `studies/agent-harness-study/sources/pydantic-ai/:1` (directory empty) |
| Central interfaces / protocols | No `Protocol` classes, no `abc.ABC` subclasses, no abstract base classes, no `typing.Protocol` definitions, no trait objects, no `Generic[T]`-shaped contract types could be located. | `studies/agent-harness-study/sources/pydantic-ai/:1` (directory empty) |
| Provider adapter implementations | No `pydantic_ai/models/`, no `pydantic_ai/models/openai.py`, no `pydantic_ai/models/anthropic.py`, no `pydantic_ai/models/bedrock.py`, no `pydantic_ai/models/gemini.py`, no `pydantic_ai/models/groq.py`, no `pydantic_ai/models/mistral.py`, no `pydantic_ai/models/cohere.py`, no `pydantic_ai/models/test.py` / `FakeModel` could be located. | `studies/agent-harness-study/sources/pydantic-ai/:1` (directory empty) |
| Tool registration contract | No `pydantic_ai/tools.py`, no `Tool` class, no `ToolDefinition`, no `@agent.tool` / `@agent.tool_plain` decorator, no tool-as-agent composition (`Agent.as_tool`) could be located. | `studies/agent-harness-study/sources/pydantic-ai/:1` (directory empty) |
| Agent / run lifecycle contract | No `pydantic_ai/agent.py`, no `Agent` class, no `AgentRun`, no `RunContext[Deps]`, no `UsageLimits`, no `Run`, no `iter` / `run` / `run_sync` lifecycle methods could be located. | `studies/agent-harness-study/sources/pydantic-ai/:1` (directory empty) |
| Result / output contract | No `pydantic_ai/result.py`, no `RunResult`, no streaming `PartDeltaEvent`, no `FinalResult` could be located. | `studies/agent-harness-study/sources/pydantic-ai/:1` (directory empty) |
| Error / cancellation / lifecycle semantics | No `pydantic_ai/exceptions.py`, no `AgentError` / `ModelHTTPError` / `UnexpectedModelBehavior` / `UsageLimitExceeded` taxonomy, no cancellation token / `asyncio.CancelledError` handling could be located. | `studies/agent-harness-study/sources/pydantic-ai/:1` (directory empty) |
| Schema / validation logic | No Pydantic model schemas that act as runtime contracts (`ModelRequest`, `ModelResponse`, `SystemPromptPart`, `UserPromptPart`, `ToolCallPart`, `ToolReturnPart`), no JSON-schema validators, no `TypeAdapter` validation sites could be located. | `studies/agent-harness-study/sources/pydantic-ai/:1` (directory empty) |
| Contract tests / conformance suites | No `tests/`, no `test_*.py`, no `conftest.py`, no model-fixture conformance tests (e.g. a harness that runs every adapter against the same `ModelRequest`), no pytest configuration could be located. | `studies/agent-harness-study/sources/pydantic-ai/:1` (directory empty) |
| Type stability markers | No `py.typed` marker, no `*.pyi` stubs, no `@runtime_checkable` protocols, no `deprecated` decorators, no `__deprecated__` attributes, no `@public_api` / `@internal` / `@experimental` stability markers could be located. | `studies/agent-harness-study/sources/pydantic-ai/:1` (directory empty) |
| Documentation site & examples | No `docs/`, no `examples/`, no `README.md`, no MkDocs/Sphinx configuration, no doctest fixtures to anchor contract narrative. | `studies/agent-harness-study/sources/pydantic-ai/:1` (directory empty) |
| Build / packaging config | No `pyproject.toml`, `pdm.lock`, `uv.lock`, `poetry.lock`, `setup.py`, `requirements*.txt`, `MANIFEST.in`. | `studies/agent-harness-study/sources/pydantic-ai/:1` (directory empty) |
| Source manifest pointer | URL `https://github.com/pydantic/pydantic-ai` declared but not fetched into the source directory. | `sources/pydantic-ai.ultraplan-source.yml:2` |
| Dimension scope | This dimension (`24.02`) is listed as applicable (line 77), confirming the study intent to evaluate the interface contract surface of pydantic-ai. | `sources/pydantic-ai.ultraplan-source.yml:77` |
| Description anchor | Manifest describes pydantic-ai as "Type-system-centric agent design with validated structured outputs", which is the only available framing signal for what contracts *should* exist. | `sources/pydantic-ai.ultraplan-source.yml:3` |
| Dimension definition (template input only) | Interface Contract Design — purpose, steps, evidence to capture, questions, rubric | `dimensions/24.02-interface-contract-design.md:1-43` |
| Sibling-source cross-check (negative) | Sibling sources `agent-framework/`, `crewai/`, `langfuse/`, `langgraph/`, `letta/`, `openhands/`, `openai-agents-sdk/`, `opa/`, `temporal/` were NOT inspected under the source-isolation rule. Their contract surfaces (per their own 24.02 reports) cannot be used as substitutes for pydantic-ai evidence. | n/a (rule 1 prohibition) |

## Answers to Dimension Questions

1. **Are interfaces small, coherent, and owned by the consumer side?**
   No clear evidence found. The selected source contains no Python files, so no consumer-side interface ownership (`Protocol`, `ABC`, abstract base class, Pydantic-driven `Generic[T]` boundary, or `runtime_checkable` typed contract) can be inspected. The metadata sidecar only declares that the source is applicable to this dimension (`sources/pydantic-ai.ultraplan-source.yml:77`); it does not document an interface ownership model. Whether pydantic-ai follows consumer-side ISP-style ownership (each adapter implements a small `Model` protocol defined next to the agent runner) cannot be determined from the empty directory.

2. **Do contracts specify behavior, not just method signatures?**
   No clear evidence found. There are no method signatures, docstrings, behavioral specs, post-condition annotations, pre-condition validators, idempotency statements, ordering statements, or conformance assertions present in the inspected path. The dimension step on "behavior, not just method signatures" (`dimensions/24.02-interface-contract-design.md:11`) presupposes the existence of signatures — they are absent here. Whether the upstream `Model` protocol documents streaming semantics, token-budget semantics, retry semantics, or partial-output semantics cannot be evaluated.

3. **Can providers, tools, stores, and runtimes be replaced safely?**
   No clear evidence found. Without an inspectable contract surface there is no way to evaluate substitutability, swappable provider abstractions, swappable tool registries, swappable stores (e.g. message-history backends), or runtime adapters. The manifest description hints at a type-driven contract surface (`sources/pydantic-ai.ultraplan-source.yml:3`), which is suggestive of substitutability but does not constitute evidence. The sibling `openai-agents-sdk` 24.02 report (`reports/source/24.02-interface-contract-design/openai-agents-sdk.md`) reached the same conclusion for its own (also empty) source, confirming this gap is not unique to pydantic-ai but applies to multiple sources in this run.

4. **Are compatibility failures caught early by tests or validation?**
   No clear evidence found. There are no tests, fixtures, schema validators, Pydantic `TypeAdapter` validation sites, model-fixture conformance suites, or runtime validators in the inspected path. The dimension step on "compile-time, schema-time, or runtime contract validation" (`dimensions/24.02-interface-contract-design.md:13`) cannot be satisfied because no contracts exist in scope. Whether upstream ships a `TestModel` / `FunctionModel` conformance harness that exercises every adapter against canonical `ModelRequest`/`ModelResponse` fixtures cannot be determined.

## Architectural Decisions

No clear evidence found. The selected source directory does not contain any implementation files, configuration, manifests, or documentation from which architectural decisions about interface design could be extracted. The only artifact present is the sidecar metadata file (`sources/pydantic-ai.ultraplan-source.yml:1-79`), which is a study-scaffold manifest, not a design document.

The following decisions cannot be inspected because the source is absent:

- Whether the canonical `Model` contract is a `Protocol`, an `ABC`, a Pydantic-driven `BaseModel`, or a duck-typed shape.
- Whether providers live in flat sub-modules (`pydantic_ai.models.openai`) or in adapter packages (`pydantic_ai.models.openai.*`).
- Whether `pydantic_ai` and `pydantic_ai_slim` co-ship and how the contract surface is partitioned across them.
- Whether tools are registered via decorator, via explicit list, via class, or via schema.
- Whether streaming, structured output, retry, and async iteration each have a dedicated public type or share one protocol.
- Whether error envelopes are centralised in a single `exceptions.py` taxonomy or scattered per-adapter.
- Whether lifecycle hooks (`before_run`, `after_run`, tool-call interceptors) form a documented extension contract or are ad-hoc.
- Whether `py.typed` is shipped and whether `mypy --strict` is part of CI.

## Notable Patterns

No clear evidence found. There are no source files in `studies/agent-harness-study/sources/pydantic-ai` from which to infer patterns such as `Protocol`-based dependency inversion, Pydantic-schema-driven tool registration, consumer-owned interfaces, `runtime_checkable` validators, or conformance suites. The sidecar description (`sources/pydantic-ai.ultraplan-source.yml:3`) suggests a "type-system-centric" pattern but does not constitute evidence at the code level.

Patterns that cannot be cited because the source is absent:

- Whether Pydantic models are passed directly to `output_type=` (type-first output validation).
- Whether `Model` providers implement a common `ABC` or `Protocol` (provider-agnostic interface).
- Whether `RunContext[Deps]` is a `Generic[T]` dependency-injection boundary.
- Whether `@agent.tool` / `@agent.tool_plain` are decorator-first registration or imperative-first registration.
- Whether `Agent.as_tool` is a documented composition contract.
- Whether `agent.iter(...)` is a documented async-iterator streaming contract yielding typed `PartDeltaEvent` instances.
- Whether a `TestModel` / `FunctionModel` testing contract is shipped for conformance.
- Whether a `Graph` / state-machine contract is shipped for graph-style runs.

## Tradeoffs

No clear evidence found. Without a manifest, a module tree, or any contract surface inside the source directory, no tradeoffs can be cited. The following tradeoff axes cannot be evaluated:

- `Protocol` (structural) vs `ABC` (nominal) contract definition for providers.
- Single distribution (`pydantic-ai`) vs. split distribution (`pydantic-ai` + `pydantic-ai-slim`) and how contracts are partitioned.
- Flat namespace (`Agent`, `Tool`, `Model` at root) vs. sub-packages (`agent.Agent`, `tool.Tool`, `models.Model`).
- Consumer-owned interface placement vs. provider-owned interface placement.
- Lazy provider loading vs. eager provider loading (impacts import-time contract failures).
- Decorator-first vs. imperative-first tool registration (impacts contract surface size).
- Type-first output (`output_type=MyModel`) vs. function-first output (impacts validator surface).
- `py.typed` shipping vs. untyped surface (impacts downstream contract enforcement).
- Pydantic model validation at request/response boundaries vs. raw dicts + manual validation.

## Failure Modes / Edge Cases

No clear evidence found in the inspected source. The dimension calls for inspecting error contracts, cancellation, streaming, and lifecycle semantics (`dimensions/24.02-interface-contract-design.md:11-13`). No failure surfaces (exception hierarchies, error codes, cancellation tokens, lifecycle hooks, retry/backoff policies) exist in the inspected path to enumerate. The following observable failure modes of the *study workflow itself* are recorded instead:

- **Source not materialised.** The study workflow assumes the source has been cloned/copied into `studies/agent-harness-study/sources/pydantic-ai/`. In this run it has not. Any dimension that depends on this source (24.01 has already produced a no-evidence report at `reports/source/24.01-public-api-surface/pydantic-ai.md`; 24.02 produces this one) will hit the same gap.
- **Cross-source isolation blocks workaround.** Hard rule #1 forbids reading sibling sources (`../langfuse/`, `../openhands/`, `../openai-agents-sdk/`, etc.) to compensate, so the analysis must terminate at the empty-directory boundary.
- **No interface-contract evidence possible at 24.02.** Unlike 24.01 (public API surface), where the absence of files can sometimes be partially inferred from build configuration, 24.02 strictly requires inspecting contracts — `Protocol`, `ABC`, Pydantic `BaseModel` schemas, adapter implementations, conformance tests, validators. None of those artefacts are present, so the report cannot move beyond "absent".
- **Manifest description is not evidence.** The line at `sources/pydantic-ai.ultraplan-source.yml:3` ("Type-system-centric agent design with validated structured outputs") is a one-line marketing summary; treating it as architectural evidence would violate rule #3 ("Cite evidence, not vibes").

## Future Considerations

- Materialise `pydantic-ai` into `studies/agent-harness-study/sources/pydantic-ai/` (e.g., `git clone https://github.com/pydantic/pydantic-ai`) before running any dimension that requires code inspection.
- Once materialised, re-run this dimension to evaluate:
  - Central `Protocol` / `ABC` definitions (e.g. `Model`, `Tool`, `RunContext`, `Agent`) with file paths and line numbers, and ownership (consumer vs. provider side).
  - Adapter implementations under `pydantic_ai/models/<provider>/` (OpenAI, Anthropic, Bedrock, Gemini, Groq, Mistral, Cohere, etc.) and whether each is a drop-in substitute for the `Model` contract.
  - Pydantic-schema-driven tool registration (`Tool` / `ToolDefinition`), tool-as-agent composition (`Agent.as_tool`), and whether `@agent.tool` / `@agent.tool_plain` form an open or closed decorator contract.
  - Lifecycle methods on `Agent` / `AgentRun` (run, run_sync, iter, stream) and whether they form a documented lifecycle contract.
  - Error envelope (`pydantic_ai/exceptions.py`): `AgentError`, `ModelHTTPError`, `UnexpectedModelBehavior`, `UsageLimitExceeded`, `ValidationError` taxonomy; whether errors are typed and substitutable.
  - Cancellation / streaming / context-propagation contract across `ModelRequest`, `ModelResponse`, `RunContext`, `PartDeltaEvent`.
  - Conformance / contract tests: a `TestModel` / `FunctionModel` harness that exercises every adapter against canonical fixtures.
  - `py.typed` marker, `*.pyi` stubs, deprecation markers (`warnings.warn(..., DeprecationWarning, stacklevel=2)`), `@runtime_checkable` decorators.
  - Single vs split distribution (`pydantic-ai` + `pydantic-ai-slim`) and the contract-partitioning rationale.
- Consider a study-level pre-flight check that fails fast when a source directory is empty, instead of producing N "no evidence" reports across dimensions.
- Consider extending the manifest schema with a `materialisation_command` or `git_ref` field so the runner can clone on demand without violating source isolation.

## Questions / Gaps

- Why is the `pydantic-ai` source directory empty while sibling `langfuse/`, `openhands/`, `crewai/`, `langgraph/`, `letta/`, `opa/`, `openai-agents-sdk/`, `temporal/`, `agent-framework/` directories are populated (or in some cases also empty)? Is there a fetch step missing from the study bootstrap, or a per-source allowlist that excludes some sources?
- Is there an out-of-band mechanism (git submodule, archive download, monorepo path) the study expects the analyst to use? If so, it must be documented in the prompt because rule #1 forbids reaching outside the source directory.
- Should future prompts allow a "source unavailable — abort" exit code instead of forcing a low-score, no-evidence report across all dimensions that depend on the source?
- Is the dimension scope at `sources/pydantic-ai.ultraplan-source.yml:5-79` (which lists `24.02` as applicable) accurate given the source has not been fetched — i.e., was the source ever actually available for earlier dimensions, and was it lost between runs, or was it never fetched at all?
- Does the runner expect to read public pydantic-ai files from an upstream cache (e.g., `~/.cache/agent-harness-study/pydantic-ai`) that the prompt does not mention?
- What is the canonical `Model` contract in the upstream `pydantic-ai`? Is it a `Protocol`, an `ABC`, a Pydantic `BaseModel`, or a duck-typed shape? Cannot answer — upstream not materialized.
- Does pydantic-ai ship a `TestModel` / `FunctionModel` conformance harness that runs every provider adapter against canonical `ModelRequest` / `ModelResponse` fixtures? Cannot answer — no tests in scope.
- Does the upstream code ship a unified error taxonomy in `pydantic_ai/exceptions.py` (e.g. `AgentError` hierarchy) and are errors typed for substitution? Cannot answer — no error code paths in scope.
- Are cancellation, streaming, dependency-injection (`RunContext[Deps]`), and lifecycle methods part of the documented contract or only informally enforced? Cannot answer — no public API in scope.
- Does the project validate tool schemas at registration time or defer to runtime? Cannot answer — no registration code in scope.
- Does the upstream code ship `py.typed` and do `Agent` / `Tool` / `Model` carry `@runtime_checkable` semantics? Cannot answer — no contract surface in scope.

---

Generated by `dimensions/24.02-interface-contract-design.md` against `pydantic-ai`.
