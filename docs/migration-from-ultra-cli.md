# Migration From `.ultra/cli`

UltraPlan Go ports the planning artifact chain from the prototype through `plan.md`. It does not port sprint implementation execution, smoke investigation execution, review automation, issue tracking, automatic Git mutation, hosted UI behavior, or multi-user collaboration.

## Artifact Mapping

Keep the existing project layout when possible:

```text
projects/<project>/
  docs/
  roadmap.md
  project-index.md
  sprints/<sprint>/
    requirements.md
    sprint-index.md
    technical-handbook.md
    reasoning/
    reasoning.md
    plan.md
    flow-state.json
```

Historical `review.md`, `issues.md`, smoke artifacts, and implementation notes may remain in the directory, but the Go planning flow does not generate or validate them as current stages.

## Migration Steps

1. Copy or keep `projects/<project>` under the Go workspace.
2. Run `ultraplan project <project> validate`.
3. Run `ultraplan sprint <project> <sprint> status` to refresh `flow-state.json`.
4. Validate the chain in order:

```bash
ultraplan sprint <project> <sprint> validate sprint-index
ultraplan sprint <project> <sprint> validate technical-handbook
ultraplan sprint <project> <sprint> validate area-reasoning
ultraplan sprint <project> <sprint> validate reasoning
ultraplan sprint <project> <sprint> validate plan
```

5. Use `prompt <stage>` before rerunning a generated stage so edited Markdown remains reviewable.
6. Use `flow --to plan --dry-run` before any runtime-backed planning run.

## Prompt And Template Defaults

The Go CLI embeds the prototype `.ultra/prompts` and `.ultra/system/templates` markdown files as built-in defaults. You do not need to copy prompt or template files into every workspace.

To customize them in a migrated workspace, materialize editable copies:

```bash
ultraplan defaults install --path <workspace>
```

Workspace files override built-ins by relative path. For example:

- `prompts/plan-sprint.md` overrides the built-in sprint planning prompt.
- `prompts/create-sprint-reasoning.md` overrides the built-in sprint reasoning prompt.
- `templates/sprint-plan.md` overrides the built-in sprint plan template.

If a prompt or template already exists and differs from the built-in default, `defaults install` lists the file and asks before overwriting it. Use `--force` only when you intentionally want to replace customized workspace files with the built-in defaults.

## Scope Differences

- `project-index.md` is a catalog. It is not a sprint plan.
- `sprint-index.md` must select from `project-index.md`; unlisted contracts, evidence, templates, or protocols fail validation.
- Technical handbooks distill selected evidence only and should not make implementation decisions.
- `reasoning.md` owns decisions, expected evidence, assumptions, and risks.
- `plan.md` executes `reasoning.md` and must keep task/evidence traceability.
- The Go workflow stops after `plan.md`.

## Release Evidence

For migrated projects, record:

- project validation result.
- sprint status result.
- per-stage validation result.
- whether `flow --to plan --dry-run` passed.
- any historical prototype artifacts intentionally left outside the Go planning flow.
