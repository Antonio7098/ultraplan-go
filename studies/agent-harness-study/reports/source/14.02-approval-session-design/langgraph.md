# Source Analysis: langgraph

## Dimension 14.02: Approval Session Design

### Source Info

| Field | Value |
|-------|-------|
| Name | langgraph |
| Path | `studies/agent-harness-study/sources/langgraph` |
| Language / Stack | Python (core `langgraph`, `checkpoint*`, `prebuilt`, `sdk-py`); JS/TS SDK stub only |
| Analyzed | 2026-08-26 |

## Summary

LangGraph does not have a first-class "approval session" object. Instead, approval is modeled as a **durable interrupt/resume primitive** built on the checkpointer. A node calls `interrupt(value)` (`studies/agent-harness-study/sources/langgraph/libs/langgraph/langgraph/types.py:851`), which raises a `GraphInterrupt` carrying an `Interrupt` payload (`value` + deterministic `id`, `libs/langgraph/langgraph/langgraph/types.py:573-618`). The executor commits the interrupt as a pending write under the reserved channel key `__interrupt__` into the checkpointer (`libs/langgraph/langgraph/pregel/_runner.py:585-591`; reserved keys at `libs/langgraph/langgraph/_internal/_constants.py:7-12`). The run ends; the human decision arrives later as `Command(resume=...)` (`libs/langgraph/langgraph/langgraph/types.py:798-848`), which is itself persisted as a `__resume__` pending write (`libs/langgraph/langgraph/pregel/_io.py:74-75`) before the node re-executes from its start and consumes resume values positionally (`libs/langgraph/langgraph/langgraph/types.py:950-965`).

Because the interrupt *and* the resume are both rows in the checkpoint write log (e.g., Postgres table `checkpoint_writes`, `libs/checkpoint-postgres/langgraph/checkpoint/postgres/base.py:66-76,146-159`), an approval session is fully durable: it survives process restarts and arbitrary client disconnects — the "browser refresh" test passes by construction. Scoping is per-task/per-interrupt-id (multiple simultaneous interrupts require an id→value resume map, enforced with a hard error). There is **no timeout/expiry** on a pending approval anywhere in the codebase, and **no dedicated audit log**: auditing is implicit in checkpoint writes plus optional lifecycle callbacks (`GraphInterruptEvent`/`GraphResumeEvent`). A prebuilt schema layer (`HumanInterruptConfig` / `ActionRequest` / `HumanResponse`, `libs/prebuilt/langgraph/prebuilt/interrupt.py:11-105`) defines what response types (accept/edit/respond/ignore) a UI may offer per request, but it is a UI contract, not an enforcement mechanism.

## Rating

**7 / 10** — Clear model with explicit interfaces, deterministic interrupt IDs, durable persistence across swappable backends (memory/SQLite/Postgres), conformance-tested resume semantics including subgraphs and time-travel. Kept from 8–9 by three gaps: no expiry/TTL on pending approvals (a stale approval can be honored arbitrarily late), no actor identity or timestamps on individual resume writes (audit trail is structural, not forensic), and response-type scoping (`HumanInterruptConfig`) that is advisory-only with no server-side validation.

## Evidence Collected

Every entry cites workspace-relative paths with line numbers.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Approval request handler | `interrupt(value)` raises resumable `GraphInterrupt` on first call in a node; returns resume value on re-execution | `studies/agent-harness-study/sources/langgraph/libs/langgraph/langgraph/types.py:851-974` |
| Interrupt payload type | `Interrupt` dataclass with `value` + `id`; `id` deprecated alias `interrupt_id` removed in v0.6 | `studies/agent-harness-study/sources/langgraph/libs/langgraph/langgraph/types.py:573-628` |
| Deterministic interrupt ID | `Interrupt.from_ns` = xxh3_128 hexdigest of checkpoint namespace → stable across restarts/processes | `studies/agent-harness-study/sources/langgraph/libs/langgraph/langgraph/types.py:616-618` |
| Resume primitive | `Command(resume=...)`: single value or mapping of interrupt ids → values | `studies/agent-harness-study/sources/langgraph/libs/langgraph/langgraph/types.py:808-824` |
| Exception types | `GraphInterrupt`; deprecated `NodeInterrupt` superseded by `interrupt()` | `studies/agent-harness-study/sources/langgraph/libs/langgraph/langgraph/errors.py:102-126` |
| Static (declarative) interrupts | `compile(interrupt_before=..., interrupt_after=...)` by node name or `"*"` | `studies/agent-harness-study/sources/langgraph/libs/langgraph/langgraph/graph/state.py:1183-1264` |
| Per-invocation override | `invoke/stream(..., interrupt_before=..., interrupt_after=...)` parameters on Pregel entry points | `studies/agent-harness-study/sources/langgraph/libs/langgraph/langgraph/pregel/main.py:2550-2646` |
| Static interrupt evaluation | `should_interrupt()` compares channel versions vs versions seen at last interrupt; loop sets status `interrupt_before`/`interrupt_after` | `studies/agent-harness-study/sources/langgraph/libs/langgraph/langgraph/pregel/_algo.py:155-185`; `.../pregel/_loop.py:667-672,720-723` |
| Persisting the request | Runner commits `(INTERRUPT, interrupts)` write to checkpointer on `GraphInterrupt` | `studies/agent-harness-study/sources/langgraph/libs/langgraph/langgraph/pregel/_runner.py:585-591` |
| Reserved write channels | `__interrupt__` / `__resume__` interned constants | `studies/agent-harness-study/sources/langgraph/libs/langgraph/langgraph/_internal/_constants.py:7-12` |
| Persisting the answer | `Command.resume` mapped to `(NULL_TASK_ID, RESUME, value)` write, saved via `put_writes` | `studies/agent-harness-study/sources/langgraph/libs/langgraph/langgraph/pregel/_io.py:74-75`; `.../pregel/_loop.py:902-931` |
| Resume matching logic | Scratchpad maps positional interrupt index → resume value list; null-resume consumption rules | `studies/agent-harness-study/sources/langgraph/libs/langgraph/langgraph/pregel/_algo.py:1280-1345`; `.../langgraph/types.py:950-965` |
| Checkpointer required | Docstring mandate + runtime error `Cannot use Command(resume=...) without checkpointer` | `studies/agent-harness-study/sources/langgraph/libs/langgraph/langgraph/types.py:870-871`; `.../pregel/_loop.py:905-908` |
| Durable storage (Postgres) | `checkpoint_writes(thread_id, checkpoint_ns, checkpoint_id, task_id, idx, channel, type, blob)`; UPSERT keyed on task_id+idx | `studies/agent-harness-study/sources/langgraph/libs/checkpoint-postgres/langgraph/checkpoint/postgres/base.py:66-76,146-159` |
| Checkpointer API | `BaseCheckpointSaver.put_writes(config, writes, task_id)`; `CheckpointTuple.pending_writes` surfaced to readers | `studies/agent-harness-study/sources/langgraph/libs/checkpoint/langgraph/checkpoint/base/__init__.py:300-318,139-146` |
| Pending interrupts readable | `StateSnapshot.interrupts` — "Interrupts ... that are pending resolution" | `studies/agent-harness-study/sources/langgraph/libs/langgraph/langgraph/types.py:683-701` |
| Multi-interrupt scoping | Singular resume rejected when >1 pending interrupt; must pass id-keyed map (`CONFIG_KEY_RESUME_MAP`) | `studies/agent-harness-study/sources/langgraph/libs/langgraph/langgraph/pregel/_loop.py:910-920`; `.../_internal/_constants.py:72-73` |
| Per-task scope of resumes | Resume list "scoped to the specific task executing the node and is not shared across tasks" | `studies/agent-harness-study/sources/langgraph/libs/langgraph/langgraph/types.py:864-868` |
| Timeout policy exists but is node-level | `TimeoutPolicy(run_timeout/idle_timeout)` bounds a node attempt, not human wait time; `NodeTimeoutError` same | `studies/agent-harness-study/sources/langgraph/libs/langgraph/langgraph/types.py:451-481`; `.../langgraph/errors.py:190-241` |
| No expiry on approvals | Only TTL found is Store KV TTL (`ttl` minutes); nothing expires checkpoints/writes/interrupts; removal only via `delete_thread` | `studies/agent-harness-study/sources/langgraph/libs/checkpoint/langgraph/store/base/__init__.py:526-573`; `.../checkpoint-postgres/langgraph/checkpoint/postgres/__init__.py:381-402` |
| Audit-adjacent callbacks | `GraphInterruptEvent(checkpoint_id, ns, status, interrupts)` / `GraphResumeEvent`, dispatched via callback manager | `studies/agent-harness-study/sources/langgraph/libs/langgraph/langgraph/callbacks.py:31-79` |
| Callback emission points | Loop pushes lifecycle events on resume (`_first`) and on interrupt suppression (`_suppress_interrupt`) | `studies/agent-harness-study/sources/langgraph/libs/langgraph/langgraph/pregel/_loop.py:376-408,1078,1339` |
| Stream surfacing of requests | `__interrupt__` emitted in `updates`/`values` stream modes when interrupt writes commit | `studies/agent-harness-study/sources/langgraph/libs/langgraph/langgraph/pregel/_loop.py:1424-1451` |
| Server-facing schema | SDK `Interrupt{value,id}`; `Thread.status` includes `'interrupted'`; `Thread.interrupts` map task_id→list[Interrupt] | `studies/agent-harness-study/sources/langgraph/libs/sdk-py/langgraph_sdk/schema.py:291-316` |
| Prebuilt approval schemas | `HumanInterruptConfig(allow_ignore/respond/edit/accept)`, `ActionRequest(action,args)`, `HumanInterrupt`, `HumanResponse(type∈{accept,ignore,response,edit})` (deprecated shims moved to `langchain.agents.interrupt`) | `studies/agent-harness-study/sources/langgraph/libs/prebuilt/langgraph/prebuilt/interrupt.py:11-105` |
| Agent Inbox integration example | README shows building an approval request from a tool call and dispatching on `response['type']` | `studies/agent-harness-study/sources/langgraph/libs/prebuilt/README.md:120-150` |
| Durability tests (fresh invocation) | `test_stateless_interrupt_resume`: interrupt in one invoke, resume in a later invoke via parent checkpointer | `studies/agent-harness-study/sources/langgraph/libs/langgraph/tests/test_subgraph_persistence.py:30-82` |
| Multi-interrupt scoping test | `test_null_resume_disallowed_with_multiple_interrupts`: parallel interrupts require id-keyed resume map | `studies/agent-harness-study/sources/langgraph/libs/langgraph/tests/test_pregel.py:8906-8951` |
| Callback behavior tests | `test_graph_callbacks_interrupt_and_resume_sync/_async` | `studies/agent-harness-study/sources/langgraph/libs/langgraph/tests/test_graph_callbacks.py:106-135` |

## Answers to Dimension Questions

**1. How is approval requested?**
Two mechanisms. (a) *Dynamic*: a node calls `interrupt(value)`; the first call in a task raises `GraphInterrupt` with the payload surfaced to the client (`libs/langgraph/langgraph/langgraph/types.py:851-974`), committed as an `__interrupt__` pending write by the runner (`libs/langgraph/langgraph/pregel/_runner.py:585-591`). (b) *Static*: `compile(interrupt_before=[...], interrupt_after=[...])` or per-call overrides pause before/after named nodes (or `"*"`) without any node cooperation (`libs/langgraph/langgraph/graph/state.py:1183-1264`; evaluation in `libs/langgraph/langgraph/langgraph/pregel/_algo.py:155-185`; enforcement in `libs/langgraph/langgraph/pregel/_loop.py:667-672,720-723`). The prebuilt package layers an opinionated request envelope (`HumanInterrupt` with `action_request` + allowed-response config) intended for inbox-style UIs (`libs/prebuilt/langgraph/prebuilt/interrupt.py:51-84`).

**2. Are approval sessions durable?**
Yes — this is the design's center of gravity. A checkpointer is mandatory (`RuntimeError` otherwise, `libs/langgraph/langgraph/pregel/_loop.py:905-908`), the interrupt and the eventual resume are both persisted as pending writes (`__interrupt__` / `__resume__`, `libs/langgraph/langgraph/pregel/_runner.py:585-591`; `libs/langgraph/langgraph/pregel/_io.py:74-75`) in backend tables like `checkpoint_writes` (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/base.py:66-76`). Interrupt IDs are content-derived (xxh3_128 of the checkpoint namespace, `libs/langgraph/langgraph/langgraph/types.py:616-618`), so resume-by-id works from any later process. Tests demonstrate pausing in one invocation and resuming in a completely separate one (`libs/langgraph/tests/test_subgraph_persistence.py:30-82`), including stateless subgraphs inheriting the parent's checkpointer. Edge cases around stale state are handled explicitly: time-travel drops cached RESUME writes so interrupts re-fire (`_loop.py:874-900`) and forks clear stale INTERRUPT writes (`_loop.py:964-971`).

**3. Can approvals be scoped?**
Partially. Scope units that exist: (i) *per-interrupt resolution* — each `Interrupt.id` can be answered independently, and with multiple concurrent interrupts a singular resume is rejected outright; callers must send `{interrupt_id: value}` (`_loop.py:910-920`, verified by `libs/langgraph/tests/test_pregel.py:8906-8951`); (ii) *per-task isolation* — resume lists are scoped to the executing task, not shared (`types.py:864-868`); (iii) *per-node static scoping* — interrupt lists or `"*"` (`state.py:1183-1217`); (iv) *per-request response affordances* — `allow_accept/allow_edit/allow_respond/allow_ignore` flags declare what the UI may offer (`libs/prebuilt/langgraph/prebuilt/interrupt.py:11-26`). What does **not** exist: "approve all future actions", persistent allowlists/denylists, role-based approver constraints, or server-side validation that the returned `HumanResponse.type` was actually permitted by the request's `config`. The flags are advisory metadata for UIs.

**4. Do approvals time out?**
No evidence of any timeout, TTL, or expiry for pending approvals. Exhaustive search over `timeout`, `expir`, `ttl` in the core and checkpoint libraries finds: node-execution timeouts (`TimeoutPolicy`, `libs/langgraph/langgraph/langgraph/types.py:451-481`; `NodeTimeoutError`, `libs/langgraph/langgraph/langgraph/errors.py:190-241`) which bound a running attempt, not a paused graph; cache-entry TTL (`CacheKey.ttl`, `types.py:655-663`); and Store KV TTL (`libs/checkpoint/langgraph/store/base/__init__.py:526-573`) — none apply to checkpoint writes. A pending approval lives until resumed or until `delete_thread` wipes the thread (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/__init__.py:381-402`). Consequence: an approval prompt shown days later is still valid, and there is no staleness signal a client could use to expire it.

**5. Are approvals audited?**
Implicitly and partially. Every request and every answer is durably recorded — the interrupt write and resume write land in `checkpoint_writes` rows keyed by thread/namespace/checkpoint/task/index (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/base.py:66-76,146-159`), and checkpoints carry `ts`, `source`, `step`, `run_id`, and parent links (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:38-62,92-103`), so a full reconstruction of who-was-asked-what-and-what-was-answered is possible from storage. For live observability there are typed lifecycle events — `GraphInterruptEvent` (with checkpoint id, namespace, status, payloads) and `GraphResumeEvent` (`libs/langgraph/langgraph/callbacks.py:42-79`), emitted by the loop at `_loop.py:1078` and `_loop.py:1339`, with sync/async coverage in `libs/langgraph/tests/test_graph_callbacks.py:106-135` — and `__interrupt__` stream chunks (`_loop.py:1424-1451`). Missing for real audit purposes: no actor/user identity on resume writes, no timestamp column on `checkpoint_writes` rows (only checkpoint-level `ts`), no tamper-evident append-only guarantee beyond ordinary DB semantics, and no dedicated approval-log API.

## Architectural Decisions

1. **Approvals as control-plane data, not sessions.** Rather than an approval-session object with its own store, LangGraph folds approval requests and responses into the checkpoint write log as reserved channels `__interrupt__`/`__resume__` (`libs/langgraph/langgraph/_internal/_constants.py:7-12`). This buys durability, replayability, and time-travel for free, but means approval-specific concerns (identity, expiry) have nowhere natural to live.
2. **Deterministic, content-derived interrupt IDs.** `Interrupt.from_ns` hashes the checkpoint namespace (`types.py:616-618`), making IDs stable across processes and enabling the multi-interrupt resume map protocol (`CONFIG_KEY_RESUME_MAP`, `_constants.py:72-73`; applied per-subgraph-namespace at `libs/langgraph/langgraph/pregel/_algo.py:1311-1314`).
3. **Re-execute-on-resume semantics.** On resume the whole node re-runs from the top and prior `interrupt()` calls replay their recorded answers positionally (`types.py:862-868` docstring contract; implementation `types.py:950-965` + scratchpad `_algo.py:1280-1345`). Side effects before the interrupt must be idempotent — a documented but user-enforced constraint.
4. **Positional vs addressed resume duality.** A bare `Command(resume=v)` targets the single next interrupt; a dict of xxh3-hexdigest keys addresses specific ones, validated by key shape (`_loop.py:910-914`) and rejected outright if ambiguous (`_loop.py:916-920`).
5. **UI contracts kept outside the engine.** The accept/edit/respond/ignore vocabulary lives in `prebuilt` (now deprecated shims pointing to `langchain.agents.interrupt`, `libs/prebuilt/langgraph/prebuilt/interrupt.py:7-10`), keeping the core generic (`Any` payloads) while giving inbox products a standard shape (`libs/prebuilt/README.md:120-150`).

## Notable Patterns

- **Bubble-up exception protocol:** `GraphInterrupt` derives from `GraphBubbleUp` (`errors.py:50-51,102-107`); the runner treats it as non-failure for sibling cancellation (`_should_stop_others`, `libs/langgraph/langgraph/pregel/_runner.py:616-634`) and commits it as writes rather than errors.
- **Subgraph transparency:** nested graphs inherit the parent checkpointer just enough to pause/resume even when compiled stateless (`libs/langgraph/tests/test_subgraph_persistence.py:30-82`); interrupt IDs remain unique per namespace because they hash `checkpoint_ns`.
- **Null-task writes:** resume values with no specific target go to `NULL_TASK_ID` (`00000000-...`, `_constants.py:93-94`) and are consumed once (`get_null_resume(consume=True)`, `_algo.py:1320-1331`), preventing double-application.
- **Run-result surface:** both streaming (`stream/run_stream.py:193-206`) and snapshot APIs expose `interrupted()`/`interrupts`, so clients can render pending approvals after reconnecting.
- **Thread status as session state:** the server-facing `Thread.status` enum includes `'interrupted'` with a task-id→interrupt map (`libs/sdk-py/langgraph_sdk/schema.py:300-316`) — effectively the durable approval queue for UIs.

## Tradeoffs

- **Durability over ergonomics:** requiring a checkpointer for any interrupt (`types.py:870-871`) makes trivial approval flows heavier to set up, but guarantees the browser-refresh survival property.
- **Generality over safety:** `interrupt(Any)` lets teams invent ad-hoc payload shapes; without the prebuilt envelope there's no schema, and even with it, response-type allow-listing is not enforced server-side.
- **Simplicity over hygiene:** no expiry keeps the engine tiny, pushes staleness policy entirely to applications, and risks zombie approvals being honored long after context changed.
- **Positional matching is compact but brittle-ish:** order-based matching of multiple `interrupt()` calls within one node (`types.py:866-868`) is concise, but reordering calls changes meaning; cross-task cases avoid this via explicit IDs.

## Failure Modes / Edge Cases

- **Zombie approvals:** a pending interrupt never expires; combined with long-lived threads, an operator could approve a stale action. Mitigation is application-side only.
- **Ambiguous singular resume:** invoking with a scalar resume while >1 interrupts pend raises `RuntimeError` (`_loop.py:916-920`) — loud failure rather than misdirected approval (tested, `test_pregel.py:8938-8946`).
- **Time-travel staleness:** replaying from an older checkpoint would otherwise return stale resume values; the loop detects replay and strips cached RESUME writes (`_loop.py:874-900`) and clears orphaned INTERRUPT writes on fork (`_loop.py:964-971`).
- **Side-effect duplication:** because nodes re-execute from the start on resume (`types.py:862-868`), non-idempotent operations before `interrupt()` run twice.
- **Deprecated dual APIs:** `NodeInterrupt` (`errors.py:110-126`) and prebuilt `HumanInterrupt*` shims still exist alongside `interrupt()`/`langchain.agents.interrupt`, risking divergent usage patterns.

## Future Considerations

- Add optional expiry (TTL or deadline) to pending interrupts — e.g., a deadline field on `Interrupt` checked during resume — to close the stale-approval hole.
- Record responder identity and wall-clock timestamps on resume writes (schema extension to `checkpoint_writes` or a metadata side-table) to upgrade the implicit audit trail into a real one.
- Enforce `HumanResponse.type` against the request's `HumanInterruptConfig` server-side, turning UI affordance flags into authorization.
- Provide "approve-all"/policy-based auto-approval primitives (scoped allowlists per tool/node) for agents that currently must hand-roll batching across many interrupts.

## Questions / Gaps

- **No evidence found** for any approval timeout/expiry mechanism; searched `timeout`, `expir`, `ttl` across `libs/langgraph`, `libs/checkpoint*` — all hits concern node execution, caches, or the KV Store, not checkpoints/writes.
- **No evidence found** for identity/attribution of approvals: neither the core loop nor the checkpointer schema records *who* supplied a resume value; the Python SDK `Command`/resume surface carries no principal field.
- **Server-side validation gap unverified beyond source:** whether LangGraph Server (closed-source deployment) enforces `HumanInterruptConfig` cannot be determined from this repository; the OSS libraries clearly do not.
- The `libs/sdk-js` directory contains only a README in this snapshot, so JS-side approval ergonomics could not be assessed.
- Documentation in-repo (`docs/`) holds generated indexes only; behavioral claims here rest on implementation and tests, not prose docs.

---

Generated by dimension 14.02 (Approval Session Design) against `langgraph`.
