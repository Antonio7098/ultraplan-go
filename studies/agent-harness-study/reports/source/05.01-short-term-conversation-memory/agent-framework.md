# Source Analysis: agent-framework

## Dimension 05.01: Short-Term Conversation Memory

### Source Info

| Field | Value |
|-------|-------|
| Name | agent-framework (microsoft/agent-framework) |
| Path | `studies/agent-harness-study/sources/agent-framework` |
| Language / Stack | Polyglot: Python (`python/packages/*`), .NET (`dotnet/src/*`); Go is a placeholder pointing to a separate repo (`go/README.md:1-3`) |
| Analyzed | 2026-08-25 |

## Summary

Short-term conversation memory in agent-framework is a first-class, provider-pluggable subsystem implemented symmetrically in the Python and .NET stacks. The model's view of "recent context" is assembled per run through a **history-provider pipeline**: an abstract storage contract (Python `HistoryProvider`, `python/packages/core/agent_framework/_sessions.py:943`; .NET `ChatHistoryProvider`, `dotnet/src/Microsoft.Agents.AI.Abstractions/ChatHistoryProvider.cs:51`) loads persisted messages before a run and appends new input/output messages after it, with state kept either in the session object (`AgentSession.state` / `AgentSession.StateBag`), on local disk (append-only JSONL/MessagePack), or server-side via service-managed conversation ids.

The default store is session-scoped and in-memory (Python `InMemoryHistoryProvider`, `_sessions.py:2087`; .NET `InMemoryChatHistoryProvider`, `InMemoryChatHistoryProvider.cs:27`), auto-attached by the agent when a session is used without explicit providers or service-side storage (`_agents.py:1394-1405`; `dotnet/src/Microsoft.Agents.AI/ChatClient/ChatClientAgent.cs:131-134`). History is passed to the model **in full** by default — there is no implicit windowing — but an optional, well-developed compaction layer (Python `agent_framework._compaction`; .NET `Microsoft.Agents.AI/Compaction/`) provides token-aware truncation, sliding windows, tool-result collapsing, LLM summarization, and pipeline composition. Tool-call groups are treated atomically during compaction so call/result pairs are never split, and unresolved approval-control messages are filtered out of replay while pending ones survive.

During multi-iteration tool-calling loops, the framework replays the accumulated transcript to the model each iteration locally, or delegates history ownership to the service when `conversation_id`/store semantics say the service manages it (`_tools.py:2768-2772`; `ChatClientAgent.cs:762-773`). Both stacks carry explicit conflict detection between local and service-side history.

## Rating

**8 / 10**

Rationale against the rubric:
- **Clear model with explicit interfaces (7–8 band):** The storage contract is small and explicit — Python `get_messages`/`save_messages` with configurable `load_messages`/`store_inputs`/`store_outputs`/`store_context_messages` flags (`_sessions.py:967-1000`); .NET `InvokingAsync`/`InvokedAsync`/`ProvideChatHistoryAsync`/`StoreChatHistoryAsync` with injectable load/store message filters (`ChatHistoryProvider.cs:69-77,141-153,248-259`).
- **Operational safeguards:** deduplication of replayed transcripts (`filter_new_messages`, `_sessions.py:216-244`, with tests at `tests/core/test_sessions.py:1387,1795,1838`), atomic tool-call grouping in compaction (`_compaction.py:280-297`), approval-placeholder filtering before replay (`_sessions.py:1056`, logic at `_sessions.py:878-940`), local-vs-service conflict warnings/errors (`_agents.py:1426-1435`; `ChatClientAgent.cs:827-852`), and security notes on stored-history prompt injection (`ChatHistoryProvider.cs:43-49`; `SummarizationCompactionStrategy` risk documented in `_compaction.py:1207-1217`).
- **Tested:** ~74 compaction tests + 138 session tests in Python core alone (`packages/core/tests/core/test_compaction.py`, `test_sessions.py`); dedicated .NET suites for every strategy plus chat-history management (`dotnet/tests/Microsoft.Agents.AI.UnitTests/Compaction/*.cs`, `ChatClient/ChatClientAgent_ChatHistoryManagementTests.cs`, `Abstractions.UnitTests/InMemoryChatHistoryProviderTests.cs`).
- **Why not 9–10:** No default protection against unbounded growth — full history is sent unless a user opts into compaction; there is no built-in observability surface showing what the model actually saw per call (only feature-usage markers such as `mark_feature_used(FeatureIndex.CORE_IN_MEMORY_HISTORY_PROVIDER)` at `_sessions.py:2144`, and .NET `FeatureUsage.MarkUsed` at `InMemoryChatHistoryProvider.cs:92`, plus compaction logging `CompactionLogMessages.cs`); cross-language parity is close but not identical (Python has incremental annotation/dedup machinery; .NET leans on external `IChatReducer` from Microsoft.Extensions.AI).

## Evidence Collected

Every entry includes a file path with line numbers.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Message type & roles | `Message(role, contents, ...)` with roles `"system" \| "user" \| "assistant" \| "tool"` (`RoleLiteral`) | python/packages/core/agent_framework/_types.py:1699,1754-1844 |
| Input normalization | `normalize_messages` coerces str/Content → user-role `Message` | python/packages/core/agent_framework/_types.py:1850-1884 |
| Instruction injection role handling | `prepend_instructions_to_messages` prepends system-role instructions with duplicate suppression | python/packages/core/agent_framework/_types.py:1887-1939 |
| Session container | `AgentSession` holds `session_id`, `service_session_id`, mutable `state` dict; serializable via `to_dict`/`from_dict` | python/packages/core/agent_framework/_sessions.py:1717-1791 |
| History storage contract | `HistoryProvider(ContextProvider)` with abstract `get_messages`/`save_messages` and `load_messages`/`store_inputs`/`store_outputs` flags | python/packages/core/agent_framework/_sessions.py:943-1037 |
| Load-before-run | `HistoryProvider.before_run` fetches messages, filters resolved approval controls, extends context | python/packages/core/agent_framework/_sessions.py:1047-1057 |
| Store-after-run | `after_run` stores configured context + inputs + response messages | python/packages/core/agent_framework/_sessions.py:1059-1075 |
| Default store (session-scoped) | `InMemoryHistoryProvider` keeps messages under `state["messages"]`; supports `skip_excluded` to honor compaction exclusions | python/packages/core/agent_framework/_sessions.py:2087-2168 |
| Durable store (file) | `FileHistoryProvider`: one append-only JSONL (or length-prefixed msgpack) file per session id; striped file locks; traversal-safe filenames; plaintext-storage security note | python/packages/core/agent_framework/_sessions.py:2172-2346 (esp. 2181-2189, 2294-2303, 2324-2346) |
| Replay dedup | `filter_new_messages` handles append-only and full-transcript-replay shapes without superlinear growth | python/packages/core/agent_framework/_sessions.py:191-244 |
| Auto-injection of history | Agent auto-appends `InMemoryHistoryProvider()` when a session exists, no loading provider registered, and no service-side storage | python/packages/core/agent_framework/_agents.py:1394-1405 |
| Per-run assembly | `_prepare_session_and_messages` runs provider `before_run` in forward order; final `session_messages = session_context.get_messages(include_input=True)` is what the client receives | python/packages/core/agent_framework/_agents.py:1572-1651,1516-1517 |
| Context attribution | `SessionContext.extend_messages` copies messages and stamps `_attribution.source_id` (+ `origin_session_ids` for cross-session content); `get_messages` flattens in provider order | python/packages/core/agent_framework/_sessions.py:624-698,757-790 |
| Service-side vs local | `store` option / `client.STORES_BY_DEFAULT` decide who owns history (OpenAI Responses client sets `STORES_BY_DEFAULT = True`); warning when a loading provider meets a storing service | python/packages/core/agent_framework/_agents.py:1379-1392,1426-1435; python/packages/openai/agent_framework_openai/_chat_client.py:397; python/packages/core/agent_framework/_clients.py:279 |
| Function-loop replay | Loop copies caller messages once, calls the model with the growing `prepared_messages`, then `_prepare_messages_for_next_iteration` either extends with response messages (local history) or replaces everything with just the last message (service-managed) | python/packages/core/agent_framework/_tools.py:3195,3240-3252,3305,2768-2772 |
| Compaction applied per model call | `BaseChatClient.get_response` → `_prepare_messages_for_model_call` applies strategy/tokenizer annotations in place on the loop-owned list (regression note cites issue #4991) | python/packages/core/agent_framework/_clients.py:366-394,527,547 |
| Grouping for windowing | `group_messages` builds atomic spans: system, user, assistant_text, tool_call (reasoning prefix + assistant function_calls + following tool messages merged; non-contiguous call/result linked via union-find) | python/packages/core/agent_framework/_compaction.py:200-325,42-48,150-198 |
| Truncation strategy | Oldest-first exclusion above `max_n` toward `compact_to` (token- or count-based), system groups preserved, last group never dropped | python/packages/core/agent_framework/_compaction.py:797-867 |
| Sliding-window strategy | Keeps most recent N non-system groups; system anchors preserved | python/packages/core/agent_framework/_compaction.py:870-913 |
| Tool-focused strategies | `SelectiveToolCallCompactionStrategy` drops older tool groups; `ToolResultCompactionStrategy` collapses them into bounded `[Tool results: …]` summary messages with bidirectional trace links | python/packages/core/agent_framework/_compaction.py:916-1101 |
| Summarization strategy | LLM-generated ≤5-sentence summary replaces old groups when non-system count exceeds target+threshold; 8k-token summarizer input budget; failure escalation after 3 consecutive failures; documented indirect-prompt-injection trust caveat | python/packages/core/agent_framework/_compaction.py:1176-1296,1193-1194,1207-1217 |
| Compaction wiring | `CompactionProvider(before_strategy, after_strategy, tokenizer, history_source_id)` compacts loaded context pre-run and persisted history post-run (deferred to end of turn via `after_run_once_per_turn`) | python/packages/core/agent_framework/_compaction.py:1494-1614,1532-1534 |
| Message injection (edit next call) | `enqueue_messages` + `MessageInjectionMiddleware` drain queued messages into the next model call for the session | python/packages/core/agent_framework/_sessions.py:1364-1435,1506+ |
| Approval controls in replay | `_filter_approval_control_messages` removes resolved approval requests/responses but preserves unresolved/pending occurrences until terminal result closes them | python/packages/core/agent_framework/_sessions.py:873-940,1056 |
| Persistence gating | `_RunPersistenceGate` defers durable history writes until egress verdict permits content | python/packages/core/agent_framework/_sessions.py:1078-1287 (gate at 1169) |
| .NET storage contract | Abstract `ChatHistoryProvider`: `InvokingCoreAsync` merges provided history (source-stamped `AgentRequestMessageSourceType.ChatHistory`) ahead of request messages; `InvokedCoreAsync` skips storage on failure and filters before `StoreChatHistoryAsync` | dotnet/src/Microsoft.Agents.AI.Abstractions/ChatHistoryProvider.cs:51-77,141-153,248-259 |
| .NET default store | `InMemoryChatHistoryProvider` stores `List<ChatMessage>` in `AgentSession.StateBag`; optional `IChatReducer` triggered BeforeMessagesRetrieval or AfterMessageAdded | dotnet/src/Microsoft.Agents.AI.Abstractions/InMemoryChatHistoryProvider.cs:27-129 |
| .NET agent wiring | `ChatClientAgent` defaults to `new InMemoryChatHistoryProvider()`; merges history+input via `LoadChatHistoryAsync`; runs `AIContextProviders.InvokingAsync` pipeline; throws/warns/clears on conversation-id-vs-provider conflict | dotnet/src/Microsoft.Agents.AI/ChatClient/ChatClientAgent.cs:131-134,762-808,816-856 |
| .NET per-service-call persistence | `PerServiceCallChatHistoryPersistingChatClient` owns history lifecycle per service call; sentinel `ConversationId` signals service-managed mode to `FunctionInvokingChatClient` | dotnet/src/Microsoft.Agents.AI/ChatClient/PerServiceCallChatHistoryPersistingChatClient.cs:57-69 |
| .NET scoping (user/thread/session) | `ChatHistoryMemoryProviderScope` exposes ApplicationId / AgentId / SessionId / UserId used as vector-store filter keys | dotnet/src/Microsoft.Agents.AI/Memory/ChatHistoryMemoryProviderScope.cs:10-52; usage in Memory/ChatHistoryMemoryProvider.cs:265-305 |
| .NET compaction suite | `CompactionStrategy` base with triggers; `ContextWindowCompactionStrategy` derives budgets as `maxContextWindowTokens - maxOutputTokens` (tool-eviction at 50%, truncation at 80%); SlidingWindow / Truncation / ToolResult / Summarization / Pipeline strategies mirror Python | dotnet/src/Microsoft.Agents.AI/Compaction/{CompactionStrategy.cs:50,ContextWindowCompactionStrategy.cs:34-44,SlidingWindowCompactionStrategy.cs,TruncationCompactionStrategy.cs,ToolResultCompactionStrategy.cs,SummarizationCompactionStrategy.cs:48,PipelineCompactionStrategy.cs} |
| Tests (Python) | Dedup tests (`test_save_messages_deduplicates_identical_messages`, different-roles-not-deduplicated, file integrity), approval-filter tests, 74 compaction tests incl. atomicity and reused-call-id cases | python/packages/core/tests/core/test_sessions.py:1387,1417,1795,1838,374-426; test_compaction.py:159-997 (74 tests) |
| Tests (.NET) | Reducer trigger-event tests, exception-skip storage test, conversation-id management tests, per-strategy compaction suites | dotnet/tests/Microsoft.Agents.AI.Abstractions.UnitTests/InMemoryChatHistoryProviderTests.cs:241-361; ChatClient/ChatClientAgent_ChatHistoryManagementTests.cs:28-120; Compaction/*.cs |

## Answers to Dimension Questions

**1. What conversation history does the model see?**
By default: *everything* ever stored for the session, oldest-first, followed by the new input. Concretely, the Python agent builds `session_messages = session_context.get_messages(include_input=True)` where context messages were loaded by each provider's `before_run` in registration order (`_agents.py:1623-1637,1516-1517`; ordering defined at `_sessions.py:757-790`). In .NET, `InvokingCoreAsync` returns provider history concatenated ahead of the request messages (`ChatHistoryProvider.cs:150-152`), then AI-context providers may further augment the list (`ChatClientAgent.cs:778-794`). Instructions/system text are prepended as system-role messages with de-duplication across layers (`_types.py:1920-1939`). If a compaction strategy is configured, exclusions/summaries shrink that set before each model call (`_clients.py:366-394`; `_compaction.py:1565-1590`).

**2. What gets dropped?**
Nothing automatically — dropping only happens if (a) a compaction/reduction strategy excludes groups (truncation drops oldest non-system groups but always retains the newest group and optionally system anchors, `_compaction.py:797-867`; sliding window drops all but the last N groups, `_compaction.py:895-913`); (b) `skip_excluded=True` makes reload honor prior exclusions (`_sessions.py:2124-2150,2302-2303`); (c) resolved approval control contents are stripped from replay while pending ones stay (`_sessions.py:921-940`); (d) audit-only configurations set `load_messages=False` or `store_outputs=False` etc. (`_sessions.py:967-972`). On the storage side, `store_context_messages` defaults to False, so other providers' injected context (e.g., RAG results) is not persisted unless opted in (`_sessions.py:970,1039-1045`).

**3. Are tool messages retained?**
Yes. Tool-role messages and function-call/result contents are ordinary `Message`s stored by `after_run` and replayed like any other (`_sessions.py:1059-1075`). Compaction goes further to keep them coherent rather than silently dropping halves: tool-call groups merge assistant `function_call` declarations, reasoning prefixes, and subsequent `tool` messages into one atomic span (`_compaction.py:280-297`), non-contiguous result→declaration pairs are linked by unique `call_id` matching with union-find (`_compaction.py:105-122,150-198`), and ambiguous duplicates deliberately stay separate (`_compaction.py:118-121`, tests at `tests/core/test_compaction.py:294-345`). The .NET side mirrors this in `CompactionMessageIndex` tests (`CompactionMessageIndexTests.cs`, 82 cases). Approval placeholder results (`[APPROVAL_PENDING]`) are recognized and not treated as terminal (`_sessions.py:873-875`).

**4. Is memory per user/thread/session?**
Scoped per `AgentSession` by construction: Python stores live in `session.state[source_id]["messages"]` keyed by auto-generated UUID `session_id` (`_sessions.py:1748-1750,2088-2095`); the file provider writes one file per session id with encoded/traversal-safe names (`_sessions.py:168-178,2172-2200`). Cross-session attribution exists for governance: injected messages can carry `origin_session_ids` (`_sessions.py:646-659,667-668`). Service-managed memory is keyed by `service_session_id` / `conversation_id` handled by the remote service (`_agents.py:1390-1402`; `ChatClientAgentSession.ConversationId` at `ChatClientAgent.cs:755-760`). There is no built-in per-user hierarchy in the default stores, but the .NET vector-backed `ChatHistoryMemoryProvider` demonstrates the intended pattern with explicit ApplicationId/AgentId/UserId/SessionId scope fields used as storage filter keys (`ChatHistoryMemoryProviderScope.cs:35-52`; `Memory/ChatHistoryMemoryProvider.cs:280-283`).

**5. Can history be edited or forked?**
Yes, through several supported seams. Editing: `InMemoryChatHistoryProvider.GetMessages/SetMessages` expose the raw list (`InMemoryChatHistoryProvider.cs:71-86`); Python callers can mutate `state["messages"]` or use `enqueue_messages`/`MessageInjectionMiddleware` to append synthetic messages for the next model call (`_sessions.py:1364-1435`); both stacks accept custom `ChatHistoryProvider`/`HistoryProvider` subclasses overriding retrieval entirely. Forking: `AgentSession.to_dict()`/`from_dict()` round-trip serialization enables snapshot/clone semantics (`_sessions.py:1757-1791`) — the loop middleware uses exactly this to snapshot and restore a session between iterations (`fresh_context` reset described in `packages/core/AGENTS.md`, implemented in `agent_framework/_harness/_loop.py`). Source-attribution stamps (`_attribution.source_id`, `WithAgentRequestMessageSource` at `ChatHistoryProvider.cs:151`) let downstream code distinguish history-sourced from fresh caller messages, which is also what prevents re-storing loaded history (`ChatHistoryProvider.cs:53-54,75`).

## Architectural Decisions

1. **Providers over monoliths.** Conversation memory is decomposed into orthogonal context providers composed per agent (history, compaction, RAG/text-search, skills, file memory). Loading order equals registration order and post-run hooks run in reverse (`_agents.py:553-606`; forward-order `before_run` at `_agents.py:1623-1637`). This lets audit-only or evaluation-only history providers exist ("stores only, doesn't load", `_sessions.py:946-949`).

2. **Explicit dual-ownership of history: local vs service.** Both stacks formalize whether the model service owns the transcript. Python resolves this from the `store` option and `client.STORES_BY_DEFAULT` (`_agents.py:1379-1392`); .NET detects a returned `ConversationId` and then warns/throws/nulls the local provider per configuration (`ChatClientAgent.cs:827-852`). A dedicated decorator (`PerServiceCallChatHistoryPersistingChatClient`, `PerServiceCallChatHistoryPersistingChatClient.cs:57-69`; Python twin `PerServiceCallHistoryPersistingMiddleware`, `_sessions.py:1535`) persists every service call even when the service owns history.

3. **Annotation-based compaction on the stored transcript.** Instead of destructive trimming, Python marks messages with group/token/exclusion annotations in `additional_properties` (`_compaction.py:27-37`) and projects the included subset (`project_included_messages`, `_compaction.py:736-737`). Original data survives in storage; exclusion is a projection decision re-evaluated each run. Summaries carry bidirectional trace links (`_summary_of_message_ids`/`_summarized_by_summary_id`, `_compaction.py:35-37`, written at `_compaction.py:1056-1078`).

4. **Atomicity of causally-linked messages.** Call/result pairing, reused-`call_id` occurrence awareness, and reasoning-prefix merging are treated as correctness requirements, not optimizations — enforced by `_function_pair_reannotation_start` (`_compaction.py:510-540`) and extensive tests (`test_compaction.py:294-345`; AGENTS.md mandates spec-level review for this area: `python/AGENTS.md` "Function-Calling Loop Changes").

## Notable Patterns

- **Zero-config default path:** passing just a session to `agent.run()` transparently attaches `InMemoryHistoryProvider` (`_agents.py:1396-1405`) — short-term memory works with no explicit wiring.
- **Identity-based deduplication:** `filter_new_messages` aligns incoming sequences against existing hashes (message-id first, role+content hash fallback) to support both append-only stores and full-transcript replays without duplicating turns (`_sessions.py:216-244`).
- **Trigger-point flexibility:** .NET reducers fire either before retrieval or after store (`InMemoryChatHistoryProviderOptions.ChatReducerTriggerEvent`, `InMemoryChatHistoryProvider.cs:50,97-123`), letting users choose read-time vs write-time reduction.
- **Budget-derived windowing (.NET):** `ContextWindowCompactionStrategy` computes the input budget as `maxContextWindowTokens − maxOutputTokens` with staged thresholds (tool eviction at 50%, truncation at 80%) — the closest thing in-repo to "does the model see enough without overflowing?" being answered quantitatively (`ContextWindowCompactionStrategy.cs:13-44`).
- **Persistence deferral for safety:** durable history writes can be held behind a run-persistence gate until an egress policy verdict allows the content (`_RunPersistenceGate`, `_sessions.py:1078-1092,1151-1287`) — an unusual, governance-oriented safeguard.
- **Failure containment:** summarization failures don't crash runs; they log, escalate after three consecutive failures, and return `False` ("no change") (`_compaction.py:1273-1284`, test at `tests/core/test_compaction.py:975-996`).

## Tradeoffs

- **Full-replay by default vs overflow risk.** Without opting into compaction, long sessions grow the prompt unboundedly until the provider rejects the request. The capability exists (`ContextWindowCompactionStrategy`, token-aware `TruncationStrategy`) but is not on by default in either stack.
- **Annotations in `additional_properties` vs payload size.** Persisting group/token/exclusion metadata alongside messages (`_compaction.py:614`) makes compaction incremental and durable but inflates stored/transmitted payloads and couples storage format to framework internals.
- **LLM summarization vs injection surface.** Summarized text becomes trusted assistant-authored history permanently; the docs explicitly warn that a compromised summarizer is a persistent indirect-prompt-injection channel and require opt-in trust parity with the primary model (`_compaction.py:1207-1217`). The same class of risk is flagged for any history backend (`ChatHistoryProvider.cs:43-49`).
- **Dedup heuristics vs legitimate repeats.** Content-hash fallback identity means two genuinely identical user messages in one batch could be collapsed by the set-based fallback path (`_sessions.py:238-243`); the sequence-alignment fast paths mitigate this for normal replays, and a test pins that different roles with same text are not deduped (`test_sessions.py:1417`).
- **Polyglot duplication.** Two parallel implementations (Python `_compaction.py` ≈1.7k lines; .NET `Compaction/` ≈15 files) must be kept behaviorally aligned; e.g., Python has incremental re-annotation machinery (`annotate_message_groups(from_index=…)`, `_compaction.py:543-624`) whose .NET counterpart lives in a differently-shaped `CompactionMessageIndex`.

## Failure Modes / Edge Cases

- **Orphaned tool results:** a tool message whose declaration was already excluded must not be retained alone; pinned by `test_sliding_window_does_not_retain_orphan_result_after_assistant_embedded_result` (`test_compaction.py:346-380`).
- **Reused `call_id` occurrences:** approval flows can reuse call ids across rounds; pairing is occurrence-based, not id-global (`_sessions.py:878-918`; `test_compaction.py:310-345`).
- **Service did not return a conversation id for a conversation-id session:** hard `InvalidOperationException` because the session cannot be honored (`ChatClientAgent.cs:818-823`).
- **Conflicting session ids in options vs session:** rejected up front (`ChatClientAgent.cs:746-753`; Python equivalent precedence rules at `_agents.py:1390-1393`).
- **Storage failure/integrity:** malformed JSONL lines are skipped with debug logs during dedup reads rather than failing the run (`_sessions.py:2334-2340`, integrity test `test_sessions.py:1838`); .NET skips storing entirely when the invocation threw (`ChatHistoryProvider.cs:250-253`, tested at `InMemoryChatHistoryProviderTests.cs:361`).
- **Summarizer outage:** consecutive failures degrade to no-op compaction with error-level logging after threshold (`_compaction.py:1273-1284`).
- **Cross-process concurrency:** `FileHistoryProvider` documents that it provides no cross-host locking and stores plaintext (`_sessions.py:2181-2189`) — durability is best-effort single-host.

## Future Considerations

- Make overflow-safe behavior opt-out rather than opt-in: a default conservative token ceiling (like .NET's `ContextWindowCompactionStrategy` thresholds) would protect naive users who today get full replay (`_agents.py:1516-1517`).
- Add a first-class introspection hook ("what did the model see") exposing the projected message list per call; today it must be reconstructed via middleware or telemetry.
- Converge Python/.NET compaction semantics behind shared conformance tests to prevent drift between the two implementations.
- Extend per-user scoping beyond the .NET vector-store provider pattern (`ChatHistoryMemoryProviderScope.cs`) into the default stores, since sessions currently identify neither user nor thread beyond an opaque UUID.

## Questions / Gaps

- **No built-in cross-session recall in the default path:** `origin_session_ids` attribution exists (`_sessions.py:646-668`), but the in-tree providers that populate it (e.g., mem0 package) are separate integrations; the pure short-term layer itself does no retrieval. Searched `packages/core` and `packages/mem0`; conclusion based on absence of retrieval calls in `HistoryProvider`/`InMemoryHistoryProvider`.
- **Go stack could not be evaluated:** `go/README.md:1-3` only links to an external repository (`microsoft/agent-framework-go`); no Go source exists inside this source directory, so all findings cover Python and .NET only.
- **Exact token-counting quality:** the shipped estimator is a 4-chars/token heuristic (`CharacterEstimatorTokenizer`, `_compaction.py:76-80`); real-tokenizer integration points exist (`TokenizerProtocol`, `_compaction.py:51-57`) but no concrete tokenizer implementation was found inside this source (searched for `count_tokens` implementations beyond the heuristic and protocol). Accuracy of budget enforcement therefore depends on user-supplied tokenizers.

---

Generated by `05.01-short-term-conversation-memory` against `agent-framework`.
