package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const stageSkillsRoot = ".agents/skills"

type StageSkill struct {
	Stage            string
	Name             string
	DisplayName      string
	ShortDescription string
	Prerequisites    []string
	Prompt           string
	PromptAvailable  bool
	StageWorkflow    string
}

type SkillsOptions struct {
	Force bool
}

type SkillsPlan struct {
	Root       string      `json:"root"`
	Selection  string      `json:"selection"`
	Operations []Operation `json:"operations"`
}

func StageSkills() []StageSkill {
	return []StageSkill{
		{
			Stage:            "requirements",
			Name:             "ultraplan-requirements",
			DisplayName:      "UltraPlan Requirements",
			ShortDescription: "Create governed sprint requirements",
			Prerequisites:    []string{"project index", "roadmap and relevant project docs"},
			Prompt:           defaultCreateRequirementsPrompt,
			PromptAvailable:  true,
			StageWorkflow: `Create or revise the exact requirements artifact from the resolved prompt.
If prior sprint reviews exist, carry forward only still-applicable decisions. Do not silently broaden the roadmap scope.`,
		},
		{
			Stage:            "sprint-index",
			Name:             "ultraplan-sprint-index",
			DisplayName:      "UltraPlan Sprint Index",
			ShortDescription: "Select sprint context and evidence",
			Prerequisites:    []string{"requirements"},
			Prompt:           defaultCreateSprintIndexPrompt,
			PromptAvailable:  true,
			StageWorkflow: `Create or revise the sprint index from the resolved prompt.
Keep it a selection document: update selected contracts, evidence, reasoning templates, carry-forward decisions, and exclusions without making implementation decisions.`,
		},
		{
			Stage:            "technical-handbook",
			Name:             "ultraplan-technical-handbook",
			DisplayName:      "UltraPlan Technical Handbook",
			ShortDescription: "Distil selected technical evidence",
			Prerequisites:    []string{"requirements", "sprint-index"},
			Prompt:           defaultCreateTechnicalHandbookPrompt,
			PromptAvailable:  true,
			StageWorkflow: `Create or revise the technical handbook from only the evidence selected by the sprint index.
Distil patterns, trade-offs, cautions, and open questions. Preserve the boundary between evidence and final design decisions.`,
		},
		{
			Stage:            "area-reasoning",
			Name:             "ultraplan-area-reasoning",
			DisplayName:      "UltraPlan Area Reasoning",
			ShortDescription: "Reason deeply about selected areas",
			Prerequisites:    []string{"requirements", "sprint-index", "technical-handbook"},
			Prompt:           defaultCreateAreaReasoningPrompt,
			PromptAvailable:  true,
			StageWorkflow: `Create or revise every area-reasoning document selected by the sprint index, and no others.
When the user requests a deep dive, treat the work as an interactive design discussion: surface design pressures, alternatives, trade-offs, risks, and evidence; resolve one meaningful decision at a time; then record the conclusions unless the user asked for discussion or a proposal only.`,
		},
		{
			Stage:            "reasoning",
			Name:             "ultraplan-reasoning",
			DisplayName:      "UltraPlan Sprint Reasoning",
			ShortDescription: "Resolve sprint design decisions",
			Prerequisites:    []string{"requirements", "sprint-index", "technical-handbook", "selected area-reasoning documents or an explicit none selection"},
			Prompt:           defaultCreateSprintReasoningPrompt,
			PromptAvailable:  true,
			StageWorkflow: `Create or revise the final sprint reasoning document from the resolved prompt.
When the user requests a deep dive, discuss design pressures, competing approaches, accepted trade-offs, technical debt, future consequences, risks, and evidence before committing the conclusions to the artifact. Do not collapse a requested discussion into a shallow one-shot answer.`,
		},
		{
			Stage:            "plan",
			Name:             "ultraplan-plan",
			DisplayName:      "UltraPlan Sprint Plan",
			ShortDescription: "Create an executable sprint plan",
			Prerequisites:    []string{"requirements", "sprint-index", "technical-handbook", "reasoning"},
			Prompt:           defaultPlanSprintPrompt,
			PromptAvailable:  true,
			StageWorkflow: `Create or revise plan.md from the resolved prompt.
Carry decisions forward rather than reopening them. Make tasks ordered, bounded, testable, traceable to requirements, and explicit about files, checks, evidence, dependencies, and stop conditions. Do not implement the plan in this stage.`,
		},
		{
			Stage:            "execute",
			Name:             "ultraplan-execute",
			DisplayName:      "UltraPlan Execute",
			ShortDescription: "Execute an approved sprint plan",
			Prerequisites:    []string{"validated plan and all planning artifacts", "resolvable target implementation directory"},
			Prompt:           defaultExecuteSprintPrompt,
			PromptAvailable:  true,
			StageWorkflow: `Run a dry-run first:

    ultraplan sprint <project> <sprint> execute --dry-run

Then execute or resume the governed plan:

    ultraplan sprint <project> <sprint> execute --resume

Follow progress, inspect partial failures, and continue until the execution state is complete or a genuine blocker requires the user. Do not replace this governed execution with untracked ad-hoc implementation.`,
		},
		{
			Stage:            "review",
			Name:             "ultraplan-review",
			DisplayName:      "UltraPlan Review",
			ShortDescription: "Run the governed sprint review",
			Prerequisites:    []string{"completed execute stage", "current planning artifacts and target implementation"},
			Prompt:           defaultReviewPrompt,
			PromptAvailable:  true,
			StageWorkflow: `For example, the input ` + "`projects/ultraplan-go/sprints/30-web-foundations/`" + ` resolves to project ` + "`ultraplan-go`" + `, sprint ` + "`30-web-foundations`" + `, and the target implementation directory declared in ` + "`projects/ultraplan-go/project-index.md`" + `. It does not resolve to the workspace repository or to a nested source checkout.

Preview the review scope first:

    ultraplan sprint <project> <sprint> review --dry-run

Then run or resume the governed review:

    ultraplan sprint <project> <sprint> review

Plain reruns resume only validated coverage and retained reviewer sessions. Use ` + "`--restart`" + ` only when the user explicitly wants to discard compatible checkpoints or when the CLI reports that the saved schema, model, or input fingerprint is incompatible. Do not use restart as a generic retry for runtime, schema, or caller-interruption failures.

The CLI owns reviewer fan-out, frozen inputs, aggregation, verdict calculation, state reconciliation, and creation of the sprint-root ` + "`review.md`" + `. Do not replace it with a single-agent ad hoc code review. After it finishes, read the generated ` + "`review.md`" + ` and the fresh review status, then summarize the verdict, findings by severity, evidence freshness, blockers, and smoke eligibility for the user.

If publication is blocked because inputs changed, report the exact changed logical paths emitted by the CLI, restore a stable input set, and rerun normally. Do not silently fix findings during the review stage; if fixes are requested, return to execute and rerun review so evidence and fingerprints remain authoritative.`,
		},
		{
			Stage:            "smoke",
			Name:             "ultraplan-smoke",
			DisplayName:      "UltraPlan Smoke",
			ShortDescription: "Run review-gated smoke verification",
			Prerequisites:    []string{"fresh completed review", "discoverable protocol-v1 smoke harness"},
			Prompt: `# Sprint Smoke Verification

Use UltraPlan's deterministic, review-gated smoke orchestration. The smoke harness, allowed mutation roots, timeouts, evidence capture, result validation, and flow-state reconciliation belong to UltraPlan; do not reproduce them with ad-hoc shell commands.`,
			StageWorkflow: `Inspect the bounded smoke plan first:

    ultraplan sprint <project> <sprint> smoke --dry-run

If the dry-run is valid, run the manually requested smoke stage:

    ultraplan sprint <project> <sprint> smoke --yes

Report the authoritative verdict, evidence paths, issues, and next action. A failed or blocked result is not a pass. Do not bypass a stale/missing review gate unless the user explicitly requests and confirms a supported diagnostic override.`,
		},
	}
}

func ResolveStageSkills(selection string) ([]StageSkill, error) {
	selection = strings.TrimSpace(strings.ToLower(selection))
	if selection == "" || selection == "all" {
		return StageSkills(), nil
	}
	selection = strings.TrimPrefix(selection, "ultraplan-")
	for _, skill := range StageSkills() {
		if skill.Stage == selection {
			return []StageSkill{skill}, nil
		}
	}
	var stages []string
	for _, skill := range StageSkills() {
		stages = append(stages, skill.Stage)
	}
	return nil, fmt.Errorf("unknown stage skill %q; expected all or one of: %s", selection, strings.Join(stages, ", "))
}

func PlanSkills(path, selection string, opts SkillsOptions) (SkillsPlan, error) {
	root, err := normalize(path)
	if err != nil {
		return SkillsPlan{}, err
	}
	skills, err := ResolveStageSkills(selection)
	if err != nil {
		return SkillsPlan{}, err
	}
	plan := SkillsPlan{Root: root, Selection: selection}
	files := stageSkillFiles(skills)

	dirSet := map[string]bool{".agents": true, stageSkillsRoot: true}
	for _, skill := range skills {
		dirSet[filepath.ToSlash(filepath.Join(stageSkillsRoot, skill.Name))] = true
		dirSet[filepath.ToSlash(filepath.Join(stageSkillsRoot, skill.Name, "agents"))] = true
	}
	dirs := make([]string, 0, len(dirSet))
	for dir := range dirSet {
		dirs = append(dirs, dir)
	}
	sort.Strings(dirs)
	for _, dir := range dirs {
		full, err := ResolveInside(root, dir)
		if err != nil {
			return SkillsPlan{}, err
		}
		if _, statErr := os.Stat(full); os.IsNotExist(statErr) {
			plan.Operations = append(plan.Operations, Operation{Action: "create", Path: dir, Type: "dir"})
		} else if statErr != nil {
			return SkillsPlan{}, fmt.Errorf("inspect skill directory %s: %w", dir, statErr)
		}
	}

	paths := make([]string, 0, len(files))
	for rel := range files {
		paths = append(paths, rel)
	}
	sort.Strings(paths)
	for _, rel := range paths {
		full, err := ResolveInside(root, rel)
		if err != nil {
			return SkillsPlan{}, err
		}
		current, readErr := os.ReadFile(full)
		switch {
		case os.IsNotExist(readErr):
			plan.Operations = append(plan.Operations, Operation{Action: "create", Path: rel, Type: "file"})
		case readErr != nil:
			return SkillsPlan{}, fmt.Errorf("read existing stage skill %s: %w", rel, readErr)
		case string(current) == files[rel]:
			continue
		case opts.Force:
			plan.Operations = append(plan.Operations, Operation{Action: "overwrite", Path: rel, Type: "file"})
		default:
			plan.Operations = append(plan.Operations, Operation{Action: "skip", Path: rel, Type: "file"})
		}
	}
	return plan, nil
}

func MaterialiseSkills(path, selection string, opts SkillsOptions) (SkillsPlan, error) {
	plan, err := PlanSkills(path, selection, opts)
	if err != nil {
		return SkillsPlan{}, err
	}
	skills, err := ResolveStageSkills(selection)
	if err != nil {
		return SkillsPlan{}, err
	}
	files := stageSkillFiles(skills)
	for _, op := range plan.Operations {
		if op.Action == "skip" {
			continue
		}
		full, err := ResolveInside(plan.Root, op.Path)
		if err != nil {
			return SkillsPlan{}, err
		}
		switch op.Type {
		case "dir":
			if err := os.MkdirAll(full, 0o755); err != nil {
				return SkillsPlan{}, fmt.Errorf("create skill directory %s: %w", op.Path, err)
			}
		case "file":
			if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
				return SkillsPlan{}, fmt.Errorf("create parent for %s: %w", op.Path, err)
			}
			if err := os.WriteFile(full, []byte(files[op.Path]), 0o644); err != nil {
				return SkillsPlan{}, fmt.Errorf("%s skill file %s: %w", op.Action, op.Path, err)
			}
		}
	}
	return plan, nil
}

func stageSkillFiles(skills []StageSkill) map[string]string {
	files := make(map[string]string, len(skills)*2)
	for _, skill := range skills {
		base := filepath.ToSlash(filepath.Join(stageSkillsRoot, skill.Name))
		files[base+"/SKILL.md"] = renderStageSkill(skill)
		files[base+"/agents/openai.yaml"] = renderStageSkillMetadata(skill)
	}
	return files
}

func renderStageSkill(skill StageSkill) string {
	prerequisites := make([]string, 0, len(skill.Prerequisites))
	for _, prerequisite := range skill.Prerequisites {
		prerequisites = append(prerequisites, "- "+prerequisite)
	}
	promptStep := "Use the embedded canonical prompt below together with the current CLI status and governed command preview."
	if skill.PromptAvailable {
		promptStep = fmt.Sprintf(`Use the current effective prompt and concrete paths:

    ultraplan sprint <project> <sprint> prompt %s

   The resolved prompt can include workspace or project overrides and therefore takes precedence over the canonical prompt below.`, skill.Stage)
	}
	return fmt.Sprintf(`---
name: %s
description: Manually run the UltraPlan %s stage when given a project sprint path or project/sprint references. Use only when the user explicitly invokes $%s or directly asks to run this exact UltraPlan stage; do not invoke implicitly.
---

# %s

Run this stage interactively while preserving UltraPlan's governed artifact chain.

## Operating contract

1. Treat a supplied sprint path as UltraPlan stage input, not as a Git target. For an input such as `+"`projects/<project>/sprints/<sprint>/`"+` or `+"`.ultra/projects/<project>/sprints/<sprint>/`"+`, find the workspace root, derive `+"`<project>`"+` and `+"`<sprint>`"+` from the path, and read the matching `+"`project-index.md`"+`. The sprint directory contains governed stage artifacts; when implementation access is required, resolve its repository from `+"`Target Implementation Directory`"+`, falling back to `+"`Repository`"+` only when the target field is absent. Resolve relative repository paths against the workspace root and verify the result before using it. Do not search nested source repositories for a similarly named skill, and do not ask what target to use merely because the supplied input is a directory.
2. If no sprint path was supplied, locate the workspace root and resolve the project and sprint from explicit references and the current location. Ask only when the project index is missing, a required implementation target cannot be resolved, or more than one project/sprint remains possible.
3. Run all UltraPlan commands from the resolved workspace root. Run `+"`ultraplan project <project> status`"+` and `+"`ultraplan sprint <project> <sprint> status --json`"+`. Treat files, the project index, and fresh CLI status as authoritative; never hand-edit flow-state JSON.
4. Check these prerequisites:

%s

5. Validate every prerequisite that has a sprint validation command. If anything is missing, invalid, stale, or internally inconsistent, show the exact gaps and ask whether to fill them. Do not fill prerequisite gaps until the user agrees. If they agree, run the corresponding earlier UltraPlan skills in canonical order, then return to this stage.
6. If the target is already complete and valid, summarize that state and ask before regenerating or materially changing it.
7. If the user explicitly asks for a proposal, analysis, or discussion only, inspect all relevant evidence and return that without writing artifacts or advancing state. Otherwise, do the stage now; do not stop at a proposal.
8. %s
9. Complete the stage-specific workflow, preserve unrelated user edits, and do not cross declared mutation boundaries.
10. Run `+"`ultraplan sprint <project> <sprint> validate %s`"+` when supported. Fix validation findings within this stage rather than declaring success early.
11. Run `+"`ultraplan sprint <project> <sprint> status --json`"+` after writes or governed execution so flow-state, freshness, and artifact status are reconciled. Re-run project status if the project or sprint index changed.
12. Inspect downstream artifacts for references made stale by this change. Update directly coupled indexes/references when safe; otherwise report the exact dependent stage that must be revisited. Never delete or silently rewrite downstream decisions.
13. Finish with the artifact/result paths, validation outcome, state transition, and any remaining blocker or dependent stage.

## Stage workflow

%s

## Canonical stage prompt

The current resolved CLI prompt wins over this embedded baseline.

%s
`, skill.Name, skill.Stage, skill.Name, skill.DisplayName, strings.Join(prerequisites, "\n"), promptStep, skill.Stage, skill.StageWorkflow, strings.TrimSpace(skill.Prompt))
}

func renderStageSkillMetadata(skill StageSkill) string {
	return fmt.Sprintf(`interface:
  display_name: %q
  short_description: %q
  default_prompt: %q
policy:
  allow_implicit_invocation: false
`, skill.DisplayName, skill.ShortDescription, "Use $"+skill.Name+" to run the "+skill.Stage+" stage for this UltraPlan sprint.")
}
