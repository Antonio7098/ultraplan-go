package tui

import (
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/ansi"
	"github.com/charmbracelet/glamour/styles"
)

func renderMarkdownContent(content string, width int) string {
	if width < 20 {
		width = 20
	}
	style := styles.DarkStyleConfig
	clearHeadingPrefixes(&style)
	renderer, err := glamour.NewTermRenderer(
		glamour.WithStyles(style),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return content
	}
	rendered, err := renderer.Render(content)
	if err != nil {
		return content
	}
	if rendered == "" {
		return content
	}
	return rendered
}

func clearHeadingPrefixes(style *ansi.StyleConfig) {
	style.H1.Prefix = ""
	style.H2.Prefix = ""
	style.H3.Prefix = ""
	style.H4.Prefix = ""
	style.H5.Prefix = ""
	style.H6.Prefix = ""
}
