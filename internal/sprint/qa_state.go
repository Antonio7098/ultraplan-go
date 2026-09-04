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

func QAEvidencePlanRelPath(s Sprint, attemptID, planID string) string {
	return filepath.ToSlash(filepath.Join("projects", s.Project, "sprints", s.Slug, "verification", "attempts", attemptID, "plans", planID+".json"))
}

func QAEvidenceRelPath(s Sprint, attemptID, evidenceID string) string {
	return filepath.ToSlash(filepath.Join("projects", s.Project, "sprints", s.Slug, "verification", "attempts", attemptID, "evidence", evidenceID+".json"))
}

func QAInvestigatorTestRelPath(s Sprint, attemptID, testID string) string {
	return filepath.ToSlash(filepath.Join("projects", s.Project, "sprints", s.Slug, "verification", "attempts", attemptID, "investigator-tests", testID))
}

func QAReproductionSpecRelPath(s Sprint, attemptID, testID string) string {
	return filepath.ToSlash(filepath.Join(QAInvestigatorTestRelPath(s, attemptID, testID), "reproduction-spec.json"))
}

func QATestBundleRelPath(s Sprint, attemptID, testID string) string {
	return filepath.ToSlash(filepath.Join(QAInvestigatorTestRelPath(s, attemptID, testID), "bundle.json"))
}

func QATestFileRelPath(s Sprint, attemptID, testID, path string) string {
	return filepath.ToSlash(filepath.Join(QAInvestigatorTestRelPath(s, attemptID, testID), "files", filepath.FromSlash(path)))
}

func QATestAuthoringAttemptRelPath(s Sprint, attemptID, testID string, number int) string {
	return filepath.ToSlash(filepath.Join(QAInvestigatorTestRelPath(s, attemptID, testID), "authoring-attempts", fmt.Sprintf("%06d.json", number)))
}

func QAReproductionRunRelPath(s Sprint, attemptID, testID, runID string) string {
	return filepath.ToSlash(filepath.Join(QAInvestigatorTestRelPath(s, attemptID, testID), "runs", runID))
}

func QAReproductionRunResultRelPath(s Sprint, attemptID, testID, runID string) string {
	return filepath.ToSlash(filepath.Join(QAReproductionRunRelPath(s, attemptID, testID, runID), "result.json"))
}

func QAArbiterEvidenceRequestRelPath(s Sprint, attemptID, requestID string) string {
	return filepath.ToSlash(filepath.Join("projects", s.Project, "sprints", s.Slug, "verification", "attempts", attemptID, "arbiter-evidence-requests", requestID+".json"))
}

func QAIssueEvidenceCoverageRelPath(s Sprint, attemptID string) string {
	return filepath.ToSlash(filepath.Join("projects", s.Project, "sprints", s.Slug, "verification", "attempts", attemptID, "issue-evidence-coverage.json"))
}

func QAInvestigatorWorkspaceCleanupRelPath(s Sprint, attemptID string) string {
	return filepath.ToSlash(filepath.Join("projects", s.Project, "sprints", s.Slug, "verification", "attempts", attemptID, "investigator-workspace-cleanup.json"))
}

func QAArbiterSessionRoundRelPath(s Sprint, attemptID, groupID string, round int) string {
	return filepath.ToSlash(filepath.Join("projects", s.Project, "sprints", s.Slug, "verification", "attempts", attemptID, "arbiter-sessions", groupID, fmt.Sprintf("%06d.json", round)))
}

func QAPatchRelPath(s Sprint, attemptID, patchID string) string {
	return filepath.ToSlash(filepath.Join("projects", s.Project, "sprints", s.Slug, "verification", "attempts", attemptID, "patches", patchID+".patch"))
}

func QAAdjudicationRelPath(s Sprint, attemptID string) string {
	return filepath.ToSlash(filepath.Join("projects", s.Project, "sprints", s.Slug, "verification", "attempts", attemptID, "adjudication.json"))
}

func QAIssuesRelPath(s Sprint, attemptID string) string {
	return filepath.ToSlash(filepath.Join("projects", s.Project, "sprints", s.Slug, "verification", "attempts", attemptID, "issues.json"))
}

func QAAssessmentRelPath(s Sprint, attemptID string) string {
	return filepath.ToSlash(filepath.Join("projects", s.Project, "sprints", s.Slug, "verification", "attempts", attemptID, "assessment.json"))
}

func QAReportRelPath(s Sprint) string {
	return filepath.ToSlash(filepath.Join("projects", s.Project, "sprints", s.Slug, "qa.md"))
}

func QARepairStateRelPath(s Sprint) string {
	return filepath.ToSlash(filepath.Join("projects", s.Project, "sprints", s.Slug, "verification", "repair-state.json"))
}

func QARepairRunRelPath(s Sprint, attemptID, runID string) string {
	return filepath.ToSlash(filepath.Join("projects", s.Project, "sprints", s.Slug, "verification", "attempts", attemptID, "repairs", runID))
}

func QARepairPacketRelPath(s Sprint, attemptID, runID string) string {
	return filepath.ToSlash(filepath.Join(QARepairRunRelPath(s, attemptID, runID), "issue-packet.json"))
}

func QARepairConfirmationRelPath(s Sprint, attemptID, runID string) string {
	return filepath.ToSlash(filepath.Join(QARepairRunRelPath(s, attemptID, runID), "confirmation.json"))
}

func QARepairCycleRelPath(s Sprint, attemptID, runID string, cycle int) string {
	return filepath.ToSlash(filepath.Join(QARepairRunRelPath(s, attemptID, runID), "cycles", fmt.Sprintf("%06d", cycle)))
}

func QARepairResultRelPath(s Sprint, attemptID, runID string) string {
	return filepath.ToSlash(filepath.Join(QARepairRunRelPath(s, attemptID, runID), "result.json"))
}

func QARepairProofRelPath(s Sprint) string {
	return filepath.ToSlash(filepath.Join("projects", s.Project, "sprints", s.Slug, "verification", "manual-repair-proof.json"))
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
	return store.loadState(true)
}

// loadStateForRecovery reads the canonical state without following artifact
// references. Recovery needs this narrow path precisely when an interrupted
// publication left one of those references dangling or digest-mismatched.
func (store QAStore) loadStateForRecovery() (QAState, error) {
	return store.loadState(false)
}

func (store QAStore) loadState(verifyArtifacts bool) (QAState, error) {
	path, err := store.StatePath()
	if err != nil {
		return QAState{}, err
	}
	var state QAState
	if err := store.readStrictVersion(path, "state", 0, &state); err != nil {
		return QAState{}, err
	}
	if err := ValidateQAState(state); err != nil {
		return QAState{}, NewQAError(QAErrorInvalidState, "load state", err.Error(), err)
	}
	if state.Project != store.sprint.Project || state.Sprint != store.sprint.Slug {
		return QAState{}, NewQAError(QAErrorInvalidState, "load state", "QA state scope does not match the selected sprint", nil)
	}
	if !verifyArtifacts {
		return state, nil
	}
	for _, ref := range []*QAArtifactRef{state.Map, state.Synthesis, state.Adjudication, state.Issues, state.Assessment, state.CanonicalReport} {
		if ref == nil {
			continue
		}
		if err := store.verifyReference(*ref); err != nil {
			return QAState{}, err
		}
	}
	return state, nil
}

func (store QAStore) clearInvalidArtifactReferences(state *QAState) []string {
	if state == nil {
		return nil
	}
	refs := []struct {
		name string
		ref  **QAArtifactRef
	}{
		{"map", &state.Map}, {"synthesis", &state.Synthesis}, {"adjudication", &state.Adjudication},
		{"issues", &state.Issues}, {"assessment", &state.Assessment}, {"report", &state.CanonicalReport},
	}
	var cleared []string
	for _, candidate := range refs {
		if *candidate.ref != nil && store.verifyReference(**candidate.ref) != nil {
			*candidate.ref = nil
			cleared = append(cleared, candidate.name)
		}
	}
	return cleared
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
	upgradeLegacyQAMap(&value)
	if err := ValidateQAMap(value); err != nil {
		return QAMap{}, NewQAError(QAErrorInvalidState, "load map", err.Error(), err)
	}
	if value.Project != store.sprint.Project || value.Sprint != store.sprint.Slug || value.SemanticAttemptID != attemptID {
		return QAMap{}, NewQAError(QAErrorInvalidState, "load map", "QA map scope does not match its path", nil)
	}
	return value, nil
}

func upgradeLegacyQAMap(value *QAMap) {
	if value != nil && value.SchemaVersion == QASchemaVersion {
		defaults := DefaultQABudgets()
		if value.Budgets.ArbiterMaxTheories == 0 {
			value.Budgets.ArbiterMaxTheories = defaults.ArbiterMaxTheories
		}
		if value.Budgets.EvidenceRoundsPerShard == 0 {
			value.Budgets.EvidenceRoundsPerShard = defaults.EvidenceRoundsPerShard
			value.Budgets.TestsPerTheory = defaults.TestsPerTheory
			value.Budgets.TestsPerIssue = defaults.TestsPerIssue
			value.Budgets.AuthoredTestFiles = defaults.AuthoredTestFiles
			value.Budgets.AuthoredTestBytes = defaults.AuthoredTestBytes
			value.Budgets.TestCommandsPerRound = defaults.TestCommandsPerRound
			value.Budgets.AuthoringRuntimeTurns = defaults.AuthoringRuntimeTurns
			value.Budgets.AuthoringWallTime = defaults.AuthoringWallTime
		}
	}
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

func (store QAStore) LoadEvidence(attemptID, evidenceID string) (QAEvidenceRecord, error) {
	if !validQAIDKind(attemptID, "attempt") || !validQAV2ID(evidenceID, "evidence") {
		return QAEvidenceRecord{}, NewQAError(QAErrorInvalidState, "load evidence", "invalid evidence identity", nil)
	}
	path, err := store.resolve(QAEvidenceRelPath(store.sprint, attemptID, evidenceID))
	if err != nil {
		return QAEvidenceRecord{}, err
	}
	var value QAEvidenceRecord
	if err := store.readStrictVersion(path, "evidence", QAEvidenceSchemaVersion, &value); err != nil {
		return QAEvidenceRecord{}, err
	}
	if value.ID != evidenceID || value.AttemptID != attemptID {
		return QAEvidenceRecord{}, NewQAError(QAErrorInvalidState, "load evidence", "evidence identity does not match its path", nil)
	}
	return value, nil
}

func (store QAStore) LoadEvidencePlan(attemptID, planID string, budgets QABudgets) (QAEvidencePlan, error) {
	if !validQAIDKind(attemptID, "attempt") || !validQAV2ID(planID, "plan") {
		return QAEvidencePlan{}, NewQAError(QAErrorInvalidState, "load evidence plan", "invalid plan identity", nil)
	}
	path, err := store.resolve(QAEvidencePlanRelPath(store.sprint, attemptID, planID))
	if err != nil {
		return QAEvidencePlan{}, err
	}
	var value QAEvidencePlan
	if err := store.readStrictVersion(path, "evidence-plan", QAEvidenceSchemaVersion, &value); err != nil {
		return QAEvidencePlan{}, err
	}
	if value.ID != planID || value.AttemptID != attemptID {
		return QAEvidencePlan{}, NewQAError(QAErrorInvalidState, "load evidence plan", "evidence plan identity does not match its path", nil)
	}
	if err := ValidateQAEvidencePlan(value, budgets); err != nil {
		return QAEvidencePlan{}, NewQAError(QAErrorInvalidState, "load evidence plan", err.Error(), err)
	}
	return value, nil
}

func (store QAStore) LoadAdjudication(attemptID string, budgets QABudgets) (QAAdjudication, error) {
	if !validQAIDKind(attemptID, "attempt") {
		return QAAdjudication{}, NewQAError(QAErrorInvalidState, "load adjudication", "invalid attempt identity", nil)
	}
	path, err := store.resolve(QAAdjudicationRelPath(store.sprint, attemptID))
	if err != nil {
		return QAAdjudication{}, err
	}
	var value QAAdjudication
	if err := store.readStrictVersion(path, "adjudication", QAEvidenceSchemaVersion, &value); err != nil {
		return QAAdjudication{}, err
	}
	if err := validateQAAdjudication(value, attemptID, budgets); err != nil {
		return QAAdjudication{}, NewQAError(QAErrorInvalidState, "load adjudication", err.Error(), err)
	}
	return value, nil
}

func (store QAStore) LoadAssessment(attemptID string) (QAAssessmentRecord, error) {
	if !validQAIDKind(attemptID, "attempt") {
		return QAAssessmentRecord{}, NewQAError(QAErrorInvalidState, "load assessment", "invalid attempt identity", nil)
	}
	path, err := store.resolve(QAAssessmentRelPath(store.sprint, attemptID))
	if err != nil {
		return QAAssessmentRecord{}, err
	}
	var value QAAssessmentRecord
	if err := store.readStrictVersion(path, "assessment", QAEvidenceSchemaVersion, &value); err != nil {
		return QAAssessmentRecord{}, err
	}
	if err := validateQAAssessment(value, attemptID); err != nil {
		return QAAssessmentRecord{}, NewQAError(QAErrorInvalidState, "load assessment", err.Error(), err)
	}
	return value, nil
}

func (store QAStore) readStrict(path, kind string, value any) error {
	return store.readStrictVersion(path, kind, QASchemaVersion, value)
}

func (store QAStore) readStrictVersion(path, kind string, expectedVersion int, value any) error {
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
	versionAccepted := header.SchemaVersion == expectedVersion
	if kind == "state" && expectedVersion == 0 {
		versionAccepted = header.SchemaVersion == QASchemaVersion || header.SchemaVersion == QAStateSchemaVersion
	}
	if !versionAccepted {
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
	Evidence  *QAEvidencePublication
}

type QAPatchRecord struct {
	ID      string
	Content []byte
}

type QAEvidencePublication struct {
	Plans             []QAEvidencePlan
	Records           []QAEvidenceRecord
	Patches           []QAPatchRecord
	Adjudication      *QAAdjudication
	Assessment        *QAAssessmentRecord
	Report            []byte
	Budgets           QABudgets
	InvestigatorTests []QATestPublication
	EvidenceRequests  []QAArbiterEvidenceRequest
	IssueCoverage     []QAIssueEvidenceCoverage
}

type QATestPublication struct {
	Spec              QAReproductionSpec
	Bundle            QATestBundle
	AuthoringAttempts []QAInvestigatorAttempt
	Runs              []QAReproductionRun
}

func (store QAStore) Publish(publication QAPublication, token QAWriterToken) (resultErr error) {
	if err := store.checkWriter(token); err != nil {
		return err
	}
	if publication.Evidence != nil {
		publication.State.SchemaVersion = QAStateSchemaVersion
	}
	if err := ValidateQAState(publication.State); err != nil {
		return NewQAError(QAErrorInvalidState, "publish", err.Error(), err)
	}
	if publication.State.Project != store.sprint.Project || publication.State.Sprint != store.sprint.Slug {
		return NewQAError(QAErrorInvalidState, "publish", "QA state scope does not match the selected sprint", nil)
	}
	statePath, err := store.StatePath()
	if err != nil {
		return err
	}
	reportPath, err := store.resolve(QAReportRelPath(store.sprint))
	if err != nil {
		return err
	}
	flowPath, err := FlowStatePath(store.root, store.sprint)
	if err != nil {
		return err
	}
	snapshots, err := captureQACanonicalFiles(statePath, reportPath, flowPath)
	if err != nil {
		return NewQAError(QAErrorPersistenceFailure, "publish", "cannot snapshot canonical QA files", err)
	}
	canonicalStarted := false
	defer func() {
		if resultErr != nil && canonicalStarted {
			if rollbackErr := restoreQACanonicalFiles(snapshots); rollbackErr != nil {
				resultErr = errors.Join(resultErr, NewQAError(QAErrorPersistenceFailure, "publish rollback", "cannot restore prior canonical QA files", rollbackErr))
			}
		}
	}()
	if publication.Map != nil {
		if err := ValidateQAMap(*publication.Map); err != nil {
			return NewQAError(QAErrorInvalidState, "publish map", err.Error(), err)
		}
		path, err := store.mapPath(publication.Map.SemanticAttemptID)
		if err != nil {
			return err
		}
		if err := store.checkWriter(token); err != nil {
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
		if err := store.checkWriter(token); err != nil {
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
		if err := store.checkWriter(token); err != nil {
			return err
		}
		digest, err := store.writeRecord("synthesis", path, publication.Synthesis, false)
		if err != nil {
			return err
		}
		publication.State.Synthesis = &QAArtifactRef{Path: QASynthesisRelPath(store.sprint, publication.Synthesis.AttemptID), Digest: digest}
	}
	if publication.Evidence != nil {
		canonicalStarted = len(publication.Evidence.Report) > 0
		if err := store.publishEvidence(publication.Evidence, &publication.State, token); err != nil {
			return err
		}
	}
	if err := ValidateQAState(publication.State); err != nil {
		return NewQAError(QAErrorInvalidState, "publish", err.Error(), err)
	}
	canonicalStarted = true
	if err := store.checkWriter(token); err != nil {
		return err
	}
	stateDigest, err := store.writeRecord("state", statePath, &publication.State, false)
	if err != nil {
		return err
	}
	publication.Flow.QA = qaFlowSummary(publication.State, stateDigest, store.sprint)
	if store.hooks.BeforeStep != nil {
		if err := store.hooks.BeforeStep("flow", flowPath); err != nil {
			return NewQAError(QAErrorPersistenceFailure, "publish flow summary", "flow summary publication was interrupted", err)
		}
	}
	if err := store.checkWriter(token); err != nil {
		return err
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
			return "", NewQAError(QAErrorConflict, "publish "+kind, "immutable QA "+kind+" already exists with different bytes", nil)
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

func (store QAStore) publishEvidence(bundle *QAEvidencePublication, state *QAState, token QAWriterToken) error {
	if bundle == nil || state == nil || !validQAIDKind(state.CurrentAttemptID, "attempt") {
		return NewQAError(QAErrorInvalidState, "publish evidence", "current attempt is required", nil)
	}
	if err := validateQABudgets(bundle.Budgets); err != nil {
		return NewQAError(QAErrorInvalidState, "publish evidence", err.Error(), err)
	}
	if len(bundle.Plans) > bundle.Budgets.GeneratedChecks || len(bundle.Records) > bundle.Budgets.EvidenceRecords || len(bundle.Patches) > bundle.Budgets.GeneratedChecks {
		return NewQAError(QAErrorBudgetExhausted, "publish evidence", "evidence bundle exceeds frozen limits", nil)
	}
	plans := make(map[string]QAEvidencePlan, len(bundle.Plans))
	patches := make(map[string]string, len(bundle.Patches))
	for _, plan := range bundle.Plans {
		if err := ValidateQAEvidencePlan(plan, bundle.Budgets); err != nil || plan.AttemptID != state.CurrentAttemptID {
			return NewQAError(QAErrorMalformedEvidence, "publish plan", "invalid frozen evidence plan", err)
		}
		if _, duplicate := plans[plan.ID]; duplicate {
			return NewQAError(QAErrorMalformedEvidence, "publish plan", "duplicate frozen evidence plan", nil)
		}
		plans[plan.ID] = plan
	}
	for _, patch := range bundle.Patches {
		if !validQAV2ID(patch.ID, "patch") || len(patch.Content) == 0 || len(patch.Content) > bundle.Budgets.GeneratedPatchBytes || bytes.IndexByte(patch.Content, 0) >= 0 {
			return NewQAError(QAErrorMalformedEvidence, "publish patch", "invalid generated patch", nil)
		}
		if _, duplicate := patches[patch.ID]; duplicate {
			return NewQAError(QAErrorMalformedEvidence, "publish patch", "duplicate generated patch", nil)
		}
		patches[patch.ID] = hashBytes(normalizePatch(patch.Content))
	}
	seenEvidence := make(map[string]struct{}, len(bundle.Records))
	for _, record := range bundle.Records {
		plan, ok := plans[record.PlanID]
		if !ok {
			return NewQAError(QAErrorMalformedEvidence, "publish evidence", "evidence references an unavailable plan", nil)
		}
		if err := ValidateQAEvidence(record, plan, bundle.Budgets); err != nil {
			return NewQAError(QAErrorMalformedEvidence, "publish evidence", err.Error(), err)
		}
		if _, duplicate := seenEvidence[record.ID]; duplicate {
			return NewQAError(QAErrorMalformedEvidence, "publish evidence", "duplicate evidence record", nil)
		}
		seenEvidence[record.ID] = struct{}{}
		if record.Patch != nil {
			patchID := strings.TrimSuffix(filepath.Base(record.Patch.Path), ".patch")
			expected := QAPatchRelPath(store.sprint, state.CurrentAttemptID, patchID)
			digest, available := patches[patchID]
			if record.Patch.Path != expected || !available || record.Patch.Digest != digest {
				return NewQAError(QAErrorMalformedEvidence, "publish evidence", "patch reference is not contained in the current attempt", nil)
			}
		}
	}
	if bundle.Adjudication != nil {
		if err := validateQAAdjudication(*bundle.Adjudication, state.CurrentAttemptID, bundle.Budgets); err != nil {
			return NewQAError(QAErrorMalformedEvidence, "publish adjudication", err.Error(), err)
		}
	}
	if bundle.Assessment != nil {
		if err := validateQAAssessment(*bundle.Assessment, state.CurrentAttemptID); err != nil {
			return NewQAError(QAErrorMalformedEvidence, "publish assessment", err.Error(), err)
		}
	}
	if len(bundle.Report) > 0 && !bytes.HasPrefix(bundle.Report, []byte("# QA")) {
		return NewQAError(QAErrorMalformedEvidence, "publish report", "canonical QA report must start with # QA", nil)
	}
	if err := store.publishAuthoredTests(bundle, state.CurrentAttemptID, token); err != nil {
		return err
	}
	for i := range bundle.Plans {
		plan := bundle.Plans[i]
		if err := ValidateQAEvidencePlan(plan, bundle.Budgets); err != nil || plan.AttemptID != state.CurrentAttemptID {
			return NewQAError(QAErrorMalformedEvidence, "publish plan", "invalid frozen evidence plan", err)
		}
		if err := store.checkWriter(token); err != nil {
			return err
		}
		path, err := store.resolve(QAEvidencePlanRelPath(store.sprint, state.CurrentAttemptID, plan.ID))
		if err != nil {
			return err
		}
		if _, err := store.writeRecord("evidence-plan", path, &plan, true); err != nil {
			return err
		}
		plans[plan.ID] = plan
	}
	for i := range bundle.Patches {
		patch := bundle.Patches[i]
		if !validQAV2ID(patch.ID, "patch") || len(patch.Content) == 0 || len(patch.Content) > bundle.Budgets.GeneratedPatchBytes || bytes.IndexByte(patch.Content, 0) >= 0 {
			return NewQAError(QAErrorMalformedEvidence, "publish patch", "invalid generated patch", nil)
		}
		if err := store.checkWriter(token); err != nil {
			return err
		}
		path, err := store.resolve(QAPatchRelPath(store.sprint, state.CurrentAttemptID, patch.ID))
		if err != nil {
			return err
		}
		digest, err := store.writeBytes("patch", path, normalizePatch(patch.Content), true)
		if err != nil {
			return err
		}
		patches[patch.ID] = digest
	}
	for i := range bundle.Records {
		record := bundle.Records[i]
		plan, ok := plans[record.PlanID]
		if !ok {
			return NewQAError(QAErrorMalformedEvidence, "publish evidence", "evidence references an unavailable plan", nil)
		}
		if err := ValidateQAEvidence(record, plan, bundle.Budgets); err != nil {
			return NewQAError(QAErrorMalformedEvidence, "publish evidence", err.Error(), err)
		}
		if record.Patch != nil {
			patchID := strings.TrimSuffix(filepath.Base(record.Patch.Path), ".patch")
			expected := QAPatchRelPath(store.sprint, state.CurrentAttemptID, patchID)
			digest, available := patches[patchID]
			if record.Patch.Path != expected || !available || record.Patch.Digest != digest {
				return NewQAError(QAErrorMalformedEvidence, "publish evidence", "patch reference is not contained in the current attempt", nil)
			}
		}
		if err := store.checkWriter(token); err != nil {
			return err
		}
		path, err := store.resolve(QAEvidenceRelPath(store.sprint, state.CurrentAttemptID, record.ID))
		if err != nil {
			return err
		}
		if _, err := store.writeRecord("evidence", path, &record, true); err != nil {
			return err
		}
	}
	state.EvidenceCount = len(bundle.Records)
	if bundle.Adjudication != nil {
		if err := validateQAAdjudication(*bundle.Adjudication, state.CurrentAttemptID, bundle.Budgets); err != nil {
			return NewQAError(QAErrorMalformedEvidence, "publish adjudication", err.Error(), err)
		}
		path, err := store.resolve(QAAdjudicationRelPath(store.sprint, state.CurrentAttemptID))
		if err != nil {
			return err
		}
		if err := store.checkWriter(token); err != nil {
			return err
		}
		digest, err := store.writeRecord("adjudication", path, bundle.Adjudication, false)
		if err != nil {
			return err
		}
		state.Adjudication = &QAArtifactRef{Path: QAAdjudicationRelPath(store.sprint, state.CurrentAttemptID), Digest: digest}
		issuesPath, err := store.resolve(QAIssuesRelPath(store.sprint, state.CurrentAttemptID))
		if err != nil {
			return err
		}
		if err := store.checkWriter(token); err != nil {
			return err
		}
		issuesDigest, err := store.writeRecord("issues", issuesPath, struct {
			SchemaVersion int       `json:"schema_version"`
			AttemptID     string    `json:"attempt_id"`
			Issues        []QAIssue `json:"issues"`
		}{QAEvidenceSchemaVersion, state.CurrentAttemptID, bundle.Adjudication.Issues}, false)
		if err != nil {
			return err
		}
		state.Issues = &QAArtifactRef{Path: QAIssuesRelPath(store.sprint, state.CurrentAttemptID), Digest: issuesDigest}
		state.RejectedCount, state.IssueCount = len(bundle.Adjudication.Rejected), len(bundle.Adjudication.Issues)
		for _, issue := range bundle.Adjudication.Issues {
			if issue.RegressionCandidate {
				state.RegressionCandidates++
			}
		}
	}
	if bundle.Assessment != nil {
		if err := validateQAAssessment(*bundle.Assessment, state.CurrentAttemptID); err != nil {
			return NewQAError(QAErrorMalformedEvidence, "publish assessment", err.Error(), err)
		}
		path, err := store.resolve(QAAssessmentRelPath(store.sprint, state.CurrentAttemptID))
		if err != nil {
			return err
		}
		if err := store.checkWriter(token); err != nil {
			return err
		}
		digest, err := store.writeRecord("assessment", path, bundle.Assessment, false)
		if err != nil {
			return err
		}
		state.Assessment = &QAArtifactRef{Path: QAAssessmentRelPath(store.sprint, state.CurrentAttemptID), Digest: digest}
		state.CanonicalAssessment = bundle.Assessment.Assessment
	}
	if len(bundle.Report) > 0 {
		if !bytes.HasPrefix(bundle.Report, []byte("# QA")) {
			return NewQAError(QAErrorMalformedEvidence, "publish report", "canonical QA report must start with # QA", nil)
		}
		path, err := store.resolve(QAReportRelPath(store.sprint))
		if err != nil {
			return err
		}
		if err := store.checkWriter(token); err != nil {
			return err
		}
		digest, err := store.writeBytes("report", path, append(bytes.TrimSpace(bundle.Report), '\n'), false)
		if err != nil {
			return err
		}
		state.CanonicalReport = &QAArtifactRef{Path: QAReportRelPath(store.sprint), Digest: digest}
	}
	return nil
}

func (store QAStore) writeBytes(kind, path string, data []byte, immutable bool) (string, error) {
	if len(data) > qaHardStateBytes {
		return "", NewQAError(QAErrorBudgetExhausted, "publish "+kind, "record exceeds the hard state limit", nil)
	}
	if immutable {
		current, err := os.ReadFile(path)
		if err == nil {
			if bytes.Equal(current, data) {
				return hashBytes(data), nil
			}
			return "", NewQAError(QAErrorConflict, "publish "+kind, "immutable record already exists with different bytes", nil)
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
	}
	if err := privateAtomicWrite(path, data, kind, store.hooks); err != nil {
		return "", NewQAError(QAErrorPersistenceFailure, "publish "+kind, "cannot atomically publish record", err)
	}
	return hashBytes(data), nil
}

func normalizePatch(data []byte) []byte {
	data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	return append(bytes.TrimSpace(data), '\n')
}

func validateQAAdjudication(value QAAdjudication, attemptID string, budgets QABudgets) error {
	if value.SchemaVersion != QAEvidenceSchemaVersion || !validQAV2ID(value.ID, "adjudication") || value.AttemptID != attemptID || !validFingerprint(value.MapFingerprint) || value.CompletedAt.IsZero() {
		return fmt.Errorf("invalid QA adjudication schema or identity")
	}
	if len(value.AcceptedIDs)+len(value.Rejected) > budgets.EvidenceRecords || len(value.Issues) > budgets.Issues {
		return fmt.Errorf("QA adjudication exceeds frozen limits")
	}
	rootGroups := make(map[string]struct{}, len(value.Groups))
	for _, group := range value.Groups {
		if !validQAV2ID(group.ID, "group") || strings.TrimSpace(group.Claim) == "" || strings.TrimSpace(group.IssueClass) == "" || len(group.EvidenceIDs) == 0 {
			return fmt.Errorf("invalid root-cause group")
		}
		if _, duplicate := rootGroups[group.ID]; duplicate {
			return fmt.Errorf("duplicate root-cause group")
		}
		rootGroups[group.ID] = struct{}{}
	}
	issues := make(map[string]struct{}, len(value.Issues))
	for _, issue := range value.Issues {
		if !validQAV2ID(issue.ID, "issue") || strings.TrimSpace(issue.Title) == "" || strings.TrimSpace(issue.IssueClass) == "" || len(issue.EvidenceIDs) == 0 {
			return fmt.Errorf("invalid promoted issue")
		}
		if _, ok := rootGroups[issue.RootCauseGroupID]; !ok {
			return fmt.Errorf("promoted issue references an unknown root-cause group")
		}
		if _, duplicate := issues[issue.ID]; duplicate {
			return fmt.Errorf("duplicate promoted issue")
		}
		issues[issue.ID] = struct{}{}
	}
	groupIDs := make(map[string]struct{}, len(value.RepairGroups))
	for _, group := range value.RepairGroups {
		if !validQAV2ID(group.ID, "group") || strings.TrimSpace(group.Reason) == "" || len(group.IssueIDs) == 0 {
			return fmt.Errorf("invalid suggested repair group")
		}
		if _, duplicate := groupIDs[group.ID]; duplicate {
			return fmt.Errorf("duplicate suggested repair group")
		}
		groupIDs[group.ID] = struct{}{}
		for _, issueID := range group.IssueIDs {
			if _, ok := issues[issueID]; !ok {
				return fmt.Errorf("suggested repair group references an unknown issue")
			}
		}
	}
	return nil
}

func validateQAAssessment(value QAAssessmentRecord, attemptID string) error {
	if value.SchemaVersion != QAEvidenceSchemaVersion || !validQAV2ID(value.ID, "assessment") || value.AttemptID != attemptID || value.CompletedAt.IsZero() || strings.TrimSpace(value.NextAction) == "" {
		return fmt.Errorf("invalid QA assessment schema or identity")
	}
	switch value.Assessment {
	case AssessmentIncomplete, AssessmentBlocked, AssessmentFail, AssessmentNotApplicable, AssessmentPassWithFindings, AssessmentPass:
		return nil
	default:
		return fmt.Errorf("invalid QA assessment")
	}
}

func privateAtomicWrite(path string, data []byte, kind string, hooks QAStateHooks) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if pathHasComponent(path, "verification") {
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

type qaFileSnapshot struct {
	path   string
	data   []byte
	mode   fs.FileMode
	exists bool
}

func captureQACanonicalFiles(paths ...string) ([]qaFileSnapshot, error) {
	values := make([]qaFileSnapshot, 0, len(paths))
	for _, path := range paths {
		value := qaFileSnapshot{path: path}
		info, err := os.Lstat(path)
		if errors.Is(err, fs.ErrNotExist) {
			values = append(values, value)
			continue
		}
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("canonical QA path is not a regular file: %s", path)
		}
		value.data, err = os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		value.exists, value.mode = true, info.Mode().Perm()
		values = append(values, value)
	}
	return values, nil
}

func restoreQACanonicalFiles(values []qaFileSnapshot) error {
	var result error
	for _, value := range values {
		if !value.exists {
			if err := os.Remove(value.path); err != nil && !errors.Is(err, fs.ErrNotExist) {
				result = errors.Join(result, err)
			}
			continue
		}
		if err := privateAtomicWrite(value.path, value.data, "rollback", QAStateHooks{}); err != nil {
			result = errors.Join(result, err)
			continue
		}
		if err := os.Chmod(value.path, value.mode); err != nil {
			result = errors.Join(result, err)
		}
	}
	return result
}

func pathHasComponent(path, component string) bool {
	for current := filepath.Clean(path); ; current = filepath.Dir(current) {
		if filepath.Base(current) == component {
			return true
		}
		parent := filepath.Dir(current)
		if parent == current {
			return false
		}
	}
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
	out := &QAFlowSummary{
		Phase: state.Phase, Fresh: state.Freshness.Current,
		CompletedShards: state.CompletedShards, TotalShards: state.TotalShards,
		Confirmed: state.OutcomeCounts[QATheoryConfirmed], Blocked: state.OutcomeCounts[QATheoryBlocked],
		Cancellation: state.Cancellation, StatePath: QAVerificationStateRelPath(sprint),
		StateDigest: stateDigest, CurrentAttemptID: state.CurrentAttemptID, NextAction: state.NextAction,
		Assessment: state.CanonicalAssessment, EvidenceCount: state.EvidenceCount, RejectedCount: state.RejectedCount,
		IssueCount: state.IssueCount,
	}
	if state.CanonicalReport != nil {
		out.ReportPath, out.ReportDigest = state.CanonicalReport.Path, state.CanonicalReport.Digest
	}
	return out
}
