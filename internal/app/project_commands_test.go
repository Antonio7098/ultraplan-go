package app

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectHelpIsRegistered(t *testing.T) {
	stdout, stderr, status := runForTest([]string{"--help"})
	if status != ExitOK || stderr != "" {
		t.Fatalf("status = %d stderr = %q", status, stderr)
	}
	assertContains(t, stdout, "project")

	for _, args := range [][]string{
		{"project", "--help"},
		{"project", "list", "--help"},
		{"project", "example", "status", "--help"},
		{"project", "example", "validate", "--help"},
	} {
		stdout, stderr, status = runForTest(args)
		if status != ExitOK || stderr != "" {
			t.Fatalf("%v status = %d stderr = %q", args, status, stderr)
		}
		assertContains(t, stdout, "ultraplan project")
	}
}

func TestProjectListStatusAndValidate(t *testing.T) {
	dir := initializedWorkspace(t)
	writeCommandProject(t, dir, "zeta")
	writeCommandProject(t, dir, "alpha")
	mkdirAll(t, dir, "projects", ".hidden")
	writeFixtureFile(t, dir, "projects", "not-a-project")

	stdout, stderr, status := runForTest([]string{"--workspace", dir, "project", "list"})
	if status != ExitOK || stderr != "" {
		t.Fatalf("status = %d stderr = %q", status, stderr)
	}
	assertContains(t, stdout, "Workspace: "+dir)
	assertInOrder(t, stdout, "  alpha\n", "  zeta\n")
	assertNotContains(t, stdout, ".hidden")

	stdout, stderr, status = runForTest([]string{"--workspace", dir, "project", "alp", "status"})
	if status != ExitOK || stderr != "" {
		t.Fatalf("status = %d stdout = %q stderr = %q", status, stdout, stderr)
	}
	assertContains(t, stdout, "Project: alpha")
	assertContains(t, stdout, "Markdown docs: 1")
	assertContains(t, stdout, "Reasoning defaults:")
	assertContains(t, stdout, "prompts/create-area-reasoning.md: builtin:prompts/create-area-reasoning.md")
	assertContains(t, stdout, "Project area reasoning documents: 0")
	assertContains(t, stdout, "Catalog: ok")

	stdout, stderr, status = runForTest([]string{"--workspace", dir, "project", "alpha", "validate"})
	if status != ExitOK || stderr != "" {
		t.Fatalf("status = %d stdout = %q stderr = %q", status, stdout, stderr)
	}
	assertContains(t, stdout, "Validation: ok")
	assertContains(t, stdout, "Findings: 0")
}

func TestProjectValidateFailureUsesExitFiveAndStderr(t *testing.T) {
	dir := initializedWorkspace(t)
	base := filepath.Join(dir, "projects", "broken")
	mkdirAll(t, base, "docs")
	writeFixtureFileContent(t, base, "# Roadmap\n", "roadmap.md")
	writeFixtureFileContent(t, base, "# Project Index\n\n## Source Documents\n| Document | Path | Summary |\n|---|---|---|\n| Missing | projects/broken/docs/MISSING.md | missing |\n", "project-index.md")

	stdout, stderr, status := runForTest([]string{"--workspace", dir, "project", "broken", "validate"})
	if status != ExitValidation {
		t.Fatalf("status = %d stdout = %q stderr = %q", status, stdout, stderr)
	}
	assertContains(t, stdout, "Validation: invalid")
	assertContains(t, stdout, "Findings:")
	assertContains(t, stderr, `section="Source Documents"`)
	assertContains(t, stderr, `entry="Missing"`)
	assertContains(t, stderr, `path="projects/broken/docs/MISSING.md"`)
	assertContains(t, stderr, `suggestion=`)
	if strings.Contains(stdout+stderr, "\x1b[") {
		t.Fatalf("unexpected ANSI escape sequence")
	}
}

func TestProjectMissingAndAmbiguousRefsAreValidationErrors(t *testing.T) {
	dir := initializedWorkspace(t)
	writeCommandProject(t, dir, "api")
	writeCommandProject(t, dir, "api-v2")

	stdout, stderr, status := runForTest([]string{"--workspace", dir, "project", "missing", "status"})
	if status != ExitValidation || stdout != "" {
		t.Fatalf("missing status = %d stdout = %q stderr = %q", status, stdout, stderr)
	}
	assertContains(t, stderr, `project reference "missing" not found`)

	stdout, stderr, status = runForTest([]string{"--workspace", dir, "project", "ap", "validate"})
	if status != ExitValidation || stdout != "" {
		t.Fatalf("ambiguous status = %d stdout = %q stderr = %q", status, stdout, stderr)
	}
	assertContains(t, stderr, `ambiguous project reference "ap"`)
}

func writeCommandProject(t *testing.T, root, name string) {
	t.Helper()
	base := filepath.Join(root, "projects", name)
	mkdirAll(t, base, "docs")
	mkdirAll(t, base, "sprints", "01-test")
	writeFixtureFileContent(t, base, "# PRD\n", "docs", "PRD.md")
	writeFixtureFileContent(t, base, structuredRoadmapFixture(name), "roadmap.md")
	writeFixtureFileContent(t, base, "# Contract\n", "contracts", "architecture.md")
	writeFixtureFileContent(t, base, indexWithRows(
		"| Document | Path | Summary |",
		"| Product Requirements | projects/"+name+"/docs/PRD.md | Product goals |",
		"",
		"## Active Contract Pool",
		"| Contract | Path | Applies To | Selection Notes |",
		"| Architecture | projects/"+name+"/contracts/architecture.md | All | Notes |",
	), "project-index.md")
}

func structuredRoadmapFixture(projectName string) string {
	return "# " + projectName + " Roadmap\n" + `
> Project: ` + "`" + projectName + "`" + `

## Phase 1: Delivery

### Sprint 1: Test Sprint

> Slug: 01-test
> Status: planned
> Depends On:

#### Goal

Prove the fixture project.

#### Build

- one deliverable

#### Acceptance

- [ ] fixture validates
`
}

func indexWithRows(lines ...string) string {
	out := "# Project Index\n\n## Source Documents\n"
	for _, line := range lines {
		if line == "" {
			out += "\n"
			continue
		}
		out += line + "\n"
	}
	return out
}
