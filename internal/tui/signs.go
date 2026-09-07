package tui

import (
	"github.com/charmbracelet/lipgloss"
)

// rowSign names the one-cell state glyph for a sidebar row, like neovim's
// sign column ("E" for error, "W" for warning, "●" for modified).
type rowSign int

const (
	signNone      rowSign = iota
	signDrill             // › — deeper route available
	signPreview           // ¶ — opens a preview pane
	signValidate          // ⚠ — runs validation
	signOperation         // ▶ — mutating/runtime action
	signActive            // ● — running study/run
	signStale             // ◌ — known stale entity
	signFailed            // ✗ — known failing entity
	signCompleted         // ✓ — clean handoff
)

// rowSignGlyph returns the short cell-width glyph for a state. Drill items
// fall through to a blank because the sidebar line already ends in "›".
func (s rowSign) glyph() string {
	switch s {
	case signDrill:
		return " "
	case signPreview:
		return "¶"
	case signValidate:
		return "⚠"
	case signOperation:
		return "▶"
	case signActive:
		return "●"
	case signStale:
		return "◌"
	case signFailed:
		return "✗"
	case signCompleted:
		return "✓"
	default:
		return " "
	}
}

// rowSignColor returns the lipgloss style for a state. Selected rows use
// the inverse contrast so the glyph stays legible on selection bg.
func (s rowSign) style(selected bool) lipgloss.Style {
	base := tuiStyles.sideNum
	switch s {
	case signDrill:
		base = base.Foreground(palette.blue)
	case signPreview:
		base = base.Foreground(palette.teal)
	case signValidate:
		base = base.Foreground(palette.amber)
	case signOperation:
		base = base.Foreground(palette.mauve)
	case signActive:
		base = base.Foreground(palette.yellow)
	case signStale:
		base = base.Foreground(palette.overlay1)
	case signFailed:
		base = base.Foreground(palette.red)
	case signCompleted:
		base = base.Foreground(palette.green)
	}
	if selected {
		return base.Background(palette.selectionBg)
	}
	return base
}

// signForNavItem picks the most relevant state for the current sidebar item.
func signForNavItem(navItems []navItem, route Route, selectedIdx, current int) rowSign {
	if current < 0 || current >= len(navItems) {
		return signNone
	}
	item := navItems[current]
	switch {
	case item.Route != nil:
		return signDrill
	case item.Path != "":
		return signPreview
	case item.EmbeddedReasoning != "":
		return signPreview
	case item.Validation != nil:
		return signValidate
	case item.Operation != nil:
		return signOperation
	case item.ViewRun != "":
		return signActive
	}
	return signNone
}
