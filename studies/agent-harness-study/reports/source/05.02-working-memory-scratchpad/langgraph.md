# Source Analysis: langgraph

## 05.02 Working Memory and Scratchpad

### Source Info

| Field | Value |
|-------|-------|
| Name | langgraph |
| Path | `studies/agent-harness-study/sources/langgraph` |
| Language / Stack | Python monorepo (`libs/langgraph` core engine, `libs/prebuilt` agent presets, `libs/checkpoint*` persistence backends) |
| Analyzed | 2026-08-25 |

All citations below are relative to `studies/agent-harness-study/sources/langgraph/` (workspace-relative prefix omitted for readability; every path is inside the selected source directory).

## Summary

LangGraph implements working memory at two distinct layers. The engine layer maintains a private, per-task scratchpad — `PregelScratchpad` (`libs/langgraph/langgraph/_internal/_scratchpad.py:8-19`) — that tracks loop position (`step`, `stop`), atomic call/interrupt/subgraph counters, and the list of resume values for pending interrupts. It lives in task config under the reserved key `__pregel_scratchpad` (`libs/langgraph/langgraph/_internal/_constants.py:64-65`, documented as "a mutable dict for temporary storage scoped to the current task") and is recreated from checkpointed pending writes on every task dispatch (`libs/langgraph/langgraph/pregel/_algo.py:1280-1345`). On top of it, a managed-values mechanism exposes derived, read-only working state (e.g., `IsLastStep`, `RemainingSteps`) into node inputs without persisting or versioning it (`libs/langgraph/langgraph/managed/is_last_step.py:9-24`, `libs/langgraph/langgraph/pregel/_checkpoint.py:246-277`).

The durable residue of working memory is deliberately narrow: only interrupt/resume values are written to the checkpointer as reserved `__resume__` writes (`libs/prebuilt/../langgraph/langgraph/pregel/_io.py:75`, rehydrated at `libs/langgraph/langgraph/pregel/_algo.py:1289-1318`). Everything else in the scratchpad is ephemeral by design and deterministically reconstructible from checkpoint metadata (`step` restored at `libs/langgraph/langgraph/pregel/_loop.py:1700-1701`). There is no user-facing todo/notes scratchpad abstraction in this tree (no evidence of any todo tool in prebuilt); agents that want visible plan state must model it as regular graph state.

## Rating

**7 / 10** — Clear model with tests, explicit interfaces, and operational safeguards. The scratchpad has a precise lifecycle (per-task creation, parent delegation for nested tasks, atomic counters), its persistence boundary is explicit (only resume values survive), boundary clearing exists (time travel drops stale resume writes, `libs/langgraph/langgraph/pregel/_loop.py:874-900`), and behavior is covered by dedicated tests (`tests/test_pregel.py:5818-5854`, `tests/test_managed_values.py:19-27`). It falls short of 9-10 because the raw scratchpad itself is not observable/auditable (only its durable projections are), there is no higher-level "agent notes" facility, and the concept is essentially undocumented outside code comments (search of `docs/` for "scratchpad"/"working memory" returned no matches).

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Scratchpad dataclass | `PregelScratchpad` fields: `step`, `stop`, `call_counter`, `interrupt_counter`, `get_null_resume`, `resume`, `subgraph_counter` | `libs/langgraph/langgraph/_internal/_scratchpad.py:9-19` |
| Config key + intent | `CONFIG_KEY_SCRATCHPAD = "__pregel_scratchpad"` — "mutable dict for temporary storage scoped to the current task" | `libs/langgraph/langgraph/_internal/_constants.py:64-65` |
| Per-task creation | `_scratchpad()` builds a fresh scratchpad for each dispatched task from `pending_writes`, `resume_map`, `step`, `stop` | `libs/langgraph/langgraph/pregel/_algo.py:1280-1345`; PULL-path call site `libs/langgraph/langgraph/pregel/_algo.py:625-634` |
| Injection into task config | Scratchpad placed under `CONFIG_KEY_SCRATCHPAD` in the executable task's configurable | `libs/langgraph/langgraph/pregel/_algo.py:747`, also `:923`, `:1093`, `:1234` |
| Atomic counters | `LazyAtomicCounter` wraps `itertools.count` behind a double-checked lock | `libs/langgraph/langgraph/pregel/_algo.py:1426-1439` (class), comment `:1333` |
| Managed-value interface | `ManagedValue.get(scratchpad) -> V`; specs are plain classes recognized via `is_managed_value` | `libs/langgraph/langgraph/managed/base.py:18-28` |
| Derived working state | `IsLastStepManager.get` = `step == stop - 1`; `RemainingStepsManager.get` = `stop - step` | `libs/langgraph/langgraph/managed/is_last_step.py:9-24` |
| Managed read during execution | `local_read` resolves managed keys through the scratchpad: `managed[k].get(scratchpad)` | `libs/langgraph/langgraph/pregel/_algo.py:222-223` |
| Exclusion from durable state | `channels_from_checkpoint` hydrates only `BaseChannel` specs; managed specs return with no values | `libs/langgraph/langgraph/pregel/_checkpoint.py:246-277` |
| Step counter lifecycle | `self.step += 1` per non-exiting superstep; restored as `checkpoint_metadata["step"] + 1` on resume | `libs/langgraph/langgraph/pregel/_loop.py:1217-1219`, `:1700-1701` (sync), `:1960-1961` (async) |
| Interrupt bookkeeping | `interrupt()` reads scratchpad, takes `interrupt_counter()`, returns prior resume values, appends new ones | `libs/langgraph/langgraph/types.py:950-974` |
| Durable resume projection | `Command(resume=...)` mapped to `(NULL_TASK_ID, RESUME, value)` write persisted via checkpointer | `libs/langgraph/langgraph/pregel/_io.py:75`; consumption at `libs/langgraph/langgraph/pregel/_algo.py:1289-1331` |
| Subgraph resume routing | Namespace-hashed `resume_map` appended to task-specific resume list | `libs/langgraph/langgraph/pregel/_algo.py:1311-1314`; populated at `libs/langgraph/langgraph/pregel/_loop.py:910-914` |
| Boundary clearing | Time-travel replay drops cached `RESUME` writes so interrupts re-fire instead of returning stale values | `libs/langgraph/langgraph/pregel/_loop.py:874-900` |
| Nested-task isolation | Child scratchpad delegates null-resume consumption to parent to avoid double-consume | `libs/langgraph/langgraph/pregel/_algo.py:1320-1324`; test rationale `libs/langgraph/tests/test_pregel.py:5818-5828` |
| Subgraph naming | Parent scratchpad's `subgraph_counter()` disambiguates repeated subgraph invocations in `checkpoint_ns` | `libs/langgraph/langgraph/pregel/_loop.py:325-340` |
| Push-task call tracking | `_call` uses `scratchpad.call_counter()` to schedule successive `@task` calls | `libs/langgraph/langgraph/pregel/_runner.py:718-732` (sync twin `:867-871`) |
| Model-visible working state | Deprecated `AgentState` includes `remaining_steps: NotRequired[RemainingSteps]` alongside user-visible `messages` | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:57-63` |
| Budget gating in prebuilt agent | `_are_more_steps_needed` stops tool-calling when `remaining_steps < 2` | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:620-634` |
| ToolNode contract | ToolNode introspects the Pregel-installed `partial(local_read, scratchpad, channels, managed, task)` but reads channels only; managed values have their own injection path | `libs/prebuilt/langgraph/prebuilt/tool_node.py:1304-1312` |
| User-visible audit surface | `StateSnapshot` exposes `values`, `tasks`, `interrupts` — not scratchpad internals | `libs/langgraph/langgraph/types.py:683-701` |
| Tests | Managed values recognized in graph schemas; child-scratchpad resume correctness; partial-shape contract | `libs/langgraph/tests/test_managed_values.py:19-27`; `libs/langgraph/tests/test_pregel.py:5818-5854` (async `:8051` region); `libs/prebuilt/tests/test_on_tool_call.py:1324-1349` |

## Answers to Dimension Questions

1. **Does the agent keep private task state?**
   Yes. Two tiers: (a) `PregelScratchpad` — fully private runtime state per task (step position, call/interrupt counters, resume accumulator), inaccessible to node code except indirectly through managed values and `interrupt()`; (b) managed values (`IsLastStep`, `RemainingSteps`) that project scratchpad facts into node input state under declared schema keys (`libs/langgraph/langgraph/managed/is_last_step.py:15-24`).

2. **Is it durable?**
   Selectively, by design. The scratchpad object itself is ephemeral — recreated on every task dispatch (`libs/langgraph/langgraph/pregel/_algo.py:1280-1345`). What must survive process restarts does so through the checkpointer: resume values are stored as pending writes keyed by `NULL_TASK_ID` or task id (`libs/langgraph/langgraph/pregel/_io.py:75`, `libs/langgraph/langgraph/pregel/_algo.py:1292,1302`), subgraph resumes via a namespace-hash map (`_algo.py:1311-1314`), and the step cursor via checkpoint metadata (`_loop.py:1700-1701`). Managed values are never checkpointed (`_checkpoint.py:246-277`) but remain correct after restart because they are pure functions of reconstructed `step`/`stop`.

3. **Is it exposed to users?**
   Partially and intentionally. `StateSnapshot` shows channel values, tasks, and pending interrupts (`types.py:683-701`); interrupt payloads are explicitly client-facing ("This value will be sent to the client", `types.py:895-898`). `RemainingSteps` appears in agent state schemas and is therefore streamable/snapshotted like any channel-like field (`chat_agent_executor.py:62`). The scratchpad's internals (counters, resume accumulation mechanics) are never surfaced.

4. **Does it pollute long-term memory?**
   No evidence of pollution. The scratchpad and managed values have no write path into checkpoints (`_checkpoint.py:265-277` returns managed specs without values) or the long-term `Store`. Cross-run memory is a separate, opt-in mechanism: the reserved `__previous__` channel (`_constants.py:24`, injected into runtime at `_algo.py:692`) and user-defined channels. Working notes cannot silently become durable facts unless an application explicitly copies them into state.

5. **Can it be audited?**
   Its durable projections can: resume writes appear in checkpoint pending-writes retrievable via the saver API (`main.py:1239-1240` applies `saved.pending_writes`; snapshot `tasks[].interrupts` at `types.py:698-701`). The in-memory scratchpad itself (counters, transient resume list) is not logged, streamed, or introspectable — auditing requires inference from checkpoints and interrupt records. This is the weakest point for observability.

## Architectural Decisions

- **Scratchpad-as-infrastructure, not as product.** Working memory is an internal engine dataclass (`_scratchpad.py:8-19`) rather than an agent-facing API; agents compose their own visible memory from graph state. This keeps the framework neutral about planning styles (no baked-in todo semantics anywhere in `libs/prebuilt` — searches for `todo`/`todos` matched only lockfile noise).
- **Recompute-over-persist.** Everything except interrupt/resume accumulations is derivable from checkpoint metadata, so the durable footprint stays small and consistent with LangGraph's checkpoint-per-superstep model (`_loop.py:1199-1219`).
- **Managed values as a typed read-only projection.** Declaring `Annotated[int, RemainingStepsManager]` in a state schema routes the key away from channels entirely (`graph/state.py:1817+`, detection at `:1925`), giving nodes fresh budget information per read with zero serialization cost.
- **Parent-delegating nested scratchpads.** A child scratchpad forwards `get_null_resume` to its parent (`_algo.py:1320-1331`) so that exactly one consumer consumes a resume value across nested `@task`/subgraph boundaries — a subtle correctness rule captured by regression tests (`test_pregel.py:5818-5854`, async counterpart near `:8051`).

## Notable Patterns

- **Atomic counters via `itertools.count`**: thread-safe monotonic indices for call/interrupt/subgraph sequencing (`_algo.py:1333-1345`, `LazyAtomicCounter` at `:1426-1445`).
- **Reserved-key namespacing**: all internal state rides under `sys.intern`-ed `__pregel_*` config keys (`_constants.py:33-81`), with a `RESERVED` set blocking collisions for most keys (`_constants.py:110-140`; enforcement in `pregel/_validate.py:24-32`). Notably `CONFIG_KEY_SCRATCHPAD` itself is absent from `RESERVED` — harmless today since it is injected post-validation, but an inconsistency.
- **Contract-by-shape testing**: `ToolNode` introspects the runner-installed `functools.partial(local_read, ...)` to recover channel names (`tool_node.py:1304-1312`), and the test suite pins that exact partial shape (`test_on_tool_call.py:1324-1349`) — fragile-looking, but explicitly guarded.
- **Deterministic resume replay**: multi-interrupt flows index resume values by `interrupt_counter()` order and assert alignment (`types.py:962`), while time travel strips cached resumes so interrupts re-fire (`_loop.py:874-900`).

## Tradeoffs

- **Ephemerality vs. auditability**: keeping the scratchpad out of checkpoints keeps storage lean and avoids leaking intermediate reasoning into durable artifacts, but operators cannot inspect what a task "was thinking" mid-flight — only its checkpointed side effects.
- **Implicit visibility for managed values**: because `RemainingSteps` sits in the state schema, it is visible to models and streaming consumers by default; useful for budget-aware prompting (`chat_agent_executor.py:627-634`) but blurs the private/public line the scratchpad otherwise draws.
- **Shape-coupled internals**: `ToolNode` reaching into `read.args[1]` of a Pregel partial (`tool_node.py:1311`) trades encapsulation for zero-copy hydration; the guard test mitigates but does not eliminate coupling.
- **Order-indexed resumes**: resuming N interrupts depends on stable interrupt ordering (`types.py:953-963`); robust given deterministic task ids and counters, but any future nondeterminism in task scheduling would surface here first.

## Failure Modes / Edge Cases

- **Crash between interrupt and resume**: safe by construction — resume values live in checkpointer pending writes before the process continues (`_io.py:75`, `_loop.py:929` onward persists them), and the rebuilt scratchpad rehydrates them (`_algo.py:1289-1318`).
- **Stale resume on time travel**: would cause silent wrong-value replays; prevented by dropping cached `RESUME` writes when replaying a specific checkpoint (`_loop.py:874-900`).
- **Nested double-consumption of resume values**: prevented by parent delegation plus an assertion that the resume list length matches the interrupt index (`_algo.py:1320-1324`, `types.py:962`).
- **Retry storms on push tasks**: `call_counter()` ensures retried parents schedule the next `@task` invocation with the same index so already-completed futures are detected instead of re-executed (`_runner.py:720-756`).

## Future Considerations

- Add an opt-in debug/observability hook that serializes scratchpad state (counters, resume depth) into task metadata or callbacks, closing the audit gap without changing durability semantics.
- Promote `CONFIG_KEY_SCRATCHPAD` into the `RESERVED` set (`_constants.py:110-140`) for consistency with the other internal config keys.
- Document the scratchpad/managed-value split publicly; currently the design rationale lives only in code comments and test docstrings (e.g., `test_pregel.py:5824-5828`).
- Consider a supported "plan notes" managed-value pattern if agent harnesses keep reinventing per-node note state on top of ordinary channels.

## Questions / Gaps

- No user-facing todo/plan-state tool exists in this tree ("No clear evidence found" — searched `todo|todos|write_todos|scratchpad` across `libs/prebuilt` and docs; only false-positive lockfile hits and the internal `PregelScratchpad`).
- Whether the hosted LangGraph Platform surfaces additional working-memory telemetry could not be verified from this source; nothing in `libs/sdk-*` references the scratchpad key.
- Long-term-memory interaction was assessed structurally (separate `Store`/`Previous` mechanisms); behavioral tests proving zero scratchpad-to-store leakage were not found — the claim rests on the absence of any such write path in `pregel/`.

---

Generated by `05.02-working-memory-and-scratchpad` against `langgraph`.
