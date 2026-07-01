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
	Sources    []sourceYAML `yaml:"sources"`
	Dimensions struct {
		Count int             `yaml:"count"`
		Items []dimensionYAML `yaml:"items"`
	} `yaml:"dimensions"`
}

type sourceYAML struct {
	Name                 string `yaml:"name"`
	URL                  string `yaml:"url"`
	Path                 string `yaml:"path"`
	Description          string `yaml:"description"`
	ApplicableDimensions any    `yaml:"applicable_dimensions"`
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

type InitValidationProblem struct {
	Code    string
	Field   string
	Message string
}

type InitValidationError struct {
	Problems []InitValidationProblem
}

func (e InitValidationError) Error() string {
	messages := make([]string, 0, len(e.Problems))
	for _, problem := range e.Problems {
		if problem.Field == "" {
			messages = append(messages, problem.Message)
			continue
		}
		messages = append(messages, problem.Field+" "+problem.Message)
	}
	return fmt.Sprintf("%v: %s", ErrInitValidation, strings.Join(messages, "; "))
}

func (e InitValidationError) Unwrap() error { return ErrInitValidation }

type InitSource struct {
	Name                 string
	URL                  string
	Path                 string
	Description          string
	ApplicableDimensions []string
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
	var problems []InitValidationProblem
	requireField(&problems, "name", raw.Name)
	requireField(&problems, "description", raw.Description)
	if raw.Name != "" && !isSafeName(raw.Name) {
		addProblem(&problems, "validation.name", "name", "must be filesystem-safe")
	}
	sourceItems := raw.Repos.Items
	validateSourceCount := true
	if len(sourceItems) == 0 && len(raw.Sources) > 0 {
		sourceItems = raw.Sources
		validateSourceCount = false
	}
	if validateSourceCount && raw.Repos.Count < len(sourceItems) {
		addProblem(&problems, "validation.count", "repos.count", "cannot be less than explicit repos.items")
	}
	if validateSourceCount && raw.Repos.Count > len(sourceItems) {
		addProblem(&problems, "validation.count", "repos.count", "is greater than repos.items; assisted completion is deferred, provide explicit repo items")
	}
	if raw.Dimensions.Count < len(raw.Dimensions.Items) {
		addProblem(&problems, "validation.count", "dimensions.count", "cannot be less than explicit dimensions.items")
	}
	if raw.Dimensions.Count > len(raw.Dimensions.Items) {
		addProblem(&problems, "validation.count", "dimensions.count", "is greater than dimensions.items; assisted completion is deferred, provide explicit dimension items")
	}

	sources := make([]InitSource, 0, len(sourceItems))
	sourceNames := map[string]bool{}
	for i, item := range sourceItems {
		prefix := fmt.Sprintf("repos.items[%d]", i)
		requireField(&problems, prefix+".name", item.Name)
		requireField(&problems, prefix+".description", item.Description)
		if item.URL == "" && item.Path == "" {
			addProblem(&problems, "validation.required", prefix+".url", "or "+prefix+".path is required")
		}
		if item.Path != "" && !isSafeRelativePath(item.Path) {
			addProblem(&problems, "validation.path", prefix+".path", "must be a safe relative path")
		}
		if item.Name != "" && !isSafeName(item.Name) {
			addProblem(&problems, "validation.name", prefix+".name", "must be filesystem-safe")
		}
		if item.Name != "" && sourceNames[item.Name] {
			addProblem(&problems, "validation.duplicate", prefix+".name", "duplicates source "+item.Name)
		}
		applicable, err := normalizeApplicableDimensions(item.ApplicableDimensions)
		if err != nil {
			addProblem(&problems, "validation.applicable_dimensions", prefix+".applicable_dimensions", err.Error())
		}
		sourceNames[item.Name] = true
		sources = append(sources, InitSource{Name: item.Name, URL: item.URL, Path: item.Path, Description: item.Description, ApplicableDimensions: applicable})
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
			addProblem(&problems, "validation.number", prefix+".number", "must be a positive number")
		}
		slug := normalizeSlug(item.Name)
		if item.Name != "" && slug == "" {
			addProblem(&problems, "validation.name", prefix+".name", "must produce a filesystem-safe slug")
		}
		if number != "" && dimensionNumbers[number] {
			addProblem(&problems, "validation.duplicate", prefix+".number", "duplicates dimension "+number)
		}
		if slug != "" && dimensionSlugs[slug] {
			addProblem(&problems, "validation.duplicate", prefix+".name", "duplicates dimension slug "+slug)
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
		return initDefinition{}, InitValidationError{Problems: problems}
	}
	return initDefinition{Name: raw.Name, Description: raw.Description, Sources: sources, Dimensions: dimensions}, nil
}

func requireField(problems *[]InitValidationProblem, field, value string) {
	if strings.TrimSpace(value) == "" {
		addProblem(problems, "validation.required", field, "is required")
	}
}

func requireList(problems *[]InitValidationProblem, field string, value []string) {
	if len(value) == 0 {
		addProblem(problems, "validation.required", field, "is required")
	}
}

func addProblem(problems *[]InitValidationProblem, code, field, message string) {
	*problems = append(*problems, InitValidationProblem{Code: code, Field: field, Message: message})
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
