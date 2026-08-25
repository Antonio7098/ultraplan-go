# Source Analysis: agent-framework

## 03.06 — Stuck and Doom-Loop Detection

### Source Info

| Field | Value |
|-------|-------|
| Name | agent-framework |
| Path | `studies/agent-harness-study/sources/agent-framework` |
| Language / Stack | Python (`agent-framework-core`, `_harness/`) + .NET (`Microsoft.Agents.AI`, `Microsoft.Agents.AI.Workflows`, `Microsoft.Agents.AI.Harness`) — two parallel implementations of the same framework |
| Analyzed | 2026-07-14 |

## Summary

The repository is Microsoft's `agent-framework`, a polyglot harness for AI agents that ships both a Python implementation (`python/packages/core/agent_framework/`) and a .NET implementation (`dotnet/src/Microsoft.Agents.AI*/`). Stuck / doom-loop detection is split between two layers: a per-tool-loop **consecutive-error counter** (Python and .NET, both backed by the same idea but different default thresholds) and a **per-orchestrator-loop judge / safety cap** (Python `AgentLoopMiddleware` + .NET `LoopAgent` and Magentic orchestrator). The .NET side additionally exposes an explicit LLM-judged "IsInLoop" / "IsProgressBeingMade" detector inside the Magentic multi-agent workflow that drives an automatic reset-and-replan intervention. Neither language has a static / structural "same-call-with-same-arguments-twice" detector; identical-but-successful tool calls are not flagged. Compaction is configurable and can erase older tool-call evidence — Python's `ToolResultCompactionStrategy` collapses older tool groups into `[Tool results: ...]` summaries (`python/packages/core/agent_framework/_compaction.py:820-895`) and .NET's `ToolResultCompactionStrategy.DefaultMinimumPreserved = 16` (`dotnet/src/Microsoft.Agents.AI/Compaction/ToolResultCompactionStrategy.cs:54`) likewise only protects the most-recent 16 tool groups. There is no "loop evidence retention" beyond that floor.

## Rating

**Score: 5 / 10**

Rationale: detection is **present but inconsistent and asymmetric across the two implementations**. The .NET Magentic orchestrator has a tested LLM-judged "is in loop / progress" detector with a documented stall threshold (`dotnet/src/Microsoft.Agents.AI.Workflows/Specialized/Magentic/MagenticTaskContext.cs:11-67`) and explicit reset-and-replan intervention (`dotnet/src/Microsoft.Agents.AI.Workflows/Specialized/Magentic/MagenticOrchestrator.cs:289-300`, `337-344`). The Python side, however, only has a `errors_in_a_row` counter driven by tool-execution exceptions (`python/packages/core/agent_framework/_tools.py:2249-2259`) and a hard `max_iterations` safety cap (`python/packages/core/agent_framework/_tools.py:90`); the primary non-streaming test for the consecutive-error behavior is **explicitly skipped** (`python/packages/core/tests/core/test_function_invocation_logic.py:1489` — "needs investigation in unified API"). Neither language has a structural detector for repeated-but-successful tool calls, alternating A/B patterns, or "no-progress" monologues at the chat-client function-invocation layer — that judgment is delegated to the LLM or to user-supplied evaluators. So the system does stop *some* wasted loops before 20 turns, but only for repeated errors and only in the LLM-judged multi-agent path; "agent keeps calling the same successful tool" will burn the full iteration budget.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

### Python — function-invocation (per-request tool loop)

| Area | Evidence | File:Line |
|------|----------|-----------|
| Consecutive-error counter default | `DEFAULT_MAX_CONSECUTIVE_ERRORS_PER_REQUEST = 3` | `python/packages/core/agent_framework/_tools.py:91` |
| Per-request iteration cap default | `DEFAULT_MAX_ITERATIONS = 40` | `python/packages/core/agent_framework/_tools.py:90` |
| Counter increments on `had_errors`, resets on success | `_handle_function_call_results` — `if had_errors: errors_in_a_row += 1; ... else: errors_in_a_row = 0` | `python/packages/core/agent_framework/_tools.py:2249-2258` |
| `stop` action returned when threshold reached | `return {"action": "stop" if reached_error_limit else "continue", ...}` | `python/packages/core/agent_framework/_tools.py:2259-2266` |
| Logged warning when consecutive-error limit fires | `logger.warning("Maximum consecutive function call errors reached (%d). ...")` | `python/packages/core/agent_framework/_tools.py:2252-2255` and `2318-2320` |
| `had_errors` definition | `had_errors = any(fcr.exception is not None for fcr in results if fcr.type == "function_result")` | `python/packages/core/agent_framework/_tools.py:1855` |
| `terminate_on_unknown_calls` short-circuits | raises `KeyError` when `function_call.name not in tool_map` (only if config flag set) | `python/packages/core/agent_framework/_tools.py:1719` |
| `max_function_calls` total budget | `max_function_calls: int \| None = ...; ... total_function_calls += approval_result.get("function_call_count", 0)` | `python/packages/core/agent_framework/_tools.py:2558`, `2583-2590` |
| `max_iterations` cap on tool loop | `max_iterations = self.function_invocation_configuration.get("max_iterations", DEFAULT_MAX_ITERATIONS)` | `python/packages/core/agent_framework/_tools.py:2565`, `2671`, `2715`, `2842` |
| `stop` → force `tool_choice="none"` and final non-tool turn | `if result.get("action") == "stop": mutable_options["tool_choice"] = "none"` | `python/packages/core/agent_framework/_tools.py:2633-2636`, `2645-2647`, `2802-2804`, `2818-2820` |
| `tool_choice="required"` reset to avoid infinite loops | "When tool_choice is 'required', reset tool_choice after one iteration to avoid infinite loops" | `python/packages/core/agent_framework/_tools.py:2647-2650`, `2818-2821` |
| Configuration TypedDict (public surface) | `FunctionInvocationConfiguration` with `max_iterations`, `max_function_calls`, `max_consecutive_errors_per_request`, `terminate_on_unknown_calls` | `python/packages/core/agent_framework/_tools.py:1344-1397` |
| Validation of `max_consecutive_errors_per_request >= 0` | `if normalized["max_consecutive_errors_per_request"] < 0: raise ValueError(...)` | `python/packages/core/agent_framework/_tools.py:1419-1421` |
| **Test skipped** in non-streaming mode | `@pytest.mark.skip(reason="Error handling and failsafe behavior needs investigation in unified API")` | `python/packages/core/tests/core/test_function_invocation_logic.py:1489-1491` |
| Streaming test active, asserts ≤2 function calls after threshold | `assert len(function_calls) <= 2` with `max_consecutive_errors_per_request = 2` | `python/packages/core/tests/core/test_function_invocation_logic.py:2934-2985` |
| Harness-level prompt guidance (not detection) | "If a tool call fails or returns unexpected results, adapt your approach rather than repeating the same call." | `python/packages/core/agent_framework/_harness/_agent.py:44-58` |

### Python — `AgentLoopMiddleware` (orchestrator-level re-invocation loop)

| Area | Evidence | File:Line |
|------|----------|-----------|
| General-loop safety cap default | `DEFAULT_MAX_ITERATIONS = 10` | `python/packages/core/agent_framework/_harness/_loop.py:117` |
| Judge-loop safety cap default | `DEFAULT_JUDGE_MAX_ITERATIONS = 5` | `python/packages/core/agent_framework/_harness/_loop.py:122` |
| `max_iterations` checked FIRST, short-circuits `should_continue` | "``max_iterations`` is a safety cap that short-circuits before ``should_continue`` is evaluated" | `python/packages/core/agent_framework/_harness/_loop.py:653-664` |
| Judge verdict markers, deliberate non-overlap so ambiguity → MORE (keep looping) | `JUDGE_VERDICT_DONE = "VERDICT: DONE"`, `JUDGE_VERDICT_MORE = "VERDICT: MORE"` | `python/packages/core/agent_framework/_harness/_loop.py:70-71` |
| `with_judge` factory builds an async `should_continue` predicate | `judge_client.get_response(...)` with `JudgeVerdict` structured output | `python/packages/core/agent_framework/_harness/_loop.py:148-208`, `344-396` |
| Predicate helpers (no built-in pattern detector) | `todos_remaining(provider)` — loops while `TodoProvider` has open items | `python/packages/core/agent_framework/_harness/_loop.py:751-767` |
| Predicate helpers (no built-in pattern detector) | `background_tasks_running(provider)` — loops while persisted tasks are RUNNING | `python/packages/core/agent_framework/_harness/_loop.py:770-796` |
| `fresh_context=True` resets session between iterations to erase history | `snapshot = context.session.to_dict() ...; self._restore_session(...)` | `python/packages/core/agent_framework/_harness/_loop.py:417`, `507-510`, `591-596` |
| Aggregated progress log exposed to callbacks | `_record_progress(...)` writes entries to `progress` list | `python/packages/core/agent_framework/_harness/_loop.py:637-651` |
| Test coverage of the loop middleware | `test_harness_loop.py` exercises max-iterations, judge, fresh-context, feedback log | `python/packages/core/tests/core/test_harness_loop.py` |

### Python — compaction (can erase loop evidence)

| Area | Evidence | File:Line |
|------|----------|-----------|
| `SlidingWindowStrategy(keep_last_groups=N)` | drops older non-system groups | `python/packages/core/agent_framework/_compaction.py:721-765` |
| `ToolResultCompactionStrategy(keep_last_tool_call_groups=N)` | collapses older tool-call groups to `[Tool results: name: result; ...]` summaries | `python/packages/core/agent_framework/_compaction.py:820-895` |
| `TruncationStrategy(max_n=…, compact_to=…)` | oldest-first exclusion by token or message count | `python/packages/core/agent_framework/_compaction.py:650-718` |
| `CompactionProvider.before_strategy` / `after_strategy` | runs before/after model calls; `before_strategy` truncates messages reaching the model | `python/packages/core/agent_framework/_compaction.py:1182-1260` |
| Harness default uses sliding window + tool-result compaction | `before_strategy=SlidingWindowStrategy(keep_last_groups=20)`, `after_strategy=ToolResultCompactionStrategy(keep_last_tool_call_groups=1)` in `create_harness_agent` example | `python/packages/core/agent_framework/_compaction.py:1199-1210`, `_harness/_agent.py:74-110` |
| Compaction erases prior tool errors | "fully excludes old tool-call groups" (`SelectiveToolCallCompactionStrategy`); `ToolResultCompactionStrategy` collapses them into a one-line summary | `python/packages/core/agent_framework/_compaction.py:767-820`, `820-895` |

### Python — workflow graph validation (static, not runtime)

| Area | Evidence | File:Line |
|------|----------|-----------|
| Self-loop edge warning in workflow graph | `_validate_self_loops` — logs `"Self-loop detected: Executor '...' connects to itself. This may cause infinite recursion if not properly handled with conditions."` | `python/packages/core/agent_framework/_workflows/_validation.py:401-413` |
| Graph connectivity validation | Detects unreachable and isolated executors (raised as `GraphConnectivityError`) | `python/packages/core/agent_framework/_workflows/_validation.py:283-359` |

### .NET — Magentic orchestrator (LLM-judged loop + progress detector)

| Area | Evidence | File:Line |
|------|----------|-----------|
| `MagenticProgressLedger.IsInLoopSlot` (LLM-judged) | `"Are we in a loop where we are repeating the same requests and or getting the same responses as before? Loops can span multiple turns, and can include repeated actions like scrolling up or down more than a handful of times."` | `dotnet/src/Microsoft.Agents.AI.Workflows/MagenticProgressLedger.cs:22-24` |
| `IsProgressBeingMadeSlot` (LLM-judged) | `"Are we making forward progress? (True if just starting, or recent messages are adding value. False if recent messages show evidence of being stuck in a loop ...)"` | `dotnet/src/Microsoft.Agents.AI.Workflows/MagenticProgressLedger.cs:26-29` |
| Stall threshold default | `TaskLimits.DefaultMaxStallCount = 3` | `dotnet/src/Microsoft.Agents.AI.Workflows/Specialized/Magentic/MagenticTaskContext.cs:16` |
| Progress-ledger retry default | `DefaultMaxProgressLedgerRetryCount = 3` | `dotnet/src/Microsoft.Agents.AI.Workflows/Specialized/Magentic/MagenticTaskContext.cs:17` |
| `IsStalled` definition | `this.TaskCounters.StallCount > this.TaskLimits.MaxStallCount` | `dotnet/src/Microsoft.Agents.AI.Workflows/Specialized/Magentic/MagenticTaskContext.cs:61` |
| Stall counter increment / decrement on each round | "if (taskContext.ProgressLedger.IsInLoop || !taskContext.ProgressLedger.IsProgressBeingMade) { taskContext.TaskCounters.StallCount++; } else { taskContext.TaskCounters.StallCount = Math.Max(0, taskContext.TaskCounters.StallCount - 1); }" | `dotnet/src/Microsoft.Agents.AI.Workflows/Specialized/Magentic/MagenticOrchestrator.cs:289-298` |
| Intervention when stalled | `ResetAndReplanAsync` — clears `ChatHistory`, increments `ResetCount`, sends `ResetChatSignal` | `dotnet/src/Microsoft.Agents.AI.Workflows/Specialized/Magentic/MagenticOrchestrator.cs:289-300`, `337-344` |
| Hard limits terminate with explicit message | `MaxRoundCount` / `MaxResetCount` → "Task execution stopped due to hitting the maximum {round\|reset} count limit." | `dotnet/src/Microsoft.Agents.AI.Workflows/Specialized/Magentic/MagenticOrchestrator.cs:248-259` |
| Ledger JSON parse retries | `for (int attempts = 0; attempts < maxRetryCount; attempts++) { ... }` with exponential-ish `Task.Delay(250 * attempts)` | `dotnet/src/Microsoft.Agents.AI.Workflows/Specialized/Magentic/MagenticManager.cs:68-110` |
| Stall detection: test asserts reset fires after 1 stall when threshold = 0 | `MaxStallCount_Triggers_ResetAsync` | `dotnet/tests/Microsoft.Agents.AI.Workflows.UnitTests/MagenticOrchestrationTests.cs:615-668` |
| Stall detection: test asserts `IsProgressBeingMade=false` also increments stall count | `Stall_NoProgress_Increments_StallCountAsync` | `dotnet/tests/Microsoft.Agents.AI.Workflows.UnitTests/MagenticOrchestrationTests.cs:952-1000` |
| Stall detection: test asserts progress decrements stall count | `Progress_Made_Decrements_StallCountAsync` | `dotnet/tests/Microsoft.Agents.AI.Workflows.UnitTests/MagenticOrchestrationTests.cs:1065-1132` |
| Stall detection: test asserts 2 consecutive stalls with `MaxStallCount=1` reset | `Consecutive_Stalls_Trigger_ResetAsync` | `dotnet/tests/Microsoft.Agents.AI.Workflows.UnitTests/MagenticOrchestrationTests.cs:1134-1190` |

### .NET — `LoopAgent` (orchestrator-level re-invocation loop)

| Area | Evidence | File:Line |
|------|----------|-----------|
| Hard safety cap default | `LoopAgent.DefaultMaxIterations = 10` | `dotnet/src/Microsoft.Agents.AI/Harness/Loop/LoopAgent.cs:68` |
| Cap evaluated BEFORE evaluators | "Enforce the global safety cap regardless of what the evaluators want" | `dotnet/src/Microsoft.Agents.AI/Harness/Loop/LoopAgent.cs:195-200`, `304-308` |
| Cap behavior on stop | `LogMaxIterationsReached`: `"LoopAgent reached the maximum of {MaxIterations} iterations and stopped."` | `dotnet/src/Microsoft.Agents.AI/Harness/Loop/LoopAgent.cs:487-493` |
| Evaluator abstraction | `LoopEvaluator.EvaluateAsync(context, ct) → LoopEvaluation { ShouldReinvoke, Feedback, Messages }` | `dotnet/src/Microsoft.Agents.AI/Harness/Loop/LoopEvaluator.cs:28-41`, `LoopEvaluation.cs:24-86` |
| `FreshContextPerIteration` resets session (snapshot+restore) | `CreateFreshIterationSessionAsync` deserializes a session snapshot each iteration | `dotnet/src/Microsoft.Agents.AI/Harness/Loop/LoopAgent.cs:43-46`, `150-156`, `500-508` |
| Pending-approval request interrupts loop | `HasPendingApprovalRequests(response)` short-circuits | `dotnet/src/Microsoft.Agents.AI/Harness/Loop/LoopAgent.cs:190-193`, `299-302`, `471-485` |
| Built-in evaluators | `AIJudgeLoopEvaluator` (LLM judge, same non-overlapping markers as Python), `CompletionMarkerLoopEvaluator`, `DelegateLoopEvaluator`, `TodoCompletionLoopEvaluator` | `dotnet/src/Microsoft.Agents.AI/Harness/Loop/AIJudgeLoopEvaluator.cs:44-201`, `CompletionMarkerLoopEvaluator.cs:23-78`, `DelegateLoopEvaluator.cs:20-40`, `TodoCompletionLoopEvaluator.cs` |
| Test: global cap stops always-continue evaluator | `RunAsync_AlwaysContinue_StopsAtGlobalCapAsync` | `dotnet/tests/Microsoft.Agents.AI.UnitTests/Harness/Loop/LoopAgentTests.cs:268-285` |
| Test: pending approval interrupts loop | `RunAsync_PendingApprovalRequest_StopsLoopAsync` | `dotnet/tests/Microsoft.Agents.AI.UnitTests/Harness/Loop/LoopAgentTests.cs:290-305` |

### .NET — function-invocation (per-request tool loop) and compaction

| Area | Evidence | File:Line |
|------|----------|-----------|
| `HarnessAgentOptions.MaximumIterationsPerRequest` wired to `FunctionInvokingChatClient.MaximumIterationsPerRequest` | "passed to `FunctionInvokingChatClient.MaximumIterationsPerRequest`" | `dotnet/src/Microsoft.Agents.AI.Harness/HarnessAgentOptions.cs:177-183` |
| Where it's wired | `.UseFunctionInvocation(loggerFactory, configure: ... ficc => ficc.MaximumIterationsPerRequest = maxIterations)` | `dotnet/src/Microsoft.Agents.AI.Harness/HarnessAgent.cs:228-230` |
| `FunctionInvokingChatClient` itself lives in the external `Microsoft.Extensions.AI` package, not in this repo (no custom loop detector in this source) | `Directory.Packages.props` references `Microsoft.Extensions.AI 10.6.0` | `dotnet/Directory.Packages.props:76-82` |
| Compaction tool-result protection floor | `ToolResultCompactionStrategy.DefaultMinimumPreserved = 16` (older tool groups can be collapsed) | `dotnet/src/Microsoft.Agents.AI/Compaction/ToolResultCompactionStrategy.cs:54` |
| Compaction summarization floor | `SummarizationCompactionStrategy.DefaultMinimumPreserved = 8` | `dotnet/src/Microsoft.Agents.AI/Compaction/SummarizationCompactionStrategy.cs:56` |
| Compaction sliding-window floor | `SlidingWindowCompactionStrategy.DefaultMinimumPreserved = 1` turn | `dotnet/src/Microsoft.Agents.AI/Compaction/SlidingWindowCompactionStrategy.cs:40` |

## Answers to Dimension Questions

1. **What stuck patterns are detected?**
   - **Python tool loop** (`_tools.py`): only *consecutive tool execution errors* — `errors_in_a_row` increments whenever any tool result has a non-None `exception` (`_tools.py:1855`, `2249-2250`); resets to 0 on a successful iteration (`_tools.py:2258`); stops when `errors_in_a_row >= max_consecutive_errors_per_request` (default 3). No detection of repeated-but-successful calls, alternating patterns, or "monologue" no-progress.
   - **Python orchestrator loop** (`_harness/_loop.py`): no built-in pattern detector; the user-supplied `should_continue` (or the LLM judge's `JudgeVerdict.answered`) decides. `todos_remaining(provider)` and `background_tasks_running(provider)` are the only shipped helpers (`_loop.py:751-796`).
   - **.NET Magentic**: `IsInLoop` (LLM-judged) + `IsProgressBeingMade` (LLM-judged) — both true/false fields the manager fills in each round (`MagenticProgressLedger.cs:22-29`). The ledger asks "Are we in a loop where we are repeating the same requests and or getting the same responses as before? Loops can span multiple turns, and can include repeated actions like scrolling up or down more than a handful of times." This is the most explicit "doom loop" detection in the codebase.
   - **.NET LoopAgent**: no built-in pattern detector beyond the `max_iterations` safety cap; pattern detection is delegated to evaluators.

2. **How far back does detection look?**
   - Python tool-loop counter: the full current request's worth of iterations; there is no sliding window — `errors_in_a_row` resets on a single clean iteration, so a single successful call after a string of errors wipes the slate (`_tools.py:2258`).
   - .NET Magentic: each round looks at the *current* ledger evaluation (`MagenticOrchestrator.cs:289-298`) — the LLM is given the full `ChatHistory` so its horizon is essentially the full history; the numeric state is `StallCount` which decays by 1 per round of progress (`MagenticOrchestrator.cs:296`).
   - .NET LoopAgent: per-iteration only — `LoopContext.LastResponse` and `LoopContext.Feedback` expose the most recent iteration plus the full feedback log (`LoopContext.cs:70-87`).
   - No implementation has a fixed-length "look-back window" of N previous turns with a fingerprint comparison (the dimension's classic sliding-window detector).

3. **Does compaction erase loop evidence?**
   - Yes, deliberately. Python: `SlidingWindowStrategy(keep_last_groups=N)` excludes older groups (`_compaction.py:721-765`); `ToolResultCompactionStrategy(keep_last_tool_call_groups=N)` collapses older tool-call groups into one-line `[Tool results: …]` summaries (`_compaction.py:820-895`); the `create_harness_agent` example defaults to `keep_last_groups=20` for the before-phase and `keep_last_tool_call_groups=1` for the after-phase (`_compaction.py:1204-1210`).
   - .NET: `ToolResultCompactionStrategy.DefaultMinimumPreserved = 16` (`dotnet/src/Microsoft.Agents.AI/Compaction/ToolResultCompactionStrategy.cs:54`), `SummarizationCompactionStrategy.DefaultMinimumPreserved = 8` (`…/SummarizationCompactionStrategy.cs:56`), `SlidingWindowCompactionStrategy.DefaultMinimumPreserved = 1` turn (`…/SlidingWindowCompactionStrategy.cs:40`).
   - Consequence: an LLM judge that tries to find "same tool called 5 turns ago" will see that tool call collapsed into a one-line summary or dropped entirely once the budget is exceeded. The most recent N turns are preserved; everything older is rewritten or removed.
   - Counterbalancing: `fresh_context=True` on Python `AgentLoopMiddleware` (`_harness/_loop.py:417`) and `LoopAgentOptions.FreshContextPerIteration` on .NET `LoopAgent` (`LoopAgent.cs:43-46`) reset the *session* (caller-supplied session is restored from a snapshot, loop-owned session is recreated), so the next iteration does not see the prior conversation at all — continuity is carried only by the feedback/progress log.

4. **What intervention happens?**
   - Python tool loop on threshold: `tool_choice = "none"` is set and a final non-tool call is made so the model must produce a plain text answer (`_tools.py:2633-2636`, `2645-2647`, `2802-2804`, `2818-2820`); the function-call history is still flushed so no orphaned `function_call` items remain.
   - Python `AgentLoopMiddleware` on `max_iterations`: returns the last `AgentResponse` and (when `fresh_context=True`) restores the session snapshot for the next iteration (`_loop.py:653-664`); the judge's "reasoning" is fed back to the agent as the next iteration's input (`_loop.py:148-208`).
   - .NET Magentic on stall: `ResetAndReplanAsync` clears `ChatHistory`, increments `ResetCount`, sends `ResetChatSignal`, then `UpdatePlanAndDelegateAsync(..., replanAfterStall: true)` regenerates the plan and picks the next speaker (`MagenticOrchestrator.cs:337-344`). This is a hard **replan**, not just a stop.
   - .NET Magentic on round/reset limit: yields an explicit "Task execution stopped due to hitting the maximum round/reset count limit" message and terminates (`MagenticOrchestrator.cs:252-256`).
   - .NET LoopAgent on cap: logs `"LoopAgent reached the maximum of {MaxIterations} iterations and stopped."` and returns the latest response (`LoopAgent.cs:487-493`).
   - .NET `FunctionInvokingChatClient.MaximumIterationsPerRequest` (set by HarnessAgent) terminates the per-request tool loop; its specific stop behavior is in the external Microsoft.Extensions.AI package and is not visible in this source.

5. **Are false positives possible?**
   - **Python consecutive-error counter**: yes — it requires *every* result in the iteration to succeed to reset. A batch of 4 parallel tool calls where one legitimately raises an exception will trip the counter (`_tools.py:1855`, `2249-2250`). Threshold = 0 is even supported (`_tools.py:1421` validation allows `>= 0`).
   - **Python tool-loop `max_iterations = 40`**: a long, legitimate multi-tool workflow can hit it before completing.
   - **.NET Magentic LLM judge**: probabilistic — a single ambiguous ledger response can flip `IsInLoop` or `IsProgressBeingMade` and push `StallCount` over the threshold. The `MagenticManager` retries up to 3 times to parse valid JSON (`MagenticManager.cs:71-110`).
   - **.NET LoopAgent global cap**: false positives only if the evaluator set is wrong; by design it's a hard cap, not a detector.
   - **Compaction**: collapsing older tool results is silent (no warning), so a model that needed the full prior trace loses it without any signal that something was dropped.

## Architectural Decisions

- **Two distinct layers, deliberately separated.** Per-request tool-call loops (`FunctionInvocationLayer` Python / `FunctionInvokingChatClient` .NET) are decoupled from per-orchestrator re-invocation loops (`AgentLoopMiddleware` Python / `LoopAgent` + Magentic .NET). Stuck/loop detection lives mostly in the orchestrator layer; the per-request tool loop only has the cheap "consecutive errors" counter.
- **No structural detector at the tool-loop layer.** Neither Python nor .NET fingerprints tool calls or compares arguments across iterations. Detection is left to the orchestrator layer or to user-supplied evaluators. This is an explicit design choice — see the dimension rationale: `loop_evidence` retention is owned by `LoopContext.Feedback` (.NET) and `progress` (Python), which carry *feedback strings*, not raw tool calls.
- **Stall-counter with decay, not absolute count.** `MagenticTaskContext.StallCount` decrements on each non-stall round (`MagenticOrchestrator.cs:296`). This avoids permanent bad state from one noisy round.
- **LLM-as-judge for "is in loop"** instead of an algorithmic fingerprint. Magentic explicitly asks the manager model "Are we in a loop where we are repeating the same requests and or getting the same responses as before?" (`MagenticProgressLedger.cs:22-24`). Tradeoff: catches soft patterns (semantic repetition) but introduces latency, cost, and judge-model failures.
- **Hard caps before soft signals.** Both languages evaluate `max_iterations` BEFORE the `should_continue` predicate / evaluator set: Python `_loop.py:653-664`, .NET `LoopAgent.cs:195-200`, `304-308`. This avoids an expensive judge call once the cap has fired.
- **Reset-and-replan, not just abort, on Magentic stall.** `ResetAndReplanAsync` sends a `ResetChatSignal` and rebuilds the plan (`MagenticOrchestrator.cs:337-344`). The plan-review event exposes `IsStalled` to downstream consumers (`MagenticPlanReviewRequest.cs:18`), so observers can tell whether a re-plan was stall-triggered (test asserts `reviewRequest2.IsStalled.Should().BeTrue()` — `MagenticOrchestrationTests.cs:792`).
- **Compaction is the explicit contract** that erases loop evidence. The before/after strategies and the floor defaults are public API (`_compaction.py:1182-1260`, `…/Compaction/ToolResultCompactionStrategy.cs:72-79`). There is no "preserve loop evidence" mode.

## Notable Patterns

- **Reset on first success** — `errors_in_a_row = 0` on the first error-free iteration (`_tools.py:2258`). Simple, but a single intermittent failure resets progress.
- **Forced final text turn** — when the tool loop hits a stop condition, the framework sets `tool_choice = "none"` and makes one more model call so the model must produce plain text instead of leaving orphaned `function_call` items (`_tools.py:2645-2647`, `2818-2820`; `LoopAgent.cs:196-200`).
- **Snapshot+restore session** — `fresh_context` mode takes `to_dict()` once before the loop and rebuilds a fresh session between iterations (`_loop.py:417`, `507-510`; `LoopAgent.cs:150-156`, `500-508`).
- **Verdict markers, deliberately non-overlapping** — `JUDGE_VERDICT_DONE` / `JUDGE_VERDICT_MORE` (Python `_loop.py:70-71`) and `DoneVerdictMarker` / `MoreVerdictMarker` (.NET `AIJudgeLoopEvaluator.cs:70-76`) are designed so neither is a substring of the other, and "MORE" wins on ambiguity so an incomplete answer keeps looping.
- **Aggregated feedback log** — `LoopContext.Feedback` (.NET `LoopContext.cs:77-87`) and `progress` (Python `_loop.py:447-504`) are exposed to every callback so evaluators can reason over prior iterations; the loop owns the log so evaluators stay stateless.
- **Self-loop warning in workflow graphs** — `_validate_self_loops` warns at build time when an executor edges to itself (`_workflows/_validation.py:401-413`). This is static graph analysis, not runtime agent loop detection.

## Tradeoffs

- **Latency vs. detection quality.** Magentic's LLM-judged "IsInLoop" adds an extra manager call per round. Cheaper than running more agent turns, but still expensive.
- **Coverage vs. false positives.** A structural detector (hash of `(tool_name, arguments)` over the last K iterations) would catch repeated successful calls with zero ambiguity, but the framework deliberately leaves this to LLM judges or user code — so "agent keeps calling the same successful tool" goes undetected at the tool-loop layer.
- **Compaction vs. forensic visibility.** Default `ToolResultCompactionStrategy.keep_last_tool_call_groups=1` collapses everything except the current call. If the loop detector wants to look back, the trace is gone.
- **Cap short-circuits the judge.** Python's `_evaluate_stop` evaluates `max_iterations` first (`_loop.py:653-664`), so once the cap fires, the expensive judge is never called. Good for cost, bad if the judge would have answered "DONE" — the loop still runs to the cap unless `should_continue` is False.
- **Hardcoded Python default.** `DEFAULT_MAX_CONSECUTIVE_ERRORS_PER_REQUEST = 3` (`_tools.py:91`) and `DEFAULT_MAX_ITERATIONS = 40` (`_tools.py:90`) are module-level constants. They can be overridden per-client via `function_invocation_configuration`, but there is no env-var or runtime knob — the framework trusts the configured values.
- **Skipped Python test.** `test_function_invocation_config_max_consecutive_errors` is `@pytest.mark.skip(...)` (`test_function_invocation_logic.py:1489`), so the non-streaming path's behavior on threshold is not actively regression-tested. The streaming path is.

## Failure Modes / Edge Cases

- **Repeated successful tool call** — neither Python nor .NET catches it at the tool-loop layer. An agent can call `get_weather("Seattle")` 40 times in a row and only `max_iterations` will stop it.
- **Intermittent failures** — Python's `errors_in_a_row` resets on any error-free iteration (`_tools.py:2258`), so a 30-iteration alternating pattern (error, success, error, success, ...) never reaches the threshold. The agent pays for the full `max_iterations` budget.
- **Compaction erases prior errors** — once a tool group is collapsed to `[Tool results: ...]`, the model can no longer see the previous exception message. The next iteration may retry a tool that the model now believes succeeded.
- **Judge ambiguity** — Python and .NET both default to "MORE wins" on ambiguous verdict text (`_loop.py:193`, `AIJudgeLoopEvaluator.cs:163`), so an unparseable judge reply causes the loop to keep running rather than risk stopping on a false positive. Combined with the hard cap, this means judges with frequent parse failures can drain the iteration budget.
- **Session-snapshot restoration** — `LoopAgent.CreateFreshIterationSessionAsync` deserializes a fresh clone (`LoopAgent.cs:500-508`). For `LoopAgentOptions.cs:46-49`, the Conversations service stores messages in a single threaded list, so "cloning" does not erase history in that storage mode — the comment explicitly calls this out.
- **Magentic ledger parse failure path** — `MagenticManager.UpdateProgressLedgerAsync` retries up to `MaxProgressLedgerRetryCount=3` then throws; the orchestrator catches non-`OperationCanceledException`, emits a `WorkflowWarningEvent`, and triggers `ResetAndReplanAsync` (`MagenticOrchestrator.cs:274-279`). This means a misbehaving manager can churn the replan budget.
- **Function call within a function call** — `tool_choice = "required"` is reset to default after one iteration to prevent infinite loops (`_tools.py:2647-2650`, `2818-2821`). However, this only resets the option; the model could still loop if its prompt asks for it.

## Future Considerations

- **Structural detector at the tool-loop layer.** A simple `(tool_name, json.dumps(arguments))` fingerprint over the last N iterations, with a configurable threshold, would catch repeated-successful-call doom loops that no current detector handles. The harness already has a natural place for it: a middleware in `AgentLoopMiddleware.process` (Python) or a custom `LoopEvaluator` (.NET).
- **Promote the skipped Python test.** `test_function_invocation_config_max_consecutive_errors` is the primary behavior test for the consecutive-error counter and is currently skipped. Re-enable it (the streaming counterpart at `test_function_invocation_logic.py:2934` already validates the same logic).
- **Configurable consecutive-error threshold via env.** `DEFAULT_MAX_CONSECUTIVE_ERRORS_PER_REQUEST = 3` is a module constant. Promoting it to environment-tunable would let operators adjust without code changes.
- **Cross-check on compaction.** When `ToolResultCompactionStrategy` collapses an older tool group that contained a failure (exception message visible in the trace), surface that to the loop detector so it can re-raise the same `errors_in_a_row` signal instead of treating the loss as a successful iteration.
- **Magentic structural-mode fallback.** When the LLM judge fails to parse (`MagenticManager.UpdateProgressLedgerAsync` retries exhausted), fall back to a structural fingerprint on the most recent N manager outputs before triggering reset-and-replan.
- **Observability.** Loop terminations due to cap or stall are logged at INFO (`LoopAgent.cs:489-491`, `_loop.py:117-122`); the consecutive-error stop is logged at WARNING (`_tools.py:2252-2255`). There is no metric/spans emission; downstream observability must scrape the logger.

## Questions / Gaps

- **`FunctionInvokingChatClient.MaximumIterationsPerRequest`** — referenced by `HarnessAgentOptions` (`dotnet/src/Microsoft.Agents.AI.Harness/HarnessAgentOptions.cs:177-183`) but its implementation lives in the external `Microsoft.Extensions.AI` NuGet package, not in this source. Whether it has a consecutive-error or stuck detector inside it cannot be verified from this repository alone.
- **`use_function_invocation`** — the AGENTS.md mentions a `use_function_invocation()` decorator, but no such symbol exists in the Python source. The current abstraction is `FunctionInvocationLayer` (`_tools.py:2388`). Documentation may be out of date.
- **No canonical "stuck detector" class.** Detection is split across `_handle_function_call_results` (Python), `MagenticProgressLedger` (Magentic), and `LoopAgent` (orchestrator). There is no shared base or interface, and the two implementations do not share code or contracts.
- **No metric/Span export for loop-stop events.** The framework logs them but does not appear to expose them as OTel spans or counters (no `OtelAttr.*` constant found for loop-stop reasons). A user wanting to alert on "agent hit cap" has to scrape logs.
- **Skipped test for the primary Python behavior.** `test_function_invocation_config_max_consecutive_errors` at `test_function_invocation_logic.py:1489` is explicitly skipped, leaving the non-streaming threshold path unverified at the test layer.
- **No `alternating` / `no-progress-monologue` detector anywhere.** Both dimensions are absent at every layer; only "consecutive tool errors" and the Magentic LLM-judged "IsInLoop" / "IsProgressBeingMade" exist.

---

Generated by `dimension-03.06-stuck-and-doom-loop-detection` against `agent-framework`.