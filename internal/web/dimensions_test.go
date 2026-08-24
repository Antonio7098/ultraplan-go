package web

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/Antonio7098/ultraplan-go/internal/app"
)

func TestStudyDimensionsPageListsExpandableCards(t *testing.T) {
	root := t.TempDir()
	var stderr strings.Builder
	if status := app.Run(app.Config{Args: []string{"init-workspace", "--path", root}, Stderr: &stderr}); status != app.ExitOK {
		t.Fatalf("init status=%d stderr=%s", status, stderr.String())
	}
	writeIntegrationFile(t, root, "studies/research/sources/source.md", "# Source\n")
	writeIntegrationFile(t, root, "studies/research/dimensions/01-contract.md", "# Contract Boundary\n\nCheck every contract.\n")
	writeIntegrationFile(t, root, "studies/research/dimensions/02-scope.md", "# Scope Discipline\n\nKeep scope tight.\n")
	writeIntegrationFile(t, root, "studies/other/dimensions/03-unrelated.md", "# Unrelated\n")

	useCases := app.NewWebUseCases(root, app.WebUseCaseOptions{})
	h, err := NewHandler(HandlerOptions{Queries: useCases, Operations: useCases.(app.WebOperations), Authority: testAuthority})
	if err != nil {
		t.Fatal(err)
	}

	res := request(h, http.MethodGet, "/studies/research/dimensions", nil)
	if res.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	body := res.Body.String()
	for _, want := range []string{
		`href="/studies/research/inputs" aria-current="page"`,
		`data-dimension-search`,
		`data-dimension-card`,
		"Contract Boundary",
		"Scope Discipline",
		"studies/research/dimensions/01-contract.md",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("study dimensions page missing %q in %s", want, body)
		}
	}
	if got := strings.Count(body, `data-dimension-card`); got != 2 {
		t.Errorf("card count=%d body=%s", got, body)
	}
	if strings.Contains(body, "Unrelated") {
		t.Error("study dimensions page leaked dimensions from another study")
	}

	queries, ok := useCases.(app.WebDimensionQueries)
	if !ok {
		t.Fatal("shared app use cases do not expose dimension queries")
	}
	result, err := queries.StudyDimensions(context.Background(), "research")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 2 || result.ReturnedCount != 2 || result.TotalCount != 2 || result.Truncated {
		t.Fatalf("dimensions result=%+v", result)
	}
	first := result.Items[0]
	if first.Study != "research" || first.Number != "01" || first.Title != "Contract Boundary" || first.Truncated {
		t.Fatalf("first dimension=%+v", first)
	}
	if _, err := queries.StudyDimensions(context.Background(), "missing"); !errors.Is(err, app.ErrWebNotFound) {
		t.Fatalf("unknown study error=%v", err)
	}
}

func TestStudyReposPageRanksAndComparesRepos(t *testing.T) {
	root := t.TempDir()
	var stderr strings.Builder
	if status := app.Run(app.Config{Args: []string{"init-workspace", "--path", root}, Stderr: &stderr}); status != app.ExitOK {
		t.Fatalf("init status=%d stderr=%s", status, stderr.String())
	}
	writeIntegrationFile(t, root, "studies/research/sources/repo/config.yml", "key: value\n")
	writeIntegrationFile(t, root, "studies/research/sources/notes.md", "# Notes\n")
	writeIntegrationFile(t, root, "studies/research/dimensions/01-contract.md", "# Contract Boundary\n")
	writeIntegrationFile(t, root, "studies/research/dimensions/02-scope.md", "# Scope Discipline\n")
	report := func(rel, rating string) {
		writeIntegrationFile(t, root, rel, "## Findings\n\nSomething.\n\n## Rating\n\n"+rating+"\n")
	}
	// repo scores 9 and 7 -> avg 8.0; notes scores only 5 on contract.
	report("studies/research/reports/source/01-contract/repo.md", "9/10")
	report("studies/research/reports/source/02-scope/repo.md", "Rating: 7")
	report("studies/research/reports/source/01-contract/notes.md", "5/10")

	useCases := app.NewWebUseCases(root, app.WebUseCaseOptions{})
	h, err := NewHandler(HandlerOptions{Queries: useCases, Operations: useCases.(app.WebOperations), Authority: testAuthority})
	if err != nil {
		t.Fatal(err)
	}

	res := request(h, http.MethodGet, "/studies/research/repos", nil)
	if res.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	body := res.Body.String()
	for _, want := range []string{
		"Repo leaderboard",
		"All scores",
		"Dimension leaders",
		`href="/studies/research/results" aria-current="page"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("repos page missing %q in %s", want, body)
		}
	}

	queries, ok := useCases.(app.WebStudyReportQueries)
	if !ok {
		t.Fatal("shared app use cases do not expose report queries")
	}
	result, err := queries.StudyRepos(context.Background(), "research")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Repos) != 2 || result.Repos[0].Name != "repo" || result.Repos[1].Name != "notes.md" {
		t.Fatalf("ranking=%+v", result.Repos)
	}
	top := result.Repos[0]
	if top.Average != 8 || top.Total != 16 || top.RatedCount != 2 || top.Applicable != 2 || top.Best == nil || top.Best.Score != 9 {
		t.Fatalf("top repo=%+v best=%+v", top, top.Best)
	}

	page := request(h, http.MethodGet, "/studies/research/repos", nil).Body.String()
	for _, want := range []string{
		">repo</span>", // leaderboard name
		"8.0",
		"2 / 2 dimensions rated",
		"best: <code>01-contract · 9</code>",
		`style="--heat: 90%"`,
		`style="--heat: 70%"`,
		`style="--heat: 50%"`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("repos page missing %q in %s", want, page)
		}
	}
	if !strings.Contains(page, "/artifacts/") {
		t.Errorf("score cells do not link to reports: %s", page)
	}
}

func TestStudyReportsPageSplitsFinalAndSourceReports(t *testing.T) {
	root := t.TempDir()
	var stderr strings.Builder
	if status := app.Run(app.Config{Args: []string{"init-workspace", "--path", root}, Stderr: &stderr}); status != app.ExitOK {
		t.Fatalf("init status=%d stderr=%s", status, stderr.String())
	}
	writeIntegrationFile(t, root, "studies/research/sources/repo/config.yml", "key: value\n")
	writeIntegrationFile(t, root, "studies/research/sources/notes.md", "# Notes\n")
	writeIntegrationFile(t, root, "studies/research/dimensions/01-contract.md", "# Contract Boundary\n")
	writeIntegrationFile(t, root, "studies/research/dimensions/02-scope.md", "# Scope Discipline\n")
	writeIntegrationFile(t, root, "studies/research/reports/final/01-contract.md", "# Final contract report\n")
	writeIntegrationFile(t, root, "studies/research/reports/source/01-contract/repo.md", "# Repo report\n")
	writeIntegrationFile(t, root, "studies/research/reports/source/01-contract/notes.md", "# Notes report\n")

	useCases := app.NewWebUseCases(root, app.WebUseCaseOptions{})
	h, err := NewHandler(HandlerOptions{Queries: useCases, Operations: useCases.(app.WebOperations), Authority: testAuthority})
	if err != nil {
		t.Fatal(err)
	}

	res := request(h, http.MethodGet, "/studies/research/reports", nil)
	if res.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	body := res.Body.String()
	for _, want := range []string{
		`role="tablist" aria-label="Report kinds"`,
		`id="reports-tab-final" aria-selected="true"`,
		`id="reports-tab-source" aria-selected="false"`,
		`aria-controls="reports-panel-final" data-report-tab="final"`,
		`aria-controls="reports-panel-source" tabindex="-1" data-report-tab="source"`,
		`id="reports-panel-final" role="tabpanel"`,
		`id="reports-panel-source" role="tabpanel" aria-labelledby="reports-tab-source" hidden`,
		">Final <span class=\"tab-count\">1</span></button>",
		"Dimension × repo <span class=\"tab-count\">2</span></button>",
		"<code>01-contract</code><span>studies/research/reports/final/01-contract.md</span>",
		"<code>repo</code><span>studies/research/reports/source/01-contract/repo.md</span>",
		"<code>notes</code><span>studies/research/reports/source/01-contract/notes.md</span>",
		`href="/studies/research/results" aria-current="page"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("reports page missing %q in %s", want, body)
		}
	}
	if !strings.Contains(body, `/artifacts/`) {
		t.Errorf("report links do not point at the artifact preview route: %s", body)
	}

	reportQueries, ok := useCases.(app.WebStudyReportQueries)
	if !ok {
		t.Fatal("shared app use cases do not expose report queries")
	}
	result, err := reportQueries.StudyReports(context.Background(), "research")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Dimensions) != 2 || result.ReturnedCount != 3 || result.TotalCount != 3 {
		t.Fatalf("reports result=%+v", result)
	}
	first := result.Dimensions[0]
	if first.Number != "01" || first.Final == nil || len(first.Sources) != 2 {
		t.Fatalf("first dimension=%+v", first)
	}
	if first.Final.DisplayPath != "studies/research/reports/final/01-contract.md" || first.Final.Source != "01-contract" {
		t.Fatalf("final report=%+v", first.Final)
	}
	if second := result.Dimensions[1]; second.Number != "02" && second.Final != nil && len(second.Sources) != 0 {
		t.Fatalf("second dimension=%+v", second)
	}
}
