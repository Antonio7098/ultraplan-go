---
name: ultraplan-smoke
description: Run review-gated sprint smoke verification and synchronize UltraPlan flow state.
---

# Smoke

Use for manual invocation of smoke verification. Default to running smoke; provide only a proposal when explicitly requested.

Discover the workspace/project/sprint and implementation repository. Inspect sprint status, `flow-state.json`, review evidence, smoke manifest/configuration, external harness state, and current git revision. Validate review and smoke prerequisites. Smoke must be review-gated and evidence must match the current implementation.

If prerequisites are missing, stale, unsafe, or blocked, explain the exact gaps and ask whether to fill them. On approval, complete only the necessary prerequisite work. Never use diagnostic overrides unless the user explicitly authorizes them and UltraPlan permits them.

Run the UltraPlan smoke workflow with the configured harness. Preserve raw external evidence in its owning harness and store only governed sprint smoke evidence/state. Assess results honestly; do not convert a failed or blocked result into success.

Validate the smoke stage, reconcile `flow-state.json`, re-run sprint status, and report commands/harness used, result, evidence locations, overrides, failures, changed artifacts, and whether the sprint chain is complete.