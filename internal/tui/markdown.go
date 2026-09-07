package tui

import (
	"fmt"
	"strings"
	"sync"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/ansi"
	"github.com/charmbracelet/glamour/styles"
)

// glamour is the expensive bit of preview scrolling: it parses the whole
// document, lays it out, and emits ANSI every call. Scrolling inside a
// preview was tearing up the framerate. Cache the rendered+split output
// keyed by (width, content); keys are content strings, which is fine for
// the artifact previews this TUI opens. A bounded LRU (mdCacheSize) keeps
// memory pinned regardless of session length.
const mdCacheSize = 16

var (
	mdCacheMu  sync.Mutex
	mdCache    = make(map[string][]string, mdCacheSize)
	mdCacheOrd = make([]string, 0, mdCacheSize)
)

func cachedMarkdownLines(content string, width int) []string {
	if width < 20 {
		width = 20
	}
	key := fmt.Sprintf("%d\x00%s", width, content)
	mdCacheMu.Lock()
	defer mdCacheMu.Unlock()
	if lines, ok := mdCache[key]; ok {
		touchOrdered(key)
		return lines
	}
	lines := glamourLines(content, width)
	for len(mdCache) >= mdCacheSize {
		old := mdCacheOrd[0]
		mdCacheOrd = mdCacheOrd[1:]
		delete(mdCache, old)
	}
	mdCache[key] = lines
	mdCacheOrd = append(mdCacheOrd, key)
	return lines
}

func touchOrdered(key string) {
	for i, k := range mdCacheOrd {
		if k == key {
			mdCacheOrd = append(mdCacheOrd[:i], mdCacheOrd[i+1:]...)
			break
		}
	}
	mdCacheOrd = append(mdCacheOrd, key)
}

func glamourLines(content string, width int) []string {
	rendered, err := renderMarkdownContent(content, width)
	if err != nil || rendered == "" {
		rendered = content
	}
	trimmed := strings.TrimRight(rendered, "\n")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "\n")
}

// renderMarkdownContent is the underlying glamour pass; it's expensive
// (10-500ms for long docs) which is why callers should go through
// cachedMarkdownLines whenever scrolling matters.
func renderMarkdownContent(content string, width int) (string, error) {
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
		return "", err
	}
	return renderer.Render(content)
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
