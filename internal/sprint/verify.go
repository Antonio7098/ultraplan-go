package sprint

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type VerifyRequest struct {
	To       PlanningStage
	DryRun   bool
	Review   ReviewRequest
	Smoke    SmokeRequest
	Progress func(FlowProgress)
}

type VerifyResult struct {
	Project      string             `json:"project"`
	Sprint       string             `json:"sprint"`
	To           PlanningStage      `json:"to"`
	DryRun       bool               `json:"dry_run"`
	ReviewResult *ReviewResult      `json:"review_result,omitempty"`
	SmokeResult  *SmokeResult       `json:"smoke_result,omitempty"`
	Verification VerificationStatus `json:"verification"`
}

// Verify is the sole review-to-smoke transition. It requires complete execute
// evidence, reuses current canonical review evidence, and applies the smoke gate.
func (s Service) Verify(ctx context.Context, projectRef, sprintRef string, req VerifyRequest) (VerifyResult, error) {
	if req.To == "" {
		req.To = StageSmoke
	}
	if req.To != StageReview && req.To != StageSmoke {
		return VerifyResult{}, fmt.Errorf("verify target must be review or smoke")
	}
	result := VerifyResult{Project: projectRef, Sprint: sprintRef, To: req.To, DryRun: req.DryRun}
	if err := s.requireCompleteExecute(projectRef, sprintRef); err != nil {
		return result, err
	}
	status, statusErr := s.VerificationStatus(projectRef, sprintRef)
	if statusErr != nil && !errors.Is(statusErr, ErrFlowStateMissing) {
		return result, statusErr
	}
	if statusErr == nil {
		result.Verification = status
	}
	currentReview := statusErr == nil && status.Review.Fresh && status.Review.ExecutionStatus == string(ReviewCompleted) && !req.Review.Restart
	if !currentReview {
		if req.Progress != nil {
			req.Progress(FlowProgress{Stage: StageReview, State: "running", Message: "obtaining current review evidence"})
		}
		reviewReq := req.Review
		reviewReq.DryRun = req.DryRun
		review, err := s.Review(ctx, projectRef, sprintRef, reviewReq)
		result.ReviewResult = &review
		allowDiagnosticContinuation := req.To == StageSmoke && review.Status == ReviewCompleted && review.Verdict == ReviewFail && req.Smoke.ForceReview && req.Smoke.OverrideConfirmed && strings.TrimSpace(req.Smoke.OverrideRationale) != ""
		if err != nil && !allowDiagnosticContinuation {
			return result, err
		}
	} else if req.Progress != nil {
		req.Progress(FlowProgress{Stage: StageReview, State: "skipped", Message: "current review evidence is acceptable"})
	}
	if req.To == StageReview || req.DryRun {
		if !req.DryRun {
			result.Verification, _ = s.VerificationStatus(projectRef, sprintRef)
		}
		return result, nil
	}
	if req.Progress != nil {
		req.Progress(FlowProgress{Stage: StageSmoke, State: "running", Message: "evaluating review gate and running smoke"})
	}
	smokeReq := req.Smoke
	smoke, err := s.RunSmoke(ctx, projectRef, sprintRef, smokeReq)
	result.SmokeResult = &smoke
	result.Verification, _ = s.VerificationStatus(projectRef, sprintRef)
	return result, err
}

func (s Service) requireCompleteExecute(projectRef, sprintRef string) error {
	sp, _, _, err := s.resolveSprintInputs(projectRef, sprintRef)
	if err != nil {
		return err
	}
	state, err := LoadExecuteRunState(s.root, sp)
	if err != nil {
		path, pathErr := ExecuteRunStatePath(s.root, sp)
		if pathErr == nil {
			var summary struct {
				Status string   `json:"status"`
				Files  []string `json:"files"`
			}
			if data, readErr := os.ReadFile(path); readErr == nil && json.Unmarshal(data, &summary) == nil && (summary.Status == "complete" || summary.Status == "completed") && len(summary.Files) > 0 {
				content, artifactErr := s.store.ReadArtifact(sp, StageExecute)
				if artifactErr == nil && strings.TrimSpace(content) != "" {
					return nil
				}
			}
		}
		return fmt.Errorf("execute evidence is incomplete: %w", err)
	}
	if len(state.Tasks) == 0 {
		return fmt.Errorf("execute evidence is incomplete: no planned tasks")
	}
	for _, task := range state.Tasks {
		if task.Status != ExecuteTaskComplete {
			return fmt.Errorf("execute evidence is incomplete: task %q is %s", task.Identity.Name, task.Status)
		}
	}
	data, err := s.store.ReadArtifact(sp, StageExecute)
	if err != nil || strings.TrimSpace(data) == "" {
		return fmt.Errorf("execute evidence is incomplete: execute.md is missing")
	}
	return nil
}

func (s Service) ExecuteComplete(projectRef, sprintRef string) (bool, error) {
	err := s.requireCompleteExecute(projectRef, sprintRef)
	if err != nil {
		return false, nil
	}
	return true, nil
}

func (s Service) VerificationStatus(projectRef, sprintRef string) (VerificationStatus, error) {
	sp, _, _, err := s.resolveSprintInputs(projectRef, sprintRef)
	if err != nil {
		return VerificationStatus{}, err
	}
	out := VerificationStatus{Project: sp.Project, Sprint: sp.Slug}
	out.Review = VerificationStage{Stage: StageReview, ExecutionStatus: "missing", Artifact: ArtifactRelPath(sp, StageReview), NextAction: "Run review."}
	out.Smoke = VerificationStage{Stage: StageSmoke, ExecutionStatus: "missing", Artifact: ArtifactRelPath(sp, StageSmoke), NextAction: "Run smoke after a current acceptable review."}
	state, err := LoadFlowState(s.root, sp)
	if err != nil {
		return out, err
	}
	// Status derives expired-attempt truth without mutating durable state.
	// The next explicit review/smoke operation owns any persisted transition.
	reconcileExpiredAttempts(&state, s.now().UTC())
	malformed := false
	if state.Review != nil {
		r := state.Review
		out.Review.ExecutionStatus, out.Review.Verdict = string(r.Status), string(r.Verdict)
		out.Review.InputFingerprint, out.Review.ArtifactDigest = r.Fingerprint, r.ArtifactDigest
		out.Review.ActiveAttempt, out.Review.LastAttempt = r.ActiveAttempt, r.LastAttempt
		if r.Resume != nil {
			out.Review.Total = len(r.Resume.Coverage)
			for _, checkpoint := range r.Resume.Coverage {
				if checkpoint.Status == AttemptCompleted {
					out.Review.Completed++
				}
				if checkpoint.SessionID != "" && checkpoint.Status != AttemptCompleted {
					out.Review.RetainedSessions++
				}
			}
			out.Review.Resumable = out.Review.Completed > 0
		}
		manifest, findings, prepareErr := s.PrepareReview(projectRef, sprintRef, ReviewRequest{})
		if prepareErr != nil || len(findings) > 0 {
			out.Review.FreshnessReasons = append(out.Review.FreshnessReasons, "governed review inputs are unavailable or invalid")
		} else if manifest.Fingerprint != r.Fingerprint {
			out.Review.FreshnessReasons = append(out.Review.FreshnessReasons, "review input fingerprint changed")
		}
		if r.Resume != nil && (prepareErr != nil || len(findings) > 0 || manifest.Fingerprint != r.Resume.InputFingerprint || manifest.Model != r.Resume.Model) {
			out.Review.Resumable = false
		}
		data, readErr := os.ReadFile(mustArtifactPath(s.root, sp, StageReview))
		if readErr != nil {
			out.Review.FreshnessReasons = append(out.Review.FreshnessReasons, "review artifact is missing")
		} else {
			if r.ArtifactDigest == "" || hashBytes(data) != r.ArtifactDigest {
				out.Review.FreshnessReasons = append(out.Review.FreshnessReasons, "review artifact digest changed")
			}
			if prepareErr == nil && len(findings) == 0 && len(ValidateReviewContent(string(data), manifest)) > 0 {
				malformed = true
				out.Review.FreshnessReasons = append(out.Review.FreshnessReasons, "review artifact is malformed")
			}
		}
		out.Review.Fresh = len(out.Review.FreshnessReasons) == 0 && r.LastComplete != nil
		if !out.Review.Fresh {
			if out.Review.Resumable {
				out.Review.NextAction = fmt.Sprintf("Resume review from %d/%d validated coverage (%d retained sessions), or restart it explicitly.", out.Review.Completed, out.Review.Total, out.Review.RetainedSessions)
			} else {
				out.Review.NextAction = "Rerun review using the current governed inputs."
			}
		} else {
			out.Review.NextAction = "Review is current."
		}
	}
	if state.Smoke != nil {
		sm := state.Smoke
		out.Smoke.ExecutionStatus, out.Smoke.Verdict = string(sm.Status), string(sm.Verdict)
		out.Smoke.InputFingerprint, out.Smoke.ArtifactDigest, out.Smoke.RunID = sm.InputFingerprint, sm.ArtifactDigest, sm.RunID
		out.Smoke.Issues, out.Smoke.Evidence, out.Smoke.Override = append([]SmokeIssue(nil), sm.Issues...), append([]EvidenceReference(nil), sm.Evidence...), sm.Override
		out.Smoke.ActiveAttempt, out.Smoke.LastAttempt = sm.ActiveAttempt, sm.LastAttempt
		if !out.Review.Fresh {
			out.Smoke.FreshnessReasons = append(out.Smoke.FreshnessReasons, "review evidence is not current")
		}
		data, readErr := os.ReadFile(mustArtifactPath(s.root, sp, StageSmoke))
		if readErr != nil {
			out.Smoke.FreshnessReasons = append(out.Smoke.FreshnessReasons, "smoke artifact is missing")
		} else {
			if sm.ArtifactDigest == "" || hashBytes(data) != sm.ArtifactDigest {
				out.Smoke.FreshnessReasons = append(out.Smoke.FreshnessReasons, "smoke artifact digest changed")
			}
			if len(ValidateSmokeContent(string(data))) > 0 {
				malformed = true
				out.Smoke.FreshnessReasons = append(out.Smoke.FreshnessReasons, "smoke artifact is malformed")
			}
		}
		if sm.LastComplete == nil || sm.InputFingerprint == "" {
			out.Smoke.FreshnessReasons = append(out.Smoke.FreshnessReasons, "smoke input fingerprint is unavailable")
		} else if fingerprint, identityErr := refreshEvidenceFingerprint(sm.LastComplete.Evidence); identityErr != nil || fingerprint != sm.InputFingerprint {
			out.Smoke.FreshnessReasons = append(out.Smoke.FreshnessReasons, "smoke inputs or external evidence changed")
		}
		out.Smoke.Fresh = len(out.Smoke.FreshnessReasons) == 0
		if !out.Smoke.Fresh {
			out.Smoke.NextAction = "Rerun the required containing smoke suite."
		} else {
			out.Smoke.NextAction = "Smoke evidence is current."
		}
	}
	out.Assessment, out.NextAction = deriveAssessment(out.Review, out.Smoke, malformed)
	return out, nil
}

func deriveAssessment(review, smoke VerificationStage, malformed bool) (OverallAssessment, string) {
	if malformed {
		return AssessmentBlocked, "Repair or regenerate malformed canonical verification evidence."
	}
	if review.Fresh && review.Verdict == string(ReviewFail) {
		return AssessmentFail, "Resolve review findings and rerun review; diagnostic smoke cannot change this result."
	}
	if review.Fresh && review.Verdict == string(ReviewVerdictBlocked) {
		return AssessmentBlocked, "Satisfy the review prerequisite and rerun review."
	}
	if !review.Fresh || review.ExecutionStatus != string(ReviewCompleted) || !smoke.Fresh || smoke.ExecutionStatus != string(SmokeCompleted) {
		if !review.Fresh || review.ExecutionStatus != string(ReviewCompleted) {
			return AssessmentIncomplete, "Run or retry review, the earliest non-current verification stage."
		}
		return AssessmentIncomplete, "Run or retry smoke, the earliest non-current verification stage."
	}
	switch SmokeVerdict(smoke.Verdict) {
	case SmokeFailVerdict:
		return AssessmentFail, "Resolve the classified failure and rerun the required containing suite."
	case SmokeBlockedVerdict:
		return AssessmentBlocked, "Restore smoke prerequisites, coverage, or evidence and rerun smoke."
	case SmokeNotApplicable:
		return AssessmentNotApplicable, "No runtime smoke is applicable; retain the current review rationale."
	case SmokePassWithOpenIssues:
		return AssessmentPassWithFindings, "Resolve linked relevant issues and rerun narrow plus containing scope."
	case SmokePass:
		if ReviewVerdict(review.Verdict) == ReviewPassWithFindings {
			return AssessmentPassWithFindings, "Address review findings if a clean assessment is required."
		}
		return AssessmentPass, "No verification action required."
	default:
		return AssessmentBlocked, "Repair the unsupported smoke verdict."
	}
}

func smokeInputFingerprint(p smokePrepared, result SmokeResult) string {
	refs := smokeIdentityReferences(p, result)
	fingerprint, _ := refreshEvidenceFingerprint(refs)
	return fingerprint
}

func smokeIdentityReferences(p smokePrepared, result SmokeResult) []EvidenceReference {
	refs := []EvidenceReference{
		{Kind: "file", Path: p.ManifestPath},
		{Kind: "target", Path: p.Target},
		{Kind: "file", Path: filepath.Join(p.Sprint.Path, "review.md")},
		{Kind: "fact", Path: "review-fingerprint", Digest: result.ReviewFingerprint},
		{Kind: "fact", Path: "review-verdict", Digest: string(result.ReviewVerdict)},
		{Kind: "fact", Path: "scope", Digest: result.ScopeKind + ":" + result.Scope},
		{Kind: "fact", Path: "harness", Digest: result.Harness + ":" + result.Protocol},
		{Kind: "fact", Path: "prerequisites", Digest: strings.Join(result.Prerequisites, "\x00")},
		{Kind: "fact", Path: "timeout", Digest: result.EffectiveTimeout.String() + ":" + result.TimeoutSource},
	}
	for _, evidence := range result.Evidence {
		path := evidence.Path
		if !filepath.IsAbs(path) {
			path = filepath.Join(p.HarnessRoot, filepath.FromSlash(path))
		}
		refs = append(refs, EvidenceReference{Kind: "file", Path: path})
	}
	for _, issue := range result.Issues {
		path := issue.Path
		if !filepath.IsAbs(path) {
			path = filepath.Join(p.HarnessRoot, filepath.FromSlash(path))
		}
		refs = append(refs, EvidenceReference{Kind: "file", Path: path}, EvidenceReference{Kind: "fact", Path: "issue:" + issue.ID, Digest: issue.Status})
	}
	sort.Slice(refs, func(i, j int) bool {
		if refs[i].Path == refs[j].Path {
			return refs[i].Kind < refs[j].Kind
		}
		return refs[i].Path < refs[j].Path
	})
	return refs
}

func refreshEvidenceFingerprint(refs []EvidenceReference) (string, error) {
	h := sha256.New()
	for _, ref := range refs {
		digest := ref.Digest
		var err error
		switch ref.Kind {
		case "file":
			digest, err = hashFile(ref.Path)
		case "target":
			digest, err = targetIdentity(ref.Path)
		}
		if err != nil {
			return "", err
		}
		fmt.Fprintf(h, "%s\x00%s\x00%s\n", ref.Kind, filepath.ToSlash(ref.Path), digest)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func targetIdentity(root string) (string, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	paths := []string{}
	head := "non-git"
	gitRootOK := false
	if output, gitErr := exec.Command("git", "-C", root, "rev-parse", "--show-toplevel").Output(); gitErr == nil {
		gitRoot, absErr := filepath.Abs(strings.TrimSpace(string(output)))
		gitRootOK = absErr == nil && filepath.Clean(gitRoot) == filepath.Clean(root)
	}
	if output, gitErr := exec.Command("git", "-C", root, "rev-parse", "HEAD").Output(); gitErr == nil && gitRootOK {
		head = strings.TrimSpace(string(output))
		if output, listErr := exec.Command("git", "-C", root, "ls-files", "-co", "--exclude-standard", "-z").Output(); listErr == nil {
			for _, value := range strings.Split(string(output), "\x00") {
				if value != "" {
					paths = append(paths, filepath.ToSlash(filepath.Clean(value)))
				}
			}
		}
	}
	if len(paths) == 0 {
		err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			rel, relErr := filepath.Rel(root, path)
			if relErr != nil {
				return relErr
			}
			if entry.IsDir() && entry.Name() == ".git" {
				return filepath.SkipDir
			}
			if rel == "." || entry.IsDir() {
				return nil
			}
			paths = append(paths, filepath.ToSlash(rel))
			return nil
		})
		if err != nil {
			return "", err
		}
	}
	sort.Strings(paths)
	h := sha256.New()
	fmt.Fprintf(h, "root=%s\nhead=%s\n", filepath.ToSlash(root), head)
	for _, rel := range paths {
		full := filepath.Join(root, filepath.FromSlash(rel))
		info, statErr := os.Lstat(full)
		if errors.Is(statErr, os.ErrNotExist) {
			fmt.Fprintf(h, "%s\x00missing\n", rel)
			continue
		}
		if statErr != nil {
			return "", statErr
		}
		if info.Mode()&os.ModeSymlink != 0 {
			target, linkErr := filepath.EvalSymlinks(full)
			if linkErr != nil || !inside(root, target) {
				return "", fmt.Errorf("target symlink escapes or is unreadable: %s", rel)
			}
			info, statErr = os.Stat(target)
			if statErr != nil {
				return "", statErr
			}
			full = target
		}
		if !info.Mode().IsRegular() {
			fmt.Fprintf(h, "%s\x00non-regular\n", rel)
			continue
		}
		if info.Size() > 64<<20 {
			return "", fmt.Errorf("target file exceeds bounded identity read: %s", rel)
		}
		digest, readErr := hashFile(full)
		if readErr != nil {
			return "", readErr
		}
		fmt.Fprintf(h, "%s\x00%s\n", rel, digest)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// targetRevisionIdentity deliberately excludes unrelated dirty files. Review
// separately freezes every execute-reported changed path, so repository-wide
// hashing would make an unrelated edit invalidate an otherwise atomic review.
func targetRevisionIdentity(root string) (string, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	head := "non-git"
	if output, gitErr := exec.Command("git", "-C", root, "rev-parse", "--show-toplevel").Output(); gitErr == nil {
		gitRoot, absErr := filepath.Abs(strings.TrimSpace(string(output)))
		if absErr == nil && filepath.Clean(gitRoot) == filepath.Clean(root) {
			if output, headErr := exec.Command("git", "-C", root, "rev-parse", "HEAD").Output(); headErr == nil {
				head = strings.TrimSpace(string(output))
			}
		}
	}
	h := sha256.New()
	fmt.Fprintf(h, "root=%s\nhead=%s\n", filepath.ToSlash(root), head)
	return hex.EncodeToString(h.Sum(nil)), nil
}

func attemptExpired(attempt *VerificationAttempt, now time.Time) bool {
	if attempt == nil || attempt.Status != AttemptRunning {
		return false
	}
	if attempt.OwnerPID > 0 && !verificationProcessAlive(attempt.OwnerPID) {
		return true
	}
	lastSeen := attempt.StartedAt
	if attempt.HeartbeatAt.After(lastSeen) {
		lastSeen = attempt.HeartbeatAt
	}
	return now.Sub(lastSeen) > 2*time.Hour
}

func reconcileExpiredAttempts(state *FlowState, now time.Time) bool {
	changed := false
	if state.Review != nil && attemptExpired(state.Review.ActiveAttempt, now) {
		attempt := *state.Review.ActiveAttempt
		attempt.Status = AttemptTimedOut
		attempt.CompletedAt = &now
		attempt.Category = "interrupted"
		attempt.Diagnostics = []string{"review attempt expired without a terminal update"}
		attempt.NextAction = "Resume the retained review coverage, or restart review explicitly."
		state.Review.ActiveAttempt = nil
		state.Review.LastAttempt = &attempt
		state.Review.Status = ReviewFailed
		state.Review.LastRunAt = &now
		changed = true
	}
	if state.Smoke != nil && attemptExpired(state.Smoke.ActiveAttempt, now) {
		attempt := *state.Smoke.ActiveAttempt
		attempt.Status = AttemptTimedOut
		attempt.CompletedAt = &now
		attempt.Category = "interrupted"
		attempt.Diagnostics = []string{"smoke attempt expired without a terminal update"}
		attempt.NextAction = "Confirm no harness process remains, then rerun smoke."
		state.Smoke.ActiveAttempt = nil
		state.Smoke.LastAttempt = &attempt
		state.Smoke.Status = SmokeFailed
		state.Smoke.LastRunAt = &now
		changed = true
	}
	return changed
}
