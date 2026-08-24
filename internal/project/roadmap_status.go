package project

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var roadmapStatusLinePattern = regexp.MustCompile(`^(\s*>\s*Status:\s*).*$`)

// MarkRoadmapSprintDelivered changes the matching sprint's governed roadmap
// status to delivered. It preserves all other roadmap content.
func MarkRoadmapSprintDelivered(path, slug string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, fmt.Errorf("read roadmap.md: %w", err)
	}
	content := string(data)
	roadmap, _ := ParseRoadmap(content)
	var match *RoadmapSprint
	for i := range roadmap.Sprints {
		if roadmap.Sprints[i].Slug != slug {
			continue
		}
		if match != nil {
			return false, fmt.Errorf("roadmap.md has more than one sprint with slug %q", slug)
		}
		match = &roadmap.Sprints[i]
	}
	if match == nil {
		// Older and test workspaces may predate governed roadmap sections.
		if len(roadmap.Sprints) == 0 {
			return false, nil
		}
		return false, fmt.Errorf("roadmap.md has no sprint with slug %q", slug)
	}
	if match.Status == RoadmapDelivered {
		return false, nil
	}

	lines := strings.Split(content, "\n")
	start := match.Line
	end := len(lines)
	for i := start; i < len(lines); i++ {
		if level := headingLevel(lines[i]); level > 0 && level <= 3 {
			end = i
			break
		}
	}
	statusLine := -1
	slugLine := -1
	for i := start; i < end; i++ {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(strings.ToLower(trimmed), "> slug:") {
			slugLine = i
		}
		if roadmapStatusLinePattern.MatchString(lines[i]) {
			statusLine = i
			break
		}
		if headingLevel(lines[i]) == 4 {
			break
		}
	}
	if statusLine >= 0 {
		lines[statusLine] = roadmapStatusLinePattern.ReplaceAllString(lines[statusLine], `${1}delivered`)
	} else {
		if slugLine < 0 {
			return false, fmt.Errorf("roadmap.md sprint %q has no Slug metadata line", slug)
		}
		lines = append(lines, "")
		copy(lines[slugLine+2:], lines[slugLine+1:])
		lines[slugLine+1] = "> Status: delivered"
	}
	if err := atomicWriteRoadmap(path, []byte(strings.Join(lines, "\n"))); err != nil {
		return false, fmt.Errorf("write roadmap.md: %w", err)
	}
	return true, nil
}

func atomicWriteRoadmap(path string, data []byte) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".roadmap.*.tmp")
	if err != nil {
		return err
	}
	temp := file.Name()
	defer os.Remove(temp)
	if err := file.Chmod(info.Mode().Perm()); err != nil {
		_ = file.Close()
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(temp, path)
}
