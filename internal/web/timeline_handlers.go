package web

import (
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/Antonio7098/ultraplan-go/internal/app"
)

// The run-history timeline is an additive, read-only observability surface. It
// projects durable runs for one sprint or study onto a shared time axis and
// carries only sanitized facts owned by run control: lifecycle, bounds, and
// committed tool-event timestamps. Raw payloads never leave the repository.
const (
	timelineMaxRuns         = 50
	timelineMaxEventsPerRun = 4096
	timelineMaxToolSamples  = 1200
)

type timelineRunDTO struct {
	RunID             string   `json:"run_id"`
	Target            string   `json:"target,omitempty"`
	Lifecycle         string   `json:"lifecycle"`
	Active            bool     `json:"active"`
	StartedAt         string   `json:"started_at"`
	EndedAt           string   `json:"ended_at,omitempty"`
	ToolEvents        []string `json:"tool_events"`
	ToolEventsSampled bool     `json:"tool_events_sampled"`
}

type timelineDTO struct {
	Sprint      string           `json:"sprint,omitempty"`
	Study       string           `json:"study,omitempty"`
	Window      string           `json:"window"`
	WindowStart string           `json:"window_start"`
	WindowEnd   string           `json:"window_end"`
	Runs        []timelineRunDTO `json:"runs"`
}

var timelineWindows = map[string]time.Duration{
	"6h":  6 * time.Hour,
	"24h": 24 * time.Hour,
	"7d":  7 * 24 * time.Hour,
	"30d": 30 * 24 * time.Hour,
}

func (h *handler) handleTimeline(w http.ResponseWriter, r *http.Request) {
	if h.runs == nil {
		h.writeError(w, r, http.StatusServiceUnavailable, "run_control_unavailable", "Durable run observation is unavailable.")
		return
	}
	values := r.URL.Query()
	if !onlyQueryKeys(values, "sprint", "study", "window", "limit") {
		h.writeError(w, r, http.StatusBadRequest, "invalid_request", "Unknown query parameters are not accepted.")
		return
	}
	sprint, study := values.Get("sprint"), values.Get("study")
	if (sprint == "") == (study == "") {
		h.writeError(w, r, http.StatusBadRequest, "invalid_scope", "Provide exactly one timeline scope: sprint or study.")
		return
	}
	windowName := values.Get("window")
	if windowName == "" {
		windowName = "24h"
	}
	window, ok := timelineWindows[windowName]
	if !ok {
		h.writeError(w, r, http.StatusBadRequest, "invalid_window", "The timeline window must be one of 6h, 24h, 7d, or 30d.")
		return
	}
	limit := 20
	if text := values.Get("limit"); text != "" {
		parsed, err := strconv.Atoi(text)
		if err != nil || parsed < 1 || parsed > timelineMaxRuns {
			h.writeError(w, r, http.StatusBadRequest, "invalid_limit", "The run limit must be between 1 and 50.")
			return
		}
		limit = parsed
	}
	page, err := h.runs.Runs(r.Context(), app.RunQuery{Sprint: sprint, Study: study, Limit: limit})
	if err != nil {
		h.handleRunControlError(w, r, err)
		return
	}
	now := h.now().UTC()
	cutoff := now.Add(-window)
	result := timelineDTO{Sprint: sprint, Study: study, Window: windowName, WindowStart: cutoff.Format(time.RFC3339), WindowEnd: now.Format(time.RFC3339), Runs: make([]timelineRunDTO, 0, len(page.Runs))}
	for _, snapshot := range page.Runs {
		if snapshot.UpdatedAt.Before(cutoff) {
			continue
		}
		result.Runs = append(result.Runs, h.timelineRun(r, snapshot))
	}
	h.writeSuccess(w, r, http.StatusOK, result, nil)
}

func (h *handler) timelineRun(r *http.Request, snapshot app.RunSnapshot) timelineRunDTO {
	started := snapshot.AcceptedAt
	if snapshot.StartedAt != nil && snapshot.StartedAt.After(started) {
		started = *snapshot.StartedAt
	}
	ended := snapshot.UpdatedAt
	if snapshot.FinishedAt != nil {
		ended = *snapshot.FinishedAt
	}
	events, err := h.runs.RunEvents(r.Context(), snapshot.RunID, 0, timelineMaxEventsPerRun)
	if err != nil {
		events = nil
	}
	var stamps []time.Time
	for _, event := range events {
		if event.Payload["tool"] == "" {
			continue
		}
		stamps = append(stamps, event.CommittedAt)
	}
	sampled := false
	if len(stamps) > timelineMaxToolSamples {
		sort.Slice(stamps, func(i, j int) bool { return stamps[i].Before(stamps[j]) })
		thinned := make([]time.Time, 0, timelineMaxToolSamples)
		step := float64(len(stamps)) / float64(timelineMaxToolSamples)
		for index := range timelineMaxToolSamples {
			thinned = append(thinned, stamps[int(float64(index)*step)])
		}
		stamps = thinned
		sampled = true
	}
	formatted := make([]string, 0, len(stamps))
	for _, stamp := range stamps {
		formatted = append(formatted, stamp.UTC().Format(time.RFC3339Nano))
	}
	return timelineRunDTO{
		RunID: string(snapshot.RunID), Target: runTargetLabel(snapshot.Target), Lifecycle: string(snapshot.Lifecycle),
		Active: snapshot.Lifecycle.IsActive(), StartedAt: started.UTC().Format(time.RFC3339),
		EndedAt: ended.UTC().Format(time.RFC3339), ToolEvents: formatted, ToolEventsSampled: sampled,
	}
}
