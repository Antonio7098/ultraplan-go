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
	Status      ReviewExecutionStatus `json:"status"`
	Verdict     ReviewVerdict         `json:"verdict,omitempty"`
	Path        string                `json:"path"`
	LastRunAt   *time.Time            `json:"lastRunAt,omitempty"`
	Fingerprint string                `json:"fingerprint,omitempty"`
	Stale       bool                  `json:"stale"`
	Completed   int                   `json:"completed"`
	Total       int                   `json:"total"`
	Diagnostics []ReviewDiagnostic    `json:"diagnostics,omitempty"`
}

type ReviewInput struct {
	ID, Kind, Name, Path, Hash string
}

type ReviewManifest struct {
	Project, Sprint, SprintRoot, Target, Fingerprint string
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
	ModelOverride      string
	Concurrency        int
	Progress           func(ReviewProgress)
}

type ReviewProgress struct {
	Completed, Total    int
	CoverageID, Message string
}

type ReviewResult struct {
	Project     string                 `json:"project"`
	Sprint      string                 `json:"sprint"`
	Prompt      string                 `json:"prompt,omitempty"`
	Artifact    string                 `json:"artifact,omitempty"`
	Fingerprint string                 `json:"fingerprint,omitempty"`
	Message     string                 `json:"message,omitempty"`
	DryRun      bool                   `json:"dry_run"`
	Status      ReviewExecutionStatus  `json:"execution_status"`
	Verdict     ReviewVerdict          `json:"verdict,omitempty"`
	Coverage    []ReviewCoverageResult `json:"coverage,omitempty"`
	Findings    []ReviewFinding        `json:"findings,omitempty"`
	Diagnostics []ReviewDiagnostic     `json:"diagnostics,omitempty"`
}

func (s Service) PrepareReview(projectRef, sprintRef string, req ReviewRequest) (ReviewManifest, []ValidationFinding, error) {
	sp, inputs, catalog, err := s.resolveSprintInputs(projectRef, sprintRef)
	if err != nil {
		return ReviewManifest{}, nil, err
	}
	manifest := ReviewManifest{Project: sp.Project, Sprint: sp.Slug, SprintRoot: workspace.Rel(s.root, sp.Path), Contents: map[string]string{}}
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
	target, targetFindings := ResolveExecuteTarget(inputs.ProjectIndex)
	findings = append(findings, targetFindings...)
	manifest.Target = target.Path
	base := []struct {
		id, kind string
		stage    PlanningStage
	}{
		{"requirements", "governed", StageRequirements}, {"sprint-index", "governed", StageSprintIndex},
		{"technical-handbook", "handbook", StageTechnicalHandbook}, {"reasoning", "governed", StageReasoning},
		{"plan", "governed", StagePlan}, {"execute", "governed", StageExecute},
	}
	for _, item := range base {
		data, readErr := s.store.ReadArtifact(sp, item.stage)
		path := ArtifactRelPath(sp, item.stage)
		if readErr != nil || strings.TrimSpace(data) == "" {
			findings = append(findings, finding("Review prerequisites", item.id, path, "missing review input", safeError(readErr), "Complete execute and all governed sprint artifacts before review."))
			continue
		}
		manifest.Contents[path] = data
		manifest.Inputs = append(manifest.Inputs, reviewInput(item.id, item.kind, item.id, path, data))
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
		manifest.ChangedPaths = reviewChangedPaths(data)
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
	// The handbook receives an independent reviewer even though it is also a governed input.
	if handbook := findReviewInput(manifest.Inputs, "technical-handbook"); handbook.Path != "" {
		handbook.ID, handbook.Kind, handbook.Name = "handbook", "handbook", "Technical Handbook"
		manifest.Coverage = append(manifest.Coverage, handbook)
	}
	if strings.TrimSpace(manifest.Model) == "" || manifest.Model == "unresolved" {
		findings = append(findings, finding("Configuration", "review model", "", "missing review model", "no review, plan, or runtime model is configured", "Set planning.review_model or planning.plan_model."))
	}
	if plan := manifest.Contents[ArtifactRelPath(sp, StagePlan)]; strings.Contains(plan, "- [ ] Task ") || strings.Contains(plan, "- [ ] **Task ") {
		findings = append(findings, finding("Plan Execution", "tasks", ArtifactRelPath(sp, StagePlan), "plan tasks are not complete", "one or more top-level plan tasks remain unchecked", "Complete execute evidence and mark implemented tasks complete before review."))
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
	return PromptPreview{Project: m.Project, Sprint: m.Sprint, Prompt: renderReviewPreview(m)}, nil
}

func (s Service) Review(ctx context.Context, projectRef, sprintRef string, req ReviewRequest) (ReviewResult, error) {
	m, findings, err := s.PrepareReview(projectRef, sprintRef, req)
	result := ReviewResult{Project: m.Project, Sprint: m.Sprint, DryRun: req.DryRun, Fingerprint: m.Fingerprint, Artifact: reviewArtifact(m), Status: ReviewReady}
	if err != nil {
		return result, err
	}
	if len(findings) > 0 {
		result.Status, result.Verdict = ReviewBlocked, ReviewVerdictBlocked
		for _, f := range findings {
			result.Diagnostics = append(result.Diagnostics, ReviewDiagnostic{Code: "preflight", Message: safeReviewText(s.root, f.Problem+": "+f.Cause)})
		}
		if !req.DryRun {
			s.saveReviewState(projectRef, sprintRef, result, 0, len(m.Coverage))
		}
		return result, fmt.Errorf("review prerequisites failed validation")
	}
	result.Prompt = renderReviewPreview(m)
	if req.DryRun || req.PromptOnly {
		result.Message = "review dry run"
		return result, nil
	}
	if s.runtime == nil {
		return result, fmt.Errorf("runtime is required for review")
	}
	result.Status = ReviewRunning
	if err := s.saveReviewState(projectRef, sprintRef, result, 0, len(m.Coverage)); err != nil {
		return result, err
	}
	workers := m.Concurrency
	if workers > len(m.Coverage) {
		workers = len(m.Coverage)
	}
	if workers < 1 {
		workers = 1
	}
	type item struct {
		index int
		value ReviewCoverageResult
	}
	jobs := make(chan int)
	done := make(chan item, len(m.Coverage))
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				c := m.Coverage[i]
				done <- item{i, s.runReviewer(ctx, m, c)}
			}
		}()
	}
	go func() {
		for i := range m.Coverage {
			jobs <- i
		}
		close(jobs)
		wg.Wait()
		close(done)
	}()
	coverage := make([]ReviewCoverageResult, len(m.Coverage))
	completed := 0
	for got := range done {
		coverage[got.index] = got.value
		completed++
		if req.Progress != nil {
			req.Progress(ReviewProgress{Completed: completed, Total: len(coverage), CoverageID: got.value.CoverageID, Message: "reviewer complete"})
		}
		result.Coverage = coverage
		_ = s.saveReviewState(projectRef, sprintRef, result, completed, len(coverage))
	}
	result.Coverage = coverage
	result.Findings, result.Diagnostics, result.Verdict = validateReviewCoverage(s.root, m, coverage)
	if ctx.Err() != nil {
		result.Status = ReviewCancelled
		result.Diagnostics = append(result.Diagnostics, ReviewDiagnostic{Code: "cancelled", Message: ctx.Err().Error()})
		s.saveReviewState(projectRef, sprintRef, result, completed, len(coverage))
		return result, ctx.Err()
	}
	if result.Verdict == ReviewVerdictBlocked {
		result.Status = ReviewFailed
		s.saveReviewState(projectRef, sprintRef, result, completed, len(coverage))
		return result, fmt.Errorf("review failed to produce complete valid coverage")
	}
	current, currentFindings, currentErr := s.PrepareReview(projectRef, sprintRef, req)
	if currentErr != nil || len(currentFindings) > 0 || current.Fingerprint != m.Fingerprint {
		result.Status = ReviewFailed
		result.Diagnostics = append(result.Diagnostics, ReviewDiagnostic{Code: "inputs-changed", Message: "review inputs changed during execution"})
		s.saveReviewState(projectRef, sprintRef, result, completed, len(coverage))
		return result, fmt.Errorf("review inputs changed during execution")
	}
	result.Status = ReviewCompleted
	content := RenderReviewMarkdown(m, result)
	if vf := ValidateReviewContent(content, m); len(vf) > 0 {
		result.Status = ReviewFailed
		result.Diagnostics = append(result.Diagnostics, ReviewDiagnostic{Code: "artifact-invalid", Message: vf[0].Problem})
		s.saveReviewState(projectRef, sprintRef, result, completed, len(coverage))
		return result, fmt.Errorf("generated review.md failed validation")
	}
	sp, _, _, _ := s.resolveSprintInputs(projectRef, sprintRef)
	path, _ := ArtifactPath(s.root, sp, StageReview)
	if err := atomicWriteReview(path, []byte(content)); err != nil {
		result.Status = ReviewFailed
		result.Diagnostics = append(result.Diagnostics, ReviewDiagnostic{Code: "write-failed", Message: safeError(err)})
		s.saveReviewState(projectRef, sprintRef, result, completed, len(coverage))
		return result, err
	}
	now := s.now().UTC()
	result.Message = "review complete"
	result.Artifact = ArtifactRelPath(sp, StageReview)
	if err := s.saveReviewState(projectRef, sprintRef, result, completed, len(coverage)); err != nil {
		return result, err
	}
	_ = now
	if result.Verdict == ReviewFail {
		return result, fmt.Errorf("review completed with failing verdict")
	}
	return result, nil
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

func (s Service) runReviewer(ctx context.Context, m ReviewManifest, c ReviewInput) (out ReviewCoverageResult) {
	out.CoverageID = c.ID
	defer func() {
		if r := recover(); r != nil {
			out.Error = fmt.Sprintf("reviewer panic: %v", r)
		}
	}()
	req := s.runtimeRequest(renderReviewerPrompt(m, c), map[string]string{"project": m.Project, "sprint": m.Sprint, "stage": string(StageReview), "coverage": c.ID, "model_source": m.ModelSource})
	req.WorkDir = m.Target
	req.Model = strings.TrimPrefix(m.Model, req.Provider+"/")
	req.Sandbox = "read_only"
	req.Permissions = "restricted"
	req.RequireCaps = appendUnique(req.RequireCaps, "permissions")
	req.Policy = pruntime.PermissionPolicy{Default: "deny", Tools: map[string]string{"read": "allow", "list": "allow", "search": "allow"}}
	r, err := s.runtime.StartRun(ctx, req)
	if err != nil {
		out.Error = safeReviewText(s.root, err.Error())
		return
	}
	if r.Permissions.UnsupportedCount > 0 {
		out.Error = "runtime could not enforce review permission policy"
		return
	}
	if !extractReviewResult(r, &out) {
		out.Error = "runtime did not return a structured review result"
	}
	if out.CoverageID == "" {
		out.CoverageID = c.ID
	}
	return
}

func validateReviewCoverage(root string, m ReviewManifest, results []ReviewCoverageResult) ([]ReviewFinding, []ReviewDiagnostic, ReviewVerdict) {
	var findings []ReviewFinding
	var diagnostics []ReviewDiagnostic
	coverage := map[string]bool{}
	for _, r := range results {
		coverage[r.CoverageID] = true
		if r.Error != "" {
			diagnostics = append(diagnostics, ReviewDiagnostic{Code: "reviewer-failed", CoverageID: r.CoverageID, Message: r.Error})
			continue
		}
		if r.SchemaVersion != 1 {
			diagnostics = append(diagnostics, ReviewDiagnostic{Code: "unsupported-schema", CoverageID: r.CoverageID, Message: "reviewer result schemaVersion must be 1"})
		}
		if !validReviewApplicability(r.Applicability) {
			diagnostics = append(diagnostics, ReviewDiagnostic{Code: "invalid-applicability", CoverageID: r.CoverageID, Message: "applicability must be applicable or not_applicable"})
		}
		for _, f := range r.Findings {
			valid := true
			if f.Severity != "info" && f.Severity != "low" && f.Severity != "medium" && f.Severity != "high" && f.Severity != "blocker" {
				valid = false
			}
			if !validReviewApplicability(f.Applicability) {
				valid = false
			}
			if len(f.Citations) == 0 && reviewApplicable(f.Applicability) {
				valid = false
			}
			for _, c := range f.Citations {
				if !validReviewCitation(root, m, c) {
					valid = false
				}
			}
			if valid {
				findings = append(findings, f)
			} else {
				diagnostics = append(diagnostics, ReviewDiagnostic{Code: "invalid-finding", CoverageID: r.CoverageID, Message: "finding failed schema or citation validation"})
			}
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
	state.Review = &ReviewStageState{Status: r.Status, Verdict: r.Verdict, Path: ArtifactRelPath(sp, StageReview), LastRunAt: &now, Fingerprint: r.Fingerprint, Completed: completed, Total: total, Diagnostics: r.Diagnostics}
	return SaveFlowState(s.root, sp, state)
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
		if t.Status != "complete" {
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
func renderReviewerPrompt(m ReviewManifest, c ReviewInput) string {
	var b strings.Builder
	b.WriteString(strings.TrimSpace(m.PromptTemplate))
	b.WriteString("\n\n")
	fmt.Fprintf(&b, "# Read-only Sprint Reviewer\n\nReview coverage `%s` (%s: %s). Do not write files, mutate git, or run destructive commands. Review only the frozen inputs and target scope. Return exactly one JSON object matching: {\"schemaVersion\":1,\"coverageId\":string,\"applicability\":\"direct|partial|not_triggered|deferred\",\"summary\":string,\"findings\":[{\"id\":string,\"severity\":\"info|low|medium|high|blocker\",\"applicability\":\"direct|partial|not_triggered|deferred\",\"title\":string,\"detail\":string,\"action\":string,\"citations\":[{\"path\":string,\"startLine\":number,\"endLine\":number}]}]}. Every direct or partial finding requires real line citations.\n\nTarget: %s\nFingerprint: %s\nChanged paths: %s\n\nCoverage source (`%s`):\n%s\n\nGoverned sprint inputs:\n", c.ID, c.Kind, c.Name, m.Target, m.Fingerprint, strings.Join(m.ChangedPaths, ", "), c.Path, m.Contents[c.Path])
	for _, in := range m.Inputs {
		if in.Path == c.Path {
			continue
		}
		fmt.Fprintf(&b, "\n## %s\n%s\n", in.Path, m.Contents[in.Path])
	}
	return b.String()
}
func extractReviewResult(r pruntime.Result, out *ReviewCoverageResult) bool {
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
		start := strings.Index(x, "{")
		end := strings.LastIndex(x, "}")
		var candidate ReviewCoverageResult
		if start >= 0 && end > start && json.Unmarshal([]byte(x[start:end+1]), &candidate) == nil && candidate.CoverageID != "" {
			*out = candidate
			return true
		}
	}
	return false
}
func validReviewCitation(root string, m ReviewManifest, c ReviewCitation) bool {
	if c.StartLine < 1 || c.EndLine < c.StartLine {
		return false
	}
	data, ok := m.Contents[c.Path]
	if !ok {
		if filepath.IsAbs(c.Path) && inside(m.Target, c.Path) {
			raw, e := os.ReadFile(c.Path)
			if e != nil {
				return false
			}
			data = string(raw)
			ok = true
		} else if m.Target != "" {
			full := filepath.Join(m.Target, filepath.FromSlash(c.Path))
			if inside(m.Target, full) {
				raw, e := os.ReadFile(full)
				if e == nil {
					data = string(raw)
					ok = true
				}
			}
		}
	}
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
	return v == "direct" || v == "partial" || v == "not_triggered" || v == "deferred"
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
