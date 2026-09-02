package process

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestIsolationCopiesNonGitTreeRunsAndCleans(t *testing.T) {
	source := t.TempDir()
	if err := os.Mkdir(filepath.Join(source, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "nested", "dirty.txt"), []byte("untracked\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	parent := t.TempDir()
	limits := IsolationLimits{MaxFiles: 10, MaxBytes: 1024, MaxFileSize: 512, Timeout: time.Second}
	workspace, err := CreateIsolation(context.Background(), IsolationRequest{SourceRoot: source, ParentDir: parent, Prefix: "qa", Limits: limits})
	if err != nil {
		t.Fatal(err)
	}
	if workspace.Source.Files != 1 || workspace.Source.Digest == "" {
		t.Fatalf("unexpected identity: %+v", workspace.Source)
	}
	data, err := os.ReadFile(filepath.Join(workspace.Path, "nested", "dirty.txt"))
	if err != nil || string(data) != "untracked\n" {
		t.Fatalf("copied data = %q, %v", data, err)
	}
	copiedInfo, err := os.Stat(filepath.Join(workspace.Path, "nested", "dirty.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if copiedInfo.Mode().Perm() != 0o644 {
		t.Fatalf("copied mode = %v", copiedInfo.Mode().Perm())
	}
	result, err := workspace.Run(context.Background(), DirectRunner{}, "nested", Request{Executable: "sh", Args: []string{"-c", "printf isolated"}, Env: SortedEnvironment(map[string]string{"PATH": os.Getenv("PATH")}), Timeout: time.Second})
	if err != nil || result.Stdout != "isolated" {
		t.Fatalf("run result = %+v, %v", result, err)
	}
	changed, err := workspace.ChangedPaths(context.Background(), limits)
	if err != nil || len(changed) != 0 {
		t.Fatalf("read-only workspace changes = %v, %v", changed, err)
	}
	cleanup := workspace.Cleanup()
	if !cleanup.Attempted || !cleanup.Complete {
		t.Fatalf("cleanup = %+v", cleanup)
	}
}

func TestIsolationCanRecreateOneFixedWorkerPathFromCurrentSource(t *testing.T) {
	source := t.TempDir()
	parent := t.TempDir()
	destination := filepath.Join(parent, "workspace")
	limits := IsolationLimits{MaxFiles: 10, MaxBytes: 1024, MaxFileSize: 512, Timeout: time.Second}
	write := func(value string) {
		if err := os.WriteFile(filepath.Join(source, "issue.txt"), []byte(value), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("first")
	first, err := CreateIsolation(context.Background(), IsolationRequest{SourceRoot: source, ParentDir: parent, Destination: destination, Limits: limits})
	if err != nil || first.Path != destination {
		t.Fatalf("first fixed isolation = %+v, %v", first, err)
	}
	if cleanup := first.Cleanup(); !cleanup.Complete {
		t.Fatalf("first cleanup = %+v", cleanup)
	}
	write("second")
	second, err := CreateIsolation(context.Background(), IsolationRequest{SourceRoot: source, ParentDir: parent, Destination: destination, Limits: limits})
	if err != nil || second.Path != destination {
		t.Fatalf("second fixed isolation = %+v, %v", second, err)
	}
	got, err := os.ReadFile(filepath.Join(second.Path, "issue.txt"))
	if err != nil || string(got) != "second" {
		t.Fatalf("recreated contents = %q, %v", got, err)
	}
}

func TestIsolationRejectsEscapeSymlinkSpecialHardlinkAndBudgets(t *testing.T) {
	limits := IsolationLimits{MaxFiles: 1, MaxBytes: 4, MaxFileSize: 4, Timeout: time.Second}
	for name, setup := range map[string]func(string) error{
		"symlink":  func(root string) error { return os.Symlink("../outside", filepath.Join(root, "bad")) },
		"oversize": func(root string) error { return os.WriteFile(filepath.Join(root, "bad"), []byte("12345"), 0o600) },
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			if err := setup(root); err != nil {
				t.Fatal(err)
			}
			_, err := CreateIsolation(context.Background(), IsolationRequest{SourceRoot: root, ParentDir: t.TempDir(), Limits: limits})
			if err == nil {
				t.Fatal("expected isolation rejection")
			}
		})
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a"), []byte("1"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(filepath.Join(root, "a"), filepath.Join(root, "b")); err == nil {
		_, copyErr := CreateIsolation(context.Background(), IsolationRequest{SourceRoot: root, ParentDir: t.TempDir(), Limits: IsolationLimits{MaxFiles: 3, MaxBytes: 10, MaxFileSize: 5, Timeout: time.Second}})
		if copyErr == nil || !strings.Contains(copyErr.Error(), "hard-linked") {
			t.Fatalf("hardlink error = %v", copyErr)
		}
	}
	workspace := IsolationWorkspace{Path: t.TempDir()}
	if _, err := workspace.Resolve("../escape"); err == nil {
		t.Fatal("expected path escape rejection")
	}
}

func TestIsolationCopiesContainedSymlinkAndIncludesItInIdentity(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "target"), []byte("ok"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("target", filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	limits := IsolationLimits{MaxFiles: 3, MaxBytes: 32, MaxFileSize: 16, Timeout: time.Second}
	workspace, err := CreateIsolation(context.Background(), IsolationRequest{SourceRoot: root, ParentDir: t.TempDir(), Limits: limits})
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.Readlink(filepath.Join(workspace.Path, "link"))
	if err != nil || got != "target" {
		t.Fatalf("copied link = %q, %v", got, err)
	}
	identity, err := IdentifyTree(context.Background(), workspace.Path, limits)
	if err != nil || identity != workspace.Source {
		t.Fatalf("identity = %+v, source = %+v, err = %v", identity, workspace.Source, err)
	}
}

func TestIsolationRejectsOverlappingParentAndCancellation(t *testing.T) {
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "a"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	limits := IsolationLimits{MaxFiles: 2, MaxBytes: 10, MaxFileSize: 10, Timeout: time.Second}
	if _, err := CreateIsolation(context.Background(), IsolationRequest{SourceRoot: source, ParentDir: filepath.Join(source, "copies"), Limits: limits}); err == nil {
		t.Fatal("expected overlapping parent rejection")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := CreateIsolation(ctx, IsolationRequest{SourceRoot: source, ParentDir: t.TempDir(), Limits: limits}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation error = %v", err)
	}
}

func TestIsolationNativeProtectedRootDeny(t *testing.T) {
	source := t.TempDir()
	protected := filepath.Join(source, "protected.txt")
	if err := os.WriteFile(protected, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	workspace, err := CreateIsolation(context.Background(), IsolationRequest{
		SourceRoot: source,
		ParentDir:  t.TempDir(),
		Limits:     IsolationLimits{MaxFiles: 4, MaxBytes: 1024, MaxFileSize: 512, Timeout: time.Second},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer workspace.Cleanup()
	if !workspace.Capabilities.NativeProtectedRootDeny {
		t.Skip("native protected-root isolation is unavailable")
	}
	result, runErr := workspace.Run(context.Background(), DirectRunner{}, ".", Request{
		Executable: "sh",
		Args:       []string{"-c", "printf changed > \"$1\"", "sh", protected},
		Env:        SortedEnvironment(map[string]string{"PATH": os.Getenv("PATH")}),
		Timeout:    time.Second,
	})
	if runErr == nil || result.ExitCode == 0 {
		t.Fatalf("protected write unexpectedly succeeded: result=%+v err=%v", result, runErr)
	}
	data, readErr := os.ReadFile(protected)
	if readErr != nil || string(data) != "original" {
		t.Fatalf("protected source changed: %q, %v", data, readErr)
	}
}
