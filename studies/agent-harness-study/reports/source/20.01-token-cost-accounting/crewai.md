# Source Analysis: crewai

## Dimension 20.01 — Token and Cost Accounting

### Source Info

| Field | Value |
|-------|-------|
| Name | crewai |
| Path | `studies/agent-harness-study/sources/crewai` |
| Language / Stack | Python (pydantic models, LiteLLM fallback + native provider SDKs: OpenAI, Anthropic, Azure, Bedrock, Gemini) |
| Analyzed | 2026-08-26 |

## Summary

CrewAI has a mature, multi-layer token accounting stack but **no monetary cost calculation at all**. Token usage is captured at the LLM-call boundary in three coordinated ways:

1. **Per-LLM-instance lifetime counters** — every native provider and the LiteLLM fallback funnel usage through a shared normalizer (`UsageMetrics.from_provider_dict`) into a private `_token_usage` dict on the LLM instance (`lib/crewai/src/crewai/llms/base_llm.py:955-986`).
2. **Event-bus aggregation** — every completed LLM call emits an `LLMCallCompletedEvent` carrying a raw `usage` dict (`lib/crewai/src/crewai/events/types/llm_events.py:90-99`), which Flows aggregate into a thread-safe per-kickoff accumulator (`lib/crewai/src/crewai/flow/runtime/__init__.py:879-942`).
3. **Legacy callback accumulation** — a `TokenCalcHandler` → `TokenProcess` path for non-`BaseLLM` agents and streaming callbacks (`lib/crewai/src/crewai/utilities/token_counter_callback.py:16-66`, `lib/crewai/src/crewai/agents/agent_builder/utilities/base_token_process.py:8-38`).

Run-level summaries are exposed on outputs (`CrewOutput.token_usage`, `LiteAgentOutput.usage_metrics`) and computed by rolling up each agent's LLM summary (`Crew.calculate_usage_metrics`, `lib/crewai/src/crewai/crew.py:2201-2225`). Per-call (vs lifetime) attribution is handled by snapshot/delta semantics (`UsageMetrics.delta_since`, applied at agent kickoff boundaries). The single field that hints at cost awareness — `LLM.completion_cost` (`lib/crewai/src/crewai/llm.py:370`) — is declared but never read or written anywhere else in the library.

**Answering "what did this run cost?"**: tokens yes (per run, per call, per flow), dollars no. A user must apply their own pricing to `usage_metrics`.

## Rating

Rating: 7/10

Clear model with explicit interfaces (`UsageMetrics`, `get_token_usage_summary`, `delta_since`, `from_provider_dict`), extensive test coverage across providers, flows, streaming, and hierarchical crews, plus operational safeguards (Anthropic cache reconciliation, negative-delta clamping, thread-safe aggregation with orphaned-handler protection). It stops short of 9–10 because monetary cost is entirely absent (a declared-but-dead `completion_cost` field), tool execution cost is untracked beyond timing, failed/retried-at-provider-level calls are invisible, and there is a documented sibling-flow over-counting hazard.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Canonical metrics model | `UsageMetrics` pydantic model: `total_tokens`, `prompt_tokens`, `cached_prompt_tokens`, `completion_tokens`, `reasoning_tokens`, `cache_creation_tokens`, `successful_requests`; `add_usage_metrics` accumulator | lib/crewai/src/crewai/types/usage_metrics.py:32-77 |
| Per-call delta semantics | `delta_since()` returns clamped field-wise differences so a reset accumulator can't produce negative usage | lib/crewai/src/crewai/types/usage_metrics.py:79-109 |
| Provider dict normalization | `from_provider_dict` maps OpenAI/Gemini/Anthropic key aliases; reconciles raw Anthropic cache keys so cached workloads don't undercount billed prompt tokens | lib/crewai/src/crewai/types/usage_metrics.py:111-189 |
| Central tracking hook | `BaseLLM._track_token_usage_internal` adds normalized metrics into instance-lifetime `_token_usage`; `get_token_usage_summary` exposes them | lib/crewai/src/crewai/llms/base_llm.py:244, 955-986 |
| Provider call sites (OpenAI) | ~12 `_track_token_usage_internal(usage)` invocations across chat/responses/streaming paths | lib/crewai/src/crewai/llms/providers/openai/completion.py:981,1128,1303,1440,1906,1931,2081,2193,2334,2359,2526 |
| Provider call sites (Anthropic) | usage extracted then tracked on sync/stream/follow-up paths | lib/crewai/src/crewai/llms/providers/anthropic/completion.py:1007-1008,1241-1242,1446-1447,1963 |
| Provider call sites (Bedrock override) | Bedrock overrides `_track_token_usage_internal` for its typed usage shape, increments `successful_requests` directly | lib/crewai/src/crewai/llms/providers/bedrock/completion.py:2067-2083 |
| Usage carried on events | `LLMCallCompletedEvent.usage: dict[str, Any] \| None` emitted via event bus from both LLM backends | lib/crewai/src/crewai/events/types/llm_events.py:90-99; lib/crewai/src/crewai/llm.py:2150-2189; lib/crewai/src/crewai/llms/base_llm.py:615-643 |
| Flow-level aggregation | Flow attaches an `LLMCallCompletedEvent` listener; accumulates under lock keyed by `current_flow_id` contextvar; `flow.usage_metrics` returns a defensive copy | lib/crewai/src/crewai/flow/runtime/__init__.py:757-758, 879-911, 913-942 |
| Legacy callback path | `TokenCalcHandler.log_success_event` sums prompt/completion/cached tokens into `TokenProcess` (used for non-BaseLLM agents and litellm streaming fallback) | lib/crewai/src/crewai/utilities/token_counter_callback.py:37-66; lib/crewai/src/crewai/agent/core.py:1162,1505,1522; lib/crewai/src/crewai/llm.py:1177-1218 |
| Crew run summary | `Crew.calculate_usage_metrics` rolls up all agents' LLM summaries (and manager agent); assigned to `self.usage_metrics` after kickoff | lib/crewai/src/crewai/crew.py:1065, 1930, 2201-2225 |
| Output exposure (crew) | `CrewOutput.token_usage: UsageMetrics` with alias property `usage_metrics` returning a dict | lib/crewai/src/crewai/crews/crew_output.py:27-57 |
| Output exposure (agent) | `LiteAgentOutput.usage_metrics` documents "kickoff call only (guardrail retries included)"; `token_usage` property validates to `UsageMetrics` | lib/crewai/src/crewai/lite_agent_output.py:42-50, 108-119 |
| Per-call scoping at kickoff | baseline snapshot taken before execution; output reports `delta_since(baseline)` instead of lifetime totals | lib/crewai/src/crewai/agent/core.py:1655, 1755-1765, 1783-1785, 1830-1832 |
| Multi-crew roll-up | concurrent crew copies' `usage_metrics` summed into one total | lib/crewai/src/crewai/crews/utils.py:442, 507-511 |
| Trace export of usage | tracing listener registers on `LLMCallCompletedEvent` and serializes full event (including `usage`) into trace events | lib/crewai/src/crewai/events/listeners/tracing/trace_listener.py:400-401, 1010-1011 |
| Cost field (dead) | `completion_cost: float \| None = None` declared on LiteLLM-backed `LLM`; zero reads/writes elsewhere in the repo | lib/crewai/src/crewai/llm.py:370 |
| Tests: crew kickoff | asserts `total_tokens/prompt_tokens/completion_tokens/successful_requests > 0` incl. streaming and hierarchical manager roll-up | lib/crewai/tests/test_crew.py:929-994, 1147-1166, 1789-1874 |
| Tests: flow aggregation | verifies event-driven aggregation, independent copies, pause/resume behavior, pre-pause snapshotting | lib/crewai/tests/test_flow_usage_metrics.py:228-267, 314-352, 389-416, 464-467 |
| Tests: provider normalization | Anthropic cache-read/cache-write reconciliation cases asserting billed-token math | lib/crewai/tests/events/test_llm_usage_event.py:254-300, 303-359 |

## Answers to Dimension Questions

1. **Are tokens counted per run?** Yes. Three scopes exist: (a) per-kickoff agent runs use baseline/delta snapshots (`lib/crewai/src/crewai/agent/core.py:1655,1830-1832`); (b) Flows aggregate all LLM events during one kickoff into `_aggregated_usage_metrics` reset per run (`lib/crewai/src/crewai/flow/runtime/__init__.py:2172, 879-911`); (c) Crews sum agent-level summaries post-run (`lib/crewai/src/crewai/crew.py:2201-2225`). Counters include reasoning and Anthropic cache-creation tokens, not just prompt/completion (`lib/crewai/src/crewai/types/usage_metrics.py:53-60`).

2. **Are costs attributed per model call?** Tokens are attributed per call (each completed call emits `usage` on `LLMCallCompletedEvent`, `lib/crewai/src/crewai/events/types/llm_events.py:97`, and increments the calling LLM instance's counters, `lib/crewai/src/crewai/llms/base_llm.py:965-971`). **Monetary cost is not calculated anywhere.** The only cost-shaped symbol, `LLM.completion_cost` (`lib/crewai/src/crewai/llm.py:370`), is never populated or consumed — searching the whole source tree for pricing/cost computation returns nothing else of substance.

3. **Are tool execution costs tracked?** No. Tool events carry timing (`started_at`/`finished_at`) and counters like `run_attempts`, but no token or currency fields (`lib/crewai/src/crewai/events/types/tool_usage_events.py:63-82`). Indirect cost is inferable because each LLM round-trip that drives a tool is separately metered, but there is no tool→token attribution join shipped in the core (the tracing layer serializes both LLM and tool events into one trace stream, enabling external correlation: `lib/crewai/src/crewai/events/listeners/tracing/trace_listener.py:394-418`).

4. **Are retry costs accounted for?** Partially, by construction rather than design. Guardrail retries re-enter the same kickoff loop *inside* the baseline/delta window, so their LLM calls are explicitly included in the reported usage ("guardrail retries included", `lib/crewai/src/crewai/lite_agent_output.py:42-50`). Follow-up/continuation calls within a provider turn (e.g., Anthropic tool-use follow-ups) are tracked too (`lib/crewai/src/crewai/llms/providers/anthropic/completion.py:1446-1447,1871-1872`). However, retries swallowed inside provider SDKs or LiteLLM that fail before yielding a response emit only `LLMCallFailedEvent` with **no usage data** (`lib/crewai/src/crewai/events/types/llm_events.py:117-121`), so burned tokens on failed attempts are silently lost. There is no retry-cost line item.

5. **Are per-run cost summaries available?** Yes for tokens: `result.token_usage` / `result.usage_metrics` on both output types (`lib/crewai/src/crewai/crews/crew_output.py:27-57`, `lib/crewai/src/crewai/lite_agent_output.py:108-119`), `crew.usage_metrics` (`lib/crewai/src/crewai/crew.py:271`), and `flow.usage_metrics` (`lib/crewai/src/crewai/flow/runtime/__init__.py:913-942`). No dollar figure is attached anywhere.

## Architectural Decisions

- **Normalize once, share everywhere.** A single normalizer (`UsageMetrics.from_provider_dict`) backs per-LLM counters, flow aggregation, and OTel trace payloads, with docstrings explicitly promising they agree on every provider (`lib/crewai/src/crewai/types/usage_metrics.py:142-155`; `lib/crewai/src/crewai/flow/runtime/__init__.py:265-272`). This eliminates drift between the three accounting layers.
- **Lifetime accumulator + delta views.** Rather than tracking per-call records, each LLM keeps monotonically increasing counters and callers derive per-call usage via `delta_since` snapshots (`lib/crewai/src/crewai/llms/base_llm.py:973-985`; `lib/crewai/src/crewai/types/usage_metrics.py:79-109`). Cheap, but means history granularity is bounded by who remembers to snapshot.
- **Events as the aggregation backbone.** Flow-level totals come from bus listeners correlated by a contextvar (`current_flow_id`), not from walking object graphs — allowing flows to measure LLM calls made outside crew machinery (`lib/crewai/src/crewai/flow/runtime/__init__.py:879-904`).
- **Dual-path compatibility.** The legacy `TokenProcess`/`TokenCalcHandler` path is retained for non-`BaseLLM` agents and third-party LLM wrappers, and `calculate_usage_metrics` falls back to it when the agent's llm isn't a `BaseLLM` (`lib/crewai/src/crewai/crew.py:2210-2217`).
- **Billed-token fidelity over naive sums.** Anthropic cache read/creation tokens are folded into `prompt_tokens`/`total_tokens` when raw provider shapes arrive unreconciled, with the rationale documented as preventing undercount of billed usage (`lib/crewai/src/crewai/types/usage_metrics.py:111-136`).

## Notable Patterns

- **Defensive coercion everywhere**: `_coerce_int` turns None/garbage into 0 (`lib/crewai/src/crewai/types/usage_metrics.py:13-19`); event validators downgrade non-string finish reasons to None rather than crashing (`lib/crewai/src/crewai/events/types/llm_events.py:101-114`).
- **Concurrency safeguards**: flow aggregation uses a `threading.Lock` and captures the accumulator in the listener closure so a stale handler queued from a prior kickoff can't pollute the next run's totals (`lib/crewai/src/crewai/flow/runtime/__init__.py:886-892`); `usage_metrics` returns `model_copy()` to prevent external mutation (`:941-942`).
- **Faithful-metrics principle**: calls that complete without visible token data are excluded from `successful_requests` rather than guessed, documented at `lib/crewai/src/crewai/flow/runtime/__init__.py:928-932`.
- **Test-as-spec for money-adjacent math**: dedicated test classes pin Anthropic cache arithmetic (`prompt = input + cache_read + cache_creation`) including mixed read/write cases (`lib/crewai/tests/events/test_llm_usage_event.py:303-359`).

## Tradeoffs

- **No cost model**: users get tokens but must bring their own price table; nothing in-repo consumes `litellm.completion_cost` even though the dependency exists — the dead `completion_cost` field (`lib/crewai/src/crewai/llm.py:370`) suggests planned-but-unrealized support.
- **Snapshot-based per-call scoping is opt-in**: any code path that forgets to capture a baseline silently reports lifetime totals (mitigated in current kickoffs, but fragile for new entry points).
- **Contextvar correlation leaks across parallel siblings**: nested child flows roll up into parents by design, but parallel siblings sharing a parent contextvar may over-count each other — acknowledged in the API docs itself (`lib/crewai/src/crewai/flow/runtime/__init__.py:918-926`).
- **Cross-process pause/resume loses history**: restored flow instances start aggregation at zero because pre-pause totals aren't persisted (`lib/crewai/src/crewai/flow/runtime/__init__.py:934-939`).

## Failure Modes / Edge Cases

- **Failed LLM calls burn unbilled-to-user tokens**: `LLMCallFailedEvent` carries error text but no usage (`lib/crewai/src/crewai/events/types/llm_events.py:117-121`); provider-level retries that fail are invisible to all summaries.
- **Structured-output/Instructor paths may skip counting**: flow docs state such calls contribute neither tokens nor request counts (`lib/crewai/src/crewai/flow/runtime/__init__.py:928-932`) — a real blind spot for conversion-heavy crews.
- **Negative-delta clamp hides resets**: if an LLM instance's counters are reset mid-run, `delta_since` clamps to zero (`lib/crewai/src/crewai/types/usage_metrics.py:93-108`), producing undercounts rather than errors.
- **Shared LLM instances double-count at crew level**: counters are per-LLM-instance and grow across calls from different agents (`lib/crewai/src/crewai/llms/base_llm.py:976-981`); two crew members sharing one `LLM` will have their joint usage added twice when `calculate_usage_metrics` iterates agents (`lib/crewai/src/crewai/crew.py:2205-2209`) unless per-kickoff deltas are used.
- **Streaming extraction is best-effort**: usage scraped off the last chunk inside broad try/except degrades silently to debug logs (`lib/crewai/src/crewai/llm.py:1193-1210`).

## Future Considerations

- Wire actual price tables (e.g., consume `litellm.completion_cost` where the stub already exists) and add a `cost_usd` field to `UsageMetrics` so run summaries answer the cost question natively.
- Persist usage alongside flow checkpoints so cross-process resume preserves totals (`lib/crewai/src/crewai/flow/runtime/__init__.py:934-939` names this gap).
- Add per-task/per-tool token attribution by joining `current_task_id` contextvar correlation (`lib/crewai/src/crewai/task.py:657`) onto `LLMCallCompletedEvent` consumers.
- Surface failed-call estimates (at minimum attempt counts) on `LLMCallFailedEvent` for honest retry-cost visibility.

## Questions / Gaps

- **Is there any cost/budget enforcement (max spend cutoffs)?** No evidence found. Searches for `budget`, `pricing`, `cost` across `lib/crewai/src` surfaced only unrelated hits (memory exploration budget at `lib/crewai/src/crewai/memory/recall_flow.py:55`; Anthropic thinking `budget_tokens` at `lib/crewai/src/crewai/llms/providers/anthropic/completion.py:192`). There is no spend guardrail mechanism.
- **Was `completion_cost` ever functional?** Undetermined. It appears solely as a declaration (`lib/crewai/src/crewai/llm.py:370`); git archaeology is outside this study's isolation boundary.
- **Do third-party/non-BaseLLM integrations report reliable usage?** The `TokenCalcHandler` path depends on response objects exposing `.prompt_tokens`/`.completion_tokens` attributes (`lib/crewai/src/crewai/utilities/token_counter_callback.py:53-58`); behavior with exotic wrappers was not verified against tests here.

---

Generated by `Dimension 20.01: Token and Cost Accounting` against `crewai`.
