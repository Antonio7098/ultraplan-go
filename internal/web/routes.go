package web

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"net/http"
	"strconv"
	"strings"
	"text/template/parse"
	"time"

	"github.com/Antonio7098/ultraplan-go/internal/app"
)

//go:embed templates/*.html templates/*/*.html static/*
var assets embed.FS

var staticAssetNames = map[string]struct{}{
	"app.css": {}, "app.js": {}, "resource-monitor.css": {}, "resource-monitor.js": {},
	"run-timeline.css": {}, "run-timeline.js": {},
	"css/tokens.css": {}, "css/base.css": {}, "css/primitives.css": {}, "css/components.css": {}, "css/layouts.css": {}, "css/utilities.css": {},
	"js/app.js": {}, "js/operations.js": {}, "js/sse.js": {}, "js/study.js": {},
}

type HandlerOptions struct {
	Queries     app.WebQueries
	Operations  app.WebOperations
	Runs        app.RunUseCases
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
	runs         app.RunUseCases
	qa           app.QAQueries
	repair       app.RepairUseCases
}

func NewHandler(opts HandlerOptions) (http.Handler, error) {
	if opts.Queries == nil {
		return nil, errors.New("web queries are required")
	}
	templates, err := parseTemplateTree(assets)
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
	qa, _ := opts.Operations.(app.QAQueries)
	if qa == nil {
		qa, _ = opts.Queries.(app.QAQueries)
	}
	repair, _ := opts.Operations.(app.RepairUseCases)
	if repair == nil {
		repair, _ = opts.Queries.(app.RepairUseCases)
	}
	h := &handler{queries: opts.Queries, templates: templates, now: opts.Now, hub: hub, preparations: newPreparationStore(opts.Now, opts.RequestID), diagnostics: opts.Diagnostics, runs: opts.Runs, qa: qa, repair: repair}
	security := newSecurityMiddleware(opts.Authority, opts.Diagnostics, opts.Now, opts.RequestID)
	return security.wrap(h), nil
}

func parseTemplateTree(source fs.FS) (*template.Template, error) {
	if err := rejectDuplicateTemplateDefinitions(source); err != nil {
		return nil, err
	}
	templates, err := template.ParseFS(source, "templates/*.html", "templates/*/*.html")
	if err != nil {
		return nil, err
	}
	if err := validateTemplateHierarchy(templates); err != nil {
		return nil, err
	}
	return templates, nil
}

func rejectDuplicateTemplateDefinitions(source fs.FS) error {
	seen := make(map[string]string)
	var paths []string
	for _, pattern := range []string{"templates/*.html", "templates/*/*.html"} {
		matches, err := fs.Glob(source, pattern)
		if err != nil {
			return err
		}
		paths = append(paths, matches...)
	}
	for _, path := range paths {
		data, err := fs.ReadFile(source, path)
		if err != nil {
			return err
		}
		parsed, err := template.New(path).Parse(string(data))
		if err != nil {
			return fmt.Errorf("parse template %s: %w", path, err)
		}
		for _, item := range parsed.Templates() {
			name := item.Name()
			if !strings.Contains(name, "/") {
				continue
			}
			if previous := seen[name]; previous != "" {
				return fmt.Errorf("template %q is defined in both %s and %s", name, previous, path)
			}
			seen[name] = path
		}
	}
	return nil
}

func validateTemplateHierarchy(templates *template.Template) error {
	layers := map[string]int{"primitive": 0, "component": 1, "layout": 2, "page": 3}
	required := []string{
		"primitive/empty", "component/artifacts", "component/findings", "component/operation-console",
		"layout/top", "layout/bottom", "page/dashboard", "page/projects", "page/project", "page/sprint",
		"page/studies", "page/study", "page/artifact", "page/operation-confirm", "page/operation", "page/error",
	}
	for _, name := range required {
		if templates.Lookup(name) == nil {
			return fmt.Errorf("required template %q is missing", name)
		}
	}
	graph := make(map[string][]string)
	for _, item := range templates.Templates() {
		name := item.Name()
		parts := strings.SplitN(name, "/", 2)
		if len(parts) != 2 {
			continue // ParseFS also exposes empty filename roots.
		}
		level, ok := layers[parts[0]]
		if !ok {
			return fmt.Errorf("template %q has an unsupported namespace", name)
		}
		dependencies := templateDependencies(item.Tree.Root)
		for _, dependency := range dependencies {
			dependencyParts := strings.SplitN(dependency, "/", 2)
			dependencyLevel, ok := layers[dependencyParts[0]]
			if len(dependencyParts) != 2 || !ok {
				return fmt.Errorf("template %q depends on unnamespaced template %q", name, dependency)
			}
			if dependencyLevel >= level {
				return fmt.Errorf("template %q has upward or same-layer dependency %q", name, dependency)
			}
		}
		graph[name] = dependencies
	}
	visiting, visited := make(map[string]bool), make(map[string]bool)
	var visit func(string) error
	visit = func(name string) error {
		if visiting[name] {
			return fmt.Errorf("template dependency cycle includes %q", name)
		}
		if visited[name] {
			return nil
		}
		visiting[name] = true
		for _, dependency := range graph[name] {
			if err := visit(dependency); err != nil {
				return err
			}
		}
		delete(visiting, name)
		visited[name] = true
		return nil
	}
	for name := range graph {
		if err := visit(name); err != nil {
			return err
		}
	}
	return nil
}

func templateDependencies(node parse.Node) []string {
	var dependencies []string
	var walk func(parse.Node)
	walk = func(current parse.Node) {
		switch value := current.(type) {
		case *parse.ListNode:
			if value != nil {
				for _, child := range value.Nodes {
					walk(child)
				}
			}
		case *parse.TemplateNode:
			dependencies = append(dependencies, value.Name)
		case *parse.IfNode:
			walk(value.List)
			walk(value.ElseList)
		case *parse.RangeNode:
			walk(value.List)
			walk(value.ElseList)
		case *parse.WithNode:
			walk(value.List)
			walk(value.ElseList)
		}
	}
	walk(node)
	return dependencies
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
	case "api_operation_prepare":
		return []string{http.MethodPost}
	case "api_operations":
		return []string{http.MethodGet, http.MethodHead, http.MethodPost}
	case "operation_prepare", "operation_start", "operation_cancel", "run_cancel", "sprint_create":
		return []string{http.MethodPost}
	case "api_operation":
		return []string{http.MethodGet, http.MethodDelete}
	case "api_operation_events":
		return []string{http.MethodGet}
	case "api_run":
		return []string{http.MethodGet, http.MethodHead, http.MethodDelete}
	case "api_run_events":
		return []string{http.MethodGet, http.MethodHead}
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
	if path == "/runs" {
		return routeMatch{name: "runs", known: true}
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
	if path == "/api/v1/models" {
		return routeMatch{name: "api_models", known: true, api: true}
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
	if path == "/api/v1/runs" {
		return routeMatch{name: "api_runs", known: true, api: true}
	}
	if path == "/api/v1/timeline" {
		return routeMatch{name: "api_timeline", known: true, api: true}
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
	case len(parts) == 5 && parts[0] == "projects" && parts[2] == "sprints" && parts[4] == "qa":
		return routeMatch{name: "sprint_qa", params: []string{parts[1], parts[3]}, known: true}
	case len(parts) == 5 && parts[0] == "projects" && parts[2] == "sprints" && parts[4] == "repair":
		return routeMatch{name: "sprint_repair", params: []string{parts[1], parts[3]}, known: true}
	case len(parts) == 7 && parts[0] == "projects" && parts[2] == "sprints" && parts[4] == "qa" && parts[5] == "shards":
		return routeMatch{name: "sprint_qa_shard", params: []string{parts[1], parts[3], parts[6]}, known: true}
	case len(parts) == 7 && parts[0] == "projects" && parts[2] == "sprints" && parts[4] == "qa" && parts[5] == "theories":
		return routeMatch{name: "sprint_qa_theory", params: []string{parts[1], parts[3], parts[6]}, known: true}
	case len(parts) == 3 && parts[0] == "projects" && validProjectPage(parts[2]):
		return routeMatch{name: "project_page", params: []string{parts[1], parts[2]}, known: true}
	case len(parts) == 2 && parts[0] == "projects":
		return routeMatch{name: "project", params: parts[1:], known: true}
	case len(parts) == 5 && parts[0] == "projects" && parts[2] == "sprints" && validSprintPage(parts[4]):
		return routeMatch{name: "sprint_page", params: []string{parts[1], parts[3], parts[4]}, known: true}
	case len(parts) == 4 && parts[0] == "projects" && parts[2] == "sprints" && parts[3] == "create":
		return routeMatch{name: "sprint_create", params: []string{parts[1]}, known: true}
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
	case len(parts) == 2 && parts[0] == "runs":
		return routeMatch{name: "run", params: parts[1:], known: true}
	case len(parts) == 3 && parts[0] == "runs" && parts[2] == "cancel":
		return routeMatch{name: "run_cancel", params: []string{parts[1]}, known: true}
	case len(parts) == 3 && parts[0] == "operations" && parts[2] == "cancel":
		return routeMatch{name: "operation_cancel", params: []string{parts[1]}, known: true}
	case len(parts) == 4 && parts[0] == "api" && parts[1] == "v1" && parts[2] == "projects":
		return routeMatch{name: "api_project", params: parts[3:], known: true, api: true}
	case len(parts) == 6 && parts[0] == "api" && parts[1] == "v1" && parts[2] == "projects" && parts[4] == "sprints":
		return routeMatch{name: "api_sprint", params: []string{parts[3], parts[5]}, known: true, api: true}
	case len(parts) == 7 && parts[0] == "api" && parts[1] == "v1" && parts[2] == "projects" && parts[4] == "sprints" && parts[6] == "qa":
		return routeMatch{name: "api_sprint_qa", params: []string{parts[3], parts[5]}, known: true, api: true}
	case len(parts) == 7 && parts[0] == "api" && parts[1] == "v1" && parts[2] == "projects" && parts[4] == "sprints" && parts[6] == "repair":
		return routeMatch{name: "api_sprint_repair", params: []string{parts[3], parts[5]}, known: true, api: true}
	case len(parts) == 8 && parts[0] == "api" && parts[1] == "v1" && parts[2] == "projects" && parts[4] == "sprints" && parts[6] == "repair" && (parts[7] == "packet" || parts[7] == "cycles" || parts[7] == "result"):
		return routeMatch{name: "api_sprint_repair_" + parts[7], params: []string{parts[3], parts[5]}, known: true, api: true}
	case len(parts) == 8 && parts[0] == "api" && parts[1] == "v1" && parts[2] == "projects" && parts[4] == "sprints" && parts[6] == "qa" && (parts[7] == "map" || parts[7] == "synthesis" || parts[7] == "adjudication" || parts[7] == "issues" || parts[7] == "assessment" || parts[7] == "smoke-suite"):
		return routeMatch{name: "api_sprint_qa_" + parts[7], params: []string{parts[3], parts[5]}, known: true, api: true}
	case len(parts) == 9 && parts[0] == "api" && parts[1] == "v1" && parts[2] == "projects" && parts[4] == "sprints" && parts[6] == "qa" && parts[7] == "shards":
		return routeMatch{name: "api_sprint_qa_shard", params: []string{parts[3], parts[5], parts[8]}, known: true, api: true}
	case len(parts) == 9 && parts[0] == "api" && parts[1] == "v1" && parts[2] == "projects" && parts[4] == "sprints" && parts[6] == "qa" && parts[7] == "theories":
		return routeMatch{name: "api_sprint_qa_theory", params: []string{parts[3], parts[5], parts[8]}, known: true, api: true}
	case len(parts) == 9 && parts[0] == "api" && parts[1] == "v1" && parts[2] == "projects" && parts[4] == "sprints" && parts[6] == "qa" && parts[7] == "evidence":
		return routeMatch{name: "api_sprint_qa_evidence", params: []string{parts[3], parts[5], parts[8]}, known: true, api: true}
	case len(parts) == 9 && parts[0] == "api" && parts[1] == "v1" && parts[2] == "projects" && parts[4] == "sprints" && parts[6] == "qa" && parts[7] == "issues":
		return routeMatch{name: "api_sprint_qa_issue", params: []string{parts[3], parts[5], parts[8]}, known: true, api: true}
	case len(parts) == 8 && parts[0] == "api" && parts[1] == "v1" && parts[2] == "projects" && parts[4] == "sprints" && parts[6] == "prompts":
		return routeMatch{name: "api_prompt_bundle", params: []string{parts[3], parts[5], parts[7]}, known: true, api: true}
	case len(parts) == 4 && parts[0] == "api" && parts[1] == "v1" && parts[2] == "studies":
		return routeMatch{name: "api_study", params: parts[3:], known: true, api: true}
	case len(parts) == 5 && parts[0] == "api" && parts[1] == "v1" && parts[2] == "studies" && parts[4] == "resources":
		return routeMatch{name: "api_study_resources", params: []string{parts[3]}, known: true, api: true}
	case len(parts) == 4 && parts[0] == "api" && parts[1] == "v1" && parts[2] == "artifacts":
		return routeMatch{name: "api_artifact", params: parts[3:], known: true, api: true}
	case len(parts) == 4 && parts[0] == "api" && parts[1] == "v1" && parts[2] == "operations":
		return routeMatch{name: "api_operation", params: parts[3:], known: true, api: true}
	case len(parts) == 5 && parts[0] == "api" && parts[1] == "v1" && parts[2] == "operations" && parts[4] == "events":
		return routeMatch{name: "api_operation_events", params: []string{parts[3]}, known: true, api: true}
	case len(parts) == 4 && parts[0] == "api" && parts[1] == "v1" && parts[2] == "runs":
		return routeMatch{name: "api_run", params: parts[3:], known: true, api: true}
	case len(parts) == 5 && parts[0] == "api" && parts[1] == "v1" && parts[2] == "runs" && parts[4] == "events":
		return routeMatch{name: "api_run_events", params: []string{parts[3]}, known: true, api: true}
	case strings.HasPrefix(path, "/static/"):
		name := strings.TrimPrefix(path, "/static/")
		if _, ok := staticAssetNames[name]; ok {
			return routeMatch{name: "static", params: []string{name}, known: true, static: true}
		}
	}
	return routeMatch{api: strings.HasPrefix(path, "/api/")}
}

func validProjectPage(page string) bool {
	switch page {
	case "roadmap", "sprints", "documentation", "artifacts", "operations", "validation":
		return true
	default:
		return false
	}
}

func validSprintPage(page string) bool {
	switch page {
	case "workflow", "run", "artifacts", "plan", "delivery", "operations", "validation":
		return true
	default:
		return false
	}
}

func validStudyPage(page string) bool {
	switch page {
	case "inputs", "progress", "results", "operations", "validation", "dimensions", "reports", "repos":
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
	w.Header().Set("Cache-Control", "public, max-age=0, must-revalidate")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
