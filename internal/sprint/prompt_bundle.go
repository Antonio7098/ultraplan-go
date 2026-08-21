package sprint

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

const promptExplanationSchemaVersion = 1

// PromptBlockExplanation exposes product-owned prompt structure without
// exposing additional content. Digests and byte counts make cache behavior
// auditable while the rendered Markdown remains the only provider payload.
type PromptBlockExplanation struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Cacheable bool   `json:"cacheable"`
	Bytes     int    `json:"bytes"`
	Digest    string `json:"sha256"`
}

// PromptExplanation describes the exact stable/volatile split of a prompt.
// CacheTransport is deliberately candid: the current OpenCode/agentwrap
// adapter receives the directive as metadata but cannot place a provider-native
// breakpoint inside its single user message yet.
type PromptExplanation struct {
	SchemaVersion      int                      `json:"schema_version"`
	TotalBytes         int                      `json:"total_bytes"`
	SharedPrefixBytes  int                      `json:"shared_prefix_bytes"`
	StageSuffixBytes   int                      `json:"stage_suffix_bytes"`
	SharedPrefixDigest string                   `json:"shared_prefix_sha256,omitempty"`
	CacheKey           string                   `json:"cache_key,omitempty"`
	CacheBreakpoint    int                      `json:"cache_breakpoint_bytes,omitempty"`
	CacheCandidate     bool                     `json:"cache_candidate"`
	CacheTransport     string                   `json:"cache_transport"`
	InputContract      *StageInputContract      `json:"input_contract,omitempty"`
	Blocks             []PromptBlockExplanation `json:"blocks"`
}

type promptBlock struct {
	id, kind  string
	cacheable bool
	content   string
}

type promptBundle struct{ blocks []promptBlock }

func (b *promptBundle) append(id, kind string, cacheable bool, content string) {
	if content == "" {
		return
	}
	b.blocks = append(b.blocks, promptBlock{id: id, kind: kind, cacheable: cacheable, content: content})
}

func (b promptBundle) render() string {
	var out strings.Builder
	for _, block := range b.blocks {
		out.WriteString(block.content)
	}
	return out.String()
}

func (b promptBundle) explain() PromptExplanation {
	prompt := b.render()
	explanation := PromptExplanation{SchemaVersion: promptExplanationSchemaVersion, TotalBytes: len(prompt), CacheTransport: "agentwrap-metadata-only"}
	var prefix strings.Builder
	for _, block := range b.blocks {
		sum := sha256.Sum256([]byte(block.content))
		explanation.Blocks = append(explanation.Blocks, PromptBlockExplanation{
			ID: block.id, Kind: block.kind, Cacheable: block.cacheable, Bytes: len(block.content), Digest: hex.EncodeToString(sum[:]),
		})
		if block.cacheable {
			prefix.WriteString(block.content)
		}
	}
	if prefix.Len() > 0 {
		sum := sha256.Sum256([]byte(prefix.String()))
		explanation.SharedPrefixBytes = prefix.Len()
		explanation.StageSuffixBytes = len(prompt) - prefix.Len()
		explanation.SharedPrefixDigest = hex.EncodeToString(sum[:])
		explanation.CacheKey = "ultraplan-sprint-v1-" + explanation.SharedPrefixDigest[:32]
		explanation.CacheBreakpoint = prefix.Len()
		explanation.CacheCandidate = true
	} else {
		explanation.StageSuffixBytes = len(prompt)
	}
	return explanation
}

func explainComposedPrompt(prompt string) PromptExplanation {
	boundaryEnd := strings.Index(prompt, sharedPromptStageBoundary)
	if boundaryEnd < 0 {
		var b promptBundle
		b.append("stage", "stage", false, prompt)
		return b.explain()
	}
	boundaryEnd += len(sharedPromptStageBoundary)
	var b promptBundle
	prefix := prompt[:boundaryEnd]
	cursor := 0
	appendThrough := func(id, kind, marker string) bool {
		end := strings.Index(prefix[cursor:], marker)
		if end < 0 {
			return false
		}
		end += cursor + len(marker)
		b.append(id, kind, true, prefix[cursor:end])
		cursor = end
		return true
	}
	if start := strings.Index(prefix, sharedRequirementsOpen); start >= 0 {
		b.append("shared-instructions", "governance", true, prefix[:start])
		cursor = start
		if !appendThrough("requirements", "artifact", sharedRequirementsClose) || !appendThrough("code-context", "artifact", sharedCodeContextClose) || !appendThrough("source-evidence", "source", sharedSourceEvidenceClose) {
			b.blocks = nil
			b.append("sprint-foundation", "shared-prefix", true, prefix)
			cursor = len(prefix)
		}
	} else {
		b.append("sprint-foundation", "shared-prefix", true, prefix)
		cursor = len(prefix)
	}
	if cursor < len(prefix) {
		b.append("stage-boundary", "boundary", true, prefix[cursor:])
	}
	b.append("stage", "stage", false, prompt[boundaryEnd:])
	return b.explain()
}

// ExplainPrompt returns a content-free structural explanation for an already
// rendered prompt. It is used by CLI previews that predate shared context.
func ExplainPrompt(prompt string) PromptExplanation { return explainComposedPrompt(prompt) }
