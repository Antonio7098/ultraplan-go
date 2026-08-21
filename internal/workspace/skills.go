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
	Stage             string
	Name              string
	DisplayName       string
	ShortDescription  string
	Prerequisites     []string
	Prompt            string
	PromptAvailable   bool
	StageWorkflow     string
	SkipValidation    bool
	ManualStateRepair bool
	StatusPromptOnly  bool
	CanonicalFlow     bool
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
			Stage:            "reconcile",
			Name:             "ultraplan-reconcile-review-smoke",
			DisplayName:      "UltraPlan Review And Smoke Reconciliation",
			ShortDescription: "Triage findings, fix genuine issues, and ready the next gate",
			Prerequisites:    []string{"an existing review.md or smoke.md result", "the sprint planning artifacts", "a resolvable target implementation directory"},
			Prompt: `# Review And Smoke Reconciliation

Assess a generated UltraPlan review or smoke result against its governed sprint scope and the actual implementation. Fix genuine in-scope defects, preserve unrelated work, reconcile durable evidence truthfully, and prove that the next verification stage is ready or report its exact blocker.`,
			PromptAvailable:   false,
			SkipValidation:    true,
			ManualStateRepair: true,
			StageWorkflow: `Use this workflow after a review or smoke result needs human assessment and remediation. It is not a substitute for initially running the governed review or smoke process.

1. Establish the frozen and current evidence:
   - Read the complete review.md or smoke.md, flow-state.json, .run-state.json, requirements.md, sprint-index.md, reasoning.md, plan.md, and execute.md.
   - Resolve the implementation target from project-index.md and inspect its current status and diff. Preserve unrelated user changes.
   - Run a review dry-run when reviewing review evidence to obtain the current governed input fingerprint. Record whether the existing result is current or stale.
2. Classify every reported finding using implementation evidence:
   - genuine sprint defect: violates an acceptance criterion, explicit plan decision, mutation/security boundary, or claimed verification property;
   - genuine platform follow-up: real but pre-existing or broader than the sprint, so it must not block this sprint unless the selected contract explicitly made it a release gate;
   - superseded/already fixed;
   - unsupported or scope-expanding: speculative, contradicted by the plan, or explicitly deferred/non-goal.
   Cite concrete files and tests. Do not accept severity labels without reproducing the behavior or tracing the relevant code path.
3. If the user requested fixes, implement only genuine authorized defects in the target repository. Add focused regression tests, then run verification proportional to risk, including the sprint's required full test/race/vet/build/diff gates when closure is claimed.
4. Re-run the governed review normally when practical so UltraPlan owns aggregation, fingerprints, artifact publication, and state. If the generated verdict is still demonstrably wrong and the user explicitly authorizes manual reconciliation:
   - retain the automated result as superseded history;
   - add a concise manual findings and reconciliation section to review.md;
   - use only supported verdict values and the current dry-run fingerprint;
   - recompute the review artifact SHA-256;
   - update review status, verdict, fingerprint, artifact digest, last-complete identity, timestamps, and diagnostic provenance coherently and atomically;
   - never invent reviewer coverage, test evidence, or a passing command result;
   - run sprint status immediately and require it to report the review as completed, fresh, and digest-consistent.
5. Reconcile obsolete next-stage attempts only when their recorded blocker has actually disappeared. Preserve historical successful evidence. A prior failed smoke attempt caused solely by a now-resolved review gate may be cleared from current smoke state; do not erase a real harness, test, cleanup, or mutation failure.
6. Discover and inspect the smoke harness before claiming readiness:
   - Start with sprint smoke --dry-run --json from the workspace root. Read ready, verdict, review_verdict, review_fingerprint, scope, prerequisites, diagnostics, and next_action.
   - Resolve the harness through the project smoke configuration and manifest. The normal sibling checkout is derived from the manifest/target relationship and is often ../ultraplan-go-smoke; do not assume that path without checking discovery output and ultraplan-smoke.json.
   - Inspect the protocol-v1 manifest and discovery implementation, especially executable/cwd, commands, evidence roots, prerequisites, suites, tests, sprintMappings, requiredCoverage, complete, and notApplicable.
   - A complete mapping must reference non-empty tests whose declared coverage satisfies requiredCoverage. Do not create an empty complete mapping to bypass smoke.
   - Use notApplicable only when the sprint plan or requirements explicitly defer or exclude external/live-runtime smoke. It must be represented as a non-contradictory mapping (notApplicable true and complete false), with a truthful rationale naming the owning later sprint or gate.
   - If a harness change is needed, edit only declared authoring paths, run the harness build/tests, then repeat the UltraPlan smoke dry-run.
7. The next stage is ready only when the dry-run reports ready true, the current review verdict/fingerprint match durable state, and the selection is either runnable with satisfied prerequisites or truthfully not_applicable. A dry-run that is blocked, stale, diagnostic-only unexpectedly, or missing coverage is not ready even if review passed.
8. When current smoke execution is passing and the user requested publication or reconciliation, update the sprint-root smoke.md and smoke flow state as one coherent result:
   - Preserve the prior generated summary as superseded history when its findings or verdict are being replaced; never erase real failed-run, cleanup, mutation-safety, or open-issue evidence.
   - Populate every required smoke.md section from the actual selected scope, sanitized invocation, prerequisites, run counts, external evidence identities and hashes, findings/issues, cleanup and mutation checks, supported verdict, and one explicit next action. Keep raw streams and secrets in the external evidence roots.
   - Use pass only when all selected required tests passed, evidence identity is complete, cleanup is certain, no prohibited mutation occurred, and the current non-overridden review gate permits promotion. Use pass_with_open_issues only for truthful non-blocking open issues; do not upgrade a diagnostic override, failed, blocked, or incomplete run.
   - Recompute the smoke.md SHA-256 and the input fingerprint from the exact durable evidence identities. Reconcile status completed, verdict, artifact path and digest, smoke fingerprint, input fingerprint, current review fingerprint, run/author/evidence identities, timestamps, issues, override facts, diagnostics, active-attempt state, and last-complete identity coherently and atomically. Never invent missing run or evidence fields.
   - Run validate smoke and sprint status --json immediately. Require smoke to be completed, fresh, digest-consistent, evidence-fingerprint-consistent, and tied to the current review; require the overall sprint flow to expose the resulting terminal assessment. If supported reconciliation cannot establish all of those facts, leave the truthful prior state intact, report reconciliation as the blocker, and do not claim that smoke or the sprint is passing.
9. Finish by reporting: reconciled review.md, smoke.md, flow-state.json, and external evidence paths; current review fingerprint, smoke input fingerprint, and artifact digest; fixes and verification performed; smoke harness path and mapping decision; final smoke verdict and freshness; overall sprint-flow assessment; and the exact remaining next action.`,
		},
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
			Stage:            "code-context",
			Name:             "ultraplan-code-context",
			DisplayName:      "UltraPlan Code Context",
			ShortDescription: "Generate grounded implementation context",
			Prerequisites:    []string{"validated requirements", "resolvable target implementation directory"},
			Prompt: `# Sprint Code Context

Run the canonical code-context flow so UltraPlan resolves the approved implementation repository, applies the read-only runtime policy, validates the reference-only artifact, and promotes it atomically.`,
			CanonicalFlow: true,
			StageWorkflow: `This manual-only skill is the narrow canonical-delegation exception. Resolve the selected project and sprint from the governed input, assign them without adding shell metacharacters, and invoke exactly:

    PROJECT="<project>"
    SPRINT="<sprint>"
    ultraplan sprint "$PROJECT" "$SPRINT" flow --to code-context

Do not inspect the implementation repository, select sources, compose the stage prompt, validate candidate output, promote the artifact, or reproduce state transitions in this skill. The canonical flow owns those mechanics. Dry-run materialisation of this skill remains non-mutating; normal materialisation preserves custom files and --force restores this built-in definition.`,
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
			StatusPromptOnly: true,
			StageWorkflow: `Use the resolved execution prompt and approved plan to perform the implementation yourself in the target implementation directory.

Act as the execution agent: work through incomplete plan tasks in order, inspect the code, edit the implementation, run the required checks directly, and maintain plan checkboxes, execution evidence, and the execution artifacts required by the prompt. Continue until the plan is complete or a genuine blocker requires the user.

For this stage, use the UltraPlan CLI only to check project or sprint state, materialise the effective execution prompt, and run the review readiness check described below. Do not use CLI execution dry runs, validation, execute, verify, smoke, or flow commands to perform, preview, validate, or complete the sprint. Inspect and verify the implementation and governed artifacts yourself, then use sprint status to reconcile the resulting state.

After the implementation and execution evidence are complete, run ` + "`ultraplan sprint <project> <sprint> review --dry-run --json`" + `. Require the top-level status and ` + "`result.execution_status`" + ` to be ` + "`ready`" + ` with no blocking diagnostics. If review is not ready, inspect its diagnostics, fix every in-scope execution artifact or evidence problem directly, reconcile with sprint status, and repeat the review dry-run until it is ready or a genuine blocker must be reported. Do not launch the actual review from this skill.`,
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

Verify the implemented sprint against the current review gate and the project's smoke requirements. Keep the work bounded to the declared target and harness mutation roots, capture reproducible evidence, and report failures or blockers honestly.`,
			StageWorkflow: `Use CLI status, validation, or a dry-run preview when useful to discover the review gate, bounded smoke scope, harness, and safety constraints.

Perform the smoke verification yourself: inspect the declared harness, run the selected checks directly with your tools, capture the required evidence, and create or update the governed smoke artifact according to the resolved contract. Do not call ` + "`sprint smoke`" + `, ` + "`verify --to smoke`" + `, or ` + "`flow --to smoke`" + ` to execute or complete the stage. Then use validation and status commands to verify and reconcile the result. A failed or blocked result is not a pass. Do not bypass a stale or missing review gate unless the user explicitly requests a supported diagnostic override.`,
		},
	}
}

func ResolveStageSkills(selection string) ([]StageSkill, error) {
	selection = strings.TrimSpace(strings.ToLower(selection))
	if selection == "" || selection == "all" {
		return StageSkills(), nil
	}
	for _, skill := range StageSkills() {
		if skill.Stage == strings.TrimPrefix(selection, "ultraplan-") || skill.Name == selection {
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
	validationStep := fmt.Sprintf("Run `ultraplan sprint <project> <sprint> validate %s` when supported. Fix validation findings within this stage rather than declaring success early.", skill.Stage)
	if skill.SkipValidation {
		validationStep = "Run the validation commands supported for the affected artifact, then use review/smoke dry runs and sprint status as the cross-stage reconciliation checks."
	}
	ownerRule := "The invoking agent owns the actual stage work. Except for the review stage, do not call an UltraPlan stage, flow, execute, verify, or smoke command to have the CLI or another runtime execute or complete the stage. CLI commands remain appropriate for discovery, effective-prompt resolution, dry-run previews, status inspection, validation, and post-write reconciliation. Review is the deliberate exception: invoke its governed CLI command because UltraPlan owns reviewer subagent fan-out, aggregation, and review state."
	prerequisiteRule := "Validate every prerequisite that has a sprint validation command. If anything is missing, invalid, stale, or internally inconsistent, show the exact gaps and ask whether to fill them. Do not fill prerequisite gaps until the user agrees. If they agree, run the corresponding earlier UltraPlan skills in canonical order, then return to this stage."
	if skill.Stage == "reconcile" {
		prerequisiteRule = "Inspect the prerequisite artifacts and report missing or internally inconsistent evidence. A stale review fingerprint is context for reconciliation, not a prerequisite failure: note the stored and current fingerprints in the proposed analysis, continue classifying the existing review findings against the current implementation, and do not ask to rerun review or stop reconciliation merely because they differ. Do not use `validate execute` as a reconciliation prerequisite after execution: that validator expects unchecked tasks for a new execute attempt, so a fully checked completed plan is expected to fail it; assess completed execution from execute evidence, run state, and the review dry-run instead. Ask before filling genuinely missing prerequisite artifacts or repairing inconsistent durable state."
	}
	if skill.StatusPromptOnly {
		ownerRule = "Act as the execution agent and perform the entire stage manually with your own file-editing and command tools. Use the UltraPlan CLI only for `project <project> status`, `sprint <project> <sprint> status --json`, `sprint <project> <sprint> prompt execute`, and the final `sprint <project> <sprint> review --dry-run --json` readiness check. Do not use any other UltraPlan CLI command during execution, including execution dry-run, validate, execute, verify, smoke, or flow commands."
		prerequisiteRule = "Inspect every prerequisite artifact directly and use fresh sprint state to check completeness and consistency. Do not run CLI validation. If anything is missing, invalid, stale, or internally inconsistent, show the exact gaps and ask whether to fill them. Do not fill prerequisite gaps until the user agrees."
		validationStep = "Inspect the completed implementation and governed execution artifacts yourself and run the plan's required checks directly. Do not use an UltraPlan CLI validation command; use sprint status only to reconcile the state after the manual work is recorded."
	}
	if skill.CanonicalFlow {
		ownerRule = "This manual-only skill deliberately delegates stage execution to the canonical UltraPlan flow command named in the stage workflow. Do not reimplement target resolution, repository inspection, prompt composition, validation, artifact promotion, or state transitions in the invoking agent."
		promptStep = "Use the canonical flow command in the stage workflow. The command resolves and applies the current effective stage prompt; do not reconstruct it in this skill."
	}
	stateRule := "Treat files, the project index, and fresh CLI status as authoritative; never hand-edit flow-state JSON."
	if skill.ManualStateRepair {
		stateRule = "Treat files, the project index, and fresh CLI status as authoritative. Do not hand-edit flow-state JSON except in the explicitly authorized manual review or smoke reconciliation branches below, where every fingerprint, digest, verdict, timestamp, evidence identity, and completion identity must be updated coherently and immediately verified through validation and sprint status."
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
3. Run all UltraPlan commands from the resolved workspace root. Run `+"`ultraplan project <project> status`"+` and `+"`ultraplan sprint <project> <sprint> status --json`"+`. %s
4. Check these prerequisites:

%s

5. %s
6. If the target is already complete and valid, summarize that state and ask before regenerating or materially changing it.
7. If the user explicitly asks for a proposal, analysis, or discussion only, inspect all relevant evidence and return that without writing artifacts or advancing state. Otherwise, do the stage now; do not stop at a proposal.
8. %s
9. %s
10. Complete the stage-specific workflow, preserve unrelated user edits, and do not cross declared mutation boundaries.
11. %s
12. Run `+"`ultraplan sprint <project> <sprint> status --json`"+` after writes or governed review execution so flow-state, freshness, and artifact status are reconciled. Re-run project status if the project or sprint index changed.
13. Inspect downstream artifacts for references made stale by this change. Update directly coupled indexes/references when safe; otherwise report the exact dependent stage that must be revisited. Never delete or silently rewrite downstream decisions.
14. Finish with the artifact/result paths, validation outcome, state transition, and any remaining blocker or dependent stage.

## Stage workflow

%s

## Canonical stage prompt

The current resolved CLI prompt wins over this embedded baseline.

%s
`, skill.Name, skill.Stage, skill.Name, skill.DisplayName, stateRule, strings.Join(prerequisites, "\n"), prerequisiteRule, ownerRule, promptStep, validationStep, skill.StageWorkflow, strings.TrimSpace(skill.Prompt))
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
