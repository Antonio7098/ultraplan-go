// Package sprint owns UltraPlan planning sprint artifacts and flow state.
//
// A planning sprint is a directory under projects/<project>/sprints/<slug>.
// This package models only the governed planning chain through plan.md:
// requirements.md, sprint-index.md, technical-handbook.md, reasoning/*.md,
// reasoning.md, plan.md, and flow-state.json.
//
// Sprint status is intentionally runtime-free. It discovers sprint roots,
// validates safe artifact paths, strictly loads flow-state.json, derives stage
// status from local artifact presence, and writes refreshed flow state
// atomically. It does not generate prompts, invoke agentwrap or OpenCode, run
// shell commands, mutate Git, execute implementation work, perform smoke
// investigation, automate review, or manage issue trackers.
//
// The app package may parse CLI arguments and render summaries, but stage
// order, artifact paths, persisted schema validation, and status derivation are
// sprint-owned behavior.
package sprint
