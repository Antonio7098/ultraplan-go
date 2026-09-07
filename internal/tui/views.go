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
	// Header: herdr-style tab bar, then breadcrumb + status on the panel color.
	headerLines := []string{renderTabBar(m, width)}
	headerLines = append(headerLines, fullWidth(tuiStyles.breadcrumb, statusDot(headerStatus(m))+"  UltraPlan · "+m.breadcrumb(), width))
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

	// Footer: herdr-style mode bar with the pill reflecting state.
	pill, alert := modePill(m)
	footerLines := []string{renderModeBar(pill, alert, helpSegments(), width)}

	bodyHeight := height - len(headerLines) - len(footerLines) - 1
	if bodyHeight < 3 {
		bodyHeight = 3
	}
	sideW := width * 30 / 100
	if sideW < 24 {
		sideW = 24
	}
	if sideW > 38 {
		sideW = 38
	}
	if width-sideW < 30 {
		sideW = width - 30
	}
	detailW := width - sideW - 1
	if detailW < 20 {
		detailW = 20
	}

	sideVp := newViewport(len(sideLines), bodyHeight).FollowSelection(selectedStart, selectedEnd)
	detailVp := newViewport(len(detailLines), bodyHeight-2).AtOffset(detailOffset)

	var out strings.Builder
	for _, line := range headerLines {
		fmt.Fprintln(&out, line)
	}
	sideStyled := make([]string, len(sideLines))
	for i, line := range sideLines {
		sideStyled[i] = sidebarCellStyle(line).Width(sideW).MaxWidth(sideW).Render(fitCell(line.text, sideW))
	}
	for i := 0; i < bodyHeight; i++ {
		sideCell := tuiStyles.body.Width(sideW).MaxWidth(sideW).Render("")
		if idx := sideVp.offset + i; idx < len(sideStyled) {
			sideCell = sideStyled[idx]
		}
		// herdr's thin │ separator between sidebar and content.
		sep := tuiStyles.separator.Render("│")
		detailLine := ""
		if i == 0 {
			detailLine = paneBorderTop(detailTitle(m), detailW, m.Focus == FocusContent)
		} else if i == bodyHeight-1 {
			detailLine = paneBorderBottom(detailW, m.Focus == FocusContent)
		} else if idx := detailVp.offset + i - 1; idx < len(detailLines) {
			detailLine = paneBorderRow(detailLines[idx], detailW, m.Focus == FocusContent)
		} else {
			detailLine = paneBorderRow("", detailW, m.Focus == FocusContent)
		}
		fmt.Fprintf(&out, "%s%s%s\n", sideCell, sep, detailLine)
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

func helpSegments() []modeSegment {
	segments := []modeSegment{}
	for _, part := range strings.Split(HelpText(), " | ") {
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

// renderTabBar mirrors herdr's tab strip: one panel-bg row, each tab a padded
// label with a one-cell gap. The focused tab is accent-on-dark; the rest are
// dimmed on surface0. A keyboard-focused tab gets the amber focus treatment.
func renderTabBar(m Model, width int) string {
	tabs := []struct {
		label  string
		active bool
	}{
		{"Projects", m.ActiveTab == TabProjects},
		{"Studies", m.ActiveTab == TabStudies},
		{"Runs", m.ActiveTab == TabRuns},
	}
	var row strings.Builder
	for _, tab := range tabs {
		style := tuiStyles.dimTab
		switch {
		case tab.active && m.Focus == FocusTabs:
			style = tuiStyles.focusedTab
		case tab.active:
			style = tuiStyles.activeTab
		}
		row.WriteString(style.Render(tab.label))
		row.WriteString(tuiStyles.tabBar.Render(" "))
	}
	return fullWidth(tuiStyles.tabBar, row.String(), width)
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

// fitCell truncates a sidebar cell to the column width so long labels clip
// instead of wrapping mid-row and breaking the selection column.
func fitCell(text string, width int) string {
	if lipgloss.Width(text) <= width {
		return text
	}
	runes := []rune(text)
	for len(runes) > 0 && lipgloss.Width(string(runes)) > width-1 {
		runes = runes[:len(runes)-1]
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
