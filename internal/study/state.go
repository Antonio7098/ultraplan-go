package study

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

const legacyRunStateGCThreshold = 64 * 1024 * 1024

var (
	ErrRunStateMissing     = errors.New("run state missing")
	ErrRunStateMalformed   = errors.New("run state malformed")
	ErrRunStateUnsupported = errors.New("run state unsupported")
)

type atomicWriteHooks struct {
	BeforeRename func(path string) error
}

func LoadRunState(study Study) (RunState, error) {
	path := RunStatePath(study)
	content, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return RunState{}, fmt.Errorf("%w: %s", ErrRunStateMissing, path)
		}
		return RunState{}, fmt.Errorf("read run state %s: %w", path, err)
	}
	var state RunState
	if err := json.Unmarshal(content, &state); err != nil {
		return RunState{}, fmt.Errorf("%w: %s: %w", ErrRunStateMalformed, path, err)
	}
	if err := ValidateRunState(state, path); err != nil {
		return RunState{}, err
	}
	compactRunStateDiagnostics(&state)
	if len(content) >= legacyRunStateGCThreshold {
		content = nil
		runtime.GC()
	}
	return state, nil
}

func SaveRunState(study Study, state RunState) error {
	return saveRunStateWithHooks(study, state, atomicWriteHooks{})
}

func saveRunStateWithHooks(study Study, state RunState, hooks atomicWriteHooks) error {
	path := RunStatePath(study)
	state = cloneRunState(state)
	state.UpdatedAt = time.Now().UTC()
	compactRunStateDiagnostics(&state)
	if err := ValidateRunState(state, path); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create run state directory %s: %w", filepath.Dir(path), err)
	}
	temp, err := os.CreateTemp(filepath.Dir(path), "."+RunStateFileName+".*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary run state %s: %w", path, err)
	}
	tempPath := temp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tempPath)
		}
	}()
	encoder := json.NewEncoder(temp)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(state); err != nil {
		_ = temp.Close()
		return fmt.Errorf("encode temporary run state %s: %w", tempPath, err)
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return fmt.Errorf("flush temporary run state %s: %w", tempPath, err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close temporary run state %s: %w", tempPath, err)
	}
	if hooks.BeforeRename != nil {
		if err := hooks.BeforeRename(path); err != nil {
			return fmt.Errorf("prepare run state rename %s: %w", path, err)
		}
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("rename run state %s: %w", path, err)
	}
	cleanup = false
	syncDir(filepath.Dir(path))
	return nil
}

func ValidateRunState(state RunState, path string) error {
	if state.SchemaVersion == 0 {
		return fmt.Errorf("%w: %s: missing schema_version", ErrRunStateMalformed, path)
	}
	if state.SchemaVersion != RunStateSchemaVersion {
		return fmt.Errorf("%w: %s: schema_version %d", ErrRunStateUnsupported, path, state.SchemaVersion)
	}
	if state.RunID == "" {
		return fmt.Errorf("%w: %s: missing run_id", ErrRunStateMalformed, path)
	}
	if state.Study == "" {
		return fmt.Errorf("%w: %s: missing study", ErrRunStateMalformed, path)
	}
	if state.CreatedAt.IsZero() || state.UpdatedAt.IsZero() {
		return fmt.Errorf("%w: %s: missing timestamps", ErrRunStateMalformed, path)
	}
	ids := map[string]struct{}{}
	for i, task := range state.Tasks {
		if task.ID == "" {
			return fmt.Errorf("%w: %s: task %d missing id", ErrRunStateMalformed, path, i)
		}
		if _, exists := ids[task.ID]; exists {
			return fmt.Errorf("%w: %s: duplicate task id %q", ErrRunStateMalformed, path, task.ID)
		}
		ids[task.ID] = struct{}{}
		if task.Kind != TaskKindAnalysis && task.Kind != TaskKindSynthesis {
			return fmt.Errorf("%w: %s: task %q has unsupported kind %q", ErrRunStateMalformed, path, task.ID, task.Kind)
		}
		if !validTaskStatus(task.Status) {
			return fmt.Errorf("%w: %s: task %q has unsupported status %q", ErrRunStateMalformed, path, task.ID, task.Status)
		}
		if task.Study == "" || task.OutputPath == "" || task.CreatedAt.IsZero() || task.UpdatedAt.IsZero() {
			return fmt.Errorf("%w: %s: task %q missing required fields", ErrRunStateMalformed, path, task.ID)
		}
		if task.Kind == TaskKindAnalysis && (task.Dimension == "" || task.Source == "" || task.SourceKind == "") {
			return fmt.Errorf("%w: %s: analysis task %q missing metadata", ErrRunStateMalformed, path, task.ID)
		}
	}
	return nil
}

func validTaskStatus(status TaskStatus) bool {
	switch status {
	case TaskStatusPending, TaskStatusRunning, TaskStatusValidating, TaskStatusCompleted, TaskStatusFailed, TaskStatusCancelled, TaskStatusSkipped, TaskStatusWaiting, TaskStatusRetrying:
		return true
	default:
		return false
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
