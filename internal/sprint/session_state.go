package sprint

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	pruntime "github.com/Antonio7098/ultraplan-go/internal/platform/runtime"
)

const stageSessionSchemaVersion = 1

type stageSessionRecord struct {
	SessionID      string    `json:"sessionId"`
	Provider       string    `json:"provider,omitempty"`
	Model          string    `json:"model,omitempty"`
	WorkDir        string    `json:"workDir"`
	PromptChecksum string    `json:"promptChecksum,omitempty"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

type stageSessionState struct {
	SchemaVersion int                           `json:"schemaVersion"`
	Sessions      map[string]stageSessionRecord `json:"sessions"`
}

func stageSessionPath(sp Sprint) string { return filepath.Join(sp.Path, ".stage-sessions.json") }

func loadStageSessions(sp Sprint) (stageSessionState, error) {
	path := stageSessionPath(sp)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return stageSessionState{SchemaVersion: stageSessionSchemaVersion, Sessions: map[string]stageSessionRecord{}}, nil
	}
	if err != nil {
		return stageSessionState{}, err
	}
	var state stageSessionState
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&state); err != nil {
		return stageSessionState{}, fmt.Errorf("decode stage sessions: %w", err)
	}
	var trailing any
	if err := dec.Decode(&trailing); !errors.Is(err, io.EOF) {
		return stageSessionState{}, fmt.Errorf("decode stage sessions: trailing JSON")
	}
	if state.SchemaVersion != stageSessionSchemaVersion || state.Sessions == nil {
		return stageSessionState{}, fmt.Errorf("unsupported stage session state")
	}
	return state, nil
}

func saveStageSessions(sp Sprint, state stageSessionState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	path := stageSessionPath(sp)
	tmp, err := os.CreateTemp(sp.Path, ".stage-sessions.*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err = tmp.Write(data); err == nil {
		err = tmp.Chmod(0o600)
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func stageSessionCompatible(record stageSessionRecord, req pruntime.Request) bool {
	// Prompt checksums are retained for diagnostics only. Exact prompt matching
	// would make interrupted-session recovery brittle after harmless artifact
	// edits; the continuation explicitly tells the agent to reread current state.
	return record.SessionID != "" && record.Provider == req.Provider && record.Model == req.Model && record.WorkDir == req.WorkDir
}

func stageSessionKey(stage PlanningStage, req pruntime.Request) string {
	parts := []string{string(stage)}
	for _, dimension := range []string{"task", "coverage", "area"} {
		parts = append(parts, strings.TrimSpace(req.Metadata[dimension]))
	}
	if parts[1] == "" && parts[2] == "" && parts[3] == "" {
		return parts[0]
	}
	return strings.Join(parts, ":")
}

func (s Service) startPlanningStageRun(ctx context.Context, sp Sprint, stage PlanningStage, req pruntime.Request) (pruntime.Result, error) {
	key := stageSessionKey(stage, req)
	originalPrompt := req.Prompt
	continuing := false
	state, loadErr := loadStageSessions(sp)
	if loadErr == nil {
		if record, ok := state.Sessions[key]; ok && stageSessionCompatible(record, req) {
			req.SessionID = record.SessionID
			req.SessionAction = "continue"
			req.Prompt = insertStageContinuation(req.Prompt, "Continue the interrupted UltraPlan stage from the existing session. Re-read the current stage prompt and filesystem state, then finish only the requested stage.")
			continuing = true
		}
	}
	previousOnEvent := req.OnEvent
	var mu sync.Mutex
	lastSession := req.SessionID
	persist := func(sessionID string) {
		if strings.TrimSpace(sessionID) == "" {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		if sessionID == lastSession {
			return
		}
		lastSession = sessionID
		current, err := loadStageSessions(sp)
		if err != nil {
			return
		}
		current.Sessions[key] = stageSessionRecord{SessionID: sessionID, Provider: req.Provider, Model: req.Model, WorkDir: req.WorkDir, PromptChecksum: req.PromptRef.Checksum, UpdatedAt: s.now().UTC()}
		_ = saveStageSessions(sp, current)
	}
	req.OnEvent = func(event pruntime.Event) {
		if previousOnEvent != nil {
			previousOnEvent(event)
		}
		persist(event.SessionID)
	}
	result, err := s.startSprintRuntime(ctx, sp, stage, req)
	persist(result.SessionID)
	if continuing && planningSessionNotFound(result, err) && ctx.Err() == nil {
		_ = clearPlanningStageSessionKey(sp, key)
		lastSession = ""
		req.SessionID = ""
		req.SessionAction = "fresh"
		req.Prompt = originalPrompt
		result, err = s.startSprintRuntime(ctx, sp, stage, req)
		persist(result.SessionID)
	}
	return result, err
}

func planningSessionNotFound(result pruntime.Result, err error) bool {
	var details []string
	if err != nil {
		details = append(details, err.Error())
	}
	details = append(details, result.TerminalOutput)
	if result.Error != nil {
		details = append(details, result.Error.UserDetail, result.Error.DebugDetail)
	}
	for _, attempt := range result.Attempts {
		details = append(details, attempt.ErrorDetail)
	}
	return strings.Contains(strings.ToLower(strings.Join(details, "\n")), "session not found")
}

func insertStageContinuation(prompt, instruction string) string {
	boundary := strings.Index(prompt, sharedPromptStageBoundary)
	if boundary < 0 {
		return instruction + "\n\n" + prompt
	}
	boundary += len(sharedPromptStageBoundary)
	return prompt[:boundary] + instruction + "\n\n" + prompt[boundary:]
}

func clearPlanningStageSession(sp Sprint, stage PlanningStage) error {
	state, err := loadStageSessions(sp)
	if err != nil {
		return err
	}
	found := false
	for key := range state.Sessions {
		if key == string(stage) || strings.HasPrefix(key, string(stage)+":") {
			delete(state.Sessions, key)
			found = true
		}
	}
	if !found {
		return nil
	}
	return saveOrRemoveStageSessions(sp, state)
}

func clearPlanningStageSessionKey(sp Sprint, key string) error {
	state, err := loadStageSessions(sp)
	if err != nil {
		return err
	}
	if _, ok := state.Sessions[key]; !ok {
		return nil
	}
	delete(state.Sessions, key)
	return saveOrRemoveStageSessions(sp, state)
}

func saveOrRemoveStageSessions(sp Sprint, state stageSessionState) error {
	if len(state.Sessions) == 0 {
		if err := os.Remove(stageSessionPath(sp)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	return saveStageSessions(sp, state)
}

func (s Service) cleanupPlanningStageSession(ctx context.Context, sp Sprint, stage PlanningStage, sessionID string) error {
	if err := s.deleteCompletedSession(ctx, sessionID); err != nil {
		return err
	}
	return clearPlanningStageSession(sp, stage)
}

func (s Service) cleanupPlanningStageSessions(ctx context.Context, sp Sprint, stage PlanningStage, result pruntime.Result) error {
	if err := s.deleteCompletedSessions(ctx, result); err != nil {
		return err
	}
	return clearPlanningStageSession(sp, stage)
}
