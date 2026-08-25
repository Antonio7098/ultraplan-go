# Source Analysis: openai-agents-sdk

## Context Refresh Inside the Loop

### Source Info

| Field | Value |
|-------|-------|
| Name | openai-agents-sdk |
| Path | `studies/agent-harness-study/sources/openai-agents-sdk` |
| Language / Stack | Python; OpenAI Agents SDK; async and streaming orchestration |
| Analyzed | 2026-07-15 |

## Summary

The source uses a hybrid context model. Within a run, each model turn reconstructs model input from the caller/session input plus the accumulated run items rather than mutating one opaque prompt buffer (`src/agents/run_internal/run_loop.py:280-287`, `src/agents/run.py:1284-1290`). Dynamic instructions, prompts, handoffs, and tools are resolved again for each model turn, so mutable application state and MCP/tool availability can refresh inside the loop (`src/agents/run_internal/run_loop.py:1751-1761`, `src/agents/agent.py:224-266`).

Long-history control is explicit but opt-in. A session can load only the latest N items or apply a custom merge callback (`src/agents/run_internal/session_persistence.py:81-88`, `src/agents/run_internal/session_persistence.py:100-169`); a pre-call filter can rewrite instructions and input on every call (`src/agents/run_internal/turn_preparation.py:51-85`); and the built-in tool-output trimmer replaces oversized old tool results with marked previews while preserving recent turns (`src/agents/extensions/tool_output_trimmer.py:90-155`). OpenAI-specific paths add provider-managed truncation and compaction settings (`src/agents/model_settings.py:120-123`, `src/agents/model_settings.py:187-192`), a compaction-aware session that replaces stored history with `responses.compact` output (`src/agents/memory/openai_responses_compaction_session.py:160-234`), and server-conversation delta tracking that sends only unsent items (`src/agents/run_internal/oai_conversation.py:417-510`).

The main limitation is that the generic runner has no default preflight token budget, no generic context-overflow recovery, and no mandatory compaction policy. Token usage is recorded only after model responses (`src/agents/run_internal/run_loop.py:1909-1918`), while unrestricted sessions retrieve all history by default (`src/agents/run_internal/session_persistence.py:85-88`). Consequently, the loop can continue beyond a context window only when the application or provider enables a session limit, pre-call filter, provider truncation, server compaction, or a compaction-aware session.

## Rating

**7/10 — Clear model with tests and operational safeguards, but not safe by default across providers.**

The score earns the 7–8 band because context assembly has explicit interfaces, streaming and non-streaming paths apply the same pre-call filter (`src/agents/run_internal/run_loop.py:1370-1405`, `src/agents/run_internal/run_loop.py:1818-1847`), dynamic tool refresh is tested in both paths (`tests/test_agent_runner.py:4423-4455`, `tests/test_agent_runner_streamed.py:1633-1666`), and compaction replacement has rollback coverage (`src/agents/memory/openai_responses_compaction_session.py:242-306`, `tests/memory/test_openai_responses_compaction_session.py:486-544`). It does not reach 8–10 because generic overflow detection, token-aware pruning priorities, compaction observability, and summary-faithfulness checks are absent; the default path can still grow without bound (`src/agents/run_internal/session_persistence.py:85-88`).

## Evidence Collected

Every entry includes a file path with line numbers.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Per-turn assembly | Converts caller input and cumulative generated run items into a fresh normalized model-input list for each call. | `src/agents/run_internal/run_loop.py:280-287` |
| Loop state update | After a turn, replaces continuation state with prior step items plus new step items while separately retaining unfiltered session items. | `src/agents/run.py:1284-1290` |
| Dynamic context refresh | Resolves system instructions and prompt immediately before each non-streaming model call. | `src/agents/run_internal/run_loop.py:1751-1754` |
| Tool/retrieval surface refresh | Fetches MCP tools and reevaluates callable tool enablement whenever the loop asks for all tools. | `src/agents/agent.py:224-266` |
| Session context builder | Loads latest-N or all history, merges new input, removes orphan calls, normalizes, and deduplicates. | `src/agents/run_internal/session_persistence.py:54-187` |
| Pre-call context hook | Accepts synchronous or asynchronous input rewriting and validates the returned model-input object. | `src/agents/run_internal/turn_preparation.py:51-85` |
| Stale/duplicate cleanup | Drops orphan tool calls and their tied reasoning items, strips SDK metadata, and keeps the latest duplicate stable ID/call ID. | `src/agents/run_internal/items.py:98-179`, `src/agents/run_internal/items.py:297-346` |
| Tool-result trimming | Protects the most recent user turns, trims only older oversized tool outputs, and inserts a visible preview marker. | `src/agents/extensions/tool_output_trimmer.py:105-155`, `src/agents/extensions/tool_output_trimmer.py:157-220` |
| Token accounting | Aggregates input/output/total tokens and retains per-request usage entries, but records observed usage rather than predicting the next request size. | `src/agents/usage.py:102-136`, `src/agents/usage.py:157-215` |
| Provider compaction configuration | Exposes Responses context-management compaction thresholds and passes them to the request. | `src/agents/model_settings.py:187-192`, `src/agents/models/openai_responses.py:831-856` |
| Compaction-aware session | Triggers on candidate count or a custom decision, calls `responses.compact`, then replaces stored history with normalized compacted output. | `src/agents/memory/openai_responses_compaction_session.py:54-56`, `src/agents/memory/openai_responses_compaction_session.py:185-234` |
| Compaction failure safeguard | Restores previous session history if clearing or writing compacted replacement data fails. | `src/agents/memory/openai_responses_compaction_session.py:242-306` |
| Server-managed deltas | Tracks IDs, call IDs, object identities, and fingerprints to suppress already acknowledged items across retries and resumes. | `src/agents/run_internal/oai_conversation.py:98-139`, `src/agents/run_internal/oai_conversation.py:417-510` |
| Compaction visibility | Converts provider compaction output into a replayable run item but deliberately emits no public stream event for it. | `src/agents/run_internal/turn_resolution.py:1717-1728`, `tests/test_stream_events.py:238-255` |
| Context tracing | Turn spans include per-turn token usage; OpenAI response spans can retain the exact final model input when sensitive-data tracing is enabled. | `src/agents/run_internal/run_loop.py:1002-1045`, `src/agents/models/openai_responses.py:489-506` |
| Session-limit tests | Verifies latest-N, zero-history, unlimited-history, and oversized-limit behavior for async, sync, and streaming runners. | `tests/memory/test_session_limit.py:15-60`, `tests/memory/test_session_limit.py:65-137` |
| Compaction integration tests | Verifies runner-triggered compaction, deferral around local tool outputs, later forced compaction, and persistence across tool turns. | `tests/memory/test_openai_responses_compaction_session.py:1028-1197` |

## Answers to Dimension Questions

1. **Is context static or rebuilt each turn?**  
   It is rebuilt for each model invocation from two evolving collections: the normalized original/caller input and accumulated generated items (`src/agents/run_internal/run_loop.py:280-287`). The non-streaming loop updates the accumulated continuation after each response (`src/agents/run.py:1284-1290`), and the streaming loop does the equivalent update (`src/agents/run_internal/run_loop.py:1061-1076`). System instructions and prompt configuration are recomputed per turn (`src/agents/run_internal/run_loop.py:1338-1345`, `src/agents/run_internal/run_loop.py:1751-1758`), as are tools and MCP-derived functions (`src/agents/run.py:1030-1033`, `src/agents/agent.py:224-266`). Session storage is read when a run is prepared, not on every inner turn (`src/agents/run.py:526-547`, `src/agents/run.py:767-768`), so external session changes during one multi-turn run are not automatically merged.

2. **What gets dropped first?**  
   There is no single global priority policy. With a session limit, the store returns the latest N items, so oldest history is omitted first (`src/agents/memory/session.py:24-30`, `tests/memory/test_session_limit.py:42-60`). With the built-in trimmer, only oversized function/tool-search outputs before the recent-turn boundary are replaced, while recent turns and small outputs remain intact (`src/agents/extensions/tool_output_trimmer.py:105-147`, `tests/extensions/test_tool_output_trimmer.py:172-221`). The sandbox compaction capability drops everything before the latest compaction item (`src/agents/sandbox/capabilities/compaction.py:206-220`). Provider `truncation="auto"` is exposed, but its exact removal order is not implemented or documented in this source (`src/agents/model_settings.py:120-123`, `src/agents/models/openai_responses.py:831-855`).

3. **Is summarization faithful?**  
   Handoff “summarization” is deterministic transcript serialization into one assistant message rather than semantic compression: it deep-copies each item and renders every record (`src/agents/handoffs/history.py:71-121`, `src/agents/handoffs/history.py:136-158`). Tests verify multiline and structured content survive flatten/reparse without truncation (`tests/test_extension_filters.py:547-597`). In contrast, session compaction trusts provider-generated compacted output and normalizes its replay shape (`src/agents/memory/openai_responses_compaction_session.py:215-229`, `src/agents/memory/openai_responses_compaction_session.py:404-496`); no source-local comparison, entailment check, or faithfulness score validates that summary against discarded history. Therefore structural handoff collapse is high-fidelity but not token-saving, while semantic compaction faithfulness is delegated and unverified.

4. **Are tool results preserved or summarized?**  
   By default, tool call outputs are converted back to model input and accumulated with other run items (`src/agents/run_internal/items.py:66-95`, `src/agents/run.py:1284-1290`). The built-in trimmer preserves recent outputs exactly and replaces only eligible older large outputs with a marker, original character count, and prefix preview (`src/agents/extensions/tool_output_trimmer.py:196-220`). Compaction-aware sessions may replace raw tool history with provider compacted output, but compaction is deferred when a just-persisted turn contains local tool outputs to avoid compacting an incomplete interaction (`src/agents/run_internal/session_persistence.py:354-387`); this deferral behavior is covered in runner tests (`tests/memory/test_openai_responses_compaction_session.py:1054-1197`).

5. **Can the model tell what context is missing?**  
   Sometimes. The built-in tool-output trimmer inserts an explicit marker with original length and preview size (`src/agents/extensions/tool_output_trimmer.py:208-220`), and sandbox text truncation inserts a removed-token or removed-character marker (`src/agents/sandbox/util/token_truncation.py:151-171`). Provider compaction items remain replayable model input (`src/agents/items.py:491-499`). However, session latest-N omission has no synthetic “older history omitted” item (`src/agents/run_internal/session_persistence.py:85-105`), and an arbitrary pre-call filter can remove context without being required to disclose that fact (`src/agents/run_internal/turn_preparation.py:66-80`). The model therefore cannot reliably infer all missing context.

## Architectural Decisions

- **Separate application context from model-visible history.** The local wrapper explicitly states that its application object is not sent to the model (`src/agents/run_context.py:43-58`); model-visible context is supplied through instructions, input, tools, and retrieval (`docs/context.md:137-144`). This keeps dependency injection independent from prompt budgeting.
- **Use explicit extension points instead of one universal memory policy.** Session merging is configurable before a run (`src/agents/run_config.py:291-296`), while model input can be rewritten immediately before every call (`src/agents/run_config.py:298-306`). This supports provider/application-specific policies but makes safe long-context behavior opt-in.
- **Maintain separate continuation and observability histories.** The loop uses filtered `new_step_items` for future model input but keeps `session_step_items`/unfiltered items for results and persistence (`src/agents/run.py:1284-1290`, `src/agents/result.py:130-158`). This prevents handoff filtering from erasing the audit history while preserving canonical replay.
- **Use deltas when the provider owns conversation history.** The server tracker records stable identifiers and fingerprints, excludes acknowledged items, and marks only post-filter input as sent (`src/agents/run_internal/oai_conversation.py:98-139`, `src/agents/run_internal/run_loop.py:1370-1390`). This reduces repeated payloads and supports retry/resume deduplication.
- **Treat compaction as a first-class replay item.** Compaction output becomes a dedicated run item that can be passed back to the model (`src/agents/items.py:491-499`, `src/agents/run_internal/turn_resolution.py:1717-1728`), while public streaming treats it as internal session bookkeeping (`tests/test_stream_events.py:238-255`).

## Notable Patterns

- **Copy, normalize, prune, deduplicate.** Input lists are shallow-copied between turns (`src/agents/run_internal/items.py:61-63`), normalized for API submission (`src/agents/run_internal/items.py:191-217`), stripped of orphan tool/reasoning pairs (`src/agents/run_internal/items.py:98-179`), and deduplicated with latest stable identity winning (`src/agents/run_internal/items.py:324-346`).
- **Dynamic availability refresh.** Each turn asks for all tools (`src/agents/run.py:1030-1033`), and tool resolution fetches MCP functions plus callable enablement state (`src/agents/agent.py:224-266`). Tests demonstrate that a tool added by one turn is callable on the next in both execution modes (`tests/test_agent_runner.py:4423-4455`, `tests/test_agent_runner_streamed.py:1633-1666`).
- **Visible local truncation.** The tool trimmer and sandbox truncator preserve prefixes and add explicit omission metadata rather than silently slicing content (`src/agents/extensions/tool_output_trimmer.py:208-220`, `src/agents/sandbox/util/token_truncation.py:85-109`).
- **Compaction deferral around side effects.** Local tool outputs cause compaction to be deferred until a later response boundary, then forced if still pending (`src/agents/run_internal/session_persistence.py:354-387`).
- **Rollback on destructive history replacement.** The compaction session snapshots all underlying items, clears, writes the replacement, and attempts restoration on either clear or write failure (`src/agents/memory/openai_responses_compaction_session.py:221-225`, `src/agents/memory/openai_responses_compaction_session.py:242-306`).

## Tradeoffs

- **Extensibility versus safe defaults.** The pre-call filter can implement any token policy (`src/agents/run_config.py:298-306`), but without one the runner sends all accumulated history (`src/agents/run_internal/run_loop.py:280-287`) and default session preparation loads all stored items (`src/agents/run_internal/session_persistence.py:85-88`).
- **Observed usage versus predictive budgeting.** Per-request token records enable cost and trend analysis (`src/agents/usage.py:125-136`), but they arrive after a request and are not used by the generic loop to size the next input (`src/agents/run_internal/run_loop.py:1909-1918`).
- **Cheap character estimates versus tokenizer accuracy.** The built-in tool trimmer uses character thresholds (`src/agents/extensions/tool_output_trimmer.py:66-69`, `src/agents/extensions/tool_output_trimmer.py:202-220`), and sandbox token truncation approximates one token per four bytes (`src/agents/sandbox/util/token_truncation.py:6-7`, `src/agents/sandbox/util/token_truncation.py:174-186`). Both are deterministic and inexpensive but can misestimate model-specific tokenization.
- **Candidate-count compaction versus actual context pressure.** The compaction-aware session defaults to ten non-user/non-compaction candidate items (`src/agents/memory/openai_responses_compaction_session.py:24-56`), so a few huge items may not trigger while many tiny items may. A custom trigger can improve this, but the SDK provides no default rendered-token calculation (`src/agents/memory/openai_responses_compaction_session.py:86-110`).
- **Delta efficiency versus hidden state.** Server-managed conversations send only unsent deltas (`src/agents/run_internal/oai_conversation.py:417-510`), which lowers client payload growth, but the complete context and provider-side compaction decisions are no longer fully represented in each local request.

## Failure Modes / Edge Cases

- **Generic context overflow remains possible.** The runner exposes provider truncation and custom filtering (`src/agents/model_settings.py:120-123`, `src/agents/run_config.py:298-306`) but has no source-local context-length exception path that compacts and retries; ordinary model errors are traced and re-raised (`src/agents/models/openai_responses.py:507-518`).
- **Session limits can create silent semantic gaps.** Latest-N retrieval is deterministic (`src/agents/memory/session.py:24-30`) and thoroughly tested (`tests/memory/test_session_limit.py:15-137`), but no omission marker is added during assembly (`src/agents/run_internal/session_persistence.py:85-105`).
- **Concurrent session updates can be stale inside one run.** Session history is loaded during initial run preparation (`src/agents/run.py:526-547`), after which the inner loop uses local `original_input` and `generated_items` state (`src/agents/run.py:767-768`, `src/agents/run.py:1284-1290`). There is no per-turn session reread in that path.
- **Compaction output can be malformed.** Invalid compacted image/file shapes raise before stored history replacement (`src/agents/memory/openai_responses_compaction_session.py:452-496`); a regression test confirms original history survives normalization failure (`tests/memory/test_openai_responses_compaction_session.py:983-1026`).
- **Destructive replacement recovery is best effort.** Previous history is restored after replacement failure, but restoration failure is only logged and the original replacement exception is re-raised (`src/agents/memory/openai_responses_compaction_session.py:284-306`); this branch is explicitly tested (`tests/memory/test_openai_responses_compaction_session.py:709-762`).
- **Filtered server deltas require precise bookkeeping.** The tracker must mark exactly the post-filter items and rewind them before retry (`src/agents/run_internal/run_loop.py:1382-1390`, `src/agents/run_internal/run_loop.py:1452-1456`). The fingerprint/object-ID machinery reduces duplicates but adds complexity around reconstructed or anonymous items (`src/agents/run_internal/oai_conversation.py:359-415`, `src/agents/run_internal/oai_conversation.py:448-510`).
- **Compaction is not visible as a semantic stream event.** It is intentionally skipped from public item streaming (`tests/test_stream_events.py:238-255`), so streaming consumers cannot directly display or react to a context-compaction transition.

## Future Considerations

- Add a provider-neutral preflight budget interface that estimates the fully rendered request—including instructions and tool schemas—before each model call, then applies deterministic priorities before overflow. The insertion point already exists immediately before the model call (`src/agents/run_internal/turn_preparation.py:51-85`).
- Provide a default adaptive policy using recent per-request input-token measurements (`src/agents/usage.py:125-136`) rather than relying only on item counts (`src/agents/memory/openai_responses_compaction_session.py:54-56`) or fixed character thresholds (`src/agents/extensions/tool_output_trimmer.py:66-69`).
- Emit a dedicated context-change trace record containing before/after item counts, estimated tokens, dropped item classes, trigger reason, and compaction response ID. Current turn spans expose usage (`src/agents/tracing/span_data.py:98-132`), and response spans can expose final input (`src/agents/models/openai_responses.py:489-506`), but compaction itself is represented only by logs (`src/agents/memory/openai_responses_compaction_session.py:197-234`).
- Add an omission-manifest input item for session limits and arbitrary filters so the model can distinguish “not present” from “never happened.” Current session limiting silently excludes history (`src/agents/run_internal/session_persistence.py:85-105`), whereas local trimmers already demonstrate explicit markers (`src/agents/extensions/tool_output_trimmer.py:208-220`).
- Make destructive session replacement transactional or versioned where the backing store supports it. The current clear-and-add sequence requires best-effort restoration (`src/agents/memory/openai_responses_compaction_session.py:242-306`).
- Define a retrieval-refresh contract that distinguishes tool availability refresh from retrieval-result refresh. MCP/tool definitions are refreshed per turn (`src/agents/agent.py:224-266`), while actual file retrieval remains model-initiated through the hosted file-search tool (`src/agents/tool.py:661-680`).

## Questions / Gaps

- No clear evidence found of a repository-map builder or repository-map refresh policy. The search boundary was the selected source’s `src/agents/**/*.py`; retrieval evidence is limited to hosted file search (`src/agents/tool.py:661-680`), MCP/tool refresh (`src/agents/agent.py:224-266`), and sandbox memory instructions (`src/agents/sandbox/capabilities/memory.py:50-79`).
- No clear evidence found of a provider-neutral tokenizer or exact rendered-request token counter in the standard runner. The closest local utility is the sandbox’s four-bytes-per-token estimator (`src/agents/sandbox/util/token_truncation.py:6-7`, `src/agents/sandbox/util/token_truncation.py:174-186`).
- No clear evidence found that provider compacted summaries are checked for factual preservation against the replaced transcript. Tests cover trigger behavior, normalization, replay safety, and rollback (`tests/memory/test_openai_responses_compaction_session.py:434-544`, `tests/memory/test_openai_responses_compaction_session.py:829-1026`), not semantic faithfulness.
- No clear evidence found of scale tests that run until a real model context limit is crossed and then prove recovery. Existing tests use fake models and mocked compaction responses (`tests/memory/test_openai_responses_compaction_session.py:1028-1197`).
- Provider-side `truncation="auto"` removal order and disclosure semantics cannot be established from this source because the SDK only forwards the setting (`src/agents/model_settings.py:120-123`, `src/agents/models/openai_responses.py:831-855`).

---

Generated by `03.07-context-refresh-inside-the-loop.md` against `openai-agents-sdk`.
