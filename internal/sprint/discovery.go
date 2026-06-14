package sprint

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"ultraplan-go/internal/project"
	"ultraplan-go/internal/workspace"
)

func DiscoverSprints(root string, p project.Project) ([]Sprint, error) {
	sprintsDir, err := workspace.ResolveInside(root, filepath.ToSlash(filepath.Join("projects", p.Name, "sprints")))
	if err != nil {
		return nil, err
	}
	entries, err := readOptionalDir(sprintsDir)
	if err != nil {
		return nil, fmt.Errorf("read sprints: %w", err)
	}
	var sprints []Sprint
	for _, entry := range entries {
		if !entry.IsDir() || isHidden(entry.Name()) || !project.IsSafeName(entry.Name()) {
			continue
		}
		sprints = append(sprints, Sprint{Project: p.Name, Slug: entry.Name(), Path: filepath.Join(sprintsDir, entry.Name())})
	}
	sort.Slice(sprints, func(i, j int) bool { return sprints[i].Slug < sprints[j].Slug })
	return sprints, nil
}

func ResolveSprint(sprints []Sprint, ref string) (Sprint, error) {
	ref = strings.TrimSpace(ref)
	if !project.IsSafeName(ref) {
		return Sprint{}, fmt.Errorf("invalid sprint reference %q: use a single safe path segment", ref)
	}
	for _, s := range sprints {
		if s.Slug == ref {
			return s, nil
		}
	}
	var matches []Sprint
	for _, s := range sprints {
		if strings.HasPrefix(s.Slug, ref) {
			matches = append(matches, s)
		}
	}
	switch len(matches) {
	case 0:
		return Sprint{}, RefError{Ref: ref, Candidates: sprintNames(sprints)}
	case 1:
		return matches[0], nil
	default:
		return Sprint{}, RefError{Ref: ref, Candidates: sprintNames(matches), Ambiguous: true}
	}
}

func sprintNames(sprints []Sprint) []string {
	names := make([]string, 0, len(sprints))
	for _, s := range sprints {
		names = append(names, s.Slug)
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
