# Source Analysis: letta

## 21.03 Extension Compatibility Testing

### Source Info

| Field | Value |
|-------|-------|
| Name | letta |
| Path | `studies/agent-harness-study/sources/letta` |
| Language / Stack | Python (FastAPI, Pydantic, SQLModel/SQLAlchemy, mcp, pytest) |
| Analyzed | 2026-08-27 |

## Summary

Letta exposes two primary extension surfaces: **custom tools** (Python/TypeScript source → JSON schema) defined by `letta/schemas/tool.py:110` (`ToolCreate`) and **MCP servers/tools** (`letta/functions/mcp_client/types.py:29`, `letta/schemas/mcp*.py`, `letta/services/mcp_manager.py`). A lightweight internal **plugin** system exists (`letta/plugins/plugins.py:8`, `letta/plugins/README.md:1`) for `experimental_check` and `summarizer`. Tool contracts are enforced by schema generation (`letta/functions/schema_generator.py:409`) and validation (`letta/functions/schema_validator.py:12`, `letta/services/tool_schema_generator.py:18`), with extensive internal tests (`tests/test_tool_schema_parsing.py:91`, `tests/managers/test_tool_manager.py:513`, `tests/managers/test_mcp_manager.py:22`). Fixtures for extension-like objects exist only as internal pytest fixtures (`tests/managers/conftest.py:189`, `tests/conftest.py:238`); no public conformance harness, packaged fixture library, or canonical example gallery is provided. `examples/` contains only notebook data (`examples/notebooks/data/task_queue_system_prompt.txt`). No stability or breaking-change policy was found; `pyproject.toml:3` declares `0.16.8` with nightly `*.dev` builds but no semver guarantee, and deprecated `ToolType` members are merely annotated `DEPRECATED` (`letta/schemas/enums.py:221`). Extension authors cannot verify conformance self-service — validation is server-side (`letta/services/tool_manager.py:219`, `letta/functions/schema_generator.py:694`).

## Rating

**5 / 10** — Present but inconsistent, weakly documented, and fragile for external authors.

**Rationale:** Pydantic contracts + schema-generation and MCP-health validators exist and are heavily tested internally, but no public conformance suite/fixtures/examples, no published stability guarantees, and no documented breaking-change communication render the extension experience ad-hoc from an author's perspective. This matches rubric 4–6.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Tool extension contract | `ToolCreate` Pydantic model defines extension inputs: `source_code`, `source_type`, `json_schema`, `args_json_schema`, with `validate_typescript_requires_schema` validator | `letta/schemas/tool.py:110-143` |
| Tool contract enforcement | `Tool.refresh_source_code_and_json_schema` generates/validates `json_schema` per `ToolType` | `letta/schemas/tool.py:74-107` |
| Schema generation entrypoint | `generate_schema_for_tool_creation` — dispatches Python vs TypeScript, handles `args_json_schema` vs docstring | `letta/services/tool_schema_generator.py:18-101` |
| Docstring conformance rule | `validate_google_style_docstring` requires Google-style `Args:` and per-param documentation | `letta/functions/schema_generator.py:15-60` |
| MCP extension contract | `MCPTool` wrapper + `MCPToolHealth` health enum; `ToolCreate.from_mcp` maps MCP `inputSchema` → Letta `json_schema` via `generate_tool_schema_for_mcp` | `letta/functions/mcp_client/types.py:21-33`, `letta/schemas/tool.py:145-169`, `letta/functions/schema_generator.py:694-903` |
| MCP health validator | `SchemaHealth` enum `STRICT_COMPLIANT`/`NON_STRICT_ONLY`/`INVALID` + `validate_complete_json_schema` recursive checks | `letta/functions/schema_validator.py:12-202` |
| MCP normalization/healing | `normalize_mcp_schema`, `inline_ref`, `deduplicate_anyof`, strict-mode healing | `letta/functions/schema_generator.py:586-903` |
| Plugin extension contract | `@runtime_checkable SummarizerProtocol` with `summarize`/`get_name`; `DEFAULT_PLUGINS` map `target = "module:attr"`; `get_plugin` via `importlib.import_module` | `letta/plugins/plugins.py:7-42` |
| Plugin docs | Minimal README describes `plugin_name.config_name=module:class` delimited by `;` | `letta/plugins/README.md:1-22` |
| Conformance test – schema parsing | `test_derive_openai_json_schema` parametrized over `simple_d20`, `all_python_complex`, pydantic examples; compares generated vs `*.json` fixtures and `_so.json` strict outputs | `tests/test_tool_schema_parsing.py:91-128` |
| Conformance test – docstring | `test_google_style_docstring_validation` parametrized pass/fail for `Args:` rules | `tests/test_tool_schema_parsing.py:448-461` |
| Conformance test – strict mode | `test_enable_strict_mode_*` suite (adds required, nullable, nested objects, arrays, anyOf, type-array) + `test_complex_nested_anyof_schema_to_structured_output` | `tests/test_tool_schema_parsing.py:475-927` |
| Conformance test – tool manager CRUD | `test_create_tool`, `test_create_tool_duplicate_name`, `test_list_tools_with_*`, `test_update_tool_by_id`, `test_attach_tool*` | `tests/managers/test_tool_manager.py:519-541`, `tests/managers/test_tool_manager.py:594-1084` |
| Conformance test – MCP manager | `test_create_mcp_server`, `test_create_mcp_server_with_tools`, `test_complex_schema_normalization`, `test_mcp_server_resync_tools` (covers added/deleted/updated diff) | `tests/managers/test_mcp_manager.py:22-110`, `tests/managers/test_mcp_manager.py:112-234`, `tests/managers/test_mcp_manager.py:236-417`, `tests/managers/test_mcp_manager.py:709-817` |
| Conformance test – plugins | `test_plugins.py` validates experimental decorator + redis flag via `settings.plugin_register` | `tests/test_plugins.py:9-96` |
| Internal fixtures – tools | `print_tool`, `bash_tool`, `other_tool` fixtures derive schema via `derive_openai_json_schema` + `create_or_update_tool_async` | `tests/managers/conftest.py:189-285` |
| Internal fixtures – MCP | `mcp_tool` fixture via `ToolCreate.from_mcp` + `create_or_update_mcp_tool_async` | `tests/managers/conftest.py:550-575` |
| Internal fixtures – top-level | `print_tool_func`, `weather_tool_func`, `roll_dice_tool_func` raw python funcs for manual wrapping | `tests/conftest.py:238-301` |
| Test data as implicit examples | Paired `*.py` + `*.json` + `*_so.json` expected schemas for schema-generation examples | `tests/test_tool_schema_parsing_files/simple_d20.py:1-15`, `tests/test_tool_schema_parsing_files/simple_d20.json:1`, `tests/test_tool_schema_parsing_files/simple_d20_so.json:1` |
| Example gap – public examples | `examples/` contains only `notebooks/data/*.pdf|txt`, no tool/MCP/extension example referenced by docs | `examples/notebooks/data/task_queue_system_prompt.txt:1` (directory listing `examples:2`) |
| Version/stability gap | `pyproject.toml` declares `version = "0.16.8"`; nightly workflow mutates to `*.devYYYYMMDDHHMMSS`; no stability policy doc, no `CHANGELOG*`/`HISTORY*` | `pyproject.toml:3`, `.github/workflows/poetry-publish-nightly.yml:48-54` |
| Deprecation without policy | `ToolType.EXTERNAL_LANGCHAIN/COMPOSIO` marked `# DEPRECATED` but no migration/stability doc; backwards-compat comments scattered (e.g., `server.py:648`) | `letta/schemas/enums.py:221-222` |
| CI as proxy for breaking changes | `core-unit-test.yml` matrix pins test suites including `test_plugins.py`, `test_tool_schema_parsing.py`, `mcp_tests/` but no extension-specific compatibility job | `.github/workflows/core-unit-test.yml:37-60` |
| Server-side validation only | `ToolManager.create` generates schema if missing (`tool.json_schema is None → generate_schema_for_tool_creation`) — no client-side offline conformance harness exported | `letta/services/tool_manager.py:214-221` |

## Answers to Dimension Questions

| Question | Answer | Evidence |
|----------|--------|----------|
| 1. Are extension contracts tested? | **Partially yes — internally, not as a public conformance suite.** Tool schema generation, docstring rules, strict-mode healing, and MCP normalization/health are unit- and integration-tested. Coverage is deep for `generate_schema`, `validate_google_style_docstring`, `enable_strict_mode`, and MCP resync, but no exported conformance harness exists for third-party authors to run offline. | `letta/functions/schema_generator.py:15`, `letta/functions/schema_validator.py:20`, `tests/test_tool_schema_parsing.py:91-461`, `tests/test_tool_schema_parsing.py:682-927`, `tests/managers/test_mcp_manager.py:236-417` |
| 2. Are fixtures provided for extension authors? | **No — internal only.** `tests/managers/conftest.py:189` and `tests/conftest.py:238` provide rich `print_tool`/`mcp_tool`/helper fixtures, but they live in `tests/` and require a full `SyncServer`/`Organization` stack (`server.tool_manager.create_or_update_tool_async`). No packaged `letta.testing` or `conftest` export, no `pip install letta[testing]` fixture entry-point. | `tests/managers/conftest.py:189-285`, `tests/managers/conftest.py:550-575`, `tests/conftest.py:238-301`, `pyproject.toml:87-100` |
| 3. Are examples provided? | **No canonical extension examples.** `examples/notebooks/data/` holds prompt txt/pdf only. The closest to examples are `tests/test_tool_schema_parsing_files/*.py` (e.g., `simple_d20.py:1`) and `expected_base_tool_schemas.py`, which are test fixtures, not documented extension samples. README shows API usage, not tool-authoring. | `examples/notebooks/data/task_queue_system_prompt.txt:1`, `tests/test_tool_schema_parsing_files/simple_d20.py:1-15`, `README.md:24-110` |
| 4. Are stability guarantees documented? | **No.** Search found no `STABILITY.md`, `VERSIONING.md`, `CHANGELOG.md`, or SemVer policy. `pyproject.toml:3` + `poetry-publish-nightly.yml:50` imply `0.x` pre-1.0 with `.dev` nightlies. `ToolType` deprecations are code comments only. | `pyproject.toml:3`, `.github/workflows/poetry-publish-nightly.yml:48-54`, `letta/schemas/enums.py:221-222`, grep `stability|breaking|SemVer` → no policy doc (evidence: empty glob `CHANGELOG*`, `HISTORY*`) |

**Can an extension author verify their implementation against the contract?** **Only via server round-trip.** An author must `derive_openai_json_schema` locally or `POST /tools` and rely on server errors; there is no `letta extension verify` CLI, no exported `validate_complete_json_schema` harness, and no documented `ToolCreate` JSON-Schema test vector package. Internal validators (`validate_google_style_docstring:15`, `validate_complete_json_schema:20`, `normalize_mcp_schema:586`) are importable but undocumented as a public contract.

## Architectural Decisions

| Decision | Description | Consequence | File:Line |
|----------|-------------|-------------|-----------|
| Pydantic-contract-first extensions | Tools/MCP servers modeled as `LettaBase` Pydantic models with `model_validator` hooks | Contracts are machine-checkable and versioned with code, but stability depends on model field churn | `letta/schemas/tool.py:31-107`, `letta/functions/mcp_client/types.py:21-45` |
| Docstring-derived schema generation | Python tools infer JSON Schema from Google-style docstring + type hints via `docstring_parser` + `inspect.signature` | Low author burden but strict docstring compliance required; `validate_google_style_docstring` enforces `Args:` per-param docs | `letta/functions/schema_generator.py:15-60`, `letta/functions/schema_generator.py:409-523` |
| Two schema paths: ad-hoc vs `args_json_schema` | `Tool.args_json_schema` allows explicit Pydantic `model_json_schema` path; otherwise docstring inference | Supports complex types (pydantic models) without docstring parsing, but dual path increases test matrix | `letta/services/tool_schema_generator.py:67-84`, `letta/schemas/tool.py:48` |
| MCP schema health + healing | Three-state `SchemaHealth` + `normalize_mcp_schema` inlines `$ref`, dedupes `anyOf`, adds `additionalProperties:false` | Tolerates loose upstream MCP schemas while preserving strict-mode deployability; hides upstream incompatibilities until runtime | `letta/functions/schema_validator.py:12-26`, `letta/functions/schema_generator.py:586-714` |
| Plugin via `importlib` + `settings.plugin_register` | Plugins resolved as `"module:attr"` strings from semicolon-delimited setting; `@runtime_checkable Protocol` checked with `isinstance` | Extremely flexible but unchecked until runtime; typo in target fails late (`TypeError: Unknown plugin type`) | `letta/plugins/plugins.py:18-42`, `letta/plugins/README.md:7-15` |
| Minimal plugin surface | Only `experimental_check` and `summarizer` in `DEFAULT_PLUGINS` | Keeps plugin contract small and stable, but extension authors have almost no hook points | `letta/plugins/plugins.py:16-25` |

## Notable Patterns

| Pattern | Where | Notes |
|---------|-------|-------|
| `model_validator(mode="after")` contract hook | `letta/schemas/tool.py:74`, `letta/schemas/tool.py:132` | Centralizes schema generation/validation on model construction |
| Health-metadata embedding | `MCP_TOOL_METADATA_SCHEMA_STATUS/WARNINGS` keys injected into `json_schema` | Lets `enable_strict_mode` skip non-strict MCP tools while preserving warnings (`tests/test_tool_schema_parsing.py:800`) |
| Deep-copy + recursive normalization | `normalize_mcp_schema:602` uses `copy.deepcopy` + `inline_ref` recursion with `max_depth=10` | Safe schema mutation pattern for untrusted upstream MCP payloads |
| Pytest fixture factory for tools | `tests/managers/conftest.py:189` derives schema then `create_or_update_tool_async` | Reusable but server-coupled; not extractable as offline fixture |
| Expected-schema golden files | `tests/test_tool_schema_parsing_files/*.json` + `*_so.json` | Golden-file testing for schema stability; implicitly documents expected contracts but not published as examples |
| `@experimental` decorator as plugin consumer | `tests/test_plugins.py:12` `helper` + `letta/helpers/decorators.py:experimental` | Plugin system exercised via decorator, not direct `get_plugin` usage |

## Tradeoffs

| Tradeoff | Pro | Con | File:Line |
|----------|-----|-----|-----------|
| Docstring-inferred schemas | Zero extra artifact for simple tools; familiar Python idiom | Requires exact Google style; `validate_google_style_docstring:40` fails on missing `Args:`; errors surface only at creation (`derive_openai_json_schema`) | `letta/functions/schema_generator.py:15-60` |
| Healing vs rejecting MCP schemas | Normalization lets more MCP servers work (`test_complex_schema_normalization:236`) | Masks upstream contract drift; author may not know schema was healed until strict-mode deployment fails | `letta/functions/schema_generator.py:586-714`, `tests/managers/test_mcp_manager.py:236-417` |
| Server-side validation only | Single source of truth; no client SDK drift | No offline `letta verify`—author must run server or import private modules | `letta/services/tool_manager.py:219-221` |
| Lightweight plugin system | 72 LOC, easy to understand (`plugins.py:72`) | Only two hook points; `plugin_register["protocol"]` bug (`plugins.py:39` checks `plugin_register` not `plugin_register[plugin_type]`) shows low investment | `letta/plugins/plugins.py:1-72` |
| Golden-file tests | Precise regression detection for schema output | Brittle to intentional schema evolution; no semantic versioning to communicate break (`pyproject.toml:3`) | `tests/test_tool_schema_parsing_files/simple_d20.json:1` |

## Failure Modes / Edge Cases

| Failure | Symptom | Mitigation (if any) | File:Line |
|---------|---------|---------------------|-----------|
| Missing Google-style docstring | `ValueError: parameter 'y' not documented` or `has no docstring` | `validate_google_style_docstring:24` + warning in `generate_schema:411` but creation still raises | `letta/functions/schema_generator.py:24-59`, `tests/test_tool_schema_parsing.py:455-461` |
| Missing type annotation | `TypeError: lacks a type annotation` at schema generation | Hard fail in `generate_schema:458` | `letta/functions/schema_generator.py:458` |
| Union types beyond Optional | `NotImplementedError: General Union types are not yet supported` | Explicit limitation, but author discovers only at runtime | `letta/functions/schema_generator.py:93-95` |
| `dict[str, Any]` without Pydantic | `ValueError: Dictionary types ... not supported (consider using a Pydantic model)` | Forces Pydantic modeling | `letta/functions/schema_generator.py:154-156` |
| TypeScript without schema | `ValueError: TypeScript tools require explicit json_schema` | Early `model_validator` fail | `letta/schemas/tool.py:138-142` |
| MCP schema `INVALID` | Tool persisted? `test_create_mcp_server_with_tools:146` shows `INVALID` filtered; `WARNING` still persisted; `health.status` stored in schema metadata | Partial: `mcp_manager.py:179`, `schema_validator.py:46` |
| Plugin target typo | `TypeError: Unknown plugin type` or `ModuleNotFoundError` from `importlib` | No conformance test for arbitrary plugin target; `get_plugin:33` splits on `:` naively | `letta/plugins/plugins.py:31-42` |
| Plugin protocol mismatch | `TypeError: does not implement SummarizerProtocol` (but bug: checks `plugin_register["protocol"]` not per-type) | `@runtime_checkable` check exists but buggy and untested for Summarizer | `letta/plugins/plugins.py:39-40` |
| Breaking `Tool` field rename | No stability doc; `model_config = ConfigDict(extra="ignore")` in `ToolUpdate:209` silently ignores unknown fields, hiding break | `extra="ignore"` masks backward-incompatibility | `letta/schemas/tool.py:209` |
| Empty object/array in required prop | `INVALID: required property allows empty object/array (OpenAI will reject)` | Caught by `validate_complete_json_schema:141-145` and stored as health reason | `letta/functions/schema_validator.py:141-145` |

## Future Considerations

- **Publish a public conformance harness:** Export `validate_google_style_docstring`, `validate_complete_json_schema`, and `derive_openai_json_schema` as `letta.testing` with a CLI `letta tool verify path/to/tool.py` and document it. Today an author must import private paths (`letta/functions/schema_generator.py:15`).
- **Ship fixture package:** Promote `tests/managers/conftest.py:189` helpers to `letta.testing.fixtures` (e.g., `make_tool_fixture(source_code) -> Tool`) so authors can pytest their tools without booting `SyncServer`.
- **Canonical examples gallery:** Move `tests/test_tool_schema_parsing_files/simple_d20.py:1`, `pydantic_as_single_arg_example.py`, `all_python_complex.py` into `examples/tools/` with README and `expected` JSON, and link from `README.md:24`.
- **Plugin contract hardening:** Fix `letta/plugins/plugins.py:39` bug (`plugin_register[plugin_type]["protocol"]`), add `get_plugin` conformance tests for success/failure, and publish `SummarizerProtocol:8` stability guarantee (e.g., "protocol will not add required methods without major bump").
- **Stability & breaking-change policy:** Add `STABILITY.md`/`CHANGELOG.md`, adopt SemVer (even if `0.x`), and document that `ToolCreate`/`Tool` fields are `extra="ignore"` vs strict — currently hidden (`ToolUpdate:209`). Announce deprecations for `ToolType.EXTERNAL_LANGCHAIN:221` with removal timeline.
- **Versioned tool JSON schema:** Emit `$schema` version in `json_schema` and `generate_tool_schema_for_mcp:886` `strict` envelope so authors can pin and test against a version.
- **CI extension-compatibility gate:** Add workflow that runs `test_tool_schema_parsing.py:91` golden files + a new `tests/test_extension_compatibility.py` on every `letta/schemas/tool.py` change to catch silent contract drift.

## Questions / Gaps

| Gap | What was searched | Why it matters |
|-----|-------------------|----------------|
| No public `letta.testing` harness | `glob tests/**/conftest.py`, `grep conformance\|fixture`, `pyproject.toml:87` optional deps | Cannot answer "can author run `pytest --extension-verify` offline?" — no evidence found |
| No stability/breaking-change doc | `glob CHANGELOG*`, `HISTORY*`, `docs/**/*`, `grep stability\|SemVer\|breaking`, `pyproject.toml:3` version | Extension authors cannot assess upgrade risk; `0.16.8` implies pre-1.0 instability |
| No packaged example tools | `read examples/` (only `notebooks/data`), `grep example` across sources | New authors lack copy-pasteable tool authoring sample |
| Plugin docs minimal | `read letta/plugins/README.md:1`, `read letta/plugins/plugins.py:72` | 22-line README + 2-plugin map is insufficient for external plugin development |
| `SummarizerProtocol` conformance untested for external impls | `read tests/test_plugins.py:96` only tests `experimental_check` | If author implements custom summarizer, no test verifies `summarize(text)->str` contract |

---

Generated by `21.03-extension-compatibility-testing` against `letta`.
