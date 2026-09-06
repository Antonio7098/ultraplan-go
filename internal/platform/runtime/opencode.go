package runtime

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Antonio7098/agentwrap"
	"github.com/Antonio7098/agentwrap/opencode"

	"github.com/Antonio7098/ultraplan-go/internal/platform/config"
)

var openCodeSessionCleanupMu sync.Mutex

// rateSnapshotDir returns the cache directory holding the model-rate snapshot
// shared across runs. An empty result disables persistence (memory-only).
func rateSnapshotDir() string {
	cache, err := os.UserCacheDir()
	if err != nil || cache == "" {
		return ""
	}
	return filepath.Join(cache, "ultraplan")
}

func NewOpenCode(c config.Config) (Adapter, error) {
	rateStore := agentwrap.NewRateTableStore(rateSnapshotDir())
	newRuntime := func(extraArgs ...string) *opencode.Runtime {
		args := append([]string(nil), c.Agentwrap.ExtraArgs...)
		args = append(args, extraArgs...)
		return opencode.NewRuntime(
			opencode.WithExecutable(c.Agentwrap.Executable),
			opencode.WithExtraArgs(args...),
			opencode.WithEnv(c.Agentwrap.Env...),
			opencode.WithSnapshots(false),
			opencode.WithStderrLimit(c.Agentwrap.StderrLimit),
			opencode.WithRateTableStore(rateStore),
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
	disabledFallbackStages := make(map[string]struct{}, len(c.Models.Fallback.DisabledStages))
	for _, stage := range c.Models.Fallback.DisabledStages {
		disabledFallbackStages[strings.ToLower(strings.TrimSpace(stage))] = struct{}{}
	}
	policy.FallbackGuards = append(policy.FallbackGuards, fallbackStageGuard(disabledFallbackStages))
	stack := agentwrap.ObservingRuntime{
		Runtime: agentwrap.ValidatingRuntime{
			Runtime: agentwrap.PolicyRunner{
				Runtime: stageRuntime,
				Policy:  missingSessionPolicy{next: policy},
			},
		},
		Policy: agentwrap.PersistencePolicy{PersistUnsafeRawPayloads: false},
	}
	adapter := Adapter{runtime: stack, health: primary}
	adapter.deleteRuntimeStore = func(_ context.Context, path string) error {
		if err := removeRuntimeStore(path); err != nil {
			markRuntimeStoreCleanupPending(path, err)
			return err
		}
		openCodeSessionCleanupMu.Lock()
		_ = pruneOpenCodeLogs(c)
		openCodeSessionCleanupMu.Unlock()
		return nil
	}
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
		if err := checkpointOpenCode(ctx, c); err != nil {
			return fmt.Errorf("checkpoint OpenCode database: %w", err)
		}
		if output, err := openCodeDBCommand(ctx, c, "VACUUM").CombinedOutput(); err != nil {
			return fmt.Errorf("vacuum OpenCode database: %w: %s", err, strings.TrimSpace(string(output)))
		}
		// In WAL mode VACUUM writes the compacted image to the WAL. Checkpoint it
		// before the next worker wave starts so the temporary copy is removed.
		if err := checkpointOpenCode(ctx, c); err != nil {
			return fmt.Errorf("checkpoint vacuumed OpenCode database: %w", err)
		}
		return nil
	}
	return adapter, nil
}

func fallbackStageGuard(disabled map[string]struct{}) func(agentwrap.PolicyContext) bool {
	return func(policyCtx agentwrap.PolicyContext) bool {
		stage := strings.ToLower(strings.TrimSpace(policyCtx.Metadata["stage"]))
		_, blocked := disabled[stage]
		return !blocked
	}
}

type missingSessionPolicy struct {
	next agentwrap.ResiliencePolicy
}

func (p missingSessionPolicy) Decide(ctx context.Context, policyCtx agentwrap.PolicyContext) (agentwrap.PolicyDecision, error) {
	if openCodeSessionNotFound(policyCtx.Err) {
		return agentwrap.PolicyDecision{Kind: agentwrap.PolicyDecisionStop, Reason: "session not found", Detail: policyCtx.Err.UserDetail, Err: policyCtx.Err}, nil
	}
	return p.next.Decide(ctx, policyCtx)
}

func openCodeSessionNotFound(err *agentwrap.SDKError) bool {
	if err == nil {
		return false
	}
	details := []string{err.UserDetail, err.DebugDetail, err.ResponseBody}
	if err.Cause != nil {
		details = append(details, err.Cause.Error())
	}
	return strings.Contains(strings.ToLower(strings.Join(details, "\n")), "session not found")
}

func checkpointOpenCode(ctx context.Context, c config.Config) error {
	var detail string
	for attempt := 0; attempt < 20; attempt++ {
		output, err := openCodeDBCommand(ctx, c, "PRAGMA wal_checkpoint(TRUNCATE)").CombinedOutput()
		detail = strings.TrimSpace(string(output))
		if err == nil {
			fields := strings.Fields(detail)
			if len(fields) >= 3 && fields[len(fields)-3] == "0" {
				return nil
			}
		}
		if err := sleepContext(ctx, 250*time.Millisecond); err != nil {
			return err
		}
	}
	return fmt.Errorf("database remained busy: %s", detail)
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
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
