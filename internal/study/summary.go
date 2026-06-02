package study

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

type SummaryWarning struct {
	Path    string
	Message string
}

type SummaryResult struct {
	Path     string
	Warnings []SummaryWarning
}

func SummaryPath(study Study) string {
	return filepath.Join(study.Path, "summary.csv")
}

func WriteSummary(study Study, dimensions []Dimension, sources []Source) (SummaryResult, error) {
	path := SummaryPath(study)
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	header := []string{"source"}
	for _, dimension := range dimensions {
		header = append(header, dimension.Ref())
	}
	header = append(header, "total")
	if err := writer.Write(header); err != nil {
		return SummaryResult{}, err
	}
	result := SummaryResult{Path: path}
	for _, source := range sources {
		row := []string{source.Name}
		total := 0
		for _, dimension := range dimensions {
			if source.Kind == SourceKindMarkdown && !SourceAppliesToDimension(source, dimension) {
				row = append(row, "N/A")
				continue
			}
			score, warning := summaryScore(study, source, dimension)
			if warning.Message != "" {
				result.Warnings = append(result.Warnings, warning)
			}
			if score >= 0 {
				total += score
				row = append(row, strconv.Itoa(score))
			} else {
				row = append(row, "")
			}
		}
		row = append(row, strconv.Itoa(total))
		if err := writer.Write(row); err != nil {
			return SummaryResult{}, err
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return SummaryResult{}, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return SummaryResult{}, err
	}
	if err := atomicWriteFile(path, buf.Bytes(), "."+filepath.Base(path)+".*.tmp"); err != nil {
		return SummaryResult{}, fmt.Errorf("write summary %s: %w", path, err)
	}
	return result, nil
}

func atomicWriteFile(path string, content []byte, tempPattern string) error {
	temp, err := os.CreateTemp(filepath.Dir(path), tempPattern)
	if err != nil {
		return fmt.Errorf("create temporary file: %w", err)
	}
	tempPath := temp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tempPath)
		}
	}()
	if _, err := temp.Write(content); err != nil {
		_ = temp.Close()
		return fmt.Errorf("write temporary file %s: %w", tempPath, err)
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return fmt.Errorf("flush temporary file %s: %w", tempPath, err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close temporary file %s: %w", tempPath, err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("rename temporary file %s: %w", tempPath, err)
	}
	cleanup = false
	syncDir(filepath.Dir(path))
	return nil
}

func summaryScore(study Study, source Source, dimension Dimension) (int, SummaryWarning) {
	path := SourceReportPath(study, source, dimension)
	content, err := os.ReadFile(path)
	if err != nil {
		return -1, SummaryWarning{Path: path, Message: "missing report"}
	}
	rating := findRating(string(content))
	switch rating.State {
	case RatingStateValid:
		return rating.Score, SummaryWarning{}
	case RatingStateMissing:
		return -1, SummaryWarning{Path: path, Message: "missing rating"}
	case RatingStateAmbiguous:
		return -1, SummaryWarning{Path: path, Message: "ambiguous rating"}
	default:
		return -1, SummaryWarning{Path: path, Message: fmt.Sprintf("invalid rating: %s", rating.Reason)}
	}
}
