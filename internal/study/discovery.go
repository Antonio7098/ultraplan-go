package study

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Antonio7098/ultraplan-go/internal/workspace"
	"gopkg.in/yaml.v3"
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
	sourceMetadata, err := readLiveSourceMetadata(study)
	if err != nil {
		return nil, err
	}
	var sources []Source
	for _, entry := range entries {
		if isHidden(entry.Name()) {
			continue
		}
		if strings.HasSuffix(entry.Name(), ".ultraplan-source.yml") || strings.HasSuffix(entry.Name(), ".ultraplan-source.yaml") {
			continue
		}
		sourcePath := filepath.Join(sourcesDir, entry.Name())
		metadata := sourceMetadata[entry.Name()]
		if entry.IsDir() {
			localMetadata, err := readSourceMetadataFile(filepath.Join(sourcePath, ".ultraplan-source.yml"))
			if err != nil {
				return nil, err
			}
			metadata = mergeApplicableDimensions(localMetadata, metadata)
			sources = append(sources, Source{
				Name:                 entry.Name(),
				Kind:                 SourceKindDirectory,
				Path:                 sourcePath,
				ApplicableDimensions: metadata,
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
			ApplicableDimensions: mergeApplicableDimensions(applicable, metadata),
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

func readLiveSourceMetadata(study Study) (map[string][]string, error) {
	sourcesDir := filepath.Join(study.Path, "sources")
	entries, err := readOptionalDir(sourcesDir)
	if err != nil {
		return nil, fmt.Errorf("read source metadata for study %q: %w", study.Name, err)
	}
	metadata := map[string][]string{}
	for _, entry := range entries {
		name, ok := sourceMetadataBaseName(entry.Name())
		if !ok || entry.IsDir() || isHidden(entry.Name()) {
			continue
		}
		applicable, err := readSourceMetadataFile(filepath.Join(sourcesDir, entry.Name()))
		if err != nil {
			return nil, err
		}
		metadata[name] = applicable
	}
	return metadata, nil
}

func sourceMetadataBaseName(name string) (string, bool) {
	for _, suffix := range []string{".ultraplan-source.yml", ".ultraplan-source.yaml"} {
		if strings.HasSuffix(name, suffix) {
			return strings.TrimSuffix(name, suffix), true
		}
	}
	return "", false
}

func readSourceMetadataFile(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read source metadata %s: %w", path, err)
	}
	var raw struct {
		ApplicableDimensions any `yaml:"applicable_dimensions"`
	}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse source metadata %s: %w", path, err)
	}
	applicable, err := normalizeApplicableDimensions(raw.ApplicableDimensions)
	if err != nil {
		return nil, fmt.Errorf("parse source metadata %s applicable_dimensions: %w", path, err)
	}
	return applicable, nil
}

func mergeApplicableDimensions(primary, fallback []string) []string {
	if len(primary) > 0 {
		return primary
	}
	return fallback
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
