package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCodeCommandHelp(t *testing.T) {
	stdout, stderr, status := runForTest([]string{"code", "--help"})
	if status != ExitOK {
		t.Fatalf("status = %d stdout = %q stderr = %q", status, stdout, stderr)
	}
	assertContains(t, stdout, "ultraplan code")
	assertContains(t, stdout, "ultraplan code <report>... [--json] [--output <path>]")
	assertContains(t, stdout, "--json")
	assertContains(t, stdout, "--output <path>")
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
}

func TestCodeCommandExtractsTextAndJSON(t *testing.T) {
	dir := initializedWorkspace(t)
	writeFixtureFileContent(t, dir, "package main\n\nfunc main() {}\n", "studies", "demo", "sources", "repo", "main.go")
	report := filepath.Join(dir, "studies", "demo", "reports", "final", "01-structure.md")
	writeFixtureFileContent(t, filepath.Dir(report), "| # | Source | Path |\n| 1 | repo | `studies/demo/sources/repo` |\n\nSee `main.go:1-3`.\n", filepath.Base(report))

	stdout, stderr, status := runForTest([]string{"--workspace", dir, "code", report})
	if status != ExitOK {
		t.Fatalf("status = %d stdout = %q stderr = %q", status, stdout, stderr)
	}
	assertContains(t, stdout, "Reference: main.go:1-3")
	assertContains(t, stdout, "studies/demo/sources/repo/main.go")
	assertContains(t, stdout, "1: package main")
	assertContains(t, stdout, "Unresolved: 0")

	out := filepath.Join(dir, "extract.json")
	stdout, stderr, status = runForTest([]string{"--workspace", dir, "code", "--json", "--output", out, report})
	if status != ExitOK {
		t.Fatalf("json status = %d stdout = %q stderr = %q", status, stdout, stderr)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty when --output is used", stdout)
	}
	content, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Status  string `json:"status"`
		Reports []struct {
			References []struct {
				Status string `json:"status"`
			} `json:"references"`
		} `json:"reports"`
	}
	if err := json.Unmarshal(content, &payload); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, content)
	}
	if payload.Status != "ok" || payload.Reports[0].References[0].Status != "resolved" {
		t.Fatalf("unexpected payload: %+v", payload)
	}
}

func TestCodeCommandProcessesReportsInArgumentOrder(t *testing.T) {
	dir := initializedWorkspace(t)
	writeFixtureFileContent(t, dir, "first\n", "sources", "repo", "first.go")
	writeFixtureFileContent(t, dir, "second\n", "sources", "repo", "second.go")
	first := filepath.Join(dir, "reports", "first.md")
	second := filepath.Join(dir, "reports", "second.md")
	writeFixtureFileContent(t, filepath.Dir(first), "| # | Source | Path |\n| 1 | repo | `sources/repo` |\n\nSee `first.go:1`.\n", filepath.Base(first))
	writeFixtureFileContent(t, filepath.Dir(second), "| # | Source | Path |\n| 1 | repo | `sources/repo` |\n\nSee `second.go:1`.\n", filepath.Base(second))

	stdout, stderr, status := runForTest([]string{"--workspace", dir, "code", second, first})
	if status != ExitOK {
		t.Fatalf("status = %d stdout = %q stderr = %q", status, stdout, stderr)
	}
	secondIndex := strings.Index(stdout, "Report: reports/second.md")
	firstIndex := strings.Index(stdout, "Report: reports/first.md")
	if secondIndex < 0 || firstIndex < 0 || secondIndex >= firstIndex {
		t.Fatalf("reports not rendered in argument order:\n%s", stdout)
	}
}

func TestCodeCommandOutputWriteFailure(t *testing.T) {
	dir := initializedWorkspace(t)
	writeFixtureFileContent(t, dir, "package main\n", "sources", "repo", "main.go")
	report := filepath.Join(dir, "report.md")
	writeFixtureFileContent(t, dir, "| # | Source | Path |\n| 1 | repo | `sources/repo` |\n\nSee `main.go:1`.\n", "report.md")

	stdout, stderr, status := runForTest([]string{"--workspace", dir, "code", "--output", dir, report})
	if status != ExitWorkspace {
		t.Fatalf("status = %d stdout = %q stderr = %q", status, stdout, stderr)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
	assertContains(t, stderr, "code.output")
}

func TestCodeCommandMapsUnresolvedAndMissingTable(t *testing.T) {
	dir := initializedWorkspace(t)
	report := filepath.Join(dir, "report.md")
	mkdirAll(t, dir, "sources", "repo")
	writeFixtureFileContent(t, dir, "| # | Source | Path |\n| 1 | repo | `sources/repo` |\n\nSee `nope.go:4`.\n", "report.md")

	stdout, stderr, status := runForTest([]string{"--workspace", dir, "code", report})
	if status != ExitPartial {
		t.Fatalf("status = %d stdout = %q stderr = %q", status, stdout, stderr)
	}
	assertContains(t, stdout, "Unresolved:")
	assertContains(t, stderr, "unresolved references")

	writeFixtureFileContent(t, dir, "See `main.go:1`.\n", "no-table.md")
	stdout, stderr, status = runForTest([]string{"--workspace", dir, "code", filepath.Join(dir, "no-table.md")})
	if status != ExitValidation {
		t.Fatalf("validation status = %d stdout = %q stderr = %q", status, stdout, stderr)
	}
	assertContains(t, stderr, "validation failed")
}

func TestCodeCommandUnreadableReportFailsFast(t *testing.T) {
	dir := initializedWorkspace(t)
	writeFixtureFileContent(t, dir, "package main\n", "sources", "repo", "main.go")
	readable := filepath.Join(dir, "readable.md")
	missing := filepath.Join(dir, "missing.md")
	writeFixtureFileContent(t, dir, "| # | Source | Path |\n| 1 | repo | `sources/repo` |\n\nSee `main.go:1`.\n", "readable.md")

	stdout, stderr, status := runForTest([]string{"--workspace", dir, "code", missing, readable})
	if status != ExitWorkspace {
		t.Fatalf("status = %d stdout = %q stderr = %q", status, stdout, stderr)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
	assertContains(t, stderr, "code.extract")
	assertContains(t, stderr, missing)
}
