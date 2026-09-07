package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/Antonio7098/ultraplan-go/internal/app"
	"github.com/charmbracelet/lipgloss"
)

func Render(m Model, width int) string {
	return RenderWithSize(m, width, 40)
}

func RenderWithSize(m Model, width, height int) string {
	if width < 40 {
		width = 40
	}
	if height <= 0 {
		height = 40
	}
	// Header: tab bar (with counts), breadcrumb + status, error/loading
	// notices. Health strip pinned to the right side of the breadcrumb row.
	health := renderHealthStrip(m)
	headerLines := []string{renderTabBar(m, width)}
	headerLines = append(headerLines, fullWidth(tuiStyles.breadcrumb, statusDot(headerStatus(m))+"  UltraPlan · "+m.breadcrumb()+breadcrumbHealthSuffix(health), width))
	if m.Loading {
		headerLines = append(headerLines, fullWidth(tuiStyles.notice, "Loading workspace status...", width))
	}
	if m.Error != "" {
		headerLines = append(headerLines, fullWidth(tuiStyles.err, "Error: "+m.Error, width))
	}
	if m.Running && m.OperationHidden {
		headerLines = append(headerLines, fullWidth(tuiStyles.notice, "Run continues in background — c cancel | select View Run for status", width))
	}

	// Detail pane content preserves every previous body builder verbatim.
	var detail strings.Builder
	if m.ParallelForm != nil {
		renderParallelForm(&detail, m)
	} else if m.RunViewStudy != "" {
		renderRunView(&detail, m)
	} else if m.Confirmation != nil {
		renderConfirmation(&detail, *m.Confirmation)
	} else if m.Operation != nil && !m.OperationHidden {
		if m.ActiveOperation.Kind == app.OperationStudyStart || m.ActiveOperation.Kind == app.OperationStudyResume {
			renderForegroundRun(&detail, *m.Operation, m.Events, m.OperationShowPrevious)
		} else {
			renderOperation(&detail, *m.Operation, m.Events)
		}
	} else if m.Validation != nil {
		renderValidation(&detail, *m.Validation)
	} else if m.Preview != nil {
		renderPreview(&detail, m, width)
	} else {
		renderRouteSummary(&detail, m)
		renderSelectedDetail(&detail, m)
	}
	detailLines := splitLines(detail.String())

	// Sidebar carries navigation; the detail pane carries meaning.
	sideLines, selectedStart, selectedEnd := renderSidebarLines(m)
	detailOffset := 0
	if m.Preview != nil {
		detailOffset = m.PreviewOffset
	}

	// Footer: either the cmdline input (neovim's ":") or herdr's mode bar.
	var footerLines []string
	switch {
	case m.Input != nil && m.Input.Active:
		footerLines = []string{renderCmdline(m.Input.Value, width)}
	default:
		pill, alert := modePill(m)
		footerLines = []string{renderModeBar(pill, alert, helpSegments(m), width)}
		if m.Toast != "" {
			footerLines = append(footerLines, fullWidth(tuiStyles.notice, m.Toast, width))
		}
	}

	bodyHeight := height - len(headerLines) - len(footerLines) - 1
	if bodyHeight < 3 {
		bodyHeight = 3
	}
	sideW := width * 30 / 100
	if sideW < 30 {
		sideW = 30
	}
	if sideW > 44 {
		sideW = 44
	}
	if width-sideW < 30 {
		sideW = width - 30
	}
	detailW := width - sideW - 1
	if detailW < 20 {
		detailW = 20
	}

	sideVp := newViewport(len(sideLines), bodyHeight).FollowSelection(selectedStart, selectedEnd)
	// Map navItem index → jump-label rune so the inner loop can stamp them
	// into the visible rows. Indices past 35 get no label.
	navItems := m.navItems()
	detailVp := newViewport(len(detailLines), bodyHeight-2).AtOffset(detailOffset)

	var out strings.Builder
	for _, line := range headerLines {
		fmt.Fprintln(&out, line)
	}
	// Only style the visible sidebar window, not every row. Sprint and QA
	// nav can have 60+ items; pre-styling all of them dominated the per-frame
	// budget even when the viewport only shows ~20 rows.
	emptySide := tuiStyles.body.Width(sideW).MaxWidth(sideW).Render("")
	emptyDetail := paneBorderRow("", detailW, m.Focus == FocusContent)
	borderStyle := borderFG(m.Focus == FocusContent)
	sep := tuiStyles.separator.Render("│")
	// Sidebar columns: 1 sign cell + 2 jump-label cells + 1 gap + the rest.
	sideReserved := 4
	sideTextW := sideW - sideReserved
	if sideTextW < 12 {
		sideTextW = sideW
		sideReserved = 0
	}
	sidebarRoute := m.currentRoute()
	for i := 0; i < bodyHeight; i++ {
		sideCell := emptySide
		if idx := sideVp.offset + i; idx >= 0 && idx < len(sideLines) {
			line := sideLines[idx]
			cell := buildSidebarCell(line, navItems, sidebarRoute, sideTextW, sideReserved, idx)
			_ = cell
			sideCell = sidebarCellStyle(line).Width(sideW).MaxWidth(sideW).Render(composeSidebarCell(line, cell))
		}
		var detailLine string
		switch {
		case i == 0:
			detailLine = paneBorderTop(detailTitle(m), detailW, m.Focus == FocusContent)
		case i == bodyHeight-1:
			detailLine = paneBorderBottom(detailW, m.Focus == FocusContent)
		default:
			rowIdx := detailVp.offset + i - 1
			if rowIdx >= 0 && rowIdx < len(detailLines) {
				detailLine = paneBorderRow(detailLines[rowIdx], detailW, m.Focus == FocusContent)
			} else {
				detailLine = emptyDetail
			}
		}
		fmt.Fprintf(&out, "%s%s%s\n", sideCell, sep, detailLine)
		_ = borderStyle
	}
	if sideVp.MaxOffset() > 0 || detailVp.MaxOffset() > 0 {
		fmt.Fprintln(&out, fullWidth(tuiStyles.scroll, fmt.Sprintf("scroll %d/%d", sideVp.offset+1, sideVp.MaxOffset()+1), width))
	} else {
		fmt.Fprintln(&out, fullWidth(tuiStyles.body, "", width))
	}
	for _, line := range footerLines {
		fmt.Fprintln(&out, line)
	}
	return out.String()
}

// headerStatus picks the dot color state for the breadcrumb row.
func headerStatus(m Model) string {
	if m.Error != "" {
		return "failed"
	}
	if m.Running {
		return "running"
	}
	if m.Loading {
		return "waiting"
	}
	return "completed"
}

// modePill names the herdr-style mode pill for the bottom bar.
func modePill(m Model) (string, bool) {
	switch {
	case m.Running:
		return "RUNNING", true
	case m.ParallelForm != nil || m.Confirmation != nil:
		return "CONFIRM", true
	case m.Preview != nil:
		return "PREVIEW", false
	case m.Validation != nil:
		return "REVIEW", false
	default:
		return "OPERATE", false
	}
}

func helpSegments(m Model) []modeSegment {
	if m.Input != nil && m.Input.Active {
		return []modeSegment{
			{key: "esc", label: "cancel"},
			{key: "↵", label: "run"},
			{key: ":g X", label: "jump to label"},
			{key: ":p", label: "projects · "},
			{key: ":s", label: "studies · "},
			{key: ":r", label: "runs"},
		}
	}
	segments := []modeSegment{}
	for _, part := range strings.Split(HelpText(), " · ") {
		fields := strings.SplitN(part, " ", 2)
		if len(fields) == 2 {
			segments = append(segments, modeSegment{key: fields[0], label: fields[1]})
		} else {
			segments = append(segments, modeSegment{label: part})
		}
	}
	return segments
}

// detailTitle puts the route label on the detail pane's top border,
// the way herdr titles its focused pane chrome.
func detailTitle(m Model) string {
	if m.ParallelForm != nil {
		return "run-loop parameters"
	}
	if m.RunViewStudy != "" {
		return "run · " + m.RunViewStudy
	}
	if m.Confirmation != nil {
		return "confirm operation"
	}
	if m.Operation != nil && !m.OperationHidden {
		return "operation"
	}
	if m.Validation != nil {
		return "validation"
	}
	if m.Preview != nil {
		title := m.PreviewTitle
		if title == "" {
			title = "preview"
		}
		return title
	}
	route := m.currentRoute()
	switch route.Kind {
	case RouteProjects:
		return "projects"
	case RouteProject:
		return route.Project + " · project"
	case RouteProjectSprints:
		return route.Project + " · sprints"
	case RouteProjectDocs:
		return route.Project + " · docs"
	case RouteSprint:
		return route.Project + " / " + route.Sprint
	case RouteSprintQA:
		return route.Project + " / " + route.Sprint + " · qa"
	case RouteSprintQAShard:
		return "qa shard · " + route.Shard
	case RouteSprintQATheory:
		return "qa theory · " + route.Theory
	case RouteSprintRepair:
		return "bounded repair"
	case RouteStudies:
		return "studies"
	case RouteStudy:
		return route.Study + " · study"
	case RouteStudyDims:
		return route.Study + " · dimensions"
	case RouteStudySources:
		return route.Study + " · sources"
	case RouteRuns:
		return "runs"
	case RouteRun:
		return "run · " + route.RunID
	default:
		return route.Project + " / " + route.Sprint
	}
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
	// fallback saturation banner: if ≥2 active tasks have fallen back from primary, surface it
	fallbackCount, staleCount := 0, 0
	fallbackExampleFrom, fallbackExampleTo := "", ""
	for _, task := range s.Tasks {
		if task.FallbackFrom != "" {
			fallbackCount++
			if fallbackExampleFrom == "" {
				fallbackExampleFrom, fallbackExampleTo = task.FallbackFrom, task.FallbackTo
			}
		}
		if task.Stale {
			staleCount++
		}
	}
	if fallbackCount >= 2 {
		fmt.Fprintf(b, "⚠ %d tasks fell back from %s → %s (primary failing)\n", fallbackCount, fallbackExampleFrom, fallbackExampleTo)
	}
	if staleCount > 0 {
		fmt.Fprintf(b, "⚠ %d running tasks stale >2m (no progress)\n", staleCount)
	}
	if s.Retries.TotalRetries > 0 {
		fmt.Fprintf(b, "Retries: %d tasks, %d total (same-session %d, fresh %d)\n", s.Retries.RetriedTasks, s.Retries.TotalRetries, s.Retries.SameSession, s.Retries.FreshSession)
	}
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
	// stale indicator: running/validating/retrying but no update >2m
	staleMark := ""
	if task.Stale {
		staleMark = " STALE"
	}
	fmt.Fprintf(b, "- %s [%s%s] %s\n", task.ID, task.Status, staleMark, identity)
	fmt.Fprintf(b, "  workflow_attempts=%d runtime_attempts=%d agent_turns=%s tokens=%s input=%d output=%d reasoning=%d cache_read=%d cache_write=%d time=%s events=%d provider=%s model=%s cost=%s\n", task.Attempts, task.RuntimeAttempts, turns, tokens, task.InputTokens, task.OutputTokens, task.ReasoningTokens, task.CacheReadTokens, task.CacheWriteTokens, task.Duration, task.Events, provider, model, task.Cost)
	if task.Stale && !task.UpdatedAt.IsZero() {
		fmt.Fprintf(b, "  stale: no update since %s (%.0fs ago)\n", task.UpdatedAt.UTC().Format(time.RFC3339), time.Since(task.UpdatedAt).Seconds())
	}
	if task.FallbackFrom != "" && task.FallbackTo != "" {
		fmt.Fprintf(b, "  fallback: %s → %s\n", task.FallbackFrom, task.FallbackTo)
	}
	if len(task.AttemptHistory) > 1 {
		var parts []string
		for _, a := range task.AttemptHistory {
			label := a.Provider + "/" + a.Model
			if label == "/" {
				label = a.Status
			}
			if a.ErrorCategory != "" {
				label += ":" + a.ErrorCategory
			}
			parts = append(parts, label)
		}
		fmt.Fprintf(b, "  attempts: %s\n", strings.Join(parts, " → "))
	}
	if task.Error != "" {
		code := task.ErrorCode
		if code == "" {
			code = "error"
		}
		fmt.Fprintf(b, "  %s: %s\n", code, truncateForDisplay(task.Error, 300))
	}
	if task.RetryAfter != nil {
		fmt.Fprintf(b, "  retry_after: %s\n", task.RetryAfter.UTC().Format(time.RFC3339))
	}
}

func truncateForDisplay(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "..."
}

func renderRouteSummary(b *strings.Builder, m Model) {
	route := m.currentRoute()
	if route.Kind == RouteRun {
		for _, run := range m.Runs {
			if string(run.RunID) != route.RunID {
				continue
			}
			productStatus := run.ProductStatus
			if productStatus == "" {
				productStatus = "unknown"
			}
			fmt.Fprintf(b, "Durable run: %s\nLifecycle: %s\nLiveness: %s\nProduct status: %s\nCancellation: %s\nHistory: %s complete=%t oldest=%d last=%d\n",
				run.RunID, run.Lifecycle, run.Liveness, productStatus, run.Cancellation.State, run.RecordState, run.HistoryComplete, run.OldestRetainedSequence, run.LastSequence)
			if run.Terminal != nil {
				fmt.Fprintf(b, "Terminal: %s (%s)\n", run.Terminal.Outcome, run.Terminal.Reason)
			}
			fmt.Fprintln(b, "\nRetained events")
			for _, event := range m.DurableEvents {
				fmt.Fprintf(b, "- %d %s stage=%s task=%s", event.Sequence, event.Type, event.Stage, event.Task)
				if event.Omission != nil {
					fmt.Fprintf(b, " omitted=%d reason=%s", event.Omission.Count, event.Omission.Reason)
				}
				fmt.Fprintln(b)
			}
			if run.Lifecycle.IsActive() {
				fmt.Fprintln(b, "\nc requests durable cancellation; q leaves the run unchanged.")
			}
			return
		}
		fmt.Fprintln(b, "Durable run is no longer retained.")
		return
	}
	if route.Kind == RouteSprint {
		sprint, ok := findSprint(m.Data.Sprints, route.Project, route.Sprint)
		if !ok {
			return
		}
		fmt.Fprintln(b, "Sprint flow summary")
		fmt.Fprintf(b, "  Conformance Review: %s verdict=%s stale=%t evidence=%s\n", sprint.Review.Status, sprint.Review.Verdict, sprint.Review.Stale, sprint.Review.Artifact)
		for _, reason := range sprint.Review.FreshnessReasons {
			fmt.Fprintf(b, "    Reason: %s\n", reason)
		}
		fmt.Fprintf(b, "  QA: %s assessment=%s fresh=%t shards=%d/%d issues=%d\n", sprint.QA.Phase, sprint.QA.Assessment, sprint.QA.Fresh, sprint.QA.CompletedShards, sprint.QA.TotalShards, sprint.QA.IssueCount)
		fmt.Fprintf(b, "  Standalone Smoke: %s verdict=%s stale=%t run=%s evidence=%s\n", sprint.Smoke.Status, sprint.Smoke.Verdict, sprint.Smoke.Stale, sprint.Smoke.RunID, sprint.Smoke.Artifact)
		for _, issue := range sprint.Smoke.Issues {
			fmt.Fprintf(b, "    Issue: %s [%s] %s\n", issue.ID, issue.Status, issue.Path)
		}
		if sprint.Smoke.Override != nil && sprint.Smoke.Override.Requested {
			fmt.Fprintf(b, "    Diagnostic override: %s\n", sprint.Smoke.Override.Rationale)
		}
		fmt.Fprintf(b, "  Overall: %s\n  Next: %s\n\n", sprint.Assessment, sprint.NextAction)
		return
	}
	if route.Kind == RouteSprintQA || route.Kind == RouteSprintQAShard || route.Kind == RouteSprintQATheory {
		renderSprintQAView(b, m, route)
		return
	}
	if route.Kind == RouteSprintRepair {
		renderSprintRepairView(b, m, route)
		return
	}
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
	if r.RunID != "" {
		fmt.Fprintf(b, "Durable run: %s\n", r.RunID)
	}
	if r.Truncated {
		fmt.Fprintln(b, "Truncated: true")
	}
	if r.Content != "" {
		fmt.Fprintln(b, r.Content)
	}
	if r.Error != nil {
		fmt.Fprintf(b, "Error code: %s (%s)\nComponent: %s\nRetryable: %t\nGuidance: %s\n", r.Error.Code, r.Error.Category, r.Error.Component, r.Error.Retryable, r.Error.Guidance)
	}
	if len(r.Findings) > 0 {
		fmt.Fprintln(b, "Findings:")
		for _, finding := range r.Findings {
			fmt.Fprintf(b, "- [%s] %s: %s\n", finding.Severity, finding.Section, finding.Problem)
			if finding.Cause != "" {
				fmt.Fprintf(b, "  Cause: %s\n", finding.Cause)
			}
			if finding.Suggestion != "" {
				fmt.Fprintf(b, "  Guidance: %s\n", finding.Suggestion)
			}
		}
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

// renderTabBar mirrors herdr's tab strip and neovim's tabline: each tab
// carries a count badge. Counts come from the dashboard; the Runs tab
// also gets a yellow dot when work is in flight. Padding lives only on
// the label style; the badge gets one-cell framing manually so the two
// halves sit flush without doubling up.
func renderTabBar(m Model, width int) string {
	type tab struct {
		label   string
		badge   string
		alerted bool
		active  bool
	}
	activeRuns := countActiveRuns(m.Runs)
	tabs := []tab{
		{label: "Projects", badge: fmt.Sprintf("%d", len(m.Data.Projects)), active: m.ActiveTab == TabProjects},
		{label: "Studies", badge: fmt.Sprintf("%d", len(m.Data.Studies)), active: m.ActiveTab == TabStudies},
		{label: "Runs", badge: fmt.Sprintf("%d", len(m.Runs)), active: m.ActiveTab == TabRuns, alerted: activeRuns > 0},
	}
	var row strings.Builder
	for _, t := range tabs {
		labelStyle := tuiStyles.dimTab
		var badgeFG, badgeBG lipgloss.Color
		bold := false
		switch {
		case t.active && m.Focus == FocusTabs:
			labelStyle = tuiStyles.focusedTab
			badgeFG, badgeBG, bold = palette.amber, palette.activeRow, true
		case t.active:
			labelStyle = tuiStyles.activeTab
			badgeFG, badgeBG, bold = palette.contrast, palette.blue, true
		default:
			badgeFG, badgeBG, bold = palette.overlay1, palette.panel, false
		}
		badgeStyle := lipgloss.NewStyle().Foreground(badgeFG).Background(badgeBG).Bold(bold)
		badgeText := t.badge
		if t.alerted {
			dot := tuiStyles.dot.Foreground(palette.yellow).Render("●")
			row.WriteString(labelStyle.Render(t.label))
			row.WriteString(dot + badgeStyle.Render(" "+badgeText+" "))
			row.WriteString(tuiStyles.tabBar.Render(" "))
			continue
		}
		row.WriteString(labelStyle.Render(t.label))
		row.WriteString(badgeStyle.Render(" " + badgeText + " "))
		row.WriteString(tuiStyles.tabBar.Render(" "))
	}
	return fullWidth(tuiStyles.tabBar, row.String(), width)
}

func countActiveRuns(runs []app.RunSnapshot) int {
	n := 0
	for _, r := range runs {
		if r.Lifecycle.IsActive() {
			n++
		}
	}
	return n
}

type sidebarLine struct {
	text     string
	header   bool
	divider  bool
	selected bool
	summary  bool
}

// renderSidebarLines builds herdr-style sidebar rows: a lowercase section
// header, one selectable row per nav item (▸ marker, › drill hint), and the
// preserved dim summary line under items that have one.
func renderSidebarLines(m Model) ([]sidebarLine, int, int) {
	items := m.navItems()
	lines := []sidebarLine{{text: fmt.Sprintf(" nav (%d)", len(items)), header: true}}
	if len(items) == 0 {
		lines = append(lines, sidebarLine{text: "(none)"})
		return lines, -1, -1
	}
	selectedStart, selectedEnd := -1, -1
	for i, item := range items {
		selected := m.Focus == FocusContent && i == m.Selected
		if selected {
			selectedStart = len(lines)
		}
		marker := "  "
		if selected {
			marker = "▸ "
		}
		row := marker + item.Label
		if item.Route != nil {
			row += " ›"
		}
		lines = append(lines, sidebarLine{text: row, selected: selected})
		var summary strings.Builder
		renderItemSummary(&summary, m, item)
		for _, line := range splitLines(summary.String()) {
			lines = append(lines, sidebarLine{text: line, selected: selected, summary: true})
		}
		if selected {
			selectedEnd = len(lines) - 1
		}
	}
	return lines, selectedStart, selectedEnd
}

// renderSelectedDetail keeps the selected item's context visible in the detail
// pane on plain navigation routes (which previously rendered no summary).
func renderSelectedDetail(b *strings.Builder, m Model) {
	item, ok := m.selectedItem()
	if !ok {
		return
	}
	var summary strings.Builder
	renderItemSummary(&summary, m, item)
	if text := strings.TrimRight(summary.String(), "\n"); text != "" {
		fmt.Fprintln(b, text)
		fmt.Fprintln(b)
	}
	switch {
	case item.Route != nil:
		fmt.Fprintln(b, "Enter drills in.")
	case item.Path != "":
		fmt.Fprintln(b, "Enter previews the artifact.")
	case item.Validation != nil:
		fmt.Fprintln(b, "Enter runs validation.")
	case item.Operation != nil:
		fmt.Fprintln(b, "Enter reviews the operation before anything runs.")
	case item.ViewRun != "":
		fmt.Fprintln(b, "Enter watches the active run.")
	}
}

func borderFG(focused bool) lipgloss.Style {
	if focused {
		return lipgloss.NewStyle().Foreground(palette.blue)
	}
	return lipgloss.NewStyle().Foreground(palette.overlay0)
}

// paneBorderTop draws herdr-style pane chrome: a single-line top border with
// the pane title seated in it (" title " interrupting the rule).
func paneBorderTop(title string, width int, focused bool) string {
	fg := borderFG(focused)
	label := " " + title + " "
	if lipgloss.Width(label) > width-4 && width > 6 {
		label = " " + title[:width-6] + " "
	}
	fill := width - 3 - lipgloss.Width(label)
	if fill < 0 {
		fill = 0
	}
	return fg.Render("┌─") + tuiStyles.paneTitle.Render(label) +
		fg.Render(strings.Repeat("─", fill)+"┐")
}

func paneBorderRow(line string, width int, focused bool) string {
	fg := borderFG(focused)
	inner := width - 2
	if inner < 1 {
		inner = 1
	}
	// Telemetry lines can exceed the pane: pad short lines, let long ones
	// overflow unwrapped so their key=value content stays greppable and the
	// terminal wraps them naturally instead of mid-token.
	cell := line
	if lipgloss.Width(line) <= inner {
		cell = detailLineStyle(line).Width(inner).MaxWidth(inner).Render(line)
	}
	return fg.Render("│") + cell + fg.Render("│")
}

func paneBorderBottom(width int, focused bool) string {
	fg := borderFG(focused)
	fill := width - 2
	if fill < 0 {
		fill = 0
	}
	return fg.Render("└" + strings.Repeat("─", fill) + "┘")
}

// detailLineStyle keeps the old body semantics (accent section headers, dim
// metadata) inside the bordered pane.
func detailLineStyle(line string) lipgloss.Style {
	trimmed := strings.TrimSpace(line)
	switch {
	case isSectionLine(line):
		return tuiStyles.section
	case strings.HasPrefix(line, "    ") || strings.HasPrefix(line, "  "):
		return tuiStyles.metadata
	case trimmed == "" || trimmed == "(none)":
		return tuiStyles.metadata
	default:
		return tuiStyles.body
	}
}

func sidebarCellStyle(line sidebarLine) lipgloss.Style {
	switch {
	case line.header:
		return tuiStyles.sideHeader
	case line.divider:
		return tuiStyles.sideDivider
	case line.selected && line.summary:
		return lipgloss.NewStyle().Foreground(palette.subtext).Background(palette.selectionBg)
	case line.selected:
		return tuiStyles.sideSelected
	case line.summary:
		return tuiStyles.metadata
	default:
		return tuiStyles.body
	}
}

func renderItemSummary(b *strings.Builder, m Model, item navItem) {
	route := m.currentRoute()
	switch route.Kind {
	case RouteProjects:
		if p, ok := findProject(m.Data.Projects, item.Label); ok {
			fmt.Fprintf(b, "    docs=%s roadmap=%s index=%s catalog=%s project_reasoning=%s/%s findings=%d\n", p.DocsDir, p.Roadmap, p.ProjectIndex, p.Catalog, p.ProjectReasoning.Mode, p.ProjectReasoning.Verdict, len(p.Findings))
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

// navItemsFor is a small helper exposed by the sidebar renderer so the sign
// column can be computed without re-running the model.
func navItemsFor(m Model) []navItem { return m.navItems() }

// renderCmdline draws the single-line input that replaces the mode bar
// when ":" is pressed. Mirrors neovim's cmdline window: prompt on the
// left in accent, typed text in default foreground, a blinking cursor
// (rendered as a solid bar) at the end, hint suffix on the right.
func renderCmdline(value string, width int) string {
	if width < 20 {
		width = 20
	}
	prompt := tuiStyles.modePill.Render(" : ")
	typed := value
	cursor := tuiStyles.key.Render("▌")
	if typed == "" {
		cursor = ""
	}
	body := typed + cursor
	// Pad with spaces so the row covers the full width and stays a single
	// visible cell at the bottom of the screen.
	padding := width - lipgloss.Width(prompt) - lipgloss.Width(body) - 1
	if padding < 1 {
		padding = 1
	}
	return fullWidth(tuiStyles.modeBase, prompt+tuiStyles.modeBase.Render(body)+strings.Repeat(" ", padding), width)
}

// renderHealthStrip builds the right-side summary that sits next to the
// breadcrumb: counts of running / stale / failed studies / runs. Empty
// strings collapse to keep the row quiet when there's nothing to say.
func renderHealthStrip(m Model) string {
	running := 0
	failed := 0
	stale := 0
	for _, r := range m.Runs {
		if r.Lifecycle.IsActive() {
			running++
		}
		if r.ProductStatus == "failed" || r.ProductStatus == "blocked" {
			failed++
		}
		if r.Liveness == "stale" {
			stale++
		}
	}
	for _, s := range m.Data.Studies {
		if s.RunActive {
			running++
		}
		if s.Failed > 0 {
			failed++
		}
	}
	var parts []string
	if running > 0 {
		parts = append(parts, fmt.Sprintf("● %d active", running))
	}
	if stale > 0 {
		parts = append(parts, fmt.Sprintf("◌ %d stale", stale))
	}
	if failed > 0 {
		parts = append(parts, fmt.Sprintf("✗ %d failed", failed))
	}
	return strings.Join(parts, " · ")
}

// breadcrumbHealthSuffix right-aligns the health strip onto the breadcrumb
// line, falling back to "" when the strip is empty.
func breadcrumbHealthSuffix(health string) string {
	if health == "" {
		return ""
	}
	return "  ·  " + health
}

// buildSidebarCell composes the visible sign + label + text columns for
// one sidebar row. Sign column = state glyph (1 cell). Jump column =
// "1 ", "a ", or "  " (2 cells). The remainder holds the sidebar text.
func buildSidebarCell(line sidebarLine, items []navItem, route Route, textWidth, reserved int, lineIndex int) string {
	if reserved == 0 {
		return fitCell(line.text, textWidth)
	}
	// Header rows and summary rows aren't selectable: leave both columns
	// blank so they read as plain text in the sidebar.
	sign := " "
	jump := "  "
	if !line.header && !line.summary {
		idx := visibleNavIndex(items, lineIndex)
		if idx >= 0 {
			s := signForNavItem(items, route, -1, idx)
			signCell := s.glyph()
			if signCell != " " {
				sign = s.style(line.selected).Render(signCell)
			}
			label := jumpLabelFor(idx)
			if label != 0 {
				jumpStyle := tuiStyles.sideNum
				if line.selected {
					jumpStyle = jumpStyle.Background(palette.selectionBg).Foreground(palette.text).Bold(true)
				}
				jump = jumpStyle.Render(fmt.Sprintf("%c ", label))
			}
		}
	}
	row := fitCell(line.text, textWidth)
	return sign + jump + row
}

// visibleNavIndex recovers the navItem index for a visible sidebarLine
// position, skipping the leading section header and any summary rows.
// Returns -1 if the visible row doesn't correspond to a nav item.
func visibleNavIndex(items []navItem, lineIndex int) int {
	if lineIndex <= 0 {
		return -1
	}
	pos := lineIndex - 1
	for i, item := range items {
		if pos == 0 {
			return i
		}
		// Each nav item occupies 1 row plus a dim summary line when the
		// route kind makes renderItemSummary emit content.
		var summary strings.Builder
		renderItemSummary(&summary, Model{Routes: []Route{{Kind: itemsRouteKind(item)}}}, item)
		summaryRows := len(splitLines(summary.String()))
		pos--
		if summaryRows > 0 {
			pos -= summaryRows
		}
		if pos < 0 {
			return -1
		}
	}
	return -1
}

// itemsRouteKind guesses the parent route kind for a given nav item based
// on its fields, used only for summary sizing inside visibleNavIndex.
func itemsRouteKind(item navItem) RouteKind {
	if item.Route != nil {
		return item.Route.Kind
	}
	return RouteProjects
}

// composeSidebarCell assembles the fully styled cell from the precomputed
// cell string. Keeping the cell prebuilt lets the per-row loop stay tiny.
func composeSidebarCell(_ sidebarLine, cell string) string { return cell }

// buildSidebarCell falls back to `line.text` when columns collapse.

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
		if preview.Kind == "markdown" {
			// Markdown previews render once via the global glamour cache
			// (see markdown.go); here we just write the cached lines.
			wrap := width - 2
			if wrap < 20 {
				wrap = 20
			}
			for _, line := range cachedMarkdownLines(preview.Content, wrap) {
				fmt.Fprintln(b, line)
			}
			return
		}
		fmt.Fprintln(b, preview.Content)
	}
}

// fitCell truncates a sidebar cell to the column width so long labels clip
// instead of wrapping mid-row and breaking the selection column. The naive
// "drop a rune, measure, repeat" loop is O(n²); for a 60-row sidebar that's
// the difference between snappy and laggy. Bound by rune count first (most
// display cells are width-1), then refine once for double-width chars.
func fitCell(text string, width int) string {
	if width <= 1 {
		return "…"
	}
	if lipgloss.Width(text) <= width {
		return text
	}
	runes := []rune(text)
	if len(runes) >= width {
		runes = runes[:width-1]
	}
	for len(runes) > 0 && lipgloss.Width(string(runes)) >= width {
		runes = runes[:len(runes)-1]
	}
	if len(runes) == 0 {
		return "…"
	}
	return string(runes) + "…"
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
