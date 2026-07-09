// Package sprint owns UltraPlan planning sprint artifacts, execute artifacts,
// and flow state.
//
// A planning sprint is a directory under projects/<project>/sprints/<slug>.
// This package models the governed planning chain through execute:
// requirements.md, sprint-index.md, technical-handbook.md, reasoning/*.md,
// reasoning.md, plan.md, execute.md, .run-state.json, and flow-state.json.
//
// Sprint status is intentionally runtime-free. It discovers sprint roots,
// validates safe artifact paths, strictly loads flow-state.json, derives stage
// status from local artifact presence, and writes refreshed flow state
// atomically. Execute behavior uses only the generic platform runtime boundary.
// It does not shell out to runtime tools directly, mutate Git, perform smoke
// investigation, automate review, or manage issue trackers.
//
// The app package may parse CLI arguments and render summaries, but stage
// order, artifact paths, persisted schema validation, and status derivation are
// sprint-owned behavior.
package sprint
