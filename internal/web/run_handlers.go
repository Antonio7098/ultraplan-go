package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Antonio7098/ultraplan-go/internal/app"
)

const runSurfaceContract template.HTML = `<!-- THESIS: The QA run page is an evidence case board, with current health and investigation state ahead of chronology. OWN-WORLD: UltraPlan's graphite field, violet focus, thin borders, compact status language, and dense disclosure controls. STORY: Operators confirm freshness, find the shard that needs attention, inspect its bounded evidence, then verify synthesis against the durable journal. FIRST VIEWPORT: Run control remains first, followed by QA progress, health, coverage, and next action. FORM: Established Operate surface, local extension, no concept seed. FINISH: unreviewed and undocumented is unfinished; this build ends with the finish review, the verdict, DESIGN.md, and every shipping raster carrying its provenance --><!-- qa-run-cockpit-v1 -->`

type runPageFilters struct {
	Project, Sprint, Study, Lifecycle string
}

type runStateView struct {
	Value string
	Cue   string
	Tone  string
}

type runRowView struct {
	RunID         app.RunID
	Target        string
	Lifecycle     runStateView
	Liveness      runStateView
	ProductStatus string
	UpdatedAt     time.Time
}

type runDetailView struct {
	RunID                  app.RunID
	LastSequence           uint64
	OldestRetainedSequence uint64
	OmissionTotal          uint64
	CurrentAttempt         string
	Title                  string
	Scope                  string
	Target                 string
	Lifecycle              runStateView
	Liveness               runStateView
	Product                string
	Cancellation           runStateView
	History                string
	Terminal               string
	TerminalOutcome        string
	TerminalReason         string
	Duration               string
	IsActive               bool
}

type runStudyInsightsView struct {
	Study       string
	Status      string
	RunID       string
	Total       int
	Completed   int
	Pending     int
	ActiveTasks int
	Failed      int
	Cancelled   int
	Retries     studyRetryDTO
	Parallelism *studyParallelismDTO
	Tasks       []studyTaskPerfDTO
	Failures    []studyTaskFailureDTO
	SeedTasks   []studyTaskSeedDTO
	Usage       runUsageView
}

type runQACountView struct {
	Label string
	Value int
}

type runQAInsightsView struct {
	Project, Sprint, StatusURL, SynthesisURL string
	QA                                       app.QAResult
	Synthesis                                app.QASynthesisResult
	Outcomes                                 []runQACountView
	CompletionPercent, ProgressMax           int
	Attempts, Commands, ContextRequests      int
	Evidence, Theories, ApprovedChecks       int
	HasSynthesis                             bool
	Unavailable, SynthesisUnavailable        string
	Historical                               bool
	CurrentRunID                             string
}

type studyTaskFailureDTO struct {
	Task    string `json:"task"`
	Code    string `json:"code,omitempty"`
	Message string `json:"message"`
}

type studyTaskSeedDTO struct {
	Task         string `json:"task"`
	Status       string `json:"status"`
	Attempts     int    `json:"attempts,omitempty"`
	Retries      int    `json:"retries,omitempty"`
	RetryAfter   string `json:"retry_after,omitempty"`
	Provider     string `json:"provider,omitempty"`
	Model        string `json:"model,omitempty"`
	Harness      string `json:"harness,omitempty"`
	SessionID    string `json:"session_id,omitempty"`
	SessionReuse string `json:"session_reuse,omitempty"`
}

type studyTaskPerfDTO struct {
	ID           string `json:"id"`
	Kind         string `json:"kind,omitempty"`
	Status       string `json:"status,omitempty"`
	Duration     string `json:"duration,omitempty"`
	Turns        int64  `json:"turns,omitempty"`
	Tokens       int64  `json:"tokens,omitempty"`
	Input        int64  `json:"input,omitempty"`
	Output       int64  `json:"output,omitempty"`
	Reasoning    int64  `json:"reasoning,omitempty"`
	CacheRead    int64  `json:"cache_read,omitempty"`
	CacheWrite   int64  `json:"cache_write,omitempty"`
	Cost         string `json:"cost,omitempty"`
	CostSource   string `json:"cost_source,omitempty"`
	Retries      int    `json:"retries,omitempty"`
	SessionReuse string `json:"session_reuse,omitempty"`
}

// runUsageView aggregates token and cost facts for a run. Cost figures are
// API-equivalent estimates: provider-reported values are exact, model-priced
// values are computed from public rate tables, and unpriced tasks contribute
// tokens but no cost.
type runUsageView struct {
	Scope            string
	HasUsage         bool
	TasksWithUsage   int
	TasksPriced      int
	TasksUnpriced    int
	Input            int64
	Output           int64
	Reasoning        int64
	CacheRead        int64
	CacheWrite       int64
	Total            int64
	TotalKnown       bool
	cacheDenominator int64
	cacheHitComplete bool
	CacheHit         string
	cost             float64
	costKnown        bool
	CostLabel        string
	ProviderReported int
	ModelPriced      int
	// Tasks preserves the per-task summaries that fed the aggregate so
	// dashboards can render a per-task breakdown alongside totals.
	Tasks []app.RunTaskSummary
}

func (v *runUsageView) addTokens(inputKnown bool, input int64, outputKnown bool, output int64, reasoningKnown bool, reasoning int64,
	cacheReadKnown bool, cacheRead int64, cacheWriteKnown bool, cacheWrite int64, totalKnown bool, total int64) {
	v.HasUsage = true
	v.TasksWithUsage++
	if !inputKnown || !cacheReadKnown || !cacheWriteKnown {
		v.cacheHitComplete = false
	}
	if inputKnown {
		v.Input += input
	}
	if outputKnown {
		v.Output += output
	}
	if reasoningKnown {
		v.Reasoning += reasoning
	}
	if cacheReadKnown {
		v.CacheRead += cacheRead
	}
	if cacheWriteKnown {
		v.CacheWrite += cacheWrite
	}
	if cacheReadKnown && cacheWriteKnown && v.cacheHitComplete {
		v.cacheDenominator += input + cacheRead + cacheWrite
	}
	if totalKnown {
		v.Total += total
		v.TotalKnown = true
	}
}

func (v *runUsageView) addCost(known bool, amount float64, source string) {
	switch source {
	case "provider_reported", "model_priced":
	case "":
		source = "provider_reported"
	default:
		source = ""
	}
	if !known {
		if v.HasUsage {
			v.TasksUnpriced++
		}
		return
	}
	v.cost += amount
	v.costKnown = true
	v.TasksPriced++
	switch source {
	case "model_priced":
		v.ModelPriced++
	case "provider_reported":
		v.ProviderReported++
	default:
		v.TasksUnpriced++
	}
}

type runEventView struct {
	Sequence      uint64
	AttemptID     string
	Type          string
	Stage         string
	Task          string
	Time          string
	DetailKind    string
	DetailType    string
	DetailTool    string
	DetailState   string
	DetailAction  string
	DetailReason  string
	DetailCount   string
	DetailText    string
	PhaseState    string
	Summary       string
	Action        string
	Reason        string
	ToolCallID    string
	ToolStatus    string
	ToolArguments string
	ToolResult    string
	ToolError     string
	Omission      string
}

func (h *handler) handleRuns(w http.ResponseWriter, r *http.Request) {
	if h.runs == nil {
		h.writeError(w, r, http.StatusServiceUnavailable, "run_control_unavailable", "Durable run observation is unavailable.")
		return
	}
	values := r.URL.Query()
	if !onlyQueryKeys(values, "project", "sprint", "study", "lifecycle", "limit", "after") {
		h.writeError(w, r, http.StatusBadRequest, "invalid_request", "Unknown query parameters are not accepted.")
		return
	}
	limit := 50
	if text := values.Get("limit"); text != "" {
		parsed, err := strconv.Atoi(text)
		if err != nil || parsed < 1 || parsed > 200 {
			h.writeError(w, r, http.StatusBadRequest, "invalid_limit", "The run limit must be between 1 and 200.")
			return
		}
		limit = parsed
	}
	lifecycles, err := webLifecycleFilter(values.Get("lifecycle"))
	if err != nil {
		h.writeError(w, r, http.StatusBadRequest, "invalid_lifecycle", "The lifecycle filter contains an unknown value.")
		return
	}
	page, err := h.runs.Runs(r.Context(), app.RunQuery{
		Lifecycle: lifecycles, Project: values.Get("project"), Sprint: values.Get("sprint"), Study: values.Get("study"),
		Limit: limit, After: values.Get("after"),
	})
	if err != nil {
		h.handleRunControlError(w, r, err)
		return
	}
	h.writeSuccess(w, r, http.StatusOK, page, nil)
}

func (h *handler) handleRunsPage(w http.ResponseWriter, r *http.Request) {
	if h.runs == nil {
		h.renderError(w, r, http.StatusServiceUnavailable, "Runs unavailable", "Durable run observation is unavailable.")
		return
	}
	values := r.URL.Query()
	if !onlyQueryKeys(values, "project", "sprint", "study", "lifecycle", "after") {
		h.renderError(w, r, http.StatusBadRequest, "Invalid filters", "Unknown run filters are not accepted.")
		return
	}
	lifecycles, err := webLifecycleFilter(values.Get("lifecycle"))
	if err != nil {
		h.renderError(w, r, http.StatusBadRequest, "Invalid filters", "The lifecycle filter contains an unknown value.")
		return
	}
	page, err := h.runs.Runs(r.Context(), app.RunQuery{
		Lifecycle: lifecycles, Project: values.Get("project"), Sprint: values.Get("sprint"), Study: values.Get("study"), Limit: 200, After: values.Get("after"),
	})
	if err != nil {
		h.renderError(w, r, http.StatusServiceUnavailable, "Runs unavailable", "The durable run repository could not be read.")
		return
	}
	nextURL := ""
	if page.NextCursor != "" {
		next := cloneURLValues(values)
		next.Set("after", page.NextCursor)
		nextURL = "/runs?" + next.Encode()
	}
	rows := make([]runRowView, 0, len(page.Runs))
	for _, snapshot := range page.Runs {
		rows = append(rows, newRunRowView(snapshot))
	}
	h.render(w, r, http.StatusOK, "runs", pageModel{
		Title: "Workspace runs", Heading: "Workspace runs", Runs: rows, NextRunsURL: nextURL,
		RunFilters: runPageFilters{Project: values.Get("project"), Sprint: values.Get("sprint"), Study: values.Get("study"), Lifecycle: values.Get("lifecycle")},
	})
}

func (h *handler) handleRunPage(w http.ResponseWriter, r *http.Request, value string) {
	if h.runs == nil {
		h.renderError(w, r, http.StatusServiceUnavailable, "Run unavailable", "Durable run observation is unavailable.")
		return
	}
	snapshot, err := h.runs.Run(r.Context(), app.RunID(value))
	if err != nil {
		status := http.StatusServiceUnavailable
		if errors.Is(err, app.ErrRunNotFound) {
			status = http.StatusNotFound
		}
		h.renderError(w, r, status, "Run unavailable", "The durable run is unavailable or no longer retained.")
		return
	}
	after := uint64(0)
	if snapshot.OldestRetainedSequence > 1 {
		after = snapshot.OldestRetainedSequence - 1
	}
	events, err := h.runs.RunEvents(r.Context(), snapshot.RunID, after, 200)
	if err != nil {
		h.renderError(w, r, http.StatusServiceUnavailable, "Run unavailable", "The retained event journal could not be read.")
		return
	}
	if len(events) > 200 {
		events = events[:200]
	}
	nextEventsURL := ""
	if len(events) > 0 && events[len(events)-1].Sequence < snapshot.LastSequence {
		nextEventsURL = "/api/v1/runs/" + url.PathEscape(value) + "/events?after=" + strconv.FormatUint(events[len(events)-1].Sequence, 10)
	}
	detail := newRunDetailView(snapshot)
	var insights *runStudyInsightsView
	if snapshot.Target.Study != "" && h.queries != nil {
		if study, err := h.queries.Study(r.Context(), snapshot.Target.Study); err == nil {
			insights = newRunStudyInsightsView(snapshot.Target.Study, study)
		}
	}
	var sprintUsage *runSprintUsageView
	if snapshot.Target.Sprint != "" && snapshot.Target.Project != "" && h.queries != nil {
		if usageQueries, ok := h.queries.(app.WebSprintUsageQueries); ok {
			if metrics, err := usageQueries.SprintRuntimeUsage(r.Context(), snapshot.Target.Project, snapshot.Target.Sprint); err == nil {
				summary := newRunSprintUsageView(snapshot.Target.Sprint, metrics)
				sprintUsage = &summary
			}
		}
	}
	var qaInsights *runQAInsightsView
	if isQARunTarget(snapshot.Target) {
		qaInsights = h.newRunQAInsightsView(r, snapshot)
	}
	eventViews := make([]runEventView, 0, len(events))
	for _, event := range events {
		eventViews = append(eventViews, newRunEventView(event))
	}
	h.render(w, r, http.StatusOK, "run", pageModel{Title: "Run " + value, Heading: "Run detail", Run: &detail, StudyInsights: insights, SprintUsage: sprintUsage, QAInsights: qaInsights, RunEvents: eventViews, NextEventsURL: nextEventsURL, Page: "run", SurfaceContract: runSurfaceContract})
}

func isQARunTarget(target app.RunTarget) bool {
	return target.Operation == string(app.OperationQAStart) || target.Operation == string(app.OperationQAResume)
}

func (h *handler) newRunQAInsightsView(r *http.Request, snapshot app.RunSnapshot) *runQAInsightsView {
	view := &runQAInsightsView{Project: snapshot.Target.Project, Sprint: snapshot.Target.Sprint}
	view.StatusURL = "/api/v1/projects/" + url.PathEscape(view.Project) + "/sprints/" + url.PathEscape(view.Sprint) + "/qa"
	view.SynthesisURL = view.StatusURL + "/synthesis"
	if h.qa == nil {
		view.Unavailable = "The canonical QA reader is unavailable. The durable event journal remains authoritative for this run."
		return view
	}
	qa, err := h.qa.QAStatus(r.Context(), app.QARequest{Project: view.Project, Sprint: view.Sprint})
	if err != nil {
		view.Unavailable = "The canonical QA snapshot could not be read. The durable event journal remains available below."
		return view
	}
	view.QA = qa
	view.ProgressMax = qa.TotalShards
	if view.ProgressMax < 1 {
		view.ProgressMax = 1
	}
	if qa.RunID != "" && qa.RunID != string(snapshot.RunID) {
		view.Historical = true
		view.CurrentRunID = qa.RunID
		return view
	}
	if qa.TotalShards > 0 {
		view.CompletionPercent = qa.CompletedShards * 100 / qa.TotalShards
	}
	keys := make([]string, 0, len(qa.OutcomeTotals))
	for key := range qa.OutcomeTotals {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		view.Outcomes = append(view.Outcomes, runQACountView{Label: key, Value: qa.OutcomeTotals[key]})
	}
	for _, shard := range qa.Shards {
		view.ApprovedChecks += len(shard.ApprovedChecks)
		view.Attempts += len(shard.Attempts)
		view.Theories += len(shard.Theories)
		for _, attempt := range shard.Attempts {
			view.Commands += len(attempt.Commands)
			view.ContextRequests += len(attempt.ContextRequests)
			view.Evidence += len(attempt.Evidence)
		}
		for _, theory := range shard.Theories {
			view.Evidence += len(theory.Evidence)
		}
	}
	synthesis, err := h.qa.QASynthesis(r.Context(), app.QARequest{Project: view.Project, Sprint: view.Sprint})
	if err != nil {
		view.SynthesisUnavailable = "The retained synthesis could not be read."
		return view
	}
	view.Synthesis = synthesis
	view.HasSynthesis = synthesis.ID != ""
	return view
}

func newRunStudyInsightsView(study string, result app.WebStudyResult) *runStudyInsightsView {
	retries := studyRetryDTO{RetriedTasks: result.Retries.RetriedTasks, TotalRetries: result.Retries.TotalRetries, SameSession: result.Retries.SameSession, FreshSession: result.Retries.FreshSession, Waiting: result.Retries.Waiting}
	if result.Retries.NextRetryAt != nil {
		next := *result.Retries.NextRetryAt
		retries.NextRetryAt = &next
	}
	tasks := make([]studyTaskPerfDTO, 0, len(result.Tasks))
	failures := make([]studyTaskFailureDTO, 0)
	seeds := make([]studyTaskSeedDTO, 0, len(result.Tasks))
	for _, task := range result.Tasks {
		tasks = append(tasks, studyTaskPerfDTO{ID: task.ID, Kind: task.Kind, Status: task.Status, Duration: task.Duration, Turns: task.Turns, Tokens: task.Tokens,
			Input: task.InputTokens, Output: task.OutputTokens, Reasoning: task.ReasoningTokens, CacheRead: task.CacheReadTokens, CacheWrite: task.CacheWriteTokens,
			Cost: task.Cost, CostSource: task.CostSource, Retries: task.Retries, SessionReuse: task.SessionReuse})
		if task.Error != "" {
			failures = append(failures, studyTaskFailureDTO{Task: task.ID, Code: task.ErrorCode, Message: task.Error})
		}
		if task.Status != "pending" && task.Status != "" {
			seed := studyTaskSeedDTO{Task: task.ID, Status: task.Status, Attempts: task.Attempts, Retries: task.Retries, Provider: task.Provider, Model: task.Model, Harness: task.Runtime, SessionID: task.SessionID, SessionReuse: task.SessionReuse}
			if task.RetryAfter != nil {
				seed.RetryAfter = task.RetryAfter.UTC().Format(time.RFC3339)
			}
			seeds = append(seeds, seed)
		}
	}
	return &runStudyInsightsView{Study: study, Status: result.Status, RunID: result.RunID, Total: result.Total, Completed: result.Completed,
		Pending: result.Pending, ActiveTasks: result.ActiveTasks, Failed: result.Failed, Cancelled: result.Cancelled,
		Retries: retries, Parallelism: mapStudyParallelism(result.Parallelism), Tasks: tasks, Failures: failures, SeedTasks: seeds, Usage: newRunUsageView("study tasks", result.Tasks)}
}

// finalizeRunUsageView derives display labels from accumulated totals.
func finalizeRunUsageView(view *runUsageView) {
	if view.cacheHitComplete && view.cacheDenominator > 0 {
		view.CacheHit = fmt.Sprintf("%.1f%%", float64(view.CacheRead)/float64(view.cacheDenominator)*100)
	} else {
		view.CacheHit = "-"
	}
	if view.costKnown {
		suffix := ""
		if view.ModelPriced > 0 {
			suffix = "*"
		}
		view.CostLabel = formatRunUSD(view.cost) + suffix
	} else {
		view.CostLabel = "-"
	}
}

// formatRunUSD renders a USD amount compactly for dashboards.
func formatRunUSD(amount float64) string {
	return "$" + strconv.FormatFloat(amount, 'f', -1, 64)
}

func newRunUsageView(scope string, tasks []app.RunTaskSummary) runUsageView {
	view := runUsageView{Scope: scope, cacheHitComplete: true, Tasks: append([]app.RunTaskSummary(nil), tasks...)}
	for _, task := range tasks {
		view.addTokens(task.InputKnown, task.InputTokens, task.OutputKnown, task.OutputTokens, task.ReasoningKnown, task.ReasoningTokens,
			task.CacheReadKnown, task.CacheReadTokens, task.CacheWriteKnown, task.CacheWriteTokens, task.TokensKnown, task.Tokens)
		view.addCost(task.CostKnown, task.CostAmount, task.CostSource)
	}
	finalizeRunUsageView(&view)
	return view
}

// runSprintUsageRow is one recent sprint stage run in the usage panel.
type runSprintUsageRow struct {
	Stage      string
	Task       string
	Model      string
	Status     string
	Tokens     int64
	CacheRead  int64
	CacheWrite int64
	Cost       string
}

// runSprintUsageView summarizes sprint runtime metrics for a durable run page.
type runSprintUsageView struct {
	runUsageView
	Rows       []runSprintUsageRow
	UpdatedAt  time.Time
	HasMetrics bool
}

func newRunSprintUsageView(slug string, metrics app.SprintMetricsSummary) runSprintUsageView {
	view := runSprintUsageView{runUsageView: runUsageView{Scope: slug, cacheHitComplete: true}}
	for _, run := range metrics.RecentRuns {
		view.addTokens(run.InputKnown, run.Input, run.OutputKnown, run.Output, run.ReasoningKnown, run.Reasoning,
			run.CacheReadKnown, run.CacheRead, run.CacheWriteKnown, run.CacheWrite, run.TotalKnown, run.Total)
		view.addCost(run.CostKnown, run.CostAmount, run.CostSource)
		cost := "-"
		if run.CostKnown {
			cost = formatRunUSD(run.CostAmount)
			if run.CostSource == "model_priced" {
				cost += "*"
			}
		}
		view.Rows = append(view.Rows, runSprintUsageRow{Stage: run.Stage, Task: run.Task, Model: run.Model, Status: run.Status,
			Tokens: run.Total, CacheRead: run.CacheRead, CacheWrite: run.CacheWrite, Cost: cost})
	}
	finalizeRunUsageView(&view.runUsageView)
	view.HasMetrics = len(metrics.RecentRuns) > 0
	return view
}

// sprintStageUsageRow is one row in the per-stage breakdown shown on the
// sprint flow overview. Each row aggregates every recent run recorded for
// that stage.
type sprintStageUsageRow struct {
	Stage    string
	Runs     int
	Tokens   string
	CacheR   string
	CacheW   string
	Cost     string
	CostNote string
}

// sprintStageUsageView summarizes sprint runtime metrics grouped by stage for
// the sprint flow page. It carries the same runUsageView totals as the run
// page so operators see one cost figure everywhere.
type sprintStageUsageView struct {
	runUsageView
	Rows       []sprintStageUsageRow
	UpdatedAt  time.Time
	HasMetrics bool
}

// sprintStageOrder returns the canonical planning-stage order used to lay out
// per-stage usage rows.
func sprintStageOrder() []string {
	return []string{
		"requirements", "code-context", "sprint-index", "technical-handbook",
		"area-reasoning", "reasoning", "plan", "execute", "review", "smoke",
	}
}

func newSprintStageUsageView(slug string, metrics app.SprintMetricsSummary) sprintStageUsageView {
	view := sprintStageUsageView{runUsageView: runUsageView{Scope: slug, cacheHitComplete: true}, UpdatedAt: metrics.UpdatedAt}
	type stageAgg struct {
		runs             int
		tokens           int64
		tokensKnown      bool
		cacheRead        int64
		cacheReadKnown   bool
		cacheWrite       int64
		cacheWriteKnown  bool
		cost             float64
		costKnown        bool
		providerReported int
		modelPriced      int
	}
	agg := map[string]*stageAgg{}
	for _, run := range metrics.RecentRuns {
		view.addTokens(run.InputKnown, run.Input, run.OutputKnown, run.Output, run.ReasoningKnown, run.Reasoning,
			run.CacheReadKnown, run.CacheRead, run.CacheWriteKnown, run.CacheWrite, run.TotalKnown, run.Total)
		view.addCost(run.CostKnown, run.CostAmount, run.CostSource)
		a, ok := agg[run.Stage]
		if !ok {
			a = &stageAgg{}
			agg[run.Stage] = a
		}
		a.runs++
		if run.TotalKnown {
			a.tokens += run.Total
			a.tokensKnown = true
		}
		if run.CacheReadKnown {
			a.cacheRead += run.CacheRead
			a.cacheReadKnown = true
		}
		if run.CacheWriteKnown {
			a.cacheWrite += run.CacheWrite
			a.cacheWriteKnown = true
		}
		if run.CostKnown {
			a.cost += run.CostAmount
			a.costKnown = true
			switch run.CostSource {
			case "model_priced":
				a.modelPriced++
			case "provider_reported":
				a.providerReported++
			}
		}
	}
	seen := map[string]bool{}
	emit := func(name string) {
		if seen[name] {
			return
		}
		seen[name] = true
		a, ok := agg[name]
		if !ok {
			return
		}
		tokens := "-"
		if a.tokensKnown {
			tokens = strconv.FormatInt(a.tokens, 10)
		}
		cacheR := "-"
		if a.cacheReadKnown {
			cacheR = strconv.FormatInt(a.cacheRead, 10)
		}
		cacheW := "-"
		if a.cacheWriteKnown {
			cacheW = strconv.FormatInt(a.cacheWrite, 10)
		}
		cost := "-"
		costNote := ""
		if a.costKnown {
			suffix := ""
			if a.modelPriced > 0 {
				suffix = "*"
			}
			cost = formatRunUSD(a.cost) + suffix
			if a.modelPriced > 0 && a.providerReported > 0 {
				costNote = "mixed"
			} else if a.modelPriced > 0 {
				costNote = "rate-table estimate"
			} else if a.providerReported > 0 {
				costNote = "provider reported"
			}
		}
		view.Rows = append(view.Rows, sprintStageUsageRow{Stage: name, Runs: a.runs, Tokens: tokens, CacheR: cacheR, CacheW: cacheW, Cost: cost, CostNote: costNote})
	}
	for _, name := range sprintStageOrder() {
		emit(name)
	}
	for name := range agg {
		emit(name)
	}
	finalizeRunUsageView(&view.runUsageView)
	view.HasMetrics = len(metrics.RecentRuns) > 0
	return view
}

// newStudyUsageView aggregates token and cost facts across the tasks of a
// study loop. It mirrors the runUsageView shown on the durable run page so
// the same cost figure appears across surfaces.
func newStudyUsageView(scope string, tasks []app.RunTaskSummary) runUsageView {
	return newRunUsageView(scope, tasks)
}

func newRunRowView(snapshot app.RunSnapshot) runRowView {
	return runRowView{
		RunID: snapshot.RunID, Target: runTargetLabel(snapshot.Target),
		Lifecycle: runLifecycleView(string(snapshot.Lifecycle)), Liveness: runLivenessView(string(snapshot.Liveness)),
		ProductStatus: firstRunViewValue(snapshot.ProductStatus, "unknown"), UpdatedAt: snapshot.UpdatedAt,
	}
}

func newRunDetailView(snapshot app.RunSnapshot) runDetailView {
	history := string(snapshot.RecordState)
	if !snapshot.HistoryComplete {
		history += " · incomplete before sequence " + strconv.FormatUint(snapshot.OldestRetainedSequence, 10)
	}
	terminal := ""
	terminalOutcome := ""
	terminalReason := ""
	if snapshot.Terminal != nil {
		terminalOutcome = string(snapshot.Terminal.Outcome)
		terminalReason = snapshot.Terminal.Reason
		terminal = terminalOutcome
		if terminalReason != "" {
			terminal += " · " + terminalReason
		}
	}
	started := snapshot.AcceptedAt
	if snapshot.StartedAt != nil {
		started = *snapshot.StartedAt
	}
	finished := snapshot.UpdatedAt
	if snapshot.FinishedAt != nil {
		finished = *snapshot.FinishedAt
	}
	return runDetailView{
		RunID: snapshot.RunID, LastSequence: snapshot.LastSequence, OldestRetainedSequence: snapshot.OldestRetainedSequence,
		OmissionTotal: snapshot.OmissionTotal, CurrentAttempt: firstRunViewValue(string(snapshot.CurrentAttemptID), "none"),
		Title: runTargetTitle(snapshot.Target), Scope: runTargetScope(snapshot.Target), Target: runTargetLabel(snapshot.Target),
		Lifecycle: runLifecycleView(string(snapshot.Lifecycle)), Liveness: runLivenessView(string(snapshot.Liveness)),
		Product:      firstRunViewValue(snapshot.ProductStatus, "unknown"),
		Cancellation: runCancellationView(string(snapshot.Cancellation.State)), History: history, Terminal: terminal,
		TerminalOutcome: terminalOutcome, TerminalReason: terminalReason, Duration: formatRunDuration(finished.Sub(started)),
		IsActive: snapshot.Lifecycle.IsActive(),
	}
}

func newRunEventView(event app.RunEvent) runEventView {
	omission := ""
	if event.Omission != nil {
		omission = fmt.Sprintf("Omitted %d detail item(s): %s", event.Omission.Count, event.Omission.Reason)
	}
	// Prefer richest observable text: text/delta/detail/message/content
	text := firstNonEmptyPayload(event.Payload, "text", "delta", "detail", "message", "content", "title", "output")
	if len(text) > 160 {
		text = text[:160] + "…"
	}
	return runEventView{Sequence: event.Sequence, AttemptID: string(event.AttemptID), Type: string(event.Type), Stage: event.Stage, Task: event.Task,
		Time: committedRunEventTime(event), DetailKind: event.Payload["kind"], DetailType: event.Payload["type"], DetailTool: firstNonEmptyPayload(event.Payload, "tool_name", "tool"), DetailText: text,
		DetailState: event.Payload["state"], DetailAction: event.Payload["action"], DetailReason: event.Payload["reason"], DetailCount: event.Payload["count"],
		PhaseState: event.Payload["phase_state"], Summary: event.Payload["summary"], Action: event.Payload["action"], Reason: event.Payload["reason"],
		ToolCallID: event.Payload["tool_call_id"], ToolStatus: event.Payload["tool_status"],
		ToolArguments: prettyObservableJSON(event.Payload["tool_arguments"]), ToolResult: prettyObservableJSON(event.Payload["tool_result"]), ToolError: prettyObservableJSON(event.Payload["tool_error"]), Omission: omission}
}

func prettyObservableJSON(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	var decoded any
	if json.Unmarshal([]byte(value), &decoded) != nil {
		return value
	}
	pretty, err := json.MarshalIndent(decoded, "", "  ")
	if err != nil {
		return value
	}
	return string(pretty)
}

func firstNonEmptyPayload(payload map[string]string, keys ...string) string {
	for _, k := range keys {
		if v := strings.TrimSpace(payload[k]); v != "" {
			return v
		}
	}
	return ""
}

func committedRunEventTime(event app.RunEvent) string {
	if event.CommittedAt.IsZero() {
		return ""
	}
	return event.CommittedAt.UTC().Format(time.RFC3339)
}

func runLifecycleView(value string) runStateView {
	cue := "Unknown"
	tone := "muted"
	switch value {
	case "accepted", "queued", "running", "cancelling":
		cue = "Active"
		tone = "info"
	case "succeeded":
		cue = "Complete"
		tone = "ok"
	case "failed", "cancelled", "timed_out", "interrupted", "cleanup_uncertain", "persistence_degraded":
		cue = "Attention"
		tone = "error"
	}
	return runStateView{Value: firstRunViewValue(value, "unknown"), Cue: cue, Tone: tone}
}

func runLivenessView(value string) runStateView {
	cue := "Unknown"
	tone := "muted"
	switch value {
	case "live":
		cue = "Live"
		tone = "info"
	case "terminal":
		cue = "Stopped"
	case "stalled", "owner_unreachable", "interrupted", "cleanup_uncertain":
		cue = "Attention"
		tone = "error"
	}
	return runStateView{Value: firstRunViewValue(value, "unknown"), Cue: cue, Tone: tone}
}

func runCancellationView(value string) runStateView {
	cue := "No request"
	tone := "muted"
	switch value {
	case "requested":
		cue = "Requested"
		tone = "warn"
	case "acknowledged":
		cue = "Acknowledged"
		tone = "warn"
	case "uncertain":
		cue = "Uncertain"
		tone = "error"
	}
	return runStateView{Value: firstRunViewValue(value, "unknown"), Cue: cue, Tone: tone}
}

func runTargetTitle(target app.RunTarget) string {
	label := target.Stage
	if label == "" {
		label = target.Operation
	}
	label = humanizeRunLabel(label)
	if label == "" {
		return "Agent run"
	}
	return label + " run"
}

func runTargetScope(target app.RunTarget) string {
	if target.Study != "" {
		return "Study " + target.Study
	}
	parts := make([]string, 0, 2)
	if target.Project != "" {
		parts = append(parts, target.Project)
	}
	if target.Sprint != "" {
		parts = append(parts, "Sprint "+target.Sprint)
	}
	return strings.Join(parts, " · ")
}

func humanizeRunLabel(value string) string {
	value = strings.TrimSpace(strings.NewReplacer("_", " ", "-", " ").Replace(value))
	if value == "" {
		return ""
	}
	return strings.ToUpper(value[:1]) + value[1:]
}

func formatRunDuration(duration time.Duration) string {
	if duration < 0 {
		return "unknown"
	}
	if duration < time.Second {
		return "<1s"
	}
	return duration.Round(time.Second).String()
}

func runTargetLabel(target app.RunTarget) string {
	values := []string{target.Kind, target.Operation, target.Project, target.Sprint, target.Study, target.Stage, target.Task}
	parts := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" {
			parts = append(parts, value)
		}
	}
	return strings.Join(parts, " / ")
}

func firstRunViewValue(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func (h *handler) handleRunPageCancel(w http.ResponseWriter, r *http.Request, value string) {
	if err := r.ParseForm(); err != nil || r.FormValue("_csrf") != csrfToken(r.Context()) {
		h.renderError(w, r, http.StatusForbidden, "Request rejected", "The browser session or CSRF proof is invalid. Refresh the run page and try again.")
		return
	}
	if h.runs == nil {
		h.renderError(w, r, http.StatusServiceUnavailable, "Run unavailable", "Durable run cancellation is unavailable.")
		return
	}
	if _, _, err := h.runs.CancelRun(r.Context(), app.RunID(value), "user_requested"); err != nil {
		h.renderError(w, r, http.StatusConflict, "Cancellation not accepted", "The durable cancellation request could not be accepted.")
		return
	}
	http.Redirect(w, r, "/runs/"+value, http.StatusSeeOther)
}

func (h *handler) handleRunShow(w http.ResponseWriter, r *http.Request, value string) {
	if h.runs == nil {
		h.writeError(w, r, http.StatusServiceUnavailable, "run_control_unavailable", "Durable run observation is unavailable.")
		return
	}
	snapshot, err := h.runs.Run(r.Context(), app.RunID(value))
	if err != nil {
		h.handleRunControlError(w, r, err)
		return
	}
	h.writeSuccess(w, r, http.StatusOK, snapshot, nil)
}

func (h *handler) handleRunCancel(w http.ResponseWriter, r *http.Request, value string) {
	if h.runs == nil {
		h.writeError(w, r, http.StatusServiceUnavailable, "run_control_unavailable", "Durable run cancellation is unavailable.")
		return
	}
	snapshot, changed, err := h.runs.CancelRun(r.Context(), app.RunID(value), "user_requested")
	if err != nil {
		h.handleRunControlError(w, r, err)
		return
	}
	h.writeSuccess(w, r, http.StatusOK, map[string]any{"changed": changed, "run": snapshot}, nil)
}

func (h *handler) handleRunEvents(w http.ResponseWriter, r *http.Request, value string) {
	if h.runs == nil {
		h.writeError(w, r, http.StatusServiceUnavailable, "run_control_unavailable", "Durable run replay is unavailable.")
		return
	}
	values := r.URL.Query()
	if !onlyQueryKeys(values, "after") {
		h.writeError(w, r, http.StatusBadRequest, "invalid_request", "Unknown query parameters are not accepted.")
		return
	}
	afterValue, queryHasAfter := values["after"]
	headerAfter := strings.TrimSpace(r.Header.Get("Last-Event-ID"))
	if queryHasAfter && headerAfter != "" {
		h.writeError(w, r, http.StatusBadRequest, "cursor_conflict", "Use either Last-Event-ID or after, not both.")
		return
	}
	cursor := "0"
	if queryHasAfter {
		if len(afterValue) != 1 {
			h.writeError(w, r, http.StatusBadRequest, "invalid_cursor", "The event cursor must be one decimal sequence.")
			return
		}
		cursor = afterValue[0]
	} else if headerAfter != "" {
		cursor = headerAfter
	}
	after, err := strconv.ParseUint(cursor, 10, 64)
	if err != nil {
		h.writeError(w, r, http.StatusBadRequest, "invalid_cursor", "The event cursor must be a non-negative decimal sequence.")
		return
	}
	runID := app.RunID(value)
	snapshot, err := h.runs.Run(r.Context(), runID)
	if err != nil {
		h.handleRunControlError(w, r, err)
		return
	}
	if after > snapshot.LastSequence {
		h.writeErrorDetails(w, r, http.StatusConflict, errorBody{Code: "cursor_ahead", Message: "The event cursor is ahead of the durable run.", Details: map[string]any{
			"requested": after, "last": snapshot.LastSequence, "run": snapshot,
		}})
		return
	}
	if after+1 < snapshot.OldestRetainedSequence {
		h.writeErrorDetails(w, r, http.StatusConflict, errorBody{Code: "replay_gap", Message: "The requested event history is no longer retained.", Details: map[string]any{
			"requested": after, "oldest": snapshot.OldestRetainedSequence, "last": snapshot.LastSequence,
			"reason": "retention_or_compaction", "run": snapshot, "recovery": []string{"refresh_snapshot", "resume_from_oldest"},
		}})
		return
	}
	if !strings.Contains(r.Header.Get("Accept"), "text/event-stream") {
		events, err := h.runs.RunEvents(r.Context(), runID, after, 512)
		if err != nil {
			h.handleRunControlError(w, r, err)
			return
		}
		h.writeSuccess(w, r, http.StatusOK, map[string]any{"run": snapshot, "events": events}, nil)
		return
	}
	h.followRunSSE(w, r, snapshot, after)
}

func (h *handler) followRunSSE(w http.ResponseWriter, r *http.Request, snapshot app.RunSnapshot, after uint64) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		h.writeError(w, r, http.StatusInternalServerError, "stream_unavailable", "Streaming is unavailable.")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()
	idle := time.NewTimer(0)
	if !idle.Stop() {
		<-idle.C
	}
	defer idle.Stop()
	for {
		events, err := h.runs.RunEvents(r.Context(), snapshot.RunID, after, 512)
		if err != nil {
			return
		}
		for _, event := range events {
			encoded, err := json.Marshal(event)
			if err != nil {
				return
			}
			if _, err := fmt.Fprintf(w, "id: %d\nevent: run\ndata: %s\n\n", event.Sequence, encoded); err != nil {
				return
			}
			after = event.Sequence
		}
		flusher.Flush()
		current, err := h.runs.Run(r.Context(), snapshot.RunID)
		if err != nil {
			return
		}
		if current.Lifecycle.IsTerminal() && after >= current.LastSequence {
			return
		}
		wait := time.Second
		if len(events) == 512 || after < current.LastSequence {
			wait = 250 * time.Millisecond
		}
		idle.Reset(wait)
		select {
		case <-r.Context().Done():
			return
		case <-idle.C:
			if _, err := fmt.Fprint(w, ": heartbeat\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func (h *handler) handleRunControlError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, app.ErrRunInvalidArgument):
		h.writeError(w, r, http.StatusBadRequest, "invalid_run_request", "The durable run request is invalid.")
	case errors.Is(err, app.ErrRunNotFound):
		h.writeError(w, r, http.StatusNotFound, "run_not_found", "The durable run is not retained.")
	case errors.Is(err, app.ErrRunConflict):
		h.writeError(w, r, http.StatusConflict, "run_conflict", "The durable run changed concurrently.")
	case errors.Is(err, app.ErrRunUnsupportedSchema):
		h.writeError(w, r, http.StatusServiceUnavailable, "unsupported_run_schema", "The run-control schema requires a matching UltraPlan binary.")
	default:
		h.writeError(w, r, http.StatusServiceUnavailable, "run_control_unavailable", "The durable run repository is unavailable.")
	}
}

func onlyQueryKeys(values map[string][]string, allowed ...string) bool {
	want := make(map[string]bool, len(allowed))
	for _, key := range allowed {
		want[key] = true
	}
	for key, entries := range values {
		if !want[key] || len(entries) != 1 {
			return false
		}
	}
	return true
}

func cloneURLValues(values url.Values) url.Values {
	result := make(url.Values, len(values))
	for key, entries := range values {
		result[key] = append([]string(nil), entries...)
	}
	return result
}

func webLifecycleFilter(value string) ([]app.RunLifecycle, error) {
	if value == "" {
		return nil, nil
	}
	var result []app.RunLifecycle
	for _, item := range strings.Split(value, ",") {
		state := app.RunLifecycle(strings.TrimSpace(item))
		if !state.IsValid() {
			return nil, errors.New("invalid lifecycle")
		}
		result = append(result, state)
	}
	return result, nil
}
