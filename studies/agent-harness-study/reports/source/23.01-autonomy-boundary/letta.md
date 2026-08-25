# Source Analysis: letta

## Dimension 23.01: Autonomy Boundary

### Source Info

| Field | Value |
|-------|-------|
| Name | letta |
| Path | `studies/agent-harness-study/sources/letta` |
| Language / Stack | Python (FastAPI server, SQLAlchemy ORM, Pydantic schemas; OpenAPI spec in `fern/openapi.json`) |
| Analyzed | 2026-08-24 |

## Summary

Letta's autonomy boundary is implemented as a **per-tool, human-in-the-loop approval system** layered on top of an otherwise fully autonomous agent loop. The core model, stated directly in the flagship system prompt, is "Continue executing and calling tools until the current task is complete or you need user input. To continue: call another tool. To yield control: end your response without calling a tool." (`studies/agent-harness-study/sources/letta/letta/prompts/system_prompts/letta_v1.py:22`). Within that loop, three categories of actions exist:

1. **Autonomous by default**: any server-side tool without a `RequiresApprovalToolRule` executes immediately via `ToolExecutionManager` (`studies/agent-harness-study/sources/letta/letta/services/tool_executor/tool_execution_manager.py:68-120`), sandboxed per `SandboxType` resolution (`studies/agent-harness-study/sources/letta/letta/settings.py:62-71`).
2. **Always gated**: tools marked with a `RequiresApprovalToolRule` (`studies/agent-harness-study/sources/letta/letta/schemas/tool_rule.py:348-357`) and all client-side tools (`ClientToolSchema`, documented as pausing execution to return control to the client — `studies/agent-harness-study/sources/letta/letta/schemas/letta_request.py:12-18`).
3. **Escalation paths**: run cancellation while a request is pending converts every pending tool call into an automated denial (`studies/agent-harness-study/sources/letta/letta/services/run_manager.py:671-748`), and sending new input while an approval is pending raises `PendingApprovalError` → HTTP 409 (`studies/agent-harness-study/sources/letta/letta/errors.py:48-56`, `studies/agent-harness-study/sources/letta/letta/server/rest_api/routers/v1/agents.py:1801-1806`).

Gating is configurable at tool granularity (a persisted `default_requires_approval` flag plus runtime PATCH endpoints) but there is **no global autonomy-level policy knob** — nothing in settings or agent config defines a coarse autonomy level; enforcement lives inside each of the three agent-loop implementations (v1/v2/v3). The system is well-tested for the happy path and cancellation path, but MCP tools and voice agents bypass the approval machinery entirely.

## Rating

**7 / 10** — Clear model with tests, explicit interfaces, and operational safeguards. The approval lifecycle (request message → human decision → approved execution / denial feedback) is explicit, persisted, observable (`pending_approval` on agent state, `requires_approval` stop reason), idempotency-guarded, and escalates correctly under cancellation, backed by a large integration suite. It falls short of 9–10 because enforcement is duplicated per agent-loop version rather than centralized, there is no global/policy-level autonomy configuration, MCP tools and voice agents are outside the gating surface, and the compiled tool-rule prompt block does not disclose approval-gated tools to the LLM.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Autonomy rule type | `ToolRuleType.requires_approval` enum value defines the gating rule category | `studies/agent-harness-study/sources/letta/letta/schemas/enums.py:197` |
| Gating rule schema | `RequiresApprovalToolRule` class docstring: "requires approval before the tool can be invoked"; its `get_valid_tools` deliberately does not restrict the allowlist (enforcement happens in the loop, not the solver) | `studies/agent-harness-study/sources/letta/letta/schemas/tool_rule.py:348-357` |
| Solver support | `ToolRulesSolver.requires_approval_tool_rules` bucket ("trigger an approval request for human-in-the-loop"), categorization in `model_post_init`, queries `is_requires_approval_tool` / `get_requires_approval_tools` | `studies/agent-harness-study/sources/letta/letta/helpers/tool_rule_solver.py:48-50,85-86,186-196` |
| Tool-level config | `Tool.default_requires_approval` field ("Whether or not to require approval before executing this tool") exposed on creation/update schemas | `studies/agent-harness-study/sources/letta/letta/schemas/tool.py:59,127,203` |
| Persistence of gate default | DB column `default_requires_approval` on the tools table | `studies/agent-harness-study/sources/letta/letta/orm/tool.py:55` |
| Auto-gating on agent create | `_resolve_tools_async` returns tools whose `default_requires_approval` is true and the caller appends a `RequiresApprovalToolRule` per tool | `studies/agent-harness-study/sources/letta/letta/services/agent_manager.py:195-226,487-488` |
| Auto-gating on attach | Attaching a tool with `default_requires_approval=True` inserts a `RequiresApprovalToolRule` into `agent.tool_rules` if absent | `studies/agent-harness-study/sources/letta/letta/services/agent_manager.py:2810-2818` |
| Runtime reconfiguration | `modify_approvals_async` adds/removes the requires-approval rule for a given tool name on an agent | `studies/agent-harness-study/sources/letta/letta/services/agent_manager.py:3064-3088` |
| REST API for gates | `PATCH /v1/agents/{agent_id}/tools/approval/{tool_name}` (`ModifyApprovalRequest.requires_approval`); body preferred over deprecated query param | `studies/agent-harness-study/sources/letta/letta/server/rest_api/routers/v1/agents.py:706-740` |
| Stop signal | `StopReasonType.requires_approval`; classified as a completed run status (not failure) | `studies/agent-harness-study/sources/letta/letta/schemas/letta_stop_reason.py:21,26-32` |
| Wire protocol | `ApprovalRequestMessage` (pending tool call(s)) and `ApprovalResponseMessage` (approve/deny + optional reason, multi-approval `approvals` list) message types | `studies/agent-harness-study/sources/letta/letta/schemas/letta_message.py:306-326,328-347` |
| Human input schema | `ApprovalCreate` ("Input to approve or deny a tool call request") and `ToolReturnCreate` unified into `MessageCreateUnion` accepted by standard message endpoints | `studies/agent-harness-study/sources/letta/letta/schemas/message.py:178-197,200-217` |
| Client-side tools always gated | `ClientToolSchema` doc: "execution pauses and returns control to the client"; request field docs repeat this contract | `studies/agent-harness-study/sources/letta/letta/schemas/letta_request.py:12-18,74-79` |
| Loop gating (v3) | In `LettaAgentV3._handle_ai_response`: requested calls matching `is_requires_approval_tool` OR client-tool names are split out into an approval request; loop returns `StopReasonType.requires_approval`; non-gated parallel calls still execute immediately | `studies/agent-harness-study/sources/letta/letta/agents/letta_agent_v3.py:1681-1709` |
| Approval response handling (v3) | Pairs approval request/response messages, partitions approvals into executed calls, denials (with user-supplied reason), and client tool returns; malformed responses abort with `invalid_tool_call` | `studies/agent-harness-study/sources/letta/letta/agents/letta_agent_v3.py:973-1017` |
| Denial feedback (v3) | Denied calls become error `ToolReturn`s ("Error: request to call tool denied. User reason: …") so the LLM sees the refusal and can adapt | `studies/agent-harness-study/sources/letta/letta/agents/letta_agent_v3.py:1752-1762`; helper at `studies/agent-harness-study/sources/letta/letta/server/rest_api/utils.py:230-264` |
| Loop gating (v2) | v2 checks `tool_rules_solver.is_requires_approval_tool` before execution unless the step already is an approval continuation | `studies/agent-harness-study/sources/letta/letta/agents/letta_agent_v2.py:1138-1155` |
| Denial feedback (v2) | On denial persists error result with heartbeat reason "Continuing: user denied request to call tool." | `studies/agent-harness-study/sources/letta/letta/agents/letta_agent_v2.py:1087-1120` |
| Pending-state guard | `_preprocess_messages`: approval input validated against pending request (idempotency check across recent + post-compaction history); regular message while pending raises `PendingApprovalError` | `studies/agent-harness-study/sources/letta/letta/agents/helpers.py:230-310` |
| Request/response ID validation | `validate_persisted_tool_call_ids` / `validate_approval_tool_call_ids` enforce symmetric diff = ∅ between request and response tool-call IDs (legacy single-request ID backfilled) | `studies/agent-harness-study/sources/letta/letta/agents/helpers.py:102-144` |
| HTTP conflict mapping | `PendingApprovalError` registered as 409 handler app-wide; agents router returns 409 with `PENDING_APPROVAL` code + `pending_request_id` | `studies/agent-harness-study/sources/letta/letta/server/rest_api/app.py:595`; `studies/agent-harness-study/sources/letta/letta/server/rest_api/routers/v1/agents.py:1801-1806` |
| Observability | Agent state exposes `pending_approval: Optional[ApprovalRequestMessage]` via `include=["agent.pending_approval"]`; ORM resolves it from latest message role `"approval"` | `studies/agent-harness-study/sources/letta/letta/schemas/agent.py:134-136`; `studies/agent-harness-study/sources/letta/letta/orm/agent.py:317-337` |
| Escalation on cancel | `cancel_run` treats `requires_approval` runs as cancellable, then synthesizes denials for ALL pending tool calls using `TOOL_CALL_DENIAL_ON_CANCEL` reason and checkpoints approval-response + tool messages | `studies/agent-harness-study/sources/letta/letta/services/run_manager.py:649-759`; constant at `studies/agent-harness-study/sources/letta/letta/constants.py:221` |
| Autonomy bound | `DEFAULT_MAX_STEPS = 50` caps unattended looping; request-level `max_steps` override available | `studies/agent-harness-study/sources/letta/letta/constants.py:75`; `studies/agent-harness-study/sources/letta/letta/schemas/letta_request.py:43-46` |
| Legacy autonomy mechanism | `request_heartbeat` param ("You MUST set this value to True if you want to send a follow-up message or run a follow-up tool call") drives `_decide_continuation` in v1/v2 loops | `studies/agent-harness-study/sources/letta/letta/constants.py:217-218`; `studies/agent-harness-study/sources/letta/letta/agents/helpers.py:496-497`; `studies/agent-harness-study/sources/letta/letta/agents/letta_agent_v2.py:1241-1285` |
| Newer autonomy model | v3 docstring: "No heartbeats (loops happen on tool calls)"; `_decide_continuation` continues on any tool call, ends on no-tool-call, hard stop at final step (`max_steps`) | `studies/agent-harness-study/sources/letta/letta/agents/letta_agent_v3.py:100-105,1967-2036` |
| Background autonomy | Sleeptime memory agent edits memory blocks autonomously in background threads (read-write blocks; no approval concept in prompt) | `studies/agent-harness-study/sources/letta/letta/prompts/system_prompts/sleeptime_v2.py:4-10` |
| Sandbox safety layer | Server-side tool execution defaults to LOCAL/E2B/Modal sandbox depending on credentials; `tool_sandbox_timeout=180` | `studies/agent-harness-study/sources/letta/letta/settings.py:23-36,57-71` |
| HITL integration tests | Full lifecycle: blocked sends while pending, wrong-ID rejection, invoke/approve/deny, cursor fetch, context checks, follow-ups+errors, client-side tools, parallel calls, cancel-during-approval, retry-after-summarization | `studies/agent-harness-study/sources/letta/tests/integration_test_human_in_the_loop.py:185-245,253-292,654+,914+,1293,1340,1456` |
| Cancellation tests | Step stops with `requires_approval`, cancel during pending yields automated denial, next run not stuck in approval state | `studies/agent-harness-study/sources/letta/tests/managers/test_cancellation.py:795-834,881,928,1010,1210-1334` |
| Rule attach/config tests | Attaching tool with `default_requires_approval=True` adds rule; `modify_approval` toggles rule; update of tool default propagates | `studies/agent-harness-study/sources/letta/tests/managers/test_tool_manager.py:443-510,1773-1785` |
| Solver unit tests | `test_is_requires_approval_tool`, `test_should_force_tool_call_requires_approval_rule` | `studies/agent-harness-study/sources/letta/tests/test_tool_rule_solver.py:64-72,672-675` |

## Answers to Dimension Questions

### 1. What determines agent autonomy?

Per-agent tool rules. A tool is gated iff a `RequiresApprovalToolRule(tool_name=...)` exists in the agent's `tool_rules` (`studies/agent-harness-study/sources/letta/letta/schemas/tool_rule.py:348-353`), checked at execution time by `ToolRulesSolver.is_requires_approval_tool` (`studies/agent-harness-study/sources/letta/letta/helpers/tool_rule_solver.py:186-188`). Rules originate from the tool creator's `default_requires_approval` flag at attach/create time (`studies/agent-harness-study/sources/letta/letta/services/agent_manager.py:2810-2818,487-488`) and can be overridden per agent afterwards. Additionally, client-side tools are structurally always gated because only the client can execute them (`studies/agent-harness-study/sources/letta/letta/agents/letta_agent_v3.py:1683-1696`). Everything else — including MCP tools and builtin/core tools — executes autonomously server-side.

### 2. Are autonomy levels configurable?

Only at tool granularity — there are no levels. The binary gate can be flipped at three layers: tool definition (`default_requires_approval`, `studies/agent-harness-study/sources/letta/letta/schemas/tool.py:127`), per agent attachment (`modify_approvals_async`, `studies/agent-harness-study/sources/letta/letta/services/agent_manager.py:3064-3088`), and via REST (`PATCH .../tools/approval/{tool_name}`, `studies/agent-harness-study/sources/letta/letta/server/rest_api/routers/v1/agents.py:714-740`; test turning the gate off at `studies/agent-harness-study/sources/letta/tests/integration_test_human_in_the_loop.py:319`). No global "approval mode" (e.g., approve-all/approve-none) exists in `letta/settings.py` (searched for `approv|autonom` — no matches). The autonomous *duration* is bounded by `max_steps` (default 50, `studies/agent-harness-study/sources/letta/letta/constants.py:75`).

### 3. Are boundaries documented?

Partially, in code rather than prose. The contract is documented on the API surface: `ApprovalCreate`/`ApprovalRequestMessage`/`ApprovalResponseMessage` docstrings (`studies/agent-harness-study/sources/letta/letta/schemas/message.py:179`, `studies/agent-harness-study/sources/letta/letta/schemas/letta_message.py:307-339`), the endpoint description "Modify the approval requirement for a tool attached to an agent" (`studies/agent-harness-study/sources/letta/letta/server/rest_api/routers/v1/agents.py:724-726`), and the generated OpenAPI spec exposing these schemas (`studies/agent-harness-study/sources/letta/fern/openapi.json:5425-5473,28498-28507,29243-29272`). Autonomy philosophy appears only in system prompts (`studies/agent-harness-study/sources/letta/letta/prompts/system_prompts/letta_v1.py:22`; legacy heartbeat language at `studies/agent-harness-study/sources/letta/letta/prompts/system_prompts/memgpt_chat.py:20-21` and `studies/agent-harness-study/sources/letta/letta/constants.py:218`). No dedicated design doc or README section explains when to gate what.

### 4. Does the system respect autonomy boundaries?

Yes, mechanically, with strong guarantees: the loop stops with `requires_approval` before executing gated calls (`studies/agent-harness-study/sources/letta/letta/agents/letta_agent_v3.py:1697-1709`); the pending state blocks other inputs with 409 (`studies/agent-harness-study/sources/letta/letta/agents/helpers.py:307-310`); responses are validated against the exact requested tool-call IDs (`studies/agent-harness-study/sources/letta/letta/agents/helpers.py:121-144`); retries are idempotent post-summarization (`studies/agent-harness-study/sources/letta/letta/agents/helpers.py:248-265`; test `studies/agent-harness-study/sources/letta/tests/integration_test_human_in_the_loop.py:1456`). Caveats: enforcement is reimplemented in each loop version (v1 `studies/agent-harness-study/sources/letta/letta/agents/letta_agent.py:1780-1794`, v2 `studies/agent-harness-study/sources/letta/letta/agents/letta_agent_v2.py:1138-1153`, v3 `studies/agent-harness-study/sources/letta/letta/agents/letta_agent_v3.py:1682-1709`), so a future loop could silently drop the check; the rule itself does not restrict the tool allowlist (`get_valid_tools` returns input unchanged, `studies/agent-harness-study/sources/letta/letta/schemas/tool_rule.py:355-357`), so nothing constrains the LLM from *requesting* a gated tool — which is by design but means the boundary is purely server-enforced, not prompt-declared (the compiled rule-prompt block omits requires-approval rules, `studies/agent-harness-study/sources/letta/letta/helpers/tool_rule_solver.py:209-237`).

## Architectural Decisions

- **Approval as a first-class message type.** Requests/responses are persisted conversation messages with dedicated roles (`MessageRole.approval`), enabling durable pending state that survives restarts and is derivable from history alone (`studies/agent-harness-study/sources/letta/letta/orm/agent.py:317-337`).
- **Gate policy stored as agent tool-rules, not tool metadata alone.** The same `RequiresApprovalToolRule` object flows through serialization/export (`studies/agent-harness-study/sources/letta/tests/test_agent_files/test_agent.af:36-91`), so boundaries travel with the agent.
- **Unified inbox semantics.** `requires_approval` stop reasons keep runs `completed` and are filterable (`last_stop_reason` filter, `studies/agent-harness-study/sources/letta/letta/services/agent_manager.py:1067-1133`; run listing at `studies/agent-harness-study/sources/letta/tests/managers/test_run_manager.py:340-349`), explicitly supporting an approval-inbox UX.
- **Cancellation degrades to denial, not deadlock.** Cancelling a run awaiting approval fabricates `ApprovalReturn(approve=False)` responses for every pending call so the conversation remains consistent (`studies/agent-harness-study/sources/letta/letta/services/run_manager.py:704-748`).
- **Loop-model migration absorbed the autonomy trigger.** Legacy heartbeat-based chaining (`request_heartbeat`, v1/v2) was replaced by tool-call-driven looping in v3 (`studies/agent-harness-study/sources/letta/letta/agents/letta_agent_v3.py:100-105,2071`), simplifying who decides to continue: the loop now continues autonomously after any tool call until the model yields.

## Notable Patterns

- **Partial-step autonomy:** when a step mixes gated and ungated calls, ungated calls execute immediately while gated ones wait (`allowed_tool_calls` vs `requested_tool_calls`, `studies/agent-harness-study/sources/letta/letta/agents/letta_agent_v3.py:1686-1709`).
- **Idempotent approval ingestion:** duplicate approval submissions are detected against recent tool messages or full post-compaction history and converted into keep-alive pings instead of errors (`studies/agent-harness-study/sources/letta/letta/agents/helpers.py:234-286`).
- **Multi-approval batching:** `ApprovalCreate.approvals` carries per-tool-call decisions with individual reasons, migrating the deprecated single `approve`/`approval_request_id` fields automatically (`studies/agent-harness-study/sources/letta/letta/schemas/message.py:187-197`).
- **Provider-visible gating:** streaming interfaces receive `requires_approval_tools` lists so clients can render pending states during token streaming (`studies/agent-harness-study/sources/letta/letta/interfaces/openai_streaming_interface.py:92-149,430`; Anthropic equivalent `studies/agent-harness-study/sources/letta/letta/interfaces/anthropic_streaming_interface.py:74-115`).

## Tradeoffs

- **Per-tool opt-in vs. safety defaults:** nothing is gated unless a developer opts in (`tests/managers/conftest.py:224-250` shows even a "scary bash operation" fixture must set `default_requires_approval=True` manually). Safe-by-default is absent; the tradeoff favors low friction.
- **Three loop implementations:** identical gating logic duplicated across `letta_agent.py`, `letta_agent_v2.py`, `letta_agent_v3.py` increases drift risk (v2 keeps heartbeat plumbing; v3 drops it), though shared helpers (`create_approval_request_message_from_llm_response`) mitigate divergence.
- **Denial-as-error-feedback:** refusals are fed back as function errors (`studies/agent-harness-study/sources/letta/letta/server/rest_api/utils.py:250-264`), which preserves conversational coherence but lets the model immediately retry variants without re-triggering a fresh approval round-trip unless the rule remains enforced each attempt (it is — the check re-runs per step).
- **Observability vs. coupling:** deriving `pending_approval` from "latest message role == approval" (`studies/agent-harness-study/sources/letta/letta/orm/agent.py:329-334`) is cheap but couples the public field to message-ordering invariants maintained elsewhere.

## Failure Modes / Edge Cases

Handled:
- Malformed/empty approval payloads abort cleanly with `invalid_tool_call` instead of executing nothing silently (`studies/agent-harness-study/sources/letta/letta/agents/letta_agent_v3.py:1007-1017`).
- Mismatched `tool_call_id`s rejected (`studies/agent-harness-study/sources/letta/letta/agents/helpers.py:121-144`; test `studies/agent-harness-study/sources/letta/tests/integration_test_human_in_the_loop.py:222-245`).
- Cancelled-while-pending runs auto-deny and do not leave the agent stuck (`studies/agent-harness-study/sources/letta/tests/managers/test_cancellation.py:808-834,928-933,1010`).
- Approval replay after summarization resolved via full-history scan (`studies/agent-harness-study/sources/letta/letta/agents/helpers.py:248-265`).
- Legacy approval requests lacking `step_id` get one synthesized (`studies/agent-harness-study/sources/letta/letta/agents/letta_agent_v3.py:1020-1026`).

Uncovered (no evidence found):
- **MCP tools have no approval integration.** Searched `letta/services/mcp/*.py` and `letta/services/tool_executor/mcp_tool_executor.py` for `approval|requires_approval` — zero matches; remote MCP tools execute ungated unless wrapped in a gated Letta tool.
- **Voice agents skip the machinery entirely:** `letta/agents/voice_agent.py` contains no approval logic and actively strips `request_heartbeat` (`studies/agent-harness-study/sources/letta/letta/agents/voice_agent.py:412-416,463-465`).
- **Background sleeptime autonomy is ungated by design:** the memory agent edits read-write memory blocks autonomously (`studies/agent-harness-study/sources/letta/letta/prompts/system_prompts/sleeptime_v2.py:4-10`); no approval hook exists for memory edits.
- No rate/quota guard on how many times a denied tool may be re-requested within `max_steps`.

## Future Considerations

- Centralize the gating check in a single loop-agnostic module (or make `ToolRulesSolver.get_allowed_tool_names` exclude gated tools pre-flight) so new agent versions cannot regress the boundary.
- Add a global or agent-level autonomy mode (e.g., `require_approval: none | selected | all`) on top of per-tool flags; today only the binary per-tool rule exists.
- Extend `RequiresApprovalToolRule` coverage to MCP tool invocation and voice/sleeptime loops.
- Surface approval requirements to the model via `compile_tool_rule_prompts` (currently omitted, `studies/agent-harness-study/sources/letta/letta/helpers/tool_rule_solver.py:218-224`) to reduce futile gated-call attempts.
- Add timeout/expiry semantics for stale pending approvals beyond manual cancellation.

## Questions / Gaps

- Is there guidance (docs, templates, or defaults) recommending which tool categories should set `default_requires_approval=True`? Searched README/docs/function sets — none found; the flag is purely creator-defined.
- Can approvals be delegated programmatically (e.g., auto-approve policies, webhook callbacks)? The OpenAPI spec advertises an `agent.pending_approval` event/stream shape (`studies/agent-harness-study/sources/letta/fern/openapi.json:456,4245,5071`) but no auto-resolution path was found in server code; resolution is strictly via `ApprovalCreate` messages or cancellation.
- What enforces ordering when multiple conversations target the same agent while one has a pending approval? `ConversationBusyError`/`PendingApprovalError` handlers exist (`studies/agent-harness-study/sources/letta/letta/server/rest_api/app.py:586-596`) but cross-conversation contention behavior was not traced end-to-end.

---

Generated by `dimensions/23.01-autonomy-boundary.md` against `letta`.
