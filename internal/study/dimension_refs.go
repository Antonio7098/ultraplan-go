package study

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var dimensionFilePattern = regexp.MustCompile(`^([0-9]+(?:\.[0-9]+)*)(?:[-_ ]+(.+))?\.md$`)

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
	parts := strings.Split(strings.TrimSpace(raw), ".")
	if len(parts) == 0 {
		return "", false
	}
	normalized := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			return "", false
		}
		n, err := strconv.Atoi(part)
		if err != nil || n <= 0 {
			return "", false
		}
		normalized = append(normalized, fmt.Sprintf("%02d", n))
	}
	return strings.Join(normalized, "."), true
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
