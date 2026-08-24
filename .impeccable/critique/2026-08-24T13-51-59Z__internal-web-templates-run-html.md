---
target: agent event streams in sprint and study flows
total_score: 16
max_score: 40
na_heuristics: 
p0_count: 0
p1_count: 4
timestamp: 2026-08-24T13-51-59Z
slug: internal-web-templates-run-html
---
Method: dual-agent (A: /root/event_stream_design_review · B: /root/event_stream_detector)

## Design health score

| # | Heuristic | Score | Key issue |
|---|---|---:|---|
| 1 | Visibility of system status | 2 | Durable state is visible, but current phase, elapsed time, attempt position, and remaining work must be inferred. |
| 2 | Match between system and real world | 2 | "Retained events," "replay boundary," `complete=false`, and raw IDs expose implementation language. |
| 3 | User control and freedom | 2 | Users can cancel and inspect, but cannot filter, search, isolate attempts, or act from a failed state. |
| 4 | Consistency and standards | 2 | Sprint stages communicate progression clearly; run detail abandons that model for a raw log. |
| 5 | Error prevention | 3 | Cancellation is confirmed, tool details are contained, and unsafe fields are omitted. |
| 6 | Recognition rather than recall | 1 | Users must mentally join process starts, failures, retries, validation, and the terminal event. |
| 7 | Flexibility and efficiency | 1 | The API helps experts, but the UI has no event search, ownership filter, attempt navigation, or complete tab keyboard behavior. |
| 8 | Aesthetic and minimalist design | 1 | Raw chronology dominates; long retry tables bury current operating state. |
| 9 | Error recovery | 1 | Failures name what happened but offer no composed stopping point, retry context, or next action. |
| 10 | Help and documentation | 1 | Lifecycle, liveness, omission, replay, wrapper runs, and child runs are not explained in context. |
| **Total** |  | **16/40** | **Poor. The durable foundation is strong, but the interface makes operators reconstruct the run.** |

## Design-specificity verdict

The sprint stage strip and live agent slots feel authored for UltraPlan. They express governed work, sequence, capacity, and completed artifacts. The run-detail page loses that identity. A raw run ID becomes the title, "Retained events" becomes the main object, and persistence records all receive similar weight. Another orchestration tool could use that page unchanged.

The representative failed run shows the cost. In 17 seconds, UltraPlan started the runtime twice, saw two runtime failures, retried once, and ended in a terminal failure. The page renders eight similar rows and repeats safe-observability omission notices. The operator has to assemble the story manually.

The deterministic scan returned zero findings in `internal/web/templates/run.html`, but this is not a reliable clean result. The detector reported that its HTML and CSS parser modules were unavailable and used a regex fallback. It could not evaluate selector matching, custom properties, or computed contrast. Browser mutation worked, but the visual overlay could not load because UltraPlan's `script-src 'self'` content-security policy blocked `http://localhost:8400/detect.js` on the port-9090 app. No user-visible overlay exists.

## Overall impression

UltraPlan records enough truth to explain a run well, but currently displays the database journal instead of the run's narrative. The biggest opportunity is one shared "run journey" grammar used for sprint stages, durable runs, study phases, and agent attempts. UltraPlan-owned transitions should form the visible spine. Runtime turns and tool calls should remain supporting evidence.

## What works

- Durable truth is explicit. Lifecycle, liveness, cancellation, replay bounds, event omissions, and the live connection state are inspectable.
- Raw evidence stays available. Tool observations use native disclosure, and the Event API gives operators a machine-readable route.
- The existing sprint stage strip and live agent slots are strong starting components. They already communicate sequence, status, active work, idle capacity, and memory throttling.

## Priority issues

### P1. UltraPlan lifecycle has no visible narrative

**Why it matters.** Storage events carry more visual weight than governed progression. Users cannot answer "where is it now?" or "what completed?" at a glance.

**Fix.** Put a stable lifecycle journey above the evidence. Use four phases: Queued, Running, Checking, Finished. Each phase supports pending, active, complete, waiting, skipped, failed, cancelled, and degraded states. A phase should be a button only when selecting it filters the evidence below.

**Suggested command.** `$impeccable shape`

### P1. Attempts and retries are flattened into one feed

**Why it matters.** Two process starts and two failures look like repeated noise instead of Attempt 1, a wait, then Attempt 2. Operators cannot judge retry behavior or identify the final stopping point.

**Fix.** Group runtime events by attempt. Put retry or backoff on the connector between attempt cards. Show duration, session reuse, tool count, and result on each attempt. Expand the failed attempt by default.

**Suggested command.** `$impeccable distill`

### P1. Study progress buries the operating picture

**Why it matters.** The inspected study shows 253 completed, 640 pending, no active tasks, 3 failed, 2 cancelled, and 409 retries across 142 tasks. The full retry table pushes parallelism and resource state far down the page.

**Fix.** Lead with a plain status such as "Idle, unfinished," a segmented task summary, attention count, and next operation. Put live agent slots directly below when active. Collapse retry detail behind a searchable summary.

**Suggested command.** `$impeccable layout`

### P1. Status semantics are weak and sometimes contradictory

**Why it matters.** Failed lifecycle, terminal liveness, accepted product status, and no cancellation request appear together without ownership or explanation. The state pills share the same basic treatment, so scanning is slow.

**Fix.** Show one authoritative run outcome in the hero. Move storage and correlation state under "Run details." Map every state to text, icon, color, and accessible name. Never rely on glow or color alone.

**Suggested command.** `$impeccable clarify`

### P2. Runtime evidence lacks ownership and priority

**Why it matters.** UltraPlan transitions, runtime lifecycle, permission setup, turns, tools, validation, repair, retries, and terminal events all read as peers.

**Fix.** Label ownership as UltraPlan, Runtime, and Agent activity. Make UltraPlan events the default journey. In the full event view, group by attempt and turn, expand lifecycle and error rows, and collapse routine turns into summaries such as "Turn 4, 3 tool calls, 18s."

**Suggested command.** `$impeccable distill`

## Recommended interaction model

### 1. Start with an operating summary

Replace the run-ID headline with a human title:

> Area reasoning, Sprint 37  
> Failed after 17s, 2 attempts

Keep the run ID below it with a copy action. Show one outcome badge and one primary action. Put liveness, cancellation, product correlation, replay, retention, and opaque attempt IDs in a collapsed details section.

### 2. Use a four-phase journey

| Actual signal | Journey treatment |
|---|---|
| Snapshot accepted or queued | Activate or complete Queued |
| Envelope lifecycle with `payload.lifecycle=running` | Complete Queued and activate Running |
| Runtime `lifecycle.transition/process_started` | Start an attempt inside Running |
| `step_start`, `step_finish`, permission, session, and tools | Roll into turn and activity counts |
| `run_finished` or `run_failed` | Finish the current attempt |
| `validation.started` and `validation.completed` | Activate or complete Checking |
| `repair.started` and `repair.completed` | Show a conditional loop inside Checking |
| Agent stage `waiting` or a retry event | Add a waiting connector between attempts |
| Envelope terminal | Set Finished to succeeded, failed, cancelled, interrupted, or degraded |

For the inspected failure, the default narrative becomes:

> Queued, complete. Running, failed. Checking, skipped. Finished, failed.  
> Attempt 1 started and failed after 5s. Retried after 3s. Attempt 2 started and failed after 7s. OpenCode exited before a successful final result.

That is the same durable data, but an operator can read it in seconds.

### 3. Keep only three peer views

- Journey is the default and explains the run.
- Agents shows live slots, planned agents, settled agents, and per-agent attempts.
- Events retains the complete chronological record for debugging.

The Events view gets one filter menu with UltraPlan events, runtime lifecycle, agent activity, errors only, and tool calls. It should support search, attempt anchors, timestamps, and a copy-diagnostics action.

### 4. Compose the ending

On failure, pin a "Where it stopped" card above the evidence. Show the phase, attempt, plain-language reason, retry count, session reuse, and last successful milestone. Offer Open failed attempt and Copy diagnostics. Add Retry run only where the product contract supports it.

On success, the final phase should confirm the artifact or report produced, duration, attempts, and the next useful destination. Do not make success another buried terminal row.

### 5. Apply the same grammar to study flow

Group the study journey into Analysis, Synthesis, and conditional Validation. Use a segmented progress bar with visible text counts for complete, failed, cancelled, and pending. When running, place the existing slot grid directly under the journey. When idle, state why and offer the next operation.

Reduce the retry block to a summary such as "142 tasks retried, 409 retries, 141 fresh sessions." Put the task table in an expandable, searchable detail. Link the study run ID to its durable run.

### 6. Fix the event contract before styling sprint progress

The study stream already preserves useful milestones. Sprint flow does not. `FlowProgress` produces checking, skipped, running, failed, and complete messages, but the durable operation recorder stores the outer operation as generically running and records the useful message as an omission. A truthful sprint journey needs structured progress fields such as owner, phase, state, attempt ordinal, and safe summary. Do not infer completed steps from missing events.

## Cognitive load and emotional journey

Six of eight cognitive-load checks fail: single focus, chunking, grouping, hierarchy, working memory, and progressive disclosure. The page can show up to 500 events without attempt or phase groups. Nearby controls also exceed four choices: the study navigation has eight destinations, the sprint strip has nine stages, the run lifecycle filter exposes fourteen values, and parallelism exposes eight values.

The opening is reassuring because durable and live state are present. The middle goes flat as permissions, model turns, tools, and omission notices pile up. Failure is the deepest valley: the useful reason appears earlier in the stream and the last row merely says terminal. The proposed journey fixes the middle and the ending. It turns activity into visible advancement, and it puts a composed recovery state where the operator expects closure.

## Persona red flags

### Jordan, first-time user

- A raw run ID is the page title instead of the work being performed.
- "Replay boundary," "liveness," "retained events," and repeated omission notices require internal knowledge.
- The header exposes several status systems without explaining their owners.
- A failed run does not answer where it stopped or what to do next.

### Alex, power user and operator

- The UI cannot search or filter 500 events by attempt, task, owner, type, or failure.
- Server-rendered event timestamps exist in data attributes but are not shown.
- There are no stable attempt or event anchors and no copy-diagnostics action.
- Agent tabs lack arrow-key behavior and roving tab focus expected of a tablist.

### Sam, accessibility-sensitive user

- Status pills do not provide enough visual differentiation for low-vision scanning.
- A "lights up" treatment needs icons, state text, `aria-current="step"`, and a reduced-motion alternative.
- Live regions should announce phase changes and failures, not every tool or turn event.
- Agent list cards look clickable, but only their nested button is keyboard-operable.

## Minor observations

- Summarize safe-observability omissions once at run level instead of repeating a warning on most rows.
- Lifecycle, liveness, and cancellation appear in both the hero pills and metadata.
- Current attempt shows an opaque ID without attempt number, start time, or duration.
- The Event API is the strongest action in the event header, which makes the page feel API-first.
- The study run ID is text rather than a link.
- Group the runs-list lifecycle choices into Active, Succeeded, and Needs attention. Disclose rare states separately.
- `renderAgentGrid` appends the idle-slot loop twice, which can display twice the intended open slots.
- The CSS contains duplicate run-page and run-component definitions, which invites drift.

## Questions to consider

- Should the first implementation prove the shared journey on one sprint run and one study run, or replace every run representation in one pass?
- Should UltraPlan-owned transitions use a distinct branded spine while runtime and tool evidence recedes?
- At what retry count should the UI stop presenting ordinary progress and warn that the run may be looping?
- Which details belong in the default operator view, and which belong only in the evidence view?
