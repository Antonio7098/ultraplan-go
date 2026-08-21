package runcontrol

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestRunControlImportBoundary(t *testing.T) {
	t.Parallel()
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
			first := strings.Split(path, "/")[0]
			standardLibrary := !strings.Contains(first, ".")
			if standardLibrary || path == "modernc.org/sqlite" || path == "golang.org/x/sys/unix" {
				continue
			}
			t.Errorf("%s imports forbidden package %q", file, path)
		}
	}
}
