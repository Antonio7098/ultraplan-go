package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Antonio7098/ultraplan-go/internal/app"
)

type operationScopeRequest struct {
	Project string `json:"project,omitempty"`
	Sprint  string `json:"sprint,omitempty"`
	Study   string `json:"study,omitempty"`
}

type operationOptionsRequest struct {
	Stage             string   `json:"stage,omitempty"`
	ToStage           string   `json:"to_stage,omitempty"`
	Task              string   `json:"task,omitempty"`
	Shard             string   `json:"shard,omitempty"`
	Model             string   `json:"model,omitempty"`
	Action            string   `json:"action,omitempty"`
	DryRun            bool     `json:"dry_run,omitempty"`
	Resume            bool     `json:"resume,omitempty"`
	Level             string   `json:"level,omitempty"`
	Suite             string   `json:"suite,omitempty"`
	Test              string   `json:"test,omitempty"`
	Timeout           string   `json:"timeout,omitempty"`
	ForceReview       bool     `json:"force_review,omitempty"`
	RestartReview     bool     `json:"restart_review,omitempty"`
	OverrideRationale string   `json:"override_rationale,omitempty"`
	ReviewFocus       []string `json:"review_focus,omitempty"`
	Sources           []string `json:"sources,omitempty"`
	Dimensions        []string `json:"dimensions,omitempty"`
	Parallelism       int      `json:"parallelism,omitempty"`
}

type operationSpecRequest struct {
	Kind    string                  `json:"kind"`
	Scope   operationScopeRequest   `json:"scope"`
	Options operationOptionsRequest `json:"options,omitempty"`
}

type prepareRequest struct {
	Operation operationSpecRequest `json:"operation"`
}

type startRequest struct {
	Operation         operationSpecRequest `json:"operation"`
	ConfirmationToken string               `json:"confirmation_token"`
}

type preparationDTO struct {
	PreparationID     string         `json:"preparation_id"`
	Operation         map[string]any `json:"operation"`
	AffectedPaths     []string       `json:"affected_paths"`
	MutationClass     string         `json:"mutation_class"`
	Runtime           map[string]any `json:"runtime,omitempty"`
	Harness           map[string]any `json:"harness,omitempty"`
	Prerequisites     []string       `json:"prerequisites"`
	InputFingerprint  string         `json:"input_fingerprint"`
	ExpiresAt         time.Time      `json:"expires_at"`
	ConfirmationToken string         `json:"confirmation_token"`
}

type operationPreparationView struct {
	Preparation preparationDTO
	Spec        operationSpecRequest
}

func (h *handler) handleOperationPrepare(w http.ResponseWriter, r *http.Request) {
	if h.hub == nil || h.hub.ops == nil {
		h.writeOperationError(w, r, errors.New("operation capability unavailable"))
		return
	}
	if h.hub.isDraining() {
		h.writeOperationError(w, r, errServerDraining)
		return
	}
	var payload prepareRequest
	if err := decodeStrictJSON(r, &payload); err != nil {
		h.writeOperationError(w, r, err)
		return
	}
	req, err := mapOperationRequest(payload.Operation)
	if err != nil {
		h.writeOperationError(w, r, err)
		return
	}
	confirmation, err := h.hub.ops.PrepareOperation(r.Context(), req)
	if err != nil {
		h.writeOperationError(w, r, err)
		return
	}
	record, err := h.preparations.issue(sessionID(r.Context()), confirmation)
	if err != nil {
		h.writeOperationError(w, r, err)
		return
	}
	h.logOperation(r, "operation_prepared", "", string(confirmation.Request.Kind), "prepared", "")
	dto := preparationDTO{
		PreparationID: record.ID, Operation: mapOperationSpec(confirmation.Request), AffectedPaths: append([]string(nil), confirmation.Paths...),
		MutationClass: confirmation.MutationClass, Prerequisites: append([]string(nil), confirmation.Prerequisites...),
		InputFingerprint: confirmation.InputFingerprint, ExpiresAt: record.ExpiresAt, ConfirmationToken: record.Token,
	}
	if confirmation.Runtime {
		dto.Runtime = runtimeConfirmationDTO(confirmation)
	}
	if confirmation.Request.Kind == app.OperationSmokeStart || confirmation.Request.Kind == app.OperationSmokeDryRun {
		dto.Harness = map[string]any{"kind": "configured_smoke_harness"}
	}
	h.writeSuccess(w, r, http.StatusOK, dto, nil)
}

// runtimeConfirmationDTO projects the runtime identity confirmed for an
// operation, including any caller-requested model override.
func runtimeConfirmationDTO(confirmation app.Confirmation) map[string]any {
	runtime := map[string]any{"kind": "configured_runtime", "model_source": confirmation.ModelSource}
	if model := strings.TrimSpace(confirmation.Request.Model); model != "" {
		runtime["model"] = model
	}
	return runtime
}

func (h *handler) handleOperationStart(w http.ResponseWriter, r *http.Request) {
	if h.hub == nil || h.hub.ops == nil {
		h.writeOperationError(w, r, errors.New("operation capability unavailable"))
		return
	}
	var payload startRequest
	if err := decodeStrictJSON(r, &payload); err != nil {
		h.writeOperationError(w, r, err)
		return
	}
	if strings.TrimSpace(payload.ConfirmationToken) == "" {
		h.writeOperationError(w, r, fmt.Errorf("confirmation_token is required"))
		return
	}
	req, err := mapOperationRequest(payload.Operation)
	if err != nil {
		h.writeOperationError(w, r, err)
		return
	}
	current, err := h.hub.ops.PrepareOperation(r.Context(), req)
	if err != nil {
		h.writeOperationError(w, r, err)
		return
	}
	doc, err := h.hub.startConfirmed(sessionID(r.Context()), confirmationDedupKey(sessionID(r.Context()), payload.ConfirmationToken), func() (app.Confirmation, error) {
		return h.preparations.consume(payload.ConfirmationToken, sessionID(r.Context()), current.CanonicalRequest, current.InputFingerprint)
	})
	if err != nil {
		h.writeOperationError(w, r, err)
		return
	}
	w.Header().Set("Location", "/api/v1/operations/"+doc.ID)
	w.Header().Set("Link", "</api/v1/runs/"+doc.ID+">; rel=canonical")
	h.logOperation(r, "operation_started", doc.ID, string(doc.Kind), doc.State, "")
	h.writeSuccess(w, r, http.StatusAccepted, doc, nil)
}

func (h *handler) handleOperationStatus(w http.ResponseWriter, r *http.Request, id string) {
	doc, err := h.hub.status(sessionID(r.Context()), id)
	if err != nil {
		doc, err = h.durableOperationStatus(r.Context(), id)
	}
	if err != nil {
		if legacyOperationID(id) {
			h.writeError(w, r, http.StatusGone, "legacy_operation_not_retained", "This pre-durable operation is no longer retained. Refresh the owning product or inspect canonical workspace runs.")
			return
		}
		h.writeOperationError(w, r, err)
		return
	}
	h.writeSuccess(w, r, http.StatusOK, doc, nil)
}

func (h *handler) handleActiveOperations(w http.ResponseWriter, r *http.Request) {
	operations := h.hub.activeOperations(sessionID(r.Context()))
	if h.runs != nil {
		page, err := h.runs.Runs(r.Context(), app.RunQuery{Lifecycle: []app.RunLifecycle{"accepted", "queued", "running", "cancelling"}, TargetKind: "operation", Limit: 200})
		if err == nil {
			seen := make(map[string]bool, len(operations))
			for _, operation := range operations {
				seen[operation.ID] = true
			}
			for _, snapshot := range page.Runs {
				if seen[string(snapshot.RunID)] || !snapshot.Lifecycle.IsActive() || snapshot.Target.Kind != "operation" {
					continue
				}
				operations = append(operations, durableActiveOperation(snapshot))
			}
		}
	}
	count := len(operations)
	h.writeSuccess(w, r, http.StatusOK, operations, &app.CollectionInfo{ReturnedCount: count, TotalCount: count})
}

func (h *handler) handleOperationCancel(w http.ResponseWriter, r *http.Request, id string) {
	doc, requested, err := h.hub.cancelOperation(sessionID(r.Context()), id, "user_request")
	if err != nil && h.runs != nil {
		snapshot, changed, cancelErr := h.runs.CancelRun(r.Context(), app.RunID(id), "user_requested")
		if cancelErr == nil && string(snapshot.RunID) == id {
			doc, requested, err = durableOperationDocument(snapshot), changed, nil
		}
	}
	if err != nil {
		if legacyOperationID(id) {
			h.writeError(w, r, http.StatusGone, "legacy_operation_not_retained", "This pre-durable operation is no longer retained. Refresh the owning product or inspect canonical workspace runs.")
			return
		}
		h.writeOperationError(w, r, err)
		return
	}
	status := http.StatusOK
	if requested {
		status = http.StatusAccepted
	}
	h.logOperation(r, "operation_cancel", doc.ID, string(doc.Kind), doc.State, doc.Reason)
	h.writeSuccess(w, r, status, doc, nil)
}

func (h *handler) handleOperationEvents(w http.ResponseWriter, r *http.Request, id string) {
	lastID, err := parseEventID(r.Header.Get("Last-Event-ID"))
	if err != nil {
		h.writeOperationError(w, r, err)
		return
	}
	replay, events, unsubscribe, err := h.hub.subscribe(sessionID(r.Context()), id, lastID)
	if err != nil {
		if h.followDurableOperationEvents(w, r, id, lastID) {
			return
		}
		if legacyOperationID(id) {
			h.writeError(w, r, http.StatusGone, "legacy_operation_not_retained", "This pre-durable operation is no longer retained. Refresh the owning product or inspect canonical workspace runs.")
			return
		}
		h.writeOperationError(w, r, err)
		return
	}
	defer unsubscribe()
	flusher, ok := w.(http.Flusher)
	if !ok {
		h.writeOperationError(w, r, errors.New("streaming unavailable"))
		return
	}
	_ = http.NewResponseController(w).SetWriteDeadline(h.now().Add(MaxStreamLifetime + SSEHeartbeat))
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	for _, event := range replay {
		if err := writeSSEEvent(w, event); err != nil {
			return
		}
	}
	flusher.Flush()
	heartbeat := time.NewTicker(SSEHeartbeat)
	defer heartbeat.Stop()
	lifetime := time.NewTimer(MaxStreamLifetime)
	defer lifetime.Stop()
	for {
		select {
		case event, open := <-events:
			if !open {
				flusher.Flush()
				return
			}
			if err := writeSSEEvent(w, event); err != nil {
				return
			}
			flusher.Flush()
		case <-heartbeat.C:
			if _, err := io.WriteString(w, ": heartbeat\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case <-lifetime.C:
			return
		case <-r.Context().Done():
			return
		}
	}
}

func (h *handler) handleHTMLOperationPrepare(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil || r.FormValue("_csrf") != csrfToken(r.Context()) {
		h.renderError(w, r, http.StatusForbidden, "Request rejected", "The browser session or CSRF proof is invalid. Refresh the page and try again.")
		return
	}
	if h.hub == nil || h.hub.ops == nil || h.hub.isDraining() {
		h.renderError(w, r, http.StatusServiceUnavailable, "Server draining", "The server is not accepting new operations.")
		return
	}
	spec := operationSpecFromForm(r)
	req, err := mapOperationRequest(spec)
	if err != nil {
		h.renderError(w, r, http.StatusBadRequest, "Invalid operation", "The selected operation scope is invalid.")
		return
	}
	prepared, err := h.hub.ops.PrepareOperation(r.Context(), req)
	if err != nil {
		h.renderError(w, r, http.StatusUnprocessableEntity, "Preparation failed", htmlOperationFailure("The operation could not be prepared from current governed state.", err))
		return
	}
	record, err := h.preparations.issue(sessionID(r.Context()), prepared)
	if err != nil {
		h.renderError(w, r, http.StatusTooManyRequests, "Capacity reached", "Too many preparations are active. Wait briefly and retry.")
		return
	}
	dto := preparationDTO{
		PreparationID: record.ID, Operation: mapOperationSpec(prepared.Request), AffectedPaths: append([]string(nil), prepared.Paths...),
		MutationClass: prepared.MutationClass, Prerequisites: append([]string(nil), prepared.Prerequisites...),
		InputFingerprint: prepared.InputFingerprint, ExpiresAt: record.ExpiresAt, ConfirmationToken: record.Token,
	}
	if prepared.Runtime {
		dto.Runtime = runtimeConfirmationDTO(prepared)
	}
	view := &operationPreparationView{Preparation: dto, Spec: spec}
	h.render(w, r, http.StatusOK, "operation-confirm", pageModel{Title: "Confirm operation", Heading: "Confirm current operation scope", Preparation: view})
}

func (h *handler) handleHTMLOperationStart(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil || r.FormValue("_csrf") != csrfToken(r.Context()) {
		h.renderError(w, r, http.StatusForbidden, "Request rejected", "The browser session or CSRF proof is invalid. Refresh and prepare again.")
		return
	}
	if h.hub == nil || h.hub.ops == nil {
		h.renderError(w, r, http.StatusServiceUnavailable, "Unavailable", "The operation capability is unavailable.")
		return
	}
	spec := operationSpecFromForm(r)
	req, err := mapOperationRequest(spec)
	if err != nil {
		h.renderError(w, r, http.StatusBadRequest, "Invalid operation", "The selected operation scope is invalid.")
		return
	}
	current, err := h.hub.ops.PrepareOperation(r.Context(), req)
	if err != nil {
		h.renderError(w, r, http.StatusUnprocessableEntity, "Preparation stale", htmlOperationFailure("The operation can no longer be prepared. Return to the owning page and prepare again.", err))
		return
	}
	token := r.FormValue("confirmation_token")
	doc, err := h.hub.startConfirmed(sessionID(r.Context()), confirmationDedupKey(sessionID(r.Context()), token), func() (app.Confirmation, error) {
		return h.preparations.consume(token, sessionID(r.Context()), current.CanonicalRequest, current.InputFingerprint)
	})
	if err != nil {
		h.renderError(w, r, http.StatusConflict, "Confirmation rejected", "The confirmation expired, changed, or was already used. Prepare the operation again.")
		return
	}
	h.logOperation(r, "operation_started", doc.ID, string(doc.Kind), doc.State, "")
	http.Redirect(w, r, "/operations/"+doc.ID, http.StatusSeeOther)
}

func (h *handler) handleHTMLOperationStatus(w http.ResponseWriter, r *http.Request, id string) {
	if h.runs != nil {
		if snapshot, runErr := h.runs.Run(r.Context(), app.RunID(id)); runErr == nil && string(snapshot.RunID) == id && snapshot.Target.Kind == "operation" {
			http.Redirect(w, r, "/runs/"+id, http.StatusSeeOther)
			return
		}
	}
	doc, err := h.hub.status(sessionID(r.Context()), id)
	if err != nil {
		if legacyOperationID(id) {
			h.renderError(w, r, http.StatusGone, "Legacy operation not retained", "This pre-durable operation expired. Refresh the owning product or open the workspace run list.")
			return
		}
		h.renderError(w, r, http.StatusNotFound, "Operation not retained", "Refresh the owning project, sprint, or study page for durable status.")
		return
	}
	h.render(w, r, http.StatusOK, "operation", pageModel{Title: "Operation " + doc.ID, Heading: "Operation status", Operation: &doc})
}

func (h *handler) handleHTMLOperationCancel(w http.ResponseWriter, r *http.Request, id string) {
	if err := r.ParseForm(); err != nil || r.FormValue("_csrf") != csrfToken(r.Context()) {
		h.renderError(w, r, http.StatusForbidden, "Request rejected", "The browser session or CSRF proof is invalid. Refresh the operation page and try again.")
		return
	}
	if h.hub == nil {
		h.renderError(w, r, http.StatusServiceUnavailable, "Unavailable", "The operation capability is unavailable.")
		return
	}
	doc, requested, err := h.hub.cancelOperation(sessionID(r.Context()), id, "user_request")
	if err != nil && h.runs != nil {
		snapshot, changed, cancelErr := h.runs.CancelRun(r.Context(), app.RunID(id), "user_requested")
		if cancelErr == nil && string(snapshot.RunID) == id {
			doc, requested, err = durableOperationDocument(snapshot), changed, nil
		}
	}
	if err != nil {
		if legacyOperationID(id) {
			h.renderError(w, r, http.StatusGone, "Legacy operation not retained", "This pre-durable operation expired. Refresh the owning product or open the workspace run list.")
			return
		}
		h.renderError(w, r, http.StatusNotFound, "Operation not retained", "Refresh the owning project, sprint, or study page for durable status.")
		return
	}
	if requested {
		h.logOperation(r, "operation_cancel_requested", doc.ID, string(doc.Kind), doc.State, doc.Reason)
	}
	http.Redirect(w, r, "/operations/"+doc.ID, http.StatusSeeOther)
}

func (h *handler) durableOperationStatus(ctx context.Context, id string) (operationDocument, error) {
	if h.runs == nil {
		return operationDocument{}, errOperationNotFound
	}
	snapshot, err := h.runs.Run(ctx, app.RunID(id))
	if err != nil || string(snapshot.RunID) != id || snapshot.Target.Kind != "operation" {
		return operationDocument{}, errOperationNotFound
	}
	return durableOperationDocument(snapshot), nil
}

func legacyOperationID(id string) bool { return strings.HasPrefix(id, "op_") }

func durableOperationDocument(snapshot app.RunSnapshot) operationDocument {
	reason := snapshot.Cancellation.Reason
	if snapshot.Terminal != nil && snapshot.Terminal.Reason != "" {
		reason = snapshot.Terminal.Reason
	}
	return operationDocument{
		ID: string(snapshot.RunID), Kind: app.OperationKind(snapshot.Target.Operation), State: string(snapshot.Lifecycle), Reason: reason,
		CreatedAt: snapshot.AcceptedAt, StartedAt: snapshot.StartedAt, FinishedAt: snapshot.FinishedAt,
		LastEventID:   strconv.FormatUint(snapshot.LastSequence, 10),
		DurableStatus: durableStatusDTO{Available: true, RefreshPath: "/runs/" + string(snapshot.RunID)},
	}
}

func durableActiveOperation(snapshot app.RunSnapshot) activeOperationDTO {
	return activeOperationDTO{
		ID: string(snapshot.RunID), Kind: app.OperationKind(snapshot.Target.Operation), State: string(snapshot.Lifecycle),
		Project: snapshot.Target.Project, Sprint: snapshot.Target.Sprint, Study: snapshot.Target.Study, StartedAt: snapshot.StartedAt,
	}
}

// followDurableOperationEvents is the compatibility projection for a run that
// was accepted by this or another local server but is absent from this
// process's transient operation hub.
func (h *handler) followDurableOperationEvents(w http.ResponseWriter, r *http.Request, id string, after uint64) bool {
	if h.runs == nil {
		return false
	}
	snapshot, err := h.runs.Run(r.Context(), app.RunID(id))
	if err != nil || string(snapshot.RunID) != id || snapshot.Target.Kind != "operation" {
		return false
	}
	if after > snapshot.LastSequence {
		h.writeError(w, r, http.StatusConflict, "cursor_ahead", "The event cursor is ahead of the durable operation.")
		return true
	}
	if after+1 < snapshot.OldestRetainedSequence {
		h.writeError(w, r, http.StatusConflict, "replay_gap", "The requested operation history is no longer retained; refresh its canonical run.")
		return true
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		h.writeOperationError(w, r, errors.New("streaming unavailable"))
		return true
	}
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()
	heartbeat := time.NewTicker(SSEHeartbeat)
	defer heartbeat.Stop()
	lifetime := time.NewTimer(MaxStreamLifetime)
	defer lifetime.Stop()
	poll := time.NewTimer(0)
	defer poll.Stop()
	for {
		select {
		case <-poll.C:
			events, eventErr := h.runs.RunEvents(r.Context(), snapshot.RunID, after, 512)
			if eventErr != nil {
				return true
			}
			for _, event := range events {
				projected := durableOperationEvent(event)
				if err := writeSSEEvent(w, projected); err != nil {
					return true
				}
				after = event.Sequence
			}
			flusher.Flush()
			snapshot, eventErr = h.runs.Run(r.Context(), snapshot.RunID)
			if eventErr != nil || snapshot.Lifecycle.IsTerminal() && after >= snapshot.LastSequence {
				return true
			}
			wait := time.Second
			if len(events) == 512 || after < snapshot.LastSequence {
				wait = 250 * time.Millisecond
			}
			poll.Reset(wait)
		case <-heartbeat.C:
			if _, err := io.WriteString(w, ": heartbeat\n\n"); err != nil {
				return true
			}
			flusher.Flush()
		case <-lifetime.C:
			return true
		case <-r.Context().Done():
			return true
		}
	}
}

func durableOperationEvent(event app.RunEvent) operationEvent {
	name := "progress"
	switch string(event.Type) {
	case "accepted", "claimed", "lifecycle":
		name = "snapshot"
	case "warning", "finding", "artifact", "terminal":
		name = string(event.Type)
	case "cancellation":
		name = "cancel_requested"
	case "recovery", "omission":
		name = "recovery_required"
	}
	payload := map[string]any{"stage": event.Stage, "task": event.Task, "payload": event.Payload, "omission": event.Omission}
	body, _ := json.Marshal(map[string]any{
		"operation_id": string(event.RunID), "time": event.CommittedAt.UTC().Format(time.RFC3339Nano),
		"sequence": event.Sequence, "payload": payload,
	})
	return operationEvent{ID: event.Sequence, Name: name, Data: body}
}

func operationSpecFromForm(r *http.Request) operationSpecRequest {
	parallelism, _ := strconv.Atoi(r.FormValue("parallelism"))
	sprintRef := r.FormValue("sprint")
	if sprintRef == "" {
		sprintRef = newSprintRef(r.FormValue("sprint_number"), r.FormValue("sprint_name"))
	}
	return operationSpecRequest{
		Kind:  r.FormValue("kind"),
		Scope: operationScopeRequest{Project: r.FormValue("project"), Sprint: sprintRef, Study: r.FormValue("study")},
		Options: operationOptionsRequest{
			ToStage:     r.FormValue("stage"),
			Task:        r.FormValue("task"),
			Shard:       r.FormValue("shard"),
			Model:       strings.TrimSpace(r.FormValue("model")),
			Suite:       strings.TrimSpace(r.FormValue("suite")),
			Parallelism: parallelism,
		},
	}
}

func newSprintRef(number, name string) string {
	number = strings.TrimSpace(number)
	name = strings.TrimSpace(name)
	if number == "" || name == "" {
		return ""
	}
	for _, r := range number {
		if r < '0' || r > '9' {
			return number + "-" + name
		}
	}
	var slug strings.Builder
	separator := false
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			if separator && slug.Len() > 0 {
				slug.WriteByte('-')
			}
			slug.WriteRune(r)
			separator = false
		case r == ' ' || r == '-' || r == '_':
			separator = true
		default:
			return number + "-" + name
		}
	}
	if slug.Len() == 0 {
		return ""
	}
	return number + "-" + slug.String()
}

func writeSSEEvent(w io.Writer, event operationEvent) error {
	_, err := fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", event.ID, event.Name, event.Data)
	return err
}

func decodeStrictJSON(r *http.Request, target any) error {
	contentType := r.Header.Get("Content-Type")
	if !strings.HasPrefix(strings.ToLower(contentType), "application/json") {
		return fmt.Errorf("Content-Type must be application/json")
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("invalid JSON request: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return fmt.Errorf("request must contain one JSON value")
	}
	return nil
}

func mapOperationRequest(spec operationSpecRequest) (app.OperationRequest, error) {
	if !validOptionalIdentifier(spec.Scope.Project) || !validOptionalIdentifier(spec.Scope.Sprint) || !validOptionalIdentifier(spec.Scope.Study) {
		return app.OperationRequest{}, fmt.Errorf("operation scope contains an invalid identifier")
	}
	options := spec.Options
	req := app.OperationRequest{
		Project: spec.Scope.Project, Sprint: spec.Scope.Sprint, Study: spec.Scope.Study,
		Stage: firstString(options.ToStage, options.Stage), Task: options.Task, Model: options.Model,
		Level: options.Level, Suite: options.Suite, Test: options.Test, Timeout: options.Timeout,
		ForceReview: options.ForceReview, RestartReview: options.RestartReview, OverrideRationale: options.OverrideRationale,
		ReviewFocus: options.ReviewFocus, Sources: options.Sources, Dimensions: options.Dimensions, Parallelism: options.Parallelism,
	}
	kind := strings.ReplaceAll(strings.TrimSpace(spec.Kind), "_", "-")
	switch kind {
	case "validation", "validate":
		req.Kind = app.OperationValidate
	case "sprint-status":
		req.Kind = app.OperationSprintStatus
	case "prompt-preview", "sprint-prompt":
		req.Kind = app.OperationPrompt
	case "dry-run", "sprint-flow-dry-run":
		req.Kind = app.OperationFlowDryRun
	case "sprint-flow":
		if options.DryRun {
			req.Kind = app.OperationFlowDryRun
		} else {
			req.Kind = app.OperationFlow
		}
	case "sprint-stage":
		if options.DryRun {
			req.Kind = app.OperationStageDryRun
		} else {
			req.Kind = app.OperationStage
		}
	case "sprint-stage-dry-run":
		req.Kind = app.OperationStageDryRun
	case "execute", "execute-start":
		switch {
		case options.Action == "status":
			req.Kind = app.OperationExecuteStatus
		case options.DryRun:
			req.Kind = app.OperationExecuteDryRun
		case options.Resume:
			req.Kind = app.OperationExecuteResume
		default:
			req.Kind = app.OperationExecuteStart
		}
	case "execute-status":
		req.Kind = app.OperationExecuteStatus
	case "execute-dry-run":
		req.Kind = app.OperationExecuteDryRun
	case "execute-resume":
		req.Kind = app.OperationExecuteResume
	case "review", "review-start":
		if options.Action == "status" {
			req.Kind = app.OperationReviewStatus
		} else if options.DryRun {
			req.Kind = app.OperationReviewDryRun
		} else {
			req.Kind = app.OperationReviewStart
		}
	case "review-status":
		req.Kind = app.OperationReviewStatus
	case "review-dry-run":
		req.Kind = app.OperationReviewDryRun
	case "smoke", "smoke-start":
		if options.Action == "status" {
			req.Kind = app.OperationSmokeStatus
		} else if options.DryRun {
			req.Kind = app.OperationSmokeDryRun
		} else {
			req.Kind = app.OperationSmokeStart
		}
	case "smoke-status":
		req.Kind = app.OperationSmokeStatus
	case "smoke-dry-run":
		req.Kind = app.OperationSmokeDryRun
	case "verify", "verify-start":
		if options.DryRun {
			req.Kind = app.OperationVerifyDryRun
		} else {
			req.Kind = app.OperationVerifyStart
		}
	case "verify-dry-run":
		req.Kind = app.OperationVerifyDryRun
	case "qa-status":
		req.Kind = app.OperationQAStatus
	case "qa-dry-run":
		req.Kind = app.OperationQADryRun
	case "qa-start":
		req.Kind = app.OperationQAStart
		req.Task = options.Shard
	case "qa-resume":
		req.Kind = app.OperationQAResume
		req.Task = options.Shard
	case "qa-recover":
		req.Kind = app.OperationQARecover
	case "study-run-loop", "study-start":
		if options.Resume {
			req.Kind = app.OperationStudyResume
		} else {
			req.Kind = app.OperationStudyStart
		}
	case "study-resume":
		req.Kind = app.OperationStudyResume
	case "study-cancel":
		req.Kind = app.OperationStudyCancel
	default:
		return app.OperationRequest{}, fmt.Errorf("unsupported operation kind %q", spec.Kind)
	}
	if req.Kind == app.OperationValidate {
		if (req.Project == "") == (req.Study == "") {
			return app.OperationRequest{}, fmt.Errorf("validation requires one project or study scope")
		}
	} else if strings.HasPrefix(string(req.Kind), "study-") {
		if req.Study == "" || req.Project != "" || req.Sprint != "" {
			return app.OperationRequest{}, fmt.Errorf("study operations require only scope.study")
		}
	} else if req.Project == "" {
		return app.OperationRequest{}, fmt.Errorf("project operations require scope.project")
	} else if req.Sprint == "" {
		return app.OperationRequest{}, fmt.Errorf("sprint operations require scope.sprint")
	}
	if strings.HasPrefix(string(req.Kind), "qa-") {
		if options.Task != "" || options.Model != "" || options.Stage != "" || options.ToStage != "" || options.Action != "" || options.DryRun || options.Resume || options.Level != "" || options.Test != "" || options.Timeout != "" || options.ForceReview || options.RestartReview || options.OverrideRationale != "" || len(options.ReviewFocus) > 0 || len(options.Sources) > 0 || len(options.Dimensions) > 0 || options.Parallelism != 0 {
			return app.OperationRequest{}, fmt.Errorf("QA operations accept only options.shard or options.suite")
		}
		if options.Suite != "" && options.Suite != "smoke" {
			return app.OperationRequest{}, fmt.Errorf("QA options.suite must be smoke")
		}
		if options.Suite != "" && req.Kind != app.OperationQAStart && req.Kind != app.OperationQADryRun {
			return app.OperationRequest{}, fmt.Errorf("QA suite is valid only for start or dry-run")
		}
		if options.Suite != "" && options.Shard != "" {
			return app.OperationRequest{}, fmt.Errorf("QA suite and shard are mutually exclusive")
		}
		req.Suite = options.Suite
		if options.Shard != "" && req.Kind != app.OperationQAStart && req.Kind != app.OperationQAResume {
			return app.OperationRequest{}, fmt.Errorf("QA shard is valid only for start or resume")
		}
	}
	return req, nil
}

func mapOperationSpec(req app.OperationRequest) map[string]any {
	scope := map[string]any{}
	if req.Project != "" {
		scope["project"] = req.Project
	}
	if req.Sprint != "" {
		scope["sprint"] = req.Sprint
	}
	if req.Study != "" {
		scope["study"] = req.Study
	}
	options := map[string]any{}
	if req.Stage != "" {
		options["stage"] = req.Stage
	}
	if req.Task != "" {
		if req.Kind == app.OperationQAStart || req.Kind == app.OperationQAResume {
			options["shard"] = req.Task
		} else {
			options["task"] = req.Task
		}
	}
	if req.Model != "" {
		options["model"] = req.Model
	}
	if req.Parallelism > 0 {
		options["parallelism"] = req.Parallelism
	}
	if req.Suite != "" {
		options["suite"] = req.Suite
	}
	return map[string]any{"kind": req.Kind, "scope": scope, "options": options}
}

func validOptionalIdentifier(value string) bool {
	return value == "" || validIdentifier(value)
}

func firstString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func (h *handler) writeOperationError(w http.ResponseWriter, r *http.Request, err error) {
	status, code, message, retryable := http.StatusBadRequest, "invalid_request", "The operation request is invalid.", false
	details := map[string]any{}
	switch {
	case errors.Is(err, errConfirmationExpired):
		status, code, message, retryable = http.StatusConflict, "confirmation_expired", "The confirmation expired. Prepare the operation again.", true
	case errors.Is(err, errConfirmationMismatch):
		status, code, message = http.StatusConflict, "confirmation_mismatch", "The confirmation does not match this session or operation."
	case errors.Is(err, errConfirmationReplayed):
		status, code, message = http.StatusConflict, "confirmation_replayed", "The confirmation was already used or is unknown."
	case errors.Is(err, errStaleConfirmation), errors.Is(err, app.ErrStaleOperation):
		status, code, message, retryable = http.StatusConflict, "stale_confirmation", "The operation inputs changed after confirmation. Prepare it again.", true
	case errors.Is(err, errOperationNotFound):
		status, code, message = http.StatusNotFound, "operation_not_found", "The operation is no longer retained. Refresh durable status."
		details["action"] = "refresh_durable_status"
	case errors.Is(err, errOperationCapacity):
		status, code, message, retryable = http.StatusTooManyRequests, "operation_capacity", "Operation capacity is currently full.", true
		w.Header().Set("Retry-After", "2")
	case errors.Is(err, errSubscriberCapacity):
		status, code, message, retryable = http.StatusTooManyRequests, "subscriber_capacity", "Stream capacity is currently full.", true
		w.Header().Set("Retry-After", "2")
	case errors.Is(err, errServerDraining):
		status, code, message, retryable = http.StatusServiceUnavailable, "server_draining", "The server is shutting down and is not accepting new work.", true
		w.Header().Set("Retry-After", "2")
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		status, code, message, retryable = http.StatusServiceUnavailable, "server_draining", "The server is shutting down.", true
	case strings.Contains(strings.ToLower(err.Error()), "validation"), strings.Contains(strings.ToLower(err.Error()), "incomplete"), strings.Contains(strings.ToLower(err.Error()), "prerequisite"):
		status, code, message = http.StatusUnprocessableEntity, "validation_failed", "The governed inputs failed validation."
	case strings.Contains(strings.ToLower(err.Error()), "lock"), strings.Contains(strings.ToLower(err.Error()), "in progress"):
		status, code, message, retryable = http.StatusConflict, "operation_conflict", "A conflicting operation is already running.", true
	case strings.Contains(strings.ToLower(err.Error()), "unavailable"):
		status, code, message, retryable = http.StatusFailedDependency, "prerequisite_unavailable", "A required operation prerequisite is unavailable.", true
	case !isClientOperationError(err):
		status, code, message = http.StatusInternalServerError, "internal_failure", "The operation could not be completed."
	}
	if cause := safeOperationCause(err); cause != "" && status < http.StatusInternalServerError {
		details["reason"] = cause
		details["guidance"] = operationFailureGuidance(code)
	}
	h.writeErrorDetails(w, r, status, errorBody{Code: code, Message: message, Retryable: retryable, Details: details})
}

func safeOperationCause(err error) string {
	if err == nil {
		return ""
	}
	text := strings.ToLower(err.Error())
	if isClientOperationError(err) || strings.Contains(text, "validation") || strings.Contains(text, "incomplete") || strings.Contains(text, "prerequisite") || strings.Contains(text, "lock") || strings.Contains(text, "in progress") || strings.Contains(text, "unavailable") {
		return safeWebText(err.Error())
	}
	return ""
}

func operationFailureGuidance(code string) string {
	switch code {
	case "validation_failed":
		return "Open the sprint Plan and Delivery pages, correct the reported incomplete or invalid evidence, then retry."
	case "operation_conflict":
		return "Wait for the active operation to finish or inspect and cancel it before retrying."
	case "prerequisite_unavailable":
		return "Restore the missing prerequisite or runtime capability, then prepare the operation again."
	default:
		return "Correct the reported reason and prepare the operation again."
	}
}

func htmlOperationFailure(summary string, err error) string {
	cause := safeOperationCause(err)
	if cause == "" {
		return summary
	}
	return summary + " Reason: " + cause + " Correct the reported prerequisite or governed artifact, then retry."
}

func isClientOperationError(err error) bool {
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "required") || strings.Contains(text, "invalid") || strings.Contains(text, "unsupported") || strings.Contains(text, "json") || strings.Contains(text, "content-type") || strings.Contains(text, "scope")
}

func (h *handler) writeErrorDetails(w http.ResponseWriter, r *http.Request, status int, body errorBody) {
	meta := responseMeta{APIVersion: "v1", RequestID: requestID(r.Context()), GeneratedAt: h.now().UTC().Format(time.RFC3339Nano)}
	writeJSON(w, status, errorEnvelope{Error: body, Meta: meta})
}

func eventIDHeader(value string) string {
	if value == "" {
		return "0"
	}
	if _, err := strconv.ParseUint(value, 10, 64); err != nil {
		return "0"
	}
	return value
}

func (h *handler) logOperation(r *http.Request, event, operationID, kind, state, reason string) {
	fmt.Fprintf(h.diagnostics,
		"event=%s request_id=%s operation_id=%s kind=%s state=%s reason=%s\n",
		safeWebText(event), safeWebText(requestID(r.Context())), safeWebText(operationID), safeWebText(kind), safeWebText(state), safeWebText(reason))
}
