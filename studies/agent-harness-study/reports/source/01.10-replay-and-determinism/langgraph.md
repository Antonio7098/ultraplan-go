# Source Analysis: langgraph

## 01.10 Replay and Determinism in Execution

### Source Info

| Field | Value |
|-------|-------|
| Name | langgraph |
| Path | `studies/agent-harness-study/sources/langgraph` |
| Language / Stack | Python (LangGraph monorepo: `libs/checkpoint`, `libs/checkpoint-postgres`, `libs/langgraph`, `libs/checkpoint-conformance`) |
| Analyzed | 2026-07-03 |

## Summary

LangGraph implements replay as a first-class execution primitive, not a side effect. Replay is triggered when a caller supplies `config.configurable.checkpoint_id` (i.e. `CONFIG_KEY_CHECKPOINT_ID`) into `Pregel.__call__`. The `PregelLoop.__init__` sets `self.is_replaying = CONFIG_KEY_CHECKPOINT_ID in config[CONF]` (`libs/langgraph/langgraph/pregel/_loop.py:308`), and that flag steers subsequent state hydration, pending-write re-application, and the optional `ReplayState` mechanism for subgraphs (`libs/langgraph/langgraph/_internal/_replay.py:14-90`). On time travel (`graph.invoke(None, before_b.config)`), the saved checkpoint tuple is loaded exactly, channels are rehydrated (DeltaChannels may reconstruct by replaying writes via `get_delta_channel_history`), `_reapply_writes_to_succeeded_nodes` (`libs/langgraph/langgraph/pregel/_loop.py:724-737`) restores successful writes from `checkpoint_pending_writes`, `_resume_error_handlers_if_applicable` (`libs/langgraph/langgraph/pregel/_loop.py:739-804`) re-runs the error-handler plumbing, and any pending tasks with empty writes re-execute. Replay is explicitly NOT a LLM-response cache: `CachePolicy` (`libs/langgraph/langgraph/pregel/_algo.py:663-687`) caches per-node writes keyed by an `identifier(proc)` + args key — useful but orthogonal to checkpoint replay. Determinism for the workflow is achieved through deterministic task-id hashing (`_xxhash_str`/`_uuid5_str` in `libs/langgraph/langgraph/pregel/_algo.py:1395-1409`), monotonic `uuid6` checkpoint ids (`libs/checkpoint/langgraph/checkpoint/base/id.py:79-109`), strictly sorted task iteration (`tasks = sorted(tasks, key=lambda t: task_path_str(t.path[:3]))` at `_algo.py:256`), and a deterministic version-comparison triggering model. LLMs themselves are not recorded; on replay they are called again. Tested at scale in `test_time_travel.py` (100+ tests) and `test_time_travel_async.py`, with replay cross-checked against the conformance harness `libs/checkpoint-conformance/langgraph/checkpoint/conformance/spec/test_get_tuple.py`.

## Rating

**9 / 10** — Clear replay model with deterministic checkpoint lineage, versioned writes, and an explicit ReplayState for subgraph fork semantics. Replay is observable (fork checkpoints, append-only history), replayable under failure (id stability, stub creation for exit-mode delta writes), and extensible (BaseCheckpointSaver contract + Postgres two-stage `get_delta_channel_history`). The single deductive item is that LLM outputs themselves are never recorded; replay re-invokes models. That is a design choice (frames "replay" as workflow replay, not model-output replay), not a defect, and it is exposed via `CachePolicy` so users can opt into per-node result caching when they want deterministic model outputs.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Replay trigger | `self.is_replaying = CONFIG_KEY_CHECKPOINT_ID in config[CONF]` in `PregelLoop.__init__` | `libs/langgraph/langgraph/pregel/_loop.py:308` |
| Replay trigger (fork path) | `is_time_traveling = self.is_replaying and ( … )` then `_put_checkpoint({"source": "fork"})` to branch | `libs/langgraph/langgraph/pregel/_loop.py:866-959` |
| Checkpoint hydration | `saved = self.checkpointer.get_tuple(self.checkpoint_config)` and `saved = CheckpointTuple(…, empty_checkpoint(), {"step": -2}, None, [])` fallback | `libs/langgraph/langgraph/pregel/_loop.py:1606-1686` |
| Subgraph replay state | `ReplayState` shares checkpoint_id across subgraph invocations; `_is_first_visit` strips task suffix so loops re-detect the same subgraph | `libs/langgraph/langgraph/_internal/_replay.py:14-90` |
| Subgraph replay wiring | `CONFIG_KEY_REPLAY_STATE: replay_state` propagated only when `is_time_traveling` | `libs/langgraph/langgraph/pregel/_loop.py:1019-1057` |
| Re-apply cached writes | `_reapply_writes_to_succeeded_nodes` restores writes to in-memory tasks unless control signals ERROR / INTERRUPT / RESUME | `libs/langgraph/langgraph/pregel/_loop.py:724-737` |
| Error handler resume | `_resume_error_handlers_if_applicable` scans `checkpoint_pending_writes` for ERROR_SOURCE_NODE, replays the handler task | `libs/langgraph/langgraph/pregel/_loop.py:739-804` |
| Replay flag scope | `self.is_replaying = False` reset after the first tick so subsequent ticks run normally | `libs/langgraph/langgraph/pregel/_loop.py:704` |
| Replay skips re-execution of done tasks | `if not self.is_replaying and self.checkpoint_pending_writes:` gates pending-write replay | `libs/langgraph/langgraph/pregel/_loop.py:655` |
| Append-only lineage | New fork checkpoint after time travel; original chain preserved — asserted in `test_replay_creates_branch_preserving_old_checkpoints` | `libs/langgraph/tests/test_time_travel.py:2985-3109` |
| Append-only lineage (subgraph) | `test_replay_creates_branch_in_subgraph` | `libs/langgraph/tests/test_time_travel.py:3112-3204` |
| Replay re-fires interrupts | `test_replay_from_before_interrupt_refires`: ask_human call_count goes 1→2→3 across original+resume+replay | `libs/langgraph/tests/test_time_travel.py:226-282` |
| Replay-after-interrupt branch | `test_replay_from_before_interrupt_then_resume`: explicit assertion of `("fork", ("ask_human",))` checkpoint after replay | `libs/langgraph/tests/test_time_travel.py:285-392` |
| Stable interrupt across replay | `test_replay_interrupt_stable_across_replays` (3 replays of same checkpoint produce equivalent values) | `libs/langgraph/tests/test_time_travel.py:395-442` |
| Subgraph time travel | `test_subgraph_time_travel_*` (40+ tests in `test_time_travel_async.py` 597-1639) | `libs/langgraph/tests/test_time_travel_async.py:597-1639` |
| Deterministic task id (≥v2) | `task_id_func = _xxhash_str if checkpoint["v"] > 1 else _uuid5_str` | `libs/langgraph/langgraph/pregel/_algo.py:550` |
| Deterministic task id (≤v1) | `_uuid5_str(namespace, *parts)` from SHA-1 | `libs/langgraph/langgraph/pregel/_algo.py:1395-1401` |
| Modern task id hash | `_xxhash_str(namespace, *parts)` from XXH3-128 | `libs/langgraph/langgraph/pregel/_algo.py:1404-1409` |
| Deterministic checkpoint id | `uuid6(clock_seq=step)` — time-ordered, monotonic per saver instance via `_last_v6_timestamp` | `libs/checkpoint/langgraph/checkpoint/base/id.py:79-109` |
| Deterministic UUID ordering | `for checkpoint_id, … in sorted(self.storage[thread_id][checkpoint_ns].items(), key=lambda x: x[0], reverse=True)` in `InMemorySaver.list` | `libs/checkpoint/langgraph/checkpoint/memory/__init__.py:353-361` |
| Deterministic Postgres list order | `ORDER BY checkpoint_id DESC` in PostgresSaver | `libs/checkpoint-postgres/langgraph/checkpoint/postgres/__init__.py:153` |
| Deterministic task iteration | `tasks = sorted(tasks, key=lambda t: task_path_str(t.path[:3]))` in `apply_writes` | `libs/langgraph/langgraph/pregel/_algo.py:256` |
| Deterministic channel ordering | `updated_channels=None if updated_channels is None else sorted(updated_channels)` in `create_checkpoint` | `libs/langgraph/langgraph/pregel/_checkpoint.py:120` |
| Deterministic node trigger ordering | `candidate_nodes: Iterable[str] = sorted(triggered_nodes)` in `prepare_next_tasks` | `libs/langgraph/langgraph/pregel/_algo.py:482` |
| Deterministic writes sort key | `for (_, _), (tid, ch, serialized, _) in sorted(step_writes.items(), reverse=True)` | `libs/checkpoint/langgraph/checkpoint/memory/__init__.py:202-205` |
| DeltaChannel write replay | `DeltaChannel.replay_writes` folds ancestor writes through reducer; ordering invariant documented | `libs/langgraph/langgraph/channels/delta.py:139-157` |
| DeltaChannel from seed | `from_checkpoint` for `_DeltaSnapshot` returns value directly (no replay) | `libs/langgraph/langgraph/channels/delta.py:118-137` |
| DeltaChannel history contract | `BaseCheckpointSaver.get_delta_channel_history` walks parent chain returning per-channel `{writes, seed}` | `libs/checkpoint/langgraph/checkpoint/base/__init__.py:582-649` |
| DeltaChannel hydration | `channels_from_checkpoint` batches all delta-needing channels into a single saver call | `libs/langgraph/langgraph/pregel/_checkpoint.py:136-184` |
| Postgres delta history (2-stage query) | `_build_delta_stage1_sql` + `_build_delta_stage2_sql` (page scan + per-channel UNION ALL) | `libs/checkpoint-postgres/langgraph/checkpoint/postgres/__init__.py:444-550` |
| Delta snapshot blob | `_DeltaSnapshot(value)` typed sentinel stored as msgpack ext | `libs/checkpoint/langgraph/checkpoint/serde/types.py:19-31` |
| LLM call recording (none) | Pregel never records model responses; `CachePolicy` is opt-in per-node writes cache keyed by `args_key` | `libs/langgraph/langgraph/pregel/_algo.py:663-687, 858-870, 1018-1034` |
| Replay verifies state, not history | `test_replay_interrupt_stable_across_replays` compares values, not LLM tokens | `libs/langgraph/tests/test_time_travel.py:395-442` |
| Failure mode: `_needs_replay` | `_DeltaSnapshot` and plain values resolve directly; only `MISSING` triggers ancestor walk | `libs/langgraph/langgraph/pregel/_checkpoint.py:124-133` |
| Failure mode: deep migration | `test_time_travel_into_pre_migration_checkpoint` — pre-delta ancestors return correct state | `libs/langgraph/tests/test_delta_channel_migration.py:197-251` |
| Failure mode: prune | `BaseCheckpointSaver.prune` documents that naive `keep_latest` can silently corrupt DeltaChannel | `libs/checkpoint/langgraph/checkpoint/base/__init__.py:374-415` |
| Stub for exit-mode replay | `_put_exit_delta_writes` lazy-creates stub checkpoint so writes are anchored on a persisted parent (or stub on first run) | `libs/langgraph/langgraph/pregel/_loop.py:1201-1292` |
| Write durability ordering | `_delta_write_futs` drained before next `_checkpointer_put_after_previous` so checkpoint is never visible without its backing writes | `libs/langgraph/langgraph/pregel/_loop.py:1515-1524, 1769-1778` |
| Save protocol | `BaseCheckpointSaver.put / put_writes / get_tuple / list / delete_thread`; channel_versions + versions_seen + pending_sends encoded in Checkpoint dataclass | `libs/checkpoint/langgraph/checkpoint/base/__init__.py:92-329` |
| Conformance harness | `run_get_tuple_tests`, `run_put_tests`, etc. — capability-detected, per-saver | `libs/checkpoint-conformance/langgraph/checkpoint/conformance/validate.py:45-128` |
| Conformance capability detection | `DetectedCapabilities.from_instance` introspects method overrides vs `BaseCheckpointSaver` | `libs/checkpoint-conformance/langgraph/checkpoint/conformance/capabilities.py:66-96` |
| Conformance round-trip | `test_put_roundtrip` writes then reads identical checkpoint via `aget_tuple` | `libs/checkpoint-conformance/langgraph/checkpoint/conformance/spec/test_put.py:39-51` |
| Conformance list ordering | `test_list_ordering` verifies `ORDER BY checkpoint_id DESC` | `libs/checkpoint-conformance/langgraph/checkpoint/conformance/spec/test_list.py:97-…` |
| Determinism test fixture | `deterministic_uuids` mocks `uuid.uuid4` to a fixed sequence for snapshot tests | `libs/langgraph/tests/conftest.py:47-52` |
| Snapshot tests use deterministic UUIDs | `test_message_graph` and `test_remote_graph` snapshot messages, IDs, etc. | `libs/langgraph/tests/test_large_cases.py:2379-…, 3099-…` |

## Answers to Dimension Questions

1. **Can execution be replayed?** Yes. Setting `config["configurable"]["checkpoint_id"]` makes `PregelLoop` rehydrate from that checkpoint (`libs/langgraph/langgraph/pregel/_loop.py:308`, `:1606-1686`) and continue forward. Replay is the mechanism underlying "time travel" (`libs/langgraph/langgraph/pregel/_loop.py:866-884`) and resume-after-interrupt (the same code path is reused when `CONFIG_KEY_RESUMING` is set, distinguished by `is_time_traveling`).

2. **What exactly is replayed?** Workflow state only: the channel values, channel versions, versions_seen map, pending writes (linked to task IDs), and the parent checkpoint lineage (encoded via `parent_checkpoint_id` in the saver and `CONFIG_KEY_CHECKPOINT_MAP` for subgraphs). Model call outputs are NOT replayed — they are re-issued unless the user opts into `CachePolicy` (`libs/langgraph/langgraph/pregel/_algo.py:663-687, 858-870, 1018-1034`). For DeltaChannels, only the seed (`_DeltaSnapshot` blob or pre-migration plain value) and the on-path ancestor writes are replayed through the reducer; off-path writes (sibling subgraph branches, pruned ancestors) are skipped (`libs/langgraph/langgraph/_internal/_replay.py:34-50`, `libs/langgraph/langgraph/channels/delta.py:139-157`, `libs/checkpoint/langgraph/checkpoint/base/__init__.py:582-649`).

3. **Are LLM calls replayed from logs or re-called?** Re-called. Pregel does not record model responses. Per-node result caching is an explicit, opt-in `CachePolicy` (`libs/langgraph/langgraph/pregel/types.py:CachePolicy` referenced from `_algo.py:668-687`). When `CachePolicy.ttl` is set and `cache_policy.key_func(val)` returns the same key for an unchanged input, the cached `task.writes` are re-applied without re-invoking the runnable (`libs/langgraph/langgraph/pregel/_loop.py:1526-1539, 1780-1793`). Otherwise, `runner.tick` re-invokes the runnable on every replay (`libs/langgraph/langgraph/pregel/main.py:2980-2998`).

4. **Does replay verify correctness or only reconstruct state?** Only reconstruct state. There is no replay-time assertion comparing past outputs to current outputs. The closest thing is the stable-interrupt test (`libs/langgraph/tests/test_time_travel.py:395-442`), which uses value equality across replays but only because the underlying model is a deterministic function. Replay is a **resumption** primitive: load state, restore successful writes, then re-execute un-resumed tasks. Verification of "did the model produce the same answer?" is left to the user.

5. **What non-determinism remains?** Three sources:
   - **LLM stochasticity.** Without `CachePolicy`, model tokens vary across replays.
   - **`uuid.uuid4` for non-checkpoint IDs.** The `deterministic_uuids` test fixture explicitly mocks it (`libs/langgraph/tests/conftest.py:47-52`), and `_messages.py:431-432` warns that `id=None` BaseMessages can produce different UUIDs on each replay. The fix lives in `PregelLoop.put_writes` which calls `ensure_message_ids` on DeltaChannel writes before the background thread serialises them (`libs/langgraph/langgraph/pregel/_loop.py:451-458`).
   - **Random salts in next-version.** `InMemorySaver.get_next_version` and `PostgresSaver.get_next_version` add a random hash component to the version string (`libs/checkpoint/langgraph/checkpoint/memory/__init__.py:619-628`, `libs/checkpoint-postgres/langgraph/checkpoint/postgres/base.py:543-552`). Because ordering is by the prefix integer, the random suffix does not affect replay correctness, but two replays produce different checkpoint_ids for identical state.
   - **Determinism in the `get_next_version` for delta writes ordering.** The set `DEFAULT_RUNTIME` ordering is fixed; `TaskIDFn` is `xxhash` of `(checkpoint_id_bytes, namespace, step, name, kind, …parts)` (`libs/langgraph/langgraph/pregel/_algo.py:1404-1409`), which is fully deterministic given a fixed checkpoint lineage.

## Architectural Decisions

- **Replay = resumption, not re-execution.** Nodes that already finished are skipped via `_reapply_writes_to_succeeded_nodes` (`libs/langgraph/langgraph/pregel/_loop.py:724-737`); nodes that haven't written yet are re-executed. Error / interrupt / resume control writes are intentionally NOT re-applied, so failed tasks re-run and interrupt handlers re-fire (`libs/langgraph/langgraph/pregel/_loop.py:734`, `:739-804`).

- **Append-only checkpoint lineage.** Replay never mutates history; it creates a fork checkpoint with `metadata.source = "fork"` that branches from the replay point (`libs/langgraph/langgraph/pregel/_loop.py:948-959`). Asserted explicitly by `test_replay_creates_branch_preserving_old_checkpoints` (`libs/langgraph/tests/test_time_travel.py:2985-3109`).

- **ReplayState for subgraphs.** Subgraphs do not receive the parent's `checkpoint_id` directly; the parent builds a `ReplayState` and passes it via `CONFIG_KEY_REPLAY_STATE` (`libs/langgraph/langgraph/pregel/_loop.py:1036-1057`). The subgraph's first invocation uses `ReplayState.get_checkpoint` to find the latest checkpoint strictly before the replay point; subsequent invocations (same subgraph in a later loop iteration) fall back to the latest checkpoint, so newly-produced checkpoints during replay are visible (`libs/langgraph/langgraph/_internal/_replay.py:34-90`).

- **Determinism from hashing, not pinning.** Task IDs are content hashes of `(checkpoint_id, namespace, step, node_name, kind, …parts)` (`libs/langgraph/langgraph/pregel/_algo.py:1404-1409`). Checkpoint IDs are `uuid6(clock_seq=step)` (`libs/checkpoint/langgraph/checkpoint/base/id.py:79-109`) — time-ordered, monotonic per saver instance, and uniquely sortable lexicographically (this is what makes `ORDER BY checkpoint_id DESC` equivalent to chronological ordering in Postgres).

- **DeltaChannels decouple write storage from value storage.** A delta channel stores a sentinel in `channel_values` between snapshots, with the actual writes kept in `checkpoint_writes` keyed by `(thread_id, checkpoint_ns, channel, version)`. Reconstruction on hydration walks the parent chain via `get_delta_channel_history` (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:582-649`), which returns `{writes: oldest→newest, seed: nearest ancestor's value or omitted}`. The seed terminates the walk, bounding replay depth even when the channel stops receiving writes.

- **Two-tiered determinism gate.** Workflow replay is always deterministic; LLM replay is opt-in via `CachePolicy`. The line is drawn at the runnable boundary: writes inside a runnable are cached as a batch keyed by the policy's `key_func`, and never broken apart per token (`libs/langgraph/langgraph/pregel/_algo.py:668-687, 858-870, 1018-1034`).

- **Shallow vs full saver.** `ShallowPostgresSaver` explicitly omits time-travel support — "It is meant to be a light-weight drop-in replacement … with the exception of time travel." (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/shallow.py:172-175`). This is a deliberate trade-off, not a bug.

- **Conformance harness as contract.** `libs/checkpoint-conformance/langgraph/checkpoint/conformance/` is a separate library that runs capability-detected tests against any `BaseCheckpointSaver` subclass (`libs/checkpoint-conformance/langgraph/checkpoint/conformance/validate.py:45-128`, `capabilities.py:15-96`). Every back-end (InMemory, Postgres, SQLite, Shallow) is expected to satisfy PUT/PUT_WRITES/GET_TUPLE/LIST/DELETE_THREAD and optionally DELETE_FOR_RUNS/COPY_THREAD/PRUNE/DELTA_CHANNEL_HISTORY.

## Notable Patterns

- **Channel-level replay as a primitive.** `BaseChannel.from_checkpoint` and `DeltaChannel.replay_writes` together form a deterministic, batched reducer (line `libs/langgraph/langgraph/channels/delta.py:118-157`). The reducer must be "batching-invariant" so LangGraph can replay writes in larger batches than originally produced without altering reconstructed state (`libs/langgraph/langgraph/channels/delta.py:40-49`). This is the formal determinism contract the user signs when using DeltaChannel.

- **Source of truth.** The persisted `Checkpoint` (`v`, `id`, `ts`, `channel_values`, `channel_versions`, `versions_seen`, `pending_sends`, `updated_channels`) plus `checkpoint_writes` (per-task) plus `checkpoint_blobs` (per-channel-per-version) form the source of truth. The in-memory `channels` mapping is *derived* from these at `__enter__` (`libs/langgraph/langgraph/pregel/_loop.py:1668-1673` → `libs/langgraph/langgraph/pregel/_checkpoint.py:136-184`). Anything not captured here is lost on replay.

- **Stub checkpoints for first-run exit-mode.** When `durability="exit"` and no parent is persisted yet, `_put_exit_delta_writes` creates a synthetic stub checkpoint with `step=-2` so delta writes can be anchored without polluting the user's data (`libs/langgraph/langgraph/pregel/_loop.py:1201-1292`).

- **Step-prefixed synthetic task IDs.** When accumulating delta writes at exit, writes are regrouped under `synth_tid = f"{step:08d}-{tid}"` so that the saver's `ORDER BY task_id, idx` returns them in chronological order (`libs/langgraph/langgraph/pregel/_loop.py:1260-1292`).

- **Re-entry via same `run_id`.** A caller can reconnect to an in-flight run by sending a request whose `metadata.run_id` matches the current head's `run_id` (`libs/langgraph/langgraph/pregel/_loop.py:847-859`). This is replay-by-config-coincidence, distinct from explicit checkpoint replay.

- **Migration as a replay constraint.** Pre-delta threads continue to work after upgrading the channel type to `DeltaChannel`, because `get_delta_channel_history` returns the stored value as `seed` and `DeltaChannel.from_checkpoint(seed)` uses it directly instead of starting empty (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:582-649`, `libs/langgraph/langgraph/channels/delta.py:118-137`). Tested in `test_delta_channel_migration.py:197-251`.

## Tradeoffs

- **Workflow replay only, model replay opt-in.** Predictable cost on rerun (model calls), but the framework never guarantees model determinism. To get model determinism, the user must provide a `CachePolicy` or pin model temperature to 0.

- **Replay re-runs un-resumed tasks.** `is_replaying = False` after the first tick (`libs/langgraph/langgraph/pregel/_loop.py:704`), so replay is a single superstep's worth of re-execution rather than a from-scratch re-run. This is the right tradeoff for time-travel debugging (you only pay for what you changed), but it means a "true replay" of an LLM agent step requires the agent itself to be idempotent or cached.

- **Random suffixes in next-version.** `PostgresSaver.get_next_version` returns `f"{next_v:032}.{next_h:016}"` where `next_h = random.random()` (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/base.py:543-552`). This gives unique row identities even under concurrent writers, but two replays of identical state produce different `(thread_id, checkpoint_ns, channel, version)` rows. Sorting by `int(version.split('.')[0])` keeps replay semantics stable.

- **DeltaChannel write depth.** `snapshot_frequency=1000` is the default (`libs/langgraph/langgraph/channels/delta.py:73-74`). Threads that are long-lived but write rarely still trigger force-snapshots via `DELTA_MAX_SUPERSTEPS_SINCE_SNAPSHOT` (default 5000) so the walk doesn't grow unbounded (`libs/langgraph/langgraph/pregel/_checkpoint.py:50-58`).

- **ShallowPostgresSaver trade-off.** Drop history → lose time travel and delta-history correctness for any thread still being written to (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/shallow.py:172-175`). Deprecated as of 2.0.20 in favour of `durability="exit"`.

- **Append-only lineage cost.** Every replay adds at least one new fork checkpoint. Long-lived threads doing exploratory replay will accumulate checkpoint rows. `BaseCheckpointSaver.prune` exists but the docs explicitly warn that naive `keep_latest` can silently corrupt DeltaChannel state by severing the chain (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:374-415`).

## Failure Modes / Edge Cases

- **Naive pruning breaks delta replay.** `BaseCheckpointSaver.prune` docstring (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:374-415`) calls this out explicitly. `get_delta_channel_history` simply returns no `seed` if the walk can't find one — no error is raised, the channel silently reconstructs as empty.

- **`id=None` on messages races with replay.** `DeltaChannel.put_writes` calls `ensure_message_ids` before the background thread serialises them (`libs/langgraph/langgraph/pregel/_loop.py:451-458`). Without this, reducers that assign IDs inside `apply_writes` race with serialisation and `get_state()` replays produce a different UUID per call (`libs/langgraph/langgraph/pregel/_messages.py:429-432`). Tested in `test_delta_channel_id_stability.py`.

- **Time-travel fork vs resume ambiguity.** When a client resumes with an explicit `checkpoint_id` pointing at the current head (e.g. LangGraph Studio sending `checkpoint: {checkpoint_id}` alongside `Command(resume=...)`), `is_replaying` is True but `is_time_traveling` is False. The loop must NOT pass `ReplayState` to subgraphs in that case, otherwise subgraphs would walk back to a stale checkpoint (`libs/langgraph/langgraph/pregel/_loop.py:1019-1057`). This is explicitly documented in the comment.

- **Interrupted replay re-fires interrupts.** `test_replay_from_before_interrupt_refires` shows that an `interrupt()` call re-fires when its checkpoint is replayed (asks the user again). This is the *intended* behavior but surprising — the test asserts `call_count["ask_human"] == 3` after original+resume+replay (`libs/langgraph/tests/test_time_travel.py:280`).

- **Multiple interrupts share resume map.** Resume is dispatched via `CONFIG_KEY_RESUME_MAP` keyed by an xxhash namespace (`libs/langgraph/langgraph/pregel/_loop.py:898-902`). A typo or stale hash loses the resume value silently — only the strict-shape check (`all(is_xxh3_128_hexdigest(k) for k in resume)`) signals misuse.

- **CachePolicy key collisions.** The cache key is `xxh3_128_hexdigest(args_key.encode())`. If two distinct inputs produce the same `args_key` via user-defined `key_func`, cached writes leak between calls (`libs/langgraph/langgraph/pregel/_algo.py:677-684, 866-868`). No explicit guard.

- **Stub checkpoint visible to readers.** `_put_exit_delta_writes` creates a stub at `step=-2` to anchor delta writes (`libs/langgraph/langgraph/pregel/_loop.py:1235-1258`). If `durability="exit"` and the run fails between stub creation and the actual exit checkpoint, readers may see the stub without the final checkpoint. The comment in `_loop.py:1202-1208` calls this the "visibility invariant", but it relies on `_checkpointer_put_after_previous` being drained.

- **`ShallowPostgresSaver` cannot time-travel.** The class explicitly documents this (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/shallow.py:172-175`). Code paths that require replay (e.g. `update_state` after migration in `test_delta_channel_migration.py:483-555`) silently degrade or fail.

## Future Considerations

- **`CachePolicy` as a model-output cache.** Today the cache is keyed on `args_key` and the user must provide a deterministic `key_func`. There is room for a built-in "deterministic LLM" wrapper that wraps `ChatModel.invoke` and writes the result into `CachePolicy` keyed on `(model, messages, tools, temperature)`. This is a feature opportunity, not a missing primitive.

- **Replay-time verification.** Currently, replay only re-hydrates state. A "replay-verify" mode that compares new run outputs to recorded `pending_writes` could close the loop on correctness. No code path exists today; the closest analog is `test_replay_interrupt_stable_across_replays` which is a test-only fixture.

- **Cross-thread replay.** `ReplayState` is single-instance per parent execution (`libs/langgraph/langgraph/_internal/_replay.py:21-23`). A user wanting to replay against a different thread's checkpoint must do it via `copy_thread`, which has its own determinism caveats around delta channels (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:361-372`).

- **Migration to `Durability="exit"` deprecates ShallowPostgresSaver.** The deprecation warning at `libs/checkpoint-postgres/langgraph/checkpoint/postgres/shallow.py:192-196` is a clean signal that time-travel and replay correctness are tied to full-history checkpointers. Anyone still on `ShallowPostgresSaver` will lose replay after 3.0.0.

- **`replay_writes` reducer constraints.** `DeltaChannel` requires `reducer(reducer(s, xs), ys) == reducer(s, xs + ys)` (`libs/langgraph/langgraph/channels/delta.py:40-49`). A static check on user-defined reducers would catch non-batching-invariant reducers at compile time instead of allowing them to produce inconsistent replay.

## Questions / Gaps

- **How is `config["configurable"]["checkpoint_id"]` validated?** No clear evidence found for input validation beyond what the saver returns when the ID doesn't exist (`libs/langgraph/langgraph/pregel/_loop.py:1614` — `get_tuple` returns `None` and a synthetic empty checkpoint is substituted). A bogus checkpoint_id silently replays from empty.

- **What happens if `durability` changes mid-run?** No clear evidence found. The durability flag is captured at loop entry (`libs/langgraph/langgraph/pregel/_loop.py:313, 1662`) and influences `_put_exit_delta_writes` activation (`libs/langgraph/langgraph/pregel/_loop.py:1301-1308`). A user toggling durability across replays would get inconsistent stub/anchor behavior.

- **Is there a published SLO on replay cost vs original run?** No clear evidence found. Empirically, replay = hydration cost + un-resumed-task execution cost, but no benchmarks are linked.

- **How does `ReplayState` interact with `delete_thread` of an ancestor?** No clear evidence found. If a user deletes the thread the replay is anchored on, `ReplayState.get_checkpoint` would walk the now-empty saver and return `None`, causing the subgraph to fall back to a synthetic empty checkpoint (`libs/langgraph/langgraph/pregel/_loop.py:1637-1640`).

- **Conformance coverage of replay correctness.** The conformance suite (`libs/checkpoint-conformance/langgraph/checkpoint/conformance/spec/`) tests round-trip of put/get/list/copy/delete/prune/delta-channel-history. It does NOT test the *workflow-level* replay semantics (does re-running produce the same state). The high-level replay tests live in `libs/langgraph/tests/test_time_travel.py` and run only against the bundled checkpointer fixtures.

---

Generated by `dimensions/01.10-replay-and-determinism-in-execution.md` against `langgraph`.