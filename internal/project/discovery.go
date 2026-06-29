package project

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Antonio7098/ultraplan-go/internal/workspace"
)

type RefError struct {
	Ref        string
	Candidates []string
	Ambiguous  bool
}

func (e RefError) Error() string {
	if e.Ambiguous {
		return fmt.Sprintf("ambiguous project reference %q; matches: %s", e.Ref, strings.Join(e.Candidates, ", "))
	}
	if len(e.Candidates) == 0 {
		return fmt.Sprintf("project reference %q not found", e.Ref)
	}
	return fmt.Sprintf("project reference %q not found; available: %s", e.Ref, strings.Join(e.Candidates, ", "))
}

func DiscoverProjects(root string) ([]Project, error) {
	projectsDir, err := workspace.ResolveInside(root, "projects")
	if err != nil {
		return nil, err
	}
	entries, err := readOptionalDir(projectsDir)
	if err != nil {
		return nil, fmt.Errorf("read projects: %w", err)
	}
	var projects []Project
	for _, entry := range entries {
		if isHidden(entry.Name()) || !entry.IsDir() || !IsSafeName(entry.Name()) {
			continue
		}
		projects = append(projects, Project{Name: entry.Name(), Path: filepath.Join(projectsDir, entry.Name())})
	}
	sort.Slice(projects, func(i, j int) bool { return projects[i].Name < projects[j].Name })
	return projects, nil
}

func ResolveProject(projects []Project, ref string) (Project, error) {
	ref = strings.TrimSpace(ref)
	if !IsSafeName(ref) {
		return Project{}, fmt.Errorf("invalid project reference %q: use a single safe path segment", ref)
	}
	for _, p := range projects {
		if p.Name == ref {
			return p, nil
		}
	}
	var matches []Project
	for _, p := range projects {
		if strings.HasPrefix(p.Name, ref) {
			matches = append(matches, p)
		}
	}
	switch len(matches) {
	case 0:
		return Project{}, RefError{Ref: ref, Candidates: projectNames(projects)}
	case 1:
		return matches[0], nil
	default:
		return Project{}, RefError{Ref: ref, Candidates: projectNames(matches), Ambiguous: true}
	}
}

func IsSafeName(name string) bool {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `/\`) || strings.HasPrefix(name, ".") {
		return false
	}
	for _, r := range name {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			return false
		}
	}
	return true
}

func projectNames(projects []Project) []string {
	names := make([]string, 0, len(projects))
	for _, p := range projects {
		names = append(names, p.Name)
	}
	sort.Strings(names)
	return names
}

func readOptionalDir(path string) ([]os.DirEntry, error) {
	entries, err := os.ReadDir(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	return entries, err
}

func isHidden(name string) bool {
	return strings.HasPrefix(name, ".")
}
