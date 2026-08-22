package web

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Antonio7098/ultraplan-go/internal/app"
)

var appRunTargetStudy = app.RunTarget{Kind: "study", Operation: string(app.OperationStudyStart), Study: "research"}

func appTimelineEvent(at time.Time) app.RunEvent {
	return app.RunEvent{RunID: testRunID, Sequence: 3, CommittedAt: at, Type: "progress", Payload: map[string]string{"tool": "edit_file"}}
}

func TestTimelineEndpointProjectsScopedRunHistory(t *testing.T) {
	runs := newFakeRunUseCases()
	h, err := NewHandler(HandlerOptions{Queries: sampleQueries(), Runs: runs, Authority: testAuthority,
		Now: func() time.Time { return time.Date(2026, 8, 21, 13, 0, 0, 0, time.UTC) }})
	if err != nil {
		t.Fatal(err)
	}
	res := request(h, http.MethodGet, "/api/v1/timeline?sprint=30-web&window=7d&limit=10", nil)
	body := res.Body.String()
	if res.Code != http.StatusOK || !strings.Contains(res.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("status=%d headers=%v body=%s", res.Code, res.Header(), body)
	}
	for _, want := range []string{`"window":"7d"`, `"run_id":"` + string(testRunID) + `"`, `"lifecycle":"succeeded"`, `"started_at":"2026-08-21T12:00:00Z"`, `"ended_at":"2026-08-21T12:00:01Z"`, `"tool_events":`} {
		if !strings.Contains(body, want) {
			t.Errorf("timeline body missing %q", want)
		}
	}
	if runs.query.Sprint != "30-web" || runs.query.Limit != 10 || runs.query.Study != "" {
		t.Fatalf("scope did not reach the canonical query: %+v", runs.query)
	}

	studyRuns := newFakeRunUseCases()
	studyRuns.snapshot.Target = appRunTargetStudy
	studyHandler, err := NewHandler(HandlerOptions{Queries: sampleQueries(), Runs: studyRuns, Authority: testAuthority,
		Now: func() time.Time { return time.Date(2026, 8, 21, 13, 0, 0, 0, time.UTC) }})
	if err != nil {
		t.Fatal(err)
	}
	studyRes := request(studyHandler, http.MethodGet, "/api/v1/timeline?study=research&limit=5", nil)
	studyBody := studyRes.Body.String()
	if studyRes.Code != http.StatusOK || !strings.Contains(studyBody, `"study":"research"`) {
		t.Fatalf("study timeline status=%d body=%s", studyRes.Code, studyBody)
	}
	if studyRuns.query.Study != "research" {
		t.Fatalf("study scope missing from query: %+v", studyRuns.query)
	}
}

func TestTimelineEndpointRejectsInvalidScopesAndBounds(t *testing.T) {
	runs := newFakeRunUseCases()
	h, err := NewHandler(HandlerOptions{Queries: sampleQueries(), Runs: runs, Authority: testAuthority})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ query, code string }{
		{"", "invalid_scope"},
		{"sprint=30-web&study=research", "invalid_scope"},
		{"sprint=30-web&window=90d", "invalid_window"},
		{"sprint=30-web&limit=0", "invalid_limit"},
		{"sprint=30-web&limit=51", "invalid_limit"},
		{"sprint=30-web&project=alpha", "invalid_request"},
	} {
		target := "/api/v1/timeline"
		if tc.query != "" {
			target += "?" + tc.query
		}
		res := request(h, http.MethodGet, target, nil)
		if res.Code != http.StatusBadRequest || !strings.Contains(res.Body.String(), `"code":"`+tc.code+`"`) {
			t.Errorf("%s status=%d body=%s", target, res.Code, res.Body.String())
		}
	}
}

func TestTimelineToolEventsCarryCommittedTimestamps(t *testing.T) {
	runs := newFakeRunUseCases()
	committed := time.Date(2026, 8, 21, 12, 0, 30, 0, time.UTC)
	runs.events = append(runs.events, appTimelineEvent(committed))
	h, err := NewHandler(HandlerOptions{Queries: sampleQueries(), Runs: runs, Authority: testAuthority,
		Now: func() time.Time { return committed.Add(time.Hour) }})
	if err != nil {
		t.Fatal(err)
	}
	body := request(h, http.MethodGet, "/api/v1/timeline?sprint=30-web", nil).Body.String()
	if !strings.Contains(body, committed.UTC().Format(time.RFC3339Nano)) {
		t.Fatalf("tool event timestamp missing from %s", body)
	}
}

func TestBrowserSprintAndStudyPagesIncludeRunHistoryTimeline(t *testing.T) {
	h, err := NewHandler(HandlerOptions{Queries: sampleQueries(), Runs: newFakeRunUseCases(), Authority: testAuthority})
	if err != nil {
		t.Fatal(err)
	}
	sprintBody := request(h, http.MethodGet, "/projects/alpha/sprints/30-web/run", nil).Body.String()
	for _, want := range []string{`data-run-timeline data-timeline-scope="sprint"`, `data-timeline-sprint="30-web"`, `/static/run-timeline.css`, `/static/run-timeline.js`, `data-timeline-window`, `data-timeline-limit`, `spikes show tool activity`} {
		if !strings.Contains(sprintBody, want) {
			t.Errorf("sprint run page missing %q", want)
		}
	}
	studyBody := request(h, http.MethodGet, "/studies/research/progress", nil).Body.String()
	for _, want := range []string{`data-run-timeline data-timeline-scope="study"`, `data-timeline-study="research"`, `/static/run-timeline.js`} {
		if !strings.Contains(studyBody, want) {
			t.Errorf("study progress page missing %q", want)
		}
	}
	js := request(h, http.MethodGet, "/static/run-timeline.js", nil).Body.String()
	for _, want := range []string{"/api/v1/timeline?", "polyline", "data-timeline-window", "toLocaleTimeString"} {
		if !strings.Contains(js, want) {
			t.Errorf("run-timeline.js missing %q", want)
		}
	}
	cssStatus := request(h, http.MethodGet, "/static/run-timeline.css", nil).Code
	if cssStatus != http.StatusOK {
		t.Fatalf("run-timeline.css status=%d", cssStatus)
	}
}
