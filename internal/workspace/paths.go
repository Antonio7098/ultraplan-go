package workspace

import (
	"fmt"
	"path/filepath"
	"strings"
)

func ResolveInside(root, rel string) (string, error) {
	root, err := normalize(root)
	if err != nil {
		return "", err
	}
	candidate := rel
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(root, candidate)
	}
	candidate, err = normalize(candidate)
	if err != nil {
		return "", err
	}
	if !isInside(root, candidate) {
		return "", fmt.Errorf("path %q escapes workspace %q", rel, root)
	}
	return candidate, nil
}

func Rel(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil || strings.HasPrefix(rel, "..") {
		return filepath.Clean(path)
	}
	return filepath.ToSlash(rel)
}

func isInside(root, candidate string) bool {
	rel, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}
