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
	if !strings.Contains(req.Prompt, "cannot write files") || strings.Contains(req.Prompt, "repair code now") {
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
	for tool, action := range req.Policy.Tools {
		if action != "deny" {
			t.Fatalf("challenger tool %s = %q", tool, action)
		}
	}
	if len(req.Prompt) > qaMap.Budgets.PromptBytes || !strings.Contains(req.Prompt, "Do not change an outcome") {
		t.Fatalf("challenger prompt is not bounded or explicit: %s", req.Prompt)
	}
	if req.PromptRef.ID != "sprint.qa.challenger" || req.PromptRef.Purpose != "qa.challenger" || req.Metadata["prompt_id"] != req.PromptRef.ID {
		t.Fatalf("challenger prompt identity = %+v metadata=%+v", req.PromptRef, req.Metadata)
	}
}

func TestApprovedQACheckCatalogUsesExplicitReadOnlyArgv(t *testing.T) {
	target := t.TempDir()
	checks, err := ApprovedQAChecks(target, []string{"internal/a.go", "README.md"}, DefaultQABudgets())
	if err != nil {
		t.Fatal(err)
	}
	if len(checks) != 1 || checks[0].Executable != "gofmt" || len(checks[0].Args) != 2 || checks[0].Args[0] != "-d" || !validFingerprint(checks[0].Fingerprint) {
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
	seen   pprocess.Request
}

func (runner *qaProcessRunner) Run(_ context.Context, request pprocess.Request) (pprocess.Result, error) {
	runner.seen = request
	if runner.drift {
		if err := os.WriteFile(filepath.Join(runner.target, "drift.txt"), []byte("changed"), 0o600); err != nil {
			return pprocess.Result{}, err
		}
	}
	return pprocess.Result{ExitCode: 0, Stdout: "diff"}, nil
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
