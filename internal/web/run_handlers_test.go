package web

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Antonio7098/ultraplan-go/internal/app"
	"github.com/Antonio7098/ultraplan-go/internal/runcontrol"
)

const testRunID app.RunID = "run_aaaaaaaaaaaaaaaaaaaaaaaaaa"

type fakeRunUseCases struct {
	snapshot  app.RunSnapshot
	events    []app.RunEvent
	cancelled int
	query     app.RunQuery
	next      string
}

type sqliteRunUseCases struct{ repository runcontrol.Repository }

func (u sqliteRunUseCases) Runs(ctx context.Context, query app.RunQuery) (app.RunPage, error) {
	return u.repository.List(ctx, query)
}
func (u sqliteRunUseCases) Run(ctx context.Context, id app.RunID) (app.RunSnapshot, error) {
	return u.repository.Snapshot(ctx, id)
}
func (u sqliteRunUseCases) RunEvents(ctx context.Context, id app.RunID, after uint64, limit int) ([]app.RunEvent, error) {
	return u.repository.Events(ctx, id, after, limit)
}
func (u sqliteRunUseCases) CancelRun(ctx context.Context, id app.RunID, reason string) (app.RunSnapshot, bool, error) {
	return u.repository.RequestCancellation(ctx, id, reason)
}
func (u sqliteRunUseCases) RunHealth(ctx context.Context) (app.RunHealthResult, error) {
	return u.repository.Health(ctx)
}

func newFakeRunUseCases() *fakeRunUseCases {
	accepted := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	finished := accepted.Add(time.Second)
	return &fakeRunUseCases{
		snapshot: app.RunSnapshot{
			RunID:     testRunID,
			Target:    app.RunTarget{Kind: "sprint", Operation: "execute", Project: "alpha", Sprint: "35"},
			Lifecycle: "succeeded", Liveness: "terminal", RecordState: "full",
			AcceptedAt: accepted, UpdatedAt: finished, FinishedAt: &finished,
			LastSequence: 2, OldestRetainedSequence: 1, HistoryComplete: true,
			Cancellation: app.RunCancellation{State: "none"},
		},
		events: []app.RunEvent{
			{RunID: testRunID, Sequence: 1, CommittedAt: accepted, Type: "progress", Payload: map[string]string{"status": "running"}},
			{RunID: testRunID, Sequence: 2, CommittedAt: finished, Type: "terminal", Payload: map[string]string{"outcome": "succeeded"}},
		},
	}
}

func (f *fakeRunUseCases) Runs(_ context.Context, query app.RunQuery) (app.RunPage, error) {
	f.query = query
	return app.RunPage{Runs: []app.RunSnapshot{f.snapshot}, NextCursor: f.next}, nil
}

func TestBrowserRunPagesPreserveFiltersAndBoundAccessibleEnhancement(t *testing.T) {
	runs := newFakeRunUseCases()
	runs.next = "opaque-next"
	runs.snapshot.Lifecycle = "running"
	runs.snapshot.Liveness = "stalled"
	runs.snapshot.Terminal = nil
	runs.snapshot.FinishedAt = nil
	runs.snapshot.CurrentAttemptID = "att_aaaaaaaaaaaaaaaaaaaaaaaaaa"
	runs.snapshot.LastSequence = 700
	runs.snapshot.OmissionTotal = 11
	runs.events = make([]app.RunEvent, 700)
	for index := range runs.events {
		runs.events[index] = app.RunEvent{RunID: testRunID, Sequence: uint64(index + 1), Type: "progress", Stage: "execute"}
	}
	h, err := NewHandler(HandlerOptions{Queries: sampleQueries(), Runs: runs, Authority: testAuthority})
	if err != nil {
		t.Fatal(err)
	}

	list := request(h, http.MethodGet, "/runs?project=alpha&sprint=35&study=research&lifecycle=running", nil)
	if list.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", list.Code, list.Body.String())
	}
	for _, want := range []string{`value="alpha"`, `value="35"`, `value="research"`, `value="running" selected`, `after=opaque-next`, `project=alpha`, `data-label="Lifecycle"`, `Attention: stalled`} {
		if !strings.Contains(list.Body.String(), want) {
			t.Errorf("filtered list missing %q", want)
		}
	}
	if runs.query.Project != "alpha" || runs.query.Sprint != "35" || runs.query.Study != "research" || len(runs.query.Lifecycle) != 1 || runs.query.Lifecycle[0] != "running" {
		t.Fatalf("URL filters did not reach the canonical query: %+v", runs.query)
	}

	detail := request(h, http.MethodGet, "/runs/"+string(testRunID), nil)
	body := detail.Body.String()
	if detail.Code != http.StatusOK {
		t.Fatalf("detail status=%d body=%s", detail.Code, body)
	}
	for _, want := range []string{`role="status" aria-live="polite"`, `data-confirm="Request durable cancellation`, `Owner attempt`, `Runtime attempts`, `data-run-journey`, `data-run-phase="checking"`, `Replay boundary`, `oldest 1, last 700, omitted 11`, `Continue retained event replay`, `data-run-agents`,
		`id="run-agent-grid"`,
		`data-run-slots`,
		`data-run-tab="history"`,
		`data-run-tab="planned"`,
		`id="run-agent-history"`,
		`id="run-agent-planned"`,
		`id="run-agent-dialog"`, `data-run-type="progress" data-run-stage="execute" data-run-task=""`} {
		if !strings.Contains(body, want) {
			t.Errorf("detail missing %q", want)
		}
	}
	if count := strings.Count(body, `data-run-sequence="`); count != 200 {
		t.Fatalf("initial enhanced DOM rows=%d, want 200", count)
	}

	js := request(h, http.MethodGet, "/static/app.js", nil).Body.String()
	for _, want := range []string{`form[data-confirm]`, `window.confirm`, `event.submitter?.focus()`, `250 - (Date.now()`, `durableTimeline.children.length > 500`, `sequence <= durableLast`, `document.hidden`, `.textContent =`, `ingestRunEvent(event)`, `ingestRunJourneyEvent(event)`, `selectRunPhase`, `"ArrowRight"`, `[data-run-agents]`, `openRunAgent(trigger.dataset.runAgent)`, `agentDialog.showModal()`, `${agent.toolCalls} tool call`} {
		if !strings.Contains(js, want) {
			t.Errorf("run enhancement missing %q", want)
		}
	}
	css := request(h, http.MethodGet, "/static/app.css", nil).Body.String()
	for _, want := range []string{`@media (max-width: 45rem)`, `data-label`, `overflow-wrap: anywhere`, `prefers-reduced-motion: reduce`} {
		if !strings.Contains(css, want) {
			t.Errorf("responsive run CSS missing %q", want)
		}
	}
}
func (f *fakeRunUseCases) Run(context.Context, app.RunID) (app.RunSnapshot, error) {
	return f.snapshot, nil
}
func (f *fakeRunUseCases) RunEvents(_ context.Context, _ app.RunID, after uint64, _ int) ([]app.RunEvent, error) {
	var result []app.RunEvent
	for _, event := range f.events {
		if event.Sequence > after {
			result = append(result, event)
		}
	}
	return result, nil
}
func (f *fakeRunUseCases) CancelRun(context.Context, app.RunID, string) (app.RunSnapshot, bool, error) {
	f.cancelled++
	return f.snapshot, f.cancelled == 1, nil
}
func (f *fakeRunUseCases) RunHealth(context.Context) (runHealth app.RunHealthResult, err error) {
	return runHealth, nil
}

func TestBrowserRunPageSurfacesStudyInsightsCompactly(t *testing.T) {
	runs := newFakeRunUseCases()
	runs.snapshot.Target.Kind = "operation"
	runs.snapshot.Target.Operation = string(app.OperationStudyStart)
	runs.snapshot.Target.Study = "research"
	h, err := NewHandler(HandlerOptions{Queries: sampleQueries(), Runs: runs, Authority: testAuthority})
	if err != nil {
		t.Fatal(err)
	}
	body := request(h, http.MethodGet, "/runs/"+string(testRunID), nil).Body.String()
	for _, want := range []string{
		`id="run-insights-heading"`, `Study insights · research`,
		`Retries · 3 across 1 task(s)`,
		`Parallelism · decreased to 2 of 4`,
		`Memory pressure reduced parallelism from 4 to 2 agent(s)`,
		`Performance · 1 task(s)`,
		`analysis:01-structure:repo`, `<td>4m32s</td>`, `<td>45678</td>`, `<td>0.42 USD</td>`, `<td>same</td>`,
		`data-run-agent-failures`,
		`Failure reasons`, `runtime.failed`,
		`provider exited before the report was committed (exit 1)`,
		`<dt>Active tasks</dt>`, `<dt>Failed</dt>`, `<dt>Pending</dt>`,
		`data-study-resources="/api/v1/studies/research/resources"`,
		`data-resource="parallelism"`, `/static/resource-monitor.js`,
		`data-run-agent-tasks`,
		`data-run-parallelism`,
		`"requested_parallelism":4`,
		`"effective_parallelism":2`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("run study insights missing %q", want)
		}
	}
	for _, want := range []string{`"retry_after":"2026-08-21T12:30:00Z"`, `"provider":"openai"`, `"model":"gpt-5.2"`, `"harness":"codex"`, `"attempts":4`, `"retries":3`, `"session_reuse":"same"`, `"session_id":"sess_study_01"`} {
		if !strings.Contains(body, want) {
			t.Errorf("agent seed facts missing %q", want)
		}
	}
	js := request(h, http.MethodGet, "/static/app.js", nil).Body.String()
	for _, want := range []string{"agentRetryWait", "Next retry in", `["Provider", facts.provider]`, `["Model", facts.model]`, `["Harness", facts.harness]`, `agent-fact`,
		"runSlotPlan", "requested_parallelism", "effective_parallelism", "agent-slot-throttled",
		"Memory pressure reduced parallelism from", "selectAgentTab"} {
		if !strings.Contains(js, want) {
			t.Errorf("agent retry/harness enhancement missing %q", want)
		}
	}
	if count := strings.Count(body, `<details class="run-insight">`); count != 3 {
		t.Fatalf("insight details blocks=%d, want 3 compacted sections", count)
	}

	runs.snapshot.Target.Study = ""
	body = request(h, http.MethodGet, "/runs/"+string(testRunID), nil).Body.String()
	if strings.Contains(body, "run-insights-heading") {
		t.Fatal("non-study run rendered study insights")
	}
}

func TestCanonicalRunListDetailReplayAndCursorErrors(t *testing.T) {
	runs := newFakeRunUseCases()
	h, err := NewHandler(HandlerOptions{
		Queries: sampleQueries(), Runs: runs, Authority: testAuthority,
		Now: func() time.Time { return time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC) }, RequestID: func() string { return "run-request" },
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, target := range []string{"/api/v1/runs?project=alpha&limit=50", "/api/v1/runs/" + string(testRunID), "/api/v1/runs/" + string(testRunID) + "/events?after=0"} {
		response := request(h, http.MethodGet, target, nil)
		if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte(testRunID)) {
			t.Fatalf("GET %s status=%d body=%s", target, response.Code, response.Body.String())
		}
	}
	ahead := request(h, http.MethodGet, "/api/v1/runs/"+string(testRunID)+"/events?after=3", nil)
	if ahead.Code != http.StatusConflict || !bytes.Contains(ahead.Body.Bytes(), []byte(`"code":"cursor_ahead"`)) {
		t.Fatalf("ahead status=%d body=%s", ahead.Code, ahead.Body.String())
	}
	conflictRequest := httptest.NewRequest(http.MethodGet, "/api/v1/runs/"+string(testRunID)+"/events?after=0", nil)
	conflictRequest.Host = testAuthority
	conflictRequest.Header.Set("Last-Event-ID", "1")
	conflict := httptest.NewRecorder()
	h.ServeHTTP(conflict, conflictRequest)
	if conflict.Code != http.StatusBadRequest || !bytes.Contains(conflict.Body.Bytes(), []byte(`"code":"cursor_conflict"`)) {
		t.Fatalf("conflict status=%d body=%s", conflict.Code, conflict.Body.String())
	}
	runs.snapshot.RecordState = "tombstone"
	runs.snapshot.HistoryComplete = false
	runs.snapshot.OldestRetainedSequence = 2
	gap := request(h, http.MethodGet, "/api/v1/runs/"+string(testRunID)+"/events?after=0", nil)
	if gap.Code != http.StatusConflict || !bytes.Contains(gap.Body.Bytes(), []byte(`"code":"replay_gap"`)) || !bytes.Contains(gap.Body.Bytes(), []byte(`"record_state":"tombstone"`)) {
		t.Fatalf("tombstone gap status=%d body=%s", gap.Code, gap.Body.String())
	}
}

func TestBrowserRunPagesEscapeHostileDurableFieldsAndExposeRecoveryFacts(t *testing.T) {
	runs := newFakeRunUseCases()
	runs.snapshot.Target.Project = `<script>alert("project")</script>`
	runs.snapshot.ProductStatus = `<img src=x onerror=alert(1)>`
	runs.snapshot.HistoryComplete = false
	runs.snapshot.OldestRetainedSequence = 2
	runs.events = []app.RunEvent{{
		RunID: testRunID, Sequence: 2, Type: "warning", Stage: `<script>alert("stage")</script>`,
		Omission: &app.RunOmission{Reason: `<img src=x onerror=alert(2)>`, Count: 3},
	}}
	h, err := NewHandler(HandlerOptions{Queries: sampleQueries(), Runs: runs, Authority: testAuthority})
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"/runs", "/runs/" + string(testRunID)} {
		response := request(h, http.MethodGet, target, nil)
		body := response.Body.String()
		if response.Code != http.StatusOK {
			t.Fatalf("GET %s status=%d body=%s", target, response.Code, body)
		}
		if bytes.Contains([]byte(body), []byte("<script>")) || bytes.Contains([]byte(body), []byte("<img src=x")) {
			t.Fatalf("GET %s rendered hostile markup: %s", target, body)
		}
		if !bytes.Contains([]byte(body), []byte("&lt;")) {
			t.Fatalf("GET %s did not render escaped hostile data: %s", target, body)
		}
	}
	detail := request(h, http.MethodGet, "/runs/"+string(testRunID), nil)
	if !bytes.Contains(detail.Body.Bytes(), []byte("incomplete before sequence 2")) || !bytes.Contains(detail.Body.Bytes(), []byte("Omitted 3 detail item(s)")) {
		t.Fatalf("detail omitted recovery facts: %s", detail.Body.String())
	}
}

func TestBrowserRunPagesRenderAgentFactsForTheRunLoopGrid(t *testing.T) {
	committed := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	runs := newFakeRunUseCases()
	runs.snapshot.Lifecycle = "running"
	runs.snapshot.Terminal = nil
	runs.snapshot.FinishedAt = nil
	runs.events = []app.RunEvent{
		{RunID: testRunID, Sequence: 1, CommittedAt: committed, Type: "progress", Stage: "started", Task: "analysis:demo:02:runtime:repo:directory", Payload: map[string]string{"state": "running"}},
		{RunID: testRunID, Sequence: 2, CommittedAt: committed.Add(time.Second), Type: "progress", Stage: "runtime", Task: "analysis:demo:02:runtime:repo:directory", Payload: map[string]string{"kind": "tool", "type": "tool.completed", "tool": "bash"}},
		{RunID: testRunID, Sequence: 3, CommittedAt: committed.Add(2 * time.Second), Type: "progress", Stage: "completed", Task: "analysis:demo:02:runtime:repo:directory", Payload: map[string]string{"state": "completed"}},
	}
	h, err := NewHandler(HandlerOptions{Queries: sampleQueries(), Runs: runs, Authority: testAuthority})
	if err != nil {
		t.Fatal(err)
	}
	body := request(h, http.MethodGet, "/runs/"+string(testRunID), nil).Body.String()
	for _, want := range []string{
		`data-run-task="analysis:demo:02:runtime:repo:directory"`,
		`data-run-stage="runtime"`,
		`data-run-time="2026-08-21T12:00:01Z"`,
		`data-run-kind="tool"`,
		`data-run-tool="bash"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("agent grid facts missing %q in %s", want, body)
		}
	}
}

func TestBrowserRunDurableOperationCompatibilitySurvivesMissingLocalHubRecord(t *testing.T) {
	runs := newFakeRunUseCases()
	runs.snapshot.Target.Kind = "operation"
	runs.snapshot.Target.Operation = string(app.OperationExecuteStart)
	h, err := NewHandler(HandlerOptions{Queries: sampleQueries(), Runs: runs, Authority: testAuthority})
	if err != nil {
		t.Fatal(err)
	}
	status := request(h, http.MethodGet, "/api/v1/operations/"+string(testRunID), nil)
	if status.Code != http.StatusOK || !bytes.Contains(status.Body.Bytes(), []byte(`"id":"`+string(testRunID)+`"`)) || !bytes.Contains(status.Body.Bytes(), []byte(`"kind":"execute-start"`)) {
		t.Fatalf("durable operation status=%d body=%s", status.Code, status.Body.String())
	}
	active := request(h, http.MethodGet, "/api/v1/operations", nil)
	if active.Code != http.StatusOK {
		t.Fatalf("durable active status=%d body=%s", active.Code, active.Body.String())
	}
	page := request(h, http.MethodGet, "/operations/"+string(testRunID), nil)
	if page.Code != http.StatusSeeOther || page.Header().Get("Location") != "/runs/"+string(testRunID) {
		t.Fatalf("operation redirect=%d location=%q", page.Code, page.Header().Get("Location"))
	}
	stream := request(h, http.MethodGet, "/api/v1/operations/"+string(testRunID)+"/events", nil)
	if stream.Code != http.StatusOK || !bytes.Contains(stream.Body.Bytes(), []byte("event: progress")) || !bytes.Contains(stream.Body.Bytes(), []byte("event: terminal")) {
		t.Fatalf("durable operation stream=%d body=%s", stream.Code, stream.Body.String())
	}
	legacy := request(h, http.MethodGet, "/api/v1/operations/op_expired", nil)
	if legacy.Code != http.StatusGone || !bytes.Contains(legacy.Body.Bytes(), []byte(`"code":"legacy_operation_not_retained"`)) {
		t.Fatalf("legacy operation status=%d body=%s", legacy.Code, legacy.Body.String())
	}
}

func TestBrowserRunTwoServerRepositoriesShareObservationAndCancellation(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	first, err := runcontrol.OpenSQLite(ctx, root, runcontrol.SQLiteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := runcontrol.OpenSQLite(ctx, root, runcontrol.SQLiteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	snapshot, err := first.Accept(ctx, runcontrol.Acceptance{Target: runcontrol.Target{Kind: "operation", Operation: string(app.OperationExecuteStart), Project: "alpha", Sprint: "35"}})
	if err != nil {
		t.Fatal(err)
	}
	owner := runcontrol.Owner{ID: "two-server-owner", Process: runcontrol.ProcessIdentity{PID: 1}}
	attempt, _, err := first.Claim(ctx, runcontrol.Claim{RunID: snapshot.RunID, Owner: owner, Lease: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	fence := runcontrol.Fence{RunID: snapshot.RunID, AttemptID: attempt.ID, OwnerID: owner.ID, FencingGeneration: attempt.FencingGeneration}
	if _, _, err := first.Append(ctx, fence, runcontrol.EventDraft{Type: runcontrol.EventProgress, Payload: map[string]string{"state": "running"}}); err != nil {
		t.Fatal(err)
	}
	one, err := NewHandler(HandlerOptions{Queries: sampleQueries(), Runs: sqliteRunUseCases{first}, Authority: testAuthority})
	if err != nil {
		t.Fatal(err)
	}
	two, err := NewHandler(HandlerOptions{Queries: sampleQueries(), Runs: sqliteRunUseCases{second}, Authority: testAuthority})
	if err != nil {
		t.Fatal(err)
	}
	for _, handler := range []http.Handler{one, two} {
		response := request(handler, http.MethodGet, "/api/v1/runs/"+string(snapshot.RunID)+"/events?after=0", nil)
		if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte(`"sequence":1`)) {
			t.Fatalf("shared observer status=%d body=%s", response.Code, response.Body.String())
		}
	}
	cookie, csrf := establishOperationSession(t, two)
	cancelled := operationMutationRequest(two, http.MethodDelete, "/api/v1/runs/"+string(snapshot.RunID), "", cookie, csrf)
	if cancelled.Code != http.StatusOK {
		t.Fatalf("cross-server cancellation status=%d body=%s", cancelled.Code, cancelled.Body.String())
	}
	observed, err := first.Snapshot(ctx, snapshot.RunID)
	if err != nil || observed.Cancellation.State != runcontrol.CancellationRequested {
		t.Fatalf("first server did not observe cancellation: snapshot=%+v err=%v", observed, err)
	}
}
