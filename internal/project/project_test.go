package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscoverProjectsFiltersAndSortsDirectSafeDirectories(t *testing.T) {
	root := workspaceFixture(t)
	mkdirAll(t, root, "projects", "zeta")
	mkdirAll(t, root, "projects", "alpha")
	mkdirAll(t, root, "projects", ".hidden")
	mkdirAll(t, root, "projects", "bad name")
	mkdirAll(t, root, "projects", "nested", "child")
	writeFile(t, root, "projects", "file")

	projects, err := DiscoverProjects(root)
	if err != nil {
		t.Fatal(err)
	}
	if got := names(projects); len(got) != 3 || got[0] != "alpha" || got[1] != "nested" || got[2] != "zeta" {
		t.Fatalf("projects = %#v", got)
	}
}

func TestResolveProjectRejectsInvalidAndReportsMissingAmbiguous(t *testing.T) {
	projects := []Project{{Name: "api"}, {Name: "api-v2"}}
	if _, err := ResolveProject(projects, "../api"); err == nil {
		t.Fatalf("expected invalid ref error")
	}
	if _, err := ResolveProject(projects, "missing"); err == nil || err.Error() != `project reference "missing" not found; available: api, api-v2` {
		t.Fatalf("missing err = %v", err)
	}
	if _, err := ResolveProject(projects, "ap"); err == nil || err.Error() != `ambiguous project reference "ap"; matches: api, api-v2` {
		t.Fatalf("ambiguous err = %v", err)
	}
}

func TestStatusAndValidateProjectFixture(t *testing.T) {
	root := workspaceFixture(t)
	writeValidProject(t, root, "ultraplan-go")

	service := NewService(root)
	status, err := service.Status("ultra")
	if err != nil {
		t.Fatal(err)
	}
	if status.Project.Name != "ultraplan-go" || status.DocsDir != StatusPresent || status.Roadmap != StatusPresent || status.ProjectIndex != StatusPresent || status.SprintsDir != StatusPresent || status.Catalog != StatusOK {
		t.Fatalf("unexpected status: %+v", status)
	}
	if len(status.MarkdownDocs) != 2 || status.MarkdownDocs[0] != "docs/ARCHITECTURE.md" || len(status.SprintDirs) != 1 {
		t.Fatalf("unexpected docs/sprints: %+v", status)
	}

	result, err := service.Validate("ultraplan-go")
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StatusOK || len(result.Findings) != 0 {
		t.Fatalf("validation = %+v", result)
	}
}

func TestValidateFindsMissingFilesMalformedRowsAndEscapes(t *testing.T) {
	root := workspaceFixture(t)
	base := filepath.Join(root, "projects", "broken")
	mkdirAll(t, base, "docs")
	mkdirAll(t, base, "sprints")
	writeFileContent(t, base, indexWithRows(
		"| Document | Path | Summary |",
		"| Product Requirements | projects/broken/docs/PRD.md | ok |",
		"| Missing Path | projects/broken/docs/MISSING.md | missing |",
		"| Escape | ../outside.md | unsafe |",
		"| No Path |  | bad |",
	), "project-index.md")
	writeFileContent(t, base, "# PRD\n", "docs", "PRD.md")
	writeFileContent(t, base, "# Roadmap\n", "roadmap.md")

	result, err := NewService(root).Validate("broken")
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StatusInvalid {
		t.Fatalf("status = %s", result.Status)
	}
	assertFinding(t, result.Findings, "catalog path not found")
	assertFinding(t, result.Findings, "catalog path escapes workspace")
	assertFinding(t, result.Findings, "malformed catalog row")
}

func TestValidateFindsEmptyDocs(t *testing.T) {
	root := workspaceFixture(t)
	base := filepath.Join(root, "projects", "empty-docs")
	mkdirAll(t, base, "docs")
	mkdirAll(t, base, "sprints")
	writeFileContent(t, base, "# Roadmap\n", "roadmap.md")
	writeFileContent(t, base, "# Project Index\n", "project-index.md")

	result, err := NewService(root).Validate("empty-docs")
	if err != nil {
		t.Fatal(err)
	}
	assertFinding(t, result.Findings, "empty docs directory")
}

func TestParseProjectIndexRecognizesCatalogSectionsAndExternalURLs(t *testing.T) {
	index, findings := ParseProjectIndex(`# Project Index

## Source Documents
| Document | Path | Summary |
|---|---|---|
| Product Requirements | docs/PRD.md | Product goals |

## Active Contract Pool
| Contract | Path | Applies To | Selection Notes |
|---|---|---|---|
| Architecture | .ultra/system/contracts/core/architecture.md | All | Notes |

## Available Evidence Reports
| Report | Path | Covers |
|---|---|---|
| report | https://example.com/report.md | external |

## Available Reasoning Templates
| Template | Path | Useful For | Status |
|---|---|---|---|
| Architecture | templates/architecture.md | Architecture | Current |

## Review Protocols
| Protocol | Path | Required When |
|---|---|---|
| Sprint Review | protocols/sprint.md | Every sprint |
`)
	if len(findings) != 0 {
		t.Fatalf("findings = %+v", findings)
	}
	if len(index.Entries) != 5 {
		t.Fatalf("entries = %+v", index.Entries)
	}
	if !index.Entries[2].External {
		t.Fatalf("expected URL entry to be external: %+v", index.Entries[2])
	}
}

func workspaceFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeFileContent(t, root, "version: 1\n", "ultraplan.yml")
	mkdirAll(t, root, "projects")
	return root
}

func writeValidProject(t *testing.T, root, name string) {
	t.Helper()
	base := filepath.Join(root, "projects", name)
	mkdirAll(t, base, "docs")
	mkdirAll(t, base, "sprints", "16-project-domain-and-index")
	for _, rel := range []string{
		"docs/PRD.md",
		"docs/ARCHITECTURE.md",
		"roadmap.md",
		"contracts/architecture.md",
		"studies/report.md",
		"templates/architecture.md",
		"protocols/sprint.md",
	} {
		writeFileContent(t, base, "# ok\n", strings.Split(rel, "/")...)
	}
	writeFileContent(t, base, indexWithRows(
		"| Document | Path | Summary |",
		"| Product Requirements | projects/"+name+"/docs/PRD.md | Product goals |",
		"",
		"## Active Contract Pool",
		"| Contract | Path | Applies To | Selection Notes |",
		"| Architecture | projects/"+name+"/contracts/architecture.md | All | Notes |",
		"",
		"## Available Evidence Reports",
		"| Report | Path | Covers |",
		"| report | projects/"+name+"/studies/report.md | Evidence |",
		"",
		"## Available Reasoning Templates",
		"| Template | Path | Useful For | Status |",
		"| Architecture | projects/"+name+"/templates/architecture.md | Architecture | Current |",
		"",
		"## Review Protocols",
		"| Protocol | Path | Required When |",
		"| Sprint Review | projects/"+name+"/protocols/sprint.md | Every sprint |",
	), "project-index.md")
}

func indexWithRows(lines ...string) string {
	out := "# Project Index\n\n## Source Documents\n"
	for _, line := range lines {
		if line == "" {
			out += "\n"
			continue
		}
		if len(line) >= 2 && line[:2] == "##" {
			out += line + "\n"
			continue
		}
		out += line + "\n"
		if line[0] == '|' && line[1] != '-' && line[2] != '-' {
			// separator rows are intentionally optional in fixtures.
		}
	}
	return out
}

func names(projects []Project) []string {
	out := make([]string, 0, len(projects))
	for _, p := range projects {
		out = append(out, p.Name)
	}
	return out
}

func assertFinding(t *testing.T, findings []ValidationFinding, problem string) {
	t.Helper()
	for _, finding := range findings {
		if finding.Problem == problem {
			return
		}
	}
	t.Fatalf("missing finding %q in %+v", problem, findings)
}

func mkdirAll(t *testing.T, base string, rel ...string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(append([]string{base}, rel...)...), 0o755); err != nil {
		t.Fatal(err)
	}
}

func writeFile(t *testing.T, base string, rel ...string) {
	t.Helper()
	writeFileContent(t, base, "test", rel...)
}

func writeFileContent(t *testing.T, base, content string, rel ...string) {
	t.Helper()
	path := filepath.Join(append([]string{base}, rel...)...)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
