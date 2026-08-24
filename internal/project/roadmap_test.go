package project

import (
	"os"
	"path/filepath"
	"testing"
)

const validRoadmapFixture = `# Alpha Roadmap

> Project: ` + "`alpha`" + `
> Scope: prove the governed roadmap structure.

## Phase 1: Foundation

Intro prose is allowed here.

### Sprint 1: First

> Slug: 01-first
> Status: delivered
> Depends On:

#### Goal

Establish the foundation.

#### Build

- module shell
- health command

#### Acceptance

- [ ] tests pass

### Sprint 2: Second

> Slug: 02-second
> Status: active
> Depends On: 1

#### Uncertainty

> Can it scale?

#### Goal

Extend the foundation.

#### Build

- cancellation support

#### Acceptance

- [ ] races pass

## Contract Gates

Document-level sections without sprints are allowed.
`

func parseFixture(t *testing.T, content string) (Roadmap, []RoadmapIssue) {
	t.Helper()
	roadmap, issues := ParseRoadmap(content)
	return roadmap, issues
}

func TestParseRoadmapAcceptsStructuredRoadmap(t *testing.T) {
	roadmap, issues := parseFixture(t, validRoadmapFixture)
	if len(issues) != 0 {
		t.Fatalf("issues = %+v", issues)
	}
	if len(roadmap.Phases) != 1 || roadmap.Phases[0].Title != "Phase 1: Foundation" {
		t.Fatalf("phases = %+v", roadmap.Phases)
	}
	if len(roadmap.Sprints) != 2 {
		t.Fatalf("sprints = %+v", roadmap.Sprints)
	}
	first := roadmap.Sprints[0]
	if first.Number != 1 || first.Title != "First" || first.Slug != "01-first" || first.Status != RoadmapDelivered {
		t.Fatalf("first = %+v", first)
	}
	if first.Goal != "Establish the foundation." {
		t.Fatalf("first goal = %q", first.Goal)
	}
	if len(first.GateItems) != 1 || first.GateItems[0] != "tests pass" {
		t.Fatalf("first gate items = %#v", first.GateItems)
	}
	second := roadmap.Sprints[1]
	if second.Number != 2 || second.Slug != "02-second" || second.Status != RoadmapActive || len(second.DependsOn) != 1 || second.DependsOn[0] != 1 {
		t.Fatalf("second = %+v", second)
	}
	if second.Goal != "Extend the foundation." {
		t.Fatalf("second goal = %q", second.Goal)
	}
	if len(second.GateItems) != 1 || second.GateItems[0] != "races pass" {
		t.Fatalf("second gate items = %#v", second.GateItems)
	}
}

func TestMarkRoadmapSprintDelivered(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "roadmap.md")
	if err := os.WriteFile(path, []byte(validRoadmapFixture), 0o640); err != nil {
		t.Fatal(err)
	}
	changed, err := MarkRoadmapSprintDelivered(path, "02-second")
	if err != nil || !changed {
		t.Fatalf("changed=%t err=%v", changed, err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	roadmap, issues := ParseRoadmap(string(data))
	if len(issues) != 0 || roadmap.Sprints[1].Status != RoadmapDelivered {
		t.Fatalf("roadmap=%+v issues=%+v", roadmap, issues)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("mode=%v", info.Mode().Perm())
	}
	changed, err = MarkRoadmapSprintDelivered(path, "02-second")
	if err != nil || changed {
		t.Fatalf("idempotent changed=%t err=%v", changed, err)
	}
}

func TestParseRoadmapIgnoresFencedContent(t *testing.T) {
	fenced := "```text\n### Sprint 99: Not A Sprint\n#### Goal\n```\n"
	roadmap, issues := parseFixture(t, validRoadmapFixture+"\n"+fenced)
	if len(issues) != 0 {
		t.Fatalf("issues = %+v", issues)
	}
	for _, sprint := range roadmap.Sprints {
		if sprint.Number == 99 {
			t.Fatalf("fenced sprint was parsed: %+v", sprint)
		}
	}
}

func TestParseRoadmapReportsMissingSubsectionsAndSlug(t *testing.T) {
	content := `# Roadmap

## Phase 1: Only

### Sprint 1: Incomplete

> Status: planned

#### Build

- deliverable
`
	_, issues := parseFixture(t, content)
	for _, problem := range []string{"missing sprint slug", "sprint missing goal", "sprint missing acceptance gate"} {
		assertRoadmapIssue(t, issues, problem)
	}
}

func TestParseRoadmapAcceptsGateVariantsAndNotes(t *testing.T) {
	content := `# Roadmap

## Phase 1: Only

### Sprint 1: Gated

> Slug: 01-gated
> Status: delivered

#### Goal

Goal.

#### Build

- item

#### Release Gate

- [ ] release checks pass

#### Notes

Historical context.
`
	roadmap, issues := parseFixture(t, content)
	if len(issues) != 0 {
		t.Fatalf("issues = %+v", issues)
	}
	if len(roadmap.Sprints) != 1 || !roadmap.Sprints[0].HasGate {
		t.Fatalf("sprints = %+v", roadmap.Sprints)
	}
}

func TestParseRoadmapReportsDuplicateNumberSlugAndOrder(t *testing.T) {
	content := `# Roadmap

## Phase 1: Only

### Sprint 2: Later

> Slug: 02-later
> Status: planned

#### Goal

Goal.

#### Build

- item

#### Acceptance

- [ ] ok

### Sprint 2: Again

> Slug: 02-later
> Status: planned

#### Goal

Goal.

#### Build

- item

#### Acceptance

- [ ] ok

### Sprint 1: Out of order

> Slug: 01-out-of-order
> Status: planned

#### Goal

Goal.

#### Build

- item

#### Acceptance

- [ ] ok
`
	_, issues := parseFixture(t, content)
	assertRoadmapIssue(t, issues, "duplicate sprint number")
	assertRoadmapIssue(t, issues, "duplicate sprint slug")
	assertRoadmapIssue(t, issues, "sprint numbers out of order")
}

func TestParseRoadmapRejectsUnknownStatusMetadataAndSubsection(t *testing.T) {
	content := `# Roadmap

## Phase 1: Only

### Sprint 1: Odd

> Slug: 01-odd
> Status: finished
> Owner: someone

#### Goal

Goal.

#### Remarks

Random notes.

#### Build

- item

#### Acceptance

- [ ] ok
`
	_, issues := parseFixture(t, content)
	assertRoadmapIssue(t, issues, "invalid sprint status")
	assertRoadmapIssue(t, issues, "unknown sprint metadata field")
	assertRoadmapIssue(t, issues, "unexpected sprint subsection")
}

func TestParseRoadmapReportsUnknownDependency(t *testing.T) {
	content := `# Roadmap

## Phase 1: Only

### Sprint 1: Lonely

> Slug: 01-lonely
> Status: planned
> Depends On: 9

#### Goal

Goal.

#### Build

- item

#### Acceptance

- [ ] ok
`
	_, issues := parseFixture(t, content)
	assertRoadmapIssue(t, issues, "unknown dependency")
}

func TestParseRoadmapRequiresPhaseBeforeSprint(t *testing.T) {
	content := `# Roadmap

### Sprint 1: Orphan

> Slug: 01-orphan
> Status: planned

#### Goal

Goal.

#### Build

- item

#### Acceptance

- [ ] ok
`
	roadmap, issues := parseFixture(t, content)
	assertRoadmapIssue(t, issues, "sprint section outside a phase")
	if len(roadmap.Sprints) != 0 {
		t.Fatalf("orphan sprint parsed: %+v", roadmap.Sprints)
	}
}

func TestValidateProjectChecksRoadmapAgainstSprintDirectories(t *testing.T) {
	root := workspaceFixture(t)
	writeValidProject(t, root, "alpha")

	result, err := NewService(root).Validate("alpha")
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StatusOK {
		t.Fatalf("validation = %+v", result)
	}

	base := filepath.Join(root, "projects", "alpha")
	original, err := os.ReadFile(filepath.Join(base, "roadmap.md"))
	if err != nil {
		t.Fatal(err)
	}
	writeFileContent(t, base, string(original), "roadmap.md")
	result, err = NewService(root).Validate("alpha")
	if err != nil {
		t.Fatal(err)
	}
	for _, finding := range result.Findings {
		if finding.Severity == SeverityError {
			t.Fatalf("unexpected error finding: %+v", finding)
		}
	}

	mkdirAll(t, base, "sprints", "03-orphan")
	result, err = NewService(root).Validate("alpha")
	if err != nil {
		t.Fatal(err)
	}
	assertFinding(t, result.Findings, "sprint directory missing from roadmap")
}

func TestValidateProjectStatusAwareSprintDirectoryChecks(t *testing.T) {
	root := workspaceFixture(t)
	base := filepath.Join(root, "projects", "alpha")
	mkdirAll(t, base, "docs")
	mkdirAll(t, base, "sprints", "02-shipped")
	writeFileContent(t, base, "# PRD\n", "docs", "PRD.md")
	writeFileContent(t, base, indexWithRows(
		"| Document | Path | Summary |",
		"| Product Requirements | projects/alpha/docs/PRD.md | Product goals |",
	), "project-index.md")

	roadmap := `# Alpha Roadmap

## Phase 1: Delivery

### Sprint 1: Missing

> Slug: 01-missing
> Status: delivered

#### Goal

Goal.

#### Build

- item

#### Acceptance

- [ ] ok

### Sprint 2: Active

> Slug: 03-active
> Status: active

#### Goal

Goal.

#### Build

- item

#### Acceptance

- [ ] ok

### Sprint 3: Shipped

> Slug: 02-shipped
> Status: delivered

#### Goal

Goal.

#### Build

- item

#### Release Gate

- [ ] ok
`
	writeFileContent(t, base, roadmap, "roadmap.md")
	result, err := NewService(root).Validate("alpha")
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StatusInvalid {
		t.Fatalf("status = %s findings = %+v", result.Status, result.Findings)
	}
	assertFinding(t, result.Findings, "active roadmap sprint directory missing")
	for _, finding := range result.Findings {
		if finding.Problem == "roadmap sprint directory absent" && finding.Severity != SeverityWarn {
			t.Fatalf("delivered absence should warn: %+v", finding)
		}
	}
}

func assertRoadmapIssue(t *testing.T, issues []RoadmapIssue, problem string) {
	t.Helper()
	for _, issue := range issues {
		if issue.Problem == problem {
			return
		}
	}
	t.Fatalf("missing issue %q in %+v", problem, issues)
}
