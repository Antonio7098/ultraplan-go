package codeextract

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var ignoredFallbackDirs = map[string]struct{}{
	".git":         {},
	".hg":          {},
	".svn":         {},
	"node_modules": {},
	".ultraplan":   {},
}

type resolver struct {
	workspaceRoot string
	reportDir     string
	sources       []Source
	// basenameCache is intentionally scoped to one extraction invocation so
	// fallback search never persists stale filesystem state.
	basenameCache map[string][]string
}

func newResolver(workspaceRoot, reportPath string, sources []Source) (*resolver, []Source, []Diagnostic) {
	r := &resolver{
		workspaceRoot: workspaceRoot,
		reportDir:     filepath.Dir(reportPath),
		basenameCache: map[string][]string{},
	}
	resolved := make([]Source, 0, len(sources))
	var diagnostics []Diagnostic
	for _, source := range sources {
		root, err := resolveSourceRoot(workspaceRoot, r.reportDir, source.Path)
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{ReportPath: reportPath, SourceName: source.Name, Path: source.Path, Reason: err.Error()})
			continue
		}
		source.Root = root
		resolved = append(resolved, source)
	}
	r.sources = resolved
	return r, resolved, diagnostics
}

func resolveSourceRoot(workspaceRoot, reportDir, raw string) (string, error) {
	candidates := []string{filepath.Join(workspaceRoot, filepath.FromSlash(raw)), filepath.Join(reportDir, filepath.FromSlash(raw))}
	for _, candidate := range candidates {
		contained, err := containedPath(workspaceRoot, candidate)
		if err != nil {
			continue
		}
		info, statErr := os.Stat(contained)
		if statErr == nil && info.IsDir() {
			return contained, nil
		}
	}
	return "", fmt.Errorf("source root not found or outside workspace")
}

func (r *resolver) resolve(citedPath string) (Source, string, *Diagnostic) {
	if strings.Contains(citedPath, "\x00") {
		return Source{}, "", &Diagnostic{Path: citedPath, Reason: "path contains NUL byte"}
	}
	if filepath.IsAbs(citedPath) {
		return Source{}, "", &Diagnostic{Path: citedPath, Reason: "absolute paths are not allowed"}
	}
	cleaned := filepath.Clean(filepath.FromSlash(citedPath))
	if cleaned == "." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) || cleaned == ".." {
		return Source{}, "", &Diagnostic{Path: citedPath, Reason: "path escapes source root"}
	}
	for _, source := range r.sources {
		if path, ok := existingContainedFile(source.Root, cleaned); ok {
			return source, path, nil
		}
		parts := strings.Split(cleaned, string(filepath.Separator))
		if len(parts) > 1 && parts[0] == source.Name {
			stripped := filepath.Join(parts[1:]...)
			if path, ok := existingContainedFile(source.Root, stripped); ok {
				return source, path, nil
			}
		}
	}
	matches := r.basenameMatches(filepath.Base(cleaned))
	if len(matches) == 1 {
		for _, source := range r.sources {
			if inside(source.Root, matches[0]) {
				return source, matches[0], nil
			}
		}
	}
	if len(matches) > 1 {
		return Source{}, "", &Diagnostic{Path: citedPath, Reason: "ambiguous basename match"}
	}
	return Source{}, "", &Diagnostic{Path: citedPath, Reason: "file not found"}
}

func existingContainedFile(root, rel string) (string, bool) {
	path, err := containedPath(root, filepath.Join(root, rel))
	if err != nil {
		return "", false
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return "", false
	}
	return path, true
}

func containedPath(root, candidate string) (string, error) {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	candAbs, err := filepath.Abs(candidate)
	if err != nil {
		return "", err
	}
	rootEval, err := filepath.EvalSymlinks(rootAbs)
	if err != nil {
		rootEval = rootAbs
	}
	candEval, err := filepath.EvalSymlinks(candAbs)
	if err != nil {
		candEval = candAbs
	}
	if !inside(rootEval, candEval) {
		return "", fmt.Errorf("path escapes root")
	}
	return candAbs, nil
}

func inside(root, candidate string) bool {
	rel, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

func (r *resolver) basenameMatches(base string) []string {
	if matches, ok := r.basenameCache[base]; ok {
		return matches
	}
	var matches []string
	for _, source := range r.sources {
		_ = filepath.WalkDir(source.Root, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if entry.IsDir() {
				if _, ignored := ignoredFallbackDirs[entry.Name()]; ignored {
					return filepath.SkipDir
				}
				return nil
			}
			if entry.Name() == base && inside(source.Root, path) {
				matches = append(matches, path)
			}
			return nil
		})
	}
	sort.Strings(matches)
	r.basenameCache[base] = matches
	return matches
}
