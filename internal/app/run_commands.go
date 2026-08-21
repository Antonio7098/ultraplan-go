package app

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Antonio7098/ultraplan-go/internal/platform/config"
	"github.com/Antonio7098/ultraplan-go/internal/runcontrol"
)

func runRun(deps dependencies, args []string) error {
	if len(args) == 0 {
		return classified(ExitUsage, "run requires a subcommand\n\nRun 'ultraplan run --help' for usage.")
	}
	switch args[0] {
	case "--help", "-h":
		_, err := io.WriteString(deps.stdout, runHelp())
		return err
	case "list":
		return runList(deps, args[1:])
	case "show":
		return runShow(deps, args[1:])
	case "follow":
		return runFollow(deps, args[1:])
	case "cancel":
		return runCancel(deps, args[1:])
	case "diagnostics":
		return runDiagnostics(deps, args[1:])
	default:
		return classified(ExitUsage, "run: unknown subcommand %q", args[0])
	}
}

func runRepository(deps dependencies) (runcontrol.Repository, string, error) {
	root, err := discoverWorkspace(deps)
	if err != nil {
		return nil, "", err
	}
	if deps.runControl == nil {
		return nil, "", classified(ExitRuntime, "run-control.init: process state unavailable")
	}
	effective, err := loadEffectiveConfig(root, deps, config.CLIOverrides{})
	if err != nil {
		return nil, "", err
	}
	repository, err := deps.runControl.repository(deps.ctx, root.Path, runControlRetentionPolicy(effective.Config))
	if err != nil {
		return nil, "", classifiedCause(ExitRuntime, err, "run-control.open")
	}
	return repository, root.Path, nil
}

func runList(deps dependencies, args []string) error {
	fs := flag.NewFlagSet("run list", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	jsonOut := fs.Bool("json", false, "emit JSON")
	project := fs.String("project", "", "filter by project")
	sprint := fs.String("sprint", "", "filter by sprint")
	study := fs.String("study", "", "filter by study")
	states := fs.String("lifecycle", "", "comma-separated lifecycle filters")
	limit := fs.Int("limit", 50, "result limit")
	after := fs.String("after", "", "opaque pagination cursor")
	if helpRequested(args) {
		_, err := io.WriteString(deps.stdout, runListHelp())
		return err
	}
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 {
		return classified(ExitUsage, "run list: %v", firstFlagError(err, fs.Args()))
	}
	lifecycle, err := parseLifecycleFilter(*states)
	if err != nil {
		return classified(ExitUsage, "run list: %v", err)
	}
	repository, root, err := runRepository(deps)
	if err != nil {
		return err
	}
	page, err := repository.List(deps.ctx, runcontrol.Query{Lifecycle: lifecycle, Project: *project, Sprint: *sprint, Study: *study, Limit: *limit, After: *after})
	if err != nil {
		return mapRunControlError("run.list", err)
	}
	if *jsonOut {
		return writeJSON(deps.stdout, "run list", root, "ok", page)
	}
	for _, snapshot := range page.Runs {
		fmt.Fprintf(deps.stdout, "%s  %-20s %-18s %s\n", snapshot.RunID, snapshot.Lifecycle, snapshot.Liveness, formatRunTarget(snapshot.Target))
	}
	if len(page.Runs) == 0 {
		fmt.Fprintln(deps.stdout, "No durable runs found.")
	}
	if page.NextCursor != "" {
		fmt.Fprintf(deps.stdout, "Next cursor: %s\n", page.NextCursor)
	}
	return nil
}

func runShow(deps dependencies, args []string) error {
	fs := flag.NewFlagSet("run show", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	jsonOut := fs.Bool("json", false, "emit JSON")
	if helpRequested(args) {
		_, err := io.WriteString(deps.stdout, runShowHelp())
		return err
	}
	if err := fs.Parse(orderRunArgs(args, nil)); err != nil || fs.NArg() != 1 {
		return classified(ExitUsage, "run show: expected '<run-id>' [--json]")
	}
	repository, root, err := runRepository(deps)
	if err != nil {
		return err
	}
	snapshot, err := repository.Snapshot(deps.ctx, runcontrol.RunID(fs.Arg(0)))
	if err != nil {
		return mapRunControlError("run.show", err)
	}
	if *jsonOut {
		return writeJSON(deps.stdout, "run show", root, "ok", snapshot)
	}
	renderRunSnapshot(deps.stdout, snapshot)
	return nil
}

func runFollow(deps dependencies, args []string) error {
	fs := flag.NewFlagSet("run follow", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	jsonOut := fs.Bool("json", false, "emit newline-delimited JSON")
	afterText := fs.String("after", "0", "last committed decimal sequence")
	if helpRequested(args) {
		_, err := io.WriteString(deps.stdout, runFollowHelp())
		return err
	}
	if err := fs.Parse(orderRunArgs(args, map[string]bool{"--after": true})); err != nil || fs.NArg() != 1 {
		return classified(ExitUsage, "run follow: expected '<run-id>' [--after <sequence>] [--json]")
	}
	after, err := strconv.ParseUint(*afterText, 10, 64)
	if err != nil {
		return classified(ExitUsage, "run follow: --after must be a decimal sequence")
	}
	repository, _, err := runRepository(deps)
	if err != nil {
		return err
	}
	runID := runcontrol.RunID(fs.Arg(0))
	if _, err := repository.Snapshot(deps.ctx, runID); err != nil {
		return mapRunControlError("run.follow", err)
	}
	encoder := json.NewEncoder(deps.stdout)
	idle := time.NewTimer(0)
	if !idle.Stop() {
		<-idle.C
	}
	defer idle.Stop()
	for {
		events, err := repository.Events(deps.ctx, runID, after, 512)
		if err != nil {
			return mapRunControlError("run.follow", err)
		}
		for _, event := range events {
			if *jsonOut {
				if err := encoder.Encode(event); err != nil {
					return err
				}
			} else {
				fmt.Fprintf(deps.stdout, "%d  %-14s %s%s\n", event.Sequence, event.Type, event.Stage, formatEventOmission(event.Omission))
			}
			after = event.Sequence
		}
		snapshot, err := repository.Snapshot(deps.ctx, runID)
		if err != nil {
			return mapRunControlError("run.follow", err)
		}
		if snapshot.Lifecycle.IsTerminal() && after >= snapshot.LastSequence {
			return nil
		}
		wait := time.Second
		if len(events) == 512 || after < snapshot.LastSequence {
			wait = 250 * time.Millisecond
		}
		idle.Reset(wait)
		select {
		case <-deps.ctx.Done():
			// Observation cancellation never changes the durable run.
			return nil
		case <-idle.C:
		}
	}
}

func runCancel(deps dependencies, args []string) error {
	fs := flag.NewFlagSet("run cancel", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	jsonOut := fs.Bool("json", false, "emit JSON")
	reason := fs.String("reason", "user_requested", "canonical cancellation reason")
	if helpRequested(args) {
		_, err := io.WriteString(deps.stdout, runCancelHelp())
		return err
	}
	if err := fs.Parse(orderRunArgs(args, map[string]bool{"--reason": true})); err != nil || fs.NArg() != 1 {
		return classified(ExitUsage, "run cancel: expected '<run-id>' [--reason <reason>] [--json]")
	}
	if !canonicalCancellationReason(*reason) {
		return classified(ExitUsage, "run cancel: unsupported reason %q", *reason)
	}
	repository, root, err := runRepository(deps)
	if err != nil {
		return err
	}
	snapshot, changed, err := repository.RequestCancellation(deps.ctx, runcontrol.RunID(fs.Arg(0)), *reason)
	if err != nil {
		return mapRunControlError("run.cancel", err)
	}
	if *jsonOut {
		return writeJSON(deps.stdout, "run cancel", root, "ok", struct {
			Changed  bool                `json:"changed"`
			Snapshot runcontrol.Snapshot `json:"run"`
		}{changed, snapshot})
	}
	fmt.Fprintf(deps.stdout, "Run: %s\nCancellation: %s\nChanged: %t\n", snapshot.RunID, snapshot.Cancellation.State, changed)
	return nil
}

func runDiagnostics(deps dependencies, args []string) error {
	fs := flag.NewFlagSet("run diagnostics", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	jsonOut := fs.Bool("json", false, "emit JSON")
	supportPath := fs.String("support-export", "", "write a private bounded support JSON file")
	if helpRequested(args) {
		_, err := io.WriteString(deps.stdout, runDiagnosticsHelp())
		return err
	}
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 {
		return classified(ExitUsage, "run diagnostics: %v", firstFlagError(err, fs.Args()))
	}
	repository, root, err := runRepository(deps)
	if err != nil {
		return err
	}
	health, err := repository.Health(deps.ctx)
	if err != nil {
		return mapRunControlError("run.diagnostics", err)
	}
	if *supportPath != "" {
		workspaceRoot, discoverErr := discoverWorkspace(deps)
		if discoverErr != nil {
			return discoverErr
		}
		effective, configErr := loadEffectiveConfig(workspaceRoot, deps, config.CLIOverrides{})
		if configErr != nil {
			return configErr
		}
		if err := writeRunSupportExport(deps.ctx, repository, root, health, effective, *supportPath); err != nil {
			return mapRunControlError("run.support_export", err)
		}
	}
	if *jsonOut {
		return writeJSON(deps.stdout, "run diagnostics", root, string(health.Status), struct {
			Health      runcontrol.Health `json:"health"`
			SupportPath string            `json:"support_export,omitempty"`
		}{health, *supportPath})
	}
	fmt.Fprintf(deps.stdout, "Status: %s\nJournal: %s\nSynchronous: %s\nForeign keys: %t\nActive runs: %d\nStalled runs: %d\nCancellation uncertainty: %d\nReconciliation backlog: %d\nOldest backlog age: %s\n",
		health.Status, health.JournalMode, health.Synchronous, health.ForeignKeys, health.ActiveRuns, health.StalledRuns,
		health.CancellationUncertain, health.ReconciliationBacklog, health.OldestBacklogAge)
	if *supportPath != "" {
		fmt.Fprintf(deps.stdout, "Support export: %s\n", *supportPath)
	}
	return nil
}

type runSupportExport struct {
	SchemaVersion  int                                 `json:"schema_version"`
	Health         runcontrol.Health                   `json:"health"`
	Config         runSupportConfig                    `json:"config"`
	Runs           []runcontrol.Snapshot               `json:"runs"`
	Events         []runSupportEvents                  `json:"events"`
	Reconciliation []runcontrol.ReconciliationEvidence `json:"reconciliation"`
	Logs           []runcontrol.LocalLogRecord         `json:"logs"`
}

type runSupportConfig struct {
	FullHistory        string `json:"full_history"`
	FullHistorySource  string `json:"full_history_source"`
	TombstoneHistory   string `json:"tombstone_history"`
	TombstoneSource    string `json:"tombstone_history_source"`
	WorkspaceQuota     int64  `json:"workspace_quota_bytes"`
	WorkspaceQuotaFrom string `json:"workspace_quota_source"`
}

type runSupportEvents struct {
	RunID  runcontrol.RunID `json:"run_id"`
	Events []struct {
		Sequence uint64               `json:"sequence"`
		Type     runcontrol.EventType `json:"type"`
		Omission *runcontrol.Omission `json:"omission,omitempty"`
	} `json:"events"`
}

func writeRunSupportExport(ctx context.Context, repository runcontrol.Repository, workspaceRoot string, health runcontrol.Health, effective config.Effective, path string) error {
	page, err := repository.List(ctx, runcontrol.Query{Limit: 50})
	if err != nil {
		return err
	}
	export := runSupportExport{
		SchemaVersion: 1,
		Health:        health,
		Config: runSupportConfig{
			FullHistory: effective.Config.RunControl.FullHistory, FullHistorySource: effective.Sources["run_control.full_history"],
			TombstoneHistory: effective.Config.RunControl.TombstoneHistory, TombstoneSource: effective.Sources["run_control.tombstone_history"],
			WorkspaceQuota: effective.Config.RunControl.WorkspaceQuota, WorkspaceQuotaFrom: effective.Sources["run_control.workspace_quota_bytes"],
		},
		Runs: page.Runs,
	}
	if reader, ok := repository.(interface {
		ReconciliationEvidence(context.Context, int) ([]runcontrol.ReconciliationEvidence, error)
	}); ok {
		export.Reconciliation, err = reader.ReconciliationEvidence(ctx, 100)
		if err != nil {
			return err
		}
	}
	export.Logs, err = runcontrol.ReadLocalLogs(workspaceRoot, 100)
	if err != nil {
		return err
	}
	for _, snapshot := range page.Runs {
		events, err := repository.Events(ctx, snapshot.RunID, 0, 50)
		if err != nil {
			return err
		}
		item := runSupportEvents{RunID: snapshot.RunID}
		for _, event := range events {
			item.Events = append(item.Events, struct {
				Sequence uint64               `json:"sequence"`
				Type     runcontrol.EventType `json:"type"`
				Omission *runcontrol.Omission `json:"omission,omitempty"`
			}{event.Sequence, event.Type, event.Omission})
		}
		export.Events = append(export.Events, item)
	}
	encoded, err := json.MarshalIndent(export, "", "  ")
	if err != nil {
		return err
	}
	if len(encoded) > 1<<20 {
		return errors.New("support export exceeds 1 MiB limit")
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	ok := false
	defer func() {
		_ = file.Close()
		if !ok {
			_ = os.Remove(path)
		}
	}()
	if _, err := file.Write(append(encoded, '\n')); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	ok = true
	return nil
}

func parseLifecycleFilter(value string) ([]runcontrol.Lifecycle, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	var out []runcontrol.Lifecycle
	for _, part := range strings.Split(value, ",") {
		state := runcontrol.Lifecycle(strings.TrimSpace(part))
		if !state.IsValid() {
			return nil, fmt.Errorf("unknown lifecycle %q", part)
		}
		out = append(out, state)
	}
	return out, nil
}

func canonicalCancellationReason(value string) bool {
	switch value {
	case "user_requested", "operator_requested", "shutdown_requested", "quota_exceeded":
		return true
	default:
		return false
	}
}

func mapRunControlError(operation string, err error) error {
	switch {
	case errors.Is(err, runcontrol.ErrInvalidArgument):
		return classifiedCause(ExitUsage, err, operation)
	case errors.Is(err, runcontrol.ErrNotFound):
		return classifiedCause(ExitValidation, err, operation)
	case errors.Is(err, runcontrol.ErrConflict), errors.Is(err, runcontrol.ErrTerminal):
		return classifiedCause(ExitPartial, err, operation)
	default:
		return classifiedCause(ExitRuntime, err, operation)
	}
}

func renderRunSnapshot(w io.Writer, snapshot runcontrol.Snapshot) {
	fmt.Fprintf(w, "Run: %s\nTarget: %s\nLifecycle: %s\nLiveness: %s\nProduct status: %s\nCancellation: %s\nAccepted: %s\nUpdated: %s\nLast sequence: %d\nOldest retained sequence: %d\nHistory complete: %t\nRecord state: %s\n",
		snapshot.RunID, formatRunTarget(snapshot.Target), snapshot.Lifecycle, snapshot.Liveness,
		firstSafeValue(snapshot.ProductStatus, "unknown"), snapshot.Cancellation.State,
		snapshot.AcceptedAt.Format(time.RFC3339), snapshot.UpdatedAt.Format(time.RFC3339), snapshot.LastSequence,
		snapshot.OldestRetainedSequence, snapshot.HistoryComplete, snapshot.RecordState)
	if snapshot.Terminal != nil {
		fmt.Fprintf(w, "Terminal: %s (%s)\n", snapshot.Terminal.Outcome, snapshot.Terminal.Reason)
	}
}

func formatRunTarget(target runcontrol.Target) string {
	parts := []string{target.Kind, target.Operation}
	for _, value := range []string{target.Project, target.Sprint, target.Study, target.Stage, target.Task} {
		if value != "" {
			parts = append(parts, value)
		}
	}
	return strings.Join(parts, "/")
}

func formatEventOmission(omission *runcontrol.Omission) string {
	if omission == nil {
		return ""
	}
	return fmt.Sprintf(" omitted=%d", omission.Count)
}

func helpRequested(args []string) bool {
	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			return true
		}
	}
	return false
}

func firstFlagError(err error, extra []string) error {
	if err != nil {
		return err
	}
	if len(extra) > 0 {
		return fmt.Errorf("unexpected argument %q", extra[0])
	}
	return errors.New("invalid arguments")
}

func orderRunArgs(args []string, valued map[string]bool) []string {
	flags := make([]string, 0, len(args))
	positionals := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		name := strings.SplitN(arg, "=", 2)[0]
		isFlag := name == "--json" || name == "-h" || name == "--help" || valued[name]
		if !isFlag {
			positionals = append(positionals, arg)
			continue
		}
		flags = append(flags, arg)
		if valued[name] && !strings.Contains(arg, "=") && i+1 < len(args) {
			i++
			flags = append(flags, args[i])
		}
	}
	return append(flags, positionals...)
}

func runHelp() string {
	return `ultraplan run

Usage:
  ultraplan run <list|show|follow|cancel|diagnostics> [flags]

Commands:
  list          List newest durable workspace runs.
  show          Show one durable run snapshot.
  follow        Replay committed events and follow until terminal.
  cancel        Persist an explicit cancellation request.
  diagnostics   Inspect repository health or write a support export.
`
}

func runListHelp() string {
	return "Usage: ultraplan run list [--project <id>] [--sprint <id>] [--study <id>] [--lifecycle <states>] [--limit <1-200>] [--after <cursor>] [--json]\n"
}
func runShowHelp() string { return "Usage: ultraplan run show <run-id> [--json]\n" }
func runFollowHelp() string {
	return "Usage: ultraplan run follow <run-id> [--after <sequence>] [--json]\n"
}
func runCancelHelp() string {
	return "Usage: ultraplan run cancel <run-id> [--reason <reason>] [--json]\n"
}
func runDiagnosticsHelp() string {
	return "Usage: ultraplan run diagnostics [--json] [--support-export <path>]\n"
}
