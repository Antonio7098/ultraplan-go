package runcontrol

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"time"
)

var allowedEventPayloadFields = map[string]struct{}{
	"runtime_event_id": {}, "runtime_run_id": {}, "agentwrap_run_id": {}, "session_id": {},
	"external_harness_run_id": {}, "type": {}, "kind": {}, "state": {}, "status": {},
	"outcome": {}, "reason": {}, "category": {}, "code": {}, "count": {}, "known": {},
	"input_tokens": {}, "output_tokens": {}, "total_tokens": {}, "turns": {},
	"artifact_kind": {}, "artifact_id": {}, "lifecycle": {}, "liveness": {},
	"message": {}, "index": {}, "scope": {}, "tool": {}, "action": {}, "detail": {},
	"tool_call_id": {}, "tool_name": {}, "tool_status": {}, "tool_arguments": {},
	"tool_result": {}, "tool_error": {},
}

// sanitizeEventDraft is the final storage gate. Producers may only submit
// bounded product-owned fields; raw payloads, prompts, arbitrary diagnostics,
// credentials, and unrestricted paths are replaced with explicit omission.
func sanitizeEventDraft(input EventDraft) EventDraft {
	out := EventDraft{
		Type: input.Type, Scope: input.Scope, Stage: safeEventValue(input.Stage, MaxTargetFieldBytes),
		Task: safeEventValue(input.Task, MaxTargetFieldBytes), Kind: safeEventValue(input.Kind, MaxSafeValueBytes),
		Tool: safeEventValue(input.Tool, MaxSafeValueBytes), Action: safeEventValue(input.Action, MaxSafeValueBytes),
		Reason: safeEventValue(input.Reason, MaxSafeValueBytes), Detail: safeEventValue(input.Detail, MaxSafeValueBytes), Lifecycle: input.Lifecycle,
		Payload: make(map[string]string), Omission: cloneOmission(input.Omission),
	}
	omitted := uint64(0)
	for key, value := range input.Payload {
		if _, ok := allowedEventPayloadFields[key]; !ok || sensitiveEventField(key) || unsafeEventValue(value) {
			omitted++
			continue
		}
		if len(value) > MaxSafeValueBytes {
			omitted++
			continue
		}
		out.Payload[key] = safeEventValue(value, MaxSafeValueBytes)
	}
	if out.Stage != input.Stage || out.Task != input.Task {
		omitted++
	}
	if omitted > 0 {
		mergeOmission(&out, "unsafe event detail omitted", omitted, nil, nil)
	}
	encoded, err := json.Marshal(out)
	if err != nil || len(encoded) > MaxEncodedEventBytes {
		return EventDraft{
			Type:     EventWarning,
			Payload:  map[string]string{"code": "event_detail_oversize", "reason": "event detail exceeded durable limit"},
			Omission: &Omission{Reason: "oversize event detail omitted", Count: max(omitted, 1)},
		}
	}
	return out
}

func mergeOmission(draft *EventDraft, reason string, count uint64, first, last *time.Time) {
	if draft.Omission == nil {
		draft.Omission = &Omission{Reason: reason, Count: count, FirstAt: first, LastAt: last}
		return
	}
	draft.Omission.Count += count
	if draft.Omission.Reason == "" {
		draft.Omission.Reason = reason
	}
	if draft.Omission.FirstAt == nil {
		draft.Omission.FirstAt = first
	}
	if last != nil {
		draft.Omission.LastAt = last
	}
}

func sensitiveEventField(key string) bool {
	key = strings.ToLower(key)
	for _, fragment := range []string{"secret", "token", "credential", "password", "prompt", "payload", "stdout", "stderr", "path", "command"} {
		if strings.Contains(key, fragment) {
			return true
		}
	}
	return false
}

func unsafeEventValue(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	if strings.ContainsAny(value, "\x00\r\n") || filepath.IsAbs(value) {
		return true
	}
	if len(value) >= 3 && value[1] == ':' && (value[2] == '\\' || value[2] == '/') {
		return true
	}
	for _, marker := range []string{"bearer ", "sk-", "ghp_", "github_pat_", "-----begin private key"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func safeEventValue(value string, limit int) string {
	value = strings.Map(func(r rune) rune {
		if r == 0 || r == '\r' || r == '\n' {
			return -1
		}
		return r
	}, strings.TrimSpace(value))
	if len(value) > limit {
		value = value[:limit]
	}
	return value
}
