# Did the Code Context Stage Improve Downstream Agent Efficiency?

Date: 2026-08-24
Question: has the code-context planning stage (and the follow-on context-reuse work) materially made downstream agents faster and less exploratory?

## 1. Implementation timeline (git)

| Commit | Date | Change |
|---|---|---|
| `c267c69` | 2026-07-31 | Plan doc for the sprint code-context stage |
| `2d10aba` | 2026-08-20 | Add code context planning stage (`code_context.go`, validation, flow wiring) |
| `c5f3d5b` | 2026-08-20 | Make code context **reference-only and self-repairing** (validated file resolution, line ranges, budgets before promotion) |
| `90e251d` | 2026-08-21 | Complete shared context / workflow safety |
| `30e9574` | 2026-08-21 11:47 | **Optimize sprint context and execution reuse**: frozen context packs, ordered prompt blocks with stable prefix, direct governed inputs per stage, single-session execute queue, `.runtime-metrics.json` capture |
| `8c4f83e` | 2026-08-24 | Keep validation repairs in the same session |
| `2da8d09`, `7fab321`, `2076339` | 2026-08-24 | Recent shake-down fixes (area-reasoning recovery, prompt truncation limits removed, snapshots disabled) |

## 2. Evidence sources and their limits

- **Live agent telemetry exists only from 2026-08-21 onward.** The OpenCode store (`~/.local/share/opencode/opencode.db`) contains no sessions earlier than 2026-08-21 18:09, and `.runtime-metrics.json` exists only for sprints 35–37. Sprints ≤34 (the true "before" era, including sprint 23's original execute stage and sprints 30–32 reviews finished 2026-08-21 08:19) have no per-session tool/token records.
- Therefore a **live pre/post A/B of tool calls and wall-clock is not reconstructable**. What we do have: (a) the deterministic A/B measured at the optimization commit, (b) rich post-change telemetry for sprints 35–37, and (c) DB session forensics for managed-run sessions.
- The prior audit (`docs/reports/sprint-efficiency.md`) itself explicitly declines to claim token/cost savings without real telemetry. This report uses the telemetry that now exists.

## 3. Deterministic A/B (baseline commit vs `30e9574`)

From `docs/reports/sprint-efficiency.md`, same fixture:

- Rendered prompt totals grew 54,492 → 77,521 bytes (+42.3%) — intentional: material is shipped instead of rediscovered.
- Two-task execution: two independent task prompts = 19,718 bytes vs one shared execute session = 10,767 bytes (**−45.4%** outbound prompt bytes).
- Shared-context composition benchmark: time/op **−65.5%**, bytes/op −60.2%, allocations/op −34.9%.

## 4. Live telemetry, sprints 35–37 (post-change)

### 4.1 Provider cache hits on a frozen shared prefix — working

Sprint 35 review fan-out (14 completed reviews):

- Every reviewer received the **byte-identical shared prefix**: 64,138 bytes, one digest `c708baf1a42f…` across all 15 attempts.
- Provider-reported `cache_read_tokens`: 265,792 – 892,288 per review, **5,564,224 total vs 1,703,447 charged input tokens** — roughly **3.3× more tokens served from cache than billed as fresh input**. Cache hits are real, though they are provider-automatic (agentwrap does not yet emit native cache breakpoints).

Sprint 36/37 planning stages likewise share a 170,218-byte (36) / 64,056-byte (37) prefix across sprint-index, handbook, area-reasoning, reasoning, plan, review, and smoke runs.

### 4.2 Downstream agents explore very little

Per-session tool forensics (opencode DB, parts by type) for managed-run sessions:

| Session | Tool calls | Breakdown | Duration |
|---|---:|---|---:|
| Sprint 36 code-context (planning) | 144 | 95 read, 36 grep, 13 glob | 6.7 min |
| Sprint 36 code-context (earlier, variant=low) | 45 | 30 read, 8 glob, 7 grep | 3.2 min |
| Sprint 35 reviewers (typical, ×12) | 10–18 | read 10–14, grep 0–8 | 5–9 min |
| Sprint 35 reviewers (best case ×3) | 0 tools | answer from packet alone | 1.4–3.2 min |
| Sprint 36 review-context session | 7 | 7 read | 1.1 min |
| Sprint 35 smoke authoring | 5–13 | bash-heavy | 1.2–7.9 min |

The pattern matches the design intent: **exploration is concentrated once, upstream, in code-context** (45–144 tool calls), while downstream reviewers mostly *read the exact files listed in their governed packets* (median 11 tool calls, almost no searching). Three sprint-35 reviewers completed with zero tool calls. Sprint 36's governed-packet reviews had a **median duration of 2.3 minutes** (n=32, range 0.2–8.5).

### 4.3 Single-session execution queue

The sprint-35 era execute/smoke queue ran as one session with compact continuation turns: e.g., a 13.8-minute session completing a whole task queue with 36 tool calls (24 bash, 7 read, 4 edit, 1 write) — versus the previous design of one fresh session per task, each re-reading shared context (the −45% two-task fixture result above quantifies that saving).

### 4.4 Self-repair now costs bytes, not re-runs

Fix `8c4f83e` (keep repairs in the same session) is visible in sprint 37's metrics: all four code-context runs reused **one session** (`ses_fcc352d01ffe…`). Repair turns were sent as **936-byte and 1,937-byte prompts** instead of resending the 32.6 KB original request — validation repairs are now an order of magnitude cheaper and preserve the agent's accumulated repo knowledge.

## 5. Problems seen in the recent logs (and status)

- **Prompt bloat → runtime exits (regression, Aug 24).** After `7fab321` removed sprint prompt truncation limits, sprint-36 review prompts jumped from ~235 KB to **~2.0 MB** (8.5×). Of 20 such review attempts 14:50–15:21, several failed with `runtime_exit` (17 review `runtime_exit` failures recorded in total); retries eventually completed but at ~4.4 min median — slower and less reliable than the 235 KB cohort (2.3 min median). The likely duplication is not the 16.6 KB `.run-state.json`: `renderReviewerPrompt` both points reviewers at a frozen review filesystem and directly embeds every common governed input plus every changed target file. The per-sprint isolated worktree already supplies a complete implementation tree, and the frozen review snapshot already supplies stable exact paths.
- **Area-reasoning `runtime_exit` ×3** in sprint 37 (Aug 24 14:36–20:15) — addressed by `2da8d09` (recover completed area reasoning after runtime exit); the stage shows complete in flow-state.
- One smoke timeout in sprint 35 (12-min limit, then 1.5-min success on retry) — harness flakiness, not context-related.

## 6. Verdict

**Yes, with caveats.** The mechanism demonstrably works as designed:

1. Exploration moved upstream and happens once (code-context: 45–144 calls), while downstream reviewers operate from frozen packets with near-zero search (median ~11 reads, some zero-tool completions, 2.3-min median reviews).
2. The stable 64 KB shared prefix produces large real provider cache reads (~3.3× the billed fresh input across sprint-35 reviewers).
3. Deterministic A/B shows −45% outbound bytes for multi-task execution and −65% composition cost.
4. Repairs are cheap session continuations (≤2 KB) instead of full re-prompts.

What cannot be claimed: a precise live before/after comparison of downstream wall-clock or tool counts for sprints ≤34 — that telemetry predates both the metrics capture and the current OpenCode store, so the baseline rests on the deterministic fixture plus the audit's structural findings.

Biggest open risk: review prompts duplicate the complete frozen filesystem inputs inline, producing ~2 MB requests that actively hurt the same review fan-out the stage was meant to accelerate.

## 7. Recommendations

1. Keep governed planning inputs complete, but stop embedding implementation files and raw `.run-state.json` in reviewer prompts. Let reviewers read the complete sprint-isolated worktree/frozen review snapshot from the exact paths already listed. The orchestrator can continue loading run state itself to validate execution and derive the changed-path set.
2. Translate the stable-prefix metadata into native provider cache breakpoints in the agentwrap adapter to make cache hits guaranteed rather than opportunistic.
3. Persist tool-call counts per run in `.runtime-metrics.json` (currently only in the OpenCode DB) so future A/Bs don't depend on DB retention.
