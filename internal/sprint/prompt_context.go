package sprint

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	maxSharedPromptPrefixBytes = 256 << 10
	sharedPromptSuffixReserve  = 64 << 10
	sharedSourceReadBuffer     = 32 << 10
)

type promptContextErrorKind string

const (
	promptContextInvalidPath  promptContextErrorKind = "invalid_path"
	promptContextContainment  promptContextErrorKind = "containment"
	promptContextFileKind     promptContextErrorKind = "file_kind"
	promptContextMissing      promptContextErrorKind = "missing_source"
	promptContextInvalidRange promptContextErrorKind = "invalid_range"
	promptContextChanged      promptContextErrorKind = "changed_during_read"
	promptContextEncoding     promptContextErrorKind = "invalid_encoding"
	promptContextBudget       promptContextErrorKind = "budget_exceeded"
)

type promptContextError struct {
	Kind      promptContextErrorKind
	Path      string
	LineRange string
	Allowed   int
	Observed  int
	Err       error
}

func (e *promptContextError) Error() string {
	location := e.Path
	if e.LineRange != "" {
		location += ":" + e.LineRange
	}
	if location == "" {
		location = "shared prompt context"
	}
	if e.Kind == promptContextBudget {
		return fmt.Sprintf("%s: %s (%d bytes observed; %d allowed)", e.Kind, location, e.Observed, e.Allowed)
	}
	if e.Err != nil {
		return fmt.Sprintf("%s: %s: %s", e.Kind, location, safePromptContextCause(e.Err))
	}
	return fmt.Sprintf("%s: %s", e.Kind, location)
}

func (e *promptContextError) Unwrap() error { return e.Err }

func safePromptContextCause(err error) string {
	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		return pathErr.Op + ": " + pathErr.Err.Error()
	}
	return err.Error()
}

type sharedContextReference struct {
	Name, Path, Lines, Symbol, Rationale string
}

type sharedLineRange struct{ Start, End int }

var codeContextSymbolRE = regexp.MustCompile(`(?im)^\s*-?\s*\*\*Symbol:\*\*\s*` + "`?" + `([^` + "`" + `\r\n]+)` + "`?" + `\s*$`)

func (s Service) prepareSharedPromptContext(ctx context.Context, sp Sprint, inputs PlanningInputs) (string, error) {
	// Pre-code-context workspaces retain the compatibility behavior established
	// when the stage was introduced. Once the artifact exists, all covered
	// agent-backed operations use the shared renderer and fail closed.
	if inputs.CodeContext == "" {
		return "", nil
	}
	if findings, err := s.codeContextPrerequisite(sp); err != nil {
		if len(findings) > 0 {
			return "", fmt.Errorf("shared prompt code-context prerequisite failed: %s: %w", formatValidationFindings(findings), err)
		}
		return "", fmt.Errorf("shared prompt code-context prerequisite failed: %w", err)
	}
	target, findings := s.resolveCodeContextTarget(inputs.ProjectIndex)
	if len(findings) > 0 {
		return "", fmt.Errorf("resolve shared prompt implementation target: %s", formatValidationFindings(findings))
	}
	return renderSharedPromptContext(ctx, sp, inputs.Requirements, inputs.CodeContext, target.Path)
}

func (s Service) composeSharedPrompt(ctx context.Context, sp Sprint, inputs PlanningInputs, preview PromptPreview) (PromptPreview, error) {
	prefix, err := s.prepareSharedPromptContext(ctx, sp, inputs)
	if err != nil {
		return PromptPreview{}, err
	}
	preview.Prompt, err = composeStagePromptChecked(prefix, preview.Prompt)
	if err != nil {
		return PromptPreview{}, err
	}
	return preview, nil
}

func composeStagePrompt(prefix, suffix string) string {
	if prefix == "" {
		return suffix
	}
	return prefix + suffix
}

func composeStagePromptChecked(prefix, suffix string) (string, error) {
	if prefix != "" && len(suffix) > sharedPromptSuffixReserve {
		return "", &promptContextError{Kind: promptContextBudget, Allowed: sharedPromptSuffixReserve, Observed: len(suffix), Err: errors.New("stage-specific suffix exceeds its reserved budget")}
	}
	return composeStagePrompt(prefix, suffix), nil
}

func renderSharedPromptContext(ctx context.Context, sp Sprint, requirements, codeContext, implementationRoot string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if findings := ValidateCodeContextContent(codeContext); len(findings) > 0 {
		return "", fmt.Errorf("shared prompt code-context artifact failed validation: %s", formatValidationFindings(findings))
	}
	references, err := parseSharedContextReferences(codeContext)
	if err != nil {
		return "", err
	}
	root, err := canonicalImplementationRoot(implementationRoot)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	b.Grow(min(maxSharedPromptPrefixBytes, len(requirements)+len(codeContext)+4096))
	b.WriteString(sharedPromptInstructions)
	fmt.Fprintf(&b, "Project: `%s`\nSprint: `%s`\n", sp.Project, sp.Slug)
	b.WriteString(sharedRequirementsOpen)
	b.WriteString(requirements)
	b.WriteString(sharedRequirementsClose)
	b.WriteString(sharedCodeContextOpen)
	b.WriteString(codeContext)
	b.WriteString(sharedCodeContextClose)
	b.WriteString(sharedSourceEvidenceOpen)
	if err := checkSharedPromptBudget(b.Len(), "", ""); err != nil {
		return "", err
	}
	for _, ref := range references {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		ranges, err := parseSharedLineRanges(ref.Path, ref.Lines)
		if err != nil {
			return "", err
		}
		available := maxSharedPromptPrefixBytes - b.Len() - len(sharedSourceEvidenceClose) - len(sharedPromptStageBoundary)
		if available < 0 {
			return "", budgetPromptContextError(ref.Path, ref.Lines, maxSharedPromptPrefixBytes, b.Len())
		}
		source, err := readSharedSource(ctx, root, ref.Path, ref.Lines, ranges, available)
		if err != nil {
			return "", err
		}
		var frame strings.Builder
		fmt.Fprintf(&frame, "\n<<< BEGIN UNTRUSTED TRANSIENT SOURCE EVIDENCE: %s:%s >>>\n", ref.Path, ref.Lines)
		fmt.Fprintf(&frame, "Selected entry: %s\n", ref.Name)
		if ref.Symbol != "" {
			fmt.Fprintf(&frame, "Symbol: %s\n", ref.Symbol)
		}
		fmt.Fprintf(&frame, "Rationale: %s\n", ref.Rationale)
		frame.WriteString("Source bytes:\n")
		frame.WriteString(source)
		fmt.Fprintf(&frame, "\n<<< END UNTRUSTED TRANSIENT SOURCE EVIDENCE: %s:%s >>>\n", ref.Path, ref.Lines)
		observed := b.Len() + frame.Len() + len(sharedSourceEvidenceClose) + len(sharedPromptStageBoundary)
		if observed > maxSharedPromptPrefixBytes {
			return "", budgetPromptContextError(ref.Path, ref.Lines, maxSharedPromptPrefixBytes, observed)
		}
		b.WriteString(frame.String())
	}
	b.WriteString(sharedSourceEvidenceClose)
	b.WriteString(sharedPromptStageBoundary)
	if err := checkSharedPromptBudget(b.Len(), "", ""); err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return b.String(), nil
}

func parseSharedContextReferences(content string) ([]sharedContextReference, error) {
	entries := codeContextEntries(sectionBody(content, "Selected Source References"))
	refs := make([]sharedContextReference, 0, len(entries))
	for _, entry := range entries {
		path := codeContextPathRE.FindStringSubmatch(entry.body)
		lines := codeContextLinesRE.FindStringSubmatch(entry.body)
		reason := codeContextReasonRE.FindStringSubmatch(entry.body)
		if len(path) != 2 || len(lines) != 2 || len(reason) != 2 {
			return nil, &promptContextError{Kind: promptContextInvalidRange, Path: entry.name, Err: errors.New("validated reference fields could not be parsed")}
		}
		ref := sharedContextReference{
			Name:      strings.TrimSpace(entry.name),
			Path:      strings.Trim(strings.TrimSpace(path[1]), "`"),
			Lines:     strings.Trim(strings.TrimSpace(lines[1]), "`"),
			Rationale: strings.TrimSpace(reason[1]),
		}
		if symbol := codeContextSymbolRE.FindStringSubmatch(entry.body); len(symbol) == 2 {
			ref.Symbol = strings.Trim(strings.TrimSpace(symbol[1]), "`")
		}
		refs = append(refs, ref)
	}
	return refs, nil
}

func parseSharedLineRanges(path, value string) ([]sharedLineRange, error) {
	var ranges []sharedLineRange
	for _, segment := range strings.Split(value, ",") {
		segment = strings.TrimSpace(segment)
		parts := strings.Split(segment, "-")
		start, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil || start < 1 || len(parts) > 2 {
			return nil, &promptContextError{Kind: promptContextInvalidRange, Path: path, LineRange: value, Err: errors.New("range must be a positive N or N-M")}
		}
		end := start
		if len(parts) == 2 {
			end, err = strconv.Atoi(strings.TrimSpace(parts[1]))
			if err != nil || end < start {
				return nil, &promptContextError{Kind: promptContextInvalidRange, Path: path, LineRange: value, Err: errors.New("range end precedes its start")}
			}
		}
		ranges = append(ranges, sharedLineRange{Start: start, End: end})
	}
	return ranges, nil
}

func canonicalImplementationRoot(root string) (string, error) {
	if root == "" || !filepath.IsAbs(root) {
		return "", &promptContextError{Kind: promptContextContainment, Err: errors.New("implementation root must be absolute")}
	}
	canonical, err := filepath.EvalSymlinks(filepath.Clean(root))
	if err != nil {
		return "", &promptContextError{Kind: promptContextContainment, Err: fmt.Errorf("resolve implementation root: %w", err)}
	}
	info, err := os.Stat(canonical)
	if err != nil {
		return "", &promptContextError{Kind: promptContextContainment, Err: fmt.Errorf("inspect implementation root: %w", err)}
	}
	if !info.IsDir() {
		return "", &promptContextError{Kind: promptContextFileKind, Err: errors.New("implementation root is not a directory")}
	}
	return canonical, nil
}

func readSharedSource(ctx context.Context, root, rel, lineRange string, ranges []sharedLineRange, budget int) (string, error) {
	if err := validateRepositoryRelativePath(rel); err != nil {
		return "", &promptContextError{Kind: promptContextInvalidPath, Path: rel, LineRange: lineRange, Err: err}
	}
	candidate := filepath.Join(root, filepath.FromSlash(rel))
	if !inside(root, candidate) {
		return "", &promptContextError{Kind: promptContextContainment, Path: rel, LineRange: lineRange, Err: errors.New("path escapes implementation root")}
	}
	current := root
	for _, component := range strings.Split(filepath.ToSlash(rel), "/") {
		current = filepath.Join(current, filepath.FromSlash(component))
		info, err := os.Lstat(current)
		if err != nil {
			kind := promptContextMissing
			if !os.IsNotExist(err) {
				kind = promptContextContainment
			}
			return "", &promptContextError{Kind: kind, Path: rel, LineRange: lineRange, Err: err}
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", &promptContextError{Kind: promptContextContainment, Path: rel, LineRange: lineRange, Err: errors.New("symlink components are not allowed")}
		}
	}
	canonical, err := filepath.EvalSymlinks(candidate)
	if err != nil || !inside(root, canonical) {
		if err == nil {
			err = errors.New("canonical path escapes implementation root")
		}
		return "", &promptContextError{Kind: promptContextContainment, Path: rel, LineRange: lineRange, Err: err}
	}
	before, err := os.Stat(candidate)
	if err != nil {
		return "", &promptContextError{Kind: promptContextMissing, Path: rel, LineRange: lineRange, Err: err}
	}
	if !before.Mode().IsRegular() {
		return "", &promptContextError{Kind: promptContextFileKind, Path: rel, LineRange: lineRange, Err: errors.New("selected source is not a regular file")}
	}
	f, err := os.Open(candidate)
	if err != nil {
		return "", &promptContextError{Kind: promptContextMissing, Path: rel, LineRange: lineRange, Err: err}
	}
	defer f.Close()
	handleBefore, err := f.Stat()
	if err != nil || !handleBefore.Mode().IsRegular() || !os.SameFile(before, handleBefore) {
		if err == nil {
			err = errors.New("opened source identity does not match the selected path")
		}
		return "", &promptContextError{Kind: promptContextChanged, Path: rel, LineRange: lineRange, Err: err}
	}

	var out strings.Builder
	for _, selected := range ranges {
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return "", &promptContextError{Kind: promptContextInvalidRange, Path: rel, LineRange: lineRange, Err: err}
		}
		data, totalLines, err := readSharedLineRange(ctx, f, selected, budget-out.Len())
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return "", err
			}
			return "", &promptContextError{Kind: promptContextBudget, Path: rel, LineRange: lineRange, Allowed: budget, Observed: budget + 1, Err: err}
		}
		if selected.End > totalLines {
			return "", &promptContextError{Kind: promptContextInvalidRange, Path: rel, LineRange: lineRange, Err: fmt.Errorf("range ends at line %d but file has %d lines", selected.End, totalLines)}
		}
		out.Write(data)
	}
	if !utf8.ValidString(out.String()) {
		return "", &promptContextError{Kind: promptContextEncoding, Path: rel, LineRange: lineRange, Err: errors.New("selected source is not valid UTF-8")}
	}
	handleAfter, err := f.Stat()
	if err != nil {
		return "", &promptContextError{Kind: promptContextChanged, Path: rel, LineRange: lineRange, Err: err}
	}
	if err := verifySharedSourceUnchanged(root, candidate, canonical, rel, lineRange, handleBefore, handleAfter); err != nil {
		return "", err
	}
	return out.String(), nil
}

func verifySharedSourceUnchanged(root, candidate, canonical, rel, lineRange string, handleBefore, handleAfter os.FileInfo) error {
	after, err := os.Stat(candidate)
	if err != nil || !os.SameFile(handleBefore, handleAfter) || !os.SameFile(handleBefore, after) || handleBefore.Size() != handleAfter.Size() || !handleBefore.ModTime().Equal(handleAfter.ModTime()) {
		if err == nil {
			err = errors.New("selected source changed while it was read")
		}
		return &promptContextError{Kind: promptContextChanged, Path: rel, LineRange: lineRange, Err: err}
	}
	canonicalAfter, err := filepath.EvalSymlinks(candidate)
	if err != nil || canonicalAfter != canonical || !inside(root, canonicalAfter) {
		if err == nil {
			err = errors.New("selected source canonical location changed while it was read")
		}
		return &promptContextError{Kind: promptContextChanged, Path: rel, LineRange: lineRange, Err: err}
	}
	return nil
}

func readSharedLineRange(ctx context.Context, r io.Reader, selected sharedLineRange, budget int) ([]byte, int, error) {
	reader := bufio.NewReaderSize(r, sharedSourceReadBuffer)
	line := 1
	total := 0
	var out []byte
	for {
		if err := ctx.Err(); err != nil {
			return nil, 0, err
		}
		fragment, err := reader.ReadSlice('\n')
		if len(fragment) > 0 {
			total = line
			if line >= selected.Start && line <= selected.End {
				if len(fragment) > budget-len(out) {
					return nil, 0, errors.New("selected source exceeds remaining shared prompt budget")
				}
				out = append(out, fragment...)
			}
			if fragment[len(fragment)-1] == '\n' {
				line++
			}
		}
		switch err {
		case nil:
			continue
		case bufio.ErrBufferFull:
			continue
		case io.EOF:
			return out, total, nil
		default:
			return nil, 0, err
		}
	}
}

func checkSharedPromptBudget(observed int, path, lineRange string) error {
	if observed <= maxSharedPromptPrefixBytes {
		return nil
	}
	return budgetPromptContextError(path, lineRange, maxSharedPromptPrefixBytes, observed)
}

func budgetPromptContextError(path, lineRange string, allowed, observed int) error {
	return &promptContextError{Kind: promptContextBudget, Path: path, LineRange: lineRange, Allowed: allowed, Observed: observed}
}
