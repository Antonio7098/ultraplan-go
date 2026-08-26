# Source Analysis: agent-framework

## Dimension 05.02: Working Memory and Scratchpad

### Source Info

| Field | Value |
|-------|-------|
| Name | agent-framework |
| Path | `studies/agent-harness-study/sources/agent-framework` |
| Language / Stack | Python (`python/packages/core`) and .NET/C# (`dotnet/src/Microsoft.Agents.AI*`) monorepo; `go/` contains only a placeholder README |
| Analyzed | 2026-08-26 |

## Summary

Agent Framework implements working memory as a set of layered, provider-scoped state stores rather than a single scratchpad object. The substrate is the mutable, serializable `AgentSession.state` dict (`studies/agent-harness-study/sources/agent-framework/python/packages/core/agent_framework/_sessions.py:1750`), partitioned by each context provider's `source_id`. On top of it, the harness ships five distinct working-memory mechanisms: (1) a todo list (`TodoProvider`, `studies/agent-harness-study/sources/agent-framework/python/packages/core/agent_framework/_harness/_todo.py:446`), (2) an operating-mode flag (`AgentModeProvider`, `studies/agent-harness-study/sources/agent-framework/python/packages/core/agent_framework/_harness/_mode.py:197`), (3) a transient queue of injected messages drained at the next model call (`MessageInjectionMiddleware`, `studies/agent-harness-study/sources/agent-framework/python/packages/core/agent_framework/_sessions.py:1383`), (4) a per-loop progress log maintained by `AgentLoopMiddleware` outside the session (`studies/agent-harness-study/sources/agent-framework/python/packages/core/agent_framework/_harness/_loop.py:515,731-745`), and (5) session-scoped file-based "working memory" that survives compaction (`FileMemoryProvider`, `studies/agent-harness-study/sources/agent-framework/python/packages/core/agent_framework/_harness/_file_memory.py:220`). A sixth layer — durable long-term memory (`MemoryContextProvider`) — is explicitly separated from scratch state by prompt-level extraction rules that exclude "transient tasks, temporary reminders" (`studies/agent-harness-study/sources/agent-framework/python/packages/core/agent_framework/_harness/_memory.py:44-56`). The .NET implementation mirrors this design with typed state records stored in a thread-safe `AgentSessionStateBag` (`studies/agent-harness-study/sources/agent-framework/dotnet/src/Microsoft.Agents.AI.Abstractions/AgentSessionStateBag.cs:21`).

Working memory is visible to the model (injected as user-role messages), to the runtime (provider tools read/write it), and to users only through explicit opt-in surfaces (harness slash commands, samples) — it is not silently exposed. Durability is pluggable: volatile in-session by default, file-backed or session-store-backed when configured. Clearing at task boundaries is partial: queued messages drain automatically, mode-change notifications are consumed once, loop iterations can reset sessions under `fresh_context`, but todo lists persist until the model removes items under advisory instructions.

## Rating

**8 / 10** — Clear model with tests, explicit interfaces, and operational safeguards.

Rationale:
- **Explicit interfaces**: abstract `TodoStore` (`_todo.py:228-242`) with two implementations (`TodoSessionStore` `_todo.py:245-284`, `TodoFileStore` `_todo.py:288-443`); public helpers `get_agent_mode`/`set_agent_mode` (`_mode.py:118-194`); exported public API surface (`python/packages/core/agent_framework/__init__.pyi:97-114`).
- **Tests covering failure modes**: atomic-write crash safety (`python/packages/core/tests/core/test_harness_todo.py:130`), malformed-state rejection (`test_harness_todo.py:152,162`), ID-collision clamping (`test_harness_todo.py:180`), lock eviction on GC (`test_harness_todo.py:200`), path traversal (`test_harness_todo.py:214`), concurrent mutation serialization (`test_harness_todo.py:322`), fresh-context/session-reset matrix (`test_harness_loop.py:530-541`), consolidation-failure state preservation (`test_harness_memory.py:799`).
- **Operational safeguards**: atomic temp-file + `os.replace` writes (`_todo.py:431-443`; `_memory.py:123-136`), per-session asyncio locks via `WeakKeyDictionary` (`_todo.py:481-491`), encoded filesystem path segments with traversal checks (`_todo.py:344-394`).
- Not 9–10 because: todo cleanup at topic/task boundaries is instruction-driven, not enforced by the runtime (`_todo.py:39-40`); the scratch-vs-durable-memory boundary is enforced only by prompts, not structurally (`_memory.py:50-51`); several components remain experimental (`test_harness_memory.py:665-672`; `TodoFileStore` stays experimental per `test_harness_todo.py:369-377`); and there is no dedicated audit log of scratchpad mutations beyond general history providers and feature-usage telemetry.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Session state substrate | `AgentSession.state` is a plain mutable dict shared by all providers; serialized via `to_dict`/`from_dict` | `studies/agent-harness-study/sources/agent-framework/python/packages/core/agent_framework/_sessions.py:1717-1791` |
| Todo item model | `TodoItem(id, title, description, is_complete)` with strict `from_dict` validation | `studies/agent-harness-study/sources/agent-framework/python/packages/core/agent_framework/_harness/_todo.py:51-106` |
| Todo default storage | `TodoSessionStore` keeps `{items, next_id}` inside `session.state[source_id]` | `studies/agent-harness-study/sources/agent-framework/python/packages/core/agent_framework/_harness/_todo.py:245-284` |
| Durable todo option | `TodoFileStore`: one JSON file per session+source_id, atomic sibling-temp write, base-root escape check | `studies/agent-harness-study/sources/agent-framework/python/packages/core/agent_framework/_harness/_todo.py:288-443` (atomic write 431-443; containment check 358-360) |
| Model-visible todo injection | `before_run` injects instructions, five `todos_*` tools (`approval_mode="never_require"`), and the current list as a user-role message | `studies/agent-harness-study/sources/agent-framework/python/packages/core/agent_framework/_harness/_todo.py:505-615` (list message 596-615) |
| Concurrency control | Per-session `asyncio.Lock` in `WeakKeyDictionary` (evicted on session GC); read-modify-write under lock | `studies/agent-harness-study/sources/agent-framework/python/packages/core/agent_framework/_harness/_todo.py:481-491,511,540,569` |
| Boundary clearing (advisory) | Default instructions tell the model to clear/update todos when the user changes topic — no runtime enforcement | `studies/agent-harness-study/sources/agent-framework/python/packages/core/agent_framework/_harness/_todo.py:39-40` |
| Mode as hidden state | Current mode + one-shot "previous mode" notification marker live in provider-scoped session state; marker popped after single injection | `studies/agent-harness-study/sources/agent-framework/python/packages/core/agent_framework/_harness/_mode.py:73,186-194,284-287` |
| Plan persistence guidance | Plan-mode instructions direct the model to write plans to a memory file so they survive compaction | `studies/agent-harness-study/sources/agent-framework/python/packages/core/agent_framework/_harness/_mode.py:49-50` |
| Transient injection queue | `enqueue_messages` appends to `session.state[MESSAGE_INJECTION_PENDING_MESSAGES_STATE_KEY]`; middleware drains and clears on next model call | `studies/agent-harness-study/sources/agent-framework/python/packages/core/agent_framework/_sessions.py:1364-1380,1419-1429` |
| Harness wiring | `create_harness_agent` adds TodoProvider/AgentModeProvider unless disabled, FileMemoryProvider by default rooted at `{cwd}/agent-file-memory`, and MessageInjectionMiddleware unconditionally | `studies/agent-harness-study/sources/agent-framework/python/packages/core/agent_framework/_harness/_agent.py:176-187,657` |
| Loop progress log | Internal `progress: list[str]` accumulated per iteration; injected as `"Progress so far:"` user message; callbacks get defensive copies | `studies/agent-harness-study/sources/agent-framework/python/packages/core/agent_framework/_harness/_loop.py:515,571,724-745,798-802,827-842` |
| In-loop state discard | `fresh_context=True` snapshots the session pre-loop (`to_dict()`) and restores it between iterations; continuity carried only by the progress log | `studies/agent-harness-study/sources/agent-framework/python/packages/core/agent_framework/_harness/_loop.py:431-475` (snapshot 436, restore 461-475) |
| Todo-driven looping | `todos_remaining` predicate reads open items from the running agent's `TodoProvider`; `todos_remaining_message` lists them | `studies/agent-harness-study/sources/agent-framework/python/packages/core/agent_framework/_harness/_loop.py:925-986,989-1021` |
| HITL boundary | Loop stops before continuing when a pending `function_approval_request` is present | `studies/agent-harness-study/sources/agent-framework/python/packages/core/agent_framework/_harness/_loop.py:442-459` |
| File working memory | Session-scoped flat file space; internal files (`*_description.md`, `memories.md` index) hidden from listing/grep/write; capped 50-entry index injected as user message | `studies/agent-harness-study/sources/agent-framework/python/packages/core/agent_framework/_harness/_file_memory.py:60-74,93-99,280-299,501-524` |
| Scratch vs durable boundary | Extraction prompt: "include only durable facts… do not include transient tasks, temporary reminders, one-off outputs, or tool chatter"; consolidation drops stale/transient items | `studies/agent-harness-study/sources/agent-framework/python/packages/core/agent_framework/_harness/_memory.py:44-68` |
| Memory pipeline | `after_run` persists transcripts then extracts memories from the turn delta; transient extractor errors skip, programmer errors propagate | `studies/agent-harness-study/sources/agent-framework/python/packages/core/agent_framework/_harness/_memory.py:1348-1396,1420-1431` |
| Compaction of history | `CompactionProvider.after_run` rewrites stored history in session state; excluded messages retained with annotations; deferred to end of turn (`after_run_once_per_turn`) so it does not rewrite mid-task working context | `studies/agent-harness-study/sources/agent-framework/python/packages/core/agent_framework/_compaction.py:1532-1534,1592-1621` |
| Background task registry | Serializable `{next_task_id, tasks}` in session state plus non-serializable `_RuntimeState` (in-flight asyncio tasks); tasks marked `LOST` when in-flight entry is gone (e.g., restart) | `studies/agent-harness-study/sources/agent-framework/python/packages/core/agent_framework/_harness/_background_agents.py:56-111,114-128,189-211,245-249` |
| Injected-message audit trail | `SessionContext.extend_messages` copies messages and stamps `_attribution` (`source_id`, `source_type`, optional cross-session `origin_session_ids`) for governance/audit | `studies/agent-harness-study/sources/agent-framework/python/packages/core/agent_framework/_sessions.py:624-690` |
| Control-plane filtering | Resolved approval request/response contents filtered out of later model replay while unresolved ones are preserved | `studies/agent-harness-study/sources/agent-framework/python/packages/core/agent_framework/_sessions.py:873-940` |
| .NET state bag | Thread-safe `ConcurrentDictionary` key-value store with JSON serialize/deserialize of all values | `studies/agent-harness-study/sources/agent-framework/dotnet/src/Microsoft.Agents.AI.Abstractions/AgentSessionStateBag.cs:21-145` |
| .NET todo/mode/background state | Typed `TodoState {Items, NextId}`, `AgentModeState {CurrentMode, PreviousModeForNotification}`, `BackgroundAgentState {NextTaskId, Tasks}` in the state bag | `studies/agent-harness-study/sources/agent-framework/dotnet/src/Microsoft.Agents.AI/Harness/Todo/TodoState.cs:12-25`; `.../AgentMode/AgentModeState.cs:10-24`; `.../BackgroundAgents/BackgroundAgentState.cs:15-28` |
| .NET todo provider | `ProviderSessionState<TodoState>` keyed state, per-session semaphore locking, `SuppressTodoListMessage` opt-out of list injection | `studies/agent-harness-study/sources/agent-framework/dotnet/src/Microsoft.Agents.AI/Harness/Todo/TodoProvider.cs:22-28,81-85` |
| .NET loop reset | `FreshContextPerIteration` snapshots caller session JSON pre-run and restores between iterations | `studies/agent-harness-study/sources/agent-framework/dotnet/src/Microsoft.Agents.AI/Harness/Loop/LoopAgent.cs:40-45,150-154` |
| User visibility surface | Harness console exposes `/todos`, `/mode`, `/session-export`; sample prints evolving todo list read directly from the store | `studies/agent-harness-study/sources/agent-framework/python/samples/02-agents/harness/console/commands/todo_handler.py:33-52`; `.../console/commands/__init__.py:5`; `.../context_providers/todo_provider.py:37-43` |

## Answers to Dimension Questions

1. **Does the agent keep private task state?** Yes, extensively. Todo items, operating mode, background-task metadata, queued injection messages, and conversation history all live in `AgentSession.state`, partitioned per provider `source_id` (`_sessions.py:1750`; `_todo.py:250-253`; `_mode.py:76-87`; `_background_agents.py:189-196`; `_sessions.py:1364-1380`). The loop's progress log is private middleware state, deliberately copied before handing to callbacks so user code cannot mutate it (`_loop.py:724-725`). FileMemoryProvider provides a second, file-backed private workspace whose internal files (description sidecars, `memories.md` index) are hidden from the agent's own discovery tools (`_file_memory.py:93-99`).

2. **Is it durable?** Configurable per store. Default `TodoSessionStore` and mode state survive as long as the `AgentSession` object and its serialization (`to_dict`/`from_dict`, `_sessions.py:1757-1791`) persist; the experimental `SessionStore`/`FileSessionStore` add snapshot durability (`_sessions.py:1794+`); `TodoFileStore` persists to atomically-written JSON files (`_todo.py:419-443`); file memory persists under `{cwd}/agent-file-memory` by default (`_harness/_agent.py:183-187`). Non-durable by design: the injection queue (consumed on the next model call) and the loop's in-flight background-task handles — persisted entries whose runtime handle vanished are demoted to `LOST` rather than silently dropped (`_background_agents.py:245-249`).

3. **Is it exposed to users?** To the *model*, yes — todo list, mode changes, memory index, and progress are injected as user-role messages each run (`_todo.py:596-615`; `_mode.py:315-325`; `_file_memory.py:509-524`; `_loop.py:798-802`). To the *end user*, only via explicit opt-in surfaces: harness console slash commands (`/todos`, `/mode`, `/session-export`) and samples that read the store directly. The framework itself does not stream scratch state into user-facing responses, and .NET offers `SuppressTodoListMessage` to suppress even model-side injection (`dotnet/.../Todo/TodoProvider.cs:81`).

4. **Does it pollute long-term memory?** Guarded, but only at prompt level. The `MemoryContextProvider` extraction prompt forbids promoting transient tasks/reminders/tool chatter into durable topics (`_memory.py:50-51`), and consolidation drops stale/transient items (`_memory.py:63-65`). However, full transcripts (including tool chatter) are still persisted to a `transcripts` store before extraction (`_memory.py:1359-1366`), and there is no structural mechanism preventing scratch content embedded in assistant text from being extracted. Working memory does not write to long-term memory implicitly — promotion requires the LLM extraction step.

5. **Can it be audited?** Partially, by design. Every provider-injected message carries `_attribution` metadata identifying the injecting source and any contributing cross-session IDs, explicitly intended for governance/audit (`_sessions.py:636-660`); feature usage is recorded via `mark_feature_used` telemetry hooks (`_todo.py:502`; `_mode.py:276`); `HistoryProvider` documents an audit-only configuration (store without replay, `_sessions.py:946-949`) and approval control-plane contents may be retained for audit by history providers (`packages/core/AGENTS.md`, Tool Approval section). There is no dedicated, tamper-evident scratchpad change log; auditing todo/mode mutations means diffing session snapshots (e.g., via `/session-export`).

## Architectural Decisions

- **Provider-partitioned shared state instead of a scratchpad object.** Each context provider owns a namespace (`source_id`) inside one serializable dict (`_sessions.py:1750`, `_todo.py:250-253`), making working memory composable and independently swappable (e.g., swap `TodoSessionStore` for `TodoFileStore` via constructor arg, `_todo.py:462-480`).
- **Scratch state enters the model only through declared injection points.** Providers push instructions/tools/messages through `SessionContext.extend_*` during `before_run` (`_todo.py:591-615`), keeping working notes distinguishable from genuine user input via role and attribution metadata.
- **Typed state records on .NET, validated dicts on Python.** .NET models scratch state as internal typed classes (`TodoState.cs:12-25`); Python validates raw dicts on load with precise corruption errors (`_todo.py:87-95,193-202`).
- **Durability behind abstract stores.** `TodoStore` ABC (`_todo.py:228-242`) and `AgentFileStore` abstraction let hosts choose volatility vs persistence without changing tool semantics.
- **Separate pipelines for scratch (session/file state) and durable memory (LLM-extracted topic files)**, joined only by an explicitly instructed extraction step (`_memory.py:1398-1441`).

## Notable Patterns

- **One-shot notification markers**: externally triggered mode changes record the prior mode in state and pop it after exactly one injection, so the model sees the switch once (`_mode.py:186-194,284-287`).
- **Snapshot-and-restore loop isolation**: `fresh_context=True` discards in-loop transcript/state changes each iteration while preserving the pre-loop baseline and carrying continuity solely in the injected progress log (`_loop.py:431-475`; verified by `tests/core/test_harness_loop.py:454-541`).
- **Defensive copies everywhere**: injected messages are copied before attribution stamping (`_sessions.py:670-676`), progress logs handed to callbacks as copies (`_loop.py:724-725`), and store reads return independent deep copies (`_sessions.py:1840-1841`).
- **Atomic state writes**: temp-file + `os.replace` pattern reused across todo, memory, and file stores to survive crashes mid-write (`_todo.py:431-443`; `_memory.py:123-136`; tested at `test_harness_todo.py:130`, `test_harness_memory.py:695`).
- **Failure-status modeling over silence**: background tasks transition to `FAILED`/`LOST` with error text persisted in state (`_background_agents.py:214-257`).
- **Cross-implementation parity**: Python `todos_remaining` is documented as the counterpart of .NET `TodoCompletionLoopEvaluator` (`_loop.py:931-932`), with matching tool names ("Align TodoProvider tool names with the C# implementation", `python/CHANGELOG.md:443`).

## Tradeoffs

- **Shared mutable dict vs type safety**: the Python `state` dict is maximally flexible but relies on runtime validation; a non-dict value under a provider key raises only when accessed (`_todo.py:256-259`, `_mode.py:81-84`). .NET pays upfront typing cost for compile-time safety.
- **Prompt-enforced hygiene vs structural guarantees**: relying on the extraction prompt keeps the architecture simple but means a misbehaving extractor can promote scratch noise into durable memory.
- **User-role injections vs system-role purity**: injecting todo lists/memory indexes as `user` messages makes them salient to the model but blurs provenance in transcripts (mitigated by `_attribution` metadata, which downstream consumers must know to look for).
- **Default-on file memory**: `create_harness_agent` creates `{cwd}/agent-file-memory` on disk by default (`_harness/_agent.py:183-187`) — convenient persistence, but hosts must be aware of incidental local writes containing potentially sensitive session content.
- **Advisory todo lifecycle**: no automatic clearing avoids destroying state the user may want back, but shifts correctness onto model compliance with instructions (`_todo.py:39-40`).

## Failure Modes / Edge Cases

- **Crash-mid-write corruption**: mitigated by atomic replace so a truncated `todos.json` cannot break subsequent tool calls (`_todo.py:435-437`; test `test_harness_todo.py:130`).
- **Corrupted/malformed state**: strict validation raises descriptive errors instead of silent resets (`_todo.py:87-95,256-275`; tests `test_harness_todo.py:152-178`); corrupt memory index skips injection for one run and self-heals on next write (`_file_memory.py:501-508`).
- **ID collisions after restore**: `next_id` is clamped above any persisted id (`_todo.py:223-225`; test `test_harness_todo.py:180`).
- **Path escape via hostile ids**: session/source/owner segments are sanitized or base64-encoded; traversal and Windows-reserved names rejected (`_todo.py:318-394`; tests `test_harness_todo.py:214`, `test_harness_memory.py:216`).
- **Concurrent mutations**: duplicate-ID and lost-update races serialized by per-session locks (`_todo.py:481-491,511`; test `test_harness_todo.py:322`); lock map uses weak references so long-running services don't leak locks per session (`_todo.py:481-483`).
- **Process restart with pending work**: background task handles vanish → statuses become `LOST`, surfaced to the model rather than hanging forever (`_background_agents.py:245-249`); loops pairing `background_tasks_running` with `max_iterations` bound this (`_loop.py:866-884`).
- **Runaway loops**: hard `max_iterations` cap evaluated before `should_continue`, and pending-approval responses halt the loop to await human input (`_loop.py:747-758,442-459`).
- **Unbounded growth**: todo list has no size cap; memory index caps at 50 entries (`_file_memory.py:78`), durable index at 200 lines/150 chars (`_memory.py:36-37`) — but session-state todo growth is the host's problem.

## Future Considerations

- Enforce scratchpad hygiene structurally: e.g., optional TTL or auto-archive for completed/stale todos at turn boundaries, complementing the advisory instructions.
- Add a first-class audit hook (event log) for scratchpad mutations (add/complete/remove/mode-set) rather than relying on snapshot diffs and general history providers.
- Consider structural filters (e.g., excluding tool-chatter spans) before feeding the memory extraction prompt, reducing reliance on prompt compliance (`_memory.py:1408` currently formats the raw input+response delta).
- Graduate remaining experimental pieces (`TodoFileStore`, `MemoryContextProvider`, background agents, looping) to stabilize the durability story end-to-end (`test_harness_todo.py:369-377`; `test_harness_memory.py:665-672`).

## Questions / Gaps

- **No evidence found** for automatic, runtime-enforced clearing of todo/mode state at task boundaries; searched for lifecycle/clear/expiry logic in `_harness/_todo.py`, `_harness/_mode.py`, and harness wiring (`_harness/_agent.py`) — clearing exists only as model instructions and manual tool calls (`todos_remove`), plus one-shot consumption of the mode-notification marker.
- **No evidence found** for redaction or sensitivity screening of content written into scratchpad stores (todo descriptions, file-memory files, background-task result text are stored verbatim); searches for PII/redact/secret handling in the harness modules returned nothing beyond generic path-safety guards.
- **No evidence found** for distributed/multi-process coordination of session state (locks are in-process `asyncio.Lock`/`SemaphoreSlim`; `SessionStore` explicitly delegates TTL/distributed concerns to custom implementations, `_sessions.py:1798-1807`).
- DevUI exposure of scratchpad internals was not examined in depth; the audited user-visibility surfaces are the console-sample slash commands and samples listed above.

---

Generated by dimension `05.02-working-memory-and-scratchpad` against `agent-framework`.
