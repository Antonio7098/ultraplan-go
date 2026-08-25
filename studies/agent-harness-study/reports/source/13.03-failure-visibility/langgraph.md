# Source Analysis: langgraph

## Dimension 13.03: Failure Visibility

### Source Info

| Field | Value |
|-------|-------|
| Name | langgraph |
| Path | `studies/agent-harness-study/sources/langgraph` |
| Language / Stack | Python (core `libs/langgraph`, `libs/prebuilt`, `libs/checkpoint`, `libs/sdk-py`); JS/TS SDK present as README stub only in this snapshot |
| Analyzed | 2026-08-23 |

## Summary

LangGraph implements failure visibility as a **multi-surface, per-stakeholder design** rather than a single logging layer. The same underlying failure event (a task exception) fans out to distinct consumers:

1. **Model**: tool failures are converted into error `ToolMessage`s (`status="error"`) with model-directed remediation text ("Please fix your mistakes"), with the level of detail configurable per node via `handle_tool_errors` (`libs/prebuilt/langgraph/prebuilt/tool_node.py:749-753`).
2. **User/caller**: exceptions propagate out of `invoke`/`stream` with actionable messages and troubleshooting URLs (`libs/langgraph/langgraph/errors.py:42-47`), framework frames are stripped from tracebacks (`libs/langgraph/langgraph/pregel/_runner.py:58-68`), and interrupts surface as structured stream payloads rather than exceptions.
3. **Developer**: failures are persisted to checkpoints on an internal `__error__` channel (`libs/langgraph/langgraph/pregel/_runner.py:595-603`, `libs/langgraph/langgraph/_internal/_constants.py:13`), retrievable post-hoc via `get_state`/`get_state_history` task errors; LangChain callback handlers receive `on_chain_error`; retry activity is logged at INFO with full stack traces.
4. **Operator**: a new lifecycle stream channel reports subgraph terminal status including `"failed"` plus error strings (`libs/langgraph/langgraph/stream/transformers.py:343-370`), timed-attempt observers receive structured `{status, error_type, error_message}` events (`libs/langgraph/langgraph/pregel/_retry.py:110-125`), and the SDK maps API errors to a typed hierarchy carrying `x-request-id` (`libs/sdk-py/langgraph_sdk/errors.py:82-93`).

The design is explicit and well-tested, but observability is unevenly distributed: structured error *codes* exist for only five graph-level conditions, log-based visibility in the core is sparse (the entire `prebuilt` library emits no log records), and message-mode streams silently drop LLM errors.

## Rating

**8 / 10** — A clear, deliberate model with distinct per-stakeholder surfaces, explicit interfaces (`ToolMessage(status="error")`, `StateSnapshot.tasks[].error`, `LifecyclePayload.error`, `NodeError`), strong configurability (`handle_tool_errors`, `RetryPolicy`, `TimeoutPolicy`, `TracePolicy`, stream modes), durable checkpointed failure records, and extensive tests exercising each path. Falls short of 9–10 because structured error codes cover only five cases (`ErrorCode`, `libs/langgraph/langgraph/errors.py:34-39`), several surfaces degrade errors to untyped `str(err)` payloads, core/prebuilt logging is minimal-to-absent, and message-stream errors are swallowed without any user-visible signal.

## Evidence Collected

Every entry includes a file path with line numbers. All paths are relative to `studies/agent-harness-study/sources/langgraph/`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Error taxonomy | Typed exception hierarchy incl. `GraphRecursionError`, `InvalidUpdateError`, `NodeCancelledError`, `NodeTimeoutError`; control-flow sentinels (`GraphBubbleUp`, `GraphInterrupt`) kept separate from real failures | libs/langgraph/langgraph/errors.py:50-145 |
| Structured error codes + docs links | `ErrorCode` enum (5 values) and `create_error_message()` appending a troubleshooting URL to every coded message | libs/langgraph/langgraph/errors.py:34-47 |
| Model sees tool errors | Handled tool failures become `ToolMessage(content=..., status="error")` addressed to the failing `tool_call_id` | libs/prebuilt/langgraph/prebuilt/tool_node.py:1005-1012 |
| Model-facing templates | `TOOL_CALL_ERROR_TEMPLATE = "Error: {error}\n Please fix your mistakes."`; invocation/execution templates include tool name + kwargs | libs/prebuilt/langgraph/prebuilt/tool_node.py:108-121 |
| Invalid tool name guidance | `INVALID_TOOL_NAME_ERROR_TEMPLATE` lists available tools for the model | libs/prebuilt/langgraph/prebuilt/tool_node.py:108-110, 1268-1279 |
| Detail-level config (model) | `handle_tool_errors` accepts bool/str/callable/exception-type/tuple; default handles only arg-validation errors and re-raises execution errors | libs/prebuilt/langgraph/prebuilt/tool_node.py:383-391, 674-694, 749-753 |
| Validation-error filtering | Pydantic validation errors filtered to exclude injected args before being shown to the model via `ToolInvocationError(filtered_errors=...)` | libs/prebuilt/langgraph/prebuilt/tool_node.py:959-966, 360-375 |
| Interrupts never masked as errors | `GraphBubbleUp` re-raised untouched inside tool execution so interrupts are not converted into ToolMessages | libs/prebuilt/langgraph/prebuilt/tool_node.py:973-983 |
| Recursion-limit message | Actionable `GraphRecursionError` naming the limit and the `recursion_limit` config key, with docs URL | libs/langgraph/langgraph/pregel/main.py:3002-3011 |
| Timeout detail | `NodeTimeoutError` carries node, elapsed, kind (`idle`/`run`), and both configured timeout values in its message | libs/langgraph/langgraph/errors.py:190-241 |
| User-callback on failure | Sync/async run managers invoke `on_chain_error(e)` before re-raising | libs/langgraph/langgraph/pregel/main.py:3018-3021, 3495-3501 |
| Traceback hygiene | Framework files excluded from exception tracebacks before re-raise | libs/langgraph/langgraph/pregel/_runner.py:58-68, 240-248 |
| Task context on exceptions | `exc.add_note("During task with name '...' and id '...'")` on Python ≥3.11 | libs/langgraph/langgraph/pregel/_retry.py:641-643, 795-798 |
| Errors persisted durably | Failed tasks write `(ERROR, exception)` to the checkpointer via `commit()` | libs/langgraph/langgraph/pregel/_runner.py:574-603 |
| Internal error channel | `ERROR = "__error__"` and `ERROR_SOURCE_NODE = "__error_source_node__"` interned constants | libs/langgraph/langgraph/_internal/_constants.py:13-15 |
| Post-hoc inspection | `StateSnapshot.tasks[].error` exposes persisted failures; test asserts `RuntimeError('Simulated failure')` recoverable from state history after crash | libs/langgraph/langgraph/types.py:637-646, 683-701; libs/langgraph/tests/test_pregel.py:5408-5427 |
| Debug stream shows errors | `map_debug_task_results` emits `"error"` key per task result; debug checkpoints include task errors | libs/langgraph/langgraph/pregel/debug.py:106-128, 176-206 |
| Errors excluded from `updates` stream | `output_writes` skips emitting writes whose first channel is ERROR | libs/langgraph/langgraph/pregel/_loop.py:1452-1459 |
| Hidden-task filtering | Tasks tagged `TAG_HIDDEN` suppressed from user-facing streams/debug output | libs/langgraph/langgraph/pregel/_loop.py:1419-1423; libs/langgraph/langgraph/pregel/debug.py:43-45 |
| Message-stream gap | Messages handler's `on_llm_error`/`on_chain_error` only pop bookkeeping metadata — no error payload reaches the messages stream | libs/langgraph/langgraph/pregel/_messages.py:181-189, 248-256 |
| v3 streaming error surfacing | Run streams raise the original error from `.output`/iteration; subgraph handles expose `.status == "failed"` and `.error` string | libs/langgraph/langgraph/stream/run_stream.py:185-216, 285-310; libs/langgraph/stream tests at libs/langgraph/tests/test_stream_events_v3_e2e.py:500-525 |
| Lifecycle events for operators | `LifecyclePayload {event, namespace, cause?, error?}`; `_status_from_exception` maps drain/interrupt/failure to statuses | libs/langgraph/langgraph/stream/transformers.py:343-370, 582-605 |
| Node-level error handlers | `add_node(..., error_handler=...)`; handler receives typed `NodeError(node, error)`; default graph-wide handler supported | libs/langgraph/langgraph/graph/state.py:277-332, 667-722, 867-916; libs/langgraph/langgraph/errors.py:148-165 |
| Handler task injection | `prepare_node_error_handler_task` injects `CONFIG_KEY_NODE_ERROR: NodeError(...)` into the handler task config | libs/langgraph/langgraph/pregel/_algo.py:1110-1248 (1236-1238) |
| Retry logging | Retries logged at INFO with class name, delay, attempt count and `exc_info=True` (full traceback) | libs/langgraph/langgraph/pregel/_retry.py:676-680, 832-836 |
| Retry policy config | `RetryPolicy(initial_interval, backoff_factor, max_interval, max_attempts, jitter, retry_on)`; `retry_on` supports type/sequence/callable | libs/langgraph/langgraph/types.py:418-437; libs/langgraph/langgraph/pregel/_retry.py:841-854 |
| Default retry classification | `default_retry_on`: retries connection/5xx; does NOT retry ValueError/TypeError/etc.; retries unknown exceptions | libs/langgraph/langgraph/_internal/_retry.py:1-29 |
| Timeout policy config | `TimeoutPolicy(run_timeout, idle_timeout, refresh_on="auto"\|"heartbeat")` — progress-refresh semantics documented | libs/langgraph/langgraph/types.py:451-481 |
| Operator attempt telemetry | `_AttemptEvent{event: start\|progress\|finish, status, error_type, error_message}` dispatched to a server-injected observer (explicit internal contract for langgraph-server) | libs/langgraph/langgraph/pregel/_retry.py:87-125, 92-96, 370-391, 407-414 |
| Trace redaction hooks | `TracePolicy.process_inputs/process_outputs` transforms recorded span payloads (explicitly not for secret redaction — deferred to LangSmith client) | libs/langgraph/langgraph/types.py:532-556 |
| SDK error hierarchy | `APIStatusError` captures `request_id` from `x-request-id`; body-based message extraction; status-specific subclasses (400…500) | libs/sdk-py/langgraph_sdk/errors.py:13-137, 82-93, 140-158 |
| SDK error logging | Client-side `logger.error(f"Error from langgraph-api: ...")` on non-2xx | libs/sdk-py/langgraph_sdk/errors.py:220, 230 |
| Tests: model-visible errors | Parametrized tests assert `ToolMessage.status == "error"`, exact template content, callable/type-tuple strategies, re-raise when unhandled | libs/prebuilt/tests/test_tool_node.py:322-444, 475-520 |
| Tests: durability of failure record | `test_checkpoint_recovery` verifies error visible in `get_state` and checkpoint history, then successful resume | libs/langgraph/tests/test_pregel.py:5372-5432 |
| Tests: recursion error raised | `pytest.raises(GraphRecursionError)` on exhausted step budget | libs/langgraph/tests/test_pregel.py:588-589 |
| Logging infrastructure | Dedicated `"langgraph"` logger defined but used in very few places | libs/langgraph/langgraph/pregel/_log.py:1-3 |

## Answers to Dimension Questions

**1. Is the model informed of failures?**
Yes — for tools. `ToolNode._execute_tool_sync` catches handled exceptions and returns `ToolMessage(content, name, tool_call_id, status="error")` back into the conversation (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1005-1012`). Default content uses `"Error: {error!r}\n Please fix your mistakes."` (`tool_node.py:111`), invalid tool names get a list of valid alternatives (`tool_node.py:108-110, 1268-1279`), and argument validation errors include field locations while filtering out framework-injected arguments (`tool_node.py:360-375, 959-966`). Graph-level failures (recursion limit, invalid updates) are *not* fed back to the model; they raise out of the run. Control-flow signals (`GraphInterrupt`, `ParentCommand`) are deliberately exempted from error conversion (`tool_node.py:973-983`).

**2. Is the user informed appropriately?**
Mostly yes. Callers get typed exceptions with remediation hints and documentation URLs (`errors.py:42-47`, `main.py:3002-3011`), clean tracebacks with framework frames pruned (`_runner.py:240-248`) and task-name notes attached (`_retry.py:642-643`). Interrupts surface as data, not errors: `{__interrupt__: [...]}` updates (`_loop.py:1365-1371`) and `Interrupt(value, id)` objects in snapshots (`types.py:573-614, 683-701`). Two gaps: (a) errors are intentionally withheld from `updates`/`values` streams (`_loop.py:1452`) — users must opt into `debug` mode or inspect state to see per-task failures; (b) `stream_mode="messages"` drops LLM errors entirely (`_messages.py:181-189, 248-256`), so partial token streams end without an in-band error marker.

**3. Can developers debug failures?**
Yes, through four complementary channels: (1) durable `(ERROR, exception)` writes to the checkpointer make any past failure replayable from `get_state_history` — proven by `test_checkpoint_recovery` (`test_pregel.py:5372-5432`); (2) LangChain callbacks fire `on_chain_error` at graph scope (`main.py:3018-3021`) and `on_tool_error` at tool scope (`_tools.py:260-268`), integrating with LangSmith-style tracers; (3) node-scoped error handlers receive structured `NodeError(node, error)` context (`errors.py:148-165`, injected at `_algo.py:1236-1238`); (4) retry attempts are logged with full tracebacks (`_retry.py:677-680`). Weakness: outside retries and a few warnings, there is almost no conventional logging — `prebuilt` contains zero logger calls, so developers integrating LangGraph without a tracer or checkpointer rely solely on raised exceptions.

**4. Can operators detect failure patterns?**
Partially, via OSS primitives that clearly target the LangGraph Platform/server: the `lifecycle` stream channel publishes per-subgraph terminal statuses (`failed`/`drained`/`interrupted`) with error strings (`transformers.py:343-370, 582-605`), and timed attempts emit observer events with `status="error"`, `error_type`, `error_message` explicitly documented as consumed by langgraph-server (`_retry.py:87-125`). The SDK surfaces HTTP failure patterns with request IDs for correlation (`sdk-py/langgraph_sdk/errors.py:82-93`). However, the OSS repo itself ships no metrics, health endpoints, or dashboards — operator monitoring depends on external infrastructure (LangSmith/LangGraph Platform), and `TracePolicy` explicitly delegates redaction/observability controls to the LangSmith client (`types.py:532-556`).

## Architectural Decisions

1. **Errors-as-data at the model boundary, errors-as-exceptions everywhere else.** The only place failures become conversational content is the ToolNode→model interface (`tool_node.py:1005-1012`); everything above that is Python exceptions. This keeps the model loop self-healing while preventing framework internals from leaking into conversation.
2. **Control-flow "failures" are a separate exception lineage.** `GraphBubbleUp`/`GraphInterrupt`/`ParentCommand` subclass a common base (`errors.py:50-133`) that is exempt from retry handling, error-handler routing, ToolMessage conversion, and sibling-task cancellation (`_retry.py:632-634`, `_runner.py:592-594, 616-634`, `tool_node.py:982-983`). This prevents interrupts from polluting every failure surface.
3. **Persist failures, not just state.** Writing `(ERROR, exc)` pending writes to the checkpointer (`_runner.py:597-603`) makes failure a first-class, durable part of run history rather than a transient log line — enabling post-mortem queries and safe resume-after-fix semantics demonstrated in `test_pregel.py:5372-5432`.
4. **Structured codes only where actionability is generic.** Only five `ErrorCode`s exist (`errors.py:34-39`), each with a generated troubleshooting URL (`errors.py:42-47`); everything else relies on exception types. Codes are reserved for the small set of errors users can fix from documentation alone.
5. **Detail levels tuned per audience by construction, not by filter.** Model messages use directive phrasing ("fix your mistakes"); caller tracebacks have framework frames surgically removed (`_runner.py:58-68`); server observers receive machine-readable `{error_type, error_message}` pairs (`_retry.py:386-389`).

## Notable Patterns

- **Traceback laundering**: recursive removal of trailing framework frames so the top of a user's stack trace points at their own code (`_runner.py:240-248`, exclusion list at 58-68).
- **Exception annotations**: `exc.add_note(f"During task with name '{task.name}' and id '{task.id}'")` enriches tracebacks without wrapping (`_retry.py:641-643, 797-798`) — preserving exception identity for `retry_on` matching.
- **Type-signature-driven error routing**: `_infer_handled_types` inspects a custom handler's parameter annotation to decide which exceptions it handles, converting mismatches into re-raises (`tool_node.py:444-479`).
- **Silent-teardown vs. real-failure disambiguation**: `asyncio.Task.cancelling() == 0` distinguishes user-raised `CancelledError` (converted to reportable `NodeCancelledError`) from framework-initiated sibling cancellation (`_retry.py:315-334, 777-794`; rationale at `errors.py:168-187`) — closing a visibility hole where failed runs reported success (referenced bug LSD-1507).
- **Debug-mode remapping**: internal `checkpoints`/`tasks` events are re-wrapped into a single timestamped `debug` envelope so operators get one coherent audit stream (`_loop.py:1380-1414`).
- **Hidden-tag suppression**: `TAG_HIDDEN` tasks are excluded from user streams and debug output (`_loop.py:1419-1423`, `debug.py:43-45`), separating framework plumbing from user-visible work.

## Tradeoffs

- **Opt-in depth vs. discoverability**: per-task errors are absent from `updates`/`values` modes (`_loop.py:1452`), keeping those streams clean for UI consumption, but a developer using only `updates` will not see why a task died without catching the terminal exception or switching modes.
- **Default tool-error strictness**: `ToolNode`'s default converts only argument-validation failures to model-visible messages and re-raises execution errors (`tool_node.py:383-391, 690-694`); `create_react_agent` does not override this (`chat_agent_executor.py:554-561`), so one flaky tool crashes the whole agent run unless users know to set `handle_tool_errors=True`. Predictable for experts, punishing for novices.
- **Stringly-typed edge payloads**: lifecycle and task-result error fields are `str(err)` (`transformers.py:588, 604`; `debug.py:118`), losing exception class and structure at exactly the boundaries remote clients consume; only the server-internal observer channel gets typed `{error_type, error_message}` (`_retry.py:386-389`).
- **Callback coupling over logging**: developer observability leans on LangChain's callback system and optional tracing integrations rather than standard logging; teams without a tracer get near-zero runtime diagnostics from the framework (contrast sparse `logger.*` usage list across `libs/langgraph`).
- **Durability vs. latency**: whether error writes reach the checkpointer immediately depends on the durability setting (`put_writes` gating at `_loop.py:466`, tested across durability modes in `test_pregel.py:5420-5423`), trading failure-record timeliness against throughput.

## Failure Modes / Edge Cases

- **Message-stream silence on LLM failure** (`_messages.py:181-189, 248-256`): a consumer rendering tokens live receives no error event; the failure appears only as stream termination/exception on the outer call.
- **Unhandled tool exceptions abort multi-tool steps**: with default settings, if one tool raises a non-validation error alongside others in the same AI message, the whole superstep fails (`tool_node.py:383-391` default + `_should_stop_others` cancellation at `_runner.py:616-634`).
- **Concurrent failure semantics**: first non-handled exception cancels siblings and panics the step (`_panic_or_proceed`, `_runner.py:650-697`); handled (error-handler-routed) exceptions are excluded via id-tracking (`_handled_exception_ids`, `_runner.py:166-169, 224-248`) to avoid double-reporting the same failure through fatal and recovery paths.
- **Sync-node cancellation ambiguity**: sync nodes cannot distinguish user vs. framework cancellation (documented at `state.py:715-722`; async-only timeout enforcement at `_retry.py:580-583`), so timeouts and user-cancel reporting are async-node capabilities only.
- **Watchdog races**: `asyncio.wait(FIRST_COMPLETED)` may complete both the task and watchdog; spurious watchdog `TimeoutError`s are suppressed so they never masquerade as `NodeTimeoutError` (`_retry.py:468-481`).
- **Observer/handler failures must not mask the primary error**: observer dispatch swallows and logs handler exceptions (`_retry.py:407-414`); trace-payload transform failures fall back to raw values (`_runnable.py:70-85`).

## Future Considerations

- Extend the `ErrorCode` taxonomy (currently 5 values, `errors.py:34-39`) to cover common node/subgraph failures so remote clients can branch on stable identifiers instead of parsing `str(err)`.
- Add structured error objects (type + message + optional stack) to `TaskResultPayload.error` and `LifecyclePayload.error` (`debug.py:118`, `transformers.py:370`) instead of plain strings.
- Introduce consistent INFO/WARN logging around task commit failures and error-handler routing in `PregelRunner` (`_runner.py:574-614`), and any logging at all in `prebuilt`.
- Emit an explicit error/abort marker into `messages`-mode streams (`_messages.py`) so UI consumers can render failed generations faithfully.
- Document and/or reconsider the strict `ToolNode` default in `create_react_agent` (`chat_agent_executor.py:554-561`), e.g. a first-class flag propagating `handle_tool_errors` for agent builders.

## Questions / Gaps

- **JS parity unverifiable in this snapshot**: `libs/sdk-js/` contains only `README.md` (submodule not checked out), so cross-language consistency of the typed error hierarchy could not be assessed. Searched `sources/langgraph/libs/sdk-js/**` — no source files present.
- **No OSS monitoring dashboards/metrics found**: searched `docs/`, `libs/langgraph`, `libs/cli` for metrics/dashboards/prometheus references — none; operator story appears fully delegated to LangGraph Platform (inferred from the server-consumed observer contract comment at `_retry.py:92-96` and `CONFIG_KEY_TIMED_ATTEMPT_OBSERVER`). No in-repo evidence of the server implementation itself.
- **Doc-site troubleshooting pages not in-repo**: error messages link to `docs.langchain.com/.../errors/{code}` (`errors.py:42-47`), but the doc content lives outside this repository; only `docs/llms.txt` and redirect manifests exist locally, so documented-vs-implemented behavior parity was not verifiable.
- **`on_chain_error` fan-out depth**: the root run manager fires `on_chain_error` (`main.py:3018-3021`); per-node chain-error callbacks originate from the runnable wrapper (`_internal/_runnable.py:442, 517, 712, 765`), but a systematic enumeration of which intermediate scopes also fire was not exhaustively traced beyond these call sites.

---

Generated by `dimensions/13.03-failure-visibility.md` against `langgraph`.
