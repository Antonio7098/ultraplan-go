# Source Analysis: openhands

## Dimension 06.06: Search, Backtracking, and Alternative Plans

### Source Info

| Field | Value |
|-------|-------|
| Name | openhands |
| Path | `studies/agent-harness-study/sources/openhands` |
| Language / Stack | Python (OpenHands SDK), Pydantic, LiteLLM, Jinja2, ThreadPoolExecutor |
| Analyzed | 2026-08-27 |

## Summary

OpenHands SDK provides infrastructure for exploring alternatives through three mechanisms: (1) **conversation forking** (`LocalConversation.fork()`), (2) **parallel sub-agent delegation** via `DelegateExecutor` (spawn/delegate) and `TaskManager`/`TaskToolSet` (preferred), and (3) **critic-gated iterative refinement** that retries on low scores. However, there is no first-class search, planning-tree, or backtracking abstraction. Alternatives are ad-hoc: the LLM-agent decides to spawn sub-agents; there is no pluggable search strategy (BFS/DFS/MCTS), no scoring/ranking registry for competing plans, no persisted branch trace with comparison, and no explicit undo/backtrack primitive. Retention of history is implicit via `EventLog` file persistence and fork deep-copy; failed branches are isolated but not ranked. Cost controls are coarse-grained (max iterations, max children, stuck detector) rather than per-branch budgets.

## Rating

**Rating: 4 / 10 — Present but inconsistent, weakly documented, and fragile for search/backtracking use cases.**

Rationale: Fork and parallel-delegation give genuine ability to try alternatives in isolated conversations, and critic iterative refinement provides bounded retry. These are tested implicitly via `EventLog`/`ConversationState` persistence. But the dimension's core expectations — systematic search over alternatives, coexisting plans with explicit scoring/comparison, retained failed-branch traces, and bounded cost per branch — are absent or ad-hoc. No `SearchStrategy`, `Plan`, `Score`, `Backtrack`, or `Checkpoint` abstraction exists; retry is confined to transient API retries and critic follow-ups, not general plan exploration.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Branching logic — fork deep-copy | `LocalConversation.fork()` deep-copies agent via `model_validate(model_dump(expose_secrets=True))`, copies `EventLog` events with `model_copy(deep=True)`, and copies `activated_knowledge_skills` and `agent_state` under state lock | `studies/agent-harness-study/sources/openhands/_sdk_inspect/sdk/conversation/impl/local_conversation.py:314-415` |
| Branching logic — fork metrics isolation | `reset_metrics` flag controls whether fork inherits cost/token stats; default `True` gives fresh `ConversationStats` | `studies/agent-harness-study/sources/openhands/_sdk_inspect/sdk/conversation/impl/local_conversation.py:321,404-406` |
| Forked sessions — persistence isolation | Fork derives persistence dir from parent's `persistence_dir.parent` and re-creates via `get_persistence_dir`; sub-agents persist under `subagents/` subdir when parent persists | `studies/agent-harness-study/sources/openhands/_sdk_inspect/sdk/conversation/impl/local_conversation.py:359-381` and `studies/agent-harness-study/sources/openhands/_sdk_inspect/tools/delegate/impl.py:189-196` and `studies/agent-harness-study/sources/openhands/_sdk_inspect/tools/task/manager.py:109-119` |
| Forked sessions — alternate plan isolation via TaskManager | `TaskManager.start_task` creates independent `LocalConversation` per task with own `conversation_id`, own LLM metrics (`reset_metrics`), and temp/persistent dir; `_evict_task` closes/pauses sub-conversation to free resources | `studies/agent-harness-study/sources/openhands/_sdk_inspect/tools/task/manager.py:144-182,137-143,291-317` |
| Alternative plans — parallel delegation (legacy) | `DelegateExecutor.spawn` + `delegate` implements `spawn` (create N agents by ID) then `delegate` (fan-out tasks via `threading.Thread`, join, aggregate results) capped by `max_children=5` | `studies/agent-harness-study/sources/openhands/_sdk_inspect/tools/delegate/impl.py:41-50,100-244,254-384` |
| Alternative plans — modern task delegation | `TaskToolSet`/`TaskExecutor` wires `TaskManager` as preferred delegation path; `TaskAction` carries `prompt`, `subagent_type`, optional `resume` to continue prior branch | `studies/agent-harness-study/sources/openhands/_sdk_inspect/tools/task/definition.py:37-63,198-262` and `studies/agent-harness-study/sources/openhands/_sdk_inspect/tools/task/manager.py:92-182` |
| Parallel exploration within a step | `ParallelToolExecutor` runs tool calls concurrently with `ThreadPoolExecutor` and `ResourceLockManager`; `AgentBase.tool_concurrency_limit` gates concurrency (default 1) | `studies/agent-harness-study/sources/openhands/_sdk_inspect/sdk/agent/parallel_executor.py:38-92` and `studies/agent-harness-study/sources/openhands/_sdk_inspect/sdk/agent/base.py:338-347` |
| Retry — iterative refinement via critic | `CriticMixin._check_iterative_refinement` increments `agent_state[iterative_refinement_iteration]` only when `critic.should_refine(result)` (score < threshold) and `iteration < max_iterations`; injects follow-up `MessageEvent` instead of marking FINISHED | `studies/agent-harness-study/sources/openhands/_sdk_inspect/sdk/agent/critic_mixin.py:76-138` and `studies/agent-harness-study/sources/openhands/_sdk_inspect/sdk/agent/agent.py:206-238` |
| Retry — critic scoring interface | `CriticBase.evaluate(events, git_patch)` returns `CriticResult(score, success)`; `IterativeRefinementConfig` defines `success_threshold` and `max_iterations` | `studies/agent-harness-study/sources/openhands/_sdk_inspect/sdk/critic/base.py:20-54,57-114` |
| Retry — transient API retry (not plan search) | Tenacity `@retry` on `_is_retryable_error` for remote workspace and critic client; exponential backoff, `stop_after_attempt` / `wait_exponential` — infrastructure retry, not alternative-plan search | `studies/agent-harness-study/sources/openhands/_sdk_inspect/sdk/workspace/remote/base.py:37-43,363-367` and `studies/agent-harness-study/sources/openhands/_sdk_inspect/sdk/critic/impl/api/client.py:17,276-298` |
| Branch traces — event persistence | `EventLog` persists every event to `FileStore` under `events/event-{idx}-{event_id}.json` with file lock, de-duplicates by `event_id`, and rebuilds index from disk; forks/tasks copy events preserving trace | `studies/agent-harness-study/sources/openhands/_sdk_inspect/sdk/conversation/event_store.py:44-153,206-254` |
| Branch traces — conversation state snapshot | `ConversationState._save_base_state` serializes to `base_state.json` on every public field mutation via `__setattr__` autosave; `create()` handles fresh vs resume paths with cipher-aware encryption | `studies/agent-harness-study/sources/openhands/_sdk_inspect/sdk/conversation/state.py:250-272,275-402,404-426` |
| Cost limits — iteration cap per run | `LocalConversation.run()` enforces `iteration >= max_iteration_per_run` → sets `ERROR` with `ConversationErrorEvent(MaxIterationsReached)`; `max_iterations` field validated `gt=0` | `studies/agent-harness-study/sources/openhands/_sdk_inspect/sdk/conversation/impl/local_conversation.py:850-872` and `studies/agent-harness-study/sources/openhands/_sdk_inspect/sdk/conversation/state.py:106-111` |
| Cost limits — children / task cap | `DelegateExecutor._spawn_agents` rejects if `len(existing)+len(new) > max_children`; `TaskManager` has no explicit max but inherits `max_iteration_per_run` from agent definition or parent | `studies/agent-harness-study/sources/openhands/_sdk_inspect/tools/delegate/impl.py:142-151` and `studies/agent-harness-study/sources/openhands/_sdk_inspect/tools/task/manager.py:218-236` |
| Cost awareness — stuck detection | `StuckDetector.is_stuck()` checks last 20 events for repeating action-observation, action-error, monologue, alternating patterns; forces `STUCK` status in run loop | `studies/agent-harness-study/sources/openhands/_sdk_inspect/sdk/conversation/stuck_detector.py:24-138` and `studies/agent-harness-study/sources/openhands/_sdk_inspect/sdk/conversation/impl/local_conversation.py:811-820` |
| Cost awareness — metrics aggregation | Parent `ConversationStats.usage_to_metrics` collects sub-agent combined metrics under `delegate:{id}` or `task:{id}` keys, replaced (not merged) on repeated delegation | `studies/agent-harness-study/sources/openhands/_sdk_inspect/tools/delegate/impl.py:340-349` and `studies/agent-harness-study/sources/openhands/_sdk_inspect/tools/task/manager.py:381-389` |
| Condenser — context-window backpressure (not backtracking) | `Agent.step` detects `LLMContextWindowExceedError` / `LLMMalformedConversationHistoryError` and emits `CondensationRequest`; `LocalConversation.condense()` explicitly requires `LLMSummarizingCondenser` | `studies/agent-harness-study/sources/openhands/_sdk_inspect/sdk/agent/agent.py:543-580,979-1053` and `studies/agent-harness-study/sources/openhands/_sdk_inspect/sdk/conversation/impl/local_conversation.py:1110-1158` |
| Explicit rejection — user-driven alternative path | `reject_pending_actions` and `pop_blocked_action` allow rejecting hook-blocked actions, moving `WAITING_FOR_CONFIRMATION → IDLE` | `studies/agent-harness-study/sources/openhands/_sdk_inspect/sdk/conversation/impl/local_conversation.py:896-926` and `studies/agent-harness-study/sources/openhands/_sdk_inspect/sdk/conversation/state.py:447-458` |

## Answers to Dimension Questions

**1. Can the system try alternatives?**
Partially. The system can try alternatives through three concrete paths, none of which is a general-purpose search framework:
- **Fork**: `LocalConversation.fork()` (`_sdk_inspect/sdk/conversation/impl/local_conversation.py:314`) creates a deep-copied conversation sharing event history but with independent future execution — caller can diverge on different prompts or agents.
- **Parallel sub-agents**: `DelegateExecutor` (`tools/delegate/impl.py:121,254`) supports `spawn` (create N workers) then `delegate` (fan-out `tasks` dict in parallel threads, blocking join). The newer `TaskManager`/`TaskToolSet` (`tools/task/manager.py:144`, `tools/task/definition.py:198`) does the same via `start_task(prompt, subagent_type, resume)` with per-task `LocalConversation`.
- **Iterative refinement**: `CriticMixin` (`sdk/agent/critic_mixin.py:76`) retries after `FinishAction` when `CriticResult.score < success_threshold`, up to `IterativeRefinementConfig.max_iterations` (`sdk/critic/base.py:42-52`), injecting a follow-up user message.
Missing: no planner that generates N candidate plans, no systematic search loop (BFS/DFS/MCTS/tree), no declarative alternative enumeration. Alternatives are agent-initiated (LLM decides to call delegate/task) rather than framework-driven exploration.

**2. Are alternatives isolated?**
Yes, isolation is explicit and robust. Forked and delegated branches each get a new `LocalConversation` with its own identity, lock (`FIFOLock` per `ConversationState` at `sdk/conversation/state.py:218`), `EventLog`, and persistence directory (`workspace/conversations/<hex>` or `<parent>/subagents/<id>` at `tools/delegate/impl.py:189-196`, `tools/task/manager.py:114`). Metrics are isolated via `llm.reset_metrics()` per sub-agent (`tools/delegate/impl.py:164-167`, `tools/task/manager.py:305-309`) and only aggregated back as `usage_to_metrics["delegate:{id}"]` after join. Workspace (`working_dir`) is shared (same filesystem directory), so file mutations can race unless `ResourceLockManager` is respected — `ParallelToolExecutor` (`sdk/agent/parallel_executor.py:93-119`) serializes tools declaring the same resource keys, but undeclared tools get a `tool:{name}` mutex and file-level isolation is not guaranteed.

**3. How are alternatives compared?**
Weakly and inconsistently.
- **Critic score**: `CriticBase.evaluate()` (`sdk/critic/base.py:82-86`) produces `CriticResult(score: float, success: bool, ...)` and `IterativeRefinementConfig` (`sdk/critic/base.py:20-54`) compares `score < success_threshold` to decide retry; `should_refine` is the only formal scoring function. This is experimental (`AgentBase.critic` docstring says API may change at `sdk/agent/base.py:328-336`) and single-branch: it gates retry on the current branch, not ranking among N branches.
- **Delegation aggregation**: `DelegateExecutor._delegate_tasks` (`tools/delegate/impl.py:352-376`) concatenates sub-agent final responses (`"Agent {id}: {result}"`) without scoring, voting, or ranking. `TaskManager` returns independent `Task` objects (`tools/task/manager.py:61-90`) with `status`/`result` but no comparison primitive. The LLM parent is expected to interpret results.
No evidence found of a `Scorer`, `Ranker`, `BranchEvaluator`, or `BestPlanSelector` abstraction; no weighted comparison or Pareto ranking.

**4. Is backtracking explicit?**
No. There is no `backtrack()`, `revert()`, `checkpoint()`, or `undo()` API. The closest mechanisms are workarounds:
- **Fork-as-checkpoint**: caller can `fork()` before a risky step and abandon the failed branch by simply not continuing it; the failed `EventLog` persists on disk but is not automatically pruned or rewound.
- **Reject/pause**: `reject_pending_actions` (`sdk/conversation/impl/local_conversation.py:896`) converts pending `ActionEvent` without observation into `UserRejectObservation`, and `pause()` sets `PAUSED`; neither rewinds already-committed events or file mutations.
- **Condensation**: `condense()` (`sdk/conversation/impl/local_conversation.py:1110`) summaries history to reclaim context window but is forward-only.
No evidence of transaction-like rollback, snapshot/restore, or Git-style reset per branch. Persistence is append-only (`EventLog.append` at `sdk/conversation/event_store.py:119` raises on duplicate ID, never deletes).

**5. Are costs bounded?**
Coarsely, not per-branch.
- **Iteration bound**: `max_iteration_per_run` default 500 (`sdk/conversation/conversation.py:72,120`, `sdk/conversation/impl/local_conversation.py:99`, `sdk/conversation/state.py:106`) caps any `run()` loop; exceeding emits `ConversationErrorEvent` and sets `ERROR` (`sdk/conversation/impl/local_conversation.py:850-872`). Sub-agents inherit or override via `factory.definition.max_iteration_per_run` (`tools/task/manager.py:232-236`).
- **Children bound**: `DelegateExecutor.max_children=5` (`tools/delegate/impl.py:44`) hard-caps concurrent sub-agents; exceeding returns an error observation.
- **Stuck detection**: `StuckDetector` (`sdk/conversation/stuck_detector.py:62`) with thresholds `action_observation`, `action_error`, `monologue`, `alternating_pattern` terminates loops by setting `STUCK` (`sdk/conversation/impl/local_conversation.py:811-820`).
- **Retry bound**: `IterativeRefinementConfig.max_iterations` default 3 (`sdk/critic/base.py:48-49`).
Missing: no token/cost budget per branch, no global cost controller that sums across forks, no time-deadline or `max_tokens` per search, no circuit breaker that aborts the lowest-scoring branches first. Parent metrics aggregate after completion (`conversation_stats.usage_to_metrics` at `tools/delegate/impl.py:340` and `tools/task/manager.py:381`) but do not preemptively throttle.

## Architectural Decisions

- **Fork as deep-copy+new-ID** (`sdk/conversation/impl/local_conversation.py:314-415`): Decisions: (a) copy events rather than copy-on-write — keeps source immutable but O(n) cost; (b) JSON round-trip for agent to avoid thread-lock pickling issues; (c) hold state lock during copy to avoid torn reads. Implication: branching is safe but not cheap at 30k+ events.
- **Two delegation stacks, one preferred** (`tools/delegate/impl.py:1` deprecation notice vs `tools/task/`): Legacy `DelegateTool` (spawn+delegate, ≤5 children, threaded) remains for compatibility; `TaskToolSet`/`TaskManager` (task-per-conversation, resume-capable, temp-dir fallback) is the forward path. Both share workspace dir — filesystem isolation is not addressed.
- **Critic as optional mixin** (`sdk/agent/base.py:328`, `sdk/agent/critic_mixin.py:25`, `sdk/critic/base.py:57`): Critic is experimental, mode `finish_and_message` vs `all_actions` (latter warned as significantly slower). Iterative refinement is embedded in `_ActionBatch.finalize` (`sdk/agent/agent.py:206-238`) — retry decision is coupled to `FinishTool` handling, not a standalone planner loop.
- **Append-only event log + autosaved state** (`sdk/conversation/event_store.py:26`, `sdk/conversation/state.py:404`): No deletion API, file-locked append with `LOCK_TIMEOUT_SECONDS=30` and NFS warning. Enables auditability of failed branches but prevents true backtrack without external snapshot.
- **Resource-level locking for parallel tools** (`sdk/agent/parallel_executor.py:38`, `sdk/conversation/resource_lock_manager.py:104`): `ResourceLockManager` serializes same-resource tools while allowing disjoint tools to run concurrently; `tool_concurrency_limit` (`sdk/agent/base.py:338`) controls pool size.

## Notable Patterns

- **Lazy plugin/agent initialization gated on first `send_message`/`run`** (`sdk/conversation/impl/local_conversation.py:569-616`, `_ensure_agent_ready`): Preserves constructor-no-IO principle; sub-agent factories are resolved after plugins merged.
- **Prompt-cache pinning per conversation** (`sdk/conversation/impl/local_conversation.py:627-633`): `_prompt_cache_key = state.id` for OpenAI prefix-cache shard reuse across sub-agents via `model_copy` inheritance.
- **Confirmation-mode implicit retry**: `Agent.step` (`sdk/agent/agent.py:485-492`) detects `get_unmatched_actions` and re-executes before sampling new actions — a two-phase run loop that implements human-in-the-loop without explicit search.
- **Context-window recovery via condensation**: `LLMSummarizingCondenser` Summaries (`sdk/context/condenser/`) triggered by `LLMContextWindowExceedError`/`LLMMalformedConversationHistoryError` (`sdk/agent/agent.py:543-580`); forward-only, not backtracking.

## Tradeoffs

- **Isolation vs storage cost**: Fork copies every event (`model_copy(deep=True)` in loop at `sdk/conversation/impl/local_conversation.py:385-386`) — safe but expensive for long histories; no shared immutable prefix or copy-on-write.
- **Shared workspace vs true sandbox**: Delegated branches share `working_dir` (`tools/delegate/impl.py:157`, `tools/task/manager.py:282`) — enables fast file sharing but means one branch's file writes can corrupt another's assumptions; no per-branch filesystem snapshot.
- **Generality vs simplicity of critic**: Single `score < threshold` gate (`sdk/critic/base.py:114`) is simple to configure but insufficient for multi-branch ranking or multi-objective tradeoffs; `all_actions` mode is accurate but costly (API per action).
- **Bounded search vs unbounded autonomy**: `max_iteration_per_run=500` and `StuckDetector` are blunt instruments — they bound infinite loops but do not allocate budget adaptively to promising branches.
- **Thread-per-task delegation**: `DelegateExecutor._delegate_tasks` (`tools/delegate/impl.py:286-338`) uses `threading.Thread` per agent with blocking `join` — simple and isolated by `FIFOLock` but not async/cancelable and scales poorly beyond `max_children=5`.

## Failure Modes / Edge Cases

- **NFS file-lock unreliability** (`sdk/conversation/event_store.py:30-35`): `EventLog` warns flock fails on NFS/network FS — concurrent fork/task writes can interleave or duplicate; `_sync_from_disk` handles torn reads but not lost writes.
- **Stale index / duplicate event ID** (`sdk/conversation/event_store.py:93-150,241-253`): Rebuilds index on stale read and rejects duplicate `event_id` with `ValueError`; concurrent fork creation could hit duplicate UUID edge if clock-collided.
- **Fork after `FINISHED`/`STUCK`** requires resetting to `IDLE` on next `send_message` (`sdk/conversation/impl/local_conversation.py:703-711`); forking a `STUCK` conversation copies stuck-inducing history verbatim — child inherits stuck pattern and may immediately re-trigger `StuckDetector`.
- **Out-of-tokens without condenser** (`sdk/agent/agent.py:979-1053`): Without `LLMSummarizingCondenser`, `LLMContextWindowExceedError` propagates as terminal `ERROR` — no automatic backtrack or pruning; `condenser.handles_condensation_requests()==False` path only logs.
- **Critic evaluation failure swallowed** (`sdk/agent/critic_mixin.py:72-74`): Any exception in `critic.evaluate` returns `None` and the refinement gate assumes `False` — failed branch silently proceeds as success, potentially finishing prematurely.
- **Sub-agent metric loss on crash**: If `_run_until_finished` throws before `_update_parent_metrics` (`tools/task/manager.py:339`), parent stats miss the child's cost; metrics replacement (not additive) also double-counts incorrectly if parent is read mid-delegation.
- **Resource lock mis-declaration**: Tools that omit `declared_resources` get `tool:{name}` mutex (`sdk/agent/parallel_executor.py:160-162`) — two tools touching same file with declared=False still serialize only by tool name, not file path, risking races.

## Future Considerations

- Introduce a first-class `Branch`/`Plan` abstraction with `score`, `cost_budget`, and `status` (active/failed/pruned) and a `SearchStrategy` interface (BFS/beam/MCTS) over forked conversations — currently callers must orchestrate search manually.
- Add scored ranking for `DelegateExecutor`/`TaskManager`: return `TaskResult(score)` and a `select_best(keys)` helper so the parent can prune low-scoring branches automatically.
- Implement per-branch cost budgets (tokens, USD, wall-time) with preemptive cancellation via `Future.cancel` or async task groups, rather than coarse `max_iteration_per_run` + `StuckDetector`.
- Provide explicit `checkpoint()`/`restore(checkpoint_id)` or filesystem snapshot (e.g., overlay or copy-on-write workspace per branch) to enable true backtracking without full event replay or shared-dir races.
- Persist failed-branch traces with structured outcome (error type, critic score, token cost) in queryable index to support learning from failures; currently `EventLog` is file-per-event without aggregate search.
- Harden `EventLog` for shared-storage deployments (replace `flock` with DB-backed or Redis lock, or make `FIFOLock` distributed) to support scaled search across processes/hosts.

## Questions / Gaps

- No evidence found of a tested search algorithm, branching strategy test suite, or scoring regression test — `tests/` not in source snapshot; search behavior verified only via manual code paths.
- Unclear how `RemoteConversation` handles fork/delegate: `sdk/conversation/impl/remote_conversation.py` was not inspected for fork support; remote branching may be unsupported or server-mediated.
- Unclear whether `TaskManager.resume` preserves correct `max_iteration_per_run` and LLM metrics continuity — `_resume_task` re-creates conversation with persisted `conversation_id` but metrics handling not explicitly tested.
- No documentation found tying `StuckDetector` thresholds to cost controls or branch pruning policy — tuning guidance absent.
- Plugin/skill merge order (last-wins) interacts with forked agent customization in untested ways — e.g., forking after plugin load vs before.

---
Generated by `Dimension 06.06: Search, Backtracking, and Alternative Plans` against `openhands`.
