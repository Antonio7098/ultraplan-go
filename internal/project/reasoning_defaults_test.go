package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveReasoningDefaultPrecedence(t *testing.T) {
	root := workspaceFixture(t)

	builtin, err := ResolveReasoningDefault(root, "alpha", AreaReasoningPromptPath)
	if err != nil {
		t.Fatal(err)
	}
	if builtin.Source != "builtin:"+AreaReasoningPromptPath {
		t.Fatalf("builtin source = %q", builtin.Source)
	}

	writeFileContent(t, root, "# Workspace reasoning prompt\n", "prompts", "create-area-reasoning.md")
	workspaceDefault, err := ResolveReasoningDefault(root, "alpha", AreaReasoningPromptPath)
	if err != nil {
		t.Fatal(err)
	}
	if workspaceDefault.Source != "workspace:"+AreaReasoningPromptPath || !strings.Contains(workspaceDefault.Content, "Workspace") {
		t.Fatalf("workspace default = %+v", workspaceDefault)
	}

	writeFileContent(t, root, "# Alpha reasoning prompt\n", "projects", "alpha", "prompts", "create-area-reasoning.md")
	projectDefault, err := ResolveReasoningDefault(root, "alpha", AreaReasoningPromptPath)
	if err != nil {
		t.Fatal(err)
	}
	if projectDefault.Source != "project:projects/alpha/"+AreaReasoningPromptPath || !strings.Contains(projectDefault.Content, "Alpha") {
		t.Fatalf("project default = %+v", projectDefault)
	}

	betaDefault, err := ResolveReasoningDefault(root, "beta", AreaReasoningPromptPath)
	if err != nil {
		t.Fatal(err)
	}
	if betaDefault.Source != "workspace:"+AreaReasoningPromptPath {
		t.Fatalf("beta should not inherit alpha override: %+v", betaDefault)
	}
}

func TestResolveReasoningDefaultRejectsInvalidExistingProjectOverride(t *testing.T) {
	root := workspaceFixture(t)
	path := filepath.Join(root, "projects", "alpha", "templates", "sprint-reasoning.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(" \n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := ResolveReasoningDefault(root, "alpha", FinalReasoningTemplatePath)
	if err == nil || !strings.Contains(err.Error(), "is empty") {
		t.Fatalf("error = %v", err)
	}
}
