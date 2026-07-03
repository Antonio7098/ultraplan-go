package runtime

import (
	"time"

	"github.com/Antonio7098/agentwrap"
	"github.com/Antonio7098/agentwrap/opencode"

	"github.com/Antonio7098/ultraplan-go/internal/platform/config"
)

func NewOpenCode(c config.Config) (Adapter, error) {
	opts := []opencode.Option{
		opencode.WithExecutable(c.Agentwrap.Executable),
		opencode.WithExtraArgs(c.Agentwrap.ExtraArgs...),
		opencode.WithEnv(c.Agentwrap.Env...),
		opencode.WithStderrLimit(c.Agentwrap.StderrLimit),
	}
	primary := opencode.NewRuntime(opts...)
	policy := agentwrap.BasicPolicy{
		MaxAttemptsPerTarget: c.Execution.DefaultRetries + 1,
		Backoff:              agentwrap.ExponentialBackoff{Initial: time.Second, Factor: 2, Max: 30 * time.Second},
		RetryRateLimits:      true,
	}
	if c.Models.Backup != "" && c.Models.Backup != c.Models.Primary {
		provider, model := splitModel(c.Models.Backup)
		policy.Fallbacks = []agentwrap.FallbackAlternative{{
			Name: "backup",
			Request: agentwrap.RunRequest{
				Provider: agentwrap.ProviderID(provider),
				Model:    agentwrap.ModelID(model),
			},
			Context: agentwrap.RuntimeContext{
				RuntimeKind: "opencode",
				RuntimeName: "opencode",
				Provider:    agentwrap.ProviderID(provider),
				Model:       agentwrap.ModelID(model),
			},
		}}
	}
	stack := agentwrap.ObservingRuntime{
		Runtime: agentwrap.ValidatingRuntime{
			Runtime: agentwrap.PolicyRunner{
				Runtime: primary,
				Policy:  policy,
			},
		},
		Policy: agentwrap.PersistencePolicy{PersistUnsafeRawPayloads: false},
	}
	return Adapter{runtime: stack, health: primary}, nil
}
