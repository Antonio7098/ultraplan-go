# Source Analysis: pydantic-ai

## Termination and Loop Bounds

### Source Info

| Field | Value |
|-------|-------|
| Name | pydantic-ai |
| Path | `studies/agent-harness-study/sources/pydantic-ai` |
| Language / Stack | Python (pydantic_ai_slim, pydantic_graph, pydantic_evals, clai) |
| Analyzed | 2026-07-02 |

## Summary

Pydantic AI's agent loop is a `pydantic_graph` `GraphRun` (pydantic_graph/pydantic_graph/graph.py:664) that drives `UserPromptNode → ModelRequestNode → CallToolsNode → (ModelRequestNode | SetFinalResult)` as defined by `build_agent_graph` at `pydantic_ai_slim/pydantic_ai/_agent_graph.py:2110-2139`. The driver at `pydantic_ai_slim/pydantic_ai/agent/abstract.py:438-449` is a `while not isinstance(node, End)` loop with **no native max-steps / max-iterations / recursion-limit counter** — the loop terminates only when a node returns `End`, an exception propagates, or a third-party limit (UsageLimits, tool/output retry budget) fires.

There are four independently enforced termination regimes:

1. **Usage limits** (`UsageLimits` at `pydantic_ai_slim/pydantic_ai/usage.py:264-421`): `request_limit` (default 50), `tool_calls_limit`, `input_tokens_limit`, `output_tokens_limit`, `total_tokens_limit`, `count_tokens_before_request`. Enforced pre-request in `_prepare_request` (`_agent_graph.py:960`) and post-response via `check_tokens` (`_agent_graph.py:1024`); raise `UsageLimitExceeded` (`exceptions.py:195`).
2. **Output retry budget** (the "output" half of `AgentRetries`): per-run global counter on `GraphAgentState.output_retries_used` (`_agent_graph.py:127`, incremented at `_agent_graph.py:175-179` via `consume_output_retry`). On exhaust, raises `UnexpectedModelBehavior('Exceeded maximum output retries ({N})')`. Default `retries={'output': 1}` (`agent/__init__.py:368-370`).
3. **Per-tool retry budget** (`ToolsetTool.max_retries`, `tool_manager.py:177-181` via `_check_max_retries`): per-tool `retries[name]` counters stored on the `ToolManager.ctx.retries` dict (`_run_context.py:60-61`). Exceeded → `UnexpectedModelBehavior('Tool {name!r} exceeded max retries count of {N}')`.
4. **Model side finish-reason hard stops**: `finish_reason == 'length'` on an empty / thinking-only response raises `UnexpectedModelBehavior('Model token limit (...) exceeded before any response was generated.')` (`_agent_graph.py:1107-1114`); `finish_reason == 'content_filter'` on an empty response raises `ContentFilterError` (`_agent_graph.py:1117-1130`).

Terminology and config keys: limits live on `UsageLimits`, retry budgets on `AgentRetries` (TypedDict with `tools` and `output` keys, `agent/abstract.py:106-129`) with configurable `default_max_retries=1` on `ToolManager` (`tool_manager.py:92`), and per-run overrides via `retries={...}` passed to `agent.run` / `agent.iter` / `agent.override`. The output budget is the only one overridable per-run (`abstract.py:359-363`). Each budget is **independent** — there is no global "steps" counter and no guard that sums them.

There is **no stuck-loop detection** (no hash-of-last-response, no repetition counter, no cosine-similarity check on consecutive model responses) anywhere in the agent loop. The framework's only protection against a runaway model/tool cycle is the user setting `UsageLimits(request_limit=...)` to cap requests — the canonical loop-bound mechanism. Tests at `tests/test_agent.py:3904-3944` show that without `request_limit` a looping tool (`Tool 'foobar'` raises ModelRetry forever) runs until `request_limit` fires; without that limit the run would continue indefinitely.

The `finish_reason` enum is a stable public type alias: `Literal['stop', 'length', 'content_filter', 'tool_call', 'error']` (`messages.py:114-120`). Each provider normalizes its native stop reason into this vocabulary (`models/openai.py:4320`, `models/anthropic.py:956`, `models/google.py:161-176`, etc.). Finish reason is *persisted on `ModelResponse.finish_reason`* (a public field, `messages.py:2143`) but is **not directly consumed by the loop as a stop signal** — only the `length` and `content_filter` paths in `_agent_graph.py:1107-1130` are interpreted; `stop` and `tool_call` proceed normally.

Final state is always accessible: `AgentRun.result` is populated when the graph reaches `End` (`run.py:135-145`), `AgentRunResult.output` and `.all_messages()` / `.new_messages()` / `.usage` / `.response` are populated (`run.py:478-619`), and exceptions propagate with full message + (where applicable) body (`exceptions.py:191-229`). There is no separate "termination reason" field on the result — exhaustion is communicated only by which exception was raised. The run is drained even on stream cancellation (cooperative handoff in `ModelRequestNode.stream`, `_agent_graph.py:606-757`).

## Rating

**7 / 10** — Clear model with explicit interfaces, well-typed `UsageLimits` + `AgentRetries` + per-tool `max_retries`, exhaustive testing in `tests/test_usage_limits.py`, `tests/test_agent.py`, `tests/v2/test_agent_retries_deprecation.py`. Each budget is independent and overridable at the right level (agent / run / per-tool / per-toolset). Termination reason is communicated by exception type. Limitations: no global step counter, no "exhausted because of X" result field (callers must catch `UsageLimitExceeded` vs `UnexpectedModelBehavior` vs `ContentFilterError` to distinguish), no stuck-loop detection short of the user setting a hard limit, the tool retry counter and the output retry counter interact in ways that are documented but easy to misconfigure (`docs/agent.md:1095-1096`), and `request_limit`'s default of 50 is generous enough that a model in a tool-retry loop can still consume substantial cost before tripping it.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Graph definition (no native step cap) | `build_agent_graph` returns the agent-loop Graph with edges start → UserPromptNode → ModelRequestNode ⇄ CallToolsNode → SetFinalResult → End | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:2110-2139` |
| Graph run-level driver | `pydantic_graph.graph.GraphRun.next()` invokes `node.run(ctx)`; runner snapshot persists; no recursion / step counter | `pydantic_graph/pydantic_graph/graph.py:664-753` |
| Agent.run iteration driver | `while not isinstance(node, End): node = await agent_run.next(node)` — the only "loop" in agent.run; no max-iterations | `pydantic_ai_slim/pydantic_ai/agent/abstract.py:438-449` |
| Agent.iter / AgentRun async-iter | `__anext__` returns the next node or raises `StopAsyncIteration` on `End`; no step limit | `pydantic_ai_slim/pydantic_ai/run.py:192-233` |
| Agent.run_stream driver | Same `while not yielded` structure, with capability-hook awareness | `pydantic_ai_slim/pydantic_ai/agent/abstract.py:784-911` |
| Default request limit | `request_limit: int \| None = 50` is the *only* default on-cap; everything else is `None` | `pydantic_ai_slim/pydantic_ai/usage.py:273` |
| UsageLimits class | `request_limit`, `tool_calls_limit`, `input_tokens_limit`, `output_tokens_limit`, `total_tokens_limit`, `count_tokens_before_request`; deprecation alias `request_tokens_limit`/`response_tokens_limit` | `pydantic_ai_slim/pydantic_ai/usage.py:264-365` |
| Pre-request usage check | `UsageLimits.check_before_request` raises `UsageLimitExceeded("The next request would exceed the request_limit of {N}")` | `pydantic_ai_slim/pydantic_ai/usage.py:379-395` |
| Post-response token check | `UsageLimits.check_tokens` raises with messages `"Exceeded the input_tokens_limit of ..."` / `output_tokens_limit` / `total_tokens_limit` | `pydantic_ai_slim/pydantic_ai/usage.py:397-411` |
| Pre-tool-call projection check | `UsageLimits.check_before_tool_call` raises `"The next tool call(s) would exceed the tool_calls_limit of {N}"`; uses `projected_usage` so parallel tool calls are validated *before* execution | `pydantic_ai_slim/pydantic_ai/usage.py:413-420` |
| `usage_limits.check_before_request` call site (counted before request) | `ctx.deps.usage_limits.check_before_request(usage)` in `_prepare_request`; optional `count_tokens_before_request` triggers `model.count_tokens` ahead of time | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:952-960` |
| `usage_limits.check_tokens` call site (counted after response) | Inside `ModelRequestNode._append_response`: `ctx.state.usage.incr(response.usage)` then `ctx.deps.usage_limits.check_tokens(ctx.state.usage)` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1015-1025` |
| Usage-limit exception type | `class UsageLimitExceeded(AgentRunError)` defined in exceptions module | `pydantic_ai_slim/pydantic_ai/exceptions.py:195-197` |
| Token `UnexpectedModelBehavior` (`IncompleteToolCall`) | Raised when `finish_reason == 'length'` AND last part is a `ToolCallPart` with invalid args | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:146-160` |
| AgentRetries TypedDict | `tools: int` + `output: int`; explicit docstring on int semantics per call-site | `pydantic_ai_slim/pydantic_ai/agent/abstract.py:106-129` |
| Normalization helper | `_normalize_agent_retries(retries, *, default: int = 1)` → `_ResolvedAgentRetries(tools, output)`; missing keys fall back to `default` | `pydantic_ai_slim/pydantic_ai/agent/__init__.py:142-148` |
| Default `AgentRetries` (both default to 1) | `Agent(...)` constructor `retries: int \| AgentRetries \| None = None`; default both `1` for tools and output | `pydantic_ai_slim/pydantic_ai/agent/__init__.py:285`, `pydantic_ai_slim/pydantic_ai/agent/__init__.py:368-370` |
| Agent stores resolved retry budget | `self._max_output_retries = resolved_retries.output`; `self._max_tool_retries = ...` (inferred from `self._output_toolset.max_retries = self._max_output_retries`) | `pydantic_ai_slim/pydantic_ai/agent/__init__.py:507-518` |
| Per-run output retry override | `effective_output_retries` resolved from spec + override; `agent.iter` sets `output_retries_used=0` | `pydantic_ai_slim/pydantic_ai/agent/__init__.py:1190-1281` |
| Output retry counter on state | `output_retries_used: int = 0` field on `GraphAgentState` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:127` |
| `consume_output_retry` (text-path exhaustion) | Increments `output_retries_used`, then `if output_retries_used > max_output_retries: check_incomplete_tool_call(); raise UnexpectedModelBehavior(f'Exceeded maximum output retries ({max_output_retries})')` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:162-179` |
| Output-exhaustion call sites (text path) | `CallToolsNode._run_stream` calls `ctx.state.consume_output_retry(ctx.deps.max_output_retries, ...)` for empty-response / thinking-only / no-actionable-output fall-throughs | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1146`, `_agent_graph.py:1171`, `_agent_graph.py:1248` |
| Output-exhaustion call sites (tool path) | `process_tool_calls` raises `UnexpectedModelBehavior(f'Exceeded maximum output retries ({max_retries})')` (for validation or execution failure on the first non-final-result output tool) | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1600-1603`, `_agent_graph.py:1641-1646` |
| Output-retry ModelRetry path (e.g. `_build_retry_node`) | Per-tool `ModelRetry` from `after_model_request` / `wrap_model_request` retried via `consume_output_retry` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1027-1040`, `_agent_graph.py:829-835` |
| Per-tool retry budget on `ToolsetTool` | `ToolsetTool.max_retries: int` carries the resolved per-tool max; precedence is per-tool > per-toolset > agent | `pydantic_ai_slim/pydantic_ai/toolsets/abstract.py:42-63` |
| `ToolManager.default_max_retries` | `default_max_retries: int = 1` on `ToolManager`; carried across `for_run_step` | `pydantic_ai_slim/pydantic_ai/tool_manager.py:90-145` |
| `ToolManager._check_max_retries` | `_check_max_retries(self, name, max_retries, error)`: `if self.ctx.retries.get(name, 0) == max_retries: raise UnexpectedModelBehavior(f'Tool {name!r} exceeded max retries count of {max_retries}')` | `pydantic_ai_slim/pydantic_ai/tool_manager.py:177-181` |
| Per-tool retry counter persistence | `RunContext.retries: dict[str, int]` keyed by tool name; `for_run_step` carries failed-tool counters across steps | `pydantic_ai_slim/pydantic_ai/_run_context.py:60-61`, `pydantic_ai_slim/pydantic_ai/tool_manager.py:120-145` |
| `RunContext.last_attempt` | `last_attempt` property: `return self.retry == self.max_retries`; exposed to tools so they can pre-empt | `pydantic_ai_slim/pydantic_ai/_run_context.py:153-156` |
| `FunctionToolset.max_retries` precedence | `FunctionToolset.call_tool` resolves `timeout` and `max_retries` from tool → toolset → ctx | `pydantic_ai_slim/pydantic_ai/toolsets/function.py:612-635` |
| Function-toolset default `max_retries=1` | `FunctionToolset(max_retries=1)` constructor default; `test_fastmcp.py:181` shows `max_retries=5` override | `pydantic_ai_slim/pydantic_ai/toolsets/function.py:65-117` |
| MCP / external / deferred-canonical default `max_retries=1` | Same default in `mcp.py:1477`, `mcp.py:2129`, `toolsets/external.py:38` | `pydantic_ai_slim/pydantic_ai/mcp.py:1477,2129`, `pydantic_ai_slim/pydantic_ai/toolsets/external.py:38` |
| Tool-execution timeout | `tool_timeout: float \| None = None` on `Agent`; per-tool `timeout` on `Tool(...)`; resolved as `anyio.fail_after(timeout)` then `raise ModelRetry(f'Timed out after {timeout} seconds.')` (counts toward retry budget) | `pydantic_ai_slim/pydantic_ai/toolsets/function.py:644-658`, `pydantic_ai_slim/pydantic_ai/agent/__init__.py:256-523` |
| Finish-reason vocabulary | `FinishReason: TypeAlias = Literal['stop', 'length', 'content_filter', 'tool_call', 'error']`; public type | `pydantic_ai_slim/pydantic_ai/messages.py:114-120` |
| `finish_reason` stored on `ModelResponse` | Public field carrying provider-normalized reason; emitted as OTel attribute `gen_ai.response.finish_reasons` | `pydantic_ai_slim/pydantic_ai/messages.py:2143-2144`, `pydantic_ai_slim/pydantic_ai/_instrumentation.py:283-284` |
| `length` finish reason → immediate raise | `_run_stream` checks `finish_reason == 'length'` before any other logic and raises `UnexpectedModelBehavior('Model token limit (...) exceeded before any response was generated.')` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1107-1114` |
| `content_filter` empty response → `ContentFilterError` | `if is_empty and self.model_response.finish_reason == 'content_filter': ... raise ContentFilterError(...)` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1117-1130` |
| `ContentFilterError` exception type | `class ContentFilterError(UnexpectedModelBehavior)` carries `body` JSON | `pydantic_ai_slim/pydantic_ai/exceptions.py:232-234` |
| `finish_reason` provider mapping | Stable OTel-aligned mapping per provider; e.g. `GoogleFinishReason.SAFETY → 'content_filter'`, `STOP → 'stop'`, `MAX_TOKENS → 'length'`; `RECITATION/LANGUAGE/...` collapse to `'content_filter'`/`'error'` | `pydantic_ai_slim/pydantic_ai/models/google.py:161-176` |
| `ModelResponseState` lifecycle | `'complete'`, `'incomplete'`, `'interrupted'` — separate from `finish_reason`. `'interrupted'` is preserved through interruption; recovery is caller's responsibility | `pydantic_ai_slim/pydantic_ai/messages.py:123`, `docs/output.md:1011` |
| Stream-cancel cooperative handoff | `ModelRequestNode.stream` uses `ready_waiter` / `stream_done` events so `cancel_and_drain` runs both halves of the wrap_model_request task | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:638-757` |
| `AgentRun.result` populated when `End` reached | `if not isinstance(self._next_node, End): return None` so `None` until finished | `pydantic_ai_slim/pydantic_ai/run.py:140-145` |
| Final result accessible after success | `AgentRunResult.output`, `.all_messages()`, `.new_messages()`, `.response`, `.usage` (property) — all public | `pydantic_ai_slim/pydantic_ai/run.py:478-619` |
| User-facing failure path on stub nodes | `raise exceptions.AgentRunError('the stream should set `self._next_node` before it ends')` last-resort guard | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1079` |
| Undrained pending-messages error | `UndrainedPendingMessagesError(UserError)` raised at `End` if `'when_idle'` messages or end-of-run redirects were not drained (only `agent.run()` drains them) | `pydantic_ai_slim/pydantic_ai/exceptions.py:170-178` |
| IncompleteToolCall exception type | `class IncompleteToolCall(UnexpectedModelBehavior)` raised from `check_incomplete_tool_call()` when `length` finish halves a tool call | `pydantic_ai_slim/pydantic_ai/exceptions.py:308-309` |
| Test: request_limit stops loop | `test_retry_limit` (TestModel + `request_limit=1`) → `UsageLimitExceeded('The next request would exceed the request_limit of 1')` | `tests/test_usage_limits.py:68-80` |
| Test: total_tokens_limit | `test_total_token_limit` → `UsageLimitExceeded('Exceeded the total_tokens_limit of 50 (total_tokens=55)')` | `tests/test_usage_limits.py:61-65` |
| Test: per-tool max retries | `test_unknown_tool` → `UnexpectedModelBehavior("Tool 'foobar' exceeded max retries count of 1")` after 1 retry of an unknown tool | `tests/test_agent.py:3897-3944` |
| Test: `length` finish on empty response | `test_empty_response_with_finish_reason_length` → `UnexpectedModelBehavior('Model token limit (10) exceeded before any response was generated.')` | `tests/test_agent.py:4123-4142` |
| Test: `length` finish on thinking-only response | `test_thinking_only_response_with_finish_reason_length` — same exception | `tests/test_agent.py:4144-4162` |
| Test: parallel tool calls exceeding limit | `test_parallel_tool_calls_must_not_exceed_limit` confirms `tool_calls_limit` is checked before any tool executes | `tests/test_usage_limits.py:865-920` |
| Test: streaming output-token mid-stream limit | `test_stream_text_enforces_output_token_limit_mid_stream` regression test | `tests/test_usage_limits.py:154-172` |
| Doc: canonical "infinite loop" example | `docs/agent.md:617-656` shows `infinite_retry_tool` raising `ModelRetry` forever, bounded by `usage_limits=UsageLimits(request_limit=3)` — *not* by `retries=` alone | `docs/agent.md:617-656` |
| Doc: output retry enforcement split | Two paths: text (single global budget per run) and tool output (default per-tool budget via `AgentRetries(output=...)`, overridable via `ToolOutput(max_retries=...)`) | `docs/agent.md:1088-1100`, `docs/agent.md:1091-1096` |
| Doc: tool retries per-tool counter | `Tool retries are tracked **per tool**: every function tool has its own counter, with no global 'tool call' budget shared across the run` — explicit warning | `docs/tools-advanced.md:492` |
| Doc: `request_limit` is the loop bound | "`request_limit`: bound the number of model turns; `tool_calls_limit`: cap successful tool executions" | `docs/agent.md:681-683` |
| Doc: `retries` not overridable per run for tools | Only the output budget can be overridden per run; `retries={'tools': ...}` at run sites raises `UserError` | `pydantic_ai_slim/pydantic_ai/agent/abstract.py:117-119` |
| Output budget exposed to output-validators | `ctx.last_attempt = (self.retry == self.max_retries)` lets validators detect "this is the last try" | `pydantic_ai_slim/pydantic_ai/_run_context.py:153-156` |
| No global "run step" cap exists | `run_step` is *only* a per-step counter, used to refresh tool discovery / loaded capabilities — not a termination condition | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:128`, `_agent_graph.py:856`, `_agent_graph.py:328` |
| `final_result` reachable via `agent_run.next_node` | `end_strategy = Literal['early', 'graceful', 'exhaustive']` controls how leftover tool calls are handled when a final result is found — *not* a loop bound | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:73`, `_agent_graph.py:196`, `_agent_graph.py:1566-1676` |
| Test: `end_strategy` 'exhaustive' collects all output tools | `test_exhaustive_strategy_*` (and equivalent streaming tests) verify all tools run when `final_result` is reached | `tests/test_agent.py:5261-5930`, `tests/test_streaming.py:2292-2730` |
| Test: tool-timeout ModelRetry path | `test_*` confirms timeout raises ModelRetry, *not* retried silently | `tests/test_tools.py:1490`, `tests/test_toolsets.py:801-1027` |
| Test: retries deprecation under v2 | `tests/v2/test_agent_retries_deprecation.py` verifies `tool_retries=` / `output_retries=` warn and route through canonical `retries={...}` | `tests/v2/test_agent_retries_deprecation.py:1-237` |

## Answers to Dimension Questions

### 1. What stops the loop?

The driver's `while not isinstance(node, End)` (in `agent/abstract.py:439`) ends only when:

- **A node returns `End[FinalResult]`** — normal success path. Produced by `CallToolsNode._handle_final_result` at `_agent_graph.py:1379` (output tool validated) or by `SetFinalResult.run` at `_agent_graph.py:1393` (streaming short-circuit).
- **`UsageLimitExceeded`** (`exceptions.py:195`) raised by `UsageLimits.check_before_request` (`usage.py:379-395`) before `ModelRequestNode` even issues its request — typically a `request_limit` (`"The next request would exceed the request_limit of {N}"`) or `input_tokens_limit` / `total_tokens_limit` pre-count.
- **`UsageLimitExceeded`** raised by `UsageLimits.check_tokens` (`usage.py:397-411`) inside `ModelRequestNode._append_response` (`_agent_graph.py:1015-1025`) right after `ctx.state.usage.incr(response.usage)` for `input_tokens_limit`, `output_tokens_limit`, `total_tokens_limit`.
- **`UsageLimitExceeded`** raised by `UsageLimits.check_before_tool_call` (`usage.py:413-420`) in `process_tool_calls` (`_agent_graph.py:1705-1708`) when a parallel-tool-call batch would exceed `tool_calls_limit`.
- **`UnexpectedModelBehavior`** (`exceptions.py:203`) — multiple causes:
  - `'_build_retry_node'` exhausted the **output retry budget** (`_agent_graph.py:1027-1040`, message `"Exceeded maximum output retries ({N})"` from `consume_output_retry` at `_agent_graph.py:178`).
  - `CallToolsNode._run_stream` exhausted the **output retry budget** for empty / thinking-only / no-actionable-output responses (`_agent_graph.py:1146-1248`).
  - `process_tool_calls` exhausted the **output retry budget** on the tool path (validation or execution failure on the first non-final output tool: `_agent_graph.py:1600-1603`, `_agent_graph.py:1641-1646`).
  - `ToolManager._check_max_retries` exhausted a **per-tool retry budget** (`tool_manager.py:177-181`, message `"Tool {name!r} exceeded max retries count of {N}"`).
  - `_run_stream` immediate raise on `finish_reason == 'length'` with empty / thinking-only response (`_agent_graph.py:1107-1114`, message `"Model token limit ({N or 'provider default'}) exceeded before any response was generated."`).
  - `check_incomplete_tool_call` (`_agent_graph.py:146-160`) raised `IncompleteToolCall` when `'length'` happened mid-`ToolCallPart`.
- **`ContentFilterError`** (`exceptions.py:232-234`, a subclass of `UnexpectedModelBehavior`) raised when `finish_reason == 'content_filter'` AND response has no actionable parts (`_agent_graph.py:1117-1130`).
- **`ModelRetry`** from a capability hook (`after_model_request`, `after_tool_execute`, output validator) — caught, the `RetryPromptPart` is appended to history, and the run continues with a fresh `ModelRequest`. Does **not** terminate by itself.
- **`asyncio.CancelledError`** / task cancellation — the graph runner propagates `cancel_and_drain` (`_agent_graph.py:244-687`); `ModelResponseState` becomes `'interrupted'` (`messages.py:123`).

### 2. Are limits configurable?

Yes — at every meaningful granularity:

- **`UsageLimits`** fields are explicitly keyword-only and typed: `request_limit`, `tool_calls_limit`, `input_tokens_limit`, `output_tokens_limit`, `total_tokens_limit`, `count_tokens_before_request` (`usage.py:264-365`). Deprecated aliases `request_tokens_limit` / `response_tokens_limit` are still accepted (`usage.py:298-345`). Pass via `usage_limits=UsageLimits(...)` on `agent.run` / `run_sync` / `run_stream` / `iter`.
- **`AgentRetries`** TypedDict (`agent/abstract.py:106-129`): independent budgets for `tools` (per-tool default) and `output` (global per run, text path; per-tool default, output-tool path). Set via `Agent(retries=...)` (`agent/__init__.py:285`).
- **`Tool(max_retries=...)`** / `@agent.tool(retries=...)` (`tools.py:472`) — per-tool override.
- **`FunctionToolset(max_retries=...)`** (`toolsets/function.py:65-117`) — per-toolset default.
- **`ToolOutput(max_retries=...)`** (`output.py:116-137`) — per-output-tool override for the structured-output path.
- **`FastMCPToolset(max_retries=...)`** / `MCPClientToolset(max_retries=...)` (`mcp.py:1477, 2129`) — defaults to `1`.
- **`Agent(tool_timeout=...)`** (`agent/__init__.py:292`) — tool-execution wall-clock; on timeout the tool raises `ModelRetry('Timed out after {t} seconds.')` which then counts toward the retry budget.
- **Per-run `output` override**: `retries={'output': N}` on `agent.run` / `run_sync` / `run_stream` / `iter` / `override`. Passing `retries={'tools': N}` at run sites raises `UserError` (`agent/abstract.py:117-119`).
- **Per-run `UsageLimits`**: the `usage_limits=UsageLimits(...)` kwarg is independently specifiable per call.
- **HTTP request retries** (`retries.md`) are a separate layer: `RetryConfig` / `TenacityTransport` for transient HTTP failures. Distinct from the agent-loop `retries`.

There is no global on/off switch to disable all limits — defaults are `request_limit=50`, `retries={'output': 1, 'tools': 1}`, `tool_timeout=None`.

### 3. Is exhaustion treated differently from success?

**Mostly yes, but only via distinct exception types** — the result object does not carry a `termination_reason` field.

- **Success (final result)**: `End(final_result)` reached. `agent_run.result` (`run.py:140`) becomes non-`None`. The user receives an `AgentRunResult` with `output`, `usage`, `all_messages()`, `response`, etc. (`run.py:478-619`).
- **`UsageLimits` exhausted**: `UsageLimitExceeded` (`exceptions.py:195`) subclass of `AgentRunError`. Distinct message per limit kind: `"The next request would exceed the request_limit of {N}"`, `"Exceeded the input_tokens_limit/output_tokens_limit/total_tokens_limit of {N} (...)"`, `"The next tool call(s) would exceed the tool_calls_limit of {N}"`. The full `body` from a failed model response is preserved on `UnexpectedModelBehavior` (`exceptions.py:225-229`).
- **Output retry budget exhausted**: `UnexpectedModelBehavior` (not a subclass) with message `"Exceeded maximum output retries ({N})"` (`_agent_graph.py:178`).
- **Per-tool retry budget exhausted**: `UnexpectedModelBehavior` with message `"Tool {name!r} exceeded max retries count of {N}"` (`tool_manager.py:181`).
- **`finish_reason == 'length'`**: `UnexpectedModelBehavior` with `"Model token limit ({last_max_tokens or 'provider default'}) exceeded before any response was generated."` (`_agent_graph.py:1113`). When the truncation landed inside a `ToolCallPart`, an `IncompleteToolCall(UnexpectedModelBehavior)` with the verbatim token-limit advice (`_agent_graph.py:158-159`) is raised instead — it carries the same diagnostic info and a different message pointer (`exceptions.py:308-309`).
- **`finish_reason == 'content_filter'` on empty response**: `ContentFilterError(UnexpectedModelBehavior)` with `body` set to the JSON of the response and `message` reads `"Content filter triggered. {Finish reason | Block reason | Refusal}: ..."` (`_agent_graph.py:1117-1130`).
- **Model API / HTTP failure**: `ModelAPIError` / `ModelHTTPError` (`exceptions.py:236-265`) carry `model_name`, `status_code`, `body`.
- **Cancel**: no exception to the user; `ModelResponse.state == 'interrupted'` (`messages.py:123`). Documented recovery note: `docs/output.md:1011` — "Pydantic AI does not clean up incomplete tool calls in interrupted responses. Passing interrupted history directly into another run can therefore fail or lead to retries".
- **Last-resort invariants**: `AgentRunError('Agent run finished without producing a final result')` (`agent/abstract.py:914`); `AgentRunError('the stream should set `self._next_node` before it ends')` (`_agent_graph.py:1079`).
- **Drained-queue violation**: `UndrainedPendingMessagesError(UserError)` raised at `End` when `'when_idle'` / end-of-run redirects were stranded because the caller used bare `async for node in agent_run` instead of `agent.run()` (`exceptions.py:170-178`).

The framework never overwrites a successful `output` with an exhaustion error — exhaustion always raises and the user's try/except catches it. The user's `AgentRunResult` is null/replaced by the exception.

### 4. Are stuck loops detected before the hard limit?

**No.** Pydantic AI performs **no stuck-loop detection**:

- No `hash(prev_response) == hash(curr_response)` check, no repetition counter, no semantic-similarity guard.
- The only protection is the user's `UsageLimits(request_limit=...)` cap, plus the per-tool `max_retries` and the output-retry budget on the model side. The docstring at `docs/agent.md:681-683` makes this explicit:

  > "Usage limits are especially relevant if you've registered many tools. Use `request_limit` to bound the number of model turns, and `tool_calls_limit` to cap the number of successful tool executions within a run."

- The tool-execution `timeout` (`agent/__init__.py:292`, `toolsets/function.py:644-658`) is a wall-clock cap on a single tool call, not a stuck-loop detector — on timeout the tool raises `ModelRetry` ("Timed out after {t} seconds"), which then counts against `max_retries`.
- `count_tokens_before_request` (`usage.py:283-296`, `_agent_graph.py:953-958`) protects against an upcoming over-token request but does not detect repetition.

A pathological pattern (model repeatedly emits the same tool call with the same args, `infinite_retry_tool` example at `docs/agent.md:617-656`) is caught *only* because the user set `usage_limits=UsageLimits(request_limit=3)`. Tests at `tests/test_agent.py:3904-3944` confirm this is the documented escape hatch.

### 5. Does the user get a useful final state?

Yes, in both success and exhaustion paths:

- **On success**: `AgentRunResult.output` carries the typed output (or `str` by default). `AgentRunResult.all_messages()` / `.new_messages()` / `.response` give the full message history (optionally filtered). `AgentRunResult.usage` gives aggregated token usage, request count, and tool-call count. `_traceparent` (when instrumentation is on) preserves the OTel trace id. `AgentRunResultEvent` (`run.py:626-634`) surfaces the result on `run_stream_events`.
- **On exhaustion**: the exception type identifies the limit (`UsageLimitExceeded` vs `UnexpectedModelBehavior` vs `ContentFilterError` vs `ModelAPIError` vs `IncompleteToolCall`); the message embeds the literal limit (e.g. `"The next request would exceed the request_limit of 3"`); `UnexpectedModelBehavior.body` carries the offending response body when available (`exceptions.py:225-229`).
- **On interruption**: `ModelResponse.state == 'interrupted'` (`messages.py:123`) lets the caller distinguish a clean `stop`/`tool_call` finish from a cancelled stream. The `usage` object is still partially populated.
- **`AgentRun.result` before `End`**: `run.py:140` returns `None` until the graph finishes, so the user never reads a stale or partial result by accident.
- **`AgentRun.usage`** (live cumulative) updates on every node.

There is **no high-level "I gave up because X" enum field on the success result** — the caller must catch specific exceptions to differentiate. The `RunUsage` (`usage.py:181-234`) object the user reads back does include `requests` and `tool_calls` totals, which lets a user-side observer detect that a hard cap was hit by comparing against a `UsageLimits.request_limit` they passed — but the framework does not surface this explicitly.

## Architectural Decisions

- **Graph-based execution** (`pydantic_graph.Graph` + `BaseNode` + `End`): the agent loop is expressed as a small graph (4 nodes, deterministic edges). Driver is a generic `GraphRun.next` (`pydantic_graph/graph.py:664-753`); the agent-specific logic is in node `run` methods. This keeps the framework-level "no max steps" property delegated to the framework graph: termination is a node returning `End`.
- **Per-step, per-tool retry counters** (not a global "steps" counter): each tool has its own `retries[name]` counter stored on `RunContext` (`_run_context.py:60-61`). Carried across `for_run_step` (`tool_manager.py:120-145`). This means the framework cannot say "the run is stuck" generically — only "this specific tool has failed N times". A model that never finishes (and never triggers a tool retry) only stops via `request_limit` / tokens.
- **Two independent budgets**: `request_limit` (requests) vs `retries={'output': N}` (validation). They overlap on behavior (both produce more model requests when exhausted) but neither alone is sufficient to bound the worst-case cost — a user must set both.
- **Output retry split into text vs tool paths** (`docs/agent.md:1091-1096`, `docs/output.md:327`): text path uses a global per-run counter; tool-output path uses a per-tool counter with `ToolOutput(max_retries=...)` per-tool override. The split is consistent and documented.
- **`usage_limits` lives outside the agent**, on the run, not the agent — a clean separation between "what an agent does" (`Agent(...)`) and "how much of it is allowed to run" (`usage_limits=UsageLimits(...)` on each call).
- **Retry budget is configured independently of request limit** (`agent/abstract.py:106-129`): no combined "max steps" knob. Users must reason about both layers.
- **Public `finish_reason` enum** (`messages.py:114-120`) is a stable, OpenTelemetry-aligned `Literal`; each provider normalizes its native stop reason (`models/openai.py:4320`, `models/anthropic.py:956`, `models/google.py:161-176`, etc.). Loop does not consult `finish_reason == 'stop'` as a stop signal — that is implicit (the loop advances only by producing `End` from `CallToolsNode`).
- **Streaming-aware handoff** (`_agent_graph.py:638-757`): `wrap_model_request` cleanup is preserved across cancellation. Stream cooperation via `ready_waiter`/`stream_done` events. Cancellation is not a stuck loop; it's a separate state (`ModelResponseState`).
- **Durable execution (Temporal / DBOS / Prefect) reuses these limits unchanged** but adds its own retry layer (`docs/durable_execution/temporal.md:280-282`, `dbos.md:156-158`, `prefect.md:212-213` — recommended to keep Pydantic AI retries off and provider retries off to avoid double-retry storms). The agent-loop `UsageLimits` / `retries` survive the durable boundary; the durable engine adds activity / task retries on top.

## Notable Patterns

- **`End[RunEndT]` as the universal terminator** (`pydantic_graph/basenode.py`) — `End(FinalResult(...))` is how `CallToolsNode` and `SetFinalResult` end the run; the driver checks `isinstance(node, End)` and breaks (`pydantic_graph/graph.py:764-765`, `agent/abstract.py:439`).
- **`while not yielded` in `run_stream`** (`agent/abstract.py:784-911`) mirrors the non-streaming driver, with capability-hook awareness (`before_node_run`, `wrap_run_event_stream`, `wrap_node_run`, `after_node_run`).
- **`consume_output_retry` is the single chokepoint for output-side exhaustion** (`_agent_graph.py:162-179`); called from text-path fall-throughs in `_run_stream` and from `_build_retry_node`.
- **`UsageLimits` is a strict pre-check + post-check** design (`usage.py:379-420`): pre-request (`check_before_request`), post-response (`check_tokens`), pre-tool-call (`check_before_tool_call`). Atomic counters mean parallel tool calls are validated *before* execution.
- **`finish_reason` is interpreted only for `length` and `content_filter`** (`_agent_graph.py:1107-1130`). Normal `stop` and `tool_call` continue the loop; the framework does not (and arguably cannot) know what to do with them as terminal signals.
- **`Agent.iter` as a public lower-level entry** (`abstract.py:1188+`, `run.py:33-633`): exposes the graph run so callers can drive `next()` themselves, hook into bare async iteration, or build their own loop. The `After node run`, `before_node_run`, `wrap_node_run` capability hooks fire only via `Agent.run` or `AgentRun.next()`, not via bare `async for` (`run.py:193-233`, `exceptions.py:170-178`) — this is documented as the most common gotcha.
- **`retries` semantics per call site** (`agent/abstract.py:106-129`): at `Agent(...)` an integer sets both; at run/iter/override time it sets only `output`. Calling sites accept `retries={'tools': N}` only at the constructor.
- **Capability-driven termination** (`AbstractCapability.before_node_run`, `wrap_node_run`): a capability can substitute a different `End`-returning node and steer the loop without raising — but the framework's docs explicitly warn that bare `async for node in agent_run` skips these (`run.py:193-233`).

## Tradeoffs

- **Pros**:
  - Small, sharp vocabulary: `UsageLimits`, `AgentRetries`, `max_retries` (per-tool) + `tool_timeout` (per-tool). Each addresses a distinct failure mode. Defaults are conservative (1).
  - Typed, explicit, keyword-only fields — no hidden state. All limits are introspectable on the constructed object.
  - All limits configurable per-layer (per-tool / per-toolset / per-run / per-agent), with precedence explicitly documented (`docs/tools-advanced.md:490-492`, `docs/agent.md:1088-1100`).
  - Each exhaustion path raises a distinct exception class with a useful message; `UnexpectedModelBehavior` carries the body.
  - `count_tokens_before_request` (`usage.py:283-296`) is a rare feature — most frameworks only check after the response, which causes wasted cost on hopeless requests.
  - Streaming + non-streaming paths share the same limits; the cancellation contract is precise.
  - Tests cover every limit surface: `tests/test_usage_limits.py`, `tests/test_agent.py:3897-4162`, `tests/test_streaming.py`, `tests/v2/test_agent_retries_deprecation.py`.

- **Cons / risks**:
  - **No global "max steps"**: the framework is opinionated about per-tool / per-output retries / per-request limits but does not provide a single "max model turns" knob. Users typically layer `request_limit=…` on top of `retries=…` to get an effective ceiling; the relationship is implicit.
  - **No stuck-loop detection**: a model that returns identical outputs without triggering tool retries is not detected. The escape hatch is `request_limit` — which costs up to N model requests before tripping. The `infinite_retry_tool` example at `docs/agent.md:617-656` is presented as the canonical loop pattern; the recommended mitigation is `UsageLimits(request_limit=3)`.
  - **Per-tool `max_retries` does not generate `final_result` retries**: when a tool calls itself recursively (in a deferred / approval / `load_capability` cycle) the framework has no "self-referential tool" detector beyond the counter.
  - **`request_limit` default is 50**: large enough that a runaway loop can burn a non-trivial amount of money before tripping. The recommended pattern (`docs/agent.md:682`) is "set this yourself for production" — but that is advice, not enforcement.
  - **`usage_limits` default request_limit (50) vs `retries` default output (1)** can be confusing: a user setting `retries=10` does not realize that `request_limit=50` (default) still bounds by request count, not by retry budget.
  - **`count_tokens_before_request` is opt-in and provider-limited** (`usage.py:283-296`, supported for Anthropic, Google, Bedrock Converse, OpenAI Responses only). Other providers silently fall back to post-response checking.
  - **`_tool_timeout` semantics**: a timed-out tool raises `ModelRetry` and counts against `max_retries`. A user setting `tool_timeout=5` on a tool that always hangs hits `max_retries` first, not a clean timeout error.
  - **Bare `async for node in agent_run` skips `wrap_node_run` / `after_node_run`** — silently, with an `UndrainedPendingMessagesError` only when end-of-run redirects / `'when_idle'` messages were queued (`run.py:193-233`, `exceptions.py:170-178`). The error surfaces late and may be hard to attribute.
  - **`MessageHistory` growth is unbounded**: nothing trims `state.message_history`. A long-running run (with high `request_limit`) accumulates an arbitrarily large history — exhausted by token limits, not bounded by a turn count. (No evidence of a `max_messages` knob; consistent with `messages.py` / `_agent_graph.py`.)

## Failure Modes / Edge Cases

| Scenario | Outcome | Source |
|----------|---------|--------|
| Model returns empty response | `ModelResponse(parts=[])` → `consume_output_retry(...)` (text path, empty branch) → retry node, capped at `max_output_retries`; exhaustion raises `UnexpectedModelBehavior('Exceeded maximum output retries (N)')` | `_agent_graph.py:1166-1173` |
| Model returns only thinking | `is_thinking_only = all(isinstance(p, ThinkingPart))` → attempts to recover text from history; on failure synthesizes `Please call a tool / return text` `RetryPromptPart` | `_agent_graph.py:1102-1248` |
| `finish_reason == 'length'` on empty response | Immediate raise `UnexpectedModelBehavior('Model token limit (...) exceeded before any response was generated.')`; no retry | `_agent_graph.py:1107-1114` |
| `finish_reason == 'length'` mid-tool-call | `check_incomplete_tool_call()` → `IncompleteToolCall(...)` (subclass of `UnexpectedModelBehavior`) | `_agent_graph.py:146-160`, `exceptions.py:308-309` |
| `finish_reason == 'content_filter'` on empty response | `ContentFilterError(UnexpectedModelBehavior)` with body; message classifies as Finish reason / Block reason / Refusal | `_agent_graph.py:1117-1130`, `exceptions.py:232-234` |
| Unknown tool name (e.g. `ToolCallPart('foobar', ...)`) | Tool dispatch surfaces `UnexpectedModelBehavior` with `"Unknown tool name"`; counted under that tool's retry budget; after `max_retries` → `"Tool 'foobar' exceeded max retries count of {N}"` | `tool_manager.py:177-181`, tests `tests/test_agent.py:3897-3944` |
| Tool `args_valid=False` (Pydantic validation fail) | Wrapped to `RetryPromptPart`; counted as one retry of *that tool*; respects `max_retries` | `tool_manager.py:184-191`, `toolsets/function.py:589-635` |
| Tool raises `ModelRetry` | Same `RetryPromptPart` mechanism; counted as one retry of that tool | `tool_manager.py:184-191` |
| Tool hangs (no timeout configured) | Loop blocks until cancellation; request and token counters only increment per request, not per tool hang. **No stuck-tool timeout unless `tool_timeout=` is set.** | `toolsets/function.py:644-658` |
| Tool times out (`tool_timeout=5`) | `anyio.fail_after(5)` raises `ModelRetry('Timed out after 5 seconds.')`; counts toward `max_retries` | `toolsets/function.py:651-657` |
| User passes `retries={'tools': ...}` to `agent.run` | `UserError` raised (construct time vs run time semantics differ) | `agent/abstract.py:117-119` |
| Parallel tool calls exceed `tool_calls_limit` | `check_before_tool_call` raises *before* any tool runs; no partial execution | `usage.py:413-420`, `_agent_graph.py:1705-1708` |
| Caller wraps in `input_tokens_limit` but model emits shorter context | Limit only enforced pre-count if `count_tokens_before_request=True`, else post-response (token accounting might overshoot) | `usage.py:283-296`, `_agent_graph.py:952-960` |
| `count_tokens_before_request=True` on unsupported provider | Silently ignored (no model.count_tokens → counter stays uncounted pre-request; still enforced post-response) | `usage.py:283-296` |
| `agent.iter` user uses bare `async for` instead of `.next()` | Capability hooks (`wrap_node_run`, `after_node_run`) skipped; `UndrainedPendingMessagesError` raised at end if `'when_idle'` queues were not drained | `run.py:193-233`, `exceptions.py:170-178` |
| Stream cancellation mid-`wrap_model_request` | `cancel_and_drain(ready_waiter, wrap_task)` runs both halves; result is `ModelResponse.state='interrupted'`; resumed runs may fail or retry | `_agent_graph.py:638-757`, `messages.py:123`, `docs/output.md:1011` |
| `End(final_result)` reached with pending `pending_messages` queue | `UndrainedPendingMessagesError` raised; the run had messages enqueued but no path drained them | `exceptions.py:170-178` |
| `ToolRetryError` from output-tool validation | Counted as `output_retries_used += 1` (`_agent_graph.py:1624, 1653`); on exhaust same `UnexpectedModelBehavior` |
| Concurrency: queue depth exceeded | `ConcurrencyLimitExceeded(AgentRunError)` raised when `max_queued` is hit; documented for `max_concurrency`-backed agents | `exceptions.py:199-200`, `docs/agent.md` (concurrency sections), `tests/test_concurrency.py:102-283` |
| Tool returns `CallDeferred` | Tool result is deferred as a `DeferredToolRequests`; run ends with `FinalResult(DeferredToolRequests)` via `End`; no exhausted retries | `_agent_graph.py:1045-1079`, `tools.py:600-602` (deferred-tools.md) |
| Tool returns `ApprovalRequired` | Same as `CallDeferred` (human-in-the-loop path) | `tools.py:598-602` |

## Future Considerations

- **Per-agent / per-run global step counter**: would close the gap that `request_limit` is the only loop-bound mechanism. The framework already has `run_step` on `GraphAgentState` (`_agent_graph.py:128`, `_agent_graph.py:856`) used for capability refresh — extending it to a termination condition would be small.
- **Stuck-loop detection**: a lightweight repetition counter (e.g. `hash(prev_response) == hash(curr_response)` over a sliding window) would catch the canonical failure pattern before the request limit fires. Currently this is a clean addition — no invariants seem to forbid it.
- **Combined `max_steps` knob**: a single keyword that maps to `max(request_limit, retries.text_path × effective_output_tools + retries.tools × n_tools + 1)` would reduce the misconfiguration surface that the `infinite_retry_tool` example illustrates.
- **Lower default `request_limit`**: a "safe by default" change. The current default of 50 was likely chosen for development convenience; production guidance already says to lower it.
- **`count_tokens_before_request` broader support**: the list of supported providers (Anthropic, Google, Bedrock Converse, OpenAI Responses) is curated (`usage.py:283-296`); extending to Mistral, Groq, Cerebras, Cohere would close a real cost leak.
- **History trimming**: `state.message_history` is unbounded; a `history_processor` exists (`_history_processor.py`, `messages-history.md`) but is opt-in. A `max_messages` knob would prevent pathological growth in long-running agents.
- **Distinguishing success vs exhaustion at the result layer**: a `RunResult.status: Literal['complete', 'usage_exhausted', 'retry_exhausted', 'canceled']` (or similar) would let users branch without `try/except`. Currently every exhaustion path requires catching distinct exceptions.
- **`tool_timeout` clean error path**: a timeout could raise a dedicated `ToolTimeoutError` (not silently re-enter the retry budget). Currently the only signal is `ModelRetry("Timed out after {t} seconds")`.

## Questions / Gaps

- **No evidence found** of a `max_concurrent_messages` or `max_pending_messages` length cap on `state.message_history`. A long-running agent with high `request_limit` grows it without bound; this is consistent with documented `messages-history.md` guidance but creates a hidden risk.
- **No evidence found** of any stuck-loop / repetition-detection heuristic in the agent loop. The framework relies entirely on the user setting `UsageLimits(request_limit=...)`.
- **No evidence found** of a global `max_steps` knob — only the layered `request_limit` + per-tool `max_retries` + output-retry budget exist.
- **No evidence found** of a `messages_history` length-bounded trim, even with `count_tokens_before_request=True`. Trimming is purely the user's responsibility via `history_processors`.
- **No evidence found** of a `max_consecutive_repetitions` or `max_idle_iterations` knob. Only `tool_calls_limit` caps the *successful* tool execution count; failing tools are bounded only by `max_retries`.
- **No evidence found** of platform-specific stuck detection (e.g. `tenacity`'s `stop_after_attempt`) at the loop layer; tenacity is used only for HTTP transport retries.
- **No evidence found** of automatic `retries` retry-storm prevention across multiple tools (each tool has its own counter; there's no agent-wide counter).

---

Generated by `01.04-termination-and-loop-bounds` against `pydantic-ai`.
