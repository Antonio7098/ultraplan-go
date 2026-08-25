# Source Analysis: langgraph

## Dimension 04.08: Agent-as-Tool and Workflow-as-Tool Composition

### Source Info

| Field | Value |
|-------|-------|
| Name | langgraph |
| Path | `studies/agent-harness-study/sources/langgraph` |
| Language / Stack | Python (core runtime + prebuilt agents); JS/TS SDK for the server API |
| Analyzed | 2026-08-23 |

Citation convention: paths are relative to the selected source directory (`studies/agent-harness-study/sources/langgraph`). Line numbers refer to the files as present in this snapshot.

## Summary

LangGraph's composition model is "everything is a `Runnable`". A compiled graph (`CompiledStateGraph`, extending `Pregel`) is itself a LangChain `Runnable`, so a higher-level capability — a ReAct agent, a workflow, or an entire sub-workflow — is exposed for reuse in two primary ways:

1. **Graph-as-node (subgraph embedding).** `StateGraph.add_node(...)` accepts any `Runnable`; when given a compiled graph it derives the node name from `action.get_name()` and treats it as a node whose execution runs a nested Pregel loop (`libs/langgraph/langgraph/graph/state.py:778-787`, class hierarchy at `libs/langgraph/langgraph/graph/state.py:1404-1407`). The prebuilt `create_react_agent` returns a `CompiledStateGraph` and its `name` parameter is documented specifically for embedding the agent as a subgraph node when building multi-agent systems (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:274-308, 466-468`).
2. **Tool-node composition.** `ToolNode` executes model-requested tool calls in parallel and returns `ToolMessage`s or `Command`s (`libs/prebuilt/langgraph/prebuilt/tool_node.py:649-660, 793-826`). Tools may themselves be anything invocable with a config, including other agents wrapped as LangChain tools; there is no langgraph-specific agent-tool wrapper — the repo relies on langchain-core's generic `Runnable` machinery (a search for `as_tool` across all Python sources returned zero definitions in this repository).

Nested graphs are discovered by bytecode/closure introspection (`find_subgraph_pregel`, `libs/langgraph/langgraph/pregel/_utils.py:45-73`) so checkpoints, streaming, and state reads work across the nesting boundary. Control and data can be pushed upward through `Command(graph=Command.PARENT, ...)` (`libs/langgraph/langgraph/types.py:799-848`), and every nested invocation is bounded by its own `recursion_limit`-derived superstep budget (`self.stop = self.step + self.config["recursion_limit"] + 1`, `libs/langgraph/langgraph/pregel/_loop.py:1700-1701`), enforced by raising `GraphRecursionError` (`libs/langgraph/langgraph/errors.py:67-87`; raised at `libs/langgraph/langgraph/pregel/main.py:3005-3011` sync and `3486-3492` async).

The main gaps: there is **no cross-level budget** (each nesting level inherits the numeric `recursion_limit` and restarts its own counter, so worst-case work scales multiplicatively with depth × fan-out), and **no built-in per-child cost/token attribution** was found in the core runtime.

## Rating

**7 / 10**

Rationale against the rubric:

- **Clear model with explicit interfaces (7–8 band):** composition is uniform (any `Runnable` can be a node, `libs/langgraph/langgraph/graph/state.py:778-787`); input/output contracts are typed and JSON-schema-exposed (`libs/langgraph/langgraph/graph/state.py:1424-1442, 1515-1523`); upward control flow is an explicit primitive (`Command.PARENT`, `libs/langgraph/langgraph/types.py:806, 848`).
- **Operational safeguards:** per-invocation recursion cap validated at entry (`recursion_limit < 1` rejected, `libs/langgraph/langgraph/pregel/main.py:2563-2564`), `GraphRecursionError` with stable error code (`libs/langgraph/langgraph/errors.py:34-39, 67-87`), interrupt/drain/cancel semantics defined (`libs/langgraph/langgraph/errors.py:50-64, 102-104, 168-206`).
- **Tests:** recursion-limit enforcement (`libs/langgraph/libs/langgraph/tests/test_pregel.py:588-589`), nested-subgraph parent commands (`libs/langgraph/libs/langgraph/tests/test_parent_command.py:9-53`), subgraph interrupt persistence/resume (`libs/langgraph/libs/langgraph/tests/test_time_travel.py:1323, 1416`), drain-from-subgraph resuming the parent (`libs/langgraph/libs/langgraph/tests/test_runtime.py:205`).
- **Why not higher:** budgets do not compose across levels (no global step/token ceiling), cost attribution is absent from the core, subgraph detection relies on best-effort introspection that fails silently (`libs/langgraph/libs/langgraph/tests/test_subgraph_detection.py:1-9` states "Detection failing is silent"), and the flagship agent factory in this repo is deprecated in favor of an out-of-repo `langchain.agents.create_agent` (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:53-56, 274-277`).

## Evidence Collected

Every entry includes a file path with line numbers, relative to the selected source directory.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Compiled graph is a Runnable (composable unit) | `CompiledStateGraph(Pregel[...])`; `Pregel` implements LangChain's `Runnable` interface | `libs/langgraph/langgraph/graph/state.py:1404-1407`; `libs/langgraph/langgraph/pregel/protocol.py:25` |
| Graph accepted as node | `add_node`: `if isinstance(action, Runnable): node = action.get_name()` | `libs/langgraph/langgraph/graph/state.py:778-787` |
| Prebuilt agent designed as subgraph/tool building block | `create_react_agent -> CompiledStateGraph`; `name` used "when adding ReAct agent graph to another graph as a subgraph node - particularly useful for building multi-agent systems"; agent name stamped onto AIMessage | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:274-308, 466-468, 682, 710` |
| Nested graph discovery | `find_subgraph_pregel` walks Runnable sequences/lambdas/nonlocals for `PregelProtocol`; skips subgraphs with `checkpointer is False` | `libs/langgraph/langgraph/pregel/_utils.py:45-73` |
| Detection wired into nodes | `PregelNode` populates `self.subgraphs` via `find_subgraph_pregel(self.bound)` | `libs/langgraph/langgraph/pregel/_read.py:150, 187-199` |
| Detection failure modes pinned by tests | Module docstring: detection failing is silent; shapes (closure, attr chain, list subclass) enumerated | `libs/langgraph/libs/langgraph/tests/test_subgraph_detection.py:1-9` |
| Input/output contracts | JSON schemas for graph input/output; per-node `input_schema` with mapper selection | `libs/langgraph/langgraph/graph/state.py:1424-1442, 1515-1523` |
| Subgraph result filtering | `_get_updates` filters child updates to parent `output_keys`; `Command(graph=PARENT)` yields no local writes | `libs/langgraph/langgraph/graph/state.py:1456-1481` |
| Upward control primitive | `Command` dataclass with `graph: None \| Command.PARENT`, `update`, `resume`, `goto`; `PARENT = "__parent__"` | `libs/langgraph/langgraph/types.py:799-848` |
| Parent-command plumbing | Root-level `Command.PARENT` raises `InvalidUpdateError("There is no parent graph")`; retry layer handles PARENT commands; `update_state` honors PARENT | `libs/langgraph/langgraph/pregel/_io.py:56-59`; `libs/langgraph/langgraph/pregel/_retry.py:627, 767`; `libs/langgraph/langgraph/graph/state.py:1761, 1794-1805` |
| Parallel child tool execution | `ToolNode._func` fans tool calls out via `executor.map` over per-call configs; async path uses `asyncio.gather` | `libs/prebuilt/langgraph/prebuilt/tool_node.py:793-826, 828-860` |
| Fan-out routing (Send API) | v2 agent routes each tool call as a separate `Send("tools", ToolCallWithContext(...))` task | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:846-859` |
| Tool error contract | `handle_tool_errors`: True/str/type/tuple/callable, `False` re-raises; default handler catches invocation errors only | `libs/prebuilt/langgraph/prebuilt/tool_node.py:674-694, 987-1006` |
| Recursion budget per invocation | `self.step = checkpoint step + 1`; `self.stop = self.step + self.config["recursion_limit"] + 1` (sync and async loops) | `libs/langgraph/langgraph/pregel/_loop.py:1700-1701, 1961` |
| Recursion limit config | `DEFAULT_RECURSION_LIMIT = int(getenv("LANGGRAPH_DEFAULT_RECURSION_LIMIT", "10007"))`; merge preserves caller-set values; `< 1` rejected | `libs/langgraph/langgraph/_internal/_config.py:32, 184-186, 335`; `libs/langgraph/langgraph/pregel/main.py:2563-2564` |
| Recursion error type/code | `GraphRecursionError(RecursionError)` with `ErrorCode.GRAPH_RECURSION_LIMIT`; raised in both sync/async streams | `libs/langgraph/langgraph/errors.py:34-35, 67-87`; `libs/langgraph/langgraph/pregel/main.py:3005-3011, 3486-3492` |
| Soft step budget for agents | Managed `RemainingSteps = stop - step`; react agent returns "Sorry, need more steps..." instead of raising when exhausted | `libs/langgraph/langgraph/managed/is_last_step.py:9-21`; `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:434-440, 620-634` |
| Nested tracing (callbacks) | Loop accepts `ParentRunManager`/`AsyncParentRunManager`; callback manager created from config with `ls_integration: langgraph`; graph lifecycle events (`on_interrupt`, `on_resume`) emitted | `libs/langgraph/langgraph/pregel/_loop.py:24, 175, 292`; `libs/langgraph/langgraph/pregel/main.py:2772-2798, 2891-2896`; `libs/langgraph/langgraph/pregel/_algo.py:22, 384` |
| Message streams across nesting boundary | Stream handler filters by namespace so nested subgraph LLM tokens route correctly depending on `subgraphs=True/False` | `libs/langgraph/langgraph/pregel/_messages.py:63-92` |
| Interrupt aggregation | Runner combines `GraphInterrupt`s raised across sibling tasks into one | `libs/langgraph/langgraph/pregel/_runner.py:690` |
| Interrupt lifecycle | Loop raises `GraphInterrupt` before/after configured nodes; `GraphInterrupt` documented as "suppressed by the root graph" | `libs/langgraph/langgraph/pregel/_loop.py:667-671, 720-724`; `libs/langgraph/langgraph/errors.py:102-104` |
| Cancellation & cooperative drain | `NodeCancelledError` converts user-raised `asyncio.CancelledError` into visible node failure; `GraphDrained` stops cooperatively at superstep boundary (e.g., SIGTERM) | `libs/langgraph/langgraph/errors.py:54-64, 168-186` |
| Node timeouts | `NodeTimeoutError` with idle/run kinds; async-only ("sync nodes cannot be safely cancelled in-process"); timed-attempt scope guards send/stream/callbacks after timeout | `libs/langgraph/langgraph/errors.py:190-206`; `libs/langgraph/langgraph/graph/state.py:715-722`; `libs/langgraph/langgraph/pregel/_retry.py:154-260` |
| Policy inheritance boundary | `set_node_defaults`: retry/cache/timeout defaults "are **not** inherited by subgraphs" | `libs/langgraph/langgraph/graph/state.py:280-284` |
| Resume signaling into subgraphs | Retry layer patches config and "signal[s] subgraphs to resume (if available)" | `libs/langgraph/langgraph/pregel/_retry.py:675-682, 831-838` |
| Tests: hard bound enforced | `pytest.raises(GraphRecursionError)` with `{"recursion_limit": 1}` | `libs/langgraph/libs/langgraph/tests/test_pregel.py:588-589`; async twin `tests/test_pregel_async.py:1584-1585` |
| Tests: nested parent command jump | `test_parent_command_from_nested_subgraph` — child node redirects parent to `parent_second`, aborting remaining child calls | `libs/langgraph/libs/langgraph/tests/test_parent_command.py:9-53` |
| Tests: subgraph interrupt durability | Full interrupt/resume flows with and without child checkpointer | `libs/langgraph/libs/langgraph/tests/test_time_travel.py:1323, 1416`; `tests/test_pregel.py:3326`; `tests/test_pregel_async.py:5082` |
| Tests: drain propagation | `test_drain_from_subgraph_can_resume_parent` | `libs/langgraph/libs/langgraph/tests/test_runtime.py:205` |

## Answers to Dimension Questions

1. **Can one agent call another?**
   Yes, via three mechanisms. (a) *Subgraph node*: any compiled graph can be added as a node (`libs/langgraph/langgraph/graph/state.py:778-787`), and `create_react_agent` is explicitly shaped for this — its `name` becomes the subgraph node name in multi-agent systems (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:466-468`) and is stamped onto produced AIMessages for provenance (`chat_agent_executor.py:682`). (b) *As a tool*: a compiled graph is a `Runnable`, so langchain-core's generic tool wrapping applies; notably, this repository defines **no** `as_tool` implementation of its own (repo-wide search returned none) — the capability is delegated to the langchain-core dependency. (c) *Inside a tool body*: a tool function can simply `.invoke()` another graph, and `find_subgraph_pregel` will still discover it via closure/nonlocal inspection (`libs/langgraph/langgraph/pregel/_utils.py:45-73`), preserving checkpoint/stream integration.

2. **Are child runs bounded?**
   Bounded, but **per level, not globally**. Every graph invocation constructs its own loop with `stop = step + recursion_limit + 1` (`libs/langgraph/langgraph/pregel/_loop.py:1700-1701, 1961`), and exceeding it raises `GraphRecursionError` (`libs/langgraph/langgraph/errors.py:67-87`; enforcement at `libs/langgraph/langgraph/pregel/main.py:3005-3011`). The child inherits the *numeric* config value from the parent (config merge keeps caller-set `recursion_limit`, `libs/langgraph/langgraph/_internal/_config.py:184-186`; default `10007`, env-overridable, `_internal/_config.py:32`), but the counter restarts for each nesting level. A depth-*d* chain therefore admits up to ~`d × limit` supersteps, and fan-out via `Send` multiplies concurrent child budgets (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:846-859`). Agents additionally get a soft budget: managed `RemainingSteps` lets a react agent degrade gracefully ("Sorry, need more steps") rather than crash (`libs/langgraph/langgraph/managed/is_last_step.py:18-21`; `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:434-440, 620-634`).

3. **Are child run costs attributed?**
   **No clear evidence found** for in-runtime cost/token attribution. Searches for cost/usage aggregation in the core (`langgraph/`) surfaced nothing beyond standard message `usage_metadata` carried on LLM messages. What does exist is structural support for external attribution: nested runs appear as child spans because callback managers propagate through config into nested loops (`ParentRunManager`, `libs/langgraph/langgraph/pregel/_loop.py:24, 175`; manager wiring at `libs/langgraph/langgraph/pregel/main.py:2772-2798`), and names/tags identify children (agent `name` on AIMessages, `chat_agent_executor.py:682`). Cost roll-up is thus deferred to the tracing backend, not provided by the harness.

4. **Can nested tools recurse forever?**
   Not within a single invocation: the superstep cap plus `GraphRecursionError` guarantees termination per loop instance (tested at `libs/langgraph/libs/langgraph/tests/test_pregel.py:588-589`). However, **no cross-level recursion guard exists**: each nested invocation receives a fresh budget, so a cyclic agent-as-tool topology can keep allocating new budgets indefinitely across levels, and parallel `Send` fan-out multiplies concurrent child executions. Two mitigations exist but neither is a depth guard: `MULTIPLE_SUBGRAPHS` error code catches ambiguous detection (`libs/langgraph/langgraph/errors.py:38`), and misuse of `Command.PARENT` at the root fails fast (`libs/langgraph/langgraph/pregel/_io.py:56-59`).

5. **Does the parent receive structured results?**
   Yes. The default contract is state-shaped: child outputs are dicts filtered to the parent's declared output keys (`_get_updates`, `libs/langgraph/langgraph/graph/state.py:1456-1468`), and schemas are published as JSON Schema (`state.py:1424-1442`). For tool-mediated composition the parent gets `ToolMessage`s, or richer `Command` objects carrying `update`/`goto`/`resume` (`libs/langgraph/langgraph/types.py:799-824`), with `Command(graph=PARENT)` letting a child mutate/navigate the *parent* — combined across parallel calls by `ToolNode._combine_tool_outputs` (`libs/prebuilt/langgraph/prebuilt/tool_node.py:895-920`) and honored by the parent's write mapper and `update_state` (`libs/langgraph/langgraph/graph/state.py:1463-1481, 1761, 1794-1805`). End-to-end behavior is pinned by `test_parent_command_from_nested_subgraph` (`libs/langgraph/libs/langgraph/tests/test_parent_command.py:9-53`). The react agent additionally offers `response_format` → `structured_response` key (`chat_agent_executor.py:373-394`).

## Architectural Decisions

- **Uniform composition substrate.** Rather than a bespoke "agent tool" abstraction, everything reduces to `Runnable`-in-graph-node. This makes agent/tool/workflow composition syntactically identical (`libs/langgraph/langgraph/graph/state.py:778-787`) at the cost of relying on an external library for the `as_tool` ergonomic.
- **Implicit subgraph discovery instead of registration.** Nested graphs are found by walking runnables and function closures/bytecode (`find_subgraph_pregel`, `libs/langgraph/langgraph/pregel/_utils.py:45-73`; closure walker at `_utils.py:161-216`). This removes boilerplate (no manifest needed) but trades reliability for convenience — the dedicated test file enumerates shapes where detection silently degrades to "introspection goes quiet" (`libs/langgraph/libs/langgraph/tests/test_subgraph_detection.py:1-9`).
- **Shared-config namespace isolation.** Children inherit the parent config (callbacks, `recursion_limit`, store) while the checkpoint namespace (`checkpoint_ns`) is patched per task, giving hierarchical traces and hierarchical checkpoints without a separate child-run API (`libs/langgraph/langgraph/pregel/_loop.py:1640-1650` namespace handling; `libs/langgraph/langgraph/_internal/_config.py:52-60` patching helper).
- **Two-tier step limiting.** A hard tier (`GraphRecursionError`) protects the engine; a soft tier (`RemainingSteps` managed value) lets agents produce user-visible degradation instead of exceptions (`libs/langgraph/langgraph/managed/is_last_step.py:9-21`; `chat_agent_executor.py:684-692`).
- **Explicit escape hatch to the parent.** `Command(graph=Command.PARENT)` formalizes "bubbling" control decisions upward — navigation (`goto`), state writes (`update`), and resume values — instead of ad-hoc exception passing (`libs/langgraph/langgraph/types.py:799-848`).
- **Policy non-inheritance across boundaries.** Node-level retry/cache/timeout defaults stop at subgraph edges (`libs/langgraph/langgraph/graph/state.py:280-284`), forcing deliberate per-graph policy but allowing parent/child policies to diverge silently.

## Notable Patterns

- **Agent-name stamping for provenance.** The producing agent's `name` is written onto each `AIMessage` (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:681-682, 709-710`), so parents can attribute downstream messages in shared `messages` state — a lightweight alternative to envelope-based child results.
- **Per-call config fan-out.** `ToolNode` derives one config per tool call via `get_config_list` and executes them concurrently (sync executor map / async gather), keeping callback context correct per parallel child (`libs/prebuilt/langgraph/prebuilt/tool_node.py:799-826, 834-860`).
- **Namespace-aware stream filtering.** Stream handlers decide whether nested-subgraph events surface based on namespace comparison (`len(ns) > 0 and ns != self.parent_ns`), enabling `subgraphs=False` suppression without losing nested LLM tokens entirely (`libs/langgraph/langgraph/pregel/_messages.py:63-92`).
- **Guarded channels after faults.** The timed-attempt scope wraps `send`/`stream`/`call` configurables with guards so a timed-out node cannot keep writing into the parent graph (`libs/langgraph/langgraph/pregel/_retry.py:169-260`).
- **Interrupt aggregation across siblings.** Multiple simultaneous child interrupts are merged into a single `GraphInterrupt` for the parent (`libs/langgraph/langgraph/pregel/_runner.py:690`), matching the bulk-synchronous execution model.

## Tradeoffs

- **Simplicity of shared-state subgraphs vs. contract rigidity.** By default a child sees/writes the parent's channels; narrowing requires explicit `input_schema=`/`output_schema=` declarations (`libs/langgraph/langgraph/graph/state.py:1515-1523`). Flexible, but accidental channel coupling between parent and child agents is easy until schemas are tightened.
- **Introspection-based detection vs. explicitness.** Zero-registration nesting is elegant, but detection is best-effort, version-sensitive (bytecode walking), and fails silently (`libs/langgraph/langgraph/pregel/_utils.py:161-216`; `tests/test_subgraph_detection.py:1-9`) — checkpoints and stream events for missed subgraphs quietly vanish.
- **Per-level budgets vs. global safety.** Restarting `recursion_limit` per level keeps each graph independently tunable, but total work grows with depth × breadth; a malicious or buggy agent-as-tool cycle has no harness-enforced global ceiling short of wall-clock timeouts (`libs/langgraph/langgraph/pregel/_loop.py:1701`; timeouts at `errors.py:190-206`).
- **Sync composability vs. cancellability.** Timeouts/idle-cancellation apply only to async nodes; sync nodes "cannot be safely cancelled in-process," so a synchronous nested agent can overshoot its deadline while still completing (`libs/langgraph/langgraph/graph/state.py:715-722`; honest caveat documented at `libs/langgraph/langgraph/pregel/_utils.py:107-130`).

## Failure Modes / Edge Cases

- **Silent loss of subgraph integration** when detection misses a nested graph (e.g., graph held in containers the walker skips) — documented as expected behavior, not an error (`libs/langgraph/libs/langgraph/tests/test_subgraph_detection.py:1-9`).
- **Ambiguous multiple subgraphs** behind one node surfaces as `MULTIPLE_SUBGRAPHS` error code (`libs/langgraph/langgraph/errors.py:38`).
- **`Command.PARENT` without a parent** raises `InvalidUpdateError("There is no parent graph")` immediately (`libs/langgraph/langgraph/pregel/_io.py:56-59`).
- **Budget exhaustion mid-child** raises `GraphRecursionError` up the stack, but because checkpoints are written per superstep, interrupted trees remain resumable (interrupt/resume flows tested at `libs/langgraph/libs/langgraph/tests/test_time_travel.py:1323, 1416`).
- **User-raised `asyncio.CancelledError`** would otherwise look like framework teardown; it is converted to `NodeCancelledError` so the run reports `error` honestly (`libs/langgraph/langgraph/errors.py:168-186`).
- **Async-only wrappers under idle timeout** may pass native-async checks yet delegate to blocking sync work that keeps running past the deadline — called out explicitly in code (`libs/langgraph/langgraph/pregel/_utils.py:107-130`).
- **Parallel tool-call partial failure:** default `handle_tool_errors` converts model-caused invocation errors into error-text `ToolMessage`s (so the loop continues) but re-raises execution errors unless configured otherwise (`libs/prebuilt/langgraph/prebuilt/tool_node.py:674-694, 987-1006`) — a mixed outcome parents must anticipate.
- **Deprecated composition entrypoint:** `create_react_agent` and its state classes carry `deprecated` markers pointing to out-of-repo `langchain.agents` (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:53-56, 65-74, 274-277`), so new composition code targeting this snapshot risks building on a moving API.

## Future Considerations

- Introduce an optional **global (cross-level) step/token budget** threaded through config so deeply nested agent-as-tool graphs have a harness-enforced ceiling, complementing per-loop `recursion_limit`.
- Provide **first-class child-run cost attribution**: aggregate message `usage_metadata` per subtree keyed by checkpoint namespace, exposed alongside state history — currently absent from the core.
- Replace best-effort closure introspection with an **explicit subgraph registration/marker API** (or at least a warning mode), converting silent detection failures into loud diagnostics (`MULTIPLE_SUBGRAPHS` shows the pattern exists).
- Add a **nesting-depth metric/guard** (max ancestor count derived from `checkpoint_ns` segments) for operators who want cycle protection stronger than wall-clock timeouts.
- Extend timeout/cancellation coverage to **sync nodes**, closing the gap where nested sync agents outrun their deadlines (`libs/langgraph/langgraph/graph/state.py:715-722`).
- Stabilize or fully extract the deprecated prebuilt agent factory so composition examples in-repo match the supported path.

## Questions / Gaps

- **Cost accounting:** Does the LangSmith/platform layer (outside this source) perform per-child token/cost roll-up? Within this repository: no evidence found; searched for cost/usage aggregation symbols across `libs/langgraph` and `libs/prebuilt`.
- **`as_tool` ergonomics:** The canonical agent-to-tool adapter lives in langchain-core (external dependency), so its exact contract (schema derivation, error strategy) could not be verified from this source alone. In-repo searches for `as_tool` returned zero definitions.
- **JS parity:** Composition semantics were analyzed from the Python runtime (`libs/langgraph`, `libs/prebuilt`). Whether `sdk-js` exposes equivalent nested-run guarantees was not examined; the SDK targets the REST server API rather than embedding a runtime.
- **RemotePregel nesting:** `libs/langgraph/langgraph/pregel/remote.py:161` documents adding a `RemoteGraph` as a subgraph node, but end-to-end recursion-limit propagation for remote children was not traced in this pass.
- **Default recursion limit value:** this snapshot reads `LANGGRAPH_DEFAULT_RECURSION_LIMIT` with fallback `"10007"` (`libs/langgraph/langgraph/_internal/_config.py:32`); upstream documentation commonly cites 25. The observed value is reported as-is from the source under study; no claim is made about which is authoritative.

---

Generated by `04.08-agent-as-tool-and-workflow-as-tool-composition` against `langgraph`.
