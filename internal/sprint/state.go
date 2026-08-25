package sprint

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type atomicWriteHooks struct {
	BeforeRename func(path string) error
}

func LoadFlowState(root string, s Sprint) (FlowState, error) {
	if state, found, err := loadFlowStateDatabase(root, s); err != nil {
		return FlowState{}, err
	} else if found {
		path, pathErr := FlowStatePath(root, s)
		if pathErr != nil {
			return FlowState{}, pathErr
		}
		if err := ValidateFlowState(root, s, state, path); err != nil {
			return FlowState{}, err
		}
		return state, nil
	}
	path, err := FlowStatePath(root, s)
	if err != nil {
		return FlowState{}, err
	}
	content, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return FlowState{}, fmt.Errorf("%w: %s", ErrFlowStateMissing, path)
		}
		return FlowState{}, fmt.Errorf("read flow state %s: %w", path, err)
	}
	var header struct {
		SchemaVersion int `json:"schemaVersion"`
	}
	if err := json.Unmarshal(content, &header); err != nil {
		return FlowState{}, fmt.Errorf("%w: %s: %w", ErrFlowStateMalformed, path, err)
	}
	if header.SchemaVersion == 0 {
		return FlowState{}, fmt.Errorf("%w: %s: missing schemaVersion", ErrFlowStateMalformed, path)
	}
	if header.SchemaVersion != FlowStateSchemaVersion && header.SchemaVersion != PreviousFlowStateSchemaVersion {
		return FlowState{}, fmt.Errorf("%w: %s: schemaVersion %d; restore version %d or regenerate state", ErrFlowStateUnsupported, path, header.SchemaVersion, FlowStateSchemaVersion)
	}
	var state FlowState
	dec := json.NewDecoder(bytes.NewReader(content))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&state); err != nil {
		return FlowState{}, fmt.Errorf("%w: %s: %w", ErrFlowStateMalformed, path, err)
	}
	var trailing any
	if err := dec.Decode(&trailing); err == nil {
		return FlowState{}, fmt.Errorf("%w: %s: multiple JSON values", ErrFlowStateMalformed, path)
	} else if !errors.Is(err, io.EOF) {
		return FlowState{}, fmt.Errorf("%w: %s: trailing JSON: %v", ErrFlowStateMalformed, path, err)
	}
	if header.SchemaVersion == PreviousFlowStateSchemaVersion {
		state = migrateFlowStateV1(root, s, state)
		if err := ValidateFlowState(root, s, state, path); err != nil {
			return FlowState{}, err
		}
	}
	if header.SchemaVersion == FlowStateSchemaVersion && isPreCodeContextStages(state.Stages) {
		state.Stages = interpretPreCodeContextStages(s, state.Stages)
	}
	if err := ValidateFlowState(root, s, state, path); err != nil {
		return FlowState{}, err
	}
	return state, nil
}

func isPreCodeContextStages(stages []StageState) bool {
	if len(stages) != len(PlanningStages())-1 {
		return false
	}
	legacy := []PlanningStage{StageRequirements, StageSprintIndex, StageTechnicalHandbook, StageAreaReasoning, StageReasoning, StagePlan}
	for i := range legacy {
		if stages[i].Stage != legacy[i] {
			return false
		}
	}
	return true
}

func interpretPreCodeContextStages(sp Sprint, stages []StageState) []StageState {
	out := make([]StageState, 0, len(stages)+1)
	out = append(out, stages[0])
	inserted := StageState{Stage: StageCodeContext, Status: StatusMissing, Path: ArtifactRelPath(sp, StageCodeContext)}
	if stages[0].Status == StatusComplete {
		inserted.Status = StatusReady
		for _, later := range stages[1:] {
			if later.Status != StatusMissing {
				inserted.Status = StatusSkipped
				inserted.LastRunAt = stages[0].LastRunAt
				break
			}
		}
	}
	out = append(out, inserted)
	out = append(out, stages[1:]...)
	return out
}

func preCodeContextFlowState(root string, sp Sprint) bool {
	path, err := FlowStatePath(root, sp)
	if err != nil {
		return false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var state FlowState
	return json.Unmarshal(data, &state) == nil && state.SchemaVersion == FlowStateSchemaVersion && isPreCodeContextStages(state.Stages)
}

// legacyFlowState reports the original versioned map-based planning state.
// It predates durable review/smoke attempt ownership, so recovery has no live
// attempt to reconcile and must preserve the file for compatibility.
func legacyFlowState(root string, s Sprint) bool {
	path, err := FlowStatePath(root, s)
	if err != nil {
		return false
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var header struct {
		SchemaVersion int `json:"schemaVersion"`
		Version       int `json:"version"`
	}
	if err := json.Unmarshal(content, &header); err != nil {
		return false
	}
	return header.SchemaVersion == 0 && header.Version == 1
}

func migrateFlowStateV1(root string, sp Sprint, state FlowState) FlowState {
	state.SchemaVersion = FlowStateSchemaVersion
	if isPreCodeContextStages(state.Stages) {
		state.Stages = interpretPreCodeContextStages(sp, state.Stages)
	}
	if state.Review != nil {
		r := state.Review
		r.Stale = true
		if r.Status == ReviewCompleted {
			digest, _ := hashFile(mustArtifactPath(root, sp, StageReview))
			if digest == "" {
				digest = "legacy-unverifiable"
			}
			if r.Fingerprint == "" {
				r.Fingerprint = "legacy-unverifiable"
			}
			completedAt := derefTime(r.LastRunAt)
			if completedAt.IsZero() {
				completedAt = state.UpdatedAt
			}
			r.ArtifactDigest = digest
			r.LastComplete = &ReviewCompletion{Verdict: r.Verdict, Artifact: r.Path, ArtifactDigest: digest, InputFingerprint: r.Fingerprint, CompletedAt: completedAt}
		}
	}
	if state.Smoke != nil {
		sm := state.Smoke
		sm.Stale = true
		if sm.Status == SmokeCompleted {
			digest, _ := hashFile(mustArtifactPath(root, sp, StageSmoke))
			if digest == "" {
				digest = "legacy-unverifiable"
			}
			completedAt := derefTime(sm.LastRunAt)
			if completedAt.IsZero() {
				completedAt = state.UpdatedAt
			}
			sm.ArtifactDigest = digest
			sm.InputFingerprint = "legacy-unverifiable"
			sm.LastComplete = &SmokeCompletion{Verdict: sm.Verdict, Artifact: sm.Path, ArtifactDigest: digest, InputFingerprint: sm.InputFingerprint, CompletedAt: completedAt, RunID: sm.RunID, EvidenceID: sm.EvidenceID}
		}
	}
	return state
}

func derefTime(value *time.Time) time.Time {
	if value == nil {
		return time.Time{}
	}
	return value.UTC()
}

func SaveFlowState(root string, s Sprint, state FlowState) error {
	// Planning-stage refreshes must not erase the last complete verification
	// evidence. Explicit stage writers load and replace their own record.
	if state.Review == nil || state.Smoke == nil || state.QA == nil {
		if prior, err := LoadFlowState(root, s); err == nil {
			if state.Review == nil {
				state.Review = prior.Review
			}
			if state.Smoke == nil {
				state.Smoke = prior.Smoke
			}
			if state.QA == nil {
				state.QA = prior.QA
			}
		} else if !errors.Is(err, ErrFlowStateMissing) {
			return err
		}
	}
	if authoritative, err := FlowStateInDatabase(root, s); err != nil {
		return err
	} else if authoritative {
		if err := saveFlowStateDatabase(root, s, state); err != nil {
			return err
		}
		if !flowStateCheckpoint(state) {
			return nil
		}
		return saveFlowStateWithHooks(root, s, state, atomicWriteHooks{})
	}
	return saveFlowStateWithHooks(root, s, state, atomicWriteHooks{})
}

func flowStateCheckpoint(state FlowState) bool {
	for _, stage := range state.Stages {
		if stage.Status != StatusComplete && stage.Status != StatusFailed && stage.Status != StatusSkipped {
			return false
		}
	}
	return len(state.Stages) > 0
}

func saveFlowStateWithHooks(root string, s Sprint, state FlowState, hooks atomicWriteHooks) error {
	path, err := FlowStatePath(root, s)
	if err != nil {
		return err
	}
	state.UpdatedAt = time.Now().UTC()
	if err := ValidateFlowState(root, s, state, path); err != nil {
		return err
	}
	content, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal flow state %s: %w", path, err)
	}
	content = append(content, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create flow state directory %s: %w", filepath.Dir(path), err)
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".flow-state.*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary flow state %s: %w", path, err)
	}
	tempPath := temp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tempPath)
		}
	}()
	if _, err := temp.Write(content); err != nil {
		_ = temp.Close()
		return fmt.Errorf("write temporary flow state %s: %w", tempPath, err)
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return fmt.Errorf("flush temporary flow state %s: %w", tempPath, err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close temporary flow state %s: %w", tempPath, err)
	}
	if hooks.BeforeRename != nil {
		if err := hooks.BeforeRename(path); err != nil {
			return fmt.Errorf("prepare flow state rename %s: %w", path, err)
		}
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("rename flow state %s: %w", path, err)
	}
	cleanup = false
	syncDir(filepath.Dir(path))
	return nil
}

func ValidateFlowState(root string, s Sprint, state FlowState, path string) error {
	if state.SchemaVersion == 0 {
		return fmt.Errorf("%w: %s: missing schemaVersion", ErrFlowStateMalformed, path)
	}
	if state.SchemaVersion != FlowStateSchemaVersion {
		return fmt.Errorf("%w: %s: schemaVersion %d", ErrFlowStateUnsupported, path, state.SchemaVersion)
	}
	if state.Project == "" || state.Sprint == "" || state.UpdatedAt.IsZero() {
		return fmt.Errorf("%w: %s: missing required top-level fields", ErrFlowStateMalformed, path)
	}
	if state.Project != s.Project {
		return fmt.Errorf("%w: %s: project mismatch %q", ErrFlowStateMalformed, path, state.Project)
	}
	if state.Sprint != s.Slug {
		return fmt.Errorf("%w: %s: sprint mismatch %q", ErrFlowStateMalformed, path, state.Sprint)
	}
	if len(state.Stages) != len(PlanningStages()) {
		return fmt.Errorf("%w: %s: expected %d stages", ErrFlowStateMalformed, path, len(PlanningStages()))
	}
	seen := map[PlanningStage]bool{}
	for i, stage := range state.Stages {
		if stage.Stage != PlanningStages()[i] {
			return fmt.Errorf("%w: %s: stage %d is %q; expected %q", ErrFlowStateMalformed, path, i, stage.Stage, PlanningStages()[i])
		}
		if !ValidStage(stage.Stage) {
			return fmt.Errorf("%w: %s: unsupported stage %q", ErrFlowStateMalformed, path, stage.Stage)
		}
		if seen[stage.Stage] {
			return fmt.Errorf("%w: %s: duplicate stage %q", ErrFlowStateMalformed, path, stage.Stage)
		}
		seen[stage.Stage] = true
		if !ValidStatus(stage.Status) {
			return fmt.Errorf("%w: %s: stage %q has unsupported status %q", ErrFlowStateMalformed, path, stage.Stage, stage.Status)
		}
		if stage.Path == "" {
			return fmt.Errorf("%w: %s: stage %d missing path", ErrFlowStateMalformed, path, i)
		}
		if filepath.IsAbs(stage.Path) || strings.HasPrefix(filepath.ToSlash(filepath.Clean(filepath.FromSlash(stage.Path))), "../") {
			return fmt.Errorf("%w: %s: stage %q has unsafe path", ErrFlowStateMalformed, path, stage.Stage)
		}
		if _, err := resolveSprintContained(root, s, stage.Path); err != nil {
			return fmt.Errorf("%w: %s: stage %q has unsafe path: %w", ErrFlowStateMalformed, path, stage.Stage, err)
		}
		if strings.ContainsAny(stage.Error, "\x00\r\n") {
			return fmt.Errorf("%w: %s: stage %q has unsafe error detail", ErrFlowStateMalformed, path, stage.Stage)
		}
		switch stage.LatestOutcome {
		case "", "failed", "cancelled", "interrupted", "cleanup_uncertain":
		default:
			return fmt.Errorf("%w: %s: stage %q has unsupported latest outcome %q", ErrFlowStateMalformed, path, stage.Stage, stage.LatestOutcome)
		}
	}
	for _, expected := range PlanningStages() {
		if !seen[expected] {
			return fmt.Errorf("%w: %s: missing stage %q", ErrFlowStateMalformed, path, expected)
		}
	}
	if state.Review != nil {
		if err := validateReviewStageState(root, s, *state.Review, path); err != nil {
			return err
		}
	}
	if state.Smoke != nil {
		if err := validateSmokeStageState(root, s, *state.Smoke, path); err != nil {
			return err
		}
	}
	if state.QA != nil {
		if err := validateQAFlowSummary(root, s, *state.QA, path); err != nil {
			return err
		}
	}
	return nil
}

func validateQAFlowSummary(root string, s Sprint, summary QAFlowSummary, path string) error {
	if !containsQAPhase(summary.Phase) || summary.CompletedShards < 0 || summary.TotalShards < 0 || summary.CompletedShards > summary.TotalShards || summary.Confirmed < 0 || summary.Blocked < 0 {
		return fmt.Errorf("%w: %s: invalid QA summary", ErrFlowStateMalformed, path)
	}
	if summary.StatePath == "" || !validFingerprint(summary.StateDigest) || strings.TrimSpace(summary.NextAction) == "" {
		return fmt.Errorf("%w: %s: incomplete QA summary pointer", ErrFlowStateMalformed, path)
	}
	resolved, err := resolveSprintContained(root, s, summary.StatePath)
	if err != nil || filepath.Base(resolved) != "state.json" || filepath.Base(filepath.Dir(resolved)) != "verification" {
		return fmt.Errorf("%w: %s: unsafe QA summary pointer", ErrFlowStateMalformed, path)
	}
	if summary.CurrentAttemptID != "" && !validQAID(summary.CurrentAttemptID) {
		return fmt.Errorf("%w: %s: invalid QA summary attempt ID", ErrFlowStateMalformed, path)
	}
	return nil
}

func NewFlowState(s Sprint, stages []StageState, now time.Time) FlowState {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	return FlowState{
		SchemaVersion: FlowStateSchemaVersion,
		Project:       s.Project,
		Sprint:        s.Slug,
		UpdatedAt:     now.UTC(),
		Stages:        stages,
	}
}

func syncDir(path string) {
	dir, err := os.Open(path)
	if err != nil {
		return
	}
	defer dir.Close()
	_ = dir.Sync()
}
