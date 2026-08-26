# Source Analysis: langgraph

## Dimension 12.02: Prompt Templating and Variable Contracts

### Source Info

| Field | Value |
|-------|-------|
| Name | langgraph |
| Path | `studies/agent-harness-study/sources/langgraph` |
| Language / Stack | Python (monorepo: `libs/langgraph`, `libs/prebuilt`, `libs/cli`, `libs/checkpoint*`; JS SDK not vendored — only a README at `libs/sdk-js/README.md`) |
| Analyzed | 2026-08-25 |

> **Path convention:** all evidence paths below are relative to the source root `studies/agent-harness-study/sources/langgraph/`.

## Summary

LangGraph deliberately does **not** implement its own prompt template engine. The core library (`libs/langgraph/langgraph/`) contains zero prompt-related code (verified by searching all `.py` files under `libs/langgraph/langgraph/` for "prompt" — no matches). Instead, prompt parameterization is concentrated in the `prebuilt` library and takes the form of a **typed prompt contract** rather than string interpolation:

1. `create_react_agent` accepts a `Prompt` union type (`SystemMessage | str | Callable[[StateSchema], LanguageModelInput] | Runnable[StateSchema, LanguageModelInput]`) (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:121-126`).
2. A plain `str` prompt is treated as **static content** — it is wrapped verbatim into a `SystemMessage` and prepended to the message list; no `.format()`, f-string, or placeholder substitution is ever applied (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:143-148`).
3. Dynamic "variables" are injected by giving user-supplied callables or Runnables access to the **entire graph state**, plus signature-inspected runtime kwargs (`config`, `store`, `writer`, `runtime`, `previous`, `error`) declared in `KWARGS_CONFIG_KEYS` (`libs/langgraph/langgraph/_internal/_runnable.py:166-229`).
4. The only literal templates in the codebase are Python `str.format` error-message constants in the tool node (`TOOL_CALL_ERROR_TEMPLATE` etc.), which format developer-controlled constant strings with exception/tool data to produce re-prompting feedback for the LLM (`libs/prebuilt/langgraph/prebuilt/tool_node.py:108-121`).

When actual template engines appear (langchain-core's `ChatPromptTemplate` with `{variable}` fields and `MessagesPlaceholder`), they come from the external `langchain-core` dependency and are composed into graphs as ordinary Runnables; LangGraph itself never validates their variables. There are **no escaping/sanitization utilities for prompt content** anywhere in the source (the only escaping helpers target SQL LIKE/GLOB patterns in checkpoint stores: `libs/checkpoint-sqlite/langgraph/store/sqlite/base.py:111-114`, `libs/checkpoint-postgres/langgraph/store/postgres/base.py:1283-1289`). Missing-variable behavior is explicit and predictable on framework paths (typed `ValueError`s) but silent (`None` defaults) on helper paths.

**Answer to the dimension question ("Can a prompt template be reused with different variables safely?"):** Yes — through the callable/Runnable prompt contract bound to a typed state schema, reuse across variable sets is type-checked at graph construction time. Static strings are inherently safe because they are never interpolated. But safety of user-authored interpolation logic is the developer's responsibility; LangGraph validates structure, not content.

## Rating

**7 / 10**

Rationale against the rubric:

- **Explicit interfaces (7-8 tier):** the `Prompt` union type is documented and typed (`chat_agent_executor.py:121-126`, docstring at `:366-371`); required state keys are validated at construction time with a precise error listing missing keys (`chat_agent_executor.py:539-545`); runtime kwarg injection is driven by an inspectable contract table (`_runnable.py:168-229`); missing required injected values raise `ValueError("Missing required config key '...' for '<name>'.")` (`_runnable.py:413-418`).
- **Tests (7-8 tier):** every accepted prompt form has dedicated tests — none, `SystemMessage`, `str`, callable (sync+async), `Runnable`, and store-injecting callables (`libs/prebuilt/tests/test_react_agent.py:91-119,148-203,207-250`).
- **Why not higher:** there is no escaping or sanitization layer for content interpolated into prompts (tool errors and untrusted tool names flow verbatim into `ToolMessage` content fed back to the model — by design but undocumented as a risk: `tool_node.py:430`, `:1272-1275`); state-key reads via the internal helper silently default to `None` (`chat_agent_executor.py:129-134`); the invalid-prompt-type `ValueError` branch (`chat_agent_executor.py:167-168`) has no test coverage; and templating proper (variable validation, missing-variable errors) is delegated wholesale to langchain-core without any integration-level contract.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Template engine | No engine in-repo. Core library has zero "prompt" matches; `jinja2` appears only as a transitive entry in lockfiles, absent from `pyproject.toml` dependencies | `libs/langgraph/pyproject.toml` (no jinja/prompt dep); `libs/prebuilt/pyproject.toml` |
| Prompt contract (type union) | `Prompt = SystemMessage \| str \| Callable[[StateSchema], LanguageModelInput] \| Runnable[...]` | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:121-126` |
| String prompts are static | `isinstance(prompt, str)` → wrapped into `SystemMessage(content=prompt)`, no formatting applied | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:143-148` |
| Variable injection point (state) | Callable/Runnable prompts receive full `StateSchema`; output passed directly to the LLM | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:370-371,590,604,614` |
| Variable injection point (kwargs) | Signature-inspected injection of `config`, `writer`, `store`, `previous`, `runtime`, `error` into any node/task/tool callable | `libs/langgraph/langgraph/_internal/_runnable.py:168-229,338-346` |
| Missing injected variable handler | Raises `ValueError(f"Missing required config key '{runtime_key}' for '{self.name}'.")` when a declared kwarg has no runtime value and no default | `libs/langgraph/langgraph/_internal/_runnable.py:413-418` |
| Weak validation of annotations | Mismatched `config` annotation emits a `UserWarning` instead of failing | `libs/langgraph/langgraph/_internal/_runnable.py:348-359` |
| Schema-level variable contract | `create_react_agent` validates `state_schema` contains `messages`, `remaining_steps` (+ `structured_response` if `response_format`), raising `ValueError` naming missing keys | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:539-545` |
| Missing-messages guard | `_get_model_input_state` raises `ValueError` if neither `messages` nor `llm_input_messages` present before prompt/model run | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:636-649` |
| Silent-default helper | `_get_state_value(state, key, default=None)` returns `None` for absent keys (dict or BaseModel) | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:129-134` |
| Literal templates (errors) | `INVALID_TOOL_NAME_ERROR_TEMPLATE`, `TOOL_CALL_ERROR_TEMPLATE`, `TOOL_EXECUTION_ERROR_TEMPLATE`, `TOOL_INVOCATION_ERROR_TEMPLATE` — `str.format` constants | `libs/prebuilt/langgraph/prebuilt/tool_node.py:108-121` |
| Untrusted data into prompt content | Tool-call errors formatted via `TOOL_CALL_ERROR_TEMPLATE.format(error=repr(e))`; unknown tool name (LLM-generated) inserted verbatim into error `ToolMessage` returned to the model | `libs/prebuilt/langgraph/prebuilt/tool_node.py:430,1272-1278` |
| Structured-output prompt injection point | `response_format` may be a `(system_prompt, schema)` tuple; the static system prompt is prepended to messages for the structured call | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:756-758,774-776` |
| History validity precondition | `_validate_chat_history` fails the run if AI tool calls lack matching ToolMessages (provider-facing prompt invariant) | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:243-268` |
| Escaping utilities | None for prompt content; escaping exists only for SQL LIKE/GLOB in stores | `libs/checkpoint-sqlite/langgraph/store/sqlite/base.py:111-155`, `libs/checkpoint-postgres/langgraph/store/postgres/base.py:1283-1289` |
| Delegated templating (tests) | `ChatPromptTemplate.from_messages([... , ("placeholder", "{messages}")])` invoked manually inside an agent node via `prompt.invoke(state)` — full state dict used as variables | `libs/langgraph/tests/test_pregel.py:3796-3801` |
| Optional template variables (example) | `MessagesPlaceholder(variable_name="messages", optional=True)` in CLI example graphs | `libs/cli/examples/graphs/storm.py:225,269,302` |
| Example-level template use | `ChatPromptTemplate.from_messages([("system", system_prompt), MessagesPlaceholder(...)]) \| llm` | `examples/chatbot-simulation-evaluation/simulation_utils.py:46-53` |
| Tests: prompt forms | `test_no_prompt`, `test_system_message_prompt`, `test_string_prompt`, `test_callable_prompt(_async)`, `test_runnable_prompt`, `test_prompt_with_store(_async)` | `libs/prebuilt/tests/test_react_agent.py:91-119,148-156,159-167,170-191,194-203,207-276` |
| Naming collision (non-prompt templates) | CLI `templates.py` manages GitHub-zip project scaffolding, unrelated to prompt templating | `libs/cli/langgraph_cli/templates.py:10-35,43-91` |

## Answers to Dimension Questions

### 1. How are prompts parameterized?

Through composition, not interpolation. Four mechanisms:

- **Static text:** `str` or `SystemMessage` prompts are prepended to `state["messages"]` unchanged (`chat_agent_executor.py:143-153`). No placeholders exist in this path.
- **Callable over state:** a function receiving the full graph state returns the `LanguageModelInput`; this is the sanctioned way to build dynamic prompts (docstring: `chat_agent_executor.py:366-371`; test: `test_react_agent.py:170-179`).
- **Runnable over state:** same contract for Runnables, enabling langchain-core `ChatPromptTemplate` pipelines whose input is the whole state dict (`test_pregel.py:3796-3801` shows `prompt.invoke(state)` with a `("placeholder", "{messages}")` field).
- **Signature-injected runtime context:** prompt callables can declare extra parameters (`config`, `store`, `writer`, `runtime`, `previous`) that the runtime injects based on name + type annotation matching (`_runnable.py:168-229`, `:339-346`, invocation-time population at `:392-419`). Test: `test_react_agent.py:216-250` (`def prompt(state, config, *, store)` fetching per-user memory from the store into a system message).

### 2. Are variable contracts explicit?

Partially — structural contracts are explicit, content contracts are not.

- Explicit: the `Prompt` type union (`chat_agent_executor.py:121-126`); the documented four-form `prompt` parameter (`chat_agent_executor.py:366-371`); the `KWARGS_CONFIG_KEYS` table enumerating exactly which kwargs may be injected and with which annotations (`_runnable.py:168-246`); construction-time validation that custom `state_schema` declares required keys (`chat_agent_executor.py:539-545`); the `pre_model_hook` output contract requiring `messages` or `llm_input_messages` (`chat_agent_executor.py:396-414`, enforced at `:641-649`).
- Not explicit: nothing validates that a `Runnable` prompt's input schema matches the agent state schema up front — mismatches surface only at invoke time; langchain-core template variables (`{var}` fields) are entirely outside LangGraph's knowledge; there is no schema describing *which* state keys a dynamic prompt consumes.

### 3. Is missing-variable behavior predictable?

Yes on framework paths, silent on helper paths.

- Framework raises precisely: missing required injected config key → `ValueError("Missing required config key '<key>' for '<name>'.")` (`_runnable.py:413-418`); missing `messages`/`llm_input_messages` → `ValueError` with the offending state dumped (`chat_agent_executor.py:644-649`); missing `state_schema` keys → `ValueError(f"Missing required key(s) {missing_keys} ...")` (`chat_agent_executor.py:544-545`).
- Silent defaults elsewhere: `_get_state_value(state, key, default=None)` swallows absent keys (`chat_agent_executor.py:129-134`); injected kwargs with defaults fall back silently (`store=None`, no-op writer — `_runnable.py:186,204-205`).
- In the delegated case, missing template variables follow langchain-core semantics (e.g., `optional=True` placeholders tolerate absence — `storm.py:225`), invisible to LangGraph.
- Untested branch: passing an unsupported `prompt` type raises `ValueError(f"Got unexpected type for \`prompt\`: {type(prompt)}")` (`chat_agent_executor.py:167-168`), but no test exercises it (searched `test_react_agent.py` for "unexpected type" — no matches).

### 4. Are variables properly escaped?

No escaping layer exists for prompt content. Values interpolated into LLM-bound strings — exception reprs (`tool_node.py:430,373-375`) and LLM-generated tool names (`tool_node.py:1269-1275`) — are inserted verbatim into `ToolMessage` content that is fed back to the model on the next turn. This is intentional error-re-prompting (the templates literally instruct "Please fix your mistakes", `tool_node.py:111`), so "injection" here is by design rather than a vulnerability in the classic sense; however, the repo ships no sanitization utilities and documents no guidance about hostile tool outputs or prompt-injection via tool errors. Note the format-string side is safe: `str.format` is always called on developer-owned constants with keyword arguments (`tool_node.py:373-375,430,1272-1275`), never on data-derived strings, so template-code injection cannot occur. The only true escaping code in the monorepo targets SQL LIKE/GLOB patterns in store filters (`checkpoint-sqlite/.../base.py:111-155`, `checkpoint-postgres/.../base.py:1283-1289`).

## Architectural Decisions

1. **Composition over templating.** Rather than embedding a template engine and a variable registry, LangGraph makes the *prompt producer* a first-class graph component (callable/Runnable over typed state). This moves variable binding from string-substitution time to ordinary Python execution under the state schema's type system (`chat_agent_executor.py:121-126,137-170`).
2. **Delegation of classical templating to langchain-core.** When `{variable}` templates are wanted, users compose `ChatPromptTemplate` as a Runnable node step; LangGraph treats it as an opaque Runnable (`test_pregel.py:3796-3801`, `examples/chatbot-simulation-evaluation/simulation_utils.py:46-53`). This avoids a second template implementation but forfeits integration-level validation.
3. **Convention-based runtime injection.** Injectable kwargs are discovered by inspecting function signatures against the `KWARGS_CONFIG_KEYS` table, with type annotations acting as opt-in markers (`_runnable.py:168-229,338-346`). This keeps prompt functions pure w.r.t. their declared dependencies and makes hidden data flow visible in the signature.
4. **Fail-fast at construction for schemas, fail-fast at invocation for values.** State-schema keys are checked when the graph is built (`chat_agent_executor.py:539-545`); missing runtime values raise when the node executes (`_runnable.py:413-418`); absent optional state keys degrade to defaults instead.
5. **Errors as prompt material.** Tool failures are converted into structured, template-formatted `ToolMessage`s so the model can self-correct within the loop (`tool_node.py:108-121,1272-1278`) — a deliberate design where the prompt grows with runtime failure data.

## Notable Patterns

- **Union-typed extension points:** `Prompt` mirrors how the rest of the API accepts `str | Callable | Runnable`, letting simple cases stay trivial while complex cases get full programmatic control (`chat_agent_executor.py:121-126`).
- **Single normalization funnel:** every accepted prompt form is normalized once in `_get_prompt_runnable` and then uniformly composed as `prompt_runnable | model` (`chat_agent_executor.py:137-170,590,604,616`), including re-resolution per step for dynamic models (`:599-618`).
- **Named runnable for observability:** the prompt step is tagged `PROMPT_RUNNABLE_NAME = "Prompt"` (`chat_agent_executor.py:119`) so it appears as a distinct trace span — an operational safeguard making prompt assembly observable in traces.
- **Optional message placeholders** (`MessagesPlaceholder(variable_name="messages", optional=True)`) used pervasively in the CLI example graph to tolerate empty histories (`storm.py:225,269,302`).
- **Contract documentation adjacent to enforcement:** the `pre_model_hook` docstring spells out the exact required output shape (`chat_agent_executor.py:396-424`) and the same requirement is enforced with a descriptive `ValueError` at runtime (`:641-649`).

## Tradeoffs

- **Safety vs. power:** static string prompts eliminate an entire class of missing-variable/format bugs (there is nothing to interpolate), but push all dynamism into user code where no framework guarantees apply.
- **Flexibility vs. validation:** accepting arbitrary Callables/Runnables means LangGraph cannot know a prompt's variable needs; mis-typed or mis-named state access surfaces as runtime `KeyError`/`AttributeError` inside user code, outside framework error handling. Annotation mismatches on injected kwargs downgrade to warnings (`_runnable.py:348-359`) — permissive for migration, weaker as a contract.
- **Delegation vs. integration:** leaning on langchain-core templates avoids duplication but leaves variable validation, undefined-variable errors, and escaping to another library whose behavior this source cannot guarantee or test end-to-end.
- **Observability vs. simplicity:** the `"Prompt"` named step aids tracing, but because dynamic prompts are arbitrary code, what went into the prompt is only as inspectable as the user's implementation.

## Failure Modes / Edge Cases

- **Missing `messages` key:** caught pre-invocation with a `ValueError` that dumps the offending state (`chat_agent_executor.py:641-649`) — predictable.
- **Missing injected config key:** `ValueError` naming the key and runnable (`_runnable.py:413-418`) — predictable; but a *typo'd annotation* yields only a warning and then either injection or a later confusing failure.
- **Silently-absent state keys:** `_get_state_value(..., default=None)` lets `[prompt] + _get_state_value(state, "messages")` become `[prompt] + None` → `TypeError` far from the cause if guards upstream change (`chat_agent_executor.py:129-134,145-148`).
- **Invalid prompt type:** unsupported types raise `ValueError` (`chat_agent_executor.py:167-168`) — correct but untested.
- **Untrusted content enters prompts:** tool kwargs, exception reprs, and hallucinated tool names are embedded verbatim into next-turn `ToolMessage`s (`tool_node.py:373-375,430,1272-1275`); adversarial tool output can steer the conversation (accepted risk, unmitigated, undocumented).
- **Async/sync mismatch:** sync invocation of an async prompt/model raises a clear `RuntimeError` (`chat_agent_executor.py:664-670,747-752`).
- **History invariant violations:** dangling tool calls without results abort with an explanatory error before the model sees a malformed prompt (`chat_agent_executor.py:243-268`).

## Future Considerations

- Add tests for the invalid-prompt-type branch and for callable prompts reading absent state keys, to pin down current missing-variable behavior (`chat_agent_executor.py:167-168`).
- Consider validating that a `Runnable` prompt's `get_input_schema()` is compatible with `state_schema` at construction time, converting invoke-time surprises into build-time errors.
- Escalate the `config` annotation-mismatch warning (`_runnable.py:352-359`) to an error once the migration window closes, strengthening the injection contract.
- Document (or sanitize) the trust boundary around error-template interpolation into `ToolMessage`s — e.g., truncation of `repr(e)` or opt-in redaction — since these strings are model-facing (`tool_node.py:108-121,430`).
- A thin adapter exposing which state keys a prompt callable touches would make variable contracts introspectable for multi-agent composition and UIs.

## Questions / Gaps

- **No in-repo docs on prompt handling:** the `docs/` directory contains only redirects/llms.txt (docs site content not vendored), so documentation claims could not be cross-checked against implementation beyond docstrings (`chat_agent_executor.py:366-371`).
- **JS parity unverifiable:** `libs/sdk-js/` ships only a README; no TypeScript source exists in this snapshot to assess JS-side templating.
- **Undefined-variable behavior of langchain-core templates** (e.g., raising on missing `{var}`) could not be verified from this source alone — it lives in the external dependency; only the `optional=True` placeholder escape hatch is evidenced (`storm.py:225`).
- **No evidence found** of any prompt-content escaping, redaction, or size-limiting utility anywhere in the selected source (searched `escape`, `sanitize`, `redact`, `truncat` across `libs/**/*.py`; only SQL-pattern escaping matched).
- Whether `jinja2`'s appearance in `uv.lock` is reachable from library code: it is not declared in any library `pyproject.toml` and no import was found, so it is assessed as transitive tooling/docs dependency noise.

---

Generated by `12.02-prompt-templating-and-variable-contracts` against `langgraph`.
