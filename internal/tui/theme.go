package tui

// Direction contract (herdr grammar, adapted to UltraPlan content):
// THESIS: an operator console in herdr's language — tab bar, nav sidebar,
// bordered detail pane, mode bar — replacing the flat full-width rows.
// OWN-WORLD: herdr Tokyo Night: panel #1A1B26, accent #7AA2F7, single-line
// pane chrome with in-border titles, dim unfocused tabs, status dots.
// STORY: operators see where they are (tabs + breadcrumb), what they can
// touch (sidebar), and what it means (detail pane), with keys always visible.
// FIRST VIEWPORT: tab bar / breadcrumb+status / sidebar|detail / mode bar.
// FORM: pinned by request — herdr layout adapted, no concept roll.
// FINISH: unreviewed and undocumented is unfinished; this build ends with
// the finish review, the verdict, DESIGN.md, and every shipping raster
// carrying its provenance.

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// palette follows herdr's Tokyo Night theme (see herdr src/app/state.rs
// Palette::tokyo_night). Legacy field names are kept so markdown.go and older
// styles keep compiling; herdr roles are added alongside.
var palette = struct {
	base, surface, raised, selected lipgloss.Color
	text, muted, blue, amber        lipgloss.Color
	green, orange, red              lipgloss.Color
	panel, activeRow, selectionBg   lipgloss.Color
	surface0, surface1, surfaceDim  lipgloss.Color
	overlay0, overlay1, subtext     lipgloss.Color
	mauve, yellow, teal, peach      lipgloss.Color
	contrast                        lipgloss.Color
}{
	base: "#1A1B26", surface: "#181825", raised: "#1A1B26", selected: "#2D3650",
	text: "#C0CAF5", muted: "#565F89", blue: "#7AA2F7", amber: "#E0AF68",
	green: "#9ECE6A", orange: "#FF9E64", red: "#F7768E",
	panel: "#1A1B26", activeRow: "#232636", selectionBg: "#2D3650",
	surface0: "#24283B", surface1: "#414868", surfaceDim: "#1A1B26",
	overlay0: "#565F89", overlay1: "#696F94", subtext: "#A9B1D6",
	mauve: "#BB9AF7", yellow: "#E0AF68", teal: "#7DCFFF", peach: "#FF9E64",
	contrast: "#1A1B26",
}

var tuiStyles = struct {
	title, tab, activeTab, dimTab, focusedTab lipgloss.Style
	tabBar, breadcrumb, notice, err           lipgloss.Style
	section, selected, body, metadata         lipgloss.Style
	footer, key, scroll                       lipgloss.Style
	sideHeader, sideSelected, sideNum         lipgloss.Style
	sideDivider, separator                    lipgloss.Style
	paneFocused, paneBlurred, paneTitle       lipgloss.Style
	modePill, modePillAlert, modeBase         lipgloss.Style
	dot                                       lipgloss.Style
}{
	title:      lipgloss.NewStyle().Bold(true).Foreground(palette.blue).Background(palette.panel).Padding(0, 1),
	tab:        lipgloss.NewStyle().Foreground(palette.overlay0).Background(palette.surface0).Padding(0, 2),
	activeTab:  lipgloss.NewStyle().Bold(true).Foreground(palette.contrast).Background(palette.blue).Padding(0, 2),
	dimTab:     lipgloss.NewStyle().Foreground(palette.overlay0).Background(palette.surface0).Padding(0, 2),
	focusedTab: lipgloss.NewStyle().Bold(true).Foreground(palette.amber).Background(palette.activeRow).Padding(0, 2),
	tabBar:     lipgloss.NewStyle().Foreground(palette.overlay0).Background(palette.panel),
	breadcrumb: lipgloss.NewStyle().Foreground(palette.subtext).Background(palette.panel).Padding(0, 1),
	notice:     lipgloss.NewStyle().Foreground(palette.amber).Background(palette.panel).Padding(0, 1),
	err:        lipgloss.NewStyle().Bold(true).Foreground(palette.red).Background(palette.panel).Padding(0, 1),
	section:    lipgloss.NewStyle().Bold(true).Foreground(palette.blue).Background(palette.base).PaddingLeft(1),
	selected:   lipgloss.NewStyle().Foreground(palette.text).Background(palette.selectionBg),
	body:       lipgloss.NewStyle().Foreground(palette.text).Background(palette.base),
	metadata:   lipgloss.NewStyle().Foreground(palette.subtext).Background(palette.base),
	footer:     lipgloss.NewStyle().Foreground(palette.overlay0).Background(palette.panel).Padding(0, 1),
	key:        lipgloss.NewStyle().Bold(true).Foreground(palette.blue).Background(palette.panel),
	scroll:     lipgloss.NewStyle().Foreground(palette.peach).Background(palette.panel).PaddingLeft(1),

	sideHeader:   lipgloss.NewStyle().Bold(true).Foreground(palette.overlay0).Background(palette.base),
	sideSelected: lipgloss.NewStyle().Foreground(palette.text).Background(palette.selectionBg),
	sideNum:      lipgloss.NewStyle().Foreground(palette.overlay0).Background(palette.base),
	sideDivider:  lipgloss.NewStyle().Foreground(palette.surface1).Background(palette.base),
	separator:    lipgloss.NewStyle().Foreground(palette.surface1).Background(palette.base),

	paneFocused:   lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(palette.blue),
	paneBlurred:   lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(palette.overlay0),
	paneTitle:     lipgloss.NewStyle().Bold(true).Foreground(palette.blue),
	modePill:      lipgloss.NewStyle().Bold(true).Foreground(palette.contrast).Background(palette.blue).Padding(0, 1),
	modePillAlert: lipgloss.NewStyle().Bold(true).Foreground(palette.contrast).Background(palette.amber).Padding(0, 1),
	modeBase:      lipgloss.NewStyle().Foreground(palette.overlay0).Background(palette.panel),
	dot:           lipgloss.NewStyle().Bold(true),
}

func fullWidth(style lipgloss.Style, value string, width int) string {
	if width < 1 {
		return style.Render(value)
	}
	return style.Width(width).MaxWidth(width).Render(value)
}

// statusDot renders a herdr-style status glyph in the role color for a state.
func statusDot(state string) string {
	switch strings.ToLower(state) {
	case "active", "running", "validating", "retrying", "in_progress", "started", "runtime", "waiting":
		return tuiStyles.dot.Foreground(palette.yellow).Render("●")
	case "completed", "complete", "done", "pass", "passed", "ok", "fresh", "available":
		return tuiStyles.dot.Foreground(palette.green).Render("●")
	case "failed", "failure", "error", "invalid", "blocked", "stale", "interrupted":
		return tuiStyles.dot.Foreground(palette.red).Render("●")
	case "cancelled", "canceled", "pending", "waiting_input", "unknown", "":
		return tuiStyles.dot.Foreground(palette.overlay0).Render("●")
	default:
		return tuiStyles.dot.Foreground(palette.teal).Render("●")
	}
}

type modeSegment struct {
	key   string
	label string
}

// renderModeBar mirrors herdr's bottom mode bar: a mode pill followed by
// key-hint segments (key in accent bold, label dimmed) on the panel color.
func renderModeBar(pill string, alert bool, segments []modeSegment, width int) string {
	pillStyle := tuiStyles.modePill
	if alert {
		pillStyle = tuiStyles.modePillAlert
	}
	var b strings.Builder
	b.WriteString(pillStyle.Render(" " + pill + " "))
	used := lipgloss.Width(" " + pill + " ")
	for _, s := range segments {
		chunk := "  " + s.key + " " + s.label
		if used+lipgloss.Width(chunk) > width {
			break
		}
		used += lipgloss.Width(chunk)
		b.WriteString(tuiStyles.modeBase.Render("  "))
		b.WriteString(tuiStyles.key.Render(s.key))
		b.WriteString(tuiStyles.modeBase.Render(" " + s.label))
	}
	return fullWidth(tuiStyles.modeBase, b.String(), width)
}

func renderHelp(help string, width int) string {
	parts := strings.Split(help, " | ")
	segments := make([]modeSegment, 0, len(parts))
	for _, part := range parts {
		fields := strings.SplitN(part, " ", 2)
		if len(fields) == 2 {
			segments = append(segments, modeSegment{key: fields[0], label: fields[1]})
		} else {
			segments = append(segments, modeSegment{label: part})
		}
	}
	return renderModeBar("OPERATE", false, segments, width)
}

func isSectionLine(line string) bool {
	line = strings.TrimSpace(line)
	return strings.HasPrefix(line, "Study summary") ||
		strings.HasPrefix(line, "Run summary") ||
		strings.HasPrefix(line, "Currently running") ||
		strings.HasPrefix(line, "Previous runs") ||
		strings.HasPrefix(line, "Run-loop parameters") ||
		strings.HasPrefix(line, "CONFIRM OPERATION") ||
		strings.HasPrefix(line, "Operation result:") ||
		strings.HasPrefix(line, "Validation:")
}
