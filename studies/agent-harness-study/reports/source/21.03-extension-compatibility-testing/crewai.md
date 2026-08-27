# Source Analysis: crewai

## 21.03 Extension Compatibility Testing

### Source Info

| Field | Value |
|-------|-------|
| Name | crewai |
| Path | `studies/agent-harness-study/sources/crewai` |
| Language / Stack | Python / Pydantic, workspace monorepo (crewai, crewai-tools, crewai-core, cli, crewai-files, devtools) |
| Analyzed | 2026-08-27 |

## Summary

CrewAI's primary extension contract is the tool system (`BaseTool`, `Tool`/`@tool`, `CrewStructuredTool`). The contract is implemented precisely in code and extensively exercised by internal unit/integration tests, but it is **not** exposed as a reusable conformance harness for third-party authors. Documentation provides copy-pasteable examples and a publish guide with package scaffolding, yet no test fixtures, helper factories, or `pytest` conformance suite are shipped for authors to assert "my tool satisfies the contract." Stability guarantees are implicit: version pinning is centralized, a detailed `changelog.mdx` and an `upgrading-crewai.mdx` migration guide communicate breaking changes reactively, but no `STABILITY.md`/semver policy or explicit extension-compatibility matrix exists. An author can verify a tool manually by instantiating it and calling `run()`/`arun()` or wiring it into a `Crew.kickoff()` — shown in docs — but there is no `assert_tool_conforms(my_tool)` equivalent.

## Rating

**5 / 10 — Present but inconsistent, weakly documented, fragile**

Rationale: `BaseTool`/`@tool` is a clear, typed, well-tested interface with rich validation and many internal tests proving behavior (args_schema inference, result_schema, usage limits, async). Examples are abundant. What is missing for *compatibility testing* is the outward-facing apparatus: no exported conformance test suite, no fixtures/factories for extension authors, and no formal stability/breaking-change contract. The `changelog` + `upgrading` guide partially mitigates breaking-change communication, but governance is ad-hoc.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Tool contract — abstract interface | `class BaseTool(BaseModel, ABC):` with required `name: str`, `description: str`, `args_schema`, `result_schema`, and `@abstractmethod def _run(...)` plus optional `_arun` | `lib/crewai/src/crewai/tools/base_tool.py:103` / `lib/crewai/src/crewai/tools/base_tool.py:387` / `lib/crewai/src/crewai/tools/base_tool.py:368` |
| Tool contract — registry & serialization | `_TOOL_TYPE_REGISTRY` populated in `__init_subclass__` for checkpoint deserialization; `tool_type` computed field `f"{cls.__module__}.{cls.__qualname__}"` | `lib/crewai/src/crewai/tools/base_tool.py:51` / `lib/crewai/src/crewai/tools/base_tool.py:109` / `lib/crewai/src/crewai/tools/base_tool.py:201` |
| Tool contract — args/result schema machinery | `args_schema` auto-generated from `_run` signature via `create_model`, with fallback to `_arun`; `result_schema` inferred from return annotation if `BaseModel`; serializers `_serialize_schema`/`_deserialize_schema` | `lib/crewai/src/crewai/tools/base_tool.py:207` / `lib/crewai/src/crewai/tools/base_tool.py:256` / `lib/crewai/src/crewai/tools/base_tool.py:160` |
| Tool contract — decorator | `@tool` overloads + runtime function generating `Tool` with type-derived `args_schema`, docstring/docstring validation, `result_schema` inference | `lib/crewai/src/crewai/tools/base_tool.py:676` / `lib/crewai/src/crewai/tools/base_tool.py:701` |
| Structured tool bridge | `class CrewStructuredTool(BaseModel):` with `from_function`, `_create_schema_from_function`, `_parse_args`, `invoke`/`ainvoke`, `formatted_description` | `lib/crewai/src/crewai/tools/structured_tool.py:189` / `lib/crewai/src/crewai/tools/structured_tool.py:234` / `lib/crewai/src/crewai/tools/structured_tool.py:301` |
| Internal conformance-like tests — BaseTool | Tests for annotation-based creation, subclass creation, `formatted_description` preservation, async/sync dispatch, validation, result_schema | `lib/crewai/tests/tools/test_base_tool.py:14` / `lib/crewai/tests/tools/test_base_tool.py:54` / `lib/crewai/tests/tools/test_base_tool.py:262` / `lib/crewai/tests/tools/test_base_tool.py:436` |
| Internal tests — StructuredTool | Initialization, `from_function`, result_schema, cache_function passthrough, side-effect count, exception handling | `lib/crewai/tests/tools/test_structured_tool.py:26` / `lib/crewai/tests/tools/test_structured_tool.py:79` / `lib/crewai/tests/tools/test_structured_tool.py:119` / `lib/crewai/tests/tools/test_structured_tool.py:397` |
| Internal tests — usage limits & async | `max_usage_count` enforcement and decorator variant; async run/arun, concurrent execution | `lib/crewai/tests/tools/test_tool_usage_limit.py:8` / `lib/crewai/tests/tools/test_tool_usage_limit.py:46` / `lib/crewai/tests/tools/test_async_tools.py:37` |
| No exported conformance harness | Search for `conformance`, `fixture.*tool`, `BaseTool.*test` yields zero hits in `lib/`; `conftest.py` provides only VCR/event-bus scaffolding, no `Tool` fixtures; no `crewai.testing` or `crewai_tools.testing` package exists | `conftest.py:190` / `conftest.py:220` / `lib/crewai/src/crewai/tools/__init__.py:1` |
| Example — custom tool authoring | Edge docs show both `BaseTool` subclass pattern and `@tool` pattern, plus typed output with `result_schema`/`format_output_for_agent` | `docs/edge/en/learn/create-custom-tools.mdx:22` / `docs/edge/en/learn/create-custom-tools.mdx:46` / `docs/edge/en/learn/create-custom-tools.mdx:68` |
| Example — publishable package | "The Tools Contract" section enumerates required fields, optional `args_schema`/`result_schema`/`EnvVar`, suggests `crewai-geolocate` package layout, `pyproject.toml`, and a `Crew.kickoff()` smoke test | `docs/edge/en/guides/tools/publish-custom-tools.mdx:18` / `docs/edge/en/guides/tools/publish-custom-tools.mdx:62` / `docs/edge/en/guides/tools/publish-custom-tools.mdx:241` |
| No dedicated fixture for extension authors | `lib/crewai-tools/tests/base_tool_test.py:1` repeats older composite-description assertions but does not expose a shared helper; `conftest.py` auto-fixtures are `cleanup_event_handlers`, `reset_event_state`, `setup_test_environment` — none aid third-party tool authors | `lib/crewai-tools/tests/base_tool_test.py:7` / `conftest.py:190` |
| Stability — workspace pinning | Root `pyproject.toml` declares `tool.uv.workspace` members and `pyproject.toml:50-59` pins internal deps `crewai-core==1.15.17`, `crewai-tools==1.15.17`, etc. | `pyproject.toml:265` / `lib/crewai/pyproject.toml:10` |
| Stability — changelog as breaking-change channel | `changelog.mdx` documents per-version `Features`/`Bug Fixes`/`Breaking Changes`/`Refactoring`; example "Breaking Changes — None" for v1.15.12 | `docs/edge/en/changelog.mdx:156` / `docs/edge/en/changelog.mdx:444` |
| Stability — upgrade guide | `upgrading-crewai.mdx:94` collects "Breaking Changes & Migration Notes" (import path renames, Agent/Crew param changes) but does not promise semver or stability tier | `docs/edge/en/guides/migration/upgrading-crewai.mdx:94` / `docs/edge/en/guides/migration/upgrading-crewai.mdx:98` |
| No stability policy file | No `STABILITY.md`, `VERSIONING.md`, or `COMPATIBILITY.md` at repo root; grep for `stability|semver|compatibility` returns only peripheral hits (tool output stabilizations, etc.) | `pyproject.toml:1` (no such key) / grep result summary |
| Hook extension point — not conformance-tested | `ToolCallHookContext` + `before_tool_call`/`after_tool_call` with reducer pattern and global registries; tests exist for decorators but no contract harness for extension hook authors | `lib/crewai/src/crewai/hooks/tool_hooks.py:31` / `lib/crewai/src/crewai/hooks/tool_hooks.py:133` |
| MCP extension point | `MCPToolWrapper(BaseTool)` and `MCPNativeTool(BaseTool)` as protocol adapters; adapter tests check resolver logic, not a general MCP-tool conformance suite | `lib/crewai/src/crewai/tools/mcp_tool_wrapper.py:16` / `lib/crewai/src/crewai/tools/mcp_native_tool.py:17` |

## Answers to Dimension Questions

**1. Are extension contracts tested?**
Partially. The execution of the contract is thoroughly tested internally — `lib/crewai/tests/tools/test_base_tool.py:14` (794 lines, covering decorated/subclassed tools, `args_schema` validation, `result_schema`, usage limits, async paths, description preservation EPD-179), `lib/crewai/tests/tools/test_structured_tool.py:26`, `lib/crewai/tests/tools/test_tool_usage_limit.py:8`, `lib/crewai/tests/tools/test_async_tools.py:37`, plus `test_tool_failure.py`/`test_tool_usage.py`. However, these are *implementation* tests of the core, not a **conformance suite** extension authors can import and run against their own `BaseTool` subclass or `@tool` function. There is no exported `assert_conforms(BaseToolSubclass)` or `pytest` plugin in `crewai`/`crewai-tools`.

**2. Are fixtures provided for extension authors?**
No. `conftest.py:190` exposes three autouse fixtures (`cleanup_event_handlers`, `reset_event_state`, `setup_test_environment`) scoped to the internal test harness (event bus isolation, temp storage, VCR patching). `lib/crewai-tools/tests/base_tool_test.py:7` and `lib/crewai/tests/tools/*` provide no reusable factories, builders, or mock LLM harnesses for external consumption. The publish guide `docs/edge/en/guides/tools/publish-custom-tools.mdx:241` suggests a manual smoke test via `Crew.kickoff()` but does not ship a fixture library.

**3. Are examples provided?**
Yes, and they are the strongest dimension. `docs/edge/en/learn/create-custom-tools.mdx:22` ("Subclassing `BaseTool`"), `:46` ("Using the `tool` Decorator"), `:68` (typed output `InventoryTool -> InventoryResult`, `result_schema` with dicts, `format_output_for_agent` override, async via `_arun`), and `docs/edge/en/guides/tools/publish-custom-tools.mdx:18` ("The Tools Contract" with `GeolocateTool` examples) plus `:192` (recommended `pyproject.toml`/package layout) and `:241` (full `Agent`+`Crew.kickoff()` snippet) are copy-ready. Multiple real tools in `lib/crewai-tools/src/crewai_tools/tools/**/ ` also serve as implicit examples, but the docs are the canonical ones.

**4. Are stability guarantees documented?**
No formal guarantees. There is no `STABILITY.md` or semver policy stating "BaseTool API is stable across minor versions." Breaking changes are communicated *reactively* through `docs/edge/en/changelog.mdx:1` (per-release notes with an explicit `### Breaking Changes` block, e.g. `docs/edge/en/changelog.mdx:156` showing `None`) and proactively via `docs/edge/en/guides/migration/upgrading-crewai.mdx:94` (import path migrations, param deprecations). Workspace pinning `pyproject.toml:265` / `lib/crewai/pyproject.toml:10` ensures synchronized releases, but not a compatibility promise. Greps for `stability|semver|compatibility` find no policy document.

> **Can an extension author verify their implementation against the contract?** Only informally: call `my_tool.run(...)`/`await my_tool.arun(...)`, inspect `my_tool.args_schema.model_json_schema()`, `my_tool.formatted_description`, or exercise via a minimal `Crew`. There is no `crewai.testing.assert_tool_conforms(my_tool)` harness, so verification is manual and relies on reading the implementation and replicating test patterns.

## Architectural Decisions

- **Pydantic-centric contract (`lib/crewai/src/crewai/tools/base_tool.py:103`)** — Tool identity, arguments, and outputs are all Pydantic models. Strength: automatic JSON schema, validation, serialization, and checkpointing (`lib/crewai/src/crewai/tools/base_tool.py:51` registry). Weakness: dynamic `create_model` from signatures is opaque to authors and couples extension correctness to Pydantic version quirks.
- **Dual interface: `BaseTool` subclass vs `@tool` decorator (`lib/crewai/src/crewai/tools/base_tool.py:521` / `lib/crewai/src/crewai/tools/base_tool.py:701`)** — Gives ergonomic surface but doubles the contract to document/test. Internal tests duplicate coverage across both surfaces (`lib/crewai/tests/tools/test_base_tool.py:14` vs `:334`).
- **Runtime schema inference (`lib/crewai/src/crewai/tools/base_tool.py:207` + `lib/crewai/src/crewai/tools/structured_tool.py:301`)** — If authors omit `args_schema`, it is inferred from Python annotations. Tradeoff: lowers barrier but hides errors until runtime; no compile-time conformance check.
- **Structured result envelope (`lib/crewai/src/crewai/tools/structured_tool.py:59` + `lib/crewai/src/crewai/tools/base_tool.py:256`)** — Typed outputs via `result_schema`/`format_output_for_agent` decouple raw return from agent-facing JSON. Requires authors to understand the two-layer rendering.
- **Workspace-monorepo pinning (`pyproject.toml:265` / `lib/crewai/pyproject.toml:10`)** — Single-version bump across `crewai`/`crewai-tools`/`crewai-core`. Helps compatibility internally but means any breaking tool-contract change forces coordinated release; not an explicit external guarantee.

## Notable Patterns

- **Authored description preservation vs LLM composite (`lib/crewai/tests/tools/test_base_tool.py:712` / `lib/crewai/src/crewai/tools/structured_tool.py:127`)** — EPD-179 regression suite ensures `description` stays verbatim and `formatted_description` carries `"Tool Name: …\nTool Arguments: …\nTool Description: …"`. Example of mature, spec-level testing that *could* be a conformance assertion but is not exported.
- **Validation with schema hints (`lib/crewai/src/crewai/tools/base_tool.py:279` + `lib/crewai/src/crewai/tools/structured_tool.py:100`)** — `build_schema_hint()` enriches `ValueError` on bad kwargs, improving DX for extension misuse. Validated in `TestBaseToolRunValidation` etc.
- **Usage-limit + thread-safe claim (`lib/crewai/src/crewai/tools/base_tool.py:302`)** — `threading.Lock` + `ToolFailure` structured return shows careful extension-facing failure mode.
- **Checkpoint-aware polymorphism (`lib/crewai/src/crewai/tools/base_tool.py:109` / `lib/crewai/src/crewai/tools/base_tool.py:59`)** — `__get_pydantic_core_schema__` + `_resolve_tool_dict` resolves `tool_type` dotted path; extension authors who rename modules break deserialization unless they preserve `tool_type`.
- **Docs-as-extension-guide** — Two dedicated MDX guides (`docs/edge/en/learn/create-custom-tools.mdx:1`, `docs/edge/en/guides/tools/publish-custom-tools.mdx:1`) with tabbed Options, optional EnvVar, caching, async — the de facto conformance spec.

## Tradeoffs

- **Thorough internal tests without export** — Investment in coverage (794-line `test_base_tool.py`, 521-line `test_structured_tool.py`, VCR cassettes) improves core confidence but does not reduce extension author's burden; authors must copy patterns.
- **Implicit inference vs explicit schema** — `_run` annotation inference lowers friction but makes the contract harder to reason about; recommended `args_schema: type[BaseModel]` in publish guide (`docs/edge/en/guides/tools/publish-custom-tools.mdx:88`) mitigates but is optional.
- **Changelog-driven breaking changes** — Lightweight process (write release notes) vs formal semver/stability tiers. Easy to maintain but nondeterministic for extension planning; `upgrading-crewai.mdx:94` must be manually scanned.
- **Monorepo version lockstep** — Ensures internal compatibility at cost of coupling; an author depending on `crewai-tools==1.15.17` gets `crewai==1.15.17` transitively rather than independent minor versioning.

## Failure Modes / Edge Cases

- **No contract-violation detector** — Misspelled `name`, missing docstring (`lib/crewai/src/crewai/tools/base_tool.py:735` raises `ValueError("Function must have a docstring")`), or untyped params surface only at tool instantiation or first `run()`, not at `pip install` or via a `crewai verify-tool` CLI.
- **Checkpoint deserialization breakage** — Renaming a tool's module/class without migrating persisted checkpoints fails at `dotted.rsplit(".",1)` (`lib/crewai/src/crewai/tools/base_tool.py:64`). No migration helper for extension authors.
- **Schema inference surprise** — A method with `*args/**kwargs` ignores those params (`lib/crewai/src/crewai/tools/base_tool.py:227`), silently creating a narrower `args_schema` than the author intended.
- **Result schema fallback (`lib/crewai/src/crewai/tools/structured_tool.py:59` + `lib/crewai/src/crewai/tools/structured_tool.py:82`)** — On invalid `result_schema` validation, only a `RuntimeWarning` fires and raw string is sent to agent; extension authors shipping a buggy `result_schema` get silent degradation, not a failed conformance check.
- **Stale `description` composition** — Pre-composed LLM blocks from old checkpoints are stripped via regex (`lib/crewai/src/crewai/tools/structured_tool.py:127`); third-party tools persisting composite strings double-wrap unless they adopt the pattern.
- **Async contract gap** — `_arun` defaults to `NotImplementedError` (`lib/crewai/src/crewai/tools/base_tool.py:374`), but no test asserts that a tool claiming async support actually implements it; callers discover only at `arun()` time.

## Future Considerations

- Export a `crewai.testing` (or `crewai-tools.testing`) conformance helper: `assert_tool_conforms(tool_or_cls)` that checks `name`/`description` non-empty, `args_schema` JSON-serializable, `_run` annotation coverage, docstring presence, `tool_type` round-trip, `run`/`arun` + `_run`/`_arun` consistency, `result_schema` serialization, and `format_output_for_agent` idempotence. Model on `test_base_tool.py:712` EPD-179 suite.
- Provide `pytest` fixtures: `make_tool_factory`, `fake_agent_context`, `tool_call_harness` to let authors exercise `ToolUsage` without a live LLM.
- Ship a `py.typed` contract and mypy plugin check (already `lib/crewai/src/crewai/mypy.py:1` exists) that flags untyped tool signatures at type-check time.
- Version the tool contract explicitly (`TOOL_CONTRACT_VERSION = "1"`) and surface in `tool_type` payload to enable migration on checkpoint restore.
- Add a dedicated `STABILITY.md` stating semver policy for `BaseTool`/`CrewStructuredTool`/`ToolCallHookContext`/`MCPToolWrapper`, and a `docs/edge/en/.../tool-compatibility-matrix.mdx` mapping crewai versions to supported tool API revisions.
- Gate breaking changes with deprecation warnings and codemods (already done sporadically via `docs/edge/en/guides/migration/upgrading-crewai.mdx:98`) but formalize as `PendingDeprecationWarning` in code.

## Questions / Gaps

- No evidence of automated extension-compatibility CI: e.g., running `lib/crewai-tools` tests against `main` of `crewai` or a sample external tool repo. Search of `.github/workflows` shows no dedicated `extension-compat` job (only `dependabot.yml`).
- No published fixture catalog for extension authors beyond doc snippets; could not locate `tests/fixtures` for tools — `lib/crewai-files/tests/fixtures` is for file types, not tool contracts.
- Changelog communicates breaking changes per release but does not assign a stability tier (stable/experimental/deprecated) to `BaseTool` fields; unclear whether `EnvVar`, `cache_function`, `result_as_answer`, `max_usage_count:lib/crewai/src/crewai/tools/base_tool.py:196` are all covered by same guarantees.
- Unknown whether `Tool.from_langchain` (`lib/crewai/src/crewai/tools/base_tool.py:424` / `:608`) is considered a supported extension path or legacy compat — no test marks it deprecated vs stable.

---

Generated by `21.03-extension-compatibility-testing` against `crewai`.
