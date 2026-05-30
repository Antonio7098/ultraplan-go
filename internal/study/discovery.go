package study

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"ultraplan-go/internal/workspace"
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
	entries, err := readOptionalDir(filepath.Join(study.Path, "sources"))
	if err != nil {
		return nil, fmt.Errorf("read sources for study %q: %w", study.Name, err)
	}
	var sources []Source
	for _, entry := range entries {
		if isHidden(entry.Name()) || !entry.IsDir() {
			continue
		}
		sources = append(sources, Source{
			Name: entry.Name(),
			Kind: SourceKindDirectory,
			Path: filepath.Join(study.Path, "sources", entry.Name()),
		})
	}
	sort.Slice(sources, func(i, j int) bool {
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
