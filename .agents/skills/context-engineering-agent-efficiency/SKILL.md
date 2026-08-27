---
name: context-engineering-agent-efficiency
description: Audit or redesign multi-stage LLM and agent workflows to reduce repeated discovery, tool calls, prompt tokens, latency, and cost. Use for context maps, code-context packs, stage handoffs, prompt-prefix caching, session reuse, model routing, or replacing agents with deterministic code and bounded API calls. Do not use for ordinary single-agent coding tasks with no workflow or context-routing question.
---

# Context engineering and agent efficiency

Design the workflow so each model call starts with the evidence it needs and does the smallest amount of work that still requires model judgment.

The target is not the fewest input tokens in isolation. Optimize successful-work cost and elapsed time while preserving evidence quality, safety, and correctness.

## Start with evidence

Inspect the implementation, rendered requests, runtime configuration, artifacts, retry paths, and available telemetry. Do not infer a stage's inputs from documentation alone.

Enumerate every model call, including repair turns, retries, fan-out workers, challengers, evaluators, summarizers, and conflict handlers. Also record deterministic stages with no model call. A product stage may contain several different workloads and those workloads may need different executors.

If measurements are missing, say so. Separate measured results, deterministic byte or token comparisons, and design estimates.

## Build the current-state map

For each call, record:

- purpose and success condition;
- producer and consumer;
- model, reasoning setting, output format, tools, permissions, and working directory;
- every supplied input, every path merely mentioned, and every live source the model must discover;
- required, optional, and forbidden inputs;
- delivery mode: inline full content, selected excerpt, structured packet, file reference, retrieval, live repository access, or conversation state;
- size, trust boundary, freshness identity, ordering, and reuse group;
- expected output, validation, repair behavior, and persistence;
- fan-out count, session reuse, retries, tool calls, tokens, cache reads and writes, latency, and cost when known.

Use the templates in [references/audit-templates.md](references/audit-templates.md). When auditing UltraPlan, also read [references/ultraplan-current-map.md](references/ultraplan-current-map.md) and verify it against current code before relying on it.

## Trace context provenance

Treat context as a produced artifact with an owner and lifecycle. For each important fact, identify who discovers it, where it is stored, which calls consume it, what makes it stale, and whether consumers receive the fact or only a pointer to it.

A file path in a prompt is not supplied context. Count the read, search, and interpretation work required to turn that pointer into usable evidence.

Prefer canonical, validated handoffs over prose summaries when downstream work needs exact facts. Include provenance, revision identity, assumptions, omissions, and unresolved questions. Do not make a prepared pack an exclusive boundary. A downstream agent may inspect live sources when verification or changed state requires it.

## Engineer code context once

When several calls need the same repository understanding, assign one upstream exploration owner. That call should derive its search questions from requirements and inspect the repository broadly enough to produce a reusable code-context pack.

The pack should contain or resolve:

- repository revision or worktree identity;
- requirement-to-code mappings;
- relevant paths, symbols, and exact ranges or content-addressed excerpts;
- call paths, data flow, interfaces, configuration, error behavior, and tests;
- existing behavior, partial implementations, contradictions, and notable absences;
- reasons for every selection;
- uncertainties and cases that still require live inspection.

Normalize overlapping selections and read each source once. Validate containment, encoding, ranges, reference counts, source-line budgets, and total request size before promotion. Freeze the shared evidence for the workflow when later edits would otherwise churn every prompt prefix.

Judge a code-context pack by downstream discovery avoided, not by its length. Shipping a small decision-relevant file in full can be cheaper than causing repeated reads. Shipping a repository dump is usually worse than targeted evidence plus live access.

## Find waste

Flag these patterns with concrete evidence:

- two or more calls repeat the same search or repository walk;
- a prompt names governed files that the model then has to find and read;
- the same content appears in a shared prefix and again in a stage packet;
- every fan-out worker receives content owned by only one worker;
- volatile IDs, timestamps, paths, or stage text appear before reusable content;
- a repair resends the original request instead of continuing the same session;
- related tasks start fresh sessions and reconstruct prior state;
- a model performs parsing, sorting, mapping, validation, fingerprinting, diffing, scheduling, or state transitions that code can own;
- a no-tool, single-turn judgment runs inside a full agent runtime;
- oversized prompts increase failures or latency without reducing tool work;
- optimizations report fewer bytes or calls without measuring final quality.

## Choose the smallest executor

Classify each workload independently.

Use deterministic code when rules can produce the answer. Typical examples are discovery indexes, dependency manifests, input validation, context selection by declared references, diffs, normalization, fingerprints, queueing, retry policy, and synthesis over already classified records.

Use a one-off model API call when all needed evidence can be supplied up front, the task needs judgment but no environment exploration, the output has a bounded schema, one validation or repair turn is enough, and the call has no open-ended side effects.

Use an agent when the model must search an unknown environment, choose tools based on observations, modify several files, run an iterative verification loop, or recover from state that cannot be fully packaged beforehand.

Keep an agent only for the part that needs agency. Deterministic orchestration should own permissions, state transitions, retries, promotion, and validation.

## Design cacheable requests

Treat provider caching as part of the request architecture, not an adapter detail added at the end.

First verify the current provider documentation. Cache rules, minimum lengths, retention, write pricing, breakpoint support, routing keys, and usage fields can change.

Build requests in decreasing order of reuse and stability:

1. Stable platform, tool, policy, and workflow instructions.
2. Versioned task-family instructions and stable tool definitions.
3. Canonical project or sprint foundation.
4. Frozen code-context evidence.
5. A cache boundary.
6. Stage-common context, followed by another boundary when the provider supports it and reuse justifies it.
7. Per-call, per-task, or per-worker inputs.

Keep shared bytes identical. Use canonical serialization and deterministic ordering. Exclude timestamps, run IDs, attempt numbers, output paths, random identifiers, and changing diagnostics from reusable prefixes unless they define the cache cohort.

Remember that the rendered context can include hidden instructions, developer content, tool definitions, conversation history, output schemas, model settings, and reasoning settings. A stable user prompt alone does not prove cache compatibility. Define the cohort using every setting that can change the rendered prefix.

Use a stable cache key derived from the workflow family, provider, model, region or retention boundary when relevant, tool set, policy version, output format, and prefix digest. A cache key helps routing; it does not prove a hit.

Place explicit breakpoints at reuse boundaries when supported. Do not pay to write a volatile suffix that is unlikely to be reused. When requests have nested reuse groups, compare one shared workflow boundary with additional stage-common boundaries.

Schedule calls that share a prefix close enough to fit the provider's cache lifetime. For concurrent fan-out, measure whether a real first request should establish the cache before the rest start. Do not add a synthetic warm-up call without an economic and latency test.

Track provider-reported cached input and cache-write tokens. Compare cache economics across successful runs, not requests alone.

## Reuse sessions deliberately

Use one session for an ordered queue when tasks share context and later tasks benefit from earlier edits or observations. Send the full governed packet once, then use compact continuation turns with the current task and changed facts.

Keep repairs in the same compatible session. A repair should contain validation failures, the required output contract, and only the data needed to correct the result. Tell the model not to repeat exploration when the evidence remains valid.

Start a fresh session when permissions, work directory, provider, model, policy, source revision, or task objective changes enough to invalidate the prior state.

## Produce a redesign, not a list of tips

Return these artifacts unless the user asks for a narrower answer:

- a current-state call graph and context ledger;
- evidence-backed duplication and discovery findings;
- a proposed context production and handoff design;
- an executor decision for every model call;
- exact cache prefix groups and breakpoint locations;
- a prioritized implementation plan with expected effect and risk;
- a measurement and regression plan.

For each recommendation, name the current behavior, proposed owner, affected calls, context bytes or tool work removed, cache effect, quality risk, and proof required.

## Measure the result

Instrument every call with prompt and prefix bytes, prefix digest, cache cohort, input, output, reasoning, cached-input and cache-write tokens, model, settings, tool calls by kind, turns, retries, repair bytes, queue position, duration, cost, status, and error category.

Evaluate representative successful workflows before and after. Report:

- task success and output validation;
- evidence completeness and defect escape rate;
- repeated searches and reads;
- tool calls and turns;
- fresh input, cached input, cache writes, output, and reasoning tokens;
- time to first output and end-to-end duration;
- cost per successful workflow;
- retry, timeout, and malformed-output rates.

Do not claim an improvement when quality falls or failed work merely becomes cheaper.

## Guardrails

- Do not confuse maximum context with useful context.
- Do not hide required evidence behind retrieval when the orchestrator already owns it.
- Do not inline large mutable files only to avoid all tool calls. Compare inline cost with targeted reads.
- Do not make provider-specific caching behavior a correctness dependency.
- Do not rerun an expensive upstream exploration because a disposable cache entry is missing.
- Do not use model judgment for a deterministic state transition.
- Do not recommend a smaller model or lower reasoning setting without representative evals.
- Preserve authorization and mutation boundaries while changing context delivery.

