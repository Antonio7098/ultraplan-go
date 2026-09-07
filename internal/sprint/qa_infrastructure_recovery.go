package sprint

import (
	"context"
	"fmt"
	"time"
)

// RetryQAInfrastructure reuses the frozen map, completed investigations and
// accepted executions. The same writer fence and mutation lease as resume apply.
func (s Service) RetryQAInfrastructure(ctx context.Context, project, sprint string, req QARunRequest) (QARunResult, error) {
	req.Resume, req.EvidenceProducing, req.InfrastructureOnly = true, true, true
	return s.RunQA(ctx, project, sprint, req)
}

func grantQAInfrastructureRecovery(requests []QAArbiterEvidenceRequest, tests []QATestPublication, now time.Time) map[string]bool {
	active := map[string]bool{}
	failedShard := map[string]bool{}
	for _, request := range requests {
		if qaInfrastructureRecoveryEligible(request, tests) {
			failedShard[request.OriginShardID] = true
		}
	}
	for _, test := range tests {
		for _, run := range test.Runs {
			single := test
			single.Runs = []QAReproductionRun{run}
			if qaTestInfrastructureBlocked(single) {
				failedShard[test.Spec.ShardID] = true
			}
		}
	}
	for i := range requests {
		request := &requests[i]
		if request.SupersededBy != "" || request.Status == "evidence_recorded" || request.Status == "superseded" {
			continue
		}
		eligible := qaInfrastructureRecoveryEligible(*request, tests)
		if request.Attempts == 0 && request.ReasonCode == "evidence_round_budget_exhausted" && failedShard[request.OriginShardID] {
			eligible = true
		}
		if !eligible {
			continue
		}
		if request.RecoveryGrantedAt == nil {
			request.RecoveryGrantedAt = &now
			request.RecoveryReason = request.ReasonCode
			if request.RecoveryReason == "" {
				request.RecoveryReason = "execution_interrupted"
			}
			request.RecoveryAllowance = 1
			request.AccountingVersion = 2
		}
		active[request.ID] = true
	}
	return active
}

func qaInfrastructureRecoveryEligible(request QAArbiterEvidenceRequest, tests []QATestPublication) bool {
	if request.Status == "evidence_recorded" || request.Status == "superseded" {
		return false
	}
	if request.Failure != nil && request.Failure.Retryable {
		return true
	}
	if test := qaLatestRequestBundle(tests, request); test != nil {
		return qaTestInfrastructureBlocked(*test)
	}
	// These old records lost the underlying errno. Allow one explicitly recorded
	// prerequisite retry, never classify them as proven disk or network failures.
	switch request.ReasonCode {
	case "investigator_workspace_unavailable", "reproduction_workspace_unavailable":
		return true
	}
	return false
}

type qaRetainedCheck struct {
	plan   QAEvidencePlan
	record QAEvidenceRecord
}

func loadQARetainedChecks(store QAStore, qaMap QAMap) ([]qaRetainedCheck, error) {
	adjudication, err := store.LoadAdjudication(qaMap.SemanticAttemptID, qaMap.Budgets)
	if err != nil {
		return nil, err
	}
	var retained []qaRetainedCheck
	for _, id := range adjudication.AcceptedIDs {
		record, err := store.LoadEvidence(qaMap.SemanticAttemptID, id)
		if err != nil {
			return nil, err
		}
		plan, err := store.LoadEvidencePlan(qaMap.SemanticAttemptID, record.PlanID, qaMap.Budgets)
		if err != nil {
			return nil, err
		}
		if plan.Kind == QACheckFact {
			retained = append(retained, qaRetainedCheck{plan, record})
		}
	}
	return retained, nil
}

func retainedQACheckForPlan(retained []qaRetainedCheck, plan QAEvidencePlan) (QAEvidenceRecord, bool) {
	for _, check := range retained {
		old := check.plan
		if old.AttemptID != plan.AttemptID || old.ImplementationFingerprint != plan.ImplementationFingerprint || old.GovernedInputFingerprint != plan.GovernedInputFingerprint || old.MapFingerprint != plan.MapFingerprint {
			continue
		}
		old.ID, old.TheoryIDs, old.FrozenAt = plan.ID, plan.TheoryIDs, plan.FrozenAt
		left, _ := fingerprintQAValue(old)
		right, _ := fingerprintQAValue(plan)
		if left == right {
			return check.record, true
		}
	}
	return QAEvidenceRecord{}, false
}

func qaRecoveryMissingCheck(plan QAEvidencePlan) error {
	return NewQAError(QAErrorAdmissionBlocked, "retry infrastructure", fmt.Sprintf("no accepted retained execution for check %s; use ordinary qa resume to run other checks", plan.CheckID), nil)
}
