package study

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDimensionFromFileNormalizesIdentity(t *testing.T) {
	dimension, ok := dimensionFromFile(filepath.Join("dimensions", "1-Command Architecture.md"))
	if !ok {
		t.Fatal("dimensionFromFile did not parse numeric markdown file")
	}
	if dimension.Number != "01" {
		t.Fatalf("Number = %q, want %q", dimension.Number, "01")
	}
	if dimension.Slug != "command-architecture" {
		t.Fatalf("Slug = %q, want %q", dimension.Slug, "command-architecture")
	}
	if dimension.File != "1-Command Architecture.md" {
		t.Fatalf("File = %q", dimension.File)
	}

	if _, ok := dimensionFromFile(filepath.Join("dimensions", "architecture.md")); ok {
		t.Fatal("dimensionFromFile parsed non-numeric filename")
	}
}

func TestDiscoverStudiesIsSortedAndIgnoresHiddenAndFiles(t *testing.T) {
	root := t.TempDir()
	mkdir(t, root, "studies", "zeta")
	mkdir(t, root, "studies", "alpha")
	mkdir(t, root, "studies", ".hidden")
	writeFile(t, root, "studies", "not-a-study")

	studies, err := DiscoverStudies(root)
	if err != nil {
		t.Fatal(err)
	}
	got := studyNames(studies)
	want := []string{"alpha", "zeta"}
	assertStrings(t, got, want)
}

func TestDiscoverSourcesIsSortedShallowAndIncludesMarkdown(t *testing.T) {
	root := t.TempDir()
	study := Study{Name: "demo", Path: filepath.Join(root, "studies", "demo")}
	mkdir(t, study.Path, "sources", "zeta", "nested-repo")
	mkdir(t, study.Path, "sources", "alpha")
	mkdir(t, study.Path, "sources", ".hidden")
	writeFileContent(t, study.Path, "---\napplicable_dimensions: [3, \"01\", 3]\ntitle: Example\n---\n# Body\n", "sources", "document.md")
	writeFileContent(t, study.Path, "# Applies to all\n", "sources", "all.md")
	writeFile(t, study.Path, "sources", "notes.txt")
	writeFile(t, study.Path, "sources", "zeta", "nested-repo", "README.md")

	sources, err := DiscoverSources(study)
	if err != nil {
		t.Fatal(err)
	}
	got := sourceNames(sources)
	want := []string{"all.md", "alpha", "document.md", "zeta"}
	assertStrings(t, got, want)
	if sources[0].Kind != SourceKindMarkdown || len(sources[0].ApplicableDimensions) != 0 {
		t.Fatalf("all.md = %+v, want unfiltered markdown", sources[0])
	}
	if sources[1].Kind != SourceKindDirectory {
		t.Fatalf("alpha kind = %q, want %q", sources[1].Kind, SourceKindDirectory)
	}
	if sources[2].Kind != SourceKindMarkdown {
		t.Fatalf("document.md kind = %q, want %q", sources[2].Kind, SourceKindMarkdown)
	}
	assertStrings(t, sources[2].ApplicableDimensions, []string{"01", "03"})
	if sources[2].Frontmatter["title"] != "Example" {
		t.Fatalf("frontmatter title = %#v", sources[2].Frontmatter["title"])
	}
}

func TestMarkdownFrontmatterParsingAndStripping(t *testing.T) {
	content := "---\napplicable_dimensions:\n  - 2\n  - \"01\"\n  - 02\nname: docs\n---\n# Body\n---\nnot metadata\n"
	frontmatter, applicable, err := parseFrontmatter(content)
	if err != nil {
		t.Fatal(err)
	}
	if frontmatter["name"] != "docs" {
		t.Fatalf("frontmatter name = %#v", frontmatter["name"])
	}
	assertStrings(t, applicable, []string{"01", "02"})
	if got := stripFrontmatter(content); got != "# Body\n---\nnot metadata\n" {
		t.Fatalf("stripFrontmatter = %q", got)
	}

	noFrontmatter := "# Body\n---\nexample\n"
	frontmatter, applicable, err = parseFrontmatter(noFrontmatter)
	if err != nil {
		t.Fatal(err)
	}
	if frontmatter != nil || applicable != nil {
		t.Fatalf("parseFrontmatter without leading block = %#v %#v", frontmatter, applicable)
	}
	if got := stripFrontmatter(noFrontmatter); got != noFrontmatter {
		t.Fatalf("stripFrontmatter changed non-frontmatter content: %q", got)
	}

	if _, _, err := parseFrontmatter("---\napplicable_dimensions: [1]\n"); err == nil {
		t.Fatal("parseFrontmatter returned nil error for unterminated frontmatter")
	}
}

func TestDiscoverSourcesReportsInvalidApplicabilityWithPathAndValue(t *testing.T) {
	root := t.TempDir()
	study := Study{Name: "demo", Path: filepath.Join(root, "studies", "demo")}
	sourcePath := filepath.Join(study.Path, "sources", "bad.md")
	writeFileContent(t, study.Path, "---\napplicable_dimensions: [bad]\n---\n# Body\n", "sources", "bad.md")

	_, err := DiscoverSources(study)
	if err == nil {
		t.Fatal("DiscoverSources returned nil error")
	}
	if !errors.Is(err, errInvalidApplicableDimension) {
		t.Fatalf("err = %v, want invalid applicable dimension cause", err)
	}
	if !strings.Contains(err.Error(), sourcePath) || !strings.Contains(err.Error(), `"bad"`) {
		t.Fatalf("err = %v, want path and offending value", err)
	}
}

func TestGetApplicableSourcesPreservesOrderAndFiltersMarkdown(t *testing.T) {
	sources := []Source{
		{Name: "repo", Kind: SourceKindDirectory},
		{Name: "all.md", Kind: SourceKindMarkdown},
		{Name: "only-two.md", Kind: SourceKindMarkdown, ApplicableDimensions: []string{"02"}},
		{Name: "only-one.md", Kind: SourceKindMarkdown, ApplicableDimensions: []string{"01"}},
	}
	got := GetApplicableSources(sources, Dimension{Number: "02"})
	assertStrings(t, sourceNames(got), []string{"repo", "all.md", "only-two.md"})
}

func TestDiscoverDimensionsIsSortedAndFilenameDerived(t *testing.T) {
	root := t.TempDir()
	study := Study{Name: "demo", Path: filepath.Join(root, "studies", "demo")}
	mkdir(t, study.Path, "dimensions", "nested")
	writeFile(t, study.Path, "dimensions", "2-runtime.md")
	writeFile(t, study.Path, "dimensions", "01-structure.md")
	writeFile(t, study.Path, "dimensions", ".03-hidden.md")
	writeFile(t, study.Path, "dimensions", "notes.txt")
	writeFile(t, study.Path, "dimensions", "architecture.md")
	writeFile(t, study.Path, "dimensions", "nested", "04-nested.md")

	dimensions, err := DiscoverDimensions(study)
	if err != nil {
		t.Fatal(err)
	}
	if len(dimensions) != 2 {
		t.Fatalf("len(dimensions) = %d, want 2: %+v", len(dimensions), dimensions)
	}
	if dimensions[0].Number != "01" || dimensions[0].Slug != "structure" {
		t.Fatalf("dimensions[0] = %+v", dimensions[0])
	}
	if dimensions[1].Number != "02" || dimensions[1].Slug != "runtime" {
		t.Fatalf("dimensions[1] = %+v", dimensions[1])
	}
}

func TestAbsentDirectoriesAreEmpty(t *testing.T) {
	root := t.TempDir()
	studies, err := DiscoverStudies(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(studies) != 0 {
		t.Fatalf("studies len = %d, want 0", len(studies))
	}

	study := Study{Name: "empty", Path: filepath.Join(root, "studies", "empty")}
	sources, err := DiscoverSources(study)
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 0 {
		t.Fatalf("sources len = %d, want 0", len(sources))
	}
	dimensions, err := DiscoverDimensions(study)
	if err != nil {
		t.Fatal(err)
	}
	if len(dimensions) != 0 {
		t.Fatalf("dimensions len = %d, want 0", len(dimensions))
	}
}

func TestResolveStudyExactPrefixMissingAndAmbiguous(t *testing.T) {
	studies := []Study{{Name: "api"}, {Name: "api-v2"}, {Name: "web"}}
	got, err := ResolveStudy(studies, "api")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "api" {
		t.Fatalf("exact got %q", got.Name)
	}
	got, err = ResolveStudy(studies, "we")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "web" {
		t.Fatalf("prefix got %q", got.Name)
	}
	_, err = ResolveStudy(studies, "ap")
	assertRefError(t, err, true)
	_, err = ResolveStudy(studies, "missing")
	assertRefError(t, err, false)
}

func TestResolveSourceExactPrefixMissingAndAmbiguous(t *testing.T) {
	sources := []Source{{Name: "alpha"}, {Name: "alpine"}, {Name: "beta"}}
	got, err := ResolveSource(sources, "alpha")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "alpha" {
		t.Fatalf("exact got %q", got.Name)
	}
	got, err = ResolveSource(sources, "bet")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "beta" {
		t.Fatalf("prefix got %q", got.Name)
	}
	_, err = ResolveSource(sources, "al")
	assertRefError(t, err, true)
	_, err = ResolveSource(sources, "missing")
	assertRefError(t, err, false)
}

func TestResolveDimensionAliasesPrefixMissingAndAmbiguous(t *testing.T) {
	dimensions := []Dimension{
		{Number: "01", Slug: "structure", File: "01-structure.md"},
		{Number: "02", Slug: "runtime", File: "02-runtime.md"},
		{Number: "03", Slug: "reliability", File: "03-reliability.md"},
		{Number: "04", Slug: "rendering", File: "04-rendering.md"},
	}
	for _, ref := range []string{"1", "01", "structure", "01-structure.md", "01-structure"} {
		got, err := ResolveDimension(dimensions, ref)
		if err != nil {
			t.Fatalf("ResolveDimension(%q): %v", ref, err)
		}
		if got.Number != "01" {
			t.Fatalf("ResolveDimension(%q) = %+v", ref, got)
		}
	}
	got, err := ResolveDimension(dimensions, "runt")
	if err != nil {
		t.Fatal(err)
	}
	if got.Number != "02" {
		t.Fatalf("prefix got %+v", got)
	}
	_, err = ResolveDimension(dimensions, "re")
	assertRefError(t, err, true)
	_, err = ResolveDimension(dimensions, "missing")
	assertRefError(t, err, false)
}

func mkdir(t *testing.T, base string, rel ...string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(append([]string{base}, rel...)...), 0o755); err != nil {
		t.Fatal(err)
	}
}

func writeFile(t *testing.T, base string, rel ...string) {
	t.Helper()
	writeFileContent(t, base, "test", rel...)
}

func writeFileContent(t *testing.T, base, content string, rel ...string) {
	t.Helper()
	path := filepath.Join(append([]string{base}, rel...)...)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func studyNames(studies []Study) []string {
	out := make([]string, 0, len(studies))
	for _, study := range studies {
		out = append(out, study.Name)
	}
	return out
}

func sourceNames(sources []Source) []string {
	out := make([]string, 0, len(sources))
	for _, source := range sources {
		out = append(out, source.Name)
	}
	return out
}

func assertStrings(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func assertRefError(t *testing.T, err error, ambiguous bool) {
	t.Helper()
	var refErr RefError
	if !errors.As(err, &refErr) {
		t.Fatalf("err = %v, want RefError", err)
	}
	if refErr.Ambiguous != ambiguous {
		t.Fatalf("Ambiguous = %v, want %v: %v", refErr.Ambiguous, ambiguous, err)
	}
	if len(refErr.Candidates) == 0 {
		t.Fatalf("Candidates empty: %v", err)
	}
}
