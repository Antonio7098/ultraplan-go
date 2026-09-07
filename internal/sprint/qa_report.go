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
	unpromoted := append([]QAUnpromotedIssue(nil), adjudication.Unpromoted...)
	sort.Slice(unpromoted, func(i, j int) bool { return unpromoted[i].CandidateID < unpromoted[j].CandidateID })
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
	fmt.Fprintln(&b, "\n## Issue candidates")
	fmt.Fprintf(&b, "\nTotal: `%d`\nPromoted: `%d`\nUnpromoted: `%d`\n", len(issues)+len(unpromoted), len(issues), len(unpromoted))
	for _, candidate := range unpromoted {
		fmt.Fprintf(&b, "- `%s` [%s] %s at `%s`, outcome `unpromoted`, reason `%s`: %s\n", candidate.CandidateID, candidate.Severity, safeReportText(candidate.Title), safeReportText(candidate.Location), safeReportText(candidate.ReasonCode), safeReportText(candidate.Detail))
	}
	fmt.Fprintln(&b, "\n## Promoted issues")
	if len(issues) == 0 {
		fmt.Fprintln(&b, "\nNone.")
	} else {
		for _, issue := range issues {
			fmt.Fprintf(&b, "- `%s` [%s] %s at `%s`, evidence `%s`, regression candidate `%t`\n", issue.ID, issue.Severity, safeReportText(issue.Title), safeReportText(issue.Location), strings.Join(issue.EvidenceIDs, "`, `"), issue.RegressionCandidate)
		}
	}
	if assessment.SmokeRunID != "" {
		fmt.Fprintln(&b, "\n## Legacy smoke evidence")
		fmt.Fprintf(&b, "\nVerdict: `%s`\nRun: `%s`\n", assessment.SmokeVerdict, safeReportText(assessment.SmokeRunID))
	}
	if len(assessment.Blockers) > 0 {
		fmt.Fprintln(&b, "\n## Active blockers")
		fmt.Fprintf(&b, "\nActive requests: `%d`\nHistorical predecessors: `%d`\n", len(assessment.Blockers), len(assessment.HistoricalBlockers))
		for _, blocker := range assessment.Blockers {
			fmt.Fprintf(&b, "- `%s`: %s. %s\n", blocker.Category, safeReportText(blocker.Summary), safeReportText(blocker.NextAction))
		}
	}
	if len(assessment.HistoricalBlockers) > 0 {
		fmt.Fprint(&b, "\n<details><summary>Historical blocker records</summary>\n\n")
		for _, blocker := range assessment.HistoricalBlockers {
			fmt.Fprintf(&b, "- `%s`: %s\n", blocker.Scope, safeReportText(blocker.Summary))
		}
		fmt.Fprintln(&b, "\n</details>")
	}
	if len(assessment.RequestTheoryCoverage) > 0 {
		fmt.Fprint(&b, "\n## Theory assertion coverage\n\n")
		fmt.Fprintln(&b, "| Theory | Accepted failing bundle / assertion |\n| --- | --- |")
		ids := make([]string, 0, len(assessment.RequestTheoryCoverage))
		for id := range assessment.RequestTheoryCoverage {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			assertions := strings.Join(assessment.RequestTheoryCoverage[id], ", ")
			if assertions == "" {
				assertions = "Missing evidence"
			}
			fmt.Fprintf(&b, "| `%s` | %s |\n", id, safeReportText(assertions))
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
