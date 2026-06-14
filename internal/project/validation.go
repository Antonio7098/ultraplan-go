package project

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"ultraplan-go/internal/workspace"
)

func ValidateProject(root string, p Project, files ProjectFiles) ValidationResult {
	var findings []ValidationFinding
	add := func(path, problem, cause, suggestion string, err error) {
		findings = append(findings, ValidationFinding{
			Severity:   SeverityError,
			Path:       path,
			Problem:    problem,
			Cause:      cause,
			Suggestion: suggestion,
			Err:        err,
		})
	}
	if !files.DocsDirExists {
		add(projectRel(p.Name, "docs"), "missing docs directory", "docs directory was not found", "Create docs/ with project Markdown documents.", nil)
	} else if len(files.MarkdownDocs) == 0 {
		add(projectRel(p.Name, "docs"), "empty docs directory", "no Markdown documents were found under docs/*.md", "Add at least one Markdown project document.", nil)
	}
	if !files.RoadmapExists {
		add(projectRel(p.Name, "roadmap.md"), "missing roadmap", "roadmap.md was not found", "Create roadmap.md for project sequencing.", nil)
	}
	if !files.ProjectIndexExists {
		add(projectRel(p.Name, "project-index.md"), "missing project index", "project-index.md was not found", "Create project-index.md with catalog tables.", nil)
	}
	if !files.SprintsDirExists {
		add(projectRel(p.Name, "sprints"), "missing sprints directory", "sprints directory was not found", "Create sprints/ for planning sprint artifacts.", nil)
	}
	if files.ProjectIndexExists {
		index, parseFindings := ParseProjectIndex(files.IndexContent)
		findings = append(findings, parseFindings...)
		for _, entry := range index.Entries {
			if entry.External {
				continue
			}
			full, err := workspace.ResolveInside(root, filepath.FromSlash(entry.Path))
			if err != nil {
				findings = append(findings, catalogFinding(entry, "catalog path escapes workspace", err.Error(), "Use a workspace-relative path inside this workspace.", err))
				continue
			}
			if _, err := os.Stat(full); err != nil {
				if os.IsNotExist(err) {
					findings = append(findings, catalogFinding(entry, "catalog path not found", fmt.Sprintf("%s does not exist", entry.Path), "Create the referenced artifact or update the catalog path.", err))
					continue
				}
				findings = append(findings, catalogFinding(entry, "catalog path cannot be read", err.Error(), "Fix filesystem permissions or update the catalog path.", err))
			}
		}
	}
	sortFindings(findings)
	status := StatusOK
	if len(findings) > 0 {
		status = StatusInvalid
	}
	return ValidationResult{Project: p, Status: status, Findings: findings}
}

func StatusFromValidation(p Project, files ProjectFiles, validation ValidationResult) ProjectStatus {
	status := ProjectStatus{
		Project:         p,
		DocsDir:         state(files.DocsDirExists),
		MarkdownDocs:    files.MarkdownDocs,
		Roadmap:         state(files.RoadmapExists),
		ProjectIndex:    state(files.ProjectIndexExists),
		SprintsDir:      state(files.SprintsDirExists),
		SprintDirs:      files.SprintDirs,
		Catalog:         validation.Status,
		ValidationFinds: validation.Findings,
	}
	if files.DocsDirExists && len(files.MarkdownDocs) == 0 {
		status.DocsDir = StatusEmpty
	}
	return status
}

func state(ok bool) StatusState {
	if ok {
		return StatusPresent
	}
	return StatusMissing
}

func catalogFinding(entry CatalogEntry, problem, cause, suggestion string, err error) ValidationFinding {
	return ValidationFinding{
		Severity:   SeverityError,
		Section:    entry.Section,
		EntryName:  entry.Name,
		Path:       entry.Path,
		Problem:    problem,
		Cause:      cause,
		Suggestion: suggestion,
		Err:        err,
	}
}

func sortFindings(findings []ValidationFinding) {
	sort.SliceStable(findings, func(i, j int) bool {
		a, b := findings[i], findings[j]
		if a.Section != b.Section {
			return a.Section < b.Section
		}
		if a.EntryName != b.EntryName {
			return a.EntryName < b.EntryName
		}
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		return a.Problem < b.Problem
	})
}

func projectRel(projectName string, elems ...string) string {
	parts := append([]string{"projects", projectName}, elems...)
	return filepath.ToSlash(filepath.Join(parts...))
}
