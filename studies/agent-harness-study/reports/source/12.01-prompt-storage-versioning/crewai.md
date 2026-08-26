# Source Analysis: crewai

## Dimension 12.01: Prompt Storage and Versioning

### Source Info

| Field | Value |
|-------|-------|
| Name | crewai |
| Path | `studies/agent-harness-study/sources/crewai` |
| Language / Stack | Python (uv monorepo: `lib/crewai`, `lib/crewai-core`, `lib/crewai-tools`, `lib/crewai-files`, `lib/cli`) |
| Analyzed | 2026-08-25 |

## Summary

CrewAI stores prompts as flat JSON "slices" shipped inside the package (`lib/crewai/src/crewai/translations/en.json`), loaded by an `I18N` Pydantic model into a process-lifetime cached singleton (`lib/crewai/src/crewai/utilities/i18n.py:131-147`). Users can override prompts three ways: a crew-level `prompt_file` pointing to a replacement JSON (`lib/crewai/src/crewai/crew.py:329-332`), per-agent string templates (`system_template`/`prompt_template`/`response_template`, `lib/crewai/src/crewai/agent/core.py:237-245`), or YAML agent config carrying the same template keys (`lib/crewai/src/crewai/project/crew_base.py:64-66`). There is **no prompt versioning whatsoever**: no version identifiers, hashes, registry, database, or external platform integration exist in the codebase. The only run-to-prompt association is incidental — event payloads carry the fully rendered prompt text (`LLMCallStartedEvent.messages`, `lib/crewai/src/crewai/events/types/llm_events.py:38-61`) and telemetry records the prompt *file path* (usually `None`) when users opt into sharing (`lib/crewai/src/crewai/telemetry/telemetry.py:354`). Prompts can be swapped at runtime via `prompt_file`, but the mechanism is whole-file replacement (not merge), cached for the life of the process.

## Rating

**4 / 10** — Present but inconsistent and fragile.

Rationale against the rubric:

- Storage has a clear shape (JSON slices + typed accessor API + tests, `lib/crewai/tests/utilities/test_i18n.py:5-43`), which keeps it out of the 1–3 band.
- But versioning is entirely absent: you cannot tell *which* prompt revision produced an output unless you happened to capture the full rendered text through the event bus. The package version (`lib/crewai/src/crewai/version.py:11-14`) is the only proxy, and it changes for reasons unrelated to prompts.
- Documentation contradicts implementation on override semantics (docs claim merging, code replaces wholesale — see Tradeoffs), and the runtime cache means edits to a `prompt_file` are invisible until restart.
- No operational safeguards: no schema validation of custom files beyond "is JSON", no key coverage checks, generic exceptions raised mid-execution for missing slices.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Built-in prompt storage | Single JSON file with `hierarchical_manager_agent`, `slices`, `errors`, `tools`, `memory` sections; 102 lines, no version metadata | `lib/crewai/src/crewai/translations/en.json:1-102` |
| Loader | `I18N.load_prompts` reads `prompt_file` if set, else resolves `../translations/en.json` relative to module | `lib/crewai/src/crewai/utilities/i18n.py:26-54` |
| Process-level caching | `get_i18n()` wrapped in `lru_cache(maxsize=None)` keyed by path; module singleton `I18N_DEFAULT = get_i18n()` | `lib/crewai/src/crewai/utilities/i18n.py:131-147` |
| Typed accessors | `slice()`, `errors()`, `tools()`, `memory()`, `retrieve(kind, key)` with fixed kind literal list | `lib/crewai/src/crewai/utilities/i18n.py:56-128` |
| Slice assembly | `Prompts.task_execution()` composes `role_playing` + tools/task slices per execution mode | `lib/crewai/src/crewai/utilities/prompts.py:93-141` |
| Code-resident prompt blocks | Date injection and skill catalog blocks are built in Python strings, not JSON | `lib/crewai/src/crewai/utilities/prompts.py:143-163`, `lib/crewai/src/crewai/utilities/prompts.py:165-209` |
| Template substitution | Naive `.replace("{{ .System }}", ...)`, then global `{goal}/{role}/{backstory}` replacement | `lib/crewai/src/crewai/utilities/prompts.py:241-257` |
| Agent-level template fields | `system_template`, `prompt_template`, `response_template` optional strings on agent | `lib/crewai/src/crewai/agent/core.py:237-245` |
| Template consumption | `_build_prompt_with_stop_words` passes agent templates into `Prompts`; stop word derived by splitting `response_template` on `{{ .Response }}` | `lib/crewai/src/crewai/agent/core.py:1091-1109` |
| YAML-config templates | `AgentConfig` TypedDict exposes `system_template`/`prompt_template`/`response_template` from `config/agents.yaml` | `lib/crewai/src/crewai/project/crew_base.py:64-66` |
| Crew-level prompt file | `prompt_file: str \| None` field documented as "Path to the prompt json file" | `lib/crewai/src/crewai/crew.py:329-332` |
| Runtime use of prompt_file | Hierarchical manager agent persona pulled from `get_i18n(prompt_file=self.prompt_file)` | `lib/crewai/src/crewai/crew.py:1532-1541` |
| Lite agent prompts | System prompt rendered at call time from `I18N_DEFAULT.slice(...)` literals | `lib/crewai/src/crewai/lite_agent.py:836-872` |
| Deprecated per-agent i18n | `Agent.i18n` field marked deprecated in favor of `Crew(prompt_file=...)` | `lib/crewai/src/crewai/agents/agent_builder/base_agent.py:323-330` |
| Telemetry privacy stance | Header comment: "No prompts, task descriptions, agent backstories/goals, responses" are recorded | `lib/crewai/src/crewai/telemetry/telemetry.py:4` |
| Only version-ish telemetry | `"i18n": I18N_DEFAULT.prompt_file` span attribute (file path, typically `None`) when `share_crew=True` | `lib/crewai/src/crewai/telemetry/telemetry.py:354`, `lib/crewai/src/crewai/telemetry/telemetry.py:884` |
| Full-text capture events | `LLMCallStartedEvent.messages` carries complete rendered messages per LLM call | `lib/crewai/src/crewai/events/types/llm_events.py:38-61` |
| Agent-run prompt event | `AgentExecutionStartedEvent.task_prompt: str` emitted with task prompt | `lib/crewai/src/crewai/events/types/agent_events.py:17-23` |
| Per-call formatting point | Executor formats stored prompt with runtime inputs (`_format_prompt`) | `lib/crewai/src/crewai/agents/crew_agent_executor.py:111`, `182-203`, `1589-1600` |
| Prompt caching (not versioning) | `mark_cache_breakpoint` marks stable prefix for provider-side prompt caching | `lib/crewai/src/crewai/llms/cache.py:27` |
| Fingerprinting excludes prompts | `Fingerprint` UUIDs identify crews/agents for tracking, not prompt content | `lib/crewai/src/crewai/security/fingerprint.py:41-58` |
| Version search result | Search for `prompt_version|prompt_hash|prompt_id` found no prompt-version identifiers anywhere in `lib/` | searched `lib/**` via grep |
| Tests | I18N unit tests incl. loading a custom `prompts.json` fixture and asserting slice content | `lib/crewai/tests/utilities/test_i18n.py:5-43` |
| Docs: customization guide | Documents `custom_prompts.json` + `Crew(prompt_file=...)`; advises repo-based version control of prompt files as user practice | `docs/edge/en/guides/advanced/customizing-prompts.mdx:140-162`, `204-212` |
| Docs vs code mismatch | Doc claims "CrewAI then merges your customizations with the defaults" while also warning to "list all top-level prompts"; loader implements full replacement only | `docs/edge/en/guides/advanced/customizing-prompts.mdx:221,227` vs `lib/crewai/src/crewai/utilities/i18n.py:37-39` |

## Answers to Dimension Questions

1. **Where are prompts stored?**
   Primarily in one JSON file shipped inside the Python package: `lib/crewai/src/crewai/translations/en.json:1-102`, organized as named slices under top-level kinds (`slices`, `errors`, `tools`, `memory`, `hierarchical_manager_agent`). It is loaded by `I18N` from a path resolved relative to the source file (`lib/crewai/src/crewai/utilities/i18n.py:41-45`) and exposed as a cached singleton (`i18n.py:147`). Some prompt fragments live directly in Python code (skill catalog block `lib/crewai/src/crewai/utilities/prompts.py:165-209`; date block `prompts.py:143-163`; post-tool reasoning nudge inserted at `lib/crewai/src/crewai/agents/crew_agent_executor.py:778-805`). There is no database, registry, or external prompt platform; observability integrations for capturing prompts are delegated to third-party tools (`docs/edge/en/guides/advanced/customizing-prompts.mdx:186-190`).

2. **Are prompt versions tracked?**
   No. A repo-wide search for `prompt_version`, `prompt_hash`, and `prompt_id` returned no matches in library code. `en.json` carries no version field; its history exists only as ordinary git commits (e.g., commit `f4731f5` touching the translations). The closest constructs are unrelated: agent/crew `Fingerprint` UUIDs identify components, not prompt content (`lib/crewai/src/crewai/security/fingerprint.py:41-58`), and the package version (`lib/crewai/src/crewai/version.py:11-14`) is not tied to prompt revisions. Telemetry deliberately excludes prompt content (`lib/crewai/src/crewai/telemetry/telemetry.py:4`) and records only the prompt file *path* (`telemetry.py:354`).

3. **Can a run be traced to the exact prompt version used?**
   Not by version identifier. Two partial mechanisms exist: (a) the event bus emits the fully rendered message list on every LLM call (`LLMCallStartedEvent.messages`, `lib/crewai/src/crewai/events/types/llm_events.py:47`) plus the task prompt on agent start (`lib/crewai/src/crewai/events/types/agent_events.py:23`) — so an operator who persists these events can reconstruct exactly what was sent; (b) the executor holds the assembled prompt and re-formats it with inputs each run (`lib/crewai/src/crewai/agents/crew_agent_executor.py:182-203`). Neither records which *revision* of the underlying template produced it. Reproducing a historical prompt therefore requires knowing the exact package release (and any user-supplied `prompt_file` content), since defaults can change between releases with no trace in run artifacts.

4. **Can prompts be updated without redeploying code?**
   Partially. `Crew(prompt_file=...)` accepts any JSON path at construction time and replaces the built-in slice table (`lib/crewai/src/crewai/crew.py:329-332`, `lib/crewai/src/crewai/utilities/i18n.py:37-39`), so prompt text can be changed by editing/re-pointing a file without touching the installed library. Limitations: the file must be structurally complete because the loader performs no merge with defaults (despite docs suggesting otherwise, `docs/edge/en/guides/advanced/customizing-prompts.mdx:221`); results are cached per path for the process lifetime via `lru_cache` (`i18n.py:131-144`), so edits require a new process; and missing keys surface only as late runtime exceptions during execution (`i18n.py:125-128`). Agent-level templates (`lib/crewai/src/crewai/agent/core.py:237-245`) travel with application code/YAML, so they deploy with the app.

## Architectural Decisions

- **Prompts-as-data inside the package**: All default prompt text lives in one JSON asset rather than scattered f-strings, giving a single override point and enabling the i18n naming ("internationalization") to double as a customization seam (`lib/crewai/src/crewai/utilities/i18n.py:13-24`).
- **Composition over templates by default**: Default prompts are assembled from ordered *slices* chosen by execution mode (`role_playing` + `tools`/`no_tools` + task variant) in `Prompts.task_execution()` (`lib/crewai/src/crewai/utilities/prompts.py:99-118`), rather than requiring users to author whole templates. Whole-template overrides remain available via the `{{ .System }}`/`{{ .Prompt }}`/`{{ .Response }}` placeholder convention (`prompts.py:241-251`).
- **Singleton read path**: `I18N_DEFAULT` is initialized at import time and memoized (`lib/crewai/src/crewai/utilities/i18n.py:131-147`); most subsystems (lite agent `lib/crewai/src/crewai/lite_agent.py:846`, tool usage, reasoning handlers) read this global rather than receiving an injected instance — simple, but it hard-wires the default prompt set unless callers remember to thread a custom `get_i18n(...)` result (only the hierarchical manager does, `lib/crewai/src/crewai/crew.py:1532`).
- **Deprecating per-agent i18n in favor of crew-scoped files**: `Agent.i18n` is explicitly deprecated with guidance to use `Crew(prompt_file=...)` (`lib/crewai/src/crewai/agents/agent_builder/base_agent.py:323-330`), consolidating prompt configuration at crew level.
- **Privacy-first telemetry**: Prompt content is excluded from product telemetry by design (`lib/crewai/src/crewai/telemetry/telemetry.py:4`), pushing run-to-prompt capture responsibility onto user-owned event listeners/observability stacks.

## Notable Patterns

- **Slice-keyed retrieval with fail-loud errors**: typed accessor methods funnel into `retrieve(kind, key)` which raises a generic exception for unknown keys (`lib/crewai/src/crewai/utilities/i18n.py:100-128`).
- **Prompt-cache anchoring as a first-class concern**: comments document that date injection is kept at the prompt tail so the stable prefix remains a cache anchor (`lib/crewai/src/crewai/utilities/prompts.py:144-148`), and `mark_cache_breakpoint` tags formatted messages (`lib/crewai/src/crewai/llms/cache.py:27`, applied at `lib/crewai/src/crewai/agents/crew_agent_executor.py:194-203`). This optimizes reuse cost but is distinct from versioning.
- **Behavioral regression tests over content snapshots**: `test_prompts_no_thought_leakage.py` asserts structural properties (which slices appear, absence of ReAct instructions for tool-less agents) rather than pinning exact prompt text (`lib/crewai/tests/utilities/test_prompts_no_thought_leakage.py:16-45`), so prompt wording stays free to drift between releases.
- **Docs as the versioning substitute**: the official guidance tells users to manage their own prompt files in version control and document changes (`docs/edge/en/guides/advanced/customizing-prompts.mdx:208-211`) — versioning is framed as a user responsibility, not a framework feature.

## Tradeoffs

- **Zero versioning buys simplicity at the cost of reproducibility**: there is no overhead of registries or schemas, but the question "which prompt produced this output?" is answerable only if the operator independently captured event payloads; otherwise they must map output timestamps to package releases manually.
- **Whole-file replacement vs merge ambiguity**: implementation loads *only* the custom file (`lib/crewai/src/crewai/utilities/i18n.py:37-39`), yet docs simultaneously promise merging and warn that all needed top-level keys must be present (`docs/edge/en/guides/advanced/customizing-prompts.mdx:221,227`). Users following the merge claim get `KeyError`-style failures deep in execution.
- **Runtime swappability vs staleness**: `prompt_file` allows non-code updates, but `lru_cache` per path (`i18n.py:131-144`) means a corrected file at the same path is ignored until restart within long-lived processes.
- **Code-resident fragments escape the override seam**: skill catalog and date blocks are appended after slice rendering (`lib/crewai/src/crewai/utilities/prompts.py:105-109`), so even a complete `prompt_file` cannot remove or reword those additions.
- **Telemetry privacy vs debuggability**: excluding prompts from telemetry protects users but removes the easiest server-side avenue for correlating behavior with prompt revisions (`telemetry.py:4` vs `telemetry.py:354` recording only a path that is usually `None`).

## Failure Modes / Edge Cases

- **Late failure on incomplete custom files**: a `prompt_file` missing any accessed key raises `Exception("Prompt for '<kind>':'<key>' not found.")` at first retrieval — potentially mid-kickoff, not at load time (`lib/crewai/src/crewai/utilities/i18n.py:125-128`).
- **Template marker fragility**: stop-word extraction assumes `{{ .Response }}` exists, splitting index `[1]` unconditionally (`lib/crewai/src/crewai/agent/core.py:1106-1109`); a malformed `response_template` raises at executor build. Placeholder substitution is blind string `.replace()` with no validation that markers were actually consumed (`lib/crewai/src/crewai/utilities/prompts.py:241-251`).
- **Global brace collisions**: `{goal}`/`{role}`/`{backstory}` are replaced across the entire composed prompt including user-authored slices (`lib/crewai/src/crewai/utilities/prompts.py:253-257`); task text containing those tokens would be silently rewritten.
- **Cache invisibility**: edits to a `prompt_file` during process lifetime never take effect due to `lru_cache(maxsize=None)` (`i18n.py:131-144`).
- **Silent cross-release drift**: because nothing records prompt revisions, an upgrade that rewrites `en.json` slices changes agent behavior with no artifact distinguishing old vs new prompts; only git history of the upstream repo shows intent (e.g., commit `f4731f5`).
- **Deprecated seam still wired**: `Agent.i18n` remains functional but deprecated (`base_agent.py:323-330`); code holding stale references gets defaults inconsistent with a crew-level `prompt_file`.

## Future Considerations

- Add a content hash or explicit `version` key to the translation file and stamp it onto `LLMCallStartedEvent`/telemetry spans (the span plumbing already exists at `lib/crewai/src/crewai/telemetry/telemetry.py:354` where only the path is written today).
- Implement true overlay merging of custom `prompt_file`s with built-in defaults (or fix the documentation), plus eager validation of required keys at load time instead of first-use (`i18n.py:37-39`, `125-128`).
- Offer cache invalidation or reload semantics for `get_i18n` in long-running services (`i18n.py:131-144`).
- Route code-built fragments (skill/date blocks, `prompts.py:143-209`) through the same data-driven slice mechanism so overrides cover the full prompt surface.

## Questions / Gaps

- No evidence found of any prompt registry, database table, or external platform client (searched `lib/` for `prompt_version|prompt_hash|prompt_id|registry` near prompts; the `crewai-core` package contains auth/settings/telemetry infrastructure only — verified via directory listing of `lib/crewai-core/src/crewai_core/`). If CrewAI's commercial offering manages prompts remotely, that logic is outside this repository.
- Whether provider-side prompt caching effectiveness is tested end-to-end could not be confirmed from prompt-storage code alone; cassettes exist for cached token accounting (e.g., `lib/crewai/tests/cassettes/llms/openai/test_openai_completions_cached_prompt_tokens.yaml`) but they measure token reporting, not breakpoint correctness.
- The docs' merge-vs-replace contradiction (`docs/edge/en/guides/advanced/customizing-prompts.mdx:221` vs `lib/crewai/src/crewai/utilities/i18n.py:37-39`) could not be reconciled by any test; no test exercises a partial custom file against the default set.

---

Generated by dimension `12.01-prompt-storage-and-versioning` against `crewai`.
