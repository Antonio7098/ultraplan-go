package sprint

import (
	"context"
	"errors"
	"sort"
)

// QAAdjudicationReplayResult describes a runtime-free replay of retained QA
// facts. Replay never runs investigators, models, authored tests, or checks.
type QAAdjudicationReplayResult struct {
	AttemptID             string `json:"attempt_id"`
	SourceAdjudicationID  string `json:"source_adjudication_id"`
	AdjudicationID        string `json:"adjudication_id"`
	CandidateCount        int    `json:"candidate_count"`
	PromotedCount         int    `json:"promoted_count"`
	UnpromotedCount       int    `json:"unpromoted_count"`
	AcceptedEvidenceCount int    `json:"accepted_evidence_count"`
}

// ReplayQAAdjudication re-applies the current deterministic promotion gate to
// the current attempt's retained facts. It permits a QA policy change, but all
// governed inputs, implementation bytes, review, and check catalog must still
// match the frozen attempt.
func (s Service) ReplayQAAdjudication(ctx context.Context, projectRef, sprintRef string) (QAAdjudicationReplayResult, error) {
	lockedCtx, release, err := s.acquireMutationContext(ctx, projectRef, sprintRef)
	if err != nil {
		return QAAdjudicationReplayResult{}, err
	}
	defer release()
	if err := lockedCtx.Err(); err != nil {
		return QAAdjudicationReplayResult{}, err
	}
	sp, err := s.resolveMutationSprint(projectRef, sprintRef)
	if err != nil {
		return QAAdjudicationReplayResult{}, err
	}
	store := NewQAStore(s.root, sp)
	state, err := store.LoadState()
	if err != nil {
		return QAAdjudicationReplayResult{}, err
	}
	if state.CurrentAttemptID == "" || state.Adjudication == nil || state.Assessment == nil || state.Synthesis == nil {
		return QAAdjudicationReplayResult{}, NewQAError(QAErrorInvalidState, "replay adjudication", "the current QA attempt has no complete adjudication inputs", nil)
	}
	qaMap, err := store.LoadMap(state.CurrentAttemptID)
	if err != nil {
		return QAAdjudicationReplayResult{}, err
	}
	current, err := s.QAMap(projectRef, sprintRef)
	if err != nil {
		return QAAdjudicationReplayResult{}, err
	}
	if err := validateQAReplayIdentity(qaMap, current.Map); err != nil {
		return QAAdjudicationReplayResult{}, err
	}
	synthesis, err := store.LoadSynthesis(state.CurrentAttemptID, qaMap.Budgets)
	if err != nil {
		return QAAdjudicationReplayResult{}, err
	}
	prior, err := store.LoadAdjudication(state.CurrentAttemptID, qaMap.Budgets)
	if err != nil {
		return QAAdjudicationReplayResult{}, err
	}
	priorAssessment, err := store.LoadAssessment(state.CurrentAttemptID)
	if err != nil {
		return QAAdjudicationReplayResult{}, err
	}
	groups, err := store.LoadLatestArbiterSessionGroups(state.CurrentAttemptID)
	if err != nil {
		return QAAdjudicationReplayResult{}, err
	}
	if len(groups) == 0 {
		return QAAdjudicationReplayResult{}, NewQAError(QAErrorInvalidState, "replay adjudication", "the current attempt has no retained arbiter groups", nil)
	}
	var provisional []QAArbiterIssue
	for _, group := range groups {
		provisional = append(provisional, group.Issues...)
	}
	reconciled := deterministicQAArbiterIssueReconciliation(qaMap, provisional)
	if len(reconciled) == 0 {
		return QAAdjudicationReplayResult{}, NewQAError(QAErrorInvalidState, "replay adjudication", "retained arbiter groups contain no issue candidates", nil)
	}

	evidence, plans, err := loadQAReplayEvidence(store, qaMap, prior)
	if err != nil {
		return QAAdjudicationReplayResult{}, err
	}
	shards, err := loadQAReplayShards(store, qaMap, synthesis)
	if err != nil {
		return QAAdjudicationReplayResult{}, err
	}
	candidates := buildQARetainedCandidates(reconciled, shards, plans, evidence)
	settings, err := s.effectiveQASettings()
	if err != nil {
		return QAAdjudicationReplayResult{}, err
	}
	now := s.now().UTC()
	adjudication, err := AdjudicateQA(QAAdjudicationRequest{
		Project: qaMap.Project, Sprint: qaMap.Sprint, AttemptID: qaMap.SemanticAttemptID,
		MapFingerprint: prior.MapFingerprint, Plans: plans, Evidence: evidence,
		Candidates: candidates, Evaluators: prior.Evaluators, Budgets: qaMap.Budgets, Now: now,
		RepairAssignmentMode: settings.RepairAssignmentMode, IssuesPerRepairAgent: settings.IssuesPerRepairAgent,
	})
	if err != nil {
		return QAAdjudicationReplayResult{}, err
	}
	adjudication.Replay = &QAAdjudicationReplay{
		SourceAdjudicationID: prior.ID, SourceAssessmentID: priorAssessment.ID,
		SourcePolicyFingerprint: qaMap.PolicyFingerprint, AppliedPolicyFingerprint: current.Map.PolicyFingerprint,
		Reason: "reapplied deterministic candidate coverage and promotion rules to retained evidence",
	}
	adjudication.ID, err = NewQAV2ID("adjudication", qaMap.Project, qaMap.Sprint, qaMap.SemanticAttemptID, struct {
		Base, Source string
	}{adjudication.ID, prior.ID})
	if err != nil {
		return QAAdjudicationReplayResult{}, err
	}
	assessmentValue, nextAction := DeriveQAAssessment(VerificationStage{Fresh: true, ExecutionStatus: string(ReviewCompleted), Verdict: string(priorAssessment.ReviewVerdict)}, evidence, adjudication, priorAssessment.Blockers)
	assessmentID, err := NewQAV2ID("assessment", qaMap.Project, qaMap.Sprint, qaMap.SemanticAttemptID, struct {
		Adjudication string
		Assessment   OverallAssessment
	}{adjudication.ID, assessmentValue})
	if err != nil {
		return QAAdjudicationReplayResult{}, err
	}
	assessment := QAAssessmentRecord{
		SchemaVersion: QAEvidenceSchemaVersion, ID: assessmentID, AttemptID: qaMap.SemanticAttemptID,
		ReviewVerdict: priorAssessment.ReviewVerdict, ReviewFingerprint: priorAssessment.ReviewFingerprint,
		Assessment: assessmentValue, EvidenceTotal: len(evidence), RejectedTotal: len(adjudication.Rejected),
		CandidateTotal: len(adjudication.Issues) + len(adjudication.Unpromoted), UnpromotedTotal: len(adjudication.Unpromoted), IssueTotal: len(adjudication.Issues),
		Blockers: append([]QABlocker(nil), priorAssessment.Blockers...), NextAction: nextAction, CompletedAt: now,
	}
	report, err := RenderQAReport(qaMap.Project, qaMap.Sprint, qaMap.GovernedInputFingerprint, evidence, adjudication, assessment)
	if err != nil {
		return QAAdjudicationReplayResult{}, err
	}
	flow, err := LoadFlowState(s.root, sp)
	if err != nil {
		return QAAdjudicationReplayResult{}, err
	}
	token := QAWriterToken{RunID: "qa-adjudication-replay", OperationalAttemptID: adjudication.ID, FencingGeneration: 1}
	store = store.WithWriterFence(func(got QAWriterToken) error {
		if got != token {
			return errors.New("adjudication replay writer token mismatch")
		}
		return nil
	})
	state.SchemaVersion = QAStateSchemaVersion
	state.Phase = QAPhaseCompleted
	state.Run.Lifecycle, state.Run.TerminalResult = QARunTerminal, QATerminalCompleted
	if assessment.Assessment == AssessmentFail || assessment.Assessment == AssessmentBlocked || assessment.Assessment == AssessmentIncomplete {
		state.Phase, state.Run.TerminalResult = QAPhaseBlocked, QATerminalBlocked
	}
	state.Freshness.Current = true
	state.Freshness.Reasons = nil
	state.Freshness.PolicyFingerprint = current.Map.PolicyFingerprint
	state.CanonicalAssessment, state.NextAction, state.UpdatedAt = assessment.Assessment, assessment.NextAction, now
	bundle := QAEvidencePublication{Plans: plans, Records: evidence, Adjudication: &adjudication, Assessment: &assessment, Report: report, Budgets: qaMap.Budgets}
	if err := store.Publish(QAPublication{State: state, Flow: flow, Evidence: &bundle}, token); err != nil {
		return QAAdjudicationReplayResult{}, err
	}
	return QAAdjudicationReplayResult{
		AttemptID: qaMap.SemanticAttemptID, SourceAdjudicationID: prior.ID, AdjudicationID: adjudication.ID,
		CandidateCount: len(adjudication.Issues) + len(adjudication.Unpromoted), PromotedCount: len(adjudication.Issues),
		UnpromotedCount: len(adjudication.Unpromoted), AcceptedEvidenceCount: len(adjudication.AcceptedIDs),
	}, nil
}

func validateQAReplayIdentity(retained, current QAMap) error {
	if retained.Project != current.Project || retained.Sprint != current.Sprint ||
		retained.GovernedInputFingerprint != current.GovernedInputFingerprint ||
		retained.ImplementationFingerprint != current.ImplementationFingerprint ||
		retained.ReviewFingerprint != current.ReviewFingerprint ||
		retained.CheckCatalogFingerprint != current.CheckCatalogFingerprint ||
		retained.Target.Fingerprint != current.Target.Fingerprint {
		return NewQAError(QAErrorStaleInput, "replay adjudication", "retained QA facts no longer describe the current governed inputs, implementation, review, or check catalog", nil)
	}
	return nil
}

func loadQAReplayEvidence(store QAStore, qaMap QAMap, prior QAAdjudication) ([]QAEvidenceRecord, []QAEvidencePlan, error) {
	ids := append([]string(nil), prior.AcceptedIDs...)
	for _, rejected := range prior.Rejected {
		ids = append(ids, rejected.EvidenceID)
	}
	ids = normalizeQAStrings(ids)
	records := make([]QAEvidenceRecord, 0, len(ids))
	planByID := make(map[string]QAEvidencePlan)
	for _, id := range ids {
		record, err := store.LoadEvidence(qaMap.SemanticAttemptID, id)
		if err != nil {
			return nil, nil, err
		}
		records = append(records, record)
		if _, ok := planByID[record.PlanID]; ok {
			continue
		}
		plan, err := store.LoadEvidencePlan(qaMap.SemanticAttemptID, record.PlanID, qaMap.Budgets)
		if err != nil {
			return nil, nil, err
		}
		planByID[plan.ID] = plan
	}
	plans := make([]QAEvidencePlan, 0, len(planByID))
	for _, plan := range planByID {
		plans = append(plans, plan)
	}
	sort.Slice(plans, func(i, j int) bool { return plans[i].ID < plans[j].ID })
	return records, plans, nil
}

func loadQAReplayShards(store QAStore, qaMap QAMap, synthesis QASynthesis) ([]QAShard, error) {
	all := append([]QAShard(nil), qaMap.Shards...)
	all = append(all, synthesis.FollowUpShards...)
	seen := make(map[string]bool)
	loaded := make([]QAShard, 0, len(all))
	for _, shard := range all {
		if seen[shard.ID] {
			continue
		}
		seen[shard.ID] = true
		value, err := store.LoadShard(qaMap.SemanticAttemptID, shard.ID)
		if err != nil {
			return nil, err
		}
		loaded = append(loaded, value)
	}
	return loaded, nil
}

func buildQARetainedCandidates(issues []QAArbiterIssue, shards []QAShard, plans []QAEvidencePlan, evidence []QAEvidenceRecord) []QAIssueCandidate {
	candidates := make(map[string]QAIssueCandidate, len(issues))
	issueByTheory := make(map[string]QAArbiterIssue)
	theoryByID := make(map[string]QATheory)
	for _, shard := range shards {
		for _, theory := range shard.Theories {
			theoryByID[theory.ID] = theory
		}
	}
	for _, issue := range issues {
		candidates[issue.ID] = QAIssueCandidate{ID: issue.ID, TheoryIDs: append([]string(nil), issue.TheoryIDs...), Claim: issue.Claim, Title: issue.Title, IssueClass: issue.IssueClass, Severity: issue.Severity, Location: issue.Location, EvidenceByTheory: map[string][]string{}}
		for _, id := range issue.TheoryIDs {
			issueByTheory[id] = issue
		}
	}
	planByID := make(map[string]QAEvidencePlan, len(plans))
	for _, plan := range plans {
		planByID[plan.ID] = plan
	}
	for _, record := range evidence {
		if record.Outcome != QAEvidenceFail {
			continue
		}
		plan := planByID[record.PlanID]
		if len(plan.TheoryIDs) == 0 {
			location := "retained-check"
			if len(plan.ApprovedPaths) > 0 {
				location = plan.ApprovedPaths[0]
			}
			key := plan.ShardID + "\x00" + plan.CheckID
			candidates[key] = QAIssueCandidate{Claim: "approved check " + plan.CheckID + " failed in the isolated copy", Title: "Approved QA check failed", IssueClass: "behavior", Severity: "medium", Location: location, EvidenceIDs: []string{record.ID}, RepairEligible: true, RegressionCandidate: true}
			continue
		}
		for _, theoryID := range plan.TheoryIDs {
			key := theoryID
			candidate := candidates[key]
			if issue, ok := issueByTheory[theoryID]; ok {
				key, candidate = issue.ID, candidates[issue.ID]
			} else if theory, ok := theoryByID[theoryID]; ok {
				candidate = QAIssueCandidate{TheoryIDs: []string{theory.ID}, Claim: theory.Claim, Title: theory.Claim, IssueClass: "behavior", Severity: theory.SeverityIfConfirmed, Location: theory.VerificationSurface}
			}
			candidate.RepairEligible, candidate.RegressionCandidate = true, true
			candidate = addQACandidateEvidence(candidate, plan.TheoryIDs, record.ID)
			candidates[key] = candidate
		}
	}
	keys := make([]string, 0, len(candidates))
	for key := range candidates {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]QAIssueCandidate, 0, len(keys))
	for _, key := range keys {
		candidate := candidates[key]
		candidate.TheoryIDs = normalizeQAStrings(candidate.TheoryIDs)
		candidate.EvidenceIDs = normalizeQAStrings(candidate.EvidenceIDs)
		result = append(result, candidate)
	}
	return result
}
