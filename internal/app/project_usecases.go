package app

import (
	"context"
	"path/filepath"

	"github.com/Antonio7098/ultraplan-go/internal/project"
	"github.com/Antonio7098/ultraplan-go/internal/workspace"
)

type ProjectSummary struct {
	Name                   string
	DocsDir                string
	MarkdownDocs           []string
	Roadmap                string
	ProjectIndex           string
	SprintsDir             string
	Catalog                string
	ReasoningDefaults      []project.ReasoningDefault
	AreaReasoningDocuments []string
	Findings               []DisplayFinding
	Artifacts              []DisplayArtifact
}

func (u dashboardUseCases) ProjectSummaries(ctx context.Context) ([]ProjectSummary, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	service := project.NewService(u.root)
	projects, err := service.ListProjects()
	if err != nil {
		return nil, mapProjectError("project.list", err)
	}
	out := make([]ProjectSummary, 0, len(projects))
	for _, p := range projects {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		status, err := service.Status(p.Name)
		if err != nil {
			return nil, mapProjectError("project.status", err)
		}
		summary := ProjectSummary{
			Name:                   p.Name,
			DocsDir:                string(status.DocsDir),
			MarkdownDocs:           append([]string(nil), status.MarkdownDocs...),
			Roadmap:                string(status.Roadmap),
			ProjectIndex:           string(status.ProjectIndex),
			SprintsDir:             string(status.SprintsDir),
			Catalog:                string(status.Catalog),
			ReasoningDefaults:      append([]project.ReasoningDefault(nil), status.ReasoningDefaults...),
			AreaReasoningDocuments: append([]string(nil), status.AreaReasoningDocuments...),
			Artifacts: []DisplayArtifact{
				{Label: "project-index", Path: workspace.Rel(u.root, filepath.Join(p.Path, "project-index.md")), Kind: "markdown"},
				{Label: "roadmap", Path: workspace.Rel(u.root, filepath.Join(p.Path, "roadmap.md")), Kind: "markdown"},
			},
		}
		for _, doc := range status.MarkdownDocs {
			summary.Artifacts = append(summary.Artifacts, DisplayArtifact{Label: "doc", Path: workspace.Rel(u.root, filepath.Join(p.Path, doc)), Kind: "markdown"})
		}
		for _, doc := range status.AreaReasoningDocuments {
			summary.Artifacts = append(summary.Artifacts, DisplayArtifact{Label: "area-reasoning", Path: doc, Kind: "markdown"})
		}
		for _, f := range status.ValidationFinds {
			summary.Findings = append(summary.Findings, projectFinding(f))
		}
		sortArtifacts(summary.Artifacts)
		out = append(out, summary)
	}
	return out, nil
}
