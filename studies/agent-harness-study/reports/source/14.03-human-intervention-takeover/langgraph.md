# Source Analysis: langgraph

## Dimension 14.03: Human Intervention and Takeover

### Source Info

| Field | Value |
|-------|-------|
| Name | langgraph |
| Path | `studies/agent-harness-study/sources/langgraph` |
| Language / Stack | Python (core, prebuilt, checkpointers, sdk-py); JS/TS (sdk-js — submodule not checked out) |
| Analyzed | 2026-08-26 |

## Summary

LangGraph implements human intervention as a first-class, checkpoint-mediated protocol rather than an ad-hoc callback. The core primitive is `interrupt()` (`libs/langgraph/langgraph/types.py:851`), which raises a `GraphInterrupt` carrying a typed `Interrupt` value that is persisted to the checkpointer as a task-scoped write (`libs/langgraph/langgraph/langgraph/pregel/_runner.py:584-591`). Humans respond by re-invoking the graph with `Command(resume=...)` (`libs/langgraph/langgraph/langgraph/types.py:798-848`), which the Pregel loop converts into pending writes keyed by task ID or interrupt ID (`libs/langgraph/langgraph/langgraph/pregel/_loop.py:902-931`). Direct state surgery is exposed through `update_state` / `bulk_update_state` with `as_node` attribution (`libs/langgraph/langgraph/langgraph/pregel/main.py:2515-2541`, `main.py:1590-2054`), and fork/time-travel is built on `get_state_history` plus `update_state` against a historical checkpoint, producing checkpoints tagged `"source": "fork"` / `"source": "update"` (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:41`). At the deployment layer, the Python SDK exposes run cancellation with `interrupt`/`rollback` actions, double-texting multitask strategies (`interrupt`, `rollback`, `enqueue`, `reject`), and mid-stream protocol commands (`libs/sdk-py/langgraph_sdk/_sync/runs.py:925-981`, `libs/sdk-py/langgraph_sdk/schema.py:81-85`, `libs/sdk-py/langgraph_sdk/stream/transport/http.py:64`).

The model is "pause at a durable boundary, edit, resume" — a human can correct a run without restarting it, but cannot inject values into an actively executing step; feedback lands at superstep/interrupt boundaries.

## Rating

**9 / 10.**

Rationale:

- **Clear model**: one canonical mechanism (`interrupt()` → checkpointed interrupt → `Command(resume=...)`) documented exhaustively in docstrings with runnable examples (`libs/langgraph/langgraph/langgraph/types.py:873-940`).
- **Explicit interfaces**: the whole surface is on the public protocol — `get_state`, `get_state_history`, `bulk_update_state`, `update_state`, and `Command`-accepting `stream/invoke` overloads (`libs/langgraph/langgraph/langgraph/pregel/protocol.py:48-105`, `protocol.py:107-231`).
- **Tests**: dedicated time-travel suite (~3,966 lines) covering forks, sibling branches, subgraphs up to three levels deep, and replay/refire semantics (`libs/langgraph/tests/test_time_travel.py:143`, `:182`, `:2530`, `:2149`); multi-interrupt resume maps (`libs/langgraph/tests/test_pregel.py:6193-6266`); parallel-interrupt fan-out (`test_pregel.py:7577`).
- **Operational safeguards**: deterministic interrupt IDs derived from namespace hashes (`types.py:612-618`), explicit errors when resuming without a checkpointer (`_loop.py:905-908`) or when multiple interrupts lack IDs (`_loop.py:916-920`), and stale-write cleanup on forks (`_loop.py:964-970`).
- Not a 10 because: human edits carry no author identity in checkpoint metadata (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:38-62` has `source`/`step`/`parents`/`run_id` only); feedback requires a boundary crossing (no injection into a live step); and node bodies re-execute from their start on resume (`types.py:864`), pushing idempotency burden onto user code.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| State edit API (protocol) | `update_state(config, values, as_node)` and async variant defined on the graph protocol | libs/langgraph/langgraph/pregel/protocol.py:92-105 |
| State edit API (impl) | `update_state` delegates to `bulk_update_state`; requires checkpointer (`"No checkpointer set"`) | libs/langgraph/langgraph/pregel/main.py:2515-2526, main.py:1616-1617 |
| Bulk edits | Multiple supersteps of `(values, as_node, task_id)` updates applied sequentially | libs/langgraph/langgraph/pregel/main.py:1590-1609, main.py:2049-2054 |
| `as_node` attribution | Update applied "as if from node X"; ambiguity raises `InvalidUpdateError("Ambiguous update, specify as_node")` | libs/langgraph/langgraph/pregel/main.py:1905-1938 |
| Clear pending tasks | `update_state(..., None, as_node=END)` clears interrupted/pending tasks | libs/langgraph/langgraph/pregel/main.py:1678-1744 |
| Reducer bypass for humans/nodes | `Overwrite` wrapper writes directly to channel, survives JSON round-trip via discriminator field | libs/langgraph/langgraph/langgraph/types.py:977-1024 |
| Interrupt primitive | `interrupt(value)` tracks per-task index, returns prior resume values, else raises `GraphInterrupt` | libs/langgraph/langgraph/langgraph/types.py:851-974 |
| Interrupt identity | `Interrupt.id` = xxh3_128 hash of checkpoint namespace; usable to resume directly | libs/langgraph/langgraph/langgraph/types.py:593-618 |
| Resume input | `Command(resume=...)`: single value or `{interrupt_id: value}` map | libs/langgraph/langgraph/langgraph/types.py:808-824 |
| Resume plumbing | Loop maps Command to writes; dict-of-hex-id resumes stored in `CONFIG_KEY_RESUME_MAP`; multi-interrupt null-resume rejected | libs/langgraph/langgraph/langgraph/pregel/_loop.py:902-931 |
| Resume resolution | Scratchpad resolves global (NULL_TASK_ID), task-specific, and namespace-mapped resume values | libs/langgraph/langgraph/langgraph/pregel/_algo.py:1280-1345 |
| Multi-interrupt test | Fan-out of 5 child graphs; resume via `{interrupt.id: answer}` map | libs/langgraph/tests/test_pregel.py:6193-6266 |
| Static interrupts | `interrupt_before`/`interrupt_after` compile args; runtime overrides per call | libs/langgraph/langgraph/graph/state.py:1183-1184, 1257-1264; libs/langgraph/langgraph/pregel/protocol.py:116-117 |
| Static interrupt gating | `should_interrupt` requires channel updates since previous interrupt to avoid loops | libs/langgraph/langgraph/langgraph/pregel/_algo.py:155-183 |
| Interrupt persistence | `commit()` saves `(INTERRUPT, ...)` writes plus any consumed RESUME writes to checkpointer | libs/langgraph/langgraph/langgraph/pregel/_runner.py:574-591 |
| Interrupts surfaced to clients | `RunStream.interrupted` / `.interrupts` properties; SDK `Interrupt{value,id}` schema; thread/task-level interrupt lists | libs/langgraph/langgraph/stream/run_stream.py:193-215; libs/sdk-py/langgraph_sdk/schema.py:291-297, 333-352 |
| Structured HITL payloads | `HumanInterruptConfig` (allow_ignore/respond/edit/accept), `ActionRequest`, `HumanResponse` (accept/ignore/response/edit) — deprecated shim moved to `langchain.agents.interrupt` | libs/prebuilt/langgraph/prebuilt/interrupt.py:11-105 |
| Fork API | `update_state` against a historical config creates branch; `as_node="__copy__"` clones without changes, metadata `source="fork"` | libs/langgraph/langgraph/pregel/main.py:1800-1878 |
| Time-travel fork safety | On replay from older checkpoint, loop persists `{"source": "fork"}` checkpoint and clears stale INTERRUPT/RESUME writes | libs/langgraph/langgraph/langgraph/pregel/_loop.py:874-900, 952-971 |
| History API | `get_state_history(config, filter, before, limit)` over checkpoint lineage | libs/langgraph/langgraph/pregel/protocol.py:58-65; impl main.py:1480 |
| Fork tests | Fork reruns downstream node with modified state; two forks from same checkpoint are independent siblings | libs/langgraph/tests/test_time_travel.py:143-179, 182-218 |
| Copy-fork refire test | `__copy__` before an interrupt re-fires it; new resume value accepted | libs/langgraph/tests/test_time_travel.py:2530-2579 |
| Cross-thread copy | `copy_thread(source_thread_id)` copies all checkpoints/writes between threads | libs/checkpoint/langgraph/checkpoint/base/__init__.py:352-355 |
| Audit trail: source tag | `CheckpointMetadata.source ∈ {"input","loop","update","fork"}` distinguishes human edits/forks from loop checkpoints | libs/checkpoint/langgraph/checkpoint/base/__init__.py:38-55 |
| Audit trail: lineage + run | `parents` map and `run_id` recorded per checkpoint | libs/checkpoint/langgraph/checkpoint/base/__init__.py:56-62 |
| Audit trail: writes | `put_writes(writes, task_id, task_path)` stores intermediate writes linked to a checkpoint; loop dedupes and persists them | libs/checkpoint/langgraph/checkpoint/base/__init__.py:300-313; libs/langgraph/langgraph/pregel/_loop.py:415-508 |
| Task-ID stability | `update_state` reuses would-be task IDs so history/results stay coherent after edits | libs/langgraph/langgraph/pregel/main.py:1880-1903 |
| Server-side takeover | `runs.cancel(action="interrupt"\|"rollback")`; `MultitaskStrategy = reject\|interrupt\|rollback\|enqueue` for double-texting | libs/sdk-py/langgraph_sdk/_sync/runs.py:925-981; libs/sdk-py/langgraph_sdk/schema.py:81-85, 137-140 |
| Server state editing | SDK `threads.update_state(thread_id, values, as_node, checkpoint)` → POST `/threads/{id}/state` | libs/sdk-py/langgraph_sdk/_sync/threads.py:611-672 |
| Resume over HTTP | `Runs.stream/create(command=..., checkpoint=..., checkpoint_id=...)` | libs/sdk-py/langgraph_sdk/_sync/runs.py:74-86, 195-217 |
| Mid-stream commands | v3 protocol transport POSTs client commands to `/threads/{thread_id}/commands` during a live stream | libs/sdk-py/langgraph_sdk/stream/transport/http.py:36, 64-93 |
| Cooperative drain | `RunControl.request_drain()` stops run cooperatively at superstep; `GraphDrained` saves checkpoint, resumable later | libs/langgraph/langgraph/runtime.py:79-104; libs/langgraph/langgraph/errors.py:54-64 |
| Dynamic interrupt behavior | Conditional `interrupt()` inside retried node is not re-executed by retry policy | libs/langgraph/tests/test_large_cases.py:4147-4177 |

## Answers to Dimension Questions

### 1. Can humans edit agent state?

Yes. `Pregel.update_state` / `aupdate_state` apply arbitrary values through the graph's channels as if a named node had produced them (`as_node`), auto-derived when unambiguous and raising `InvalidUpdateError` when ambiguous (`libs/langgraph/langgraph/langgraph/pregel/main.py:1905-1937`). `bulk_update_state` applies sequences of supersteps atomically per superstep (`main.py:1590-1623`). Edits respect reducers by default; `Overwrite` bypasses them (`types.py:977-1024`). Special modes exist: clearing pending tasks (`as_node=END`, `main.py:1678`), injecting fresh input (`as_node=INPUT`, `main.py:1747`), and cloning a checkpoint (`as_node="__copy__"`, `main.py:1800`). The same operations are reachable remotely via the server SDK (`libs/sdk-py/langgraph_sdk/_sync/threads.py:611-672`).

### 2. Can humans provide mid-run feedback?

At interruption boundaries, yes; into a live step, no. A node calls `interrupt(payload)`; execution halts durably (interrupt persisted via `commit()`, `_runner.py:584-591`) and the stream ends with `__interrupt__`. The human's response arrives as `Command(resume=value_or_map)` on the next invocation (`_loop.py:902-931`), matched either positionally per task (`_algo.py:1300-1314`) or by interrupt-ID map (`_loop.py:910-914`). Parallel/multiple interrupts require ID-keyed resumes — a bare value raises `RuntimeError` when more than one interrupt is pending (`_loop.py:916-920`; tested at `test_pregel.py:6193-6266`). There is no API that mutates state under a currently executing step; the closest remote capabilities are double-texting strategies (`multitask_strategy="interrupt"/"rollback"/"enqueue"/"reject"`, `schema.py:81-85`) and mid-stream protocol commands (`stream/transport/http.py:64`).

### 3. Can humans take over execution?

There is no sandbox abstraction in this repository ("No evidence found" for sandbox takeover; searched for `sandbox`, `takeover`, `container` concepts in `libs/langgraph` and `libs/sdk-py`). Takeover is expressed as: (a) cancel a run with `action="interrupt"` (stop now, state kept) or `"rollback"` (`runs.py:925-981`, semantics declared in `schema.py:137-140`); (b) cooperative drain at superstep boundaries for graceful shutdown, leaving a resumable checkpoint (`runtime.py:79-104`, `errors.py:54-64`); (c) fork execution from any historical checkpoint with edited state (`main.py:1800-1878`, `_loop.py:952-971`). Prebuilt agent tool approval flows (`HumanInterrupt`/`HumanResponse` with accept/edit/ignore/respond actions) existed here but were migrated to `langchain.agents.interrupt` (`libs/prebuilt/langgraph/prebuilt/interrupt.py:7-10`).

### 4. Are human interventions traceable?

Structurally, yes; by identity, no. Every intervention produces a new checkpoint whose metadata records `source` (`"update"` for manual edits, `"fork"` for clones/replays), `step`, parent lineage, and the creating `run_id` (`base/__init__.py:38-62`); tests assert these tags (`test_time_travel.py:2582-2617`). Task-scoped writes (including interrupts and resume markers) are persisted via `checkpointer.put_writes` with explicit `task_id`/`task_path` (`base/__init__.py:300-313`), and `update_state` deliberately reuses the would-be task IDs so edited histories stay coherent (`main.py:1880-1903`). Full timelines are walkable via `get_state_history` (`protocol.py:58-65`). However, `CheckpointMetadata` has no author/user field — attribution is to nodes/tasks/runs, not to the human who intervened. Authentication context exists separately in `ServerInfo.user` (`runtime.py:70-76`) but is not stamped onto checkpoints.

## Architectural Decisions

1. **Interventions ride the persistence layer, not a side channel.** Interrupts, resume values, and manual edits are all checkpoint writes (`_runner.py:584-591`, `main.py:2005-2007`). Consequence: any intervention is durable, crash-safe, and replayable, but impossible without a checkpointer (`_loop.py:905-908`, `main.py:1616-1617`).
2. **Resume = deterministic re-entry, not stack restoration.** On resume the node re-runs from its start and `interrupt()` returns the stored resume value positionally (`types.py:864`, `types.py:955-965`). This keeps the engine simple and serializable at the cost of re-executing side effects.
3. **Stable, content-derived interrupt identities.** `Interrupt.id` is an xxh3_128 hash of the checkpoint namespace (`types.py:612-618`), so clients can hold references across processes and resume specific interrupts out of order.
4. **Forks are ordinary checkpoints with provenance tags.** Rather than a separate branching API, `source: "fork"`/`"update"` metadata plus parent configs give immutable lineages (`base/__init__.py:41-60`, `main.py:1812-1824`), letting multiple sibling branches coexist (tested at `test_time_travel.py:182-218`).
5. **Static and dynamic interrupts share one gate.** Compile-time `interrupt_before/after` and runtime `interrupt()` both flow through `should_interrupt`/INTERRUPT channel logic (`_algo.py:155-183`), avoiding two divergent pause mechanisms.

## Notable Patterns

- **Scratchpad-based resume matching** (`_algo.py:1280-1345`): a per-task scratchpad layers three resume sources — global null-task resume, task-specific resume, and namespace-hash-mapped resume — so nested subgraphs inherit the right values.
- **Write-deduplication with last-write-wins for special channels** (`_loop.py:420-437`) prevents repeated resumes from accumulating stale entries.
- **Time-travel hygiene** (`_loop.py:874-900`, `964-971`): cached RESUME writes are dropped when replaying so interrupts re-fire, while active resumes keep them for multi-interrupt flows; fork checkpoints are emitted eagerly so a replay that hits an interrupt still branches correctly.
- **Deprecated-shim migration**: HITL TypedDicts remain importable from prebuilt but forward users to `langchain.agents.interrupt` (`interrupt.py:7-10`, `29-32`, `47-50`), showing the payload vocabulary (approve/edit/respond/ignore) is being productized upstream.
- **Client-facing interrupt projection**: streaming surfaces `.interrupted`/`.interrupts` (`run_stream.py:193-215`) and the SDK models interrupts at thread, task, and state levels (`schema.py:315-316`, `327`, `351-352`).

## Tradeoffs

- **Durability vs. liveness**: feedback always crosses a persistence boundary; you cannot nudge a running LLM call. Long-running tools need internal `interrupt()` calls or the drain mechanism (`errors.py:54-64`) to become responsive to humans.
- **Re-execution semantics vs. simplicity**: because nodes rerun on resume (`types.py:864`), non-idempotent side effects must be guarded (e.g., using `StreamWriter`/checkpoint checks). Tests confirm retry policies intentionally do not re-fire interrupts (`test_large_cases.py:4176`).
- **Positional vs. addressed resume**: single-value resume is ergonomic but forbidden with multiple pending interrupts (`_loop.py:916-920`); the ID-map form is verbose but race-free.
- **`None` is not a valid resume value** because it is indistinguishable from "missing" over HTTP (`_algo.py:1296-1298`) — a deliberate interoperability-over-expressiveness choice.
- **Edit power vs. safety**: `update_state(as_node=...)` executes only the node's writers, not its body (`main.py:1953-1971`), so edits bypass validation logic that lives in node code; reducers and channel validation are the only guardrails.

## Failure Modes / Edge Cases

- Resuming without a checkpointer fails fast with `RuntimeError` (`_loop.py:905-908`).
- Ambiguous `as_node` inference raises `InvalidUpdateError` instead of guessing (`main.py:1934-1935`).
- Replaying from an old checkpoint could consume stale cached resumes; the loop strips RESUME-tagged writes when time-traveling (`_loop.py:874-900`) and clears stale INTERRUPT writes on fork (`_loop.py:964-970`) — comments document a real bug this prevents ("subsequent resumes load the wrong state").
- Empty `Command` inputs raise `EmptyInputError` (`_loop.py:927-928`).
- Durability mode `"exit"` skips intermediate write persistence (`_loop.py:466`), weakening mid-run intervention guarantees if a crash occurs before exit.
- Multiple forks from one checkpoint create independent siblings; neither sees the other's writes (`test_time_travel.py:182-218`).

## Future Considerations

- Stamp actor identity onto interventions (extend `CheckpointMetadata`) to close the audit-trail gap between structural traceability and "who did this".
- Expose richer approval-policy primitives in-repo once the `langchain.agents.interrupt` migration stabilizes (`interrupt.py:8`), so core docs/tests can cover the structured accept/edit/respond contract end-to-end.
- Consider a first-class "inject into live run" facility layered on the v3 protocol command channel (`stream/transport/http.py:36`) if interactive steering beyond superstep boundaries becomes a requirement.

## Questions / Gaps

- No evidence found for user-attributed edit logs: searched `CheckpointMetadata` fields (`base/__init__.py:38-86`) and `update_state` call paths; nothing records the intervening principal.
- `libs/sdk-js` contains only a README in this checkout (submodule not populated), so JS client parity for `command=`/`checkpoint=` parameters could not be verified.
- In-repo documentation of HITL is docstring-only: `docs/` holds redirects (`docs/llms.txt`, `docs/redirects.json`); no markdown HITL guide exists in-tree to cite.
- Sandbox takeover: no sandbox/container/VM abstraction exists anywhere in this source; the dimension's "sandbox takeover" question is answered by cancel/drain/fork semantics above.

---

Generated by `dimension 14.03-human-intervention-and-takeover` against `langgraph`.
