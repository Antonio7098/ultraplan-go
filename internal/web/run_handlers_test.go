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

func TestBrowserRunPageSurfacesCompleteQACockpit(t *testing.T) {
	runs := newFakeRunUseCases()
	runs.snapshot.Target = app.RunTarget{Kind: "operation", Operation: string(app.OperationQAStart), Project: "alpha", Sprint: "30-web"}
	runs.events = []app.RunEvent{{RunID: testRunID, Sequence: 1, Type: "progress", Stage: "running", Task: "qa-v1-shard-aaaaaaaaaaaaaaaaaaaaaaaa", Payload: map[string]string{"state": "running", "action": "shard_terminal", "reason": "completed", "detail": "boundary", "count": "1/1"}}}
	runs.snapshot.LastSequence = 1
	started := time.Date(2026, 8, 24, 10, 0, 0, 0, time.UTC)
	completed := started.Add(2 * time.Second)
	attempt := app.QAInvestigatorAttemptSummary{ID: "op/shard/1", Number: 1, StartedAt: started, CompletedAt: &completed, Duration: "2s", ImplementationBefore: "impl-before", ImplementationAfter: "impl-after", StopReason: "terminal investigator output accepted", ContextRequests: []app.QAContextRequestSummary{{ID: "context-1", Paths: []string{"internal/app"}, Reason: "inspect boundary", Approved: true}}, Commands: []app.QACommandSummary{{CheckID: "go-test-app", DescriptorFingerprint: "check-fingerprint", ExitCode: 0, Duration: "1.2s", StdoutDigest: "stdout-digest", OutputBytes: 128, Truncated: true}}, Evidence: []app.QAEvidenceSummary{{Kind: "check", Summary: "bounded evidence", Paths: []string{"internal/app"}, CheckID: "go-test-app", OutputDigest: "output-digest"}}}
	theory := app.QATheorySummary{ID: "qa-v1-theory-bbbbbbbbbbbbbbbbbbbbbbbb", ShardID: "qa-v1-shard-aaaaaaaaaaaaaaaaaaaaaaaa", Claim: "hostile <script>alert(1)</script> boundary theory", Basis: "shared path", VerificationSurface: "API response", ExpectationRefs: []string{"AC-1"}, SeverityIfConfirmed: "high", ConfirmationCondition: "request fails", RefutationCondition: "request passes", InconclusiveCondition: "check unavailable", SafeEvidenceStrategy: "approved check", ImplementationFingerprint: "impl-before", AttemptHistory: []app.QAInvestigatorAttemptSummary{attempt}, Evidence: attempt.Evidence, Outcome: "refuted", OutcomeReason: "approved check passed"}
	qa := app.QAResult{SchemaVersion: 1, Project: "alpha", Sprint: "30-web", Phase: "completed", Fresh: true, AttemptID: "qa-v1-attempt-cccccccccccccccccccccccc", RunID: string(testRunID), OperationalAttemptID: "att_aaaaaaaaaaaaaaaaaaaaaaaaaa", FencingGeneration: 7, RunLifecycle: "terminal", TerminalResult: "completed", GovernedInputFingerprint: "governed", ImplementationFingerprint: "implementation", ReviewFingerprint: "review", MapFingerprint: "map", PolicyFingerprint: "policy", CheckCatalogFingerprint: "catalog", UpdatedAt: completed, MapRecord: &app.QAArtifactRefSummary{Path: "verification/map.json", Digest: "map-digest"}, SynthesisRecord: &app.QAArtifactRefSummary{Path: "verification/synthesis.json", Digest: "synthesis-digest"}, EffectiveSources: []app.QAEffectiveSourceSummary{{Field: "runtime.model", Source: "workspace config"}}, Target: app.QATargetIdentitySummary{Fingerprint: "target", Scope: "current_checkout", GitHead: "head", GitIndex: "index", GitWorktree: "worktree", WorkspaceBranch: "ultraplan/alpha/30-web", WorkspaceBaseline: "baseline", BaselineRelation: "ahead_of_baseline", CommitsSinceBase: 4, Categories: map[string]string{"status": "dirty"}}, Coverage: app.QACoverageSummary{ChangedPaths: []string{"internal/app/a.go", "internal/app/b.go"}, PrimaryOwners: map[string]string{"internal/app/a.go": "qa-v1-shard-aaaaaaaaaaaaaaaaaaaaaaaa"}, BoundaryOverlaps: map[string][]string{"internal/app/shared.go": {"qa-v1-shard-aaaaaaaaaaaaaaaaaaaaaaaa"}}, BlockedPaths: []string{"internal/app/blocked.go"}}, InputRefs: []app.QAArtifactRefSummary{{Path: "requirements.md", Digest: "requirements-digest"}}, Limits: app.QALimitsSummary{ConcurrentInvestigators: 3, TheoriesPerShard: 12, OutputRepairAttempts: 1, CommandsPerAttempt: 8, ContextExpansions: 2, PathsPerExpansion: 16, CommandTimeout: "5m0s", ShardTimeout: "20m0s", RunTimeout: "1h0m0s", CommandOutputBytes: 262144, ShardOutputBytes: 1048576, PromptBytes: 524288, FollowUpShards: 4}, ChangedPaths: 2, CoveredPaths: 2, CompletedShards: 1, TotalShards: 1, OutcomeTotals: map[string]int{"refuted": 1}, NextAction: "Inspect retained outcomes.", Shards: []app.QAShardSummary{{ID: "qa-v1-shard-aaaaaaaaaaaaaaaaaaaaaaaa", AttemptID: "qa-v1-attempt-cccccccccccccccccccccccc", Kind: "boundary", Title: "API boundary", Phase: "completed", ChangedPaths: []string{"internal/app/a.go"}, ContextPaths: []string{"internal/app/b.go"}, OverlapPaths: []string{"internal/app/shared.go"}, BoundaryReason: "shared behavior", BehavioralConcerns: []string{"cancellation"}, ExpectationRefs: []string{"AC-1"}, RiskTags: []string{"concurrency"}, ApprovedChecks: []app.QAApprovedCheckSummary{{ID: "go-test-app", Fingerprint: "check-fingerprint"}}, Attempts: []app.QAInvestigatorAttemptSummary{attempt}, TheoryCount: 1, Theories: []app.QATheorySummary{theory}}}}
	synth := app.QASynthesisResult{QAResult: qa, ID: "qa-v1-synthesis-dddddddddddddddddddddddd", MapID: "map", NextAction: "Ship the verified change.", TheoryIDs: []string{theory.ID}, Challenges: []app.QAChallengeSummary{{ID: "challenge-1", TheoryIDs: []string{theory.ID}, Claim: "challenge claim", Basis: "challenger basis", SafeEvidenceStrategy: "compare evidence", EvidenceRefs: []string{"output-digest"}}}, OutcomeTotals: map[string]int{"refuted": 1}, Deduplicated: map[string][]string{"canonical-theory": {theory.ID}}, Contradictions: [][]string{{theory.ID, "qa-v1-theory-other"}}, Interactions: []string{"API and cancellation paths interact"}, Blockers: []app.QABlockerSummary{{Category: "policy", Scope: "synthesis", Summary: "review required", NextAction: "Inspect policy."}}, FollowUpShards: []app.QAShardSummary{{ID: "qa-v1-shard-followup", Title: "Follow-up", Phase: "completed"}}}
	fixture := &qaQueryFixture{fakeQueries: sampleQueries(), qa: qa, synth: synth}
	h, err := NewHandler(HandlerOptions{Queries: fixture, Runs: runs, Authority: testAuthority})
	if err != nil {
		t.Fatal(err)
	}
	body := request(h, http.MethodGet, "/runs/"+string(testRunID), nil).Body.String()
	for _, want := range []string{"qa-run-cockpit-v1", `data-qa-cockpit`, "QA command center", "Operator next action", "1 / 1 shards", "current evidence", "Investigation shards", "API boundary", "Investigator attempts", "Approved checks executed", "go-test-app", "stdout-digest", "Context requests", "bounded evidence", "Theories and outcomes", "approved check passed", "Synthesis", "challenge claim", "Deduplication", "Contradictions", "API and cancellation paths interact", "Current checkout tested.", "4 commits after the sprint workspace baseline", "current_checkout", "ultraplan/alpha/30-web", "Map inputs and target identity", "workspace config", "map-digest", "requirements-digest", "Coverage ownership · 1 paths", "internal/app/blocked.go", "Effective safety limits", "Evidence boundary", "action=shard_terminal", "result=completed", "progress=1/1"} {
		if !strings.Contains(body, want) {
			t.Errorf("QA cockpit missing %q", want)
		}
	}
	if strings.Contains(body, "<script>alert(1)</script>") || strings.Contains(body, `data-run-agents`) {
		t.Fatal("QA cockpit rendered unsafe theory copy or study-agent UI")
	}
	js := request(h, http.MethodGet, "/static/app.js", nil).Body.String()
	for _, want := range []string{"refreshQAObservation", "scheduleQAObservationRefresh", "DOMParser", "CSS.escape", "payload.count"} {
		if !strings.Contains(js, want) {
			t.Errorf("QA live refresh missing %q", want)
		}
	}
}

func TestBrowserHistoricalQARunDoesNotMixCurrentCanonicalState(t *testing.T) {
	runs := newFakeRunUseCases()
	runs.snapshot.Target = app.RunTarget{Kind: "operation", Operation: string(app.OperationQAResume), Project: "alpha", Sprint: "30-web"}
	qa := app.QAResult{Project: "alpha", Sprint: "30-web", RunID: "run_bbbbbbbbbbbbbbbbbbbbbbbbbb", Phase: "running"}
	fixture := &qaQueryFixture{fakeQueries: sampleQueries(), qa: qa}
	h, err := NewHandler(HandlerOptions{Queries: fixture, Runs: runs, Authority: testAuthority})
	if err != nil {
		t.Fatal(err)
	}
	body := request(h, http.MethodGet, "/runs/"+string(testRunID), nil).Body.String()
	if !strings.Contains(body, "This is a historical QA run") || !strings.Contains(body, qa.RunID) || strings.Contains(body, "Investigation shards") {
		t.Fatalf("historical QA isolation missing: %s", body)
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

func TestSprintStageUsageViewGroupsByStageAndAggregatesCost(t *testing.T) {
	metrics := app.SprintMetricsSummary{
		RecentRuns: []app.SprintMetricRow{
			{Stage: "plan", Model: "x", Status: "ok", InputKnown: true, Input: 1000, OutputKnown: true, Output: 250,
				CacheReadKnown: true, CacheRead: 100, CacheWriteKnown: true, CacheWrite: 0, TotalKnown: true, Total: 1350,
				CostKnown: true, CostAmount: 0.012, CostSource: "model_priced"},
			{Stage: "plan", Model: "x", Status: "ok", InputKnown: true, Input: 500, OutputKnown: true, Output: 120,
				CacheReadKnown: true, CacheRead: 50, CacheWriteKnown: true, CacheWrite: 0, TotalKnown: true, Total: 670,
				CostKnown: true, CostAmount: 0.006, CostSource: "provider_reported"},
			{Stage: "execute", Model: "y", Status: "ok", InputKnown: true, Input: 2000, OutputKnown: true, Output: 400,
				CacheReadKnown: true, CacheRead: 200, CacheWriteKnown: true, CacheWrite: 0, TotalKnown: true, Total: 2600,
				CostKnown: false, CostSource: "unpriced"},
		},
	}
	view := newSprintStageUsageView("35", metrics)
	if !view.HasMetrics {
		t.Fatal("expected HasMetrics to be true")
	}
	if view.TasksWithUsage != 3 {
		t.Fatalf("expected 3 priced-or-unpriced tasks, got %d", view.TasksWithUsage)
	}
	if view.TasksPriced != 2 || view.TasksUnpriced != 1 {
		t.Fatalf("expected 2 priced and 1 unpriced task, got priced=%d unpriced=%d", view.TasksPriced, view.TasksUnpriced)
	}
	if view.Total != 1350+670+2600 {
		t.Fatalf("aggregate total tokens mismatch: got %d want %d", view.Total, 1350+670+2600)
	}
	if len(view.Rows) != 2 {
		t.Fatalf("expected two stage rows (plan, execute) in canonical order, got %d", len(view.Rows))
	}
	if view.Rows[0].Stage != "plan" || view.Rows[0].Runs != 2 || view.Rows[0].Tokens != "2020" {
		t.Fatalf("plan row mismatch: %+v", view.Rows[0])
	}
	if !strings.HasSuffix(view.Rows[0].Cost, "*") {
		t.Fatalf("plan cost should carry the rate-table asterisk, got %q", view.Rows[0].Cost)
	}
	if view.Rows[1].Stage != "execute" || view.Rows[1].Cost != "-" {
		t.Fatalf("execute row should report unpriced cost as dash, got %+v", view.Rows[1])
	}
	if view.Rows[1].CacheR != "200" || view.Rows[1].CacheW != "0" {
		t.Fatalf("execute cache splits mismatch: %+v", view.Rows[1])
	}
}

func TestStudyUsageViewAggregatesTaskTokensAndCost(t *testing.T) {
	tasks := []app.RunTaskSummary{
		{ID: "a", Status: "completed", InputKnown: true, InputTokens: 100, OutputKnown: true, OutputTokens: 25,
			CacheReadKnown: true, CacheReadTokens: 10, CacheWriteKnown: true, CacheWriteTokens: 0,
			TokensKnown: true, Tokens: 135, CostKnown: true, CostAmount: 0.01, CostSource: "model_priced", Cost: "$0.01"},
		{ID: "b", Status: "completed", InputKnown: true, InputTokens: 200, OutputKnown: true, OutputTokens: 50,
			CacheReadKnown: true, CacheReadTokens: 20, CacheWriteKnown: true, CacheWriteTokens: 0,
			TokensKnown: true, Tokens: 270, CostKnown: true, CostAmount: 0.02, CostSource: "provider_reported", Cost: "$0.02"},
	}
	view := newStudyUsageView("research", tasks)
	if !view.HasUsage {
		t.Fatal("expected HasUsage to be true")
	}
	if view.Input != 300 || view.Output != 75 || view.CacheRead != 30 || view.Total != 405 {
		t.Fatalf("aggregate mismatch: input=%d output=%d cache=%d total=%d", view.Input, view.Output, view.CacheRead, view.Total)
	}
	if view.TasksPriced != 2 || view.ModelPriced != 1 || view.ProviderReported != 1 {
		t.Fatalf("provenance counts wrong: priced=%d model=%d reported=%d", view.TasksPriced, view.ModelPriced, view.ProviderReported)
	}
	if !strings.HasSuffix(view.CostLabel, "*") {
		t.Fatalf("CostLabel should mark mixed provenance with asterisk, got %q", view.CostLabel)
	}
	if len(view.Tasks) != 2 {
		t.Fatalf("expected Tasks slice to preserve the input rows, got %d", len(view.Tasks))
	}
}

func TestStudyUsageViewIsNilSafeForEmptyTasks(t *testing.T) {
	view := newStudyUsageView("research", nil)
	if view.HasUsage || len(view.Tasks) != 0 {
		t.Fatalf("empty tasks should produce empty view, got %+v", view)
	}
}
