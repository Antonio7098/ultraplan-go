package study

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

type Study struct {
	Name string
	Path string
}

type SourceKind string

const (
	SourceKindDirectory SourceKind = "directory"
	SourceKindMarkdown  SourceKind = "markdown"
)

type Source struct {
	Name                 string
	Kind                 SourceKind
	Path                 string
	ApplicableDimensions []string
	Frontmatter          map[string]any
}

type Dimension struct {
	Number string
	Slug   string
	File   string
	Path   string
}

func (d Dimension) Ref() string {
	if d.Slug == "" {
		return d.Number
	}
	return d.Number + "-" + d.Slug
}

var dimensionFilePattern = regexp.MustCompile(`^([0-9]+)(?:[-_ ]+(.+))?\.md$`)

func dimensionFromFile(path string) (Dimension, bool) {
	file := filepath.Base(path)
	matches := dimensionFilePattern.FindStringSubmatch(file)
	if matches == nil {
		return Dimension{}, false
	}
	number, ok := normalizeDimensionNumber(matches[1])
	if !ok {
		return Dimension{}, false
	}
	slug := normalizeSlug(matches[2])
	return Dimension{
		Number: number,
		Slug:   slug,
		File:   file,
		Path:   path,
	}, true
}

func normalizeDimensionNumber(raw string) (string, bool) {
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return "", false
	}
	return fmt.Sprintf("%02d", n), true
}

func normalizeDimensionRef(ref string) string {
	if number, ok := normalizeDimensionNumber(ref); ok {
		return number
	}
	return strings.TrimSpace(ref)
}

func normalizeSlug(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimSuffix(raw, ".md")
	raw = strings.Trim(raw, "-_ ")
	raw = strings.ToLower(raw)
	var b strings.Builder
	lastDash := false
	for _, r := range raw {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
			lastDash = false
		case r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case r == '-' || r == '_' || r == ' ':
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}
