# Source Analysis: openai-agents-sdk

## Dimension 11.01: Context Selection Policy

### Source Info

| Field | Value |
|-------|-------|
| Name | openai-agents-sdk |
| Path | `studies/agent-harness-study/sources/openai-agents-sdk` |
| Language / Stack | Python (pydantic, asyncio, OpenAI Responses/Chat Completions APIs) |
| Analyzed | 2026-08-25 |

All citations below are relative to the source root `studies/agent-harness-study/sources/openai-agents-sdk/`.

## Summary

The OpenAI Agents SDK does not have a single "context selector" object; instead it exposes a deterministic, layered assembly pipeline with explicit override hooks at each stage. Model context is rebuilt from scratch on every model call: session history is fetched (optionally bounded by `SessionSettings.limit`), merged with the new turn input (optionally rewritten by a user-supplied `session_input_callback`), normalized and sanitized (`drop_orphan_function_calls`, `strip_internal_input_item_metadata`, dedupe-preferring-latest), combined with the per-agent system prompt and prompt-template config, and finally passed through the `RunConfig.call_model_input_filter` hook immediately before the model call (`src/agents/run_internal/run_loop.py:2097-2152`). Handoff boundaries add a second selection layer: `Handoff.input_filter` and the opt-in `nest_handoff_history` compaction decide how much of the prior transcript the next agent sees (`src/agents/handoffs/__init__.py:158-172`, `src/agents/handoffs/history.py:83-157`). The SDK ships concrete policy implementations — `ToolOutputTrimmer` (sliding-window trimming of old tool outputs, `src/agents/extensions/tool_output_trimmer.py:87-202`) and `OpenAIResponsesCompactionSession` (threshold-triggered server-side compaction, `src/agents/memory/openai_responses_compaction_session.py:28-60`). Tool surface itself is selected dynamically per run via enabled-checks and MCP allow/block/dynamic filters (`src/agents/agent.py:272-292`, `src/agents/mcp/server.py:983-1019`). Sensitive-data policy is explicit but scoped to observability channels (traces/logs), not to model input content: traces redact errors when `trace_include_sensitive_data=False` (`src/agents/util/_error_tracing.py:46-53`), logs suppress model/tool data by default (`src/agents/_debug.py:12-27`), and output guardrails replace rejected tool outputs with a data-free placeholder before replay/persistence (`src/agents/run_internal/blocked_output.py:40`).

## Rating

**8 / 10.**

Rationale against the rubric:

- The selection pipeline is explicit, typed, and documented at every stage: `ModelInputData`/`CallModelData` (`src/agents/run_config.py:59-76`), `CallModelInputFilter` (`src/agents/run_config.py:438-446`), `SessionInputCallback` (`src/agents/run_config.py:431-436`), `HandoffInputFilter` (`src/agents/handoffs/__init__.py:118-119`), `SessionSettings.limit` (`src/agents/memory/session_settings.py:30-39`).
- Behavior is heavily tested: end-to-end filter tests (`tests/test_call_model_input_filter.py:18-163`), handoff-filter tests (`tests/test_extension_filters.py:215-461`), trimmer unit tests (`tests/extensions/test_tool_output_trimmer.py:61-383`), and session-limit tests (`tests/memory/test_session_limit.py`).
- Operational safeguards exist: orphan tool-call pruning prevents API-rejected payloads (`src/agents/run_internal/items.py:171-271`), dedupe keeps latest tool outputs (`src/agents/run_internal/items.py:768-800`), internal metadata never reaches the wire (`src/agents/run_internal/items.py:715-723`), and retry rewind restores session tails atomically (`src/agents/run_internal/session_persistence.py:978-1040`).
- It falls short of 9-10 because: (a) there is no built-in token-budget-aware or relevance-based selector — everything is item-count or character-count based and relies on opt-in hooks; (b) no content-level sensitive-field redaction applies to what enters model context (only to traces/logs); (c) inclusion rationale is not surfaced to users — provenance tracking for nested handoff history exists but is private machinery (`src/agents/run_internal/items.py:93-128`).

## Evidence Collected

Every entry includes a file path with line numbers relative to the source root.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Per-turn context assembly entry point | Runner gathers system prompt + prompt config, resolves tools/handoffs, then assembles input items for the model call | `src/agents/run_internal/run_loop.py:2097-2150` |
| Turn item builder | `_prepare_turn_input_items` converts caller input + generated items into model input | `src/agents/run_internal/run_loop.py:334-341` |
| Input normalization + orphan pruning | `prepare_model_input_items` normalizes caller items and prunes orphaned calls only from runner-generated history | `src/agents/run_internal/items.py:331-342` |
| Approval items excluded from model input | `run_item_to_input_item` returns `None` for `tool_approval_item`; reasoning IDs optionally stripped by policy | `src/agents/run_internal/items.py:139-168` |
| Orphan call pruning rules | `drop_orphan_function_calls` drops unmatched tool/program calls plus preceding dangling reasoning items | `src/agents/run_internal/items.py:171-304` |
| Internal metadata stripped before send | `strip_internal_input_item_metadata` removes SDK-only session keys before model submission | `src/agents/run_internal/items.py:715-723` |
| Dedupe preferring latest outputs | `deduplicate_input_items_preferring_latest` keeps newest value per stable key, anchors causal precursors earliest | `src/agents/run_internal/items.py:768-800` |
| Pre-model-call user hook | `maybe_filter_model_input` invokes `RunConfig.call_model_input_filter` and validates return type | `src/agents/run_internal/turn_preparation.py:51-93` |
| Hook contract | `call_model_input_filter` documented as "edit the input sent to the model e.g. to stay within a token limit" | `src/agents/run_config.py:438-446` |
| Filter applied then re-deduped | After filtering, input is deduped again; server-managed conversations validate filtered input | `src/agents/run_internal/run_loop.py:2144-2162` |
| Session history fetch with limit | `prepare_input_with_session` reads history honoring `SessionSettings.limit` and merges with new input | `src/agents/run_internal/session_persistence.py:317-357` |
| Custom history merge callback | `session_input_callback` may reorder/drop/duplicate; persistence diffing avoids re-saving old history | `src/agents/run_internal/session_persistence.py:377-458` |
| Session read sanitization | History items are format-normalized, metadata-stripped, and reasoning-ID-policy-applied on read | `src/agents/run_internal/session_persistence.py:359-368` |
| Session limit setting | `SessionSettings.limit`: "Maximum number of items to retrieve. If None, retrieves all items." | `src/agents/memory/session_settings.py:30-39` |
| SQLite limit semantics | `get_items` returns latest N in chronological order; corrupt-row window expansion keeps limit accurate | `src/agents/memory/sqlite_session.py:288-358` |
| Session protocol | `Session.get_items/add_items/pop_item/clear_session` define pluggable history storage | `src/agents/memory/session.py:16-56` |
| Server-managed conversation mode | When `conversation_id`/`previous_response_id` set, history stays server-side and local history is excluded from prepared input | `src/agents/run.py:652-668` |
| Dynamic instructions | `Agent.get_system_prompt` supports static string or `(context, agent)` callable evaluated every run | `src/agents/agent.py:1042-1071` |
| Prompt template config | `Agent.get_prompt` resolves `ResponsePromptParam` (server-side prompt template) per run | `src/agents/agent.py:1073-1083` |
| Tool-surface selection | `get_all_tools` filters tools by dynamic `is_enabled(run_context, agent)` and prunes orphaned search tools | `src/agents/agent.py:272-292` |
| MCP tool filtering (static) | Allowlist/blocklist filter applied when listing MCP tools that become model-visible tools | `src/agents/mcp/server.py:1003-1019` |
| MCP tool filtering (dynamic) | Callable filters receive `ToolFilterContext(run_context, agent, server_name)` per tool | `src/agents/mcp/server.py:1021-1052` |
| MCP filter factory | `create_static_tool_filter(allowed_tool_names, blocked_tool_names)` convenience API | `src/agents/mcp/util.py:213-237` |
| Handoff context selection hook | `Handoff.input_filter` decides "the inputs that are passed to the next agent"; default forwards entire history | `src/agents/handoffs/__init__.py:158-172` |
| Handoff input data shape | `HandoffInputData.input_history/pre_handoff_items/new_items/input_items` partition what the next agent sees vs. session history | `src/agents/handoffs/__init__.py:71-99` |
| Built-in handoff filter | `remove_all_tools` strips all tool call/output/MCP/reasoning items from forwarded history | `src/agents/extensions/handoff_filters.py:33-118` |
| Nested handoff summary | `nest_handoff_history` compresses prior transcript into ordered assistant summary segments, preserving verbatim message items with digest-based provenance | `src/agents/handoffs/history.py:83-157` |
| Summary-only item types | Function calls/outputs and reasoning are summarized, not forwarded verbatim (`_SUMMARY_ONLY_INPUT_TYPES`) | `src/agents/handoffs/history.py:42-49` |
| Transcript summarizer | `default_handoff_history_mapper` renders one numbered assistant message wrapped in `<CONVERSATION HISTORY>` markers | `src/agents/handoffs/history.py:311-398` |
| Run-level nesting config | `RunConfig.nest_handoff_history` (opt-in beta) and `handoff_history_mapper` override | `src/agents/run_config.py:374-389` |
| Built-in trimmer policy | `ToolOutputTrimmer` replaces large old tool outputs with bounded previews; recent-N user messages exempt | `src/agents/extensions/tool_output_trimmer.py:87-202` |
| Trimmer boundary rule | `_find_recent_boundary` walks backward counting user messages; fewer than N ⇒ nothing trimmed | `src/agents/extensions/tool_output_trimmer.py:204-220` |
| Compaction trigger | Default triggers `responses.compact` at ≥10 candidate items; user messages and prior compaction items excluded as candidates | `src/agents/memory/openai_responses_compaction_session.py:28-60` |
| Reasoning ID policy | `RunConfig.reasoning_item_id_policy` of `"omit"` strips `rs_...` IDs from built model input (read-side too) | `src/agents/run_config.py:459-464`; `src/agents/run_internal/session_persistence.py:362-368` |
| Trace sensitive-data switch | `trace_include_sensitive_data` gates inputs/outputs in traces; env-default `OPENAI_AGENTS_TRACE_INCLUDE_SENSITIVE_DATA=true` | `src/agents/run_config.py:404-410,53-56` |
| Error redaction for traces | `get_trace_error` returns `"Error details are redacted."` unless sensitive data allowed | `src/agents/util/_error_tracing.py:46-53` |
| Log redaction default-on | `DONT_LOG_MODEL_DATA`/`DONT_LOG_TOOL_DATA` default True so LLM/tool payloads stay out of logs | `src/agents/_debug.py:12-27` |
| Guardrail-blocked output replacement | Rejected terminal tool outputs replaced by constant `"Output withheld by an output guardrail."` payload across replay/persistence | `src/agents/run_internal/blocked_output.py:40`; allowlisted-field rebuild at `src/agents/run_internal/blocked_output.py:112-120` |
| Blocked-message formatter contract | `output_guardrail_blocked_message` formatter never receives rejected output or guardrail `output_info` | `src/agents/run_config.py:489-496` |
| Model-requestable context growth | Function/hosted tools (`WebSearchTool`, `FileSearchTool`, hosted MCP) return results that enter subsequent context; `ToolSearchTool` lets the model discover deferred-loading tools on demand | `src/agents/tool.py:1536-1550`; `src/agents/tool.py:1510-1524` |
| Deferred tool loading | `function_tool(..., defer_loading=True)` and `tool_namespace(...)` keep tool bodies out of the initial tool surface until searched | `src/agents/tool.py:1554-1574`; validation at `src/agents/models/openai_responses.py:1817-1905` |
| Tests: filter pipeline | Sync/async filter invocation, invalid-return error, duplicate-output preference, reasoning-before-follower ordering | `tests/test_call_model_input_filter.py:18-400` |
| Tests: handoff filters | Tools removed from history/new-items, programmatic transcript removal, nested-summary wrapping/parsing | `tests/test_extension_filters.py:215-461` |
| Tests: trimmer | Boundary detection, structured-output preview budgets, opaque-part accounting, schema-prose stripping | `tests/extensions/test_tool_output_trimmer.py:61-383` |

## Answers to Dimension Questions

**1. What decides what goes into context?**
A fixed composition order owned by the runner: (a) session history fetched through `prepare_input_with_session` under `SessionSettings.limit` (`src/agents/run_internal/session_persistence.py:346-357`, invoked from `src/agents/run.py:675-682` and `src/agents/run_internal/run_loop.py:1089-1093`); (b) the new turn's caller input; (c) all generated run items from prior turns converted via `run_item_to_input_item` minus approval items (`src/agents/run_internal/items.py:139-168`); (d) the resolved agent's system prompt and prompt template (`src/agents/run_internal/run_loop.py:2097-2100`); (e) the tool schema list resolved from agent tools + MCP servers after enabled/filter checks (`src/agents/agent.py:272-292`). Everything then passes through normalization (orphan pruning, metadata strip, dedupe) and finally the optional `call_model_input_filter`. Signals included: conversation history, tool results (as first-class items), retrieved documents (hosted `file_search_call`/`web_search_call`/MCP results become items), system prompt, and prompt templates. There is no separate "user profile" or workspace-state injection mechanism in the core loop; those patterns would be implemented through the context object passed to dynamic instructions/tools.

**2. Is selection policy explicit or implicit?**
Explicit at the interface level and implicit in defaults. Every stage has a named, typed extension point (`CallModelInputFilter`, `SessionInputCallback`, `HandoffInputFilter`, `HandoffHistoryMapper`, `reasoning_item_id_policy`, `SessionSettings.limit` — see Evidence table). But the default policies are conservative-inclusion: full history (`limit=None`), all tool items retained, entire transcript forwarded on handoff unless a filter is attached (`src/agents/handoffs/__init__.py:160-162`), and `nest_handoff_history` is opt-in beta (`src/agents/run_config.py:374-381`). So the *mechanism* is explicit; the *default policy* is "include everything," which shifts responsibility to application code.

**3. Can the model influence what it sees?**
Yes, indirectly and directly. Indirectly: every function-tool result becomes a `ToolCallOutputItem` that persists in future turns' context (`src/agents/run_internal/items.py:158-168`), so calling a retrieval/search tool grows its own context. Directly: hosted `ToolSearchTool` lets the model search deferred-loading tool namespaces, and deferred function tools keep their schemas/bodies out of the initial surface until discovered (`src/agents/tool.py:1510-1524`, `src/agents/models/openai_responses.py:1899-1905`). Client-executed `tool_search` calls are explicitly unsupported in the standard runner (`src/agents/run_internal/turn_resolution.py:3023-3027`), showing this is a deliberate boundary rather than an accident.

**4. Are sensitive fields redacted?**
Not from model input content — from side channels. Traces redact error details and omit inputs/outputs when `trace_include_sensitive_data=False` (`src/agents/util/_error_tracing.py:46-53`, `src/agents/tracing/model_tracing.py:7-12`), logging of LLM/tool payloads is off by default (`src/agents/_debug.py:20-27`), and output-guardrail-tripped tool outputs are replaced with a fixed data-free string everywhere the item is replayed or persisted (`src/agents/run_internal/blocked_output.py:40`, formatter contract at `src/agents/run_config.py:489-496`). However, nothing scans or masks content flowing into the model itself (no PII filter, no field-level redaction in `normalize_input_items_for_api`, which only strips SDK-internal keys, `src/agents/run_internal/items.py:316-328`). Applications must implement content redaction inside `call_model_input_filter` or tool wrappers.

## Architectural Decisions

1. **Rebuild-from-source per call, not incremental windowing.** Each model call reconstructs input from session store + generated items (`_prepare_turn_input_items`, `src/agents/run_internal/run_loop.py:334-341`). This makes any filter stateless per call and keeps the session store the single durable truth, at the cost of repeated conversion work.
2. **Single last-mile mutation point.** All selection policies converge on `call_model_input_filter` right before the request (`src/agents/run_internal/turn_preparation.py:51-93`), so one hook can express token-budget trimming, prompt injection, or redaction regardless of upstream storage choices.
3. **Separation of "model view" vs. "session record."** Handoff filters can shrink model input while `new_items` still persist intact (`input_items` vs `new_items`, `src/agents/handoffs/__init__.py:94-99`; enforced by nested-history ownership digests, `src/agents/run_internal/items.py:93-128`). Filtering never destroys history.
4. **Structural validity as a hard invariant.** Orphan pruning, reasoning-follower preservation, dedupe anchoring, and status-field stripping exist because the Responses API rejects malformed sequences (`docstrings` at `src/agents/run_internal/items.py:177-186`, `279-284`). Selection cannot break causality even when users rewrite history arbitrarily.
5. **Server-managed conversations bypass local selection.** With `conversation_id`/`previous_response_id`, history lives server-side; local filters are disabled with warnings and pending-input admission is response-confirmed (`src/agents/run_config.py:366-381`; `src/agents/run_internal/session_persistence.py:85-186`). This acknowledges two distinct context owners instead of pretending one policy covers both.

## Notable Patterns

- **Filter-chaining with clone semantics**: `HandoffInputData.clone` preserves private provenance (`_nested_history_owned_items`) across chained filters so `remove_all_tools` after `nest_handoff_history` doesn't lose track of summarized items (`src/agents/handoffs/__init__.py:101-115`; chaining comment at `src/agents/extensions/handoff_filters.py:44-49`).
- **Digest-based occurrence identity**: SHA-256 digests over canonicalized items (`digest_input_item`, `src/agents/run_internal/items.py:393-412`) let the SDK match rewritten history back to original run items without relying on unstable indexes — a provenance ledger for context decisions.
- **Policy objects as callables**: `ToolOutputTrimmer` is a dataclass implementing the `CallModelInputFilter` protocol via `__call__` (`src/agents/extensions/tool_output_trimmer.py:136-202`), making a complex policy a one-line config value.
- **Read-side and write-side symmetry**: reasoning-ID policy is applied both on persistence write (`save_result_to_session`) and on history read (`prepare_input_with_session`) so pre-existing stored history can't poison later requests (`src/agents/run_internal/session_persistence.py:362-368`).
- **Safe-preview encoding**: the trimmer guarantees replacements fit the configured char budget by trying progressively shorter headers rather than emitting an oversized summary (`src/agents/extensions/tool_output_trimmer.py:294-332`).

## Tradeoffs

- **Simplicity vs. scale**: default full-history inclusion means unbounded growth for long sessions unless the app opts into `SessionSettings.limit` (`src/agents/memory/session_settings.py:38-39`) or a trimmer; there is no automatic token accounting anywhere in `src/agents/run_internal/`.
- **Character budgets vs. semantic value**: `ToolOutputTrimmer` trims by length and recency only; an important old tool output gets truncated exactly like noise, with only a `[Trimmed: <tool> …]` marker left behind (`src/agents/extensions/tool_output_trimmer.py:260-270`).
- **Flexibility vs. safety**: `call_model_input_filter` receives the complete input including any sensitive content and may return anything; type validation exists (`UserError` on non-`ModelInputData`, `src/agents/run_internal/turn_preparation.py:78-79`) but no content constraints.
- **Server-managed simplicity vs. filter capability**: choosing server-managed conversation state forfeits handoff input filters entirely (`src/agents/handoffs/__init__.py:169-171`), forcing an either/or decision between provider-side memory and client-side selection control.

## Failure Modes / Edge Cases

- **Malformed stored history**: corrupt JSON rows in SQLite sessions are skipped silently, and the fetch window doubles until the limit is satisfied by valid rows (`src/agents/memory/sqlite_session.py:300-345`) — resilient but can mask data-loss bugs.
- **Callback misuse**: `session_input_callback` returning a non-list raises `UserError`; returning duplicated history is detected by reference/frequency maps so retries don't double-persist (`src/agents/run_internal/session_persistence.py:403-404, 406-449`).
- **Filter exceptions during streaming**: errors in `call_model_input_filter` attach span errors respecting the sensitive-data flag, then re-raise (`src/agents/run_internal/turn_preparation.py:81-93`); tested in `tests/test_call_model_input_filter.py:93-119`.
- **Rewind safety on retry**: session tail rewind verifies exact serialized suffix match and restores popped items on mismatch, avoiding cross-turn corruption under concurrent writes (`src/agents/run_internal/session_persistence.py:978-1040`).
- **Compaction starvation edge**: compaction candidates exclude user messages and prior compaction items (`select_compaction_candidate_items`, `src/agents/memory/openai_responses_compaction_session.py:34-55`); a session dominated by huge user messages never crosses the 10-item threshold despite growing tokens.
- **Trimmer no-op guarantees**: if the replacement summary would be longer than the original, the item is left untouched rather than inflating context (`src/agents/extensions/tool_output_trimmer.py:265-266, 407-408`).

## Future Considerations

- Add a token-budget-aware selector (counting via tokenizer or usage telemetry already captured in `src/agents/usage.py`) so trimming doesn't depend purely on character counts.
- Expose an opt-in, user-facing provenance report ("why was this item included") built on the existing nested-history ownership digests (`src/agents/run_internal/items.py:114-128`) — today that machinery answers the dimension question internally but is not surfaced.
- Provide a content-redaction hook positioned between normalization and the model call (distinct from tracing flags) for PII/secret scrubbing, since `call_model_input_filter` conflates selection with transformation.
- Document and warn when neither `session_settings.limit` nor a `call_model_input_filter` nor a compaction session is configured for long-running sessions — currently unbounded context growth is silent.

## Questions / Gaps

- **No evidence found** for any relevance/scoring-based document selection (no embeddings, rerankers, or retrieval scoring in `src/agents/`); retrieval enters context only via whole-result tool items. Searched `src/agents/**` for `retriev|rerank|embedding|relevance` patterns via tool-call item types and found only hosted `file_search_call` handling (`src/agents/run_internal/items.py:43-55`).
- **No evidence found** for field-level sensitive-data masking of model input; searches centered on `redact`, `sensitive`, `sanitize` (all hits concern traces, logs, or Conversations-API ID stripping, e.g., `src/agents/run_internal/session_persistence.py:910-930`).
- Whether `prompts.py` prompt templates can inject per-user variables at runtime was not traced exhaustively here; `PromptUtil.to_model_input` (`src/agents/agent.py:1079-1083`) resolves them per run, but variable sourcing deserves a dedicated study (dimension overlap with prompt management).
- Observability of final assembled context is limited: debug logs print item counts/IDs only (`src/agents/run_internal/run_loop.py:2130-2135, 2155-2158`); there is no supported dump of the exact post-filter payload short of attaching a `call_model_input_filter` or reading trace spans with sensitive data enabled.

---

Generated by `11.01-context-selection-policy` against `openai-agents-sdk`.
