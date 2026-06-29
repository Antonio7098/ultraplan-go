package study

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Antonio7098/ultraplan-go/internal/workspace"
)

func DiscoverStudies(root string) ([]Study, error) {
	studiesDir, err := workspace.ResolveInside(root, "studies")
	if err != nil {
		return nil, err
	}
	entries, err := readOptionalDir(studiesDir)
	if err != nil {
		return nil, fmt.Errorf("read studies: %w", err)
	}
	var studies []Study
	for _, entry := range entries {
		if isHidden(entry.Name()) || !entry.IsDir() {
			continue
		}
		studies = append(studies, Study{
			Name: entry.Name(),
			Path: filepath.Join(studiesDir, entry.Name()),
		})
	}
	sort.Slice(studies, func(i, j int) bool {
		return studies[i].Name < studies[j].Name
	})
	return studies, nil
}

func DiscoverSources(study Study) ([]Source, error) {
	sourcesDir := filepath.Join(study.Path, "sources")
	entries, err := readOptionalDir(sourcesDir)
	if err != nil {
		return nil, fmt.Errorf("read sources for study %q: %w", study.Name, err)
	}
	var sources []Source
	for _, entry := range entries {
		if isHidden(entry.Name()) {
			continue
		}
		sourcePath := filepath.Join(sourcesDir, entry.Name())
		if entry.IsDir() {
			sources = append(sources, Source{
				Name: entry.Name(),
				Kind: SourceKindDirectory,
				Path: sourcePath,
			})
			continue
		}
		if strings.ToLower(filepath.Ext(entry.Name())) != ".md" {
			continue
		}
		content, err := os.ReadFile(sourcePath)
		if err != nil {
			return nil, fmt.Errorf("read source %s: %w", sourcePath, err)
		}
		frontmatter, applicable, err := parseFrontmatter(string(content))
		if err != nil {
			return nil, fmt.Errorf("parse source %s metadata: %w", sourcePath, err)
		}
		sources = append(sources, Source{
			Name:                 entry.Name(),
			Kind:                 SourceKindMarkdown,
			Path:                 sourcePath,
			ApplicableDimensions: applicable,
			Frontmatter:          frontmatter,
		})
	}
	sort.Slice(sources, func(i, j int) bool {
		if sources[i].Name == sources[j].Name {
			if sources[i].Kind == sources[j].Kind {
				return sources[i].Path < sources[j].Path
			}
			return sources[i].Kind < sources[j].Kind
		}
		return sources[i].Name < sources[j].Name
	})
	return sources, nil
}

func DiscoverDimensions(study Study) ([]Dimension, error) {
	entries, err := readOptionalDir(filepath.Join(study.Path, "dimensions"))
	if err != nil {
		return nil, fmt.Errorf("read dimensions for study %q: %w", study.Name, err)
	}
	var dimensions []Dimension
	for _, entry := range entries {
		if isHidden(entry.Name()) || entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			continue
		}
		dimension, ok := dimensionFromFile(filepath.Join(study.Path, "dimensions", entry.Name()))
		if !ok {
			continue
		}
		dimensions = append(dimensions, dimension)
	}
	sort.Slice(dimensions, func(i, j int) bool {
		if dimensions[i].Number == dimensions[j].Number {
			return dimensions[i].File < dimensions[j].File
		}
		return dimensions[i].Number < dimensions[j].Number
	})
	return dimensions, nil
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
