package sprint

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Antonio7098/agentwrap"
	pruntime "github.com/Antonio7098/ultraplan-go/internal/platform/runtime"
)

type qaInvestigatorRuntime struct {
	active atomic.Int32
	max    atomic.Int32
	mode   string
	mutate string
	turns  int64
}

type qaRetryRuntime struct {
	qaInvestigatorRuntime
	failures int32
	calls    atomic.Int32
}

type qaOutputRetryRuntime struct {
	qaInvestigatorRuntime
	calls    atomic.Int32
	failures int32
	requests []pruntime.Request
}

type qaCancellationRuntime struct {
	started chan struct{}
}

type qaPanicRuntime struct{}

type qaEmptyTheoryRuntime struct{}

func (qaPanicRuntime) StartRun(context.Context, pruntime.Request) (pruntime.Result, error) {
	panic("runtime adapter panic")
}

func (qaEmptyTheoryRuntime) StartRun(context.Context, pruntime.Request) (pruntime.Result, error) {
	return pruntime.Result{
		Status:         "completed",
		TerminalOutput: `{"schema_version":1,"theories":[],"evidence":[],"context_requests":[],"check_requests":[]}`,
		Permissions:    pruntime.PermissionSummary{Mode: "restricted", Default: "deny"},
		Usage: pruntime.Usage{
			InputTokens: 101, InputTokensKnown: true,
			OutputTokens: 17, OutputTokensKnown: true,
			CacheReadTokens: 73, CacheReadTokensKnown: true,
			Turns: 1, TurnsKnown: true,
		},
		EstimatedCost: &pruntime.CostEstimate{Amount: 0.001, Currency: "USD", Estimate: true},
	}, nil
}

func withTestQAMapFence(service Service, fence func(QAMap) error) Service {
	return service.WithQAMapFence(fence)
}

func (runtime *qaCancellationRuntime) StartRun(ctx context.Context, _ pruntime.Request) (pruntime.Result, error) {
	close(runtime.started)
	<-ctx.Done()
	return pruntime.Result{Permissions: pruntime.PermissionSummary{Mode: "restricted", Default: "deny"}}, ctx.Err()
}

func (runtime *qaRetryRuntime) StartRun(ctx context.Context, req pruntime.Request) (pruntime.Result, error) {
	if runtime.calls.Add(1) <= runtime.failures {
		return pruntime.Result{Permissions: pruntime.PermissionSummary{Mode: "restricted", Default: "deny"}}, errors.New("temporary runtime failure")
	}
	return runtime.qaInvestigatorRuntime.StartRun(ctx, req)
}

func (runtime *qaOutputRetryRuntime) StartRun(ctx context.Context, req pruntime.Request) (pruntime.Result, error) {
	runtime.requests = append(runtime.requests, req)
	if runtime.calls.Add(1) <= runtime.failures {
		return pruntime.Result{SessionID: "qa-session", TerminalOutput: "not json", Permissions: pruntime.PermissionSummary{Mode: "restricted", Default: "deny"}}, nil
	}
	return runtime.qaInvestigatorRuntime.StartRun(ctx, req)
}

func (runtime *qaInvestigatorRuntime) StartRun(_ context.Context, req pruntime.Request) (pruntime.Result, error) {
	active := runtime.active.Add(1)
	for {
		prior := runtime.max.Load()
		if active <= prior || runtime.max.CompareAndSwap(prior, active) {
			break
		}
	}
	time.Sleep(10 * time.Millisecond)
	if runtime.mutate != "" {
		_ = os.WriteFile(filepath.Join(runtime.mutate, "drift.txt"), []byte("drift"), 0o600)
	}
	runtime.active.Add(-1)
	output := qaInvestigatorOutput{SchemaVersion: QASchemaVersion, Theories: []qaInvestigatorTheory{{Claim: "the changed branch may reject valid input", Basis: "a new conditional branch", VerificationSurface: req.Metadata["shard"], ExpectationRefs: []string{"REQ-1"}, SeverityIfConfirmed: "medium", ConfirmationCondition: "the branch rejects the valid case", RefutationCondition: "the branch accepts the valid case", InconclusiveCondition: "the branch cannot be reached with retained evidence", SafeEvidenceStrategy: "inspect the assigned source", Outcome: QATheoryRefuted, OutcomeReason: "the branch preserves the valid case"}}, Evidence: []QAEvidenceSummary{}, Context: []QAContextRequest{}, Checks: []QAApprovedCheckRef{}}
	data, _ := json.Marshal(output)
	mode := runtime.mode
	if mode == "" {
		mode = "restricted"
	}
	result := pruntime.Result{Status: "completed", TerminalOutput: string(data), Permissions: pruntime.PermissionSummary{Mode: mode, Default: "deny"}}
	if runtime.turns > 0 {
		result.Usage.TurnsKnown = true
		result.Usage.Turns = runtime.turns
		result.Usage.TotalTokensKnown = true
		result.Usage.TotalTokens = 42
		result.EstimatedCost = &pruntime.CostEstimate{Amount: 0.01, Currency: "USD", Estimate: true}
	}
	return result, nil
}

func TestQAInvestigationUsesBoundedWorkersAndPersistsTerminalShards(t *testing.T) {
	root, sp, target, qaMap, flow, state, token := qaRunFixture(t)
	runtime := &qaInvestigatorRuntime{}
	service := withTestQAMapFence(NewService(root).WithRuntime(runtime).WithQASettings(QASettings{Runtime: StageRuntime{Model: "openai/qa", Variant: "high"}, Budgets: qaMap.Budgets}), func(QAMap) error { return nil })
	store := NewQAStore(root, sp).WithWriterFence(func(got QAWriterToken) error {
		if got != token {
			return context.Canceled
		}
		return nil
	})
	if err := store.Publish(QAPublication{Map: &qaMap, Shards: qaMap.Shards, State: state, Flow: flow}, token); err != nil {
		t.Fatal(err)
	}
	state, err := store.LoadState()
	if err != nil {
		t.Fatal(err)
	}
	shards, updated, err := service.runQAShardBatch(context.Background(), store, flow, qaMap, target, append([]QAShard(nil), qaMap.Shards...), state, QARunRequest{WriterToken: token})
	if err != nil {
		t.Fatal(err)
	}
	if got := runtime.max.Load(); got != int32(qaMap.Budgets.ConcurrentInvestigators) {
		t.Fatalf("maximum concurrent investigators = %d", got)
	}
	if updated.CompletedShards != len(shards) {
		t.Fatalf("completed shards = %d/%d", updated.CompletedShards, len(shards))
	}
	for _, shard := range shards {
		if shard.Phase != QAPhaseCompleted || len(shard.Theories) != 1 {
			t.Fatalf("shard = %+v", shard)
		}
		if _, err := store.LoadShard(qaMap.SemanticAttemptID, shard.ID); err != nil {
			t.Fatal(err)
		}
	}
}

func TestQAShardBatchStopsWorkersAfterPublicationFailure(t *testing.T) {
	root, sp, target, qaMap, flow, state, token := qaRunFixture(t)
	runtime := &qaInvestigatorRuntime{}
	service := NewService(root).WithRuntime(runtime).WithQASettings(QASettings{Runtime: StageRuntime{Model: "openai/qa"}, Budgets: qaMap.Budgets}).WithQAMapFence(func(QAMap) error { return nil })
	store := NewQAStore(root, sp).WithWriterFence(func(QAWriterToken) error { return nil })
	if err := store.Publish(QAPublication{Map: &qaMap, Shards: qaMap.Shards, State: state, Flow: flow}, token); err != nil {
		t.Fatal(err)
	}
	store = store.WithHooks(QAStateHooks{BeforeStep: func(kind, _ string) error {
		if kind == "shard" {
			return errors.New("injected shard publication failure")
		}
		return nil
	}})
	if _, _, err := service.runQAShardBatch(context.Background(), store, flow, qaMap, target, append([]QAShard(nil), qaMap.Shards...), state, QARunRequest{WriterToken: token}); err == nil {
		t.Fatal("expected publication failure")
	}
	if active := runtime.active.Load(); active != 0 {
		t.Fatalf("investigators still active after batch return: %d", active)
	}
}

func TestQAShardBatchContainsRuntimePanicAndPublishesBlockedShard(t *testing.T) {
	root, sp, target, qaMap, flow, state, token := qaRunFixture(t)
	service := NewService(root).WithRuntime(qaPanicRuntime{}).WithQASettings(QASettings{Runtime: StageRuntime{Model: "openai/qa"}, Budgets: qaMap.Budgets}).WithQAMapFence(func(QAMap) error { return nil })
	store := NewQAStore(root, sp).WithWriterFence(func(QAWriterToken) error { return nil })
	if err := store.Publish(QAPublication{Map: &qaMap, Shards: qaMap.Shards, State: state, Flow: flow}, token); err != nil {
		t.Fatal(err)
	}
	shards, _, err := service.runQAShardBatch(context.Background(), store, flow, qaMap, target, append([]QAShard(nil), qaMap.Shards...), state, QARunRequest{WriterToken: token, FocusShard: qaMap.Shards[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	if shards[0].Phase != QAPhaseBlocked || shards[0].Blocker == nil || shards[0].Blocker.Category != QAErrorRuntimeUnavailable {
		t.Fatalf("panic result = %+v", shards[0])
	}
}

func TestQATerminalFailurePublicationAndProgressBound(t *testing.T) {
	root, sp, _, qaMap, flow, state, token := qaRunFixture(t)
	store := NewQAStore(root, sp).WithWriterFence(func(QAWriterToken) error { return nil })
	service := NewService(root)
	result, err := service.publishTerminalQAFailure(store, flow, qaMap, qaMap.Shards, state, token, errors.New("synthesis failed"))
	if err == nil || result.State.Phase != QAPhaseBlocked || result.State.Run.Lifecycle != QARunTerminal {
		t.Fatalf("terminal synthesis result=%+v err=%v", result.State, err)
	}
	loaded, loadErr := store.LoadState()
	if loadErr != nil || loaded.Phase != QAPhaseBlocked || loaded.Blocker == nil {
		t.Fatalf("persisted terminal state=%+v err=%v", loaded, loadErr)
	}

	var events []QAProgress
	emit := boundedQAProgress(func(event QAProgress) { events = append(events, event) }, 2)
	for i := 0; i < 5; i++ {
		emit(QAProgress{Event: fmt.Sprintf("event-%d", i)})
	}
	if len(events) != 2 {
		t.Fatalf("bounded progress retained %d events", len(events))
	}
}

func TestCountCompletedQAShardsExcludesBlockedTerminalShards(t *testing.T) {
	shards := []QAShard{
		{Phase: QAPhaseCompleted},
		{Phase: QAPhaseBlocked},
		{Phase: QAPhaseCancelled},
		{Phase: QAPhaseMapped},
	}
	if got := countCompletedQAShards(shards); got != 1 {
		t.Fatalf("completed evidence-ready shards = %d, want 1", got)
	}
}

func TestQATerminalSynthesisFailureRetainsSynthesisArtifact(t *testing.T) {
	root, sp, _, qaMap, flow, state, token := qaRunFixture(t)
	store := NewQAStore(root, sp).WithWriterFence(func(QAWriterToken) error { return nil })
	service := NewService(root)
	synthesis, err := SynthesizeQA(qaMap, qaMap.Shards)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.publishTerminalQAFailureWithSynthesis(store, flow, qaMap, qaMap.Shards, synthesis, state, token, errors.New("synthesis failed"))
	if err == nil || result.Synthesis.ID != synthesis.ID {
		t.Fatalf("terminal synthesis result=%+v err=%v", result.Synthesis, err)
	}
	loaded, loadErr := store.LoadSynthesis(qaMap.SemanticAttemptID, qaMap.Budgets)
	if loadErr != nil || loaded.ID != synthesis.ID {
		t.Fatalf("persisted synthesis=%+v err=%v", loaded, loadErr)
	}
}

func TestQANewStartRejectsExistingSemanticAttemptBeforeRuntimeWork(t *testing.T) {
	root, sp, _, qaMap, flow, state, token := qaRunFixture(t)
	store := NewQAStore(root, sp).WithWriterFence(func(QAWriterToken) error { return nil })
	if err := store.Publish(QAPublication{Map: &qaMap, Shards: qaMap.Shards, State: state, Flow: flow}, token); err != nil {
		t.Fatal(err)
	}

	service := NewService(root)
	_, _, err := service.prepareQAAttempt(store, flow, qaMap, QARunRequest{WriterToken: token}, QASettings{Budgets: qaMap.Budgets})
	typed, ok := AsQAError(err)
	if !ok || typed.Category != QAErrorConflict || !strings.Contains(typed.Detail, "qa resume") {
		t.Fatalf("error = %v", err)
	}
}

func TestQAEvidenceAdmissionRejectsStaleReviewWithoutStartingRuntime(t *testing.T) {
	root, sp := reviewFixture(t)
	target := t.TempDir()
	if err := os.WriteFile(filepath.Join(target, "source.go"), []byte("package source\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	identity, err := targetIdentity(target)
	if err != nil {
		t.Fatal(err)
	}
	qaMap := QAMap{
		ImplementationFingerprint: identity,
		Coverage: QACoverage{
			ChangedPaths:  []string{"source.go"},
			PrimaryOwners: map[string]string{"source.go": "owner"},
		},
	}
	runtime := &qaRetryRuntime{}
	service := NewService(root).WithRuntime(runtime)
	_, _, err = service.validateQAEvidenceAdmission(sp, qaMap, target)
	typed, ok := AsQAError(err)
	if !ok || typed.Category != QAErrorAdmissionBlocked {
		t.Fatalf("stale admission error = %v", err)
	}
	if calls := runtime.calls.Load(); calls != 0 {
		t.Fatalf("runtime calls before admission rejection = %d", calls)
	}
}

func TestQAPermissionRejectsFallbackAndTargetDrift(t *testing.T) {
	root, _, target, qaMap, _, _, token := qaRunFixture(t)
	settings := QASettings{Runtime: StageRuntime{Model: "openai/qa", Variant: "high"}, Budgets: qaMap.Budgets}
	service := withTestQAMapFence(NewService(root).WithQASettings(settings), func(QAMap) error { return nil })
	permissionRuntime := &qaInvestigatorRuntime{mode: "unrestricted"}
	service = service.WithRuntime(permissionRuntime)
	if _, err := service.runOneQAShard(context.Background(), qaMap, qaMap.Shards[0], target, token); err == nil {
		t.Fatal("permission fallback accepted")
	} else if typed, ok := AsQAError(err); !ok || typed.Category != QAErrorPermissionDenied {
		t.Fatalf("permission error = %v", err)
	}
	driftRuntime := &qaInvestigatorRuntime{mutate: target}
	service = service.WithRuntime(driftRuntime)
	if _, err := service.runOneQAShard(context.Background(), qaMap, qaMap.Shards[0], target, token); err == nil {
		t.Fatal("target drift accepted")
	} else if typed, ok := AsQAError(err); !ok || typed.Category != QAErrorPermissionDenied {
		t.Fatalf("drift error = %v", err)
	}
	if err := os.Remove(filepath.Join(target, "drift.txt")); err != nil {
		t.Fatal(err)
	}
	var mapChecks atomic.Int32
	mapDriftService := withTestQAMapFence(NewService(root).WithQASettings(settings).WithRuntime(&qaInvestigatorRuntime{}), func(QAMap) error {
		if mapChecks.Add(1) > 1 {
			return errors.New("governed input changed")
		}
		return nil
	})
	if _, err := mapDriftService.runOneQAShard(context.Background(), qaMap, qaMap.Shards[0], target, token); err == nil {
		t.Fatal("governed input drift accepted")
	} else if typed, ok := AsQAError(err); !ok || typed.Category != QAErrorStaleInput {
		t.Fatalf("governed input drift error = %v", err)
	}
	if mapChecks.Load() != 2 {
		t.Fatalf("governed map checks = %d", mapChecks.Load())
	}
}

func TestQAInvestigatorValidationUsesAgentwrapRepairPolicy(t *testing.T) {
	budgets := DefaultQABudgets()
	budgets.OutputRepairAttempts = 2
	spec := qaInvestigatorValidationSpec(budgets, &qaOutputCapture{})
	if spec.Repair.MaxAttempts != 2 || spec.Repair.SessionAction != agentwrap.SessionActionContinue || !spec.Repair.AllowFreshSessionFallback || !spec.Repair.FreshSessionFallbackOnError {
		t.Fatalf("repair config=%+v", spec.Repair)
	}
	failure := agentwrap.ValidationFailure{Observed: `kind=unknown_field; json: unknown field "extra"`}
	prompt := spec.Repair.BuildPrompt(agentwrap.RepairContext{Validation: agentwrap.ValidationResult{Failures: []agentwrap.ValidationFailure{failure}}, Attempt: 1, MaxAttempts: 2})
	if !strings.Contains(prompt, `unknown field "extra"`) || !strings.Contains(prompt, "theories must contain at least one") {
		t.Fatalf("repair prompt=%q", prompt)
	}
	continued := spec.Repair.OverrideRequest(agentwrap.RepairContext{Attempt: 1}, agentwrap.RunRequest{SessionID: "qa-session", SessionAction: agentwrap.SessionActionContinue})
	fresh := spec.Repair.OverrideRequest(agentwrap.RepairContext{Attempt: 2}, continued)
	if continued.SessionID != "qa-session" || continued.SessionAction != agentwrap.SessionActionContinue || fresh.SessionID != "" || fresh.SessionAction != agentwrap.SessionActionFresh {
		t.Fatalf("continued=%+v fresh=%+v", continued, fresh)
	}
}

func TestQAInvestigatorAgentwrapValidatorReturnsDiagnosticFailure(t *testing.T) {
	budgets := DefaultQABudgets()
	spec := qaInvestigatorValidationSpec(budgets, nil)
	invalid := agentwrap.ValidateRun(context.Background(), agentwrap.RunRequest{}, agentwrap.RunResult{Status: agentwrap.StatusCompleted, TerminalOutput: `{"schema_version":1,"theories":[],"evidence":[],"context_requests":[],"check_requests":[],"extra":true}`}, *spec)
	if invalid.Passed || len(invalid.Failures) != 1 || !strings.Contains(invalid.Failures[0].Observed, "unknown_field") || !strings.Contains(invalid.Failures[0].Observed, `unknown field "extra"`) {
		t.Fatalf("invalid validation=%+v", invalid)
	}
	empty := agentwrap.ValidateRun(context.Background(), agentwrap.RunRequest{}, agentwrap.RunResult{Status: agentwrap.StatusCompleted, TerminalOutput: `{"schema_version":1,"theories":[],"evidence":[],"context_requests":[],"check_requests":[]}`}, *spec)
	if empty.Passed || len(empty.Failures) != 1 || !strings.Contains(empty.Failures[0].Observed, "theories array is empty") {
		t.Fatalf("empty validation=%+v", empty)
	}
	runtimeResult, err := (&qaInvestigatorRuntime{}).StartRun(context.Background(), pruntime.Request{Metadata: map[string]string{"shard": "qa-v1-shard-test"}})
	if err != nil {
		t.Fatal(err)
	}
	valid := agentwrap.ValidateRun(context.Background(), agentwrap.RunRequest{}, agentwrap.RunResult{Status: agentwrap.StatusCompleted, TerminalOutput: runtimeResult.TerminalOutput}, *spec)
	if !valid.Passed {
		t.Fatalf("valid validation=%+v", valid)
	}
}

func TestQAInvestigationEnforcesTurnsAndRetainsUsageCost(t *testing.T) {
	root, _, target, qaMap, _, _, token := qaRunFixture(t)
	settings := QASettings{Runtime: StageRuntime{Model: "openai/qa"}, Budgets: qaMap.Budgets}
	service := NewService(root).WithQASettings(settings).WithRuntime(&qaInvestigatorRuntime{turns: int64(qaMap.Budgets.IterationsPerAttempt)}).WithQAMapFence(func(QAMap) error { return nil })
	completed, err := service.runOneQAShard(context.Background(), qaMap, qaMap.Shards[0], target, token)
	if err != nil || len(completed.Attempts) != 1 || !completed.Attempts[0].Usage.TurnsKnown || completed.Attempts[0].Usage.TotalTokens != 42 || completed.Attempts[0].EstimatedCost == nil {
		t.Fatalf("usage projection shard=%+v err=%v", completed, err)
	}
	service = service.WithRuntime(&qaInvestigatorRuntime{turns: int64(qaMap.Budgets.IterationsPerAttempt + 1)})
	blocked, err := service.runOneQAShard(context.Background(), qaMap, qaMap.Shards[0], target, token)
	if typed, ok := AsQAError(err); !ok || typed.Category != QAErrorBudgetExhausted || len(blocked.Attempts) != 1 || blocked.Attempts[0].StopReason != "investigator iteration limit exceeded" {
		t.Fatalf("iteration limit shard=%+v err=%v", blocked, err)
	}
}

func TestQAInvestigationRetainsAttemptWhenRuntimeReturnsNoTheories(t *testing.T) {
	root, _, target, qaMap, _, _, token := qaRunFixture(t)
	service := NewService(root).
		WithQASettings(QASettings{Runtime: StageRuntime{Model: "openai/qa"}, Budgets: qaMap.Budgets}).
		WithRuntime(qaEmptyTheoryRuntime{}).
		WithQAMapFence(func(QAMap) error { return nil })

	blocked, err := service.runOneQAShard(context.Background(), qaMap, qaMap.Shards[0], target, token)
	typed, ok := AsQAError(err)
	if !ok || typed.Category != QAErrorInvalidState {
		t.Fatalf("error = %v", err)
	}
	if len(blocked.Attempts) != 1 {
		t.Fatalf("attempts = %d", len(blocked.Attempts))
	}
	attempt := blocked.Attempts[0]
	if attempt.Usage.InputTokens != 101 || attempt.Usage.OutputTokens != 17 || attempt.Usage.CacheReadTokens != 73 || attempt.Usage.Turns != 1 || attempt.EstimatedCost == nil {
		t.Fatalf("retained attempt = %+v", attempt)
	}
}

func TestQACancellationPersistsActiveShardWithoutRetry(t *testing.T) {
	root, sp, target, qaMap, flow, state, token := qaRunFixture(t)
	runtime := &qaCancellationRuntime{started: make(chan struct{})}
	service := withTestQAMapFence(NewService(root).WithRuntime(runtime).WithQASettings(QASettings{Runtime: StageRuntime{Model: "openai/qa", Variant: "high"}, Budgets: qaMap.Budgets}), func(QAMap) error { return nil })
	store := NewQAStore(root, sp).WithWriterFence(func(got QAWriterToken) error {
		if got != token {
			return errors.New("stale token")
		}
		return nil
	})
	if err := store.Publish(QAPublication{Map: &qaMap, Shards: qaMap.Shards, State: state, Flow: flow}, token); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, _, err := service.runQAShardBatch(ctx, store, flow, qaMap, target, append([]QAShard(nil), qaMap.Shards...), state, QARunRequest{WriterToken: token, FocusShard: qaMap.Shards[0].ID})
		done <- err
	}()
	<-runtime.started
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation error = %v", err)
	}
	shard, err := store.LoadShard(qaMap.SemanticAttemptID, qaMap.Shards[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if shard.Phase != QAPhaseCancelled || len(shard.Attempts) != 1 {
		t.Fatalf("cancelled shard = %+v", shard)
	}
}

func TestQAInvestigationOutputIsStrictAndBounded(t *testing.T) {
	valid := `{"schema_version":1,"theories":[],"evidence":[],"context_requests":[],"check_requests":[]}`
	if _, _, err := decodeQAInvestigatorOutput(pruntime.Result{TerminalOutput: valid}, len(valid)); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{"unknown": `{"schema_version":1,"theories":[],"evidence":[],"context_requests":[],"check_requests":[],"extra":true}`, "missing": `{"schema_version":1,"theories":[]}`, "trailing": valid + `{}`, "version": `{"schema_version":2,"theories":[],"evidence":[],"context_requests":[],"check_requests":[]}`} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := decodeQAInvestigatorOutput(pruntime.Result{TerminalOutput: content}, 1024); err == nil {
				t.Fatal("invalid output accepted")
			}
		})
	}
	if _, _, err := decodeQAInvestigatorOutput(pruntime.Result{TerminalOutput: valid}, len(valid)-1); err == nil {
		t.Fatal("oversized output accepted")
	}
}

func TestApproveQAContextPathsRequiresContainedRegularFiles(t *testing.T) {
	target := t.TempDir()
	if err := os.MkdirAll(filepath.Join(target, "internal", "web"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "internal", "web", "routes.go"), []byte("package web\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if approved, reason := approveQAContextPaths(target, []string{"internal/web/routes.go"}); !approved || reason != "" {
		t.Fatalf("contained context denied: approved=%t reason=%q", approved, reason)
	}
	if approved, reason := approveQAContextPaths(target, []string{"internal/web/missing.go"}); approved || reason == "" {
		t.Fatalf("missing context approved: approved=%t reason=%q", approved, reason)
	}
	outside := filepath.Join(t.TempDir(), "outside.go")
	if err := os.WriteFile(outside, []byte("package outside\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(target, "internal", "web", "outside.go")); err != nil {
		t.Fatal(err)
	}
	if approved, reason := approveQAContextPaths(target, []string{"internal/web/outside.go"}); approved || reason != "requested context path escapes the QA target" {
		t.Fatalf("escaping context approval: approved=%t reason=%q", approved, reason)
	}
}

func TestQAOutputDiagnosticsAndRepairPromptExposeSafeParserFailure(t *testing.T) {
	_, diagnostic, err := decodeQAInvestigatorOutput(pruntime.Result{Status: "completed", SessionID: "session", TerminalOutput: `{"schema_version":1,"theories":[],"extra":true}`}, 1024)
	if err == nil || diagnostic.Kind != "unknown_field" || !strings.Contains(diagnostic.Detail, `unknown field "extra"`) || diagnostic.OutputBytes == 0 || !diagnostic.Session {
		t.Fatalf("diagnostic=%+v err=%v", diagnostic, err)
	}
}

func TestQARecoveryMissingStateIsRuntimeFreeNoOp(t *testing.T) {
	root, sp, _, _, _, _, _ := qaRunFixture(t)
	snapshot, err := NewService(root).RecoverQA(context.Background(), sp.Project, sp.Slug)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.State.Phase != QAPhaseMissing || snapshot.State.Project != sp.Project || snapshot.State.Sprint != sp.Slug {
		t.Fatalf("missing recovery snapshot = %+v", snapshot)
	}
}

func qaRunFixture(t *testing.T) (string, Sprint, string, QAMap, FlowState, QAState, QAWriterToken) {
	t.Helper()
	root := t.TempDir()
	sp := Sprint{Project: "alpha", Slug: "01-test", Path: filepath.Join(root, "projects", "alpha", "sprints", "01-test")}
	if err := os.MkdirAll(sp.Path, 0o755); err != nil {
		t.Fatal(err)
	}
	target := t.TempDir()
	for _, path := range []string{"internal/app/usecases.go", "internal/sprint/qa.go", "internal/web/handlers.go"} {
		full := filepath.Join(target, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("package test\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	identity, err := targetIdentity(target)
	if err != nil {
		t.Fatal(err)
	}
	input := qaMapInputFixture()
	input.ImplementationFingerprint = identity
	input.Target.Fingerprint = identity
	input.Settings.Budgets.ConcurrentInvestigators = 2
	qaMap, err := BuildQAMap(input)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	token := QAWriterToken{RunID: "run-1", OperationalAttemptID: "op-1", FencingGeneration: 1}
	state := QAState{SchemaVersion: QASchemaVersion, Project: sp.Project, Sprint: sp.Slug, Phase: QAPhaseRunning, Freshness: QAFreshness{Current: true, GovernedInputFingerprint: qaMap.GovernedInputFingerprint, ImplementationFingerprint: identity, ReviewFingerprint: qaMap.ReviewFingerprint, PolicyFingerprint: qaMap.PolicyFingerprint}, CurrentAttemptID: qaMap.SemanticAttemptID, TotalShards: len(qaMap.Shards), Run: qaRunCorrelation(token, QARunActive), NextAction: "Run shards.", UpdatedAt: now}
	return root, sp, target, qaMap, NewFlowState(sp, emptyPlanningStageStates(sp), now), state, token
}
