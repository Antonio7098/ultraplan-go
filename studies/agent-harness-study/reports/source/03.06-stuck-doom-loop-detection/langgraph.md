# Source Analysis: langgraph

## 03.06 — Stuck and Doom-Loop Detection

### Source Info

| Field | Value |
|-------|-------|
| Name | langgraph |
| Path | `studies/agent-harness-study/sources/langgraph` |
| Language / Stack | Python 3.10+ (core `langgraph` package, `langgraph-prebuilt`); monorepo with parallel `checkpoint`, `checkpoint-postgres`, `checkpoint-sqlite`, `cli`, `sdk-py`, `sdk-js` (TypeScript) libraries |
| Analyzed | 2026-07-15 |

## Summary

LangGraph ships **no first-class doom-loop detector**. The framework relies on four blunt, count- or time-based safety mechanisms, none of which inspect recent tool calls, message history, or task output to detect *patterns*:

1. **Recursion limit (superstep counter).** `lib/libs/langgraph/langgraph/_internal/_config.py:32` defines `DEFAULT_RECURSION_LIMIT = int(getenv("LANGGRAPH_DEFAULT_RECURSION_LIMIT", "10007"))`. The Pregel loop's `tick()` method short-circuits when `self.step > self.stop` (`libs/langgraph/langgraph/pregel/_loop.py:600-602`), and the parent runner raises `GraphRecursionError` (`libs/langgraph/langgraph/errors.py:67-87`) when the loop exits with `status == "out_of_steps"` (`libs/langgraph/langgraph/pregel/main.py:3017-3026` and `3498-3507`). `stop` is `self.step + self.config["recursion_limit"] + 1` (`libs/langgraph/langgraph/pregel/_loop.py:1677,1936`). The cap is checked *first* in `tick()` (`_loop.py:599-602`), so the loop stops before any tool runs once the cap is hit.
2. **Step timeout.** `Pregel.step_timeout: float | None = None` (`libs/langgraph/langgraph/pregel/main.py:726-727,816`) is wired into `runner.tick(..., timeout=self.step_timeout, ...)` (`main.py:2984,3457`), which raises an `asyncio.TimeoutError` / `TimeoutError` if a superstep does not finish within the budget (`libs/langgraph/langgraph/pregel/_runner.py:280-289,479-487,696-697`). This is a per-superstep watchdog, not a stuck-pattern detector.
3. **Per-task `RetryPolicy` + `TimeoutPolicy`.** `RetryPolicy(max_attempts=3, initial_interval=0.5, backoff_factor=2.0, max_interval=128.0, jitter=True, retry_on=...)` (`libs/langgraph/langgraph/types.py:416-435`) drives `run_with_retry` / `arun_with_retry` (`libs/langgraph/langgraph/pregel/_retry.py:573-682, 685-854`). `TimeoutPolicy(run_timeout=..., idle_timeout=..., refresh_on="auto"|"heartbeat")` (`libs/langgraph/langgraph/types.py:449-509`) drives the `_TimedAttemptScope` and `_IdleProgressCallbackHandler` in `_retry.py:128-340`, which refresh idle-timeout on any LangChain callback event (`_retry.py:274-313`). These mechanisms catch *single-task* failures and stalls; they do not detect *cross-task* loops.
4. **Prebuilt `create_react_agent` "soft bail" via `RemainingSteps`.** The prebuilt ReAct agent injects a managed value `RemainingSteps = scratchpad.stop - scratchpad.step` (`libs/langgraph/langgraph/managed/is_last_step.py:18-24`). When the LLM response has tool calls but `remaining_steps < 2`, the agent short-circuits with a synthetic AI message `"Sorry, need more steps to process this request."` (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:620-634, 684-692, 711-719`) instead of looping again. This is a *threshold-based* cap driven by the same `recursion_limit`; it is not a doom-loop pattern detector.

**What is absent.** `grep -rn` across `libs/` for `doom`, `loop_detection`, `consecutive.*tool`, `recent_tool_calls`, `repeated_action`, `no_progress`, and `replan` returns **0 hits** in `langgraph/` and `prebuilt/`. There is no detector for repeated identical tool calls, alternating A/B tool-call patterns, "no-progress monologues", or for changes between successive `AIMessage`/`ToolMessage` pairs. Compaction / summarisation strategies (`grep "summariz|truncat|compaction"`) return 0 hits in the framework code (only in unrelated third-party tests and a Postgres store namespace-truncation utility at `libs/checkpoint-postgres/langgraph/store/postgres/base.py:575-616`). Doom-loop evidence therefore **cannot be erased by a framework-provided compaction**, but it also cannot be *found* by one — there is no sliding-window history of tool calls to inspect. Detection reduces to "how many supersteps have run?" and "has the current superstep timed out?".

**Answering the prompt's headline question** ("Does the system stop repeated behavior before the user pays for 20 useless turns?"): **Yes, but only when the user has explicitly tuned `recursion_limit` and/or `step_timeout`**. The default `recursion_limit=10007` (`libs/langgraph/langgraph/_internal/_config.py:32`) means a graph will burn ~10k supersteps before stopping, which is far past 20 turns in a typical ReAct agent. There is no built-in signal that fires earlier than the count cap, no matter how obvious the doom loop is.

## Rating

**Score: 3 / 10** — Present but implicit, blunt, and configuration-dependent. Every "detection" is a counter or timer with no pattern recognition. The prebuilt ReAct agent's `RemainingSteps` "Sorry, need more steps" escape hatch is the closest thing to an "early stop" signal and is also counter-driven. The framework explicitly *delegates* pattern detection to the LLM (via its own reasoning about "should I keep calling the same tool?") or to user code — there is no `DoomLoopDetector` class, no `recent_tool_calls` window, no `_should_stop_loop(state)` predicate, and no `recent_messages` field on `AgentState` (the prebuilt `AgentState` has only `messages` and `remaining_steps` — `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:57-62`). False positives from the framework's own mechanisms are impossible (because they don't inspect content), but false negatives are the norm: identical-but-successful tool calls, alternating A/B patterns, and "I keep reading the same file" no-progress monologues all consume the full recursion budget.

## Evidence Collected

Every entry includes file path and line numbers. Format: `path/to/file.py:NN`.

### Recursion-limit detection (superstep counter)

| Area | Evidence | File:Line |
|------|----------|-----------|
| Default recursion limit constant | `DEFAULT_RECURSION_LIMIT = int(getenv("LANGGRAPH_DEFAULT_RECURSION_LIMIT", "10007"))` | `libs/langgraph/langgraph/_internal/_config.py:32` |
| Validation that limit is ≥1 | `if config["recursion_limit"] < 1: raise ValueError("recursion_limit must be at least 1")` | `libs/langgraph/langgraph/pregel/main.py:2578-2579` |
| `stop` set on the loop (sync) | `self.stop = self.step + self.config["recursion_limit"] + 1` | `libs/langgraph/langgraph/pregel/_loop.py:1677` |
| `stop` set on the loop (async) | same expression | `libs/langgraph/langgraph/pregel/_loop.py:1936` |
| Step-cap check at start of `tick()` | `if self.step > self.stop: self.status = "out_of_steps"; return False` | `libs/langgraph/langgraph/pregel/_loop.py:599-602` |
| `out_of_steps` → `GraphRecursionError` (sync) | `raise GraphRecursionError(msg)` with `ErrorCode.GRAPH_RECURSION_LIMIT` | `libs/langgraph/langgraph/pregel/main.py:3017-3026` |
| `out_of_steps` → `GraphRecursionError` (async) | same | `libs/langgraph/langgraph/pregel/main.py:3498-3507` |
| Error class definition | `class GraphRecursionError(RecursionError)` | `libs/langgraph/langgraph/errors.py:67-87` |
| Error code enum | `ErrorCode.GRAPH_RECURSION_LIMIT = "GRAPH_RECURSION_LIMIT"` | `libs/langgraph/langgraph/errors.py:34-39` |
| Error message suggests raising the limit | `"You can increase the limit by setting the 'recursion_limit' config key."` | `libs/langgraph/langgraph/pregel/main.py:3020-3022`, `3501-3503` |
| Default propagates through `ensure_config` | `recursion_limit=DEFAULT_RECURSION_LIMIT` | `libs/langgraph/langgraph/_internal/_config.py:335` |
| SDK exposes recursion_limit in `Config` | `recursion_limit: int` (docstring says "defaults to 25" — actually 10007 in engine) | `libs/sdk-py/langgraph_sdk/schema.py:194-197` |

### Step timeout (superstep watchdog)

| Area | Evidence | File:Line |
|------|----------|-----------|
| Public attribute | `step_timeout: float \| None = None` with docstring `"Maximum time to wait for a step to complete, in seconds."` | `libs/langgraph/langgraph/pregel/main.py:726-727` |
| Attribute stored on the instance | `self.step_timeout = step_timeout` | `libs/langgraph/langgraph/pregel/main.py:816` |
| Used by runner in stream loop (sync) | `for _ in runner.tick([...], timeout=self.step_timeout, ...)` | `libs/langgraph/langgraph/pregel/main.py:2982-2984` |
| Used by runner in stream loop (async) | same | `libs/langgraph/langgraph/pregel/main.py:3455-3457` |
| Sync timeout enforcement | `timeout=(max(0, end_time - time.monotonic()) if end_time else None)` | `libs/langgraph/langgraph/pregel/_runner.py:280-289` |
| Async timeout enforcement | same, against `loop.time()` | `libs/langgraph/langgraph/pregel/_runner.py:479-487, 540-548` |
| Timeout raises | `raise timeout_exc_cls("Timed out")` (default `TimeoutError`) | `libs/langgraph/langgraph/pregel/_runner.py:696-697, 653` |
| Async timeout class | `timeout_exc_cls=asyncio.TimeoutError` | `libs/langgraph/langgraph/pregel/_runner.py:559` |
| Step-timeout tests exist (public surface, not loop-detection test) | `test_step_timeout_on_stream_hang` | `libs/langgraph/langgraph/tests/test_pregel_async.py:1164-1187` |

### Per-task retry (transient-failure handling, not a doom detector)

| Area | Evidence | File:Line |
|------|----------|-----------|
| `RetryPolicy` dataclass | `max_attempts=3, initial_interval=0.5, backoff_factor=2.0, max_interval=128.0, jitter=True` | `libs/langgraph/langgraph/types.py:416-435` |
| Default `retry_on` predicate | `default_retry_on` (defined elsewhere; only retries *exceptions*, not successful-but-no-progress tasks) | `libs/langgraph/langgraph/types.py:432-435` |
| Sync `run_with_retry` | Increments `attempts` only on exception, gives up at `attempts >= matching_policy.max_attempts` | `libs/langgraph/langgraph/pregel/_retry.py:600-682` |
| Async `arun_with_retry` | Same loop, with timed-attempt scope integration | `libs/langgraph/langgraph/pregel/_retry.py:719-854` |
| User-raised `CancelledError` becomes `NodeCancelledError` | `raise NodeCancelledError(task.name) from exc` (so silent cancellation ≠ loop silence) | `libs/langgraph/langgraph/pregel/_retry.py:635-640` |

### Per-task idle / run timeout (progress-aware timeout, not a doom detector)

| Area | Evidence | File:Line |
|------|----------|-----------|
| `TimeoutPolicy` dataclass | `run_timeout`, `idle_timeout`, `refresh_on: Literal["auto","heartbeat"]` | `libs/langgraph/langgraph/types.py:449-509` |
| Idle-progress callback handler resets clock on any LC callback | `on_llm_start = _touch`, `on_tool_start = _touch`, etc. (17 callbacks) | `libs/langgraph/langgraph/pregel/_retry.py:274-313` |
| `_TimedAttemptScope.wait_for_idle_timeout` raises `asyncio.TimeoutError` when clock expires | `if remaining <= 0: raise asyncio.TimeoutError` | `libs/langgraph/langgraph/pregel/_retry.py:215-223` |
| `runtime.heartbeat()` manual progress | `heartbeat=self.touch` patched onto `Runtime` | `libs/langgraph/langgraph/pregel/_retry.py:181-183` |
| Heartbeat tests (idle timeout fires) | `test_arun_with_retry_idle_timeout_resets_on_runtime_heartbeat`, `test_arun_with_retry_heartbeat_refresh_mode_*` | `libs/langgraph/langgraph/tests/test_retry.py:1006-1059, 1693-1730` |

### Prebuilt ReAct agent "soft bail" via managed `RemainingSteps`

| Area | Evidence | File:Line |
|------|----------|-----------|
| `IsLastStepManager` (boolean) | `return scratchpad.step == scratchpad.stop - 1` | `libs/langgraph/langgraph/managed/is_last_step.py:9-12` |
| `RemainingStepsManager` (int) | `return scratchpad.stop - scratchpad.step` | `libs/langgraph/langgraph/managed/is_last_step.py:18-21` |
| `RemainingSteps` annotation re-export | `from langgraph.managed.is_last_step import IsLastStep, RemainingSteps` | `libs/langgraph/langgraph/managed/__init__.py:1-3` |
| `_are_more_steps_needed` predicate | `if remaining_steps is not None: ... elif remaining_steps < 2 and has_tool_calls: return True` | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:620-634` |
| Soft-bail message (sync) | `AIMessage(id=response.id, content="Sorry, need more steps to process this request.")` | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:684-692` |
| Soft-bail message (async) | same | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:711-719` |
| Docstring `remaining_steps` semantics | `"Calculated roughly as recursion_limit - total_steps_taken"` | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:435-440` |
| Test asserts soft-bail fires when limit is hit | `app.invoke({...}, {"recursion_limit": 2})` returns `[_AnyIdAIMessage(content="Sorry, need more steps...")]` | `libs/langgraph/langgraph/tests/test_large_cases.py:1524-1533` |
| `AgentState` schema | `messages: ...; remaining_steps: NotRequired[RemainingSteps]` (no `recent_tool_calls`, no `error_count`, no `last_tool_call`) | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:57-62` |

### "Plan" and "loop" terminology in the engine — none refer to doom-loop detection

| Area | Evidence | File:Line |
|------|----------|-----------|
| BSP "Plan" phase (algorithm, not LLM plan) | "Plan: Determine which actors to execute in this step" | `libs/langgraph/langgraph/pregel/main.py:465-468` |
| Per-superstep actor selection (`prepare_next_tasks`) | Selects PUSH + PULL tasks from `trigger_to_nodes + updated_channels` | `libs/langgraph/langgraph/pregel/_algo.py:392-513` |
| Graph-lifecycle events (resume / interrupt) — not loop detection | `_graph_lifecycle_events: deque()` | `libs/langgraph/langgraph/pregel/_loop.py:315` |

### Search boundaries (negative evidence)

| Area | Search | Result |
|------|--------|--------|
| Doom / doom-loop keyword | `grep -rn "doom\|loop_detection\|loop_detect" libs/` | 0 hits in `langgraph/` and `prebuilt/` (only in unrelated test fixture `tests/fixtures/response.txt` referencing auth tokens) |
| Repeated / consecutive tool calls | `grep -rn "consecutive.*tool\|same.*tool.*call\|repeat.*tool\|recent_tool" libs/` | 0 hits in `langgraph/` and `prebuilt/` |
| Recent-event window | `grep -rn "recent_calls\|recent_messages\|last_n\|previous_message" libs/` | 0 hits in `langgraph/` and `prebuilt/` (only in unrelated `checkpoint` / `cli` / `sdk-py` paths) |
| No-progress / monologue detection | `grep -rn "no_progress\|progress_check\|forward_prog\|alive.*check" libs/langgraph/langgraph libs/prebuilt/langgraph/prebuilt` | 0 hits |
| Replan / replanning event | `grep -rn "replan" libs/langgraph/langgraph libs/prebuilt/langgraph/prebuilt` | 0 hits |
| Summary / compaction / truncation of messages | `grep -rn "summariz\|truncat\|compaction" libs/langgraph/langgraph libs/prebuilt/langgraph/prebuilt` | 0 hits in framework code |
| Doom-loop detector class | `grep -rn "class .*LoopDetect\|class .*Doom\|class .*Stuck" libs/` | 0 hits |
| `recent_messages` / `recent_tool_calls` field on any state schema | `grep -rn "recent_messages\|recent_tool_calls" libs/langgraph/langgraph libs/prebuilt/langgraph/prebuilt` | 0 hits |

## Answers to Dimension Questions

### 1. What stuck patterns are detected?

**None.** LangGraph's stuck detection is *not* pattern-based. It only fires on:

- A counter hitting `recursion_limit` (count of Pregel supersteps since loop start — `libs/langgraph/langgraph/pregel/_loop.py:600-602, 1677, 1936`).
- A wall-clock timer hitting `step_timeout` (max seconds for a single superstep — `libs/langgraph/langgraph/pregel/main.py:726-727, 2982-2984`).
- A per-task `RetryPolicy` failing `max_attempts` (only counts *exceptions*, not no-progress — `libs/langgraph/langgraph/types.py:416-435, libs/langgraph/langgraph/pregel/_retry.py:641-682`).
- A per-task `TimeoutPolicy` failing `run_timeout` or `idle_timeout` (only fires on no-progress within a single task invocation — `libs/langgraph/langgraph/types.py:449-509, libs/langgraph/langgraph/pregel/_retry.py:128-340`).
- A managed-value threshold `RemainingSteps < 2 && has_tool_calls` in the prebuilt ReAct agent (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:620-634`).

**Not detected**: repeated identical tool calls (e.g. `search("same query")` 20 times in a row), alternating A/B tool-call patterns, "I keep reading the same file" no-progress monologues, repeated identical error responses, or "agent gets the same tool result and keeps calling the same tool anyway".

### 2. How far back does detection look?

- **Recursion limit** looks back to the **start of the run** (step counter is reset only at run boundaries — `libs/langgraph/langgraph/pregel/_loop.py:1676`). Window size = `recursion_limit` (default `10007` — `libs/langgraph/langgraph/_internal/_config.py:32`).
- **Step timeout** looks back **0 seconds** — it is a single-superstep wall-clock cap, not a moving window (`libs/langgraph/langgraph/pregel/main.py:726-727`).
- **`RetryPolicy.max_attempts`** looks back **across `max_attempts` consecutive failed attempts within one task invocation** (default 3 — `libs/langgraph/langgraph/types.py:428`).
- **`TimeoutPolicy.idle_timeout`** looks back **across the most recent progress-signal timestamp** for a single task invocation (`libs/langgraph/langgraph/pregel/_retry.py:161, 215-223`).
- **`RemainingSteps`** looks back **to the start of the run**, but is gated on the *current* `AIMessage` having `tool_calls` (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:627-632`). It does not remember history of past messages.

There is **no N-step sliding window** over tool calls, no `recent_tool_calls: deque[ToolCall]`, no `consecutive_error_count` field. The agent state in the prebuilt package has only `messages` and `remaining_steps` (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:57-62`).

### 3. Does compaction erase loop evidence?

**There is no framework-level compaction in `libs/langgraph/` or `libs/prebuilt/`** (0 hits for `summariz|truncat|compaction` in those paths). Because no pattern detection inspects message history, there is nothing for compaction to erase — but also nothing for compaction to feed. The only "compaction-like" behaviour in the broader monorepo is **outside the framework**:

- `libs/checkpoint-postgres/langgraph/store/postgres/base.py:575-616` truncates *namespace prefixes* in the store, unrelated to messages.
- `libs/cli/langgraph_cli/cli.py:212` and `libs/cli/langgraph_cli/deploy.py:1333` truncate CLI help text.
- `libs/checkpoint-sqlite/langgraph/store/sqlite/base.py:571-612` uses a CTE to split/truncate namespace prefixes.

The prebuilt `AgentState` deliberately uses `add_messages` reducer (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:60`), which **preserves all messages** (`libs/langgraph/langgraph/graph/message.py` `add_messages`). So message history accumulates without bound unless the user wires in their own compaction (via a custom reducer or a `pre_model_hook`). Loop-detection-relevant evidence is therefore never erased by the framework, but it is also never *read*.

### 4. What intervention happens?

| Detector | Intervention |
|----------|--------------|
| `recursion_limit` exceeded | Loop exits with `status = "out_of_steps"`; runner raises `GraphRecursionError` (`RecursionError`) with code `GRAPH_RECURSION_LIMIT` (`libs/langgraph/langgraph/pregel/main.py:3017-3026, libs/langgraph/langgraph/errors.py:67-87`). The error message tells the user to "increase the limit by setting the `recursion_limit` config key". **Stops the loop.** |
| `step_timeout` exceeded | `asyncio.TimeoutError` / `TimeoutError` raised out of `runner.tick` (`libs/langgraph/langgraph/pregel/_runner.py:696-697`). Propagates up; the run fails. **Stops the superstep.** |
| `RetryPolicy.max_attempts` exceeded | Original exception re-raised from `run_with_retry` (`libs/langgraph/langgraph/pregel/_retry.py:660-661`). **Stops the task.** |
| `TimeoutPolicy.run_timeout` / `idle_timeout` exceeded | `NodeTimeoutError` raised (`libs/langgraph/langgraph/errors.py:190-241`); falls into the same retry/error-handler path. **Stops the attempt.** |
| `RemainingSteps < 2 && has_tool_calls` in prebuilt ReAct | Synthetic `AIMessage(content="Sorry, need more steps to process this request.")` returned; graph routes through `tools_condition` → `__end__` because the message has no `tool_calls` (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:684-692, libs/prebuilt/langgraph/prebuilt/tool_node.py:1582-1585`). **Ends the run without raising.** |
| Node-level `error_handler=` | A user-supplied recovery node is invoked; the framework routes the exception via `_should_route_to_error_handler` (`libs/langgraph/langgraph/pregel/_runner.py:171-174`) and `schedule_error_handler` (`libs/langgraph/langgraph/pregel/_runner.py:305-323`). This is **user-supplied**, not framework-detected. |

**No intervention** triggers on pattern-based detection (identical calls, alternating calls, no-progress monologues). No "hint", no "compaction", no "escalation", no "replan" exists in the framework.

### 5. Are false positives possible?

- **Recursion limit**: **False positives possible** but extremely unlikely in well-shaped graphs — a 10007-step graph *might* be legitimate (long document processing, multi-agent handoffs). Users routinely raise it (`libs/langgraph/langgraph/bench/*.py` use `20000000000` — `bench/wide_state.py:156, bench/pydantic_state.py:320, bench/wide_dict.py:144, bench/sequential.py:39, bench/react_agent.py:74, bench/__main__.py:28-82`), confirming the default is too high for normal runs.
- **Step timeout**: **False positives possible** if a tool legitimately takes longer than the configured wall-clock; the timeout is coarse (per-superstep).
- **`RetryPolicy`**: **False positives possible** if the wrong exception type is matched (e.g. `retry_on=ConnectionError` would retry a user-code `KeyError` if user passed the wrong callable). Mitigated by the explicit `retry_on` predicate.
- **`TimeoutPolicy.idle_timeout`**: **False positives possible** if a tool legitimately pauses between events longer than `idle_timeout`. Mitigated by `refresh_on="auto"` or `runtime.heartbeat()`.
- **Prebuilt `RemainingSteps < 2` bail**: **False positives possible** for an agent that needs one more step to actually solve the problem. The agent returns a synthetic "Sorry" message instead of raising — the user sees a polite refusal, not a `RecursionError`.

**No false positives from content-based detection** — because there is none.

## Architectural Decisions

- **Count, not pattern.** LangGraph treats "stuck" as "too many supersteps" or "too many seconds per superstep". This is a deliberate scope choice: the framework is a *graph execution* engine, not an agent-judge. It deliberately delegates pattern recognition to the LLM (which can read the message history) or to the user (who can wire up a custom reducer or middleware).
- **`recursion_limit` is a single global per run.** There is no per-node cap. A loop in one node burns the same budget as a loop in another.
- **`RemainingSteps` is exposed as a managed value** (`libs/langgraph/langgraph/managed/is_last_step.py:18-24`) so user agents can read `stop - step` and decide for themselves. The prebuilt ReAct agent is the canonical consumer (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:620-634`).
- **Per-task `RetryPolicy` is in-band with the retry mechanism** (`libs/langgraph/langgraph/pregel/_retry.py:600-682, 719-854`): retry is implemented as a `while True` loop around `task.proc.invoke`, not as a graph-level planner. This means retries happen *inside* a single superstep; the framework's recursion counter does *not* increment on retry, only on superstep.
- **`TimeoutPolicy.refresh_on="auto"` reuses LangChain callbacks** (`libs/langgraph/langgraph/pregel/_retry.py:189-193, 274-313`). Idle-timeout fires when *no* LC callback (LLM, tool, retriever, chain, agent, text, retry, custom) has fired for `idle_timeout` seconds. This is closer to "no-progress" detection than the recursion limit, but it is scoped to a single task invocation.
- **`GraphRecursionError` is a `RecursionError` subclass** (`libs/langgraph/langgraph/errors.py:67-87`), so user code that catches the built-in `RecursionError` will catch it too. This is the **only** loop-related exit signal that propagates out of `invoke`/`ainvoke`/`stream`/`astream` at the framework boundary.

## Notable Patterns

- **Managed-value escape hatches.** `IsLastStep` and `RemainingSteps` (`libs/langgraph/langgraph/managed/is_last_step.py:9-24`) are managed values that let user graphs peek at loop status. The prebuilt agent uses `RemainingSteps` to soft-bail; users could write custom nodes that use these values to do richer checks.
- **Per-task `TimeoutPolicy` heartbeat.** `runtime.heartbeat()` (`libs/langgraph/langgraph/pregel/_retry.py:181-183`) lets long-running tools signal "I'm making progress" without producing an LC callback. Tests exercise this (`libs/langgraph/langgraph/tests/test_retry.py:1006-1059, 1693-1730`).
- **Cancellable sync paths.** Sync nodes cannot be timed out (`libs/langgraph/langgraph/pregel/_retry.py:580-583` raises `sync_timeout_unsupported`); only async nodes can use `run_timeout` / `idle_timeout`. This is a documented limitation.
- **Checkpoint-aware retry.** `CONFIG_KEY_RESUMING` is patched onto the config when retrying (`libs/langgraph/langgraph/pregel/_retry.py:682`), so downstream task code can know "I'm in a retry attempt".

## Tradeoffs

- **Counter-based cap is trivially correct** but pays 10007 iterations in worst case. Most ReAct-style doom loops need < 20 iterations to be obvious; the default would let them run to completion (or near-completion).
- **No content-based detection** means false positives are essentially zero for the framework's own mechanisms, but the user is left to do pattern detection themselves.
- **Prebuilt `RemainingSteps < 2` bail is the only "early stop"**, and it fires on a *fixed* threshold tied to `recursion_limit`. With the default `recursion_limit=10007`, an agent that emits tool calls on every iteration would only see the "Sorry" message after ~10005 iterations — far past the prompt's 20-turn budget.
- **`step_timeout` is per-superstep, not per-task.** A superstep can dispatch many parallel tasks; one slow task can consume the whole superstep budget. This is documented as "Maximum time to wait for a step to complete" (`libs/langgraph/langgraph/pregel/main.py:726-727`).
- **No compaction means no evidence erasure.** Long-running graphs grow their `messages` list without bound (unless the user adds a `pre_model_hook`). At some point this becomes the real bottleneck — context-window exhaustion — which is not detected by the framework.

## Failure Modes / Edge Cases

- **Identical-but-successful tool calls**: not detected. An agent that calls `search("same query")` and gets the same answer back will burn the full budget (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:620-634` only checks `remaining_steps`, not message content).
- **Alternating A/B patterns**: not detected. `call_A → call_B → call_A → call_B` burns the full budget.
- **Long-running successful tool with no intermediate progress**: not detected by `TimeoutPolicy(idle_timeout=..., refresh_on="auto")` because the tool itself will keep producing LC callbacks (e.g. `on_tool_start`, streamed tokens via `on_tool_end`). User must call `runtime.heartbeat()` manually or set `refresh_on="heartbeat"`.
- **Sync node hangs forever**: not detectable — sync timeouts are explicitly unsupported (`libs/langgraph/langgraph/pregel/_retry.py:580-583`). The only escape is the process being killed.
- **Prebuilt agent with `recursion_limit=10007`**: a typical ReAct agent will run for thousands of iterations even if the LLM is stuck in a loop.
- **Subgraph recursion**: a graph that calls a subgraph that calls the parent graph compounds `recursion_limit` across all levels. There is no per-level cap.
- **Resumed run with exhausted budget**: when resuming a checkpointed run, `self.step = self.checkpoint_metadata["step"] + 1` (`libs/langgraph/langgraph/pregel/_loop.py:1676`) — but `stop = self.step + self.config["recursion_limit"] + 1` (1677) is recomputed from the *new* run's config, so a run resumed with a lower `recursion_limit` may stop earlier. This is correct behaviour, but worth noting.
- **Cancelled task counted as success**: a framework-initiated `task.cancel()` (when a peer task fails — `libs/langgraph/langgraph/pregel/_retry.py:635-640`) is converted into `NodeCancelledError` only when `asyncio.Task.cancelling() == 0` (i.e. the user raised it). Framework-initiated cancellation is silent, so a task that hangs after sibling failure can stall the superstep until `step_timeout` fires.

## Future Considerations

- **Add a `recent_tool_calls` managed value** so user agents can detect identical-call patterns without writing their own reducer.
- **Add a `consecutive_error_count` managed value** so user agents (and a future prebuilt) can detect "same error N times in a row".
- **Lower the default `recursion_limit`** from 10007 (which appears to be chosen for Pregel benchmark numbers — `libs/langgraph/langgraph/bench/*:39,74,144,156,320`) to something like 25-50 for the prebuilt ReAct agent, with the agent using `RemainingSteps` to bail early.
- **Add a "soft loop breaker"** in `tick()` that compares the current channel-state hash to the previous-N channel-state hashes and short-circuits when they are equal. This is a cheap structural detector that catches identical-state doom loops.
- **Add a framework-side hook for compaction** (a `pre_model_hook` that summarises older messages) so long-running agents can survive without exhausting context. Currently the only official mitigation is user-supplied compaction via a custom reducer.
- **Wire `Runtime.heartbeat()` into the recursion-limit error path** so a graph that reports progress never trips the limit, while a stuck graph trips it earlier.

## Questions / Gaps

- **Is there a higher-level agent judge / pattern detector in `langgraph` (the org) that lives outside this repository?** The `LANGGRAPH_API_URL` references in `libs/sdk-py/tests/fixtures/response.txt` and the `langgraph_server` references throughout suggest a separate server component. If that server contains a doom-loop detector, it is not in this source tree and could not be evaluated. Search boundary: `libs/` only.
- **Are there any community middleware examples** (in `examples/` or docs) that implement doom-loop detection on top of `langgraph`? The repo's `examples/` directory exists (`studies/agent-harness-study/sources/langgraph/examples`) but was not exhaustively searched for custom detectors. Search boundary: only `examples/` exists in this source tree; deeper docs live at `docs.langchain.com`.
- **Does the TypeScript `sdk-js` library add detection?** `libs/sdk-js` is a parallel SDK for the LangGraph REST API and appears to be a thin client (per `libs/langgraph/AGENTS.md:25-30`). It likely defers all detection to the server. Search boundary: not opened in this analysis.
- **What happens at the very last step before the limit?** When `step == stop - 1`, `IsLastStepManager.get` returns `True` (`libs/langgraph/langgraph/managed/is_last_step.py:11-12`). The framework exposes this but does *not* act on it; users are expected to inspect `IsLastStep` in their own nodes. This is a deliberate "give the LLM one last chance" affordance, not a detector.

---

Generated by `03.06-stuck-and-doom-loop-detection` against `langgraph`.