package app

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunHelp(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{name: "no args", args: nil},
		{name: "long help", args: []string{"--help"}},
		{name: "short help", args: []string{"-h"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, status := runForTest(tc.args)

			if status != ExitOK {
				t.Fatalf("status = %d, want %d", status, ExitOK)
			}
			if stderr != "" {
				t.Fatalf("stderr = %q, want empty", stderr)
			}
			assertContains(t, stdout, "ultraplan")
			assertContains(t, stdout, "Usage:")
			assertContains(t, stdout, "version")

			for _, deferred := range []string{
				"workspace",
				"config",
				"health",
				"study",
				"runtime",
				"summary",
				"validation",
				"code",
				"target",
				"sprint",
			} {
				assertNotContains(t, stdout, deferred)
			}
		})
	}
}

func TestRunVersion(t *testing.T) {
	stdout, stderr, status := runForTest([]string{"version"})

	if status != ExitOK {
		t.Fatalf("status = %d, want %d", status, ExitOK)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}

	for _, field := range []string{
		"Version: 1.2.3-test",
		"Commit: abc123",
		"BuildDate: 2026-05-30",
		"GoVersion: go-test",
	} {
		assertContains(t, stdout, field)
	}
}

func TestRunUnknownCommand(t *testing.T) {
	stdout, stderr, status := runForTest([]string{"definitely-unknown"})

	if status != ExitUsage {
		t.Fatalf("status = %d, want %d", status, ExitUsage)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
	assertContains(t, stderr, `unknown command "definitely-unknown"`)
	assertContains(t, stderr, "ultraplan --help")
}

func runForTest(args []string) (string, string, int) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	status := Run(Config{
		Args:   args,
		Stdout: &stdout,
		Stderr: &stderr,
		Version: Version{
			Version:   "1.2.3-test",
			Commit:    "abc123",
			BuildDate: "2026-05-30",
			GoVersion: "go-test",
		},
	})

	return stdout.String(), stderr.String(), status
}

func assertContains(t *testing.T, got, want string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Fatalf("expected %q to contain %q", got, want)
	}
}

func assertNotContains(t *testing.T, got, unwanted string) {
	t.Helper()
	if strings.Contains(got, unwanted) {
		t.Fatalf("expected %q not to contain %q", got, unwanted)
	}
}
