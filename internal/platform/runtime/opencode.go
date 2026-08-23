package runtime

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/Antonio7098/agentwrap"
	"github.com/Antonio7098/agentwrap/opencode"

	"github.com/Antonio7098/ultraplan-go/internal/platform/config"
)

var openCodeSessionCleanupMu sync.Mutex

func NewOpenCode(c config.Config) (Adapter, error) {
	newRuntime := func(extraArgs ...string) *opencode.Runtime {
		args := append([]string(nil), c.Agentwrap.ExtraArgs...)
		args = append(args, extraArgs...)
		return opencode.NewRuntime(
			opencode.WithExecutable(c.Agentwrap.Executable),
			opencode.WithExtraArgs(args...),
			opencode.WithEnv(c.Agentwrap.Env...),
			opencode.WithStderrLimit(c.Agentwrap.StderrLimit),
		)
	}
	primary := newRuntime()
	stageRuntime := requestVariantRuntime{
		base: primary,
		withVariant: func(variant string) agentwrap.Runtime {
			return newRuntime("--variant", variant)
		},
	}
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
				Runtime: stageRuntime,
				Policy:  policy,
			},
		},
		Policy: agentwrap.PersistencePolicy{PersistUnsafeRawPayloads: false},
	}
	adapter := Adapter{runtime: stack, health: primary}
	adapter.deleteSession = func(ctx context.Context, sessionID string) error {
		openCodeSessionCleanupMu.Lock()
		defer openCodeSessionCleanupMu.Unlock()

		// OpenCode stores its event stream under an aggregate whose ID is the
		// session ID, but event_sequence has no foreign key back to session.
		// The session CLI therefore cannot cascade into this often much larger
		// payload. Delete it explicitly; event rows cascade from event_sequence.
		query := "DELETE FROM event_sequence WHERE aggregate_id = " + sqliteString(sessionID)
		if output, err := openCodeDBCommand(ctx, c, query).CombinedOutput(); err != nil {
			return fmt.Errorf("delete OpenCode session events %s: %w: %s", sessionID, err, strings.TrimSpace(string(output)))
		}
		cmd := exec.CommandContext(ctx, c.Agentwrap.Executable, "session", "delete", sessionID)
		cmd.Env = append(os.Environ(), c.Agentwrap.Env...)
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("delete OpenCode session %s: %w: %s", sessionID, err, strings.TrimSpace(string(output)))
		}
		// Deleted pages can now be reused by sibling runs. A passive checkpoint
		// bounds WAL growth without waiting for active writers or running VACUUM,
		// which needs substantial temporary disk space and an exclusive lock.
		_, _ = openCodeDBCommand(ctx, c, "PRAGMA wal_checkpoint(PASSIVE)").CombinedOutput()
		return nil
	}
	return adapter, nil
}

func openCodeDBCommand(ctx context.Context, c config.Config, query string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, c.Agentwrap.Executable, "db", query)
	cmd.Env = append(os.Environ(), c.Agentwrap.Env...)
	return cmd
}

func sqliteString(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

// requestVariantRuntime translates UltraPlan's stage-specific variant metadata
// into an adapter invocation. agentwrap deliberately keeps request metadata
// runtime-neutral, while OpenCode exposes reasoning effort as --variant.
type requestVariantRuntime struct {
	base        agentwrap.Runtime
	withVariant func(string) agentwrap.Runtime
}

func (r requestVariantRuntime) StartRun(ctx context.Context, req agentwrap.RunRequest) (agentwrap.Run, error) {
	variant := strings.TrimSpace(req.Metadata["variant"])
	if variant == "" || r.withVariant == nil {
		return r.base.StartRun(ctx, req)
	}
	return r.withVariant(variant).StartRun(ctx, req)
}

func (r requestVariantRuntime) Capabilities(ctx context.Context) (agentwrap.Capabilities, error) {
	return r.base.Capabilities(ctx)
}

// ListModels forwards model enumeration to the primary runtime when it
// supports the optional listing capability.
func (r requestVariantRuntime) ListModels(ctx context.Context, req agentwrap.ModelsRequest) ([]agentwrap.ModelInfo, error) {
	lister, ok := r.base.(agentwrap.ModelLister)
	if !ok {
		return nil, nil
	}
	return lister.ListModels(ctx, req)
}
