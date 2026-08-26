# Source Analysis: pydantic-ai

## Dimension 15.03: Shared State and Conflict Resolution

### Source Info

| Field | Value |
|-------|-------|
| Name | pydantic-ai |
| Path | `studies/agent-harness-study/sources/pydantic-ai` |
| Language / Stack | Python 3.10+ (asyncio/anyio, dataclasses/pydantic; uv workspace with `pydantic_ai_slim`, `pydantic_graph`, `pydantic_evals`, `clai`) |
| Analyzed | 2026-08-26 |

## Summary

Pydantic AI has no first-class multi-agent runtime: agents are deliberately stateless and designed to be global (`docs/multi-agent-applications.md:18`), so "shared state" is not a framework-managed store but a set of explicit opt-in sharing mechanisms layered over per-run state. The core model is:

1. **Per-run state is owned by the run.** All mutable loop state (message history, usage counters, retry budgets, pending-message queue, event buffer) lives in one `GraphAgentState` dataclass created fresh for each run (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:298-343`).
2. **Cross-agent sharing is by reference passing.** A parent agent shares its token/cost budget with a delegate by passing the same mutable `RunUsage` object via `usage=ctx.usage` (`docs/multi-agent-applications.md:20`, adopted by reference into run state at `pydantic_ai_slim/pydantic_ai/agent/__init__.py:1480-1487`); dependencies are shared the same way via `deps=ctx.deps`.
3. **Parallel writes converge through joins/reducers.** In `pydantic_graph`, forked branches write to a shared graph state, but all writes to join state are funneled through a single iterator task that applies user-supplied reducer functions serially (`pydantic_ai_slim/pydantic_ai/../pydantic_graph/pydantic_graph/graph_builder.py:717-726`), with reducers like append/extend/dict-update/sum and an early-stopping "first value wins + cancel siblings" reducer (`pydantic_graph/pydantic_graph/join.py:101-147`).
4. **Resource contention is coordinated with limiters.** An anyio-based `ConcurrencyLimiter` wraps models/agents, enforces queue backpressure atomically under a lock, and logs waiting to OpenTelemetry spans (`pydantic_ai_slim/pydantic_ai/concurrency.py:95-247`).
5. **Namespace conflicts are detected eagerly.** Combining toolsets raises a `UserError` on duplicate tool names, pointing at `PrefixedToolset`/`RenamedToolset` as the resolution (`pydantic_ai_slim/pydantic_ai/toolsets/combined.py:70-77`).

Conflict *resolution* in the classic sense (merge strategies for concurrent updates to one resource) exists only inside the graph join/reducer model; elsewhere the strategy is prevention (serialization through a single event-loop task, cloning before mutation, atomic check-and-increment under locks) rather than reconciliation.

## Rating

**6 / 10** — Present and well-modeled within a single process: clear interfaces (`RunUsage` reference sharing, `Join`+reducer API, `AbstractConcurrencyLimiter`), race-condition regression tests (`tests/test_concurrency.py:142-196`), batch-atomic limit enforcement (`pydantic_ai_slim/pydantic_ai/_tool_execution.py:444-448`), and OTel observability of contention. It falls short of 7–8 on durability grounds: shared mutable objects (`RunUsage`, toolset instances) rely on single-event-loop serialization rather than synchronization, cross-thread budget sharing is unsafe, durable-execution boundaries silently drop shared usage (documented and pinned by `tests/test_temporal.py:11507-11528`), there is no distributed lock or transactional compare-and-swap primitive, and conflicts are surfaced as spans/errors rather than queryable conflict logs.

## Evidence Collected

Every entry cites a path relative to the source root `studies/agent-harness-study/sources/pydantic-ai`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Run-scoped shared state container | `GraphAgentState`: `message_history`, `usage`, `output_retries_used`, `run_step`, `pending_messages`, `event_stream_buffer`, `mcp_tool_defs_cache` — created per run | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:298-343` |
| Shared-by-reference invariant for capability/tool sets | Comment documents that `loaded_capability_ids` and `discovered_tool_names` are shared by reference into every `RunContext` and only mutated in place — identity survives shallow copies | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:413-421` |
| Usage object adopted by reference into run state | `state = GraphAgentState(message_history=..., usage=usage, ...)` — caller's `RunUsage` instance becomes the run's live counter | `pydantic_ai_slim/pydantic_ai/agent/__init__.py:1480-1487` |
| Cross-agent budget sharing API | Docs instruct passing `usage=ctx.usage` to delegate runs so child usage counts toward the parent budget | `docs/multi-agent-applications.md:20,43-49` |
| Shared budget test | Delegate agent run asserts merged totals across parent/delegate (`RunUsage(requests=3, ...)`) | `tests/test_usage_limits.py:275-286` |
| Dependency sharing guidance | Delegates should receive the same deps or a subset; docs recommend reusing connections instead of re-initializing | `docs/multi-agent-applications.md:99-104,141-147` |
| RunContext exposes shared live objects | `deps`, `usage`, `usage_limits` documented as the live objects the run enforces against; read-only convention stated | `pydantic_ai_slim/pydantic_ai/_run_context.py:64-82` |
| Limit enforcement against shared state | `check_before_request`/`check_tokens`/`check_cost` raise `UsageLimitExceeded` reading the accumulated `RunUsage` | `pydantic_ai_slim/pydantic_ai/usage.py:492-572` |
| Batch-atomic conflict prevention | Parallel tool-call batch is checked as a whole against `tool_calls_limit` using `deepcopy` of projected usage before any call executes | `pydantic_ai_slim/pydantic_ai/_tool_execution.py:444-452` |
| Coordination lock (concurrency limiter) | `ConcurrencyLimiter` wraps `anyio.CapacityLimiter`; `_queue_lock = anyio.Lock()` guards waiting-count bookkeeping | `pydantic_ai_slim/pydantic_ai/concurrency.py:125-131` |
| Atomic backpressure check | Queue-depth check and registration done atomically under `_queue_lock`; comment names the prevented race | `pydantic_ai_slim/pydantic_ai/concurrency.py:207-219` |
| Contention logging | Waiting operations emit OTel spans with `source`, `waiting_count`, `max_running`, limiter name attributes | `pydantic_ai_slim/pydantic_ai/concurrency.py:224-240` |
| Shared limiter across agents/models | `AbstractConcurrencyLimiter` instances are shareable ("Redis-backed distributed limiters" given as subclass example); `ConcurrencyLimitedModel` example shows two models sharing one pool | `pydantic_ai_slim/pydantic_ai/concurrency.py:35-59`; `pydantic_ai_slim/pydantic_ai/models/concurrency.py:47-53` |
| Shared-limiter tests | `test_with_shared_limiter`, `test_shared_limiter_limits_across_models`, `test_agent_with_shared_limiter` | `tests/test_concurrency.py:398,410,486` |
| Race-condition regression test | `test_backpressure_race_condition` proves max_queued enforcement is atomic under concurrent load | `tests/test_concurrency.py:142-196` |
| Join/reducer conflict-resolution model | `Join` aggregates parallel branch outputs via reducer functions; `JoinState.current` holds the converging value per fork run | `pydantic_graph/pydantic_graph/join.py:32-38,150-199` |
| Built-in reducers | `reduce_list_append`, `reduce_list_extend`, `reduce_dict_update` (last-writer-wins per key), `reduce_sum`, `reduce_null` | `pydantic_graph/pydantic_graph/join.py:101-137` |
| Early-stopping resolution | `ReduceFirstValue` returns the first input and calls `ctx.cancel_sibling_tasks()`; iterator honors it via `_cancel_sibling_tasks` | `pydantic_graph/pydantic_graph/join.py:140-147`; `pydantic_graph/pydantic_graph/graph_builder.py:1096-1105` |
| Serialized reducer application | The single `_GraphIterator` loop applies `join_node.reduce(...)` for all branch results arriving over a memory-object stream — no concurrent reducer execution | `pydantic_graph/pydantic_graph/graph_builder.py:687-726` |
| Tool-name conflict detection | `CombinedToolset.get_tools` raises `UserError` naming both conflicting toolsets when two toolsets define the same tool name | `pydantic_ai_slim/pydantic_ai/toolsets/combined.py:70-77` |
| Conflict-hint resolution paths | `tool_name_conflict_hint` points to renaming or `PrefixedToolset`; `RenamedToolset` maps ambiguous names | `pydantic_ai_slim/pydantic_ai/toolsets/abstract.py:107-110`; `docs/toolsets.md:299,336` |
| Toolset sharing default + isolation hook | `for_run()` defaults to returning `self` (shared across runs); override to return a fresh instance for per-run state isolation | `pydantic_ai_slim/pydantic_ai/toolsets/abstract.py:112-118` |
| Clone-before-mutate safeguard | Output toolset is copied before mutating `max_retries` "so concurrent runs don't race on the shared agent-level toolset" | `pydantic_ai_slim/pydantic_ai/agent/__init__.py:1462-1474` |
| Cancellation fan-out coordination | `CancellationToken` is thread-safe (`threading.Lock`), idempotent, cancels all registered runs — "cancels a whole tree of runs at once" | `pydantic_ai_slim/pydantic_ai/_cancel.py:42-89`; `docs/multi-agent-applications.md:22` |
| First-party vs external cancel arbitration | `resolve()` consumes own cancellations via `Task.uncancel()`; external cancellation wins races; residual window documented (#7240) | `pydantic_ai_slim/pydantic_ai/_cancel.py:203-242` |
| Message-history sharing between agents | Programmatic hand-off passes `message_history=result.all_messages()` to the next agent; `conversation_id` inherited from history | `docs/multi-agent-applications.md:211-215,274-276`; `pydantic_ai_slim/pydantic_ai/_agent_graph.py:239-264` |
| Durable-execution isolation boundary | Temporal activities receive deserialized copies of the run context, so `usage=ctx.usage` does not merge back — pinned by test referencing issue #6886 | `tests/test_temporal.py:11507-11528`; `docs/multi-agent-applications.md:84-85` |

## Answers to Dimension Questions

**1. What state is shared between agents?**
Nothing is shared implicitly. Agents themselves hold no conversational or domain state (`docs/multi-agent-applications.md:18`). Sharing happens only through explicitly passed references:
- **Usage/budget**: the parent's `RunUsage` object, passed as `usage=ctx.usage`, is mutated in place by both parent and delegate (`RunUsage.incr`, `pydantic_ai_slim/pydantic_ai/usage.py:371-381`) and enforced against via `UsageLimits` checks (`pydantic_ai_slim/pydantic_ai/usage.py:492-572`).
- **Dependencies**: arbitrary app state (`deps_type`) passed as `deps=ctx.deps` — typically clients/connections (`docs/multi-agent-applications.md:99-104`).
- **Message history**: handed off explicitly between runs via `all_messages()` / `message_history=` (`docs/multi-agent-applications.md:211-215,274-276`).
- **Cancellation scope**: a shared `CancellationToken` registers multiple concurrent runs and cancels them together (`pydantic_ai_slim/pydantic_ai/_cancel.py:42-48`).
- **Concurrency pools**: a named `ConcurrencyLimiter` can be shared across models and agents (`pydantic_ai_slim/pydantic_ai/models/concurrency.py:50-53`).
Within a single run, `GraphAgentState` fields (`message_history`, `usage`, `event_stream_buffer`, `mcp_tool_defs_cache`) and the invariant-shared `loaded_capability_ids`/`discovered_tool_names` sets are shared by reference into every `RunContext` (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:298-343,413-421`).

**2. How are conflicts detected?**
Three distinct detectors:
- **Namespace collisions**: duplicate tool names across combined toolsets raise `UserError` at tool-listing time, before any model request (`pydantic_ai_slim/pydantic_ai/toolsets/combined.py:73-77`). Capability-level equivalents exist via `PrefixTools` (`docs/capabilities/prefix-tools.md:3`).
- **Contention/backpressure**: exceeding a concurrency queue raises `ConcurrencyLimitExceeded`; detection of the underlying check-vs-wait race is made atomic with an `anyio.Lock` (`pydantic_ai_slim/pydantic_ai/concurrency.py:210-217`).
- **Budget overrun**: projected usage is deep-copied and checked before executing a parallel tool-call batch, so an over-limit batch executes zero calls (`pydantic_ai_slim/pydantic_ai/_tool_execution.py:444-448`); request/token/cost limits are checked before each request and after each response (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:1723,1786-1790`).

**3. How are conflicts resolved?**
- **Prevention by serialization**: parallel graph branches never mutate join state concurrently — results flow over a memory stream into one iterator task that applies reducers sequentially (`pydantic_graph/pydantic_graph/graph_builder.py:687-726`).
- **Declarative merge strategies**: applications choose reducers per join — list append/extend, dict update (last-write-wins per key), sum, discard, or `ReduceFirstValue` for first-wins-with-cancellation semantics (`pydantic_graph/pydantic_graph/join.py:101-147`).
- **Eager failure**: name conflicts fail fast with actionable resolution hints (prefix/rename, `pydantic_ai_slim/pydantic_ai/toolsets/abstract.py:107-110`).
- **Clone-before-mutate**: shared agent-level output toolsets are copied before per-run mutation to avoid cross-run races (`pydantic_ai_slim/pydantic_ai/agent/__init__.py:1465-1474`).
- **Arbitrated precedence**: if a first-party cancellation races an external one, external wins; the conservative direction is deliberate (`pydantic_ai_slim/pydantic_ai/_cancel.py:213-242`).
There is no automatic merge/rebase of divergent writes outside the join model; last-writer-wins via `reduce_dict_update` is the only merge semantic offered.

**4. Is shared state consistent?**
Consistent within one event loop, by construction rather than by locking: `RunUsage.incr` increments are synchronous read-modify-writes that cannot interleave at await points in a single-threaded asyncio task, and the graph iterator serializes reducer application. Consistency guarantees weaken at three boundaries: (a) **threads** — `CancellationToken` is thread-safe, but `RunUsage` has no synchronization, so multi-threaded budget sharing is unguarded; (b) **durable execution** — Temporal activities get a deserialized copy of the run context, so delegate usage silently fails to merge back (documented limitation pinned by `tests/test_temporal.py:11507-11528`); (c) **cross-process** — no built-in distributed coordination; users must subclass `AbstractConcurrencyLimiter` (the docstring sketches a Redis-backed limiter, `pydantic_ai_slim/pydantic_ai/concurrency.py:46-58`) or manage budgets externally, and the migration guide itself warns that check-before-call/update-after-call is not safe under concurrency or process failure (`pydantic_ai_slim/pydantic_ai/.agents/skills/migrating-langchain-to-pydantic-ai/references/WORKAROUND-RECIPES.md:207`).

## Architectural Decisions

1. **Stateless agents + reference-passed shared state.** Instead of a shared blackboard, sharing is explicit object identity: pass the same `RunUsage` or deps object to make it common (`docs/multi-agent-applications.md:20,101`). Simple and type-safe, but correctness depends on the caller understanding aliasing.
2. **Centralized reduction for parallelism.** `pydantic_graph` forks branches as tasks but funnels every branch result through one iterator that applies reducers (`pydantic_graph/pydantic_graph/graph_builder.py:696-726`). This trades parallel merge throughput for the elimination of write races on join state.
3. **Coordination as composable wrappers.** Concurrency limiting is a model wrapper (`ConcurrencyLimitedModel`) accepting a shareable `AbstractConcurrencyLimiter` extension point rather than a global scheduler (`pydantic_ai_slim/pydantic_ai/models/concurrency.py:27-76`).
4. **Fail-fast namespace management.** Tool-name conflicts abort configuration with a `UserError` plus remediation hint, rather than silent shadowing (`pydantic_ai_slim/pydantic_ai/toolsets/combined.py:73-77`).
5. **Observability as the conflict log.** Contention (waiting on limiters), usage, and cost surface as OTel spans/attributes (`pydantic_ai_slim/pydantic_ai/concurrency.py:224-240`; `pydantic_ai_slim/pydantic_ai/usage.py:218-253`) instead of a dedicated persistent conflict/event log.
6. **Documented degradation at serialization boundaries.** Where shared-by-reference state cannot survive (Temporal), the behavior is documented, tested, and tracked as an issue rather than hidden (`tests/test_temporal.py:11507-11516`).

## Notable Patterns

- **Shared-by-reference invariants written down**: comments on `loaded_capability_ids`/`discovered_tool_names` spell out exactly why these sets must be mutated in place and never reassigned, because shallow copies preserve identity (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:413-419`).
- **Projected-usage simulation**: deepcopying current usage, adding the hypothetical batch size, then checking limits — enforcing constraints on a future state without mutating shared state (`pydantic_ai_slim/pydantic_ai/_tool_execution.py:446-448`).
- **First-value-wins with sibling cancellation**: a reducer that both resolves the conflict (keeps first result) and enforces the decision (cancels losing branches) via `ReducerContext.cancel_sibling_tasks()` (`pydantic_graph/pydantic_graph/join.py:140-147`).
- **Idempotent, thread-safe control-plane handles**: `CancellationToken.cancel()` is idempotent, callable from any thread, and safe to re-register after firing (`pydantic_ai_slim/pydantic_ai/_cancel.py:61-85`).
- **Opt-in state isolation hooks**: `for_run()`/`for_run_step()` let a stateful toolset return fresh or transformed instances per run/step while defaulting to shared (`pydantic_ai_slim/pydantic_ai/toolsets/abstract.py:112-128`).

## Tradeoffs

- **Simplicity vs safety**: reference-passed mutable `RunUsage` makes budget sharing trivial in-process but provides no protection against cross-thread use or accidental double counting across unrelated runs that reuse one object.
- **Central reduction vs throughput**: serializing join reductions avoids locks entirely but means reducer functions must be fast; heavy aggregation happens on the iterator's critical path (`pydantic_graph/pydantic_graph/graph_builder.py:717-724`).
- **Default-share vs default-isolate toolsets**: defaulting `for_run()` to `self` maximizes reuse (MCP server connections etc.) but shifts the burden of avoiding cross-run state bleed onto toolset authors (`pydantic_ai_slim/pydantic_ai/toolsets/abstract.py:115-118`).
- **Fail-fast vs tolerance**: raising on tool-name collisions prevents subtle misrouting but blocks compositions that could otherwise be auto-namespaced; the library chose error-plus-hint over auto-prefixing (`pydantic_ai_slim/pydantic_ai/toolsets/combined.py:75-77`).
- **OTel-only contention visibility**: spans show who waited and why, but there is no persisted record of rejected/failed acquisitions beyond raised exceptions, limiting post-hoc forensics.

## Failure Modes / Edge Cases

- **Durable-execution usage loss**: delegate usage accrued inside a Temporal activity never merges back to the workflow-side run because the activity receives a copy — asserted explicitly in `tests/test_temporal.py:11507-11528` (issue #6886).
- **Cancellation attribution window**: if user code catches a first-party cancellation and calls `Task.uncancel()`, a subsequent external cancel with matching count can be misattributed as first-party; acknowledged as requiring issuance-identity tracking to fix properly (#7240) (`pydantic_ai_slim/pydantic_ai/_cancel.py:216-242`).
- **Python 3.10 degraded arbitration**: without `Task.cancelling()/uncancel()`, a raced first-party cancellation is always translated to `RunCancelled` even if external cancellation arrived simultaneously — documented degraded behavior (`pydantic_ai_slim/pydantic_ai/_cancel.py:18-20,227-230`).
- **Concurrent-run mutation of shared toolsets**: mitigated by clone-before-mutate, but only for the output toolset path called out in comments; any similar shared-instance mutation added later would reintroduce the race (`pydantic_ai_slim/pydantic_ai/agent/__init__.py:1462-1474`).
- **Queue-depth rejection is lossy**: once `max_queued` is hit, work raises `ConcurrencyLimitExceeded` immediately; callers own retry/backoff policy (`pydantic_ai_slim/pydantic_ai/concurrency.py:211-217`).
- **Dict-update reducers lose concurrent edits by design**: `reduce_dict_update` is last-arrival-wins per key; two branches writing the same key silently overwrite (`pydantic_graph/pydantic_graph/join.py:118-121`).
- **No answer found for**: a generic, reusable "conflict log" store (queryable record of detected/resolved conflicts). Searched for terms like `conflict log`, `conflict detection`, and merge/resolution helpers across `pydantic_ai_slim` and `pydantic_graph`; only OTel span emission and exception-raising were found.

## Future Considerations

- Add synchronized or sharded accumulation for `RunUsage` (or document thread-affinity loudly) so multi-threaded delegations do not corrupt shared budgets.
- Provide a first-class merge-back story for durable execution (e.g., activity-reported usage deltas reconciled by the workflow), closing the gap pinned in `tests/test_temporal.py:11507`.
- Ship a built-in distributed limiter implementation (the `AbstractConcurrencyLimiter` seam already anticipates Redis-backed coordination, `pydantic_ai_slim/pydantic_ai/concurrency.py:46-58`).
- Offer richer reducer primitives (versioned writes, CRDT-style counters/maps) in `pydantic_graph` joins for higher-fidelity parallel convergence.
- Persist contention events (acquisitions, rejections, cancellations) in a structured, queryable form alongside OTel spans to serve as a true conflict log.

## Questions / Gaps

- **How do teams coordinate budgets across processes today?** No evidence found in-repo beyond the extension-point sketch in `pydantic_ai_slim/pydantic_ai/concurrency.py:46-58` and external advice in the migration skill (`.../WORKAROUND-RECIPES.md:207`); no shipped Redis/DB-backed limiter exists.
- **Is cross-thread `RunUsage` sharing supported?** No synchronization was found in `pydantic_ai_slim/pydantic_ai/usage.py:371-414`; searched for `threading`/`Lock` in usage accumulation paths — none present. Behavior under threads appears undefined/unhandled.
- **Are there file-system/artifact-sharing toolsets with write-conflict handling?** The docs point to third-party packages for Task Management and File Operations (`docs/multi-agent-applications.md:368-369`; `docs/capabilities/third-party.md:7,31`); no built-in artifact store or file-edit conflict detection was found inside this source tree (searched `common_tools/` and `toolsets/`).
- **Does anything log resolved conflicts durably?** No evidence found; searches for conflict-log-like structures surfaced only OTel instrumentation (`_instrumentation.py`, `capabilities/instrumentation.py`) and exception messages.

---

Generated by `Dimension 15.03: Shared State and Conflict Resolution` against `pydantic-ai`.
