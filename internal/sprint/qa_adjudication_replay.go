package sprint

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"
)

type QAAdjudicationReplayResult struct {
	AttemptID          string `json:"attempt_id"`
	RewindID           string `json:"rewind_id"`
	ArchivePath        string `json:"archive_path"`
	RetainedShardCount int    `json:"retained_shard_count"`
}

// ReplayQAAdjudication rewinds the current attempt to immediately before
// arbitration. The next qa resume reuses retained terminal shards and starts
// fresh arbiters; this operation does not itself run a model.
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
	if state.CurrentAttemptID == "" || state.Map == nil || state.Synthesis == nil {
		return QAAdjudicationReplayResult{}, NewQAError(QAErrorInvalidState, "rewind arbitration", "the current attempt has no completed investigation output", nil)
	}
	retained, err := store.LoadMap(state.CurrentAttemptID)
	if err != nil {
		return QAAdjudicationReplayResult{}, err
	}
	current, err := s.QAMap(projectRef, sprintRef)
	if err != nil {
		return QAAdjudicationReplayResult{}, err
	}
	if err := validateQAReplayIdentity(retained, current.Map); err != nil {
		return QAAdjudicationReplayResult{}, err
	}
	synthesis, err := store.LoadSynthesis(state.CurrentAttemptID, retained.Budgets)
	if err != nil {
		return QAAdjudicationReplayResult{}, err
	}
	shardIDs := make([]string, 0, len(retained.Shards)+len(synthesis.FollowUpShards))
	for _, shard := range append(append([]QAShard(nil), retained.Shards...), synthesis.FollowUpShards...) {
		if _, err := store.LoadShard(state.CurrentAttemptID, shard.ID); err != nil {
			return QAAdjudicationReplayResult{}, err
		}
		shardIDs = append(shardIDs, shard.ID)
	}
	shardIDs = normalizeQAStrings(shardIDs)
	now := s.now().UTC()
	rewindID := fmt.Sprintf("qa-v1-rewind-%s", hashBytes([]byte(state.CurrentAttemptID + now.Format(time.RFC3339Nano)))[:24])
	archiveRel := filepath.ToSlash(filepath.Join("projects", sp.Project, "sprints", sp.Slug, "verification", "attempts", state.CurrentAttemptID, "rewinds", rewindID))
	archiveRoot, err := store.resolve(archiveRel)
	if err != nil {
		return QAAdjudicationReplayResult{}, err
	}
	if err := os.MkdirAll(archiveRoot, 0o755); err != nil {
		return QAAdjudicationReplayResult{}, NewQAError(QAErrorPersistenceFailure, "rewind arbitration", "cannot create rewind archive", err)
	}

	attemptRel := filepath.ToSlash(filepath.Join("projects", sp.Project, "sprints", sp.Slug, "verification", "attempts", state.CurrentAttemptID))
	targets := []string{"synthesis.json", "arbiter-sessions", "arbiter-evidence-requests", "investigator-tests", "plans", "evidence", "patches", "adjudication.json", "issues.json", "assessment.json", "issue-evidence-coverage.json", "investigator-workspace-cleanup.json"}
	type movedPath struct{ from, to string }
	var moved []movedPath
	rollback := func() {
		for i := len(moved) - 1; i >= 0; i-- {
			_ = os.Rename(moved[i].to, moved[i].from)
		}
		_ = os.Remove(archiveRoot)
	}
	for _, name := range targets {
		from, resolveErr := store.resolve(filepath.ToSlash(filepath.Join(attemptRel, name)))
		if resolveErr != nil {
			rollback()
			return QAAdjudicationReplayResult{}, resolveErr
		}
		if _, statErr := os.Stat(from); errors.Is(statErr, fs.ErrNotExist) {
			continue
		} else if statErr != nil {
			rollback()
			return QAAdjudicationReplayResult{}, statErr
		}
		to := filepath.Join(archiveRoot, name)
		if renameErr := os.Rename(from, to); renameErr != nil {
			rollback()
			return QAAdjudicationReplayResult{}, NewQAError(QAErrorPersistenceFailure, "rewind arbitration", "cannot archive post-investigation artifacts", renameErr)
		}
		moved = append(moved, movedPath{from, to})
	}
	reportPath, err := store.resolve(QAReportRelPath(sp))
	if err != nil {
		rollback()
		return QAAdjudicationReplayResult{}, err
	}
	if _, statErr := os.Stat(reportPath); statErr == nil {
		to := filepath.Join(archiveRoot, "qa.md")
		if renameErr := os.Rename(reportPath, to); renameErr != nil {
			rollback()
			return QAAdjudicationReplayResult{}, renameErr
		}
		moved = append(moved, movedPath{reportPath, to})
	}
	flow, err := LoadFlowState(s.root, sp)
	if err != nil {
		rollback()
		return QAAdjudicationReplayResult{}, err
	}
	state.Synthesis, state.Adjudication, state.Issues, state.Assessment, state.CanonicalReport = nil, nil, nil, nil, nil
	state.EvidenceCount, state.RejectedCount, state.CandidateCount, state.UnpromotedCount = 0, 0, 0, 0
	state.IssueCount, state.RegressionCandidates = 0, 0
	state.CanonicalAssessment, state.CurrentFailure, state.Blocker = "", nil, nil
	state.Phase = QAPhaseInterrupted
	state.Run.Lifecycle, state.Run.TerminalResult = QARunTerminal, QATerminalInterrupted
	state.Freshness.Current, state.Freshness.Reasons = true, nil
	state.Freshness.PolicyFingerprint = current.Map.PolicyFingerprint
	state.ArbitrationRewind = &QAArbitrationRewind{ID: rewindID, ArchivePath: archiveRel, RetainedShardIDs: shardIDs, SourcePolicyFingerprint: retained.PolicyFingerprint, AppliedPolicyFingerprint: current.Map.PolicyFingerprint, RewoundAt: now}
	state.NextAction = "Run qa resume to start fresh arbitration from the retained investigations."
	state.UpdatedAt = now
	if err := store.SaveRecoveredState(state, flow); err != nil {
		rollback()
		return QAAdjudicationReplayResult{}, err
	}
	return QAAdjudicationReplayResult{AttemptID: state.CurrentAttemptID, RewindID: rewindID, ArchivePath: archiveRel, RetainedShardCount: len(shardIDs)}, nil
}

func validateQAReplayIdentity(retained, current QAMap) error {
	if retained.Project != current.Project || retained.Sprint != current.Sprint || retained.GovernedInputFingerprint != current.GovernedInputFingerprint || retained.ImplementationFingerprint != current.ImplementationFingerprint || retained.ReviewFingerprint != current.ReviewFingerprint || retained.CheckCatalogFingerprint != current.CheckCatalogFingerprint || retained.Target.Fingerprint != current.Target.Fingerprint {
		return NewQAError(QAErrorStaleInput, "rewind arbitration", "retained investigations no longer describe the current governed inputs, implementation, review, or check catalog", nil)
	}
	return nil
}

func retainedQARewindShards(store QAStore, qaMap QAMap, rewind *QAArbitrationRewind) ([]QAShard, error) {
	if rewind == nil || len(rewind.RetainedShardIDs) == 0 {
		return append([]QAShard(nil), qaMap.Shards...), nil
	}
	shards := make([]QAShard, 0, len(rewind.RetainedShardIDs))
	for _, id := range rewind.RetainedShardIDs {
		shard, err := store.LoadShard(qaMap.SemanticAttemptID, id)
		if err != nil {
			return nil, err
		}
		shards = append(shards, shard)
	}
	sort.Slice(shards, func(i, j int) bool { return shards[i].ID < shards[j].ID })
	return shards, nil
}
