# Source Analysis: langgraph

## Dimension 23.01: Autonomy Boundary

### Source Info

| Field | Value |
|-------|-------|
| Name | langgraph |
| Path | `studies/agent-harness-study/sources/langgraph` |
| Language / Stack | Python (core: `libs/langgraph`, prebuilt agents: `libs/prebuilt`, platform SDKs: `libs/sdk-py`, `libs/sdk-js`) with a JS/TS SDK |
| Analyzed | 2026-08-24 |

## Summary

LangGraph treats autonomy as an **opt-in, developer-configured boundary**, not an enforced default. A compiled graph runs fully autonomously unless the developer installs one of three gating mechanisms: (1) static, compile-time `interrupt_before` / `interrupt_after` node lists (`libs/langgraph/langgraph/graph/state.py:1183-1184`), (2) dynamic in-node pauses via the `interrupt()` function that raises `GraphInterrupt` (`libs/langgraph/langgraph/langgraph/types.py:851-974`), or (3) platform-level API authorization via `@auth.on` handlers in `langgraph_sdk.auth` (`libs/sdk-py/langgraph_sdk/auth/types.py:236-250`). All gating is built on durable checkpoints — pausing requires a checkpointer (`types.py:870-871`), and resuming is done with the `Command(resume=...)` primitive (`types.py:798-848`). The engine guarantees boundary integrity mechanically: interrupt decisions are computed from channel versions (`pregel/_algo.py:155-185`), interrupts are never swallowed by tool error handlers (`libs/prebuilt/langgraph/prebuilt/tool_node.py:973-983`), and they propagate through subgraphs with namespace-scoped resume mapping. What LangGraph does *not* provide is a default policy: nothing is gated unless the developer says so, and the human-response schema (`HumanInterruptConfig`, `HumanResponse`) is advisory metadata whose enforcement is left to clients.

## Rating

**7/10** — Clear model with tests, explicit interfaces, and operational safeguards.

Rationale: The autonomy boundary mechanism is explicit, typed, and heavily tested (`tests/test_interruption.py`, `tests/test_interrupt_migration.py`, dynamic-interrupt cases in `tests/test_large_cases.py:4147-4462`, subgraph approval flows in `tests/test_pregel.py:7331-7414`). Pause/resume is durable and observable (`__interrupt__` stream payloads, `get_state().tasks[].interrupts`). It falls short of 9-10 because there is no default-deny or policy layer inside the core loop (a graph with no configured interrupts executes every tool call autonomously), resume payloads are not validated against any schema in core, and the normative documentation lives off-repo (docs.langchain.com), leaving only README claims (`README.md:40`) and docstrings in-tree.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Static gating config | `compile(interrupt_before=..., interrupt_after=...)` accepted on `StateGraph.compile` | `libs/langgraph/langgraph/graph/state.py:1183-1184` |
| Gating validation | Interrupt node names validated against known nodes at compile time | `libs/langgraph/langgraph/pregel/_validate.py:100-107` |
| Gating enforcement (before) | `before_tick` sets status `"interrupt_before"` and raises `GraphInterrupt` when `should_interrupt` matches | `libs/langgraph/langgraph/pregel/_loop.py:666-671` |
| Gating enforcement (after) | `after_tick` checkpoints first, then raises `GraphInterrupt` for `interrupt_after` | `libs/langgraph/langgraph/pregel/_loop.py:718-724` |
| Interrupt decision algorithm | `should_interrupt()` requires channel updates since last interrupt AND task-name match; hidden tasks excluded | `libs/langgraph/langgraph/pregel/_algo.py:155-185` |
| Dynamic pause primitive | `interrupt(value)` raises resumable `GraphInterrupt`; requires checkpointer; full worked example in docstring | `libs/langgraph/langgraph/langgraph/types.py:851-940` |
| Resume primitive | `Command(resume=...)` dataclass for resuming interrupts, incl. per-interrupt-id maps | `libs/langgraph/langgraph/langgraph/types.py:798-848` |
| Resume matching | Resume values matched by index within task scratchpad; `None` explicitly disallowed as resume value for HTTP ambiguity | `libs/langgraph/langgraph/langgraph/types.py:950-965`, `libs/langgraph/langgraph/langgraph/pregel/_algo.py:1290-1331` |
| Interrupt identity | `Interrupt` dataclass with stable `id` derived from checkpoint namespace (`from_ns`) enabling direct targeted resume | `libs/langgraph/langgraph/langgraph/types.py:573-628` |
| Deprecated legacy gate | `NodeInterrupt(GraphInterrupt)` deprecated in favor of `interrupt()` | `libs/langgraph/langgraph/langgraph/errors.py:110-119` |
| Interrupt cannot be swallowed | ToolNode re-raises `GraphBubbleUp` before generic error handling converts errors to ToolMessages | `libs/prebuilt/langgraph/prebuilt/tool_node.py:973-983` |
| HITL interaction schema | `HumanInterruptConfig(allow_ignore/allow_respond/allow_edit/allow_accept)`, `ActionRequest`, `HumanInterrupt`, `HumanResponse(type: accept\|ignore\|response\|edit)` | `libs/prebuilt/langgraph/prebuilt/interrupt.py:11-105` |
| Prebuilt agent gating hook | `create_react_agent(..., interrupt_before=[...])` documented as "user confirmation ... before taking an action" | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:302-303,447-451` |
| Observability of interruption | `RunStream.interrupted` / `.interrupts` accessors on stream state | `libs/langgraph/langgraph/langgraph/stream/run_stream.py:193-206,557-569` |
| Subgraph propagation + tests | Subgraph approval-node interrupt resumes via `Command(resume={"user_message": "Yes"})`; regression-tested through parent graph streaming | `libs/langgraph/tests/test_pregel.py:7331-7414` |
| Dynamic interrupt tests | Conditional interrupt based on state ("DE" market); asserts interrupts aren't retried and resume flow works; also `durability="exit"` variant | `libs/langgraph/tests/test_large_cases.py:4147-4260` |
| Hard execution bound | `GraphRecursionError` raised when step budget exhausted (autonomy ceiling independent of HITL) | `libs/langgraph/langgraph/langgraph/errors.py:67-87` |
| Operator escalation (drain) | `RunControl.request_drain()` → cooperative stop at superstep boundary, checkpoint saved, resumable; `GraphDrained` exception | `libs/langgraph/langgraph/langgraph/runtime.py:79-104`, `libs/langgraph/langgraph/langgraph/errors.py:54-64` |
| Durability configurability | `Durability = Literal["sync", "async", "exit"]`; default `"async"` resolved from run config | `libs/langgraph/langgraph/langgraph/types.py:89-90`, `libs/langgraph/langgraph/langgraph/pregel/main.py:2602-2603` |
| Runtime gating override (remote) | Remote/PaaS graph APIs accept `interrupt_before`/`interrupt_after` per request | `libs/langgraph/langgraph/langgraph/pregel/remote.py:731-813` |
| Platform authorization | SDK auth types: user identity + permissions; `@auth.on` deny-by-default example scoped to Studio users vs thread owners | `libs/sdk-py/langgraph_sdk/auth/types.py:150-203,236-250` |
| Documentation posture | README markets "human oversight by inspecting and modifying agent state at any point"; detailed docs moved off-repo | `README.md:40`, `docs/llms.txt:1-5` |

## Answers to Dimension Questions

### 1. What determines agent autonomy?

Three layers, all developer-supplied:

- **Static topology gates**: node-name lists passed to `compile()` (`libs/langgraph/langgraph/graph/state.py:1183-1184`). The loop evaluates them each superstep via `should_interrupt()`, which fires only if some channel version advanced since the last interrupt and the pending task matches the list (or `"*"` for all nodes). Internal tasks tagged `TAG_HIDDEN` are structurally exempt from wildcard interrupts (`libs/langgraph/langgraph/langgraph/pregel/_algo.py:176-179`) — system machinery cannot be gated.
- **Dynamic application logic**: nodes/tools call `interrupt(value)` at runtime to halt and surface a payload (`libs/langgraph/langgraph/langgraph/types.py:851-874`). This makes gating conditional on state (e.g., only interrupt for risky tool calls), as tested in `test_dynamic_interrupt` (`libs/langgraph/tests/test_large_cases.py:4147-4176`: interrupt only when `market == "DE"`).
- **Platform authorization**: who may create/read/resume runs is governed by `@auth.on` handlers over user identity/permissions (`libs/sdk-py/langgraph_sdk/auth/types.py:236-250`).

The default is full autonomy: with no gates installed, the graph executes every node and tool call without human contact.

### 2. Are autonomy levels configurable?

Yes, but as composition of primitives rather than named levels:

- Gate placement (`interrupt_before`/`interrupt_after`, including `"*"`), dynamic conditions coded in nodes, and per-request overrides on remote graphs (`libs/langgraph/langgraph/langgraph/pregel/remote.py:782-783`).
- Per-interrupt interaction affordances via `HumanInterruptConfig` flags (`allow_ignore`, `allow_respond`, `allow_edit`, `allow_accept`, `libs/prebuilt/langgraph/prebuilt/interrupt.py:11-26`) and a four-way response vocabulary (`accept|ignore|response|edit`, `interrupt.py:104`).
- Durability level (`sync|async|exit`) controls how much state survives between steps (`libs/langgraph/langgraph/langgraph/types.py:89-90`), which determines whether a pause is even possible mid-run.
- There is no global "autonomy level" dial or policy engine; granularity is per-graph/per-node/per-call-site.

### 3. Are boundaries documented?

Partially, and increasingly off-repo. In-tree evidence: the `interrupt()` docstring is a complete tutorial including the checkpointer requirement and `__interrupt__` stream output (`libs/langgraph/langgraph/langgraph/types.py:852-940`); `create_react_agent` documents `interrupt_before` as confirmation-before-action (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:447-451`); README markets human-in-the-loop as a headline feature (`README.md:40`). However, `docs/llms.txt:1-5` shows normative docs moved to docs.langchain.com, so in-repo guidance is docstring-level. Notably, the prebuilt HITL types were migrated out to `langchain.agents.interrupt` with deprecation shims retained (`libs/prebuilt/langgraph/prebuilt/interrupt.py:7-10`), signaling the boundary vocabulary itself is in flux.

### 4. Does the system respect autonomy boundaries?

Yes — mechanically enforced, not advisory:

- Gates fire inside the loop engine itself (`before_tick`/`after_tick`, `libs/langgraph/langgraph/langgraph/pregel/_loop.py:666-671,719-724`), so application code cannot skip them.
- `GraphBubbleUp` exceptions bypass ToolNode's error-to-ToolMessage conversion (`libs/prebuilt/langgraph/prebuilt/tool_node.py:982-983`), preventing error handlers from accidentally absorbing a human gate; comments enumerate all raise sites (inside tools, graphs-as-tools, subgraphs).
- Invalid gate configs fail fast at compile time (`libs/langgraph/langgraph/langgraph/pregel/_validate.py:100-107`).
- Interrupts survive across process restarts because they ride on checkpoints; regression tests cover migration (`tests/test_interrupt_migration.py`), time-travel interplay (`tests/test_time_travel.py`), and subgraph persistence (`tests/test_subgraph_persistence.py`).

Caveat: the *response side* is respected only by convention. Core does not validate that a `HumanResponse` conforms to the `allow_*` configuration of the `HumanInterrupt` it answers; any resume value is delivered verbatim (`libs/langgraph/langgraph/langgraph/types.py:950-965`). Enforcement of "edit allowed but accept not" is delegated entirely to client UIs.

## Architectural Decisions

1. **Gating as control-flow exception, not scheduler feature**: `interrupt()` raises `GraphInterrupt` (subclass of `GraphBubbleUp`, `libs/langgraph/langgraph/langgraph/errors.py:50-51,102-108`) unwound to the nearest superstep boundary, where the loop persists a checkpoint and returns an `__interrupt__` update. Resume re-executes the node from its start (`types.py:862-868` documents the replay semantics and index-based matching of multiple interrupts).
2. **Checkpoints as the substrate for human review**: every gate depends on a checkpointer (`types.py:870-871`); durability mode tunes the cost/consistency tradeoff (`main.py:2705-2735` shows the deprecated `checkpoint_during` mapping onto `durability`). Human review latency is thus unbounded without risking state loss.
3. **Stable interrupt identity for distributed resume**: `Interrupt.id` is an xxh3 hash of the checkpoint namespace (`types.py:607-618`), letting clients resume specific interrupts by id (`Command.resume` accepts a mapping of ids to values, `types.py:808-812`).
4. **Opt-in autonomy with hidden-task exemption**: wildcard interrupts deliberately exclude `TAG_HIDDEN` tasks (`_algo.py:176-179`), keeping internal bookkeeping ungated even under maximum scrutiny settings.
5. **Separation of engine gating from UI semantics**: the engine knows nothing about `accept/edit/respond/ignore`; those live in prebuilt/deprecated schemas (`libs/prebuilt/langgraph/prebuilt/interrupt.py`), keeping the core policy-free.

## Notable Patterns

- **Version-diff interrupt predicate**: `should_interrupt` compares channel versions seen since the last `INTERRUPT` marker, avoiding spurious re-interrupts on resume replays (`_algo.py:161-185`).
- **Scratchpad-based resume bookkeeping**: per-task `PregelScratchpad` counts interrupts and threads resume values by invocation order, with parent-namespace fallback for subgraphs (`_algo.py:1320-1345`, `_internal/_scratchpad.py:14-16`).
- **Sentinel-free resume protocol**: `None` is banned as a resume value to avoid HTTP null-vs-missing ambiguity (`_algo.py:1296-1298`) — an explicit distributed-systems consideration inside the boundary design.
- **Cooperative drain as operator escalation**: `RunControl.request_drain()` stops the run cleanly at a superstep boundary with a reason string, checkpointing for later resumption (`runtime.py:79-104`, `errors.py:54-64`) — an autonomy boundary aimed at infrastructure operators rather than domain reviewers.
- **Deprecation-with-shim migrations**: `NodeInterrupt` → `interrupt()` (`errors.py:110-119`) and prebuilt HITL types → `langchain.agents.interrupt` (`interrupt.py:7-10`) preserve old gating code paths while consolidating vocabulary.

## Tradeoffs

- **Safety vs. default behavior**: maximal expressiveness with zero defaults means an unconfigured agent has no autonomy boundary at all; safety depends entirely on developer diligence (no deny-by-default mode exists in the core loop).
- **Replay-for-resume vs. side-effect safety**: resuming re-executes the whole node from the start (`types.py:862-864`); non-idempotent side effects before the `interrupt()` call will repeat unless developers structure code defensively. Tests assert interrupts themselves aren't retried (`test_large_cases.py:4176`), but preceding side effects are the developer's problem.
- **Policy-free core vs. client burden**: `HumanInterruptConfig` flags are descriptive metadata; two clients can enforce different policies against the same interrupt. Predictability across UIs is not guaranteed by the framework.
- **Static list gates vs. dynamic risk models**: name-list gates are coarse; fine-grained "gate only dangerous calls" requires application-level conditional `interrupt()` calls, moving the boundary decision into user code where it escapes framework audit.

## Failure Modes / Edge Cases

- **Missing checkpointer**: `interrupt()` cannot work without persistence (documented requirement, `types.py:870-871`); `durability` is ignored with a warning when no checkpointer exists (`main.py:2802-2804`). A developer who adds gates but forgets the checkpointer silently gets none.
- **Interrupt ordering fragility**: multiple `interrupt()` calls in one node match resume values positionally (`types.py:866-868`); reordering calls changes which human answer lands in which variable — a silent contract between node code and resume history.
- **Hidden-task blind spot**: `TAG_HIDDEN` tasks are invisible to wildcard interrupts (`_algo.py:176-179`); anything marked hidden is ungated by construction.
- **Error-handler interference**: any custom wrapper that catches broad `Exception` around tool/node execution could swallow `GraphInterrupt` unless it special-cases `GraphBubbleUp`, as ToolNode deliberately does (`tool_node.py:982-983`).
- **Unvalidated responses**: a client can resume an edit-gated interrupt with arbitrary payloads; type errors surface (if at all) downstream in node logic, not at the boundary.

## Future Considerations

- Enforce `HumanInterruptConfig.allow_*` server-side (validate `HumanResponse.type` against the pending `HumanInterrupt.config` before accepting a resume write) so policy travels with the interrupt rather than the client.
- Provide a declarative per-tool gating policy (e.g., "always interrupt before tools matching X") at the prebuilt-agent level, since `create_react_agent` currently exposes only node-level `interrupt_before` (`chat_agent_executor.py:302-303,824-825`).
- Restore normative HITL documentation in-tree or generate it from docstrings, given `docs/llms.txt:1-5` confirms external hosting.
- Extend observability: surface gate provenance (which rule — static list vs. dynamic call — produced each interrupt) in stream metadata for auditing.

## Questions / Gaps

- No evidence found of a core-side validator for `HumanResponse` payloads or `allow_*` enforcement; searched `langgraph` and `prebuilt` libs for consumers of `HumanResponse.type` outside its definition (`libs/prebuilt/langgraph/prebuilt/interrupt.py:87-105`) — enforcement appears wholly client-side.
- No evidence found of named autonomy levels (e.g., "supervised"/"semi"/"auto") anywhere in the repo; searched for `escalat|approval|permission` across `libs/` — hits are limited to test fixtures, MCP provider config (`require_approval` is an OpenAI/Anthropic provider parameter exercised in `libs/prebuilt/tests/test_react_agent.py:313-327`, not a LangGraph gate), and the platform auth layer.
- In-repo docs contain no dedicated human-in-the-loop guide (docs moved off-repo per `docs/llms.txt:1-5`); the single example notebook `examples/human_in_the_loop/wait-user-input.ipynb` was not executable-inspected here but exists as the only in-repo HITL example artifact.

---

Generated by `dimensions/23.01-autonomy-boundary` against `langgraph`.
