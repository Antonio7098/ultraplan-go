# Source Analysis: crewai

## Interface Contract Design

### Source Info

| Field | Value |
|-------|-------|
| Name | crewai |
| Path | `studies/agent-harness-study/sources/crewai` |
| Language / Stack | Unknown — source directory is empty; manifest references `https://github.com/crewAIInc/crewAI` (Python multi-agent orchestration framework; expected primary stack: Python on top of LiteLLM/Provider abstractions, with `crewai` core package, `crewai-tools`, `crewai-flows`) |
| Analyzed | 2026-08-22 |

## Summary

The selected source directory `studies/agent-harness-study/sources/crewai` contains no files. Searched the directory recursively for files, subdirectories, hidden files, symlinks, and any contents — only the directory itself exists. The sibling manifest `studies/agent-harness-study/sources/crewai.ultraplan-source.yml` exists at line 1-87 and references `https://github.com/crewAIInc/crewAI`, but the manifest is metadata describing this study's plan, not part of the source itself and therefore off-limits for interface-contract evidence under the isolation rule. No source code, configuration, package manifests, public interface definitions, examples, conformance suites, or documentation files are present to inspect. Consequently, no claims about interface contract design in crewai (central protocols, abstract base classes, error/cancellation/lifecycle semantics, adapter substitutability, validation logic) can be substantiated from local evidence.

Search boundary: `find studies/agent-harness-study/sources/crewai -type f` returned zero results; `find … -type d` returned only the source root itself; `ls -la` confirms a single empty directory entry (`.` and `..` only, no `README`, no `pyproject.toml`, no `setup.py`, no `requirements.txt`, no `package.json`, no source tree, no `docs/`, no `examples/`, no `tests/`, no `LICENSE`). No `src/`, no `crewai/`, no `crewai_tools/`, no `flows/`, no `tests/`, no `contracts/` directory exists.

## Rating

**Score: 1 / 10 — Absent.**

Rationale (per the dimension rubric): the interface contract surface is absent from the inspection boundary because the source material itself is absent. A score of 1 is warranted under the rubric band "Absent, implicit, ad-hoc, or unsafe." Without any local artifacts to inspect, the dimension cannot be evaluated for interface size, dependency direction, error/cancellation/lifecycle semantics, substitutability of providers/tools/stores/runtimes, contract tests or conformance suites, compile-time or runtime contract validation, or whether contracts encode semantic guarantees versus only structural shape. A higher score is not defensible: there is no interface contract to grade, only an empty source directory.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Source presence | `find studies/agent-harness-study/sources/crewai -type f` returned zero results; directory listing contains only `.` and `..` | `studies/agent-harness-study/sources/crewai/:1` (directory entry) |
| Manifest reference (metadata only, not source) | The source manifest names the upstream URL `https://github.com/crewAIInc/crewAI` and lists applicable dimensions; this file is the study's planning metadata, not source code | `sources/crewai.ultraplan-source.yml:2` |
| Central interfaces / protocols / abstract base classes | No clear evidence found — no `Protocol`/`ABC`/`interface` declarations, no `BaseTool`/`BaseLLM`/`BaseMemory`/`BaseAgent`/`BaseCrew`/`BaseTask`/`BaseFlow`/`BaseEvent` definitions exist in the selected source directory | n/a (no file present) |
| Adapter implementations | No clear evidence found — no LLM provider adapters, tool adapters, memory store adapters, or storage backend adapters exist locally; no `crewai/llms/`, `crewai/tools/`, `crewai/memory/storage/`, `crewai/utilities/`, or `crewai/flows/` tree is present | n/a (no file present) |
| Contract tests / conformance suites | No clear evidence found — no `tests/contract/`, no `tests/conformance/`, no `pytest.mark.parametrize` over alternate implementations, no `assert` against protocol behavior, no property-based tests, no `hypothesis` strategies exist | n/a (no file present) |
| Error, cancellation, streaming, lifecycle semantics | No clear evidence found — no exception hierarchy, no `AgentError`/`CrewError`/`ToolError`/`LLMError` types, no cancellation token/context, no streaming protocol, no `start`/`stop`/`close`/`reset` lifecycle methods exist in the selected source directory | n/a (no file present) |
| Validation logic for implementations or schemas | No clear evidence found — no Pydantic validators, no `__init__` argument validation, no `register` decorators that enforce interface conformance, no schema-time or runtime `isinstance`/`runtime_checkable` checks exist | n/a (no file present) |
| Schema definitions (tool schemas, message schemas, config schemas) | No clear evidence found — no JSON Schema, no OpenAPI, no Pydantic models, no `ToolSchema`, no `AgentSchema`, no `CrewSchema`, no `FlowSchema` exist in the selected source directory | n/a (no file present) |
| Context propagation / dependency injection | No clear evidence found — no `ToolContext`, no `AgentContext`, no `FlowContext`, no `Depends()`-style injection, no scoped resources, no `with`-statement lifecycle for adapters exist locally | n/a (no file present) |
| Compile-time contract enforcement | No clear evidence found — no `typing.Protocol` with `runtime_checkable`, no ABC method enforcement, no overloaded type hints, no `ParamSpec`, no `TypeVar`-bounded generics exist | n/a (no file present) |

## Answers to Dimension Questions

1. **Are interfaces small, coherent, and owned by the consumer side?**
   No clear evidence found. The selected source directory is empty; there are no `Protocol`/`ABC` declarations, no `BaseTool`/`BaseLLM`/`BaseMemory`/`BaseAgent`/`BaseCrew`/`BaseTask`/`BaseFlow` definitions, no `__all__` discipline, and no consumer-side ownership markers (e.g., a `crewai.contracts` package re-exporting only what harnesses should depend on) present locally. Whether upstream crewai uses the "consumer-owned interface" pattern (where `crewai.tools` defines the `BaseTool` protocol that `crewai-tools` adapters implement) cannot be confirmed from this study.

2. **Do contracts specify behavior, not just method signatures?**
   No clear evidence found. With no source files present, no behavioral contract can be observed — no docstring contracts (`"""Must return … within N seconds"""`), no formal specification of retry/backoff/idempotency expectations, no pre/post-condition language, no invariant assertions on adapter registration, no "Protocol with semantic guarantees" beyond method names. The expected crewai behavior of `Crew.kickoff()` (delegation, hierarchical manager invocation, memory write-through, tool dispatch) cannot be evidenced locally.

3. **Can providers, tools, stores, and runtimes be replaced safely?**
   No clear evidence found. No adapter pattern, no `LLM`/`Tool`/`Memory`/`Storage`/`EventBus`/`Serializer` protocol surface, no `register(...)` or `set_default(...)` plugin entry points, no LiteLLM-style provider abstraction, no `@tool` decorator that gates conformance, no "drop-in replacement" tests are present locally. Whether two independent LLM providers (OpenAI, Anthropic, Ollama, Bedrock) can satisfy the same `BaseLLM` contract without relying on undocumented behavior cannot be evaluated.

4. **Are compatibility failures caught early by tests or validation?**
   No clear evidence found. No test suite, no contract tests, no conformance test matrix, no CI gating of interface compatibility, no schema diff, no Pydantic migration check, no `mypy --strict` enforcement of protocol members, no `pytest.fixture`-based alternate-implementation parametrization exists in the selected source. Whether crewai ships a `tests/contract/` directory that runs every adapter against a shared harness of behavioral assertions cannot be confirmed.

## Architectural Decisions

No clear evidence found. No source files, configuration, manifests, or documentation are present in the selected source directory to identify architectural decisions about interface boundaries (central protocols, abstract bases, consumer-owned vs. producer-owned interfaces), validation strategy (compile-time `Protocol` vs. runtime `isinstance` vs. schema-time JSON Schema), error/cancellation/lifecycle modeling, or substitutability guarantees. Upstream knowledge (off-limits) suggests crewai historically relies on Pydantic models for the data contract surface (`Agent`, `Task`, `Crew`, `Tool` are Pydantic `BaseModel` subclasses), LiteLLM for the LLM provider abstraction, and a `BaseTool` ABC for the tool surface; but none of this can be cited from local files.

## Notable Patterns

No clear evidence found. No patterns (Protocol/ABC interface segregation, Adapter pattern, Registry/Plugin pattern, Consumer-Defined Interface, Null Object, Decorator for tool wrapping, Strategy for LLM provider selection, Circuit Breaker for retry/cancellation, Context Manager for resource lifecycle, Builder for Crew/Agent composition, etc.) can be observed because no source code is present. Whether `crewai` ships a `BaseTool.run(..., context)` signature with a `ToolContext` injection parameter — the pattern that would indicate a deliberately scoped interface — cannot be evaluated.

## Tradeoffs

No clear evidence found. Without source material, no tradeoff discussion (interface size vs. expressiveness, structural vs. behavioral contracts, compile-time vs. runtime validation overhead, consumer-owned vs. producer-owned interface discipline, broad pluggability vs. core-team-controlled conformance) is grounded in evidence. Upstream tradeoff that would normally be evaluated here — Pydantic-driven data contracts (heavy runtime validation, strong ergonomics) vs. `typing.Protocol` (cheap at runtime, weaker external conformance) — cannot be examined.

## Failure Modes / Edge Cases

No clear evidence found. No interface definitions, validation logic, error envelopes, Pydantic model validators, protocol conformance checks, or runtime guard rails exist locally to study failure modes. The only observable failure mode is at the study-input layer: an empty source directory prevents evidence-based analysis of the dimension at all. A second-order failure mode worth flagging: an empty source for a dimension that depends on cross-cutting interface-contract observations also blocks downstream dimensions (e.g., 04.01 tool registration contract, 04.02 tool schema validation, 04.06 tool result contract) for crewai unless the source is populated first. A third failure mode relevant to this dimension specifically: when a framework exposes multiple "parallel" abstractions (e.g., `Crew` vs. `Flow` in crewai) the risk of undocumented behavioral overlap (which runtime owns delegation, which owns memory write-through, which owns cancellation) cannot be assessed without the source.

## Future Considerations

If the source directory is populated (e.g., via `git clone https://github.com/crewAIInc/crewAI` into `studies/agent-harness-study/sources/crewai/`), the analysis should be re-run. Specifically, re-inspect:

- The `BaseTool` abstract base class or `Tool` Protocol in `crewai/tools/`: method surface (`run`, `_run`, `name`, `description`, `args_schema`), Pydantic `args_schema` enforcement, error contract on `run` (does it raise or return a `ToolResult` envelope?), async/sync duality
- The `BaseLLM` interface in `crewai/llms/` (or the LiteLLM wrapper layer): `call`, `stream`, `acall`, `astream`, token accounting, retry/backoff hooks, function-calling/tool-calling schema propagation
- The `BaseMemory` / `Memory` interface in `crewai/memory/`: `save`, `search`, `reset`, `load`, plus the storage-backend split (`Storage` protocol for short-term, long-term, entity, external memory)
- The `Agent` and `Crew` Pydantic models in `crewai/agent.py`, `crewai/crew.py`: field validation, cross-field validators (e.g., `allow_delegation=True` requires hierarchical `Process`), error envelopes raised by `kickoff`
- The `Process` enum and its `sequential` / `hierarchical` implementations: whether the execution strategy is an injected Strategy object (substitutable) vs. hard-coded branches inside `Crew.kickoff` (non-substitutable)
- The `Flow`, `start`, `listen`, `and_`, `or_`, `persist` surface in `crewai.flows` / `crewai/flow/`: state machine contract, event payload schema, persistence boundary, restart semantics
- Conformance tests: `tests/contract/test_base_tool_compliance.py`, `tests/contract/test_llm_provider_substitutability.py`, `tests/contract/test_memory_store_substitutability.py`
- Cancellation and context propagation: whether `kickoff` accepts a `CancellationToken` / `asyncio.CancelledError` propagates through tool and LLM calls, whether `ToolContext` carries request-scoped state (memory handles, loggers, tracers)
- Streaming contract: whether `kickoff(inputs, callbacks=...)` or `kickoff(..., stream=True)` emits a typed event stream (`AgentAction`, `ToolResult`, `LLMStreamChunk`, `CrewFinalOutput`) versus ad-hoc dict/str emissions
- Validation of user-supplied adapters: `register(...)` decorators, `isinstance` checks, `runtime_checkable` Protocols, schema-time validation of `args_schema`

## Questions / Gaps

- Was the upstream repository `https://github.com/crewAIInc/crewAI` expected to be cloned into `studies/agent-harness-study/sources/crewai/` before dimension tasks were dispatched? The selected source directory is empty, while sibling sources (`langfuse`, `openhands`) were cloned with commits visible in `git status`.
- Should the harness study runner pre-clone source repositories before scheduling dimension tasks, or is the empty directory an intentional placeholder to be filled by a later step?
- Is the upstream repository even publicly accessible at the URL recorded in `sources/crewai.ultraplan-source.yml:2`? No remote fetch was performed under the isolation rule.
- Without local source, every dimension question against `crewai` is unanswerable. The orchestration layer should treat empty source directories as a hard pre-condition failure rather than dispatching dimension tasks.
- crewai has historically distinguished between `crewai` (orchestration), `crewai-tools` (tool catalog), and `crewai-flows` / `crewai.flow` (event-driven state machines) as separate distributions. The study runner should pre-decide whether the dimension analyzes all three subpackages or only the head `crewai` package, since the answer materially changes the interface-contract evaluation — `crewai-tools` is the boundary where consumer-defined tool interfaces meet producer-supplied adapters, and `crewai-flows` introduces a second, parallel interface surface that may overlap or conflict with `Crew` semantics.
- The dimension question "Can two independent implementations satisfy the same contract without relying on undocumented behavior?" cannot be evaluated without a conformance test suite, an interface declaration, and at least two adapter implementations present in the selected source.

---

Generated by `24.02-interface-contract-design` against `crewai`.
