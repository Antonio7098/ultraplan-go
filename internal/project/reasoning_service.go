package project

import (
	"context"
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

func (s Service) WithRuntime(rt ReasoningRuntime) Service { s.reasoningRuntime = rt; return s }

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
	check := func(key, out string, inputs []string) {
		rel := filepath.ToSlash(filepath.Join("projects", p.Name, "project-reasoning", out))
		st.Outputs = append(st.Outputs, rel)
		rec, ok := state.Artifacts[key]
		if !ok {
			st.Fresh = false
			st.Blockers = append(st.Blockers, rel+" has no completed state")
			return
		}
		full := filepath.Join(base, out)
		fp, e := digestFile(full)
		if e != nil || fp.SHA256 != rec.OutputSHA256 {
			st.Fresh = false
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
				st.Fresh = false
				st.Blockers = append(st.Blockers, rel+" is stale because "+in.Path+" changed")
			}
		}
	}
	for _, a := range m.Areas {
		check("area:"+a.Name, strings.TrimPrefix(normalizeCatalogPath(a.Output), filepath.ToSlash(filepath.Join("projects", p.Name, "project-reasoning"))+"/"), nil)
	}
	check("reasoning", "reasoning.md", nil)
	check("review", "review.md", nil)
	st.Verdict = state.Verdict
	acceptedVerdict := st.Verdict == st.RequiredVerdict || (st.Verdict == "pass_with_findings" && st.RequiredVerdict == "pass_with_findings")
	st.Accepted = st.Fresh && acceptedVerdict
	if !acceptedVerdict {
		st.Blockers = append(st.Blockers, filepath.ToSlash(filepath.Join("projects", p.Name, "project-reasoning/review.md"))+" has no accepted current verdict.")
		st.CurrentStage = ProjectReasoningReview
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
		fmt.Fprintf(&b, "\nCreate `%s/index.md` with Reasoning Areas, Evidence Assignments, Source Document Assignments, and Excluded Evidence tables. Select templates only from Available Project Reasoning Templates. Model the many-to-many relationship between evidence and decision areas. Outputs must stay under `%s/areas/`. Reject duplicate outputs and dependency cycles.\n\nCatalog:\n", baseRel, baseRel)
		for _, e := range idx.Entries {
			if e.Section == SectionProjectReasoningTemplates || e.Section == SectionAvailableEvidenceReports || e.Section == SectionSourceDocuments || e.Section == SectionActiveContractPool {
				fmt.Fprintf(&b, "- %s | %s | %s\n", e.Section, e.Name, e.Path)
			}
		}
	case ProjectAreaReasoning:
		fmt.Fprintf(&b, "\nComplete the selected project reasoning area outputs in dependency order. Each document must include Project conclusions, Trade-Offs, Evidence, Risks, and Self-critique, plus every specialist section required by its template. Write only under `%s/areas/`. Explicit Path and Lines references are resolved and supplied below.\n", baseRel)
	case ProjectFinalReasoning:
		fmt.Fprintf(&b, "\nSynthesize all required area documents into `%s/reasoning.md`. Resolve or retain contradictions explicitly. Separate accepted constraints from provisional conclusions and route remaining questions to phases or sprints. Include Project conclusions, Trade-Offs, Evidence, Risks, and Self-critique.\n", baseRel)
	case ProjectReasoningReview:
		fmt.Fprintf(&b, "\nAdversarially review `%s/reasoning.md` and its area evidence. Check evidence coverage, contradictions, unsupported claims, negative transfer, feasibility, and scope leakage. Write `%s/review.md`. End with exactly `Verdict: pass`, `Verdict: pass_with_findings`, or `Verdict: fail`.\n", baseRel, baseRel)
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
		current := true
		rec, ok := state.Artifacts[key]
		if ok {
			fp, e := digestFile(output)
			current = e == nil && fp.SHA256 == rec.OutputSHA256
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
		req := pruntime.Request{Prompt: prompt, WorkDir: s.root, Metadata: map[string]string{"project": p.Name, "stage": string(stage), "output_path": workspace.Rel(s.root, output)}}
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
		if e != nil {
			rollback()
			return fmt.Errorf("stage %s did not create %s: %w", stage, workspace.Rel(s.root, output), e)
		}
		if stage == ProjectAreaReasoning || stage == ProjectFinalReasoning {
			if missing := validateReasoningDocument(string(data)); len(missing) > 0 {
				rollback()
				return fmt.Errorf("%s missing required sections: %s", workspace.Rel(s.root, output), strings.Join(missing, ", "))
			}
		}
		if stage == ProjectReasoningReview && parseVerdict(string(data)) == "" {
			rollback()
			return fmt.Errorf("%s has no machine-readable verdict", workspace.Rel(s.root, output))
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
		state.Artifacts[key] = ReasoningArtifactState{Stage: stage, Output: workspace.Rel(s.root, output), Inputs: fps, OutputSHA256: ofp.SHA256, CompletedAt: time.Now().UTC()}
		result.Completed = append(result.Completed, key)
		return writeReasoningState(statePath, state)
	}
	for _, stage := range order {
		prompt, _ := s.ReasoningPrompt(ref, stage)
		if stage == ProjectReasoningIndex {
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
				for _, x := range m.Evidence {
					if x.Area == a.Name {
						inputs = append(inputs, x.Evidence)
					}
				}
				for _, x := range m.Sources {
					if x.Area == a.Name {
						inputs = append(inputs, x.Source)
					}
				}
				for _, d := range a.DependsOn {
					for _, da := range m.Areas {
						if da.Name == d {
							inputs = append(inputs, da.Output)
						}
					}
				}
				templateData, _ := os.ReadFile(filepath.Join(s.root, filepath.FromSlash(a.Template)))
				resolved, _, e := ResolveReasoningReferences(s.root, string(templateData))
				if e != nil {
					return result, e
				}
				areaPrompt := prompt + "\nArea: " + a.Name + "\nTemplate: " + a.Template + "\nOutput: " + a.Output + "\n" + resolved
				if err = run(stage, "area:"+a.Name, filepath.Join(s.root, filepath.FromSlash(a.Output)), inputs, areaPrompt); err != nil {
					return result, err
				}
			}
		}
		if stage == ProjectFinalReasoning {
			inputs := []string{filepath.Join(base, "index.md")}
			var resolved strings.Builder
			for _, a := range m.Areas {
				if a.Required {
					inputs = append(inputs, a.Output)
					if data, readErr := os.ReadFile(filepath.Join(s.root, filepath.FromSlash(a.Output))); readErr == nil {
						packet, _, resolveErr := ResolveReasoningReferences(s.root, string(data))
						if resolveErr != nil {
							return result, resolveErr
						}
						resolved.WriteString(packet)
					}
				}
			}
			if err = run(stage, "reasoning", filepath.Join(base, "reasoning.md"), inputs, prompt+resolved.String()); err != nil {
				return result, err
			}
		}
		if stage == ProjectReasoningReview {
			inputs := []string{filepath.Join(base, "index.md"), filepath.Join(base, "reasoning.md")}
			for _, a := range m.Areas {
				inputs = append(inputs, a.Output)
			}
			if data, readErr := os.ReadFile(filepath.Join(base, "reasoning.md")); readErr == nil {
				packet, _, resolveErr := ResolveReasoningReferences(s.root, string(data))
				if resolveErr != nil {
					return result, resolveErr
				}
				prompt += packet
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
