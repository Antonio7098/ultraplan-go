package gitpublish

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPublisherCommitsOnlyOwnedPathsAndPreservesIndex(t *testing.T) {
	repo := initRepository(t)
	writeFile(t, filepath.Join(repo, "owned.txt"), "before\n")
	writeFile(t, filepath.Join(repo, "staged.txt"), "before\n")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "initial")

	writeFile(t, filepath.Join(repo, "owned.txt"), "after\n")
	writeFile(t, filepath.Join(repo, "new.txt"), "new\n")
	writeFile(t, filepath.Join(repo, "staged.txt"), "user staged\n")
	git(t, repo, "add", "staged.txt")

	result, err := New(Policy{Mode: ModeCommit}).Publish(context.Background(), Request{
		Root: repo, Paths: []string{filepath.Join(repo, "owned.txt"), "new.txt"},
		Message: "ultraplan: test stage", Identity: "test-stage",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Committed || result.Pushed || result.Commit == "" {
		t.Fatalf("unexpected result: %#v", result)
	}
	changed := strings.Fields(git(t, repo, "show", "--pretty=format:", "--name-only", "HEAD"))
	if strings.Join(changed, ",") != "new.txt,owned.txt" {
		t.Fatalf("committed paths = %v", changed)
	}
	if got := strings.TrimSpace(git(t, repo, "diff", "--cached", "--name-only")); got != "staged.txt" {
		t.Fatalf("staged user path = %q", got)
	}
	if body := git(t, repo, "log", "-1", "--pretty=%B"); !strings.Contains(body, "UltraPlan-Publication: test-stage") {
		t.Fatalf("missing publication trailer: %s", body)
	}
}

func TestPublisherRetriesPushWithoutDuplicateCommit(t *testing.T) {
	repo := initRepository(t)
	writeFile(t, filepath.Join(repo, "owned.txt"), "before\n")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "initial")
	writeFile(t, filepath.Join(repo, "owned.txt"), "after\n")
	git(t, repo, "remote", "add", "origin", filepath.Join(t.TempDir(), "missing.git"))

	publisher := New(Policy{Mode: ModeCommitAndPush, Remote: "origin", PushTimeout: 5 * time.Second})
	first, err := publisher.Publish(context.Background(), Request{Root: repo, Paths: []string{"owned.txt"}, Message: "ultraplan: test push", Identity: "push-stage"})
	if err == nil || !first.Committed || first.Pushed {
		t.Fatalf("first publish = %#v, %v", first, err)
	}
	count := git(t, repo, "rev-list", "--count", "HEAD")

	remote := filepath.Join(t.TempDir(), "remote.git")
	if output, initErr := exec.Command("git", "init", "--bare", remote).CombinedOutput(); initErr != nil {
		t.Fatalf("init bare: %s: %v", output, initErr)
	}
	git(t, repo, "remote", "set-url", "origin", remote)
	second, err := publisher.Publish(context.Background(), Request{Root: repo, Paths: []string{"owned.txt"}, Message: "ultraplan: test push", Identity: "push-stage"})
	if err != nil {
		t.Fatal(err)
	}
	if second.Committed || !second.Pushed || second.Commit != first.Commit {
		t.Fatalf("retry publish = %#v, first = %#v", second, first)
	}
	if got := git(t, repo, "rev-list", "--count", "HEAD"); got != count {
		t.Fatalf("commit count changed from %s to %s", count, got)
	}
	if got := strings.TrimSpace(git(t, remote, "rev-parse", "refs/heads/main")); got != first.Commit {
		t.Fatalf("remote commit = %q, want %q", got, first.Commit)
	}
}

func initRepository(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	git(t, repo, "init", "-b", "main")
	git(t, repo, "config", "user.name", "UltraPlan Test")
	git(t, repo, "config", "user.email", "ultraplan@example.test")
	return repo
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "LC_ALL=C")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %s: %v", strings.Join(args, " "), output, err)
	}
	return strings.TrimSpace(string(output))
}
