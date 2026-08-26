# Source Analysis: crewai

## Dimension 07.03: Idempotency and Retry Semantics

### Source Info

| Field | Value |
|-------|-------|
| Name | crewai |
| Path | `studies/agent-harness-study/sources/crewai` |
| Language / Stack | Python (uv monorepo: `lib/crewai` core framework, `lib/crewai-tools`, `lib/crewai-files`, `lib/cli`) |
| Analyzed | 2026-08-26 |

All citations below are relative to the source root `studies/agent-harness-study/sources/crewai/`.

## Summary

CrewAI has four distinct retry layers, each with different safety characteristics:

1. **Tool-call retries (ReAct path)**: any exception raised by a tool is blindly re-invoked with identical arguments until `_run_attempts` exceeds `_max_parsing_attempts` (3 attempts, or 2 for large OpenAI models) (`lib/crewai/src/crewai/tools/tool_usage.py:451-479`, `lib/crewai/src/crewai/tools/tool_usage.py:109-134`). There is **no error classification and no idempotency awareness** in this loop — a timeout after a committed side effect would be re-executed.
2. **Task-level retries (agent loop)**: non-Litellm exceptions during task execution re-enter `execute_task` up to `max_retry_limit` (default 2) times, while exceptions from the `litellm` module are deliberately treated as terminal (`lib/crewai/src/crewai/agent/core.py:721-747`, field at `lib/crewai/src/crewai/agent/core.py:255-258`). This is the only explicit retryable/non-retryable split in the core agent loop.
3. **Guardrail retries**: a failed output guardrail re-executes the *entire task* (and therefore all its tools) up to `guardrail_max_retries` (default 3) times, feeding the validation error back as context; tool-failure records from overlapping attempts are deduplicated via `merge_tool_failures` (`lib/crewai/src/crewai/task.py:1337-1437`, defaults at `lib/crewai/src/crewai/task.py:275-282`).
4. **Provider/infra wrappers**: LLM SDKs receive `max_retries=2`; Bedrock uses adaptive retries; MCP tools, file uploads, A2A clients, Brave Search, and LanceDB each implement their own backoff loops with local error classification.

There is **no idempotency-key mechanism anywhere** in the codebase. Duplicate detection exists but is weak: only the immediately preceding call is compared by name + arguments (`_check_tool_repeated_usage`, `lib/crewai/src/crewai/tools/tool_usage.py:779-789`). The closest thing to an idempotency store — the result `CacheHandler` keyed on `f"{tool}-{input}"` (`lib/crewai/src/crewai/agents/cache/cache_handler.py:43-44`) — is **opt-in** at the Crew level (`cache: bool = False`, `lib/crewai/src/crewai/crew.py:229-238`) and lives only in memory.

The newest mitigation is declarative: tools can return a structured `ToolFailure(retryable=...)` instead of raising (`lib/crewai/src/crewai/tools/tool_failure.py:95-98`); declared failures never trigger automatic re-invocation and are never cached (`lib/crewai/src/crewai/agents/cache/cache_handler.py:38-41`). However, the `retryable` flag is currently descriptive metadata — no core code path branches on it.

**Can a payment/email/delete tool be retried safely?** Not by default. If such a tool raises (e.g., timeout after commit), the ReAct path will re-execute it identically up to 3 times without consulting any side-effect metadata. Safety requires developer opt-in: return `ToolFailure` instead of raising, set `max_usage_count=1`, or enable caching with a restrictive `cache_function`.

## Rating

**5 / 10** — Present but inconsistent and fragile in the core loop.

Rationale against the rubric:

- **Why not lower**: real mechanisms exist and some are tested — atomic usage-limit claims under a lock (`lib/crewai/src/crewai/tools/base_tool.py:302-324`), "failures are never cached" with a dedicated test class ("A cached failure would make a transient error permanent", `lib/crewai/tests/tools/test_tool_failure.py:915-953`), a resolvable failure-policy chain (`lib/crewai/src/crewai/tools/tool_failure.py:177-208`), and well-classified retries at external API wrappers (`lib/crewai-files/src/crewai_files/resolution/resolver.py:337-398`, `lib/crewai-tools/src/crewai_tools/tools/brave_search_tool/base.py:51-79`).
- **Why not higher**: no idempotency keys; duplicate detection checks exactly one previous call; exception-driven blind retries can duplicate side effects; the cache-based idempotency store is off by default and process-local; retry state is never persisted; and the ReAct path vs. native function-calling path behave differently (`lib/crewai/src/crewai/utilities/agent_utils.py:1747-1786` has neither auto-retry nor repeated-usage detection).

## Evidence Collected

Every entry cites `path:line` relative to `studies/agent-harness-study/sources/crewai/`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Retry wrapper (tool, sync) | Exception → `should_retry=True` → recursion into `self.use(...)` with same calling | `lib/crewai/src/crewai/tools/tool_usage.py:708-758` |
| Retry wrapper (tool, async) | Mirror of sync path; recursion into `await self.ause(...)` | `lib/crewai/src/crewai/tools/tool_usage.py:451-501` |
| Retry bound | `_run_attempts = 1`, `_max_parsing_attempts = 3`; 2 for `OPENAI_BIGGER_MODELS` | `lib/crewai/src/crewai/tools/tool_usage.py:109-110,130-134,455,712` |
| Retry classification (task level) | litellm-module exceptions re-raised immediately; others retried until `_times_executed > max_retry_limit` (default 2) | `lib/crewai/src/crewai/agent/core.py:721-747`; `lib/crewai/src/crewai/agent/core.py:255-258` |
| Guardrail retry policy | `guardrail_max_retries` default 3, deprecated `max_retries` alias, per-guardrail retry counters | `lib/crewai/src/crewai/task.py:275-282,1332-1341` |
| Guardrail retry execution | Whole task re-run with validation error as context; exhaustion raises | `lib/crewai/src/crewai/task.py:1376-1408` |
| Duplicate detection | Compares current call to `last_used_tool` only (name + exact args) | `lib/crewai/src/crewai/tools/tool_usage.py:779-789`; `lib/crewai/src/crewai/agents/tools_handler.py:24,39` |
| Idempotency store | `CacheHandler`: in-memory dict keyed `f"{tool}-{input}"`, RWLock-guarded; failures refused | `lib/crewai/src/crewai/agents/cache/cache_handler.py:20-44,38-41` |
| Cache opt-in | `Crew.cache` defaults to `False` with an explicit warning about state-mutating tools | `lib/crewai/src/crewai/crew.py:229-238`; handler created at `lib/crewai/src/crewai/crew.py:632` |
| Cache write decision | Per-tool `cache_function` consulted before storing (default always-true) | `lib/crewai/src/crewai/tools/base_tool.py:81-82,176-182`; applied `lib/crewai/src/crewai/tools/tool_usage.py:376-391` |
| Usage-limit protection | Atomic `_claim_usage` under lock returns structured `ToolFailure(USAGE_LIMIT)` | `lib/crewai/src/crewai/tools/base_tool.py:302-324,267-272` |
| Usage-limit guard at invoke | `has_reached_max_usage_count()` checked in `invoke`/`ainvoke` with model-facing refusal text | `lib/crewai/src/crewai/tools/structured_tool.py:398-400,433-435,450-461` |
| Declarative failure signal | `ToolFailure.retryable` flag, `ToolFailureReason` taxonomy, `IGNORE/WARN/RAISE` policies | `lib/crewai/src/crewai/tools/tool_failure.py:35-108,57-68` |
| Failure record dedup | `merge_tool_failures` drops duplicates across guardrail-retry attempts by composite key | `lib/crewai/src/crewai/tools/tool_failure.py:211-234` |
| Failure isolation across retries | ContextVar-based `tool_failure_collector` so guardrail retries don't cross-contaminate records | `lib/crewai/src/crewai/tools/tool_failure.py:260-285` |
| Native-path divergence | Native function-calling path caches results but has NO auto-retry and NO repeated-usage check; exceptions become error strings | `lib/crewai/src/crewai/utilities/agent_utils.py:1700-1794` |
| MCP retry wrapper | Exponential backoff (2^attempt), max 3; classification: timeouts/network/json retryable, auth/not-found/ImportError terminal | `lib/crewai/src/crewai/tools/mcp_tool_wrapper.py:10-13,91-154` |
| Upload retry classification | `PermanentUploadError` stops immediately; `TransientUploadError` backs off; unknown treated transient | `lib/crewai-files/src/crewai_files/resolution/resolver.py:363-398` |
| Error classification helper | Status-code based: ≥500/429 transient; 401/403/400 permanent; type-name heuristics first | `lib/crewai-files/src/crewai_files/processing/exceptions.py:106-145` |
| Web search retry | 429 (minus quota codes) and 5xx retryable; quota exhaustion excluded; honors `Retry-After` header | `lib/crewai-tools/src/crewai_tools/tools/brave_search_tool/base.py:51-79` |
| A2A error classes | `is_retryable_error` = {INTERNAL_ERROR, RATE_LIMIT_EXCEEDED, TASK_TIMEOUT}; `retry_on_401` refreshes credentials between attempts | `lib/crewai/src/crewai/a2a/errors.py:464-478`; `lib/crewai/src/crewai/a2a/auth/utils.py:194-250` |
| LLM provider retries | `max_retries=2` delegated to OpenAI/Anthropic/Azure SDKs; Bedrock adaptive mode `max_attempts=3` | `lib/crewai/src/crewai/llms/providers/openai/completion.py:223,371-372`; `lib/crewai/src/crewai/llms/providers/anthropic/completion.py:221,300-301`; `lib/crewai/src/crewai/llms/providers/azure/completion.py:82,217-218`; `lib/crewai/src/crewai/llms/providers/bedrock/completion.py:316` |
| Semantic LLM retry | 400 rejecting `reasoning_effort`+tools retried once with `reasoning_effort="none"` | `lib/crewai/src/crewai/llms/providers/openai/completion.py:538-547,1730-1767` |
| Rate limiting (not retry) | `RPMController` blocks until next minute rather than retrying | `lib/crewai/src/crewai/utilities/rpm_controller.py:38-64` |
| Storage-layer retry | LanceDB write retries on commit conflicts with exponential delay | `lib/crewai/src/crewai/memory/storage/lancedb_storage.py:36-39,129-150` |
| Model-visible duplicate refusal | `"I tried reusing the same input, I must stop using this action input..."` returned as tool result | `lib/crewai/src/crewai/translations/en.json:49`; used at `lib/crewai/src/crewai/tools/tool_usage.py:256-265,512-521` |
| Model-visible exhausted retries | `tool_usage_exception` text "... Moving on then." returned after attempt budget spent | `lib/crewai/src/crewai/translations/en.json:53`; `lib/crewai/src/crewai/tools/tool_usage.py:460-469` |
| Parse-error retry visibility | OutputParserError appended to messages as user role; verbose printer note after N iterations | `lib/crewai/src/crewai/utilities/agent_utils.py:763-776` |
| Guardrail retry visibility | Printer message "(attempt X/Y), retrying due to:" + error fed back as task context | `lib/crewai/src/crewai/task.py:1394-1402` |
| Observability events | `ToolUsageErrorEvent`, `ToolFailureDetectedEvent` emitted to event bus; telemetry on repeated usage/errors | `lib/crewai/src/crewai/utilities/agent_utils.py:1773-1785`; `lib/crewai/src/crewai/tools/tool_failure.py:355-379`; `lib/crewai/src/crewai/tools/tool_usage.py:260-264,334-337` |
| Tests: failures not cached | "A cached failure would make a transient error permanent"; repeated failures recorded once each | `lib/crewai/tests/tools/test_tool_failure.py:915-953` |
| Tests: usage limits | Limit enforcement through `ToolUsage._check_usage_limit` and reset behavior | `lib/crewai/tests/tools/test_tool_usage_limit.py:95-151` |
| Tests: HTTP retry semantics | 429 retries once then succeeds; persistent 429 exhausts 3 attempts raises; exponential backoff asserted | `lib/crewai-tools/tests/tools/brave_search_tool_test.py:694-771` |
| Tests: task-level retry config/events | `max_retry_limit` values exercised incl. event emission on failure | `lib/crewai/tests/utilities/test_events.py:354-458`; `lib/crewai/tests/agents/test_agent.py:1292-1318` |
| Non-idempotent tool authorship | Daytona sandbox tool documents that `create_folder` "is not idempotent on the server" and swallows repeat errors | `lib/crewai-tools/src/crewai_tools/tools/daytona_sandbox_tool/daytona_file_tool.py:391-410` |

## Answers to Dimension Questions

**1. Which tool failures are retried?**
On the ReAct path (`ToolUsage._use`/`_ause`), *any* exception escaping the tool invocation is retried with identical arguments until the shared `_run_attempts` budget (`_max_parsing_attempts`, 3 or 2) is exhausted (`lib/crewai/src/crewai/tools/tool_usage.py:451-479,708-736`) — there is no classification of which exception types deserve retry. Declared `ToolFailure` returns are explicitly *not* retried (detection happens post-invocation, `lib/crewai/src/crewai/tools/tool_failure.py:154-162`); they follow the IGNORE/WARN/RAISE policy instead (`lib/crewai/src/crewai/tools/tool_failure.py:324-382`). At the task layer, everything except `litellm`-module exceptions gets a full-task retry bounded by `max_retry_limit` (`lib/crewai/src/crewai/agent/core.py:731-747`). The native function-calling path retries nothing automatically (`lib/crewai/src/crewai/utilities/agent_utils.py:1767-1786`).

**2. Are repeated attempts safe?**
Only partially. Three safeguards exist: (a) exact-duplicate refusal against the immediately preceding call (`lib/crewai/src/crewai/tools/tool_usage.py:779-789`) — though this fires *before* the retry recursion, it does not block retries because failed calls never update `last_used_tool` (`lib/crewai/src/crewai/agents/tools_handler.py:39`); (b) opt-in result cache short-circuiting identical calls (`lib/crewai/src/crewai/tools/tool_usage.py:301-312`); (c) `max_usage_count` caps enforced atomically even across threads (`lib/crewai/src/crewai/tools/base_tool.py:302-324`). None of these distinguish a side-effect-free tool from a mutating one, and the cache is disabled unless the user opts in (`lib/crewai/src/crewai/crew.py:229-238`).

**3. Is retry state persisted?**
No evidence found of persistence. All retry state is in-memory and process-local: `CacheHandler._cache` is a plain dict (`lib/crewai/src/crewai/agents/cache/cache_handler.py:20`), usage counters are pydantic fields on tool instances (`lib/crewai/src/crewai/tools/base_tool.py:184-195`), and `_times_executed`, `_run_attempts`, `retry_count`, and `_guardrail_retry_counts` are runtime attributes (`lib/crewai/src/crewai/agent/core.py:745`, `lib/crewai/src/crewai/tools/tool_usage.py:109`, `lib/crewai/src/crewai/task.py:282,304`). A crash loses all deduplication context.

**4. Are non-idempotent tools protected?**
Weakly, and only by convention. The framework offers per-tool escape hatches — `cache_function` to veto caching (`lib/crewai/src/crewai/tools/base_tool.py:176-182`), `max_usage_count` hard caps (`lib/crewai/src/crewai/tools/base_tool.py:184-195`), and returning `ToolFailure` to avoid auto-retry — but nothing marks a tool as mutating, and the `Crew.cache` description pushes the burden onto users ("do not enable for live-data or state-mutating tools unless they set a cache_function...", `lib/crewai/src/crewai/crew.py:232-236`). Tool authors sometimes handle this themselves, e.g., Daytona's file tool documenting server-side non-idempotency of `create_folder` (`lib/crewai-tools/src/crewai_tools/tools/daytona_sandbox_tool/daytona_file_tool.py:395-397`). The `ToolFailure.retryable` field exists (`lib/crewai/src/crewai/tools/tool_failure.py:95-98`) but no core code consumes it for decisions — searches found it only set/read within `tool_failure.py` itself.

**5. Can retries create duplicate side effects?**
Yes, through two concrete mechanisms. First, blind exception retries: a mutating tool that fails *after* committing (network drop on response read) is re-invoked up to 3 times with identical arguments (`lib/crewai/src/crewai/tools/tool_usage.py:476-479,497-499`). Second, guardrail retries re-run the whole task including all its tools, so a successful-but-blocked run's side effects are repeated wholesale (`lib/crewai/src/crewai/task.py:1403-1408`); only the *failure records* are deduplicated afterward, not the effects. Mitigations are opt-in: cache hits skip re-execution entirely when enabled (`lib/crewai/src/crewai/tools/tool_usage.py:301-312`), and `max_usage_count=1` would hard-stop a second call. The MCP wrapper compounds exposure differently: it retries network/timeout errors against external servers transparently (`lib/crewai/src/crewai/tools/mcp_tool_wrapper.py:139-153`), so a mutating remote tool behind a flaky connection can fire multiple times inside one logical call.

## Architectural Decisions

- **Retry at multiple scopes instead of one**: tool-call retries (attempt budget), task retries (`max_retry_limit`), guardrail retries (`guardrail_max_retries`), and transport retries (SDK/wrapper level) are independent layers with separate counters. No global budget ties them together.
- **Litellm exceptions are terminal at the agent layer**: `_check_execution_error` re-raises anything whose class module starts with `litellm` (`lib/crewai/src/crewai/agent/core.py:743-744`), delegating transport-level retries to the provider SDK's own `max_retries` configuration (`lib/crewai/src/crewai/llms/providers/openai/completion.py:371-372`). This avoids double-retrying the same request.
- **Declarative over heuristic failure detection**: "Detection is strictly declarative -- nothing here guesses whether a string 'looks like' an error" (`lib/crewai/src/crewai/tools/tool_failure.py:8-11,154-162`). This prevents false-positive retry/suppression cycles from string matching.
- **Failures are never cached**: `CacheHandler.add` silently refuses `ToolFailure` outputs so a transient error cannot become permanent via a cache hit (`lib/crewai/src/crewai/agents/cache/cache_handler.py:23-41`), backed by tests stating that rationale verbatim (`lib/crewai/tests/tools/test_tool_failure.py:916-923`).
- **Cache is opt-in with a documented mutation hazard**: the `Crew.cache=False` default and its field description encode the tradeoff explicitly rather than solving it (`lib/crewai/src/crewai/crew.py:229-238`).
- **Structured usage limits, atomically claimed**: `_claim_usage` performs check-and-increment under a private lock and returns a `ToolFailure` so every path records a spent limit (`lib/crewai/src/crewai/tools/base_tool.py:302-309`).
- **Error classification lives at integration boundaries, not the core**: files uploads (`lib/crewai-files/src/crewai_files/processing/exceptions.py:106-145`), Brave Search (`lib/crewai-tools/src/crewai_tools/tools/brave_search_tool/base.py:51-64`), and A2A (`lib/crewai/src/crewai/a2a/errors.py:464-478`) classify retryability locally; the core tool loop does not.

## Notable Patterns

- **Sync/async mirror duplication**: `_use` and `_ause` are near-identical ~250-line implementations of the same retry/caching logic (`lib/crewai/src/crewai/tools/tool_usage.py:503-758` vs `238-501`) — every semantic change must land twice.
- **Policy resolution chain**: most-specific-wins lookup across tool → wrapped tool → task → agent → crew with graceful degradation on malformed values (`resolve_tool_failure_policy`, `lib/crewai/src/crewai/tools/tool_failure.py:177-208`).
- **ContextVar-scoped failure collection**: `tool_failure_collector` gives each concurrent execution its own record list, safe under nesting because "a guardrail retry can open its own scope inside the outer one" (`lib/crewai/src/crewai/tools/tool_failure.py:265-280`).
- **Server-hint-aware backoff**: Brave Search prefers the `Retry-After` header over local exponential backoff (`lib/crewai-tools/src/crewai_tools/tools/brave_search_tool/base.py:67-79`); `retry_on_401` refreshes credentials between attempts rather than blind-retrying (`lib/crewai/src/crewai/a2a/auth/utils.py:240-246`).
- **Semantic single-shot LLM retry**: the OpenAI provider detects a specific recoverable 400 (reasoning_effort rejected with tools) and retries once with corrected params, carefully avoiding emitting a failure event for the call that will succeed (`lib/crewai/src/crewai/llms/providers/openai/completion.py:2035-2038`).
- **Blocking instead of retrying for rate limits**: `RPMController.check_or_wait` sleeps until the next minute window rather than failing and re-entering a retry loop (`lib/crewai/src/crewai/utilities/rpm_controller.py:47-58`).
- **Model-facing refusal texts as i18n keys**: duplicate-usage and exhausted-retry messages are prompt-engineered strings returned into the conversation (`lib/crewai/src/crewai/translations/en.json:49,53`), steering the model away from repeating inputs.

## Tradeoffs

- **Blind tool retry favors availability over safety**: re-invoking on any exception recovers transient parse/validation hiccups without user intervention, but cannot distinguish "call never reached the server" from "server executed then response was lost". No idempotency key is attached that could make the redelivery safe.
- **Opt-in cache favors correctness of live data over dedup safety**: defaulting `Crew.cache=False` avoids stale reads, but leaves the strongest duplicate-suppression mechanism disabled in the default configuration where it would matter most for mutating tools.
- **Last-call-only duplicate detection is cheap but shallow**: comparing only `last_used_tool` catches immediate loops yet misses alternating sequences (A,B,A,B) or repeats after any intervening call (`lib/crewai/src/crewai/tools/tool_usage.py:784-789`).
- **Whole-task guardrail retries maximize repair probability at the cost of effect duplication**: re-running `execute_task` lets the agent redo everything with feedback, but already-succeeded side effects recur; only bookkeeping (failure records) survives deduplication (`lib/crewai/src/crewai/task.py:1339-1341,1435`).
- **Transport-level classification is good but non-uniform**: each wrapper invents its own retryable sets and backoff constants (MCP: 3 tries/2^n s; uploads: `UPLOAD_MAX_RETRIES`; Brave: 3 tries honoring `Retry-After`), so behavior differs by subsystem.
- **Descriptive-vs-operational `retryable` flag**: capturing intent in the schema (`lib/crewai/src/crewai/tools/tool_failure.py:95-98`) builds toward automated safe retries, but today it informs humans/traces, not the executor.

## Failure Modes / Edge Cases

- **Double execution of mutating tools on post-commit exceptions** (ReAct path): described above; highest-severity gap for payment/email/delete style tools.
- **Path-dependent safety**: identical code is auto-retried on the ReAct path but never retried on the native function-calling path (`lib/crewai/src/crewai/utilities/agent_utils.py:1767-1770`), and the native path also lacks the repeated-usage refusal — a user switching execution modes silently changes retry semantics.
- **Duplicate detection bypass**: alternating or spaced-out identical calls evade `last_used_tool` comparison; argument serialization differences (dict ordering aside — `json.dumps` is deterministic here, but `str(calling.arguments)` fallback is not canonical) can defeat cache-key equality (`lib/crewai/src/crewai/agents/tools_handler.py:40-51`).
- **Retry budget is shared across distinct errors**: `_run_attempts` counts all exceptions cumulatively for the ToolUsage instance, so three unrelated one-off failures exhaust the budget just like one persistent bug (`lib/crewai/src/crewai/tools/tool_usage.py:109,454-455`).
- **MCP wrapper masks retry activity**: the caller/model sees one final string after up to 3 internal attempts (`lib/crewai/src/crewai/tools/mcp_tool_wrapper.py:113-115`), so downstream observers cannot tell how many times the remote tool actually ran.
- **String-matching classification fragility**: MCP retryability hinges on substrings like `"connection"` or `"json"` in lowercased error text (`lib/crewai/src/crewai/tools/mcp_tool_wrapper.py:139-153`), and upload classification on exception type names (`lib/crewai-files/src/crewai_files/processing/exceptions.py:119-128`) — both break silently when provider messages change wording.
- **Unknown exceptions default to retryable** in upload resolution (`lib/crewai-files/src/crewai_files/resolution/resolver.py:384-386`), biasing toward redundant attempts of possibly-committed operations.
- **Guardrail retry counter asymmetry**: legacy scalar `retry_count` and per-index `_guardrail_retry_counts` coexist (`lib/crewai/src/crewai/task.py:1332-1340`), so multi-guardrail tasks track budgets separately while old code observes only the aggregate.

## Future Considerations

- Make `ToolFailure.retryable` operational: have `ToolUsage` consult it (or a tool-declared `idempotent: bool`) before setting `should_retry=True`, closing the post-commit double-execution hole.
- Introduce optional idempotency keys for tool calls (e.g., hash of tool name + canonicalized args surfaced to tools as a header/kwarg), enabling downstream APIs to deduplicate safely.
- Persist minimal retry/dedup state (usage counters, recent call fingerprints) alongside existing task-output storage so resumed runs inherit budgets.
- Unify the ReAct and native paths on one execution routine (or extract shared retry/cache middleware) to eliminate divergent semantics between `tool_usage.py` and `agent_utils.py`.
- Standardize transport retry classification into a shared helper (status-code table + exception-type table + `Retry-After` handling) reused by MCP, uploads, search, and A2A.
- Emit an explicit event per automatic retry attempt (currently only telemetry aggregates exist, e.g., `attempts=self._run_attempts` at `lib/crewai/src/crewai/tools/tool_usage.py:260-264`) so trace UIs can show hidden re-invocations.

## Questions / Gaps

- **No idempotency-key infrastructure**: searched `idempot*` across the entire source tree; matches were docs snapshots, tracing/script idempotence notes, and one tool-author comment (`lib/crewai-tools/src/crewai_tools/tools/daytona_sandbox_tool/daytona_file_tool.py:397`). No request-id/key plumbing exists.
- **Who consumes `retryable`?** Grep shows the field defined and defaulted in `tool_failure.py` only; no executor branch was found. Intent appears forward-looking.
- **Persistence boundary unverified beyond code reading**: I did not find any serialization of `CacheHandler`/counters, but a full audit of every storage backend (e.g., `utilities/file_store.py`) was out of scope; the claim "never persisted" rests on the cited in-memory implementations.
- **Hierarchical-process delegation retries**: whether manager-agent delegation failures retry through the same `max_retry_limit` path was not independently traced end-to-end; evidence covers the generic task loop only.
- **Flow (`@flow`) execution retry semantics** were not analyzed in depth; the study focused on crew/agent/tool layers where the dimension's evidence targets concentrate.

---

Generated by `Dimension 07.03: Idempotency and Retry Semantics` against `crewai`.
