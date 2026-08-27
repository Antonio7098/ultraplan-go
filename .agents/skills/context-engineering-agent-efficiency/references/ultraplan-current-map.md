# UltraPlan current context map

This map reflects the repository on 2026-08-27. Verify it against current code before using it in an audit.

## Shared sprint foundation

Sprint index, handbook, area reasoning, final reasoning, plan, execute, review, and smoke use a common prefix assembled in `internal/sprint/prompt_context.go`:

1. shared sprint instructions;
2. project and sprint identity;
3. exact `requirements.md` bytes;
4. exact reference-only `code-context.md` bytes;
5. canonical source excerpts resolved from the code-context references;
6. the stage boundary.

The source selections merge duplicate, adjacent, and overlapping ranges. Runtime composition can reuse a frozen content-addressed pack, so execute edits do not churn the planning prefix. The prefix is bounded, hashed, explained by block, and carried to the runtime with cache metadata. The current agentwrap and OpenCode adapter transports that metadata but does not place provider-native breakpoints.

## Stage and call inputs

| Stage or call | Inputs delivered or made available | Current executor shape |
|---|---|---|
| requirements | Prompt and requirements template; project index; roadmap; project Markdown docs; prior sprint reviews; catalog entries | Agent runtime, although governed inputs are copied directly and the output is one Markdown artifact |
| code-context | Prompt and template; complete validated requirements; read-only sprint implementation worktree | Repository-exploring agent; UltraPlan validates and promotes its returned Markdown candidate |
| sprint-index | Shared sprint foundation; prompt and template; project index; roadmap; project docs; catalog entries | Agent runtime with complete governed inputs |
| technical-handbook | Shared foundation; prompt and template; sprint index; selected evidence reports | Agent runtime with complete selected evidence |
| area-reasoning | Shared foundation; prompt; one selected area template; project docs; sprint index; handbook; selected contracts, evidence reports, and review protocols | One isolated agent call per selected area; sibling templates are forbidden |
| reasoning | Shared foundation; prompt and template; project index; roadmap; project docs; sprint index; handbook; selected context; all required area outputs | Agent runtime with complete governed inputs |
| plan | Shared foundation; prompt and plan template; project index; roadmap; project docs; sprint index; handbook; area outputs; final reasoning | Agent runtime with complete governed inputs |
| execute | Shared foundation; task instructions; project definitions; sprint index; handbook; area outputs; final reasoning; plan; ordered task queue and current task; writable sprint worktree | One agent session owns the ordered queue; later tasks use compact continuation turns |
| review worker | Shared foundation; review instructions and output contract; one coverage contract or the handbook; common governed project and sprint inputs; selected protocols; execution handoff; baseline-to-sprint patch; read-only frozen changed-file paths | Independent read-only agent per coverage item. Raw run state and inline target files are intentionally omitted from the direct packet |
| smoke author | Shared foundation; smoke prompt; sprint index; handbook; area outputs; final reasoning; plan; execute summary; review outcome; execution handoff; harness manifest and allowed paths; read-only product target; writable harness scope | Restricted agent that may edit only allowlisted harness paths, followed by deterministic discovery and harness execution |
| QA map | Current review manifest and fingerprint; governed input digests; review artifact; changed paths; adjacent context paths; expectation IDs; risk tags; approved check catalog; implementation and Git identity; budgets | Deterministic code |
| QA investigator | Fixed instructions plus canonical shard JSON containing changed paths, context paths, concerns, expectation references, approved checks, implementation fingerprint, and budgets; path-limited read-only target access | Read-only agent per shard; can request bounded context or approved checks |
| QA repair | Parser or validator failures plus the strict output contract | Same-session compact repair turn |
| QA challenger | Frozen retained theory summaries and limits; no tools | No-tool model call |
| failed QA evidence evaluator | Immutable evidence record and frozen evidence plan; no tools; three observations adjudicated by code | Three no-tool model calls plus deterministic adjudication |
| QA synthesis | Validated shard theories, challenges, blockers, and policy limits | Deterministic code |
| merge description | Deterministic merge inspection packet; read-only source worktree | Read-only model call that returns bounded JSON |
| merge conflict reconciliation | Merge state and listed conflict paths; writable target limited by post-run digest checks | Agent edits only active conflict paths; Git mechanics remain deterministic |

## What is already working

The code-context stage moved shared exploration upstream. Recorded post-change sessions show 45 to 144 code-context exploration calls, while downstream reviewers usually perform targeted reads with little or no search. Some reviews completed without tools. The stable prefix has produced substantial provider-reported cache reads. Execute sends its shared packet once per queue, and code-context validation repairs continue in the same session with small prompts.

Review context was tightened after oversized prompts caused runtime failures. Review now supplies a complete implementation patch and execution handoff, while changed target files remain available at exact frozen paths instead of being copied inline.

## Proposed next audit targets

### Add stage-common cache layers

The shared sprint foundation ends before all stage instructions and direct inputs. Review workers repeat a large common governed packet after coverage-specific text has already diverged. Area-reasoning calls have a similar common stage packet followed by one area-specific template.

Design nested request groups where the transport supports them:

1. sprint foundation;
2. review-common or reasoning-common packet;
3. worker-specific request.

Measure whether a second breakpoint improves cache reads enough to justify cache-write cost. Keep correctness independent of a hit.

### Give QA semantic context instead of identifiers alone

QA investigator packets carry expectation reference IDs but not necessarily the referenced acceptance text. Their tool policy permits only assigned implementation paths, so a reference may be unusable unless its meaning is encoded elsewhere in the packet.

Build a frozen QA foundation from the relevant requirements, current review findings, execution handoff, and baseline diff. Put shared QA evidence before a cache boundary. Add only each shard's exact expectation text, changed hunks, and context paths after that boundary. Compare this against the current live-read design for prompt size, tool calls, theory quality, and defect detection.

### Replace bounded no-tool agents with direct calls

Strong first candidates are QA challenger, failed-evidence evaluator, and merge description. They receive complete packets, have no permitted tools, and return bounded structures. Use direct Responses API calls with structured output, strict validation, and one compact repair when needed.

Requirements, sprint index, handbook, area reasoning, final reasoning, and plan are conditional candidates. Their governed inputs are already copied into one request and UltraPlan owns validation and artifact promotion. Verify from telemetry that they do not use tools or need iterative observation before moving them out of the agent runtime.

Keep code-context, execute, smoke authoring, QA investigation, and conflict reconciliation as agents while they still need live exploration, iterative tools, or bounded writes.

### Finish the metrics loop

`.runtime-metrics.json` records prompt split, tokens, cache reads and writes, turns, cost, duration, status, and errors. It does not record tool-call counts by kind for every sprint runtime call. QA stores an observed tool-call count separately.

Add common per-call tool counts, repair and retry relationships, and continuation bytes to runtime metrics. Preserve enough baseline telemetry to compare complete successful sprints without relying on the OpenCode database retention window.

### Audit cache cohorts end to end

The current prompt explanation and runtime metadata describe the intended stable prefix, but provider-native breakpoint support remains outside the sprint package. Trace provider, model, reasoning setting, output format, tool definitions and order, permission policy, work directory, region, retention mode, and cache key through the final adapter.

The audit should prove which bytes and settings reach the provider, not only which metadata UltraPlan computes.

