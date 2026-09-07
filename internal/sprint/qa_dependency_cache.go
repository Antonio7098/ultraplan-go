package sprint

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Antonio7098/ultraplan-go/internal/platform/config"
	pprocess "github.com/Antonio7098/ultraplan-go/internal/platform/process"
)

// The trusted preparation step writes the cache in a disposable isolation. Test
// processes only see the published directory through native read-only isolation.
func prepareQADependencyCache(ctx context.Context, req QAReproductionRequest) (string, string, error) {
	if filepath.Base(req.Spec.Command.Executable) != "go" || !qaHasModule(req.TargetRoot, req.Spec.Command.WorkingDirectory) {
		return "", "", nil
	}
	if err := validateQARuntimeLocation(append(req.ProtectedRoots, req.TargetRoot)); err != nil {
		return "", "", err
	}
	executable, err := exec.LookPath(req.Spec.Command.Executable)
	if err != nil {
		return "", "", err
	}
	versionCommand := exec.CommandContext(ctx, executable, "version")
	versionCommand.Env = append(os.Environ(), "GOTOOLCHAIN=local")
	version, err := versionCommand.Output()
	if err != nil {
		return "", "", err
	}
	// Include implementation identity because local replacements and workspace
	// modules may change independently of the root go.mod and go.sum.
	digest := sha256.Sum256([]byte(req.ExpectedTargetID + "\n" + string(version) + "\n" + qaModuleDirectory(req.TargetRoot, req.Spec.Command.WorkingDirectory)))
	key := hex.EncodeToString(digest[:])
	base := filepath.Join(qaRuntimeRoot(), "modules")
	if err := os.MkdirAll(base, 0o700); err != nil {
		return "", "", err
	}
	unlock, err := qaStorageLockContext(ctx, filepath.Join(base, key+".lock"))
	if err != nil {
		return "", "", err
	}
	defer unlock()
	cache := filepath.Join(base, key)
	if _, err := os.Stat(filepath.Join(cache, ".ready")); err == nil {
		return cache, strings.TrimSpace(string(version)), nil
	}
	release, err := reserveQAResources(ctx, req.Budgets.TreeBytes+(2<<30), 0)
	if err != nil {
		return "", "", err
	}
	defer release()
	parent, err := qaRuntimeTemp("ultraplan-qa-dependencies-")
	if err != nil {
		return "", "", err
	}
	defer os.RemoveAll(parent)
	limits := pprocess.IsolationLimits{MaxFiles: req.Budgets.TreeFiles, MaxBytes: req.Budgets.TreeBytes, MaxFileSize: req.Budgets.FileBytes, Timeout: req.Budgets.ShardTimeout}
	workspace, err := pprocess.CreateIsolation(ctx, pprocess.IsolationRequest{SourceRoot: req.TargetRoot, ParentDir: parent, Prefix: "prepare", ProtectedRoots: append(req.ProtectedRoots, req.TargetRoot), Limits: limits})
	if err != nil {
		return "", "", err
	}
	defer workspace.Cleanup()
	if !workspace.Capabilities.NativeProtectedRootDeny {
		return "", "", fmt.Errorf("dependency preparation requires native isolation")
	}
	runtimeRoot := filepath.Join(workspace.Path, ".ultraplan-dependency-runtime")
	for _, name := range []string{"modules", "cache", "tmp", "home"} {
		if err := os.MkdirAll(filepath.Join(runtimeRoot, name), 0o700); err != nil {
			return "", "", err
		}
	}
	moduleCache := filepath.Join(runtimeRoot, "modules")
	env := map[string]string{"PATH": os.Getenv("PATH"), "HOME": filepath.Join(runtimeRoot, "home"), "GOMODCACHE": moduleCache, "GOCACHE": filepath.Join(runtimeRoot, "cache"), "GOTMPDIR": filepath.Join(runtimeRoot, "tmp"), "TMPDIR": filepath.Join(runtimeRoot, "tmp"), "GOTOOLCHAIN": "local"}
	workdir := req.Spec.Command.WorkingDirectory
	if workdir == "" {
		workdir = "."
	}
	for _, args := range [][]string{{"mod", "download"}, {"mod", "verify"}} {
		result, runErr := workspace.Run(ctx, pprocess.DirectRunner{}, workdir, pprocess.Request{Executable: executable, Args: args, Env: pprocess.SortedEnvironment(env), Timeout: req.Budgets.CommandTimeout, StdoutLimit: req.Budgets.CommandOutputBytes, StderrLimit: req.Budgets.CommandOutputBytes, CleanupGrace: req.Budgets.CleanupTimeout})
		if runErr != nil || result.ExitCode != 0 || result.TimedOut || result.Cancelled || !result.CleanupComplete {
			return "", "", fmt.Errorf("dependency preparation: %s: %w", config.RedactText(result.Stderr+"\n"+result.Stdout), errorsOrFailure(runErr))
		}
	}
	if err := os.WriteFile(filepath.Join(moduleCache, ".ready"), version, 0o600); err != nil {
		return "", "", err
	}
	if err := os.Rename(moduleCache, cache); err != nil {
		return "", "", err
	}
	return cache, strings.TrimSpace(string(version)), nil
}

func errorsOrFailure(err error) error {
	if err != nil {
		return err
	}
	return fmt.Errorf("command failed")
}

func qaModuleDirectory(target, workdir string) string {
	dir := filepath.Join(target, workdir)
	for inside(target, dir) {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			rel, _ := filepath.Rel(target, dir)
			return rel
		}
		if dir == target {
			break
		}
		dir = filepath.Dir(dir)
	}
	return workdir
}

func qaHasModule(target, workdir string) bool {
	dir := filepath.Join(target, qaModuleDirectory(target, workdir))
	_, err := os.Stat(filepath.Join(dir, "go.mod"))
	return err == nil
}
