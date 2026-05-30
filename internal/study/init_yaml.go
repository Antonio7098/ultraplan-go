package study

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

type initYAML struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Repos       struct {
		Count int          `yaml:"count"`
		Items []sourceYAML `yaml:"items"`
	} `yaml:"repos"`
	Dimensions struct {
		Count int             `yaml:"count"`
		Items []dimensionYAML `yaml:"items"`
	} `yaml:"dimensions"`
}

type sourceYAML struct {
	Name        string `yaml:"name"`
	URL         string `yaml:"url"`
	Path        string `yaml:"path"`
	Description string `yaml:"description"`
}

type dimensionYAML struct {
	Number      string   `yaml:"number"`
	Name        string   `yaml:"name"`
	Title       string   `yaml:"title"`
	Description string   `yaml:"description"`
	Purpose     string   `yaml:"purpose"`
	Steps       []string `yaml:"steps"`
	Citations   []string `yaml:"citations"`
	Questions   []string `yaml:"questions"`
}

type initDefinition struct {
	Name        string
	Description string
	Sources     []InitSource
	Dimensions  []InitDimension
}

type InitSource struct {
	Name        string
	URL         string
	Path        string
	Description string
}

type InitDimension struct {
	Number      string
	Name        string
	Slug        string
	FileName    string
	Title       string
	Description string
	Purpose     string
	Steps       []string
	Citations   []string
	Questions   []string
}

var safeNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)

func parseInitYAML(path string) (initDefinition, error) {
	if strings.TrimSpace(path) == "" {
		return initDefinition{}, fmt.Errorf("%w: input path is required", ErrInitValidation)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return initDefinition{}, fmt.Errorf("read study init yaml: %w", err)
	}
	var raw initYAML
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return initDefinition{}, fmt.Errorf("%w: parse YAML: %w", ErrInitValidation, err)
	}
	return normalizeInit(raw)
}

func normalizeInit(raw initYAML) (initDefinition, error) {
	var problems []string
	requireField(&problems, "name", raw.Name)
	requireField(&problems, "description", raw.Description)
	if raw.Name != "" && !isSafeName(raw.Name) {
		problems = append(problems, "name must be filesystem-safe")
	}
	if raw.Repos.Count < len(raw.Repos.Items) {
		problems = append(problems, "repos.count cannot be less than explicit repos.items")
	}
	if raw.Repos.Count > len(raw.Repos.Items) {
		problems = append(problems, "repos.count is greater than repos.items; assisted completion is deferred, provide explicit repo items")
	}
	if raw.Dimensions.Count < len(raw.Dimensions.Items) {
		problems = append(problems, "dimensions.count cannot be less than explicit dimensions.items")
	}
	if raw.Dimensions.Count > len(raw.Dimensions.Items) {
		problems = append(problems, "dimensions.count is greater than dimensions.items; assisted completion is deferred, provide explicit dimension items")
	}

	sources := make([]InitSource, 0, len(raw.Repos.Items))
	sourceNames := map[string]bool{}
	for i, item := range raw.Repos.Items {
		prefix := fmt.Sprintf("repos.items[%d]", i)
		requireField(&problems, prefix+".name", item.Name)
		requireField(&problems, prefix+".description", item.Description)
		if item.URL == "" && item.Path == "" {
			problems = append(problems, prefix+".url or "+prefix+".path is required")
		}
		if item.Path != "" && !isSafeRelativePath(item.Path) {
			problems = append(problems, prefix+".path must be a safe relative path")
		}
		if item.Name != "" && !isSafeName(item.Name) {
			problems = append(problems, prefix+".name must be filesystem-safe")
		}
		if item.Name != "" && sourceNames[item.Name] {
			problems = append(problems, prefix+".name duplicates source "+item.Name)
		}
		sourceNames[item.Name] = true
		sources = append(sources, InitSource{Name: item.Name, URL: item.URL, Path: item.Path, Description: item.Description})
	}

	dimensions := make([]InitDimension, 0, len(raw.Dimensions.Items))
	dimensionNumbers := map[string]bool{}
	dimensionSlugs := map[string]bool{}
	for i, item := range raw.Dimensions.Items {
		prefix := fmt.Sprintf("dimensions.items[%d]", i)
		requireField(&problems, prefix+".number", item.Number)
		requireField(&problems, prefix+".name", item.Name)
		requireField(&problems, prefix+".title", item.Title)
		requireField(&problems, prefix+".description", item.Description)
		requireField(&problems, prefix+".purpose", item.Purpose)
		requireList(&problems, prefix+".steps", item.Steps)
		requireList(&problems, prefix+".citations", item.Citations)
		requireList(&problems, prefix+".questions", item.Questions)
		number, ok := normalizeDimensionNumber(item.Number)
		if item.Number != "" && !ok {
			problems = append(problems, prefix+".number must be a positive number")
		}
		slug := normalizeSlug(item.Name)
		if item.Name != "" && slug == "" {
			problems = append(problems, prefix+".name must produce a filesystem-safe slug")
		}
		if number != "" && dimensionNumbers[number] {
			problems = append(problems, prefix+".number duplicates dimension "+number)
		}
		if slug != "" && dimensionSlugs[slug] {
			problems = append(problems, prefix+".name duplicates dimension slug "+slug)
		}
		dimensionNumbers[number] = true
		dimensionSlugs[slug] = true
		dimensions = append(dimensions, InitDimension{
			Number: number, Name: item.Name, Slug: slug, FileName: number + "-" + slug + ".md",
			Title: item.Title, Description: item.Description, Purpose: item.Purpose,
			Steps: item.Steps, Citations: item.Citations, Questions: item.Questions,
		})
	}
	if len(problems) > 0 {
		return initDefinition{}, fmt.Errorf("%w: %s", ErrInitValidation, strings.Join(problems, "; "))
	}
	return initDefinition{Name: raw.Name, Description: raw.Description, Sources: sources, Dimensions: dimensions}, nil
}

func requireField(problems *[]string, field, value string) {
	if strings.TrimSpace(value) == "" {
		*problems = append(*problems, field+" is required")
	}
}

func requireList(problems *[]string, field string, value []string) {
	if len(value) == 0 {
		*problems = append(*problems, field+" is required")
	}
}

func isSafeName(name string) bool {
	return safeNamePattern.MatchString(name) && !strings.Contains(name, "..")
}

func isSafeRelativePath(path string) bool {
	if filepath.IsAbs(path) {
		return false
	}
	clean := filepath.Clean(path)
	return clean != "." && clean != ".." && !strings.HasPrefix(clean, ".."+string(filepath.Separator))
}
