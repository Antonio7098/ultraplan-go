package tui

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Antonio7098/ultraplan-go/internal/app"
)

type Tab string

const (
	TabProjects Tab = "projects"
	TabStudies  Tab = "studies"
	TabRuns     Tab = "runs"
)

type FocusArea string

const (
	FocusTabs    FocusArea = "tabs"
	FocusContent FocusArea = "content"
)

type RouteKind string

const (
	RouteProjects       RouteKind = "projects"
	RouteProject        RouteKind = "project"
	RouteProjectSprints RouteKind = "project-sprints"
	RouteProjectDocs    RouteKind = "project-docs"
	RouteSprint         RouteKind = "sprint"
	RouteSprintQA       RouteKind = "sprint-qa"
	RouteSprintQAShard  RouteKind = "sprint-qa-shard"
	RouteSprintQATheory RouteKind = "sprint-qa-theory"
	RouteSprintRepair   RouteKind = "sprint-repair"
	RouteStudies        RouteKind = "studies"
	RouteStudy          RouteKind = "study"
	RouteStudyDims      RouteKind = "study-dimensions"
	RouteStudySources   RouteKind = "study-sources"
	RouteRuns           RouteKind = "runs"
	RouteRun            RouteKind = "run"
)

type Route struct {
	Kind    RouteKind
	Project string
	Sprint  string
	Study   string
	RunID   string
	Shard   string
	Theory  string
}

type Model struct {
	UseCases              app.OperationalUseCases
	Data                  app.DashboardResult
	ActiveTab             Tab
	Focus                 FocusArea
	Routes                []Route
	Selected              int
	Preview               *app.ArtifactPreviewResult
	PreviewTitle          string
	Error                 string
	Loading               bool
	Quit                  bool
	PreviewOffset         int
	Validation            *app.ValidationOperationResult
	Confirmation          *app.Confirmation
	Operation             *app.OperationResult
	Events                []app.OperationEvent
	Running               bool
	RunViewStudy          string
	RunViewShowPrevious   bool
	ActiveOperation       app.OperationRequest
	OperationShowPrevious bool
	OperationHidden       bool
	ParallelForm          *app.OperationRequest
	ParallelValue         string
	ParallelError         string
	ActiveRunID           string
	Runs                  []app.RunSnapshot
	DurableEvents         []app.RunEvent
}

type Message interface{}

type LoadMsg struct {
	Result app.DashboardResult
	Runs   []app.RunSnapshot
	Events []app.RunEvent
	Err    error
}
type RefreshMsg struct {
	Result app.DashboardResult
	Runs   []app.RunSnapshot
	Events []app.RunEvent
	Err    error
}
type PreviewMsg struct {
	Result app.ArtifactPreviewResult
	Err    error
	Route  Route
	Title  string
}
type KeyMsg string
type ValidationMsg struct {
	Result app.ValidationOperationResult
	Err    error
	Route  Route
}
type ConfirmationMsg struct {
	Result app.Confirmation
	Err    error
	Route  Route
}
type OperationMsg struct {
	Result app.OperationResult
	Err    error
	Route  Route
	Events []app.OperationEvent
}
type OperationEventMsg struct{ Event app.OperationEvent }

type navItem struct {
	Label      string
	Route      *Route
	Path       string
	Validation *app.ValidationRequest
	Operation  *app.OperationRequest
	ViewRun    string
}

func NewModel(useCases app.OperationalUseCases) Model {
	return Model{
		UseCases:  useCases,
		ActiveTab: TabProjects,
		Focus:     FocusContent,
		Routes:    []Route{{Kind: RouteProjects}},
		Loading:   true,
	}
}

func (m Model) Load(ctx context.Context) (Model, error) {
	result, err := m.UseCases.Dashboard(ctx)
	return m.Update(LoadMsg{Result: result, Err: err}), err
}

func (m Model) Refresh(ctx context.Context) (Model, error) {
	result, err := m.UseCases.Dashboard(ctx)
	return m.Update(RefreshMsg{Result: result, Err: err}), err
}

func (m Model) PreviewSelected(ctx context.Context) (Model, error) {
	item, ok := m.selectedItem()
	if !ok || item.Path == "" {
		return m.Update(PreviewMsg{Result: app.ArtifactPreviewResult{Error: "no previewable artifact selected"}, Route: m.currentRoute(), Title: "Preview"}), nil
	}
	result, err := m.UseCases.PreviewArtifact(ctx, item.Path)
	return m.Update(PreviewMsg{Result: result, Err: err, Route: m.currentRoute(), Title: item.Label}), err
}

func (m Model) Update(msg Message) Model {
	switch v := msg.(type) {
	case LoadMsg:
		m.Loading = false
		if v.Err != nil {
			m.Error = v.Err.Error()
			return m
		}
		m.Data = v.Result
		m.Runs = v.Runs
		m.DurableEvents = dedupeDurableEvents(v.Events)
		m.Error = ""
		m.clampSelection()
	case RefreshMsg:
		m.Loading = false
		m.Preview = nil
		m.PreviewOffset = 0
		if v.Err != nil {
			m.Error = v.Err.Error()
			return m
		}
		m.Data = v.Result
		m.Runs = v.Runs
		m.DurableEvents = dedupeDurableEvents(v.Events)
		m.Error = ""
		m.clampSelection()
	case PreviewMsg:
		if v.Route != (Route{}) && v.Route != m.currentRoute() {
			return m
		}
		if v.Err != nil {
			m.Error = v.Err.Error()
			return m
		}
		m.Preview = &v.Result
		m.PreviewTitle = v.Title
		m.Error = ""
		m.PreviewOffset = 0
	case ValidationMsg:
		m.Loading = false
		if v.Route != m.currentRoute() {
			return m
		}
		if v.Err != nil {
			m.Error = v.Err.Error()
			return m
		}
		m.Validation = &v.Result
		m.Error = ""
	case ConfirmationMsg:
		m.Loading = false
		if v.Route != m.currentRoute() {
			return m
		}
		if v.Err != nil {
			m.Error = v.Err.Error()
			return m
		}
		m.Confirmation = &v.Result
	case OperationEventMsg:
		if !m.Running {
			return m
		}
		m.Events = append(m.Events, v.Event)
		if len(m.Events) > 100 {
			m.Events = m.Events[len(m.Events)-100:]
		}
	case OperationMsg:
		m.Running = false
		m.Loading = false
		m.Confirmation = nil
		m.Operation = &v.Result
		m.Events = append(m.Events, v.Events...)
		if len(m.Events) > 100 {
			m.Events = m.Events[len(m.Events)-100:]
		}
		if v.Err != nil {
			m.Error = v.Err.Error()
		}
	case KeyMsg:
		switch KeyToAction(string(v)) {
		case ActionQuit:
			m.Quit = true
		case ActionFocusNext:
			if m.ActiveTab == TabProjects {
				m.setTab(TabStudies)
			} else {
				m.setTab(TabProjects)
			}
		case ActionBack:
			if m.Running && !m.OperationHidden {
				return m
			} else if m.ParallelForm != nil {
				m.ParallelForm = nil
				m.ParallelValue = ""
				m.ParallelError = ""
			} else if m.RunViewStudy != "" {
				m.RunViewStudy = ""
				m.RunViewShowPrevious = false
				m.Operation = nil
				m.Events = nil
			} else if m.Confirmation != nil {
				m.Confirmation = nil
			} else if m.Operation != nil && !m.OperationHidden {
				m.Operation = nil
				m.Events = nil
				m.ActiveOperation = app.OperationRequest{}
				m.OperationShowPrevious = false
			} else if m.Validation != nil {
				m.Validation = nil
			} else if m.Preview != nil {
				m.Preview = nil
				m.PreviewOffset = 0
			} else if m.Focus == FocusContent && len(m.Routes) > 1 {
				m.Routes = m.Routes[:len(m.Routes)-1]
				m.Selected = 0
			}
		case ActionUp:
			if m.Preview != nil {
				m.PreviewOffset--
				if m.PreviewOffset < 0 {
					m.PreviewOffset = 0
				}
			} else if m.Focus == FocusContent && m.Selected > 0 {
				m.Selected--
			} else if m.Focus == FocusContent && m.Selected == 0 {
				m.Focus = FocusTabs
			}
		case ActionDown:
			if m.Preview != nil {
				m.PreviewOffset++
			} else if m.Focus == FocusTabs {
				m.Focus = FocusContent
			} else if m.Focus == FocusContent {
				m.Selected++
				m.clampSelection()
			}
		case ActionLeft:
			if m.Focus == FocusTabs {
				if m.ActiveTab == TabRuns {
					m.setTab(TabStudies)
				} else {
					m.setTab(TabProjects)
				}
				m.Focus = FocusTabs
			}
		case ActionRight:
			if m.Focus == FocusTabs {
				if m.ActiveTab == TabProjects {
					m.setTab(TabStudies)
					m.Focus = FocusTabs
				} else {
					m.Focus = FocusContent
				}
			}
		case ActionProjects:
			m.setTab(TabProjects)
		case ActionStudies:
			m.setTab(TabStudies)
		case ActionRuns:
			m.setTab(TabRuns)
		case ActionOpen:
			if m.Preview != nil {
				return m
			}
			if m.Focus == FocusTabs {
				m.Focus = FocusContent
				return m
			}
			if item, ok := m.selectedItem(); ok && item.Route != nil {
				m.Routes = append(m.Routes, *item.Route)
				m.Selected = 0
			}
		}
	}
	return m
}

func dedupeDurableEvents(events []app.RunEvent) []app.RunEvent {
	seen := make(map[string]bool, len(events))
	result := make([]app.RunEvent, 0, min(len(events), 200))
	for _, event := range events {
		key := string(event.RunID) + ":" + fmt.Sprint(event.Sequence)
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, event)
	}
	if len(result) > 200 {
		result = result[len(result)-200:]
	}
	return result
}

func (m *Model) setTab(tab Tab) {
	m.ActiveTab = tab
	m.Focus = FocusContent
	m.Selected = 0
	m.Preview = nil
	m.PreviewOffset = 0
	if tab == TabStudies {
		m.Routes = []Route{{Kind: RouteStudies}}
		return
	}
	if tab == TabRuns {
		m.Routes = []Route{{Kind: RouteRuns}}
		return
	}
	m.Routes = []Route{{Kind: RouteProjects}}
}

func (m *Model) clampSelection() {
	max := len(m.navItems()) - 1
	if max < 0 {
		m.Selected = 0
		return
	}
	if m.Selected > max {
		m.Selected = max
	}
}

func (m Model) currentRoute() Route {
	if len(m.Routes) == 0 {
		if m.ActiveTab == TabStudies {
			return Route{Kind: RouteStudies}
		}
		if m.ActiveTab == TabRuns {
			return Route{Kind: RouteRuns}
		}
		return Route{Kind: RouteProjects}
	}
	return m.Routes[len(m.Routes)-1]
}

func (m Model) selectedItem() (navItem, bool) {
	items := m.navItems()
	if m.Selected < 0 || m.Selected >= len(items) {
		return navItem{}, false
	}
	return items[m.Selected], true
}

func (m Model) navItems() []navItem {
	route := m.currentRoute()
	switch route.Kind {
	case RouteProjects:
		items := make([]navItem, 0, len(m.Data.Projects))
		for _, p := range m.Data.Projects {
			items = append(items, navItem{Label: p.Name, Route: &Route{Kind: RouteProject, Project: p.Name}})
		}
		return items
	case RouteProject:
		return []navItem{
			{Label: "Sprints", Route: &Route{Kind: RouteProjectSprints, Project: route.Project}},
			{Label: "Docs", Route: &Route{Kind: RouteProjectDocs, Project: route.Project}},
			{Label: "Project Index", Path: projectArtifactPath(m.Data.Projects, route.Project, "project-index")},
			{Label: "Roadmap", Path: projectArtifactPath(m.Data.Projects, route.Project, "roadmap")},
			{Label: "Validate Project", Validation: &app.ValidationRequest{Subject: app.ValidationProject, Project: route.Project}},
		}
	case RouteProjectSprints:
		var items []navItem
		for _, s := range m.Data.Sprints {
			if s.Project == route.Project {
				sprintRoute := Route{Kind: RouteSprint, Project: s.Project, Sprint: s.Slug}
				items = append(items, navItem{Label: s.Slug, Route: &sprintRoute})
			}
		}
		return items
	case RouteProjectDocs:
		if p, ok := findProject(m.Data.Projects, route.Project); ok {
			var items []navItem
			for _, artifact := range p.Artifacts {
				if artifact.Label == "doc" {
					items = append(items, navItem{Label: filepath.Base(artifact.Path), Path: artifact.Path})
				}
			}
			return items
		}
	case RouteSprint:
		if s, ok := findSprint(m.Data.Sprints, route.Project, route.Sprint); ok {
			stageLabel := func(stage string) string {
				if stage == "review" {
					return "Conformance Review"
				}
				return stage
			}
			items := []navItem{
				{Label: "Requirements", Path: artifactByLabel(s.Artifacts, "requirements")},
				{Label: "Sprint Index", Path: artifactByLabel(s.Artifacts, "sprint-index")},
				{Label: "Technical Handbook", Path: artifactByLabel(s.Artifacts, "technical-handbook")},
				{Label: "Reasoning", Path: artifactByLabel(s.Artifacts, "reasoning")},
				{Label: "Plan", Path: artifactByLabel(s.Artifacts, "plan")},
				{Label: "Execute", Path: artifactByLabel(s.Artifacts, "execute")},
				{Label: "Conformance Review", Path: artifactByLabel(s.Artifacts, "review")},
				{Label: "QA report", Path: artifactByLabel(s.Artifacts, "qa")},
				{Label: "Smoke", Path: artifactByLabel(s.Artifacts, "smoke")},
				{Label: "Merge", Path: artifactByLabel(s.Artifacts, "merge")},
				{Label: "Flow State", Path: artifactByLabel(s.Artifacts, "flow-state")},
				{Label: "Run State", Path: artifactByLabel(s.Artifacts, "run-state")},
				{Label: "QA", Route: &Route{Kind: RouteSprintQA, Project: route.Project, Sprint: route.Sprint}},
				{Label: "Bounded repair", Route: &Route{Kind: RouteSprintRepair, Project: route.Project, Sprint: route.Sprint}},
			}
			stages := []string{"requirements", "sprint-index", "technical-handbook", "area-reasoning", "reasoning", "plan", "execute", "review"}
			items = append(items, navItem{Label: "Sprint Status", Operation: &app.OperationRequest{Kind: app.OperationSprintStatus, Project: route.Project, Sprint: route.Sprint}})
			for _, stage := range stages {
				items = append(items, navItem{Label: "Validate " + stageLabel(stage), Validation: &app.ValidationRequest{Subject: app.ValidationSprint, Project: route.Project, Sprint: route.Sprint, Stage: stage}})
				items = append(items, navItem{Label: "Preview " + stageLabel(stage) + " Prompt", Operation: &app.OperationRequest{Kind: app.OperationPrompt, Project: route.Project, Sprint: route.Sprint, Stage: stage}})
			}
			items = append(items,
				navItem{Label: "Dry Run Flow to QA", Operation: &app.OperationRequest{Kind: app.OperationFlowDryRun, Project: route.Project, Sprint: route.Sprint, Stage: "qa"}},
				navItem{Label: "Run Flow to QA [RUNTIME]", Operation: &app.OperationRequest{Kind: app.OperationFlow, Project: route.Project, Sprint: route.Sprint, Stage: "qa"}},
				navItem{Label: "Inspect Flow to merge", Operation: &app.OperationRequest{Kind: app.OperationFlowDryRun, Project: route.Project, Sprint: route.Sprint, Stage: "merge"}},
				navItem{Label: "Run Flow to merge [RUNTIME + GIT]", Operation: &app.OperationRequest{Kind: app.OperationFlow, Project: route.Project, Sprint: route.Sprint, Stage: "merge"}},
				navItem{Label: "Validate merge", Validation: &app.ValidationRequest{Subject: app.ValidationSprint, Project: route.Project, Sprint: route.Sprint, Stage: "merge"}})
			for _, stage := range stages {
				items = append(items,
					navItem{Label: "Dry Run Flow to " + stageLabel(stage), Operation: &app.OperationRequest{Kind: app.OperationFlowDryRun, Project: route.Project, Sprint: route.Sprint, Stage: stage}},
					navItem{Label: "Run Flow to " + stageLabel(stage) + " [RUNTIME]", Operation: &app.OperationRequest{Kind: app.OperationFlow, Project: route.Project, Sprint: route.Sprint, Stage: stage}})
			}
			items = append(items,
				navItem{Label: "Execute Status", Operation: &app.OperationRequest{Kind: app.OperationExecuteStatus, Project: route.Project, Sprint: route.Sprint, Stage: "execute"}},
				navItem{Label: "Execute Dry Run", Operation: &app.OperationRequest{Kind: app.OperationExecuteDryRun, Project: route.Project, Sprint: route.Sprint, Stage: "execute"}},
				navItem{Label: "Execute Start [RUNTIME]", Operation: &app.OperationRequest{Kind: app.OperationExecuteStart, Project: route.Project, Sprint: route.Sprint, Stage: "execute"}},
				navItem{Label: "Execute Resume [RUNTIME]", Operation: &app.OperationRequest{Kind: app.OperationExecuteResume, Project: route.Project, Sprint: route.Sprint, Stage: "execute"}})
			items = append(items,
				navItem{Label: "Conformance Review Status", Operation: &app.OperationRequest{Kind: app.OperationReviewStatus, Project: route.Project, Sprint: route.Sprint, Stage: "review"}},
				navItem{Label: "Conformance Review Dry Run", Operation: &app.OperationRequest{Kind: app.OperationReviewDryRun, Project: route.Project, Sprint: route.Sprint, Stage: "review"}},
				navItem{Label: "Run/Resume Conformance Review [RUNTIME]", Operation: &app.OperationRequest{Kind: app.OperationReviewStart, Project: route.Project, Sprint: route.Sprint, Stage: "review"}},
				navItem{Label: "Restart Conformance Review [RUNTIME]", Operation: &app.OperationRequest{Kind: app.OperationReviewStart, Project: route.Project, Sprint: route.Sprint, Stage: "review", RestartReview: true}})
			items = append(items,
				navItem{Label: "Verify to Conformance Review Preview", Operation: &app.OperationRequest{Kind: app.OperationVerifyDryRun, Project: route.Project, Sprint: route.Sprint, Stage: "review"}},
				navItem{Label: "Verify to Conformance Review [RUNTIME]", Operation: &app.OperationRequest{Kind: app.OperationVerifyStart, Project: route.Project, Sprint: route.Sprint, Stage: "review"}},
				navItem{Label: "Verify to Smoke Preview", Operation: &app.OperationRequest{Kind: app.OperationVerifyDryRun, Project: route.Project, Sprint: route.Sprint, Stage: "smoke"}},
				navItem{Label: "Verify to Smoke [RUNTIME + EXTERNAL]", Operation: &app.OperationRequest{Kind: app.OperationVerifyStart, Project: route.Project, Sprint: route.Sprint, Stage: "smoke"}},
				navItem{Label: "Validate smoke", Validation: &app.ValidationRequest{Subject: app.ValidationSprint, Project: route.Project, Sprint: route.Sprint, Stage: "smoke"}},
				navItem{Label: "Smoke Status", Operation: &app.OperationRequest{Kind: app.OperationSmokeStatus, Project: route.Project, Sprint: route.Sprint, Stage: "smoke"}},
				navItem{Label: "Smoke Preview", Operation: &app.OperationRequest{Kind: app.OperationSmokeDryRun, Project: route.Project, Sprint: route.Sprint, Stage: "smoke"}},
				navItem{Label: "Run Smoke [EXTERNAL]", Operation: &app.OperationRequest{Kind: app.OperationSmokeStart, Project: route.Project, Sprint: route.Sprint, Stage: "smoke"}},
				navItem{Label: "Run Smoke Diagnostic Override [EXTERNAL]", Operation: &app.OperationRequest{Kind: app.OperationSmokeStart, Project: route.Project, Sprint: route.Sprint, Stage: "smoke", ForceReview: true, OverrideRationale: "operator requested guarded TUI diagnostic evidence"}})
			return items
		}
	case RouteSprintQA:
		if s, ok := findSprint(m.Data.Sprints, route.Project, route.Sprint); ok {
			items := []navItem{
				{Label: "QA Status", Operation: &app.OperationRequest{Kind: app.OperationQAStatus, Project: route.Project, Sprint: route.Sprint}},
				{Label: "QA Dry Run", Operation: &app.OperationRequest{Kind: app.OperationQADryRun, Project: route.Project, Sprint: route.Sprint}},
				{Label: "Start QA [RUNTIME]", Operation: &app.OperationRequest{Kind: app.OperationQAStart, Project: route.Project, Sprint: route.Sprint}},
				{Label: "Resume QA [RUNTIME]", Operation: &app.OperationRequest{Kind: app.OperationQAResume, Project: route.Project, Sprint: route.Sprint}},
				{Label: "Recover QA", Operation: &app.OperationRequest{Kind: app.OperationQARecover, Project: route.Project, Sprint: route.Sprint}},
			}
			if s.QA.RunID != "" {
				items = append(items, navItem{Label: "View QA durable run  " + s.QA.RunID, Route: &Route{Kind: RouteRun, RunID: s.QA.RunID}})
			}
			for _, shard := range s.QA.Shards {
				items = append(items, navItem{Label: shard.Title + "  " + shard.Phase, Route: &Route{Kind: RouteSprintQAShard, Project: route.Project, Sprint: route.Sprint, Shard: shard.ID}})
			}
			return items
		}
	case RouteSprintQAShard:
		items := []navItem{
			{Label: "Run focused shard [RUNTIME]", Operation: &app.OperationRequest{Kind: app.OperationQAStart, Project: route.Project, Sprint: route.Sprint, Task: route.Shard}},
			{Label: "Resume focused shard [RUNTIME]", Operation: &app.OperationRequest{Kind: app.OperationQAResume, Project: route.Project, Sprint: route.Sprint, Task: route.Shard}},
		}
		if s, ok := findSprint(m.Data.Sprints, route.Project, route.Sprint); ok {
			for _, shard := range s.QA.Shards {
				if shard.ID != route.Shard {
					continue
				}
				for _, theory := range shard.Theories {
					items = append(items, navItem{Label: theory.Claim + "  " + theory.Outcome, Route: &Route{Kind: RouteSprintQATheory, Project: route.Project, Sprint: route.Sprint, Shard: shard.ID, Theory: theory.ID}})
				}
			}
		}
		return items
	case RouteSprintRepair:
		if s, ok := findSprint(m.Data.Sprints, route.Project, route.Sprint); ok {
			items := []navItem{}
			switch s.Repair.Phase {
			case "proposing", "applying", "reverifying", "cleaning", "interrupted":
				items = append(items, navItem{Label: "Recover interrupted repair [MUTATION SAFETY]", Operation: &app.OperationRequest{Kind: app.OperationRepairRecover, Project: route.Project, Sprint: route.Sprint, RepairRunID: s.Repair.RepairRunID}})
			}
			if s.Repair.Packet != nil && s.Repair.Confirmation == nil && s.Repair.Fresh {
				items = append(items, navItem{Label: "Review and confirm manual repair [RUNTIME + MUTATION]", Operation: &app.OperationRequest{Kind: app.OperationRepairStart, Project: route.Project, Sprint: route.Sprint, RepairRunID: s.Repair.RepairRunID, RepairMode: "manual", RepairConfirmer: "tui-session"}})
			}
			if s.Repair.OperationRunID != "" {
				items = append(items, navItem{Label: "View repair durable run  " + s.Repair.OperationRunID, Route: &Route{Kind: RouteRun, RunID: s.Repair.OperationRunID}})
			}
			return items
		}
	case RouteStudies:
		items := make([]navItem, 0, len(m.Data.Studies))
		for _, s := range m.Data.Studies {
			items = append(items, navItem{Label: s.Name, Route: &Route{Kind: RouteStudy, Study: s.Name}})
		}
		return items
	case RouteStudy:
		var items []navItem
		if summary, ok := findStudy(m.Data.Studies, route.Study); ok && summary.RunActive {
			items = append(items, navItem{Label: "View Run [ACTIVE]", ViewRun: route.Study})
		} else {
			items = append(items, navItem{Label: "Run Loop [RUNTIME]", Operation: &app.OperationRequest{Kind: app.OperationStudyResume, Study: route.Study, Parallelism: 3}})
		}
		items = append(items,
			navItem{Label: "Dimensions", Route: &Route{Kind: RouteStudyDims, Study: route.Study}},
			navItem{Label: "Sources", Route: &Route{Kind: RouteStudySources, Study: route.Study}},
			navItem{Label: "Run State", Path: studyArtifactPath(m.Data.Studies, route.Study, "run-state")},
			navItem{Label: "Validate Study", Validation: &app.ValidationRequest{Subject: app.ValidationStudy, Study: route.Study}},
		)
		return items
	case RouteStudyDims:
		if s, ok := findStudy(m.Data.Studies, route.Study); ok {
			return artifactItemsByLabel(s.Artifacts, "dimension")
		}
	case RouteStudySources:
		if s, ok := findStudy(m.Data.Studies, route.Study); ok {
			return artifactItemsByLabel(s.Artifacts, "source")
		}
	case RouteRuns:
		items := make([]navItem, 0, len(m.Runs))
		for _, run := range m.Runs {
			label := string(run.RunID) + "  " + string(run.Lifecycle) + "  " + run.Target.Kind + "/" + run.Target.Operation
			items = append(items, navItem{Label: label, Route: &Route{Kind: RouteRun, RunID: string(run.RunID)}})
		}
		return items
	}
	return nil
}

func (m Model) breadcrumb() string {
	route := m.currentRoute()
	switch route.Kind {
	case RouteProject:
		return "Projects > " + route.Project
	case RouteProjectSprints:
		return "Projects > " + route.Project + " > Sprints"
	case RouteProjectDocs:
		return "Projects > " + route.Project + " > Docs"
	case RouteSprint:
		return "Projects > " + route.Project + " > Sprints > " + route.Sprint
	case RouteSprintQA:
		return "Projects > " + route.Project + " > Sprints > " + route.Sprint + " > QA"
	case RouteSprintQAShard:
		return "Projects > " + route.Project + " > Sprints > " + route.Sprint + " > QA > " + route.Shard
	case RouteSprintQATheory:
		return "Projects > " + route.Project + " > Sprints > " + route.Sprint + " > QA > " + route.Theory
	case RouteSprintRepair:
		return "Projects > " + route.Project + " > Sprints > " + route.Sprint + " > Bounded repair"
	case RouteStudy:
		return "Studies > " + route.Study
	case RouteStudyDims:
		return "Studies > " + route.Study + " > Dimensions"
	case RouteStudySources:
		return "Studies > " + route.Study + " > Sources"
	case RouteStudies:
		return "Studies"
	case RouteRuns:
		return "Runs"
	case RouteRun:
		return "Runs > " + route.RunID
	default:
		return "Projects"
	}
}

func projectArtifactPath(projects []app.ProjectSummary, project, label string) string {
	if p, ok := findProject(projects, project); ok {
		return artifactByLabel(p.Artifacts, label)
	}
	return ""
}

func studyArtifactPath(studies []app.StudySummary, study, label string) string {
	if s, ok := findStudy(studies, study); ok {
		return artifactByLabel(s.Artifacts, label)
	}
	return ""
}

func artifactItemsByLabel(artifacts []app.DisplayArtifact, label string) []navItem {
	var items []navItem
	for _, artifact := range artifacts {
		if artifact.Label == label {
			items = append(items, navItem{Label: strings.TrimSuffix(filepath.Base(artifact.Path), filepath.Ext(artifact.Path)), Path: artifact.Path})
		}
	}
	return items
}

func artifactByLabel(artifacts []app.DisplayArtifact, label string) string {
	for _, artifact := range artifacts {
		if artifact.Label == label {
			return artifact.Path
		}
	}
	return ""
}

func findProject(projects []app.ProjectSummary, name string) (app.ProjectSummary, bool) {
	for _, p := range projects {
		if p.Name == name {
			return p, true
		}
	}
	return app.ProjectSummary{}, false
}

func findSprint(sprints []app.SprintSummary, project, slug string) (app.SprintSummary, bool) {
	for _, s := range sprints {
		if s.Project == project && s.Slug == slug {
			return s, true
		}
	}
	return app.SprintSummary{}, false
}

func findStudy(studies []app.StudySummary, name string) (app.StudySummary, bool) {
	for _, s := range studies {
		if s.Name == name {
			return s, true
		}
	}
	return app.StudySummary{}, false
}
