package web

import (
	"context"
	"embed"
	"errors"
	"html/template"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Antonio7098/ultraplan-go/internal/app"
)

//go:embed templates/*.html static/*
var assets embed.FS

type HandlerOptions struct {
	Queries     app.WebQueries
	Operations  app.WebOperations
	Authority   string
	Diagnostics io.Writer
	Now         func() time.Time
	RequestID   func() string
	RootContext context.Context
	Hub         *operationHub
}

type handler struct {
	queries      app.WebQueries
	templates    *template.Template
	now          func() time.Time
	hub          *operationHub
	preparations *preparationStore
	diagnostics  io.Writer
}

func NewHandler(opts HandlerOptions) (http.Handler, error) {
	if opts.Queries == nil {
		return nil, errors.New("web queries are required")
	}
	templates, err := template.ParseFS(assets, "templates/*.html")
	if err != nil {
		return nil, err
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Diagnostics == nil {
		opts.Diagnostics = io.Discard
	}
	hub := opts.Hub
	if hub == nil {
		hub = newOperationHub(opts.RootContext, opts.Operations, opts.Now, opts.RequestID)
	}
	h := &handler{queries: opts.Queries, templates: templates, now: opts.Now, hub: hub, preparations: newPreparationStore(opts.Now, opts.RequestID), diagnostics: opts.Diagnostics}
	security := newSecurityMiddleware(opts.Authority, opts.Diagnostics, opts.Now, opts.RequestID)
	return security.wrap(h), nil
}

type routeMatch struct {
	name   string
	params []string
	known  bool
	api    bool
	static bool
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	match := matchRoute(r.URL.Path)
	if !match.known {
		if match.api {
			h.writeError(w, r, http.StatusNotFound, "not_found", "The requested resource was not found.")
		} else {
			h.renderError(w, r, http.StatusNotFound, "Page not found", "The requested page was not found.")
		}
		return
	}
	allowed := allowedMethods(match)
	if !methodAllowed(r.Method, allowed) {
		w.Header().Set("Allow", strings.Join(allowed, ", "))
		h.writeRouteError(w, r, match.api, http.StatusMethodNotAllowed, "method_not_allowed", "The method is not supported for this resource.")
		return
	}
	if r.Method == http.MethodHead {
		w = headResponseWriter{ResponseWriter: w}
	}
	if match.static {
		h.serveStatic(w, r, match.params[0])
		return
	}
	h.dispatch(w, r, match)
}

func allowedMethods(match routeMatch) []string {
	switch match.name {
	case "api_operation_prepare", "api_operations":
		return []string{http.MethodPost}
	case "operation_prepare", "operation_start":
		return []string{http.MethodPost}
	case "api_operation":
		return []string{http.MethodGet, http.MethodDelete}
	case "api_operation_events":
		return []string{http.MethodGet}
	default:
		return []string{http.MethodGet, http.MethodHead}
	}
}

func methodAllowed(method string, allowed []string) bool {
	for _, candidate := range allowed {
		if method == candidate {
			return true
		}
	}
	return false
}

func matchRoute(path string) routeMatch {
	if path == "/" {
		return routeMatch{name: "dashboard", known: true}
	}
	if path == "/projects" {
		return routeMatch{name: "projects", known: true}
	}
	if path == "/studies" {
		return routeMatch{name: "studies", known: true}
	}
	if path == "/api/v1/dashboard" {
		return routeMatch{name: "api_dashboard", known: true, api: true}
	}
	if path == "/api/v1/projects" {
		return routeMatch{name: "api_projects", known: true, api: true}
	}
	if path == "/api/v1/studies" {
		return routeMatch{name: "api_studies", known: true, api: true}
	}
	if path == "/api/v1/validations" {
		return routeMatch{name: "api_validations", known: true, api: true}
	}
	if path == "/api/v1/health" {
		return routeMatch{name: "api_health", known: true, api: true}
	}
	if path == "/api/v1/operations/prepare" {
		return routeMatch{name: "api_operation_prepare", known: true, api: true}
	}
	if path == "/api/v1/operations" {
		return routeMatch{name: "api_operations", known: true, api: true}
	}
	if path == "/operations/prepare" {
		return routeMatch{name: "operation_prepare", known: true}
	}
	if path == "/operations/start" {
		return routeMatch{name: "operation_start", known: true}
	}
	parts := splitPath(path)
	switch {
	case len(parts) == 4 && parts[0] == "projects" && (parts[2] == "documentation" || parts[2] == "artifacts"):
		return routeMatch{name: "project_artifact", params: []string{parts[1], parts[3]}, known: true}
	case len(parts) == 6 && parts[0] == "projects" && parts[2] == "sprints" && parts[4] == "artifacts":
		return routeMatch{name: "sprint_artifact", params: []string{parts[1], parts[3], parts[5]}, known: true}
	case len(parts) == 3 && parts[0] == "projects" && validProjectPage(parts[2]):
		return routeMatch{name: "project_page", params: []string{parts[1], parts[2]}, known: true}
	case len(parts) == 2 && parts[0] == "projects":
		return routeMatch{name: "project", params: parts[1:], known: true}
	case len(parts) == 5 && parts[0] == "projects" && parts[2] == "sprints" && validSprintPage(parts[4]):
		return routeMatch{name: "sprint_page", params: []string{parts[1], parts[3], parts[4]}, known: true}
	case len(parts) == 4 && parts[0] == "projects" && parts[2] == "sprints":
		return routeMatch{name: "sprint", params: []string{parts[1], parts[3]}, known: true}
	case len(parts) == 3 && parts[0] == "studies" && validStudyPage(parts[2]):
		return routeMatch{name: "study_page", params: []string{parts[1], parts[2]}, known: true}
	case len(parts) == 2 && parts[0] == "studies":
		return routeMatch{name: "study", params: parts[1:], known: true}
	case len(parts) == 2 && parts[0] == "artifacts":
		return routeMatch{name: "artifact", params: parts[1:], known: true}
	case len(parts) == 2 && parts[0] == "operations":
		return routeMatch{name: "operation", params: parts[1:], known: true}
	case len(parts) == 4 && parts[0] == "api" && parts[1] == "v1" && parts[2] == "projects":
		return routeMatch{name: "api_project", params: parts[3:], known: true, api: true}
	case len(parts) == 6 && parts[0] == "api" && parts[1] == "v1" && parts[2] == "projects" && parts[4] == "sprints":
		return routeMatch{name: "api_sprint", params: []string{parts[3], parts[5]}, known: true, api: true}
	case len(parts) == 4 && parts[0] == "api" && parts[1] == "v1" && parts[2] == "studies":
		return routeMatch{name: "api_study", params: parts[3:], known: true, api: true}
	case len(parts) == 4 && parts[0] == "api" && parts[1] == "v1" && parts[2] == "artifacts":
		return routeMatch{name: "api_artifact", params: parts[3:], known: true, api: true}
	case len(parts) == 4 && parts[0] == "api" && parts[1] == "v1" && parts[2] == "operations":
		return routeMatch{name: "api_operation", params: parts[3:], known: true, api: true}
	case len(parts) == 5 && parts[0] == "api" && parts[1] == "v1" && parts[2] == "operations" && parts[4] == "events":
		return routeMatch{name: "api_operation_events", params: []string{parts[3]}, known: true, api: true}
	case len(parts) == 2 && parts[0] == "static" && (parts[1] == "app.css" || parts[1] == "app.js"):
		return routeMatch{name: "static", params: parts[1:], known: true, static: true}
	}
	return routeMatch{api: strings.HasPrefix(path, "/api/")}
}

func validProjectPage(page string) bool {
	switch page {
	case "sprints", "documentation", "artifacts", "operations", "validation":
		return true
	default:
		return false
	}
}

func validSprintPage(page string) bool {
	switch page {
	case "run", "artifacts", "plan", "delivery", "operations", "validation":
		return true
	default:
		return false
	}
}

func validStudyPage(page string) bool {
	switch page {
	case "inputs", "progress", "operations", "validation", "artifacts":
		return true
	default:
		return false
	}
}

func splitPath(path string) []string {
	if path == "" || path == "/" || strings.HasSuffix(path, "/") {
		return nil
	}
	return strings.Split(strings.TrimPrefix(path, "/"), "/")
}

type headResponseWriter struct{ http.ResponseWriter }

func (w headResponseWriter) Write(data []byte) (int, error) { return len(data), nil }

func (h *handler) serveStatic(w http.ResponseWriter, _ *http.Request, name string) {
	data, err := assets.ReadFile("static/" + name)
	if err != nil {
		http.Error(w, "static asset unavailable", http.StatusInternalServerError)
		return
	}
	if strings.HasSuffix(name, ".css") {
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
	} else {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	}
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
