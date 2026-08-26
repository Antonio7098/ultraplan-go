# Source Analysis: pydantic-ai

## Dimension 07.03: Idempotency and Retry Semantics

### Source Info

| Field | Value |
|-------|-------|
| Name | pydantic-ai |
| Path | `studies/agent-harness-study/sources/pydantic-ai` |
| Language / Stack | Python (pydantic-core, anyio, tenacity, httpx/httpx2; durable-exec integrations for Temporal, DBOS, Prefect) |
| Analyzed | 2026-08-26 |

All citations below are workspace-relative to `studies/agent-harness-study/sources/pydantic-ai/`. For readability the prefix is abbreviated in tables as `…` = `studies/agent-harness-study/sources/pydantic-ai`.

## Summary

Pydantic AI implements retry semantics as an explicit **five-layer model with five independent budgets** — transport (HTTP), model fallback (explicitly "not a retry"), tool retries, output retries, and model-request-hook retries (`docs/retries.md:3-15`). Tool retries are not blind re-execution: a retry is a *message to the model* (`RetryPromptPart`, `pydantic_ai_slim/pydantic_ai/messages.py:1636-1739`) that replaces the tool result in history under the same `tool_call_id`, and the model issues a new call. Failure classification is deliberate: `ModelRetry` consumes the per-tool budget, `ToolFailed` reports a terminal failure without consuming budget, deferrals (`CallDeferred`/`ApprovalRequired`) are control flow that never touches the budget, and unknown exceptions abort the run unless an error hook converts them (`pydantic_ai_slim/pydantic_ai/exceptions.py:57-147`, `pydantic_ai_slim/pydantic_ai/tool_manager.py:256-307`).

There is **no framework-level idempotency-key store**; safety of side-effecting tools is achieved structurally instead: per-tool named budgets that reset on success, duplicate-`tool_call_id` fail-closed checks on resume, a deterministic/idempotent transcript-repair pass that synthesizes `'interrupted'` returns for dangling calls rather than re-running them, external tools that can never execute locally (`max_retries=0`, `RuntimeError` guard), and value-addressed task caching in the Prefect integration specifically engineered so flow retries *replay* tool results instead of re-running non-idempotent tools. Where side effects genuinely re-execute (Temporal activity retries of event handlers), the framework documents and delegates idempotency responsibility to the user.

**Answer to the dimension question — can a payment/email/delete tool be retried safely?** Only partially by the framework. A retried call is a fresh model-initiated invocation; the framework guarantees the failure is visible, bounded (per-tool budget), and traceable, but it does not deduplicate the underlying effect. Safe use requires the developer to either make the effect idempotent, route it through approval/deferral, or run under a durable engine whose recorded-result replay prevents re-execution (Prefect cache policies do this explicitly).

## Rating

**8 / 10** — A clear, layered retry model with explicit interfaces (`ModelRetry`/`ToolFailed`/`RetryPromptPart`), enforced budgets with documented precedence, operational safeguards (duplicate-ID fail-closed, availability-refusal free pass, non-replayable-stream refusal at transport level), exceptional documentation (`docs/retries.md` maps all five layers), and strong tests including determinism tests for history repair (`tests/test_transcript_repair.py:622`). It falls short of 9–10 because actual side-effect idempotency is delegated to users/durable engines with no built-in key store, the reset-on-success counter admits unbounded fail/success alternation (documented at `docs/retries.md:35`), hallucinated tool names receive a fresh budget each time (documented quirk, `docs/retries.md:37`), and a known dual-budget mismatch between output validators and output tools is acknowledged as an open issue (`pydantic_ai_slim/pydantic_ai/tool_manager.py:855-862`, referencing pydantic-ai#5238).

## Evidence Collected

Every entry uses `…` = `studies/agent-harness-study/sources/pydantic-ai`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Retry policy map | Five retry layers (transport, fallback, tool, output, hooks) with separate budgets; only the last three cost a model round trip | `…/docs/retries.md:3-15` |
| Transport retry wrapper | `HTTPX2TenacityTransport.handle_request` wraps transport in tenacity `@retry(**config)`; deprecated `TenacityTransport` kept until v3 | `…/pydantic_ai_slim/pydantic_ai/retries.py:195-221`, `343-410` |
| Async transport retry wrapper | `AsyncHTTPX2TenacityTransport.handle_async_request` mirrors sync path | `…/pydantic_ai_slim/pydantic_ai/retries.py:298-324` |
| Retry config surface | `RetryConfig` TypedDict exposes exactly tenacity's decorator args (stop/wait/retry/reraise/before_sleep…) | `…/pydantic_ai_slim/pydantic_ai/retries.py:72-134` |
| Retry-After-aware wait | `wait_retry_after` parses seconds or HTTP-date from `Retry-After`, caps at `max_wait`, falls back to exponential backoff | `…/pydantic_ai_slim/pydantic_ai/retries.py:514-590` |
| Non-replayable body guard | Transport refuses to retry requests whose body is a consumed stream (`httpx2.StreamConsumed`) — an explicit transport-level idempotency constraint | `…/pydantic_ai_slim/pydantic_ai/retries.py:151-153`, `250-252` |
| Model-facing retry signal | `ModelRetry` exception carries a message sent back to the model; serializable via pydantic core schema | `…/pydantic_ai_slim/pydantic_ai/exceptions.py:57-97` |
| Terminal-failure signal | `ToolFailed` produces a failed result the model sees, "does not consume the tool's retry budget"; bounded by `UsageLimits` instead | `…/pydantic_ai_slim/pydantic_ai/exceptions.py:100-112` |
| Retry-prompt message part | `RetryPromptPart`: content is error details or string, keyed by `tool_name` + `tool_call_id`; renders with "Fix the errors and try again." | `…/pydantic_ai_slim/pydantic_ai/messages.py:1636-1739` (render at 1699-1721) |
| Error → retry prompt builder | `RetryPromptPart.from_error` is "the exact message the model receives", reused by instrumentation so spans match | `…/pydantic_ai_slim/pydantic_ai/messages.py:1676-1697` |
| Per-tool budget check | `_check_max_retries`: raises `UnexpectedModelBehavior` when `ctx.retries[name] >= max_retries` (`>=` guards negative budgets) | `…/pydantic_ai_slim/pydantic_ai/tool_manager.py:256-265` |
| Budget carry-over across steps | `ToolManager.for_run_step`: failed tools increment count; succeeded tools drop out (reset); carried in `ctx.retries` dict | `…/pydantic_ai_slim/pydantic_ai/tool_manager.py:187-220` |
| Success/failure bookkeeping | `failed_tools`/`succeeded_tools` sets; success recorded only when error-wrapping enabled so nested/raw-mode callers don't corrupt budgets | `…/pydantic_ai_slim/pydantic_ai/tool_manager.py:155-158`, `987-991`, `904-906` |
| Availability-refusal free pass | First `_ToolUnavailable` refusal per tool is not charged against the budget (`availability_refused` set spans whole run); later refusals charge normally | `…/pydantic_ai_slim/pydantic_ai/tool_manager.py:159-166`, `587-595` |
| Unknown-tool retry | Unknown name → `ModelRetry('Unknown tool name…')` listing currently available tools; charged under the invented name's own budget | `…/pydantic_ai_slim/pydantic_ai/tool_manager.py:496-517`; `…/docs/retries.md:37` |
| Validation failure handling | `_make_validation_failure` checks retries then wraps as `ToolRetryError`; `ToolFailed` wrapped as `ToolFailedError` *without* budget check | `…/pydantic_ai_slim/pydantic_ai/tool_manager.py:569-607`, `682-692` |
| Execution-time retry wrap | `_raw_execute` catches `ModelRetry` → `_check_max_retries` → `failed_tools.add` → `ToolRetryError`; `usage.tool_calls += 1` only on success | `…/pydantic_ai_slim/pydantic_ai/tool_manager.py:994-1027` |
| Deferral ≠ retry | `ApprovalRequired`/`CallDeferred` during validation report `args_valid=True`, consume no retry budget; re-raised at execution boundary | `…/pydantic_ai_slim/pydantic_ai/tool_manager.py:85-94`, `661-672` |
| Timeout → retryable | Tool timeout converted to `ModelRetry(f'Timed out after {timeout} seconds.')`, counting toward the tool's limit | `…/pydantic_ai_slim/pydantic_ai/toolsets/function.py:679-691`; `…/docs/tools-advanced.md:607-635` |
| Output-retry budget | `consume_output_retry` increments `output_retries_used`, raises `UnexpectedModelBehavior` past `max_output_retries` | `…/pydantic_ai_slim/pydantic_ai/_agent_graph.py:361-378` |
| Retry node construction | `_build_retry_node`: consumes output budget, creates `ModelRequestNode` carrying a `RetryPromptPart` (model-request hook path) | `…/pydantic_ai_slim/pydantic_ai/_agent_graph.py:1797-1810` |
| Budget configuration | `retries={'tools': N, 'output': N}` on Agent/run/override; default 1 each; precedence tool > toolset > agent > run override | `…/pydantic_ai_slim/pydantic_ai/agent/__init__.py:352-371`, `579-587`, `657-672`; `…/docs/tools-advanced.md:558-562` |
| RunContext retry fields | `ctx.retries: dict[str,int]` ("Number of retries for each tool so far"), `ctx.retry`, `ctx.max_retries` exposed to tools | `…/pydantic_ai_slim/pydantic_ai/_run_context.py:97-114` |
| Duplicate ID detection (resume) | `_duplicate_tool_call_ids` + hard `UserError` when deferred results cannot be matched unambiguously | `…/pydantic_ai_slim/pydantic_ai/_tool_execution.py:119-127`, `405-420` |
| Dangling-call repair (idempotent) | `_repair_dangling_tool_calls` synthesizes `outcome='interrupted'` returns marked `SYNTHESIZED_TOOL_RETURN_METADATA_KEY`; "deterministic and idempotent" | `…/pydantic_ai_slim/pydantic_ai/_agent_graph.py:2702-2733`, `2816-2898` (quote at 2844-2849) |
| Orphaned-result removal | `_drop_orphaned_tool_results` removes results whose call never existed (provider-validity pass) | `…/pydantic_ai_slim/pydantic_ai/_agent_graph.py:2771-2813` |
| Ordered-walk matching | Duplicate/reused/out-of-place results do not mask genuinely dangling calls | `…/pydantic_ai_slim/pydantic_ai/_agent_graph.py:2706-2733`, `2839-2842` |
| Never both return and retry | A retried call has a `RetryPromptPart` *instead of* a `ToolReturnPart`, same `tool_call_id` | `…/docs/retries.md:41-95` |
| External tools can't run locally | `RuntimeError('External tools cannot be called')`; external toolset pins `max_retries=0` | `…/pydantic_ai_slim/pydantic_ai/tool_manager.py:976-977`; `…/pydantic_ai_slim/pydantic_ai/toolsets/external.py:37` |
| Approval re-validation with override | Approved calls may carry `override_args`; re-validated before execution (`_validate_approved_call`) | `…/pydantic_ai_slim/pydantic_ai/_tool_execution.py:609-623` |
| Sub-agent cancellation isolation | Nested-run `RunCancelled` becomes a failed tool return the model can react to, not a crash of the caller | `…/pydantic_ai_slim/pydantic_ai/_tool_execution.py:41-62` |
| Prefect replay-vs-rerun | Cache keys deliberately keep framework-generated `tool_call_id`s out of identity but keep the dispatching call's own ID verbatim ("must each execute rather than replay each other") | `…/pydantic_ai_slim/pydantic_ai/durable_exec/prefect/_cache_policies.py:80-106` |
| Prefect non-idempotent fix | Value-addressed tool identity fixed a bug where "tool results never replay, re-running non-idempotent tools on every retry" | `…/pydantic_ai_slim/pydantic_ai/durable_exec/prefect/_cache_policies.py:171-196` (quote 182-183) |
| Temporal non-retryable classes | Activity retry policies mark `UserError`, `PydanticUserError`, `UnexpectedModelBehavior`, `FallbackExceptionGroup`, `PayloadSizeError` non-retryable | `…/pydantic_ai_slim/pydantic_ai/durable_exec/temporal/_toolset.py:156-170`; `…/docs/durable_execution/temporal.md:155` |
| Temporal stacking warning | Docs recommend disabling HTTP-request retries under Temporal to avoid compounding retries with improper `Retry-After` handling | `…/docs/durable_execution/temporal.md:474-478` |
| User-delegated idempotency | "a handler may run more than once if an activity retries, so keep its side effects idempotent" | `…/docs/durable_execution/temporal.md:292` |
| OTel visibility | Instrumentation records the retry prompt as `gen_ai.tool.result` attribute on execute_tool spans when content capture is on | `…/pydantic_ai_slim/pydantic_ai/capabilities/instrumentation.py:332-363`, `434-476` |
| Event visibility | `FunctionToolResultEvent`/`OutputToolResultEvent` yield retry parts to `agent.iter()` consumers | `…/pydantic_ai_slim/pydantic_ai/_tool_execution.py:130-138`, `750-772` |
| Cancellation preserves settled effects | `RunCancelled.all_messages()` keeps finished tool results; unanswered calls closed with synthesized `'interrupted'` returns on resume | `…/pydantic_ai_slim/pydantic_ai/exceptions.py:281-286` |
| Transport retry tests | Sync/async transports retry 503→200 via validator; `wait_retry_after` header parsing covered | `…/tests/test_tenacity.py:42-120` (and file-wide) |
| Budget semantics tests | `ctx.retry` observed as [0, 1] across attempts; `ToolFailed` must not trip `retries=1`; max-retries exceeded raises | `…/tests/test_tools.py:4127-4149`, `~1625`, `3979-3983` |
| Repair tests | Dangling/orphan/duplicate-ID repair incl. `test_full_pipeline_idempotent_and_deterministic`, `test_duplicate_result_ignored`, `test_reused_tool_call_id_*` | `…/tests/test_transcript_repair.py:622`, `665`, `749`, `784` |
| Duplicate-ID resume test | Resume with ambiguous duplicate IDs raises `UserError('duplicate tool_call_id')` | `…/tests/test_agent.py:11014` |

## Answers to Dimension Questions

### 1. Which tool failures are retried?

Retried (a new model round trip): Pydantic argument-validation failures, explicit `ModelRetry` from the tool/args-validator/hooks, tool timeouts (`pydantic_ai_slim/pydantic_ai/toolsets/function.py:691`), unknown tool names (`pydantic_ai_slim/pydantic_ai/tool_manager.py:514`), and unavailable-tool refusals after the first free one (`pydantic_ai_slim/pydantic_ai/tool_manager.py:594-597`). Not retried: `ToolFailed` (terminal, budget untouched, `pydantic_ai_slim/pydantic_ai/exceptions.py:100-112`), any other exception (propagates unless an `on_tool_execute_error` hook converts it — `docs/retries.md:131`), `prepare` callbacks and `before_model_request` (never become retry prompts — `docs/retries.md:129-130`). Under durable execution, framework-level `UserError`/`UnexpectedModelBehavior` are classified non-retryable at the engine layer (`pydantic_ai_slim/pydantic_ai/durable_exec/temporal/_toolset.py:156-170`).

### 2. Are repeated attempts safe?

The mechanism itself is safe-by-construction for *dispatch*: a retry is a fresh `ToolCallPart` from the model answered once — history invariant "there is never both" a return and a retry for one ID (`docs/retries.md:43`), duplicates rejected or repaired deterministically (`pydantic_ai_slim/pydantic_ai/_tool_execution.py:405-420`; `pydantic_ai_slim/pydantic_ai/_agent_graph.py:2844-2849`). Safety of the *effect* is the developer's job: nothing intercepts a second execution of a payment-style tool beyond budget exhaustion, and the docs/skills say so outright ("never blindly retry side effects" — `pydantic_ai_slim/pydantic_ai/.agents/skills/migrating-langchain-to-pydantic-ai/references/WORKAROUND-RECIPES.md:111`).

### 3. Is retry state persisted?

In-memory per run: counters live in `RunContext.retries` and `ToolManager.failed_tools/succeeded_tools` (`pydantic_ai_slim/pydantic_ai/_run_context.py:97-98`; `pydantic_ai_slim/pydantic_ai/tool_manager.py:155-158`) and are rebuilt per step via `for_run_step`. Nothing persists them across runs; what crosses runs is the *history* — replaying `all_messages()` shows the model its earlier `RetryPromptPart`s (`docs/retries.md:97`). The Temporal integration serializes the `retries`/`retry` fields into activity payloads for replay fidelity (`docs/durable_execution/temporal.md:218`).

### 4. Are non-idempotent tools protected?

Indirectly, not by keys: per-tool budgets bound repeated attempts (default 1 retry, precedence ladder at `pydantic_ai_slim/pydantic_ai/agent/__init__.py:579-587`); `requires_approval=True` gates execution behind human approval with optional arg overrides (`pydantic_ai_slim/pydantic_ai/tools.py:306`, `333`; `pydantic_ai_slim/pydantic_ai/_tool_execution.py:609-623`); external tools never run locally and get `max_retries=0` (`pydantic_ai_slim/pydantic_ai/tool_manager.py:976-977`; `pydantic_ai_slim/pydantic_ai/toolsets/external.py:37`); `sequential=True` barriers prevent overlapping parallel executions of a single tool (`pydantic_ai_slim/pydantic_ai/tools.py:583-589`). No `idempotency_key` parameter exists anywhere in the tool API.

### 5. Can retries create duplicate side effects?

Yes at the framework level, mitigated at the edges: (a) the counter resets on success, so alternating fail/success invocations are unbounded within `request_limit` (`docs/retries.md:35`); (b) each hallucinated tool name gets a fresh budget (`docs/retries.md:37`); (c) under Temporal, event-handler activities may re-run on activity retry — the docs instruct users to keep handler side effects idempotent (`docs/durable_execution/temporal.md:292`). The strongest protection is the Prefect integration's value-addressed cache, which replays recorded tool results across flow retries precisely so non-idempotent tools do not re-run (`pydantic_ai_slim/pydantic_ai/durable_exec/prefect/_cache_policies.py:171-196`), while still executing two same-argument parallel calls separately by keeping the dispatching `tool_call_id` verbatim in the key (`_cache_policies.py:82-86`).

## Architectural Decisions

1. **Retry-as-conversation, not retry-as-loop.** Tool/output retries produce a `RetryPromptPart` and a new model request; the framework never silently re-invokes a failed function tool. This makes every retry visible, bounded by model behavior, and auditable in history (`pydantic_ai_slim/pydantic_ai/messages.py:1636-1650`; `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1797-1810`).
2. **Budget separation.** Five layers, five independent budgets, no sharing (`docs/retries.md:3`); within agent retries, per-name tool budgets vs. a global output budget are enforced in different places (`ToolManager._check_max_retries` vs. `GraphAgentState.consume_output_retry`, `pydantic_ai_slim/pydantic_ai/_agent_graph.py:361-378`).
3. **Explicit retryability taxonomy.** `ModelRetry` (chargeable) vs. `ToolFailed` (terminal, uncharged) vs. deferrals (control flow, uncharged) vs. raw exceptions (abort) gives developers first-class vocabulary for "should this be attempted again?" (`pydantic_ai_slim/pydantic_ai/exceptions.py:57-147`).
4. **Fail-closed on ambiguity.** Resume results that cannot be matched unambiguously to calls raise `UserError` instead of guessing (`pydantic_ai_slim/pydantic_ai/_tool_execution.py:405-420`).
5. **Deterministic history repair.** Provider-invalid histories are healed by pure, idempotent passes that synthesize marked `'interrupted'` returns rather than dropping calls — protecting both provider acceptance and prompt-cache prefixes (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:2960-2986`).
6. **Idempotency delegated downward.** Transport-level guards (non-replayable streams), engine-level non-retryable classifications, and user-level guidance ("keep its side effects idempotent") form a layered responsibility chain instead of a central idempotency store.

## Notable Patterns

- **Free first refusal**: the first time a model calls a tool that isn't available yet, the correction message is free; only repeat disobedience spends budget — separating "state of the run" failures from "mistake about arguments" failures (`pydantic_ai_slim/pydantic_ai/tool_manager.py:587-595`).
- **Raw-error escape hatch**: `wrap_validation_errors=False` lets nested/sandboxed/streaming callers bypass budget accounting entirely so inner executions don't corrupt outer budgets (`pydantic_ai_slim/pydantic_ai/tool_manager.py:640-647`, `987-991`).
- **Shared rendering contract**: `RetryPromptPart.from_error` is declared the single source of the model-visible message, and OTel spans reuse it so telemetry matches exactly what the model saw (`pydantic_ai_slim/pydantic_ai/messages.py:1684-1687`; `pydantic_ai_slim/pydantic_ai/capabilities/instrumentation.py:332-363`).
- **Retry-wins invariant**: if a function tool produced a retry while an output tool succeeded in the same step, the final result is suppressed so the model addresses the retry first (`pydantic_ai_slim/pydantic_ai/_tool_execution.py:881-908`).
- **Exhaustive-projection cache keys with an exhaustiveness test**: Prefect's RunContext projection lists every field a task could depend on and a dedicated test fails when a new field is uncategorized (`pydantic_ai_slim/pydantic_ai/durable_exec/prefect/_cache_policies.py:62-74`).

## Tradeoffs

- **Visibility vs. cost**: every tool retry costs a full model round trip (by design, `docs/retries.md:15`); cheap deterministic flakiness must be handled below the agent (transport retries) or it becomes expensive.
- **Reset-on-success counter vs. runaway loops**: clearing a tool's count on success permits indefinite fail→succeed→fail cycling; the framework accepts this in exchange for predictable per-call semantics and pushes run-level bounding onto `UsageLimits.request_limit` (default 50, `pydantic_ai_slim/pydantic_ai/usage.py:429`) — note `tool_calls_limit` counts only successful calls (`pydantic_ai_slim/pydantic_ai/usage.py:431-432`), so a `ToolFailed` loop is bounded mainly by requests.
- **No central idempotency store vs. simplicity**: avoiding a key/value receipt store keeps the core slim, but shifts exactly-once concerns to users and durable engines.
- **Stacked retries**: enabling transport retries together with Temporal activities can multiply attempts with divergent `Retry-After` handling; docs resolve this by recommending one owner per layer (`docs/durable_execution/temporal.md:474-478`).

## Failure Modes / Edge Cases

- **Non-idempotent tool retried after partial effect**: `ModelRetry` after a side effect already happened (e.g., write succeeded, response parsing failed) will re-invoke the tool — the framework provides no journal to prevent it outside durable engines.
- **Hallucinated-name budget farming**: a model inventing a different wrong tool name each turn gets a fresh budget each time; bounded only by output/request limits (`docs/retries.md:37`).
- **Dual-budget mismatch on output tools**: when `ToolOutput(max_retries=N)` exceeds `max_output_retries`, a validator's `ctx.last_attempt` can fire before the run actually terminates; tracked upstream as pydantic-ai#5238 (`pydantic_ai_slim/pydantic_ai/tool_manager.py:855-862`).
- **Negative budgets**: guarded by `>=` comparison raising immediately rather than looping forever (`pydantic_ai_slim/pydantic_ai/tool_manager.py:259-261`); `max_retries < 0` rejected at construction (`pydantic_ai_slim/pydantic_ai/tools.py:281-283`).
- **Stream-body retries**: a request whose body was a consumed stream cannot be retried and surfaces `httpx2.StreamConsumed` instead of silently resending (`pydantic_ai_slim/pydantic_ai/retries.py:151-153`).
- **Crash mid-tool-execution**: dangling calls are closed with synthesized `'interrupted'` returns on next request; the *last* response's calls stay answerable for resumption/approval flows (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:2835-2837`).
- **Duplicate resume results**: ambiguous `deferred_tool_results` mapping fails closed with `UserError` rather than double-binding one result to two calls (`pydantic_ai_slim/pydantic_ai/_tool_execution.py:405-412`).

## Future Considerations

- An optional framework-level idempotency-key/receipt interface on `Tool`/`ToolDefinition` would let payment/email/delete tools declare safe-replay contracts instead of relying on convention; today the closest analogues are Prefect cache keys and Temporal activity memoization.
- Resolving the output-budget duality (#5238) would unify `ctx.retry` semantics between text-path and tool-path outputs (`pydantic_ai_slim/pydantic_ai/tool_manager.py:855-862`).
- A unified usage-return channel across durable engines (under discussion upstream, referenced at `docs/durable_execution/temporal.md:223`) would also fix accounting skew between replayed and executed steps, which currently differs among Temporal/DBOS/Prefect.

## Questions / Gaps

- **No evidence found** for any persisted idempotency-key store, attempt ledger, or receipt table inside the source tree; searches for `idempoten*` returned only incidental uses (cancel/no-op idempotence, Prefect comments, migration-skill guidance). The dimension's "idempotency stores" item is therefore answered in the negative for the core library and positively only for Prefect's cache-policy approximation.
- Whether `on_tool_execute_error` hooks see timeouts converted to `ModelRetry` *before* conversion (i.e., can a hook distinguish a timeout from a deliberate `ModelRetry`) was not traced exhaustively; the conversion happens inside `FunctionToolset.call_tool` (`pydantic_ai_slim/pydantic_ai/toolsets/function.py:686-691`), which sits below the hook boundary, suggesting hooks see the already-wrapped retry — verified only by code reading, not by a dedicated test.
- Real-provider VCR coverage of multi-attempt tool-retry histories exists indirectly through snapshot-based tests (e.g., `tests/test_tools.py` retry snapshots), but no cassette-specifically pinning `RetryPromptPart` wire shape across all three major providers was inspected in this study.

---

Generated by `dimensions/07.03-idempotency-and-retry-semantics` against `pydantic-ai`.
