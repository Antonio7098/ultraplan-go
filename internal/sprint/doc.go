// Package sprint owns UltraPlan planning, execute, review, QA, merge, and
// standalone smoke artifacts
// and flow state.
//
// A planning sprint is a directory under projects/<project>/sprints/<slug>.
// This package models the governed chain through QA and merge:
// requirements.md, sprint-index.md, technical-handbook.md, reasoning/*.md,
// reasoning.md, plan.md, execute.md, review.md, qa.md, merge.md,
// .run-state.json, and flow-state.json. smoke.md belongs to the separate
// external-harness verification path.
//
// Sprint status is intentionally runtime-free. It discovers sprint roots,
// validates safe artifact paths, strictly loads flow-state.json, derives stage
// status from local artifact presence, and writes refreshed flow state
// atomically. Execute, review, and QA use controlled runtime boundaries;
// standalone smoke uses the generic direct-process boundary. Merge owns its
// bounded Git mutations. The package does not invoke shell command strings or
// manage issue trackers.
//
// The app package may parse CLI arguments and render summaries, but stage
// order, artifact paths, persisted schema validation, and status derivation are
// sprint-owned behavior.
package sprint
