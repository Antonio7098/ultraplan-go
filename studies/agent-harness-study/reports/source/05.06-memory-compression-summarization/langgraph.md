# Source Analysis: langgraph

## 05.06 Memory Compression and Summarization

### Source Info

| Field | Value |
|-------|-------|
| Name | langgraph |
| Path | `studies/agent-harness-study/sources/langgraph` (monorepo: `libs/langgraph`, `libs/prebuilt`, `libs/checkpoint*`, `libs/cli`, `libs/sdk-py`) |
| Language / Stack | Python (plus a JS SDK pointer stub under `libs/sdk-js`) |
| Analyzed | 2026-08-25 |

All citations below are relative to the source root (`studies/agent-harness-study/sources/langgraph/`).

## Summary

LangGraph ships **no built-in summarization node, no summary prompt, no token counter, and no token-budget trigger**. A repo-wide search for `summariz` across all `libs/**/*.py` matches only three docstring/comment sites: the `pre_model_hook` docstring of `create_react_agent` (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:397`), and the `TracePolicy.process_inputs` / `process_outputs` docs (`libs/langgraph/langgraph/types.py:550`, `libs/langgraph/langgraph/types.py:556`). In every case "summarization" is named as something *user-supplied code* may do — the framework provides only the substrate.

That substrate is nonetheless well-engineered and directly aimed at this problem:

1. **A removal-aware messages reducer.** `add_messages` (`libs/langgraph/langgraph/graph/message.py:60-244`) is append-only by default but supports three compression primitives: same-ID overwrite (`message.py:216-225`), tombstone deletion via `RemoveMessage(id=...)` (`message.py:221-222`, with a hard error on unknown IDs at `message.py:227-230`), and an atomic wipe-and-replace via `RemoveMessage(id=REMOVE_ALL_MESSAGES)` (`message.py:38`, `message.py:209-213`) that returns only the entries after the sentinel — exactly the primitive needed to durably swap raw history for a summary.
2. **A sanctioned summarization seam.** `create_react_agent`'s `pre_model_hook` (`chat_agent_executor.py:396-424`) documents the two intended patterns: return `"llm_input_messages"` to compress only what the LLM sees while leaving stored state intact, or return `"messages": [RemoveMessage(id=REMOVE_ALL_MESSAGES), *new]` to durably overwrite history ("where a summary would be written"). The hook node is placed before the model call and `_get_model_input_state` prefers `llm_input_messages` over `messages` when it exists (`chat_agent_executor.py:636-658`).
3. **History lifecycle management at the storage layer.** `BaseCheckpointSaver.prune(thread_ids, strategy="keep_latest"|"delete")` (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:374-415`), `delete_thread` (`base/__init__.py:320-329`), `delete_for_runs` (`base/__init__.py:331-348`), plus platform-level TTL configuration (`ThreadTTLConfig` in `libs/cli/langgraph_cli/schemas.py:109-124`). These manage *retention*, not semantic content.
4. **Lossless storage-side compression.** `DeltaChannel` (`libs/langgraph/langgraph/channels/delta.py:25-202`) stores only a sentinel per step and reconstructs state by replaying ancestor writes, writing full `_DeltaSnapshot` blobs every `snapshot_frequency` updates (default 1000) or after a bounded superstep count (`delta.py:50-64`) — bounding replay depth rather than prompt size.
5. **Observability-only summarization.** `TracePolicy.process_inputs/process_outputs` can "omit or summarize large payloads (e.g. message history)" on traces without affecting execution (`types.py:532-567`).

The in-repo documentation confirms summarization guidance was deliberately moved out of the library: `docs/redirects.json:23-25` redirects `/how-tos/memory/manage-conversation-history`, `/how-tos/memory/delete-messages`, and `/how-tos/memory/add-summary-conversation-history` to external `langchain.com` pages (`...add-memory#summarize-messages`).

Net assessment for this dimension: the *mechanics* of replacing long history with compressed content are explicit, tested, and operationally documented; the *semantics* of summarization (triggers, prompts, coverage, drift detection, evaluation) are entirely absent by design.

## Rating

**4 / 10** — Present but incomplete.

Rationale against the rubric:
- Summarization proper (summary records, summary prompts, coverage ranges, refresh logic) is **absent**: zero implementation classes exist anywhere in `libs/`; the only mentions are docstrings pointing at user code (`chat_agent_executor.py:397`, `types.py:550-558`) and redirects proving the how-tos left the repo (`docs/redirects.json:23-25`). Token-budget logic (`trim_messages`, `count_tokens`, tiktoken) has **no occurrences** in any Python library source; the closest analogue is a loop bound, `remaining_steps ≈ recursion_limit − steps_taken` (`chat_agent_executor.py:434-440`, `chat_agent_executor.py:620-634`), which caps iterations, not context tokens.
- The substrate earns points above the "absent/ad-hoc" band: `RemoveMessage`/`REMOVE_ALL_MESSAGES` semantics are precisely specified and unit-tested (`libs/langgraph/tests/test_messages_state.py:96-145`, `:312-338`); both sanctioned compression patterns are integration-tested (`libs/prebuilt/tests/test_react_agent.py:1924-1954`); pruning has a conformance suite (`libs/checkpoint-conformance/langgraph/checkpoint/conformance/spec/test_prune.py:34-194`); and failure modes of history deletion are explicitly documented with safe strategies (`base/__init__.py:387-413`).
- It stops short of 7-8 because there is no summarization model at all — nothing triggers compression, nothing evaluates whether a rewrite preserved decisions/facts/uncertainty, and one reducer variant actively cannot express the atomic-replace pattern (`_messages_delta_reducer` does not support `REMOVE_ALL_MESSAGES`, `message.py:260-262`).

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Atomic history replacement primitive | `REMOVE_ALL_MESSAGES = "__remove_all__"` sentinel; `add_messages` returns only entries after the sentinel, enabling durable summary rewrites | `libs/langgraph/langgraph/graph/message.py:38`, `:206-213` |
| Tombstone deletion + safety | Same-ID overwrite vs `RemoveMessage` delete; `ValueError` when removing a nonexistent ID | `libs/langgraph/langgraph/graph/message.py:216-234`, `:227-230` |
| Default retention policy | Reducer is append-only unless IDs collide — raw history grows unbounded by default | `libs/langgraph/langgraph/graph/message.py:67-70` |
| Batch delta reducer limitation | `_messages_delta_reducer` handles dedup/tombstones but "does not support REMOVE_ALL_MESSAGES" | `libs/langgraph/langgraph/graph/message.py:247-262` |
| Sanctioned summarization seam (doc) | `pre_model_hook` "useful for managing long message histories (e.g., message trimming, summarization…)"; two patterns: transient `llm_input_messages` vs durable overwrite warning | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:396-424` |
| Hook → model input plumbing | `_get_model_input_state` prefers `llm_input_messages`; post-hook history validated for tool-call integrity | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:636-658`; `_validate_chat_history` at `:243` |
| Integration test of both patterns | `test_pre_model_hook`: transient view leaves state untouched; `RemoveMessage(REMOVE_ALL_MESSAGES)` replaces stored history | `libs/prebuilt/tests/test_react_agent.py:1924-1954`
| Reducer unit tests | Remove-by-id, double-remove idempotence, unknown-id error, wipe-and-replace ordering | `libs/langgraph/tests/test_messages_state.py:96-145`, `:312-338` |
| Tools can compress history | ToolNode permits a tool returning `[RemoveMessage(id=REMOVE_ALL_MESSAGES)]`, skipping ToolMessage validation | `libs/prebuilt/langgraph/prebuilt/tool_node.py:1541-1546`; test at `libs/prebuilt/tests/test_tool_node.py:1280-1306` |
| Node-driven deletion end-to-end | `test_remove_message_from_node` deletes a message from graph state mid-run | `libs/langgraph/tests/test_pregel.py:3979-4001` |
| Checkpoint pruning API | `prune(thread_ids, strategy="keep_latest"\|"delete")` with DeltaChain-safety doctrine | `libs/checkpoint/langgraph/checkpoint/base/__init__.py:374-415` |
| Thread/run deletion | `delete_thread`, `delete_for_runs` (with reconstruction-breakage warning) | `libs/checkpoint/langgraph/checkpoint/base/__init__.py:320-348` |
| Raw-history recovery path | `get_delta_channel_history` walks parent chain to reconstruct values from writes until a `_DeltaSnapshot` seed | `libs/checkpoint/langgraph/checkpoint/base/__init__.py:582+`; impl `libs/checkpoint/langgraph/checkpoint/memory/__init__.py:142-228` |
| Prune conformance tests | keep_latest single/multi-thread/namespaced, write preservation, delete-all, noop edge cases | `libs/checkpoint-conformance/langgraph/checkpoint/conformance/spec/test_prune.py:34-194` |
| Storage-level compression | `DeltaChannel` stores sentinel per step; snapshot every `snapshot_frequency=1000` updates or 5000 supersteps | `libs/langgraph/langgraph/channels/delta.py:25-64`, `:193-202` |
| Platform TTL lifecycle | `ThreadTTLConfig` strategy `delete\|keep_latest`, `default_ttl`, sweep interval/limit | `libs/cli/langgraph_cli/schemas.py:109-124` |
| Long-term memory eviction | Store-level `TTLConfig` (`refresh_on_read`, `omit_expired`, `default_ttl`) and semantic `IndexConfig` | `libs/checkpoint/langgraph/store/base/__init__.py:545-575` |
| Trace-only summarization | `TracePolicy.process_inputs/process_outputs` may "summarize large payloads (e.g. message history)" for observability; `omit_payload` helper | `libs/langgraph/langgraph/types.py:532-567` |
| Docs moved off-repo | Redirects for add-summary/delete-messages/manage-history how-tos to external docs | `docs/redirects.json:23-25` |

## Answers to Dimension Questions

**1. When does summarization happen?**
Never automatically. There are no triggers, thresholds, or token budgets anywhere in the libraries (verified: zero occurrences of `trim_messages`/`count_tokens`/tiktoken/token budgeting in `libs/**` Python source). The framework's answer is positional, not conditional: user code runs in `pre_model_hook` before every model call (`chat_agent_executor.py:396`, wired as a graph node before `"agent"`), which is where a user would decide "history too long → summarize." No evidence found of any built-in condition that fires such a hook's logic.

**2. What evidence does the summary cover?**
Not applicable — no summary records, prompts, or coverage ranges exist in the repo. Whatever a user writes into state after `RemoveMessage(id=REMOVE_ALL_MESSAGES)` is just ordinary messages; the framework tracks nothing about what the replacement covers. Coverage tracking is absent.

**3. Can summary drift be detected?**
No. Nothing compares summaries against superseded history or evaluates fidelity. The only post-manipulation safety check is structural, not semantic: `_validate_chat_history` ensures AIMessage tool calls have matching ToolMessages (`chat_agent_executor.py:243`, invoked at `:651`) so a compressed history cannot leave dangling tool-call pairs — it says nothing about whether decisions, facts, or uncertainty survived compression.

**4. Is raw history retained?**
By default, yes — twice over. (a) State is append-only unless nodes deliberately remove messages (`message.py:67-70`), and the transient pattern (`llm_input_messages`) compresses only the model's view while stored state keeps everything (`chat_agent_executor.py:404-406`, tested at `test_react_agent.py:1928-1939`). (b) Even after a destructive rewrite, older checkpoints persist until `prune`/`delete_thread`/TTL removes them (`base/__init__.py:320-415`; `schemas.py:109-124`), so pre-summary raw history remains recoverable within the retention window.

**5. Can summaries be regenerated?**
Not by the framework — there is no regeneration or refresh logic. Recovery of the *inputs* to a summary is possible via time travel over checkpoint history (`get_state_history`) or delta reconstruction (`get_delta_channel_history`, `base/__init__.py:582+`; operator recipe in `examples/delta-channel-dump/README.md:92-116`), provided ancestors were not pruned. Notably, naive pruning can permanently sever that recovery path for `DeltaChannel` keys — the base class documents that the surviving checkpoint "would silently reconstruct as empty (no error raised)" (`base/__init__.py:397-401`).

## Architectural Decisions

- **Substrate over policy.** LangGraph implements the write mechanics of compression (append/overwrite/remove/atomic-replace in one reducer, `message.py:60-244`) and delegates all *policy* (when, what prompt, what to keep) to application code. This keeps core semantics small and testable but means out-of-the-box behavior never compresses anything.
- **Two-tier compression contract.** The `llm_input_messages` vs `messages` distinction (`chat_agent_executor.py:400-413`) separates *view compression* (non-destructive, per-call) from *state rewriting* (destructive, durable). This is the key design decision: agents can shrink context without corrupting durable history.
- **Destructive edits are explicit and fail-loud.** Removal requires either exact known IDs or the global sentinel; unknown-ID removal raises (`message.py:227-230`). There is no silent truncation API.
- **Retention as a first-class, separate concern.** History size is managed at the storage layer (`prune`, `delete_thread`, thread TTLs) with strategy names shared across CLI schema, SDK wrappers, and saver interface (`schemas.py:112`, `libs/sdk-py/langgraph_sdk/schema.py:130-134`, `base/__init__.py:378`).
- **Lossless compression lives below semantics.** Storage growth is bounded by `DeltaChannel` delta-replay with periodic snapshots (`delta.py:50-64`) — orthogonal to LLM-facing context size.
- **Docs externalized.** All how-to prose about summarizing/deleting history was redirected out of the repo (`docs/redirects.json:23-25`), signaling these are considered application concerns, not library features.

## Notable Patterns

- **Sentinel-driven bulk replace.** A magic ID (`__remove_all__`) inside the standard remove mechanism turns a list reducer into a replace-all operation without new APIs (`message.py:38`, `:209-213`).
- **Hook-as-extension-point.** `pre_model_hook` becomes its own graph node between tools and agent (pinned by snapshots in `libs/prebuilt/tests/test_react_agent_graph.py` + `__snapshots__/test_react_agent_graph.ambr`), so history management composes with the ReAct loop without subclassing.
- **Tool-initiated history surgery.** Tools may return `Command(update={"messages": [RemoveMessage(id=REMOVE_ALL_MESSAGES)]})`, and ToolNode relaxes its validation accordingly (`tool_node.py:1541-1546`) — compression can be triggered from inside a tool call.
- **Documented-danger API design.** Destructive operations carry extensive docstring warnings describing exactly how they break (`delete_for_runs` severing delta chains, `base/__init__.py:340-346`; prune strategies, `:387-413`) — operational knowledge embedded at the interface.
- **Batch-invariant reducers.** Delta replay requires `reducer(reducer(state, xs), ys) == reducer(state, xs + ys)` (`delta.py:41-48`), which constrains how history reducers (including future summarizing ones) may behave.

## Tradeoffs

- **Zero built-in convenience vs. correctness risk shifted to users.** Every application reimplements trigger logic and prompts; mistakes (e.g., dropping tool-call pairs, losing task state) are caught only structurally (`chat_agent_executor.py:243`) or not at all.
- **Append-only default maximizes recoverability but guarantees unbounded growth.** Without user intervention, both prompt size and checkpoint storage grow monotonically; only platform TTL/prune mitigate the latter (`schemas.py:109-124`).
- **Transient compression preserves auditability at the cost of repeated work:** `llm_input_messages` recomputes the trimmed view each call and stores full history forever.
- **Atomic replace is simple but lossy-by-design:** once `REMOVE_ALL_MESSAGES` runs, in-state raw history is gone immediately; recovery depends on checkpoint retention settings, coupling compression quality to ops config.
- **Delta optimization narrows the safe-operation surface:** pruning/deleting becomes dangerous around `DeltaChannel` keys, to the point where the docs suggest "skip pruning threads whose graph uses DeltaChannel" as a legitimate option (`base/__init__.py:412-413`).

## Failure Modes / Edge Cases

- **Unknown-ID removal raises** `ValueError: Attempting to delete a message with an ID that doesn't exist` (`message.py:227-230`), tested at `test_messages_state.py:118`.
- **Silent empty reconstruction after bad pruning.** A naive `keep_latest` prune severs delta chains and channels reconstruct as empty with *no error* (`base/__init__.py:397-401`) — the most dangerous documented mode for history durability.
- **Run deletion breaks reconstruction** if the run produced ancestor writes or the only `_DeltaSnapshot` blob (`base/__init__.py:340-346`).
- **Partial thread copies break delta state**: copies must include the complete parent chain (`base/__init__.py:361-370`).
- **Feature asymmetry between reducers:** `_messages_delta_reducer` cannot execute the canonical summary-rewrite pattern (no `REMOVE_ALL_MESSAGES` support, no missing-ID UUID assignment, `message.py:260-262`) — a summary rewrite on a `DeltaChannel`-backed messages key needs different mechanics than on a plain `add_messages` key.
- **Structural-only validation after compression:** `_validate_chat_history` catches broken tool-call pairing but not lost facts/decisions (`chat_agent_executor.py:243`).
- **Step-bound masquerading as budget:** `remaining_steps < 2` yields "Sorry, need more steps…" (`chat_agent_executor.py:434-440`, `:620-634`) — bounds loop length, offers no protection against context-window overflow.

## Future Considerations

- An optional built-in summarization node (trigger + prompt + coverage bookkeeping) would close the largest gap while preserving the substrate-first philosophy; the `pre_model_hook` seam and `REMOVE_ALL_MESSAGES` primitive are already sufficient interfaces for it.
- Parity for `_messages_delta_reducer` with `REMOVE_ALL_MESSAGES` (or an official alternative rewrite path) would let the same compression code run on both channel kinds.
- Machine-checkable prune safety (e.g., auto-preserving ancestors up to the nearest `_DeltaSnapshot`, already sketched in `base/__init__.py:404-411`) would convert documented doctrine into enforced behavior.
- Token-budget hooks (accepting a counter callable) would give agents a standard way to bound context without importing langchain-core utilities outside this repo.

## Questions / Gaps

- **No summary records/prompts/coverage tracking exist** — searched `summariz`, `summary`, `compress`, `condense` across all `libs/**` Python sources; only docstring mentions (`chat_agent_executor.py:397`, `types.py:550-556`) matched. Questions 2, 3, and the "refresh logic" evidence item therefore have no in-repo answer.
- **Token counting is out of scope of this repo**: `trim_messages`/`count_tokens` live in langchain-core, which is not vendored here; claims about them could not be verified from this source tree.
- **JS parity unknown**: `libs/sdk-js` contains only a README pointer to the separate langgraphjs repository, so no JS-side conclusions were drawn.
- **Runtime server behavior (actual TTL sweeps, prune endpoint implementations)** is configured here (`schemas.py:109-124`; SDK wrappers `libs/sdk-py/langgraph_sdk/_async/threads.py:444-479`) but executed in the closed-source LangGraph platform; only the client-side contracts could be inspected.

---

Generated by dimension `05.06-memory-compression-and-summarization` against `langgraph`.
