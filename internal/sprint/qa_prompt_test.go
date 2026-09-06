package sprint

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pprocess "github.com/Antonio7098/ultraplan-go/internal/platform/process"
)

func TestQAInvestigatorRequestIsReadOnlyDefaultDenyAndPathBounded(t *testing.T) {
	input := qaMapInputFixture()
	qaMap, err := BuildQAMap(input)
	if err != nil {
		t.Fatal(err)
	}
	shard := qaMap.Shards[0]
	service := NewService(t.TempDir()).WithQASettings(input.Settings)
	target := t.TempDir()
	req, err := service.QAInvestigatorRequest(qaMap, shard, target)
	if err != nil {
		t.Fatal(err)
	}
	if req.Sandbox != "read_only" || req.Permissions != "restricted" || req.Policy.Default != "deny" {
		t.Fatalf("permission request = sandbox=%q permissions=%q policy=%+v", req.Sandbox, req.Permissions, req.Policy)
	}
	for _, tool := range []string{"read", "list", "search", "glob"} {
		if req.Policy.Tools[tool] != "allow" {
			t.Fatalf("tool %s = %q", tool, req.Policy.Tools[tool])
		}
	}
	for _, tool := range []string{"write", "edit", "patch", "bash", "shell"} {
		if req.Policy.Tools[tool] != "deny" {
			t.Fatalf("tool %s = %q", tool, req.Policy.Tools[tool])
		}
	}
	for _, rule := range req.Policy.PathRules {
		if !inside(target, rule.Path) || rule.Action != "allow" {
			t.Fatalf("path rule = %+v", rule)
		}
	}
	if !strings.Contains(req.Prompt, "read-only repository tools") || strings.Contains(req.Prompt, "repair code now") {
		t.Fatalf("investigator prompt = %s", req.Prompt)
	}
	if req.PromptRef.ID != "sprint.qa.investigator" || req.PromptRef.Purpose != "qa.investigator" || req.Metadata["prompt_id"] != req.PromptRef.ID {
		t.Fatalf("investigator prompt identity = %+v metadata=%+v", req.PromptRef, req.Metadata)
	}
}

func TestQAChallengerRequestIsBoundedAndHasNoToolOrPathAuthority(t *testing.T) {
	qaMap, shards := qaSynthesisFixture(t)
	service := NewService(t.TempDir()).WithQASettings(qaMapInputFixture().Settings)
	req, err := service.QAChallengerRequest(qaMap, shards, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if req.Sandbox != "read_only" || req.Permissions != "restricted" || req.Policy.Default != "deny" || len(req.Policy.PathRules) != 0 {
		t.Fatalf("challenger permissions = %+v", req)
	}
	for _, tool := range []string{"read", "list", "search", "glob"} {
		if req.Policy.Tools[tool] != "allow" {
			t.Fatalf("challenger tool %s = %q", tool, req.Policy.Tools[tool])
		}
	}
	if len(req.Prompt) > qaMap.Budgets.PromptBytes || !strings.Contains(req.Prompt, "Do not change an outcome") {
		t.Fatalf("challenger prompt is not bounded or explicit: %s", req.Prompt)
	}
	if req.PromptRef.ID != "sprint.qa.challenger" || req.PromptRef.Purpose != "qa.challenger" || req.Metadata["prompt_id"] != req.PromptRef.ID {
		t.Fatalf("challenger prompt identity = %+v metadata=%+v", req.PromptRef, req.Metadata)
	}
}

func TestQARoleModelOverridesAreIndependent(t *testing.T) {
	input := qaMapInputFixture()
	input.Settings.Mapper = StageRuntime{Model: "provider/mapper", Variant: "medium"}
	input.Settings.Investigator = StageRuntime{Model: "provider/investigator", Variant: "low"}
	input.Settings.Challenger = StageRuntime{Model: "provider/challenger", Variant: "high"}
	input.Settings.Arbiter = StageRuntime{Model: "provider/arbiter", Variant: "high"}
	input.Settings.Reconciler = StageRuntime{Model: "provider/reconciler", Variant: "high"}
	qaMap, err := BuildQAMap(input)
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(t.TempDir()).WithQASettings(input.Settings)
	investigator, err := service.QAInvestigatorRequest(qaMap, qaMap.Shards[0], t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	challenger, err := service.QAChallengerRequest(qaMap, qaMap.Shards, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if investigator.Provider != "provider" || investigator.Model != "investigator" || investigator.Metadata["reasoning_effort"] != "low" {
		t.Fatalf("investigator runtime = %+v", investigator)
	}
	if challenger.Provider != "provider" || challenger.Model != "challenger" || challenger.Metadata["reasoning_effort"] != "high" {
		t.Fatalf("challenger runtime = %+v", challenger)
	}
	for role, want := range map[string]string{"mapper": "mapper", "arbiter": "arbiter", "reconciler": "reconciler"} {
		if got := input.Settings.RuntimeFor(role); got.Model != "provider/"+want {
			t.Fatalf("%s runtime = %+v", role, got)
		}
	}
}

func TestQARequestModelOverrideAppliesToEveryRole(t *testing.T) {
	settings := qaMapInputFixture().Settings
	settings.Mapper.Model = "provider/mapper"
	settings.Investigator.Model = "provider/investigator"
	settings.Repair.Model = "provider/repair"

	overridden := settings.WithModelOverride("  provider/requested  ")
	for _, role := range []string{"mapper", "investigator", "challenger", "arbiter", "reconciler", "evaluator", "repair"} {
		if got := overridden.RuntimeFor(role).Model; got != "provider/requested" {
			t.Fatalf("%s model = %q", role, got)
		}
	}
	if settings.RuntimeFor("mapper").Model != "provider/mapper" {
		t.Fatal("model override mutated the configured settings")
	}
}

func TestApprovedQACheckCatalogUsesExplicitReadOnlyArgv(t *testing.T) {
	target := t.TempDir()
	checks, err := ApprovedQAChecks(target, []string{"internal/a.go", "README.md"}, DefaultQABudgets())
	if err != nil {
		t.Fatal(err)
	}
	if len(checks) != 2 || checks[0].Executable != "gofmt" || len(checks[0].Args) != 2 || checks[0].Args[0] != "-d" || !checks[0].RequireEmptyOut || checks[1].ID != "go-source-integrity" || !validFingerprint(checks[0].Fingerprint) {
		t.Fatalf("checks = %+v", checks)
	}
	again, err := ApprovedQAChecks(target, []string{"README.md", "internal/a.go"}, DefaultQABudgets())
	if err != nil || checks[0].Fingerprint != again[0].Fingerprint {
		t.Fatalf("catalog is not deterministic: %+v %+v %v", checks, again, err)
	}
}

func TestQACheckPolicyRejectsShellGitWritesEscapesAndEnvironment(t *testing.T) {
	target := t.TempDir()
	base := QACheckDescriptor{ID: "safe", Executable: "gofmt", Args: []string{"-d", "a.go"}, WorkingDirectory: target, Timeout: DefaultQABudgets().CommandTimeout, OutputLimit: DefaultQABudgets().CommandOutputBytes}
	for name, mutate := range map[string]func(*QACheckDescriptor){
		"shell":       func(d *QACheckDescriptor) { d.Executable = "bash" },
		"git":         func(d *QACheckDescriptor) { d.Executable = "git" },
		"write":       func(d *QACheckDescriptor) { d.Args = []string{"-w", "a.go"} },
		"escape":      func(d *QACheckDescriptor) { d.Args = []string{"-d", "../a.go"} },
		"indirection": func(d *QACheckDescriptor) { d.Args = []string{"-d", "$(touch x)"} },
		"environment": func(d *QACheckDescriptor) { d.Environment = []string{"PATH"} },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := base
			mutate(&candidate)
			if err := validateQACheckDescriptor(target, candidate, DefaultQABudgets()); err == nil {
				t.Fatal("unsafe descriptor accepted")
			}
		})
	}
}

type qaProcessRunner struct {
	target string
	drift  bool
	stdout string
	seen   pprocess.Request
}

func (runner *qaProcessRunner) Run(_ context.Context, request pprocess.Request) (pprocess.Result, error) {
	runner.seen = request
	if runner.drift {
		if err := os.WriteFile(filepath.Join(runner.target, "drift.txt"), []byte("changed"), 0o600); err != nil {
			return pprocess.Result{}, err
		}
	}
	return pprocess.Result{ExitCode: 0, Stdout: runner.stdout}, nil
}

func TestRunApprovedQACheckRejectsUnownedAndDetectsTargetDrift(t *testing.T) {
	target := t.TempDir()
	if err := os.MkdirAll(filepath.Join(target, "internal"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "internal", "a.go"), []byte("package internal\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	checks, err := ApprovedQAChecks(target, []string{"internal/a.go"}, DefaultQABudgets())
	if err != nil {
		t.Fatal(err)
	}
	descriptor := checks[0]
	ref := QAApprovedCheckRef{ID: descriptor.ID, Fingerprint: descriptor.Fingerprint}
	input := qaMapInputFixture()
	input.ChangedPaths = []string{"internal/a.go"}
	input.ApprovedChecks = []QAApprovedCheckRef{ref}
	qaMap, err := BuildQAMap(input)
	if err != nil {
		t.Fatal(err)
	}
	runner := &qaProcessRunner{target: target}
	service := NewService(t.TempDir()).WithProcessRunner(runner)
	if _, err := service.RunApprovedQACheck(context.Background(), qaMap, descriptor, QAApprovedCheckRef{ID: "other", Fingerprint: descriptor.Fingerprint}); err == nil {
		t.Fatal("unowned check accepted")
	}
	runner.stdout = "diff"
	if _, err := service.RunApprovedQACheck(context.Background(), qaMap, descriptor, ref); err == nil {
		t.Fatal("gofmt diff output was accepted")
	}
	runner.stdout = ""
	if _, err := service.RunApprovedQACheck(context.Background(), qaMap, descriptor, ref); err != nil {
		t.Fatal(err)
	}
	if runner.seen.Executable != "gofmt" || runner.seen.Args[0] != "-d" {
		t.Fatalf("process request = %+v", runner.seen)
	}
	runner.drift = true
	if _, err := service.RunApprovedQACheck(context.Background(), qaMap, descriptor, ref); err == nil {
		t.Fatal("target drift was not detected")
	} else {
		qaErr, ok := AsQAError(err)
		if !ok || qaErr.Category != QAErrorPermissionDenied {
			t.Fatalf("drift error = %v", err)
		}
	}
}

func TestTargetIdentityRejectsSymlinkEscapeAndRecordsContainedSymlink(t *testing.T) {
	target := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.go")
	if err := os.WriteFile(outside, []byte("package outside\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(target, "escape.go")); err != nil {
		t.Fatal(err)
	}
	if _, err := targetIdentity(target); err == nil {
		t.Fatal("escaping symlink was accepted")
	}
	if err := os.Remove(filepath.Join(target, "escape.go")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "source.go"), []byte("package source\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "other.go"), []byte("package source\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("source.go", filepath.Join(target, "alias.go")); err != nil {
		t.Fatal(err)
	}
	first, err := targetIdentity(target)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(target, "alias.go")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("other.go", filepath.Join(target, "alias.go")); err != nil {
		t.Fatal(err)
	}
	second, err := targetIdentity(target)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("symlink identity change was not recorded")
	}
}
