# Source Analysis: langfuse

## Dimension 12.02 — Prompt Templating and Variable Contracts

### Source Info

| Field | Value |
|-------|-------|
| Name | langfuse |
| Path | `studies/agent-harness-study/sources/langfuse` |
| Language / Stack | TypeScript monorepo: Next.js (`web`), BullMQ worker (`worker`), shared package (`packages/shared`); Zod, Prisma, CodeMirror |
| Analyzed | 2026-08-25 |

> Citation convention: all `file:line` paths below are relative to the source root `studies/agent-harness-study/sources/langfuse/`.

## Summary

Langfuse is a prompt *management* platform rather than an agent harness with a single prompt pipeline, so templating is deliberately distributed across surfaces. There is no third-party template engine (no mustache/handlebars/jinja dependency). Instead the repo implements a small, custom mustache-style substitution layer:

1. **Shared primitives** in `packages/shared/src/utils/stringChecks.ts`: canonical regexes for variable detection (`MUSTACHE_REGEX`, multiline/unclosed detectors) plus `extractVariables` and `stringifyValue`.
2. **A lenient string compiler** `compileTemplateString` (`packages/shared/src/utils/prompts.ts:14-28`) used by LLM-as-a-judge evals and dataset experiments.
3. **A chat-message compiler** `compileChatMessages` / `replaceTextVariables` (`packages/shared/src/server/llm/compileChatMessages.ts:21-91`) used by the playground and experiment runs, which also expands whole-message placeholders.
4. **A separate composition system** based on `@@@langfusePrompt:name=…@@@` dependency tags resolved by `PromptService.buildAndResolvePromptGraph` (`packages/shared/src/server/services/PromptService/index.ts:242-403`) with cycle and depth guards.

Variable contracts are explicit at the edges that matter operationally: editor-time lint diagnostics (`web/src/components/editor/CodeMirrorEditor.tsx:73-115`), server-side assertion that evaluator variables exactly match prompt variables and mappings are complete (`web/src/features/evals/v2/server/evaluators/evaluatorValidation.ts:19-84`), and fail-fast checks in the playground (`web/src/features/playground/page/context/index.tsx:958-980`). Missing-variable behavior is *predictable but inconsistent by design*: the shared compiler silently preserves unfilled placeholders, while playground and placeholder expansion throw user-facing errors, and the eval worker degrades unmapped variables to empty strings.

## Rating

**6 / 10** — Present, tested, and observable, but inconsistent across surfaces. The core compiler has a thorough test matrix (`worker/src/__tests__/evalService.test.ts:136-209`, `packages/shared/src/utils/prompts.test.ts:4-32`), editor diagnostics catch malformed syntax live, and evaluator v2 enforces explicit variable contracts server-side. It falls short of 7–8 because two substitution implementations coexist with different semantics (silent preserve vs. throw; `String()` vs. JSON serialization of objects), there is no output escaping of interpolated values, one exported function constructs a `RegExp` from unescaped variable names (`packages/shared/src/server/llm/compileChatMessages.ts:28`), and the public prompt-create API validates structure but not variable syntax (`packages/shared/src/features/prompts/types.ts:28-61`, consumed at `web/src/features/prompts/server/handlers/promptsHandler.ts:48`).

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Template engine | Custom regex-based mustache-style engine; no external template library found | `packages/shared/src/utils/stringChecks.ts:10-12` |
| Variable name contract | `VARIABLE_REGEX = /^\p{L}[\p{L}\p{N}_]*$/u`; unicode letter start, letters/digits/underscore | `packages/shared/src/utils/stringChecks.ts:7-8` |
| Placeholder extraction | `MUSTACHE_REGEX = /{{([^{}]*)}}/g` with comment documenting `{{{name}}}` → `{value}` literal behavior | `packages/shared/src/utils/stringChecks.ts:10-12` |
| Malformed-syntax detectors | `MULTILINE_VARIABLE_REGEX`, `UNCLOSED_VARIABLE_REGEX` | `packages/shared/src/utils/stringChecks.ts:14-18` |
| Variable extraction API | `extractVariables` filters matches through `isValidVariableName`, dedupes | `packages/shared/src/utils/stringChecks.ts:20-30` |
| Value serialization | `stringifyValue`: strings raw, numbers/booleans `toString()`, default `JSON.stringify` | `packages/shared/src/utils/stringChecks.ts:32-43` |
| String compiler (lenient) | `compileTemplateString`: missing key → placeholder preserved; null/undefined → `""`; try/catch returns template on error | `packages/shared/src/utils/prompts.ts:14-28` |
| Eval prompt compile | `compileEvalPrompt` maps `{var,value}` pairs through `parseUnknownToString` before substitution | `packages/shared/src/utils/prompts.ts:30-43` |
| Object→string serializer | `parseUnknownToString`: null/undefined → `""`, objects → JSON.stringify | `packages/shared/src/features/evals/utilities.ts:8-27` |
| Chat message compiler | `replaceTextVariables` builds per-variable `RegExp(`{{\\s*${varName}\\s*}}`,`g")` from unescaped names | `packages/shared/src/server/llm/compileChatMessages.ts:21-32` |
| Placeholder expansion | `expandPlaceholder` throws on missing value, non-array values, or non-object entries | `packages/shared/src/server/llm/compileChatMessages.ts:34-63` |
| Compile entry points | `compileChatMessages` / `compileChatMessagesWithIds` exported from shared barrel | `packages/shared/src/index.ts:99` |
| Placeholder name schema | `PlaceholderMessageSchema.name` regex `/^[a-zA-Z][a-zA-Z0-9_]*$/` with error message | `packages/shared/src/server/llm/types.ts:196-204` |
| Prompt content schema | Chat prompts accept `{role,content}` or validated placeholder messages | `packages/shared/src/server/llm/types.ts:235-242` |
| Create-prompt validation | `CreatePromptSchema` union (text/chat) — validates name/labels/config, not variable syntax inside content | `packages/shared/src/features/prompts/types.ts:28-61` |
| Public API consumption | `CreatePromptSchema.parse(req.body)` in v2 prompts POST handler | `web/src/features/prompts/server/handlers/promptsHandler.ts:48` |
| Variable/placeholder collision check | Server rejects prompts where a variable name equals a placeholder name | `web/src/features/prompts/server/actions/createPrompt.ts:117-127` |
| Editor linting | `getPromptVariableDiagnostics`: errors for multiline, unclosed, empty, invalid-name variables and malformed dependency tags | `web/src/components/editor/CodeMirrorEditor.tsx:73-115` |
| Editor highlighting | Custom CodeMirror language marks valid vars as `variable`, invalid as `error`; `{{{` treated literally | `web/src/components/editor/CodeMirrorEditor.tsx:41-71` |
| Playground variable sync | Derives variable list from message contents via `extractVariables`, tracks `isUsed` flags | `web/src/features/playground/page/context/index.tsx:238-269` |
| Playground fail-fast #1 | Missing used variables block submit with named list: "Please set a value for the following variables: …" | `web/src/features/playground/page/context/index.tsx:958-965` |
| Playground fail-fast #2 | Post-compile leftover-variable scan throws "Error replacing variables. Please check your inputs." | `web/src/features/playground/page/context/index.tsx:368-380` |
| Evals v2 variable contract | `assertEvaluatorVariablesMatchPrompt` — declared vars must equal extracted prompt vars | `web/src/features/evals/v2/server/evaluators/evaluatorValidation.ts:19-32` |
| Evals v2 mapping completeness | `assertCompleteEvaluatorVariableMapping` rejects duplicates, unknown mappings, missing mappings | `web/src/features/evals/v2/server/evaluators/evaluatorValidation.ts:34-84` |
| Mapping schema | `variableMapping` zod: templateVariable, langfuseObject, selectedColumnId, jsonSelector; objectName required for observations | `packages/shared/src/features/evals/types.ts:108-129` |
| Legacy template form constraint | Eval template prompt variables restricted to `[A-Za-z_]+` via refine on `extractVariables` output | `web/src/features/evals/utils/template-form-schema.ts:22-36` |
| Eval worker degradation | Unmapped variable / unknown column / missing dataset item → `{var, value: ""}` with logs | `worker/src/features/evaluation/evalService.ts:1510-1541` |
| Eval worker hard failure | Deleted dataset item throws with user-facing remediation message | `worker/src/features/evaluation/evalService.ts:1557-1566` |
| Eval compile call site | `executeLlmEvaluator` → `compileEvalPrompt(templatePrompt, variables)` | `packages/shared/src/server/evals/llmEvaluatorExecution.ts:21-39` |
| Preview gating | `buildInterpolatedPromptPreview` returns "unavailable" with actionable message until every variable is mapped | `web/src/features/evals/v2/fns/promptEditor/buildInterpolatedPromptPreview.ts:22-47` |
| Preview rendering | Unmapped-but-valid variables render as empty strings in fragments | `web/src/features/evals/v2/fns/promptEditor/buildInterpolatedPromptPreview.ts:63-67` |
| Experiment compilation | `replaceVariablesInPrompt` filters itemInput to declared variables, compiles only if `{{` present | `worker/src/features/experiments/utils.ts:76-99` |
| Experiment variable discovery | Worker extracts variables from text or JSON-stringified chat prompts + placeholder names | `worker/src/features/experiments/utils.ts:236-248` |
| Composition tag grammar | `PromptDependencyRegex = /@@@langfusePrompt:(.*?)@@@/g`, zod-validated version/label variants; malformed tags skipped silently | `packages/shared/src/features/prompts/parsePromptDependencyTags.ts:3-61` |
| Composition resolution safety | `escapeRegex` applied to names/versions/labels in resolution patterns | `packages/shared/src/server/services/PromptService/utils.ts:1-3`, `index.ts:365-373` |
| Replacement-string escaping | `$` doubled (`$$$$`) to avoid `$`-pattern semantics in `String.replace` | `packages/shared/src/server/services/PromptService/index.ts:375-377` |
| Cycle & depth guards | `MAX_PROMPT_NESTING_DEPTH = 5`; circular dependency and same-name cross-version references rejected | `packages/shared/src/server/services/PromptService/index.ts:19,265-281` |
| Missing dependency behavior | Resolution throws `LangfuseConflictError("Prompt dependency not found: …")` | `packages/shared/src/server/services/PromptService/index.ts:341-348` |
| Compiler tests (shared) | Missing placeholders preserved; object → `[object Object]`; structured var → JSON | `packages/shared/src/utils/prompts.test.ts:4-32` |
| Compiler tests (worker) | Matrix covering basic/single/no/empty-context/missing/null/undefined/numeric cases | `worker/src/__tests__/evalService.test.ts:136-209` |
| UI variable chips | `PromptVariableListPreview` surfaces available variables under the editor | `web/src/features/prompts/components/PromptVariableListPreview.tsx:3-26` |
| Reference parsing UI | Combined dependency-tag + mustache regex for prompt reference links | `web/src/components/ui/PromptReferences.tsx:234` |

## Answers to Dimension Questions

### 1. How are prompts parameterized?

Two orthogonal mechanisms. First, inline mustache-style variables `{{name}}` embedded in text or per-message content, detected by `MUSTACHE_REGEX` (`packages/shared/src/utils/stringChecks.ts:12`) and substituted either by the lenient string compiler (`packages/shared/src/utils/prompts.ts:19-24`) or by per-variable regex replacement over chat messages (`packages/shared/src/server/llm/compileChatMessages.ts:26-31`). Second, whole-message placeholders — special messages typed `PlaceholderMessage` whose entire slot expands to an array of messages supplied at run time (`packages/shared/src/server/llm/compileChatMessages.ts:34-63`). A third, composition-level mechanism embeds other stored prompts verbatim via `@@@langfusePrompt:name=…|version|label@@@` tags (`packages/shared/src/features/prompts/parsePromptDependencyTags.ts:3`), resolved recursively server-side (`packages/shared/src/server/services/PromptService/index.ts:358-380`). Note the server stores templates verbatim; SDK-side f-string/Jinja compilation lives outside this repo (searched all `.ts/.tsx/.py` for `f-string|fstring|f_string` — no evidence found).

### 2. Are variable contracts explicit?

Partially, and strongest where execution money is at stake. The lexical contract is explicit and shared: `VARIABLE_REGEX` (`packages/shared/src/utils/stringChecks.ts:8`), a stricter ASCII rule for placeholders (`packages/shared/src/server/llm/types.ts:198-203`), and a legacy `[A-Za-z_]+` restriction for eval templates (`web/src/features/evals/utils/template-form-schema.ts:27-36`). Relational contracts are enforced server-side for evaluators v2: declared vars must exactly match prompt-extracted vars, and mappings must be complete without duplicates or unknowns (`web/src/features/evals/v2/server/evaluators/evaluatorValidation.ts:19-84`), backed by the mapping schema at `packages/shared/src/features/evals/types.ts:108-129`. Prompt creation additionally forbids variable/placeholder name collisions (`web/src/features/prompts/server/actions/createPrompt.ts:117-127`). However, there is no unified "prompt schema" object declaring required variables with types — contracts are recomputed ad hoc by re-scanning content at each surface (playground: `web/src/features/playground/page/context/index.tsx:242`; experiments worker: `worker/src/features/experiments/utils.ts:237-241`; public-API adapter derives `vars` from prompt: `web/src/features/evals/server/unstable-public-api/adapters.ts:125-128`), and the public create endpoint does not validate variable syntax at all.

### 3. Is missing-variable behavior predictable?

Yes within each surface, but the semantics differ by surface, which is the main predictability hazard:

- Shared compiler: **preserve** — unmatched keys return the original placeholder text (`packages/shared/src/utils/prompts.ts:20`), pinned by tests (`worker/src/__tests__/evalService.test.ts:162-188`).
- Playground: **fail fast pre-flight** — submit blocked naming the unset variables (`web/src/features/playground/page/context/index.tsx:958-965`), plus a post-compile leftover scan (`context/index.tsx:368-380`).
- Message placeholders: **throw** — expansion errors name the offending placeholder (`packages/shared/src/server/llm/compileChatMessages.ts:40-50`).
- Eval worker: **degrade to empty string** with debug/error logs (`worker/src/features/evaluation/evalService.ts:1516-1541`), except deleted dataset items which throw with remediation guidance (`evalService.ts:1557-1566`).
- Evals v2 preview: **block preview** until mappings resolve, with actionable copy (`web/src/features/evals/v2/fns/promptEditor/buildInterpolatedPromptPreview.ts:22-47`).
- Experiments: keys absent from the dataset item input simply remain unsubstituted via the preserve semantics (`worker/src/features/experiments/utils.ts:86-99`).

### 4. Are variables properly escaped?

Names yes, values no. Where user-controlled strings feed `RegExp` construction in composition resolution, `escapeRegex` is applied (`packages/shared/src/server/services/PromptService/utils.ts:1-3`, used at `index.ts:365-373`), and `$` in replacement text is doubled to neutralize `$&`-style patterns (`index.ts:375-377`). But interpolated **values** are inserted into prompt text raw everywhere: `String(value)` in `compileTemplateString` (`packages/shared/src/utils/prompts.ts:23`) and direct `result.replace(pattern, varValue)` in `replaceTextVariables` (`packages/shared/src/server/llm/compileChatMessages.ts:29`). There is no HTML/contextual escaping utility for content (the `StringNoHTMLNonEmpty` guard applies only to prompt *names*, `packages/shared/src/utils/zod.ts:212-217`). Additionally, `replaceTextVariables` interpolates the variable *name* into a `RegExp` without `escapeRegex` (`compileChatMessages.ts:28`) — currently safe because call sites pass names filtered by `isValidVariableName`, but it is an exported function with no internal defense (see Failure Modes).

## Architectural Decisions

1. **Custom minimal templating instead of a library.** A ~20-line regex substituter (`packages/shared/src/utils/prompts.ts:14-28`) avoids dependency weight and keeps behavior identical across web/worker/shared; the cost is duplicated logic and divergent edge-case semantics versus a maintained engine.
2. **Store templates verbatim; compile at point of use.** The DB persists raw content (`CreatePromptSchema`, `packages/shared/src/features/prompts/types.ts:28-56`); each consumer (playground, experiments, evals) extracts variables itself, so the platform never owns a runtime binding between template and data.
3. **Shared lexical core, divergent executors.** Regexes and validators live in one client-safe module reused by both UI linting and worker compilation (imports at `web/src/components/editor/CodeMirrorEditor.tsx:31` and `worker/src/features/experiments/utils.ts:8`), yet two different substitution functions implement the actual replace step.
4. **Shift-left validation into editors.** Live diagnostics (`web/src/components/editor/CodeMirrorEditor.tsx:73-115`) and linter-style mapping highlights (`web/src/features/evals/v2/components/Evaluators/Judges/PromptVariableEditor/PromptVariableEditor.tsx:64-102`) push contract violations to authoring time rather than API time.
5. **Composition guarded like a graph algorithm.** Prompt-in-prompt inclusion uses cycle detection, depth limit 5, and conflict errors (`packages/shared/src/server/services/PromptService/index.ts:19,265-281,341-348`) — treating templates as a dependency graph, not naive string nesting.
6. **Fail-fast for interactive surfaces, degrade-for-batch.** Interactive flows throw with named variables; background evals log and substitute `""` so one bad row doesn't kill a batch (`worker/src/features/evaluation/evalService.ts:1510-1541`) — an explicit availability-vs-correctness split.

## Notable Patterns

- **Regex-as-contract**: every surface derives its notion of "variable" from the same exported regexes (`MUSTACHE_REGEX`, `VARIABLE_REGEX`, `UNCLOSED_VARIABLE_REGEX`, `MULTILINE_VARIABLE_REGEX`, `packages/shared/src/utils/stringChecks.ts:8-18`), giving one vocabulary to editors, previews, and workers.
- **Documented escape hatch**: triple-brace literals (`{{{name}}}` renders `{value}`) are encoded in the regex design and its comment (`packages/shared/src/utils/stringChecks.ts:10-12`) and mirrored in the editor tokenizer (`web/src/components/editor/CodeMirrorEditor.tsx:54-58`).
- **Linter-in-editor**: CodeMirror diagnostics with severities and human-readable messages for each malformation class (`web/src/components/editor/CodeMirrorEditor.tsx:77-115`).
- **Test matrix as spec**: the compile behavior is specified almost entirely through table-driven vitest cases including whitespace variants `{{  name  }}` (`worker/src/__tests__/evalService.test.ts:184-188`).
- **Observability at the compile seam**: eval execution sets tracing span attributes per stage including `eval.execution.stage = "compile_prompt"` and logs the first 200 chars of the interpolated prompt (`worker/src/features/evaluation/evalService.ts:920,1018-1020`).
- **Silent-skip composition tags**: malformed `@@@langfusePrompt:` tags are skipped rather than rejected during parsing (`packages/shared/src/features/prompts/parsePromptDependencyTags.ts:32-39`), relying on editor diagnostics to catch them earlier.

## Tradeoffs

- **Lenient-by-default vs. fail-fast-by-default**: preserving `{{missing}}` keeps partial rendering useful (and is test-pinned), but it means a typo'd variable can silently ship to an LLM in eval/experiment paths; the playground compensates with its own leftover scan, i.e., safety is re-implemented per caller rather than guaranteed by the compiler.
- **No value escaping**: raw interpolation maximizes fidelity for LLM prompt text (users generally want literal insertion), but offers zero protection when a variable value needs to survive JSON blocks, XML-ish delimiters, or injection-sensitive downstream parsing.
- **Re-scanning instead of declaring**: deriving variables from current content at each surface stays always-consistent with edits, but prevents type/default/required metadata and makes the public API unable to reject bad templates.
- **Per-variable regex loop**: `replaceTextVariables` runs N replacements over each message (`packages/shared/src/server/llm/compileChatMessages.ts:26-31`) — simpler than a single-pass scanner but O(N×content) and dependent on name validity for correctness.
- **Empty-string degradation in evals**: keeps pipelines running at scale, at the cost of evaluations that can look successful while judging truncated context (only mitigated by log noise at `debug`/`error` level).

## Failure Modes / Edge Cases

- **Object serialization footgun**: `compileTemplateString` applies `String(value)` directly, so a plain object becomes `[object Object]` (`packages/shared/src/utils/prompts.test.ts:14-21`) unless callers route through `compileEvalPrompt`'s `parseUnknownToString` which JSON-serializes objects (`packages/shared/src/utils/prompts.test.ts:24-32`). Two sibling paths serialize the same data differently.
- **Latent regex injection**: `new RegExp(`{{\\s*${varName}\\s*}}`)` with an unescaped name (`packages/shared/src/server/llm/compileChatMessages.ts:28`) would throw `Invalid regular expression` (caught nowhere in that function) or mis-match if ever called with names containing metacharacters; today's call sites pass `isValidVariableName`-filtered names only, so this is a latent, not active, defect.
- **Silent skip of malformed composition tags**: a `@@@langfusePrompt:` tag missing the leading `name=` parameter or with extra parts is dropped without error (`packages/shared/src/features/prompts/parsePromptDependencyTags.ts:32-39`) — the composed prompt just omits the dependency.
- **Whitespace asymmetry between compilers**: the lenient compiler tolerates `{{ name }}` via `\s*` (`packages/shared/src/utils/prompts.ts:19`) while `MUSTACHE_REGEX` captures inner text untrimmed, requiring `.trim()` at call sites (`web/src/features/playground/page/context/index.tsx:242-244`); mismatched handling was significant enough to earn its own test (`worker/src/__tests__/evalService.test.ts:184-188`).
- **Multiline/unclosed braces**: caught only at editor-lint level (`web/src/components/editor/CodeMirrorEditor.tsx:77-94`); nothing server-side prevents storing such content since the create API doesn't scan variables.
- **Eval empty-string masking**: unmapped variables become `""` and evaluation proceeds (`worker/src/features/evaluation/evalService.ts:1516-1541`), so a broken mapping yields a plausible-looking score built on incomplete judge context.
- **Compile errors swallowed**: `compileTemplateString` wraps substitution in try/catch returning the untouched template (`packages/shared/src/utils/prompts.ts:25-27`) — availability preserved, failures invisible.

## Future Considerations

- **Unify the two compilers** behind one implementation (single-pass scanner honoring `isValidVariableName`, optional-whitespace, and a chosen missing-key policy) so preserve-vs-throw is an explicit option, not an accident of which function a caller imported.
- **Add an optional strict mode** to `compileTemplateString` (throw or collect unresolved keys) for batch surfaces that currently inherit silent pass-through.
- **Escape regex metacharacters in `replaceTextVariables`** defensively using the existing `escapeRegex` helper (`packages/shared/src/server/services/PromptService/utils.ts:1-3`) even though current inputs are pre-validated.
- **Standardize value serialization**: adopt `parseUnknownToString` (`packages/shared/src/features/evals/utilities.ts:8-27`) for all paths to eliminate the `[object Object]` branch.
- **Validate variables at the public API boundary** (reuse `getPromptVariableDiagnostics` rules server-side) so SDK/API-created prompts get the same guarantees as UI-authored ones.
- **Emit metrics/counters on degraded substitutions** in the eval worker (currently debug logs only) so silent `""` fills are observable in aggregate.

## Questions / Gaps

- **No evidence found** for f-string or Jinja support anywhere in this source (searched `*.ts/*.tsx/*.py` for `f-string|fstring|f_string`, and `jinja` across `web/src`/`worker/src`); Langfuse's documented multi-syntax SDK support is outside this repository, and this server stores template text syntax-agnostically.
- **No dedicated unit test file for `stringChecks.ts`** itself (`packages/shared/src/utils/` contains tests for `prompts`, `json`, `stringify`, `mediaReferences` — none for `stringChecks`); its behavior is exercised indirectly via `prompts.test.ts` and consumer suites.
- **Type-level variable typing absent**: no evidence of typed variable schemas (e.g., per-prompt `Record<var, type>` declarations); contracts are purely name-based.
- Whether the playground's post-compile leftover check (`web/src/features/playground/page/context/index.tsx:368-380`) is reachable given the pre-flight value check is unclear — likely only via non-obvious mismatches between extraction passes; no test pinning this path was found.
- No evidence of escaping utilities for interpolated *values* anywhere in the source; if such protection exists, it lives outside this repo (SDKs/downstream consumers).

---

Generated by `Dimension 12.02: Prompt Templating and Variable Contracts` against `langfuse`.
