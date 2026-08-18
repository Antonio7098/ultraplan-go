package web

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Antonio7098/ultraplan-go/internal/app"
)

const (
	MaxBodyBytes       = 64 * 1024
	MaxRequestTarget   = 8 * 1024
	MaxIdentifierBytes = 128
)

type requestIDKey struct{}
type sessionIDKey struct{}
type csrfTokenKey struct{}

type trackedWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (w *trackedWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

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

func (w *trackedWriter) Flush() {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

type securityMiddleware struct {
	authority   string
	origin      string
	diagnostics io.Writer
	now         func() time.Time
	requestID   func() string
	sem         chan struct{}
	active      atomic.Int64
	secret      [32]byte
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
	m := &securityMiddleware{
		authority:   authority,
		origin:      "http://" + authority,
		diagnostics: diagnostics,
		now:         now,
		requestID:   requestID,
		sem:         make(chan struct{}, MaxInFlight),
	}
	if _, err := rand.Read(m.secret[:]); err != nil {
		m.secret = sha256.Sum256([]byte(authority + now().UTC().Format(time.RFC3339Nano)))
	}
	return m
}

func (m *securityMiddleware) wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := m.now()
		tracked := &trackedWriter{ResponseWriter: w}
		applySecurityHeaders(tracked.Header())
		id := m.requestID()
		tracked.Header().Set("X-Request-ID", id)
		r = r.WithContext(context.WithValue(r.Context(), requestIDKey{}, id))
		session, validSession := m.readSession(r)
		if !validSession {
			session = randomRequestID()
			http.SetCookie(tracked, &http.Cookie{Name: "ultraplan_session", Value: m.signSession(session), Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: 3600})
		}
		csrf := m.csrfFor(session)
		tracked.Header().Set("X-CSRF-Token", csrf)
		ctx := context.WithValue(r.Context(), sessionIDKey{}, session)
		ctx = context.WithValue(ctx, csrfTokenKey{}, csrf)
		r = r.WithContext(ctx)

		select {
		case m.sem <- struct{}{}:
			m.active.Add(1)
			defer func() {
				m.active.Add(-1)
				<-m.sem
			}()
		case <-r.Context().Done():
			m.reject(tracked, r, http.StatusServiceUnavailable, "unavailable", "The service is unavailable.")
			return
		}

		apiOperationMutation := (r.Method == http.MethodPost || r.Method == http.MethodDelete) && strings.HasPrefix(r.URL.Path, "/api/v1/operations")
		htmlOperationMutation := r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/operations/")
		operationMutation := apiOperationMutation || htmlOperationMutation
		matchedRoute := matchRoute(r.URL.Path)
		operationRead := (r.Method == http.MethodGet || r.Method == http.MethodHead) && (matchedRoute.name == "api_operation" || matchedRoute.name == "api_operation_events")
		staticAsset := (r.Method == http.MethodGet || r.Method == http.MethodHead) && strings.HasPrefix(r.URL.Path, "/static/")
		hasBody := r.ContentLength > 0 || len(r.TransferEncoding) > 0
		operationBody := r.Method == http.MethodPost && (matchedRoute.name == "api_operation_prepare" || matchedRoute.name == "api_operations" || matchedRoute.name == "operation_prepare" || matchedRoute.name == "operation_start" || matchedRoute.name == "operation_cancel")
		switch {
		case len(r.RequestURI) > MaxRequestTarget:
			m.reject(tracked, r, http.StatusBadRequest, "invalid_request", "The request target is too long.")
		case r.Host != m.authority:
			m.reject(tracked, r, http.StatusForbidden, "host_rejected", hostRejectionMessage(m.authority, r.Host))
		case operationMutation && !validSession:
			m.reject(tracked, r, http.StatusForbidden, "session_required", "Establish a browser session before submitting commands.")
		case operationMutation && !validCommandOrigin(r.Header.Get("Origin"), m.origin):
			m.reject(tracked, r, http.StatusForbidden, "origin_rejected", originRejectionMessage(m.origin, r.Header.Get("Origin")))
		case apiOperationMutation && subtle.ConstantTimeCompare([]byte(r.Header.Get("X-CSRF-Token")), []byte(csrf)) != 1:
			m.reject(tracked, r, http.StatusForbidden, "csrf_failed", "The CSRF proof did not match this browser session. Refresh the UltraPlan page and prepare the operation again.")
		case operationRead && !validSession:
			m.reject(tracked, r, http.StatusForbidden, "session_required", "The operation stream belongs to a browser session that is no longer available. Refresh the owning UltraPlan page.")
		case operationRead && !validOperationReadOrigin(r.Header.Get("Origin"), m.origin):
			m.reject(tracked, r, http.StatusForbidden, "origin_rejected", originRejectionMessage(m.origin, r.Header.Get("Origin")))
		case !operationMutation && !operationRead && !staticAsset && !validOrigin(r.Header.Get("Origin"), m.origin):
			m.reject(tracked, r, http.StatusForbidden, "origin_rejected", originRejectionMessage(m.origin, r.Header.Get("Origin")))
		case r.ContentLength > MaxBodyBytes:
			m.reject(tracked, r, http.StatusRequestEntityTooLarge, "request_too_large", "The request is too large.")
		case hasBody && !operationBody:
			m.reject(tracked, r, http.StatusBadRequest, "invalid_request", "Request bodies are not accepted.")
		default:
			if operationBody {
				r.Body = http.MaxBytesReader(tracked, r.Body, MaxBodyBytes)
			}
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

func (m *securityMiddleware) reject(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	fmt.Fprintf(m.diagnostics, "event=security_rejection request_id=%s route=%s method=%s status=%d code=%s\n", requestID(r.Context()), normalizedRoute(r.URL.Path), r.Method, status, code)
	writePolicyError(w, r, status, code, message)
}

func originRejectionMessage(expected, received string) string {
	return fmt.Sprintf("The browser sent the request from %s, but this server only accepts commands from %s. Open the exact Dashboard URL printed by UltraPlan, refresh it, and retry.", displayOrigin(received), expected)
}

func hostRejectionMessage(expected, received string) string {
	return fmt.Sprintf("The request used host %s, but this server is listening as %s. Open the exact Dashboard URL printed by UltraPlan.", displayAuthority(received), expected)
}

func displayOrigin(value string) string {
	if value == "" {
		return "a request with no Origin header"
	}
	if value == "null" {
		return "the opaque origin 'null'"
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil {
		return "an invalid origin"
	}
	return parsed.Scheme + "://" + displayAuthority(parsed.Host)
}

func displayAuthority(value string) string {
	value = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, value)
	if len(value) > 256 {
		value = value[:256] + "…"
	}
	if value == "" {
		return "an empty host"
	}
	return value
}

func (m *securityMiddleware) signSession(session string) string {
	mac := hmac.New(sha256.New, m.secret[:])
	_, _ = mac.Write([]byte("session:" + session))
	return session + "." + hex.EncodeToString(mac.Sum(nil))
}

func (m *securityMiddleware) readSession(r *http.Request) (string, bool) {
	cookie, err := r.Cookie("ultraplan_session")
	if err != nil {
		return "", false
	}
	parts := strings.Split(cookie.Value, ".")
	if len(parts) != 2 || len(parts[0]) != 32 {
		return "", false
	}
	expected := m.signSession(parts[0])
	return parts[0], subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(expected)) == 1
}

func (m *securityMiddleware) csrfFor(session string) string {
	mac := hmac.New(sha256.New, m.secret[:])
	_, _ = mac.Write([]byte("csrf:" + session))
	return hex.EncodeToString(mac.Sum(nil))
}

func applySecurityHeaders(header http.Header) {
	header.Set("Cache-Control", "no-store")
	header.Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
	header.Set("Referrer-Policy", "same-origin")
	header.Set("X-Content-Type-Options", "nosniff")
	header.Set("X-Frame-Options", "DENY")
	header.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
}

func validOrigin(origin, expected string) bool {
	if origin == "" {
		return true
	}
	return validCommandOrigin(origin, expected)
}

// validCommandOrigin permits a browser shell or local reverse proxy to omit or
// rewrite only the port while preserving the exact numeric loopback address.
// Mutating requests still require the signed session and CSRF proof.
func validCommandOrigin(origin, expected string) bool {
	if origin == expected {
		return true
	}
	actualURL, actualIP, actualOK := numericLoopbackOrigin(origin)
	expectedURL, expectedIP, expectedOK := numericLoopbackOrigin(expected)
	return actualOK && expectedOK && actualURL.Scheme == expectedURL.Scheme && actualIP.Equal(expectedIP)
}

func validOperationReadOrigin(origin, expected string) bool {
	return origin == "" || validCommandOrigin(origin, expected)
}

func numericLoopbackOrigin(value string) (*url.URL, net.IP, bool) {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "http" || parsed.Host == "" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, nil, false
	}
	ip := net.ParseIP(parsed.Hostname())
	if ip == nil || !ip.IsLoopback() {
		return nil, nil, false
	}
	return parsed, ip, true
}

func requestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

func sessionID(ctx context.Context) string {
	id, _ := ctx.Value(sessionIDKey{}).(string)
	return id
}

func csrfToken(ctx context.Context) string {
	token, _ := ctx.Value(csrfTokenKey{}).(string)
	return token
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

type preparationRecord struct {
	ID           string
	Token        string
	Session      string
	Canonical    string
	Fingerprint  string
	Confirmation app.Confirmation
	ExpiresAt    time.Time
	Consumed     bool
}

type preparationStore struct {
	mu      sync.Mutex
	now     func() time.Time
	id      func() string
	records map[string]*preparationRecord
}

func newPreparationStore(now func() time.Time, id func() string) *preparationStore {
	if now == nil {
		now = time.Now
	}
	if id == nil {
		id = randomRequestID
	}
	return &preparationStore{now: now, id: id, records: make(map[string]*preparationRecord)}
}

func (s *preparationStore) issue(session string, confirmation app.Confirmation) (*preparationRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reapLocked()
	if len(s.records) >= MaxPreparations {
		return nil, errOperationCapacity
	}
	record := &preparationRecord{
		ID: "prep_" + s.id(), Token: "confirm_" + s.id(), Session: session,
		Canonical: confirmation.CanonicalRequest, Fingerprint: confirmation.InputFingerprint,
		Confirmation: confirmation, ExpiresAt: s.now().UTC().Add(PreparationTTL),
	}
	s.records[record.Token] = record
	copy := *record
	return &copy, nil
}

func (s *preparationStore) consume(token, session, canonical, fingerprint string) (app.Confirmation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record := s.records[token]
	if record == nil {
		return app.Confirmation{}, errConfirmationReplayed
	}
	if record.Consumed {
		return app.Confirmation{}, errConfirmationReplayed
	}
	if !s.now().UTC().Before(record.ExpiresAt) {
		record.Consumed = true
		return app.Confirmation{}, errConfirmationExpired
	}
	if record.Session != session || record.Canonical != canonical {
		record.Consumed = true
		return app.Confirmation{}, errConfirmationMismatch
	}
	if record.Fingerprint != fingerprint {
		record.Consumed = true
		return app.Confirmation{}, errStaleConfirmation
	}
	record.Consumed = true
	return record.Confirmation, nil
}

func (s *preparationStore) reapLocked() {
	now := s.now().UTC()
	for token, record := range s.records {
		if !now.Before(record.ExpiresAt) {
			delete(s.records, token)
		}
	}
}

var (
	errConfirmationExpired  = errors.New("confirmation expired")
	errConfirmationMismatch = errors.New("confirmation mismatch")
	errConfirmationReplayed = errors.New("confirmation replayed")
	errStaleConfirmation    = errors.New("stale confirmation")
)
