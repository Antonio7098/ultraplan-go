package tui

import (
	"embed"
	"strings"
)

// reasoningDocFS holds the project reasoning scaffold files that ship with
// UltraPlan. They are not workspace artifacts, so they bypass PreviewArtifact
// (which only resolves inside the active workspace) and are rendered as
// read-only previews on the project page.
//
//go:embed reasoning_docs/sprint-reasoning.md reasoning_docs/create-sprint-reasoning.md reasoning_docs/create-area-reasoning.md
var reasoningDocFS embed.FS

type reasoningDoc struct {
	Key     string
	Label   string
	Summary string
	Path    string
}

var reasoningDocs = []reasoningDoc{
	{
		Key:     "sprint-reasoning",
		Label:   "Sprint Reasoning Template",
		Summary: "Final-decision synthesis template written to reasoning.md.",
		Path:    "reasoning_docs/sprint-reasoning.md",
	},
	{
		Key:     "create-sprint-reasoning",
		Label:   "Create Sprint Reasoning Prompt",
		Summary: "Author instructions for the sprint reasoning agent.",
		Path:    "reasoning_docs/create-sprint-reasoning.md",
	},
	{
		Key:     "create-area-reasoning",
		Label:   "Create Area Reasoning Prompt",
		Summary: "Author instructions for per-area reasoning documents.",
		Path:    "reasoning_docs/create-area-reasoning.md",
	},
}

func reasoningDocByKey(key string) (reasoningDoc, bool) {
	for _, d := range reasoningDocs {
		if d.Key == key {
			return d, true
		}
	}
	return reasoningDoc{}, false
}

func loadReasoningDoc(key string) (string, bool) {
	doc, ok := reasoningDocByKey(key)
	if !ok {
		return "", false
	}
	data, err := reasoningDocFS.ReadFile(doc.Path)
	if err != nil {
		return "", false
	}
	return strings.TrimRight(string(data), "\n") + "\n", true
}