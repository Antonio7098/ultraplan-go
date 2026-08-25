# Source Analysis: pydantic-ai

## Dimension 03.07 — Context Refresh Inside the Loop

### Source Info

| Field | Value |
|-------|-------|
| Name | pydantic-ai |
| Path | `studies/agent-harness-study/sources/pydantic-ai` |
| Language / Stack | Python (asyncio, dataclasses, Pydantic, pydantic-graph) — `pydantic_ai_slim/pydantic_ai/` is the core agent loop |
| Analyzed | 2026-07-17 |

## Summary

Pydantic AI's agent loop runs on `pydantic-graph` and represents a conversation as an append-only `list[ModelMessage]` on `GraphAgentState.message_history` (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:125`). Per turn the loop does NOT rebuild the context from scratch — it appends the new `ModelRequest`, calls a per-turn `before_model_request` capability chain (`_agent_graph.py:895`), runs `Model.prepare_messages` (`_agent_graph.py:938`) which may split native tool-search parts, merges consecutive same-role messages (`_clean_message_history`, `_agent_graph.py:2220`), increments usage, then hands the messages to the adapter.

There is no built-in summarizer, no built-in sliding-window dropper, and no built-in overflow detector inside the framework loop. The framework's only first-party mechanism for shrinking context is **user-supplied `ProcessHistory` capabilities** (`pydantic_ai_slim/pydantic_ai/capabilities/process_history.py:27-44`), which run on every `before_model_request` and rewrite the in-flight `ModelRequestContext.messages`. The framework exposes two **provider-native compaction** capabilities — `AnthropicCompaction` (`pydantic_ai_slim/pydantic_ai/models/anthropic.py:2016-2081`) and `OpenAICompaction` (`pydantic_ai_slim/pydantic_ai/models/openai.py:3995-4194`) — that delegate compaction to the provider's `context_management`/`/responses/compact` endpoint and surface the result as a `CompactionPart` (`pydantic_ai_slim/pydantic_ai/messages.py:1681-1727`).

For **context visibility** the loop keeps two per-run state sets that are *recomputed from the message history on every model request*: `loaded_capability_ids` (rebuilt by `_refresh_loaded_capability_ids`, `_agent_graph.py:1431-1447`) and `discovered_tool_names` (rebuilt by `_refresh_discovered_tool_names`, `_agent_graph.py:1450-1456`). Dynamic system prompts are re-evaluated mid-run for any `SystemPromptPart` carrying a `dynamic_ref` (`_reevaluate_dynamic_prompts`, `_agent_graph.py:416-441`). Together these implement a **tool / capability / system-prompt refresh** inside the loop, but they don't drop or summarize the conversational history itself.

Truncation is opt-in for one specific tool: `WebFetchLocalTool` truncates fetched content to `max_content_length` characters (default 50 000) and appends `[Content truncated]` (`pydantic_ai_slim/pydantic_ai/common_tools/web_fetch.py:127-128`). Token limits can be enforced via `UsageLimits` (`pydantic_ai_slim/pydantic_ai/usage.py:263-420`), which can run a `count_tokens` pass before the request when `count_tokens_before_request=True` (`usage.py:283-296`) and supported by Anthropic, Google, Bedrock Converse, OpenAI Responses (`_agent_graph.py:953-958`). On overflow the model returns `finish_reason='length'`, which the loop translates into `UnexpectedModelBehavior` or `IncompleteToolCall` (`_agent_graph.py:1111-1113`, `_agent_graph.py:147-160`) — there is no automatic compaction retry path.

Net: context refresh inside the loop is **explicit and extensible** (ProcessHistory + provider compaction + tool search + capability loading), but the framework ships **no default summarizer, no default dropper, no automatic context-window guard**. Out-of-the-box the loop keeps appending; once the conversation exceeds the model's window, the user is responsible for hooking in a `ProcessHistory` or a `*Compaction` capability.

## Rating

**Score: 7 / 10 — Clear model with explicit interfaces, tests for the public mechanisms, and operational safeguards for the mechanisms that exist, but no first-party default summarizer/compactor and no automatic context-window overflow handling inside the loop.**

### Rationale

What works well (and is rated highly):

- The "messages list grows, and a per-turn capability chain can rewrite it" model is clean, well documented, and well tested (`tests/test_history_processor.py:1-1785`, `docs/message-history.md:509-743`).
- Tool / capability / system-prompt refresh *is* automatic and well-tested (tool search re-loads from history; deferred capabilities re-load via `load_capability`).
- Token accounting is explicit (`RunUsage`/`RequestUsage` + `UsageLimits`) and the framework supports pre-request token counting on Anthropic, Google, Bedrock, OpenAI Responses.
- `CompactionPart` is a first-class message part with provider-name-aware routing, ensuring the framework can round-trip provider-specific compaction state.
- The plumbing is extensively documented — `ProcessHistory`, `AnthropicCompaction`, `OpenAICompaction`, `ToolSearchToolset`, `DeferredLoadingToolset`, and `UsageLimits` are all described in docs.

What keeps it at 7 rather than 9-10:

- **No default context-management policy.** A user who runs a long conversation with no `ProcessHistory` capability will see the framework keep appending messages; the framework does not auto-summarize, auto-trim, or auto-fail-fast on context growth. The summarization recipe in `docs/message-history.md:629-659` is shown as a user-supplied pattern, not a built-in.
- **No automatic overflow response.** `finish_reason='length'` raises `UnexpectedModelBehavior` with a message pointing at `max_tokens`, not at compaction (`_agent_graph.py:1111-1113`). There is no `try_compact_then_retry` loop.
- **Token counting before request is opt-in (`count_tokens_before_request=False` by default, `usage.py:283-296`) and only supported on four providers.**
- **Provider-side compaction is provider-specific.** `AnthropicCompaction` only works on Anthropic; `OpenAICompaction` only works on OpenAI Responses; other providers (Google, Mistral, Cohere, Bedrock non-Anthropic, xAI, Groq, HuggingFace) only have the base `compact_messages` `NotImplementedError` fallback (`models/__init__.py:288-300`).
- **Tool results are preserved verbatim.** No framework mechanism drops or summarizes old `ToolReturnPart`s; they only get dropped via a user-supplied `ProcessHistory` (and the docs explicitly warn about tool-call/return pairing, `docs/message-history.md:599-600, 661-662`).
- **The historical `Agent.history_processors` API is deprecated and re-routed through `ProcessHistory` (`agent/__init__.py:421-431`)**; this is a sign that the abstraction is still evolving.

## Evidence Collected

Every entry includes a file path with line numbers. Format: `path/to/file.py:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| State holding the message history | `GraphAgentState.message_history: list[_messages.ModelMessage]` — the canonical, append-only conversation log shared with `capture_run_messages` via `ctx.state.message_history[:] = messages` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:125`, `pydantic_ai_slim/pydantic_ai/_agent_graph.py:284-288` |
| Per-turn request append | Each `ModelRequestNode.run` appends the current `self.request` before any model call | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:854` |
| Per-turn response append | Each model response is appended via `_append_response` (also increments `RunUsage` and runs `UsageLimits.check_tokens`) | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1015-1025` |
| Per-turn `run_step` increment | `ctx.state.run_step += 1` (triggers `for_run_step` re-resolution) | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:856` |
| Per-turn capability hook | `before_model_request` is the documented interception point for history rewriting | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:895-899` |
| Per-turn tool manager refresh | `for_run_step` rebuilds the `ToolManager` (calls `toolset.for_run_step` and `toolset.get_tools`) — discovers deferred tools and resolves dynamic toolsets | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:872`, `pydantic_ai_slim/pydantic_ai/tool_manager.py:120-145` |
| Per-turn loaded-capability set refresh | `_refresh_loaded_capability_ids` scans history for `LoadCapabilityReturnPart` and mutates the shared `ctx.deps.loaded_capability_ids` set | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1431-1447`, `pydantic_ai_slim/pydantic_ai/_deferred_capabilities.py:141-154` |
| Per-turn discovered-tool set refresh | `_refresh_discovered_tool_names` scans history for `ToolSearchReturnPart` / `NativeToolSearchReturnPart` and mutates the shared `ctx.deps.discovered_tool_names` set | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1450-1456`, `pydantic_ai_slim/pydantic_ai/toolsets/_tool_search.py:185-216` |
| Dynamic system prompt re-evaluation | `_reevaluate_dynamic_prompts` walks messages, re-runs any `SystemPromptPart` whose `dynamic_ref` is set | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:416-441` |
| Static-vs-dynamic instruction ordering | `InstructionPart.sorted` orders static (`dynamic=False`) before dynamic so providers can cache the static prefix | `pydantic_ai_slim/pydantic_ai/messages.py:1542-1545` |
| `Model.prepare_messages` — per-turn cross-provider translation | Default `prepare_messages` synthesizes local-shape `ToolSearch*Part` from native ones when `ToolSearchTool not in profile.supported_native_tools`; wraps non-leading system prompts as `<system>` user messages when `supports_inline_system_prompts=False` | `pydantic_ai_slim/pydantic_ai/models/__init__.py:410-435` |
| History cleanup (merge same-role messages) | `_clean_message_history` merges consecutive `ModelRequest`s (sorting `ToolReturnPart`/`RetryPromptPart` to the front) and consecutive synthetic `ModelResponse`s | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:2220-2268` |
| User-supplied history rewriting | `ProcessHistory` capability runs the user's callable on every `before_model_request` and rewrites `request_context.messages` | `pydantic_ai_slim/pydantic_ai/capabilities/process_history.py:27-69` |
| `ProcessHistory` callable contract | Sync/async, with/without `RunContext`; warns docs user about preserving trailing `ModelRequest` identity and `new_messages()` semantics | `pydantic_ai_slim/pydantic_ai/_history_processor.py:1-26`, `docs/message-history.md:509-541, 602-627` |
| ProcessHistory docs example: summarization | Doc example shows user-supplied summarizer that LLM-summarizes the oldest 10 messages | `docs/message-history.md:629-659` |
| ProcessHistory docs example: keep-last-N | Doc example shows user-supplied sliding window `messages[-5:]` | `docs/message-history.md:579-597` |
| ProcessHistory docs warning | Slicing must keep tool calls and returns paired | `docs/message-history.md:599-600, 661-662` |
| Multiple processors | Processors are applied in the order they're listed | `docs/message-history.md:718-742` |
| ProcessHistory test fixture | `received_messages` `FunctionModel` fixture to capture what the provider actually sees — proves the messages are filtered through to the wire | `tests/test_history_processor.py:36-100` |
| Anthropic provider-side compaction capability | `AnthropicCompaction` sets `anthropic_context_management`; threshold defaults to 150_000 tokens; min 50_000 | `pydantic_ai_slim/pydantic_ai/models/anthropic.py:2016-2081` |
| Anthropic compaction round-trip | `_add_compaction_params` adds the `compact-2026-01-12` beta when `CompactionPart`s are present, even without `AnthropicCompaction` active | `pydantic_ai_slim/pydantic_ai/models/anthropic.py:740-760` |
| OpenAI provider-side compaction capability | `OpenAICompaction` has stateful and stateless modes; stateless calls `/responses/compact` from `before_model_request` | `pydantic_ai_slim/pydantic_ai/models/openai.py:3995-4194` |
| Stateless OpenAI compaction trigger | `_should_compact` defaults to message-count threshold; hook replaces `request_context.messages[:-1]` with the compacted response | `pydantic_ai_slim/pydantic_ai/models/openai.py:4147-4190` |
| Stateful OpenAI compaction | `get_model_settings` injects `openai_context_management = [{'type': 'compaction'}]` and lets the server drive | `pydantic_ai_slim/pydantic_ai/models/openai.py:4128-4145` |
| OpenAI compaction result type | `_map_compaction_item` maps OpenAI `ResponseCompactionItem` to framework `CompactionPart` | `pydantic_ai_slim/pydantic_ai/models/openai.py:4261-4268` |
| `CompactionPart` schema | Provider-aware (`provider_name` required when `provider_details` set), opaque content for OpenAI, readable `content` for Anthropic | `pydantic_ai_slim/pydantic_ai/messages.py:1681-1727` |
| Compaction summary as fallback text | When the only response part is `CompactionPart`, its `content` is used as the response text (e.g. Anthropic `pause_after_compaction=True`) | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1199-1208` |
| Base model `compact_messages` (default) | Default raises `NotImplementedError("Message compaction is not supported by ...")`; only OpenAI Responses overrides | `pydantic_ai_slim/pydantic_ai/models/__init__.py:288-300` |
| Token budget enforcement | `UsageLimits` checks `request_limit` before request and token limits after response; `check_before_request` / `check_tokens` / `check_before_tool_call` raise `UsageLimitExceeded` | `pydantic_ai_slim/pydantic_ai/usage.py:379-420`, `pydantic_ai_slim/pydantic_ai/_agent_graph.py:953-960, 1022-1024` |
| Pre-request token counting | `count_tokens_before_request=True` runs `model.count_tokens` before the request and merges into `usage`; supported by Anthropic, Google, Bedrock Converse, OpenAI Responses | `pydantic_ai_slim/pydantic_ai/usage.py:283-296`, `pydantic_ai_slim/pydantic_ai/_agent_graph.py:953-958` |
| Token usage accumulation | `RunUsage.incr` sums `input_tokens`, `cache_write_tokens`, `cache_read_tokens`, audio, output, and `details` per request | `pydantic_ai_slim/pydantic_ai/usage.py:215-254` |
| Tool search (`defer_loading` → discover → reveal) | `ToolSearchToolset.get_tools` partitions tools into visible/deferred, joins `revealed_tool_names = ctx.discovered_tool_names ∪ loaded_capability_tools`; sets `defer_loading=name not in revealed_tool_names` | `pydantic_ai_slim/pydantic_ai/toolsets/_tool_search.py:284-345` |
| Tool search corpus population | `parse_discovered_tools` extracts discovered names from `ToolSearchReturnPart` / `NativeToolSearchReturnPart` typed content (and legacy `metadata['discovered_tools']` for backward compat) | `pydantic_ai_slim/pydantic_ai/toolsets/_tool_search.py:185-216` |
| Deferred capability loading | `parse_loaded_capabilities` extracts IDs from `LoadCapabilityCallPart` ↔ `LoadCapabilityReturnPart` pairs | `pydantic_ai_slim/pydantic_ai/_deferred_capabilities.py:141-154` |
| Capability / tool reflection on `RunContext` | `available_capability_ids` and `available_tool_names` properties compute the union of always-on and runtime-revealed | `pydantic_ai_slim/pydantic_ai/_run_context.py:153-218` |
| Web fetch tool truncation | `WebFetchLocalTool` truncates fetched content to `max_content_length` chars (default 50 000) and appends `[Content truncated]` | `pydantic_ai_slim/pydantic_ai/common_tools/web_fetch.py:127-128` |
| Length-finish handling (output side) | Empty response with `finish_reason='length'` raises `UnexpectedModelBehavior` with a `max_tokens` hint | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1111-1114` |
| Length-finish handling (tool side) | `check_incomplete_tool_call` raises `IncompleteToolCall` if the last response was truncated mid-tool-call | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:146-160` |
| Caching for context preservation | Anthropic settings offer `anthropic_cache_tool_definitions`, `anthropic_cache_instructions`, `anthropic_cache_messages`, `anthropic_cache` (auto); explicit `CachePoint` part | `pydantic_ai_slim/pydantic_ai/models/anthropic.py:298-348` |
| Tool search keeps `search_tools` on the wire to preserve cache | Doc explains dropping `search_tools` once everything is discovered would invalidate the request prefix on the next turn | `pydantic_ai_slim/pydantic_ai/toolsets/_tool_search.py:330-336` |
| Native tool swap rules | `Model.prepare_request`'s `_resolve_native_tool_swap` decides which tools reach the wire based on profile support (corpus vs. local vs. native) | `pydantic_ai_slim/pydantic_ai/models/__init__.py:437-489` |
| Profile supports tool search | `ModelProfile.supported_native_tools` carries the set of native tools a profile supports; default is all native tools | `pydantic_ai_slim/pydantic_ai/profiles/__init__.py:137-142` |
| `InstructionPart.sorted` for cache layout | Static instructions sorted before dynamic so providers can cache the static prefix | `pydantic_ai_slim/pydantic_ai/messages.py:1542-1545` |
| `model.prepare_messages` identity check (skip redundant cleanup) | If `prepare_messages` returns the same list (no split), the second `_clean_message_history` pass is skipped | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:945-948` |
| Conversation boundary resolution | `resolve_conversation_id` resolves `conversation_id` priority: `'new'` → explicit → most-recent-on-history → fresh UUID7 | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:94-118` |
| History carrying across runs | `message_history` arg to `Agent.run` is preserved on `GraphAgentState.message_history`; `_first_new_message_index`/`_is_same_request` enable resume-without-prompt | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:298-308, 2184-2217` |

## Answers to Dimension Questions

1. **Is context static or rebuilt each turn?**
   Neither fully. The conversation log is appended (`_agent_graph.py:854, 1025`); it is NOT rebuilt from scratch each turn. However, before each model call the loop:
   - runs `before_model_request` capabilities (`_agent_graph.py:895`), so a user-supplied `ProcessHistory` can rewrite the in-flight message list (`capabilities/process_history.py:33-40`);
   - calls `Model.prepare_messages` to normalize provider-specific shapes (`models/__init__.py:410-435`);
   - runs `_clean_message_history` to merge consecutive same-role messages (`_agent_graph.py:928, 946`);
   - refreshes the loaded-capability and discovered-tool sets from history (`_agent_graph.py:1431-1456`);
   - re-evaluates any `SystemPromptPart` carrying a `dynamic_ref` (`_agent_graph.py:416-441`);
   - rebuilds the `ToolManager` via `for_run_step`, which re-resolves deferred toolsets (`tool_manager.py:120-145`).
   So *derived context* (tools, capabilities, system prompt, model-request parameters, cleaned history) is rebuilt every turn; *raw history* grows monotonically.

2. **What gets dropped first?**
   **Nothing is dropped automatically by the framework.** The framework defines no eviction policy. A user-supplied `ProcessHistory` is the only mechanism to drop anything; the docs explicitly warn the user to keep tool calls and returns paired when slicing (`docs/message-history.md:599-600, 661-662`). `WebFetchLocalTool` truncates only the tool's own return content (`common_tools/web_fetch.py:127-128`), not history.

3. **Is summarization faithful?**
   **The framework ships no summarizer; summarization is a user pattern.** The example in `docs/message-history.md:629-659` shows how a user can LLM-summarize the oldest 10 messages and re-inject them as a single `ModelRequest`. Faithfulness is the user's responsibility: the docs warn that re-summarization can break tool-call/return pairing (`docs/message-history.md:661-662`) and that summary text replacing prior assistant output changes the model's interpretation of the conversation.
   Provider-native compaction via `AnthropicCompaction`/`OpenAICompaction` is "faithful" in the sense that the provider keeps a server-side state (Anthropic's compact beta, OpenAI's encrypted `ResponseCompactionItem`); the framework just preserves the typed `CompactionPart` (`messages.py:1681-1727`, `openai.py:4261-4268`).

4. **Are tool results preserved or summarized?**
   **Preserved verbatim by default.** All `ToolReturnPart`s stay in `message_history` indefinitely. There is no framework mechanism that summarizes tool results mid-run. To summarize or drop tool results, a user must supply a `ProcessHistory` (with the caveats above).

5. **Can the model tell what context is missing?**
   Indirectly, yes via three signals:
   - `RunUsage`/`RequestUsage` exposes `input_tokens`/`output_tokens`/cache read tokens, accessible on `RunContext.usage` (`_run_context.py:44`, `usage.py:18-44`); a `ProcessHistory` callable can use this to make a drop/summarize decision (`docs/message-history.md:612-624`).
   - `finish_reason='length'` signals that the model was truncated mid-response (`_agent_graph.py:151, 1111`); `check_incomplete_tool_call` and the empty-response path raise `IncompleteToolCall` / `UnexpectedModelBehavior` with a `max_tokens` hint, NOT a compaction hint.
   - The `ModelResponse` carries `usage` per-turn; provider errors are mapped through `on_model_request_error` (`_agent_graph.py:826, 1010-1013`), but the framework does not synthesize a "context was dropped because of size" message — there is no auto-compaction retry path.

## Architectural Decisions

1. **Append-only history, with a per-turn rewrite seam.** `GraphAgentState.message_history` is shared by reference with `capture_run_messages` and mutated via slice assignment (`_agent_graph.py:284-288, 917`) to keep both views consistent. This keeps history cheap to append but means concurrent mutation requires care (the docs call this out for `enqueue` from worker threads, `docs/message-history.md:498-507`).
2. **`ProcessHistory` is a thin wrapper over `before_model_request`** (`capabilities/process_history.py:33-44`, `docs/message-history.md:518-524`). The framework chose to make history rewriting a capability rather than a loop primitive — this keeps the core loop small and lets users compose multiple processors (`docs/message-history.md:718-742`).
3. **Deferred tools / capabilities are tracked in sets, not in `message_history` itself.** `_refresh_loaded_capability_ids` and `_refresh_discovered_tool_names` reconstruct these sets every turn from the message history (`_agent_graph.py:1431-1456`). The sets are mutated in place because `RunContext` instances created via `replace(...)` share the same set objects (`_agent_graph.py:1422-1428`). This is what enables the model to "discover more tools" mid-run and have them appear in subsequent turns.
4. **Provider-native compaction is a capability, not a built-in.** `AnthropicCompaction` (`anthropic.py:2016-2081`) and `OpenAICompaction` (`openai.py:3995-4194`) are separate, provider-specific capabilities that set `model_settings` (stateful mode) or hook `before_model_request` (stateless mode). This keeps the loop provider-agnostic while allowing provider-specific compaction state.
5. **`CompactionPart` is round-trippable and provider-aware.** Required `provider_name` ensures the framework doesn't accidentally replay compaction state from one provider to another (`messages.py:1708-1720`). `_map_compaction_item` translates OpenAI's `ResponseCompactionItem` to the framework type (`openai.py:4261-4268`).
6. **Tool search keeps `search_tools` on the wire even after everything is discovered** to preserve prompt cache (`toolsets/_tool_search.py:330-336`). This is a deliberate cache-stability vs. tool-slot-size tradeoff.
7. **`_clean_message_history` is run twice in some paths** (`_agent_graph.py:928, 946`) — first on the raw messages, second if `prepare_messages` split them. The second pass is skipped on identity (`_agent_graph.py:945-948`).
8. **Token budget is enforced at the boundary, not mid-loop.** `UsageLimits.check_before_request` runs before the call, `check_tokens` runs after the response, and `check_before_tool_call` runs before tool execution (`usage.py:379-420`). When limits are breached, the framework raises — it does not auto-compact.
9. **`count_tokens_before_request` is opt-in and provider-restricted.** Defaults to `False`; only Anthropic, Google, Bedrock Converse, and OpenAI Responses support it (`usage.py:283-296`). Other providers can't enforce pre-flight token limits.
10. **History `instructions` are joined and stored on the `ModelRequest`** (`messages.py:1538-1545`, `_agent_graph.py:877-878`), with `InstructionPart.sorted` putting static before dynamic for cache locality.

## Notable Patterns

- **Capability composition for cross-cutting concerns.** History rewriting, dynamic instructions, deferred tools, deferred capabilities, and provider compaction are all implemented as capabilities that hook into the per-turn lifecycle. The pattern keeps the core loop unchanged when a new capability is added.
- **Typed discriminated-union message parts** with discriminator tags in `_TYPED_PART_TAGS` (`_tool_search.py:407-413`, `_deferred_capabilities.py:131-138`). This allows `isinstance` checks (`isinstance(part, ToolSearchReturnPart)`) to work cross-provider after deserialization.
- **Identity-preserving slice mutations.** `ctx.state.message_history[:] = messages` (rather than reassignment) is used so that other code holding a reference to the list sees the changes (`_agent_graph.py:917`). The shared-set invariant for `loaded_capability_ids` and `discovered_tool_names` is documented explicitly (`_agent_graph.py:1422-1428`).
- **Cache stability by keeping a stable prefix.** `search_tools` is kept on the wire even after discovery is complete so Anthropic / OpenAI prompt caches stay valid (`toolsets/_tool_search.py:330-336`); `InstructionPart.sorted` keeps static instructions in the prefix (`messages.py:1542-1545`).
- **Provider-aware message normalization.** `Model.prepare_messages` is the per-model seam for translating native tool-search parts to local-shape and wrapping non-leading system prompts (`models/__init__.py:410-435`). `FallbackModel` re-runs `prepare_messages` per underlying model (`models/fallback.py:228-289`).
- **Test fixtures that capture what the provider sees.** `tests/test_history_processor.py:36-100` shows the canonical pattern: a `FunctionModel` records the messages it actually receives, so a `ProcessHistory` can be tested by snapshotting what survived.

## Tradeoffs

- **Per-turn refresh is correct but does not free context.** Re-resolving tools, capabilities, system prompts and discovering tools does not shrink the conversation. A growing `message_history` is the framework's biggest source of long-run cost.
- **`ProcessHistory` is powerful but dangerous.** It can rewrite the message list arbitrarily — including dropping messages that the model needs (tool-call/return pairing), synthesizing messages that confuse `new_messages()`, or stripping the trailing `ModelRequest` (the docs warn against each, `docs/message-history.md:526-541`).
- **`CompactionPart` round-trips opaque provider state.** Anthropic compaction is a readable summary; OpenAI compaction is opaque encrypted content. Either way, the framework can't *display* or *summarize* the compaction itself — it's a black box the provider owns.
- **Provider-native compaction is provider-locked.** Anthropic compaction requires `AnthropicCompaction`; OpenAI compaction requires `OpenAIResponsesModel`. There is no portable `Compaction` capability; for providers without native compaction, the user must supply `ProcessHistory` or use a third-party package (the docs list `summarization-pydantic-ai`, `docs/capabilities.md:1732`).
- **No automatic overflow response.** When the model returns `finish_reason='length'`, the framework raises `UnexpectedModelBehavior` with a hint about `max_tokens` (`_agent_graph.py:1111-1114`), not about compaction. A user who hasn't configured `*Compaction` or `ProcessHistory` will see the agent fail rather than auto-recover.
- **Pre-request token counting is restricted.** Most providers can't enforce `input_tokens_limit` proactively; the framework relies on the model to report `input_tokens` after the request.
- **`count_tokens` for Anthropic strips server-side tools.** `_messages_count_tokens` removes `web_search`, `code_execution`, `web_fetch`, `tool_search`, and `mcp_servers` from the count, which means `count_tokens_before_request` undercounts the real prompt (`anthropic.py:850-863`). The framework's doc-comment promises this is a temporary workaround (`anthropic.py:859`).
- **`tool_manager.tools` may be None.** When `for_run_step` hasn't run yet (the resume-without-prompt path), `available_tool_names` falls back to `discovered_tool_names` (`_run_context.py:195-196`). This is a graceful degradation but means a `ProcessHistory` callable that runs before tool resolution has a limited view.

## Failure Modes / Edge Cases

1. **Token limit hit mid-response.** `ModelResponse.finish_reason='length'` → if the response is empty, `UnexpectedModelBehavior` with `max_tokens` hint (`_agent_graph.py:1111-1114`); if the response contains a `ToolCallPart` whose args don't parse, `IncompleteToolCall` with the same hint (`_agent_graph.py:146-160`).
2. **Tool-call/return desync from history rewriting.** A user `ProcessHistory` that drops a `ToolCallPart` without dropping the matching `ToolReturnPart` (or vice versa) will produce provider errors on the next request (`docs/message-history.md:599-600, 661-662`).
3. **`new_messages()` semantics under history rewriting.** Rebuilding the trailing `ModelRequest` strips it from `new_messages()`, which breaks downstream accounting unless the processor preserves `parts`/`timestamp`/`instructions`/`metadata` or sets `run_id=ctx.run_id` (`docs/message-history.md:536-541`).
4. **Cross-provider tool-search replay.** When the next turn runs on a provider without native tool-search support, `prepare_messages` synthesizes local-shape parts from native history (`models/__init__.py:427-433`); if a history is replayed without that translation, the model would see provider-specific wire shapes it doesn't understand.
5. **Compaction data leaked across providers.** `CompactionPart.provider_name` is required when `provider_details` is set (`messages.py:1708-1712`); the framework does NOT enforce this at runtime — a user that round-trips a compaction item across providers can produce malformed history.
6. **Legacy `discovered_tools` metadata.** Pre-typed-content histories carry discovery on `metadata['discovered_tools']`; `parse_discovered_tools` validates against a TypedDict and falls back gracefully (`toolsets/_tool_search.py:204-209, 224-230`).
7. **`Agent.history_processors` deprecated.** Old-style processors are re-routed through `consume_deprecated_history_processors_as_capabilities` (`agent/__init__.py:421-431`, `_utils.py:923-955`). Users on the legacy path get a deprecation warning, not a migration guide.
8. **OpenAI compaction with ZDR.** Stateless mode is required when `openai_store=False`; if a user picks stateful mode under ZDR, compaction state will not be retained (`openai.py:4014-4019`).
9. **`for_run_step` no-op on same step.** `ToolManager.for_run_step` returns the same instance if `ctx.run_step == self.ctx.run_step` (`tool_manager.py:122-124`). The doc-comment on `UserPromptNode` resume path notes that calling `for_run_step` twice for the same step is fine (`_agent_graph.py:870-871`).
10. **`enqueue` from cross-thread enqueueer.** The queue isn't atomic against cross-thread appends; the docs warn to marshal via `loop.call_soon_threadsafe` (`docs/message-history.md:498-507`).

## Future Considerations

- A first-party `ProcessHistory` "compactor" capability that uses an LLM to summarize old tool calls and returns. The framework provides the seam (`ProcessHistory` + `capabilities/process_history.py`) and the example pattern (`docs/message-history.md:629-659`) but ships no built-in implementation. The third-party `summarization-pydantic-ai` package listed in `docs/capabilities.md:1732` covers `ContextManagerCapability`, `SummarizationCapability`, `SlidingWindowCapability`, `LimitWarnerCapability` — a strong signal of user demand.
- A portable `Compaction` capability that abstracts over Anthropic / OpenAI / Bedrock / Gemini, so users don't have to pick a provider-specific one. Today the framework only ships `AnthropicCompaction` and `OpenAICompaction`.
- An automatic overflow response: when the model returns `finish_reason='length'` on a `ModelRequest` whose `instructions` includes a system-prompt hint about context size, run a configured compaction strategy and retry. Today the framework only raises.
- A `count_tokens` adapter for more providers. Currently Anthropic, Google, Bedrock Converse, OpenAI Responses (`usage.py:283-296`); other providers can't enforce `input_tokens_limit` proactively.
- An `enforce tool-call/return pairing` helper for `ProcessHistory` users. The doc warnings (`docs/message-history.md:599-600, 661-662`) are the only thing standing between users and broken state.
- A `get_messages()` accessor on `RunContext`/`GraphAgentState` that returns the *post-ProcessHistory* messages for the current turn, so tools can introspect what the model will actually see.
- A `MessageHistory` slice primitive: e.g. `drop_older_than(tokens=N, keep_recent=...)` to make sliding-window policies ergonomic without writing a full `ProcessHistory`.

## Questions / Gaps

1. **Is there a documented guarantee about what survives across `Agent.iter` resumptions?** The code path `_first_new_message_index` + `_is_same_request` (`_agent_graph.py:2184-2217`) supports "resume-without-prompt," but the public contract for what survives a `UserPromptNode` reentry (e.g., dynamically loaded capabilities, discovered tools, in-progress `pending_messages`) is implicit.
2. **What happens if a `ProcessHistory` returns an empty list?** The code path at `_agent_graph.py:905-909` requires `len(messages) == 0` to raise `UserError`, but a `ProcessHistory` returning `[]` would also pass `if len(messages) == 0`. Worth confirming via test (`tests/test_history_processor.py`).
3. **How are `CompactionPart`s handled on a non-supporting adapter?** The base `compact_messages` raises `NotImplementedError` (`models/__init__.py:288-300`), but the loop's `_finish_handling` does not check for `CompactionPart` presence before dispatch — adapters are expected to map or skip independently.
4. **Is there a way to mark specific messages as "do not summarize" (e.g., user instruction parts)?** Not visible in the source. A user who wants to preserve certain messages through a `ProcessHistory` rewrite has to write that logic themselves.
5. **Does the framework emit a warning when `message_history` grows past a heuristic size?** No evidence found in the source. The framework only emits a warning on `len(retries) == max_retries` (`tool_manager.py:177-181`) and on usage-limit breach (`usage.py:379-420`).
6. **How is the conversation's "effective system prompt" rebuilt after compaction?** With provider-native compaction, the system prompt comes back as part of the compaction; with a user `ProcessHistory`, the user is responsible for re-injecting it. `ReinjectSystemPrompt` is a separate capability (`docs/capabilities.md` index, mentioned in `docs/message-history.md:151`) that re-adds the agent's configured `system_prompt` if the history doesn't carry one — useful when history comes from a compaction pipeline or external store.

---

Generated by `03.07-context-refresh-inside-the-loop` against `pydantic-ai`.
