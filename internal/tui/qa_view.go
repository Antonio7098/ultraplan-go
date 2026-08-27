package tui

import (
	"fmt"
	"strings"
)

// renderSprintQAView renders only the bounded app projection. It never reads
// verification persistence or derives product outcomes from observer events.
func renderSprintQAView(b *strings.Builder, m Model, route Route) {
	summary, ok := findSprint(m.Data.Sprints, route.Project, route.Sprint)
	if !ok {
		return
	}
	qa := summary.QA
	if qa.Phase == "completed" {
		fmt.Fprintln(b, "Read-only QA completed (compatibility status); evidence-producing QA uses isolated writable copies")
	} else {
		fmt.Fprintln(b, "QA")
	}
	fmt.Fprintf(b, "Phase: %s\nFresh: %t\nConformance Review: status=%s verdict=%s fresh=%t\nCoverage: %d/%d changed paths\nShards: %d/%d\n", qa.Phase, qa.Fresh, qa.ConformanceReviewStatus, qa.ConformanceReviewVerdict, qa.ConformanceReviewFresh, qa.CoveredPaths, qa.ChangedPaths, qa.CompletedShards, qa.TotalShards)
	if qa.Assessment != "" {
		fmt.Fprintf(b, "Assessment: %s\nEvidence: %d total, %d rejected\nIssues: %d\nRegression candidates: %d\n", qa.Assessment, qa.EvidenceCount, qa.RejectedEvidenceCount, qa.IssueCount, qa.RegressionCandidateCount)
	}
	if qa.CanonicalReport != nil {
		fmt.Fprintf(b, "Canonical report: %s\n", qa.CanonicalReport.Path)
	}
	if qa.CurrentFailure != nil {
		fmt.Fprintf(b, "Current failure: %s [%s]\n", qa.CurrentFailure.Summary, qa.CurrentFailure.Category)
	}
	if qa.Cancellation.Requested {
		fmt.Fprintf(b, "Cancellation requested: %s\n", qa.Cancellation.Reason)
	}
	if qa.Blocker != nil {
		fmt.Fprintf(b, "Blocker: %s [%s]\n", qa.Blocker.Summary, qa.Blocker.Category)
	}
	if route.Shard != "" {
		for _, shard := range qa.Shards {
			if shard.ID != route.Shard {
				continue
			}
			fmt.Fprintf(b, "Focused shard: %s\nKind: %s\nStatus: %s\nTheories: %d\n", shard.Title, shard.Kind, shard.Phase, shard.TheoryCount)
			if route.Theory == "" {
				continue
			}
			for _, theory := range shard.Theories {
				if theory.ID == route.Theory {
					fmt.Fprintf(b, "Focused theory: %s\nClaim: %s\nBasis: %s\nOutcome: %s\nReason: %s\n", theory.ID, theory.Claim, theory.Basis, theory.Outcome, theory.OutcomeReason)
				}
			}
		}
	}
	fmt.Fprintf(b, "Next: %s\n\n", qa.NextAction)
}

func renderSprintRepairView(b *strings.Builder, m Model, route Route) {
	summary, ok := findSprint(m.Data.Sprints, route.Project, route.Sprint)
	if !ok {
		return
	}
	repair := summary.Repair
	fmt.Fprintf(b, "Bounded repair\nPhase: %s\nFresh authority: %t\nMode: %s\nCycle: %d\nDurable lifecycle: %s\n", repair.Phase, repair.Fresh, repair.Mode, repair.CurrentCycle, repair.RunLifecycle)
	if repair.Packet != nil {
		fmt.Fprintf(b, "Packet: %s\nIssue: %s — %s\nTarget: %s\nChecks: %d\nLimits: %d apply, %d files, %d bytes, %s\n", repair.Packet.Digest, repair.Packet.IssueID, repair.Packet.IssueTitle, repair.Packet.Target.Fingerprint, repair.Packet.CheckCount, repair.Packet.Budgets.MaxMutationCycles, repair.Packet.Budgets.MaxFiles, repair.Packet.Budgets.MaxBytes, repair.Packet.Budgets.WallTime)
	}
	if repair.Confirmation != nil {
		fmt.Fprintf(b, "Confirmation: %s by %s\n", repair.Confirmation.Digest, repair.Confirmation.Confirmer)
	}
	if repair.Outcome != "" {
		fmt.Fprintf(b, "Semantic outcome: %s\nStop reason: %s\nProduction applied: %t\nCleanup proven: %t\n", repair.Outcome, repair.StopReason, repair.ProductionApplied, repair.CleanupComplete)
	}
	for _, reason := range repair.FreshnessReasons {
		fmt.Fprintf(b, "Freshness: %s\n", reason)
	}
	fmt.Fprintf(b, "Automatic mode: unavailable pending qualifying real manual proof\nNext: %s\n\n", repair.NextAction)
}
