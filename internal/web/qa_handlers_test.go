package web

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Antonio7098/ultraplan-go/internal/app"
	"github.com/Antonio7098/ultraplan-go/internal/sprint"
)

type qaQueryFixture struct {
	*fakeQueries
	qa       app.QAResult
	shard    app.QAShardResult
	shardErr error
	theory   app.QATheoryResult
	synth    app.QASynthesisResult
	repair   app.RepairStatusResult
}

func (fixture *qaQueryFixture) QAMap(context.Context, app.QARequest) (app.QAResult, error) {
	result := fixture.qa
	result.Phase = "mapped"
	return result, nil
}
func (fixture *qaQueryFixture) QAStatus(context.Context, app.QARequest) (app.QAResult, error) {
	return fixture.qa, nil
}
func (fixture *qaQueryFixture) QAShard(context.Context, app.QARequest) (app.QAShardResult, error) {
	return fixture.shard, fixture.shardErr
}

func TestQAQueryErrorsUseStablePublicCodes(t *testing.T) {
	fixture := &qaQueryFixture{
		fakeQueries: sampleQueries(),
		shardErr: &app.QAUseCaseError{
			Code:      "qa.invalid_state",
			Category:  "invalid_state",
			Message:   "The QA request is not valid for the current governed state.",
			Guidance:  "Run explicit QA recovery.",
			Operation: "load state",
		},
	}
	h, err := NewHandler(HandlerOptions{Queries: fixture, Authority: testAuthority})
	if err != nil {
		t.Fatal(err)
	}
	response := request(h, http.MethodGet, "/api/v1/projects/alpha/sprints/30-web/qa/shards/qa-v1-shard-aaaaaaaaaaaaaaaaaaaaaaaa", nil)
	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), `"code":"qa.invalid_state"`) || !strings.Contains(response.Body.String(), `"category":"invalid_state"`) || !strings.Contains(response.Body.String(), `"guidance":"Run explicit QA recovery."`) || !strings.Contains(response.Body.String(), `"correlation_id":"`) || strings.Contains(response.Body.String(), "persistence") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}
func (fixture *qaQueryFixture) QATheory(context.Context, app.QARequest) (app.QATheoryResult, error) {
	return fixture.theory, nil
}
func (fixture *qaQueryFixture) QASynthesis(context.Context, app.QARequest) (app.QASynthesisResult, error) {
	return fixture.synth, nil
}
func (fixture *qaQueryFixture) RepairStatus(context.Context, app.RepairRequest) (app.RepairStatusResult, error) {
	return fixture.repair, nil
}

func TestQARoutesReturnBoundedJSONAndCompleteNoJavaScriptHTML(t *testing.T) {
	queries := sampleQueries()
	data, err := os.ReadFile(filepath.Join("..", "testdata", "qa-canonical-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var qa app.QAResult
	if err := json.Unmarshal(data, &qa); err != nil {
		t.Fatal(err)
	}
	shardID := qa.Shards[0].ID
	theory := qa.Shards[0].Theories[0]
	theoryID := theory.ID
	qa.Shards[0].Title = "hostile <script>alert(1)</script>"
	theory.Claim = "input <script>alert(2)</script> remains valid"
	fixture := &qaQueryFixture{fakeQueries: queries, qa: qa, shard: app.QAShardResult{QAResult: qa, Shard: qa.Shards[0], Theories: []app.QATheorySummary{theory}}, theory: app.QATheoryResult{QAResult: qa, Theory: theory}, synth: app.QASynthesisResult{QAResult: qa, ID: "qa-v1-synthesis-cccccccccccccccccccccccc", TheoryIDs: []string{theoryID}}}
	queries.sprint.QA = qa
	h, err := NewHandler(HandlerOptions{Queries: fixture, Authority: testAuthority, Now: func() time.Time { return time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC) }, RequestID: func() string { return "qa-request" }})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		"/api/v1/projects/alpha/sprints/30-web/qa",
		"/api/v1/projects/alpha/sprints/30-web/qa/map",
		"/api/v1/projects/alpha/sprints/30-web/qa/shards/" + shardID,
		"/api/v1/projects/alpha/sprints/30-web/qa/theories/" + theoryID,
		"/api/v1/projects/alpha/sprints/30-web/qa/synthesis",
	} {
		response := request(h, http.MethodGet, path, nil)
		if response.Code != http.StatusOK || !strings.Contains(response.Header().Get("Content-Type"), "application/json") || !strings.Contains(response.Body.String(), `"schema_version":1`) {
			t.Fatalf("%s status=%d body=%s", path, response.Code, response.Body.String())
		}
	}
	for _, path := range []string{"/projects/alpha/sprints/30-web/qa", "/projects/alpha/sprints/30-web/qa/shards/" + shardID, "/projects/alpha/sprints/30-web/qa/theories/" + theoryID} {
		response := request(h, http.MethodGet, path, nil)
		body := response.Body.String()
		if response.Code != http.StatusOK || !strings.Contains(body, "Read-only QA") || !strings.Contains(body, "Conformance Review") || !strings.Contains(body, "<noscript>") || strings.Contains(body, "<script>alert") {
			t.Fatalf("%s status=%d body=%s", path, response.Code, body)
		}
	}
	method := request(h, http.MethodPost, "/api/v1/projects/alpha/sprints/30-web/qa", bytes.NewReader(nil))
	if method.Code != http.StatusMethodNotAllowed {
		t.Fatalf("QA query POST status=%d", method.Code)
	}
}

func TestRepairRoutesReturnCanonicalBoundedJSONAndNoJavaScriptHTML(t *testing.T) {
	queries := sampleQueries()
	repair := app.RepairStatusResult{
		SchemaVersion: 1, Project: "alpha", Sprint: "30-web", Phase: "prepared", Fresh: true,
		RepairRunID: "repair-v1-run-aaaaaaaaaaaaaaaaaaaaaaaa", Mode: "manual", NextAction: "Review and confirm the frozen packet.",
		Packet:   &app.RepairPacketSummary{Digest: "sha256:packet", IssueID: "qa-v1-issue-current", IssueTitle: `hostile <script>alert(1)</script>`, Target: app.QATargetIdentitySummary{Fingerprint: "target"}, AllowedPaths: []string{"internal/app/repair.go"}, AcceptanceCriteria: []string{"exact reproducer passes"}, CheckCount: 7, Budgets: app.RepairBudgetSummary{MaxCycles: 1, MaxMutationCycles: 1, MaxFiles: 8, MaxBytes: 1024, WallTime: "45m0s"}},
		Consumed: sprint.RepairConsumed{Cycles: 1, MutationCycles: 1, ChangedFiles: 1, ChangedBytes: 42, Commands: 2, OutputBytes: 9},
		Runtime:  &sprint.RepairRuntimeObservation{Provider: "openrouter", Model: "minimax/minimax-m3:free", DurationMS: 1234, RuntimeEvents: 8, RetainedEvents: 6, ObservedToolCalls: 2, Usage: sprint.QAUsageSummary{TurnsKnown: true, Turns: 3, InputTokensKnown: true, InputTokens: 100, OutputTokensKnown: true, OutputTokens: 20, CacheReadTokensKnown: true, CacheReadTokens: 80}},
		Cycles:   []sprint.RepairCycleSnapshot{{Cycle: sprint.RepairCycle{Number: 1}, Reverification: &sprint.RepairReverification{Gates: []sprint.RepairGateResult{{Gate: sprint.RepairGateExactReproducer, Status: sprint.RepairGatePassed, DurationMS: 25, OutputBytes: 9}}}}},
	}
	fixture := &qaQueryFixture{fakeQueries: queries, repair: repair}
	h, err := NewHandler(HandlerOptions{Queries: fixture, Authority: testAuthority, RequestID: func() string { return "repair-request" }})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		"/api/v1/projects/alpha/sprints/30-web/repair",
		"/api/v1/projects/alpha/sprints/30-web/repair/packet",
		"/api/v1/projects/alpha/sprints/30-web/repair/cycles",
		"/api/v1/projects/alpha/sprints/30-web/repair/result",
	} {
		response := request(h, http.MethodGet, path, nil)
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"repair_run_id":"repair-v1-run-`) || strings.Contains(response.Body.String(), "proposal_patch") {
			t.Fatalf("%s status=%d body=%s", path, response.Code, response.Body.String())
		}
	}
	response := request(h, http.MethodGet, "/projects/alpha/sprints/30-web/repair", nil)
	body := response.Body.String()
	if response.Code != http.StatusOK || !strings.Contains(body, "Bounded repair") || !strings.Contains(body, "Runtime and verification evidence") || !strings.Contains(body, "minimax/minimax-m3:free") || !strings.Contains(body, "Cache read") || !strings.Contains(body, "exact_reproducer") || !strings.Contains(body, "25 ms") || !strings.Contains(body, "Automatic campaign admission") || !strings.Contains(body, "<noscript>") || strings.Contains(body, "<script>alert") {
		t.Fatalf("repair page status=%d body=%s", response.Code, body)
	}
	method := request(h, http.MethodPost, "/api/v1/projects/alpha/sprints/30-web/repair", bytes.NewReader(nil))
	if method.Code != http.StatusMethodNotAllowed {
		t.Fatalf("repair query POST status=%d", method.Code)
	}
}
