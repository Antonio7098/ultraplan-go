package gitpublish

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Mode string

const (
	ModeOff           Mode = "off"
	ModeCommit        Mode = "commit"
	ModeCommitAndPush Mode = "commit-and-push"
)

type Policy struct {
	Mode        Mode
	Remote      string
	PushTimeout time.Duration
}

type Request struct {
	Root     string
	Paths    []string
	All      bool
	Message  string
	Identity string
}

type Result struct {
	Repository string `json:"repository"`
	Branch     string `json:"branch"`
	Commit     string `json:"commit,omitempty"`
	Remote     string `json:"remote,omitempty"`
	Committed  bool   `json:"committed"`
	Pushed     bool   `json:"pushed"`
	Skipped    bool   `json:"skipped"`
}

type Publisher interface {
	Publish(context.Context, Request) (Result, error)
}

type CommandPublisher struct {
	policy Policy
}

func New(policy Policy) CommandPublisher {
	if policy.Mode == "" {
		policy.Mode = ModeOff
	}
	if strings.TrimSpace(policy.Remote) == "" {
		policy.Remote = "origin"
	}
	if policy.PushTimeout <= 0 {
		policy.PushTimeout = 2 * time.Minute
	}
	return CommandPublisher{policy: policy}
}

func (p CommandPublisher) Publish(ctx context.Context, req Request) (Result, error) {
	if p.policy.Mode == ModeOff {
		return Result{Skipped: true}, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	root := filepath.Clean(strings.TrimSpace(req.Root))
	if root == "" || root == "." {
		return Result{}, fmt.Errorf("git publish: repository root is required")
	}
	repo, err := gitOutput(ctx, root, nil, "rev-parse", "--show-toplevel")
	if err != nil {
		return Result{}, fmt.Errorf("git publish: resolve repository from %s: %w", root, err)
	}
	repo, err = filepath.Abs(strings.TrimSpace(repo))
	if err != nil {
		return Result{}, fmt.Errorf("git publish: resolve repository path: %w", err)
	}
	branch, err := gitOutput(ctx, repo, nil, "branch", "--show-current")
	if err != nil {
		return Result{}, fmt.Errorf("git publish: resolve branch: %w", err)
	}
	branch = strings.TrimSpace(branch)
	if branch == "" {
		return Result{}, fmt.Errorf("git publish: detached HEAD is not supported")
	}
	result := Result{Repository: repo, Branch: branch}
	paths, err := publishPaths(repo, req.Paths, req.All)
	if err != nil {
		return result, err
	}
	common, err := gitOutput(ctx, repo, nil, "rev-parse", "--git-common-dir")
	if err != nil {
		return result, fmt.Errorf("git publish: resolve common directory: %w", err)
	}
	common = strings.TrimSpace(common)
	if !filepath.IsAbs(common) {
		common = filepath.Join(repo, common)
	}
	common, err = filepath.Abs(filepath.Clean(common))
	if err != nil {
		return result, fmt.Errorf("git publish: resolve common directory path: %w", err)
	}
	lock, err := acquireLock(ctx, filepath.Join(common, "ultraplan-publish.lock"))
	if err != nil {
		return result, fmt.Errorf("git publish: acquire repository lock: %w", err)
	}
	defer lock.release()

	if len(paths) > 0 {
		commit, committed, commitErr := commitPaths(ctx, repo, common, paths, req.Message, req.Identity)
		result.Commit, result.Committed = commit, committed
		if commitErr != nil {
			return result, commitErr
		}
	}
	if result.Commit == "" {
		result.Commit, err = gitOutput(ctx, repo, nil, "rev-parse", "HEAD")
		if err != nil {
			return result, fmt.Errorf("git publish: resolve HEAD: %w", err)
		}
		result.Commit = strings.TrimSpace(result.Commit)
	}
	if p.policy.Mode != ModeCommitAndPush {
		return result, nil
	}
	if !validRemoteName(p.policy.Remote) {
		return result, fmt.Errorf("git publish: invalid remote name %q", p.policy.Remote)
	}
	pushCtx, cancel := context.WithTimeout(ctx, p.policy.PushTimeout)
	defer cancel()
	upstreamRemote, upstreamErr := gitOutput(pushCtx, repo, nil, "for-each-ref", "--format=%(upstream:remotename)", "refs/heads/"+branch)
	if upstreamErr != nil {
		return result, fmt.Errorf("git publish: resolve upstream: %w", upstreamErr)
	}
	upstreamRemote = strings.TrimSpace(upstreamRemote)
	if upstreamRemote != "" {
		if !validRemoteName(upstreamRemote) {
			return result, fmt.Errorf("git publish: invalid upstream remote name %q", upstreamRemote)
		}
		upstreamRef, refErr := gitOutput(pushCtx, repo, nil, "for-each-ref", "--format=%(upstream:remoteref)", "refs/heads/"+branch)
		if refErr != nil || strings.TrimSpace(upstreamRef) == "" {
			return result, fmt.Errorf("git publish: resolve upstream ref: %w", refErr)
		}
		result.Remote = upstreamRemote
		_, err = gitOutput(pushCtx, repo, nil, "push", "--porcelain", "--", upstreamRemote, "HEAD:"+strings.TrimSpace(upstreamRef))
	} else {
		result.Remote = p.policy.Remote
		_, err = gitOutput(pushCtx, repo, nil, "push", "--porcelain", "--set-upstream", "--", p.policy.Remote, "HEAD:refs/heads/"+branch)
	}
	if err != nil {
		if errors.Is(pushCtx.Err(), context.DeadlineExceeded) {
			return result, fmt.Errorf("git publish: push timed out after %s: %w", p.policy.PushTimeout, pushCtx.Err())
		}
		return result, fmt.Errorf("git publish: push %s/%s: %w", result.Remote, branch, err)
	}
	result.Pushed = true
	return result, nil
}

func publishPaths(repo string, requested []string, all bool) ([]string, error) {
	if all {
		return []string{"."}, nil
	}
	set := map[string]bool{}
	for _, raw := range requested {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		path := filepath.Clean(raw)
		if !filepath.IsAbs(path) {
			path = filepath.Join(repo, path)
		}
		path, err := filepath.Abs(path)
		if err != nil {
			return nil, fmt.Errorf("git publish: resolve path %q: %w", raw, err)
		}
		rel, err := filepath.Rel(repo, path)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return nil, fmt.Errorf("git publish: path %q escapes repository %s", raw, repo)
		}
		set[filepath.ToSlash(rel)] = true
	}
	paths := make([]string, 0, len(set))
	for path := range set {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths, nil
}

func commitPaths(ctx context.Context, repo, common string, paths []string, message, identity string) (string, bool, error) {
	message = strings.TrimSpace(message)
	if message == "" {
		return "", false, fmt.Errorf("git publish: commit message is required")
	}
	if identity != "" {
		message += "\n\nUltraPlan-Publication: " + strings.TrimSpace(identity)
	}
	parent, err := gitOutput(ctx, repo, nil, "rev-parse", "HEAD")
	if err != nil {
		return "", false, fmt.Errorf("git publish: resolve commit parent: %w", err)
	}
	parent = strings.TrimSpace(parent)
	temp, err := os.CreateTemp(common, "ultraplan-index-*")
	if err != nil {
		return "", false, fmt.Errorf("git publish: create temporary index: %w", err)
	}
	indexPath := temp.Name()
	if closeErr := temp.Close(); closeErr != nil {
		_ = os.Remove(indexPath)
		return "", false, fmt.Errorf("git publish: close temporary index: %w", closeErr)
	}
	_ = os.Remove(indexPath)
	defer os.Remove(indexPath)
	env := []string{"GIT_INDEX_FILE=" + indexPath}
	if _, err := gitOutput(ctx, repo, env, "read-tree", parent); err != nil {
		return "", false, fmt.Errorf("git publish: seed temporary index: %w", err)
	}
	args := append([]string{"add", "-A", "--"}, paths...)
	if _, err := gitOutput(ctx, repo, env, args...); err != nil {
		return "", false, fmt.Errorf("git publish: stage owned paths: %w", err)
	}
	tree, err := gitOutput(ctx, repo, env, "write-tree")
	if err != nil {
		return "", false, fmt.Errorf("git publish: write tree: %w", err)
	}
	tree = strings.TrimSpace(tree)
	parentTree, err := gitOutput(ctx, repo, nil, "rev-parse", parent+"^{tree}")
	if err != nil {
		return "", false, fmt.Errorf("git publish: resolve parent tree: %w", err)
	}
	if tree == strings.TrimSpace(parentTree) {
		return parent, false, nil
	}
	commit, err := gitInput(ctx, repo, env, message+"\n", "commit-tree", tree, "-p", parent)
	if err != nil {
		return "", false, fmt.Errorf("git publish: create commit: %w", err)
	}
	commit = strings.TrimSpace(commit)
	ref, err := gitOutput(ctx, repo, nil, "symbolic-ref", "-q", "HEAD")
	if err != nil || strings.TrimSpace(ref) == "" {
		return "", false, fmt.Errorf("git publish: resolve branch ref: %w", err)
	}
	if _, err := gitOutput(ctx, repo, nil, "update-ref", strings.TrimSpace(ref), commit, parent); err != nil {
		return "", false, fmt.Errorf("git publish: branch changed while committing: %w", err)
	}
	resetArgs := append([]string{"reset", "-q", "HEAD", "--"}, paths...)
	if _, err := gitOutput(ctx, repo, nil, resetArgs...); err != nil {
		return commit, true, fmt.Errorf("git publish: reconcile index after commit: %w", err)
	}
	return commit, true, nil
}

func gitOutput(ctx context.Context, dir string, extraEnv []string, args ...string) (string, error) {
	return gitInput(ctx, dir, extraEnv, "", args...)
}

func gitInput(ctx context.Context, dir string, extraEnv []string, input string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	cmd.Env = gitEnvironment(extraEnv)
	if input != "" {
		cmd.Stdin = strings.NewReader(input)
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		detail := strings.TrimSpace(string(output))
		if detail == "" {
			detail = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), detail)
	}
	return strings.TrimSpace(string(output)), nil
}

func gitEnvironment(extra []string) []string {
	overrides := map[string]string{"GIT_TERMINAL_PROMPT": "0", "LC_ALL": "C"}
	for _, item := range extra {
		if key, value, ok := strings.Cut(item, "="); ok {
			overrides[key] = value
		}
	}
	env := make([]string, 0, len(os.Environ())+len(overrides))
	for _, item := range os.Environ() {
		key, _, _ := strings.Cut(item, "=")
		if _, replaced := overrides[key]; !replaced {
			env = append(env, item)
		}
	}
	for key, value := range overrides {
		env = append(env, key+"="+value)
	}
	return env
}

func validRemoteName(name string) bool {
	if name == "" || strings.HasPrefix(name, "-") {
		return false
	}
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || strings.ContainsRune("._/-", r) {
			continue
		}
		return false
	}
	return true
}
