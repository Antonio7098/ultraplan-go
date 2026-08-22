package sprint

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	pruntime "github.com/Antonio7098/ultraplan-go/internal/platform/runtime"
	"github.com/Antonio7098/ultraplan-go/internal/project"
	"github.com/Antonio7098/ultraplan-go/internal/workspace"
)

const StageReview PlanningStage = "review"

type ReviewExecutionStatus string
type ReviewVerdict string

const (
	ReviewReady     ReviewExecutionStatus = "ready"
	ReviewRunning   ReviewExecutionStatus = "running"
	ReviewCompleted ReviewExecutionStatus = "completed"
	ReviewFailed    ReviewExecutionStatus = "failed"
	ReviewCancelled ReviewExecutionStatus = "cancelled"
	ReviewBlocked   ReviewExecutionStatus = "blocked"

	ReviewPass             ReviewVerdict = "pass"
	ReviewPassWithFindings ReviewVerdict = "pass_with_findings"
	ReviewFail             ReviewVerdict = "fail"
	ReviewVerdictBlocked   ReviewVerdict = "blocked"
)

type ReviewDiagnostic struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	CoverageID string `json:"coverage_id,omitempty"`
}

type ReviewStageState struct {
	Status             ReviewExecutionStatus `json:"status"`
	Verdict            ReviewVerdict         `json:"verdict,omitempty"`
	ProvisionalVerdict ReviewVerdict         `json:"provisionalVerdict,omitempty"`
	Path               string                `json:"path"`
	LastRunAt          *time.Time            `json:"lastRunAt,omitempty"`
	Fingerprint        string                `json:"fingerprint,omitempty"`
	Stale              bool                  `json:"stale"`
	Completed          int                   `json:"completed"`
	Total              int                   `json:"total"`
	Diagnostics        []ReviewDiagnostic    `json:"diagnostics,omitempty"`
	ArtifactDigest     string                `json:"artifactDigest,omitempty"`
	ActiveAttempt      *VerificationAttempt  `json:"activeAttempt,omitempty"`
	LastAttempt        *VerificationAttempt  `json:"lastAttempt,omitempty"`
	Resume             *ReviewResumeState    `json:"resume,omitempty"`
	LastComplete       *ReviewCompletion     `json:"lastComplete,omitempty"`
}

type ReviewResumeState struct {
	AttemptID        string                     `json:"attemptId"`
	InputFingerprint string                     `json:"inputFingerprint"`
	Model            string                     `json:"model"`
	UpdatedAt        time.Time                  `json:"updatedAt"`
	Coverage         []ReviewCoverageCheckpoint `json:"coverage"`
}

type ReviewCoverageCheckpoint struct {
	CoverageID string                `json:"coverageId"`
	Status     AttemptStatus         `json:"status"`
	SessionID  string                `json:"sessionId,omitempty"`
	UpdatedAt  time.Time             `json:"updatedAt"`
	Result     *ReviewCoverageResult `json:"result,omitempty"`
}

type ReviewCompletion struct {
	Verdict          ReviewVerdict          `json:"verdict"`
	Artifact         string                 `json:"artifact"`
	ArtifactDigest   string                 `json:"artifactDigest"`
	InputFingerprint string                 `json:"inputFingerprint"`
	CompletedAt      time.Time              `json:"completedAt"`
	Coverage         []ReviewCoverageResult `json:"coverage,omitempty"`
}

type ReviewInput struct {
	ID, Kind, Name, Path, Hash string
}

type ReviewManifest struct {
	Project, Sprint, SprintRoot, Target, Fingerprint string
	WorkspaceRoot                                    string
	ReviewerRoot                                     string
	SharedPrefix                                     string
	Model, ModelSource, Variant                      string
	Concurrency                                      int
	Inputs, Coverage                                 []ReviewInput
	ChangedPaths                                     []string
	Contents                                         map[string]string
	PromptTemplate, OutputTemplate                   string
}

type ReviewCitation struct {
	Path      string `json:"path"`
	StartLine int    `json:"startLine"`
	EndLine   int    `json:"endLine"`
}

type ReviewFinding struct {
	ID            string           `json:"id"`
	Severity      string           `json:"severity"`
	Applicability string           `json:"applicability"`
	Title         string           `json:"title"`
	Detail        string           `json:"detail"`
	Action        string           `json:"action,omitempty"`
	Citations     []ReviewCitation `json:"citations"`
}

type ReviewCoverageResult struct {
	SchemaVersion int             `json:"schemaVersion"`
	CoverageID    string          `json:"coverageId"`
	Applicability string          `json:"applicability"`
	Summary       string          `json:"summary"`
	Findings      []ReviewFinding `json:"findings"`
	Error         string          `json:"-"`
}

type ReviewRequest struct {
	DryRun, PromptOnly bool
	Restart            bool
	ModelOverride      string
	Concurrency        int
	Focus              []string
	Progress           func(ReviewProgress)
}

type ReviewProgress struct {
	Completed, Total    int
	CoverageID, Message string
}

type ReviewResult struct {
	Project            string                 `json:"project"`
	Sprint             string                 `json:"sprint"`
	Prompt             string                 `json:"prompt,omitempty"`
	Artifact           string                 `json:"artifact,omitempty"`
	Fingerprint        string                 `json:"fingerprint,omitempty"`
	Message            string                 `json:"message,omitempty"`
	DryRun             bool                   `json:"dry_run"`
	Status             ReviewExecutionStatus  `json:"execution_status"`
	Verdict            ReviewVerdict          `json:"verdict,omitempty"`
	ProvisionalVerdict ReviewVerdict          `json:"provisional_verdict,omitempty"`
	Coverage           []ReviewCoverageResult `json:"coverage,omitempty"`
	Findings           []ReviewFinding        `json:"findings,omitempty"`
	Diagnostics        []ReviewDiagnostic     `json:"diagnostics,omitempty"`
	Focused            []string               `json:"focused,omitempty"`
	Resumed            bool                   `json:"resumed,omitempty"`
	Restarted          bool                   `json:"restarted,omitempty"`
	Reused             int                    `json:"reused_coverage,omitempty"`
}

func (s Service) PrepareReview(projectRef, sprintRef string, req ReviewRequest) (ReviewManifest, []ValidationFinding, error) {
	sp, inputs, catalog, err := s.resolveSprintInputs(projectRef, sprintRef)
	if err != nil {
		return ReviewManifest{}, nil, err
	}
	manifest := ReviewManifest{Project: sp.Project, Sprint: sp.Slug, SprintRoot: workspace.Rel(s.root, sp.Path), Target: "", WorkspaceRoot: s.root, Contents: map[string]string{}}
	manifest.Concurrency = req.Concurrency
	if manifest.Concurrency <= 0 {
		manifest.Concurrency = s.reviewConcurrency
	}
	if manifest.Concurrency <= 0 {
		manifest.Concurrency = 3
	}
	if manifest.Concurrency > 16 {
		manifest.Concurrency = 16
	}
	selection := s.reviewModelSelection(req.ModelOverride)
	manifest.Model, manifest.ModelSource = selection.Model, selection.Source
	if rt, ok := s.stageRuntime[StageReview]; ok {
		manifest.Variant = rt.Variant
	} else if rt, ok := s.stageRuntime[StagePlan]; ok {
		manifest.Variant = rt.Variant
	}
	var findings []ValidationFinding
	var assetErr error
	manifest.PromptTemplate, assetErr = loadReviewAsset(s.root, "prompts/review.md", []string{"Automated Sprint Review"})
	if assetErr != nil {
		findings = append(findings, finding("Review assets", "prompts/review.md", "prompts/review.md", "invalid review prompt override", assetErr.Error(), "Remove or correct the intentional override."))
	}
	manifest.OutputTemplate, assetErr = loadReviewAsset(s.root, "templates/review.md", []string{"Review Context", "Final Assessment"})
	if assetErr != nil {
		findings = append(findings, finding("Review assets", "templates/review.md", "templates/review.md", "invalid review template override", assetErr.Error(), "Remove or correct the intentional override."))
	}
	if manifest.PromptTemplate != "" {
		manifest.Inputs = append(manifest.Inputs, reviewInput("review-prompt", "asset", "review prompt", "prompts/review.md", manifest.PromptTemplate))
		manifest.Contents["prompts/review.md"] = manifest.PromptTemplate
	}
	if manifest.OutputTemplate != "" {
		manifest.Inputs = append(manifest.Inputs, reviewInput("review-template", "asset", "review template", "templates/review.md", manifest.OutputTemplate))
		manifest.Contents["templates/review.md"] = manifest.OutputTemplate
	}
	idx, indexFindings := ValidateSprintIndexContent(inputs.SprintIndex, catalog)
	findings = append(findings, indexFindings...)
	target, targetFindings := s.resolveSprintTarget(sp, inputs.ProjectIndex, false)
	findings = append(findings, targetFindings...)
	manifest.Target = target.Path
	base := []struct {
		id, kind string
		stage    PlanningStage
	}{
		{"requirements", "governed", StageRequirements}, {"code-context", "governed", StageCodeContext}, {"sprint-index", "governed", StageSprintIndex},
		{"technical-handbook", "handbook", StageTechnicalHandbook}, {"reasoning", "governed", StageReasoning},
		{"plan", "governed", StagePlan}, {"execute", "governed", StageExecute},
	}
	projectRoot := filepath.Join(s.root, "projects", sp.Project)
	projectInputs := []struct{ id, path string }{{"project-index", filepath.Join(projectRoot, "project-index.md")}, {"roadmap", filepath.Join(projectRoot, "roadmap.md")}}
	for _, doc := range inputs.Docs {
		projectInputs = append(projectInputs, struct{ id, path string }{"doc-" + slugReviewID(doc), filepath.Join(projectRoot, filepath.FromSlash(doc))})
	}
	for _, item := range projectInputs {
		rel := workspace.Rel(s.root, item.path)
		data, readErr := os.ReadFile(item.path)
		if readErr != nil {
			manifest.Inputs = append(manifest.Inputs, ReviewInput{ID: item.id, Kind: "governed", Name: item.id, Path: rel, Hash: "missing"})
			findings = append(findings, finding("Review prerequisites", item.id, rel, "missing governed project input", safeError(readErr), "Restore the governed project input."))
			continue
		}
		content := string(data)
		if item.id == "project-index" {
			content = reviewRelevantProjectIndexContent(content)
		}
		manifest.Contents[rel] = content
		manifest.Inputs = append(manifest.Inputs, reviewInput(item.id, "governed", item.id, rel, content))
	}
	for _, item := range base {
		data, readErr := s.store.ReadArtifact(sp, item.stage)
		path := ArtifactRelPath(sp, item.stage)
		if readErr != nil || strings.TrimSpace(data) == "" {
			manifest.Inputs = append(manifest.Inputs, ReviewInput{ID: item.id, Kind: item.kind, Name: item.id, Path: path, Hash: "missing"})
			findings = append(findings, finding("Review prerequisites", item.id, path, "missing review input", safeError(readErr), "Complete execute and all governed sprint artifacts before review."))
			continue
		}
		manifest.Contents[path] = data
		manifest.Inputs = append(manifest.Inputs, reviewInput(item.id, item.kind, item.id, path, data))
	}
	areaDir, _ := ArtifactPath(s.root, sp, StageAreaReasoning)
	if entries, readErr := os.ReadDir(areaDir); readErr == nil {
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".md") {
				continue
			}
			full := filepath.Join(areaDir, entry.Name())
			data, fileErr := os.ReadFile(full)
			rel := workspace.Rel(s.root, full)
			if fileErr != nil {
				manifest.Inputs = append(manifest.Inputs, ReviewInput{ID: "area-" + slugReviewID(entry.Name()), Kind: "governed", Name: entry.Name(), Path: rel, Hash: "missing"})
				findings = append(findings, finding("Review prerequisites", entry.Name(), rel, "missing area reasoning input", safeError(fileErr), "Restore selected area reasoning."))
				continue
			}
			manifest.Contents[rel] = string(data)
			manifest.Inputs = append(manifest.Inputs, reviewInput("area-"+slugReviewID(entry.Name()), "governed", entry.Name(), rel, string(data)))
		}
	}
	planManifest, planFindings := s.planManifest(sp, inputs, catalog)
	findings = append(findings, planFindings...)
	if len(planFindings) == 0 {
		findings = append(findings, ValidatePlanContent(manifest.Contents[ArtifactRelPath(sp, StagePlan)], planManifest)...)
	}
	runPath := ExecuteRunStateRelPath(sp)
	if data, readErr := os.ReadFile(filepath.Join(s.root, filepath.FromSlash(runPath))); readErr == nil {
		manifest.Contents[runPath] = string(data)
		manifest.Inputs = append(manifest.Inputs, reviewInput("run-state", "governed", "run-state", runPath, string(data)))
		manifest.ChangedPaths = excludeGovernedReviewPaths(reviewChangedPaths(data), manifest.Inputs)
	} else {
		manifest.Inputs = append(manifest.Inputs, ReviewInput{ID: "run-state", Kind: "governed", Name: "run-state", Path: runPath, Hash: "missing"})
	}
	if target.Path != "" {
		identity, identityErr := targetRevisionIdentity(target.Path)
		if identityErr != nil {
			findings = append(findings, finding("Review target", "target identity", "target/.identity", "target identity unavailable", identityErr.Error(), "Restore a contained readable target tree."))
			manifest.Inputs = append(manifest.Inputs, ReviewInput{ID: "target-identity", Kind: "target", Name: "target identity", Path: "target/.identity", Hash: "missing"})
		} else {
			manifest.Inputs = append(manifest.Inputs, ReviewInput{ID: "target-identity", Kind: "target", Name: "target identity", Path: "target/.identity", Hash: identity})
		}
	}
	resolve := func(selected SelectedItem, section project.CatalogSection, kind, prefix string, reviewer bool) {
		entry, ok := catalogEntry(catalog, section, selected)
		if !ok {
			findings = append(findings, finding("Review manifest", selected.Name, selected.Path, "catalog entry unresolved", "selected entry is not uniquely resolvable", "Fix project-index.md or sprint-index.md."))
			return
		}
		path := entry.Path
		full, pathErr := workspace.ResolveInside(s.root, path)
		if pathErr != nil {
			findings = append(findings, finding("Review manifest", selected.Name, path, "unsafe catalog path", pathErr.Error(), "Use a contained workspace path."))
			return
		}
		data, readErr := os.ReadFile(full)
		if readErr != nil {
			manifest.Inputs = append(manifest.Inputs, ReviewInput{ID: prefix + "-" + slugReviewID(selected.Name), Kind: kind, Name: selected.Name, Path: path, Hash: "missing"})
			findings = append(findings, finding("Review manifest", selected.Name, path, "unreadable catalog entry", readErr.Error(), "Restore the selected catalog file."))
			return
		}
		id := prefix + "-" + slugReviewID(selected.Name)
		in := reviewInput(id, kind, selected.Name, path, string(data))
		manifest.Inputs = append(manifest.Inputs, in)
		if reviewer {
			manifest.Coverage = append(manifest.Coverage, in)
		}
		manifest.Contents[path] = string(data)
	}
	for _, selected := range idx.Contracts {
		resolve(selected, project.SectionActiveContractPool, "contract", "contract", true)
	}
	for _, selected := range idx.ReviewProtocols {
		resolve(selected, project.SectionReviewProtocols, "protocol", "protocol", false)
	}
	if unresolvedExecutePlanTasks(manifest.Contents[ArtifactRelPath(sp, StagePlan)], manifest.Contents[runPath], planManifest) {
		findings = append(findings, finding("Plan Execution", "tasks", ArtifactRelPath(sp, StagePlan), "plan tasks are not resolved", "one or more unchecked top-level tasks lack an explicit deferred run-state outcome", "Complete each executable task or defer it with an explicit rationale before review."))
	}
	// The handbook receives an independent reviewer even though it is also a governed input.
	if handbook := findReviewInput(manifest.Inputs, "technical-handbook"); handbook.Path != "" {
		handbook.ID, handbook.Kind, handbook.Name = "handbook", "handbook", "Technical Handbook"
		manifest.Coverage = append(manifest.Coverage, handbook)
	}
	if strings.TrimSpace(manifest.Model) == "" || manifest.Model == "unresolved" {
		findings = append(findings, finding("Configuration", "review model", "", "missing review model", "no review, plan, or runtime model is configured", "Set planning.review_model or planning.plan_model."))
	}
	for _, command := range reviewVerificationCommands(manifest.Contents[ArtifactRelPath(sp, StagePlan)]) {
		if !strings.Contains(manifest.Contents[ArtifactRelPath(sp, StageExecute)], command) {
			findings = append(findings, finding("Verification Evidence", command, ArtifactRelPath(sp, StageExecute), "approved verification evidence missing", "execute.md does not record the planned command", "Run the approved command and record its result in execute.md."))
		}
	}
	if run := manifest.Contents[runPath]; run != "" && reviewRunStateIncomplete([]byte(run)) {
		findings = append(findings, finding("Plan Execution", "run-state", runPath, "execute is incomplete", "run state contains non-complete tasks or status", "Complete or safely resolve execute before review."))
	}
	if len(manifest.ChangedPaths) == 0 {
		manifest.ChangedPaths = []string{"(execute evidence did not enumerate changed paths)"}
	} else {
		for _, changed := range manifest.ChangedPaths {
			full := changed
			if !filepath.IsAbs(full) {
				full = filepath.Join(manifest.Target, filepath.FromSlash(changed))
			}
			if !inside(manifest.Target, full) {
				findings = append(findings, finding("Review target", changed, changed, "changed path escapes target", "path is outside the approved implementation target", "Use contained execute evidence."))
				continue
			}
			data, readErr := os.ReadFile(full)
			if readErr != nil {
				findings = append(findings, finding("Review target", changed, changed, "changed target input unreadable", readErr.Error(), "Restore the changed path or correct execute evidence."))
				continue
			}
			rel, _ := filepath.Rel(manifest.Target, full)
			manifest.Inputs = append(manifest.Inputs, reviewInput("target-"+slugReviewID(rel), "target", rel, "target/"+filepath.ToSlash(rel), string(data)))
			manifest.Contents["target/"+filepath.ToSlash(rel)] = string(data)
		}
	}
	sort.Slice(manifest.Coverage, func(i, j int) bool { return manifest.Coverage[i].ID < manifest.Coverage[j].ID })
	sort.Slice(manifest.Inputs, func(i, j int) bool { return manifest.Inputs[i].Path < manifest.Inputs[j].Path })
	manifest.Fingerprint = fingerprintReviewManifest(manifest)
	sortSprintFindings(findings)
	return manifest, findings, nil
}

func (s Service) PromptReview(projectRef, sprintRef string, req ReviewRequest) (PromptPreview, error) {
	m, findings, err := s.PrepareReview(projectRef, sprintRef, req)
	if err != nil {
		return PromptPreview{}, err
	}
	if len(findings) > 0 {
		return PromptPreview{}, fmt.Errorf("review prerequisites failed validation")
	}
	sp, inputs, _, err := s.resolveSprintInputs(projectRef, sprintRef)
	if err != nil {
		return PromptPreview{}, err
	}
	m.SharedPrefix, err = s.prepareSharedPromptContext(context.Background(), sp, inputs, false)
	if err != nil {
		return PromptPreview{}, err
	}
	stagePrompt := renderReviewPreview(m)
	if len(m.Coverage) > 0 {
		stagePrompt = renderReviewerPrompt(m, m.Coverage[0])
	}
	prompt, err := composeStagePromptChecked(m.SharedPrefix, stagePrompt)
	if err != nil {
		return PromptPreview{}, err
	}
	explanation := explainComposedPrompt(prompt)
	return PromptPreview{Project: m.Project, Sprint: m.Sprint, Prompt: prompt, Explanation: &explanation}, nil
}

func (s Service) Review(ctx context.Context, projectRef, sprintRef string, req ReviewRequest) (ReviewResult, error) {
	m, findings, err := s.PrepareReview(projectRef, sprintRef, req)
	result := ReviewResult{Project: m.Project, Sprint: m.Sprint, DryRun: req.DryRun, Fingerprint: m.Fingerprint, Artifact: reviewArtifact(m), Status: ReviewReady, Focused: append([]string(nil), req.Focus...)}
	if err != nil {
		return result, err
	}
	if !req.DryRun && !req.PromptOnly {
		lockedCtx, release, lockErr := s.acquireMutationContext(ctx, projectRef, sprintRef)
		if lockErr != nil {
			return result, lockErr
		}
		defer release()
		ctx = lockedCtx
	}
	if len(findings) > 0 {
		result.Status, result.Verdict = ReviewBlocked, ReviewVerdictBlocked
		for _, f := range findings {
			result.Diagnostics = append(result.Diagnostics, ReviewDiagnostic{Code: "preflight", Message: safeReviewText(s.root, f.Problem+": "+f.Cause)})
		}
		if !req.DryRun {
			if saveErr := s.saveReviewState(projectRef, sprintRef, result, 0, len(m.Coverage)); saveErr != nil {
				return result, errors.Join(fmt.Errorf("review prerequisites failed validation"), saveErr)
			}
		}
		return result, fmt.Errorf("review prerequisites failed validation")
	}
	sp, inputs, _, err := s.resolveSprintInputs(projectRef, sprintRef)
	if err != nil {
		return result, err
	}
	m.SharedPrefix, err = s.prepareSharedPromptContext(ctx, sp, inputs, true)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			result.Status = ReviewCancelled
			result.Diagnostics = append(result.Diagnostics, ReviewDiagnostic{Code: "cancelled", Message: safeReviewText(s.root, err.Error())})
			if !req.DryRun && !req.PromptOnly {
				return s.persistReviewFailure(projectRef, sprintRef, result, 0, len(m.Coverage), err)
			}
		}
		return result, err
	}
	result.Prompt, err = composeStagePromptChecked(m.SharedPrefix, renderReviewPreview(m))
	if err != nil {
		return result, err
	}
	if req.DryRun || req.PromptOnly {
		result.Message = "review dry run"
		return result, nil
	}
	if s.runtime == nil {
		return result, fmt.Errorf("runtime is required for review")
	}
	if req.Restart && len(req.Focus) > 0 {
		return result, fmt.Errorf("--restart cannot be combined with focused review")
	}
	result.Status = ReviewRunning
	result.Restarted = req.Restart
	if err := s.saveReviewState(projectRef, sprintRef, result, 0, len(m.Coverage)); err != nil {
		return result, err
	}
	runCoverage, coverage, focusErr := s.reviewCoveragePlan(projectRef, sprintRef, m, req.Focus)
	if focusErr != nil {
		result.Status, result.Verdict = ReviewBlocked, ReviewVerdictBlocked
		result.Diagnostics = append(result.Diagnostics, ReviewDiagnostic{Code: "focus", Message: safeReviewText(s.root, focusErr.Error())})
		return s.persistReviewFailure(projectRef, sprintRef, result, 0, len(m.Coverage), focusErr)
	}
	resumeSessions := map[string]string{}
	completed := 0
	if len(req.Focus) == 0 {
		resume, rebased, resumeErr := s.initializeReviewResume(projectRef, sprintRef, m, req.Restart)
		if resumeErr != nil {
			result.Status, result.Verdict = ReviewFailed, ReviewVerdictBlocked
			result.Diagnostics = append(result.Diagnostics, ReviewDiagnostic{Code: "resume-state", Message: safeReviewText(s.root, resumeErr.Error())})
			return s.persistReviewFailure(projectRef, sprintRef, result, 0, len(m.Coverage), resumeErr)
		}
		runCoverage, coverage, resumeSessions, completed = reviewResumePlan(s.root, m, resume)
		result.Resumed = !req.Restart && (completed > 0 || len(resumeSessions) > 0)
		result.Reused = completed
		if rebased > 0 {
			result.Diagnostics = append(result.Diagnostics, ReviewDiagnostic{Code: "resume-rebased", Message: fmt.Sprintf("reused %d validated reviewer result(s) from the prior input fingerprint; incomplete coverage uses the current snapshot", rebased)})
		}
	}
	reviewerRoot, snapshotErr := prepareReviewSnapshot(m)
	if snapshotErr != nil {
		result.Status, result.Verdict = ReviewFailed, ReviewVerdictBlocked
		result.Diagnostics = append(result.Diagnostics, ReviewDiagnostic{Code: "snapshot", Message: safeReviewText(s.root, snapshotErr.Error())})
		if saveErr := s.saveReviewState(projectRef, sprintRef, result, 0, len(m.Coverage)); saveErr != nil {
			return result, errors.Join(snapshotErr, saveErr)
		}
		return result, snapshotErr
	}
	m.ReviewerRoot = reviewerRoot
	workers := m.Concurrency
	if workers > len(runCoverage) {
		workers = len(runCoverage)
	}
	if workers < 1 {
		workers = 1
	}
	type item struct {
		index     int
		value     ReviewCoverageResult
		sessionID string
	}
	type sessionItem struct {
		coverageID string
		sessionID  string
	}
	jobs := make(chan int)
	done := make(chan item, len(runCoverage))
	sessions := make(chan sessionItem, max(1, len(runCoverage)*4))
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				c := runCoverage[i]
				sessionID := resumeSessions[c.ID]
				lastSessionID := sessionID
				value, finalSessionID := s.runReviewer(ctx, m, c, sessionID, func(id string) {
					if id != "" && id != lastSessionID {
						lastSessionID = id
						sessions <- sessionItem{coverageID: c.ID, sessionID: id}
					}
				})
				done <- item{index: i, value: value, sessionID: finalSessionID}
			}
		}()
	}
	go func() {
		for i := range runCoverage {
			jobs <- i
		}
		close(jobs)
		wg.Wait()
		close(sessions)
		close(done)
	}()
	var progressSaveErr error
	for done != nil || sessions != nil {
		select {
		case session, ok := <-sessions:
			if !ok {
				sessions = nil
				continue
			}
			if session.sessionID != "" && len(req.Focus) == 0 {
				if saveErr := s.saveReviewResumeSession(projectRef, sprintRef, m.Fingerprint, session.coverageID, session.sessionID); saveErr != nil {
					progressSaveErr = errors.Join(progressSaveErr, saveErr)
				}
			}
		case got, ok := <-done:
			if !ok {
				done = nil
				continue
			}
			if got.value.Error == "" && !reviewCoverageCheckpointValid(s.root, m, got.value) {
				got.value.Error = "reviewer result failed schema or citation validation"
			}
			for i := range m.Coverage {
				if m.Coverage[i].ID == got.value.CoverageID {
					coverage[i] = got.value
					break
				}
			}
			completed++
			if req.Progress != nil {
				req.Progress(ReviewProgress{Completed: completed, Total: len(coverage), CoverageID: got.value.CoverageID, Message: "reviewer complete"})
			}
			result.Coverage = coverage
			if len(req.Focus) == 0 {
				if saveErr := s.saveReviewResumeResult(projectRef, sprintRef, m.Fingerprint, got.value, got.sessionID); saveErr != nil {
					progressSaveErr = errors.Join(progressSaveErr, saveErr)
				}
			}
			if saveErr := s.saveReviewState(projectRef, sprintRef, result, completed, len(coverage)); saveErr != nil {
				progressSaveErr = errors.Join(progressSaveErr, saveErr)
			}
		}
	}
	result.Coverage = coverage
	validatedFindings, validationDiagnostics, validatedVerdict := validateReviewCoverage(s.root, m, coverage)
	result.Findings = validatedFindings
	result.Diagnostics = append(result.Diagnostics, validationDiagnostics...)
	result.Verdict = validatedVerdict
	if progressSaveErr != nil {
		result.Status, result.Verdict = ReviewFailed, ReviewVerdictBlocked
		result.Diagnostics = append(result.Diagnostics, ReviewDiagnostic{Code: "state-write-failed", Message: safeReviewText(s.root, progressSaveErr.Error())})
		return s.persistReviewFailure(projectRef, sprintRef, result, completed, len(coverage), progressSaveErr)
	}
	if ctx.Err() != nil {
		result.Status = ReviewCancelled
		result.Diagnostics = append(result.Diagnostics, ReviewDiagnostic{Code: "cancelled", Message: ctx.Err().Error()})
		return s.persistReviewFailure(projectRef, sprintRef, result, completed, len(coverage), ctx.Err())
	}
	if result.Verdict == ReviewVerdictBlocked {
		result.Status = ReviewFailed
		return s.persistReviewFailure(projectRef, sprintRef, result, completed, len(coverage), fmt.Errorf("review failed to produce complete valid coverage"))
	}
	current, currentFindings, currentErr := s.PrepareReview(projectRef, sprintRef, req)
	if currentErr != nil || len(currentFindings) > 0 || current.Fingerprint != m.Fingerprint {
		for _, message := range reviewManifestChanges(m, current, currentFindings, currentErr) {
			result.Diagnostics = append(result.Diagnostics, ReviewDiagnostic{Code: "inputs-changed", Message: safeReviewText(s.root, message)})
		}
	}
	result.Status = ReviewCompleted
	content := RenderReviewMarkdown(m, result)
	if vf := ValidateReviewContent(content, m); len(vf) > 0 {
		result.Status = ReviewFailed
		result.Diagnostics = append(result.Diagnostics, ReviewDiagnostic{Code: "artifact-invalid", Message: vf[0].Problem})
		return s.persistReviewFailure(projectRef, sprintRef, result, completed, len(coverage), fmt.Errorf("generated review.md failed validation"))
	}
	path, _ := ArtifactPath(s.root, sp, StageReview)
	if err := atomicWriteReview(path, []byte(content)); err != nil {
		result.Status = ReviewFailed
		result.Diagnostics = append(result.Diagnostics, ReviewDiagnostic{Code: "write-failed", Message: safeError(err)})
		return s.persistReviewFailure(projectRef, sprintRef, result, completed, len(coverage), err)
	}
	now := s.now().UTC()
	result.Message = "review complete"
	result.Artifact = ArtifactRelPath(sp, StageReview)
	if err := s.saveReviewState(projectRef, sprintRef, result, completed, len(coverage)); err != nil {
		return result, err
	}
	_ = os.RemoveAll(reviewerRoot)
	_ = now
	if result.Verdict == ReviewFail {
		return result, fmt.Errorf("review completed with failing verdict")
	}
	return result, nil
}

func (s Service) persistReviewFailure(projectRef, sprintRef string, result ReviewResult, completed, total int, cause error) (ReviewResult, error) {
	if saveErr := s.saveReviewState(projectRef, sprintRef, result, completed, total); saveErr != nil {
		return result, errors.Join(cause, fmt.Errorf("persist terminal review state: %w", saveErr))
	}
	return result, cause
}

func (s Service) reviewCoveragePlan(projectRef, sprintRef string, m ReviewManifest, focus []string) ([]ReviewInput, []ReviewCoverageResult, error) {
	coverage := make([]ReviewCoverageResult, len(m.Coverage))
	if len(focus) == 0 {
		return m.Coverage, coverage, nil
	}
	wanted := map[string]bool{}
	for _, raw := range focus {
		for _, id := range strings.Split(raw, ",") {
			id = strings.TrimSpace(id)
			if id != "" {
				wanted[id] = true
			}
		}
	}
	state, err := LoadFlowState(s.root, Sprint{Project: m.Project, Slug: m.Sprint, Path: filepath.Join(s.root, filepath.FromSlash(m.SprintRoot))})
	if err != nil || state.Review == nil || state.Review.LastComplete == nil {
		return nil, nil, fmt.Errorf("focused review requires a previous complete review with retained coverage")
	}
	previous := state.Review.LastComplete
	if previous.InputFingerprint != m.Fingerprint {
		return nil, nil, fmt.Errorf("focused review cannot retain coverage from a different input fingerprint")
	}
	retained := map[string]ReviewCoverageResult{}
	for _, item := range previous.Coverage {
		retained[item.CoverageID] = item
	}
	var run []ReviewInput
	for i, item := range m.Coverage {
		if wanted[item.ID] {
			run = append(run, item)
			delete(wanted, item.ID)
			continue
		}
		prior, ok := retained[item.ID]
		if !ok || prior.Error != "" {
			return nil, nil, fmt.Errorf("focused review lacks valid retained coverage %q", item.ID)
		}
		coverage[i] = prior
	}
	if len(wanted) > 0 {
		return nil, nil, fmt.Errorf("focused review names unknown coverage")
	}
	if len(run) == 0 {
		return nil, nil, fmt.Errorf("focused review requires at least one coverage id")
	}
	return run, coverage, nil
}

func (s Service) initializeReviewResume(projectRef, sprintRef string, m ReviewManifest, restart bool) (ReviewResumeState, int, error) {
	sp, _, _, err := s.resolveSprintInputs(projectRef, sprintRef)
	if err != nil {
		return ReviewResumeState{}, 0, err
	}
	state, err := LoadFlowState(s.root, sp)
	if err != nil {
		return ReviewResumeState{}, 0, err
	}
	if state.Review == nil || state.Review.ActiveAttempt == nil {
		return ReviewResumeState{}, 0, fmt.Errorf("review attempt state is unavailable")
	}
	current := state.Review.Resume
	rebased := 0
	if restart || !reviewResumeCompatible(current, m) {
		checkpoints := make([]ReviewCoverageCheckpoint, 0, len(m.Coverage))
		now := s.now().UTC()
		prior := map[string]ReviewCoverageCheckpoint{}
		if !restart && reviewResumeShapeCompatible(current, m) {
			for _, checkpoint := range current.Coverage {
				prior[checkpoint.CoverageID] = checkpoint
			}
		}
		for _, item := range m.Coverage {
			checkpoint := ReviewCoverageCheckpoint{CoverageID: item.ID, Status: AttemptPending, UpdatedAt: now}
			if previous, ok := prior[item.ID]; ok && previous.Status == AttemptCompleted && previous.Result != nil && reviewCoverageCheckpointValid(s.root, m, *previous.Result) {
				checkpoint.Status = AttemptCompleted
				checkpoint.Result = previous.Result
				rebased++
			}
			checkpoints = append(checkpoints, checkpoint)
		}
		current = &ReviewResumeState{
			AttemptID:        state.Review.ActiveAttempt.ID,
			InputFingerprint: m.Fingerprint,
			Model:            m.Model,
			UpdatedAt:        now,
			Coverage:         checkpoints,
		}
	} else {
		current.AttemptID = state.Review.ActiveAttempt.ID
		current.UpdatedAt = s.now().UTC()
	}
	state.Review.Resume = current
	if err := SaveFlowState(s.root, sp, state); err != nil {
		return ReviewResumeState{}, 0, err
	}
	return *current, rebased, nil
}

func reviewResumeCompatible(state *ReviewResumeState, m ReviewManifest) bool {
	return state != nil && state.InputFingerprint == m.Fingerprint && reviewResumeShapeCompatible(state, m)
}

func reviewResumeShapeCompatible(state *ReviewResumeState, m ReviewManifest) bool {
	if state == nil || state.Model != m.Model || len(state.Coverage) != len(m.Coverage) {
		return false
	}
	seen := map[string]bool{}
	for _, item := range state.Coverage {
		seen[item.CoverageID] = true
	}
	for _, item := range m.Coverage {
		if !seen[item.ID] {
			return false
		}
	}
	return true
}

func reviewResumePlan(root string, m ReviewManifest, state ReviewResumeState) ([]ReviewInput, []ReviewCoverageResult, map[string]string, int) {
	checkpoints := map[string]ReviewCoverageCheckpoint{}
	for _, item := range state.Coverage {
		checkpoints[item.CoverageID] = item
	}
	coverage := make([]ReviewCoverageResult, len(m.Coverage))
	sessions := map[string]string{}
	run := make([]ReviewInput, 0, len(m.Coverage))
	completed := 0
	for i, item := range m.Coverage {
		checkpoint := checkpoints[item.ID]
		if checkpoint.Status == AttemptCompleted && checkpoint.Result != nil && reviewCoverageCheckpointValid(root, m, *checkpoint.Result) {
			coverage[i] = *checkpoint.Result
			completed++
			continue
		}
		if checkpoint.SessionID != "" && state.InputFingerprint == m.Fingerprint {
			sessions[item.ID] = checkpoint.SessionID
		}
		run = append(run, item)
	}
	return run, coverage, sessions, completed
}

func reviewCoverageCheckpointValid(root string, m ReviewManifest, result ReviewCoverageResult) bool {
	if result.Error != "" {
		return false
	}
	single := m
	single.Coverage = []ReviewInput{{ID: result.CoverageID}}
	_, diagnostics, _ := validateReviewCoverage(root, single, []ReviewCoverageResult{result})
	return len(diagnostics) == 0
}

func (s Service) saveReviewResumeSession(projectRef, sprintRef, fingerprint, coverageID, sessionID string) error {
	return s.updateReviewResume(projectRef, sprintRef, fingerprint, coverageID, func(checkpoint *ReviewCoverageCheckpoint, now time.Time) {
		// Session events and completed results arrive on separate channels. A
		// buffered session event may therefore be persisted after the terminal
		// result; never let that late event restore a cleared session or regress a
		// terminal checkpoint to running.
		if checkpoint.Status == AttemptCompleted || checkpoint.Status == AttemptFailed {
			return
		}
		checkpoint.SessionID = sessionID
		checkpoint.Status = AttemptRunning
		checkpoint.UpdatedAt = now
	})
}

func (s Service) saveReviewResumeResult(projectRef, sprintRef, fingerprint string, result ReviewCoverageResult, sessionID string) error {
	return s.updateReviewResume(projectRef, sprintRef, fingerprint, result.CoverageID, func(checkpoint *ReviewCoverageCheckpoint, now time.Time) {
		checkpoint.UpdatedAt = now
		if result.Error == "" {
			checkpoint.SessionID = ""
			checkpoint.Status = AttemptCompleted
			copy := result
			checkpoint.Result = &copy
		} else {
			checkpoint.SessionID = sessionID
			checkpoint.Status = AttemptFailed
			checkpoint.Result = nil
		}
	})
}

func (s Service) updateReviewResume(projectRef, sprintRef, fingerprint, coverageID string, update func(*ReviewCoverageCheckpoint, time.Time)) error {
	sp, _, _, err := s.resolveSprintInputs(projectRef, sprintRef)
	if err != nil {
		return err
	}
	state, err := LoadFlowState(s.root, sp)
	if err != nil {
		return err
	}
	if state.Review == nil || state.Review.Resume == nil || state.Review.Resume.InputFingerprint != fingerprint {
		return fmt.Errorf("review resume checkpoint no longer matches active inputs")
	}
	now := s.now().UTC()
	if state.Review.ActiveAttempt != nil {
		state.Review.ActiveAttempt.HeartbeatAt = now
		state.Review.ActiveAttempt.OwnerPID = os.Getpid()
	}
	for i := range state.Review.Resume.Coverage {
		if state.Review.Resume.Coverage[i].CoverageID == coverageID {
			update(&state.Review.Resume.Coverage[i], now)
			state.Review.Resume.UpdatedAt = now
			return SaveFlowState(s.root, sp, state)
		}
	}
	return fmt.Errorf("review resume checkpoint missing coverage %q", coverageID)
}

func (s Service) ValidateReview(projectRef, sprintRef string) (ValidationResult, error) {
	m, findings, err := s.PrepareReview(projectRef, sprintRef, ReviewRequest{})
	if err != nil {
		return ValidationResult{}, err
	}
	sp, _, _, _ := s.resolveSprintInputs(projectRef, sprintRef)
	path := ArtifactRelPath(sp, StageReview)
	if len(findings) == 0 {
		data, readErr := s.store.ReadArtifact(sp, StageReview)
		if readErr != nil {
			findings = append(findings, finding("review.md", "", path, "missing review", readErr.Error(), "Run the review stage."))
		} else {
			findings = append(findings, ValidateReviewContent(data, m)...)
		}
	}
	return ValidationResult{Project: sp.Project, Sprint: sp.Slug, Artifact: path, Findings: findings}, nil
}

func (s Service) runReviewer(ctx context.Context, m ReviewManifest, c ReviewInput, resumeSession string, onSession func(string)) (out ReviewCoverageResult, sessionID string) {
	out.CoverageID = c.ID
	defer func() {
		if r := recover(); r != nil {
			out.Error = fmt.Sprintf("reviewer panic: %v", r)
		}
	}()
	stagePrompt := renderReviewerPrompt(m, c)
	if resumeSession != "" {
		stagePrompt = "Resume the interrupted review using the refreshed frozen snapshot paths in this request. " + stagePrompt
	}
	prompt, composeErr := composeStagePromptChecked(m.SharedPrefix, stagePrompt)
	if composeErr != nil {
		out.Error = safeReviewText(s.root, composeErr.Error())
		return
	}
	if len(prompt) > reviewPromptMaxBytes {
		out.Error = fmt.Sprintf("review prompt exceeds safe subprocess argument budget: %d > %d bytes", len(prompt), reviewPromptMaxBytes)
		return
	}
	req := s.runtimeRequest(prompt, map[string]string{"project": m.Project, "sprint": m.Sprint, "stage": string(StageReview), "coverage": c.ID, "model_source": m.ModelSource})
	req.WorkDir = m.ReviewerRoot
	req.Model = strings.TrimPrefix(m.Model, req.Provider+"/")
	req.Sandbox = "read_only"
	req.Permissions = "restricted"
	req.RequireCaps = appendUnique(req.RequireCaps, "permissions")
	req.Policy = pruntime.PermissionPolicy{Default: "deny", Tools: map[string]string{"read": "allow", "list": "allow", "search": "allow"}}
	captured := &reviewOutputCapture{}
	req.Validation = s.reviewValidationSpec(m, c, captured)
	if resumeSession != "" {
		req.SessionID, req.SessionAction = resumeSession, "continue"
	}
	previousOnEvent := req.OnEvent
	req.OnEvent = func(event pruntime.Event) {
		captured.observe(event.Payload)
		if previousOnEvent != nil {
			previousOnEvent(event)
		}
		if event.SessionID != "" {
			if onSession != nil {
				onSession(event.SessionID)
			}
		}
	}
	sp := Sprint{Project: m.Project, Slug: m.Sprint, Path: filepath.Join(s.root, "projects", m.Project, "sprints", m.Sprint)}
	r, err := s.startSprintRuntime(ctx, sp, StageReview, req)
	sessionID = r.SessionID
	if sessionID != "" && onSession != nil {
		onSession(sessionID)
	}
	if r.Permissions.UnsupportedCount > 0 {
		out.Error = "runtime could not enforce review permission policy"
		return
	}
	if ctx.Err() != nil {
		out.Error = safeReviewText(s.root, ctx.Err().Error())
		return
	}
	candidate, problems := extractValidatedReviewResult(s.root, m, c.ID, r)
	if err == nil && len(problems) == 0 {
		return candidate, sessionID
	}
	// The production AgentWrap stack already performed its bounded repairs.
	// Runtime implementations without that wrapper retain a small equivalent
	// fallback here so the product boundary remains deterministic in tests and
	// alternate adapters.
	if r.Validation.Configured {
		if len(problems) == 0 && !r.Validation.Passed && len(r.Validation.Details) > 0 {
			problems = append(problems, r.Validation.Details...)
		}
		if len(problems) > 0 {
			out.Error = safeReviewText(s.root, strings.Join(problems, "; "))
		} else if err != nil {
			out.Error = safeReviewText(s.root, err.Error())
		}
		return
	}
	if len(problems) == 0 && err != nil {
		problems = []string{safeReviewText(s.root, err.Error())}
	}
	for attempt := 1; attempt <= 2; attempt++ {
		repair := req
		repair.Validation = nil
		repair.Prompt, composeErr = composeStagePromptChecked(m.SharedPrefix, buildReviewRepairPrompt(m, c, problems, r.TerminalOutput))
		if composeErr != nil {
			out.Error = safeReviewText(s.root, composeErr.Error())
			return
		}
		if attempt == 1 && sessionID != "" {
			repair.SessionID, repair.SessionAction = sessionID, "continue"
		} else {
			repair.SessionID, repair.SessionAction = "", "fresh"
		}
		repaired, repairErr := s.startSprintRuntime(ctx, sp, StageReview, repair)
		if repaired.SessionID != "" {
			sessionID = repaired.SessionID
			if onSession != nil {
				onSession(sessionID)
			}
		}
		if repaired.Permissions.UnsupportedCount > 0 {
			out.Error = "runtime could not enforce review permission policy during repair"
			return
		}
		if ctx.Err() != nil {
			out.Error = safeReviewText(s.root, ctx.Err().Error())
			return
		}
		candidate, problems = extractValidatedReviewResult(s.root, m, c.ID, repaired)
		if repairErr == nil && len(problems) == 0 {
			return candidate, sessionID
		}
		if len(problems) == 0 && repairErr != nil {
			problems = []string{safeReviewText(s.root, repairErr.Error())}
		}
	}
	out.Error = safeReviewText(s.root, "structured review result remained invalid after bounded repair: "+strings.Join(problems, "; "))
	return
}

func validateReviewCoverage(root string, m ReviewManifest, results []ReviewCoverageResult) ([]ReviewFinding, []ReviewDiagnostic, ReviewVerdict) {
	var findings []ReviewFinding
	var diagnostics []ReviewDiagnostic
	coverage := map[string]bool{}
	findingIDs := map[string]string{}
	for i, r := range results {
		expectedID := r.CoverageID
		if i < len(m.Coverage) {
			expectedID = m.Coverage[i].ID
		}
		if r.Error != "" {
			diagnostics = append(diagnostics, ReviewDiagnostic{Code: "reviewer-failed", CoverageID: expectedID, Message: r.Error})
			continue
		}
		normalizeReviewResultForManifest(m, &r)
		problems := reviewResultProblems(root, m, expectedID, r)
		if len(problems) > 0 {
			diagnostics = append(diagnostics, ReviewDiagnostic{Code: "invalid-result", CoverageID: expectedID, Message: safeReviewText(root, strings.Join(problems, "; "))})
			continue
		}
		coverage[r.CoverageID] = true
		for _, f := range r.Findings {
			if priorCoverage, duplicate := findingIDs[f.ID]; duplicate {
				diagnostics = append(diagnostics, ReviewDiagnostic{Code: "duplicate-finding-id", CoverageID: r.CoverageID, Message: fmt.Sprintf("finding id %q already emitted by %s", f.ID, priorCoverage)})
				continue
			}
			findingIDs[f.ID] = r.CoverageID
			findings = append(findings, f)
		}
	}
	for _, c := range m.Coverage {
		if !coverage[c.ID] {
			diagnostics = append(diagnostics, ReviewDiagnostic{Code: "missing-coverage", CoverageID: c.ID, Message: "required reviewer result missing"})
		}
	}
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].Severity != findings[j].Severity {
			return findings[i].Severity < findings[j].Severity
		}
		return findings[i].ID < findings[j].ID
	})
	if len(diagnostics) > 0 {
		return findings, diagnostics, ReviewVerdictBlocked
	}
	verdict := ReviewPass
	for _, f := range findings {
		if !reviewApplicable(f.Applicability) {
			continue
		}
		if f.Severity == "high" || f.Severity == "blocker" {
			verdict = ReviewFail
			break
		}
		verdict = ReviewPassWithFindings
	}
	return findings, diagnostics, verdict
}

func RenderReviewMarkdown(m ReviewManifest, r ReviewResult) string {
	var b strings.Builder
	fmt.Fprintln(&b, "# Sprint Review")
	fmt.Fprintf(&b, "\nReview status: `%s`\nVerdict: `%s`\nInput fingerprint: `%s`\nModel: `%s`\nModel source: `%s`\nTarget: `%s`\n", r.Status, r.Verdict, m.Fingerprint, m.Model, m.ModelSource, m.Target)
	sections := []string{"Review Context", "Input Fingerprint And Scope", "Decision Conformance", "Plan Execution", "Verification Evidence", "Contract Conformance", "Technical Handbook Conformance", "Applicability And Deferred Scope", "Findings", "Deviations", "Final Assessment"}
	for _, section := range sections {
		fmt.Fprintf(&b, "\n## %s\n\n", section)
		switch section {
		case "Review Context":
			fmt.Fprintf(&b, "Project `%s`, sprint `%s`; automated product-owned review.\n", m.Project, m.Sprint)
		case "Input Fingerprint And Scope":
			for _, in := range m.Inputs {
				fmt.Fprintf(&b, "- `%s` `%s`\n", in.Path, in.Hash)
			}
			fmt.Fprintln(&b, "- Changed paths:")
			for _, p := range m.ChangedPaths {
				fmt.Fprintf(&b, "  - `%s`\n", p)
			}
		case "Contract Conformance", "Technical Handbook Conformance", "Applicability And Deferred Scope":
			for _, c := range r.Coverage {
				fmt.Fprintf(&b, "- `%s` — %s: %s\n", c.CoverageID, firstNonEmptyString(c.Applicability, "invalid"), firstNonEmptyString(c.Summary, c.Error, "no summary"))
			}
		case "Findings":
			if len(r.Findings) == 0 {
				fmt.Fprintln(&b, "No applicable findings.")
			} else {
				for _, f := range r.Findings {
					fmt.Fprintf(&b, "- [%s] `%s` %s — %s\n", f.Severity, f.ID, f.Title, f.Detail)
					fmt.Fprintf(&b, "  - action: %s\n", f.Action)
					for _, c := range f.Citations {
						fmt.Fprintf(&b, "  - citation: `%s:%d-%d`\n", c.Path, c.StartLine, c.EndLine)
					}
				}
			}
		case "Deviations":
			if len(r.Diagnostics) == 0 {
				fmt.Fprintln(&b, "None.")
			} else {
				for _, d := range r.Diagnostics {
					fmt.Fprintf(&b, "- `%s` %s\n", d.Code, d.Message)
				}
			}
		case "Final Assessment":
			fmt.Fprintf(&b, "Deterministic verdict: `%s`.\n", r.Verdict)
		default:
			fmt.Fprintln(&b, "Covered by the frozen manifest, deterministic checks, and cited reviewer evidence above.")
		}
	}
	return b.String()
}

func ValidateReviewContent(content string, m ReviewManifest) []ValidationFinding {
	var out []ValidationFinding
	if strings.TrimSpace(content) == "" || containsPlaceholder(content) {
		out = append(out, finding("review.md", "content", reviewArtifact(m), "empty or placeholder review", "canonical review must contain complete rendered evidence", "Rerun review."))
	}
	for _, h := range []string{"Review Context", "Input Fingerprint And Scope", "Decision Conformance", "Plan Execution", "Verification Evidence", "Contract Conformance", "Technical Handbook Conformance", "Applicability And Deferred Scope", "Findings", "Deviations", "Final Assessment"} {
		if !markdownHeadingPresent(content, h) {
			out = append(out, finding("review.md", h, reviewArtifact(m), "missing required section", "section was not found", "Regenerate review.md."))
		}
	}
	if !strings.Contains(content, "Input fingerprint: `"+m.Fingerprint+"`") {
		out = append(out, finding("review.md", "fingerprint", reviewArtifact(m), "stale or missing fingerprint", "artifact does not match current governed inputs", "Rerun review."))
	}
	validVerdict := false
	for _, v := range []ReviewVerdict{ReviewPass, ReviewPassWithFindings, ReviewFail} {
		if strings.Contains(content, "Verdict: `"+string(v)+"`") {
			validVerdict = true
		}
	}
	if !validVerdict {
		out = append(out, finding("review.md", "verdict", reviewArtifact(m), "missing valid verdict", "verdict is absent or unsupported", "Rerun review."))
	}
	for _, coverage := range m.Coverage {
		if !strings.Contains(content, "`"+coverage.ID+"`") {
			out = append(out, finding("review.md", coverage.ID, reviewArtifact(m), "missing reviewer coverage", "required coverage id is absent", "Rerun review."))
		}
	}
	return out
}

func validateReviewStageState(root string, sp Sprint, state ReviewStageState, path string) error {
	if state.Path != ArtifactRelPath(sp, StageReview) {
		return fmt.Errorf("%w: %s: review path mismatch", ErrFlowStateMalformed, path)
	}
	switch state.Status {
	case ReviewReady, ReviewRunning, ReviewCompleted, ReviewFailed, ReviewCancelled, ReviewBlocked:
	default:
		return fmt.Errorf("%w: %s: unsupported review status %q", ErrFlowStateMalformed, path, state.Status)
	}
	if strings.ContainsAny(state.Fingerprint, "\x00\r\n") || state.Completed < 0 || state.Total < 0 || state.Completed > state.Total {
		return fmt.Errorf("%w: %s: invalid review state", ErrFlowStateMalformed, path)
	}
	if state.Verdict != "" && state.Verdict != ReviewPass && state.Verdict != ReviewPassWithFindings && state.Verdict != ReviewFail && state.Verdict != ReviewVerdictBlocked {
		return fmt.Errorf("%w: %s: unsupported review verdict %q", ErrFlowStateMalformed, path, state.Verdict)
	}
	if state.ProvisionalVerdict != "" && state.ProvisionalVerdict != ReviewPass && state.ProvisionalVerdict != ReviewPassWithFindings && state.ProvisionalVerdict != ReviewFail {
		return fmt.Errorf("%w: %s: unsupported provisional review verdict %q", ErrFlowStateMalformed, path, state.ProvisionalVerdict)
	}
	if err := validateAttempt(state.ActiveAttempt, true); err != nil {
		return fmt.Errorf("%w: %s: review active attempt: %v", ErrFlowStateMalformed, path, err)
	}
	if err := validateAttempt(state.LastAttempt, false); err != nil {
		return fmt.Errorf("%w: %s: review last attempt: %v", ErrFlowStateMalformed, path, err)
	}
	if state.Resume != nil {
		resume := state.Resume
		if resume.AttemptID == "" || resume.InputFingerprint == "" || resume.Model == "" || resume.UpdatedAt.IsZero() || len(resume.Coverage) == 0 {
			return fmt.Errorf("%w: %s: invalid review resume state", ErrFlowStateMalformed, path)
		}
		if strings.ContainsAny(resume.AttemptID+resume.InputFingerprint+resume.Model, "\x00\r\n") {
			return fmt.Errorf("%w: %s: unsafe review resume state", ErrFlowStateMalformed, path)
		}
		if state.Status == ReviewRunning && (state.ActiveAttempt == nil || state.ActiveAttempt.ID != resume.AttemptID) {
			return fmt.Errorf("%w: %s: review resume attempt mismatch", ErrFlowStateMalformed, path)
		}
		seen := map[string]bool{}
		for _, checkpoint := range resume.Coverage {
			if checkpoint.CoverageID == "" || checkpoint.UpdatedAt.IsZero() || seen[checkpoint.CoverageID] || strings.ContainsAny(checkpoint.CoverageID+checkpoint.SessionID, "\x00\r\n") || len(checkpoint.SessionID) > 512 {
				return fmt.Errorf("%w: %s: invalid review resume checkpoint", ErrFlowStateMalformed, path)
			}
			seen[checkpoint.CoverageID] = true
			switch checkpoint.Status {
			case AttemptPending, AttemptRunning, AttemptCompleted, AttemptFailed:
			default:
				return fmt.Errorf("%w: %s: invalid review resume checkpoint status %q", ErrFlowStateMalformed, path, checkpoint.Status)
			}
			if checkpoint.Status == AttemptCompleted {
				if checkpoint.Result == nil || checkpoint.Result.CoverageID != checkpoint.CoverageID || checkpoint.Result.Error != "" {
					return fmt.Errorf("%w: %s: invalid completed review resume checkpoint", ErrFlowStateMalformed, path)
				}
			} else if checkpoint.Result != nil {
				return fmt.Errorf("%w: %s: incomplete review resume checkpoint contains a result", ErrFlowStateMalformed, path)
			}
		}
	}
	if state.LastComplete != nil {
		if state.LastComplete.Artifact != ArtifactRelPath(sp, StageReview) || state.LastComplete.CompletedAt.IsZero() || state.LastComplete.InputFingerprint == "" || state.LastComplete.ArtifactDigest == "" {
			return fmt.Errorf("%w: %s: invalid review lastComplete", ErrFlowStateMalformed, path)
		}
	}
	return nil
}

func (s Service) saveReviewState(projectRef, sprintRef string, r ReviewResult, completed, total int) error {
	sp, _, _, err := s.resolveSprintInputs(projectRef, sprintRef)
	if err != nil {
		return err
	}
	state, err := LoadFlowState(s.root, sp)
	if err != nil {
		if !errors.Is(err, ErrFlowStateMissing) {
			return err
		}
		snap, e := s.store.ReadArtifacts(sp)
		if e != nil {
			return e
		}
		state = NewFlowState(sp, DeriveStages(sp, snap, nil), s.now())
	}
	now := s.now().UTC()
	current := state.Review
	if current == nil {
		current = &ReviewStageState{Path: ArtifactRelPath(sp, StageReview)}
	}
	if r.Restarted && r.Status == ReviewRunning && completed == 0 && len(r.Coverage) == 0 {
		current.ActiveAttempt = nil
		current.Resume = nil
	}
	if len(r.Focused) > 0 && r.Status == ReviewRunning && completed == 0 && len(r.Coverage) == 0 {
		current.ActiveAttempt = nil
		current.Resume = nil
	}
	current.Status, current.LastRunAt, current.Completed, current.Total, current.Diagnostics = r.Status, &now, completed, total, append([]ReviewDiagnostic(nil), r.Diagnostics...)
	current.ProvisionalVerdict = r.ProvisionalVerdict
	attempt := VerificationAttempt{ID: fmt.Sprintf("review-%d", now.UnixNano()), Status: AttemptRunning, StartedAt: now, HeartbeatAt: now, OwnerPID: os.Getpid()}
	if current.ActiveAttempt != nil {
		attempt = *current.ActiveAttempt
	}
	if r.Status == ReviewRunning {
		attempt.HeartbeatAt = now
		attempt.OwnerPID = os.Getpid()
		current.ActiveAttempt = &attempt
		if current.Resume != nil && !r.Restarted {
			current.Resume.AttemptID = attempt.ID
		}
	} else {
		attempt.CompletedAt = &now
		attempt.Diagnostics = reviewDiagnosticStrings(r.Diagnostics)
		attempt.Status = reviewAttemptStatus(r.Status)
		current.ActiveAttempt, current.LastAttempt = nil, &attempt
	}
	if r.Status == ReviewCompleted {
		current.ProvisionalVerdict = ""
		digest, _ := hashFile(mustArtifactPath(s.root, sp, StageReview))
		current.Verdict, current.Fingerprint, current.ArtifactDigest, current.Stale = r.Verdict, r.Fingerprint, digest, false
		current.LastComplete = &ReviewCompletion{Verdict: r.Verdict, Artifact: ArtifactRelPath(sp, StageReview), ArtifactDigest: digest, InputFingerprint: r.Fingerprint, CompletedAt: now, Coverage: append([]ReviewCoverageResult(nil), r.Coverage...)}
		current.Resume = nil
	} else if current.LastComplete != nil {
		current.Verdict, current.Fingerprint, current.ArtifactDigest = current.LastComplete.Verdict, current.LastComplete.InputFingerprint, current.LastComplete.ArtifactDigest
	} else {
		current.Verdict, current.Fingerprint = r.Verdict, r.Fingerprint
	}
	state.Review = current
	return SaveFlowState(s.root, sp, state)
}

func reviewDiagnosticStrings(values []ReviewDiagnostic) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, safeError(errors.New(value.Code+": "+value.Message)))
	}
	return out
}

func reviewAttemptStatus(status ReviewExecutionStatus) AttemptStatus {
	switch status {
	case ReviewCompleted:
		return AttemptCompleted
	case ReviewCancelled:
		return AttemptCancelled
	case ReviewBlocked:
		return AttemptBlocked
	default:
		return AttemptFailed
	}
}

func (s Service) reviewModelSelection(override string) ExecuteModelSelection {
	if strings.TrimSpace(override) != "" {
		return ExecuteModelSelection{Model: override, Source: "command override"}
	}
	if rt, ok := s.stageRuntime[StageReview]; ok && strings.TrimSpace(rt.Model) != "" {
		return ExecuteModelSelection{Model: rt.Model, Source: "planning.review_model"}
	}
	sel := s.executeModelSelection("")
	if rt, ok := s.stageRuntime[StagePlan]; ok && strings.TrimSpace(rt.Model) != "" {
		sel.Source = "planning.plan_model"
	}
	return sel
}

func reviewInput(id, kind, name, path, data string) ReviewInput {
	sum := sha256.Sum256([]byte(data))
	return ReviewInput{ID: id, Kind: kind, Name: name, Path: path, Hash: hex.EncodeToString(sum[:])}
}
func findReviewInput(in []ReviewInput, id string) ReviewInput {
	for _, v := range in {
		if v.ID == id {
			return v
		}
	}
	return ReviewInput{}
}
func catalogEntry(c project.ProjectIndex, section project.CatalogSection, s SelectedItem) (project.CatalogEntry, bool) {
	var matches []project.CatalogEntry
	for _, e := range c.Entries {
		if e.Section == section && strings.EqualFold(e.Name, s.Name) && (s.Path == "" || e.Path == s.Path) {
			matches = append(matches, e)
		}
	}
	return func() (project.CatalogEntry, bool) {
		if len(matches) == 1 {
			return matches[0], true
		}
		return project.CatalogEntry{}, false
	}()
}
func slugReviewID(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	dash := false
	for _, r := range s {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
			dash = false
		} else if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}
	return strings.Trim(b.String(), "-")
}
func fingerprintReviewManifest(m ReviewManifest) string {
	h := sha256.New()
	fmt.Fprintf(h, "project=%s\nsprint=%s\ntarget=%s\n", m.Project, m.Sprint, m.Target)
	for _, i := range m.Inputs {
		fmt.Fprintf(h, "%s\x00%s\x00%s\n", i.Path, i.ID, i.Hash)
	}
	for _, p := range m.ChangedPaths {
		fmt.Fprintf(h, "changed=%s\n", p)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// reviewRelevantProjectIndexContent removes smoke-only configuration from the
// frozen review view while preserving line numbers. Smoke owns validation of
// its harness catalog; moving that harness must not invalidate implementation
// review evidence or restart every reviewer.
func reviewRelevantProjectIndexContent(content string) string {
	lines := strings.Split(content, "\n")
	inSmokeHarnesses := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.EqualFold(trimmed, "## Smoke Harnesses") {
			inSmokeHarnesses = true
			lines[i] = ""
			continue
		}
		if inSmokeHarnesses {
			if strings.HasPrefix(trimmed, "## ") {
				inSmokeHarnesses = false
			} else {
				lines[i] = ""
				continue
			}
		}
		plain := strings.ToLower(strings.ReplaceAll(trimmed, "**", ""))
		if strings.HasPrefix(plain, "- smoke harness directory:") {
			lines[i] = ""
		}
	}
	return strings.Join(lines, "\n")
}

func reviewChangedPaths(data []byte) []string {
	var raw struct {
		Files []string `json:"files"`
		Tasks []struct {
			Evidence []struct {
				Path string `json:"path"`
			} `json:"evidence"`
		} `json:"tasks"`
	}
	_ = json.Unmarshal(data, &raw)
	set := map[string]bool{}
	for _, p := range raw.Files {
		if strings.TrimSpace(p) != "" {
			set[p] = true
		}
	}
	for _, t := range raw.Tasks {
		for _, e := range t.Evidence {
			if strings.TrimSpace(e.Path) != "" {
				set[e.Path] = true
			}
		}
	}
	var out []string
	for p := range set {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

func excludeGovernedReviewPaths(paths []string, inputs []ReviewInput) []string {
	governed := make(map[string]bool, len(inputs))
	for _, input := range inputs {
		if input.Kind == "target" || strings.TrimSpace(input.Path) == "" {
			continue
		}
		governed[filepath.ToSlash(filepath.Clean(input.Path))] = true
	}
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		if !governed[filepath.ToSlash(filepath.Clean(path))] {
			out = append(out, path)
		}
	}
	return out
}

func reviewRunStateIncomplete(data []byte) bool {
	var raw struct {
		Status string `json:"status"`
		Tasks  []struct {
			Status string `json:"status"`
		} `json:"tasks"`
	}
	if json.Unmarshal(data, &raw) != nil {
		return true
	}
	if raw.Status != "" && raw.Status != "complete" && raw.Status != "completed" && raw.Status != "success" {
		return true
	}
	for _, t := range raw.Tasks {
		if t.Status != "complete" && t.Status != "deferred" {
			return true
		}
	}
	return false
}

func unresolvedExecutePlanTasks(plan, runState string, manifest PlanManifest) bool {
	tasks, findings := extractExecutePlanTasks(plan, manifest, true)
	if len(findings) > 0 {
		return true
	}
	var raw struct {
		Tasks []struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"tasks"`
	}
	if json.Unmarshal([]byte(runState), &raw) != nil {
		return true
	}
	status := make(map[string]string, len(raw.Tasks))
	for _, task := range raw.Tasks {
		status[task.ID] = task.Status
	}
	for _, task := range tasks {
		if !task.Checked && status[task.ID] != string(ExecuteTaskDeferred) {
			return true
		}
	}
	return false
}

func reviewVerificationCommands(plan string) []string {
	set := map[string]bool{}
	for _, line := range strings.Split(plan, "\n") {
		rest := line
		for {
			start := strings.Index(rest, "`")
			if start < 0 {
				break
			}
			rest = rest[start+1:]
			end := strings.Index(rest, "`")
			if end < 0 {
				break
			}
			v := strings.TrimSpace(rest[:end])
			rest = rest[end+1:]
			if strings.HasPrefix(v, "go test ") || strings.HasPrefix(v, "go build ") {
				set[v] = true
			}
		}
	}
	var out []string
	for v := range set {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}
func renderReviewPreview(m ReviewManifest) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Review Stage Preview\n\nProject: `%s`\nSprint: `%s`\nFingerprint: `%s`\nTarget: `%s`\nModel: `%s` (%s)\nConcurrency: %d\nPermitted writes: sprint-root `review.md` and review fields in `flow-state.json` only.\n\nReviewers:\n", m.Project, m.Sprint, m.Fingerprint, m.Target, m.Model, m.ModelSource, m.Concurrency)
	for _, c := range m.Coverage {
		fmt.Fprintf(&b, "- `%s` %s: %s (`%s`)\n", c.ID, c.Kind, c.Name, c.Path)
	}
	fmt.Fprintln(&b, "\nSelected review protocols:")
	for _, in := range m.Inputs {
		if in.Kind == "protocol" {
			fmt.Fprintf(&b, "- %s (`%s`)\n", in.Name, in.Path)
		}
	}
	return b.String()
}

const reviewPromptMaxBytes = maxSharedPromptPrefixBytes + sharedPromptSuffixReserve

func renderReviewerPrompt(m ReviewManifest, c ReviewInput) string {
	var b strings.Builder
	b.WriteString(strings.TrimSpace(m.PromptTemplate))
	b.WriteString("\n\n")
	fmt.Fprintf(&b, "# Read-only Sprint Reviewer\n\nReview coverage `%s` (%s: %s). Do not write files, mutate git, or run destructive commands. Read the coverage source and governed inputs from the exact paths below; their hashes form the frozen fingerprint and inputs are checked again before promotion. Review only those inputs and the target scope. Cite logical manifest paths, not absolute read paths. Return exactly one JSON object matching: {\"schemaVersion\":1,\"coverageId\":string,\"applicability\":\"direct|partial|not_triggered|explicitly_deferred\",\"summary\":string,\"findings\":[{\"id\":string,\"severity\":\"info|low|medium|high|blocker\",\"applicability\":\"direct|partial|not_triggered|explicitly_deferred\",\"title\":string,\"detail\":string,\"action\":string,\"citations\":[{\"path\":string,\"startLine\":number,\"endLine\":number}]}]}. Every direct or partial finding requires real line citations. Findings are only for actionable deviations; summarize confirmed conformance in `summary` instead of emitting informational findings. Use stable unique finding IDs and emit no more than %d findings.\n\nTarget: %s\nFingerprint: %s\nChanged paths: %s\n\nCoverage source: logical `%s`; read `%s`; sha256 `%s`.\n\nFrozen input index:\n", c.ID, c.Kind, c.Name, maxReviewFindingsPerCoverage, m.Target, m.Fingerprint, strings.Join(m.ChangedPaths, ", "), c.Path, reviewInputReadPath(m, c), c.Hash)
	for _, in := range reviewerInputPacket(m, c) {
		fmt.Fprintf(&b, "- logical `%s`; kind `%s`; read `%s`; sha256 `%s`\n", in.Path, in.Kind, reviewInputReadPath(m, in), in.Hash)
	}
	fmt.Fprintln(&b, "\nThe review prompt asset is already applied above. Assets marked `<embedded>` are consumed by the deterministic review orchestrator and do not require a file read.")
	var direct []directPromptInput
	for _, input := range reviewerInputPacket(m, c) {
		if input.ID == "requirements" || input.ID == "code-context" || input.Path == "target/.identity" {
			continue
		}
		content, ok := m.Contents[input.Path]
		if !ok || strings.TrimSpace(content) == "" {
			direct = append(direct, directPromptInput{ID: input.ID, Kind: input.Kind, Path: input.Path, Missing: "captured content unavailable"})
			continue
		}
		direct = append(direct, directContentInput(input.ID, input.Kind, input.Path, content))
	}
	return appendDirectInputPacket(b.String(), direct, sharedPromptSuffixReserve)
}

func reviewerInputPacket(m ReviewManifest, coverage ReviewInput) []ReviewInput {
	packet := make([]ReviewInput, 0, len(m.Inputs))
	for _, input := range m.Inputs {
		// Independent coverage agents receive their own contract or handbook,
		// never sibling coverage sources. Common governed planning inputs,
		// protocols, execution evidence, and changed target files remain shared.
		if (input.Kind == "contract" || input.Kind == "handbook") && input.Path != coverage.Path {
			continue
		}
		packet = append(packet, input)
	}
	return packet
}

func reviewInputReadPath(m ReviewManifest, in ReviewInput) string {
	if in.Kind == "asset" || in.Path == "target/.identity" {
		return "<embedded>"
	}
	if m.ReviewerRoot != "" {
		if strings.HasPrefix(in.Path, "target/") {
			return filepath.Join(m.ReviewerRoot, "target", filepath.FromSlash(strings.TrimPrefix(in.Path, "target/")))
		}
		return filepath.Join(m.ReviewerRoot, "workspace", filepath.FromSlash(in.Path))
	}
	if strings.HasPrefix(in.Path, "target/") {
		return filepath.Join(m.Target, filepath.FromSlash(strings.TrimPrefix(in.Path, "target/")))
	}
	return filepath.Join(m.WorkspaceRoot, filepath.FromSlash(in.Path))
}

func prepareReviewSnapshot(m ReviewManifest) (string, error) {
	if m.Fingerprint == "" || strings.ContainsAny(m.Fingerprint, "\\/\x00\r\n") {
		return "", fmt.Errorf("create frozen review snapshot: invalid fingerprint")
	}
	root := filepath.Join(m.WorkspaceRoot, ".ultra", "cache", "review", m.Project, m.Sprint, m.Fingerprint)
	marker := filepath.Join(root, ".complete")
	if _, err := os.Stat(marker); err == nil {
		return root, nil
	}
	if err := os.RemoveAll(root); err != nil {
		return "", fmt.Errorf("reset incomplete frozen review snapshot: %w", err)
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", fmt.Errorf("create frozen review snapshot: %w", err)
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(root)
		}
	}()
	for _, in := range m.Inputs {
		if in.Kind == "asset" || in.Path == "target/.identity" {
			continue
		}
		content, ok := m.Contents[in.Path]
		if !ok {
			return "", fmt.Errorf("frozen review input %q has no captured content", in.Path)
		}
		copyManifest := m
		copyManifest.ReviewerRoot = root
		path := reviewInputReadPath(copyManifest, in)
		if !inside(root, path) {
			return "", fmt.Errorf("frozen review input %q escapes snapshot", in.Path)
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return "", fmt.Errorf("create frozen review snapshot directory: %w", err)
		}
		if err := os.WriteFile(path, []byte(content), 0o400); err != nil {
			return "", fmt.Errorf("write frozen review input %q: %w", in.Path, err)
		}
	}
	if err := os.WriteFile(marker, []byte(m.Fingerprint+"\n"), 0o400); err != nil {
		return "", fmt.Errorf("complete frozen review snapshot: %w", err)
	}
	cleanup = false
	return root, nil
}
func extractReviewResult(r pruntime.Result, out *ReviewCoverageResult) bool {
	if extractReviewValue(r.TerminalOutput, out) {
		return true
	}
	for i := len(r.Events) - 1; i >= 0; i-- {
		if extractReviewValue(r.Events[i].Payload, out) {
			return true
		}
	}
	return false
}
func extractReviewValue(v any, out *ReviewCoverageResult) bool {
	switch x := v.(type) {
	case map[string]any:
		var candidate ReviewCoverageResult
		if raw, e := json.Marshal(x); e == nil && json.Unmarshal(raw, &candidate) == nil && candidate.CoverageID != "" {
			*out = candidate
			return true
		}
		for _, k := range []string{"review_result", "structured_output", "output", "content", "text", "message", "part"} {
			if y, ok := x[k]; ok && extractReviewValue(y, out) {
				return true
			}
		}
	case []any:
		for i := len(x) - 1; i >= 0; i-- {
			if extractReviewValue(x[i], out) {
				return true
			}
		}
	case string:
		return extractReviewJSON(x, out)
	}
	return false
}

// extractReviewJSON tolerates runtime output that contains reasoning, Markdown,
// or other prose before the canonical result. Trying each object boundary with
// a streaming decoder avoids the brittle first-"{"/last-"}" assumption while
// still accepting only an object that decodes as an actual review result.
func extractReviewJSON(value string, out *ReviewCoverageResult) bool {
	found := false
	for offset := 0; offset < len(value); {
		relative := strings.IndexByte(value[offset:], '{')
		if relative < 0 {
			break
		}
		start := offset + relative
		var candidate ReviewCoverageResult
		if err := json.NewDecoder(strings.NewReader(value[start:])).Decode(&candidate); err == nil && candidate.CoverageID != "" {
			*out = candidate
			found = true
		}
		offset = start + 1
	}
	return found
}
func validReviewCitation(root string, m ReviewManifest, c ReviewCitation) bool {
	if c.StartLine < 1 || c.EndLine < c.StartLine {
		return false
	}
	data, ok := m.Contents[c.Path]
	if !ok {
		return false
	}
	return c.EndLine <= len(strings.Split(data, "\n"))
}

type reviewWriteHooks struct{ BeforeRename func(string) error }

func atomicWriteReview(path string, data []byte) error {
	return atomicWriteReviewWithHooks(path, data, reviewWriteHooks{})
}
func atomicWriteReviewWithHooks(path string, data []byte, hooks reviewWriteHooks) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".review.*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if hooks.BeforeRename != nil {
		if err := hooks.BeforeRename(path); err != nil {
			return err
		}
	}
	if err = os.Rename(tmp, path); err != nil {
		return err
	}
	syncDir(filepath.Dir(path))
	return nil
}
func reviewArtifact(m ReviewManifest) string {
	return filepath.ToSlash(filepath.Join(m.SprintRoot, "review.md"))
}
func appendUnique(values []string, v string) []string {
	for _, x := range values {
		if x == v {
			return values
		}
	}
	return append(values, v)
}

func safeReviewText(root, value string) string {
	return strings.ReplaceAll(safeExecuteText("review.diagnostic", value), root, ".")
}

func validReviewApplicability(v string) bool {
	return v == "direct" || v == "partial" || v == "not_triggered" || v == "explicitly_deferred" || v == "deferred"
}
func reviewApplicable(v string) bool { return v == "direct" || v == "partial" }

func loadReviewAsset(root, rel string, required []string) (string, error) {
	full, err := workspace.ResolveInside(root, rel)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(full)
	if err != nil {
		if !os.IsNotExist(err) {
			return "", err
		}
		builtin, ok := workspace.DefaultOverrideFile(rel)
		if !ok {
			return "", fmt.Errorf("embedded default is missing")
		}
		data = []byte(builtin)
	}
	content := string(data)
	if strings.TrimSpace(content) == "" || containsPlaceholder(content) {
		return "", fmt.Errorf("asset is empty or contains placeholder content")
	}
	for _, text := range required {
		if !strings.Contains(content, text) {
			return "", fmt.Errorf("asset is missing %q", text)
		}
	}
	return content, nil
}
func atoiReview(s string) int { n, _ := strconv.Atoi(s); return n }

var _ = errors.Is
