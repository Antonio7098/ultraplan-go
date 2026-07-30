---
name: ultraplan-requirements
description: Create or update sprint requirements and synchronize UltraPlan flow state.
---

# Requirements

Use when the user manually invokes the requirements stage. Default to doing the work; provide only a proposal when explicitly requested.

1. Discover the workspace, project, and sprint.
2. Read project docs, roadmap, project index, existing sprint artifacts, and `flow-state.json`.
3. Run sprint/project status and validate the project catalogue.
4. Check prerequisites: a defined sprint intent and enough project context to state outcomes, boundaries, constraints, acceptance criteria, and non-goals.
5. If inputs are missing, identify exact gaps and ask whether to fill them. On approval, gather or create only what is required.
6. Create or update `requirements.md` from the effective UltraPlan prompt/template. Resolve ambiguity where evidence permits; record genuine open decisions explicitly.
7. Validate requirements, reconcile flow state, and re-run status so disk artifacts and state agree.

Requirements must be implementation-neutral where possible, testable, traceable to project goals, explicit about exclusions, and strong enough to govern later design and review.

Finish by reporting changes, validation, unresolved decisions, and the next eligible stage.