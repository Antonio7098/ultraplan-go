---
name: ultraplan-area-reasoning
description: Perform a focused design deep dive for one selected reasoning area and keep UltraPlan state synchronized.
---

# Area Reasoning

Use when the user manually invokes a specialised reasoning document. Default to completing the analysis and document; give a proposal only when explicitly requested.

Discover the workspace/project/sprint and the requested reasoning area. Inspect project and sprint status, `flow-state.json`, requirements, sprint index, technical handbook, selected evidence, and existing reasoning documents. Confirm the requested reasoning source is listed in the project index and belongs to the current project.

If prerequisites or the reasoning area are missing, state the exact gaps and ask whether to fill them. On approval, complete only the required prerequisite work.

Create or update the appropriate `reasoning/<area>.md`. This stage may be an in-depth collaborative design discussion: examine alternatives, constraints, trade-offs, failure modes, operational consequences, migration paths, testing implications, and evidence. Challenge assumptions and preserve unresolved decisions rather than manufacturing certainty. When the user is discussing interactively, keep the document synchronized with conclusions as they emerge.

Validate the reasoning artifact and applicable sprint stages, reconcile `flow-state.json`, re-run status, and report decisions, rejected alternatives, open questions, changed files, and the next eligible stage.