package sprint

import (
	"fmt"
	"strings"
)

const maxPreparedHandoffBytes = 32 << 10

type handoffBuilder struct {
	body    strings.Builder
	omitted []string
}

func (h *handoffBuilder) add(label, path, content string, sections ...string) {
	h.addSelected(label, path, content, true, sections...)
}

func (h *handoffBuilder) addOptional(label, path, content string, sections ...string) {
	h.addSelected(label, path, content, false, sections...)
}

func (h *handoffBuilder) addSelected(label, path, content string, reportMissing bool, sections ...string) {
	extracted := selectedMarkdownSections(content, sections...)
	if strings.TrimSpace(extracted) == "" {
		if reportMissing {
			h.omitted = append(h.omitted, label+" (selected sections unavailable; read "+path+")")
		}
		return
	}
	block := fmt.Sprintf("\n### %s\n\nSource: `%s`\n\n%s", label, path, strings.TrimSpace(extracted))
	if h.body.Len()+len(block) > maxPreparedHandoffBytes {
		h.omitted = append(h.omitted, label+" (handoff byte budget reached; read "+path+")")
		return
	}
	h.body.WriteString(block)
	h.body.WriteString("\n")
}

func (h *handoffBuilder) render() string {
	if h.body.Len() == 0 && len(h.omitted) == 0 {
		return ""
	}
	var out strings.Builder
	out.WriteString("\n\n## UltraPlan Prepared Stage Handoff\n\n")
	out.WriteString("UltraPlan extracted the decision-relevant sections below in canonical dependency order. Use these copies directly; do not spend tool calls rereading their source files unless a required section is listed as omitted or the task needs material outside the prepared packet.\n")
	out.WriteString(h.body.String())
	if len(h.omitted) > 0 {
		out.WriteString("\nOmitted inputs:\n")
		for _, item := range h.omitted {
			fmt.Fprintf(&out, "- %s\n", item)
		}
	}
	return out.String()
}

func selectedMarkdownSections(content string, names ...string) string {
	sections := markdownSections(content)
	var out strings.Builder
	for _, name := range names {
		body := strings.TrimSpace(sections[name])
		if body == "" {
			continue
		}
		fmt.Fprintf(&out, "## %s\n\n%s\n\n", name, body)
	}
	return out.String()
}

func appendPreparedHandoff(preview PromptPreview, handoff string) PromptPreview {
	preview.Prompt += handoff
	return preview
}

func (s Service) finalReasoningHandoff(sp Sprint, manifest ReasoningManifest) string {
	var handoff handoffBuilder
	if handbook, err := s.store.ReadArtifact(sp, StageTechnicalHandbook); err == nil {
		handoff.add("Technical handbook conclusions", ArtifactRelPath(sp, StageTechnicalHandbook), handbook, "Relevant Patterns", "Trade-Offs", "Anti-Patterns And Warnings", "Open Questions For Reasoning", "Evidence Pointers")
	}
	for _, entry := range manifest.ReasoningTemplates {
		path, err := resolveSprintContained(s.root, sp, entry.OutputPath)
		if err != nil {
			handoff.omitted = append(handoff.omitted, entry.Name+" (unsafe path)")
			continue
		}
		content, err := s.store.ReadFile(path)
		if err != nil {
			handoff.omitted = append(handoff.omitted, entry.Name+" (read "+entry.OutputPath+")")
			continue
		}
		handoff.add(entry.Name+" area reasoning", entry.OutputPath, content, "Area Decisions", "Trade-Offs", "Evidence", "Risks")
	}
	return handoff.render()
}

func (s Service) planHandoff(sp Sprint, inputs PlanningInputs) string {
	var handoff handoffBuilder
	handoff.add("Requirements execution contract", ArtifactRelPath(sp, StageRequirements), inputs.Requirements, "Acceptance Criteria", "Constraints", "Review Expectations")
	s.addTechnicalHandbookExamples(&handoff, sp)
	if reasoning, err := s.store.ReadArtifact(sp, StageReasoning); err == nil {
		handoff.add("Final reasoning decisions", ArtifactRelPath(sp, StageReasoning), reasoning, "Final Decisions", "Decisions", "Expected Evidence", "Assumptions And Risks", "Implementation Constraints", "Plan Handoff")
	}
	return handoff.render()
}

func (s Service) executeHandoff(sp Sprint) string {
	var handoff handoffBuilder
	s.addTechnicalHandbookExamples(&handoff, sp)
	return handoff.render()
}

func (s Service) addTechnicalHandbookExamples(handoff *handoffBuilder, sp Sprint) {
	handbook, err := s.store.ReadArtifact(sp, StageTechnicalHandbook)
	if err != nil {
		return
	}
	handoff.addOptional(
		"Technical handbook examples worth investigating",
		ArtifactRelPath(sp, StageTechnicalHandbook),
		handbook,
		"Examples Worth Investigating",
		"Examples Worth Inspecting",
	)
}

func (s Service) smokeAuthorHandoff(sp Sprint, inputs PlanningInputs) string {
	var handoff handoffBuilder
	handoff.add("Acceptance and review contract", ArtifactRelPath(sp, StageRequirements), inputs.Requirements, "Acceptance Criteria", "Constraints", "Review Expectations")
	for _, artifact := range []struct {
		stage    PlanningStage
		label    string
		sections []string
	}{
		{StagePlan, "Planned verification", []string{"Evidence Checklist", "Verification Commands", "Risks And Blockers", "Completion Criteria"}},
		{StageExecute, "Execution evidence", []string{"Task Counts", "Tasks"}},
		{StageReview, "Review outcome", []string{"Verification Evidence", "Findings", "Deviations", "Final Assessment"}},
	} {
		content, err := s.store.ReadArtifact(sp, artifact.stage)
		if err != nil {
			handoff.omitted = append(handoff.omitted, artifact.label+" (read "+ArtifactRelPath(sp, artifact.stage)+")")
			continue
		}
		handoff.add(artifact.label, ArtifactRelPath(sp, artifact.stage), content, artifact.sections...)
	}
	return handoff.render()
}
