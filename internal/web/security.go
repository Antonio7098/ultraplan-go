package web

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"
)

const (
	MaxBodyBytes       = 64 * 1024
	MaxRequestTarget   = 8 * 1024
	MaxIdentifierBytes = 128
)

type requestIDKey struct{}

type trackedWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (w *trackedWriter) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *trackedWriter) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	n, err := w.ResponseWriter.Write(data)
	w.bytes += n
	return n, err
}

type securityMiddleware struct {
	authority   string
	origin      string
	diagnostics io.Writer
	now         func() time.Time
	requestID   func() string
	sem         chan struct{}
	active      atomic.Int64
}

func newSecurityMiddleware(authority string, diagnostics io.Writer, now func() time.Time, requestID func() string) *securityMiddleware {
	if diagnostics == nil {
		diagnostics = io.Discard
	}
	if now == nil {
		now = time.Now
	}
	if requestID == nil {
		requestID = randomRequestID
	}
	return &securityMiddleware{
		authority:   authority,
		origin:      "http://" + authority,
		diagnostics: diagnostics,
		now:         now,
		requestID:   requestID,
		sem:         make(chan struct{}, MaxInFlight),
	}
}

func (m *securityMiddleware) wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := m.now()
		tracked := &trackedWriter{ResponseWriter: w}
		applySecurityHeaders(tracked.Header())
		id := m.requestID()
		tracked.Header().Set("X-Request-ID", id)
		r = r.WithContext(context.WithValue(r.Context(), requestIDKey{}, id))

		select {
		case m.sem <- struct{}{}:
			m.active.Add(1)
			defer func() {
				m.active.Add(-1)
				<-m.sem
			}()
		case <-r.Context().Done():
			writePolicyError(tracked, r, http.StatusServiceUnavailable, "unavailable", "The service is unavailable.")
			return
		}

		switch {
		case len(r.RequestURI) > MaxRequestTarget:
			writePolicyError(tracked, r, http.StatusBadRequest, "invalid_request", "The request target is too long.")
		case r.Host != m.authority:
			writePolicyError(tracked, r, http.StatusForbidden, "request_rejected", "The request was rejected.")
		case !validOrigin(r.Header.Get("Origin"), m.origin):
			writePolicyError(tracked, r, http.StatusForbidden, "request_rejected", "The request was rejected.")
		case r.ContentLength > MaxBodyBytes:
			writePolicyError(tracked, r, http.StatusRequestEntityTooLarge, "request_too_large", "The request is too large.")
		case r.ContentLength > 0 || len(r.TransferEncoding) > 0:
			writePolicyError(tracked, r, http.StatusBadRequest, "invalid_request", "Request bodies are not accepted.")
		default:
			next.ServeHTTP(tracked, r)
		}
		if tracked.status == 0 {
			tracked.status = http.StatusOK
		}
		fmt.Fprintf(m.diagnostics,
			"event=http_request request_id=%s route=%s method=%s status=%d duration_ms=%d response_bytes=%d\n",
			id, normalizedRoute(r.URL.Path), r.Method, tracked.status, m.now().Sub(started).Milliseconds(), tracked.bytes)
	})
}

func applySecurityHeaders(header http.Header) {
	header.Set("Cache-Control", "no-store")
	header.Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'none'")
	header.Set("Referrer-Policy", "no-referrer")
	header.Set("X-Content-Type-Options", "nosniff")
	header.Set("X-Frame-Options", "DENY")
	header.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
}

func validOrigin(origin, expected string) bool {
	if origin == "" {
		return true
	}
	if origin == "null" || origin != expected {
		return false
	}
	parsed, err := url.Parse(origin)
	return err == nil && parsed.Scheme == "http" && parsed.Host != "" &&
		parsed.User == nil && parsed.Path == "" && parsed.RawQuery == "" && parsed.Fragment == ""
}

func requestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

func randomRequestID() string {
	var data [16]byte
	if _, err := rand.Read(data[:]); err != nil {
		return "00000000000000000000000000000000"
	}
	return hex.EncodeToString(data[:])
}

func normalizedRoute(path string) string {
	switch {
	case path == "/":
		return "dashboard"
	case path == "/projects":
		return "projects"
	case strings.HasPrefix(path, "/projects/") && strings.Contains(path, "/sprints/"):
		return "sprint_detail"
	case strings.HasPrefix(path, "/projects/"):
		return "project_detail"
	case path == "/studies":
		return "studies"
	case strings.HasPrefix(path, "/studies/"):
		return "study_detail"
	case strings.HasPrefix(path, "/artifacts/"):
		return "artifact"
	case strings.HasPrefix(path, "/api/v1/artifacts/"):
		return "api_artifact"
	case strings.HasPrefix(path, "/api/v1/"):
		return "api_v1"
	case strings.HasPrefix(path, "/api/"):
		return "api_unknown"
	case strings.HasPrefix(path, "/static/"):
		return "static"
	default:
		return "not_found"
	}
}
