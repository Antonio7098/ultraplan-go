package runtime

import (
	"context"
	"testing"

	"github.com/Antonio7098/agentwrap"
)

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
