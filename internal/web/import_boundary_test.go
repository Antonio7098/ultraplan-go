package web

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestWebImportBoundary(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, spec := range parsed.Imports {
			path, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				t.Fatal(err)
			}
			if path == "github.com/Antonio7098/ultraplan-go/internal/app" || standardLibraryImport(path) {
				continue
			}
			t.Errorf("%s imports forbidden package %q", file, path)
		}
	}
}

func standardLibraryImport(path string) bool {
	first := strings.Split(path, "/")[0]
	return !strings.Contains(first, ".")
}
