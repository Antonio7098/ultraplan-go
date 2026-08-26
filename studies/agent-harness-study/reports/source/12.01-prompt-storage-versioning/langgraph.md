# Source Analysis: langgraph

## 12.01 Prompt Storage and Versioning

### Source Info

| Field | Value |
|-------|-------|
| Name | langgraph |
| Path | `studies/agent-harness-study/sources/langgraph` |
| Language / Stack | Python (core, prebuilt, cli, sdk-py, checkpoint*), TypeScript (sdk-js stub); monorepo |
| Analyzed | 2026-08-25 |

## Summary

LangGraph is a framework, not an application: it ships **no prompt corpus of its own**. Prompts are caller-supplied artifacts that enter the system at graph-construction time through a typed `Prompt` union (`str | SystemMessage | Callable | Runnable`) on the prebuilt agent constructor (`studies/agent-harness-study/sources/langgraph/libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:121-126`). A `str` prompt is wrapped into a `SystemMessage` and prepended to message state at invoke time (`chat_agent_executor.py:143-148`); the prompt-application step is wrapped in a runnable named `"Prompt"` (`chat_agent_executor.py:119`) so it appears as an identifiable trace span.

There is **no prompt versioning anywhere in the repository**: repo-wide searches for `prompt_version`, `template_version`, `PROMPT_VERSION`, `prompt_id`, and `prompt_hash` returned zero hits. There is no prompt registry or hub integration in library code (the LangSmith/LangChain hub appears only in example notebooks under `examples/rag/`). Run-to-prompt association is possible only through generic observability plumbing — full prompt content is visible in trace spans named `Prompt`, and users can attach arbitrary `tags`/`metadata`/`run_name` to runs via config (`studies/agent-harness-study/sources/langgraph/libs/langgraph/langgraph/_internal/_config.py:194-312`) or via SDK run creation (`studies/agent-harness-study/sources/langgraph/libs/sdk-py/langgraph_sdk/schema.py:522-534`) — but nothing automatically binds an output to a prompt version. Prompts cannot be updated independently of code: the CLI's `langgraph.json` has no prompt key (`_KNOWN_CONFIG_KEYS`, `studies/agent-harness-study/sources/langgraph/libs/cli/langgraph_cli/config.py:564-590`), and graphs are baked into Docker images by import string (`config.py:866-924`). The only escape hatches are ones users build themselves: env/file loading at import time, or supplying a `Callable`/`Runnable` prompt that fetches from an external store at runtime.

Answering the dimension's guiding question — *can you tell exactly which prompt version produced a given output?* — **not from anything this framework records**: you can recover the exact prompt *content* from a trace span, but there is no version identifier, no registry binding, and no deployment mechanism that decouples prompts from code.

## Rating

**3 / 10** — Prompt storage as a managed capability is absent; run-to-prompt association is implicit (trace content only). The score sits at the top of the "absent/implicit" band rather than the bottom because the storage model itself is deliberate and well-typed: prompts are first-class constructor parameters with a documented four-form union and per-form dispatch (`chat_agent_executor.py:137-170`), and every prompt application emits a named, content-bearing trace span — so prompt content is always observable post-hoc. But version identifiers, registries, run bindings, and independent deployment are all missing, and no tests cover any prompt-management behavior because none exists.

## Evidence Collected

Every entry includes a workspace-relative file path with line numbers. Format: `path/to/file.py:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Prompt type (storage contract) | `Prompt = SystemMessage \| str \| Callable[[StateSchema], LanguageModelInput] \| Runnable[StateSchema, LanguageModelInput]` — prompts are caller-supplied values, not stored artifacts | `studies/agent-harness-study/sources/langgraph/libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:121-126` |
| Prompt parameter on agent factory | `create_react_agent(..., prompt: Prompt \| None = None)`; docstring enumerates str/SystemMessage/Callable/Runnable forms | `studies/agent-harness-study/sources/langgraph/libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:292,366-371` |
| String→SystemMessage conversion | `_get_prompt_runnable`: `SystemMessage(content=prompt)`, prepended to `state["messages"]` at invoke time (derived input, not persisted to state) | `studies/agent-harness-study/sources/langgraph/libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:143-148` |
| Trace-span naming of prompt step | `PROMPT_RUNNABLE_NAME = "Prompt"` applied to every branch of prompt application — makes prompt content visible in LangSmith traces but carries no version ID | `studies/agent-harness-study/sources/langgraph/libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:119,140-166` |
| No hardcoded production prompts | Only uppercase "prompt" constants in `libs/prebuilt` are `PROMPT_RUNNABLE_NAME` and a docstring example (`prompt="You are a helpful assistant"`); core `libs/langgraph/langgraph` has zero `SystemMessage(content=...)` prompt literals outside tests | `studies/agent-harness-study/sources/langgraph/libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:119,510`; grep over `libs/langgraph/langgraph/**/*.py` for `SystemMessage\(content=\|system_prompt\|You are` → 0 non-test hits |
| Structured-response secondary prompt | `response_format` may be `(prompt, schema)` tuple; second `SystemMessage` built inline | `studies/agent-harness-study/sources/langgraph/libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:385-387,757-776` |
| File-based prompt pattern (fixture only) | Test-fixture agent reads `prompt.txt`/`subprompt.txt` at import time: `open(Path(__file__).parent.parent / "prompt.txt").read()` — files are 0-byte placeholders demonstrating load-from-file, not a supported mechanism | `studies/agent-harness-study/sources/langgraph/libs/cli/examples/graphs_reqs_a/graphs_submod/agent.py:21-22`; `libs/cli/examples/graphs_reqs_a/prompt.txt` (0 bytes) |
| Inline prompts only in fixtures | Integration graphs hardcode system prompts as plain strings (e.g., `"You are a research assistant. Use the search tool when asked."`) | `studies/agent-harness-study/sources/langgraph/libs/sdk-py/integration/graph/tools_agent.py:95`; `libs/sdk-py/integration/graph/deep_agent.py:79-93` |
| No versioning identifiers | Searches for `prompt_version|template_version|PROMPT_VERSION|prompt_id|prompt_hash` across entire source tree → 0 results | repo-wide grep, `studies/agent-harness-study/sources/langgraph` |
| No prompt registry/hub in libraries | `hub.pull`/`langchainhub`/`prompt_hub` appear only in example notebooks, never in `libs/**` source | e.g., `studies/agent-harness-study/sources/langgraph/examples/rag/langgraph_agentic_rag.ipynb:367,385` |
| Generic run metadata/tags plumbing | `patch_config(run_name=...)`; callback managers merge `config["tags"]` and `config["metadata"]` into traces (`inheritable_metadata`, `langsmith_inheritable_metadata`) — the hooks a user could repurpose to stamp prompt versions | `studies/agent-harness-study/sources/langgraph/libs/langgraph/langgraph/_internal/_config.py:194-233,237-312` |
| Config merge semantics | `merge_configs` merges `metadata` keys and concatenates `tags` across parent/child configs | `studies/agent-harness-study/sources/langgraph/libs/langgraph/langgraph/_internal/_config.py:147-176` |
| Runnable naming honors config | Per-invocation span name resolves `config.get("run_name") or self.get_name()` — user-overridable trace identity, not prompt identity | `studies/agent-harness-study/sources/langgraph/libs/langgraph/langgraph/_internal/_runnable.py:422,426,495-499` |
| Node-level tags | `ToolNode(tags=...)` documented as "Optional metadata tags to associate with the node for filtering and organization" | `studies/agent-harness-study/sources/langgraph/libs/prebuilt/langgraph/prebuilt/tool_node.py:672-673,772` |
| Server API run metadata | `RunCreate` accepts `metadata: dict` ("Additional metadata to associate with the run") and `config` per run — free-form provenance channel, nothing prompt-specific | `studies/agent-harness-study/sources/langgraph/libs/sdk-py/langgraph_sdk/schema.py:522-534`; params mirrored in `libs/sdk-py/langgraph_sdk/_sync/runs.py:83-205` |
| UI event metadata | Streamed UI events embed `{run_id, tags, name: config.get("run_name"), ...}` | `studies/agent-harness-study/sources/langgraph/libs/langgraph/langgraph/graph/ui.py:116-123` |
| Deployment config excludes prompts | `_KNOWN_CONFIG_KEYS` contains no prompt/template key; unknown keys produce warnings | `studies/agent-harness-study/sources/langgraph/libs/cli/langgraph_cli/config.py:564-605` |
| Prompts baked into images | Graphs referenced by import string (`./agent.py:graph`) and rewritten into Docker build context by the CLI — prompt text ships inside the deployed image | `studies/agent-harness-study/sources/langgraph/libs/cli/langgraph_cli/config.py:866-924` |
| Externalization escape hatch | `env` key (dict or `.env` path) is passed through to deployments — the only sanctioned injection channel usable for external prompt text | `studies/agent-harness-study/sources/langgraph/libs/cli/langgraph_cli/config.py:378`; schema docs `libs/cli/schemas/schema.json:113-125` |
| Docs gap | In-repo docs contain only redirect tables and outbound links; no prompt-management/versioning documentation survives (redirects only) | `studies/agent-harness-study/sources/langgraph/docs/redirects.json:65,279`; `docs/llms.txt` |

## Answers to Dimension Questions

**1. Where are prompts stored?**
Nowhere centrally. The framework defines prompts as caller-supplied values typed by the `Prompt` union (`studies/agent-harness-study/sources/langgraph/libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:121-126`). In practice within this repo they live as inline strings/objects in user code at graph construction; test/integration fixtures show exactly this pattern (`libs/sdk-py/integration/graph/tools_agent.py:95`). There is no database table, no prompt directory, no registry, and no template-engine machinery (no jinja/mustache usage outside lockfiles). A file-read pattern exists only as 0-byte fixture placeholders (`libs/cli/examples/graphs_reqs_a/graphs_submod/agent.py:21-22`).

**2. Are prompt versions tracked?**
No. Zero occurrences of any version-identifier concept for prompts anywhere in the source tree (searched `prompt_version`, `template_version`, `PROMPT_VERSION`, `prompt_id`, `prompt_hash`). The nearest adjacent mechanism is generic metadata merging whose docstring notes packages can contribute package versions to trace metadata (`studies/agent-harness-study/sources/langgraph/libs/langgraph/langgraph/_internal/_config.py:87-98`) — that records library versions, not prompt versions.

**3. Can a run be traced to the exact prompt version used?**
Content yes, version no. Every prompt application runs inside a runnable named `"Prompt"` (`chat_agent_executor.py:119,137-170`), so a trace span captures the exact prompt payload for a given run — sufficient to reconstruct *what* was sent if tracing is enabled and retained. But there is no version identifier, hash, or registry pointer stamped onto runs, checkpoints, or SDK run records; `RunCreate.metadata` (`studies/agent-harness-study/sources/langgraph/libs/sdk-py/langgraph_sdk/schema.py:531-532`) is free-form and empty unless the caller populates it. Correlation between a run and a prompt revision is therefore entirely the integrator's responsibility.

**4. Can prompts be updated without redeploying code?**
Not natively. The CLI compiles graphs (and whatever prompt values their code references) into Docker images keyed off import strings (`studies/agent-harness-study/sources/langgraph/libs/cli/langgraph_cli/config.py:866-924`), and `langgraph.json` offers no prompt indirection key (`config.py:564-590`). Three user-built workarounds are compatible with the design: (a) read prompt text from the `env` config channel at import time (`config.py:378`); (b) pass a `Callable`/`Runnable` prompt that fetches current prompt text from an external store per invocation — explicitly permitted by the `Prompt` type (`chat_agent_executor.py:124-125`); (c) pull from the LangChain hub in user code, as examples do (`examples/rag/langgraph_agentic_rag.ipynb:367`). None of these ship as supported prompt management in this repository.

## Architectural Decisions

1. **Prompts as code-resident constructor arguments, not stored assets.** The `Prompt` union type (`chat_agent_executor.py:121-126`) makes prompt provenance a compile-time property of the graph definition. This is a deliberate scoping decision: LangGraph treats prompt lifecycle management as out-of-framework, deferring to LangChain ecosystem tooling (hub, `ChatPromptTemplate`) or platform products.
2. **Uniform dispatch over four prompt forms.** `_get_prompt_runnable` (`chat_agent_executor.py:137-170`) normalizes str/SystemMessage/async-callable/callable/Runnable inputs into a single runnable boundary, so downstream nodes never special-case prompt origin.
3. **Observability instead of versioning.** Rather than stamping version IDs, the framework guarantees that prompt application is a named, inspectable trace step (`PROMPT_RUNNABLE_NAME`, `chat_agent_executor.py:119`) riding on generic tag/metadata propagation (`_config.py:237-312`). Provenance is reconstructed from traces, not recorded structurally.
4. **Deployment unit = code image.** The CLI's deployment model bundles graph code, dependencies, and implicitly its prompt literals into one artifact (`config.py:866-924`), making prompt changes inherently redeploy events unless users build externalization themselves.
5. **Free-form metadata as the universal extension point.** Both the runtime config (`tags`/`metadata`, `_config.py:250-270`) and the server API (`RunCreate.metadata`, `schema.py:531-532`) expose untyped dictionaries rather than schema'd fields like `prompt_version` — maximum flexibility, zero enforced discipline.

## Notable Patterns

- **Typed escape hatch for externalized prompts**: accepting `Runnable[StateSchema, LanguageModelInput]` as a prompt (`chat_agent_executor.py:125,165-166`) means a user can drop in a remote-config-backed prompt fetcher without framework changes — extension by composition rather than configuration.
- **Named-step tracing convention**: wrapping each prompt branch in `RunnableCallable(..., name=PROMPT_RUNNABLE_NAME)` (`chat_agent_executor.py:140-163`) yields uniform span names regardless of prompt form, which keeps trace queries stable even though content varies.
- **Import-time file reads as the folk pattern**: even the repo's own fixtures reach for `open(...).read()` on `.txt` files (`libs/cli/examples/graphs_reqs_a/graphs_submod/agent.py:21-22`) — evidence that when the framework offers no storage layer, users (and its own examples) fall back to filesystem adjacency.
- **Tag filtering hygiene**: internal bookkeeping tags (`seq:step:*`) are stripped before surfacing tags to stream consumers (`_config.py:463-473`), keeping user-supplied tags (a plausible prompt-version stamp) clean in outputs.

## Tradeoffs

- **Simplicity vs. governance**: no prompt store means zero infrastructure burden and trivially reviewable prompts in code diffs, but enterprises lose audit trails, rollback, and A/B switching that a registry provides. The framework pushes that entire burden to integrators.
- **Trace-content provenance vs. structural provenance**: recovering prompt content from a `"Prompt"` span is complete but expensive (requires tracing enabled, retention, and manual inspection); a `prompt_version` field would be cheap and queryable. LangGraph chose the former by default.
- **Compile-time binding vs. hot updates**: baking prompts into deployed images (`config.py:866-924`) gives reproducible deployments but forces a rebuild/redeploy cycle for a one-line prompt change — the exact friction prompt platforms exist to remove.
- **Untyped metadata vs. enforced schema**: free-form `metadata` dicts accept anything, including ad-hoc prompt labels, but nothing validates consistency, so two teams will inevitably stamp different shapes.

## Failure Modes / Edge Cases

- **Unreconstructible provenance without tracing**: if tracing/callbacks are disabled, the only record of what prompt produced an output is whatever the caller remembered to put in `metadata`. Nothing else persists prompt identity (checkpoint metadata stores state lineage, not prompt IDs).
- **Mutable-object prompt drift**: a `SystemMessage` or closure captured at construction time (`chat_agent_executor.py:149-153`) can be mutated after compile, so two runs of the same compiled graph may observe different effective prompt content while every trace still says merely `"Prompt"` — silent divergence with no version marker.
- **Import-time file/env reads are fragile**: the fixture pattern (`agent.py:21-22`) binds prompt text at module import; a changed environment or missing file breaks graph import (deployment failure), not just a degraded run.
- **Config-key false confidence**: `langgraph.json` warns on unknown keys (`config.py:593-605`), so a team inventing a `"prompts"` key gets a warning — but nothing stops them shipping env-var-indirection schemes invisible to the deployment tooling, creating untracked configuration surface.
- **Hub-coupled examples rot silently**: examples pulling `hub.pull("rlm/rag-prompt")` (`examples/rag/langgraph_agentic_rag.ipynb:367`) depend on third-party registry state outside this repo; a hub-side edit changes example behavior with no local diff.

## Future Considerations

- Add an optional `prompt_id`/`prompt_version` field alongside `PROMPT_RUNNABLE_NAME` so the existing trace span could carry a structured identifier without changing the storage model.
- Support a `PromptLoader` protocol (mirroring the existing `Callable`/`Runnable` acceptance) with a caching layer, giving the file-read fixture pattern a first-class, testable home.
- Extend `langgraph.json` with an optional prompt-source mapping (file/URL/env) validated by `_KNOWN_CONFIG_KEYS`, enabling prompt-only image rebuilds or runtime resolution.
- Stamp package/version metadata (already merged generically per `_config.py:87-98`) with an opt-in graph-definition hash to approximate run→definition-version correlation today.

## Questions / Gaps

- **No evidence found** for any server-side prompt storage: this monorepo contains the client SDKs and CLI, but the LangGraph Platform server implementation is not in this source tree, so whether the hosted service adds prompt registries could not be verified here (searched all of `libs/`, `docs/`, `examples/`).
- **No evidence found** for checkpoint-level prompt recording: checkpoint metadata structures were inspected only via config/metadata plumbing (`_internal/_config.py:64-98`); a dedicated study of checkpoint payloads would be needed to rule out incidental persistence of injected system messages.
- Whether `libs/sdk-js` had any prompt handling is unverifiable in-tree: the package moved to a separate repository and contains only an 11-line README stub (`studies/agent-harness-study/sources/langgraph/libs/sdk-js/README.md`).
- The relationship between the `"langsmith:hidden"` tag convention (`studies/agent-harness-study/sources/langgraph/libs/langgraph/langgraph/constants.py:26`) and prompt-span visibility was not traced end-to-end; it may hide some spans from traces under certain configurations.

---

Generated by `12.01-prompt-storage-and-versioning` against `langgraph`.
