# Source Analysis: agent-framework

## Dimension 05.06: Memory Compression and Summarization

### Source Info

| Field | Value |
|-------|-------|
| Name | agent-framework |
| Path | `studies/agent-harness-study/sources/agent-framework` |
| Language / Stack | Multi-language monorepo: Python (`python/packages/core`, primary), .NET (`dotnet/src/Microsoft.Agents.AI`), Go (stub — `go/README.md:1` points to a separate repo; no compaction code) |
| Analyzed | 2026-08-25 |

## Summary

Memory compression in agent-framework is implemented as a dedicated **compaction subsystem** with parallel implementations in Python and .NET. The core model is identical in both languages: messages are first partitioned into *atomic groups* (system / user / assistant-text / tool-call, including reasoning prefixes and non-contiguous function-call/result pairs), strategies annotate groups with exclusion flags rather than deleting them, and a projection step decides what the model actually sees. Summarization proper is one strategy among several: Python's `SummarizationStrategy` (`python/packages/core/agent_framework/_compaction.py:1197`) triggers when included non-system message count exceeds `target_count + threshold`, sends whole message groups (bounded by a default 8,000-token input budget) to an explicitly-injected summarizer chat client, replaces the summarized groups with a single assistant-role summary message carrying **bidirectional trace links** (summary → original message/group IDs; originals → summary ID), and keeps raw history in storage with only annotations marking it excluded. The .NET `SummarizationCompactionStrategy` (`dotnet/src/Microsoft.Agents.AI/Compaction/SummarizationCompactionStrategy.cs:48`) is predicate-trigger driven (`CompactionTriggers`: token/message/turn/group thresholds), protects a hard floor of recent groups, rolls back exclusions transactionally if the LLM call fails, and emits OpenTelemetry activities for each summarize operation. Both implementations document a persistent indirect-prompt-injection risk from untrusted summarizers and make LLM summarization strictly opt-in. Deterministic alternatives (`ToolResultCompactionStrategy`) collapse old tool results into capped text summaries without an LLM.

## Rating

**8 / 10**

Rationale: This is a clear, well-specified model with explicit interfaces (`CompactionStrategy` protocol at `python/packages/core/agent_framework/_compaction.py:60-73`; abstract `CompactionStrategy` at `dotnet/src/Microsoft.Agents.AI/Compaction/CompactionStrategy.cs`), extensive unit tests (~90 tests in `python/packages/core/tests/core/test_compaction.py`; 20 summarization tests in `dotnet/tests/Microsoft.Agents.AI.UnitTests/Compaction/SummarizationCompactionStrategyTests.cs`), and operational safeguards: atomic group boundaries, failure rollback/no-mutation guarantees, consecutive-failure escalation logging, token budgets on summarizer input, mid-loop safety (`after_run_once_per_turn`, `_compaction.py:1534`), and regression tests tied to specific issues (#4991, #7011). It falls short of 9–10 because summaries themselves are never evaluated (no quality/drift detection beyond traceability metadata), there is no built-in regeneration API, the default tokenizer is a crude 4-chars/token heuristic, Python observability is limited to log lines while .NET has structured telemetry, and the .NET implementation is still marked `[Experimental]` (`SummarizationCompactionStrategy.cs:47`).

## Evidence Collected

Every entry cites workspace-relative paths into `sources/agent-framework`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Compaction module | All grouping, annotation, strategies, provider live in one module | `python/packages/core/agent_framework/_compaction.py:1-1773` |
| Strategy interface | `CompactionStrategy` runtime-checkable Protocol: async `__call__(messages) -> bool`, mutates annotations/list in place | `python/packages/core/agent_framework/_compaction.py:60-73` |
| Atomic group model | `group_messages` computes spans of kind system/user/assistant_text/tool_call | `python/packages/core/agent_framework/_compaction.py:200-325` |
| Non-contiguous tool pairing | Union-find links function-call declarations to distant results under one group id | `python/packages/core/agent_framework/_compaction.py:105-122,150-198` |
| Reasoning atomicity | Reasoning-only assistant messages join adjacent tool_call groups | `python/packages/core/agent_framework/_compaction.py:256-297`; spec note in `python/packages/core/tests/core/test_compaction.py:175-208` |
| Incremental annotation | `annotate_message_groups` re-annotates only new suffix; expands to unique call declaration on late-arriving result | `python/packages/core/agent_framework/_compaction.py:543-624`; incremental append helpers at `:676-702` |
| Token budget logic | `annotate_token_counts` + `CharacterEstimatorTokenizer` (4 chars/token) | `python/packages/core/agent_framework/_compaction.py:76-80,648-673` |
| Exclusion-as-annotation | `set_excluded` writes `_excluded`/`_exclude_reason`; projection filters via `project_included_messages` | `python/packages/core/agent_framework/_compaction.py:33-34,705-737` |
| Summarization trigger | Fires when included non-system message count > `target_count + threshold` (defaults 4+2) | `python/packages/core/agent_framework/_compaction.py:1223-1241,1311-1312` |
| Summary prompt | `DEFAULT_SUMMARIZATION_PROMPT`: ≤5 sentences, preserve both parties' contributions, incorporate prior summary, no speculation, never omit earlier-summary details | `python/packages/core/agent_framework/_compaction.py:1176-1191` |
| Summarizer input budget | `DEFAULT_SUMMARY_INPUT_TOKEN_BUDGET = 8_000`; whole-group selection via `_select_summary_input_groups`; oversized first group skipped | `python/packages/core/agent_framework/_compaction.py:1138-1173,1193`; tests `python/packages/core/tests/core/test_compaction.py:879-945` |
| Raw→summary links | Forward: `SUMMARY_OF_MESSAGE_IDS_KEY`/`SUMMARY_OF_GROUP_IDS_KEY`; backward: `SUMMARIZED_BY_SUMMARY_ID_KEY` | `python/packages/core/agent_framework/_compaction.py:35-37,1373-1392` |
| Summary insertion point | Summary inserted at index of first summarized group, then incrementally re-annotated | `python/packages/core/agent_framework/_compaction.py:1394-1396` |
| Failure behavior (Python) | Exception or empty text → warn, mutate nothing, return False; escalation ERROR after 3 consecutive failures, reset on success | `python/packages/core/agent_framework/_compaction.py:1273-1288,1348-1372`; tests `python/packages/core/tests/core/test_compaction.py:975-1067` |
| Rolling refresh | Prior summaries remain as included groups and the prompt mandates incorporating them → implicit rolling re-summarization; no separate regenerate API exists | `python/packages/core/agent_framework/_compaction.py:1183,1290-1325` |
| Deterministic tool summaries | `ToolResultCompactionStrategy` collapses old tool groups into `[Tool results: name: value]` text, 4096-char cap + `[truncated]` marker, same bidirectional linking | `python/packages/core/agent_framework/_compaction.py:969-1101` |
| Composition & fallback | `TokenBudgetComposedStrategy`: ordered strategies until budget met, deterministic eviction fallback incl. strict anchor-exclusion phase | `python/packages/core/agent_framework/_compaction.py:1400-1472` |
| Context-window strategy | Two-phase pipeline: tool eviction at 50% and truncation at 80% of input budget (`context_window − max_output`) | `python/packages/core/agent_framework/_compaction.py:1624-1736` |
| Chat-client wiring | `_prepare_messages_for_model_call` runs compaction per model call on the caller's list so inserted summaries survive the function loop (issue #4991 comment) | `python/packages/core/agent_framework/_clients.py:366-394` |
| Provider wiring | `CompactionProvider.before_run` compacts loaded context; `.after_run` compacts persisted session history in place; excluded messages kept in storage ("annotations are preserved") | `python/packages/core/agent_framework/_compaction.py:1494-1621` |
| Load-time filtering | History providers' `skip_excluded` flag omits `_excluded` messages on reload | `python/packages/core/agent_framework/_sessions.py:2112-2149,2267,2302` |
| Mid-loop safety | `after_run_once_per_turn = True` defers persisted-history rewrite to end of user turn (comment explains loop-transcript hazard); consumed in agent run/loop | `python/packages/core/agent_framework/_compaction.py:1532-1534`; `python/packages/core/agent_framework/_agents.py:592-600`; `python/packages/core/agent_framework/_harness/_loop.py:485-493` |
| Harness integration | `_assemble_compaction`: before-strategy wired as agent `compaction_strategy` chat option (before_run hook would be a no-op under per-service-call persistence); shared `ContextWindowCompactionStrategy` default for both phases | `python/packages/core/agent_framework/_harness/_agent.py:82-142`; regression test `python/packages/core/tests/core/test_harness_agent.py:373-428` (issue #7011) |
| Cross-turn coherence | Regression test proving per-call exclusion leakage onto shared Message objects doesn't corrupt stored history | `python/packages/core/tests/core/test_harness_agent.py:431-441` |
| Public exports | `CompactionProvider`, `SummarizationStrategy`, `apply_compaction`, trace-link keys exported from root package | `python/packages/core/agent_framework/__init__.py:72-83,430,549,606` |
| .NET summarization strategy | Trigger-predicate driven; `MinimumPreservedGroups` hard floor (default 8); marks groups then single LLM call; inserts `[Summary]` assistant message | `dotnet/src/Microsoft.Agents.AI/Compaction/SummarizationCompactionStrategy.cs:95-223` |
| .NET rollback on failure | On LLM exception all freshly excluded groups restored, no summary inserted; cancellation propagates without restore | `dotnet/src/Microsoft.Agents.AI/Compaction/SummarizationCompactionStrategy.cs:196-208`; tests `dotnet/tests/Microsoft.Agents.AI.UnitTests/Compaction/SummarizationCompactionStrategyTests.cs:420,455,493,576` |
| .NET trigger library | `TokensExceed`, `MessagesExceed`, `TurnsExceed`, `GroupsExceed`, `HasToolCalls`, `All`/`Any` combinators | `dotnet/src/Microsoft.Agents.AI/Compaction/CompactionTriggers.cs:42-133` |
| .NET coverage metrics | Index exposes Included/Total token, byte, message, group, turn counts; `RawMessageCount` excludes summary-kind groups | `dotnet/src/Microsoft.Agents.AI/Compaction/CompactionMessageIndex.cs:319-374` |
| .NET summary marker | `_is_summary` additional-properties key identifies summary groups (no id-level bidirectional links) | `dotnet/src/Microsoft.Agents.AI/Compaction/CompactionMessageGroup.cs:40`; set at `dotnet/src/Microsoft.Agents.AI/Compaction/SummarizationCompactionStrategy.cs:216` |
| .NET telemetry | ActivitySource names `compaction.compact`/`provider.invoke`/`summarize`; tags before/after tokens/messages/groups, duration, groups_summarized, summary_length | `dotnet/src/Microsoft.Agents.AI/Compaction/CompactionTelemetry.cs:20-44`; used at `SummarizationCompactionStrategy.cs:186-212` |
| .NET persistence | CompactionProvider persists `CompactionMessageIndex` groups into session state; attributes generated summaries as ChatHistory source; skips remote-service-managed sessions | `dotnet/src/Microsoft.Agents.AI/Compaction/CompactionProvider.cs:145-190` |
| .NET harness wiring | `HarnessAgentOptions.CompactionStrategy`/`DisableCompaction`; custom strategy wins over token params; bridge to `IChatReducer` via `AsChatReducer()` | `dotnet/src/Microsoft.Agents.AI.Harness/HarnessAgent.cs:200-230`; `dotnet/src/Microsoft.Agents.AI/Compaction/ChatStrategyExtensions.cs:32-36` |
| Security documentation | Persistent indirect-prompt-injection warning for untrusted summarizers in class docstrings, sample README, and .NET remarks | `python/packages/core/agent_framework/_compaction.py:1207-1216`; `python/samples/02-agents/compaction/README.md:27-36`; `dotnet/src/Microsoft.Agents.AI/Compaction/SummarizationCompactionStrategy.cs:36-45` |
| Samples | End-to-end usage incl. expected output showing trace links | `python/samples/02-agents/compaction/summarization.py:86-119`; composed pipeline in `python/samples/02-agents/compaction/advanced.py:156` |
| Changelog corroboration | "Bound summarization input before provider calls (#7375)" confirms active maintenance of the input budget | `python/CHANGELOG.md:115` |

## Answers to Dimension Questions

**1. When does summarization happen?**
Three distinct execution sites. (a) *Per model call*: the chat client applies the configured `compaction_strategy` inside `BaseChatClient.get_response` via `_prepare_messages_for_model_call` (`python/packages/core/agent_framework/_clients.py:366-394`) — this is where the harness wires its default `ContextWindowCompactionStrategy`. (b) *Pre-run*: `CompactionProvider.before_run` compacts context loaded by earlier providers (`python/packages/core/agent_framework/_compaction.py:1565-1590`). (c) *Post-turn*: `CompactionProvider.after_run` compacts the history provider's persisted session state (`:1592-1621`), deliberately deferred until the user turn ends (`after_run_once_per_turn = True`, `:1534`). Within `SummarizationStrategy`, the trigger itself is a message-count condition: included non-system messages > `target_count + threshold` (default 6) (`:1311`). The .NET variant generalizes the trigger into injectable predicates over coverage metrics (`dotnet/src/Microsoft.Agents.AI/Compaction/CompactionTriggers.cs:42-133`) checked by the base-class run loop.

**2. What evidence does the summary cover?**
The oldest included non-system groups, selected as *whole* atomic groups until adding the next group would exceed `max_summary_input_tokens` (default 8,000 tokens, `python/packages/core/agent_framework/_compaction.py:1193,1334-1339`). Groups that don't fit are left verbatim, even if that means the oldest oversized group is skipped entirely (tested at `python/packages/core/tests/core/test_compaction.py:917-945`). System messages are never summarized (Python filter `:1298-1300`; .NET skip at `dotnet/src/Microsoft.Agents.AI/Compaction/SummarizationCompactionStrategy.cs:155`). The transcript sent to the summarizer is formatted as numbered `[role] text` lines (`_format_messages_for_summary`, `:1124-1135`); non-text contents degrade to their type names (`:1126-1127`), which is lossy for multimodal content. Coverage intent is encoded in the prompts: Python requires reflecting both parties, preserving dialogue context, incorporating any previous summary, and omitting nothing from earlier summaries (`:1176-1191`); .NET asks for key facts, decisions, user preferences, and tool-call outcomes (`dotnet/src/Microsoft.Agents.AI/Compaction/SummarizationCompactionStrategy.cs:53-62`).

**3. Can summary drift be detected?**
Only structurally, not semantically. Every summary carries forward links to the exact original message IDs and group IDs, and every replaced original carries a back-link to its summary ID (`python/packages/core/agent_framework/_compaction.py:35-37,1056-1077,1373-1392`), asserted by tests (`test_summarization_strategy_adds_bidirectional_trace_links`, `python/packages/core/tests/core/test_compaction.py:848-877`; `test_tool_result_compaction_bidirectional_tracing`, `:1356`). This makes drift auditable — you can always reconstruct which raw content a summary stands for — but there is **no evidence found** of any automated evaluation of summary fidelity, staleness detection, or comparison of summary against linked originals. Searches for evaluation/regeneration/quality logic across `python/packages/core` and `dotnet/src` returned nothing.

**4. Is raw history retained?**
Yes — this is a central design decision. Compaction only annotates: `_excluded`/`_exclude_reason` flags plus summary link keys are written to `message.additional_properties` (`python/packages/core/agent_framework/_compaction.py:33-34,718-724,1390-1392`), and `CompactionProvider.after_run` explicitly keeps excluded messages in storage "so annotations are preserved" (`:1619-1621`); loading-side `skip_excluded` controls whether they re-enter context (`python/packages/core/agent_framework/_sessions.py:2124-2149`). In .NET, `GetAllMessages()` vs `GetIncludedMessages()` preserves the same distinction (`dotnet/src/Microsoft.Agents.AI/Compaction/CompactionMessageIndex.cs:307-314`), and the provider persists full group state in session storage (`dotnet/src/Microsoft.Agents.AI/Compaction/CompactionProvider.cs:176-178`). The tradeoff is that persisted transcripts grow monotonically; only the projected model input shrinks.

**5. Can summaries be regenerated?**
Not through any dedicated API — **no regenerate/re-summarize endpoint or method was found** in either language (searches for `regenerat.*summar` across core sources returned nothing). Regeneration is possible *in principle* because raw originals persist alongside back-links, but nothing in the code exercises that path. What does exist is *rolling refresh by accretion*: prior summaries remain included groups, future summarization rounds include their text in the summarizer transcript, and the default prompt forbids dropping details from earlier summaries (`python/packages/core/agent_framework/_compaction.py:1183,1190`); the advanced sample demonstrates a second-round summary whose input includes the first summary (`python/samples/02-agents/compaction/advanced.py:147,203`).

## Architectural Decisions

- **Exclusion-as-annotation over deletion** — compaction mutates flags on immutable-position messages and projects at read time (`python/packages/core/agent_framework/_compaction.py:718-737`), keeping raw history durable and making every compression reversible-by-data.
- **Atomic group boundaries as the unit of compression** — tool calls, their (possibly non-contiguous) results, and attached reasoning are unioned into single groups (`:150-198,262-297`) so no strategy can split a call/result pair; this contract is heavily tested and mirrored in the .NET `CompactionMessageIndex` (`dotnet/src/Microsoft.Agents.AI/Compaction/CompactionMessageIndex.cs:88`).
- **LLM summarization is strictly opt-in and security-documented** — unlike drop-only strategies, `SummarizationStrategy` must be constructed with a trusted client; the injection risk is spelled out identically in three places (`python/packages/core/agent_framework/_compaction.py:1207-1216`, `python/samples/02-agents/compaction/README.md:27-36`, `dotnet/src/Microsoft.Agents.AI/Compaction/SummarizationCompactionStrategy.cs:36-45`).
- **Mark-after-success (Python) vs mark-then-roll-back (.NET)** — Python performs zero mutation until the summarizer returns text (`_compaction.py:1367-1395`); .NET excludes groups up front for target checking and restores them in a catch block (`SummarizationCompactionStrategy.cs:168-177,196-208`). Both guarantee no partial state.
- **Layered execution sites** — the same strategy interface runs per-model-call (chat client option), pre-run (provider), and post-turn (provider on persisted state), letting hosts choose between transient per-call bounding and durable transcript rewriting (`python/packages/core/agent_framework/_harness/_agent.py:92-142`).
- **Deterministic-first defaults** — the harness/token-budget default pipeline prefers safe operations (tool-result collapsing, truncation) and treats LLM summarization as a composable, optional member (`python/packages/core/agent_framework/_compaction.py:1400-1472,1624-1736`).

## Notable Patterns

- **Bidirectional provenance links** (`SUMMARY_OF_MESSAGE_IDS_KEY` ↔ `SUMMARIZED_BY_SUMMARY_ID_KEY`) turn summaries into auditable projections rather than opaque replacements (`python/packages/core/agent_framework/_compaction.py:35-37`).
- **Incremental annotation with gap detection**: `_first_annotation_gaps` finds the first unannotated/untokenized suffix and re-annotates only from there, walking back to group boundaries and unique call declarations (`:474-540,556-586`) — O(new work) per turn instead of O(history).
- **Budget-aware whole-group selection**: the summarizer request is bounded by prompt+transcript token estimation that never splits a group and never retokenizes already-counted transcript text (`:1138-1173`, test `python/packages/core/tests/core/test_compaction.py:948-972`).
- **Failure escalation ladder**: warnings per failure, a single ERROR after `SUMMARY_FAILURE_ERROR_THRESHOLD = 3` consecutive failures, reset on success (`python/packages/core/agent_framework/_compaction.py:1194,1273-1288`) — avoids alert fatigue while surfacing silent degradation.
- **Adapter bridges to host ecosystems**: .NET wraps any strategy as an `IChatReducer` (`dotnet/src/Microsoft.Agents.AI/Compaction/ChatStrategyExtensions.cs:32-54`); Python exposes `apply_compaction` for standalone use (`_compaction.py:1475-1488`).
- **Trigger/target symmetry (.NET)**: `target` defaults to the inverse of the trigger so compaction stops exactly when the trigger condition clears (`SummarizationCompactionStrategy.cs:91-94,172-176`).

## Tradeoffs

- **Durability vs storage growth**: retaining raw history plus annotations means session stores grow without bound even as model input stays bounded (`python/packages/core/agent_framework/_compaction.py:1619-1621`). Auditability is bought with bytes.
- **Count-based vs token-based triggers**: `SummarizationStrategy`'s default trigger counts messages, not tokens, so a few huge messages can overflow long before the count trips; token awareness exists elsewhere (`TruncationStrategy` with tokenizer, `ContextWindowCompactionStrategy`) but not in the summarization trigger itself (`:1223-1241`).
- **Heuristic default tokenizer**: `CharacterEstimatorTokenizer` (len//4) miscounts CJK and JSON-heavy payloads despite the `ensure_ascii=False` mitigation (`:76-80,643-645`); real-tokenizer support is pluggable but on the caller (`tiktoken_tokenizer.py` sample).
- **Lossy transcript formatting**: non-text contents become bare type names in the summarizer input (`:1126-1127`), so image/tool-schema information can silently vanish from summaries.
- **Observability asymmetry**: .NET records OTel spans with before/after token, message, group tags and duration (`dotnet/src/Microsoft.Agents.AI/Compaction/CompactionTelemetry.cs:20-44`); Python logs warnings/errors only — harder to operate at scale.
- **Provenance asymmetry**: Python's summaries carry full ID-level links; .NET summaries carry only an `_is_summary` marker (`dotnet/src/Microsoft.Agents.AI/Compaction/CompactionMessageGroup.cs:40`), weakening drift auditing there.

## Failure Modes / Edge Cases

- **Summarizer outage degrades gracefully but visibly**: failures leave history untouched (`changed=False`) and escalate to a single ERROR log after 3 consecutive failures; meanwhile nothing compresses, so context keeps growing toward provider limits (`python/packages/core/agent_framework/_compaction.py:1273-1288,1359-1365`; tests `python/packages/core/tests/core/test_compaction.py:975-1045`).
- **Empty summarizer output treated as failure**: no summary inserted, nothing excluded (`:1367-1371`; .NET uses a `"[Summary unavailable]"` placeholder instead — `SummarizationCompactionStrategy.cs:210` — a deliberate divergence).
- **Unsummarizable oversized group**: if no complete group fits the input budget, the cycle warns, records a failure, and skips (`:1340-1346`); the offending group remains verbatim (test `python/packages/core/tests/core/test_compaction.py:917-945`).
- **Mid-loop transcript rewrite hazard**: compacting persisted history during an in-flight function-invocation loop would rewrite the list the loop iterates; mitigated by once-per-turn deferral (`_compaction.py:1532-1534`) and by mutating the caller's list in place so inserted summaries survive iterations (issue #4991 comment, `python/packages/core/agent_framework/_clients.py:383-389`).
- **Shared-object annotation leakage**: per-call compaction writes `_excluded` onto Message objects shared with stored history; correctness relies on reload semantics (default `skip_excluded=False`), pinned by a multi-turn coherence regression test (`python/packages/core/tests/core/test_harness_agent.py:431-441`).
- **Late tool results**: a result arriving after its declaration was already annotated forces re-annotation back to the unique matching declaration, including ambiguity guards for reused `call_id`s (`python/packages/core/agent_framework/_compaction.py:510-540`; tests `:481-550`).
- **Prompt-injection persistence**: a malicious summarizer's output becomes trusted assistant history indefinitely — accepted risk gated behind explicit opt-in and trust guidance (citations above).

## Future Considerations

- Add summary **evaluation/drift detection**: compare regenerated or sampled originals against linked summaries using the existing bidirectional ID graph; nothing currently measures whether a summary preserved decisions/facts/uncertainty beyond prompt instructions.
- Provide a **regeneration API** that replays `SUMMARY_OF_MESSAGE_IDS` targets through the summarizer to rebuild a drifted summary — the retained raw history makes this feasible today with no schema change.
- Make the Python summarization trigger **token-aware** (the .NET `CompactionTriggers.TokensExceed` shows the pattern) instead of message-count based.
- Port .NET-grade **telemetry** (activity names/tags in `CompactionTelemetry.cs`) to Python, replacing log-only observability.
- Reconcine empty-response behavior between languages: Python skips, .NET inserts a placeholder (`"[Summary unavailable]"`) — one of these is likely wrong for downstream consumers.
- Promote .NET compaction out of `[Experimental]` once the rollback/failure suite (already 20 tests) soaks in production use.

## Questions / Gaps

- **No summary-quality evaluation found.** Searched `python/packages/core`, `python/samples`, `dotnet/src` for evaluation/quality/scoring of summaries; only traceability metadata exists. Question 3's drift detection is therefore structural only.
- **No regeneration path found** (searched `regenerat` across core sources). Raw retention enables it, but no code implements it.
- **Uncertainty preservation unverified.** The dimension asks whether compression preserves uncertainty; neither default prompt mentions confidence/open questions (Python prompt bans speculation outright, `python/packages/core/agent_framework/_compaction.py:1186-1189`), and no evidence shows unresolved items being tracked distinctly. No clear evidence found either way beyond the prompt text.
- **Multimodal content in summaries**: formatting collapses non-text contents to type names (`:1126-1127`); whether any adapter serializes richer input for the summarizer was not found — likely a genuine gap.
- **Go implementation**: absent from this repository (`go/README.md:1` defers to `microsoft/agent-framework-go`); out of scope here, noted for study completeness.
- **Scale behavior**: incremental annotation bounds per-turn cost, but no benchmarks or load tests for very long histories were found inside the source tree.

---

Generated by `Dimension 05.06: Memory Compression and Summarization` against `agent-framework`.
