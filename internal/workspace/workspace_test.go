package workspace

import (
	"os"
	"path/filepath"
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
	plan, err = Init(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Operations) != 0 {
		t.Fatalf("idempotent init operations = %v", plan.Operations)
	}
}
