# Source Analysis: langgraph

## 07.08: Tool Failure Escalation

### Source Info

| Field | Value |
|-------|-------|
| Name | langgraph |
| Path | `studies/agent-harness-study/sources/langgraph` |
| Language / Stack | Python (core `libs/langgraph`, prebuilt agents `libs/prebuilt`), plus JS SDK `libs/sdk-js`; version 1.2.11 (`libs/langgraph/pyproject.toml`) |
| Analyzed | 2026-08-25 |

## Summary

LangGraph treats tool failure escalation as a layered, multi-audience problem with explicit mechanisms for each audience. At the innermost layer, the prebuilt `ToolNode` converts failures into **model-facing error envelopes** — `ToolMessage` objects carrying `status="error"` and actionable templates ("Please fix your mistakes", `libs/prebuilt/langgraph/prebuilt/tool_node.py:108-121`) — so a failed tool call becomes a recovery input to the next model turn rather than a dead end. The error-handling strategy is highly configurable (`handle_tool_errors` accepting bool/str/callable/exception-type/tuple, `tool_node.py:749-753`), with type-inferred handlers (`_infer_handled_types`, `tool_node.py:444-507`) and validation errors filtered down to only the arguments the model controls (`_filter_validation_errors`, `tool_node.py:510-563`). A `wrap_tool_call` interceptor layer enables programmatic retry/caching wrappers around tool execution (`tool_node.py:202-277`).

At the executor layer, node-level (not just tool-level) failures flow through a retry engine (`run_with_retry`/`arun_with_retry`, `libs/langgraph/langgraph/pregel/_retry.py:573-838`) governed by `RetryPolicy` with exponential backoff, jitter, and cause classification via `retry_on` (`libs/langgraph/langgraph/types.py:418-437`). Retry exhaustion escalates to either the caller (exception re-raised, siblings cancelled) or to dedicated **error-handler nodes** that receive a structured `NodeError(node, error)` context and can recover the run by returning state updates or `Command` routing (`libs/langgraph/langgraph/errors.py:148-165`, `libs/langgraph/langgraph/pregel/_algo.py:1110-1240`). Failures are persisted as `__error__` channel writes to the checkpointer so they survive resume (`libs/langgraph/langgraph/pregel/_runner.py:594-603`, `libs/langgraph/langgraph/_internal/_constants.py:13`).

Human escalation is an explicit, opt-in primitive: `interrupt()` raises a resumable `GraphInterrupt` whose payload is surfaced to the client and checkpointed (`libs/langgraph/langgraph/types.py:851-960`); there is no automatic promotion of repeated tool failures into human approval. Operator visibility comes from standard logging of retries (`pregel/_retry.py:677-680`), LangChain callback hooks (`on_tool_error`, `libs/langgraph/langgraph/pregel/_tools.py:260-265`), exception annotations ("During task with name ...", `pregel/_retry.py:642-643`), and a typed timed-attempt observer event stream carrying `status`/`error_type`/`error_message` consumed by LangGraph server (`pregel/_retry.py:87-126`). What is missing is in-core failure aggregation/grouping by root cause and first-party metrics/alerting sinks; those are delegated to callbacks and the platform.

**Verdict:** a failed tool in LangGraph is predominantly a *recovery path* — the default configuration feeds a structured, actionable error back to the model; dead ends occur only when developers opt out (`handle_tool_errors=False`) or when unhandled exception types escape the tool boundary.

## Rating

**8 / 10** — Clear model with tests, explicit interfaces, and operational safeguards.

Rationale:
- Explicit, documented taxonomy of failure types (invalid name vs. invalid arguments vs. execution failure) with distinct message templates (`tool_node.py:108-121`).
- Every audience has a defined surface: model (`ToolMessage status="error"`), caller (re-raised exceptions, `StateSnapshot.tasks[].error`), operator (logs + callback + observer events), human supervisor (`interrupt()`).
- Retry exhaustion has a first-class escalation target: graph-level/per-node error-handler nodes receiving `NodeError`, proven by dedicated tests including resume-survival (`libs/langgraph/tests/test_retry.py:2009-2397`).
- Cause-based classification exists at the retry layer (`retry_on`, `_should_retry_on` at `pregel/_retry.py:841-854`) and framework-error level (`ErrorCode` enum, `errors.py:34-39`).
- Falls short of 9–10 because: no built-in aggregation of failures grouped by cause beyond per-exception matching; no native metrics/alerting integration (relies on the external LangChain callback system and the private server observer); model-facing content embeds raw `repr(e)` which can leak internals; sync nodes cannot use timeouts (`sync_timeout_unsupported`, `pregel/_retry.py:580-583`).

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Error envelopes (model-facing) | Failed/handled tool calls become `ToolMessage(content=..., status="error")` returned into state | `libs/prebuilt/langgraph/prebuilt/tool_node.py:1005-1012` |
| Actionable message templates | `TOOL_CALL_ERROR_TEMPLATE` ("Please fix your mistakes"), `TOOL_EXECUTION_ERROR_TEMPLATE`, `TOOL_INVOCATION_ERROR_TEMPLATE` ("Please fix the error and try again.") | `libs/prebuilt/langgraph/prebuilt/tool_node.py:111-121` |
| Invalid-tool-name envelope | `INVALID_TOOL_NAME_ERROR_TEMPLATE` lists available tools; `_validate_tool_call` returns error ToolMessage instead of raising | `libs/prebuilt/langgraph/prebuilt/tool_node.py:108-110,1268-1279` |
| Failure type classification | `ToolInvocationError` (validation) distinguished from execution errors; wraps Pydantic `ValidationError` with tool name/kwargs | `libs/prebuilt/langgraph/prebuilt/tool_node.py:339-380,959-966` |
| LLM-focused validation filtering | `_filter_validation_errors` drops injected-arg errors so the model only sees what it can fix | `libs/prebuilt/langgraph/prebuilt/tool_node.py:510-563` |
| Default handler policy | `_default_handle_tool_errors`: invocation errors → message; other exceptions re-raised | `libs/prebuilt/langgraph/prebuilt/tool_node.py:383-391` |
| Configurable handling strategies | `handle_tool_errors` = True/str/callable/type/tuple/False; strategy dispatch in `_handle_tool_error` | `libs/prebuilt/langgraph/prebuilt/tool_node.py:674-694,394-441` |
| Type-inferred handled exceptions | `_infer_handled_types` reads handler signature annotations (incl. unions) | `libs/prebuilt/langgraph/prebuilt/tool_node.py:444-507` |
| Interceptor-based retries | `wrap_tool_call`/`awrap_tool_call`: `execute` callable may be invoked multiple times; wrapper exceptions also converted to error ToolMessages | `libs/prebuilt/langgraph/prebuilt/tool_node.py:202-283,1044-1067,1204-1222` |
| Interrupt passthrough | `GraphBubbleUp` always re-raised from tool execution — interrupts are never swallowed as tool errors | `libs/prebuilt/langgraph/prebuilt/tool_node.py:973-983` |
| Node retry engine | `run_with_retry` / `arun_with_retry`: backoff `min(max_interval, interval*factor^(n-1))`, jitter, per-policy match | `libs/langgraph/langgraph/pregel/_retry.py:573-682,685-838` |
| Retry cause classification | `RetryPolicy.retry_on` (type/sequence/callable) matched by `_should_retry_on`; `default_retry_on` retries ConnectionError & 5xx, refuses ValueError/TypeError/etc. | `libs/langgraph/langgraph/types.py:430-437`, `libs/langgraph/langgraph/pregel/_retry.py:841-854`, `libs/langgraph/langgraph/_internal/_retry.py:1-29` |
| Operator log on retry | `logger.info("Retrying task {name} after {sleep:.2f} seconds (attempt {n}) after {ExcClass} {exc}", exc_info=exc)` | `libs/langgraph/langgraph/pregel/_retry.py:676-680,832-836` |
| Exception context annotation | `exc.add_note("During task with name '{task.name}' and id '{task.id}'")` (py≥3.11) | `libs/langgraph/langgraph/pregel/_retry.py:51,641-643,796-798` |
| Retry exhaustion | When `attempts >= max_attempts`, exception propagates (no swallow) | `libs/langgraph/langgraph/pregel/_retry.py:657-661,812-817` |
| Timeout-as-failure conversion | Watchdogs convert `asyncio.TimeoutError` into `NodeTimeoutError` (deliberately not an `OSError` subclass so `RetryPolicy` treats it retryable), clearing partial writes | `libs/langgraph/langgraph/errors.py:190-206`, `libs/langgraph/langgraph/pregel/_retry.py:482-502` |
| Cancellation disambiguation | User-raised `CancelledError` converted to `NodeCancelledError` so run reports `error`, not silent success | `libs/langgraph/langgraph/errors.py:168-187`, `libs/langgraph/langgraph/pregel/_retry.py:777-794` |
| Error persistence | Failed task writes `(ERROR, exception)` (`__error__`) to checkpointer; readable back on resume via `_coerce_pending_error` | `libs/langgraph/langgraph/pregel/_runner.py:594-599`, `libs/langgraph/langgraph/_internal/_constants.py:13`, `libs/langgraph/langgraph/pregel/_algo.py:761-781` |
| Escalation-to-handler routing | On failure, runner routes to mapped error-handler node (`schedule_error_handler`) instead of panicking; `ERROR_SOURCE_NODE` marker persisted | `libs/langgraph/langgraph/pregel/_runner.py:205-310,596-603`, `libs/langgraph/libs/langgraph/langgraph/pregel/_algo.py:296-310` |
| Handler error context | Handler task config carries `CONFIG_KEY_NODE_ERROR=NodeError(node, error)` injected as handler parameter | `libs/langgraph/langgraph/pregel/_algo.py:1230-1240`, `libs/langgraph/langgraph/errors.py:148-165` |
| Handler registration API | `add_node(..., error_handler=...)`; auto-created `__error_handler__{node}` nodes; graph-default handler for all plain nodes | `libs/langgraph/langgraph/graph/state.py:699,867-915,1292-1321` |
| Caller-facing failure | `_panic_or_proceed` cancels inflight tasks and re-raises first non-handled exception | `libs/langgraph/langgraph/pregel/_runner.py:650-696` |
| Sibling cancellation on failure | `_should_stop_others` treats any non-`GraphBubbleUp` exception as fatal → cancel peers | `libs/langgraph/langgraph/pregel/_runner.py:620-637` |
| Caller introspection | `StateSnapshot.tasks[].error` / `PregelTask.error` expose persisted errors without raising | `libs/langgraph/langgraph/types.py:637-646,683-701` |
| Framework error codes | `ErrorCode` enum + `create_error_message` appending troubleshooting doc URLs (e.g., GRAPH_RECURSION_LIMIT raised at recursion limit) | `libs/langgraph/langgraph/errors.py:34-47,67-87`, `libs/langgraph/langgraph/pregel/main.py:3005-3011` |
| Human escalation primitive | `interrupt(value)` raises resumable `GraphInterrupt`, surfaces value to client, requires checkpointer; `Interrupt(value, id)` dataclass | `libs/langgraph/langgraph/types.py:575-618,851-960` |
| Interrupt persistence | Commit path saves `(INTERRUPT, ...)` writes to checkpoint on `GraphInterrupt` | `libs/langgraph/langgraph/pregel/_runner.py:582-590` |
| Operator alerting hook | Callback handlers receive `on_tool_error` (also drives idle-timeout progress) | `libs/langgraph/langgraph/pregel/_tools.py:260-265`, `libs/langgraph/langgraph/pregel/_retry.py:294-312` |
| Server observability events | `_AttemptEvent(status="success"\|"error", error_type, error_message)` dispatched to `CONFIG_KEY_TIMED_ATTEMPT_OBSERVER` (consumed by langgraph-server) | `libs/langgraph/langgraph/pregel/_retry.py:87-126,370-404` |
| HITL config moved out | `HumanInterruptConfig`/`ActionRequest`/`HumanInterrupt` deprecated in prebuilt, relocated to `langchain.agents.interrupt` | `libs/prebuilt/langgraph/prebuilt/interrupt.py:8-9,36-38,71-73` |
| Tests: envelope behavior | Default handler produces error-status ToolMessage with kwargs in content; unhandled ValueError re-raised; callable/tuple strategies; `handle_tool_errors=False` propagation | `libs/prebuilt/tests/test_tool_node.py:272-340,475+,539+` |
| Tests: retry & exhaustion | Max-attempts exhaustion re-raises; multiple policies; timeout retries under `retry_on=TimeoutError` | `libs/langgraph/tests/test_retry.py:175-247,447-480,786-808` |
| Tests: handler escalation | Handler runs after retry exhaustion; can route via `Command`; handler failure fails run; error context survives checkpoint resume; nodes without handler still fail run | `libs/langgraph/tests/test_retry.py:2009-2397` |
| Tests: interceptor errors | Wrapper-thrown exceptions converted to `status="error"` ToolMessage when `handle_tool_errors=True` | `libs/prebuilt/tests/test_on_tool_call.py:246-261,711-726` |

## Answers to Dimension Questions

**1. Who sees tool failure?**
All four audiences, each through a defined channel:
- **Model**: error `ToolMessage` written into `messages` state, so it is the direct input of the next model invocation (`tool_node.py:1005-1012`, combined into output at `tool_node.py:862-887`).
- **User/caller (developer client)**: if the failure isn't converted to a message, the exception propagates through the pregel runner, cancels sibling tasks, and re-raises to the `invoke`/`stream` caller (`_runner.py:650-696`, `_runner.py:620-637`). Persisted errors are also introspectable without raising via `StateSnapshot.tasks[].error` (`types.py:683-701`).
- **Operator**: Python `logging` on every retry with class+message+traceback (`pregel/_retry.py:676-680`); exception notes identifying task name/id (`pregel/_retry.py:642-643`); LangChain callbacks (`on_chain_error` at `pregel/main.py:3020`, `on_tool_error` at `pregel/_tools.py:260-265`); and structured attempt lifecycle events with error metadata for the server (`pregel/_retry.py:370-390`).
- **Human supervisor**: only via explicit `interrupt()` calls placed by the developer; interrupt payloads reach the executing client as `__interrupt__` stream output (`types.py:851-905`).

**2. Is the error actionable?**
Yes, unusually so. Templates explicitly instruct the model what to do next: "Please fix your mistakes" (`tool_node.py:111`) and "Please fix the error and try again." (`tool_node.py:112-121`). Unknown-tool errors enumerate valid alternatives (`tool_node.py:108-110,1272-1275`). Validation failures include field locations and messages but are filtered to exclude system-injected arguments the model cannot control, and injected values are scrubbed from `input_value` details (`tool_node.py:510-563`). Execution errors embed tool name, kwargs, and the underlying error (`TOOL_EXECUTION_ERROR_TEMPLATE`).

**3. Can the model recover?**
Yes — this is the designed happy path. An error ToolMessage enters state and the agent loop returns to the model, which can correct arguments or choose another tool (default `create_agent` wiring constructs `ToolNode` with the default handler at `chat_agent_executor.py:558-561`). Programmatic recovery layers exist too: interceptors can retry `execute()` transparently (`tool_node.py:239-265`), `RetryPolicy` re-runs whole node attempts (`types.py:418-437`), and error-handler nodes can repair state or route around the failed node using `Command(update=..., goto=...)` (`errors.py:156-158`, test `test_graph_error_handler_can_route_with_command` at `tests/test_retry.py:2058`). Dead-end conditions are deliberate opt-outs: `handle_tool_errors=False` or an exception outside the configured handled types re-raises (`tool_node.py:1001-1003`).

**4. When is failure escalated to a human?**
Never automatically. Human-in-the-loop escalation requires the developer to call `interrupt()` inside a node/tool (`types.py:851-905`), typically wrapped in approval middleware; the framework's role is to make that escalation durable — interrupts are saved to the checkpointer (`_runner.py:582-590`), surfaced with stable IDs (`Interrupt.id`, `types.py:600-601`), and resumed via `Command(resume=...)`. Notably, the richer HITL request schema (`HumanInterrupt` with allow_accept/edit/respond/ignore flags) was moved out of this repo into `langchain.agents.interrupt` (`prebuilt/interrupt.py:8-9,71-73`), so within-langgraph escalation payloads are free-form values.

**5. Are failures grouped by cause?**
Partially.
- Retry classification groups causes via `retry_on` types/callables and first-match-wins across multiple policies (`pregel/_retry.py:647-655,802-810,841-854`).
- Framework-level errors carry stable machine codes and doc links via `ErrorCode`/`create_error_message` (`errors.py:34-47`).
- Observer events carry a string `error_type` per attempt (`pregel/_retry.py:124`), suitable downstream grouping.
- However, there is no in-core aggregation/deduplication/clustering of tool failures by root cause (e.g., counting repeated identical failures across tasks), no rate-based alerting, and `ErrorCode` covers only 5 graph-level conditions — none of them tool-execution-specific (`errors.py:34-39`). Grouping is left to operators via logs/callbacks/platform.

## Architectural Decisions

1. **Errors as state, not just signals.** Handled tool failures become first-class `ToolMessage`s in conversation state (`tool_node.py:1007-1012`) rather than side-channel diagnostics. This makes the model itself the primary recovery actor and keeps recovery checkpoint-durable, since messages persist with the thread.

2. **Policy object split between two layers.** Tool-call granularity uses the `handle_tool_errors` union type (`bool | str | Callable | type | tuple`, `tool_node.py:749-753`), while node granularity uses declarative `RetryPolicy`/`TimeoutPolicy` dataclasses attached at `add_node` time (`types.py:418-489`, `graph/state.py:699`). The two compose: retry exhaustion precedes error-handler routing (`tests/test_retry.py:2009-2043` asserts handler runs after exactly `max_attempts` tries).

3. **Explicit escalation contract via `NodeError`.** Rather than catching exceptions invisibly, LangGraph passes a frozen `NodeError(node, error)` value object into user-written handler functions through dependency injection keyed on the parameter annotation (`errors.py:148-165`, config injection at `_algo.py:1230-1240`, argument binding at `_internal/_runnable.py:403,477`). Handlers return updates or `Command`s, integrating failure recovery into normal graph control flow.

4. **Durable failure records.** Errors are written to the checkpointer as `(__error__, exception)` pending writes and can be coerced back to exceptions on resume (`_runner.py:599`, `_algo.py:761-771`), enabling post-mortem inspection (`get_state().tasks[].error`) and idempotent re-routing of already-failed tasks after restart (`_loop.py:751-800`, incl. `ERROR_SOURCE_NODE` marker to avoid double-handling).

5. **Interrupts outrank errors.** `GraphBubbleUp` (interrupts, parent commands, drain) is never converted into an error envelope and always bypasses both tool error handlers and retry logic (`tool_node.py:973-983`, `pregel/_retry.py:618-634,753-776`), preventing supervision pauses from being masked as tool failures.

6. **Timeouts modeled as retryable failures.** `NodeTimeoutError` deliberately does not subclass built-in `TimeoutError` (an `OSError`) so `default_retry_on`'s blanket "retry unknown exceptions" rule applies to it (`errors.py:190-199`, `_internal/_retry.py:11-29`) — a subtle choice trading surprise-minimization for retryability.

## Notable Patterns

- **Template-driven model coaching**: constant message templates centralize the tone/content of model-facing errors (`tool_node.py:108-121`), analogous to `ErrorCode` doc-link templates for humans (`errors.py:42-47`).
- **Signature-driven behavior**: exception types a handler catches are inferred from its type annotations (`_infer_handled_types`, `tool_node.py:444-507`), mirroring how `NodeError` injection works — configuration lives in ordinary function signatures.
- **Interceptor onion for cross-cutting concerns**: `wrap_tool_call` receives an `execute` callable that "CAN BE CALLED MULTIPLE TIMES," making retry/cache/fallback middleware trivial to implement without touching ToolNode internals (`tool_node.py:202-283`); wrapper failures funnel through the same error-envelope machinery (`tool_node.py:1054-1067`).
- **Watchdog + scoped-config pattern**: async timeouts wrap the node's send/stream/call/runtime hooks so any observable work counts as progress and post-timeout writes are dropped atomically (`_TimedAttemptScope`, `pregel/_retry.py:128-271`).
- **Graceful-shutdown as a distinct outcome**: `GraphDrained` (SIGTERM-cooperative stop) is separated from errors and bubbles up like interrupts (`errors.py:54-64`, raised via loop status at `pregel/main.py:3012-3015`).

## Tradeoffs

- **Recovery-by-default vs. silent failure risk.** With `handle_tool_errors=True` (a common user choice), *every* exception becomes a benign-looking ToolMessage; stack traces vanish from the caller unless operators watch callbacks/logs. The default (`_default_handle_tool_errors`) narrows this by re-raising non-invocation errors (`tool_node.py:383-391`), but users overriding with `True` lose fail-fast semantics.
- **Information leakage vs. actionability.** Embedding `repr(e)` and full tool kwargs in model-facing content (`tool_node.py:430`, `TOOL_EXECUTION_ERROR_TEMPLATE`) maximizes model actionability but can leak filesystem paths, URLs, or internal details into prompts and transcripts; there is no redaction hook.
- **Retry-on defaults favor availability over correctness.** `default_retry_on` retries any unrecognized exception (`_internal/_retry.py:29 return True`) while refusing common deterministic bugs (ValueError/TypeError...). Unknown custom exception types will be retried even when non-transient.
- **Platform-coupled observability.** The richest operational signal (timed-attempt lifecycle events with error metadata) flows through a private config key explicitly reserved for langgraph-server (`pregel/_retry.py:92-96`); OSS users must reconstruct observability from logs and callbacks.
- **Python-version-dependent fidelity.** Exception notes require ≥3.11, and user-vs-framework cancellation discrimination relies on `asyncio.Task.cancelling()` (≥3.11) (`pregel/_retry.py:51-56,315-334`); on older runtimes cancellation failures degrade to silent tear-down.

## Failure Modes / Edge Cases

- **Unhandled tool error aborts the whole superstep**: one failing parallel tool call triggers sibling cancellation via `_should_stop_others` (`_runner.py:620-637`) and panic-rethrow (`_runner.py:650-696`); successful siblings' writes are discarded for that step (their completed writes were persisted per-task, but the step fails).
- **Wrapper exceptions masked as tool results**: exceptions thrown inside `wrap_tool_call` are converted to error ToolMessages whenever `handle_tool_errors` is truthy (`tool_node.py:1056-1067`), which can disguise middleware bugs (e.g., a typo in the interceptor) as ordinary tool errors — tested intentionally at `test_on_tool_call.py:711-726`.
- **Handler-node failure fails the run**: if the error handler itself raises, the run fails rather than looping (`test_graph_error_handler_failure_fails_run`, `test_retry.py:2100`) — no nested-handler chain.
- **Resume replays handler, not failure**: after checkpoint resume, failed tasks routed to handlers are not re-executed; the persisted `__error__` write is replayed into a fresh handler task (`_loop.py:751-800`), verified by `test_graph_error_handler_error_context_survives_checkpoint_resume` (`test_retry.py:2162`).
- **Sync nodes cannot time out**: passing `timeout=` on a sync node raises immediately (`sync_timeout_unsupported`, `pregel/_retry.py:579-583`) — a coverage gap between sync/async policies.
- **Non-exception error values**: persisted `__error__` values that aren't exception instances are coerced with `Exception(str(value))` (`_algo.py:761-765`), losing type fidelity across serialization boundaries.
- **Recursion-limit as terminal failure**: runaway agent loops end in `GraphRecursionError` with remediation instructions embedded (`errors.py:67-87`, `main.py:3005-3011`) — escalated to the caller, not to the model.

## Future Considerations

- Add an opt-in **failure sink interface** (metrics/tracing-friendly) so OSS deployments get parity with the server's `_AttemptEvent` feed without reaching into private config keys (`pregel/_retry.py:92-96`).
- Provide **content-redaction hooks** for model-facing error envelopes to complement `repr(e)` templating (`tool_node.py:430`).
- Consider **cause-grouped failure summaries** at run end (counts by exception type per node), building on existing `error_type` fields, to answer "are failures grouped by cause?" natively.
- Extend **timeout support to sync nodes** or auto-wrap sync bodies in executors to close the sync/async policy gap (`pregel/_retry.py:579-583`).
- Re-home or re-document the HITL schema now that `HumanInterrupt` moved to `langchain.agents` (`prebuilt/interrupt.py:8-9`) so escalation-payload contracts have an owner in-repo.

## Questions / Gaps

- **No evidence found** for any built-in alerting/metrics emission (searched `otel|opentelemetry|prometheus|metrics` across `libs/langgraph/langgraph` — zero production hits). Alerting is delegated entirely to LangChain callbacks and LangGraph Platform.
- **No evidence found** for automatic escalation of *repeated* tool failures to humans (e.g., after N failed retries, trigger an interrupt); searched `interrupt` usages within `tool_node.py` and retry paths — interrupt is only ever developer-invoked.
- The **JS/TS parity story** was not assessed: `libs/sdk-js` is a REST API client, and no JS ToolNode implementation exists in this monorepo, so all findings describe the Python runtime.
- Exact behavior of `stream_mode="values"` surfacing mid-run errors to clients was verified indirectly (exceptions re-raise through `astream` at `pregel/main.py:3016-3021`); streaming-specific error chunk shapes (if any) were not separately audited.

---

Generated by `dimensions/07.08-tool-failure-escalation` against `langgraph`.
