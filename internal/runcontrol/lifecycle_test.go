package runcontrol

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type mutableClock struct{ at time.Time }

func (c *mutableClock) Now() time.Time { return c.at }

type staticProcessProbe struct {
	identity ProcessIdentity
	err      error
	seenPID  int
}

func (p *staticProcessProbe) Probe(_ context.Context, pid int) (ProcessIdentity, error) {
	p.seenPID = pid
	return p.identity, p.err
}

func TestHeartbeatAndDurableCancellationAreFencedAndIdempotent(t *testing.T) {
	ctx := context.Background()
	clock := &mutableClock{at: time.Date(2026, 8, 21, 10, 0, 0, 0, time.UTC)}
	repository, err := OpenSQLite(ctx, t.TempDir(), SQLiteOptions{Clock: clock})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })
	owner := Owner{ID: "owner-one", Process: ProcessIdentity{HostDigest: "host", BootID: "boot", PID: 41, BirthToken: "birth"}}
	snapshot, err := repository.Accept(ctx, Acceptance{Target: Target{Kind: "sprint", Operation: "execute"}})
	if err != nil {
		t.Fatal(err)
	}
	attempt, _, err := repository.Claim(ctx, Claim{RunID: snapshot.RunID, Owner: owner, Lease: OwnerLeaseDuration})
	if err != nil {
		t.Fatal(err)
	}
	fence := Fence{RunID: snapshot.RunID, AttemptID: attempt.ID, OwnerID: owner.ID, FencingGeneration: attempt.FencingGeneration}

	clock.at = clock.at.Add(HeartbeatInterval)
	heartbeat, err := repository.Heartbeat(ctx, fence, OwnerLeaseDuration)
	if err != nil {
		t.Fatal(err)
	}
	if heartbeat.Liveness != LivenessLive || heartbeat.Lifecycle != LifecycleRunning {
		t.Fatalf("heartbeat snapshot = %+v", heartbeat)
	}
	requested, changed, err := repository.RequestCancellation(ctx, snapshot.RunID, "user_requested")
	if err != nil || !changed || requested.Cancellation.State != CancellationRequested || requested.Lifecycle != LifecycleCancelling {
		t.Fatalf("request snapshot=%+v changed=%v err=%v", requested, changed, err)
	}
	duplicate, changed, err := repository.RequestCancellation(ctx, snapshot.RunID, "user_requested")
	if err != nil || changed || duplicate.LastSequence != requested.LastSequence {
		t.Fatalf("duplicate snapshot=%+v changed=%v err=%v", duplicate, changed, err)
	}
	acknowledged, changed, err := repository.AcknowledgeCancellation(ctx, fence)
	if err != nil || !changed || acknowledged.Cancellation.State != CancellationAcknowledged {
		t.Fatalf("ack snapshot=%+v changed=%v err=%v", acknowledged, changed, err)
	}
	acknowledgedAgain, changed, err := repository.AcknowledgeCancellation(ctx, fence)
	if err != nil || changed || acknowledgedAgain.LastSequence != acknowledged.LastSequence {
		t.Fatalf("duplicate ack snapshot=%+v changed=%v err=%v", acknowledgedAgain, changed, err)
	}

	stale := fence
	stale.OwnerID = "wrong-owner"
	if _, err := repository.Heartbeat(ctx, stale, OwnerLeaseDuration); !errors.Is(err, ErrStaleFence) {
		t.Fatalf("stale heartbeat error = %v", err)
	}
	if _, _, err := repository.AcknowledgeCancellation(ctx, stale); !errors.Is(err, ErrStaleFence) {
		t.Fatalf("stale cancellation acknowledgement error = %v", err)
	}
}

func TestCompletionMayWinAfterCancellationAndTerminalCancelIsIdempotent(t *testing.T) {
	ctx := context.Background()
	repository, err := OpenSQLite(ctx, t.TempDir(), SQLiteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	runID, fence := acceptedClaimedRun(t, repository)
	requested, changed, err := repository.RequestCancellation(ctx, runID, "user_requested")
	if err != nil || !changed || requested.Cancellation.State != CancellationRequested {
		t.Fatalf("request cancellation: changed=%t snapshot=%+v err=%v", changed, requested, err)
	}
	winner, won, err := repository.ProposeTerminal(ctx, fence, TerminalProposal{Outcome: TerminalSucceeded, Reason: "completion already committed", ProposedBy: "owner-test"})
	if err != nil || !won || winner.Lifecycle != LifecycleSucceeded || winner.Cancellation.State != CancellationRequested {
		t.Fatalf("completion winner: won=%t snapshot=%+v err=%v", won, winner, err)
	}
	again, changed, err := repository.RequestCancellation(ctx, runID, "user_requested")
	if err != nil || changed || again.Lifecycle != LifecycleSucceeded || again.LastSequence != winner.LastSequence {
		t.Fatalf("terminal cancellation retry rewrote winner: changed=%t snapshot=%+v err=%v", changed, again, err)
	}
}

func TestEveryTerminalOutcomeUsesOneImmutableArbitrationPath(t *testing.T) {
	outcomes := []TerminalOutcome{TerminalSucceeded, TerminalFailed, TerminalCancelled, TerminalTimedOut, TerminalInterrupted, TerminalCleanupUncertain, TerminalPersistenceLost}
	for _, outcome := range outcomes {
		t.Run(string(outcome), func(t *testing.T) {
			repository, fence := openClaimedRepository(t)
			winner, won, err := repository.ProposeTerminal(context.Background(), fence, TerminalProposal{Outcome: outcome, Reason: "table-driven terminal", ProposedBy: "test"})
			if err != nil || !won || winner.Lifecycle != outcome.Lifecycle() || winner.Terminal == nil || winner.Terminal.Outcome != outcome {
				t.Fatalf("winner=%+v won=%t err=%v", winner, won, err)
			}
			unchanged, wonAgain, err := repository.ProposeTerminal(context.Background(), fence, TerminalProposal{Outcome: TerminalFailed, Reason: "late loser", ProposedBy: "other"})
			if err != nil || wonAgain || unchanged.Terminal == nil || unchanged.Terminal.Outcome != outcome || unchanged.LastSequence != winner.LastSequence {
				t.Fatalf("immutable winner=%+v won=%t err=%v", unchanged, wonAgain, err)
			}
		})
	}
}

func TestCancellationAndTerminalRacePreservesOneWinnerAndIdempotentCommand(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	ownerRepository, err := OpenSQLite(ctx, root, SQLiteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer ownerRepository.Close()
	observerRepository, err := OpenSQLite(ctx, root, SQLiteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer observerRepository.Close()
	snapshot, err := ownerRepository.Accept(ctx, Acceptance{Target: Target{Kind: "race", Operation: "cancel-terminal"}})
	if err != nil {
		t.Fatal(err)
	}
	owner := Owner{ID: "race-owner", Process: ProcessIdentity{PID: 1}}
	attempt, _, err := ownerRepository.Claim(ctx, Claim{RunID: snapshot.RunID, Owner: owner, Lease: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	fence := Fence{RunID: snapshot.RunID, AttemptID: attempt.ID, OwnerID: owner.ID, FencingGeneration: attempt.FencingGeneration}
	start := make(chan struct{})
	var wait sync.WaitGroup
	wait.Add(2)
	errs := make(chan error, 2)
	go func() {
		defer wait.Done()
		<-start
		_, _, raceErr := observerRepository.RequestCancellation(ctx, snapshot.RunID, "user_requested")
		errs <- raceErr
	}()
	go func() {
		defer wait.Done()
		<-start
		_, _, raceErr := ownerRepository.ProposeTerminal(ctx, fence, TerminalProposal{Outcome: TerminalSucceeded, Reason: "completed concurrently", ProposedBy: owner.ID})
		errs <- raceErr
	}()
	close(start)
	wait.Wait()
	close(errs)
	for raceErr := range errs {
		if raceErr != nil {
			t.Fatal(raceErr)
		}
	}
	final, err := observerRepository.Snapshot(ctx, snapshot.RunID)
	if err != nil || final.Terminal == nil || final.Terminal.Outcome != TerminalSucceeded || final.Lifecycle != LifecycleSucceeded {
		t.Fatalf("final snapshot=%+v err=%v", final, err)
	}
	again, changed, err := observerRepository.RequestCancellation(ctx, snapshot.RunID, "user_requested")
	if err != nil || changed || again.LastSequence != final.LastSequence || again.Terminal == nil || again.Terminal.Outcome != TerminalSucceeded {
		t.Fatalf("terminal cancel retry snapshot=%+v changed=%t err=%v", again, changed, err)
	}
}

func TestReconcileUsesExactProcessBirthAndNeverInfersSuccess(t *testing.T) {
	tests := []struct {
		name          string
		probeIdentity ProcessIdentity
		probeErr      error
		wantLifecycle Lifecycle
		wantLiveness  Liveness
	}{
		{name: "exact live process is stalled", probeIdentity: ProcessIdentity{HostDigest: "host", BootID: "boot", PID: 42, BirthToken: "birth"}, wantLifecycle: LifecycleRunning, wantLiveness: LivenessStalled},
		{name: "missing process is interrupted", probeErr: ErrProcessNotFound, wantLifecycle: LifecycleInterrupted, wantLiveness: LivenessTerminal},
		{name: "pid reuse is interrupted", probeIdentity: ProcessIdentity{HostDigest: "host", BootID: "boot", PID: 42, BirthToken: "different"}, wantLifecycle: LifecycleInterrupted, wantLiveness: LivenessTerminal},
		{name: "unsupported exact probe is uncertain", probeErr: ErrProcessIdentityUnavailable, wantLifecycle: LifecycleCleanupUncertain, wantLiveness: LivenessTerminal},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			clock := &mutableClock{at: time.Date(2026, 8, 21, 10, 0, 0, 0, time.UTC)}
			repository, err := OpenSQLite(ctx, t.TempDir(), SQLiteOptions{Clock: clock})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = repository.Close() })
			expected := ProcessIdentity{HostDigest: "host", BootID: "boot", PID: 42, BirthToken: "birth"}
			snapshot, err := repository.Accept(ctx, Acceptance{Target: Target{Kind: "study", Operation: "analysis"}})
			if err != nil {
				t.Fatal(err)
			}
			if _, _, err := repository.Claim(ctx, Claim{RunID: snapshot.RunID, Owner: Owner{ID: "owner", Process: expected}, Lease: OwnerLeaseDuration}); err != nil {
				t.Fatal(err)
			}
			clock.at = clock.at.Add(OwnerLeaseDuration + ReconciliationGrace + time.Second)
			probe := &staticProcessProbe{identity: tt.probeIdentity, err: tt.probeErr}
			report, err := repository.Reconcile(ctx, probe, ReconcileOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if report.Scanned != 1 || probe.seenPID != expected.PID {
				t.Fatalf("report=%+v seen_pid=%d", report, probe.seenPID)
			}
			evidence, err := repository.ReconciliationEvidence(ctx, 10)
			if err != nil {
				t.Fatal(err)
			}
			if len(evidence) != 1 || evidence[0].RunID != snapshot.RunID || evidence[0].Evidence != "safe_identity_only" || evidence[0].Decision == "" {
				t.Fatalf("reconciliation evidence = %+v", evidence)
			}
			got, err := repository.Snapshot(ctx, snapshot.RunID)
			if err != nil {
				t.Fatal(err)
			}
			if got.Lifecycle != tt.wantLifecycle || got.Liveness != tt.wantLiveness || got.Lifecycle == LifecycleSucceeded {
				t.Fatalf("reconciled snapshot = %+v", got)
			}
		})
	}
}

func TestReconcileClockJumpNeverExpiresAnOwnerEarly(t *testing.T) {
	ctx := context.Background()
	clock := &mutableClock{at: time.Date(2026, 8, 21, 10, 0, 0, 0, time.UTC)}
	repository, err := OpenSQLite(ctx, t.TempDir(), SQLiteOptions{Clock: clock})
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	snapshot, err := repository.Accept(ctx, Acceptance{Target: Target{Kind: "test", Operation: "clock-jump"}})
	if err != nil {
		t.Fatal(err)
	}
	identity := ProcessIdentity{HostDigest: "host", BootID: "boot", PID: 73, BirthToken: "birth"}
	if _, _, err := repository.Claim(ctx, Claim{RunID: snapshot.RunID, Owner: Owner{ID: "owner", Process: identity}, Lease: OwnerLeaseDuration}); err != nil {
		t.Fatal(err)
	}
	clock.at = clock.at.Add(-time.Hour)
	report, err := repository.Reconcile(ctx, &staticProcessProbe{identity: identity}, ReconcileOptions{})
	if err != nil || report.Scanned != 0 {
		t.Fatalf("backward jump reconciled live lease early: report=%+v err=%v", report, err)
	}
	clock.at = clock.at.Add(2 * time.Hour)
	report, err = repository.Reconcile(ctx, &staticProcessProbe{err: ErrProcessNotFound}, ReconcileOptions{})
	if err != nil || report.Terminal != 1 {
		t.Fatalf("forward jump did not conservatively reconcile expired owner: report=%+v err=%v", report, err)
	}
}

func TestReconcileTerminalizesAcceptanceThatWasNeverClaimedAfterGrace(t *testing.T) {
	ctx := context.Background()
	clock := &mutableClock{at: time.Date(2026, 8, 21, 10, 0, 0, 0, time.UTC)}
	repository, err := OpenSQLite(ctx, t.TempDir(), SQLiteOptions{Clock: clock})
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	snapshot, err := repository.Accept(ctx, Acceptance{Target: Target{Kind: "sprint", Operation: "execute"}})
	if err != nil {
		t.Fatal(err)
	}
	health, err := repository.Health(ctx)
	if err != nil || health.ReconciliationBacklog != 0 {
		t.Fatalf("fresh acceptance health=%+v err=%v", health, err)
	}
	clock.at = clock.at.Add(ReconciliationGrace + time.Second)
	health, err = repository.Health(ctx)
	if err != nil || health.ReconciliationBacklog != 1 || health.OldestBacklogAge < ReconciliationGrace {
		t.Fatalf("expired acceptance health=%+v err=%v", health, err)
	}
	report, err := repository.Reconcile(ctx, &staticProcessProbe{err: errors.New("must not probe an unclaimed run")}, ReconcileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if report.Scanned != 1 || report.Terminal != 1 || len(report.Decisions) != 1 || report.Decisions[0].Decision != "owner_never_claimed_after_grace" {
		t.Fatalf("reconciliation report = %+v", report)
	}
	got, err := repository.Snapshot(ctx, snapshot.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Lifecycle != LifecycleInterrupted || got.Liveness != LivenessTerminal || got.CurrentAttemptID != "" || got.Terminal == nil || got.Terminal.Reason != "owner_never_claimed_after_grace" {
		t.Fatalf("reconciled unclaimed snapshot = %+v", got)
	}
	health, err = repository.Health(ctx)
	if err != nil || health.ReconciliationBacklog != 0 || health.Metrics.Terminal.Count != 1 {
		t.Fatalf("reconciled health=%+v err=%v", health, err)
	}
	events, err := repository.Events(ctx, snapshot.RunID, 0, 10)
	if err != nil || len(events) != 1 || events[0].Type != EventTerminal || events[0].AttemptID != "" {
		t.Fatalf("unclaimed events=%+v err=%v", events, err)
	}
	if _, _, err := repository.Claim(ctx, Claim{RunID: snapshot.RunID, Owner: Owner{ID: "late-owner", Process: ProcessIdentity{PID: 1}}, Lease: time.Minute}); !errors.Is(err, ErrTerminal) {
		t.Fatalf("late claim error=%v, want terminal", err)
	}
	evidence, err := repository.ReconciliationEvidence(ctx, 10)
	if err != nil || len(evidence) != 1 || evidence[0].AttemptID != "" || evidence[0].Evidence != "acceptance_timestamp_only" {
		t.Fatalf("unclaimed reconciliation evidence=%+v err=%v", evidence, err)
	}
}

func TestNativeProcessProbeReturnsExactCurrentLinuxIdentity(t *testing.T) {
	owner, err := NewProcessOwner()
	if err != nil {
		t.Fatal(err)
	}
	if owner.ID == "" || owner.Process.PID <= 0 {
		t.Fatalf("owner = %+v", owner)
	}
	observed, err := (NativeProcessProbe{}).Probe(context.Background(), owner.Process.PID)
	if err != nil {
		if errors.Is(err, ErrProcessIdentityUnavailable) {
			t.Skip("platform does not expose exact process birth identity")
		}
		t.Fatal(err)
	}
	if observed.HostDigest == "" || observed.BootID == "" || observed.BirthToken == "" || observed != owner.Process {
		t.Fatalf("owner=%+v observed=%+v", owner.Process, observed)
	}
}
