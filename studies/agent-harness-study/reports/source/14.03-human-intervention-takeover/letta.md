# Source Analysis: letta

## Dimension 14.03 — Human Intervention and Takeover

> Citation convention: all paths are relative to the source root `studies/agent-harness-study/sources/letta/`.

### Source Info

| Field | Value |
|-------|-------|
| Name | letta |
| Path | `studies/agent-harness-study/sources/letta` |
| Language / Stack | Python (FastAPI REST server, SQLAlchemy ORM, Redis, Modal/E2B sandboxes) |
| Analyzed | 2026-08-26 |

## Summary

Letta (formerly MemGPT) implements human intervention as a first-class, message-protocol-driven system rather than an ad-hoc escape hatch. The core mechanism is a structured **approval pause/resume loop**: when the agent requests a tool flagged `requires_approval` (or a client-side tool), the agent loop persists an `approval`-role message containing pending tool calls and stops with `stop_reason=requires_approval`; a human later submits an `ApprovalCreate` input that approves, denies (with a reason), or replaces tool calls with human-executed results (`ToolReturnCreate`), and execution resumes from the exact persisted state. Conversation-level correction is handled by **forking** (shared immutable messages + freshly compiled system prompt) — direct message editing was deliberately removed (HTTP 405). Humans can also edit persistent state directly through block/agent PATCH APIs with forced system-prompt recompilation, cancel running executions at step boundaries (with automatic repair of pending approval state via synthesized denials), and reattach to background streams. Traceability is strong: interventions are persisted as typed messages in conversation history, and block edits carry actor attribution (`letta_user` vs `letta_agent`) in a versioned history table with undo/redo manager support plus an optional git-backed variant. The main gaps: no true mid-step feedback injection (interventions land at pause points or between steps), no interactive sandbox takeover (no exec/attach into the tool sandbox), and the block-history undo/redo API is not exposed over public REST in this checkout.

## Rating

**8 / 10** — Clear intervention model with explicit interfaces (dedicated message types `ApprovalRequestMessage`/`ApprovalResponseMessage`, `ApprovalCreate`/`ToolReturnCreate` inputs, stop-reason enum), operational safeguards (tool-call-ID validation, idempotent approval replay after cancellation/compaction races, `PendingApprovalError` guard while awaiting approval, conversation locks), and an unusually thorough integration test suite covering approve/deny/client-side/parallel/cancellation-race/error-recovery paths. Falls short of 9–10 because mid-run injection is limited to step/pause boundaries, block-history undo is not publicly exposed, and there is no sandbox takeover surface.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Approval request generation (v3 loop) | Tool calls requiring approval or client-side tools are split out; an approval message is persisted and the loop stops with `StopReasonType.requires_approval` instead of executing | letta/agents/letta_agent_v3.py:1681-1709 |
| Stop reason enum | `requires_approval = "requires_approval"` on `StopReasonType` | letta/schemas/enums.py:197 |
| Legacy v1 loop equivalent | Same pause behavior in `LettaAgent._handle_ai_response` via `create_approval_request_message_from_llm_response` | letta/agents/letta_agent.py:1780-1794 |
| Approval request message construction | Builds assistant message + `role=MessageRole.approval` message holding requested vs allowed tool calls; deterministic ID pairing via `decrement_message_uuid` | letta/server/rest_api/utils.py:304-371 |
| Human approval input schema | `ApprovalCreate`: `approvals` list of per-tool-call approve/reason entries; deprecated `approve`/`approval_request_id` fields migrated by validator | letta/schemas/message.py:178-197 |
| Human-executed tool results | `ToolReturnCreate` lets the client submit tool returns directly ("equivalent to sending an ApprovalCreate with tool return approvals") | letta/schemas/message.py:200-217 |
| Resume path | Loop detects last in-context message role `"approval"`, reloads step metrics, and re-dispatches the stored tool call with `is_approval`/`is_denial`/`denial_reason` from the human input | letta/agents/letta_agent.py:248-271 |
| Denial handling | Denial persists an error tool-return "Error: request to call tool denied. User reason: {denial_reason}" and continues stepping | letta/agents/letta_agent.py:1739-1763 |
| Response validation safeguards | `validate_approval_tool_call_ids` enforces symmetric-difference match between requested and answered tool-call IDs (with legacy fallback); duplicate approvals detected idempotently even post-compaction; regular messages rejected while pending (`PendingApprovalError`) | letta/agents/helpers.py:121-145, 255-293, 307-310 |
| Per-tool approval policy API | `PATCH /v1/agents/{agent_id}/tools/approval/{tool_name}` → `agent_manager.modify_approvals_async` adds/removes a `RequiresApprovalToolRule`; tool default comes from `default_requires_approval` | letta/server/rest_api/routers/v1/agents.py:714-740; letta/services/agent_manager.py:3064-3078; letta/orm/tool.py:55 |
| Message immutability / fork-based correction | `modify_message` raises 405: "Message editing is no longer supported. Messages are immutable as they may be shared across multiple conversations via forking." | letta/server/rest_api/routers/v1/agents.py:1627-1644 |
| Fork API | `POST /v1/conversations/{conversation_id}/fork` (incl. agent-direct mode); creates new conversation sharing source messages, compiles fresh system message from latest blocks | letta/server/rest_api/routers/v1/conversations.py:122-161; letta/services/conversation_manager.py:105-172, 175-219 |
| Direct state editing | `PATCH .../core-memory/blocks/{label}` updates a block then force-rebuilds the system prompt; generic `PATCH /v1/blocks/{block_id}`; agent config via `PATCH /v1/agents/{id}`; archival passages create/delete | letta/server/rest_api/routers/v1/agents.py:1268-1288, 629-638, 1488-1503, 1556-1577; letta/server/rest_api/routers/v1/blocks.py:157-166 |
| Read-only block protection for agents | Agent-facing memory tools raise `READ_ONLY_BLOCK_EDIT_ERROR` when `block.read_only` (human API edits bypass this by design) | letta/services/tool_executor/core_tool_executor.py:320-321, 336-337; letta/schemas/block.py:36 |
| Cancellation / takeover of runs | `POST .../messages/cancel` and conversation cancel resolve active runs (Redis run-id key with DB fallback) and call `run_manager.cancel_run`; if the agent was awaiting approval, denials + tool-return messages are synthesized and checkpointed so state stays consistent | letta/server/rest_api/routers/v1/agents.py:1887-1956; letta/server/rest_api/routers/v1/conversations.py:836-923; letta/services/run_manager.py:619-783 |
| Step-boundary cancellation checks | `_check_run_cancellation()` polled at top of each step; yields `stop_reason=cancelled` | letta/agents/letta_agent.py:155, 284-289 |
| Stream reattachment (resume observation) | `POST /v1/conversations/{id}/stream` resumes SSE for an active background run by run_id, otid→run mapping, or latest-active lookup; 3h expiration guard | letta/server/rest_api/routers/v1/conversations.py:658-833 |
| Concurrency guard | Distributed conversation lock raises `ConversationBusyError` with lock-holder token; concurrent sends cannot interleave | letta/data_sources/redis_client.py:194-244 |
| Audit trail: approvals as messages | Responses persisted as `role=approval` messages carrying `approvals`, `approve`, `denial_reason`; surfaced as typed `ApprovalRequestMessage`/`ApprovalResponseMessage` stream entities | letta/server/rest_api/utils.py:213-227; letta/schemas/letta_message.py:306-345 |
| Audit trail: block edit actor attribution | `BlockHistory` snapshots store `actor_type` (`letta_user`/`letta_agent`) + `actor_id`; checkpoint attributes to agent if `agent_id` passed else to user | letta/orm/block_history.py:12-48; letta/services/block_manager.py:889-900 |
| Block versioning variants | Linear undo/redo stack with future-entry truncation (`checkpoint_block_async`, `undo_checkpoint_block`); optional git-backed manager keeps full history in object storage when agent tagged `git-memory-enabled` | letta/services/block_manager.py:842-911, 952-1000; letta/services/block_manager_git.py:27-40, 533 |
| Pending-approval observability | Agent ORM exposes `pending_approval` relationship (`include=["agent.pending_approval"]`) and persisted `last_stop_reason` | letta/orm/agent.py:108, 317, 462-518; letta/schemas/agent.py:59, 134 |
| HITL integration tests | Approve/deny/client-side/parallel flows, invalid-ID rejection, blocked user messages while pending, stop-reason assertions, approve-vs-cancel race, retry-after-summarization idempotency | tests/integration_test_human_in_the_loop.py:185-201, 253-292, 604-646, 1175-1290, 1340-1453, 1456-1530 |
| Fork & cancellation tests | Fork shares messages and survives source deletion; dedicated cancellation suite incl. pending-approval cleanup | tests/managers/test_conversation_manager.py:1504-1728; tests/managers/test_cancellation.py |

## Answers to Dimension Questions

### 1. Can humans edit agent state?

Yes — broadly, but with one deliberate exclusion. Humans can edit:
- **Core memory**: `PATCH /v1/agents/{agent_id}/core-memory/blocks/{block_label}` (letta/server/rest_api/routers/v1/agents.py:1268-1288) which force-rebuilds the system prompt so the change takes effect on the next turn (agents.py:1285-1286). A parallel internal endpoint exists (letta/server/rest_api/routers/v1/internal_agents.py:33-53).
- **Agent configuration**: model, system prompt, tools via `PATCH /v1/agents/{agent_id}` (agents.py:629-638) and tool attach/detach (agents.py:670-707).
- **Approval policy**: flip `requires_approval` per tool per agent at runtime (agents.py:714-740), changing how much oversight future turns need.
- **Archival memory**: create/delete passages directly (agents.py:1488-1503, 1556-1577).
- **Conversation transcript**: NOT editable — `modify_message` returns 405 with an explicit rationale tying immutability to fork sharing (agents.py:1641-1644). Correction is done by forking into a new branch (conversations.py:122-161). A destructive `reset-messages` remains available (agents.py:2329-2344).

The asymmetry is intentional: agent-side memory tools honor `read_only` blocks (letta/services/tool_executor/core_tool_executor.py:320-321), while human API edits do not — humans are treated as privileged editors of state.

### 2. Can humans provide mid-run feedback?

At **pause points**, yes, richly; truly mid-step, no. When the model requests a gated action, the loop persists the request and stops (`stop_reason=requires_approval`, letta/agents/letta_agent_v3.py:1697-1709). The human's next input can combine: per-tool-call approvals/denials with reasons, substitute tool returns (human executed it themselves), and free-form follow-up user messages processed in the same turn (letta/agents/helpers.py:295-306; test at tests/integration_test_human_in_the_loop.py:604-646). Denials inject an explanatory error tool-return so the agent can adapt without restart (letta/agents/letta_agent.py:1739-1763; deny-test asserts the agent uses the human's hint, integration_test_human_in_the_loop.py:654-687).

While a request is pending, ordinary messages are hard-blocked with `PendingApprovalError` ("Please approve or deny the pending request before continuing", letta/errors.py:53; raised at helpers.py:309-310; tested at integration_test_human_in_the_loop.py:204-217). During active streaming, concurrent input is rejected via a distributed conversation lock (`ConversationBusyError`, letta/data_sources/redis_client.py:194-229), and cancellation is only observed between steps (letta/agents/letta_agent.py:284-289). So the design channels all human steering through safe synchronization points instead of allowing arbitrary interleaving.

### 3. Can humans take over execution?

Partially. There is **no interactive sandbox takeover** — no evidence of exec/attach/shell access into the tool-execution sandboxes (searched `takeover`, `take over`, `exec into`, `attach to sandbox` across `letta/` and `sandbox/`; sandboxes are batch executors: E2B/local under letta/services/tool_sandbox/, Modal under letta/sandbox/modal_executor.py and letta/services/tool_sandbox/modal_sandbox_v2.py). What exists:
- **Per-call takeover**: any tool can be declared client-side; the server pauses exactly as in the approval flow and the human executes it, returning results via `ToolReturnCreate` (letta/schemas/message.py:200-217; v3 treats client tools like approval-gated tools, letta/agents/letta_agent_v3.py:1683-1696; tested at integration_test_human_in_the_loop.py:914-949).
- **Kill switch**: cancel active runs by agent or conversation, including Lettuce-managed runs, with DB fallback if Redis lacks the mapping (letta/server/rest_api/routers/v1/agents.py:1887-1956; conversations.py:836-923).
- **Reattachment**: resume observing a background run's SSE stream by run_id/otid after network interruption (conversations.py:658-833).
- **Branching**: fork a conversation to try an alternate continuation without touching the original (conversations.py:122-161).

### 4. Are human interventions traceable?

Yes, this is a strength.
- Every approval round-trip is persisted in conversation history: the request as an `approval`-role message (letta/server/rest_api/utils.py:355-370) and the decision as an approval-response message carrying `approvals`, `approve`, and `denial_reason` (utils.py:213-227), both exposed as typed stream/list entities `ApprovalRequestMessage`/`ApprovalResponseMessage` (letta/schemas/letta_message.py:306-345). Cursor-pagination tests assert exact ordering `user → approval_request → approval_response → tool_return` (integration_test_human_in_the_loop.py:430-475, 690-735).
- Memory edits are attributed: each `BlockHistory` snapshot records whether the editor was a user or an agent plus its ID (letta/orm/block_history.py:34-37; attribution logic at letta/services/block_manager.py:898-899), enabling per-editor histories and undo (tests/managers/test_managers.py:6317-6325 verifies agent attribution; undo tested at :6431-6470).
- Run/agent surfaces expose intervention state: `last_stop_reason` (letta/orm/agent.py:108), `run.status`/`stop_reason` set to `cancelled` on interruption (letta/services/run_manager.py:662-667), and a queryable `pending_approval` relationship (letta/orm/agent.py:317; letta/schemas/agent.py:134).
- Cancellations during pending approval leave an auditable trail too: synthesized denial responses and tool returns are checkpointed into the transcript (letta/services/run_manager.py:671-783).

What is *not* traceable at message granularity: direct block PATCHes record history only if a checkpoint is taken (the git-backed variant records continuously when enabled, letta/services/block_manager_git.py:30-40); there is no per-HTTP-call audit log of who patched which block outside the `BlockHistory` mechanism.

## Architectural Decisions

1. **Intervention as protocol, not side-channel.** Approvals are messages with their own role and lifecycle, integrated into context assembly — the resumed LLM call literally contains the approval exchange (letta/agents/letta_agent.py:248-271). This makes intervention durable, replayable, and provider-agnostic, unlike callback/webhook-only designs.
2. **Immutability + fork instead of editable transcripts.** Because conversations share Message rows (fork copies links, not content — letta/services/conversation_manager.py:142-158; deletion-safety tested at tests/managers/test_conversation_manager.py:1652-1703), editing would corrupt siblings; the API therefore refuses edits (agents.py:1641-1644) and offers branching as the correction primitive.
3. **Deterministic pairing IDs.** Approval request/response messages get adjacent UUIDs (`decrement_message_uuid`, letta/server/rest_api/utils.py:366-379) and tool-call-ID symmetric-difference validation with a legacy escape hatch (helpers.py:137-145), making mismatched or forged responses detectable server-side.
4. **Fail-safe cancellation around approvals.** Cancelling a run that is waiting on a human does not strand the agent: the system fabricates denial messages and checkpoints them (run_manager.py:671-783), keeping the transcript valid for the next turn.
5. **Actor-typed versioned memory.** Block checkpoints distinguish `letta_user` vs `letta_agent` editors (block_manager.py:898-899), supporting both undo/redo and provenance queries, with a git-backed mode for full durable history (block_manager_git.py:1-40).

## Notable Patterns

- **Pause/resume symmetry**: the same `_handle_ai_response` path executes a tool live or replays it after approval, differing only in `is_approval`/`is_denial` flags (letta/agents/letta_agent.py:1780 vs 248-266) — one code path, fewer drift bugs.
- **Batched multi-tool decisions**: parallel tool calls produce one approval request with many tool calls, and one human response may mix approve + deny + client-supplied returns (letta/server/rest_api/routers/v1/agents.py streaming schema note at runs.py:332; tested end-to-end at integration_test_human_in_the_loop.py:1175-1290).
- **Idempotency everywhere at pause points**: replayed approvals after crash/cancel/compaction are detected by matching persisted tool returns and converted to keep-alive turns instead of double-execution (helpers.py:250-286; exercised by tests/integration_test_human_in_the_loop.py:1418-1453 and 1456-1530).
- **Deprecation-with-teeth**: dead interaction surfaces (message PATCH, `/voice-beta`) fail loudly with explanatory errors rather than silently misbehaving (agents.py:1641-1644; voice.py:17-60).

## Tradeoffs

- **Safety vs immediacy**: blocking non-approval input while pending (helpers.py:307-310) prevents corruption but means a human cannot correct course with plain text until resolving the gate — urgency must be expressed through the denial `reason`.
- **Shared-message forks save storage but prevent surgical edits**: correcting one typo requires forking and re-running, not patching (agents.py:1638-1643).
- **Redis-centric operations**: cancellation lookups degrade gracefully to DB scans (agents.py:1914-1929), but stream reattachment hard-fails 503 without Redis (conversations.py:718-726), making some takeover features deployment-sensitive.
- **Checkpoint-on-write vs continuous audit**: Postgres block history captures states only when checkpoints are made; full continuous auditing requires opting into git-backed memory per agent tag (block_manager_git.py:27-40).

## Failure Modes / Edge Cases

- **Approve/cancel race**: a human approving while an operator cancels is resolved deterministically — cancelled wins, response becomes idempotent keep-alive on retry (helpers.py:250-293; run_manager.py:649-653 treats already-terminal runs as no-op unless pending approval; verified by test_approve_with_cancellation, integration_test_human_in_the_loop.py:1340-1453).
- **Malformed approval responses**: wrong tool-call IDs raise descriptive errors listing expected vs received (helpers.py:143-145; test_send_approval_message_with_incorrect_request_id, integration_test_human_in_the_loop.py:222-245); unsolicited approvals are rejected (integration_test_human_in_the_loop.py:185-201).
- **Corrupt approval requests**: a trailing approval message with no tool calls logs a warning and skips synthesized denial cleanup rather than crashing (run_manager.py:777-781).
- **LLM failure right after approval**: mocked adapter failure proves the agent is not bricked and accepts subsequent turns (integration_test_human_in_the_loop.py:554-601, 817-861).
- **Post-compaction retries**: approval retries still succeed after summarization evicts the request from context, via full-history idempotency search (helpers.py:250-265; test_retry_with_summarization, integration_test_human_in_the_loop.py:1456-1530).
- **Stale streams**: background runs expire after 3 hours and reattachment fails explicitly (`LettaExpiredError`, conversations.py:804-806).

## Future Considerations

- Expose the existing block-history/undo machinery over public REST (currently manager+test level only: letta/services/block_manager.py:952; tests/test_managers.py:6431) so external operators get reversible state edits without the git-mode opt-in.
- Add a queued mid-run feedback channel (e.g., accepted-during-execution user notes flushed at the next step boundary) to complement pause-point gating and cancellation-plus-resend.
- Add TTL/alerting semantics for stuck `requires_approval` runs beyond the 3-hour stream expiry (conversations.py:805-806).
- Consider surfacing per-intervention metrics (approval latency, deny rate per tool) from the already-persisted approval messages.

## Questions / Gaps

- **Sandbox takeover**: No evidence found. Searched `takeover`, `take over`, `exec into`, `attach to sandbox` across `letta/` and `sandbox/`; the sandbox layer (E2B/local/Modal) is programmatic batch execution only (letta/services/tool_sandbox/base.py; letta/sandbox/modal_executor.py). Interactive debugging of a live sandbox is out of scope for this codebase.
- **Undo/redo exposure**: `undo_checkpoint_block` has no callers inside `letta/` besides tests in this checkout — either served by an external service/ADE UI not present here, or currently dormant.
- **Human identity granularity**: approval responses attribute to the API `actor` implicitly; there is no per-human approver identity field distinct from the organization actor in the approval message schema (letta/schemas/message.py:178-197).
- The Letta ADE (desktop/UI where much human editing presumably happens) is external to this repository; its behaviors could not be verified here beyond the internal endpoints it likely consumes (e.g., letta/server/rest_api/routers/v1/internal_agents.py:33).

---

Generated by dimension `14.03-human-intervention-and-takeover` against `letta`.
