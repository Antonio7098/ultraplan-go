# Source Analysis: langgraph

## Dimension 10.01: Span Hierarchy and Run Tree

### Source Info

| Field | Value |
|-------|-------|
| Name | langgraph |
| Path | `studies/agent-harness-study/sources/langgraph` |
| Language / Stack | Python (core `libs/langgraph`), plus Python/JS SDKs (`libs/sdk-py`, `libs/sdk-js`); tracing delegated to the LangChain-core callback/tracer system and LangSmith |
| Analyzed | 2026-08-24 |

## Summary

LangGraph does not implement its own span exporter. It builds a single run tree per invocation by threading LangChain's callback-manager chain through every layer of the Pregel execution engine: `Pregel.stream` opens one root `on_chain_start` run (`libs/langgraph/langgraph/pregel/main.py:2792-2797`), the loop passes that run's manager into task preparation (`libs/langgraph/langgraph/pregel/_loop.py:622`), and each node/task is spawned with `callbacks=manager.get_child(f"graph:step:{step}")` so it becomes a child span of the graph run (`libs/langgraph/langgraph/pregel/_algo.py:706-720`). Node functions are wrapped in runnables that open their own runs, patch child callbacks into the function's config, and set a contextvar/LangSmith parent-tracing context so nested LLM calls, tool calls, and even LangSmith `@traceable` functions attach as grandchildren (`libs/langgraph/langgraph/_internal/_runnable.py:421-445`, `libs/langgraph/langgraph/_internal/_runnable.py:88-140`). Subgraphs invoked inside a node simply inherit the config, so their own root run nests under the calling node's run. Streaming surfaces (`messages`, `tools`, `debug`) and graph lifecycle events (interrupt/resume) are correlated back to the tree via per-task metadata (`langgraph_step`, `langgraph_node`, `langgraph_checkpoint_ns`) injected at task creation (`libs/langgraph/langgraph/pregel/_algo.py:654-660`). Cross-process propagation exists only in the platform direction: the Python SDK can ask the server to route traces to a specific LangSmith project (`langsmith_tracer` payload key), but no W3C `traceparent`-style header propagation exists anywhere in this source. A per-node `TracePolicy` allows transforming recorded trace payloads (`libs/langgraph/langgraph/types.py:532-558`).

## Rating

**7 / 10 — Clear model with tests, explicit interfaces, and operational safeguards.**

Rationale:
- The hierarchy model is explicit and uniform: root graph run → step tasks → node runnable run → nested model/tool/user-code runs, enforced mechanically by callback-manager chaining at exactly three well-defined sites (root: `main.py:2792`; task: `_algo.py:716-720`; node: `_runnable.py:423-430`).
- Tree shape is regression-tested with a dedicated tracer harness asserting `child_runs`, `parent_run_id`, inputs/outputs (`tests/fake_tracer.py:10-91`, `tests/test_pregel_async.py:2324-2331`), and LangSmith interop (`dotted_order`/`trace_id` continuity) is covered by `tests/test_tracing_interops.py:109-118`.
- Operational safeguards exist: `TracePolicy` payload redaction that fails open (`_runnable.py:76-85`), `TAG_HIDDEN` suppression of framework internals from traces/streams (`constants.py:26-27`), and lifecycle interrupt/resume events carrying run/checkpoint identity (`callbacks.py:42-76`).
- It falls short of 9-10 because: the flagship end-to-end LangSmith interop test is `@pytest.mark.skip("This test times out in CI")` (`test_tracing_interops.py:60`); there is no first-class OpenTelemetry support — OTel arrives only via third-party monkey-patching that LangGraph merely avoids breaking (`tests/test_graph_callbacks.py:281-295`); cross-process trace-ID propagation is vendor-specific (server-side `langsmith_tracer`) rather than an open standard; and "guardrails" have no span representation at all.

## Evidence Collected

Every entry cites paths relative to the source root `studies/agent-harness-study/sources/langgraph`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Trace provider | Tracing is delegated to any LangChain tracer passed via `config["callbacks"]`; code explicitly looks up `LangChainTracer` in handlers to bind run objects | `libs/langgraph/langgraph/_internal/_runnable.py:432-437` |
| Trace provider (LangSmith) | Node execution sets LangSmith's internal parent-tracing context (`_set_tracing_context({"parent": run})`) so `@traceable` calls nest under the node run | `libs/langgraph/langgraph/_internal/_runnable.py:96-101` |
| Trace provider (OTel) | OTel support is external: test documents `opentelemetry-instrumentation-langchain` monkey-patching `BaseCallbackManager.__init__`; LangGraph only guarantees non-crash compat | `libs/langgraph/tests/test_graph_callbacks.py:281-295` |
| Root span creation (sync) | Root run opened with `on_chain_start(None, input, name=config.get("run_name", self.get_name()), run_id=config.get("run_id"))`; metadata `ls_integration: "langgraph"` added; closed with `on_chain_end(loop.output)` / `on_chain_error(e)` | `libs/langgraph/langgraph/pregel/main.py:2790-2797`, `libs/langgraph/langgraph/pregel/main.py:3017-3021` |
| Root span creation (async) | Async equivalents at `callback_manager.on_chain_start(...)` and `await run_manager.on_chain_end/on_chain_error` | `libs/langgraph/langgraph/pregel/main.py:3205`, `libs/langgraph/langgraph/pregel/main.py:3498-3501` |
| Parent-child linkage (task level) | Loop forwards the root `manager` into task preparation; every PULL/Send/PUSH task gets patched `run_name` + `callbacks=manager.get_child(f"graph:step:{step}")` | `libs/langgraph/langgraph/pregel/_loop.py:622`, `libs/langgraph/langgraph/pregel/_algo.py:711-720`, `libs/langgraph/langgraph/pregel/_algo.py:898-902`, `libs/langgraph/langgraph/pregel/_algo.py:1065-1070` |
| Span definitions (node) | `RunnableCallable.invoke`: `on_chain_start` → `patch_config(config, callbacks=run_manager.get_child())` → run func in `set_config_context(child_config, run)` → `on_chain_end/on_chain_error` | `libs/langgraph/langgraph/_internal/_runnable.py:421-445` |
| Span definitions (node seq) | `RunnableSeq.invoke` opens the node run (applying `trace_inputs`) and marks each inner step as child run `seq:step:N`; outputs recorded through `trace_outputs` | `libs/langgraph/langgraph/_internal/_runnable.py:680-716` |
| Per-run identity/metadata | Every task config carries `langgraph_step`, `langgraph_node`, `langgraph_triggers`, `langgraph_path`, `langgraph_checkpoint_ns` metadata; `Runtime.execution_info` carries checkpoint/task/thread/run ids | `libs/langgraph/langgraph/pregel/_algo.py:654-660`, `libs/langgraph/langgraph/pregel/_algo.py:688-700` |
| Model-call spans | Chat-model runs stream/correlate via `StreamMessagesHandler.on_chat_model_start` keyed by `run_id`, namespace derived from `metadata["langgraph_checkpoint_ns"]`; attached as inheritable handler on the root run | `libs/langgraph/langgraph/pregel/_messages.py:130-149`, `libs/langgraph/langgraph/pregel/main.py:2821-2827` |
| Tool-call spans | `StreamToolCallHandler` fires on `on_tool_*` callbacks emitting `tool-started/tool-output-delta/tool-finished/tool-error`; correlates start→end by `run_id` | `libs/langgraph/langgraph/pregel/_tools.py:35-53`, `libs/langgraph/langgraph/pregel/_tools.py:80-85` |
| Nested execution (functional API) | `@task` futures carry the caller's `config["callbacks"]` (`call.callbacks`) so task spans nest under the invoking entrypoint/node run | `libs/langgraph/langgraph/pregel/_call.py:288-297`, `libs/langgraph/langgraph/pregel/_algo.py:901` |
| Nested execution (subgraphs) | Compiled subgraphs as nodes inherit the patched config; subgraph root run nests under the node run; messages/tools handlers dedupe explicit sub-subgraph streams using `parent_ns` | `libs/langgraph/langgraph/pregel/_messages.py:60-95`, `libs/langgraph/langgraph/pregel/_tools.py:87-119` |
| Distributed tracing headers | None found. No `traceparent`/W3C trace-context or B3 headers anywhere in repo (searched `*.py/*.ts/*.js`). SDKs accept custom `headers` but add none themselves | `libs/sdk-py/langgraph_sdk/_shared/utilities.py:51-69` (auth/custom headers only) |
| Cross-process trace routing (platform) | Python SDK `LangSmithTracing {project_name, example_id}` sent as `langsmith_tracer` in run-create/stream payloads; client learns server-side `run_id` from response metadata (`Content-Location`) | `libs/sdk-py/langgraph_sdk/schema.py:111-117`, `libs/sdk-py/langgraph_sdk/_async/runs.py:338`, `libs/sdk-py/langgraph_sdk/_shared/utilities.py:103-110` |
| Remote run streaming | Local adapter wraps SDK thread stream (`run.start` returns `run_id`); projections (`values/messages/tool_calls/subgraphs/tasks/...`) mirror local stream modes | `libs/langgraph/langgraph/pregel/_remote_run_stream.py:126-159`, `libs/langgraph/langgraph/pregel/_remote_run_stream.py:80-107` |
| Trace viewer/UI | External: `langgraph dev` launches LangGraph Studio UI; README/docs point observability at LangSmith | `libs/cli/langgraph_cli/cli.py:353`, `README.md:42`, `docs/llms.txt:21` |
| In-repo textual trace view | `stream_mode="debug"` emits `task`/`task_result`/`checkpoint` events including task ids, names, triggers, errors, interrupts, and next-tasks | `libs/langgraph/langgraph/pregel/debug.py:41-71`, `libs/langgraph/langgraph/pregel/debug.py:144-206` |
| Hidden spans safeguard | `TAG_HIDDEN = "langsmith:hidden"` tags framework-internal nodes; filtered from debug tasks, message streams, tool streams, node_finished hooks | `libs/langgraph/langgraph/constants.py:26-27`, `libs/langgraph/langgraph/pregel/debug.py:44`, `libs/langgraph/langgraph/pregel/_messages.py:205`, `libs/langgraph/langgraph/pregel/_runner.py:605-608` |
| Trace redaction policy | `TracePolicy(process_inputs/process_outputs)` + `omit_payload` helper; wired into node runnables; processors fail open recording untransformed payloads | `libs/langgraph/langgraph/types.py:532-567`, `libs/langgraph/langgraph/pregel/_read.py:226-248`, `libs/langgraph/langgraph/_internal/_runnable.py:76-85` |
| Lifecycle spans beyond run tree | `GraphCallbackHandler.on_interrupt/on_resume` receive `run_id`, status, `checkpoint_id`, `checkpoint_ns` tuple — resume across process restarts re-associates with the same logical run | `libs/langgraph/langgraph/callbacks.py:42-111`, `libs/langgraph/langgraph/pregel/main.py:2798-2801`, `libs/langgraph/langgraph/pregel/main.py:2888-2897` |
| Callbacks as progress bus | Idle-timeout scope injects `_IdleProgressCallbackHandler` via merged `config["callbacks"]`; touches on every `on_llm_*`/`on_chain_*`/`on_tool_*`/`on_retriever_*` event of descendant runs only | `libs/langgraph/langgraph/pregel/_retry.py:189-192`, `libs/langgraph/langgraph/pregel/_retry.py:274-305` |
| Test: run-tree shape | `FakeTracer` records full `Run` trees (`child_runs`, `parent_run_id`, `trace_id`, `dotted_order`); entrypoint test asserts root→entrypoint→2 concurrent `mapper` task children with exact recorded inputs | `libs/langgraph/tests/fake_tracer.py:46-85`, `libs/langgraph/tests/test_pregel_async.py:2288-2331` |
| Test: single root run w/ nested graphs+interrupts | Nested subgraph with interrupts produces "exactly 1 root run" | `libs/langgraph/tests/test_large_cases_async.py:3500-3510`, `libs/langgraph/tests/test_large_cases.py:5786-5795` |
| Test: LangSmith interop | `dotted_order` prefix ordering and shared `trace_id` across `parent_node` → `@traceable some_traceable` → `child_graph` runs | `libs/langgraph/tests/test_tracing_interops.py:109-118` |
| Test: trace policy | Recorded run inputs/outputs transformed while real data flows unchanged; processor exceptions fail open | `libs/langgraph/tests/test_trace_policy.py:25-49`, `libs/langgraph/tests/test_trace_policy.py:112-127` |

## Answers to Dimension Questions

1. **Is there a single coherent trace tree?**
   Yes, within a process. One invocation produces exactly one root run (`main.py:2792-2797`; asserted in `tests/test_large_cases_async.py:3510` "Should produce exactly 1 root run"). Parentage is enforced structurally: the loop hands the root `ParentRunManager` to task preparation (`_loop.py:622`), tasks receive `manager.get_child("graph:step:N")` callbacks (`_algo.py:716-720`), and nodes patch `run_manager.get_child()` into the config their function receives (`_runnable.py:430`). Coherence across tracer backends is maintained by `dotted_order`/`trace_id` conventions inherited from LangChain-core, verified against LangSmith in `tests/test_tracing_interops.py:113-118`. Caveat: that interop test is skipped in CI (`test_tracing_interops.py:60`).

2. **Are all execution steps represented?**
   Nearly all. Graph run, per-step node runs (including parallel `Send` fan-out — each packet becomes its own child run at `_algo.py:1060-1070`), functional-API `@task` runs (`_algo.py:893-933`), retries (each attempt re-invokes the traced runnable under the same parent, so failed attempts appear as errored sibling runs), and LLM/tool calls via standard `on_llm_*`/`on_tool_*` callbacks consumed by both tracers and stream handlers (`_messages.py:130-149`, `_tools.py:35-53`). Interrupts/resume are not spans but dedicated lifecycle events carrying run/checkpoint identity (`callbacks.py:42-76`). Checkpointing appears only as debug-stream events (`debug.py:144-206`), not tracer spans. **Guardrail evaluations have no representation — no guardrail concept or span type exists in this source** (searched `guardrail`, `guard`, safety-related symbols; only retry/timeout/error-handler machinery found). Evals are likewise absent here (delegated to LangSmith; see README.md:56).

3. **Do handoffs and subagent calls nest correctly?**
   Yes. LangGraph expresses handoffs/subagents either as subgraphs added as nodes (config inheritance makes the child graph's root run a descendant of the node run — exercised by the weather-router subgraph test `tests/test_large_cases_async.py:3531-3658` and the skipped interop test's `child_graph` nesting at `tests/test_tracing_interops.py:82-117`), or via `Command(goto=...)` which yields ordinary sibling node runs. Functional-API tasks nest under the calling run via `call.callbacks` (`_call.py:290-297`, asserted `tests/test_pregel_async.py:2324-2331`). Even non-LangChain user code nests: `set_config_context` installs LangSmith's tracing parent (`_runnable.py:96-101`) and snapshots contextvars onto asyncio tasks (`_runnable.py:143-153`). Edge case handled explicitly: a node that itself streams a subgraph with `stream_mode="messages"/"tools"` must not double-emit; `parent_ns` matching suppresses duplicates (`_messages.py:72-95`, `_tools.py:66-79`).

4. **Can you follow a request from start to finish?**
   In-process: yes — root-to-tool-result in one trace, with per-run metadata locating each span in the graph (`langgraph_node`/`step`/`checkpoint_ns`, `_algo.py:654-660`) and `TAG_HIDDEN` keeping framework noise out (`constants.py:26-27`). Across processes: partially. The Python SDK can direct server-side tracing to a chosen LangSmith project/dataset example (`schema.py:111-117`, `runs.py:338`) and clients correlate via returned `run_id` (`utilities.py:103-110`; remote adapter `_remote_run_stream.py:158`), but there is **no W3C Trace Context / B3 header propagation** in either SDK (no `traceparent` match in the whole source), so stitching a client-side trace to a server-side graph trace requires vendor plumbing or manual header injection.

## Architectural Decisions

- **Delegate spans to the host ecosystem instead of building a tracer.** LangGraph emits only generic LangChain callback events (`on_chain_start/end/error`, `on_llm_*`, `on_tool_*`) through managers obtained from `get_callback_manager_for_config(config)` (`main.py:2772`); any tracer (LangSmith, OTel instrumentation, custom) plugs in unchanged. Cost: no native OTel exporter, and correctness depends on callback discipline spread across the engine.
- **Make the config object the propagation vehicle.** `RunnableConfig["callbacks"]` is threaded through loop → task prep → node runnable → user function → subgraph; child managers are derived at each boundary (`get_child()`). This single mechanism simultaneously powers tracing, streaming (`messages`/`tools` handlers are just inheritable callback handlers appended to the root run's manager, `main.py:2821-2838`), timeout progress detection (`_retry.py:189-192`), and lifecycle events.
- **Correlate streamed fragments to tree positions via injected metadata, not span pointers.** Stream handlers map `run_id → (namespace, metadata)` using `langgraph_checkpoint_ns` parsed from run metadata (`_messages.py:141-149`, `_tools.py:112-118`), avoiding coupling between the streaming layer and tracer internals.
- **Treat the checkpoint namespace as the durable spine of the run tree.** `checkpoint_ns` tuples (`node_name:task_id`, NS_SEP-joined per nesting level) survive interrupts, resumes, and process boundaries, letting resumed executions reattach lifecycle events to the right subtree (`callbacks.py:55-76`, `debug.py:156-174`).
- **Redact at the span boundary, not in business logic.** `TracePolicy` transforms only what the node's *own* run records, explicitly scoped away from child runs and real data, failing open (`types.py:533-546`, `_runnable.py:76-85`).

## Notable Patterns

- **Manager-chain span factory**: three call sites fully determine the tree shape (root `_algo`-task-node), making the hierarchy auditable.
- **Contextvar bridging for foreign tracers**: `set_config_context(child_config, run)` publishes both the config and the tracer run object so context-based APIs (`@traceable`, `get_config()`) join the tree without explicit parameter passing (`_runnable.py:88-153`).
- **Callbacks-as-bus**: the same event stream feeds tracers, stream projections, idle-timeout heartbeats, and lifecycle dispatchers — one emission point, many observers (`_retry.py:274-305`, `callbacks.py:264-278`).
- **Deterministic test oracle**: `FakeTracer` rewrites UUIDs deterministically while preserving `dotted_order` structure, enabling golden-tree assertions (`fake_tracer.py:46-57`).
- **Opt-out tagging**: `TAG_NOSTREAM` ("nostream") and `TAG_HIDDEN` ("langsmith:hidden") let nodes exclude themselves from token streaming vs. entire trace views (`constants.py:24-27`).

## Tradeoffs

- **Ecosystem leverage vs. first-class standards**: zero-cost integration with LangSmith, but no W3C-compliant distributed context; multi-vendor deployments must rely on third-party monkey-patching (compat-only guarantee, `test_graph_callbacks.py:281-295`).
- **Config-threading purity vs. escape hatches**: any user code that drops the `config` argument (or spawns threads/tasks without the provided helpers) silently detaches its subtree from the trace; LangGraph mitigates with contextvars and `create_task_in_config_context` (`_runnable.py:143-153`) but cannot prevent all leaks.
- **Metadata-rich spans vs. payload exposure**: recording full node inputs/outputs aids debugging but risks sensitive-data leakage; `TracePolicy` addresses this per-node yet explicitly punts holistic redaction to the LangSmith client (`types.py:540-542`).
- **Debug stream as poor-man's viewer**: `stream_mode="debug"` gives a textual run/checkpoint timeline without any backend (`debug.py:41-206`), but it is fire-and-forget — no persistence or queryability.

## Failure Modes / Edge Cases

- **Flaky canonical test**: the end-to-end LangSmith nesting test is disabled due to CI timeouts (`test_tracing_interops.py:60`), so dotted-order continuity across sync/async boundaries is currently unenforced in CI.
- **Handler filtering surprises**: v1 `messages` streaming strips inherited `StreamMessagesHandlerV2` handlers to avoid protocol bleed (`main.py:2773-2789`) — custom handler stacks interleaving v1/v2 modes can lose events if they relied on the removed handler.
- **Interrupts are not retried and are specially routed in the tree**: PUSH tasks with pending calls skip emitting their own interrupt writes so the parent owns them (`_loop.py:1424-1429`); a mis-read of the trace could attribute the interrupt to the wrong subtree without checking `checkpoint_ns`.
- **Duplicate-message hazard across resumes**: dedupe relies on message ids collected at `on_chain_start`; a past bug shows Pydantic-state subgraphs could duplicate streamed messages after interrupts (`tests/test_pregel.py:7331-7415`, regression fixed).
- **Fail-open redaction**: if a `TracePolicy` processor raises, the raw payload is still recorded (`_runnable.py:80-85`) — availability is prioritized over confidentiality, which may surprise security-focused users.
- **Cross-type callback manager reconstruction**: converting between sync/async graph managers copies only `GraphCallbackHandler`s (`callbacks.py:164-175`), so mixing arbitrary handlers through that path silently drops them.

## Future Considerations

- Adopt W3C Trace Context (`traceparent`) injection/extraction in `sdk-py`/`sdk-js` transports so local and server traces stitch without vendor-specific `langsmith_tracer` routing.
- Promote OTel from "don't crash when monkey-patched" (`test_graph_callbacks.py:281-295`) to a supported exporter mapping graph/node/task/model/tool runs onto standard semantic-convention spans.
- Un-skip and stabilize `test_nested_tracing` (`test_tracing_interops.py:60`) or replace it with a hermetic equivalent asserting `dotted_order` continuity.
- Consider representing interrupts/resumes and checkpoint transitions as tracer-visible events (today they live only in `GraphCallbackHandler` and the `debug` stream), closing the gap between the run tree and the durability timeline.

## Questions / Gaps

- **Guardrails**: no evidence found. Searched for guardrail/guard/safety span types, validation-node conventions, and callback variants across `libs/langgraph`; nothing beyond retry/timeout/error-handler nodes exists. Guardrails-as-spans would be user-modeled (e.g., plain nodes), not framework-provided.
- **Evals**: no eval harness in this source; only references to external LangSmith evals (`README.md:56`, `docs/redirects.json:121-122`).
- **Retries in traces**: each attempt re-runs the traced runnable, implying one errored child run per failed attempt, but no test was found that asserts the exact multi-attempt run-tree shape (only that interrupts aren't retried: `tests/test_pregel_async.py:638`).
- **JS core parity**: `libs/sdk-js` is a REST/WebSocket client only; no LangGraph.js runtime sources are present in this snapshot, so JS-side run-tree behavior could not be assessed (no `langsmith`/tracing matches in `sdk-js`).

---

Generated by `Dimension 10.01: Span Hierarchy and Run Tree` against `langgraph`.
