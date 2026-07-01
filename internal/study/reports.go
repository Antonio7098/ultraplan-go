package study

import (
	"path/filepath"
	"strings"
)

func SourceReportPath(study Study, source Source, dimension Dimension) string {
	return filepath.Join(study.Path, "reports", "source", dimension.Ref(), sourceReportFileName(source))
}

func FinalReportPath(study Study, dimension Dimension) string {
	return filepath.Join(study.Path, "reports", "final", dimension.Ref()+".md")
}

func sourceReportFileName(source Source) string {
	name := strings.TrimSuffix(source.Name, ".md")
	if name == "" {
		name = source.Name
	}
	return name + ".md"
}
