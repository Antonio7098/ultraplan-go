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
	"sort"
	"strings"
)

const qaHardStateBytes = 128 << 20

type QAStateHooks struct {
	BeforeStep   func(kind, path string) error
	BeforeRename func(kind, path string) error
}

type QAStore struct {
	root   string
	sprint Sprint
	hooks  QAStateHooks
	fence  func(QAWriterToken) error
}

func NewQAStore(root string, sprint Sprint) QAStore {
	return QAStore{root: root, sprint: sprint}
}

func (store QAStore) WithHooks(hooks QAStateHooks) QAStore {
	store.hooks = hooks
	return store
}

func (store QAStore) WithWriterFence(fence func(QAWriterToken) error) QAStore {
	store.fence = fence
	return store
}

// VerificationBytes returns the retained private QA footprint without
// following symbolic links.
func (store QAStore) VerificationBytes() (int64, error) {
	root, err := store.resolve(filepath.ToSlash(filepath.Join("projects", store.sprint.Project, "sprints", store.sprint.Slug, "verification")))
	if err != nil {
		return 0, err
	}
	var total int64
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if errors.Is(walkErr, fs.ErrNotExist) {
			return nil
		}
		if walkErr != nil {
			return walkErr
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			return infoErr
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return NewQAError(QAErrorInvalidState, "measure state", "QA state contains a symbolic link", nil)
		}
		if info.Mode().IsRegular() {
			total += info.Size()
		}
		return nil
	})
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, NewQAError(QAErrorPersistenceFailure, "measure state", "cannot measure retained QA state", err)
	}
	return total, nil
}

// PruneAttempts keeps the newest bounded set plus the protected current
// semantic attempt. It removes only validated attempt directories.
func (store QAStore) PruneAttempts(protected string, retain int) error {
	if retain <= 0 {
		return NewQAError(QAErrorInvalidState, "prune attempts", "retention limit must be positive", nil)
	}
	root, err := store.resolve(filepath.ToSlash(filepath.Join("projects", store.sprint.Project, "sprints", store.sprint.Slug, "verification", "attempts")))
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(root)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return NewQAError(QAErrorPersistenceFailure, "prune attempts", "cannot list retained attempts", err)
	}
	type candidate struct {
		name string
		path string
		mod  int64
	}
	values := make([]candidate, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || !validQAIDKind(entry.Name(), "attempt") {
			return NewQAError(QAErrorInvalidState, "prune attempts", "attempt storage contains an invalid entry", nil)
		}
		info, infoErr := entry.Info()
		if infoErr != nil || info.Mode()&os.ModeSymlink != 0 {
			return NewQAError(QAErrorInvalidState, "prune attempts", "attempt storage contains an unsafe entry", infoErr)
		}
		values = append(values, candidate{name: entry.Name(), path: filepath.Join(root, entry.Name()), mod: info.ModTime().UnixNano()})
	}
	sort.Slice(values, func(i, j int) bool {
		if values[i].mod == values[j].mod {
			return values[i].name > values[j].name
		}
		return values[i].mod > values[j].mod
	})
	keep := map[string]bool{}
	for _, value := range values {
		if value.name == protected {
			keep[value.name] = true
			break
		}
	}
	for _, value := range values {
		if keep[value.name] || len(keep) < retain {
			keep[value.name] = true
			continue
		}
		if err := os.RemoveAll(value.path); err != nil {
			return NewQAError(QAErrorPersistenceFailure, "prune attempts", "cannot remove an expired QA attempt", err)
		}
	}
	return nil
}

func QAVerificationStateRelPath(s Sprint) string {
	return filepath.ToSlash(filepath.Join("projects", s.Project, "sprints", s.Slug, "verification", "state.json"))
}

func QAMapRelPath(s Sprint, attemptID string) string {
	return filepath.ToSlash(filepath.Join("projects", s.Project, "sprints", s.Slug, "verification", "attempts", attemptID, "map.json"))
}

func QAShardRelPath(s Sprint, attemptID, shardID string) string {
	return filepath.ToSlash(filepath.Join("projects", s.Project, "sprints", s.Slug, "verification", "attempts", attemptID, "shards", shardID+".json"))
}

func QASynthesisRelPath(s Sprint, attemptID string) string {
	return filepath.ToSlash(filepath.Join("projects", s.Project, "sprints", s.Slug, "verification", "attempts", attemptID, "synthesis.json"))
}

func (store QAStore) StatePath() (string, error) {
	return store.resolve(QAVerificationStateRelPath(store.sprint))
}

func (store QAStore) mapPath(attemptID string) (string, error) {
	if !validQAIDKind(attemptID, "attempt") {
		return "", NewQAError(QAErrorInvalidState, "resolve map", "invalid semantic attempt ID", nil)
	}
	return store.resolve(QAMapRelPath(store.sprint, attemptID))
}

func (store QAStore) shardPath(attemptID, shardID string) (string, error) {
	if !validQAIDKind(attemptID, "attempt") || !validQAIDKind(shardID, "shard") {
		return "", NewQAError(QAErrorInvalidState, "resolve shard", "invalid attempt or shard ID", nil)
	}
	return store.resolve(QAShardRelPath(store.sprint, attemptID, shardID))
}

func (store QAStore) synthesisPath(attemptID string) (string, error) {
	if !validQAIDKind(attemptID, "attempt") {
		return "", NewQAError(QAErrorInvalidState, "resolve synthesis", "invalid semantic attempt ID", nil)
	}
	return store.resolve(QASynthesisRelPath(store.sprint, attemptID))
}

func validQAIDKind(value, kind string) bool {
	return validQAID(value) && strings.HasPrefix(value, QAIDScope+"-"+kind+"-")
}

func (store QAStore) resolve(rel string) (string, error) {
	full, err := resolveSprintContained(store.root, store.sprint, rel)
	if err != nil {
		return "", NewQAError(QAErrorInvalidState, "resolve path", "QA path escapes the selected sprint", err)
	}
	sprintRoot, err := resolveSprintContained(store.root, store.sprint, filepath.ToSlash(filepath.Join("projects", store.sprint.Project, "sprints", store.sprint.Slug)))
	if err != nil {
		return "", err
	}
	relToSprint, err := filepath.Rel(sprintRoot, full)
	if err != nil || relToSprint == "." || strings.HasPrefix(relToSprint, "..") {
		return "", NewQAError(QAErrorInvalidState, "resolve path", "QA path is outside the selected sprint", err)
	}
	current := sprintRoot
	for _, part := range strings.Split(filepath.Clean(relToSprint), string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, statErr := os.Lstat(current)
		if errors.Is(statErr, fs.ErrNotExist) {
			continue
		}
		if statErr != nil {
			return "", NewQAError(QAErrorPersistenceFailure, "inspect path", "cannot inspect QA path", statErr)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", NewQAError(QAErrorInvalidState, "resolve path", "QA path contains a symbolic link", nil)
		}
	}
	return full, nil
}

func (store QAStore) LoadState() (QAState, error) {
	path, err := store.StatePath()
	if err != nil {
		return QAState{}, err
	}
	var state QAState
	if err := store.readStrict(path, "state", &state); err != nil {
		return QAState{}, err
	}
	if err := ValidateQAState(state); err != nil {
		return QAState{}, NewQAError(QAErrorInvalidState, "load state", err.Error(), err)
	}
	if state.Project != store.sprint.Project || state.Sprint != store.sprint.Slug {
		return QAState{}, NewQAError(QAErrorInvalidState, "load state", "QA state scope does not match the selected sprint", nil)
	}
	for _, ref := range []*QAArtifactRef{state.Map, state.Synthesis} {
		if ref == nil {
			continue
		}
		if err := store.verifyReference(*ref); err != nil {
			return QAState{}, err
		}
	}
	return state, nil
}

func (store QAStore) LoadMap(attemptID string) (QAMap, error) {
	path, err := store.mapPath(attemptID)
	if err != nil {
		return QAMap{}, err
	}
	var value QAMap
	if err := store.readStrict(path, "map", &value); err != nil {
		return QAMap{}, err
	}
	if err := ValidateQAMap(value); err != nil {
		return QAMap{}, NewQAError(QAErrorInvalidState, "load map", err.Error(), err)
	}
	if value.Project != store.sprint.Project || value.Sprint != store.sprint.Slug || value.SemanticAttemptID != attemptID {
		return QAMap{}, NewQAError(QAErrorInvalidState, "load map", "QA map scope does not match its path", nil)
	}
	return value, nil
}

func (store QAStore) LoadShard(attemptID, shardID string) (QAShard, error) {
	path, err := store.shardPath(attemptID, shardID)
	if err != nil {
		return QAShard{}, err
	}
	var value QAShard
	if err := store.readStrict(path, "shard", &value); err != nil {
		return QAShard{}, err
	}
	if err := ValidateQAShard(value); err != nil {
		return QAShard{}, NewQAError(QAErrorInvalidState, "load shard", err.Error(), err)
	}
	if value.AttemptID != attemptID || value.ID != shardID {
		return QAShard{}, NewQAError(QAErrorInvalidState, "load shard", "QA shard identity does not match its path", nil)
	}
	return value, nil
}

func (store QAStore) LoadSynthesis(attemptID string, budgets QABudgets) (QASynthesis, error) {
	path, err := store.synthesisPath(attemptID)
	if err != nil {
		return QASynthesis{}, err
	}
	var value QASynthesis
	if err := store.readStrict(path, "synthesis", &value); err != nil {
		return QASynthesis{}, err
	}
	if err := ValidateQASynthesis(value, budgets); err != nil {
		return QASynthesis{}, NewQAError(QAErrorInvalidState, "load synthesis", err.Error(), err)
	}
	if value.AttemptID != attemptID {
		return QASynthesis{}, NewQAError(QAErrorInvalidState, "load synthesis", "QA synthesis identity does not match its path", nil)
	}
	return value, nil
}

func (store QAStore) readStrict(path, kind string, value any) error {
	info, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return NewQAError(QAErrorInvalidState, "load "+kind, kind+" is missing", err)
		}
		return NewQAError(QAErrorPersistenceFailure, "load "+kind, "cannot inspect QA file", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		return NewQAError(QAErrorInvalidState, "load "+kind, "QA file must be a regular private 0600 file", nil)
	}
	file, err := os.Open(path)
	if err != nil {
		return NewQAError(QAErrorPersistenceFailure, "load "+kind, "cannot open QA file", err)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, qaHardStateBytes+1))
	if err != nil {
		return NewQAError(QAErrorPersistenceFailure, "load "+kind, "cannot read QA file", err)
	}
	if len(data) > qaHardStateBytes {
		return NewQAError(QAErrorBudgetExhausted, "load "+kind, "QA file exceeds the hard state limit", nil)
	}
	var header struct {
		SchemaVersion int `json:"schema_version"`
	}
	if err := json.Unmarshal(data, &header); err != nil {
		return NewQAError(QAErrorInvalidState, "load "+kind, "malformed QA JSON", err)
	}
	if header.SchemaVersion != QASchemaVersion {
		category := QAErrorUnknownSchema
		if header.SchemaVersion == 0 {
			category = QAErrorInvalidState
		}
		return NewQAError(category, "load "+kind, fmt.Sprintf("unsupported QA schema version %d", header.SchemaVersion), nil)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return NewQAError(QAErrorInvalidState, "load "+kind, "malformed or unknown QA field", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err == nil {
		return NewQAError(QAErrorInvalidState, "load "+kind, "multiple JSON values", nil)
	} else if !errors.Is(err, io.EOF) {
		return NewQAError(QAErrorInvalidState, "load "+kind, "trailing QA JSON", err)
	}
	return nil
}

type QAPublication struct {
	Map       *QAMap
	Shards    []QAShard
	Synthesis *QASynthesis
	State     QAState
	Flow      FlowState
}

func (store QAStore) Publish(publication QAPublication, token QAWriterToken) error {
	if err := store.checkWriter(token); err != nil {
		return err
	}
	if err := ValidateQAState(publication.State); err != nil {
		return NewQAError(QAErrorInvalidState, "publish", err.Error(), err)
	}
	if publication.State.Project != store.sprint.Project || publication.State.Sprint != store.sprint.Slug {
		return NewQAError(QAErrorInvalidState, "publish", "QA state scope does not match the selected sprint", nil)
	}
	if publication.Map != nil {
		if err := ValidateQAMap(*publication.Map); err != nil {
			return NewQAError(QAErrorInvalidState, "publish map", err.Error(), err)
		}
		path, err := store.mapPath(publication.Map.SemanticAttemptID)
		if err != nil {
			return err
		}
		digest, err := store.writeRecord("map", path, publication.Map, true)
		if err != nil {
			return err
		}
		publication.State.Map = &QAArtifactRef{Path: QAMapRelPath(store.sprint, publication.Map.SemanticAttemptID), Digest: digest}
		publication.State.CurrentAttemptID = publication.Map.SemanticAttemptID
	}
	sort.Slice(publication.Shards, func(i, j int) bool { return publication.Shards[i].ID < publication.Shards[j].ID })
	for i := range publication.Shards {
		shard := &publication.Shards[i]
		if err := ValidateQAShard(*shard); err != nil {
			return NewQAError(QAErrorInvalidState, "publish shard", err.Error(), err)
		}
		path, err := store.shardPath(shard.AttemptID, shard.ID)
		if err != nil {
			return err
		}
		if _, err := store.writeRecord("shard", path, shard, false); err != nil {
			return err
		}
	}
	if publication.Synthesis != nil {
		if err := ValidateQASynthesis(*publication.Synthesis, MaximumQABudgets()); err != nil {
			return NewQAError(QAErrorInvalidState, "publish synthesis", err.Error(), err)
		}
		path, err := store.synthesisPath(publication.Synthesis.AttemptID)
		if err != nil {
			return err
		}
		digest, err := store.writeRecord("synthesis", path, publication.Synthesis, false)
		if err != nil {
			return err
		}
		publication.State.Synthesis = &QAArtifactRef{Path: QASynthesisRelPath(store.sprint, publication.Synthesis.AttemptID), Digest: digest}
	}
	statePath, err := store.StatePath()
	if err != nil {
		return err
	}
	stateDigest, err := store.writeRecord("state", statePath, &publication.State, false)
	if err != nil {
		return err
	}
	publication.Flow.QA = qaFlowSummary(publication.State, stateDigest, store.sprint)
	if store.hooks.BeforeStep != nil {
		flowPath, _ := FlowStatePath(store.root, store.sprint)
		if err := store.hooks.BeforeStep("flow", flowPath); err != nil {
			return NewQAError(QAErrorPersistenceFailure, "publish flow summary", "flow summary publication was interrupted", err)
		}
	}
	if err := SaveFlowState(store.root, store.sprint, publication.Flow); err != nil {
		return NewQAError(QAErrorPersistenceFailure, "publish flow summary", "cannot publish QA flow summary", err)
	}
	return nil
}

// SaveRecoveredState is the explicit runtime-free recovery mutation. It can
// only record a non-active phase and never writes map, shard, or synthesis
// evidence. A previously validated completed state may be republished solely
// to reconcile its flow-summary pointer and digest.
func (store QAStore) SaveRecoveredState(state QAState, flow FlowState) error {
	switch state.Phase {
	case QAPhaseInterrupted, QAPhaseStale, QAPhaseInvalid, QAPhaseBlocked, QAPhaseCancelled, QAPhaseCompleted:
	default:
		return NewQAError(QAErrorInvalidState, "recover", "recovery cannot publish active QA state", nil)
	}
	if err := ValidateQAState(state); err != nil {
		return NewQAError(QAErrorInvalidState, "recover", err.Error(), err)
	}
	path, err := store.StatePath()
	if err != nil {
		return err
	}
	digest, err := store.writeRecord("state", path, &state, false)
	if err != nil {
		return err
	}
	flow.QA = qaFlowSummary(state, digest, store.sprint)
	if err := SaveFlowState(store.root, store.sprint, flow); err != nil {
		return NewQAError(QAErrorPersistenceFailure, "recover", "cannot publish recovered flow summary", err)
	}
	return nil
}

func (store QAStore) checkWriter(token QAWriterToken) error {
	if err := token.Validate(); err != nil {
		return NewQAError(QAErrorConflict, "publish", err.Error(), err)
	}
	if store.fence == nil {
		return NewQAError(QAErrorConflict, "publish", "no QA writer fence is configured", nil)
	}
	if err := store.fence(token); err != nil {
		return NewQAError(QAErrorConflict, "publish", "QA writer token is stale", err)
	}
	return nil
}

func (store QAStore) writeRecord(kind, path string, value any, immutable bool) (string, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return "", NewQAError(QAErrorInvalidState, "publish "+kind, "cannot encode QA record", err)
	}
	data = append(data, '\n')
	if len(data) > qaHardStateBytes {
		return "", NewQAError(QAErrorBudgetExhausted, "publish "+kind, "QA record exceeds the hard state limit", nil)
	}
	if store.hooks.BeforeStep != nil {
		if err := store.hooks.BeforeStep(kind, path); err != nil {
			return "", NewQAError(QAErrorPersistenceFailure, "publish "+kind, "QA publication was interrupted", err)
		}
	}
	if immutable {
		current, readErr := os.ReadFile(path)
		if readErr == nil {
			if bytes.Equal(current, data) {
				return hashBytes(data), nil
			}
			return "", NewQAError(QAErrorConflict, "publish "+kind, "immutable QA map already exists with different bytes", nil)
		}
		if !errors.Is(readErr, fs.ErrNotExist) {
			return "", NewQAError(QAErrorPersistenceFailure, "publish "+kind, "cannot inspect existing QA record", readErr)
		}
	}
	if err := privateAtomicWrite(path, data, kind, store.hooks); err != nil {
		return "", NewQAError(QAErrorPersistenceFailure, "publish "+kind, "cannot atomically publish QA record", err)
	}
	return hashBytes(data), nil
}

func privateAtomicWrite(path string, data []byte, kind string, hooks QAStateHooks) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	for current := dir; ; current = filepath.Dir(current) {
		if err := os.Chmod(current, 0o700); err != nil {
			return err
		}
		if filepath.Base(current) == "verification" {
			break
		}
		parent := filepath.Dir(current)
		if parent == current {
			return fmt.Errorf("verification directory not found")
		}
	}
	temp, err := os.CreateTemp(dir, ".qa-"+kind+"-*.tmp")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	remove := true
	defer func() {
		if remove {
			_ = os.Remove(tempPath)
		}
	}()
	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		return err
	}
	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if hooks.BeforeRename != nil {
		if err := hooks.BeforeRename(kind, path); err != nil {
			return err
		}
	}
	if err := os.Rename(tempPath, path); err != nil {
		return err
	}
	remove = false
	syncDir(dir)
	return nil
}

func (store QAStore) verifyReference(ref QAArtifactRef) error {
	if filepath.IsAbs(ref.Path) || !validFingerprint(ref.Digest) {
		return NewQAError(QAErrorInvalidState, "load state", "invalid QA artifact reference", nil)
	}
	path, err := store.resolve(ref.Path)
	if err != nil {
		return err
	}
	digest, err := hashFile(path)
	if err != nil {
		return NewQAError(QAErrorPersistenceFailure, "load state", "cannot read referenced QA artifact", err)
	}
	if digest != ref.Digest {
		return NewQAError(QAErrorInvalidState, "load state", "QA artifact digest mismatch", nil)
	}
	return nil
}

func qaFlowSummary(state QAState, stateDigest string, sprint Sprint) *QAFlowSummary {
	return &QAFlowSummary{
		Phase: state.Phase, Fresh: state.Freshness.Current,
		CompletedShards: state.CompletedShards, TotalShards: state.TotalShards,
		Confirmed: state.OutcomeCounts[QATheoryConfirmed], Blocked: state.OutcomeCounts[QATheoryBlocked],
		Cancellation: state.Cancellation, StatePath: QAVerificationStateRelPath(sprint),
		StateDigest: stateDigest, CurrentAttemptID: state.CurrentAttemptID, NextAction: state.NextAction,
	}
}
