# Source Analysis: langgraph

## 11.02 Token Budgeting and Compression

### Source Info

| Field | Value |
|-------|-------|
| Name | langgraph |
| Path | `studies/agent-harness-study/sources/langgraph` |
| Language / Stack | Python (core, prebuilt, checkpoint libs) + TypeScript (sdk-js); monorepo at commit `f09cfe8` |
| Analyzed | 2026-08-25 |

## Summary

LangGraph contains **no token budgeting system**. A repo-wide search for token-counting mechanisms (`count_tokens`, `tiktoken`, `get_num_tokens`, `num_tokens`) across all libraries returns zero hits; the only occurrence of `max_tokens` in the entire repository is a commented-out example line (`libs/cli/examples/graphs/storm.py:27`). This is a deliberate architectural posture: LangGraph treats message history as durable graph state and delegates context-window management to user code via explicit extension points and reducer primitives.

What the framework does provide is:

1. **Safe state-manipulation primitives** — an append-only `add_messages` reducer with ID-based upsert and tombstone deletion (`libs/langgraph/langgraph/graph/message.py:60-244`), a `REMOVE_ALL_MESSAGES` sentinel for wholesale replacement (`libs/langgraph/langgraph/graph/message.py:38`, `209-213`), and an experimental batch-invariant `_messages_delta_reducer` (`libs/langgraph/langgraph/graph/message.py:247-309`).
2. **A dedicated injection point before the LLM call** — `pre_model_hook` on `create_react_agent`, documented as "Useful for managing long message histories (e.g., message trimming, summarization, etc.)" (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:396-397`), including a non-destructive `llm_input_messages` channel that changes what the model sees without mutating persisted state (`chat_agent_executor.py:400-409`, `636-658`).
3. **Step budgets instead of token budgets** — `recursion_limit` enforced by the Pregel loop (`libs/langgraph/langgraph/pregel/main.py:2563-2564`, `3005-3007`) and a `RemainingSteps` managed value that lets the prebuilt agent degrade gracefully ("Sorry, need more steps...") rather than crash (`libs/prebuilt/.../chat_agent_executor.py:620-634`, `684-692`; `libs/langgraph/langgraph/managed/is_last_step.py:18-24`).
4. **Checkpoint-storage compression (adjacent)** — the beta `DeltaChannel` stores only deltas in checkpoint blobs and reconstructs state by replaying ancestor writes, with snapshot cadence bounded by per-channel frequency and a global env-configurable superstep cap (`libs/langgraph/langgraph/channels/delta.py:25-64`; `libs/langgraph/langgraph/pregel/_checkpoint.py:50-71`; `libs/langgraph/langgraph/_internal/_config.py:33-35`). This compresses *storage*, not model context.
5. **Observability-only payload compression** — `TracePolicy.process_inputs/process_outputs` can "omit or summarize large payloads" but explicitly "Not intended to affect the value passed to the node" (`libs/langgraph/langgraph/types.py:548-558`; also documented at `libs/langgraph/langgraph/graph/state.py:700-703`).

Consequently, context overflow is not prevented or detected by the framework; it surfaces as a provider-side error. The framework's contribution is making user-implemented trimming safe: tombstoned deletes are validated, and broken tool-call/tool-result pairings after trimming raise `INVALID_CHAT_HISTORY` before reaching the provider (`chat_agent_executor.py:243-271`).

## Rating

**3 / 10** — Absent by design.

Token counting, token budget configuration, truncation logic, summarization triggers, and priority ranking are all absent from the codebase (search boundary described in "Questions / Gaps"). Per the rubric this maps to the 1–3 band. The score sits at 3 rather than 1 because the delegation strategy is coherent and well-engineered: the escape hatches (`pre_model_hook`, `llm_input_messages`, `RemoveMessage` semantics) have precise contracts, docstrings, and tests (`libs/prebuilt/tests/test_react_agent.py:1924-1954`), and the adjacent step-budget and storage-compression subsystems are mature. But against the dimension's core question — *"Can the system handle context overflow without losing critical information?"* — the honest answer is no: nothing in the framework measures tokens, so it cannot react to overflow at all.

## Evidence Collected

Every entry cites file paths relative to the source root with line numbers.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Token counters | None found. Repo-wide search for `count_tokens|tiktoken|get_num_tokens|num_tokens` over `*.py` returned zero matches; nearest concept is post-hoc `usage_metadata` passthrough on streamed messages, never acted upon | `libs/sdk-py/tests/streaming/_events.py:168-177` (test fixture projecting `input_tokens`/`total_tokens` into stream events) |
| Token usage passthrough only | `usage_metadata` appears solely as data carried on AI messages in streaming projections and doc examples; no code reads it to make decisions | `libs/sdk-py/langgraph_sdk/_async/runs.py:301`; `libs/langgraph/tests/test_pregel.py:3823` |
| No token budgets | Only `max_tokens` occurrence in repo is a comment in an example graph | `libs/cli/examples/graphs/storm.py:27` |
| Step budget: recursion limit | `DEFAULT_RECURSION_LIMIT = int(getenv("LANGGRAPH_DEFAULT_RECURSION_LIMIT", "10007"))`; validated `>= 1`; GraphRecursionError raised when reached | `libs/langgraph/langgraph/_internal/_config.py:32`; `libs/langgraph/langgraph/pregel/main.py:2563-2564`, `3005-3007`; `libs/langgraph/langgraph/errors.py:71-83` |
| Step budget: RemainingSteps | Managed value computed as `stop - step`; prebuilt agent checks `remaining_steps < 2 and has_tool_calls` and substitutes a graceful final AIMessage | `libs/langgraph/langgraph/managed/is_last_step.py:18-24`; `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:620-634`, `684-692`, `435-440` |
| Context-extension point | `pre_model_hook` param on `create_react_agent`: "Useful for managing long message histories (e.g., message trimming, summarization, etc.)"; hook node wired before agent node | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:296`, `396-424`, `795-800`, `876-881` |
| Non-destructive LLM view | Hook may return `llm_input_messages` used as model input without updating state; `_get_model_input_state` prefers it over `messages` | `chat_agent_executor.py:400-409`, `636-658`; schema injected at `724-742` |
| Wholesale history replacement | `RemoveMessage(id=REMOVE_ALL_MESSAGES)` sentinel handled in reducer: `return right[remove_all_idx + 1:]` | `libs/langgraph/langgraph/graph/message.py:38`, `209-213` |
| Tombstone deletion semantics | `add_messages` merges by ID; `RemoveMessage` deletes existing IDs; deleting a nonexistent ID raises `ValueError` | `libs/langgraph/langgraph/graph/message.py:216-234` |
| Post-trim safety guard | `_validate_chat_history` raises `INVALID_CHAT_HISTORY` if any AIMessage tool_call lacks a matching ToolMessage before each model call | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:243-271`, invoked at `651` |
| Summarization triggers | None built-in. Only trace-level payload summarization via `TracePolicy`, explicitly non-execution-affecting | `libs/langgraph/langgraph/types.py:532-558`; `libs/langgraph/langgraph/graph/state.py:700-703` |
| Priority/ranking of context | None found. Message order is strictly append/merge order preserved by the reducer; no scoring, selection, or reordering code | `libs/langgraph/langgraph/graph/message.py:60-244` |
| Storage compression (adjacent) | `DeltaChannel` stores sentinel in blobs, replays ancestor writes through reducer; requires batching-invariant reducers | `libs/langgraph/langgraph/channels/delta.py:25-64`, `139-157` |
| Snapshot cadence policy | Pure predicate: snapshot when updates >= `snapshot_frequency` (default 1000) OR supersteps >= `DELTA_MAX_SUPERSTEPS_SINCE_SNAPSHOT` (default 5000, env-tunable) | `libs/langgraph/langgraph/pregel/_checkpoint.py:50-71`; `libs/langgraph/langgraph/channels/delta.py:74`; `libs/langgraph/langgraph/_internal/_config.py:33-35` |
| Batch-invariant message reducer | `_messages_delta_reducer` processes dedup + tombstones in one pass; batching-invariant by contract | `libs/langgraph/langgraph/graph/message.py:247-309` |
| Tests: pre_model_hook behavior | `test_pre_model_hook` verifies both `llm_input_messages` (state untouched) and `REMOVE_ALL_MESSAGES` overwrite paths | `libs/prebuilt/tests/test_react_agent.py:1924-1954` |
| Tests: delta snapshot bound | Force-snapshot after N idle supersteps prevents unbounded ancestor walks; plus migration, exit-mode, update_state, id-stability, benchmark suites | `libs/langgraph/tests/test_delta_channel_supersteps_bound.py:1-31`; sibling files `test_delta_channel_migration.py`, `test_delta_channel_exit_mode.py`, `test_delta_channel_update_state.py`, `test_channels.py:329-516` |

## Answers to Dimension Questions

**1. Is token usage measured before calling the model?**
No. There is no pre-call measurement anywhere in the repository. The search for `count_tokens`, `tiktoken`, `get_num_tokens`, and `num_tokens` across every library returned zero results. Token usage appears only as `usage_metadata` attached to streamed AI messages — data that flows through observability channels but is never read by framework logic (`libs/sdk-py/tests/streaming/_events.py:168-177`). The prompt runnable passes full state messages straight to the model with no size gate (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:137-170`).

**2. What gets dropped when budget is exceeded?**
Nothing is dropped automatically, because there is no token budget to exceed. When the *step* budget (`recursion_limit`) is exhausted, the Pregel loop raises `GraphRecursionError` naming the config key to raise (`libs/langgraph/langgraph/pregel/main.py:3005-3007`); the prebuilt ReAct agent intercepts near-exhaustion one step earlier and returns a canned final message `"Sorry, need more steps to process this request."` instead of continuing the loop (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:684-692`). If the *context window* overflows, the failure surfaces uncaught from the provider call inside `call_model` (`chat_agent_executor.py:677-679`) — subject to node retry policies, but with no framework-side content reduction first.

**3. Is summarization faithful?**
Not applicable — there is no built-in summarization to evaluate. Summarization is entirely user-supplied via `pre_model_hook` (`chat_agent_executor.py:396-424`), so fidelity is the user's responsibility. Two design choices support faithfulness indirectly: (a) the `llm_input_messages` path lets users summarize *for the model only*, leaving canonical state intact so future turns can re-read original content (`chat_agent_executor.py:400-409`); (b) destructive rewriting is validated — trimmed histories with orphaned tool calls fail fast with `INVALID_CHAT_HISTORY` (`chat_agent_executor.py:243-271`). Separately, `TracePolicy` can summarize trace payloads, but its docstring states it is not intended to affect execution values (`libs/langgraph/langgraph/types.py:550-558`).

**4. Is budget configurable?**
Token budgets: no configuration exists. Step budgets: yes, at two granularities — a per-invocation `recursion_limit` config key (validated `>= 1` at `libs/langgraph/langgraph/pregel/main.py:2563-2564`) whose default comes from env var `LANGGRAPH_DEFAULT_RECURSION_LIMIT` (`libs/langgraph/langgraph/_internal/_config.py:32`), and storage-compression cadence: per-channel `snapshot_frequency=1000` (`channels/delta.py:74`) plus a process-wide env-tunable `LANGGRAPH_DELTA_MAX_SUPERSTEPS_SINCE_SNAPSHOT=5000` (`_internal/_config.py:33-35`). Neither is expressed in tokens or tied to any model.

## Architectural Decisions

1. **Context management is delegated, not owned.** LangGraph positions itself below the context-policy layer: it persists full message history as state and exposes `pre_model_hook` as the sanctioned interception point, explicitly advertising trimming/summarization as user concerns (`chat_agent_executor.py:396-397`). This mirrors the dependency structure where token-aware utilities like `trim_messages` live in `langchain-core`/`langchain`, outside this repo.
2. **Non-destructive views over destructive edits.** The `llm_input_messages` input-schema extension (`chat_agent_executor.py:724-742`) lets a hook show compressed context to the model while preserving the authoritative transcript — the same separation `TracePolicy` applies to traces (`types.py:544-552`). Canonical state remains replayable from checkpoints.
3. **Budgets expressed in supersteps, not tokens.** The only hard budgets are `recursion_limit` and `RemainingSteps`. This keeps the runtime model-agnostic (LangGraph binds arbitrary providers via `init_chat_model`, `chat_agent_executor.py:569-580`) at the cost of no protection against single-step context blowups.
4. **Compression applied to storage, not prompts.** Engineering effort went into `DeltaChannel` — delta-encoded checkpoints with bounded replay depth — which addresses checkpoint blob growth, an infra concern, rather than LLM context limits (`channels/delta.py:26-55`).

## Notable Patterns

- **Sentinel-based bulk delete**: `REMOVE_ALL_MESSAGES = "__remove_all__"` short-circuits the merge and returns only post-sentinel writes (`graph/message.py:209-213`), giving hooks a one-line "replace everything" idiom documented directly in the hook docstring (`chat_agent_executor.py:416-424`).
- **Fail-fast referential integrity**: after any user trimming, `_validate_chat_history` re-checks tool-call/result pairing before every model invocation (`chat_agent_executor.py:651`), converting a would-be provider API error into a typed local error (`errors.py`, `ErrorCode.INVALID_CHAT_HISTORY`).
- **Pure-predicate scheduling**: `delta_channels_to_snapshot()` isolates the snapshot decision into a side-effect-free function over `(channels, counters)` (`pregel/_checkpoint.py:50-61`), which makes the cadence policy trivially unit-testable (`tests/test_delta_channel_supersteps_bound.py` patches the constant directly).
- **Batching-invariant reducers**: `DeltaChannel` requires `reducer(reducer(state, xs), ys) == reducer(state, xs + ys)` (`channels/delta.py:41-48`) — a formal contract that lets write replay be coalesced without changing reconstructed state; `_messages_delta_reducer` implements this for message lists (`graph/message.py:250-262`).

## Tradeoffs

- **Model-agnosticism vs. safety**: by refusing to count tokens, LangGraph works uniformly across providers with no tokenizer dependencies, but offers zero defense against context overflow — a long-running thread grows until the provider rejects it.
- **Durability vs. prompt size**: persisting the full transcript means resumable conversations and time-travel debugging, but every turn pays the full-history cost unless the user builds trimming themselves.
- **Flexibility vs. discoverability**: `pre_model_hook` is powerful but opt-in; nothing warns a developer who is silently accumulating an oversized history.
- **Delta encoding vs. read cost**: `DeltaChannel` shrinks checkpoint blobs but shifts cost to reconstruction walks; the superstep cap (5000) bounds worst-case replay depth at the price of periodic full snapshots (`channels/delta.py:50-55`, marked Beta with unstable surrounding contract at lines 29-36).

## Failure Modes / Edge Cases

- **Unbounded history growth**: default agents accumulate messages forever; the only automatic stop is the step budget, which converts the problem into `"Sorry, need more steps..."` output (`chat_agent_executor.py:684-692`) — a graceful degradation that masks the underlying context pressure.
- **Provider exceptions as the overflow signal**: with no measurement, the first symptom of overflow is typically an exception from `model.invoke` inside the agent node; behavior then depends entirely on user-configured retry policies (`graph/state.py:695-697`).
- **Trimming hazards contained**: naive trimming that drops a ToolMessage paired with a pending tool call now fails deterministically in-graph with `INVALID_CHAT_HISTORY` rather than at the provider (`chat_agent_executor.py:264-271`).
- **Deleting nonexistent IDs**: `add_messages` raises on `RemoveMessage` for unknown IDs (`graph/message.py:227-230`), preventing silent no-op deletions during concurrent updates.
- **Experimental-path parity gaps**: `_messages_delta_reducer` intentionally does not handle `REMOVE_ALL_MESSAGES` or unknown-id error cases (`graph/message.py:260-262`), so behavior differs between the standard and delta-channel message paths.

## Future Considerations

- A token-aware middleware between `pre_model_hook` and the model call could measure `usage_metadata` already flowing on messages (`libs/sdk-py/tests/streaming/_events.py:168-177`) and apply configurable budgets without new dependencies, since usage data is present post-first-turn.
- Surfacing `remaining_steps`-style degradation for context pressure (analogous to `RemainingSteps`) would let agents summarize proactively instead of crashing mid-loop; the hook plumbing (`llm_input_messages`, `chat_agent_executor.py:636-658`) already supports the required shape.
- Promoting `DeltaChannel` out of beta and reconciling `_messages_delta_reducer`'s missing edge cases with `add_messages` semantics (`graph/message.py:260-262`) would unify the two message-reduction paths.
- In-repo documentation cannot help here: docs have moved offsite (`docs/llms.txt:3`), so guidance on trimming lives outside audited source.

## Questions / Gaps

- **Where does official trimming/summarization live now?** The deprecated `create_react_agent` points to `create_agent` in the `langchain` package (`chat_agent_executor.py:53-56`, `274-277`), which is outside this source tree; per isolation rules I did not inspect sibling sources. Within-langgraph, no successor implementation exists.
- **Does anything server-side bound request size?** `langgraph_sdk` and `langgraph_cli` were searched for payload/token limits; none found beyond unrelated help-text truncation (`libs/cli/langgraph_cli/cli.py:212`) and namespace-depth caps in the store (`libs/checkpoint/langgraph/store/base/__init__.py:421`). Any such limits would live in the closed-source LangGraph Server, not in this repo.
- **Search boundary**: findings cover `libs/langgraph`, `libs/prebuilt`, `libs/checkpoint*`, `libs/sdk-py`, `libs/sdk-js`, `libs/cli`, `examples/`, and `docs/`. Greps used: `count_tokens|tiktoken|get_num_tokens|num_tokens`, `max_tokens|maxTokens|context_window`, `trim|truncat`, `summariz`, `usage_metadata|input_tokens|total_tokens`, `gzip|zlib|compress` (Python), and token/trim/summarize patterns over `*.ts`. All negative results except those cited above.

---

Generated by `11.02-token-budgeting-and-compression` against `langgraph`.
