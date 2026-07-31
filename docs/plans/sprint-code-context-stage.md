# Plan: Sprint Code Context Stage

## Summary

Add a dedicated `code-context` planning stage immediately after `requirements`.

The stage runs one agent against the sprint requirements, selected sprint scope, and project source repository. The agent explores the repository and writes a curated `code-context.md` pack containing the code needed to understand and plan the sprint.

That stored pack is then injected unchanged into every downstream agent prompt. This gives later agents a shared foundation instead of requiring each one to rediscover the same implementation independently.

The pack is not a restrictive boundary. Downstream agents may inspect additional repository files whenever that would improve their work.

The design deliberately does **not** introduce an UltraPlan cache subsystem, cache keys, source indexes, amendment workflows, or a second machine-readable context format. UltraPlan runs the stage once, stores the artifact, and reuses it.

## Goals

- Explore the project repository once for a sprint rather than repeating the same discovery in every stage.
- Give all downstream agents the same high-quality understanding of the existing implementation.
- Select enough code to support thorough reasoning while avoiding a repository dump.
- Preserve the selected source text in one durable sprint artifact.
- Arrange downstream prompts so the common requirements and code-context prefix is byte-for-byte stable, allowing provider prompt-caching optimisations where supported.
- Keep agents free to inspect more code when the stored pack is insufficient.
- Fit the new stage into the existing stage, prompt, validation, flow-state, defaults, skill, and documentation conventions.

## Non-goals

- Building a repository index, embedding store, retrieval database, or symbol graph.
- Adding an UltraPlan cache directory or deriving cache keys.
- Automatically refreshing the pack when source files change.
- Preventing downstream agents from reading the repository.
- Requiring downstream agents to request formal context-pack amendments.
- Splitting the pack into JSON metadata plus Markdown rendering.
- Guaranteeing provider cache hits or adding provider-specific cache APIs in this change.
- Reusing the project code-context workflow for studies in the first implementation.

## Stage order

The planning chain becomes:

```text
requirements
→ code-context
→ sprint-index
→ technical-handbook
→ area-reasoning
→ reasoning
→ plan
```

Execution and verification stages should also receive the pack when they build an agent prompt:

```text
plan
→ execute
→ review
→ smoke
```

`requirements.md` must exist and validate before `code-context` can run. A valid `code-context.md` makes `sprint-index` ready.

## Artifact

The authoritative output is one Markdown file:

```text
projects/<project>/sprints/<sprint>/code-context.md
```

Do not add a parallel JSON manifest. The Markdown file should contain both the selection rationale and the actual source excerpts needed by downstream agents.

The file should be treated like the other sprint planning artifacts:

- persisted in the sprint directory;
- included in status and flow state;
- validated before dependent stages run;
- regenerated only when the user explicitly reruns the stage;
- committed when sprint artifacts are committed.

## Context-agent inputs

The `code-context` prompt should provide the agent with:

- `requirements.md`;
- the sprint identity and output path;
- the selected roadmap or project scope already used to create the requirements;
- relevant project documentation and contracts available to the sprint workflow;
- prior sprint decisions or outputs when they are already part of the selected project context;
- the project source repository path or worktree;
- the `code-context.md` template.

The project source repository must be available as the runtime working context so the agent can search, read, and trace the implementation. Reuse the project repository/worktree resolution already used by implementation stages. Add only the smallest missing wiring needed to expose that repository to the planning runtime; do not introduce a new repository indexing subsystem.

The stage may read the source repository but must only write `code-context.md` in the UltraPlan sprint directory.

## Context-agent responsibilities

The agent should inspect the repository broadly enough to understand the sprint, then select the smallest useful set of source excerpts that establishes a strong foundation for downstream work.

For each material part of the sprint scope, it should look for the relevant:

- current implementation;
- public interfaces and package/module boundaries;
- callers and consumers;
- data structures and persisted state;
- configuration and defaults;
- validation and error paths;
- tests and test helpers;
- adjacent code that demonstrates local conventions;
- partial implementations, contradictions, or notable absences;
- likely change locations.

The agent may inspect considerably more files than it includes. The pack should contain code that materially helps later agents understand design constraints, behaviour, or likely implementation work.

The agent must not make the final architecture decisions or write the sprint plan. It may explain why code is relevant and identify uncertainties, but its primary job is source selection and contextualisation.

## Proposed `code-context.md` structure

Add an embedded default template with these sections:

```markdown
# Sprint Code Context: <sprint>

> Project: <project>
> Sprint: <sprint>
> Requirements: projects/<project>/sprints/<sprint>/requirements.md

## Scope Interpreted

A concise description of the source-level questions derived from the sprint requirements.

## Repository Areas Inspected

A concise list of the packages, directories, tests, configuration, and entry points inspected while preparing the pack. This records exploration without copying everything inspected.

## Selected Code

### <stable descriptive heading>

**Path:** `path/to/file.go`
**Lines:** `120-186`
**Symbol:** `OptionalSymbolName`
**Why this matters:** Explanation tied to the sprint scope.

```go
<exact source excerpt>
```

## Important Relationships

A concise explanation of the important call paths, data flow, ownership boundaries, and test relationships visible across the selected excerpts.

## Existing Constraints And Open Questions

Known constraints, incomplete behaviour, ambiguities, or areas that downstream agents may need to investigate further.
```

The selected-code section may contain as many entries as needed. Do not impose an arbitrary fixed count.

The prompt should prefer complete logical units over tiny disconnected fragments. Large files should be reduced to relevant symbols or contiguous ranges rather than copied wholesale.

## Source references

Each selected excerpt should include:

- repository-relative path;
- line range at the time the pack was generated;
- symbol name where useful;
- a short explanation of relevance;
- the exact source text in a language-tagged fenced block.

Line references support navigation and review, but the embedded source text is what downstream agents consume. UltraPlan does not need to re-read or re-materialise each range before every stage.

The pack is a snapshot created for the sprint. If the project source changes enough to make it stale, the user can rerun `code-context`.

## Downstream prompt composition

Every downstream prompt should include the same canonical shared context before its stage-specific instructions.

Use this conceptual order:

```text
1. Stable UltraPlan shared planning instructions
2. Sprint identity
3. Exact requirements.md content
4. Exact code-context.md content
5. Other context shared by the relevant downstream stages
6. Stage-specific instructions, template, output path, and constraints
```

The requirements and code-context text must be inserted without stage-specific decoration or rewriting. Preserve the same headings, whitespace, ordering, and content for every downstream call.

Do not include timestamps, run IDs, stage names, output paths, or other changing values inside the shared requirements/code-context block.

The current prompt-rendering path should be adjusted so stage-specific prompt text does not precede the shared sprint context. Introduce one small shared planning-prompt renderer rather than duplicating prefix assembly across every stage.

This structure allows automatic provider prompt caching to reuse the common prefix when:

- the provider supports prompt-prefix caching;
- the calls use a compatible provider and model;
- they occur within the provider's applicable caching behaviour;
- the prefix remains exactly identical.

UltraPlan should not claim or depend on a cache hit. No UltraPlan cache key or provider-specific cache-control implementation is required for the first version.

## Downstream agent behaviour

The shared prompt should describe `code-context.md` as the prepared foundation for the sprint, not as the only permitted source context.

Use guidance equivalent to:

> The sprint code-context pack contains the source evidence selected up front for this sprint. Use it as the common foundation for your work. You may inspect any additional repository files needed to verify assumptions, resolve uncertainty, deepen analysis, or complete the stage well.

Do not tell agents to avoid repository discovery. Do not require them to report amendment requests. Normal stage outputs can cite or discuss additional files they inspect where useful.

## CLI surface

Add `code-context` everywhere planning stages are accepted:

```bash
ultraplan sprint <project> <sprint> prompt code-context
ultraplan sprint <project> <sprint> validate code-context
ultraplan sprint <project> <sprint> flow --to code-context
ultraplan sprint <project> <sprint> flow --to plan
```

`flow --to plan` should run cumulative stages in this order:

```text
requirements
code-context
sprint-index
technical-handbook
area-reasoning
reasoning
plan
```

A dry run for `code-context` should print the context-agent prompt without writing the artifact or changing flow state.

## Embedded prompt and template

Add:

```text
internal/workspace/scaffold/prompts/create-code-context.md
internal/workspace/scaffold/templates/code-context.md
```

Register them with the existing embedded defaults and `ultraplan defaults install` behaviour.

The context prompt should emphasise:

- requirements-driven repository exploration;
- thorough inspection before selection;
- exact source excerpts;
- useful surrounding context;
- implementation, boundary, error, configuration, and test coverage where relevant;
- concise rationale for each selected excerpt;
- no source-repository mutation;
- only `code-context.md` may be written;
- no final design or implementation plan.

## Domain and flow changes

### Stage model

Add:

```go
StageCodeContext PlanningStage = "code-context"
```

Insert it after `StageRequirements` in `PlanningStages()` and every ordered stage list.

Update:

- stage validity;
- artifact path mapping;
- stage status derivation;
- flow success and failure transitions;
- cumulative flow dispatch;
- prerequisite validation;
- status output;
- flow-state compatibility handling if the persisted schema requires it.

A completed requirements stage should make `code-context` ready. A completed context stage should make `sprint-index` ready.

### Service surface

Add methods following the existing planning-stage pattern:

```go
PromptCodeContext(projectRef, sprintRef string) (PromptPreview, error)
ValidateCodeContext(projectRef, sprintRef string) (ValidationResult, error)
FlowCodeContext(ctx context.Context, projectRef, sprintRef string, req FlowRequest) (FlowResult, error)
```

The runtime-backed flow should:

1. resolve and validate the sprint and requirements;
2. resolve the project source repository/worktree;
3. render the context-agent prompt;
4. run the configured agent in a context where it can read the source repository and write the sprint artifact;
5. confirm `code-context.md` exists;
6. validate it;
7. update flow state.

### Runtime configuration

Add stage-specific model and variant configuration matching existing planning stages:

```yaml
planning:
  code_context_model: ...
  code_context_variant: ...
```

Use existing fallback behaviour when these values are unset.

## Validation

Keep validation structural and useful rather than attempting to prove that the agent found every relevant file.

`ValidateCodeContextContent` should report findings for:

- missing or empty artifact;
- placeholder content;
- missing required sections;
- no selected code excerpts;
- selected entries without a repository-relative path;
- selected entries without a reason;
- selected entries without a fenced source block;
- clearly unsafe absolute paths or paths escaping the project repository;
- malformed line ranges where a range is supplied.

Do not add strict maximum entry counts or reject the pack merely because it is large. Quality and completeness remain primarily the context agent's responsibility.

Validation should not require every downstream discovery to be present in the pack.

## Prompt integration by stage

### Sprint index

Inject requirements and the exact code-context pack. The sprint-index agent should use the existing implementation context when selecting reports, contracts, reasoning areas, and exclusions.

### Technical handbook

Inject the exact pack before stage-specific instructions. The handbook should combine external study evidence with the project's current implementation context.

### Area reasoning and final reasoning

Inject the exact pack before stage-specific instructions. Reasoning agents can use the prepared implementation foundation and still inspect more source when needed.

### Plan

Inject the exact pack before stage-specific instructions so plan tasks and file targets are grounded in the implementation observed before planning began.

### Execute, review, and smoke

When these stages invoke an agent, include the same exact pack in their shared sprint context. These stages will naturally inspect the live repository as part of implementation and verification; the pack simply preserves the original common understanding.

## Manual skill

Add a manually invokable `code-context` stage skill alongside the existing stage skills and include it in skill materialisation.

The skill should:

- inspect flow state and prerequisites;
- confirm `requirements.md` is complete;
- run or propose the `code-context` stage according to the standard skill interaction model;
- validate the resulting artifact;
- keep flow state in sync.

It should use the canonical UltraPlan CLI command rather than duplicating the context-selection procedure inside the skill.

## Documentation updates

Update at least:

- `README.md` stage-chain descriptions and examples;
- `docs/cli-reference.md`;
- `docs/user-guide.md`;
- `docs/recovery.md`;
- planning smoke documentation;
- architecture documentation where planning prompt composition and stage ownership are described;
- generated workspace README/default command examples;
- stage skill documentation and materialisation examples.

Document the central rule clearly:

> `code-context.md` is generated once from the sprint requirements and reused as the shared source foundation for downstream stages. It reduces repeated discovery but does not prevent any agent from inspecting more of the repository.

## Tests

Add focused tests for:

### Stage ordering and state

- `PlanningStages()` includes `code-context` after `requirements`.
- completing requirements makes context ready.
- completing context makes sprint index ready.
- missing or failed context blocks later planning stages.
- existing flow-state migration or compatibility behaviour remains valid.

### CLI

- prompt, validate, and flow commands accept `code-context`.
- help text lists the new stage.
- `flow --to plan` dispatches context exactly once in the correct order.
- dry run does not create `code-context.md`.

### Prompt generation

- the context prompt includes requirements, project scope, source-repository location, template, output path, and write constraints.
- downstream prompts include the exact `requirements.md` and `code-context.md` text.
- the shared prefix is identical across representative downstream stages up to the point where stage-specific instructions begin.
- dynamic metadata is not inserted into the shared prefix.
- downstream guidance explicitly permits additional repository inspection.

### Runtime flow

- a fake runtime can generate a valid context artifact.
- missing output fails the stage.
- invalid output records findings and fails the stage.
- the runtime can read the source repository but the planning stage only writes the sprint artifact.
- stage-specific model and variant overrides are applied.

### Validation

- valid packs pass.
- placeholders, missing sections, missing excerpts, missing paths, missing reasons, and malformed ranges produce actionable findings.

### Defaults and skills

- defaults installation includes the new prompt and template.
- stage skill materialisation includes the code-context skill.
- embedded defaults and materialised defaults stay in sync.

## Implementation sequence

1. Add the stage constant, artifact path, ordering, status derivation, and flow transitions.
2. Add the embedded `code-context` prompt and Markdown template.
3. Add prompt, validation, and runtime-backed service methods.
4. Wire CLI prompt, validate, flow, help, and stage-specific runtime configuration.
5. Build a shared downstream planning-prompt prefix containing exact requirements and code-context content.
6. Inject that prefix into sprint-index, handbook, reasoning, plan, execute, review, and smoke agent requests.
7. Add and materialise the manual stage skill.
8. Add tests for stage flow, validation, prompt-prefix stability, runtime behaviour, defaults, and skills.
9. Update all user and architecture documentation.

## Acceptance criteria

- [ ] UltraPlan exposes `code-context` as a first-class planning stage after `requirements`.
- [ ] The context agent receives the validated sprint requirements and relevant scope, can inspect the project repository, and writes `code-context.md`.
- [ ] `code-context.md` contains selected source excerpts with paths, ranges where useful, and relevance explanations.
- [ ] The context stage runs once during a cumulative flow and the stored artifact is reused by later stages.
- [ ] Sprint-index, technical-handbook, area-reasoning, reasoning, plan, execute, review, and smoke prompts receive the exact stored pack.
- [ ] Requirements and code-context appear in a stable common prompt prefix before stage-specific instructions.
- [ ] No UltraPlan cache subsystem, cache key, repository index, JSON context manifest, or amendment-request workflow is introduced.
- [ ] Downstream agents are explicitly allowed to inspect additional repository code whenever useful.
- [ ] CLI commands, flow state, validation, runtime overrides, defaults, skills, tests, and documentation all recognise the stage.
- [ ] Existing planning and execution flows continue to pass their tests.

## Later possibilities

These should not be included in the first implementation, but the stored pack leaves room for them if real usage demonstrates a need:

- explicit provider cache-control hints;
- automatic stale-pack detection;
- rerunning only the context stage after requirements change;
- context-pack metrics such as input tokens and downstream reuse;
- applying the same gather-once pattern to individual study reports.

They should be earned from observed limitations rather than designed in advance.
