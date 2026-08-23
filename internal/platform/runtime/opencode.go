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
	adapter.deleteSessions = func(ctx context.Context, sessionIDs []string) error {
		openCodeSessionCleanupMu.Lock()
		defer openCodeSessionCleanupMu.Unlock()

		for _, sessionID := range sessionIDs {
			if strings.TrimSpace(sessionID) == "" {
				continue
			}
			// OpenCode stores its event stream under an aggregate whose ID is the
			// session ID, but event_sequence has no foreign key back to session.
			query := "DELETE FROM event_sequence WHERE aggregate_id = " + sqliteString(sessionID)
			if output, err := openCodeDBCommand(ctx, c, query).CombinedOutput(); err != nil {
				return fmt.Errorf("delete OpenCode session events %s: %w: %s", sessionID, err, strings.TrimSpace(string(output)))
			}
			cmd := exec.CommandContext(ctx, c.Agentwrap.Executable, "session", "delete", sessionID)
			cmd.Env = append(os.Environ(), c.Agentwrap.Env...)
			if output, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("delete OpenCode session %s: %w: %s", sessionID, err, strings.TrimSpace(string(output)))
			}
		}
		if output, err := openCodeDBCommand(ctx, c, "PRAGMA wal_checkpoint(TRUNCATE)").CombinedOutput(); err != nil {
			return fmt.Errorf("checkpoint OpenCode database: %w: %s", err, strings.TrimSpace(string(output)))
		}
		if output, err := openCodeDBCommand(ctx, c, "VACUUM").CombinedOutput(); err != nil {
			return fmt.Errorf("vacuum OpenCode database: %w: %s", err, strings.TrimSpace(string(output)))
		}
		// In WAL mode VACUUM writes the compacted image to the WAL. Checkpoint it
		// before the next worker wave starts so the temporary copy is removed.
		if output, err := openCodeDBCommand(ctx, c, "PRAGMA wal_checkpoint(TRUNCATE)").CombinedOutput(); err != nil {
			return fmt.Errorf("checkpoint vacuumed OpenCode database: %w: %s", err, strings.TrimSpace(string(output)))
		}
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
