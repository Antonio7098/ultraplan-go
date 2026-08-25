# Source Analysis: agent-framework

## Dimension 15.02: Message Routing and Termination

### Source Info

| Field | Value |
|-------|-------|
| Name | agent-framework (microsoft/agent-framework) |
| Path | `studies/agent-harness-study/sources/agent-framework` |
| Language / Stack | C# (.NET, `dotnet/src/Microsoft.Agents.AI.Workflows`) and Python (`python/packages/core/agent_framework/_workflows`, `python/packages/orchestrations`); `go/` contains only a README placeholder (no Go implementation) |
| Analyzed | 2026-08-25 |

All file citations below are relative to the source root `studies/agent-harness-study/sources/agent-framework/`.

## Summary

Message routing in agent-framework is layered. At the bottom sits a Pregel-like superstep engine: executors are graph nodes connected by typed edges, and a run converges when no messages remain in flight (Python: `python/packages/core/agent_framework/_workflows/_runner.py:106-177`; .NET: `dotnet/src/Microsoft.Agents.AI.Workflows/InProc/InProcessRunner.cs:212-241`). On top of that, three multi-agent routing patterns are implemented on both stacks:

1. **Group chat (centralized)** — an orchestrator/manager owns the canonical conversation, broadcasts deltas to all participants except the last speaker, and grants the "right to speak" via a `TurnToken` message or targeted request envelope (`.NET`: `dotnet/src/Microsoft.Agents.AI.Workflows/Specialized/GroupChatHost.cs:45-110`; Python: `python/packages/orchestrations/agent_framework_orchestrations/_base_group_chat_orchestrator.py:411-495`).
2. **Handoff (decentralized)** — agents route control themselves by calling synthetic `handoff_to_<target>` tools; Python intercepts them with function middleware and dispatches a targeted message over a fully connected fan-out mesh (`_handoff.py:124-154, 397-419`); .NET models each hop as a predicate switch on a `HandoffState` record with a default branch to an end executor (`HandoffWorkflowBuilder.cs:473-488`, `Specialized/HandoffState.cs:5-9`).
3. **Magentic (ledger-driven)** — an LLM manager produces a structured progress ledger whose `next_speaker`, `is_request_satisfied`, `is_progress_being_made`, and `is_in_loop` fields jointly drive speaker selection, stall detection, reset/replan, and termination (Python: `_magentic.py:1086-1154`; .NET: `Specialized/Magentic/MagenticOrchestrator.cs:248-337`).

Termination is defense-in-depth rather than a single mechanism: pluggable user predicates, numeric caps (rounds/iterations/turn limits), LLM-decided completion flags, human-in-the-loop gates, same-speaker re-selection guards, and runner-level convergence checks. Deadlock prevention is primarily static (build-time graph validation) plus bounded loops; there is **no runtime deadlock detector**.

**Can a multi-agent conversation terminate without human intervention?** Yes, by default in most configurations: group chat terminates at its iteration cap (default 40, `GroupChatManager.cs:41-49`) or via a custom predicate; handoff workflows pause for user input only when autonomous mode is off (that pause *is* the default HIL contract, `_handoff.py:427-439`), but with `with_autonomous_mode()` they terminate at the turn limit (default 50) or when a termination condition fires; Magentic terminates when the ledger reports task satisfaction or limits are hit. The one documented unbounded case: a Python `GroupChatOrchestrator` created with neither `max_rounds` nor `termination_condition` runs indefinitely by design (`_group_chat.py:140-143`).

## Rating

**8 / 10**

Rationale against the rubric:

- Clear model with explicit interfaces: `GroupChatManager.SelectNextAgentAsync`/`ShouldTerminateAsync` (`GroupChatManager.cs:58-94`), `GroupChatSelectionFunction` (`_group_chat.py:95`), `TerminationCondition` type alias (`_base_group_chat_orchestrator.py:59`), `SwitchBuilder` case/default routing (`SwitchBuilder.cs:33-89`).
- Extensive test coverage of routing and termination behavior on both stacks: round-robin cycling/wrap-around/custom early termination (`dotnet/tests/Microsoft.Agents.AI.Workflows.UnitTests/RoundRobinGroupChatManagerTests.cs:14-99`), return-to-previous and autonomous-mode termination tests (`HandoffOrchestrationTests.cs:849-952, 1325-1506`), Python max-rounds/termination-condition tests (`python/packages/orchestrations/tests/test_group_chat.py:383-497`), and Magentic stall/reset-limit tests (`test_magentic.py:820-838`).
- Operational safeguards: checkpointed routing cursors and speaker identity for resumable runs (`RoundRobinGroupChatManager.cs:74-86`, `GroupChatHost.cs:133-158`, `MagenticOrchestrator.cs:357-404`), build-time unreachable/isolated executor validation (`WorkflowBuilder.cs:609-613`; `_validation.py:289-334`), bounded convergence loop with `WorkflowConvergenceException` after 100 supersteps (`_runner.py:176-177`, `_const.py:4`).

It falls short of 9–10 because: (a) there is no runtime liveness/deadlock detector — a misconfigured workflow can stall silently until external intervention (e.g., pending request-info) or run forever in the documented unbounded Python group-chat configuration; (b) some failure paths only warn (duplicate handoff tool calls resolved to the last call, `HandoffAgentExecutor.cs:457-461`; agents with no handoff targets warn "your workflow may get stuck", `_handoff.py:1095-1099`); (c) semantics are duplicated across two independent stacks and already diverge in edge cases (see Failure Modes); and (d) Python autonomous mode uses recursion (`_run_agent_and_emit` calls itself, `_handoff.py:428-435`) which bounds depth only by the turn limit rather than iterating.

## Evidence Collected

Every entry cites file paths with line numbers relative to the source root.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Core convergence loop (Python) | `run_until_convergence` runs supersteps until `has_messages()` is false; raises `WorkflowConvergenceException` past `max_iterations` | `python/packages/core/agent_framework/_workflows/_runner.py:106-177` |
| Default superstep cap | `DEFAULT_MAX_ITERATIONS = 100` | `python/packages/core/agent_framework/_workflows/_const.py:4` |
| Core superstep runner (.NET) | `RunSuperStepAsync` returns `false` when no queued messages, external deliveries, or joined-runner actions — the run ends | `dotnet/src/Microsoft.Agents.AI.Workflows/InProc/InProcessRunner.cs:212-241` |
| Message delivery | Per-receiver delivery loops with per-edge order preservation documented | `InProcessRunner.cs:243-303`; `_runner.py:179-209` |
| Speak-permission token | `TurnToken(emitEvents)` sent to exactly one participant executor to request a response | `dotnet/src/Microsoft.Agents.AI.Workflows/TurnToken.cs:12-19`; used at `GroupChatHost.cs:87-89` |
| Manager contract | Abstract `SelectNextAgentAsync`, virtual `UpdateHistoryAsync` broadcast filter, virtual `ShouldTerminateAsync`, `Reset()`, checkpoint hooks | `dotnet/src/Microsoft.Agents.AI.Workflows/GroupChatManager.cs:58-164` |
| Iteration cap default 40 | `MaximumIterationCount` getter/setter with `Throw.IfLessThan(value, 1)` | `GroupChatManager.cs:41-49` |
| Round-robin selection | Index modulo agent count; cursor persisted/restored via state key `next_index` with out-of-range repair | `dotnet/src/Microsoft.Agents.AI.Workflows/RoundRobinGroupChatManager.cs:44-52, 74-86` |
| Group chat host loop | Terminate check → broadcast delta (excluding current speaker) → select next → send TurnToken; re-selecting current speaker treated as termination; `CompleteAsync` yields history and resets | `dotnet/src/Microsoft.Agents.AI.Workflows/Specialized/GroupChatHost.cs:45-120` |
| Star topology wiring | Bidirectional edges between `GroupChatHost` and every participant; participants configured not to echo messages back | `dotnet/src/Microsoft.Agents.AI.Workflows/GroupChatWorkflowBuilder.cs:54-79` |
| Python orchestrator routing helpers | `_broadcast_messages_to_participants` (fan-out with exclusion list) and `_send_request_to_participant` (dual envelope: `AgentExecutorRequest` for agents vs `GroupChatRequestMessage` for custom executors) | `python/packages/orchestrations/agent_framework_orchestrations/_base_group_chat_orchestrator.py:411-495` |
| Speaker selection function | `GroupChatState(current_round, participants, conversation)` passed to pluggable selection func; unknown name raises `RuntimeError` | `python/packages/orchestrations/agent_framework_orchestrations/_group_chat.py:73-95, 239-254` |
| Agent-driven selection + termination | Structured output `AgentOrchestrationOutput{terminate, reason, next_speaker, final_message}`; defensive parsing fallbacks; retry loop feeding parse errors back to the model | `_group_chat.py:262-281, 420-487, 526-542` |
| Handoff tool naming & mesh default | `handoff_to_{target_id}` naming; when no explicit handoffs, every agent can hand off to every other (mesh) | `python/packages/orchestrations/agent_framework_orchestrations/_handoff.py:124-126, 1061-1074` |
| Handoff interception (Python) | `_AutoHandoffMiddleware` short-circuits handoff tool execution with synthetic result `{handoff_to: target}` via `MiddlewareTermination` | `_handoff.py:132-154` |
| Handoff dispatch (Python) | Validates target against registered set (`ValueError` on unknown), queues tool-result message, sends empty `AgentExecutorRequest(should_respond=True)` to target, emits `handoff_sent` event | `_handoff.py:397-419` |
| Parallel-handoff prevention (Python) | Cloned agents forced to `allow_multiple_tool_calls = False` so at most one handoff tool fires per response | `_handoff.py:287-289` |
| Handoff functions (.NET) | Declaration-only `AIFunction`s named `{FunctionPrefix}{index}` (`handoff_to_*`) with reason descriptions; mapped tool-name→agent-id | `dotnet/src/Microsoft.Agents.AI.Workflows/Specialized/HandoffAgentExecutor.cs:87-147`; prefix at `HandoffWorkflowBuilder.cs:42` |
| Duplicate handoff handling (.NET) | >1 handoff call in one turn emits `WorkflowWarningEvent` and uses the last call | `HandoffAgentExecutor.cs:457-465` |
| Routing switch per agent (.NET) | Each agent executor gets a switch: cases match `RequestedHandoffTargetAgentId == target && !IsTerminated`, default routes to `HandoffEndExecutor` | `dotnet/src/Microsoft.Agents.AI.Workflows/HandoffWorkflowBuilder.cs:473-488` |
| Return-to-previous routing | Start executor switch routes subsequent turns directly to the last specialist (`PreviousAgentId` cases, initial agent as default); disabled by default | `HandoffWorkflowBuilder.cs:575-591`; state capture at `Specialized/HandoffStartExecutor.cs:82-88` |
| Autonomous loop-back switch | Switch downstream of End routes synthetic self-handoff `HandoffState` back to matching agent | `HandoffWorkflowBuilder.cs:597-610`; emission at `Specialized/HandoffEndExecutor.cs:119-127` |
| Autonomous turn limit (.NET) | Per-agent counters in shared state; limit default 50; counter resets on handoff, on terminal path, and at each fresh user turn | `HandoffEndExecutor.cs:92-139`; `HandoffStartExecutor.cs:78-80`; `HandoffAgentExecutor.cs:300-303`; defaults at `HandoffAgentExecutor.cs:40-44` |
| Autonomous continuation (Python) | Recursive `_run_agent_and_emit` guarded by `_autonomous_mode_turns < _autonomous_mode_turn_limit` (default 50); counter serialized in checkpoints | `_handoff.py:196-197, 427-435, 549-561` |
| Termination conditions (both stacks) | Sync/async predicates over conversation history: `WithTerminationCondition` (.NET), `with_termination_condition` / `TerminationCondition` alias (Python); evaluated pre-run and post-response | `HandoffWorkflowBuilder.cs:415-443`; `_base_group_chat_orchestrator.py:59, 338-372`; `_handoff.py:365-367, 421-425, 534-547` |
| Max rounds enforcement (Python) | `_check_round_limit_and_yield` coerces `max_rounds >= 1`, logs warning, yields synthesized "maximum number of rounds" completion message | `_base_group_chat_orchestrator.py:160-165, 499-538` |
| Documented unbounded config | Docstring warns that neither `max_rounds` nor `termination_condition` means the conversation continues indefinitely | `python/packages/orchestrations/agent_framework_orchestrations/_group_chat.py:142-143` |
| Magentic ledger routing (Python) | Progress ledger drives: satisfaction → final answer; stall/loop counters with decrement on progress; invalid next speaker → finalize; instruction targeted at chosen speaker only | `_magentic.py:1086-1154` |
| Magentic limits (Python) | `_check_within_limits_or_complete` yields "terminated due to reaching maximum {round\|reset} count" and sets `_terminated`; post-termination invocations raise `RuntimeError` | `_magentic.py:921-925, 1229-1265` |
| Magentic coordination (.NET) | Same loop: `CheckLimits` → ledger update (retrying) → satisfied → final answer → stall check → delegate `TurnToken` to next speaker | `Specialized/Magentic/MagenticOrchestrator.cs:186-337` |
| Magentic limits data | `TaskLimits(MaxStallCount=3 default, MaxRoundCount, MaxResetCount, MaxProgressLedgerRetryCount=3)`; `CheckLimits()`; `Reset()` bumps ResetCount and clears stall | `Specialized/Magentic/MagenticTaskContext.cs:11-18, 76-82, 104-109` |
| Ledger parse resilience | Bounded retries with linear backoff (0.25s × attempt) on JSON parse failure, both stacks | `Specialized/Magentic/MagenticManager.cs:66-105`; `_magentic.py:720-739` |
| Broadcast guard after restore | Reply broadcast skipped if current-speaker id unknown (post-checkpoint-restore window) to avoid echoing to author | `Specialized/Magentic/MagenticOrchestrator.cs:218-240` |
| Build-time validation (.NET) | `InvalidOperationException` listing unreachable executors at build | `dotnet/src/Microsoft.Agents.AI.Workflows/WorkflowBuilder.cs:609-613` |
| Build-time validation (Python) | `GraphConnectivityError` for unreachable/isolated executors; warnings for self-loops ("may cause infinite recursion") and dead ends | `python/packages/core/agent_framework/_workflows/_validation.py:157-163, 289-334, 401-429` |
| Switch primitive | Predicate cases evaluated in insertion order with default fallback; reduced to a fan-out edge with an `EdgeSelector` | `dotnet/src/Microsoft.Agents.AI.Workflows/SwitchBuilder.cs:33-113` |
| Checkpointed routing state | Host persists history + current speaker; manager persists iteration count and subclass state under prefixed keys | `GroupChatHost.cs:133-158`; `GroupChatManager.cs:148-164` |
| Halt escape hatch | `IWorkflowContext.RequestHaltAsync` exposed to executors/managers (delegated through the state-prefixing context decorator) | `GroupChatManager.cs:190`; `IWorkflowContext.cs` |
| Tests (.NET, RR) | Cycling, wrap-around, default cap termination, custom-predicate early termination | `dotnet/tests/Microsoft.Agents.AI.Workflows.UnitTests/RoundRobinGroupChatManagerTests.cs:14-99` |
| Tests (.NET, handoff) | ~30 scenario tests incl. `ReturnToPrevious_*` routing matrix (lines 849–952), `AutonomousMode_RespectsTurnLimitAsync` (1370), `SyncTerminationCondition_EndsAutonomousLoopAsync` (1444), `TerminationCondition_NotInvokedOnHandoffAsync` (1506) | `dotnet/tests/Microsoft.Agents.AI.Workflows.UnitTests/HandoffOrchestrationTests.cs` |
| Tests (.NET, group chat) | Iteration-cap-bounded conversations with approval/tool flows | `dotnet/tests/Microsoft.Agents.AI.Workflows.UnitTests/GroupChatOrchestrationTests.cs:41-231` |
| Tests (Python, group chat) | `test_max_rounds_enforcement`, `test_termination_condition_halts_conversation`, streaming max-rounds yield | `python/packages/orchestrations/tests/test_group_chat.py:383-497` |
| Tests (Python, magentic) | `test_magentic_stall_and_reset_reach_limits` asserts "terminated due to reaching maximum reset count"; round-limit partial result test | `python/packages/orchestrations/tests/test_magentic.py:820-838, 400-419` |

## Answers to Dimension Questions

### 1. How are messages routed?

Three layers cooperate:

- **Graph layer**: messages flow along typed edges between executors in supersteps. Python delivers via edge runners with per-edge ordering guarantees (`_runner.py:179-209`); .NET dequeues per-receiver envelope queues each step (`InProcessRunner.cs:243-336`). Handler selection inside an executor is type-based (`IMessageRouter.cs:10-17`).
- **Orchestration layer**: each pattern imposes a topology. Group chat is a star: the host/orchestrator holds the canonical history, broadcasts deltas to everyone except the author, and unicasts a speak-grant (`TurnToken` in .NET at `GroupChatHost.cs:62-110`; targeted `AgentExecutorRequest` in Python at `_base_group_chat_orchestrator.py:443-495`). Handoff is a mesh of edges (Python wires full fan-out between all executors, `_handoff.py:998-1010`) with actual control transfer decided by tool-call interception; .NET instead builds explicit per-agent predicate switches over `HandoffState` (`HandoffWorkflowBuilder.cs:473-488`). Magentic routes point-to-point from the manager to whichever participant the ledger names (`MagenticOrchestrator.cs:306-337`).
- **Primitive layer**: conditional routing reduces to declarative constructs — `SwitchBuilder` cases/default compiled into a fan-out edge with an edge selector (`SwitchBuilder.cs:91-113`), and Python builders expose `add_switch_case`/fan-out/fan-in edges (`test_validation.py`, `_edge.py`).

### 2. How is the next speaker selected?

Four strategies exist:

1. **Deterministic rotation**: `RoundRobinGroupChatManager.SelectNextAgentAsync` cycles an index modulo the participant list (`RoundRobinGroupChatManager.cs:44-52`), with the cursor checkpointed and repaired if out of range after restore (`RoundRobinGroupChatManager.cs:78-86`).
2. **Pluggable function**: Python's `GroupChatOrchestrator` calls a user-supplied `selection_func(GroupChatState)`; returning an unknown participant raises immediately (`_group_chat.py:239-254`).
3. **LLM judge**: `AgentBasedGroupChatOrchestrator` asks an agent for structured output choosing `terminate` vs `next_speaker` (`_group_chat.py:262-281`), with defensive parsing (structured value → strict JSON → concatenated-JSON fallback) and retries that feed the parse error back to the model (`_group_chat.py:419-542`).
4. **Progress-ledger election**: Magentic's manager answers five questions (`is_request_satisfied`, `is_progress_being_made`, `is_in_loop`, `next_speaker`, `instruction_or_question`); non-string answers fall back to the first participant, unknown names terminate with a final answer (`_magentic.py:1111-1154`; .NET mirror at `MagenticOrchestrator.cs:283-322`).

### 3. How are handoffs managed?

- **Contract**: a handoff is a model tool call named `handoff_to_<target>` (Python: `_handoff.py:124-126`; .NET: `handoff_to_{index}`, `HandoffWorkflowBuilder.cs:42`, `HandoffAgentExecutor.cs:133-143`) with a description derived from the target's description/name/instructions; registration requires at least one of these or construction fails (`HandoffWorkflowBuilder.cs:232-245`).
- **Mechanism**: Python injects no-op tools plus `_AutoHandoffMiddleware`, which short-circuits execution with a synthetic `{handoff_to: target}` result and prevents parallel tool calls so at most one handoff happens per response (`_handoff.py:145-154, 287-289`). The executor validates the target, preserves the tool-result message for the agent's own history, then sends an empty respond-request to the target and emits a `handoff_sent` event (`_handoff.py:397-419`). .NET detects candidate calls while streaming, synthesizes the `FunctionResultContent("Transferred.")`, forbids handoffs while external requests are pending, and stamps the decision into an outgoing `HandoffState` (`HandoffAgentExecutor.cs:333-345, 445-506`) consumed by the routing switches (`HandoffState.cs:5-9`, `HandoffWorkflowBuilder.cs:473-488`).
- **Hygiene**: internal handoff tool-call/result traffic is filtered from what the next agent sees so it doesn't derail the target model (`HandoffAgentExecutor.cs:241-251` with `HandoffMessagesFilter`), and shared conversation state uses bookmarks so re-entrant invocations replay only new messages (`HandoffAgentExecutor.cs:262-308, 363-376`).
- **Topology options**: explicit edges via `WithHandoff`/`add_handoff`, or a default all-to-all mesh when none are registered (`HandoffWorkflowBuilder.cs:505-544`; `_handoff.py:1061-1074`), plus optional return-to-previous fast path (`HandoffWorkflowBuilder.cs:575-591`, tested at `HandoffOrchestrationTests.cs:849-952`).

### 4. When does a group conversation terminate?

A layered set of conditions, any of which ends the conversation:

1. **Iteration/round caps**: .NET `MaximumIterationCount` (default 40) checked in the base `ShouldTerminateAsync` (`GroupChatManager.cs:91-94`); Python `max_rounds` coerced to ≥1 with a forced completion message (`_base_group_chat_orchestrator.py:165, 499-538`).
2. **Custom predicates**: sync/async callbacks over the conversation, evaluated before running an agent and again after each response (`RoundRobinGroupChatManager.cs:55-64`; `_handoff.py:365-367, 421-425`). Notably, .NET deliberately skips the termination condition whenever a handoff was requested (stamping at `HandoffAgentExecutor.cs:310-345`, tested at `HandoffOrchestrationTests.cs:1506`).
3. **LLM-decided completion**: the structured-output orchestrator's `terminate` flag (`_group_chat.py:557-566`) and Magentic's `is_request_satisfied` leading to `prepare_final_answer` and a permanent `_terminated`/`IsTerminated` flag (`_magentic.py:1111-1114, 1227`; `MagenticTaskContext.cs:74`).
4. **Limit exhaustion**: Magentic round/reset limits produce a terminal partial answer and set terminated (`MagenticOrchestrator.cs:250-261`; Python `_magentic.py:1229-1265`, asserted in `test_magentic.py:820-838`).
5. **Degenerate-routing guard**: if the .NET group-chat manager selects the agent who just spoke, the host treats it as termination instead of livelocking (`GroupChatHost.cs:78-85`).
6. **Human gates**: handoff without autonomous mode issues `request_info` after each non-handoff response (`_handoff.py:427-439`); an empty user response terminates the workflow (`_handoff.py:461-465`); Magentic plan sign-off pauses until approved (`_magentic.py:955-958, 993-1038`).
7. **Engine-level quiescence**: regardless of pattern, a run ends when the superstep loop finds nothing left to process (`_runner.py:170-177` raising `WorkflowConvergenceException` past 100 iterations; `InProcessRunner.cs:212-241`).

### 5. Is deadlock possible?

No dedicated deadlock detector exists (searched for `deadlock`, `stuck`, `idle`, `convergence` across both stacks; the only hits were implementation notes such as `Execution/FanInEdgeState.cs:40` warning about lock choices). What protects liveness is indirect:

- **Static validation**: unreachable-executor errors at build time on both stacks (`WorkflowBuilder.cs:609-613`; `_validation.py:311-334`), isolated-executor errors and self-loop/dead-end warnings (`_validation.py:322-334, 401-429`).
- **Bounded loops**: every autonomous/iterative construct has a numeric ceiling (40 manager iterations; 50 autonomous turns; 100 supersteps; configurable Magentic round/reset/stall limits).
- **Known livelock windows**: a Python group chat with neither `max_rounds` nor `termination_condition` is explicitly documented to continue indefinitely (`_group_chat.py:142-143`) — the framework relies on the 100-superstep `WorkflowConvergenceException` only if messages keep flowing, since a starved-but-idle loop simply exits. A handoff agent registered with no targets only produces a log warning that the "workflow may get stuck" (`_handoff.py:1095-1099`). Human-in-the-loop pauses persist indefinitely until an external response arrives (by design; resumable via checkpoints).

So: hard deadlock (cycle of waiting executors) is largely prevented because routing is asynchronous message passing with no cyclic blocking waits; soft deadlock (workflow parked forever awaiting input, or an unbounded conversational loop) is possible and acknowledged rather than detected.

## Architectural Decisions

- **Superstep core, orchestration patterns as sugar**: group chat/handoff/magentic are compiled down to plain graphs of executors and edges executed by the shared engine (`GroupChatWorkflowBuilder.cs:70-90`; `_handoff.py:989-1012`), so routing semantics inherit engine-level ordering, checkpointing, and validation for free.
- **Explicit speak-token**: the `TurnToken` message makes "whose turn is it" an addressable, checkpointable piece of protocol state rather than implicit control flow (`TurnToken.cs:12-19`; persisted at `GroupChatHost.cs:136, 150`).
- **Centralized vs decentralized routing as a first-class distinction**, stated in module docs: "Group Chat: centralized orchestration… Handoff: decentralized routing by agents themselves" (`_handoff.py:16-22`).
- **Control flow encoded as data**: .NET handoff decisions ride in a serializable `HandoffState(TurnToken, RequestedHandoffTargetAgentId, PreviousAgentId, IsTerminated)` consumed by declarative switch cases (`HandoffState.cs:5-9`; `HandoffWorkflowBuilder.cs:480-487`), enabling the whole routing graph to be validated, visualized, and checkpoint-restored.
- **Termination as composition of predicates and budgets** rather than one hook: user predicates + hard caps + model verdicts + engine quiescence (evidence table rows above).
- **Dual-stack parity by reimplementation, not code sharing**: Python and .NET implement the same patterns separately (e.g., Python handoff via middleware interception vs .NET via streaming-call collection and switch edges), accepting divergence risk in exchange for idiomatic APIs.
- **Resumability of routing state**: round-robin cursors, current-speaker ids, conversation bookmarks, autonomous-turn counters, and Magentic counters/ledgers are all serialized into checkpoints (`RoundRobinGroupChatManager.cs:74-86`; `GroupChatHost.cs:133-158`; `HandoffAgentExecutor.cs:388-421`; `HandoffEndExecutor.cs:84-149`; `MagenticOrchestrator.cs:357-404`).

## Notable Patterns

- **Broadcast-with-exclusion**: every group-chat variant broadcasts the latest delta to all participants except the author, keeping per-agent sessions synchronized while avoiding echo (`GroupChatHost.cs:96-110`; `_group_chat.py:224-231`; `BroadcastReplyToOtherParticipantsAsync`, `MagenticOrchestrator.cs:218-240`).
- **Dual-envelope participant abstraction**: agents receive bare `AgentExecutorRequest`s while custom executors receive richer envelopes, letting executors and agents participate uniformly (`_base_group_chat_orchestrator.py:453-495`).
- **Tool call as routing signal**: handoff reuses the model's function-calling surface as a control-flow channel, intercepted before real execution (`_AutoHandoffMiddleware`, `_handoff.py:132-154`) — no side effects, deterministic results.
- **Structured-output orchestrator**: the speaker-selection LLM must answer `{terminate, reason, next_speaker, final_message}` with strict schema and defensive parsing fallbacks (`_group_chat.py:265-281, 419-487`).
- **Ledger-driven self-correction**: Magentic converts "no progress / in a loop" into an incrementing stall counter with decay on progress; crossing the threshold triggers context reset, participant reset broadcast (`MagenticResetSignal`, `_magentic.py:1267-1271`; `ResetChatSignal`, `MagenticOrchestrator.cs:344`), and replanning (`_magentic.py:1117-1125, 1156-1192`).
- **Bounded retry with backoff for model-format failures**: progress-ledger JSON extraction retries up to 3 times with 250 ms × attempt delays, then escalates to reset (`MagenticManager.cs:70-105`; `_magentic.py:720-739`).
- **Graceful degradation over hard failure for speaker identity**: non-string/empty next-speaker answers fall back to the first participant with warnings (`MagenticOrchestrator.cs:307-313`; `_magentic.py:1128-1132`), while *unknown* names fail toward termination (`MagenticOrchestrator.cs:315-322`; `_magentic.py:1135-1138`) or raise (plain group chat, `_group_chat.py:251-252`).

## Tradeoffs

- **Mesh topologies cost O(n²) edges**: default handoff wiring connects every pair (`_handoff.py:1003-1010`), fine for small teams but scaling poorly in edge count and broadcast volume compared to the star topology of group chat.
- **LLM-selected speakers trade robustness for flexibility**: hallucinated names and malformed ledgers are routine, forcing the fallback/retry machinery above; a deterministic manager avoids this class entirely.
- **Predicate-based switches defer errors to runtime**: because predicates close over message content, build-time type validation across conditional edges is intentionally not performed in .NET (documented limitation, `WorkflowBuilder.cs:588-604`).
- **Human gates buy auditability at the cost of liveness**: default handoff mode blocks after every non-handoff response until user input arrives; autonomous mode removes that but needs its own budgeted limits.
- **Cross-stack duplication**: identical semantics implemented twice (e.g., autonomous turn counting per-agent with resets in .NET shared state vs a single recursive counter in Python) creates drift risk; behaviors already differ subtly (see below).
- **Recursion vs iteration for autonomous loops**: Python's `_run_agent_and_emit` recurses per autonomous continuation (`_handoff.py:428-435`), relying on the turn limit (default 50) to bound stack growth, whereas .NET loops through the End executor and a graph switch (`HandoffEndExecutor.cs:101-130`).

## Failure Modes / Edge Cases

- **Unbounded conversation**: Python group chat with neither `max_rounds` nor `termination_condition` never terminates on its own (documented, `_group_chat.py:142-143`).
- **Silent misconfiguration**: an agent with no handoff targets logs "your workflow may get stuck" instead of failing the build (`_handoff.py:1095-1099`).
- **Duplicate handoff calls resolved arbitrarily**: multiple handoff tool calls in one .NET turn emit a warning and use the *last* call (`HandoffAgentExecutor.cs:457-465`); Python structurally prevents this via `allow_multiple_tool_calls=False` (`_handoff.py:287-289`) — divergent defenses for the same hazard.
- **Self-selection guard differs by stack**: .NET group chat terminates if the manager re-selects the current speaker (`GroupChatHost.cs:78-85`); Python relies on the user's selection function and round caps.
- **Post-restore broadcast suppression**: after a Magentic checkpoint restore, replies are not broadcast until the current-speaker id is re-established, temporarily degrading participant synchronization rather than risking echo (`MagenticOrchestrator.cs:222-227`).
- **Handoff while holding pending approvals/tool calls throws**: `InvalidOperationException` rather than queueing (`HandoffAgentExecutor.cs:257-260`) — a sharp edge if an agent requests a handoff mid-tool-flow.
- **Stalled conversations become partial answers**: hitting Magentic round/reset limits yields a synthesized "stopped due to hitting maximum … count" message flagged as the final output (`MagenticOrchestrator.cs:252-261`), which callers must distinguish from genuine completion by inspecting events/state.
- **Re-entrant invocation safety**: delivering a `HandoffState` to an agent mid-turn raises `"Cannot have multiple simultaneous conversations"` (`HandoffAgentExecutor.cs:354-358`), protecting shared-state consistency at the cost of rejecting concurrent inputs.

## Future Considerations

- Add a **runtime liveness watchdog**: detect supersteps that make no routing progress (no message delivery, no state change) and surface a diagnostic event, complementing the existing `WorkflowConvergenceException`.
- Unify **same-hazard defenses across stacks** (parallel handoff prevention, self-selection handling, unknown-speaker policy) so behavior is predictable regardless of language choice.
- Promote the **documented footgun to a guardrail**: require at least one of `max_rounds`/`termination_condition` (or emit a loud warning event) when constructing a Python `GroupChatOrchestrator`.
- Replace Python's recursive autonomous continuation with an iterative loop to remove stack-depth coupling with the turn limit (`_handoff.py:427-435`).
- Extend build-time validation to cover conditional edges once async executor factories and polymorphic handler typing allow it (the .NET builder documents this as a deliberate TODO, `WorkflowBuilder.cs:588-604`).
- Complete the retirement of duplicated public surfaces (obsolete `HandoffsWorkflowBuilder` alias marked for removal, `HandoffWorkflowBuilder.cs:19-25`).

## Questions / Gaps

- **No explicit deadlock/liveness detector found.** Searched both stacks for `deadlock`, `stuck`, `idle`, `convergence`; findings limited to build-time connectivity validation, convergence exceptions, and incidental comments (`FanInEdgeState.cs:40`). If the project considers deadlock detection in scope, there is no evidence of it in this snapshot.
- **Go implementation absent.** `go/README.md` exists with no packages; dimension analysis therefore covers .NET and Python only.
- **Broadcast delivery guarantees** are documented informally in the Python runner docstring (per-edge ordering rules, `_runner.py:186-198`); no formal spec or test asserting cross-participant broadcast atomicity was found within this study's boundary.
- **Magentic plan-review revision loops** re-request human review after each revision (`_magentic.py:1035-1038`); no cap on revision rounds was found — termination depends on the human eventually approving.
- **Custom .NET `GroupChatManager` implementations beyond round-robin** could not be located in-tree (only `RoundRobinGroupChatManager` ships in `dotnet/src`); the extension story rests on the abstract base class (`GroupChatManager.cs:16-31`) and its XML-doc contract rather than shipped examples.

---

Generated by `15.02-message-routing-and-termination` against `agent-framework`.
