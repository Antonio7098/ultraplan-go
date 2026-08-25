# Source Analysis: langgraph

## Dimension 11.03: Repository and Workspace Context Maps

### Source Info

| Field | Value |
|-------|-------|
| Name | langgraph |
| Path | `studies/agent-harness-study/sources/langgraph` |
| Language / Stack | Python (core `libs/langgraph`, `libs/prebuilt`, `libs/checkpoint*`, `libs/cli`, `libs/sdk-py`); JSON schemas (`libs/cli/schemas/schema.json`); `libs/sdk-js` is a README-only stub |
| Analyzed | 2026-08-25 |

## Summary

LangGraph is a stateful graph-orchestration framework, not a coding agent, and it contains **no repository-map, file-selection, or symbol-indexing machinery for LLM context**. Explicit searches for `repo_map|repomap|code_map|tree-sitter|ctags|symbol_index|file_tree` across the whole source returned zero matches; the only "workspace" hits are uv/Docker build-workspace handling in the CLI (`libs/cli/tests/unit_tests/test_config.py:34-2233`). This absence is architectural, not a gap in a coding harness.

What LangGraph does provide is a well-engineered analogue at the *state* level rather than the *filesystem* level:

1. **Structural self-map of the computation graph** — `Pregel.get_graph()` returns a drawable `Graph` (Mermaid/PNG via langchain-core), assembled by a static-analysis walk over node writers/conditional writes with subgraph "xray" expansion (`libs/langgraph/langgraph/pregel/main.py:845-871`, `libs/langgraph/langgraph/pregel/_draw.py:42-277`). A remote variant fetches the same map from the server via `GET /assistants/{assistant_id}/graph` (`libs/langgraph/langgraph/pregel/remote.py:256-289`).
2. **Explicit construction as the selection policy** — nodes, edges, and conditional edges are registered by hand (`StateGraph.add_node/add_edge/add_conditional_edges`, `libs/langgraph/langgraph/graph/state.py:376,928,982`; compiled via `compile()`, `libs/langgraph/langgraph/graph/state.py:1177`). Nothing auto-discovers files or symbols.
3. **Fresh workspace-state snapshots** — `get_state()` / `get_state_history()` read current and historical channel values from the checkpointer on every call (`libs/langgraph/langgraph/pregel/main.py:1392-1435,1480-1534`), materialized into `StateSnapshot` values/next/tasks/interrupts (`libs/langgraph/langgraph/types.py:683-707`).
4. **Incremental state representation** — checkpoints carry monotonic `channel_versions`, per-node `versions_seen`, and `updated_channels` (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:92-127`), and the beta `DeltaChannel` stores only deltas plus periodic snapshots, replaying ancestor writes through the reducer (`libs/langgraph/langgraph/channels/delta.py:25-64,139-157`).
5. **Semantic retrieval for long-term memory** — `BaseStore.search()` with `IndexConfig` (embedding dims/provider/fields) lets an agent find relevant memory items by natural-language query without being told their key — the closest analog to "find the right file without being told the path," but scoped to store items, not repo files (`libs/checkpoint/langgraph/store/base/__init__.py:779-800,578-620`).
6. **Deployment-time file selection** — the CLI resolves graph/auth/checkpointer entry points from explicit paths in `langgraph.json` (`$defs.GraphDef {path, description}`, `libs/cli/schemas/schema.json`; validation at `libs/cli/langgraph_cli/config.py:323`) and filters Docker build contexts through `.dockerignore` (`libs/cli/langgraph_cli/_ignore.py`, exercised at `libs/cli/tests/unit_tests/test_config.py:1940-1942`).

The dimension's headline question ("Can the model find the right file without being told the path?") is answered **no by design**: any repo/file awareness must be supplied by agents built on top of LangGraph (e.g., via retrieval tools or the Store).

## Rating

**3 / 10** — For this dimension's core subject matter (repo maps, file scoring for model context, symbol indexing), LangGraph scores in the bottom band: absent and implicit. The score sits at the top of the band because the adjacent *workspace-state* machinery that the dimension also probes (workspace summarization, incremental map updates) exists in mature form: versioned channels, snapshot/history APIs with pending-write overlays, delta-based incremental checkpointing with bounded replay depth, a checkpointer conformance test suite, and opt-in semantic search — all implemented with tests and explicit interfaces (`libs/langgraph/tests/test_delta_channel_*.py`, `libs/checkpoint-conformance/tests/test_validate_memory.py:16-20`). None of it maps a repository, so it cannot score in the 7-8 band.

## Evidence Collected

Every entry cites `path/to/file.py:NN` relative to `studies/agent-harness-study/sources/langgraph`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Repo map generator | No evidence found. Searched `repo_map\|repomap\|code_map\|tree-sitter\|ctags\|symbol_index\|file_tree` across all libs — zero matches | N/A |
| Structural graph map generator | `draw_graph()` simulates the Pregel loop statically (input writes → task preparation → static `ChannelWrite.get_static_writes` → edge discovery) to assemble a `langchain_core.runnables.graph.Graph` | `libs/langgraph/langgraph/pregel/_draw.py:42-277` |
| Static-analysis step cap | drawing loop bounded by `limit: int = 250` supersteps to terminate conditional-edge discovery | `libs/langgraph/langgraph/pregel/_draw.py:53,118` |
| Public map API | `Pregel.get_graph(config, *, xray)` — "Return a drawable representation of the computation graph"; xray recurses into subgraphs with depth countdown | `libs/langgraph/langgraph/pregel/main.py:845-871` |
| Protocol contract | `get_graph`/`aget_graph(xray: int \| bool)` declared on `PregelProtocol` returning `langchain_core.runnables.graph.Graph` | `libs/langgraph/langgraph/pregel/protocol.py:32-45` |
| Subgraph namespace enumeration | `get_subgraphs(namespace=..., recurse=...)` yields `(name, subgraph)` pairs joined with `NS_SEP` | `libs/langgraph/langgraph/pregel/main.py:1076-1112` |
| Remote graph map | remote `Pregel` builds the drawable graph from server response of `GET /assistants/{assistant_id}/graph` with `xray` passthrough | `libs/langgraph/langgraph/pregel/remote.py:256-289` |
| Map construction API | `add_node` (multiple typed overloads), `add_edge`, `add_conditional_edges`, `compile` | `libs/langgraph/langgraph/graph/state.py:376-667,928,982,1177` |
| Map rendering tests | mermaid snapshots incl. xray expansion, e.g. `app.get_graph().draw_mermaid(with_styles=False) == snapshot`; `graph.get_graph(xray=True).draw_mermaid()` | `libs/langgraph/tests/test_pregel.py:1754,3135`; `libs/langgraph/tests/test_large_cases.py:589,6530` |
| Workspace-state snapshot type | `StateSnapshot(values, next, config, metadata, created_at, parent_config, tasks, interrupts)` | `libs/langgraph/langgraph/types.py:683-707` |
| Fresh state reads | `get_state` fetches `checkpointer.get_tuple(config)` per call (no caching layer); subgraph dispatch via `recast_checkpoint_ns` | `libs/langgraph/langgraph/pregel/main.py:1392-1435` |
| State history + filtering | `get_state_history(filter=..., before=..., limit=...)` walks `checkpointer.list(...)` snapshots | `libs/langgraph/langgraph/pregel/main.py:1480-1533` |
| Snapshot assembly w/ pending writes | `_prepare_state_snapshot` migrates checkpoint, rehydrates channels, applies `pending_writes` overlay | `libs/langgraph/langgraph/pregel/main.py:1145-1185` |
| Checkpoint as versioned context record | `Checkpoint(v, id, ts, channel_values, channel_versions, versions_seen, updated_channels)`; `versions_seen`: "map from node ID to ... versions of the channels that each node has seen" | `libs/checkpoint/langgraph/checkpoint/base/__init__.py:92-127` |
| Version bumping (incremental update logic) | `apply_writes` bumps `channel_versions[chan] = next_version` only for updated channels and computes `updated_channels`; `versions_seen` updated per task trigger | `libs/langgraph/langgraph/pregel/_algo.py:262-269,316-345` |
| Version allocator | `BaseCheckpointSaver.get_next_version(current, channel)` abstract hook | `libs/checkpoint/langgraph/checkpoint/base/__init__.py:692` |
| Delta-only state representation | `DeltaChannel` stores sentinel `MISSING` in blobs; reconstructs by replaying ancestor writes via reducer; batching-invariance requirement documented | `libs/langgraph/langgraph/channels/delta.py:25-64,193-202` |
| Delta replay | `replay_writes(writes)` folds oldest→newest with overwrite-reset semantics | `libs/langgraph/langgraph/channels/delta.py:139-157` |
| Snapshot cadence predicate | `delta_channels_to_snapshot` fires when updates ≥ `snapshot_frequency` (default 1000) or supersteps ≥ `DELTA_MAX_SUPERSTEPS_SINCE_SNAPSHOT` (default 5000) | `libs/langgraph/langgraph/pregel/_checkpoint.py:50-73`; `libs/langgraph/langgraph/channels/delta.py:62-63` |
| Delta history API + prune hazards | `DeltaChannelHistory(writes, seed)`; `delete_thread`/`prune` docstrings warn custom savers must be DeltaChannel-aware or reconstruction breaks | `libs/checkpoint/langgraph/checkpoint/base/__init__.py:149-166,320-412,582` |
| Delta-channel tests | dedicated suites for supersteps bound, exit mode, id stability, migration, update_state, benchmark | `libs/langgraph/tests/test_delta_channel_supersteps_bound.py:133-146` et al. |
| Message-history compaction primitives | `REMOVE_ALL_MESSAGES = "__remove_all__"` sentinel; `add_messages` merges by ID (append-or-overwrite) | `libs/langgraph/langgraph/graph/message.py:38,61-100` |
| Context-compaction hook (prebuilt agent) | `pre_model_hook` docstring: "Useful for managing long message histories (e.g., message trimming, summarization)" via `RemoveMessage(id=REMOVE_ALL_MESSAGES)` or `llm_input_messages`; wired as node before `agent` | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:396-419,795-798` |
| Semantic search over memory items | `BaseStore.search(namespace_prefix, *, query, filter, limit, offset, refresh_ttl)` — natural-language query support | `libs/checkpoint/langgraph/store/base/__init__.py:779-800` |
| Embedding index config | `IndexConfig(dims, embed, fields)` — provider strings like `"openai:text-embedding-3-small"`; without index, vector args ignored | `libs/checkpoint/langgraph/store/base/__init__.py:578-660` |
| Deployment-time file selection | `validate_config` resolves graph entry points; `$defs.GraphDef {description, path}` schema; `.dockerignore` honored when copying sources | `libs/cli/langgraph_cli/config.py:323`; `libs/cli/schemas/schema.json` ($defs); `libs/cli/tests/unit_tests/test_config.py:1940-1942` |
| Checkpointer conformance suite | `checkpointer_test` harness asserts savers pass base capability tests (incl. DeltaChannel history) | `libs/checkpoint-conformance/tests/test_validate_memory.py:16-20` |

## Answers to Dimension Questions

1. **Does the system build a map of the repository?**
   No. No evidence found after searching for repomap/code-map/tree-sitter/ctags/symbol-index constructs across all libraries. The only "map" LangGraph builds is a structural self-map of its own computation graph: `draw_graph` replays the static write/trigger structure into a drawable `Graph` (`libs/langgraph/langgraph/pregel/_draw.py:42-277`), exposed via `get_graph(xray=...)` (`libs/langgraph/langgraph/pregel/main.py:845`) and rendered as Mermaid/PNG downstream (tests pin output at `libs/langgraph/tests/test_pregel.py:1754`). The remote client mirrors this from the server API (`libs/langgraph/langgraph/pregel/remote.py:256-289`).

2. **How are relevant files selected?**
   Not applicable for model context — there is no file scorer or selector. Two narrow analogues exist: (a) developers explicitly register code objects as graph nodes/edges (`libs/langgraph/langgraph/graph/state.py:376,928,982`), so "selection" is entirely human-authored at build time; (b) deployment tooling selects files by explicit config paths (`$defs.GraphDef.path` in `libs/cli/schemas/schema.json`, validated at `libs/cli/langgraph_cli/config.py:323`) and prunes build contexts using `.dockerignore` rules (`libs/cli/langgraph_cli/_ignore.py`; behavior pinned in `libs/cli/tests/unit_tests/test_config.py:1988-2033`). Neither feeds an LLM prompt.

3. **Are symbols indexed for the model?**
   No code-symbol indexing anywhere. The nearest mechanisms are: JSON Schemas generated for graph input/output state (`CompiledStateGraph.get_input_jsonschema/get_output_jsonschema`, `libs/langgraph/langgraph/graph/state.py:1424-1434`), which describe the data contract rather than program symbols; and the Store's embedding index for memory items (`IndexConfig`, `libs/checkpoint/langgraph/store/base/__init__.py:578-620`) queried via natural language (`search(query=...)`, `libs/checkpoint/langgraph/store/base/__init__.py:779-800`). These let a model retrieve *state/memory*, not *code locations*.

4. **Is workspace context stale or fresh?**
   Fresh-by-construction for graph state: every `get_state`/`get_state_history` call hits the checkpointer storage layer (`checkpointer.get_tuple(config)`, `libs/langgraph/langgraph/pregel/main.py:1428`; `checkpointer.list(...)`, `libs/langgraph/langgraph/pregel/main.py:1527`) with no snapshot cache. Consistency between "what a node has consumed" and "what changed" is tracked exactly via monotonically increasing `channel_versions` vs. per-node `versions_seen` (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:109-122`; update sites at `libs/langgraph/langgraph/pregel/_algo.py:262-269,320-334`). Uncommitted in-flight results surface as `pending_writes` overlays on snapshots (`libs/langgraph/langgraph/pregel/main.py:1433,1145-1185`). For delta-backed channels, staleness is bounded structurally: full-value snapshots are forced every ≤5000 supersteps even if writes stop (`libs/langgraph/langgraph/channels/delta.py:50-55`; predicate at `libs/langgraph/langgraph/pregel/_checkpoint.py:50-73`).

## Architectural Decisions

- **Delegate rendering, own topology.** The drawable `Graph` type and Mermaid/PNG emitters come from `langchain-core` (`from langchain_core.runnables.graph import Graph, Node`, `libs/langgraph/langgraph/pregel/_draw.py:8`); LangGraph owns only edge/topology discovery. This keeps the map format stable outside the framework but means map-quality bugs can live in an unexamined dependency.
- **Simulate-to-draw.** Conditional edges are discovered by executing task writers against an empty checkpoint with input `{}` and collecting static writes (`libs/langgraph/langgraph/pregel/_draw.py:88-151`), capped at 250 steps (`_draw.py:53`). Deterministic but inherently conservative: branches guarded by runtime state may not appear.
- **Versioned channels instead of diffing text.** Incrementality is modeled as monotonic channel versions plus per-node seen-sets (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:109-122`), driving both scheduling and what appears in new checkpoints.
- **Deltas with periodic anchors (beta).** `DeltaChannel` trades blob size for replay cost: non-snapshot steps omit the channel value entirely (`checkpoint() -> MISSING`, `libs/langgraph/langgraph/channels/delta.py:193-202`) and reconstruction walks ancestor writes via `get_delta_channel_history` (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:582`; result shape `DeltaChannelHistory` at `:149-166`).
- **Opt-in semantic indexing.** Vector search activates only when `index` is configured; otherwise put-index arguments are ignored (`libs/checkpoint/langgraph/store/base/__init__.py:578-583`) — no hidden embedding dependency.
- **Explicit-path deployment registry.** Graph/auth/checkpointer/UI entry points must be declared in `langgraph.json` and are validated up front (`libs/cli/langgraph_cli/config.py:323,827-1056`) rather than discovered by scanning.

## Notable Patterns

- **Self-describing artifacts:** compiled graphs expose their own map, schemas (`libs/langgraph/langgraph/graph/state.py:1424-1434`), and subgraph tree (`libs/langgraph/langgraph/pregel/main.py:1076-1112`) — tooling like Studio consumes these without reimplementing traversal (server endpoint mirrored in `libs/langgraph/langgraph/pregel/remote.py:256-289`).
- **Snapshot tests as spec:** Mermaid renderings are pinned byte-for-byte in dozens of cases including xray expansions (`libs/langgraph/tests/test_pregel.py:3090-3173`, `libs/langgraph/tests/test_large_cases.py:589,6530`), making map regressions visible.
- **Pending-writes overlay:** `CheckpointTuple.pending_writes` (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:139-146`) lets `StateSnapshot` show uncommitted results without mutating stored checkpoints (`libs/langgraph/langgraph/pregel/main.py:1145-1185`).
- **Conformance extraction:** saver expectations (including DeltaChannel-aware pruning) are codified in a separate conformance package run against each implementation (`libs/checkpoint-conformance/tests/test_validate_memory.py:16-20`), not just ad-hoc unit tests.
- **Reducer batching-invariance contract:** `reducer(reducer(state, xs), ys) == reducer(state, xs + ys)` is stated as a hard requirement so replay batches need not match original write batches (`libs/langgraph/langgraph/channels/delta.py:41-48`).

## Tradeoffs

- **Framework-level vs harness-level context:** LangGraph gives precise, versioned control over *conversation/task state*, but delegates all *repository* awareness to application code; teams building coding agents must add their own repo-map/RAG layer on top.
- **Delta compression vs read cost:** storing sentinels shrinks checkpoint blobs but makes every read of a delta channel pay an ancestor-walk + reducer fold (`libs/langgraph/langgraph/channels/delta.py:26-39,139-157`); snapshot cadence knobs (1000 updates / 5000 supersteps, `delta.py:54,62-63`) trade blob size against replay depth, and a benchmark suite exists to tune this (`libs/langgraph/tests/test_delta_channel_benchmark.py:1-83`).
- **Static map fidelity vs termination:** the 250-step simulation cap guarantees `get_graph()` terminates on loopy graphs but can under-report edges reachable only after many iterations (`libs/langgraph/langgraph/pregel/_draw.py:53,118`).
- **Explicitness vs discoverability:** requiring `langgraph.json` paths avoids wrong-file surprises at deploy time, at the cost of zero automatic project introspection.
- **Semantic search power vs infra burden:** natural-language retrieval needs embedding dims/providers configured per deployment (`libs/checkpoint/langgraph/store/base/__init__.py:584-620`); without it, only filter/limit search remains.

## Failure Modes / Edge Cases

- **DeltaChannel destruction by retention ops:** `prune`/`delete_thread` can permanently break delta reconstruction if ancestor rows are removed without keeping DeltaChannel-aware anchors; docstrings instruct either implementing `get_delta_channel_history` correctly or skipping pruning for such threads (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:340-412,551`).
- **Custom savers silently breaking beta features:** implementations lacking `get_delta_channel_history` degrade delta channels (no seed ⇒ start-empty semantics, `libs/checkpoint/langgraph/checkpoint/base/__init__.py:149-166,387-407`).
- **Overwrite races in delta channels:** more than one Overwrite value per super-step raises `InvalidUpdateError` (`libs/langgraph/langgraph/channels/delta.py:163-171`); replay handles a single trailing Overwrite as a reset point (`delta.py:150-155`).
- **Map blind spots:** conditional edges taken only under real inputs won't appear in the empty-input static simulation; deferred nodes get special edge stitching (`libs/langgraph/langgraph/pregel/_draw.py:192-216`).
- **Unknown-channel writes dropped:** tasks writing to undeclared channels are ignored with a warning during apply_writes (`libs/langgraph/langgraph/pregel/_algo.py:308-313`) — silent context loss if schemas drift.
- **Beta-surface instability:** the entire delta surface (blob shape, metadata field `counters_since_delta_snapshot`, bound env `LANGGRAPH_DELTA_MAX_SUPERSTEPS_SINCE_SNAPSHOT`) is flagged as changeable (`libs/langgraph/langgraph/channels/delta.py:29-36`; `libs/checkpoint/langgraph/checkpoint/base/__init__.py:60-76`).

## Future Considerations

- Stabilize and document the DeltaChannel contract (snapshot cadence, prune interplay) before recommending it for large-state workloads; today it is explicitly beta (`libs/langgraph/langgraph/channels/delta.py:29-36`).
- Expose richer map metadata (per-edge triggers, managed channels) through `get_graph` output for better observability tooling; currently only node metadata like `__interrupt`/`defer` is attached (`libs/langgraph/langgraph/pregel/_draw.py:218-231`).
- Consider a first-class "context view" API combining `get_state`, `get_state_history`, and Store search, since consumers currently must orchestrate three separate surfaces (`libs/langgraph/langgraph/pregel/main.py:1392,1480`; `libs/checkpoint/langgraph/store/base/__init__.py:779`).
- Teams needing true repository/workspace maps should treat this dimension's capability as an application-layer concern (tools + Store-backed indexes), not expect framework support.

## Questions / Gaps

- **Rendering dependency out of scope:** `draw_mermaid`/`draw_ascii`/PNG generation live in `langchain-core`, not this source; only usage sites and snapshots were verifiable here (`libs/langgraph/langgraph/pregel/main.py:915-921`, `libs/langgraph/tests/test_pregel.py:1754`).
- **Server-side graph endpoint internals** (`GET /assistants/{assistant_id}/graph`) are implemented in the closed-source server; this source shows only the client mirror (`libs/langgraph/langgraph/pregel/remote.py:256-289`), so server-side caching/freshness of the map could not be assessed.
- **JS-side parity unverifiable:** `libs/sdk-js` contains only `README.md` (no implementation files found), so whether JS clients expose equivalent map/state APIs could not be confirmed from this source.
- **No evidence found** for any token-budgeted or relevance-scored file/context selection (adjacent dimension territory): searches for ranking/scoring machinery surfaced only store-search pagination and unrelated CLI code.
- **In-repo documentation is minimal:** `docs/` holds redirect/llms.txt plumbing (`sources/langgraph/docs/llms.txt`), so documentation-level claims about visualization could not be tied back beyond code and examples.

---

Generated by `dimensions/11.03-repository-and-workspace-context-maps.md` against `langgraph`.
