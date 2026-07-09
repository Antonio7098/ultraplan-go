package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/Antonio7098/ultraplan-go/internal/app"
	tea "github.com/charmbracelet/bubbletea"
)

func TestModelTabsRoutesPreviewAndQuit(t *testing.T) {
	fake := &fakeUseCases{result: fixtureDashboard(), preview: app.ArtifactPreviewResult{Content: "# Plan\n"}}
	model, err := NewModel(fake).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if fake.dashboardCalls != 1 || model.Loading {
		t.Fatalf("model = %+v calls=%d", model, fake.dashboardCalls)
	}
	if model.ActiveTab != TabProjects || model.currentRoute().Kind != RouteProjects {
		t.Fatalf("initial route = %+v tab=%s", model.currentRoute(), model.ActiveTab)
	}

	model = model.Update(KeyMsg("enter"))
	if model.currentRoute().Kind != RouteProject || model.currentRoute().Project != "alpha" {
		t.Fatalf("project route = %+v", model.currentRoute())
	}
	model = model.Update(KeyMsg("enter"))
	if model.currentRoute().Kind != RouteProjectSprints {
		t.Fatalf("sprints route = %+v", model.currentRoute())
	}
	model = model.Update(KeyMsg("enter"))
	if model.currentRoute().Kind != RouteSprint || model.currentRoute().Sprint != "01" {
		t.Fatalf("sprint route = %+v", model.currentRoute())
	}
	model.Selected = 4
	model, err = model.PreviewSelected(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if fake.previewCalls != 1 || model.Preview == nil || model.PreviewTitle != "Plan" || !strings.Contains(model.Preview.Content, "Plan") {
		t.Fatalf("preview title=%q preview=%+v calls=%d", model.PreviewTitle, model.Preview, fake.previewCalls)
	}
	model = model.Update(KeyMsg("down"))
	if model.PreviewOffset != 1 {
		t.Fatalf("preview offset = %d, want 1", model.PreviewOffset)
	}
	model = model.Update(KeyMsg("esc"))
	if model.Preview != nil {
		t.Fatalf("preview was not closed")
	}
	model = model.Update(KeyMsg("esc"))
	if model.currentRoute().Kind != RouteProjectSprints {
		t.Fatalf("back route = %+v", model.currentRoute())
	}
	model = model.Update(KeyMsg("2"))
	if model.ActiveTab != TabStudies || model.currentRoute().Kind != RouteStudies {
		t.Fatalf("studies tab route = %+v tab=%s", model.currentRoute(), model.ActiveTab)
	}
	model = model.Update(KeyMsg("q"))
	if !model.Quit {
		t.Fatalf("quit not set")
	}
}

func TestFocusAndTabControls(t *testing.T) {
	model := NewModel(nil)
	model = model.Update(KeyMsg("tab"))
	if model.Focus != FocusTabs {
		t.Fatalf("focus = %s, want tabs", model.Focus)
	}
	model = model.Update(KeyMsg("right"))
	if model.ActiveTab != TabStudies || model.currentRoute().Kind != RouteStudies {
		t.Fatalf("right tab route = %+v tab=%s", model.currentRoute(), model.ActiveTab)
	}
	model = model.Update(KeyMsg("left"))
	if model.ActiveTab != TabProjects || model.currentRoute().Kind != RouteProjects {
		t.Fatalf("left tab route = %+v tab=%s", model.currentRoute(), model.ActiveTab)
	}
	model = model.Update(KeyMsg("tab"))
	if model.Focus != FocusContent {
		t.Fatalf("focus = %s, want content", model.Focus)
	}
}

func TestKeyBindingsExposeOnlyReadOnlyActions(t *testing.T) {
	for _, key := range []string{"x", "!", "e", "g", "3"} {
		if action := KeyToAction(key); action != ActionNone {
			t.Fatalf("key %q action = %s", key, action)
		}
	}
	for _, key := range []string{"q", "esc", "tab", "left", "right", "r", "enter", "1", "2"} {
		if action := KeyToAction(key); action == ActionNone {
			t.Fatalf("key %q was not bound", key)
		}
	}
}

func TestTeaModelLoadsRefreshesPreviewsAndQuits(t *testing.T) {
	fake := &fakeUseCases{result: fixtureDashboard(), preview: app.ArtifactPreviewResult{Content: "# Index\n"}}
	model := newTeaModel(context.Background(), fake, 80)
	msg := model.Init()()
	loaded, cmd := model.Update(msg)
	if cmd != nil {
		t.Fatalf("load returned unexpected command")
	}
	model = loaded.(teaModel)
	if fake.dashboardCalls != 1 || model.model.Loading {
		t.Fatalf("model = %+v calls=%d", model.model, fake.dashboardCalls)
	}

	refreshed, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if cmd == nil {
		t.Fatalf("refresh did not return command")
	}
	model = refreshed.(teaModel)
	msg = cmd()
	modelIface, _ := model.Update(msg)
	model = modelIface.(teaModel)
	if fake.dashboardCalls != 2 {
		t.Fatalf("refresh calls = %d", fake.dashboardCalls)
	}

	opened, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Fatalf("project open should not preview")
	}
	model = opened.(teaModel)
	model.model.Selected = 2
	previewing, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatalf("project index preview did not return command")
	}
	model = previewing.(teaModel)
	msg = cmd()
	modelIface, _ = model.Update(msg)
	model = modelIface.(teaModel)
	if fake.previewCalls != 1 || model.model.Preview == nil {
		t.Fatalf("preview = %+v calls=%d", model.model.Preview, fake.previewCalls)
	}
	closed, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd != nil {
		t.Fatalf("close preview returned unexpected command")
	}
	model = closed.(teaModel)
	if model.model.Preview != nil {
		t.Fatalf("preview was not closed")
	}

	quitting, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	model = quitting.(teaModel)
	if cmd == nil || !model.model.Quit {
		t.Fatalf("quit did not set state or return command")
	}
}

func fixtureDashboard() app.DashboardResult {
	return app.DashboardResult{
		Workspace: "/tmp/ws",
		Projects: []app.ProjectSummary{{
			Name:         "alpha",
			DocsDir:      "present",
			Roadmap:      "present",
			ProjectIndex: "present",
			Catalog:      "ok",
			Artifacts: []app.DisplayArtifact{
				{Label: "project-index", Path: "projects/alpha/project-index.md"},
				{Label: "roadmap", Path: "projects/alpha/roadmap.md"},
				{Label: "doc", Path: "projects/alpha/docs/PRD.md"},
			},
		}},
		Studies: []app.StudySummary{{
			Name:       "research",
			Sources:    []string{"repo"},
			Dimensions: []string{"01-structure"},
			Status:     "complete=false",
			Artifacts: []app.DisplayArtifact{
				{Label: "run-state", Path: "studies/research/.ultraplan/run-state.json"},
				{Label: "dimension", Path: "studies/research/dimensions/01-structure.md"},
				{Label: "source", Path: "studies/research/sources/brief.md"},
			},
		}},
		Sprints: []app.SprintSummary{{
			Project: "alpha",
			Slug:    "01",
			Status:  "available",
			Artifacts: []app.DisplayArtifact{
				{Label: "requirements", Path: "projects/alpha/sprints/01/requirements.md"},
				{Label: "sprint-index", Path: "projects/alpha/sprints/01/sprint-index.md"},
				{Label: "technical-handbook", Path: "projects/alpha/sprints/01/technical-handbook.md"},
				{Label: "reasoning", Path: "projects/alpha/sprints/01/reasoning.md"},
				{Label: "plan", Path: "projects/alpha/sprints/01/plan.md"},
				{Label: "execute", Path: "projects/alpha/sprints/01/execute.md"},
				{Label: "flow-state", Path: "projects/alpha/sprints/01/flow-state.json"},
				{Label: "run-state", Path: "projects/alpha/sprints/01/.run-state.json"},
			},
		}},
	}
}
