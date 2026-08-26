# Source Analysis: pydantic-ai

## Dimension 12.01: Prompt Storage and Versioning

### Source Info

| Field | Value |
|-------|-------|
| Name | pydantic-ai |
| Path | `studies/agent-harness-study/sources/pydantic-ai` |
| Language / Stack | Python (uv workspace; `pydantic_ai_slim` core package, Pydantic v2, OpenTelemetry) |
| Analyzed | 2026-08-26 |

## Summary

Pydantic AI stores prompts in code or in declarative spec files, never in a database, prompt registry, or external platform. There are four storage forms: literal strings passed to `Agent(instructions=...)` / `Agent(system_prompt=...)` (`pydantic_ai_slim/pydantic_ai/agent/__init__.py:492-493`), Python functions registered via the `@agent.instructions` / `@agent.system_prompt` decorators (`pydantic_ai_slim/pydantic_ai/agent/__init__.py:2141`, `pydantic_ai_slim/pydantic_ai/agent/__init__.py:2282-2288`), Handlebars `TemplateStr` templates validated against the agent's deps type (`pydantic_ai_slim/pydantic_ai/template.py:16-108`), and `AgentSpec` YAML/JSON files loaded via `Agent.from_file` (`pydantic_ai_slim/pydantic_ai/agent/spec.py:33-69`) whose stated purpose includes "separating agent configuration from application code" and letting "non-developers (prompt engineers, domain experts) configure agents" (`docs/agent-spec.md:1-9`).

Prompts are **not versioned**. The only stable identifier attached to a generated prompt is `SystemPromptPart.dynamic_ref`, set to the generating function's `__qualname__` (`pydantic_ai_slim/pydantic_ai/_system_prompt.py:55`, field at `pydantic_ai_slim/pydantic_ai/messages.py:194`). This references *which function* produced the text, not *which revision* of its logic or content. Run-to-prompt association is therefore content-based, not identity-based: rendered system prompts persist inside serialized message history as timestamped `SystemPromptPart`s, joined instructions are stamped onto every `ModelRequest.instructions` (`pydantic_ai_slim/pydantic_ai/messages.py:1845`; stamping at `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1491-1499`), and OTel spans record the exact instructions plus input messages per model request under `gen_ai.system_instructions` / `gen_ai.input.messages` (`pydantic_ai_slim/pydantic_ai/models/instrumented.py:253-298`). With telemetry content capture enabled you can recover the exact prompt text a run saw; you cannot name or pin a prompt version.

Deployment decoupling from code is real but bounded: spec files are read at agent-construction time (`pydantic_ai_slim/pydantic_ai/agent/spec.py:66-68`), so a prompt change is a config-file change rather than a Python change, and dynamic instruction/system-prompt functions can fetch text from arbitrary runtime sources — but there is no hot reload, no watched store, and no built-in remote prompt source.

**Rating: 5/10**

Rationale against the rubric: the storage model that exists is clear, explicitly typed, well documented, and heavily tested (static/dynamic/template/spec all have dedicated tests, e.g. `tests/test_template.py:216-240,256-272` and `tests/test_agent.py:7898-8019`), and content-level traceability through instrumentation and message history is strong. However, the dimension's central question — "Can you tell exactly which prompt version produced a given output?" — is answered only indirectly: there are no version identifiers, no content hashes, no registry, no immutability guarantees, and the one reference mechanism (`dynamic_ref`) silently changes meaning whenever the referenced function's body changes. That places it in "present but inconsistent / fragile": durable mechanisms for storage and capture, absent mechanisms for identity.

## Evidence Collected

Every entry cites file paths with line numbers relative to the source root `studies/agent-harness-study/sources/pydantic-ai/`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Prompt fields live on `Agent` | `_instructions`, `_system_prompts`, `_system_prompt_functions`, `_system_prompt_dynamic_functions` dataclass fields | `pydantic_ai_slim/pydantic_ai/agent/__init__.py:454-457` |
| Constructor accepts `instructions=` and `system_prompt=` | Constructor params and normalization into internal stores | `pydantic_ai_slim/pydantic_ai/agent/__init__.py:492-493,651-655` |
| Decorator registration of prompt functions | `@agent.instructions` overloads; `@agent.system_prompt(dynamic=...)` registers runners keyed by `func.__qualname__` | `pydantic_ai_slim/pydantic_ai/agent/__init__.py:2141-2159,2262-2288,2328` |
| Instruction union type incl. `TemplateStr` | `AgentInstructions = TemplateStr \| str \| SystemPromptFunc \| Sequence[...] \| None` | `pydantic_ai_slim/pydantic_ai/_instructions.py:12-18` |
| Template compilation & validation | `TemplateStr` compiles Handlebars against `deps_type` at construction; auto-detected when a string contains `{{` during Pydantic validation | `pydantic_ai_slim/pydantic_ai/template.py:50-70,89-116` |
| Declarative spec storage | `AgentSpec` model with `instructions: TemplateStr[Any] \| str \| list[...]` and free-form `metadata`; loaded from YAML/JSON via `from_file`/`from_text`/`from_dict` | `pydantic_ai_slim/pydantic_ai/agent/spec.py:33-49,51-111` |
| Spec purpose statement | "Separating agent configuration from application code", non-developer prompt editing, config-file storage | `docs/agent-spec.md:1-9` |
| Dynamic system-prompt resolution | `resolve_system_prompts` emits `SystemPromptPart(prompt or '', dynamic_ref=runner.function.__qualname__)` for dynamic runners | `pydantic_ai_slim/pydantic_ai/_system_prompt.py:40-58` |
| Per-turn dynamic re-evaluation | `_reevaluate_dynamic_prompts` looks up runners by `part.dynamic_ref` and rebuilds the part each turn | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:707-732` |
| Instructions stamped onto history | `self.request.instructions = _messages.InstructionPart.join(instruction_parts)` before sending the request | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1491-1499` |
| `SystemPromptPart` shape | `content`, `timestamp`, `dynamic_ref`, `part_kind='system-prompt'` | `pydantic_ai_slim/pydantic_ai/messages.py:180-201` |
| `InstructionPart` static/dynamic flag | `dynamic: bool` distinguishes literal strings from functions/templates/toolsets; static parts sorted first | `pydantic_ai_slim/pydantic_ai/messages.py:1751-1785` |
| `ModelRequest.instructions` persistence slot | `instructions: str \| None` recorded per request; `run_id` and `conversation_id` also stamped | `pydantic_ai_slim/pydantic_ai/messages.py:1845,1851-1859` |
| Telemetry capture of exact prompts | `gen_ai.system_instructions` attribute set from joined instructions; `gen_ai.input.messages` records full request; gated by `include_content` | `pydantic_ai_slim/pydantic_ai/models/instrumented.py:110-122,253-298` |
| History-fallback instruction recovery | `_instrumentation.get_instructions` reads `ModelRequest.instructions` from the last two requests when parameters are unavailable | `pydantic_ai_slim/pydantic_ai/_instrumentation.py:580-620` |
| Instructions not reused across agents/runs | On `message_history` resume, only the current agent's instructions are included; recommendation to prefer `instructions` over `system_prompt` | `docs/agent.md:1345-1356` |
| Static/dynamic ordering rationale (prompt caching) | Static-before-dynamic ordering lets Anthropic/Bedrock cache the stable prefix outside dynamic instructions | `docs/agent.md:1358-1364` |
| UI adapter prompt ownership | `manage_system_prompt='server'` strips client `SystemMessage`s and reinjects via `ReinjectSystemPrompt`; instructions described as "injected fresh on every request, never persisted" | `docs/ui/ag-ui.md:479-488` |
| Server-side prompt authority capability | `ReinjectSystemPrompt(replace_existing=True)` strips untrusted history prompts and prepends the agent's configured prompt | `pydantic_ai_slim/pydantic_ai/capabilities/reinject_system_prompt.py:18-44,46-60` |
| Tests: dynamic re-evaluation semantics | `test_dynamic_true_reevaluate_system_prompt`, `test_dynamic_false_no_reevaluate`, none-return and no-change branches assert `dynamic_ref` round-trips | `tests/test_agent.py:7898-7970,7803-8019,3526-3558` |
| Tests: spec/template prompt storage | `test_from_spec_template_instructions_stored`, `test_agent_run_with_template_instructions`, `test_spec_instructions_template/list_with_templates` | `tests/test_template.py:216-240,256-272` |
| Provider-side instruction caching knobs | `anthropic_cache_instructions`, `bedrock_cache_instructions`, `openrouter_cache_instructions` place cache points after static instructions | `pydantic_ai_slim/pydantic_ai/models/anthropic.py:424-428,2216-2231`; `pydantic_ai_slim/pydantic_ai/models/bedrock.py:528-531`; `pydantic_ai_slim/pydantic_ai/models/openrouter.py:295-301,831-851` |

## Answers to Dimension Questions

**1. Where are prompts stored?**
In four places, all local to the deploying application: (a) literal strings in Python constructors (`pydantic_ai_slim/pydantic_ai/agent/__init__.py:492-493`), (b) Python functions registered as decorators (`pydantic_ai_slim/pydantic_ai/agent/__init__.py:2141,2282`), (c) Handlebars `TemplateStr` sources compiled at validation time (`pydantic_ai_slim/pydantic_ai/template.py:16-70`), and (d) YAML/JSON `AgentSpec` files loaded via `from_file` (`pydantic_ai_slim/pydantic_ai/agent/spec.py:51-69`). There is no database table, no hosted prompt registry, and no third-party prompt-platform integration. A repo-wide search for `langfuse|langsmith|promptfoo|humanloop|promptlayer|prompt_registry|prompt hub` found no integration points; Langfuse appears only as an external telemetry consumer (`docs/logfire.md:241`) and in token-attribution comments (`pydantic_ai_slim/pydantic_ai/usage.py:24`).

**2. Are prompt versions tracked?**
No evidence of prompt versioning was found. There is no version field on `SystemPromptPart` (`pydantic_ai_slim/pydantic_ai/messages.py:180-201`), `InstructionPart` (`pydantic_ai_slim/pydantic_ai/messages.py:1751-1785`), or `AgentSpec` beyond a free-form, unparsed `metadata` dict (`pydantic_ai_slim/pydantic_ai/agent/spec.py:48`). Searched for `prompt_version`, `prompt_id`, `PromptRegistry`, and hash-based identifiers across `*.py`; nothing matched. The closest construct is `SystemPromptPart.dynamic_ref = runner.function.__qualname__` (`pydantic_ai_slim/pydantic_ai/_system_prompt.py:55`), which names the producing function — useful for lookup during re-evaluation (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:717-726`) but not a content version: editing the function body keeps the same ref while changing the emitted text. Note: the `version` parameter on `InstrumentationSettings` is a telemetry wire-format version, explicitly "unrelated to the Pydantic AI package version" (`pydantic_ai_slim/pydantic_ai/models/instrumented.py:124-126`) and unrelated to prompts.

**3. Can a run be traced to the exact prompt version used?**
To the exact prompt *text*, yes — conditionally; to a prompt *version*, no. Two content-level trails exist:
- Message history: each `ModelRequest` persists its rendered `SystemPromptPart`s (with timestamps and `dynamic_ref`) and its joined `instructions` string (`pydantic_ai_slim/pydantic_ai/messages.py:1845,180-201`), stamped during the run at `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1491-1499`, and is serializable via `result.all_messages_json()` (`pydantic_ai_slim/pydantic_ai/result.py:544,877`).
- OpenTelemetry: model-request spans carry `gen_ai.system_instructions` and full `gen_ai.input.messages` (`pydantic_ai_slim/pydantic_ai/models/instrumented.py:266-289`), with a history-fallback extractor for span building (`pydantic_ai_slim/pydantic_ai/_instrumentation.py:580-620`). Capture is gated behind the privacy switch `include_content` (`pydantic_ai_slim/pydantic_ai/models/instrumented.py:110-112`), so tracing is opt-in.
Neither trail carries a version identity: two runs using different revisions of the same `dynamic_ref` function are indistinguishable except by comparing raw text. Caveats further weaken the trail: instructions are deliberately *not* taken from supplied history on resume — only the current agent's instructions apply (`docs/agent.md:1347-1354`), and UI adapters strip client-submitted system prompts by default (`docs/ui/ag-ui.md:487`), so persisted history does not always reflect what a prior run actually sent.

**4. Can prompts be updated without redeploying code?**
Partially. Updating a YAML/JSON `AgentSpec` changes prompts without touching Python source (`pydantic_ai_slim/pydantic_ai/agent/spec.py:51-69`; motivation stated at `docs/agent-spec.md:3-9`), and template strings let one spec serve many contexts by rendering `{{...}}` placeholders against deps at run time (`pydantic_ai_slim/pydantic_ai/template.py:72-87`; example at `docs/dependencies.md:103`). Dynamic instruction/system-prompt functions execute per run (`docs/agent.md:1305,1361`), so an application can point them at any external store it builds itself. But the framework loads specs eagerly at construction (`pydantic_ai_slim/pydantic_ai/agent/spec.py:66-68`), provides no hot-reload/watch mechanism, ships no remote prompt-source adapter, and offers no atomicity/rollback story for a swapped spec. In-process prompt changes still require a process restart or re-construction of the `Agent`.

## Architectural Decisions

- **Prompts as typed constructor/decorator inputs, not artifacts.** Prompts are ordinary values on the `Agent` dataclass (`pydantic_ai_slim/pydantic_ai/agent/__init__.py:454-457`) with a union type that folds strings, functions, and templates together (`pydantic_ai_slim/pydantic_ai/_instructions.py:12-18`). Versioning is delegated upward to the user's own VCS/deployment.
- **Static/dynamic classification as a first-class axis.** Every instruction part records whether it came from a literal or a changeable source (`pydantic_ai_slim/pydantic_ai/messages.py:1766-1772`), normalized uniformly so plain strings from toolsets are treated as dynamic (`pydantic_ai_slim/pydantic_ai/_instructions.py:58-76`). This classification doubles as the prompt-cache boundary decision: static parts sort first (`pydantic_ai_slim/pydantic_ai/messages.py:1782-1785`) so providers can cache the stable prefix (`docs/agent.md:1364`; concrete implementations at `pydantic_ai_slim/pydantic_ai/models/anthropic.py:2216-2231` and `pydantic_ai_slim/pydantic_ai/models/openrouter.py:831-851`, which even skips caching entirely when dynamic instructions are present).
- **Function identity as the linkage key.** Dynamic prompts carry `dynamic_ref = __qualname__` (`pydantic_ai_slim/pydantic_ai/_system_prompt.py:55`) and re-evaluation resolves runners through that dict key (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:717-726`). Identity-of-generator instead of identity-of-content.
- **Two persistence postures by design.** `system_prompt` is retained in history across turns/agents; `instructions` are recomputed fresh per request and never inherited from provided history (`docs/agent.md:1347-1356`; restated for adapters at `docs/ui/ag-ui.md:481`). This is a deliberate traceability tradeoff: fresh instructions avoid stale-prompt bugs at the cost of history no longer being a complete record of what past requests saw unless telemetry captured it.
- **Declarative specs as the escape hatch from code-bound prompts**, including a JSON schema published alongside specs for editor validation (`pydantic_ai_slim/pydantic_ai/agent/spec.py:113-163,173-228`).

## Notable Patterns

- **Content-addressable-by-telemetry fallback:** when `model_request_parameters` are unavailable, the instrumentation layer reconstructs instructions by scanning the last two `ModelRequest`s in reverse (`pydantic_ai_slim/pydantic_ai/_instrumentation.py:600-620`), showing that history itself is treated as the prompt ledger of record.
- **Server-authoritative prompt reinjection:** `ReinjectSystemPrompt` restores the configured prompt when history arrives from sources that don't round-trip system prompts, optionally stripping existing parts when history is untrusted (`pydantic_ai_slim/pydantic_ai/capabilities/reinject_system_prompt.py:18-44,52-53`). This treats prompt provenance as a security property, echoing the injection-authority guidance in the AG-UI docs (`docs/ui/ag-ui.md:255-259`).
- **Mid-conversation system prompts without cache invalidation:** enqueued `SystemPromptPart`s stay inline in history rather than being hoisted, preserving cached prefixes (`pydantic_ai_slim/pydantic_ai/.agents/skills/building-pydantic-ai-agents/references/INPUT-AND-HISTORY.md:106`), and `ToolAvailabilityDeltaPart` documents that tool withdrawal must invalidate caches — cache-safety reasoning pervades prompt handling (`pydantic_ai_slim/pydantic_ai/messages.py:1790-1799`).
- **Tests encode the versioning posture precisely:** snapshots pin `dynamic_ref=IsStr()` alongside updated content (`tests/test_agent.py:3553-3558`), and dedicated tests cover the no-change, none-return, and dynamic-off branches of re-evaluation (`tests/test_agent.py:7803-8019`).

## Tradeoffs

- **Zero-versioning simplicity vs. reproducibility.** Keeping prompts as code/config values avoids registry infrastructure, but means "which prompt version produced this output?" is answerable only by diffing raw captured text (`pydantic_ai_slim/pydantic_ai/models/instrumented.py:266-289`) — and only if `include_content` was enabled.
- **Fresh instructions vs. historical fidelity.** Recomputing instructions per run (`docs/agent.md:1347-1349`) guarantees consistency within a run but makes cross-run comparison depend on external telemetry rather than the history payload.
- **Qualname refs vs. content hashes.** `dynamic_ref` survives serialization cheaply and enables per-turn refresh (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:712-726`), yet is ambiguous across code revisions and collides conceptually if the same function moves modules.
- **Spec files vs. programmatic power.** YAML specs make prompts editable by non-developers (`docs/agent-spec.md:5-9`) but restrict callables; templates bridge the gap by validating `{{field}}` usage against the deps type at load time (`pydantic_ai_slim/pydantic_ai/template.py:63-70`) — catching drift early, though only for deps-shaped variables.
- **Privacy gating vs. auditability.** `include_content=False` keeps prompts out of spans (`pydantic_ai_slim/pydantic_ai/models/instrumented.py:110-112`), which is right for PII but leaves teams with no prompt audit trail at all.

## Failure Modes / Edge Cases

- **Silent semantic drift under a stable ref:** changing a dynamic system-prompt function's body keeps the same `dynamic_ref` (`pydantic_ai_slim/pydantic_ai/_system_prompt.py:55`); old histories replayed later will be re-evaluated with the *new* logic while claiming continuity with the old (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:707-732`), and nothing flags the mismatch. Covered intentionally by tests asserting re-evaluation replaces content in place (`tests/test_agent.py:7898-7970`).
- **Untrusted-history prompt substitution:** a client-supplied history containing attacker-chosen `SystemPromptPart`s would override operator intent; mitigated by default stripping/reinjection in server mode (`docs/ui/ag-ui.md:487`, `pydantic_ai_slim/pydantic_ai/capabilities/reinject_system_prompt.py:29-32,52-53`) — but only for adapters/users who adopt that mode or capability.
- **Telemetry-dependent forensics:** with `include_content=False`, neither spans nor anything else records prompt text, so post-hoc reconstruction of what a failed run saw is impossible from the framework alone (`pydantic_ai_slim/pydantic_ai/models/instrumented.py:110-112`).
- **Instruction loss on resume across agents:** switching agents mid-conversation drops prior agents' instructions by design (`docs/agent.md:1347-1354`); users expecting history to carry them will see behavior changes that no version identifier would have explained either.
- **Empty-string prompts vanish quietly:** returning `''` from an instruction function yields no instruction message (`docs/agent.md:1404`; empty results skipped at `pydantic_ai_slim/pydantic_ai/_system_prompt.py:56-57` and whitespace-only toolset instructions dropped at `pydantic_ai_slim/pydantic_ai/_instructions.py:74-75`), which can silently change effective prompt composition.

## Future Considerations

- Attach an optional content hash or explicit version label to `SystemPromptPart`/`InstructionPart` next to `dynamic_ref`/`dynamic` (`pydantic_ai_slim/pydantic_ai/messages.py:194,1766`) so histories become self-describing without new infrastructure.
- Let `AgentSpec.metadata` conventions (or a dedicated field, `pydantic_ai_slim/pydantic_ai/agent/spec.py:48`) flow into run/span attributes so spec revisions correlate with runs automatically.
- Offer a pluggable prompt-source interface (file watch, remote registry) behind the existing `SystemPromptRunner` seam (`pydantic_ai_slim/pydantic_ai/_system_prompt.py:14-37`), keeping the library slim per its stated philosophy (root `AGENTS.md`, "strong primitives ... over narrow solutions").
- Emit a warning when a resolved `dynamic_ref` in incoming history has no matching runner (currently guarded as `pragma: lax no cover` at `pydantic_ai_slim/pydantic_ai/_agent_graph.py:719-721`), since that indicates a stale or foreign history.

## Questions / Gaps

- No evidence found of any prompt-version identifier, content hash, or immutable prompt artifact anywhere in the package. Search boundary: `rg`-style greps across `*.py` and `*.md` under the source root for `prompt_version|prompt_id|PromptRegistry|prompt_registry|langfuse|langsmith|promptfoo|humanloop|promptlayer|prompt hub`, plus manual inspection of `messages.py`, `_system_prompt.py`, `_instructions.py`, `agent/spec.py`, `models/instrumented.py`.
- Whether the maintainers consider prompt versioning in scope for the library (vs. an application concern) could not be determined from the repository alone; the design consistently delegates it to user code and VCS.
- The durable-execution integrations (`pydantic_ai_slim/pydantic_ai/durable_exec/`) were not analyzed for whether replayed workflows snapshot prompt text independently of message history; that would be a natural follow-up study under replay/determinism dimensions.

---

Generated by `12.01-prompt-storage-and-versioning` against `pydantic-ai`.
