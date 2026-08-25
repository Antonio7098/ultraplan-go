# Source Analysis: pydantic-ai

## 23.01 Autonomy Boundary

### Source Info

| Field | Value |
|-------|-------|
| Name | pydantic-ai |
| Path | `studies/agent-harness-study/sources/pydantic-ai` |
| Language / Stack | Python (agent framework; `pydantic_ai_slim` core, `pydantic_graph` loop, UI adapters, durable-execution integrations) |
| Analyzed | 2026-08-24 |

All citations below are workspace-relative paths under the selected source root `studies/agent-harness-study/sources/pydantic-ai/`.

## Summary

Pydantic AI's autonomy boundary model is: **tools execute autonomously by default; gating is opt-in per tool call, expressed in three composable ways and enforced at a single choke point before side effects run.**

1. **Declarative per-tool gate** — `requires_approval=True` on `@agent.tool` / `@agent.tool_plain` / `Tool` / `FunctionToolset.tool` (`pydantic_ai_slim/pydantic_ai/toolsets/function.py:78`, `pydantic_ai_slim/pydantic_ai/tools.py:431`). This flips the tool's `ToolDefinition.kind` to `'unapproved'` (`pydantic_ai_slim/pydantic_ai/tools.py:506`); external execution uses kind `'external'`. Both kinds are exposed as `ToolDefinition.defer` (`pydantic_ai_slim/pydantic_ai/tools.py:739-745`).
2. **Dynamic wrapper gate** — `ApprovalRequiredToolset` wraps any toolset and consults an `approval_required_func(ctx, tool_def, tool_args)` predicate before executing (`pydantic_ai_slim/pydantic_ai/toolsets/approval_required.py:22-30`), attachable fluently via `AbstractToolset.approval_required()` (`pydantic_ai_slim/pydantic_ai/toolsets/abstract.py:232-244`).
3. **Imperative raise** — tools (or their `args_validator`) raise `ApprovalRequired` for conditional human-in-the-loop gating or `CallDeferred` for external execution, each carrying optional correlation `metadata` (`pydantic_ai_slim/pydantic_ai/exceptions.py:150-183`; usage example `docs/deferred-tools.md:125-129`).

When gated, the run either pauses — ending with a structured `DeferredToolRequests` output separating `approvals` from externally-executed `calls` (`pydantic_ai_slim/pydantic_ai/_deferred.py:26-42`) — or resolves inline through a `HandleDeferredToolCalls` capability handler (`pydantic_ai_slim/pydantic_ai/capabilities/deferred_tool_handler.py:14-75`). Resumption supplies `DeferredToolResults` keyed by `tool_call_id`: approve (`ToolApproved`, optionally with `override_args`), deny (`ToolDenied` with model-facing message), or arbitrary results/retries for external calls (`pydantic_ai_slim/pydantic_ai/_deferred.py:99-118`, `pydantic_ai_slim/pydantic_ai/_deferred.py:154-197`).

The framework is explicit that this boundary protects **against the model acting without human sign-off, not against malicious clients**: approvals submitted through UI adapters are trusted-by-design with loud warnings to enforce authorization inside tool functions (`docs/deferred-tools.md:101-102`, `docs/ui/overview.md:138-141`, `docs/message-history.md:406-417`). A past class of bypass bug — declaratively deferred tools silently running when callers skipped graph-side kind classification — was fixed by centralizing enforcement in `ToolManager.handle_call` (`pydantic_ai_slim/pydantic_ai/tool_manager.py:1099-1121`), and the realtime module carries a standing policy note that reimplementing that policy layer "is a security bug, not a style problem: the realtime approval-bypass was exactly this" (`pydantic_ai_slim/pydantic_ai/realtime/AGENTS.md:20-23`).

## Rating

**8 / 10**

Rationale against the rubric:

- **Clear model (7–8 band):** autonomy is binary per tool call but configurable through three explicit mechanisms; gating decisions are made after argument validation and before execution, so humans see validated arguments.
- **Explicit interfaces:** public exception types (`ApprovalRequired`, `CallDeferred`), typed request/result dataclasses (`DeferredToolRequests`/`DeferredToolResults`), wrapper toolset, capability hook (`handle_deferred_tool_calls`), all exported from the package root (`pydantic_ai_slim/pydantic_ai/__init__.py:32-33,164,209-210,318`).
- **Operational safeguards:** duplicate `tool_call_id` rejection before handing requests out (`pydantic_ai_slim/pydantic_ai/_tool_execution.py:966-973`); hard `UserError` if deferrals occur without `DeferredToolRequests` declared as an output type or an inline handler (`pydantic_ai_slim/pydantic_ai/_tool_execution.py:1043-1049`); denials recorded with `outcome='denied'` rather than fake success (`docs/deferred-tools.md:246-250`).
- **Tests:** pause/resume semantics including `run_id` stamping across the pause (`tests/test_agent.py:4055-4077`), denial propagation (`tests/test_agent.py:10683-10753`), validator-raised deferrals (`tests/test_tools.py:4200-4213`, `tests/test_tools.py:4332-4350`), streaming deferral events (`tests/test_streaming.py:5110-5325`), OTel span attributes for deferrals (`tests/test_logfire.py:3432-3482`), UI adapter approval round-trips (`tests/test_vercel_ai.py:3687-3731`).
- **Why not 9–10:** there is no built-in server-side pending-action store or approver-identity/RBAC concept — correlation security is delegated to application code (documented, but still delegated); autonomy levels are per-call binary rather than graded policies; and one historical bypass bug (realtime) proves the boundaries needed hardening after the fact. The system knows when it is out of its depth (it stops the world), but proving it under failure/scale relies on the durable-execution integrations rather than first-party guarantees.

## Evidence Collected

Every entry cites a file path with line numbers relative to `studies/agent-harness-study/sources/pydantic-ai/`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Declarative gate parameter | `requires_approval: bool = False` on function tools; docstring points to HITL docs | `pydantic_ai_slim/pydantic_ai/toolsets/function.py:78,107-108,132` |
| Gate encoded as tool kind | `kind='unapproved' if self.requires_approval else 'function'` on `ToolDefinition` | `pydantic_ai_slim/pydantic_ai/tools.py:506` |
| Kind → defer predicate | `defer` property true for `'external'`/`'unapproved'` | `pydantic_ai_slim/pydantic_ai/tools.py:739-745` |
| Dynamic wrapper gate | `ApprovalRequiredToolset.call_tool` raises `ApprovalRequired` unless `ctx.tool_call_approved` | `pydantic_ai_slim/pydantic_ai/toolsets/approval_required.py:26-32` |
| Fluent composition | `AbstractToolset.approval_required(func)` returns wrapped toolset | `pydantic_ai_slim/pydantic_ai/toolsets/abstract.py:232-244` |
| Deferral exceptions | `CallDeferred` / `ApprovalRequired` with optional `metadata`, picklable (`__reduce__`) for durable engines | `pydantic_ai_slim/pydantic_ai/exceptions.py:150-183` |
| Request type | `DeferredToolRequests.calls/.approvals/.metadata` usable as agent `output_type` | `pydantic_ai_slim/pydantic_ai/_deferred.py:26-42` |
| Result builder | `build_results()` validates ID/kind matching, supports `approve_all=True` (fail-closed `ValueError` on unknown IDs) | `pydantic_ai_slim/pydantic_ai/_deferred.py:44-86` |
| Approval decision types | `ToolApproved(override_args=...)` / `ToolDenied(message='The tool call was denied.')` | `pydantic_ai_slim/pydantic_ai/_deferred.py:99-118` |
| Resume payload | `DeferredToolResults.approvals/calls/metadata` + normalization `to_tool_call_results()` | `pydantic_ai_slim/pydantic_ai/_deferred.py:154-197` |
| Approved flag on context | `RunContext.tool_call_approved` (+ `tool_call_metadata`) set from resume state | `pydantic_ai_slim/pydantic_ai/_run_context.py:117-120`; set at `pydantic_ai_slim/pydantic_ai/tool_manager.py:295-304` |
| Graph pre-execution classification | Calls bucketed by kind; `'external'`/`'unapproved'` collected, never executed unresolved | `pydantic_ai_slim/pydantic_ai/_tool_execution.py:380-403`, `921-953` |
| Streaming-path detection | `_get_deferred_tool_requests` reads `tool_def.kind == 'unapproved'/'external'` off response tool calls | `pydantic_ai_slim/pydantic_ai/result.py:1065-1083` |
| Single enforcement choke point | `handle_call` converts declarative `tool_def.defer` into raised deferral so every caller inherits the gate; comment documents the drift-bug fix | `pydantic_ai_slim/pydantic_ai/tool_manager.py:1099-1121` |
| Denial execution path | `ToolDenied` short-circuits execution in `_call_tool` | `pydantic_ai_slim/pydantic_ai/_tool_execution.py:669-675` |
| Approved-args revalidation | `_validate_approved_call` revalidates with handler `override_args` | `pydantic_ai_slim/pydantic_ai/_tool_execution.py:609-623` |
| Inline resolution flow | Batch event → handler → re-execute resolved calls → bubble-up remainder | `pydantic_ai_slim/pydantic_ai/_tool_execution.py:964-1050` |
| Inline handler capability | `HandleDeferredToolCalls` with user handler; `None` declines to next handler/output | `pydantic_ai_slim/pydantic_ai/capabilities/deferred_tool_handler.py:14-75` |
| External toolset | `ExternalToolset` marks defs kind `'external'`, `call_tool` raises `NotImplementedError` | `pydantic_ai_slim/pydantic_ai/toolsets/external.py:15-46` |
| Validation-before-deferral invariant | Deferral only honored once arguments validated; hook placement rules (`before_tool_validate` may not defer) | `pydantic_ai_slim/pydantic_ai/tool_manager.py:306-428`; codified in `pydantic_ai_slim/pydantic_ai/.agents/skills/building-pydantic-ai-agents/references/CAPABILITIES-AND-HOOKS.md:105` |
| Pause/resume API surface | `deferred_tool_results=` accepted by `run`/`run_sync`/`iter` variants | `pydantic_ai_slim/pydantic_ai/agent/__init__.py:1139-1276`; `pydantic_ai_slim/pydantic_ai/agent/abstract.py:476-973` |
| Realtime degradation | Sessions cannot pause; unresolved approval answered as `outcome='failed'` explanation, never fake success | `pydantic_ai_slim/pydantic_ai/realtime/_session.py:378-403`, catch at `2148` |
| Realtime policy note | Reimplementing the shared policy layer caused the historical realtime approval bypass | `pydantic_ai_slim/pydantic_ai/realtime/AGENTS.md:18-23` |
| Instrumentation semantics | Deferrals treated as control flow, not errors (spans left UNSET, `pydantic_ai.tool.deferral.name` attribute) | `pydantic_ai_slim/pydantic_ai/capabilities/instrumentation.py:433-452`; asserted in `tests/test_logfire.py:3432-3482` |
| UI sanitization | Adapter strips client-submitted parts; trailing tool calls kept only when resolved in `deferred_tool_results` | `pydantic_ai_slim/pydantic_ai/ui/_adapter.py:395-438` |
| Docs: HITL guide + warning | Dedicated deferred-tools page; "Approval is not an authorization boundary" admonition | `docs/deferred-tools.md:91-104` (warning at `101-102`) |
| Docs: toolset-level gating | "Requiring Tool Approval" section with runnable example incl. deny path | `docs/toolsets.md:454-509` |
| Docs: adapter trust model | Client-submitted approvals/denials/results are untrusted inputs; mitigation guidance | `docs/ui/overview.md:138-141`; `docs/message-history.md:406-417` |
| Docs: AG-UI interrupts | `requires_approval=True` maps onto interrupt-aware run lifecycle | `docs/ui/ag-ui.md:314-362` |
| Docs: Vercel approval chunks | `tool-approval-request` chunks (SDK v6+); strict JSON-boolean decisions, no coercion of `1`/`"true"` | `docs/ui/vercel-ai.md:165-188`; chunk type at `pydantic_ai_slim/pydantic_ai/ui/vercel_ai/response_types.py:162-168` |
| Durable execution carry-through | `metadata` on `ApprovalRequired`/`CallDeferred` crosses Temporal boundary as JSON shapes | `docs/durable_execution/temporal.md:213` |
| Tests: pause/resume identity | New run gets fresh `run_id`; paused history keeps original stamps | `tests/test_agent.py:4055-4077` |
| Tests: denials preserved end-to-end | `ToolDenied('File cannot be deleted')` mapped back by ID; denial reason preserved via adapters | `tests/test_agent.py:10683-10753`; `tests/test_vercel_ai.py:3687-3731` |
| Tests: validator-raised deferral | `args_validator` raising `ApprovalRequired(metadata={'reason': 'sensitive'})`; parity between body-raise and validator-raise metadata | `tests/test_tools.py:4200-4213`, `4332-4382` |
| Tests: exhaustive-strategy deferral | Under `exhaustive`, `ApprovalRequired` defers and resume-with-approval executes | `tests/test_agent.py:7206-7222` |

## Answers to Dimension Questions

**1. What determines agent autonomy?**
Execution is autonomous unless a call is gated. Gating is determined by: (a) the static registration flag `requires_approval=True` (`pydantic_ai_slim/pydantic_ai/tools.py:431`), encoded as tool-definition kind (`pydantic_ai_slim/pydantic_ai/tools.py:506`); (b) a dynamic per-call predicate supplied to `ApprovalRequiredToolset` receiving run context, definition, and validated args (`pydantic_ai_slim/pydantic_ai/toolsets/approval_required.py:22-30`); (c) imperative raises of `ApprovalRequired`/`CallDeferred` from the tool body or its `args_validator` (`pydantic_ai_slim/pydantic_ai/exceptions.py:150-183`, `pydantic_ai_slim/pydantic_ai/tool_manager.py:342-346`); and (d) tools marked externally executed via `ExternalToolset` (`pydantic_ai_slim/pydantic_ai/toolsets/external.py:32-41`). The framework's stance: the model decides *when* to act, the application decides *whether* protected effects run.

**2. Are autonomy levels configurable?**
Yes, per call and per toolset — but binary (gate / don't gate) plus rich decision payloads, not graded levels. Configurability points: the wrapper predicate can implement any conditionality (`docs/toolsets.md:456-468` shows name-prefix gating); the inline `HandleDeferredToolCalls` handler implements arbitrary policy at resolution time, including `approve_all=True` auto-approval (`pydantic_ai_slim/pydantic_ai/capabilities/deferred_tool_handler.py:40-41`) or per-ID deny with custom model-facing messages (`docs/deferred-tools.md:47-56`); resumers can override approved arguments via `ToolApproved.override_args` (`pydantic_ai_slim/pydantic_ai/_deferred.py:103-104`). There is no global "autonomy level" enum or role-based approver config anywhere in the core (searched `approval`, `autonomy`, `permission` across `pydantic_ai_slim/pydantic_ai/`; only per-call constructs exist).

**3. Are boundaries documented?**
Extensively and unusually honestly. A dedicated page covers both deferred flavors with complete examples (`docs/deferred-tools.md`); toolset gating has its own section (`docs/toolsets.md:454-509`); hook-placement rules for deferrals are codified for extension authors (`pydantic_ai_slim/pydantic_ai/.agents/skills/building-pydantic-ai-agents/references/CAPABILITIES-AND-HOOKS.md:105`); and three separate trust-model warnings state that approval guards against the model, not the client (`docs/deferred-tools.md:101-102`, `docs/ui/overview.md:138-141`, `docs/message-history.md:406-417`). Even the internal agent skills document where a deferral may be raised and why (`...references/TOOLS-ADVANCED.md:40,100`).

**4. Does the system respect autonomy boundaries?**
Yes, with enforcement concentrated where it cannot drift. The graph pipeline classifies calls by kind *before* executing anything (`pydantic_ai_slim/pydantic_ai/_tool_execution.py:380-403`, `921-953`), and `ToolManager.handle_call` converts the declarative kind into a raised deferral so direct callers (e.g., realtime sessions, sandboxes) get the same gate — the code comment explicitly describes the prior failure mode ("a `requires_approval=True` tool would simply run: approval silently skipped", `pydantic_ai_slim/pydantic_ai/tool_manager.py:1099-1121`). Escalation is appropriate: runs stop cleanly with `DeferredToolRequests`, refuse loudly if the output type cannot express deferral (`pydantic_ai_slim/pydantic_ai/_tool_execution.py:1043-1049`), degrade to an honest `failed` return in realtime rather than pretending success (`pydantic_ai_slim/pydantic_ai/realtime/_session.py:378-403`), and emit stream events so frontends learn a run is waiting (`pydantic_ai_slim/pydantic_ai/_tool_execution.py:981-984`; `docs/deferred-tools.md:487-494`). The known residual risk — forged client-supplied approvals — is documented as out of cryptographic scope with concrete mitigations (`docs/message-history.md:414`).

## Architectural Decisions

1. **Gate travels with the schema.** Encoding the gate as `ToolDefinition.kind` (`pydantic_ai_slim/pydantic_ai/tools.py:506`) means classification survives streaming (`pydantic_ai_slim/pydantic_ai/result.py:1065-1083`), message-history round-trips, provider serialization, and durable replay without re-running user code.
2. **One resolution pipeline for declarative and raised deferrals.** `handle_call` normalizes both into the same exception-driven path (`pydantic_ai_slim/pydantic_ai/tool_manager.py:1110-1129`), so new callers inherit the gate "for free" — the stated motivation of the fix.
3. **Deferral-as-control-flow, not error.** `ApprovalRequired`/`CallDeferred` are plain `Exception`s excluded from retry/error instrumentation; spans stay UNSET and deferral names land in span attributes (`pydantic_ai_slim/pydantic_ai/capabilities/instrumentation.py:433-452`).
4. **Pause-the-world and inline-handler resolution compose.** A handler can resolve a subset and let the rest bubble up as run output (`pydantic_ai_slim/pydantic_ai/_deferred.py:88-96` `remaining()`, `pydantic_ai_slim/pydantic_ai/_tool_execution.py:1030-1041`), letting approval live in-process while externals resolve elsewhere.
5. **Validation-before-deferral invariant.** Humans are shown validated arguments: deferrals from hooks are rejected before validation completes and honored after it (`pydantic_ai_slim/pydantic_ai/tool_manager.py:376-424`), and the rule is documented for extension authors (`...references/CAPABILITIES-AND-HOOKS.md:105`).
6. **Wrapper-toolset composability for cross-cutting gating** rather than modifying base classes — an explicit repository convention (`pydantic_ai_slim/pydantic_ai/AGENTS.md:20`, `pydantic_ai_slim/pydantic_ai/toolsets/AGENTS.md:5`).

## Notable Patterns

- **Fail-closed result building:** `build_results()` raises `ValueError` on unknown tool-call IDs instead of ignoring them, and `approve_all=True` only approves IDs actually pending (`pydantic_ai_slim/pydantic_ai/_deferred.py:70-84`).
- **Human-corrected arguments:** `ToolApproved.override_args` lets the approver modify args; they are revalidated on resume (`pydantic_ai_slim/pydantic_ai/_tool_execution.py:609-623`).
- **Denials reach the model as first-class history:** denied calls produce `ToolReturnPart(outcome='denied')` with a customizable message, defaulting to `'The tool call was denied.'` (`pydantic_ai_slim/pydantic_ai/_deferred.py:110-114`; `docs/deferred-tools.md:246-250`).
- **Metadata channel for correlation:** `ApprovalRequired(metadata=...)` flows to `DeferredToolRequests.metadata[tool_call_id]` and back into `RunContext.tool_call_metadata` on approval — used to link background task IDs (`docs/deferred-tools.md:128,313`; `pydantic_ai_slim/pydantic_ai/_run_context.py:119-120`).
- **Honest audit trail in constrained surfaces:** realtime sessions answer unresolved approvals with `outcome='failed'` explanations because recording a refusal as success "would be a misleading audit trail" (`pydantic_ai_slim/pydantic_ai/realtime/_session.py:378-385`).
- **Post-mortem-encoded policy:** the realtime guidelines turn a past bypass bug into a structural rule ("policy lives in the shared core, never in the session") (`pydantic_ai_slim/pydantic_ai/realtime/AGENTS.md:18-23`).

## Tradeoffs

- **Stateless core vs. safe resumption.** Pydantic AI keeps no pending-approval registry; applications must persist `DeferredToolRequests` and correlate resumes themselves. Adapters accept client-submitted `DeferredToolResults` by design (`docs/ui/vercel-ai.md:188`), which keeps the core simple but pushes authentication/correlation work to every deployment (`docs/message-history.md:406-417`).
- **Binary gates vs. graded autonomy.** Any conditional logic (amount thresholds, roles, time windows) must be hand-written into predicates or handlers; there is no shared policy vocabulary, so two agents in one codebase can encode very different approval conventions.
- **Inline handlers vs. auditability.** `HandleDeferredToolCalls(approve_all=True)` makes full-auto approval one line (`pydantic_ai_slim/pydantic_ai/capabilities/deferred_tool_handler.py:40-41`); convenient, but nothing in-core distinguishes an auto-approved run from an ungated one beyond span attributes.
- **Pause-the-world latency vs. liveness.** Text-based runs can pause indefinitely for a human; realtime sessions cannot, so the same gated tool becomes a refusal there (`pydantic_ai_slim/pydantic_ai/realtime/_session.py:390-397`) — correct, but a behavioral difference developers must know.

## Failure Modes / Edge Cases

- **Historical enforcement drift (fixed):** callers that executed tools without graph-side kind classification could skip declarative gates entirely; now centralized in `handle_call` with an explanatory comment (`pydantic_ai_slim/pydantic_ai/tool_manager.py:1099-1121`), plus a parity-test requirement for realtime (`pydantic_ai_slim/pydantic_ai/realtime/AGENTS.md:25`).
- **Client-forged approvals:** a client able to reach the endpoint can approve a call of its own making; explicitly documented as behaving-as-designed, with mitigations (server-side persistence of paused runs, in-tool authorization against deps) (`docs/ui/overview.md:138-141`, `docs/message-history.md:414-417`).
- **Ambiguous correlation:** duplicate `tool_call_id`s in a deferred batch would make result matching ambiguous; rejected up front with `UnexpectedModelBehavior` (`pydantic_ai_slim/pydantic_ai/_tool_execution.py:966-973`).
- **Unexpressible deferral:** a deferral with neither `DeferredToolRequests` in `output_type` nor an inline handler raises `UserError` naming both remedies (`pydantic_ai_slim/pydantic_ai/_tool_execution.py:1043-1049`); registering a gated tool without the output type is allowed up front and fails only if a deferral actually occurs (`tests/test_agent.py:11123-11130`).
- **Re-deferral after approval:** an approved tool may raise `CallDeferred` on execution; it is re-collected under the new kind without emitting a second batch event (documented ambiguity-avoidance choice) (`pydantic_ai_slim/pydantic_ai/_tool_execution.py:1031-1041`).
- **Handler-supplied overrides failing validation:** if `override_args` fail validation after retries are exhausted inline, a defensive branch re-raises `UnexpectedModelBehavior` (`pydantic_ai_slim/pydantic_ai/_tool_execution.py:1010-1015`).
- **Hook misuse:** raising a deferral from `before_tool_validate` (arguments not yet valid) is converted to `UserError` (`pydantic_ai_slim/pydantic_ai/tool_manager.py:380-381`).

## Future Considerations

- **Graded autonomy policies:** a first-class policy object (roles, thresholds, budgets, per-tool defaults) would consolidate the three ad-hoc gating mechanisms and make organization-wide conventions auditable.
- **Optional server-side pending-action registry:** authenticated correlation of issued tool calls to received decisions would close the forged-approval gap the docs currently delegate to application code (`docs/message-history.md:414`); the migration skill already prescribes exactly this shape for production slices (`pydantic_ai_slim/pydantic_ai/.agents/skills/migrating-langchain-to-pydantic-ai/references/WORKAROUND-RECIPES.md:156-158`).
- **Distinguishing auto-approval from ungated execution** in history/telemetry (e.g., recording the resolver identity on `ToolReturnPart`) would improve auditability of `approve_all=True` deployments.

## Questions / Gaps

- **No approver identity or RBAC in core.** Searched `approval|autonomy|permission` across `pydantic_ai_slim/pydantic_ai/`; the only identity-bearing construct is app-supplied `deps`. Who approved is invisible to the framework.
- **CLI approval prompts not found in this source tree.** `docs/interfaces.md:14` claims "approval prompts in the CLI", but a case-insensitive search of `clai/` for `approval|requires_approval` returned no matches; within this source, no CLI approval implementation exists (the claim likely refers to the separately-versioned harness package referenced by that doc). Treated as unsupported here.
- **Forged-result tests are prescriptive, not implemented.** The migration skill instructs teams to forge foreign/unknown/consumed tool-call IDs and reject them at an authenticated server boundary (`...references/VERIFICATION-AND-CUTOVER.md:106`), but the framework tests cover adapter-level strictness (non-boolean approvals fail validation, `tests/test_vercel_ai.py:3767`) rather than end-to-end forgery rejection — consistent with the documented stance that this is application territory.
- **No evidence found** of a global autonomy configuration knob (env var, settings object, or profile field) that raises or lowers the default autonomy level fleet-wide; the search boundary was the entire `pydantic_ai_slim/pydantic_ai/` package plus `docs/`.

---

Generated by dimension `23.01 Autonomy Boundary` against `pydantic-ai`.
