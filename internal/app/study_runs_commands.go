package app

import (
	"fmt"

	"github.com/Antonio7098/ultraplan-go/internal/study"
	"github.com/Antonio7098/ultraplan-go/internal/workspace"
)

func runStudyRuns(deps dependencies, root string, service study.Service, studyRef string, args []string) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		_, err := deps.stdout.Write([]byte(studyRunsHelp()))
		return err
	}
	switch args[0] {
	case "summary":
		if len(args) != 1 {
			return classified(ExitUsage, "study runs summary: unknown argument %q", args[1])
		}
		listing, err := service.ListStudy(studyRef)
		if err != nil {
			return mapStudyError(err)
		}
		state, err := study.LoadRunState(listing.Study)
		if err != nil {
			return mapStudyRunLoopError(err)
		}
		if err := study.SyncRunHistory(listing.Study, state); err != nil {
			return mapStudyExecutionError("study.runs.summary", err)
		}
		fmt.Fprintf(deps.stdout, "Run summary: %s\n", workspace.Rel(root, study.RunHistorySummaryPath(listing.Study)))
		fmt.Fprintf(deps.stdout, "Run ledger: %s\n", workspace.Rel(root, study.RunHistoryPath(listing.Study)))
		return nil
	default:
		return classified(ExitUsage, "study runs: expected 'summary'")
	}
}

func studyRunsHelp() string {
	return `ultraplan study <study> runs

Usage:
  ultraplan study <study> runs summary

Commands:
  summary  Refresh studies/<study>/.ultraplan/runs/summary.md from run-state and tasks.jsonl without runtime execution.
`
}
