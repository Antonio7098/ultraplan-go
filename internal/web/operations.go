package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Antonio7098/ultraplan-go/internal/app"
)

const (
	MaxActiveOperations        = 8
	MaxPreparations            = 128
	PreparationTTL             = 2 * time.Minute
	MaxEventsPerOperation      = 256
	MaxEventBytesPerOperation  = 256 * 1024
	MaxEncodedEventBytes       = 16 * 1024
	MaxTerminalResultBytes     = 256 * 1024
	MaxSubscribersPerOperation = 8
	MaxConcurrentStreams       = 32
	SubscriberQueueSize        = 32
	TerminalRetention          = 10 * time.Minute
	SSEHeartbeat               = 15 * time.Second
	MaxStreamLifetime          = 30 * time.Minute
)

var (
	errOperationCapacity  = errors.New("operation capacity reached")
	errSubscriberCapacity = errors.New("subscriber capacity reached")
	errServerDraining     = errors.New("server draining")
	errOperationNotFound  = errors.New("operation not found")
)

type operationDocument struct {
	ID            string              `json:"id"`
	Kind          app.OperationKind   `json:"kind"`
	State         string              `json:"state"`
	Reason        string              `json:"reason,omitempty"`
	CreatedAt     time.Time           `json:"created_at"`
	StartedAt     *time.Time          `json:"started_at,omitempty"`
	FinishedAt    *time.Time          `json:"finished_at,omitempty"`
	LastEventID   string              `json:"last_event_id"`
	DurableStatus durableStatusDTO    `json:"durable_status"`
	Result        *operationResultDTO `json:"result,omitempty"`
}

type activeOperationDTO struct {
	ID        string            `json:"id"`
	Kind      app.OperationKind `json:"kind"`
	State     string            `json:"state"`
	Project   string            `json:"project,omitempty"`
	Sprint    string            `json:"sprint,omitempty"`
	Study     string            `json:"study,omitempty"`
	StartedAt *time.Time        `json:"started_at,omitempty"`
}

type durableStatusDTO struct {
	Available   bool   `json:"available"`
	RefreshPath string `json:"refresh_path,omitempty"`
}

type operationResultDTO struct {
	State     string       `json:"state"`
	Subject   string       `json:"subject,omitempty"`
	Message   string       `json:"message,omitempty"`
	Content   string       `json:"content,omitempty"`
	Truncated bool         `json:"truncated,omitempty"`
	Findings  []findingDTO `json:"findings,omitempty"`
	Error     *errorBody   `json:"error,omitempty"`
}

type operationEvent struct {
	ID   uint64
	Name string
	Data []byte
}

type operationSubscriber struct {
	id uint64
	ch chan operationEvent
}

type operationRecord struct {
	doc         operationDocument
	session     string
	request     app.OperationRequest
	cancel      context.CancelFunc
	cancelOnce  sync.Once
	done        chan struct{}
	nextEventID uint64
	events      []operationEvent
	eventBytes  int
	subscribers map[uint64]*operationSubscriber
	nextSubID   uint64
	dedupKey    string
}

type operationHub struct {
	rootCtx context.Context
	ops     app.WebOperations
	now     func() time.Time
	id      func() string

	mu       sync.Mutex
	records  map[string]*operationRecord
	dedup    map[string]string
	active   int
	streams  int
	draining bool
	counters operationCounters
}

type operationCounters struct {
	starts             atomic.Int64
	active             atomic.Int64
	terminal           atomic.Int64
	capacityRejections atomic.Int64
	cancellations      atomic.Int64
	activeStreams      atomic.Int64
	slowSubscribers    atomic.Int64
	replayGaps         atomic.Int64
	projectionDrops    atomic.Int64
	shutdownCleanups   atomic.Int64
}

func newOperationHub(rootCtx context.Context, ops app.WebOperations, now func() time.Time, id func() string) *operationHub {
	if rootCtx == nil {
		rootCtx = context.Background()
	}
	if now == nil {
		now = time.Now
	}
	if id == nil {
		id = randomRequestID
	}
	return &operationHub{rootCtx: rootCtx, ops: ops, now: now, id: id, records: make(map[string]*operationRecord), dedup: make(map[string]string)}
}

func (h *operationHub) start(session string, prepared app.Confirmation) (operationDocument, error) {
	return h.startConfirmed(session, "", func() (app.Confirmation, error) { return prepared, nil })
}

func (h *operationHub) startConfirmed(session, dedupKey string, confirm func() (app.Confirmation, error)) (operationDocument, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.reapLocked()
	if id := h.dedup[dedupKey]; dedupKey != "" && id != "" {
		if record := h.records[id]; record != nil && record.session == session {
			return cloneOperationDocument(record.doc), nil
		}
		delete(h.dedup, dedupKey)
	}
	if h.draining {
		h.counters.capacityRejections.Add(1)
		return operationDocument{}, errServerDraining
	}
	if h.active >= MaxActiveOperations {
		h.counters.capacityRejections.Add(1)
		return operationDocument{}, errOperationCapacity
	}
	if h.ops == nil {
		return operationDocument{}, errors.New("operation capability unavailable")
	}
	prepared, err := confirm()
	if err != nil {
		return operationDocument{}, err
	}
	id := "op_" + h.id()
	for h.records[id] != nil {
		id = "op_" + h.id()
	}
	created := h.now().UTC()
	ctx, cancel := context.WithCancel(h.rootCtx)
	if manager, ok := h.ops.(app.DurableOperationManager); ok {
		accepted, acceptErr := manager.AcceptOperation(ctx, prepared, dedupKey)
		if acceptErr != nil {
			if !errors.Is(acceptErr, app.ErrWebUnavailable) {
				cancel()
				return operationDocument{}, acceptErr
			}
		} else {
			id = accepted.RunID
			if accepted.Context != nil {
				ctx = accepted.Context
			}
			if accepted.Existing {
				doc := operationDocument{ID: id, Kind: prepared.Request.Kind, State: accepted.Lifecycle, CreatedAt: created, DurableStatus: durableStatusDTO{Available: true, RefreshPath: "/runs/" + id}}
				cancel()
				return doc, nil
			}
			if confirmer, ok := h.ops.(app.DurableOperationConfirmer); ok {
				if confirmErr := confirmer.ConfirmAcceptedOperation(ctx, accepted, prepared); confirmErr != nil {
					finishCtx, finishCancel := context.WithTimeout(context.Background(), 30*time.Second)
					_ = manager.FinishOperation(finishCtx, id, app.OperationFailed, confirmErr)
					finishCancel()
					cancel()
					return operationDocument{}, confirmErr
				}
			} else if prepared.Request.Kind == app.OperationRepairStart {
				confirmErr := errors.New("durable repair confirmation capability unavailable")
				finishCtx, finishCancel := context.WithTimeout(context.Background(), 30*time.Second)
				_ = manager.FinishOperation(finishCtx, id, app.OperationFailed, confirmErr)
				finishCancel()
				cancel()
				return operationDocument{}, confirmErr
			}
			if prepared.Request.Kind != app.OperationRepairPrepare {
				dispatched, dispatchErr := manager.DispatchOperation(ctx, id)
				if dispatchErr != nil {
					finishCtx, finishCancel := context.WithTimeout(context.Background(), 30*time.Second)
					_ = manager.FinishOperation(finishCtx, id, app.OperationFailed, dispatchErr)
					finishCancel()
					cancel()
					return operationDocument{}, dispatchErr
				}
				if dispatched.Context != nil {
					ctx = dispatched.Context
				}
			}
		}
	}
	record := &operationRecord{
		doc: operationDocument{
			ID: id, Kind: prepared.Request.Kind, State: "accepted", CreatedAt: created,
			DurableStatus: durableStatusDTO{Available: prepared.DurableRefreshPath != "", RefreshPath: prepared.DurableRefreshPath},
		},
		session: session, request: prepared.Request, cancel: cancel, done: make(chan struct{}),
		subscribers: make(map[uint64]*operationSubscriber), dedupKey: dedupKey,
	}
	h.records[id] = record
	if dedupKey != "" {
		h.dedup[dedupKey] = id
	}
	h.active++
	h.counters.starts.Add(1)
	h.counters.active.Add(1)
	h.appendEventLocked(record, "snapshot", map[string]any{"state": "accepted"})
	doc := cloneOperationDocument(record.doc)
	if prepared.Request.Kind == app.OperationRepairPrepare {
		// Repair preparation is deliberately synchronous and runtime-free. The
		// accepted writer context is sufficient; no ownership-control or worker
		// goroutine may exist before the immutable packet is published.
		h.mu.Unlock()
		h.run(ctx, record)
		h.mu.Lock()
		return cloneOperationDocument(record.doc), nil
	}
	go h.run(ctx, record)
	return doc, nil
}

func (h *operationHub) run(ctx context.Context, record *operationRecord) {
	started := h.now().UTC()
	h.mu.Lock()
	if record.doc.State == "accepted" {
		record.doc.State = "running"
		record.doc.StartedAt = &started
		h.appendEventLocked(record, "progress", map[string]any{"message": "operation started", "state": "running"})
	}
	h.mu.Unlock()

	result, runErr := h.ops.RunOperation(ctx, record.request, func(event app.OperationEvent) {
		h.publishAppEvent(record, event)
	})
	if manager, ok := h.ops.(app.DurableOperationManager); ok {
		finishCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		finishErr := manager.FinishOperation(finishCtx, record.doc.ID, result.State, runErr)
		cancel()
		if finishErr != nil && !errors.Is(finishErr, app.ErrWebUnavailable) {
			runErr = errors.Join(runErr, finishErr)
		}
	}
	h.finish(record, result, runErr)
}

func (h *operationHub) publishAppEvent(record *operationRecord, event app.OperationEvent) {
	if manager, ok := h.ops.(app.DurableOperationManager); ok {
		committed, err := manager.RecordOperationEvent(context.Background(), record.doc.ID, event)
		if err != nil && !errors.Is(err, app.ErrWebUnavailable) {
			record.cancel()
			return
		}
		if err == nil && !committed {
			return
		}
	}
	name := "progress"
	if event.State == app.OperationFailed {
		name = "warning"
	}
	payload := map[string]any{
		"state": string(event.State), "stage": safeWebText(event.Stage), "task": safeWebText(event.Task),
		"message": safeWebText(event.Message), "completed": event.Completed, "total": event.Total, "attempt": event.Attempt,
		"event_kind": safeWebText(event.EventKind), "event_type": safeWebText(event.EventType), "tool": safeWebText(event.Tool), "action": safeWebText(event.Action),
		"tool_call_id": safeWebText(event.ToolCallID), "tool_status": safeWebText(event.ToolStatus),
		"tool_arguments": event.ToolArguments, "tool_result": event.ToolResult, "tool_error": event.ToolError,
		"reason": safeWebText(event.Reason), "detail": safeWebText(event.Detail),
		"runtime_attempts": event.RuntimeAttempts, "turns": event.Turns, "turns_known": event.TurnsKnown,
		"tokens": event.Tokens, "tokens_known": event.TokensKnown, "duration": safeWebText(event.Duration),
		"provider": safeWebText(event.Provider), "model": safeWebText(event.Model), "harness": safeWebText(event.Harness), "cost": safeWebText(event.Cost), "runtime_events": event.RuntimeEvents,
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if terminalOperationState(record.doc.State) {
		return
	}
	h.appendEventLocked(record, name, payload)
}

func (h *operationHub) finish(record *operationRecord, result app.OperationResult, runErr error) {
	finished := h.now().UTC()
	h.mu.Lock()
	defer h.mu.Unlock()
	if terminalOperationState(record.doc.State) {
		return
	}
	state := "succeeded"
	switch {
	case errors.Is(runErr, context.Canceled), result.State == app.OperationCancelled:
		state = "cancelled"
	case runErr != nil, result.State == app.OperationFailed:
		state = "failed"
	case result.State == app.OperationPartial:
		state = "interrupted"
	}
	record.doc.State = state
	record.doc.FinishedAt = &finished
	record.doc.Result = projectOperationResult(result)
	h.appendEventLocked(record, "terminal", map[string]any{"state": state, "reason": record.doc.Reason, "result": record.doc.Result})
	h.active--
	h.counters.active.Add(-1)
	h.counters.terminal.Add(1)
	close(record.done)
	for id, subscriber := range record.subscribers {
		close(subscriber.ch)
		delete(record.subscribers, id)
		h.streams--
		h.counters.activeStreams.Add(-1)
	}
}

func (h *operationHub) status(session, id string) (operationDocument, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.reapLocked()
	record := h.records[id]
	if record == nil || record.session != session {
		return operationDocument{}, errOperationNotFound
	}
	return cloneOperationDocument(record.doc), nil
}

func (h *operationHub) activeOperations(session string) []activeOperationDTO {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.reapLocked()
	result := make([]activeOperationDTO, 0, h.active)
	for _, record := range h.records {
		if record.session != session || terminalOperationState(record.doc.State) {
			continue
		}
		result = append(result, activeOperationDTO{
			ID: record.doc.ID, Kind: record.doc.Kind, State: record.doc.State,
			Project: record.request.Project, Sprint: record.request.Sprint, Study: record.request.Study,
			StartedAt: record.doc.StartedAt,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		left, right := result[i].StartedAt, result[j].StartedAt
		if left == nil || right == nil {
			return left != nil
		}
		return left.After(*right)
	})
	return result
}

func (h *operationHub) cancelOperation(session, id, reason string) (operationDocument, bool, error) {
	h.mu.Lock()
	record := h.records[id]
	if record == nil || (session != "" && record.session != session) {
		h.mu.Unlock()
		return operationDocument{}, false, errOperationNotFound
	}
	if terminalOperationState(record.doc.State) {
		doc := cloneOperationDocument(record.doc)
		h.mu.Unlock()
		return doc, false, nil
	}
	requested := false
	record.cancelOnce.Do(func() {
		requested = true
		record.doc.State = "cancelling"
		record.doc.Reason = canonicalCancelReason(reason)
		h.appendEventLocked(record, "cancel_requested", map[string]any{"reason": record.doc.Reason})
	})
	doc := cloneOperationDocument(record.doc)
	cancel := record.cancel
	h.mu.Unlock()
	if requested {
		h.counters.cancellations.Add(1)
		cancel()
	}
	return doc, requested, nil
}

func (h *operationHub) subscribe(session, id string, lastID uint64) ([]operationEvent, <-chan operationEvent, func(), error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.reapLocked()
	record := h.records[id]
	if record == nil || record.session != session {
		return nil, nil, nil, errOperationNotFound
	}
	if h.streams >= MaxConcurrentStreams || len(record.subscribers) >= MaxSubscribersPerOperation {
		h.counters.capacityRejections.Add(1)
		return nil, nil, nil, errSubscriberCapacity
	}
	if lastID > 0 && len(record.events) > 0 && lastID < record.events[0].ID-1 {
		h.counters.replayGaps.Add(1)
		h.appendEventLocked(record, "recovery_required", map[string]any{
			"oldest_retained_id": strconv.FormatUint(record.events[0].ID, 10),
			"newest_retained_id": strconv.FormatUint(record.nextEventID, 10),
			"refresh_path":       record.doc.DurableStatus.RefreshPath,
		})
		h.appendEventLocked(record, "snapshot", map[string]any{"state": record.doc.State, "reason": record.doc.Reason})
	}
	replay := make([]operationEvent, 0, len(record.events))
	for _, event := range record.events {
		if event.ID > lastID {
			replay = append(replay, event)
		}
	}
	record.nextSubID++
	sub := &operationSubscriber{id: record.nextSubID, ch: make(chan operationEvent, SubscriberQueueSize)}
	if terminalOperationState(record.doc.State) {
		close(sub.ch)
	} else {
		record.subscribers[sub.id] = sub
		h.streams++
		h.counters.activeStreams.Add(1)
	}
	var once sync.Once
	unsubscribe := func() {
		once.Do(func() {
			h.mu.Lock()
			defer h.mu.Unlock()
			if current, ok := record.subscribers[sub.id]; ok {
				delete(record.subscribers, sub.id)
				h.streams--
				h.counters.activeStreams.Add(-1)
				close(current.ch)
			}
		})
	}
	return replay, sub.ch, unsubscribe, nil
}

func (h *operationHub) appendEventLocked(record *operationRecord, name string, payload any) {
	name = stableOperationEventName(name)
	record.nextEventID++
	body := map[string]any{
		"operation_id": record.doc.ID,
		"time":         h.now().UTC().Format(time.RFC3339Nano),
		"sequence":     record.nextEventID,
		"payload":      payload,
	}
	data, err := json.Marshal(body)
	if err != nil {
		h.counters.projectionDrops.Add(1)
		return
	}
	if len(data) > MaxEncodedEventBytes {
		data, _ = json.Marshal(map[string]any{
			"operation_id": record.doc.ID, "time": h.now().UTC().Format(time.RFC3339Nano),
			"sequence": record.nextEventID, "payload": map[string]any{"message": "event projection exceeded the safe size limit"},
		})
		name = "warning"
	}
	event := operationEvent{ID: record.nextEventID, Name: name, Data: data}
	record.events = append(record.events, event)
	record.eventBytes += len(data)
	for len(record.events) > MaxEventsPerOperation || record.eventBytes > MaxEventBytesPerOperation {
		record.eventBytes -= len(record.events[0].Data)
		record.events = record.events[1:]
	}
	record.doc.LastEventID = strconv.FormatUint(event.ID, 10)
	for id, subscriber := range record.subscribers {
		select {
		case subscriber.ch <- event:
		default:
			close(subscriber.ch)
			delete(record.subscribers, id)
			h.streams--
			h.counters.activeStreams.Add(-1)
			h.counters.slowSubscribers.Add(1)
		}
	}
}

func stableOperationEventName(name string) string {
	switch name {
	case "snapshot", "progress", "warning", "finding", "artifact", "cancel_requested", "recovery_required", "terminal":
		return name
	default:
		return "progress"
	}
}

func (h *operationHub) drainAndWait(ctx context.Context) error {
	h.mu.Lock()
	h.draining = true
	var records []*operationRecord
	for _, record := range h.records {
		if !terminalOperationState(record.doc.State) {
			records = append(records, record)
		}
	}
	h.mu.Unlock()
	for _, record := range records {
		_, _, _ = h.cancelOperation("", record.doc.ID, "server_shutdown")
	}
	h.counters.shutdownCleanups.Add(int64(len(records)))
	for _, record := range records {
		select {
		case <-record.done:
		case <-ctx.Done():
			persistErr := h.persistCleanupUncertain(records)
			h.markCleanupUncertain(records)
			return errors.Join(ctx.Err(), persistErr)
		}
	}
	return nil
}

func (h *operationHub) persistCleanupUncertain(records []*operationRecord) error {
	recorder, ok := h.ops.(app.OperationCleanupRecorder)
	if !ok {
		return errors.New("operation capability cannot persist cleanup uncertainty")
	}
	var result error
	for _, record := range records {
		h.mu.Lock()
		terminal := terminalOperationState(record.doc.State)
		h.mu.Unlock()
		if terminal {
			continue
		}
		persistCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		err := recorder.RecordOperationCleanupUncertain(persistCtx, app.OperationCleanupUncertain{
			OperationID: record.doc.ID,
			Request:     record.request,
			Reason:      "server_shutdown",
			RecordedAt:  h.now().UTC(),
		})
		cancel()
		if err != nil {
			result = errors.Join(result, fmt.Errorf("persist cleanup uncertainty for %s: %w", record.doc.ID, err))
		}
	}
	return result
}

func (h *operationHub) markCleanupUncertain(records []*operationRecord) {
	h.mu.Lock()
	defer h.mu.Unlock()
	finished := h.now().UTC()
	for _, record := range records {
		if terminalOperationState(record.doc.State) {
			continue
		}
		record.doc.State = "cleanup_uncertain"
		record.doc.Reason = "server_shutdown"
		record.doc.FinishedAt = &finished
		record.doc.Result = &operationResultDTO{State: "cleanup_uncertain", Message: "Cleanup did not finish before the shutdown deadline. Refresh durable status before retrying."}
		h.appendEventLocked(record, "terminal", map[string]any{"state": record.doc.State, "reason": record.doc.Reason})
		h.active--
		h.counters.active.Add(-1)
		h.counters.terminal.Add(1)
		close(record.done)
		for id, subscriber := range record.subscribers {
			close(subscriber.ch)
			delete(record.subscribers, id)
			h.streams--
			h.counters.activeStreams.Add(-1)
		}
	}
}

func (h *operationHub) isDraining() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.draining
}

func (h *operationHub) reapLocked() {
	now := h.now().UTC()
	for id, record := range h.records {
		if record.doc.FinishedAt != nil && now.Sub(*record.doc.FinishedAt) >= TerminalRetention {
			delete(h.records, id)
			if record.dedupKey != "" {
				delete(h.dedup, record.dedupKey)
			}
		}
	}
}

func projectOperationResult(result app.OperationResult) *operationResultDTO {
	out := &operationResultDTO{
		State: safeWebText(string(result.State)), Subject: safeWebText(result.Subject), Message: safeWebText(result.Message),
		Content: safeProjectedText(result.Content, MaxTerminalResultBytes/2), Truncated: result.Truncated,
	}
	for _, finding := range result.Findings {
		out.Findings = append(out.Findings, findingDTO{
			Severity: safeWebText(finding.Severity), Section: safeWebText(finding.Section), Problem: safeWebText(finding.Problem),
			Cause: safeWebText(finding.Cause), Suggestion: safeWebText(finding.Suggestion),
		})
	}
	if result.Error != nil {
		out.Error = &errorBody{Code: safeWebText(result.Error.Code), Message: safeWebText(result.Error.Message), Retryable: result.Error.Retryable, Details: map[string]any{
			"category": safeWebText(result.Error.Category), "component": safeWebText(result.Error.Component), "cause": safeWebText(result.Error.Cause), "guidance": safeWebText(result.Error.Guidance),
		}}
	}
	if data, _ := json.Marshal(out); len(data) > MaxTerminalResultBytes {
		out.Content = ""
		out.Findings = nil
		out.Truncated = true
		out.Message = safeBoundedText(out.Message, 4096)
	}
	return out
}

func cloneOperationDocument(doc operationDocument) operationDocument {
	clone := doc
	if doc.Result != nil {
		result := *doc.Result
		result.Findings = append([]findingDTO(nil), doc.Result.Findings...)
		if doc.Result.Error != nil {
			err := *doc.Result.Error
			result.Error = &err
		}
		clone.Result = &result
	}
	return clone
}

func terminalOperationState(state string) bool {
	switch state {
	case "succeeded", "failed", "cancelled", "interrupted", "cleanup_uncertain":
		return true
	default:
		return false
	}
}

func canonicalCancelReason(reason string) string {
	switch reason {
	case "server_shutdown", "timeout", "recovery":
		return reason
	default:
		return "user_request"
	}
}

func safeWebText(value string) string {
	return safeProjectedText(value, 4096)
}

func safeProjectedText(value string, limit int) string {
	value = strings.TrimSpace(value)
	lower := strings.ToLower(value)
	for _, marker := range []string{"token=", "secret=", "authorization:", "cookie:"} {
		if index := strings.Index(lower, marker); index >= 0 {
			value = value[:index] + "[redacted]"
			lower = strings.ToLower(value)
		}
	}
	if strings.Contains(value, "/home/") || strings.Contains(value, `C:\\Users\\`) {
		value = "[redacted path]"
	}
	return safeBoundedText(value, limit)
}

func safeBoundedText(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "…"
}

func parseEventID(value string) (uint64, error) {
	if strings.TrimSpace(value) == "" {
		return 0, nil
	}
	id, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid Last-Event-ID")
	}
	return id, nil
}
