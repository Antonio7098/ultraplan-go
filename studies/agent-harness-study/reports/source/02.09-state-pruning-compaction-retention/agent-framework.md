# Source Analysis: agent-framework

## State Pruning, Compaction, and Retention

### Source Info

| Field | Value |
|-------|-------|
| Name | agent-framework |
| Path | `studies/agent-harness-study/sources/agent-framework` |
| Language / Stack | Python (core, harness, providers), with mirrored .NET surface; Pydantic-style `Message` types; Chat/MCP/Workflow providers |
| Analyzed | 2026-07-11 |

## Summary

The framework treats message-history growth as a first-class problem. The core solution lives in `python/packages/core/agent_framework/_compaction.py:1-1456`, which defines a `CompactionStrategy` `Protocol` (`_compaction.py:51-63`), six concrete strategies (`TruncationStrategy`, `SlidingWindowStrategy`, `SelectiveToolCallCompactionStrategy`, `ToolResultCompactionStrategy`, `SummarizationStrategy`, `TokenBudgetComposedStrategy`), a token-aware `ContextWindowCompactionStrategy` (`_compaction.py:1307-1419`) that composes tool-eviction at 50% and truncation at 80% of the model's input budget, and a `CompactionProvider` (`_compaction.py:1182-1305`) that wires the strategies into the `ContextProvider.before_run`/`after_run` lifecycle. Compaction is implemented as in-place **annotation** rather than deletion — messages are kept but flagged via `_excluded` and `_exclude_reason` keys (`_compaction.py:31-32, 582-597`), with back-pointers from summary messages to originals (`_compaction.py:33-35, 894-913`) so replay/debugging is preserved. Retention is enforced (a) in-memory via the `_excluded` flag that downstream `included_messages` filters (`_compaction.py:569-579`), (b) on disk via `RedisHistoryProvider.max_messages` LTRIM (`python/packages/redis/agent_framework_redis/_history_provider.py:40-91, 162-165`), and (c) per-session via `clear()` methods on `RedisHistoryProvider` (`_history_provider.py:181-187`) and `CosmosHistoryProvider` (`python/packages/azure-cosmos/agent_framework_azure_cosmos/_history_provider.py:203-226`). The framework **explicitly notes** that service-managed histories own their own compaction (`docs/decisions/0019-python-context-compaction-strategy.md:72-83`). Workflow checkpoints have a `delete(checkpoint_id)` API across all three storage backends (`_workflows/_checkpoint.py:158, 217, 392; azure-cosmos/_checkpoint_storage.py:304-336`) but **no automatic retention** — the runner's `_run_cleanup` only clears the runtime in-memory checkpoint storage reference (`_workflows/_workflow.py:832-836`).

The model is mature: there is a Protocol, an extensible chain model (`TokenBudgetComposedStrategy`), atomic-group guarantees that prevent breaking tool-call/result pairings (`_compaction.py:120-243, 567`), bidirectional trace metadata (`_compaction.py:900-913, 1067-1086`), and 60+ dedicated tests (`python/packages/core/tests/core/test_compaction.py:107-1207`). It is also honest about its boundaries: only local storage is compacted; service-side threads are the model's responsibility.

## Rating

**8 / 10 — Clear model with explicit interfaces, atomic-group guarantees, and tests, but uneven retention across backends and limited privacy/anonymization primitives.**

Rationale:
- (+) First-class compaction module with a `Protocol`, six named strategies, a budget-aware composer, and a `ContextWindowCompactionStrategy` that derives thresholds from `max_context_window_tokens - max_output_tokens` (`_compaction.py:1383-1385`).
- (+) Bidirectional back-pointers (`SUMMARY_OF_MESSAGE_IDS_KEY`, `SUMMARIZED_BY_SUMMARY_ID_KEY`) keep full replay history even after compaction (`_compaction.py:33-35, 894-913, 1067-1086`).
- (+) Extensive test coverage with threshold-driven assertions and idempotency checks (`test_compaction.py:1057-1207`).
- (+) `ContextProvider`-integrated `CompactionProvider.before_run` and `after_run` (`_compaction.py:1249-1305`) make compaction a first-class hook, not an ad-hoc sweep.
- (−) Retention is **opt-in per provider** — `InMemoryHistoryProvider` (`_sessions.py:814-893`) and `FileHistoryProvider` (`_sessions.py:894-1134`) have **no `max_messages`, no TTL, no eviction**; only `RedisHistoryProvider` trims.
- (−) No general-purpose "delete user data" surface; `clear()` exists only on `RedisHistoryProvider` and `CosmosHistoryProvider`. `InMemoryHistoryProvider`/`FileHistoryProvider` have no such method.
- (−) Workflow checkpoints grow unbounded — no `max_checkpoints` or TTL on `CheckpointStorage` (`_workflows/_checkpoint.py:119-189`).
- (−) No telemetry/metrics on compaction frequency, summary length, or excluded-message counts.
- (−) `CharacterEstimatorTokenizer` (4 chars/token, `_compaction.py:66-70`) is the default; the only other built-in tokenizer is user-supplied via `TokenizerProtocol`. No token-accurate budget for production models out of the box.

## Evidence Collected

Every entry includes file path and line number. Format: `path/to/file.py:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Compaction protocol surface | `CompactionStrategy` is a `runtime_checkable` `Protocol` returning `bool` after in-place mutation. | `_compaction.py:41-63` |
| Tokenizer protocol | `TokenizerProtocol` + `CharacterEstimatorTokenizer` (4 chars/token heuristic). | `_compaction.py:42-70` |
| Group annotation | `group_messages` builds spans for `system` / `user` / `assistant_text` / `tool_call`, keeping tool call + tool result + reasoning prefix atomic. | `_compaction.py:120-243` |
| Group annotation metadata keys | `GROUP_ANNOTATION_KEY` plus per-span `id/kind/index/has_reasoning/token_count`. | `_compaction.py:24-30` |
| Excluded annotations | `EXCLUDED_KEY`, `EXCLUDE_REASON_KEY`, `set_excluded`, `exclude_group_ids`. | `_compaction.py:31-32, 569-597` |
| Summary forward/back links | `SUMMARY_OF_MESSAGE_IDS_KEY`, `SUMMARY_OF_GROUP_IDS_KEY`, `SUMMARIZED_BY_SUMMARY_ID_KEY` and `_set_group_summarized_by_summary_id`. | `_compaction.py:33-35, 288-293, 894-913, 1067-1086` |
| Incremental reannotation | `annotate_message_groups` re-annotates only the suffix; `_reannotation_start` preserves prefixes. | `_compaction.py:392-490` |
| Token counting (incremental) | `annotate_token_counts` extends group annotations and only counts untokenized messages. | `_compaction.py:512-537` |
| `TruncationStrategy` (token or message threshold) | Trims oldest non-system groups; supports `preserve_system=True`. | `_compaction.py:650-718` |
| `SlidingWindowStrategy` | Keeps last `keep_last_groups` non-system groups. | `_compaction.py:721-764` |
| `SelectiveToolCallCompactionStrategy` | Keeps last `keep_last_tool_call_groups` tool-call groups. | `_compaction.py:767-817` |
| `ToolResultCompactionStrategy` | Replaces old tool-call groups with `[Tool results: ...]` summary messages; preserves forward + backward links. | `_compaction.py:820-918` |
| `SummarizationStrategy` | Calls a chat client to summarize oldest groups when included non-system message count exceeds `target_count + threshold`; default prompt mandates no critique/correction/omission. | `_compaction.py:921-1088` |
| Default summarization prompt | Five-sentence, neutral, includes prior summary. | `_compaction.py:931-946` |
| `TokenBudgetComposedStrategy` | Runs a strategy chain, refreshes token counts after each step, falls back to oldest-group exclusion if budget is not met, with a strict fallback that drops system anchors last. | `_compaction.py:1091-1160` |
| `apply_compaction` (top-level API) | Annotates, runs strategy, projects included messages. | `_compaction.py:1163-1176` |
| `CompactionProvider.before_run` | Annotates messages already in context from earlier providers, projects included ones back into `context.context_messages`. | `_compaction.py:1249-1273` |
| `CompactionProvider.after_run` | Reads stored history from `session.state[history_source_id]`, annotates + applies `after_strategy`. **Keeps all messages in storage, only annotates exclusions** — relies on the history provider's `skip_excluded` flag on the next load. | `_compaction.py:1275-1305` |
| `ContextWindowCompactionStrategy` | Two-phase pipeline: tool-eviction at 50% (`tool_eviction_threshold`) collapses tool groups; truncation at 80% (`truncation_threshold`) drops oldest non-system groups; thresholds must satisfy `0 < tool ≤ truncation ≤ 1.0`. | `_compaction.py:1307-1419` |
| Compaction annotations exported | `EXCLUDED_KEY`, `EXCLUDE_REASON_KEY`, `COMPACTION_STATE_KEY`, all group keys, all strategy names. | `_compaction.py:1422-1456` |
| Group-atomic guarantee tests | `test_group_annotations_keep_tool_call_and_tool_result_atomic`, `test_group_annotations_include_reasoning_in_tool_call_group`. | `tests/core/test_compaction.py:107-135` |
| Incremental annotation tests | `test_extend_compaction_messages_preserves_existing_annotations_and_tokens`, `test_incremental_annotation_assigns_unique_message_ids`. | `tests/core/test_compaction.py:172-254` |
| Truncation strategy tests | `test_truncation_strategy_keeps_system_anchor`, `test_truncation_strategy_compacts_when_token_limit_exceeded`, validation of negative/non-monotonic values. | `tests/core/test_compaction.py:257-305` |
| Tool-call strategy tests | Zero/keep and negative-rejection coverage. | `tests/core/test_compaction.py:308-348` |
| Summarization tests | Failure/empty-summary graceful skip; bidirectional link assertions. | `tests/core/test_compaction.py:393-466` |
| Token-budget composer tests | Budget satisfaction and deterministic fallback coverage. | `tests/core/test_compaction.py:468-503` |
| Tool-result compaction tests | Idempotency, multi-group combinations, preserved tool-result content. | `tests/core/test_compaction.py:521-848` |
| CompactionProvider integration tests | `before_run`/`after_run` + `skip_excluded` flag; both-strategies; no-history noop. | `tests/core/test_compaction.py:850-1014` |
| Skip-excluded provider tests | `test_in_memory_history_provider_skip_excluded` and default-loads-all sibling test. | `tests/core/test_compaction.py:1016-1051` |
| `ContextWindowCompactionStrategy` tests | Under-threshold noop; tool-eviction at 50%; truncation at 80%; `keep_last_tool_call_groups` respected; threshold validation. | `tests/core/test_compaction.py:1057-1207` |
| Harness integration: default strategy wiring | `_assemble_compaction_provider` builds `ContextWindowCompactionStrategy` for `before` and `ToolResultCompactionStrategy(keep_last_tool_call_groups=2)` for `after` when token params are present. | `_harness/_agent.py:72-120, 340-355` |
| Harness history-provider default | `InMemoryHistoryProvider` is used when no custom history provider is passed. | `_harness/_agent.py:348, 431` |
| `InMemoryHistoryProvider` (no retention) | Stores messages in `session.state["messages"]`; exposes only `skip_excluded: bool = False`. **No `max_messages`, no `clear()`, no TTL.** | `_sessions.py:814-893` |
| `FileHistoryProvider` (no retention) | JSONL per session; offers `skip_excluded` only. No `clear`, no rotation, no max-line cap. | `_sessions.py:894-1134` |
| `RedisHistoryProvider.max_messages` trimming | After `rpush`, calls `ltrim(key, -max_messages, -1)` to keep only the last N messages. | `python/packages/redis/agent_framework_redis/_history_provider.py:40-91, 151-165` |
| `RedisHistoryProvider.clear` | Calls `_redis_client.delete(_redis_key(session_id))` to wipe a session. | `python/packages/redis/agent_framework_redis/_history_provider.py:181-187` |
| Redis max_messages tests | `test_max_messages_trimming` calls `ltrim(key, -10, -1)`; `test_no_trim_when_under_limit` confirms `ltrim` is not called when under cap. | `python/packages/redis/tests/test_providers.py:466-486` |
| Redis `clear` test | `test_clear_calls_delete` mocks `client.delete` and asserts it is called with the per-session key. | `python/packages/redis/tests/test_providers.py:489-496` |
| `CosmosHistoryProvider.clear` (batched) | Queries items by `(session_id, source_id)`, batches deletes in groups of 100 (`_BATCH_OPERATION_LIMIT`). | `python/packages/azure-cosmos/agent_framework_azure_cosmos/_history_provider.py:203-226, 40` |
| `CosmosHistoryProvider.list_sessions` | Cross-partition query to list distinct session IDs (useful for retention sweeps). | `python/packages/azure-cosmos/agent_framework_azure_cosmos/_history_provider.py:228-240` |
| `CosmosHistoryProvider.clear` test | `test_clear_deletes_all_session_items` verifies two-item batched delete. | `python/packages/azure-cosmos/tests/test_cosmos_history_provider.py:252-272` |
| `FileMemoryProvider` index cap | `_MAX_INDEX_ENTRIES = 50` caps the `memories.md` index; older files are still on disk but not surfaced. | `_harness/_file_memory.py:69, 217-235` |
| `FileMemoryProvider` per-file delete | `file_memory_delete_file` tool removes file + description sidecar and rebuilds the index. | `_harness/_file_memory.py:304-326` |
| `MemoryContextProvider.history_message_filter` | Optional callback that can **rewrite or drop** messages before transcript save — the only "redact-before-persist" hook in core. | `_harness/_memory.py:69, 954-1015, 1147-1159` |
| `MemoryContextProvider` consolidation cadence | `consolidation_interval = 24h` (`DEFAULT_MEMORY_CONSOLIDATION_INTERVAL`), `consolidation_min_sessions = 5`, `max_extractions = 5`. Consolidation LLM-compresses topic files. | `_harness/_memory.py:35-40, 945-1014, 1484-1552` |
| `MemoryContextProvider` index cap | `index_line_limit` (default 200), `index_line_length` (default 150) caps the memory index. | `_harness/_memory.py:35-38, 945-1014` |
| `MemoryContextProvider.recent_turns` window | `_select_recent_turn_messages` returns the last `turn_count` user-turn groups (optionally including tool-call turns). | `_harness/_memory.py:184-208, 1186-1190` |
| Workflow checkpoint protocol | `CheckpointStorage` Protocol: `save / load / list_checkpoints / delete / get_latest / list_checkpoint_ids`. | `_workflows/_checkpoint.py:119-189` |
| Workflow checkpoint in-memory storage | `InMemoryCheckpointStorage` implements delete + list + get_latest. | `_workflows/_checkpoint.py:192-231` |
| Workflow checkpoint file storage | `FileCheckpointStorage.delete` removes the JSON file; no `max_checkpoints`, no TTL. | `_workflows/_checkpoint.py:340-410` |
| Cosmos checkpoint storage delete | `CosmosCheckpointStorage.delete` queries by `checkpoint_id` then issues `container.delete_item`. | `python/packages/azure-cosmos/agent_framework_azure_cosmos/_checkpoint_storage.py:304-336` |
| Workflow run cleanup | `_run_cleanup` only clears the runtime in-memory checkpoint storage reference; **does not** delete persisted checkpoints. | `_workflows/_workflow.py:763-836` |
| Decisions / docs | ADR-0019 (`docs/decisions/0019-python-context-compaction-strategy.md`) lays out boundary vs in-run compaction, composability requirements, atomic-group correctness constraint, "tool calls and their results must be kept together" rule. | `docs/decisions/0019-python-context-compaction-strategy.md:9-895` |
| Service-side disclaimer | "All compaction discussed in this ADR is irrelevant when using only service-managed storage. The service is responsible for its own context window management and compaction." | `docs/decisions/0019-python-context-compaction-strategy.md:72-83` |
| Sample folder | `python/samples/02-agents/compaction/` has 7 samples including `basics.py`, `summarization.py`, `advanced.py`, `tiktoken_tokenizer.py`, `compaction_provider.py`. | `python/samples/02-agents/compaction/README.md:1-25` |

## Answers to Dimension Questions

### 1. What grows forever?

- **Conversation history in `InMemoryHistoryProvider`** (`_sessions.py:814-893`): unbounded `list[Message]` in `session.state["messages"]`. There is **no** `max_messages`, no eviction, no TTL.
- **Conversation history in `FileHistoryProvider`** (`_sessions.py:894-1134`): each `save_messages` appends one JSON line per message (`_sessions.py:1048-1071`); no rotation, no cap.
- **Workflow checkpoints** (`_workflows/_checkpoint.py:119-450`, `azure-cosmos/_checkpoint_storage.py:35-336`): the `CheckpointStorage` Protocol has no `max_checkpoints` or TTL; `get_latest` is a query, not a cap.
- **Per-session memory transcripts** (`_harness/_memory.py:1119-1159`): transcripts grow per session; only the *index* is capped (200 lines × 150 chars), not the topics themselves.
- **`FileMemoryProvider` index** (`_harness/_file_memory.py:69`): capped at 50 entries; the underlying files persist.

### 2. What gets summarized?

- Older **non-system message groups** when the `SummarizationStrategy` trigger fires (`target_count + threshold` exceeded, `_compaction.py:1016`).
- Older **tool-call groups** when `ToolResultCompactionStrategy` runs (`_compaction.py:849-918`) — collapsed into a single `assistant` message `[Tool results: func: result; func2: result2]`.
- **Topic memory files** by `MemoryContextProvider.consolidation_interval` (default 24h) using an LLM to compress durable facts into a tighter form (`_harness/_memory.py:56-67, 1484-1636`).

### 3. What gets deleted?

- **Excluded messages are not removed from in-memory or file storage** — they are marked via `EXCLUDED_KEY` (`_compaction.py:569-597, 1275-1305`) and filtered at read time only when the history provider's `skip_excluded=True` (`_sessions.py:839-865, 956-990, 1044-1046`). This is a deliberate design choice so compaction is reversible and replayable.
- **Redis session lists** are trimmed via `LTRIM` to `max_messages` (`python/packages/redis/agent_framework_redis/_history_provider.py:162-165`).
- **Whole Redis sessions** are deleted via `RedisHistoryProvider.clear` (`_history_provider.py:181-187`).
- **Whole Cosmos sessions** are deleted via `CosmosHistoryProvider.clear` (`azure-cosmos/_history_provider.py:203-226`).
- **Workflow checkpoints** can be deleted by ID on any backend (`_workflows/_checkpoint.py:158, 217, 392; azure-cosmos/_checkpoint_storage.py:304-336`) but **the framework does not do this automatically** — it only surfaces the API.
- **Per-file memories** can be deleted via `file_memory_delete_file` (`_harness/_file_memory.py:304-326`).
- **Memory topics** can be deleted via `delete_memory_topic` tool (`_harness/_memory.py:1233-1245`).

### 4. Does compaction break replay?

**No.** Compaction preserves the full original message list with bidirectional links:
- Summaries carry `_summary_of_message_ids` + `_summary_of_group_ids` (`_compaction.py:33-34, 900-903, 1067-1070`).
- Excluded originals carry `_summarized_by_summary_id` (`_compaction.py:35, 288-293, 895-896, 1081-1083`).
- Tool-result summaries carry `Tool results: ...` human-readable labels (`_compaction.py:886-889`).
- `EXCLUDE_REASON_KEY` records *why* a message was excluded (`truncation`, `sliding_window`, `tool_call_compaction`, `tool_result_compaction`, `summarized`, `token_budget_fallback`, `token_budget_fallback_strict` — `_compaction.py:717, 763, 816, 897, 1083, 1146, 1157`).

Caveat: by default, history providers (`InMemoryHistoryProvider`/`FileHistoryProvider`) do **not** filter excluded messages at load — they only do so when `skip_excluded=True` (`_sessions.py:839-865, 956-990`). If a consumer forgets to set that flag, compaction annotations exist but are silently ignored at the next run.

### 5. Can users request deletion?

**Partially.**
- **Redis sessions**: `RedisHistoryProvider.clear(session_id)` exists (`python/packages/redis/agent_framework_redis/_history_provider.py:181-187`) and is tested (`tests/test_providers.py:489-496`).
- **Cosmos sessions**: `CosmosHistoryProvider.clear(session_id)` exists with batched deletes (`azure-cosmos/_history_provider.py:203-226`) and is tested (`tests/test_cosmos_history_provider.py:252-272`).
- **Per-file memories**: `file_memory_delete_file` tool exposed to the model (`_harness/_file_memory.py:304-326`).
- **Memory topics**: `delete_memory_topic` tool (`_harness/_memory.py:1233-1245`).
- **`InMemoryHistoryProvider` / `FileHistoryProvider`**: **no `clear()` method** — to "delete" you would have to drop the session state key (`session.state.pop(source_id, ...)`) or remove the JSONL file outside the framework.
- **Workflow checkpoints**: `CheckpointStorage.delete(checkpoint_id)` is part of the Protocol (`_workflows/_checkpoint.py:158-167`) but no helper invokes it on retention — it's manual.
- **Privacy/anonymization**: the only pre-persist hook is `history_message_filter` in `MemoryContextProvider` (`_harness/_memory.py:69, 1147-1159`), which can drop or rewrite a single message before transcript save. There is **no** built-in `redact_pii` or field-level scrubber.

## Architectural Decisions

1. **Annotation over deletion.** `_compaction.py:582-597, 1275-1305` deliberately keep excluded messages in storage with `_excluded=True` rather than removing them. Tradeoff: replay fidelity, audit, and the ability to "un-compact" by re-inclusion — at the cost of not freeing memory until the host process drops the list or `skip_excluded=True` is set.
2. **Atomic group boundaries.** `_compaction.py:120-243` ensures tool-call + tool-result + reasoning-prefix never split. ADR-0019 (`docs/decisions/0019-python-context-compaction-strategy.md:54`) calls this out as the "critical correctness constraint" — without it, OpenAI/Azure APIs reject messages. Tests confirm atomicity (`tests/core/test_compaction.py:107-135`).
3. **Composable budget-aware strategies.** `TokenBudgetComposedStrategy` (`_compaction.py:1091-1160`) runs a chain, refreshes tokens, and falls back to oldest-group exclusion if the budget is unmet, with a strict fallback that drops system anchors only as a last resort (`_compaction.py:1149-1160`). The two-phase `ContextWindowCompactionStrategy` (`_compaction.py:1307-1419`) wraps two such composers (tool-eviction at 50% input budget, truncation at 80%) so each fires only on its own threshold.
4. **Default wiring in the harness.** When token params are passed to `create_harness_agent`, `_assemble_compaction_provider` (`_harness/_agent.py:72-120`) defaults to `ContextWindowCompactionStrategy` (before) + `ToolResultCompactionStrategy(keep_last_tool_call_groups=2)` (after). When neither token params nor custom strategies are provided, the provider is skipped entirely (`_harness/_agent.py:112-113`) — compaction is opt-in.
5. **Service-managed histories are explicitly out of scope.** ADR-0019 (`docs/decisions/0019-python-context-compaction-strategy.md:72-83`) and the `_assemble_compaction_provider` docstring (`_harness/_agent.py:87`) state that compaction is irrelevant when `service_session_id` is set, because the service owns its context window. This is a deliberate narrowing of scope.
6. **Per-provider retention is fragmented.** Only `RedisHistoryProvider` enforces a hard `max_messages` cap (`_history_provider.py:40-91, 151-165`). `CosmosHistoryProvider` has no `max_messages`; it relies on container-side TTL if the operator configures it. `InMemoryHistoryProvider` and `FileHistoryProvider` have no retention at all. There is no uniform `HistoryProvider.delete_user_data(session_id)` interface — the closest is the per-subclass `clear()`.

## Notable Patterns

- **In-place `__call__` with `bool` return** (`_compaction.py:54-62, 691-718, 746-764, 793-817, 849-918, 995-1088, 1122-1160, 1412-1419`): each strategy mutates `messages` and returns whether anything changed. Zero allocation in the no-op case.
- **Bidirectional trace metadata** (`_compaction.py:33-35, 288-293, 894-913, 1067-1086`): summaries store which message/group IDs they cover, and originals store which summary ID replaced them. This is the framework's answer to "compaction breaks replay": it doesn't.
- **`_serialize_message` excludes `raw_representation` and `items` from token estimation** (`_compaction.py:493-509`) to avoid double-counting `result`/`items` mirrors in `function_result` content.
- **`exclude_group_ids` with explicit reason** (`_compaction.py:591-597`): compaction strategies always pass a reason string (`"truncation"`, `"sliding_window"`, `"tool_call_compaction"`, etc.), giving downstream observability without needing separate instrumentation.
- **`_reannotation_start` walks back to group boundary** (`_compaction.py:413-425`): when a new message is appended, the previous group is re-annotated as a unit so its index/tokens stay aligned with the prefix.
- **`COMPACTION_STATE_KEY = "_compaction_messages"`** is exported (`_compaction.py:1179`) as the canonical session-state key namespace, but the module does not actually read/write this constant internally — it is a convention handed to host applications.
- **Two-phase budget pipeline** (`_compaction.py:1307-1419`): rather than a single token check, the framework composes *two* `TokenBudgetComposedStrategy` instances with different budgets, so a model run with 100k tokens first triggers tool-eviction at 50% of input budget (preserving recency of non-tool messages), then truncation at 80% (preserving recency of user/assistant exchanges).

## Tradeoffs

- **Replay safety vs. memory pressure.** The annotation model is great for replay but means the `session.state["messages"]` list never shrinks unless `skip_excluded=True` is set on the history provider. A long-running agent could pin gigabytes of old messages in RAM.
- **Provider-local retention.** Different providers implement different retention surfaces (Redis: LTRIM; Cosmos: TTL on the container; in-memory/file: nothing). There is no uniform contract; consumers must know per-provider.
- **Compaction is opt-in for the harness.** If you don't pass `max_context_window_tokens` and `max_output_tokens` (or custom strategies) to `create_harness_agent`, no `CompactionProvider` is built (`_harness/_agent.py:112-113`). A long session in a default-wired harness agent will not be compacted.
- **`CharacterEstimatorTokenizer` is the default** (`_compaction.py:66-70, 1382`). For real models this over/under-estimates by 20–50%, so the `tool_eviction_threshold=0.5` may trigger too early or too late. Production use should pass a `tiktoken`-backed tokenizer (sample at `python/samples/02-agents/compaction/tiktoken_tokenizer.py`).
- **Summarization cost.** `SummarizationStrategy` makes a chat-client call (`_compaction.py:1043-1052`) every time it triggers. There is **no caching** of the summary text — the model is re-prompted with all kept + summarized content on each cycle. ADR-0019 design §"Chainable / Composable" (`docs/decisions/0019-python-context-compaction-strategy.md:88-95`) discusses "incorporates any previously provided summary" but the implementation rebuilds the prompt from scratch.
- **Skip-excluded is **not** the default** on `InMemoryHistoryProvider`/`FileHistoryProvider`. Without `skip_excluded=True`, compaction annotations are silent — the same messages keep getting sent to the model.

## Failure Modes / Edge Cases

1. **All-tokenizer-stripped.** `apply_compaction` (`_compaction.py:1163-1176`) returns `messages` unchanged when `strategy is None`. Easy to forget — a `null` strategy means **no** compaction even if the budget is exceeded.
2. **Empty `summarized_by` chain.** A summary message references message IDs that were never persisted (e.g., truncated before storage); the back-link is harmless but the forward link may dangle. No integrity check.
3. **`skip_excluded` forgotten.** Compaction annotations exist but the model still sees the full history on the next turn. The provider defaults to `skip_excluded=False` (`_sessions.py:839, 956`).
4. **Empty storage round-trip.** `InMemoryHistoryProvider.save_messages` does `[*existing, *messages]` (`_sessions.py:889-890`) — repeated empty-list calls do nothing, but a malformed state where `state["messages"]` is a non-list raises a silent `TypeError` only at index time.
5. **Redis LTRIM race.** `RedisHistoryProvider.save_messages` runs `rpush ... pipeline.execute()` then `llen` then `ltrim` in two round trips (`_history_provider.py:157-165`). Between the `llen` and `ltrim`, a concurrent writer can push more, causing the trim to leave the list longer than `max_messages`. Not transactional.
6. **Cosmos `clear` is not atomic across batches.** Deletes are split into 100-item batches (`_history_provider.py:222-226`); a mid-run failure leaves partial state. Tests cover happy path only.
7. **System anchor exclusion in strict fallback.** `TokenBudgetComposedStrategy` (`_compaction.py:1149-1160`) will drop **system messages** to meet a token budget that is smaller than the system prompt. This is intentional but a sharp edge — the model's instructions vanish.
8. **Summarization `Exception` swallowing.** `SummarizationStrategy` (`_compaction.py:1053-1058`) catches *any* exception from the chat client and logs a warning, returning `False`. Transient failures silently skip compaction, allowing unbounded growth during outages.
9. **`include_tokens=True` mismatch in `_first_annotation_gaps`** (`_compaction.py:392-410`): when token annotations are missing and `tokenizer is None`, the function only checks group annotations. If a caller passes a tokenizer but does not also annotate token counts (e.g., custom `extend_compaction_messages` usage), token counts may be missing on the appended tail.
10. **Service-managed storage skips compaction entirely.** If a consumer sets `service_session_id` and *also* wires a `CompactionProvider`, the provider's `before_run`/`after_run` still runs against the loaded history, but the persisted history lives on the service side and won't be rewritten. `after_run` writes back to `session.state[history_source_id]` (`_compaction.py:1288-1295`), which is irrelevant if the service is the source of truth.

## Future Considerations

- **Unify retention across history providers.** Add a `max_messages` / `max_age_seconds` parameter to `HistoryProvider` base class (`_sessions.py:413-535`) and implement trim in `before_run` (consistent with how Redis LTRIMs in `save_messages`). Today, only Redis enforces a cap.
- **Add `clear()` to the base `HistoryProvider`.** Provide a default `NotImplementedError`-raising method so subclass absence is explicit, and implement it on `InMemoryHistoryProvider`/`FileHistoryProvider`.
- **Telemetry.** Surface per-run compaction metrics: `excluded_count_by_reason`, `included_token_count`, `summarizer_latency_ms`. The exclusion-reason taxonomy (`truncation` / `sliding_window` / `tool_call_compaction` / `tool_result_compaction` / `summarized` / `token_budget_fallback` / `token_budget_fallback_strict` — `_compaction.py:717-1157`) is already uniform and is one mapping away from OTel counters.
- **Bounded checkpoint retention.** Add `CheckpointStorage.max_checkpoints` and a periodic `evict_older_than(retention)` method, invoked from `Workflow._run_cleanup` (`_workflows/_workflow.py:832-836`).
- **Default tokenizer.** Ship a `tiktoken` implementation in core (sample exists at `python/samples/02-agents/compaction/tiktoken_tokenizer.py` but isn't the default). `CharacterEstimatorTokenizer` (`_compaction.py:66-70`) miscounts CJK and code.
- **Summary caching.** Reuse prior summary text on subsequent `SummarizationStrategy` runs to avoid re-summarizing the same conversation each turn; the default prompt already says "Incorporate any previously provided summary" (`_compaction.py:938`) but the implementation doesn't follow through.
- **Privacy primitives.** Add a `redact` strategy that scrubs PII patterns from summarized content, or a `history_message_filter` default that drops user messages past N days. Today `MemoryContextProvider.history_message_filter` (`_harness/_memory.py:954-1015`) is the only such hook.
- **Atomic `CosmosHistoryProvider.clear`.** Wrap multi-batch delete in a transactional batch or commit per-batch with a tombstone; the current implementation can leave partial state on failure (`_history_provider.py:222-226`).
- **Doc user flow when service + compaction are combined.** Currently a user who sets `service_session_id` plus a `CompactionProvider` will see compaction annotations on the locally-loaded history but the service-side thread is unchanged. Surface this in the harness docs / `CompactionProvider` constructor docstring.

## Questions / Gaps

1. **Retention for in-memory/file history?** `InMemoryHistoryProvider` (`_sessions.py:814-893`) and `FileHistoryProvider` (`_sessions.py:894-1134`) have no `max_messages`, no TTL, no `clear()`. Search boundary: I grepped for `max_messages | retention | delete_session | cap_messages | ttl` in both files; only `skip_excluded` matches. **No clear evidence found** for any retention primitive on these two providers.
2. **Compact-then-persist pipeline.** ADR-0019 §"Boundary vs In-Run Compaction" (`docs/decisions/0019-python-context-compaction-strategy.md:1208-1262`) discusses a `compact()` method on `HistoryProvider` that compacts and then writes back; the implementation in `_compaction.py:1182-1305` instead only annotates in place during `after_run` and relies on `skip_excluded` on the next `before_run`. **No clear evidence found** that compaction is *ever* persisted as a reduced transcript; the storage keeps the full annotated list.
3. **Service-side compaction contract.** ADR-0019 says "the service is responsible for its own context window management and compaction" but I did not find a machine-readable contract describing what behavior the framework assumes the service provides. **No clear evidence found** in the source tree (outside docs/decisions) for this contract.
4. **Telemetry for compaction.** No metrics module surfaces exclusion counts, summary lengths, or per-strategy trigger frequencies. Searched for `telemetry | metrics | counter` adjacent to `_compaction.py`; the only observability surface is the chat-client span (`observability.py:1535+`). **No clear evidence found** for compaction-specific observability.
5. **Per-message redaction hook on standard history providers.** Only `MemoryContextProvider.history_message_filter` (`_harness/_memory.py:69, 954-1015`) offers pre-persist rewriting. `InMemoryHistoryProvider`/`FileHistoryProvider`/`RedisHistoryProvider`/`CosmosHistoryProvider` do not expose an equivalent hook. **No clear evidence found** for a generic per-message filter on the standard `HistoryProvider` base class.
6. **`CharacterEstimatorTokenizer` accuracy.** Default is `len(text) // 4` (`_compaction.py:69-70`), which undercounts for English (typical ratio is ~3.5–4 chars/token, varies by model) and overcounts for code/CJK. There is no built-in tokenizer tied to the chat client's tokenizer family. **No clear evidence found** for a model-aware tokenizer in core.
7. **Multi-session retention sweep.** `CosmosHistoryProvider.list_sessions` exists (`azure-cosmos/_history_provider.py:228-240`) but no scheduled sweep is provided. **No clear evidence found** for an out-of-the-box TTL/sweeper on Cosmos.

---

Generated by `dimensions/02.09-state-pruning-compaction-and-retention.md` against `agent-framework`.
