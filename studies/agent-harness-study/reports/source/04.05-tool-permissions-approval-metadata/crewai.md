# Source Analysis: crewai

## Tool Permissions and Approval Metadata

### Source Info

| Field | Value |
|-------|-------|
| Name | crewai |
| Path | `studies/agent-harness-study/sources/crewai` |
| Language / Stack | Python (CrewAI framework: Pydantic, LangChain integration) |
| Analyzed | 2026-08-15 |

## Summary

CrewAI ships **no built-in risk classification, permission enum, or native approval policy** for tools. The base tool classes (`BaseTool`, `Tool`, `CrewStructuredTool`) expose only execution-control metadata such as `max_usage_count`, `result_as_answer`, `args_schema`, `result_schema`, `cache_function`, and `env_vars` (`lib/crewai/src/crewai/tools/base_tool.py:139-191`, `lib/crewai/src/crewai/tools/structured_tool.py:115-142`). There is no `risk_level`, `requires_approval`, `side_effects`, or `permission_scope` field anywhere on the tool model.

Approval and permission enforcement is delegated entirely to a **user-registered before/after-tool-call hook subsystem**. The hook store is process-global (`lib/crewai/src/crewai/hooks/tool_hooks.py:124`), and a hook that returns `False` from its `before_tool_call` callback blocks the tool (`lib/crewai/src/crewai/utilities/tool_utils.py:105-114`, `lib/crewai/src/crewai/agents/crew_agent_executor.py:963-979`, `lib/crewai/src/crewai/utilities/agent_utils.py:1508-1528`, `lib/crewai/src/crewai/experimental/agent_executor.py:1953-1977`). The `ToolCallHookContext` exposes a `request_human_input` helper that pauses the live UI and calls `builtins.input()` for an approval prompt (`lib/crewai/src/crewai/hooks/tool_hooks.py:79-121`), and the doc page `docs/edge/en/learn/tool-hooks.mdx:181-204` codifies the "human approval gate" recipe.

There is **no persistence** of approval decisions; hooks live in module-level lists that vanish with the process. There is **no allow/deny list** shipped by the framework; risk classification is delegated to user code (typically a hard-coded `destructive_tools = [...]` array inside a hook, per `docs/edge/en/learn/tool-hooks.mdx:160-179`). The `SecurityConfig`/`Fingerprint` pair looks like a permission system at first glance but is purely an identity/tracking construct (`lib/crewai/src/crewai/security/security_config.py:20-87`, `lib/crewai/src/crewai/security/fingerprint.py:41-157`) — the fingerprint is only forwarded into the tool's `config` argument for telemetry/audit (`lib/crewai/src/crewai/tools/tool_usage.py:1023-1054`). The Agent-level `apps` field (`lib/crewai/src/crewai/agents/agent_builder/base_agent.py:366-369`) gates which CrewAI-Platform apps can be called but still does not classify tools by risk.

Net effect: **the runtime can stop a high-risk tool if the user has registered a hook that targets it; out of the box there is no policy and no risk metadata visible to the model or runtime.**

## Rating

**5/10** — Present but inconsistent, weakly documented at the framework level, and fragile.

Rationale:
- The runtime hook plumbing is real and exercised in production paths (`utilities/tool_utils.py:105-114`, `agents/crew_agent_executor.py:963-979`, `experimental/agent_executor.py:1953-1977`, `utilities/agent_utils.py:1508-1528`), and blocking actually short-circuits tool invocation — that earns execution enforcement.
- Tests demonstrate end-to-end block-and-replace semantics (`lib/crewai/tests/hooks/test_tool_hooks.py:728-796`, `lib/crewai/tests/hooks/test_human_approval.py:228-276`).
- But there is **no permission enum, no risk classifier, no allow/deny list, no persisted approvals, and no default policy** — every safeguard is opt-in user code. The `SecurityConfig` is identity-only (`security/security_config.py:20-67`). The Skill `allowed-tools` frontmatter is documented as "experimental metadata only — it does not provision or inject any tool" (`docs/v1.14.0/pt-BR/concepts/skills.mdx:296`). A2A has `AuthorizationFailedError` but only for cross-agent protocol auth, not tool risk (`a2a/errors.py:79,336-348`).
- The system is global (module-level lists), not per-session and not serializable; `clear_all_tool_call_hooks()` is provided for tests but not for production teardown.

## Evidence Collected

Every entry includes a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Permission enums | **None found.** No `Permission`, `Risk`, `SideEffect`, `Approval`, or `Scope` enum exists in `lib/crewai/src/crewai/tools/`. | `lib/crewai/src/crewai/tools/base_tool.py:139-191`, `lib/crewai/src/crewai/tools/structured_tool.py:115-142` |
| Tool metadata fields | `BaseTool` fields: `name`, `description`, `env_vars`, `args_schema`, `result_schema`, `cache_function`, `result_as_answer`, `max_usage_count`, `current_usage_count` — no permission/risk field. | `lib/crewai/src/crewai/tools/base_tool.py:139-191` |
| Tool metadata fields | `CrewStructuredTool` fields: `name`, `description`, `args_schema`, `result_schema`, `func`, `result_as_answer`, `max_usage_count`, `current_usage_count`, `cache_function` — no permission/risk field. | `lib/crewai/src/crewai/tools/structured_tool.py:115-142` |
| Approval policy | Module-level global hook lists (the only "policy" surface). | `lib/crewai/src/crewai/hooks/tool_hooks.py:124-125` |
| Approval registration | `register_before_tool_call_hook` / `register_after_tool_call_hook`, plus `clear_*` / `unregister_*` helpers. | `lib/crewai/src/crewai/hooks/tool_hooks.py:128-193,218-301` |
| Approval decorator | `@before_tool_call` / `@after_tool_call` with optional `tools=[...]` and `agents=[...]` filters. | `lib/crewai/src/crewai/hooks/decorators.py:208-249,269-305` |
| Human-input approval helper | `ToolCallHookContext.request_human_input` pauses live UI and reads from stdin via `input()`. | `lib/crewai/src/crewai/hooks/tool_hooks.py:79-121` |
| Risk classifications | **None found in framework.** Documentation shows a user-supplied `destructive_tools` list and `sensitive_tools` list as the canonical pattern. | `docs/edge/en/learn/tool-hooks.mdx:160-179` |
| Confirmation states | Only `ask_for_human_input: bool` on `CrewAgentExecutor` (final-answer confirmation, not tool-level approval). | `lib/crewai/src/crewai/agents/crew_agent_executor.py:132,227,243,1128,1144` |
| Permission metadata visible to runtime | Fingerprint is passed to the tool config under `security_context` — it is identity metadata, not a permission. | `lib/crewai/src/crewai/tools/tool_usage.py:1023-1054` |
| Permission enforcement | `execute_tool_and_check_finality` runs `before_hooks`; if any returns `False` the call is short-circuited with `"Tool execution blocked by hook. Tool: ..."`. | `lib/crewai/src/crewai/utilities/tool_utils.py:225-234` |
| Permission enforcement (async path) | `aexecute_tool_and_check_finality` mirrors the sync path. | `lib/crewai/src/crewai/utilities/tool_utils.py:105-114` |
| Permission enforcement (native calling) | Native-function path: `get_before_tool_call_hooks()` is iterated; first `False` blocks. | `lib/crewai/src/crewai/agents/crew_agent_executor.py:954-979` |
| Permission enforcement (legacy/experimental) | Experimental `AgentExecutor` mirrors the same blocking pattern. | `lib/crewai/src/crewai/experimental/agent_executor.py:1953-1977` |
| Permission enforcement (legacy agent_utils) | Text-parsing tool path in `utilities/agent_utils.py` also iterates `get_before_tool_call_hooks()`. | `lib/crewai/src/crewai/utilities/agent_utils.py:1508-1528` |
| Usage cap (related control) | `_claim_usage` enforces `max_usage_count` and increments atomically with a `threading.Lock`. | `lib/crewai/src/crewai/tools/base_tool.py:295-312` |
| Usage cap enforcement | `_check_usage_limit` short-circuits a tool run with an error message when the cap is reached. | `lib/crewai/src/crewai/tools/tool_usage.py:740-757,306-313,546-553` |
| Identity (not permission) | `SecurityConfig` only carries a `Fingerprint`; docstring explicitly defers auth/scoping/delegation with `*TODO*`. | `lib/crewai/src/crewai/security/security_config.py:20-67` |
| Identity (not permission) | `Fingerprint` is `uuid4`/`uuid5` + metadata; equality/hash by UUID; no scopes/roles. | `lib/crewai/src/crewai/security/fingerprint.py:41-157` |
| Identity is forwarded but not enforced | Tool invocation receives fingerprint in `config={"security_context": ...}`; tools can read it but the framework never gates on it. | `lib/crewai/src/crewai/tools/tool_usage.py:1023-1054` |
| Agent-level app gate | `apps: list[PlatformAppOrAction]` filters which CrewAI-Platform apps an agent may call — closer to an allow-list but not a risk classification. | `lib/crewai/src/crewai/agents/agent_builder/base_agent.py:366-369` |
| Agent-level MCP gate | `mcps: list[str | MCPServerConfig]` controls which MCP servers are exposed — also an allow-list. | `lib/crewai/src/crewai/agents/agent_builder/base_agent.py:370-373` |
| Skill-level allow-list (experimental, metadata only) | `SkillFrontmatter.allowed_tools` is parsed but documented as non-binding. | `lib/crewai/src/crewai/skills/models.py:78-93`, `docs/v1.14.0/pt-BR/concepts/skills.mdx:296` |
| Crew-scoped hook binding | `CrewBase._register_hooks` finds `is_before_tool_call_hook`-marked methods and binds them globally with optional `tools` / `agents` filters. | `lib/crewai/src/crewai/project/crew_base.py:441-555` |
| Test: hook block | `test_before_hook_blocks_tool_execution_in_crew` proves a `return False` from a `@before_tool_call` hook prevents the tool function from running. | `lib/crewai/tests/hooks/test_tool_hooks.py:728-796` |
| Test: human approval gate | `TestApprovalHookIntegration.test_approval_hook_blocks_execution` / `test_approval_hook_allows_execution`. | `lib/crewai/tests/hooks/test_human_approval.py:228-276` |
| Test: human approval integration | `TestToolHookHumanInput.test_request_human_input_returns_user_response`, etc., exercise `request_human_input`. | `lib/crewai/tests/hooks/test_human_approval.py:136-220` |
| Test: filter scope | `test_before_tool_call_with_combined_filters` proves `tools=[...]` + `agents=[...]` filter scoping on `@before_tool_call`. | `lib/crewai/tests/hooks/test_decorators.py:215-237` |
| Test: order + first-block wins | `test_first_blocking_hook_stops_execution` confirms first `False` stops subsequent hooks. | `lib/crewai/tests/hooks/test_tool_hooks.py:340-376` |
| Test: hook lifecycle | `clear_hooks` autouse fixture plus `register/unregister/clear` tests prove hooks are mutable at runtime. | `lib/crewai/tests/hooks/test_tool_hooks.py:52-69,465-494` |
| Documentation: human approval gate recipe | "Human Approval Gate" example: hard-coded `approval_required` list inside a `@before_tool_call` hook. | `docs/edge/en/learn/tool-hooks.mdx:181-204` |
| Documentation: destructive-tools list | `safety_check` example: hard-coded `destructive_tools` list returning `False`. | `docs/edge/en/learn/tool-hooks.mdx:158-179` |
| Documentation: rate limiting recipe | Hook-based rate limiter keyed on `context.tool_name`. | `docs/edge/en/learn/tool-hooks.mdx:301-328` |
| A2A authorization (out-of-scope, but worth noting) | `A2AErrorCode.AUTHORIZATION_FAILED` and `AuthorizationFailedError` are protocol-level, not tool-risk. | `lib/crewai/src/crewai/a2a/errors.py:79-80,336-348` |
| Identity-only SecurityConfig exports | `__all__ = ["Fingerprint", "SecurityConfig"]`; module docstring says "Future: authentication, scoping, and delegation mechanisms". | `lib/crewai/src/crewai/security/__init__.py:1-14` |

## Answers to Dimension Questions

1. **Are tools risk-classified?**
   No. Neither `BaseTool` (`tools/base_tool.py:139-191`), `Tool` (same file), nor `CrewStructuredTool` (`tools/structured_tool.py:115-142`) exposes a risk field. No read-only / write / delete / network / money enum exists in the framework. The only categorization is the user-supplied `destructive_tools` / `sensitive_tools` arrays in the docs example (`docs/edge/en/learn/tool-hooks.mdx:162-174`). `apps` and `mcps` on the agent (`agents/agent_builder/base_agent.py:366-373`) function as coarse allow-lists but not as risk metadata.

2. **Are permissions enforced?**
   Only if a hook is registered. Default behavior is "allow all". The four runtime enforcement points (`utilities/tool_utils.py:105-114,225-234`, `agents/crew_agent_executor.py:954-979`, `experimental/agent_executor.py:1953-1977`, `utilities/agent_utils.py:1508-1528`) all iterate `get_before_tool_call_hooks()`; a `False` return blocks the tool with `"Tool execution blocked by hook. Tool: ..."`. The built-in usage cap (`base_tool.py:295-312`, `tool_usage.py:740-757`) is the only native enforcement, and it is quota-based, not risk-based.

3. **Can users approve selectively?**
   Yes — by writing a `@before_tool_call(tools=[...], agents=[...])` hook that calls `ToolCallHookContext.request_human_input(...)` and returns `False` on deny (`hooks/tool_hooks.py:79-121`, `hooks/decorators.py:208-249`, `docs/edge/en/learn/tool-hooks.mdx:181-204`). Filter scope (tools + agents) is supported (`tests/hooks/test_decorators.py:215-237`). There is no UI prompt beyond `builtins.input()` (`hooks/tool_hooks.py:114`), no async-native prompt by default — the async helper `_async_readline` falls back to `asyncio.to_thread(input)` (`core/providers/human_input.py:425-442`).

4. **Are approvals persisted?**
   No. Hooks live in module-level lists (`hooks/tool_hooks.py:124-125`); they are not serialized, checkpointed, or reloaded. `clear_all_tool_call_hooks()` is provided (`hooks/tool_hooks.py:304-317`) but only as a teardown helper for tests. There is no approval ledger or audit log; only the existing `ToolUsageStartedEvent`/`ToolUsageFinishedEvent`/`ToolUsageErrorEvent` telemetry (`tools/tool_usage.py:273,455,996`) record tool calls.

5. **Can policy block a model-requested tool?**
   Yes, **only if a hook is registered**. If no hook targets the tool name, the model gets an unmediated call. The `before_tool_call` hook returning `False` does block (`utilities/tool_utils.py:228-234`), but this is opt-in. Default-installed agents have no policy.

## Architectural Decisions

- **Hooks-as-policy** — CrewAI's design choice to make before/after-tool-call hooks the *only* native approval surface, instead of declaring risk metadata on the tool itself. The pattern is well-documented (`docs/edge/en/learn/tool-hooks.mdx`) and the implementation is uniform across the four execution paths.
- **Global hook registry** — Hooks are stored as module-level Python lists (`hooks/tool_hooks.py:124-125`) rather than attached to `Agent` or `Crew` instances. Crew-scoped hooks are still added to the same global list, just optionally wrapped with `_filter_tools`/`_filter_agents` predicates (`project/crew_base.py:524-555`). This simplifies enforcement (one lookup per call site) but eliminates any per-session isolation.
- **Identity ≠ permission** — `SecurityConfig` (`security/security_config.py:20-67`) is deliberately scoped to identity (a UUID fingerprint with metadata); the docstring marks authentication/scoping/delegation as `*TODO*` (`security/security_config.py:25-29`). This makes the framework safe to talk about in compliance contexts but leaves the user to build real policy.
- **Usage cap as a soft quota, not a permission** — `max_usage_count` is exposed as a per-instance int field (`base_tool.py:184-187`) and enforced under a `threading.Lock` (`base_tool.py:295-312`, `tool_usage.py:740-757`). It is a resource-cap, not a risk gate — there is no semantic distinction between "blocked because dangerous" and "blocked because quota exceeded".
- **Tool schema is opaque to LLM for permission** — The `args_schema`/JSON-schema is what the LLM sees (`base_tool.py:482-490`); permission metadata is not part of that surface, so the model has no way to "know" a tool is dangerous.

## Notable Patterns

- **Hook context with mutable input dict** — `ToolCallHookContext.tool_input` is intentionally a live reference (`hooks/tool_hooks.py:32-37`); hooks must mutate in place (`tests/hooks/test_tool_hooks.py:113-126`).
- **Filter-driven hook scoping** — `@before_tool_call(tools=[...], agents=[...])` decorates the function with `_filter_tools`/`_filter_agents` attributes (`hooks/decorators.py:52-56`), and the registry wrapper consults them (`hooks/decorators.py:60-69`). This is reused by `CrewBase` for crew-scoped hooks (`project/crew_base.py:524-555`).
- **First-block-wins, sequential iteration** — Hooks run in registration order; the first `False` short-circuits the rest and the tool call (`utilities/tool_utils.py:107-114`, `tests/hooks/test_tool_hooks.py:340-376`).
- **Synchronous stdin as the approval surface** — `ToolCallHookContext.request_human_input` calls `builtins.input()` (`hooks/tool_hooks.py:114`), with the event listener's live UI paused around it (`hooks/tool_hooks.py:109-121`). Async equivalent uses `asyncio.StreamReader` with a `to_thread` fallback (`core/providers/human_input.py:425-442`).
- **Skill-level allow-list as decorative metadata** — `SkillFrontmatter.allowed_tools` parses a YAML string into a list (`skills/models.py:78-93`) but the docs explicitly warn it does not provision tools or gate calls (`docs/v1.14.0/pt-BR/concepts/skills.mdx:296`).

## Tradeoffs

- **Flexibility vs. safety-by-default.** Because there is no default policy, a careless user gets no protection at all. Documentation compensates with extensive copy-paste recipes (`docs/edge/en/learn/tool-hooks.mdx`), but recipes are not code that ships.
- **Global registry vs. per-session isolation.** A global hook list is simple to enforce but means hooks leak across agents/crews unless filtered, and there is no way to scope hooks to a single run. `clear_all_tool_call_hooks()` exists (`hooks/tool_hooks.py:304-317`) but is mostly used in test fixtures (`tests/hooks/test_tool_hooks.py:52-69`).
- **Stdin prompt vs. UI prompt.** Using `builtins.input()` works in a CLI but blocks any non-terminal deployment (web server, notebook, headless run) and is not async-native (`core/providers/human_input.py:425-442` falls back to `to_thread`). There is no protocol-level approval (HTTP/webhook) shipping in the framework.
- **Identity without policy.** `SecurityConfig` looks like a permission layer but is fingerprint-only (`security/security_config.py:20-67`). Operators can build policy on top of the fingerprint, but nothing in the runtime enforces it.
- **No persistence.** Approvals die with the process; there is no audit log beyond the existing `ToolUsageStartedEvent`/`ToolUsageFinishedEvent` events (`tools/tool_usage.py:273,996`).

## Failure Modes / Edge Cases

- **No hooks registered → no protection.** A new user who wires `tools=[...]` and never registers a `@before_tool_call` hook has zero permission enforcement. The four enforcement sites all run `for hook in get_before_tool_call_hooks()` — an empty list is a no-op (`utilities/tool_utils.py:107`, `agents/crew_agent_executor.py:965`, `experimental/agent_executor.py:1964`, `utilities/agent_utils.py:1518`).
- **Hook exceptions are swallowed.** All four sites wrap the iteration in `try/except` and log (`utilities/tool_utils.py:115-116`, `agents/crew_agent_executor.py:970-975`, `experimental/agent_executor.py:1969-1974`). A buggy hook silently *allows* the call. There is no fail-closed default.
- **Tool name fuzzy matching before hook runs.** `ToolUsage._select_tool` uses `SequenceMatcher` with a 0.85 ratio threshold (`tools/tool_usage.py:759-802`), so a hook that matches an exact name can miss a near-match. Hook filters are applied by exact name match (`hooks/decorators.py:60-63`), so a fuzzy-matched tool name can bypass the hook's deny list.
- **Native vs. text-parsing paths.** `ToolUsage._use`/`_ause` (`tools/tool_usage.py:469-707,222-467`) does **not** call `get_before_tool_call_hooks()` — only the `execute_tool_and_check_finality` / `utilities/agent_utils` / `crew_agent_executor` paths do. If a call site bypasses those (e.g., direct invocation through `Tool.run`/`arun`/`Tool._run`), no hook fires. Same for `CrewStructuredTool.func(...)` direct invocation.
- **`max_usage_count` is instance-scoped, not agent-scoped.** If the same tool instance is shared across agents, the quota is shared; if each agent gets its own tool, quotas are independent (`base_tool.py:184-191,295-312`). There is no agent-aware quota.
- **`request_human_input` is blocking.** `builtins.input()` blocks the event loop (`hooks/tool_hooks.py:114`); in async contexts, `asyncio.to_thread` is used (`core/providers/human_input.py:441`), which can break tests that mock `builtins.input` if they call into a coroutine path.
- **Global state leaks in tests.** The autouse `clear_hooks` fixture in `tests/hooks/test_tool_hooks.py:52-69` exists precisely because `_before_tool_call_hooks`/`_after_tool_call_hooks` survive across tests if not reset.
- **`SkillFrontmatter.allowed_tools` is non-binding.** Documentation explicitly warns users that the field "does not provision or inject any tool" (`docs/v1.14.0/pt-BR/concepts/skills.mdx:296`), so users who trust it as an allow-list get no enforcement.

## Future Considerations

- Add a first-class `risk` / `side_effects` enum to `BaseTool` (`tools/base_tool.py:139-191`) so the LLM schema and the runtime can reason about risk together. Today the schema is silent on risk.
- Add an `allowed_agents` / `required_role` field on `BaseTool` to enable declarative per-agent allow-listing instead of user-written filter hooks.
- Promote `SkillFrontmatter.allowed_tools` from decorative metadata to an enforced policy (`skills/models.py:78-93`).
- Provide a default-shipped "human approval" hook that targets tools tagged as `high_risk` so users get protection out of the box.
- Replace the global hook lists with a context-var or per-execution-context registry so cross-session leakage is impossible.
- Add an async-native approval surface (webhook / queue) alongside `request_human_input`; `core/providers/human_input.py:425-442` is a step in that direction but only at the executor (final-answer) level, not the tool-call level.
- Wire `SecurityConfig` into tool invocation so the fingerprint identity becomes a policy lever, not just telemetry metadata (`tools/tool_usage.py:1023-1054` already forwards it; nothing gates on it).

## Questions / Gaps

- **No evidence found** of any built-in policy file (e.g., a YAML/JSON allow-list loader). Searched `lib/crewai/src/crewai/` for `allow_list`, `deny_list`, `whitelist`, `blacklist`, `tool_policy`, `tool_role`, `require_role`, `scope` — no policy file type found.
- **No evidence found** that hook decisions are emitted as durable events. `ToolUsageErrorEvent` is emitted on tool errors (`tools/tool_usage.py:965-973`) but a successful denial is returned only as a `ToolResult(result=blocked_message, result_as_answer=False)` (`utilities/tool_utils.py:230-234`) with no specific "ToolDeniedByPolicyEvent".
- **No evidence found** of any built-in tool classification (read-only, write, network, secret, money). Users must classify tools themselves in hook code (`docs/edge/en/learn/tool-hooks.mdx:160-179`).
- **No evidence found** of selective per-tool persistence (e.g., "user approved `send_email` for the next hour"). Hooks cannot remember prior decisions; each invocation is fresh.
- **No evidence found** of a server-mode approval (webhook/HTTP). The only approval surface is stdin (`hooks/tool_hooks.py:114`) — `core/providers/human_input.py` is executor-level, not tool-level.
- **No evidence found** of tool metadata being sent to the LLM in a "this tool is dangerous" form. The `_generate_description` method only emits `Tool Name`, `Tool Arguments` (JSON schema), and `Tool Description` (`tools/base_tool.py:482-490`) — no risk annotations.

---

Generated by `dimension_file` against `crewai`.
