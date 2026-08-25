# Source Analysis: langgraph

## Dimension 05.01: Short-Term Conversation Memory

### Source Info

| Field | Value |
|-------|-------|
| Name | langgraph |
| Path | `studies/agent-harness-study/sources/langgraph` |
| Language / Stack | Python (monorepo: `libs/langgraph`, `libs/checkpoint`, `libs/checkpoint-postgres`, `libs/checkpoint-sqlite`, `libs/prebuilt`, `libs/checkpoint-conformance`) |
| Analyzed | 2026-08-25 |

## Summary

LangGraph implements short-term conversation memory as **graph state persisted by a pluggable checkpointer**, not as a dedicated message store. Conversation history lives in a state key (by convention `messages`) whose channel uses the `add_messages` reducer (`libs/langgraph/langgraph/langgraph/graph/message.py:61`), an append-with-upsert-by-ID merge function. Every superstep, the whole channel value (the full message list) is snapshotted into a `Checkpoint` keyed by `(thread_id, checkpoint_ns, checkpoint_id)` and written through a `BaseCheckpointSaver` (`libs/checkpoint/langgraph/langgraph/checkpoint/base/__init__.py:92-124`, `libs/checkpoint/langgraph/langgraph/checkpoint/base/__init__.py:176`). On each new invocation the loop loads the latest checkpoint (or an explicitly requested one) and hydrates channels from it (`libs/langgraph/langgraph/langgraph/pregel/_loop.py:1629-1708`).

The model therefore sees **the entire message list in state** on every call — there is no built-in windowing, summarization, or retrieval layer anywhere in this repo. Trimming is delegated to application code via `RemoveMessage` / `REMOVE_ALL_MESSAGES` tombstones applied through the reducer (`libs/langgraph/langgraph/langgraph/graph/message.py:209-234`) or via the prebuilt agent's `pre_model_hook`, which can feed the model a filtered `llm_input_messages` view without mutating stored history (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:395-425`, `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:636-658`). History is scoped per `thread_id` (plus `checkpoint_ns` for subgraphs), and editing/forking history is first-class: `update_state(..., as_node=...)`, checkpoint copy-forks with `source: "fork"` metadata, and time-travel replay that branches rather than overwrites.

## Rating

**9 / 10**

Rationale against the rubric:

- **Clear model, explicit interfaces**: memory = channel state + named reducer; storage = `BaseCheckpointSaver` protocol with typed `Checkpoint`/`CheckpointTuple`/`CheckpointMetadata` structures (`libs/checkpoint/langgraph/langgraph/checkpoint/base/__init__.py:38-146`).
- **Tests**: exhaustive reducer unit tests (`libs/langgraph/tests/test_messages_state.py:29-338`), plus a dedicated checkpointer conformance suite covering `delete_thread`, `copy_thread`, `prune`, `list` ordering/filtering, and pending writes (`libs/checkpoint-conformance/langgraph/checkpoint/conformance/spec/test_delete_thread.py:18-110`, `.../spec/test_list.py:43-300`).
- **Operational safeguards**: strict msgpack deserialization allowlist (`libs/checkpoint/README.md:49-50`, `libs/checkpoint/langgraph/langgraph/checkpoint/base/__init__.py:713-742`), pending-writes resume semantics so failed supersteps don't re-run successful nodes (`libs/checkpoint/README.md:52-54`), chat-history validation for orphaned tool calls (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:243-271`).
- **Durable / observable / extensible**: Postgres and SQLite saver implementations ship alongside the in-memory saver; full timeline inspection via `get_state_history` (`libs/langgraph/langgraph/langgraph/pregel/main.py:1480-1531`); custom savers supported by the base protocol; delta-snapshot metadata with bounded ancestor walks (`libs/checkpoint/langgraph/langgraph/checkpoint/base/__init__.py:63-86`).
- Point withheld from 10: **no built-in context-window management** — the framework guarantees persistence and correct rehydration but provides no automatic trimming, summarization, or token budgeting, so "does the model see enough without overflowing?" is entirely the application's problem (see Failure Modes).

## Evidence Collected

Every entry includes a file path with line numbers relative to the selected source root.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Message store (state shape) | `MessagesState` defines conversation history as `messages: Annotated[list[AnyMessage], add_messages]`; `MessageGraph` deprecated in favor of it | libs/langgraph/langgraph/graph/message.py:372-373, libs/langgraph/langgraph/graph/message.py:312-316 |
| Reducer (selection of what accumulates) | `add_messages` merges right into left: append-only unless ID matches (upsert), assigns UUIDs to missing IDs, honors `RemoveMessage` tombstones and `REMOVE_ALL_MESSAGES` wipe | libs/langgraph/langgraph/graph/message.py:187-244 |
| Wipe sentinel | `REMOVE_ALL_MESSAGES = "__remove_all__"` short-circuits merge to keep only post-tombstone messages | libs/langgraph/langgraph/graph/message.py:38, libs/langgraph/langgraph/graph/message.py:209-213 |
| Role/content conversion | Inputs coerced via `convert_to_messages` then chunk→message normalization before merging | libs/langgraph/langgraph/graph/message.py:193-201 |
| Provider-format option | `format="langchain-openai"` converts content blocks/tool responses to OpenAI-shaped messages at write time | libs/langgraph/langgraph/graph/message.py:236-240, libs/langgraph/langgraph/graph/message.py:376-389 |
| Experimental batch reducer | `_messages_delta_reducer` for `DeltaChannel`: dedup-by-ID + tombstoning in one pass, batching-invariant | libs/langgraph/langgraph/graph/message.py:247-309 |
| Checkpoint structure | `Checkpoint` TypedDict: `channel_values`, `channel_versions`, `versions_seen`, monotonic `id`, ISO `ts` | libs/checkpoint/langgraph/checkpoint/base/__init__.py:92-124 |
| Checkpoint source taxonomy | Metadata `source: "input" \| "loop" \| "update" \| "fork"` distinguishes fresh inputs, loop steps, manual edits, forks | libs/checkpoint/langgraph/checkpoint/base/__init__.py:41-48 |
| Saver interface | `BaseCheckpointSaver.get_tuple/list/put/put_writes/delete_thread/copy_thread/prune/delete_for_runs` define the pluggable store contract | libs/checkpoint/langgraph/langgraph/checkpoint/base/__init__.py:227-415 |
| In-memory store layout | `InMemorySaver.storage`: thread_id → checkpoint_ns → checkpoint_id → (ckpt blob, metadata blob, parent id); plus `writes` and per-version channel `blobs` maps | libs/checkpoint/langgraph/langgraph/checkpoint/memory/__init__.py:68-99 |
| Latest-checkpoint read | `get_tuple` returns max checkpoint ID when no `checkpoint_id` given; reconstructs `channel_values` from versioned blobs | libs/checkpoint/langgraph/langgraph/checkpoint/memory/__init__.py:275-310 |
| Per-invocation load | Loop `__enter__` fetches latest or explicit checkpoint, hydrates channels via `channels_from_checkpoint`, sets step/recursion bounds | libs/langgraph/langgraph/langgraph/pregel/_loop.py:1629-1708 |
| Model input selection (prebuilt agent) | Prompt runnable prepends system prompt to **full** `state["messages"]`; `call_model` passes entire history after `_validate_chat_history` | libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:137-170, libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:636-658 |
| Model-view override hook | `pre_model_hook` may return `llm_input_messages` (model-only view) or overwrite `messages` using `[RemoveMessage(id=REMOVE_ALL_MESSAGES), *new]` | libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:395-425 |
| Tool-message integrity guard | `_validate_chat_history` raises `INVALID_CHAT_HISTORY` if any `AIMessage.tool_calls` lack matching `ToolMessage`s | libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:243-271 |
| ToolNode Command validation | Tools returning `Command` must include a `ToolMessage` matching the invoked `tool_call_id` unless wiping all messages | libs/prebuilt/langgraph/prebuilt/tool_node.py:1545-1578 |
| Thread scoping | `thread_id` is primary key; docstring states reuse across invocations accumulates chat history | libs/checkpoint/langgraph/langgraph/checkpoint/base/__init__.py:182-199 |
| Subgraph namespaces | `checkpoint_ns` separates subgraph state under same thread; nested loops append counters to ns | libs/langgraph/langgraph/langgraph/pregel/_loop.py:325-340, libs/checkpoint/README.md:74-75 |
| History editing | `update_state(config, values, as_node)` writes values as if from a node, creating a new checkpoint with `source: "update"` | libs/langgraph/langgraph/langgraph/pregel/main.py:2515-2526, libs/langgraph/langgraph/langgraph/pregel/main.py:1729-1741 |
| Forking | `as_node="__copy__"` clones current checkpoint with `source: "fork"` metadata, optionally applying updates in the same superstep | libs/langgraph/langgraph/langgraph/pregel/main.py:1800-1878 |
| Time travel | Passing `checkpoint_id` marks run as replaying; loop writes a fork checkpoint before resuming so replays branch instead of clobbering | libs/langgraph/langgraph/langgraph/pregel/_loop.py:315, libs/langgraph/langgraph/langgraph/pregel/_loop.py:952-971 |
| Timeline observability | `get_state_history` iterates all checkpoints (filter/before/limit) as `StateSnapshot`s | libs/langgraph/langgraph/langgraph/pregel/main.py:1480-1531 |
| Step budget | `RemainingSteps` managed value = `stop - step`; loop sets `stop = step + recursion_limit + 1`; agent emits graceful fallback AIMessage when out of steps | libs/langgraph/langgraph/langgraph/managed/is_last_step.py:18-24, libs/langgraph/langgraph/langgraph/pregel/_loop.py:1701, libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:684-692 |
| Streaming dedupe (view, not store) | `StreamMessagesHandler` tracks seen message IDs so streamed chunks aren't double-emitted; separate from persisted history | libs/langgraph/langgraph/langgraph/pregel/_messages.py:94-104 |
| Serde safety | Default `JsonPlusSerializer`; strict msgpack allowlist via env `LANGGRAPH_STRICT_MSGPACK=true` or `with_allowlist` clone | libs/checkpoint/README.md:49-50, libs/checkpoint/langgraph/langgraph/checkpoint/base/__init__.py:713-742 |
| Conformance suite | Capability detection (put/get_tuple/list/delete_thread mandatory; copy_thread/prune optional) drives spec tests for all savers | libs/checkpoint-conformance/langgraph/checkpoint/conformance/capabilities.py:15-63 |
| Delta-channel history walk | `get_delta_channel_history` walks parent chain accumulating per-channel writes until seed snapshot; overridden for performance in `InMemorySaver` | libs/checkpoint/langgraph/checkpoint/base/__init__.py:582-649, libs/checkpoint/langgraph/checkpoint/memory/__init__.py:142-223 |

## Answers to Dimension Questions

**1. What conversation history does the model see?**
The full `messages` list held in graph state at that superstep, hydrated from the latest (or explicitly addressed) checkpoint. In the prebuilt agent the exact sequence is: state messages → optional `pre_model_hook` transformation → optional prepended `SystemMessage` from `prompt` (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:137-170`, `chat_agent_executor.py:672-679`). Nothing is silently filtered between state and model call; the only automatic intervention is appending a synthetic "Sorry, need more steps" AIMessage when the recursion budget is exhausted (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:684-692`).

**2. What gets dropped?**
Nothing automatically. Deletion happens only through explicit developer action: `RemoveMessage(id=...)` tombstones, the `RemoveMessage(id=REMOVE_ALL_MESSAGES)` full wipe (`libs/langgraph/langgraph/graph/message.py:209-234`, tested at `libs/langgraph/tests/test_messages_state.py:96-177, 312-338`), or overwriting history inside a `pre_model_hook`. Non-message ephemeral routing data can be kept out of durable state via `EphemeralValue` channels which clear after a step (`libs/langgraph/langgraph/channels/ephemeral_value.py:15-79`), but chat messages use persistent channels.

**3. Are tool messages retained?**
Yes — `ToolMessage` is just another `AnyMessage` in the list and survives like any other message (`libs/langgraph/langgraph/graph/message.py:14-22`). The framework actively protects tool-call pairing: invoking the agent raises `INVALID_CHAT_HISTORY` if an `AIMessage` with `tool_calls` lacks a matching `ToolMessage` (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:243-271`), and tools returning `Command` must include the matching `ToolMessage` unless they deliberately wipe all messages (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1545-1578`). This makes naive "drop old messages" trimming dangerous — removing a `ToolMessage` while keeping its requester breaks the invariant.

**4. Is memory per user/thread/session?**
Per **thread**, not per user. `thread_id` is the required primary key in `configurable` (`libs/checkpoint/langgraph/langgraph/checkpoint/base/__init__.py:186-192`); subgraphs get isolated state under `checkpoint_ns` (`libs/langgraph/langgraph/langgraph/pregel/_loop.py:325-340`). There is no user/account identity concept in the saver interface — multi-tenant mapping of user → thread is left to the caller (the README frames threads as enabling "multi-tenant chat applications" but delegates partitioning to app design, `libs/checkpoint/README.md:31-43`).

**5. Can history be edited or forked?**
Yes, extensively. `update_state(values, as_node=...)` injects edits attributed to any node (`libs/langgraph/langgraph/langgraph/pregel/main.py:2515-2526`); bulk superstep edits are supported (`main.py:1590-1623`); `as_node="__copy__"` creates a fork checkpoint tagged `"fork"` (`main.py:1800-1878`); invoking with a historical `checkpoint_id` time-travels, and the loop auto-writes a fork checkpoint so the replay becomes a new branch rather than rewriting history (`libs/langgraph/langgraph/langgraph/pregel/_loop.py:952-971`). Whole-thread copying/deletion exists via `copy_thread` / `delete_thread` / `prune` / `delete_for_runs` (`libs/checkpoint/langgraph/langgraph/checkpoint/base/__init__.py:320-415`).

## Architectural Decisions

1. **Memory as reducible state, not a bespoke store.** Conversation history is an ordinary channel whose merge semantics live in one pure function, `add_messages` (`libs/langgraph/langgraph/langgraph/graph/message.py:61-244`). This means any state key can become conversational memory, and custom reducers can replace it — the framework imposes only the convention.
2. **Append-mostly with ID-based upsert.** Rather than immutable logs or last-write-wins, the reducer supports patching existing messages by ID (streaming partial chunks converge into single messages) and tombstone deletion (`message.py:215-234`). This is what enables both streaming accumulation and surgical history edits.
3. **Storage behind a protocol.** All persistence goes through `BaseCheckpointSaver` (`base/__init__.py:176-690`) with in-memory, SQLite, and Postgres implementations in-repo, plus a conformance harness that classifies mandatory vs optional capabilities (`libs/checkpoint-conformance/langgraph/checkpoint/conformance/capabilities.py:29-48`). Memory policy is thus orthogonal to backend choice.
4. **Checkpoints every superstep, keyed by (thread, namespace, id).** The loop snapshots state each step (`_loop.py:1031-1033` for input, `_put_checkpoint` at `_loop.py:1081+` for loop steps) with monotonically increasing UUIDv6-style IDs (`base/id.py`, used at `base/__init__.py:854`), making the entire conversation timeline replayable.
5. **Fork-on-replay instead of mutate-on-replay.** Time travel writes a `"fork"`-sourced checkpoint before continuing (`_loop.py:952-971`), preserving prior branches — history is treated as immutable audit trail with cheap branching.
6. **Context management pushed above the framework.** No windowing/summarization primitives exist in core or prebuilt; the documented extension points are `pre_model_hook` (model-only view) and reducer-level wipes (`chat_agent_executor.py:395-425`). Long-term/semantic memory is a different subsystem (`BaseStore`, `libs/checkpoint/langgraph/checkpoint/store`), deliberately separate from short-term state.

## Notable Patterns

- **Tombstone deletion protocol**: `RemoveMessage` objects flow through the same write path as real messages, letting deletions be checkpointed, replayed, and merged deterministically (`libs/langgraph/langgraph/graph/message.py:221-234`).
- **Dual-view pattern for model input**: `llm_input_messages` vs `messages` lets a hook show the model a compressed window while keeping canonical history intact — the schema is dynamically extended when `pre_model_hook` is set (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:724-742`).
- **Managed (non-persisted) state values**: `remaining_steps` is computed per-run from scratchpad position, never stored in checkpoints (`libs/langgraph/langgraph/langgraph/managed/is_last_step.py:18-24`), separating run-scoped control state from durable conversation state.
- **Blob-store + manifest split**: `InMemorySaver` stores channel values as versioned blobs and checkpoints as lightweight manifests referencing versions (`memory/__init__.py:125-140, 442-458`), avoiding duplicating unchanged channels across steps — the same layout the Postgres saver mirrors.
- **Pending-writes durability**: writes from tasks that completed within a failed superstep are persisted and reused on resume so successful work isn't redone (`libs/checkpoint/README.md:52-54`; write index map including error/interrupt slots at `libs/checkpoint/langgraph/langgraph/checkpoint/base/__init__.py:788-795`).
- **Conformance-driven ecosystem**: third-party savers are validated against generated spec tests rather than prose docs (`libs/checkpoint-conformance/langgraph/checkpoint/conformance/spec/test_put_writes.py`, `test_get_tuple.py`, etc.).

## Tradeoffs

- **Full-history fidelity vs context budget**: persisting and resending everything guarantees correctness (tool pairing, replay) but guarantees eventual context overflow without app-level trimming; the framework offers the mechanics (tombstones, hooks) but no policy.
- **Per-step checkpoint cost vs resumability**: snapshotting every superstep (with per-channel blobs to amortize) makes interrupts/time-travel trivially correct at the price of write amplification; the beta `DeltaChannel` machinery with frequency-bounded snapshots exists precisely to trade this back (`libs/checkpoint/langgraph/langgraph/checkpoint/base/__init__.py:63-86`).
- **Reducer flexibility vs safety**: anyone can return arbitrary `messages` updates from a node; the only guards are the orphaned-tool-call validations in the prebuilt agent and ToolNode (`chat_agent_executor.py:243-271`, `tool_node.py:1559-1578`) — core LangGraph itself won't stop you from corrupting pairing in a hand-rolled graph.
- **Thread-scoped simplicity vs multi-tenancy**: `thread_id`-keyed storage is simple and fast, but user-level queries, quotas, or cross-thread retention policies must be built above the saver API.
- **Strict serde security vs compatibility**: default deserialization accepts any Python type found in blobs; restricting it (`LANGGRAPH_STRICT_MSGPACK=true`) requires curating an allowlist, an opt-in friction point (`libs/checkpoint/README.md:49-50`).

## Failure Modes / Edge Cases

- **Unbounded growth → provider overflow**: no trimmer runs by default; a long-lived thread will eventually exceed model context. Mitigation exists (`RemoveMessage` + `pre_model_hook`, tested at `libs/prebuilt/tests/test_react_agent.py:1924-1954`) but must be wired up manually.
- **Broken tool pairing after trimming**: deleting an `AIMessage`'s `ToolMessage` (or vice versa) trips `_validate_chat_history` at the next model call (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:264-271`) — a runtime error, not a silent drop, but it surfaces far from the edit site.
- **Deleting nonexistent message IDs raises**: `add_messages([HumanMessage(id="1")], RemoveMessage(id="2"))` raises `ValueError` (`message.py:227-230`; test `libs/langgraph/tests/test_messages_state.py:118-124`), which can turn a benign race (two nodes trimming concurrently) into a failed superstep. Concurrent conflicting writes to a plain `LastValue` channel also raise `InvalidUpdateError` (`libs/langgraph/langgraph/channels/last_value.py:56-67`).
- **InMemorySaver volatility**: process death loses all threads; docs explicitly restrict it to debugging/testing and recommend Postgres (`memory/__init__.py:40-44`). Its `PersistentDict` pickle-based disk helper is legacy (`memory/__init__.py:628-698`).
- **Pruning can sever delta reconstruction**: naive `keep_latest` pruning on graphs using `DeltaChannel` can silently reconstruct channels as empty because reconstruction depends on ancestor writes/snapshots; the base class documents safe strategies (`base/__init__.py:374-415`).
- **Ambiguous update attribution**: `update_state` without `as_node` fails when two nodes updated state simultaneously (ambiguity check at `main.py:1921-1929`).
- **Time-travel interrupt staleness**: stale `INTERRUPT` writes are stripped when forking from a replay to avoid confusing multi-interrupt resume logic (`_loop.py:964-970`).

## Future Considerations

- Ship an official trimming/summarization middleware in-repo (the deprecation notice pointing `create_react_agent` users toward `langchain.agents` middleware, `chat_agent_executor.py:274-277`, suggests context management will land there); today every team reimplements token-budget pruning on top of `pre_model_hook`.
- Promote `DeltaChannel` out of beta and make pruners delta-aware by default (`base/__init__.py:386-414` currently relies on implementer discipline).
- Add user/tenant-level identity or retention hooks to the saver protocol to complement `thread_id` scoping.
- Extend the conformance suite into CI enforcement for the bundled Postgres/SQLite savers beyond the memory-saver smoke test (`libs/checkpoint-conformance/tests/test_validate_memory.py`).

## Questions / Gaps

- **No in-repo narrative documentation**: `docs/` contains only redirects/link manifests (`docs/llms.txt:14-15` points to hosted guides for "Add Memory"); claims about recommended windowing patterns could not be verified against implementation here beyond the `pre_model_hook` docstring (`chat_agent_executor.py:395-425`).
- **Summarization absent**: searched all libs for `trim_messages`, `summar`, and related terms — no summarization node/util exists in this repository (search boundary: `libs/**` Python sources and tests). If it exists, it lives in the external `langchain` package, outside this source tree.
- **Token accounting**: no evidence of tokenizer integration or message-cost estimation anywhere in the checkpoint/pregel/prebuilt code paths.
- **User-level scoping**: nothing in the saver interface addresses per-user isolation beyond the app choosing thread IDs; no evidence of auth/partition keys in `BaseCheckpointSaver`.

---

Generated by `05.01-short-term-conversation-memory` against `langgraph`.
