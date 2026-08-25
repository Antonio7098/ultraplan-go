# Source Analysis: letta

## Dimension 13.04: Recovery vs Escalation

### Source Info

| Field | Value |
|-------|-------|
| Name | letta |
| Path | `studies/agent-harness-study/sources/letta` |
| Language / Stack | Python (FastAPI server, SQLAlchemy async ORM, Pydantic v2, OpenTelemetry) |
| Analyzed | 2026-08-25 |

> Citation convention: all file paths below are relative to the source root `studies/agent-harness-study/sources/letta/` (e.g., `letta/llm_api/llm_api_tools.py:38` resolves to `studies/agent-harness-study/sources/letta/letta/llm_api/llm_api_tools.py:38`). No files outside this source directory were accessed.

## Summary

Letta implements recovery as a layered stack rather than a single policy. At the bottom, provider SDK-level retries are configurable per provider (`anthropic_max_retries`, `gemini_max_retries` in `letta/settings.py:181,218`). Above that, an application-level exponential-backoff wrapper with OTEL audit events retries HTTP 429s (`letta/llm_api/llm_api_tools.py:38-118`). The dominant recovery mechanism is context-window overflow handling: every agent-loop generation (v1/v2/v3) catches `ContextWindowExceededError`, runs compaction/summarization, and retries the LLM request up to a configurable limit (`summarizer_settings.max_summarizer_retries`, default 3, env-tunable via `LETTA_SUMMARIZER_` prefix, `letta/settings.py:74-96`). The summarizer itself has cascading fallbacks (clamped tool returns → hard transcript truncation, `letta/services/summarizer/summarizer.py:564-619`), and the v3 loop can fail over to a fallback model via an LLM routing client with circuit-breaker signals (`letta/agents/letta_agent_v3.py:1184-1211`, `letta/services/llm_router/llm_router_client_base.py:42-93`).

Escalation to humans exists in exactly one first-class form: the human-in-the-loop approval flow. Tools flagged `requires_approval` (or client-side tools) cause the agent to persist an approval-request message and stop with `StopReasonType.requires_approval` (`letta/agents/letta_agent_v3.py:1681-1709`), which maps to `RunStatus.completed` — escalation is modeled as a graceful pause, not a failure (`letta/schemas/letta_stop_reason.py:24-32`). Approval policy is user-configurable per tool via REST (`PATCH /v1/agents/{agent_id}/tools/approval/{tool_name}`, `letta/server/rest_api/routers/v1/agents.py:707-740`). All other failures terminate runs with a typed stop reason and persisted error metadata; there is no automatic re-drive of failed runs and no generic "escalation" framework — the word "escalat*" appears nowhere in the codebase.

Auditability is strong: retry attempts, retry exhaustion, non-retryable errors, and run terminal states are all recorded as OTEL events, structured logs, run metadata (`metadata={"error": ...}`), agent `last_stop_reason`, and callback dispatch results.

## Rating

**8 / 10** — Clear model with tests, explicit interfaces, and operational safeguards.

Rationale:
- Explicit taxonomy of 12 stop reasons mapped deterministically to run statuses (`letta/schemas/letta_stop_reason.py:9-49`).
- Layered, test-covered recovery: embedding batch-splitting retries (`tests/test_embeddings.py:103`, `tests/test_file_processor.py:68-98`), provider-error mapping regression tests (`tests/adapters/test_letta_llm_stream_adapter_error_handling.py:72-251`), approval idempotency/retry-after-summarization (`tests/integration_test_human_in_the_loop.py:1418-1469`), cancellation during LLM errors (`tests/managers/test_cancellation.py:1475-1503`), and a watchdog validation script (`test_watchdog_hang.py` at repo root).
- Configurable thresholds for summarizer retries, provider SDK retries, and approval requirements.
- Deductions: recovery logic is triplicated across three agent-loop generations with drift risk; the legacy backoff wrapper has dead code and only handles sync `requests` errors (`letta/llm_api/llm_api_tools.py:50-51,64`); webhook/callback notification failures are swallowed silently (`letta/services/webhook_service.py:48-55`, `letta/services/run_manager.py:504-508`); no unified or documented escalation policy beyond approvals.

## Evidence Collected

Every entry cites paths relative to the source root `studies/agent-harness-study/sources/letta/`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Retry/backoff engine | `retry_with_exponential_backoff`: max_retries=20, jittered exponential delay on HTTP 429, raises `RateLimitExceededError("Maximum number of retries exceeded")` when exhausted | `letta/llm_api/llm_api_tools.py:38-118` (exhaustion at :84-94) |
| Retry audit events | OTEL events `llm_retry_attempt`, `llm_max_retries_exceeded`, `llm_non_retryable_error`, `llm_unexpected_error` | `letta/llm_api/llm_api_tools.py:72-81,85-93,107-110,114-116` |
| Provider SDK retries | `anthropic_max_retries: int = 3` and `gemini_max_retries: int = 5` settings injected into Anthropic/Bedrock/Gemini clients | `letta/settings.py:181,218`; `letta/llm_api/anthropic_client.py:440-497`; `letta/llm_api/bedrock_client.py:66,74`; `letta/llm_api/google_vertex_client.py:55` |
| Context-overflow recovery (v2 loop) | Catch `ContextWindowExceededError` → summarize → retry within `range(summarizer_settings.max_summarizer_retries + 1)`; `LLMError` re-raised immediately as non-retryable | `letta/agents/letta_agent_v2.py:518,556-573` |
| Context-overflow recovery (v3 loop) | Compaction + system-prompt rebuild + retry, emits compaction event message before compacting | `letta/agents/letta_agent_v3.py:1093,1217-1269` |
| Context-overflow recovery (v1 loop) | Recursive `inner_step` retry after `summarize_messages_inplace()`; fatal `ContextWindowExceededError` with token-count details after budget exhausted | `letta/agent.py:1026-1067` |
| Summarizer fallbacks | Fallback A: clamp tool returns; Fallback B: middle-truncate transcript to byte budget (~60% of context window) | `letta/services/summarizer/summarizer.py:564-619` |
| Retry threshold config | `SummarizerSettings` pydantic-settings with `env_prefix="letta_summarizer_"`; `max_summarizer_retries = 3`; doc comment says exceeding it "throws a fatal error" | `letta/settings.py:74-96` |
| Provider error classification | `handle_llm_error` maps timeouts, connection drops, 429, 400/`context_length_exceeded`, 402/credits, 401, 403, 404, 413→ContextWindowExceeded, 5xx→LLMServerError into typed hierarchy | `letta/llm_api/openai_client.py:1216-1420`; hierarchy at `letta/errors.py:259-349` |
| Overflow heuristics | Centralized string matching for context-window and insufficient-credits messages across providers | `letta/llm_api/error_utils.py:8-23,26-41` |
| Model failover routing | On LLM failure with a fallback route: record success/failure (circuit breaker), switch `active_llm_config` to fallback handle and continue retrying | `letta/agents/letta_agent_v3.py:1171-1211`; interface `letta/services/llm_router/llm_router_client_base.py:42-93` |
| Gemini malformed-call retry | `FinishReason.MALFORMED_FUNCTION_CALL` triggers bounded retry (`MAX_RETRIES`) that injects a corrective warning message into the prompt | `letta/llm_api/google_vertex_client.py:133-198` |
| Tool-failure self-correction | Tool exceptions are converted to `ToolExecutionResult(status="error", func_return=friendly_msg, stderr=traceback)` fed back to the model instead of crashing the step | `letta/services/tool_executor/tool_execution_manager.py:101,131-155` |
| Embedding adaptive retry | Token-limit errors reduce batch size or split individual texts and retry | `letta/llm_api/openai_client.py:1085-1190` |
| DB transient-error retry | 3 attempts with doubling delay for connection errors; friendly `LettaServiceUnavailableError` surfaced after exhaustion | `letta/server/db.py:82-115` |
| DB deadlock retry | Deadlock detection and retry in ORM create/update/delete paths (`_DEADLOCK_MAX_RETRIES`) | `letta/orm/sqlalchemy_base.py:606,666,699,738,784` |
| Stop-reason → status mapping | 12 `StopReasonType` values; `requires_approval`/`end_turn`/`max_steps`/`tool_rule` → completed; error-family → failed; cancelled → cancelled | `letta/schemas/letta_stop_reason.py:9-49` |
| Human escalation trigger | Requested tool calls requiring approval (or client-side tools) produce approval-request messages; loop stops with `requires_approval` | `letta/agents/letta_agent_v3.py:1681-1709` |
| Approval rule types | `ToolRuleType.requires_approval`; `RequiresApprovalToolRule` schema; solver `is_requires_approval_tool`/`get_requires_approval_tools` | `letta/schemas/enums.py:197`; `letta/schemas/tool_rule.py:353`; `letta/helpers/tool_rule_solver.py:48-196` |
| Approval configurability | Per-tool `default_requires_approval` persisted on tool rows; REST endpoint to toggle per agent+tool | `letta/orm/tool.py:55`; `letta/schemas/tool.py:59,127,203`; `letta/server/rest_api/routers/v1/agents.py:707-740`; `letta/services/agent_manager.py:3064-3074` |
| Human response channel | `ApprovalCreate` input (approve/deny + reason) converted to role=`approval` messages; denial becomes an error-status tool return visible to the model | `letta/schemas/message.py:179,307`; `letta/server/rest_api/utils.py:213-227,230-264` |
| Cancellation auto-denial | Cancelling a run while pending approval denies ALL pending tool calls with `TOOL_CALL_DENIAL_ON_CANCEL`; cancellation is idempotent for terminal runs | `letta/services/run_manager.py:649-739` |
| Graceful credit stop | Pre-flight/per-step credit verification stops the loop with `insufficient_credits` stop reason; maps to 402 HTTP handler | `letta/agents/letta_agent_v2.py:240,380,738-747`; `letta/schemas/letta_stop_reason.py:46-47`; `letta/server/rest_api/app.py:725-736`; `letta/services/credit_verification_service.py` |
| Run failure persistence | Failed runs updated with `RunUpdate(status=RunStatus.failed, stop_reason=..., metadata={"error": ...})` in background streams and routers | `letta/server/rest_api/redis_stream_manager.py:396-402`; `letta/server/rest_api/routers/v1/agents.py:2144,2155,2306` |
| Run lifecycle guardrails | Invalid terminal transitions logged as errors (e.g., updating a completed run); missing stop_reason on completion logged | `letta/services/run_manager.py:341-364` |
| Agent last-stop-reason audit | Terminal updates write agent `last_stop_reason`; listable filter documented for ops queries | `letta/services/run_manager.py:398-410`; `letta/services/helpers/agent_manager_helper.py:760` |
| Callback/notification dispatch | Terminal runs POST to `run.callback_url` (5s timeout); result status code/error stored on the run row; failures "continue silently" | `letta/services/run_manager.py:450-510` |
| Step-completion webhook | Optional `STEP_COMPLETE_WEBHOOK` env webhook; timeout/HTTP errors logged and swallowed (returns False) | `letta/services/webhook_service.py:13-56` |
| Stream terminal-event synthesis | Background stream without `[DONE]`/error synthesizes terminal; task cancellation distinguished from explicit run cancellation; error path marks run failed | `letta/server/rest_api/redis_stream_manager.py:283-315,317-377,396-402` |
| Mid-stream SSE error contract | Errors after first chunk yield `event: error` + typed `LettaErrorMessage` and suppress finish chunks; pre-first-chunk errors raise for proper HTTP status | `letta/agents/letta_agent_v3.py:649-692,718-736` |
| Infra watchdog | Thread-based event-loop hang detection with heartbeat lag metrics, readiness degradation/recovery gates, critical log + stack dump on freeze | `letta/monitoring/event_loop_watchdog.py:26-206,279+`; `letta/monitoring/readiness_state.py:63-84`; validation script `test_watchdog_hang.py:29-97` |
| Bounded loop termination | All loops iterate `for i in range(max_steps)` (default `DEFAULT_MAX_STEPS`) and set `max_steps` stop reason — no unbounded autonomous retry | `letta/agents/letta_agent_v2.py:233-234,408-409`; `letta/agents/letta_agent_v3.py:328,394-395` |
| Tests: recovery behaviors | Embedding retry/batch-split tests; 413→ContextWindowExceeded mapping regression; approval retry-after-summarization idempotency; LLMError mid-stream cancellation test | `tests/test_embeddings.py:103-201`; `tests/test_file_processor.py:68-188`; `tests/adapters/test_letta_llm_stream_adapter_error_handling.py:72-251`; `tests/integration_test_human_in_the_loop.py:1418-1568`; `tests/managers/test_cancellation.py:1475-1503` |

## Answers to Dimension Questions

### 1. When does the system retry vs escalate?

Retry decisions are keyed by exception type, not count alone:

- **Retry automatically**: HTTP 429 rate limits (up to 20 attempts with jittered exponential backoff, `letta/llm_api/llm_api_tools.py:38-104`); context-window overflow (compaction + retry up to `max_summarizer_retries`, `letta/agents/letta_agent_v3.py:1217-1269`); DB connection errors and deadlocks (`letta/server/db.py:82-109`, `letta/orm/sqlalchemy_base.py:606`); embedding token-limit errors (batch splitting, `letta/llm_api/openai_client.py:1160-1190`); Gemini malformed function calls (`letta/llm_api/google_vertex_client.py:133-198`); provider-declared fallback models exist for the current handle (`letta/agents/letta_agent_v3.py:1184-1211`).
- **Fail fast (no retry)**: `LLMError` subclasses other than overflow — auth failures, bad requests, permission denied, timeouts — are re-raised immediately with `stop_reason=llm_api_error` (`letta/agents/letta_agent_v2.py:559-561`, `letta/agents/letta_agent_v3.py:1214-1216`); invalid LLM responses raise with `invalid_llm_response` (`letta/agents/letta_agent_v2.py:556-558`).
- **Pause and wait for human**: tool calls governed by `requires_approval` rules or client-side tools (`letta/agents/letta_agent_v3.py:1681-1709`). This is the only true human-escalation path.
- **Stop permanently**: exhausted summarizer retries → fatal `ContextWindowExceededError` with diagnostic details (`letta/agent.py:1055-1067`); insufficient credits → graceful stop with `insufficient_credits` (`letta/agents/letta_agent_v2.py:738-747`); any unhandled exception → run marked `failed` with error metadata (`letta/server/rest_api/routers/v1/agents.py:2144`).

Notably, ordinary tool execution failures are neither retried nor escalated — they are returned to the model as error tool-returns so the agent can self-correct (`letta/services/tool_executor/tool_execution_manager.py:143-155`).

### 2. Are escalation thresholds configurable?

Partially:

- **Yes**: summarizer/compaction retry ceiling via env (`LETTA_SUMMARIZER_MAX_SUMMARIZER_RETRIES`, `letta/settings.py:75,96`); provider SDK retry counts (`anthropic_max_retries` settings.py:181, `gemini_max_retries` settings.py:218); which tools require human approval, per tool and per agent, toggled at runtime through the API (`letta/server/rest_api/routers/v1/agents.py:714-740`); max steps per request (`max_steps` parameter, `letta/server/rest_api/routers/v1/agents.py:2072`).
- **No**: the application-level backoff parameters (initial_delay=1, base=2, max_retries=20, error_codes=(429,)) are hardcoded function defaults on `retry_with_exponential_backoff` (`letta/llm_api/llm_api_tools.py:38-47`); DB retry counts are local constants (`max_retries = 3`, `letta/server/db.py:84`); the watchdog thresholds are constructor arguments but deployment values live outside config files; there is no config surface for "when to notify a human" beyond approval flags — callbacks depend solely on the client-supplied `callback_url` field on the run.

### 3. Can the system stop gracefully?

Yes, through several mechanisms:

- Every loop iteration checks for run cancellation before proceeding (`letta/agents/letta_agent_v2.py:505-509`), and cancellation is idempotent plus approval-aware (pending approvals are auto-denied cleanly so conversation state stays consistent, `letta/services/run_manager.py:649-739`).
- The stop-reason enum forces every exit to declare itself, and the `run_status` property makes completed-vs-failed-vs-cancelled deterministic (`letta/schemas/letta_stop_reason.py:9-49`). `requires_approval` and `max_steps` exits are classified as *completed*, preserving resumability.
- Streaming paths guarantee terminal events even when the generator dies: missing terminals are synthesized (`letta/server/rest_api/redis_stream_manager.py:283-315`), cleanup-phase errors still emit a final stop reason + error event (`letta/agents/letta_agent_v3.py:718-736`), and conversation locks are released on terminal updates (`letta/services/run_manager.py:390-396`).
- Approval pauses persist the request message immediately "to prevent agent from getting into a bad state" (`letta/agents/letta_agent_v2.py:636-646`), and a retry of the same approval response after context eviction succeeds via an idempotency check (`tests/integration_test_human_in_the_loop.py:1456-1568`).
- Server-level graceful degradation: the readiness gate marks the pod degraded under load/hangs and recovers only when all degradation sources clear (`letta/monitoring/readiness_state.py:63-84`).

### 4. Are recovery decisions auditable?

Largely yes:

- Each retry attempt, retry exhaustion, and non-retryable rejection emits a named OTEL event with attempt number, delay, status code, and error type (`letta/llm_api/llm_api_tools.py:72-115`), on top of OTel spans recording exceptions with full attributes (`letta/otel/tracing.py:63-68,409-413`).
- Terminal outcomes are durable: failed runs store the error string in run metadata (`letta/server/rest_api/redis_stream_manager.py:396-402`), agents retain `last_stop_reason` queryable via a list filter (`letta/services/run_manager.py:398-410`, `letta/services/helpers/agent_manager_helper.py:760`), and invalid lifecycle transitions are logged rather than silently applied (`letta/services/run_manager.py:341-356`).
- Callback delivery outcomes (`callback_sent_at`, `callback_status_code`, `callback_error`) are persisted on the run row (`letta/services/run_manager.py:470-477`).
- Tool failures carry stderr tracebacks in the result and increment labeled counters (`tool.execution_success` attribute, `letta/services/tool_executor/tool_execution_manager.py:141-160`).
- Gaps: compaction/summarization retries inside v2/v3 loops log via `self.logger.info/warning` only (e.g., `letta/agents/letta_agent_v3.py:1220-1222`) — they do not emit structured events like the LLM-rate-limit retrier does, so correlating compaction retries across agents relies on log aggregation rather than telemetry.

## Architectural Decisions

1. **Escalation is a stop reason, not a control-flow subsystem.** Human-in-the-loop is expressed as `StopReasonType.requires_approval` mapping to `RunStatus.completed` (`letta/schemas/letta_stop_reason.py:21,30-32`), so paused-for-human runs look successful to schedulers and can be resumed with a plain follow-up message. There is no escalation manager, policy engine, or pager integration anywhere in `letta/`.

2. **Typed error taxonomy drives recovery routing.** Provider-specific exceptions are normalized once per client in `handle_llm_error` implementations (base contract at `letta/llm_api/llm_client_base.py:369`; richest implementation `letta/llm_api/openai_client.py:1216-1420`) into a shared hierarchy (`letta/errors.py:259-349`). Agent loops then make binary retry decisions purely via `isinstance` checks (`letta/agents/letta_agent_v3.py:1214-1218`), keeping provider quirks out of loop logic.

3. **Compaction-on-overflow as the primary self-healing loop.** Rather than failing long conversations, overflow triggers summarize-and-retry with persisted summary messages and system-prompt rebuild (`letta/agents/letta_agent_v3.py:1241-1262`), with the summarizer having its own two-stage degradation ladder (`letta/services/summarizer/summarizer.py:568-619`).

4. **Model failover delegated to a routing abstraction.** The v3 loop consults `LLMRoutingClient.get_fallback_handle` and records success/failure signals (circuit-breaker inputs) around each attempt (`letta/agents/letta_agent_v3.py:1171-1211`), separating "which model" from "how to recover."

5. **Errors to the model, exceptions to the operator.** Tool failures become conversational content (`status="error"` tool returns, `letta/services/tool_executor/tool_execution_manager.py:143-155`), while infrastructure/LLM failures become run-terminal events with metadata. This keeps the agent's self-correction loop separate from the platform's failure accounting.

## Notable Patterns

- **Layered defense in depth for one failure class**: context overflow is handled at four levels — proactive token pre-check raising early (`letta/llm_api/llm_api_tools.py:150-156`), loop-level compaction retry (`letta/agents/letta_agent_v3.py:1217-1251`), summarizer-internal fallbacks (`letta/services/summarizer/summarizer.py:568-619`), and finally a fatal typed error carrying diagnostics (`letta/agent.py:1060-1067`).
- **Terminal-event synthesis for streams**: background stream processing never leaves clients hanging — absent terminals are fabricated and every except branch writes `[DONE]` (`letta/server/rest_api/redis_stream_manager.py:283-315,317-394`).
- **Cancellation disambiguation**: task-level cancellation inspects persisted run state to distinguish user-initiated cancel from pod shutdown, emitting different terminal markers accordingly (`letta/server/rest_api/redis_stream_manager.py:324-377`).
- **Watchdog with self-validation**: the event-loop watchdog ships with its own runnable verification script asserting hang detection behavior (`test_watchdog_hang.py:61-77`), and its observability code is explicitly fenced so it cannot interfere with safety ("Observability must never interfere with watchdog safety", `letta/monitoring/event_loop_watchdog.py:100`).
- **Multi-source degradation gating**: readiness recovery requires *all* registered degradation sources to clear, preventing flapping when independent gates fire (`letta/monitoring/readiness_state.py:19-21,63-84`).

## Tradeoffs

- **Triplicated recovery logic**: v1 (`letta/agent.py:1022-1072`), v2 (`letta/agents/letta_agent_v2.py:518-573`), and v3 (`letta/agents/letta_agent_v3.py:1093-1297`) each reimplement the overflow-retry pattern with slightly different behaviors (recursive inner_step vs inline loop vs compaction events). Bug fixes must be applied three times; the legacy path already contains a known "patch... should be removed" hack (`letta/agent.py:1029-1035`).
- **Silent notification failure vs guaranteed escalation**: callback and webhook senders deliberately swallow errors to avoid affecting run completion (`letta/services/run_manager.py:508`, `letta/services/webhook_service.py:48-55`). This protects run integrity but means a human depending on webhooks learns of failures only if they poll — the failure itself is recorded (`callback_error` column) but never re-attempted.
- **Hardcoded aggressive defaults**: 20 rate-limit retries with unbounded sleep growth (`letta/llm_api/llm_api_tools.py:43,96-104`) can hold a request thread for a very long time; making this configurable would trade operational flexibility for another config surface.
- **String-heuristic provider classification**: overflow/credit detection relies on substring matching over provider messages (`letta/llm_api/error_utils.py:16-41`). It is centralized and tested, but inherently fragile against provider wording changes — a missed phrase converts a recoverable overflow into a fatal `LLMBadRequestError`.
- **Self-correction vs runaway loops**: feeding tool errors back to the model enables recovery without human intervention but relies on `max_steps` bounds (`letta/agents/letta_agent_v3.py:328`) to prevent infinite error-correction cycles; the bound costs a hard stop even when one more step might have succeeded.

## Failure Modes / Edge Cases

- **Dead retry decorator on modern paths**: `retry_with_exponential_backoff` only catches `requests.exceptions.HTTPError` (`letta/llm_api/llm_api_tools.py:64`), but the active async clients use httpx/OpenAI SDKs whose errors take different shapes; the wrapper also contains unreachable dead code (`pass` then initialization at :50-54). Recovery on those paths depends entirely on SDK-level `max_retries` and loop-level handling.
- **Summarizer exhaustion is fatal and lossy**: after `max_summarizer_retries`, the legacy path raises `ContextWindowExceededError` embedding full in-context message text in details (`letta/agent.py:1060-1067`) — useful for debugging but potentially huge in logs/metadata.
- **Mid-stream errors cannot change HTTP status**: once streaming started, failures degrade to SSE `event: error` payloads with a generic user-facing message ("An error occurred during agent execution.", `letta/agents/letta_agent_v3.py:664-692`); clients must parse the stream to detect failure.
- **Completed-run mutation window**: a run marked completed can be flipped to cancelled if accompanied by a `requires_approval` stop reason — allowed explicitly (`letta/services/run_manager.py:342-352`) — which is intentional for approval flows but widens the state machine.
- **Credit check race**: `_check_run_cancellation` and credit verification swallow lookup exceptions and return "keep going" (`letta/agents/letta_agent_v2.py:750-757`), favoring availability over strict enforcement.
- **Callback payload size**: terminal callbacks embed full result messages including refreshed history (`letta/services/run_manager.py:451-460`), which can be large for long runs; delivery has a fixed 5-second timeout (:501).

## Future Considerations

- Consolidate the overflow-retry pattern into one shared component used by all agent generations, deleting the legacy recursive path (`letta/agent.py:1029-1053`) and the dead sync-era backoff wrapper (`letta/llm_api/llm_api_tools.py:38-118`).
- Add structured telemetry (OTEL events mirroring `llm_retry_attempt`) to compaction retries and summarizer fallbacks so recovery activity is queryable, not just greppable.
- Introduce at-least-one retry with backoff for run callbacks/webhooks, or expose delivery failures through the same stop-reason/metadata channel clients already consume.
- Promote the implicit escalation inventory (approvals, credit stops, fatal compaction failure) into an explicit, documented policy table — currently discoverable only by reading three agent loops and the stop-reason map.
- Make `retry_with_exponential_backoff` parameters and DB retry counts environment-configurable alongside the existing `LETTA_SUMMARIZER_*` and provider retry knobs.

## Questions / Gaps

- No evidence found of a general-purpose escalation framework (searched `escalat*` across all `.py` files — zero matches). If product-level escalation (paging, admin notification) exists, it lives outside this repository.
- No evidence found of automatic re-drive/resume of failed runs; recovery from failure is delegated to clients, as implied by user-facing copy ("Background stream processing was interrupted before completion. Please retry.", `letta/server/rest_api/redis_stream_manager.py:358`). A scheduled job/scheduler module exists (`letta/jobs/scheduler.py:108-133`) but it manages leader locks, not failure retries.
- Whether `RateLimitExceededError` (raised post-backoff-exhaustion, `letta/llm_api/llm_api_tools.py:94`) is ever caught and handled upstream was not traced exhaustively; it appears to surface as a generic error response via the FastAPI handlers (`letta/server/rest_api/app.py:782-793`).
- Watchdog deployment thresholds (production `check_interval`/`timeout_threshold`) were not located in committed config; only constructor defaults (5s/15s) and the test script's values are in-tree.
- Human notification for *failures* (as opposed to step completions) beyond caller-supplied `callback_url` shows no dedicated mechanism in this source; `WEBHOOK_SETUP.md` at the repo root documents setup but was treated as documentation, verified only against `letta/services/webhook_service.py:13-14` env-var names.

---

Generated by dimension `13.04-recovery-vs-escalation` against source `letta`.
