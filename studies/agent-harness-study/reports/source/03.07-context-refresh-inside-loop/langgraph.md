# Source Analysis: langgraph

## 03.07 Context Refresh Inside the Loop

### Source Info

| Field | Value |
|-------|-------|
| Name | langgraph |
| Path | `studies/agent-harness-study/sources/langgraph` |
| Language / Stack | Python (monorepo: `libs/langgraph`, `libs/prebuilt`, `libs/checkpoint`, `libs/checkpoint-postgres`, `libs/checkpoint-sqlite`, `libs/cli`, `libs/sdk-py`, `libs/sdk-js`) |
| Analyzed | 2026-07-15 |

## Summary

LangGraph is a low-level orchestration framework (Pregel-style graph execution on top of channels, checkpoints, and a typed store) that is **explicitly LLM-agnostic**. The "context" the loop manages is the **graph state** — a dict of channels, the canonical one being a `messages` channel typically reduced by `add_messages` (`libs/langgraph/langgraph/graph/message.py:60-244`).

The framework's design treats context refresh as an **application responsibility, not a runtime concern**. There is no built-in token counter, no context-window guard, no message summarization, and no automatic trimming. What LangGraph provides is the **plumbing** that lets user code refresh context: a remove-by-id message primitive (`RemoveMessage` and `REMOVE_ALL_MESSAGES` at `libs/langgraph/langgraph/graph/message.py:38, 209-213, 217-234`), a separate `llm_input_messages` channel that can override the LLM call without mutating canonical state (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:636-658, 723-740`), a `pre_model_hook` slot on the prebuilt ReAct agent specifically recommended for "message trimming, summarization, etc." (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:396-424`), and a long-term `BaseStore` with semantic search and TTL for retrieval (`libs/checkpoint/langgraph/store/base/__init__.py:700-1000+`).

The only true "context refresh" mechanism inside the loop is the `DeltaChannel` (`libs/langgraph/langgraph/channels/delta.py:25-204`) plus `_put_checkpoint` / `delta_channels_to_snapshot` (`libs/langgraph/langgraph/pregel/_checkpoint.py:37-58, 61-121` and `libs/langgraph/langgraph/pregel/_loop.py:1064-1199`). That mechanism is a **storage-compression** strategy (replay writes from a periodically snapshotted seed), not a context-window-protection mechanism. The counter that drives snapshots is keyed on (per-channel updates since last snapshot, supersteps since last snapshot) and the bound is 5000 supersteps by default (`libs/langgraph/langgraph/_internal/_config.py:33-34`). It reduces what is *persisted*, not what is *computed* per superstep.

Net effect: a LangGraph loop can keep running across arbitrarily long message histories because the framework never inspects the LLM context window. It just keeps writing to the `messages` channel. Whether the LLM call inside a node succeeds, fails, or is truncated is fully delegated to whatever the user puts in the `agent` node (or `pre_model_hook`).

## Rating

**3 / 10 — Present, weakly documented, fragile, and externally delegated.**

Rationale tied to rubric:

- The framework offers building blocks (removal primitives, separate input channel, pre-model hook, store-based retrieval, DeltaChannel for storage compression) but no first-class model of "context refresh inside the loop."
- No token counter or context-window awareness: the loop has no way to know when it has overflowed a model limit (`grep` for `token_usage|Token.*count|num_tokens|max_input|max_output` across `libs/langgraph/langgraph` returns no matches).
- No summarizer: `grep` for `summariz|truncat|compress|max_tokens` across `libs/langgraph/langgraph` returns only graph-trimming utilities in `_draw.py:265-266` (visualization).
- Recursion limit is a step bound, not a context budget (`libs/langgraph/langgraph/pregel/main.py:2578-2579, 3017-3026`).
- The "design" is the explicit handoff to the user via `pre_model_hook`, with one example note in the docstring and no reference implementation in this repo.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| No internal token counter / context window awareness | `grep` for `token_usage\|Token.*count\|num_tokens\|max_input\|max_output` returns no results in `libs/langgraph/langgraph` | (search boundary) |
| No internal summarizer in core | `grep` for `summariz\|truncat\|compress\|max_tokens` in `libs/langgraph/langgraph` returns only `_draw.py:265-266` (graph-trim for visualization) | `libs/langgraph/langgraph/pregel/_draw.py:265-266` |
| Recursion limit (step bound, not context bound) | `recursion_limit` config key validated, error raised | `libs/langgraph/langgraph/pregel/main.py:2578-2579, 3017-3026` |
| Recursion limit applied to loop | `self.stop = self.step + self.config["recursion_limit"] + 1` | `libs/langgraph/langgraph/pregel/_loop.py:1677, 1936` |
| `IsLastStep` / `RemainingSteps` managed values | Reveal step counter, not context size | `libs/langgraph/langgraph/managed/is_last_step.py:9-24` |
| Context assembly per turn: read full state into each node | `read_channels` reads selected channels (returns latest accumulated state, no windowing) | `libs/langgraph/langgraph/pregel/_io.py:23-53` |
| Context assembly per turn: state is incrementally updated each superstep | `apply_writes` applies task writes to channels and bumps versions | `libs/langgraph/langgraph/pregel/_algo.py:232-345` |
| Loop ticks; channels are reused, not rebuilt | `PregelLoop.tick` calls `prepare_next_tasks` and reuses `self.channels` | `libs/langgraph/langgraph/pregel/_loop.py:592-674` |
| Resume path: hydrate from checkpoint | `channels_from_checkpoint` / `achannels_from_checkpoint` restore channels from blob | `libs/langgraph/langgraph/pregel/_checkpoint.py:136-226` |
| `add_messages` reducer (canonical message context) | Merges left+right lists by message ID; supports `RemoveMessage` and `REMOVE_ALL_MESSAGES` | `libs/langgraph/langgraph/graph/message.py:60-244` |
| `RemoveMessage` removal primitive | Removes by ID; raises if ID not found | `libs/langgraph/langgraph/graph/message.py:217-234` |
| `REMOVE_ALL_MESSAGES` sentinel | Clears the entire list | `libs/langgraph/langgraph/graph/message.py:38, 209-213` |
| `_messages_delta_reducer` (DeltaChannel variant) | Batched reducer for messages; no `REMOVE_ALL_MESSAGES` support, no auto-trim | `libs/langgraph/langgraph/graph/message.py:247-309` |
| `pre_model_hook` slot for user-implemented context refresh | Docstring: "Useful for managing long message histories (e.g., message trimming, summarization, etc.)" | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:396-424` |
| `pre_model_hook` is wired into the prebuilt ReAct graph | `add_node("pre_model_hook", ...)` and edge to `agent` | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:795-798, 876-879` |
| `llm_input_messages` channel lets the hook override the LLM input without mutating state | `_get_model_input_state` reads `llm_input_messages` first | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:636-658` |
| `llm_input_messages` schema is dynamically added when `pre_model_hook` is present | `CallModelInputSchema` extends `state_schema` with `llm_input_messages` | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:723-740` |
| `pre_model_hook` test coverage (one example, both `llm_input_messages` and `messages` paths) | `test_pre_model_hook` | `libs/prebuilt/tests/test_react_agent.py:1924-1954` |
| `BaseStore` (long-term memory / retrieval) | Get/put/search/list_namespaces with TTL and vector search | `libs/checkpoint/langgraph/store/base/__init__.py:700-1000+` |
| `BaseStore` TTL on items; refresh on read or write | `TTLConfig` (refresh_on_read, default_ttl, sweep_interval_minutes) | `libs/checkpoint/langgraph/store/base/__init__.py:545-567, 1278-1296` |
| `BaseStore` index config for semantic search | `IndexConfig` (dims, embed, fields) | `libs/checkpoint/langgraph/store/base/__init__.py:570-697` |
| Store TTL sweeper runs in background thread | `sweep_ttl` and `_ttl_sweeper` loop | `libs/checkpoint-sqlite/langgraph/store/sqlite/base.py:1047-1102` |
| `Runtime.store` exposes the store to nodes | Runtime dataclass | `libs/langgraph/langgraph/runtime.py:124, 203-204` |
| DeltaChannel: storage-compression strategy (not context-window protection) | Stores sentinel; replays ancestor writes via reducer | `libs/langgraph/langgraph/channels/delta.py:25-204` |
| DeltaChannel snapshot cadence (per-channel + superstep bound) | `snapshot_frequency` (default 1000) OR `DELTA_MAX_SUPERSTEPS_SINCE_SNAPSHOT` (default 5000) | `libs/langgraph/langgraph/channels/delta.py:50-64, 74-79` |
| `DELTA_MAX_SUPERSTEPS_SINCE_SNAPSHOT` env var override | `LANGGRAPH_DELTA_MAX_SUPERSTEPS_SINCE_SNAPSHOT` | `libs/langgraph/langgraph/_internal/_config.py:33-34` |
| `delta_channels_to_snapshot` predicate | Snapshots when `updates >= snapshot_frequency` OR `supersteps >= bound` | `libs/langgraph/langgraph/pregel/_checkpoint.py:37-58` |
| `create_checkpoint` writes `_DeltaSnapshot` blob for snapshotted channels | `_DeltaSnapshot(ch.get())` | `libs/langgraph/langgraph/pregel/_checkpoint.py:61-121` |
| `get_delta_channel_history` walks parent chain to find seed + accumulate writes | Saver default impl | `libs/checkpoint/langgraph/checkpoint/base/__init__.py:582-649` |
| Per-delta-channel counter bookkeeping in metadata | `counters_since_delta_snapshot` updated in `_put_checkpoint` | `libs/langgraph/langgraph/pregel/_loop.py:1094-1107` |
| Write provenance: every write tagged with task_id; traced through `checkpoint_pending_writes` | `put_writes` | `libs/langgraph/langgraph/pregel/_loop.py:408-501` |
| `_exit_delta_writes` accumulator captures delta writes for exit-mode persistence | Captures in `after_tick` and `_first` | `libs/langgraph/langgraph/pregel/_loop.py:697-700, 1000-1003` |
| LastValue, BinaryOperatorAggregate, EphemeralValue channels (no auto-trim) | Channels hold full state until user mutates | `libs/langgraph/langgraph/channels/last_value.py:20-79, 81-151`, `libs/langgraph/langgraph/channels/binop.py:51-141`, `libs/langgraph/langgraph/channels/ephemeral_value.py:15-79` |
| `Channel.consume()` is for trigger semantics, not for trimming | Base contract | `libs/langgraph/langgraph/channels/base.py:101-121` |
| `get_config_jsonschema` / `get_context_jsonschema` (config schema, not context refresh) | Schema exposure only | `libs/langgraph/langgraph/pregel/main.py:950, 955, 976, 983, 993` |
| `Runtime.context` is static per-run | Documented as "Static context for the graph run, like `user_id`, `db_conn`, etc." | `libs/langgraph/langgraph/runtime.py:198-201` |

## Answers to Dimension Questions

1. **Is context static or rebuilt each turn?**
   - It is **incrementally updated**, not rebuilt. The same `self.channels: Mapping[str, BaseChannel]` instance survives across supersteps (`libs/langgraph/langgraph/pregel/_loop.py:198, 592-674`); `apply_writes` (`:232-345`) merges new writes into existing channel values, and `read_channels` (`:38-53`) returns the current accumulated snapshot per superstep.
   - On resume from a checkpoint, `channels_from_checkpoint` (`:136-184`) **rebuilds** channel instances by reading stored `channel_values` and (for `DeltaChannel`) replaying ancestor writes via `get_delta_channel_history` (`:582-649`).
   - Net: the `messages` channel is monotonically appended to by the `add_messages` reducer; the only ways to shrink it are (a) explicit `RemoveMessage` writes, (b) `REMOVE_ALL_MESSAGES` reset, or (c) `Overwrite` (which is a write that *replaces* the channel value entirely — see `libs/langgraph/langgraph/channels/binop.py:115-127`).

2. **What gets dropped first?**
   - **Nothing, by default.** The loop never drops messages. There is no priority, no LRU, no token-budget eviction.
   - What gets *hidden* in the LLM call (without dropping from state) is the `llm_input_messages` channel: the `pre_model_hook` can return a different message list for the model call, leaving canonical state untouched (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:636-658`).
   - The `DeltaChannel` mechanism trims **storage** (deletes intermediate snapshot blobs and forces replay from the nearest `_DeltaSnapshot` seed), not context shown to the model.

3. **Is summarization faithful?**
   - **N/A — the framework does not summarize.** No summarizer, no chunker, no faithfulness check. The only reducer for messages is `add_messages` (`libs/langgraph/langgraph/graph/message.py:60-244`) and `_messages_delta_reducer` (`:247-309`); both are deterministic, idempotent, and lossless.
   - If a user implements summarization in a `pre_model_hook`, faithfulness is entirely the user's concern. There is no framework-side verification step.

4. **Are tool results preserved or summarized?**
   - **Preserved.** Tool nodes write `ToolMessage` objects to the `messages` channel via `add_messages` (`libs/prebuilt/langgraph/prebuilt/tool_node.py:88, 1247, 1250, 1524`); the reducer's ID-based merge keeps them intact. The `StreamMessagesHandlerV2` (`:307-334`) actively *filters* `ToolMessage` from the v2 streaming channel — but the canonical state still holds them.
   - There is no tool-result summarization in the framework. The `pre_model_hook` can drop them by ID, but only if the user does it.

5. **Can the model tell what context is missing?**
   - **No.** Nothing in the loop signals to the model that earlier context was trimmed or omitted. The framework emits no `system` instruction, no metadata field, no token count, and no warning that `llm_input_messages` differs from canonical `messages`. The `messages` channel in state is always the full record.

## Architectural Decisions

- **Context = graph state, not a separate "context object."** The loop reads from `self.channels` (a `Mapping[str, BaseChannel]`) on every superstep and passes selected keys to each node as the node's `state` input (`libs/langgraph/langgraph/pregel/_io.py:38-53`). There is no separate context window.
- **Reducers are the source of truth for accumulation.** `add_messages` (`:60-244`) is the standard "append-by-id" reducer; `BinaryOperatorAggregate` (`:51-141`) is the generic binary op; `DeltaChannel` (`:25-204`) is the new batched reducer with snapshot+replay storage. All three are deterministic and idempotent — they do not mutate prior state, only add/overwrite. The choice between `BinaryOperatorAggregate` and `DeltaChannel` is a **storage** decision (full snapshot vs. delta), not a context-decision one.
- **Removal is a write, not a side effect.** The framework models message removal as a `RemoveMessage` write that the reducer interprets (`libs/langgraph/langgraph/graph/message.py:217-234`). `REMOVE_ALL_MESSAGES` is a sentinel ID with special meaning (`:38, 209-213`). This keeps the "what's in the messages channel" decision fully auditable from the write log.
- **LLM context assembly is delegated to the `pre_model_hook` slot.** The prebuilt ReAct agent (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:296, 396-424, 723-740, 795-798, 876-879`) explicitly reserves a node before the `agent` node for "message trimming, summarization, etc." The hook can either mutate state via `RemoveMessage` / `Overwrite` or return a transient `llm_input_messages` key that overrides only the LLM call.
- **Long-term memory is via `BaseStore`, not via the messages channel.** The `BaseStore` (`:700-1000+`) is a namespaced, versioned, optionally vector-indexed K/V store with TTL. The `Runtime.store` attribute (`:124, 203-204`) makes it available to nodes for ad-hoc retrieval. Refresh is user-driven (`store.search(...)` in a node); the loop does not orchestrate it.
- **The recursion limit is a step budget, not a context budget.** `self.stop = self.step + self.config["recursion_limit"] + 1` (`libs/langgraph/langgraph/pregel/_loop.py:1677, 1936`) caps the number of supersteps; on overflow, `loop.status = "out_of_steps"` and a `GraphRecursionError` is raised (`libs/langgraph/langgraph/pregel/main.py:3017-3026`).

## Notable Patterns

- **Removal as a first-class message type.** `RemoveMessage` is a `BaseMessage` subclass from `langchain_core.messages` that the reducer interprets. Same shape as a regular message; idempotent by ID (`libs/langgraph/langgraph/graph/message.py:217-234`).
- **Two channels for "what the model sees" vs. "what the state holds."** `messages` is canonical; `llm_input_messages` is the optional pre-model-hook override. This separation lets the user rewrite the LLM input per turn without losing or distorting the persistent record (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:636-658, 723-740`).
- **Delta storage with bounded replay depth.** The `DeltaChannel` keeps only writes since the last `_DeltaSnapshot` blob in the checkpoint; replay reconstructs the full state by walking `get_delta_channel_history` (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:582-649`). Bounded by `snapshot_frequency` (default 1000 updates) and `DELTA_MAX_SUPERSTEPS_SINCE_SNAPSHOT` (default 5000 supersteps) — these caps prevent unbounded write-log accumulation (`libs/langgraph/langgraph/channels/delta.py:50-79`, `libs/langgraph/langgraph/_internal/_config.py:33-34`).
- **Per-channel counter metadata.** `counters_since_delta_snapshot` is carried on `CheckpointMetadata` per superstep, so the snapshot cadence is observable and tunable from outside (`libs/langgraph/langgraph/pregel/_loop.py:1094-1107`).
- **Write provenance.** Every write is tagged with the originating `task_id` in `checkpoint_pending_writes`; `_put_pending_writes` groups by task and `put_writes` (`:408-501`) sends them to the saver with that tag, so the checkpoint log preserves *who wrote what*. This is the framework's main observability surface for context evolution.
- **Static `Runtime.context` per run.** `Runtime.context` is a frozen dataclass slot populated once at the start of the run (`libs/langgraph/langgraph/runtime.py:198-201`). The framework does not "refresh" it across turns.

## Tradeoffs

- **Pro: zero coupling to LLM context windows.** LangGraph never inspects message length, token count, or model limits, so it works for any LLM (or no LLM at all). The user picks the model and the trimming policy.
- **Pro: deterministic, replayable state.** Because all state changes are reducer-driven and the reducer is pure, the same sequence of writes always produces the same state. Debugging and time-travel (`get_state_history`) are first-class (`libs/langgraph/langgraph/pregel/main.py:1479, 1532`).
- **Pro: `DeltaChannel` storage optimization is a real win for long histories.** Without it, every checkpoint carries the full `messages` list; with it, only the latest snapshot blob plus deltas. The 5000-superstep bound prevents pathological growth.
- **Con: no first-class "we exceeded the context window" signal.** If the LLM call inside a node fails because of context overflow, the failure bubbles up as a generic exception. The framework does not pre-empt it, does not measure input size, and does not suggest a remedy.
- **Con: context refresh is a hand-rolled responsibility.** Every application must implement its own trim/summarize policy, in a `pre_model_hook` (prebuilt) or a regular node (custom). The framework ships no reference implementation, no token counter, no summarizer template, no rate-limit-aware strategy.
- **Con: the `_messages_delta_reducer` does NOT handle `REMOVE_ALL_MESSAGES`** — only per-ID `RemoveMessage` works there (`:247-309`, comment at `:260-262`). Applications using `DeltaChannel` for messages lose the bulk-clear affordance.
- **Con: `counters_since_delta_snapshot` is per-channel, not per-state.** Two channels with the same effective "age" can snapshot at different times, complicating reasoning about replay cost.
- **Con: `pre_model_hook` only mutates the messages channel.** It cannot directly influence other channels (e.g., a `summary` channel that the agent reads in addition to `messages`). To do that, the user must write a separate summary node upstream.

## Failure Modes / Edge Cases

- **Unbounded message history.** Without a `pre_model_hook` (or equivalent), `messages` grows on every model + tool call. After enough turns, the LLM call in `agent` will fail with a context-length error from the model provider. The framework will surface that as a regular node error and (with a `retry_policy`) retry — but retrying with the same messages will fail identically. There is no automatic fallback to a smaller context.
- **`RemoveMessage` for an unknown ID raises.** In the `add_messages` reducer (`:227-230`), removing a non-existent message raises `ValueError`. The `_messages_delta_reducer` (`:247-309`) does not raise in that case (it just no-ops), so behavior diverges by channel type.
- **`REMOVE_ALL_MESSAGES` is not idempotent across `BinaryOperatorAggregate` and `DeltaChannel`.** Works in `add_messages` (`:209-213`); explicitly *not* supported in `_messages_delta_reducer` (`:260-262`).
- **`DeltaChannel` snapshot-vs-version race.** `create_checkpoint` has a branch (`:104-107`) that bumps a channel's version manually when exit mode decides to snapshot a channel that wasn't written this superstep — because the saver only receives a version if `apply_writes` bumped it. The comment notes this is "effectively dead code" in sync/async durability but exercised in exit mode.
- **Resumed loop with stale `messages`.** On time-travel replay, `_first` strips cached `RESUME` writes so interrupts re-fire (`libs/langgraph/langgraph/pregel/_loop.py:885-888`). The state itself (full message history) is not trimmed or rolled back — the user still sees all prior messages when the loop resumes.
- **`IsLastStep`/`RemainingSteps` is a step counter, not a context counter.** A user wiring a "stop if too many tokens" guard into `messages` cannot use these managed values (`libs/langgraph/langgraph/managed/is_last_step.py:9-24`); they would need to roll their own managed value reading the reducer's accumulated size.
- **Context-related `get_state_history` may not return intermediate `messages` states for `DeltaChannel` unless snapshot exists.** Without a snapshot at a given checkpoint, `messages` is reconstructed by replay (`libs/langgraph/langgraph/pregel/_checkpoint.py:136-184`); a slow query against an in-memory store would be O(replay depth).
- **`recursion_limit` is checked at tick time** (`libs/langgraph/langgraph/pregel/_loop.py:600-602`); a long-running node that exceeds the limit only after the superstep completes will not be preempted mid-execution.

## Future Considerations

- **A first-class `LLMContext` channel type** — a `DeltaChannel` variant that knows about message size, exposes a `compress` hook, and provides a built-in summarization step — would close the most glaring gap.
- **A standard `pre_model_hook` library** shipped with the prebuilt agent (trim-by-count, trim-by-tokens-via-langchain, summarize-via-llm) would give applications a working default and reduce the surface area for hand-rolled errors.
- **A built-in token counter** that participates in `IsLastStep` semantics would let managed values report "estimated tokens used" alongside "remaining steps."
- **Honest support for `REMOVE_ALL_MESSAGES` in `_messages_delta_reducer`** would unify behavior across channel types.
- **`llm_input_messages` generalisation.** Allowing per-node `input_overrides` (not only the prebuilt `agent` node) would let users rewrite the state slice fed to any node — useful for subgraphs and tool callers.
- **Visibility into `counters_since_delta_snapshot` and replay depth** in `get_state` / `get_state_history` would help operators diagnose slow rehydration on long-lived threads.
- **Explicit `overwrite` semantics on the `messages` channel** (using the existing `Overwrite` value type) are supported by `_messages_delta_reducer` (`:173-182`) and `BinaryOperatorAggregate` (`:115-127`) but not exercised in any built-in pattern in this repo.

## Questions / Gaps

- **Is there a reference summarization node anywhere in the repo?** `grep` finds no `summariz`/`truncat`/`compress`/`max_tokens` in the langgraph and prebuilt libraries. The docstring at `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:397` points users to a docs page, but no in-repo example is present. *No evidence found* in this source.
- **Does any built-in node count tokens?** *No evidence found.* `grep` for `token_usage|num_tokens|max_input|max_output` returns no results in `libs/langgraph/langgraph`.
- **Does the loop ever rewrite a `messages` channel without an explicit user write?** *No evidence found.* The only mechanism that drops messages is `RemoveMessage` / `REMOVE_ALL_MESSAGES` / `Overwrite` writes, all user-initiated.
- **How does `get_state_history` cope with `DeltaChannel` rehydration cost on a million-step thread?** The walk is bounded by `DELTA_MAX_SUPERSTEPS_SINCE_SNAPSHOT` (default 5000), so a single `get_state` call is at most ~5000 write replays, but the *set* of channels being replayed can multiply that. No aggregate bound documented. *No evidence found* for an upper bound on total replay time.
- **What happens if `pre_model_hook` returns both `messages` and `llm_input_messages`?** The prebuilt code (`:636-658`) prefers `llm_input_messages` when present. If both are returned, the canonical `messages` is updated by the reducer *and* the LLM sees `llm_input_messages`. The relationship between the two is not documented in the hook contract beyond "at least one of" (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:413`).
- **Is the `messages` channel required to be `BinaryOperatorAggregate(add_messages)` or `DeltaChannel(_messages_delta_reducer)`?** The prebuilt ReAct agent builds the state with `add_messages` (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:60, 72`). Custom graphs may use `DeltaChannel`, but then they lose `REMOVE_ALL_MESSAGES` support (`:260-262`). The framework does not warn about this.

---

Generated by `03.07-context-refresh-inside-loop` against `langgraph`.
