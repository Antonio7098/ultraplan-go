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
)

type qaQueryFixture struct {
	*fakeQueries
	qa       app.QAResult
	shard    app.QAShardResult
	shardErr error
	theory   app.QATheoryResult
	synth    app.QASynthesisResult
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
