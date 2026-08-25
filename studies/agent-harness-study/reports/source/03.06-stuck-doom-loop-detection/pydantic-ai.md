# Source Analysis: pydantic-ai

## 03.06 — Stuck and Doom-Loop Detection

### Source Info

| Field | Value |
|-------|-------|
| Name | pydantic-ai |
| Path | `studies/agent-harness-study/sources/pydantic-ai` |
| Language / Stack | Python 3.10+; `pydantic_ai_slim` agent framework + `pydantic_graph` typed-graph runtime that backs the agent loop (`pydantic_graph/pydantic_graph/graph.py:549-800` drives all `Agent.run*` paths); `pydantic_evals` evaluation framework; `clai` CLI; optional provider integrations (`openai`, `anthropic`, `google`, `bedrock`, `cohere`, `groq`, `mistral`, `huggingface`, `outlines`, `test`, `gateway`) and durable-execution adapters (`temporal`, `prefect`, `dbos`, `kitaru`, `restate`). Agent model and execution are layered over `pydantic_graph`'s `BaseNode`/`GraphRun` engine (graph repo: `pydantic_graph/pydantic_graph/`). |
| Analyzed | 2026-07-15 |

## Summary

Pydantic AI ships **no first-class doom-loop or stuck-pattern detector**. Pattern recognition is delegated entirely to the LLM (via `ModelRetry` → `RetryPromptPart` as a hint) and to the user (via `Agent(retries=...)`, `UsageLimits(request_limit=...)`, and `UsageLimits(tool_calls_limit=...)`). The framework contributes five blunt count-based caps:

1. **Per-tool retry budget** keyed on `RunContext.retries[name]` with default `1` (`pydantic_ai_slim/pydantic_ai/tool_manager.py:92-93`, `177-181`). On exhaustion, `ToolManager._check_max_retries` raises `UnexpectedModelBehavior(f"Tool {name!r} exceeded max retries count of {max_retries}")` (`tool_manager.py:177-181`).
2. **Output-validation retry budget** (`max_output_retries`), per-run `output_retries_used: int` on `GraphAgentState` (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:127, 162-179`). On exhaustion, `consume_output_retry` raises `UnexpectedModelBehavior("Exceeded maximum output retries ({max_output_retries})")` (`_agent_graph.py:175-179`).
3. **Request cap** on `UsageLimits.request_limit: int | None = 50` checked in `UsageLimits.check_before_request` and called from `ModelRequestNode._prepare_request` (`pydantic_ai_slim/pydantic_ai/usage.py:273, 379-395`, `pydantic_ai_slim/pydantic_ai/_agent_graph.py:960`). Default `50`.
4. **Tool-call cap** on `UsageLimits.tool_calls_limit: int | None = None` projected against the next call via `check_before_tool_call` and called from `process_tool_calls` (`pydantic_ai_slim/pydantic_ai/usage.py:275, 413-421`, `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1704-1708`). Default `None` (unbounded).
5. **Token caps** (`input_tokens_limit`, `output_tokens_limit`, `total_tokens_limit`) checked after each response (`pydantic_ai_slim/pydantic_ai/usage.py:397-411`) and (optionally) before each request when `count_tokens_before_request=True` (`usage.py:283-296`).

None of these inspect recent tool calls, message history, or model response shape to recognise a *pattern*. They count attempts and surface a hard error after the budget is exhausted. The only "retry hint" surfaced in the conversation is the `RetryPromptPart` written back into the model history when a tool body or validator raises `ModelRetry` (`pydantic_ai_slim/pydantic_ai/tool_manager.py:183-191, 339-350, 763-765`, and `_agent_graph.py:1027-1040` which builds a retry `ModelRequestNode` for `ModelRetry` from `wrap_model_request` or `on_model_request_error`). The hint is text the LLM reads; the framework never compares calls to each other.

`grep -rn` for `stuck`, `doom`, `consecutive.{0,30}tool`, `repeated_action`, `same_tool`, `no_progress`, `replan`, `recent_turn`, and `loop_detect` across `pydantic_ai_slim/` and `pydantic_graph/` returns **0 relevant matches** in the framework code itself. The only "loop"-adjacent name in the engine is `pydantic_graph.parent_forks.GraphNodeStatusError.check`, which guards *graph-node* re-entry (not an iteration-counter-detector) and only fires on snapshot-status corruption (`pydantic_graph/pydantic_graph/graph.py:740-751` snapshots each node, `pydantic_graph/pydantic_graph/_utils.py:53-59` is just `asyncio.get_event_loop`).

The `pydantic_graph.GraphRun` loop has no maximum-step counter of its own. `GraphRun.next(node)` (`pydantic_graph/pydantic_graph/graph.py:664-753`) just persists state and awaits the node's `run(ctx)`; iteration ends only when a node returns `End[result]` or the user stops iterating. There is no `recursion_limit` knob — the closest thing is `UsageLimits.request_limit` in the agent layer.

**Answering the prompt's headline question ("Does the system stop repeated behavior before the user pays for 20 useless turns?"):** **No.** The default `UsageLimits(request_limit=50)` (set when no `usage_limits=` argument is supplied — `pydantic_ai_slim/pydantic_ai/agent/__init__.py:1312: usage_limits = usage_limits or _usage.UsageLimits()`) allows up to 50 model invocations, *not* 20. An LLM calling `get_weather` 50 times in a row with the same arguments and the same answer will run all 50 times. The framework raises `UsageLimitExceeded` only after the budget is gone; it never detects the repetition in-flight. The per-tool `max_retries` and `max_output_retries` (defaults `1`) fire only when the tool/validator keeps raising `ModelRetry` — successful repeated calls produce no retry errors and therefore consume no retry budget.

## Rating

**Score: 4 / 10** — Present but inconsistent and blunt. Pydantic AI has five count-based caps and a `ModelRetry`-driven retry hint, and each cap raises a hard error (`UnexpectedModelBehavior` for retry exhaustion, `UsageLimitExceeded` for usage-limit exhaustion). All are exactly that — count caps — and none inspect recent tool calls, recent errors, or message content to detect repeating patterns. The defaults are inconsistent: `request_limit=50` but `tool_calls_limit=None` (a successful tool-loop with no retries burns through 50 model turns); per-tool `max_retries=1` (one extra attempt) but `output_retries` also default to 1 (text validator gets one retry). The retry hint (`RetryPromptPart`) is text the LLM reads; the framework never compares a call to its predecessors. No detector for A→B→A alternation, no sliding window of recent tool names, no detection of "I have called the same N tools five times in a row with no change in `ToolReturn` content". False *negatives* are the norm; false positives are possible only in the sense that a tool whose `ModelRetry` budget is too tight will be killed early. There are no tests for "stuck-loop detection" because there is no detector — `tests/test_tools.py` has `test_infinite_retry_tool` (asserts the count cap fires) and `test_unknown_tool_per_tool_retries_exceeded` (asserts the same for unknown tools), `tests/test_usage_limits.py` has `test_parallel_tool_calls_limit_enforced`, but none exercise a model that loops on its own.

## Evidence Collected

Every entry includes file path and line numbers. Format: `path/to/file.py:NN`.

### Per-tool retry budget (count cap on a single tool's failed calls)

| Area | Evidence | File:Line |
|------|----------|-----------|
| Default per-tool retry budget | `default_max_retries: int = 1` | `pydantic_ai_slim/pydantic_ai/tool_manager.py:92-93` |
| Public default — `Agent(retries=None)` resolves both `tools` and `output` to `1` | `_normalize_agent_retry_overrides(retries)` → `{'tools': retries, 'output': retries}` then `default=1` | `pydantic_ai_slim/pydantic_ai/agent/__init__.py:142-148, 498-507` |
| Per-tool `_check_max_retries` raises `UnexpectedModelBehavior` | `if self.ctx.retries.get(name, 0) == max_retries: raise UnexpectedModelBehavior(f'Tool {name!r} exceeded max retries count of {max_retries}') from error` | `pydantic_ai_slim/pydantic_ai/tool_manager.py:177-181` |
| Default `max_retries` of `Tool` propagates to `RunContext.max_retries` and to the failure check | `_build_tool_context`: `retry=self.ctx.retries.get(call.tool_name, 0)`, `max_retries=tool.max_retries` | `pydantic_ai_slim/pydantic_ai/tool_manager.py:203-213` |
| Retry counter survives across steps | `retries = {failed_tool_name: self.ctx.retries.get(failed_tool_name, 0) + 1 for failed_tool_name in self.failed_tools}` in `ToolManager.for_run_step` | `pydantic_ai_slim/pydantic_ai/tool_manager.py:120-145` |
| `last_attempt` flag the tool body can read | `return self.retry == self.max_retries` | `pydantic_ai_slim/pydantic_ai/_run_context.py:153-156` |
| `ModelRetry` from the tool body is wrapped into a `RetryPromptPart` sent back to the model | `_wrap_error_as_retry` builds `RetryPromptPart(tool_name=name, content=..., tool_call_id=call.tool_call_id)` | `pydantic_ai_slim/pydantic_ai/tool_manager.py:183-191` |
| Tests assert the cap raises after N failures (default `max_retries=1`, plus `infinite_retry_tool` test) | `pytest.raises(UnexpectedModelBehavior, match="Tool 'infinite_retry_tool' exceeded max retries count of 5")` | `tests/test_tools.py:1490-1491`, `tests/test_toolsets.py:801-940, 1027` |

### Output-validation retry budget (count cap on text/output-tool validation failures)

| Area | Evidence | File:Line |
|------|----------|-----------|
| Per-state counter | `output_retries_used: int = 0` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:127` |
| Counter check / escalation | `self.output_retries_used += 1; if self.output_retries_used > max_output_retries: ... raise UnexpectedModelBehavior(...)` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:162-179` |
| Carries also the `IncompleteToolCall` check before raising | `def check_incomplete_tool_call(self): raise IncompleteToolCall(...)` then `raise UnexpectedModelBehavior(message)` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:146-160, 175-179` |
| Empty / thinking-only / no-output responses bump the counter | `ctx.state.consume_output_retry(ctx.deps.max_output_retries)` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1145-1146, 1171-1172` |
| Build-retry-node path: `wrap_model_request` / `on_model_request_error` raise `ModelRetry` is turned into a retry `ModelRequestNode` with a `RetryPromptPart` | `_build_retry_node` constructs `ModelRequestNode(_messages.ModelRequest(parts=[m]))` and bumps the counter | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1027-1040` |
| Per-tool retry on the output validator path | `_check_max_retries(name, tool.max_retries, cause)` inside the `ToolRetryError` arm of `process_tool_calls` for output tools | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1647-1654` |
| Per-tool override via `ToolOutput(max_retries=N)` | `_max_retries_overrides: dict[str, int]` is consulted in `OutputToolset.build` | `pydantic_ai_slim/pydantic_ai/_output.py:1411, 1486-1487, 1518-1524` |
| Public per-run override (`Agent.run(retries={'output': N})`) | At construction, `int` sets both; at run/override, `int` overrides only `output`; `'tools'` is rejected at run time | `pydantic_ai_slim/pydantic_ai/agent/__init__.py:142-167, 1178-1186` |
| Tests assert cap raises | `pytest.raises(UnexpectedModelBehavior, match='Exceeded maximum output retries')` cases throughout | `tests/test_agent.py:5740-5984, 10735, 11137` |

### `UsageLimits` request and tool-call caps (turn and successful-tool counters)

| Area | Evidence | File:Line |
|------|----------|-----------|
| `UsageLimits` dataclass defaults — `request_limit=50`, `tool_calls_limit=None`, token limits `None` | `request_limit: int \| None = 50; tool_calls_limit: int \| None = None; ...` | `pydantic_ai_slim/pydantic_ai/usage.py:263-296` |
| Pre-request cap | `if request_limit is not None and usage.requests >= request_limit: raise UsageLimitExceeded(...)` | `pydantic_ai_slim/pydantic_ai/usage.py:379-395` |
| Pre-tool-call cap (projected against batch) | `if tool_calls_limit is not None and tool_calls > tool_calls_limit: raise UsageLimitExceeded(...)` | `pydantic_ai_slim/pydantic_ai/usage.py:413-421` |
| Wired in `ModelRequestNode._prepare_request` | `ctx.deps.usage_limits.check_before_request(usage)` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:960` |
| Wired in `process_tool_calls` (projected before execution) | `projected_usage.tool_calls += len(calls_to_run); ctx.deps.usage_limits.check_before_tool_call(projected_usage)` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1704-1708` |
| Token-cap check after each response | `check_tokens(usage)` invoked from `ModelResponse._append_response` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1015-1025`, `pydantic_ai_slim/pydantic_ai/usage.py:397-411` |
| Optional pre-request token counting | `count_tokens_before_request: bool = False`, `usage_limits.count_tokens_before_request` | `pydantic_ai_slim/pydantic_ai/usage.py:283-296` |
| Default constructed when caller omits `usage_limits` | `usage_limits = usage_limits or _usage.UsageLimits()` — defaults above | `pydantic_ai_slim/pydantic_ai/agent/__init__.py:1312` |
| Doc explicitly markets request_limit as loop prevention | "Restricting the number of requests can be useful in preventing infinite loops or excessive tool calling" | `docs/agent.md:617-683` |
| Tests assert the caps fire | `test_parallel_tool_calls_limit_enforced`, `Exceeded the request_limit`, `UsageLimitExceeded` | `tests/test_usage_limits.py:48-79, 506-922` |

### `ModelRetry` → `RetryPromptPart` (in-loop retry hint, not detection)

| Area | Evidence | File:Line |
|------|----------|-----------|
| `ModelRetry` exception users raise from tools / validators / capability hooks | `class ModelRetry(Exception): message: str` | `pydantic_ai_slim/pydantic_ai/exceptions.py:40-77` |
| `RetryPromptPart` written back into the next `ModelRequest` | `RetryPromptPart(tool_name=name, content=content, tool_call_id=call.tool_call_id)` | `pydantic_ai_slim/pydantic_ai/tool_manager.py:190`, `pydantic_ai_slim/pydantic_ai/messages.py:1570-...` |
| Wrapped as `ToolRetryError(m)` so retry-budget accounting can see it | `def _wrap_error_as_retry(name, call, error) -> ToolRetryError` | `pydantic_ai_slim/pydantic_ai/tool_manager.py:183-191` |
| Caught in `_handle_tool_calls` → bumps `output_retries_used` and builds a `RetryPromptPart`-only request | `ctx.state.consume_output_retry(...); self._next_node = ModelRequestNode(_messages.ModelRequest(parts=[e.tool_retry]))` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1247-1249` |
| `wrap_model_request` (capability) raising `ModelRetry` triggers the same retry-node path | `return await self._build_retry_node(ctx, e)` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:829-835, 1027-1040` |

### Empty / thinking-only response recovery (re-prompt for no-output, not loop detection)

| Area | Evidence | File:Line |
|------|----------|-----------|
| Empty / thinking-only response rebuilds a retry `ModelRequest` and consumes an output retry | empty path: `ctx.state.consume_output_retry(...); self._next_node = ModelRequestNode(_messages.ModelRequest(parts=[]))` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1110-1173` |
| Recovery path that recovers text from prior responses (still resets — not "progress vs no-progress" detection) | `_recover_text_from_message_history` walks history backward | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1301-1320` |
| `is_empty or is_thinking_only` causes `raise ToolRetryError(m)` with `alternatives = ['call a tool', 'return text', ...]` — the framework *prompts* the model for actionable output but does not record that it has prompted before | `raise ToolRetryError(m)` after building `alternatives` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1210-1246` |

### `pydantic_graph` engine: no doom-loop detector, no node-cycle detection

| Area | Evidence | File:Line |
|------|----------|-----------|
| `GraphRun.next(node)` body — just persists, awaits, and records `End`/`BaseNode`; no cycle/recursion-cap counter | `self._next_node = await node.run(ctx); if isinstance(self._next_node, End): ... elif isinstance(self._next_node, BaseNode): ...` | `pydantic_graph/pydantic_graph/graph.py:738-753` |
| `GraphRun` has no step counter (only `_is_started: bool`) | `self._is_started: bool = False` | `pydantic_graph/pydantic_graph/graph.py:633` |
| `parent_forks.py` comment admits the visited-tracking is "to prevent infinite loops if there are bugs" — i.e. not user-visible | "The visited-tracking shouldn't be necessary, but I included it to prevent infinite loops if there are bugs" | `pydantic_graph/pydantic_graph/parent_forks.py:115` |
| Async iteration just continues until `End` | `async def __anext__: ... if isinstance(self._next_node, End): raise StopAsyncIteration; return await self.next(self._next_node)` | `pydantic_graph/pydantic_graph/graph.py:758-767` |
| `GraphRuntimeError` is the only `GraphRun` failure mode — no recursion error class | `class GraphRuntimeError(RuntimeError)` | `pydantic_graph/pydantic_graph/exceptions.py:40-48` |

### `end_strategy` — orthogonal concern (controls whether remaining tool calls run after a final result, not loop detection)

| Area | Evidence | File:Line |
|------|----------|-----------|
| `'early'` (default) skips remaining function-tool calls once a final result is found | `if final_result and ctx.deps.end_strategy == 'early': for call in tool_calls_by_kind['function']: output_parts.append(_messages.ToolReturnPart(tool_name=call.tool_name, content='Tool not executed - a final result was already processed.', tool_call_id=call.tool_call_id))` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1666-1674` |
| `'graceful'` skips remaining *output* tools but still runs function tools | docstring `"'graceful'`: Output tools are executed first. Once a valid final result is found, remaining output tool calls are skipped, but function tools are still executed` | `pydantic_ai_slim/pydantic_ai/agent/__init__.py:215-219` |
| `'exhaustive'` runs everything and selects the first valid output-tool result | docstring — same | `pydantic_ai_slim/pydantic_ai/agent/__init__.py:215-219` |
| Tests | `test_exhaustive_strategy_second_output_max_retries_exceeded`, `test_early_strategy_second_output_max_retries_exceeded` | `tests/test_agent.py:5919-5984` |

### Evidence erasure (compaction affects only structural cleanup, not loop evidence *as such*)

| Area | Evidence | File:Line |
|------|----------|-----------|
| `_clean_message_history` merges only structurally adjacent same-role messages (e.g. `ModelRequest` + `ModelRequest`) — does not analyse call patterns | `if isinstance(message, _messages.ModelRequest): if last_message and isinstance(last_message, _messages.ModelRequest) and (not last_message.instructions or not message.instructions or last_message.instructions == message.instructions): merged_message = _messages.ModelRequest(parts=[*last_message.parts, *message.parts], ...)` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:2220-2269` |
| Provider-side `CompactionPart` is opaque and round-tripped only between the same provider (OpenAI Responses / Anthropic) | `class CompactionPart: """A compaction part that summarizes previous conversation history."""`, "Compaction data is only sent back to the same provider." | `pydantic_ai_slim/pydantic_ai/messages.py:1682-1730` |
| OpenAI/Anthropic compaction adapters translate `CompactionPart` to local-shape but never inspect repeated calls | `elif isinstance(item, CompactionPart): pass  # Compaction parts are not sent back to models that don't support compaction.` | `pydantic_ai_slim/pydantic_ai/models/openai.py:1326-1327`, `pydantic_ai_slim/pydantic_ai/models/anthropic.py` (parallel) |
| `pydantic_ai` has no client-side compaction policy (only provider-side opaque summaries) | No `compact`, `summarize`, or `truncate` policy found in `_agent_graph.py`, `_history_processor.py`, `capabilities/process_history.py` | `pydantic_ai_slim/pydantic_ai/_history_processor.py:1-25`, `pydantic_ai_slim/pydantic_ai/capabilities/process_history.py:1-100` |

### `consume_deprecated_output_retries` (forward of legacy `output_retries=` kwarg into `retries={'output': ...}`)

| Area | Evidence | File:Line |
|------|----------|-----------|
| Migration helper — not a detector; only translates legacy kwarg to new retry dict | `def consume_deprecated_output_retries(...)` | `pydantic_ai_slim/pydantic_ai/_utils.py:1027-1057` |

## Answers to Dimension Questions

1. **What stuck patterns are detected?**
   None. Pydantic AI does not detect repeated identical tool calls, alternating A→B→A tool patterns, repeated-error streaks, or "no-progress monologues". The framework offers five count caps — per-tool `max_retries` (default `1`), per-run `max_output_retries` (default `1`), `UsageLimits.request_limit` (default `50`), `UsageLimits.tool_calls_limit` (default `None`), and three token caps (`input_tokens_limit`, `output_tokens_limit`, `total_tokens_limit`, all default `None`) — plus the `RunContext.last_attempt` boolean (`pydantic_ai_slim/pydantic_ai/_run_context.py:153-156`), which is a tool-side hint, not a detector. Every cap surfaces a hard error (`UnexpectedModelBehavior` for retry exhaustion, `UsageLimitExceeded` for usage-limit exhaustion) rather than a graded intervention. See `pydantic_ai_slim/pydantic_ai/tool_manager.py:177-181`, `pydantic_ai_slim/pydantic_ai/_agent_graph.py:162-179`, `pydantic_ai_slim/pydantic_ai/usage.py:379-421`.

2. **How far back does detection look?**
   Not applicable. Detection (such as it is) is purely count-based against `RunContext.retries[name]` (per-tool) and `GraphAgentState.output_retries_used` (global output); there is no sliding window of recent tool calls or recent error messages. The request counter (`usage.requests`) is checked against `request_limit` before each model call in `ModelRequestNode._prepare_request` (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:960`). The history itself (`ctx.state.message_history`) is a `list[ModelMessage]` (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:125`), so an external observer could scan it for patterns, but the framework does not.

3. **Does compaction erase loop evidence?**
   Yes — *if* the user opts into provider-side compaction. Client-side `_clean_message_history` (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:2220-2269`) is structural only and preserves all parts when merging adjacent same-role messages. The provider-driven `CompactionPart` (`pydantic_ai_slim/pydantic_ai/messages.py:1682-1730`) replaces past history with an opaque token (or returns nothing at all on providers that don't support it; see `pydantic_ai_slim/pydantic_ai/models/openai.py:1326-1327`, `models/anthropic.py` mirror). Any future detector that wanted raw tool-call evidence would lose it on those providers. Note that this also means the framework *itself* does not forget — repeated calls remain in `message_history` for the duration of the run.

4. **What intervention happens?**
   The same five caps fire five hard errors. There is no hint, no escalation, no replanning, and no automatic recovery (`grep -rn` for `replan|escalation|fallback|intervention` returns matches only in model-layer patterns and provider-fallback code: `pydantic_ai_slim/pydantic_ai/models/fallback.py:70-244` is for *falling back to an alternative model*, not for breaking a loop). On tool retry exhaustion: `ToolManager._check_max_retries` raises `UnexpectedModelBehavior` (`tool_manager.py:177-181`); on output retry exhaustion: `GraphAgentState.consume_output_retry` raises `UnexpectedModelBehavior` (`_agent_graph.py:175-179`); on request/tool-call limit overflow: `UsageLimits.check_before_request` / `check_before_tool_call` raise `UsageLimitExceeded` (`usage.py:382-383, 415-419`). The in-conversation retry hint is `ModelRetry` → `RetryPromptPart` (`tool_manager.py:183-191`), but that hint is consumed by the LLM in the *next* model call (`_agent_graph.py:1037-1040, 1247-1249`); it does not feed any framework-side decision.

5. **Are false positives possible?**
   Only in the trivial sense: a tool whose `max_retries=N` is misconfigured for its environment (e.g., a flaky remote tool with `max_retries=0`) will be killed after one failure. False negatives are the norm: a model calling the same successful tool repeatedly with the same arguments and a stable answer will run through all 50 turns of the default `request_limit` without any framework-level signal. The framework has no detector, so there are no false positives from a detector — only from hard caps that fire under conditions the user did not anticipate.

## Architectural Decisions

- **Detection is delegated to the LLM, not the harness.** `ModelRetry` writes a `RetryPromptPart` into the next model request (`_agent_graph.py:1027-1040`) and trusts the model to alter its behaviour. The harness contributes only a counter that turns into a hard error after `N` retries. This is documented as the canonical example: `docs/agent.md:617-683` shows how to combine `retries={'tools': 3}` and `UsageLimits(request_limit=3)` to bound an "infinite retry loop" (`docs/agent.md:622-656`).
- **Five layered caps with inconsistent defaults.** Per-tool `default_max_retries=1` (`tool_manager.py:92-93`), per-run `max_output_retries=1` (from `retries=None` resolution at `agent/__init__.py:142-148, 505-507`), `request_limit=50` (`usage.py:273`), `tool_calls_limit=None` (`usage.py:275`), token limits `None`. A typical agent is bounded by per-tool retry (one extra attempt) and `request_limit=50`. There is no single "iteration cap" knob analogous to OpenAI Agents SDK's `max_turns` or LangGraph's `recursion_limit`.
- **Errors are programming errors, not graceful bailouts.** `UnexpectedModelBehavior` (`pydantic_ai_slim/pydantic_ai/exceptions.py:203-229`) extends `AgentRunError` and is documented as "Error caused by unexpected Model behavior, e.g. an unexpected response code" — i.e., the framework's contract says a tool that retries past its budget means the developer misconfigured the agent. `UsageLimitExceeded` (`exceptions.py:195`) is the same shape. Neither offers an automatic "inject a fallback prompt and continue" branch; the user must catch the exception.
- **Counters reset across tools but persist per-tool.** `RunContext.retries` is a `dict[str, int]` (`_run_context.py:60-77`) keyed on tool name; `ToolManager.for_run_step` propagates +1 increments for failed tools across steps (`tool_manager.py:120-145`). The global `output_retries_used` (`_agent_graph.py:127`) is per-run. There is no notion of "consecutive failures of the same tool" — every failure of `tool_name` bumps that name's counter by one.
- **`end_strategy` is *not* a stuck-loop detector.** It only governs whether remaining tool calls run after a final result has been found (`agent/__init__.py:215-219`, `_agent_graph.py:1666-1674`). The closest prebuilt equivalent of "soft bail" is the ReAct guide's recommendation to keep `UsageLimits(request_limit=K)` low, which is itself a count cap.
- **`pydantic_graph.GraphRun.next()` has no step cap.** `GraphRun` exposes `next_node` / `result` but no `max_steps` (`pydantic_graph/pydantic_graph/graph.py:549-668`). Iteration relies entirely on `End[result]` being returned; an `async for node in agent_run` loop only stops when the agent decides to stop. The graph-level `parent_forks` comment (`parent_forks.py:115`) admits that visited-tracking exists "to prevent infinite loops if there are bugs" — it is *not* a user-facing detector.

## Notable Patterns

- **Retry hint via `RetryPromptPart` written into the same `ModelRequest` slot.** A tool that raises `ModelRetry` becomes a `RetryPromptPart(tool_name=name, content=..., tool_call_id=...)` (`tool_manager.py:183-191`), which `_clean_message_history` (`_agent_graph.py:2220-2269`) co-locates at the start of the next `ModelRequest`'s parts alongside `ToolReturnPart`s. This is the closest the framework gets to "tell the model what just happened" — it does not analyse repetition.
- **`RunContext.retries` + `last_attempt`.** The per-tool retry counter is exposed to the tool via `RunContext.retry` (scalar, the count for *this* tool) and `RunContext.last_attempt` (boolean: `retry == max_retries`) (`_run_context.py:60-77, 153-156`). Tools can read these to prepare a richer final attempt, e.g., a polite-summary mode. This is the only "are we about to fail" signal surfaced to user code, and it is **incremental, not consecutive** — a successful retry resets nothing.
- **`consume_output_retry` couples the retry counter with `check_incomplete_tool_call`.** Before raising `UnexpectedModelBehavior('Exceeded maximum output retries (...)')`, the state helper calls `self.check_incomplete_tool_call()` to convert a `ModelResponse` with `finish_reason='length'` truncated mid-tool-call into the more actionable `IncompleteToolCall` (`_agent_graph.py:146-160, 175-179`). The intent is to surface the *cause*, but the implementation is still count-based — only two specific reasons are bucketed, and both happen after the budget is gone.
- **Empty-response / thinking-only-response → re-prompt path.** When the model returns no actionable parts, `CallToolsNode._run_stream` rebuilds a retry `ModelRequestNode(parts=[])` and consumes one output retry (`_agent_graph.py:1110-1173`). A genuine "model keeps returning `''`" loop is eventually bounded by `max_output_retries` — but the framework does not remember "the model has done this before"; it relies on the running counter.
- **`ToolManager.failed_tools` is a per-step set, not a window.** `failed_tools: set[str] = field(default_factory=set[str])` (`tool_manager.py:90-91`) tracks names of tools that failed validation/execution in the current step. `for_run_step` consumes it to propagate the retry counter across the step boundary (`tool_manager.py:120-145`). It is a per-step bookkeeping set, not a recent-failure window.

## Tradeoffs

- **Trusting the LLM to detect loops.** Same shape as the OpenAI Agents SDK decision: cheaper to build; only fails badly when the LLM itself is the source of the repetition. Combined with `request_limit=50` this turns an "agent stuck on a successful loop" into a 50-turn `UsageLimitExceeded`.
- **All caps surface hard errors.** `UnexpectedModelBehavior` and `UsageLimitExceeded` are both `AgentRunError` subclasses (`exceptions.py:181-229`). No internal recovery: the user catches and decides. This is honest but means there is no automatic "this looks like a loop, here's a hint" branch — every intervention is a hard stop.
- **Per-tool counter is keyed on `name`, not `name+args`.** Two distinct calls to `lookup_user(123)` and `lookup_user(456)` both bump `RunContext.retries['lookup_user']` to `1` (separately or together depending on timing). Whether the same `max_retries` budget is consumed by these distinct calls depends on whether they were attempted within the same `for_run_step`. There is no de-dupe on `name+args`.
- **`request_limit=50` is generous for a typical ReAct.** The default lets the agent run for 50 model turns before stopping. `multi-agent-applications.md:21` acknowledges this with `usage_limits=UsageLimits(request_limit=5, total_tokens_limit=500)`. Without explicit user tuning, a runaway agent runs 50 turns.
- **`tool_calls_limit=None` defaults to unbounded.** `usage.py:275`; `check_before_tool_call` skips the check entirely when `tool_calls_limit is None` (`usage.py:415-417`). Combined with `request_limit=50`, a successful tool-loop runs to model-turn-50 by default. Test `test_parallel_tool_calls_limit_enforced` (`tests/test_usage_limits.py:864-922`) only runs because the test sets the cap.
- **`ModelRetry` is a tool-side signal, not a detector.** A tool that always raises `ModelRetry` is the only way to spend the per-tool retry budget; a tool that always returns successfully never spends it. The framework treats "successful call producing the same observable output as last time" as different from "failing call": only one of them counts.

## Failure Modes / Edge Cases

- **Successful repetition.** A model calling `get_weather` 20 times with the same arguments and the same answer (each with a new `tool_call_id`) passes every detector: `_check_max_retries` only fires on `ModelRetry` (`tool_manager.py:177-181`), `output_retries_used` only fires on validation failures (`_agent_graph.py:162-179`), `request_limit` only fires after 50 turns (`usage.py:382-383`). The user pays for 50 turns.
- **Alternating A → B → A → B tool pattern.** Not detected. There is no A/B window concept anywhere. `for_run_step` increments retries per-tool-name but only against a `max_retries` budget set on the failing tool itself (`tool_manager.py:120-145`).
- **Successful tool A on step 1, transient tool B failure on step 2, successful tool A again on step 3 — but `B` still has retries left.** The retry budget for `A` is unaffected; `B`'s counter is incremented. The framework does not correlate success/failure across tools.
- **Compaction (provider-side) erases evidence.** OpenAI Responses and Anthropic `CompactionPart` replaces past history with an opaque token (`messages.py:1682-1730`); the framework does not preserve a copy. Any future detector that wanted raw tool-call evidence loses it on those providers.
- **`ModelRetry` raised inside a hook during `wrap_model_request`.** Triggers `_build_retry_node` (`_agent_graph.py:829-835, 1027-1040`) and increments `output_retries_used`. The hook short-circuits the model call. The next retry-node prompts the model fresh — no record that this exact error happened earlier.
- **`end_strategy='exhaustive'` still runs every tool to its per-tool retry budget** (`agent/__init__.py:215-219`, `_agent_graph.py:1666-1674`). With `end_strategy='exhaustive'` and a model that emits a parallel batch of failing tool calls, each tool's `max_retries` is checked independently; the framework gives up only on the *first* exhausted tool's `UnexpectedModelBehavior` (or on output retry exhaustion).
- **`Tool(...)` registered with `max_retries=None` inherits agent default (`1`).** `_normalize_agent_retry_overrides(retries)` with `retries=None` and `default=1` (`agent/__init__.py:142-148`) means silent inheritance; tests like `test_unknown_tool_per_tool_retries_exceeded` (`tests/test_agent.py:4072-4080`) assert the default applies.
- **Resumed runs (after `_clean_message_history` merging consecutive `ModelRequest`s into one) do *not* lose per-step retry context.** `RunContext.retries` survives across steps via `ToolManager.for_run_step` (`tool_manager.py:120-145`). Resumed runs use `RunContext` only inside the running step; `_check_max_retries` is robust to message-history merging because it reads `RunContext.retries`, not the history itself.

## Future Considerations

- **A `consecutive_tool_errors` counter inside `RunContext` or `GraphAgentState`.** Symmetric to the `AgentToolUseTracker` shape from the OpenAI Agents SDK analysis; counts `ToolCallOutput` parts whose outcome indicates failure and resets on success. Would integrate via a new arm in `consume_output_retry` and a new `UnexpectedModelBehavior` variant (`_agent_graph.py:162-179` is the natural seam).
- **A `recent_tool_calls` sliding window.** Could live on `GraphAgentState` or be plumbed through `RunContext`. `RunContext.retries` (`_run_context.py:60-77`) is the current per-tool counter; an additional `recent: list[(name, args_hash)]` ring of length N would let a future `consume_recent_repetition` raise or warn when the last K tool calls are identical. No such hook exists today.
- **Promote `last_attempt` into a richer "warning" signal.** Today `RunContext.last_attempt` is a boolean (`_run_context.py:153-156`). A graduated `progress_warning` field with thresholds (`recent = 0.25, 0.5, 0.75, 1.0`) would let the framework inject escalating retry hints without a hard stop.
- **An explicit `doom_loop_detector` capability seam.** Today, before-model-request hooks (`ctx.deps.root_capability.before_model_request` in `_agent_graph.py:895-898`) can mutate the request but cannot stop the loop. A `before_model_request` hook returning e.g. `SkipModelRequest(...)` already exists (`exceptions.SkipModelRequest`, `exceptions.py:116-128`), so the *seam* is present — but no public hook gives "this looks like a doom loop" as a first-class signal.
- **Unify `request_limit` and `max_output_retries` into a single knob.** The two are conceptually similar (turn vs. retry-after-failure) but expose different error classes (`UsageLimitExceeded` vs. `UnexpectedModelBehavior`) and different defaults (`50` vs. `1`). A unified cap with sub-budgets would make "how long can an agent run?" a single configuration decision.
- **Tests for non-progress patterns.** No tests in `tests/` exercise a model that repeats the same successful call, an A→B alternation, or a "no actionable response for K steps" loop. Tests in `test_usage_limits.py` and `test_tools.py` exercise only the count caps.
- **Tracing for stuck runs.** There is no dedicated `stuck_loop` span/event in `capabilities/instrumentation.py` or `_instrumentation.py`. Counts (`usage.requests`, `output_retries_used`) are available on the state, but no telemetry signals them as anomalous.

## Questions / Gaps

- **Is there an undocumented detector?** A `grep -rn` for "loop", "stuck", "doom", "consecutive", "repeat", "no_progress", "replan", "halt" across `pydantic_ai_slim/` and `pydantic_graph/` returns no framework-side detector. The only matches in `capabilities/` are unrelated ("workload hooks", "lifecycle", "ordering"). If a detector exists, it is not publicly named.
- **How is `consume_output_retry` meant to interact with `check_incomplete_tool_call`?** `_agent_graph.py:177` calls `check_incomplete_tool_call` *before* raising `UnexpectedModelBehavior('Exceeded maximum output retries (N)')`. The intent appears to be "if this retry storm was caused by token truncation, surface the better diagnostic". Whether this is a documented contract or an incidental code path is unclear — there is no test pinning the precedence.
- **What does the doc claim?** `docs/agent.md:617-683` markets `UsageLimits(request_limit=K)` and `Agent(retries={'tools': N})` as "preventing infinite loops or excessive tool calling". The model of "loop prevention" presented to the user is exactly the count-cap mechanism. There is no claim that Pydantic AI detects patterns — the docs openly acknowledge that the LLM is expected to handle in-loop reasoning.
- **`pydantic_graph.GraphRun` has no step counter.** A user implementing a custom graph (not `Agent`) cannot bound iteration without writing their own loop outside the graph, because `GraphRun.next()` returns `End`/`BaseNode` with no max-steps limit (`pydantic_graph/pydantic_graph/graph.py:664-753`). The `Agent` layer adds `request_limit` but it is an LLM-call counter, not a node counter.
- **`failed_tools: set[str]` is not exposed.** It exists on `ToolManager` (`tool_manager.py:90-91`) but is only consumed by `for_run_step` (`tool_manager.py:120-145`). No public hook lets user code ask "what failed this step?" outside of `capability.before_tool_validate` / `on_tool_validate_error` calls. A `RecentToolFailures` capability seam would be the natural place for a future detector.

---

Generated by `03.06-stuck-doom-loop-detection.md` against `pydantic-ai`.
