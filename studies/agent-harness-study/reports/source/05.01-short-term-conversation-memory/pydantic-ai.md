# Source Analysis: pydantic-ai

## Dimension 05.01: Short-Term Conversation Memory

### Source Info

| Field | Value |
|-------|-------|
| Name | pydantic-ai |
| Path | `studies/agent-harness-study/sources/pydantic-ai` |
| Language / Stack | Python (pydantic v2, pydantic-graph, httpx; provider SDKs optional) |
| Analyzed | 2026-08-25 |

Citation note: paths below are workspace-relative to `studies/agent-harness-study/sources/pydantic-ai/`.

## Summary

Pydantic AI implements short-term conversation memory as an **explicit, caller-owned list of typed messages** (`ModelMessage = ModelRequest | ModelResponse`, `pydantic_ai_slim/pydantic_ai/messages.py:2764`). There is no built-in session store or daemon-side conversation database: the library is deliberately stateless across runs, and the documented contract is that the caller persists the history and passes it back via the `message_history` parameter of `Agent.run(...)` (`pydantic_ai_slim/pydantic_ai/agent/__init__.py:1138`; design statement at `docs/message-history.md:406`). Within a single run, history lives in `GraphAgentState.message_history` (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:298-302`) and grows by appending each new `ModelRequest` before the model call and each `ModelResponse` after it (`_agent_graph.py:1469`, `_agent_graph.py:1747-1795`).

By default the model sees **the full history, unwindowed and unsummarized**, plus a freshly built request. Selection is all-or-nothing with three sanctioned reduction mechanisms:

1. **User-supplied history processors** — `ProcessHistory` capabilities run a sync/async function over the whole message list before each model request and *replace* history in run state (`pydantic_ai_slim/pydantic_ai/capabilities/process_history.py:31-38`, applied at `_agent_graph.py:1515-1523`). Sliding-window and LLM-summarization are documented recipes on this hook, not built-ins (`docs/message-history.md:760-845`).
2. **Provider-native compaction** — `OpenAICompaction` (`models/openai.py:4716`) and `AnthropicCompaction` (`models/anthropic.py:2681`) configure server-side compaction; the resulting `CompactionPart` acts as a visibility boundary that adapters enforce when rendering requests (`models/__init__.py:2072-2131`) and that framework state derivation honors via `post_compaction_window` (`messages.py:2774-2814`).
3. **Provider-validity cleanup** — `_clean_message_history` (`_agent_graph.py:2960-2986`) runs before every request to drop orphaned tool results, synthesize returns for dangling tool calls, and merge consecutive same-role messages.

Roles are exactly two message types mapped onto user/assistant sides: `ModelRequest` carries user prompts, tool returns, retry prompts, system-prompt parts and instructions (`messages.py:1832-1870`); `ModelResponse` carries text, thinking, tool calls and compaction items (`messages.py:2539` onward). Tool exchanges are first-class typed parts (`ToolCallPart`, `messages.py:2276`; `ToolReturnPart`, `messages.py:1572`) and are retained in full. Every message is pydantic-serializable through `ModelMessagesTypeAdapter` (`messages.py:2768-2771`), which is the de facto persistence format for resuming conversations.

The model is well-tested (dedicated suites for processors, sanitization, cache-prefix stability), observable (OTel `gen_ai.conversation.id`, per-run `run_id` stamping), and hardened against failure (cancelled/crashed runs yield resumable snapshots; broken tool-call pairing is repaired automatically).

**Rating: 9 / 10**

Rationale per rubric: this is a mature, durable, observable and extensible model proven under failure. The one point short of 10: there is no first-party durable conversation store or built-in windowing/summarization strategy in the core library — these are delegated to callers, provider APIs, or the separate "Pydantic AI Harness" package (`docs/capabilities/compaction.md:35-37`) — which is a stated design choice but means out-of-the-box memory management beyond a single process is DIY.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| In-run storage | `GraphAgentState.message_history: list[ModelMessage]` mutated by reference during the graph run | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:298-302` |
| History entry point | `message_history` parameter on every run method; seeds graph state | `pydantic_ai_slim/pydantic_ai/agent/__init__.py:1138`, `agent/__init__.py:1481` |
| Message roles | `ModelRequest` (user side) vs `ModelResponse` (assistant side); discriminated union `ModelMessage` | `pydantic_ai_slim/pydantic_ai/messages.py:1832-1849`, `messages.py:2539`, `messages.py:2764` |
| Request assembly | New `ModelRequest` appended to state before each call; empty-history guard | `_agent_graph.py:1466-1499` |
| System prompt policy | Standing system prompts only generated when history is empty (`if not messages: parts.extend(await self._sys_parts(...))`) | `_agent_graph.py:646-654` |
| Instructions re-sent per request | Instructions joined from agent + toolsets and set on each outgoing request, never stored as history | `_agent_graph.py:1492-1495`; `InstructionPart` at `messages.py:1752` |
| Resume without new prompt | Trailing `ModelRequest` popped and reused; flagged `is_resuming_without_prompt` so it stays "prior context" for `new_messages()` | `_agent_graph.py:563-587`, `_agent_graph.py:1534-1542` |
| History processor interface | `HistoryProcessor` union type (sync/async × with/without `RunContext`); `ProcessHistory.before_model_request` replaces `request_context.messages` | `pydantic_ai_slim/pydantic_ai/_history_processor.py:11-26`; `capabilities/process_history.py:31-38` |
| Processor application site | `root_capability.before_model_request(...)` invoked with `ctx.state.message_history[:]` copy; result written back via slice assignment | `_agent_graph.py:1505-1552` |
| Post-processor validation | Errors if processed history is empty or doesn't end with `ModelRequest` | `_agent_graph.py:1525-1529`; tests at `tests/test_history_processor.py:973-992` |
| Provider-validity pipeline | `_clean_message_history` = drop orphaned results → repair dangling calls → merge consecutive same-role messages | `_agent_graph.py:2960-2986` |
| Orphaned tool-result drop | Tool returns whose `tool_call_id` has no preceding call are removed (providers reject them) | `_agent_graph.py:2771-2813` |
| Dangling-call repair | Synthesized `ToolReturnPart(outcome='interrupted')` with `SYNTHESIZED_TOOL_RETURN_METADATA_KEY` marker inserted for unanswered calls; deterministic/idempotent to protect prompt caches | `_agent_graph.py:2816-2898`, marker at `_agent_graph.py:2702` |
| Same-role merge | Adjacent `ModelRequest`s merged (tool results hoisted ahead of user parts); adjacent synthetic responses merged only when not provider-originated | `_agent_graph.py:2901-2957` |
| Adapter-level role conversion | `Model.prepare_messages` translates cross-provider parts, wraps mid-conversation `SystemPromptPart`s as `<system>`-tagged user text when profile lacks inline support | `pydantic_ai_slim/pydantic_ai/models/__init__.py:690-783` |
| Standing vs mid-conversation system prompt | Position-based rule `_standing_system_prompt_count`; only leading parts of the first request hoist to the top-level system parameter (cache-aware rationale) | `models/__init__.py:2052-2069`, wrapper at `models/__init__.py:2162-2195` |
| Compaction boundary semantics | `post_compaction_window` returns messages from latest `CompactionPart` onward; part-level precision inside the carrying response | `messages.py:2774-2814` |
| Wire-level compaction trim | `_trim_messages_before_compaction` drops pre-boundary history, re-inserts standing prompt; adapter-declared flags (`compaction_requires_encrypted_content`, `compaction_retains_standing_prompt`) | `models/__init__.py:2072-2131`; declarations referenced in `models/AGENTS.md:46-49` |
| OpenAI compaction capability | Stateful server-side `context_management` (token threshold) or stateless `/responses/compact` endpoint (message-count threshold or custom trigger) | `models/openai.py:4716-4835` |
| Anthropic compaction capability | `anthropic_context_management` edit with `input_tokens` trigger (default 150k), custom summarization instructions, optional pause | `models/anthropic.py:2681-2746` |
| Untrusted-history sanitization | `sanitize_messages` strips client system prompts, non-HTTP file URLs, unresolved tail tool calls, compaction provenance; optional `strip_compaction_parts` when mixing with trusted server history | `messages.py:2953-3026` |
| Result API boundary | `all_messages()` vs `new_messages()` sliced at `new_message_index` computed by `_first_new_message_index` | `result.py:527-574`; `_agent_graph.py:2644-2684` |
| Cancellation durability | `RunCancelled.all_messages()` returns detached resumable snapshot; dangling calls auto-closed with synthesized interrupted returns | `exceptions.py:281-286`, `exceptions.py:364-393` |
| Run/conversation identity | `fill_run_metadata` stamps `timestamp`/`run_id`/`conversation_id`; `resolve_conversation_id` inherits most recent id from history; `'new'` forks | `_utils.py:560-569`; `_agent_graph.py:232-261` |
| `run_id` uniqueness guard | Reusing a `run_id` present in `message_history` raises `UserError` (protects `new_messages()` boundaries) | `_agent_graph.py:264-295` |
| Token-limit awareness | Optional pre-request token counting gated by `UsageLimits.count_tokens_before_request` with per-request input-token checks | `_agent_graph.py:1603-1621` |
| Truncation detection | `check_incomplete_tool_call` raises `IncompleteToolCall` when a `finish_reason='length'` response cuts a tool call's args mid-stream | `_agent_graph.py:345-359` |
| Tests: processors | Full behavioral suite: replacement semantics, ordering, pruning vs `new_messages()`, callable-class and async variants | `tests/test_history_processor.py:75-122`, `tests/test_history_processor.py:1101-1248` |
| Tests: sanitization & wire | Dedicated suites for `sanitize_messages` and cache-prefix stability across consecutive requests | `tests/test_sanitize_messages.py`; `tests/test_cache_prefix_stability.py` |
| Reference chat loop | CLI keeps a plain in-process `list[ModelMessage]`, passing `all_messages()` back each turn — the canonical memory pattern | `pydantic_ai_slim/pydantic_ai/_cli/__init__.py:359`, `382-394`, `438` |

## Answers to Dimension Questions

**1. What conversation history does the model see?**
Everything in the caller-supplied `message_history` plus the newly appended `ModelRequest` — no automatic truncation. Concretely: `UserPromptNode.run` copies `state.message_history` into the captured-messages list (`_agent_graph.py:537-541`); `ModelRequestNode._prepare_request` appends the new request (`_agent_graph.py:1469`), passes `ctx.state.message_history[:]` to capability hooks (`_agent_graph.py:1505-1518`), then cleans and prepares the final wire list (`_agent_graph.py:1568-1598`). Standing system prompts are injected only into the *first* request of an empty history (`_agent_graph.py:646-654`); instructions are re-evaluated and re-sent on every request (`_agent_graph.py:1492-1495`), so directive context does not depend on history round-tripping.

**2. What gets dropped?**
Only what providers would reject, and only via explicit mechanisms:
- Orphaned tool results (result without call) are removed (`_agent_graph.py:2771-2813`).
- Dangling calls get synthesized `'interrupted'` returns rather than being dropped (`_agent_graph.py:2876-2887`).
- Client-supplied unsafe content (system prompts, non-HTTP file URLs, uploaded files, unresolved tail tool calls) is dropped by `sanitize_messages` (`messages.py:2969-3005`).
- Pre-compaction-boundary history is hidden from the model by adapters honoring `CompactionPart`s (`models/__init__.py:2113-2131`).
- Native/builtin tool parts are deliberately exempt from all dropping (`_agent_graph.py:2781-2785`).
There is **no built-in sliding window or age-based eviction**; those exist only as user-written `ProcessHistory` functions (`docs/message-history.md:764-781`) or external packages (`docs/capabilities/third-party.md:17`).

**3. Are tool messages retained?**
Yes, as first-class typed parts, not stringified blobs. Calls live in `ModelResponse.parts` (`ToolCallPart`, `messages.py:2276`); returns live in `ModelRequest.parts` (`ToolReturnPart`, `messages.py:1572`; retry feedback as tool-bound `RetryPromptPart`, `messages.py:1637`). The cleanup pass exists specifically because providers require exact call/result pairing across message boundaries (`_agent_graph.py:2974-2981`), and tests pin parallel-tool-call merging behavior (`_agent_graph.py:1561-1563` referencing `tests/test_tools.py::test_parallel_tool_return_with_deferred`). Deferred/human-in-the-loop tools keep their calls pending in history until results arrive (`_agent_graph.py:543-544`, `_agent_graph.py:687-697`).

**4. Is memory per user/thread/session?**
Not enforced — scoped by convention. The framework provides identity primitives instead of storage: every message is stamped with `run_id` (unique per run, never inherited) and `conversation_id` (inherited from history, fresh UUID7 otherwise) (`_utils.py:560-569`; `_agent_graph.py:237-261`); `conversation_id` is emitted as OTel `gen_ai.conversation.id` (`messages.py:1854-1859`). UI adapters assume the client transmits the entire history per request, with an explicit trust warning and a recommended server-side persistence pattern keyed by thread (`docs/ui/overview.md:128-143`). The bundled CLI demonstrates the minimal pattern: one in-memory list reused across turns (`_cli/__init__.py:359-394`).

**5. Can history be edited or forked?**
Yes, both, at several levels:
- **Edit**: `ProcessHistory` processors receive the full list and their return value *replaces* run-state history (`capabilities/process_history.py:36`, `_agent_graph.py:1552`); docs warn processors must copy if they want to preserve originals (`docs/message-history.md:701-703`). Dynamic `SystemPromptPart`s with `dynamic_ref` are re-evaluated in stored history each run (`_agent_graph.py:707-732`). The last message's output-tool return content can be rewritten via `all_messages(output_tool_return_content=...)` (`result.py:527-542`).
- **Fork**: pass `conversation_id='new'` to branch off supplied history under a fresh UUID7 (`_agent_graph.py:245-254`, `docs/message-history.md:300`). Because histories are plain lists, re-running any suffix against a different agent is routine (cross-agent reuse documented at `docs/message-history.md:505-551`).

## Architectural Decisions

1. **Stateless core, explicit history hand-off.** Runs are reconstructed from `message_history` rather than held server-side (`docs/message-history.md:406`); durable-execution integrations checkpoint the same list (`docs/durable_execution/temporal.md:318`). This trades convenience for transparency and composability.
2. **Two-role message algebra with typed parts.** Everything is `ModelRequest`/`ModelResponse` with discriminated part kinds (`messages.py:2443-2537`), enabling lossless serialization (`ModelMessagesTypeAdapter`, `messages.py:2768`) and exhaustive adapter mapping.
3. **Repair-at-the-wire, don't-police-the-caller.** Rather than validating hand-built histories up front, `_clean_message_history` normalizes whatever arrives just before sending ("massage the history however we can... drop only what's fundamentally unsendable", `_agent_graph.py:2963-2982`).
4. **Selection as a capability, not a parameter.** Windowing/summarization hooks into the generic `before_model_request` lifecycle via `ProcessHistory` (`capabilities/process_history.py:26-38`), keeping core free of context-management policy while remaining composable and ordered (`tests/test_history_processor.py:318-406` verifies sequencing).
5. **Compaction as a portable protocol part.** `CompactionPart` makes provider-side summarization inspectable and provider-agnostic at the state layer (`post_compaction_window`, `messages.py:2782-2800`), with conservative intersection semantics across `FallbackModel` failover.
6. **Identity stamping over session objects.** `run_id`/`conversation_id` on each message enable `new_messages()` boundary detection and trace correlation without a session abstraction (`_agent_graph.py:2644-2684`, `docs/message-history.md:269-273`).

## Notable Patterns

- **Slice-assignment write-back**: `ctx.state.message_history[:] = messages` keeps the captured-messages list and run state aliased so `capture_run_messages()` observes processed history (`_agent_graph.py:1551-1552`, capture API at `_agent_graph.py:2523-2563`).
- **Layered fallback matching** for locating the resumed request after arbitrary processor mutations — identity → value equality → pinned index → `run_id` scan — each documented with its specific blind spot (`_agent_graph.py:2651-2684`).
- **Cache-prefix discipline**: repair passes are deterministic and idempotent so they "never churn provider prompt-cache prefixes" (`_agent_graph.py:2844-2846`), and system-prompt hoisting avoids rewriting the first cache section (`models/__init__.py:2059-2062`); regression-tested by `tests/test_cache_prefix_stability.py`.
- **Docs-as-contracts**: docstrings carry operational caveats (e.g., processor warnings about preserving `ToolAvailabilityDeltaPart`s, `docs/message-history.md:705-709`) that map directly to enforcement points in code (`_agent_graph.py:1570-1577` derives reveal state post-processing).

## Tradeoffs

- **Full-history default**: correct and simple, but token cost grows linearly until the user adds a processor or compaction; the library mitigates (token counting gates, `_agent_graph.py:1603-1621`) but does not solve it.
- **Caller-managed persistence**: maximal flexibility, but multi-turn deployments must build storage themselves; the trust burden is explicit (`sanitize_messages` exists precisely because history is an attack surface, `messages.py:2963-2968`, `docs/message-history.md:383-406`).
- **Replace-semantics processors** are powerful but foot-gunny: a careless processor can drop pairing evidence or shift `new_message_index`; the code compensates with layered resumed-request detection (`_agent_graph.py:2651-2670`) and documents residual failure combinations (`_agent_graph.py:2666-2670`).
- **Provider-specific compaction semantics** leak complexity: encrypted-vs-plaintext payload rules and standing-prompt retention differ per adapter, handled by declared flags rather than a unified mode (acknowledged open item #7255 in `messages.py:2839`).

## Failure Modes / Edge Cases

- **Crash/cancel mid-tool-execution**: trailing interrupted request closed out with synthesized returns on next run (`_agent_graph.py:546-556`); `RunCancelled` exposes a resumable snapshot (`exceptions.py:281-286`).
- **Token-limit truncation mid-tool-call**: detected and raised as `IncompleteToolCall` instead of shipping unparsable args (`_agent_graph.py:345-359`).
- **Hand-built or adapter-round-tripped histories**: orphaned results dropped, misplaced results not silently reordered (`_agent_graph.py:2786-2792`).
- **Suspended provider turns** (Anthropic `pause_turn`, OpenAI background): history ending in a suspended `ModelResponse` resumes instead of accepting a new prompt; a new prompt on such history raises `UserError` (`_agent_graph.py:589-600`, `619-626`).
- **Unprocessed tool calls + new prompt**: rejected outright unless the response was interrupted (`_agent_graph.py:627-636`).
- **Client-fabricated history**: sanitized by default in UI adapters; server-side mixing requires `strip_compaction_parts=True` to prevent clients hiding trusted history (`messages.py:3000-3005`, `docs/capabilities/compaction.md:20-26`).
- **Known residual gap**: a processor that simultaneously rebuilds and shifts the resumed request defeats all fallback layers except `run_id` inclusion (`_agent_graph.py:2666-2670`) — documented, not guarded.

## Future Considerations

- A declared per-model compaction mode would unify the execution-availability gate with adapter render conditions (`messages.py:2835-2839`, issue #7255 referenced).
- Realtime sessions do not run history-processing capabilities before seeding; preprocessing remains caller-side today (`docs/realtime/capabilities.md:63`, known issue #7299).
- Bundled tiered compaction strategies live in the separate Pydantic AI Harness distribution (`docs/capabilities/compaction.md:35-37`); upstreaming a minimal strategy could reduce ecosystem fragmentation.

## Questions / Gaps

- **No first-party session store**: search found no SQLite/Redis/file-based conversation store in `pydantic_ai_slim` (searched `sources/pydantic-ai/pydantic_ai_slim` for session/persistence modules; persistence exists only as durable-execution checkpointing in `pydantic_graph`). Intentional per docs, but teams wanting plug-and-play memory must look outside the repo.
- **Summarization is example-grade**: the documented LLM-summarization processor (`docs/message-history.md:816-845`) is prose guidance, not shipped code; no built-in summary-marker message type other than `CompactionPart` (provider-produced). No evidence of a first-party summarization capability was found in `pydantic_ai_slim/pydantic_ai/capabilities/`.
- **Window sizing heuristics absent**: no token-budget-aware selector ships in-core; users combine `UsageLimits.count_tokens_before_request` (`_agent_graph.py:1603`) with hand-written processors.
- Per the isolation rules, sibling sources and generated reports were not consulted; findings reflect only `studies/agent-harness-study/sources/pydantic-ai`.

---

Generated by `Dimension 05.01: Short-Term Conversation Memory` against `pydantic-ai`.
