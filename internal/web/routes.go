package web

import (
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
	Authority   string
	Diagnostics io.Writer
	Now         func() time.Time
	RequestID   func() string
}

type handler struct {
	queries   app.WebQueries
	templates *template.Template
	now       func() time.Time
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
	h := &handler{queries: opts.Queries, templates: templates, now: opts.Now}
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
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		h.writeRouteError(w, r, match.api, http.StatusMethodNotAllowed, "method_not_allowed", "Only GET and HEAD are supported.")
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
	parts := splitPath(path)
	switch {
	case len(parts) == 2 && parts[0] == "projects":
		return routeMatch{name: "project", params: parts[1:], known: true}
	case len(parts) == 4 && parts[0] == "projects" && parts[2] == "sprints":
		return routeMatch{name: "sprint", params: []string{parts[1], parts[3]}, known: true}
	case len(parts) == 2 && parts[0] == "studies":
		return routeMatch{name: "study", params: parts[1:], known: true}
	case len(parts) == 2 && parts[0] == "artifacts":
		return routeMatch{name: "artifact", params: parts[1:], known: true}
	case len(parts) == 4 && parts[0] == "api" && parts[1] == "v1" && parts[2] == "projects":
		return routeMatch{name: "api_project", params: parts[3:], known: true, api: true}
	case len(parts) == 6 && parts[0] == "api" && parts[1] == "v1" && parts[2] == "projects" && parts[4] == "sprints":
		return routeMatch{name: "api_sprint", params: []string{parts[3], parts[5]}, known: true, api: true}
	case len(parts) == 4 && parts[0] == "api" && parts[1] == "v1" && parts[2] == "studies":
		return routeMatch{name: "api_study", params: parts[3:], known: true, api: true}
	case len(parts) == 4 && parts[0] == "api" && parts[1] == "v1" && parts[2] == "artifacts":
		return routeMatch{name: "api_artifact", params: parts[3:], known: true, api: true}
	case len(parts) == 2 && parts[0] == "static" && (parts[1] == "app.css" || parts[1] == "app.js"):
		return routeMatch{name: "static", params: parts[1:], known: true, static: true}
	}
	return routeMatch{api: strings.HasPrefix(path, "/api/")}
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
