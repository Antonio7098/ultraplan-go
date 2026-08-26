# Source Analysis: letta

## 05.07 Memory Privacy, Scope, and Deletion

### Source Info

| Field | Value |
|-------|-------|
| Name | letta |
| Path | `studies/agent-harness-study/sources/letta` |
| Language / Stack | Python (FastAPI server, SQLAlchemy ORM on PostgreSQL/SQLite, optional Turbopuffer vector DB, optional git-backed memory repo) |
| Analyzed | 2026-08-26 |

*All citations below are relative to the letta repository root.*

## Summary

Letta (the MemGPT successor) treats the **organization** as the primary tenant boundary for all memory primitives: core-memory blocks (`letta/orm/block.py`), archival passages (`letta/orm/passage.py`), archives (`letta/schemas/archive.py:16`), and messages all carry a mandatory `organization_id` via `OrganizationMixin` (`letta/orm/mixins.py:19-24`). Scoping is enforced centrally in `SqlalchemyBase.apply_access_predicate` (`letta/orm/sqlalchemy_base.py:871-902`), which injects `WHERE organization_id == actor.organization_id` (or `user_id == actor.id`) into list/read/bulk-delete queries, and cross-org isolation is explicitly tested (`tests/managers/test_block_manager.py:303-355`, `tests/managers/test_block_manager.py:733-765`). Deletion is comprehensive and mostly *hard delete*: blocks, passages, messages, conversations, agents, users, and organizations all expose manager-level delete methods plus REST endpoints, with dual deletion propagated to the Turbopuffer vector store (`letta/services/passage_manager.py:767-796`) and prefix-deletion for the git-backed memory repo (`letta/services/memory_repo/git_operations.py:629-638`).

The privacy story is narrower. There are **no privacy filters or PII redaction for memory content**: block values, passage text, and message text are stored as plaintext columns (`letta/orm/passage.py:28`); the only scrubbing mechanisms target LLM reasoning traces (`letta/helpers/reasoning_helper.py:25`) and export sanitization (`letta/services/agent_serialization_manager.py:196`, `347`). Encryption exists but only for *credentials* — AES-256-GCM via `CryptoUtils` (`letta/helpers/crypto_utils.py:44-59`) backing `_enc` ORM columns for provider keys, MCP tokens, OAuth secrets, and sandbox env vars — and it silently degrades to plaintext storage when `LETTA_ENCRYPTION_KEY` is unset (`letta/schemas/secret.py:59-68`; default key is `None`, `letta/settings.py:449`). Retention policies for memory content are absent; no TTL/expiry mechanism touches blocks, passages, or messages. Auditing is limited to attribution columns (`created_by_id`/`last_updated_by_id`, `letta/orm/base.py:30-35`) and block-checkpoint actor records (`letta/orm/block_history.py:35-37`) — there is no audit log of memory reads, writes, or deletes.

## Rating

**6 / 10** — Tenant scoping is a clear, consistently applied model with dedicated isolation tests (7–8 territory on its own), and deletion APIs are thorough including vector-store cleanup. However, the dimension's full scope — privacy filters, retention policy, encryption of sensitive *memory*, and access auditing — is only partially present: no content redaction, no retention configuration, credential-only encryption with a plaintext fallback, no read/write audit trail, and a default-actor fallback that concentrates all anonymous traffic into one shared org/user. Per the rubric this lands in "present but inconsistent … fragile" at the edges, lifted by the strong scoping core.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Org scope mixin | `OrganizationMixin` adds FK `organization_id` used by blocks/passages/messages/archives | `letta/orm/mixins.py:19-24` |
| Access types | `AccessType` enum = ORGANIZATION \| USER | `letta/orm/sqlalchemy_base.py:116-119` |
| Scope enforcement | `apply_access_predicate` injects org/user WHERE clause; access levels (read/write/admin) currently ignored ("entrypoint for row-level permissions") | `letta/orm/sqlalchemy_base.py:871-902` |
| Missing-actor guard | Logs "SECURITY: Listing org-scoped model without actor" warning when actor absent | `letta/orm/sqlalchemy_base.py:256-258` |
| Actor resolution | REST routers derive actor from client-supplied `user_id` header via `get_actor_or_default_async`; falls back to shared `DEFAULT_USER_ID` unless `no_default_actor=True` | `letta/server/rest_api/dependencies.py:38-54`; `letta/services/user_manager.py:113-135`; `letta/settings.py:466-469` |
| Server auth | Single shared password gate (`X-BARE-PASSWORD` / Bearer); bearer token maps to admin user or API-key→user | `letta/server/rest_api/middleware/check_password.py:23-31`; `letta/server/rest_api/auth_token.py:11-22` |
| Cross-org isolation tests | Blocks created in two orgs invisible to each other; bulk update skips other-org block with warning | `tests/managers/test_block_manager.py:303-355`; `tests/managers/test_block_manager.py:733-765`; fixture `tests/managers/conftest.py:104-127` |
| Block sharing semantics | Blocks attach to multiple agents via `blocks_agents` pivot within an org; `read_only` flag marks agent-immutable blocks | `letta/orm/blocks_agents.py`; `letta/schemas/block.py:36` |
| Read-only enforcement | Memory tools raise on writes to `read_only` blocks | `letta/services/tool_executor/core_tool_executor.py:320-321`, `336-337`, `354-355` |
| Visibility filter | `hidden` flag excluded from listings unless `show_hidden_blocks=True` (visibility, not security) | `letta/schemas/block.py:41-44`; `letta/services/block_manager.py:350-351` |
| Privacy scrubbing (reasoning only) | `scrub_inner_thoughts_from_messages` strips hidden CoT from context when reasoning disabled | `letta/helpers/reasoning_helper.py:25`; usage `letta/agents/letta_agent.py:1660-1661` |
| Export scrubbing | `scrub_messages` excludes all messages from agent serialization/export; MCP auth scrubbed | `letta/services/agent_serialization_manager.py:196-246`, `347`, `381-392`; REST param `letta/server/rest_api/routers/v1/agents.py:310` |
| Credential encryption | AES-256-GCM + PBKDF2(100k) `CryptoUtils`; async variants run in dedicated crypto thread pool | `letta/helpers/crypto_utils.py:44-101` |
| Encrypted columns | `_enc` columns for provider keys, MCP tokens/headers, OAuth codes/tokens, sandbox env vars | `letta/orm/provider.py:42-43`; `letta/orm/mcp_server.py:44-50`; `letta/orm/mcp_oauth.py:38-54`; `letta/orm/sandbox_config.py:49-53` |
| Plaintext fallback | Without `LETTA_ENCRYPTION_KEY` (default None), Secrets stored as plaintext with warning only | `letta/schemas/secret.py:59-68`; `letta/settings.py:449` |
| Secret redaction | `Secret.__str__/__repr__` never expose value | `letta/schemas/secret.py:271-279` |
| No content encryption/redaction | Passage text is a plain `Mapped[str]` column; no PII filter found anywhere in memory paths | `letta/orm/passage.py:28`; search across `letta/**` for redact/PII/anonymize returned only reasoning-redaction and credential hits |
| Hard delete primitive | `hard_delete_async` performs real row removal (soft `delete_async` exists, rarely used by managers) | `letta/orm/sqlalchemy_base.py:674-706` |
| Bulk scoped delete | `bulk_hard_delete_async` re-applies `apply_access_predicate` on raw DELETE | `letta/orm/sqlalchemy_base.py:708-740` |
| Block deletion | Deletes pivot rows + tags then hard-deletes block; history cascades via FK `ondelete=CASCADE` | `letta/services/block_manager.py:275-286`; `letta/orm/block_history.py:40-44` |
| Passage deletion w/ vector cleanup | SQL hard delete + Turbopuffer `delete_passage`; `strict_mode` re-raises on vector-store failure | `letta/services/passage_manager.py:767-796`; `letta/helpers/tpuf_client.py:1563-1625` |
| Message deletion | Single + bulk + per-agent deletes include explicit `organization_id == actor.organization_id` predicate and Turbopuffer recall cleanup | `letta/services/message_manager.py:822-850`, `1045-1090` |
| Archive & passage REST deletes | `DELETE /v1/archives/{archive_id}` and `/passages/{passage_id}` endpoints | `letta/server/rest_api/routers/v1/archives.py:161-162`, `258-259` |
| Agent deletion gap | `delete_agent_async` removes agent (+ sleeptime group) but never calls archive cleanup → orphaned archives | `letta/services/agent_manager.py:1320-1396` (no `archive_manager` reference) |
| Org deletion | Bare `hard_delete_async(org)`; child FKs declare no `ondelete` on `organization_id` (`mixins.py:24`) so cascade behavior is DB-dependent | `letta/services/organization_manager.py:79-83` |
| Vector namespaces scoped | Messages/tools namespaced by `organization_id`; archival passages namespaced per-archive | `letta/helpers/tpuf_client.py:312-350` |
| Git memory repo deletion | `delete_repo(agent_id, org_id)` prefix-deletes storage; storage backends implement `delete_prefix` | `letta/services/memory_repo/git_operations.py:629-638`; `letta/services/memory_repo/storage/base.py:58-82` |
| Git HTTP proxy authz | Resolves actor, verifies agent access, forwards authenticated `X-Organization-Id` to memfs | `letta/server/rest_api/routers/v1/git_http.py:282-294` |
| Audit trail (partial) | Every record stores `created_by_id`/`last_updated_by_id`; block checkpoints store `actor_type`/`actor_id`; OTEL ClickHouse traces framed for "auditing" | `letta/orm/base.py:30-35`; `letta/orm/block_history.py:35-37`; `letta/services/llm_trace_reader.py:3`, `87` |
| No audit log table | Grep for `audit` across `letta/**` found no audit-log schema/table or write hooks | search boundary: `letta/` package, pattern `audit\|Audit` |
| No retention config | Grep `retention\|ttl\|expir` surfaced only OAuth-session cleanup, Redis stream TTLs, run expiry, prompt-cache retention — nothing for blocks/passages/messages | `letta/services/mcp_server_manager.py:1267-1283`; `letta/server/rest_api/redis_stream_manager.py:42`; `letta/server/rest_api/routers/v1/runs.py:372` |

## Answers to Dimension Questions

1. **Can memory leak between users?** Across organizations, no — every query path funnels through `apply_access_predicate` (`letta/orm/sqlalchemy_base.py:891-900`), bulk deletes re-check scope (`letta/orm/sqlalchemy_base.py:726-728`), raw DELETE statements add explicit org predicates (`letta/services/message_manager.py:1059-1061`), and cross-org invisibility is asserted by tests (`tests/managers/test_block_manager.py:349-355`). Within an organization, yes by design: any actor can read/write any org memory because the `access` argument is discarded (`letta/orm/sqlalchemy_base.py:890`) — there is no per-user row-level separation inside an org. Additionally, the self-hosted default resolves unknown/missing `user_id` headers to a single shared default actor/org (`letta/services/user_manager.py:122-133`), so out-of-the-box multi-user separation depends entirely on clients sending correct headers behind the one shared server password.
2. **Can users delete memory?** Yes, extensively: blocks (`letta/services/block_manager.py:275`), archival passages incl. vector-store dual-delete (`letta/services/passage_manager.py:767`), whole archives (`letta/services/archive_manager.py:266`), messages individually/in-bulk/per-agent (`letta/services/message_manager.py:822`, `1094`, `1045`), conversations (`letta/services/conversation_manager.py:556`), agents (`letta/services/agent_manager.py:1320`), and entire users/orgs (`letta/services/user_manager.py:79`; `letta/services/organization_manager.py:79`). All are hard deletes; exports can also strip messages via `scrub_messages` (`letta/server/rest_api/routers/v1/agents.py:310`).
3. **Is sensitive data stored?** Memory *content* (block values, passage/message text) is stored plaintext with no redaction pipeline (`letta/orm/passage.py:28`). Credentials are encrypted at rest **only if** `LETTA_ENCRYPTION_KEY` is configured; otherwise `Secret.from_plaintext` logs a warning and stores plaintext in the `_enc` column (`letta/schemas/secret.py:55-68`).
4. **Is memory access audited?** Not systematically. Attribution columns record who created/last-updated each row (`letta/orm/base.py:30-35`), block checkpoints record editor identity (`letta/orm/block_history.py:35-37`), and OTEL/ClickHouse traces support post-hoc debugging (`letta/services/llm_trace_reader.py:87`) — but there is no audit log capturing memory reads, deletes, or failed access attempts. No evidence found despite searching for `audit` across the package.
5. **Are scopes enforced in queries?** Yes — centrally and consistently. The ORM base applies the access predicate on list/read/size/bulk-delete paths (`letta/orm/sqlalchemy_base.py:265-267`, `871-902`, `726-728`), managers add redundant org filters (`letta/services/block_manager.py:324-327`), the Turbopuffer layer partitions data into org-scoped namespaces for messages/tools and archive-scoped namespaces for passages (`letta/helpers/tpuf_client.py:312-350`), and the git-http memory-repo proxy re-verifies agent access before forwarding an authenticated org header (`letta/server/rest_api/routers/v1/git_http.py:284-294`). A logged SECURITY warning fires if an org-scoped query is attempted without an actor (`letta/orm/sqlalchemy_base.py:256-258`).

## Architectural Decisions

- **Org-first tenancy, not user-first.** All memory tables inherit `OrganizationMixin` (`letta/orm/mixins.py:19-24`), and the default access type is `AccessType.ORGANIZATION` (`letta/orm/sqlalchemy_base.py:144`). Users (actors) exist mainly for attribution; memory visibility stops at the org wall. This matches Letta's product model where agents/archives/blocks are shared team resources within a workspace.
- **Centralized predicate instead of scattered WHERE clauses.** Rather than trusting every call site, the ORM base class applies `apply_access_predicate` inside `list_async`/`_list_preprocess`/`bulk_hard_delete_async` (`letta/orm/sqlalchemy_base.py:265-267`, `726-728`). Managers may still double-filter (e.g., `letta/services/block_manager.py:327`), giving defense-in-depth at slight redundancy cost.
- **Row-level permissions scaffolded but not implemented.** The `access` parameter (read/write/admin) is accepted everywhere yet explicitly discarded today (`letta/orm/sqlalchemy_base.py:890`) — a deliberate extension point, meaning intra-org permissions are currently all-or-nothing.
- **Hard delete as the deletion contract.** Deletion APIs physically remove rows rather than tombstoning (`is_deleted` soft delete exists at `letta/orm/base.py:18` and `delete_async` at `letta/orm/sqlalchemy_base.py:674-682` but managers overwhelmingly use `hard_delete_async`). This satisfies "right to erasure" expectations but makes accidental deletion unrecoverable except via block-history checkpoints.
- **Dual-write consistency with the vector store, best-effort by default.** Passages live in both SQL and optionally Turbopuffer; deletions propagate to both, with `strict_mode` opting into failing closed when the vector delete fails (`letta/services/passage_manager.py:783-792`). Default behavior tolerates divergence (logged only) — a deliberate availability-over-consistency tradeoff.
- **App-layer envelope encryption for credentials only.** `Secret` + `CryptoUtils` encrypt provider/MCP/OAuth/env-var values with AES-256-GCM (`letta/helpers/crypto_utils.py:104-150`), keeping plaintext out of ORM serialization and logs (`letta/schemas/secret.py:271-279`), while memory content relies on deployment-level disk encryption.

## Notable Patterns

- **Mixin-driven scoping.** `OrganizationMixin`, `UserMixin`, `AgentMixin`, `ArchiveMixin`, etc. (`letta/orm/mixins.py:19-98`) standardize ownership columns; cascade behavior is expressed declaratively via FK `ondelete="CASCADE"` (e.g., `letta/orm/block_history.py:40-44`, `letta/orm/mixins.py:40`).
- **Actor-threading convention.** Nearly every service method takes an explicit `actor: PydanticUser` parameter that flows into access predicates and attribution columns (e.g., `letta/services/block_manager.py:275`, `805`), making the acting principal visible at each layer.
- **Test-as-specification for isolation.** Dedicated fixtures build a second org and user (`tests/managers/conftest.py:104-127`), and tests assert both directions of invisibility plus skip-with-warning behavior for bulk updates touching foreign-org IDs (`tests/managers/test_block_manager.py:303-355`, `733-765`).
- **Prefix-namespaced external stores.** Both Turbopuffer namespaces (`messages_{org}`, `tools_{org}`, per-archive namespaces; `letta/helpers/tpuf_client.py:312-350`) and git memory repos (`_repo_path(agent_id, org_id)` + `delete_prefix`; `letta/services/memory_repo/git_operations.py:636-637`) encode tenancy into storage topology so scope checks survive outside SQL.
- **Heuristic-encrypted-value detection.** `CryptoUtils.is_encrypted` uses base64 length heuristics with an allowlist of known plaintext API-key prefixes to avoid double-encrypting credentials (`letta/helpers/crypto_utils.py:21-41`, `304-322`).

## Tradeoffs

- **Shared-block concurrency vs. isolation:** a block linked to many agents is a single mutable row for the whole org (`letta/orm/blocks_agents.py`); edits by one agent (or developer) propagate to every connected agent's system prompt (`letta/services/block_manager.py:61-68`). Efficient collaboration, but one writer changes everyone's memory.
- **Best-effort vector deletion vs. guaranteed erasure:** non-strict mode leaves deleted passages recoverable in Turbopuffer after partial failure (`letta/services/passage_manager.py:789-792`) — better uptime, weaker erasure guarantees.
- **Plaintext fallback vs. boot friction:** requiring `LETTA_ENCRYPTION_KEY` would break existing deployments, so missing-key installs silently keep credentials readable in the DB (`letta/schemas/secret.py:60-68`).
- **Default-actor convenience vs. accountability:** auto-fallback to `DEFAULT_USER_ID` makes local development frictionless but collapses attribution and lets unscoped clients operate under one identity unless operators set `no_default_actor=true` (`letta/services/user_manager.py:122-135`).
- **Org-wall simplicity vs. user privacy:** org-wide readability maximizes agent-team utility but means individual end-user data contributed to shared blocks/archives is visible to every actor in the org; there is no per-user ACL layer yet.

## Failure Modes / Edge Cases

- **Orphaned archives on agent deletion.** `delete_agent_async` (`letta/services/agent_manager.py:1320-1396`) never detaches or deletes the agent's archive, so archival memories persist after the agent is gone (possibly intended for shared archives, but silent for owned ones — `attach ... is_owner=True` at `letta/services/archive_manager.py:541-547`).
- **Org deletion cascade ambiguity.** `delete_organization_by_id_async` hard-deletes only the org row (`letta/services/organization_manager.py:79-83`); child FKs on `organization_id` declare no `ondelete` action (`letta/orm/mixins.py:24`), so outcome ranges from FK-violation errors to orphaned memory rows depending on dialect/ordering.
- **Vector-store drift.** If a Turbopuffer delete fails in non-strict mode, deleted content remains searchable in the vector namespace while gone from SQL (`letta/services/passage_manager.py:789-792`); bulk message deletion with `exclude_ids` is acknowledged as not fully supported upstream (`letta/services/message_manager.py:1080-1082`).
- **Unauthenticated-by-default posture.** With `no_default_actor=False` and a single shared password, any holder of the server password can act as the default actor and reach the default org's entire memory corpus (`letta/server/rest_api/middleware/check_password.py:23-31`; `letta/services/user_manager.py:126-133`).
- **Decryption-key loss.** Credentials encrypted under a lost/rotated `LETTA_ENCRYPTION_KEY` become undecryptable; decryption failures raise only at use time (`letta/schemas/secret.py:208-210`), with no key-versioning found (No evidence found for key rotation machinery).
- **Hidden ≠ protected.** The `hidden` flag merely filters listings (`letta/services/block_manager.py:350-351`); hidden blocks remain fully addressable by ID and are injected into agent context.

## Future Considerations

- Implement the scaffolded row-level permissions: honor the `access` list in `apply_access_predicate` (`letta/orm/sqlalchemy_base.py:884-890`) to enable per-user/per-role memory visibility inside an org.
- Add an append-only audit log (who read/wrote/deleted which memory primitive) alongside the existing attribution columns and OTEL traces.
- Introduce configurable retention/TTL for recall messages and archival passages (currently nothing expires them; contrast with OAuth-session cleanup at `letta/services/mcp_server_manager.py:1267`).
- Fail closed on secret handling: make `LETTA_ENCRYPTION_KEY` required (or gate startup) instead of the plaintext fallback at `letta/schemas/secret.py:59-68`.
- Clean up or explicitly detach owned archives during agent deletion; define and test org-deletion cascade semantics for all memory tables.
- Consider strict-by-default vector-store deletion (flip `strict_mode` default) so erasure failures surface to callers.

## Questions / Gaps

- No privacy/PII filtering layer for memory content was found (searched `redact|sensitive|privacy|PII|scrub|anonymi` across `letta/**`; only reasoning-trace scrubbing, export scrubbing, and credential encryption matched). Whether Letta Cloud adds such filtering above this open-source layer cannot be determined from this source.
- Whether `api_key_to_user` provides real per-key identity binding: it is referenced in `letta/server/rest_api/auth_token.py:17` and `letta/server/rest_api/auth/index.py:36` but its implementation was not located in this snapshot (searched `def api_key_to_user` across the package). The OSS server's effective auth surface appears to be the shared-password middleware plus client-supplied `user_id` headers.
- No evidence found of backup/restore tooling or GDPR-style export-per-user workflows beyond the agent-level `scrub_messages` export (`letta/server/rest_api/routers/v1/agents.py:310`).
- Encryption coverage of the externalized stores (Turbopuffer namespaces, memfs git objects) could not be verified from this source; only SQL-side `_enc` columns are demonstrably encrypted.

---

Generated by `05.07-memory-privacy-scope-and-deletion` against `letta`.
