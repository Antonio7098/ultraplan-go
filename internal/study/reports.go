package study

import "path/filepath"

func SourceReportPath(study Study, source Source, dimension Dimension) string {
	return filepath.Join(study.Path, "reports", "source", sourceReportFileName(source, dimension))
}

func FinalReportPath(study Study) string {
	return filepath.Join(study.Path, "reports", "final", "report.md")
}

func sourceReportFileName(source Source, dimension Dimension) string {
	return source.Name + "-" + dimension.Ref() + ".md"
}
