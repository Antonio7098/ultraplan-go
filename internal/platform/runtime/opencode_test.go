package runtime

import (
	"context"
	"errors"
	"testing"

	"github.com/Antonio7098/agentwrap"
)

type countingPolicy struct {
	calls int
}

func (p *countingPolicy) Decide(context.Context, agentwrap.PolicyContext) (agentwrap.PolicyDecision, error) {
	p.calls++
	return agentwrap.PolicyDecision{Kind: agentwrap.PolicyDecisionRetry}, nil
}

func TestMissingSessionPolicyStopsWithoutRetrying(t *testing.T) {
	next := &countingPolicy{}
	policy := missingSessionPolicy{next: next}
	sdkErr := agentwrap.NewError(agentwrap.ErrorRuntimeExit, "opencode run", "OpenCode exited before a successful final result", errors.New("Error: Session not found"))
	decision, err := policy.Decide(context.Background(), agentwrap.PolicyContext{Err: sdkErr})
	if err != nil || decision.Kind != agentwrap.PolicyDecisionStop || next.calls != 0 {
		t.Fatalf("decision=%+v next calls=%d err=%v", decision, next.calls, err)
	}
}

func TestFallbackStagePolicyVetoesOnlyConfiguredStages(t *testing.T) {
	runtimeErr := agentwrap.NewError(agentwrap.ErrorRuntimeUnavailable, "primary", "unavailable", nil)
	policy := agentwrap.BasicPolicy{
		MaxAttemptsPerTarget: 2,
		Fallbacks:            []agentwrap.FallbackAlternative{{Name: "backup"}},
		FallbackGuards:       []func(agentwrap.PolicyContext) bool{fallbackStageGuard(map[string]struct{}{"execute": {}})},
	}

	retry, err := policy.Decide(context.Background(), agentwrap.PolicyContext{Err: runtimeErr, AttemptOnTarget: 1, Metadata: map[string]string{"stage": " Execute "}})
	if err != nil || retry.Kind != agentwrap.PolicyDecisionRetry {
		t.Fatalf("retry decision=%+v err=%v", retry, err)
	}
	stopped, err := policy.Decide(context.Background(), agentwrap.PolicyContext{Err: runtimeErr, AttemptOnTarget: 2, Metadata: map[string]string{"stage": " Execute "}})
	if err != nil || stopped.Kind != agentwrap.PolicyDecisionStop || stopped.Err != runtimeErr {
		t.Fatalf("stopped decision=%+v err=%v", stopped, err)
	}
	allowed, err := policy.Decide(context.Background(), agentwrap.PolicyContext{Err: runtimeErr, AttemptOnTarget: 2, Metadata: map[string]string{"stage": "plan"}})
	if err != nil || allowed.Kind != agentwrap.PolicyDecisionFallback {
		t.Fatalf("allowed decision=%+v err=%v", allowed, err)
	}
}

type variantProbeRuntime struct {
	calls int
}

func (r *variantProbeRuntime) StartRun(context.Context, agentwrap.RunRequest) (agentwrap.Run, error) {
	r.calls++
	return nil, nil
}

func (*variantProbeRuntime) Capabilities(context.Context) (agentwrap.Capabilities, error) {
	return agentwrap.Capabilities{RuntimeKind: "probe"}, nil
}

func TestRequestVariantRuntimeRoutesStageVariant(t *testing.T) {
	base := &variantProbeRuntime{}
	low := &variantProbeRuntime{}
	requested := ""
	runtime := requestVariantRuntime{
		base: base,
		withVariant: func(variant string) agentwrap.Runtime {
			requested = variant
			return low
		},
	}

	if _, err := runtime.StartRun(context.Background(), agentwrap.RunRequest{Metadata: map[string]string{"variant": " low "}}); err != nil {
		t.Fatal(err)
	}
	if requested != "low" || low.calls != 1 || base.calls != 0 {
		t.Fatalf("requested=%q low.calls=%d base.calls=%d", requested, low.calls, base.calls)
	}

	if _, err := runtime.StartRun(context.Background(), agentwrap.RunRequest{}); err != nil {
		t.Fatal(err)
	}
	if base.calls != 1 {
		t.Fatalf("base.calls=%d, want 1", base.calls)
	}
}

func TestRequestVariantRuntimeUsesBaseCapabilities(t *testing.T) {
	base := &variantProbeRuntime{}
	runtime := requestVariantRuntime{base: base}
	capabilities, err := runtime.Capabilities(context.Background())
	if err != nil || capabilities.RuntimeKind != "probe" {
		t.Fatalf("capabilities=%+v err=%v", capabilities, err)
	}
}

func TestSQLiteStringEscapesSessionID(t *testing.T) {
	if got, want := sqliteString("ses_'quoted"), "'ses_''quoted'"; got != want {
		t.Fatalf("sqliteString() = %q, want %q", got, want)
	}
}
