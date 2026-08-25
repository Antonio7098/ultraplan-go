# Source Analysis: agent-framework

## Dimension 12.02: Prompt Templating and Variable Contracts

### Source Info

| Field | Value |
|-------|-------|
| Name | agent-framework |
| Path | `studies/agent-harness-study/sources/agent-framework` |
| Language / Stack | C# (.NET), Python; `go/` contains only a README (`go/README.md`), no Go implementation to study |
| Analyzed | 2026-08-25 |

## Summary

agent-framework does not use a single prompt-template engine. Instead, it ships **at least five distinct templating subsystems**, each with its own placeholder syntax, resolution model, and missing-variable semantics:

1. **.NET Magentic orchestration prompts** — a hand-rolled regex substitution engine (`dotnet/src/Microsoft.Agents.AI.Workflows/Specialized/Magentic/PromptTemplates.cs:12-31`) over public default templates, with a documented, per-prompt placeholder contract (`dotnet/src/Microsoft.Agents.AI.Workflows/MagenticPromptOverrides.cs:11-72`) and dedicated tests.
2. **Python Magentic orchestration prompts** — plain `str.format()` over module-level template constants (`python/packages/orchestrations/agent_framework_orchestrations/_magentic.py:108-255`, applied at `_magentic.py:628,635,646-651,712-716,743`). No validation layer; literal braces must be hand-doubled by the template author.
3. **.NET declarative workflows** — Power Fx (`RecalcEngine`) is the "template engine": templates are parsed into `TemplateLine`/`TemplateSegment` trees (parse provided by the external `Microsoft.Agents.ObjectModel` package) and rendered via `Format` extensions (`dotnet/src/Microsoft.Agents.AI.Workflows.Declarative/Extensions/TemplateExtensions.cs:12-41`), with scope-bound variables in `WorkflowFormulaState` (`dotnet/src/Microsoft.Agents.AI.Workflows.Declarative/PowerFx/WorkflowFormulaState.cs:19-147`).
4. **Python declarative workflows** — a two-pass scheme: `=expr` strings evaluated as Power Fx via the `powerfx` package with a hand-written fallback evaluator, then `{Variable.Path}` interpolation against workflow state (`python/packages/declarative/agent_framework_declarative/_workflows/_declarative_base.py:907-939` and `_executors_basic.py:246-251`), with an allowlisted `Env` symbol for environment access (`_declarative_base.py:70-122`).
5. **Core Python harnesses** — the skills provider validates custom instruction templates at construction with probe-formatting and XML-escapes injected values (`python/packages/core/agent_framework/_skills.py:2295-2360`), while the loop judge substitutes a single documented `{{criteria}}` placeholder via `str.replace` (`python/packages/core/agent_framework/_harness/_loop.py:69,405-407`).

Variable contracts are **explicit and well-documented on the .NET Magentic surface** (XML docs enumerate placeholders per prompt; tests assert their presence) and **partially explicit elsewhere**: the declarative Python interpolator documents its grammar in code, and path-safety rules have a dedicated test module. However, required variables are generally **not validated upfront** — most subsystems resolve missing variables silently (empty string) or leave tokens literal, and only construction-time probes (skills) or eval-time exceptions (.NET declarative) surface problems. Escaping of substituted values into prompts is essentially absent everywhere (by design, since these are LLM prompts), but injection *into state resolution* is guarded (path-segment regex, single-pass substitution, env allowlist). The same conceptual operation ("fill this prompt") behaves differently depending on which subsystem a developer touches.

## Rating

**Score: 6 / 10**

Rationale: Templating is clearly present and in several places well-engineered — the .NET Magentic contract is documented and tested including a no-re-expansion guarantee (`dotnet/tests/Microsoft.Agents.AI.Workflows.UnitTests/PromptTemplatesTests.cs:210-232`); Python declarative interpolation has a dedicated path-safety test suite (`python/packages/declarative/tests/test_declarative_state_path_safety.py:1-24,231-293`); the skills provider validates custom templates eagerly (`_skills.py:2324-2343`). But the overall model is **inconsistent across subsystems**: four different placeholder syntaxes (`{task}`, `{Local.Path}` + `=PowerFx`, `{skills}` str-format, `{{criteria}}`), four different missing-variable behaviors (leave-literal, KeyError, empty-string, exception), and the declarative `Template` schema's `strict` flag exists but is never enforced at runtime (`python/packages/declarative/agent_framework_declarative/_models.py:488-499`, no other non-test references found). This matches "present but inconsistent" (4–6) at its upper bound rather than the 7–8 band, which expects one clear model.

## Evidence Collected

Every entry includes a file path with line numbers. Paths are workspace-relative to the source root `studies/agent-harness-study/sources/agent-framework/`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Template engine (.NET Magentic) | Regex `\{(\w+)\}` single-pass `Substitute`; unmatched tokens left untouched; inserted values never re-scanned | dotnet/src/Microsoft.Agents.AI.Workflows/Specialized/Magentic/PromptTemplates.cs:12-31 |
| Placeholder contract (.NET Magentic) | XML docs enumerate placeholders per prompt; notes divergence from Python `str.format`; `{schema}` marked required for progress-ledger overrides | dotnet/src/Microsoft.Agents.AI.Workflows/MagenticPromptOverrides.cs:11-72 |
| Public defaults (.NET Magentic) | `MagenticDefaultPrompts` exposed so callers can base overrides on them; per-prompt placeholder list in remarks | dotnet/src/Microsoft.Agents.AI.Workflows/MagenticDefaultPrompts.cs:9-30 |
| Tests: substitution & language pin | Default render, override composition, `{schema}` injection, placeholder presence assertions | dotnet/tests/Microsoft.Agents.AI.Workflows.UnitTests/PromptTemplatesTests.cs:64-261 |
| No-re-expansion safety test | Task text containing `{schema}`/`{team}` survives verbatim while real placeholders substitute | dotnet/tests/Microsoft.Agents.AI.Workflows.UnitTests/PromptTemplatesTests.cs:209-232 |
| Template engine (Python Magentic) | Module-level prompt constants with `{task}`/`{team}`/`{facts}`/`{plan}`/`{names}`; JSON braces hand-escaped as `{{ }}` | python/packages/orchestrations/agent_framework_orchestrations/_magentic.py:108-255 |
| Render calls (Python Magentic) | Direct `str.format(task=...)`, `.format(team=...)`, etc.; no validation wrapper | python/packages/orchestrations/agent_framework_orchestrations/_magentic.py:628,635,646-651,665,675,691-696,712-716,743 |
| Override plumbing (Python Magentic) | Constructor accepts optional prompt strings, defaults to built-ins | python/packages/orchestrations/agent_framework_orchestrations/_magentic.py:534-576 |
| Template engine (.NET declarative) | Power Fx `RecalcEngine.Format` over `TemplateLine`/`TextSegment`/`ExpressionSegment`; unsupported segment throws `DeclarativeModelException` | dotnet/src/Microsoft.Agents.AI.Workflows.Declarative/Extensions/TemplateExtensions.cs:12-41 |
| Variable scopes (.NET declarative) | `WorkflowFormulaState` binds Local/System/Global/etc. scopes into the engine; unknown scope name throws | dotnet/src/Microsoft.Agents.AI.Workflows.Declarative/PowerFx/WorkflowFormulaState.cs:38-60,127-141 |
| Missing variable (.NET declarative) | `Get` returns `FormulaValue.NewBlank()`; `BlankValue` string result maps to `string.Empty` | dotnet/src/Microsoft.Agents.AI.Workflows.Declarative/PowerFx/WorkflowFormulaState.cs:49-57; dotnet/src/Microsoft.Agents.AI.Workflows.Declarative/PowerFx/WorkflowExpressionEngine.cs:79-82 |
| Error propagation (.NET declarative) | Power Fx `ErrorValue` → `DeclarativeActionException`; wrong output type → `InvalidExpressionOutputTypeException` | dotnet/src/Microsoft.Agents.AI.Workflows.Declarative/PowerFx/WorkflowExpressionEngine.cs:270-285,89-94 |
| Injection points (.NET declarative) | `SendActivityExecutor` formats `MessageActivityTemplate.Text`; `QuestionExecutor.FormatPrompt`; generic `FormatTemplateAsync` | dotnet/src/Microsoft.Agents.AI.Workflows.Declarative/ObjectModel/SendActivityExecutor.cs:17-49; dotnet/src/Microsoft.Agents.AI.Workflows.Declarative/ObjectModel/QuestionExecutor.cs:159-191; dotnet/src/Microsoft.Agents.AI.Workflows.Declarative/Kit/IWorkflowContextExtensions.cs:54-65 |
| Tests: .NET declarative templating | Text/expression/variable segments, empty/null lines, undefined segment → exception | dotnet/tests/Microsoft.Agents.AI.Workflows.Declarative.UnitTests/PowerFx/TemplateExtensionsTests.cs:13-138 |
| Declarative agent instructions (.NET) | `promptAgent.Instructions?.ToTemplateString()` feeds ChatOptions; model options individually `Eval(engine)`'d | dotnet/src/Microsoft.Agents.AI.Declarative/Extensions/PromptAgentExtensions.cs:22-54 |
| Template schema (Python declarative models) | `Format(kind, strict=False, options)` / `Parser(kind)` / `Template(format, parser)` classes; all string fields pass through `_try_powerfx_eval` | python/packages/declarative/agent_framework_declarative/_models.py:488-527 |
| Load-time expression eval (Python) | `_try_powerfx_eval`: only strings starting with `=` are evaluated; engine unavailable → warning, value returned as-is; eval failure logged, original returned | python/packages/declarative/agent_framework_declarative/_models.py:51-80 |
| Safe-mode env gate (Python load-time) | `safe_mode=True` (default ContextVar) evaluates without symbols; `False` exposes `Env = dict(os.environ)` | python/packages/declarative/agent_framework_declarative/_models.py:37-40,70-74 |
| Runtime expression eval (Python workflows) | `eval()`: `=` prefix → PowerFx with state symbols; failure logs warning and falls back to `_eval_simple` | python/packages/declarative/agent_framework_declarative/_workflows/_state.py:355-386; fallback evaluator `_state.py:388-561` |
| Interpolation (Python workflows) | `{Variable.Path}` regex requiring identifier root; unresolved token → empty string; non-path-like tokens left literal | python/packages/declarative/agent_framework_declarative/_workflows/_declarative_base.py:907-939 |
| Two-pass pipeline (Python workflows) | `SendActivityExecutor`: first `eval_if_expression`, then `interpolate_string` | python/packages/declarative/agent_framework_declarative/_workflows/_executors_basic.py:246-251 |
| Path-safety rules & tests (Python) | Empty segments rejected; attribute segments must match `[A-Za-z][A-Za-z0-9_]*`; dunder traversal blocked; literals like `{foo-bar}`, `{Ctrl+C}` preserved | python/packages/declarative/agent_framework_declarative/_workflows/_declarative_base.py:64-67,337-405; python/packages/declarative/tests/test_declarative_state_path_safety.py:1-24,231-293 |
| Env symbol allowlist (Python workflows) | `DeclarativeEnvConfig`: config values always exposed; `os.environ` only when `restrict_to_configuration=False` AND name appears in scanned `Env.NAME` references | python/packages/declarative/agent_framework_declarative/_workflows/_declarative_base.py:70-134; factory kwarg `_factory.py:100,198` |
| Skills instruction template (Python core) | Custom templates probe-formatted at construction; missing `{skills}` or bad format → `ValueError`; skill names/descriptions XML-escaped before insertion | python/packages/core/agent_framework/_skills.py:2295-2360 |
| Loop judge placeholder (Python core) | `CRITERIA_PLACEHOLDER = "{{criteria}}"` replaced via `str.replace`, removed when no criteria | python/packages/core/agent_framework/_harness/_loop.py:69,86,405-407 |
| External dependency note | `TemplateLine.Parse` and segment types come from NuGet packages `Microsoft.Agents.ObjectModel(.PowerFx)`, not in-repo source | dotnet/src/Microsoft.Agents.AI.Workflows.Declarative/Microsoft.Agents.AI.Workflows.Declarative.csproj:32-34 |

## Answers to Dimension Questions

### 1. How are prompts parameterized?

Three parameterization styles coexist:

- **Named single-brace tokens with a fixed value map** (.NET Magentic): each render method passes an explicit `(token, value)` tuple list to `Substitute` (`PromptTemplates.cs:16-31,54-117`); e.g., `ToProgressLedgerPrompt` supplies `{task}`, `{team}`, `{questions}`, `{schema}` (`PromptTemplates.cs:98-109`).
- **State-backed expressions** (declarative workflows, both languages): .NET renders `MessageActivityTemplate` segment trees through Power Fx (`SendActivityExecutor.cs:21`, `TemplateExtensions.cs:20-41`); Python evaluates `=expr` strings against scoped symbols (`Workflow.Inputs`, `Local`, `System`, `Agent`, `Conversation`, `inputs` alias; `_state.py:327-353`) then interpolates `{Var.Path}` tokens (`_declarative_base.py:907-939`). Injection points include SendActivity, Question prompts, and agent-config fields (`_executors_agents.py:567-584`).
- **Python `str.format` keyword filling** (Python Magentic, skills): `.format(task=..., team=...)` (`_magentic.py:628-651`) and `.format(skills=..., runner_instructions=..., resource_instructions=...)` (`_skills.py:2356-2360`).

### 2. Are variable contracts explicit?

**Partially.** The .NET Magentic surface is exemplary: every override property documents its placeholders, marks `{schema}` as required with the consequence of omission spelled out (`MagenticPromptOverrides.cs:52-66`), defaults are public so overrides can be based on them (`MagenticDefaultPrompts.cs:9-14`), and a test pins that published defaults keep the expected placeholders (`PromptTemplatesTests.cs:250-261`). The skills provider makes its contract executable: a custom template must survive a probe format containing all three keys and reproduce the sentinel, else construction fails (`_skills.py:2324-2343`). The Python declarative interpolator documents its grammar precisely in its docstring (`_declarative_base.py:908-921`). By contrast, Python Magentic prompts have **no declared contract** — placeholders exist implicitly in the template strings and callers overriding them must discover valid keys by reading the call sites; nothing validates an override at configuration time. The Python declarative `Template`/`Format`/`Parser` schema declares `kind` and `strict` fields but they are not consumed anywhere outside the model definition (searched `packages/declarative` excluding tests and `_models.py`; no usages), so the schema promises a contract the runtime does not honor.

### 3. Is missing-variable behavior predictable?

Predictable within a subsystem, but **inconsistent across subsystems**, which is itself a predictability hazard for developers crossing boundaries:

| Subsystem | Missing variable behavior | Evidence |
|-----------|---------------------------|----------|
| .NET Magentic | Token left verbatim in output | PromptTemplates.cs:29-30 |
| Python Magentic | Unhandled `KeyError` from `str.format` propagates at render time | _magentic.py:628,743 (no try/except) |
| Python `{Var.Path}` interpolation | Replaced with empty string; malformed tokens stay literal | _declarative_base.py:927,932-937 |
| Python PowerFx eval | Engine failure → warning log → fallback evaluator returns the formula text as-is if unresolvable | _state.py:376-386,560-561 |
| .NET declarative variables | Blank → `string.Empty` for strings, `default` for numerics/bools | WorkflowExpressionEngine.cs:55-58,79-82,106-111 |
| .NET declarative malformed segment | Throws `DeclarativeModelException`; eval errors throw `DeclarativeActionException` | TemplateExtensions.cs:40; WorkflowExpressionEngine.cs:279-282 |
| Skills custom template | Fails fast at construction with actionable `ValueError` | _skills.py:2326-2342 |

The silent-empty-string behaviors (Python interpolation, .NET blank mapping) mean a typo like `{Local.UserNamme}` produces a prompt with a hole and no diagnostic; there is no warn-on-unresolved-token option in `interpolate_string`.

### 4. Are variables properly escaped?

**Escaping is selective and aimed at the resolver, not the model.** Concrete mechanisms:

- **Re-expansion prevention** (.NET Magentic): single-pass replacement guarantees inserted values containing `{token}` text are never re-substituted (`PromptTemplates.cs:16-18`, tested at `PromptTemplatesTests.cs:209-232`). This prevents content-driven corruption of later placeholders.
- **Path-traversal resistance** (Python declarative): the interpolation pattern requires an identifier root (`_declarative_base.py:932`), `get()` rejects unsafe attribute segments with a warning (`_declarative_base.py:391-399`), and tests prove `X={Local.obj.__class__.__init__.__globals__.os.environ}` yields `X=` rather than leaking environment data (`test_declarative_state_path_safety.py:241-247`).
- **Environment-variable containment**: load-time evaluation hides env vars behind a default-safe ContextVar (`_models.py:37-40,70-74`), runtime `Env` exposure requires explicit opt-in plus a scanned reference allowlist (`_declarative_base.py:95-122,125-134`).
- **XML escaping of injected metadata** (skills): skill names/descriptions are `xml_escape`d before entering the prompt (`_skills.py:2352-2353`).
- **Sandboxed expression engine**: both languages evaluate Power Fx through a controlled engine with typed scopes rather than ad-hoc `eval` (the Python fallback evaluator is hand-written string parsing, `_state.py:388-561`).

What is *not* escaped: substituted **values** (task text, facts, tool results) are inserted verbatim into prompts in every renderer — classic indirect prompt injection into downstream LLM calls is structurally possible and accepted (no sanitization hook exists anywhere in the studied code). The Python Magentic templates require authors to hand-double literal braces (`{{` in the JSON schema block, `_magentic.py:220-242`), whereas the .NET regex approach needs no such escaping — a cross-language footgun the .NET docs explicitly call out (`MagenticPromptOverrides.cs:15-19`).

## Architectural Decisions

1. **Regex substitution over a template library for orchestration prompts (.NET)** — avoids a dependency and enables the "only documented placeholders are replaced, literal braces need no escaping" contract (`PromptTemplates.cs:10-11`, `MagenticPromptOverrides.cs:15-19`). Tradeoff: no expression support; values are plain strings.
2. **Power Fx as the declarative expression/template engine (.NET)** — templates are structured segment trees evaluated against bound scopes (`WorkflowFormulaState.Bind`, `WorkflowFormulaState.cs:96-123`), giving typed evaluation, scoping aliases (component→Local, `WorkflowFormulaState.cs:133-140`), and error surfacing. Cost: parse/format primitives live in external `Microsoft.Agents.ObjectModel` packages (`csproj:32-34`), so template syntax behavior is not fully visible or versionable in-repo.
3. **Dual-layer text processing (Python declarative)** — separate `=expr` evaluation from `{path}` interpolation, executed sequentially (`_executors_basic.py:246-251`). This keeps plain prose untouched (non-`=` strings returned as-is, `_state.py:370-371`) and confines interpolation to identifier-rooted paths.
4. **Fail-open with layered fallbacks (Python)** — missing `powerfx` package or .NET runtime degrades to a simplified evaluator instead of failing (`_state.py:19-26,376-386`), and failed evaluations return the original text (`_models.py:75-80`). Availability wins over strictness; correctness differences between full PowerFx and `_eval_simple` are absorbed silently.
5. **Fail-fast where templates are caller-supplied API surface** — skills validate custom templates at construction with a probe (`_skills.py:2324-2343`); .NET Magentic overrides compose deterministically with language directives appended after the body (`PromptTemplates.cs:36-44`).
6. **Security posture concentrated at the boundary** — env access gated twice (load-time safe mode, runtime allowlist) rather than sanitized inside templates; state-path traversal blocked at the resolver (`get`), not at the template syntax level.

## Notable Patterns

- **Contract-as-documentation + contract-as-test**: .NET Magentic pairs XML-doc placeholder lists (`MagenticPromptOverrides.cs:24-72`) with a theory test asserting the defaults retain those placeholders (`PromptTemplatesTests.cs:250-261`) — documentation cannot drift without breaking CI.
- **Sentinel probe validation**: formatting a candidate template with `__PROBE__` sentinels to verify both syntax validity and required-placeholder presence (`_skills.py:2326-2342`) is a cheap, reusable validation idiom.
- **Single-pass, order-independent substitution** with an explicit regression test for adversarial task text (`PromptTemplatesTests.cs:209-232`).
- **Scope-name aliasing normalization**: component-scoped variables aliased to `Local`, invalid namespaces rejected centrally (`WorkflowFormulaState.cs:127-141`).
- **Graceful degradation ladder**: full PowerFx → custom function set (`CUSTOM_FUNCTIONS`, `_powerfx_functions.py:18-327`) → return-original (`_state.py:376-386,421-431,560-561`).
- **Literal-preservation heuristic**: interpolation tokens that don't match the path grammar (e.g. `{Ctrl+C}`) are deliberately left intact so non-variable braces survive rendering (`_declarative_base.py:908-914`, tested at `test_declarative_state_path_safety.py:251-254`).

## Tradeoffs

- **Flexibility vs consistency**: five subsystems optimize locally (regex speed, PowerFx expressiveness, stdlib-only `str.format`) at the cost of a unified mental model. A developer moving from .NET Magentic overrides to Python Magentic overrides loses brace-freedom and gains `KeyError` risk; moving from Python `{Var.Path}` to .NET declarative switches syntax entirely.
- **Silence vs diagnosability**: empty-string-for-missing keeps workflows running (good for chat UX) but turns typos into invisible prompt degradation. Only the .NET declarative engine surfaces hard errors, and only for structural/type failures — not for absent variables.
- **In-repo simplicity vs external power**: relying on `Microsoft.Agents.ObjectModel` for `TemplateLine.Parse` keeps the repo lean but means the template grammar's authority lives outside the studied tree; behavioral changes can arrive via package bumps.
- **Availability vs equivalence (Python fallback)**: `_eval_simple` approximates PowerFx (e.g., space-delimited operators only, `_state.py:448-509`), so the same YAML can evaluate differently with and without the optional dependency.
- **Strictness flag without enforcement**: `Format.strict` (`_models.py:494`) suggests a planned validation mode; shipping it inert invites false confidence.

## Failure Modes / Edge Cases

- **Typo'd variable in Python interpolation** → silently empty segment in user-visible activity text (`_declarative_base.py:927`); no log emitted for unresolved-but-well-formed tokens (warnings exist only for unsafe attribute segments, `_declarative_base.py:394-398`).
- **Override drift in Python Magentic**: supplying an override template that omits a key used by `.format(...)` (or adds unbraced literals) raises `KeyError`/`ValueError` mid-orchestration, after agents may already have run (`_magentic.py:628-651`).
- **Cross-language brace handling**: a progress-ledger override copied from the Python default (with `{{ }}` escapes) into .NET would render doubled braces literally, and vice versa a .NET-style JSON body pasted into Python breaks `str.format`.
- **Missing `{schema}` in a .NET progress-ledger override** leaves the framework's response-parsing contract broken — documented as required (`MagenticPromptOverrides.cs:58-66`) but not mechanically enforced; the framework still substitutes remaining tokens and proceeds.
- **Fallback-evaluator divergence**: expressions using nested parentheses or non-spaced operators fall through `_eval_simple`'s split-based parsing and return the formula text as the value (`_state.py:424,560-561`), potentially injecting raw `=...` residue into rendered prompts when PowerFx is unavailable.
- **Load-time vs runtime evaluation mismatch**: agent YAML fields evaluated eagerly at load (`_try_powerfx_eval`) have no workflow-state symbols available, so state-dependent expressions in `instructions:` resolve at load time only (`_models.py:51-80`).
- **Undefined expression segment (.NET)**: an `ExpressionSegment` with neither expression nor variable reference aborts rendering with `DeclarativeModelException` mid-workflow (`TemplateExtensions.cs:40`, tested `TemplateExtensionsTests.cs:114-123`).

## Future Considerations

- Adopt one shared placeholder grammar and renderer per language (or a shared spec), keeping subsystem-specific resolvers behind it; this would let the .NET single-pass/no-re-expansion guarantee and the Python path-safety rules apply uniformly.
- Add opt-in diagnostics to silent-resolve paths: warn (or collect) unresolved interpolation tokens in `interpolate_string` and blank PowerFx lookups, analogous to the existing unsafe-segment warning.
- Either enforce `Format.strict` in the Python declarative runtime or remove the field; a probe-based validator like the skills one (`_skills.py:2326-2342`) would fit.
- Validate Python Magentic prompt overrides at builder time against a declared per-prompt placeholder set, mirroring the .NET XML-doc + test pairing.
- Consider escaping/flagging substituted control text (e.g., delimiting injected task/user content) to mitigate indirect prompt injection through ledger/fact values, which currently flow verbatim into manager prompts.
- Document the availability-dependent evaluation differences (`powerfx` present vs `_eval_simple`) in the declarative package docs, since rendered prompts can differ per environment.

## Questions / Gaps

- **Where exactly does `ToTemplateString()` convert agent instructions, and how are per-agent input schemas bound?** The conversion entry point is `dotnet/src/Microsoft.Agents.AI.Declarative/Extensions/PromptAgentExtensions.cs:38`, but `Instructions`/`ToTemplateString`/`TemplateLine.Parse` are defined in the external `Microsoft.Agents.ObjectModel` NuGet packages (`Microsoft.Agents.AI.Workflows.Declarative.csproj:32-34`); their source is not in this repository, so load-time binding semantics for declarative-agent inputs (e.g., whether `inputSchema` values are injected into instruction templates) could not be verified. Searched all in-repo `src/` and `tests/` `.cs` files for `ToTemplateString` — single hit listed above.
- **Is the Python declarative `Template`/`Parser` schema consumed anywhere?** Grep across `packages/declarative` (excluding tests) found definitions only in `_models.py:488-527`; no executor reads `template.format.kind` or enforces `strict`. If intended as forward-compat scaffolding, that intent is undocumented in the studied tree.
- **No evidence found** for any prompt-injection countermeasure applied to substituted *values* (as opposed to resolver-path safety): searched for sanitize/escape/escape-adjacent helpers around all render call sites in `_magentic.py`, `PromptTemplates.cs`, `_declarative_base.py`, and `_skills.py`; only the skills XML-escaping of skill metadata exists.
- **Go implementation**: `go/` contains only `go/README.md`; no templating code exists to assess on that stack.

---

Generated by `Dimension 12.02: Prompt Templating and Variable Contracts` against `agent-framework`.
