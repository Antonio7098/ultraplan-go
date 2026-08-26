# Source Analysis: agent-framework

## 20.01 Token and Cost Accounting

### Source Info

| Field | Value |
|-------|-------|
| Name | agent-framework |
| Path | `studies/agent-harness-study/sources/agent-framework` |
| Language / Stack | Python + .NET (C#) monorepo; Go is a pointer to a separate repo (`go/README.md:1-5`) |
| Analyzed | 2026-08-26 |

## Summary

agent-framework implements **token accounting as a first-class, typed, end-to-end aggregated concern**, but deliberately leaves **monetary cost calculation out of the framework**. Both the Python and .NET runtimes define a normalized usage type (`UsageDetails`), parse provider-specific usage into it per model call, and sum it at every level where a logical run re-invokes an inner call: the function-calling loop, message-injection loops, tool-approval auto-approval loops, agent loops, and workflow executor merges. The final response object of every run carries the run-total token counts, and OpenTelemetry spans/metrics expose both per-call and run-aggregated token attributes under GenAI semconv.

Monetary cost appears only as opaque passthrough data: GitHub Copilot's `Cost` value is stored in `AdditionalCounts` after truncating a double to long (`dotnet/src/Microsoft.Agents.AI.GitHub.Copilot/GitHubCopilotAgent.cs:505-508`), and an MCP server's `_meta.cost` is dropped entirely when converting results (`python/packages/core/tests/core/test_mcp.py:1194-1213`). No pricing tables, rate calculators, or currency-formatted run summaries exist in either language — answering "what did this run cost?" in dollars requires an external observability/pricing layer; answering it in tokens takes one property read (`response.usage_details` / `response.Usage`).

## Rating

**7 / 10**

Rationale: Token accounting earns the 7–8 band on its own — explicit typed interfaces (`UsageDetails`, `add_usage_details`, `UsageAggregator`), aggregation at every loop boundary, null-aware and malformed-data-safe summation semantics, OTel metrics/traces aligned to semconv, and direct tests for aggregation including failure-shaped edge cases (non-int values skipped, `null` treated as "not reported" rather than zero, cap-exhaustion aggregation). It falls short of 8–9 because two dimensions under study are absent by design: no model-cost calculation in any currency (only lossy provider passthroughs — Copilot's double `Cost` truncated to `long`), no tool-execution cost tracking beyond call *counts* (a function-call budget, not dollars), and retry accounting is implicit (retries are user middleware whose repeated calls are only captured if each attempt returns a response that flows through aggregation).

## Evidence Collected

Every entry cites workspace-relative paths from `sources/agent-framework/`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Normalized usage type | `UsageDetails` TypedDict with `input_token_count`, `output_token_count`, `total_token_count`, `cache_creation_input_token_count`, `cache_read_input_token_count`, `reasoning_output_token_count`; open dict via `extra_items=int` for provider fields | `sources/agent-framework/python/packages/core/agent_framework/_types.py:406-427` |
| Usage combiner (Python) | `add_usage_details` sums all numeric keys across two dicts; skips non-int values with a warning instead of crashing | `sources/agent-framework/python/packages/core/agent_framework/_types.py:430-468` |
| Usage content item | `Content.from_usage(...)` factory creates `"usage"` content carrying details; `usage_details` field on unified `Content` | `sources/agent-framework/python/packages/core/agent_framework/_types.py:962-978`, `_types.py:498-499,561` |
| Streaming usage merge | When folding `ChatResponseUpdate`s into a `ChatResponse`, `case "usage":` sums into `response.usage_details` | `sources/agent-framework/python/packages/core/agent_framework/_types.py:1989-1993` |
| Response surfaces (Python) | `ChatResponse.usage_details` attribute; `AgentResponse.usage_details`; chat→agent conversion forwards usage | `sources/agent-framework/python/packages/core/agent_framework/_types.py:2280-2324`, `_types.py:2730`, `_types.py:2898` |
| Provider parsing (OpenAI) | `_parse_usage_from_openai` maps Responses-API `ResponseUsage` incl. cached/cache-write/reasoning tokens into `UsageDetails`; streaming emits usage content events | `sources/agent-framework/python/packages/openai/agent_framework_openai/_chat_client.py:3399-3417`, `_chat_client.py:3044-3047` |
| Provider parsing (Anthropic) | Cumulative stream snapshots converted to increments (`_incremental_usage`) so downstream sums stay correct; non-streaming parses `message.usage` | `sources/agent-framework/python/packages/anthropic/agent_framework_anthropic/_chat_client.py:1189-1192`, `_chat_client.py:1096` |
| Function-loop aggregation (Python) | Non-streaming loop accumulates `aggregated_usage = add_usage_details(aggregated_usage, response.usage_details)` after every model turn, including the final tools-disabled turn, and writes the total onto the returned response | `sources/agent-framework/python/packages/core/agent_framework/_tools.py:3198`, `3257`, `3326`, `3333` |
| Agent-loop aggregation (Python) | `AgentLoopMiddleware` sums `result.usage_details` per iteration and builds the combined `AgentResponse(usage_details=usage)` | `sources/agent-framework/python/packages/core/agent_framework/_harness/_loop.py:48`, `538-539`, `780-792` |
| Workflow-level merge (Python) | Workflow agent executors merge usage across parallel/concurrent branches with `add_usage_details` before emitting responses | `sources/agent-framework/python/packages/core/agent_framework/_workflows/_agent.py:509-545`, `855`, `889-890`, `913-914`, `936` |
| .NET usage aggregator | Internal `UsageAggregator.Combine/Accumulate`: null-aware per-counter sums plus per-key `AdditionalCounts` merge so "cached, reasoning, or cost counters aggregate correctly" | `sources/agent-framework/dotnet/src/Shared/Usage/UsageAggregator.cs:17-67`, `82-83`, `89-122` |
| .NET run summary surface | `AgentResponse.Usage` public property documented as resource usage for the response | `sources/agent-framework/dotnet/src/Microsoft.Agents.AI.Abstractions/AgentResponse.cs:210-217` |
| .NET loop aggregation | `LoopAgent` accumulates usage "across every inner invocation … rather than only its final iteration" and applies it to all three exit paths (approval-pending, cap reached, evaluator stop) | `sources/agent-framework/dotnet/src/Microsoft.Agents.AI/Harness/Loop/LoopAgent.cs:166-168`, `181`, `196-212` |
| .NET approval-loop aggregation | Auto-approval re-invocations accumulate usage; comment notes hitting the cap must not "discard every prior turn's cost" | `sources/agent-framework/dotnet/src/Microsoft.Agents.AI/Harness/ToolApproval/ToolApprovalAgent.cs:148-174` |
| .NET message-injection aggregation | `MessageInjectingChatClient` accumulates usage across injected-message iterations | `sources/agent-framework/dotnet/src/Microsoft.Agents.AI/ChatClient/MessageInjectingChatClient.cs:80-101` |
| .NET apply helper | `ApplyAggregatedUsage` swaps run-total usage onto the concluding response, mirroring `FunctionInvokingChatClient` semantics | `sources/agent-framework/dotnet/src/Shared/Usage/UsageAggregationExtensions.cs:25-41`, `60-69` |
| .NET workflow merge | `MessageMerger` combines `response.Usage` across merged/aggregated messages | `sources/agent-framework/dotnet/src/Microsoft.Agents.AI.Workflows/MessageMerger.cs:120-135`, `222` |
| OTel token metric (Python) | Histogram `gen_ai.client.token.usage` records input/output tokens with `gen_ai.token.type` dimension | `sources/agent-framework/python/packages/core/agent_framework/observability.py:275`, `1511-1517`, `3175-3178` |
| OTel attribute mapping | `USAGE_DETAIL_TO_OTEL_ATTR` maps canonical + provider-specific keys (`anthropic.cache_*`, `openai.cached_input_tokens`, `prompt/cached_tokens`, …) to semconv attrs | `sources/agent-framework/python/packages/core/agent_framework/observability.py:381-396` |
| Agent-span run totals | `INNER_ACCUMULATED_USAGE` ContextVar accumulates inner chat usage and stamps the summed tokens onto the `invoke_agent` span | `sources/agent-framework/python/packages/core/agent_framework/observability.py:128`, `3078-3103` |
| Tool-call budget (count, not cost) | `max_function_calls` described as the "primary knob for controlling cost"; persisted `budget_state["total_function_calls"]` / `attempt_count` survive phase transitions | `sources/agent-framework/python/packages/core/agent_framework/_tools.py:1342-1395`, `2757`, `3200`, `3240-3241` |
| Local token estimation | `TokenizerProtocol.count_tokens` + `CharacterEstimatorTokenizer` (4 chars/token) used by compaction — estimation for context management, not billing | `sources/agent-framework/python/packages/core/agent_framework/_compaction.py:52-57`, `76-80` |
| Cost passthrough (Copilot) | `AssistantUsageData.Cost` stored as `AdditionalCounts[nameof(Cost)] = (long)cost` (double→long truncation); duration also folded in as ms | `sources/agent-framework/dotnet/src/Microsoft.Agents.AI.GitHub.Copilot/GitHubCopilotAgent.cs:490-518` |
| MCP `_meta` cost dropped | Test fixture supplies `_meta={"executionTime": 1.5, "cost": {"usd": 0.002}, ...}`; assertion shows only text content survives conversion | `sources/agent-framework/python/packages/core/tests/core/test_mcp.py:1193-1213` |
| Run display helpers | Sample infra prints `[Usage] Tokens: {Total}, Input: {In}, Output: {Out}` from `response.Usage` and streamed `UsageContent` | `sources/agent-framework/dotnet/src/Shared/Samples/BaseSample.cs:94-113`, `186-191` |
| Tests: combiner semantics | `test_usage_details_addition`, `test_usage_details_add_skips_non_int`, `test_usage_details_iadd_edge_cases`, `test_content_add_usage_content*` | `sources/agent-framework/python/packages/core/tests/core/test_types.py:636-697`, `2039-2056`, `3285-3333` |
| Tests: run aggregation | Span test asserts invoke_agent span equals sum of both chat calls' input/output/cache/reasoning tokens; separate test covers max-iterations exhaustion aggregation | `sources/agent-framework/python/packages/core/tests/core/test_observability.py:5630-5702`, `5735-5740` |
| Tests: zero usage recorded | `test_capture_response_records_zero_token_usage` verifies zero-token responses still emit metrics | `sources/agent-framework/python/packages/core/tests/core/test_observability.py:4381` |
| Tests: .NET aggregator | `UsageAggregatorTests` cover null/null, one-sided nulls, additional-count merging, Accumulate sequences, and M.E.AI `Add` parity | `sources/agent-framework/dotnet/tests/Microsoft.Agents.AI.UnitTests/Shared/UsageAggregatorTests.cs:13-320` |

## Answers to Dimension Questions

1. **Are tokens counted per run?** Yes. Tokens are not locally estimated for billing; they are taken from provider-reported usage normalized into `UsageDetails` (Python: `python/packages/core/agent_framework/_types.py:406-427`; parsed from OpenAI at `python/packages/openai/agent_framework_openai/_chat_client.py:3399-3417` and Anthropic at `python/packages/anthropic/agent_framework_anthropic/_chat_client.py:1096`). Per-run totals are produced by summing every inner invocation's usage onto the concluding response (e.g., `python/packages/core/agent_framework/_tools.py:3333`; `dotnet/src/Microsoft.Agents.AI/Harness/Loop/LoopAgent.cs:181`). A local heuristic tokenizer exists but serves compaction/context-window math, not accounting (`python/packages/core/agent_framework/_compaction.py:76-80`).
2. **Are costs attributed per model call?** Token costs yes, monetary costs no. Each model call yields its own `ChatResponse.usage_details` and its own OTel chat span with token attributes (`python/packages/core/tests/core/test_observability.py:5688-5696` asserts per-span token values). No code path multiplies tokens by prices: searches for `cost|price|pricing|dollar` across `.py` and `.cs` sources return only comments, unrelated sample data, or the Copilot `Cost` passthrough (`dotnet/src/Microsoft.Agents.AI.GitHub.Copilot/GitHubCopilotAgent.cs:505-508`). The framework's stance is attribution-by-observation: it emits `model` + token attributes so an external backend can price them.
3. **Are tool execution costs tracked?** No monetary tracking. Tool consumption is bounded and counted, not priced: `max_function_calls` is explicitly framed as the "primary knob for controlling cost" (`python/packages/core/agent_framework/_tools.py:1342-1395`) with a persisted per-run budget (`_tools.py:2757,3200,3240-3241`); .NET bounds auto-approval chains because "a model that keeps requesting an auto-approved tool bills indefinitely" (`dotnet/src/Microsoft.Agents.AI/Harness/ToolApproval/ToolApprovalAgent.cs:143-146`). Server-reported tool economics are discarded — the MCP `_meta.cost` fixture demonstrates the metadata never reaches converted output (`python/packages/core/tests/core/test_mcp.py:1196,1210-1213`).
4. **Are retry costs accounted for?** Partially, by construction rather than by policy. There is no built-in model-retry mechanism in core (retry is shown as user-authored middleware in a docstring example, `python/packages/core/agent_framework/_middleware.py:552-565`; MCP RPC retries retry transport, not billing units, `_mcp.py:2096-2160`). What the framework does guarantee is that *any* repetition inside one logical run is reflected in the total: function-calling iterations (`_tools.py:3257`), approval-driven re-invocations (`dotnet/.../ToolApprovalAgent.cs:148-174`), injected-message loops (`MessageInjectingChatClient.cs:80-101`), and agent loops (`LoopAgent.cs:166-181`; Python `_harness/_loop.py:538-539`). A discarded-attempt pattern (retry middleware that drains then discards a stream) can issue telemetry/persistence for work the caller never sees, which the hooks layer handles for persistence gating (`_agent_hooks.py:1130-1136`) but there is no equivalent dedup or attempt-labeling for cost.
5. **Are per-run cost summaries available?** Yes for tokens, no for currency. The run-final `AgentResponse.usage_details` / `AgentResponse.Usage` carries whole-run totals (`python/packages/core/agent_framework/_types.py:2898`; `dotnet/src/Microsoft.Agents.AI.Abstractions/AgentResponse.cs:217`); workflows merge branch usage into emitted responses (`_workflows/_agent.py:889-914`; `dotnet/.../MessageMerger.cs:135,222`); the `invoke_agent` span reports summed tokens (`observability.py:3096-3103`, tested at `test_observability.py:5698-5702`); sample harnesses print run totals (`dotnet/src/Shared/Samples/BaseSample.cs:107-113`). The only dollar figure anywhere is the Copilot passthrough, truncated to integer (`GitHubCopilotAgent.cs:505-508`).

## Architectural Decisions

- **Normalize once, aggregate everywhere.** A single usage shape (`UsageDetails` in Python; M.E.AI's `UsageDetails` in .NET) is populated at the provider boundary and every higher layer composes with one combiner primitive (`add_usage_details`, `UsageAggregator.Combine/Accumulate`). This keeps loop authors honest: each new re-invocation site adds one line (`Accumulate(ref aggregatedUsage, response.Usage)`).
- **Run-total replaces last-call usage.** Aggregators intentionally overwrite the final response's usage with the run total (`UsageAggregationExtensions.cs:15-24`; Python `_tools.py:3333`), so callers reading `response.usage_details` get whole-run numbers without knowing how many internal calls occurred.
- **Open-ended counters instead of closed schemas.** `UsageDetails` accepts arbitrary int-valued provider keys (`extra_items=int`, `_types.py:406-420`) and .NET preserves `AdditionalCounts` per key (`UsageAggregator.cs:60-64`), letting unknown provider counters flow through aggregation untouched — this is exactly the channel Copilot uses to smuggle a cost figure.
- **Costs delegated to observability, not computed in-process.** GenAI semconv span attributes + the `gen_ai.client.token.usage` histogram (`observability.py:381-396`, `1511-1517`) push pricing responsibility to backends; the framework owns token truth.
- **Count-based budgets as the cost proxy.** Where money is impossible to compute locally, iteration/call caps (`max_iterations`, `max_function_calls`, `_maxAutoApprovalIterations`) bound spend exposure, with comments framing them explicitly as cost controls (`_tools.py:1342-1344`; `ToolApprovalAgent.cs:143-146`).

## Notable Patterns

- **Null-aware summation.** Both implementations treat "not reported" differently from zero: Python skips non-int values with a warning (`_types.py:462-467`); .NET's `AddCounts` returns the surviving side when one is null so aggregates don't fake zeros (`UsageAggregator.cs:82-83`), with parity against M.E.AI's own `Add` verified by test (`tests/Microsoft.Agents.AI.UnitTests/Shared/UsageAggregatorTests.cs:245-299`).
- **Incrementalization of cumulative streams.** Anthropic streams cumulative usage snapshots; the client converts them to deltas before emission so downstream additive merges stay correct (`anthropic/.../_chat_client.py:1189-1192`).
- **Streaming parity.** Usage arrives mid-stream as `"usage"` content items and is folded into the final response during update-to-response reduction (`_types.py:1989-1993`), so streaming and non-streaming runs report equal totals.
- **Telemetry-layer accumulation via ContextVar.** Inner chat spans record their usage; the agent span later reads an accumulated ContextVar to stamp run totals without re-walking responses (`observability.py:128,3087-3103`).
- **Sample/console surfacing.** Usage is displayed at every UX layer — console harness status bars (`python/samples/02-agents/harness/console/observers/usage_display.py:18-55`), sample output writers (`BaseSample.cs:94-113`), workflow runner token prints (`dotnet/src/Shared/Workflows/Execution/WorkflowRunner.cs:272-275`).

## Tradeoffs

- **No local pricing = no instant dollar answers.** Teams wanting "$ per run" must join token metrics with external price sheets; the framework provides model attribution (`RESPONSE_MODEL`, `observability.py:3147-3150`) but no calculator. This avoids stale price tables in-library, at the cost of pushing the last mile to users.
- **Lossy passthrough channels.** Folding `Cost` (double USD) into `AdditionalCounts<long>` truncates fractional cents (`GitHubCopilotAgent.cs:505-508`), and MCP `_meta.cost` is dropped outright (`test_mcp.py:1210-1213`) — provider-supplied economics survive only when they fit the int-counter mold.
- **Aggregation assumes additivity.** Cached-token semantics differ per provider (cache reads may be billed at discounts); summing raw counters preserves volume but not blended cost, again deferring pricing outward.
- **Retry visibility depends on response plumbing.** Because retries are middleware, an attempt whose exception never becomes a `ChatResponse` contributes nothing to totals; only successful-response repetitions are captured. The `_agent_hooks` persistence gate acknowledges such discarded attempts exist (`_agent_hooks.py:1130-1136`) but cost-wise they are invisible.

## Failure Modes / Edge Cases

- **Malformed provider usage cannot poison totals**: non-int entries are dropped with a warning rather than raising (`_types.py:462-467`), and mixed null/valued counters resolve to the reported value (`UsageAggregatorTests.cs:89-134`).
- **Cap exhaustion still accounts**: hitting `max_iterations` triggers a final tools-disabled call whose usage joins the aggregate (`_tools.py:3307-3333`), tested explicitly (`test_observability.py:5735-5740`); the .NET approval cap folds the capped turn in too (`ToolApprovalAgent.cs:157-169`).
- **Zero-usage runs remain observable**: zero-token captures are recorded rather than skipped (`test_observability.py:4381`).
- **Pending-approval exits preserve totals**: loop exits caused by approval requests return the aggregated-so-far usage, so paused runs report partial spend accurately (`LoopAgent.cs:196-199`; Python escape hatch at `_harness/_loop.py` mirrors this behavior).
- **Unpriced runaway risk**: without monetary tracking, the practical guard against runaway spend is the iteration/call caps; a host that disables caps has no framework-level spend ceiling.

## Future Considerations

- Add an optional pluggable cost-calculator seam (model→rate table) that could populate a currency-denominated sibling of `UsageDetails`, consuming the already-emitted `RESPONSE_MODEL` + token attributes.
- Preserve hosted-tool/server economics end-to-end (MCP `_meta.executionTime`/`cost`, Copilot sub-integer costs) instead of dropping or truncating them, e.g., as float-typed additional properties.
- Distinguish attempt-level usage for retry/fallback middleware (per-attempt labels or dedup) so discarded attempts are visible in cost accounting.
- Expose a first-class per-run summary object (tokens + wall time + call counts) distinct from the response payload, since `budget_state["total_function_calls"]`/`attempt_count` already track the count half internally but aren't surfaced publicly (`_tools.py:2757,3240-3241`).

## Questions / Gaps

- No evidence found of any pricing table, currency formatting, or cost estimator anywhere in the repo (searched `(?i)(cost|price|pricing|dollar)` over `*.py` and `*.cs` in `python/packages/` and `dotnet/src/`; all hits were incidental comments, unrelated sample data, or the Copilot/MCP passthroughs cited above).
- No evidence found of tool-execution monetary accounting; the closest artifacts are call-count budgets (`max_function_calls`) and dropped server-side cost metadata (`test_mcp.py:1193-1213`).
- No evidence found of a dedicated retry-cost ledger; retry behavior is delegated to user middleware (`_middleware.py:552-565`) and only successful responses feed aggregation.
- The Go implementation was not analyzed: the vendored `go/` directory contains only a pointer README (`go/README.md:1-5`) directing to a separate repository, which is outside this source's directory.
- .NET token *metric* emission (histogram) lives in the external Microsoft.Extensions.AI/OpenAI Telemetry stack rather than in this repo; within this source, .NET telemetry is limited to span metadata ("token counts" mentioned in `dotnet/src/Microsoft.Agents.AI/OpenTelemetryAgent.cs:26,128`), so the histogram evidence above is Python-only.

---

Generated by `dimensions/20.01-token-cost-accounting.md` against `agent-framework`.
