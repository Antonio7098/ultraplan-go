# Source Analysis: letta

## Prompt Templating and Variable Contracts (Dimension 12.02)

### Source Info

| Field | Value |
|-------|-------|
| Name | letta |
| Path | `studies/agent-harness-study/sources/letta` |
| Language / Stack | Python (FastAPI server, SQLAlchemy ORM, Pydantic schemas) |
| Analyzed | 2026-08-25 |

> **Path convention:** all citations below are workspace-relative to the source root `studies/agent-harness-study/sources/letta/`.

## Summary

Letta does not use a dedicated prompt-template engine at runtime. The system-prompt pipeline is built around a single reserved variable, `{CORE_MEMORY}` (`IN_CONTEXT_MEMORY_KEYWORD`, `letta/constants.py:64`), which is substituted into the agent's stored system string either by plain `str.replace()` (the default path) or by a custom `str.format_map` wrapper called `safe_format` that preserves unknown `{placeholders}` verbatim (`letta/prompts/prompt_generator.py:91-103`, duplicated in `letta/services/helpers/agent_manager_helper.py:229-247`). Everything else that looks like a "template" in the repo is either (a) module-level f-strings interpolated once at import time (summarizer prompts), (b) imperative string assembly via `StringIO` in `Memory.compile` (`letta/schemas/memory.py:688-732`), (c) hand-built line lists for generated sandbox scripts (`letta/services/tool_sandbox/base.py:177-388`), or (d) deprecated/no-op template fields kept for API compatibility. Three Jinja2-syntax files exist under `letta/templates/` but are dead artifacts: nothing in the codebase imports jinja2 or loads `.j2` files; `summary_request_text.j2` has been reimplemented as the Python function `build_summary_request_text` (`letta/services/summarizer/summarizer.py:436-457`). User-defined template variables are explicitly unimplemented (`NotImplementedError`), and a declared `"mustache"` format option also raises `NotImplementedError`. Missing-variable behavior is deliberately forgiving: if `{CORE_MEMORY}` is absent from a system prompt it is silently appended rather than rejected. Escaping is minimal: memory-block values are interpolated raw into XML-style tags with no escaping utility, while tool-execution argument injection relies on `repr()` for strings.

## Rating

**5 / 10** — Present but inconsistent and fragile in places. The core variable contract is extremely simple (one reserved variable), predictable, and covered by unit tests; however, the templating logic is duplicated across two modules, user-variable and mustache paths are unreachable stubs, three orphaned Jinja2 templates drift from their live reimplementation, several schema fields advertise template support that is explicitly ignored, and there is no escaping layer when block values are rendered into XML-structured prompts.

## Evidence Collected

Every entry cites file path with line numbers, relative to source root `studies/agent-harness-study/sources/letta/`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Template engine | No jinja2/mako import anywhere in package code; jinja2 present only as transitive dependency in lockfile | `uv.lock:2144`; grep over `letta/**/*.py` returned no matches for `import jinja`/`from jinja` |
| Template engine | `PreserveMapping(dict)` with `__missing__` returning `"{" + key + "}"` — undefined variables preserved verbatim | `letta/prompts/prompt_generator.py:15-19` |
| Template engine | `safe_format`: escapes empty `{}` → `{{}}`, then `format_map(PreserveMapping(variables))` | `letta/prompts/prompt_generator.py:91-103` |
| Duplication | Identical `PreserveMapping` + module-level `safe_format` duplicated outside the class | `letta/services/helpers/agent_manager_helper.py:229-247` |
| Variable contract | Reserved variable `CORE_MEMORY` defined as constant | `letta/constants.py:64` |
| Variable contract | Docstrings document `CORE_MEMORY` as the only reserved variable | `letta/prompts/prompt_generator.py:121-127`; `letta/services/helpers/agent_manager_helper.py:268-274` |
| Variable contract | `user_defined_variables` param raises `NotImplementedError` (both sync and async variants) | `letta/prompts/prompt_generator.py:128-130`, `202-206` |
| Variable contract | Protected-name check raises `ValueError` if `CORE_MEMORY` appears in user vars (unreachable in practice — see Failure Modes) | `letta/prompts/prompt_generator.py:135-136` |
| Format options | `template_format: Literal["f-string", "mustache"] = "f-string"`; non-f-string raises `NotImplementedError` | `letta/prompts/prompt_generator.py:115`, `173-175`; `letta/services/helpers/agent_manager_helper.py:260`, `328-330` |
| Default render path | When no user variables: `system_prompt.replace(memory_variable_string, full_memory_string)` — plain replace, braces never interpreted | `letta/prompts/prompt_generator.py:164-169`; `letta/services/helpers/agent_manager_helper.py:319-324` |
| Missing-variable behavior | If `{CORE_MEMORY}` missing from system prompt and `append_icm_if_missing=True`, append it at end; warning log commented out | `letta/prompts/prompt_generator.py:156-162` (commented log at :161); `letta/services/helpers/agent_manager_helper.py:312-317` |
| Error wrapping | Any formatting exception wrapped in `ValueError` including full system prompt text | `letta/prompts/prompt_generator.py:170-171` |
| Dead templates | Jinja2-syntax templates on disk: `sandbox_code_file.py.j2`, `sandbox_code_file_async.py.j2`, `summary_request_text.j2` | `letta/templates/sandbox_code_file.py.j2:1-77`; `letta/templates/sandbox_code_file_async.py.j2:1-88`; `letta/templates/summary_request_text.j2:1-16` |
| Dead templates | No loader references `.j2` files; grep for `sandbox_code_file`/`summary_request_text`/`load_template` finds no consumer of the templates dir | grep results; live implementation is `build_summary_request_text` at `letta/services/summarizer/summarizer.py:436-457` duplicating `summary_request_text.j2` content |
| Deprecated template surface | `Memory.prompt_template` field: "Deprecated. Ignored for performance."; setters store without validating/using | `letta/schemas/memory.py:106-120` |
| Deprecated template surface | `get_prompt_template_for_agent_type()` returns `""` ("Templates are not used anymore") | `letta/schemas/agent.py:559-561` |
| Deprecated template surface | Tool-rule `prompt_template` field "(ignored). Rendering uses fast built-in formatting"; each rule hardcodes its prompt in `render_prompt()` f-strings | `letta/schemas/tool_rule.py:17-20`, `115-117` |
| System prompt registry | Prompts are plain Python constants in a dict; optional override via `~/.letta/system_prompts/<key>.txt`; unknown key → `FileNotFoundError` | `letta/prompts/gpt_system.py:7-24`; registry `letta/prompts/system_prompts/__init__.py:15-27` |
| Build-time interpolation | Summarizer prompts are f-strings evaluated at import time (`{ALL_WORD_LIMIT}` resolved at load) | `letta/prompts/summarizer_prompt.py:1-4`, `26`, `45`; `letta/prompts/gpt_summarize.py:1-11` |
| Memory renderer | `Memory.compile` builds prompt imperatively with `StringIO` (no engine); selects standard vs line-numbered rendering by provider/agent type | `letta/schemas/memory.py:688-732` |
| Injection point: init | `initialize_message_sequence(_async)` compiles system message when creating message sequence | `letta/services/helpers/agent_manager_helper.py:346-359`, `410-423` |
| Injection point: rebuild | `rebuild_system_prompt(_async)` recompiles and swaps stored system message on memory change, gated by substring check + united diff | `letta/services/agent_manager.py:1465-1517`, `1577-1588` |
| Injection point: step | Agent steps recompile per turn (`base_agent`, `letta_agent_v2`, voice agent, conversation manager) | `letta/agents/base_agent.py:157`; `letta/agents/letta_agent_v2.py:850-870`; `letta/agents/voice_agent.py:150`; `letta/services/conversation_manager.py:272` |
| Injection point: request-scoped | `generate_request_system_prompt` appends `<available_skills>` block at request time without persisting | `letta/agents/letta_agent_v2.py:794-810`; renderer `letta/schemas/memory.py:483-539` |
| Injection point: tool rules | `ToolRulesSolver.compile_tool_rule_prompts` joins per-rule `render_prompt()` outputs into ephemeral `tool_usage_rules` Block | `letta/helpers/tool_rule_solver.py:209-237` |
| Escaping (prompts) | Block values/descriptions written raw inside XML-style tags; labels become tag names raw (`f"<{label}>"`) — no escaping utility applied | `letta/schemas/memory.py:150-173`, `184-203` |
| Escaping (display only) | `html.escape` used only for HTML UI rendering of responses, never for prompt assembly | `letta/schemas/letta_response.py:103-149` |
| Escaping (sandbox codegen) | String args embedded via `repr()`; other types via `str(raw_value)`; array branch marked "need more testing here" | `letta/services/helpers/tool_parser_helper.py:47-75` |
| Escaping (sandbox codegen) | `agent_state` pickle and markers embedded with `repr(bytes)`; tool source concatenated raw | `letta/services/tool_sandbox/base.py:230`, `267-268`, `376` |
| Structured packaging | User/system messages packaged as JSON envelopes (`json_dumps`) rather than string concatenation | `letta/system.py:126-147` |
| Tests: substitution | `test_formatter` covers no-vars, known var, unknown var preserved (`{USER_MEMORY}` stays literal), empty `{}` preserved | `tests/test_utils.py:416-467` |
| Tests: request-scoped skills | Assert exactly one structural `<available_skills>` tail block, idempotency across repeated calls, literal `` `<available_skills>` `` text in block value preserved unescaped, no persistence mutation | `tests/test_letta_agent_v2_skills.py:69-86`, `127-150`, `153-180`, `183-221` |
| Tests: stability | Integration tests assert compiled system prompt remains stable across memory edits/messages (prefix caching) | `tests/integration_test_system_prompt_prefix_caching.py:40-193` |
| Tests: sandbox script gen | Assertions on generated script contents (imports, client init, marker protocol) | `tests/integration_test_async_tool_sandbox.py:1298-1310` |
| Tests: memory rendering | Line-number warning presence toggled by render mode | `tests/test_memory.py:62-95` |

## Answers to Dimension Questions

### 1. How are prompts parameterized?

Through one runtime variable plus compile-time composition. The only runtime placeholder is `{CORE_MEMORY}` (`letta/constants.py:64`), injected by `PromptGenerator.get_system_message_from_compiled_memory` (`letta/prompts/prompt_generator.py:105-177`) or its sync twin `compile_system_message` (`letta/services/helpers/agent_manager_helper.py:250-332`). The value substituted is produced by `Memory.compile()` (`letta/schemas/memory.py:688-732`), which imperatively renders memory blocks, tool-usage rules (from `ToolRulesSolver.compile_tool_rule_prompts`, `letta/helpers/tool_rule_solver.py:209-237`), and file directories into an XML-ish string, plus a metadata block generated by `compile_memory_metadata_block` (`letta/prompts/prompt_generator.py:24-89`). Other parameterization is static: summarizer prompts interpolate word limits at module import (`letta/prompts/summarizer_prompt.py:4,26`), tool rules hardcode their prompt fragments in `render_prompt()` (`letta/schemas/tool_rule.py:115-117`), and sandbox execution scripts are assembled as Python string lines rather than templates (`letta/services/tool_sandbox/base.py:177-388`).

### 2. Are variable contracts explicit?

Partially. The single reserved variable is documented in docstrings ("The following are reserved variables: - CORE_MEMORY", `letta/prompts/prompt_generator.py:121-127`; `letta/services/helpers/agent_manager_helper.py:268-274`) and enforced nominally via a protected-name check (`prompt_generator.py:135-136`). However, the richer contract surfaces are vestigial: `user_defined_variables` raises `NotImplementedError` (`prompt_generator.py:128-130`), the `template_format` literal accepts `"mustache"` but always raises (`prompt_generator.py:115,173-175`), and `Memory.prompt_template`, `set_prompt_template`, and tool-rule `prompt_template` fields are explicitly deprecated/ignored (`letta/schemas/memory.py:106-120`; `letta/schemas/tool_rule.py:17-20`; `letta/schemas/agent.py:559-561`). There is no schema, registry, or validator enumerating allowed template keys anywhere.

### 3. Is missing-variable behavior predictable?

Yes, and intentionally lenient. Two distinct behaviors exist: (a) unknown placeholders in a *user-supplied* template are preserved verbatim by `PreserveMapping.__missing__` (`letta/prompts/prompt_generator.py:15-19`), verified by `tests/test_utils.py:437-467` where `{USER_MEMORY}` survives rendering untouched; (b) a *missing required* `{CORE_MEMORY}` token is silently appended to the end of the prompt when `append_icm_if_missing=True` (`prompt_generator.py:156-162`), so memory is never dropped — but the operator gets no signal because the warning log is commented out (`prompt_generator.py:161`). This matters in practice: current shipped prompts like `letta_v1` contain no placeholder at all (`letta/prompts/system_prompts/letta_v1.py:1-25`), so every such agent runs through the silent-append path. Render failure is loud only when `format_map` itself throws, wrapped as `ValueError` including the whole prompt (`prompt_generator.py:170-171`).

### 4. Are variables properly escaped?

No general escaping exists for prompt assembly. Block values, descriptions, and labels are written raw into the XML-shaped prompt (`letta/schemas/memory.py:157-170`: `s.write(f"<{label}>")`, `s.write(f"{value}\n")` inside `<value>` tags) with no equivalent of `html.escape` — the latter exists only for HTML response display (`letta/schemas/letta_response.py:103-149`). A block value containing `</memory_blocks>` or a label containing whitespace/angle brackets can corrupt the structural markup the model (and `ContextWindowCalculator`'s section parsing, cf. `tests/test_context_window_calculator.py:73-211` asserting on `<memory_blocks>`/`<memory_metadata>` tags) relies on. The team appears aware and tolerant: `tests/test_letta_agent_v2_skills.py:45-66` deliberately asserts a block value containing literal `<available_skills>` text passes through unescaped. For generated sandbox code, string arguments are `repr()`-escaped (`letta/services/helpers/tool_parser_helper.py:56-58`), pickles/markers use `repr(bytes)` (`letta/services/tool_sandbox/base.py:230,376`), but non-string scalars fall back to raw `str()` with an admitted gap ("TODO increase sanitization checks", `tool_parser_helper.py:50`; array branch commented "need more testing here", :69-74). Message envelopes avoid concatenation issues by using JSON serialization (`letta/system.py:126-147`).

## Architectural Decisions

1. **Single-reserved-variable design instead of a template engine.** Rather than adopt Jinja2/Mustache (despite both appearing in lockfiles/dead files), Letta reduced the runtime template surface to one token, `{CORE_MEMORY}`, and moved all composition into Python code (`Memory.compile`, `letta/schemas/memory.py:688`). The deprecation comments ("Deprecated. Templates are not used anymore; fast renderer handles formatting", `letta/schemas/agent.py:559-561`) show this was a deliberate performance-motivated retreat from earlier template-based designs.
2. **Fail-safe substitution semantics.** Unknown variables preserved verbatim (`PreserveMapping`, `prompt_generator.py:15-19`) and missing required variable appended (`prompt_generator.py:156-162`) prioritize never losing memory context over strict validation.
3. **Dual-path rendering: strict-safe vs trivial-replace.** `safe_format` runs only when user variables exist; otherwise plain `str.replace` is used so arbitrary braces in prompts (e.g., JSON examples inside `voice_sleeptime.PROMPT`, `letta/prompts/system_prompts/voice_sleeptime.py:35-53`) can never break rendering (`prompt_generator.py:164-169`).
4. **Persisted-compiled-prompt model.** The compiled system message is persisted as message[0] and rebuilt only when memory changes, guarded by substring check and united diff (`letta/services/agent_manager.py:1465-1517`), with request-scoped additions (client skills) layered on at call time without persistence (`letta/agents/letta_agent_v2.py:794-810`).
5. **Code generation by string building for sandboxes.** Execution scripts are constructed as explicit line lists (`letta/services/tool_sandbox/base.py:195-388`), keeping full control over serialization protocol (pickle/base64/marker framing) instead of relying on a template engine.

## Notable Patterns

- **Twin implementations:** sync logic lives twice — as `PromptGenerator` static methods and as free functions in `agent_manager_helper` (`letta/prompts/prompt_generator.py:91-103` vs `letta/services/helpers/agent_manager_helper.py:236-247`) — with the async path delegating back to `PromptGenerator` (`agent_manager_helper.py:410`).
- **Template-as-dead-artifact:** `.j2` files retained in-tree (`letta/templates/summary_request_text.j2`) alongside a byte-similar Python reimplementation (`build_summary_request_text`, `letta/services/summarizer/summarizer.py:436-457`); the two have already diverged slightly (e.g., trailing newline handling), a textbook documentation-drift hazard.
- **Self-describing deprecation:** ignored template fields keep their API shape but document their own uselessness in field descriptions (`letta/schemas/tool_rule.py:17-20`, `letta/schemas/memory.py:106`).
- **Import-time interpolation:** config-like constants baked into prompt constants at module load (`ALL_PROMPT = f"""... {ALL_WORD_LIMIT} ..."""`, `letta/prompts/summarizer_prompt.py:4,26`) — simple, but makes limits non-configurable at runtime.
- **Structural-tag prompt format:** memory/rules/files are composed as pseudo-XML sections (`<memory_blocks>`, `<tool_usage_rules>`, `<available_skills>`), which tests treat as parseable structure (`tests/test_context_window_calculator.py:205-211`, `tests/test_letta_agent_v2_skills.py:61-66`) — yet nothing escapes content against that grammar.

## Tradeoffs

- **Simplicity vs extensibility:** one hardcoded variable makes the common case bulletproof but blocks user-defined prompt variables entirely (`NotImplementedError`, `prompt_generator.py:128-130`); the "mustache" option is a dead enum member (`prompt_generator.py:174-175`).
- **Leniency vs observability:** silent-append guarantees memory presence but hides authoring mistakes — the diagnostic log was written then disabled (`prompt_generator.py:161`), so misconfigured prompts fail invisibly.
- **Raw interpolation vs robustness:** writing values unescaped keeps `Memory.compile` fast (explicit design goal per deprecation comments) but leaves prompt-structure injection as an accepted risk, partially mitigated by read_only flags and char limits (`letta/schemas/memory.py:162-166`, `letta/schemas/block.py:20`).
- **Duplication vs decoupling:** copying `safe_format` into `agent_manager_helper` avoids a circular import but creates two sources of truth for escaping semantics.

## Failure Modes / Edge Cases

1. **Unreachable safety checks:** the protected-variable `ValueError` (`prompt_generator.py:135-136`) can never fire because `user_defined_variables != None` already raised at :128-130; similarly `safe_format` (:167) is unreachable from the public entry points since `user_defined_variables` must be falsy to get there (`agent_manager_helper.py:281-285,320-324`).
2. **Silent prompt corruption risk:** a block label like `my block>` produces malformed tags `"<my block>>"` (`memory.py:157,170`); a value containing `</value>` terminates the section early. No validation rejects such labels/values at write time (block create/update paths do not screen for markup tokens).
3. **Double-brace ambiguity:** `safe_format` pre-escapes only empty `{}` (`prompt_generator.py:100`); literal `{{literal}}` sequences in a template passed through `format_map` would be collapsed to `{literal}` — but only in the currently unreachable user-variable path, so latent.
4. **Sandbox arg injection gaps:** non-string scalar args bypass `repr` (`str(raw_value)` fallback, `tool_parser_helper.py:75`), and array/object handling is explicitly unfinished (`tool_parser_helper.py:69-74`); tool source code is pasted verbatim into generated scripts (`base.py:267-268`), trusting uploaded tools by design (they execute anyway).
5. **Stale-template drift:** edits to `summary_request_text.j2` have no effect on behavior since only `build_summary_request_text` runs — a maintainer editing the wrong artifact would see no test failure.
6. **Substring rebuild guard false-positives:** rebuild skip logic uses substring containment of compiled memory in the system message (`agent_manager.py:1467`, `1562`), acknowledged in-code as potentially problematic if blocks are removed ("could this cause issues if a block is removed?", `agent_manager.py:1468`).

## Future Considerations

- Collapse `PromptGenerator` and the `agent_manager_helper` duplicates into one module to restore a single substitution semantic (`letta/prompts/prompt_generator.py:91-103`; `letta/services/helpers/agent_manager_helper.py:229-247`).
- Delete or wire up `letta/templates/*.j2`; if kept as reference, generate the Python function from them or add a CI check comparing output parity with `build_summary_request_text`.
- Re-enable (and route to structured logging/tracing) the missing-`{CORE_MEMORY}` warning at `prompt_generator.py:161`, or make `append_icm_if_missing=False` + missing token a hard error for agent types whose prompts declare the variable.
- Introduce minimal escaping or delimiter-randomization for block labels/values in `_render_memory_blocks_*` (`memory.py:143-203`), or validate labels against a safe charset at `Block` creation (`letta/schemas/block.py`).
- Either implement `user_defined_variables` end-to-end (schema + validation + `safe_format` activation) or remove the parameter and `PreserveMapping` machinery to shrink dead surface (`prompt_generator.py:113,128-130`).
- Finish sanitization TODOs in `convert_param_to_str_value` (`tool_parser_helper.py:50,69-75`) before expanding sandbox argument types.

## Questions / Gaps

- **No evidence found** for any runtime consumer of `letta/templates/` — searched all `.py` files for `jinja`, `.j2`, `load_template`, `read_template`, and template filenames; only hits were the template files themselves and unrelated `tool_source_code` identifiers. If these files are consumed externally (e.g., by another repo or build step), that usage is outside this source tree.
- **No evidence found** for prompt-level validation at agent creation: neither `create_agent` nor `modify_agent` paths were found checking whether a custom `system` string contains `{CORE_MEMORY}` or forbidden markup; verification happened only indirectly via integration tests (`tests/managers/test_agent_manager.py:677-701` asserts custom text appears in the compiled message). Search boundary: `letta/services/agent_manager.py`, REST routers under `letta/server/rest_api/routers/v1/agents.py`, and `tests/managers/`.
- Whether the `"mustache"` enum value is exercised by external SDK clients could not be determined from this source alone; internally every call site uses the default `"f-string"` (grep across `letta/**` shows only definition sites).
- Historical rationale for disabling the missing-variable warning (commented-out logger at `prompt_generator.py:161`) is not documented in-repo.

---

Generated by dimension `12.02-prompt-templating-and-variable-contracts` against `letta`.
