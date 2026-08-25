# Source Analysis: langgraph

## Dimension 11.01: Context Selection Policy

### Source Info

| Field | Value |
|-------|-------|
| Name | langgraph |
| Path | `studies/agent-harness-study/sources/langgraph` |
| Language / Stack | Python (core `libs/langgraph`, `libs/prebuilt`, `libs/checkpoint`); JS/TS SDK (`libs/sdk-js`) |
| Analyzed | 2026-08-25 |

> Citation convention: all file paths below are relative to the selected source root `studies/agent-harness-study/sources/langgraph/`.

## Summary

LangGraph does not ship a single "context builder." Instead, context selection is decomposed across three explicit, layered mechanisms:

1. **Channel projection at the framework layer.** A node's view of the world is determined by its declared input schema: when a graph is compiled, each node is wired to read only the channels (state keys) named in its input schema, with a mapper coercing the raw channel dict to the schema class (`libs/langgraph/langgraph/graph/state.py:1515-1547`). The `PregelNode.channels` field is documented as "the channels that will be passed as input to bound" (`libs/langgraph/langgraph/pregel/_read.py:102-105`). This is static, declarative per-node selection of workspace/state signals.

2. **Message-history assembly at the agent layer.** In the prebuilt ReAct agent, the model input is the full `messages` state key, optionally prefixed by a system prompt (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:137-170`). History curation is exposed through two extension points: the `pre_model_hook` node, which may either return `llm_input_messages` (a model-only view that does not mutate persisted state) or overwrite `messages` using `RemoveMessage(id=REMOVE_ALL_MESSAGES)` (`libs/prebuilt/langgraph/langgraph/../prebuilt/chat_agent_executor.py:396-424`; split-view wiring at `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:636-658,723-742`). There is no built-in trimmer or summarizer in this repo; the docstring explicitly frames trimming/summarization as user-provided hook content ("Useful for managing long message histories (e.g., message trimming, summarization, etc.)", `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:397`), and the whole factory is deprecated in favor of `langchain.agents.create_agent` middleware (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:53-56,274-277`).

3. **Injection-based tool context at the tool layer.** Tools declare what context they need via `InjectedState` (whole state or a single field), `InjectedStore` (persistent store handle), and `ToolRuntime` annotations (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1753-1826,1829-1901,1663-1729`). At execution time `ToolNode._inject_tool_args` resolves these dependencies and — critically — strips any LLM-supplied values for injected parameter names before merging trusted values last (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1421-1430`). Injected parameters are excluded from tool schemas shown to the model (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1814-1819,1894-1896`).

The overall policy is **explicit and declarative but minimal**: LangGraph provides the interfaces, invariants, and safeguards for context selection, while the actual inclusion policy (what to keep, trim, summarize, retrieve) is delegated to graph authors. Long-term memory retrieval is an opt-in `BaseStore.search/get` API rather than an automatic pipeline (`libs/checkpoint/langgraph/store/base/__init__.py:708-725,756-789`).

## Rating

**Score: 7 / 10**

Rationale against the rubric:

- **Clear model + explicit interfaces (7-8 band):** Selection policy is expressed in code-visible artifacts — state schemas and channel lists (`libs/langgraph/langgraph/graph/state.py:1515-1547`), the typed `Prompt` union (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:121-126`), injection annotations (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1753-1826`), and the reducer contract (`libs/langgraph/langgraph/graph/message.py:60-244`). Behavior is pinned by tests, including adversarial ones (e.g., "InjectedState should use graph state, not LLM-supplied values" at `libs/prebuilt/tests/test_tool_node.py:2202-2232`; injected-arg stripping for custom subclasses at `libs/prebuilt/tests/test_tool_node.py:2172-2199`).
- **Operational safeguards present but narrow:** LLM-forged hidden arguments are stripped (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1421-1429`), invalid chat history is rejected before model invocation (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:243-271`), and unknown-id removals raise instead of silently corrupting history (`libs/langgraph/langgraph/graph/message.py:227-230`). Sensitive-key filtering exists only for tracing metadata, not model context (`libs/langgraph/langgraph/_internal/_config.py:423-432`).
- **Why not higher:** No built-in token-budget management, trimming, or summarization (all delegated to hooks or migrated to `langchain`); no PII redaction on model-bound messages — the repo's own threat model records unbounded retention and "No field-level encryption or redaction" for conversation history (`.github/THREAT_MODEL.md:179,396`); tool outputs enter context verbatim with no size cap (no truncation logic found in `ToolNode`; see Gaps). The answer to the dimension's guiding question — "Can the system explain why a particular document was included in context?" — is only partially: provenance is structural (channel/schema/annotation), but there is no per-item inclusion rationale recorded anywhere.

## Evidence Collected

Every entry includes a file path with line numbers, relative to `studies/agent-harness-study/sources/langgraph/`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Node-level context selection | Compiled nodes read only channels declared in their input schema; mapper coerces to schema class ("read state keys and managed values") | libs/langgraph/langgraph/graph/state.py:1515-1547 |
| Channel projection primitive | `PregelNode.channels`: "channels that will be passed as input to bound" (str = single value, list = dict) | libs/langgraph/langgraph/pregel/_read.py:102-105 |
| Branch/routers also schema-scoped | `attach_branch` selects channels from branch/node input schema | libs/langgraph/langgraph/graph/state.py:1598-1605 |
| Message merge reducer | `add_messages`: append-only merge, ID-based upsert, `RemoveMessage` deletion, `REMOVE_ALL_MESSAGES` sentinel returns only suffix | libs/langgraph/langgraph/graph/message.py:60-244 (sentinel def at :38; remove-all semantics :209-213; unknown-id error :227-230) |
| OpenAI-format normalization | `format="langchain-openai"` converts merged history to OpenAI-compatible content blocks before it reaches the model | libs/langgraph/langgraph/graph/message.py:236-240,376-389 |
| Default agent state | `AgentState.messages: Annotated[Sequence[BaseMessage], add_messages]` + `remaining_steps` guard | libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:57-62 |
| Prompt assembly | `Prompt` type (str/SystemMessage/Callable/Runnable); default prompt = bare `state["messages"]` | libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:119-170 (:139-142 default) |
| Model-only vs persisted views | `pre_model_hook` contract: return `llm_input_messages` (view only) or rewrite `messages` via `RemoveMessage(id=REMOVE_ALL_MESSAGES)` | libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:396-424 |
| View resolution into model call | `_get_model_input_state` prefers `llm_input_messages` over `messages`; validates history first | libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:636-658 |
| Hook input schema | Dynamic `CallModelInputSchema` adds `llm_input_messages` to the agent node's input schema | libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:723-742 |
| Structured-response context | Separate structured-output call reuses full history, optionally prefixed with tuple-supplied SystemMessage | libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:744-767 |
| History integrity gate | `_validate_chat_history` rejects AIMessage tool_calls lacking ToolMessages before model invocation | libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:243-271 |
| Step-budget signal | `remaining_steps < 2` forces canned final AI message instead of recursion error | libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:434-440,620-634,684-692 |
| State injection annotation | `InjectedState(field)` injects whole state or one field; excluded from model-facing tool schemas | libs/prebuilt/langgraph/prebuilt/tool_node.py:1753-1826 (:1814-1821 notes) |
| Store injection annotation | `InjectedStore()` gives tools persistent cross-session storage; requires store at compile time | libs/prebuilt/langgraph/prebuilt/tool_node.py:1829-1901 |
| Runtime injection | `ToolRuntime` dataclass: state, config, context, store, stream_writer, tool_call_id, tools | libs/prebuilt/langgraph/prebuilt/tool_node.py:1663-1729 |
| Injection resolver + trust boundary | `_inject_tool_args` strips caller/LLM-supplied values for injected keys, then merges trusted values last ("prevents an LLM from forging hidden InjectedToolArg fields") | libs/prebuilt/langgraph/prebuilt/tool_node.py:1315-1430 (:1421-1430 strip+merge) |
| Send-path state hydration | `_extract_state` reads fresh channel values via `CONFIG_KEY_READ` for v2 Send payloads instead of trusting inlined state | libs/prebuilt/langgraph/prebuilt/tool_node.py:1281-1313 |
| Invalid tool name feedback | Unknown tool call → error ToolMessage listing available tools (context repair loop for the model) | libs/prebuilt/langgraph/prebuilt/tool_node.py:1268-1279 (:108-110 template) |
| Error-to-context templates | `handle_tool_errors` converts exceptions into ToolMessage content ("Please fix your mistakes") | libs/prebuilt/langgraph/prebuilt/tool_node.py:108-121,394-439,1002-1008 |
| Retrieval API (long-term memory) | `BaseStore.get/search` with `query`, `filter`, `limit`, `offset`; semantic search opt-in via index config | libs/checkpoint/langgraph/store/base/__init__.py:708-725,756-789 |
| Tracing metadata hygiene | Configurable keys containing "key/token/secret/password/auth" excluded from tracer metadata | libs/langgraph/langgraph/_internal/_config.py:423-447 |
| Trace scrubbing (observability only) | `TracePolicy.process_inputs/process_outputs` can omit/summarize payloads on traces; explicitly "not intended to redact secrets" | libs/langgraph/langgraph/types.py:533-558,561+ |
| Stream redaction lane | v3 stream transformers support `before_builtins` lane for PII redaction before projections snapshot text | libs/langgraph/langgraph/stream/_types.py:94-109; libs/langgraph/langgraph/stream/_mux.py:67-68 |
| Test: injection security | LLM-supplied `auth={"role":"admin"}` is ignored; injected graph-state value wins | libs/prebuilt/tests/test_tool_node.py:2202-2232 |
| Test: subclass stripping | Custom `InjectedToolArg` subclass values supplied by the model are also stripped | libs/prebuilt/tests/test_tool_node.py:2172-2199 |
| Test: optional injected fields | Missing `InjectedState("city")` field injects `None` rather than failing | libs/prebuilt/tests/test_injected_state_not_required.py:1-44,87+ |
| Test: validation-error filtering | Pydantic errors on injected args are filtered from model-facing error messages | libs/prebuilt/tests/test_tool_node_validation_error_filtering.py:42-52 |
| Test: pre-model hook views | `llm_input_messages` view does not alter persisted state; `RemoveMessage(REMOVE_ALL_MESSAGES)` rewrite does | libs/prebuilt/tests/test_react_agent.py:1924-1954 |
| Test: reducer semantics | Remove-by-ID, duplicate removes, REMOVE_ALL_MESSAGES ordering covered | libs/langgraph/tests/test_messages_state.py:86-134,315-332 |

## Answers to Dimension Questions

**1. What decides what goes into context?**
Three decision layers. (a) *Framework:* a node's input schema decides which state channels it sees — compilation wires `PregelNode.channels` to exactly those keys (`libs/langgraph/langgraph/graph/state.py:1515-1547`). (b) *Agent:* the prebuilt ReAct agent passes the entire `messages` list to the model, optionally prefixed with a prompt; the `Prompt` callable/Runnable form lets authors derive arbitrary model inputs from full state (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:121-170`). (c) *Tools:* individual tools declare their own context needs via annotations, resolved at call time (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1315-1430`). Retrieved documents are not auto-injected; they enter only if a node/tool writes them into state or messages.

**2. Is selection policy explicit or implicit?**
Explicit. Selection is declared in type-level artifacts (input schemas at `libs/langgraph/langgraph/graph/state.py:1516-1523`; `Annotated[..., add_messages]` reducers at `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:60`; injection annotations at `libs/prebuilt/langgraph/prebuilt/tool_node.py:1753,1829`) rather than hidden heuristics. However, the *default* policy — pass the whole message history untouched — is implicit and maximal (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:139-142`): without a user hook, nothing is ever dropped.

**3. Can the model influence what it sees?**
Indirectly, yes — through sanctioned loops. Tool calls append `ToolMessage`s that become future context (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:478-483`); invalid tool names produce corrective error ToolMessages listing valid options (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1268-1279,108-110`); tool errors are converted into "Please fix your mistakes" guidance (`libs/prebuilt/langgraph/prebuilt/tool_node.py:111,430`). Direct forgery is blocked: the model cannot supply values for injected parameters — they are stripped and overwritten (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1421-1430`) and are invisible in its tool schema (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1814-1819`). Routing functions can use `Send` to construct custom per-task state, but v2 Send payloads hydrate state from channels server-side rather than trusting inlined snapshots (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1281-1313`).

**4. Are sensitive fields redacted?**
Not from model context. The repo's redaction mechanisms target observability surfaces only: sensitive configurable keys are excluded from tracing metadata (`libs/langgraph/langgraph/_internal/_config.py:423-432`), `TracePolicy` can scrub trace inputs/outputs but explicitly disclaims secret-redaction duties (`libs/langgraph/langgraph/types.py:540-546`), and v3 stream transformers provide a lane for PII redaction of streamed events (`libs/langgraph/langgraph/stream/_types.py:94-109`). Messages flowing to the LLM are never filtered; the repository's threat model states conversation history has "No field-level encryption or redaction" and unbounded retention (`.github/THREAT_MODEL.md:179,396`). The closest model-context safeguard is preventing the *model itself* from injecting forged privileged values (`libs/prebuilt/tests/test_tool_node.py:2202-2232`).

## Architectural Decisions

- **Context equals state; state equals channels.** All model-facing information must live in typed channels, so selection reduces to schema declaration. This makes inclusion auditable from the graph definition alone (`libs/langgraph/langgraph/graph/state.py:1515-1547`, `libs/langgraph/langgraph/pregel/_read.py:102-105`).
- **Append-only history with tombstone deletes.** `add_messages` merges by message ID, supporting upsert and `RemoveMessage` deletion, plus a global `REMOVE_ALL_MESSAGES` sentinel whose trailing messages survive (`libs/langgraph/langgraph/graph/message.py:209-234`). Curation becomes ordinary node output rather than a special API.
- **Split persisted history from model view.** `llm_input_messages` lets a pre-model hook show the model something different from what is checkpointed, avoiding destructive trimming while bounding tokens (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:396-424,636-658`).
- **Capability-scoped tool context.** Tools pull only declared fields (`InjectedState("foo")`) instead of receiving everything, with injection hidden from the model's schema and hardened against argument forgery (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1753-1765,1421-1430`).
- **Retrieval as an opt-in service, not a pipeline.** `BaseStore.search/get` exposes query/filter/limit primitives with semantic search disabled unless configured (`libs/checkpoint/langgraph/store/base/__init__.py:708-725,779-789`) — no implicit RAG.
- **Deprecate-and-migrate for richer policies.** `create_react_agent` is deprecated toward `langchain.agents.create_agent` middleware (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:53-56,274-317`), signaling that trimming/summarization policy now lives outside this repo.

## Notable Patterns

- **Mapper-based projection:** one shared mapper per input schema coerces raw channel dicts into TypedDict/dataclass/Pydantic inputs, cached per schema (`libs/langgraph/langgraph/graph/state.py:1519-1523`).
- **Sentinel-driven bulk delete:** `RemoveMessage(id=REMOVE_ALL_MESSAGES)` returns only messages after the sentinel index — an elegant way to atomically rewrite history (`libs/langgraph/langgraph/graph/message.py:209-213`), special-cased even inside tool `Command` validation where no terminator check is needed (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1544-1546`).
- **Trust-boundary dict merge:** `{**stripped_llm_args, **injected_args}` guarantees system values win collisions (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1424-1429`), verified by dedicated tests (`libs/prebuilt/tests/test_tool_node.py:2202-2232`).
- **Error-as-context:** every failure mode an LLM can cause (bad tool name, bad args, runtime error) round-trips back as structured `ToolMessage(status="error")` content so the next turn can self-correct (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1268-1279,1002-1008`; validation-error filtering for non-LLM-controlled args in `libs/prebuilt/tests/test_tool_node_validation_error_filtering.py:42-52`).
- **Batching-invariant experimental reducer:** `_messages_delta_reducer` replicates dedup/tombstone semantics in one pass for `DeltaChannel`, explicitly documenting which `add_messages` features it drops (`libs/langgraph/langgraph/graph/message.py:247-309`).

## Tradeoffs

- **Simplicity vs safety of the default:** passing full history maximizes fidelity but means any long conversation grows context without bound unless authors write hooks; the framework offers no token accounting (searched for truncation/max-token logic in `libs/prebuilt` and `libs/langgraph`; none found affecting model-bound messages).
- **Delegation vs consistency:** because trimming/summarization is user-authored, two agents in the same org can have wildly different effective context policies; the deprecation to `langchain` middleware centralizes this later but leaves the current repo without a reference implementation (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:397,311-317`).
- **Expressiveness vs auditability of `Prompt` callables:** a Callable prompt receives the full state and can emit anything (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:160-164`), which is flexible but bypasses all schema-level guarantees.
- **Observability-safe defaults vs model-context exposure:** strong filtering exists for traces/metadata (`libs/langgraph/langgraph/_internal/_config.py:423-432`) yet none for prompts, so secrets placed in state reach the provider verbatim.

## Failure Modes / Edge Cases

- **Unknown-ID removal raises:** `RemoveMessage` for a nonexistent ID raises `ValueError`, so replayed or stale deletions fail loudly rather than corrupting history (`libs/langgraph/langgraph/graph/message.py:227-230`).
- **Broken tool-call pairing halts the run:** missing ToolMessage counterparts raise `INVALID_CHAT_HISTORY` before the model call (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:243-271`), and tool `Command` updates must contain a matching terminating ToolMessage (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1557-1578`).
- **Missing injected state fails fast:** a required `InjectedState("field")` on absent state raises `KeyError`/`AttributeError` (optional args excepted) (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1389-1405`); `InjectedStore` without a compiled store raises `ValueError` (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1407-1415`).
- **List-shaped state restricts injection:** tools requesting non-message fields against a pure message-list state raise a descriptive error (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1367-1387`).
- **Step exhaustion substitutes a canned reply:** near the step budget the agent returns "Sorry, need more steps..." instead of recursing (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:620-634,684-692`) — context stays valid but the turn's intent is dropped.
- **Retention/redaction gap:** checkpoints persist full conversation history indefinitely with no TTL by default; flagged in-repo as an accepted risk with GDPR implications (`.github/THREAT_MODEL.md:131-134,396`).

## Future Considerations

- Reintroduce (or point users to the successor library's) built-in summarization/trimming middleware, since the current hooks require every team to hand-roll curation (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:396-424`).
- Add size-aware handling of `ToolMessage` content (cap, chunk, or offload large tool outputs) — currently verbatim (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1441-1443`).
- Extend the observability-only redaction lanes (`libs/langgraph/langgraph/stream/_types.py:94-109`) to model-bound inputs, closing the gap acknowledged in `.github/THREAT_MODEL.md:179`.
- Record inclusion rationale (which channel/schema/hook admitted an item) to make "why was this document in context?" answerable post-hoc; today only structural provenance exists.

## Questions / Gaps

- **Token-budget selection:** No evidence found. Searched `trim_messages`, `summariz`, `truncat`, `max_tokens`, `max_length` across `libs/`; the only hits are docstrings describing hooks (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:397`) and unrelated CLI/store namespace truncation. Any budget enforcement lives outside this repo.
- **PII/sensitive-field redaction for model context:** No evidence found of filtering applied to messages or tool results before provider calls; redaction exists only on trace/metadata/stream surfaces (`libs/langgraph/langgraph/_internal/_config.py:423-432`; `libs/langgraph/langgraph/types.py:540-546`; `libs/langgraph/langgraph/stream/_mux.py:67-68`).
- **Automatic retrieval / RAG pipeline:** None in the core libraries; RAG patterns exist only as example notebooks under `examples/rag/` (e.g., `examples/rag/langgraph_adaptive_rag.ipynb`), i.e., user-space compositions, not framework policy.
- **Inclusion-rationale logging:** No evidence found. Nothing tags why a message/document entered context beyond its producing channel/node.
- **JS parity for these mechanisms:** `libs/sdk-js` targets the REST API rather than local graph construction, so no equivalent client-side selection policy was expected or examined in depth.
- Note: `docs/` contains only redirects and an index (`docs/redirects.json`, `docs/llms.txt`); narrative guidance (memory, summarization how-tos) lives on the external docs site and could not be cited as in-repo evidence.

---

Generated by `Dimension 11.01: Context Selection Policy` against `langgraph`.
