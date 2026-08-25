# Source Analysis: letta

## Capability Model and Trust Boundaries (Dimension 08.01)

### Source Info

| Field | Value |
|-------|-------|
| Name | letta |
| Path | `studies/agent-harness-study/sources/letta` |
| Language / Stack | Python (FastAPI server, SQLAlchemy ORM, pydantic schemas); sandbox execution via subprocess / E2B / Modal; TypeScript tools supported |
| Analyzed | 2026-08-24 |

> Citation convention: all paths below are relative to the source root `studies/agent-harness-study/sources/letta/`.

## Summary

Letta's capability model is organized around a typed tool registry with a **type-dispatched executor factory** (`letta/services/tool_executor/tool_execution_manager.py:32-65`): core memory tools run in-process against the database, "custom" tools default to a pluggable sandbox chain (Modal → E2B → local subprocess, `letta/services/tool_executor/sandbox_tool_executor.py:71-130`), MCP and Composio tools proxy to external services, and built-ins (`run_code`, `web_search`, `fetch_webpage`) execute server-side. The model itself never executes anything directly: it emits tool calls, and the agent loop enforces an allowlist derived from a first-class **tool rules** system (9 rule types, `letta/schemas/tool_rule.py:360-373`) before dispatching to executors.

The primary human-in-the-loop boundary is the **approval flow**: tools marked `RequiresApprovalToolRule` or declared as client-side tools are intercepted before execution and converted into an `approval_request_message` with stop reason `requires_approval` (`letta/agents/letta_agent_v3.py:1681-1709`); only calls approved by the client in a follow-up message are executed (`letta/agents/letta_agent_v3.py:975-1017`). Approval is opt-in per tool via `default_requires_approval` (`letta/schemas/tool.py:59,127`), auto-materialized as a rule when such a tool is attached to an agent (`letta/services/agent_manager.py:2810-2818`).

Trust boundaries between tenants are enforced as organization-scoped queries threaded through every manager via an `actor: User` parameter (e.g., tool attach checks `ToolModel.organization_id == actor.organization_id`, `letta/services/agent_manager.py:2777-2785`). Secrets are wrapped in an encrypted-at-rest `Secret` value type (AES-256-GCM, `letta/helpers/crypto_utils.py:104-150`; masked reprs, `letta/schemas/secret.py:271-279`), and MCP server URLs pass an SSRF filter that rejects internal hostnames and non-global IPs (`letta/helpers/url_validation.py:31-82`). Stdio MCP servers — which spawn local processes — are disabled by default for multi-tenant safety (`letta/settings.py:45-54`).

The weakest boundary is deployment-default: HTTP auth is optional (a shared-password middleware is only installed when `LETTA_SERVER_SECURE=true`, `letta/server/rest_api/app.py:797-799`), actor identity comes from a caller-supplied `user_id` header with an auto-created default actor fallback (`letta/server/rest_api/dependencies.py:39`, `letta/services/user_manager.py:113-135`), and the default LOCAL sandbox copies the entire server OS environment into the tool subprocess (`letta/services/tool_sandbox/base.py:487`).

**Rating: 6 / 10**

## Rating

**6 / 10** — Present but inconsistent.

Rationale: The capability *model* is explicit and well-engineered — typed executor dispatch (`tool_execution_manager.py:35-43`), a declarative tool-rules language with runtime enforcement (`letta/helpers/tool_rule_solver.py:96-125`, violation blocking at `letta/agents/letta_agent_v3.py:1780,1825-1827`), approval-gated execution with integration tests (`tests/integration_test_human_in_the_loop.py:253,654`), and encrypted secret handling with tests (`tests/test_crypto_utils.py:16,50`). This earns the 7–8 band's "clear model with tests and explicit interfaces." However, several operational trust boundaries are fragile or absent by default: unauthenticated-by-default serving, header-asserted identity, full OS-env leakage into local sandboxes, no SSRF guard on `fetch_webpage`, and dead legacy auth endpoints referencing methods that no longer exist (`letta/server/rest_api/auth/index.py:36,39` vs `class SyncServer` at `letta/server/server.py:114`). These inconsistencies pull the overall score down to 6.

## Evidence Collected

Every entry cites a file path with line numbers relative to `studies/agent-harness-study/sources/letta/`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Tool type → executor dispatch | `ToolExecutorFactory._executor_map` maps LETTA_CORE/CORE_MEMORY/SLEEPTIME→core executor, MULTI_AGENT→sandbox, BUILTIN→builtin, FILES→files, EXTERNAL_MCP→MCP executor; unknown types fall back to `SandboxToolExecutor` | letta/services/tool_executor/tool_execution_manager.py:35-57 |
| Full built-in capability inventory | `BASE_TOOLS`, `BASE_MEMORY_TOOLS` (v1/v2/v3), `MULTI_AGENT_TOOLS`, `LOCAL_ONLY_MULTI_AGENT_TOOLS`, `BUILTIN_TOOLS`, `FILES_TOOLS`, union `LETTA_TOOL_SET` | letta/constants.py:112-179 |
| Model can only request | Tool calls flagged requires-approval or client-side are split out of the executed set; an `ApprovalRequestMessage` is persisted and the step stops with `StopReasonType.requires_approval` | letta/agents/letta_agent_v3.py:1681-1709; letta/schemas/enums.py:197 |
| Approval resolution | Approved IDs extracted from `ApprovalReturn.approve`; denied calls become denials; empty approvals treated as malformed and abort the step | letta/agents/letta_agent_v3.py:982-1017 |
| Approval advertised to LLM adapter | `requires_approval_tools` passed into request construction so provider-side gating/prompt reflects them | letta/agents/letta_agent_v3.py:1155-1163 |
| Client-side tools | `ClientToolSchema` accepted per-request (`client_tools` field); client tools override same-named server tools in the exposed schema | letta/schemas/letta_request.py:12,75; letta/agents/letta_agent_v3.py:2047-2065 |
| Runtime rule enforcement | `tool_rule_violated = name not in valid_tool_names`; violated specs return a rule-violation result instead of executing | letta/agents/letta_agent_v3.py:1780,1821-1827 |
| Allowlist computation | `get_allowed_tool_names` intersects Child/Parent/Conditional/MaxCount rule sets; Init rules constrain first call | letta/helpers/tool_rule_solver.py:96-125 |
| Rule taxonomy | ChildToolRule, ParentToolRule, ConditionalToolRule, InitToolRule, TerminalToolRule, ContinueToolRule, RequiredBeforeExitToolRule, MaxCountPerStepToolRule, RequiresApprovalToolRule | letta/schemas/tool_rule.py:64-357,360-373 |
| Parallel-call suppression under rules | Provider request fields forced off when tool rules present (Anthropic `disable_parallel_tool_use`, OpenAI `parallel_tool_calls=False`) | letta/agents/letta_agent_v3.py:1111-1146 |
| Per-tool approval metadata + auto-rule | `default_requires_approval` on tool create/update schemas; attaching such a tool appends `RequiresApprovalToolRule` to the agent | letta/schemas/tool.py:59,127,203; letta/services/agent_manager.py:2810-2818 |
| Hallucinated tool rejection | `_execute_tool` returns error "Tool not found" if the call is not among attached tools | letta/agents/letta_agent_v2.py:1301-1306; letta/agents/letta_agent.py:1936-1942 |
| Local sandbox = bare subprocess | Generated script written to temp file, run via `asyncio.create_subprocess_exec` with 180 s timeout (`tool_settings.tool_sandbox_timeout`) then terminate/kill | letta/services/tool_sandbox/local_sandbox.py:192-207; letta/settings.py:36 |
| OS env leakage to local sandbox | `_gather_env_vars(..., is_local=True)` starts from `os.environ.copy()`; remote sandboxes start from `{}` | letta/services/tool_sandbox/base.py:479-487 |
| Sandbox gets platform credentials | Generated script initializes a `Letta` client from `LETTA_API_KEY`; env layering injects `LETTA_AGENT_ID/PROJECT_ID/TOOL_ID` | letta/services/tool_sandbox/base.py:235-255,508-513 |
| Agent-state injection into sandbox | `pickle.dumps(agent_state)` embedded into the generated script when the tool declares `agent_state` param | letta/services/tool_sandbox/base.py:85-90,156,229-232 |
| Nested-execution guard | Deep-copied agent state passed to sandbox has `tools=[]` and `tool_rules=[]` stripped; post-run assertion that memory was untouched | letta/services/tool_executor/sandbox_tool_executor.py:135-138,171-178 |
| Modal opt-in gate | Modal used only if tool metadata `sandbox == "modal"` AND `tool_settings.modal_sandbox_enabled`; failure falls back to E2B/local | letta/services/tool_executor/sandbox_tool_executor.py:71-99 |
| E2B lifecycle | Fresh `AsyncSandbox` per run (fingerprint metadata), env vars passed explicitly (`is_local=False`), killed in `finally` | letta/services/tool_sandbox/e2b_sandbox.py:71,81,102,171-173 |
| Default sandbox selection | E2B if `e2b_api_key` set else LOCAL fallback | letta/settings.py:61-71 |
| Built-in code exec requires E2B | `run_code_with_tools`/`run_code` raise if `tool_settings.e2b_api_key is None`; inject agent tool sources or client-based stubs into executed code | letta/services/tool_executor/builtin_tool_executor.py:52-67,100-125,148-177 |
| Network egress (built-ins) | `web_search` uses Exa key from agent env or settings; `fetch_webpage` validates scheme http/https only (no private-IP/host blocklist) | letta/services/tool_executor/builtin_tool_executor.py:233-237,330-336 |
| MCP SSRF policy | `validate_mcp_server_url` blocks localhost/cloud-metadata/.local/.svc/.cluster.local and non-global IPs incl. resolved addresses; applied at schema validation and connection time | letta/helpers/url_validation.py:5-18,44-80; letta/schemas/mcp_server.py:40-60; letta/services/mcp_manager.py:821 |
| stdio MCP disabled by default | `mcp_disable_stdio: bool = True` with rationale about multi-tenant deployments | letta/settings.py:45-54 |
| MCP routing & auth storage | Tools tagged `mcp_server:<name>` route to `execute_mcp_server_tool` scoped by actor; tokens/custom headers stored as encrypted `Secret` | letta/services/tool_executor/mcp_tool_executor.py:36-58; letta/services/mcp_server_manager.py:454,460 |
| Org-scoped authority | Attach/delete checks filter by `actor.organization_id`; all managers take `actor: User` | letta/services/agent_manager.py:2776-2785 |
| Cross-agent messaging scope | Multi-agent tool docstring limits targets to "within the same organization"; executes through Letta client with org-scoped API | letta/functions/function_sets/multi_agent.py:60-97 |
| Env-scoped capability gating | `send_message_to_agent_async` excluded from base tool upsert when `settings.environment == "prod"` | letta/constants.py:155; letta/services/tool_manager.py:650-657 |
| Secrets encryption | AES-256-GCM, PBKDF2-HMAC-SHA256 (100k iters) from `LETTA_ENCRYPTION_KEY`; provider keys persisted via `Secret.from_plaintext` | letta/helpers/crypto_utils.py:56-59,104-150; letta/server/server.py:217-370 |
| Secret hygiene | `Secret.__str__/__repr__` mask values; falls back to plaintext storage with warning when no encryption key configured | letta/schemas/secret.py:60-68,271-279 |
| Env var secrets | Sandbox/agent environment variables carry `value_enc: Secret` with `repr=False` plaintext; encrypted on write, decrypted only at execution | letta/schemas/environment_variables.py:14,20; letta/services/sandbox_config_manager.py:215,329 |
| Credentials webhook | Optional external service supplies per-tool/per-agent sandbox credentials keyed by user/org IDs | letta/services/sandbox_credentials_service.py:19-70 |
| Auth optional by default | `CheckPasswordMiddleware` added only when `LETTA_SERVER_SECURE == "true"` or `--secure`; middleware accepts single shared password via Bearer or X-BARE-PASSWORD | letta/server/rest_api/app.py:797-799; letta/server/rest_api/middleware/check_password.py:10-31 |
| Identity = caller-supplied header | Actor resolved from `user_id` header pattern-validated but unauthenticated; default actor auto-created unless `no_default_actor` set | letta/server/rest_api/dependencies.py:38-61; letta/services/user_manager.py:113-135 |
| Legacy/dead auth path | `/auth` handler calls `server.api_key_to_user` / `server.authenticate_user`, neither defined on `SyncServer`; bearer-token dependency `get_current_user` referenced by no router | letta/server/rest_api/auth/index.py:26-40; letta/server/rest_api/auth_token.py:11-18; letta/server/server.py:114 |
| Approval tests | Approve flow asserts `approval_request_message`, pending_approval state transitions; deny flow; toggling `requires_approval` off re-enables direct execution | tests/integration_test_human_in_the_loop.py:253-292,319-332,654 |
| Client-tool pause/resume tests | Full flow asserts `stop_reason == "requires_approval"` then resume via `approval` message with `ToolReturn` | tests/integration_test_client_side_tools.py:67-141 |
| Crypto tests | Roundtrip, wrong-key failure, plaintext-detection cases | tests/test_crypto_utils.py:16,50,74 |

## Answers to Dimension Questions

### 1. What can the agent do?

The agent's total capability surface is exactly the set of tools attached to its state plus client-declared tools. The built-in inventory spans: conversation/memory primitives (`send_message`, `conversation_search`, archival search/insert — `letta/constants.py:112-118`), self-editing memory tools (`memory_replace/insert/rethink/...`, dispatched in `letta/services/tool_executor/core_tool_executor.py:41-56`), multi-agent messaging (`MULTI_AGENT_TOOLS`, `letta/constants.py:154`), files tools operating on uploaded sources (`open_files`, `grep_files`, `semantic_search_files` — `letta/constants.py:170`, executor at `letta/services/tool_executor/files_tool_executor.py:110`), arbitrary code execution (`run_code`, `run_code_with_tools` — E2B-hosted, `letta/services/tool_executor/builtin_tool_executor.py:52-177`), web access (`web_search`, `fetch_webpage` — executed in-process on the server, `builtin_tool_executor.py:193-399`), user-defined custom tools (sandbox chain, `sandbox_tool_executor.py:101-130`), external MCP tools (`mcp_tool_executor.py:51-58`), and Composio actions (`composio_tool_executor.py:43-45`). Memory writes go directly through managers scoped to `actor` (e.g., `core_memory_append` → `BlockManager` update, `core_tool_executor.py:319-344`).

### 2. What can the model only request but not directly do?

Three classes of calls are requests, not actions: (a) tools carrying a `RequiresApprovalToolRule` (or `default_requires_approval`), (b) client-side tools declared per-request via `client_tools` (`letta/schemas/letta_request.py:75`), and (c) any tool outside the current allowlist computed from tool rules. Categories (a)/(b) produce an `ApprovalRequestMessage` and halt with `requires_approval` (`letta_agent_v3.py:1686-1709`); execution happens only after the client returns approvals (`letta_agent_v3.py:982-992`). Category (c) never executes at all — the loop substitutes a rule-violation error result back into context (`letta_agent_v3.py:1780,1825-1827`). Additionally, the model cannot invent capabilities: calls not among attached tools fail with "Tool not found" (`letta_agent_v2.py:1301-1306`). Note the asymmetry: for ordinary server tools the model's "request" *is* the action — there is no global ask-before-run mode.

### 3. Where is authority enforced?

Layered:

1. **LLM request shaping** — only allowed tools' schemas are sent (`_get_valid_tools`, `letta_agent_v3.py:2050-2074`); parallel calls disabled when rules exist (`letta_agent_v3.py:1111-1146`).
2. **Post-generation gate** — allowlist check before dispatch (`letta_agent_v3.py:1780`).
3. **Executor resolution** — fixed type→executor map; unknown types default to the sandbox executor (`tool_execution_manager.py:35-57`).
4. **Sandboxing of custom code** — Modal (opt-in) / E2B (per-run VM, killed after) / local subprocess (`sandbox_tool_executor.py:77-130`, `e2b_sandbox.py:173`, `local_sandbox.py:192-207`).
5. **Data-layer scoping** — org/user filters on every manager query using `actor` (`agent_manager.py:2776-2785`).
6. **Network policy** — SSRF filter on MCP URLs (`url_validation.py:31-82`); stdio transport disabled by default (`settings.py:45-54`).
7. **Transport auth** — optional shared-password middleware (`check_password.py:10-31`).

### 4. Are dangerous capabilities isolated?

Partially, and inconsistently. Arbitrary code execution is well-isolated when E2B or Modal is configured (fresh VM/function per run; E2B always killed, `e2b_sandbox.py:171-173`; nested tool execution prevented by stripping tools from the injected state, `sandbox_tool_executor.py:171-178`). But the fallback default is a **local subprocess on the server host** (`local_sandbox.py:192-193`) whose environment begins as a copy of the entire server process environment (`base.py:487`) — including DB URIs, provider keys, and `LETTA_API_KEY` deliberately provided so tools can call back into the platform (`base.py:235-255,508-513`). There is no filesystem, network, or syscall restriction visible on this path; isolation reduces to process boundaries plus a 180-second timeout (`local_sandbox.py:196-207`). Server-side built-ins (`fetch_webpage`) perform outbound fetches from the server process with only scheme validation (`builtin_tool_executor.py:330-336`), unlike MCP which has a full SSRF guard. Dangerous-capability isolation therefore depends heavily on deployment choices rather than being structural.

## Architectural Decisions

1. **Type-driven executor factory instead of a permission engine.** Capability containment is achieved by choosing *where* code runs per tool type (`tool_execution_manager.py:35-43`), not by evaluating permissions at call time. Simple, auditable, but binary: once an executor is selected, no further authorization applies inside it.
2. **Tool rules as a declarative constraint DSL.** Nine typed rules (`tool_rule.py:360-373`) cover sequencing (`ChildToolRule` with arg prefill), gating (`ParentToolRule`), branching on outputs (`ConditionalToolRule`), budget (`MaxCountPerStepToolRule`), loop control (`Init/Terminal/Continue/RequiredBeforeExit`), and human-in-the-loop (`RequiresApprovalToolRule`). The solver computes an allowlist each step (`tool_rule_solver.py:96-125`) and the loop blocks violations (`letta_agent_v3.py:1780`) — prompt hints are advisory, enforcement is separate.
3. **Human-in-the-loop modeled as message protocol, not a callback.** Approval pauses are persisted messages (`ApprovalRequestMessage`, `letta/schemas/letta_message.py:306`) with resumable state; clients answer with typed `ApprovalReturn`/`ToolReturn` payloads (`letta/schemas/message.py:178-191`). This makes approvals durable and multi-client friendly.
4. **Org-first tenancy.** Every primitive carries `organization_id` and managers thread `actor` everywhere (`schemas/user.py:20`, `agent_manager.py:2777-2785`); agents can message other agents in-org via sandbox-injected clients (`multi_agent.py:62`).
5. **Secrets as a value type.** Encryption is pushed into the `Secret` pydantic type with masked serialization (`secret.py:271-279,291-343`), applied uniformly to provider keys, MCP tokens, custom headers, and sandbox env vars.
6. **Deployment-profile capability gating.** Some capabilities are removed entirely in prod builds (`LOCAL_ONLY_MULTI_AGENT_TOOLS` excluded when `environment == "prod"`, `tool_manager.py:650-657`) and stdio MCP is default-off (`settings.py:45-54`) — acknowledging that some powers are unsafe for shared hosting.

## Notable Patterns

- **Allowlist + deny-by-default at the loop level:** valid names come from the solver; everything else short-circuits to an error result fed back to the model (`letta_agent_v3.py:1780-1827`).
- **Capability advertisement:** `requires_approval_tools` are passed into LLM request construction so providers/models see them distinctly (`letta_agent_v3.py:1155-1163`).
- **State-stripping for delegation:** sandboxed tools receive an agent-state copy with `tools=[]` and `tool_rules=[]`, cutting recursion paths (`sandbox_tool_executor.py:171-178`), plus an integrity assert that sandbox runs cannot mutate memory (`sandbox_tool_executor.py:135-138`).
- **Fingerprinted sandbox configs:** E2B sandboxes carry a config fingerprint in metadata (`e2b_sandbox.py:184-200`) and results echo `sandbox_config_fingerprint`, giving provenance for what environment executed a tool (`local_sandbox.py:230`).
- **Defense-in-depth on output parsing:** local-sandbox results are framed with UUID markers + length + MD5 checksum to survive noisy stdout (`base.py:26-27`, `local_sandbox.py:253-275`).
- **Env-var identity injection:** sandboxes learn their agent/project/tool IDs through `LETTA_AGENT_ID/PROJECT_ID/TOOL_ID` rather than parameters (`base.py:508-513`).

## Tradeoffs

- **Developer convenience vs blast radius (LOCAL sandbox):** running tools as host subprocesses with inherited `os.environ` (`base.py:487`) maximizes compatibility for single-user/desktop use but hands agent-authored code the server's secrets in multi-tenant misconfigurations. E2B avoids this by passing only gathered envs (`base.py:487` else-branch, consumed at `e2b_sandbox.py:102`).
- **Opt-in approvals vs safe defaults:** approval gating exists and is tested, but nothing requires dangerous tools to be enrolled; `default_requires_approval` is nullable (`schemas/tool.py:59`).
- **Encryption with plaintext fallback:** `Secret.from_plaintext` stores plaintext (with a warning) when `LETTA_ENCRYPTION_KEY` is unset (`secret.py:60-68`), prioritizing availability over confidentiality; heuristic `is_encrypted` detection is acknowledged as unreliable (`crypto_utils.py:303-322`).
- **Header identity vs real authz:** pattern-validating a `user_id` header (`dependencies.py:57-61`) keeps the API simple for embedded/single-user scenarios but delegates all real authentication to reverse proxies or the optional password middleware.
- **In-process built-ins vs isolation consistency:** `web_search`/`fetch_webpage` run on the server itself (`builtin_tool_executor.py:269-281,373-394`) for latency, sacrificing the sandbox discipline applied to custom tools and skipping the SSRF treatment given to MCP.

## Failure Modes / Edge Cases

- **Local sandbox env exfiltration:** a malicious custom tool in a LOCAL-fallback deployment can read the whole server environment (`base.py:487`) and phone home; nothing in the local path restricts network or file access.
- **Unauthenticated deployment:** without `LETTA_SERVER_SECURE`, all routers (including internal `/_internal_agents`) are open (`app.py:797-799`, `internal_agents.py:12`); with secure mode, one shared password grants full admin-equivalent access (`check_password.py:23-27`).
- **Identity spoofing behind weak auth:** any caller can set `user_id` to another actor within reach of the deployment since actor resolution trusts the header (`dependencies.py:39`, `user_manager.py:126`).
- **Legacy auth crash surface:** `/auth` invokes `SyncServer.api_key_to_user`, which does not exist (`auth/index.py:36` vs `server.py:114`) — hitting the endpoint with a non-matching password raises AttributeError (500), and the unused `get_current_user` (`auth_token.py:11-18`) suggests an abandoned per-user API-key model.
- **SSRF gap in built-in web tools:** `fetch_webpage` accepts public-looking hosts that resolve internally (no `validate_mcp_server_url` equivalent, `builtin_tool_executor.py:330-336` vs `url_validation.py:31-82`).
- **Malformed approval payloads:** handled defensively — all-empty approvals abort the step with `invalid_tool_call` rather than executing anything (`letta_agent_v3.py:1007-1017`).
- **Timeout kill window:** local sandbox terminate→kill escalation leaves a 5-second grace period where a runaway process still lives (`local_sandbox.py:198-205`); E2B has no per-run timeout shown beyond config defaults.
- **Pickle as inter-trust-domain transport:** pickled `AgentState` is embedded into sandbox scripts (`base.py:156,230`); `safe_pickle.py` guards size/recursion for robustness, not deserialization safety (`safe_pickle.py:1-18`) — acceptable because flow direction is server→sandbox, but fragile if ever reversed.

## Future Considerations

1. **Make the local sandbox visibly degraded:** refuse custom-tool execution in multi-tenant mode unless E2B/Modal is configured, or scrub `os.environ` to an explicit allowlist (`base.py:479-516`) — currently the risky default is silent.
2. **Replace header identity with token-derived actors:** bind actors to issued API keys (the vestigial `api_key_to_user` path) and drop the default-actor fallback (`user_manager.py:122-135`) for hosted environments.
3. **Extend the MCP SSRF validator to all outbound fetches**, including `fetch_webpage`/Exa fetch fallbacks (`builtin_tool_executor.py:312-399`), ideally with DNS-rebinding-safe pinning (resolve-once-and-connect-to-IP).
4. **First-class capability manifests per agent:** derive `RequiresApprovalToolRule`s from a risk classification (network egress, code exec, spend) rather than per-tool author flags (`schemas/tool.py:59`).
5. **Observability of denials:** rule violations and approval denials already flow back as tool results (`letta_agent_v3.py:1752-1762`); surfacing them as security metrics (counters exist for executions, `tool_execution_manager.py:156-160`) would make boundary crossings measurable.
6. **Remove or implement dead auth surfaces** (`auth/index.py:36-39`, `auth_token.py`) to eliminate confusing half-built trust paths.

## Questions / Gaps

- **No evidence found** for OS-level sandboxing of the LOCAL executor (no seccomp/AppArmor/container wrapping anywhere in `letta/services/tool_sandbox/local_sandbox.py`); searched for `seccomp|apparmor|docker|nsjail` patterns across the package — absent.
- **No evidence found** for per-tool network egress policies or domain allowlists for custom tools; the only network policy located targets MCP server registration (`url_validation.py`), not tool runtime egress.
- **No evidence found** that the `user_id` header is cross-checked against the presented credential in any router (searched `Depends(security)` usages: none outside the unused `auth_token.py`).
- Whether Letta's hosted offering adds an auth layer in front of this codebase could not be verified from the repository; `SECURITY.md` covers only vulnerability reporting (SECURITY.md:1-15).
- The interaction between `no_default_actor` (`user_manager.py:123-124`) and the password middleware suggests an intended hardening story, but no test exercises them together; `tests/integration_test_human_in_the_loop.py` and client-tool tests run against an authenticated client fixture without probing identity separation.

---

Generated by `dimensions/08.01-capability-model-and-trust-boundaries` against `letta`.
