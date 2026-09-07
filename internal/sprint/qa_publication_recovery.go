package sprint

import (
	"path/filepath"
)

// Recover only an interrupted synthesis whose arbitration was already
// checkpointed in immutable session rounds. This restores progress, never a
// completed assessment. Other mismatched references keep the normal clear and
// rebuild behavior. The caller holds the sprint mutation lease.
func (store QAStore) recoverInterruptedSynthesis(state *QAState) (bool, error) {
	if state.Phase != QAPhaseSynthesizing || state.Synthesis == nil || state.Map == nil || state.Adjudication == nil || state.Synthesis.Path != QASynthesisRelPath(store.sprint, state.CurrentAttemptID) || store.verifyReference(*state.Synthesis) == nil {
		return false, nil
	}
	for _, ref := range []*QAArtifactRef{state.Map, state.Adjudication, state.Issues, state.Assessment, state.CanonicalReport} {
		if ref != nil && store.verifyReference(*ref) != nil {
			return false, nil
		}
	}
	qaMap, err := store.LoadMap(state.CurrentAttemptID)
	if err != nil {
		return false, nil
	}
	synthesis, err := store.LoadSynthesis(state.CurrentAttemptID, qaMap.Budgets)
	if err != nil || synthesis.MapID != qaMap.ID || synthesis.Arbitration == nil || synthesis.Arbitration.MapID != qaMap.ID {
		return false, nil
	}
	groups, err := store.LoadLatestArbiterSessionGroups(state.CurrentAttemptID)
	if err != nil || len(groups) == 0 || len(groups) != len(synthesis.Arbitration.Groups) {
		return false, nil
	}
	byID := map[string]string{}
	for _, group := range groups {
		byID[group.ID], err = fingerprintQAValue(group)
		if err != nil {
			return false, err
		}
	}
	for _, group := range synthesis.Arbitration.Groups {
		digest, err := fingerprintQAValue(group)
		if err != nil || byID[group.ID] != digest {
			return false, nil
		}
	}
	// Rebuild all deterministic synthesis fields from the retained shards.
	var shards []QAShard
	for _, planned := range append(append([]QAShard(nil), qaMap.Shards...), synthesis.FollowUpShards...) {
		shard, err := store.LoadShard(state.CurrentAttemptID, planned.ID)
		if err != nil {
			return false, nil
		}
		shards = append(shards, shard)
	}
	rebuilt, err := SynthesizeQA(qaMap, applyQAArbitration(shards, *synthesis.Arbitration))
	if err != nil {
		return false, nil
	}
	rebuilt.Arbitration = synthesis.Arbitration
	if err := finalizeQASynthesisFollowUps(&rebuilt, qaMap, shards); err != nil {
		return false, nil
	}
	expected, err := fingerprintQAValue(rebuilt)
	if err != nil {
		return false, err
	}
	actual, err := fingerprintQAValue(synthesis)
	if err != nil || expected != actual {
		return false, nil
	}
	path, err := store.synthesisPath(state.CurrentAttemptID)
	if err != nil {
		return false, err
	}
	digest, err := hashFile(path)
	if err != nil {
		return false, err
	}
	// Preserve the prior pointer and exact candidate for the recovery audit.
	history := filepath.Join(filepath.Dir(path), "publication-recovery", digest)
	if _, err := store.writeRecord("recovery-prior-state", filepath.Join(history, "state.json"), state, true); err != nil {
		return false, err
	}
	if _, err := store.writeRecord("recovery-synthesis", filepath.Join(history, "synthesis.json"), &synthesis, true); err != nil {
		return false, err
	}
	state.Synthesis = &QAArtifactRef{Path: state.Synthesis.Path, Digest: digest}
	return true, nil
}
