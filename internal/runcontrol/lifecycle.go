package runcontrol

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

func (r *SQLiteRepository) Heartbeat(ctx context.Context, fence Fence, lease time.Duration) (Snapshot, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := fence.Validate(); err != nil {
		return Snapshot{}, err
	}
	if lease <= 0 {
		return Snapshot{}, invalidField("lease", "must be positive")
	}
	if usage, err := r.storageBytes(); err != nil {
		return Snapshot{}, err
	} else if usage >= r.retention.HardQuotaBytes {
		return Snapshot{}, runError(CodeQuota, "heartbeat_quota", fence.RunID, "hard quota requires the owner to stop active work", true, nil)
	}
	now := r.now()
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return Snapshot{}, classifyStoreError("heartbeat_begin", "begin owner heartbeat failed", err)
	}
	defer tx.Rollback()
	if err := verifyFence(ctx, tx, fence); err != nil {
		return Snapshot{}, err
	}
	result, err := tx.ExecContext(ctx, `
UPDATE attempts SET heartbeat_at = ?, lease_expires_at = ?
WHERE run_id = ? AND attempt_id = ? AND owner_id = ? AND fencing_generation = ? AND outcome IS NULL`,
		formatTime(now), formatTime(now.Add(lease)), string(fence.RunID), string(fence.AttemptID), fence.OwnerID, fence.FencingGeneration)
	if err != nil {
		return Snapshot{}, classifyStoreError("heartbeat", "persist owner heartbeat failed", err)
	}
	if changed, err := result.RowsAffected(); err != nil || changed != 1 {
		return Snapshot{}, runError(CodeStaleFence, "heartbeat", fence.RunID, "owner heartbeat lost its authoritative fence", false, err)
	}
	result, err = tx.ExecContext(ctx, `UPDATE runs SET liveness = ?, updated_at = ? WHERE run_id = ? AND current_attempt_id = ? AND terminal_outcome IS NULL`,
		string(LivenessLive), formatTime(now), string(fence.RunID), string(fence.AttemptID))
	if err != nil {
		return Snapshot{}, classifyStoreError("heartbeat_snapshot", "update heartbeat snapshot failed", err)
	}
	if changed, err := result.RowsAffected(); err != nil || changed != 1 {
		return Snapshot{}, runError(CodeStaleFence, "heartbeat", fence.RunID, "run is no longer owned by heartbeat attempt", false, err)
	}
	snapshot, err := loadSnapshot(ctx, tx, fence.RunID)
	if err != nil {
		return Snapshot{}, err
	}
	if err := tx.Commit(); err != nil {
		return Snapshot{}, classifyStoreError("heartbeat_commit", "commit owner heartbeat failed", err)
	}
	r.log(ctx, LogDebug, "run owner heartbeat committed",
		LogField{Key: "run_id", Value: string(fence.RunID)}, LogField{Key: "attempt_id", Value: string(fence.AttemptID)},
		LogField{Key: "owner_id", Value: fence.OwnerID}, LogField{Key: "fencing_generation", Value: fmt.Sprint(fence.FencingGeneration)})
	r.metrics.leaseRenewals.Add(1)
	return snapshot, nil
}

func (r *SQLiteRepository) RequestCancellation(ctx context.Context, runID RunID, reason string) (Snapshot, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := runID.Validate(); err != nil {
		return Snapshot{}, false, err
	}
	reason = strings.TrimSpace(reason)
	if reason == "" || len(reason) > MaxSafeValueBytes || strings.ContainsAny(reason, "\x00\r\n") {
		return Snapshot{}, false, invalidField("cancellation.reason", "must be a bounded canonical reason")
	}
	now := r.now()
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return Snapshot{}, false, classifyStoreError("cancel_begin", "begin cancellation request failed", err)
	}
	defer tx.Rollback()
	current, err := loadSnapshot(ctx, tx, runID)
	if err != nil {
		return Snapshot{}, false, err
	}
	if current.Terminal != nil || current.Cancellation.State != CancellationNone {
		if err := tx.Commit(); err != nil {
			return Snapshot{}, false, classifyStoreError("cancel_read_commit", "finish cancellation state read failed", err)
		}
		return current, false, nil
	}
	sequence := current.LastSequence + 1
	payload, err := marshalBounded(map[string]string{"state": string(CancellationRequested), "reason": reason}, MaxSafeValueBytes*2)
	if err != nil {
		return Snapshot{}, false, err
	}
	var attemptValue any
	if current.CurrentAttemptID != "" {
		attemptValue = string(current.CurrentAttemptID)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO events (run_id, sequence, committed_at, event_type, attempt_id, payload_json)
VALUES (?, ?, ?, 'cancellation', ?, ?)`, string(runID), sequence, formatTime(now), attemptValue, payload); err != nil {
		return Snapshot{}, false, classifyStoreError("cancel_event", "persist cancellation event failed", err)
	}
	result, err := tx.ExecContext(ctx, `
UPDATE runs SET lifecycle = ?, cancellation_state = ?, cancellation_reason = ?, cancellation_requested_at = ?, updated_at = ?, last_sequence = ?
WHERE run_id = ? AND terminal_outcome IS NULL AND cancellation_state = ? AND last_sequence = ?`,
		string(LifecycleCancelling), string(CancellationRequested), reason, formatTime(now), formatTime(now), sequence,
		string(runID), string(CancellationNone), current.LastSequence)
	if err != nil {
		return Snapshot{}, false, classifyStoreError("cancel", "persist cancellation request failed", err)
	}
	if changed, err := result.RowsAffected(); err != nil || changed != 1 {
		return Snapshot{}, false, runError(CodeConflict, "cancel", runID, "cancellation state changed concurrently", true, err)
	}
	snapshot, err := loadSnapshot(ctx, tx, runID)
	if err != nil {
		return Snapshot{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return Snapshot{}, false, classifyStoreError("cancel_commit", "commit cancellation request failed", err)
	}
	r.notify(runID, sequence)
	r.log(ctx, LogInfo, "run cancellation requested",
		LogField{Key: "run_id", Value: string(runID)}, LogField{Key: "sequence", Value: fmt.Sprint(sequence)},
		LogField{Key: "cancellation_state", Value: string(snapshot.Cancellation.State)}, LogField{Key: "cancellation_reason", Value: reason})
	r.metrics.cancellationRequests.Add(1)
	return snapshot, true, nil
}

func (r *SQLiteRepository) AcknowledgeCancellation(ctx context.Context, fence Fence) (Snapshot, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := fence.Validate(); err != nil {
		return Snapshot{}, false, err
	}
	now := r.now()
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return Snapshot{}, false, classifyStoreError("cancel_ack_begin", "begin cancellation acknowledgement failed", err)
	}
	defer tx.Rollback()
	if err := verifyFence(ctx, tx, fence); err != nil {
		return Snapshot{}, false, err
	}
	current, err := loadSnapshot(ctx, tx, fence.RunID)
	if err != nil {
		return Snapshot{}, false, err
	}
	if current.Terminal != nil || current.Cancellation.State == CancellationAcknowledged {
		if err := tx.Commit(); err != nil {
			return Snapshot{}, false, classifyStoreError("cancel_ack_read_commit", "finish cancellation acknowledgement read failed", err)
		}
		return current, false, nil
	}
	if current.Cancellation.State != CancellationRequested {
		return Snapshot{}, false, runError(CodeConflict, "cancel_ack", fence.RunID, "cancellation has not been requested", false, nil)
	}
	sequence := current.LastSequence + 1
	payload, _ := marshalBounded(map[string]string{"state": string(CancellationAcknowledged)}, MaxSafeValueBytes)
	if _, err := tx.ExecContext(ctx, `INSERT INTO events (run_id, sequence, committed_at, event_type, attempt_id, payload_json)
VALUES (?, ?, ?, 'cancellation', ?, ?)`, string(fence.RunID), sequence, formatTime(now), string(fence.AttemptID), payload); err != nil {
		return Snapshot{}, false, classifyStoreError("cancel_ack_event", "persist cancellation acknowledgement event failed", err)
	}
	result, err := tx.ExecContext(ctx, `
UPDATE runs SET cancellation_state = ?, cancellation_acknowledged_at = ?, updated_at = ?, last_sequence = ?
WHERE run_id = ? AND current_attempt_id = ? AND cancellation_state = ? AND terminal_outcome IS NULL AND last_sequence = ?`,
		string(CancellationAcknowledged), formatTime(now), formatTime(now), sequence, string(fence.RunID), string(fence.AttemptID),
		string(CancellationRequested), current.LastSequence)
	if err != nil {
		return Snapshot{}, false, classifyStoreError("cancel_ack", "persist cancellation acknowledgement failed", err)
	}
	if changed, err := result.RowsAffected(); err != nil || changed != 1 {
		return Snapshot{}, false, runError(CodeStaleFence, "cancel_ack", fence.RunID, "cancellation acknowledgement lost its fence", false, err)
	}
	snapshot, err := loadSnapshot(ctx, tx, fence.RunID)
	if err != nil {
		return Snapshot{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return Snapshot{}, false, classifyStoreError("cancel_ack_commit", "commit cancellation acknowledgement failed", err)
	}
	r.notify(fence.RunID, sequence)
	r.log(ctx, LogInfo, "run cancellation acknowledged",
		LogField{Key: "run_id", Value: string(fence.RunID)}, LogField{Key: "attempt_id", Value: string(fence.AttemptID)},
		LogField{Key: "sequence", Value: fmt.Sprint(sequence)}, LogField{Key: "cancellation_state", Value: string(snapshot.Cancellation.State)})
	return snapshot, true, nil
}

func (r *SQLiteRepository) List(ctx context.Context, query Query) (Page, error) {
	limit := query.Limit
	if limit == 0 {
		limit = 50
	}
	if limit < 1 || limit > 200 {
		return Page{}, invalidField("query.limit", "must be between 1 and 200")
	}
	clauses := []string{"1=1"}
	args := make([]any, 0, 8)
	if len(query.Lifecycle) > 0 {
		parts := make([]string, 0, len(query.Lifecycle))
		for _, lifecycle := range query.Lifecycle {
			if !lifecycle.IsValid() {
				return Page{}, invalidField("query.lifecycle", "contains an unknown state")
			}
			parts = append(parts, "?")
			args = append(args, string(lifecycle))
		}
		clauses = append(clauses, "lifecycle IN ("+strings.Join(parts, ",")+")")
	}
	for column, value := range map[string]string{
		"target_kind": query.TargetKind, "project_id": query.Project, "sprint_id": query.Sprint, "study_id": query.Study,
	} {
		if value != "" {
			clauses = append(clauses, column+" = ?")
			args = append(args, value)
		}
	}
	if query.After != "" {
		cursor, err := decodeRunCursor(query.After)
		if err != nil {
			return Page{}, err
		}
		clauses = append(clauses, "(updated_at < ? OR (updated_at = ? AND run_id < ?))")
		args = append(args, cursor.UpdatedAt, cursor.UpdatedAt, string(cursor.RunID))
	}
	rows, err := r.db.QueryContext(ctx, `SELECT run_id, updated_at FROM runs WHERE `+strings.Join(clauses, " AND ")+` ORDER BY updated_at DESC, run_id DESC LIMIT ?`, append(args, limit+1)...)
	if err != nil {
		return Page{}, classifyStoreError("list", "list runs failed", err)
	}
	defer rows.Close()
	type listedRun struct {
		id        RunID
		updatedAt string
	}
	listed := make([]listedRun, 0, limit+1)
	for rows.Next() {
		var item listedRun
		if err := rows.Scan(&item.id, &item.updatedAt); err != nil {
			return Page{}, classifyStoreError("list_scan", "decode run list failed", err)
		}
		listed = append(listed, item)
	}
	if err := rows.Err(); err != nil {
		return Page{}, classifyStoreError("list_rows", "read run list failed", err)
	}
	page := Page{Runs: make([]Snapshot, 0, min(limit, len(listed)))}
	if len(listed) > limit {
		page.NextCursor = encodeRunCursor(runCursor{UpdatedAt: listed[limit-1].updatedAt, RunID: listed[limit-1].id})
		listed = listed[:limit]
	}
	for _, item := range listed {
		snapshot, err := r.Snapshot(ctx, item.id)
		if err != nil {
			return Page{}, err
		}
		page.Runs = append(page.Runs, snapshot)
	}
	return page, nil
}

type runCursor struct {
	UpdatedAt string `json:"updated_at"`
	RunID     RunID  `json:"run_id"`
}

func encodeRunCursor(cursor runCursor) string {
	encoded, _ := json.Marshal(cursor)
	return base64.RawURLEncoding.EncodeToString(encoded)
}

func decodeRunCursor(value string) (runCursor, error) {
	encoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(encoded) > 1024 {
		return runCursor{}, invalidField("query.after", "is not a valid opaque cursor")
	}
	var cursor runCursor
	if err := json.Unmarshal(encoded, &cursor); err != nil || cursor.UpdatedAt == "" || cursor.RunID.Validate() != nil {
		return runCursor{}, invalidField("query.after", "is not a valid opaque cursor")
	}
	if _, err := parseTime(cursor.UpdatedAt); err != nil {
		return runCursor{}, invalidField("query.after", "is not a valid opaque cursor")
	}
	return cursor, nil
}

type reconciliationCandidate struct {
	runID      RunID
	attemptID  AttemptID
	ownerID    string
	generation uint64
	process    ProcessIdentity
}

func (r *SQLiteRepository) Reconcile(ctx context.Context, probe ProcessProbe, opts ReconcileOptions) (ReconcileReport, error) {
	if probe == nil {
		return ReconcileReport{}, invalidField("process_probe", "is required")
	}
	grace := opts.Grace
	if grace == 0 {
		grace = ReconciliationGrace
	}
	if grace < 0 {
		return ReconcileReport{}, invalidField("reconciliation.grace", "must not be negative")
	}
	limit := opts.Limit
	if limit == 0 {
		limit = DefaultReconcileBatch
	}
	if limit < 1 || limit > MaximumReconcileBatch {
		return ReconcileReport{}, invalidField("reconciliation.limit", fmt.Sprintf("must be between 1 and %d", MaximumReconcileBatch))
	}
	report := ReconcileReport{Decisions: make([]ReconcileDecision, 0, limit)}
	acceptedPredicate, acceptedArgs := r.expiredTimestampPredicate("accepted_at", grace)
	unclaimedRows, err := r.db.QueryContext(ctx, `
SELECT run_id FROM runs
WHERE terminal_outcome IS NULL AND current_attempt_id IS NULL AND `+acceptedPredicate+`
ORDER BY accepted_at ASC, run_id ASC LIMIT ?`, append(acceptedArgs, limit)...)
	if err != nil {
		return ReconcileReport{}, classifyStoreError("reconcile_scan", "scan unclaimed runs failed", err)
	}
	var unclaimed []RunID
	for unclaimedRows.Next() {
		var runID RunID
		if err := unclaimedRows.Scan(&runID); err != nil {
			unclaimedRows.Close()
			return ReconcileReport{}, classifyStoreError("reconcile_scan", "decode unclaimed run failed", err)
		}
		unclaimed = append(unclaimed, runID)
	}
	if err := unclaimedRows.Close(); err != nil {
		return ReconcileReport{}, classifyStoreError("reconcile_scan", "close unclaimed run scan failed", err)
	}
	for _, runID := range unclaimed {
		won, err := r.reconcileUnclaimed(ctx, runID)
		if err != nil {
			return report, err
		}
		if !won {
			continue
		}
		report.Scanned++
		report.Terminal++
		decision := "owner_never_claimed_after_grace"
		candidate := reconciliationCandidate{runID: runID}
		_ = r.recordReconciliation(ctx, candidate, "terminal_proposal", decision)
		report.Decisions = append(report.Decisions, ReconcileDecision{RunID: runID, Action: "terminal_proposal", Decision: decision})
	}
	remaining := limit - report.Scanned
	if remaining == 0 {
		r.finishReconciliationLog(ctx, &report)
		return report, nil
	}
	predicate, predicateArgs := r.expiredLeasePredicate(grace)
	queryArgs := append(predicateArgs, remaining)
	rows, err := r.db.QueryContext(ctx, `
SELECT runs.run_id, attempts.attempt_id, attempts.owner_id, attempts.fencing_generation,
       attempts.host_digest, attempts.boot_id, attempts.pid, attempts.process_birth_token
FROM runs JOIN attempts ON attempts.attempt_id = runs.current_attempt_id
WHERE runs.terminal_outcome IS NULL AND `+predicate+`
ORDER BY attempts.lease_expires_at ASC, runs.run_id ASC LIMIT ?`, queryArgs...)
	if err != nil {
		return ReconcileReport{}, classifyStoreError("reconcile_scan", "scan expired run owners failed", err)
	}
	var candidates []reconciliationCandidate
	for rows.Next() {
		var candidate reconciliationCandidate
		if err := rows.Scan(&candidate.runID, &candidate.attemptID, &candidate.ownerID, &candidate.generation,
			&candidate.process.HostDigest, &candidate.process.BootID, &candidate.process.PID, &candidate.process.BirthToken); err != nil {
			rows.Close()
			return ReconcileReport{}, classifyStoreError("reconcile_scan", "decode expired run owner failed", err)
		}
		candidates = append(candidates, candidate)
	}
	if err := rows.Close(); err != nil {
		return ReconcileReport{}, classifyStoreError("reconcile_scan", "close expired owner scan failed", err)
	}
	report.Scanned += len(candidates)
	for _, candidate := range candidates {
		observed, probeErr := probe.Probe(ctx, candidate.process.PID)
		decision, outcome := reconcileProcessDecision(candidate.process, observed, probeErr)
		action := "terminal_proposal"
		if outcome == "" {
			action = "mark_stalled"
			if err := r.markStalled(ctx, candidate); err != nil {
				if errors.Is(err, ErrTerminal) || errors.Is(err, ErrStaleFence) {
					continue
				}
				return report, err
			}
			report.Stalled++
		} else {
			fence := Fence{RunID: candidate.runID, AttemptID: candidate.attemptID, OwnerID: candidate.ownerID, FencingGeneration: candidate.generation}
			_, won, err := r.ProposeTerminal(ctx, fence, TerminalProposal{Outcome: outcome, Reason: decision, ProposedBy: "reconciler"})
			if err != nil && !errors.Is(err, ErrTerminal) && !errors.Is(err, ErrStaleFence) {
				return report, err
			}
			if won {
				report.Terminal++
				if outcome == TerminalCleanupUncertain {
					report.Uncertain++
				}
			}
		}
		_ = r.recordReconciliation(ctx, candidate, action, decision)
		report.Decisions = append(report.Decisions, ReconcileDecision{RunID: candidate.runID, AttemptID: candidate.attemptID, Action: action, Decision: decision})
	}
	r.finishReconciliationLog(ctx, &report)
	return report, nil
}

func (r *SQLiteRepository) finishReconciliationLog(ctx context.Context, report *ReconcileReport) {
	r.log(ctx, LogInfo, "run reconciliation completed",
		LogField{Key: "reconciliation_scanned", Value: fmt.Sprint(report.Scanned)},
		LogField{Key: "reconciliation_stalled", Value: fmt.Sprint(report.Stalled)},
		LogField{Key: "reconciliation_terminal", Value: fmt.Sprint(report.Terminal)},
		LogField{Key: "reconciliation_uncertain", Value: fmt.Sprint(report.Uncertain)})
	r.metrics.reconciliations.Add(1)
}

func (r *SQLiteRepository) reconcileUnclaimed(ctx context.Context, runID RunID) (wonResult bool, resultErr error) {
	metricStarted := time.Now()
	defer func() { r.metrics.terminal.observe(metricStarted, resultErr) }()
	now := r.now()
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, classifyStoreError("reconcile_unclaimed_begin", "begin unclaimed reconciliation failed", err)
	}
	defer tx.Rollback()
	current, err := loadSnapshot(ctx, tx, runID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	if current.Terminal != nil || current.CurrentAttemptID != "" {
		return false, nil
	}
	sequence := current.LastSequence + 1
	reason := "owner_never_claimed_after_grace"
	payload, err := marshalBounded(map[string]string{"outcome": string(TerminalInterrupted), "reason": reason}, MaxSafeValueBytes*2)
	if err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO events (run_id, sequence, committed_at, event_type, payload_json)
VALUES (?, ?, ?, 'terminal', ?)`, string(runID), sequence, formatTime(now), payload); err != nil {
		return false, classifyStoreError("reconcile_unclaimed_event", "persist unclaimed terminal event failed", err)
	}
	result, err := tx.ExecContext(ctx, `UPDATE runs SET lifecycle = ?, liveness = ?, finished_at = ?, updated_at = ?, last_sequence = ?,
terminal_outcome = ?, terminal_reason = ?, terminal_at = ?, terminal_proposed_by = 'reconciler'
WHERE run_id = ? AND current_attempt_id IS NULL AND terminal_outcome IS NULL`,
		string(LifecycleInterrupted), string(LivenessTerminal), formatTime(now), formatTime(now), sequence,
		string(TerminalInterrupted), reason, formatTime(now), string(runID))
	if err != nil {
		return false, classifyStoreError("reconcile_unclaimed_cas", "persist unclaimed terminal state failed", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return false, classifyStoreError("reconcile_unclaimed_cas", "inspect unclaimed terminal state failed", err)
	}
	if changed != 1 {
		return false, nil
	}
	if err := tx.Commit(); err != nil {
		return false, classifyStoreError("reconcile_unclaimed_commit", "commit unclaimed reconciliation failed", err)
	}
	r.notify(runID, sequence)
	r.log(ctx, LogInfo, "unclaimed run reconciled",
		LogField{Key: "run_id", Value: string(runID)}, LogField{Key: "sequence", Value: fmt.Sprint(sequence)},
		LogField{Key: "terminal_outcome", Value: string(TerminalInterrupted)})
	return true, nil
}

func reconcileProcessDecision(expected, observed ProcessIdentity, probeErr error) (string, TerminalOutcome) {
	if errors.Is(probeErr, ErrProcessNotFound) {
		return "owner_process_missing_after_grace", TerminalInterrupted
	}
	if probeErr != nil {
		return "owner_process_identity_uncertain_after_grace", TerminalCleanupUncertain
	}
	if expected.HostDigest == "" || expected.BootID == "" || expected.BirthToken == "" || observed.HostDigest == "" || observed.BootID == "" || observed.BirthToken == "" {
		return "owner_process_identity_incomplete_after_grace", TerminalCleanupUncertain
	}
	if expected.HostDigest != observed.HostDigest || expected.BootID != observed.BootID || expected.PID != observed.PID || expected.BirthToken != observed.BirthToken {
		return "owner_process_birth_mismatch_after_grace", TerminalInterrupted
	}
	return "exact_owner_process_still_live_but_lease_expired", ""
}

func (r *SQLiteRepository) markStalled(ctx context.Context, candidate reconciliationCandidate) error {
	result, err := r.db.ExecContext(ctx, `
UPDATE runs SET liveness = ?, updated_at = ?
WHERE run_id = ? AND current_attempt_id = ? AND terminal_outcome IS NULL`,
		string(LivenessStalled), formatTime(r.now()), string(candidate.runID), string(candidate.attemptID))
	if err != nil {
		return classifyStoreError("reconcile_stalled", "persist stalled owner state failed", err)
	}
	if changed, err := result.RowsAffected(); err != nil || changed != 1 {
		return runError(CodeStaleFence, "reconcile_stalled", candidate.runID, "owner changed during reconciliation", false, err)
	}
	return nil
}

func (r *SQLiteRepository) recordReconciliation(ctx context.Context, candidate reconciliationCandidate, action, decision string) error {
	evidenceClass := "safe_identity_only"
	if candidate.attemptID == "" {
		evidenceClass = "acceptance_timestamp_only"
	}
	evidence, _ := json.Marshal(map[string]string{"evidence": evidenceClass})
	var attempt any
	if candidate.attemptID != "" {
		attempt = string(candidate.attemptID)
	}
	_, err := r.db.ExecContext(ctx, `INSERT INTO reconciliation_log (run_id, attempt_id, observed_at, action, decision, evidence_json) VALUES (?, ?, ?, ?, ?, ?)`,
		string(candidate.runID), attempt, formatTime(r.now()), action, decision, string(evidence))
	if err != nil {
		return classifyStoreError("reconcile_log", "persist reconciliation evidence failed", err)
	}
	return nil
}

// ReconciliationEvidence returns newest-first safe reconciliation facts for a
// bounded support export. The stored evidence payload is deliberately reduced
// to its allowlisted classification rather than exposed as arbitrary JSON.
func (r *SQLiteRepository) ReconciliationEvidence(ctx context.Context, limit int) ([]ReconciliationEvidence, error) {
	if limit < 1 || limit > 200 {
		return nil, invalidField("reconciliation_evidence.limit", "must be between 1 and 200")
	}
	rows, err := r.db.QueryContext(ctx, `
SELECT run_id, attempt_id, observed_at, action, decision, evidence_json
FROM reconciliation_log ORDER BY observed_at DESC, id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, classifyStoreError("reconciliation_evidence", "read reconciliation evidence failed", err)
	}
	defer rows.Close()
	result := make([]ReconciliationEvidence, 0, limit)
	for rows.Next() {
		var item ReconciliationEvidence
		var attempt sql.NullString
		var observed, encodedEvidence string
		if err := rows.Scan(&item.RunID, &attempt, &observed, &item.Action, &item.Decision, &encodedEvidence); err != nil {
			return nil, classifyStoreError("reconciliation_evidence", "decode reconciliation evidence failed", err)
		}
		item.AttemptID = AttemptID(attempt.String)
		item.ObservedAt, err = parseTime(observed)
		if err != nil {
			return nil, err
		}
		var stored map[string]string
		if json.Unmarshal([]byte(encodedEvidence), &stored) == nil {
			switch stored["evidence"] {
			case "safe_identity_only", "acceptance_timestamp_only":
				item.Evidence = stored["evidence"]
			}
		}
		if item.Evidence == "" {
			item.Evidence = "omitted"
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, classifyStoreError("reconciliation_evidence", "read reconciliation evidence failed", err)
	}
	return result, nil
}
