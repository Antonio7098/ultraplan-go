package study

import (
	"fmt"
	"strings"
)

func renderNormalizedYAML(def initDefinition) string {
	var b strings.Builder
	fmt.Fprintf(&b, "name: %s\n", def.Name)
	fmt.Fprintf(&b, "description: %s\n", yamlQuote(def.Description))
	fmt.Fprintf(&b, "repos:\n  count: %d\n  items:\n", len(def.Sources))
	for _, source := range def.Sources {
		fmt.Fprintf(&b, "    - name: %s\n", source.Name)
		if source.URL != "" {
			fmt.Fprintf(&b, "      url: %s\n", yamlQuote(source.URL))
		}
		if source.Path != "" {
			fmt.Fprintf(&b, "      path: %s\n", yamlQuote(source.Path))
		}
		fmt.Fprintf(&b, "      description: %s\n", yamlQuote(source.Description))
		if len(source.ApplicableDimensions) > 0 {
			writeYAMLList(&b, "applicable_dimensions", source.ApplicableDimensions)
		}
	}
	fmt.Fprintf(&b, "dimensions:\n  count: %d\n  items:\n", len(def.Dimensions))
	for _, dim := range def.Dimensions {
		fmt.Fprintf(&b, "    - number: \"%s\"\n", dim.Number)
		fmt.Fprintf(&b, "      name: %s\n", dim.Slug)
		fmt.Fprintf(&b, "      title: %s\n", yamlQuote(dim.Title))
		fmt.Fprintf(&b, "      description: %s\n", yamlQuote(dim.Description))
		fmt.Fprintf(&b, "      purpose: %s\n", yamlQuote(dim.Purpose))
		writeYAMLList(&b, "steps", dim.Steps)
		writeYAMLList(&b, "citations", dim.Citations)
		writeYAMLList(&b, "questions", dim.Questions)
	}
	return b.String()
}

func renderReadme(def initDefinition) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n%s\n\n", def.Name, def.Description)
	fmt.Fprintf(&b, "## Sources\n\n")
	for _, source := range def.Sources {
		target := source.Path
		if target == "" {
			target = "sources/" + source.Name
		}
		fmt.Fprintf(&b, "- `%s`: %s (%s)\n", source.Name, source.Description, target)
	}
	fmt.Fprintf(&b, "\n## Dimensions\n\n")
	for _, dim := range def.Dimensions {
		fmt.Fprintf(&b, "- `%s`: %s (`dimensions/%s`)\n", dim.Number, dim.Title, dim.FileName)
	}
	fmt.Fprintf(&b, "\n## Generated Paths\n\n")
	for _, path := range []string{"study-init.yml", "dimensions/", "sources/", "reports/source/", "reports/final/"} {
		fmt.Fprintf(&b, "- `%s`\n", path)
	}
	fmt.Fprintf(&b, "\n## Next Commands\n\n")
	fmt.Fprintf(&b, "- `ultraplan study list`\n")
	fmt.Fprintf(&b, "- `ultraplan study %s list`\n", def.Name)
	return b.String()
}

func renderDimensionMarkdown(dim InitDimension) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s %s\n\n", dim.Number, dim.Title)
	fmt.Fprintf(&b, "%s\n\n", dim.Description)
	fmt.Fprintf(&b, "## Purpose\n\n%s\n\n", dim.Purpose)
	writeMarkdownList(&b, "Steps", dim.Steps)
	writeMarkdownList(&b, "Citations", dim.Citations)
	writeMarkdownList(&b, "Questions", dim.Questions)
	return b.String()
}

func writeMarkdownList(b *strings.Builder, title string, values []string) {
	fmt.Fprintf(b, "## %s\n\n", title)
	for _, value := range values {
		fmt.Fprintf(b, "- %s\n", value)
	}
	b.WriteString("\n")
}

func writeYAMLList(b *strings.Builder, field string, values []string) {
	fmt.Fprintf(b, "      %s:\n", field)
	for _, value := range values {
		fmt.Fprintf(b, "        - %s\n", yamlQuote(value))
	}
}

func yamlQuote(value string) string {
	if value == "" {
		return `""`
	}
	escaped := strings.ReplaceAll(value, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	return `"` + escaped + `"`
}
