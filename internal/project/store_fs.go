package project

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Antonio7098/ultraplan-go/internal/workspace"
)

type FSStore struct {
	Root string
}

type ProjectFiles struct {
	DocsDirExists      bool
	MarkdownDocs       []string
	RoadmapExists      bool
	ProjectIndexExists bool
	SprintsDirExists   bool
	SprintDirs         []string
	IndexContent       string
}

func NewFSStore(root string) FSStore {
	return FSStore{Root: root}
}

func (s FSStore) ReadProjectFiles(p Project) (ProjectFiles, error) {
	projectRel := filepath.ToSlash(filepath.Join("projects", p.Name))
	projectRoot, err := workspace.ResolveInside(s.Root, projectRel)
	if err != nil {
		return ProjectFiles{}, err
	}
	if filepath.Clean(projectRoot) != filepath.Clean(p.Path) {
		return ProjectFiles{}, fmt.Errorf("project path mismatch for %q", p.Name)
	}
	var files ProjectFiles
	docsDir, err := workspace.ResolveInside(s.Root, filepath.ToSlash(filepath.Join(projectRel, "docs")))
	if err != nil {
		return ProjectFiles{}, err
	}
	if entries, err := readOptionalDir(docsDir); err != nil {
		return ProjectFiles{}, fmt.Errorf("read project docs: %w", err)
	} else if entries != nil {
		files.DocsDirExists = true
		for _, entry := range entries {
			if entry.IsDir() || isHidden(entry.Name()) || strings.ToLower(filepath.Ext(entry.Name())) != ".md" {
				continue
			}
			files.MarkdownDocs = append(files.MarkdownDocs, filepath.ToSlash(filepath.Join("docs", entry.Name())))
		}
	}
	sort.Strings(files.MarkdownDocs)

	files.RoadmapExists = fileExists(filepath.Join(projectRoot, "roadmap.md"))
	indexPath := filepath.Join(projectRoot, "project-index.md")
	files.ProjectIndexExists = fileExists(indexPath)
	if files.ProjectIndexExists {
		content, err := os.ReadFile(indexPath)
		if err != nil {
			return ProjectFiles{}, fmt.Errorf("read project-index.md: %w", err)
		}
		files.IndexContent = string(content)
	}

	sprintsDir, err := workspace.ResolveInside(s.Root, filepath.ToSlash(filepath.Join(projectRel, "sprints")))
	if err != nil {
		return ProjectFiles{}, err
	}
	if entries, err := readOptionalDir(sprintsDir); err != nil {
		return ProjectFiles{}, fmt.Errorf("read project sprints: %w", err)
	} else if entries != nil {
		files.SprintsDirExists = true
		for _, entry := range entries {
			if entry.IsDir() && !isHidden(entry.Name()) {
				files.SprintDirs = append(files.SprintDirs, entry.Name())
			}
		}
	}
	sort.Strings(files.SprintDirs)
	return files, nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
