package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Antonio7098/ultraplan-go/internal/app"
)

const testAuthority = "127.0.0.1:8080"

func TestRouteInventoryHTMLAndAPI(t *testing.T) {
	h := testHandler(t, sampleQueries(), nil)
	tests := []struct {
		path, contentType string
	}{
		{"/", "text/html"},
		{"/projects", "text/html"},
		{"/projects/alpha", "text/html"},
		{"/projects/alpha/documentation", "text/html"},
		{"/projects/alpha/sprints", "text/html"},
		{"/projects/alpha/operations", "text/html"},
		{"/projects/alpha/validation", "text/html"},
		{"/projects/alpha/artifacts", "text/html"},
		{"/projects/alpha/sprints/30-web", "text/html"},
		{"/projects/alpha/sprints/30-web/plan", "text/html"},
		{"/projects/alpha/sprints/30-web/delivery", "text/html"},
		{"/projects/alpha/sprints/30-web/operations", "text/html"},
		{"/projects/alpha/sprints/30-web/validation", "text/html"},
		{"/projects/alpha/sprints/30-web/artifacts", "text/html"},
		{"/projects/alpha/sprints/30-web/artifacts/artifact_ref", "text/html"},
		{"/studies", "text/html"},
		{"/studies/research", "text/html"},
		{"/studies/research/inputs", "text/html"},
		{"/studies/research/progress", "text/html"},
		{"/studies/research/operations", "text/html"},
		{"/studies/research/validation", "text/html"},
		{"/studies/research/artifacts", "text/html"},
		{"/artifacts/artifact_ref", "text/html"},
		{"/api/v1/dashboard", "application/json"},
		{"/api/v1/projects", "application/json"},
		{"/api/v1/projects/alpha", "application/json"},
		{"/api/v1/projects/alpha/sprints/30-web", "application/json"},
		{"/api/v1/studies", "application/json"},
		{"/api/v1/studies/research", "application/json"},
		{"/api/v1/validations?scope=project&ref=project_ref", "application/json"},
		{"/api/v1/artifacts/artifact_ref", "application/json"},
		{"/api/v1/health", "application/json"},
		{"/static/app.css", "text/css"},
		{"/static/app.js", "text/javascript"},
	}
	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			res := request(h, http.MethodGet, tc.path, nil)
			if res.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
			}
			if !strings.HasPrefix(res.Header().Get("Content-Type"), tc.contentType) {
				t.Fatalf("content-type=%q", res.Header().Get("Content-Type"))
			}
		})
	}
}

func TestAPISuccessEnvelopeAndCollectionMetadata(t *testing.T) {
	h := testHandler(t, sampleQueries(), nil)
	res := request(h, http.MethodGet, "/api/v1/projects", nil)
	var payload struct {
		Data []map[string]any `json:"data"`
		Meta struct {
			Version   string `json:"api_version"`
			RequestID string `json:"request_id"`
			Generated string `json:"generated_at"`
			Returned  int    `json:"returned_count"`
			Total     int    `json:"total_count"`
			Truncated bool   `json:"truncated"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Data) != 1 || payload.Meta.Version != "v1" || payload.Meta.RequestID != "request-id" ||
		payload.Meta.Generated == "" || payload.Meta.Returned != 1 || payload.Meta.Total != 1 || payload.Meta.Truncated {
		t.Fatalf("payload=%+v", payload)
	}
}

func TestMethodHeadAndUnknownAPIRouting(t *testing.T) {
	h := testHandler(t, sampleQueries(), nil)
	get := request(h, http.MethodGet, "/api/v1/projects", nil)
	head := request(h, http.MethodHead, "/api/v1/projects", nil)
	if head.Code != http.StatusOK || head.Body.Len() != 0 ||
		head.Header().Get("Content-Type") != get.Header().Get("Content-Type") ||
		head.Header().Get("Content-Length") != get.Header().Get("Content-Length") {
		t.Fatalf("HEAD status=%d headers=%v body=%q", head.Code, head.Header(), head.Body.String())
	}
	method := request(h, http.MethodPost, "/api/v1/projects", nil)
	if method.Code != http.StatusMethodNotAllowed || method.Header().Get("Allow") != "GET, HEAD" {
		t.Fatalf("method status=%d allow=%q body=%s", method.Code, method.Header().Get("Allow"), method.Body.String())
	}
	for _, path := range []string{"/api/v2/projects", "/api/unknown", "/api/v1/operations/id/unknown"} {
		res := request(h, http.MethodGet, path, nil)
		if res.Code != http.StatusNotFound || !strings.Contains(res.Header().Get("Content-Type"), "application/json") ||
			!strings.Contains(res.Body.String(), `"code":"not_found"`) {
			t.Fatalf("%s status=%d type=%q body=%s", path, res.Code, res.Header().Get("Content-Type"), res.Body.String())
		}
	}
	html := request(h, http.MethodGet, "/unknown", nil)
	if html.Code != http.StatusNotFound || !strings.Contains(html.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("html status=%d type=%q", html.Code, html.Header().Get("Content-Type"))
	}
}

func TestDashboardCountsUseStableSnakeCaseFields(t *testing.T) {
	h := testHandler(t, sampleQueries(), nil)
	res := request(h, http.MethodGet, "/api/v1/dashboard", nil)
	body := res.Body.String()
	for _, want := range []string{`"returned_count":1`, `"total_count":1`, `"truncated":false`} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in %s", want, body)
		}
	}
	if strings.Contains(body, "ReturnedCount") || strings.Contains(body, "TotalCount") {
		t.Fatalf("app field names leaked: %s", body)
	}
}

func TestAPIValidationAndIdentifierBoundaries(t *testing.T) {
	h := testHandler(t, sampleQueries(), nil)
	for _, path := range []string{
		"/api/v1/projects/bad%20value",
		"/api/v1/artifacts/..",
		"/api/v1/projects?unknown=1",
		"/api/v1/validations",
		"/api/v1/validations?scope=project&ref=project_ref&extra=x",
		"/api/v1/validations?scope=invalid&ref=project_ref",
		"/api/v1/validations?scope=project&scope=study&ref=project_ref",
	} {
		res := request(h, http.MethodGet, path, nil)
		if res.Code != http.StatusBadRequest || !strings.Contains(res.Body.String(), `"code":"invalid_request"`) {
			t.Fatalf("%s status=%d body=%s", path, res.Code, res.Body.String())
		}
	}
}

func TestAPIErrorProjectionDoesNotLeakInternalCause(t *testing.T) {
	queries := sampleQueries()
	for _, tc := range []struct {
		err    error
		status int
		code   string
	}{
		{app.ErrWebNotFound, http.StatusNotFound, "not_found"},
		{app.ErrWebUnavailable, http.StatusServiceUnavailable, "unavailable"},
		{errors.New("secret=/home/user/token"), http.StatusInternalServerError, "internal_error"},
	} {
		queries.err = tc.err
		h := testHandler(t, queries, nil)
		res := request(h, http.MethodGet, "/api/v1/projects", nil)
		if res.Code != tc.status || !strings.Contains(res.Body.String(), `"code":"`+tc.code+`"`) ||
			strings.Contains(res.Body.String(), "secret") || strings.Contains(res.Body.String(), "/home/") {
			t.Fatalf("err=%v status=%d body=%s", tc.err, res.Code, res.Body.String())
		}
	}
}

type blockingDashboardQueries struct {
	*fakeQueries
	started chan struct{}
	once    sync.Once
}

func (q *blockingDashboardQueries) Dashboard(ctx context.Context) (app.WebDashboardResult, error) {
	q.once.Do(func() { close(q.started) })
	<-ctx.Done()
	return app.WebDashboardResult{}, ctx.Err()
}

func TestRouteRequestCancellationReachesAppQuery(t *testing.T) {
	queries := &blockingDashboardQueries{fakeQueries: sampleQueries(), started: make(chan struct{})}
	h, err := NewHandler(HandlerOptions{Queries: queries, Authority: testAuthority, RequestID: func() string { return "cancel-id" }})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard", nil).WithContext(ctx)
	req.Host = testAuthority
	res := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		h.ServeHTTP(res, req)
		close(done)
	}()
	<-queries.started
	cancel()
	<-done
	if res.Code != http.StatusServiceUnavailable || !strings.Contains(res.Body.String(), `"code":"unavailable"`) {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
}

func testHandler(t *testing.T, queries *fakeQueries, diagnostics *bytes.Buffer) http.Handler {
	t.Helper()
	var diag bytes.Buffer
	if diagnostics == nil {
		diagnostics = &diag
	}
	h, err := NewHandler(HandlerOptions{
		Queries: queries, Authority: testAuthority, Diagnostics: diagnostics,
		Now:       func() time.Time { return time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC) },
		RequestID: func() string { return "request-id" },
	})
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func request(handler http.Handler, method, target string, body *bytes.Reader) *httptest.ResponseRecorder {
	var req *http.Request
	if body == nil {
		req = httptest.NewRequest(method, target, nil)
	} else {
		req = httptest.NewRequest(method, target, body)
	}
	req.Host = testAuthority
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	return res
}
