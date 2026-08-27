# Source Analysis: openai-agents-sdk

## 21.03 Extension Compatibility Testing

### Source Info

| Field | Value |
|-------|-------|
| Name | openai-agents-sdk |
| Path | `studies/agent-harness-study/sources/openai-agents-sdk` |
| Language / Stack | Python / Pydantic, OpenAI SDK, MCP, pytest |
| Analyzed | 2026-08-27 |

## Summary

The SDK defines multiple explicit extension contracts (`Model`/`ModelProvider`, `Session`/`SessionABC`, `TracingProcessor`/`TracingExporter`, `MCPServer`, `STTModel`/`TTSModel`/`VoiceWorkflowBase`/`VoiceModelProvider`, function-tool and sandbox capabilities) with abstract base classes, `Protocol` types, and runtime-checked storage. Stability is enforced by a rolling released-API contract (`tests/fixtures/released_api_contract.json:1` + `tests/test_released_api_contract.py:42` + `tests/fixtures/released_api_contract_policy.json:1`) that freezes top-level exports, signatures, dataclass fields, public class members, and typed-dict shapes per `pyproject.toml:3` version. Change policy is documented in `docs/release.md:1` with per-minor breaking-change changelogs and beta disclaimers (`src/agents/voice/realtime/README.md:3`, `docs/models/index.md:682`). Examples for every extension point are abundant (`examples/model_providers/custom_example_provider.py:43`, `examples/memory/*.py`, `examples/mcp/filesystem_example/main.py:1`). What is missing is a runnable conformance suite or reusable fixture that lets a third-party `Model`, `Session`, `MCPServer`, or voice implementation verify itself against the contract: the SDK ships deterministic doubles (`src/agents/testing/model.py:249` `ScriptedModel`, `src/agents/testing/sandbox.py:572` `scripted_sandbox_session`, `src/agents/voice/testing.py:153` `ScriptedSTTModel`) to test *SDK orchestration*, not to validate *extension compliance*. Extension tests exist internally (`tests/extensions/memory/test_redis_session.py:1`, `tests/mcp/test_*`, `tests/voice/test_testing.py:1`) but are not packaged as importable harnesses.

## Rating

**5/10 — Present but inconsistent, weakly documented, and fragile for extension authors**

Rationale: Interfaces are explicit, versioned, and guarded by contract tests and changelog discipline (good stability backstop), and examples are thorough. However there is no exported conformance test suite or fixture an extension author can run to prove `get_response`/`stream_response` call-ID invariants, `Session` atomicity/limit semantics, `TracingProcessor` thread-safety, or voice lifecycle invariants. The `agents.testing` helpers validate the runner, not the extension. Beta surfaces (`extensions/experimental`, LiteLLM/AnyLLM) are explicitly best-effort. A developer can read the ABC and copy an example, but cannot mechanically verify compatibility.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Extension contract: Model/Provider | `Model` ABC with 2 abstract methods and call-ID invariant docs; `ModelProvider.get_model` abstract | `src/agents/models/interface.py:37` , `src/agents/models/interface.py:138` |
| Extension contract: Session | `Session` Protocol + `SessionABC` ABC defining `get_items/add_items/pop_item/clear_session`; wrapper-aware helpers `_session_accepts_wrapper`, `_call_session_method` | `src/agents/memory/session.py:15` , `src/agents/memory/session.py:59` , `src/agents/memory/session.py:174` |
| Extension contract: Tracing | `TracingProcessor` ABC (5 abstract methods) with thread-safety notes, `TracingExporter` ABC | `src/agents/tracing/processor_interface.py:9` , `src/agents/tracing/processor_interface.py:132` |
| Extension contract: Voice | `STTModel`, `TTSModel`, `StreamedTranscriptionSession`, `VoiceModelProvider`, `VoiceWorkflowBase` ABCs | `src/agents/voice/model.py:91` , `src/agents/voice/model.py:147` , `src/agents/voice/workflow.py:1` |
| Extension contract: MCP | `MCPServer` ABC with `connect/cleanup/list_tools/call_tool/list_prompts/get_prompt` abstracts | `src/agents/mcp/server.py:542` |
| Extension contract: Tools | `Computer`, `ComputerTool`, `FunctionTool` dataclasses + `Tool` union orchestrated via helpers | `src/agents/tool.py:440` , `src/agents/tool.py:842` |
| Conformance enforcement (internal) | Released API contract freeze/verify logic: baseline pin, signature/type-alias/TypedDict/class-member validation | `tests/test_released_api_contract.py:42` , `tests/test_released_api_contract.py:211` |
| Policy defining stable surface | Canonical imports, optional dependency bindings, public class contracts, public properties/typed-dicts | `tests/fixtures/released_api_contract_policy.json:1` |
| Persisted contract artifact | Frozen JSON manifest checked at release | `tests/fixtures/released_api_contract.json:1` |
| RunState compatibility fixtures | Historical `RunState` corpus per schema version with provenance notes | `tests/fixtures/run_state/README.md:3` , `tests/fixtures/run_state/sources.json:44` , `tests/test_run_state_compatibility_corpus.py:34` |
| No conformance suite for Model | No `tests/test_*conformance*` or `agents.testing` validator for `Model`; grep finds zero `conformance` hits in src/tests except skills | `src/agents/testing/model.py:249` ( ScriptedModel is runner test double, not extension validator) |
| No conformance suite for Session | `tests/extensions/memory/*` test built-ins but not exported harness; `Session` has no `tests/test_session_*conformance` | `tests/extensions/memory/test_redis_session.py:1` , `tests/memory/test_session.py:1` |
| Testing doubles (not conformance) | `ScriptedModel` with `ModelStep` validation, `ScriptedSandboxSession` with method FIFO validation, `ScriptedSTTModel/ScriptedTTSModel/ScriptedVoiceWorkflow` | `src/agents/testing/model.py:249` , `src/agents/testing/sandbox.py:301` , `src/agents/voice/testing.py:153` |
| Testing docs scope | Docs state scripted utilities test orchestration, keep provider-wire tests on real adapters | `docs/testing.md:532` |
| Examples: Model provider | Custom provider example implementing `ModelProvider.get_model` returning `OpenAIChatCompletionsModel` | `examples/model_providers/custom_example_provider.py:43` |
| Examples: Sessions | 15 examples covering SQLite, Redis, SQLAlchemy, Dapr, Mongo, Encrypted, Advanced | `examples/memory/sqlite_session_example.py:1` , `examples/memory/redis_session_example.py:1` , `examples/memory/sqlalchemy_session_example.py:1` |
| Examples: MCP/Sandbox | Filesystem MCP example, sandbox capability mounts | `examples/mcp/filesystem_example/main.py:1` , `examples/sandbox/extensions/daytona/usaspending_text2sql/schema/glossary.md:745` |
| Extensions package | Thin re-export plus experimental beta namespace | `src/agents/extensions/__init__.py:1` , `src/agents/extensions/experimental/__init__.py:1` |
| Sandbox clients optional | Memory/sandbox extensions declare optional deps + platform guards | `tests/fixtures/released_api_contract_policy.json:364` , `src/agents/extensions/memory/redis_session.py:31` |
| Stability docs | Semver `0.Y.Z` policy, minor = breaking for non-beta publics, patch = beta/private | `docs/release.md:1` |
| Breaking-change log | Per-version changelog with migration notes (0.22.0–0.1.0) | `docs/release.md:20` |
| Beta/unstable disclaimer | Realtime beta, LiteLLM/AnyLLM beta best-effort warnings | `src/agents/realtime/README.md:3` , `docs/models/index.md:682` |
| Docs reference for extensions | Extensions ref linked, examples gallery | `docs/llms.txt:54` , `docs/sessions/index.md:749` |

## Answers to Dimension Questions

**1. Are extension contracts tested?**

Partially, but not as a consumable conformance suite for external authors. Internally the SDK enforces *surface stability* via `tests/test_released_api_contract.py:42` which validates that `src/agents/models/interface.py:37` (`Model`), `src/agents/memory/session.py:15` (`Session`), `src/agents/tracing/processor_interface.py:9` (`TracingProcessor`), `src/agents/mcp/server.py:542` (`MCPServer`), and `src/agents/voice/model.py:91` signatures, abstract members, and owned inherited methods do not drift without policy change (`tests/fixtures/released_api_contract_policy.json:575`). `tests/test_run_state_compatibility_corpus.py:34` validates persisted `RunState` backward reads against `tests/fixtures/run_state/*`. `tests/extensions/memory/test_*.py` and `tests/mcp/*` exercise built-in extension implementations against the runtime. However there is no exported `validate_my_model_conforms()` or `validate_my_session_conforms()` harness; `src/agents/testing/model.py:249` `ScriptedModel` and `src/agents/testing/sandbox.py:572` `scripted_sandbox_session` test the *runner* using a fake extension, not the extension itself. A `grep` for `conformance` across src/tests returns no extension conformance suite (only skill references). An author must read the ABC/Protocol docs and write bespoke tests — grep finds zero runnable extension-conformance commands.

**2. Are fixtures provided for extension authors?**

Not as dedicated conformance fixtures. The repository ships:

- `tests/fixtures/released_api_contract.json:1` and `tests/fixtures/run_state/*` — stability fixtures for the SDK itself, not for third-party extensions.
- `src/agents/testing/__init__.py:1` and `src/agents/voice/testing.py:1` doubles — they snapshot calls (`ModelCall` `src/agents/testing/model.py:137`, `SandboxCall` `src/agents/testing/sandbox.py:122`, `STTCall` `src/agents/voice/testing.py:42`) and enforce FIFO consumption with `assert_complete()` / `UnconsumedModelSteps`, `UnexpectedModelCall`, `InvalidModelStep` etc., but they are provider-neutral test utilities for SDK workflow drift (`docs/testing.md:532`), not validators that assert a *custom Model/Session* respects call-ID uniqueness, streaming event ordering, retry-advice identity, Redis pipeline atomicity (`src/agents/extensions/memory/redis_session.py:470`), or voice PCM framing.
- `examples/memory/*.py` and `src/agents/extensions/memory/*` — reference implementations, not fixtures.

There is no `agents.testing.session_conformance` or `agents.mcp.testing` module, no `conftest` fixture factory for `Session` atomicity or `MCPServer` discovery error redaction invariants (`src/agents/mcp/server.py:1176`). The closest to a reusable fixture is `scripted_sandbox_session()` which lets an author drive a fake sandbox, but still requires the author to script steps rather than running a battery against their implementation.

**3. Are examples provided?**

Yes — extensive and per-extension-type. `examples/model_providers/custom_example_provider.py:43` shows `class CustomModelProvider(ModelProvider)` returning `OpenAIChatCompletionsModel`; sibling files show global client (`custom_example_global.py:1`), per-agent model (`custom_example_agent.py:1`), and adapter routing (`any_llm_provider.py:1`, `litellm_provider.py:1`, `litellm_auto.py:1`) with `examples/model_providers/README.md:1` instructions. Sessions: `examples/memory/sqlite_session_example.py:1`, `redis_session_example.py:1`, `sqlalchemy_session_example.py:1`, `dapr_session_example.py:1`, `mongodb_session_example.py:1`, `encrypted_session_example.py:1`, `advanced_sqlite_session_example.py:1`, plus HITL and compaction variants (`examples/memory/memory_session_hitl_example.py:1`). MCP: `examples/mcp/filesystem_example/main.py:1`, `examples/hosted_mcp/simple.py:1`, `examples/tools/web_search.py:1`, `examples/tools/code_interpreter.py:1`. Voice/Realtime: `examples/voice/streamed/main.py:1`, `examples/voice/static/main.py:1`. Docs cross-link these (`docs/sessions/index.md:749`, `docs/models/index.md:314`). Examples are runnable and reviewed in PRs (`AGENTS.md:298`).

**4. Are stability guarantees documented?**

Yes, explicitly but scoped to `0.Y.Z` pre-1.0 semantics. `docs/release.md:1` defines `0.Y.Z` where `Y` = breaking for non-beta publics, `Z` = non-breaking/beta/private; recommends pinning `0.0.x`. Each minor has a `## Breaking change changelog` entry with highlights and migration snippets (e.g., `docs/release.md:20` for 0.22.0, 0.21.0 `openai>=3` migration, 0.17.0 `extra_path_grants` sandbox boundary). Contract generation policy is encoded in `tests/fixtures/released_api_contract_policy.json:1` (canonical imports `agents.testing.model:1`, optional bindings `voice:1`, `extensions/memory:1`, `extensions/sandbox:1`, public class contracts `voice.model:575`, typed-dicts `ModelStepSpec:784`). Policy change requires explicit review (`tests/README.md:48`). Non-beta third-party adapters remain `best-effort beta` (`docs/models/index.md:682`, `docs/models/index.md:694`). Realtime is `beta: expect breaking changes` (`src/agents/realtime/README.md:3`). This communicates guarantees but also carves out large beta exclusion zones.

*Breaking-change communication:* via `docs/release.md` changelog, version bump discipline (`pyproject.toml:3` `version = "0.22.0"`), contract baseline pin (`tests/fixtures/released_api_contract.json` `baseline`, `baseline_commit` checked in `tests/test_released_api_contract.py:145`), and release-candidate prep that freezes and checks the contract (`.agents/skills/release-candidate-prep/SKILL.md:15`). No separate `CHANGELOG.md` or deprecation-warning helpers were found; deprecation is via changelog text and type narrowing rather than `warnings.warn` codemods.

## Architectural Decisions

- **Protocol-first with ABC fallback.** `Session` is a `Protocol` (`src/agents/memory/session.py:15`) for duck-typing third parties plus `SessionABC` (`src/agents/memory/session.py:59`) for internal inheritance; voice `STTModel`/`TTSModel` are strict ABCs (`src/agents/voice/model.py:91`). This maximizes interop but splits documentation between typing and runtime checks.
- **Contract-as-code via manifest.** Rather than hand-written stability docs, the SDK generates and pins a machine-readable manifest (`tests/fixtures/released_api_contract.json:1`) via `integration_tests/_contract_support` and validates it in unit tests (`tests/test_released_api_contract.py:42`) and release gates. Keeps guarantees enforceable but opaque to extension authors until the PR fails.
- **Beta namespace isolation.** `src/agents/extensions/experimental/__init__.py:1` houses hosted multi-agent, Codex, etc., explicitly exempt from `Y` breaking guarantees; similarly `docs/models/index.md:682` marks AnyLLM/LiteLLM as `best-effort beta`. Contain churn but reduces trust for those adapters.
- **Testing doubles are runner-centric.** `ScriptedModel` (`src/agents/testing/model.py:249`) models normalized output items and stream events, `scripted_sandbox_session` (`src/agents/testing/sandbox.py:572`) models `BaseSandboxSession` methods — both focus on proving runner behavior without live providers, not on proving extension correctness.
- **Capability-tables vs type unions.** Extensions like sandbox clients use optional-dependency tables in `tests/fixtures/released_api_contract_policy.json:364` and `pyproject.toml:36` extras (`voice`, `litellm`, `any-llm`, `redis`, etc.) rather than a unified capability registry — simple but requires docs to explain which extra unlocks which symbol.

## Notable Patterns

- **Detached snapshot + assert_complete drift detection.** Every scripted double records detached, immutable call snapshots (`_snapshot_model_call` `src/agents/testing/model.py:154`, `_snapshot_call` `src/agents/testing/sandbox.py:185`, `_snapshot_stt_call` `src/agents/voice/testing.py:93`) and enforces `assert_complete()`/`remaining_steps` (`src/agents/testing/model.py:313`) with structured errors (`InvalidModelStep` `src/agents/testing/model.py:105`, `SandboxCallMatcherError` `src/agents/testing/sandbox.py:97`). Pattern is reusable for conformance but currently only applied to the runner side.
- **Wrapper-opt-in Session context.** `_session_method_accepts_wrapper` / `_session_accepts_wrapper` (`src/agents/memory/session.py:155`) lets legacy `Session` impls stay structural while context-aware impls opt into `wrapper` injection — preserves backward compat without marker interfaces.
- **Credential-safe error mapping.** MCP transport errors are mapped to redacted `UserError` without chaining unsafe URLs (`src/agents/mcp/server.py:1176` ` _run_request_with_transport_error_redaction`), a conformance invariant that custom `MCPServer` impls would need to replicate.
- **Async-lock-guarded state.** `RedisSession` uses `asyncio.Lock` plus WATCH/MULTI pipeline with retry on `WatchError` (`src/agents/extensions/memory/redis_session.py:393`, `src/agents/extensions/memory/redis_session.py:470`) — shows the level of edge-case behavior a Session extension must handle that no harness checks.

## Tradeoffs

- **Surface stability vs author productivity.** Frozen manifest (`tests/fixtures/released_api_contract_policy.json:1`) gives consumers strong drift prevention, but because it is SDK-owned, an external author gets failure only after upgrading, not a local `pip install openai-agents[dev] && pytest --conformance` workflow.
- **Runner doubles vs extension doubles.** Publishing `ScriptedModel` helps SDK contributors; an extension author who wants to test their own `Model` against the spec must invert the pattern and write their own fake runner, duplicating logic the SDK already has.
- **Wide extension surface, shallow examples.** 15 memory examples (`examples/memory/**`) and 4+ model provider examples provide coverage breadth; depth (e.g., failure modes like `ModelTimeoutError`, `ModelRetryAdvice` replay safety `src/agents/voice/testing.py:180`, session corruption handling `src/agents/extensions/memory/redis_session.py:631`) is only illustrated in internal `tests/extensions/memory/*`, not in examples authors will copy.
- **Beta carve-outs reduce guarantee scope.** Marking whole families as experimental/best-effort avoids strict promises but means paying users of LiteLLM/AnyLLM or sandbox providers cannot rely on extension compatibility — Docs explicitly push validation to deploy-time (`docs/models/index.md:694`).
- **Polyglot MCP/Python async idioms.** `MCPServer` exposes both sync and async filter styles (`src/agents/mcp/server.py:1021` `_apply_dynamic_tool_filter` supporting awaitable and sync callables), which aids author ergonomics but expands the conformance matrix (every filter must be tested both ways).

## Failure Modes / Edge Cases

- **Unproven call-ID contract.** `src/agents/models/interface.py:37` requires each tool invocation carry a non-empty, non-reused call ID; `src/agents/testing/model.py:641` `_convert_output_items` enforces it for scripted output, but a real custom `Model` that reuses IDs will not be caught until the runtime fails to correlate tool outputs (`src/agents/run_internal/tool_execution.py:1`). No pre-flight validator exists.
- **Session limit/corruption asymmetry.** `RedisSession.get_items` limit handling expands fetch window on corrupt JSON (`src/agents/extensions/memory/redis_session.py:658`), while `SQLiteSession` and `AsyncSQLiteSession` have distinct compaction branches — an extension that skips this edge case will silently drop items under corruption. No shared `SessionConformanceTest::test_corrupt_item_is_skipped`.
- **Watch-conflict livelock.** `RedisSession` retries on `WatchError` indefinitely (`src/agents/extensions/memory/redis_session.py:593` `while True`), which can livelock under contention; author using naive `pipeline` without retry would instead lose writes — both behaviors pass the happy-path example but violate expected `Session` semantics.
- **Tracing processor blocking.** `TracingProcessor` docs warn methods must be thread-safe and not block (`src/agents/tracing/processor_interface.py:47`), but there is no test that asserts an author’s `on_span_end` does not deadlock — a blocking processor would stall the runner with no diagnostic (`src/agents/tracing/setup.py:1`).
- **Voice lifecycle leaks.** `ScriptedTranscriptionSession` tracks `closed/close_calls` and `assert_complete` on remaining turns (`src/agents/voice/testing.py:126`, `src/agents/voice/testing.py:258`), but a real `STTModel.create_session` extension that forgets to close its websocket leaks tasks — `tests/voice/test_testing.py:1` only covers the scripted double.
- **Optional-dependency import side effects.** `src/agents/extensions/memory/redis_session.py:31` imports `redis.asyncio` at import time and raises via `raise_optional_dependency_error`; an extension that imports lazily inside methods would mismatch the contract’s `optional_bindings` policy (`tests/fixtures/released_api_contract_policy.json:386`) and produce different `ImportError` semantics.

## Future Considerations

- Ship an importable conformance harness, e.g., `agents.testing.validate_model(Model) -> list[Violation]` and `agents.testing.validate_session(session) -> ...`, reusing the snapshot/matcher infrastructure already in `src/agents/testing/model.py:249` and `src/agents/testing/sandbox.py:301`. Start with Model call-ID/streaming invariants and Session `limit/limit=None`, `clear_session`, and corruption skip, porting assertions from `tests/extensions/memory/test_*.py`.
- Add fixture factories under `tests/fixtures/extension_compatibility/` with parametrized payloads (minimal conversation, tool loop, reasoning items, handoffs) so authors can run `pytest --extensions` without copying `docs/testing.md:532` recipes. Include the `run_state` provenance pattern (`tests/fixtures/run_state/sources.json:44`) as provenance for extension corpora.
- Promote stable extensions out of experimental: graduate `handoff_filters` (`src/agents/extensions/handoff_filters.py:1`) and `tool_output_trimmer` (`src/agents/extensions/tool_output_trimmer.py:1`) with explicit `public_class_contracts` entries and `release.md` migration notes; keep the beta label only for hosted multi-agent/Codex.
- Publish a breaking-change policy for `public_type_aliases`/`public_typed_dicts` (`tests/fixtures/released_api_contract_policy.json:770`, `tests/fixtures/released_api_contract_policy.json:784`) so that `Literal`/TTS voice or `ModelStepSpec` field additions are communicated as additive vs breaking.
- Add `agents.mcp.testing` scripted server double analogous to `scripted_sandbox_session` to let MCP extension authors validate credential-safe redaction and pagination cursors (`src/agents/mcp/server.py:1176`) deterministically.

## Questions / Gaps

- No evidence of a public `openai-agents[testing]` fixture that an external package can `pytest.importorskip` to validate its `Model`/`Session` against the current SDK version — searched `src/agents/testing/*`, `tests/test_released_api_contract.py`, `docs/testing.md`, and grep for `conformance|extension.*test` found none.
- Unclear whether `agents.extensions.memory` Session extensions are covered by `public_class_contracts` — `tests/fixtures/released_api_contract_policy.json:403` lists them only under `modules/optional_bindings`, not under `public_class_contracts`; abstract-member drift would not be caught by `_validate_public_class_contract`.
- Docs state LiteLLM/AnyLLM are `best-effort beta` (`docs/models/index.md:682`) but do not state the stability expectation for community sandbox clients (`src/agents/extensions/sandbox/e2b/*`, `modal`, `daytona`) — are they experimental or semver-covered?
- Breaking-change communication relies on `docs/release.md:20` changelog text; there is no `DeprecationWarning` timeline or codemod guidance for renamed fields (`src/agents/stream_events.py:32` notes a deliberate misspelling kept for compat without timeline).
- Extension authors cannot discover the contract surface without reading source: top-level re-exports (`src/agents/__init__.py:1`) expose `Model`, `Session`, etc., but there is no single `docs/extensions.md` aggregating all extension points, examples, and testing recipes.

---
Generated by `Dimension 21.03: Extension Compatibility Testing` against `openai-agents-sdk`.
