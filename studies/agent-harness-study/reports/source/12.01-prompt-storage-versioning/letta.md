# Source Analysis: letta

## 12.01 Prompt Storage and Versioning

### Source Info

| Field | Value |
|-------|-------|
| Name | letta (formerly MemGPT) |
| Path | `studies/agent-harness-study/sources/letta` |
| Language / Stack | Python (FastAPI server, SQLAlchemy ORM, Alembic migrations, Pydantic schemas) |
| Analyzed | 2026-08-25 |

> All citations below are relative to the source root `studies/agent-harness-study/sources/letta/`.

## Summary

Letta stores prompts in three layers with copy-on-create snapshot semantics: (1) built-in system prompts as Python string constants keyed in a central registry (`letta/prompts/system_prompts/__init__.py:15-27`), (2) a per-agent `system` prompt copied into the database at agent creation (`letta/orm/agent.py:66`, selected via `derive_system_message` at `letta/services/helpers/agent_manager_helper.py:164-226`), and (3) the fully rendered system message persisted verbatim as a role=`system` row in the `messages` table (`letta/orm/message.py:23-43`).

Versioning is implicit only — template names carry generation markers (`memgpt_v2_chat`, `sleeptime_v2`, `letta_v1`) but there is no prompt-version identifier anywhere in the schema, ORM, or step/run metadata. The rendered system message is updated **in place** on rebuild (same message ID reused, `letta/services/agent_manager.py:1595-1604`), so historical rendered versions are destroyed rather than appended. A DB-backed `prompts` registry table exists but is dormant — created by migration `alembic/versions/ddecfe4902bc_add_prompts.py:22-36` with an ORM model (`letta/orm/prompt.py:8-13`) that no manager or API route ever references. Runtime updates without redeploy are possible for per-agent prompts (REST `UpdateAgent.system`, request-scoped `override_system`) and via a local file override directory (`~/.letta/system_prompts/*.txt`, `letta/prompts/gpt_system.py:12-22`), but built-in template changes require a code deploy.

**Answer to the dimension's core question ("Can you tell exactly which prompt version produced a given output?"):** Only partially. You can recover the exact *rendered* prompt text for the current context window from the persisted system message plus step-linked messages, but you cannot tell which *template version* produced it, and past rendered versions are overwritten in place.

## Rating

**4 / 10** — Present but inconsistent and fragile.

Rationale against the rubric:
- Storage locations are explicit and centralized (constants + per-agent DB column + persisted rendered message), which keeps this out of the 1–3 band.
- But version tracking is absent beyond naming conventions; rendered-prompt history is actively destroyed by in-place updates (`letta/services/agent_manager.py:1600`); the `prompts` registry is dead infrastructure; the file-override mechanism is undocumented and untested; and no tests cover template selection logic (`derive_system_message`). This matches "present but inconsistent, weakly documented, or fragile" (4–6), at the low end.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Built-in system prompt constants | 11 named templates (`voice_chat`, `memgpt_v2_chat`, `sleeptime_v2`, `react`, `letta_v1`, `workflow`, etc.) registered in a `SYSTEM_PROMPTS` dict | `letta/prompts/system_prompts/__init__.py:15-27` |
| Example template content | `PROMPT = r"""<base_instructions>..."""` for the letta_v1 agent loop | `letta/prompts/system_prompts/letta_v1.py:1-25` |
| File-based override fallback | `get_system_text(key)` checks Python constants first, then falls back to `~/.letta/system_prompts/{key}.txt`; raises `FileNotFoundError` otherwise | `letta/prompts/gpt_system.py:7-24` |
| Override dir creation | `create_config_dir()` creates `~/.letta/{personas,humans,system_prompts,presets,...}` | `letta/config.py:292-310` |
| Template selection by agent type | `derive_system_message(agent_type, enable_sleeptime, system)` maps `AgentType` → registry key | `letta/services/helpers/agent_manager_helper.py:164-226` |
| AgentType enum values | `memgpt_agent`, `memgpt_v2_agent`, `letta_v1_agent`, `react_agent`, `workflow_agent`, `sleeptime_agent`, `voice_*` | `letta/schemas/enums.py:81-94` |
| Per-agent prompt persistence (schema) | `system: Mapped[Optional[str]] ... doc="The system prompt used by the agent."` | `letta/orm/agent.py:66` |
| Copy-on-create | Agent creation calls `derive_system_message(...)` and stores result on the new agent row | `letta/services/agent_manager.py:516-520` |
| Rendered message storage | `messages` table stores role/text/content; linked to `step_id` and `run_id` | `letta/orm/message.py:23-53` |
| Initial render | `initialize_message_sequence(_async)` compiles `{CORE_MEMORY}` into the template and emits role=`system` first message | `letta/services/helpers/agent_manager_helper.py:336-396`, `399-463` |
| Templating mechanism | `{CORE_MEMORY}` f-string substitution + `<memory_metadata>` block (agent id, conversation id, recompile timestamp, counts); mustache raises `NotImplementedError` | `letta/prompts/prompt_generator.py:107-177`, `26-89`; `letta/constants.py:64` |
| Rebuild triggers | Memory-block edits force rebuild of connected agents' system messages | `letta/services/block_manager.py:61-66`, `268` |
| **In-place overwrite (no history)** | Async rebuild sets `temp_message.id = curr_system_message.id` then `update_message_by_id_async` — same ID mutated; sync path likewise | `letta/services/agent_manager.py:1595-1604`, `1500-1514` |
| Change detection by substring match | `if curr_memory_str in curr_system_message_openai["content"] and not force:` skip rebuild (code comments admit fragility) | `letta/services/agent_manager.py:1562-1567`, `1467-1472` |
| Summarizer prompts as constants | `ALL_PROMPT`, `SLIDING_PROMPT`, `SELF_ALL_PROMPT`, `SELF_SLIDING_PROMPT` | `letta/prompts/summarizer_prompt.py:4-60+`; legacy `letta/prompts/gpt_summarize.py:1-12` |
| Compaction prompt snapshot per agent | `CompactionSettings.set_mode_specific_prompt()` materializes default prompt at agent creation; stored as JSON column | `letta/services/summarizer/summarizer_config.py:35-45`, `68`, `84-89`; `letta/orm/agent.py:86` |
| Boot-message "versions" | `get_initial_boot_messages(version)` dispatches on strings `"startup"`, `"startup_with_send_message"`, `"startup_with_send_message_gpt35"` | `letta/system.py:18-93`; text at `letta/constants.py:231-238` |
| **Dormant prompts table** | Migration creates `prompts(id, prompt, project_id, ...)`; ORM + Pydantic model exist; zero consumers outside `letta.orm.__init__` | `alembic/versions/ddecfe4902bc_add_prompts.py:22-36`; `letta/orm/prompt.py:8-13`; `letta/schemas/prompt.py:6-9`; `letta/orm/__init__.py:30` |
| Deprecated templating remnants | `Memory.prompt_template` marked "Deprecated. Ignored for performance."; `get_prompt_template_for_agent_type()` returns `""` with docstring "Templates are not used anymore" | `letta/schemas/memory.py:106`; `letta/schemas/agent.py:559-561` |
| Template lineage proxy (cloud) | Agents/blocks/groups carry `template_id`/`base_template_id`/`deployment_id`; runs/steps record them as metrics | `letta/orm/mixins.py:88-90`; `letta/schemas/run.py:34`; `letta/schemas/step_metrics.py:24-25`; `letta/schemas/agent.py:126-128` |
| Steps record model, not prompt identity | `Step` columns: provider, model, token counts, trace/request IDs — no prompt/template/version field | `letta/orm/step.py:34-87` |
| Runtime update without redeploy | `UpdateAgent.system` optional field; PATCH flows call `rebuild_system_prompt_async(force=True)` | `letta/schemas/agent.py:225`; `letta/server/rest_api/routers/v1/agents.py:1286`, `1312`; `letta/server/rest_api/routers/v1/internal_agents.py:51` |
| Request-scoped override (not persisted) | `override_system` alias `system` on LettaRequest; honored exactly in `generate_request_system_prompt` | `letta/schemas/letta_request.py:119-121`; `letta/agents/letta_agent_v2.py:795-810` |
| Prefix-caching stability tests | Rendered system prompt asserted byte-stable across memory edits/messages; changes only after reset | `tests/integration_test_system_prompt_prefix_caching.py:40-112`, `197-232` |
| Request-scoped composition tests | Skills block appended to request prompt without mutating persisted storage; stale skills never repaired in place | `tests/test_letta_agent_v2_skills.py:69-86`, `90-124`, `127-150`, `153-179` |
| Vestigial Jinja templates | `letta/templates/*.j2` (3 files) exist but no `jinja` import or loader anywhere in `letta/` | `letta/templates/summary_request_text.j2` (unreferenced) |
| Prompt evolution via code PRs | Git log shows prompt text changed through feature PRs, e.g. `5da764a65 feat: new prompt for letta_v1_agent (#5197)` | git history of `letta/prompts/system_prompts/` |

## Answers to Dimension Questions

### 1. Where are prompts stored?

Three active layers plus several vestigial ones:

- **Built-in templates**: Python module-level string constants under `letta/prompts/system_prompts/*.py`, aggregated into the `SYSTEM_PROMPTS` key→text dict (`letta/prompts/system_prompts/__init__.py:15-27`). Accessor is `gpt_system.get_system_text(key)` (`letta/prompts/gpt_system.py:7-10`).
- **Per-agent prompt**: a plain string copied onto the agent row at creation (`letta/orm/agent.py:66`; write path `letta/services/agent_manager.py:516-520`). Selection defaults come from `derive_system_message`, which switches on `AgentType` (`letta/services/helpers/agent_manager_helper.py:186-224`) and raises `ValueError` for unknown types (`:223-224`).
- **Rendered system message**: after compiling `{CORE_MEMORY}` and the `<memory_metadata>` header (`letta/prompts/prompt_generator.py:107-177`), the final text is persisted as a role=`system` Message (`letta/orm/message.py:41-43`) and is what actually reaches the LLM.
- **Summarizer/compaction prompts**: constants in `letta/prompts/summarizer_prompt.py:4-45+`, snapshotted per-agent into `compaction_settings.prompt` JSON (`letta/services/summarizer/summarizer_config.py:85-89`; `letta/orm/agent.py:86`).
- **Vestigial**: an unused DB `prompts` table (`letta/orm/prompt.py:8-13`), unreferenced `.j2` Jinja files under `letta/templates/`, legacy persona/human example `.txt` files (`letta/personas/examples/`, `letta/humans/examples/`), and deprecated `Memory.prompt_template` (`letta/schemas/memory.py:106`).

### 2. Are prompt versions tracked?

**No — not explicitly.** A repo-wide search for `prompt_version` / `PromptVersion` / `prompt_template_version` returned no results. Versioning exists only as:
- Naming conventions baked into registry keys: `memgpt_chat` vs `memgpt_v2_chat`, `sleeptime_v2`, `letta_v1` (`letta/prompts/system_prompts/__init__.py:16-26`).
- Boot-message variant strings (`"startup"`, `"startup_with_send_message_gpt35"`) dispatched inside application code (`letta/system.py:18-93`).
- Git history of the constant files (e.g., commit `5da764a65` replacing the `letta_v1` prompt), which is invisible to the runtime.

The deprecation trail (`letta/schemas/memory.py:106`, `letta/schemas/agent.py:559-561`) shows templating infrastructure was recently stripped down, and the dormant `prompts` table suggests an unfinished migration toward DB-backed prompts.

### 3. Can a run be traced to the exact prompt version used?

**Partially — exact bytes yes, provenance no.**
- Messages link to `run_id`/`step_id` (`letta/orm/message.py:48-53`), so all inputs for a given run, including the full rendered system text, can be reconstructed from the DB.
- However, the rendered system message is **overwritten in place**: rebuilds reuse the existing message ID (`temp_message.id = curr_system_message.id`, `letta/services/agent_manager.py:1595-1604`; sync equivalent `:1505-1514`). Once memory or the template changes, the previous rendering is gone from the live tables — only the latest rendering survives.
- No step/run column records template name, template hash, or prompt version (`letta/orm/step.py:34-87` records only provider/model/token/trace fields). The cloud-only `template_id`/`base_template_id` lineage (`letta/schemas/run.py:34`, `letta/schemas/step_metrics.py:24-25`) identifies an agent-template family, not prompt text.
- Indirect reconstruction: because the per-agent `system` base string is snapshotted and the renderer is deterministic given memory state (pinned by stability tests, `tests/integration_test_system_prompt_prefix_caching.py:40-112`), one could re-render historical prompts — but only if the historical block/memory state is also known.

### 4. Can prompts be updated without redeploying code?

**Yes, through three mechanisms; built-in templates cannot.**
- **Per-agent REST updates**: `UpdateAgent.system` (`letta/schemas/agent.py:225`) mutates the agent's base prompt at runtime; PATCH handlers then force a rebuild (`letta/server/rest_api/routers/v1/agents.py:1286`, `1312`).
- **Request-scoped override**: `override_system` (`letta/schemas/letta_request.py:119-121`) replaces the prompt for a single request without persistence (`letta/agents/letta_agent_v2.py:801-804`).
- **Local file override**: dropping `~/.letta/system_prompts/{key}.txt` shadows any built-in registry key (`letta/prompts/gpt_system.py:12-22`). This enables ops-level prompt changes without code deploys, but it is environment-scoped, undocumented, and untested (no test references found).
- Built-in template text itself lives in Python modules, so changing it requires a code release; existing agents are insulated either way because their `system` was copied at creation (`letta/services/agent_manager.py:516-520`).

## Architectural Decisions

1. **Constants-first registry with typed accessor.** All built-in prompts are import-time Python constants behind `SYSTEM_PROMPTS` and `get_system_text` (`letta/prompts/gpt_system.py:7-24`), trading hot-reloadability for zero-I/O lookups and type-safe refactoring.
2. **Copy-on-create snapshot semantics.** Both the agent system prompt (`letta/services/agent_manager.py:516-520`) and the compaction prompt (`letta/services/summarizer/summarizer_config.py:84-89`) are copied onto the agent at creation. Deploys that edit defaults affect only newly created agents; running agents keep their prompt. This prioritizes behavioral stability over fleet-wide consistency.
3. **Single mutable rendered message instead of append-only versions.** The design keeps exactly one role=`system` message per agent/conversation and edits it in place (`letta/services/agent_manager.py:1595-1604`), optimizing for prefix caching and a simple in-context window contract at the cost of auditability.
4. **Template = base string + single reserved variable.** The renderer supports exactly one injected variable, `{CORE_MEMORY}`, appending it if missing (`letta/prompts/prompt_generator.py:154-171`; keyword at `letta/constants.py:64`). Mustache support is stubbed with `NotImplementedError` (`letta/services/helpers/agent_manager_helper.py:328-330`).
5. **Half-finished shift toward DB-backed prompts.** The `prompts` table and ORM exist (`letta/orm/prompt.py:8-13`) with project scoping and audit columns (`alembic/versions/ddecfe4902bc_add_prompts.py:22-36`) but no manager, service, or route consumes them — an abandoned scaffold rather than an operating registry.

## Notable Patterns

- **Key-based prompt addressing**: prompts are addressed by short string keys (`"react"`, `"summary_system_prompt"`) resolved at call sites across the codebase (`letta/services/helpers/agent_manager_helper.py:187-221`, `letta/agents/ephemeral_summary_agent.py:79`, `letta/server/rest_api/routers/v1/tools.py:946`), decoupling consumers from file layout.
- **Request-scoped composition without persistence**: `generate_request_system_prompt` layers client skills onto the stored text per-request and is tested to be non-mutating and deterministic (`letta/agents/letta_agent_v2.py:795-810`; `tests/test_letta_agent_v2_skills.py:127-179`).
- **Diff-gated rebuilds**: rebuild paths compute a `united_diff` between old and rendered text and skip writes when empty (`letta/services/agent_manager.py:1500-1517`, `1590-1592`), plus substring-based change detection with self-admitted fragility comments (`:1562-1563`: "could this cause issues if a block is removed?").
- **Prefix-cache-aware laziness**: v2 agents only recompile when forced (post-compaction) to avoid busting provider prompt caches (`letta/agents/letta_agent_v2.py:760-792`), enforced by integration tests (`tests/integration_test_system_prompt_prefix_caching.py:40-112`).
- **Deprecation breadcrumbs**: explicit markers that old templating surfaces were retired (`letta/schemas/memory.py:106`, `letta/schemas/agent.py:559-561`) document the migration direction even though the replacement registry isn't wired up.

## Tradeoffs

- **Stability over observability**: in-place mutation of the system message keeps prefix caches warm and recall storage uncluttered (the docstring says avoiding "flood[ing] recall storage with excess messages", `letta/services/agent_manager.py:1531-1536`), but destroys the historical record needed to answer "which prompt produced this output?" retroactively.
- **Snapshots over inheritance**: copying prompts at creation isolates agents from deploy-time regressions, but means a critical prompt fix does not reach existing agents and there's no mechanism (version pin, drift report, or bulk migration) to manage divergence across a fleet.
- **Simplicity over safety in overrides**: the `~/.letta/system_prompts` fallback gives no-code prompt swaps (`letta/prompts/gpt_system.py:12-22`) but silently shadows shipped templates based on local disk state, enabling environment-dependent behavior drift with no logging of which source won.
- **Determinism over flexibility**: single-variable f-string templating (`letta/prompts/prompt_generator.py:154-175`) is easy to reason about and cache-friendly, but blocks richer templating (conditionals, loops) that registries like Langfuse/Prompty provide.

## Failure Modes / Edge Cases

- **Audit gap on rebuild**: every memory-triggered rebuild permanently overwrites the prior rendered prompt (`letta/services/agent_manager.py:1600-1604`); debugging "why did the agent behave differently last week?" has no artifact to inspect.
- **False-negative change detection**: `curr_memory_str in curr_system_message_openai["content"]` substring matching can wrongly conclude nothing changed if removed content coincidentally remains present (flagged in-code at `letta/services/agent_manager.py:1562-1563`, `1467-1468`).
- **Silent file shadowing**: a stray `~/.letta/system_prompts/react.txt` changes production prompt text with no warning; `get_system_text` prefers it over the shipped constant (`letta/prompts/gpt_system.py:8-22`), and the directory is auto-created (`letta/config.py:303`).
- **Hard failure on unknown agent type**: `derive_system_message` raises bare `ValueError(f"Invalid agent type")` (`letta/services/helpers/agent_manager_helper.py:223-224`) — adding a new `AgentType` without updating this mapping breaks agent creation at runtime.
- **Missing-variable injection surprise**: if a custom prompt omits `{CORE_MEMORY}`, the renderer appends the entire memory block at the end anyway (`letta/prompts/prompt_generator.py:158-162`), so user-crafted prompts silently gain a large suffix.
- **Template drift between sync/async paths**: two parallel implementations of compile/rebuild exist (`compile_system_message` vs `PromptGenerator.compile_system_message_async`, `letta/services/helpers/agent_manager_helper.py:251-332` vs `letta/prompts/prompt_generator.py:181-224`); they must be kept manually in sync, and the sync path lacks archive-tag support seen in the async one.

## Future Considerations

- **Finish the prompts registry or remove it**: wire `letta/orm/prompt.py` into a manager/API (it already has project scoping and audit columns, `alembic/versions/ddecfe4902bc_add_prompts.py:22-36`), or drop the dead table and `.j2` files to reduce confusion.
- **Record prompt identity on steps**: add `template_key` and a `prompt_sha256` of the base + rendered text to `Step` (`letta/orm/step.py`) so runs become attributable to template versions cheaply.
- **Append-version rendered prompts**: replace in-place ID reuse with insert-new/retire-old system messages (or a side table of renderings) to preserve history while keeping one active pointer.
- **Document and test the override channel**: promote `~/.letta/system_prompts` to documented configuration with startup logging of resolved sources, and add tests for `derive_system_message`'s type→template mapping (currently zero direct test coverage; searches of `tests/` for `derive_system_message`/`SYSTEM_PROMPTS` returned nothing).
- **Fleet drift management**: since prompts are frozen at creation, add tooling to diff agents against current defaults and opt-in bulk upgrades, mirroring how `rebuild_system_prompt_async(force=True)` already exists as a hook point (`letta/server/rest_api/routers/v1/agents.py:1286`).

## Questions / Gaps

- **No evidence found** for any prompt-version identifier recorded at runtime: searches for `prompt_version*`, `template_version`, and inspection of `Step` (`letta/orm/step.py:34-87`), `Message` (`letta/orm/message.py:40-80`), and `Run` (`letta/schemas/run.py:34`) schemas found only cloud `template_id` lineage, which identifies agent-template families, not prompt text revisions.
- **Intended consumer of the `prompts` table unknown**: the migration date (2025-07-21, `alembic/versions/ddecfe4902bc_add_prompts.py:5`) and project-scoped schema suggest planned cloud functionality; whether Letta Cloud uses it externally could not be determined from this OSS tree.
- **Whether `~/.letta/system_prompts` overrides are used in practice** is unverifiable from the repository — no docs page in-tree mentions it and no test exercises it; only `letta/config.py:292-310` and `letta/prompts/gpt_system.py:12-22` reference the directory.
- **Sync-vs-async renderer parity** was not exhaustively diffed; the async `PromptGenerator` path carries `archive_tags` (`letta/prompts/prompt_generator.py:118`) while the sync helper does not (`letta/services/helpers/agent_manager_helper.py:251-267`), implying divergent output in archival-enabled setups — confirmed by reading but not by test execution.

---

Generated by Dimension 12.01 (Prompt Storage and Versioning) against `letta`.
