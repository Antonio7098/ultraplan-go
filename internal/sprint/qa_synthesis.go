package sprint

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

func BuildQAChallenge(project, sprint string, qaMap QAMap, identity QAChallengeIdentity) (QAChallenge, error) {
	identity.TheoryIDs = normalizeQAStrings(identity.TheoryIDs)
	identity.EvidenceRefs = normalizeQAStrings(identity.EvidenceRefs)
	identity.Claim = strings.TrimSpace(identity.Claim)
	identity.Basis = strings.TrimSpace(identity.Basis)
	identity.SafeEvidenceStrategy = strings.TrimSpace(identity.SafeEvidenceStrategy)
	id, err := NewQAChallengeID(project, sprint, qaMap.ID, identity)
	if err != nil {
		return QAChallenge{}, err
	}
	challenge := QAChallenge{SchemaVersion: QASchemaVersion, ID: id, MapID: qaMap.ID, TheoryIDs: identity.TheoryIDs, Claim: identity.Claim, Basis: identity.Basis, SafeEvidenceStrategy: identity.SafeEvidenceStrategy, EvidenceRefs: identity.EvidenceRefs}
	if err := ValidateQAChallenge(challenge, qaMap.Budgets); err != nil {
		return QAChallenge{}, err
	}
	return challenge, nil
}

func SynthesizeQA(qaMap QAMap, shards []QAShard) (QASynthesis, error) {
	return SynthesizeQAWithChallenges(qaMap, shards, nil)
}

// SynthesizeQAWithChallenges is pure for its normalized inputs. Challenger
// records can question theory groups, but cannot alter their retained outcome.
func SynthesizeQAWithChallenges(qaMap QAMap, shards []QAShard, challenges []QAChallenge) (QASynthesis, error) {
	if err := ValidateQAMap(qaMap); err != nil {
		return QASynthesis{}, NewQAError(QAErrorInvalidState, "synthesize", err.Error(), err)
	}
	shards = append([]QAShard(nil), shards...)
	sort.Slice(shards, func(i, j int) bool { return shards[i].ID < shards[j].ID })
	shardByTheory := map[string]QAShard{}
	var theories []QATheory
	var blockers []QABlocker
	for _, shard := range shards {
		if shard.AttemptID != qaMap.SemanticAttemptID {
			return QASynthesis{}, NewQAError(QAErrorStaleInput, "synthesize", "shard belongs to a different semantic attempt", nil)
		}
		if shard.Blocker != nil {
			blockers = append(blockers, *shard.Blocker)
		}
		for _, theory := range shard.Theories {
			if err := ValidateQATheory(theory); err != nil {
				return QASynthesis{}, NewQAError(QAErrorInvalidState, "synthesize", err.Error(), err)
			}
			shardByTheory[theory.ID] = shard
			theories = append(theories, theory)
		}
	}
	sort.Slice(theories, func(i, j int) bool { return theories[i].ID < theories[j].ID })
	theorySet := make(map[string]struct{}, len(theories))
	for _, theory := range theories {
		theorySet[theory.ID] = struct{}{}
	}
	challenges = append([]QAChallenge(nil), challenges...)
	if len(challenges) > qaMap.Budgets.FollowUpShards {
		return QASynthesis{}, NewQAError(QAErrorBudgetExhausted, "synthesize", "challenger record budget is exhausted", nil)
	}
	for i := range challenges {
		challenges[i].TheoryIDs = normalizeQAStrings(challenges[i].TheoryIDs)
		challenges[i].EvidenceRefs = normalizeQAStrings(challenges[i].EvidenceRefs)
		if err := ValidateQAChallenge(challenges[i], qaMap.Budgets); err != nil || challenges[i].MapID != qaMap.ID {
			return QASynthesis{}, NewQAError(QAErrorInvalidState, "synthesize", "challenger record is invalid or stale", err)
		}
		for _, theoryID := range challenges[i].TheoryIDs {
			if _, ok := theorySet[theoryID]; !ok {
				return QASynthesis{}, NewQAError(QAErrorStaleInput, "synthesize", "challenger record references a theory outside the current map", nil)
			}
		}
	}
	sort.Slice(challenges, func(i, j int) bool { return challenges[i].ID < challenges[j].ID })
	theoryIDs := make([]string, 0, len(theories))
	outcomeCounts := map[QATheoryOutcome]int{}
	groups := map[string][]QATheory{}
	for _, theory := range theories {
		theoryIDs = append(theoryIDs, theory.ID)
		outcomeCounts[theory.Outcome]++
		groups[qaTheoryEquivalenceKey(theory)] = append(groups[qaTheoryEquivalenceKey(theory)], theory)
	}
	deduplicated := map[string][]string{}
	var contradictions [][]string
	var followCandidates []QATheory
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		group := groups[key]
		ids := make([]string, 0, len(group))
		confirmed, refuted := false, false
		for _, theory := range group {
			ids = append(ids, theory.ID)
			confirmed = confirmed || theory.Outcome == QATheoryConfirmed
			refuted = refuted || theory.Outcome == QATheoryRefuted
			if theory.Outcome == QATheoryCrossShard || theory.Outcome == QATheoryInconclusive {
				followCandidates = append(followCandidates, theory)
			}
		}
		sort.Strings(ids)
		if len(ids) > 1 {
			deduplicated[ids[0]] = append([]string(nil), ids[1:]...)
		}
		if confirmed && refuted {
			contradictions = append(contradictions, ids)
		}
	}
	sort.Slice(contradictions, func(i, j int) bool {
		return strings.Join(contradictions[i], "\x00") < strings.Join(contradictions[j], "\x00")
	})
	interactions := qaSynthesisInteractions(theories, shardByTheory)
	sort.Slice(followCandidates, func(i, j int) bool { return followCandidates[i].ID < followCandidates[j].ID })
	if len(followCandidates) > qaMap.Budgets.FollowUpShards {
		followCandidates = followCandidates[:qaMap.Budgets.FollowUpShards]
	}
	followUps := make([]QAShard, 0, len(followCandidates))
	for _, theory := range followCandidates {
		parent := shardByTheory[theory.ID]
		context := qaFollowUpContext(parent)
		if len(context) > qaMap.Budgets.ContextPathsPerShard {
			context = context[:qaMap.Budgets.ContextPathsPerShard]
		}
		identity := QAShardIdentity{Kind: QAShardFollowUp, ContextPaths: context, BehavioralConcerns: []string{"follow up: " + theory.Claim}, ExpectationRefs: normalizeQAStrings(theory.ExpectationRefs), ParentTheoryIDs: []string{theory.ID}}
		id, err := NewQAShardID(qaMap.Project, qaMap.Sprint, qaMap.ID, identity)
		if err != nil {
			return QASynthesis{}, err
		}
		followUps = append(followUps, QAShard{SchemaVersion: 1, ID: id, AttemptID: qaMap.SemanticAttemptID, Kind: QAShardFollowUp, Title: "Follow up " + theory.ID, ContextPaths: context, BehavioralConcerns: identity.BehavioralConcerns, ExpectationRefs: identity.ExpectationRefs, ParentTheoryIDs: identity.ParentTheoryIDs, Phase: QAPhaseMapped})
	}
	sort.Slice(followUps, func(i, j int) bool { return followUps[i].ID < followUps[j].ID })
	followIDs := make([]string, 0, len(followUps))
	for _, shard := range followUps {
		followIDs = append(followIDs, shard.ID)
	}
	challengeIDs := make([]string, 0, len(challenges))
	for _, challenge := range challenges {
		challengeIDs = append(challengeIDs, challenge.ID)
	}
	synthesisID, err := NewQASynthesisID(qaMap.Project, qaMap.Sprint, qaMap.SemanticAttemptID, QASynthesisIdentity{MapID: qaMap.ID, TheoryIDs: theoryIDs, ChallengeIDs: challengeIDs, FollowUpIDs: followIDs, PolicyFingerprint: qaMap.PolicyFingerprint})
	if err != nil {
		return QASynthesis{}, err
	}
	next := "Inspect the retained theory outcomes."
	if len(followUps) > 0 {
		next = "Run the bounded parent-linked follow-up shards."
	} else if len(blockers) > 0 {
		next = "Inspect the retained shard blockers and resume only after their stated prerequisites are restored."
	}
	sort.Slice(blockers, func(i, j int) bool {
		return strings.Join([]string{string(blockers[i].Category), blockers[i].Scope, blockers[i].Summary}, "\x00") < strings.Join([]string{string(blockers[j].Category), blockers[j].Scope, blockers[j].Summary}, "\x00")
	})
	result := QASynthesis{SchemaVersion: 1, ID: synthesisID, AttemptID: qaMap.SemanticAttemptID, MapID: qaMap.ID, TheoryIDs: theoryIDs, Challenges: challenges, Deduplicated: deduplicated, Contradictions: contradictions, Interactions: interactions, FollowUpShards: followUps, OutcomeCounts: outcomeCounts, Blockers: blockers, NextAction: next}
	if len(result.Deduplicated) == 0 {
		result.Deduplicated = nil
	}
	if err := ValidateQASynthesis(result, qaMap.Budgets); err != nil {
		return QASynthesis{}, NewQAError(QAErrorInvalidState, "synthesize", err.Error(), err)
	}
	return result, nil
}

func qaFollowUpContext(parent QAShard) []string {
	var requested []string
	for _, attempt := range parent.Attempts {
		for _, request := range attempt.ContextRequests {
			if request.Approved {
				requested = append(requested, request.Paths...)
			}
		}
	}
	requested = normalizeQAStrings(requested)
	inherited := normalizeQAStrings(append(append([]string(nil), parent.ChangedPaths...), parent.ContextPaths...))
	seen := make(map[string]struct{}, len(requested)+len(inherited))
	context := make([]string, 0, len(requested)+len(inherited))
	for _, paths := range [][]string{requested, inherited} {
		for _, path := range paths {
			if _, duplicate := seen[path]; duplicate {
				continue
			}
			seen[path] = struct{}{}
			context = append(context, path)
		}
	}
	return context
}

func NormalizedQASynthesisBytes(value QASynthesis) ([]byte, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func qaTheoryEquivalenceKey(theory QATheory) string {
	refs := normalizeQAStrings(theory.ExpectationRefs)
	return strings.ToLower(strings.Join([]string{strings.TrimSpace(theory.Claim), strings.TrimSpace(theory.VerificationSurface), strings.Join(refs, "\x00")}, "\x00"))
}

func qaSynthesisInteractions(theories []QATheory, owners map[string]QAShard) []string {
	var interactions []string
	for i := 0; i < len(theories); i++ {
		for j := i + 1; j < len(theories); j++ {
			left, right := owners[theories[i].ID], owners[theories[j].ID]
			if left.ID == right.ID {
				continue
			}
			if theories[i].Outcome == QATheoryCrossShard || theories[j].Outcome == QATheoryCrossShard || shareQAString(left.RiskTags, right.RiskTags) {
				interactions = append(interactions, fmt.Sprintf("%s <-> %s", theories[i].ID, theories[j].ID))
			}
		}
	}
	return normalizeQAStrings(interactions)
}

func shareQAString(left, right []string) bool {
	set := map[string]struct{}{}
	for _, value := range left {
		set[value] = struct{}{}
	}
	for _, value := range right {
		if _, ok := set[value]; ok {
			return true
		}
	}
	return false
}
