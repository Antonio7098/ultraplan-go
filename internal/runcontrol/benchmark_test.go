package runcontrol

import (
	"context"
	"testing"
	"time"
)

func BenchmarkSQLiteAppendCommittedEvent(b *testing.B) {
	ctx := context.Background()
	repository, err := OpenSQLite(ctx, b.TempDir(), SQLiteOptions{})
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = repository.Close() })
	run, err := repository.Accept(ctx, Acceptance{Target: Target{Kind: "benchmark", Operation: "append"}})
	if err != nil {
		b.Fatal(err)
	}
	owner := Owner{ID: "benchmark-owner"}
	attempt, _, err := repository.Claim(ctx, Claim{RunID: run.RunID, Owner: owner, Lease: time.Hour})
	if err != nil {
		b.Fatal(err)
	}
	fence := Fence{RunID: run.RunID, AttemptID: attempt.ID, OwnerID: owner.ID, FencingGeneration: attempt.FencingGeneration}
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		if _, _, err := repository.Append(ctx, fence, EventDraft{Type: EventMessage, Payload: map[string]string{"state": "benchmark"}}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSQLiteReplayPage(b *testing.B) {
	ctx := context.Background()
	repository, err := OpenSQLite(ctx, b.TempDir(), SQLiteOptions{})
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = repository.Close() })
	run, err := repository.Accept(ctx, Acceptance{Target: Target{Kind: "benchmark", Operation: "replay"}})
	if err != nil {
		b.Fatal(err)
	}
	owner := Owner{ID: "benchmark-owner"}
	attempt, _, err := repository.Claim(ctx, Claim{RunID: run.RunID, Owner: owner, Lease: time.Hour})
	if err != nil {
		b.Fatal(err)
	}
	fence := Fence{RunID: run.RunID, AttemptID: attempt.ID, OwnerID: owner.ID, FencingGeneration: attempt.FencingGeneration}
	for index := 0; index < 512; index++ {
		if _, _, err := repository.Append(ctx, fence, EventDraft{Type: EventMessage, Payload: map[string]string{"state": "benchmark"}}); err != nil {
			b.Fatal(err)
		}
	}
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		events, err := repository.Events(ctx, run.RunID, 0, 512)
		if err != nil || len(events) != 512 {
			b.Fatalf("events=%d err=%v", len(events), err)
		}
	}
}
