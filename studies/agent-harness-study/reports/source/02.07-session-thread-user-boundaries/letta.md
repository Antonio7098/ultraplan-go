# Source Analysis: letta

## Session, Thread, and User Boundaries

### Source Info

| Field | Value |
|-------|-------|
| Name | letta |
| Path | `studies/agent-harness-study/sources/letta` |
| Language / Stack | Python 3.12, FastAPI + SQLAlchemy 2.x (async) + Alembic; PostgreSQL/SQLite; Redis (optional, for conversation locks) |
| Analyzed | 2026-07-13 |

## Summary

Letta scopes state with a single dominant tenant axis, `organization_id`, and a weaker but explicit actor axis, `user_id`. Every mutable entity other than `Job` inherits `OrganizationMixin` (`letta/orm/mixins.py:19-24`), so the schema is structurally tenant-tagged; the universal read path `SqlalchemyBase.read_async` / `list_async` / `_list_preprocess` calls `apply_access_predicate` to inject `WHERE organization_id = actor.organization_id` whenever an actor is supplied (`letta/orm/sqlalchemy_base.py:871-902`, `letta/orm/sqlalchemy_base.py:266-267`, `letta/orm/sqlalchemy_base.py:527-529`). The `actor` is a `User` resolved from the `user_id` request header (`letta/server/rest_api/dependencies.py:38-83`) plus `user_manager.get_actor_or_default_async` (`letta/services/user_manager.py:111-135`).

Threads in Letta are **conversations** (one agent, many concurrent conversation threads sharing a per-agent message table) and **runs** (a single processing session inside a conversation). The first-class model is `Conversation` with FK to `agents.id` and `OrganizationMixin` (`letta/orm/conversation.py:22-61`); every run is FK-bound to an agent and may carry an optional `conversation_id` (`letta/orm/run.py:22-77`, `letta/orm/run.py:52-57`). Conversation concurrency is guarded by a Redis-backed lock keyed on `conversation_id` (`letta/data_sources/redis_client.py:194-249`, `letta/constants.py:474-475`).

Memory that crosses threads is explicit and narrow. A `Conversation` can clone a block with `isolated_block_labels` into a conversation-scoped copy via the `blocks_conversations` junction (`letta/services/conversation_manager.py:894-957`, `letta/orm/blocks_conversations.py:1-30`); `Conversation.fork_conversation` shares the underlying `Message` rows between source and fork (`letta/services/conversation_manager.py:103-172`). Archival memory and "recall" live at the agent level (`letta/functions/function_sets/base.py:87-161`), so they cross threads intentionally but are still org-scoped through `ArchivalPassage.organization_id` (`letta/orm/passage.py:21-46`, `letta/orm/passage.py:76-104`).

Identities (`org`/`user`/`other`) sit on top of users as another way to tag which external principal "owns" an agent or block, but their unique key is `(identifier_key, project_id, organization_id)` (`letta/orm/identity.py:18-50`), so they cannot accidentally collide across tenants.

## Rating

**7/10** — Clear and consistent org-scoped model with explicit interfaces (`OrganizationMixin`, `AccessType`, `apply_access_predicate`, `isolated_block_labels`, `actor: User` plumbed through every manager) and an opt-in hard mode (`settings.no_default_actor`, `letta/settings.py:466-469`). Two points held back: (1) the access predicate is **convention, not storage-enforced** — there is no DB-level row-level security or check constraint preventing a row with the wrong `organization_id`, so a query that omits `actor=` or `apply_access_predicate` silently returns cross-org data with only a `logger.warning` (`letta/orm/sqlalchemy_base.py:257-258`, `letta/orm/sqlalchemy_base.py:501-505`, `letta/orm/sqlalchemy_base.py:807-810`); (2) the default actor fallback means a request with no `user_id` header silently becomes the global default user (`letta/services/user_manager.py:113-135`), collapsing every unauthenticated caller into one org unless `no_default_actor` is set.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Organization is top of object tree | `Organization` docstring + cascade to every entity | `letta/orm/organization.py:32-81` |
| Default org constant | `DEFAULT_ORG_ID = "org-00000000-0000-4000-8000-000000000000"` | `letta/constants.py:55` |
| Tenant-scoped schema mixin | `OrganizationMixin.organization_id` FK to `organizations.id` | `letta/orm/mixins.py:19-24` |
| User-scoped schema mixin | `UserMixin.user_id` FK to `users.id` (only Job uses it) | `letta/orm/mixins.py:27-32` |
| Optional user-scoped column | `Job.organization_id` nullable FK | `letta/orm/job.py:43` |
| Per-project free-form tag | `ProjectMixin.project_id` nullable string, no FK | `letta/orm/mixins.py:67-72` |
| ID prefix system | `PrimitiveType` enum drives every entity ID prefix | `letta/schemas/enums.py:4-42` |
| ID regex validator | `PRIMITIVE_ID_PATTERNS` enforces `<prefix>-<uuid4>` | `letta/validators.py:12-19` |
| Bare UUID upgrade | `LettaBase.allow_bare_uuids` upgrades legacy UUIDs to prefixed | `letta/schemas/letta_base.py:79-89` |
| Created/updated by tracking | `_created_by_id`/`_last_updated_by_id` on every row, requires `user-` prefix | `letta/orm/base.py:30-85` |
| Universal org predicate | `apply_access_predicate` for ORGANIZATION adds `WHERE organization_id = org_id` | `letta/orm/sqlalchemy_base.py:871-902` |
| Per-user predicate | `apply_access_predicate` for USER adds `WHERE user_id = user_id` | `letta/orm/sqlalchemy_base.py:896-900` |
| `actor=None` warning | Logs `SECURITY: ... without actor. This bypasses organization filtering.` | `letta/orm/sqlalchemy_base.py:257-258`, `letta/orm/sqlalchemy_base.py:501-505`, `letta/orm/sqlalchemy_base.py:807-810` |
| Identity scoping | Unique `(identifier_key, project_id, organization_id)` | `letta/orm/identity.py:23-33` |
| Identity type taxonomy | `IdentityType.org` / `user` / `other` | `letta/schemas/identity.py:11-18` |
| Conversation FK + org | `Conversation.OrganizationMixin`, `agent_id` FK, `conv-` prefix | `letta/orm/conversation.py:22-61` |
| Conversation defaults on create | `conversation.organization_id = actor.organization_id` | `letta/services/conversation_manager.py:67-101` |
| Conversation lock (Redis) | `acquire_conversation_lock` / `release_conversation_lock`, prefix `conversation:lock:` | `letta/data_sources/redis_client.py:194-249`, `letta/constants.py:474-475` |
| Conversation uses lock | Router takes lock per `conversation_id` before processing | `letta/server/rest_api/routers/v1/conversations.py:338` |
| Run ↔ conversation scoping | `Run.conversation_id` optional FK, indexed `(organization_id, conversation_id)` | `letta/orm/run.py:22-77` |
| Run creation sets org + project | `run.organization_id = organization_id; run.project_id = agent.project_id` | `letta/services/run_manager.py:48-90` |
| Run list filters by org | `RunModel.organization_id == actor.organization_id` | `letta/services/run_manager.py:166-167` |
| Conversation list filters by org | `ConversationModel.organization_id == actor.organization_id` | `letta/services/conversation_manager.py:390-393` |
| Message manager org filter | `delete_all_messages_for_agent_async` filters by `MessageModel.organization_id == actor.organization_id` | `letta/services/message_manager.py:1056-1062` |
| Step list filters by org | `filter_kwargs = {"organization_id": actor.organization_id}` | `letta/services/step_manager.py:62-87` |
| Identity list filters by org | `filters = {"organization_id": actor.organization_id}` | `letta/services/identity_manager.py:51-82` |
| Identity cross-tenant 403 | `existing_identity.organization_id != actor.organization_id → HTTPException(403)` | `letta/services/identity_manager.py:177-178` |
| Cross-tenant bulk-update skip | `bulk_update_block_values_async` logs `"skipping during bulk update"` for foreign-org IDs | `tests/test_managers.py:6207-6240` |
| Cross-tenant list isolation test | `test_get_blocks_comprehensive` proves other-org blocks are invisible | `tests/test_managers.py:5964-6017` |
| Actor resolution from header | `get_headers(actor_id: user_id)` validates `user-` prefix | `letta/server/rest_api/dependencies.py:38-83` |
| Actor fallback (default user) | `get_actor_or_default_async` returns default user when `actor_id` is None | `letta/services/user_manager.py:111-135` |
| Default user id | `user-00000000-0000-4000-8000-000000000000` | `letta/services/user_manager.py:24-25` |
| Disable default-actor mode | `settings.no_default_actor` raises `NoResultFound` if header missing | `letta/settings.py:466-469`, `letta/services/user_manager.py:122-124` |
| Server bootstrap creates default org+user | `init_async` calls `create_default_organization_async` + `create_default_actor_async` | `letta/server/server.py:374-417` |
| `user_id` header → `actor_id` | `HeaderParams.actor_id` resolved before every router | `letta/server/rest_api/dependencies.py:26-35`, `letta/server/rest_api/routers/v1/agents.py:184` |
| Routers must opt into actor | `actor = await server.user_manager.get_actor_or_default_async(actor_id=headers.actor_id)` is repeated in every router | `letta/server/rest_api/routers/v1/conversations.py:79`, `letta/server/rest_api/routers/v1/runs.py:81`, `letta/server/rest_api/routers/v1/agents.py:184` |
| Validate agent exists in actor org | `validate_agent_exists_async` requires `AgentModel.organization_id == actor.organization_id` | `letta/services/helpers/agent_manager_helper.py:1313-1332` |
| Conversation fork | `fork_conversation` shares message rows; new `conversation_id` | `letta/services/conversation_manager.py:103-172` |
| Isolated (per-conversation) blocks | `blocks_conversations` junction, `unique_conversation_block`, `unique_label_per_conversation` | `letta/orm/blocks_conversations.py:1-30` |
| Isolated blocks are cloned copies | `_create_isolated_blocks` makes a new `Block` with same label/value, separate id | `letta/services/conversation_manager.py:894-957` |
| Apply isolated blocks to agent state | `apply_isolated_blocks_to_agent_state` overrides per-conversation in-memory only | `letta/services/conversation_manager.py:984-1023` |
| Archival passages org-scoped | `ArchivalPassage.organization_id`, `archive_id` FK | `letta/orm/passage.py:21-46`, `letta/orm/passage.py:76-104` |
| Source passages org-scoped | `SourcePassage.organization_id`, `file_id`, `source_id` FKs | `letta/orm/passage.py:48-73` |
| Conversation search tool | Agent-side `conversation_search(query, roles, dates)` filters by `agent_id` via `self.user` actor | `letta/functions/function_sets/base.py:87-161` |
| Group manager org filter | `GroupModel.apply_access_predicate(..., AccessType.ORGANIZATION)` | `letta/services/group_manager.py:42-45` |
| Sandbox config manager org filter | `organization_id=actor.organization_id` on create | `letta/services/sandbox_config_manager.py:84`, `letta/services/sandbox_config_manager.py:177` |
| `created_by_id` / `last_updated_by_id` plumbing | Agent create sets `created_by_id=actor.id, last_updated_by_id=actor.id` | `letta/services/agent_manager.py:536-537` |
| Job user+org scoping | `Job` uses `UserMixin` and stores both `user_id` + `organization_id` | `letta/orm/job.py:19-65` |
| `Job` access predicate is per-user | `JobModel.apply_access_predicate(..., AccessType.USER)` | `letta/services/job_manager.py:198`, `letta/services/job_manager.py:449` |
| User ↔ Org FK | `User.organization_id` FK to `organizations.id` | `letta/orm/user.py:13-26` |
| Org cascade to users | `Organization.users` cascade delete-orphan | `letta/orm/organization.py:41` |
| Indexes help org-scoped reads | `ix_runs_organization_id`, `ix_conversations_org_agent`, `ix_messages_org_agent` | `letta/orm/run.py:32`, `letta/orm/conversation.py:28-31`, `letta/orm/message.py:33` |
| Hard org boundary check on identity | "403 Forbidden" if `existing_identity.organization_id != actor.organization_id` | `letta/services/identity_manager.py:177-178` |
| OTID/run mapping for cross-request dedup | `OTID_RUN_PREFIX = "otid:run:"` is per-run, not per-actor | `letta/constants.py:478-479` |

## Answers to Dimension Questions

### 1. What scopes state?

The dominant scope is **`organization_id`**, inherited by every ORM model except `Job` via `OrganizationMixin` (`letta/orm/mixins.py:19-24`) and is the column added by `apply_access_predicate` for ORGANIZATION access (`letta/orm/sqlalchemy_base.py:891-895`). A second, weaker scope is **`user_id`** — only `Job` uses `UserMixin` (`letta/orm/mixins.py:27-32`, `letta/orm/job.py:19`), and `JobManager` queries with `AccessType.USER` (`letta/services/job_manager.py:198`, `letta/services/job_manager.py:449`). Every mutable entity also carries `_created_by_id`/`_last_updated_by_id` (`letta/orm/base.py:30-85`); these are recorded but not enforced as authorization boundaries.

A `project_id` column exists via `ProjectMixin` but is an unconstrained `String` with no FK (`letta/orm/mixins.py:67-72`), used as a free-form tag for filtering and reporting (`letta/services/run_manager.py:68-69`).

The "thread" scope is `Conversation.id` (`conv-<uuid4>`) (`letta/schemas/enums.py:29`), FK-bound to an agent and org (`letta/orm/conversation.py:22-61`). The "run" scope is `Run.id` (`run-<uuid4>`) (`letta/orm/run.py:37`), with optional FK to a conversation (`letta/orm/run.py:55-57`). Identities add a fourth scope `identifier_key` unique within `(project_id, organization_id)` (`letta/orm/identity.py:23-33`).

### 2. Can state leak across users or sessions?

In the **default OSS mode** (no `no_default_actor`), a request with no `user_id` header is silently rewritten to the global default user (`user-00000000-0000-4000-8000-000000000000`, `letta/services/user_manager.py:24-25`), so two anonymous users share the same actor and the same org. This collapses all unauthenticated callers into one effective identity — a real risk if the server is exposed without reverse-proxy auth.

With an actor supplied, the predominant pattern is **`actor.organization_id == row.organization_id`** at every read/write. The plumbed-in places that enforce it:
- `validate_agent_exists_async` (`letta/services/helpers/agent_manager_helper.py:1326-1331`),
- `Run.list_runs` (`letta/services/run_manager.py:166`),
- `Conversation.list_conversations` (`letta/services/conversation_manager.py:390-393`),
- `Message.list_messages` for deletes (`letta/services/message_manager.py:1056-1062`),
- `Step.list_steps` (`letta/services/step_manager.py:64`),
- `IdentityManager.list_identities_async` (`letta/services/identity_manager.py:51-82`) and 403 on cross-org update (`letta/services/identity_manager.py:177-178`).

The leakage risk is **convention**, not storage-enforced. `apply_access_predicate` is only called when an actor is passed (`letta/orm/sqlalchemy_base.py:266-267`, `letta/orm/sqlalchemy_base.py:527-529`); when `actor=None`, only a `logger.warning("SECURITY: ... bypasses organization filtering.")` is emitted (`letta/orm/sqlalchemy_base.py:257-258`, `letta/orm/sqlalchemy_base.py:501-505`, `letta/orm/sqlalchemy_base.py:807-810`). The DB schema has no `CHECK` constraint or row-level security preventing an `organization_id` mismatch — only the convention of always passing the actor. The `bulk_update_block_values_async` test (`tests/test_managers.py:6207-6240`) shows the pattern: cross-org IDs are silently skipped with a log line, not rejected at the DB level.

### 3. Is cross-thread memory explicit?

Yes, and it is deliberately narrow:
- **Conversation-scoped block overrides**: `isolated_block_labels` causes a clone of the agent's block to be created as a new `Block` row tied to the conversation via `blocks_conversations` (`letta/services/conversation_manager.py:894-957`, `letta/orm/blocks_conversations.py:1-30`). Cross-thread reads of that block require the same conversation id. The uniqueness constraints `unique_conversation_block` and `unique_label_per_conversation` (`letta/orm/blocks_conversations.py:12-13`) prevent accidental re-binding.
- **Conversation forks**: `fork_conversation` creates a new `Conversation` row but reuses the existing `Message` rows (`letta/services/conversation_manager.py:103-172`), so the fork shares history with its source.
- **Cross-thread archival**: `archival_memory_insert` / `archival_memory_search` are agent-scoped (`letta/functions/function_sets/base.py:87-161`); the tool passes `self.user` as actor and Letta scopes results via the `ArchivalPassage` model (`letta/orm/passage.py:76-104`), so cross-thread memory is via "agent", not "conversation".
- **Cross-conversation search via `conversation_search`**: agent-wide over all messages belonging to that agent (`letta/functions/function_sets/base.py:87-161`).

There is no implicit leak: a conversation's `isolated_blocks` only appear when `apply_isolated_blocks_to_agent_state` is called for that conversation (`letta/services/conversation_manager.py:984-1023`), and the message-to-conversation mapping is opt-in via `ConversationMessage` (`letta/orm/conversation_messages.py:15-73`).

### 4. Are tenant boundaries enforced at the storage level?

**No, only structurally.** Every tenant-tagged table has `organization_id String` plus composite indexes that include `organization_id` (`letta/orm/run.py:32`, `letta/orm/conversation.py:28-31`, `letta/orm/message.py:33`, `letta/orm/passage.py:64-66`), and many FKs are composite-scoped (`letta/orm/blocks_conversations.py:12-13`). But there is no PostgreSQL `ROW LEVEL SECURITY`, no `CHECK (organization_id IS NOT NULL)`, and no FK from `organization_id` to `organizations.id` (it is a `String`, not a `ForeignKey`, on the mixin: `letta/orm/mixins.py:24`). Isolation is implemented entirely in application-layer predicates (`apply_access_predicate`, manager `WHERE organization_id = actor.organization_id`). Cascade deletes from `Organization` to its entities (`letta/orm/organization.py:41-80`) provide storage-level lifecycle containment, but the access control layer is application-only.

Cascade deletes on `Organization.users`, `Organization.agents`, `Organization.messages`, etc. (`letta/orm/organization.py:41-80`) mean deleting an org deletes everything in it — including archives, sources, providers, jobs, runs. That is strong isolation, but it is also destructive without undo.

### 5. Can users fork sessions safely?

**For conversations, yes.** `fork_conversation` shares `Message` rows (so both conversations point to the same messages in `messages` and `conversation_messages`), creates a new system message, and writes to a new `Conversation` row with a fresh `conv-<uuid4>` id (`letta/services/conversation_manager.py:103-172`). Because the `ConversationMessage` table has `UniqueConstraint("conversation_id", "message_id")` (`letta/orm/conversation_messages.py:30`), a single message can be associated with many conversations without conflict. The forked conversation inherits the source's `agent_id`, `organization_id`, `model`, `model_settings`, and may carry isolated block labels.

**Concurrency within a single conversation is guarded.** `acquire_conversation_lock` blocks on `conversation_id` for `CONVERSATION_LOCK_TTL_SECONDS = 300` (`letta/constants.py:474-475`, `letta/data_sources/redis_client.py:194-249`), so two simultaneous sends on the same conversation_id see `ConversationBusyError`. When Redis is unavailable the lock primitives are no-ops (`letta/data_sources/redis_client.py:209-210`), so in OSS-without-Redis the conversation concurrency guarantee weakens to "best-effort, single-process".

**For runs, no explicit fork.** A run is created per agent message processing (`letta/services/run_manager.py:48-90`) and is immutable in shape; "forking" is implemented as a new run on a new conversation.

## Architectural Decisions

- **Single dominant tenant axis (`organization_id`).** Every mutable entity except `Job` inherits `OrganizationMixin` (`letta/orm/mixins.py:19-24`). `Job` deliberately uses `UserMixin` so background jobs are per-user, not per-org (`letta/orm/mixins.py:27-32`, `letta/orm/job.py:19`).
- **Pluggable access predicate.** `apply_access_predicate(query, actor, access, access_type)` is the single chokepoint that translates an actor into a `WHERE` clause, with `AccessType.ORGANIZATION` and `AccessType.USER` as the two values (`letta/orm/sqlalchemy_base.py:116-119`, `letta/orm/sqlalchemy_base.py:871-902`). The `access: ["read","write","admin"]` parameter is a documented placeholder for row-level permissions (`letta/orm/sqlalchemy_base.py:890`).
- **Actor as a `User` ORM object, not a string.** Every manager takes `actor: PydanticUser` and reads `actor.organization_id` and `actor.id`. There is no string-typed actor.
- **Default actor fallback.** When `user_id` header is missing and `no_default_actor=False`, the server fabricates the global default user and the global default org (`letta/services/user_manager.py:111-135`, `letta/server/server.py:374-417`). When `no_default_actor=True`, missing header raises `NoResultFound` (`letta/services/user_manager.py:122-124`).
- **Prefix-keyed IDs.** Every primitive type has a UUID4-prefixed id (`letta/schemas/letta_base.py:32-89`), validated by regex (`letta/validators.py:12-19`). The prefix system makes it impossible to confuse a `run-…` with a `user-…` at the type level.
- **Per-conversation memory overrides.** A conversation can request `isolated_block_labels`, which clones the agent's blocks and binds them to that conversation via `blocks_conversations` (`letta/services/conversation_manager.py:894-957`). This is the explicit mechanism for "different conversation → different memory".
- **Conversation lock in Redis.** Cross-process serialization of concurrent conversation sends (`letta/data_sources/redis_client.py:194-249`, `letta/constants.py:474-475`), optional and degraded to no-op when Redis is unavailable.
- **Identity as orthogonal tag.** `Identity` introduces `identifier_key` unique per `(project_id, organization_id)` (`letta/orm/identity.py:23-33`), letting one agent be reachable by multiple external user identifiers without colliding across tenants.
- **Created/Updated by with `user-` prefix enforcement.** `_user_id_setter` asserts `prefix == "user"` (`letta/orm/base.py:81-82`), so audit fields cannot be silently populated with non-user IDs.

## Notable Patterns

- **Actor threaded through manager constructors.** Every service takes `actor: PydanticUser` as a method argument, never reads from a thread-local or global. This makes the call graph reviewable for missing-actor bugs.
- **Centralized actor resolution at the API boundary.** The `HeaderParams` dependency (`letta/server/rest_api/dependencies.py:26-83`) validates the `user_id` header before any router runs, so downstream code can assume a well-formed `actor_id` or a missing one.
- **Identity-as-policy.** Identities are stored separately from users, so a user within one org can own multiple `identifier_key`s (`org`, `user`, `other`) without creating more `User` rows.
- **Conversation lock pattern with TTL.** `CONVERSATION_LOCK_TTL_SECONDS = 300` (`letta/constants.py:475`) — auto-release on crash. The lock is non-blocking (`blocking=False` at `letta/data_sources/redis_client.py:217`), so callers fail fast with `ConversationBusyError`.
- **List-then-page composition.** `SqlalchemyBase._list_preprocess` and `_read_multiple_preprocess` are split so the actor predicate, the `WHERE` clauses, and the `LIMIT` are composed at one place (`letta/orm/sqlalchemy_base.py:230-411`, `letta/orm/sqlalchemy_base.py:487-535`).
- **Project id is a free-form tag, not a tenant.** No FK, no separate `Project` table. It is a header-supplied grouping label (`letta/server/rest_api/dependencies.py:41-42`, `letta/orm/mixins.py:67-72`), which means two different "projects" in two different orgs can share an id without conflict.

## Tradeoffs

- **Default-actor fallback collapses unauthenticated callers.** Two requests with no `user_id` header from different end-users both resolve to the same `user-00000000-0000-4000-8000-000000000000` (`letta/services/user_manager.py:24-25`, `letta/services/user_manager.py:111-135`). They share org and user state. The escape hatch is `settings.no_default_actor=True` (`letta/settings.py:466-469`), but it is opt-in.
- **App-layer predicate only.** A new query path that omits `actor=` is silently cross-org; only a log line warns (`letta/orm/sqlalchemy_base.py:257-258`). There is no DB-level guard.
- **`organization_id` is `String`, not FK.** The mixin defines `String, ForeignKey("organizations.id")` (`letta/orm/mixins.py:24`), but several tables redeclare `organization_id` as `String` without FK (e.g., `Job.organization_id` at `letta/orm/job.py:43`). Storage-level referential integrity is uneven.
- **`access` parameter is `del`-ed.** The `apply_access_predicate` implementation immediately discards `access` with `del access` (`letta/orm/sqlalchemy_base.py:890`) and defaults to "same org, all permissions". Granular per-row permissions are an entry point only.
- **Project is an unconstrained tag.** `project_id` has no FK (`letta/orm/mixins.py:67-72`), so a typo creates a "project" nobody can enumerate or clean up. The cost of "just a tag" is no referential integrity.
- **Conversation lock requires Redis.** When Redis is absent, locks degrade to no-ops (`letta/data_sources/redis_client.py:209-210`), so in single-process OSS without Redis the conversation concurrency guarantee is weakened.
- **Identities are deprecated for `agent_ids`/`block_ids`.** Schema still carries `agent_ids: List[str] = Field(..., deprecated=True)` (`letta/schemas/identity.py:50-51`), so dual sources of truth for "which agent belongs to which identity" exist.
- **`Job` uses `UserMixin` and `Job.organization_id` is nullable** (`letta/orm/job.py:43`). Pre-org jobs may have `organization_id IS NULL`, so a `Job` without an org is not a tenant escape — it just lives outside the org scope and is queryable only via `AccessType.USER`.

## Failure Modes / Edge Cases

- **Silent default-user sharing.** With `no_default_actor=False` and no reverse-proxy auth, all anonymous calls share one global identity (`letta/services/user_manager.py:111-135`).
- **Cross-org read on missing actor.** Code path that calls `read_async(..., actor=None)` returns the row regardless of org and logs a warning rather than raising (`letta/orm/sqlalchemy_base.py:501-505`).
- **Conversation lock release swallowed on terminal update.** `release_conversation_lock` swallows exceptions (`letta/services/run_manager.py:391-396`), so if Redis is down the lock will expire after `CONVERSATION_LOCK_TTL_SECONDS` instead of being cleaned up — a stuck-conversation window.
- **Forked conversation shares messages.** `fork_conversation` does not copy messages; it adds a new `ConversationMessage` row (`letta/services/conversation_manager.py:142-158`). Editing/deleting a message in the source will affect the fork, which may not be obvious to API callers.
- **Sequence IDs and pagination.** `list_messages` uses `sequence_id` cursors (`letta/services/message_manager.py:1000-1024`). The `sequence_id` is global per org (monotonic), so a leaked `message_id` from another org would still be queryable via `before`/`after` cursors if the actor check is bypassed.
- **Job user+org collision.** A job is owned by one user but belongs to an org. If a user moves between orgs, their old jobs remain in the old org (`letta/orm/job.py:43`).
- **`identifier_key` collisions across orgs are allowed.** Identity uniqueness is `(identifier_key, project_id, organization_id)` (`letta/orm/identity.py:23-33`), so two orgs can have an identity with the same `identifier_key` and the system has no way to disambiguate them globally.
- **Bulk operations skip cross-org IDs silently.** `bulk_update_block_values_async` logs `"skipping during bulk update"` for foreign-org IDs but still returns a partial result (`tests/test_managers.py:6207-6240`). A client that doesn't read logs will not see the gap.
- **OTID/run mapping is per-`run_id`**, not per-actor, so cross-request dedup keys on the run alone (`letta/constants.py:478-479`).
- **No `is_deleted` filter on every read.** `_read_multiple_preprocess` only applies `is_deleted=False` if `check_is_deleted=True` is passed (`letta/orm/sqlalchemy_base.py:531-533`). Some callers omit it, so soft-deleted rows can still appear.

## Future Considerations

- **Move isolation to PostgreSQL RLS.** Define row-security policies on every tenant-scoped table keyed on a session-level `letta.organization_id`, so DB rejects cross-org reads even if `actor=` is omitted (`letta/orm/sqlalchemy_base.py:257-258`).
- **Default to `no_default_actor=True`.** The default-user fallback (`letta/services/user_manager.py:111-135`) is convenient for OSS but is the easiest way to leak; flip the default and keep `no_default_actor=False` as the OSS-only escape.
- **Materialize `Project` as a real entity.** The free-form `project_id` string (`letta/orm/mixins.py:67-72`) has no referential integrity, no quota, and no lifecycle. A `projects` table with `project_id` FK would make per-project isolation, billing, and cleanup first-class.
- **Implement the `access` parameter.** Currently `del access` (`letta/orm/sqlalchemy_base.py:890`); once per-row permissions exist, the same `apply_access_predicate` switch becomes the read/write/admin gateway.
- **Make `organization_id` a real FK in `Job`.** Today `Job.organization_id` is a bare `String` (`letta/orm/job.py:43`); promoting it to `ForeignKey("organizations.id")` aligns it with every other table.
- **Auto-expire soft-deleted rows.** `is_deleted` is preserved but not vacuumed, so soft-deleted conversation/run rows persist indefinitely (`letta/orm/sqlalchemy_base.py:18`, `letta/orm/base.py:18`).
- **Provide a "thread migration" tool.** Forks share messages (`letta/services/conversation_manager.py:103-172`) and isolation is via `isolated_block_labels`; a primitive to "promote a conversation to its own agent" would let users safely graduate a thread without re-architecting.
- **Enforce conversation lock in single-process too.** When Redis is missing, `acquire_conversation_lock` returns `None` (`letta/data_sources/redis_client.py:209-210`); an in-process `asyncio.Lock` keyed by `conversation_id` would close the gap for OSS.

## Questions / Gaps

- **What happens when `api_key_to_user` resolves to a user from another org?** The header-to-actor path (`letta/server/rest_api/auth_token.py:11-21`) returns a user_id but does not state whether the actor's `organization_id` is recomputed from the user record. `server.api_key_to_user` is referenced but the implementation was not located in this search boundary; downstream callers re-fetch the actor via `get_actor_or_default_async`, which would rehydrate `organization_id` from the User row — but if the implementation returns a different user than the request header implies, isolation depends on which field wins.
- **Where is `apply_access_predicate` called vs. not?** Some service-level queries (e.g., `Message.list_messages` at `letta/services/message_manager.py:895-1042`) bypass `apply_access_predicate` and rely on agent-existence validation (`validate_agent_exists_async` at `letta/services/helpers/agent_manager_helper.py:1313-1332`) as the gate. If a user supplies an `agent_id` from their own org, the messages are scoped by that agent — but if `agent_id` is None and only `actor` is provided, the query does not add an `actor.organization_id` predicate explicitly. The behaviour with no `agent_id` is to return all messages for any agent that the actor can see, which is correct only because messages inherit agent->org scoping implicitly via join. No clear evidence found that this is tested for cross-org leakage.
- **Is there a test for two users with similar runs sharing state?** Searched for `test_cross`/`test_isolation`/`test_leak` in the test suite; only `test_handle_uniqueness_per_org` (`tests/test_server_providers.py:1585-1640`) and `test_get_blocks_comprehensive` (`tests/test_managers.py:5964-6017`) appear. There is no direct cross-user-on-same-agent test, and `test_handle_uniqueness_per_org` only asserts provider handles can repeat across orgs. No clear evidence found of a focused "two users, same agent name, no leakage" test.
- **Job ↔ org migration.** `Job.organization_id` is nullable (`letta/orm/job.py:43`), so a job created before a user had an org or while org was being assigned will not be org-scoped. No clear evidence found of a backfill migration that enforces org on every job.
- **Identity ↔ user relationship.** `Identity` is keyed by `identifier_key` not by `user_id` (`letta/orm/identity.py:23-33`). The link "this identity = this user" is implicit through `identifier_key` strings; there is no FK. No clear evidence found of how an external system reconciling identities to users does so safely when `identifier_key` collisions are allowed across orgs.

---

Generated by `02.07-session-thread-user-boundaries` against `letta`.