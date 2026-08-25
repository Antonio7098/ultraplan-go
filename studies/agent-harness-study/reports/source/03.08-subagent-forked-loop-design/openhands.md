# Source Analysis: openhands

## 03.08 — Subagent and Forked-Loop Design

### Source Info

| Field | Value |
|-------|-------|
| Name | openhands |
| Path | `studies/agent-harness-study/sources/openhands` |
| Language / Stack | Python 3.12 (FastAPI app server, SQLAlchemy, Pydantic), React/TypeScript frontend, V1 conversation models, agent runtime delegated to `openhands-sdk` / `openhands-tools` (external pip packages, not vendored) |
| Analyzed | 2026-07-19 |

## Summary

OpenHands exposes two surfaces that map onto "subagent / forked loop":

1. **Sub-conversations — a real parent/child conversation model owned by the openhands repo.** A top-level V1 conversation can spawn a child conversation that records `parent_conversation_id`, shares the same sandbox, and inherits git/LLM configuration. The model is implemented in the app-server (`openhands/app_server/app_conversation/...`) and persisted to SQL with a `parent_conversation_id` column. This is the surface currently used by "plan mode" (a `PLAN`-typed agent is launched as a sub-conversation of the `DEFAULT` one) and by `enable_sub_agents`.

2. **Built-in sub-agents — a delegation surface whose runtime lives outside the source directory.** `live_status_app_conversation_service.py` imports `openhands.sdk.subagent.get_registered_agent_definitions` and `openhands.tools.preset.default.register_builtins_agents`, but the implementation of those entry points (the `TaskToolSet`, the dispatch loop, error bubbling, trace linking, etc.) is owned by the pip-installed `openhands-sdk` and `openhands-tools` packages and is NOT vendored here. Inside this directory there is therefore **no observed code that performs a child run, bubbles errors back, or links child traces to the parent** — those mechanisms either live in the SDK (out of scope) or are not implemented.

The parent/child model is well-defined (DB columns, API filter, cascade delete, configuration inheritance) and has explicit unit tests. The actual subagent dispatch (the inner loop that gives the parent agent a tool to spawn a child) is not visible from this source tree.

## Rating: 4

Rationale: a clear, explicitly modeled parent/child conversation contract exists with persistence, tests, and an `enable_sub_agents` setting — that earns a few points. However, the dimension asks about delegation as a *run-time* behaviour (handoff, child state isolation, nested traces, parallel child loops, error bubbling) and the run-time half of that story is owned by an external pip package that the source directory does not vendor. There is no observed code path that nests a run, no trace linkage between child and parent, no shared/isolated memory contract beyond "shares sandbox", and the parallel-child-runs question can only be answered by the SDK.

## Evidence Collected

Every entry includes a workspace-relative file path with line numbers from the selected source directory.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Subagent enable toggle (user setting) | `enable_sub_agents=False` default in the user settings model | `frontend/src/services/settings.ts:54` |
| Subagent enable toggle (Settings API test) | `enable_sub_agents` listed as a general field | `tests/unit/app_server/test_settings_api.py:122` |
| Subagent enable plumbing to SDK | `enable_sub_agents=user.agent_settings.enable_sub_agents` passed into `get_default_tools`, then `register_builtins_agents(enable_browser=True)` called on the DEFAULT path | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:1611-1625` |
| Built-in agent registry import (subagent discovery surface) | `from openhands.sdk.subagent import get_registered_agent_definitions` | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:124` |
| Built-in subagent SDK registration import | `register_builtins_agents` imported from `openhands.tools.preset.default` | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:134` |
| Built-in subagent list forwarded to agent-server | `agent_definitions = list(get_registered_agent_definitions())` and passed into `ConversationSettings` as `agent_definitions` | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:1624-1625,1712` |
| Subagent opt-out semantics | When `enable_sub_agents=False`, `register_builtins_agents` is still called but `get_registered_agent_definitions` is gated | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:1619-1625` |
| Subagent plumbing tests (true case) | `test_build_request_passes_enable_sub_agents_true` asserts `get_registered_agent_definitions` is called and `agent_definitions` reaches `StartConversationRequest` | `tests/unit/app_server/test_live_status_app_conversation_service.py:1080-1118` |
| Subagent plumbing tests (false case) | `test_build_request_passes_enable_sub_agents_false` asserts `enable_sub_agents=False` is forwarded but `get_registered_agent_definitions` is not called | `tests/unit/app_server/test_live_status_app_conversation_service.py:1120-1161` |
| Subagent schema import (in tests) | `from openhands.sdk.subagent.schema import AgentDefinition` referenced as `name='general-purpose', description='General-purpose subagent', tools=['terminal']` | `tests/unit/app_server/test_live_status_app_conversation_service.py:1085-1092` |
| TaskToolSet translation key (i18n) | "Enable sub-agent delegation via TaskToolSet." | `frontend/src/i18n/translation.json:2145-2159` |
| Parent/child conversation model — fields | `parent_conversation_id: OpenHandsUUID \| None = None` and `sub_conversation_ids: list[OpenHandsUUID] = Field(default_factory=list)` | `openhands/app_server/app_conversation/app_conversation_models.py:131-132` |
| Start request carries parent ID | `parent_conversation_id: OpenHandsUUID \| None = None` on `AppConversationStartRequest` | `openhands/app_server/app_conversation/app_conversation_models.py:222` |
| DB column for parent ID | `parent_conversation_id: Mapped[str \| None] = mapped_column(String, nullable=True, index=True)` | `openhands/app_server/app_conversation/sql_app_conversation_info_service.py:109-111` |
| Sub-conversation DB persistence | `parent_conversation_id=(str(info.parent_conversation_id) if info.parent_conversation_id else None)` written into `StoredConversationMetadata` | `openhands/app_server/app_conversation/sql_app_conversation_info_service.py:377-381` |
| Sub-conversation query (find children) | `async def get_sub_conversation_ids(self, parent_conversation_id: UUID) -> list[UUID]` filters on `StoredConversationMetadata.parent_conversation_id` | `openhands/app_server/app_conversation/sql_app_conversation_info_service.py:276-294` |
| Sub-conversation abstract interface | `get_sub_conversation_ids(...)` declared on `AppConversationInfoService` | `openhands/app_server/app_conversation/app_conversation_info_service.py:74-86` |
| Sub-conversations returned in info model | `sub_conversation_ids=sub_conversation_ids or []` populated when fetching info rows | `openhands/app_server/app_conversation/sql_app_conversation_info_service.py:315-316,335-340,570` |
| Search filter that hides sub-conversations | `include_sub_conversations: bool = False` on `search_app_conversation_info` — when false, SQL filters out rows with non-null `parent_conversation_id` | `openhands/app_server/app_conversation/sql_app_conversation_info_service.py:130-152` |
| Search filter exposed via API | `include_sub_conversations: Annotated[bool, Query(...)]` on `GET /api/v1/app-conversations` | `openhands/app_server/app_conversation/app_conversation_router.py:260-281` |
| Cascade delete parent → children | `await self._delete_sub_conversations(conversation_id)` then delete the parent | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:2183-2193` |
| `_delete_sub_conversations` implementation | Iterates `get_sub_conversation_ids(parent_id)`, calls `_delete_from_agent_server` then `_delete_from_database` per child, continues on failure | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:2203-2236` |
| Sub-conversation inheritance | `_inherit_configuration_from_parent(request, parent_info)` propagates `sandbox_id`, `selected_repository`, `selected_branch`, `git_provider`, `llm_model` when not explicitly set | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:1014-1042` |
| Parent lookup before child spawn | If `request.parent_conversation_id` is set, the parent is loaded and `_inherit_configuration_from_parent` is called before the start task is yielded | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:320-331` |
| `parent_conversation_id` stored when child is created | Persisted on `AppConversationInfo` after agent-server returns | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:493` |
| Plan agent is a sub-conversation | `AgentType.PLAN` vs `AgentType.DEFAULT`; PLAN uses `get_planning_tools(plan_path=...)` instead of `get_default_tools(... enable_sub_agents=...)`; system-prompt suffix (`PLANNING_AGENT_INSTRUCTION`) restricts it to non-executing role | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:1594-1617,184-196` |
| `AgentType` enum | `DEFAULT = 'default'`, `PLAN = 'plan'` | `openhands/app_server/app_conversation/app_conversation_models.py:61-65` |
| Condenser picked per agent type | `_create_condenser` chooses `usage_id='condenser'` for DEFAULT vs `'planning_condenser'` for PLAN | `openhands/app_server/app_conversation/app_conversation_service_base.py:578-612` |
| Server-only agent overrides per agent type | `_apply_server_agent_overrides` switches `system_prompt_filename` to `system_prompt_planning.j2` and feeds `plan_structure` for PLAN agents | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:1320-1337` |
| Frontend triggers plan-mode child | `useHandlePlanClick` calls `createConversation({ parentConversationId: conversation.id, agentType: 'plan' })` and tracks `subConversationTaskId` | `frontend/src/hooks/use-handle-plan-click.ts:42-82` |
| Frontend payload forwarding | `useCreateConversation` forwards `parentConversationId` and `agentType` to `V1ConversationService.createConversation` | `frontend/src/hooks/mutation/use-create-conversation.ts:16-72` |
| Two child-kinds of "ACP" distinguished | `agent_kind` tag-bearing conversations (`'acp'` vs `'openhands'`) routed through `_build_acp_start_conversation_request` instead of `_build_start_conversation_request_for_user` | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:1534-1555,1775` |
| Frontend comment on ACP sub-agent | "ACP sub-agent owns its own LLM and condenser, so those pages stay disabled" (settings nav) | `frontend/src/constants/settings-nav.tsx:28`, `frontend/src/routes/agent-settings.tsx:80-84` |
| Frontend `sub_conversation_ids` consumption | WebSocket provider pulls `conversation.sub_conversation_ids` and feeds a child-conversation store | `frontend/src/contexts/websocket-provider-wrapper.tsx:4-55` |
| Plan-mode uses one sub-conversation | "Currently, there is only one sub-conversation and it uses the planning agent." | `frontend/src/contexts/conversation-websocket-context.tsx:208` |
| Frontend ACL guard on re-plan | `use-handle-plan-click.test.tsx` "prevents plan creation when conversation has existing sub_conversation_ids" | `frontend/__tests__/hooks/use-handle-plan-click.test.tsx:215-218` |
| Trace attribution to user (not parent) | `laminar_user_id = await self.user_context.get_user_email() or user.id`; injected into `body_json['user_id']` and into SDK `user_id=laminar_user_id` | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:313-318,418-430,1718-1731,1913-1923` |
| `trace_user_id` plumbing (`litellm_extra_body`) | `get_llm_metadata` writes `trace_user_id`, `session_id`, `trace_version` tags for LiteLLM tracing — conversation_id (parent or child) is the trace id | `openhands/app_server/utils/llm_metadata.py:28-72` |
| DB tests for parent/child | `tests/unit/app_server/test_sql_app_conversation_info_service.py:619-976` covers `include_sub_conversations` SQL filter, multiple parents, and inheritance invariants | `tests/unit/app_server/test_sql_app_conversation_info_service.py:619-976` |
| `parent_conversation_id` preservation in webhook | Tests in `test_webhook_router_parent_conversation.py` assert the field survives on_conversation_update calls | `tests/unit/app_server/test_webhook_router_parent_conversation.py:112-515` |

## Answers to Dimension Questions

1. **Can agents delegate?**
   - **Yes — at two layers.** A conversation can delegate to another conversation via the `parent_conversation_id` field (plan-mode today; arbitrary future use). The default conversation's tool set also receives a list of `AgentDefinition` instances from the SDK's `get_registered_agent_definitions` — the *tool* that uses them is the SDK's `TaskToolSet`, which is not vendored here. Source: `live_status_app_conversation_service.py:124,1622-1625,1712`.
2. **Is delegation just a tool call or a real child run?**
   - Sub-conversations are real child conversations with their own DB row, sandbox binding, history, and lifecycle (`live_status_app_conversation_service.py:2183-2236`, `app_conversation_models.py:131-132`). The agent-as-tool variant from the SDK would have to be inspected in `openhands-sdk`; no `as_tool`, `TaskTool`, or `child_run` symbol was found in this source tree.
3. **Is child state isolated?**
   - **Conditionally, by inheritance, not by design.** `_inherit_configuration_from_parent` reuses the parent's `sandbox_id` and the parent's git context unless the child request overrides them (`live_status_app_conversation_service.py:1014-1042`). The DB schema has no "isolated memory" flag for sub-conversations; the column is plain `String` nullable (`sql_app_conversation_info_service.py:109-111`). Two sub-conversations that share a parent therefore share the same sandbox filesystem and git checkout.
4. **Are child traces linked to parent traces?**
   - **Not explicitly.** Laminar/LiteLLM tracing captures `session_id = conversation_id` and `trace_user_id` per conversation (`utils/llm_metadata.py:28-72`, `live_status_app_conversation_service.py:418-430`). No `parent_trace_id`, `span_link`, or OTel `parent_span_context` field is written; nothing wires the child's `conversation_id` to the parent's on the trace side. The parent/child link is observable only in the DB (`parent_conversation_id`).
5. **Can child loops run in parallel?**
   - **No direct evidence inside the source.** Two sub-conversations could be visible simultaneously (DB allows multiple children per parent), but the orchestration is left to the frontend (`use-handle-plan-click.ts`) and to the SDK's `TaskToolSet`, both of which sit outside this directory. The only multi-child guard is the frontend preventing re-plan when `sub_conversation_ids` is non-empty (`use-handle-plan-click.tsx:50-58`).

## Architectural Decisions

- **Two distinct "child run" notions coexist and are deliberately not unified.** A plan-mode child is a *whole conversation* (own sandbox slot, own DB row, own lifecycle, own API endpoints); a built-in sub-agent (`AgentDefinition`) is metadata handed to the default conversation's agent so its LLM can spawn a same-process child. The boundary is enforced by `AgentType` (`live_status_app_conversation_service.py:1594-1617`) and by `_build_acp_start_conversation_request`/`_build_start_conversation_request_for_user` branching on `ACPAgentSettings` (`live_status_app_conversation_service.py:1534-1555`).
- **Persistence-first design.** The parent/child contract is enforced at the SQL layer (`sql_app_conversation_info_service.py:109-111,288-291`), the info-service contract (`app_conversation_info_service.py:74-86`), the API surface (`app_conversation_router.py:260-281`), and the frontend cache (`use-user-conversation.test.tsx`). Toggle `include_sub_conversations` is consistent across all four.
- **Inheritance is opt-out, not opt-in.** `_inherit_configuration_from_parent` overwrites child request fields only if they are falsy (`live_status_app_conversation_service.py:1029-1042`), so callers must explicitly override. The default behaviour is "share everything the parent has".
- **Subagent enable is a user-level toggle.** `enable_sub_agents` lives on `agent_settings` (`live_status_app_conversation_service.py:1622-1625`) and is a persisted user setting (`services/settings.ts:54`); both sides prove the toggle is meant to be user-facing, not a developer feature flag.
- **Subagent dispatch is delegated to the SDK.** The SDK module name (`openhands.sdk.subagent`) and the hook surface (`get_registered_agent_definitions`, `register_builtins_agents`) are stable contracts; the loop, error contract, and trace wiring live elsewhere. Keeping that out of this source tree means the source tree can evolve independently of the runtime model.

## Notable Patterns

- **Plan-mode "child is a sibling conversation".** Plan mode is implemented as `parent_conversation_id` on a real V1 conversation, not a tool call. The plan agent's role is hard-bounded by `PLANNING_AGENT_INSTRUCTION` ("You CANNOT execute code or make changes", `live_status_app_conversation_service.py:185-196`) so it is non-executing by construction; execution is left to a *different* conversation, which is the `DEFAULT` agent that has its own `sub_conversation_ids` entry pointing back at the plan.
- **Two-phase agent-server provisioning.** The parent conversation's agent is built on the app-server with `register_builtins_agents(enable_browser=True)` and forwarded via `ConversationSettings(..., agent_definitions=...) → StartConversationRequest(agent_definitions=...)` (`live_status_app_conversation_service.py:1619-1716`). The actual serving happens in the SDK/agent-server.
- **Cascade delete with per-child best-effort error tolerance.** `_delete_sub_conversations` swallows per-child exceptions and keeps iterating so one bad child cannot leak the parent's deletion (`live_status_app_conversation_service.py:2230-2236`). This is the closest the source comes to an error-bubbling story.
- **Frontend task-poll pattern for sub-conversation readiness.** Plan-mode returns a `v1_task_id`; the UI polls it via `useSubConversationTaskPolling` and only then adds the child to `sub_conversation_ids` (`use-handle-plan-click.ts:62-79`, `use-sub-conversations.ts`).
- **ACP conversation is also a subagent flavour.** `_build_acp_start_conversation_request` strips `api_key`/`base_url` from the LLM because the ACP sub-process (Claude Code / Codex / Gemini CLI) handles its own LLM calls (`live_status_app_conversation_service.py:1875-1884`). ACP conversations are tagged with `acpserver` so the UI can distinguish, but ACP is structurally just another top-level conversation — not a child — and the comment at `frontend/src/constants/settings-nav.tsx:28` calls this out: "the sub-agent owns its own LLM and condenser".

## Tradeoffs

- **Strong DB-level contract, weak runtime-level contract.** Search, list, cascade delete, and inheritance all work, but inside the loop we have to read SDK code to know what the parent and child share. A future maintainer auditing delegation semantics cannot answer all the dimension's questions from this directory alone.
- **Plan-mode child shares the parent's sandbox.** Cheap to set up and inherits git state for free, but means a plan child and a default child write into the same working tree — there is no per-child `worktree`, no per-child branch, and no per-child filesystem snapshot. Cross-conversation contamination is prevented by sequencing (one sub-conversation at a time per plan agent) rather than isolation.
- **Explicit opt-out for sub-agents, default-on for plan-mode inheritance.** `enable_sub_agents=False` is honoured by the SDK tool-gate (`live_status_app_conversation_service.py:1622-1625`), but `include_sub_conversations=False` is the only way to *hide* plan children from the conversation list (`sql_app_conversation_info_service.py:146-152`). Both are useful, but they live at different layers.
- **Tracing is per-conversation, not per-tree.** `get_llm_metadata` stamps `trace_user_id` + `session_id` (`conversation_id`) on each call (`utils/llm_metadata.py:28-72`); the resulting Laminar traces are joined on user, not on parent. Operators asking "what did the plan agent and the executor do together?" must re-link manually via `parent_conversation_id`.
- **Webhook persistence path treats `parent_conversation_id` as immutable.** Tests in `test_webhook_router_parent_conversation.py:1-515` prove the field is intentionally never overwritten by webhook updates, which prevents accidental re-parenting — a useful invariant, but it also means there is no migration path for "convert a top-level conversation into a sub-conversation" without a manual DB step.

## Failure Modes / Edge Cases

- **`register_builtins_agents` is called before the user toggle is read.** Both the `enable_sub_agents=True` and `enable_sub_agents=False` branches in the test (`tests/unit/app_server/test_live_status_app_conversation_service.py:1079-1161`) call `register_builtins_agents(enable_browser=True)`; only the *read* (`get_registered_agent_definitions`) is gated. If a future SDK version makes `register_builtins_agents` depend on the toggle, every conversation would still register all agents and then strip them out via `agent_definitions` — wasting work but not failing.
- **Parent missing in `_start_app_conversation`** raises a `ValueError` (`live_status_app_conversation_service.py:328-330`) instead of polling for creation. Any caller that races parent creation (e.g., concurrent UI clicks) will hit this edge.
- **`get_sub_conversation_ids(parent)` is called inside `_to_info` for every row in `batch_get_app_conversation_info`** (`sql_app_conversation_info_service.py:335-340`). At scale, a single bulk fetch issues one extra query per row, which is the standard N+1 trap and could matter once conversation counts grow.
- **`_delete_sub_conversations` tolerates per-child errors** but the parent deletion proceeds regardless (`live_status_app_conversation_service.py:2183-2193`). Orphans are possible if the parent is deleted while a child sandbox is still starting.
- **`is_create + parent_conversation_id` does not block writes when the parent is `MISSING`/`ERROR`.** The cascade covers deletion but not creation under a stopped parent; tests assert the converse (`test_webhook_router_parent_conversation.py:399-456`), not creation-side validation.
- **`include_sub_conversations` is per-call, not per-user.** A misconfigured client passing `False` will not see plan-mode children, which is desirable in the list view but could mask audit questions.

## Future Considerations

- **Wire a parent_trace_id into `get_llm_metadata`.** Adding `metadata['parent_conversation_id']` (and a derived OTel `parent_span_context`) when known would let Laminar render sub-conversation trees without a manual join (`utils/llm_metadata.py:28-72`).
- **Promote parent/child observability past search.** `sub_conversation_ids` is well-tested for filtering and cascade, but there is no top-level "tree" endpoint that returns parent + children as one payload. Adding `GET /api/v1/app-conversations/{id}/tree` would consolidate the work spread across `get_app_conversation_info` + `get_sub_conversation_ids` (`app_conversation_info_service.py:74-86`).
- **Surface `agent_definitions` in the API contract.** `agent_definitions` is forwarded to the agent-server via `StartConversationRequest` (`live_status_app_conversation_service.py:1712,1729-1731`) but no router endpoint exposes them back to a debugger. A `GET /api/v1/app-conversations/{id}/agent-definitions` would close the loop.
- **Document the SDK boundary explicitly.** The repository should call out, near the sub-agent settings UI, which behaviours (run, error capture, trace link, parallel) live in `openhands-sdk` and which live in `live_status_app_conversation_service.py` — operators currently cannot tell from this directory alone whether a delegation feature is enabled.

## Questions / Gaps

- **Where is the subagent dispatch loop?** No `TaskTool`, `delegate_to_agent`, or `run_subagent` was found in `openhands/`. The wiring from `agent_definitions` → `StartConversationRequest` is here; the loop that turns that into a child run is in `openhands-sdk` and is out of scope.
- **How are child errors surfaced to the parent?** No `try/except`, no `ChildRunResult`, and no `failure_reason` field is propagated in this directory. The only error tolerance observed is the best-effort cascade delete (`live_status_app_conversation_service.py:2227-2236`), which is structural, not runtime.
- **What is the child LLM profile?** The plan agent's condenser uses `usage_id='planning_condenser'` (`app_conversation_service_base.py:596-604`), but the LLM is inherited wholesale from the parent (`_inherit_configuration_from_parent`, `live_status_app_conversation_service.py:1041-1042`); there is no per-child override path discovered here.
- **Is `enable_sub_agents` honoured by the SDK agent-server, or only by the app-server plumbing?** Tests verify that `get_default_tools(..., enable_sub_agents=...)` receives the value (`tests/unit/app_server/test_live_status_app_conversation_service.py:1117,1160`), but whether the SDK actually consumes it to enable the `TaskToolSet` cannot be confirmed from this directory.
- **Sub-conversation concurrency model.** Can the parent spawn two plan children, or a plan child and a sub-agent, in the same superstep? The source tree shows one plan child at a time enforced by the frontend (`use-handle-plan-click.ts:51-58`) but no server-side guard. Parallel-subagent semantics must come from the SDK.
- **Migration path for an existing conversation that should become a child.** The webhook tests prove the field is immutable on update (`test_webhook_router_parent_conversation.py:112-515`); there is no API to add or change `parent_conversation_id` after creation. Whether that is intended needs to be confirmed with the team.

---

Generated by `03.08-subagent-and-forked-loop-design` against `openhands`.
