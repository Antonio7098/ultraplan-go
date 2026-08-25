# Source Analysis: pydantic-ai

## 04.08 — Agent-as-Tool and Workflow-as-Tool Composition

### Source Info

| Field | Value |
|-------|-------|
| Name | pydantic-ai |
| Path | `studies/agent-harness-study/sources/pydantic-ai` |
| Language / Stack | Python (pydantic-ai-slim core agent framework, plus `pydantic_graph` and `pydantic_evals` packages; asyncio-based) |
| Analyzed | 2026-08-23 |

Citation convention: all paths are relative to the source root (`studies/agent-harness-study/sources/pydantic-ai/`).

## Summary

Pydantic AI deliberately has **no dedicated "Agent.as_tool()" wrapper API** in this revision. Composition is modeled as *delegation through ordinary Python*: an agent's tool function is an async callable that awaits another agent's `run()`, and the framework supplies the surrounding machinery — shared usage accumulation (`usage=ctx.usage`), run-scoped cancellation with sub-agent isolation, per-run usage limits, OTel run/tool span nesting with baggage propagation, and typed child results (`AgentRunResult[T]`). The docs codify five composition levels (tool delegation, programmatic hand-off, graph control flow via the embedded `pydantic_graph` engine, deep agents) in `docs/multi-agent-applications.md:3-11`, and the agent loop itself is a pydantic-graph state machine built by `build_agent_graph` (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:2580`). The library ships two first-party examples of the pattern as tools backed by nested agents: image generation and X search fallbacks (`pydantic_ai_slim/pydantic_ai/common_tools/image_generation.py:98-108`, `pydantic_ai_slim/pydantic_ai/common_tools/x_search.py:68-78`). The model is well-tested for usage attribution and cancellation semantics, but there is **no runtime recursion guard** (bounding is economic, not structural), child runs do **not** automatically inherit the parent's usage limits, cost attribution is opt-in and lossy across differently-priced models, and the library's own subagent-backed tools omit usage attribution.

## Rating

**7 / 10** — Clear composition model with explicit interfaces (`run()`/`iter()` signatures carrying `usage`, `usage_limits`, `cancellation_token`; `pydantic_ai_slim/pydantic_ai/agent/abstract.py:469-544`), tests pinning delegation usage attribution (`tests/test_usage_limits.py:209-336`) and sub-agent cancellation isolation (`tests/test_run_cancellation.py:1786-1847`), and operational safeguards (usage limits enforced before requests, after responses, mid-stream, and before tool calls). It falls short of 8–9 because recursion is bounded only by budgets rather than a structural depth guard, child-run limit inheritance and attribution are opt-in and silently skipped by the bundled subagent tools, and durable-execution attribution has a documented unresolved gap (`docs/durable_execution/temporal.md:223`, referencing pydantic-ai#6886).

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Delegation pattern (docs, tested example) | Parent tool awaits `joke_generation_agent.run(..., usage=ctx.usage)`; doc states delegation = "agents using another agent via tools" | `docs/multi-agent-applications.md:13-97` |
| No `as_tool` wrapper exists | Repo-wide search for `as_tool`/`AgentAsTool` in `pydantic_ai_slim` returns no definition; only subagent-backed common tools exist | search boundary noted in Questions/Gaps |
| Run entry carries composition knobs | `run(..., usage_limits=..., cancellation_token=..., usage=...)`; `usage` documented as "useful for resuming … or agents used in tools" | `pydantic_ai_slim/pydantic_ai/agent/abstract.py:521-579` |
| Shared usage accumulator | `GraphAgentState.usage` field; `iter()` passes caller's object straight into state (`usage=usage`) so parent and child mutate one instance | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:298-303`; `pydantic_ai_slim/pydantic_ai/agent/__init__.py:1480-1487` |
| Usage increments in place on shared state | `ctx.state.usage.incr(response.usage)` in `_append_response`; `requests += 1` at each request site | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1789-1791, 1161, 1202, 1284, 1394, 1443-1447` |
| Usage attribution pinned by tests | Without `usage=ctx.usage` delegate tokens absent from parent; with it, `result2.usage` equals sum of both agents | `tests/test_usage_limits.py:209-226, 275-286, 335-336` |
| Manual attribution also supported | Tool calls `ctx.usage.incr(new_usage)` directly | `tests/test_usage_limits.py:402-414` |
| Per-run limits resolved fresh | `usage_limits = usage_limits or _usage.UsageLimits()` — children get their own defaults unless passed | `pydantic_ai_slim/pydantic_ai/agent/__init__.py:1516` |
| Limit surface & defaults | `request_limit=50` default; `cost_limit`, `tool_calls_limit`, token limits, per-request input cap | `pydantic_ai_slim/pydantic_ai/usage.py:417-472` |
| Enforcement points | Before request (`check_before_request`), after response (`check_tokens`/`check_cost`), projected tool-call check | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1602-1621, 1723, 1789-1797`; `pydantic_ai_slim/pydantic_ai/_tool_execution.py:444-448` |
| Sub-agent self-cancel isolated as failed tool return | `cancelled_sub_agent_return()` builds `outcome='failed'` return; `_call_tool` catches `RunCancelled` | `pydantic_ai_slim/pydantic_ai/_tool_execution.py:41-62, 701-702` |
| Cancellation tests | Self-cancel isolated (`…is_isolated_as_tool_failure`), opt-in propagation, non-tool-site escape re-stamped with parent history | `tests/test_run_cancellation.py:1786-1847` |
| Whole-tree cancellation | Thread-safe `CancellationToken` registers multiple runs and cancels them all | `pydantic_ai_slim/pydantic_ai/_cancel.py:42-90`; `docs/agent.md:841-847` |
| Nested binding isolation | `take_run_binding` consumed at most once so nested runs don't inherit outer handle's cancellation binding | `pydantic_ai_slim/pydantic_ai/_cancel.py:285-293`; `tests/test_run_cancellation.py:1419-1529` |
| Tracing: run span + IDs | `invoke_agent` span with `gen_ai.agent.call.id` (run_id), `gen_ai.conversation.id` | `pydantic_ai_slim/pydantic_ai/capabilities/instrumentation.py:168-189` |
| Tracing: baggage propagation to nested runs | Baggage set around handler; read back into every tool-span attributes | `pydantic_ai_slim/pydantic_ai/capabilities/instrumentation.py:187-190, 397`; `pydantic_ai_slim/pydantic_ai/_instrumentation.py:123-132` |
| Child trace identity | Each run resolves its own UUID7 `run_id`/`conversation_id` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:306-318` |
| Span-tree ancestor queries for delegation | Evals can assert child spans have parent agent ancestors ("no circular delegation" sample) | `docs/evals/evaluators/span-based.md:530-549`; `pydantic_evals/pydantic_evals/evaluators/common.py:342-356` |
| Nested tool calls counted by evals | Agentic evaluators count spans "including tool calls made by nested sub-agents (agent-as-tool delegation)" | `pydantic_evals/pydantic_evals/evaluators/agentic.py:28-36` |
| Built-in agent-as-tool wrappers | `ImageGenerationSubagentTool.__call__` constructs an `Agent(output_type=BinaryImage)` inside a tool and runs it; same shape for X search | `pydantic_ai_slim/pydantic_ai/common_tools/image_generation.py:79-108`; `pydantic_ai_slim/pydantic_ai/common_tools/x_search.py:54-78` |
| Error translation in wrappers | Child `UnexpectedModelBehavior` → `ModelRetry` to parent loop | `pydantic_ai_slim/pydantic_ai/common_tools/image_generation.py:104-107` |
| Structured child results | `AgentRunResult.output/.usage/.all_messages/.new_messages/.run_id/.conversation_id` | `pydantic_ai_slim/pydantic_ai/run.py:593-743` |
| Orchestrator example | Triage agent tools return structured `TriageFinalOutput` wrapping child `MedicalReport`; catches `ModelHTTPError` | `examples/pydantic_ai_examples/medical_agent_delegation.py:180-238` |
| Graph-based multi-agent workflow | Agents awaited inside graph nodes (email writer + feedback loop); agent loop itself is a `pydantic_graph.Graph` | `docs/graph.md:363-466`; `pydantic_ai_slim/pydantic_ai/_agent_graph.py:2580` |
| Durable-execution attribution gap | Temporal activity receives copied `RunContext`; delegate `usage` never reaches parent; fix under discussion (#6886) | `docs/durable_execution/temporal.md:223` |

## Answers to Dimension Questions

1. **Can one agent call another?** Yes. There is no wrapper class; the sanctioned mechanism is awaiting `sub_agent.run(...)` directly inside a parent tool (`docs/multi-agent-applications.md:13-97`), with `deps=ctx.deps` and `usage=ctx.usage` as the coupling points (`pydantic_ai_slim/pydantic_ai/agent/abstract.py:481,536`). First-party instances of the pattern exist as fallback tools that construct an `Agent` inline (`pydantic_ai_slim/pydantic_ai/common_tools/image_generation.py:98-105`). Programmatic hand-off and `pydantic_graph` workflows cover coarser compositions (`docs/multi-agent-applications.md:205-215`, `docs/graph.md:363-466`).
2. **Are child runs bounded?** Partially, and opt-in at the tree level. Every run — including nested ones — resolves its own `UsageLimits` defaulting to `request_limit=50` (`pydantic_ai_slim/pydantic_ai/agent/__init__.py:1516`; `pydantic_ai_slim/pydantic_ai/usage.py:429`), checked against the (possibly shared) usage object before each request, after each response, and before projected tool calls (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:1621, 1723, 1790-1797`; `pydantic_ai_slim/pydantic_ai/_tool_execution.py:444-448`). But the parent's custom limits are **not inherited**: unless the developer passes the same `usage_limits` instance down, a delegate gets fresh defaults. Because checks read the shared accumulator when `usage=ctx.usage` was used, cumulative request/token/cost limits do bite across the tree if the same limits object accompanies the shared usage.
3. **Are child run costs attributed?** Opt-in via shared mutable `RunUsage`: passing `usage=ctx.usage` makes child increments land on the parent's totals (`pydantic_ai_slim/pydantic_ai/agent/__init__.py:1482`; increment sites `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1161, 1789-1791`), pinned by snapshots in `tests/test_usage_limits.py:275-286, 335-336`. Cost sums arithmetically when priced (`pydantic_ai_slim/pydantic_ai/usage.py:393-395`), but cross-model monetary reconstruction is explicitly disclaimed (`docs/multi-agent-applications.md:25`), and under Temporal the delegate's usage is structurally lost because activities see a copy of the context (`docs/durable_execution/temporal.md:223`). Notably, the library's own subagent tools omit `usage=ctx.usage`, so their nested spend is unattributed (`pydantic_ai_slim/pydantic_ai/common_tools/image_generation.py:104-105`, `x_search.py:74-75`).
4. **Can nested tools recurse forever?** Structurally yes — there is no depth counter or cycle detector for delegation (searched `depth`, `max_depth`, `recursi*` across `pydantic_ai_slim/pydantic_ai/**`; only schema-recursion and queue-depth hits). Bounding is economic: each run enforces `request_limit=50` by default and optional `total_tokens_limit`/`cost_limit`/`tool_calls_limit` (`pydantic_ai_slim/pydantic_ai/usage.py:427-438`), but since limits attach per run, aggregate spend scales with tree size unless shared objects are threaded through. Circular delegation is detectable only post-hoc via eval span queries (`docs/evals/evaluators/span-based.md:541-549`).
5. **Does the parent receive structured results?** Yes. The delegate returns a full `AgentRunResult[T]` whose `.output` is validated against the child's `output_type`, plus `.usage`, `.all_messages()`/`.new_messages()`, `.run_id`, `.conversation_id` (`pydantic_ai_slim/pydantic_ai/run.py:593-743`). What reaches the parent *model* is whatever the tool body returns; `build_tool_return_part` normalizes it into history with outcome tracking (`pydantic_ai_slim/pydantic_ai/_tool_execution.py:65-116`). The medical example shows typed end-to-end flow (`MedicalReport` wrapped into `TriageFinalOutput`) with error handling (`examples/pydantic_ai_examples/medical_agent_delegation.py:180-238`).

## Architectural Decisions

1. **Composition via language primitives, not a wrapper abstraction.** Rather than `agent.as_tool()` scaffolding, any async function can be a delegation seam; the framework instead guarantees the cross-cutting concerns (usage sharing, cancellation scoping, tracing). This matches the repo philosophy of "strong primitives over opinionated batteries" (`AGENTS.md`, Philosophy section) and keeps the public surface small.
2. **Shared-mutable-object accounting.** Usage attribution is implemented by handing the child the parent's live `RunUsage` instance (`pydantic_ai_slim/pydantic_ai/agent/__init__.py:1482`), incremented in place (`_incr_usage_tokens`/`_incr_usage_cost`, `pydantic_ai_slim/pydantic_ai/usage.py:371-414`). Cheap and exact for tokens/requests, but it makes correctness depend on a keyword argument and breaks across serialization boundaries (`docs/durable_execution/temporal.md:223`).
3. **Run-scoped cancellation with isolation-by-default.** `cancel()` cancels only the run owning the context (`pydantic_ai_slim/pydantic_ai/_run_context.py:467-498`); a sub-agent's self-cancel surfacing inside a tool is converted to a failed `ToolReturnPart` the parent model can react to (`cancelled_sub_agent_return`, `pydantic_ai_slim/pydantic_ai/_tool_execution.py:41-62, 701-702`), while whole-tree teardown requires sharing a `CancellationToken` (`pydantic_ai_slim/pydantic_ai/_cancel.py:42-90`). Isolation-by-default with opt-in propagation is the inverse of most frameworks, which cancel upward.
4. **Tracing through OTel context, not custom child-trace plumbing.** Nested delegation needs no special trace IDs: the child's `invoke_agent` span opens inside the parent's tool-execution span context, and baggage carries agent name/run/conversation IDs onto descendant spans (`pydantic_ai_slim/pydantic_ai/capabilities/instrumentation.py:183-190, 397`; `pydantic_ai_slim/pydantic_ai/_instrumentation.py:123-132`).
5. **The agent loop itself is a graph.** `build_agent_graph` (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:2580`) means "workflow-as-tool" composes at two levels: graphs can host agents as nodes (`docs/graph.md:363-466`) and agents embed the graph engine internally.

## Notable Patterns

- **Fallback-via-subagent**: native-tool capability gaps are bridged by delegating to a subagent running a different model (`pydantic_ai_slim/pydantic_ai/capabilities/image_generation.py:25-30`, `pydantic_ai_slim/pydantic_ai/common_tools/x_search.py:38-43`) — a first-party showcase that the delegation pattern is intended for production use, not just user code.
- **Isolation seams are explicit**: cancellation isolation applies specifically when the child is awaited in a *tool body*; awaiting from output validators/handlers runs on the parent task and propagates (`docs/agent.md:845`; test `test_sub_agent_cancel_from_non_tool_site_reports_parent_history`, `tests/test_run_cancellation.py:1831-1847`).
- **Binding hygiene**: nested runs cannot steal the outer streaming handle's cancellation binding because `take_run_binding()` consumes the context var once (`pydantic_ai_slim/pydantic_ai/_cancel.py:285-293`), pinned by `tests/test_run_cancellation.py:1419-1529`.
- **Evaluation hooks for composition**: agentic evaluators count nested sub-agent tool calls and span-tree ancestor queries can assert delegation topology (`pydantic_evals/pydantic_evals/evaluators/agentic.py:28-36`; `docs/evals/evaluators/span-based.md:530-549`).
- **Retry translation at the boundary**: builtin subagent tools map child `UnexpectedModelBehavior` to `ModelRetry` so the parent loop treats a broken child as a correctable event (`pydantic_ai_slim/pydantic_ai/common_tools/image_generation.py:104-107`).

## Tradeoffs

- **Simplicity vs. discoverability/guarantees**: no `as_tool()` means no schema-level contract for exposing an agent (name/description/arg-schema derivation, max-result size, timeouts). Developers hand-roll prompt text and arg coercion, and nothing statically prevents forgetting `usage=ctx.usage`.
- **Opt-in attribution vs. silent under-counting**: attribution failures are silent (tokens simply missing from parent totals). Even the maintainers' own tools skip it, and the durable-execution engines lose it (`docs/durable_execution/temporal.md:223`).
- **Per-run budget vs. tree budget**: default `request_limit=50` bounds each run independently; without threading shared `usage_limits`, a delegation fan-out multiplies the worst case linearly with nesting width/depth.
- **Isolation-by-default vs. surprise**: a developer expecting a child's `cancel()` (or a shared-token-free external cancel) to stop the parent must learn the tool-body-only isolation rule; conversely external cancellation of the parent does tear down inline-awaited children (same-task semantics, `docs/agent.md:847`).
- **Aggregate usage vs. per-agent observability**: shared accumulation yields one total; per-agent breakdown exists only if you inspect OTel spans or keep your own ledger — `result.usage` cannot answer "what did the cardiology agent cost".

## Failure Modes / Edge Cases

- **Unattributed child spend** when `usage=ctx.usage` is omitted — demonstrated by `test_multi_agent_usage_no_incr` (`tests/test_usage_limits.py:209-226`); includes the shipped `image_generation_tool`/`x_search_tool` wrappers (`pydantic_ai_slim/pydantic_ai/common_tools/image_generation.py:104-105`, `x_search.py:74-75`).
- **Temporal/DBOS/Prefect attribution loss**: activity-context copies drop delegate usage mutations; a return channel is an open issue (#6886) (`docs/durable_execution/temporal.md:223`).
- **Sync-in-tool footgun**: `run_sync()` inside a tool raises `UserError`; delegation must use `async def` + `await` (`docs/multi-agent-applications.md:79-82`).
- **Sub-agent cancel escaping non-tool sites** terminates the parent but is re-stamped so the escaping `RunCancelled` carries the *parent's* history, with the child's cancellation preserved as `__cause__` (`tests/test_run_cancellation.py:1831-1847`).
- **Cancelled-stream usage is partial/best-effort**, so attributed totals after a cancelled child are provider-dependent (`docs/agent.md:837-839`).
- **Cross-model cost aggregation is lossy**: summed token counts cannot reconstruct monetary cost when delegates use differently-priced models (`docs/multi-agent-applications.md:25`).
- **No recursion guard**: a self-referential delegation loops until budgets trip (or forever with default limits raised); detection is post-hoc via eval span queries (`docs/evals/evaluators/span-based.md:541-549`).
- **Deps coupling is unchecked**: delegates are expected to accept deps that are the parent's or a subset; nothing validates this (`docs/multi-agent-applications.md:99-104`).

## Future Considerations

- A first-class agent-to-tool adapter (auto-derived name/description/schema from `output_type`, with forced usage/limits/deps forwarding options) would close the gap between what the pattern requires and what users must hand-roll.
- Structural recursion controls (per-tree depth counter carried in shared state, or a cycle-detecting agent registry) would complement budget-based bounding.
- Making attribution non-opt-in (e.g., `RunContext.usage` auto-shared unless overridden, mirroring how `deps` must be passed explicitly today) or emitting a warning when a nested run completes with zero usage would harden cost accounting; issue #6886 covers the durable-execution return channel.
- Per-agent usage breakdown in `AgentRunResult` (child-run ledger keyed by `run_id`) would make multi-agent cost observable without OTel.

## Questions / Gaps

- **Where did `as_tool()` go?** Historical versions exposed `Agent.as_tool()`; this revision defines none (searched `as_tool|AgentAsTool` across `pydantic_ai_slim/` and `docs/` — no definitions; only false-positive substring hits like `has_tool_search`). Whether removal was a deprecation cycle or rename could not be confirmed from this snapshot alone (`docs/migration.md` contains no matching entry).
- **Child limit inheritance**: no mechanism propagates the parent's `UsageLimits` into delegates; confirmed by reading `Agent.iter()` resolution (`pydantic_ai_slim/pydantic_ai/agent/__init__.py:1516`). If intended, it is undocumented as such in the source inspected.
- **Realtime delegation**: realtime sessions reuse `cancelled_sub_agent_return` for unsettled calls (`pydantic_ai_slim/pydantic_ai/realtime/_session.py:26,389`), but full nested-run semantics under realtime hand-off were out of scope for this pass.
- **Third-party deep-agent stacks** (`subagents-pydantic-ai`, listed in `docs/capabilities/third-party.md:23`) provide task-spawn toolsets with soft/hard cancel; they were not analyzed as they live outside this source.

---

Generated by `04.08-agent-as-tool-and-workflow-as-tool-composition` against `pydantic-ai`.
