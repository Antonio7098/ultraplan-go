package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Antonio7098/ultraplan-go/internal/app"
)

type fakeWebOperations struct {
	mu      sync.Mutex
	started chan struct{}
	release chan struct{}
	done    chan struct{}
	once    sync.Once
	runs    int
	cleanup []app.OperationCleanupUncertain
}

type capacityWebOperations struct {
	*fakeWebOperations
	startedMany chan struct{}
	releaseMany chan struct{}
	finished    chan struct{}
}

type deadlineWebOperations struct {
	*fakeWebOperations
	startedDeadline chan struct{}
	releaseDeadline chan struct{}
	finished        chan struct{}
}

func newDeadlineWebOperations() *deadlineWebOperations {
	return &deadlineWebOperations{
		fakeWebOperations: newFakeWebOperations(),
		startedDeadline:   make(chan struct{}),
		releaseDeadline:   make(chan struct{}),
		finished:          make(chan struct{}),
	}
}

func (f *deadlineWebOperations) RunOperation(_ context.Context, _ app.OperationRequest, emit func(app.OperationEvent)) (app.OperationResult, error) {
	emit(app.OperationEvent{State: app.OperationRunning, Stage: "plan", Message: "working"})
	close(f.startedDeadline)
	<-f.releaseDeadline
	close(f.finished)
	return app.OperationResult{State: app.OperationComplete, Message: "complete"}, nil
}

func newCapacityWebOperations() *capacityWebOperations {
	return &capacityWebOperations{
		fakeWebOperations: newFakeWebOperations(),
		startedMany:       make(chan struct{}, MaxActiveOperations),
		releaseMany:       make(chan struct{}),
		finished:          make(chan struct{}, MaxActiveOperations),
	}
}

func (f *capacityWebOperations) RunOperation(ctx context.Context, _ app.OperationRequest, emit func(app.OperationEvent)) (app.OperationResult, error) {
	emit(app.OperationEvent{State: app.OperationRunning, Stage: "plan", Message: "working"})
	f.startedMany <- struct{}{}
	defer func() { f.finished <- struct{}{} }()
	select {
	case <-f.releaseMany:
		return app.OperationResult{State: app.OperationComplete, Message: "complete"}, nil
	case <-ctx.Done():
		return app.OperationResult{State: app.OperationCancelled, Message: "cancelled"}, ctx.Err()
	}
}

func (f *fakeWebOperations) RecordOperationCleanupUncertain(_ context.Context, record app.OperationCleanupUncertain) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cleanup = append(f.cleanup, record)
	return nil
}

func newFakeWebOperations() *fakeWebOperations {
	return &fakeWebOperations{started: make(chan struct{}), release: make(chan struct{}), done: make(chan struct{})}
}

func (f *fakeWebOperations) Validate(context.Context, app.ValidationRequest) (app.ValidationOperationResult, error) {
	return app.ValidationOperationResult{Status: "valid"}, nil
}

func (f *fakeWebOperations) PrepareOperation(_ context.Context, req app.OperationRequest) (app.Confirmation, error) {
	canonical := string(req.Kind) + ":" + req.Project + ":" + req.Sprint + ":" + req.Study + ":" + req.Stage
	fingerprint := "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	req.ExpectedFingerprint = fingerprint
	return app.Confirmation{
		Request: req, Subject: req.Project + "/" + req.Sprint, Paths: []string{"projects/" + req.Project + "/sprints/" + req.Sprint},
		MutationClass: "sprint_mutation", CanonicalRequest: canonical, InputFingerprint: fingerprint,
		Prerequisites: []string{"workspace readable"}, DurableRefreshPath: "/api/v1/projects/" + req.Project + "/sprints/" + req.Sprint,
	}, nil
}

func (f *fakeWebOperations) RunOperation(ctx context.Context, _ app.OperationRequest, emit func(app.OperationEvent)) (app.OperationResult, error) {
	f.mu.Lock()
	f.runs++
	f.mu.Unlock()
	f.once.Do(func() { close(f.started) })
	emit(app.OperationEvent{State: app.OperationRunning, Stage: "plan", Message: "working"})
	defer close(f.done)
	select {
	case <-f.release:
		return app.OperationResult{State: app.OperationComplete, Message: "complete"}, nil
	case <-ctx.Done():
		return app.OperationResult{State: app.OperationCancelled, Message: "cancelled"}, ctx.Err()
	}
}

func TestOperationHubLifecycleCancellationAndSessionOwnership(t *testing.T) {
	ops := newFakeWebOperations()
	now := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	hub := newOperationHub(context.Background(), ops, func() time.Time { return now }, func() string { return "id" })
	prepared, _ := ops.PrepareOperation(context.Background(), app.OperationRequest{Kind: app.OperationFlow, Project: "alpha", Sprint: "31-web", Stage: "plan"})
	doc, err := hub.start("session-a", prepared)
	if err != nil {
		t.Fatal(err)
	}
	<-ops.started
	if _, err := hub.status("session-b", doc.ID); !errors.Is(err, errOperationNotFound) {
		t.Fatalf("cross-session status error=%v", err)
	}
	cancelling, requested, err := hub.cancelOperation("session-a", doc.ID, "user_request")
	if err != nil || !requested || cancelling.State != "cancelling" || cancelling.Reason != "user_request" {
		t.Fatalf("cancel doc=%+v requested=%t err=%v", cancelling, requested, err)
	}
	<-ops.done
	terminal, err := hub.status("session-a", doc.ID)
	if err != nil || terminal.State != "cancelled" || terminal.Result == nil {
		t.Fatalf("terminal=%+v err=%v", terminal, err)
	}
	_, requested, err = hub.cancelOperation("session-a", doc.ID, "user_request")
	if err != nil || requested {
		t.Fatalf("idempotent cancel requested=%t err=%v", requested, err)
	}
}

func TestOperationHubDrainCancelsOwnedWorkAndRejectsNewStarts(t *testing.T) {
	ops := newFakeWebOperations()
	hub := newOperationHub(context.Background(), ops, time.Now, func() string { return "drain" })
	prepared, _ := ops.PrepareOperation(context.Background(), app.OperationRequest{Kind: app.OperationFlow, Project: "alpha", Sprint: "31-web", Stage: "plan"})
	if _, err := hub.start("session", prepared); err != nil {
		t.Fatal(err)
	}
	<-ops.started
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := hub.drainAndWait(ctx); err != nil {
		t.Fatal(err)
	}
	<-ops.done
	if _, err := hub.start("session", prepared); !errors.Is(err, errServerDraining) {
		t.Fatalf("start while draining error=%v", err)
	}
}

func TestOperationHubDeadlinePersistsCleanupUncertaintyBeforeTerminalProjection(t *testing.T) {
	ops := newDeadlineWebOperations()
	hub := newOperationHub(context.Background(), ops, time.Now, func() string { return "deadline" })
	prepared, _ := ops.PrepareOperation(context.Background(), app.OperationRequest{Kind: app.OperationFlow, Project: "alpha", Sprint: "31-web", Stage: "plan"})
	doc, err := hub.start("session", prepared)
	if err != nil {
		t.Fatal(err)
	}
	<-ops.startedDeadline
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := hub.drainAndWait(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("drain error=%v", err)
	}
	ops.mu.Lock()
	cleanup := append([]app.OperationCleanupUncertain(nil), ops.cleanup...)
	ops.mu.Unlock()
	if len(cleanup) != 1 || cleanup[0].OperationID != doc.ID || cleanup[0].Reason != "server_shutdown" || cleanup[0].Request.Project != "alpha" {
		t.Fatalf("cleanup records=%+v", cleanup)
	}
	terminal, err := hub.status("session", doc.ID)
	if err != nil || terminal.State != "cleanup_uncertain" {
		t.Fatalf("terminal=%+v err=%v", terminal, err)
	}
	close(ops.releaseDeadline)
	<-ops.finished
}

func TestOperationHubRejectsNinthActiveOperation(t *testing.T) {
	ops := newCapacityWebOperations()
	sequence := 0
	hub := newOperationHub(context.Background(), ops, time.Now, func() string {
		sequence++
		return fmt.Sprintf("capacity-%d", sequence)
	})
	prepared, _ := ops.PrepareOperation(context.Background(), app.OperationRequest{Kind: app.OperationFlow, Project: "alpha", Sprint: "31-web", Stage: "plan"})
	for i := 0; i < MaxActiveOperations; i++ {
		if _, err := hub.start("session", prepared); err != nil {
			t.Fatalf("start %d: %v", i+1, err)
		}
	}
	for i := 0; i < MaxActiveOperations; i++ {
		<-ops.startedMany
	}
	if _, err := hub.start("session", prepared); !errors.Is(err, errOperationCapacity) {
		t.Fatalf("ninth start error=%v, want %v", err, errOperationCapacity)
	}
	close(ops.releaseMany)
	for i := 0; i < MaxActiveOperations; i++ {
		<-ops.finished
	}
}

func TestOperationHubBoundsReplayAndSlowSubscriberIsolation(t *testing.T) {
	ops := newFakeWebOperations()
	sequence := 0
	hub := newOperationHub(context.Background(), ops, time.Now, func() string { sequence++; return string(rune('a' + sequence)) })
	prepared, _ := ops.PrepareOperation(context.Background(), app.OperationRequest{Kind: app.OperationFlow, Project: "alpha", Sprint: "31-web", Stage: "plan"})
	doc, err := hub.start("session", prepared)
	if err != nil {
		t.Fatal(err)
	}
	<-ops.started
	replay, events, unsubscribe, err := hub.subscribe("session", doc.ID, 0)
	if err != nil || len(replay) == 0 {
		t.Fatalf("replay=%d err=%v", len(replay), err)
	}
	defer unsubscribe()
	hub.mu.Lock()
	record := hub.records[doc.ID]
	for i := 0; i < SubscriberQueueSize+4; i++ {
		hub.appendEventLocked(record, "progress", map[string]any{"current": i})
	}
	_, stillSubscribed := record.subscribers[1]
	hub.mu.Unlock()
	if stillSubscribed {
		t.Fatal("slow subscriber was not disconnected")
	}
	select {
	case _, open := <-events:
		if !open {
			return
		}
	default:
	}
	close(ops.release)
	<-ops.done
}

func TestPreparationStoreBindingExpiryReplayAndCapacity(t *testing.T) {
	now := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	seq := 0
	store := newPreparationStore(func() time.Time { return now }, func() string { seq++; return string(rune('a' + seq)) })
	confirmation := app.Confirmation{CanonicalRequest: "canonical", InputFingerprint: "sha256:abc"}
	record, err := store.issue("session", confirmation)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.consume(record.Token, "other", "canonical", "sha256:abc"); !errors.Is(err, errConfirmationMismatch) {
		t.Fatalf("mismatch error=%v", err)
	}
	if _, err := store.consume(record.Token, "session", "canonical", "sha256:abc"); !errors.Is(err, errConfirmationReplayed) {
		t.Fatalf("replay error=%v", err)
	}
	record, _ = store.issue("session", confirmation)
	now = now.Add(PreparationTTL)
	if _, err := store.consume(record.Token, "session", "canonical", "sha256:abc"); !errors.Is(err, errConfirmationExpired) {
		t.Fatalf("expiry error=%v", err)
	}
	now = time.Date(2026, 8, 16, 13, 0, 0, 0, time.UTC)
	store = newPreparationStore(func() time.Time { return now }, func() string { seq++; return string(rune(1000 + seq)) })
	for i := 0; i < MaxPreparations; i++ {
		if _, err := store.issue("session", confirmation); err != nil {
			t.Fatalf("issue %d: %v", i, err)
		}
	}
	if _, err := store.issue("session", confirmation); !errors.Is(err, errOperationCapacity) {
		t.Fatalf("capacity error=%v", err)
	}
}

func TestOperationHTTPPrepareStartSSEAndCancel(t *testing.T) {
	ops := newFakeWebOperations()
	h := operationTestHandler(t, ops)
	cookie, csrf := establishOperationSession(t, h)
	prepareBody := `{"operation":{"kind":"sprint_flow","scope":{"project":"alpha","sprint":"31-web"},"options":{"to_stage":"plan"}}}`
	prepared := operationMutationRequest(h, http.MethodPost, "/api/v1/operations/prepare", prepareBody, cookie, csrf)
	if prepared.Code != http.StatusOK {
		t.Fatalf("prepare status=%d body=%s", prepared.Code, prepared.Body.String())
	}
	var prepareEnvelope struct {
		Data preparationDTO `json:"data"`
	}
	if err := json.Unmarshal(prepared.Body.Bytes(), &prepareEnvelope); err != nil {
		t.Fatal(err)
	}
	if prepareEnvelope.Data.ConfirmationToken == "" || prepareEnvelope.Data.InputFingerprint == "" {
		t.Fatalf("preparation=%+v", prepareEnvelope.Data)
	}
	startBody := `{"operation":{"kind":"sprint_flow","scope":{"project":"alpha","sprint":"31-web"},"options":{"to_stage":"plan"}},"confirmation_token":"` + prepareEnvelope.Data.ConfirmationToken + `"}`
	started := operationMutationRequest(h, http.MethodPost, "/api/v1/operations", startBody, cookie, csrf)
	if started.Code != http.StatusAccepted || started.Header().Get("Location") == "" {
		t.Fatalf("start status=%d headers=%v body=%s", started.Code, started.Header(), started.Body.String())
	}
	<-ops.started
	var startedEnvelope struct {
		Data operationDocument `json:"data"`
	}
	if err := json.Unmarshal(started.Body.Bytes(), &startedEnvelope); err != nil {
		t.Fatal(err)
	}

	sseDone := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/operations/"+startedEnvelope.Data.ID+"/events", nil)
		req.Host = testAuthority
		req.AddCookie(cookie)
		res := httptest.NewRecorder()
		h.ServeHTTP(res, req)
		sseDone <- res
	}()
	cancelled := operationMutationRequest(h, http.MethodDelete, "/api/v1/operations/"+startedEnvelope.Data.ID, "", cookie, csrf)
	if cancelled.Code != http.StatusAccepted {
		t.Fatalf("cancel status=%d body=%s", cancelled.Code, cancelled.Body.String())
	}
	<-ops.done
	sse := <-sseDone
	if sse.Code != http.StatusOK || !strings.Contains(sse.Body.String(), "event: snapshot") || !strings.Contains(sse.Body.String(), "event: terminal") {
		t.Fatalf("sse status=%d body=%s", sse.Code, sse.Body.String())
	}
	status := operationSessionRequest(h, http.MethodGet, "/api/v1/operations/"+startedEnvelope.Data.ID, cookie)
	if status.Code != http.StatusOK || !strings.Contains(status.Body.String(), `"state":"cancelled"`) {
		t.Fatalf("status=%d body=%s", status.Code, status.Body.String())
	}
	replay := operationMutationRequest(h, http.MethodPost, "/api/v1/operations", startBody, cookie, csrf)
	if replay.Code != http.StatusConflict || !strings.Contains(replay.Body.String(), "confirmation_replayed") {
		t.Fatalf("replay status=%d body=%s", replay.Code, replay.Body.String())
	}
}

func TestOperationHTTPStrictJSONAndCSRF(t *testing.T) {
	ops := newFakeWebOperations()
	h := operationTestHandler(t, ops)
	cookie, csrf := establishOperationSession(t, h)
	unknown := operationMutationRequest(h, http.MethodPost, "/api/v1/operations/prepare", `{"operation":{"kind":"validation","scope":{"project":"alpha"}},"fingerprint":"caller"}`, cookie, csrf)
	if unknown.Code != http.StatusBadRequest || !strings.Contains(unknown.Body.String(), "invalid_request") {
		t.Fatalf("unknown status=%d body=%s", unknown.Code, unknown.Body.String())
	}
	badCSRF := operationMutationRequest(h, http.MethodPost, "/api/v1/operations/prepare", `{"operation":{"kind":"validation","scope":{"project":"alpha"}}}`, cookie, "wrong")
	if badCSRF.Code != http.StatusForbidden || !strings.Contains(badCSRF.Body.String(), "csrf_failed") {
		t.Fatalf("csrf status=%d body=%s", badCSRF.Code, badCSRF.Body.String())
	}
}

func TestOperationRequestAllowlistSupportsProjectSprintAndStudyValidation(t *testing.T) {
	for _, spec := range []operationSpecRequest{
		{Kind: "validation", Scope: operationScopeRequest{Project: "alpha"}},
		{Kind: "validation", Scope: operationScopeRequest{Project: "alpha", Sprint: "31-web"}, Options: operationOptionsRequest{ToStage: "plan"}},
		{Kind: "validation", Scope: operationScopeRequest{Study: "research"}},
	} {
		if req, err := mapOperationRequest(spec); err != nil || req.Kind != app.OperationValidate {
			t.Fatalf("spec=%+v req=%+v err=%v", spec, req, err)
		}
	}
}

func TestOperationProjectionRedactsBeforeRetention(t *testing.T) {
	result := projectOperationResult(app.OperationResult{
		State: app.OperationFailed, Message: "token=super-secret /home/user/private",
		Content: "authorization: bearer-secret", Findings: []app.DisplayFinding{{Severity: "error", Problem: "/home/user/private"}},
		Error: &app.OperationError{Code: "validation.reference", Category: "validation", Cause: "secret=hidden", Guidance: "Complete execute evidence before review."},
	})
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, forbidden := range []string{"super-secret", "bearer-secret", "/home/user"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("projection retained %q: %s", forbidden, text)
		}
	}
	if result.Error == nil || result.Error.Details["guidance"] != "Complete execute evidence before review." {
		t.Fatalf("safe failure guidance was not retained: %+v", result.Error)
	}
}

func TestOperationErrorResponseIncludesSafeCauseAndGuidance(t *testing.T) {
	h := &handler{now: func() time.Time { return time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC) }}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/operations/prepare", nil)
	req = req.WithContext(context.WithValue(req.Context(), requestIDKey{}, "reason-id"))
	res := httptest.NewRecorder()
	h.writeOperationError(res, req, errors.New("execute evidence is incomplete: task implementation is failed"))
	body := res.Body.String()
	for _, want := range []string{`"reason":"execute evidence is incomplete`, `"guidance":"`, `"code":"validation_failed"`} {
		if !strings.Contains(body, want) {
			t.Errorf("response missing %q: %s", want, body)
		}
	}
}

func TestServerRenderedOperationFlowWorksWithoutJavaScript(t *testing.T) {
	ops := newFakeWebOperations()
	h := operationTestHandler(t, ops)
	cookie, csrf := establishOperationSession(t, h)
	prepare := url.Values{"_csrf": {csrf}, "kind": {"sprint_flow"}, "project": {"alpha"}, "sprint": {"31-web"}, "stage": {"plan"}}
	prepared := operationFormRequest(h, "/operations/prepare", prepare, cookie)
	if prepared.Code != http.StatusOK || !strings.Contains(prepared.Body.String(), "Normalized impact") || !strings.Contains(prepared.Body.String(), "Confirm and start") {
		t.Fatalf("prepare status=%d body=%s", prepared.Code, prepared.Body.String())
	}
	match := regexp.MustCompile(`name="confirmation_token" value="([^"]+)"`).FindStringSubmatch(prepared.Body.String())
	if len(match) != 2 {
		t.Fatalf("confirmation token missing: %s", prepared.Body.String())
	}
	start := url.Values{"_csrf": {csrf}, "confirmation_token": {match[1]}, "kind": {"sprint_flow"}, "project": {"alpha"}, "sprint": {"31-web"}, "stage": {"plan"}}
	started := operationFormRequest(h, "/operations/start", start, cookie)
	if started.Code != http.StatusSeeOther || !strings.HasPrefix(started.Header().Get("Location"), "/operations/op_") {
		t.Fatalf("start status=%d headers=%v body=%s", started.Code, started.Header(), started.Body.String())
	}
	<-ops.started
	status := operationSessionRequest(h, http.MethodGet, started.Header().Get("Location"), cookie)
	if status.Code != http.StatusOK || !strings.Contains(status.Body.String(), "Run status") || !strings.Contains(status.Body.String(), "Run progress") || !strings.Contains(status.Body.String(), `data-operation-id="`) || !strings.Contains(status.Body.String(), "Refresh run status") || !strings.Contains(status.Body.String(), `method="post" action="`+started.Header().Get("Location")+`/cancel"`) {
		t.Fatalf("status=%d body=%s", status.Code, status.Body.String())
	}
	cancelled := operationFormRequest(h, started.Header().Get("Location")+"/cancel", url.Values{"_csrf": {csrf}}, cookie)
	if cancelled.Code != http.StatusSeeOther || cancelled.Header().Get("Location") != started.Header().Get("Location") {
		t.Fatalf("cancel status=%d headers=%v body=%s", cancelled.Code, cancelled.Header(), cancelled.Body.String())
	}
	<-ops.done
}

func TestSingleStageOperationMapping(t *testing.T) {
	for _, tc := range []struct {
		kind string
		want app.OperationKind
	}{
		{kind: "sprint-stage", want: app.OperationStage},
		{kind: "sprint-stage-dry-run", want: app.OperationStageDryRun},
	} {
		req, err := mapOperationRequest(operationSpecRequest{Kind: tc.kind, Scope: operationScopeRequest{Project: "alpha", Sprint: "31-web"}, Options: operationOptionsRequest{ToStage: "reasoning"}})
		if err != nil || req.Kind != tc.want || req.Stage != "reasoning" {
			t.Fatalf("kind=%s req=%+v err=%v", tc.kind, req, err)
		}
	}
}

func operationTestHandler(t *testing.T, operations app.WebOperations) http.Handler {
	t.Helper()
	h, err := NewHandler(HandlerOptions{
		Queries: sampleQueries(), Operations: operations, Authority: testAuthority,
		Now: time.Now, RequestID: randomRequestID,
	})
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func establishOperationSession(t *testing.T, h http.Handler) (*http.Cookie, string) {
	t.Helper()
	res := request(h, http.MethodGet, "/", nil)
	cookies := res.Result().Cookies()
	if len(cookies) == 0 || res.Header().Get("X-CSRF-Token") == "" {
		t.Fatalf("session headers=%v", res.Header())
	}
	return cookies[0], res.Header().Get("X-CSRF-Token")
}

func operationMutationRequest(h http.Handler, method, target, body string, cookie *http.Cookie, csrf string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, bytes.NewBufferString(body))
	req.Host = testAuthority
	req.Header.Set("Origin", "http://"+testAuthority)
	req.Header.Set("X-CSRF-Token", csrf)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	req.AddCookie(cookie)
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	return res
}

func operationSessionRequest(h http.Handler, method, target string, cookie *http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, nil)
	req.Host = testAuthority
	req.AddCookie(cookie)
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	return res
}

func operationFormRequest(h http.Handler, target string, values url.Values, cookie *http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(values.Encode()))
	req.Host = testAuthority
	req.Header.Set("Origin", "http://"+testAuthority)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	return res
}
