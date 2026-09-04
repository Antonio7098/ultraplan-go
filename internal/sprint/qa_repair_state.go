package sprint

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
)

type RepairCyclePublication struct {
	Cycle          RepairCycle
	Proposal       []byte
	Scope          *RepairScopeRecord
	Reverification *RepairReverification
	Cleanup        *RepairCleanup
	Journal        *RepairApplyJournal
}

func (store QAStore) LoadRepairState() (RepairState, error) {
	path, err := store.resolve(QARepairStateRelPath(store.sprint))
	if err != nil {
		return RepairState{}, err
	}
	var state RepairState
	if err := store.readStrictVersion(path, "repair-state", QARepairSchemaVersion, &state); err != nil {
		return RepairState{}, err
	}
	if err := ValidateRepairState(state); err != nil {
		return RepairState{}, NewQAError(QAErrorInvalidState, "load repair state", err.Error(), err)
	}
	if state.Project != store.sprint.Project || state.Sprint != store.sprint.Slug {
		return RepairState{}, NewQAError(QAErrorInvalidState, "load repair state", "repair state scope does not match selected sprint", nil)
	}
	for _, ref := range []*QAArtifactRef{state.Packet, state.Confirmation, state.Result} {
		if ref != nil {
			if err := store.verifyReference(*ref); err != nil {
				return RepairState{}, err
			}
		}
	}
	return state, nil
}

func (store QAStore) LoadRepairPacket(attemptID, runID string) (RepairIssuePacket, error) {
	if err := validateRepairScopeIDs(attemptID, runID); err != nil {
		return RepairIssuePacket{}, err
	}
	path, err := store.resolve(QARepairPacketRelPath(store.sprint, attemptID, runID))
	if err != nil {
		return RepairIssuePacket{}, err
	}
	var value RepairIssuePacket
	if err := store.readStrictVersion(path, "repair-packet", QARepairSchemaVersion, &value); err != nil {
		return RepairIssuePacket{}, err
	}
	if err := ValidateRepairPacket(value); err != nil || value.QAAttemptID != attemptID || value.RepairRunID != runID {
		return RepairIssuePacket{}, NewQAError(QAErrorInvalidState, "load repair packet", "repair packet is invalid or stored under the wrong identity", err)
	}
	return value, nil
}

func (store QAStore) LoadRepairConfirmation(attemptID, runID string, packet RepairIssuePacket) (RepairConfirmation, error) {
	if err := validateRepairScopeIDs(attemptID, runID); err != nil {
		return RepairConfirmation{}, err
	}
	path, err := store.resolve(QARepairConfirmationRelPath(store.sprint, attemptID, runID))
	if err != nil {
		return RepairConfirmation{}, err
	}
	var value RepairConfirmation
	if err := store.readStrictVersion(path, "repair-confirmation", QARepairSchemaVersion, &value); err != nil {
		return RepairConfirmation{}, err
	}
	if err := ValidateRepairConfirmation(value, packet); err != nil {
		return RepairConfirmation{}, NewQAError(QAErrorInvalidState, "load repair confirmation", err.Error(), err)
	}
	return value, nil
}

func (store QAStore) LoadRepairResult(attemptID, runID string) (RepairResult, error) {
	if err := validateRepairScopeIDs(attemptID, runID); err != nil {
		return RepairResult{}, err
	}
	path, err := store.resolve(QARepairResultRelPath(store.sprint, attemptID, runID))
	if err != nil {
		return RepairResult{}, err
	}
	var value RepairResult
	if err := store.readStrictVersion(path, "repair-result", QARepairSchemaVersion, &value); err != nil {
		return RepairResult{}, err
	}
	if err := ValidateRepairResult(value); err != nil || value.QAAttemptID != attemptID || value.RepairRunID != runID {
		return RepairResult{}, NewQAError(QAErrorInvalidState, "load repair result", "repair result is invalid or stored under the wrong identity", err)
	}
	return value, nil
}

func (store QAStore) LoadRepairApplyJournal(attemptID, runID string, cycle int) (RepairApplyJournal, error) {
	if err := validateRepairScopeIDs(attemptID, runID); err != nil || cycle <= 0 {
		return RepairApplyJournal{}, NewQAError(QAErrorInvalidState, "load repair apply journal", "invalid repair cycle identity", err)
	}
	path, err := store.resolve(filepath.ToSlash(filepath.Join(QARepairCycleRelPath(store.sprint, attemptID, runID, cycle), "apply-journal.json")))
	if err != nil {
		return RepairApplyJournal{}, err
	}
	var value RepairApplyJournal
	if err := store.readStrictVersion(path, "repair-apply-journal", QARepairSchemaVersion, &value); err != nil {
		return RepairApplyJournal{}, err
	}
	if err := ValidateRepairApplyJournal(value); err != nil || value.RepairRunID != runID || value.Cycle != cycle {
		return RepairApplyJournal{}, NewQAError(QAErrorInvalidState, "load repair apply journal", "repair apply journal is invalid or stored under the wrong identity", err)
	}
	return value, nil
}

func (store QAStore) LoadRepairCycle(attemptID, runID string, cycle int) (RepairCycle, error) {
	if err := validateRepairScopeIDs(attemptID, runID); err != nil || cycle <= 0 {
		return RepairCycle{}, NewQAError(QAErrorInvalidState, "load repair cycle", "invalid repair cycle identity", err)
	}
	path, err := store.resolve(filepath.ToSlash(filepath.Join(QARepairCycleRelPath(store.sprint, attemptID, runID, cycle), "cycle.json")))
	if err != nil {
		return RepairCycle{}, err
	}
	var value RepairCycle
	if err := store.readStrictVersion(path, "repair-cycle", QARepairSchemaVersion, &value); err != nil {
		return RepairCycle{}, err
	}
	if err := ValidateRepairCycle(value); err != nil || value.RepairRunID != runID || value.Number != cycle {
		return RepairCycle{}, NewQAError(QAErrorInvalidState, "load repair cycle", "repair cycle is stored under the wrong identity", nil)
	}
	for _, ref := range []*QAArtifactRef{value.Proposal, value.TestPatch, value.ImplementationPatch, value.Scope, value.Reverification, value.Cleanup} {
		if ref != nil {
			if err := store.verifyReference(*ref); err != nil {
				return RepairCycle{}, err
			}
		}
	}
	return value, nil
}

func (store QAStore) loadRepairCycleRecord(attemptID, runID string, cycle int, name string, value any) error {
	path, err := store.resolve(filepath.ToSlash(filepath.Join(QARepairCycleRelPath(store.sprint, attemptID, runID, cycle), name)))
	if err != nil {
		return err
	}
	return store.readStrictVersion(path, "repair-"+strings.TrimSuffix(name, ".json"), QARepairSchemaVersion, value)
}

func (store QAStore) loadRepairPreimage(operation RepairApplyOperation) ([]byte, error) {
	path, err := store.resolve(operation.PreimagePath)
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		return nil, NewQAError(QAErrorInvalidState, "load repair preimage", "repair preimage is not a private regular file", nil)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if hashBytes(data) != operation.PreimageDigest {
		return nil, NewQAError(QAErrorInvalidState, "load repair preimage", "repair preimage digest mismatch", nil)
	}
	return data, nil
}

func (store QAStore) LoadManualRepairProof() (ManualRepairProof, error) {
	path, err := store.resolve(QARepairProofRelPath(store.sprint))
	if err != nil {
		return ManualRepairProof{}, err
	}
	var value ManualRepairProof
	if err := store.readStrictVersion(path, "manual-repair-proof", QARepairSchemaVersion, &value); err != nil {
		return ManualRepairProof{}, err
	}
	if value.Project != store.sprint.Project || value.Sprint != store.sprint.Slug || !validRepairID(value.RepairRunID, "run") || value.PublishedAt.IsZero() {
		return ManualRepairProof{}, NewQAError(QAErrorInvalidState, "load manual repair proof", "manual repair proof scope is invalid", nil)
	}
	return value, nil
}

func (store QAStore) PublishRepairPacket(packet RepairIssuePacket, state RepairState, flow FlowState, token QAWriterToken) error {
	if err := ValidateRepairPacket(packet); err != nil {
		return NewQAError(QAErrorInvalidState, "publish repair packet", err.Error(), err)
	}
	if state.Packet != nil || state.Confirmation != nil || state.Result != nil || state.QAAttemptID != packet.QAAttemptID || state.RepairRunID != packet.RepairRunID {
		return NewQAError(QAErrorInvalidState, "publish repair packet", "initial repair state is inconsistent with packet", nil)
	}
	path, err := store.resolve(QARepairPacketRelPath(store.sprint, packet.QAAttemptID, packet.RepairRunID))
	if err != nil {
		return err
	}
	digest, err := store.writeRepairRecord(token, "repair-packet", path, packet, true)
	if err != nil {
		return err
	}
	state.Packet = &QAArtifactRef{Path: QARepairPacketRelPath(store.sprint, packet.QAAttemptID, packet.RepairRunID), Digest: digest}
	state.Phase = RepairPhasePrepared
	return store.publishRepairState(state, flow, token)
}

func (store QAStore) PublishRepairConfirmation(value RepairConfirmation, state RepairState, flow FlowState, token QAWriterToken) error {
	packet, err := store.LoadRepairPacket(state.QAAttemptID, state.RepairRunID)
	if err != nil {
		return err
	}
	if err := ValidateRepairConfirmation(value, packet); err != nil {
		return NewQAError(QAErrorInvalidState, "publish repair confirmation", err.Error(), err)
	}
	if value.OperationRunID != token.RunID || value.OperationalAttemptID != token.OperationalAttemptID || value.FencingGeneration != token.FencingGeneration {
		return NewQAError(QAErrorConflict, "publish repair confirmation", "confirmation is not bound to the current durable writer", nil)
	}
	path, err := store.resolve(QARepairConfirmationRelPath(store.sprint, value.QAAttemptID, value.RepairRunID))
	if err != nil {
		return err
	}
	digest, err := store.writeRepairRecord(token, "repair-confirmation", path, value, true)
	if err != nil {
		return err
	}
	state.Confirmation = &QAArtifactRef{Path: QARepairConfirmationRelPath(store.sprint, value.QAAttemptID, value.RepairRunID), Digest: digest}
	state.Run = QARunCorrelation{Lifecycle: QARunClaimed, RunID: token.RunID, OperationalAttemptID: token.OperationalAttemptID, FencingGeneration: token.FencingGeneration}
	state.Phase = RepairPhaseConfirmed
	return store.publishRepairState(state, flow, token)
}

func (store QAStore) PublishRepairProposal(state RepairState, flow FlowState, cycle int, proposal []byte, token QAWriterToken) error {
	if cycle <= 0 || cycle != state.CurrentCycle || len(proposal) == 0 {
		return NewQAError(QAErrorInvalidState, "publish repair proposal", "proposal cycle and bytes are required", nil)
	}
	packet, err := store.LoadRepairPacket(state.QAAttemptID, state.RepairRunID)
	if err != nil {
		return err
	}
	if len(proposal) > packet.Budgets.MaxPatchBytes || bytes.IndexByte(proposal, 0) >= 0 {
		return NewQAError(QAErrorBudgetExhausted, "publish repair proposal", "proposal patch exceeds the frozen limit or contains binary bytes", nil)
	}
	base := QARepairCycleRelPath(store.sprint, state.QAAttemptID, state.RepairRunID, cycle)
	normalized := normalizePatch(proposal)
	testPatch, implementationPatch := splitRepairProposalPatches(normalized, packet.RegressionTestPaths)
	path, err := store.resolve(filepath.ToSlash(filepath.Join(base, "proposal.patch")))
	if err != nil {
		return err
	}
	if _, err := store.writeRepairBytes(token, "repair-proposal", path, normalized, true); err != nil {
		return err
	}
	for name, content := range map[string][]byte{"test.patch": testPatch, "implementation.patch": implementationPatch} {
		if len(content) == 0 && name == "test.patch" {
			continue
		}
		componentPath, resolveErr := store.resolve(filepath.ToSlash(filepath.Join(base, name)))
		if resolveErr != nil {
			return resolveErr
		}
		if _, writeErr := store.writeRepairBytes(token, "repair-"+strings.TrimSuffix(name, ".patch"), componentPath, content, true); writeErr != nil {
			return writeErr
		}
	}
	state.NextAction = "Apply the retained proposal through the product-owned boundary."
	return store.publishRepairState(state, flow, token)
}

func (store QAStore) StageRepairApplyJournal(state RepairState, flow FlowState, cycle int, target string, replacements map[string][]byte, expected map[string]string, token QAWriterToken) (RepairApplyJournal, error) {
	if cycle <= 0 || cycle != state.CurrentCycle || state.Phase != RepairPhaseApplying || len(replacements) == 0 {
		return RepairApplyJournal{}, NewQAError(QAErrorInvalidState, "stage repair apply", "repair is not at the applying boundary", nil)
	}
	paths := mapKeys(replacements)
	journal := RepairApplyJournal{SchemaVersion: QARepairSchemaVersion, RepairRunID: state.RepairRunID, Cycle: cycle, State: "planned", UpdatedAt: state.UpdatedAt}
	base := QARepairCycleRelPath(store.sprint, state.QAAttemptID, state.RepairRunID, cycle)
	for _, rel := range paths {
		path := filepath.Join(target, filepath.FromSlash(rel))
		before, err := os.ReadFile(path)
		created := errors.Is(err, fs.ErrNotExist) && expected[rel] == repairMissingPreimageDigest && ClassifyRepairPath(rel) == RepairPathTestAsset
		if created {
			before, err = nil, nil
			if err := ensureRepairCreatablePath(target, path); err != nil {
				return RepairApplyJournal{}, err
			}
		} else if err == nil {
			if err := ensureRepairRegularPath(target, path); err != nil {
				return RepairApplyJournal{}, err
			}
		}
		if err != nil {
			return RepairApplyJournal{}, err
		}
		if digest := hashBytes(before); created && expected[rel] != repairMissingPreimageDigest || !created && (!validFingerprint(expected[rel]) || digest != expected[rel]) {
			return RepairApplyJournal{}, NewQAError(QAErrorConflict, "stage repair apply", "production preimage changed before journal publication", nil)
		}
		preimageRel := filepath.ToSlash(filepath.Join(base, "preimages", rel))
		preimagePath, err := store.resolve(preimageRel)
		if err != nil {
			return RepairApplyJournal{}, err
		}
		if _, err := store.writeRepairBytes(token, "repair-preimage", preimagePath, before, true); err != nil {
			return RepairApplyJournal{}, err
		}
		journal.Operations = append(journal.Operations, RepairApplyOperation{Path: rel, PreimageDigest: expected[rel], PreimagePath: preimageRel, PostimageDigest: hashBytes(replacements[rel]), Created: created})
	}
	if err := store.PublishRepairApplyJournal(state, flow, journal, token); err != nil {
		return RepairApplyJournal{}, err
	}
	return journal, nil
}

func (store QAStore) PublishRepairApplyJournal(state RepairState, flow FlowState, journal RepairApplyJournal, token QAWriterToken) error {
	if err := ValidateRepairApplyJournal(journal); err != nil || journal.RepairRunID != state.RepairRunID || journal.Cycle != state.CurrentCycle {
		return NewQAError(QAErrorInvalidState, "publish repair apply journal", "repair apply journal is invalid", err)
	}
	path, err := store.resolve(filepath.ToSlash(filepath.Join(QARepairCycleRelPath(store.sprint, state.QAAttemptID, state.RepairRunID, journal.Cycle), "apply-journal.json")))
	if err != nil {
		return err
	}
	if _, err := store.writeRepairRecord(token, "repair-apply-journal", path, journal, false); err != nil {
		return err
	}
	return store.publishRepairState(state, flow, token)
}

func (store QAStore) PublishRepairCycle(publication RepairCyclePublication, state RepairState, flow FlowState, token QAWriterToken) error {
	cycle := publication.Cycle
	if cycle.SchemaVersion != QARepairSchemaVersion || cycle.RepairRunID != state.RepairRunID || cycle.Number <= 0 || cycle.Number != state.CurrentCycle {
		return NewQAError(QAErrorInvalidState, "publish repair cycle", "repair cycle identity is invalid", nil)
	}
	base := QARepairCycleRelPath(store.sprint, state.QAAttemptID, state.RepairRunID, cycle.Number)
	if len(publication.Proposal) > 0 {
		packet, err := store.LoadRepairPacket(state.QAAttemptID, state.RepairRunID)
		if err != nil {
			return err
		}
		if len(publication.Proposal) > packet.Budgets.MaxPatchBytes || bytes.IndexByte(publication.Proposal, 0) >= 0 {
			return NewQAError(QAErrorBudgetExhausted, "publish repair proposal", "proposal patch exceeds the frozen limit or contains binary bytes", nil)
		}
		normalized := normalizePatch(publication.Proposal)
		testPatch, implementationPatch := splitRepairProposalPatches(normalized, packet.RegressionTestPaths)
		path, err := store.resolve(filepath.ToSlash(filepath.Join(base, "proposal.patch")))
		if err != nil {
			return err
		}
		digest, err := store.writeRepairBytes(token, "repair-proposal", path, normalized, true)
		if err != nil {
			return err
		}
		cycle.Proposal = &QAArtifactRef{Path: filepath.ToSlash(filepath.Join(base, "proposal.patch")), Digest: digest}
		for name, content := range map[string][]byte{"test.patch": testPatch, "implementation.patch": implementationPatch} {
			if len(content) == 0 && name == "test.patch" {
				continue
			}
			componentPath, resolveErr := store.resolve(filepath.ToSlash(filepath.Join(base, name)))
			if resolveErr != nil {
				return resolveErr
			}
			componentDigest, writeErr := store.writeRepairBytes(token, "repair-"+strings.TrimSuffix(name, ".patch"), componentPath, content, true)
			if writeErr != nil {
				return writeErr
			}
			ref := &QAArtifactRef{Path: filepath.ToSlash(filepath.Join(base, name)), Digest: componentDigest}
			if name == "test.patch" {
				cycle.TestPatch = ref
			} else {
				cycle.ImplementationPatch = ref
			}
		}
	}
	if publication.Scope != nil {
		if err := ValidateRepairScope(*publication.Scope); err != nil || publication.Scope.RepairRunID != state.RepairRunID || publication.Scope.Cycle != cycle.Number {
			return NewQAError(QAErrorInvalidState, "publish repair scope", "repair scope is invalid or stored under the wrong identity", err)
		}
		path, err := store.resolve(filepath.ToSlash(filepath.Join(base, "scope.json")))
		if err != nil {
			return err
		}
		digest, err := store.writeRepairRecord(token, "repair-scope", path, publication.Scope, true)
		if err != nil {
			return err
		}
		cycle.Scope = &QAArtifactRef{Path: filepath.ToSlash(filepath.Join(base, "scope.json")), Digest: digest}
	}
	if publication.Reverification != nil {
		if err := ValidateRepairReverification(*publication.Reverification); err != nil {
			return NewQAError(QAErrorInvalidState, "publish repair reverification", err.Error(), err)
		}
		path, err := store.resolve(filepath.ToSlash(filepath.Join(base, "reverification.json")))
		if err != nil {
			return err
		}
		digest, err := store.writeRepairRecord(token, "repair-reverification", path, publication.Reverification, true)
		if err != nil {
			return err
		}
		cycle.Reverification = &QAArtifactRef{Path: filepath.ToSlash(filepath.Join(base, "reverification.json")), Digest: digest}
	}
	if publication.Cleanup != nil {
		if err := ValidateRepairCleanup(*publication.Cleanup); err != nil || publication.Cleanup.RepairRunID != state.RepairRunID || publication.Cleanup.Cycle != cycle.Number {
			return NewQAError(QAErrorInvalidState, "publish repair cleanup", "repair cleanup is invalid or stored under the wrong identity", err)
		}
		path, err := store.resolve(filepath.ToSlash(filepath.Join(base, "cleanup.json")))
		if err != nil {
			return err
		}
		digest, err := store.writeRepairRecord(token, "repair-cleanup", path, publication.Cleanup, true)
		if err != nil {
			return err
		}
		cycle.Cleanup = &QAArtifactRef{Path: filepath.ToSlash(filepath.Join(base, "cleanup.json")), Digest: digest}
	}
	if publication.Journal != nil {
		path, err := store.resolve(filepath.ToSlash(filepath.Join(base, "apply-journal.json")))
		if err != nil {
			return err
		}
		if _, err := store.writeRepairRecord(token, "repair-apply-journal", path, publication.Journal, false); err != nil {
			return err
		}
	}
	cyclePath, err := store.resolve(filepath.ToSlash(filepath.Join(base, "cycle.json")))
	if err != nil {
		return err
	}
	if _, err := store.writeRepairRecord(token, "repair-cycle", cyclePath, cycle, true); err != nil {
		return err
	}
	state.Consumed.Cycles = maxInt(state.Consumed.Cycles, cycle.Number)
	return store.publishRepairState(state, flow, token)
}

func splitRepairProposalPatches(proposal []byte, regressionPaths []string) (testPatch, implementationPatch []byte) {
	regression := stringSet(regressionPaths)
	lines := strings.SplitAfter(string(proposal), "\n")
	var test, implementation strings.Builder
	var destination *strings.Builder
	for _, line := range lines {
		if strings.HasPrefix(line, "--- a/") {
			path := strings.TrimSpace(strings.TrimPrefix(line, "--- a/"))
			destination = &implementation
			if regression[path] {
				destination = &test
			}
		}
		if destination != nil {
			destination.WriteString(line)
		}
	}
	return []byte(test.String()), []byte(implementation.String())
}

func (store QAStore) PublishRepairResult(result RepairResult, state RepairState, flow FlowState, token QAWriterToken) error {
	if err := ValidateRepairResult(result); err != nil || result.QAAttemptID != state.QAAttemptID || result.RepairRunID != state.RepairRunID {
		return NewQAError(QAErrorInvalidState, "publish repair result", "terminal repair result is invalid", err)
	}
	path, err := store.resolve(QARepairResultRelPath(store.sprint, state.QAAttemptID, state.RepairRunID))
	if err != nil {
		return err
	}
	digest, err := store.writeRepairRecord(token, "repair-result", path, result, true)
	if err != nil {
		return err
	}
	state.Phase = RepairPhaseTerminal
	state.Outcome = result.Outcome
	state.StopReason = result.StopReason
	state.Consumed = result.Consumed
	state.Result = &QAArtifactRef{Path: QARepairResultRelPath(store.sprint, state.QAAttemptID, state.RepairRunID), Digest: digest}
	state.Run.Lifecycle = QARunTerminal
	state.Run.TerminalResult = repairTerminalResult(result.Outcome)
	state.NextAction = result.NextAction
	return store.publishRepairState(state, flow, token)
}

func (store QAStore) PublishManualRepairProof(proof ManualRepairProof, packet RepairIssuePacket, result RepairResult, protocol, runtime string, token QAWriterToken) error {
	if err := QualifyManualRepairProof(proof, packet, result, protocol, runtime); err != nil {
		return NewQAError(QAErrorAdmissionBlocked, "publish manual repair proof", err.Error(), err)
	}
	path, err := store.resolve(QARepairProofRelPath(store.sprint))
	if err != nil {
		return err
	}
	_, err = store.writeRepairRecord(token, "manual-repair-proof", path, proof, false)
	return err
}

func (store QAStore) publishRepairState(state RepairState, flow FlowState, token QAWriterToken) (resultErr error) {
	if err := ValidateRepairState(state); err != nil {
		return NewQAError(QAErrorInvalidState, "publish repair state", err.Error(), err)
	}
	statePath, err := store.resolve(QARepairStateRelPath(store.sprint))
	if err != nil {
		return err
	}
	flowPath, err := FlowStatePath(store.root, store.sprint)
	if err != nil {
		return err
	}
	snapshots, err := captureQACanonicalFiles(statePath, flowPath)
	if err != nil {
		return NewQAError(QAErrorPersistenceFailure, "publish repair state", "cannot snapshot repair pointers", err)
	}
	started := false
	defer func() {
		if resultErr != nil && started {
			if rollbackErr := restoreQACanonicalFiles(snapshots); rollbackErr != nil {
				resultErr = errors.Join(resultErr, NewQAError(QAErrorPersistenceFailure, "publish repair rollback", "cannot restore prior repair pointers", rollbackErr))
			}
		}
	}()
	started = true
	digest, err := store.writeRepairRecord(token, "repair-state", statePath, state, false)
	if err != nil {
		return err
	}
	flow.Repair = repairFlowSummary(state, digest, store.sprint)
	if err := store.checkWriter(token); err != nil {
		return err
	}
	if err := SaveFlowState(store.root, store.sprint, flow); err != nil {
		return NewQAError(QAErrorPersistenceFailure, "publish repair flow summary", "cannot publish repair flow summary", err)
	}
	return nil
}

func ValidateRepairState(state RepairState) error {
	if state.SchemaVersion != QARepairSchemaVersion || !safeQAName(state.Project) || !safeQAName(state.Sprint) || !validQAIDKind(state.QAAttemptID, "attempt") || !validRepairID(state.RepairRunID, "run") || !validRepairMode(state.Mode) || !validRepairPhase(state.Phase) {
		return fmt.Errorf("invalid repair state schema, scope, or phase")
	}
	if strings.TrimSpace(state.NextAction) == "" || state.UpdatedAt.IsZero() || state.CurrentCycle < 0 || state.EarliestCycle < 0 || state.EarliestCycle > state.CurrentCycle && state.CurrentCycle > 0 {
		return fmt.Errorf("repair state progress fields are invalid")
	}
	if err := ValidateRepairConsumed(state.Consumed); err != nil {
		return err
	}
	if state.Runtime != nil {
		if err := ValidateRepairRuntime(*state.Runtime); err != nil {
			return err
		}
	}
	if state.Outcome != "" && !validRepairOutcome(state.Outcome) {
		return fmt.Errorf("repair state outcome is invalid")
	}
	if state.Outcome == RepairOutcomeStalled && state.Mode != RepairModeAutomatic {
		return fmt.Errorf("manual repair cannot be stalled")
	}
	if state.Phase == RepairPhaseTerminal && (state.Outcome == "" || state.Result == nil) {
		return fmt.Errorf("terminal repair state requires result and outcome")
	}
	if state.Phase != RepairPhasePrepared && state.Packet == nil {
		return fmt.Errorf("repair state phase requires packet authority")
	}
	if state.Phase == RepairPhaseConfirmed || state.Phase == RepairPhaseProposing || state.Phase == RepairPhaseApplying || state.Phase == RepairPhaseReverifying || state.Phase == RepairPhaseCleaning || state.Phase == RepairPhaseTerminalizing || state.Phase == RepairPhaseTerminal {
		if state.Confirmation == nil {
			return fmt.Errorf("mutable repair state requires confirmation")
		}
	}
	return nil
}

func ValidateRepairResult(result RepairResult) error {
	if result.SchemaVersion != QARepairSchemaVersion || !safeQAName(result.Project) || !safeQAName(result.Sprint) || !validQAIDKind(result.QAAttemptID, "attempt") || !validRepairID(result.RepairRunID, "run") || !validRepairMode(result.Mode) || !validRepairOutcome(result.Outcome) || strings.TrimSpace(result.Reason) == "" || strings.TrimSpace(result.NextAction) == "" || result.CompletedAt.IsZero() {
		return fmt.Errorf("invalid repair result")
	}
	if !validFingerprint(result.Target.Fingerprint) || len(result.Evidence) == 0 {
		return fmt.Errorf("repair result lacks target or evidence authority")
	}
	if err := ValidateRepairConsumed(result.Consumed); err != nil {
		return err
	}
	if result.Consumed.MutationCycles > result.Consumed.Cycles || result.Consumed.StagnantCycles > result.Consumed.Cycles || result.Consumed.ChangedFiles > 0 && result.Consumed.MutationCycles == 0 || result.Consumed.ChangedBytes > 0 && result.Consumed.ChangedFiles == 0 {
		return fmt.Errorf("terminal repair consumption counters are inconsistent")
	}
	if result.Runtime != nil {
		if err := ValidateRepairRuntime(*result.Runtime); err != nil {
			return err
		}
	}
	for _, ref := range result.Evidence {
		path, err := normalizeRepairPath(ref.Path)
		if err != nil || path != ref.Path || !validFingerprint(ref.Digest) {
			return fmt.Errorf("repair result evidence reference is invalid")
		}
	}
	if result.Outcome == RepairOutcomeStalled && result.Mode != RepairModeAutomatic {
		return fmt.Errorf("manual repair cannot publish stalled")
	}
	verified := result.Outcome == RepairOutcomeVerified || result.Outcome == RepairOutcomeVerifiedWithFindings
	pending := result.Outcome == RepairOutcomeCampaignPending
	if verified && (!result.CleanupComplete || !result.ProductionApplied || !result.CompleteLadder || len(result.UnresolvedIssues) != 0 || result.StopReason != RepairStopVerified) {
		return fmt.Errorf("verified repair result lacks complete apply, ladder, cleanup, or issue resolution")
	}
	if !verified && result.CompleteLadder {
		return fmt.Errorf("non-verified repair result cannot claim a complete ladder")
	}
	if pending && (!result.CleanupComplete || !result.ProductionApplied || len(result.UnresolvedIssues) != 0 || result.StopReason != RepairStopNone) {
		return fmt.Errorf("campaign-pending repair lacks issue-scoped apply, cleanup, or resolution")
	}
	if result.ProductionApplied && result.Consumed.MutationCycles == 0 {
		return fmt.Errorf("repair result apply claim lacks a consumed mutation cycle")
	}
	return nil
}

func ValidateRepairConsumed(value RepairConsumed) error {
	if value.Cycles < 0 || value.MutationCycles < 0 || value.Reopenings < 0 || value.StagnantCycles < 0 || value.ChangedFiles < 0 || value.ChangedBytes < 0 || value.RuntimeAttempts < 0 || value.ModelTurns < 0 || value.Commands < 0 || value.OutputBytes < 0 {
		return fmt.Errorf("repair consumption contains a negative counter")
	}
	return nil
}

func ValidateRepairRuntime(value RepairRuntimeObservation) error {
	provider, model := strings.TrimSpace(value.Provider), strings.TrimSpace(value.Model)
	// Provider and model were added to repair schema v1 after its first real
	// runs. Accept both absent for those retained records, but reject partial
	// identity in newly written observations.
	if provider == "" != (model == "") || value.StartedAt.IsZero() || value.CompletedAt.Before(value.StartedAt) || value.Duration < 0 || value.DurationMS < 0 || value.RuntimeEvents < 0 || value.RetainedEvents < 0 || value.ObservedToolCalls < 0 || int64(value.RetainedEvents) > value.RuntimeEvents {
		return fmt.Errorf("invalid repair runtime observation")
	}
	usage := value.Usage
	if usage.InputTokens < 0 || usage.OutputTokens < 0 || usage.TotalTokens < 0 || usage.ReasoningTokens < 0 || usage.CacheReadTokens < 0 || usage.CacheWriteTokens < 0 || usage.Turns < 0 {
		return fmt.Errorf("repair runtime usage contains a negative counter")
	}
	if value.EstimatedCost != nil && value.EstimatedCost.Amount < 0 {
		return fmt.Errorf("repair runtime cost is negative")
	}
	return nil
}

func ValidateRepairCycle(value RepairCycle) error {
	if value.SchemaVersion != QARepairSchemaVersion || !validRepairID(value.RepairRunID, "run") || value.Number <= 0 || value.StartedAt.IsZero() {
		return fmt.Errorf("invalid repair cycle identity")
	}
	if value.CompletedAt != nil && value.CompletedAt.Before(value.StartedAt) {
		return fmt.Errorf("repair cycle completion precedes its start")
	}
	for _, ref := range []*QAArtifactRef{value.Proposal, value.Scope, value.Reverification, value.Cleanup} {
		if ref == nil {
			continue
		}
		path, err := normalizeRepairPath(ref.Path)
		if err != nil || path != ref.Path || !validFingerprint(ref.Digest) {
			return fmt.Errorf("repair cycle contains an invalid artifact reference")
		}
	}
	return nil
}

func ValidateRepairScope(value RepairScopeRecord) error {
	if value.SchemaVersion != QARepairSchemaVersion || !validRepairID(value.RepairRunID, "run") || value.Cycle <= 0 || !validFingerprint(value.Before.Fingerprint) || !validFingerprint(value.After.Fingerprint) || value.ChangedBytes < 0 {
		return fmt.Errorf("invalid repair scope record")
	}
	intended, err := NormalizeRepairPaths(value.IntendedPaths)
	if err != nil || !reflect.DeepEqual(intended, value.IntendedPaths) {
		return fmt.Errorf("repair scope intended paths are invalid or unordered")
	}
	actual, err := NormalizeRepairPaths(value.ActualPaths)
	if err != nil || !reflect.DeepEqual(actual, value.ActualPaths) {
		return fmt.Errorf("repair scope actual paths are invalid or unordered")
	}
	if value.Enforced && !sameRepairPaths(value.IntendedPaths, value.ActualPaths) {
		return fmt.Errorf("enforced repair scope does not match actual paths")
	}
	return nil
}

func ValidateRepairCleanup(value RepairCleanup) error {
	if value.SchemaVersion != QARepairSchemaVersion || !validRepairID(value.RepairRunID, "run") || value.Cycle <= 0 || value.Duration < 0 {
		return fmt.Errorf("invalid repair cleanup record")
	}
	proved := value.ProcessTreeTerminated && value.WorkspaceRemoved && value.CompensationKnown && value.TargetCurrent && value.LeaseReleased
	if value.Complete != proved {
		return fmt.Errorf("repair cleanup completion disagrees with cleanup facts")
	}
	return nil
}

func ValidateRepairReverification(value RepairReverification) error {
	if value.SchemaVersion != QARepairSchemaVersion || !validRepairID(value.RepairRunID, "run") || value.Cycle <= 0 || value.CompletedAt.IsZero() || len(value.Gates) != len(RepairGateOrder()) {
		return fmt.Errorf("invalid repair reverification record")
	}
	blocked := false
	for i, gate := range value.Gates {
		if gate.Gate != RepairGateOrder()[i] {
			return fmt.Errorf("repair reverification gate order changed")
		}
		switch gate.Status {
		case RepairGatePassed:
			if blocked {
				return fmt.Errorf("wider repair gate passed after a non-pass")
			}
		case RepairGateFailed, RepairGateBlocked:
			blocked = true
		case RepairGateSkipped:
			if !blocked || strings.TrimSpace(gate.Reason) == "" || strings.TrimSpace(gate.NextAction) == "" {
				return fmt.Errorf("skipped repair gate lacks prior stop and guidance")
			}
		case RepairGateDeferred:
			if blocked || gate.Gate != RepairGateContainingSmoke || strings.TrimSpace(gate.Reason) == "" || strings.TrimSpace(gate.NextAction) == "" {
				return fmt.Errorf("deferred repair gate is not a guided final campaign smoke deferral")
			}
		default:
			return fmt.Errorf("repair reverification is not terminal")
		}
	}
	return nil
}

func ValidateRepairApplyJournal(value RepairApplyJournal) error {
	if value.SchemaVersion != QARepairSchemaVersion || !validRepairID(value.RepairRunID, "run") || value.Cycle <= 0 || value.UpdatedAt.IsZero() || len(value.Operations) == 0 {
		return fmt.Errorf("invalid repair apply journal identity")
	}
	switch value.State {
	case "planned", "applying", "applied", "compensated", "uncertain":
	default:
		return fmt.Errorf("invalid repair apply journal state")
	}
	prior := ""
	for _, operation := range value.Operations {
		path, err := normalizeRepairPath(operation.Path)
		class := ClassifyRepairPath(path)
		validPreimage := validFingerprint(operation.PreimageDigest) || operation.Created && operation.PreimageDigest == repairMissingPreimageDigest
		if err != nil || path != operation.Path || path <= prior || class != RepairPathProduction && !(operation.Created && class == RepairPathTestAsset) || !validPreimage || !validFingerprint(operation.PostimageDigest) || strings.TrimSpace(operation.PreimagePath) == "" {
			return fmt.Errorf("invalid repair apply operation")
		}
		prior = path
	}
	return nil
}

func (store QAStore) PruneRepairCycles(attemptID, runID string, current, retain int) (int, error) {
	if err := validateRepairScopeIDs(attemptID, runID); err != nil || current <= 0 || retain <= 0 {
		return 0, NewQAError(QAErrorInvalidState, "prune repair cycles", "invalid repair retention request", err)
	}
	root, err := store.resolve(filepath.ToSlash(filepath.Join(QARepairRunRelPath(store.sprint, attemptID, runID), "cycles")))
	if err != nil {
		return 0, err
	}
	entries, err := os.ReadDir(root)
	if errors.Is(err, fs.ErrNotExist) {
		return current, nil
	}
	if err != nil {
		return 0, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	earliest := current
	removeBefore := current - retain + 1
	for _, entry := range entries {
		if !entry.IsDir() || len(entry.Name()) != 6 {
			return 0, NewQAError(QAErrorInvalidState, "prune repair cycles", "repair cycle storage contains an invalid entry", nil)
		}
		var number int
		if _, scanErr := fmt.Sscanf(entry.Name(), "%06d", &number); scanErr != nil || number <= 0 || number > current {
			return 0, NewQAError(QAErrorInvalidState, "prune repair cycles", "repair cycle storage contains an invalid number", scanErr)
		}
		if number < removeBefore {
			if err := os.RemoveAll(filepath.Join(root, entry.Name())); err != nil {
				return 0, err
			}
			continue
		}
		if number < earliest {
			earliest = number
		}
	}
	return earliest, nil
}

func (store QAStore) writeRepairRecord(token QAWriterToken, kind, path string, value any, immutable bool) (string, error) {
	copyStore := store
	prior := copyStore.hooks.BeforeRename
	copyStore.hooks.BeforeRename = func(renameKind, renamePath string) error {
		if err := copyStore.checkWriter(token); err != nil {
			return err
		}
		if prior != nil {
			return prior(renameKind, renamePath)
		}
		return nil
	}
	if err := copyStore.checkWriter(token); err != nil {
		return "", err
	}
	return copyStore.writeRecord(kind, path, value, immutable)
}

func (store QAStore) writeRepairBytes(token QAWriterToken, kind, path string, data []byte, immutable bool) (string, error) {
	copyStore := store
	prior := copyStore.hooks.BeforeRename
	copyStore.hooks.BeforeRename = func(renameKind, renamePath string) error {
		if err := copyStore.checkWriter(token); err != nil {
			return err
		}
		if prior != nil {
			return prior(renameKind, renamePath)
		}
		return nil
	}
	if err := copyStore.checkWriter(token); err != nil {
		return "", err
	}
	return copyStore.writeBytes(kind, path, data, immutable)
}

func validateRepairScopeIDs(attemptID, runID string) error {
	if !validQAIDKind(attemptID, "attempt") || !validRepairID(runID, "run") {
		return NewQAError(QAErrorInvalidState, "repair scope", "invalid QA attempt or repair run identity", nil)
	}
	return nil
}

func repairFlowSummary(state RepairState, digest string, sprint Sprint) *RepairFlowSummary {
	return &RepairFlowSummary{Phase: state.Phase, Fresh: state.Freshness.Current, Mode: state.Mode, RepairRunID: state.RepairRunID, QAAttemptID: state.QAAttemptID, CurrentCycle: state.CurrentCycle, Outcome: state.Outcome, StopReason: state.StopReason, StatePath: QARepairStateRelPath(sprint), StateDigest: digest, NextAction: state.NextAction}
}

func repairTerminalResult(outcome RepairOutcome) QATerminalResult {
	switch outcome {
	case RepairOutcomeBlocked:
		return QATerminalBlocked
	case RepairOutcomeEscalated:
		return QATerminalCleanupUncertain
	default:
		return QATerminalCompleted
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
