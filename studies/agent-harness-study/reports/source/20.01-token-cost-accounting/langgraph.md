# Source Analysis: langgraph

## Dimension 20.01: Token and Cost Accounting

### Source Info

| Field | Value |
|-------|-------|
| Name | langgraph |
| Path | `studies/agent-harness-study/sources/langgraph` |
| Language / Stack | Python monorepo (`libs/langgraph` core, `libs/prebuilt`, `libs/sdk-py`, `libs/checkpoint-*`); JS SDK moved to external langgraphjs repo (`libs/sdk-js/README.md:1-9`) |
| Analyzed | 2026-08-26 |

## Summary

LangGraph implements **no first-class token counting, cost calculation, or per-run cost summary**. As a low-level orchestration framework, it deliberately delegates usage metering two levels down and one level out:

1. **Downstream to langchain-core**: token counts arrive as the standardized `usage_metadata` field on `AIMessage` objects produced by chat models. LangGraph's role is purely transportal — it streams those messages through its `messages` stream mode (`libs/langgraph/langgraph/pregel/_messages.py:49-256`) and, in the experimental v3 streaming protocol, exposes per-LLM-call `ChatModelStream` handles whose typed projections include `.usage` (`libs/langgraph/langgraph/stream/transformers.py:155-197`). The `ChatModelStream` class itself, including usage accumulation, is imported from langchain-core (`libs/langgraph/langgraph/stream/transformers.py:6-11`, `libs/langgraph/langgraph/stream/run_stream.py:14-18`).
2. **Outward to LangSmith**: operational metrics, tracing, and observability are positioned as a LangSmith product capability, not a framework feature (`README.md:42`, `README.md:56`).

There is no cost calculator anywhere in the repository (a repo-wide search for `cost`/`price`/`pricing` returns only unrelated hits and one docstring example naming "cost lookup" as a hypothetical custom transformer task, `libs/langgraph/langgraph/stream/_types.py:70-74`). Tool execution carries no cost fields; retries track attempt counts and backoff timings but not their token/cost impact; and run-lifecycle completion events carry status/error but no aggregated usage. Consequently, "what did this run cost?" cannot be answered from LangGraph itself in under a minute — a user must either sum `usage_metadata` over all messages themselves (possible via state inspection or the messages stream, with correctness aided by LangGraph's message-dedupe machinery) or rely on an external observability backend such as LangSmith.

**Rating rationale**: token visibility exists only as an *implicit passthrough* of upstream metadata; everything else in this dimension (model-cost attribution, tool cost, retry cost, per-run summaries) is absent. This places the source in the lowest rubric band, at the top of it because the passthrough surfaces are well-designed and tested.

## Rating

**3 / 10** — Absent/implicit. Per-model-call token counts are observable, but only because langchain-core embeds them in messages that LangGraph transports. There is no aggregation, no cost model, no tool/retry cost accounting, and no per-run summary anywhere in the framework.

## Evidence Collected

Every entry cites a path relative to the workspace root, format `path/to/file.py:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Token counters | None in-repo. `usage_metadata` appears only in tests as opaque passthrough data attached to messages/state by users | `studies/agent-harness-study/sources/langgraph/libs/langgraph/tests/test_pregel.py:3823`; `studies/agent-harness-study/sources/langgraph/libs/langgraph/tests/test_remote_graph.py:1351` |
| Message-stream transport of usage-bearing messages | `StreamMessagesHandler` streams `AIMessageChunk`s and emits the finalized message on `on_llm_end`; usage rides inside the message object | `studies/agent-harness-study/sources/langgraph/libs/langgraph/langgraph/pregel/_messages.py:49-58`, `151-164`, `166-179` |
| v3 per-LLM-call usage projection | `MessagesTransformer` docstring lists typed projections `.text`, `.reasoning`, `.tool_calls`, `.usage`, `.output` on each `ChatModelStream`; implementation imported from langchain-core, not local | `studies/agent-harness-study/sources/langgraph/libs/langgraph/langgraph/stream/transformers.py:155-197` (projections at 161), `6-11` |
| Wire-level token usage (server protocol) | Test fixture defines `message-finish` event carrying `usage: {input_tokens, output_tokens, total_tokens}` | `studies/agent-harness-study/sources/langgraph/libs/sdk-py/tests/streaming/_events.py:164-182` |
| Client reconstruction of usage_metadata | SDK messages projection asserts final `AIMessage.usage_metadata == {input_tokens: 2, output_tokens: 3, total_tokens: 5}` from scripted server events | `studies/agent-harness-study/sources/langgraph/libs/sdk-py/tests/streaming/test_messages_projection.py:39-75` |
| Usage routing decoder | `MessagesDecoder` routes `message-start/delta/finish` events to per-message stream handles; `message-finish` closes the handle (usage lands in `output` projection) | `studies/agent-harness-study/sources/langgraph/libs/sdk-py/langgraph_sdk/stream/decoders.py:139-196` |
| Cost calculators | None. Only occurrence of "cost lookup" is a docstring example of a hypothetical async transformer task ("async moderation scoring, cost lookup, external tracing") | `studies/agent-harness-study/sources/langgraph/libs/langgraph/langgraph/stream/_types.py:70-74` |
| Tool cost tracking | Absent. Tool channel events carry `tool_call_id`, `name`, `input`, `output`, error text only — no cost/latency/token fields | `studies/agent-harness-study/sources/langgraph/libs/sdk-py/langgraph_sdk/stream/decoders.py:199-254` |
| Retry cost accounting | Absent. `RetryPolicy` models intervals/backoff/max_attempts/jitter/retry_on only | `studies/agent-harness-study/sources/langgraph/libs/langgraph/langgraph/types.py:418-437` |
| Retry attempt observability (no cost) | Retry loops increment `attempts`, publish `node_attempt` into runtime, and log retry sleep times; timed-attempt observer records timestamps/status but no token fields | `studies/agent-harness-study/sources/langgraph/libs/langgraph/langgraph/pregel/_retry.py:600-682`, `87-107`, `343-404` |
| Run cost summaries | Absent. Lifecycle/subgraph terminal events carry `(status, error)` only; run-stream native projections are values/messages/etc., with no usage roll-up | `studies/agent-harness-study/sources/langgraph/libs/langgraph/langgraph/stream/transformers.py:582-605`; `studies/agent-harness-study/sources/langgraph/libs/langgraph/langgraph/stream/run_stream.py:36-60` |
| Metrics integrations | No OpenTelemetry/Prometheus/metrics hooks in core (grep for `opentelemetry|otel|prometheus|metrics` returns nothing under `libs/langgraph/langgraph`); observability delegated to LangSmith | `studies/agent-harness-study/sources/langgraph/README.md:42`, `README.md:56-57` |
| Double-count safeguard (indirect accounting aid) | Dedupe by message id prevents emitting the same finalized message twice; v2 handler explicitly documents avoiding double-counting between streamed chunks and chain output | `studies/agent-harness-study/sources/langgraph/libs/langgraph/langgraph/pregel/_messages.py:97-104`, `398-408` |

## Answers to Dimension Questions

1. **Are tokens counted per run?**
   No. Nothing aggregates tokens across a run. Per-run totals exist only if the user sums `usage_metadata` across messages in final state or across streamed messages. The pregel engine never inspects usage; a graph node returning `{"usage_metadata": {"total_tokens": 123}}` is just opaque state (`libs/langgraph/tests/test_pregel.py:3823`). Per *message*, counts flow through faithfully (v1 messages mode, v3 `ChatModelStream.usage`, SDK wire protocol).

2. **Are costs attributed per model call?**
   Token counts are attributable per model call (one `ChatModelStream` per LLM call, correlated by run_id — `libs/langgraph/langgraph/stream/transformers.py:203-205`, `292-322`), but *monetary* costs are never computed. No pricing tables, cost multipliers, or currency handling exist anywhere in the repo.

3. **Are tool execution costs tracked?**
   No. The tools channel tracks lifecycle (`tool-started`, `tool-output-delta`, `tool-finished`, `tool-error`) with id/name/input/output payloads only (`libs/sdk-py/langgraph_sdk/stream/decoders.py:216-254`). Non-LLM tool invocation costs (API fees, compute) are entirely out of scope of the framework.

4. **Are retry costs accounted for?**
   No. `run_with_retry`/`arun_with_retry` count failed attempts, apply exponential backoff, log timings (`libs/langgraph/langgraph/pregel/_retry.py:657-682`, `812-838`), and expose the current attempt number via `node_attempt` in `Runtime` (`_retry.py:607-611`, `726-730`). The `_AttemptContext` observer contract records task/attempt/run/thread ids and timestamps (`_retry.py:87-107`) — but contains no token, cost, or even LLM-call-count field. Work performed by discarded failed attempts is invisible to any ledger.

5. **Are per-run cost summaries available?**
   No. Terminal lifecycle events emit only `(status, error)` (`libs/langgraph/langgraph/stream/transformers.py:591-605`); the run stream's final output is the graph's return value. A consumer wanting a per-run figure must build it externally (e.g., fold over streamed messages' `usage_metadata`, or export traces to LangSmith per the README positioning, `README.md:42`).

## Architectural Decisions

- **Delegation over implementation.** Usage metering lives in langchain-core's message schema and `ChatModelStream`; LangGraph's contract is "transport messages, don't interpret them." Evidence: `ChatModelStream`/`AsyncChatModelStream` are TYPE_CHECKING imports from langchain_core (`libs/langgraph/langgraph/stream/run_stream.py:14-18`) and `message_to_events` comes from `langchain_core.language_models._compat_bridge` (`libs/langgraph/langgraph/stream/transformers.py:6`).
- **Observability outsourced to LangSmith.** The README names LangSmith as the place for "runtime metrics" and production visibility (`README.md:42`, `README.md:56-57`); no neutral OpenTelemetry hook is offered in-core.
- **Extensibility point instead of a feature.** Rather than shipping a cost tracker, the v3 stream mux invites users to register custom `StreamTransformer`s, with "cost lookup" given as the canonical example of decoupled async work (`libs/langgraph/langgraph/stream/_types.py:44-74`).
- **Identity stability as an accounting prerequisite.** `ensure_message_ids` stamps stable UUIDs onto messages before checkpoint writes so replays don't mint new identities (`libs/langgraph/langgraph/pregel/_messages.py:426-461`) — a correctness foundation for anyone summing usage over replayed history, even though LangGraph itself never sums.

## Notable Patterns

- **Usage piggybacks on content.** Token counts ride inside `AIMessage.usage_metadata` (schema-visible in snapshots, `libs/langgraph/tests/__snapshots__/test_large_cases.ambr:91`), so every existing message pathway (state, values mode, messages mode, checkpoints) doubles as a usage carrier without dedicated plumbing.
- **Per-handle correlation by run_id/message_id.** Both local (`MessagesTransformer._by_run`, `transformers.py:203-205`, `315-322`) and remote (`MessagesDecoder._active` keyed by `message:<id>`, `decoders.py:64-76`) route usage-bearing finish events to exactly one stream, keyed on message_id to survive concurrent same-run messages (`test_messages_projection.py:148-178`).
- **Dedupe-before-emit.** `_emit(..., dedupe=True)` plus the v2 `_streamed_run_ids` guard ensure a message (and its usage payload) is surfaced once whether it arrived via streaming or as node output (`_messages.py:97-104`, `307-355`, `398-406`).
- **Attempt-indexed runtime metadata.** Retries stamp `node_attempt` (1-indexed) into `Runtime.execution_info` each loop (`_retry.py:600-612`, `719-731`), giving downstream observers a key to attribute per-attempt behavior — a natural join point if cost were ever tracked.

## Tradeoffs

- **Simplicity vs. answerability.** Zero in-framework accounting keeps the core small and provider-neutral, but leaves the "what did this run cost?" question unanswered without external tooling — the exact failure mode this dimension probes.
- **Passthrough fidelity vs. aggregation.** Faithful message-level transport (with tested wire reconstruction, `sdk-py/tests/streaming/test_messages_projection.py:71-75`) means accurate inputs exist, yet the last-mile aggregation is left to every user.
- **Vendor-coupled observability.** Delegating metrics to LangSmith (`README.md:42`) yields deep integration for that platform but no vendor-neutral cost surface (no OTel exporter) inside the repo.
- **Retry transparency vs. cost opacity.** Attempt counts and backoff logs make *that* retries happened observable (`_retry.py:677-680`), but the duplicated token spend of retried LLM calls is neither recorded nor attributed to the surviving attempt.

## Failure Modes / Edge Cases

- **Silent absence**: if a chat model doesn't populate `usage_metadata` (e.g., non-LangChain model invoked in a node), runs proceed with no signal that usage was lost — there is no validator or warning anywhere in the pregel loop.
- **Streaming failure drops usage mid-flight**: on run error, open `ChatModelStream` handles are failed with the run error (`transformers.py:335-339`) — any partial usage accumulated before the failure is unrecoverable from the projection.
- **Subgraph exclusion**: the messages projection only surfaces events at the caller's own namespace level; deeper subgraph tokens are "left in the main event log but excluded from `.messages`" (`transformers.py:184-191`), so a naive sum over `run.messages` understates hierarchical agent spend.
- **Legacy-mode gaps**: v1 `AIMessageChunk` tuples from `on_llm_new_token` are intentionally ignored by the v3 projection (`transformers.py:177-188`, `286-288`); consumers on legacy paths get usage only if they inspect the final message themselves.
- **Retried-attempt blind spot**: cleared writes on retry (`task.writes.clear()`, `_retry.py:615`, `738`) mean a failed attempt's partial outputs — including any usage-bearing messages it emitted to the stream — are wiped without ledger compensation.
- **Timeout cancellation**: `NodeTimeoutError` paths cancel background work and discard writes (`_retry.py:486-506`); in-flight LLM calls that completed server-side still incurred cost that no record captures.

## Future Considerations

- Add a usage-reducing channel or a built-in `StreamTransformer` that folds `usage_metadata` across all messages (root + subgraphs) into a per-run summary on the lifecycle/completed event.
- Extend `_AttemptContext`/`_AttemptEvent` (`_retry.py:87-127`) with optional token/cost fields so timed-attempt observers (consumed by langgraph-server, per the docstring) can attribute spend per attempt, making retry cost visible.
- Provide a vendor-neutral metrics hook (OpenTelemetry-compatible) alongside the LangSmith-oriented story in `README.md:42-57`.
- Ship a reference `CostTransformer` implementing the "cost lookup" example sketched in `stream/_types.py:70-74`, turning the documented extension pattern into supported functionality.

## Questions / Gaps

- **No evidence found** for any monetary cost computation, pricing configuration, budget/quota enforcement, or spend alerts: searches for `cost`, `price`, `pricing`, `budget` across `libs/**/*.py` returned only unrelated store-test fixtures (`checkpoint-sqlite/tests/test_store.py:1122-1154`) and a test-retry deadline constant (`sdk-py/integration/scripts/test_update_state.py:37`).
- **No evidence found** for tool-invocation cost or latency capture: the tools wire schema (`sdk-py/langgraph_sdk/stream/decoders.py:199-254`) carries no such fields, and core `_tools.py` concerns itself solely with writer plumbing.
- **No evidence found** for token-aware context management in `prebuilt`: `chat_agent_executor.py` mentions message trimming/summarization only as a user-supplied `prompt` concern (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:397`); any token counting behind that remains in langchain-core, outside this boundary.
- Whether the hosted LangGraph Server aggregates usage per run could not be verified from this repository: the OSS SDK only reconstructs per-message `usage_metadata` (`sdk-py/tests/streaming/test_messages_projection.py:71-75`); server internals are not part of this source tree.

---

Generated by `dimensions/20.01-token-cost-accounting.md` against `langgraph`.
