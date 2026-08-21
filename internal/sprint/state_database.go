package sprint

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/Antonio7098/ultraplan-go/internal/productstate"
)

const (
	sprintFlowStateKind    = "sprint_flow"
	sprintExecuteStateKind = "sprint_execute"
)

func sprintStateScope(s Sprint) string { return s.Project + "/" + s.Slug }

func loadFlowStateDatabase(root string, s Sprint) (FlowState, bool, error) {
	record, found, err := loadSprintRecord(root, sprintFlowStateKind, sprintStateScope(s))
	if err != nil || !found {
		return FlowState{}, found, err
	}
	var state FlowState
	if err := json.Unmarshal(record.Header, &state); err != nil {
		return FlowState{}, false, err
	}
	for _, item := range record.Items {
		var stage StageState
		if err := json.Unmarshal(item.Payload, &stage); err != nil {
			return FlowState{}, false, err
		}
		state.Stages = append(state.Stages, stage)
	}
	return state, true, nil
}

func loadExecuteStateDatabase(root string, s Sprint) (ExecuteRunState, bool, error) {
	record, found, err := loadSprintRecord(root, sprintExecuteStateKind, sprintStateScope(s))
	if err != nil || !found {
		return ExecuteRunState{}, found, err
	}
	var state ExecuteRunState
	if err := json.Unmarshal(record.Header, &state); err != nil {
		return ExecuteRunState{}, false, err
	}
	for _, item := range record.Items {
		var task ExecuteTaskRecord
		if err := json.Unmarshal(item.Payload, &task); err != nil {
			return ExecuteRunState{}, false, err
		}
		state.Tasks = append(state.Tasks, task)
	}
	return state, true, nil
}

func loadSprintRecord(root, kind, scope string) (productstate.Record, bool, error) {
	store, enabled, err := productstate.Existing(root)
	if err != nil || !enabled {
		return productstate.Record{}, false, err
	}
	record, err := store.Load(context.Background(), kind, scope)
	if errors.Is(err, productstate.ErrNotFound) {
		return productstate.Record{}, false, nil
	}
	return record, err == nil, err
}

func saveFlowStateDatabase(root string, s Sprint, state FlowState) error {
	state.UpdatedAt = time.Now().UTC()
	path, err := FlowStatePath(root, s)
	if err != nil {
		return err
	}
	if err := ValidateFlowState(root, s, state, path); err != nil {
		return err
	}
	header := state
	header.Stages = nil
	headerJSON, err := json.Marshal(header)
	if err != nil {
		return err
	}
	items := make([]productstate.Item, 0, len(state.Stages))
	for index, stage := range state.Stages {
		payload, err := json.Marshal(stage)
		if err != nil {
			return err
		}
		items = append(items, productstate.Item{Key: string(stage.Stage), Ordinal: index, Payload: payload})
	}
	store, err := productstate.Ensure(root)
	if err != nil {
		return err
	}
	return store.Save(context.Background(), productstate.Record{Kind: sprintFlowStateKind, Scope: sprintStateScope(s), SchemaVersion: state.SchemaVersion, Header: headerJSON, Items: items})
}

func saveExecuteStateDatabase(root string, s Sprint, state ExecuteRunState) error {
	state.UpdatedAt = time.Now().UTC()
	path, err := ExecuteRunStatePath(root, s)
	if err != nil {
		return err
	}
	if err := ValidateExecuteRunState(root, s, state, path); err != nil {
		return err
	}
	header := state
	header.Tasks = nil
	headerJSON, err := json.Marshal(header)
	if err != nil {
		return err
	}
	items := make([]productstate.Item, 0, len(state.Tasks))
	for index, task := range state.Tasks {
		payload, err := json.Marshal(task)
		if err != nil {
			return err
		}
		items = append(items, productstate.Item{Key: task.ID, Ordinal: index, Payload: payload})
	}
	store, err := productstate.Ensure(root)
	if err != nil {
		return err
	}
	return store.Save(context.Background(), productstate.Record{Kind: sprintExecuteStateKind, Scope: sprintStateScope(s), SchemaVersion: state.SchemaVersion, Header: headerJSON, Items: items})
}

func SprintStateInDatabase(root string, s Sprint, kind string) (bool, error) {
	store, enabled, err := productstate.Existing(root)
	if err != nil || !enabled {
		return false, err
	}
	return store.Has(context.Background(), kind, sprintStateScope(s))
}

func FlowStateInDatabase(root string, s Sprint) (bool, error) {
	return SprintStateInDatabase(root, s, sprintFlowStateKind)
}
func ExecuteStateInDatabase(root string, s Sprint) (bool, error) {
	return SprintStateInDatabase(root, s, sprintExecuteStateKind)
}
func MigrateFlowStateToDatabase(root string, s Sprint, state FlowState) error {
	return saveFlowStateDatabase(root, s, state)
}
func MigrateExecuteStateToDatabase(root string, s Sprint, state ExecuteRunState) error {
	return saveExecuteStateDatabase(root, s, state)
}
