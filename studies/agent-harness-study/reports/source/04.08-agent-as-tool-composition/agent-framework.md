# Source Analysis: agent-framework

## Dimension 04.08: Agent-as-Tool and Workflow-as-Tool Composition

### Source Info

| Field | Value |
|-------|-------|
| Name | agent-framework |
| Path | `studies/agent-harness-study/sources/agent-framework` |
| Language / Stack | Multi-language monorepo: Python (primary composition surface: `BaseAgent.as_tool`, `Workflow.as_agent`, hosting-mcp adapters), .NET (`AIAgentExtensions.AsAIFunction` + workflow/declarative packages), Go (README stub only) |
| Analyzed | 2026-08-23 |

All evidence citations below are workspace-relative paths under `studies/agent-harness-study/sources/agent-framework/`.

## Summary

Composition is adapter-based rather than platform-based: any agent that implements `SupportsAgentRun` can be wrapped into an ordinary tool by `BaseAgent.as_tool()`, which returns a `FunctionTool` whose closure invokes the child agent and returns its text (`python/packages/core/agent_framework/_agents.py:608-724`). Because the wrapper is just another function tool, it inherits the parent's whole function-invocation machinery — approval gating (`approval_mode`), middleware, runtime-kwarg forwarding, and OTel instrumentation — instead of defining a separate nested-run protocol. The same uniformity makes workflows composable upward: `Workflow.as_agent()` exposes a graph as a `WorkflowAgent` (`python/packages/core/agent_framework/_workflows/_workflow.py:1195-1236`), which is itself a `BaseAgent` and therefore also `.as_tool()`-able, and `RawAgent.as_mcp_server()` publishes any agent as a single MCP tool built on the same wrapper (`python/packages/core/agent_framework/_agents.py:1653-1699`). The deliberate cost of this simplicity is a **string-in/string-out contract at every composition boundary**: input is one required string argument with `additionalProperties: false` (`_agents.py:663-673`) and output is `final_response.text` (Python, `_agents.py:710-714`) or `response.Text` (.NET, `dotnet/src/Microsoft.Agents.AI/AgentExtensions.cs:67-90`), so token usage and structured payloads do not cross the boundary unless composition goes through the `WorkflowAgent` event path, which does merge usage across nested agent responses (`python/packages/core/agent_framework/_workflows/_agent.py:509-580`). Bounding is per-run rather than global: each composed run gets its own function loop capped at 40 iterations by default (`python/packages/core/agent_framework/_tools.py:95`, `:1392-1400`), workflows cap supersteps at 100 (`python/packages/core/agent_framework/_workflows/_const.py:4`), and harness loops default to 10 (`python/packages/core/agent_framework/_harness/_loop.py:122`) — but there is no nesting-depth budget or cycle guard for agent-as-tool graphs, so recursive composition is bounded only by these local caps. Tracing of nested execution is implicit but real: every agent run is wrapped in an `invoke_agent` OTel span (`python/packages/core/agent_framework/observability.py:1921-1972`), so parent→child chains appear as span trees via normal async context propagation.

## Rating

**7 / 10** — Clear model with tests, explicit interfaces, and operational safeguards. Composition has a first-class, documented API in both languages (`as_tool` / `AsAIFunction`), dedicated unit-test suites including multi-level A→B→C delegation kwargs propagation (`python/packages/core/tests/core/test_as_tool_kwargs_propagation.py:111`; `dotnet/tests/Microsoft.Agents.AI.UnitTests/AgentExtensionsTests.cs:15`), race-aware session sharing, HITL propagation, and per-run loop bounds. It falls short of 8–10 because the tool boundary discards usage and structure (text-only return), there is no recursion-depth or cycle detection for agent-as-tool graphs (only workflow-graph self-loop *warnings*, `python/packages/core/agent_framework/_workflows/_validation.py:401-413`), `max_function_calls` defaults to unlimited and is best-effort even when set (`python/packages/core/agent_framework/_tools.py:1346-1351`), and child runs carry no explicit correlation IDs beyond ambient OTel spans.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Agent tool wrapper (Python) | `BaseAgent.as_tool(name, description, arg_name, arg_description, approval_mode, stream_callback, propagate_session)` returns a `FunctionTool` wrapping the agent; requires `SupportsAgentRun` | python/packages/core/agent_framework/_agents.py:608-618, :653-655 |
| Input contract | Single required string argument schema with `additionalProperties: false`; default arg name `task` | python/packages/core/agent_framework/_agents.py:663-673 |
| Output contract | Wrapper consumes the child stream and returns `final_response.text` | python/packages/core/agent_framework/_agents.py:694-714 |
| Session sharing opt-in | `propagate_session=False` default; when True, a child session shares the parent's state dict by reference but isolates `service_session_id` to avoid mutation races under concurrent `asyncio.gather` invocations | python/packages/core/agent_framework/_agents.py:682-692 |
| Approval gating on delegation | Delegated tool registered with `approval_mode="always_require"`/`"never_require"` like any other tool | python/packages/core/agent_framework/_agents.py:615, :718-724 |
| Streaming observer | Optional host-facing `stream_callback` fed released updates through the egress gate, never via transform hooks | python/packages/core/agent_framework/_agents.py:616, :700-709 |
| HITL propagation | Wrapper raises `UserInputRequiredException` if child response carries user-input requests; function layer converts them to call-id-bound function-result contents | python/packages/core/agent_framework/_agents.py:711-712; python/packages/core/agent_framework/_tools.py:1706-1722 |
| Agent-as-MCP-server | `RawAgent.as_mcp_server()` builds one MCP tool from `self.as_tool(...)` and serves it over the low-level MCP server | python/packages/core/agent_framework/_agents.py:1653-1699 |
| Workflow-as-agent | `Workflow.as_agent()` wraps the graph as `WorkflowAgent`; raises `ValueError` if the start executor cannot accept `list[Message]` | python/packages/core/agent_framework/_workflows/_workflow.py:1195-1236; _workflows/_agent.py:126-132 |
| Event forwarding boundary | Only `output`/`intermediate`/`data`(deprecated)/`request_info` events cross `workflow.as_agent()`; lifecycle/orchestration events stay internal | python/packages/core/agent_framework/_workflows/_events.py:133-141 |
| Child-run cost attribution (workflow path) | Non-streaming conversion merges `usage_details` from every nested `AgentResponse` via `add_usage_details` into the returned response | python/packages/core/agent_framework/_workflows/_agent.py:509-580 |
| Per-run loop bound | Function-invocation `max_iterations` defaults to 40 per request; each composed child run gets its own loop with its own cap | python/packages/core/agent_framework/_tools.py:95, :1392-1394 |
| Cumulative call bound | `max_function_calls` optional cumulative cap, default `None` (unlimited); documented best-effort — checked after each parallel batch completes | python/packages/core/agent_framework/_tools.py:1342-1351, :1382-1383, :2714-2715 |
| Consecutive-error bound | Tool loop abandons after `max_consecutive_errors_per_request` (default 3) | python/packages/core/agent_framework/_tools.py:95-96, :1352-1353 |
| Workflow superstep bound | Workflow-level `DEFAULT_MAX_ITERATIONS = 100` caps run/superstep loops | python/packages/core/agent_framework/_workflows/_const.py:4; _workflows/_workflow_builder.py:93 |
| Harness loop bound | `AgentLoopMiddleware.DEFAULT_MAX_ITERATIONS = 10` (judge variant 5), with approval-request escape hatch before evaluating continuation | python/packages/core/agent_framework/_harness/_loop.py:122 |
| Recursion guard (workflows only) | Graph validation detects self-loops and warns "may cause infinite recursion if not properly handled with conditions" — advisory, not blocking | python/packages/core/agent_framework/_workflows/_validation.py:401-413 |
| Nested-run tracing | Every agent run wrapped by `AgentTelemetryLayer` in an `invoke_agent` INTERNAL-kind OTel span; spans parent via async context so nested compositions form span trees; token/duration histograms recorded per invocation | python/packages/core/agent_framework/observability.py:1921, :1972, :2409-2412, :3097 |
| Nested-delegation tests (Python) | Dedicated suite proves runtime kwargs propagate verbatim through A→B→C delegation layers; context kwargs excluded from model-visible arguments | python/packages/core/tests/core/test_as_tool_kwargs_propagation.py:30-72, :74-109, :111 |
| as_tool unit tests (Python) | Basic/custom/default parameter cases, missing-name ValueError, execution, stream-callback coverage | python/packages/core/tests/core/test_agents.py:1545-1640 |
| Agent tool wrapper (.NET) | `AIAgentExtensions.AsAIFunction(agent, options, session)` creates an `AIFunction` ("Invoke an agent to retrieve some information") taking a string query and returning `response.Text` | dotnet/src/Microsoft.Agents.AI/AgentExtensions.cs:67-90 |
| .NET parent-context propagation | Parent's `FunctionInvokingChatClient.CurrentContext.Options.AdditionalProperties` forwarded onto the child run's `AgentRunOptions` | dotnet/src/Microsoft.Agents.AI/AgentExtensions.cs:76-79 |
| .NET concurrency caveat | XML docs state the resulting function is stateful and warn against concurrent use of a shared session | dotnet/src/Microsoft.Agents.AI/AgentExtensions.cs:61-65 |
| .NET cancellation/error propagation | CancellationToken flows into child `RunAsync`; exceptions propagate to the caller; name sanitized via ASCII regex | dotnet/src/Microsoft.Agents.AI/AgentExtensions.cs:71-83, :99-113 |
| AsAIFunction unit tests (.NET) | Null-agent throw, name/description inference, custom options, invocation, cancellation passing, exception propagation | dotnet/tests/Microsoft.Agents.AI.UnitTests/AgentExtensionsTests.cs:19-175 |
| Hosting wrappers | `AgentMCPTool` / `WorkflowMCPTool` publish agents/workflows as native MCP tools without owning transport/auth | python/packages/hosting-mcp/agent_framework_hosting_mcp/_agent_tool.py:22-183; _workflow_tool.py:23 |
| Reference samples | One agent used as another's tool in both languages; session-propagation sample; remote A2A skills converted to tools via `as_tool()` | dotnet/samples/02-agents/Agents/Agent_Step09_AsFunctionTool/Program.cs; python/samples/02-agents/tools/agent_as_tool_with_session_propagation.py; python/samples/02-agents/a2a/a2a_agent_as_function_tools.py |

## Answers to Dimension Questions

**1. Can one agent call another?**
Yes, through four mechanisms. (a) Locally, `agent.as_tool()` wraps any `SupportsAgentRun` implementer as a `FunctionTool` the parent places in its `tools=` list (`python/packages/core/agent_framework/_agents.py:608-652`, docstring shows coordinator/research composition). (b) In .NET, `weatherAgent.AsAIFunction()` is passed directly as the parent agent's tool (`dotnet/samples/02-agents/Agents/Agent_Step09_AsFunctionTool/Program.cs`; implementation `dotnet/src/Microsoft.Agents.AI/AgentExtensions.cs:67`). (c) Workflows compose agents downward via orchestrators and upward via `Workflow.as_agent()` (`python/packages/core/agent_framework/_workflows/_workflow.py:1195`), and since `WorkflowAgent` extends `BaseAgent` it too can be wrapped with `as_tool`. (d) Remotely, an A2A peer's advertised skills each become a `FunctionTool` on the host agent (`python/samples/02-agents/a2a/a2a_agent_as_function_tools.py`).

**2. Are child runs bounded?**
Yes, but per-run rather than globally. Each composed child run executes its own function-invocation loop with `max_iterations=40` by default and an optional cumulative `max_function_calls` (default unlimited, best-effort — enforced after each parallel batch, `_tools.py:1342-1351, :1392-1396, :2714-2715`), plus a consecutive-error cap of 3 (`:95-96`). Workflow runs are bounded at 100 supersteps (`_workflows/_const.py:4`) and harness-style loops at 10 iterations with a judge variant of 5 (`_harness/_loop.py:122`). There is no framework-wide budget spanning nesting levels: a chain of depth N gets N independent 40-iteration budgets.

**3. Are child run costs attributed?**
Partially, depending on the composition path. Through the `WorkflowAgent` boundary, usage from every nested agent response is merged with `add_usage_details` into the returned `AgentResponse.usage_details` (`python/packages/core/agent_framework/_workflows/_agent.py:509-580`), so costs roll up. Through `as_tool()`/`AsAIFunction()` — the primary composition APIs — **no**: only `final_response.text` / `response.Text` crosses the boundary (`_agents.py:714`; `AgentExtensions.cs:82`), and the child's `usage_details` are dropped. Cost observability is otherwise available out-of-band via per-invocation OTel token histograms attached to each `invoke_agent` span (`observability.py:1921, :3097`), which attributes tokens per agent but requires telemetry correlation rather than returning them to the parent programmatically.

**4. Can nested tools recurse forever?**
There is no structural recursion guard. `as_tool()` performs no cycle detection — an agent can hold a tool wrapping itself or an ancestor — and no nesting-depth counter exists anywhere in the function layer. Termination relies on the per-run bounds above (each level's 40-iteration loop eventually trips, disabling further tool calls and forcing a text response) plus workflow self-loop validation that only *warns* about possible infinite recursion (`_workflows/_validation.py:408-413`). A pathological composition can therefore still burn bounded-but-large compute: depth × 40 model round-trips, unbounded cumulative calls within each level unless `max_function_calls` is configured.

**5. Does the parent receive structured results?**
Essentially no. The contract is a single string argument in, plain text out (`_agents.py:663-673, :714`; `AgentExtensions.cs:73-82`); there are no subagent result schemas, typed payloads, or metadata envelopes on the tool path. Two structured exceptions exist: (a) child user-input/approval requests propagate as typed contents bound to the parent's `call_id` via `UserInputRequiredException` handling (`_agents.py:711-712`; `_tools.py:1706-1722`), keeping human-in-the-loop functional across the boundary; (b) the workflow path forwards typed events (`output`, `intermediate`, `request_info`) and preserves raw representations and merged usage (`_workflows/_events.py:133-141`; `_agent.py:509-585`). Runtime kwargs also flow verbatim through delegation layers for context injection, tested A→B→C (`test_as_tool_kwargs_propagation.py:111`), but result data remains text-only.

## Architectural Decisions

1. **Adapters over registries.** Composition is a closure created at construction time (`as_tool` returns a plain `FunctionTool`, `_agents.py:718-724`), so nested agents reuse the existing function-invocation pipeline (approval, middleware, limits, OTel) instead of the framework maintaining a separate nested-run/subtask protocol.
2. **Uniformity of the agent abstraction enables layered composition.** Because `SupportsAgentRun`/`BaseAgent` is the only requirement (`_agents.py:653-655`), workflows, A2A proxies, and chat-backed agents all participate identically; `WorkflowAgent` exists precisely to convert a graph into that shape (`_workflows/_agent.py:52-53`).
3. **Text-only boundary contract.** Simplicity and universal provider compatibility were chosen over rich result schemas; anything structured must ride the workflow event path or be serialized into text by the child itself.
4. **Session sharing is opt-in and race-aware.** `propagate_session` defaults to False, and even when enabled the wrapper clones a child session sharing state by reference while nulling `service_session_id`, explicitly to survive concurrent `asyncio.gather` tool invocations (`_agents.py:684-692`).
5. **Per-run bounds instead of global nesting budgets.** Loop caps live on the function-invocation configuration consumed independently by every run (`_tools.py:1392-1400`), trading guaranteed global termination for composability without hidden cross-layer coupling.
6. **HITL is the one privileged signal.** User-input requests are rethrown as exceptions and re-materialized as parent-visible contents with preserved call ids (`_tools.py:1706-1722`), ensuring approval semantics survive arbitrary nesting depth.
7. **Observability by ambient spans, not correlation fields.** Nested execution is traced by parenting each run's `invoke_agent` span in the current async context (`observability.py:2409-2412`) rather than stamping child trace IDs onto results.

## Notable Patterns

- **Child-session-by-reference clone**: shares mutable state dict across parent/child while isolating service-side conversation identity — concurrency-safe session propagation without deep copies (`_agents.py:684-692`).
- **Kwargs verbatim forwarding through delegation**: runtime kwargs pass untouched through every `as_tool` layer (asserted equal across A/B/C captures), while context kwargs are deliberately excluded from what the model sees (`test_as_tool_kwargs_propagation.py:69-72, :108-109`).
- **Name sanitization symmetry**: Python sanitizes the agent name into a tool name and refuses unnamed agents (`_agents.py:657-659`); .NET replaces all non-alphanumerics with underscores via generated regex (`AgentExtensions.cs:99-113`).
- **Self-loop linting**: workflow validation treats executor-to-itself edges as review warnings rather than errors, acknowledging intentional recursion (`_workflows/_validation.py:401-413`).
- **Gated streaming observer**: `stream_callback` observes only gate-released updates, with an explicit comment forbidding hook-registration because hooks could observe pre-verdict content (`_agents.py:700-709`).
- **Sample-driven composition idioms**: dedicated samples for agent-as-function-tool (.NET), session propagation (Python), and remote-skill-to-tool mapping over A2A keep the pattern discoverable.

## Tradeoffs

- **Cost and structure are lost at the most common boundary.** Text-only returns mean parents cannot make usage-aware decisions or consume typed sub-results; only the workflow path aggregates usage (`_workflows/_agent.py:545`).
- **No recursion safety net.** Cycle detection was left to workflow graph linting only; agent-as-tool cycles rely on model behavior plus per-level iteration caps, which multiply rather than constrain across depth.
- **Best-effort cumulative limits.** `max_function_calls` is checked after each batch, so overshoot up to the full parallel batch size is accepted by design (`_tools.py:1346-1351`).
- **Shared-session hazard documented, not prevented (.NET).** `AsAIFunction` warns about undefined behavior under concurrent use of one session instead of enforcing serialization (`AgentExtensions.cs:61-65`).
- **Internal workflow events invisible to agent callers.** The forwarded-event frozenset hides lifecycle/diagnostic/orchestration events (`_workflows/_events.py:133-141`), so parents composing a workflow see outputs but not internal progress unless they drop below the agent abstraction.

## Failure Modes / Edge Cases

- **Concurrent delegation with propagated sessions**: mitigated by the child-session clone so sibling `asyncio.gather` invocations don't mutate one session in place (`_agents.py:684-692`).
- **Sub-agent needs user input mid-run**: `UserInputRequiredException` raised inside the wrapper surfaces as call-id-bound contents in the parent's function results rather than a crash (`_agents.py:711-712`; `_tools.py:1706-1722`).
- **Middleware termination inside nested tools**: `MiddlewareTermination` results are converted back into function-result content so outer loops continue coherently (`_tools.py:1696-1705`).
- **Iteration cap exhaustion**: once the limit disables local tools, locally actionable calls and their approval requests are removed from output — a composed child cannot silently stall waiting for approvals it can no longer emit (documented loop contract in `python/packages/core/AGENTS.md`, function-invocation section).
- **Type-incompatible workflow entry points**: `Workflow.as_agent()` fails fast at construction with `ValueError` when the start executor cannot handle `list[Message]` (`_workflows/_agent.py:126-132`), catching miscomposition before runtime.
- **Unnamed agents**: `as_tool()` raises `ValueError` rather than generating opaque tool names (`_agents.py:658-659`).

## Future Considerations

- Add an opt-in nesting-depth/cycle guard for agent-as-tool composition (e.g., ambient composition-context depth counter or ancestor-set check), since per-run caps only bound each level independently.
- Offer a richer result mode for `as_tool` (structured payload + usage roll-up) so parents get what the `WorkflowAgent` path already provides.
- Attach explicit child run/correlation IDs to delegated results or `function_invocation_kwargs` to complement ambient OTel parenting.
- Make `max_function_calls` enforceable pre-batch (or document worst-case overshoot prominently) for cost-sensitive compositions.
- Port the Python session-propagation safeguards (child-session clone) to .NET's `AsAIFunction`, which currently documents the concurrency hazard without mitigation.

## Questions / Gaps

- **No subagent result schemas found.** Dimension evidence targets "subagent result schemas"; nothing beyond the single-string contract exists on the tool path — structured outputs inside a child agent terminate at its own response and are flattened to text (`_agents.py:714`).
- **No child trace IDs surfaced to callers.** Nesting is observable only through OTel span hierarchy (`observability.py:2409-2412`); programmatic access to a child's run id from the parent's tool result is absent.
- **Recursion prevention unverified at scale.** No test or runtime mechanism demonstrates long-cycle termination; analysis concludes bounded-by-caps rather than prevented, and no benchmark of deeply nested compositions exists in-repo.
- **Go implementation out of scope**: the Go directory is a pointer stub to a separate repository (`go/README.md:1-3`), so composition behavior there could not be verified from this source.

---

Generated by dimension `04.08-agent-as-tool-and-workflow-as-tool-composition` against `agent-framework`.
