package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// palette is deliberately close to the muted, editor-like colours used by the
// reference UI. Keeping it here makes the presentation easy to tune later.
var palette = struct {
	base, surface, raised, selected lipgloss.Color
	text, muted, blue, amber        lipgloss.Color
	green, orange, red              lipgloss.Color
}{
	base: "#191A24", surface: "#1E202B", raised: "#262936", selected: "#30354B",
	text: "#C5C9E8", muted: "#737A9A", blue: "#82A7FF", amber: "#E5AD61",
	green: "#9DD75F", orange: "#FF9A5C", red: "#E76D78",
}

var tuiStyles = struct {
	title, tab, activeTab, focusedTab lipgloss.Style
	breadcrumb, notice, err           lipgloss.Style
	section, selected, body, metadata lipgloss.Style
	footer, key, scroll               lipgloss.Style
}{
	title:      lipgloss.NewStyle().Bold(true).Foreground(palette.blue).Background(palette.base).Padding(0, 1),
	tab:        lipgloss.NewStyle().Foreground(palette.muted).Background(palette.surface).Padding(0, 1),
	activeTab:  lipgloss.NewStyle().Bold(true).Foreground(palette.base).Background(palette.blue).Padding(0, 1),
	focusedTab: lipgloss.NewStyle().Bold(true).Foreground(palette.amber).Background(palette.raised).Padding(0, 1),
	breadcrumb: lipgloss.NewStyle().Foreground(palette.text).Background(palette.raised).Padding(0, 1),
	notice:     lipgloss.NewStyle().Foreground(palette.amber).Background(palette.surface).Padding(0, 1),
	err:        lipgloss.NewStyle().Bold(true).Foreground(palette.red).Background(palette.surface).Padding(0, 1),
	section:    lipgloss.NewStyle().Bold(true).Foreground(palette.amber).Background(palette.raised).PaddingLeft(1),
	selected:   lipgloss.NewStyle().Foreground(palette.text).Background(palette.selected),
	body:       lipgloss.NewStyle().Foreground(palette.text).Background(palette.base),
	metadata:   lipgloss.NewStyle().Foreground(palette.muted).Background(palette.base),
	footer:     lipgloss.NewStyle().Foreground(palette.muted).Background(palette.raised).Padding(0, 1),
	key:        lipgloss.NewStyle().Bold(true).Foreground(palette.blue),
	scroll:     lipgloss.NewStyle().Foreground(palette.orange).Background(palette.surface).PaddingLeft(1),
}

func fullWidth(style lipgloss.Style, value string, width int) string {
	if width < 1 {
		return style.Render(value)
	}
	return style.Width(width).MaxWidth(width).Render(value)
}

func renderHelp(help string, width int) string {
	parts := strings.Split(help, " | ")
	for i, part := range parts {
		fields := strings.SplitN(part, " ", 2)
		if len(fields) == 2 {
			parts[i] = tuiStyles.key.Render(fields[0]) + " " + fields[1]
		}
	}
	return fullWidth(tuiStyles.footer, strings.Join(parts, "  •  "), width)
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
