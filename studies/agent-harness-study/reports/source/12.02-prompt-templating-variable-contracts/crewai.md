# Source Analysis: crewai

## Dimension 12.02 — Prompt Templating and Variable Contracts

### Source Info

| Field | Value |
|-------|-------|
| Name | crewai |
| Path | `studies/agent-harness-study/sources/crewai` |
| Language / Stack | Python (monorepo; primary package `lib/crewai`, v1.15.17 per `lib/crewai/src/crewai/__init__.py`) |
| Analyzed | 2026-08-25 |

All citations below are workspace-relative paths rooted at the selected source directory (`studies/agent-harness-study/sources/crewai/`).

## Summary

CrewAI does not use a single prompt-template engine. It operates **five distinct templating mechanisms** across layers, each with different variable syntax and different missing-variable behavior:

1. **Hand-rolled `{var}` interpolation** (`interpolate_only`, `lib/crewai/src/crewai/utilities/string_utils.py:79-150`) — regex-matched identifiers replaced via `str.replace`, with input type validation and loud missing-variable errors. This is the contract used for user-facing crew/task inputs.
2. **Python `str.format()` over JSON prompt slices** stored in `lib/crewai/src/crewai/translations/en.json` (e.g., `slices.role_playing` = `"You are {role}. {backstory}\nYour personal goal is: {goal}"`), filled at call sites such as `lib/crewai/src/crewai/lite_agent.py:846-862` and `lib/crewai/src/crewai/tools/agent_tools/agent_tools.py:28-33`. Missing keys here raise raw `KeyError` from `str.format`.
3. **Go/Ollama-style custom agent templates** using `{{ .System }}`, `{{ .Prompt }}`, `{{ .Response }}` markers, substituted by plain `.replace()` in `Prompts._build_prompt` (`lib/crewai/src/crewai/utilities/prompts.py:241-249`); silently no-op when markers are absent.
4. **Raw `str.replace` in executor prompt formatting** with direct dict indexing (`_format_prompt`, `lib/crewai/src/crewai/agents/crew_agent_executor.py:1589-1601`) — crashes with bare `KeyError` on missing `"input"`.
5. **CEL-based `${...}` template expressions** for declarative Flows (`lib/crewai/src/crewai/flow/expressions.py:175-399`) — the most mature subsystem, with declaration-time validation against allowed roots, typed rendering rules, structured errors, and an extensive test suite.

Additionally, Python's `string.Template` ($-substitution) is used for A2A protocol notices (`lib/crewai/src/crewai/a2a/templates.py:7-29`), and Jinja2 is used only for rendering Markdown documentation artifacts, not LLM prompts (`lib/crewai/src/crewai/flow/skill.py:42-48`, autoescape explicitly disabled).

The strongest property of the core path is that interpolation is deliberately brace-conservative: only `{identifier}` patterns are touched, so embedded JSON examples in task descriptions survive intact (`lib/crewai/tests/utilities/test_string_utils.py:33-76`). The weakest properties are the absence of any escaping layer for interpolated values (raw string substitution throughout) and inconsistent missing-variable behavior ranging from hard `ValueError` to silent literal passthrough depending on which layer misses.

## Rating

**6 / 10** — Present but inconsistent and partially fragile.

Rationale against the rubric:
- The core `interpolate_only` function has a clear model, thorough tests including failure modes, and predictable errors (`lib/crewai/src/crewai/utilities/string_utils.py:79-150`; `lib/crewai/tests/utilities/test_string_utils.py:78-100,133-146`).
- The Flow CEL expression system is genuinely well-engineered: explicit contract text, declaration-time validation with allowed roots, typed render semantics, structured `ExpressionError`s, and tests proving rejection of unknown roots and unterminated expressions (`lib/crewai/src/crewai/flow/expressions.py:94-106,195-243`; `lib/crewai/tests/test_flow_from_definition.py:944-975,3040-3069`). On its own it would score 8+.
- However, the classic Crew/Agent path fragments into four coexisting substitution styles with divergent failure behavior: loud wrapped `ValueError` (tasks), bare `KeyError` (agent role/goal/backstory via `lib/crewai/src/crewai/agents/agent_builder/base_agent.py:794-802`; `_format_prompt` via `lib/crewai/src/crewai/agents/crew_agent_executor.py:1599-1601`), silent no-op replacement (`{{ .System }}` markers and `{goal}`/`{role}`/`{backstory}` in `lib/crewai/src/crewai/utilities/prompts.py:241-257`), and silent literal passthrough when kickoff inputs are empty (`lib/crewai/src/crewai/task.py:1077-1078`). No escaping utilities exist anywhere in the prompt path, and no schema declares which variables a given template requires.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Template engine (core) | `interpolate_only`: regex `\{([A-Za-z_][A-Za-z0-9_\-]*)\}` + `str.replace` loop; docstring states JSON is left untouched | `lib/crewai/src/crewai/utilities/string_utils.py:12,79-150` |
| Input type validation | `_validate_type` allows str/int/float/bool/dict/list only; rejects others with `ValueError` | `lib/crewai/src/crewai/utilities/string_utils.py:102-124` |
| Missing-variable handling (core) | Pre-scans template variables, raises `KeyError("Template variable 'X' not found in inputs dictionary")` naming first miss | `lib/crewai/src/crewai/utilities/string_utils.py:135-142` |
| Empty-inputs guard | Raises `ValueError` if placeholders exist but inputs dict empty | `lib/crewai/src/crewai/utilities/string_utils.py:130-133` |
| Task injection point | `Task.interpolate_inputs_and_add_conversation_history` interpolates description, expected_output, output_file from preserved originals | `lib/crewai/src/crewai/task.py:1057-1109` |
| Error wrapping (task) | `KeyError` re-raised as `ValueError(f"Missing required template variable '{e.args[0]}' in description")` — field context added | `lib/crewai/src/crewai/task.py:1084-1089,1096-1109` |
| Silent passthrough edge | `if not inputs: return` before any interpolation — empty kickoff inputs leave `{topic}` literals in prompts | `lib/crewai/src/crewai/task.py:1077-1078` |
| Agent injection point | `BaseAgent.interpolate_inputs` interpolates role/goal/backstory; no try/except, raw `KeyError` propagates | `lib/crewai/src/crewai/agents/agent_builder/base_agent.py:784-802` |
| Crew orchestration | `Crew._interpolate_inputs` fans out to all tasks then agents at kickoff | `lib/crewai/src/crewai/crew.py:2185-2195` |
| Kickoff entry | Normalized inputs (defaulting to `{}`) trigger interpolation during kickoff setup | `lib/crewai/src/crewai/crews/utils.py:300-313,345-353` |
| i18n template store | `I18N` loads `translations/en.json` (or custom `prompt_file`) in a pydantic validator; cached via `lru_cache` | `lib/crewai/src/crewai/utilities/i18n.py:26-54,131-147` |
| i18n missing key | `retrieve` raises generic `Exception(f"Prompt for '{kind}':'{key}' not found.")` | `lib/crewai/src/crewai/utilities/i18n.py:125-128` |
| Variable schemas (implicit) | 60+ placeholder keys across slices/errors/tools/memory/reasoning/planning, e.g. `{role}`, `{input}`, `{tool_names}`, `{expected_output}` | `lib/crewai/src/crewai/translations/en.json` |
| `str.format` fill sites | System prompt built via `I18N_DEFAULT.slice(...).format(role=..., backstory=..., goal=..., tools=..., tool_names=...)` | `lib/crewai/src/crewai/lite_agent.py:846-862` |
| Delegation tool descriptions | `I18N_DEFAULT.tools("delegate_work").format(coworkers=coworkers)` | `lib/crewai/src/crewai/tools/agent_tools/agent_tools.py:28-33` |
| Custom agent templates | `system_template`/`prompt_template`/`response_template` fields on Agent | `lib/crewai/src/crewai/agent/core.py:237-245` |
| Ollama-style marker substitution | `system_template.replace("{{ .System }}", ...)`, `prompt_template.replace("{{ .Prompt }}", ...)`, `response_template.split("{{ .Response }}")[0]` — silent no-op if markers absent | `lib/crewai/src/crewai/utilities/prompts.py:230-251` |
| Unvalidated attribute substitution | `prompt.replace("{goal}", self.agent.goal)` etc. applied post-join; TypeError if None, silent no-op if placeholder absent | `lib/crewai/src/crewai/utilities/prompts.py:253-257` |
| Executor formatting | `_format_prompt` uses `inputs["input"]` direct indexing → bare `KeyError` if absent | `lib/crewai/src/crewai/agents/crew_agent_executor.py:1589-1601` |
| Date-injection validation | `inject_date` format checked against whitelist `VALID_DATE_FORMAT_CODES`; invalid format logs warning and returns "" (graceful degradation) | `lib/crewai/src/crewai/utilities/prompts.py:13-24,143-163` |
| CEL engine (Flow) | `${...}` parsing with balanced-brace scanner; empty/unterminated → `ExpressionError` | `lib/crewai/src/crewai/flow/expressions.py:55-91` |
| Explicit variable contract (text) | `FLOW_TEMPLATE_EXPRESSION_RULES`: "Use `${...}` inside action mapping strings… Use `state` for input data. Use `outputs.step_name`…" | `lib/crewai/src/crewai/flow/expressions.py:94-106` |
| CEL root allowlist validation | `validate_expression`/`validate_template` reject roots outside `allowed_roots` with named error listing permitted roots | `lib/crewai/src/crewai/flow/expressions.py:195-227` |
| Typed render rule | Single `${...}` keeps value type; mixed strings stringify (null → ""); non-text values become JSON | `lib/crewai/src/crewai/flow/expressions.py:236-244,326-344` |
| Declaration-time enforcement | `_validate_action_cel` validates every action's `with`/`inputs`/`expr`/`in` at definition load | `lib/crewai/src/crewai/flow/flow_definition.py:928-967` |
| Flow runtime render sites | CodeAction/ToolAction/CrewAction/AgentAction render templates before invoking handlers/crews | `lib/crewai/src/crewai/flow/runtime/_actions.py:71-79,101-107,163-168` |
| Trust-boundary statement | Script action: "intentionally do not interpolate user input… still arbitrary trusted Python execution… disabled by default behind `CREWAI_ALLOW_FLOW_SCRIPT_EXECUTION`" | `lib/crewai/src/crewai/flow/runtime/_actions.py:268-273` |
| Escaping (absence) | Only JSON re-serialization escapes exist (converter output), none for prompt-interpolated values; Jinja2 autoescape off for Markdown skill docs (`# noqa: S701 - renders trusted Markdown`) | `lib/crewai/src/crewai/flow/skill.py:43-48`; `lib/crewai/src/crewai/utilities/converter.py:230,425` |
| `string.Template` usage | A2A agent-status/conversation templates use `$var` substitution | `lib/crewai/src/crewai/a2a/templates.py:7-29` |
| Documented contract in field help | "Crew inputs are interpolated with `{name}` …" repeated in role/goal/backstory/description/expected_output field descriptions | `lib/crewai/src/crewai/project/crew_definition.py:72,80,88,250,257` |
| Test: JSON preservation | Templates containing JSON objects keep braces because regex excludes non-identifier content | `lib/crewai/tests/utilities/test_string_utils.py:33-76,148-176` |
| Test: missing var & types | Missing var raises `KeyError`; unsupported types raise `ValueError` | `lib/crewai/tests/utilities/test_string_utils.py:78-100` |
| Test: brace edge cases | `{123}`, `{!var}` preserved verbatim while `{valid_var}` interpolates; `interpolate_only("{}", ...)` returns `"{}"` | `lib/crewai/tests/utilities/test_string_utils.py:133-146`; `lib/crewai/tests/test_task.py:1481-1488` |
| Test: nested-value rejection | set(), nested invalid objects, and custom objects rejected as input values | `lib/crewai/tests/test_task.py:1420-1457` |
| Test: partial custom templates tolerated | Agents with only `system_template`, only `prompt_template`, or missing `response_template` construct without error | `lib/crewai/tests/agents/test_agent.py:140-181` |
| Test: full custom template round-trip | `{{ .System }}/{{ .Prompt }}/{{ .Response }}` markers expanded into expected llama-style prompt | `lib/crewai/tests/agents/test_agent.py:940-989` |
| Test: i18n fallback file + not-found | Custom `prompts.json` loads; unknown kind/key raises | `lib/crewai/tests/utilities/test_i18n.py:29-43` |
| Test: CEL rejection matrix | Unterminated `${`, empty `${}`, unknown CEL root all raise `ValidationError` at declaration load; `${'${'}` escape produces literal `${x}` | `lib/crewai/tests/test_flow_from_definition.py:920-975,3040-3069` |
| Config surface | YAML `AgentConfig` exposes `system_template`/`prompt_template`/`response_template`/`date_format` | `lib/crewai/src/crewai/project/crew_base.py:64-66,90-91` |

## Answers to Dimension Questions

### 1. How are prompts parameterized?

Three parameterization styles coexist:

- **Crew inputs**: users write `{variable}` placeholders in task descriptions, expected outputs, output-file paths, and agent role/goal/backstory. At `kickoff()`, `Crew._interpolate_inputs` fans inputs out to every task (`lib/crewai/src/crewai/task.py:1057-1109`) and every agent (`lib/crewai/src/crewai/agents/agent_builder/base_agent.py:784-802`). Originals are snapshotted so repeated kickoffs interpolate from pristine templates (`lib/crewai/src/crewai/task.py:1070-1075`).
- **Framework-internal slices**: ReAct scaffolding lives in `lib/crewai/src/crewai/translations/en.json` and is composed by `Prompts.task_execution` selecting component slices (`role_playing`, `tools`/`no_tools`, `native_task`/`task`/`task_no_tools`) based on tool availability and native function-calling mode (`lib/crewai/src/crewai/utilities/prompts.py:93-141`). Fill happens via `str.format` at call sites (e.g., `lib/crewai/src/crewai/lite_agent.py:846-854`).
- **Declarative Flow values**: `${...}` CEL expressions inside action mapping strings are rendered against serialized flow state and method outputs just before handlers run (`lib/crewai/src/crewai/flow/runtime/_actions.py:71-107,163-168`).

### 2. Are variable contracts explicit?

Partially, and unevenly by layer:

- **Strongest**: the Flow CEL layer enforces a machine-checked contract — allowed root identifiers (`state`, `outputs`, locals) are validated at declaration time and violations fail loading with a message enumerating permitted roots (`lib/crewai/src/crewai/flow/expressions.py:195-215`; wiring at `lib/crewai/src/crewai/flow/flow_definition.py:928-967`). The human-readable contract is also embedded in the skill documentation constants (`lib/crewai/src/crewai/flow/expressions.py:94-106`).
- **Documented but unchecked**: crew-level contracts are stated in pydantic field descriptions ("Crew inputs are interpolated with `{name}`", `lib/crewai/src/crewai/project/crew_definition.py:72,80,88,250,257`) and enforced only opportunistically at runtime by `interpolate_only`'s pre-scan (`lib/crewai/src/crewai/utilities/string_utils.py:135-142`). There is no schema declaring the required variable set of a task/agent up front, so a typo like `{topik}` surfaces only at kickoff, mid-run.
- **Implicit**: framework slice variables (`{tool_names}`, `{tools}`, `{memory}`, etc. in `lib/crewai/src/crewai/translations/en.json`) are bound positionally-by-name at each call site; nothing verifies that a slice's placeholders match the kwargs provided to `.format(...)` beyond Python's native KeyError/IndexError.
- **Absent**: custom `{{ .System }}`-style templates have no validation at all — a template missing its marker silently renders without the intended content (`lib/crewai/src/crewai/utilities/prompts.py:241-249`).

### 3. Is missing-variable behavior predictable?

No — it varies by mechanism, though each individual mechanism is deterministic:

| Layer | Missing variable outcome | Evidence |
|-------|--------------------------|----------|
| Task description | `ValueError` naming the variable and the field | `lib/crewai/src/crewai/task.py:1084-1089` |
| Task expected_output / output_file | `ValueError` naming the field | `lib/crewai/src/crewai/task.py:1100-1109` |
| Agent role/goal/backstory | Bare `KeyError` from `interpolate_only`, no field context | `lib/crewai/src/crewai/agents/agent_builder/base_agent.py:794-802` |
| Kickoff with empty/no inputs | Interpolation skipped entirely; `{placeholders}` remain literally in the prompt | `lib/crewai/src/crewai/task.py:1077-1078`; normalization to `{}` at `lib/crewai/src/crewai/crews/utils.py:302-305` |
| Executor `_format_prompt` | Bare `KeyError` on `inputs["input"]` | `lib/crewai/src/crewai/agents/crew_agent_executor.py:1599-1601` |
| i18n slice key | Generic `Exception("Prompt for '<kind>':'<key>' not found.")` | `lib/crewai/src/crewai/utilities/i18n.py:125-128` |
| `str.format` on slices | Native `KeyError`/`IndexError` | e.g., `lib/crewai/src/crewai/lite_agent.py:846-862` |
| Custom `{{ .X }}` markers | Silent no-op (missing marker = unexpanded template) | `lib/crewai/src/crewai/utilities/prompts.py:241-249` |
| `{goal}`/`{role}`/`{backstory}` in `Prompts._build_prompt` | Silent no-op if absent from template; `TypeError` if agent attribute is None | `lib/crewai/src/crewai/utilities/prompts.py:253-257` |
| Date injection | Warning logged, date omitted (graceful) | `lib/crewai/src/crewai/utilities/prompts.py:153-161` |
| Flow CEL | `ExpressionError`/`ValidationError` with precise source path (e.g., `methods.search.do.with.inputs`) | `lib/crewai/src/crewai/flow/expressions.py:255-269,361-364`; `lib/crewai/src/crewai/flow/flow_definition.py:944-959` |

### 4. Are variables properly escaped?

No. There is **no escaping layer for prompt interpolation** anywhere in the codebase:

- Substituted values are inserted raw via `result.replace(placeholder, value)` (`lib/crewai/src/crewai/utilities/string_utils.py:144-148`); a search for escaping utilities in the prompt path finds none (the only "escape"-related code concerns JSON re-serialization in `lib/crewai/src/crewai/utilities/converter.py:230,425` and unrelated filesystem checks).
- This is partly mitigated *for the template itself* by the conservative regex: braces that don't form valid identifiers (JSON bodies, `{123}`, `{!var}`) are never interpreted, verified by tests (`lib/crewai/tests/utilities/test_string_utils.py:33-76,133-146`). But *values* are never sanitized: a task input containing `{other_var}` text can be double-expanded when `other_var` also appears later in the same template (single pass iterates original-template variables but replaces against the mutated result, `lib/crewai/src/crewai/utilities/string_utils.py:135-148`).
- Prompt-injection through data values (an input containing "ignore previous instructions…") passes through unquoted and undelimited — consistent with the framework's documented trust model, where flow scripts explicitly note they "intentionally do not interpolate user input" and gate script execution behind `CREWAI_ALLOW_FLOW_SCRIPT_EXECUTION` (`lib/crewai/src/crewai/flow/runtime/_actions.py:268-273`), but no equivalent defense exists for crew input values.
- Where HTML output matters (visualization), Jinja2 autoescaping *is* enabled (`lib/crewai/src/crewai/flow/visualization/renderers/interactive.py:400`), showing the team applies escaping selectively rather than uniformly.

## Architectural Decisions

1. **Deliberate avoidance of a general template engine for LLM prompts.** Rather than Jinja2/Mako, the core uses a ~70-line hand-rolled replacer whose regex deliberately ignores non-identifier braces so JSON examples embedded in task descriptions survive interpolation (`lib/crewai/src/crewai/utilities/string_utils.py:83-85`; test proof at `lib/crewai/tests/test_task.py:962-989`). A full engine would either require escaping discipline from users or mangle JSON-laden prompts.
2. **Translation-table prompt composition.** All ReAct scaffolding is externalized to `en.json` keyed slices selected at runtime by capability flags (tools/native tool calling/system-prompt mode), enabling localization by swapping one file via `I18N(prompt_file=...)` (`lib/crewai/src/crewai/utilities/i18n.py:21-24,37-45`; caching at :131-147).
3. **Ollama-compatible custom template escape hatch.** Custom agent templates reuse the `{{ .System }}`/`{{ .Prompt }}`/`{{ .Response }}` marker convention from Ollama Modelfiles, implemented as three `.replace()` calls rather than a parser (`lib/crewai/src/crewai/utilities/prompts.py:236-251`).
4. **CEL as the typed templating substrate for declarative flows.** Newer Flow definitions moved to CEL expressions with compile-once parse caching (`lru_cache(maxsize=256)`, `lib/crewai/src/crewai/flow/expressions.py:76-77`), declaration-time root allowlisting (:195-215), and type-preserving single-expression semantics (:335-336). This is the only subsystem with fail-fast validation before execution.
5. **Interpolate-at-kickoff with original snapshots.** Both tasks and agents snapshot their original text before first interpolation so subsequent kickoffs don't compound substitutions (`lib/crewai/src/crewai/task.py:1070-1075`; `lib/crewai/src/crewai/agents/agent_builder/base_agent.py:786-791`).
6. **Prompt-cache-aware assembly.** Volatile blocks (current date, skill catalogs) are appended at the tail so the stable prefix remains cacheable (`lib/crewai/src/crewai/utilities/prompts.py:144-148,165-171`), and executor messages carry explicit cache breakpoints (`lib/crewai/src/crewai/agents/crew_agent_executor.py:189-199`).

## Notable Patterns

- **Regex-guarded interpolation preserving JSON**: identifier-only pattern plus early exit when no braces present (`lib/crewai/src/crewai/utilities/string_utils.py:126-135`).
- **Error enrichment at boundaries**: the task layer translates internal `KeyError` into contextual `ValueError`s naming both variable and field (`lib/crewai/src/crewai/task.py:1084-1089`) — a pattern notably absent from the agent-layer equivalent (`base_agent.py:794-802`).
- **Contract-as-documentation**: interpolation syntax is declared in field descriptions consumed by schema generation (`lib/crewai/src/crewai/project/crew_definition.py:72,250,257`) and in reusable skill-doc constants (`lib/crewai/src/crewai/flow/expressions.py:94-107`), so generated references always teach the current syntax.
- **Graceful degradation for auxiliary injections**: invalid `date_format` logs a warning and drops the block instead of failing the run (`lib/crewai/src/crewai/utilities/prompts.py:153-161`) — appropriate for non-essential context.
- **Escape hatch for literals in CEL**: `${'${'}` yields a literal `${` (tested at `lib/crewai/tests/test_flow_from_definition.py:932-941`), acknowledging that `${` will legitimately appear in prompt text.
- **Source-path-tagged validation errors**: CEL failures carry dotted source paths (`f"{path}.with.inputs"`, `lib/crewai/src/crewai/flow/flow_definition.py:951-958`), making multi-nested declarations debuggable.

## Tradeoffs

- **Simplicity vs safety**: raw `str.replace` substitution is trivially predictable and JSON-safe for templates, but offers zero escaping/quoting of values; unsafe content flows into prompts unchanged (`lib/crewai/src/crewai/utilities/string_utils.py:144-148`).
- **Fail-late vs fail-fast (classic crews)**: variable mistakes surface only at kickoff, potentially after long-running upstream work in a pipeline, whereas the Flow layer validates everything at load time. The two generations of the framework embody opposite philosophies.
- **Silent tolerance vs correctness (custom templates)**: accepting partial template sets and no-op-ing missing markers lowers the barrier for users porting Modelfiles but can produce subtly wrong prompts with no signal (`lib/crewai/src/crewai/utilities/prompts.py:230,241-249`; tolerated combinations tested at `lib/crewai/tests/agents/test_agent.py:140-181`).
- **Localization flexibility vs contract drift**: allowing arbitrary `prompt_file` overrides means a custom JSON omitting a slice fails only when that slice is first requested, with a generic exception far from authoring time (`lib/crewai/src/crewai/utilities/i18n.py:37-49,125-128`).
- **Five mechanisms vs one**: matching each feature's syntax to its community's expectations (Modelfile markers, `{name}` f-string familiarity, CEL for typed data) aids adoption but multiplies maintenance surface and makes behavior non-uniform across the framework.

## Failure Modes / Edge Cases

- **Empty-inputs passthrough**: `kickoff()` or `kickoff({})` skips interpolation entirely (`lib/crewai/src/crewai/task.py:1077-1078`), so templates containing `{topic}` execute with the literal placeholder visible to the LLM — no warning, unlike the loud failure when inputs are non-empty but incomplete.
- **Nested expansion leak**: an input value containing `{b}` expands again if `{b}` appears later in the same template, since replacements apply to the mutated string (`lib/crewai/src/crewai/utilities/string_utils.py:135-148`); bounded but surprising.
- **None attributes crash `_build_prompt`**: `prompt.replace("{goal}", self.agent.goal)` raises `TypeError` if goal is None instead of a descriptive error (`lib/crewai/src/crewai/utilities/prompts.py:253-257`).
- **Bare `KeyError` from `_format_prompt`**: direct `inputs["input"]` indexing gives an unactionable `'input'` error with no template context (`lib/crewai/src/crewai/agents/crew_agent_executor.py:1599-1601`).
- **Custom template marker omission**: a `system_template` lacking `{{ .System }}` renders as-is, dropping role/backstory/goal silently (`lib/crewai/src/crewai/utilities/prompts.py:241-243`); similarly `split("{{ .Response }}")[0]` discards everything after the marker when present, or truncates nothing when absent (:247-248).
- **i18n override drift**: a custom `prompt_file` missing any consumed slice key raises a generic Exception only at first use (`lib/crewai/src/crewai/utilities/i18n.py:125-128`); there is no completeness check against required keys.
- **`str.format` fragility on framework slices**: any literal `{` in future slice text (e.g., JSON examples inside `en.json` entries) would break `.format()` calls — the reason user-authored templates use `interpolate_only` instead; the split between the two systems must be maintained manually.
- **Unbounded value size/type coercion**: values are coerced with bare `str(value)` (`string_utils.py:146-147`); dicts/lists serialize via Python repr rather than JSON, so interpolated structures may render in non-JSON syntax unless pre-stringified by callers (conversation history is explicitly `json.dumps`-ed upstream at `lib/crewai/src/crewai/task.py:1116-1119`).

## Future Considerations

- **Declare variable requirements per template**: a lightweight schema (task → required vars) would enable fail-fast validation at crew construction, mirroring what the Flow layer already achieves with CEL root allowlisting (`lib/crewai/src/crewai/flow/flow_definition.py:928-967`).
- **Unify missing-variable semantics**: route agent-layer interpolation through the task-layer wrapping pattern (field-context `ValueError`s) and validate custom-template markers eagerly instead of silent `.replace` no-ops (`lib/crewai/src/crewai/utilities/prompts.py:241-257`).
- **Add opt-in value sanitization**: delimit or neutralize `{identifier}` sequences inside interpolated values to prevent nested expansion, analogous to the `${'${'}` escape already supported in CEL (`lib/crewai/tests/test_flow_from_definition.py:932-941`).
- **Consolidate engines**: migrate legacy `{{ .System }}` support and `_format_prompt` onto `interpolate_only` (or vice versa) to shrink the five-mechanism surface; retain `en.json` `str.format` fills behind a validated binder that checks slice placeholders against supplied kwargs.
- **Extend declaration-time validation to crew YAML**: `AgentConfig`/`TaskConfig` files (`lib/crewai/src/crewai/project/crew_base.py:64-66`) could be linted for unresolved `{vars}` against declared crew inputs before kickoff.

## Questions / Gaps

- **No evidence found** for any escaping, quoting, or delimiter mechanism applied to interpolated prompt values; searches covered `escape|sanitize.*prompt|prompt.*sanitiz` across `lib/**/src` and reviewed all substitution sites listed above.
- **No evidence found** for a declarative registry mapping framework slices (`en.json` keys) to their required variables; the contract exists only implicitly in the JSON text and per-call-site kwargs.
- **No evidence found** of telemetry/events emitted on interpolation failures (only logger warnings for date injection, `lib/crewai/src/crewai/utilities/prompts.py:157-161`); whether kickoff-time interpolation errors reach the event bus was not traced exhaustively.
- The `ContentProcessorProvider` hook (`lib/crewai/src/crewai/core/providers/content_processor.py:7-78`) allows post-interpolation transformation of task descriptions, but no in-repo provider demonstrates sanitization use — its intended production processors were not found in this source.
- Whether `test_i18n.py`'s explicit `i18n.load_prompts()` calls reflect a stale public API versus the current pydantic-validator design (`lib/crewai/src/crewai/utilities/i18n.py:26-54`) was not resolved; the tests may exercise deprecated behavior.

---

Generated by `12.02-prompt-templating-and-variable-contracts` against `crewai`.
