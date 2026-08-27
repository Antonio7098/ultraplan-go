package sprint

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Antonio7098/ultraplan-go/internal/platform/gitpublish"
	pruntime "github.com/Antonio7098/ultraplan-go/internal/platform/runtime"
)

const (
	StageMerge              PlanningStage = "merge"
	mergeStateSchemaVersion               = 1
)

type MergeStatus string

const (
	MergeReady      MergeStatus = "ready"
	MergeDescribing MergeStatus = "describing"
	MergeMerging    MergeStatus = "merging"
	MergeConflicts  MergeStatus = "conflicts"
	MergeValidating MergeStatus = "validating"
	MergeCompleted  MergeStatus = "completed"
	MergeFailed     MergeStatus = "failed"
	MergeAborted    MergeStatus = "aborted"
	MergeStale      MergeStatus = "stale"
)

type MergeDescription struct {
	Title        string   `json:"title"`
	Summary      []string `json:"summary"`
	Verification []string `json:"verification,omitempty"`
	RiskNotes    []string `json:"risk_notes,omitempty"`
}

func (d *MergeDescription) UnmarshalJSON(data []byte) error {
	type wireDescription struct {
		Title        string          `json:"title"`
		Summary      json.RawMessage `json:"summary"`
		Verification json.RawMessage `json:"verification"`
		RiskNotes    json.RawMessage `json:"risk_notes"`
	}
	var wire wireDescription
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	if strings.TrimSpace(wire.Title) == "" {
		return fmt.Errorf("title is required")
	}
	var err error
	if d.Summary, err = decodeMergeDescriptionList(wire.Summary); err != nil {
		return fmt.Errorf("summary: %w", err)
	}
	if d.Verification, err = decodeMergeDescriptionList(wire.Verification); err != nil {
		return fmt.Errorf("verification: %w", err)
	}
	if d.RiskNotes, err = decodeMergeDescriptionList(wire.RiskNotes); err != nil {
		return fmt.Errorf("risk_notes: %w", err)
	}
	d.Title = wire.Title
	return nil
}

func decodeMergeDescriptionList(data json.RawMessage) ([]string, error) {
	if len(data) == 0 || string(data) == "null" {
		return nil, nil
	}
	var list []string
	if err := json.Unmarshal(data, &list); err == nil {
		return list, nil
	}
	var item string
	if err := json.Unmarshal(data, &item); err != nil {
		return nil, fmt.Errorf("must be a string or an array of strings")
	}
	return []string{item}, nil
}

type MergeInspection struct {
	SchemaVersion             int      `json:"schema_version"`
	Project                   string   `json:"project"`
	Sprint                    string   `json:"sprint"`
	SourceRoot                string   `json:"source_root"`
	SourceWorktree            string   `json:"source_worktree"`
	SourceBranch              string   `json:"source_branch"`
	SourceCommit              string   `json:"source_commit"`
	TargetBranch              string   `json:"target_branch"`
	TargetCommit              string   `json:"target_commit"`
	Baseline                  string   `json:"baseline"`
	MergeBase                 string   `json:"merge_base"`
	Commits                   []string `json:"commits,omitempty"`
	ChangedPaths              []string `json:"changed_paths,omitempty"`
	SourceDirtyPaths          []string `json:"source_dirty_paths,omitempty"`
	SourceWorktreeFingerprint string   `json:"source_worktree_fingerprint,omitempty"`
	LikelyConflicts           []string `json:"likely_conflicts,omitempty"`
	AlreadyMerged             bool     `json:"already_merged"`
	Ready                     bool     `json:"ready"`
	Diagnostics               []string `json:"diagnostics,omitempty"`
}

type MergeCheck struct {
	Name   string `json:"name"`
	Passed bool   `json:"passed"`
	Detail string `json:"detail,omitempty"`
}

type MergeState struct {
	SchemaVersion int               `json:"schema_version"`
	Project       string            `json:"project"`
	Sprint        string            `json:"sprint"`
	Status        MergeStatus       `json:"status"`
	SourceBranch  string            `json:"source_branch"`
	SourceCommit  string            `json:"source_commit"`
	TargetBranch  string            `json:"target_branch"`
	TargetBefore  string            `json:"target_before"`
	MergeBase     string            `json:"merge_base"`
	MergeCommit   string            `json:"merge_commit,omitempty"`
	Description   *MergeDescription `json:"description,omitempty"`
	ConflictPaths []string          `json:"conflict_paths,omitempty"`
	Checks        []MergeCheck      `json:"checks,omitempty"`
	RuntimeRunIDs []string          `json:"runtime_run_ids,omitempty"`
	StartedAt     time.Time         `json:"started_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
	CompletedAt   *time.Time        `json:"completed_at,omitempty"`
	Diagnostic    string            `json:"diagnostic,omitempty"`
}

type MergeRequest struct {
	DryRun        bool
	Confirm       bool
	ModelOverride string
	Continue      bool
}

type MergeResult struct {
	Inspection   MergeInspection     `json:"inspection"`
	State        MergeState          `json:"state"`
	Artifact     string              `json:"artifact,omitempty"`
	Publications []gitpublish.Result `json:"publications,omitempty"`
}

func MergeStateRelPath(sp Sprint) string {
	return filepath.ToSlash(filepath.Join("projects", sp.Project, "sprints", sp.Slug, ".merge-state.json"))
}

func mergeArtifactRelPath(sp Sprint) string {
	return filepath.ToSlash(filepath.Join("projects", sp.Project, "sprints", sp.Slug, "merge.md"))
}

func (s Service) InspectMerge(projectRef, sprintRef string) (MergeInspection, error) {
	sp, err := s.resolveMutationSprint(projectRef, sprintRef)
	if err != nil {
		return MergeInspection{}, err
	}
	record, err := loadSprintWorkspace(sp)
	if err != nil {
		return MergeInspection{}, fmt.Errorf("merge: load sprint workspace: %w", err)
	}
	out := MergeInspection{SchemaVersion: 1, Project: sp.Project, Sprint: sp.Slug, SourceRoot: record.SourceRoot, SourceWorktree: record.Path, SourceBranch: record.Branch, TargetBranch: record.IntegrationBranch, Baseline: record.Baseline}
	if err := validateSprintWorkspace(record, record.SourceRoot); err != nil {
		out.Diagnostics = append(out.Diagnostics, err.Error())
		return out, nil
	}
	checks := []struct {
		dir  string
		args []string
		dst  *string
	}{
		{record.Path, []string{"rev-parse", "HEAD"}, &out.SourceCommit},
		{record.SourceRoot, []string{"rev-parse", "HEAD"}, &out.TargetCommit},
		{record.SourceRoot, []string{"merge-base", record.IntegrationBranch, record.Branch}, &out.MergeBase},
	}
	for _, check := range checks {
		value, gitErr := gitOutput(check.dir, check.args...)
		if gitErr != nil {
			out.Diagnostics = append(out.Diagnostics, gitErr.Error())
		} else {
			*check.dst = strings.TrimSpace(value)
		}
	}
	targetBranch, targetErr := gitOutput(record.SourceRoot, "branch", "--show-current")
	if targetErr != nil || strings.TrimSpace(targetBranch) != record.IntegrationBranch {
		out.Diagnostics = append(out.Diagnostics, fmt.Sprintf("target checkout must be on recorded integration branch %q", record.IntegrationBranch))
	}
	status, statusErr := gitStatusOutput(record.Path, "--untracked-files=all")
	if statusErr != nil {
		out.Diagnostics = append(out.Diagnostics, "sprint worktree status is unavailable")
	} else {
		out.SourceDirtyPaths = mergeStatusPaths(status)
		out.SourceWorktreeFingerprint = mergeWorktreeFingerprint(record.Path)
	}
	targetStatus, targetStatusErr := gitStatusOutput(record.SourceRoot, "--untracked-files=normal")
	if targetStatusErr != nil || strings.TrimSpace(targetStatus) != "" {
		out.Diagnostics = append(out.Diagnostics, "target worktree is not clean")
	}
	if mergeHead, _ := gitOutput(record.SourceRoot, "rev-parse", "-q", "--verify", "MERGE_HEAD"); mergeHead != "" {
		out.Diagnostics = append(out.Diagnostics, "target worktree already has an active merge")
	}
	if out.SourceCommit != "" && out.TargetCommit != "" {
		out.AlreadyMerged = gitCommand(record.SourceRoot, "merge-base", "--is-ancestor", out.SourceCommit, out.TargetCommit) == nil
		if out.AlreadyMerged {
			out.Diagnostics = append(out.Diagnostics, "sprint commit is already contained in the target branch")
		}
	}
	if out.Baseline != "" && gitCommand(record.SourceRoot, "merge-base", "--is-ancestor", out.Baseline, out.SourceCommit) != nil {
		out.Diagnostics = append(out.Diagnostics, "sprint branch no longer descends from its recorded baseline")
	}
	out.Commits = gitLines(record.SourceRoot, "log", "--format=%h %s", out.TargetCommit+".."+out.SourceCommit)
	out.ChangedPaths = uniqueSorted(append(gitLines(record.SourceRoot, "diff", "--name-only", out.MergeBase+".."+out.SourceCommit), out.SourceDirtyPaths...))
	if len(out.SourceDirtyPaths) == 0 {
		out.LikelyConflicts = likelyMergeConflicts(record.SourceRoot, out.TargetCommit, out.SourceCommit)
	}
	verification, verificationErr := s.VerificationStatus(projectRef, sprintRef)
	if verificationErr != nil {
		out.Diagnostics = append(out.Diagnostics, "verification status is unavailable: "+safeError(verificationErr))
	} else if verification.Assessment != AssessmentPass && verification.Assessment != AssessmentPassWithFindings {
		out.Diagnostics = append(out.Diagnostics, "current review and smoke evidence must pass before merge")
	}
	out.Diagnostics = uniqueSorted(out.Diagnostics)
	out.Ready = len(out.Diagnostics) == 0
	return out, nil
}

func likelyMergeConflicts(root, ours, theirs string) []string {
	if ours == "" || theirs == "" {
		return nil
	}
	output, err := exec.Command("git", "-C", root, "merge-tree", "--write-tree", ours, theirs).CombinedOutput()
	if err == nil {
		return nil
	}
	var paths []string
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 4 && (fields[0] == "100644" || fields[0] == "100755" || fields[0] == "120000") {
			paths = append(paths, fields[len(fields)-1])
		}
	}
	return uniqueSorted(paths)
}

func gitLines(dir string, args ...string) []string {
	value, err := gitOutput(dir, args...)
	if err != nil || strings.TrimSpace(value) == "" {
		return nil
	}
	return strings.Split(strings.TrimSpace(value), "\n")
}

func gitCommand(dir string, args ...string) error {
	output, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s: %s: %w", strings.Join(args, " "), strings.TrimSpace(string(output)), err)
	}
	return nil
}

func (s Service) RunMerge(ctx context.Context, projectRef, sprintRef string, req MergeRequest) (MergeResult, error) {
	inspection, err := s.InspectMerge(projectRef, sprintRef)
	result := MergeResult{Inspection: inspection}
	if err != nil {
		return result, err
	}
	sp, err := s.resolveMutationSprint(projectRef, sprintRef)
	if err != nil {
		return result, err
	}
	if req.DryRun {
		result.State = MergeState{SchemaVersion: 1, Project: sp.Project, Sprint: sp.Slug, Status: MergeReady, SourceBranch: inspection.SourceBranch, SourceCommit: inspection.SourceCommit, TargetBranch: inspection.TargetBranch, TargetBefore: inspection.TargetCommit, MergeBase: inspection.MergeBase}
		if !inspection.Ready {
			return result, fmt.Errorf("merge is not ready: %s", strings.Join(inspection.Diagnostics, "; "))
		}
		return result, nil
	}
	if !req.Confirm {
		return result, fmt.Errorf("merge requires --yes")
	}
	if !inspection.Ready && !req.Continue {
		return result, fmt.Errorf("merge is not ready: %s", strings.Join(inspection.Diagnostics, "; "))
	}
	release, err := acquireMergeLock(inspection.SourceRoot)
	if err != nil {
		return result, err
	}
	defer release()
	now := s.now().UTC()
	state := MergeState{SchemaVersion: mergeStateSchemaVersion, Project: sp.Project, Sprint: sp.Slug, Status: MergeDescribing, SourceBranch: inspection.SourceBranch, SourceCommit: inspection.SourceCommit, TargetBranch: inspection.TargetBranch, TargetBefore: inspection.TargetCommit, MergeBase: inspection.MergeBase, StartedAt: now, UpdatedAt: now}
	if req.Continue {
		state, err = s.LoadMergeState(projectRef, sprintRef)
		if err != nil {
			return result, err
		}
		if state.Status != MergeConflicts && state.Status != MergeFailed {
			return result, fmt.Errorf("merge cannot continue from status %q", state.Status)
		}
		mergeHead, mergeHeadErr := gitOutput(inspection.SourceRoot, "rev-parse", "-q", "--verify", "MERGE_HEAD")
		if mergeHeadErr != nil || strings.TrimSpace(mergeHead) != state.SourceCommit {
			return result, fmt.Errorf("active merge no longer matches the recorded sprint commit")
		}
		if len(state.ConflictPaths) > 0 {
			state.Status = MergeConflicts
		}
	} else {
		if err := s.saveMergeState(sp, state); err != nil {
			return result, err
		}
		description, runID, describeErr := s.generateMergeDescription(ctx, sp, inspection, req.ModelOverride)
		if describeErr != nil {
			state.Status, state.Diagnostic = MergeFailed, safeError(describeErr)
			_ = s.saveMergeState(sp, state)
			return MergeResult{Inspection: inspection, State: state}, describeErr
		}
		state.Description = &description
		if runID != "" {
			state.RuntimeRunIDs = append(state.RuntimeRunIDs, runID)
		}
		if len(inspection.SourceDirtyPaths) > 0 {
			if mergeWorktreeFingerprint(inspection.SourceWorktree) != inspection.SourceWorktreeFingerprint {
				return result, fmt.Errorf("sprint worktree changed while the merge description was generated; inspect and retry")
			}
			if snapshotErr := commitSprintSnapshot(inspection.SourceWorktree, sp, description); snapshotErr != nil {
				state.Status, state.Diagnostic = MergeFailed, safeError(snapshotErr)
				_ = s.saveMergeState(sp, state)
				return MergeResult{Inspection: inspection, State: state}, snapshotErr
			}
			inspection.SourceCommit, _ = gitOutput(inspection.SourceWorktree, "rev-parse", "HEAD")
			inspection.SourceDirtyPaths = nil
			inspection.SourceWorktreeFingerprint = mergeWorktreeFingerprint(inspection.SourceWorktree)
			inspection.Commits = gitLines(inspection.SourceRoot, "log", "--format=%h %s", inspection.TargetCommit+".."+inspection.SourceCommit)
			inspection.ChangedPaths = gitLines(inspection.SourceRoot, "diff", "--name-only", inspection.MergeBase+".."+inspection.SourceCommit)
			inspection.LikelyConflicts = likelyMergeConflicts(inspection.SourceRoot, inspection.TargetCommit, inspection.SourceCommit)
			state.SourceCommit = inspection.SourceCommit
		}
		state.Status, state.UpdatedAt = MergeMerging, s.now().UTC()
		if err := s.saveMergeState(sp, state); err != nil {
			return result, err
		}
		mergeErr := gitCommand(inspection.SourceRoot, "merge", "--no-ff", "--no-commit", inspection.SourceCommit)
		if mergeErr != nil {
			state.ConflictPaths = gitLines(inspection.SourceRoot, "diff", "--name-only", "--diff-filter=U")
			if len(state.ConflictPaths) == 0 {
				state.Status, state.Diagnostic = MergeFailed, safeError(mergeErr)
				_ = s.saveMergeState(sp, state)
				return MergeResult{Inspection: inspection, State: state}, mergeErr
			}
			state.Status = MergeConflicts
			_ = s.saveMergeState(sp, state)
		}
	}
	if state.Status == MergeConflicts {
		runID, reconcileErr := s.reconcileMergeConflicts(ctx, sp, inspection.SourceRoot, state, req.ModelOverride)
		if runID != "" {
			state.RuntimeRunIDs = append(state.RuntimeRunIDs, runID)
		}
		if reconcileErr != nil {
			state.Status, state.Diagnostic = MergeFailed, safeError(reconcileErr)
			_ = s.saveMergeState(sp, state)
			return MergeResult{Inspection: inspection, State: state}, reconcileErr
		}
		for _, path := range state.ConflictPaths {
			if err := gitCommand(inspection.SourceRoot, "add", "--", path); err != nil {
				return result, err
			}
		}
	}
	state.Status, state.UpdatedAt = MergeValidating, s.now().UTC()
	checks, validateErr := validateMergeCheckout(ctx, inspection.SourceRoot, state)
	state.Checks = checks
	if validateErr != nil {
		state.Status, state.Diagnostic = MergeFailed, safeError(validateErr)
		_ = s.saveMergeState(sp, state)
		return MergeResult{Inspection: inspection, State: state}, validateErr
	}
	message := renderMergeCommitMessage(*state.Description)
	if err := exec.Command("git", "-C", inspection.SourceRoot, "commit", "-m", message).Run(); err != nil {
		state.Status, state.Diagnostic = MergeFailed, safeError(err)
		_ = s.saveMergeState(sp, state)
		return MergeResult{Inspection: inspection, State: state}, fmt.Errorf("create merge commit: %w", err)
	}
	state.MergeCommit, _ = gitOutput(inspection.SourceRoot, "rev-parse", "HEAD")
	completed := s.now().UTC()
	state.Status, state.UpdatedAt, state.CompletedAt = MergeCompleted, completed, &completed
	state.Diagnostic = ""
	if err := s.saveMergeState(sp, state); err != nil {
		return result, err
	}
	artifact := mergeArtifactRelPath(sp)
	if err := atomicWriteFile(filepath.Join(s.root, filepath.FromSlash(artifact)), []byte(renderMergeMarkdown(state))); err != nil {
		return result, err
	}
	publications, publishErr := s.publishMergeStage(ctx, sp, inspection.SourceRoot, artifact)
	return MergeResult{Inspection: inspection, State: state, Artifact: artifact, Publications: publications}, publishErr
}

func mergeStatusPaths(status string) []string {
	var paths []string
	for _, line := range strings.Split(strings.TrimRight(status, "\r\n"), "\n") {
		if len(line) < 4 {
			continue
		}
		path := strings.TrimSpace(line[3:])
		if before, after, renamed := strings.Cut(path, " -> "); renamed {
			paths = append(paths, before, after)
		} else {
			paths = append(paths, path)
		}
	}
	return uniqueSorted(paths)
}

func gitStatusOutput(root string, untracked string) (string, error) {
	output, err := exec.Command("git", "-C", root, "status", "--porcelain", untracked).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s: %w", strings.TrimSpace(string(output)), err)
	}
	return strings.TrimRight(string(output), "\r\n"), nil
}

func mergeWorktreeFingerprint(root string) string {
	status, statusErr := gitStatusOutput(root, "--untracked-files=all")
	if statusErr != nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(status)
	for _, path := range mergeStatusPaths(status) {
		b.WriteByte('\x00')
		b.WriteString(path)
		b.WriteByte('\x00')
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		if err != nil {
			b.WriteString("missing")
		} else {
			b.WriteString(hashBytes(data))
		}
	}
	return hashBytes([]byte(b.String()))
}

func commitSprintSnapshot(root string, sp Sprint, description MergeDescription) error {
	if err := gitCommand(root, "add", "-A"); err != nil {
		return fmt.Errorf("stage sprint snapshot: %w", err)
	}
	message := fmt.Sprintf("ultraplan: snapshot sprint %s/%s\n\n%s", sp.Project, sp.Slug, renderMergeCommitMessage(description))
	command := exec.Command("git", "-C", root, "commit", "-m", message)
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("commit sprint snapshot: %s: %w", strings.TrimSpace(string(output)), err)
	}
	status, err := gitStatusOutput(root, "--untracked-files=normal")
	if err != nil || strings.TrimSpace(status) != "" {
		return fmt.Errorf("sprint snapshot did not leave a clean worktree")
	}
	return nil
}

func (s Service) publishMergeStage(ctx context.Context, sp Sprint, targetRoot, artifact string) ([]gitpublish.Result, error) {
	if s.publisher == nil {
		return nil, nil
	}
	identity := fmt.Sprintf("sprint/%s/%s/%s", sp.Project, sp.Slug, StageMerge)
	message := fmt.Sprintf("ultraplan: sprint %s/%s complete %s", sp.Project, sp.Slug, StageMerge)
	target, err := s.publisher.Publish(ctx, gitpublish.Request{Root: targetRoot, Message: message, Identity: identity + "/implementation"})
	results := visiblePublication(target)
	if err != nil {
		return results, err
	}
	workspaceResult, err := s.publisher.Publish(ctx, gitpublish.Request{Root: s.root, Paths: []string{filepath.Join(s.root, filepath.FromSlash(artifact)), filepath.Join(s.root, filepath.FromSlash(MergeStateRelPath(sp)))}, Message: message, Identity: identity + "/workspace"})
	return append(results, visiblePublication(workspaceResult)...), err
}

func (s Service) AbortMerge(projectRef, sprintRef string) (MergeState, error) {
	sp, err := s.resolveMutationSprint(projectRef, sprintRef)
	if err != nil {
		return MergeState{}, err
	}
	record, err := loadSprintWorkspace(sp)
	if err != nil {
		return MergeState{}, err
	}
	state, err := s.LoadMergeState(projectRef, sprintRef)
	if err != nil {
		return MergeState{}, err
	}
	if err := gitCommand(record.SourceRoot, "merge", "--abort"); err != nil {
		return state, err
	}
	state.Status, state.UpdatedAt = MergeAborted, s.now().UTC()
	state.Diagnostic = "merge aborted by operator"
	return state, s.saveMergeState(sp, state)
}

func (s Service) ValidateMerge(projectRef, sprintRef string) (ValidationResult, error) {
	sp, err := s.resolveMutationSprint(projectRef, sprintRef)
	if err != nil {
		return ValidationResult{}, err
	}
	artifact := mergeArtifactRelPath(sp)
	result := ValidationResult{Project: sp.Project, Sprint: sp.Slug, Artifact: artifact}
	state, err := s.LoadMergeState(projectRef, sprintRef)
	if err != nil {
		result.Findings = append(result.Findings, finding("merge state", "", MergeStateRelPath(sp), "missing or invalid merge state", safeError(err), "Run or resume sprint merge."))
		return result, nil
	}
	if state.Status != MergeCompleted || state.MergeCommit == "" {
		result.Findings = append(result.Findings, finding("merge state", "", MergeStateRelPath(sp), "merge is not complete", string(state.Status), "Resume or rerun sprint merge."))
		return result, nil
	}
	record, recordErr := loadSprintWorkspace(sp)
	if recordErr != nil {
		return result, recordErr
	}
	head, headErr := gitOutput(record.SourceRoot, "rev-parse", state.MergeCommit+"^{commit}")
	if headErr != nil || strings.TrimSpace(head) != state.MergeCommit {
		result.Findings = append(result.Findings, finding("merge commit", "", record.SourceRoot, "recorded merge commit is unavailable", safeError(headErr), "Restore the commit or rerun merge."))
	}
	parents, parentErr := gitOutput(record.SourceRoot, "show", "-s", "--format=%P", state.MergeCommit)
	if parentErr != nil || !mergeContainsString(strings.Fields(parents), state.TargetBefore) || !mergeContainsString(strings.Fields(parents), state.SourceCommit) {
		result.Findings = append(result.Findings, finding("merge commit", "", state.MergeCommit, "merge parents do not match recorded inputs", strings.TrimSpace(parents), "Inspect Git history and rerun from a clean target branch."))
	}
	data, readErr := os.ReadFile(filepath.Join(s.root, filepath.FromSlash(artifact)))
	if readErr != nil || !strings.HasPrefix(string(data), "# Sprint merge\n") {
		result.Findings = append(result.Findings, finding("merge artifact", "", artifact, "missing or malformed merge artifact", safeError(readErr), "Restore merge.md from the recorded merge state."))
	}
	return result, nil
}

func mergeContainsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func (s Service) LoadMergeState(projectRef, sprintRef string) (MergeState, error) {
	sp, err := s.resolveMutationSprint(projectRef, sprintRef)
	if err != nil {
		return MergeState{}, err
	}
	data, err := os.ReadFile(filepath.Join(s.root, filepath.FromSlash(MergeStateRelPath(sp))))
	if err != nil {
		return MergeState{}, err
	}
	var state MergeState
	if err := json.Unmarshal(data, &state); err != nil {
		return MergeState{}, err
	}
	if state.SchemaVersion != mergeStateSchemaVersion || state.Project != sp.Project || state.Sprint != sp.Slug {
		return MergeState{}, fmt.Errorf("invalid merge state")
	}
	return state, nil
}

func (s Service) saveMergeState(sp Sprint, state MergeState) error {
	state.UpdatedAt = s.now().UTC()
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return atomicWriteFile(filepath.Join(s.root, filepath.FromSlash(MergeStateRelPath(sp))), append(data, '\n'))
}

func (s Service) generateMergeDescription(ctx context.Context, sp Sprint, inspection MergeInspection, model string) (MergeDescription, string, error) {
	if s.runtime == nil {
		return MergeDescription{}, "", fmt.Errorf("merge description runtime is not configured")
	}
	payload, _ := json.MarshalIndent(inspection, "", "  ")
	prompt := "Write the merge description for this sprint. Return one JSON object with title, summary, verification, and risk_notes. The title must be imperative and at most 72 characters. Do not edit files or run Git.\n\n" + string(payload)
	req := s.runtimeRequest(prompt, map[string]string{"project": sp.Project, "sprint": sp.Slug, "stage": string(StageMerge), "operation": "describe"})
	req.WorkDir = inspection.SourceWorktree
	if strings.TrimSpace(model) != "" {
		req.Provider, req.Model = splitProviderModel(model)
	}
	req.Sandbox, req.Permissions = "read_only", "restricted"
	req.Policy = pruntime.PermissionPolicy{Default: "deny", Tools: map[string]string{"read": "allow", "list": "allow", "search": "allow"}}
	run, err := s.startSprintRuntime(ctx, sp, StageMerge, req)
	if err != nil {
		return MergeDescription{}, run.RunID, err
	}
	var description MergeDescription
	if err := decodeRuntimeJSON(run, &description); err != nil {
		return description, run.RunID, fmt.Errorf("decode merge description: %w", err)
	}
	if err := validateMergeDescription(description); err != nil {
		return description, run.RunID, err
	}
	return description, run.RunID, nil
}

func (s Service) reconcileMergeConflicts(ctx context.Context, sp Sprint, root string, state MergeState, model string) (string, error) {
	if s.runtime == nil {
		return "", fmt.Errorf("merge reconciliation runtime is not configured")
	}
	payload, _ := json.MarshalIndent(state, "", "  ")
	prompt := "Reconcile the active Git merge conflicts listed below. Edit only the listed conflict paths. Preserve the intent of both sides and remove conflict markers. Do not run git add, commit, merge, checkout, reset, push, or branch commands. Do not edit any other path. Finish with a short plain-text resolution summary.\n\n" + string(payload)
	req := s.runtimeRequest(prompt, map[string]string{"project": sp.Project, "sprint": sp.Slug, "stage": string(StageMerge), "operation": "reconcile"})
	req.WorkDir = root
	if strings.TrimSpace(model) != "" {
		req.Provider, req.Model = splitProviderModel(model)
	}
	req.Sandbox, req.Permissions = "workspace_write", "restricted"
	req.Policy = pruntime.PermissionPolicy{Default: "deny", Tools: map[string]string{"read": "allow", "list": "allow", "search": "allow", "edit": "allow", "write": "allow"}}
	before := mergeWorkingDigests(root)
	run, err := s.startSprintRuntime(ctx, sp, StageMerge, req)
	if err != nil {
		return run.RunID, err
	}
	after := mergeWorkingDigests(root)
	allowed := map[string]bool{}
	for _, path := range state.ConflictPaths {
		allowed[path] = true
	}
	allPaths := map[string]bool{}
	for path := range before {
		allPaths[path] = true
	}
	for path := range after {
		allPaths[path] = true
	}
	for path := range allPaths {
		if !allowed[path] && before[path] != after[path] {
			return run.RunID, fmt.Errorf("merge reconciliation changed unapproved path %q", path)
		}
	}
	for _, path := range state.ConflictPaths {
		data, readErr := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
			return run.RunID, readErr
		}
		if strings.Contains(string(data), "<<<<<<<") || strings.Contains(string(data), ">>>>>>>") {
			return run.RunID, fmt.Errorf("conflict markers remain in %q", path)
		}
	}
	return run.RunID, nil
}

func mergeWorkingDigests(root string) map[string]string {
	result := map[string]string{}
	output, err := exec.Command("git", "-C", root, "status", "--porcelain", "-z", "--untracked-files=all").Output()
	if err != nil {
		return result
	}
	entries := strings.Split(string(output), "\x00")
	for i := 0; i < len(entries); i++ {
		entry := entries[i]
		if len(entry) < 4 {
			continue
		}
		path := entry[3:]
		if entry[0] == 'R' || entry[0] == 'C' || entry[1] == 'R' || entry[1] == 'C' {
			if i+1 < len(entries) {
				i++
				path = entries[i]
			}
		}
		data, readErr := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		if readErr != nil {
			result[path] = "missing"
		} else {
			result[path] = hashBytes(data)
		}
	}
	return result
}

func decodeRuntimeJSON(run pruntime.Result, dst any) error {
	if decodeRuntimeJSONValue(run.TerminalOutput, dst) {
		return nil
	}
	for i := len(run.Events) - 1; i >= 0; i-- {
		if decodeRuntimeJSONValue(run.Events[i].Payload, dst) {
			return nil
		}
	}
	return fmt.Errorf("runtime returned no valid JSON object")
}

func decodeRuntimeJSONValue(value any, dst any) bool {
	switch value := value.(type) {
	case map[string]any:
		if data, err := json.Marshal(value); err == nil && json.Unmarshal(data, dst) == nil {
			return true
		}
		for _, key := range []string{"structured_output", "output", "content", "text", "message", "part"} {
			if nested, ok := value[key]; ok && decodeRuntimeJSONValue(nested, dst) {
				return true
			}
		}
	case []any:
		for i := len(value) - 1; i >= 0; i-- {
			if decodeRuntimeJSONValue(value[i], dst) {
				return true
			}
		}
	case string:
		for offset := 0; offset < len(value); {
			relative := strings.IndexByte(value[offset:], '{')
			if relative < 0 {
				break
			}
			start := offset + relative
			if json.NewDecoder(strings.NewReader(value[start:])).Decode(dst) == nil {
				return true
			}
			offset = start + 1
		}
	}
	return false
}

func validateMergeDescription(value MergeDescription) error {
	value.Title = strings.TrimSpace(value.Title)
	if value.Title == "" || len(value.Title) > 72 || strings.ContainsAny(value.Title, "\r\n\x00") {
		return fmt.Errorf("merge description title is invalid")
	}
	if len(value.Summary) == 0 || len(value.Summary) > 8 {
		return fmt.Errorf("merge description needs 1 to 8 summary entries")
	}
	for _, list := range [][]string{value.Summary, value.Verification, value.RiskNotes} {
		for _, item := range list {
			if strings.TrimSpace(item) == "" || len(item) > 300 || strings.ContainsAny(item, "\r\x00") {
				return fmt.Errorf("merge description contains an invalid entry")
			}
		}
	}
	return nil
}

func validateMergeCheckout(ctx context.Context, root string, state MergeState) ([]MergeCheck, error) {
	checks := []MergeCheck{}
	unmerged := gitLines(root, "diff", "--name-only", "--diff-filter=U")
	checks = append(checks, MergeCheck{Name: "unmerged paths", Passed: len(unmerged) == 0, Detail: strings.Join(unmerged, ", ")})
	mergeHead, err := gitOutput(root, "rev-parse", "-q", "--verify", "MERGE_HEAD")
	checks = append(checks, MergeCheck{Name: "merge head", Passed: err == nil && strings.TrimSpace(mergeHead) == state.SourceCommit, Detail: strings.TrimSpace(mergeHead)})
	if len(unmerged) > 0 {
		return checks, fmt.Errorf("unmerged paths remain: %s", strings.Join(unmerged, ", "))
	}
	if err != nil || strings.TrimSpace(mergeHead) != state.SourceCommit {
		return checks, fmt.Errorf("active merge no longer matches the recorded sprint commit")
	}
	diffCheck := exec.CommandContext(ctx, "git", "-C", root, "diff", "--check", "--cached")
	diffOutput, diffErr := diffCheck.CombinedOutput()
	checks = append(checks, MergeCheck{Name: "git diff --check", Passed: diffErr == nil, Detail: boundedMergeOutput(diffOutput)})
	if diffErr != nil {
		return checks, fmt.Errorf("merged tree failed git diff --check: %s", boundedMergeOutput(diffOutput))
	}
	if _, statErr := os.Stat(filepath.Join(root, "go.mod")); statErr == nil {
		command := exec.CommandContext(ctx, "go", "test", "./...")
		command.Dir = root
		output, testErr := command.CombinedOutput()
		checks = append(checks, MergeCheck{Name: "go test ./...", Passed: testErr == nil, Detail: boundedMergeOutput(output)})
		if testErr != nil {
			return checks, fmt.Errorf("merged tree failed go test ./...: %s", boundedMergeOutput(output))
		}
	}
	return checks, nil
}

func boundedMergeOutput(value []byte) string {
	const limit = 4096
	value = []byte(strings.TrimSpace(string(value)))
	if len(value) > limit {
		value = append(value[:limit], []byte("... output truncated")...)
	}
	return string(value)
}

func renderMergeCommitMessage(value MergeDescription) string {
	var b strings.Builder
	b.WriteString(strings.TrimSpace(value.Title))
	for _, item := range value.Summary {
		fmt.Fprintf(&b, "\n\n- %s", strings.TrimSpace(item))
	}
	if len(value.Verification) > 0 {
		b.WriteString("\n\nVerification:")
		for _, item := range value.Verification {
			fmt.Fprintf(&b, "\n- %s", strings.TrimSpace(item))
		}
	}
	return b.String()
}

func renderMergeMarkdown(state MergeState) string {
	var b strings.Builder
	fmt.Fprintln(&b, "# Sprint merge")
	fmt.Fprintf(&b, "\n- Source: `%s` at `%s`\n- Target: `%s` from `%s`\n- Merge commit: `%s`\n", state.SourceBranch, state.SourceCommit, state.TargetBranch, state.TargetBefore, state.MergeCommit)
	if state.Description != nil {
		fmt.Fprintf(&b, "\n## %s\n", state.Description.Title)
		for _, item := range state.Description.Summary {
			fmt.Fprintf(&b, "\n- %s", item)
		}
	}
	if len(state.ConflictPaths) > 0 {
		fmt.Fprintln(&b, "\n## Reconciled conflicts")
		for _, path := range state.ConflictPaths {
			fmt.Fprintf(&b, "\n- `%s`", path)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func acquireMergeLock(root string) (func(), error) {
	common, err := gitCommonDirectory(root)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(common, "ultraplan-merge.lock")
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, fmt.Errorf("acquire merge lock: %w", err)
	}
	_, _ = fmt.Fprintf(file, "%d\n", os.Getpid())
	_ = file.Close()
	return func() { _ = os.Remove(path) }, nil
}
