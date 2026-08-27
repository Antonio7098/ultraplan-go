# Source Analysis: pydantic-ai

## Planning Location and Responsibility

### Source Info

| Field | Value |
|-------|-------|
| Name | pydantic-ai |
| Path | `studies/agent-harness-study/sources/pydantic-ai` |
| Language / Stack | Python — `pydantic_ai_slim` + `pydantic_graph` + `pydantic_evals` (uv workspace) |
| Analyzed | 2026-08-27 |

## Summary

Pydantic AI core (`pydantic_ai_slim`) contains **no first-party planning runtime object**. The agent loop in `pydantic_ai_slim/pydantic_ai/_agent_graph.py:442` is a fixed triple `UserPromptNode -> ModelRequestNode -> CallToolsNode` driven entirely by the model; there is no `PlannerNode`, `Plan` type, plan store, or task-decomposition code. Planning is explicitly outsourced: the docs declare `Planning` as a Harness capability (`docs/capabilities/overview.md:72-73` — "Model-owned task plans with a cache-safe live reminder") sourced from the external `pydantic-ai-harness` repo (`docs/navigation.yml:311` — `source: "harness"`), referenced in `README.md:28` and `README.md:52` as part of `Coder()` and nowhere implemented under `pydantic_ai_slim/pydantic_ai/capabilities/__init__.py:72-96` (no `Planning` export). Complementarily, `docs/toolsets.md:897-899` points to third-party `pydantic-ai-todo` / `pydantic-deep` for planning. `pydantic_graph` provides a generic typed workflow engine (`pydantic_graph/pydantic_graph/graph_builder.py:1139`) but is used only as the substrate for the agent loop, never as a planner. Planning, where it happens, is therefore **model-owned imperative tool use inside prose/instructions, optionally remembered via an external capability that re-injects a live reminder**, not a verified runtime artefact.

## Rating

**2 / 10 — Absent / implicit / ad-hoc in core**

No planner prompt, planning agent, plan schema, or progress-tracking state exists in `pydantic_ai_slim`. The only built-in planning affordance is the `Capability`/`Toolset` extension point that a separate package must fill. Within the studied source the planning question is answered by delegation to a sibling repo plus third-party toolsets, with zero tests, types, or observability inside `pydantic-ai` itself. The generic `pydantic_graph` graph примитив is mature but not wired for planning. This matches the rubric's 1–3 band: planning is prose or external tooling, not a runtime object.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Planner prompts | No planner prompt file under `pydantic_ai_slim`; grep `plan` finds only docs references. Canonical doc states planning lives in Harness. | `docs/capabilities/overview.md:72-73` |
| Planning capability origin | Navigation pins Planning to Harness source, not core. | `docs/navigation.yml:310-313` |
| Harness framing | README advertises `Planning()` as bundled inside `Coder()` from `pydantic-ai-harness`, not `pydantic-ai`. | `README.md:28` , `README.md:52` , `README.md:66` |
| Capabilities registry | Core capability registry enumerates 16 types (`MCP`, `Thinking`, `ToolSearch` …) with no `Planning`. | `pydantic_ai_slim/pydantic_ai/capabilities/__init__.py:72-95` |
| Agent graph topology | Only three nodes declared: `UserPromptNode`, `ModelRequestNode`, `CallToolsNode`; no planner node. Graph loop detail: `AgentNode` base + `GraphAgentState`/`GraphAgentDeps`. | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:442-453` , `pydantic_ai_slim/pydantic_ai/_agent_graph.py:500-741` |
| Agent constructor | `Agent.__init__` exposes `model`, `instructions`, `tools`, `toolsets`, `capabilities`, `end_strategy` — no `planner`, `plan`, `todo`, `task` param. | `pydantic_ai_slim/pydantic_ai/agent/__init__.py:487-555` |
| Workflow graph primitive | `pydantic_graph` is a generic typed builder (`Graph`, `GraphRun`, `Step`, `Fork`, `Join`, `Decision`) with no planning specialization. | `pydantic_graph/pydantic_graph/__init__.py:1-77` , `pydantic_graph/pydantic_graph/graph_builder.py:1139-1165` |
| Task decomposition code | Zero code matches `todo|planner|decomposition|Plan` in `pydantic_ai_slim/pydantic_ai` (verified via grep). | `pydantic_ai_slim/pydantic_ai/` *(no match)* |
| Third-party delegation | Docs route planning toolsets to external `pydantic-ai-todo` `TodoToolset` (`read_todos`/`write_todos`) and `pydantic-deep`. | `docs/toolsets.md:897-899` |
| Capability extensibility | Planning would be built via `AbstractCapability` (`id`, `description`, `defer_loading`, `get_instructions`, `wrap_model_request` etc.) but core ships no such subclass. | `pydantic_ai_slim/pydantic_ai/capabilities/abstract.py:162-228` , `pydantic_ai_slim/pydantic_ai/capabilities/abstract.py:202-228` |
| Multi-agent planning mention | Docs position planning under "Deep Agents — autonomous agents with planning, file operations, task delegation…" delegated to third-party frameworks. | `docs/multi-agent-applications.md:9` |
| Agent loop is model-driven | `_agent_graph.py` preparation merges `ToolDefinition`s and probes `discovered_tool_names` — loop is entirely model-decided tool invocation, not planner-dispatched. | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:764-843` |

## Answers to Dimension Questions

### 1. Where does planning happen?

**Outside the studied source.** Inside `pydantic-ai` there is no planning location: not in prompts (`docs/capabilities/overview.md:72` points outward), not in runtime code (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:442` has no planner node), not in a planner agent, not in `pydantic_graph` (generic `pydantic_graph/pydantic_graph/graph_builder.py:1139`), and not as an external orchestrator shipped by this repo. The only place it can happen within this codebase is **in-model via ad-hoc tool calls to a user- or harness-provided `Toolset`** (e.g. `pydantic-ai-todo`'s `read_todos`/`write_todos` at `docs/toolsets.md:898`) that the model is nudged to call — i.e. imperative prose plus a filesystem tool. The canonical implementation lives in the sibling `pydantic-ai-harness` repository (navigation source `harness` at `docs/navigation.yml:312`), uninspected per source-isolation rules.

### 2. Who owns the plan?

**Model-owned.** Harness descriptor explicitly says "Model-owned task plans" (`docs/capabilities/overview.md:73`). Core ships no `Plan` dataclass, no plan validator, no owner field. Under the `AbstractCapability` model the model owns creation and mutation (tool invocation → tool result → history re-prompt); the framework only owns re-injection. `pydantic_ai_slim/pydantic_ai/capabilities/abstract.py:328-338` `get_instructions` is the hook a harness `Planning` capability uses to append a "live reminder" to `InstructionPart`, keeping a cache-busted plan visible without the runtime ever asserting correctness.

### 3. Is planning required?

**No — fully optional.** An `Agent` constructs and runs with zero capabilities (`pydantic_ai_slim/pydantic_ai/agent/__init__.py:394` defaults). Planning is absent unless the caller composes `Planning()` from harness or a third-party `TodoToolset` (`docs/toolsets.md:898`). No graph path or validation forces a plan step; `UserPromptNode.run()` (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:523-658`) goes directly to `ModelRequestNode`, never through a plan gate.

### 4. Is planning visible?

**Only via generic message history or an external live reminder — not as a first-class artefact inside this repo.** Core has no `plan` field on `GraphAgentState` (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:298-343`), no plan-aware span, and no `mcp`/`instrumentation` specialization for plans (`pydantic_ai_slim/pydantic_ai/capabilities/__init__.py:72-95` has no Planning telemetry). Visibility depends on: (a) the model-emitted tool calls appearing in `message_history` (replayed like any tool), and (b) the harness capability's "cache-safe live reminder" (`docs/capabilities/overview.md:73`), which re-injects the current plan as an instruction each turn so cache prefix stays warm. Without that capability, visibility is exactly as visible as any other tool artifact (i.e. buried in history). No plan-diff store, no progress bar type, no `StepPersistence` coupling is implemented in slim.

### 5. Is planning reusable?

**No — not as a runtime object inside the studied source.** There is no `Plan` type to serialize, deserialize, or share across runs/agents; `pydantic_graph`'s `Graph`/`GraphRun` state (`pydantic_graph/pydantic_graph/graph_builder.py:156-241`) is generic workflow state, not a plan template. Reuse inside `pydantic-ai` amounts to re-prompting or reusing message history. The harness `Planning` capability presumably makes reuse ergonomic (persisted via harness memory/step-persistence layers referenced in `docs/capabilities/overview.md:128-131`), but that code was not inspected and is not part of this source's import closure.

## Architectural Decisions

- **Decision: Core stays planner-free; planning is a Harness concern.** Evidence: `docs/navigation.yml:311` + `docs/capabilities/overview.md:27-28` + `README.md:28`. Rationale deducible from docs: keeps `pydantic-ai-slim` minimal, typed, provider-agnostic; avoids opinionated multi-step orchestration in the loop. Consequence: planning innovation lives next door; core users must opt into an external package.

- **Decision: Agent loop is minimal LLM-tool loop, not a supervisor-planner-executor.** Evidence: `pydantic_ai_slim/pydantic_ai/_agent_graph.py:442-658` three-node graph, `_prepare_request_parameters` at `pydantic_ai_slim/pydantic_ai/_agent_graph.py:764-843` merely unions `ToolDefinition`s. No orchestration state.

- **Decision: Capability-as-extension-point for planning, reusing `id`/`defer_loading`/`get_instructions`.** Evidence: `pydantic_ai_slim/pydantic_ai/capabilities/abstract.py:202-228`, `pydantic_ai_slim/pydantic_ai/capabilities/__init__.py:64`. Planning loads like any deferred skill via `load_capability`, with instructions surfacing once loaded.

- **Decision: Generic `pydantic_graph` for workflows, not planning.** Evidence: `pydantic_graph/pydantic_graph/__init__.py:1-7`, `pydantic_graph/pydantic_graph/graph_builder.py:430-841` fork/join/decision/reducer primitives duplicated for agent execution, not task planning. Planning could be built on top but is not wired.

## Notable Patterns

- **"Batteries, composably" via Capability composition** (`docs/capabilities/overview.md:3-15`, `pydantic_ai_slim/pydantic_ai/capabilities/__init__.py:64-96`). Planning slots in exactly like `WebSearch` or `MCP` — a `CombinedCapability` whose `get_toolset` yields a planning toolset and whose `get_instructions` appends the live reminder. Proves a consistent extension surface.

- **Cache-safe live reminder** (`docs/capabilities/overview.md:73`). Mentioned as distinct from naive prompt appending: harness re-injects the same instruction prefix each turn so provider prefix-cache survives while the plan stays live. Implementation not in slim, but descriptor signals awareness of token-cost failure modes.

- **Model-owned plan with framework-owned reminder.** Mirrors the bank-support example in `README.md:256-345` where `Capability` bundles behavior without a central planner — model decides when to branch into `refunds`/`refund_status`; planning follows the same delegatory idiom.

- **Third-party planning toolsets as first-class docs entries** (`docs/toolsets.md:897-899` `pydantic-ai-todo`, `pydantic-deep`). Signals pattern: task tracking is expected to be supplied by community toolsets, validated by core only as `AbstractToolset`/`Tool` definitions (`pydantic_ai_slim/pydantic_ai/toolsets/abstract.py:5`).

## Tradeoffs

- **Minimal dependency vs. feature gap.** Omitting planning keeps `pydantic-ai-slim` small, typed, and provider-neutral (`pyproject.toml` / `pydantic_ai_slim/` isolation). Tradeoff: teams that want enforced plans pay integration cost for `pydantic-ai-harness` or a third-party toolset and inherit their operational model (file persistence vs. DB).

- **Flexibility vs. guarantees.** Model-owned plans impose no structure; any multi-step decomposition the model invents is accepted. No schema validation means maximal flexibility (model can plan in natural language) but zero contract enforcement (malformed or stale plans are not detected by runtime — contrast `OutputSpec`/`output_type` validation at `pydantic_ai_slim/pydantic_ai/agent/__init__.py:648` for final output).

- **Cache efficiency vs. visibility.** Live-reminder pattern sacrifices history-purity (plan appears as instructions each turn) to preserve prefix cache. Without it, stuffing the plan into `UserPromptPart` would bust cache per variant plan length.

- **Graph maturity vs. missed wiring.** `pydantic_graph` already supplies fork/join/reducers and persistence hooks (`pydantic_graph/pydantic_graph/graph_builder.py:156-361`) well-suited to verified multi-step plans, yet no planner composes over it in core — a deliberate "strong primitives, not batteries" stance per `AGENTS.md` phrasing ("general primitives … over narrow solutions").

- **Runtime-owned reminder but model-owned truth.** Framework owns presentation, model owns content; halves failure surface (framework can't corrupt plan) but leaves drift (reminder may lag the model's internal intent).

## Failure Modes / Edge Cases

- **Hallucinated or stale plan.** With no runtime plan object or validator (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:298-343` no plan fields), a model that mutates its plan implicitly (continues with different steps than it announced) has no guardrail; execution silently diverges.

- **Plan drift under long context.** Without periodic re-anchoring, instructions fade; core has no `ReinjectSystemPrompt`-style hook for plans except whatever the harness reminder does. Grepping `pydantic_ai_slim/pydantic_ai` finds no periodic plan re-validation.

- **Cache bust if mis-integrated.** If a planner toolset inserts plan text as a fresh `UserPromptPart` each turn instead of as stable-prefix instructions, every turn busts prompt cache. Harness advertises "cache-safe" precisely to avoid this; a naive third-party `TodoToolset` will not.

- **No concurrency fencing on plan mutation.** `GraphAgentState` and `ToolManager` have no locking around plan state; parallel tool calls (default `ToolManager` parallel mode) could interleave plan reads/writes where a todo store is backed by a file without atomicity.

- **Persistence gap.** `pydantic_graph` / `AgentRun` message history is the only durable artefact; mid-run crashes lose any in-memory plan unless a harness `StepPersistence` capability (`docs/capabilities/overview.md:128`) is also composed. Core restart alone cannot resume an incomplete plan.

- **No approval/rollback for plan steps.** Unlike `ApprovalRequired`/`CallDeferred` tool control flow (`pydantic_ai_slim/pydantic_ai/capabilities/abstract.py:1099-1123`), planning has no per-step gate, so a risky step cannot be paused without the user building their own guardrail capability.

- **Model context exhaustion by plan.** If the plan grows (many subtasks + tool outputs) the context window budget measured by `UsageLimits` / `_check_continuation_usage` at `pydantic_ai_slim/pydantic_ai/_agent_graph.py:875-893` will eventually trip, but only as a global token limit, not as a typed "plan too large" error.

## Future Considerations

- Add a typed `Plan`/`PlanStep` schema (status, assignee, deps) exposed as `pydantic_ai` types optionally validated via `PlanToolset`, allowing runtime validation without adopting full harness.
- Wire an opt-in `PlanningCapability` wrapper that reuses `AbstractCapability` hooks (`before_model_request`, `after_model_request`, `wrap_run_event_stream`) to enforce plan-step gating, progress-stream events, and cache-safe reinjection directly in slim, with tests in `tests/`.
- Expose plan observability via OTEL (`pydantic_ai_slim/pydantic_ai/capabilities/instrumentation.py:5` `Instrumentation`) — emit `plan.created`/`plan.updated` spans so Logfire can surface staleness.
- Consider a `pydantic_graph`-backed planner that compiles a plan into a `GraphBuilder` subgraph (Fork/Join/Decision) yielding resumable, type-checked execution — leverages existing graph primitives rather than reinventing them.
- Introduce `AgentSpec` serialization for plans (`pydantic_ai_slim/pydantic_ai/agent/spec.py:5`) so plans travel with YAML/JSON specs consistently with current capability serialization (`CAPABILITY_TYPES` at `pydantic_ai_slim/pydantic_ai/capabilities/__init__.py:72`).

## Questions / Gaps

- Harness `Planning` source not inspected per source-isolation hard rule; claims about its live-reminder, persistence, and API rely solely on docs descriptors (`docs/capabilities/overview.md:73`, `docs/navigation.yml:312`). Implementation details, line numbers, tests, and failure handling inside that repo are unverified here.
- No tests for planning were found in `tests/` (grep returned no contract). Whether `pydantic-ai` CI covers harness planning behavior via integration tests in the harness repo is unknown from this source alone.
- `pydantic_ai_slim/pydantic_ai/durable_exec/` and `.agents/skills/` mention planning-adjacent concepts (deferred tools, durability) but contain no planning policy; whether durable execution and planning interact (e.g. plan checkpoints as durable activities) is not evidenced.
- Provider-native planning affordances (e.g. Anthropic extended thinking vs. planning) are documented separately (`docs/capabilities/thinking.md:5`, `docs/capabilities/overview.md:72` Thinking capability) and were not evaluated as planners.

---

Generated by `06.01-planning-location-and-responsibility` against `pydantic-ai`.
