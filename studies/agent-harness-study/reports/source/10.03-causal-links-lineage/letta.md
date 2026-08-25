# Source Analysis: letta

## Causal Links and Lineage

### Source Info

| Field | Value |
|-------|-------|
| Name | letta |
| Path | `studies/agent-harness-study/sources/letta` |
| Language / Stack | Python (FastAPI server, SQLAlchemy ORM, Pydantic schemas, PostgreSQL/SQLite; optional ClickHouse + OpenTelemetry) |
| Analyzed | 2026-08-25 |

## Summary

Letta implements causal lineage as a first-class relational data model rather than an ad-hoc logging afterthought. Every persisted message row carries foreign keys to the `step` that produced it and the `run` it belongs to (`studies/agent-harness-study/sources/letta/letta/orm/message.py:48-53`), every step records the exact provider/model configuration used (`studies/agent-harness-study/sources/letta/letta/orm/step.py:34-52`), and provider traces capture the full LLM request/response payloads keyed by `step_id`, `run_id`, and `agent_id` (`studies/agent-harness-study/sources/letta/letta/schemas/provider_trace.py:46-66`). Tool results are linked to the assistant's tool calls via OpenAI-style `tool_call_id`s stored on both sides of the exchange (`studies/agent-harness-study/sources/letta/letta/schemas/message.py:287-295`). Approvals reference the originating tool call by required `tool_call_id` and the approval-request message by `approval_request_id` (`studies/agent-harness-study/sources/letta/letta/schemas/letta_message.py:31-35`, `studies/agent-harness-study/sources/letta/letta/orm/message.py:75-83`). Retrieved passages carry source/file provenance columns (`studies/agent-harness-study/sources/letta/letta/schemas/passage.py:21-32`), and memory edits are versioned via snapshot history and a git-style commit model. A full REST traversal surface (run → steps → messages → trace) makes causal chains externally auditable.

The main weaknesses are erosion vectors: all lineage FKs are nullable with `ON DELETE SET NULL`, run/step stamping is not enforced at the schema level, the legacy `prompts` table is orphaned, tool calls resolve tools by *name* rather than immutable tool entity ID, provider-trace capture is opt-in and its retrieval silently swallows errors, and a few legacy summarizer paths checkpoint messages with `run_id=None, step_id=None`.

## Rating

**8 / 10** — Clear, explicit lineage model backed by database foreign keys, indexes, dedicated query endpoints, telemetry correlation IDs, and integration tests that assert the links (`tests/test_provider_trace.py:240-273`, `tests/integration_test_human_in_the_loop.py:430-474`). It falls short of 9-10 because lineage integrity is not enforced (nullable FKs, `SET NULL` on deletion, commented-out validation), provenance capture is partially opt-in/config-dependent, and some legacy paths bypass lineage stamping entirely.

## Evidence Collected

Every entry includes a file path with line numbers relative to `studies/agent-harness-study/sources/letta/`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Message→step/run/conversation linkage | `Message` ORM has `step_id` FK (`steps.id`, SET NULL), `run_id` FK (`runs.id`, SET NULL), `conversation_id` FK | `letta/orm/message.py:48-71` |
| Ordered causal sequence | Monotonic `sequence_id` column plus composite indexes `(run_id, sequence_id)` and `(agent_id, conversation_id, sequence_id)` | `letta/orm/message.py:30-36, 86-91` |
| Single persistence choke point | `_checkpoint_messages` stamps `message.step_id/run_id/conversation_id` on every new message before persisting ("ONLY place where messages are persisted") | `letta/agents/letta_agent_v3.py:758-786` |
| Step metadata = model version record | Step stores `provider_id`, `provider_name`, `provider_category`, `model`, `model_handle`, `model_endpoint`, `context_window_limit` | `letta/orm/step.py:34-52`; `letta/services/step_manager.py:202-225` |
| OTEL correlation on steps | Step persists `trace_id` (from current span), transaction id `tid`, and cloud `request_id` | `letta/orm/step.py:73-77`; `letta/services/step_manager.py:157-158`; `letta/otel/tracing.py:499-502` |
| Full request/response provenance | `ProviderTrace` schema stores raw `request_json`/`response_json` keyed by `step_id`, `run_id`, `agent_id`, `call_type`, plus `llm_config` | `letta/schemas/provider_trace.py:46-66` |
| ClickHouse trace analytics | `LLMTrace` carries org/project/agent/run/step/OTEL trace ids, provider+model, and `llm_config_json` for cost analytics | `letta/schemas/llm_trace.py:57-96` |
| Tool call ↔ result linking | Assistant messages store `tool_calls` (OpenAI `id`+function name+arguments); tool messages carry `tool_call_id` and per-call `tool_returns` | `letta/schemas/message.py:287-295, 2668-2675`; `letta/orm/message.py:46-57` |
| Result→call join logic | `to_letta_messages_from_list` builds `assistant_messages_by_tool_call` map keyed on `tool_call.id` to pair results with requests | `letta/schemas/message.py:355-373` |
| Parallel tool message construction | `create_parallel_tool_messages_from_llm_response` receives `tool_call_specs=[{name,args,id}]` and stamps each produced message with `step_id`/`run_id` | `letta/agents/letta_agent_v3.py:1916-1939`; `letta/server/rest_api/utils.py:382-429` |
| Serialization requires linkage | OpenAI serialization raises `TypeError("OpenAI API requires tool_call_id to be set.")` when a tool return lacks its id | `letta/schemas/message.py:1460-1468, 1513-1526` |
| Duplicate-return guard | Dedupe of duplicate tool returns across messages by `tool_call_id` ("never see the same tool_call_id's result twice in a single request") | `letta/schemas/message.py:2506, 2536-2581` |
| Retrieval provenance fields | Passages carry `archive_id`, `source_id` (deprecated → `folder_id`), `file_id`, `file_name`, `metadata`, `tags` | `letta/schemas/passage.py:21-32` |
| Archival search returns provenance | `search_agent_archival_memory_async` returns `{id, timestamp, content, tags}` per passage | `letta/services/agent_manager.py:2651, 2668-2670` |
| File search provenance | Semantic file search formats results with file name headers, passage id, and score (`{"text", "score", "passage_id"}`) | `letta/services/tool_executor/files_tool_executor.py:643, 660-673, 806-824` |
| Memory edit lineage | `BlockHistory` snapshots block value with `actor_type`/`actor_id` and monotonic `sequence_number`; git-based `MemoryCommit` adds `sha`/`parent_sha`/author | `letta/orm/block_history.py:12-48`; `letta/schemas/memory_repo.py:19-36` |
| Approval→action link | `ApprovalReturn` **requires** `tool_call_id` of the approved/denied call plus `approve` flag | `letta/schemas/letta_message.py:31-35` |
| Approval request persistence | Approval requests are `role=approval` messages embedding the exact `tool_calls` (id/name/arguments) from the LLM response | `letta/server/rest_api/utils.py:304-371` |
| Approval response persistence | Response messages persist `approval_request_id`, `approve`, `denial_reason`, and itemized `approvals` list | `letta/server/rest_api/utils.py:213-227`; `letta/orm/message.py:75-83` |
| Cancel-time denial audit trail | `cancel_run` fabricates denials for ALL pending tool calls keyed by `tool_call.id` and checkpoints them onto the cancelled run | `letta/services/run_manager.py:669-748` |
| Approval gating metadata | `RequiresApprovalToolRule` marks which registered tools trigger the approval flow | `letta/schemas/tool_rule.py:348` |
| Run artifact model | `Run` binds agent, conversation, status, stop_reason, callback outcome, TTFT/duration metrics | `letta/orm/run.py:22-77`; `letta/schemas/run.py:17-51` |
| Job↔message association table | `job_messages` join table with unique constraint `(job_id, message_id)` | `letta/orm/job_messages.py:13-33` |
| Batch artifacts linked to items/messages | `LLMBatchItem` FKs to batch job + agent, stores per-item `llm_config` and raw provider result; `Message.batch_item_id` closes the loop | `letta/orm/llm_batch_items.py:34-54`; `letta/orm/message.py:62-65` |
| REST lineage traversal | `GET /v1/steps/{id}`, `/messages`, `/trace`; `GET /v1/runs/{id}/messages`, `/steps`, `/usage`, `/metrics`, `/trace` (OTEL spans) | `letta/server/rest_api/routers/v1/steps.py:71-158`; `letta/server/rest_api/routers/v1/runs.py:162-289` |
| Streaming chunk→message derivation | OTIDs deterministically derive from the persisted message UUID (last 7 bits = chunk index): `generate_otid_from_id` | `letta/schemas/message.py:2648-2665` |
| Tests asserting step↔trace↔run chain | `test_provider_trace_contains_step_id`, `test_provider_trace_contains_run_id_for_async_job`, multi-step trace tests | `tests/test_provider_trace.py:240-273, 276-290+` |
| Test asserting approval linkage round trip | `assert messages[0].approval_request_id == tool_call_id` after approve flow | `tests/integration_test_human_in_the_loop.py:430-474` |

## Answers to Dimension Questions

### 1. Can every output be traced to its inputs?

**Largely yes, with caveats.** The canonical path is complete: user input messages → persisted with `run_id` → step created with generated id (`generate_step_id`, `studies/agent-harness-study/sources/letta/letta/agents/helpers.py:373`) → LLM response messages and tool messages stamped with the same `step_id`/`run_id` at the single persistence choke point (`studies/agent-harness-study/sources/letta/letta/agents/letta_agent_v3.py:773-786`) → provider trace stores the entire request payload (which contains those inputs verbatim) under the same `step_id`. Given any final assistant message you can walk `message.step_id → step.model/provider/trace_id → provider_trace.request_json` and recover exactly what the model saw. Caveats: (a) the schema does not enforce this — a validator asserting `run_id` presence exists only as commented-out code (`studies/agent-harness-study/sources/letta/letta/schemas/message.py:312-317`); (b) manual compaction routes checkpoint summary messages with `run_id=None, step_id=None` (`studies/agent-harness-study/sources/letta/letta/server/rest_api/routers/v1/conversations.py:1128`, `studies/agent-harness-study/sources/letta/letta/server/rest_api/routers/v1/agents.py:2503`); (c) in-loop approval handling tolerates historical approval messages missing `step_id` by synthesizing one (`studies/agent-harness-study/sources/letta/letta/agents/letta_agent_v3.py:1019-1026`).

### 2. Is provenance preserved through transformations?

**Mostly yes.** Transformations preserve linkage explicitly: streaming chunks derive deterministic OTIDs from the eventual persisted message UUID so client-side partials can be reassembled to the authoritative row (`studies/agent-harness-study/sources/letta/letta/schemas/message.py:2648-2665`; applied throughout `studies/agent-harness-study/sources/letta/letta/interfaces/openai_streaming_interface.py:355-1493`). Tool execution results flow through `ToolExecutionResult` into packaged function responses that keep their `tool_call_id` through packaging, truncation, deduplication, and re-serialization to each provider format (Anthropic `tool_use_id`, Google AI, OpenAI Responses `call_id` — `studies/agent-harness-study/sources/letta/letta/schemas/message.py:2031-2106, 2353-2388, 1642-1660`). Legacy single-tool messages get `tool_returns[0].tool_call_id` backfilled from the message-level field (`studies/agent-harness-study/sources/letta/letta/schemas/message.py:860-861`). Weak spot: tool *returns* can be truncated for context budgeting (`return_char_limit`, `studies/agent-harness-study/sources/letta/letta/agents/letta_v3` path at `letta/agents/letta_agent_v3.py:1878-1884`), and truncation replaces content with a marker (`truncate_tool_return`, `studies/agent-harness-study/sources/letta/letta/schemas/message.py:68-73`) without keeping a pointer to untruncated content beyond what the provider trace preserves. Summarization transforms history into a summary message whose lineage to consumed messages is implicit (the summary message references no source-message ids), though compaction stats are embedded in the payload (`extract_compaction_stats_from_packed_json`, `studies/agent-harness-study/sources/letta/letta/schemas/message.py:48`).

### 3. Are model versions tracked in lineage?

**Yes, redundantly across three layers.** Each `Step` row persists `model`, `model_handle`, `model_endpoint`, `provider_name`, `provider_category`, `provider_id`, and `context_window_limit` (`studies/agent-harness-study/sources/letta/letta/orm/step.py:34-52`). Messages additionally record the `model` string used (`studies/agent-harness-study/sources/letta/letta/orm/message.py:44`). Provider traces embed the full `llm_config` dict, and the ClickHouse `LLMTrace` stores `model`, `provider`, and `llm_config_json` for analytics (`studies/agent-harness-study/sources/letta/letta/schemas/llm_trace.py:66-96`). Notably, "auto mode" routing updates the persisted step with the *actually resolved* model even if resolution fails mid-flight, specifically so billing/audit sees the true model (`studies/agent-harness-study/sources/letta/letta/agents/letta_agent_v3.py:1076-1087`). What is tracked is the model identifier/handle, not a hash of weights or a provider-side snapshot version — standard for an orchestrator, but worth noting.

### 4. Can causal chains be audited?

**Yes, via both SQL relations and public API.** Foreign keys and purpose-built indexes (`ix_messages_run_sequence`, `idx_messages_step_id`, `ix_steps_run_id` — `studies/agent-harness-study/sources/letta/letta/orm/message.py:34-36`, `studies/agent-harness-study/sources/letta/letta/orm/step.py:25`) support reverse queries, and the REST surface exposes the whole chain: run → steps (`studies/agent-harness-study/sources/letta/letta/server/rest_api/routers/v1/runs.py:234-264`), run → messages (`runs.py:162-185`), run → OTEL spans (`runs.py:267-289`), step → messages (`studies/agent-harness-study/sources/letta/letta/server/rest_api/routers/v1/steps.py:134-158`), step → provider trace (`steps.py:97-112`), plus per-run/per-step metrics and usage. Steps also carry `trace_id`/`request_id`/`tid` for cross-system correlation into OTEL and cloud request logs (`studies/agent-harness-study/sources/letta/letta/orm/step.py:73-77`). Answering "which tool call provided fact X in answer Y" is therefore mechanical: find the assistant message in the run, read its `tool_calls[].id`, then locate the tool-role message whose `tool_call_id` matches. Audit gaps: trace retrieval silently returns nothing on error (`except Exception: pass`, `studies/agent-harness-study/sources/letta/letta/server/rest_api/routers/v1/steps.py:103-110`), and deleting runs/steps nulls out message lineage pointers due to `ondelete="SET NULL"` (`studies/agent-harness-study/sources/letta/letta/orm/message.py:48-53`).

## Architectural Decisions

1. **Relational lineage over event logs.** Lineage lives in normalized tables with real FKs (`messages.step_id/run_id`, `steps.run_id`, `job_messages` join table) instead of append-only event envelopes, giving referential structure and indexed bidirectional traversal (`studies/agent-harness-study/sources/letta/letta/orm/message.py:27-37`).
2. **Single-writer checkpoint discipline.** All agent-loop persistence funnels through `_checkpoint_messages`, which force-stamps `run_id`/`step_id`/`conversation_id` before insert — a deliberate invariant that lineage cannot be forgotten by individual code paths (`studies/agent-harness-study/sources/letta/letta/agents/letta_agent_v3.py:758-786`).
3. **Adopt OpenAI wire identifiers as the join keys.** Tool-call linkage, approvals, and denial bookkeeping all reuse the provider-native `tool_call_id` rather than inventing a parallel Letta-scoped id, minimizing translation loss across providers (`studies/agent-harness-study/sources/letta/letta/schemas/message.py:287-295`; `studies/agent-harness-study/sources/letta/letta/schemas/letta_message.py:31-35`).
4. **Deterministic ID derivation for streaming.** OTIDs encode chunk index in the low bits of the message UUID, making streamed partials addressable to their persisted row without extra state (`studies/agent-harness-study/sources/letta/letta/schemas/message.py:2648-2665`).
5. **Layered observability with denormalized hot path.** Postgres holds structured lineage; provider traces fan out to pluggable backends (postgres/clickhouse/socket — `studies/agent-harness-study/sources/letta/letta/services/provider_trace_backends/factory.py:8-45`); ClickHouse gets a wide, denormalized `LLMTrace` optimized for cost queries (`studies/agent-harness-study/sources/letta/letta/schemas/llm_trace.py:14-49`).
6. **Cancellation preserves the audit trail.** Rather than dropping pending work, cancellation writes explicit denial records referencing every affected `tool_call.id` (`studies/agent-harness-study/sources/letta/letta/services/run_manager.py:694-748`).

## Notable Patterns

- **Prefix-typed IDs as soft lineage markers**: `step-`, `run-`, `batch_item-`, `memcommit-` prefixes make entity roles self-describing in traces and logs (`studies/agent-harness-study/sources/letta/letta/orm/step.py:27`, `studies/agent-harness-study/sources/letta/letta/orm/run.py:37`, `studies/agent-harness-study/sources/letta/letta/orm/llm_batch_items.py:34`), and validators enforce prefix correctness on lineage params (`studies/agent-harness-study/sources/letta/letta/services/step_manager.py:91-94`).
- **Defensive backfilling**: legacy rows are healed on read — e.g., tool-message `tool_call_id` derived from its first tool return during pydantic conversion (`studies/agent-harness-study/sources/letta/letta/orm/message.py:112-122`), and approval responses tolerate `approvals=None` with a warning log (`studies/agent-harness-study/sources/letta/letta/server/rest_api/utils.py:200-206`).
- **Provenance-rich retrieval output**: search tools deliberately format passage/file provenance (ids, timestamps, scores, filenames) into the text the model itself consumes, so provenance survives into the model's own reasoning context (`studies/agent-harness-study/sources/letta/letta/services/tool_executor/files_tool_executor.py:653-673`; `studies/agent-harness-study/sources/letta/letta/services/agent_manager.py:2651`).
- **Git semantics for memory lineage**: block history with actor attribution and sequence numbers, plus commit objects with parent SHAs, give memory edits VCS-grade ancestry (`studies/agent-harness-study/sources/letta/letta/orm/block_history.py:27-48`; `studies/agent-harness-study/sources/letta/letta/schemas/memory_repo.py:24-36`).
- **Test-enforced lineage contracts**: integration tests pin the exact linkage invariants (trace↔step↔run equality; approval response ↔ tool call id) rather than just feature presence (`studies/agent-harness-study/sources/letta/tests/test_provider_trace.py:240-273`; `studies/agent-harness-study/sources/letta/tests/integration_test_human_in_the_loop.py:469-472`).

## Tradeoffs

- **Structured FK lineage vs. write amplification**: stamping and validating run existence on every message insert (`studies/agent-harness-study/sources/letta/letta/services/message_manager.py:548-555`) buys integrity at the cost of extra checks per batch.
- **Full payload retention vs. storage growth**: storing complete `request_json`/`response_json` per step enables byte-exact replay/audit but is heavy; Letta mitigates with backend selection and metadata-only variants (`ProviderTraceMetadata`, `studies/agent-harness-study/sources/letta/letta/schemas/provider_trace.py:69-86`) and configurable `track_provider_trace` (`studies/agent-harness-study/sources/letta/letta/server/rest_api/routers/v1/steps.py:104`).
- **Provider-native join keys vs. provider quirks**: reusing `tool_call_id` means inheriting provider-specific mangling (length clamps, sanitization) which slightly erodes global uniqueness guarantees (`studies/agent-harness-study/sources/letta/letta/schemas/message.py:1460-1477`; sanitize helper imported at `studies/agent-harness-study/sources/letta/letta/schemas/message.py:65`).
- **Soft-deletion lineage (`SET NULL`) vs. hard referential closure**: deleting a run/step keeps messages but orphans them from their causal context — friendlier operationally, weaker forensically (`studies/agent-harness-study/sources/letta/letta/orm/message.py:48-53`).
- **OTID bit-packing vs. bounded fan-out**: deriving chunk indices from UUID low bits avoids state but caps at 128 chunks per message, raising `ValueError` beyond that (`studies/agent-harness-study/sources/letta/letta/schemas/message.py:2655-2662`).

## Failure Modes / Edge Cases

- **Unstamped legacy/manual paths**: direct compaction endpoints persist summary messages with no run/step (`studies/agent-harness-study/sources/letta/letta/server/rest_api/routers/v1/conversations.py:1128`) — such messages are invisible to step-based audits.
- **Silent trace-retrieval failure**: `GET /v1/steps/{id}/trace` swallows all exceptions and returns `null`, which can mask backend misconfiguration as "no trace" (`studies/agent-harness-study/sources/letta/letta/server/rest_api/routers/v1/steps.py:103-110`).
- **Nonexistent run degradation**: `log_step_async` downgrades a bad `run_id` to `NULL` with a warning instead of failing, quietly producing steps detached from their run (`studies/agent-harness-study/sources/letta/letta/services/step_manager.py:239-243`).
- **Historical approval messages without steps**: old approval requests lacking `step_id` trigger synthetic step generation mid-loop (`studies/agent-harness-study/sources/letta/letta/agents/letta_agent_v3.py:1019-1026`), creating steps that did not correspond to an actual LLM call.
- **Malformed approval payloads**: corrupted approvals are detected late (empty extraction after parsing) and abort the turn with `invalid_tool_call` stop reason (`studies/agent-harness-study/sources/letta/letta/agents/letta_agent_v3.py:1010-1017`).
- **Name-based tool resolution collisions**: two attached tools with the same name resolve to whichever appears first in `agent_state.tools` (`studies/agent-harness-study/sources/letta/letta/agents/letta_agent_v3.py:1829,1848`), and lineage records only the name.
- **Deprecated dual identity for approvals**: `approval_request_id` is marked deprecated on some schemas in favor of embedding approvals with `tool_call_id` (`studies/agent-harness-study/sources/letta/letta/schemas/message.py:184-192`), so older clients may populate either field.

## Future Considerations

- Make `step_id`/`run_id` mandatory at the schema layer (revive the commented validator at `studies/agent-harness-study/sources/letta/letta/schemas/message.py:312-317`) or provide an explicit "unattributed" role for system-generated messages like manual summaries.
- Replace `ON DELETE SET NULL` with tombstoned lineage (soft-delete runs/steps) so post-deletion audits remain possible.
- Record the resolved tool entity `tool.id` (not just function name) on tool call/result messages to close the definition-lineage loop; MCP already maintains a server↔tool mapping that could generalize (`studies/agent-harness-study/sources/letta/letta/services/mcp_server_manager.py:66`).
- Surface trace-backend failures instead of swallowing them, and add health indicators for `track_provider_trace` being disabled.
- Either remove or repurpose the unused `prompts` table (`studies/agent-harness-study/sources/letta/letta/orm/prompt.py:8-13`); today prompt-level provenance exists only inside opaque provider-trace JSON blobs.
- Add lineage links from summary/compaction messages to the span of source message ids they consumed.

## Questions / Gaps

- **Prompt-record lineage**: No evidence found of a per-step prompt entity. The `prompts` table exists but a repo-wide search found zero usages outside its definition and the `orm/__init__` export (`studies/agent-harness-study/sources/letta/letta/orm/prompt.py:8-13`; import at `studies/agent-harness-study/sources/letta/letta/orm/__init__.py:30`). Input-prompt provenance is only recoverable via provider-trace payloads, which are opt-in.
- **Model *version* (vs. model *name*) tracking**: Steps/traces record model handles and endpoints; no evidence found of weight-version/snapshot hashes (e.g., OpenAI model snapshot ids) being captured. Searched step schema, provider trace schema, and llm_trace schema.
- **Cross-conversation message reuse**: Whether a message row can legitimately belong to multiple conversations was not determined; `conversation_id` is single-valued with a separate `conversation_messages` join table used in conversation mode (`studies/agent-harness-study/sources/letta/letta/agents/letta_agent_v3.py:788-798`), suggesting the join table is authoritative there while the column is a convenience — the migration boundary between these two representations was not fully traced.
- **Retention policy for provider traces**: Evidence shows ClickHouse/Postgres backends and retry logic (`studies/agent-harness-study/sources/letta/letta/services/llm_trace_writer.py:138`), but no TTL/pruning policy for trace payloads was located within the studied directory.

---

Generated by `10.03-causal-links-and-lineage` against `letta`.
