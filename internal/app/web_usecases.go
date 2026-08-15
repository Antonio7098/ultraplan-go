package app

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Antonio7098/ultraplan-go/internal/sprint"
	"github.com/Antonio7098/ultraplan-go/internal/workspace"
)

const (
	WebCollectionLimit  = 200
	WebPreviewByteLimit = 256 * 1024
)

var (
	ErrWebNotFound    = errors.New("web resource not found")
	ErrWebUnavailable = errors.New("web query unavailable")
)

type WebQueries interface {
	Dashboard(context.Context) (WebDashboardResult, error)
	Projects(context.Context) (WebProjectsResult, error)
	Project(context.Context, string) (WebProjectResult, error)
	Sprint(context.Context, string, string) (WebSprintResult, error)
	Studies(context.Context) (WebStudiesResult, error)
	Study(context.Context, string) (WebStudyResult, error)
	Validations(context.Context, string, string) (WebValidationResult, error)
	Artifact(context.Context, string) (WebArtifactPreview, error)
	Health(context.Context) (WebHealthResult, error)
}

type WebUseCaseOptions struct {
	StageRuntime      map[sprint.PlanningStage]sprint.StageRuntime
	ReviewConcurrency int
	SmokeSettings     sprint.SmokeSettings
}

type CollectionInfo struct {
	ReturnedCount int
	TotalCount    int
	Truncated     bool
}

type WebArtifactLink struct {
	Ref         string
	Label       string
	DisplayPath string
	MediaType   string
}

type WebProjectResult struct {
	Ref          string
	Name         string
	Docs         []string
	Findings     []DisplayFinding
	Artifacts    []WebArtifactLink
	Sprints      []WebSprintResult
	SprintCounts CollectionInfo
}

type WebSprintResult struct {
	Ref        string
	Project    string
	Slug       string
	Status     string
	Assessment string
	NextAction string
	Stages     []StageSummary
	Execute    ExecuteSummary
	Review     ReviewSummary
	Smoke      SmokeSummary
	Findings   []DisplayFinding
	Artifacts  []WebArtifactLink
}

type WebStudyResult struct {
	Ref         string
	Name        string
	Sources     []string
	Dimensions  []string
	Status      string
	RunID       string
	Total       int
	Completed   int
	Failed      int
	RunActive   bool
	ActiveTasks int
	Pending     int
	Cancelled   int
	Findings    []DisplayFinding
	Artifacts   []WebArtifactLink
}

type WebProjectsResult struct {
	Items []WebProjectResult
	CollectionInfo
}

type WebStudiesResult struct {
	Items []WebStudyResult
	CollectionInfo
}

type WebDashboardResult struct {
	Ref          string
	Workspace    string
	Projects     WebProjectsResult
	Sprints      []WebSprintResult
	Studies      WebStudiesResult
	SprintCounts CollectionInfo
}

type WebValidationResult struct {
	Scope    string
	Ref      string
	Findings []DisplayFinding
	CollectionInfo
}

type WebArtifactPreview struct {
	Ref           string
	DisplayPath   string
	MediaType     string
	Content       string
	SizeBytes     int64
	ReturnedBytes int
	Truncated     bool
	JSONValid     bool
}

type WebHealthResult struct {
	Status    string
	Server    bool
	Workspace bool
}

type webRefTarget struct {
	kind   string
	values []string
}

type webUseCases struct {
	root      string
	dashboard dashboardUseCases
	secret    [32]byte
	mu        sync.RWMutex
	refs      map[string]webRefTarget
}

func NewWebUseCases(root string, opts WebUseCaseOptions) WebQueries {
	u := &webUseCases{
		root: root,
		dashboard: dashboardUseCases{
			root:              root,
			stageRuntime:      opts.StageRuntime,
			reviewConcurrency: opts.ReviewConcurrency,
			smokeSettings:     opts.SmokeSettings,
			readOnly:          true,
		},
		refs: make(map[string]webRefTarget),
	}
	if _, err := rand.Read(u.secret[:]); err != nil {
		u.secret = sha256.Sum256([]byte(filepath.Clean(root)))
	}
	return u
}

func (u *webUseCases) Dashboard(ctx context.Context) (WebDashboardResult, error) {
	if err := ctx.Err(); err != nil {
		return WebDashboardResult{}, err
	}
	projects, err := u.Projects(ctx)
	if err != nil {
		return WebDashboardResult{}, err
	}
	studies, err := u.Studies(ctx)
	if err != nil {
		return WebDashboardResult{}, err
	}
	summaries, err := u.dashboard.SprintSummaries(ctx)
	if err != nil {
		return WebDashboardResult{}, err
	}
	total := len(summaries)
	summaries = bounded(summaries)
	sprints := make([]WebSprintResult, 0, len(summaries))
	for _, item := range summaries {
		sprints = append(sprints, u.webSprint(item))
	}
	return WebDashboardResult{
		Ref:          u.issue("workspace"),
		Workspace:    filepath.Base(filepath.Clean(u.root)),
		Projects:     projects,
		Sprints:      sprints,
		Studies:      studies,
		SprintCounts: collectionInfo(len(sprints), total),
	}, nil
}

func (u *webUseCases) Projects(ctx context.Context) (WebProjectsResult, error) {
	items, err := u.dashboard.ProjectSummaries(ctx)
	if err != nil {
		return WebProjectsResult{}, err
	}
	total := len(items)
	items = bounded(items)
	out := make([]WebProjectResult, 0, len(items))
	for _, item := range items {
		out = append(out, u.webProject(item, nil))
	}
	return WebProjectsResult{Items: out, CollectionInfo: collectionInfo(len(out), total)}, nil
}

func (u *webUseCases) Project(ctx context.Context, name string) (WebProjectResult, error) {
	items, err := u.dashboard.ProjectSummaries(ctx)
	if err != nil {
		return WebProjectResult{}, err
	}
	var selected *ProjectSummary
	for i := range items {
		if items[i].Name == name {
			selected = &items[i]
			break
		}
	}
	if selected == nil {
		return WebProjectResult{}, ErrWebNotFound
	}
	allSprints, err := u.dashboard.SprintSummaries(ctx)
	if err != nil {
		return WebProjectResult{}, err
	}
	var projectSprints []SprintSummary
	for _, item := range allSprints {
		if item.Project == name {
			projectSprints = append(projectSprints, item)
		}
	}
	total := len(projectSprints)
	projectSprints = bounded(projectSprints)
	out := make([]WebSprintResult, 0, len(projectSprints))
	for _, item := range projectSprints {
		out = append(out, u.webSprint(item))
	}
	result := u.webProject(*selected, out)
	result.SprintCounts = collectionInfo(len(out), total)
	return result, nil
}

func (u *webUseCases) Sprint(ctx context.Context, project, slug string) (WebSprintResult, error) {
	items, err := u.dashboard.SprintSummaries(ctx)
	if err != nil {
		return WebSprintResult{}, err
	}
	for _, item := range items {
		if item.Project == project && item.Slug == slug {
			return u.webSprint(item), nil
		}
	}
	return WebSprintResult{}, ErrWebNotFound
}

func (u *webUseCases) Studies(ctx context.Context) (WebStudiesResult, error) {
	items, err := u.dashboard.StudySummaries(ctx)
	if err != nil {
		return WebStudiesResult{}, err
	}
	total := len(items)
	items = bounded(items)
	out := make([]WebStudyResult, 0, len(items))
	for _, item := range items {
		out = append(out, u.webStudy(item))
	}
	return WebStudiesResult{Items: out, CollectionInfo: collectionInfo(len(out), total)}, nil
}

func (u *webUseCases) Study(ctx context.Context, name string) (WebStudyResult, error) {
	items, err := u.dashboard.StudySummaries(ctx)
	if err != nil {
		return WebStudyResult{}, err
	}
	for _, item := range items {
		if item.Name == name {
			return u.webStudy(item), nil
		}
	}
	return WebStudyResult{}, ErrWebNotFound
}

func (u *webUseCases) Validations(ctx context.Context, scope, ref string) (WebValidationResult, error) {
	target, ok := u.resolve(ref, scope)
	if !ok {
		return WebValidationResult{}, ErrWebNotFound
	}
	var findings []DisplayFinding
	switch scope {
	case "workspace":
		if err := ctx.Err(); err != nil {
			return WebValidationResult{}, err
		}
		result := workspace.Validate(u.root)
		for _, issue := range result.Issues {
			findings = append(findings, DisplayFinding{Severity: "error", Section: "workspace", Problem: displaySafe(issue)})
		}
	case "project":
		result, err := u.Project(ctx, target.values[0])
		if err != nil {
			return WebValidationResult{}, err
		}
		findings = append(findings, result.Findings...)
	case "sprint":
		result, err := u.Sprint(ctx, target.values[0], target.values[1])
		if err != nil {
			return WebValidationResult{}, err
		}
		findings = append(findings, result.Findings...)
	case "study":
		result, err := u.Study(ctx, target.values[0])
		if err != nil {
			return WebValidationResult{}, err
		}
		findings = append(findings, result.Findings...)
	default:
		return WebValidationResult{}, ErrWebNotFound
	}
	total := len(findings)
	findings = bounded(findings)
	if findings == nil {
		findings = []DisplayFinding{}
	}
	return WebValidationResult{Scope: scope, Ref: ref, Findings: findings, CollectionInfo: collectionInfo(len(findings), total)}, nil
}

func (u *webUseCases) Artifact(ctx context.Context, ref string) (WebArtifactPreview, error) {
	target, ok := u.resolve(ref, "artifact")
	if !ok || len(target.values) != 1 {
		return WebArtifactPreview{}, ErrWebNotFound
	}
	rel := target.values[0]
	if err := ctx.Err(); err != nil {
		return WebArtifactPreview{}, err
	}
	if !supportedPreviewPath(rel) {
		return WebArtifactPreview{}, ErrWebNotFound
	}
	path, err := u.containedArtifactPath(rel)
	if err != nil {
		return WebArtifactPreview{}, ErrWebNotFound
	}
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return WebArtifactPreview{}, ErrWebNotFound
		}
		return WebArtifactPreview{}, fmt.Errorf("%w: artifact read", ErrWebUnavailable)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return WebArtifactPreview{}, ErrWebNotFound
	}
	data, err := io.ReadAll(io.LimitReader(file, WebPreviewByteLimit+1))
	if err != nil {
		return WebArtifactPreview{}, fmt.Errorf("%w: artifact read", ErrWebUnavailable)
	}
	truncated := len(data) > WebPreviewByteLimit
	if truncated {
		data = data[:WebPreviewByteLimit]
	}
	media := mediaTypeForPath(rel)
	return WebArtifactPreview{
		Ref:           ref,
		DisplayPath:   filepath.ToSlash(rel),
		MediaType:     media,
		Content:       string(data),
		SizeBytes:     info.Size(),
		ReturnedBytes: len(data),
		Truncated:     truncated,
		JSONValid:     media != "application/json" || json.Valid(data),
	}, nil
}

func (u *webUseCases) Health(ctx context.Context) (WebHealthResult, error) {
	if err := ctx.Err(); err != nil {
		return WebHealthResult{}, err
	}
	info, err := os.Stat(filepath.Join(u.root, workspace.MarkerFile))
	available := err == nil && !info.IsDir()
	status := "ok"
	if !available {
		status = "unavailable"
	}
	return WebHealthResult{Status: status, Server: true, Workspace: available}, nil
}

func (u *webUseCases) webProject(item ProjectSummary, sprints []WebSprintResult) WebProjectResult {
	docs := append([]string(nil), item.MarkdownDocs...)
	if docs == nil {
		docs = []string{}
	}
	findings := bounded(append([]DisplayFinding(nil), item.Findings...))
	if findings == nil {
		findings = []DisplayFinding{}
	}
	return WebProjectResult{
		Ref:       u.issue("project", item.Name),
		Name:      item.Name,
		Docs:      docs,
		Findings:  findings,
		Artifacts: u.webArtifacts(item.Artifacts),
		Sprints:   nonNil(sprints),
	}
}

func (u *webUseCases) webSprint(item SprintSummary) WebSprintResult {
	stages := append([]StageSummary(nil), item.Stages...)
	if stages == nil {
		stages = []StageSummary{}
	}
	findings := bounded(append([]DisplayFinding(nil), item.Findings...))
	if findings == nil {
		findings = []DisplayFinding{}
	}
	return WebSprintResult{
		Ref:        u.issue("sprint", item.Project, item.Slug),
		Project:    item.Project,
		Slug:       item.Slug,
		Status:     item.Status,
		Assessment: item.Assessment,
		NextAction: displaySafe(item.NextAction),
		Stages:     stages,
		Execute:    item.Execute,
		Review:     item.Review,
		Smoke:      item.Smoke,
		Findings:   findings,
		Artifacts:  u.webArtifacts(item.Artifacts),
	}
}

func (u *webUseCases) webStudy(item StudySummary) WebStudyResult {
	findings := bounded(append([]DisplayFinding(nil), item.Findings...))
	if findings == nil {
		findings = []DisplayFinding{}
	}
	return WebStudyResult{
		Ref:         u.issue("study", item.Name),
		Name:        item.Name,
		Sources:     nonNil(append([]string(nil), item.Sources...)),
		Dimensions:  nonNil(append([]string(nil), item.Dimensions...)),
		Status:      item.Status,
		RunID:       displaySafe(item.RunID),
		Total:       item.Total,
		Completed:   item.Completed,
		Failed:      item.Failed,
		RunActive:   item.RunActive,
		ActiveTasks: item.ActiveTasks,
		Pending:     item.Pending,
		Cancelled:   item.Cancelled,
		Findings:    findings,
		Artifacts:   u.webArtifacts(item.Artifacts),
	}
}

func (u *webUseCases) webArtifacts(items []DisplayArtifact) []WebArtifactLink {
	out := make([]WebArtifactLink, 0, len(items))
	for _, item := range items {
		rel := filepath.ToSlash(filepath.Clean(filepath.FromSlash(item.Path)))
		if filepath.IsAbs(rel) || !supportedPreviewPath(rel) {
			continue
		}
		out = append(out, WebArtifactLink{
			Ref:         u.issue("artifact", rel),
			Label:       item.Label,
			DisplayPath: rel,
			MediaType:   mediaTypeForPath(rel),
		})
		if len(out) == WebCollectionLimit {
			break
		}
	}
	if out == nil {
		return []WebArtifactLink{}
	}
	return out
}

func (u *webUseCases) issue(kind string, values ...string) string {
	mac := hmac.New(sha256.New, u.secret[:])
	_, _ = mac.Write([]byte(kind))
	for _, value := range values {
		_, _ = mac.Write([]byte{0})
		_, _ = mac.Write([]byte(value))
	}
	ref := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	target := webRefTarget{kind: kind, values: append([]string(nil), values...)}
	u.mu.Lock()
	u.refs[ref] = target
	u.mu.Unlock()
	return ref
}

func (u *webUseCases) resolve(ref, kind string) (webRefTarget, bool) {
	u.mu.RLock()
	target, ok := u.refs[ref]
	u.mu.RUnlock()
	return target, ok && target.kind == kind
}

func (u *webUseCases) containedArtifactPath(rel string) (string, error) {
	path, err := workspace.ResolveInside(u.root, rel)
	if err != nil {
		return "", err
	}
	root, err := filepath.EvalSymlinks(u.root)
	if err != nil {
		return "", err
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	inside, err := filepath.Rel(root, path)
	if err != nil || inside == ".." || strings.HasPrefix(inside, ".."+string(filepath.Separator)) {
		return "", errors.New("artifact escapes workspace")
	}
	return path, nil
}

func mediaTypeForPath(path string) string {
	if strings.EqualFold(filepath.Ext(path), ".json") {
		return "application/json"
	}
	return "text/markdown"
}

func bounded[T any](items []T) []T {
	if len(items) > WebCollectionLimit {
		items = items[:WebCollectionLimit]
	}
	return items
}

func nonNil[T any](items []T) []T {
	if items == nil {
		return []T{}
	}
	return items
}

func collectionInfo(returned, total int) CollectionInfo {
	return CollectionInfo{ReturnedCount: returned, TotalCount: total, Truncated: returned < total}
}
