# Source Analysis: openai-agents-sdk

## 12.02: Prompt Templating and Variable Contracts

### Source Info

| Field | Value |
|-------|-------|
| Name | openai-agents-sdk |
| Path | `studies/agent-harness-study/sources/openai-agents-sdk` |
| Language / Stack | Python 3.10+ (openai-agents SDK, Responses API / Chat Completions / Realtime) |
| Analyzed | 2026-08-25 |

*All citations below are relative to the source root `sources/openai-agents-sdk/`.*

## Summary

The OpenAI Agents SDK deliberately does **not** ship a client-side prompt template engine. Its primary parameterization mechanism delegates templating to the server: an `Agent.prompt` field accepts a `Prompt` TypedDict (`id`, optional `version`, optional `variables`) or a dynamic function returning one (`src/agents/prompts.py:23-33`, `src/agents/agent.py:325-329`), and the resolved object is forwarded verbatim to the OpenAI Responses API as the `prompt` request parameter (`src/agents/models/openai_responses.py:980`). Variable substitution into the template body happens on the OpenAI platform; the SDK only transports the variable map.

Client-side "templating" exists only in internal subsystems (sandbox memory prompts, remote-mount policy instructions, manifest descriptions, event-sink paths) and uses ad-hoc mechanisms: `str.format` on repo-owned `.md` templates (`src/agents/sandbox/memory/prompts.py:105`), chained `str.replace` placeholder substitution (`src/agents/sandbox/memory/prompts.py:64-68`), and f-string concatenation (`src/agents/extensions/handoff_prompt.py:15-19`). There is no shared escaping utility, no per-template variable schema, and validation is limited to structural checks (dict-like/callable shape, return type); missing or wrong variables surface as raw `KeyError`, a `UserError` for wrong backend, or a provider error at request time.

## Rating

**6 / 10 — Present but inconsistent at the edges; clear model on the primary path.**

Rationale against the rubric:

- The public path has a clear model with explicit interfaces (`Prompt` TypedDict, `DynamicPromptFunction`, `PromptUtil.to_model_input`), tests (`tests/test_agent_prompt.py:57-221`), docs (`docs/agents.md:66-123`), and examples (`examples/basic/prompt_template.py:31-67`) — this alone would justify 7.
- It loses points because variable contracts are implicit (no schema tying variables to a template ID, no client-side completeness check), the failure mode for a missing required key is a raw `KeyError` rather than a typed error (`src/agents/prompts.py:79`), behavior differs across backends (silently ignored on Chat Completions by default, `src/agents/models/openai_chatcompletions.py:85-102`), and internal renderers mix three substitution styles with inconsistent failure handling (silent fallback vs. exception).
- Reusability verdict ("Can a prompt template be reused with different variables safely?"): Yes on the primary path — variables travel as structured JSON fields to the server, so reuse across values is injection-safe by construction. Safety of *which* variables are valid is not enforced anywhere client-side.

## Evidence Collected

Every entry cites file paths with line numbers relative to the source root.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Template engine (primary) | No template library in runtime deps; templating delegated to server via Responses API `prompt` param | `pyproject.toml:12-20`; `src/agents/prompts.py:23-33`; `src/agents/models/openai_responses.py:980` |
| Variable schema (public) | `Prompt` TypedDict: `id` required, `version`/`variables` NotRequired; value type re-exported from `openai.types.responses.response_prompt_param.Variables` | `src/agents/prompts.py:8-11,23-33` |
| Dynamic prompt contract | `DynamicPromptFunction = Callable[[GenerateDynamicPromptData], MaybeAwaitable[Prompt]]`; dataclass carries run context + agent | `src/agents/prompts.py:36-48` |
| Resolution + validation | `PromptUtil.to_model_input`: dict pass-through, callable invocation, await handling, non-dict return → `UserError("Dynamic prompt function must return a Prompt")`; `resolved_prompt["id"]` raises bare `KeyError` if absent | `src/agents/prompts.py:58-82` |
| Agent-level shape validation | `__post_init__`: instructions must be str/callable/None; prompt must be dict-like (`hasattr(self.prompt, "get")`)/callable/None | `src/agents/agent.py:457-475` |
| Instructions contract | `instructions: str \| Callable[(RunContextWrapper, Agent), MaybeAwaitable[str]]`; signature enforced to exactly 2 params at resolution time | `src/agents/agent.py:309-323,1045-1055` |
| Injection point: run loop | System prompt and prompt resolved concurrently with sibling cancellation (`gather_with_cancel`), then passed to both streaming and non-streaming model calls | `src/agents/run_internal/run_loop.py:2097-2100,2233,2424-2461,2616` |
| Injection point: Responses payload | `prompt` emitted via `_non_null_or_omit`; when set, `model` omitted unless explicit, empty local tools payload omitted, opaque tool-search surface allowed | `src/agents/models/openai_responses.py:880-926,975-980` |
| Backend restriction | Chat Completions: `_handle_unsupported_prompt` warns once (or raises `UserError` under strict feature validation) and sends `prompt=None` | `src/agents/models/openai_chatcompletions.py:85-102,259,471` |
| Realtime injection point | `model_settings["prompt"]` mapped into session-create `ResponsePrompt(id, variables, version)` | `src/agents/realtime/openai_realtime.py:1808-1815` |
| Internal renderer: format | Memory prompts loaded from cached `.md` files; user message built with `.format(terminal_metadata_json=..., rollout_contents=...)` | `src/agents/sandbox/memory/prompts.py:11-19,100-108` |
| Internal renderer: replace | `render_memory_read_prompt` chains `.replace("{memory_dir}", ...)`, `.replace("{memory_update_instructions}", ...)`, `.replace("{memory_summary}", ...)` | `src/agents/sandbox/memory/prompts.py:53-68,71-97` |
| Escaping-adjacent safeguard | Truncation markers injected into truncated prompt content so the model does not treat partial views as complete | `src/agents/sandbox/memory/phase_one.py:20-26,70-83`; `src/agents/sandbox/manifest_render.py:10-27` |
| Silent fallback behavior | `WorkspaceJsonlSink._resolve_relpath` catches any `template.format` exception and falls back to the literal path | `src/agents/sandbox/session/sinks.py:184-197` |
| MCP prompts (external templating) | `list_prompts()` / `get_prompt(name, arguments)` abstract methods; argument substitution owned by MCP servers | `src/agents/mcp/server.py:638-649,1665-1675` |
| Handoff prefix helper | `prompt_with_handoff_instructions` prepends `RECOMMENDED_PROMPT_PREFIX` via f-string | `src/agents/extensions/handoff_prompt.py:3-19` |
| Tests: public contract | Static/dynamic prompt resolution, pass-through to model, omit-model/tools payload semantics, sibling cancellation on prompt failure | `tests/test_agent_prompt.py:57-109,122-149,153-221` |
| Tests: internal renders | Extra-prompt section omitted/included; removed-rollout listing rendered; truncation marker asserted | `tests/sandbox/test_memory.py:431-445,544-590`; `tests/test_handoff_prompt.py:7-12` |
| Docs | Prompt templates guide: platform-created template with `{{poem_style}}` variable, static and dynamic usage | `docs/agents.md:66-123`; `examples/basic/prompt_template.py:7-39` |

## Answers to Dimension Questions

1. **How are prompts parameterized?**
   Three layers. (a) Public/server-side: `Agent.prompt` carries `{id, version, variables}` to the Responses API; substitution occurs on the platform, documented with a `{{poem_style}}` example (`docs/agents.md:66-93`). (b) Public/client-side code-as-template: `instructions` may be a `(context, agent)` callable producing the system prompt per run (`src/agents/agent.py:1042-1071`), and `prompt` may be a `DynamicPromptFunction` receiving run context and agent (`src/agents/prompts.py:36-48`). (c) Internal string assembly for sandbox/memory/mount-policy instructions using `str.format`, `str.replace`, or f-strings (`src/agents/sandbox/memory/prompts.py:105,114`; `src/agents/sandbox/remote_mount_policy.py:42-46`).

2. **Are variable contracts explicit?**
   Only structurally. The `Prompt` TypedDict makes `id` mandatory and `variables` an untyped-keyed map whose values are constrained only by the imported OpenAI wire type (`src/agents/prompts.py:23-33`). There is no per-template declaration of expected variable names/types, no cross-check between `variables` keys and the referenced platform template, and internal templates declare placeholders only implicitly inside `.md` files (e.g., `{{ extra_prompt_section }}`, `{{ memory_root }}` at `src/agents/sandbox/memory/prompts.py:21-22`) plus hand-written replace/format call sites.

3. **Is missing-variable behavior predictable?**
   Partly. Predictable cases are explicitly handled: dynamic function returning non-dict → `UserError` (`src/agents/prompts.py:75-76`); prompt on Chat Completions → one-time warning or `UserError` under strict validation (`src/agents/models/openai_chatcompletions.py:85-102`); failing dynamic prompt cancels the concurrently resolving instructions coroutine deterministically (`src/agents/run_internal/run_loop.py:2097-2100`; verified by `tests/test_agent_prompt.py:153-221`). Unpredictable/rough cases: a missing `id` raises a bare `KeyError('id')` from dict indexing rather than a domain error (`src/agents/prompts.py:79`); a missing platform-side variable is only detected as an API error at request time — the SDK performs no pre-flight check; and `WorkspaceJsonlSink` silently degrades to a literal (untemplated) path on any formatting exception (`src/agents/sandbox/session/sinks.py:189-196`).

4. **Are variables properly escaped?**
   On the primary path, escaping is moot by design: variables are transmitted as JSON fields alongside the prompt ID, never interpolated into prompt text locally (`src/agents/models/openai_responses.py:980`). For internal renders, untrusted content is inserted as *values*, not re-parsed as templates: `.format(rollout_contents=...)` inserts rollout bytes without recursive formatting (`src/agents/sandbox/memory/prompts.py:105-108`), so braces in content cannot trigger `KeyError`/injection through that call. Two caveats: (a) `render_memory_read_prompt` chains sequential whole-document `.replace()` calls (`src/agents/sandbox/memory/prompts.py:64-68`), so a value substituted earlier (e.g., `memory_dir`) that itself contains a later placeholder such as `{memory_summary}` would be substituted again — order-dependent, undocumented behavior; (b) there is no shared escaping utility module anywhere under `src/agents/` (searched for `jinja|Template\(|substitute|escape` — only the matches listed above). The closest thing to a prompt-hygiene safeguard is truncation-marker injection that prevents models from mistaking truncated content as complete (`src/agents/sandbox/memory/phase_one.py:20-26,74-79`; `src/agents/sandbox/manifest_render.py:11-27`).

## Architectural Decisions

- **Server-owned templating over a local engine.** Runtime dependencies contain no template library (`pyproject.toml:12-20` lists `openai`, `pydantic`, `griffelib`, `typing-extensions`, `requests`, `websockets`, `mcp`; Jinja2 appears in `uv.lock` only as a dev/docs transitive dependency). Versioning, storage, and variable substitution for reusable prompts live on the platform; the SDK reduces its contract to transport plus a hook for computing the variable map at runtime (`src/agents/prompts.py:47-82`).
- **Dual escape hatch symmetry.** Both `instructions` and `prompt` accept either a static value or a function of `(context)`/`(context, agent)`, resolved concurrently each turn (`src/agents/agent.py:1042-1083`; `src/agents/run_internal/run_loop.py:2097-2100`), giving per-run dynamism without a caching layer.
- **Backend capability gating instead of emulation.** Rather than emulating prompt templates on Chat Completions, the SDK warns or fails fast depending on `strict_feature_validation` (`src/agents/models/openai_chatcompletions.py:85-102`), keeping one source of truth.
- **Payload-shape cooperation with server-managed prompts.** When `prompt` is present and the model was not explicitly named, the request omits `model`; empty tool payloads and dict-shaped tool choices are omitted too, letting the platform template own those decisions (`src/agents/models/openai_responses.py:880-926`; test `tests/test_agent_prompt.py:120-149`).

## Notable Patterns

- **TypedDict-as-schema with coercion helper**: `_coerce_prompt_dict` casts a runtime-validated mapping into the `Prompt` view rather than deep-validating it (`src/agents/prompts.py:51-53`), consistent with the SDK's validate-shape-once posture (`Agent.__post_init__` duck-types prompt via `hasattr(..., "get")`, `src/agents/agent.py:467-475`).
- **File-backed prompt assets with cached loading**: sandbox memory templates are read from package-relative `.md` files through `functools.cache` (`src/agents/sandbox/memory/prompts.py:8-19`); default sandbox instructions load from `agents.sandbox/instructions/prompt.md` via `importlib.resources` with an LRU cache and graceful `None` on failure (`src/agents/sandbox/runtime_agent_preparation.py:28-39`).
- **Placeholder conventions split by subsystem**: single-brace tokens for `str.replace`/`str.format` (`{memory_dir}`, `{session_id}`) versus double-brace tokens for visually distinct slots (`{{ extra_prompt_section }}`, `{{ phase_two_input_selection }}`) within the same file (`src/agents/sandbox/memory/prompts.py:21-22`).
- **Structured rendering helpers over raw interpolation**: selection summaries are rendered into bullet lists by dedicated functions before insertion (`_render_phase_two_input_selection`, `src/agents/sandbox/memory/prompts.py:117-152`), keeping template strings declarative.

## Tradeoffs

- **Zero-dependency templating vs. weak feedback loops**: delegating substitution to the platform keeps the SDK small and avoids escaping bugs, but developers get variable errors only after a network round trip, with no offline linting of `variables` against the template (`src/agents/prompts.py:58-82`).
- **Ad-hoc internal templating vs. a shared mini-engine**: each renderer is trivially auditable, yet the three coexisting mechanisms (`format`, `replace`, f-string) have different failure semantics — `KeyError` from `format` on stray braces in a template, silent no-op for unmatched `.replace` tokens, silent literal fallback in sinks (`src/agents/sandbox/session/sinks.py:189-196`).
- **Warning-by-default backend gating vs. silent misconfiguration**: ignoring `prompt` on Chat Completions keeps apps running but can produce agents that quietly lose their intended system configuration unless strict validation is enabled (`src/agents/models/openai_chatcompletions.py:94-102`).

## Failure Modes / Edge Cases

- Missing `id` in a static prompt dict → unhandled `KeyError` at resolution time, not a `UserError` (`src/agents/prompts.py:79`).
- Dynamic prompt function raising → deterministic cancellation of the sibling instructions coroutine; surfaced to the caller unchanged (`RuntimeError` propagates; `tests/test_agent_prompt.py:153-221`).
- Variables referencing nonexistent platform template variables → deferred provider error at request time; no client-side detection (absence confirmed: no validation logic beyond `prompts.py:58-82` exists; searched `variables` across `src/` — only transport sites matched).
- Stray brace in an internal `.md` template → `KeyError`/`IndexError` from `str.format` at first render (`src/agents/sandbox/memory/prompts.py:105,114`); conversely, braces inside inserted values are safe because they are format arguments.
- Order-dependent replacement hazard: a `memory_dir` value containing `{memory_summary}` would be rewritten by the later `.replace` pass (`src/agents/sandbox/memory/prompts.py:64-68`).
- Oversized untrusted content is guarded by token truncation plus an explicit in-prompt omission marker stating the view is incomplete (`src/agents/sandbox/memory/phase_one.py:19,68-79`; asserted in `tests/sandbox/test_memory.py:431-445`).
- Untemplatable sink path (bad placeholder in `workspace_relpath`) silently writes to the literal path instead of surfacing misconfiguration (`src/agents/sandbox/session/sinks.py:189-196`).

## Future Considerations

- Introduce a typed error for malformed `Prompt` dicts (e.g., raise `UserError` on missing `id` in `PromptUtil.to_model_input`) to make missing-variable behavior uniformly predictable (`src/agents/prompts.py:78-81`).
- Optionally fetch/cache declared template variables from the platform to enable pre-flight validation of the `variables` map before dispatch (`src/agents/prompts.py:32-33` currently carries no metadata for this).
- Consolidate internal renderers onto one placeholder convention and one substitution routine with defined unknown-token behavior, replacing the mixed `.replace`/`.format` usage in `src/agents/sandbox/memory/prompts.py:53-114` and the silent fallback in `src/agents/sandbox/session/sinks.py:184-197`.
- Document the sequential-replacement caveat of `render_memory_read_prompt` or switch to single-pass substitution to eliminate the ordering hazard (`src/agents/sandbox/memory/prompts.py:64-68`).

## Questions / Gaps

- **Exact wire type of variable values**: `Variables` is imported from the installed `openai` package (`src/agents/prompts.py:8-11`); its precise union shape lives outside this source directory and was not inspected per source-isolation rules. Behavior described here covers only how the SDK transports it.
- **Server-side missing-variable error surface**: whether the platform returns a typed error for unknown/missing variables could not be verified from this repository; no client-side handler for such errors exists (searched `src/agents/` for prompt-variable-specific exception handling — none found beyond the sites cited).
- **No evidence found** of any escaping utilities for prompt text (search terms: `escape`, `sanitize`, `Template(`, `jinja`, `substitute` across `src/agents/`); the only related mechanisms are the truncation markers cited above.

---

Generated by dimension `12.02-prompt-templating-and-variable-contracts` against `openai-agents-sdk`.
