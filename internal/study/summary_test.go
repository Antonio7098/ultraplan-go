package study

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteSummaryDeterministicRatingsAndInapplicableCells(t *testing.T) {
	root, st := executionFixture(t)
	dimensions, err := DiscoverDimensions(st)
	if err != nil {
		t.Fatal(err)
	}
	sources, err := DiscoverSources(st)
	if err != nil {
		t.Fatal(err)
	}
	writeReport(t, SourceReportPath(st, Source{Name: "repo", Kind: SourceKindDirectory}, Dimension{Number: "01", Slug: "structure"}), validSourceReport)
	writeReport(t, SourceReportPath(st, Source{Name: "doc.md", Kind: SourceKindMarkdown}, Dimension{Number: "01", Slug: "structure"}), validMarkdownReport)

	result, err := WriteSummary(st, dimensions, sources)
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(result.Path)
	if err != nil {
		t.Fatal(err)
	}
	want := "source,01-structure,total\n" +
		"doc.md,8,8\n" +
		"repo,8,8\n" +
		"other.md,N/A,0\n"
	if string(content) != want {
		t.Fatalf("summary:\n%s\nwant:\n%s", content, want)
	}
	if len(result.Warnings) != 0 {
		t.Fatalf("Warnings = %+v", result.Warnings)
	}
	if !filepath.IsAbs(filepath.Join(root, "studies", "demo", "summary.csv")) {
		t.Fatal("fixture path assumption broken")
	}
}

func TestWriteSummaryWarnsForMissingAndAmbiguousRatings(t *testing.T) {
	_, st := executionFixture(t)
	dimensions := []Dimension{{Number: "01", Slug: "structure"}}
	sources := []Source{{Name: "a", Kind: SourceKindDirectory}, {Name: "b", Kind: SourceKindDirectory}}
	writeReport(t, SourceReportPath(st, sources[1], dimensions[0]), "# Report\n\n## Rating\nRating: 3 and 9/10\n")

	result, err := WriteSummary(st, dimensions, sources)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Warnings) != 2 {
		t.Fatalf("Warnings = %+v", result.Warnings)
	}
	content, err := os.ReadFile(result.Path)
	if err != nil {
		t.Fatal(err)
	}
	want := "source,01-structure,total\n" +
		"a,,0\n" +
		"b,,0\n"
	if string(content) != want {
		t.Fatalf("summary:\n%s\nwant:\n%s", content, want)
	}
}

func TestWriteSummaryWarnsForRatingsOnSeparateLines(t *testing.T) {
	_, st := executionFixture(t)
	dimension := Dimension{Number: "01", Slug: "structure"}
	source := Source{Name: "repo", Kind: SourceKindDirectory}
	writeReport(t, SourceReportPath(st, source, dimension), "# Report\n\n## Rating\nRating: 7\nRating: 9\n")

	result, err := WriteSummary(st, []Dimension{dimension}, []Source{source})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Warnings) != 1 {
		t.Fatalf("Warnings = %+v", result.Warnings)
	}
	if result.Warnings[0].Message != "ambiguous rating" {
		t.Fatalf("warning = %+v", result.Warnings[0])
	}
	content, err := os.ReadFile(result.Path)
	if err != nil {
		t.Fatal(err)
	}
	want := "source,01-structure,total\nrepo,,0\n"
	if string(content) != want {
		t.Fatalf("summary:\n%s\nwant:\n%s", content, want)
	}
}

func TestWriteSummaryFailureDoesNotReplaceExistingSummary(t *testing.T) {
	_, st := executionFixture(t)
	path := SummaryPath(st)
	writeReport(t, path, "existing summary\n")
	if err := os.Chmod(st.Path, 0o500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(st.Path, 0o700)

	_, err := WriteSummary(st, []Dimension{{Number: "01", Slug: "structure"}}, []Source{{Name: "repo", Kind: SourceKindDirectory}})
	if err == nil {
		t.Fatal("expected summary write failure")
	}
	content, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(content) != "existing summary\n" {
		t.Fatalf("summary was replaced: %q", content)
	}
}
