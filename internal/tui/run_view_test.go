package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Antonio7098/ultraplan-go/internal/app"
	"github.com/Antonio7098/ultraplan-go/internal/runcontrol"
)

type durableTUIUseCases struct {
	*fakeUseCases
	page      app.RunPage
	events    []app.RunEvent
	cancelled int
}

func (f *durableTUIUseCases) Runs(context.Context, app.RunQuery) (app.RunPage, error) {
	return f.page, nil
}
func (f *durableTUIUseCases) Run(_ context.Context, id app.RunID) (app.RunSnapshot, error) {
	return f.page.Runs[0], nil
}
func (f *durableTUIUseCases) RunEvents(context.Context, app.RunID, uint64, int) ([]app.RunEvent, error) {
	return f.events, nil
}
func (f *durableTUIUseCases) CancelRun(context.Context, app.RunID, string) (app.RunSnapshot, bool, error) {
	f.cancelled++
	return f.page.Runs[0], true, nil
}
func (f *durableTUIUseCases) RunHealth(context.Context) (app.RunHealthResult, error) {
	return app.RunHealthResult{}, nil
}

func TestDurableRunListDetailUsesCanonicalVocabulary(t *testing.T) {
	now := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	runID := app.RunID("run_aaaaaaaaaaaaaaaaaaaaaaaaaa")
	useCases := &durableTUIUseCases{fakeUseCases: &fakeUseCases{}, page: app.RunPage{Runs: []app.RunSnapshot{{
		RunID: runID, Target: app.RunTarget{Kind: "sprint", Operation: "execute", Project: "alpha", Sprint: "35"},
		Lifecycle: "running", Liveness: "stalled", RecordState: "compacted", AcceptedAt: now, UpdatedAt: now,
		OldestRetainedSequence: 2, LastSequence: 3, HistoryComplete: false, Cancellation: app.RunCancellation{State: "none"},
	}}}, events: []app.RunEvent{
		{RunID: runID, Sequence: 2, Type: "warning", Omission: &app.RunOmission{Reason: "unsafe detail omitted", Count: 1}},
		{RunID: runID, Sequence: 2, Type: "warning", Omission: &app.RunOmission{Reason: "duplicate delivery", Count: 1}},
	}}
	m := NewModel(useCases)
	m = m.Update(LoadMsg{Runs: useCases.page.Runs, Events: useCases.events})
	if len(m.DurableEvents) != 1 {
		t.Fatalf("durable replay was not deduplicated: %+v", m.DurableEvents)
	}
	m = m.Update(KeyMsg("w"))
	if m.ActiveTab != TabRuns || m.currentRoute().Kind != RouteRuns || len(m.navItems()) != 1 {
		t.Fatalf("run list model=%+v items=%+v", m, m.navItems())
	}
	m = m.Update(KeyMsg("enter"))
	m.DurableEvents = useCases.events
	view := Render(m, 120)
	for _, want := range []string{string(runID), "Lifecycle: running", "Liveness: stalled", "History: compacted complete=false", "unsafe detail omitted"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q:\n%s", want, view)
		}
	}
	teaView := newTeaModel(context.Background(), useCases, 100)
	teaView.model = m
	_, refresh := teaView.Update(runViewTickMsg{})
	if refresh == nil {
		t.Fatal("durable run detail did not schedule repository recovery polling")
	}
}

func TestDurableRunDetailRendersLifecycleLivenessRetentionAndUnknownMatrix(t *testing.T) {
	now := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	runID := app.RunID("run_aaaaaaaaaaaaaaaaaaaaaaaaaa")
	cases := []struct {
		name     string
		snapshot app.RunSnapshot
		want     []string
	}{
		{"active", app.RunSnapshot{RunID: runID, Lifecycle: "running", Liveness: "live", RecordState: "full", HistoryComplete: true, OldestRetainedSequence: 1, Cancellation: app.RunCancellation{State: "none"}}, []string{"Lifecycle: running", "Liveness: live", "c requests durable cancellation"}},
		{"stalled-gap", app.RunSnapshot{RunID: runID, Lifecycle: "running", Liveness: "stalled", RecordState: "compacted", HistoryComplete: false, OldestRetainedSequence: 9, LastSequence: 12, Cancellation: app.RunCancellation{State: "uncertain"}}, []string{"Liveness: stalled", "Cancellation: uncertain", "complete=false oldest=9 last=12"}},
		{"unreachable", app.RunSnapshot{RunID: runID, Lifecycle: "running", Liveness: "owner_unreachable", RecordState: "full", HistoryComplete: true, OldestRetainedSequence: 1, Cancellation: app.RunCancellation{State: "none"}}, []string{"Liveness: owner_unreachable", "Product status: unknown"}},
		{"uncertain-terminal", app.RunSnapshot{RunID: runID, Lifecycle: "cleanup_uncertain", Liveness: "terminal", RecordState: "tombstone", HistoryComplete: false, OldestRetainedSequence: 4, LastSequence: 4, Cancellation: app.RunCancellation{State: "uncertain"}, Terminal: &runcontrol.Terminal{Outcome: "cleanup_uncertain", Reason: "owner identity unavailable", WonAt: now}}, []string{"Lifecycle: cleanup_uncertain", "History: tombstone", "Terminal: cleanup_uncertain"}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			m := NewModel(&durableTUIUseCases{fakeUseCases: &fakeUseCases{}})
			m.Routes = []Route{{Kind: RouteRuns}, {Kind: RouteRun, RunID: string(runID)}}
			m.ActiveTab = TabRuns
			m.Runs = []app.RunSnapshot{test.snapshot}
			view := Render(m, 100)
			for _, want := range test.want {
				if !strings.Contains(view, want) {
					t.Fatalf("view missing %q:\n%s", want, view)
				}
			}
		})
	}
}
