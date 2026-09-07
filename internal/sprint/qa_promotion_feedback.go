package sprint

import (
	"fmt"
	"sort"
	"strings"
)

// ensureQAPromotionRequests closes the gap between semantic confirmation and
// executable promotion. Model omissions cannot silently bypass evidence work.
// Requests use the investigator's frozen conditions, not a new invented claim.
func ensureQAPromotionRequests(qaMap QAMap, arbitration *QAArbitration, shards []QAShard, tests []QATestPublication) error {
	covered := map[string]bool{}
	for _, test := range tests {
		if !qaUsableReproduction(qaMap, test) || test.Runs[len(test.Runs)-1].Outcome != QAEvidenceFail {
			continue
		}
		for _, id := range test.Spec.TheoryIDs {
			covered[id] = true
		}
	}
	requested := map[string]bool{}
	for _, request := range arbitration.EvidenceRequests {
		for _, id := range request.TheoryIDs {
			requested[id] = true
		}
	}
	theories := map[string]QATheory{}
	for _, shard := range shards {
		for _, theory := range shard.Theories {
			theories[theory.ID] = theory
		}
	}
	for _, group := range arbitration.Groups {
		targets := map[string]QATheoryOutcome{}
		for _, id := range group.TheoryIDs {
			targets[id] = theories[id].Outcome
		}
		for _, override := range group.Overrides {
			for _, id := range override.TheoryIDs {
				targets[id] = override.Outcome
			}
		}
		for _, issue := range group.Issues {
			for _, id := range issue.TheoryIDs {
				targets[id] = QATheoryConfirmed
			}
		}
		ids := make([]string, 0, len(targets))
		for id := range targets {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			switch targets[id] {
			case QATheoryConfirmed, QATheoryInconclusive, QATheoryCrossShard:
			default:
				continue
			}
			if covered[id] && targets[id] == QATheoryConfirmed || requested[id] {
				continue
			}
			theory, ok := theories[id]
			if !ok {
				return fmt.Errorf("promotion candidate references unknown theory %q", id)
			}
			request := QAArbiterEvidenceRequest{AccountingVersion: qaMap.EvidenceAccountingVersion,
				ArbiterGroupID: group.ID, OriginShardID: theory.ShardID, TheoryIDs: []string{id},
				Gap:                 "The theory needs sufficient executable evidence for a final decision: " + theory.Claim,
				RequestedEvidence:   "Create an executable test of the claim at " + theory.VerificationSurface + ". Exercise the actual claimed entry point. Use a contract or static assertion if behavioral reproduction is unsuitable. Preserve the frozen requirements; report why verification is unavailable if no valid check can be constructed.",
				RequiredObservation: "Confirm only if: " + theory.ConfirmationCondition + ". Refute only if: " + theory.RefutationCondition + ". Otherwise: " + theory.InconclusiveCondition,
				ControlRequirement:  "Include a passing control reaching the same entry point and assert fixture preconditions before the defect assertion.",
				Priority:            "high",
			}
			var err error
			// If the arbiter still finds a returned result inconclusive, a new
			// evidence revision must not reuse the already answered request.
			revision := ""
			if targets[id] != QATheoryConfirmed {
				revision, _ = fingerprintQAValue(theory.Evidence)
			}
			request.ID, err = NewQAV2ID("request", qaMap.Project, qaMap.Sprint, qaMap.ID, struct {
				Theory, Claim, Confirm, Refute, Revision string
			}{id, theory.Claim, theory.ConfirmationCondition, theory.RefutationCondition, revision})
			if err != nil {
				return err
			}
			arbitration.EvidenceRequests = append(arbitration.EvidenceRequests, request)
			requested[id] = true
		}
	}
	sort.Slice(arbitration.EvidenceRequests, func(i, j int) bool { return arbitration.EvidenceRequests[i].ID < arbitration.EvidenceRequests[j].ID })
	return nil
}

func qaUsableReproduction(qaMap QAMap, test QATestPublication) bool {
	if len(test.Runs) == 0 || test.Spec.AttemptID != qaMap.SemanticAttemptID || test.Spec.ImplementationFingerprint != qaMap.ImplementationFingerprint {
		return false
	}
	run := test.Runs[len(test.Runs)-1]
	return run.TargetIdentity == qaMap.ImplementationFingerprint && run.Cleanup.Complete &&
		(run.Outcome == QAEvidenceFail || run.Outcome == QAEvidencePass) &&
		ValidateQAReproductionSpec(test.Spec, qaMap.Budgets) == nil &&
		ValidateQATestBundle(test.Bundle, test.Spec, qaMap.Budgets) == nil &&
		ValidateQAReproductionRun(run, test.Spec, test.Bundle) == nil
}

func qaTestAnswersRequest(test QATestPublication, request QAArbiterEvidenceRequest) bool {
	// Legacy specs have no exact request binding. Keep them as historical
	// evidence, but do not use them to close a different/new evidence request.
	return test.Spec.EvidenceRequestID == request.ID && request.ID != "" &&
		test.Spec.ShardID == request.OriginShardID && len(request.TheoryIDs) > 0 &&
		sharedQAStrings(test.Spec.TheoryIDs, request.TheoryIDs) == len(request.TheoryIDs)
}

func qaLatestRequestTest(tests []QATestPublication, request QAArbiterEvidenceRequest) *QATestPublication {
	var latest *QATestPublication
	for i := range tests {
		test := &tests[i]
		if !qaTestAnswersRequest(*test, request) || len(test.Runs) == 0 {
			continue
		}
		if latest == nil || test.Runs[len(test.Runs)-1].CompletedAt.After(latest.Runs[len(latest.Runs)-1].CompletedAt) {
			latest = test
		}
	}
	return latest
}

func qaEvidenceRoundsUsed(shards []QAShard, requests []QAArbiterEvidenceRequest) map[string]int {
	rounds := map[string]int{}
	for _, request := range requests {
		if request.EvidenceRound > rounds[request.OriginShardID] {
			rounds[request.OriginShardID] = request.EvidenceRound
		}
	}
	// Older artifacts persisted authoring attempts but not request counters.
	for _, shard := range shards {
		for _, attempt := range shard.Attempts {
			if strings.Contains(attempt.ID, "/evidence/") && attempt.Number-1 > rounds[shard.ID] {
				rounds[shard.ID] = attempt.Number - 1
			}
		}
	}
	return rounds
}

func stopQAEvidenceRequest(request *QAArbiterEvidenceRequest, reason string) {
	request.Status, request.ReasonCode = "inconclusive", reason
	request.NextAction = "Evidence remains unresolved: " + reason + ". Inspect the retained diagnostic and required observation."
	if reason == "infrastructure_retry_budget_exhausted" && request.RecoveryAllowance > 0 {
		request.NextAction += " The recorded recovery allowance is spent. Correct the prerequisite and start a new governed attempt."
	} else if qaRetryableReason(reason) || reason == "investigator_workspace_unavailable" || reason == "reproduction_workspace_unavailable" || reason == "infrastructure_retry_budget_exhausted" {
		request.NextAction += " Restore the prerequisite, then run qa retry-infrastructure for a bounded recovery."
	} else if strings.Contains(reason, "budget_exhausted") {
		request.NextAction += " Infrastructure-blocked work may qualify for qa retry-infrastructure; other exhausted work requires a new governed attempt."
	}
}

// A later arbitration round may replace an earlier question with a stronger
// one. Record that edge, but keep the old request blocking until its replacement
// is answered. Never let an earlier test discharge a newer request.
func linkQAReplacementRequests(history, next []QAArbiterEvidenceRequest) {
	active := map[string]bool{}
	known := map[string]bool{}
	for _, request := range next {
		active[request.ID] = true
	}
	for _, request := range history {
		known[request.ID] = true
	}
	for i := range history {
		old := &history[i]
		if active[old.ID] || old.SupersededBy != "" {
			continue
		}
		for _, replacement := range next {
			// Only point forward to a newly introduced request; this prevents
			// cycles if an arbiter later repeats an older question.
			if known[replacement.ID] || old.OriginShardID != replacement.OriginShardID || len(old.TheoryIDs) == 0 || sharedQAStrings(old.TheoryIDs, replacement.TheoryIDs) != len(old.TheoryIDs) {
				continue
			}
			old.SupersededBy = replacement.ID
			break
		}
	}
}
