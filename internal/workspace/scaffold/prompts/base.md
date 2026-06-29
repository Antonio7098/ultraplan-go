# Base Dimension — Execution Instructions

This file defines the shared workflow for every study dimension. Read it first, then read the selected dimension file for the study-specific purpose, steps, evidence, and questions.

## Hard Rules

These rules are NOT optional. Violations invalidate the study.

1. **NO cross-source filesystem access.** When studying a source, you may ONLY access files inside that source's directory. Accessing files from another source (e.g., reading `../other-source/`) is BANNED. Each source is studied in isolation.
2. **EVERY code mention MUST include a file path.** Whenever you reference a class, function, type, config key, test, or any code element, you MUST include the file path and line number (e.g., `src/core/loop.ts:42`). Line numbers are highly encouraged even for non-code mentions.
3. **Cite evidence, not vibes.** Every claim about architecture, patterns, or tradeoffs must trace back to a specific file path. If you cannot find evidence, state "No evidence found" and describe what you searched.

Violations of rules 1 or 2 require a rewrite before the study can be accepted.

## Execution Workflow

1. **Read the dimension file**
   - Read `../../prompts/base.md` for shared execution rules.
   - Read the selected `../../dimensions/{NN}-{name}.md` for the study content.

2. **Analyze each reference source**
   - For every source in the selected group, inspect the code following the selected dimension.
   - Prefer implementation, tests, configuration, and public interfaces over README-level claims.
   - For each dimension, assign a rating score (1–10) based on the rubric in the dimension file. Include the score and rationale in the output.
    - Write findings to `reports/source/{NN}-{dimension-name}/{source-name}.md` using `../../templates/repo-analysis.md`.

## Quality Bar

Each study should:

- Cite concrete evidence for major findings: file paths AND line numbers, symbols, config keys, tests, docs, or observed behavior.
- Distinguish implemented behavior from inferred intent.
- Capture tradeoffs and failure modes, not just feature presence.
- Call out missing evidence explicitly when a question cannot be answered.
- Keep recommendations specific enough to become engineering work.
- Format every evidence citation as `path/to/file.ts:NN` — not just a filename alone.

## Template Usage

- Use `../../templates/repo-analysis.md` for each per-source analysis.
- Fill every `{{placeholder}}`.
- Replace placeholders with concrete prose, tables, or bullet lists as appropriate.
- Do not leave empty sections. If there is no finding, write `No clear evidence found` and explain the search boundary.

## Output Structure

```
reports/source/{NN}-{dimension-name}/
├── {source-1}.md
├── {source-2}.md
└── ...
```

## Evidence Guidelines

Every piece of evidence MUST include a file path. Include line numbers whenever possible.

Format: `path/to/file.ts:NN` (e.g., `src/core/loop.ts:42`)

Useful evidence includes:

- Source files that implement the behavior under study — with line numbers pointing to the relevant symbols.
- Public APIs, type definitions, interfaces, schemas, or decorators — with line numbers.
- Tests that show intended behavior or edge cases — with test name and line number.
- Runtime configuration, policy files, workflow definitions, or plugin manifests — with line numbers.
- Documentation only when it is tied back to implementation or accepted as a stated design goal — include the file path.

**Bad**: "The agent loop uses an event-driven pattern."
**Good**: "The agent loop uses an event-driven pattern (`src/core/loop.ts:42-58`), dispatching events through a central bus (`src/core/bus.ts:12`)."

Avoid unsupported claims such as "robust", "enterprise-grade", "flexible", or "production-ready" unless the analysis explains the concrete mechanism that earns the label.
