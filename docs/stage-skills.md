# Manually Invoked Stage Skills

UltraPlan embeds one project skill for each governed sprint stage:

| Stage | Skill |
|---|---|
| requirements | `$ultraplan-requirements` |
| sprint-index | `$ultraplan-sprint-index` |
| technical-handbook | `$ultraplan-technical-handbook` |
| area-reasoning | `$ultraplan-area-reasoning` |
| reasoning | `$ultraplan-reasoning` |
| plan | `$ultraplan-plan` |
| execute | `$ultraplan-execute` |
| review | `$ultraplan-review` |
| smoke | `$ultraplan-smoke` |

The skills are interactive forms of the built-in stage prompts. They preserve
the prompt's inputs, outputs, rules, skip criteria, and quality bar, then add
state inspection, prerequisite handling, proposal-only mode, validation, and
state reconciliation.

## Materialise Skills

Write all stage skills into the current workspace:

```bash
ultraplan skills materialise
```

Write one skill:

```bash
ultraplan skills materialise reasoning
```

Preview or choose another workspace:

```bash
ultraplan skills materialise all --dry-run
ultraplan skills materialise all --path /path/to/workspace
```

The American spelling `materialize` is also accepted.

Materialisation writes:

```text
.agents/skills/
  ultraplan-requirements/
    SKILL.md
    agents/openai.yaml
  ...
```

Each `agents/openai.yaml` sets `policy.allow_implicit_invocation: false`.
Consequently, the skills are available for explicit `$skill-name` invocation
but are not selected implicitly.

Missing files are created. Identical files are left unchanged. If an existing
skill file differs from the embedded version, UltraPlan lists it and asks
before overwriting it. A negative or empty answer preserves the local
customisation. Use `--force` only when intentionally replacing customised
skill files.

## Interaction Contract

Every stage skill:

1. Resolves the workspace, project, and sprint without guessing ambiguous
   references.
2. Reads project and sprint status.
3. Validates prerequisites in canonical order.
4. Lists missing, invalid, stale, or inconsistent prerequisites and asks before
   filling those gaps.
5. Asks before replacing an already valid stage.
6. Produces only a proposal or discussion when explicitly requested; otherwise
   performs the stage.
7. Uses the effective CLI prompt when one exists, so project and workspace
   overrides retain precedence over the embedded baseline.
8. Performs the actual stage work itself rather than asking an UltraPlan CLI
   stage or flow command to run another agent/runtime. The CLI remains available
   for discovery, prompt resolution, dry-run previews, status, validation, and
   reconciliation. Review is the sole exception because its governed CLI owns
   reviewer subagent fan-out and aggregation.
9. Validates the result and reconciles flow state with `sprint status --json`.
10. Checks whether downstream references were made stale and reports any stage
   that must be revisited.

The two reasoning skills add an explicit deep-dive path. When requested, the
agent should explore design pressures, alternatives, evidence, trade-offs,
risks, technical debt, and future consequences interactively before recording
final decisions. A request for a deep discussion is not treated as permission
to skip the artifact indefinitely; after conclusions are reached, the default
remains to update the stage unless the user requested discussion or proposal
only.

## State Ownership

Agents must not hand-edit `flow-state.json` or manufacture completion evidence.
They create and maintain stage artifacts themselves when the applicable prompt
requires those artifacts; `review.md` remains owned by the governed review CLI.

- Planning artifacts may be written by the invoking agent, then validated and
  reconciled by `sprint status`.
- Execute is performed directly by the invoking agent, which maintains the plan,
  run state, execution summary, and verification evidence required by the
  effective execution prompt. It must not delegate the work to `sprint execute`.
- Review must use the governed review orchestrator.
- Smoke is performed directly by the invoking agent within the review gate and
  declared harness/mutation boundaries. CLI dry runs and validation may inform
  and verify that work, but the agent must not delegate it to `sprint smoke`,
  `verify --to smoke`, or a flow command.

Running `sprint status` persists the newly derived planning-stage state while
preserving review and smoke evidence. Existing malformed or unsupported flow
state still fails closed instead of being repaired silently.
