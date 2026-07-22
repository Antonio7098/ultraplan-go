// Package sprint owns UltraPlan planning, execute, review, and smoke artifacts
// and flow state.
//
// A planning sprint is a directory under projects/<project>/sprints/<slug>.
// This package models the governed chain through smoke:
// requirements.md, sprint-index.md, technical-handbook.md, reasoning/*.md,
// reasoning.md, plan.md, execute.md, review.md, smoke.md, .run-state.json, and
// flow-state.json.
//
// Sprint status is intentionally runtime-free. It discovers sprint roots,
// validates safe artifact paths, strictly loads flow-state.json, derives stage
// status from local artifact presence, and writes refreshed flow state
// atomically. Execute/review use the generic platform runtime boundary; smoke
// uses the generic direct-process boundary. It does not invoke a shell command
// string, mutate Git, or manage issue trackers.
//
// The app package may parse CLI arguments and render summaries, but stage
// order, artifact paths, persisted schema validation, and status derivation are
// sprint-owned behavior.
package sprint
