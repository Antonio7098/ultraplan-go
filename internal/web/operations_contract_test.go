package web

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"regexp"
	"sort"
	"testing"
	"time"

	"github.com/Antonio7098/ultraplan-go/internal/app"
)

// These fixtures are the in-repository producer/consumer contract for the
// dependency-free browser client. A public shape change must update the Go
// handler, embedded client, and this compatibility table together.
func TestBrowserOperationKindContract(t *testing.T) {
	projectScope := operationScopeRequest{Project: "alpha", Sprint: "31-web"}
	studyScope := operationScopeRequest{Study: "research"}
	cases := []struct {
		kind  string
		want  app.OperationKind
		scope operationScopeRequest
	}{
		{"validation", app.OperationValidate, projectScope},
		{"sprint-status", app.OperationSprintStatus, projectScope},
		{"sprint-prompt", app.OperationPrompt, projectScope},
		{"sprint-flow-dry-run", app.OperationFlowDryRun, projectScope},
		{"sprint-flow", app.OperationFlow, projectScope},
		{"sprint-stage-dry-run", app.OperationStageDryRun, projectScope},
		{"sprint-stage", app.OperationStage, projectScope},
		{"execute-status", app.OperationExecuteStatus, projectScope},
		{"execute-dry-run", app.OperationExecuteDryRun, projectScope},
		{"execute-start", app.OperationExecuteStart, projectScope},
		{"execute-resume", app.OperationExecuteResume, projectScope},
		{"review-status", app.OperationReviewStatus, projectScope},
		{"review-dry-run", app.OperationReviewDryRun, projectScope},
		{"review-start", app.OperationReviewStart, projectScope},
		{"smoke-status", app.OperationSmokeStatus, projectScope},
		{"smoke-dry-run", app.OperationSmokeDryRun, projectScope},
		{"smoke-start", app.OperationSmokeStart, projectScope},
		{"verify-dry-run", app.OperationVerifyDryRun, projectScope},
		{"verify-start", app.OperationVerifyStart, projectScope},
		{"study-start", app.OperationStudyStart, studyScope},
		{"study-resume", app.OperationStudyResume, studyScope},
		{"study-cancel", app.OperationStudyCancel, studyScope},
	}
	for _, tc := range cases {
		t.Run(tc.kind, func(t *testing.T) {
			req, err := mapOperationRequest(operationSpecRequest{Kind: tc.kind, Scope: tc.scope})
			if err != nil || req.Kind != tc.want {
				t.Fatalf("kind=%q mapped=%q err=%v", tc.kind, req.Kind, err)
			}
		})
	}
}

func TestBrowserLifecycleDocumentContract(t *testing.T) {
	states := []string{"accepted", "running", "cancelling", "succeeded", "failed", "cancelled", "interrupted", "cleanup_uncertain"}
	for _, state := range states {
		doc := operationDocument{
			ID: "op_contract", Kind: app.OperationFlow, State: state,
			CreatedAt: time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC), LastEventID: "7",
			DurableStatus: durableStatusDTO{Available: true, RefreshPath: "/api/v1/projects/alpha/sprints/31-web"},
		}
		data, err := json.Marshal(doc)
		if err != nil {
			t.Fatal(err)
		}
		var browser struct {
			ID            string `json:"id"`
			Kind          string `json:"kind"`
			State         string `json:"state"`
			LastEventID   string `json:"last_event_id"`
			DurableStatus struct {
				Available   bool   `json:"available"`
				RefreshPath string `json:"refresh_path"`
			} `json:"durable_status"`
		}
		if err := json.Unmarshal(data, &browser); err != nil {
			t.Fatal(err)
		}
		if browser.ID == "" || browser.Kind != string(app.OperationFlow) || browser.State != state || browser.LastEventID != "7" || !browser.DurableStatus.Available || browser.DurableStatus.RefreshPath == "" {
			t.Fatalf("state %q projection=%+v json=%s", state, browser, data)
		}
		if terminalOperationState(state) != (state == "succeeded" || state == "failed" || state == "cancelled" || state == "interrupted" || state == "cleanup_uncertain") {
			t.Fatalf("terminal classification drift for %q", state)
		}
	}
}

func TestBrowserSSEEventNameAndFrameContract(t *testing.T) {
	want := []string{"artifact", "cancel_requested", "finding", "progress", "recovery_required", "snapshot", "terminal", "warning"}
	js, err := os.ReadFile("static/app.js")
	if err != nil {
		t.Fatal(err)
	}
	matches := regexp.MustCompile(`for \(const name of \[([^]]+)\]\)`).FindSubmatch(js)
	if len(matches) != 2 {
		t.Fatal("embedded client event subscription list is missing")
	}
	var client []string
	if err := json.Unmarshal(append([]byte{'['}, append(matches[1], ']')...), &client); err != nil {
		t.Fatalf("decode client event names: %v", err)
	}
	sort.Strings(client)
	if !reflect.DeepEqual(client, want) {
		t.Fatalf("client events=%v want=%v", client, want)
	}
	for i, name := range want {
		if stableOperationEventName(name) != name {
			t.Fatalf("server rejected stable event %q", name)
		}
		recorder := httptest.NewRecorder()
		event := operationEvent{ID: uint64(i + 1), Name: name, Data: []byte(`{"operation_id":"op_contract","sequence":1,"payload":{}}`)}
		if err := writeSSEEvent(recorder, event); err != nil {
			t.Fatal(err)
		}
		body := recorder.Body.String()
		if !regexp.MustCompile(`(?m)^id: [0-9]+$`).MatchString(body) || !regexp.MustCompile(`(?m)^event: `+regexp.QuoteMeta(name)+`$`).MatchString(body) || !regexp.MustCompile(`(?m)^data: \{`).MatchString(body) {
			t.Fatalf("invalid %q frame: %q", name, body)
		}
	}
	if stableOperationEventName("provider_native_event") != "progress" {
		t.Fatal("unknown producer event escaped the compatibility boundary")
	}
}

func TestOperationErrorCompatibilityContract(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"invalid", errors.New("invalid operation scope"), http.StatusBadRequest, "invalid_request"},
		{"expired", errConfirmationExpired, http.StatusConflict, "confirmation_expired"},
		{"mismatch", errConfirmationMismatch, http.StatusConflict, "confirmation_mismatch"},
		{"replayed", errConfirmationReplayed, http.StatusConflict, "confirmation_replayed"},
		{"stale", errStaleConfirmation, http.StatusConflict, "stale_confirmation"},
		{"not-found", errOperationNotFound, http.StatusNotFound, "operation_not_found"},
		{"conflict", errors.New("mutation lock is in progress"), http.StatusConflict, "operation_conflict"},
		{"validation", errors.New("validation failed"), http.StatusUnprocessableEntity, "validation_failed"},
		{"prerequisite", errors.New("runtime unavailable"), http.StatusFailedDependency, "prerequisite_unavailable"},
		{"operation-capacity", errOperationCapacity, http.StatusTooManyRequests, "operation_capacity"},
		{"subscriber-capacity", errSubscriberCapacity, http.StatusTooManyRequests, "subscriber_capacity"},
		{"draining", errServerDraining, http.StatusServiceUnavailable, "server_draining"},
		{"internal", errors.New("database exploded"), http.StatusInternalServerError, "internal_failure"},
	}
	h := &handler{now: func() time.Time { return time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC) }}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/operations", nil).WithContext(context.Background())
			res := httptest.NewRecorder()
			h.writeOperationError(res, req, tc.err)
			var envelope errorEnvelope
			if err := json.Unmarshal(res.Body.Bytes(), &envelope); err != nil {
				t.Fatal(err)
			}
			if res.Code != tc.status || envelope.Error.Code != tc.code {
				t.Fatalf("status=%d code=%q body=%s", res.Code, envelope.Error.Code, res.Body.String())
			}
		})
	}
}
