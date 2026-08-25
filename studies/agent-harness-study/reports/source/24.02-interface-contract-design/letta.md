# Source Analysis: letta

## Interface Contract Design

### Source Info

| Field | Value |
|-------|-------|
| Name | letta |
| Path | `studies/agent-harness-study/sources/letta` |
| Language / Stack | Unknown — source directory is empty; manifest references `https://github.com/letta-ai/letta` (formerly MemGPT; memory-first agent architecture; primary stack is Python with FastAPI server, SQLAlchemy persistence, Pydantic schemas, multi-LLM provider adapters, archival/context recall memory managers, agent loop, and a `Letta`/`Agent`/`Memory`/`Tool`/`LLM`/`Persistence`/`Block`/`Passage`/`ArchivalMemory`/`RecallMemory` style contract surface per upstream public docs) |
| Analyzed | 2026-08-22 |

## Summary

The selected source directory `studies/agent-harness-study/sources/letta` contains no files. Searched the directory recursively for files, subdirectories, hidden files, symlinks, and any contents — only the directory itself exists. The sibling manifest `studies/agent-harness-study/sources/letta.ultraplan-source.yml:1-75` exists and references `https://github.com/letta-ai/letta`, but the manifest is metadata describing this study's plan, not part of the source itself and therefore off-limits for interface-contract evidence under the isolation rule. No source code, configuration, package manifests, public API definitions, examples, conformance suites, or documentation files are present to inspect. Consequently, no claims about the interface contract design of letta (central `Agent`/`Memory`/`Tool`/`LLM`/`Persistence` interfaces, Pydantic schema contracts, tool-call envelopes, memory/archival/recall contracts, error/cancellation/streaming semantics, adapter substitutability across LLM providers, schema validation) can be substantiated from local evidence.

Search boundary: `find studies/agent-harness-study/sources/letta -type f` returned zero results; `find … -type d` returned only the source root itself; `ls -la` confirms a single empty directory entry (`.` and `..` only, no `README`, no `pyproject.toml`, no `setup.py`, no `requirements.txt`, no `package.json`, no `tsconfig.json`, no source tree, no `docs/`, no `examples/`, no `LICENSE`). No `letta/`, no `letta/agent/`, no `letta/memory/`, no `letta/schemas/`, no `letta/llm/`, no `letta/persistence/`, no `tests/`, no `conformance/`, no `libs/` directory exists. The dimension's central objects of study — interfaces, protocols, abstract base classes, trait objects, schemas, and service contracts — are all absent from the inspection boundary.

## Rating

**Score: 1 / 10 — Absent.**

Rationale (per the dimension rubric): interface contracts are absent from the inspection boundary because the source material itself is absent. A score of 1 is warranted under the rubric band "Absent, implicit, ad-hoc, or unsafe." Without any local artifacts to inspect, the dimension cannot be evaluated for interface size, dependency direction, error contracts, context propagation, cancellation semantics, lifecycle methods, substitutability, or compile-time/schema-time/runtime contract validation. A higher score is not defensible: there are no contracts to grade, only an empty source directory.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Source presence | `find studies/agent-harness-study/sources/letta -type f` returned zero results; directory listing contains only `.` and `..` | `studies/agent-harness-study/sources/letta/:1` (directory entry) |
| Manifest reference (metadata only, not source) | The source manifest names the upstream URL `https://github.com/letta-ai/letta` and lists applicable dimensions; this file is the study's planning metadata, not source code | `sources/letta.ultraplan-source.yml:2` |
| Interface / protocol / abstract base class definitions | No clear evidence found — no `.py`, `.ts`, `.ipynb`, or `.pyx` files exist; no `class …(Protocol)`, `abstractmethod`, `interface`, or `type` declarations are present | n/a (no file present) |
| Adapter implementations | No clear evidence found — no LLM provider adapter files exist (e.g., no `OpenAILLM`, `AnthropicLLM`, `GoogleLLM`, `AzureOpenAILLM`, `OllamaLLM` adapters; no `PostgresPersistence`, `SQLitePersistence`, `RedisPersistence` adapters; no `Tool`/`ToolCall` adapters) | n/a (no file present) |
| Contract tests / conformance suites | No clear evidence found — no `tests/`, `__tests__`, conformance fixtures, golden files, or memory/archival/recall test scenarios exist | n/a (no file present) |
| Error, cancellation, streaming, and lifecycle semantics | No clear evidence found — no `LettaError`/`AgentError`/`MemoryError`/`LLMError`/`ToolError`/`PersistenceError`/`TokenLimitError`/`ContextWindowError`/`InvalidToolCallError` symbols, no `StreamingResponse`/`ServerSentEvent`/context-manager lifecycle, no `agent.step`/`agent.stream`/`agent.compile` lifecycle methods exist | n/a (no file present) |
| Validation logic (compile-time, schema-time, runtime) | No clear evidence found — no `pydantic` `BaseModel` schemas for `Memory`/`ArchivalMemory`/`RecallMemory`/`ToolCall`/`Message`/`AgentState`, no `typing.Protocol`/`runtime_checkable` markers, no JSON-Schema/OpenAPI definitions, and no Pydantic `validator`/`model_validator` rules exist | n/a (no file present) |
| Documentation tied to contract design | No clear evidence found — no `README`, no `docs/`, no `examples/`, no ADRs, no MIGRATION notes exist in the selected source directory | n/a (no file present) |
| Consumer-side ownership markers | No clear evidence found — no `__all__` exports, no `py.typed`, no `Protocol` declared at consumer side, no `@runtime_checkable`, no re-export boundaries exist | n/a (no file present) |

## Answers to Dimension Questions

1. **Are interfaces small, coherent, and owned by the consumer side?**
   No clear evidence found. The selected source directory is empty; there are no interfaces, protocols, abstract base classes, trait objects, or schemas present locally to evaluate size, coherence, or ownership direction. Whether letta places interface ownership on the consumer side (e.g., whether users supply their own `Protocol` for `Memory`, whether `Memory`/`ArchivalMemory`/`RecallMemory` types are declared where the user instantiates them versus in `letta.memory`) cannot be verified from this study. Whether the framework keeps contracts narrow (e.g., a small `LLM`/`Tool`/`Persistence` surface) versus exposing large façade classes cannot be assessed.

2. **Do contracts specify behavior, not just method signatures?**
   No clear evidence found. With no source files present, no behavioral contracts, docstrings, invariants, pre/post-conditions, type-level guarantees, memory-tier semantic guarantees, archival-recall consistency guarantees, or tool-call envelope semantics can be observed. Whether letta encodes semantic guarantees (e.g., "a `RecallMemory` `insert` is durable before the next `agent.step` observes it", "an `ArchivalMemory` `passage` round-trips losslessly through `passage.text`", "a `ToolCall` envelope maps 1:1 to provider function-call JSON", "context-window eviction follows the configured `Memory` `limit` policy and never silently drops messages") cannot be assessed.

3. **Can providers, tools, stores, and runtimes be replaced safely?**
   No clear evidence found. No substitutability evidence — no `LLM` adapter registry, no `Tool` indirection, no `Persistence`/`ArchivalMemory`/`RecallMemory` indirection, no `Memory` tier indirection, no `Agent` runtime swappable backend, no provider abstraction layer — exists locally. Whether two independent implementations (e.g., an in-memory `Memory` versus a Postgres-backed `Memory`; an `OpenAI` LLM adapter versus an `Anthropic`/`Ollama`/`Azure`/`Google` adapter; a built-in `Tool` versus a user-supplied `Tool`; a local agent loop versus a server-hosted `Letta` runtime) can satisfy the same contract without relying on undocumented behavior is unverifiable from this study.

4. **Are compatibility failures caught early by tests or validation?**
   No clear evidence found. No test files, conformance suites, contract tests, golden file comparisons, schema validation harnesses, snapshot fixtures, or CI configuration exist in the selected source to demonstrate that compatibility failures are caught early. Whether the upstream repository ships a conformance matrix across LLM providers (`OpenAI`/`Anthropic`/`Google`/`Azure`/`Ollama`), across persistence backends (`Postgres`/`SQLite`/`Redis`), across memory tiers (`Core`/`Recall`/`Archival`), or across tool adapters is unknown from local evidence.

## Architectural Decisions

No clear evidence found. No source files, configuration, manifests, or documentation are present in the selected source directory to identify architectural decisions about interface segregation, dependency inversion, error envelope shape, cancellation propagation, lifecycle ownership, or schema versioning.

## Notable Patterns

No clear evidence found. No patterns (consumer-defined interfaces, port-and-adapter, hexagonal architecture, capability providers, schema-first design, `Protocol`/`runtime_checkable` boundaries, `pydantic` model contracts, `LLM`/`Tool`/`Memory`/`Persistence`/`Agent` composability, multi-tier memory model (`Core`/`Recall`/`Archival`), tool-call envelope mapping, streaming `ServerSentEvent`/`StreamingResponse` lifecycle, context-window eviction policies, host-application SDK layering) can be observed because no source code is present.

## Tradeoffs

No clear evidence found. Without source material, no tradeoff discussion (e.g., narrow `LLM` contract versus richer convenience methods, structural `Protocol` typing versus nominal ABC inheritance, schema-strictness via `pydantic` versus flexibility of `TypedDict`, runtime validation in `Memory`/`ToolCall` versus compile-time guarantees, sync-versus-async agent loop contracts, in-process versus server-hosted agent runtime contracts) is grounded in evidence.

## Failure Modes / Edge Cases

No clear evidence found. No interface definitions, validation logic, error envelopes, streaming/lifecycle semantics, context-window eviction edge cases, archival-recall consistency under failure, tool-call envelope edge cases, or deprecation markers exist locally to study failure modes. The only observable failure mode is at the study-input layer: an empty source directory prevents evidence-based analysis of the dimension at all.

## Future Considerations

If the source directory is populated (e.g., via `git clone https://github.com/letta-ai/letta` into `studies/agent-harness-study/sources/letta/`), the analysis should be re-run. Specifically, re-inspect:

- The `letta.agent.Agent` and `letta.client.Letta`/`AsyncLetta`/`LocalClient`/`RESTClient` surfaces for explicit Pydantic contracts versus runtime-attribute stores; whether `Agent` `memory`, `tools`, `system`, and `llm_config` are required to be typed.
- The `letta.memory` tier surface (`CoreMemory`, `RecallMemory`, `ArchivalMemory`) and whether each tier declares a small Protocol/ABC versus a single concrete façade; whether cross-tier consistency (e.g., "archival passage insert is durable before recall sees it") is part of the contract.
- The `letta.llm` provider adapter interface (e.g., `LLM.chat`/`LLM.stream`/`LLM.embed`) and whether the contract is narrow enough to allow third-party adapters (e.g., `vLLM`, `Together`, `Bedrock`, `OpenRouter`) without undocumented coupling to provider-specific function-call shapes.
- The `letta.schemas` Pydantic models for `Message`, `ToolCall`, `ToolReturn`, `Passage`, `Block`, `Memory`, `AgentState`, `LLMConfig`: whether error and tool-call types form stable, discriminated unions with documented semantics.
- The `letta.tools` and tool-call envelope mapping: whether `ToolCall`/`ToolReturn`/`Tool` defines an explicit `BaseTool`/`Tool` interface, whether user-defined tools are first-class, whether host-supplied tools (via SDK) cross the same envelope.
- The `letta.errors` exception hierarchy (`LettaError`/`AgentError`/`MemoryError`/`LLMError`/`ToolError`/`PersistenceError`/`TokenLimitError`/`ContextWindowError`/`InvalidToolCallError`): whether error types form a stable, documented hierarchy versus ad-hoc string errors.
- Whether `py.typed` is shipped so static type checkers (`mypy --strict`) can enforce the `Agent`/`Memory`/`ToolCall`/`LLMConfig` contracts at compile time.
- Whether the upstream ships OpenAPI/JSON-Schema for the REST surface (`/v1/agents`, `/v1/agents/{id}/messages`, `/v1/agents/{id}/memory`, `/v1/tools`, `/v1/archival-memory`, `/v1/recall-memory`, `/v1/blocks`) consumable by external clients; whether the schema is the canonical contract.
- Whether a conformance suite (e.g., `tests/conformance/`, `tests/llm_providers/`, `tests/memory_tiers/`) runs every LLM provider, persistence backend, and memory tier against the same scenario matrix.
- Whether the streaming `ServerSentEvent`/`StreamingResponse` lifecycle (`message_created`/`tool_call`/`tool_return`/`message_delta`/`message_done`/`error`/`done` event types) is documented as part of the public contract or as implementation detail.
- Whether `interrupt`/pause/resume semantics, context-window eviction policies, and `agent.step`/`agent.stream`/`agent.compile` lifecycle hooks are part of the public contract or implementation detail.
- Whether the `Letta` Python SDK, REST client, and any TypeScript/JS SDK (`@letta-ai/letta-client`) expose consistent contracts across languages, and how divergence is handled.

## Questions / Gaps

- Was the upstream repository `https://github.com/letta-ai/letta` expected to be cloned into `studies/agent-harness-study/sources/letta/` before dimension tasks were dispatched? The selected source directory is empty, while sibling sources (`langfuse`, `openhands`) were cloned with commits visible in `git status`.
- Should the harness study runner pre-clone source repositories before scheduling dimension tasks, or is the empty directory an intentional placeholder to be filled by a later step?
- Is the upstream repository accessible at the URL recorded in `sources/letta.ultraplan-source.yml:2`? No remote fetch was performed under the isolation rule.
- Without local source, every dimension question against `letta` is unanswerable. The orchestration layer should treat empty source directories as a hard pre-condition failure rather than dispatching dimension tasks.
- Whether the upstream `letta` ships a TypeScript sibling (`@letta-ai/letta-client`/`@letta-ai/letta-react`) with its own contract surface, and whether the contracts are kept in sync across languages, is unknown from this study; this matters because consumer-defined interfaces differ between Python (`Protocol`/`runtime_checkable`, Pydantic discriminated unions) and TypeScript (structural `interface`/`type` with optional nominal brands).
- Whether the upstream ships Rust, Go, Java, or .NET bindings for `letta` client/server contract is unknown from this study.
- Whether `letta` (the server runtime) has a different contract surface than the in-process `Agent` runtime — and whether that distinction is documented as part of the public contract — is unknown.
- Whether `letta-cloud` (the hosted offering) exposes a contract surface distinct from the open-source server, and how divergence is versioned, is unknown from this study.

---

Generated by `24.02-interface-contract-design` against `letta`.
