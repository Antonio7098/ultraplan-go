# Source Analysis: openhands

## Dimension 11.03: Repository and Workspace Context Maps

### Source Info

| Field | Value |
|-------|-------|
| Name | openhands |
| Path | `studies/agent-harness-study/sources/openhands` |
| Language / Stack | TypeScript / React 19 (React Router framework mode), Vite, TanStack Query, Zustand, Electron packaging ("agent-canvas" frontend) |
| Analyzed | 2026-08-25 |

## Summary

This source is the OpenHands **frontend** (agent-canvas), one part of a multi-repo system whose agent/server side lives in a sibling SDK repo (`AGENTS.md` "Repository Map — what belongs where" section; the repo's own `AGENTS.md` explicitly states this repo "is **only the agent-canvas frontend**"). Consequently, model-facing context construction (LLM repo maps, symbol indexes) is architecturally out of this repo's scope. What this source does implement is a well-engineered **human-facing workspace context map**: a bounded workspace file enumeration (`src/hooks/query/use-workspace-files.ts:42-47`), a client-side tree builder (`src/utils/file-tree.ts:69-98`), file content classification (`src/hooks/query/use-workspace-file-content.ts:96-112`), git-based repository detection (`src/hooks/query/use-local-git-info.ts:38-75`), and event-driven incremental cache invalidation that keeps the map fresh as the agent writes files (`src/hooks/use-auto-refresh-files-on-edit.ts:70-151`). Conversation-level context summarization exists via a server-side condense action (`src/hooks/mutation/use-condense-conversation.ts:15-31`). There is **no symbol indexing of any kind** in this source — searches for tree-sitter/ctags/AST/symbol-index returned only CSS `outline-*` utilities and terminal prompt parsing (`src/utils/parse-terminal-output.ts:4-15`). File discovery for *the model* is delegated to the agent's own tools executing inside the sandbox; the frontend never feeds the agent a repository map.

## Rating

**Score: 5 / 10**

Rationale against the rubric:

- The workspace-map slice that exists is genuinely solid: dual-backend transports with explicit rationale comments (`src/hooks/query/use-workspace-files.ts:54-64`, `116-128`), bounded enumeration with an exclusion list and hard cap (`use-workspace-files.ts:16,24-47`), a tree builder with a documented O(1) side-table optimization and a performance regression test (`src/utils/file-tree.ts:22-35`; `__tests__/utils/file-tree.test.ts:63`), and incremental refresh with careful dedup (`src/hooks/use-auto-refresh-files-on-edit.ts:77-103`). That alone would score 7.
- However, the dimension's core concerns are largely absent from this source: no symbol indexing (no evidence found anywhere), no token-budgeted or model-facing repository map, and one purpose-built file scorer (`sortFilesByPriority`) is dead code with zero production consumers (`src/utils/file-priority.ts:98-106`; only reference is its own test `__tests__/utils/file-priority.test.ts:3`). Cloud binary handling is acknowledged as lossy (`src/hooks/query/use-workspace-file-content.ts:224-228`), and the 2000-file cap truncates silently (`use-workspace-files.ts:104,155`). Per the rubric these gaps place it in "present but inconsistent / partial": **5**.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Workspace map generation (local) | Bounded `find` command listing all regular files, pruning heavy dirs, capped at 2000 | `src/hooks/query/use-workspace-files.ts:42-47` (dirs at 24-39, cap at 16) |
| Workspace map generation (cloud) | First-class cloud endpoint runs same bounded `find` server-side | `src/hooks/query/use-workspace-files.ts:129-165` |
| Tree construction | `buildFileTree` builds nested tree from flat paths; Map side-table for O(1) insertion; leaf→directory promotion | `src/utils/file-tree.ts:69-98`, `38-67` |
| File scorer (orphaned) | Entry-point priority ranking (`index.html`, `README.md`, `package.json`, …) with depth-first ordering | `src/utils/file-priority.ts:11-36,73-106` (no production consumers) |
| Directory search for workspace selection | Paginated `searchSubdirectories` walk with repeated-page-id guard | `src/hooks/query/use-search-subdirs.ts:26-51` |
| Repository detection priority chain | conversation metadata → task polling → local git probe | `src/hooks/use-conversation-primary-repository.ts:6-27` |
| Local git probe | Consolidated bash script reads origin remote + branch, falls back to exactly one nested repo ≤4 levels deep | `src/hooks/query/use-local-git-info.ts:38-50,52-75` |
| Path anchoring convention | `getGitPath` anchors relative paths on `DEFAULT_WORKING_DIR = "workspace/project"` | `src/utils/get-git-path.ts:3-20`; `src/api/agent-server-config.ts:1` |
| Content classification | Extension-based kind guess + NUL-byte sniff (first ~8KB) for binary detection | `src/hooks/query/use-workspace-file-content.ts:96-112` |
| Incremental update trigger | Watches `FileEditorObservation`/`StrReplaceEditorObservation`/`PlanningFileEditorObservation` (skipping read-only `view`), plus bash/Terminal observations | `src/hooks/use-auto-refresh-files-on-edit.ts:8-25` |
| Incremental invalidation targets | Invalidates `workspace-files`, `workspace-file-content`, `file_changes`, `file_diff`, `git_commits` query keys | `src/hooks/use-auto-refresh-files-on-edit.ts:135-141` |
| Cache-buster mechanism | Monotonic mutation counter appended as `?v=<n>` to static workspace URLs | `src/stores/use-workspace-mutation-counter.ts:30-34,41-53` |
| Freshness TTLs | workspace files staleTime 30s/gcTime 5min; file content staleTime 5s; git info re-probe every 10s | `src/hooks/query/use-workspace-files.ts:108-109`; `src/hooks/query/use-workspace-file-content.ts:299-300`; `src/hooks/query/use-local-git-info.ts:158-159` |
| Selection scoping safeguard | File selection tagged with conversation id so selections never leak across workspaces (issue #1350) | `src/routes/files-tab.tsx:93-106` |
| Conversation-level summarization | Manual condense via `POST /api/conversations/{id}/condense`; awaits `Condensation` event + per_turn_token drop (2.5s settle, 90s timeout) | `src/hooks/mutation/use-condense-conversation.ts:15-31`; `src/hooks/use-await-context-compaction.ts:6-9,57-163` |
| Context-window observability | Usage meter sourced from live metrics store, falling back to accumulated token usage | `src/hooks/use-context-window-usage.ts:31-52` |
| Environment topology map injected into agent prompt | `<RUNTIME_SERVICES>` block built by `buildRuntimeServicesSystemSuffix`, attached via `buildAgentContext` → `agent_context.system_message_suffix` | `src/api/agent-server-adapter.ts:215-297,749-755` |
| Workspace identity sent at conversation start | `workspace: { working_dir }` payload (`LocalWorkspacePayload`) sent to agent-server | `src/api/agent-server-adapter.ts:427-441,396-398` |
| Tests — map generation | Local bash path vs cloud endpoint path; normalization/dedupe; route-id fallback regression | `__tests__/hooks/query/use-workspace-files.test.tsx:108-199` |
| Tests — tree building | nesting, sort order, dedup, empty input, leaf promotion, wide-directory perf regression | `__tests__/utils/file-tree.test.ts:5-63` |
| Tests — scorer | depth/basename priority ordering cases | `__tests__/utils/file-priority.test.ts:3-85` |
| E2E coverage | Mock-LLM spec exercising Files tab and git control bar end-to-end | `tests/e2e/mock-llm/files/mock-llm-files-and-git.spec.ts` |

## Answers to Dimension Questions

1. **Does the system build a map of the repository?**
   Partially, and only for humans. The Files tab enumerates the entire workspace tree via a pruned, capped `find` invocation on local backends (`src/hooks/query/use-workspace-files.ts:42-47,65-114`) or via a first-class cloud listing endpoint that performs the same bounded find server-side (`use-workspace-files.ts:129-165`), then builds a sorted directory tree client-side (`src/utils/file-tree.ts:69-98`). There is no model-facing repository map (nothing like a token-budgeted repomap): the agent discovers code through its own tools in the sandbox, and this repo ships no such structure. No evidence found of any LLM-oriented map artifact being generated or transmitted; the closest machine-authored environment map is the `<RUNTIME_SERVICES>` service-topology block appended to the system message suffix (`src/api/agent-server-adapter.ts:215-297`).

2. **How are relevant files selected?**
   Three mechanisms, none semantic. (a) Coarse structural filtering: the enumeration prunes 14 known-heavy directories (`.git`, `node_modules`, `dist`, …) and truncates at 2000 entries (`src/hooks/query/use-workspace-files.ts:16,24-39,46`). (b) A hand-curated basename priority list ranks entry points (`index.html`, `README.md`, `package.json`, `Dockerfile`, …) with shallower-depth-first ordering — but it is currently orphaned, consumed only by its own test (`src/utils/file-priority.ts:11-36,98-106`; `__tests__/utils/file-priority.test.ts:3`). (c) User-driven selection: clicking nodes in the Files tab tree sets the selected path, scoped per-conversation so a selection cannot leak into another workspace (`src/routes/files-tab.tsx:93-106`). For choosing which *workspace* to open at all, a paginated directory search walks subdirectories server-side (`src/hooks/query/use-search-subdirs.ts:26-51`), and repository identity is resolved through a metadata → task-polling → local-git-probe chain (`src/hooks/use-conversation-primary-repository.ts:16-23`).

3. **Are symbols indexed for the model?**
   No. No evidence found. Searches across the whole source for `tree-sitter`, `ctags`, `symbol index`, `SymbolIndex`, and AST-related terms produced only CSS focus-ring utilities (`focus:outline-none` throughout `src/ui/`, e.g. `src/ui/dropdown/dropdown-menu.tsx:66`) and terminal-prompt "symbol" parsing (`src/utils/parse-terminal-output.ts:4-15`). File understanding stops at extension/MIME guessing and a NUL-byte binary sniff (`src/hooks/query/use-workspace-file-content.ts:96-112`); there is no syntax-aware analysis, outline extraction, or definition indexing anywhere in this source.

4. **Is workspace context stale or fresh?**
   Deliberately fresh, via three layers. (1) Event-driven invalidation: every file-editor observation (excluding read-only `view` commands) and bash/terminal observation invalidates the workspace-files, file-content, git-changes, diff, and commits queries (`src/hooks/use-auto-refresh-files-on-edit.ts:8-25,127-141`), with idempotent processing guaranteed by an id Set plus a WeakSet for id-less events (`use-auto-refresh-files-on-edit.ts:102-103`). (2) A monotonic mutation counter is appended as a `?v=<n>` cache-buster so iframe/img previews of rewritten files bypass browser HTTP caches (`src/stores/use-workspace-mutation-counter.ts:30-34,41-53`). (3) TTL-based freshness: 30s staleTime for listings, 5s for file content, and a 10-second polling re-probe of git metadata (`src/hooks/query/use-workspace-files.ts:108-109`; `use-workspace-file-content.ts:299-300`; `use-local-git-info.ts:158-159`). Staleness is therefore bounded to seconds during active sessions; the residual risk is the silent 2000-file cap and the fact that invalidation only refetches actively-mounted queries (acknowledged cost-saving behavior, `use-auto-refresh-files-on-edit.ts:60-68`).

## Architectural Decisions

- **Workspace maps live in the frontend, agent context does not.** The repo's own `AGENTS.md` partitions responsibility: this repo owns UI and how it calls backend endpoints; agent/tool behavior and new API endpoints belong to the sibling `software-agent-sdk`. So the absence of a model-facing repo map here is by design, not omission.
- **Bash-as-a-service over dedicated endpoints for local enumeration.** Rather than requiring a new backend endpoint, the local Files tab issues a real `find` command through `/api/bash/execute_bash_command` (`src/hooks/query/use-workspace-files.ts:83-89`); the cloud path instead uses a first-class listing endpoint because the browser cannot drive the bash endpoint cross-origin (`use-workspace-files.ts:58-63`). Both deliberately return unchanged tracked files too, matching experiences across backends (`use-workspace-files.ts:124-127`).
- **Backend-kind branching read from a store, not context.** `useWorkspaceFiles` selects transport via `useSyncExternalStore` over the backend registry because the transport layer itself branches on the store; using React context could disagree and POST to a removed proxy route (`src/hooks/query/use-workspace-files.ts:175-196`). This subtlety is documented inline and regression-tested (`__tests__/hooks/query/use-workspace-files.test.tsx:10-14,160-162`).
- **Push-on-event rather than poll-on-interval for mutations.** File freshness is driven by observing the conversation event stream (editor/bash observations) rather than periodic refetching, keeping costs proportional to actual writes and limited to mounted queries (`src/hooks/use-auto-refresh-files-on-edit.ts:50-68`).
- **Conversation-scoped UI state as a correctness boundary.** File selections and open tabs are tagged with the conversation id and hydrated from conversation-scoped localStorage, preventing cross-workspace leakage after switching conversations (`src/routes/files-tab.tsx:93-99,119-132`).
- **Conversation summarization is server-executed, client-observed.** The condense action POSTs and then waits for a `Condensation` event plus a measured `per_turn_token` drop, treating the HTTP ack as "work started," not "work done" (`src/hooks/use-await-context-compaction.ts:57-61`; baseline snapshot captured before request fires, `src/hooks/use-compact-context-action.ts:83-88`).

## Notable Patterns

- **Query-key-driven cache coherence**: TanStack Query keys encode conversation id/url/key/working-dir (`src/hooks/query/use-workspace-files.ts:74-81`), enabling surgical invalidation by key prefix from the event watcher (`src/hooks/use-auto-refresh-files-on-edit.ts:135-141`); sha-addressed commit queries are deliberately exempted as immutable (`use-auto-refresh-files-on-edit.ts:131-134`).
- **Defensive data shaping**: listing results are normalized (`./` stripping), de-duped, and re-capped after fetch (`src/hooks/query/use-workspace-files.ts:97-104`); the tree builder promotes leaf nodes to directories when deeper paths arrive out of order (`src/utils/file-tree.ts:51-57`).
- **Performance-aware rendering**: the tree builder replaced an O(n) child scan with a parent→Map side-table, cutting ~500k string comparisons to ~1000 for 1000-sibling directories, with a perf regression test locking it in (`src/utils/file-tree.ts:22-35`; `__tests__/utils/file-tree.test.ts:63`).
- **Documented negative space**: hooks explain what they intentionally do *not* do — e.g., bash observations don't bump the iframe-busting counter to avoid canvas flicker (`src/hooks/use-auto-refresh-files-on-edit.ts:56-62`), and the local git probe stays disabled on cloud backends to avoid leaking local paths (`src/hooks/query/use-local-git-info.ts:82-87`).

## Tradeoffs

- **Simplicity vs scale in enumeration**: a flat capped `find` is cheap and dependency-free but silently truncates repos above 2000 files (`src/hooks/query/use-workspace-files.ts:16,46,104`) — no "results truncated" signal reaches the user.
- **Extension heuristics vs correctness in classification**: MIME guessing plus NUL-sniffing avoids parsing dependencies but misclassifies unusual types; the cloud image/PDF path is explicitly best-effort because the endpoint returns text and can't round-trip bytes faithfully (`src/hooks/query/use-workspace-file-content.ts:100-102,224-228`).
- **Freshness vs flicker**: bumping the preview cache-buster on every shell command would reload iframes constantly, so only editor observations bump it — accepting brief staleness for shell-side edits in rich previews while still refreshing diff views (`src/hooks/use-auto-refresh-files-on-edit.ts:56-64`).
- **Human-facing map richness vs architectural boundary**: a symbol index or LLM-oriented map could be built here cheaply-ish, but adding it would duplicate responsibilities owned by the agent-server/SDK repo (per `AGENTS.md` placement table), so the frontend stays intentionally shallow.

## Failure Modes / Edge Cases

- **Silent truncation at MAX_FILES**: workspaces exceeding 2000 entries show an incomplete map with no indicator (`src/hooks/query/use-workspace-files.ts:16,104,155`).
- **Late/out-of-order events**: the event store can insert older events between newer ones, breaking slice-based diffs; handled via id Set + WeakSet dedup (`src/hooks/use-auto-refresh-files-on-edit.ts:77-101`). Id-less events would otherwise re-bump the counter on every render — explicitly reasoned through in comments.
- **Cross-conversation state leakage**: a stored selection from conversation A must not attempt to open a nonexistent file in conversation B's workspace (fixed as issue #1350) (`src/routes/files-tab.tsx:93-99`).
- **Cloud metadata lag**: the cloud listing previously never fired when the batch-get conversation query was still loading; fixed by sourcing the id from the route and falling back to the default working dir (`src/hooks/query/use-workspace-files.ts:130-134`; regression test `__tests__/hooks/query/use-workspace-files.test.tsx:180-199`).
- **Non-git or unborn-HEAD workspaces**: the local probe returns null fields for non-checkouts and callers treat that as "no repo detected" (`src/hooks/query/use-local-git-info.ts:66,93-94`); attachment-state detection elsewhere deliberately avoids filesystem probes because the server pre-initializes worktrees (`AGENTS.md`, Files tab diff-view default note).
- **Nested-repo ambiguity**: the git probe resolves at most *one* nested repo within 4 levels and gives up on multiple matches (`src/hooks/query/use-local-git-info.ts:41-48`), so monorepo layouts may report null repository info.

## Future Considerations

- Wire up or remove the orphaned entry-point scorer: `sortFilesByPriority` (`src/utils/file-priority.ts:98-106`) is tested but unused; either surface it in the Files tab landing experience (its docstring says it was meant for the top row) or delete it to avoid drift.
- Surface truncation: emit a visible "showing first 2000 files" marker when the cap binds (`src/hooks/query/use-workspace-files.ts:16,46`).
- Byte-accurate cloud binary serving needs a first-class download endpoint, as the code itself notes (`src/hooks/query/use-workspace-file-content.ts:224-228`).
- If model-facing navigation ever becomes a frontend concern (e.g., attaching a file map to prompts), the natural seam already exists: `agent_context.system_message_suffix` injection (`src/api/agent-server-adapter.ts:749-755`) — but ownership should be settled with the SDK repo first, per the placement table in `AGENTS.md`.

## Questions / Gaps

- **Where does the agent's own file discovery live?** Out of scope for this source by architecture (`AGENTS.md` placement table). Verifying whether the software-agent-sdk builds repo maps/symbol indexes for the model would require inspecting that sibling source, which the study isolation rules forbid. No evidence found within this repo either way.
- **Is the 30s listing staleness ever observed as stale in practice?** No telemetry or tests cover user-visible staleness of the Files tab between editor events; only unit tests of the fetch paths exist (`__tests__/hooks/query/use-workspace-files.test.tsx`). The mock-LLM E2E spec covers the surface functionally (`tests/e2e/mock-llm/files/mock-llm-files-and-git.spec.ts`) but not freshness timing.
- **Why was the entry-point scorer abandoned?** No commit-level rationale is recoverable from code alone; the module and its tests remain green (`__tests__/utils/file-priority.test.ts`) while unreferenced.

---

Generated by dimension 11.03 (Repository and Workspace Context Maps) against `openhands`.
