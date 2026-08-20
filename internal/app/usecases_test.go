package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestReadOnlyUseCasesDashboardAndPreview(t *testing.T) {
	dir := initializedWorkspace(t)
	projectRoot := filepath.Join(dir, "projects", "alpha")
	writeFixtureFile(t, projectRoot, "docs", "PRD.md")
	writeFixtureFile(t, projectRoot, "roadmap.md")
	writeFixtureFile(t, projectRoot, "project-index.md")
	sprintRoot := filepath.Join(projectRoot, "sprints", "01-foundation")
	writeFixtureFile(t, sprintRoot, "requirements.md")
	writeFixtureFile(t, sprintRoot, "sprint-index.md")
	writeFixtureFile(t, sprintRoot, "technical-handbook.md")
	writeFixtureFile(t, sprintRoot, "reasoning.md")
	writeFixtureFile(t, sprintRoot, "plan.md")

	useCases := NewReadOnlyUseCases(dir)
	dashboard, err := useCases.Dashboard(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(dashboard.Projects) != 1 || dashboard.Projects[0].Name != "alpha" {
		t.Fatalf("projects = %+v", dashboard.Projects)
	}
	if len(dashboard.Sprints) != 1 || dashboard.Sprints[0].Slug != "01-foundation" {
		t.Fatalf("sprints = %+v", dashboard.Sprints)
	}
	if !dashboard.Sprints[0].RefreshMayWrite {
		t.Fatalf("sprint refresh mutation note was not exposed")
	}

	preview, err := useCases.PreviewArtifact(context.Background(), "projects/alpha/project-index.md")
	if err != nil {
		t.Fatal(err)
	}
	if preview.Missing || preview.Error != "" {
		t.Fatalf("preview = %+v", preview)
	}

	escape, err := useCases.PreviewArtifact(context.Background(), "../secret.md")
	if err != nil {
		t.Fatal(err)
	}
	if escape.Error == "" {
		t.Fatalf("expected path escape rejection")
	}
}

func TestPreviewArtifactTruncatesAndValidatesJSON(t *testing.T) {
	dir := initializedWorkspace(t)
	projectRoot := filepath.Join(dir, "projects", "alpha", "sprints", "01-foundation")
	if err := os.MkdirAll(projectRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	large := make([]byte, PreviewByteLimit+10)
	for i := range large {
		large[i] = 'x'
	}
	if err := os.WriteFile(filepath.Join(projectRoot, "flow-state.json"), large, 0o644); err != nil {
		t.Fatal(err)
	}
	useCases := NewReadOnlyUseCases(dir)
	preview, err := useCases.PreviewArtifact(context.Background(), "projects/alpha/sprints/01-foundation/flow-state.json")
	if err != nil {
		t.Fatal(err)
	}
	if !preview.Truncated || !preview.Invalid {
		t.Fatalf("preview = %+v", preview)
	}
}

func TestPreviewArtifactRejectsSymlinkedAllowlistedPath(t *testing.T) {
	dir := initializedWorkspace(t)
	projectRoot := filepath.Join(dir, "projects", "alpha")
	if err := os.MkdirAll(projectRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(dir, "unrelated.md")
	if err := os.WriteFile(secret, []byte("do not disclose\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, filepath.Join(projectRoot, "project-index.md")); err != nil {
		t.Fatal(err)
	}

	preview, err := NewReadOnlyUseCases(dir).PreviewArtifact(context.Background(), "projects/alpha/project-index.md")
	if err != nil {
		t.Fatal(err)
	}
	if preview.Error == "" || preview.Content != "" {
		t.Fatalf("symlink preview = %+v", preview)
	}
}

func TestPreviewSupportsStudyArtifacts(t *testing.T) {
	dir := initializedWorkspace(t)
	studyRoot := filepath.Join(dir, "studies", "research")
	writeFixtureFileContent(t, studyRoot, "# Dimension\n", "dimensions", "01-structure.md")
	writeFixtureFileContent(t, studyRoot, "# Source\n", "sources", "brief.md")
	writeFixtureFileContent(t, studyRoot, `{"schema_version":1}`+"\n", ".ultraplan", "run-state.json")

	useCases := NewReadOnlyUseCases(dir)
	for _, rel := range []string{
		"studies/research/dimensions/01-structure.md",
		"studies/research/sources/brief.md",
		"studies/research/.ultraplan/run-state.json",
	} {
		preview, err := useCases.PreviewArtifact(context.Background(), rel)
		if err != nil {
			t.Fatal(err)
		}
		if preview.Error != "" || preview.Missing {
			t.Fatalf("%s preview = %+v", rel, preview)
		}
	}
}
