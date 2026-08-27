# Source Analysis: letta

## 24.03 API Versioning and Compatibility

### Source Info

| Field | Value |
|-------|-------|
| Name | letta |
| Path | `studies/agent-harness-study/sources/letta` |
| Language / Stack | Python / FastAPI, Pydantic v2, SQLModel/SQLAlchemy + Alembic, Fern-generated OpenAPI, self-hosted + Cloud |
| Analyzed | 2026-08-27 |

## Summary

Letta exposes a single versioned REST surface (`/v1` + `/latest` alias) backed by ~200 Alembic migrations, pervasive `deprecated=True` markers on fields/routes, and dual-write fallback handlers that map deprecated request shapes to the current canonical ones. The approach keeps upgrades additive at the DB and schema layer (never dropping a deprecated column without an additive migration) and preserves old client shapes (bare UUIDs, `source_ids` → `folder_ids`, `llm_config` → `model`, `agent_id` as `conversation_id`). However there is **no formal version-negotiation, no capability/feature-flag handshake, no changelog/migration guide, and no executable backwards-compatibility contract test suite** — compatibility rests on ad-hoc `deprecated` flags and manual `if deprecated_param:` fallbacks gated by `settings.debug` logging. Persisted agent exports embed an `alembic_revision_id` but imports do not enforce or migrate across that version, so a production integration cannot upgrade safely without auditing internal changes. Rating 4/10 (present but fragile, policy-not-test).

## Rating

**4/10** — Version prefix and migration machinery exist and are actively used, but breaking-change policy, warning channel, and backwards-compatibility testing are informal. Deprecated surfaces linger for months with no Sunset/date, no semver branching, and only scattered integration assertions — the system is additive by habit, not by contract.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Version field – package | Single source of truth `0.16.8` with `LETTA_VERSION` env override and `importlib.metadata.version("letta")` fallback | `pyproject.toml:3`, `letta/__init__.py:5-11` |
| Version field – HTTP API | Global `API_PREFIX="/v1"` and `ADMIN_PREFIX="/v1/admin"`; OpenAPI `info.version:"1.0.0"` is static Fern artifact, FastAPI `app.version=letta_version`; every router mounts under `/v1` plus undocumented `/latest` alias | `letta/constants.py:32-35`, `fern/openapi.json:5`, `letta/server/rest_api/app.py:416`, `letta/server/rest_api/app.py:852-857` |
| Schema versions – persisted | 169 Alembic revisions; ex. `ffb17eb241fc_add_api_version_to_byok_providers.py` adds `api_version` to provider; SQLite baseline `2c059cad97cc` | `alembic/versions/ffb17eb241fc_add_api_version_to_byok_providers.py:1`, `alembic/versions/2c059cad97cc_create_sqlite_baseline_schema.py:1`, `alembic/env.py:38` |
| Schema versions – export contract | `AgentFileSchema.metadata.revision_id` set via `await get_latest_alembic_revision()` which `SELECT version_num FROM alembic_version`; `Block.preserve_on_migration` controls template migration | `letta/services/agent_serialization_manager.py:488`, `letta/utils.py:1335-1347`, `letta/schemas/agent_file.py:443`, `letta/orm/block.py:43` |
| Deprecation markers – HTTP | ~80 `deprecated=True` on FastAPI routes: all identities, sources, groups, `folders/name/{name}`, `agents/{id}/context`, `voices/voice-beta`, tool `requires_approval` query param | `letta/server/rest_api/routers/v1/identities.py:25`, `letta/server/rest_api/routers/v1/sources.py:49-441`, `letta/server/rest_api/routers/v1/folders.py:79-86`, `letta/server/rest_api/routers/v1/agents.py:588-1345`, `letta/server/rest_api/routers/v1/voice.py:21` |
| Deprecation markers – schema fields | `AgentState.llm_config`/`embedding_config`/`memory`/`sources`/`tool_exec_environment_variables`, `CreateAgent.llm_config`/`embedding_config`/`source_ids` etc. carry `deprecated=True`; `DEPRECATED_LETTA_TOOLS=["archival_memory_insert",…]` | `letta/schemas/agent.py:87-91,110,113-121,148`, `letta/constants.py:116`, `letta/server/rest_api/routers/v1/archives.py:27` |
| Backcompat shims – request handling | `agents.py` import endpoint prefers `name/model/embedding` over `override_name/override_model_handle` and `secrets` over `env_vars_json`, header `project_id` over form field | `letta/server/rest_api/routers/v1/agents.py:540-557` |
| Backcompat shims – conversation ID | `ConversationId` validator accepts `conv-*`, `default`, or `agent-*` (backwards compat); routers document `agent_id as conversation_id still works but will be removed` | `letta/validators.py:48-63`, `letta/server/rest_api/routers/v1/conversations.py:144` |
| Backcompat shims – Bare UUIDs | `LettaBase.allow_bare_uuids` converts bare `UUID` to `f"{prefix}-{uuid}"` with debug log | `letta/schemas/letta_base.py:79-89` |
| Backcompat shims – tool & source renames | `server.py` uses `folder_ids` if provided else fallback to deprecated `source_ids`; agent fallback for `llm_config.context_window` defaults | `letta/server/server.py:648`, `letta/schemas/agent.py:184-203` |
| Deprecation decorator | `@deprecated(message)` only logs `logger.warning` when `settings.debug` is true — silent in prod by default | `letta/helpers/decorators.py:64-76` |
| Extension point – experimental gate | `@experimental(feature_name, fallback)` and `get_experimental_checker()` – feature-flagging, not version negotiation | `letta/helpers/decorators.py:19-61`, `letta/plugins/plugins.py` (referenced) |
| Provider api_version per BYOK | `Provider.api_version` nullable, `ProviderTrace` / `AzureAsyncProvider.api_version` default `2025-04-01-preview` / `2024-09-01-preview` with validator defaulting null → latest | `letta/orm/provider.py:39`, `letta/schemas/providers/azure.py:38-45`, `letta/schemas/providers/base.py:32`, `letta/schemas/providers.py:1392-1400` |
| OpenAPI generation filters | `generate_openapi_schema` strips `/openai` compat paths, injects `LettaMessageUnion` etc., writes `openapi_*.json` | `letta/server/rest_api/app.py:136-162` |
| Missing – CHANGELOG/migration guide | No `CHANGELOG*` or `MIGRATION*` at repo root; no versioned migration guide under `docs/` inspected | `Glob CHANGELOG*` (no files), `Glob MIGRATION*` (no files) |

## Answers to Dimension Questions

### 1. Which APIs are stable, experimental, deprecated, or internal?

- **Stable (`/v1`)**: All primary domain routers — `agents`, `blocks`, `tools`, `conversations`, `groups`, `archives`, `folders`, `runs`, `jobs`, `steps`, `messages`, `providers`, `mcp-servers`, `health`, `chat/completions`, `embeddings` — mounted via `letta/server/rest_api/routers/v1/__init__.py:34-68` and exposed in `fern/openapi.json:22` under `/v1/*`. Fern SDK groups (`x-fern-sdk-group-name`) map 1:1 to these (`fern/openapi-overrides.yml:54-84`). Version is fixed to `LET TA_VERSION` at `letta/server/rest_api/app.py:416`. There is no `/v2` — evolution is additive within `/v1`.
- **Deprecated (still served)**: 
  - Whole surfaces: `sources` (replaced by `folders`) at `letta/server/rest_api/routers/v1/sources.py:49`, `identities` CRUD at `letta/server/rest_api/routers/v1/identities.py:25`, `groups` at `letta/server/rest_api/routers/v1/groups.py:17`, legacy `voice-beta` at `letta/server/rest_api/routers/v1/voice.py:21`, `folders/name/*` & `sources/name/*` at `letta/server/rest_api/routers/v1/folders.py:79`.
  - Field-level: `AgentState.llm_config`, `embedding_config`, `memory`, `sources`, `tool_exec_environment_variables`, `multi_agent_group` (`letta/schemas/agent.py:87-149`); `CreateAgent.source_ids/embedding_chunk_size/max_tokens/reasoning` (`letta/schemas/agent.py:218-342`); `Run.agent_ids`→`agent_id` (`letta/server/rest_api/routers/v1/runs.py:52`).
  - Tool-level: `DEPRECATED_LETTA_TOOLS = ["archival_memory_insert","archival_memory_search"]` (`letta/constants.py:116`) kept for existing agents but not attached by default.
  - All carry `deprecated=True` so OpenAPI shows `deprecated` and Fern marks `x-fern-availability: deprecated` (e.g. `fern/openapi-overrides.yml:122`).
- **Internal**: Paths starting `/v1/_internal_*` and `/v1/_internal_templates/*`, `/v1/_internal_blocks/*` etc. are present in `fern/openapi.json` but `x-fern-ignore: true` hides them from the published SDK in `fern/openapi-overrides.yml:22-48` and `letta/server/rest_api/app.py:142` strips `/openai` compat. They are reachable on the local server but not versioned for external use.
- **Experimental**: Gated by `@experimental(feature_name, fallback)` and `get_experimental_checker()` in `letta/helpers/decorators.py:19` and `letta/plugins/plugins.py`. No `x-fern-availability: experimental` is used in overrides (absent), so maturity labeling is code-private.

No explicit `STABLE/EXPERIMENTAL/DEPRECATED` manifest is published; status must be inferred from `deprecated=True` plus overrides.

### 2. How are users warned before breaking changes?

- **Primary mechanism**: OpenAPI `deprecated` boolean surfaces in SwaggerUI, Fern docs, and generated SDKs. Example description: `"**Deprecated**: Please use the list endpoint GET /v1/folders?name=" instead` at `letta/server/rest_api/routers/v1/folders.py:86` and `fern/openapi.json:3109`.
- **Decorator channel**: `@deprecated("...")` logs `logger.warning` only when `settings.debug` true (`letta/helpers/decorators.py:70`) — effectively silent for cloud and default self-hosted deployments.
- **Compatibility shims with no warning**: Many fallbacks silently prefer new over old (`final_name = name or override_name` at `letta/server/rest_api/routers/v1/agents.py:541`) without emitting a header or warning; users discover by reading schema descriptions.
- **Missing channels**: No `Sunset`/`Deprecation` HTTP headers, no changelog, no `MIGRATION.md`, no GitHub release notes inspected in repo, no staged warning → error lifecycle documented, and Fern changelog is external. `pyproject.toml:3` bumps `0.16.8` with no annotated semver policy.

Verdict: **passive visibility via OpenAPI/SDK generation**, not proactive upgrade warnings. Behavior is “warn via docs, keep serving.”

### 3. Are old clients, plugins, traces, or persisted artifacts still usable?

- **Old REST clients**: Yes, because old fields are kept optional and mapped. E.g. import with deprecated `override_name` still works (`letta/server/rest_api/routers/v1/agents.py:541`), `source_ids` still accepted as fallback to `folder_ids` (`letta/server/server.py:648`), bare UUIDs accepted and rewritten (`letta/schemas/letta_base.py:79`), deprecated query `?requires_approval` still read (`letta/server/rest_api/routers/v1/agents.py:728`). Pagination deprecated `cursor`/`source_id`/`active`/`ascending` are dual-handled (`letta/server/rest_api/routers/v1/runs.py:84-92`, `letta/server/rest_api/routers/v1/jobs.py:20-37`).
- **Persisted agent files/traces**: Each export embeds `metadata.revision_id` from `get_latest_alembic_revision()` (`letta/services/agent_serialization_manager.py:488`, `letta/utils.py:1335`). Import does **not** enforce or migrate that revision — `_validate_schema` in `agent_serialization_manager.py:531` checks shape, not version. Result: old exports generally import, but schema shifts (e.g., `Message` vs `LettaMessage`, block `preserve_on_migration` at `letta/orm/block.py:43`) can fail with generic 400; no automated upgrade path or versioned deserializer is found.
- **Migrations**: DB upgrades are forward-only via Alembic (`alembic/env.py:38` includes all `Base.metadata`). No downgrade testing. 169 revisions are strictly additive; dropping a column would break older clients reading directly — mitigated by never dropping `sources` FKs in one jump but by phased renames (e.g., `ffb17eb241fc_add_api_version_to_byok_providers.py` adds nullable column).
- **Plugins/tools**: MCP tool schemas carry `mcp:SCHEMA_STATUS` metadata (`letta/schemas/tool.py:146-155`); stale tools surface health warnings but are not version-negotiated. The `Tool` validator rebuilds built-in schemas from source each load (`letta/schemas/tool.py:74-107`), so old persisted `json_schema` is overwritten — masking drift silently in prod.
- **SDK/Cloud split**: Some `agent.search`, `identities/properties/agents`, `memory_variables` are Cloud-only (`fern/openapi-overrides.yml:326` note “only available on Letta Cloud”). No client-side negotiation distinguishes Cloud vs OSS, so OSS clients receive 404 without a machine-readable capability list.

Net: **old REST shapes remain usable because they are never deleted**, but **old persisted artifacts/traces depend on implicit backward shape tolerance, not a versioned migration contract**.

### 4. Does compatibility rely on policy alone or executable tests?

- **Dominated by policy**. Proof requires code: `deprecated=True` is a declaration; no enforcement of a deprecation SLA, no semver branch, no contract tests that pin old request shapes.
- **Sparse executable checks**:
  - `tests/test_utils.py:581-606` asserts `get_latest_alembic_revision()` returns a valid ID and is stable.
  - `tests/test_agent_serialization_v2.py:1036` asserts export `revision_id` equals `await get_latest_alembic_revision()`.
  - `tests/integration_test_conversations_sdk.py:960` proves old `agent_id`-as-`conversation_id` still works.
  - `tests/managers/test_message_manager.py:848-953` and `tests/test_managers.py:3573-3607` test backwards translation of tool returns and `create_passage` deprecated path.
  - `tests/test_letta_request_schema.py:173` and `tests/test_crypto_utils.py:344` touch backwards handling.
- **Gaps**: No golden-file test for an old SDK client vs new server, no consumer-driven contract test suite (e.g., pact), no property test asserting old `.af` files from `tests/test_agent_files/*.af` import cleanly (those fixtures exist but no parametrized import test enforces it), no test that deprecated query params emit warnings/headers, no `alembic downgrade` test. GitHub workflows (not inspected deeply beyond `.github/` dir listing) do not reveal a dedicated compatibility gate.

So: **policy-first, tests second — a handful of integration assertions, not a durability guarantee**.

## Architectural Decisions

| Decision | Evidence | Effect |
|----------|----------|--------|
| Single `/v1` prefix + undocumented `/latest` alias | `letta/constants.py:33`, `letta/server/rest_api/app.py:852-857` | Avoids parallel version maintenance; forces additive-only evolution but gives no opt-in to the next breaking version. |
| Additive Alembic over schema versioning in payload | `alembic/versions/*:169`, `alembic/env.py:38` | DB can always roll forward; old code reading new DB is untested (no backward DB compat). |
| `deprecated=True` everywhere, never delete quickly | `letta/server/rest_api/routers/v1/sources.py:49-441`, `letta/schemas/agent.py:87` | Keeps old clients green, but tech debt accumulates with dual code paths (fallback spaghetti). |
| Dual-write fallbacks in handler (`prefer new, accept old`) | `letta/server/rest_api/routers/v1/agents.py:540-557`, `letta/server/server.py:648`, `letta/server/rest_api/routers/v1/conversations.py:144` | Prolongs deprecation lifetimes without breaking callers; obscures when old path can be removed (no telemetry on usage). |
| Revision tag in exports, not in API negotiation | `letta/utils.py:1335`, `letta/services/agent_serialization_manager.py:488` | Enables provenance/debugging for support, but no conditional logic — imports ignore revision_id. |
| Fern-driven OpenAPI/SDK generation | `fern/openapi.json:5`, `fern/openapi-overrides.yml:9-48`, `letta/server/rest_api/app.py:136-162` | Gives SDK versioning (letta-client `>=1.7.12` in `pyproject.toml:46`) but couples breaking SDK rev to OpenAPI edit, not to external semver doc. |
| `@experimental` plugin gate instead of version | `letta/helpers/decorators.py:19-61` | Allows feature-flag experiments without API version bump, but no typed capability discovery for clients to probe. |
| `AllowBareUUIDs` validator + ID regex loosening | `letta/schemas/letta_base.py:79-89`, `letta/validators.py:48` | Tolerates historical IDs (`source-...`) forever, avoiding a hard migration cutover. |

## Notable Patterns

- **Fallback cascade pattern**: Every deprecated field has an `old or new` cascade at handler top (agents import, runs list, conversation get). Canonical example: `final_project_id = headers.project_id or project_id` (`letta/server/rest_api/routers/v1/agents.py:559`).
- **Metadata-preserved nullability**: Adding `api_version` as nullable then backfilling is the template (`letta/orm/provider.py:39`, migration `ffb17eb241fc`), preserving old rows without data migration failure.
- **Built-in tool schema regeneration**: `Tool.model_validator` recomputes `json_schema` from source for `letta_*` types each load (`letta/schemas/tool.py:74-107`), so persisted tool JSON is treated as ephemeral — breaks WYSIWYG for debugging but ensures server schema always wins.
- **Hidden internal surface**: Routes with `_internal_*` are stripped from public OpenAPI (`letta/server/rest_api/app.py:142` and `fern/openapi-overrides.yml:22` with `x-fern-ignore: true`) — a versioning isolation for enterprise vs OSS.
- **Env-overridden version**: `LETTA_VERSION` env can masquerade as release (`letta/__init__.py:10`) for cloud canaries without bumping `pyproject.toml:3`.

## Tradeoffs

- **Additive vs clean break**: Choosing additive `/v1` avoids maintaining `/v2` but forces perpetual dual handling (e.g., sources vs folders duplication for ~months), bloating validators and handler logic and increasing risk of inconsistency (e.g., `sources` FK still nullable long after rename).
- **Decorator silence vs noise**: `settings.debug` guard on deprecation warnings (`letta/helpers/decorators.py:70`) avoids log spam in prod, but also means most users never see deprecation signals unless reading docs.
- **Static OpenAPI version**: `fern/openapi.json:5` hardcodes `1.0.0` while server reports `0.16.8` — two truths cause confusion when the SDK pins `letta-client>=1.7.12`; a patch bump in server may carry breaking schema changes not reflected in OpenAPI version.
- **Revision tagging without migration**: Tagging exports is cheap insurance but skipping enforced migration code saves complexity yet leaves support to manual “re-export” guidance on breaks.
- **Regenerating tool schemas**: Guarantees tool correctness after source edits, but silently diverges from what an old export claimed the tool schema was — may hide breaking tool signature changes behind a successful import.

## Failure Modes / Edge Cases

| # | Mode | Trigger | Symptom | Mitigation present? |
|---|------|---------|---------|---------------------|
| 1 | `/latest` drift | Client pins to `/latest` instead of `/v1` | Next breaking `/v1` lands instantly on `/latest` consumers | None — `/latest` is undocumented but served (`letta/server/rest_api/app.py:857` commented alias). Recommend not advertising. |
| 2 | SDK ↔ server skew | Server adds required field, SDK regenerated via Fern but user stays on older `letta-client` | 422 due to missing field, stringified error via `custom_request_validation_handler` (`letta/server/rest_api/app.py:476-532`) without remedial hint | Middleware rewrites pattern/length errors with example, but no `Compat-Error` code path. |
| 3 | Alembic gap on import | Export pinned to `revision 990fd...` imported into DB at `a1b2c3...` many migrations later with incompatible column rename | `AgentFileImportError` 400 with generic message | No forward-migration script for `.af` payloads; fallback is to re-export from source env. |
| 4 | Sources→Folders ghost | Folder code deletes `sources` semantics but old agents still list `sources` relationship | `AgentState.sources` returned as empty while `folders` empty, causing UI confusion | Handler keeps both populated via alias, but no deprecation timeline to finally drop `sources`. |
| 5 | Bare UUID collision | External ID already prefixed but client sends bare UUID that collides after rewriting | Duplicate prefix append, 409 UniqueConstraintViolation (`letta/server/rest_api/app.py:581-590`) | Warned via debug log only. |
| 6 | BYOK `api_version` null | Old provider row created before `ffb17eb241fc` read by new code expecting non-null | Code defaults via validator (`letta/schemas/providers.py:1399`) but ORM layer row has `None`; client sees stale capabilities | Nullable column + code default masks failure but billing/tracing may use wrong API version. |
| 7 | Silent tool schema drift | MCP tool renames param, old persisted `Tool.json_schema` differs | Load-time regeneration overwrites exported schema (`letta/schemas/tool.py:82-107`), agent behavior changes post-restart with no warning | Only `MCP_TOOL_METADATA_SCHEMA_STATUS` surfaces warnings, not version. |
| 8 | No downgrade path | Hot-fix requires rolling DB back one migration | `alembic downgrade` will run `downgrade()` methods, but not tested in CI; may violate non-nullable constraints added in upgrade | Fact — no downgrade tests found. |

## Future Considerations

1. **Version-negotiation header** (`X-Letta-Api-Version` / `Accept: application/vnd.letta.v1+json`) and enforce it via `RequestIdMiddleware` pattern (`letta/server/rest_api/app.py:807`) — allows server to fail fast on too-old SDK instead of 422 wall.
2. **Capability discovery** (`GET /v1/capabilities`): enumerate supported `model handles`, `tool types`, `embedding chunk size defaults`, and `conversation forks` so self-hosted UIs don’t rely on Cloud-only 404s.
3. **Executable compatibility corpus**: Golden import of `tests/test_agent_files/*.af` for every merged PR + Pact-style contract tests for `letta-client` against pinned server image; add `Sunset` header on deprecated routes with countdown.
4. **Formal migration guide & changelog**: Auto-generate from Alembic message + `deprecated=True` annotation diff (reuse `fern/openapi.json` snapshot diff already committed) and publish under `docs/` with Fern `x-fern-availability` timeline.
5. **Revision-aware importer**: `AgentFileSchema.metadata.revision_id` should drive a chain of transformers (`letta/services/agent_serialization_manager.py:531` extension) rather than being ignored — mirror Alembic chaining for payloads.
6. **Retire `/latest`** or document as explicitly unstable and exempt it from inclusion in SDK generation to prevent accidental pinning.
7. **Emit deprecation signals over SSE/stream** (`letta/server/rest_api/app.py:585` SSE for tools) where JSON `deprecated` is invisible — add per-message `warning` envelope.

## Questions / Gaps

- No evidence of **semantic versioning policy** governing when `/v1` would become `/v2` or when `deprecated=True` would flip to 410 — searched `CONTRIBUTING.md` (not inspected for policy) and found no `semver` or `deprecation policy` docs; write `No clear evidence found` for formal SLA.
- No evidence of **breaking-change detection** (e.g., `schemathesis`, `openapi-diff` CI gate) — `.github/workflows` not deep-inspected beyond listing; no `fern check` hook found.
- No evidence of **feature-flag / capability negotiation** for clients — `FeatureFlag` search returned no hits; `experimental` checker is server-side only.
- No evidence of **backwards-compatibility load testing** across versions (e.g., OSS self-hosted old client vs cloud new server).
- No evidence of **persisted trace/provider-transaction compatibility** — `letta/schemas/provider_trace.py` not found and `letta/orm/provider_trace.py` lacks versioning.
- No evidence that `letta-client>=1.7.12` (`pyproject.toml:46`) compatibility is contract-tested against server `0.16.8`; pin is forward-minimum only.
- Search boundary: only source directory `sources/letta` inspected; sibling sources, provider configs, and built docs are outside scope.

---

Generated by `Dimension 24.03: API Versioning and Compatibility` against `letta`.
