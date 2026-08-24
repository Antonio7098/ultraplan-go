package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

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
	Name              string `json:"name"`
	Status            string `json:"status"`
	Error             string `json:"error,omitempty"`
	ArtifactAvailable bool   `json:"artifact_available,omitempty"`
	ArtifactValid     bool   `json:"artifact_valid,omitempty"`
	LatestOutcome     string `json:"latest_outcome,omitempty"`
	NextAction        string `json:"next_action,omitempty"`
}

type promptInputContractDTO struct {
	Stage     string   `json:"stage"`
	Required  []string `json:"required"`
	Optional  []string `json:"optional,omitempty"`
	Forbidden []string `json:"forbidden,omitempty"`
}

type promptBlockDTO struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Mode      string `json:"mode,omitempty"`
	Cacheable bool   `json:"cacheable"`
	Bytes     int    `json:"bytes"`
	Digest    string `json:"sha256"`
	Content   string `json:"content"`
}

type promptBundleDTO struct {
	Stage                string                 `json:"stage"`
	Available            bool                   `json:"available"`
	Scope                string                 `json:"scope"`
	UnavailableReason    string                 `json:"unavailable_reason,omitempty"`
	InputContract        promptInputContractDTO `json:"input_contract"`
	SchemaVersion        int                    `json:"schema_version,omitempty"`
	TotalBytes           int                    `json:"total_bytes,omitempty"`
	SharedPrefixBytes    int                    `json:"shared_prefix_bytes,omitempty"`
	StageSuffixBytes     int                    `json:"stage_suffix_bytes,omitempty"`
	SharedPrefixDigest   string                 `json:"shared_prefix_sha256,omitempty"`
	CacheKey             string                 `json:"cache_key,omitempty"`
	CacheBreakpointBytes int                    `json:"cache_breakpoint_bytes,omitempty"`
	CacheCandidate       bool                   `json:"cache_candidate"`
	CacheTransport       string                 `json:"cache_transport,omitempty"`
	Blocks               []promptBlockDTO       `json:"blocks"`
}

type projectDTO struct {
	Ref       string            `json:"ref"`
	Name      string            `json:"name"`
	Docs      []string          `json:"docs"`
	Findings  []findingDTO      `json:"findings"`
	Artifacts []artifactDTO     `json:"artifacts"`
	Sprints   []sprintDTO       `json:"sprints"`
	Roadmap   []roadmapPhaseDTO `json:"roadmap"`
}

type roadmapPhaseDTO struct {
	Title   string             `json:"title"`
	Sprints []roadmapSprintDTO `json:"sprints"`
}

type roadmapSprintDTO struct {
	Number          int      `json:"number"`
	Title           string   `json:"title"`
	Slug            string   `json:"slug,omitempty"`
	Status          string   `json:"status"`
	Goal            string   `json:"goal,omitempty"`
	GateItems       []string `json:"acceptance,omitempty"`
	DependsOn       []int    `json:"depends_on,omitempty"`
	Exists          bool     `json:"exists"`
	Assessment      string   `json:"assessment,omitempty"`
	CompletedStages int      `json:"completed_stages,omitempty"`
	TotalStages     int      `json:"total_stages,omitempty"`
	CurrentStage    string   `json:"current_stage,omitempty"`
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
	StartedAt *time.Time    `json:"started_at,omitempty"`
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

type studyRetryDTO struct {
	RetriedTasks int        `json:"retried_tasks"`
	TotalRetries int        `json:"total_retries"`
	SameSession  int        `json:"same_session"`
	FreshSession int        `json:"fresh_session"`
	Waiting      int        `json:"waiting,omitempty"`
	NextRetryAt  *time.Time `json:"next_retry_at,omitempty"`
}

type studyParallelismDTO struct {
	Decreased            bool `json:"decreased"`
	Events               int  `json:"events,omitempty"`
	RequestedParallelism int  `json:"requested_parallelism,omitempty"`
	EffectiveParallelism int  `json:"effective_parallelism,omitempty"`
}

type studyTaskRetryDTO struct {
	ID           string `json:"id"`
	Kind         string `json:"kind,omitempty"`
	Status       string `json:"status,omitempty"`
	Retries      int    `json:"retries"`
	SessionReuse string `json:"session_reuse,omitempty"`
}

type studyDTO struct {
	Ref          string                `json:"ref"`
	Name         string                `json:"name"`
	Sources      []string              `json:"sources"`
	Dimensions   []string              `json:"dimensions"`
	Status       string                `json:"status"`
	RunID        string                `json:"run_id,omitempty"`
	Total        int                   `json:"total"`
	Completed    int                   `json:"completed"`
	Failed       int                   `json:"failed"`
	RunActive    bool                  `json:"run_active"`
	ActiveTasks  int                   `json:"active_tasks"`
	Pending      int                   `json:"pending"`
	Cancelled    int                   `json:"cancelled"`
	Retries      studyRetryDTO         `json:"retries"`
	Parallelism  *studyParallelismDTO  `json:"parallelism,omitempty"`
	RetriedTasks []studyTaskRetryDTO   `json:"retried_tasks,omitempty"`
	Failures     []studyTaskFailureDTO `json:"failures,omitempty"`
	Findings     []findingDTO          `json:"findings"`
	Artifacts    []artifactDTO         `json:"artifacts"`
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

// dimensionCard is one expandable card on the dimensions browse page.
type dimensionCard struct {
	Study                 string
	Number                string
	Slug                  string
	Title                 string
	DisplayPath           string
	Bytes                 int64
	Truncated             bool
	CodeCitationsDisabled bool
	Body                  template.HTML
}

// reportLinkView is one report file link on the study reports page.
type reportLinkView struct {
	Label       string
	Ref         string
	DisplayPath string
	Bytes       int64
}

// reportGroupView holds one dimension's per-source report links.
type reportGroupView struct {
	Dimension string
	Reports   []reportLinkView
}

// repoBoardEntry is one ranked row of the study repo leaderboard.
type repoBoardEntry struct {
	Rank     int
	Name     string
	Average  string
	BarWidth int
	Coverage string
	BestRef  string
	BestText string
}

// repoMatrixColumn is one dimension column of the study repo score matrix.
type repoMatrixColumn struct {
	Number string
	Label  string
}

// repoMatrixCell is one score cell of the study repo score matrix.
type repoMatrixCell struct {
	Label   string
	Scored  bool
	Leader  bool
	Percent int
	Ref     string
}

// repoMatrixRow is one repo row of the study repo score matrix.
type repoMatrixRow struct {
	Name    string
	Average string
	Cells   []repoMatrixCell
}

// repoLeaderView is one ranked leader inside a dimension champion list.
type repoLeaderView struct {
	Rank  int
	Name  string
	Score int
	Ref   string
}

// repoChampion names the leading repos for one dimension.
type repoChampion struct {
	Dimension string
	Leaders   []repoLeaderView
}

type pageModel struct {
	Title             string
	Heading           string
	Description       string
	Workspace         string
	Projects          []app.WebProjectResult
	Project           *app.WebProjectResult
	Sprints           []app.WebSprintResult
	Sprint            *app.WebSprintResult
	Studies           []app.WebStudyResult
	Study             *app.WebStudyResult
	Artifact          *app.WebArtifactPreview
	ArtifactHTML      template.HTML
	SmokeArtifact     bool
	Health            *app.WebHealthResult
	Status            int
	Error             string
	CSRF              string
	Preparation       *operationPreparationView
	Operation         *operationDocument
	Runs              []runRowView
	Run               *runDetailView
	DimensionList     []dimensionCard
	ReportFinals      []reportLinkView
	ReportGroups      []reportGroupView
	ReportSourceCount int
	RepoBoard         []repoBoardEntry
	RepoColumns       []repoMatrixColumn
	RepoMatrix        []repoMatrixRow
	RepoChampions     []repoChampion
	StudyInsights     *runStudyInsightsView
	SprintUsage       *runSprintUsageView
	RunEvents         []runEventView
	NextRunsURL       string
	NextEventsURL     string
	RunFilters        runPageFilters
	Page              string
}

func (h *handler) dispatch(w http.ResponseWriter, r *http.Request, match routeMatch) {
	allowsQuery := match.name == "api_validations" || match.name == "api_runs" || match.name == "api_run_events" || match.name == "runs" || match.name == "api_timeline"
	if !allowsQuery && r.URL.RawQuery != "" {
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
		if page == "sprints" || page == "roadmap" {
			page = "roadmap"
			result.Sprints = newestSprintsFirst(result.Sprints)
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
		h.render(w, r, http.StatusOK, "sprint", pageModel{Title: artifact.DisplayPath + " · " + result.Slug, Heading: result.Slug, Sprint: &result, Artifact: &artifact, ArtifactHTML: renderMarkdown(artifact), SmokeArtifact: artifact.DisplayPath == "smoke.md" || strings.HasSuffix(artifact.DisplayPath, "/smoke.md"), Page: "artifacts"})
	case "studies":
		result, err := h.queries.Studies(r.Context())
		if err != nil {
			h.handleQueryError(w, r, false, err)
			return
		}
		h.render(w, r, http.StatusOK, "studies", pageModel{Title: "Studies", Heading: "Studies", Studies: result.Items})
	case "runs":
		h.handleRunsPage(w, r)
	case "run":
		h.handleRunPage(w, r, match.params[0])
	case "run_cancel":
		h.handleRunPageCancel(w, r, match.params[0])
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
		model := pageModel{Title: studyPageTitle(page) + " · " + result.Name, Heading: result.Name, Study: &result, Page: page}
		if page == "dimensions" {
			cards, err := h.studyDimensionCards(r, match.params[0])
			if err != nil {
				h.handleQueryError(w, r, false, err)
				return
			}
			model.DimensionList = cards
		}
		if page == "reports" {
			finals, groups, err := h.studyReportViews(r, match.params[0])
			if err != nil {
				h.handleQueryError(w, r, false, err)
				return
			}
			model.ReportFinals = finals
			model.ReportGroups = groups
			for _, group := range groups {
				model.ReportSourceCount += len(group.Reports)
			}
		}
		if page == "repos" {
			if err := h.loadRepoBoard(r, match.params[0], &model); err != nil {
				h.handleQueryError(w, r, false, err)
				return
			}
		}
		h.render(w, r, http.StatusOK, "study", model)
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
	case "api_models":
		queries, ok := h.queries.(app.WebModelQueries)
		if !ok {
			h.handleQueryError(w, r, true, app.ErrWebUnavailable)
			return
		}
		result, err := queries.Models(r.Context())
		if err != nil {
			h.handleQueryError(w, r, true, err)
			return
		}
		models := make([]map[string]any, 0, len(result.Models))
		for _, model := range result.Models {
			models = append(models, map[string]any{"provider": model.Provider, "id": model.ID, "reference": strings.TrimPrefix(model.Provider+"/"+model.ID, "/")})
		}
		h.writeSuccess(w, r, http.StatusOK, map[string]any{"models": models, "default": result.Default}, nil)
	case "api_prompt_bundle":
		queries, ok := h.queries.(app.WebPromptQueries)
		if !ok {
			h.handleQueryError(w, r, true, app.ErrWebUnavailable)
			return
		}
		result, err := queries.PromptBundle(r.Context(), match.params[0], match.params[1], match.params[2])
		if err != nil {
			h.handleQueryError(w, r, true, err)
			return
		}
		h.writeSuccess(w, r, http.StatusOK, mapPromptBundle(result), nil)
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
	case "api_study_resources":
		queries, ok := h.queries.(app.WebResourceQueries)
		if !ok {
			h.handleQueryError(w, r, true, app.ErrWebUnavailable)
			return
		}
		result, err := queries.StudyResources(r.Context(), match.params[0])
		if err != nil {
			h.handleQueryError(w, r, true, err)
			return
		}
		h.writeSuccess(w, r, http.StatusOK, result, nil)
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
		data := map[string]any{"status": result.Status, "server": result.Server, "workspace": result.Workspace}
		if h.runs != nil {
			if runHealth, runErr := h.runs.RunHealth(r.Context()); runErr == nil {
				data["run_control"] = runHealth
			} else {
				data["run_control"] = map[string]any{"status": "failed"}
				status = http.StatusServiceUnavailable
			}
		}
		h.writeSuccess(w, r, status, data, nil)
	case "api_runs":
		h.handleRuns(w, r)
	case "api_timeline":
		h.handleTimeline(w, r)
	case "api_run":
		if r.Method == http.MethodDelete {
			h.handleRunCancel(w, r, match.params[0])
		} else {
			h.handleRunShow(w, r, match.params[0])
		}
	case "api_run_events":
		h.handleRunEvents(w, r, match.params[0])
	case "api_operation_prepare":
		h.handleOperationPrepare(w, r)
	case "api_operations":
		if r.Method == http.MethodPost {
			h.handleOperationStart(w, r)
		} else {
			h.handleActiveOperations(w, r)
		}
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
	case "sprint_create":
		h.handleSprintCreate(w, r, match.params[0])
	case "operation_start":
		h.handleHTMLOperationStart(w, r)
	case "operation_cancel":
		h.handleHTMLOperationCancel(w, r, match.params[0])
	case "operation":
		h.handleHTMLOperationStatus(w, r, match.params[0])
	default:
		h.writeRouteError(w, r, match.api, http.StatusNotFound, "not_found", "The requested resource was not found.")
	}
}

func projectPageTitle(page string) string {
	switch page {
	case "roadmap", "sprints":
		return "Roadmap"
	default:
		return "Docs"
	}
}

func renderMarkdown(artifact app.WebArtifactPreview) template.HTML {
	if artifact.MediaType != "text/markdown" {
		return ""
	}
	return safeMarkdown(artifact.Content)
}

func safeMarkdown(source string) template.HTML {
	rendered, err := app.RenderSafeMarkdown(source)
	if err != nil {
		return ""
	}
	return template.HTML(rendered) // RenderSafeMarkdown returns a reviewed safe projection.
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
	case "reports":
		return "Reports"
	case "repos":
		return "Repos"
	default:
		return "Reports"
	}
}

func (h *handler) loadRepoBoard(r *http.Request, name string, model *pageModel) error {
	queries, ok := h.queries.(app.WebStudyReportQueries)
	if !ok {
		return app.ErrWebUnavailable
	}
	result, err := queries.StudyRepos(r.Context(), name)
	if err != nil {
		return err
	}
	byName := make(map[string]app.WebRepoScore, len(result.Repos))
	for _, repo := range result.Repos {
		byName[repo.Name] = repo
	}
	labels := make(map[string]string)
	columns := make([]string, 0)
	for _, repo := range result.Repos {
		for _, entry := range repo.Scores {
			if _, seen := labels[entry.Number]; !seen {
				labels[entry.Number] = entry.Dimension
				columns = append(columns, entry.Number)
			}
		}
	}
	sort.Strings(columns)
	topScores := make(map[string]int, len(columns))
	for _, number := range columns {
		best := 0
		for _, repo := range result.Repos {
			for _, score := range repo.Scores {
				if score.Number == number && score.Score > best {
					best = score.Score
				}
			}
		}
		topScores[number] = best
	}
	for _, number := range columns {
		model.RepoColumns = append(model.RepoColumns, repoMatrixColumn{Number: number, Label: labels[number]})
	}
	for index, repo := range result.Repos {
		entry := repoBoardEntry{
			Rank:     index + 1,
			Name:     repo.Name,
			Coverage: fmt.Sprintf("%d / %d dimensions rated", repo.RatedCount, repo.Applicable),
		}
		if repo.RatedCount > 0 {
			entry.Average = strconv.FormatFloat(repo.Average, 'f', 1, 64)
			entry.BarWidth = int(math.Round(repo.Average * 10))
		} else {
			entry.Average = "-"
		}
		if repo.Best != nil {
			entry.BestRef = repo.Best.Ref
			entry.BestText = repo.Best.Dimension + " · " + strconv.Itoa(repo.Best.Score)
		}
		model.RepoBoard = append(model.RepoBoard, entry)

		scoreByName := byName[repo.Name]
		applicable := make(map[string]bool, len(scoreByName.ApplicableNumbers))
		for _, number := range scoreByName.ApplicableNumbers {
			applicable[number] = true
		}
		row := repoMatrixRow{Name: repo.Name, Average: entry.Average}
		for _, column := range columns {
			cell := repoMatrixCell{Label: "n/a"}
			if applicable[column] {
				cell = repoMatrixCell{Label: "–"}
			}
			for _, score := range scoreByName.Scores {
				if score.Number == column {
					cell = repoMatrixCell{Label: strconv.Itoa(score.Score), Scored: true, Percent: score.Score * 10, Leader: topScores[column] > 0 && score.Score == topScores[column], Ref: score.Ref}
					break
				}
			}
			row.Cells = append(row.Cells, cell)
		}
		model.RepoMatrix = append(model.RepoMatrix, row)
	}
	for _, dimension := range result.Dimensions {
		champion := repoChampion{Dimension: dimension.Dimension}
		for _, leader := range dimension.Leaders {
			champion.Leaders = append(champion.Leaders, repoLeaderView{Rank: leader.Rank, Name: leader.Name, Score: leader.Score, Ref: leader.Ref})
		}
		model.RepoChampions = append(model.RepoChampions, champion)
	}
	return nil
}

func (h *handler) studyReportViews(r *http.Request, name string) ([]reportLinkView, []reportGroupView, error) {
	queries, ok := h.queries.(app.WebStudyReportQueries)
	if !ok {
		return nil, nil, app.ErrWebUnavailable
	}
	result, err := queries.StudyReports(r.Context(), name)
	if err != nil {
		return nil, nil, err
	}
	var finals []reportLinkView
	var groups []reportGroupView
	for _, dimension := range result.Dimensions {
		ref := dimension.Number
		if dimension.Slug != "" {
			ref += "-" + dimension.Slug
		}
		if dimension.Final != nil {
			finals = append(finals, reportLinkView{Label: ref, Ref: dimension.Final.Ref, DisplayPath: dimension.Final.DisplayPath, Bytes: dimension.Final.Bytes})
		}
		if len(dimension.Sources) == 0 {
			continue
		}
		group := reportGroupView{Dimension: ref, Reports: make([]reportLinkView, 0, len(dimension.Sources))}
		for _, source := range dimension.Sources {
			group.Reports = append(group.Reports, reportLinkView{Label: source.Source, Ref: source.Ref, DisplayPath: source.DisplayPath, Bytes: source.Bytes})
		}
		groups = append(groups, group)
	}
	return nonNilSlice(finals), nonNilSlice(groups), nil
}

func (h *handler) studyDimensionCards(r *http.Request, name string) ([]dimensionCard, error) {
	queries, ok := h.queries.(app.WebDimensionQueries)
	if !ok {
		return nil, app.ErrWebUnavailable
	}
	result, err := queries.StudyDimensions(r.Context(), name)
	if err != nil {
		return nil, err
	}
	cards := make([]dimensionCard, 0, len(result.Items))
	for _, item := range result.Items {
		cards = append(cards, dimensionCard{
			Study: item.Study, Number: item.Number, Slug: item.Slug,
			Title: item.Title, DisplayPath: item.DisplayPath,
			Bytes: item.Bytes, Truncated: item.Truncated,
			CodeCitationsDisabled: item.CodeCitationsDisabled,
			Body:                  safeMarkdown(item.Content),
		})
	}
	return cards, nil
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
	if err := h.templates.ExecuteTemplate(&buf, "page/"+name, page); err != nil {
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
	out := projectDTO{Ref: item.Ref, Name: item.Name, Docs: nonNilSlice(item.Docs), Findings: mapFindings(item.Findings), Artifacts: mapArtifacts(item.Artifacts), Sprints: []sprintDTO{}, Roadmap: []roadmapPhaseDTO{}}
	if includeSprints {
		out.Sprints = mapSprints(item.Sprints)
	}
	for _, phase := range item.Roadmap {
		sprints := make([]roadmapSprintDTO, 0, len(phase.Sprints))
		for _, entry := range phase.Sprints {
			sprints = append(sprints, roadmapSprintDTO{
				Number: entry.Number, Title: entry.Title, Slug: entry.Slug, Status: entry.Status,
				Goal: entry.Goal, GateItems: nonNilSlice(entry.GateItems), DependsOn: nonNilSlice(entry.DependsOn),
				Exists: entry.Exists, Assessment: entry.Assessment,
				CompletedStages: entry.CompletedStages, TotalStages: entry.TotalStages, CurrentStage: entry.CurrentStage,
			})
		}
		out.Roadmap = append(out.Roadmap, roadmapPhaseDTO{Title: phase.Title, Sprints: sprints})
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
		stages = append(stages, stageDTO{Name: stage.Name, Status: stage.Status, Error: stage.Error, ArtifactAvailable: stage.ArtifactAvailable, ArtifactValid: stage.ArtifactValid, LatestOutcome: stage.LatestOutcome, NextAction: stage.NextAction})
	}
	reviewers := make([]reviewerDTO, 0, len(item.Review.Reviewers))
	for _, reviewer := range item.Review.Reviewers {
		reviewers = append(reviewers, reviewerDTO{ID: reviewer.ID, Name: reviewer.Name, Kind: reviewer.Kind, Path: reviewer.Path, Status: reviewer.Status, Summary: reviewer.Summary})
	}
	return sprintDTO{
		Ref: item.Ref, Project: item.Project, Slug: item.Slug, Status: item.Status,
		Assessment: item.Assessment, NextAction: item.NextAction, Stages: stages,
		Execute:  executeDTO{Available: item.Execute.Available, Total: item.Execute.Total, Pending: item.Execute.Pending, Running: item.Execute.Running, Complete: item.Execute.Complete, Failed: item.Execute.Failed, Cancelled: item.Execute.Cancelled},
		Review:   reviewDTO{Available: item.Review.Available, Status: item.Review.Status, Verdict: item.Review.Verdict, Stale: item.Review.Stale, Completed: item.Review.Completed, Total: item.Review.Total, Pending: item.Review.Pending, Running: item.Review.Running, Failed: item.Review.Failed, Reviewers: reviewers, StartedAt: item.Review.StartedAt},
		Smoke:    smokeDTO{Available: item.Smoke.Available, Status: item.Smoke.Status, Verdict: item.Smoke.Verdict, Stale: item.Smoke.Stale, RunID: item.Smoke.RunID},
		Findings: mapFindings(item.Findings), Artifacts: mapArtifacts(item.Artifacts),
	}
}

func mapPromptBundle(item app.WebPromptBundleResult) promptBundleDTO {
	contract := promptInputContractDTO{
		Stage: string(item.InputContract.Stage), Required: item.InputContract.Required,
		Optional: item.InputContract.Optional, Forbidden: item.InputContract.Forbidden,
	}
	out := promptBundleDTO{
		Stage: string(item.Stage), Available: item.Available, Scope: item.Scope,
		UnavailableReason: item.UnavailableReason, InputContract: contract, Blocks: []promptBlockDTO{},
	}
	if item.Explanation == nil {
		return out
	}
	explanation := item.Explanation
	out.SchemaVersion = explanation.SchemaVersion
	out.TotalBytes = explanation.TotalBytes
	out.SharedPrefixBytes = explanation.SharedPrefixBytes
	out.StageSuffixBytes = explanation.StageSuffixBytes
	out.SharedPrefixDigest = explanation.SharedPrefixDigest
	out.CacheKey = explanation.CacheKey
	out.CacheBreakpointBytes = explanation.CacheBreakpoint
	out.CacheCandidate = explanation.CacheCandidate
	out.CacheTransport = explanation.CacheTransport
	for index, block := range explanation.Blocks {
		content := ""
		if index < len(item.BlockContents) {
			content = item.BlockContents[index]
		}
		out.Blocks = append(out.Blocks, promptBlockDTO{ID: block.ID, Kind: block.Kind, Mode: block.Mode, Cacheable: block.Cacheable, Bytes: block.Bytes, Digest: block.Digest, Content: content})
	}
	return out
}

func mapSprints(items []app.WebSprintResult) []sprintDTO {
	out := make([]sprintDTO, 0, len(items))
	for _, item := range items {
		out = append(out, mapSprint(item))
	}
	return out
}

func mapStudyParallelism(item *app.ParallelismSummary) *studyParallelismDTO {
	if item == nil {
		return nil
	}
	return &studyParallelismDTO{Decreased: item.Decreased, Events: item.Events, RequestedParallelism: item.RequestedParallelism, EffectiveParallelism: item.EffectiveParallelism}
}

func mapStudy(item app.WebStudyResult) studyDTO {
	retries := studyRetryDTO{RetriedTasks: item.Retries.RetriedTasks, TotalRetries: item.Retries.TotalRetries, SameSession: item.Retries.SameSession, FreshSession: item.Retries.FreshSession, Waiting: item.Retries.Waiting}
	if item.Retries.NextRetryAt != nil {
		next := *item.Retries.NextRetryAt
		retries.NextRetryAt = &next
	}
	retried := make([]studyTaskRetryDTO, 0, len(item.RetriedTasks))
	for _, task := range item.RetriedTasks {
		retried = append(retried, studyTaskRetryDTO{ID: task.ID, Kind: task.Kind, Status: task.Status, Retries: task.Retries, SessionReuse: task.SessionReuse})
	}
	var failures []studyTaskFailureDTO
	for _, task := range item.Tasks {
		if task.Error != "" {
			failures = append(failures, studyTaskFailureDTO{Task: task.ID, Code: task.ErrorCode, Message: task.Error})
		}
	}
	return studyDTO{Ref: item.Ref, Name: item.Name, Sources: nonNilSlice(item.Sources), Dimensions: nonNilSlice(item.Dimensions), Status: item.Status, RunID: item.RunID, Total: item.Total, Completed: item.Completed, Failed: item.Failed, RunActive: item.RunActive, ActiveTasks: item.ActiveTasks, Pending: item.Pending, Cancelled: item.Cancelled, Retries: retries, Parallelism: mapStudyParallelism(item.Parallelism), RetriedTasks: retried, Failures: failures, Findings: mapFindings(item.Findings), Artifacts: mapArtifacts(item.Artifacts)}
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

func newestSprintsFirst(items []app.WebSprintResult) []app.WebSprintResult {
	out := append([]app.WebSprintResult(nil), items...)
	for left, right := 0, len(out)-1; left < right; left, right = left+1, right-1 {
		out[left], out[right] = out[right], out[left]
	}
	return out
}

// handleSprintCreate materializes a roadmap sprint workspace directory and
// returns the browser straight to the sprint page.
func (h *handler) handleSprintCreate(w http.ResponseWriter, r *http.Request, projectName string) {
	if err := r.ParseForm(); err != nil || r.FormValue("_csrf") != csrfToken(r.Context()) {
		h.renderError(w, r, http.StatusForbidden, "Request rejected", "The browser session or CSRF proof is invalid. Refresh the page and try again.")
		return
	}
	mutation, ok := h.queries.(app.WebSprintWorkspaceMutation)
	if !ok {
		h.renderError(w, r, http.StatusServiceUnavailable, "Unavailable", "Sprint workspace creation is unavailable on this surface.")
		return
	}
	slug := r.FormValue("sprint")
	if !validOptionalIdentifier(projectName) || projectName == "" || !validIdentifier(slug) {
		h.renderError(w, r, http.StatusBadRequest, "Invalid sprint", "The project or sprint reference is not a single safe path segment.")
		return
	}
	if err := mutation.CreateSprintWorkspace(r.Context(), projectName, slug); err != nil {
		h.renderError(w, r, http.StatusUnprocessableEntity, "Creation failed", htmlOperationFailure("The sprint workspace could not be created from the roadmap slug.", err))
		return
	}
	http.Redirect(w, r, "/projects/"+projectName+"/sprints/"+slug, http.StatusSeeOther)
}

func intPtr(value int) *int    { return &value }
func boolPtr(value bool) *bool { return &value }
