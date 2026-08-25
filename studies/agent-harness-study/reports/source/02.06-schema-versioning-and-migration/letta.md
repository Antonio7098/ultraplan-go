# Source Analysis: letta

## Schema Versioning and Migration

### Source Info

| Field | Value |
|-------|-------|
| Name | letta |
| Path | `studies/agent-harness-study/sources/letta` |
| Language / Stack | Python (FastAPI + SQLAlchemy 2.0 + Pydantic v2 + Alembic), PostgreSQL / SQLite |
| Analyzed | 2026-07-10 |

## Summary

Letta has two distinct persistence dimensions — the relational database (PostgreSQL / SQLite via SQLAlchemy + Alembic) and an exported agent file format (JSON, used for portability, backup, and `.af` upload) — and each has a different approach to versioning.

For relational state, Alembic drives schema migrations with a 167-revision linear chain (`alembic/versions/`). Each migration declares `revision` and `down_revision` IDs and contains paired `upgrade()` / `downgrade()` functions. Heavy migrations include backfills (e.g., `e991d2e3b428_add_monotonically_increasing_ids_to_.py:41-122` backfills `sequence_id` with `ROW_NUMBER()`), ORM-model rewrites (e.g., `d05669b60ebe_migrate_agents_to_orm.py:24-100` migrates agents to the new ORM pattern), and data-shape changes such as encryption-at-rest transitions (`eff256d296cb_mcp_encrypted_data_migration.py:29-175` and `d06594144ef3_add_and_migrate_encrypted_columns_for_.py:29-307`). Most migrations early-exit on SQLite and run on PostgreSQL only (`if not settings.letta_pg_uri_no_default: return`).

For agent-file portability, Letta uses two formats. The legacy single-blob `AgentSchema` Pydantic model (in `letta/serialize_schemas/pydantic_agent_schema.py`) carries an explicit `version: str` field (line 132) that is stamped on export (`marshmallow_agent.py:182`) and compared on import (`marshmallow_agent.py:223-230`), but the comparison is purely informational — it prints `Version mismatch: expected {version}, got {version}` and proceeds anyway. The current production format is `AgentFileSchema` (`letta/schemas/agent_file.py:431-445`), which has no top-level `schema_version`; instead its `metadata: Dict[str, str]` records `revision_id` set to the current Alembic revision at export time (`letta/services/agent_serialization_manager.py:488`), read back via `get_latest_alembic_revision()` (`letta/utils.py:1335-1351`). The schema-level import path does not consult that `revision_id` to gate behavior.

This is a clear "present but inconsistent" setup: there are two distinct versioning channels, the JSON file format is not actually version-checked, and the DB layer uses Alembic's revision pointer only as a stamp (no compatibility check on import). Old-state fixtures exist only for the legacy MemGPT era (tests/data/memgpt-0.2.11) and are not exercised by the test suite.

## Rating

**5 / 10 — Present but inconsistent, weakly documented, fragile.**

Rationale:
- Alembic migrations are well-structured, paired (upgrade/downgrade), and chain correctly.
- Data migrations for major refactors (agents-to-ORM, blocks-to-ORM, sources-to-ORM, MCP encryption) are present and carefully written.
- However the JSON agent-file format is *not* version-checked on import (the legacy `check_version` prints but does nothing; the new `AgentFileSchema` does not inspect `revision_id` at all).
- `use_legacy_format=True` is explicitly rejected (`letta/server/rest_api/routers/v1/agents.py:323`) and the legacy `AgentSchema` import raises 400 (`letta/server/rest_api/routers/v1/agents.py:580-583`).
- No fixtures for old-schema round-trips; the only "old state" in tree is `tests/data/memgpt-0.2.11` (MemGPT-era local-file layout) which has no associated test.
- "Will today's saved run still load after next month's refactor?" — for the relational DB: yes, via Alembic. For the portable `.af` file: unknown, because there is no machine-checked version field on the new format.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Alembic configuration & target metadata | `Base.metadata` registered as target; dialect URLs overridden per backend | `alembic/env.py:7-38` |
| Number of Alembic revisions | 167 migration scripts in linear chain | `alembic/versions/` (167 files) |
| Alembic baseline revision | `down_revision = None` (root) | `alembic/versions/9a505cc7eca9_create_a_baseline_migrations.py:20` |
| SQLite baseline (separate single-step) | Full schema snapshot, downgrade raises `NotImplementedError` | `alembic/versions/2c059cad97cc_create_sqlite_baseline_schema.py:23-790`, `:792-797` |
| Most migrations are PG-only | `if not settings.letta_pg_uri_no_default: return` guard | `alembic/versions/d05669b60ebe_migrate_agents_to_orm.py:25-27`, repeated in ~150 revisions |
| Per-dialect guard implementation | Settings flag `letta_pg_uri_no_default` distinguishes PG vs SQLite | `letta/settings.py`, used in `alembic/versions/**` |
| Sample paired migration (downgrade present) | add `timezone` to agents | `alembic/versions/c7ac45f69849_add_timezone_to_agents_table.py:23-40` |
| Sample data migration (backfill) | `sequence_id` column backfill via `ROW_NUMBER()` | `alembic/versions/e991d2e3b428_add_monotonically_increasing_ids_to_.py:41-122` |
| Sample ORM rewrite migration | agents user_id → organization_id + drop redundant columns | `alembic/versions/d05669b60ebe_migrate_agents_to_orm.py:24-100` |
| Blocks ORM rewrite | add `is_template`, `organization_id`, audit cols; drop `user_id`, `template` | `alembic/versions/f7507eab4bb9_migrate_blocks_to_orm_model.py:23-83` |
| Sources ORM rewrite | backfill `organization_id` from `user_id` | `alembic/versions/c5d964280dff_add_passages_orm_drop_legacy_passages_.py:35-49` |
| Source-id denormalization backfill | write `source_id` into `files_agents` | `alembic/versions/495f3f474131_write_source_id_directly_to_files_agents.py:23-50` |
| MCP encryption-at-rest migration (Phase 1) | add `*_enc` columns | `alembic/versions/d06594144ef3_add_and_migrate_encrypted_columns_for_.py:29-307` |
| MCP encryption migration (Phase 2) | batched encryption of plaintext into `_enc` | `alembic/versions/eff256d296cb_mcp_encrypted_data_migration.py:29-296` |
| MCP encryption: plaintext retained for rollback | "Plaintext columns are retained for rollback safety" | `alembic/versions/eff256d296cb_mcp_encrypted_data_migration.py:296` |
| MCP encryption: silent skip if no key | prints warning, returns early | `alembic/versions/eff256d296cb_mcp_encrypted_data_migration.py:30-35` |
| `preserve_on_migration` flag (template migration semantics) | new column added to `block` | `alembic/versions/341068089f14_add_preserve_on_migration_to_block.py:23-30` |
| Block ORM model field | `preserve_on_migration` | `letta/orm/block.py:43` |
| Block Pydantic schema field | `preserve_on_migration` | `letta/schemas/block.py:30, 100` |
| AgentFileSchema carries revision_id (NOT schema_version) | metadata dict on schema | `letta/schemas/agent_file.py:442-444` |
| Export stamps `revision_id` into metadata | reads `alembic_version` table | `letta/services/agent_serialization_manager.py:488` |
| Helper that reads Alembic version | `SELECT version_num FROM alembic_version` | `letta/utils.py:1335-1351` |
| Legacy AgentSchema `version` field (legacy format only) | top-level Pydantic field | `letta/serialize_schemas/pydantic_agent_schema.py:132` |
| Legacy AgentSchema marshmallow: stamp `version` on dump | post_dump sets `data[FIELD_VERSION] = letta.__version__` | `letta/serialize_schemas/marshmallow_agent.py:182` |
| Legacy AgentSchema marshmallow: `check_version` on load | prints mismatch, **does not raise** | `letta/serialize_schemas/marshmallow_agent.py:223-230` |
| Marshmallow version field name | `FIELD_VERSION = "version"` | `letta/serialize_schemas/marshmallow_agent.py:29` |
| Module version source | `letta.__version__` resolved from package metadata, env-override supported | `letta/__init__.py:1-11` |
| Agent-file import gate rejects legacy single-agent format | raises HTTPException 400 | `letta/server/rest_api/routers/v1/agents.py:580-583` |
| Agent-file export rejects `use_legacy_format=True` | raises HTTPException 400 | `letta/server/rest_api/routers/v1/agents.py:322-323` |
| `use_legacy_format` parameter (deprecated, marked) | declared on `GET /agents/{id}/export` | `letta/server/rest_api/routers/v1/agents.py:301-305` |
| Internal legacy import helper still exists | `import_agent_legacy` (unused by router path) | `letta/server/rest_api/routers/v1/agents.py:387-412` |
| Pydantic base schema | `extra="forbid"`, `from_attributes=True` — strict | `letta/schemas/letta_base.py:18-27` |
| ID format migration: bare UUIDs → prefixed IDs | validator logs and prefixes | `letta/schemas/letta_base.py:79-89` |
| ORM common mixin audit columns | `created_at`, `updated_at`, `is_deleted`, `_created_by_id`, `_last_updated_by_id` | `letta/orm/base.py:13-19` |
| ORM session helpers | no version-aware validation | `letta/orm/sqlalchemy_base.py:121-1003` |
| Deserialization helpers tolerate missing legacy fields | `deserialize_llm_config` defaults `strict` by provider | `letta/helpers/converters.py:76-94` |
| Deserializer pops legacy `requires_approval` | comment: "legacy field" | `letta/helpers/converters.py:239` |
| Deserializer maps legacy `transport` → `type` | comment: "legacy `transport` field to required `type` if missing" | `letta/helpers/converters.py:630` |
| Memory block deserializer handles two label conventions | current `skills/<name>` and legacy `skills/<name>/SKILL` | `letta/schemas/memory.py:405-417` |
| Test: export stamps revision_id | validates length 12 hex chars | `tests/test_agent_serialization_v2.py:1035-1048` |
| Test: AgentFileSchema metadata holds revision_id | uses hardcoded "495f3f474131" | `tests/test_agent_serialization_v2.py:1602-1621, 1640-1664` |
| Test: get_latest_alembic_revision helper | length-12 hex, stable across calls | `tests/test_utils.py:580-609` |
| Test: legacy agent download/upload flow | uses `use_legacy_format=True` (will now 400) | `tests/test_agent_serialization.py:631-683` |
| Test: legacy memory_blocks wrapper parser | "regression test from main" | `tests/test_context_window_calculator.py:423-424` |
| Test: SDK version compatibility check | `is_1_0_sdk_version` for HTTP client compat | `tests/test_utils.py:727-736`, helper at `letta/utils.py:1435-1491` |
| Old MemGPT state fixture (unreferenced by tests) | file system dump with `letta_version: "0.2.11"` | `tests/data/memgpt-0.2.11/agents/agent_test/config.json` |
| Old MemGPT sqlite fixture (unreferenced by tests) | sqlite blob from MemGPT era | `tests/data/memgpt-0.3.17/sqlite.db` |
| Old agent-file fixtures | various `.af` files, some stamped `"version": "0.6.47" .. "0.10.0"`, others unversioned (new `AgentFileSchema` shape) | `tests/test_agent_files/*.af` |
| MCP encryption migration tests | plain/encrypted coexistence, plaintext-only returns None | `tests/test_mcp_encryption.py:422-521` |
| CLI version command | `letta version` prints `__version__` | `letta/cli/cli.py:45-48` |

## Answers to Dimension Questions

1. **Does persisted state carry a version?**
   - **Database**: Yes, indirectly via the `alembic_version.version_num` table that Alembic maintains (`letta/utils.py:1341`). The schema itself does not have a row-level `schema_version` column.
   - **Legacy single-agent export (`AgentSchema`)**: Yes, top-level `version: str` Pydantic field (`letta/serialize_schemas/pydantic_agent_schema.py:132`), stamped at export with `letta.__version__` (`marshmallow_agent.py:182`).
   - **Current `AgentFileSchema`**: No `schema_version` field. It carries a `metadata: Dict[str, str]` whose `revision_id` is set to the *current Alembic revision* at export (`agent_serialization_manager.py:488`). The new format is otherwise not versioned.

2. **What happens when old state is loaded?**
   - **DB**: Old state is read by SQLAlchemy mapping; if the table column was dropped, the SELECT will fail at runtime. There is no schema-version negotiation layer.
   - **Legacy AgentSchema import**: `check_version` prints the mismatch to stdout (`marshmallow_agent.py:223-230`) but does **not** raise, so any version is accepted — there is no actual migration.
   - **New `AgentFileSchema` import**: Validates only ID format, duplicate IDs, and cross-entity references (`agent_serialization_manager.py:993-1020`). The `metadata.revision_id` is never inspected; no compatibility gate.
   - **HTTP API**: Legacy `AgentSchema` (single-agent) JSON now produces `HTTP 400 Legacy AgentSchema format is deprecated` (`agents.py:580-583`); `use_legacy_format=True` likewise 400 (`agents.py:322-323`). This is a hard rejection rather than a compatibility shim.

3. **Are migrations automatic or manual?**
   - **DB**: Manual invocation of `alembic upgrade head`. There is no CLI subcommand in `letta/cli/cli.py` for migration; the user is expected to run `alembic upgrade head` themselves. Most migrations early-exit on SQLite so a fresh SQLite DB gets a single baseline migration (`2c059cad97cc_create_sqlite_baseline_schema.py`) that snapshots the entire schema.
   - **JSON file**: There is no automatic migration logic. Old files load verbatim into the new code; missing fields rely on Pydantic `extra="forbid"` causing load failure, and legacy-only fields are silently dropped in a few helper paths (e.g., `requires_approval` in `converters.py:239`, `transport` → `type` in `converters.py:630`).
   - **MCP encryption**: Two-phase approach is automatic on next `alembic upgrade head` *only if* `LETTA_ENCRYPTION_KEY` is set (`eff256d296cb_mcp_encrypted_data_migration.py:30-35`). If unset, migration prints a warning and skips; plaintext columns remain in place for fallback reads.

4. **Are JSON blobs versioned?**
   - The legacy blob has a top-level `version` field but the loader does not act on it.
   - The current blob (`AgentFileSchema`) carries `metadata.revision_id` (the Alembic revision pointer) but the loader does not consult it.
   - Effectively: **no enforced versioning on JSON blobs**.

5. **Can old checkpoints/events still be read?**
   - **Old agent files** (`.af` with `version` field): technically yes for the format shape, but the legacy import path is now hard-rejected via the REST API. Only the server-internal `import_agent_legacy` still exists.
   - **Old messages with no `content_parts`**: works because the column is nullable (`2cceb07c2384_add_content_parts_to_message.py`).
   - **Old MCP tokens stored in plaintext**: read returns `None` for `*_enc` columns (`tests/test_mcp_encryption.py:474-489`); users must re-authenticate.
   - **Old bare UUID IDs**: tolerated via `allow_bare_uuids` validator (`letta/schemas/letta_base.py:79-89`), deprecated.
   - **Old MemGPT-era filesystem dumps** (`tests/data/memgpt-0.2.11`): no tested loader in the modern code base — left as unmaintained test data.

## Architectural Decisions

- **Alembic as the source of truth for DB schema.** Linear 167-revision chain with paired upgrade/downgrade, single root (`9a505cc7eca9`). No branching; all changes must be applied in order.
- **SQLite is treated as a development-only DB.** A single SQLite baseline migration (`2c059cad97cc`) snapshots the full schema and short-circuits all later migrations. There is no SQLite downgrade path (`raise NotImplementedError`).
- **Dual JSON serialization layer: Marshmallow for legacy single-agent export; Pydantic for the new multi-entity `AgentFileSchema`.** Both layers exist in `letta/serialize_schemas/` and `letta/schemas/agent_file.py` and are partially overlapping in purpose.
- **Versioning on the JSON file is informational, not enforced.** The `version` field on the legacy `AgentSchema` and the `revision_id` on `AgentFileSchema.metadata` are written but not checked on import.
- **Hard cut-over from legacy format to new format on the REST API.** `use_legacy_format=True` returns 400; legacy-shaped JSON returns 400. There is no compatibility adapter.
- **Encryption-at-rest migrations are designed to be non-destructive.** Plaintext columns are retained for rollback safety (`eff256d296cb:296`); plaintext reads are explicitly ignored after migration completes (`tests/test_mcp_encryption.py:422-489`).
- **ORM models use a shared audit mixin** (`letta/orm/base.py:13-19`) — `created_at`, `updated_at`, `is_deleted`, `created_by_id`, `last_updated_by_id` — applied via SQLAlchemy `declarative_mixin`, so all rows get consistent metadata without per-table schemas.
- **ORM rewrite migrations deliberately drop user_id in favor of organization_id** with backfill from `users.organization_id` (`d05669b60ebe_migrate_agents_to_orm.py:53-61`, `c5d964280dff_add_passages_orm_drop_legacy_passages_.py:38-49`, `f7507eab4bb9_migrate_blocks_to_orm_model.py:42-50`). This is a multi-table refactor handled in 3 separate migrations.
- **Message ordering refactor:** `sequence_id` BigInt + PostgreSQL sequence + backfill via `ROW_NUMBER()` rather than a JSON column. SQLite gets a `message_sequence` table plus a counter (`2c059cad97cc_create_sqlite_baseline_schema.py:423-432`).

## Notable Patterns

- **Dialect-gated migrations.** The pattern `if not settings.letta_pg_uri_no_default: return` is reused in ~150 migrations; SQLite gets a single monolithic baseline. This keeps the migration graph readable but means developers can only see schema-shape changes happening on PG.
- **`preserve_on_migration` flag on Block.** A per-row opt-in marker used during template migration to decide which blocks to carry over. The field is added via Alembic and threaded through the ORM, Pydantic schema, and the agent-file schema (`alembic/versions/341068089f14...py:29`, `letta/orm/block.py:43`, `letta/schemas/block.py:30, 100`, `letta/schemas/agent_file.py:294`).
- **Two-phase encryption rollout.** Add `_enc` columns nullable first (`d06594144ef3`), then backfill encrypted values batch by batch (`eff256d296cb`). Both phases early-exit if `LETTA_ENCRYPTION_KEY` is missing — operations are idempotent because the SELECT-then-UPDATE pattern only touches rows where the `_enc` column is `NULL`.
- **ID remapping on agent-file import.** Human-readable IDs (`agent-0`, `block-1`) inside the JSON file are remapped to prefixed UUID IDs in the DB during import (`agent_serialization_manager.py:475-490, 1023-1028`). The reverse happens on export.
- **Pydantic `extra="forbid"`** as a strict serialization invariant (`letta/schemas/letta_base.py:24`): unknown fields fail validation rather than being silently dropped — except in the legacy load path where this would break older files, so loaders sometimes pre-filter (`agent_serialization_manager.py:1023-1028`).
- **Print-and-continue version check.** `marshmallow_agent.py:223-230` exemplifies the "log mismatch, proceed anyway" pattern — useful for debugging but provides no safety net.

## Tradeoffs

- **Linear Alembic chain** is simple to reason about but means any backward-incompatible change blocks all installs.
- **SQLite short-circuit** removes test friction for the dev DB but masks dialect-specific bugs that only show up on PG (and vice versa).
- **Informational `version` on the JSON file** means a single field can satisfy "we have a version" without requiring an actual compatibility policy.
- **Hard-rejection of legacy JSON at the HTTP boundary** is simple but loses existing user files; there is no in-process adapter that converts old `AgentSchema` JSON to the new `AgentFileSchema`.
- **`preserve_on_migration` per row** is granular but couples a schema field to a runtime workflow (template migration), which is not immediately obvious from the column name.
- **Two-phase encryption** trades extra storage (plaintext + ciphertext) for safe rollback; the tradeoff is enforced by code, not by tests that exercise failure modes.
- **Pydantic strict mode** (`extra="forbid"`) prevents drift but creates friction for forward compatibility: a new field added to a class will cause older exports to fail validation.
- **Batched backfills** with `BATCH_SIZE=1000` (`eff256d296cb:41`) and periodic commits keep large migrations responsive at the cost of non-atomicity if a migration is interrupted mid-batch.

## Failure Modes / Edge Cases

- **MCP encryption migration with no `LETTA_ENCRYPTION_KEY`** prints a warning and returns — leaves `*_enc` columns NULL. Reads after migration return `None` (`tests/test_mcp_encryption.py:474-489`). User must re-encrypt manually or run a one-off script.
- **MCP encryption mid-migration crash**: because plaintext is preserved and the `_enc` write is per-row with a `NULL`-check predicate, restart is safe but progress is not transactional across batches.
- **Old `.af` files from versions < 0.10**: hard-rejected by the REST API (`agents.py:580-583`). Workaround is to wrap them in the new `AgentFileSchema` shape manually or use the still-existing internal `import_agent_legacy`.
- **Old agent files where `version` field is missing or wrong**: silently accepted; no error or warning to the caller beyond a print in the legacy marshmallow path.
- **Bare-UUID IDs** are still parseable via `allow_bare_uuids` (`letta/schemas/letta_base.py:79-89`) with a `logger.debug`, but downstream consumers (URL construction, prefix-stripping) may still fail.
- **Alembic downgrade paths are paired** but some destructive migrations (e.g., `d05669b60ebe_migrate_agents_to_orm.py`) lose data; calling `alembic downgrade` past them is unsafe.
- **`AgentFileSchema.metadata.revision_id` mismatch** between export and current DB: not detected by the importer. A file produced on a newer code base with newer columns can be imported into an older DB and will only fail at first use of a missing column.
- **`use_legacy_format=True`** now returns HTTP 400, but old client SDKs may still pass it; clients must be updated to drop the parameter (`agents.py:301-305`).
- **Pydantic `extra="forbid"`** on `LettaBase` means new schema fields silently break older files unless the field has a default.
- **`is_1_0_sdk_version`** helper (`letta/utils.py:1435-1491`) is a User-Agent/version parser — a mis-parsed User-Agent string silently toggles the response shape (e.g., `None` vs full AgentState).

## Future Considerations

- Add a real `schema_version` field to `AgentFileSchema` and gate the import path on it; produce migration shims that translate old formats to the new model rather than rejecting outright.
- Extend the legacy `check_version` to actually raise (or invoke a migration function) when a version mismatch is detected, instead of only printing.
- Introduce a CI migration test that runs `alembic upgrade head` against a snapshot of the schema at an older revision to confirm forward compatibility, and tests round-tripping of legacy `.af` fixtures through an adapter layer.
- Centralize the per-dialect SQLite/PG guard so that schema-affecting migrations cannot accidentally regress SQLite. The repeated `if not settings.letta_pg_uri_no_default: return` pattern is error-prone.
- Add observability: surface the current `revision_id` on the health endpoint so operators can detect drift between DB schema and running app version.
- The MCP encryption migration's silent skip on missing `LETTA_ENCRYPTION_KEY` should likely be promoted to a hard error or require an explicit opt-out flag, because the post-migration read path returns `None` and users lose access silently.
- Document or codify the `preserve_on_migration` semantics in migration scripts; today the workflow depends on undocumented cooperation between a column and a migration handler.

## Questions / Gaps

- **No automatic migration runner.** `letta/cli/cli.py` does not expose an `upgrade` subcommand. Is there a documented or expected invocation flow for upgrades, and is it covered in deployment scripts (`compose.yaml`, `Dockerfile`)?
- **No fixtures for old `AgentFileSchema` revisions.** Existing `tests/test_agent_files/*.af` are written by the current code, not snapshots of older versions. There is no regression test confirming a file from `letta 0.6.x` still loads into `letta 0.10.x`.
- **The `metadata.revision_id` is unused by the importer.** Is this intentional (so that the same file format works across minor schema changes) or an oversight?
- **`use_legacy_format=True` is rejected at the API but the underlying `import_agent_legacy` and `MarshmallowAgentSchema` are still maintained.** What is the migration plan to remove them entirely?
- **Old MemGPT state in `tests/data/memgpt-0.2.11` and `memgpt-0.3.17`** is not referenced by any test in the repository. Was the loader removed, or was the fixture left behind during the rewrite?
- **No documented policy on what happens when `AgentFileSchema.metadata.revision_id` is older than the current Alembic head.** The importer neither auto-upgrades nor refuses. Is this a known unsupported scenario, or should it be?
- **SQLite downgrade is hard-disallowed** (`NotImplementedError` in `2c059cad97cc:797`). What is the recommended recovery path for a corrupted SQLite DB?
- **The encryption-at-rest migrations depend on the application code's `CryptoUtils`** (`from letta.helpers.crypto_utils import CryptoUtils`). If the algorithm changes between the migration running and the application starting, decryption will fail. Is there a `version` or `algorithm_id` stored alongside the ciphertext?