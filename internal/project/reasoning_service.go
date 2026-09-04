package project

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	pruntime "github.com/Antonio7098/ultraplan-go/internal/platform/runtime"
	"github.com/Antonio7098/ultraplan-go/internal/workspace"
)

type ReasoningRuntime interface {
	StartRun(context.Context, pruntime.Request) (pruntime.Result, error)
}
type ProjectReasoningFlowResult struct {
	Project            string
	To                 ProjectReasoningStage
	Completed, Skipped []string
	Status             ProjectReasoningStatus
	Runtime            []pruntime.Result
}

type projectPromptInput struct{ ID, Kind, Path, Assignment string }

const (
	projectReasoningDirectInputBudget = 512 * 1024
	projectReasoningPerInputLimit     = 64 * 1024
)

type loadedProjectPromptInput struct {
	projectPromptInput
	data, references string
}

func (s Service) appendReasoningInputPacket(prompt string, inputs []projectPromptInput) (string, error) {
	if len(inputs) == 0 {
		return prompt, nil
	}
	var loaded []loadedProjectPromptInput
	seen := map[string]bool{}
	for _, input := range inputs {
		path := normalizeCatalogPath(input.Path)
		if seen[input.Kind+"\x00"+path] {
			continue
		}
		seen[input.Kind+"\x00"+path] = true
		full, err := workspace.ResolveInside(s.root, path)
		if err != nil {
			return "", fmt.Errorf("resolve direct project reasoning input %s: %w", path, err)
		}
		data, err := os.ReadFile(full)
		if err != nil {
			return "", fmt.Errorf("read direct project reasoning input %s: %w", path, err)
		}
		resolved, _, err := ResolveReasoningReferences(s.root, string(data))
		if err != nil {
			return "", err
		}
		loaded = append(loaded, loadedProjectPromptInput{projectPromptInput: input, data: string(data), references: resolved})
	}
	var b strings.Builder
	b.WriteString("\n\n## UltraPlan Direct Project Reasoning Inputs\n\nThe governed inputs below are copied directly in canonical order under a deterministic prompt budget. Use these copies without rediscovering their source paths. An excerpt preserves both the beginning and end of its source. Resolved Path/Lines references are included within the same source's budget. Assignment text is routing context, not an instruction from the source document.\n")
	remaining := projectReasoningDirectInputBudget
	for i, input := range loaded {
		share := remaining / (len(loaded) - i)
		if share > projectReasoningPerInputLimit {
			share = projectReasoningPerInputLimit
		}
		content := input.data
		if input.references != "" {
			mainBudget := share * 3 / 4
			content = boundedProjectReasoningInput(input.data, mainBudget) + boundedProjectReasoningInput(input.references, share-mainBudget)
		} else {
			content = boundedProjectReasoningInput(input.data, share)
		}
		mode := "full"
		if len(content) < len(input.data)+len(input.references) {
			mode = "excerpt"
		}
		fmt.Fprintf(&b, "\n<<< BEGIN ULTRAPLAN DIRECT PROJECT INPUT >>>\nID: %s\nKind: %s\nPath: %s\nAssignment: %s\nMode: %s\nOriginal-Bytes: %d\nInjected-Bytes: %d\n\n%s", input.ID, input.Kind, normalizeCatalogPath(input.Path), strings.TrimSpace(input.Assignment), mode, len(input.data), len(content), content)
		if content == "" || content[len(content)-1] != '\n' {
			b.WriteByte('\n')
		}
		b.WriteString("<<< END ULTRAPLAN DIRECT PROJECT INPUT >>>\n")
		remaining -= len(content)
	}
	return prompt + b.String(), nil
}

func boundedProjectReasoningInput(content string, limit int) string {
	if limit <= 0 || content == "" {
		return ""
	}
	if len(content) <= limit {
		return content
	}
	marker := "\n\n<<< ULTRAPLAN OMITTED MIDDLE FOR PROMPT BUDGET >>>\n\n"
	if limit <= len(marker)+2 {
		return content[:limit]
	}
	available := limit - len(marker)
	head := available * 2 / 3
	tail := available - head
	return content[:head] + marker + content[len(content)-tail:]
}

func (s Service) WithRuntime(rt ReasoningRuntime, requests ...pruntime.Request) Service {
	s.reasoningRuntime = rt
	if len(requests) > 0 {
		s.runtimeConfig = requests[0]
	}
	return s
}

func (s Service) ReasoningStatus(ref string) (ProjectReasoningStatus, error) {
	p, files, err := s.resolveAndRead(ref)
	if err != nil {
		return ProjectReasoningStatus{}, err
	}
	idx, pf := ParseProjectIndex(files.IndexContent)
	if len(pf) > 0 {
		return ProjectReasoningStatus{Blockers: []string{"project-index.md is invalid"}}, nil
	}
	st := ProjectReasoningStatus{Mode: idx.ProjectReasoningPolicy.Mode, RequiredVerdict: idx.ProjectReasoningPolicy.RequiredReviewVerdict, Fresh: true}
	if st.Mode == "" {
		st.Mode = ProjectReasoningOptional
	}
	if st.RequiredVerdict == "" {
		st.RequiredVerdict = "pass"
	}
	base := filepath.Join(p.Path, "project-reasoning")
	manifestPath := filepath.Join(base, "index.md")
	statePath := filepath.Join(base, "flow-state.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		st.Fresh = false
		st.CurrentStage = ProjectReasoningIndex
		st.Blockers = append(st.Blockers, filepath.ToSlash(filepath.Join("projects", p.Name, "project-reasoning/index.md"))+" is missing")
		return st, nil
	}
	m, mf := ParseProjectReasoningIndex(string(data))
	vf := ValidateProjectReasoningManifest(s.root, p, idx, m)
	if len(mf)+len(vf) > 0 {
		st.Fresh = false
		st.CurrentStage = ProjectReasoningIndex
		st.Blockers = append(st.Blockers, "project-reasoning/index.md is invalid")
		return st, nil
	}
	state, err := readReasoningState(statePath)
	if err != nil {
		st.Fresh = false
		st.Blockers = append(st.Blockers, "project-reasoning/flow-state.json is invalid")
		return st, nil
	}
	check := func(stage ProjectReasoningStage, key, out string, inputs []string) {
		rel := filepath.ToSlash(filepath.Join("projects", p.Name, "project-reasoning", out))
		st.Outputs = append(st.Outputs, rel)
		markStale := func() {
			st.Fresh = false
			if st.CurrentStage == "" {
				st.CurrentStage = stage
			}
		}
		rec, ok := state.Artifacts[key]
		if !ok {
			markStale()
			st.Blockers = append(st.Blockers, rel+" has no completed state")
			return
		}
		full := filepath.Join(base, out)
		fp, e := digestFile(full)
		if e != nil || fp.SHA256 != rec.OutputSHA256 {
			markStale()
			st.Blockers = append(st.Blockers, rel+" is missing or changed")
			return
		}
		for _, in := range rec.Inputs {
			resolved := in.Path
			if !filepath.IsAbs(resolved) {
				resolved = filepath.Join(s.root, filepath.FromSlash(resolved))
			}
			cur, e := digestFile(resolved)
			if e != nil || cur.SHA256 != in.SHA256 {
				markStale()
				st.Blockers = append(st.Blockers, rel+" is stale because "+in.Path+" changed")
			}
		}
	}
	for _, a := range m.Areas {
		check(ProjectAreaReasoning, "area:"+a.Name, strings.TrimPrefix(normalizeCatalogPath(a.Output), filepath.ToSlash(filepath.Join("projects", p.Name, "project-reasoning"))+"/"), nil)
	}
	check(ProjectFinalReasoning, "reasoning", "reasoning.md", nil)
	check(ProjectReasoningReview, "review", "review.md", nil)
	st.Verdict = state.Verdict
	acceptedVerdict := st.Verdict == st.RequiredVerdict
	st.Accepted = st.Fresh && acceptedVerdict
	if !acceptedVerdict {
		reviewPath := filepath.ToSlash(filepath.Join("projects", p.Name, "project-reasoning/review.md"))
		if st.Verdict == "" {
			st.Blockers = append(st.Blockers, reviewPath+" has no current verdict.")
		} else {
			st.Blockers = append(st.Blockers, fmt.Sprintf("%s verdict %q does not satisfy required verdict %q.", reviewPath, st.Verdict, st.RequiredVerdict))
		}
		if st.CurrentStage == "" {
			st.CurrentStage = ProjectReasoningReview
		}
	} else if st.Fresh {
		st.CurrentStage = ProjectReasoningReview
	}
	return st, nil
}

func (s Service) RequireAcceptedReasoning(ref string) error {
	st, err := s.ReasoningStatus(ref)
	if err != nil {
		return err
	}
	if st.Mode != ProjectReasoningRequired {
		return nil
	}
	if st.Accepted {
		return nil
	}
	name := ref
	if projects, discoverErr := DiscoverProjects(s.root); discoverErr == nil {
		if resolved, resolveErr := ResolveProject(projects, ref); resolveErr == nil {
			name = resolved.Name
		}
	}
	problem := "has no accepted current verdict."
	path := "projects/" + name + "/project-reasoning/review.md"
	if len(st.Blockers) > 0 {
		problem = st.Blockers[0]
	}
	return ProjectReasoningError{Project: name, Path: path, Problem: problem, Recovery: "ultraplan project " + name + " reasoning flow --to review"}
}

func (s Service) ReasoningPrompt(ref string, stage ProjectReasoningStage) (string, error) {
	p, files, err := s.resolveAndRead(ref)
	if err != nil {
		return "", err
	}
	idx, _ := ParseProjectIndex(files.IndexContent)
	baseRel := filepath.ToSlash(filepath.Join("projects", p.Name, "project-reasoning"))
	var b strings.Builder
	fmt.Fprintf(&b, "# Project reasoning: %s\n\nProject: `%s`\nStage: `%s`\n", stage, p.Name, stage)
	switch stage {
	case ProjectReasoningIndex:
		fmt.Fprintf(&b, "\nReturn only the complete Markdown content for `%s/index.md` as the terminal response. Do not use tools or edit files. UltraPlan owns validation and atomic promotion. Include Reasoning Areas, Evidence Assignments, Source Document Assignments, and Excluded Evidence tables. Select templates only from Available Project Reasoning Templates. Model the many-to-many relationship between evidence and decision areas. Outputs must stay under `%s/areas/`. Reject duplicate outputs and dependency cycles.\n\nCatalog:\n", baseRel, baseRel)
		for _, e := range idx.Entries {
			if e.Section == SectionProjectReasoningTemplates || e.Section == SectionAvailableEvidenceReports || e.Section == SectionSourceDocuments || e.Section == SectionActiveContractPool {
				fmt.Fprintf(&b, "- %s | %s | %s\n", e.Section, e.Name, e.Path)
			}
		}
	case ProjectAreaReasoning:
		fmt.Fprintf(&b, "\nReturn only the complete Markdown content for the selected area output as the terminal response. Do not use tools or edit files. UltraPlan owns validation and atomic promotion. Each document must contain exact level-two headings named Project conclusions, Trade-Offs, Evidence, Risks, and Self-critique, plus every specialist section required by its template. The output belongs under `%s/areas/`. Explicit Path and Lines references are resolved and supplied below.\n", baseRel)
	case ProjectFinalReasoning:
		fmt.Fprintf(&b, "\nReturn only the complete Markdown content for `%s/reasoning.md` as the terminal response. Do not use tools or edit files. UltraPlan owns validation and atomic promotion. Resolve or retain contradictions explicitly. Separate accepted constraints from provisional conclusions and route remaining questions to phases or sprints. Include exact level-two headings named Project conclusions, Trade-Offs, Evidence, Risks, and Self-critique.\n", baseRel)
	case ProjectReasoningReview:
		fmt.Fprintf(&b, "\nReturn only the complete Markdown content for `%s/review.md` as the terminal response. Do not use tools or edit files. UltraPlan owns validation and atomic promotion. Adversarially review `%s/reasoning.md` and its area evidence. Check evidence coverage, contradictions, unsupported claims, negative transfer, feasibility, and scope leakage. Verdict semantics are strict: use `pass` when there are zero actionable contract defects; use `pass_with_findings` only when one or more actionable but non-blocking defects remain; use `fail` when a blocking defect remains. Editorial observations, verbosity, and acknowledged future proof obligations are not findings and must not turn a pass into pass_with_findings. End with exactly two machine-readable lines: `Actionable Findings: N` followed by `Verdict: pass`, `Verdict: pass_with_findings`, or `Verdict: fail`. N must equal the number of actionable defects and must be zero for pass and greater than zero for pass_with_findings or fail.\n", baseRel, baseRel)
	default:
		return "", fmt.Errorf("unknown project reasoning stage %q", stage)
	}
	return b.String(), nil
}

func (s Service) ValidateReasoning(ref string) (ProjectReasoningStatus, []ValidationFinding, error) {
	st, err := s.ReasoningStatus(ref)
	if err != nil {
		return st, nil, err
	}
	var f []ValidationFinding
	for _, b := range st.Blockers {
		f = append(f, ValidationFinding{Severity: SeverityError, Section: "Project Reasoning", Problem: "project reasoning incomplete", Cause: b, Suggestion: "Run ultraplan project " + ref + " reasoning flow --to review."})
	}
	return st, f, nil
}

func (s Service) ReasoningFlow(ctx context.Context, ref string, to ProjectReasoningStage) (ProjectReasoningFlowResult, error) {
	order := []ProjectReasoningStage{ProjectReasoningIndex, ProjectAreaReasoning, ProjectFinalReasoning, ProjectReasoningReview}
	valid := false
	for _, x := range order {
		if x == to {
			valid = true
		}
	}
	if !valid {
		return ProjectReasoningFlowResult{}, fmt.Errorf("unknown project reasoning stage %q", to)
	}
	if s.reasoningRuntime == nil {
		return ProjectReasoningFlowResult{}, fmt.Errorf("project reasoning runtime is not configured")
	}
	p, files, err := s.resolveAndRead(ref)
	if err != nil {
		return ProjectReasoningFlowResult{}, err
	}
	idx, _ := ParseProjectIndex(files.IndexContent)
	base := filepath.Join(p.Path, "project-reasoning")
	if err = os.MkdirAll(filepath.Join(base, "areas"), 0o755); err != nil {
		return ProjectReasoningFlowResult{}, err
	}
	statePath := filepath.Join(base, "flow-state.json")
	state, _ := readReasoningState(statePath)
	result := ProjectReasoningFlowResult{Project: p.Name, To: to}
	run := func(stage ProjectReasoningStage, key, output string, inputs []string, prompt string) error {
		promptSum := fmt.Sprintf("%x", sha256.Sum256([]byte(prompt)))
		current := true
		rec, ok := state.Artifacts[key]
		if ok {
			fp, e := digestFile(output)
			promptCurrent := rec.PromptSHA256 == promptSum || (rec.PromptSHA256 == "" && stage != ProjectReasoningReview)
			current = e == nil && fp.SHA256 == rec.OutputSHA256 && promptCurrent
			for _, in := range rec.Inputs {
				path := in.Path
				if !filepath.IsAbs(path) {
					path = filepath.Join(s.root, filepath.FromSlash(path))
				}
				x, e := digestFile(path)
				if e != nil || x.SHA256 != in.SHA256 {
					current = false
				}
			}
		}
		if current && ok {
			result.Skipped = append(result.Skipped, key)
			return nil
		}
		req := s.runtimeConfig
		req.Prompt = prompt
		req.WorkDir = s.root
		req.Metadata = map[string]string{"project": p.Name, "stage": string(stage), "output_path": workspace.Rel(s.root, output)}
		req.Sandbox = "read_only"
		req.Permissions = "restricted"
		req.Policy = pruntime.PermissionPolicy{Default: "deny"}
		previous, previousErr := os.ReadFile(output)
		hadPrevious := previousErr == nil
		rollback := func() {
			if hadPrevious {
				_ = os.WriteFile(output, previous, 0o644)
			} else {
				_ = os.Remove(output)
			}
		}
		rr, e := s.reasoningRuntime.StartRun(ctx, req)
		result.Runtime = append(result.Runtime, rr)
		if e != nil {
			rollback()
			return e
		}
		data, e := os.ReadFile(output)
		data, e = projectReasoningCandidate(data, e, rr.TerminalOutput)
		if e != nil {
			rollback()
			return fmt.Errorf("stage %s did not create %s: %w", stage, workspace.Rel(s.root, output), e)
		}
		if stage == ProjectAreaReasoning || stage == ProjectFinalReasoning {
			if missing := validateReasoningDocument(string(data)); len(missing) > 0 {
				rollback()
				return fmt.Errorf("%s missing required sections: %s; candidate begins %q", workspace.Rel(s.root, output), strings.Join(missing, ", "), boundedReasoningCandidatePreview(string(data)))
			}
		}
		if stage == ProjectReasoningReview {
			if e = validateReviewVerdict(string(data)); e != nil {
				rollback()
				return fmt.Errorf("%s has an invalid machine-readable verdict: %w", workspace.Rel(s.root, output), e)
			}
		}
		if stage == ProjectReasoningIndex {
			manifest, parseFindings := ParseProjectReasoningIndex(string(data))
			validationFindings := ValidateProjectReasoningManifest(s.root, p, idx, manifest)
			if len(parseFindings)+len(validationFindings) > 0 {
				rollback()
				return fmt.Errorf("project-reasoning/index.md failed validation")
			}
		}
		candidate := output + ".candidate"
		if e = os.WriteFile(candidate, data, 0o644); e != nil {
			rollback()
			return e
		}
		if e = os.Rename(candidate, output); e != nil {
			rollback()
			return fmt.Errorf("promote %s: %w", workspace.Rel(s.root, output), e)
		}
		fps := []FingerprintRecord{}
		for _, in := range inputs {
			full := in
			if !filepath.IsAbs(full) {
				full = filepath.Join(s.root, filepath.FromSlash(in))
			}
			fp, e := digestFile(full)
			if e != nil {
				return e
			}
			fp.Path = workspace.Rel(s.root, full)
			fps = append(fps, fp)
		}
		ofp, _ := digestFile(output)
		state.Artifacts[key] = ReasoningArtifactState{Stage: stage, Output: workspace.Rel(s.root, output), Inputs: fps, PromptSHA256: promptSum, OutputSHA256: ofp.SHA256, CompletedAt: time.Now().UTC()}
		result.Completed = append(result.Completed, key)
		return writeReasoningState(statePath, state)
	}
	for _, stage := range order {
		prompt, _ := s.ReasoningPrompt(ref, stage)
		if stage == ProjectReasoningIndex {
			prompt, err = s.appendReasoningInputPacket(prompt, []projectPromptInput{{ID: "project-index", Kind: "project-index", Path: filepath.ToSlash(filepath.Join("projects", p.Name, "project-index.md")), Assignment: "Authoritative project catalog and reasoning policy."}})
			if err != nil {
				return result, err
			}
			if err = run(stage, "index", filepath.Join(base, "index.md"), []string{filepath.Join(p.Path, "project-index.md")}, prompt); err != nil {
				return result, err
			}
		}
		data, e := os.ReadFile(filepath.Join(base, "index.md"))
		if e != nil {
			return result, e
		}
		m, pf := ParseProjectReasoningIndex(string(data))
		vf := ValidateProjectReasoningManifest(s.root, p, idx, m)
		if len(pf)+len(vf) > 0 {
			return result, fmt.Errorf("project-reasoning/index.md failed validation")
		}
		if stage == ProjectAreaReasoning {
			areas := topologicalAreas(m.Areas)
			for _, a := range areas {
				inputs := []string{a.Template, filepath.Join(base, "index.md")}
				direct := []projectPromptInput{{ID: "project-reasoning-index", Kind: "manifest", Path: workspace.Rel(s.root, filepath.Join(base, "index.md")), Assignment: "Selected decision areas, assignments, and dependencies."}, {ID: "template", Kind: "selected-template", Path: a.Template, Assignment: a.Why}}
				for _, x := range m.Evidence {
					if x.Area == a.Name {
						inputs = append(inputs, x.Evidence)
						direct = append(direct, projectPromptInput{ID: "evidence-" + a.Name, Kind: "assigned-evidence", Path: x.Evidence, Assignment: "Relevant questions: " + x.RelevantQuestions + ". Why assigned: " + x.Why})
					}
				}
				for _, x := range m.Sources {
					if x.Area == a.Name {
						inputs = append(inputs, x.Source)
						direct = append(direct, projectPromptInput{ID: "source-" + a.Name, Kind: "assigned-source", Path: x.Source, Assignment: "Authority: " + x.Authority + ". Why assigned: " + x.Why})
					}
				}
				for _, d := range a.DependsOn {
					for _, da := range m.Areas {
						if da.Name == d {
							inputs = append(inputs, da.Output)
							direct = append(direct, projectPromptInput{ID: "dependency-" + d, Kind: "dependency-output", Path: da.Output, Assignment: "Declared dependency for " + a.Name + "."})
						}
					}
				}
				areaPrompt := prompt + "\nArea: " + a.Name + "\nTemplate: " + a.Template + "\nOutput: " + a.Output + "\n"
				areaPrompt, e = s.appendReasoningInputPacket(areaPrompt, direct)
				if e != nil {
					return result, e
				}
				if err = run(stage, "area:"+a.Name, filepath.Join(s.root, filepath.FromSlash(a.Output)), inputs, areaPrompt); err != nil {
					return result, err
				}
			}
		}
		if stage == ProjectFinalReasoning {
			inputs := []string{filepath.Join(base, "index.md")}
			direct := []projectPromptInput{{ID: "project-reasoning-index", Kind: "manifest", Path: workspace.Rel(s.root, filepath.Join(base, "index.md")), Assignment: "Accepted project reasoning selection and dependency graph."}}
			for _, a := range m.Areas {
				if a.Required {
					inputs = append(inputs, a.Output)
					direct = append(direct, projectPromptInput{ID: "area-" + a.Name, Kind: "required-area-output", Path: a.Output, Assignment: a.Why})
				}
			}
			prompt, err = s.appendReasoningInputPacket(prompt, direct)
			if err != nil {
				return result, err
			}
			if err = run(stage, "reasoning", filepath.Join(base, "reasoning.md"), inputs, prompt); err != nil {
				return result, err
			}
		}
		if stage == ProjectReasoningReview {
			inputs := []string{filepath.Join(base, "index.md"), filepath.Join(base, "reasoning.md")}
			for _, a := range m.Areas {
				inputs = append(inputs, a.Output)
			}
			direct := []projectPromptInput{{ID: "project-reasoning-index", Kind: "manifest", Path: workspace.Rel(s.root, filepath.Join(base, "index.md")), Assignment: "Review coverage and assignments."}, {ID: "project-reasoning", Kind: "project-synthesis", Path: workspace.Rel(s.root, filepath.Join(base, "reasoning.md")), Assignment: "Candidate accepted project contract."}}
			for _, a := range m.Areas {
				direct = append(direct, projectPromptInput{ID: "area-" + a.Name, Kind: "area-output", Path: a.Output, Assignment: a.Why})
			}
			prompt, err = s.appendReasoningInputPacket(prompt, direct)
			if err != nil {
				return result, err
			}
			out := filepath.Join(base, "review.md")
			if err = run(stage, "review", out, inputs, prompt); err != nil {
				return result, err
			}
			review, _ := os.ReadFile(out)
			state, _ = readReasoningState(statePath)
			state.Verdict = parseVerdict(string(review))
			if state.Verdict == "" {
				return result, fmt.Errorf("review.md has no machine-readable verdict")
			}
			if err = writeReasoningState(statePath, state); err != nil {
				return result, err
			}
		}
		if stage == to {
			break
		}
	}
	result.Status, _ = s.ReasoningStatus(ref)
	return result, nil
}

func boundedReasoningCandidatePreview(content string) string {
	content = strings.Join(strings.Fields(content), " ")
	const limit = 320
	if len(content) > limit {
		return content[:limit] + "..."
	}
	return content
}

func projectReasoningResultContent(output string) string {
	content := strings.TrimSpace(output)
	if strings.HasPrefix(content, "```markdown\n") && strings.HasSuffix(content, "```") {
		content = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(content, "```markdown\n"), "```"))
	} else if strings.HasPrefix(content, "```md\n") && strings.HasSuffix(content, "```") {
		content = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(content, "```md\n"), "```"))
	}
	return content + "\n"
}

func projectReasoningCandidate(existing []byte, readErr error, terminal string) ([]byte, error) {
	if strings.TrimSpace(terminal) != "" {
		return []byte(projectReasoningResultContent(terminal)), nil
	}
	return existing, readErr
}

func topologicalAreas(in []ReasoningArea) []ReasoningArea {
	by := map[string]ReasoningArea{}
	for _, a := range in {
		by[a.Name] = a
	}
	var out []ReasoningArea
	done := map[string]bool{}
	var add func(string)
	add = func(n string) {
		if done[n] {
			return
		}
		for _, d := range by[n].DependsOn {
			add(d)
		}
		done[n] = true
		out = append(out, by[n])
	}
	names := make([]string, 0, len(by))
	for n := range by {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		add(n)
	}
	return out
}
func parseVerdict(s string) string {
	for _, v := range []string{"pass_with_findings", "pass", "fail"} {
		if strings.Contains(strings.ToLower(s), "verdict: "+v) {
			return v
		}
	}
	return ""
}

func validateReviewVerdict(s string) error {
	verdict := parseVerdict(s)
	if verdict == "" {
		return fmt.Errorf("verdict is missing")
	}
	count := -1
	for _, line := range strings.Split(s, "\n") {
		if _, err := fmt.Sscanf(strings.TrimSpace(line), "Actionable Findings: %d", &count); err == nil {
			break
		}
	}
	if count < 0 {
		return fmt.Errorf("Actionable Findings count is missing")
	}
	if verdict == "pass" && count != 0 {
		return fmt.Errorf("pass requires zero actionable findings, got %d", count)
	}
	if verdict != "pass" && count == 0 {
		return fmt.Errorf("%s requires at least one actionable finding", verdict)
	}
	return nil
}
