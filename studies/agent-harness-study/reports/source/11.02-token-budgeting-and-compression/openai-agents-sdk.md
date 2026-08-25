# Source Analysis: openai-agents-sdk

## 11.02 Token Budgeting and Compression

### Source Info

| Field | Value |
|-------|-------|
| Name | openai-agents-sdk |
| Path | `studies/agent-harness-study/sources/openai-agents-sdk` |
| Language / Stack | Python (OpenAI Agents SDK; pydantic dataclasses, asyncio, OpenAI Responses / Chat Completions APIs) |
| Analyzed | 2026-08-25 |

> Citation convention: all file paths below are workspace-relative and rooted at the selected source directory `studies/agent-harness-study/sources/openai-agents-sdk`.

## Summary

The SDK does not maintain a unified, client-side token budget. Instead it layers four independent compression mechanisms around the model call: (1) **post-hoc usage accounting** — provider-reported token counts are aggregated into a `Usage` object with per-request entries (`src/agents/usage.py:196-313`) and surfaced on traces; (2) **server-side compaction** — either delegated per-request via `ModelSettings.context_management` with a `compact_threshold` (`src/agents/model_settings.py:191-196`, forwarded at `src/agents/models/openai_responses.py:997`) or driven between turns by the `OpenAIResponsesCompactionSession` wrapper that calls the `responses.compact` endpoint and transactionally rewrites session history with rollback-on-failure (`src/agents/memory/openai_responses_compaction_session.py:170-263`, `271-336`); (3) **client-side trimming** — the opt-in `ToolOutputTrimmer` input filter replaces large tool outputs in old turns with bounded previews while never touching recent turns (`src/agents/extensions/tool_output_trimmer.py:87-202`), and sandbox tool outputs are truncated head+tail style under an approximate 4-bytes-per-token policy (`src/agents/sandbox/util/token_truncation.py:6`, `73-113`); (4) **handoff transcript summarization** — an opt-in beta (`RunConfig.nest_handoff_history`, `src/agents/run_config.py:374-381`) collapses prior transcripts into numbered, re-parseable assistant summary segments (`src/agents/handoffs/history.py:83-94`, `376-398`). Overflow is not measured before calls; when Chat Completions terminates with `finish_reason="length"` and no visible output, the SDK raises an explicit `ModelBehaviorError` rather than silently continuing (`src/agents/models/openai_chatcompletions.py:328-343`). The mechanisms are individually well-tested and documented, but there is no single budget coordinator that measures rendered context size against a model's context window before dispatch.

## Rating

**7 / 10** — Clear model with tests, explicit interfaces, and operational safeguards.

Rationale:
- Multiple explicit, documented compression surfaces (session compaction wrapper, server-side `context_management`, input-filter trimmer, handoff summarizer, sandbox truncation utilities), each with dedicated tests: 1,803 lines covering compaction-session behavior including cancellation rollback (`tests/memory/test_openai_responses_compaction_session.py:1455-1640`), 1,167 lines for the trimmer including tight-budget prioritization (`tests/extensions/test_tool_output_trimmer.py:291-433`), and sandbox capability coverage (`tests/sandbox/capabilities/test_compaction_capability.py`).
- Operational safeguards are real: compaction history replacement is treated as a recoverable transaction with restore-on-cancel (`src/agents/memory/openai_responses_compaction_session.py:271-336`) and documented failure semantics (`docs/sessions/index.md:289`).
- What keeps it out of 8–10: no client-side tokenizer or pre-call context-size measurement anywhere in the core runner; budgets are heterogeneous and heuristic (item counts, characters, ~4 bytes/token); nothing ties `max_tokens`, `compact_threshold`, trimmer limits, and session limits into one coherent per-model or per-agent budget; overflow handling ultimately depends on server behavior.

## Evidence Collected

Every entry cites a path relative to `studies/agent-harness-study/sources/openai-agents-sdk`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Token counters (post-hoc) | `Usage` dataclass aggregates `input_tokens`, `output_tokens`, `total_tokens`, plus cached/cache-write/reasoning details; normalized for providers that omit fields | src/agents/usage.py:196-255 |
| Per-request usage breakdown | `request_usage_entries` preserves `[100K, 150K, 80K]`-style per-call inputs "helpful … for context window management" | src/agents/usage.py:218-229 |
| Usage aggregation API | `Usage.add()` merges totals and appends per-request entries | src/agents/usage.py:257-312 |
| Usage observability | Span serializers `turn_usage_to_span_data`, `task_usage_to_span_data`, `model_usage_to_span_usage` expose tokens on traces | src/agents/usage.py:434-485 |
| Compaction request billed to run | Compaction response usage added to run wrapper's `Usage` | src/agents/memory/openai_responses_compaction_session.py:240-242 |
| Output-token budget config | `ModelSettings.max_tokens` ("maximum number of output tokens") passed as `max_output_tokens` to Responses API | src/agents/model_settings.py:129; src/agents/models/openai_responses.py:984 |
| Provider truncation passthrough | `truncation: Literal["auto", "disabled"]` forwarded verbatim to Responses API | src/agents/model_settings.py:123-127; src/agents/models/openai_responses.py:983 |
| Server-side compaction threshold | `context_management=[{"type": "compaction", "compact_threshold": N}]` setting; validated field-by-field | src/agents/model_settings.py:191-196, 361-368 |
| Sandbox compaction capability | `Compaction(Capability)` injects `context_management.compact_threshold` into sampling params; part of default capabilities | src/agents/sandbox/capabilities/compaction.py:162-208; src/agents/sandbox/capabilities/capabilities.py:10 |
| Context-window table | `_MODEL_CONTEXT_WINDOWS` hardcodes windows (1_047_576 / 400_000 / 200_000 / 128_000) keyed by normalized model names | src/agents/sandbox/capabilities/compaction.py:25-115 |
| Dynamic vs static policy | `DynamicCompactionPolicy.threshold=0.9 × context_window`; `StaticCompactionPolicy` default `_DEFAULT_COMPACT_THRESHOLD = 240_000` | src/agents/sandbox/capabilities/compaction.py:12, 143-159 |
| Post-compaction context pruning | `Compaction.process_context` drops everything before the last `compaction` item | src/agents/sandbox/capabilities/compaction.py:210-225 |
| Server compaction item ingestion | Runner converts `compaction` outputs into `CompactionItem`s; excluded from streamed events | src/agents/run_internal/turn_resolution.py:3002-3013; src/agents/run_internal/streaming.py:58-59 |
| Client-side session compaction trigger | Default trigger: ≥10 candidate items (`DEFAULT_COMPACTION_THRESHOLD`); overridable via `should_trigger_compaction` hook receiving response_id/mode/items | src/agents/memory/openai_responses_compaction_session.py:28, 58-60, 130-134, 206-214 |
| Candidate selection priority | User messages and existing `compaction` items excluded from compaction candidates | src/agents/memory/openai_responses_compaction_session.py:42-55 |
| Compaction modes | `"previous_response_id"`, `"input"`, `"auto"`; auto falls back to input when unstored/no response_id | src/agents/memory/openai_responses_compaction_session.py:31, 589-604 |
| Transactional history replacement | clear→add treated as one transaction; exception or `CancelledError` restores previous history; shielded re-await drains restore despite repeated cancellation | src/agents/memory/openai_responses_compaction_session.py:271-336 |
| Replay-safety normalization | Strips orphaned assistant IDs when reasoning items were dropped (avoids Responses 400s); normalizes compacted user image/file content | src/agents/memory/openai_responses_compaction_session.py:458-484, 510-579 |
| Deferred compaction ordering | Runner defers compaction when local tool/handoff outputs exist this turn; forces it on next eligible turn | src/agents/run_internal/session_persistence.py:656-696 |
| Trimmer sliding window | `recent_turns` user messages define a boundary; older items only are trimmed | src/agents/extensions/tool_output_trimmer.py:204-220 |
| Trimmer budget knobs | `max_output_chars=500`, `preview_chars=200`, optional `trimmable_tools` allowlist (bare + namespaced names) | src/agents/extensions/tool_output_trimmer.py:112-134, 171-176 |
| Trimmer replacement format | `[Trimmed: {tool} output — N chars → M char preview]\n{preview}...` replaces oversized outputs; refuses if summary would be larger | src/agents/extensions/tool_output_trimmer.py:243-270 |
| Structured-output priority | Text parts preserved first; opaque parts (images/files) counted and dropped with typed notes; header cascade fits tight budgets | src/agents/extensions/tool_output_trimmer.py:288-332, 338-390 |
| Schema prose stripping | Tool-search results drop `description`/`title`/`$comment`/`examples` and recurse through subschema keywords | src/agents/extensions/tool_output_trimmer.py:43-77, 436-495 |
| Approximate token counting | `APPROX_BYTES_PER_TOKEN = 4`; `approx_token_count`, `approx_bytes_for_tokens` used instead of a tokenizer | src/agents/sandbox/util/token_truncation.py:6, 209-221 |
| Head+tail truncation | Budget split 50/50 between prefix and suffix; marker `…N tokens truncated…` records removed amount | src/agents/sandbox/util/token_truncation.py:89-113, 186-194 |
| Per-tool output budget | Shell tools accept `max_output_tokens` argument and truncate output before returning to model | src/agents/sandbox/capabilities/tools/shell_tool.py:25-26, 104-137 |
| Fixed memory budgets | Memory summary capped at 15_000 tokens (`_MEMORY_SUMMARY_MAX_TOKENS`); phase-one rollouts at 150_000 (`_PHASE_ONE_ROLLOUT_TOKEN_LIMIT`) | src/agents/sandbox/capabilities/memory.py:15, 71; src/agents/sandbox/memory/phase_one.py:19, 72 |
| History length limit | `SessionSettings.limit` caps items fetched when assembling model input; resolved from session + RunConfig | src/agents/memory/session_settings.py:38; src/agents/run_internal/session_persistence.py:346-357 |
| Input filter hook point | `call_model_input_filter` runs immediately before each model call; errors traced and raised as `UserError` | src/agents/run_internal/turn_preparation.py:51-93; src/agents/run_config.py:438-443 |
| Handoff transcript summarization | Opt-in `nest_handoff_history`; default mapper emits numbered transcript lines inside `<CONVERSATION HISTORY>` markers as one assistant message segment | src/agents/run_config.py:374-389; src/agents/handoffs/history.py:29-49, 376-398 |
| Summary fidelity mechanics | Items serialized as JSON (with legacy role/content fallback); nested summaries flattened and re-parsed on later handoffs so summaries stay lossless-enough to replay | src/agents/handoffs/history.py:401-453, 456-491 |
| Verbatim vs summary ranking | Messages forwarded verbatim; `function_call`, `function_call_output`, `reasoning` summarized-only; programmatic transcript items indivisible | src/agents/handoffs/history.py:42-49, 633-664 |
| Length-truncation detection | Non-streaming and streaming Chat Completions raise `ModelBehaviorError` when `finish_reason="length"` yields no text/tool/refusal; usage still attached to span | src/agents/models/openai_chatcompletions.py:303-343; src/agents/models/chatcmpl_stream_handler.py:1189-1215 |
| Tests: compaction lifecycle | Runner-flow compaction, skip-with-tool-outputs, deferred persistence across turns, force bypassing threshold | tests/memory/test_openai_responses_compaction_session.py:1455-1640, 1191 |
| Tests: rollback safety | History restored on add failure, on clear-cancelled-after-mutation, on repeated cancellation during restore | tests/memory/test_openai_responses_compaction_session.py:545-881, 1080-1190 |
| Tests: trimmer edge cases | Tight-budget text prioritization, allowlists, qualified names, schema-keyword preservation | tests/extensions/test_tool_output_trimmer.py:291-433, 549-694 |

## Answers to Dimension Questions

**1. Is token usage measured before calling the model?**
No. There is no client-side tokenizer and no pre-flight context-size check in the core runner. Measurement is post-hoc: provider-reported usage is captured after each call into `Usage` (`src/agents/usage.py:196-313`) and attached to generation spans (`src/agents/models/openai_chatcompletions.py:306-313`). Pre-call sizing is approximate and character-based where it exists at all: the `ToolOutputTrimmer` compares serialized character counts (`src/agents/extensions/tool_output_trimmer.py:253-256`), and sandbox utilities estimate tokens as UTF-8 bytes ÷ 4 (`src/agents/sandbox/util/token_truncation.py:6, 209-211`). Whether the *rendered* context crosses the window is decided server-side (Responses `truncation="auto"` passthrough at `src/agents/models/openai_responses.py:983`; `compact_threshold` at `src/agents/model_settings.py:191-196`). The one client-side guard is terminal: a length-truncated empty completion raises `ModelBehaviorError` (`src/agents/models/openai_chatcompletions.py:328-343`).

**2. What gets dropped when budget is exceeded?**
Depends on which mechanism fires:
- Server-side compaction (`context_management`): the provider decides; the SDK then prunes all items before the last `compaction` item from subsequent input (`src/agents/sandbox/capabilities/compaction.py:210-225`).
- Session compaction: the whole stored history is replaced by the compacted output of `responses.compact` (`src/agents/memory/openai_responses_compaction_session.py:238-255`). User messages are deliberately kept out of candidates so the summarizer sees them but the trigger count does not (`select_compaction_candidate_items`, lines 42-55).
- `ToolOutputTrimmer`: oldest large tool outputs become `[Trimmed: …]` previews; the last `recent_turns` user messages and everything after them are untouched (`src/agents/extensions/tool_output_trimmer.py:151-220`); within structured outputs, opaque parts (images/files) are dropped before text (`src/agents/extensions/tool_output_trimmer.py:288-332`).
- Sandbox command output: middle of the string is removed, keeping head+tail halves with an exact removed-count marker (`src/agents/sandbox/util/token_truncation.py:100-113, 192-194`).
- Chat Completions overflow producing nothing visible: nothing is silently dropped — the run fails fast with `ModelBehaviorError` (`src/agents/models/chatcmpl_stream_handler.py:1189-1215`).

**3. Is summarization faithful?**
Two distinct answers. The handoff summary is *structurally* faithful rather than semantic: every transcript item is serialized (JSON preferred, legacy `role: content` fallback) into numbered records wrapped in markers (`src/agents/handoffs/history.py:376-398, 401-453`), and those records are re-parsed/flattened on subsequent handoffs so earlier summaries survive later ones (`src/agents/handoffs/history.py:456-491`) — content is reformatted, not interpreted, though tool payloads inside summaries are not replayed as structured items (`_SUMMARY_ONLY_INPUT_TYPES`, lines 42-49). The LLM-semantic summarization (`responses.compact`) is delegated entirely to the server; the SDK cannot vouch for its faithfulness but engineers the *plumbing* for safety: orphaned assistant IDs are stripped to avoid 400s (`src/agents/memory/openai_responses_compaction_session.py:458-469`), compacted user image/file parts are normalized to replayable shapes (`510-579`), and any failed or cancelled replacement restores prior history (`271-336`). No evidence found of any local verification that a compacted summary preserves task-critical information.

**4. Is budget configurable?**
Extensively, but per-mechanism rather than as one budget: output cap via `ModelSettings.max_tokens` (`src/agents/model_settings.py:129`); provider truncation mode (`123-127`); server compaction threshold via `context_management` (`191-196`), including static vs dynamic (0.9 × known window) policies for sandbox agents (`src/agents/sandbox/capabilities/compaction.py:143-159`); session-compaction trigger hook and mode per session instance (`src/agents/memory/openai_responses_compaction_session.py:96-134`); trimmer recency/size/allowlist parameters (`src/agents/extensions/tool_output_trimmer.py:112-115`); per-tool `max_output_tokens` for shell commands (`src/agents/sandbox/capabilities/tools/shell_tool.py:137, 152`); and retrieved-history length via `SessionSettings.limit` (`src/agents/memory/session_settings.py:38`, applied at `src/agents/run_internal/session_persistence.py:350-357`). Configuration can be set globally on `RunConfig.model_settings` or per-agent (`Agent.model_settings` merged in `get_model_settings`, `src/agents/run_internal/turn_preparation.py:162-167`), satisfying "per-model/per-agent" configurability. What is *not* configurable is a single declared context budget per agent that all mechanisms consult.

## Architectural Decisions

1. **Delegate overflow decisions to the server.** The core runner sends `truncation` and `context_management.compact_threshold` verbatim to the Responses API (`src/agents/models/openai_responses.py:983, 997`) instead of estimating rendered tokens locally. This avoids shipping a tokenizer but makes client-side behavior dependent on provider support.
2. **Compaction as a session decorator, not runner logic.** `OpenAIResponsesCompactionSession` wraps any `Session` and intercepts post-turn persistence (`src/agents/run_internal/session_persistence.py:656-696`), keeping the runner ignorant of compaction except for defer/force bookkeeping.
3. **Replacement-as-transaction.** Clear→rewrite of history is guarded by a mutation lock plus restore-on-exception/cancel with shielded draining (`src/agents/memory/openai_responses_compaction_session.py:142-144, 271-336`) — treating history destruction as the primary hazard rather than the compaction call itself.
4. **Compression hooks as composition points.** Both `call_model_input_filter` (`src/agents/run_internal/turn_preparation.py:51-93`) and the capability `process_context` pipeline (`src/agents/sandbox/runtime_agent_preparation.py:159-169`) are generic extension seams; `ToolOutputTrimmer` and sandbox `Compaction` are consumers, so users can swap in their own budget logic.
5. **Approximation over precision in sandbox contexts.** A fixed 4 bytes/token constant (`src/agents/sandbox/util/token_truncation.py:6`) trades accuracy for zero dependencies; acceptable for tool-output shaping, risky as a general budget basis.
6. **Fail loudly on unusable truncation.** Empty, length-truncated completions surface as `ModelBehaviorError` with usage preserved on the span (`src/agents/models/chatcmpl_stream_handler.py:1203-1215`) rather than being coerced into refusals or empty turns.

## Notable Patterns

- **Recency-first protection:** every client-side mechanism protects recent state — trimmer's recent-turns boundary (`src/agents/extensions/tool_output_trimmer.py:204-220`), compaction's exclusion of user messages from candidates (`src/agents/memory/openai_responses_compaction_session.py:42-55`), and handoff forwarding of role-bearing messages verbatim while summarizing tool/reasoning items (`src/agents/handoffs/history.py:633-653`).
- **Marker-delimited, re-parseable summaries:** `<CONVERSATION HISTORY>` wrappers with configurable markers (`src/agents/handoffs/history.py:29-41, 52-80`) make summaries idempotent under repeated handoffs.
- **Deferral for consistency:** compaction is deferred when a turn produced local tool outputs so the summary includes them, then forced on the next eligible turn (`src/agents/run_internal/session_persistence.py:657-673`; test at `tests/memory/test_openai_responses_compaction_session.py:1558-1600`).
- **Graceful degradation ladders:** trimmer header cascade picks the largest informative `[Trimmed: …]` header that fits the budget (`src/agents/extensions/tool_output_trimmer.py:308-332`); compaction mode `auto` downgrades `previous_response_id` → `input` when responses are unstored (`src/agents/memory/openai_responses_compaction_session.py:589-604`).
- **Observability of savings and cost:** trimmed char counts logged (`src/agents/extensions/tool_output_trimmer.py:195-200`); compaction usage folded into run totals and spans (`src/agents/memory/openai_responses_compaction_session.py:240-242`; `docs/usage.md:33`).

## Tradeoffs

- **No pre-call measurement ⇒ reactive posture.** The system discovers overflow only after the provider reports it; a long-context request still pays the failed/trimmed call. Mitigated server-side, invisible to non-OpenAI providers using plain Chat Completions, where only the empty-length error exists (`src/agents/models/openai_chatcompletions.py:340-343`).
- **Heterogeneous units.** Tokens (provider usage), bytes÷4 approximations (sandbox), and raw characters (trimmer) coexist without conversion; a "500 chars" budget has different token costs for JSON vs prose (`src/agents/extensions/tool_output_trimmer.py:99-105` vs `src/agents/sandbox/util/token_truncation.py:6`).
- **Hardcoded knowledge.** The context-window table (`src/agents/sandbox/capabilities/compaction.py:25-115`) must track model releases; unknown models fall back to a flat 240k static threshold or raise on `for_model` (`118-133`).
- **Auto-compaction blocks completion.** Documented explicitly: streaming iterators stay open until heavy compaction finishes (`docs/sessions/index.md:285-291`), with manual-trigger escape hatch shown in docs and supported via `should_trigger_compaction=lambda _: False` (`src/agents/memory/openai_responses_compaction_session.py:130-134`).
- **Beta-gated handoff compression.** `nest_handoff_history` defaults off pending stabilization (`src/agents/run_config.py:374-381`), so most multi-agent runs ship full transcripts unless users opt in or write filters.

## Failure Modes / Edge Cases

- **Restore-of-last-resort:** if both replacement *and* restore fail, prior history stays lost and only a warning is logged (`src/agents/memory/openai_responses_compaction_session.py:360-384`; docs acknowledge unrestored history at `docs/sessions/index.md:289`).
- **Concurrent mutation during in-flight compaction:** mutations can complete while the remote compact call runs and then be overwritten by successful replacement — documented race requiring caller discipline (`docs/sessions/index.md:289`; lock covers wrapper ops at `src/agents/memory/openai_responses_compaction_session.py:412-436`).
- **Orphaned IDs after reasoning stripping:** compacted output retaining assistant message IDs without paired reasoning items causes Responses 400s; handled by stripping (`src/agents/memory/openai_responses_compaction_session.py:458-484`; regression noted at `docs/release.md:170`).
- **Legacy summary parsing hazards:** bare-role records and separator-less lines from older summaries are recovered conservatively to avoid fabricating message content (`src/agents/handoffs/history.py:529-570, 615-620`).
- **Trimmer no-op guards:** trimming is skipped whenever the summary would equal/exceed the original size, preventing pathological growth (`src/agents/extensions/tool_output_trimmer.py:265-266, 407-408`).
- **Unknown-model compaction requests fail fast:** `CompactionModelInfo.for_model` raises `ValueError` for models outside the table (`src/agents/sandbox/capabilities/compaction.py:128-133`).

## Future Considerations

- Introduce a client-side rendered-context estimator (even approximate) consulted in `maybe_filter_model_input` (`src/agents/run_internal/turn_preparation.py:51-93`) so trimming thresholds can be expressed in tokens and triggered proactively rather than reactively.
- Unify the scattered knobs behind a single per-agent/per-model `ContextBudget` object that coordinates `SessionSettings.limit`, trimmer sizes, and a compaction trigger based on estimated size instead of item counts (`DEFAULT_COMPACTION_THRESHOLD = 10`, `src/agents/memory/openai_responses_compaction_session.py:28`).
- Externalize the model context-window table (e.g., config or provider metadata) instead of compiling literals (`src/agents/sandbox/capabilities/compaction.py:25-115`).
- Extend the deferred-compaction pattern's guarantees to cover the concurrent-mutation-during-inflight-compact window currently left to caller discipline (`docs/sessions/index.md:289`).
- Graduate `nest_handoff_history` from opt-in beta once stabilized, since full-transcript handoffs remain the silent default for multi-agent runs (`src/agents/run_config.py:374-381`).

## Questions / Gaps

- **No evidence found of any client-side tokenizer** (tiktoken or similar): searched `token`, `tokenizer`, `tiktoken`, `encoding` across `src/`; only byte-ratio approximation exists (`src/agents/sandbox/util/token_truncation.py:6`).
- **No evidence found of prompt/priority ranking across context sections** (e.g., system > tools > recent messages weighting): searched `priority`, `rank`, `importance` in `src/agents/`; ordering is positional (history order + recency boundary), not score-based. The closest analogues are the trimmer's text-over-opaque preference (`src/agents/extensions/tool_output_trimmer.py:288-332`) and handoff verbatim-vs-summary partitioning (`src/agents/handoffs/history.py:633-653`).
- **Faithfulness of `responses.compact` output is unverifiable locally**: no checksumming, coverage metrics, or validation beyond replayability normalization was found; whether summaries preserve critical facts depends entirely on the server endpoint.
- Realtime and voice stacks handle audio-item truncation (`conversation.item.truncate`, `src/agents/realtime/openai_realtime.py:1100-1116, 2142`) but expose no text-token budgeting; they were treated as out of scope for this dimension beyond noting their absence.

---

Generated by dimension `11.02: Token Budgeting and Compression` against `openai-agents-sdk`.
