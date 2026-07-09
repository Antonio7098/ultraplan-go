package tui

import (
	"fmt"
	"strings"
)

func Render(m Model, width int) string {
	return RenderWithSize(m, width, 40)
}

func RenderWithSize(m Model, width, height int) string {
	var header strings.Builder
	fmt.Fprintf(&header, "UltraPlan TUI - read-only dashboard\n")
	fmt.Fprintf(&header, "%s\n", renderTabs(m))
	fmt.Fprintf(&header, "%s\n", m.breadcrumb())
	if m.Loading {
		fmt.Fprintf(&header, "Loading workspace status...\n")
	}
	if m.Error != "" {
		fmt.Fprintf(&header, "Error: %s\n", m.Error)
	}
	var body strings.Builder
	selectedStart, selectedEnd := -1, -1
	if m.Preview != nil {
		renderPreview(&body, m, width)
	} else {
		selectedStart, selectedEnd = renderNavItems(&body, m)
	}
	mode := viewportSelection
	offset := 0
	if m.Preview != nil {
		mode = viewportOffset
		offset = m.PreviewOffset
	}
	return renderFrame(header.String(), body.String(), HelpText(), selectedStart, selectedEnd, offset, mode, height)
}

func renderTabs(m Model) string {
	project := " Projects "
	study := " Studies "
	if m.ActiveTab == TabProjects {
		project = "[" + strings.TrimSpace(project) + "]"
	}
	if m.ActiveTab == TabStudies {
		study = "[" + strings.TrimSpace(study) + "]"
	}
	if m.Focus == FocusTabs {
		return "Tabs: > " + project + " " + study
	}
	return "Tabs:   " + project + " " + study
}

func renderNavItems(b *strings.Builder, m Model) (int, int) {
	items := m.navItems()
	if len(items) == 0 {
		fmt.Fprintln(b, "(none)")
		return -1, -1
	}
	selectedStart, selectedEnd := -1, -1
	for i, item := range items {
		start := lineCount(b.String())
		marker := " "
		if m.Focus == FocusContent && i == m.Selected {
			marker = ">"
			selectedStart = start
		}
		fmt.Fprintf(b, "%s %s", marker, item.Label)
		if item.Route != nil {
			fmt.Fprintf(b, " >")
		}
		fmt.Fprintln(b)
		renderItemSummary(b, m, item)
		if m.Focus == FocusContent && i == m.Selected {
			selectedEnd = lineCount(b.String()) - 1
		}
	}
	return selectedStart, selectedEnd
}

func renderItemSummary(b *strings.Builder, m Model, item navItem) {
	route := m.currentRoute()
	switch route.Kind {
	case RouteProjects:
		if p, ok := findProject(m.Data.Projects, item.Label); ok {
			fmt.Fprintf(b, "    docs=%s roadmap=%s index=%s catalog=%s findings=%d\n", p.DocsDir, p.Roadmap, p.ProjectIndex, p.Catalog, len(p.Findings))
		}
	case RouteProjectSprints:
		if s, ok := findSprint(m.Data.Sprints, route.Project, item.Label); ok {
			fmt.Fprintf(b, "    status=%s findings=%d execute=%s\n", s.Status, len(s.Findings), s.Execute.Message)
		}
	case RouteStudies:
		if s, ok := findStudy(m.Data.Studies, item.Label); ok {
			fmt.Fprintf(b, "    sources=%d dimensions=%d status=%s failed=%d\n", len(s.Sources), len(s.Dimensions), s.Status, s.Failed)
		}
	}
}

func renderPreview(b *strings.Builder, m Model, width int) {
	preview := m.Preview
	if preview == nil {
		return
	}
	title := m.PreviewTitle
	if title == "" {
		title = "Preview"
	}
	fmt.Fprintf(b, "%s\n", title)
	fmt.Fprintf(b, "Kind: %s\n", preview.Kind)
	if preview.Missing {
		fmt.Fprintf(b, "Missing: %s\n", preview.Error)
		return
	}
	if preview.Error != "" {
		fmt.Fprintf(b, "Preview error: %s\n", preview.Error)
	}
	if preview.Invalid {
		fmt.Fprintln(b, "Format: invalid")
	}
	if preview.Truncated {
		fmt.Fprintln(b, "Truncated: true")
	}
	if preview.Content != "" {
		fmt.Fprintln(b)
		content := preview.Content
		if preview.Kind == "markdown" {
			content = renderMarkdownContent(preview.Content, width)
		}
		fmt.Fprintln(b, content)
	}
}

type viewportMode int

const (
	viewportSelection viewportMode = iota
	viewportOffset
)

func renderFrame(header, body, help string, selectedStart, selectedEnd, offset int, mode viewportMode, height int) string {
	headerLines := splitLines(header)
	bodyLines := splitLines(body)
	footerLines := []string{help}
	if height <= 0 {
		height = 40
	}
	bodyHeight := height - len(headerLines) - len(footerLines) - 1
	if bodyHeight < 1 {
		bodyHeight = 1
	}
	vp := newViewport(len(bodyLines), bodyHeight)
	switch mode {
	case viewportOffset:
		vp = vp.AtOffset(offset)
	default:
		vp = vp.FollowSelection(selectedStart, selectedEnd)
	}
	var out strings.Builder
	for _, line := range headerLines {
		fmt.Fprintln(&out, line)
	}
	for _, line := range bodyLines[vp.offset:vp.End()] {
		fmt.Fprintln(&out, line)
	}
	if len(bodyLines) > bodyHeight {
		fmt.Fprintf(&out, "scroll %d/%d\n", vp.offset+1, vp.MaxOffset()+1)
	} else {
		fmt.Fprintln(&out)
	}
	for _, line := range footerLines {
		fmt.Fprintln(&out, line)
	}
	return out.String()
}

func splitLines(value string) []string {
	value = strings.TrimRight(value, "\n")
	if value == "" {
		return nil
	}
	return strings.Split(value, "\n")
}

func lineCount(value string) int {
	return len(splitLines(value))
}
