# Source Analysis: pydantic-ai

## Dimension 12.02: Prompt Templating and Variable Contracts

### Source Info

| Field | Value |
|-------|-------|
| Name | pydantic-ai |
| Path | `studies/agent-harness-study/sources/pydantic-ai` |
| Language / Stack | Python (pydantic-core, Pydantic v2, optional `pydantic-handlebars`) |
| Analyzed | 2026-08-25 |

> Citation convention: all file paths below are relative to the source root
> `studies/agent-harness-study/sources/pydantic-ai/`.

## Summary

Pydantic AI parameterizes prompts through a dedicated, opt-in templating layer built on
**Handlebars** via the external `pydantic-handlebars` library, wrapped by the public
`TemplateStr` class (`pydantic_ai_slim/pydantic_ai/template.py:16`). A template is a string
containing `{{variable}}` placeholders that is rendered against the run's dependency object
(`RunContext.deps`) at prompt-build time. The distinctive design choice is that **templates are
compiled once and validated against an explicit variable contract at construction time**, not at
render time: when a `deps_type` (Python type) or `deps_schema` (JSON Schema) is supplied,
compilation checks every placeholder name against the contract and fails fast on unknown fields.

Variable contracts flow through two mechanisms:

1. **Python constructor path**: users pass explicit `TemplateStr(...)` instances (optionally with
   `deps_type=`) or — preferred per docs — plain callables taking `RunContext`
   (`docs/agent-spec.md:83`, `pydantic_ai_slim/pydantic_ai/template.py:29-45`). Strings passed to
   `Agent(instructions=...)` are stored verbatim without compilation
   (`pydantic_ai_slim/pydantic_ai/agent/__init__.py:651`).
2. **Spec path** (`Agent.from_spec` / YAML): strings containing the `{{` sentinel are
   automatically compiled into `TemplateStr` during Pydantic validation, using a validation
   context carrying `deps_type`/`deps_schema`
   (`pydantic_ai_slim/pydantic_ai/template.py:100-108`,
   `pydantic_ai_slim/pydantic_ai/agent/__init__.py:4097-4105`).

At runtime, `TemplateStr` is callable with a `RunContext`
(`pydantic_ai_slim/pydantic_ai/template.py:85-87`), so the agent machinery treats it identically
to any dynamic instruction function via `SystemPromptRunner`
(`pydantic_ai_slim/pydantic_ai/agent/__init__.py:3013-3020`,
`pydantic_ai_slim/pydantic_ai/_instructions.py:46-55`). A second, unrelated mini-template
mechanism exists for prompted structured output: `{schema}` placeholders substituted with
`str.format` (`pydantic_ai_slim/pydantic_ai/_output.py:708-720`).

## Rating

**7 / 10** — Clear model with tests, explicit interfaces, and operational safeguards.
The variable-contract idea is genuinely strong: templates fail at *construction* time against a
typed or schema-based contract (`tests/test_template.py:73-80`), contracts are threaded through
Pydantic's validation context so spec-loaded agents get the same guarantees as Python-built ones
(`pydantic_ai_slim/pydantic_ai/_template.py:14-54`), and there is full serialization round-trip
support plus observability integration (`tests/test_template.py:114-118`,
`tests/test_logfire.py:3943-3971`). It falls short of 8–9 because: (a) render-time
missing-variable behavior is delegated entirely to the external handlebars library with no
in-repo handling, error typing, or documentation; (b) two divergent template syntaxes coexist
(Handlebars `{{var}}` vs `str.format` `{schema}`) with different failure semantics; (c)
auto-compilation of `{{` strings happens only on the spec path, creating an asymmetry where the
same literal behaves differently in YAML vs Python; (d) no escaping/injection-prevention
utilities exist anywhere in the repo.

## Evidence Collected

Every entry cites paths relative to `studies/agent-harness-study/sources/pydantic-ai/`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Template engine | `TemplateStr` wraps `pydantic-handlebars`; lazily imported with install hint pointing at the `[spec]` extra | `pydantic_ai_slim/pydantic_ai/template.py:125-136` |
| Engine pinning | `pydantic-handlebars >=0.1.0` locked as optional `[spec]` extra dependency | `uv.lock:5874-5883`, `pyproject.toml:182` |
| Typed compile + validation | `hbs.compile(source, deps_type)` validates placeholders against the deps type at construction | `pydantic_ai_slim/pydantic_ai/template.py:63-65` |
| Schema-based validation | `hbs.check_template_compatibility(source, deps_schema, raise_on_error=True)` when only JSON Schema available | `pydantic_ai_slim/pydantic_ai/template.py:67-68` |
| Render API | `render(deps)` renders typed compile directly; untyped path dumps deps to dict first | `pydantic_ai_slim/pydantic_ai/template.py:72-83` |
| RunContext integration | `__call__(ctx)` renders against `ctx.deps`, making `TemplateStr` interchangeable with prompt functions | `pydantic_ai_slim/pydantic_ai/template.py:85-87` |
| Auto-detection sentinel | Pydantic validator compiles any string containing `{{`; otherwise raises to fall through to plain `str` branch | `pydantic_ai_slim/pydantic_ai/template.py:100-102` |
| Contract propagation (spec) | `_validate_spec` builds `template_context={'deps_type':…}` then injects `deps_schema` after validation | `pydantic_ai_slim/pydantic_ai/agent/__init__.py:4097-4105` |
| Contract consumption | Validator reads `deps_type`/`deps_schema` from `info.context` | `pydantic_ai_slim/pydantic_ai/template.py:104-108` |
| Capability arg validation | `validate_from_spec_args` TypeAdapter-validates `TemplateStr`-hinted `from_spec` params with the shared context | `pydantic_ai_slim/pydantic_ai/_template.py:14-54` |
| Variable schema types | `TemplateStr[AgentDepsT]` generic; `AgentInstructions` union; `AgentSpec.description`/`.instructions` fields typed `TemplateStr[Any] \| str` | `pydantic_ai_slim/pydantic_ai/_instructions.py:12-18`, `pydantic_ai_slim/pydantic_ai/agent/spec.py:40-42` |
| Injection point: instructions | Constructor normalizes without compiling (explicit `TemplateStr` needed); spec instructions arrive pre-compiled | `pydantic_ai_slim/pydantic_ai/agent/__init__.py:651`, `pydantic_ai_slim/pydantic_ai/agent/__init__.py:1943` |
| Injection point: description | Raw source exposed via property; rendered per-run via `render_description(deps)` | `pydantic_ai_slim/pydantic_ai/agent/__init__.py:1090-1112` |
| Injection point: capability args | Capability `from_spec` hints inspected for nested `TemplateStr` | `pydantic_ai_slim/pydantic_ai/_template.py:57-63` |
| Runtime resolution | Instructions split into literals vs `SystemPromptRunner`s; `TemplateStr` handled in callable branch | `pydantic_ai_slim/pydantic_ai/agent/__init__.py:3013-3023` |
| Dynamic system prompts | `SystemPromptRunner.run` inspects signature; dynamic parts carry `dynamic_ref` for re-evaluation across turns | `pydantic_ai_slim/pydantic_ai/_system_prompt.py:21-37,51-55`, `pydantic_ai_slim/pydantic_ai/_agent_graph.py:707-732` |
| Prompted-output template | `{schema}` placeholder replaced via `str.format(schema=json.dumps(schema))`; missing `{schema}` gets schema appended | `pydantic_ai_slim/pydantic_ai/_output.py:708-720` |
| Default prompted-output template | Provider-profile default with `{schema}` placeholder | `pydantic_ai_slim/pydantic_ai/profiles/__init__.py:32-41,225` |
| Template override precedence | Per-request `NativeOutput(template=…)` overrides profile default; `template=False` disables prompt | `pydantic_ai_slim/pydantic_ai/output.py:185-205,257-275`, `pydantic_ai_slim/pydantic_ai/models/__init__.py:640-653` |
| Negative test: unknown variable | `TemplateStr('Hello {{nonexistent}}', deps_type=MyDeps)` raises at construction (both typed and schema modes) | `tests/test_template.py:73-80` |
| Positive tests: rendering | Typed, schema, dict, and no-context renders all covered | `tests/test_template.py:52-71,312-332` |
| Integration test | Full agent run renders `'You are helping {{name}}, age {{age}}.'` into request instructions | `tests/test_template.py:240-249,301-309` |
| Spec-path tests | Mixed template/plain instruction lists; plain strings stay `str` | `tests/test_template.py:255-282` |
| Observability test | Rendered `TemplateStr` description lands in OTel span attribute `gen_ai.agent.description` | `tests/test_logfire.py:3943-3971` |
| Description render call site | Instrumentation capability calls `ctx.agent.render_description(ctx.deps)` per run | `pydantic_ai_slim/pydantic_ai/capabilities/instrumentation.py:178-181` |
| Prompted-output template tests | Custom template, `template=False`, profile-default fallback | `tests/test_agent.py:2850-2921`, `tests/test_native_output_schema.py:23-138` |
| Docs tied to implementation | Template strings documented with construction-time validation guarantee | `docs/agent-spec.md:69-105,141-143`, `docs/dependencies.md:103` |
| Escaping utilities | Grep across package for escape/markupsafe/html-escape found no template-escaping utilities (only OTel exception escaping and unrelated matches) | search boundary noted below |

## Answers to Dimension Questions

**1. How are prompts parameterized?**
Three mechanisms. Primary: Handlebars `{{variable}}` templates rendered against `RunContext.deps`
via `TemplateStr` (`pydantic_ai_slim/pydantic_ai/template.py:16-87`), usable for `instructions`,
agent `description`, and capability `from_spec` arguments. Secondary: plain Python callables
receiving `RunContext` wrapped by `SystemPromptRunner`
(`pydantic_ai_slim/pydantic_ai/_system_prompt.py:14-37`) — the docs explicitly prefer this in
Python code for IDE/type safety (`docs/agent-spec.md:83`). Tertiary: `{schema}` string-format
templates for prompted structured output, resolved from the model profile or overridden per
output declaration (`pydantic_ai_slim/pydantic_ai/_output.py:708-720`,
`pydantic_ai_slim/pydantic_ai/models/__init__.py:640-653`). Bedrock additionally passes provider
prompt variables straight through (`pydantic_ai_slim/pydantic_ai/models/bedrock.py:506-510`).

**2. Are variable contracts explicit?**
Yes, unusually so. Contracts come in two forms: a Python `deps_type` (validated structurally by
the handlebars compiler, `pydantic_ai_slim/pydantic_ai/template.py:63-65`) or a `deps_schema`
JSON Schema for spec-only usage where no Python type exists
(`docs/agent-spec.md:141-143`, `pydantic_ai_slim/pydantic_ai/template.py:67-68`). The generic
parameter `TemplateStr[AgentDepsT]` ties each template to its deps type statically
(`pydantic_ai_slim/pydantic_ai/_instructions.py:12-18`), and the agent threads `deps_type` into
Pydantic's validation context so spec-loaded strings are checked against the same contract
(`pydantic_ai_slim/pydantic_ai/agent/__init__.py:4097-4105`). Caveat: the contract is opt-in —
a `TemplateStr` built with neither `deps_type` nor `deps_schema` compiles unvalidated
(`tests/test_template.py:48-50,120-124`).

**3. Is missing-variable behavior predictable?**
Split across three regimes. (a) Unknown *names* with a contract: fail-fast `ValueError`-style
error at construction, verified by tests (`tests/test_template.py:73-80`). (b) No contract:
compile succeeds; render-time behavior is whatever `pydantic_handlebars` does — the repo adds no
handling, wrapper exception, or documentation, so predictability depends on an external library
not studied here. (c) Deps shape mismatches: if deps dump to a non-dict, render silently falls
back to a context-free render rather than erroring
(`pydantic_ai_slim/pydantic_ai/template.py:78-83`, `tests/test_template.py:327-332`) —
deliberate but quiet degradation. For `{schema}` templates, a custom template containing other
brace pairs would make `str.format` raise `KeyError`/`IndexError`
(`pydantic_ai_slim/pydantic_ai/_output.py:720`) — predictable but crash-only.

**4. Are variables properly escaped?**
No evidence found of in-repo escaping utilities for template variables. Searches for
escape/markupsafe/html-escape patterns across the package surfaced only unrelated hits (OTel
exception escaping at `pydantic_ai_slim/pydantic_ai/capabilities/instrumentation.py:364`,
surrogate-escape JSON notes at `pydantic_ai_slim/pydantic_ai/_instrumentation.py:236-238`).
Any HTML-escaping semantics live inside `pydantic-handlebars` and are not asserted by any repo
test. Since rendered variables are developer-supplied deps interpolated into instructions sent
to an LLM, unescaped interpolation is a prompt-injection surface by design; the framework offers
no guardrail, sanitizer, or even a documented caveat. The `ReinjectSystemPrompt` capability does
address *untrusted history* overwriting system prompts
(`pydantic_ai_slim/pydantic_ai/capabilities/reinject_system_prompt.py:19-38`), showing awareness
of trust boundaries adjacent to templating, but nothing equivalent exists for variable content.

## Architectural Decisions

- **Compile-once, validate-early.** Templates are compiled in `__init__`, not per render
  (`pydantic_ai_slim/pydantic_ai/template.py:57-70`), shifting all name-resolution failures to
  construction time. This trades startup cost and rigidity (deps structure must be known upfront)
  for determinism.
- **Reuse Pydantic as the validation bus.** Rather than a bespoke parser, template conversion is
  a Pydantic core-schema hook (`pydantic_ai_slim/pydantic_ai/template.py:89-116`) fed by
  validation context. This lets declarative specs, capability `from_spec` kwargs, and nested
  structures (lists of instructions) get identical treatment with one mechanism
  (`pydantic_ai_slim/pydantic_ai/_template.py:43-54`).
- **Templates as callables.** Implementing `__call__(ctx)` lets the existing dynamic-prompt
  pipeline (`SystemPromptRunner`, signature introspection) consume templates with zero special
  cases downstream (`pydantic_ai_slim/pydantic_ai/agent/__init__.py:3013-3020`,
  `pydantic_ai_slim/pydantic_ai/_instructions.py:47-55`).
- **Optional engine behind lazy import.** The handlebars engine is an optional `[spec]` extra;
  missing installs produce a targeted `ImportError` with remediation text
  (`pydantic_ai_slim/pydantic_ai/template.py:131-136`, `docs/install.md:83`), keeping the slim
  distribution lean at the cost of a feature that silently doesn't exist until needed.
- **Dual contract backends.** `deps_type` for Python-first users, `deps_schema` JSON Schema for
  YAML/JSON spec files with no importable type
  (`pydantic_ai_slim/pydantic_ai/agent/spec.py:42`, `docs/agent-spec.md:143`).

## Notable Patterns

- **Sentinel-driven coercion**: `'{{' not in value` gates whether a string becomes a template,
  deliberately raising inside the union validator so Pydantic falls through to the plain-`str`
  branch (`pydantic_ai_slim/pydantic_ai/template.py:100-102`, commented as intentional). This
  makes mixed lists like `['Hello {{name}}', 'Be helpful']` coerce per-item
  (`tests/test_template.py:272-282`).
- **Serialization transparency**: the serializer emits the raw template source
  (`pydantic_ai_slim/pydantic_ai/template.py:112-115`), so specs round-trip as plain YAML while
  re-validation recompiles (`tests/test_template.py:114-118`).
- **Raw-source vs rendered duality**: `agent.description` returns the raw source for static
  metadata, while `render_description(deps)` produces the per-run rendered value used in OTel
  spans (`pydantic_ai_slim/pydantic_ai/agent/__init__.py:1090-1112`,
  `pydantic_ai_slim/pydantic_ai/capabilities/instrumentation.py:178-181`).
- **Hint-reflection helper**: `_hint_contains_template_str` recursively scans unions so only
  parameters actually declared `TemplateStr | str` pay validation cost
  (`pydantic_ai_slim/pydantic_ai/_template.py:34-35,57-63`).

## Tradeoffs

- **Fail-fast vs flexibility**: construction-time validation catches typos early but forbids
  dynamically-shaped deps (e.g., free-form dicts with arbitrary keys) unless users skip the
  contract — which then forfeits all checking. There is no middle ground such as "warn on
  unknown key".
- **Spec path magic vs constructor explicitness**: YAML authors get automatic compilation of
  `{{...}}` strings; Python authors must wrap in `TemplateStr(...)` themselves
  (`docs/agent-spec.md:75-83`). The same literal string therefore has different meanings per
  entry point, and a user migrating code between them can silently change behavior.
- **Two template syntaxes**: `{{var}}` (Handlebars, deps-bound, validated) vs `{schema}`
  (`str.format`, schema-bound, unvalidated) invite confusion; a stray `{literal}` in a
  prompted-output template raises `KeyError` at run preparation
  (`pydantic_ai_slim/pydantic_ai/_output.py:717-720`).
- **Optional dependency**: avoiding a hard dep keeps installs slim but means template support
  availability varies by environment, and tests must `importorskip`
  (`tests/test_template.py:11`, `tests/test_logfire.py:3948`).

## Failure Modes / Edge Cases

- **Unknown variable, no contract**: compiles fine; failure surfaces only at render time with
  whatever error `pydantic_handlebars` raises — no wrapping, retry, or fallback policy in-repo.
- **Non-dict deps**: `render('some string deps')` quietly ignores the deps and renders without
  context instead of erroring (`tests/test_template.py:327-332`).
- **Literal braces in spec strings**: any YAML instruction containing `{{` (e.g., JSON examples
  embedded in prose) will be compiled as Handlebars and may fail construction or alter output —
  there is no escape syntax documented in-repo for emitting literal `{{`.
- **Custom `{schema}` templates with other placeholders**: `str.format` interprets additional
  brace groups as fields → `KeyError`/`IndexError` during request-parameter preparation
  (`pydantic_ai_slim/pydantic_ai/_output.py:720`); conversely, omitting `{schema}` silently
  appends the schema (`_output.py:717-718`).
- **Unvalidated contract drift**: `deps_schema` validation is name-level only; docs state it
  does not validate actual runtime deps objects (`docs/agent-spec.md:143`), so a deps instance
  missing a declared field still reaches render unchecked.
- **Dynamic re-render staleness**: system prompts marked dynamic are re-evaluated per turn via
  `dynamic_ref` lookup keyed by function qualname
  (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:707-732`); durable-execution adapters
  explicitly zero this mapping (`pydantic_ai_slim/pydantic_ai/durable_exec/temporal/_agent.py:1097`),
  meaning dynamic prompts do not survive workflow replay boundaries.

## Future Considerations

- Wrap handlebars render errors in a dedicated exception type (e.g., a `TemplateRenderError`)
  and document missing-key semantics so callers can distinguish template failures from tool/model
  failures.
- Provide an escaping utility or documented convention for injecting untrusted strings into
  instructions (analogous to `format_as_xml`'s structured serialization at
  `pydantic_ai_slim/pydantic_ai/format_prompt.py:20-77`, which already exists as a safer
  alternative for data-bearing prompt fragments).
- Unify or cross-document the `{schema}` and `{{var}}` syntaxes, including how to emit literal
  braces in both.
- Extend `deps_schema` compatibility checks to cover value types (currently name-presence only,
  per `docs/agent-spec.md:143`).
- Add negative integration tests proving render-time (post-construction) failure behavior for
  untyped templates, pinning whatever the external library does before it changes underneath.

## Questions / Gaps

- **Render-time missing-key behavior**: not answerable from this source alone; `pydantic-handlebars`
  is external and no repo test exercises a missing key at render time for an untyped template
  (searched `tests/test_template.py`, `tests/test_agent.py`, package-wide for missing/render
  error handling). Stated here as "No evidence found" within the isolation boundary.
- **Escaping semantics of `pydantic_handlebars.render`** (HTML auto-escaping on/off): no evidence
  in-repo; would require reading the dependency's source, outside the selected source directory.
- Whether `TemplateStr` is intended for user-prompt (as opposed to instructions/description)
  templating: no injection point for templated user prompts was found; `run(prompt=…)` accepts
  plain content only.

---

Generated by dimension `12.02-prompt-templating-and-variable-contracts` against `pydantic-ai`.
