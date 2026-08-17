package web

import (
	"net/http"
	"strings"
	"testing"

	"github.com/Antonio7098/ultraplan-go/internal/app"
)

func TestTemplateAccessibilityStaticAndHostileNames(t *testing.T) {
	queries := sampleQueries()
	hostile := `<script>alert(1)</script>`
	queries.dashboard.Projects.Items[0].Name = hostile
	h := testHandler(t, queries, nil)
	res := request(h, http.MethodGet, "/", nil)
	body := res.Body.String()
	if res.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.Code, body)
	}
	for _, want := range []string{
		`lang="en"`, `href="#main"`, `<main id="main"`, `<h1>Workspace dashboard</h1>`,
		`aria-label="Primary"`, `aria-live="polite"`, `/static/app.css`, `/static/app.js`,
		"&lt;script&gt;alert(1)&lt;/script&gt;", "Workspace files and product run state remain authoritative",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q in %s", want, body)
		}
	}
	if strings.Contains(body, hostile) || strings.Contains(body, "<script>alert(1)") || strings.Contains(body, "style=") {
		t.Fatalf("hostile or inline content rendered: %s", body)
	}
	css := request(h, http.MethodGet, "/static/app.css", nil).Body.String()
	for _, want := range []string{":focus-visible", "@media (max-width:", "prefers-reduced-motion", "overflow: auto"} {
		if !strings.Contains(css, want) {
			t.Errorf("CSS missing %q", want)
		}
	}
}

func TestOperationTemplatesAndEnhancementStayBoundedAndAccessible(t *testing.T) {
	h := testHandler(t, sampleQueries(), nil)
	for _, path := range []string{"/projects/alpha/operations", "/projects/alpha/sprints/30-web/operations", "/studies/research/operations"} {
		body := request(h, http.MethodGet, path, nil).Body.String()
		for _, want := range []string{`class="operation-form`, `aria-live="polite"`, `id="operation-timeline"`, `type="button"`, `<noscript>`} {
			if !strings.Contains(body, want) {
				t.Fatalf("%s missing %q in %s", path, want, body)
			}
		}
	}
	js := request(h, http.MethodGet, "/static/app.js", nil).Body.String()
	for _, want := range []string{"new EventSource", "stream.onopen", "EventSource.CONNECTING", "Reconnecting automatically", "while (timeline.children.length > 100)", `method = "POST"`, `"DELETE"`, "stream.close()", "event.submitter", "window.location.assign", "window.location.reload", "refreshReviewers", "setInterval(refreshReviewers, 2000)", "live-reviewer-grid", "reviewer-result-open", "showModal"} {
		if !strings.Contains(js, want) {
			t.Fatalf("JavaScript missing %q", want)
		}
	}
}

func TestReviewOperationTemplateContainsLiveReviewerGrid(t *testing.T) {
	data, err := assets.ReadFile("templates/operation.html")
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	for _, want := range []string{`eq .Kind "review-start"`, `data-review-status`, `id="live-reviewer-grid"`, `id="review-count-running"`, "Loading reviewer checkpoints"} {
		if !strings.Contains(body, want) {
			t.Errorf("live review template missing %q", want)
		}
	}
}

func TestShellContainsAccessibleReviewerResultDialog(t *testing.T) {
	body, err := assets.ReadFile("templates/shell.html")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`<dialog id="reviewer-result-dialog"`, `aria-labelledby="reviewer-result-title"`, `id="reviewer-result-close"`, `id="reviewer-result-content"`} {
		if !strings.Contains(string(body), want) {
			t.Errorf("reviewer result dialog missing %q", want)
		}
	}
}

func TestSprintRunExposesStageControls(t *testing.T) {
	body := request(testHandler(t, sampleQueries(), nil), http.MethodGet, "/projects/alpha/sprints/30-web/run", nil).Body.String()
	for _, want := range []string{`class="stage-timeline"`, `id="stage-requirements"`, `data-operation-kind="sprint-stage"`, "Run stage", "Run flow to here", "Preview prompt", "Check readiness"} {
		if !strings.Contains(body, want) {
			t.Errorf("stage controls missing %q in %s", want, body)
		}
	}
}

func TestSprintArtifactNavigatorKeepsExplorerContext(t *testing.T) {
	h := testHandler(t, sampleQueries(), nil)
	overview := request(h, http.MethodGet, "/projects/alpha/sprints/30-web/artifacts", nil).Body.String()
	for _, want := range []string{`aria-label="Artefact navigation"`, `>Overview</a>`, ">Definition</summary>", ">Delivery</summary>", "/projects/alpha/sprints/30-web/artifacts/artifact_ref"} {
		if !strings.Contains(overview, want) {
			t.Errorf("artefact overview missing %q", want)
		}
	}
	preview := request(h, http.MethodGet, "/projects/alpha/sprints/30-web/artifacts/artifact_ref", nil).Body.String()
	for _, want := range []string{`class="detail-layout sprint-detail-layout sprint-artifact-layout"`, `aria-label="Project navigation"`, `aria-label="Sprint navigation"`, `aria-label="Artefact navigation"`, `aria-label="Escaped artifact source"`, "# Plan"} {
		if !strings.Contains(preview, want) {
			t.Errorf("nested artefact preview missing %q", want)
		}
	}
}

func TestSprintRunExposesDeliveryActions(t *testing.T) {
	body := request(testHandler(t, sampleQueries(), nil), http.MethodGet, "/projects/alpha/sprints/30-web/run", nil).Body.String()
	for _, want := range []string{"Run execute", "Run review", "Check prerequisites", "Run smoke", "Check scope"} {
		if !strings.Contains(body, want) {
			t.Errorf("delivery observability missing %q", want)
		}
	}
}

func TestSprintDeliveryShowsReviewerStatusGrid(t *testing.T) {
	body := request(testHandler(t, sampleQueries(), nil), http.MethodGet, "/projects/alpha/sprints/30-web/run", nil).Body.String()
	for _, want := range []string{`class="reviewer-grid"`, "Security contract", "completed", "API contract", "running", "Technical handbook", "pending"} {
		if !strings.Contains(body, want) {
			t.Errorf("reviewer grid missing %q in %s", want, body)
		}
	}
}

func TestDetailTemplatesIncludeRoutedContextualNavigation(t *testing.T) {
	h := testHandler(t, sampleQueries(), nil)
	tests := []struct {
		path, label, active string
		links               []string
	}{
		{path: "/projects/alpha/documentation", label: "Project navigation", active: "/projects/alpha/documentation", links: []string{"/projects/alpha", "/projects/alpha/sprints", "/projects/alpha/operations", "/projects/alpha/validation", "/projects/alpha/artifacts"}},
		{path: "/projects/alpha/sprints/30-web/run", label: "Sprint navigation", active: "/projects/alpha/sprints/30-web/run", links: []string{"/projects/alpha/sprints/30-web", "/projects/alpha/sprints/30-web/run", "/projects/alpha/sprints/30-web/artifacts"}},
		{path: "/studies/research/progress", label: "Study navigation", active: "/studies/research/progress", links: []string{"/studies/research", "/studies/research/inputs", "/studies/research/operations", "/studies/research/validation", "/studies/research/artifacts"}},
	}
	for _, tt := range tests {
		body := request(h, http.MethodGet, tt.path, nil).Body.String()
		if !strings.Contains(body, `class="detail-sidebar"`) || !strings.Contains(body, `aria-label="`+tt.label+`"`) {
			t.Fatalf("%s missing contextual sidebar in %s", tt.path, body)
		}
		for _, link := range tt.links {
			if !strings.Contains(body, `href="`+link+`"`) {
				t.Errorf("%s missing destination %s", tt.path, link)
			}
		}
		if !strings.Contains(body, `href="`+tt.active+`" aria-current="page"`) {
			t.Errorf("%s does not identify the current page", tt.path)
		}
	}
	sprintBody := request(h, http.MethodGet, "/projects/alpha/sprints/30-web/run", nil).Body.String()
	if !strings.Contains(sprintBody, `class="detail-layout sprint-detail-layout"`) || !strings.Contains(sprintBody, `aria-label="Project navigation"`) || !strings.Contains(sprintBody, `/projects/alpha/sprints" aria-current="page"`) {
		t.Errorf("sprint page is missing persistent project navigation: %s", sprintBody)
	}
}

func TestDetailOverviewPagesStayFocused(t *testing.T) {
	h := testHandler(t, sampleQueries(), nil)
	for _, path := range []string{"/projects/alpha", "/studies/research"} {
		body := request(h, http.MethodGet, path, nil).Body.String()
		if !strings.Contains(body, `class="destination-grid"`) || strings.Contains(body, `class="operation-form"`) || strings.Contains(body, `<h2>Artifacts</h2>`) {
			t.Errorf("%s is not a focused overview: %s", path, body)
		}
	}
	sprintBody := request(h, http.MethodGet, "/projects/alpha/sprints/30-web", nil).Body.String()
	for _, want := range []string{"What this sprint is", "Make sprint delivery easier to understand.", "Current status", "Run flow"} {
		if !strings.Contains(sprintBody, want) {
			t.Errorf("sprint overview missing %q", want)
		}
	}
}

func TestTemplateEmptyErrorAndTruncationStates(t *testing.T) {
	queries := sampleQueries()
	queries.projects.Items = []app.WebProjectResult{}
	queries.dashboard.Projects.Items = []app.WebProjectResult{}
	h := testHandler(t, queries, nil)
	empty := request(h, http.MethodGet, "/projects", nil)
	if !strings.Contains(empty.Body.String(), "No projects found") {
		t.Fatalf("empty body=%s", empty.Body.String())
	}
	notFound := request(h, http.MethodGet, "/missing", nil)
	if notFound.Code != http.StatusNotFound || !strings.Contains(notFound.Body.String(), `role="alert"`) {
		t.Fatalf("not found status=%d body=%s", notFound.Code, notFound.Body.String())
	}
	queries.artifact.Truncated = true
	queries.artifact.SizeBytes = int64(queries.artifact.ReturnedBytes + 1)
	h = testHandler(t, queries, nil)
	truncated := request(h, http.MethodGet, "/artifacts/artifact_ref", nil)
	if !strings.Contains(truncated.Body.String(), "Preview truncated") || !strings.Contains(truncated.Body.String(), `role="status"`) {
		t.Fatalf("truncation body=%s", truncated.Body.String())
	}
}
