# Source Analysis: agent-framework

## Dimension 11.03: Repository and Workspace Context Maps

### Source Info

| Field | Value |
|-------|-------|
| Name | agent-framework (Microsoft Agent Framework) |
| Path | `studies/agent-harness-study/sources/agent-framework` |
| Language / Stack | Python (`agent-framework-core` + packages), C#/.NET (`Microsoft.Agents.AI*`); `go/` is a README stub pointing at a separate repo |
| Analyzed | 2026-08-25 |

## Summary

Microsoft Agent Framework does **not** build repository or workspace context maps. There is no repo-map generator, no symbol indexer (no tree-sitter, ctags, AST outline, or definition index exposed to agents), and no ranked/scoring file selector. Case-insensitive searches for `repo[_ -]?map`, `repository[_ -]?map`, `code.?index`, `symbol_index`, `ctags`, `tree[-\s]?sitter`, and `outline` across `python/packages/**/*.py` and `dotnet/src/**/*.cs` return zero agent-facing matches; the only `ast` use is DevUI sample discovery (`python/packages/devui/agent_framework_devui/_discovery.py:556-559`) and Roslyn appears only in compile-time workflow source generators.

Instead, the framework answers "can the model find the right file without being told the path?" through a **tool-mediated exploration model**: a sandboxed file store rooted at `{cwd}/agent-file-memory`, explored level-by-level via `file_access_ls` with glob filters and searched recursively via case-insensitive regex `file_access_grep`, in both Python (`python/packages/core/agent_framework/_harness/_file_access.py:1488, 1548`) and .NET (`dotnet/src/Microsoft.Agents.AI/Harness/FileAccess/FileAccessProvider.cs:89-92`). Around this sit three genuine workspace-context mechanisms: (1) **self-curated, incrementally rebuilt markdown memory indexes** — `memories.md` (`python/packages/core/agent_framework/_harness/_file_memory.py:77-78, 280-299`) auto-injected every run, and `MEMORY.md` as an "always-loaded table of contents" with keyword-scored topic selection (`python/packages/core/agent_framework/_harness/_memory.py:32, 1078-1098, 1167-1295`); (2) **filesystem skill discovery** scanning for `SKILL.md` up to depth 2 (`python/packages/core/agent_framework/_skills.py:2790-2794, 3545-3560`; .NET twin `dotnet/src/Microsoft.Agents.AI/Skills/File/AgentFileSkillsSource.cs:21-32`); and (3) a **shell-environment snapshot context provider** reporting OS/cwd/tooling into context (`python/packages/tools/agent_framework_tools/shell/_environment.py:103-104, 271-272`). Conversation-scoped summarization/compaction is fully mature but targets message history, not workspace state. The deliberate design position: the framework provides safe primitives plus self-maintained indexes of *its own* memory; structural understanding of a codebase is delegated entirely to the model's exploration tools (or hosted `file_search` passthroughs such as `python/packages/openai/agent_framework_openai/_chat_client.py:1339-1372`).

## Rating

**4 / 10** — Present but partial and asymmetric. For the dimension's core subject — repo maps, symbol indexing, and scored file selection over a repository — there is **no implementation** ("No evidence found" after exhaustive pattern search; see Questions/Gaps). What keeps this above the 1–3 floor is that the adjacent machinery is real, tested, and incrementally maintained: the memory index rebuilds after every write/delete and self-heals when corrupt (`_file_memory.py:266-299, 496-520`), topic selection scores entries against input keywords (`_memory.py:1078-1098`), file enumeration has glob filtering plus symlink/junction safety with dedicated tests (`test_harness_file_access.py:98, 341, 404, 429`), and the same model exists in both languages (.NET `FileMemoryProvider.cs:66-67, 440-489`). It stays below 7–8 because the mechanisms target the agent's *own task sandbox*, not a code repository: no structural map, no symbol layer, no relevance ranking of project files, so the dimension's headline question is answered generically (grep harder) rather than by design.

## Evidence Collected

Every entry cites paths relative to the selected source root.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Repo map generation | **Absent.** Searches for `repo[\s_-]?map`, `repository[\s_-]?map`, `code.?map`, `project.?map`, `code.?index`, `symbol_index`, `outline`, `ctags`, `tree[-\s]?sitter` over `.py`/`.cs` sources returned zero agent-facing matches | (search-based negative result) |
| Working-directory anchoring | Harness default file-memory root is `{cwd}/agent-file-memory` | python/packages/core/agent_framework/_harness/_agent.py:186; dotnet/src/Microsoft.Agents.AI.Harness/HarnessAgent.cs:339 |
| Confined shell workdir | Shell executors support configurable, confinable working directories; Docker default `/workspace` | dotnet/src/Microsoft.Agents.AI.Tools.Shell/LocalShellExecutorOptions.cs:43-51; dotnet/src/Microsoft.Agents.AI.Tools.Shell/DockerShellExecutor.cs:75 |
| File selection tool (ls) | `file_access_ls` lists direct children with optional glob filter (`_matches_glob`, fnmatch semantics) | python/packages/core/agent_framework/_harness/_file_access.py:1488-1502, 193 |
| File selection tool (grep) | `file_access_grep` recursive case-insensitive regex search returning relative paths + snippets + line numbers | python/packages/core/agent_framework/_harness/_file_access.py:1548-1561; dotnet/src/Microsoft.Agents.AI/Harness/FileAccess/FileAccessProvider.cs:42, 425-451 |
| Glob matching (.NET) | `Matcher` from `Microsoft.Extensions.FileSystemGlobbing`; `StorePaths.CreateGlobMatcher` | dotnet/src/Microsoft.Agents.AI/Harness/FileAccess/FileAccessProvider.cs:340; dotnet/src/Microsoft.Agents.AI/Harness/FileStore/StorePaths.cs:93-101 |
| Tool wiring + approvals | Read/ls/grep always on; write/delete/replace approval-wrapped in `CreateTools()` | dotnet/src/Microsoft.Agents.AI/Harness/FileAccess/FileAccessProvider.cs:41-47, 453-480 |
| Symbol indexing | **Absent as runtime feature.** Only incidental `ast.parse` in DevUI sample discovery; Roslyn confined to build-time workflow generators | python/packages/devui/agent_framework_devui/_discovery.py:556-559 |
| Memory index file | `memories.md` index, capped at `_MAX_INDEX_ENTRIES = 50`, rebuilt after every write/delete | python/packages/core/agent_framework/_harness/_file_memory.py:77-78, 280-299, 342, 384 |
| Index injection each run | `before_run` injects "your memory index — a list of files you have previously written"; corrupt index skipped with warning, self-heals next write | python/packages/core/agent_framework/_harness/_file_memory.py:496-520 |
| .NET memory index twin | Same model: `memories.md`, cap 50, `RebuildMemoryIndexAsync` | dotnet/src/Microsoft.Agents.AI/Harness/FileMemory/FileMemoryProvider.cs:66-67, 148-158, 440-489 |
| MEMORY.md table of contents | `DEFAULT_MEMORY_INDEX_FILE_NAME = "MEMORY.md"`, index caps (200 lines × 150 chars), injected as "always-loaded table of contents" | python/packages/core/agent_framework/_harness/_memory.py:32, 44-68, 1295 |
| Keyword-scored topic selection | `_topic_score` counts keyword overlap; `_select_topics` sorts and takes top-N (default 3) with score > 0 | python/packages/core/agent_framework/_harness/_memory.py:1078-1098 |
| LLM memory consolidation | Extraction prompt (durable facts from transcript deltas) + consolidation prompt (summarize a topic file) = incremental summarization of agent state | python/packages/core/agent_framework/_harness/_memory.py:44-68 |
| Skill discovery by directory scan | `FileSkillsSource` scans roots up to `MAX_SEARCH_DEPTH = 2` for `SKILL.md`; fails closed on symlinks/junctions; harness defaults to process cwd | python/packages/core/agent_framework/_skills.py:2790-2794, 1649, 3545-3560; dotnet/src/Microsoft.Agents.AI.Harness/HarnessAgent.cs:354-358 |
| Shell environment snapshot | `ShellEnvironmentProvider(ContextProvider)` injects OS/cwd/tooling block incl. `working_directory` field | python/packages/tools/agent_framework_tools/shell/_environment.py:39-57, 103-104, 271-272; dotnet/src/Microsoft.Agents.AI.Tools.Shell/ShellEnvironmentProvider.cs:155-185, 274-276 |
| Hosted file-search passthrough | Provider-side semantic file search (`type="file_search"`), protocol `SupportsFileSearchTool`; .NET maps `HostedFileSearchTool` | python/packages/openai/agent_framework_openai/_chat_client.py:1339-1372; python/packages/core/agent_framework/_clients.py:789; dotnet/src/Microsoft.Agents.AI.AzureAI.Persistent/PersistentAgentsClientExtensions.cs:399 |
| Conversation summarization (adjacent) | `SummarizationStrategy` replaces older groups with LLM summary; harness applies token-budget compaction pre/post turn | python/packages/core/agent_framework/_compaction.py:1197, 1176-1191; python/packages/core/agent_framework/_harness/_agent.py:82-142 |
| Tests: file access | Glob case-insensitivity, recursive search with snippets, invalid/oversize regex rejection, symlink/junction skipping | python/packages/core/tests/core/test_harness_file_access.py:98, 167-185, 248, 341, 357, 404, 429 |
| Tests: memory index/topic selection | Index round-trip/trim, unchanged-index not rewritten, concurrent-write preservation, injection behavior | python/packages/core/tests/core/test_harness_memory.py:95, 134, 292, 378, 640 |

## Answers to Dimension Questions

**1. Does the system build a map of the repository?**
No. No component constructs a structural map of a repository or workspace. The nearest analogs are (a) the sandboxed file store rooted at the cwd that agents explore manually (`python/packages/core/agent_framework/_harness/_agent.py:186`; `dotnet/src/Microsoft.Agents.AI.Harness/HarnessAgent.cs:339`), and (b) the self-written `MEMORY.md`/`memories.md` indexes, which map the agent's *own prior outputs*, not the repository (`python/packages/core/agent_framework/_harness/_memory.py:1295`; `python/packages/core/agent_framework/_harness/_file_memory.py:77`). There is also no directory-tree rendering tool — enumeration is strictly level-by-level `ls` (pattern search `directory.?tree|tree.?view` → zero matches).

**2. How are relevant files selected?**
By the model itself, using tools rather than a selector: `file_access_ls` with optional glob filters (`python/packages/core/agent_framework/_harness/_file_access.py:1488-1502`, fnmatch via `_matches_glob`:193; .NET `Matcher` at `dotnet/src/Microsoft.Agents.AI/Harness/FileStore/StorePaths.cs:93-101`) and recursive regex `file_access_grep` (`_file_access.py:1548-1561`; .NET `FileAccessProvider.cs:425-451`). No scoring, ranking, embedding, or recency-based file prioritization exists anywhere. Two indirect selection mechanisms do exist: keyword-scored selection of *memory topics* loaded into context (top-N=3 by keyword overlap, `_memory.py:1078-1098`) and hosted provider-side `file_search` for externally uploaded stores (`python/packages/openai/agent_framework_openai/_chat_client.py:1339-1372`), which is a cloud RAG passthrough, not local repo selection.

**3. Are symbols indexed for the model?**
No. No ctags/tree-sitter/symbol-outline subsystem exists (pattern searches returned nothing). The only `ast` usage is DevUI discovering `agent = ...`/`workflow = ...` exports in sample files (`python/packages/devui/agent_framework_devui/_discovery.py:556-559`), and Roslyn usage is confined to compile-time workflow source generators — neither is agent-facing context. Code understanding is left entirely to whatever the agent reads via `file_access_read`/grep/shell tools.

**4. Is workspace context stale or fresh?**
Fresh for what it covers. The `memories.md` index is rebuilt synchronously after every write/delete under a per-instance lock (`python/packages/core/agent_framework/_harness/_file_memory.py:266-299, 342, 384`); if the index is unreadable, injection is skipped with a warning and the index self-heals on the next successful write (`_file_memory.py:496-520`). `MemoryContextProvider` avoids rewriting an unchanged index (tested at `python/packages/core/tests/core/test_harness_memory.py:292`). Skill discovery re-scans the filesystem per resolution (`python/packages/core/agent_framework/_skills.py:2915`), and the shell environment snapshot reports live cwd/tool versions per run (`python/packages/tools/agent_framework_tools/shell/_environment.py:194, 271-272`). However, because there is no repository map at all, nothing can be stale-or-fresh about actual repo structure — freshness applies only to the agent's own memory and environment.

## Architectural Decisions

1. **Exploration over mapping.** Rather than precomputing a repo/workspace map, the framework exposes safe primitives (`ls`+glob, recursive regex grep, read/write with approvals — `dotnet/src/Microsoft.Agents.AI/Harness/FileAccess/FileAccessProvider.cs:41-47, 453-480`) and lets the LLM drive discovery. This trades token efficiency and determinism for simplicity and language-agnosticism.
2. **Sandbox-first scoping.** Context maps would be dangerous against arbitrary host filesystems; instead the harness scopes all file tools to a task store rooted at `{cwd}/agent-file-memory/{timestamp}_{guid}` (`dotnet/src/Microsoft.Agents.AI.Harness/HarnessAgentOptions.cs:240`), with confined shell workdirs (`dotnet/src/Microsoft.Agents.AI.Tools.Shell/LocalShellExecutorOptions.cs:43-51`) and fail-closed skill discovery across symlinks (`python/packages/core/agent_framework/_skills.py:3552-3560`).
3. **Self-maintained markdown indexes as the context map surrogate.** Both memory providers maintain plain-markdown index files (`memories.md`, `MEMORY.md`) that function as always-loaded tables of contents into larger stores (`python/packages/core/agent_framework/_harness/_memory.py:1295`), rebuilt incrementally and injected automatically — a deliberately inspectable, human-editable format.
4. **Dual-language parity.** Every mechanism described here exists twice, near-identically: Python `_file_memory.py` ↔ .NET `FileMemoryProvider.cs`, Python `_file_access.py` ↔ .NET `FileAccessProvider.cs`, Python `_environment.py` ↔ .NET `ShellEnvironmentProvider.cs`.
5. **Hosted offloading where intelligence is needed.** Semantic file search is delegated to provider APIs (`SupportsFileSearchTool`, `python/packages/core/agent_framework/_clients.py:789`) instead of building a local embedding index.

## Notable Patterns

- **Context-provider pipeline as the injection point:** `ContextProvider.before_run` is the uniform hook through which memory indexes, environment snapshots, and compaction enter the prompt (`python/packages/core/agent_framework/_sessions.py:793-797`; `python/packages/tools/agent_framework_tools/shell/_environment.py:103-104`).
- **Keyword-overlap retrieval as a poor-man's ranker:** `_select_topics` scores index entries by word intersection with the user's input and loads only score > 0 hits, capped at N (`python/packages/core/agent_framework/_harness/_memory.py:1078-1098`) — cheap, deterministic, explainable.
- **Index-as-cache with self-healing:** the index is derived state; any corruption degrades gracefully to "skip injection" and repairs on next mutation (`python/packages/core/agent_framework/_harness/_file_memory.py:496-520`).
- **Depth-bounded discovery with trust boundaries:** skill scanning stops descending once a `SKILL.md` boundary is found and refuses symlinked roots (`python/packages/core/agent_framework/_skills.py:3548-3560`), mirroring the symlink/junction-skipping in file-store search tests (`python/packages/core/tests/core/test_harness_file_access.py:404, 429`).
- **Glob-filtered tool surfaces everywhere:** the same filter parameter pattern recurs in `file_access_ls`, grep search, and memory listing — one consistent selection vocabulary instead of per-tool ad hoc flags.

## Tradeoffs

- **Token economics vs. correctness:** without a repo map, an agent must spend multiple `ls`/grep round-trips to locate relevant files; a map would compress this, but maps go stale and are expensive to build correctly across languages. The framework chose the former cost explicitly.
- **Markdown indexes vs. structured indexes:** human-readable, model-friendly, diffable — but capped (50 entries / 200 lines × 150 chars) and unranked beyond keyword overlap, so large stores lose fidelity (`python/packages/core/agent_framework/_harness/_file_memory.py:78`; `python/packages/core/agent_framework/_harness/_memory.py:44-68` region).
- **Sandbox safety vs. real-repo usefulness:** the confinement-first design means the out-of-the-box experience cannot map a real checkout even when an integrator wants it; doing so requires composing raw shell tools with their own policy burden.
- **Parity maintenance cost:** dual implementations double the surface where drift bugs can appear (mitigated by mirrored doc comments and test suites in both languages).

## Failure Modes / Edge Cases

- **Corrupt/unreadable index:** handled — skip injection, warn, self-heal on next write (`python/packages/core/agent_framework/_harness/_file_memory.py:498-512`).
- **Regex abuse in grep:** invalid and oversize patterns are rejected (`python/packages/core/tests/core/test_harness_file_access.py:248`); catastrophic-backing regexes are bounded by oversize rejection, though no explicit step/time budget was found.
- **Symlink/junction escape:** both search and skill discovery skip link boundaries fail-closed (`python/packages/core/tests/core/test_harness_file_access.py:404, 429`; `python/packages/core/agent_framework/_skills.py:3552-3560`).
- **Concurrent writes to the same memory topic:** preserved via atomic write + locking (tested `python/packages/core/tests/core/test_harness_memory.py:640, 695`).
- **Uncovered gaps:** no eviction/aging policy for memory topics beyond entry caps; keyword selection silently returns nothing when input keywords don't overlap the index (`_memory.py:1087-1089`), meaning the agent gets no topics rather than a fallback "recent" set; no cache invalidation concern exists only because the index is always rebuilt — at scale this becomes O(store) work per mutation.

## Future Considerations

- A lightweight **directory-tree renderer or depth-limited workspace outline tool** would close the biggest gap between "ls one level" and full repo awareness at modest cost.
- Extending the keyword-scored topic-selection pattern (`_memory.py:1078-1098`) to **score/rank project files** (path/name/recency heuristics before embeddings) would give the model a principled first probe of an unfamiliar workspace.
- An optional **local symbol-outline provider** (even regex/heuristic based, per-language) would let code-focused agents answer "where is X defined?" without multi-hop grepping.
- Calibration of the memory-index caps against observed store sizes, and an aging/eviction strategy, would keep the "table of contents" useful as stores grow.

## Questions / Gaps

- **Repo map generation:** No evidence found. Searched `repo[\s_-]?map`, `repository[\s_-]?map`, `code.?map`, `project.?map`, `codebase.*(index|map|summary)` over all `.py`/`.cs` under `python/packages` and `dotnet/src` — zero matches.
- **Symbol indexing:** No evidence found. Searched `ctags`, `tree[-\s]?sitter`, `treesitter`, `symbol_index`, `code.?index`, `outline` — only incidental `ast` in DevUI sample discovery (`python/packages/devui/agent_framework_devui/_discovery.py:556-559`) and build-time-only Roslyn generators.
- **Scored/ranked file selection:** No evidence found beyond memory-topic keyword scoring; no embedding or vector index of local files exists (Azure AI Search package is external-RAG over caller-supplied data, not repo indexing).
- **Go implementation:** Out of scope for this checkout — `go/README.md` defers to the separate `microsoft/agent-framework-go` repository; whether that repo has workspace-mapping features could not be verified here (source-isolation rule).
- **Workspace-state summarization of a *repository*** (build status, project layout, dependency graph): No evidence found; nearest analogs summarize conversation history (`python/packages/core/agent_framework/_compaction.py:1197`) and the shell environment (`python/packages/tools/agent_framework_tools/shell/_environment.py:103-104`).

---

Generated by `dimensions/11.03-repository-and-workspace-context-maps.md` against `agent-framework`.
