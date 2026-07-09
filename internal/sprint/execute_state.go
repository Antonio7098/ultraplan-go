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

func ExecuteRunStatePath(root string, s Sprint) (string, error) {
	return resolveSprintContained(root, s, ExecuteRunStateRelPath(s))
}

func NewExecuteRunState(s Sprint, target ExecuteTargetRef, planPath, planFingerprint string, tasks []ExecuteTaskRecord, now time.Time) ExecuteRunState {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	return ExecuteRunState{
		SchemaVersion:   ExecuteRunStateSchemaVersion,
		Project:         s.Project,
		Sprint:          s.Slug,
		Target:          target,
		PlanPath:        filepath.ToSlash(planPath),
		PlanFingerprint: planFingerprint,
		CreatedAt:       now.UTC(),
		UpdatedAt:       now.UTC(),
		Tasks:           tasks,
	}
}

func LoadExecuteRunState(root string, s Sprint) (ExecuteRunState, error) {
	path, err := ExecuteRunStatePath(root, s)
	if err != nil {
		return ExecuteRunState{}, err
	}
	content, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return ExecuteRunState{}, fmt.Errorf("%w: %s", ErrExecuteRunStateMissing, path)
		}
		return ExecuteRunState{}, fmt.Errorf("read execute run state %s: %w", path, err)
	}
	var state ExecuteRunState
	if err := json.Unmarshal(content, &state); err != nil {
		return ExecuteRunState{}, fmt.Errorf("%w: %s: %w", ErrExecuteRunStateMalformed, path, err)
	}
	if err := ValidateExecuteRunState(root, s, state, path); err != nil {
		return ExecuteRunState{}, err
	}
	return state, nil
}

func SaveExecuteRunState(root string, s Sprint, state ExecuteRunState) error {
	return saveExecuteRunStateWithHooks(root, s, state, atomicWriteHooks{})
}

func saveExecuteRunStateWithHooks(root string, s Sprint, state ExecuteRunState, hooks atomicWriteHooks) error {
	path, err := ExecuteRunStatePath(root, s)
	if err != nil {
		return err
	}
	state.UpdatedAt = time.Now().UTC()
	if err := ValidateExecuteRunState(root, s, state, path); err != nil {
		return err
	}
	content, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal execute run state %s: %w", path, err)
	}
	content = append(content, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create execute run state directory %s: %w", filepath.Dir(path), err)
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".run-state.*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary execute run state %s: %w", path, err)
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
		return fmt.Errorf("write temporary execute run state %s: %w", tempPath, err)
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return fmt.Errorf("flush temporary execute run state %s: %w", tempPath, err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close temporary execute run state %s: %w", tempPath, err)
	}
	if hooks.BeforeRename != nil {
		if err := hooks.BeforeRename(path); err != nil {
			return fmt.Errorf("prepare execute run state rename %s: %w", path, err)
		}
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("rename execute run state %s: %w", path, err)
	}
	cleanup = false
	syncDir(filepath.Dir(path))
	return nil
}

func ValidateExecuteRunState(root string, s Sprint, state ExecuteRunState, path string) error {
	if state.SchemaVersion == 0 {
		return fmt.Errorf("%w: %s: missing schemaVersion", ErrExecuteRunStateMalformed, path)
	}
	if state.SchemaVersion != ExecuteRunStateSchemaVersion {
		return fmt.Errorf("%w: %s: schemaVersion %d", ErrExecuteRunStateUnsupported, path, state.SchemaVersion)
	}
	if state.Project == "" || state.Sprint == "" || state.Target.Path == "" || state.Target.Source == "" || state.PlanPath == "" || state.PlanFingerprint == "" || state.CreatedAt.IsZero() || state.UpdatedAt.IsZero() {
		return fmt.Errorf("%w: %s: missing required top-level fields", ErrExecuteRunStateMalformed, path)
	}
	if state.Project != s.Project {
		return fmt.Errorf("%w: %s: project mismatch %q", ErrExecuteRunStateMalformed, path, state.Project)
	}
	if state.Sprint != s.Slug {
		return fmt.Errorf("%w: %s: sprint mismatch %q", ErrExecuteRunStateMalformed, path, state.Sprint)
	}
	if !safeRelPath(state.PlanPath) {
		return fmt.Errorf("%w: %s: unsafe planPath", ErrExecuteRunStateMalformed, path)
	}
	if _, err := resolveSprintContained(root, s, state.PlanPath); err != nil {
		return fmt.Errorf("%w: %s: unsafe planPath: %w", ErrExecuteRunStateMalformed, path, err)
	}
	if strings.ContainsAny(state.PlanFingerprint, "\x00\r\n") {
		return fmt.Errorf("%w: %s: unsafe planFingerprint", ErrExecuteRunStateMalformed, path)
	}
	if len(state.Tasks) == 0 {
		return fmt.Errorf("%w: %s: missing tasks", ErrExecuteRunStateMalformed, path)
	}
	seen := map[string]bool{}
	for i, task := range state.Tasks {
		if task.ID == "" || task.Identity.Name == "" || task.CreatedAt.IsZero() || task.UpdatedAt.IsZero() {
			return fmt.Errorf("%w: %s: task %d missing required fields", ErrExecuteRunStateMalformed, path, i)
		}
		if seen[task.ID] {
			return fmt.Errorf("%w: %s: duplicate task id %q", ErrExecuteRunStateMalformed, path, task.ID)
		}
		seen[task.ID] = true
		if strings.ContainsAny(task.ID+task.Identity.Name, "\x00\r\n") {
			return fmt.Errorf("%w: %s: task %d has unsafe identity", ErrExecuteRunStateMalformed, path, i)
		}
		if task.Identity.PlanLine < 1 {
			return fmt.Errorf("%w: %s: task %q has invalid planLine", ErrExecuteRunStateMalformed, path, task.ID)
		}
		if !ValidExecuteTaskStatus(task.Status) {
			return fmt.Errorf("%w: %s: task %q has unsupported status %q", ErrExecuteRunStateMalformed, path, task.ID, task.Status)
		}
		if task.Attempts < 0 {
			return fmt.Errorf("%w: %s: task %q has invalid attempts", ErrExecuteRunStateMalformed, path, task.ID)
		}
		if task.Status == ExecuteTaskRunning && task.StartedAt == nil {
			return fmt.Errorf("%w: %s: running task %q missing startedAt", ErrExecuteRunStateMalformed, path, task.ID)
		}
		if isTerminalExecuteStatus(task.Status) && task.CompletedAt == nil {
			return fmt.Errorf("%w: %s: terminal task %q missing completedAt", ErrExecuteRunStateMalformed, path, task.ID)
		}
		for j, diagnostic := range task.Diagnostics {
			if diagnostic.Code == "" || diagnostic.Message == "" || diagnostic.At.IsZero() || strings.ContainsAny(diagnostic.Code+diagnostic.Message, "\x00\r\n") {
				return fmt.Errorf("%w: %s: task %q diagnostic %d invalid", ErrExecuteRunStateMalformed, path, task.ID, j)
			}
		}
		for j, evidence := range task.Evidence {
			if evidence.Kind == "" || evidence.Summary == "" || strings.ContainsAny(evidence.Kind+evidence.Summary, "\x00\r\n") {
				return fmt.Errorf("%w: %s: task %q evidence %d invalid", ErrExecuteRunStateMalformed, path, task.ID, j)
			}
			if evidence.Path != "" && !safeRelPath(evidence.Path) {
				return fmt.Errorf("%w: %s: task %q evidence %d has unsafe path", ErrExecuteRunStateMalformed, path, task.ID, j)
			}
		}
	}
	return nil
}

func isTerminalExecuteStatus(status ExecuteTaskStatus) bool {
	return status == ExecuteTaskComplete || status == ExecuteTaskFailed || status == ExecuteTaskCancelled
}

func safeRelPath(path string) bool {
	if path == "" || filepath.IsAbs(path) {
		return false
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(path)))
	return clean != "." && !strings.HasPrefix(clean, "../") && !strings.Contains(clean, "\x00")
}
