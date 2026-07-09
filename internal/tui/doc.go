// Package tui owns UltraPlan's local terminal dashboard.
//
// Sprint 24 keeps this package read-only and dependency-contained. The package
// uses Bubble Tea for the terminal event loop and keeps its model/update/render
// state testable without a real terminal. Terminal program setup, terminal
// library types, navigation, key handling, preview state, rendering, and
// UI-local error panes belong here and must not leak into internal/app or
// product packages.
//
// The dashboard consumes typed read-only app use cases. It does not call CLI
// command handlers, parse stdout/stderr, invoke ultraplan as a subprocess, run
// validation, run sprint flow, render prompt previews, execute plans, run study
// loops, launch runtimes, mutate Git, launch plugins, generate smoke/review/
// issue artifacts, or persist TUI-specific state.
package tui
