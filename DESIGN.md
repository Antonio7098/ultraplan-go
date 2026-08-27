---
name: UltraPlan operating dashboards
description: Dark local-first dashboards for operating and reviewing governed entities and durable runs.
colors:
  workspace: "oklch(0.22813 0.020366 307.469)"
  panel: "oklch(0.267101 0.02016 311.799)"
  panel-raised: "oklch(0.279864 0.021572 309.532)"
  panel-overlay: "oklch(0.154761 0.01316 338.901)"
  panel-hover: "oklch(0.364912 0.050794 308.491)"
  control: "oklch(0.313674 0.030572 310.061)"
  input-border: "oklch(0.266817 0.02897 344.461)"
  text: "oklch(0.980735 0.004092 301.426)"
  text-muted: "oklch(0.880303 0.03077 342.696)"
  text-subtle: "oklch(0.657087 0.028226 307.985)"
  accent: "oklch(0.460685 0.185347 4.099)"
  accent-strong: "oklch(0.591646 0.217985 0.584)"
  accent-foreground: "oklch(0.901233 0.057189 343.694)"
  accent-soft: "oklch(0.256077 0.063004 342.914)"
  border: "oklch(0.266943 0.015262 302.425)"
  border-soft: "oklch(0.269132 0.030766 351.067)"
  success: "#56c793"
  warning: "oklch(0.836861 0.164422 84.429)"
  danger: "oklch(0.901233 0.057189 343.694)"
typography:
  display:
    fontFamily: "Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, Segoe UI, sans-serif"
    fontSize: "clamp(3rem, 8vw, 5.5rem)"
    fontWeight: 700
    lineHeight: 0.82
    letterSpacing: "-0.04em"
  headline:
    fontFamily: "Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, Segoe UI, sans-serif"
    fontSize: "clamp(1.65rem, 3vw, 2.15rem)"
    fontWeight: 650
    lineHeight: 1.15
    letterSpacing: "-0.025em"
  title:
    fontFamily: "Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, Segoe UI, sans-serif"
    fontSize: "1.05rem"
    fontWeight: 630
    lineHeight: 1.3
    letterSpacing: "-0.025em"
  body:
    fontFamily: "Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, Segoe UI, sans-serif"
    fontSize: "14px"
    fontWeight: 400
    lineHeight: 1.55
  label:
    fontFamily: "Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, Segoe UI, sans-serif"
    fontSize: "0.7rem"
    fontWeight: 650
    letterSpacing: "0.11em"
rounded:
  matrix-cell: "0.18rem"
  control: "0.5rem"
  panel: "0.65rem"
  pill: "999px"
spacing:
  compact: "0.45rem"
  control: "0.65rem"
  panel: "1.15rem"
  dashboard-gap: "1rem"
components:
  dashboard-panel:
    backgroundColor: "{colors.panel}"
    textColor: "{colors.text}"
    rounded: "{rounded.panel}"
    padding: "{spacing.panel}"
  dashboard-panel-hover:
    backgroundColor: "{colors.panel-raised}"
    textColor: "{colors.text}"
    rounded: "{rounded.panel}"
    padding: "{spacing.panel}"
  button:
    backgroundColor: "{colors.control}"
    textColor: "{colors.text}"
    rounded: "{rounded.control}"
    padding: "0.45rem 0.75rem"
    height: "2.25rem"
  button-link:
    backgroundColor: "{colors.control}"
    textColor: "{colors.text}"
    rounded: "{rounded.control}"
    padding: "0.5rem 0.85rem"
    height: "2.4rem"
  status:
    backgroundColor: "transparent"
    textColor: "{colors.text-muted}"
    rounded: "{rounded.pill}"
    padding: "0.13rem 0.48rem"
  progress-track:
    backgroundColor: "{colors.control}"
    rounded: "{rounded.pill}"
    height: "0.55rem"
---

# Design System: UltraPlan operating dashboards

## Overview

**Creative North Star: "The governed workbench"**

UltraPlan's project, sprint, and study overviews and its run detail pages are dark operating dashboards. They place current truth first, then expose the evidence needed to judge it. The interface is compact and calm. Fine borders and close tonal steps separate information without turning every fact into a card.

Each overview uses one dashboard grammar, but its previews keep the shape of the underlying work. A roadmap reads as a sequence, a sprint reads as staged execution, and a study reads as a research matrix. Actions sit inside the panel whose state they change. Linked panels become the navigation when they contain no competing controls.

Run pages use the same grammar at greater density. The QA command center begins with the canonical snapshot and ordered path, then keeps investigation records, synthesis, outcome, identity, and limits available through semantic disclosures. A historical run never borrows facts from the newer canonical pointer.

**Key Characteristics:**

- One dominant current-state panel per entity overview
- Truthful progress that distinguishes complete, active, failed, stale, and pending work
- Canonical run paths that link stages directly to their retained facts
- Content-shaped evidence previews instead of interchangeable statistic tiles
- Compact attention regions reserved for findings and degraded health
- Full-width stacking below the 48rem breakpoint

## Colors

The palette is a plum-black neutral field with one magenta operational accent and semantic green, amber, and pink state colors.

### Primary

- **Operational Magenta:** Marks active work, focus, linked-panel hover, and the running segment of progress. Its stronger companion carries text cues and current-state dots. The soft form appears only as a restrained wash behind the dominant sprint panel.

### Secondary

- **Completion Green:** Means complete or healthy. Use it for finished progress and successful state marks.
- **Attention Amber:** Means stale, waiting, or in need of inspection.
- **Failure Pink:** Means failed or blocked. It is not a decorative accent.

### Neutral

- **Workspace Plum:** Fills the page and code-like recesses.
- **Workbench Panel:** Holds standard dashboard panels and fact cells.
- **Raised Panel:** Marks hover and selected depth without changing the component's shape.
- **Overlay Plum:** Backs inset facts and the darkest local contrast areas.
- **Readable White:** Carries headings and decisive values.
- **Muted Lilac:** Carries descriptions and secondary labels.
- **Subtle Mauve:** Carries paths, metadata, and low-priority context.
- **Quiet Borders:** Separate panels, rows, and fact cells with narrow tonal changes.

### Named Rules

**The State Color Rule.** Green, amber, pink, and strong magenta communicate workflow state. Do not use them to decorate neutral content.

**The Quiet Border Rule.** Separate ordinary regions with neutral borders. Warning-colored borders belong only on attention and degraded-health panels.

## Typography

**Display Font:** Inter with the system sans-serif stack

**Body Font:** Inter with the system sans-serif stack

**Label/Mono Font:** SFMono-Regular with Consolas and Liberation Mono fallbacks

**Character:** The dashboard uses one compact sans-serif family with modest weight changes. Large numerals provide the only display-scale type. Monospace is reserved for paths, identifiers, and numbered decision markers.

### Hierarchy

- **Display** (700, fluid 3rem to 5.5rem, 0.82): A single completed-work count in the study state panel. Use tabular numerals.
- **Headline** (650, fluid 1.65rem to 2.15rem, 1.15): Entity page titles outside the dashboard grid.
- **Title** (630, 1.05rem, 1.3): Standard panel headings. The dominant sprint status panel may rise to a fluid 1.7rem.
- **Lead** (400, fluid 1.12rem to 1.45rem, 1.45): Project goals and other brief statements, capped near 68 characters.
- **Body** (400, 14px, 1.55): Descriptions, findings, and operational guidance.
- **Label** (650, 0.7rem, 0.11em): Uppercase eyebrows and compact taxonomy labels.

### Named Rules

**The Numeric Truth Rule.** Counts and progress values use tabular numerals. Large type belongs to the state that matters now, not to decorative summary metrics.

## Layout

Entity dashboards sit in a centered content area with a maximum width of 96rem and fluid page padding. Their grid gap is 1rem.

Project overviews use a 1.55 to 0.75 two-column ratio. The project brief spans both columns and the roadmap preview occupies the taller left track. Study overviews use a 1.35 to 0.65 ratio, with the current study state and recent findings spanning both columns. Sprint overviews use near-even columns and let the dominant current-status panel span the full width. Other panels grow to the height of their content.

The QA run command center starts with a 1.35 to 0.65 progress and next-action deck, a six-cell fact strip, and the canonical eight-stage path. Above 64rem, the path uses four columns and two rows. At 64rem and below it uses two columns and four rows. At 48rem and below it becomes one vertical sequence. Each stage remains an anchor to its retained fact region at every width.

The QA evidence area pairs a fluid main column with a 17rem to 20rem identity rail. It becomes one column at 64rem. The rail uses two columns until 48rem, then stacks. At 48rem, the command deck, scope groups, synthesis groups, and condition groups also become single-column layouts. The six-cell fact strip becomes three columns at 48rem and two columns at 32rem.

At 48rem and below, all three dashboard grids become a single column. Spans reset, document stacks and gate pairs collapse, and dense four-part counts become two columns. Preserve reading order in the markup so the current state, evidence, actions, and attention remain coherent without CSS grid placement.

**The Current State Rule.** Give one panel clear visual and spatial priority. It must answer what is happening now and what the next valid action is.

## Elevation & Depth

The system is flat by default. Standard panels use a one-pixel border and a nearly imperceptible top highlight. A whole-panel link lifts by 2px and moves to the raised panel color on hover or keyboard focus. The dominant sprint status panel uses a soft ambient shadow, a low-contrast accent gradient, and a blurred magenta glow. The active QA stage marker uses a smaller magenta glow so the current stage is visible inside the process path.

### Shadow Vocabulary

- **Panel hairline** (`0 1px 0 rgb(255 255 255 / 2%)`): Standard dashboard panels at rest.
- **Dominant state** (`0 16px 42px rgb(0 0 0 / 17%)`): The sprint current-status panel only.
- **Active QA stage** (`0 4px 18px color-mix(in oklab, var(--accent) 28%, transparent)`): The active marker in the canonical QA path only.

### Named Rules

**The Flat-by-Default Rule.** Borders and tonal layering establish ordinary hierarchy. Reserve a real shadow for the single dominant current-state panel.

## Shapes

Dashboard panels use gently rounded 0.65rem corners. Buttons use 0.5rem corners. Progress tracks, state badges, and small counters use full pill shapes. Matrix cells use tight 0.18rem corners so they read as data, while stage and delivery marks are circular.

Borders stay one pixel wide. Inset fact groups and document stacks use one-pixel gaps against a border-colored backing rather than rounding every child cell. This keeps dense evidence visually continuous.

## Components

### Current-state panel

- **Purpose:** The largest or widest panel states the current phase, stage, or completion count and places the valid controls beside it.
- **Project:** A brief with goal, phase, repository, validation state, and a validation action.
- **Sprint:** A full-width staged-progress panel with next action, progress track, and stage sequence.
- **Study:** A full-width completion panel with segmented task state, operational facts, and run or validation actions.

### Linked dashboard panel

- **Shape:** The entire panel is the link when it has no nested controls.
- **State:** Hover and keyboard focus raise the panel by 2px, use the raised neutral, and apply an accent border. Keyboard focus also receives a 2px focus outline with a 3px offset.
- **Cue:** Optional destination copy appears in strong magenta with a short rule after it. Do not add a cue when the content already makes the destination clear.

### Buttons and adjacent controls

- **Shape:** Compact 0.5rem corners with a 2.25rem minimum height.
- **Style:** Neutral control fill, readable white text, inset top highlight, and a small dark shadow.
- **Hover / Focus:** Hover changes the border to magenta and raises the control fill. Focus uses the shared 2px outline. Active buttons move down by 1px.
- **Placement:** Put forms and links inside the current-state panel or evidence region they affect. Wrap them only when the available width requires it.

### Status badges

- **Style:** A transparent pill with a one-pixel border, compact label, and a small circular state mark.
- **State:** Green is complete, strong magenta is active or informational, amber is stale or waiting, pink is failed, and muted lilac is inactive. The state mark may glow except in the muted state.

### Progress and sequence

- **Progress tracks:** Thin rounded tracks on the neutral control color. Use green for completed work and strong magenta for active work.
- **Segmented progress:** Preserve separate complete, active, failed, and pending segments. Never flatten these into a single optimistic percentage.
- **Stage sequence:** Keep ordered stage labels visible. Add stale markers beside the affected stage instead of folding staleness into a generic warning count.

### QA run command center

- **Summary:** Lead with canonical phase, freshness, shard progress, next action, blocker or cancellation, and six retained investigation totals.
- **Canonical path:** Show exactly eight stages in order: Admission, QA map, Investigate, Checks, Evidence, Synthesis, Adjudication, and Terminal. Every stage links directly to its retained fact region.
- **Stage states:** Complete uses a green connector and check mark. Active uses the magenta wash and glow. Pending stays neutral and numbered. Failed uses the failure color and an exclamation mark at the stage where work stopped. Keep the text state available to assistive technology and include a visible legend.
- **Evidence body:** Put investigation shards, attempts, approved checks, theories, synthesis, and the canonical outcome in the main column. Keep identity, ownership, limits, and the evidence boundary in the secondary rail. Use native `details` elements for dense records instead of hiding them behind custom dialogs.
- **Live refresh:** Refresh from the canonical HTML snapshot after committed events. Preserve open disclosures by stable semantic IDs, defer replacement while focus is inside the command center, and leave the last committed snapshot visible when refresh fails.
- **Historical runs:** When the canonical pointer moves to another run, replace the command center facts with a compact historical notice and link to the current run. Keep the event journal below scoped to the historical run.
- **Motion:** The active marker may use the 700ms arrival animation. Disable this animation, all transitions, and smooth scrolling under `prefers-reduced-motion: reduce`.

**The Canonical Pointer Rule.** A run page shows canonical QA facts only when that run owns the current pointer. Never mix a historical event journal with a newer run's QA snapshot.

**The Live Disclosure Rule.** A live refresh may replace data, but it must not close a disclosure the operator opened or move focus out of the command center.

### Evidence previews

- **Roadmap pulse:** A short previous-to-next timeline with delivered and current or planned nodes.
- **Delivery health:** Compact rows with one labeled sprint and separate execute, review, and smoke state dots.
- **Project knowledge:** Three content-width dossier previews with kind, name, summary, and file metadata.
- **Decision record:** A numbered list with monospace ordinal markers and one explicit open-risk line.
- **Research matrix:** Source labels paired with compact state cells for dimension groups.
- **Emerging results:** Report labels, short summaries, and file paths in content-height rows.

**The Evidence Keeps Its Shape Rule.** Choose a preview that matches the underlying artifact or process. Do not replace timelines, matrices, sequences, or document excerpts with generic number cards.

**The Control Adjacency Rule.** An action belongs beside the state or object it changes. A panel with nested controls is not a whole-panel link.

### Attention

Attention is a compact strip with an amber-tinted border. It reports current findings or degraded run health and collapses to content height when empty. Do not give healthy empty states the same visual weight as active problems.

## Do's and Don'ts

### Do:

- **Do** make the dominant panel answer current state, progress, and next action with real workflow data.
- **Do** preserve complete, active, failed, stale, and pending distinctions in progress previews.
- **Do** keep all eight QA stages in canonical order and link each one to retained facts.
- **Do** use content-shaped evidence such as timelines, matrices, stage rails, decision lists, and document excerpts.
- **Do** make a control-free dashboard panel one coherent keyboard-focusable link.
- **Do** keep actions inside the panel whose state they change.
- **Do** preserve open QA disclosures during canonical live refresh.
- **Do** stack panels in a truthful reading order below 48rem.

### Don't:

- **Don't** fill the overview with equal-weight metric cards.
- **Don't** turn semantic state colors into decoration.
- **Don't** hide failure, stale evidence, or waiting work inside a single completion percentage.
- **Don't** let live runtime prose replace the canonical QA snapshot or retained evidence.
- **Don't** show facts from the current canonical run inside a historical run page.
- **Don't** wrap a panel in a link when it contains buttons, forms, or conflicting destinations.
- **Don't** promote empty attention or healthy reliability into a dominant panel.
- **Don't** use decorative shadows on ordinary dashboard panels.
