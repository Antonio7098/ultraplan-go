# Source Analysis: openai-agents-sdk

## Dimension 12.01: Prompt Storage and Versioning

### Source Info

| Field | Value |
|-------|-------|
| Name | openai-agents-sdk |
| Path | `studies/agent-harness-study/sources/openai-agents-sdk` |
| Language / Stack | Python (Agents SDK library; OpenAI Responses API integration) |
| Analyzed | 2026-08-25 |

## Summary

The SDK uses a **three-tier hybrid prompt storage model**:

1. **Server-side hosted prompt registry (OpenAI platform)** — the only tier with real versioning. An agent references a platform-managed prompt by `id` plus an *optional* `version` and `variables` via the `Prompt` TypedDict (`src/agents/prompts.py:23-33`). The SDK resolves this to a `ResponsePromptParam` (`src/agents/prompts.py:78-82`) and forwards it verbatim in the Responses API request body (`src/agents/models/openai_responses.py:980`). Template content, template variables, iteration history, and versions live entirely on the OpenAI side; the repo only stores a reference string such as `"pmpt_123"` (`docs/agents.md:88`, `examples/basic/prompt_template.py:21`).
2. **In-code prompts** — `Agent.instructions` accepts a plain string or a `(context, agent) -> str` callable (`src/agents/agent.py:309-323`, resolution at `src/agents/agent.py:1042-1071`); small canned instruction constants are embedded directly in Python modules (`src/agents/extensions/handoff_prompt.py:3-12`, `src/agents/sandbox/capabilities/shell.py:16-23`, `src/agents/voice/model.py:14-15`). None of these carry version identifiers.
3. **Packaged markdown templates** — internal sandbox/memory prompts ship as `.md` files inside the package and are read at import time: the default sandbox system prompt (`src/agents/sandbox/instructions/prompt.md`, 192 lines, loaded with `lru_cache(maxsize=1)` via `importlib.resources` at `src/agents/sandbox/runtime_agent_preparation.py:28-39`) and four memory-pipeline templates loaded once through `functools.cache` into module-level constants (`src/agents/sandbox/memory/prompts.py:8-19`).

Run-to-prompt-version traceability is the weakest area: the resolved `{id, version, variables}` is sent per model turn (`src/agents/run_internal/run_loop.py:2097-2100`, `2220-2234`) but never recorded on traces — `ResponseSpanData.export()` emits only `response_id` and `usage` (`src/agents/tracing/span_data.py:236-241`) — so "which prompt version produced this output" is answerable only if the application logs it itself.

## Rating

**6 / 10** — Present but inconsistent. The hosted-prompt path is a clear, tested, explicitly-typed interface that enables independent prompt deployment without code changes (`src/agents/prompts.py:56-82`, tests at `tests/test_agent_prompt.py:57-149`), which is genuinely good design. But it is dragged down by: no version tracking whatsoever for in-code or packaged markdown templates; optional (and thus omittable) version pinning on hosted prompts, where omission silently tracks server-side latest; and zero run-time recording of the exact prompt id/version used, making post-hoc provenance impossible from SDK artifacts alone.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Hosted prompt reference type | `Prompt` TypedDict: required `id: str`, optional `version`, optional `variables` | `src/agents/prompts.py:23-33` |
| Dynamic prompt selection | `DynamicPromptFunction = Callable[[GenerateDynamicPromptData], MaybeAwaitable[Prompt]]`; data carries run context + agent | `src/agents/prompts.py:36-47` |
| Resolution to wire format | `PromptUtil.to_model_input` returns `{"id", "version", "variables"}`; raises `UserError` if dynamic fn returns non-dict | `src/agents/prompts.py:58-82` |
| Agent field + intent | `Agent.prompt` docstring: "configure the instructions, tools and other config for an agent outside of your code"; Responses-API-only | `src/agents/agent.py:325-329` |
| Per-turn resolution | Runner resolves system prompt and prompt config together at each agent start | `src/agents/run_internal/run_loop.py:2097-2100` |
| Wire forwarding | `create_kwargs["prompt"] = self._non_null_or_omit(prompt)` in Responses request builder | `src/agents/models/openai_responses.py:980` |
| Prompt-managed tools/model | When a prompt is set, `model` is omitted unless explicit (`:880-881`), opaque tool search allowed (`:900-906`), empty tools payload omitted (`:917-926`) | `src/agents/models/openai_responses.py:880-926` |
| Non-OpenAI models ignore it | Chat Completions adapter hardcodes `prompt=None` on both paths | `src/agents/models/openai_chatcompletions.py:259`, `src/agents/models/openai_chatcompletions.py:471` |
| Realtime surface | `RealtimeAgent.prompt: Prompt \| None`; forwarded as `ResponsePrompt(id, variables, version)` on session create | `src/agents/realtime/agent.py:61-62`, `src/agents/realtime/openai_realtime.py:1809-1816` |
| Packaged md loading (sandbox) | Default 192-line system prompt read via `importlib.resources`, memoized with `lru_cache(maxsize=1)`, silent `None` on failure | `src/agents/sandbox/runtime_agent_preparation.py:28-39` |
| Packaged md loading (memory) | Four `.md` templates read once via `functools.cache` at import; render functions do string `.replace()` for placeholders like `{memory_dir}`, `{{ extra_prompt_section }}` | `src/agents/sandbox/memory/prompts.py:8-19`, `53-97` |
| Instruction composition order | Fixed order documented: default sandbox prompt → `base_instructions` → `instructions` → capability fragments → remote-mount policy → filesystem tree | `src/agents/sandbox/runtime_agent_preparation.py:190-233`, `docs/sandbox/guide.md:119` |
| Embedded prompt constant | `RECOMMENDED_PROMPT_PREFIX` + `prompt_with_handoff_instructions()` wrapper | `src/agents/extensions/handoff_prompt.py:3-19` |
| Tracing does not record prompt | `ResponseSpanData` holds `response/input/usage`; `export()` emits only `type`, `response_id`, `usage` — no prompt id/version fields anywhere under `src/agents/tracing/` | `src/agents/tracing/span_data.py:212-241` |
| Run-state identity includes prompt | Serialized agent identity signature hashes normalized `instructions` and `prompt` values (for duplicate-name disambiguation on resume), not provenance | `src/agents/run_state.py:4913-4918`, `_build_agent_identity_map` `src/agents/run_state.py:4980-5027` |
| Tests: resolution & passthrough | Static/dynamic prompt resolution asserted exactly; captured prompt reaches the Model unchanged | `tests/test_agent_prompt.py:57-109` |
| Tests: prompt-managed request shape | With `prompt={"id": "pmpt_agent"}`, request asserts `model is omit`, `tools is omit` | `tests/test_agent_prompt.py:122-149` |
| Tests: failure isolation | Failing dynamic prompt function cancels sibling instructions resolution (run + streamed) | `tests/test_agent_prompt.py:153-221` |
| Tests: memory prompt rendering | Placeholder substitution verified (`{{ extra_prompt_section }}` absent by default, truncation markers) | `tests/sandbox/test_memory.py:431-445`, `544-553` |
| Example: hosted registry workflow | Instructions to create prompt at `platform.openai.com/playground/prompts`, pin `"version": "1"` in static and dynamic forms | `examples/basic/prompt_template.py:7-39`, `54-64`; `docs/agents.md:66-93` |
| Public exports | `Prompt`, `DynamicPromptFunction`, `GenerateDynamicPromptData` exported at top level | `src/agents/__init__.py:98`, `411-413` |
| Adjacent (not versioning): cache keys | SDK generates hashed `prompt_cache_key` values persisted across resume — caching affinity, not prompt provenance | `src/agents/run_internal/prompt_cache_key.py:13`, `119-130`, `src/agents/run_state.py:808-809` |

## Answers to Dimension Questions

**1. Where are prompts stored?**
Three places. (a) The OpenAI-hosted prompt registry referenced by id (`docs/agents.md:68-93`; `examples/basic/prompt_template.py:21`) — content lives outside the repository. (b) In code: string/callable `instructions` (`src/agents/agent.py:309-323`) and embedded constants (`src/agents/extensions/handoff_prompt.py:3-12`, `src/agents/sandbox/capabilities/shell.py:16-23`). (c) Package-shipped markdown files: `src/agents/sandbox/instructions/prompt.md` and `src/agents/sandbox/memory/prompts/*.md`, loaded via `importlib.resources` / `functools.cache` (`src/agents/sandbox/runtime_agent_preparation.py:28-39`, `src/agents/sandbox/memory/prompts.py:8-19`). There is no database, no user-facing local prompt registry, and no external-platform adapter beyond the OpenAI one.

**2. Are prompt versions tracked?**
Only for hosted prompts, optionally: `version: NotRequired[str]` (`src/agents/prompts.py:29-30`), forwarded verbatim (`src/agents/prompts.py:80`, `src/agents/models/openai_responses.py:980`, `src/agents/realtime/openai_realtime.py:1815`). Version semantics (pinning vs latest) are owned server-side by OpenAI; the SDK neither validates nor defaults it. In-code strings, embedded constants, and packaged `.md` templates have **no version identifiers, labels, or content hashes at all** — they change implicitly with git history and package releases. No evidence found of any mechanism stamping a template revision at runtime (searched `src/` for version/hash/label near all prompt modules).

**3. Can a run be traced to the exact prompt version used?**
Not from SDK-produced artifacts alone. The resolved prompt config exists transiently in the runner loop (`src/agents/run_internal/run_loop.py:2097-2100`, passed at `2233`/`2605-2616` region), but `ResponseSpanData.export()` emits only `response_id` and `usage` (`src/agents/tracing/span_data.py:236-241`) and nothing under `src/agents/tracing/` mentions prompts (grep returned zero matches). Hooks see filtered instructions but not the prompt config (`src/agents/run_internal/run_loop.py:2165`). Serialized `RunState` folds `instructions` and `prompt` into an agent identity signature used solely to disambiguate duplicate agent names on resume (`src/agents/run_state.py:4917-4918`, `tests/test_run_state.py:385-409`) — it proves "same agent config," not "this output came from version N." To answer the dimension's core question ("can you tell exactly which prompt version produced a given output?") an application must record `agent.get_prompt()` results itself; the SDK provides the accessor (`src/agents/agent.py:1073-1083`) but no automatic capture.

**4. Can prompts be updated without redeploying code?**
Yes for hosted prompts: editing the prompt on the OpenAI platform takes effect without touching the app, since only `pmpt_*` ids live in code; omitting `version` deliberately floats to whatever the platform serves next (optionality per `src/agents/prompts.py:29-30`; example pins `"version": "1"` when reproducibility matters, `examples/basic/prompt_template.py:35,59`). Additionally, `DynamicPromptFunction` lets applications fetch prompts from arbitrary stores at run time with context-aware selection (`src/agents/prompts.py:36-47`, `70-76`). No for packaged/in-code prompts: memory and sandbox templates are frozen into the wheel (import-time load, `functools.cache`/`lru_cache`), so changes require an SDK release and process restart.

## Architectural Decisions

- **Delegate template storage and versioning to the platform rather than build a registry in the SDK.** The SDK's entire versioning story is a pass-through of `{id, version, variables}` (`src/agents/prompts.py:23-33` → `src/agents/models/openai_responses.py:980`). This keeps the SDK provider-neutral-thin but couples the feature to the Responses API: the Chat Completions adapter silently drops the prompt (`prompt=None`, `src/agents/models/openai_chatcompletions.py:259,471`) and the field docstring states it is "Only usable with OpenAI models, using the Responses API" (`src/agents/agent.py:328-329`).
- **Treat prompts as first-class per-agent configuration alongside `instructions`.** Both resolve concurrently at each agent start (`src/agents/run_internal/run_loop.py:2097-2100`) and both feed the agent identity signature in serialized state (`src/agents/run_state.py:4917-4918`), so prompt config participates in resume-consistency checks.
- **Let prompts own the request when present**: setting a prompt causes the SDK to omit `model` (unless explicit) and the tools payload so the server-side prompt can manage them (`src/agents/models/openai_responses.py:880-881,917-926`; regression-tested at `tests/test_agent_prompt.py:122-149`). This makes the hosted prompt, not the code, the source of truth for those knobs.
- **Ship internal harness prompts as package data, not literals, but still freeze them at import time.** Markdown files keep large prompts editable/reviewable (`src/agents/sandbox/instructions/prompt.md`; `src/agents/sandbox/memory/prompts/`), yet import-time `functools.cache` binding (`src/agents/sandbox/memory/prompts.py:11-19`) means they behave exactly like compiled-in constants at runtime.
- **Compose runtime instructions deterministically** in a fixed order (default/base → agent instructions → capability fragments → mount policy → filesystem tree, `src/agents/sandbox/runtime_agent_preparation.py:190-233`), trading flexibility for predictable, reviewable prompt assembly.

## Notable Patterns

- **Reference-vs-content split**: application code holds stable ids; mutable content lives behind the API boundary — the classic indirection that enables independent deployment (`docs/agents.md:66-93`).
- **Dynamic resolution hook with typed inputs**: `GenerateDynamicPromptData(context, agent)` gives the callback full run context for store lookups or A/B selection (`src/agents/prompts.py:36-47`), and sync/async return shapes are both handled (`src/agents/prompts.py:70-74`).
- **Minimal-string-template rendering**: packaged memory prompts use naive `.replace()` on `{{ placeholder }}` and `{placeholder}` tokens rather than a templating engine (`src/agents/sandbox/memory/prompts.py:64-87`) — zero dependencies, with placeholder-leakage covered by tests (`tests/sandbox/test_memory.py:544-553`).
- **Graceful degradation with silent fallback**: missing sandbox `prompt.md` yields `None` base instructions instead of an error (`src/agents/sandbox/runtime_agent_preparation.py:38-39`).
- **Deliberate separation of concerns between prompt *versioning* (server-side) and prompt *cache keys* (client-side)**: `PromptCacheKeyResolver` derives hashed cache-affinity keys from conversation/session/group ids and persists them across resume (`src/agents/run_internal/prompt_cache_key.py:16-88,119-130`) — adjacent naming, unrelated to provenance.

## Tradeoffs

- **Platform-delegated versioning**: gains independent deployment, dashboard-based iteration, and variable management for free; costs portability (Responses-API-only, dropped on other adapters per `src/agents/models/openai_chatcompletions.py:259`), offline inspection, and local diffability of effective prompts.
- **Optional version pinning**: flexible (float to latest) but silently non-reproducible; nothing warns when `version` is absent (`src/agents/prompts.py:80` just forwards `None`).
- **Package-data prompts**: better authoring ergonomics than string literals, but import-time freezing prevents runtime override, hot reload, or per-environment substitution; there is no config hook to point at alternative template directories (checked `src/agents/_config.py` — no prompt-related settings).
- **No provenance capture**: keeps spans lean and avoids duplicating sensitive prompt text into telemetry (only `response_id`/`usage` export, `src/agents/tracing/span_data.py:236-241`); the cost is that audit/repro workflows get no SDK support.
- **Naive string templating**: avoids a dependency and keeps templates greppable, but offers no escaping, conditionals, or validation of leftover placeholders beyond targeted tests.

## Failure Modes / Edge Cases

- **Unvalidated static prompt dicts**: `_coerce_prompt_dict` blindly casts (`src/agents/prompts.py:51-53`); a dict missing `id` fails late with a raw `KeyError` at `resolved_prompt["id"]` during a run (`src/agents/prompts.py:79`) rather than at construction. Only the dynamic path gets an explicit `UserError` for wrong return type (`:75-76`).
- **Silent loss of base instructions**: if `instructions/prompt.md` cannot be read, `get_default_sandbox_instructions()` returns `None` and the sandbox agent runs with no base prompt, with no warning (`src/agents/sandbox/runtime_agent_preparation.py:38-39`).
- **Version drift**: runs using unpinned hosted prompts may change behavior after a platform-side edit, with no local signal distinguishing "prompt changed" from "model/code changed."
- **Dynamic prompt failures abort resolution loudly but mid-flight**: a raising prompt function cancels sibling instructions resolution and surfaces `RuntimeError` (tested for both run modes, `tests/test_agent_prompt.py:153-221`) — fail-fast, but at run time rather than definition time.
- **Cache-key confusion risk**: `prompt_cache_key` (caching, `src/agents/run_internal/prompt_cache_key.py:13`) vs `prompt.version` (identity) naming invites operators to conflate cache affinity with provenance; they are independent mechanisms.
- **Identity-signature sensitivity on resume**: because `prompt` values feed the agent identity hash (`src/agents/run_state.py:4918`), changing a prompt dict between persist and resume contributes to a different identity ordering key for duplicate-named agents (`src/agents/run_state.py:4967-4977`) — an intentional consistency guard, but it means prompt edits can affect how resumed runs map agents.

## Future Considerations

- Record resolved `{id, version}` on `ResponseSpanData` (even redacted-by-default, opt-in via existing `trace_include_sensitive_data` flag) to close the provenance gap (`src/agents/tracing/span_data.py:212-241`).
- Add construction-time validation for static prompt dicts (require `id`, type-check `version`) in `Agent.__post_init__` (`src/agents/agent.py:432+`) to convert runtime `KeyError`s into early errors, consistent with the repo's stated preference for actionable errors before invocation.
- Emit a warning (or expose an explicit "latest" sentinel) when a hosted prompt is used without a pinned `version`, given the reproducibility cost.
- Provide an override seam for packaged templates (config key or resource-path injection) so deployments can patch memory/sandbox prompts without an SDK release.
- Consider surfacing the effective composed instructions/prompt through hooks or the result object, since `on_llm_start` currently receives only filtered instructions, not the prompt config (`src/agents/run_internal/run_loop.py:2165`).

## Questions / Gaps

- **Exact server-side semantics of omitted `version`** (pin-to-latest vs draft channel) are defined by the OpenAI platform, not this repo; no evidence in-repo beyond the optional typing (`src/agents/prompts.py:29-30`). Searched `src/`, `docs/`, `integration_tests/` — no integration test exercises a hosted prompt against the live API (`grep pmpt_` matches only examples/docs).
- **No evidence of prompt A/B, rollback, or approval workflows** in-repo; if they exist, they are purely platform features invisible to the SDK.
- **Whether the full `response` attached in-memory when tracing includes sensitive data** (`src/agents/models/openai_responses.py:604-606`) could be used downstream to recover prompt metadata was not verifiable: the exported span intentionally drops everything but `response_id`/`usage`, and the Response object itself reflects server output, not the request's prompt reference.
- **Git history of the prompt `.md` files** (as a de facto changelog) was out of scope for this analysis; versioning via commit history exists implicitly but nothing in code references it.

---

Generated by `12.01-prompt-storage-and-versioning` against `openai-agents-sdk`.
