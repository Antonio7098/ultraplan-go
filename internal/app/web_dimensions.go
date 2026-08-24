package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Antonio7098/ultraplan-go/internal/study"
	"github.com/Antonio7098/ultraplan-go/internal/workspace"
)

// WebDimensionContentLimit bounds the markdown content returned per dimension.
const WebDimensionContentLimit = 64 * 1024

// WebDimensionQueries is an additive read-only capability used by study
// dimension browsing surfaces without widening the compatibility-critical
// WebQueries interface implemented by existing embedders.
type WebDimensionQueries interface {
	StudyDimensions(context.Context, string) (WebDimensionsResult, error)
}

// WebDimension describes one study analysis dimension with its bounded
// markdown content so browsers can expand cards without further requests.
type WebDimension struct {
	Study                 string
	Number                string
	Slug                  string
	Title                 string
	DisplayPath           string
	Content               string
	Bytes                 int64
	Truncated             bool
	CodeCitationsDisabled bool
}

// WebDimensionsResult is the bounded dimension listing shown on study pages.
type WebDimensionsResult struct {
	Items []WebDimension
	CollectionInfo
}

// StudyDimensions lists every analysis dimension of one study ordered by
// dimension number. Content is bounded per dimension and the collection
// itself is bounded like every other web listing.
func (u *webUseCases) StudyDimensions(ctx context.Context, name string) (WebDimensionsResult, error) {
	if err := ctx.Err(); err != nil {
		return WebDimensionsResult{}, err
	}
	listing, err := study.NewService(u.root).ListStudy(name)
	if err != nil {
		return WebDimensionsResult{}, fmt.Errorf("%w: study dimensions", ErrWebNotFound)
	}
	total := len(listing.Dimensions)
	items := make([]WebDimension, 0, total)
	for _, dimension := range listing.Dimensions {
		if len(items) == WebCollectionLimit {
			break
		}
		entry, err := u.webDimension(name, dimension)
		if err != nil {
			return WebDimensionsResult{}, err
		}
		items = append(items, entry)
	}
	return WebDimensionsResult{Items: items, CollectionInfo: collectionInfo(len(items), total)}, nil
}

func (u *webUseCases) webDimension(studyName string, dimension study.Dimension) (WebDimension, error) {
	content, size, truncated, err := readBoundedFile(dimension.Path, WebDimensionContentLimit)
	if err != nil {
		return WebDimension{}, fmt.Errorf("read dimension %s: %w", dimension.Path, err)
	}
	displayPath := ""
	if rel, relErr := filepath.Rel(u.root, dimension.Path); relErr == nil {
		displayPath = filepath.ToSlash(rel)
	}
	return WebDimension{
		Study:                 studyName,
		Number:                dimension.Number,
		Slug:                  dimension.Slug,
		Title:                 dimensionTitle(content),
		DisplayPath:           displayPath,
		Content:               content,
		Bytes:                 size,
		Truncated:             truncated,
		CodeCitationsDisabled: dimension.DisableCodeCitations,
	}, nil
}

func readBoundedFile(path string, limit int64) (string, int64, bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", 0, false, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return "", 0, false, os.ErrInvalid
	}
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return "", 0, false, err
	}
	truncated := int64(len(data)) > limit
	if truncated {
		data = data[:limit]
	}
	return string(data), info.Size(), truncated, nil
}

func dimensionTitle(content string) string {
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "# ") {
			continue
		}
		title := strings.TrimSpace(strings.TrimPrefix(trimmed, "# "))
		title = strings.Trim(title, "`*_ ")
		if title != "" {
			return title
		}
	}
	return ""
}

// WebStudyReportQueries is an additive read-only capability exposing the
// generated study reports without widening the compatibility-critical
// WebQueries interface implemented by existing embedders.
type WebStudyReportQueries interface {
	StudyReports(context.Context, string) (WebStudyReportsResult, error)
	StudyRepos(context.Context, string) (WebStudyReposResult, error)
}

// WebStudyReportFile links one existing report file on disk to its preview.
type WebStudyReportFile struct {
	Source      string
	Ref         string
	DisplayPath string
	Bytes       int64
}

// WebStudyDimensionReports groups one dimension's final report with its
// per-source (dimension × repo) reports; only files that exist are listed.
type WebStudyDimensionReports struct {
	Number  string
	Slug    string
	Final   *WebStudyReportFile
	Sources []WebStudyReportFile
}

// WebStudyReportsResult is the bounded report listing shown on study pages.
type WebStudyReportsResult struct {
	Dimensions []WebStudyDimensionReports
	CollectionInfo
}

// StudyReports lists every generated report for one study: the final report of
// each dimension plus every per-source report underneath it, ordered by
// dimension number and source name.
func (u *webUseCases) StudyReports(ctx context.Context, name string) (WebStudyReportsResult, error) {
	if err := ctx.Err(); err != nil {
		return WebStudyReportsResult{}, err
	}
	service := study.NewService(u.root)
	listing, err := service.ListStudy(name)
	if err != nil {
		return WebStudyReportsResult{}, fmt.Errorf("%w: study reports", ErrWebNotFound)
	}
	dimensions := make([]WebStudyDimensionReports, 0, len(listing.Dimensions))
	total := 0
	collect := func(path string) (WebStudyReportFile, bool) {
		if total == WebCollectionLimit {
			return WebStudyReportFile{}, false
		}
		file, ok := u.webReportFile(path)
		if !ok {
			return WebStudyReportFile{}, false
		}
		total++
		return file, true
	}
	for _, dimension := range listing.Dimensions {
		if err := ctx.Err(); err != nil {
			return WebStudyReportsResult{}, err
		}
		entry := WebStudyDimensionReports{Number: dimension.Number, Slug: dimension.Slug}
		if final, ok := collect(study.FinalReportPath(listing.Study, dimension)); ok {
			entry.Final = &final
		}
		for _, source := range listing.Sources {
			if report, ok := collect(study.SourceReportPath(listing.Study, source, dimension)); ok {
				entry.Sources = append(entry.Sources, report)
			}
		}
		dimensions = append(dimensions, entry)
	}
	return WebStudyReportsResult{Dimensions: dimensions, CollectionInfo: collectionInfo(total, total)}, nil
}

func (u *webUseCases) webReportFile(path string) (WebStudyReportFile, bool) {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return WebStudyReportFile{}, false
	}
	rel := workspace.Rel(u.root, path)
	if !supportedPreviewPath(rel) {
		return WebStudyReportFile{}, false
	}
	source := strings.TrimSuffix(filepath.Base(path), ".md")
	return WebStudyReportFile{
		Source:      source,
		Ref:         u.issue("artifact", rel),
		DisplayPath: filepath.ToSlash(rel),
		Bytes:       info.Size(),
	}, true
}
