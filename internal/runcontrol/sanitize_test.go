package runcontrol

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestAppendSanitizesHostileAndRawShapedDetailBeforeStorage(t *testing.T) {
	repository, fence := openClaimedRepository(t)
	_, _, err := repository.Append(context.Background(), fence, EventDraft{
		Type: EventMessage,
		Payload: map[string]string{
			"message":      "safe summary",
			"prompt":       "full private prompt",
			"access_token": "sk-secret-value",
			"path":         "/private/workspace/file",
			"kind":         "Bearer credential",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	events, err := repository.Events(context.Background(), fence.RunID, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Payload["message"] != "safe summary" || events[0].Omission == nil || events[0].Omission.Count != 4 {
		t.Fatalf("sanitized event = %+v", events)
	}
	encoded := events[0].Payload["prompt"] + events[0].Payload["access_token"] + events[0].Payload["path"] + events[0].Payload["kind"]
	if strings.Contains(encoded, "secret") || strings.Contains(encoded, "/private") || strings.Contains(encoded, "prompt") {
		t.Fatalf("unsafe detail persisted: %+v", events[0])
	}
}

func TestAppendReplacesOversizeAllowedDetailWithWarning(t *testing.T) {
	repository, fence := openClaimedRepository(t)
	large := strings.Repeat("a", MaxSafeValueBytes)
	payload := map[string]string{
		"runtime_event_id": large, "runtime_run_id": large, "agentwrap_run_id": large,
		"session_id": large, "external_harness_run_id": large, "type": large,
		"kind": large, "state": large, "status": large,
	}
	event, _, err := repository.Append(context.Background(), fence, EventDraft{Type: EventProgress, Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	if event.Type != EventWarning || event.Payload["code"] != "event_detail_oversize" || event.Omission == nil {
		t.Fatalf("oversize replacement = %+v", event)
	}
}

func openClaimedRepository(t *testing.T) (*SQLiteRepository, Fence) {
	t.Helper()
	ctx := context.Background()
	repository, err := OpenSQLite(ctx, t.TempDir(), SQLiteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })
	snapshot, err := repository.Accept(ctx, Acceptance{Target: Target{Kind: "test", Operation: "journal"}})
	if err != nil {
		t.Fatal(err)
	}
	owner := Owner{ID: "owner", Process: ProcessIdentity{PID: 1}}
	attempt, _, err := repository.Claim(ctx, Claim{RunID: snapshot.RunID, Owner: owner, Lease: 15 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	return repository, Fence{RunID: snapshot.RunID, AttemptID: attempt.ID, OwnerID: owner.ID, FencingGeneration: attempt.FencingGeneration}
}
