package sprint

import (
	"encoding/json"
	"errors"
	"fmt"
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
	var state FlowState
	if err := json.Unmarshal(content, &state); err != nil {
		return FlowState{}, fmt.Errorf("%w: %s: %w", ErrFlowStateMalformed, path, err)
	}
	if err := ValidateFlowState(root, s, state, path); err != nil {
		return FlowState{}, err
	}
	return state, nil
}

func SaveFlowState(root string, s Sprint, state FlowState) error {
	return saveFlowStateWithHooks(root, s, state, atomicWriteHooks{})
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
	}
	for _, expected := range PlanningStages() {
		if !seen[expected] {
			return fmt.Errorf("%w: %s: missing stage %q", ErrFlowStateMalformed, path, expected)
		}
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
