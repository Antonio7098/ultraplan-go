package workspace

import (
	"fmt"
	"os"
	"path/filepath"
)

const MarkerFile = "ultraplan.yml"

type Root struct {
	Path string
}

type DiscoverOptions struct {
	ExplicitPath string
	EnvWorkspace string
	StartDir     string
}

func Discover(opts DiscoverOptions) (Root, error) {
	if opts.ExplicitPath != "" {
		return requireWorkspace(opts.ExplicitPath)
	}
	if opts.EnvWorkspace != "" {
		return requireWorkspace(opts.EnvWorkspace)
	}
	start := opts.StartDir
	if start == "" {
		var err error
		start, err = os.Getwd()
		if err != nil {
			return Root{}, fmt.Errorf("get working directory: %w", err)
		}
	}
	start, err := normalize(start)
	if err != nil {
		return Root{}, err
	}
	for {
		if HasMarker(start) {
			return Root{Path: start}, nil
		}
		parent := filepath.Dir(start)
		if parent == start {
			break
		}
		start = parent
	}
	return Root{}, fmt.Errorf("workspace not found: initialize one with 'ultraplan init-workspace'")
}

func requireWorkspace(path string) (Root, error) {
	root, err := normalize(path)
	if err != nil {
		return Root{}, err
	}
	if !HasMarker(root) {
		return Root{}, fmt.Errorf("invalid workspace %q: missing %s", root, MarkerFile)
	}
	return Root{Path: root}, nil
}

func HasMarker(root string) bool {
	info, err := os.Stat(filepath.Join(root, MarkerFile))
	return err == nil && !info.IsDir()
}

func normalize(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("path is required")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("normalize path %q: %w", path, err)
	}
	return filepath.Clean(abs), nil
}
