package sprint

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strings"
)

const qaContextBlockMaxBytes = 8 << 10

func BuildQAFoundation(manifest ReviewManifest, governed, implementation, review string, checks []QAApprovedCheckRef, budgets QABudgets, reviewArtifact string) (QAFoundation, error) {
	if !validFingerprint(governed) || !validFingerprint(implementation) || !validFingerprint(review) {
		return QAFoundation{}, fmt.Errorf("QA foundation requires current governed, implementation, and review fingerprints")
	}
	var candidates []QAContextBlock
	for _, coverage := range manifest.Coverage {
		if content := manifest.Contents[coverage.Path]; strings.TrimSpace(content) != "" {
			candidates = append(candidates, newQAContextBlock("acceptance", coverage.Path, content, "review coverage "+coverage.ID, []string{coverage.ID}, nil))
		}
	}
	for _, input := range manifest.Inputs {
		content := manifest.Contents[input.Path]
		if strings.TrimSpace(content) == "" {
			continue
		}
		switch input.ID {
		case "requirements":
			candidates = append(candidates, newQAContextBlock("requirements", input.Path, content, "governed requirements", nil, nil))
		case "execution-handoff":
			candidates = append(candidates, newQAContextBlock("prior", input.Path, content, "frozen execution handoff", nil, manifest.ChangedPaths))
		case "implementation-diff":
			candidates = append(candidates, qaDiffBlocks(content)...)
		}
	}
	if strings.TrimSpace(reviewArtifact) != "" {
		candidates = append(candidates, newQAContextBlock("prior", ArtifactRelPath(Sprint{Project: manifest.Project, Slug: manifest.Sprint}, StageReview), reviewArtifact, "current conformance review", nil, manifest.ChangedPaths))
	}
	sourcePaths := append([]string(nil), manifest.ChangedPaths...)
	for path := range manifest.Contents {
		if strings.HasPrefix(path, "target/") {
			sourcePaths = append(sourcePaths, strings.TrimPrefix(path, "target/"))
		}
	}
	for _, rel := range normalizeQAStrings(sourcePaths) {
		content := manifest.Contents["target/"+rel]
		if strings.TrimSpace(content) == "" {
			continue
		}
		block := newQAContextBlock("source", rel, content, "frozen implementation target", nil, []string{rel})
		block.Symbols, block.RelatedPaths = qaSourceRelationships(rel, content, block.RelatedPaths)
		candidates = append(candidates, block)
	}
	checkJSON, err := canonicalQAJSON(checks)
	if err != nil {
		return QAFoundation{}, err
	}
	candidates = append(candidates, newQAContextBlock("evidence", "", string(checkJSON), "approved product-owned check catalog", nil, manifest.ChangedPaths))
	authority, err := canonicalQAJSON(struct {
		Readable []string `json:"readable_paths"`
		Writable []string `json:"writable_paths"`
		Policy   string   `json:"live_read_policy"`
		Budget   int      `json:"prompt_bytes"`
	}{normalizeQAStrings(manifest.ChangedPaths), nil, "frozen blocks first; bounded live reads only for declared omissions", budgets.PromptBytes})
	if err != nil {
		return QAFoundation{}, err
	}
	candidates = append(candidates, newQAContextBlock("authority", "", string(authority), "product QA policy", nil, manifest.ChangedPaths))

	priority := map[string]int{"change": 0, "source": 1, "acceptance": 2, "requirements": 3, "prior": 4, "evidence": 5, "authority": 6}
	sort.SliceStable(candidates, func(i, j int) bool {
		if priority[candidates[i].Kind] != priority[candidates[j].Kind] {
			return priority[candidates[i].Kind] < priority[candidates[j].Kind]
		}
		if candidates[i].Path != candidates[j].Path {
			return candidates[i].Path < candidates[j].Path
		}
		return candidates[i].ID < candidates[j].ID
	})
	limit := budgets.PromptBytes - (64 << 10)
	if limit < 64<<10 {
		limit = 64 << 10
	}
	foundation := QAFoundation{SchemaVersion: QASchemaVersion, GovernedInputFingerprint: governed, ImplementationFingerprint: implementation, ReviewFingerprint: review, ApprovedChecks: append([]QAApprovedCheckRef(nil), checks...)}
	for _, block := range candidates {
		encoded, _ := json.Marshal(block)
		if foundation.TotalBytes+len(encoded) > limit {
			foundation.Omissions = append(foundation.Omissions, fmt.Sprintf("%s:%s omitted by the %d-byte foundation budget", block.Kind, block.Path, limit))
			continue
		}
		foundation.Blocks = append(foundation.Blocks, block)
		foundation.TotalBytes += len(encoded)
	}
	fingerprintInput := foundation
	fingerprintInput.ID, fingerprintInput.Fingerprint = "", ""
	fingerprint, err := fingerprintQAValue(fingerprintInput)
	if err != nil {
		return QAFoundation{}, err
	}
	foundation.Fingerprint = fingerprint
	foundation.ID = QAIDScope + "-foundation-" + fingerprint[:24]
	return foundation, ValidateQAFoundation(foundation)
}

func newQAContextBlock(kind, path, content, provenance string, expectations, related []string) QAContextBlock {
	content = strings.TrimSpace(content)
	omittedBytes := 0
	if len(content) > qaContextBlockMaxBytes {
		omittedBytes = len(content) - qaContextBlockMaxBytes
		half := (qaContextBlockMaxBytes - len("\n\n[... bounded excerpt ...]\n\n")) / 2
		content = content[:half] + "\n\n[... bounded excerpt ...]\n\n" + content[len(content)-half:]
	}
	digest := sha256.Sum256([]byte(content))
	hexDigest := hex.EncodeToString(digest[:])
	identityDigest := sha256.Sum256([]byte(kind + "\x00" + filepath.ToSlash(path) + "\x00" + content))
	identityHex := hex.EncodeToString(identityDigest[:])
	lines := 0
	if content != "" {
		lines = strings.Count(content, "\n") + 1
	}
	return QAContextBlock{ID: QAIDScope + "-block-" + identityHex[:24], Kind: kind, Path: filepath.ToSlash(path), StartLine: 1, EndLine: lines, Content: content, ContentSHA256: hexDigest, Provenance: provenance, ExpectationRefs: normalizeQAStrings(expectations), RelatedPaths: normalizeQAStrings(related), OmittedBytes: omittedBytes}
}

func qaDiffBlocks(patch string) []QAContextBlock {
	parts := strings.Split(patch, "diff --git ")
	blocks := make([]QAContextBlock, 0, len(parts))
	for _, part := range parts[1:] {
		content := "diff --git " + part
		line := strings.SplitN(part, "\n", 2)[0]
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		path := strings.TrimPrefix(fields[1], "b/")
		blocks = append(blocks, newQAContextBlock("change", path, content, "baseline-to-sprint diff hunk", nil, []string{path}))
	}
	return blocks
}

func qaSourceRelationships(path, content string, related []string) ([]string, []string) {
	if !strings.HasSuffix(path, ".go") {
		return nil, normalizeQAStrings(related)
	}
	file, err := parser.ParseFile(token.NewFileSet(), path, content, 0)
	if err != nil {
		return nil, normalizeQAStrings(related)
	}
	var symbols []string
	for _, decl := range file.Decls {
		switch value := decl.(type) {
		case *ast.FuncDecl:
			symbols = append(symbols, value.Name.Name)
		case *ast.GenDecl:
			for _, spec := range value.Specs {
				if named, ok := spec.(*ast.TypeSpec); ok {
					symbols = append(symbols, named.Name.Name)
				}
			}
		}
	}
	for _, imported := range file.Imports {
		value := strings.Trim(imported.Path.Value, `"`)
		if strings.Contains(value, "/") {
			related = append(related, value)
		}
	}
	return normalizeQAStrings(symbols), normalizeQAStrings(related)
}

func ValidateQAFoundation(value QAFoundation) error {
	if value.SchemaVersion != QASchemaVersion || !validFingerprint(value.Fingerprint) || !validFingerprint(value.GovernedInputFingerprint) || !validFingerprint(value.ImplementationFingerprint) || !validFingerprint(value.ReviewFingerprint) || len(value.Blocks) == 0 || value.TotalBytes <= 0 {
		return fmt.Errorf("invalid QA foundation identity or content")
	}
	seen := map[string]bool{}
	for _, block := range value.Blocks {
		if seen[block.ID] || !strings.HasPrefix(block.ID, QAIDScope+"-block-") || !validFingerprint(block.ContentSHA256) || strings.TrimSpace(block.Content) == "" || strings.TrimSpace(block.Provenance) == "" {
			return fmt.Errorf("invalid or duplicate QA context block %q", block.ID)
		}
		seen[block.ID] = true
		digest := sha256.Sum256([]byte(block.Content))
		if hex.EncodeToString(digest[:]) != block.ContentSHA256 {
			return fmt.Errorf("QA context block %q digest mismatch", block.ID)
		}
	}
	return nil
}

func qaProjectFoundation(foundation *QAFoundation, shard QAShard) []QAContextBlock {
	if foundation == nil {
		return nil
	}
	paths := normalizeQAStrings(append(append([]string(nil), shard.ChangedPaths...), shard.ContextPaths...))
	expectations := normalizeQAStrings(shard.ExpectationRefs)
	citedBlocks := stringSet(shard.ContextBlockIDs)
	selected := make([]QAContextBlock, 0)
	for _, block := range foundation.Blocks {
		include := citedBlocks[block.ID] || block.Kind == "authority" || block.Kind == "evidence"
		if !include && (shareQAString(block.RelatedPaths, paths) || containsQAString(paths, block.Path)) {
			include = true
		}
		if !include && shareQAString(block.ExpectationRefs, expectations) {
			include = true
		}
		if !include && block.Kind == "requirements" {
			include = true
		}
		if include {
			selected = append(selected, block)
		}
	}
	return selected
}

func qaContextBlockIDs(blocks []QAContextBlock) []string {
	ids := make([]string, 0, len(blocks))
	for _, block := range blocks {
		ids = append(ids, block.ID)
	}
	return normalizeQAStrings(ids)
}

func qaShardPackComplete(qaMap QAMap, shard QAShard) bool {
	blocks := qaProjectFoundation(qaMap.Foundation, shard)
	if len(blocks) == 0 || qaMap.Foundation == nil {
		return false
	}
	sources := map[string]bool{}
	expectations := map[string]bool{}
	for _, block := range blocks {
		if block.OmittedBytes > 0 {
			return false
		}
		if block.Kind == "source" {
			sources[block.Path] = true
		}
		for _, ref := range block.ExpectationRefs {
			expectations[ref] = true
		}
	}
	for _, path := range normalizeQAStrings(append(append([]string(nil), shard.ChangedPaths...), shard.ContextPaths...)) {
		if !sources[path] {
			return false
		}
	}
	for _, ref := range shard.ExpectationRefs {
		if !expectations[ref] {
			return false
		}
	}
	return len(qaMap.Foundation.Omissions) == 0
}
