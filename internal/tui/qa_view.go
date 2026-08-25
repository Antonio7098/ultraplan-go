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
		fmt.Fprintln(b, "Read-only QA completed")
	} else {
		fmt.Fprintln(b, "Read-only QA")
	}
	fmt.Fprintf(b, "Phase: %s\nFresh: %t\nConformance Review: status=%s verdict=%s fresh=%t\nCoverage: %d/%d changed paths\nShards: %d/%d\n", qa.Phase, qa.Fresh, qa.ConformanceReviewStatus, qa.ConformanceReviewVerdict, qa.ConformanceReviewFresh, qa.CoveredPaths, qa.ChangedPaths, qa.CompletedShards, qa.TotalShards)
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
