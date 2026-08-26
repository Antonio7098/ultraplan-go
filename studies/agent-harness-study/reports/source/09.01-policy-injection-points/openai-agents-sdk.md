# Source Analysis: openai-agents-sdk

## Dimension 09.01: Policy Injection Points

### Source Info

| Field | Value |
|-------|-------|
| Name | openai-agents-sdk |
| Path | `studies/agent-harness-study/sources/openai-agents-sdk` |
| Language / Stack | Python 3.10+ (asyncio, pydantic, dataclasses; OpenAI Responses/Chat Completions APIs) |
| Analyzed | 2026-08-26 |

## Summary

The OpenAI Agents SDK has no single "policy engine"; instead, governance rules enter the system through five distinct injection points, all expressed as Python callables or declarative fields attached at construction time:

1. **Agent-level guardrails** — `InputGuardrail`/`OutputGuardrail` objects (built via `@input_guardrail`/`@output_guardrail` decorators) attached to the `Agent.input_guardrails` / `Agent.output_guardrails` lists (`src/agents/guardrail.py:72`, `src/agents/agent.py:350`, `src/agents/agent.py:355`). They enforce a binary tripwire model: a guardrail function returns `GuardrailFunctionOutput` whose `tripwire_triggered` flag halts execution (`src/agents/guardrail.py:19-32`).
2. **Run-level (cross-cutting) guardrails** — `RunConfig.input_guardrails` and `RunConfig.output_guardrails` (`src/agents/run_config.py:391-395`) that are concatenated onto agent-level lists at run time.
3. **Tool metadata annotations** — per-tool policy attributes on `FunctionTool`: `needs_approval` (bool or per-call callable), `is_enabled` (dynamic gating), and tool-scoped `tool_input_guardrails`/`tool_output_guardrails` (`src/agents/tool.py:472-493`).
4. **MCP server approval policies** — `MCPServer(require_approval=...)` accepting `"always"`/`"never"` literals, per-tool name maps, `{always:{tool_names}, never:{tool_names}}` list schemas, or callables, normalized at server construction (`src/agents/mcp/server.py:548-576`, `src/agents/mcp/server.py:709-813`).
5. **Prompt-injected behavioral rules** — e.g. the sandbox remote-mount policy template rendered into system instructions with an allowlist of permitted commands (`src/agents/sandbox/remote_mount_policy.py:8-18`, `30-46`), plus OpenAI hosted prompts referenced by id with an optional version (`src/agents/prompts.py:17-24`).

Runtime configuration knobs add operational policy: sandbox archive/concurrency limits with constructor-time validation (`src/agents/run_config.py:161-215`), tool-name collision policy (`run vs error`, `src/agents/run_config.py:480-487`), and trace data-exposure policy via env vars (`OPENAI_AGENTS_TRACE_INCLUDE_SENSITIVE_DATA` at `src/agents/run_config.py:53-56`; `OPENAI_AGENTS_DISABLE_TRACING` at `src/agents/tracing/provider.py:346-356`).

There is no external policy-engine integration (no OPA/Cedar/CEL hooks found) and no hot-reload mechanism: policies are fixed for the lifetime of the constructed object graph. However, because every policy hook is a callable receiving the live `RunContextWrapper`, applications can delegate decisions to arbitrary external state (feature flags, databases) inside the callback without modifying SDK code.

Precedence is deliberately additive rather than overriding: agent and RunConfig guardrails both run, and the first tripwire cancels siblings. Conflicts are mostly prevented structurally — MCP always/never overlap is rejected with `UserError` at construction (`src/agents/mcp/server.py:783-788`), and output guardrails are rejected up-front when combined with server-managed conversations they cannot support (`src/agents/run_internal/agent_runner_helpers.py:263-275`).

Auditing is achieved through tracing (every guardrail executes inside a dedicated `guardrail` span recording its triggered status, `src/agents/tracing/span_data.py:292-306`) and through a durable approval ledger serialized into resumable, schema-versioned `RunState` payloads (`src/agents/run_state.py:1300-1324`, `CURRENT_SCHEMA_VERSION = "1.17"` at `src/agents/run_state.py:182`). There is no dedicated policy-version registry; identity is the guardrail's name string.

## Rating

**7 / 10** — Clear model with tests, explicit interfaces, and operational safeguards.

Rationale:
- The policy surface is explicit and typed (`InputGuardrail`, `ToolGuardrailFunctionOutput` behaviors, `RequireApprovalSetting`), enforced at well-defined pipeline points, and heavily covered by tests (`tests/test_guardrails.py`, `tests/test_tool_guardrails.py`, `tests/test_run_internal_approvals.py`).
- Operational safeguards are real: fail-closed behavior when an MCP callable policy cannot be evaluated (`src/agents/mcp/server.py:822-831`), construction-time conflict validation, sibling cancellation on tripwire, and blocking-guardrail-before-sandbox-prep ordering (`src/agents/run_internal/run_loop.py:1202-1233`).
- It falls short of 9-10 because policies are code-bound (no external policy file/engine, no runtime reload), there is no policy content versioning or change audit beyond tracing spans, and precedence between overlapping sources is "all of the above" concatenation rather than an explicitly documented conflict-resolution model.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Input/output guardrail contract | `GuardrailFunctionOutput.tripwire_triggered` halts execution; decorators create guards from functions | `src/agents/guardrail.py:19-32`, `src/agents/guardrail.py:224-270`, `src/agents/guardrail.py:305-343` |
| Parallel vs blocking input guardrails | `run_in_parallel: bool = True` field selects concurrent-vs-blocking mode | `src/agents/guardrail.py:100-103`; docs `docs/guardrails.md:32-38` |
| Agent attachment point | `Agent.input_guardrails` / `Agent.output_guardrails` list fields validated at construction | `src/agents/agent.py:350-358`, `src/agents/agent.py:497-505` |
| Run-level injection point | `RunConfig.input_guardrails` / `RunConfig.output_guardrails` | `src/agents/run_config.py:391-395` |
| Tool metadata annotations | `FunctionTool.is_enabled` (bool/callable), `tool_input_guardrails`, `tool_output_guardrails`, `needs_approval` (bool/callable) | `src/agents/tool.py:472-493` |
| Tool guardrail behavior model | `allow` / `reject_content(message)` / `raise_exception` TypedDicts + factory classmethods | `src/agents/tool_guardrails.py:40-117` |
| Tool guardrail decorators | `@tool_input_guardrail` / `@tool_output_guardrail` | `src/agents/tool_guardrails.py:228-243`, `264-279` |
| MCP approval policy normalization | `require_approval` accepts "always"/"never", name maps, tool-list schemas, callables; normalized once at server init | `src/agents/mcp/server.py:548-576`, `src/agents/mcp/server.py:709-813` |
| MCP fail-closed safeguard | Callable policy without an available agent yields `needs_approval=True` | `src/agents/mcp/server.py:815-846` |
| Approval decision evaluation helper | Bool-or-callable resolution with strict invalid-value error | `src/agents/util/_approvals.py:32-51` |
| Precedence: agent+RunConfig concat (input) | `starting_agent.input_guardrails + (run_config.input_guardrails or [])`, turn 0 only | `src/agents/run_internal/run_loop.py:1192-1196` |
| Precedence: agent+RunConfig concat (output) | `agent.output_guardrails + (run_config.output_guardrails or [])` in streaming and non-streaming paths | `src/agents/run_internal/run_loop.py:454`, `762-763` |
| Tripwire wins / sibling cancellation | First triggered result cancels remaining tasks and raises `InputGuardrailTripwireTriggered` | `src/agents/run_internal/guardrails.py:144-168`, `200-222` |
| Blocking guards before sandbox prep | Sequential first-turn guardrails run before sandbox session creation can mutate state | `src/agents/run_internal/run_loop.py:1202-1233` |
| Guardrail scope limitation | Input guardrails only on turn 0 of fresh runs; re-run for pending input after interruptions | `src/agents/run_internal/run_loop.py:1192-1196`, `1461-1481` |
| Up-front structural validation | Output guardrails rejected with server-managed conversations (`validate_output_guardrails_with_server_managed_conversation`) | `src/agents/run_internal/agent_runner_helpers.py:263-275`; invoked at `src/agents/run_internal/run_loop.py:1185-1191` |
| Approval-before-guardrail ordering (default) | Approval status checked first; pre-approval tool-input guardrails opt-in via `ToolExecutionConfig.pre_approval_tool_input_guardrails` | `src/agents/run_internal/tool_execution.py:1884-1952`, `2044-2048`; config at `src/agents/run_config.py:146-150` |
| Post-approval guardrail re-check | Input guardrails run again immediately before invocation after approval | `src/agents/run_internal/tool_execution.py:2012-2020` |
| Tool output guardrails post-invoke | Output guardrails wrap the real tool result; rejection bypasses output schema | `src/agents/run_internal/tool_execution.py:2106-2125` |
| Sequential tool guardrail evaluation | Guards iterate in list order; first `raise_exception` throws, first `reject_content` short-circuits | `src/agents/run_internal/tool_execution.py:2705-2724`, `2741-2765` |
| Prompt-based policy text | `REMOTE_MOUNT_POLICY` template with command allowlist injected into instructions from manifest | `src/agents/sandbox/remote_mount_policy.py:8-18`, `30-46` |
| Hosted prompt versioning | `Prompt` TypedDict carries `id` plus optional `version` | `src/agents/prompts.py:17-24` |
| Trace-audit of guardrails | Each guardrail runs inside `guardrail_span` recording `triggered`; tripwires attach `SpanError` to parent spans | `src/agents/run_internal/guardrails.py:37-40`, `85-96`, `151-157`; span type at `src/agents/tracing/span_data.py:292-306` |
| Env-var trace policy | `OPENAI_AGENTS_TRACE_INCLUDE_SENSITIVE_DATA` default true; `OPENAI_AGENTS_DISABLE_TRACING` read once, manual override wins | `src/agents/run_config.py:53-56`, `404-410`; `src/agents/tracing/provider.py:339-356` |
| Durable approval ledger | `RunState.approve()/reject()` mutate context records; `_serialize_approvals()` persists decisions incl. sticky scopes into resumable state | `src/agents/run_state.py:1255-1298`, `1300-1324`; storage at `src/agents/run_context.py:89` |
| State schema versioning | `CURRENT_SCHEMA_VERSION = "1.17"`, hosted-MCP approvals min schema "1.14", versioned summaries | `src/agents/run_state.py:176-225` |
| Sandbox resource limits as policy | Validated `SandboxConcurrencyLimits` / `SandboxArchiveLimits` defaults | `src/agents/run_config.py:45-50`, `161-215` |
| Handoff filter precedence | Per-handoff `Handoff.input_filter` takes precedence over `RunConfig.handoff_input_filter` | `src/agents/run_config.py:366-372` |
| Tests: intended behavior | Parallel/blocking modes, decorator forms, invalid guardrails raise `UserError` | `tests/test_guardrails.py:298-330`; `tests/test_tool_guardrails.py:88-391` |
| Examples: human-in-the-loop usage | Approval interruption/resume pattern demonstrated | `examples/agent_patterns/human_in_the_loop.py`, `examples/tools/tool_guardrails.py` |

## Answers to Dimension Questions

### 1. Where do governance rules live?

In code, attached to four host objects, plus prompt text:

- **Agent definition**: guardrail lists on the `Agent` dataclass (`src/agents/agent.py:350`, `355`).
- **Run configuration**: cross-cutting guardrails and behavior knobs on `RunConfig` (`src/agents/run_config.py:391-395`, `472-496`).
- **Tool metadata**: `FunctionTool.needs_approval`, `is_enabled`, and tool-scoped guardrail lists (`src/agents/tool.py:472-493`); equivalent approval fields exist on shell/apply-patch/custom tools (`src/agents/tool.py:1368`, `1423`, `1463`) and hosted variants forbid them (`src/agents/tool.py:1405-1409`).
- **Server connection config**: MCP `require_approval` policies (`src/agents/mcp/server.py:548-576`).
- **Prompt**: sandbox remote-mount rules rendered from the manifest into system instructions (`src/agents/sandbox/remote_mount_policy.py:8-46`). This is advisory (prompt-level), complemented by hard enforcement elsewhere (e.g., read-only mount semantics come from manifest data, not the prompt).

No evidence found of policy definitions in eval configuration or deployment manifests (beyond the two tracing env vars); the search boundary was `src/agents/**` plus docs/examples.

### 2. Can policies be updated at runtime?

Not by replacing policy objects mid-run — guardrail lists, `require_approval`, and limits are captured at Agent/RunConfig/MCPServer construction. Two dynamic mechanisms exist:

- **Callable policies evaluated per event**: `needs_approval` may be a function receiving `(run_context, tool_parameters, call_id)` per invocation (`src/agents/tool.py:486-493`, resolved by `src/agents/util/_approvals.py:32-51`); MCP callable policies receive `(run_context, agent, tool)` (`src/agents/mcp/server.py:829-841`); `is_enabled` gates tools dynamically (`src/agents/tool.py:472-475`). These callbacks can consult external, mutable sources of truth each time, giving effective runtime policy updates without SDK changes.
- **One-shot environment flags**: tracing disablement is read once from `OPENAI_AGENTS_DISABLE_TRACING` on first use, after which only the manual override applies ("further env changes are ignored", `src/agents/tracing/provider.py:339-356`).

There is no file-watcher, no remote-config subscription, and no API to swap guardrail lists on a running loop. Note that input guardrails are bound at specific points (turn 0, `src/agents/run_internal/run_loop.py:1192-1196`), so mutating an agent's list mid-stream would not reliably take effect anyway.

### 3. What happens when policies conflict?

- **Additive union, not override**: agent-level and RunConfig guardrails are concatenated and all execute (`src/agents/run_internal/run_loop.py:1192-1196`, `454`, `762-763`). The strictest outcome wins operationally: the first tripwire cancels sibling guardrail tasks and raises (`src/agents/run_internal/guardrails.py:144-158`).
- **Ordering conflicts resolved by fixed pipeline order**: approvals are evaluated before tool guardrails by default; pre-approval guardrail execution is strictly opt-in (`src/agents/run_config.py:146-150`, enforced at `src/agents/run_internal/tool_execution.py:1907-1952`), and guardrails re-run post-approval before actual execution (`tool_execution.py:2012-2020`).
- **Construction-time rejection**: MCP `always`/`never` overlap raises `UserError` naming the offending tools (`src/agents/mcp/server.py:783-788`); invalid `needs_approval` value types raise `UserError` (`src/agents/util/_approvals.py:46-50`); empty-string blocked-message formatters are rejected (`src/agents/run_config.py:534-545`).
- **Incompatible combinations refused up front**: output guardrails + server-managed conversations abort before the loop starts (`src/agents/run_internal/agent_runner_helpers.py:263-275`).
- **Name collision policy**: duplicate tool/handoff names default to warn-and-shadow with an optional `"error"` escalation (`src/agents/run_config.py:480-487`).
- Within one tool's guardrail list, evaluation is sequential and first decisive verdict short-circuits (`src/agents/run_internal/tool_execution.py:2705-2724`).

There is no generic precedence declaration (e.g., priority weights); ordering is implicit in concatenation and call sites.

### 4. Are policy changes audited?

Partially:

- **Execution audit**: every guardrail invocation emits a `guardrail` span with name and triggered status (`src/agents/run_internal/guardrails.py:37-40`, `43-52`; `src/agents/tracing/span_data.py:292-306`), and tripwires attach structured `SpanError` data to the parent span (`src/agents/run_internal/guardrails.py:85-96`, `207-213`). Sensitive-content inclusion in traces is itself policy-controlled (`src/agents/run_config.py:404-410`).
- **Decision audit**: approve/reject decisions are recorded per tool in `RunContextWrapper._approvals` (`src/agents/run_context.py:89`) and serialized — including sticky scopes and rejection messages — into resumable `RunState` (`src/agents/run_state.py:1300-1324`). Who approved is not captured; the ledger records outcomes, not actor identity.
- **Change audit**: none. There is no record of when a guardrail/policy was added or modified, no policy content hash, and no diff history inside the runtime. Versioning exists only indirectly: hosted prompts carry an optional `version` (`src/agents/prompts.py:17-24`) and persisted approval state is schema-versioned (`src/agents/run_state.py:182`, `184`).

## Architectural Decisions

1. **Policies as first-class typed objects, not strings/config files.** Guardrails are dataclasses wrapping callables (`src/agents/guardrail.py:71-93`, `133-152`), giving full language expressivity but binding policy lifecycle to application deploys.
2. **Two-layer injection (per-agent + per-run).** `RunConfig` duplicates the guardrail surface so platform owners can impose org-wide checks without touching agent authors' definitions (`src/agents/run_config.py:391-395`), combined additively at `src/agents/run_internal/run_loop.py:1192-1196`.
3. **Three-valued tool-guardrail verdicts.** Unlike binary agent tripwires, tool guardrails distinguish soft rejection (message back to the model, run continues) from hard halt (`src/agents/tool_guardrails.py:40-77`), enabling graceful policy enforcement inside multi-turn loops.
4. **Human approval as interruptible state, not inline callback-only.** Approvals materialize as `ToolApprovalItem` interruptions; decisions persist in serializable `RunState` so a process can die between request and approval and resume (`src/agents/run_state.py:1255-1298`).
5. **Fail-closed defaults where policy can't be evaluated.** An MCP callable approval policy without an agent context resolves to `True` (approval required) (`src/agents/mcp/server.py:822-831`); unparseable JSON arguments make approval inspection return `None` rather than guessing (`src/agents/util/_approvals.py:18-29`).
6. **Prompt-level guidance kept separate from enforcement.** Remote-mount rules are advisory prompt text derived deterministically from the trusted manifest (`src/agents/sandbox/remote_mount_policy.py:30-46`), while mount authority/secret redaction is enforced in code (`src/agents/sandbox/_mount_security.py:155-160`).

## Notable Patterns

- **Decorator-based policy authoring**: `@input_guardrail(name=..., run_in_parallel=False)` mirrors `@function_tool`, keeping policy definitions adjacent to business code (`src/agents/guardrail.py:238-270`).
- **Partitioned scheduling**: input guardrails split into sequential (blocking) and parallel cohorts from one flag, letting cheap fast checks block expensive runs while slow LLM-judge checks run concurrently (`src/agents/run_internal/run_loop.py:1197-1198`).
- **Result-sink accumulation**: guardrail results are appended to caller-owned sinks as each completes, so even a raising run reports partial results (`src/agents/run_internal/guardrails.py:122-141`, docstring at 124-127).
- **Data-free placeholder on rejection**: terminal outputs blocked by output guardrails are replaced with a configurable data-free message, and the formatter is deliberately synchronous to keep the redaction boundary safe (`src/agents/run_config.py:125-132`, `489-496`).
- **Schema-versioned durable policy state**: `SCHEMA_VERSION_SUMMARIES` documents what each persisted-state version means, gating features like hosted-MCP approvals behind minimum versions (`src/agents/run_state.py:176-225`).

## Tradeoffs

- **Expressivity vs operability**: Python-callable policies are maximally flexible but require a code deploy to change; there is no YAML/env policy surface for non-developers (contrast with external engines like OPA). Mitigation: callables can proxy any external decision service.
- **Additive precedence vs predictability**: running both agent- and run-level guardrails guarantees no check is silently dropped, but total enforcement cost grows and "which policy blocked me?" requires inspecting results rather than consulting one authoritative rule.
- **Latency vs safety in guardrail placement**: parallel-by-default optimizes latency at the cost of possible wasted work/side effects before a tripwire lands; blocking mode trades latency for guaranteed pre-execution stop (`docs/guardrails.md:36-38`).
- **Approval-first ordering**: checking approvals before guardrails minimizes user friction but means unvetted inputs skip guardrails unless `pre_approval_tool_input_guardrails=True` doubles their execution cost (`src/agents/run_config.py:146-150`).
- **Name-based identity**: guardrails are identified by name string for tracing (`src/agents/guardrail.py:105-109`); renaming silently breaks trace continuity, and duplicate names across guards are not disambiguated.

## Failure Modes / Edge Cases

- **Guardrail exception propagation**: a raising guardrail function fails the run after cancelling siblings, rather than being treated as pass/fail (`src/agents/run_internal/guardrails.py:159-166`) — availability risk if a flaky dependency backs a guardrail.
- **Turn-0 scoping**: input guardrails do not re-run on later turns of a multi-turn loop except for pending-input resumes (`src/agents/run_internal/run_loop.py:1192-1196`, `1461-1481`); handoff-driven turns are not re-checked against the original input policy.
- **Uninspectable arguments**: if tool-call JSON fails to parse, approval callbacks receive `None` parameters (`src/agents/util/_approvals.py:18-29`) — policies must handle that shape or mis-decide.
- **Hosted-tool mismatch**: setting `needs_approval` on a hosted-environment shell tool raises at construction (`src/agents/tool.py:1405-1409`), catching a policy that could never be honored.
- **Env-flag freeze**: tracing policy read from the environment once per process can surprise operators who flip the variable expecting immediate effect (`src/agents/tracing/provider.py:340-356`).
- **Streaming race containment**: early streamed guardrail failures cancel the run-loop task and push completion sentinels to avoid hangs (`src/agents/run_internal/guardrails.py:99-112`).

## Future Considerations

- Add an optional declarative policy source (file/URL) with checksum + load-time validation to support ops-managed rules without code deploys.
- Attach stable ids/content hashes and versions to guardrails so trace audits can attribute outcomes to specific policy revisions, not just names.
- Expose explicit precedence declarations (e.g., severity/order weights) between agent-, run-, and tool-level policies instead of implicit concatenation order.
- Record actor identity and timestamps in the serialized approval ledger for stronger compliance auditing (`src/agents/run_state.py:1300-1324` currently stores outcomes only).
- Generalize the `pre_approval_tool_input_guardrails` double-execution tradeoff into per-guardrail placement hints rather than a global boolean.

## Questions / Gaps

- No evidence found of external policy-engine integration (searched `src/` for OPA/policy-engine references; nothing). If deployments need centralized policy, it must be built on the callable hooks.
- No evidence found of policy versioning or change auditing within the runtime; whether the surrounding platform (e.g., trace exporter backend) compensates is outside this repository.
- Whether output guardrails' interaction with resumed (`RunState`) runs preserves all guardrail results across serialization is partially visible (`run_state._input_guardrail_results`, `src/agents/run_internal/run_loop.py:1479-1481`) but the resume path's full fidelity was not traced end-to-end in this study.
- Realtime and voice pipelines have their own guardrail surfaces (`src/agents/realtime/`); this analysis focused on the core `Runner` loop and did not verify parity of policy injection points there.

---

Generated by `09.01-policy-injection-points` against `openai-agents-sdk`.
