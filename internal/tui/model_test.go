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

func TestSprintNavigationExposesFullReviewOperation(t *testing.T) {
	m := NewModel(&fakeUseCases{})
	m.Data = fixtureDashboard()
	m.Routes = []Route{{Kind: RouteSprint, Project: "alpha", Sprint: "01"}}
	labels := map[string]bool{}
	for _, item := range m.navItems() {
		labels[item.Label] = true
	}
	for _, want := range []string{"Review", "Validate review", "Preview review Prompt", "Review Status", "Review Dry Run", "Run Review [RUNTIME]"} {
		if !labels[want] {
			t.Fatalf("missing review navigation %q", want)
		}
	}
}

func TestModelValidationResultAndBack(t *testing.T) {
	m := NewModel(&fakeUseCases{})
	m.Loading = true
	m.Routes = []Route{{Kind: RouteProject, Project: "alpha"}}
	m = m.Update(ValidationMsg{Route: m.currentRoute(), Result: app.ValidationOperationResult{
		Operation: "validate", Subject: "alpha", Status: "invalid",
		Findings: []app.DisplayFinding{{Severity: "error", Path: "projects/alpha/roadmap.md", Problem: "missing roadmap", Suggestion: "create roadmap.md"}},
	}})
	if m.Loading || m.Validation == nil || m.Validation.Status != "invalid" {
		t.Fatalf("validation state = %+v", m)
	}
	view := Render(m, 80)
	for _, want := range []string{"Validation: alpha", "Status: invalid", "missing roadmap", "Guidance: create roadmap.md"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q:\n%s", want, view)
		}
	}
	m = m.Update(KeyMsg("esc"))
	if m.Validation != nil {
		t.Fatal("validation pane remained after back")
	}
}

func TestOperationConfirmationProgressBoundAndTerminal(t *testing.T) {
	m := NewModel(&fakeUseCases{})
	m.Routes = []Route{{Kind: RouteSprint, Project: "alpha", Sprint: "01"}}
	req := app.OperationRequest{Kind: app.OperationExecuteStart, Project: "alpha", Sprint: "01", Stage: "execute"}
	m = m.Update(ConfirmationMsg{Route: m.currentRoute(), Result: app.Confirmation{Request: req, Subject: "alpha/01", Runtime: true, Mutates: true, Warning: "RUNTIME + APPROVED TARGET MUTATION", Paths: []string{"projects/alpha/sprints/01"}}})
	view := Render(m, 80)
	for _, want := range []string{"CONFIRM OPERATION", "Runtime: true", "Mutates: true", "Affected path:", "Press Enter to confirm"} {
		if !strings.Contains(view, want) {
			t.Fatalf("confirmation missing %q:\n%s", want, view)
		}
	}
	m.Running = true
	m.Confirmation = nil
	m.Operation = &app.OperationResult{State: app.OperationRunning, Subject: "alpha/01"}
	for i := 0; i < 120; i++ {
		m = m.Update(OperationEventMsg{Event: app.OperationEvent{State: app.OperationRunning, Task: "task", Attempt: 2, Tokens: 456, TokensKnown: true, Duration: "12s", RuntimeEvents: 9, Provider: "opencode", Model: "demo", Cost: "0.02 USD"}})
	}
	if len(m.Events) != 100 {
		t.Fatalf("events=%d want 100", len(m.Events))
	}
	progressView := Render(m, 100)
	for _, want := range []string{"workflow_attempts=2 runtime_attempts=0 agent_turns=n/a tokens=456", "time=12s events=9 provider=opencode model=demo cost=0.02 USD"} {
		if !strings.Contains(progressView, want) {
			t.Fatalf("progress stats missing %q:\n%s", want, progressView)
		}
	}
	m = m.Update(OperationMsg{Route: m.currentRoute(), Result: app.OperationResult{State: app.OperationComplete, Subject: "alpha/01", Message: "done"}})
	if m.Running || m.Operation == nil || m.Confirmation != nil {
		t.Fatalf("terminal state=%+v", m)
	}
}

func TestFocusAndTabControls(t *testing.T) {
	model := NewModel(nil)
	model = model.Update(KeyMsg("tab"))
	if model.ActiveTab != TabStudies || model.currentRoute().Kind != RouteStudies || model.Focus != FocusContent {
		t.Fatalf("tab did not switch directly to studies: %+v", model)
	}
	model = model.Update(KeyMsg("tab"))
	if model.ActiveTab != TabProjects || model.currentRoute().Kind != RouteProjects || model.Focus != FocusContent {
		t.Fatalf("second tab did not switch directly to projects: %+v", model)
	}
	model.Focus = FocusTabs
	model = model.Update(KeyMsg("right"))
	if model.ActiveTab != TabStudies || model.currentRoute().Kind != RouteStudies {
		t.Fatalf("right tab route = %+v tab=%s", model.currentRoute(), model.ActiveTab)
	}
	model = model.Update(KeyMsg("left"))
	if model.ActiveTab != TabProjects || model.currentRoute().Kind != RouteProjects {
		t.Fatalf("left tab route = %+v tab=%s", model.currentRoute(), model.ActiveTab)
	}
}

func TestArrowNavigationMovesBetweenContentAndTabs(t *testing.T) {
	m := NewModel(nil)
	m.Loading = false
	m.Selected = 0
	m.Focus = FocusContent
	m = m.Update(KeyMsg("up"))
	if m.Focus != FocusTabs {
		t.Fatalf("up focus=%s", m.Focus)
	}
	m = m.Update(KeyMsg("down"))
	if m.Focus != FocusContent {
		t.Fatalf("down focus=%s", m.Focus)
	}
	m.Focus = FocusTabs
	m.ActiveTab = TabStudies
	m.Routes = []Route{{Kind: RouteStudies}}
	m = m.Update(KeyMsg("right"))
	if m.Focus != FocusContent {
		t.Fatalf("right-edge focus=%s", m.Focus)
	}
	m = m.Update(KeyMsg("tab"))
	if m.ActiveTab != TabProjects || m.Focus != FocusContent {
		t.Fatalf("tab switch tab=%s focus=%s", m.ActiveTab, m.Focus)
	}
}

func TestKeyBindingsExposeOperationalConfirmation(t *testing.T) {
	for _, key := range []string{"x", "!", "e", "g", "3", "y"} {
		if action := KeyToAction(key); action != ActionNone {
			t.Fatalf("key %q action = %s", key, action)
		}
	}
	for _, key := range []string{"q", "esc", "tab", "left", "right", "r", "enter", "1", "2", "c"} {
		if action := KeyToAction(key); action == ActionNone {
			t.Fatalf("key %q was not bound", key)
		}
	}
}

func TestTeaConfirmImmediatelyShowsRunningState(t *testing.T) {
	fake := &fakeUseCases{}
	m := newTeaModel(context.Background(), fake, 80)
	req := app.OperationRequest{Kind: app.OperationStudyStart, Study: "demo", Parallelism: 3}
	m.model.Loading = false
	m.model.Confirmation = &app.Confirmation{Request: req, Subject: "demo", Runtime: true, Mutates: true}
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("confirm did not start operation commands")
	}
	m = updated.(teaModel)
	if !m.model.Running || m.model.Confirmation != nil || m.model.Operation == nil || m.model.Operation.State != app.OperationRunning {
		t.Fatalf("confirm state=%+v", m.model)
	}
	if !strings.Contains(m.View(), "Run summary — demo") || !strings.Contains(m.View(), "Press c or q to cancel") {
		t.Fatalf("running view missing:\n%s", m.View())
	}
	if m.cancel != nil {
		m.cancel()
	}
}

func TestEscapeHidesForegroundRunWithoutCancelling(t *testing.T) {
	fake := &fakeUseCases{}
	m := newTeaModel(context.Background(), fake, 80)
	m.model.Loading = false
	m.model.Running = true
	m.model.ActiveOperation = app.OperationRequest{Kind: app.OperationStudyResume, Study: "demo"}
	m.model.Operation = &app.OperationResult{State: app.OperationRunning, Subject: "demo"}
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("escape did not request background refresh")
	}
	m = updated.(teaModel)
	if !m.model.Running || !m.model.OperationHidden || m.model.Operation == nil {
		t.Fatalf("detached state=%+v", m.model)
	}
	if !strings.Contains(m.View(), "Run continues in background") {
		t.Fatalf("background banner missing:\n%s", m.View())
	}
	m.model.ActiveTab = TabStudies
	m.model.Focus = FocusContent
	m.model.Routes = []Route{{Kind: RouteStudy, Study: "demo"}}
	m.model.Data.Studies = []app.StudySummary{{Name: "demo", RunActive: true, RunStatus: "active"}}
	m.model.Selected = 0
	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("hidden run could not reopen View Run")
	}
	m = updated.(teaModel)
	if m.model.RunViewStudy != "demo" {
		t.Fatalf("view run did not open: %+v", m.model)
	}
}

func TestBackNavigatesRoutesAfterRunIsBackgrounded(t *testing.T) {
	m := newTeaModel(context.Background(), &fakeUseCases{}, 80)
	m.model.Loading = false
	m.model.Running = true
	m.model.OperationHidden = true
	m.model.ActiveOperation = app.OperationRequest{Kind: app.OperationStudyResume, Study: "demo"}
	m.model.ActiveTab = TabStudies
	m.model.Focus = FocusContent
	m.model.Routes = []Route{{Kind: RouteStudies}, {Kind: RouteStudy, Study: "demo"}, {Kind: RouteStudyDims, Study: "demo"}}
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd != nil {
		t.Fatal("normal route back unexpectedly returned command")
	}
	m = updated.(teaModel)
	if got := m.model.currentRoute(); got.Kind != RouteStudy || got.Study != "demo" {
		t.Fatalf("back route=%+v", got)
	}
	if !m.model.Running || !m.model.OperationHidden {
		t.Fatalf("background run state changed: %+v", m.model)
	}
}

func TestCancelKeyWorksInsideRunView(t *testing.T) {
	m := newTeaModel(context.Background(), &fakeUseCases{}, 80)
	m.model.Loading = false
	m.model.RunViewStudy = "demo"
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	if cmd == nil {
		t.Fatal("c did not start cancellation")
	}
	m = updated.(teaModel)
	if !m.model.Running || m.model.RunViewStudy != "" || m.model.Operation == nil {
		t.Fatalf("cancel state=%+v", m.model)
	}
	if m.cancel != nil {
		m.cancel()
	}
}

func TestEnterTogglesPreviousRunsInsideRunView(t *testing.T) {
	m := newTeaModel(context.Background(), &fakeUseCases{}, 80)
	m.model.Loading = false
	m.model.RunViewStudy = "demo"
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Fatal("toggle unexpectedly returned command")
	}
	m = updated.(teaModel)
	if !m.model.RunViewShowPrevious {
		t.Fatal("enter did not expand previous runs")
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(teaModel)
	if m.model.RunViewShowPrevious {
		t.Fatal("second enter did not collapse previous runs")
	}
}

func TestRunLoopParallelParameterEntry(t *testing.T) {
	fake := &fakeUseCases{}
	m := newTeaModel(context.Background(), fake, 80)
	req := app.OperationRequest{Kind: app.OperationStudyResume, Study: "demo", Parallelism: 1}
	m.model.ParallelForm = &req
	for _, key := range []rune{'1', '2'} {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{key}})
		m = updated.(teaModel)
	}
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("valid parallelism did not proceed to confirmation")
	}
	m = updated.(teaModel)
	msg := cmd()
	updated, _ = m.Update(msg)
	m = updated.(teaModel)
	if m.model.Confirmation == nil || m.model.Confirmation.Request.Parallelism != 12 {
		t.Fatalf("confirmation=%+v", m.model.Confirmation)
	}
	m.model.Confirmation = nil
	m.model.ParallelForm = &req
	m.model.ParallelValue = "65"
	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(teaModel)
	if cmd != nil || m.model.ParallelError == "" {
		t.Fatalf("invalid parallelism state=%+v", m.model)
	}
}

func TestRunLoopParallelBlankUsesThree(t *testing.T) {
	fake := &fakeUseCases{}
	m := newTeaModel(context.Background(), fake, 80)
	req := app.OperationRequest{Kind: app.OperationStudyResume, Study: "demo", Parallelism: 3}
	m.model.ParallelForm = &req
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("blank default did not proceed")
	}
	m = updated.(teaModel)
	msg := cmd()
	updated, _ = m.Update(msg)
	m = updated.(teaModel)
	if m.model.Confirmation == nil || m.model.Confirmation.Request.Parallelism != 3 {
		t.Fatalf("default confirmation=%+v", m.model.Confirmation)
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
