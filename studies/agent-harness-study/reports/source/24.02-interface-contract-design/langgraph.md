# Source Analysis: langgraph

## Interface Contract Design

### Source Info

| Field | Value |
|-------|-------|
| Name | langgraph |
| Path | `studies/agent-harness-study/sources/langgraph` |
| Language / Stack | Unknown — source directory is empty; manifest references `https://github.com/langchain-ai/langgraph` (LangChain's durable graph execution library; primary stack is Python with a TypeScript sibling, expected to expose a `Pregel`-style runtime, `StateGraph`/`MessageGraph` graph builders, checkpoint/`BaseCheckpointSaver` persistence interfaces, `interrupt`/`Command`/`Send` runtime primitives, and tool/node-typed contracts) |
| Analyzed | 2026-08-22 |

## Summary

The selected source directory `studies/agent-harness-study/sources/langgraph` contains no files. Searched the directory recursively for files, subdirectories, hidden files, symlinks, and any contents — only the directory itself exists. The sibling manifest `studies/agent-harness-study/sources/langgraph.ultraplan-source.yml:1-119` exists and references `https://github.com/langchain-ai/langgraph`, but the manifest is metadata describing this study's plan, not part of the source itself and therefore off-limits for interface-contract evidence under the isolation rule. No source code, configuration, package manifests, public API definitions, examples, conformance suites, or documentation files are present to inspect. Consequently, no claims about the interface contract design of langgraph (central `Pregel`/`StateGraph` interfaces, `BaseCheckpointSaver`/`BaseStore`/`BaseCache` abstract base classes, `Runnable`/`RunnableConfig`/`RunnableLambda` contracts, node/tool/channel contracts, error/cancellation/interrupt semantics, adapter substitutability, schema validation) can be substantiated from local evidence.

Search boundary: `find studies/agent-harness-study/sources/langgraph -type f` returned zero results; `find … -type d` returned only the source root itself; `ls -la` confirms a single empty directory entry (`.` and `..` only, no `README`, no `pyproject.toml`, no `setup.py`, no `requirements.txt`, no `package.json`, no `tsconfig.json`, no source tree, no `docs/`, no `examples/`, no `LICENSE`). No `langgraph/`, no `langgraph/pregel/`, no `langgraph/graph/`, no `langgraph/checkpoint/`, no `langgraph/store/`, no `tests/`, no `conformance/`, no `libs/` directory exists. The dimension's central objects of study — interfaces, protocols, abstract base classes, trait objects, schemas, and service contracts — are all absent from the inspection boundary.

## Rating

**Score: 1 / 10 — Absent.**

Rationale (per the dimension rubric): interface contracts are absent from the inspection boundary because the source material itself is absent. A score of 1 is warranted under the rubric band "Absent, implicit, ad-hoc, or unsafe." Without any local artifacts to inspect, the dimension cannot be evaluated for interface size, dependency direction, error contracts, context propagation, cancellation semantics, lifecycle methods, substitutability, or compile-time/schema-time/runtime contract validation. A higher score is not defensible: there are no contracts to grade, only an empty source directory.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Source presence | `find studies/agent-harness-study/sources/langgraph -type f` returned zero results; directory listing contains only `.` and `..` | `studies/agent-harness-study/sources/langgraph/:1` (directory entry) |
| Manifest reference (metadata only, not source) | The source manifest names the upstream URL `https://github.com/langchain-ai/langgraph` and lists applicable dimensions; this file is the study's planning metadata, not source code | `sources/langgraph.ultraplan-source.yml:2` |
| Interface / protocol / abstract base class definitions | No clear evidence found — no `.py`, `.ts`, `.ipynb`, or `.pyx` files exist; no `class …(Protocol)`, `abstractmethod`, `interface`, or `type` declarations are present | n/a (no file present) |
| Adapter implementations | No clear evidence found — no provider/checkpointer/store adapter files exist (e.g., no `MemorySaver`, `PostgresSaver`, `SqliteSaver`, `InMemoryStore`, `AsyncPostgresStore` implementations, no `langgraph-sdk`, `langgraph-api` adapters) | n/a (no file present) |
| Contract tests / conformance suites | No clear evidence found — no `tests/`, `__tests__`, conformance fixtures, golden files, or graph-fixture `.json`/`.yaml` files exist | n/a (no file present) |
| Error, cancellation, streaming, and lifecycle semantics | No clear evidence found — no `GraphInterrupt`, `NodeInterrupt`, `ParentCommand`, `PregelTask`, `StreamWriter`, `CachePolicy`, or `Durability`/`RetryPolicy` symbols exist | n/a (no file present) |
| Validation logic (compile-time, schema-time, runtime) | No clear evidence found — no `pydantic` models, `typing.Protocol`/`runtime_checkable` markers, `StateGraph`/`MessagesState` schemas, JSON-Schema definitions, or channel/branch validators exist | n/a (no file present) |
| Documentation tied to contract design | No clear evidence found — no `README`, no `docs/`, no `examples/`, no ADRs, no MIGRATION notes exist in the selected source directory | n/a (no file present) |
| Consumer-side ownership markers | No clear evidence found — no `__all__` exports, no `py.typed`, no `Protocol` declared at consumer side, no `@runtime_checkable`, no re-export boundaries exist | n/a (no file present) |

## Answers to Dimension Questions

1. **Are interfaces small, coherent, and owned by the consumer side?**
   No clear evidence found. The selected source directory is empty; there are no interfaces, protocols, abstract base classes, trait objects, or schemas present locally to evaluate size, coherence, or ownership direction. Whether langgraph places interface ownership on the consumer side (e.g., whether users supply their own `Protocol` for `State`, whether `StateGraph` types are declared where the user instantiates them versus in `langgraph.graph.state`) cannot be verified from this study. Whether the framework keeps contracts narrow (e.g., a small `BaseCheckpointSaver`/`BaseStore` surface) versus exposing large façade classes cannot be assessed.

2. **Do contracts specify behavior, not just method signatures?**
   No clear evidence found. With no source files present, no behavioral contracts, docstrings, invariants, pre/post-conditions, type-level guarantees, `Durability`/`RetryPolicy`/`CachePolicy` contracts, or thread/queue semantic guarantees can be observed. Whether langgraph encodes semantic guarantees (e.g., "a `Pregel` step is at-least-once and idempotent under the configured `RetryPolicy`", "a `BaseCheckpointSaver.put` is durable before the next `Pregel.tick` advances", "an `interrupt()` payload round-trips losslessly through `Command(resume=…)`", "a `Send` payload is broadcast to subgraph channels without re-running the parent") cannot be assessed.

3. **Can providers, tools, stores, and runtimes be replaced safely?**
   No clear evidence found. No substitutability evidence — no `BaseCheckpointSaver` registry, no `BaseStore` indirection, no `BaseCache` indirection, no `langgraph-sdk`/`langgraph-api` adapter layer, no `Pregel` runtime swappable backend, no tool/callback handler protocol — exists locally. Whether two independent implementations (e.g., an in-process `MemorySaver` versus a remote `PostgresSaver`; an in-memory store versus a Postgres-vector store; a local executor versus a remote `langgraph-api` deployment) can satisfy the same contract without relying on undocumented behavior is unverifiable from this study.

4. **Are compatibility failures caught early by tests or validation?**
   No clear evidence found. No test files, conformance suites, contract tests, golden file comparisons, schema validation harnesses, snapshot fixtures, or CI configuration exist in the selected source to demonstrate that compatibility failures are caught early. Whether the upstream repository ships a conformance matrix across checkpointer implementations (`MemorySaver`/`SqliteSaver`/`PostgresSaver`/`AsyncPostgresSaver`), across store backends (`InMemoryStore`/`PostgresStore`), or across SDKs (`langgraph-sdk` Python/JS) is unknown from local evidence.

## Architectural Decisions

No clear evidence found. No source files, configuration, manifests, or documentation are present in the selected source directory to identify architectural decisions about interface segregation, dependency inversion, error envelope shape, cancellation propagation, lifecycle ownership, or schema versioning.

## Notable Patterns

No clear evidence found. No patterns (consumer-defined interfaces, port-and-adapter, hexagonal architecture, capability providers, schema-first design, `Protocol`/`runtime_checkable` boundaries, `pydantic` model contracts, `Runnable`/`RunnableConfig`/`RunnableLambda` composability, `Pregel` superstep execution model, `StateGraph`/`MessagesState`/`MessagesStateGraph` channel typing, `Send`/`Command` branching, `interrupt` static/dynamic modes, `Durability` modes like `sync`/`async`/`exit`) can be observed because no source code is present.

## Tradeoffs

No clear evidence found. Without source material, no tradeoff discussion (e.g., narrow `BaseCheckpointSaver` contract versus richer convenience methods, structural `Protocol` typing versus nominal ABC inheritance, schema-strictness via `pydantic` versus flexibility of `TypedDict`, runtime validation in `StateGraph` versus compile-time guarantees, sync-versus-async saver contracts, in-process versus distributed executor contracts) is grounded in evidence.

## Failure Modes / Edge Cases

No clear evidence found. No interface definitions, validation logic, error envelopes, interrupt semantics, `NodeInterrupt`/`GraphInterrupt` propagation, channel-narrowing edge cases, distributed-executor timeouts, store/crdt conflict resolution, or deprecation markers exist locally to study failure modes. The only observable failure mode is at the study-input layer: an empty source directory prevents evidence-based analysis of the dimension at all.

## Future Considerations

If the source directory is populated (e.g., via `git clone https://github.com/langchain-ai/langgraph` into `studies/agent-harness-study/sources/langgraph/`), the analysis should be re-run. Specifically, re-inspect:

- The `langgraph.graph.state.StateGraph` / `langgraph.graph.message.MessagesState` surface for explicit `TypedDict`/`pydantic` `State` contracts versus runtime-attribute stores; whether `state_schema` is required to be a typed mapping.
- The `langgraph.pregel.Pregel` runtime interface (the central orchestration contract) versus the `Pregel.stream` / `Pregel.invoke` / `Pregel.ainvoke` / `Pregel.astream` public surface; how much of the contract is "behavioral" (superstep execution, superstep barriers, channel updates) versus "structural".
- The `langgraph.checkpoint.base.BaseCheckpointSaver` abstract interface (`get`/`put`/`list` configurable namespaces, `put_writes` for pending writes, async variants); whether the saver contract is narrow enough to allow third-party implementations (e.g., Redis, FoundationDB, S3) without undocumented coupling to channel serialization.
- The `langgraph.store.base.BaseStore` / `BaseStore.put`/`get`/`search`/`batch` contract for cross-thread memory; whether the store contract enforces any isolation or consistency guarantees.
- The `langgraph.cache.base.BaseCache` interface and whether it is a separate contract from the checkpointer or unified under `BaseCheckpointSaver`.
- The `langgraph.types.Command`/`Interrupt`/`Send`/`StreamWriter`/`RunnableConfig` type contracts: whether `Command.resume` payloads are typed (`TypedDict` discriminated union), whether `Interrupt` is structurally typed, whether `Send` is a `TypedDict` mapping.
- The `langgraph.errors` exception hierarchy (e.g., `GraphInterrupt`, `NodeInterrupt`, `ParentCommand`, `InvalidUpdateError`, `EmptyChannelError`, `CheckpointError`): whether error types form a stable, documented hierarchy versus ad-hoc string errors.
- Whether `py.typed` is shipped so static type checkers (`mypy --strict`) can enforce the `State`/`Command`/`Interrupt` contracts at compile time.
- Whether a conformance suite (e.g., `tests/conformance/`, `libs/checkpoint/tests/`) runs every checkpointer implementation against the same scenario matrix; whether store implementations are conformance-tested.
- Whether the `Durability` / `RetryPolicy` / `CachePolicy` contracts are expressed as enums/typed dicts with documented semantics, and whether the runtime honors them uniformly.
- Whether `interrupt` static-versus-dynamic mode, `before`/`after`/`on` node hooks, and subgraph communication (`Send`/`Command(goto=…)`) are part of the public contract or implementation detail.
- Whether `langgraph-sdk` (Python/JS) and `langgraph-api` server expose their own `Protocol` surface independent of the core library, indicating intentional layered contracts.
- Whether the upstream ships JSON-Schema or OpenAPI for graph configuration (`LangGraphConfig` schema) and tool/function definitions consumable by external agents.

## Questions / Gaps

- Was the upstream repository `https://github.com/langchain-ai/langgraph` expected to be cloned into `studies/agent-harness-study/sources/langgraph/` before dimension tasks were dispatched? The selected source directory is empty, while sibling sources (`langfuse`, `openhands`) were cloned with commits visible in `git status`.
- Should the harness study runner pre-clone source repositories before scheduling dimension tasks, or is the empty directory an intentional placeholder to be filled by a later step?
- Is the upstream repository accessible at the URL recorded in `sources/langgraph.ultraplan-source.yml:2`? No remote fetch was performed under the isolation rule.
- Without local source, every dimension question against `langgraph` is unanswerable. The orchestration layer should treat empty source directories as a hard pre-condition failure rather than dispatching dimension tasks.
- Whether the upstream `langgraph` ships a TypeScript sibling (`@langchain/langgraph`) with its own contract surface (`StateGraph`, `Pregel`, `BaseCheckpointSaver`), and whether the contracts are kept in sync across languages, is unknown from this study; this matters because consumer-defined interfaces differ between Python (`Protocol`/`runtime_checkable`) and TypeScript (structural `interface`/`type` with optional nominal brands).
- Whether the upstream ships Rust, Go, Java, or .NET bindings for `langgraph` runtime contract is unknown from this study.
- Whether `langgraph-api` (the server runtime) has a different contract surface than the in-process `Pregel` runtime — and whether that distinction is documented as part of the public contract — is unknown.

---

Generated by `24.02-interface-contract-design` against `langgraph`.