# Source Analysis: langgraph

## Dimension 14.01: Human-in-the-Loop Trigger Policy

### Source Info

| Field | Value |
|-------|-------|
| Name | langgraph |
| Path | `studies/agent-harness-study/sources/langgraph` |
| Language / Stack | Python (core `libs/langgraph`, prebuilt agents `libs/prebuilt`, checkpointers `libs/checkpoint`) |
| Analyzed | 2026-08-26 |

All citations below are workspace-relative and point into the selected source directory.

## Summary

LangGraph implements human-in-the-loop (HITL) as a **pause/resume mechanism built on durable checkpoints**, not as a policy engine. There are exactly two trigger families:

1. **Static, declarative triggers** — `interrupt_before` / `interrupt_after` node lists supplied at compile time (`studies/agent-harness-study/sources/langgraph/libs/langgraph/langgraph/graph/state.py:1183-1184`) or overridden per invocation (`studies/agent-harness-study/sources/langgraph/libs/langgraph/langgraph/pregel/main.py:2569-2570`). The Pregel loop raises `GraphInterrupt` before/after a superstep when triggered tasks match the list (`studies/agent-harness-study/sources/langgraph/libs/langgraph/langgraph/pregel/_loop.py:667-671`, `_loop.py:720-724`), gated by `should_interrupt()` (`studies/agent-harness-study/sources/langgraph/libs/langgraph/langgraph/pregel/_algo.py:155-185`).
2. **Dynamic, in-node triggers** — the `interrupt(value)` callable (`studies/agent-harness-study/sources/langgraph/libs/langgraph/langgraph/types.py:851-974`), which any node can invoke under arbitrary developer-authored conditions (tool risk, validation failure, low confidence). The first call raises `GraphInterrupt`; on resume the node is re-executed from its start and `interrupt()` returns the human-supplied value (`types.py:864-868`, `types.py:950-965`).

There are **no built-in semantic triggers**: no risk scoring, cost/budget thresholds, or policy-violation detectors ship in the framework. The only framework-level "budget" behavior is `GraphRecursionError` (`studies/agent-harness-study/sources/langgraph/libs/langgraph/langgraph/errors.py:67-83`), which halts with an error rather than routing to a human. Everything risk-related is delegated to application code inside nodes or to prebuilt-agent hooks. Trigger events *are* recorded: every interrupt is persisted as an `INTERRUPT` pending write via the checkpointer (`studies/agent-harness-study/sources/langgraph/libs/langgraph/langgraph/pregel/_runner.py:585-591`), surfaced through stream output (`__interrupt__` key), state snapshots (`StateSnapshot.interrupts`), and debug events — making trigger decisions auditable after the fact.

## Rating

**8 / 10** — Clear model with tests, explicit interfaces, and operational safeguards.

Rationale:
- The trigger model is explicit and dual-track (declarative node lists + imperative `interrupt()`), with deterministic interrupt IDs (`xxh3_128` of checkpoint namespace, `studies/agent-harness-study/sources/langgraph/libs/langgraph/langgraph/types.py:616-618`) enabling precise resume addressing.
- Triggers are highly configurable: per-node lists, `"*"` wildcard (`studies/agent-harness-study/sources/langgraph/tests/test_interruption.py:32`), and per-invocation override (`main.py:2569-2570`).
- Multiple trigger conditions combine cleanly; concurrent interrupts from parallel tasks are matched to resume values by ID map, enforced by a runtime check that rejects ambiguous resumes (`studies/agent-harness-study/sources/langgraph/libs/langgraph/langgraph/pregel/_loop.py:916-919`, `_loop.py:818-846`).
- Durability is proven: interruption works even when no state key was updated (`test_interruption_without_state_updates`, `studies/agent-harness-study/sources/langgraph/libs/langgraph/tests/test_interruption.py:11-50`), and hidden/system tasks cannot trigger spurious interrupts (`_algo.py:176-178`).
- Not 9–10 because: (a) all *semantic* triggers (risk, uncertainty, budget) are developer-authored with no framework support or schema; (b) the HITL request/response schemas (`HumanInterruptConfig`, `ActionRequest`, `HumanInterrupt`) were deprecated and moved out of this repo (`studies/agent-harness-study/sources/langgraph/libs/prebuilt/langgraph/prebuilt/interrupt.py:7-10`, `:29-32`, `:47-50`); (c) there is no first-class audit record of the human's decision (who/what/why) beyond raw persisted writes.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Dynamic trigger primitive | `interrupt(value)` raises `GraphInterrupt` on first call, returns resume value on replay; requires checkpointer | `studies/agent-harness-study/sources/langgraph/libs/langgraph/langgraph/types.py:851-974` |
| Resume matching by index/id | Scratchpad tracks interrupt counter; resume values matched positionally, sent as `RESUME` writes | `studies/agent-harness-study/sources/langgraph/libs/langgraph/langgraph/types.py:950-965` |
| Deterministic interrupt ID | `Interrupt.from_ns` derives id via `xxh3_128_hexdigest(ns)`; `Interrupt.id` documented as resumable handle | `studies/agent-harness-study/sources/langgraph/libs/langgraph/langgraph/types.py:596-618` |
| Static triggers at compile | `compile(interrupt_before=..., interrupt_after=...)` params on `StateGraph.compile` | `studies/agent-harness-study/sources/langgraph/libs/langgraph/langgraph/graph/state.py:1183-1184` |
| Wildcard config | `interrupt_before = "*"` supported; validated against known node names unless `"*"` | `studies/agent-harness-study/sources/langgraph/libs/langgraph/langgraph/pregel/_validate.py:100-105` |
| Per-invocation trigger override | `invoke/stream(..., interrupt_before=..., interrupt_after=...)` falls back to compile-time lists | `studies/agent-harness-study/sources/langgraph/libs/langgraph/langgraph/pregel/main.py:2550-2570` |
| Loop enforcement (before) | `should_interrupt(checkpoint, interrupt_before, tasks)` → status `interrupt_before`, raise `GraphInterrupt` | `studies/agent-harness-study/sources/langgraph/libs/langgraph/langgraph/pregel/_loop.py:666-671` |
| Loop enforcement (after) | Same check post-superstep → status `interrupt_after` | `studies/agent-harness-study/sources/langgraph/libs/langgraph/langgraph/pregel/_loop.py:719-724` |
| Trigger predicate | `should_interrupt`: fires only if channels updated since last interrupt AND task matches list; excludes `TAG_HIDDEN` tasks | `studies/agent-harness-study/sources/langgraph/libs/langgraph/langgraph/pregel/_algo.py:155-185` |
| Anti-re-trigger guard | On resume, `versions_seen[INTERRUPT]` records channel versions so static interrupts don't loop forever | `studies/agent-harness-study/sources/langgraph/libs/langgraph/langgraph/pregel/_loop.py:946-951` |
| Persistence of trigger event | Task raising `GraphInterrupt` commits `(INTERRUPT, ...)` (+ any `RESUME`) writes to checkpointer | `studies/agent-harness-study/sources/langgraph/libs/langgraph/langgraph/pregel/_runner.py:584-591` |
| Checkpointer write API | `put_writes` stores task-keyed writes durably (InMemorySaver shown; same interface in Postgres/SQLite) | `studies/agent-harness-study/sources/langgraph/libs/checkpoint/langgraph/checkpoint/memory/__init__.py:467-503` |
| Pending-interrupt accounting | `_pending_interrupts()` diffs `INTERRUPT` vs `RESUME` writes to find unresolved interrupt ids | `studies/agent-harness-study/sources/langgraph/libs/langgraph/langgraph/pregel/_loop.py:818-846` |
| Multi-interrupt safety | Resuming with >1 pending interrupt without an id→value map raises `RuntimeError` | `studies/agent-harness-study/sources/langgraph/libs/langgraph/langgraph/pregel/_loop.py:916-919` |
| Resume primitive | `Command(resume=...)` accepts single value or mapping of interrupt ids to values | `studies/agent-harness-study/sources/langgraph/libs/langgraph/langgraph/types.py:808-824` |
| Stream surfacing | Interrupts emitted as `{INTERRUPT: (...)}` updates/values chunks; `GraphOutput.interrupts` typed field for v2 | `studies/agent-harness-study/sources/langgraph/libs/langgraph/langgraph/pregel/main.py:3935-3954` |
| Snapshot surfacing | `StateSnapshot.interrupts` aggregates pending interrupts across tasks; populated from saved pending writes | `studies/agent-harness-study/sources/langgraph/libs/langgraph/langgraph/types.py:698-701`; `.../pregel/main.py:1257-1266` |
| Debug audit trail | `stream_mode="debug"` task_result payloads include `interrupts: [asdict(Interrupt)]` | `studies/agent-harness-study/sources/langgraph/libs/langgraph/langgraph/pregel/debug.py:122-127` |
| Explicit human input injection | `update_state(as_node=...)` creates a synthetic INTERRUPT-tagged task to apply human-provided values | `studies/agent-harness-study/sources/langgraph/libs/langgraph/langgraph/pregel/main.py:1958-1968` |
| Prebuilt agent hooks | `create_react_agent(post_model_hook=..., interrupt_before=["tools"])` for approval gates before tool execution | `studies/agent-harness-study/sources/langgraph/libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:297-303`, `:425-451`, `:824-825` |
| HITL request/response schemas | `HumanInterruptConfig` (allow_ignore/respond/edit/accept), `ActionRequest`, `HumanInterrupt`, `HumanResponse` — all `@deprecated`, moved to `langchain.agents.interrupt` | `studies/agent-harness-study/sources/langgraph/libs/prebuilt/langgraph/prebuilt/interrupt.py:7-26`, `:51-105` |
| Budget-like halt (not HITL) | `GraphRecursionError` raised at `recursion_limit`; docs say raise limit via config, not ask a human | `studies/agent-harness-study/sources/langgraph/libs/langgraph/langgraph/errors.py:67-83`; `.../pregel/main.py:3005-3011` |
| Deprecated node-level exception | `NodeInterrupt(GraphInterrupt)` deprecated in favor of `langgraph.types.interrupt` | `studies/agent-harness-study/sources/langgraph/libs/langgraph/langgraph/errors.py:111-119` |
| Test: dynamic trigger | `test_dynamic_interrupt` — conditional `interrupt()` inside a node based on state content | `studies/agent-harness-study/sources/langgraph/libs/langgraph/tests/test_large_cases.py:4147` |
| Test: static trigger w/o state change | `test_interruption_without_state_updates` — pause/resume cycle verified via `get_state().next` and checkpoint history counts | `studies/agent-harness-study/sources/langgraph/libs/langgraph/tests/test_interruption.py:11-50` |
| Test: agent tool approval gate | react-agent test uses `interrupt_before=["tools"]` then `Command(resume=True)` to approve | `studies/agent-harness-study/sources/langgraph/libs/langgraph/tests/test_pregel.py:5615` |
| Test: human-assistance tool | Prebuilt agent test where a tool calls `interrupt({"query": query})` and asserts single execution across resumes | `studies/agent-harness-study/sources/langgraph/libs/prebuilt/tests/test_react_agent.py:602-672` |
| Example notebook | `examples/human_in_the_loop/wait-user-input.ipynb` demonstrates pausing until user input | `studies/agent-harness-study/sources/langgraph/examples/human_in_the_loop/wait-user-input.ipynb` |

## Answers to Dimension Questions

### 1. What triggers human review?

Three mechanisms:

- **Declarative node-gate triggers**: `interrupt_before`/`interrupt_after` lists name the nodes whose entry/exit pauses execution (`studies/agent-harness-study/sources/langgraph/libs/langgraph/langgraph/graph/state.py:1183-1184`; enforcement at `_loop.py:667-671` and `_loop.py:720-724`). The predicate fires only when channels were updated since the last interrupt and the scheduled task matches the configured set; hidden tasks (e.g., internal control nodes tagged `TAG_HIDDEN`) never trigger (`_algo.py:163-184`).
- **Imperative in-node triggers**: `interrupt(value)` lets developers encode any condition — risky tool detected, validation failure, low model confidence — directly in node code (`types.py:851-974`). The docstring frames this explicitly as "enables human-in-the-loop workflows" (`types.py:854-856`).
- **No automatic semantic triggers.** Searches for `risk`, `approval`, `dangerous`, budget/cost-based triggers found only test fixtures and user-space examples; the framework itself never initiates review on its own judgment. `recursion_limit` exhaustion raises `GraphRecursionError` (an error, not a review request) (`errors.py:67-83`).

### 2. Are triggers configurable?

Yes, extensively:

- Compile-time: node-name lists or the `"*"` wildcard (`state.py:1183-1184`, validated in `studies/agent-harness-study/sources/langgraph/libs/langgraph/langgraph/pregel/_validate.py:100-105`).
- Invocation-time: every `invoke/stream/batch` accepts overriding `interrupt_before`/`interrupt_after`, falling back to compiled defaults (`main.py:2569-2570`).
- Semantic conditions are unbounded-by-framework: whatever the developer writes before calling `interrupt()` (see `test_dynamic_interrupt`, `test_large_cases.py:4147`).
- Prebuilt convenience: `create_react_agent(interrupt_before=["tools"], post_model_hook=...)` wires an approval gate between LLM response and tool execution (`chat_agent_executor.py:297-303`, `:804-806`, `:998-999`).
- What is *not* configurable: there is no registry mapping tool names/classes to auto-review policies; per-tool gating must be hand-coded inside the tool or a wrapper node.

### 3. Can users request human review?

Two directions exist:

- **Framework→human (primary)**: any node can solicit human input by calling `interrupt()`; the surfaced value is arbitrary payload (question, action request) received by the client (`types.py:894-901` example).
- **Human→framework (explicit injection)**: users can inject decisions/state mid-run through `update_state(...)` (which applies writes attributed to a chosen node, `main.py:1958-1968`) or drive step-wise execution by repeatedly invoking `None` past static interrupts (`studies/agent-harness-study/sources/langgraph/libs/langgraph/tests/test_interruption.py:42-48`). However, there is no first-class "ask a reviewer to look at this run" API — a human-initiated review must be modeled as graph topology (e.g., a dedicated review node) by the developer.
- Structured request/response vocabulary exists but was relocated: `HumanInterrupt`/`HumanInterruptConfig`/`ActionRequest`/`HumanResponse` define accept/ignore/respond/edit semantics, yet carry deprecation notices pointing to `langchain.agents.interrupt` (`studies/agent-harness-study/sources/langgraph/libs/prebuilt/langgraph/prebuilt/interrupt.py:7-10`, `:87-105`).

### 4. Are trigger decisions auditable?

Substantially yes, with one gap:

- **Trigger side is durable**: the interrupt payload (including id and value) is written to the checkpointer as a pending write keyed by task (`_runner.py:585-591`; storage at `studies/agent-harness-study/sources/langgraph/libs/checkpoint/langgraph/checkpoint/memory/__init__.py:493-503`) and survives process restarts.
- **Observable everywhere it matters**: stream `updates`/`values` chunks carry `__interrupt__` (`main.py:3935-3954`); `get_state()` returns `StateSnapshot.interrupts` (`types.py:700-701`, populated at `main.py:1265`); `debug` mode emits structured task-result records including serialized interrupts (`debug.py:122-127`); full checkpoint history is replayable via `get_state_history` (`test_interruption.py:39-40`).
- **Resume side partially recorded**: resume values flow through `RESUME` channel writes that are also persisted (`types.py:957-964` feeding `_runner.py:589-591`), so *what* the human answered is recoverable. But there is no dedicated audit record capturing *decision metadata* (responder identity, timestamp, rationale, accept-vs-edit classification); applications must layer that on top.

## Architectural Decisions

1. **Checkpoints are the substrate for HITL.** `interrupt()` refuses to work without a checkpointer (`types.py:870-871` docstring; runtime check at `_loop.py:905-908` raising `RuntimeError` for `Command(resume=...)` without one). This makes every pause/resume crash-safe and replayable rather than best-effort.
2. **Replay-on-resume semantics.** A resumed node re-executes from its start, and `interrupt()` calls replay previously supplied values positionally (`types.py:864-868`, `types.py:950-965`). This keeps side-effect ordering deterministic but demands nodes be idempotent up to the interrupt point.
3. **Version-vector gating for static triggers.** `should_interrupt` compares channel versions against `versions_seen[INTERRUPT]` so a static interrupt fires once per state change, preventing infinite pause loops (`_algo.py:161-168`; bookkeeping at `_loop.py:946-951`).
4. **Deterministic, content-addressed interrupt IDs.** IDs derive from `xxh3_128(checkpoint_ns)` (`types.py:616-618`), letting clients address specific interrupts in multi-interrupt resumes (`Command(resume={id: value})`, `types.py:811-812`) and letting the loop detect unresolved/hanging interrupts (`_loop.py:818-846`).
5. **Fail loudly on ambiguity.** Resuming multiple pending interrupts without an ID map raises immediately instead of guessing (`_loop.py:916-919`) — a deliberate operational safeguard over convenience.

## Notable Patterns

- **Dual-track triggering**: coarse declarative gates (`interrupt_before/after`) for predictable checkpoints plus fine-grained imperative `interrupt()` for data-dependent review — combinable in the same graph (`test_large_cases.py:1011` exercises wildcard gates while other tests use dynamic interrupts).
- **Control-plane-as-data**: interrupts, resumes, errors are all just channel writes (`INTERRUPT`, `RESUME`, `ERROR` constants at `studies/agent-harness-study/sources/langgraph/libs/langgraph/langgraph/_internal/_constants.py:9`), so the same write-application machinery handles them uniformly (e.g., `_loop.py:741-749` deliberately skips control signals when restoring cached writes).
- **Emit-suppress lifecycle**: `GraphInterrupt` propagating out of the loop is caught and suppressed after persisting/emitting (`_loop.py:1336-1375`), turning an exception into normal control flow visible downstream.
- **Prebuilt ergonomics layered on core**: `create_react_agent` exposes `post_model_hook` and `interrupt_before=["tools"]` so the common "approve this tool call" pattern needs zero custom nodes (`chat_agent_executor.py:425-451`).

## Tradeoffs

- **Generality vs. guidance**: the framework provides mechanics, not policy. Teams get bulletproof pause/resume but must invent their own risk taxonomy; nothing stops a developer from shipping a dangerous-tool path with no gate at all.
- **Positional resume matching fragility**: when resume values arrive as a plain list rather than an ID map, matching relies on interrupt order within the task (`types.py:866-868`). Editing a node to add/move an `interrupt()` call silently changes which resume value binds to which call on replay — the ID-map path avoids this but requires more client code.
- **Full-node replay cost**: re-running a node from its start on resume (`types.py:864`) is simple and correct, but expensive or non-idempotent pre-interrupt work (API calls, file writes) gets repeated.
- **Schema drift across repos**: canonical HITL types now live outside this monorepo (`prebuilt/interrupt.py:7-10`), splitting the audit surface between frameworks.

## Failure Modes / Edge Cases

- **Ambiguous multi-interrupt resume**: rejected with `RuntimeError` requiring ID-addressed resume (`_loop.py:916-919`) — safe failure, but a hard runtime stop.
- **Stale interrupt writes during time-travel**: replaying from an older checkpoint can leave orphaned `INTERRUPT` writes; the loop proactively strips them and forks a checkpoint to keep future resume accounting correct (`_loop.py:952-971`).
- **Interruption with no state delta**: static interrupts fire even when a step wrote nothing, relying on the version-update predicate rather than write presence — verified by `test_interruption.py:14-15`.
- **Hidden-task false positives**: internal/control tasks are excluded from triggering (`_algo.py:176-178`); without this exclusion, framework plumbing could spuriously pause graphs.
- **Cached-write interference**: cached successful-task writes skip caching for `INTERRUPT`/`ERROR` results (`_loop.py:1864-1874`), ensuring interrupted tasks always re-execute rather than serving stale cache.

## Future Considerations

- Add an optional **policy registry** mapping tools/nodes/risk annotations to automatic `interrupt_before` insertion, giving declarative risk-based triggers without bespoke nodes.
- Persist **structured decision metadata** (responder, timestamp, action type) alongside `RESUME` writes to close the decision-audit gap.
- Provide a stable in-repo (or version-pinned) location for `HumanInterrupt`/`HumanResponse` schemas, or document the external dependency contract explicitly.
- Consider warning (rather than erroring) pathways for budget-style limits that offer `Command(resume="extend")`-style human continuation as an alternative to `GraphRecursionError`.

## Questions / Gaps

- No evidence found of any built-in cost/token/budget-based review trigger; searched `libs/` for `risk`, `approval`, `dangerous`, `auto_approve`, budget terms — all hits were tests or user-space examples.
- No evidence found of identity/attribution capture for human responses within the checkpoint layer (no responder field in `CheckpointMetadata`, `studies/agent-harness-study/sources/langgraph/libs/checkpoint/langgraph/checkpoint/base/__init__.py:38-86`).
- Documentation boundary: the repo ships no narrative HITL concept docs (only `docs/llms.txt` redirect artifacts and one example notebook, `examples/human_in_the_loop/wait-user-input.ipynb`); behavioral claims here rest on implementation and tests, not prose documentation.
- The exact production deployment story for decision auditing presumably lives in LangGraph Server/platform code, which is outside this source tree; within this repository, auditability is limited to what the checkpointer persists.

---

Generated by `14.01-human-in-the-loop-trigger-policy` against `langgraph`.
