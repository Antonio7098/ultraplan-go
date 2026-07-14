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
	applyMarkdownTheme(&style)
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

func applyMarkdownTheme(style *ansi.StyleConfig) {
	text, muted := string(palette.text), string(palette.muted)
	blue, amber := string(palette.blue), string(palette.amber)
	green, orange := string(palette.green), string(palette.orange)
	raised := string(palette.raised)
	style.Document.Color = &text
	style.Text.Color = &text
	style.Paragraph.Color = &text
	style.H1.Color = &blue
	style.H2.Color, style.H3.Color = &amber, &amber
	style.H4.Color, style.H5.Color, style.H6.Color = &amber, &amber, &amber
	style.Item.Color, style.Enumeration.Color = &green, &green
	style.Link.Color, style.LinkText.Color = &blue, &blue
	style.BlockQuote.Color = &orange
	style.Code.Color, style.Code.BackgroundColor = &green, &raised
	style.CodeBlock.BackgroundColor = &raised
	style.HorizontalRule.Color = &muted
}

func clearHeadingPrefixes(style *ansi.StyleConfig) {
	style.H1.Prefix = ""
	style.H2.Prefix = ""
	style.H3.Prefix = ""
	style.H4.Prefix = ""
	style.H5.Prefix = ""
	style.H6.Prefix = ""
}
