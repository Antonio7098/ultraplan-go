// Package tui owns UltraPlan's local terminal dashboard.
//
// The package is dependency-contained and delegates operational work through
// typed application use cases. It
// uses Bubble Tea for the terminal event loop and keeps its model/update/render
// state testable without a real terminal. Terminal program setup, terminal
// library types, navigation, key handling, preview state, rendering, and
// UI-local error panes belong here and must not leak into internal/app or
// product packages.
//
// The dashboard supports validation, bounded prompt previews and dry runs, and
// explicitly confirmed sprint flow, execute, and resumable study run-loop operations.
// Runtime operations are single-owner and cancellable; product services retain
// durable-state ownership. The package does not call CLI handlers, parse their
// output, invoke ultraplan as a subprocess, mutate Git, launch plugins, create
// smoke/review/issue artifacts, or persist TUI-specific state.
package tui
