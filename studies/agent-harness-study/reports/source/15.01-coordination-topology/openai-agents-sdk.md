# Source Analysis: openai-agents-sdk

## Dimension 15.01: Coordination Topology

### Source Info

| Field | Value |
|-------|-------|
| Name | openai-agents-sdk |
| Path | `studies/agent-harness-study/sources/openai-agents-sdk` |
| Language / Stack | Python (asyncio, pydantic, httpx; OpenAI Responses/Chat Completions APIs) |
| Analyzed | 2026-08-25 |

All citations below are relative to the source root `studies/agent-harness-study/sources/openai-agents-sdk/`.

## Summary

The OpenAI Agents SDK implements a **centralized, single-process supervisor/delegation topology over a static, developer-declared agent graph**. There is no peer-to-peer messaging, no message bus, no blackboard, and no runtime discovery protocol (searched for `blackboard`, `message_bus`, `pubsub`, `agent_registry`, `peer` across `src/` — only incidental matches in `src/agents/_tool_identity.py:388` and `src/agents/run_internal/approvals.py:4`). All inter-agent coordination flows through exactly two LLM-driven mechanisms plus one code-driven mechanism:

1. **Handoffs (control transfer)** — a handoff is surfaced to the model as an ordinary function tool named `transfer_to_<agent_name>` (`src/agents/handoffs/__init__.py:207-211`, confirmed in docs at `docs/handoffs.md:5`). When the active agent emits that tool call, the central runner invokes the handoff (`src/agents/run_internal/turn_resolution.py:584-586`), produces a `HandoffOutputItem` recording source and target agents (`src/agents/run_internal/turn_resolution.py:599-607`), and replaces the run's active agent with the target (`NextStepHandoff`, `src/agents/run_internal/run_steps.py:156-157`; swap sites in `src/agents/run_internal/run_loop.py:1390-1412` streaming and `src/agents/run.py:2089-2091` non-streaming). The receiving agent takes over the conversation for the rest of the run.
2. **Agents-as-tools (subtask delegation)** — `Agent.as_tool()` wraps an agent as a `FunctionTool`; the calling agent keeps control and the callee runs as a nested, bounded sub-run whose final output returns as a tool result (`src/agents/agent.py:583-634`). The docstring explicitly contrasts the two models: handoffs transfer conversation ownership; tools keep it with the caller (`src/agents/agent.py:608-612`).
3. **Code orchestration (deterministic)** — developers chain or parallelize whole runs themselves via structured outputs, output chaining, evaluator loops, and `asyncio.gather` (`docs/multi_agent.md:43-52`; working example at `examples/agent_patterns/parallelization.py:29-43`).

Communication content is exclusively (a) shared conversation-history items passed through `HandoffInputData` (`src/agents/handoffs/__init__.py:70-115`), optionally compacted into a nested `<CONVERSATION HISTORY>` summary by `nest_handoff_history` (`src/agents/handoffs/history.py:83-94`, summary builder at `src/agents/handoffs/history.py:376-398`); (b) tool-call JSON payloads (handoff input schema via `input_type`, validated strict JSON, `src/agents/handoffs/__init__.py:313-343`); and (c) a shared mutable context object (`RunContextWrapper.context`) visible to every participant. The only experimental alternative — `OpenAIHostedMultiAgentModel` — moves orchestration server-side behind a WebSocket beta (`responses_multi_agent=v1`, root agent `/root`, `src/agents/extensions/experimental/hosted_multi_agent/model.py:44-45`) with a configurable `max_concurrent_subagents` cap (`model.py:76-86`), and explicitly rejects combining with SDK handoffs (`model.py:396-400`).

**Topology of a multi-agent run** (handoff edges = control transfer; tool edges = bounded subtask calls):

```
                         ┌──────────────────────────────────────────────────┐
                         │        Runner (centralized, one while-loop)      │
                         │   current_agent := starting_agent                │
                         │   max N turns (DEFAULT_MAX_TURNS=10)             │
                         └───────┬──────────────────────────────────▲───────┘
                                 │ model turn                       │ NextStepHandoff(new_agent)
                                 ▼                                  │
                     ┌────────────────────┐   transfer_to_billing    ┌──────────────────┐
        user input ──▶│  Triage Agent      │─────────────────────────▶│ Billing Agent    │
                     │  (supervisor)      │   HandoffOutputItem      │  (now active)    │
                     └──────┬─────────────┘   {assistant:"Billing"}  └────────┬─────────┘
                            │ as_tool()                                       │ transfer_to_refund
                            │ (nested sub-run, control retained)              ▼
                            ▼                                        ┌──────────────────┐
                   ┌─────────────────┐                               │ Refund Agent     │
                   │ Research Agent  │──final output──▶ back to Triage└──────────────────┘
                   └─────────────────┘
   Edges are declared statically via Agent.handoffs / Agent.tools; is_enabled can hide/show
   edges per turn; no runtime edge creation, no direct agent↔agent channels.
```

## Rating

**8 / 10**

Rationale against the rubric:

- **Clear model with explicit interfaces**: handoff vs. agents-as-tool semantics are precisely documented and enforced in code (`src/agents/agent.py:608-612`, `docs/multi_agent.md:22-31`); the `Handoff` dataclass, `HandoffInputData`, input filters, and history mappers are public, typed extension points (`src/agents/handoffs/__init__.py:125-218`).
- **Tests**: extensive coverage of topology behavior — handoff parsing and next-step routing (`tests/test_run_step_processing.py:172`, `tests/test_run_step_execution.py:2871`), multiple-handoff arbitration (`tests/test_run_step_processing.py:321`, `tests/test_run_step_execution.py:4085`), missing-handoff failure (`tests/test_run_step_processing.py:303`), nested-history provenance across repeated handoffs and resume (`tests/test_handoff_history_duplication.py:2279`, `tests/test_handoff_history_duplication.py:1758`), tracing of agent spans (`tests/test_tracing_errors_streamed.py:398`).
- **Operational safeguards**: multiple simultaneous handoffs collapse to the first with the rest ignored and an error recorded on the span (`src/agents/run_internal/turn_resolution.py:563-597`); `MaxTurnsExceeded` bounds runaway delegation (`src/agents/run_config.py:45`, `src/agents/run_internal/run_loop.py:1542-1552`); handoff tool names reserved against MCP namespacing collisions (`src/agents/agent.py:216-235`, tested in `tests/mcp/test_runner_calls_mcp.py:262`); server-managed conversations disable filters/nesting with warnings rather than corrupting state (`src/agents/run_internal/turn_resolution.py:495-524`).
- **Observable**: dedicated `handoff_span(from_agent, to_agent)` (`src/agents/run_internal/turn_resolution.py:578-597`), agent spans listing available handoffs (`src/agents/tracing/create.py:91-103`), and stream events `handoff_requested`/`handoff_occured` plus `AgentUpdatedStreamEvent` on every switch (`src/agents/run_internal/streaming.py:37-39`, `src/agents/stream_events.py:52-55`).

Why not 9–10: execution within a run is strictly sequential — exactly one active agent per turn, no parallel sub-agents inside the runner (parallelism exists only when user code gathers independent runs). Topology is fixed at construction except for `is_enabled` gating; there is no dynamic rewiring, no inter-agent messaging primitive, and no discovery protocol. The hosted multi-agent model that would add concurrency is explicitly experimental/beta and mutually exclusive with SDK handoffs (`src/agents/extensions/experimental/hosted_multi_agent/model.py:396-400`).

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Communication channel: handoff-as-tool | Handoff exposed to the LLM as `transfer_to_<agent>` tool with generated name/description | `src/agents/handoffs/__init__.py:207-218` |
| Communication channel: history payload | `HandoffInputData` carries input history, pre-handoff items, new items, run context to the next agent | `src/agents/handoffs/__init__.py:70-115` |
| Communication channel: nested history summary | Default mapper compacts prior transcript into one assistant message wrapped in `<CONVERSATION HISTORY>` markers | `src/agents/handoffs/history.py:29-38`, `src/agents/handoffs/history.py:376-398`, `src/agents/handoffs/history.py:311-317` |
| Topology definition (static edges) | `Agent.handoffs: list[Agent \| Handoff]` declared at construction; plain `Agent` entries auto-wrapped into handoffs | `src/agents/agent.py:331-335`, `src/agents/run_internal/turn_preparation.py:96-103` |
| Central coordinator | Single runner loop owns `current_agent`; handoff result swaps it and continues the same loop | `src/agents/run_internal/run_loop.py:1184`, `src/agents/run_internal/run_loop.py:1390-1412`, `src/agents/run.py:2089-2091` |
| Control-transfer decision | Model tool call routed to handoff pipeline when bare name matches a handoff tool | `src/agents/run_internal/turn_resolution.py:206-213` |
| Handoff execution + transfer record | `execute_handoffs` invokes target resolver, emits `HandoffOutputItem(source_agent, target_agent)` and `on_handoff` hooks | `src/agents/run_internal/turn_resolution.py:527-628` |
| Multiple-handoff arbitration | Only first handoff executes; extras get "Multiple handoffs detected, ignoring this one." outputs and span error | `src/agents/run_internal/turn_resolution.py:563-597` |
| Agents-as-tool channel | `as_tool()` nests a full sub-run; caller retains control; output extracted as tool result | `src/agents/agent.py:583-634` |
| Dynamic edge gating | `Handoff.is_enabled` bool-or-callable evaluated each turn via cancelled-gather; disabled handoffs hidden from LLM | `src/agents/handoffs/__init__.py:185-193`, `src/agents/run_internal/turn_preparation.py:105-115` |
| Destination binding | `handoff()` helper always returns the captured agent; dynamic destination requires custom `Handoff.on_invoke_handoff` | `src/agents/handoffs/__init__.py:278-280`, `src/agents/handoffs/__init__.py:343` |
| Target identity kept weakly | `_agent_ref: weakref.ReferenceType[AgentBase]` avoids strong reference cycles between graph nodes | `src/agents/handoffs/__init__.py:195-198`, `src/agents/handoffs/__init__.py:373` |
| Static topology visualization | Graphviz DOT generator walks `tools`/`mcp_servers`/`handoffs` edges recursively from a root agent | `src/agents/extensions/visualization.py:32-57`, `src/agents/extensions/visualization.py:146-168` |
| Resume-time re-discovery | `_build_handoffs_map` maps handoff tool names → definitions from the current agent for RunState restore | `src/agents/run_state.py:2864-2880` |
| Last-active-agent tracking | `RunResult.last_agent` (weakref-backed) lets callers continue multi-turn runs from the final specialist | `src/agents/result.py:381-388`, `src/agents/result.py:522-533`; consumed by REPL `src/agents/repl.py:75` and voice workflow `src/agents/voice/workflow.py:77-101` |
| Streaming observability | `handoff_requested` / `handoff_occured` item events; `AgentUpdatedStreamEvent(new_agent)` on switch | `src/agents/run_internal/streaming.py:37-39`, `src/agents/stream_events.py:52-55` |
| Tracing of transfers | `handoff_span(from_agent)` records `to_agent` and "Multiple handoffs requested" errors | `src/agents/run_internal/turn_resolution.py:578-597`; agent span lists candidate handoffs `src/agents/tracing/create.py:91-103` |
| Realtime surface parity | Same handoff model over realtime sessions: collect/filter enabled handoffs per `RealtimeAgent` | `src/agents/realtime/handoffs.py:35-69`, field at `src/agents/realtime/agent.py:66` |
| Hosted alternative (experimental) | Server-side orchestration beta with root agent `/root`, `max_concurrent_subagents`, rejects SDK handoffs | `src/agents/extensions/experimental/hosted_multi_agent/model.py:44-45`, `model.py:76-86`, `model.py:396-400` |
| Code orchestration escape hatch | Parallelization via `asyncio.gather` over independent `Runner.run` calls under one trace | `examples/agent_patterns/parallelization.py:29-57`; patterns doc `docs/multi_agent.md:43-52` |
| Loop bound | `DEFAULT_MAX_TURNS = 10`; exceeded turns raise `MaxTurnsExceeded` | `src/agents/run_config.py:45`, `src/agents/run_internal/run_loop.py:1542-1552` |
| Guardrail scoping follows topology | Input guardrails run "only if the agent is the first agent in the chain" | `src/agents/agent.py:350-353` |
| Tool-name collision safeguard | MCP reserved-name snapshot includes enabled handoff tool names to avoid namespace clashes | `src/agents/agent.py:99-101`, `src/agents/agent.py:216-235` |

## Answers to Dimension Questions

1. **How do agents coordinate?**
   Through the central runner, never directly. Coordination takes three forms: (a) *handoffs* — the active agent's model emits a `transfer_to_<name>` tool call; the runner resolves it to an `Agent`, appends a `HandoffOutputItem` carrying `{"assistant": "<name>"}` (`src/agents/handoffs/__init__.py:203-204`), fires `on_handoff` hooks, and makes the target the sole active agent for subsequent turns (`src/agents/run_internal/turn_resolution.py:577-628`); (b) *agents-as-tools* — a nested `Runner` invocation whose final output is returned as a tool result while the parent stays active (`src/agents/agent.py:606-612`); (c) *shared state* — the `RunContextWrapper.context` object and session items are common to all participants, and history handed to the successor is either forwarded verbatim or compacted into a numbered `<CONVERSATION HISTORY>` summary (`src/agents/handoffs/history.py:379-398`). Data passed at handoff time beyond history is limited to the strict-JSON `input_type` payload delivered to `on_handoff` (`src/agents/handoffs/__init__.py:316-341`).

2. **Is the topology fixed or dynamic?**
   Fixed by construction, with narrow runtime gating. Edges come solely from developer-declared `Agent.handoffs` / `Agent.tools` lists (`src/agents/agent.py:331-335`, `src/agents/agent.py:194-195`). At runtime, `is_enabled` predicates (bool or ctx-dependent callable) decide which edges are offered to the model each turn (`src/agents/run_internal/turn_preparation.py:105-115`), but no code path adds, removes, or retargets edges mid-run: `handoff()` closes over one destination and its invoker always returns that captured agent (`src/agents/handoffs/__init__.py:278-280`, `:343`). A custom `Handoff` could return different agents per invocation (the interface returns `Awaitable[TAgent]`, `src/agents/handoffs/__init__.py:148-153`), but this is an escape hatch, not a supported dynamic-topology feature; the docs steer users to register one handoff per destination instead (`docs/handoffs.md:44`).

3. **Is there a single point of failure?**
   Yes — the runner loop. One `while True` iteration processes exactly one agent's turn; all routing decisions funnel through `SingleStepResult.next_step` (`src/agents/run_internal/run_steps.py:199`) handled at two dispatch sites (`src/agents/run_internal/run_loop.py:1390-1449` resumed-streaming, `src/agents/run_internal/run_loop.py:1848-1929` streaming, `src/agents/run.py:2089-2138` non-streaming). If the loop dies (exception, cancellation), the entire agent tree stops; mitigation is serialization/resume via `RunState`, which persists the current agent and pending handoffs and validates them on restore (`src/agents/run_state.py:2864-2880`; resume tests at `tests/test_tool_name_collision_policy.py:210`, `tests/test_handoff_history_duplication.py:1822`). The centralized design also means the loop enforces safety globally: turn cap (`src/agents/run_internal/run_loop.py:1542-1552`), approval pauses surfacing run-wide even from nested agents-as-tools (`docs/human_in_the_loop.md:5`).

4. **Can agents discover each other?**
   Not autonomously. Discovery is static and developer-mediated in three ways: (a) the model "discovers" peers through tool definitions injected each turn — name/description derived from the target agent's `name` and `handoff_description` (`src/agents/agent.py:189-192`, `src/agents/handoffs/__init__.py:214-218`); (b) humans discover the graph offline via the visualization module that renders handoff/tool/MCP edges as DOT (`src/agents/extensions/visualization.py:146-168`); (c) the framework re-resolves peers during resume by rebuilding handoff/tool maps from serialized agent graphs (`src/agents/run_state.py:2864-2880`, complex-graph test at `tests/test_run_state.py:4160`). There is no registry, announcement protocol, or capability query between agents.

## Architectural Decisions

1. **Represent handoffs as function tools rather than a separate protocol** (`src/agents/handoffs/__init__.py:207-218`, `src/agents/run_internal/turn_resolution.py:206-213`). This reuses the existing tool-call pipeline, schemas, and strict-JSON validation; the cost is that routing quality depends entirely on prompt/tool-description quality, which the SDK addresses with `RECOMMENDED_PROMPT_PREFIX` guidance (`docs/handoffs.md:141-145`).

2. **One active agent at a time (control-transfer semantics)** (`src/agents/run_internal/run_loop.py:1390-1412`). Handoff changes *who* the runner drives next, not how many run concurrently. This keeps usage accounting, guardrails, approvals, and sessions trivially consistent, at the price of forbidding in-run parallel specialists.

3. **Two delegation primitives instead of one** (`src/agents/agent.py:608-612`): handoff for ownership transfer, `as_tool()` for bounded subtasks with isolated input and extracted output. The combination is explicitly encouraged (triage → specialist → specialist-calls-tools, `docs/multi_agent.md:31`).

4. **History policy as a pluggable boundary** (`src/agents/handoffs/__init__.py:158-172` input_filter; `src/agents/run_config.py:366-388` run-level filter, `nest_handoff_history` opt-in beta default-off at `:374`, custom mapper at `:383`): what the successor sees is decoupled from what the session stores; provenance tracking prevents double-appending summarized occurrences (`src/agents/handoffs/history.py:97-157`, tests `tests/test_handoff_history_duplication.py:481`).

5. **Deterministic orchestration delegated to user code** (`docs/multi_agent.md:43-52`): the SDK provides no DAG/pipeline abstraction; chaining, judge loops, and fan-out/fan-in are composed from public `Runner.run` calls (e.g., `examples/agent_patterns/parallelization.py:30-43`), coordinated only by sharing a trace (`examples/agent_patterns/parallelization.py:29`).

6. **Server-side orchestration isolated behind an experimental model adapter** (`src/agents/extensions/experimental/hosted_multi_agent/model.py:369-385`): hosted concurrency arrives as a drop-in `Model`, deliberately incompatible with local handoffs and approvals so the two topologies cannot be silently mixed (`model.py:396-411`).

## Notable Patterns

- **Triage/supervisor routing**: a front agent declares specialists as handoffs; the chosen specialist becomes the active agent and typically terminates the run (`docs/multi_agent.md:27`, example wiring `docs/handoffs.md:17-29`). Multi-turn continuation uses `result.last_agent` as the next starting agent (`src/agents/repl.py:75`, `src/agents/voice/workflow.py:93-101`).
- **Manager-with-tools**: manager calls specialists via `as_tool()`, combines outputs, owns the final answer (`docs/multi_agent.md:26`).
- **Transfer-message envelope**: the handoff tool output is a machine-readable JSON marker `{"assistant": "<name>"}` rather than prose (`src/agents/handoffs/__init__.py:203-204`), giving downstream consumers a stable record of the switch.
- **Graph snapshotting for name reservation**: a contextvar pins the enabled-handoff set so MCP tool-name prefixing reserves exactly the live handoff names (`src/agents/agent.py:99-101`, `:238-248`) — a small but telling example of keeping topology-consistent state across subsystems.
- **Weakref-based cycle avoidance**: both `Handoff._agent_ref` and `RunResult.last_agent` use weak references so cyclic agent graphs and long-lived results do not leak agents (`src/agents/handoffs/__init__.py:195-198`, `src/agents/result.py:522-533`).
- **Same topology model across surfaces**: text runs, voice (`SingleAgentVoiceWorkflow` swaps `_current_agent` from `last_agent`, `src/agents/voice/workflow.py:77-101`), and realtime (`collect_enabled_handoffs`, `src/agents/realtime/handoffs.py:55-69`) all reuse the declare-edges/route-via-runner pattern.

## Tradeoffs

- **Simplicity and auditability vs. expressiveness**: sequential control transfer makes every run a linear chain of owners — easy to trace (`handoff_span` with from/to, `src/agents/run_internal/turn_resolution.py:578-597`) and to serialize/resume, but incapable of expressing negotiation, debate, or concurrent specialists without dropping to raw `asyncio` in user code.
- **LLM-routed edges vs. deterministic routing**: handoff selection is a model decision over tool descriptions; misroutes are only recoverable if the target declares a reverse edge statically. The SDK mitigates with recommended prompts and per-handoff descriptions but offers no router enforcement (contrast: forcing tool use example `examples/agent_patterns/forcing_tool_use.py`).
- **Full-history forwarding vs. context hygiene**: by default the successor sees everything (`docs/handoffs.md:105`); filters/nesting fix leakage and bloat but are opt-in and partially unsupported with server-managed conversations (`src/agents/run_internal/turn_resolution.py:495-524`).
- **Static graph vs. runtime flexibility**: declaring edges up front enables validation (missing-handoff errors at parse time, `tests/test_run_step_processing.py:303`) and DOT rendering, but means topology changes require new `Agent` objects (e.g., via `clone`, `src/agents/agent.py:568-581`) rather than in-place mutation.

## Failure Modes / Edge Cases

- **Multiple simultaneous handoff calls**: only the first executes; the rest are answered "Multiple handoffs detected, ignoring this one." and the span records the requested list (`src/agents/run_internal/turn_resolution.py:563-597`; behavior pinned by `tests/test_run_step_execution.py:4085`). Silent-ish arbitration — the losing calls are not retried.
- **Unknown handoff requested**: raises `ModelBehaviorError` ("missing handoff") rather than rerouting (`tests/test_run_step_processing.py:303`).
- **Runaway delegation**: capped by `DEFAULT_MAX_TURNS = 10` with `MaxTurnsExceeded`, overridable per run (`src/agents/run_config.py:45`, `src/agents/run_internal/run_loop.py:1542-1552`).
- **Server-managed conversations**: input filters forbidden (error) and nested-history disabled with a warning, since the server owns history the client cannot rewrite (`src/agents/run_internal/turn_resolution.py:495-524`).
- **Summary round-trip fragility**: nested-history summaries are text-parsed back into transcript items on later handoffs (`src/agents/handoffs/history.py:469-570`); unparseable lines are dropped defensively and legacy bare-role records recovered (`history.py:544-554`), an inherent lossy step acknowledged by extensive parsing tests (`tests/test_extension_filters.py:377-1006`).
- **Guardrail scope after handoff**: input guardrails only fire on the first agent in the chain (`src/agents/agent.py:350-353`); post-handoff protection shifts to tool guardrails, which themselves do not cover the handoff call (`docs/guardrails.md:65`) — a real gap if a specialist is reached maliciously.
- **Hosted mode restrictions**: hosted multi-agent cannot restore interrupted responses from serialized state, so approvals are rejected outright (`src/agents/extensions/experimental/hosted_multi_agent/model.py:402-411`).

## Future Considerations

- **Concurrency inside the runner**: `max_concurrent_subagents` in the hosted beta (`model.py:80-86`) signals demand for parallel sub-agents; a local equivalent would close the largest gap versus the asyncio.gather pattern.
- **Dynamic topologies**: `is_enabled` covers gating; retargeting edges at runtime (e.g., registry-driven destinations) remains custom-`Handoff` territory and could be productized if demand appears.
- **Stabilizing nested handoff history**: currently an opt-in beta defaulting off (`src/agents/run_config.py:374`); once stable it becomes the sane default for long chains where verbatim forwarding bloats context.

## Questions / Gaps

- **No evidence found** of any distributed/multi-process coordination: nothing in `src/agents/` queues work to remote agents; the only cross-process element is the hosted beta's WebSocket transport (`model.py:69-73`).
- **No evidence found** of agent-to-agent negotiation or shared-memory blackboard primitives; searches for `blackboard`, `message_bus`, `pubsub`, `discover` produced no coordination machinery.
- Whether a custom `Handoff.on_invoke_handoff` returning an arbitrary agent is a *supported* dynamic-dispatch contract is ambiguous: the type signature allows it (`src/agents/handoffs/__init__.py:148-153`), the helper discourages it (`:278-280`), and `docs/handoffs.md:44` permits it for "your own handoff code". Serialization/resume behavior for such dynamic targets is untested in the files reviewed.
- The report does not quantify routing accuracy or token cost of nested-history summaries under real models — out of scope for a static analysis.

---

Generated by `15.01-coordination-topology` against `openai-agents-sdk`.
