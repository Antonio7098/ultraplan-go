package app

import (
	"bytes"
	"fmt"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
)

// RenderSafeMarkdown converts bounded, app-owned Markdown artifact content to
// safe HTML. Goldmark's default renderer omits raw HTML and filters unsafe link
// destinations; the web adapter remains responsible for marking this reviewed
// projection as trusted template content.
func RenderSafeMarkdown(source string) (string, error) {
	var out bytes.Buffer
	markdown := goldmark.New(goldmark.WithExtensions(extension.GFM))
	if err := markdown.Convert([]byte(source), &out); err != nil {
		return "", fmt.Errorf("render safe markdown: %w", err)
	}
	return out.String(), nil
}
