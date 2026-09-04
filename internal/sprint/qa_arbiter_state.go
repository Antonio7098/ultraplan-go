package sprint

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func validateRetainedQAArbiterGroup(group QAArbiterGroup) error {
	if !safeQAName(group.ID) || len(group.TheoryIDs) == 0 || group.Round < 1 {
		return fmt.Errorf("retained arbiter group identity is invalid")
	}
	if strings.TrimSpace(group.SessionID) == "" || strings.TrimSpace(group.Provider) == "" || strings.TrimSpace(group.Model) == "" || strings.TrimSpace(group.RuntimeStoreRef) == "" || !validFingerprint(group.WorkspaceID) {
		return fmt.Errorf("retained arbiter runtime identity is incomplete")
	}
	if len(normalizeQAStrings(group.TheoryIDs)) != len(group.TheoryIDs) {
		return fmt.Errorf("retained arbiter theory identity is invalid")
	}
	return nil
}

func (store QAStore) PublishArbiterSessionGroups(attemptID string, groups []QAArbiterGroup, token QAWriterToken) error {
	if err := store.checkWriter(token); err != nil {
		return err
	}
	if !validQAIDKind(attemptID, "attempt") {
		return NewQAError(QAErrorInvalidState, "publish arbiter sessions", "invalid attempt identity", nil)
	}
	seen := map[string]bool{}
	for _, group := range groups {
		if err := validateRetainedQAArbiterGroup(group); err != nil || seen[group.ID] {
			return NewQAError(QAErrorInvalidState, "publish arbiter sessions", "invalid or duplicate retained arbiter group", err)
		}
		seen[group.ID] = true
		path, err := store.resolve(QAArbiterSessionRoundRelPath(store.sprint, attemptID, group.ID, group.Round))
		if err != nil {
			return err
		}
		envelope := struct {
			SchemaVersion int            `json:"schema_version"`
			AttemptID     string         `json:"attempt_id"`
			Group         QAArbiterGroup `json:"group"`
		}{QAEvidenceSchemaVersion, attemptID, group}
		if _, err := store.writeRecord("arbiter-session-round", path, &envelope, true); err != nil {
			return err
		}
	}
	return nil
}

func (store QAStore) LoadLatestArbiterSessionGroups(attemptID string) ([]QAArbiterGroup, error) {
	if !validQAIDKind(attemptID, "attempt") {
		return nil, NewQAError(QAErrorInvalidState, "load arbiter sessions", "invalid attempt identity", nil)
	}
	root, err := store.resolve(filepath.ToSlash(filepath.Join("projects", store.sprint.Project, "sprints", store.sprint.Slug, "verification", "attempts", attemptID, "arbiter-sessions")))
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(root)
	if errors.Is(err, fs.ErrNotExist) {
		return []QAArbiterGroup{}, nil
	}
	if err != nil {
		return nil, NewQAError(QAErrorPersistenceFailure, "load arbiter sessions", "cannot list retained arbiter sessions", err)
	}
	groups := make([]QAArbiterGroup, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || !safeQAName(entry.Name()) {
			return nil, NewQAError(QAErrorInvalidState, "load arbiter sessions", "arbiter session storage contains an invalid group", nil)
		}
		roundRoot := filepath.Join(root, entry.Name())
		rounds, readErr := os.ReadDir(roundRoot)
		if readErr != nil || len(rounds) == 0 {
			return nil, NewQAError(QAErrorPersistenceFailure, "load arbiter sessions", "retained arbiter group has no readable rounds", readErr)
		}
		var latest QAArbiterGroup
		for _, roundEntry := range rounds {
			if roundEntry.IsDir() || filepath.Ext(roundEntry.Name()) != ".json" {
				return nil, NewQAError(QAErrorInvalidState, "load arbiter sessions", "arbiter session storage contains an invalid round", nil)
			}
			path := filepath.Join(roundRoot, roundEntry.Name())
			var envelope struct {
				SchemaVersion int            `json:"schema_version"`
				AttemptID     string         `json:"attempt_id"`
				Group         QAArbiterGroup `json:"group"`
			}
			if err := store.readStrictVersion(path, "arbiter-session-round", QAEvidenceSchemaVersion, &envelope); err != nil {
				return nil, err
			}
			if envelope.AttemptID != attemptID || envelope.Group.ID != entry.Name() || roundEntry.Name() != fmt.Sprintf("%06d.json", envelope.Group.Round) {
				return nil, NewQAError(QAErrorInvalidState, "load arbiter sessions", "arbiter session round identity does not match its path", nil)
			}
			if err := validateRetainedQAArbiterGroup(envelope.Group); err != nil {
				return nil, NewQAError(QAErrorInvalidState, "load arbiter sessions", err.Error(), err)
			}
			if envelope.Group.Round > latest.Round {
				latest = envelope.Group
			}
		}
		groups = append(groups, latest)
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].ID < groups[j].ID })
	return groups, nil
}
