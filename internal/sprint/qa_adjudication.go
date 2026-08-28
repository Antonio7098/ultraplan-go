package sprint

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

type QAIssueCandidate struct {
	Claim               string
	Title               string
	IssueClass          string
	Severity            string
	Location            string
	EvidenceIDs         []string
	RepairEligible      bool
	RegressionCandidate bool
}

type QAAdjudicationRequest struct {
	Project              string
	Sprint               string
	AttemptID            string
	MapFingerprint       string
	Plans                []QAEvidencePlan
	Evidence             []QAEvidenceRecord
	Candidates           []QAIssueCandidate
	Evaluators           []QAModelObservation
	Budgets              QABudgets
	Now                  time.Time
	RepairAssignmentMode string
	IssuesPerRepairAgent int
}

// AdjudicateQA is pure. It consumes already frozen records and never reads
// files, executes commands, or grants model prose authority over promotion.
func AdjudicateQA(req QAAdjudicationRequest) (QAAdjudication, error) {
	if !safeQAName(req.Project) || !safeQAName(req.Sprint) || !validQAIDKind(req.AttemptID, "attempt") || !validFingerprint(req.MapFingerprint) {
		return QAAdjudication{}, fmt.Errorf("invalid QA adjudication scope")
	}
	if err := validateQABudgets(req.Budgets); err != nil {
		return QAAdjudication{}, err
	}
	if len(req.Evidence) > req.Budgets.EvidenceRecords || len(req.Candidates) > req.Budgets.Issues {
		return QAAdjudication{}, NewQAError(QAErrorBudgetExhausted, "adjudicate", "evidence or issue count exceeds the frozen budget", nil)
	}
	evaluatorSessions := make(map[string]struct{}, len(req.Evaluators))
	evaluatorCalls := make(map[string]struct{}, len(req.Evaluators))
	for _, evaluator := range req.Evaluators {
		if !evaluator.Valid || !validFingerprint(evaluator.EvidenceDigest) || evaluator.CallID == "" || evaluator.SessionID == "" || evaluator.Outcome != QAEvidencePass && evaluator.Outcome != QAEvidenceFail {
			return QAAdjudication{}, NewQAError(QAErrorMalformedEvidence, "adjudicate", "an evaluator observation is incomplete or invalid", nil)
		}
		if _, exists := evaluatorSessions[evaluator.SessionID]; exists {
			return QAAdjudication{}, NewQAError(QAErrorMalformedEvidence, "adjudicate", "evaluator sessions are not fresh", nil)
		}
		if _, exists := evaluatorCalls[evaluator.CallID]; exists {
			return QAAdjudication{}, NewQAError(QAErrorMalformedEvidence, "adjudicate", "evaluator call identities are not unique", nil)
		}
		evaluatorSessions[evaluator.SessionID] = struct{}{}
		evaluatorCalls[evaluator.CallID] = struct{}{}
	}
	plans := make(map[string]QAEvidencePlan, len(req.Plans))
	for _, plan := range req.Plans {
		if err := ValidateQAEvidencePlan(plan, req.Budgets); err != nil || plan.AttemptID != req.AttemptID || plan.MapFingerprint != req.MapFingerprint {
			return QAAdjudication{}, NewQAError(QAErrorMalformedEvidence, "adjudicate", "invalid or stale evidence plan", err)
		}
		plans[plan.ID] = plan
	}
	accepted := make(map[string]QAEvidenceRecord, len(req.Evidence))
	rejected := make([]QARejectedEvidence, 0)
	for _, record := range req.Evidence {
		plan, ok := plans[record.PlanID]
		if !ok {
			rejected = append(rejected, rejectEvidence(record.ID, "plan_missing", "the frozen evidence plan is unavailable"))
			continue
		}
		if err := ValidateQAEvidence(record, plan, req.Budgets); err != nil {
			rejected = append(rejected, rejectEvidence(record.ID, "evidence_invalid", err.Error()))
			continue
		}
		if record.Outcome == QAEvidenceBlocked {
			rejected = append(rejected, rejectEvidence(record.ID, "evidence_blocked", "the evidence attempt did not complete"))
			continue
		}
		// A fact check has deterministic sufficiency when it executes the frozen,
		// product-owned command to completion. Behavioral checks still need
		// repeatability before a failure can be admitted.
		if record.Outcome == QAEvidenceFail && !record.Repeatable && len(record.Commands) > 0 && plan.Kind != QACheckFact {
			rejected = append(rejected, rejectEvidence(record.ID, "not_repeatable", "failing behavioral evidence was not repeatable"))
			continue
		}
		accepted[record.ID] = record
	}

	type issueAggregate struct {
		title               string
		severity            string
		repairEligible      bool
		regressionCandidate bool
	}
	groups := map[string]*QARootCauseGroup{}
	issueAggregates := map[string]*issueAggregate{}
	for _, candidate := range req.Candidates {
		candidate.EvidenceIDs = normalizeQAStrings(candidate.EvidenceIDs)
		if strings.TrimSpace(candidate.Claim) == "" || strings.TrimSpace(candidate.Title) == "" || strings.TrimSpace(candidate.IssueClass) == "" || len(candidate.EvidenceIDs) == 0 {
			continue
		}
		admitted := true
		for _, evidenceID := range candidate.EvidenceIDs {
			record, ok := accepted[evidenceID]
			if !ok || record.Outcome != QAEvidenceFail {
				admitted = false
				break
			}
		}
		if !admitted {
			continue
		}
		location := normalizeIssueLocation(candidate.Location)
		groupKey := strings.ToLower(strings.TrimSpace(candidate.Claim)) + "\x00" + strings.ToLower(strings.TrimSpace(candidate.IssueClass)) + "\x00" + location
		groupID, err := NewQAV2ID("group", req.Project, req.Sprint, req.AttemptID, groupKey)
		if err != nil {
			return QAAdjudication{}, err
		}
		group := groups[groupID]
		if group == nil {
			group = &QARootCauseGroup{ID: groupID, Claim: strings.TrimSpace(candidate.Claim), IssueClass: strings.TrimSpace(candidate.IssueClass), Location: location}
			groups[groupID] = group
		}
		group.EvidenceIDs = normalizeQAStrings(append(group.EvidenceIDs, candidate.EvidenceIDs...))
		title := strings.TrimSpace(candidate.Title)
		severity := normalizeQASeverity(candidate.Severity)
		aggregate := issueAggregates[groupID]
		if aggregate == nil {
			aggregate = &issueAggregate{title: title, severity: severity}
			issueAggregates[groupID] = aggregate
		} else {
			if title < aggregate.title {
				aggregate.title = title
			}
			if qaSeverityRank(severity) > qaSeverityRank(aggregate.severity) {
				aggregate.severity = severity
			}
		}
		aggregate.repairEligible = aggregate.repairEligible || candidate.RepairEligible
		aggregate.regressionCandidate = aggregate.regressionCandidate || candidate.RegressionCandidate
	}

	groupIDs := make([]string, 0, len(groups))
	for groupID := range groups {
		groupIDs = append(groupIDs, groupID)
	}
	sort.Strings(groupIDs)
	groupList := make([]QARootCauseGroup, 0, len(groupIDs))
	issues := make([]QAIssue, 0, len(groupIDs))
	for _, groupID := range groupIDs {
		group := groups[groupID]
		aggregate := issueAggregates[groupID]
		issueID, err := NewQAV2ID("issue", req.Project, req.Sprint, groupID, struct {
			Title       string
			EvidenceIDs []string
		}{aggregate.title, group.EvidenceIDs})
		if err != nil {
			return QAAdjudication{}, err
		}
		groupList = append(groupList, *group)
		issues = append(issues, QAIssue{ID: issueID, RootCauseGroupID: groupID, Title: aggregate.title, IssueClass: group.IssueClass, Severity: aggregate.severity, Location: group.Location, EvidenceIDs: group.EvidenceIDs, PromotionReason: "current contained failing evidence is repeatable or deterministically sufficient and satisfies the frozen plan", RepairEligible: aggregate.repairEligible, RegressionCandidate: aggregate.regressionCandidate})
	}
	repairGroups := suggestedRepairIssueGroups(groupList, issues)
	mode, perAgent := req.RepairAssignmentMode, req.IssuesPerRepairAgent
	if mode == "" {
		mode = "per_issue"
	}
	if perAgent == 0 {
		perAgent = 1
	}
	assignments, err := PlanRepairAssignments(QAAdjudication{Issues: issues, RepairGroups: repairGroups}, mode, perAgent)
	if err != nil {
		return QAAdjudication{}, err
	}
	sort.Slice(rejected, func(i, j int) bool {
		if rejected[i].EvidenceID == rejected[j].EvidenceID {
			return rejected[i].Code < rejected[j].Code
		}
		return rejected[i].EvidenceID < rejected[j].EvidenceID
	})
	acceptedRecords := make([]QAEvidenceRecord, 0, len(accepted))
	for _, record := range accepted {
		acceptedRecords = append(acceptedRecords, record)
	}
	acceptedIDs := sortedEvidenceIDs(acceptedRecords)
	completedAt := req.Now.UTC()
	if completedAt.IsZero() {
		completedAt = time.Unix(0, 0).UTC()
	}
	id, err := NewQAV2ID("adjudication", req.Project, req.Sprint, req.AttemptID, struct {
		Map      string
		Accepted []string
		Rejected []QARejectedEvidence
		Issues   []QAIssue
	}{req.MapFingerprint, acceptedIDs, rejected, issues})
	if err != nil {
		return QAAdjudication{}, err
	}
	return QAAdjudication{SchemaVersion: QAEvidenceSchemaVersion, ID: id, AttemptID: req.AttemptID, MapFingerprint: req.MapFingerprint, AcceptedIDs: acceptedIDs, Rejected: rejected, Groups: groupList, Issues: issues, RepairGroups: repairGroups, RepairAssignments: assignments, Evaluators: append([]QAModelObservation(nil), req.Evaluators...), CompletedAt: completedAt}, nil
}

func qaSeverityRank(severity string) int {
	switch normalizeQASeverity(severity) {
	case "critical":
		return 4
	case "high":
		return 3
	case "medium":
		return 2
	default:
		return 1
	}
}

func suggestedRepairIssueGroups(groups []QARootCauseGroup, issues []QAIssue) []QARepairIssueGroup {
	byRoot := make(map[string][]string)
	for _, issue := range issues {
		if issue.RepairEligible {
			byRoot[issue.RootCauseGroupID] = append(byRoot[issue.RootCauseGroupID], issue.ID)
		}
	}
	claims := make(map[string]string, len(groups))
	for _, group := range groups {
		claims[group.ID] = group.Claim
	}
	ids := make([]string, 0, len(byRoot))
	for id := range byRoot {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	result := make([]QARepairIssueGroup, 0, len(ids))
	for _, id := range ids {
		issueIDs := normalizeQAStrings(byRoot[id])
		groupID, _ := NewQAV2ID("group", "qa", "suggestion", id, struct {
			Purpose  string
			IssueIDs []string
		}{"repair-assignment", issueIDs})
		result = append(result, QARepairIssueGroup{ID: groupID, IssueIDs: issueIDs, Reason: "shared adjudicated root cause: " + strings.TrimSpace(claims[id])})
	}
	return result
}

// PlanRepairAssignments creates sequential worker queues. It never combines
// issues into one repair run.
func PlanRepairAssignments(adjudication QAAdjudication, mode string, issuesPerAgent int) ([]QARepairAssignment, error) {
	if mode != "per_issue" && mode != "grouped" {
		return nil, fmt.Errorf("repair assignment mode must be per_issue or grouped")
	}
	if issuesPerAgent < 1 || issuesPerAgent > 16 {
		return nil, fmt.Errorf("issues per repair agent must be between 1 and 16")
	}
	if mode == "per_issue" {
		issuesPerAgent = 1
	}
	var ordered []string
	reasons := map[string]string{}
	seen := map[string]bool{}
	for _, group := range adjudication.RepairGroups {
		for _, issueID := range group.IssueIDs {
			if !seen[issueID] {
				ordered = append(ordered, issueID)
				seen[issueID] = true
				reasons[issueID] = group.Reason
			}
		}
	}
	for _, issue := range adjudication.Issues {
		if issue.RepairEligible && !seen[issue.ID] {
			ordered = append(ordered, issue.ID)
			seen[issue.ID] = true
		}
	}
	var result []QARepairAssignment
	for len(ordered) > 0 {
		n := issuesPerAgent
		if n > len(ordered) {
			n = len(ordered)
		}
		chunk := append([]string(nil), ordered[:n]...)
		ordered = ordered[n:]
		assignment := QARepairAssignment{Agent: len(result) + 1, Issues: chunk}
		for _, id := range chunk {
			if reason := reasons[id]; reason != "" && !containsQAString(assignment.Reasons, reason) {
				assignment.Reasons = append(assignment.Reasons, reason)
			}
		}
		result = append(result, assignment)
	}
	return result, nil
}

func containsQAString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func AdjudicateFailedShard(evidenceDigest string, evaluators []QAModelObservation) (QAEvidenceOutcome, error) {
	if !validFingerprint(evidenceDigest) || len(evaluators) != 3 {
		return QAEvidenceBlocked, NewQAError(QAErrorMalformedEvidence, "evaluate shard", "exactly three evaluator results bound to one evidence digest are required", nil)
	}
	passes := 0
	sessions := map[string]struct{}{}
	for _, evaluator := range evaluators {
		if !evaluator.Valid || evaluator.EvidenceDigest != evidenceDigest || evaluator.CallID == "" || evaluator.SessionID == "" || !validEvidenceOutcome(evaluator.Outcome) || evaluator.Outcome == QAEvidenceBlocked {
			return QAEvidenceBlocked, NewQAError(QAErrorMalformedEvidence, "evaluate shard", "an evaluator result is incomplete or mismatched", nil)
		}
		if _, duplicate := sessions[evaluator.SessionID]; duplicate {
			return QAEvidenceBlocked, NewQAError(QAErrorMalformedEvidence, "evaluate shard", "evaluator sessions are not fresh", nil)
		}
		sessions[evaluator.SessionID] = struct{}{}
		if evaluator.Outcome == QAEvidencePass {
			passes++
		}
	}
	if passes >= 2 {
		return QAEvidencePass, nil
	}
	return QAEvidenceFail, nil
}

func DeriveQAAssessment(review VerificationStage, evidence []QAEvidenceRecord, adjudication QAAdjudication, smoke *VerificationStage, blockers []QABlocker) (OverallAssessment, string) {
	if !review.Fresh || review.ExecutionStatus != string(ReviewCompleted) {
		return AssessmentIncomplete, "Run the independent Conformance Review with current inputs."
	}
	if review.Verdict == string(ReviewFail) {
		return AssessmentFail, "Resolve Conformance Review findings before QA can pass."
	}
	if review.Verdict != string(ReviewPass) && review.Verdict != string(ReviewPassWithFindings) {
		return AssessmentBlocked, "Restore an acceptable Conformance Review verdict."
	}
	if len(blockers) > 0 || len(adjudication.Rejected) > 0 {
		return AssessmentBlocked, "Resolve blocked or rejected QA evidence and start a current attempt."
	}
	if len(evidence) == 0 || len(adjudication.AcceptedIDs) != len(evidence) {
		return AssessmentIncomplete, "Complete every required evidence plan."
	}
	for _, record := range evidence {
		if record.Outcome == QAEvidenceBlocked {
			return AssessmentBlocked, "Resolve blocked evidence and rerun QA."
		}
	}
	if smoke != nil {
		if !smoke.Fresh || smoke.ExecutionStatus != string(SmokeCompleted) {
			return AssessmentIncomplete, "Run the required containing smoke suite."
		}
		if smoke.Verdict == string(SmokeFailVerdict) {
			return AssessmentFail, "Resolve the containing smoke failure."
		}
		if smoke.Verdict != string(SmokePass) && smoke.Verdict != string(SmokePassWithOpenIssues) {
			return AssessmentBlocked, "Restore valid containing smoke evidence."
		}
	}
	if len(adjudication.Issues) > 0 {
		return AssessmentPassWithFindings, "Review the promoted issues before governed repair."
	}
	if review.Verdict == string(ReviewPassWithFindings) {
		return AssessmentPassWithFindings, "Review the current Conformance Review findings."
	}
	return AssessmentPass, "QA evidence is current and complete."
}

func rejectEvidence(id, code, detail string) QARejectedEvidence {
	return QARejectedEvidence{EvidenceID: id, Code: code, Detail: detail}
}

func normalizeQASeverity(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "critical", "high", "medium", "low":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "medium"
	}
}
