---
name: ultraplan-sprint-index
description: Create or update the current sprint index while keeping UltraPlan flow state synchronized.
---

# Sprint Index

Use this skill only when the user manually invokes the sprint-index stage.

## Operating mode

Default to performing the stage. Produce a proposal only when the user explicitly asks for one.

1. Discover the UltraPlan workspace, project, and sprint from the current directory and user request.
2. Run `ultraplan sprint <project> <sprint> status` and `ultraplan sprint <project> <sprint> validate sprint-index`.
3. Inspect `flow-state.json` and the current artifact chain. Treat the CLI and files on disk as the source of truth.
4. Check prerequisites. The sprint must have usable requirements and the project index/catalogue must be valid enough to select contracts, evidence, reasoning documents, review protocols, and smoke configuration.
5. When prerequisites are missing, explain the concrete gaps and ask whether to fill them. If approved, complete only the necessary prerequisite stages, validating after each one.
6. Create or update `sprint-index.md` using the effective UltraPlan prompt/template and selected project evidence. Do not merely describe what should be written.
7. Validate the completed stage, reconcile `flow-state.json`, and re-run sprint status. Never leave artifact validity and flow state out of sync.

## Quality bar

The sprint index must define scope, selected evidence, applicable contracts, reasoning inputs, review protocol, smoke harness, and explicit exclusions. References must exist and remain project-local where required.

## Completion response

Report what changed, validations run, remaining gaps, and the next eligible stage.