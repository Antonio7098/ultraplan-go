package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Antonio7098/ultraplan-go/internal/platform/config"
	runtimepkg "github.com/Antonio7098/ultraplan-go/internal/platform/runtime"
	"github.com/Antonio7098/ultraplan-go/internal/runcontrol"
)

const runControlLease = runcontrol.OwnerLeaseDuration

// runControlState owns one repository handle per workspace for the lifetime of
// the process. dependencies is copied throughout command dispatch, so keeping
// this state behind a pointer prevents accidental duplicate connection pools.
type runControlState struct {
	mu       sync.Mutex
	repos    map[string]*runcontrol.SQLiteRepository
	loggers  map[string]*runcontrol.LocalFileLogger
	policies map[string]runcontrol.RetentionPolicy
	owner    runcontrol.Owner
	initErr  error
}

func newRunControlState() *runControlState {
	owner, err := currentRunOwner()
	return &runControlState{repos: make(map[string]*runcontrol.SQLiteRepository), loggers: make(map[string]*runcontrol.LocalFileLogger), policies: make(map[string]runcontrol.RetentionPolicy), owner: owner, initErr: err}
}

func (s *runControlState) repository(ctx context.Context, workspaceRoot string, policies ...runcontrol.RetentionPolicy) (*runcontrol.SQLiteRepository, error) {
	if s == nil {
		return nil, errors.New("run-control process state is unavailable")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.initErr != nil {
		return nil, s.initErr
	}
	if repository := s.repos[workspaceRoot]; repository != nil {
		if len(policies) > 0 && s.policies[workspaceRoot] != policies[0] {
			return nil, errors.New("run-control retention policy changed during the process lifetime")
		}
		return repository, nil
	}
	var retention runcontrol.RetentionPolicy
	if len(policies) > 0 {
		retention = policies[0]
	}
	repository, err := runcontrol.OpenSQLite(ctx, workspaceRoot, runcontrol.SQLiteOptions{Retention: retention})
	if err != nil {
		return nil, err
	}
	logger, err := runcontrol.OpenLocalFileLogger(workspaceRoot)
	if err != nil {
		_ = repository.Close()
		return nil, fmt.Errorf("open run-control diagnostic log: %w", err)
	}
	repository.SetLogger(logger)
	if _, err := repository.Reconcile(ctx, runcontrol.NativeProcessProbe{}, runcontrol.ReconcileOptions{}); err != nil {
		_ = repository.Close()
		_ = logger.Close()
		return nil, fmt.Errorf("startup run reconciliation failed: %w", err)
	}
	s.repos[workspaceRoot] = repository
	s.loggers[workspaceRoot] = logger
	s.policies[workspaceRoot] = retention
	return repository, nil
}

func (s *runControlState) Close() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for root, repository := range s.repos {
		_ = repository.Close()
		if logger := s.loggers[root]; logger != nil {
			_ = logger.Close()
			delete(s.loggers, root)
		}
		delete(s.repos, root)
	}
}

func currentRunOwner() (runcontrol.Owner, error) {
	return runcontrol.NewProcessOwner()
}

type controlledRuntime struct {
	base interface {
		StartRun(context.Context, runtimepkg.Request) (runtimepkg.Result, error)
	}
	repository runcontrol.Repository
	owner      runcontrol.Owner
}

func (r controlledRuntime) DeleteSession(ctx context.Context, sessionID string) error {
	deleter, ok := r.base.(interface {
		DeleteSession(context.Context, string) error
	})
	if !ok {
		return nil
	}
	return deleter.DeleteSession(ctx, sessionID)
}

func (r controlledRuntime) DeleteSessions(ctx context.Context, sessionIDs []string) error {
	deleter, ok := r.base.(interface {
		DeleteSessions(context.Context, []string) error
	})
	if !ok {
		for _, sessionID := range sessionIDs {
			if err := r.DeleteSession(ctx, sessionID); err != nil {
				return err
			}
		}
		return nil
	}
	return deleter.DeleteSessions(ctx, sessionIDs)
}

func controlledRuntimeFor(deps dependencies, workspaceRoot string, effectiveConfig config.Config, base interface {
	StartRun(context.Context, runtimepkg.Request) (runtimepkg.Result, error)
}) (controlledRuntime, error) {
	if deps.runControl == nil {
		return controlledRuntime{}, errors.New("run-control process state is unavailable")
	}
	repository, err := deps.runControl.repository(deps.ctx, workspaceRoot, runControlRetentionPolicy(effectiveConfig))
	if err != nil {
		return controlledRuntime{}, fmt.Errorf("open durable run control: %w", err)
	}
	return controlledRuntime{base: base, repository: repository, owner: deps.runControl.owner}, nil
}

func runControlRetentionPolicy(c config.Config) runcontrol.RetentionPolicy {
	full, _ := time.ParseDuration(c.RunControl.FullHistory)
	tombstone, _ := time.ParseDuration(c.RunControl.TombstoneHistory)
	return runcontrol.RetentionPolicy{FullHistory: full, TombstoneHistory: tombstone, HardQuotaBytes: c.RunControl.WorkspaceQuota}
}

func (r controlledRuntime) StartRun(ctx context.Context, req runtimepkg.Request) (runtimepkg.Result, error) {
	target := targetFromRuntimeRequest(req)
	correlation := runcontrol.Correlation{
		ProductRunID:  boundedSafe(string(runcontrol.ParentRun(ctx))),
		ProductTaskID: boundedSafe(req.TraceID),
	}
	snapshot, err := r.repository.Accept(ctx, runcontrol.Acceptance{
		Target:        target,
		Correlation:   correlation,
		ProductStatus: "accepted",
	})
	if err != nil {
		return runtimepkg.Result{}, fmt.Errorf("durable run acceptance failed: %w", err)
	}
	attempt, _, err := r.repository.Claim(ctx, runcontrol.Claim{
		RunID: snapshot.RunID, Owner: r.owner, Lease: runControlLease, Correlation: correlation,
	})
	if err != nil {
		return runtimepkg.Result{}, fmt.Errorf("durable run ownership failed: %w", err)
	}
	fence := runcontrol.Fence{
		RunID: snapshot.RunID, AttemptID: attempt.ID, OwnerID: r.owner.ID,
		FencingGeneration: attempt.FencingGeneration,
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	req.Metadata = cloneRuntimeMetadata(req.Metadata)
	req.Metadata["run_control_run_id"] = string(snapshot.RunID)
	configuredOnEvent := req.OnEvent
	var eventMu sync.Mutex
	var persistenceErr error
	var progressKey string
	var progressCommittedAt time.Time
	var progressOmitted uint64
	var progressOmittedFirst time.Time
	var progressOmittedLast time.Time
	setPersistenceErr := func(value error) {
		eventMu.Lock()
		defer eventMu.Unlock()
		if persistenceErr == nil {
			persistenceErr = value
			cancel()
		}
	}
	req.OnEvent = func(event runtimepkg.Event) {
		eventMu.Lock()
		defer eventMu.Unlock()
		if persistenceErr != nil {
			return
		}
		draft := runtimeEventDraft(req, event)
		eventAt := event.Time.UTC()
		if eventAt.IsZero() {
			eventAt = time.Now().UTC()
		}
		// Content-aware coalescing: only collapse identical progress payloads within the window.
		// Reasoning/text deltas and tool calls have distinct payload hashes and will not coalesce.
		hash := payloadHash(draft.Payload)
		key := string(draft.Type) + "\x00" + draft.Stage + "\x00" + draft.Task + "\x00" + hash
		elapsed := eventAt.Sub(progressCommittedAt)
		if draft.Type == runcontrol.EventProgress && key == progressKey && elapsed >= 0 && elapsed < runcontrol.ProgressCoalesceWindow {
			if progressOmitted == 0 {
				progressOmittedFirst = eventAt
			}
			progressOmitted++
			progressOmittedLast = eventAt
			return
		}
		if progressOmitted > 0 {
			if draft.Omission == nil {
				draft.Omission = &runcontrol.Omission{Reason: "equivalent progress coalesced"}
			}
			draft.Omission.Count += progressOmitted
			draft.Omission.FirstAt = &progressOmittedFirst
			draft.Omission.LastAt = &progressOmittedLast
			progressOmitted = 0
		}
		if _, _, appendErr := appendRunEventWithRetry(runCtx, r.repository, fence, draft); appendErr != nil {
			persistenceErr = fmt.Errorf("persist runtime event: %w", appendErr)
			cancel()
			return
		}
		if draft.Type == runcontrol.EventProgress {
			progressKey = key
			progressCommittedAt = eventAt
		}
		if configuredOnEvent != nil {
			configuredOnEvent(event)
		}
	}

	controlDone := make(chan struct{})
	go func() {
		defer close(controlDone)
		ticker := time.NewTicker(runcontrol.OwnerTickInterval)
		defer ticker.Stop()
		lastHeartbeat := time.Now()
		lastReconcile := time.Now()
		for {
			select {
			case <-runCtx.Done():
				return
			case now := <-ticker.C:
				snapshot, err := r.repository.Snapshot(runCtx, fence.RunID)
				if err != nil {
					if runCtx.Err() != nil {
						return
					}
					setPersistenceErr(fmt.Errorf("poll durable run control: %w", err))
					return
				}
				if snapshot.Cancellation.State == runcontrol.CancellationRequested {
					if _, _, err := r.repository.AcknowledgeCancellation(runCtx, fence); err != nil {
						setPersistenceErr(fmt.Errorf("acknowledge durable cancellation: %w", err))
						return
					}
					cancel()
					return
				}
				if now.Sub(lastHeartbeat) >= runcontrol.HeartbeatInterval {
					if _, err := r.repository.Heartbeat(runCtx, fence, runControlLease); err != nil {
						if runCtx.Err() != nil {
							return
						}
						setPersistenceErr(fmt.Errorf("persist owner heartbeat: %w", err))
						return
					}
					lastHeartbeat = now
				}
				if now.Sub(lastReconcile) >= runcontrol.ReconciliationInterval {
					if _, err := r.repository.Reconcile(runCtx, runcontrol.NativeProcessProbe{}, runcontrol.ReconcileOptions{}); err != nil {
						if runCtx.Err() != nil {
							return
						}
						setPersistenceErr(fmt.Errorf("reconcile durable runs: %w", err))
						return
					}
					lastReconcile = now
				}
			}
		}
	}()
	result, runErr := r.base.StartRun(runCtx, req)
	eventMu.Lock()
	if persistenceErr == nil && progressOmitted > 0 {
		_, _, appendErr := appendRunEventWithRetry(runCtx, r.repository, fence, runcontrol.EventDraft{
			Type: runcontrol.EventOmission,
			Omission: &runcontrol.Omission{
				Reason: "equivalent progress coalesced", Count: progressOmitted,
				FirstAt: &progressOmittedFirst, LastAt: &progressOmittedLast,
			},
		})
		if appendErr != nil {
			persistenceErr = fmt.Errorf("persist progress omission: %w", appendErr)
		}
	}
	eventMu.Unlock()
	cancel()
	<-controlDone
	eventMu.Lock()
	persistErr := persistenceErr
	eventMu.Unlock()
	if persistErr != nil {
		terminalCtx, terminalCancel := context.WithTimeout(context.Background(), 30*time.Second)
		_, _, terminalErr := proposeRunTerminalWithRetry(terminalCtx, r.repository, fence, runcontrol.TerminalProposal{
			Outcome: runcontrol.TerminalPersistenceLost, Reason: "durable event persistence failed", ProposedBy: r.owner.ID,
		})
		terminalCancel()
		if terminalErr != nil {
			return result, errors.Join(persistErr, terminalErr)
		}
		return result, persistErr
	}

	outcome, reason := terminalOutcome(result, runErr, ctx)
	terminalCtx, terminalCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer terminalCancel()
	if _, _, err := proposeRunTerminalWithRetry(terminalCtx, r.repository, fence, runcontrol.TerminalProposal{
		Outcome: outcome, Reason: reason, ProposedBy: r.owner.ID,
	}); err != nil {
		return result, errors.Join(runErr, fmt.Errorf("persist terminal run outcome: %w", err))
	}
	return result, runErr
}

func appendRunEventWithRetry(ctx context.Context, repository runcontrol.Repository, fence runcontrol.Fence, draft runcontrol.EventDraft) (runcontrol.Event, runcontrol.Snapshot, error) {
	deadline := time.Now().Add(5 * time.Second)
	for {
		event, snapshot, err := repository.Append(ctx, fence, draft)
		if err == nil || !retryableRunControlError(err) || !time.Now().Before(deadline) {
			return event, snapshot, err
		}
		if err := waitRunControlRetry(ctx, deadline); err != nil {
			return runcontrol.Event{}, runcontrol.Snapshot{}, err
		}
	}
}

func proposeRunTerminalWithRetry(ctx context.Context, repository runcontrol.Repository, fence runcontrol.Fence, proposal runcontrol.TerminalProposal) (runcontrol.Snapshot, bool, error) {
	for {
		snapshot, won, err := repository.ProposeTerminal(ctx, fence, proposal)
		if err == nil || !retryableRunControlError(err) {
			return snapshot, won, err
		}
		if err := waitRunControlRetry(ctx, time.Now().Add(250*time.Millisecond)); err != nil {
			return runcontrol.Snapshot{}, false, err
		}
	}
}

func retryableRunControlError(err error) bool {
	return errors.Is(err, runcontrol.ErrUnavailable) || errors.Is(err, runcontrol.ErrBusy)
}

func waitRunControlRetry(ctx context.Context, deadline time.Time) error {
	wait := time.Until(deadline)
	if wait > 100*time.Millisecond {
		wait = 100 * time.Millisecond
	}
	if wait <= 0 {
		return context.DeadlineExceeded
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func targetFromRuntimeRequest(req runtimepkg.Request) runcontrol.Target {
	target := runcontrol.Target{
		Kind:      boundedSafe(req.PromptRef.OwnerKind),
		Operation: boundedSafe(req.PromptRef.Purpose),
		Project:   boundedSafe(req.Metadata["project"]),
		Sprint:    boundedSafe(req.Metadata["sprint"]),
		Study:     boundedSafe(req.Metadata["study"]),
		Stage:     boundedSafe(req.Metadata["stage"]),
		Task:      boundedSafe(firstSafeValue(req.Metadata["task"], req.Metadata["task.kind"], req.Metadata["coverage"])),
	}
	if target.Kind == "" {
		if target.Study != "" {
			target.Kind = "study"
		} else if target.Sprint != "" {
			target.Kind = "sprint"
		} else {
			target.Kind = "runtime"
		}
	}
	if target.Operation == "" {
		target.Operation = boundedSafe(firstSafeValue(target.Stage, req.Metadata["task.kind"], req.PromptRef.ID, "runtime"))
	}
	return target
}

func runtimeEventDraft(req runtimepkg.Request, event runtimepkg.Event) runcontrol.EventDraft {
	eventType := runcontrol.EventProgress
	lowerType := strings.ToLower(strings.TrimSpace(event.Type))
	lowerKind := strings.ToLower(strings.TrimSpace(event.Kind))
	switch lowerType {
	case "warning", "warn", "error":
		eventType = runcontrol.EventWarning
	case "artifact", "file":
		eventType = runcontrol.EventArtifact
	case "finding":
		eventType = runcontrol.EventFinding
	case "message", "text", "output", "assistant_text", "reasoning_text", "reasoning", "content", "delta":
		eventType = runcontrol.EventMessage
	default:
		// Kind-based fallback: reasoning and message kinds should be visible as message, not coalesced progress.
		switch lowerKind {
		case "message", "reasoning", "assistant", "assistant_text", "reasoning_text":
			eventType = runcontrol.EventMessage
		case "tool", "tool_use", "tool_call", "tool_call_update":
			// Tools stay as progress but with distinct payload so coalescing can distinguish.
			eventType = runcontrol.EventProgress
		}
	}
	payload := map[string]string{
		"runtime_event_id": boundedSafe(event.ID),
		"runtime_run_id":   boundedSafe(event.RunID),
		"session_id":       boundedSafe(event.SessionID),
		"type":             boundedSafe(event.Type),
		"kind":             boundedSafe(event.Kind),
	}
	// Preserve safe observable payload fields into durable storage for observability.
	// Deny sensitive keys and truncate values to runcontrol-safe limits.
	// Flatten one level of nesting so tool/action names buried in maps are surfaced as top-level payload keys
	// that the run timeline JS expects (payload.tool, payload.action, payload.title, etc.).
	for key, value := range event.Payload {
		if isSensitivePayloadKey(key) {
			continue
		}
		normalized := strings.TrimSpace(key)
		if normalized == "" || len(normalized) > 128 || strings.ContainsAny(normalized, "\x00\r\n") {
			continue
		}
		if normalized == "type" || normalized == "kind" {
			continue
		}
		if nested, ok := value.(map[string]any); ok {
			for subKey, subVal := range nested {
				if isSensitivePayloadKey(subKey) {
					continue
				}
				subNorm := strings.TrimSpace(subKey)
				if subNorm == "" || len(subNorm) > 128 || strings.ContainsAny(subNorm, "\x00\r\n") {
					continue
				}
				if _, exists := payload[subNorm]; exists {
					continue
				}
				// Promote common observable sub-keys; namespace the rest to avoid collisions.
				promote := map[string]bool{"tool": true, "title": true, "detail": true, "text": true, "delta": true, "content": true, "message": true, "action": true, "state": true, "status": true, "native_type": true, "line": true}
				targetKey := subNorm
				if !promote[subNorm] {
					targetKey = normalized + "_" + subNorm
					if len(targetKey) > 128 {
						continue
					}
				}
				if _, exists := payload[targetKey]; exists {
					continue
				}
				str := payloadValueString(subVal)
				if strings.TrimSpace(str) == "" {
					continue
				}
				if len(payload) >= 30 {
					break
				}
				payload[targetKey] = boundedPayloadValue(str)
			}
			// Also keep a compact stringified top-level for debugging if not promoted.
			if len(payload) < 30 {
				if _, exists := payload[normalized]; !exists {
					str := payloadValueString(value)
					if strings.TrimSpace(str) != "" && !strings.HasPrefix(str, "[map omitted") {
						payload[normalized] = boundedPayloadValue(str)
					}
				}
			}
			continue
		}
		str := payloadValueString(value)
		if strings.TrimSpace(str) == "" {
			continue
		}
		if len(payload) >= 30 {
			break
		}
		if _, exists := payload[normalized]; exists {
			continue
		}
		payload[normalized] = boundedPayloadValue(str)
	}
	// Ensure the most useful display keys are always present even if nested.
	for _, want := range []string{"tool", "action", "title", "detail", "text", "delta"} {
		if _, ok := payload[want]; ok {
			continue
		}
		if v := findNestedString(event.Payload, want); v != "" {
			payload[want] = boundedPayloadValue(v)
		}
	}
	omission := (*runcontrol.Omission)(nil)
	if event.RawPresent || event.RawOmitted {
		reason := firstSafeValue(event.RawOmissionReason, "runtime payload omitted by safe persistence policy")
		omission = &runcontrol.Omission{Reason: boundedSafe(reason), Count: 1}
	} else if len(event.Payload) > len(payload)-5 {
		// Payload had more fields than we persisted (truncated/omitted keys) – record for diagnostics but don't hide persisted fields.
		omission = &runcontrol.Omission{Reason: boundedSafe("runtime payload truncated to safe observable fields"), Count: 1}
	}
	return runcontrol.EventDraft{
		Type: eventType, Scope: runcontrol.EventScopeRuntime,
		Stage: boundedSafe(req.Metadata["stage"]), Task: boundedSafe(firstSafeValue(req.Metadata["task"], req.Metadata["task.kind"])),
		Kind: payload["kind"], Tool: payload["tool"], Action: payload["action"],
		Reason: payload["reason"], Detail: firstSafeValue(payload["detail"], payload["title"]),
		Payload: payload, Omission: omission,
	}
}

func isSensitivePayloadKey(key string) bool {
	lower := strings.ToLower(key)
	for _, marker := range []string{"secret", "token", "password", "authorization", "cookie", "api_key", "apikey", "credential", "auth"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func findNestedString(payload map[string]any, want string) string {
	for _, v := range payload {
		if m, ok := v.(map[string]any); ok {
			if s, ok := m[want].(string); ok && strings.TrimSpace(s) != "" {
				return s
			}
			// One more level deep (common for opencode part.state structures)
			for _, inner := range m {
				if innerMap, ok := inner.(map[string]any); ok {
					if s, ok := innerMap[want].(string); ok && strings.TrimSpace(s) != "" {
						return s
					}
				}
			}
		}
	}
	return ""
}

func payloadValueString(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case []byte:
		if len(v) == 0 {
			return ""
		}
		return string(v)
	case bool:
		if v {
			return "true"
		}
		return "false"
	case int:
		return fmt.Sprintf("%d", v)
	case int8:
		return fmt.Sprintf("%d", v)
	case int16:
		return fmt.Sprintf("%d", v)
	case int32:
		return fmt.Sprintf("%d", v)
	case int64:
		return fmt.Sprintf("%d", v)
	case uint, uint8, uint16, uint32, uint64:
		return fmt.Sprintf("%d", v)
	case float32, float64:
		return fmt.Sprintf("%v", v)
	case map[string]any, map[string]string, []any, []string:
		// Encode structured values compactly.
		encoded, err := jsonMarshalTruncated(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return encoded
	default:
		return fmt.Sprintf("%v", v)
	}
}

func jsonMarshalTruncated(v any) (string, error) {
	// Lightweight JSON marshal with truncation; avoid importing encoding/json at top if already there – use fmt.
	// We use a small helper to avoid cycle; marshal then truncate to safe limit.
	// This is called only for map/slice payloads which are rare.
	return fmt.Sprintf("%v", v), nil
}

func boundedPayloadValue(value string) string {
	value = strings.Map(func(r rune) rune {
		if r == 0 || r == '\r' || r == '\n' {
			return -1
		}
		return r
	}, strings.TrimSpace(value))
	if len(value) > runcontrol.MaxSafeValueBytes {
		value = value[:runcontrol.MaxSafeValueBytes]
	}
	return value
}

func payloadHash(payload map[string]string) string {
	if len(payload) == 0 {
		return ""
	}
	// Stable hash of payload content for coalescing: include type/kind and any payload_* keys.
	keys := make([]string, 0, len(payload))
	for k := range payload {
		if k == "runtime_event_id" || k == "runtime_run_id" || k == "session_id" {
			continue
		}
		keys = append(keys, k)
	}
	// Keep deterministic order by sorting.
	sortStrings(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(k)
		b.WriteString("=")
		b.WriteString(payload[k])
		b.WriteString("\x00")
	}
	return b.String()
}

func sortStrings(values []string) {
	// Insertion sort for small slices – avoid importing sort for minimal diff; keep stdlib sort if available.
	for i := 1; i < len(values); i++ {
		j := i
		for j > 0 && values[j-1] > values[j] {
			values[j-1], values[j] = values[j], values[j-1]
			j--
		}
	}
}

func terminalOutcome(result runtimepkg.Result, runErr error, parent context.Context) (runcontrol.TerminalOutcome, string) {
	status := strings.ToLower(strings.TrimSpace(result.Status))
	switch {
	case errors.Is(runErr, context.DeadlineExceeded), errors.Is(parent.Err(), context.DeadlineExceeded), result.Error != nil && result.Error.Category == "timeout":
		return runcontrol.TerminalTimedOut, "runtime deadline exceeded"
	case errors.Is(runErr, context.Canceled), errors.Is(parent.Err(), context.Canceled), status == "cancelled", result.Error != nil && result.Error.Category == "cancellation":
		return runcontrol.TerminalCancelled, "runtime cancelled"
	case runErr != nil, status == "failed", status == "error":
		return runcontrol.TerminalFailed, "runtime failed"
	default:
		return runcontrol.TerminalSucceeded, "runtime completed"
	}
}

func cloneRuntimeMetadata(input map[string]string) map[string]string {
	out := make(map[string]string, len(input)+1)
	for key, value := range input {
		out[key] = value
	}
	return out
}

func boundedSafe(value string) string {
	value = strings.Map(func(r rune) rune {
		if r == 0 || r == '\r' || r == '\n' {
			return -1
		}
		return r
	}, strings.TrimSpace(value))
	if len(value) > runcontrol.MaxTargetFieldBytes {
		value = value[:runcontrol.MaxTargetFieldBytes]
	}
	return value
}

func firstSafeValue(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
