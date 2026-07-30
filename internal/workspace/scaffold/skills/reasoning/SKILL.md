---
name: ultraplan-reasoning
description: Synthesize sprint design reasoning, trade-offs, and decisions while keeping UltraPlan state synchronized.
---

# Sprint Reasoning

Use for manual invocation of the final reasoning stage. Default to doing the work; return only a proposal when explicitly requested.

Discover the workspace/project/sprint. Inspect status, `flow-state.json`, requirements, sprint index, technical handbook, all selected area reasoning documents, contracts, evidence, and existing `reasoning.md`. Validate prerequisite stages before proceeding. If material prerequisites are absent or invalid, identify the exact gaps and ask whether to fill them; on approval, complete only the required stages and validate each.

Create or update `reasoning.md` with the effective project/workspace/built-in prompt and template. This can be a deep design session with the user. Compare viable approaches, make trade-offs explicit, trace decisions to evidence and requirements, cover failure and recovery semantics, observability, security, compatibility, migration, and testing, and record uncertainty honestly. Keep the document synchronized as discussion changes decisions.

The result must converge sufficiently to govern planning without pretending every implementation detail is settled.

Validate reasoning, reconcile flow state, re-run sprint status, and report decisions, rejected options, open questions, changed files, and the next eligible stage.