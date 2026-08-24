# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

## Users

UltraPlan's web UI serves both operators who run project and study workflows locally and reviewers who inspect progress, evidence, and results. The web UI is a complete product interface, not a read-only companion to the CLI.

## Product Purpose

UltraPlan provides durable architecture studies and governed sprint delivery. The web UI must let users inspect, start, monitor, validate, recover, and review the full workflow without falling back to the CLI.

## Positioning

UltraPlan combines research studies and staged sprint delivery in a local-first workspace. It keeps workflow state, generated artifacts, validation findings, and run history tied to the project, sprint, or study that owns them.

## Operating Context

Users move between projects, their roadmaps and sprints, and independent studies. Long-running work appears as durable runs. A user may need to start work, monitor active agents, diagnose failures, inspect artifacts, or review completed evidence in the same session.

## Capabilities and Constraints

- Projects and Studies are primary destinations in the top navigation.
- Active Runs are also a top-level navigation destination.
- Each project, sprint, and study has an overview page that summarizes its nested content and status, and links to focused detail pages.
- Commands belong beside the object and state they affect. The product must not collect unrelated commands on generic Operations pages.
- Selected common actions can run from an entity overview. Contextual pages such as a project roadmap or sprint page may expose the same action when that placement matches the user's task.
- More than one route level is acceptable when it preserves the project, sprint, study, or run hierarchy. Persistent entity sidebars are not required for navigation.
- Existing HTML pages work without JavaScript. JavaScript adds live updates and richer interaction.

## Evidence on Hand

The existing Go templates, handlers, compatibility tests, and `docs/ui-audit.md` document the implemented web workflows. No external marketing claims or user research artifacts are present in this repository.

## Product Principles

- Place every action beside the state or object it changes.
- Make each entity overview answer what is happening, what needs attention, and where to go next.
- Navigate from whole dashboard components and preserve parent context with breadcrumbs instead of persistent sidebars or entity tab rows.
- Keep live work globally discoverable through Runs while also linking each run to its owning entity.
- Support operating and reviewing the whole product from the web UI.

## Accessibility & Inclusion

Preserve keyboard navigation, skip links, live regions, semantic status, reduced-motion support, and complete no-JavaScript flows already present in the web implementation.
