package app

import (
	"strings"
	"testing"
)

func TestRenderSafeMarkdownAllowsStructureAndRejectsActiveContent(t *testing.T) {
	got, err := RenderSafeMarkdown("# Heading\n\n<script>alert(1)</script>\n\n[bad](javascript:alert(1))\n\n| A | B |\n| - | - |\n| x | y |\n")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"<h1>Heading</h1>", "<table>"} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered markdown missing %q: %s", want, got)
		}
	}
	for _, forbidden := range []string{"<script", "javascript:"} {
		if strings.Contains(strings.ToLower(got), forbidden) {
			t.Errorf("rendered markdown contains %q: %s", forbidden, got)
		}
	}
}
