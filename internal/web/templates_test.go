package web

import (
	"net/http"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/Antonio7098/ultraplan-go/internal/app"
	"github.com/Antonio7098/ultraplan-go/internal/project"
)

func TestTemplateHierarchyIsNamespacedAndDownwardOnly(t *testing.T) {
	templates, err := parseTemplateTree(assets)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"primitive/empty", "component/artifacts", "layout/top", "page/dashboard", "page/operation"} {
		if templates.Lookup(name) == nil {
			t.Errorf("missing %q", name)
		}
	}
	for _, item := range templates.Templates() {
		if strings.Contains(item.Name(), "/") && !strings.HasPrefix(item.Name(), "primitive/") && !strings.HasPrefix(item.Name(), "component/") && !strings.HasPrefix(item.Name(), "layout/") && !strings.HasPrefix(item.Name(), "page/") {
			t.Errorf("unexpected template namespace %q", item.Name())
		}
	}
}

func TestTemplateHierarchyFailsClosed(t *testing.T) {
	root := &fstest.MapFile{Data: []byte("{{/* root */}}")}
	missing := fstest.MapFS{
		"templates/root.html":        root,
		"templates/pages/pages.html": {Data: []byte(`{{define "page/dashboard"}}ok{{end}}`)},
	}
	if _, err := parseTemplateTree(missing); err == nil || !strings.Contains(err.Error(), "required template") {
		t.Fatalf("missing definitions error=%v", err)
	}

	upward := fstest.MapFS{
		"templates/root.html":        root,
		"templates/pages/pages.html": {Data: []byte(templateHierarchyFixture(`{{template "page/projects" .}}`))},
	}
	if _, err := parseTemplateTree(upward); err == nil || !strings.Contains(err.Error(), "upward or same-layer") {
		t.Fatalf("upward dependency error=%v", err)
	}

	duplicate := fstest.MapFS{
		"templates/root.html":                  root,
		"templates/pages/pages.html":           {Data: []byte(templateHierarchyFixture("ok"))},
		"templates/components/components.html": {Data: []byte(`{{define "component/findings"}}duplicate{{end}}`)},
	}
	if _, err := parseTemplateTree(duplicate); err == nil {
		t.Fatal("duplicate template definition was accepted")
	}
}

func templateHierarchyFixture(dashboardBody string) string {
	return `
{{define "primitive/empty"}}{{end}}
{{define "component/artifacts"}}{{end}}
{{define "component/findings"}}{{end}}
{{define "component/operation-console"}}{{end}}
{{define "layout/top"}}{{end}}
{{define "layout/bottom"}}{{end}}
{{define "page/dashboard"}}` + dashboardBody + `{{end}}
{{define "page/projects"}}{{end}}
{{define "page/project"}}{{end}}
{{define "page/sprint"}}{{end}}
{{define "page/studies"}}{{end}}
{{define "page/study"}}{{end}}
{{define "page/artifact"}}{{end}}
{{define "page/operation-confirm"}}{{end}}
{{define "page/operation"}}{{end}}
{{define "page/error"}}{{end}}`
}

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
	for _, path := range []string{"/projects/alpha", "/projects/alpha/sprints/30-web", "/studies/research/progress"} {
		body := request(h, http.MethodGet, path, nil).Body.String()
		for _, want := range []string{`class="operation-form`, `action="/operations/prepare"`} {
			if !strings.Contains(body, want) {
				t.Fatalf("%s missing %q in %s", path, want, body)
			}
		}
	}
	js := request(h, http.MethodGet, "/static/app.js", nil).Body.String()
	studyOperations := request(h, http.MethodGet, "/studies/research/progress", nil).Body.String()
	for _, want := range []string{`name="parallelism"`, `<option value="1" selected>1</option>`, `<option value="8">8</option>`} {
		if !strings.Contains(studyOperations, want) {
			t.Fatalf("study operations missing parallelism choice %q", want)
		}
	}
	if !strings.Contains(js, "form.elements?.parallelism?.value") {
		t.Fatal("operation JavaScript ignores the chosen parallelism")
	}
	for _, want := range []string{"new EventSource", "stream.onopen", "EventSource.CONNECTING", "Reconnecting automatically", "while (timeline.children.length > 100)", "timeline.scrollTop = timeline.scrollHeight", `method = "POST"`, `"DELETE"`, "stream.close()", "event.submitter", "window.location.assign", "window.location.reload", "data-stage-select", "data-stage-operation-status", "setInterval(refreshReviewers, 2000)", "durableStatusPath", "durableProcesses", `item.kind === "sprint-flow"`, "activeFlows.has(sprintScope)", "operation.href", `processes.addEventListener("pointerenter"`, "groupActiveRuns", `kind === "study-loop"`, "parallel agent"} {
		if !strings.Contains(js, want) {
			t.Fatalf("JavaScript missing %q", want)
		}
	}
	operationsJS := request(h, http.MethodGet, "/static/js/operations.js", nil).Body.String()
	if !strings.Contains(operationsJS, "result.shard = selectedShard") {
		t.Fatal("modular operation serializer does not preserve the map-owned QA shard option")
	}
	for _, want := range []string{"body.error?.details?.reason", "body.error?.details?.guidance", `code === "session_required"`, `code === "csrf_failed"`, `response.headers.get("X-CSRF-Token")`, "attempt < 2"} {
		if !strings.Contains(operationsJS, want) {
			t.Fatalf("operation JavaScript missing safe error detail %q", want)
		}
	}
}

func TestProjectReasoningLifecycleAndAdmissionAreRendered(t *testing.T) {
	queries := sampleQueries()
	queries.project.ProjectReasoning = project.ProjectReasoningStatus{
		Mode: project.ProjectReasoningRequired, RequiredVerdict: "pass_with_findings", CurrentStage: project.ProjectAreaReasoning,
		Fresh: false, Outputs: []string{"projects/alpha/project-reasoning/areas/api.md"}, Blockers: []string{"area reasoning is stale"},
	}
	h := testHandler(t, queries, nil)
	overview := request(h, http.MethodGet, "/projects/alpha", nil).Body.String()
	for _, want := range []string{"Sprint admission", "Current stage", "area-reasoning", "Required verdict", "pass_with_findings", "reasoning-stage-track", "Area reasoning", "tracked outputs", "project-reasoning-flow", "project-reasoning-validate", "project-reasoning-prompt", "Prepare reasoning run"} {
		if !strings.Contains(overview, want) {
			t.Errorf("project reasoning overview missing %q in %s", want, overview)
		}
	}
	roadmap := request(h, http.MethodGet, "/projects/alpha/roadmap", nil).Body.String()
	for _, want := range []string{"Sprint admission is blocked", "Complete project reasoning", "Reasoning required"} {
		if !strings.Contains(roadmap, want) {
			t.Errorf("blocked roadmap missing %q in %s", want, roadmap)
		}
	}
	if strings.Contains(roadmap, ">Add sprint</button>") || strings.Contains(roadmap, ">Create folder</button>") {
		t.Fatalf("blocked roadmap exposes sprint admission controls: %s", roadmap)
	}

	queries.project.ProjectReasoning.Accepted = true
	queries.project.ProjectReasoning.Fresh = true
	queries.project.ProjectReasoning.Blockers = nil
	roadmap = request(h, http.MethodGet, "/projects/alpha/roadmap", nil).Body.String()
	if !strings.Contains(roadmap, ">Add sprint</button>") || !strings.Contains(roadmap, ">Create folder</button>") || strings.Contains(roadmap, "Sprint admission is blocked") {
		t.Fatalf("accepted roadmap does not expose sprint controls: %s", roadmap)
	}
}

func TestOperationTemplateUsesOneLiveStatusPanelWithoutReviewers(t *testing.T) {
	data, err := assets.ReadFile("templates/operation.html")
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	for _, want := range []string{`class="operation-run-panel"`, `id="operation-live"`, `id="operation-timeline"`, "Cancel run"} {
		if !strings.Contains(body, want) {
			t.Errorf("live operation template missing %q", want)
		}
	}
	if strings.Contains(body, "Reviewer") || strings.Contains(body, "reviewer") {
		t.Errorf("operation panel still contains reviewer UI")
	}
}

func TestShellDoesNotContainReviewerResultDialog(t *testing.T) {
	body, err := assets.ReadFile("templates/shell.html")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "reviewer-result") {
		t.Error("shell still contains the reviewer result dialog")
	}
}

func TestPrimaryNavigationUsesTopBarDestinations(t *testing.T) {
	body, err := assets.ReadFile("templates/shell.html")
	if err != nil {
		t.Fatal(err)
	}
	shell := string(body)
	for _, want := range []string{`class="brand" href="/"`, `<a href="/projects">Projects</a>`, `<a href="/studies">Studies</a>`, `aria-label="Running processes: 0"`, `class="run-history-link" href="/runs"`} {
		if !strings.Contains(shell, want) {
			t.Errorf("primary navigation missing %q", want)
		}
	}
	if strings.Contains(shell, `<a href="/">Dashboard</a>`) {
		t.Error("primary navigation still contains a dashboard item")
	}
	for _, want := range []string{`data-nav-flyout`, `aria-label="Show projects"`, `aria-label="Show studies"`, `data-endpoint="/api/v1/projects"`, `data-endpoint="/api/v1/studies"`} {
		if !strings.Contains(shell, want) {
			t.Errorf("primary navigation flyout missing %q", want)
		}
	}
}

func TestSprintRunExposesStageControls(t *testing.T) {
	body := request(testHandler(t, sampleQueries(), nil), http.MethodGet, "/projects/alpha/sprints/30-web/run", nil).Body.String()
	for _, want := range []string{`class="stage-timeline"`, `data-stage-workspace`, `class="stage-details"`, `href="#stage-requirements" data-stage-select="stage-requirements" data-stage-has-artifact="true"`, `id="stage-requirements"`, `data-operation-kind="sprint-flow"`, `data-stage-operation-status role="status" aria-live="polite"`, "Start run to smoke", "Open result summary", `id="operation-timeline"`, "Stage links and run forms work without JavaScript", `class="prompt-observability" data-prompt-observability`, `Stage input contract`, `Injected in this order`, `input contract + bundle summary`, `/api/v1/projects/alpha/sprints/30-web/prompts/requirements`, `<code>project-index</code>`, `<code>project-docs</code>`, `Open JSON summary`} {
		if !strings.Contains(body, want) {
			t.Errorf("stage controls missing %q in %s", want, body)
		}
	}
	if strings.Contains(body, `data-stage-panel hidden`) || strings.Contains(body, "JavaScript is required") {
		t.Fatalf("server-rendered run controls are hidden without JavaScript: %s", body)
	}
}

func TestPromptBundleSummaryAPIIncludesRenderedBlockContent(t *testing.T) {
	queries := sampleQueries()
	handler := testHandler(t, queries, nil)
	page := request(handler, http.MethodGet, "/projects/alpha/sprints/30-web/run", nil)
	if page.Code != http.StatusOK || queries.promptCalls != 0 {
		t.Fatalf("run page eagerly prepared prompt bundle: status=%d calls=%d", page.Code, queries.promptCalls)
	}
	response := request(handler, http.MethodGet, "/api/v1/projects/alpha/sprints/30-web/prompts/plan", nil)
	body := response.Body.String()
	if response.Code != http.StatusOK || queries.promptCalls != 1 {
		t.Fatalf("status=%d calls=%d body=%s", response.Code, queries.promptCalls, body)
	}
	for _, want := range []string{`"stage":"plan"`, `"available":true`, `"scope":"Deterministic stage preview"`, `"input_contract"`, `"required"`, `"total_bytes"`, `"cache_candidate":true`, `"blocks"`, `"sha256"`} {
		if !strings.Contains(body, want) {
			t.Errorf("prompt summary missing %q in %s", want, body)
		}
	}
	if !strings.Contains(body, `"content"`) {
		t.Fatalf("prompt summary omitted rendered block content: %s", body)
	}
}

func TestSmokeCoverageMappingAppearsInRunAndSmokeResultOnly(t *testing.T) {
	queries := sampleQueries()
	overview := request(testHandler(t, queries, nil), http.MethodGet, "/projects/alpha/sprints/30-web", nil).Body.String()
	if strings.Contains(overview, `id="smoke-coverage-heading"`) {
		t.Fatal("smoke coverage mapping should not appear on the sprint overview")
	}
	run := request(testHandler(t, queries, nil), http.MethodGet, "/projects/alpha/sprints/30-web/run", nil).Body.String()
	queries.artifact.DisplayPath = "projects/alpha/sprints/30-web/smoke.md"
	smokeResult := request(testHandler(t, queries, nil), http.MethodGet, "/projects/alpha/sprints/30-web/artifacts/artifact_ref", nil).Body.String()
	for name, body := range map[string]string{"run": run, "smoke result": smokeResult} {
		for _, want := range []string{`id="smoke-coverage-heading"`, `class="coverage-matrix"`, `class="coverage-requirement-trigger status-ok"`, `data-coverage-id="AC-01"`, `data-coverage-description="The browser boundary is exercised."`, `data-coverage-status="mapped"`, `data-coverage-id="AC-02"`, `data-coverage-status="unmapped"`, `class="coverage-requirement-popover"`, `data-coverage-requirement-dialog role="dialog"`, "provider probe missing", "browser-boundary", "sprint-30", "✓", "incomplete"} {
			if !strings.Contains(body, want) {
				t.Errorf("%s smoke coverage mapping missing %q", name, want)
			}
		}
	}
}

func TestSprintCodeContextOrderAndPreservedArtifactOutcome(t *testing.T) {
	body := request(testHandler(t, sampleQueries(), nil), http.MethodGet, "/projects/alpha/sprints/30-web/run", nil).Body.String()
	requirements := strings.Index(body, `href="#stage-requirements"`)
	codeContext := strings.Index(body, `href="#stage-code-context"`)
	sprintIndex := strings.Index(body, `href="#stage-sprint-index"`)
	if requirements < 0 || codeContext <= requirements || sprintIndex <= codeContext {
		t.Fatalf("stage order is not requirements, code-context, sprint-index: %d %d %d", requirements, codeContext, sprintIndex)
	}
	for _, want := range []string{"Latest attempt:</strong> failed", "Why it failed", "provider failed", "Authoritative artifact:</strong> available and structurally valid", "A prior valid artifact is preserved", "Start run to code-context"} {
		if !strings.Contains(body, want) {
			t.Errorf("code-context presentation missing %q", want)
		}
	}
}

func TestSprintArtifactNavigatorKeepsExplorerContext(t *testing.T) {
	h := testHandler(t, sampleQueries(), nil)
	overview := request(h, http.MethodGet, "/projects/alpha/sprints/30-web/artifacts", nil).Body.String()
	for _, want := range []string{`class="breadcrumb"`, `href="/projects/alpha/roadmap"`, ">Definition</h2>", ">Delivery</h2>", "/artifacts/artifact_ref"} {
		if !strings.Contains(overview, want) {
			t.Errorf("artifact overview missing %q", want)
		}
	}
	preview := request(h, http.MethodGet, "/projects/alpha/sprints/30-web/artifacts/artifact_ref", nil).Body.String()
	for _, want := range []string{`class="breadcrumb"`, `href="/projects/alpha/roadmap"`, `class="markdown-body"`, "<h1>Plan</h1>"} {
		if !strings.Contains(preview, want) {
			t.Errorf("nested artifact preview missing %q", want)
		}
	}
}

func TestSprintRunOnlyExposesFlowToStageAction(t *testing.T) {
	body := request(testHandler(t, sampleQueries(), nil), http.MethodGet, "/projects/alpha/sprints/30-web/run", nil).Body.String()
	for _, unwanted := range []string{"Run execute", "Run review", "Check prerequisites", "Check scope", "Preview prompt", "Check readiness"} {
		if strings.Contains(body, unwanted) {
			t.Errorf("run panel contains extra action %q", unwanted)
		}
	}
}

func TestSprintRunShowsReviewerStatusGridInRunningReviewStage(t *testing.T) {
	body := request(testHandler(t, sampleQueries(), nil), http.MethodGet, "/projects/alpha/sprints/30-web/run", nil).Body.String()
	for _, want := range []string{`data-review-status`, `data-activity-panel`, `class="activity-summary"`, `id="activity-time"`, `id="activity-agents"`, `id="activity-actions"`, `id="activity-tools"`, `id="latest-event"`, `class="event-log"`, `id="event-log-count"`, `class="reviewer-grid"`, `id="review-count-complete"`, `id="review-count-running"`, `id="review-count-pending"`, `id="review-count-failed"`, `id="reviewer-result-dialog"`, "This Conformance Review is running", "Durable progress and worker transitions appear below", "Security contract", "API contract", "Technical handbook"} {
		if !strings.Contains(body, want) {
			t.Errorf("running review stage is missing reviewer UI %q", want)
		}
	}
}

func TestDetailTemplatesUseBreadcrumbsWithoutTopNavigation(t *testing.T) {
	h := testHandler(t, sampleQueries(), nil)
	for _, path := range []string{"/projects/alpha/documentation", "/projects/alpha/sprints/30-web/workflow", "/studies/research/progress"} {
		body := request(h, http.MethodGet, path, nil).Body.String()
		if !strings.Contains(body, `class="breadcrumb"`) || strings.Contains(body, `class="entity-nav"`) {
			t.Errorf("%s does not use breadcrumb-only navigation: %s", path, body)
		}
	}
}

func TestProjectDashboardComponentsOwnTheirDestinations(t *testing.T) {
	h := testHandler(t, sampleQueries(), nil)
	body := request(h, http.MethodGet, "/projects/alpha", nil).Body.String()
	for _, want := range []string{`href="/projects/alpha/roadmap"`, `href="/projects/alpha/documentation"`, "sprints done", "Previous sprint", "Next sprint"} {
		if !strings.Contains(body, want) {
			t.Errorf("project dashboard missing %q", want)
		}
	}
	if strings.Contains(body, `class="section-heading"><h2>Documentation`) || strings.Contains(body, `class="entity-nav"`) {
		t.Error("project dashboard retained redundant navigation")
	}
}

func TestNestedNavigationHasNoSidebarOrEntityTabs(t *testing.T) {
	h := testHandler(t, sampleQueries(), nil)
	tests := []struct {
		path  string
		wants []string
	}{
		{"/projects/alpha/documentation", []string{`class="breadcrumb"`}},
		{"/projects/alpha/sprints/30-web/artifacts", []string{`class="breadcrumb"`}},
	}
	for _, tt := range tests {
		body := request(h, http.MethodGet, tt.path, nil).Body.String()
		for _, want := range tt.wants {
			if !strings.Contains(body, want) {
				t.Errorf("%s missing drill-down sidebar marker %q", tt.path, want)
			}
		}
		if strings.Contains(body, `class="detail-sidebar"`) || strings.Contains(body, `data-sidebar-stack`) || strings.Contains(body, `class="entity-nav"`) {
			t.Errorf("%s retained sidebar navigation", tt.path)
		}
	}
}

func TestDetailOverviewPagesStayFocused(t *testing.T) {
	h := testHandler(t, sampleQueries(), nil)
	for _, path := range []string{"/projects/alpha", "/studies/research"} {
		body := request(h, http.MethodGet, path, nil).Body.String()
		if !strings.Contains(body, `entity-summary`) || !strings.Contains(body, `class="operation-form`) || strings.Contains(body, `>Operations</a>`) {
			t.Errorf("%s is not a focused overview: %s", path, body)
		}
	}
	sprintBody := request(h, http.MethodGet, "/projects/alpha/sprints/30-web", nil).Body.String()
	for _, want := range []string{"Sprint brief", "Make sprint delivery easier to understand.", "Current status", "Run flow"} {
		if !strings.Contains(sprintBody, want) {
			t.Errorf("sprint overview missing %q", want)
		}
	}
}

func TestProjectRoadmapRendersPhasesAndSprintEntries(t *testing.T) {
	queries := sampleQueries()
	queries.project.Artifacts = append(queries.project.Artifacts, app.WebArtifactLink{Ref: "roadmap_ref", Label: "roadmap", DisplayPath: "projects/alpha/roadmap.md", MediaType: "text/markdown"})
	h := testHandler(t, queries, nil)

	for _, path := range []string{"/projects/alpha/roadmap", "/projects/alpha/sprints"} {
		body := request(h, http.MethodGet, path, nil).Body.String()
		for _, want := range []string{
			`>Roadmap</h2>`,
			"Product Phase 4 Wave — Local Web",
			"Sprint 29: Phase 3 Documentation And Release",
			"Stabilize review and smoke as supported workflows.",
			"Sprint 30: Local Web Foundation",
			"Serve a loopback-only read-only browser dashboard.",
			"Sprint 31: Guarded Web Operations And SSE Progress",
			"Stream truthful live progress to the browser.",
			`status-ok">delivered</span>`,
			`status-info">active</span>`,
			`status-warn">planned</span>`,
			`href="/projects/alpha/sprints/30-web"`,
			`href="/projects/alpha/documentation/roadmap_ref">Open source document</a>`,
			"1 acceptance criteria",
			"3 of 5 stages complete",
			`data-add-sprint`, `name="sprint_number"`, `name="kind" value="sprint-flow"`, `>Prepare sprint flow</button>`,
			`data-start-sprint="start-sprint-31-web-operations"`, `id="start-sprint-31-web-operations"`,
			`action="/projects/alpha/sprints/create"`, `name="sprint" value="31-web-operations"`, "no folder yet", ">Create folder</button>",
		} {
			if !strings.Contains(body, want) {
				t.Errorf("roadmap page %s missing %q", path, want)
			}
		}
	}

	planned := request(h, http.MethodGet, "/projects/alpha/roadmap", nil).Body.String()
	if strings.Contains(planned, `href="/projects/alpha/sprints/31-web-operations"`) {
		t.Errorf("unmaterialized planned sprint is rendered as a link: %s", planned)
	}
	if !strings.Contains(planned, `class="milestone-entry milestone-start"`) {
		t.Errorf("unmaterialized planned sprint lacks create-folder trigger markup: %s", planned)
	}

	js := request(testHandler(t, queries, nil), http.MethodGet, "/static/app.js", nil).Body.String()
	for _, want := range []string{`[data-add-sprint]`, `[data-start-sprint]`, `dialog?.showModal()`, `dialog?.close()`} {
		if !strings.Contains(js, want) {
			t.Errorf("add sprint dialog behavior missing %q", want)
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

func TestSprintFlowUsagePanelRendersPerStageBreakdown(t *testing.T) {
	queries := sampleQueries()
	queries.sprintUsage = app.SprintMetricsSummary{
		RecentRuns: []app.SprintMetricRow{
			{Stage: "requirements", Status: "ok", Model: "x", InputKnown: true, Input: 1000, OutputKnown: true, Output: 200,
				CacheReadKnown: true, CacheRead: 50, CacheWriteKnown: true, CacheWrite: 0, TotalKnown: true, Total: 1250,
				CostKnown: true, CostAmount: 0.012, CostSource: "model_priced"},
			{Stage: "plan", Status: "ok", Model: "y", InputKnown: true, Input: 500, OutputKnown: true, Output: 100,
				CacheReadKnown: true, CacheRead: 25, CacheWriteKnown: true, CacheWrite: 0, TotalKnown: true, Total: 625,
				CostKnown: true, CostAmount: 0.006, CostSource: "provider_reported"},
		},
	}
	body := request(testHandler(t, queries, nil), http.MethodGet, "/projects/alpha/sprints/30-web", nil).Body.String()
	for _, want := range []string{
		`id="sprint-flow-usage-heading"`,
		"Token usage &amp; cost by stage",
		"Requirements",
		"Plan",
		`class="stage-usage-table"`,
		"$0.018",
		"LiteLLM public rates",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("sprint flow usage panel missing %q", want)
		}
	}
}

func TestSprintRuntimeMetricsPageRendersCompleteCallLedger(t *testing.T) {
	queries := sampleQueries()
	queries.sprintUsage = app.SprintMetricsSummary{
		SchemaVersion: 2,
		AllRuns: []app.SprintMetricRow{{
			Sequence: 17, StageSequence: 3, Stage: "qa", Operation: "qa-arbitrate", Role: "arbiter", Status: "completed",
			CallID: "call-17", RunID: "run-17", SessionID: "session-17", QAAttemptID: "qa-attempt", ArbiterGroupID: "group-2", Coverage: "changed-paths", Cycle: "2", Call: "1",
			Provider: "openrouter", Model: "minimax/minimax-m3:free", Variant: "high", Sandbox: "read_only", FinalTargetIndex: 0,
			PromptBytes: 4096, PrefixBytes: 3072, SuffixBytes: 1024, PrefixDigest: strings.Repeat("a", 64), CacheKey: "qa-arbiter/cohort", CacheMode: "stable-prefix", CacheTransport: "agentwrap-metadata-only",
			InputKnown: true, Input: 84, OutputKnown: true, Output: 21, ReasoningKnown: true, Reasoning: 5,
			CacheReadKnown: true, CacheRead: 27763, CacheWriteKnown: true, CacheWrite: 0, TotalKnown: true, Total: 27873, TurnsKnown: true, Turns: 1,
			ToolCalls: 0, ToolCallCountExact: true, RuntimeEvents: 8, RetainedEvents: 8, PermissionMode: "restricted", PermissionDefault: "deny",
			DurationMS: 27121, CostKnown: true, CostAmount: 0, CostCurrency: "USD", CostEstimated: true, CostSource: "provider_reported",
		}},
	}
	body := request(testHandler(t, queries, nil), http.MethodGet, "/projects/alpha/sprints/30-web/metrics", nil).Body.String()
	for _, want := range []string{
		`id="runtime-ledger-heading"`, "Runtime call ledger", "Complete token rows", "Calls", "qa-arbitrate", "arbiter",
		"openrouter/minimax/minimax-m3:free", "27763", "agentwrap-metadata-only", strings.Repeat("a", 64), "call-17", "session-17",
		"Tool calls", "exact", "restricted", "provider_reported", "changed-paths", "Cycle / call", "final target 0", "estimated",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("runtime metrics page missing %q", want)
		}
	}
}

func TestStudyLoopUsagePanelRendersPerTaskBreakdown(t *testing.T) {
	queries := sampleQueries()
	queries.study.Tasks = []app.RunTaskSummary{
		{ID: "analysis:01-structure:repo", Kind: "analysis", Dimension: "01", Source: "repo",
			Status: "completed", TurnsKnown: true, Turns: 5,
			InputKnown: true, InputTokens: 200, OutputKnown: true, OutputTokens: 50,
			CacheReadKnown: true, CacheReadTokens: 20, CacheWriteKnown: true, CacheWriteTokens: 0,
			TokensKnown: true, Tokens: 270, CostKnown: true, CostAmount: 0.01, CostSource: "model_priced", Cost: "$0.01"},
		{ID: "synthesis:01-structure:final", Kind: "synthesis", Dimension: "01", Source: "synthesis",
			Status: "completed", TurnsKnown: true, Turns: 3,
			InputKnown: true, InputTokens: 100, OutputKnown: true, OutputTokens: 30,
			CacheReadKnown: true, CacheReadTokens: 10, CacheWriteKnown: true, CacheWriteTokens: 0,
			TokensKnown: true, Tokens: 140, CostKnown: true, CostAmount: 0.005, CostSource: "provider_reported", Cost: "$0.005"},
	}
	body := request(testHandler(t, queries, nil), http.MethodGet, "/studies/research", nil).Body.String()
	for _, want := range []string{
		`id="study-loop-usage-heading"`,
		"Token usage &amp; cost by task",
		`class="study-task-usage-table"`,
		"analysis:01-structure:repo",
		"synthesis:01-structure:final",
		"rate-table",
		"$0.015",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("study loop usage panel missing %q", want)
		}
	}
}

func TestSprintAndStudyUsagePanelsHideWhenNoData(t *testing.T) {
	queries := sampleQueries()
	queries.sprintUsage = app.SprintMetricsSummary{}
	queries.study.Tasks = nil
	body := request(testHandler(t, queries, nil), http.MethodGet, "/projects/alpha/sprints/30-web", nil).Body.String()
	if strings.Contains(body, `id="sprint-flow-usage-heading"`) {
		t.Error("sprint flow usage panel must hide when no metrics are reported")
	}
	body = request(testHandler(t, queries, nil), http.MethodGet, "/studies/research", nil).Body.String()
	if strings.Contains(body, `id="study-loop-usage-heading"`) {
		t.Error("study loop usage panel must hide when no usage data exists")
	}
}
