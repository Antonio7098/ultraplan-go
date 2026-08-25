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
	Target                 string
	Lifecycle              runStateView
	Liveness               runStateView
	Product                string
	Cancellation           runStateView
	History                string
	Terminal               string
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
	Cost         string `json:"cost,omitempty"`
	Retries      int    `json:"retries,omitempty"`
	SessionReuse string `json:"session_reuse,omitempty"`
}

type runEventView struct {
	Sequence     uint64
	Type         string
	Stage        string
	Task         string
	Time         string
	DetailKind   string
	DetailType   string
	DetailTool   string
	DetailState  string
	DetailAction string
	DetailReason string
	DetailCount  string
	DetailText   string
	Omission     string
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
	var qaInsights *runQAInsightsView
	if isQARunTarget(snapshot.Target) {
		qaInsights = h.newRunQAInsightsView(r, snapshot)
	}
	eventViews := make([]runEventView, 0, len(events))
	for _, event := range events {
		eventViews = append(eventViews, newRunEventView(event))
	}
	h.render(w, r, http.StatusOK, "run", pageModel{Title: "Run " + value, Heading: "Run detail", Run: &detail, StudyInsights: insights, QAInsights: qaInsights, RunEvents: eventViews, NextEventsURL: nextEventsURL, Page: "run", SurfaceContract: runSurfaceContract})
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
		tasks = append(tasks, studyTaskPerfDTO{ID: task.ID, Kind: task.Kind, Status: task.Status, Duration: task.Duration, Turns: task.Turns, Tokens: task.Tokens, Cost: task.Cost, Retries: task.Retries, SessionReuse: task.SessionReuse})
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
		Retries: retries, Parallelism: mapStudyParallelism(result.Parallelism), Tasks: tasks, Failures: failures, SeedTasks: seeds}
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
	if snapshot.Terminal != nil {
		terminal = string(snapshot.Terminal.Outcome) + " · " + snapshot.Terminal.Reason
	}
	return runDetailView{
		RunID: snapshot.RunID, LastSequence: snapshot.LastSequence, OldestRetainedSequence: snapshot.OldestRetainedSequence,
		OmissionTotal: snapshot.OmissionTotal, CurrentAttempt: firstRunViewValue(string(snapshot.CurrentAttemptID), "none"), Target: runTargetLabel(snapshot.Target),
		Lifecycle: runLifecycleView(string(snapshot.Lifecycle)), Liveness: runLivenessView(string(snapshot.Liveness)),
		Product:      firstRunViewValue(snapshot.ProductStatus, "unknown"),
		Cancellation: runCancellationView(string(snapshot.Cancellation.State)), History: history, Terminal: terminal,
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
	return runEventView{Sequence: event.Sequence, Type: string(event.Type), Stage: event.Stage, Task: event.Task,
		Time: committedRunEventTime(event), DetailKind: event.Payload["kind"], DetailType: event.Payload["type"], DetailTool: event.Payload["tool"], DetailState: event.Payload["state"], DetailAction: event.Payload["action"], DetailReason: event.Payload["reason"], DetailCount: event.Payload["count"], DetailText: text, Omission: omission}
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
	switch value {
	case "accepted", "queued", "running", "cancelling":
		cue = "Active"
	case "succeeded":
		cue = "Complete"
	case "failed", "cancelled", "timed_out", "interrupted", "cleanup_uncertain", "persistence_degraded":
		cue = "Attention"
	}
	return runStateView{Value: firstRunViewValue(value, "unknown"), Cue: cue}
}

func runLivenessView(value string) runStateView {
	cue := "Unknown"
	switch value {
	case "live":
		cue = "Live"
	case "terminal":
		cue = "Stopped"
	case "stalled", "owner_unreachable", "interrupted", "cleanup_uncertain":
		cue = "Attention"
	}
	return runStateView{Value: firstRunViewValue(value, "unknown"), Cue: cue}
}

func runCancellationView(value string) runStateView {
	cue := "No request"
	switch value {
	case "requested":
		cue = "Requested"
	case "acknowledged":
		cue = "Acknowledged"
	case "uncertain":
		cue = "Uncertain"
	}
	return runStateView{Value: firstRunViewValue(value, "unknown"), Cue: cue}
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
