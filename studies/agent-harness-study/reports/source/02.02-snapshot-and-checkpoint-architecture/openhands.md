# Source Analysis: openhands

## Dimension 02.02: Snapshot and Checkpoint Architecture

### Source Info

| Field | Value |
|-------|-------|
| Name | openhands |
| Path | `studies/agent-harness-study/sources/openhands` |
| Language / Stack | Python (FastAPI app server) + React/TS frontend; agent runtime delegated to external `openhands-sdk==1.29.0` |
| Analyzed | 2026-07-06 |

## Summary

The version of OpenHands in this source directory is the **V1 app server** (`openhands/app_server/`), which orchestrates conversations that actually execute inside a separate sandbox (an `openhands-sdk`-based agent-server, pinned as `openhands-sdk==1.29.0` in `pyproject.toml:61`). Because of this split, there is **no first-class "checkpoint" or "snapshot" object** in this repository — grep for `checkpoint`/`snapshot` in the backend returns only `MetricsSnapshot` (an imported SDK type) and unrelated frontend browser thumbnails. The point-in-time execution state (the agent's live loop, LLM context, tool state) lives inside the SDK/sandbox, which is out of scope for this source.

What this source *does* implement, and what functions as its durable state model, is:

1. **An append-only per-event JSON log** persisted by the app server as events stream in from the sandbox via webhook (`openhands/app_server/event/event_service_base.py:190`, `openhands/app_server/event_callback/webhook_router.py:453`). One file per event, named by event ID, under `{persistence_dir}/{user_id}/v1_conversations/{conversation_id}/`.
2. **A SQL conversation-metadata record** that survives sandbox destruction (`openhands/app_server/app_conversation/sql_app_conversation_info_service.py:120-125`), holding title, parent/child links, tags, and a rolled-up metrics snapshot.
3. **Sandbox pause/resume** as the actual execution-resume mechanism — a paused Docker container retains live state and is `unpause`d on resume (`openhands/app_server/sandbox/docker_sandbox_service.py:517-534`); a persistent workspace volume (`openhands-workspace-{sandbox_id}`) survives container restarts (`openhands/app_server/sandbox/docker_sandbox_service.py:566`).

So "resume" is achieved by keeping the sandbox alive (pause/unpause), not by reconstructing agent state from a stored snapshot in this source. The persisted event log is a **read model / trajectory export**, not a re-hydration source for execution.

## Rating

**4 / 10.** State capture is present and reasonably structured (append-only event files + SQL metadata + pause/resume), but from this source's perspective it is a mirror/read-model rather than a true checkpoint system: there is no versioned snapshot object, no parent-checkpoint chain, no documented restore-from-snapshot path (resume relies on the live sandbox), and no evidence of safe pruning of the event log. The genuine execution-state persistence lives in the out-of-scope SDK. Within the studied boundary the model is present-but-fragile, matching the 4-6 band, at its lower end because durable restore depends on an external container staying alive.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Event write (append, one file per event id) | `save_event` computes `{id_hex}.json` path and delegates to `_store_event` | `openhands/app_server/event/event_service_base.py:190-197` |
| Event blob format | `_store_event` writes `event.model_dump_json(indent=2)`; `_load_event` uses `Event.model_validate_json` | `openhands/app_server/event/filesystem_event_service.py:33-36`, `:23-27` |
| Storage path / scoping | `{prefix}/{user_id}/v1_conversations/{conversation_id.hex}` | `openhands/app_server/event/event_service_base.py:66-84`; `openhands/app_server/conversation_paths.py:38-73` |
| Persistence root | prefix = `get_global_config().persistence_dir` | `openhands/app_server/event/filesystem_event_service.py:62-69` |
| Event ingress (checkpoint trigger) | webhook `POST /events/{conversation_id}` saves every streamed event | `openhands/app_server/event_callback/webhook_router.py:453-466` |
| Conversation-info persistence trigger | webhook `POST /conversations` fires on start/pause/resume/delete | `openhands/app_server/event_callback/webhook_router.py:333-416` |
| Durable metadata record (survives sandbox loss) | `SQLAppConversationInfoService` docstring: "store a record ... even after its sandbox ceases to exist" | `openhands/app_server/app_conversation/sql_app_conversation_info_service.py:120-125` |
| Rolled-up metrics snapshot persistence | `process_stats_event` → `update_conversation_statistics` from `ConversationStateUpdateEvent(key='stats')` | `openhands/app_server/event_callback/webhook_router.py:468-473`; `sql_app_conversation_info_service.py:472-506` |
| Metrics rebuilt from columns | `MetricsSnapshot` reconstructed from stored token/cost columns | `openhands/app_server/app_conversation/sql_app_conversation_info_service.py:529-544` |
| Parent/child relationship (branching) | `parent_conversation_id`, `sub_conversation_ids` fields + indexed SQL column | `openhands/app_server/app_conversation/app_conversation_models.py:131-132`; `sql_app_conversation_info_service.py:109-111`, `:276-290` |
| Resume = unpause / restart container | `resume_sandbox` unpauses or starts container | `openhands/app_server/sandbox/docker_sandbox_service.py:517-534` |
| Pause preserves state | `pause_sandbox` pauses running container | `openhands/app_server/sandbox/docker_sandbox_service.py:536-548` |
| Workspace volume durability | persistent volume `openhands-workspace-{sandbox_id}` removed only on delete | `openhands/app_server/sandbox/docker_sandbox_service.py:459-492`, `:564-568` |
| Archived (state-lost) state | `SandboxStatus.MISSING` default when sandbox gone | `openhands/app_server/app_conversation/app_conversation_models.py:173-181` |
| Trajectory export ("snapshot" download) | `export_conversation` zips all events + `meta.json`; `iter_events_for_export` orders by timestamp | `openhands/app_server/app_conversation/app_conversation_service.py:159-173`; `event/event_service_base.py:145-156` |
| Pluggable storage backends | filesystem / AWS / GCS event services; local/memory/s3/gcs file stores | `openhands/app_server/event/` (`aws_event_service.py`, `google_cloud_event_service.py`, `filesystem_event_service.py`); `openhands/app_server/file_store/` |
| Session/thread identity | `session_api_key`, `conversation_id`, `sandbox_id` on `AppConversation` | `openhands/app_server/app_conversation/app_conversation_models.py:173-187` |

## Answers to Dimension Questions

1. **When is a checkpoint written?**
   There is no explicit checkpoint. The closest events: (a) each conversation event is persisted as it streams from the sandbox via `POST /events/{conversation_id}` (`webhook_router.py:453-466`), and (b) the SQL conversation-info record is upserted on conversation start/pause/resume/delete via `POST /conversations` (`webhook_router.py:333-416`, `:414`). Metrics are updated whenever a `ConversationStateUpdateEvent` with `key='stats'` arrives (`webhook_router.py:468-473`).

2. **What is inside it?**
   - Per-event file: the full SDK `Event` serialized as indented JSON (`filesystem_event_service.py:35`).
   - SQL record: id, `created_by_user_id`, `sandbox_id`, title, tags, `parent_conversation_id`, LLM model, and a token/cost `MetricsSnapshot` (`sql_app_conversation_info_service.py:95-117`, `:529-544`).
   - It does **not** contain the agent's live in-memory loop state, tool state, or LLM message context — those remain in the sandbox/SDK.

3. **Can it restore execution or only data?**
   Only **data** within this source. The event log + SQL metadata reconstruct the *conversation history* for UI display and trajectory export (`app_conversation_service.py:159-173`). Actual *execution* is resumed by keeping/unpausing the sandbox container (`docker_sandbox_service.py:517-534`); if the sandbox is `MISSING` the conversation is archived read-only (`app_conversation_models.py:173-181`) — no code here re-hydrates a live agent from the stored events.

4. **Can checkpoints branch?**
   Conversations support a parent/child (sub-conversation) hierarchy (`app_conversation_models.py:131-132`; `sql_app_conversation_info_service.py:109-111`, `:276-290`, and `include_sub_conversations` filtering at `:141-151`). This is conversation-level branching/nesting, not point-in-time snapshot branching or time-travel forking of a single conversation's state.

5. **Are checkpoints pruned safely?**
   No evidence of event-log pruning or retention policy was found in the event services (`event/event_service_base.py`, `filesystem_event_service.py`). Cleanup is coarse-grained: deleting a sandbox removes the workspace volume (`docker_sandbox_service.py:564-568`), and `delete_app_conversation_info` removes the SQL record (`sql_app_conversation_info_service.py:276-315`). The `count_conversations_by_sandbox_id` guard (`app_conversation_info_service.py:88-93`) prevents deleting a sandbox still referenced by conversations, which is the one explicit safety check observed.

## Architectural Decisions

- **Execution state and durable record are deliberately split.** The app server persists an event mirror + metadata (`sql_app_conversation_info_service.py:120-125`) while the SDK sandbox owns live state. This lets the conversation record outlive the sandbox but means restore-of-execution is not an app-server concern.
- **Append-only, one-file-per-event log** keyed by event UUID (`event_service_base.py:190-197`), making writes idempotent-by-id and avoiding rewrite-in-place of a monolithic snapshot.
- **Pluggable, prefix-scoped backends.** The abstract `EventServiceBase` (`event_service_base.py:31-54`) has filesystem, AWS, and GCS implementations; permission isolation is enforced purely by the storage path prefix (`event_service_base.py:33-35`, `:66-84`).
- **Pause/unpause as the resume primitive** rather than serialize/restore (`docker_sandbox_service.py:517-548`), trading durability for speed and simplicity.

## Notable Patterns

- **Read-model / event-sourcing-lite:** events streamed from the runtime are persisted and later replayed *for display and export only* (`iter_events_for_export`, `event_service_base.py:145-156`).
- **Webhook-driven persistence:** the sandbox pushes state changes to the app server (`webhook_router.py:333`, `:453`) instead of the app server polling.
- **Timestamp-ordered reconstruction:** events are sorted by ISO-8601 `timestamp` at read time (`event_service_base.py:127-131`, `:154`), not by a stored sequence number.
- **Metadata durability tier separate from blob tier:** SQL for queryable metadata, file store for event blobs.

## Tradeoffs

- **Speed vs. durability of execution state:** pause/unpause resumes instantly and preserves exact in-memory state, but if the container/volume is lost the execution cannot be rebuilt — the conversation becomes archived read-only (`app_conversation_models.py:173-181`).
- **Path-prefix isolation is simple but blunt:** security depends entirely on correct prefix construction (`event_service_base.py:33-35`); a bug in `get_conversation_path` (`:66-84`) could cross-scope events.
- **Read-time sort/filter/paginate over all event files** (`event_service_base.py:107-143`) is simple but O(n) per query — every search loads and validates every event file (bounded only by `EVENT_SERVICE_LOAD_EVENT_CONCURRENCY`, `:24-28`), which will not scale for very long conversations.
- **No sequence numbers:** ordering relies on string comparison of ISO timestamps (`:121-131`), which is fragile to clock skew or identical timestamps.

## Failure Modes / Edge Cases

- **Corrupt/partial event file:** `_load_event` swallows exceptions and returns `None`, logging only if the file still exists (`filesystem_event_service.py:23-31`); a bad file is silently dropped from history rather than failing the read.
- **Sandbox lost mid-run:** state is unrecoverable; conversation shows `SandboxStatus.MISSING` and the UI switches to an archived/read-only banner (`app_conversation_models.py:173-181`; documented in `AGENTS.md` "Archived Conversations").
- **Resume of an `exited` (not merely paused) container** calls `container.start()` (`docker_sandbox_service.py:529-530`) — in-memory agent state from before exit is gone; only the on-disk workspace volume survives, and re-hydration then depends on SDK-side persistence not present in this source.
- **Duplicate/late webhook events:** because files are keyed by event id, a re-delivered event overwrites its own file (idempotent), but there is no dedup for stats aggregation beyond `process_stats_event` error-swallowing (`sql_app_conversation_info_service.py:507-512`).

## Future Considerations

- Introduce an explicit, versioned conversation snapshot (sequence-numbered) so history reconstruction does not rely on ISO-timestamp string sort (`event_service_base.py:121-131`).
- Add an index/manifest per conversation to avoid loading every event file on each `search_events` call (`event_service_base.py:107-125`).
- Define an event-log retention/pruning policy; none is present today.
- Document and, if desired, implement true restore-from-events so a lost sandbox can be re-hydrated instead of archived.

## Questions / Gaps

- **Where the real point-in-time execution state lives is out of scope.** It resides in `openhands-sdk==1.29.0` (`pyproject.toml:61`), which is not vendored in this source, so its checkpointer interface, snapshot schema, and any parent-checkpoint IDs could not be inspected. Searches: `grep -ri checkpoint|snapshot` over `openhands/` (only `MetricsSnapshot` import and frontend browser thumbnails found); `find` for `*state*`/`*persist*`/`*checkpoint*` (none in backend `openhands/`).
- No evidence found of a resume path that rebuilds agent execution from the persisted event log; resume is via sandbox pause/unpause only (`docker_sandbox_service.py:517-534`).
- No evidence found of event-log pruning, compaction, or TTL within the studied directory.

---

Generated by `02.02-snapshot-and-checkpoint-architecture` against `openhands`.
