# Source Analysis: langgraph

## 20.03 Quality-Cost Routing

### Source Info

| Field | Value |
|-------|-------|
| Name | langgraph |
| Path | `studies/agent-harness-study/sources/langgraph` |
| Language / Stack | Python (langgraph core + prebuilt, langchain-core) |
| Analyzed | 2026-08-26 |

## Summary

LangGraph is a model-agnostic graph orchestration framework; it deliberately contains no quality-cost routing layer. There is no multi-model tier config, no cost/latency/quality/risk router, no fallback-chain abstraction, and no routing-decision tracing. The only related primitives are (a) a dynamic-model callable in `create_react_agent` that lets user code select a model at runtime (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:279-356`), and (b) generic per-node resilience primitives (`RetryPolicy`, `TimeoutPolicy`, `error_handler` fallback nodes, cache) that can be repurposed for fallback but are not model-aware. All routing that exists in the codebase is graph control-flow routing (`add_conditional_edges` / `Send`), not model-tier routing. LangChain's `init_chat_model("provider:model")` and `Runnable.with_fallbacks()` exist outside LangGraph and must be composed by the user.

## Rating

**2 / 10 — Absent / Ad-hoc**

No first-class model tier, cost/latency/quality router, or fallback-chain is implemented in the selected source. Dynamic model selection via a user-supplied callable provides an extension point but without criteria, policy, observability, or tests for quality-cost decisions. Retry/error-handler gives generic failure fallback, not cheap→expensive escalation. Rating is not 1 because the extension point is explicit, typed, tested, and documented; but it does not satisfy any of the dimension's positive criteria.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Multi-model config | No multi-model or tier config exists in `libs/langgraph/langgraph` or `libs/prebuilt`. `pyproject.toml` dependencies are `langchain-core`, `langgraph-checkpoint`, `langgraph-sdk`, `langgraph-prebuilt`, `xxhash`, `pydantic` — no model-tier registry. | `libs/langgraph/pyproject.toml:26-33` |
| Dynamic model extension point | `create_react_agent` accepts `model` as `str \| LanguageModelLike \| Callable[[State, Runtime[Context]], BaseChatModel]` enabling user-implemented routing. Static string path uses `init_chat_model` (`cast(BaseChatModel, init_chat_model(model))`). | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:278-332`, `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:569-580` |
| Dynamic model runtime resolution | `_resolve_model` / `_aresolve_model` composes prompt runnable with user-selected model at each `call_model` invocation; no cost/quality logic inside. | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:599-618`, `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:661-695` |
| Cost/latency/quality criteria | No search hit for `cost`, `latency`, `quality`, `price`, `tier` as routing criteria in `libs/langgraph/langgraph` outside benchmarks/docs (`bench` latency is graph execution, not model selection). | `libs/langgraph/langgraph/bench/__main__.py:36`, `libs/langgraph/README.md:24` |
| Graph routing != model routing | Framework routing is `add_conditional_edges` / `BranchSpec` / `Send` for node selection, not model selection. Example adaptive-RAG "router" is a user prompt `RouteQuery` chain, not a framework router. | `libs/langgraph/langgraph/graph/state.py:982-1030`, `examples/rag/langgraph_adaptive_rag.ipynb:196-212` |
| Fallback chain definitions | No `with_fallbacks` or model fallback chain in `libs/langgraph`. Only generic node-level `RetryPolicy` (retry on failure with backoff) and `error_handler` fallback nodes. Verified by `grep with_fallbacks` returning 0 hits under `libs/langgraph/langgraph`. | `libs/langgraph/langgraph/types.py:418-437`, `libs/langgraph/langgraph/graph/state.py:298-332` |
| Retry policy implementation | `RetryPolicy(initial_interval, backoff_factor, max_interval, max_attempts, retry_on)` and matcher `_should_retry_on` handle retries; sleep with jitter, logging. Not model-aware. | `libs/langgraph/langgraph/types.py:418-437`, `libs/langgraph/langgraph/pregel/_retry.py:641-682`, `libs/langgraph/langgraph/pregel/_retry.py:841-854` |
| Error-handler fallback node | `add_node(..., error_handler=...)` creates auto-named `__error_handler__{node}` subgraph; `set_node_defaults(error_handler=...)` provides default fallback node. Handler is not retried, not chained, and failures fail the run. | `libs/langgraph/langgraph/graph/state.py:1291-1310`, `libs/langgraph/langgraph/graph/_node.py:97-98` |
| Timeout as latency-related fallback | `TimeoutPolicy(run_timeout, idle_timeout, refresh_on)` with `NodeTimeoutError` and retry integration; supports cooperative cancellation via `_TimedAttemptScope`. This is execution-time fallback, not model-tier fallback. | `libs/langgraph/langgraph/types.py:450-514`, `libs/langgraph/langgraph/pregel/_retry.py:62-84` |
| Routing decision traces | No routing-decision event type. Stream modes (`values`, `updates`, `messages`, `tasks`, `debug`, `checkpoints`) emit node execution and checkpoint metadata (`langgraph_node`, `langgraph_step`, `langgraph_triggers`) but not model-selection decisions. Dynamic model callable is opaque to tracing; `TracePolicy` only redacts/transforms node I/O recording. | `libs/langgraph/langgraph/types.py:122-136`, `libs/langgraph/langgraph/types.py:532-567` |
| Routing policy config | No routing-policy config object or config key. Configurable policies inspected: `RetryPolicy`, `CachePolicy`, `TimeoutPolicy`, `TracePolicy`, `StateGraph.set_node_defaults`. None encodes cost/quality. Config constants file exposes only graph constants (`START`, `END`, `TAG_NOSTREAM`, `TAG_HIDDEN`). | `libs/langgraph/langgraph/graph/state.py:272-335`, `libs/langgraph/langgraph/constants.py:10-26` |
| Tests for model routing | No tests for quality-cost routing. `test_retry.py` covers retry; `test_large_cases.py:6481-6506` defines a manual `router_node` with `FakeMessagesListChatModel` as a user graph pattern, not a framework feature. | `libs/langgraph/tests/test_retry.py:1-60`, `libs/langgraph/tests/test_large_cases.py:6481-6521` |

## Answers to Dimension Questions

**1. Are multiple model tiers available?**
No. LangGraph does not define or configure model tiers. The prebuilt agent `create_react_agent` accepts a single `model` (string identifier resolved via `langchain.chat_models.init_chat_model` at `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:572-580`) or a user-supplied callable returning a `BaseChatModel` (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:278-286`). Multiple models can only exist if the user instantiates them externally (e.g., `gpt4_model` vs `gpt35_model` in docstring example at `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:342-355`) and selects one inside the callable. No tier registry, no cost metadata, no declarative multi-model config.

**2. What criteria drive model selection?**
No framework criteria. The dimension's criteria (cost, latency, quality, risk) are not referenced anywhere in `libs/langgraph/langgraph` as model-selection inputs. The dynamic callable receives `(state, runtime)` with `runtime.context` typed by `ContextT` (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:599-604`), so users *could* implement cost/latency/quality logic externally, but the framework provides no scoring, threshold, classifier, or policy object. Search for `cost|latency|quality` under `libs/langgraph/langgraph` yields only bench latency measurements (`libs/langgraph/bench/__main__.py:36`) and doc claims, not routing logic.

**3. Are fallback chains defined?**
Not for models. LangGraph provides generic per-node resilience that superficially resembles fallback but is not model-tier fallback:
- `RetryPolicy` retries the *same* node with exponential backoff and optional `retry_on` filter (`libs/langgraph/langgraph/types.py:418-437`, `libs/langgraph/langgraph/pregel/_retry.py:573-682`). Sequence of policies picks first matching policy (`libs/langgraph/langgraph/pregel/_retry.py:649-655`).
- `error_handler` fallback node per `add_node` (`libs/langgraph/langgraph/graph/_node.py:98`) or default via `set_node_defaults(error_handler=...)` (`libs/langgraph/langgraph/graph/state.py:298-332`, `1302-1310`). This routes to a single handler node on exception, not a chain of model tiers, and `error_handler` failures fail the run.
- LangChain's `RunnableWithFallbacks` / `with_fallbacks()` is not wrapped or exposed by LangGraph; users must compose it themselves outside the graph.
No `on_error: {fallback_model: ...}` config, no ordered list of cheap→expensive models, no conditional fallback on validation vs. transient error.

**4. Are routing decisions observable?**
No model-routing decisions are observable. Graph routing (which node ran, which edge was taken) is observable via `stream_mode="tasks"/"debug"/"checkpoints"` emitting `TaskPayload`/`DebugPayload` with `langgraph_node`, `langgraph_step`, `langgraph_triggers`, `checkpoint_ns` (`libs/langgraph/langgraph/types.py:122-260`). Model selection inside the dynamic callable emits no event, is not logged by LangGraph's retry observer (`_AttemptContext`/`_AttemptEvent` at `libs/langgraph/langgraph/pregel/_retry.py:87-126` tracks attempt timing, not model identity), and `TracePolicy` (`libs/langgraph/langgraph/types.py:532-567`) only transforms the node's own trace payload. To observe which tier was chosen, users must add manual logging/callbacks inside their callable.

## Architectural Decisions

- **Model-agnostic orchestration:** LangGraph treats models as opaque `Runnable`/`BaseChatModel` instances injected by the user. This is explicit in `pyproject.toml` dependency on `langchain-core` (`libs/langgraph/pyproject.toml:27`) and in `create_react_agent` docstring deprecation note pointing to `langchain.agents.create_agent` as successor (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:311-317`). Tradeoff: maximum flexibility for custom graphs, but zero opinionated quality-cost behavior.
- **Resilience via generic `RetryPolicy` + `error_handler`, not model fallback:** Decisions at `libs/langgraph/langgraph/graph/state.py:272-335` and `libs/langgraph/langgraph/types.py:418-514` expose retry/timeout/cache/error-handler as node defaults applied at compile time (`libs/langgraph/langgraph/graph/state.py:1288-1325`). This isolates failure handling to node execution, keeping the pregel loop (`libs/langgraph/langgraph/pregel/_retry.py`) clean, but makes cheap→expensive escalation impossible without user code.
- **Dynamic model via callable + Runtime Context:** The pattern `Callable[[State, Runtime[ContextT]], BaseChatModel]` (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:279-283`) with `Runtime[ContextT]` propagation is the sole extension point for quality-cost routing. It benefits from typed `context_schema` (`StateGraph.__init__` at `libs/langgraph/langgraph/graph/state.py:216-264`) but puts all policy, caching, and observability burden on the user.
- **No built-in routing policy object:** The absence of a `RoutingPolicy`/`ModelTier` type alongside `RetryPolicy`/`TimeoutPolicy`/`CachePolicy` is an intentional non-decision; routing is delegated to conditional edges (`libs/langgraph/langgraph/graph/state.py:982-1030`) for graph topology, not model selection.

## Notable Patterns

- **User-implemented router as graph node (not framework):** Tests and examples show the idiomatic way to do routing is a manual graph node + conditional edge (e.g., `router_node` → `route_after_prediction` → `weather_graph` in `libs/langgraph/tests/test_large_cases.py:6496-6545` and `examples/rag/langgraph_adaptive_rag.ipynb:196`). This pattern leverages `add_conditional_edges` but requires the user to author the classifier LLM themselves.
- **Runnable composition for model variants:** `_should_bind_tools` / `_get_model` helpers (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:173-241`) and `_get_prompt_runnable` pattern show LangGraph composes `Runnable`s (`prompt | model`) rather than selecting among model tiers. Fallback, if desired, must be composed as `model.with_fallbacks([fallback_model])` from `langchain-core` before passing to the graph — undocumented in LangGraph but technically compatible because any `Runnable` is accepted.
- **Attempt observer for timeouts/retries (internal contract):** `_AttemptContext`/`_AttemptEvent`/`_TimedAttemptScope` (`libs/langgraph/langgraph/pregel/_retry.py:87-312`) with configurable `CONFIG_KEY_TIMED_ATTEMPT_OBSERVER` provides structured observability for retries/timeouts to `langgraph-server`, but is not exposed as a routing-decision trace.

## Tradeoffs

- **Flexibility vs. out-of-the-box cost optimization:** Being model-agnostic avoids lock-in and supports any provider, but teams wanting automatic cheap→expensive escalation must build it from scratch (classifier node, multiple `BaseChatModel` instances, custom callback for tracing). No code generation or template provides this.
- **Generic resilience vs. semantic fallback:** `RetryPolicy(max_attempts, backoff_factor, retry_on)` handles transient errors well with jitter and exponential backoff (`libs/langgraph/langgraph/pregel/_retry.py:664-681`), and `error_handler` provides a deterministic fallback path; however, neither distinguishes *why* to fall back to a better model (e.g., low confidence vs. tool-call parse failure vs. timeout). Using retry for quality fallback wastes latency and cost.
- **Dynamic callable power vs. operability:** The callable supports per-request context-aware selection (e.g., `runtime.context.model_name` at `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:352-355`) without framework changes, but without built-in caching, both models are re-instantiated or re-resolved on every step, and no framework-level rate limiting or cost accounting exists.

## Failure Modes / Edge Cases

- **No fallback on quality failure — silent wrong answer:** If a cheap model hallucinates or returns low-quality tool calls, LangGraph has no validation-triggered fallback. `RetryPolicy.retry_on` (`libs/langgraph/langgraph/types.py:434-437`) only retries on exceptions, not on semantic quality signals. Users must add a validation node + conditional edge manually.
- **Retry storm on overloaded cheap model:** `max_attempts=3` default with `initial_interval=0.5` and `jitter=True` (`libs/langgraph/langgraph/types.py:424-433`) will retry the same failing cheap model 3 times before surfacing the error to the `error_handler`. If the failure is rate limiting or quota, this increases cost/latency before any tier escalation.
- **Timeout vs. retry interaction gap:** Sync nodes cannot be timed out (`raise sync_timeout_unsupported` at `libs/langgraph/langgraph/pregel/_retry.py:583`), so latency-based routing (e.g., "if cheap model exceeds 2s, fall back") is only available for async graphs via `TimeoutPolicy` + `arun_with_retry` (`libs/langgraph/langgraph/pregel/_retry.py:695-839`). Sync `create_react_agent` with `is_async_dynamic_model` raises at call time (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:663-670`).
- **Unobservable routing decisions:** Because the dynamic model callable is opaque, incident response cannot answer "which tier answered request X" without user-added logging. `TracePolicy.process_inputs/process_outputs` (`libs/langgraph/langgraph/types.py:548-558`) can redact but not emit routing metadata.
- **Error handler not chained:** Only one `error_handler_node` per node (`libs/langgraph/langgraph/graph/_node.py:98`); no cascade `cheap → expensive → human`. If the handler also fails, the run fails. No circuit breaker.
- **Drift from `langchain.agents`:** `create_react_agent` is deprecated in favor of `langchain.agents.create_agent` (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:274-277`), so any future quality-cost router is likely to land in `langchain` rather than `langgraph`, leaving this source intentionally thin.

## Future Considerations

- Add a first-class `RoutingPolicy`/`ModelTier` alongside `RetryPolicy`/`TimeoutPolicy` (e.g., `RouterPolicy(tiers=[Tier(model="openai:gpt-4o-mini", cost=1), Tier(model="openai:gpt-4o", cost=10)], strategy="confidence_threshold", fallback_on=["validation_error","timeout"])`) with typed `Callable` for scoring; integrate with `StateGraph.set_node_defaults` pattern at `libs/langgraph/langgraph/graph/state.py:272-335`.
- Integrate with `langchain-core` `RunnableWithFallbacks` by documenting and testing `model.with_fallbacks(...)` as the fallback-chain mechanism, or wrapping it as `error_handler` automatically for `create_agent` nodes.
- Emit a structured routing-decision event (model id, tier, criteria scores, cost estimate) via the existing `_AttemptEvent` observer channel (`libs/langgraph/langgraph/pregel/_retry.py:87-126`) or a new `StreamMode="routing"` / `TasksStreamPart` extension, so LangSmith can correlate cost vs. quality.
- Provide a `CachePolicy`-aware router that caches cheap-model answers keyed by `default_cache_key` (`libs/langgraph/langgraph/types.py:524`) and only escalates on cache miss or validation failure, reducing cost.

## Questions / Gaps

- **No evidence of cost accounting:** Search for `cost`, `price`, token pricing under `libs/langgraph` yielded no implementation; any cost-based routing would need to integrate with provider usage metadata (`AIMessage.usage_metadata`) outside LangGraph. Confirmed by `grep cost|price` under `libs/langgraph/langgraph` returning only `stream/_types.py:74` (scoring placeholder) — `No clear evidence found` for cost-aware routing.
- **Is dynamic model callable intended as the routing layer?** Docstring at `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:320-355` describes it as "context-dependent model selection" with an example switching on `runtime.context.model_name`, but does not mention cost, latency, or fallback chains. Design intent appears to be multi-tenant/model-per-user, not quality-cost escalation.
- **Server-side observer:** `_AttemptContext` observer is described as consumed by `langgraph-server` (`libs/langgraph/langgraph/pregel/_retry.py:92-95`), which is out of scope for this source-isolated study. Whether server adds routing traces is unknown — `No evidence found` within `studies/agent-harness-study/sources/langgraph/libs/langgraph`.
- **Tests for fallback on model error:** No test asserts that a failing cheap model falls back to an expensive one. The closest is `test_large_cases.py:6481-6545` which tests graph routing with `FakeMessagesListChatModel`, not tiered fallback.

---

Generated by `20.03-quality-cost-routing` against `langgraph`.
