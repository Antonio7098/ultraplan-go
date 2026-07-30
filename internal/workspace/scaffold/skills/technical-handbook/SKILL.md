---
name: ultraplan-technical-handbook
description: Distill selected evidence into the sprint technical handbook and synchronize UltraPlan state.
---

# Technical Handbook

Use for manual invocation of the technical-handbook stage. Do the stage by default; propose only when explicitly asked.

Discover the workspace/project/sprint, inspect status and `flow-state.json`, then validate requirements and sprint index. Confirm that selected contracts and evidence exist and are readable. When prerequisites are missing, describe the precise gaps and ask whether to fill them; if approved, complete and validate only those prerequisite stages.

Create or update `technical-handbook.md` with the effective UltraPlan prompt/template. Distill concrete implementation-relevant patterns, constraints, interfaces, failure semantics, testing approaches, and citations from selected sources. Do not turn it into generic advice or prematurely choose a design that belongs in reasoning.

Validate the handbook, reconcile flow state, re-run sprint status, and report changed artifacts, validation results, unresolved evidence gaps, and the next eligible stage.