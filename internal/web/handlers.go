package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"

	"github.com/Antonio7098/ultraplan-go/internal/app"
)

var identifierPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
var opaqueRefPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

type responseMeta struct {
	APIVersion    string `json:"api_version"`
	RequestID     string `json:"request_id"`
	GeneratedAt   string `json:"generated_at,omitempty"`
	ReturnedCount *int   `json:"returned_count,omitempty"`
	TotalCount    *int   `json:"total_count,omitempty"`
	Truncated     *bool  `json:"truncated,omitempty"`
}

type successEnvelope struct {
	Data any          `json:"data"`
	Meta responseMeta `json:"meta"`
}

type errorBody struct {
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	Retryable bool           `json:"retryable,omitempty"`
	Details   map[string]any `json:"details,omitempty"`
}

type errorEnvelope struct {
	Error errorBody    `json:"error"`
	Meta  responseMeta `json:"meta"`
}

type artifactDTO struct {
	Ref         string `json:"ref"`
	Label       string `json:"label,omitempty"`
	DisplayPath string `json:"display_path"`
	MediaType   string `json:"media_type"`
}

type findingDTO struct {
	Severity   string `json:"severity"`
	Section    string `json:"section,omitempty"`
	Problem    string `json:"problem"`
	Cause      string `json:"cause,omitempty"`
	Suggestion string `json:"suggestion,omitempty"`
}

type stageDTO struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

type projectDTO struct {
	Ref       string        `json:"ref"`
	Name      string        `json:"name"`
	Docs      []string      `json:"docs"`
	Findings  []findingDTO  `json:"findings"`
	Artifacts []artifactDTO `json:"artifacts"`
	Sprints   []sprintDTO   `json:"sprints"`
}

type collectionDTO struct {
	ReturnedCount int  `json:"returned_count"`
	TotalCount    int  `json:"total_count"`
	Truncated     bool `json:"truncated"`
}

type sprintDTO struct {
	Ref        string        `json:"ref"`
	Project    string        `json:"project"`
	Slug       string        `json:"slug"`
	Status     string        `json:"status"`
	Assessment string        `json:"assessment,omitempty"`
	NextAction string        `json:"next_action,omitempty"`
	Stages     []stageDTO    `json:"stages"`
	Execute    executeDTO    `json:"execute"`
	Review     reviewDTO     `json:"review"`
	Smoke      smokeDTO      `json:"smoke"`
	Findings   []findingDTO  `json:"findings"`
	Artifacts  []artifactDTO `json:"artifacts"`
}

type executeDTO struct {
	Available bool `json:"available"`
	Total     int  `json:"total,omitempty"`
	Pending   int  `json:"pending,omitempty"`
	Running   int  `json:"running,omitempty"`
	Complete  int  `json:"complete,omitempty"`
	Failed    int  `json:"failed,omitempty"`
	Cancelled int  `json:"cancelled,omitempty"`
}

type reviewDTO struct {
	Available bool          `json:"available"`
	Status    string        `json:"status,omitempty"`
	Verdict   string        `json:"verdict,omitempty"`
	Stale     bool          `json:"stale,omitempty"`
	Completed int           `json:"completed,omitempty"`
	Total     int           `json:"total,omitempty"`
	Pending   int           `json:"pending,omitempty"`
	Running   int           `json:"running,omitempty"`
	Failed    int           `json:"failed,omitempty"`
	Reviewers []reviewerDTO `json:"reviewers"`
}

type reviewerDTO struct {
	ID      string `json:"id"`
	Name    string `json:"name,omitempty"`
	Kind    string `json:"kind,omitempty"`
	Path    string `json:"path,omitempty"`
	Status  string `json:"status"`
	Summary string `json:"summary,omitempty"`
}

type smokeDTO struct {
	Available bool   `json:"available"`
	Status    string `json:"status,omitempty"`
	Verdict   string `json:"verdict,omitempty"`
	Stale     bool   `json:"stale,omitempty"`
	RunID     string `json:"run_id,omitempty"`
}

type studyDTO struct {
	Ref         string        `json:"ref"`
	Name        string        `json:"name"`
	Sources     []string      `json:"sources"`
	Dimensions  []string      `json:"dimensions"`
	Status      string        `json:"status"`
	RunID       string        `json:"run_id,omitempty"`
	Total       int           `json:"total"`
	Completed   int           `json:"completed"`
	Failed      int           `json:"failed"`
	RunActive   bool          `json:"run_active"`
	ActiveTasks int           `json:"active_tasks"`
	Pending     int           `json:"pending"`
	Cancelled   int           `json:"cancelled"`
	Findings    []findingDTO  `json:"findings"`
	Artifacts   []artifactDTO `json:"artifacts"`
}

type artifactPreviewDTO struct {
	Ref           string `json:"ref"`
	DisplayPath   string `json:"display_path"`
	MediaType     string `json:"media_type"`
	Content       string `json:"content"`
	SizeBytes     int64  `json:"size_bytes"`
	ReturnedBytes int    `json:"returned_bytes"`
	Truncated     bool   `json:"truncated"`
	JSONValid     *bool  `json:"json_valid,omitempty"`
}

type pageModel struct {
	Title        string
	Heading      string
	Description  string
	Workspace    string
	Projects     []app.WebProjectResult
	Project      *app.WebProjectResult
	Sprints      []app.WebSprintResult
	Sprint       *app.WebSprintResult
	Studies      []app.WebStudyResult
	Study        *app.WebStudyResult
	Artifact     *app.WebArtifactPreview
	ArtifactHTML template.HTML
	Health       *app.WebHealthResult
	Status       int
	Error        string
	CSRF         string
	Preparation  *operationPreparationView
	Operation    *operationDocument
	Page         string
}

func (h *handler) dispatch(w http.ResponseWriter, r *http.Request, match routeMatch) {
	if match.name != "api_validations" && r.URL.RawQuery != "" {
		h.writeRouteError(w, r, match.api, http.StatusBadRequest, "invalid_request", "Unknown query parameters are not accepted.")
		return
	}
	for _, value := range match.params {
		valid := validIdentifier(value)
		if strings.Contains(match.name, "artifact") {
			valid = validOpaqueRef(value)
		}
		if !valid {
			h.writeRouteError(w, r, match.api, http.StatusBadRequest, "invalid_request", "The resource identifier is invalid.")
			return
		}
	}

	switch match.name {
	case "dashboard":
		result, err := h.queries.Dashboard(r.Context())
		if err != nil {
			h.handleQueryError(w, r, false, err)
			return
		}
		h.render(w, r, http.StatusOK, "dashboard", pageModel{Title: "Workspace dashboard", Heading: "Workspace dashboard", Workspace: result.Workspace, Projects: result.Projects.Items, Sprints: result.Sprints, Studies: result.Studies.Items})
	case "projects":
		result, err := h.queries.Projects(r.Context())
		if err != nil {
			h.handleQueryError(w, r, false, err)
			return
		}
		h.render(w, r, http.StatusOK, "projects", pageModel{Title: "Projects", Heading: "Projects", Projects: result.Items})
	case "project":
		result, err := h.queries.Project(r.Context(), match.params[0])
		if err != nil {
			h.handleQueryError(w, r, false, err)
			return
		}
		h.render(w, r, http.StatusOK, "project", pageModel{Title: "Project " + result.Name, Heading: result.Name, Project: &result, Page: "overview"})
	case "project_page":
		result, err := h.queries.Project(r.Context(), match.params[0])
		if err != nil {
			h.handleQueryError(w, r, false, err)
			return
		}
		page := match.params[1]
		if page == "artifacts" || page == "validation" {
			page = "documentation"
		}
		h.render(w, r, http.StatusOK, "project", pageModel{Title: projectPageTitle(page) + " · " + result.Name, Heading: result.Name, Project: &result, Page: page})
	case "project_artifact":
		result, err := h.queries.Project(r.Context(), match.params[0])
		if err != nil {
			h.handleQueryError(w, r, false, err)
			return
		}
		artifact, err := h.queries.Artifact(r.Context(), match.params[1])
		if err != nil {
			h.handleQueryError(w, r, false, err)
			return
		}
		h.render(w, r, http.StatusOK, "project", pageModel{Title: artifact.DisplayPath + " · " + result.Name, Heading: result.Name, Project: &result, Artifact: &artifact, ArtifactHTML: renderMarkdown(artifact), Page: "documentation"})
	case "sprint":
		result, err := h.queries.Sprint(r.Context(), match.params[0], match.params[1])
		if err != nil {
			h.handleQueryError(w, r, false, err)
			return
		}
		h.render(w, r, http.StatusOK, "sprint", pageModel{Title: "Sprint " + result.Slug, Heading: result.Slug, Sprint: &result, Page: "overview"})
	case "sprint_page":
		result, err := h.queries.Sprint(r.Context(), match.params[0], match.params[1])
		if err != nil {
			h.handleQueryError(w, r, false, err)
			return
		}
		page := match.params[2]
		if page == "plan" || page == "delivery" || page == "operations" || page == "validation" {
			page = "run"
		}
		h.render(w, r, http.StatusOK, "sprint", pageModel{Title: sprintPageTitle(page) + " · " + result.Slug, Heading: result.Slug, Sprint: &result, Page: page})
	case "sprint_artifact":
		result, err := h.queries.Sprint(r.Context(), match.params[0], match.params[1])
		if err != nil {
			h.handleQueryError(w, r, false, err)
			return
		}
		artifact, err := h.queries.Artifact(r.Context(), match.params[2])
		if err != nil {
			h.handleQueryError(w, r, false, err)
			return
		}
		h.render(w, r, http.StatusOK, "sprint", pageModel{Title: artifact.DisplayPath + " · " + result.Slug, Heading: result.Slug, Sprint: &result, Artifact: &artifact, ArtifactHTML: renderMarkdown(artifact), Page: "artifacts"})
	case "studies":
		result, err := h.queries.Studies(r.Context())
		if err != nil {
			h.handleQueryError(w, r, false, err)
			return
		}
		h.render(w, r, http.StatusOK, "studies", pageModel{Title: "Studies", Heading: "Studies", Studies: result.Items})
	case "study":
		result, err := h.queries.Study(r.Context(), match.params[0])
		if err != nil {
			h.handleQueryError(w, r, false, err)
			return
		}
		h.render(w, r, http.StatusOK, "study", pageModel{Title: "Study " + result.Name, Heading: result.Name, Study: &result, Page: "overview"})
	case "study_page":
		result, err := h.queries.Study(r.Context(), match.params[0])
		if err != nil {
			h.handleQueryError(w, r, false, err)
			return
		}
		page := match.params[1]
		h.render(w, r, http.StatusOK, "study", pageModel{Title: studyPageTitle(page) + " · " + result.Name, Heading: result.Name, Study: &result, Page: page})
	case "artifact":
		result, err := h.queries.Artifact(r.Context(), match.params[0])
		if err != nil {
			h.handleQueryError(w, r, false, err)
			return
		}
		if err := validateArtifact(result); err != nil {
			h.handleQueryError(w, r, false, err)
			return
		}
		h.render(w, r, http.StatusOK, "artifact", pageModel{Title: "Artifact preview", Heading: result.DisplayPath, Artifact: &result, ArtifactHTML: renderMarkdown(result)})
	case "api_dashboard":
		result, err := h.queries.Dashboard(r.Context())
		if err != nil {
			h.handleQueryError(w, r, true, err)
			return
		}
		h.writeSuccess(w, r, http.StatusOK, map[string]any{
			"workspace": result.Workspace,
			"ref":       result.Ref,
			"projects":  mapProjects(result.Projects.Items, false),
			"sprints":   mapSprints(result.Sprints),
			"studies":   mapStudies(result.Studies.Items),
			"counts": map[string]any{
				"projects": mapCollection(result.Projects.CollectionInfo),
				"sprints":  mapCollection(result.SprintCounts),
				"studies":  mapCollection(result.Studies.CollectionInfo),
			},
		}, nil)
	case "api_projects":
		result, err := h.queries.Projects(r.Context())
		if err != nil {
			h.handleQueryError(w, r, true, err)
			return
		}
		h.writeSuccess(w, r, http.StatusOK, mapProjects(result.Items, false), &result.CollectionInfo)
	case "api_project":
		result, err := h.queries.Project(r.Context(), match.params[0])
		if err != nil {
			h.handleQueryError(w, r, true, err)
			return
		}
		h.writeSuccess(w, r, http.StatusOK, mapProject(result, true), &result.SprintCounts)
	case "api_sprint":
		result, err := h.queries.Sprint(r.Context(), match.params[0], match.params[1])
		if err != nil {
			h.handleQueryError(w, r, true, err)
			return
		}
		h.writeSuccess(w, r, http.StatusOK, mapSprint(result), nil)
	case "api_studies":
		result, err := h.queries.Studies(r.Context())
		if err != nil {
			h.handleQueryError(w, r, true, err)
			return
		}
		h.writeSuccess(w, r, http.StatusOK, mapStudies(result.Items), &result.CollectionInfo)
	case "api_study":
		result, err := h.queries.Study(r.Context(), match.params[0])
		if err != nil {
			h.handleQueryError(w, r, true, err)
			return
		}
		h.writeSuccess(w, r, http.StatusOK, mapStudy(result), nil)
	case "api_validations":
		h.handleValidations(w, r)
	case "api_artifact":
		result, err := h.queries.Artifact(r.Context(), match.params[0])
		if err != nil {
			h.handleQueryError(w, r, true, err)
			return
		}
		if err := validateArtifact(result); err != nil {
			h.handleQueryError(w, r, true, err)
			return
		}
		h.writeSuccess(w, r, http.StatusOK, mapArtifactPreview(result), nil)
	case "api_health":
		result, err := h.queries.Health(r.Context())
		if err != nil {
			h.handleQueryError(w, r, true, err)
			return
		}
		status := http.StatusOK
		if result.Status != "ok" || !result.Workspace {
			status = http.StatusServiceUnavailable
		}
		h.writeSuccess(w, r, status, map[string]any{"status": result.Status, "server": result.Server, "workspace": result.Workspace}, nil)
	case "api_operation_prepare":
		h.handleOperationPrepare(w, r)
	case "api_operations":
		h.handleOperationStart(w, r)
	case "api_operation":
		if r.Method == http.MethodDelete {
			h.handleOperationCancel(w, r, match.params[0])
		} else {
			h.handleOperationStatus(w, r, match.params[0])
		}
	case "api_operation_events":
		h.handleOperationEvents(w, r, match.params[0])
	case "operation_prepare":
		h.handleHTMLOperationPrepare(w, r)
	case "operation_start":
		h.handleHTMLOperationStart(w, r)
	case "operation":
		h.handleHTMLOperationStatus(w, r, match.params[0])
	default:
		h.writeRouteError(w, r, match.api, http.StatusNotFound, "not_found", "The requested resource was not found.")
	}
}

func projectPageTitle(page string) string {
	switch page {
	case "sprints":
		return "Sprints"
	default:
		return "Docs"
	}
}

func renderMarkdown(artifact app.WebArtifactPreview) template.HTML {
	if artifact.MediaType != "text/markdown" {
		return ""
	}
	var out bytes.Buffer
	md := goldmark.New(goldmark.WithExtensions(extension.GFM))
	if err := md.Convert([]byte(artifact.Content), &out); err != nil {
		return ""
	}
	return template.HTML(out.String()) // Goldmark omits raw HTML and filters unsafe link destinations by default.
}

func sprintPageTitle(page string) string {
	switch page {
	case "run":
		return "Run"
	default:
		return "Artefact Navigator"
	}
}

func studyPageTitle(page string) string {
	switch page {
	case "inputs":
		return "Inputs"
	case "progress":
		return "Progress"
	case "operations":
		return "Operations"
	case "validation":
		return "Validation"
	default:
		return "Artifacts"
	}
}

func (h *handler) handleValidations(w http.ResponseWriter, r *http.Request) {
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || len(values) != 2 || len(values["scope"]) != 1 || len(values["ref"]) != 1 {
		h.writeError(w, r, http.StatusBadRequest, "invalid_request", "Validation requires one scope and one ref.")
		return
	}
	scope, ref := values.Get("scope"), values.Get("ref")
	if (scope != "workspace" && scope != "project" && scope != "sprint" && scope != "study") || !validOpaqueRef(ref) {
		h.writeError(w, r, http.StatusBadRequest, "invalid_request", "The validation query is invalid.")
		return
	}
	result, err := h.queries.Validations(r.Context(), scope, ref)
	if err != nil {
		h.handleQueryError(w, r, true, err)
		return
	}
	h.writeSuccess(w, r, http.StatusOK, map[string]any{"scope": result.Scope, "ref": result.Ref, "findings": mapFindings(result.Findings)}, &result.CollectionInfo)
}

func validIdentifier(value string) bool {
	return len(value) > 0 && len(value) <= MaxIdentifierBytes && value != "." && value != ".." &&
		!strings.ContainsAny(value, `/\`) && identifierPattern.MatchString(value)
}

func validOpaqueRef(value string) bool {
	return len(value) > 0 && len(value) <= MaxIdentifierBytes && opaqueRefPattern.MatchString(value)
}

func (h *handler) handleQueryError(w http.ResponseWriter, r *http.Request, apiRoute bool, err error) {
	status, code, message := http.StatusInternalServerError, "internal_error", "The request could not be completed."
	switch {
	case errors.Is(err, app.ErrWebNotFound):
		status, code, message = http.StatusNotFound, "not_found", "The requested resource was not found."
	case errors.Is(err, app.ErrWebUnavailable), errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		status, code, message = http.StatusServiceUnavailable, "unavailable", "The service is unavailable."
	}
	h.writeRouteError(w, r, apiRoute, status, code, message)
}

func (h *handler) writeRouteError(w http.ResponseWriter, r *http.Request, apiRoute bool, status int, code, message string) {
	if apiRoute {
		h.writeError(w, r, status, code, message)
		return
	}
	h.renderError(w, r, status, http.StatusText(status), message)
}

func (h *handler) writeSuccess(w http.ResponseWriter, r *http.Request, status int, data any, collection *app.CollectionInfo) {
	meta := responseMeta{APIVersion: "v1", RequestID: requestID(r.Context()), GeneratedAt: h.now().UTC().Format(time.RFC3339Nano)}
	if collection != nil {
		meta.ReturnedCount = intPtr(collection.ReturnedCount)
		meta.TotalCount = intPtr(collection.TotalCount)
		meta.Truncated = boolPtr(collection.Truncated)
	}
	writeJSON(w, status, successEnvelope{Data: data, Meta: meta})
}

func (h *handler) writeError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	writeJSONErrorResponse(w, requestID(r.Context()), status, code, message)
}

func writeJSONErrorResponse(w http.ResponseWriter, id string, status int, code, message string) {
	writeJSON(w, status, errorEnvelope{Error: errorBody{Code: code, Message: message}, Meta: responseMeta{APIVersion: "v1", RequestID: id}})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(true)
	if err := encoder.Encode(payload); err != nil {
		http.Error(w, "response encoding failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Length", strconv.Itoa(buf.Len()))
	w.WriteHeader(status)
	_, _ = w.Write(buf.Bytes())
}

func writePolicyError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		writeJSONErrorResponse(w, requestID(r.Context()), status, code, message)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, "<!doctype html><html lang=\"en\"><head><meta charset=\"utf-8\"><title>Request rejected</title></head><body><main><h1>Request rejected</h1><p>%s</p></main></body></html>", templateText(message))
}

func templateText(value string) string {
	return template.HTMLEscapeString(value)
}

func (h *handler) render(w http.ResponseWriter, r *http.Request, status int, name string, page pageModel) {
	page.CSRF = csrfToken(r.Context())
	var buf bytes.Buffer
	if err := h.templates.ExecuteTemplate(&buf, name, page); err != nil {
		http.Error(w, "page rendering failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Length", strconv.Itoa(buf.Len()))
	w.WriteHeader(status)
	_, _ = w.Write(buf.Bytes())
}

func (h *handler) renderError(w http.ResponseWriter, r *http.Request, status int, title, message string) {
	h.render(w, r, status, "error", pageModel{Title: title, Heading: title, Status: status, Error: message})
}

func mapProject(item app.WebProjectResult, includeSprints bool) projectDTO {
	out := projectDTO{Ref: item.Ref, Name: item.Name, Docs: nonNilSlice(item.Docs), Findings: mapFindings(item.Findings), Artifacts: mapArtifacts(item.Artifacts), Sprints: []sprintDTO{}}
	if includeSprints {
		out.Sprints = mapSprints(item.Sprints)
	}
	return out
}

func mapCollection(item app.CollectionInfo) collectionDTO {
	return collectionDTO{ReturnedCount: item.ReturnedCount, TotalCount: item.TotalCount, Truncated: item.Truncated}
}

func mapProjects(items []app.WebProjectResult, includeSprints bool) []projectDTO {
	out := make([]projectDTO, 0, len(items))
	for _, item := range items {
		out = append(out, mapProject(item, includeSprints))
	}
	return out
}

func mapSprint(item app.WebSprintResult) sprintDTO {
	stages := make([]stageDTO, 0, len(item.Stages))
	for _, stage := range item.Stages {
		stages = append(stages, stageDTO{Name: stage.Name, Status: stage.Status})
	}
	reviewers := make([]reviewerDTO, 0, len(item.Review.Reviewers))
	for _, reviewer := range item.Review.Reviewers {
		reviewers = append(reviewers, reviewerDTO{ID: reviewer.ID, Name: reviewer.Name, Kind: reviewer.Kind, Path: reviewer.Path, Status: reviewer.Status, Summary: reviewer.Summary})
	}
	return sprintDTO{
		Ref: item.Ref, Project: item.Project, Slug: item.Slug, Status: item.Status,
		Assessment: item.Assessment, NextAction: item.NextAction, Stages: stages,
		Execute:  executeDTO{Available: item.Execute.Available, Total: item.Execute.Total, Pending: item.Execute.Pending, Running: item.Execute.Running, Complete: item.Execute.Complete, Failed: item.Execute.Failed, Cancelled: item.Execute.Cancelled},
		Review:   reviewDTO{Available: item.Review.Available, Status: item.Review.Status, Verdict: item.Review.Verdict, Stale: item.Review.Stale, Completed: item.Review.Completed, Total: item.Review.Total, Pending: item.Review.Pending, Running: item.Review.Running, Failed: item.Review.Failed, Reviewers: reviewers},
		Smoke:    smokeDTO{Available: item.Smoke.Available, Status: item.Smoke.Status, Verdict: item.Smoke.Verdict, Stale: item.Smoke.Stale, RunID: item.Smoke.RunID},
		Findings: mapFindings(item.Findings), Artifacts: mapArtifacts(item.Artifacts),
	}
}

func mapSprints(items []app.WebSprintResult) []sprintDTO {
	out := make([]sprintDTO, 0, len(items))
	for _, item := range items {
		out = append(out, mapSprint(item))
	}
	return out
}

func mapStudy(item app.WebStudyResult) studyDTO {
	return studyDTO{Ref: item.Ref, Name: item.Name, Sources: nonNilSlice(item.Sources), Dimensions: nonNilSlice(item.Dimensions), Status: item.Status, RunID: item.RunID, Total: item.Total, Completed: item.Completed, Failed: item.Failed, RunActive: item.RunActive, ActiveTasks: item.ActiveTasks, Pending: item.Pending, Cancelled: item.Cancelled, Findings: mapFindings(item.Findings), Artifacts: mapArtifacts(item.Artifacts)}
}

func mapStudies(items []app.WebStudyResult) []studyDTO {
	out := make([]studyDTO, 0, len(items))
	for _, item := range items {
		out = append(out, mapStudy(item))
	}
	return out
}

func mapFindings(items []app.DisplayFinding) []findingDTO {
	out := make([]findingDTO, 0, len(items))
	for _, item := range items {
		out = append(out, findingDTO{Severity: item.Severity, Section: item.Section, Problem: item.Problem, Cause: item.Cause, Suggestion: item.Suggestion})
	}
	return out
}

func mapArtifacts(items []app.WebArtifactLink) []artifactDTO {
	out := make([]artifactDTO, 0, len(items))
	for _, item := range items {
		out = append(out, artifactDTO{Ref: item.Ref, Label: item.Label, DisplayPath: item.DisplayPath, MediaType: item.MediaType})
	}
	return out
}

func mapArtifactPreview(item app.WebArtifactPreview) artifactPreviewDTO {
	out := artifactPreviewDTO{Ref: item.Ref, DisplayPath: item.DisplayPath, MediaType: item.MediaType, Content: item.Content, SizeBytes: item.SizeBytes, ReturnedBytes: item.ReturnedBytes, Truncated: item.Truncated}
	if item.MediaType == "application/json" {
		out.JSONValid = boolPtr(item.JSONValid)
	}
	return out
}

func nonNilSlice[T any](items []T) []T {
	if items == nil {
		return []T{}
	}
	return items
}

func intPtr(value int) *int    { return &value }
func boolPtr(value bool) *bool { return &value }
