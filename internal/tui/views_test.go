package tui

import (
	"strings"
	"testing"

	"github.com/Antonio7098/ultraplan-go/internal/app"
)

func TestRenderTabsAndProjectNavigationWithoutArtifactPaths(t *testing.T) {
	model := NewModel(nil)
	model.Data = fixtureDashboard()
	out := RenderWithSize(model, 100, 20)
	for _, want := range []string{"[Projects]", "Studies", "> alpha", "docs=present", "enter open"} {
		if !strings.Contains(out, want) {
			t.Fatalf("render missing %q:\n%s", want, out)
		}
	}
	for _, hidden := range []string{"projects/alpha/project-index.md", "projects/alpha/docs/PRD.md"} {
		if strings.Contains(out, hidden) {
			t.Fatalf("navigation leaked path %q:\n%s", hidden, out)
		}
	}

	model = model.Update(KeyMsg("enter"))
	out = RenderWithSize(model, 100, 20)
	for _, want := range []string{"Projects > alpha", "> Sprints", "Docs", "Project Index", "Roadmap"} {
		if !strings.Contains(out, want) {
			t.Fatalf("project detail missing %q:\n%s", want, out)
		}
	}
}

func TestRenderSprintArtifactsByNameOnly(t *testing.T) {
	model := NewModel(nil)
	model.Data = fixtureDashboard()
	model.Routes = []Route{{Kind: RouteProjects}, {Kind: RouteProject, Project: "alpha"}, {Kind: RouteProjectSprints, Project: "alpha"}, {Kind: RouteSprint, Project: "alpha", Sprint: "01"}}
	model.Selected = 4
	out := RenderWithSize(model, 100, 20)
	for _, want := range []string{"Projects > alpha > Sprints > 01", "Requirements", "Sprint Index", "Technical Handbook", "> Plan"} {
		if !strings.Contains(out, want) {
			t.Fatalf("sprint detail missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "projects/alpha/sprints/01/plan.md") {
		t.Fatalf("sprint detail leaked artifact path:\n%s", out)
	}
}

func TestRenderStudySummaryAboveNavigation(t *testing.T) {
	m := NewModel(&fakeUseCases{})
	m.Loading = false
	m.ActiveTab = TabStudies
	m.Routes = []Route{{Kind: RouteStudy, Study: "demo"}}
	m.Data.Studies = []app.StudySummary{{Name: "demo", Dimensions: []string{"01-a", "02-b"}, Sources: []string{"repo", "notes.md", "api.md"}, Total: 7, Completed: 4, ActiveTasks: 1, RunActive: true, RunStatus: "active", Tasks: []app.RunTaskSummary{{ID: "analysis-active", Status: "running", Dimension: "02-b", Source: "repo", Attempts: 2, Duration: "5s", DurationMS: 5000}, {ID: "analysis-01-repo", Status: "completed", Dimension: "01-a", Source: "repo", Attempts: 1, Tokens: 1234, TokensKnown: true, Duration: "42s", DurationMS: 42000, Events: 8, Provider: "opencode", Model: "demo-model", Cost: "0.01 USD"}}}}
	view := Render(m, 80)
	for _, want := range []string{"Study summary", "Dimensions: 2", "Sources: 3", "Planned runs: 7", "Done so far: 4", "Run status: active (4/7 done)", "> View Run [ACTIVE]"} {
		if !strings.Contains(view, want) {
			t.Fatalf("study view missing %q:\n%s", want, view)
		}
	}
	if strings.Index(view, "Study summary") > strings.Index(view, "> View Run [ACTIVE]") {
		t.Fatalf("summary was not above navigation:\n%s", view)
	}
	if !strings.Contains(view, "View Run [ACTIVE]") || strings.Contains(view, "Run Loop [RUNTIME]") {
		t.Fatalf("active run actions incorrect:\n%s", view)
	}
	m.RunViewStudy = "demo"
	view = Render(m, 80)
	for _, want := range []string{"Run summary — demo", "Total: 7  Completed: 4  Remaining: 3  Active: 1", "Total tokens: 1234 (known tasks)", "Total runtime: 47s", "Press c to cancel this run.", "Currently running (1)", "analysis-active [running]", "> See More (1 previous runs)"} {
		if !strings.Contains(view, want) {
			t.Fatalf("live run view missing %q:\n%s", want, view)
		}
	}
	m.RunViewShowPrevious = true
	view = Render(m, 100)
	for _, want := range []string{"Previous runs (1) — Enter: Show Less", "analysis-01-repo [completed]", "workflow_attempts=1 runtime_attempts=0 agent_turns=n/a tokens=1234"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expanded previous missing %q:\n%s", want, view)
		}
	}
}

func TestInactiveStudyHasOneUnifiedRunLoopAction(t *testing.T) {
	m := NewModel(&fakeUseCases{})
	m.Loading = false
	m.ActiveTab = TabStudies
	m.Routes = []Route{{Kind: RouteStudy, Study: "demo"}}
	m.Data.Studies = []app.StudySummary{{Name: "demo", Total: 3}}
	view := Render(m, 80)
	if strings.Count(view, "Run Loop [RUNTIME]") != 1 || strings.Contains(view, "Start Run Loop") || strings.Contains(view, "Resume Run Loop") || strings.Contains(view, "View Run") {
		t.Fatalf("unified run-loop action incorrect:\n%s", view)
	}
	if !strings.Contains(view, "> Run Loop [RUNTIME]") || strings.Index(view, "> Run Loop [RUNTIME]") > strings.Index(view, "\n  Dimensions >") {
		t.Fatalf("run loop was not first:\n%s", view)
	}
}

func TestRenderRunLoopParallelForm(t *testing.T) {
	m := NewModel(&fakeUseCases{})
	req := app.OperationRequest{Kind: app.OperationStudyResume, Study: "demo"}
	m.ParallelForm = &req
	m.ParallelValue = "8"
	view := Render(m, 80)
	for _, want := range []string{"Run-loop parameters", "Study: demo", "Parallel workers (1-64): 8", "Enter to review and confirm"} {
		if !strings.Contains(view, want) {
			t.Fatalf("parallel form missing %q:\n%s", want, view)
		}
	}
}

func TestRenderPreviewErrorsAndTruncation(t *testing.T) {
	preview := app.ArtifactPreviewResult{Path: "projects/a/sprints/b/.run-state.json", Kind: "json", Error: "invalid JSON preview", Invalid: true, Truncated: true, Content: "{bad"}
	out := Render(Model{ActiveTab: TabProjects, Focus: FocusContent, Routes: []Route{{Kind: RouteProjects}}, Preview: &preview, PreviewTitle: "Run State"}, 100)
	for _, want := range []string{"Run State", "invalid JSON preview", "Format: invalid", "Truncated: true"} {
		if !strings.Contains(out, want) {
			t.Fatalf("render missing %q:\n%s", want, out)
		}
	}
}

func TestRenderMarkdownPreviewUsesMarkdownRenderer(t *testing.T) {
	preview := app.ArtifactPreviewResult{Path: "projects/a/roadmap.md", Kind: "markdown", Content: "# Title\n\n- item\n"}
	out := Render(Model{ActiveTab: TabProjects, Focus: FocusContent, Routes: []Route{{Kind: RouteProjects}}, Preview: &preview, PreviewTitle: "Roadmap"}, 100)
	for _, want := range []string{"Roadmap", "Title", "item"} {
		if !strings.Contains(out, want) {
			t.Fatalf("render missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "# Title") {
		t.Fatalf("markdown was not rendered:\n%s", out)
	}
}

func TestRenderMarkdownPreviewRemovesHeadingMarkers(t *testing.T) {
	rendered := renderMarkdownContent("## Required Outputs\n", 100)
	if strings.Contains(rendered, "## Required Outputs") {
		t.Fatalf("heading marker was preserved:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Required") || !strings.Contains(rendered, "Outputs") {
		t.Fatalf("heading text missing:\n%s", rendered)
	}
}

func TestRenderPreviewScrollsWithPreviewOffset(t *testing.T) {
	preview := app.ArtifactPreviewResult{Path: "projects/a/sprints/b/flow-state.json", Kind: "json", Content: "line-1\nline-2\nline-3\nline-4\nline-5\nline-6\nline-7\nline-8\nline-9\nline-10\nline-11\nline-12"}
	top := RenderWithSize(Model{ActiveTab: TabProjects, Focus: FocusContent, Routes: []Route{{Kind: RouteProjects}}, Preview: &preview, PreviewTitle: "Plan"}, 100, 12)
	if !strings.Contains(top, "line-1") || strings.Contains(top, "line-7") {
		t.Fatalf("unexpected top preview:\n%s", top)
	}
	scrolled := RenderWithSize(Model{ActiveTab: TabProjects, Focus: FocusContent, Routes: []Route{{Kind: RouteProjects}}, Preview: &preview, PreviewTitle: "Plan", PreviewOffset: 6}, 100, 12)
	if strings.Contains(scrolled, "\nline-1\n") || !strings.Contains(scrolled, "line-4") {
		t.Fatalf("unexpected scrolled preview:\n%s", scrolled)
	}
}

func TestRenderFollowsSelectedItemWithinTerminalHeight(t *testing.T) {
	var projects []app.ProjectSummary
	for i := 0; i < 12; i++ {
		projects = append(projects, app.ProjectSummary{Name: "project-" + string(rune('a'+i)), Catalog: "ok"})
	}
	model := NewModel(nil)
	model.Data = app.DashboardResult{Workspace: "/tmp/ws", Projects: projects}
	top := RenderWithSize(model, 100, 8)
	if !strings.Contains(top, "scroll 1/") {
		t.Fatalf("top render missing scroll indicator:\n%s", top)
	}
	if strings.Contains(top, "project-l") {
		t.Fatalf("top render leaked bottom row:\n%s", top)
	}
	model.Selected = 10
	scrolled := RenderWithSize(model, 100, 8)
	if !strings.Contains(scrolled, "scroll ") {
		t.Fatalf("scrolled render missing scroll position:\n%s", scrolled)
	}
	if !strings.Contains(scrolled, "> project-k") {
		t.Fatalf("scrolled render did not expose selected row:\n%s", scrolled)
	}
}
