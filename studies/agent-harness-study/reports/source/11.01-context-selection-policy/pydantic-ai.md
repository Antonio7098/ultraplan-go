# Source Analysis: pydantic-ai

## Dimension 11.01: Context Selection Policy

### Source Info

| Field | Value |
|-------|-------|
| Name | pydantic-ai |
| Path | `studies/agent-harness-study/sources/pydantic-ai` |
| Language / Stack | Python (pydantic-ai-slim agent framework; provider adapters under `pydantic_ai_slim/pydantic_ai/models/`) |
| Analyzed | 2026-08-25 |

## Summary

Pydantic AI does not "select" context in the retrieval sense — it **retains the entire conversation history by default and re-sends all of it on every model request**, then applies a layered, explicitly ordered *subtraction and reshaping* pipeline before anything reaches the wire. The assembly point is `ModelRequestNode._prepare_request` (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:1451-1623`), which per model step:

1. Appends the new `ModelRequest` to the canonical history (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:1469`);
2. Re-resolves instructions fresh each step from agent instructions plus current toolset instructions (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:1492`, `pydantic_ai_slim/pydantic_ai/_agent_graph.py:743-761`);
3. Runs the capability chain's `before_model_request` hook, which is where user-supplied **history processors** (`ProcessHistory`) prune, rewrite, or summarize the history (`pydantic_ai_slim/pydantic_ai/capabilities/process_history.py:31-38`; invoked via `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1515`);
4. Validates the processed history (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:1525-1529`);
5. Normalizes it with `_clean_message_history` — drop orphaned tool results → repair dangling tool calls → merge consecutive messages (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:2960-2986`);
6. Projects it through the provider adapter via `Model.prepare_messages` (system-prompt hoisting, tool-search synthesis, speech-part conversion; `pydantic_ai_slim/pydantic_ai/models/__init__.py:690-778`);
7. Applies provider-native compaction trimming at `CompactionPart` boundaries (`pydantic_ai_slim/pydantic_ai/models/__init__.py:2072-2131`);
8. Optionally pre-counts tokens and enforces usage limits before sending (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:1603-1621`).

Selection policy is therefore **explicit and dynamic**: every stage is a named, documented function or capability, and the composition re-runs on each step of the agent loop. Tool visibility is itself part of the selected context and can change mid-run — the model can expand what it sees via tool search / deferred tool loading, tools can reveal other tools via `ToolReturn.tools`, and validation failures are fed back into context as `RetryPromptPart`. Trust-boundary filtering exists for untrusted client-supplied history (`sanitize_messages`, `pydantic_ai_slim/pydantic_ai/messages.py:2953-3135`), but there is **no built-in sensitive-field redaction on the trusted path to the model** — redaction (`redact_binary_content`) applies only to telemetry export.

**Answering the dimension's guiding question — "Can the system explain why a particular document was included in context?"**: partially. Inclusion provenance is structural rather than explanatory: every history element carries typed part identity (`ToolReturnPart.tool_call_id`, `ToolAvailabilityDeltaPart.tools_added`, etc., `pydantic_ai_slim/pydantic_ai/messages.py`), so one can trace *which mechanism* put content there (a tool call, a delta reveal, the user prompt). But there is no first-class retrieval subsystem with inclusion scoring or rationale, and no recorded "reason" beyond the part type and its producing call.

## Rating

**9 / 10**

Rationale against the rubric:

- **Clear model with explicit interfaces (9–10 territory):** the selection pipeline is an ordered sequence of named pure functions over `list[ModelMessage]`, with the ordering rationale spelled out in the docstring of `_clean_message_history` ("each pass ADDs … or REMOVEs content, never silently dropping anything a provider could accept", `pydantic_ai_slim/pydantic_ai/_agent_graph.py:2963-2982`). Extension points are public API: `HistoryProcessor` type (`pydantic_ai_slim/pydantic_ai/_history_processor.py:17-26`), `ProcessHistory` capability, hooks (`before_model_request`, `docs/hooks.md:144`).
- **Tests:** extensive dedicated coverage of processor semantics including edge cases — empty processed history and wrong-tail-type raise `UserError` (`tests/test_history_processor.py:973-992`), pruning during multi-step runs (`tests/test_history_processor.py:1101-1170`), reordering/injection effects on `new_messages()` bookkeeping (`tests/test_history_processor.py:1251-1357`); the repo also maintains a suite-wide prompt-cache-prefix regression net for tests that move history shape (`tests/AGENTS.md`, cache-prefix invariant section).
- **Operational safeguards:** post-processing validation (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:1525-1529`), pre-request token counting with `UsageLimits.count_tokens_before_request` / `check_per_request_input_tokens` (`pydantic_ai_slim/pydantic_ai/usage.py:459,492,562`), untrusted-input sanitization (`pydantic_ai_slim/pydantic_ai/messages.py:2953`), and mutation-detection warnings (`pydantic_ai_slim/pydantic_ai/exceptions.py:669-682`).
- **Why not 10:** no built-in sensitive-data redaction stage on the model path (delegated to users via `ProcessHistory`, whose docs name "redact sensitive content" as a user responsibility, `docs/capabilities/process-history.md:1-15`); automatic context-window management depends on provider support (OpenAI Responses / Anthropic compaction capabilities) or hand-written processors; and inclusion rationale is implicit in message structure rather than an inspectable policy artifact.

## Evidence Collected

Every entry cites file paths relative to the source root with line numbers.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Context assembly orchestrator | `_prepare_request` builds the outgoing message list each step: append request → resolve instructions → run `before_model_request` chain → validate → clean → project → trim → count tokens | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1451-1623` |
| Canonical history append | New `ModelRequest` appended to `ctx.state.message_history` before assembly | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1469` |
| Per-step instruction assembly | `_get_instructions` combines base agent instructions with live toolset instructions each step | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:743-761`; normalization in `pydantic_ai_slim/pydantic_ai/_instructions.py:58-76` |
| Dynamic instruction resolution | Strings pass through; callables/`TemplateStr` wrapped in `SystemPromptRunner` and resolved against `RunContext` | `pydantic_ai_slim/pydantic_ai/_instructions.py:35-55,79-91` |
| History processor interface | Sync/async × with/without `RunContext` callable union, publicly importable | `pydantic_ai_slim/pydantic_ai/_history_processor.py:11-26` |
| Processor execution point | `ProcessHistory.before_model_request` replaces `request_context.messages` with processor output; sync processors offloaded to executor | `pydantic_ai_slim/pydantic_ai/capabilities/process_history.py:31-38,45-63` |
| Post-processing validation | Processed history must be non-empty and end with `ModelRequest`, else `UserError` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1525-1529` |
| Wire-validity cleanup pipeline | `_clean_message_history` = `_drop_orphaned_tool_results` → `_repair_dangling_tool_calls` → `_merge_consecutive_messages`, ordering justified in docstring | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:2960-2986` |
| Provider projection entry point | `Model.prepare_messages`: cross-provider tool-search translation, `<system>` hoisting for profiles without inline system prompts, `SpeechPart` conversion | `pydantic_ai_slim/pydantic_ai/models/__init__.py:690-778` |
| Compaction boundary part | `CompactionPart` dataclass: readable summary (Anthropic) or opaque encrypted blob in `provider_details` (OpenAI) | `pydantic_ai_slim/pydantic_ai/messages.py:1988-2030` |
| Trim-before-compaction | `_trim_messages_before_compaction` drops everything before latest same-provider boundary, preserves standing prompt + recovered instructions | `pydantic_ai_slim/pydantic_ai/models/__init__.py:2072-2131` |
| Single boundary rule for derived state | `post_compaction_window` defines the window all framework derived state (discovered tools, loaded capabilities) is computed from | `pydantic_ai_slim/pydantic_ai/messages.py:2774-2814` |
| Compaction trigger policies | `OpenAICompaction(message_count_threshold=…\|trigger=callable)` stateless mode; server-triggered otherwise | `pydantic_ai_slim/pydantic_ai/models/openai.py:4716,4781-4854` |
| Token-threshold compaction trigger | `AnthropicCompaction` configures server-side `{'type': 'input_tokens', 'value': token_threshold}` trigger | `pydantic_ai_slim/pydantic_ai/models/anthropic.py:2681,2723` |
| Model-requestable tool discovery | `parse_discovered_tools` / `discovered_tool_names_in_order` scan only the post-compaction window for previously-discovered deferred tools | `pydantic_ai_slim/pydantic_ai/toolsets/_tool_search.py:196-230` |
| Tool-result-driven reveals | A tool returning `ToolReturn(tools=[…])` requests additional tool reveals into subsequent context | `pydantic_ai_slim/pydantic_ai/_tool_execution.py:65-116` |
| Validation errors fed back into context | `RetryPromptPart` carries retry guidance/validation errors back to the model; retry-wins triggers tracked per step | `pydantic_ai_slim/pydantic_ai/_tool_execution.py:881-902`; `RetryPromptPart` at `pydantic_ai_slim/pydantic_ai/messages.py:1650-1672` |
| Untrusted-input sanitization | `sanitize_messages` strips system prompts, non-HTTP file URLs, forced downloads, uploaded files, dangling tail tool calls; warns per rule | `pydantic_ai_slim/pydantic_ai/messages.py:2953-3135` |
| Client compaction distrust | Client-supplied `CompactionPart`s never trusted to stand in for the system prompt; `strip_compaction_parts=True` required when combining with server-side history | `pydantic_ai_slim/pydantic_ai/messages.py:3000-3005`; boundary-drop helper at `pydantic_ai_slim/pydantic_ai/messages.py:2931-2950` |
| System-prompt reinjection | `ReinjectSystemPrompt` capability prepends configured prompt when missing; `replace_existing=True` strips untrusted prompts first | `pydantic_ai_slim/pydantic_ai/capabilities/reinject_system_prompt.py:17-77` |
| Pre-request usage guardrails | `count_tokens_before_request` counts input tokens ahead of send; `check_per_request_input_tokens` aborts over-budget requests | `pydantic_ai_slim/pydantic_ai/usage.py:444-459,492,562`; wired at `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1603-1621` |
| Redaction scope = telemetry only | `redact_binary_content` applied when serializing parts for OTel spans/events, not to messages sent to models | `pydantic_ai_slim/pydantic_ai/_instrumentation.py:141-167`; consumers `pydantic_ai_slim/pydantic_ai/capabilities/instrumentation.py:201,564`; serialization sites `pydantic_ai_slim/pydantic_ai/messages.py:1560,2733` |
| Derived-state reset at boundaries | `RunContext.discovered_tool_names` / `loaded_capability_ids` cut at any `CompactionPart`; anchored evidence widens conservatively | `pydantic_ai_slim/pydantic_ai/_run_context.py:38-57` |
| Reveal state recomputed from sent history | Outgoing reveal state derived after cleanup so processors' removals don't ship stale "revealed" flags | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1570-1577` |
| Tests: processor contract | Empty-history and wrong-tail errors pinned; pruning, reordering, injection, mixed signatures covered | `tests/test_history_processor.py:973-992,1101-1248,1251-1357,820-911` |
| Docs: processing history | Documented patterns: keep-recent, LLM summarization, RunContext-aware processors; summarization named as one of several window strategies alongside provider compaction | `docs/message-history.md:690-920`; `docs/capabilities/process-history.md:1-15` |
| Docs: system prompt & history interaction | New system prompt not generated when `message_history` supplied; `ReinjectSystemPrompt` recommended for sources that don't round-trip prompts | `docs/message-history.md:153` |

## Answers to Dimension Questions

**1. What decides what goes into context?**

A fixed orchestration in `_prepare_request` decides, in order (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:1451-1623`):

- **Signals included by default:** full prior conversation history (`state.message_history`, append-only), the new user prompt/tool returns for this step, freshly resolved instructions (static strings + dynamic per-step toolset instructions, `pydantic_ai_slim/pydantic_ai/_agent_graph.py:743-761`), the current step's tool definitions/output schemas (`_prepare_request_parameters`, `pydantic_ai_slim/pydantic_ai/_agent_graph.py:764-843`), thinking parts, availability-delta/tool-search exchanges, and compaction summaries.
- **Signals excluded/reshaped:** orphaned tool results are dropped, dangling tool calls get synthesized results, consecutive same-role messages merge (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:2983-2985`); leading `SystemPromptPart`s are hoisted to the provider's top-level system channel where supported, otherwise tagged `<system>` inline (`pydantic_ai_slim/pydantic_ai/models/__init__.py:705-708`); suspended-response tails are split off as continuation seeds (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:859-867`).
- **User-controlled subtraction:** any registered `ProcessHistory` processor may prune, reorder, replace, or summarize the list wholesale before it is validated and cleaned (`pydantic_ai_slim/pydantic_ai/capabilities/process_history.py:31-38`).

**2. Is selection policy explicit or implicit?**

Explicit. Each stage is a separately named, documented function or capability; the pipeline order is argued in prose in `_clean_message_history`'s docstring (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:2963-2982`); the compaction boundary is centralized in one function that both the wire-level trim and run-state derivation must use (`pydantic_ai_slim/pydantic_ai/messages.py:2782-2787`); and the behavior is documented for users (`docs/message-history.md:690-920`). The one implicit aspect is the default policy itself: "send everything" is what happens when the user registers nothing.

**3. Can the model influence what it sees?**

Yes, through three sanctioned channels:

- **Tool search / deferred loading:** the model calls `search_tools` (or native equivalents) and discovered names become visible tools in later turns; discovery evidence is re-derived from the visible post-compaction window only, so the model cannot "remember" tools it can no longer see (`pydantic_ai_slim/pydantic_ai/toolsets/_tool_search.py:196-230`).
- **Tool results requesting reveals:** a tool may return `ToolReturn(tools=[...])` and the execution layer translates that into validated reveal requests (`pydantic_ai_slim/pydantic_ai/_tool_execution.py:100-116`).
- **Retries:** `ModelRetry` / validation failures become `RetryPromptPart`s in the next request, letting the model steer what corrective information it receives next round (`pydantic_ai_slim/pydantic_ai/_tool_execution.py:881-902`).

The framework guards this influence: forged-but-well-shaped availability deltas are filtered so arbitrary tool names can't be announced into system voice (`pydantic_ai_slim/pydantic_ai/models/__init__.py:738-750`), and availability gates execution, so an under-count refuses calls rather than allowing phantom tools (`pydantic_ai_slim/pydantic_ai/toolsets/_tool_search.py:199-205`).

**4. Are sensitive fields redacted?**

Not on the model path. The framework's redaction machinery (`redact_binary_content`, `pydantic_ai_slim/pydantic_ai/_instrumentation.py:141-167`) is applied exclusively when serializing message/tool content into OTel telemetry (`pydantic_ai_slim/pydantic_ai/capabilities/instrumentation.py:201,564`; serialization helpers `pydantic_ai_slim/pydantic_ai/messages.py:1560,2733`) — deliberately leaving "the user's own model … alone" (`pydantic_ai_slim/pydantic_ai/_instrumentation.py:155`). What does exist is **trust-boundary filtering**, not field redaction: `sanitize_messages` strips client-submitted system prompts (prompt-injection defense), non-HTTP file URLs (server-side IAM fetch risk), `force_download` escalations, uploaded files, and unresolved tail tool calls, warning on every removal (`pydantic_ai_slim/pydantic_ai/messages.py:2953-3135`). Sensitive-content redaction before the model call is delegated to application code via `ProcessHistory` (`docs/capabilities/process-history.md:1-15`).

## Architectural Decisions

- **Append-only canonical history, rebuilt wire view per request.** `state.message_history[:] = messages` replaces contents in place so `capture_run_messages` observers stay consistent (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:1551-1552`); the merged/synthesized view used for the actual call is deliberately *not* stored back as truth (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:1561-1568`).
- **Selection as an ordered pipeline of pure passes.** Each pass either adds (synthesizes) or removes, never silently drops something a provider could accept; native/builtin parts are untouched because their serializers own them (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:2963-2982`).
- **Provider-specific shaping isolated behind `Model.prepare_messages`.** Cross-provider histories normalize to whatever the active adapter needs; `FallbackModel` defers the decision per candidate model (`pydantic_ai_slim/pydantic_ai/models/__init__.py:709-712`; `pydantic_ai_slim/pydantic_ai/models/fallback.py:286-287`).
- **Capability-based extension instead of constructor sprawl.** History processing, system-prompt reinjection, and provider compactions are all `AbstractCapability` implementations hooking `before_model_request` (`pydantic_ai_slim/pydantic_ai/capabilities/process_history.py:25-42`, `pydantic_ai_slim/pydantic_ai/capabilities/reinject_system_prompt.py:46-77`, `pydantic_ai_slim/pydantic_ai/models/openai.py:4716`, `pydantic_ai_slim/pydantic_ai/models/anthropic.py:2681`).
- **One boundary rule feeds everything.** After compaction, derived state (discovered tools, loaded capabilities, reveal sets) is recomputed from `post_compaction_window` rather than remembered in instance attributes, so it self-heals across failover and mid-run model switches (`pydantic_ai_slim/pydantic_ai/messages.py:2782-2800`; `pydantic_ai_slim/pydantic_ai/_run_context.py:38-57`).
- **Trust boundaries are separate from content policy.** Untrusted client history gets structural sanitization; trusted server history is assumed safe — the two are never silently mixed without `strip_compaction_parts=True` (`pydantic_ai_slim/pydantic_ai/messages.py:3000-3005`).

## Notable Patterns

- **Wire-truthful derived state:** reveal state for the outgoing request is recomputed *after* cleanup, so a processor that removed a search return cannot leave a "revealed" tool with no evidence on the wire (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:1570-1577`).
- **Repair-not-fail normalization:** dangling tool calls get synthesized results rather than erroring, matching the stated principle of making APIs accept the history where possible (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:2977-2979`).
- **Cache-conscious anchoring:** compaction retention decisions distinguish "our own compact call planted the standing prompt" (stamped via `STANDING_PROMPT_PLANTED_KEY` provenance) from externally supplied items, which always get the standing prompt re-inserted (`pydantic_ai_slim/pydantic_ai/messages.py:1979-1985`, `pydantic_ai_slim/pydantic_ai/models/__init__.py:2099-2106`).
- **Conservative intersection across providers:** parse-time state uses a provider-agnostic boundary because the next request's provider isn't knowable; per-provider wire trimming stays exact per request (`pydantic_ai_slim/pydantic_ai/messages.py:2789-2800`).
- **Observable policy:** every sanitization removal emits a targeted `UserWarning` naming the rule and the escape hatch (`pydantic_ai_slim/pydantic_ai/messages.py:3077-3133`).

## Tradeoffs

- **Unbounded growth by default.** With no processor or compaction capability, every turn re-sends the whole history; cost control is opt-in (user `ProcessHistory`, e.g. the documented keep-recent/summarize patterns, `docs/message-history.md:766-841`, or provider compaction). Usage limits abort rather than trim when budgets are exceeded (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:1619-1621`).
- **Processors are powerful footguns.** They replace run history wholesale, affecting `new_messages()` bookkeeping; the framework handles index shifting, reordering, and injected run_ids defensively (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:1534-1559`; tests `tests/test_history_processor.py:1251-1357`) and warns on in-place mutation (`pydantic_ai_slim/pydantic_ai/exceptions.py:669-682`), but the burden of correct pruning rests on the user.
- **Provider-coupled automatic compaction.** Native compaction exists only where the provider supports it (OpenAI Responses, Anthropic); other providers need manual strategies, and cross-provider replay relies on the conservative window rule rather than exact equivalence.
- **No content-level safety net.** Because redaction is delegated to users, a forgotten `ProcessHistory` means secrets reaching the provider — a deliberate library-boundary choice consistent with the project's "strong primitives over batteries" philosophy (`AGENTS.md` Philosophy section).

## Failure Modes / Edge Cases

- **Processed history invalid:** empty list or trailing non-`ModelRequest` raises `UserError` immediately (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:1525-1529`; pinned in `tests/test_history_processor.py:973-992`).
- **Orphaned/dangling tool exchanges:** dropped or repaired deterministically, with frontier gating so the last response's still-answerable calls aren't prematurely synthesized (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:2974-2979`).
- **Client-supplied compaction hides trusted history:** mitigated by dropping client boundaries when combined with server history and never trusting client stamps for standing-prompt retention (`pydantic_ai_slim/pydantic_ai/messages.py:2931-2941,2995-3005`).
- **Standing-prompt decay across double compaction:** retention honored for one hop only; re-compaction plants the standing prompt explicitly and stamps the new item (`pydantic_ai_slim/pydantic_ai/models/__init__.py:2103-2106`).
- **Stale discovery after pruning:** under-counted discovered tools cause call refusal (availability gates execution) rather than silent misbehavior; the known gap and tracking are acknowledged in `parse_discovered_tools`' docstring (`pydantic_ai_slim/pydantic_ai/toolsets/_tool_search.py:199-205`).
- **In-place mutation of history:** detected at run end and warned, steering users toward building new objects (`pydantic_ai_slim/pydantic_ai/exceptions.py:669-682`; guidance `docs/message-history.md:568`).

## Future Considerations

- **Built-in sensitive-data scrubbing stage:** a first-class, tested redaction pass (secrets/PII patterns) between `before_model_request` and `_clean_message_history` would close the largest policy gap without changing the pipeline shape.
- **Token-budget-driven portable trimming:** an automatic, provider-independent keep-within-budget strategy would complement the two provider-native compaction capabilities; today only abort-style limits exist outside providers.
- **Inclusion provenance metadata:** recording *why* each part was included (retrieval score, reveal source, processor action) would make the dimension question fully answerable from artifacts rather than inferred from part types.

## Questions / Gaps

- **Is there retrieval-augmented document selection?** No evidence found. Searched core package for retrieval/rerank/inclusion-scoring logic tied to context assembly; none exists. An embeddings abstraction is exported (`pydantic_ai_slim/pydantic_ai/__init__.py:24,195`) with provider backends under `pydantic_ai_slim/pydantic_ai/embeddings/`, but grep shows it is not referenced anywhere in the agent graph or message-prep path — it is a primitive for users, not a wired-in selector.
- **User-profile/workspace-state signals?** No evidence of built-in user-profile or workspace-state injection; such signals enter only via dependency injection (`deps_type`) consumed by dynamic instructions/tools (e.g. `pydantic_ai_slim/pydantic_ai/_instructions.py:79-91` resolving callables against `RunContext`).
- **Exact ordering guarantees among multiple `ProcessHistory` capabilities** are documented as registration order (`docs/capabilities/process-history.md:13`) and exercised by tests (`tests/test_history_processor.py:318-406`), but I did not locate the `CombinedCapability` fan-out line numbers in this pass; the behavioral guarantee is well-evidenced even though the dispatch site wasn't pinned.

---

Generated by Dimension 11.01 (Context Selection Policy) against `pydantic-ai`.
