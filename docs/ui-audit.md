# UltraPlan Web UI Audit & Redesign Report

Scope: `internal/web` (templates, CSS, JS), benchmarked against two references the team likes:

- **agent-chat-ui demo** (`~/coding/agent-chat-ui/index.html`) — a single-page mock of the ideal "run" experience: run summary panel + live action track.
- **t3code** (`~/coding/t3code`, `apps/web`) — production React/Tailwind agent-harness IDE with sidebar, transcript, right dock, command palette.

---

## 1. Map of UltraPlan's UI surfaces

All pages share one shell (`templates/shell.html`: sticky pill header, `<main>`, footer) and are served by hand-routed handlers (`routes.go:276-370`, `handlers.go:220-336`).

| Surface | Route | Template | What it shows |
|---|---|---|---|
| Dashboard | `/` | `dashboard.html` (15 ln) | Three stacked sections: project cards, sprints table, study cards. No metrics, no "running now". |
| Projects list | `/projects` | `projects.html` (6 ln) | Card grid ("N docs · N findings"). |
| Project | `/projects/{name}` (+ `/sprints`, `/documentation`, …) | `project.html` (63 ln) | Multi-mode template: overview, sprints (hero + milestone timeline), operations, documentation (dual-sidebar doc explorer). |
| Sprint | `/projects/{p}/sprints/{s}` (+ `/run`, `/artifacts`) | `sprint.html` (105 ln) | Richest page: overview hero + progress bar + continue form; Run tab with stage timeline tabs, per-stage panels, reviewer grid, coverage matrix; artifact navigator. |
| Studies / Study | `/studies`, `/studies/{name}` | `studies.html`, `study.html` | Card grid; classic static left sidebar with 6 sections. |
| Runs list | `/runs` | `runs.html` (12 ln) | Filter form + 6-column table (raw IDs, absolute timestamps) + single "Older runs" link. |
| Run detail | `/runs/{id}` | `run.html` (15 ln) | Snapshot dl (lifecycle badge, target as plain text) + raw "Retained events" ordered list fed by SSE. Links point at the JSON API. |
| Artifact | `/artifacts/{ref}` | `artifact.html` (7 ln) | Standalone preview: metadata dl + markdown/code body. No parent context. |
| Operation | `/operations/{id}`, confirm page | `operation.html`, `operation-confirm.html` | State badge, live SSE timeline, cancel; no-JS confirmation renders JSON scope. |

**Asset layer**: real stylesheet is `static/app.css` (427 ln) importing six near-empty module files (`css/*.css`, 27 ln total). Real behavior is `static/app.js` (**984 ln monolith**) plus three tiny modules under `js/` — note there are **two files named `app.js`** with different roles.

## 2. What's wrong (audit findings)

### Navigation & information architecture
1. **No breadcrumbs on project/sprint pages** — the deepest hierarchies have zero breadcrumb; only an auto-collapsing hover sidebar provides context.
2. **Dead breadcrumb terminals** — `run.html:2` ends in literal "Run"; `artifact.html:2` gives no parent (which sprint produced it?).
3. **Run detail doesn't link to its target** — target is plain text (`run.html:6`); you can't jump from a failed run to its sprint.
4. **Three coexisting sidebar paradigms** — static (study), collapsible pinned stack (project/sprint), none (dashboard/lists). Same mental model, three behaviors.
5. **Sidebar "back buttons" are fake navigation** — "← Project"/"← Sprint" swap side panels without changing main content (`app.js:349-359`).
6. **Runs area leaks the API** — "Continue retained event replay" / "Open canonical event resource" link to raw JSON endpoints.
7. Dashboard has no workspace overview: nothing running now, no recent activity, no next-action affordance.

### Consistency
8. **Lifecycle badges uncolored on runs pages** — bare `.status` pill in `runs.html:9`/`run.html:5`; succeeded and failed runs look identical, unlike sprint stages and operation states.
9. **Unstyled filter form** — `.filter-form` (`runs.html:3`) has no CSS rule anywhere.
10. Spelling drift: "Artefacts" vs "Artifacts".
11. Duplicated systems: tokens split between `app.css:8-39` and unused `css/tokens.css`; reduced-motion block twice; fetch wrapper duplicated (`app.js:552` ≈ `operations.js:6`); dead hooks (`data-state`, `data-sidebar-open`).

### Density, hierarchy, states
12. Runs table shows 6 columns of raw IDs and absolute timestamps — unscannable.
13. Primary action buried: "Continue the sprint" is the third panel (`sprint.html:37-42`).
14. Operation confirmation dumps pretty-printed JSON into a `<pre>` as the sole review surface.
15. Terminal SSE event triggers full `window.location.reload()` — loses scroll/expansion state.
16. Tiny type (.62–.72rem badges/metadata) hurts legibility; success token is hex while everything else is OKLCH.

**What's already good** (keep it): SSE replay cursors with gap detection, noscript parity, empty states nearly everywhere, skip links/live regions, roving-tabindex tabs, layer-enforced template primitives.

## 3. What the references get right

### The demo (`agent-chat-ui`) — the run experience
The demo's two panels solve exactly what UltraPlan's run/sprint pages fail at:

1. **Run summary panel** (`index.html:184-254`, `.agent-actions-panel`): one sticky header strip answers *what happened* at a glance — model identity → reasoning effort → elapsed timer → tool-call count → files changed → **"View full diff"** button. Everything after it is detail.
2. **Action track** (`.agent-action-track`): every tool call is a compact tappable chip — icon + verb ("Read File", "Search", "Edit", "Run Tests") + target ("runtime.go") + micro-stat ("52 results · 18ms"). The whole run reads as a horizontal story; the latest action is highlighted, and a chevron expands chips into a grid.
3. **Tool result cards** (`index.html:147-181`): status word ("Exploring") + kind + target + range + tokens + duration in one header line, collapsible code window beneath.
4. **Diff modal** (`index.html:284-365`): full diff behind one button, file headers with green/red stats.

Lessons: **summary-first hierarchy**, **verbs not event types**, **micro-stats on everything**, **diffs one click away**.

### t3code — the app shell
1. **Persistent, resizable left sidebar** = thread/run list always visible (`AppSidebarLayout.tsx`, `Sidebar.tsx`); grouped by project, pinned rows, hover quick-actions, relative timestamps.
2. **Breadcrumb topbar** (`WorkspaceBreadcrumb.tsx`) where the project crumb doubles as a "new thread here" button and the title is inline-renameable.
3. **Right dock with tabs** (`RightPanelTabs.tsx`): diff / files / terminal / preview live beside the conversation instead of separate pages.
4. **Turn-scoped changed-files cards** (`ChangedFilesTree.tsx`): "N changed files" + diffstat label + directory tree, collapsed by default, expandable — never a raw event log.
5. **Folded work logs**: "+N previous tool calls" keeps transcripts short (`MessagesTimeline.tsx`).
6. **Self-ticking timers** mutating text nodes directly — live "Working for Xs" with zero re-renders.
7. **Command palette + keyboard system** (`mod+K`, `mod+D`, `mod+J`, remappable, labels surfaced in tooltips).
8. Semantic shadcn-style token set (`index.css` ~2k lines), light/dark theming, lucide icons, proper `ui/empty` states.

## 4. Recommendations

### P0 — Fix what makes it feel broken (small diffs)
1. Add breadcrumbs to project/sprint pages; make terminal crumbs real names/IDs; link run target → sprint/study page.
2. Color lifecycle/status pills consistently via one mapping helper (`.status-ok/warn/error`) used everywhere.
3. Style the runs filter form; add active-filter chips + clear-all; show match count.
4. Replace raw API links on run detail with in-page pagination ("Load older events") and a "View JSON" secondary action.
5. Humanize timestamps (relative, absolute in tooltip — copy t3code's `timestampFormat.ts` idea); drop raw ID from the runs-table first column into a muted mono cell.
6. Kill the terminal-event `location.reload()`; re-render in place preserving scroll.
7. Pick one spelling ("Artifacts") and delete dead hooks (`data-state`, `data-sidebar-open`) and duplicated CSS/JS wrappers.
8. Promote "Continue the sprint" to the sprint hero.

### P1 — Rebuild the run surface around a summary panel (the demo pattern)
Restructure `run.html` (and the sprint Run tab) as:
- **Summary header strip**: model/agent · stage · elapsed (self-ticking timer) · tool-call count · files/artifacts touched · status pill · primary action ("Open latest artifact" / "Cancel").
- **Activity track below**: translate the SSE event stream into verb-chips (Read/Search/Edit/Test/Build/Diff) with target + micro-duration, latest highlighted, older folded behind "+N earlier events" instead of a 200-item `<ol>` of `sequence · type`.
- **Result card**: on completion, show outcome + changed-artifact list with diffstat-style indicators, expanding to the artifact preview in place.

### P2 — Unify the shell (the t3code pattern)
- One persistent left sidebar across all entity types: projects → sprints → studies → runs tree, grouped, with running-status pulses and relative times. Delete the three-sidebar-paradigm split and the hover-collapse stack.
- Turn the dashboard into a true home: "Running now" section (currently hidden in a header flyout), recent activity feed, per-project progress cards.
- Introduce real design-system primitives: `.btn` variants (primary/secondary/destructive), consistent card/badge/tab components consuming the (currently ignored) spacing tokens in `css/tokens.css`; unify tokens into one file; add light theme support later if wanted.
- Split the 984-line `static/app.js` into feature modules matching `static/js/*` (rename one of the two `app.js` files), and hash-bust assets instead of `max-age=0`.

### Suggested sequencing
P0 items are each <50-line changes to templates/CSS. The P1 summary-panel rewrite touches `run.html`, `sprint.html`, and the SSE render path in `app.js` (~one focused PR). P2 is the structural investment — do it after P1 proves the component vocabulary.
