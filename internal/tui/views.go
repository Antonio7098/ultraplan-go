package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/Antonio7098/ultraplan-go/internal/app"
)

func Render(m Model, width int) string {
	return RenderWithSize(m, width, 40)
}

func RenderWithSize(m Model, width, height int) string {
	var header strings.Builder
	fmt.Fprintf(&header, "%s\n", fullWidth(tuiStyles.title, "UltraPlan  ·  operational dashboard", width))
	fmt.Fprintf(&header, "%s\n", renderTabs(m, width))
	fmt.Fprintf(&header, "%s\n", fullWidth(tuiStyles.breadcrumb, m.breadcrumb(), width))
	if m.Loading {
		fmt.Fprintf(&header, "%s\n", fullWidth(tuiStyles.notice, "Loading workspace status...", width))
	}
	if m.Error != "" {
		fmt.Fprintf(&header, "%s\n", fullWidth(tuiStyles.err, "Error: "+m.Error, width))
	}
	if m.Running && m.OperationHidden {
		fmt.Fprintln(&header, fullWidth(tuiStyles.notice, "Run continues in background — c cancel | select View Run for status", width))
	}
	var body strings.Builder
	selectedStart, selectedEnd := -1, -1
	if m.ParallelForm != nil {
		renderParallelForm(&body, m)
	} else if m.RunViewStudy != "" {
		renderRunView(&body, m)
	} else if m.Confirmation != nil {
		renderConfirmation(&body, *m.Confirmation)
	} else if m.Operation != nil && !m.OperationHidden {
		if m.ActiveOperation.Kind == app.OperationStudyStart || m.ActiveOperation.Kind == app.OperationStudyResume {
			renderForegroundRun(&body, *m.Operation, m.Events, m.OperationShowPrevious)
		} else {
			renderOperation(&body, *m.Operation, m.Events)
		}
	} else if m.Validation != nil {
		renderValidation(&body, *m.Validation)
	} else if m.Preview != nil {
		renderPreview(&body, m, width)
	} else {
		renderRouteSummary(&body, m)
		selectedStart, selectedEnd = renderNavItems(&body, m)
	}
	mode := viewportSelection
	offset := 0
	if m.Preview != nil {
		mode = viewportOffset
		offset = m.PreviewOffset
	}
	return renderFrame(header.String(), body.String(), HelpText(), selectedStart, selectedEnd, offset, mode, width, height)
}

func renderParallelForm(b *strings.Builder, m Model) {
	fmt.Fprintln(b, "Run-loop parameters")
	fmt.Fprintf(b, "Study: %s\nParallel workers (1-64): %s\n", m.ParallelForm.Study, m.ParallelValue)
	if m.ParallelValue == "" {
		fmt.Fprintln(b, "Default: 3")
	}
	if m.ParallelError != "" {
		fmt.Fprintf(b, "Error: %s\n", m.ParallelError)
	}
	fmt.Fprintln(b, "Type a number, Enter to review and confirm, Esc to cancel.")
}

func renderForegroundRun(b *strings.Builder, r app.OperationResult, events []app.OperationEvent, showPrevious bool) {
	latest := map[string]app.OperationEvent{}
	order := []string{}
	completed, total := 0, 0
	for _, e := range events {
		if e.Total > 0 {
			completed, total = e.Completed, e.Total
		}
		if e.Task != "" {
			if _, ok := latest[e.Task]; !ok {
				order = append(order, e.Task)
			}
			latest[e.Task] = e
		}
	}
	remaining := total - completed
	if remaining < 0 {
		remaining = 0
	}
	var active, previous []app.OperationEvent
	var tokens int64
	known := 0
	var duration time.Duration
	for _, id := range order {
		e := latest[id]
		if e.TokensKnown {
			tokens += e.Tokens
			known++
		}
		if d, err := time.ParseDuration(e.Duration); err == nil {
			duration += d
		}
		if e.Stage == "started" || e.Stage == "runtime" || e.Stage == "waiting" {
			active = append(active, e)
		} else {
			previous = append(previous, e)
		}
	}
	tokenText := "n/a"
	if known > 0 {
		tokenText = fmt.Sprintf("%d", tokens)
	}
	fmt.Fprintf(b, "Run summary — %s\nStatus: %s\nTotal: %d  Completed: %d  Remaining: %d  Active: %d\nTotal tokens: %s  Total runtime: %s\n\nCurrently running (%d)\n", r.Subject, r.State, total, completed, remaining, len(active), tokenText, duration.Round(time.Second), len(active))
	if len(active) == 0 {
		fmt.Fprintln(b, "(waiting for active task events)")
	}
	for _, e := range active {
		renderOperationTask(b, e)
	}
	if len(previous) > 0 {
		if showPrevious {
			fmt.Fprintf(b, "\nPrevious runs (%d) — Enter: Show Less\n", len(previous))
			for _, e := range previous {
				renderOperationTask(b, e)
			}
		} else {
			fmt.Fprintf(b, "\n> See More (%d previous runs) — press Enter\n", len(previous))
		}
	}
	fmt.Fprintln(b, "\nPress c or q to cancel this run.")
}

func renderOperationTask(b *strings.Builder, e app.OperationEvent) {
	tokens := "n/a"
	if e.TokensKnown {
		tokens = fmt.Sprintf("%d", e.Tokens)
	}
	turns := "n/a"
	if e.TurnsKnown {
		turns = fmt.Sprintf("%d", e.Turns)
	}
	duration := e.Duration
	if duration == "" {
		duration = "n/a"
	}
	provider := e.Provider
	if provider == "" {
		provider = "n/a"
	}
	model := e.Model
	if model == "" {
		model = "n/a"
	}
	cost := e.Cost
	if cost == "" {
		cost = "n/a"
	}
	fmt.Fprintf(b, "- %s [%s] %s\n  workflow_attempts=%d runtime_attempts=%d agent_turns=%s tokens=%s input=%d output=%d reasoning=%d cache_read=%d cache_write=%d time=%s events=%d provider=%s model=%s cost=%s\n", e.Task, e.Stage, e.Message, e.Attempt, e.RuntimeAttempts, turns, tokens, e.InputTokens, e.OutputTokens, e.ReasoningTokens, e.CacheReadTokens, e.CacheWriteTokens, duration, e.RuntimeEvents, provider, model, cost)
}

func renderRunView(b *strings.Builder, m Model) {
	s, ok := findStudy(m.Data.Studies, m.RunViewStudy)
	if !ok {
		fmt.Fprintln(b, "Run status unavailable")
		return
	}
	remaining := s.Total - s.Completed
	if remaining < 0 {
		remaining = 0
	}
	var totalTokens, totalDuration int64
	knownTokens := 0
	for _, task := range s.Tasks {
		if task.TokensKnown {
			totalTokens += task.Tokens
			knownTokens++
		}
		totalDuration += task.DurationMS
	}
	tokenText := "n/a"
	if knownTokens > 0 {
		tokenText = fmt.Sprintf("%d", totalTokens)
		if knownTokens < len(s.Tasks) {
			tokenText += " (known tasks)"
		}
	}
	timeText := (time.Duration(totalDuration) * time.Millisecond).Round(time.Second).String()
	if totalDuration == 0 {
		timeText = "0s"
	}
	fmt.Fprintf(b, "Run summary — %s\nStatus: %s\nTotal: %d  Completed: %d  Remaining: %d  Active: %d\nFailed: %d  Cancelled: %d  Pending: %d\nTotal tokens: %s  Total runtime: %s\n", s.Name, s.RunStatus, s.Total, s.Completed, remaining, s.ActiveTasks, s.Failed, s.Cancelled, s.Pending, tokenText, timeText)
	if s.RunActive {
		fmt.Fprintln(b, "\nPress c to cancel this run.")
	} else {
		fmt.Fprintln(b, "\nRun is no longer active. Press esc to return.")
	}
	var active, previous []app.RunTaskSummary
	for _, task := range s.Tasks {
		if activeRunTask(task.Status) {
			active = append(active, task)
		} else {
			previous = append(previous, task)
		}
	}
	fmt.Fprintf(b, "\nCurrently running (%d)\n", len(active))
	if len(active) == 0 {
		fmt.Fprintln(b, "(none)")
	}
	for _, task := range active {
		renderRunTask(b, task)
	}
	if len(previous) > 0 {
		if m.RunViewShowPrevious {
			fmt.Fprintf(b, "\nPrevious runs (%d) — Enter: Show Less\n", len(previous))
			for _, task := range previous {
				renderRunTask(b, task)
			}
		} else {
			fmt.Fprintf(b, "\n> See More (%d previous runs) — press Enter\n", len(previous))
		}
	}
}

func activeRunTask(status string) bool {
	return status == "running" || status == "validating" || status == "retrying"
}

func renderRunTask(b *strings.Builder, task app.RunTaskSummary) {
	tokens := "n/a"
	if task.TokensKnown {
		tokens = fmt.Sprintf("%d", task.Tokens)
	}
	identity := task.Dimension
	if task.Source != "" {
		identity += " / " + task.Source
	}
	model := task.Model
	if model == "" {
		model = "n/a"
	}
	provider := task.Provider
	if provider == "" {
		provider = "n/a"
	}
	turns := "n/a"
	if task.TurnsKnown {
		turns = fmt.Sprintf("%d", task.Turns)
	}
	fmt.Fprintf(b, "- %s [%s] %s\n", task.ID, task.Status, identity)
	fmt.Fprintf(b, "  workflow_attempts=%d runtime_attempts=%d agent_turns=%s tokens=%s input=%d output=%d reasoning=%d cache_read=%d cache_write=%d time=%s events=%d provider=%s model=%s cost=%s\n", task.Attempts, task.RuntimeAttempts, turns, tokens, task.InputTokens, task.OutputTokens, task.ReasoningTokens, task.CacheReadTokens, task.CacheWriteTokens, task.Duration, task.Events, provider, model, task.Cost)
}

func renderRouteSummary(b *strings.Builder, m Model) {
	route := m.currentRoute()
	if route.Kind != RouteStudy {
		return
	}
	study, ok := findStudy(m.Data.Studies, route.Study)
	if !ok {
		return
	}
	fmt.Fprintln(b, "Study summary")
	fmt.Fprintf(b, "  Dimensions: %d\n", len(study.Dimensions))
	fmt.Fprintf(b, "  Sources: %d\n", len(study.Sources))
	fmt.Fprintf(b, "  Planned runs: %d\n", study.Total)
	fmt.Fprintf(b, "  Done so far: %d\n", study.Completed)
	if study.RunActive {
		status := study.RunStatus
		if status == "" {
			status = "active"
		}
		fmt.Fprintf(b, "  Run status: %s (%d/%d done)\n", status, study.Completed, study.Total)
	}
	if study.Failed > 0 {
		fmt.Fprintf(b, "  Failed: %d\n", study.Failed)
	}
	fmt.Fprintln(b)
}

func renderConfirmation(b *strings.Builder, c app.Confirmation) {
	fmt.Fprintf(b, "CONFIRM OPERATION\nSubject: %s\nWarning: %s\nRuntime: %t  Mutates: %t\n", c.Subject, c.Warning, c.Runtime, c.Mutates)
	for _, s := range c.Scope {
		fmt.Fprintf(b, "Scope: %s\n", s)
	}
	if c.Request.Kind == app.OperationStudyStart || c.Request.Kind == app.OperationStudyResume {
		fmt.Fprintf(b, "Parallel workers: %d\n", c.Request.Parallelism)
	}
	for _, p := range c.Paths {
		fmt.Fprintf(b, "Affected path: %s\n", p)
	}
	fmt.Fprintln(b, "Press Enter to confirm; Esc to cancel without changes.")
}
func renderOperation(b *strings.Builder, r app.OperationResult, events []app.OperationEvent) {
	fmt.Fprintf(b, "Operation result: %s\nSubject: %s\n%s\n", r.State, r.Subject, r.Message)
	if r.Truncated {
		fmt.Fprintln(b, "Truncated: true")
	}
	if r.Content != "" {
		fmt.Fprintln(b, r.Content)
	}
	if r.Error != nil {
		fmt.Fprintf(b, "Error code: %s (%s)\nComponent: %s\nRetryable: %t\nGuidance: %s\n", r.Error.Code, r.Error.Category, r.Error.Component, r.Error.Retryable, r.Error.Guidance)
	}
	for _, e := range events {
		fmt.Fprintf(b, "[%s] %s %s", e.State, e.Stage, e.Message)
		if e.Total > 0 {
			fmt.Fprintf(b, " | %d/%d", e.Completed, e.Total)
		}
		if e.Task != "" {
			fmt.Fprintf(b, " | %s", e.Task)
		}
		fmt.Fprintln(b)
		if e.Task != "" {
			tokens := "n/a"
			if e.TokensKnown {
				tokens = fmt.Sprintf("%d", e.Tokens)
			}
			duration := e.Duration
			if duration == "" {
				duration = "n/a"
			}
			provider := e.Provider
			if provider == "" {
				provider = "n/a"
			}
			model := e.Model
			if model == "" {
				model = "n/a"
			}
			cost := e.Cost
			if cost == "" {
				cost = "n/a"
			}
			turns := "n/a"
			if e.TurnsKnown {
				turns = fmt.Sprintf("%d", e.Turns)
			}
			fmt.Fprintf(b, "  workflow_attempts=%d runtime_attempts=%d agent_turns=%s tokens=%s input=%d output=%d reasoning=%d cache_read=%d cache_write=%d time=%s events=%d provider=%s model=%s cost=%s\n", e.Attempt, e.RuntimeAttempts, turns, tokens, e.InputTokens, e.OutputTokens, e.ReasoningTokens, e.CacheReadTokens, e.CacheWriteTokens, duration, e.RuntimeEvents, provider, model, cost)
		}
	}
}

func renderValidation(b *strings.Builder, result app.ValidationOperationResult) {
	fmt.Fprintf(b, "Validation: %s\nStatus: %s\n", result.Subject, result.Status)
	if len(result.Findings) == 0 {
		fmt.Fprintln(b, "No findings.")
		return
	}
	for _, f := range result.Findings {
		fmt.Fprintf(b, "- [%s] %s: %s\n", f.Severity, f.Path, f.Problem)
		if f.Suggestion != "" {
			fmt.Fprintf(b, "  Guidance: %s\n", f.Suggestion)
		}
	}
}

func renderTabs(m Model, width int) string {
	project := "Projects"
	study := "Studies"
	if m.ActiveTab == TabProjects {
		project = "[Projects]"
	}
	if m.ActiveTab == TabStudies {
		study = "[Studies]"
	}
	projectStyle, studyStyle := tuiStyles.tab, tuiStyles.tab
	if m.ActiveTab == TabProjects {
		projectStyle = tuiStyles.activeTab
	}
	if m.ActiveTab == TabStudies {
		studyStyle = tuiStyles.activeTab
	}
	if m.Focus == FocusTabs && m.ActiveTab == TabProjects {
		projectStyle = tuiStyles.focusedTab
	}
	if m.Focus == FocusTabs && m.ActiveTab == TabStudies {
		studyStyle = tuiStyles.focusedTab
	}
	row := " " + projectStyle.Render(project) + "  " + studyStyle.Render(study)
	return fullWidth(tuiStyles.body, row, width)
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

func renderFrame(header, body, help string, selectedStart, selectedEnd, offset int, mode viewportMode, width, height int) string {
	headerLines := splitLines(header)
	bodyLines := splitLines(body)
	footerLines := []string{renderHelp(help, width)}
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
	for i, line := range bodyLines[vp.offset:vp.End()] {
		absolute := vp.offset + i
		switch {
		case absolute >= selectedStart && absolute <= selectedEnd:
			fmt.Fprintln(&out, fullWidth(tuiStyles.selected, line, width))
		case isSectionLine(line):
			fmt.Fprintln(&out, fullWidth(tuiStyles.section, line, width))
		case strings.HasPrefix(line, "    ") || strings.HasPrefix(line, "  "):
			fmt.Fprintln(&out, fullWidth(tuiStyles.metadata, line, width))
		default:
			fmt.Fprintln(&out, fullWidth(tuiStyles.body, line, width))
		}
	}
	if len(bodyLines) > bodyHeight {
		fmt.Fprintln(&out, fullWidth(tuiStyles.scroll, fmt.Sprintf("scroll %d/%d", vp.offset+1, vp.MaxOffset()+1), width))
	} else {
		fmt.Fprintln(&out, fullWidth(tuiStyles.body, "", width))
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
