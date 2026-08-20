package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscoverPrecedenceAndParents(t *testing.T) {
	explicit := t.TempDir()
	env := t.TempDir()
	child := filepath.Join(env, "a", "b")
	if _, err := Init(explicit); err != nil {
		t.Fatal(err)
	}
	if _, err := Init(env); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatal(err)
	}

	root, err := Discover(DiscoverOptions{ExplicitPath: explicit, EnvWorkspace: env, StartDir: child})
	if err != nil {
		t.Fatal(err)
	}
	if root.Path != explicit {
		t.Fatalf("root = %q, want explicit %q", root.Path, explicit)
	}

	root, err = Discover(DiscoverOptions{EnvWorkspace: env, StartDir: explicit})
	if err != nil {
		t.Fatal(err)
	}
	if root.Path != env {
		t.Fatalf("root = %q, want env %q", root.Path, env)
	}

	root, err = Discover(DiscoverOptions{StartDir: child})
	if err != nil {
		t.Fatal(err)
	}
	if root.Path != env {
		t.Fatalf("root = %q, want parent %q", root.Path, env)
	}
}

func TestResolveInsideRejectsEscape(t *testing.T) {
	root := t.TempDir()
	if _, err := ResolveInside(root, "prompts/base.md"); err != nil {
		t.Fatalf("inside rejected: %v", err)
	}
	if _, err := ResolveInside(root, "../outside"); err == nil {
		t.Fatal("expected escape error")
	}
}

func TestInitAndValidate(t *testing.T) {
	root := t.TempDir()
	plan, err := PlanInit(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Operations) == 0 {
		t.Fatal("expected operations")
	}
	if _, err := os.Stat(filepath.Join(root, MarkerFile)); !os.IsNotExist(err) {
		t.Fatalf("dry plan wrote marker: %v", err)
	}
	if _, err := Init(root); err != nil {
		t.Fatal(err)
	}
	result := Validate(root)
	if !result.Valid {
		t.Fatalf("validation issues: %v", result.Issues)
	}
	for _, rel := range []string{"prompts/base.md", "prompts/synthesize.md", "templates/repo-analysis.md", "templates/report.md"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); !os.IsNotExist(err) {
			t.Fatalf("init should not materialize optional override %s: %v", rel, err)
		}
	}
	defaultsPlan, err := InstallDefaults(root, DefaultsOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(defaultsPlan.Operations) == 0 {
		t.Fatal("expected defaults install operations")
	}
	repoTemplate, err := os.ReadFile(filepath.Join(root, "templates", "repo-analysis.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(repoTemplate), "# Source Analysis: {{source_name}}") {
		t.Fatalf("repo analysis template was not scaffolded from full template:\n%s", repoTemplate)
	}
	reportTemplate, err := os.ReadFile(filepath.Join(root, "templates", "report.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(reportTemplate), "# {{dimension_title}} - Combined Study Report") {
		t.Fatalf("report template was not scaffolded from full template:\n%s", reportTemplate)
	}
	basePrompt, err := os.ReadFile(filepath.Join(root, "prompts", "base.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(basePrompt), "# Base Dimension") || !strings.Contains(string(basePrompt), "Hard Rules") {
		t.Fatalf("base prompt was not scaffolded from full prototype:\n%s", basePrompt)
	}
	synthesizePrompt, err := os.ReadFile(filepath.Join(root, "prompts", "synthesize.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(synthesizePrompt), "# Synthesis Dimension") || !strings.Contains(string(synthesizePrompt), "Required Rating Summary") {
		t.Fatalf("synthesize prompt was not scaffolded from full prototype:\n%s", synthesizePrompt)
	}
	codeContextPrompt, err := os.ReadFile(filepath.Join(root, "prompts", "create-code-context.md"))
	if err != nil || !strings.Contains(string(codeContextPrompt), "# Create Sprint Code Context") {
		t.Fatalf("code-context prompt was not materialized from the embedded default: %v", err)
	}
	codeContextTemplate, err := os.ReadFile(filepath.Join(root, "templates", "code-context.md"))
	if err != nil || !strings.Contains(string(codeContextTemplate), "## Selected Source Excerpts") {
		t.Fatalf("code-context template was not materialized from the embedded default: %v", err)
	}
	plan, err = Init(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Operations) != 0 {
		t.Fatalf("idempotent init operations = %v", plan.Operations)
	}
}

func TestEmbeddedPromptsDoNotRequireManualPromptOrTemplateReads(t *testing.T) {
	for rel, content := range DefaultOverrideFiles() {
		if !strings.HasPrefix(rel, "prompts/") {
			continue
		}
		for _, forbidden := range []string{
			"../../prompts/",
			"../../templates/",
			"../../dimensions/",
			".ultra/system/templates/",
			"Read `../../",
		} {
			if strings.Contains(content, forbidden) {
				t.Fatalf("%s contains manual prompt/template read instruction %q", rel, forbidden)
			}
		}
	}
}

func TestCodeContextDefaultsEmbeddedAndMaterialized(t *testing.T) {
	root := t.TempDir()
	if _, err := Init(root); err != nil {
		t.Fatal(err)
	}
	if _, err := InstallDefaults(root, DefaultsOptions{}); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"prompts/create-code-context.md", "templates/code-context.md"} {
		embedded, ok := DefaultOverrideFile(rel)
		if !ok || strings.TrimSpace(embedded) == "" {
			t.Fatalf("embedded default missing: %s", rel)
		}
		materialized, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil || string(materialized) != embedded {
			t.Fatalf("materialized default differs for %s: %v", rel, err)
		}
	}
}

func TestReviewDefaultsAreEmbeddedAndNotInitializedAsOverrides(t *testing.T) {
	prompt, ok := DefaultOverrideFile("prompts/review.md")
	if !ok || !strings.Contains(prompt, "Automated Sprint Review") {
		t.Fatal("embedded review prompt missing")
	}
	template, ok := DefaultOverrideFile("templates/review.md")
	if !ok || !strings.Contains(template, "## Final Assessment") {
		t.Fatal("embedded review template missing")
	}
	root := t.TempDir()
	if _, err := Init(root); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"prompts/review.md", "templates/review.md"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); !os.IsNotExist(err) {
			t.Fatalf("init materialized optional override %s: %v", rel, err)
		}
	}
}
