package web

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSecurityHostOriginNoCORSAndHeaders(t *testing.T) {
	h := testHandler(t, sampleQueries(), nil)
	tests := []struct {
		name, host, origin string
		want               int
	}{
		{"absent origin", testAuthority, "", http.StatusOK},
		{"exact origin", testAuthority, "http://" + testAuthority, http.StatusOK},
		{"wrong host", "localhost:8080", "", http.StatusForbidden},
		{"cross origin", testAuthority, "http://127.0.0.1:9999", http.StatusForbidden},
		{"null origin", testAuthority, "null", http.StatusForbidden},
		{"malformed origin", testAuthority, "://bad", http.StatusForbidden},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
			req.Host = tc.host
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			res := httptest.NewRecorder()
			h.ServeHTTP(res, req)
			if res.Code != tc.want {
				t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
			}
			for _, name := range []string{"Content-Security-Policy", "X-Content-Type-Options", "Referrer-Policy", "X-Frame-Options", "Cache-Control", "X-Request-ID"} {
				if res.Header().Get(name) == "" {
					t.Errorf("missing %s", name)
				}
			}
			if res.Header().Get("Access-Control-Allow-Origin") != "" {
				t.Errorf("unexpected CORS header")
			}
		})
	}
}

func TestSecurityAllowsKnownStaticAssetsWithOpaqueSubresourceOrigin(t *testing.T) {
	h := testHandler(t, sampleQueries(), nil)
	for _, path := range []string{"/static/app.css", "/static/app.js"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Host = testAuthority
		req.Header.Set("Origin", "null")
		res := httptest.NewRecorder()
		h.ServeHTTP(res, req)
		if res.Code != http.StatusOK {
			t.Fatalf("%s status=%d body=%s", path, res.Code, res.Body.String())
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	req.Host = testAuthority
	req.Header.Set("Origin", "null")
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusForbidden {
		t.Fatalf("API status=%d, want forbidden", res.Code)
	}
}

func TestSecurityBracketedIPv6AuthorityAndOrigin(t *testing.T) {
	h, err := NewHandler(HandlerOptions{Queries: sampleQueries(), Authority: "[::1]:8080", RequestID: func() string { return "ipv6-id" }})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "http://[::1]:8080/api/v1/health", nil)
	req.Host = "[::1]:8080"
	req.Header.Set("Origin", "http://[::1]:8080")
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
}

func TestSecurityBodyTargetAndRequestIDLimits(t *testing.T) {
	h := testHandler(t, sampleQueries(), nil)
	large := bytes.NewReader(make([]byte, MaxBodyBytes+1))
	res := request(h, http.MethodGet, "/api/v1/projects", large)
	if res.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("large body status=%d body=%s", res.Code, res.Body.String())
	}
	small := bytes.NewReader([]byte("x"))
	res = request(h, http.MethodGet, "/api/v1/projects", small)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("body status=%d body=%s", res.Code, res.Body.String())
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil)
	req.Host = testAuthority
	req.RequestURI = "/api/v1/projects?" + strings.Repeat("x", MaxRequestTarget)
	res = httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("target status=%d body=%s", res.Code, res.Body.String())
	}
	req = httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	req.Host = testAuthority
	req.Header.Set("X-Request-ID", "caller-secret-id")
	res = httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Header().Get("X-Request-ID") != "request-id" || strings.Contains(res.Body.String(), "caller-secret-id") {
		t.Fatalf("request id header=%q body=%s", res.Header().Get("X-Request-ID"), res.Body.String())
	}
}

func TestRequestDiagnosticsAreNormalizedAndRedacted(t *testing.T) {
	var diagnostics bytes.Buffer
	h := testHandler(t, sampleQueries(), &diagnostics)
	res := request(h, http.MethodGet, "/api/v1/validations?scope=project&ref=project_ref&secret=hunter2", nil)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", res.Code)
	}
	got := diagnostics.String()
	for _, leak := range []string{"hunter2", "project_ref", "scope=", testAuthority, "/api/v1/validations"} {
		if strings.Contains(got, leak) {
			t.Fatalf("diagnostic leaked %q: %s", leak, got)
		}
	}
	for _, want := range []string{"request_id=request-id", "route=api_v1", "method=GET", "status=400", "response_bytes="} {
		if !strings.Contains(got, want) {
			t.Fatalf("diagnostic missing %q: %s", want, got)
		}
	}
}
