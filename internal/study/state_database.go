package study

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"time"

	"github.com/Antonio7098/ultraplan-go/internal/productstate"
)

const studyRunStateKind = "study_run"

func studyWorkspaceRoot(study Study) string { return filepath.Dir(filepath.Dir(study.Path)) }

func loadRunStateDatabase(study Study) (RunState, bool, error) {
	store, enabled, err := productstate.Existing(studyWorkspaceRoot(study))
	if err != nil || !enabled {
		return RunState{}, false, err
	}
	record, err := store.Load(context.Background(), studyRunStateKind, study.Name)
	if errors.Is(err, productstate.ErrNotFound) {
		return RunState{}, false, nil
	}
	if err != nil {
		return RunState{}, false, err
	}
	var state RunState
	if err := json.Unmarshal(record.Header, &state); err != nil {
		return RunState{}, false, err
	}
	for _, item := range record.Items {
		var task TaskState
		if err := json.Unmarshal(item.Payload, &task); err != nil {
			return RunState{}, false, err
		}
		state.Tasks = append(state.Tasks, task)
	}
	return state, true, nil
}

func saveRunStateDatabase(study Study, state RunState) error {
	state.UpdatedAt = time.Now().UTC()
	compactRunStateDiagnostics(&state)
	if err := ValidateRunState(state, RunStatePath(study)); err != nil {
		return err
	}
	header := cloneRunState(state)
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
	store, err := productstate.Ensure(studyWorkspaceRoot(study))
	if err != nil {
		return err
	}
	return store.Save(context.Background(), productstate.Record{Kind: studyRunStateKind, Scope: study.Name, SchemaVersion: state.SchemaVersion, Header: headerJSON, Items: items})
}

func RunStateInDatabase(study Study) (bool, error) {
	store, enabled, err := productstate.Existing(studyWorkspaceRoot(study))
	if err != nil || !enabled {
		return false, err
	}
	return store.Has(context.Background(), studyRunStateKind, study.Name)
}

func MigrateRunStateToDatabase(study Study, state RunState) error {
	return saveRunStateDatabase(study, state)
}
