package sprint

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
)

func RenderQAReport(project, sprintSlug, inputFingerprint string, evidence []QAEvidenceRecord, adjudication QAAdjudication, assessment QAAssessmentRecord) ([]byte, error) {
	if !safeQAName(project) || !safeQAName(sprintSlug) || !validFingerprint(inputFingerprint) {
		return nil, fmt.Errorf("invalid QA report scope")
	}
	if err := validateQAAssessment(assessment, adjudication.AttemptID); err != nil {
		return nil, err
	}
	evidence = append([]QAEvidenceRecord(nil), evidence...)
	sort.Slice(evidence, func(i, j int) bool { return evidence[i].ID < evidence[j].ID })
	issues := append([]QAIssue(nil), adjudication.Issues...)
	sort.Slice(issues, func(i, j int) bool { return issues[i].ID < issues[j].ID })
	rejected := append([]QARejectedEvidence(nil), adjudication.Rejected...)
	sort.Slice(rejected, func(i, j int) bool { return rejected[i].EvidenceID < rejected[j].EvidenceID })
	var b bytes.Buffer
	fmt.Fprintln(&b, "# QA")
	fmt.Fprintln(&b)
	fmt.Fprintf(&b, "Project: `%s`\nSprint: `%s`\nInput fingerprint: `%s`\nAttempt: `%s`\nAssessment: `%s`\n", project, sprintSlug, inputFingerprint, adjudication.AttemptID, assessment.Assessment)
	fmt.Fprintln(&b, "\n## Evidence")
	fmt.Fprintf(&b, "\nAccepted: `%d`\nRejected: `%d`\n", len(adjudication.AcceptedIDs), len(rejected))
	for _, record := range evidence {
		fmt.Fprintf(&b, "- `%s` %s, reason `%s`, contained `%t`, cleanup `%t`\n", record.ID, record.Outcome, safeReportText(record.ReasonCode), record.Contained, record.Cleanup.Complete)
	}
	if len(rejected) > 0 {
		fmt.Fprintln(&b, "\n## Rejected evidence")
		for _, record := range rejected {
			fmt.Fprintf(&b, "- `%s` `%s`: %s\n", record.EvidenceID, safeReportText(record.Code), safeReportText(record.Detail))
		}
	}
	fmt.Fprintln(&b, "\n## Promoted issues")
	if len(issues) == 0 {
		fmt.Fprintln(&b, "\nNone.")
	} else {
		for _, issue := range issues {
			fmt.Fprintf(&b, "- `%s` [%s] %s at `%s`, evidence `%s`, regression candidate `%t`\n", issue.ID, issue.Severity, safeReportText(issue.Title), safeReportText(issue.Location), strings.Join(issue.EvidenceIDs, "`, `"), issue.RegressionCandidate)
		}
	}
	fmt.Fprintln(&b, "\n## Smoke evidence")
	if assessment.SmokeRunID == "" {
		fmt.Fprintln(&b, "\nNo containing smoke evidence is attached to this assessment.")
	} else {
		fmt.Fprintf(&b, "\nVerdict: `%s`\nRun: `%s`\n", assessment.SmokeVerdict, safeReportText(assessment.SmokeRunID))
	}
	if len(assessment.Blockers) > 0 {
		fmt.Fprintln(&b, "\n## Blockers")
		for _, blocker := range assessment.Blockers {
			fmt.Fprintf(&b, "- `%s`: %s. %s\n", blocker.Category, safeReportText(blocker.Summary), safeReportText(blocker.NextAction))
		}
	}
	fmt.Fprintln(&b, "\n## Next action")
	fmt.Fprintf(&b, "\n%s\n", safeReportText(assessment.NextAction))
	return b.Bytes(), nil
}

func safeReportText(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(value, "\r", " "), "\n", " "))
	value = strings.ReplaceAll(value, "`", "'")
	if len(value) > 2048 {
		value = value[:2048]
	}
	return value
}
