# Source Analysis: langgraph

## Reflection, ReAsk, and Self-Correction Loops

### Source Info

| Field | Value |
|-------|-------|
| Name | langgraph |
| Path | `studies/agent-harness-study/sources/langgraph` |
| Language / Stack | Python 3.11+ / asyncio / LangGraph (Pregel runtime, StateGraph, Func API) |
| Analyzed | 2026-07-14 |

## Summary

LangGraph does not ship a single named "reflection" or "reask" abstraction; instead, self-correction is decomposed across four cooperating layers:

1. **Declarative retry policies** at the node level (`RetryPolicy` NamedTuple) — `libs/langgraph/langgraph/types.py:416-435` — combined with per-attempt bounds (`max_attempts`, exponential backoff, jitter, configurable `retry_on` predicate).
2. **Per-node error handler routing** (a LangGraph-specific feature): `StateGraph.add_node(..., error_handler=...)` registers a sibling handler node that is automatically scheduled with a frozen `NodeError` context when the primary node fails *after* retries are exhausted (`libs/langgraph/langgraph/graph/state.py:857-870`, `libs/langgraph/langgraph/graph/state.py:1278-1354`, `libs/langgraph/langgraph/pregel/_runner.py:230, 305-306`, `libs/langgraph/langgraph/pregel/_algo.py:1110-1248`).
3. **Tool-level validation feedback** in `ToolNode` — `handle_tool_errors`/`wrap_tool_call` produce a `ToolMessage(status="error", ...)` that is replayed into the model's context, forming the standard ReAct-style "re-ask the model with the failure" loop (`libs/prebuilt/langgraph/prebuilt/tool_node.py:108-122, 339-441, 922-1012`).
4. **Global step / recursion bound** (`recursion_limit`) is the *only* outer guard on the agent re-prompt cycle; `GraphRecursionError` is raised once exhausted (`libs/langgraph/langgraph/_internal/_config.py:32`, `libs/langgraph/langgraph/pregel/_loop.py:1677, 1936`, `libs/langgraph/langgraph/pregel/main.py:3017-3026`).

The system deliberately separates *transient retry* (mechanical, bounded) from *semantic re-ask* (delegated to the graph author via error handlers / tool error feedback), and exposes retry observability through per-attempt `ExecutionInfo` (`libs/langgraph/langgraph/pregel/_retry.py:600-612, 728-731`) plus an `_AttemptEvent` observer contract (`libs/langgraph/langgraph/pregel/_retry.py:87-125`).

## Rating

**8 / 10 — Clear model with explicit interfaces, broad tests, and operational safeguards.** A dedicated `RetryPolicy` plus a per-node `error_handler` plus tool-level `handle_tool_errors` give application code deterministic, testable control over self-correction. The framework distinguishes framework- vs. user-initiated cancellation (`libs/langgraph/langgraph/pregel/_retry.py:315-334`), makes `NodeTimeoutError` non-`OSError` so the default retry covers it (`libs/langgraph/langgraph/errors.py:193-194`), and survives failure across checkpoint resume (`test_graph_error_handler_error_context_survives_checkpoint_resume` at `libs/langgraph/tests/test_retry.py:2161`). Gaps: the re-ask cycle itself has no per-attempt counter (only `recursion_limit`), no built-in critic/quality judge, and the example notebooks for reflection have been retired (see `examples/reflection/reflection.ipynb:9-13` redirecting to consolidated docs).

## Evidence Collected

Every entry includes a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Retry policy declaration | `RetryPolicy` NamedTuple with `initial_interval`, `backoff_factor`, `max_interval`, `max_attempts`, `jitter`, `retry_on` | `libs/langgraph/langgraph/types.py:416-435` |
| Default `retry_on` filter | Class-blacklist + 5xx-HTTP retry, retries everything else | `libs/langgraph/langgraph/_internal/_retry.py:1-29` |
| Sync retry runner | `run_with_retry` with attempts counter + backoff + `ParentCommand` handling | `libs/langgraph/langgraph/pregel/_retry.py:573-683` |
| Async retry runner | `arun_with_retry` with timeout/retry interaction + observer protocol | `libs/langgraph/langgraph/pregel/_retry.py:685-838` |
| Retry predicate (`_should_retry_on`) | Supports exception class / tuple / callable predicate | `libs/langgraph/langgraph/pregel/_retry.py:841-854` |
| Per-attempt execution metadata | `execution_info.node_attempt`, `node_first_attempt_time` exposed to node | `libs/langgraph/langgraph/pregel/_retry.py:600-612, 728-731` |
| Retried task is checkpoint-persistent | `exc.add_note(...)` and resumed via `CONFIG_KEY_RESUMING` | `libs/langgraph/langgraph/pregel/_retry.py:643, 682, 798, 838` |
| Cancellation classification | `_is_user_raised_cancelled` using `asyncio.Task.cancelling()` | `libs/langgraph/langgraph/pregel/_retry.py:315-334` |
| User-raised `CancelledError` routed to error path | Converts to `NodeCancelledError` so the run reports as failure, not silent tear-down (LSD-1507) | `libs/langgraph/langgraph/pregel/_retry.py:635-640, 777-794`, `libs/langgraph/langgraph/errors.py:168-187` |
| `NodeTimeoutError` made retryable | Subclasses `Exception`, NOT `TimeoutError`, so default `RetryPolicy` retries it | `libs/langgraph/langgraph/errors.py:190-241` |
| Node-level error handler registration | Per-node `error_handler` parameter on `add_node` | `libs/langgraph/langgraph/graph/state.py:672, 857-870` |
| Graph-level default error handler | `set_node_defaults(error_handler=...)` via `_DEFAULT_ERROR_HANDLER_NODE = "__default_error_handler__"` | `libs/langgraph/langgraph/graph/state.py:98, 1278-1326` |
| Retry policy propagation rules | Per-node `retry_policy` overrides default; error handlers are still retried | `libs/langgraph/langgraph/graph/state.py:1309-1312` |
| Handler routing in runner | `schedule_error_handler` wired in fast-path and multi-future path | `libs/langgraph/langgraph/pregel/_runner.py:148-165, 222-232, 297-323` |
| Handler routing loop (async) | `aschedule_error_handler` analog | `libs/langgraph/langgraph/pregel/_runner.py:414-423, 503-531` |
| Loop-level error scheduler | Drains pending writes before running the handler for durability | `libs/langgraph/langgraph/pregel/_loop.py:1549-1584, 1803-1838` |
| Handler task preparation | `prepare_node_error_handler_task` builds executable task with `NodeError` context and checkpoint namespace | `libs/langgraph/langgraph/pregel/_algo.py:1110-1248` |
| `NodeError` context object | Frozen dataclass (`node`, `error`) | `libs/langgraph/langgraph/errors.py:148-165` |
| Handler `error: NodeError` injection | Lookup of `CONFIG_KEY_NODE_ERROR` injected into handler kwarg `error` | `libs/langgraph/langgraph/_internal/_runnable.py:381-382, 455-456` |
| Tool invocation error path | `ToolInvocationError` template filters out injected args before reporting to the model | `libs/prebuilt/langgraph/prebuilt/tool_node.py:339-441, 510-563, 957-1012` |
| `handle_tool_errors` config matrix | bool / str / type / tuple / callable; callable arg types inferred via `_infer_handled_types` | `libs/prebuilt/langgraph/prebuilt/tool_node.py:383-507, 984-1012, 1131-1159` |
| Unregistered tool name template | `INVALID_TOOL_NAME_ERROR_TEMPLATE` produces a re-prompt-able error message | `libs/prebuilt/langgraph/prebuilt/tool_node.py:108-110, 1268-1279` |
| `wrap_tool_call` re-ask hook | `ToolCallWrapper` can call `execute(req)` repeatedly; example test retry-middleware | `libs/prebuilt/langgraph/prebuilt/tool_node.py:202-277`, `libs/prebuilt/tests/test_on_tool_call.py:790-838` |
| Deprecated `ValidationNode` | Pydantic schema validation; returns `ToolMessage(..., additional_kwargs={"is_error": True})` for re-prompting | `libs/prebuilt/langgraph/prebuilt/tool_validator.py:43-220` |
| ReAct re-prompt router | `tools_condition` and `should_continue` route back to the model on tool calls | `libs/prebuilt/langgraph/prebuilt/tool_node.py:1582-1640`, `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:830-867` |
| Outer loop bound | `recursion_limit` (`stop = step + recursion_limit + 1`); default 10007 via env | `libs/langgraph/langgraph/_internal/_config.py:32-225`, `libs/langgraph/langgraph/pregel/_loop.py:1677, 1936` |
| `GraphRecursionError` escalation | Raised when `loop.status == "out_of_steps"` | `libs/langgraph/langgraph/pregel/main.py:3017-3026, 3498-3507` |
| Per-attempt observer (server contract) | `_AttemptContext` / `_AttemptEvent` (start/progress/finish) — referenced by langgraph-server | `libs/langgraph/langgraph/pregel/_retry.py:87-125, 343-414` |
| Retry observability (exhaustion telemetry) | Tests assert observer emits `finish` before retry backoff and before final raise | `libs/langgraph/tests/test_retry.py:1813-1867, 1921-1960` |
| `RemainingSteps` re-ask signal | Managed value exposes remaining step budget so agent can self-terminate | `libs/langgraph/langgraph/managed/is_last_step.py:18-23` |
| Cache skips a re-ask | `match_cached_writes` short-circuits the retry path on cache hit | `libs/langgraph/langgraph/pregel/_retry.py:714-718` |
| Submission-channel write gating | Timed-attempt scope discards writes that raced past timeout boundary | `libs/langgraph/langgraph/pregel/_retry.py:493-502, 211-216` |
| Retry tests (broad) | 100+ `test_` entries in `test_retry.py` covering single/jitter/multi-policy/max-exhaustion/cancellation paths | `libs/langgraph/tests/test_retry.py:50-2943` |
| Error-handler regression tests | `test_graph_error_handler_runs_after_retry_exhaustion`, `..._can_route_with_command`, `..._failure_fails_run`, `..._handles_subgraph_internal_failure`, `..._error_context_survives_checkpoint_resume`, `..._does_not_swallow_interrupt_concurrent`, `test_node_error_handlers_route_to_matching_handler` | `libs/langgraph/tests/test_retry.py:2008, 2057, 2099, 2120, 2161, 2197, 2245` |
| Pre-deprecation examples are archived | `examples/reflection/reflection.ipynb` and `examples/reflexion/reflexion.ipynb` redirect to consolidated docs (i.e., reflection as a pattern, not a built-in) | `sources/langgraph/examples/reflection/reflection.ipynb:9-13` |
| Compile-time validation | `validate_graph` ensures channels/triggers/interrupts exist at compile time (separate from output validation) | `libs/langgraph/langgraph/pregel/_validate.py:13-107` |

## Answers to Dimension Questions

1. **When does the agent self-correct?**
   - On any retryable exception during a node invocation, *per the `RetryPolicy.retry_on` predicate* (`libs/langgraph/langgraph/pregel/_retry.py:841-854`) up to `max_attempts` total tries (`libs/langgraph/langgraph/types.py:428`).
   - On any tool error reaching `ToolNode` with `handle_tool_errors` enabled, the error is materialised as a `ToolMessage(status="error", ...)` (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1005-1012`) and re-injected into the agent loop via the standard messages channel.
   - After retry exhaustion, a registered `error_handler` (per-node or graph-default) is scheduled with the `NodeError` context (`libs/langgraph/langgraph/pregel/_algo.py:1236-1238`, `libs/langgraph/langgraph/pregel/_runner.py:230, 305-306`). This is a *second* self-correction mechanism beyond the retry loop.
   - The graph topology itself can express self-correction via `add_conditional_edges` (e.g., a `should_reprompt` edge that routes back to the model when validation marks `is_error=True`, see `libs/prebuilt/langgraph/prebuilt/tool_validator.py:97-106`).

2. **Is correction bounded?**
   - Yes at the per-node level: `RetryPolicy.max_attempts` (`libs/langgraph/langgraph/types.py:428`) plus `recursion_limit` (`libs/langgraph/langgraph/pregel/_loop.py:1677, 1936`, `libs/langgraph/langgraph/_internal/_config.py:32`) at the run level. `NodeTimeoutError` is shaped so the default `RetryPolicy` treats it as retryable but still bounded.
   - Error-handler failure *also* fails the run (`tests/test_retry.py:2099`) — handler chains cannot hide an infinite handler loop.
   - The re-prompt cycle has only the coarse `recursion_limit` outer bound; there is no separate "max times the LLM can be re-asked because the tool call failed" counter exposed by the framework. Application code must compose `RemainingSteps` (`libs/langgraph/langgraph/managed/is_last_step.py:18-23`) to bound that cycle.

3. **What evidence is shown during correction?**
   - The exception message is shown to the *framework*; the user node body is wrapped with `exc.add_note(f"During task with name '{task.name}' and id '{task.id}'")` (`libs/langgraph/langgraph/pregel/_retry.py:643, 798`).
   - The retry log records `Retrying task %s after %.2f seconds (attempt %d) after %s %s` (`libs/langgraph/langgraph/pregel/_retry.py:677-680, 833-836`) at INFO level.
   - The timed-attempt observer emits structured `_AttemptEvent`s (`libs/langgraph/langgraph/pregel/_retry.py:87-414`) — used by `langgraph-server` (line 95 comment).
   - For tool errors, the user gets a `ToolMessage` whose content embeds the formatted error (`libs/prebuilt/langgraph/prebuilt/tool_node.py:430-441`); the tool framework *filters out* injected-argument errors so the LLM only sees feedback about args it controls (`libs/prebuilt/langgraph/prebuilt/tool_node.py:510-563, 960-966`).
   - The error handler receives a frozen `NodeError(node, error)` (`libs/langgraph/langgraph/errors.py:148-165`), which is *serialized into the checkpoint* so a handler run that started before a crash can resume (`libs/langgraph/tests/test_retry.py:2161-2195`).

4. **Does correction improve outputs or hide errors?**
   - Retry layer: improves — it is bounded, recorded, and re-raises the original if `max_attempts` is exhausted (`libs/langgraph/langgraph/pregel/_retry.py:660-661, 816-817`).
   - Error handler: explicit recovery — the framework requires the handler to return a state update (or `Command`) so failure is converted into a defined next step (`libs/langgraph/langgraph/graph/state.py:857-870`, `libs/langgraph/tests/test_retry.py:2020-2052`).
   - Tool-error feedback: *explicit re-prompt* via message history — the failure is *shown* to the model in the next turn, never hidden (`libs/prebuilt/langgraph/prebuilt/tool_node.py:108-122, 984-1012`).
   - One known failure mode is the silent tear-down bug LSD-1507: a user node that raises `asyncio.CancelledError` from its own body would have been treated as silent tear-down; the framework now converts it to `NodeCancelledError` and routes it through the normal error path (`libs/langgraph/langgraph/pregel/_retry.py:777-794`).

5. **Are repeated failures escalated?**
   - Per-attempt, the retry loop escalates by *waiting longer* (`initial_interval * backoff_factor ** (attempts - 1)`, capped by `max_interval`, with optional jitter) (`libs/langgraph/langgraph/pregel/_retry.py:663-674, 820-830`).
   - Per-node, exhaustion escalates to the `error_handler` sibling (`libs/langgraph/langgraph/pregel/_runner.py:222-232, 297-323`).
   - Per-run, exhaustion escalates to `GraphRecursionError` (`libs/langgraph/langgraph/pregel/main.py:3017-3026`).
   - The error *handler* itself participates in the retry cycle via `proc.retry_policy` (`libs/langgraph/langgraph/pregel/_algo.py:1166`), so a flaky handler is also subject to backoff.
   - `NodeTimeoutError` and `NodeCancelledError` flow through the same handling path and surface distinctly in the error trace (`libs/langgraph/langgraph/errors.py:168-241`).

## Architectural Decisions

- **Separation of retry from re-ask.** Retry is mechanical and bounded by `RetryPolicy`; semantic "ask the LLM again" is delegated back to the graph author (typically via `add_conditional_edges` or by surfacing `ToolMessage(status="error",...)`). There is no built-in "weak answer" detector or critic agent.
- **Errors are first-class graph citizens.** A failure in node `foo` that survives retry is converted to a `PUSH` task for `__error_handler__foo` (`libs/langgraph/langgraph/pregel/_algo.py:1141-1153, 1232-1238`). This composes naturally with the existing graph topology — handlers can `Command(goto=...)`, write back to the failed node's channels, or even loop the run forward.
- **`NodeTimeoutError` deliberately does not subclass `OSError.TimeoutError`.** A docstring comment explains this: "Does **not** inherit from the built-in `TimeoutError` (a subclass of `OSError`) so that the default `RetryPolicy` treats it as retryable" (`libs/langgraph/langgraph/errors.py:193-194`). This is a small but visible design choice that prevents the common Python `TimeoutError`-is-not-an-`OSError` mismatch from silently masking retries.
- **User- vs. framework-initiated cancellation are distinguished.** Using `asyncio.Task.cancelling()` (Py3.11+) lets `_is_user_raised_cancelled` (`libs/langgraph/langgraph/pregel/_retry.py:315-334`) flag user nodes that raise `CancelledError` so they are routed through the failure path, not silently swallowed.
- **Writes survive cancellation but not timeout.** The `_TimedAttemptScope` (`libs/langgraph/langgraph/pregel/_retry.py:128-272`) discards writes that race past the timeout boundary but lets them through on cooperative cancel. Idle-timeout watchdog and run-timeout watchdog are explicitly separated (`libs/langgraph/langgraph/pregel/_retry.py:461-506`).
- **Per-attempt observability.** `_AttemptContext` and `_AttemptEvent` (`libs/langgraph/langgraph/pregel/_retry.py:87-125`) form a private contract with `langgraph-server`; this lets the runtime track attempt start/progress/finish without polluting public types.
- **The re-ask evidence goes through messages, not a side channel.** Tool failure text becomes a `ToolMessage(content=...)`, not a separate "reflection prompt". This means the LLM sees the failure in the same schema it would see any tool result — there is no shadow prompt-builder pathway. The deprecated `ValidationNode` (`libs/prebuilt/langgraph/prebuilt/tool_validator.py:43-220`) was the explicit "validator + re-ask" sentinel pattern; it is replaced by `handle_tool_errors` in current usage.

## Notable Patterns

- **Error-handler duality.** Per-node `error_handler` and graph-default `error_handler` (`libs/langgraph/langgraph/graph/state.py:857-870, 1278-1326`) — the latter catches nodes that didn't declare their own, so a global safety net is a one-liner: `.compile(set_node_defaults(error_handler=fallback_fn))`.
- **Failure context survives checkpoint.** When a graph crashes mid-handler-run, the handler can be resumed because `NodeError` is serialized as part of the pending writes — see `test_graph_error_handler_error_context_survives_checkpoint_resume` (`libs/langgraph/tests/test_retry.py:2161-2195`).
- **Errors can interrupt other tasks cleanly.** When a node in a fan-out fails with a handler, sibling tasks stop; otherwise they finish normally. The pattern `SKIP_RERAISE_SET` (`libs/langgraph/langgraph/pregel/_runner.py:70-72, 304, 502`) and `_handled_exception_ids` (`libs/langgraph/langgraph/pregel/_runner.py:166-169`) keep "handler-routed" exceptions out of the panic path.
- **Tool-call interceptors as retry middleware.** `wrap_tool_call(req, execute)` is *explicitly designed to let `execute` be invoked multiple times* (`libs/prebuilt/langgraph/prebuilt/tool_node.py:202-277`); the test `test_retry_middleware_with_exception` (`libs/prebuilt/tests/test_on_tool_call.py:790-838`) demonstrates the pattern.
- **Compose retry with error handler.** The test `test_graph_error_handler_runs_after_retry_exhaustion` (`libs/langgraph/tests/test_retry.py:2008-2055`) shows the canonical composition: retry first, then escalate to a handler.
- **`Commander-style recovery.** Handlers may return a `Command(update={...}, goto="...")` to both patch state and route the run (`libs/langgraph/langgraph/graph/state.py:857`, `libs/langgraph/tests/test_retry.py:2057-2097, 2245-2284`).

## Tradeoffs

- **Outer bound on re-ask is coarse.** The only thing stopping a model from being re-asked indefinitely is the run-wide `recursion_limit` (`libs/langgraph/langgraph/_internal/_config.py:32`). There is no per-re-ask budget or "give up after N identical errors" signal built in. Application code must compose `RemainingSteps` (`libs/langgraph/langgraph/managed/is_last_step.py:18-23`) or track re-asks itself.
- **No critic / quality judge.** A weak answer that doesn't error will not trigger any built-in re-ask. The reflection/reflexion examples have been retired (`sources/langgraph/examples/reflection/reflection.ipynb:9-13`); authors must build the critic loop themselves.
- **Errors are routed, not raised, on fan-out.** When a node has an `error_handler` and a sibling node in the same tick fails, the run continues with the handler's update rather than failing — this can mask up-stack complaints if the handler swallows too liberally (mitigation: handler *itself* has retry policy and is not allowed to ignore its own failures: `tests/test_retry.py:2099`).
- **Tool-error messages can be lossy when filter is large.** Pydantic errors are filtered to only LLM-controlled args (`libs/prebuilt/langgraph/prebuilt/tool_node.py:510-563`), which is good for the LLM but means a developer digging through `state.messages` may not see the full validation error.
- **`ValidationNode` is deprecated.** `libs/prebuilt/langgraph/prebuilt/tool_validator.py:43-46` deprecation message points to `langchain.agents.create_agent` and custom tool error handling — i.e., the framework retired the dedicated validator in favor of `handle_tool_errors` + `wrap_tool_call`. Users who relied on a dedicated validation node must migrate.

## Failure Modes / Edge Cases

- **Infinite loop without error.** A perfectly-modeled but always-repeated reflection cycle (no exception, just no progress) is only caught by `recursion_limit`. No semantic-progress check exists.
- **Handler hide-and-seek.** A handler that *succeeds* but does not emit any state change can make the run appear to make forward progress while the topology is unchanged. `set_node_defaults(error_handler=...)` plus no routing is a footgun.
- **Cancellation triage on Py<3.11.** `_is_user_raised_cancelled` falls back to `False` when `asyncio.Task.cancelling()` is unavailable (`libs/langgraph/langgraph/pregel/_retry.py:329-334`); on Py3.10 user-raised `CancelledError` keeps the legacy LSD-1507 behaviour (silent tear-down). Documented in `test_retry.py:79-83`.
- **Pre-timeout writes leak into cache.** Although `_TimedAttemptScope._guard_send` blocks timed-out writes (`libs/langgraph/langgraph/pregel/_retry.py:228-234`), the *cache* does not always re-validate cached payloads against a fresh retry policy; if a cache key is reused, the cached writes are returned without going through retry (`libs/langgraph/langgraph/pregel/_runner.py:1526-1539, 1780-1793`). Test coverage: `test_arun_with_retry_timeout_discards_stale_executor_writes` (`libs/langgraph/tests/test_retry.py:1090-1142`).
- **Recursion limit off-by-one.** `stop = step + recursion_limit + 1` (`libs/langgraph/langgraph/pregel/_loop.py:1677, 1936`) — easy to misread as "+limit", so authors that need "at most N cycles" must verify experimentally (test: `libs/langgraph/tests/test_pregel.py:574-575` raises with `recursion_limit=1`).
- **`status="error"` is a string field.** Tool-error handlers set `status="error"`, but only `ToolNode` and downstream `tools_condition` know about it (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1005-1012, 1268-1279`). A model that ignores message `status` may re-request the same failing tool call indefinitely.

## Future Considerations

- **Per-re-ask budget.** A natural extension is a `ReaskPolicy(max_reasks=N, dedupe=...)` analogous to `RetryPolicy`, but stopping at LLM re-prompt cycles rather than node retries.
- **First-class critic node.** LangGraph exposes the building blocks (`StateNode`, `PUSH` triggers, `NodeError`) needed for a built-in reflection node. The deletion of `examples/reflection` and `examples/reflexion` notebooks (`sources/langgraph/examples/reflection/reflection.ipynb:9-13`) suggests this is left to `langchain.agents` rather than added here.
- **Stronger handler contracts.** The handler currently executes regardless of whether the failure looked retryable; a `handler_for: tuple[type[BaseException], ...]` selector would mirror `retry_on` and shrink the handler's blast radius.
- **State-aware error budgets.** The chain `RetryPolicy(max_attempts, backoff) → error_handler → recursion_limit` is non-trivial. A "self-correction budget" abstraction (per-thread: how many retry+handler executions before fail-run) would collapse all three into one configurable knob.
- **Telemetry for handler routes.** The `_AttemptEvent` observer (`libs/langgraph/langgraph/pregel/_retry.py:87-125, 343-414`) covers node timeouts but not handler lifecycle events. Adding an analogous `_HandlerEvent` would make handler-routed self-correction auditable.

## Questions / Gaps

- Is the `_AttemptEvent` observer contract (`libs/langgraph/langgraph/pregel/_retry.py:87-125`) consumed somewhere in OSS, or only in `langgraph-server` as the comment at line 95 implies? **No clear evidence found in the OSS tree** — grep for `_AttemptEvent` outside `_retry.py` and the tests is empty.
- How does `wrap_tool_call` handle the case where the user wrapper re-throws but `handle_tool_errors=False`? The path exists at `libs/prebuilt/langgraph/prebuilt/tool_node.py:1054-1067` (re-raises) but I could not find an explicit test that demonstrates this interaction. **No clear evidence found** beyond the implementation.
- Does the SDK expose any "give up after N identical errors" loop counter on top of `recursion_limit`? **No evidence found.** The closest managed value is `RemainingSteps` (`libs/langgraph/langgraph/managed/is_last_step.py:18-23`), which only mirrors the recursion budget.
- What happens if an `error_handler` itself depends on a node that has no handler? Handlers can theoretically recurse; the topology cycle is bounded only by `recursion_limit`. **No clear evidence found** in the test suite for a "handler chain that loops" interaction — `test_set_node_defaults_chaining` (`libs/langgraph/tests/test_retry.py:2620`) only chains retry+handler defaults on flat graphs.

---

Generated by `reports/source/03.05-reflection-reask-self-correction/langgraph.md` (dimension `03.05-reflection-reask-and-self-correction-loops`) against `langgraph`.
