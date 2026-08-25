# Source Analysis: letta

## 02.01 — State Taxonomy and Ownership

### Source Info

| Field | Value |
|-------|-------|
| Name | letta |
| Path | `studies/agent-harness-study/sources/letta` |
| Language / Stack | Python 3 / FastAPI / SQLAlchemy 2 (async) / PostgreSQL + pgvector / Alembic / Redis / Pydantic v2 |
| Analyzed | 2026-07-12 |

## Summary

Letta stores agent state primarily in a PostgreSQL schema (~50 SQLAlchemy ORM models under `letta/orm/`, with 167 Alembic revisions under `alembic/versions/`) and a complementary object-store backed "memory repo" for git-style archival of core-memory blocks. Conversation, run, step, message, block, passage, file, tool, identity, and provider-trace records are durable and survive process restarts. Ephemeral coordination lives in Redis (conversation locks, run-id idempotency mappings, per-tool inclusion/exclusion sets, and SSE chunk streams) and in Python process memory (per-agent `in_context_messages`, `response_messages`, `context_token_estimate`, `client_tools`, `logprobs`, `turns`). Ownership is split between ORM model classes, pydantic schemas (`letta/schemas/`), and a layer of service managers (`letta/services/`). Optimistic locking is implemented for `Block` only; all other tables rely on PostgreSQL deadlock retries. Soft-delete via an `is_deleted` flag and audit fields (`_created_by_id`, `_last_updated_by_id`) come from `CommonSqlalchemyMetaMixins` (`letta/orm/base.py:12-85`). Cross-process state crosses naturally via Postgres, intentionally via Redis, and asynchronously via Lettuce / ClickHouse provider-trace backends.

## Rating

**8 / 10** — Clear, well-typed model with explicit schemas, explicit ownership modules, durable persistence, durability-aware failure handling (deadlock retries, optimistic versioning, conversation locks, OTID idempotency, ClickHouse TTL). Deductions: optimistic locking only on `Block`; in-memory trackers on `LettaAgentV3` (`in_context_messages`, `context_token_estimate`, `response_messages`, `turns`) are not snapshotted and drift on crash; `Agent.message_ids` is a denormalized cache of conversation state that can fall out of sync with `conversation_messages.in_context`; the actor/`organization_id` predicate is logged as a warning rather than enforced when `actor=None` (`letta/orm/sqlalchemy_base.py:256-258`); and eval/benchmark state is absent (only a per-step `feedback` column exists).

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| ORM base / audit / soft-delete mixin | `Base` + `CommonSqlalchemyMetaMixins` define `id`, `created_at`, `updated_at`, `is_deleted`, `_created_by_id`, `_last_updated_by_id` | `letta/orm/base.py:8-85` |
| Common CRUD / deadlock / access predicate | `SqlalchemyBase.create_async`, `update_async`, `hard_delete_async` retry on Postgres `40P01`; `apply_access_predicate` filters by `organization_id` unless `actor` is a `User` | `letta/orm/sqlalchemy_base.py:559-902` |
| Optimistic locking (Block only) | `Block.version` mapped via `__mapper_args__ = {"version_id_col": version}` -> raises `StaleDataError` -> `ConcurrentUpdateError` | `letta/orm/block.py:56-61`; `letta/orm/sqlalchemy_base.py:779-780`; `letta/errors.py:73` |
| Agent (durable configuration / system prompt / message_ids pointer) | `Agent` row holds `system`, `llm_config`, `embedding_config`, `tool_rules`, `message_ids` (JSON pointer list), `compaction_settings`, `response_format`, `last_run_*` denormalized counters | `letta/orm/agent.py:45-204` |
| Conversation (durable conversation state) | `Conversation` + `ConversationMessage` join with `in_context: bool` + `position: int` per conversation_id | `letta/orm/conversation.py:22-76`; `letta/orm/conversation_messages.py:15-73` |
| Run / Step (durable execution state) | `Run.status`, `completed_at`, `callback_url`, `ttft_ns`, `total_duration_ns`; `Step.id` ties to `provider_id`, `model`, `error_*`, `status`, `feedback` | `letta/orm/run.py:22-77`; `letta/orm/step.py:20-97` |
| Run / Step metrics | `RunMetrics` (`num_steps`, `tools_used`, `run_ns`) keyed by `runs.id`; `StepMetrics` keyed by `steps.id` | `letta/orm/run_metrics.py:19-86`; `letta/orm/step_metrics.py:20-121` |
| Block (core memory) + BlockHistory | `Block.value`, `limit`, `current_history_entry_id` -> `BlockHistory` rows with monotonic `sequence_number` for undo/redo | `letta/orm/block.py:20-115`; `letta/orm/block_history.py:12-48` |
| Memory repo (git-backed blocks) | `StorageBackend` abstraction (GCS / S3 / local); `GitOperations` writes commits and acquires Redis lock | `letta/services/memory_repo/storage/base.py:7-127`; `letta/services/memory_repo/git_operations.py:385-412` |
| Archival + Source passages | `ArchivalPassage` and `SourcePassage` extend `BasePassage` with pgvector `embedding` + dual-stored `tags` JSON + `PassageTag` junction | `letta/orm/passage.py:21-104`; `letta/orm/passage_tag.py:14-53` |
| File / FileContent / FileAgent (artifact state) | `FileMetadata` org-scoped; `FileContent` deliberately NOT org-scoped (comment: "potentially dangerous"); `FileAgent` join row holds `is_open`, `visible_content`, `start_line`, `end_line` | `letta/orm/file.py:17-107` |
| Tool registry | `Tool` row with `default_requires_approval`, `enable_parallel_execution`, JSON schemas, pip/npm requirements | `letta/orm/tool.py:17-60` |
| Tool routing per-group (ephemeral, Redis) | `create_inclusion_exclusion_keys` writes per-group allow/deny sets | `letta/data_sources/redis_client.py:548-559` |
| MCP servers | `MCPServer` + `MCPTools` (server_id, tool_id); encrypted `token_enc`, `custom_headers_enc` | `letta/orm/mcp_server.py:20-88` |
| Provider + ProviderTrace | `Provider` holds `api_key_enc`, `access_key_enc`; `ProviderTrace` captures full `request_json` / `response_json` per step | `letta/orm/provider.py:16-52`; `letta/orm/provider_trace.py:15-49` |
| Secrets / sandbox config | `SandboxConfig`, `SandboxEnvironmentVariable`, `AgentEnvironmentVariable` with `value_enc` (PBKDF2-encrypted) | `letta/orm/sandbox_config.py:18-81` |
| Tag junction tables | `AgentsTags` (`agent_id`, `tag` PK); `BlocksTags` (separate org-scoped PK); `PassageTag` (junction) | `letta/orm/agents_tags.py:12-29`; `letta/orm/blocks_tags.py:13-40`; `letta/orm/passage_tag.py:14-53` |
| Group (multi-agent) | `Group.agent_ids` (ordered JSON) + `turns_counter`, `last_processed_message_id`, `sleeptime_agent_frequency` | `letta/orm/group.py:17-43` |
| Identity (per-block, per-agent) | `Identity.identifier_key`, `properties` JSON; `IdentitiesBlocks` and `IdentitiesAgents` junction tables | `letta/orm/identity.py:18-74`; `letta/orm/identities_blocks.py:7-13` |
| Approval state on Message | `role="approval"`, `approval_request_id`, `approve`, `denial_reason`, `approvals` list column on `Message` | `letta/orm/message.py:75-83` |
| Eval-style feedback | `Step.feedback` text column ('positive'/'negative') + `Step.is_err` flag | `letta/orm/step.py:78-86` |
| Message sequencing (SQLite/Postgres) | SQLAlchemy `before_insert` event allocates `sequence_id` atomically from a `message_sequence` row | `letta/orm/message.py:130-265` |
| DB session / engine (durable) | `engine` and `async_session_factory` built once at module load; `DatabaseRegistry` yields `AsyncSession` with rollback-on-cancel | `letta/server/db.py:57-100` |
| Connection pooling / readiness | `Settings` declares `pg_pool_size`, `pg_max_overflow`, `pg_pool_timeout`, `pg_pool_recycle`, `disable_sqlalchemy_pooling=True` | `letta/settings.py:301-316` |
| Redis-backed conversation lock | `acquire_conversation_lock` (non-blocking, raises `ConversationBusyError`), TTL `CONVERSATION_LOCK_TTL_SECONDS` | `letta/data_sources/redis_client.py:194-229` |
| Redis-backed memory repo lock | `acquire_memory_repo_lock(agent_id, token)` for git commit serialization | `letta/data_sources/redis_client.py:331-363`; `letta/services/memory_repo/git_operations.py:385-387` |
| OTID idempotency (run replay) | `set_otid_run_mapping` -> `OTID_RUN_PREFIX:{otid}` -> `run_id` with TTL | `letta/data_sources/redis_client.py:250-289` |
| SSE chunk buffering (Redis streams) | `RedisSSEStreamWriter` -> `sse:run:{run_id}` stream with `stream_ttl=10800s`, `max_stream_length=10000` | `letta/server/rest_api/redis_stream_manager.py:26-200` |
| Postgres advisory lock (single-leader scheduler) | `_try_acquire_lock_and_start_scheduler` uses `pg_try_advisory_lock(ADVISORY_LOCK_KEY)` | `letta/jobs/scheduler.py:44-58` |
| In-memory agent process state (lost on crash) | `LettaAgentV3._initialize_state` populates `context_token_estimate`, `in_context_messages`, `client_tools`, `client_skills`, `logprobs`, `turns`, `response_messages`, `last_function_response`, `num_messages`, `num_archival_memories` | `letta/agents/letta_agent_v3.py:122-141` |
| Stateless ephemeral agent | `EphemeralAgent` intentionally has no state beyond `agent_id`, `openai_client`, `actor` | `letta/agents/ephemeral_agent.py:16-72` |
| Persistence of in-context pointer (rebuildable) | `AgentManager.set_in_context_messages_async` -> updates `Agent.message_ids` JSON | `letta/services/agent_manager.py:1622-1667` |
| Per-conversation in-context rebuild source of truth | `ConversationManager.update_in_context_messages` -> flips `in_context` and `position` on `ConversationMessage` | `letta/services/conversation_manager.py:750-797` |
| Isolated block override per conversation | `ConversationManager.apply_isolated_blocks_to_agent_state` replaces agent blocks with conversation-specific copies from `blocks_conversations` | `letta/services/conversation_manager.py:986-1032`; `letta/orm/blocks_conversations.py:7-19` |
| Provider trace backends (ClickHouse, socket, Postgres) | `TelemetryManager` writes to ALL configured backends concurrently (multi-backend dual-write) | `letta/services/telemetry_manager.py:50-80` |
| Lettuce (external agent runtime service) | `RunManager.get_run_by_id` falls back to `LettuceClient.create()` for run status | `letta/services/run_manager.py:105-128` |
| Streaming service lock + cleanup | `streaming_service` uses `lock_key` per agent/conversation; releases on terminal update | `letta/services/streaming_service.py:270-407`; `letta/services/run_manager.py:388-396` |
| Alembic schema evolution (167 revisions) | Migrations under `alembic/versions/` cover every model + index | `alembic/versions/` (167 files) |
| Settings/runtime config (Pydantic Settings) | `Settings`, `ModelSettings`, `SummarizerSettings`, `ToolSettings`, `TelemetrySettings`, `ReadinessSettings`, `LogSettings` | `letta/settings.py:114-648` |
| Schema base classes | `LettaBase` + `OrmMetadataBase` provide `__id_prefix__`, UUID validation, `metadata_`/`metadata` aliasing | `letta/schemas/letta_base.py:15-103` |
| In-memory client-side tools/skills | `BaseAgent.client_skills`, `BaseAgent.conversation_id` | `letta/agents/base_agent.py:52-54` |

## Answers to Dimension Questions

1. **What kinds of state exist?**
   - **Conversation state** (`Conversation`, `ConversationMessage` with `in_context` + `position`; per-conversation isolated `Block`s via `BlocksConversations`).
   - **Execution state** (`Run`, `Step`, `RunMetrics`, `StepMetrics`, `Job`, `LLMBatchJob`, `LLMBatchItem`).
   - **Tool state** (`Tool` registry, `MCPServer`, `MCPTools`, `ToolRule`, `requires_approval_tools` resolved at runtime).
   - **Memory state** (core: `Block` + `BlockHistory`; archival: `ArchivalPassage`; recall: `Message`; git-backed memory via external `StorageBackend`).
   - **Artifact state** (`FileMetadata`, `FileContent`, `FileAgent`, `Source`, `SourcePassage`).
   - **Approval state** (`Message.role="approval"`, `approval_request_id`, `approve`, `denial_reason`, `approvals`; `Tool.default_requires_approval`; `requires_approval_tools` derived from `tool_rules`).
   - **Eval state** (no first-class model; `Step.feedback` + `Step.is_err` are the only persisted evaluations).
   - **Runtime configuration** (`Settings`, `ModelSettings`, `SummarizerSettings`, `ToolSettings`, `TelemetrySettings`, `ReadinessSettings`; per-agent `LLMConfig`, `EmbeddingConfig`, `ResponseFormatUnion`, `ToolRule`, `Provider` + `ProviderModel`).
   - **Identity/Org/User/Group** (multi-tenant `Organization`, `User`, `Identity`, `Group`).
   - **Secret state** (`SandboxConfig`, `SandboxEnvironmentVariable`, `AgentEnvironmentVariable.value_enc`, `MCPServer.token_enc`, `Provider.api_key_enc`).
   - **Tag state** (`AgentsTags`, `BlocksTags`, `PassageTag`, `Group.agent_ids` ordered list).
   - **Coordination state** (Postgres advisory lock for scheduler leader; Redis conversation locks; Redis memory-repo commit lock; Redis OTID->run-id mapping; Redis inclusion/exclusion sets; Redis SSE chunk streams).

2. **Which state is source-of-truth?**
   - For in-context messages: `ConversationMessage.in_context` + `position` is the durable source of truth (`letta/orm/conversation_messages.py:15-73`). `Agent.message_ids` is a denormalized JSON cache (`letta/orm/agent.py:71`).
   - For block content: the current `Block.value` row; `BlockHistory` rows are append-only snapshots (`letta/orm/block.py:53-61`; `letta/orm/block_history.py:14-48`).
   - For agent configuration: the `Agent` row (`letta/orm/agent.py:45-204`) is source; in-memory `AgentState` Pydantic is rebuilt from it via `to_pydantic` (`letta/orm/agent.py:214-315`).
   - For conversation-scoped overrides: `Conversation.isolated_blocks` (via `blocks_conversations`) is the source (`letta/orm/conversation.py:54-60`).
   - For memory-repo content: the git repo in the configured `StorageBackend` (`letta/services/memory_repo/storage/base.py:7-127`).

3. **Which state is derived?**
   - `AgentState.memory` (Pydantic `Memory` with blocks + file_blocks) is rebuilt from `Agent.core_memory` + `Agent.file_agents` on each `to_pydantic` call (`letta/orm/agent.py:284-302`).
   - `AgentState.tags` is derived from `AgentsTags` rows on load (`letta/orm/agent.py:281`).
   - `Agent.last_run_completion` / `last_run_duration_ms` / `last_stop_reason` are denormalized counters updated from runs (`letta/orm/agent.py:101-110`).
   - `Run.ttft_ns` / `total_duration_ns` are derived from step-level metrics.
   - `Message.sequence_id` is derived/generated server-side via a `message_sequence` table for SQLite only (`letta/orm/message.py:130-265`); Postgres uses a `BigInteger` `FetchedValue()` server sequence.

4. **Which state is ephemeral?**
   - All per-request state on `LettaAgentV3` (`letta/agents/letta_agent_v3.py:122-141`): `context_token_estimate`, `in_context_messages`, `client_tools`, `client_skills`, `logprobs`, `turns`, `response_messages`, `last_function_response`, `num_messages`, `num_archival_memories`.
   - Conversation lock tokens in Redis (`letta/data_sources/redis_client.py:212-229`) - lost on Redis restart; `CONVERSATION_LOCK_TTL_SECONDS` bounds the lifetime.
   - OTID->run-id idempotency mappings (TTL `OTID_RUN_TTL_SECONDS`) (`letta/data_sources/redis_client.py:250-289`).
   - SSE chunks in Redis streams (TTL `stream_ttl_seconds=10800`, `max_stream_length=10000`) (`letta/server/rest_api/redis_stream_manager.py:42-43`).
   - Scheduler leader flag and advisory lock session (`letta/jobs/scheduler.py:18-22`).
   - Redis inclusion/exclusion tool routing sets.
   - Lettuce in-process client when not configured to persist.

5. **Which state is safe to lose?**
   - Redis conversation locks (re-acquired on next request; TTL bounded).
   - SSE chunk streams (regenerable by re-attaching to the persisted `Run`).
   - Lettuce status cache (re-fetched from Postgres).
   - In-process `response_messages` / `last_function_response` (regenerated on the next step).
   - `context_token_estimate` (recomputed from step usage).

   The following are NOT safe to lose:
   - `Agent` row (entire agent definition); recoverable only via backup.
   - `Block.value` (core memory content) — recoverable via `BlockHistory` undo chain (`letta/orm/block.py:52-58`).
   - `Message` rows (recall memory) — durable source-of-truth for conversation history; no fallback.
   - `ConversationMessage` rows — durable source-of-truth for in-context pointers; no fallback.
   - `Run` / `Step` rows — required for cancel/resume and observability.
   - `ArchivalPassage` / `SourcePassage` — vector memory; no fallback outside embeddings in Postgres.
   - `Provider` rows with `api_key_enc` — encrypted secrets are decryptable only with `encryption_key` env var (`letta/settings.py:448-449`); losing `encryption_key` loses all secrets permanently.
   - Identity/Org/Group relationships.

## Architectural Decisions

- **ORM/Pydantic split** — every model has a `__pydantic_model__` and a `to_pydantic` method (`letta/orm/sqlalchemy_base.py:979-991`; e.g. `letta/orm/agent.py:214-315`). This is the canonical ownership boundary: ORM is the storage representation; Pydantic is the wire representation.
- **Soft delete via `is_deleted`** from `CommonSqlalchemyMetaMixins` (`letta/orm/base.py:12-85`) — all durable entities participate; `_list_preprocess` honors `is_deleted == False` only when `check_is_deleted=True` (`letta/orm/sqlalchemy_base.py:393-395`).
- **Actor-based row-level access** — `apply_access_predicate` filters by `organization_id` for org-scoped models, by `user_id` for `UserMixin`-only models (`letta/orm/sqlalchemy_base.py:872-902`). When `actor=None` it logs a security warning rather than raising (`letta/orm/sqlalchemy_base.py:256-258`), which is the weakest link in ownership enforcement.
- **Optimistic locking only on Block** — `Block.version` with `__mapper_args__ = {"version_id_col": version}` (`letta/orm/block.py:56-61`). All other entities rely on Postgres deadlock retries (`_DEADLOCK_MAX_RETRIES = 3`, `_DEADLOCK_BASE_DELAY = 0.1`, exponential backoff) inside `create_async`, `update_async`, `hard_delete_async` (`letta/orm/sqlalchemy_base.py:40-41, 591-611, 768-791`).
- **Per-conversation in-context message tracking via `ConversationMessage`** is the newer source-of-truth; the legacy `Agent.message_ids` JSON column is kept as a denormalized cache (`letta/orm/agent.py:71`; `letta/orm/conversation_messages.py:18-22`).
- **Dual-stored tags** on archival passages — JSON column + junction table for efficient DISTINCT/COUNT (`letta/orm/passage.py:32`; `letta/orm/passage_tag.py:14-30`).
- **Conversation-scoped memory isolation** via `BlocksConversations` junction and `Conversation.isolated_blocks` (`letta/orm/blocks_conversations.py:7-19`; `letta/orm/conversation.py:54-60`). Applied to `AgentState.memory` in-place via `apply_isolated_blocks_to_agent_state` (`letta/services/conversation_manager.py:986-1032`).
- **Git-backed memory blocks** stored in an external `StorageBackend` (GCS / S3 / local) for OSS self-hosting (`letta/services/memory_repo/storage/base.py:7-127`). Enabled per-agent via the `git-memory-enabled` tag, detected in `to_pydantic` (`letta/orm/agent.py:289-293`).
- **Encrypted secrets via PBKDF2 + Fernet-style field** — `value_enc` columns on `Provider`, `MCPServer`, `AgentEnvironmentVariable`, `SandboxEnvironmentVariable` (`letta/orm/sandbox_config.py:53`; `letta/orm/mcp_server.py:44`; `letta/orm/provider.py:42-43`). Encryption key from `Settings.encryption_key` env (`letta/settings.py:448-449`).
- **Multi-backend dual-write for provider traces** — `TelemetryManager` writes to Postgres, ClickHouse, and Unix socket concurrently; reads from primary only (`letta/services/telemetry_manager.py:50-80`).
- **Single-leader scheduler** via Postgres advisory lock (`letta/jobs/scheduler.py:44-58`); non-leaders poll on `poll_lock_retry_interval_seconds`.
- **Redis conversation lock** with non-blocking acquire + bounded TTL prevents stuck locks; surfaces as `ConversationBusyError` -> HTTP 409 (`letta/data_sources/redis_client.py:194-229`; `letta/server/rest_api/app.py:586-593`).

## Notable Patterns

- **Source-of-truth duplication with reconciliation path**: `Agent.message_ids` is a JSON column mirror of the relational `ConversationMessage` rows. The reconciler is `ConversationManager.update_in_context_messages` (`letta/services/conversation_manager.py:750-797`) which flips `in_context` and rewrites `position`. This pattern allows cheap O(1) reads on the Agent row while preserving correctness through periodic reconciliation.
- **Idempotent retries via OTID hash**: `derive_request_token` hashes all message OTIDs and stores `OTID_RUN_PREFIX:{hash} -> run_id` in Redis (`letta/services/streaming_service.py:64-80`; `letta/data_sources/redis_client.py:250-289`). Duplicate POSTs return the original run stream instead of starting a new one.
- **Streaming with persistence and replay**: `RedisSSEStreamWriter` writes chunked SSE events to a Redis Stream `sse:run:{run_id}` with both TTL and max-length truncation (`letta/server/rest_api/redis_stream_manager.py:42-43, 132-164`). The `redis_sse_stream_generator` re-reads via `xrange` from a cursor (`letta/server/rest_api/redis_stream_manager.py:486-493`).
- **Async-only ORM access**: every manager exposes both sync and async variants (`AgentManager.get_agent_by_id` / `get_agent_by_id_async`, `set_in_context_messages` / `set_in_context_messages_async`). Async uses `asyncio.gather` to parallelize relationship loads in `Agent.to_pydantic_async` (`letta/orm/agent.py:464-476`).
- **Noop fallbacks for optional infrastructure**: `NoopAsyncRedisClient` (`letta/data_sources/redis_client.py:562-661`) and `NoopTelemetryManager` (`letta/services/telemetry_manager.py:105-130`) let the system degrade gracefully when Redis/telemetry backends are missing.
- **Pydantic Settings + YAML config layering**: `apply_config_to_env()` runs before settings construction (`letta/settings.py:11, 15`), enabling YAML -> env -> pydantic layering.
- **Template inheritance via `TemplateMixin` / `ProjectMixin` / `OrganizationMixin`**: applied on most durable models; supports scoped permissions and template hierarchies (`letta/orm/mixins.py`).

## Tradeoffs

- **Optimistic locking only on `Block`** — chosen because Block updates from concurrent tools are the most contention-prone write. Other writes (Agent, Conversation, Run, Message) fall back to Postgres deadlock retries, which works for low-contention workloads but does not protect against stale-read/lost-update bugs across long-running agent loops.
- **Actor=`None` allowed with warning** — convenient for tests and background jobs but a real authorization risk (`letta/orm/sqlalchemy_base.py:256-258`).
- **`FileContent` deliberately not org-scoped** — self-documented as "potentially dangerous" (`letta/orm/file.py:17-19`). This trades data isolation for simpler joins.
- **In-memory trackers on `LettaAgentV3` not snapshotted** — `context_token_estimate`, `turns`, `logprobs`, `response_messages`, `in_context_messages` live only in process memory (`letta/agents/letta_agent_v3.py:122-141`). On crash mid-step, the next instance must rebuild from `Agent.message_ids` + `ConversationMessage`. RL training (`turns`, `logprobs`) loses data.
- **Dual storage for in-context messages** — `Agent.message_ids` (denormalized cache) vs `ConversationMessage.in_context` (durable truth). Drift possible if `set_in_context_messages_async` updates one without the other (`letta/services/agent_manager.py:1622-1667`; `letta/services/conversation_manager.py:750-797`).
- **Redis as coordination substrate** — locks and idempotency mappings are eventually consistent. Without Redis, `NoopAsyncRedisClient` short-circuits locks, so multi-replica deployments without Redis can produce concurrent runs on the same agent.
- **External git storage optional** — `StorageBackend` is pluggable, but the local backend lives under `~/.letta/memfs/` which is node-local (`letta/services/memory_repo/storage/local.py:14`). Multi-replica deployments need GCS / S3.
- **Encrypted secrets depend on single env var** — losing `LETTA_ENCRYPTION_KEY` permanently destroys `value_enc` data across `Provider`, `MCPServer`, `AgentEnvironmentVariable`, `SandboxEnvironmentVariable` (`letta/settings.py:448-449`).
- **`ApproximateWordCount` for tags via JSON + junction** — dual writes per tag change (`letta/orm/passage.py:32`; `letta/orm/passage_tag.py:14-30`); faster reads, slower writes.
- **SQLite path is a fallback**, not first-class — message sequencing requires a custom `message_sequence` table and `before_insert` event (`letta/orm/message.py:130-265`); features like `blocks_tags` only work on Postgres (`letta/services/block_manager.py:102`).

## Failure Modes / Edge Cases

- **Postgres deadlock retry**: `_DEADLOCK_MAX_RETRIES = 3` with `_DEADLOCK_BASE_DELAY * 2**attempt` backoff (`letta/orm/sqlalchemy_base.py:40-41, 591-611`). Stale data errors on `Block` raise `ConcurrentUpdateError` (`letta/orm/sqlalchemy_base.py:779-780`).
- **Statement timeout / pool checkout timeout**: `handle_db_timeout` decorator catches `TimeoutError` and asyncpg `QueryCanceledError`, wraps as `DatabaseTimeoutError`, emits an OTel metric (`letta/orm/sqlalchemy_base.py:69-108`; `letta/orm/errors.py`).
- **Lock-not-available**: `DatabaseLockNotAvailableError` raised on `55P03` / `LockNotAvailableError` (`letta/orm/sqlalchemy_base.py:920-928, 969-974`).
- **Unique constraint / FK violation**: mapped to `UniqueConstraintViolationError` / `ForeignKeyConstraintViolationError` (`letta/orm/sqlalchemy_base.py:930-962`).
- **Conversation lock busy**: `ConversationBusyError` raised on lock acquisition failure; HTTP 409 surfaced via `_conversation_busy_handler` (`letta/server/rest_api/app.py:586-593`).
- **OTID replay recovery**: `try_recover_duplicate_request` returns the existing run's stream when OTID already mapped (`letta/services/streaming_service.py:83-117`).
- **Memory-repo commit lock**: `acquire_memory_repo_lock(agent_id, lock_token)` serialized via Redis; `release_memory_repo_lock` after commit (`letta/services/memory_repo/git_operations.py:385-412`).
- **Redis unavailable**: `get_redis_client` returns `NoopAsyncRedisClient` after init failure (`letta/data_sources/redis_client.py:663-680`); silently disables locks/idempotency/streaming replay.
- **Cancelled task on DB session**: `DatabaseRegistry.async_session` explicitly rolls back on `asyncio.CancelledError` to avoid "idle in transaction" pool leaks (`letta/server/db.py:91-97`).
- **Optimistic version conflict on `Block`**: `StaleDataError` -> `ConcurrentUpdateError` (`letta/orm/sqlalchemy_base.py:779-780`). No equivalent protection for `Agent` or `Message`.
- **`actor=None` security bypass**: only a warning, not an error (`letta/orm/sqlalchemy_base.py:256-258, 500-505, 805-810`). Combined with `apply_access_predicate` returning unfiltered queries when actor is missing.
- **`FileContent` join across orgs** — `FileContent` is intentionally unscoped, so a misuse in a query can leak file content across organizations (`letta/orm/file.py:17-19`).
- **Lost encryption key**: `value_enc` columns become unreadable. No key rotation mechanism visible.
- **Step feedback race**: `Step.feedback` is a single `String` column, last-writer-wins; no audit history.
- **`provider_trace_metadata_only=True`**: writes only metadata to Postgres, no JSON, per `provider_trace_metadata` table (`letta/orm/provider_trace_metadata.py`; `letta/services/provider_trace_backends/postgres.py:18-21`).
- **`ConversationMessage` cascade on Agent delete**: `ondelete="CASCADE"` on the FK (`letta/orm/conversation_messages.py:34-49`) — deleting an agent wipes all conversation message associations.
- **In-memory tracker loss on crash**: `LettaAgentV3.context_token_estimate`, `turns`, `logprobs`, `response_messages` are not persisted; on a multi-step run, the values before the last commit point are lost.

## Future Considerations

- **Generalize optimistic locking** from `Block` to `Agent`, `Run`, `Message`. The infrastructure exists (`StaleDataError` mapping at `letta/orm/sqlalchemy_base.py:779-780`); only `__mapper_args__` needs to be added per model.
- **Make `actor=None` raise by default** for production deployments, behind a `letta_security_strict_actor=True` flag, instead of warning.
- **Snapshot `LettaAgentV3` in-memory trackers** to a `run_state` table or to Redis for crash-safe RL training replay (`turns`, `logprobs`).
- **Reconcile `Agent.message_ids` with `ConversationMessage`** automatically on read; currently relies on explicit call sites (`letta/services/conversation_manager.py:750-797`; `letta/services/agent_manager.py:1622-1667`).
- **Encrypt `FileContent`** at rest, or scope it strictly by organization.
- **Key rotation for `value_enc`** columns, with a `key_version` discriminator column.
- **First-class eval/benchmark state model** — current `Step.feedback` is a single text column; an `EvaluationResult` table tied to `Step`/`Run`/`Agent` would cover the eval dimension cleanly.
- **Lettuce-run state recovery** — when `LettuceClient.create()` fails, fallback to DB only; the current code returns "DB run with current status" which can mask diverged state (`letta/services/run_manager.py:105-128`).
- **ClickHouse trace TTL** is configured at DDL level (90 days) but no automatic Postgres-side retention policy exists for `provider_traces`.
- **Conversation memory isolation** could be generalized to per-tool/per-user scopes, not just conversation.
- **Scheduler leader failover**: `_lock_retry_task` polls every `poll_lock_retry_interval_seconds=8 min` (`letta/settings.py:421`) — high failover latency for short-running cron-style jobs.

## Questions / Gaps

- Where exactly is `provider_trace_metadata` table defined vs. `provider_traces`? Both exist in `letta/orm/__init__.py:33-34` but the relationship / partition strategy is not visible at the ORM level.
- How does the Lettuce service own its own agent state — is the same ORM schema used, or a separate runtime? The Lettuce client (`letta/services/lettuce/lettuce_client_base.py`) is a thin client; the server side is not in this repository.
- What is the schema for `mcp_oauth`? It is imported (`letta/orm/__init__.py:24`) but no model file appears in `letta/orm/`. Search boundary: `grep "mcp_oauth" sources/letta/letta` returns only the import.
- Does the `agent_actor_id` predicate get enforced when `no_default_actor=True`? The flag is on `Settings.no_default_actor` (`letta/settings.py:466-469`) but enforcement logic is not visible in the captured code.
- The `step.feedback` column accepts any string; is there a validation layer in the service that restricts to 'positive'/'negative'? The comment says "Must be either 'positive' or 'negative'" but no validator is visible in the captured `step.py`.
- Does the conversation lock guarantee FIFO ordering across agents in the same Group, or just serialization per `conversation_id`?
- Are there TTLs on the `Messages` table beyond `is_deleted`? No retention policy was found.
- Does the git-backed memory repo have a quota / max repo size check before writes?
- `letta/orm/provider_trace_metadata.py` exists but the body was not inspected in this pass.
- The `approval_request_id` on Message is a free `String`, not a FK — is there a separate `ApprovalRequest` table that owns the IDs?
- `Job.message_messages` (join table) relationship was not fully traced.

---

Generated by `02.01-state-taxonomy-and-ownership` against `letta`.
