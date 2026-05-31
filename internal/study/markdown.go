package study

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

var errInvalidApplicableDimension = errors.New("invalid applicable dimension")

func parseFrontmatter(content string) (map[string]any, []string, error) {
	block, ok, err := leadingFrontmatterBlock(content)
	if err != nil {
		return nil, nil, err
	}
	if !ok {
		return nil, nil, nil
	}
	if strings.TrimSpace(block) == "" {
		return map[string]any{}, nil, nil
	}
	var frontmatter map[string]any
	if err := yaml.Unmarshal([]byte(block), &frontmatter); err != nil {
		return nil, nil, fmt.Errorf("parse YAML frontmatter: %w", err)
	}
	if frontmatter == nil {
		frontmatter = map[string]any{}
	}
	applicable, err := normalizeApplicableDimensions(frontmatter["applicable_dimensions"])
	if err != nil {
		return nil, nil, err
	}
	return frontmatter, applicable, nil
}

func stripFrontmatter(content string) string {
	_, end, ok, err := leadingFrontmatterRange(content)
	if err != nil || !ok {
		return content
	}
	return content[end:]
}

func leadingFrontmatterBlock(content string) (string, bool, error) {
	start, _, ok, err := leadingFrontmatterRange(content)
	if err != nil || !ok {
		return "", ok, err
	}
	searchFrom := start
	for searchFrom < len(content) {
		line, _ := nextLine(content, searchFrom)
		if strings.TrimSuffix(line, "\r") == "---" {
			return content[start:searchFrom], true, nil
		}
		_, searchFrom = nextLine(content, searchFrom)
	}
	return "", false, fmt.Errorf("unterminated leading frontmatter")
}

func leadingFrontmatterRange(content string) (int, int, bool, error) {
	if content == "" {
		return 0, 0, false, nil
	}
	if strings.HasPrefix(content, "\ufeff") {
		return 0, 0, false, nil
	}
	firstLine, firstLineEnd := nextLine(content, 0)
	if strings.TrimSuffix(firstLine, "\r") != "---" {
		return 0, 0, false, nil
	}
	searchFrom := firstLineEnd
	for searchFrom < len(content) {
		line, next := nextLine(content, searchFrom)
		if strings.TrimSuffix(line, "\r") == "---" {
			return firstLineEnd, next, true, nil
		}
		searchFrom = next
	}
	return 0, 0, false, fmt.Errorf("unterminated leading frontmatter")
}

func nextLine(content string, start int) (string, int) {
	if start >= len(content) {
		return "", len(content)
	}
	rel := strings.IndexByte(content[start:], '\n')
	if rel < 0 {
		return content[start:], len(content)
	}
	end := start + rel
	return content[start:end], end + 1
}

func normalizeApplicableDimensions(raw any) ([]string, error) {
	if raw == nil {
		return nil, nil
	}
	values, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("%w: %v", errInvalidApplicableDimension, raw)
	}
	seen := make(map[string]bool)
	var out []string
	for _, value := range values {
		normalized, err := normalizeApplicableDimension(value)
		if err != nil {
			return nil, err
		}
		if normalized == "" || seen[normalized] {
			continue
		}
		seen[normalized] = true
		out = append(out, normalized)
	}
	sort.Strings(out)
	return out, nil
}

func normalizeApplicableDimension(raw any) (string, error) {
	switch value := raw.(type) {
	case int:
		if value <= 0 {
			return "", fmt.Errorf("%w: %v", errInvalidApplicableDimension, raw)
		}
		return fmt.Sprintf("%02d", value), nil
	case int64:
		if value <= 0 {
			return "", fmt.Errorf("%w: %v", errInvalidApplicableDimension, raw)
		}
		return fmt.Sprintf("%02d", value), nil
	case float64:
		if value <= 0 || value != float64(int(value)) {
			return "", fmt.Errorf("%w: %v", errInvalidApplicableDimension, raw)
		}
		return fmt.Sprintf("%02d", int(value)), nil
	case string:
		value = strings.TrimSpace(value)
		if value == "" {
			return "", nil
		}
		normalized, ok := normalizeDimensionNumber(value)
		if !ok {
			return "", fmt.Errorf("%w: %q", errInvalidApplicableDimension, raw)
		}
		return normalized, nil
	case uint64:
		if value == 0 || value > uint64(^uint(0)>>1) {
			return "", fmt.Errorf("%w: %v", errInvalidApplicableDimension, raw)
		}
		return fmt.Sprintf("%02d", value), nil
	case yaml.Node:
		return normalizeApplicableDimensionFromNode(value)
	default:
		return "", fmt.Errorf("%w: %v", errInvalidApplicableDimension, raw)
	}
}

func normalizeApplicableDimensionFromNode(node yaml.Node) (string, error) {
	switch node.Tag {
	case "!!int":
		n, err := strconv.Atoi(node.Value)
		if err != nil || n <= 0 {
			return "", fmt.Errorf("%w: %q", errInvalidApplicableDimension, node.Value)
		}
		return fmt.Sprintf("%02d", n), nil
	case "!!str":
		return normalizeApplicableDimension(node.Value)
	default:
		return "", fmt.Errorf("%w: %q", errInvalidApplicableDimension, node.Value)
	}
}
