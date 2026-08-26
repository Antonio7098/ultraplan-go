# Source Analysis: agent-framework

## Prompt Storage and Versioning

### Source Info

| Field | Value |
|-------|-------|
| Name | agent-framework |
| Path | `studies/agent-harness-study/sources/agent-framework` |
| Language / Stack | Python (primary, `python/packages/*`), .NET (`dotnet/src/*`), Go (stub `go/README.md` only) |
| Analyzed | 2026-08-25 |

> Citation convention: paths are relative to the source root `studies/agent-harness-study/sources/agent-framework/`.

## Summary

Microsoft Agent Framework stores prompts in three distinct tiers with no unified registry. First, **code-defined prompt constants**: the core harness and orchestration layers embed default prompt strings as inline module-level Python constants (e.g., `DEFAULT_HARNESS_INSTRUCTIONS` at `python/packages/core/agent_framework/_harness/_agent.py:54`, the seven Magentic orchestrator prompts at `python/packages/orchestrations/agent_framework_orchestrations/_magentic.py:108-245`). Every one of these exposes a constructor-parameter override, but none carries a version identifier. Second, **declarative YAML definitions**: the `declarative` package loads `PromptAgent` instructions from YAML files or inline YAML via `AgentFactory.create_agent_from_yaml_path` / `create_agent_from_yaml` (`python/packages/declarative/agent_framework_declarative/_loader.py:291,416`), enabling prompt updates without code changes. Third, **service-side storage**: Foundry hosted PromptAgents keep prompts in the Foundry service; the client pins an explicit `agent_version` (env `FOUNDRY_AGENT_VERSION`) into the Responses API `agent_reference` payload (`python/packages/foundry/agent_framework_foundry/_agent.py:88-94,143-152`).

Version tracking is the weak spot. No framework-defined prompt constant, declarative schema field, or serialization record carries a prompt version ID; the only version identifiers found are *protocol* versions (`ProtocolVersionRecord` at `python/packages/declarative/agent_framework_declarative/_models.py:963`; hosting manifests pinning `protocol: responses, version: 2.0.0` at `python/samples/04-hosting/foundry-hosted-agents/responses/foundry_toolbox/agent.yaml:3-5`) and the service-owned Foundry `agent_version`. De facto, prompt history is whatever the surrounding git/package release process provides.

Run-to-prompt association is achieved only through OpenTelemetry GenAI semconv: the effective system instructions of each model call are captured onto spans as `gen_ai.system_instructions` (`python/packages/core/agent_framework/observability.py:285,2794-2806`), but this is gated behind experimental semconv opt-in plus sensitive-data capture (default off) and is ephemeral trace content — not a durable run→prompt-version ledger.

**Answer to the dimension's guiding question ("Can you tell exactly which prompt version produced a given output?"): not from the framework alone.** You can reconstruct it if you (a) enabled OTel instruction capture for that run, and (b) independently know the package commit/version that was running. The framework itself records neither.

## Rating

**5 / 10 — Present but inconsistent, weakly documented, and fragile as a system.**

Rationale against the rubric:
- Toward 7–8: every built-in prompt has an explicit, tested override interface (e.g., all seven Magentic prompts overridable per constructor and asserted in `python/packages/orchestrations/tests/test_magentic.py:1139-1193`); effective system instructions are observably captured on spans with dedicated tests (`python/packages/core/tests/core/test_observability.py:345-364,395-418`); two genuine out-of-code update paths exist (declarative YAML; Foundry service-side agents with pinned versions).
- Held back from 7+: storage is scattered across dozens of modules with no registry, no prompt identity, no content hash, and no version stamp anywhere in the Python runtime; run-to-prompt traceability silently degrades to nothing when the (default-off) sensitive-data telemetry flag is off; there is no documented lifecycle or audit story tying prompt text to runs. The .NET side shows slightly more maturity (public `MagenticDefaultPrompts` surface designed for override derivation, `dotnet/src/Microsoft.Agents.AI.Workflows/MagenticPromptOverrides.cs:22-63`), but equally lacks version identifiers.

## Evidence Collected

Every entry includes a file path with line numbers, relative to the source root.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Code-defined harness system prompt | `DEFAULT_HARNESS_INSTRUCTIONS` multi-line string constant; assembled with agent instructions by `_assemble_instructions()` | `python/packages/core/agent_framework/_harness/_agent.py:54-69,72-79` |
| Harness feature prompt constants | `DEFAULT_FILE_ACCESS_INSTRUCTIONS` (`:48`, applied `:1334`), `DEFAULT_BACKGROUND_AGENTS_INSTRUCTIONS` (`:31`, applied `:324`), `DEFAULT_MODE_INSTRUCTIONS` (`:15`, applied `:257`), `DEFAULT_TODO_INSTRUCTIONS` (`:25`, applied `:479`), `DEFAULT_FILE_MEMORY_INSTRUCTIONS` (`:60`, applied `:264`), `DEFAULT_JUDGE_INSTRUCTIONS` (`:78`, applied `:405`) | `python/packages/core/agent_framework/_harness/_file_access.py:48`, `_background_agents.py:31`, `_mode.py:15`, `_todo.py:25`, `_file_memory.py:60`, `_loop.py:78` |
| Memory-provider LLM prompts | `DEFAULT_MEMORY_CONTEXT_PROMPT`, `DEFAULT_MEMORY_EXTRACTION_PROMPT`, `DEFAULT_MEMORY_CONSOLIDATION_PROMPT`; injectable via constructor args `extraction_prompt`/`consolidation_prompt`/`context_prompt` | `python/packages/core/agent_framework/_harness/_memory.py:31,44-69,955-956,1009` |
| Orchestration prompt constants (Python) | Seven Magentic prompts as module-level string constants with `{task}`/`{team}` placeholders; assigned to overridable manager fields | `python/packages/orchestrations/agent_framework_orchestrations/_magentic.py:108,138,148,169,186,194,245,576-586` |
| Orchestration prompt override tests | Custom prompts passed to manager constructor and asserted verbatim | `python/packages/orchestrations/tests/test_magentic.py:1139-1193` |
| Handoff nudge prompt | `_AUTONOMOUS_MODE_DEFAULT_PROMPT` constant, overridable via `autonomous_mode_prompt` | `python/packages/orchestrations/agent_framework_orchestrations/_handoff.py:196,244` |
| User-supplied agent instructions | `Agent(instructions=...)` normalized into `default_options["instructions"]` | `python/packages/core/agent_framework/_agents.py:893-894,905-928` |
| Runtime application of prompts | Chat client prepends `options["instructions"]` as a `system` message before each model call via `prepend_instructions_to_messages(..., role="system")` | `python/packages/openai/agent_framework_openai/_chat_completion_client.py:753-756` |
| Per-run instruction merging | Session/context-provider instructions concatenated into `chat_options["instructions"]` via `_append_instructions` | `python/packages/core/agent_framework/_agents.py:59,171-173,1646-1649` |
| Declarative YAML storage | `AgentFactory.create_agent_from_yaml_path` / `create_agent_from_yaml` / `create_agent_from_dict`; YAML parsed then dispatched to `PromptAgent` | `python/packages/declarative/agent_framework_declarative/_loader.py:291,416-416,418-484` |
| Declarative schema carries instructions, not versions | `PromptAgent.__init__` fields: `model`, `tools`, `template`, `instructions`, `additionalInstructions` — no version field | `python/packages/declarative/agent_framework_declarative/_models.py:863-903` |
| Env-driven prompt values without redeploy | PowerFx evaluation of `=Env.X` expressions in YAML values (incl. `instructions`); safe_mode ContextVar gates env access | `python/packages/declarative/agent_framework_declarative/_models.py:51-80,40,902` |
| Service-side versioned prompts (Foundry) | `agent_version` setting (env `FOUNDRY_AGENT_VERSION`); `_build_agent_reference` emits `{name, type: agent_reference, version}` into `extra_body` | `python/packages/foundry/agent_framework_foundry/_agent.py:80-94,143-152,238,390-394` |
| Run-to-prompt association (telemetry) | Span attribute `SYSTEM_INSTRUCTIONS = "gen_ai.system_instructions"`; captured from options at chat-span creation and propagated to the owning agent span | `python/packages/core/agent_framework/observability.py:285,1656-1673,1787-1801,2794-2806` |
| Telemetry gating | Capture requires `OBSERVABILITY_SETTINGS.use_latest_experimental_gen_ai_semconv`; sensitive capture requires `enable_sensitive_data` (env `ENABLE_SENSITIVE_DATA`, default False) | `python/packages/core/agent_framework/observability.py:2798,771-780,833,895,925` |
| Telemetry tests | Non-streaming capture, baseline-semconv omission, and streaming capture of `gen_ai.system_instructions` | `python/packages/core/tests/core/test_observability.py:345-364,372-388,395-418` |
| DevUI inspection of live prompts | Entity discovery extracts `instructions` metadata from running agent objects for display | `python/packages/devui/agent_framework_devui/_discovery.py:348-362`; `python/packages/devui/agent_framework_devui/_utils.py:29` |
| Deployment independence (hosting) | DevUI deployment eligibility targets Azure Container Apps for directory-based entities (requires `__init__.py`) | `python/packages/devui/agent_framework_devui/_discovery.py:483-506` |
| Hosting manifests pin protocol (not prompt) versions | `protocols: [{protocol: responses, version: 2.0.0}]` in `agent.yaml` / `agent.manifest.yaml` | `python/samples/04-hosting/foundry-hosted-agents/responses/foundry_toolbox/agent.yaml:3-5`, `agent.manifest.yaml:11-16` |
| .NET prompt storage + overrides | `MagenticDefaultPrompts` public defaults; `MagenticPromptOverrides` record with per-prompt placeholder-documented overrides; single-brace regex substitution at render time | `dotnet/src/Microsoft.Agents.AI.Workflows/MagenticDefaultPrompts.cs`, `MagenticPromptOverrides.cs:22-63`, `Specialized/Magentic/PromptTemplates.cs:11-24,46-52` |
| Externalization precedent (migration docs) | Semantic Kernel migration sample wraps prompts in `KernelPromptTemplate(PromptTemplateConfig(template=...))` — external template engines supported only via user code | `python/samples/semantic-kernel-migration/orchestrations/group_chat.py:41,114` |

## Answers to Dimension Questions

**1. Where are prompts stored?**
Three tiers. (a) Inline in source code as module-level string constants across the core harness (`python/packages/core/agent_framework/_harness/_agent.py:54`), harness providers (`_memory.py:31,44,57`; `_file_access.py:48`), orchestrations (`_magentic.py:108-245`; `_handoff.py:196`), and security tooling (`security.py:2442`); application-level agent instructions live on `Agent.default_options["instructions"]` (`_agents.py:909`). (b) In declarative YAML files loaded at runtime (`_loader.py:291-337`), including inline YAML strings (`_loader.py:177-188`). (c) Service-side in Microsoft Foundry for hosted PromptAgents (`foundry/_agent.py:159-162` — "Connects to existing PromptAgents… Does not create or delete agents"). There is no database-backed or platform-registry prompt store inside the repo; searches for prompt-registry/prompt-platform integrations (langfuse/promptlayer-style) found no evidence.

**2. Are prompt versions tracked?**
Not within the framework. No prompt constant, declarative field, or serialized record carries a prompt version identifier — `PromptAgent` has no version field (`_models.py:863-903`), and the only `version` constructs are protocol records (`_models.py:963-972`) and hosting-manifest protocol versions (`agent.yaml:3-5`). The sole exception is delegation to the Foundry service, where `agent_version` pins a service-side revision (`foundry/_agent.py:88-94,150-151`). Otherwise, prompt history is implicit in git history and package releases (the Python release workflow centralizes CHANGELOG assembly per `python/AGENTS.md` "Changelog Ownership"), which is provenance-by-release, not prompt versioning.

**3. Can a run be traced to the exact prompt version used?**
Partially, and only under opt-in configuration. Each model call's effective system instructions are written to the span attribute `gen_ai.system_instructions` (`observability.py:1656,2794-2806`), including merged provider/session contributions (`_agents.py:1646-1649`), so a recorded trace captures the exact prompt *text* used. However: capture requires the latest-experimental GenAI semconv flag (`observability.py:2798`) and, for message content generally, `ENABLE_SENSITIVE_DATA=true` (default False, explicitly discouraged outside dev/test — `observability.py:762-780`); the attribute stores text, not a version/hash, so correlating to "a version" still requires knowing the running build; and traces are ephemeral unless the operator ships them to a retained backend (`observability.py:766-769`). With tracing disabled or on baseline semconv, there is **no** durable record linking a run to its prompt (`test_observability.py:372-388` confirms baseline omits the attribute). DevUI can display current instructions for interactive inspection (`devui/_discovery.py:348-362`) but does not log per-run associations.

**4. Can prompts be updated without redeploying code?**
Yes, through two mechanisms. (a) Declarative YAML: edit the `instructions` field of an `agent.yaml` and reload via `AgentFactory.create_agent_from_yaml_path` (`_loader.py:291-337`); PowerFx `=Env.VAR` indirection even lets environment variables supply prompt/model values at load time (`_models.py:51-80,74`), though `safe_mode=True` (default) blocks env access (`_models.py:37-40`). (b) Foundry hosted agents: the prompt lives in the service; clients select revisions by name+version reference without shipping new code (`foundry/_agent.py:143-152`). For code-defined prompts (all built-in harness/orchestration defaults), updates require a package change — the constants ship inside the wheel, mitigated only by their constructor override parameters.

## Architectural Decisions

1. **Defaults-in-code, overrides-in-constructor.** Every built-in prompt is a named module-level constant paired with a constructor parameter (`ORCHESTRATOR_TASK_LEDGER_FACTS_PROMPT` → `task_ledger_facts_prompt=` at `python/packages/orchestrations/agent_framework_orchestrations/_magentic.py:108,576`). This makes the default discoverable and diffable in review but welds prompt evolution to package releases.
2. **Instructions as request options, not a first-class prompt object (Python).** `instructions` is just a key in `default_options` (`_agents.py:905-928`) that each provider client materializes into a system message at call time (`openai/_chat_completion_client.py:753-756`). This keeps the runtime simple but means there is no object to hang version/metadata on.
3. **Structured composition over templating.** Rather than a template engine, instructions are composed by concatenation helpers (`_append_instructions` used at `_agents.py:171-173,1646-1649`; harness+agent assembly at `_harness/_agent.py:72-79`). Placeholders are resolved ad hoc per consumer (Magentic uses `{token}` substitution; the loop judge substitutes `{{criteria}}` at `_harness/_loop.py:405`).
4. **Delegation of versioning to services where available.** For Foundry PromptAgents, identity = name+version resolved server-side (`foundry/_agent.py:143-152`); the framework deliberately does not create or manage those agents (`_agent.py:159-162`).
5. **Telemetry as the traceability substrate.** Instead of a run→prompt ledger, the design leans on OTel GenAI semconv attributes (`observability.py:285,283-284`) — consistent with the framework's broader observability-first posture, but inheriting its privacy gating.
6. **Cross-language divergence in prompt API maturity (.NET vs Python).** .NET formalizes Magentic overrides as a public record with documented per-prompt placeholders and composable language directives (`dotnet/src/Microsoft.Agents.AI.Workflows/MagenticPromptOverrides.cs:22-63`, `PromptTemplates.cs:26-43`), while Python exposes plain constructor strings — same storage model, different interface rigor.

## Notable Patterns

- **Public-default pattern (.NET):** `MagenticDefaultPrompts` exists specifically "so callers can read and base overrides on them" (`dotnet/src/Microsoft.Agents.AI.Workflows/Specialized/Magentic/PromptTemplates.cs:13-15`) — treating prompt text as public API surface, which is a lightweight substitute for versioning (a breaking prompt change is at least a visible API change).
- **Placeholder-safe substitution:** the .NET renderer replaces only `\{(\w+)\}` tokens in a single pass so literal braces (JSON inside prompts) survive intact (`PromptTemplates.cs:11-21`); Python Magentic relies on plain format-style `{task}` slots in the constants (`_magentic.py:108-137`).
- **Safe-mode indirection:** declarative YAML values starting with `=` are evaluated as PowerFx, with env access behind a `ContextVar` default-safe toggle (`_models.py:37-40,70-74`) — prompts can reference config without hard-redeploy while keeping secrets out by default.
- **Span-propagation discipline:** chat-level instruction capture is copied up to the owning agent span only when parentage matches and instructions agree (`observability.py:2809-2846`) — careful attribution logic for the run→prompt link it does provide.
- **DevUI as prompt inspector:** discovery surfaces `instructions` alongside tools/model for every entity (`devui/_discovery.py:344-362`), giving developers a live view of what will be sent.

## Tradeoffs

- **Code-resident defaults** maximize review safety and IDE discoverability but couple prompt iteration cadence to package releases; teams must fork or wrap to hot-fix a bad prompt.
- **Opt-in content telemetry** protects privacy by default (sensitive capture explicitly flagged as unsafe outside dev/test, `observability.py:762-774`) at the cost of losing the run→prompt trail exactly where production incidents happen.
- **Text-not-hash span attributes**: capturing full instruction text (`observability.py:2800-2806`) is unambiguous for audits but bloats span payloads and still yields no comparable identity for grouping runs by prompt revision.
- **Constructor-string overrides (Python)** are trivially flexible but unversioned and unvalidated — an override typo ships silently, whereas .NET's placeholder contract at least documents required slots (e.g., `{schema}` is mandatory for progress-ledger parsing, `MagenticPromptOverrides.cs:55-62`).
- **Service-side versioning (Foundry)** provides real revision pinning, but only for that hosting path; local/self-hosted deployments get nothing equivalent.

## Failure Modes / Edge Cases

- **Silent non-capture:** with default settings, `gen_ai.system_instructions` is absent from spans (baseline-semconv path asserted at `test_observability.py:372-388`; gate check at `observability.py:2798`), so post-hoc incident analysis cannot recover which prompt variant ran.
- **Untracked drift between environments:** because declarative YAML supports env-var substitution (`_models.py:74`), two deployments of the same `agent.yaml` can run materially different instructions with no artifact difference other than environment state — and no version field to flag it.
- **Override breakage in structured-output prompts:** the .NET docs warn that omitting the required `{schema}` placeholder in a progress-ledger override breaks JSON parsing and next-speaker routing (`MagenticPromptOverrides.cs:55-62`); Python offers no such documented contract, making prompt overrides riskier there.
- **Concatenation collisions:** instructions arriving from multiple context providers are joined with `"\n".join(...)` (`_agents.py:1646-1649`); conflicting guidance composes rather than errors, and the resulting blended prompt has no identity of its own.
- **PowerFx degradation:** if the `powerfx` engine is unavailable, `=...` values are returned unevaluated and only logged at debug (`_models.py:62-69`) — a prompt could reach the model containing raw expression syntax with no loud failure.

## Future Considerations

- Add a stable prompt identity (content hash or semantic version) to the few framework-owned prompt surfaces — most tractably on the Magentic manager fields (`_magentic.py:576-586`) — and emit it as a span attribute alongside the existing text capture, giving run→prompt correlation that survives privacy gating.
- Extend the declarative `PromptAgent` schema with optional version/metadata fields (`_models.py:863-903`) and echo them into agent `additional_properties` so YAML-loaded prompts carry provenance end to end.
- Mirror the .NET `MagenticDefaultPrompts` public-surface pattern in Python so override baselines are readable without importing private modules, closing the cross-language parity gap noted above.
- Document a supported recipe for durable run→prompt auditing (OTel setup + retention) since the primitives already exist (`observability.py:285,283-284`) but the wiring is left entirely to operators.

## Questions / Gaps

- **No evidence found** for any database-, registry-, or platform-backed prompt store within the source: searched `python/packages/**` for `prompt_template|PromptTemplate|promptregistry|promptlayer|langfuse` — the only `PromptTemplate` hit outside tests is a Semantic Kernel migration sample (`python/samples/semantic-kernel-migration/orchestrations/group_chat.py:41,114`).
- **Go implementation is absent**: `go/` contains only `go/README.md` (placeholder), so no Go prompt behavior could be studied.
- Whether Foundry service-side PromptAgents expose immutable version history could not be verified from this repository — the client merely forwards `version` in `agent_reference` (`foundry/_agent.py:143-152`); the semantics live outside this source.
- The `Template`/`Format`/`Parser` classes in the declarative schema (`_models.py:488-527`) appear to be manifest-template plumbing (dispatch entries at `_models.py:1139-1140`); no evidence they constitute a prompt-rendering pipeline. Intent inferred from usage sites, not documented.
- DevUI "deployment" (`_discovery.py:483-506`) checks packaging eligibility for Azure Container Apps but the actual deployment pipeline lives outside this source; whether deployed artifacts preserve prompt provenance is unverifiable here.

---

Generated by `Dimension 12.01: Prompt Storage and Versioning` against `agent-framework`.
