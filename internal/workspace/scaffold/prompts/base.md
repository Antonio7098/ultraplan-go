# Base Dimension — Execution Instructions

This file defines the shared workflow for one study analysis task. Each analysis task covers exactly one selected source and one selected dimension. UltraPlan injects this base prompt, the selected dimension, the selected source metadata, and the output template into the rendered prompt.

## Hard Rules

These rules are NOT optional. Violations invalidate the study.

1. **NO cross-source filesystem access.** You are studying exactly one selected source. You may ONLY access files inside that source's directory. Accessing files from another source (e.g., reading `../other-source/`) is BANNED. Each source is studied in isolation in its own task.
2. **EVERY code mention MUST include a file path.** Whenever you reference a class, function, type, config key, test, or any code element, you MUST include the file path and line number (e.g., `src/core/loop.ts:42`). Line numbers are highly encouraged even for non-code mentions.
3. **Cite evidence, not vibes.** Every claim about architecture, patterns, or tradeoffs must trace back to a specific file path. If you cannot find evidence, state "No evidence found" and describe what you searched.

Violations of rules 1 or 2 require a rewrite before the study can be accepted.

## Execution Workflow

1. **Use the injected dimension section**
   - Use the injected selected dimension section for the study-specific purpose, steps, evidence, and questions.

2. **Analyze the selected reference source**
   - Inspect only the selected source for the selected dimension.
   - Prefer implementation, tests, configuration, and public interfaces over README-level claims.
   - Assign a rating score (1–10) based on the rubric in the selected dimension file. Include the score and rationale in the output.
   - Write findings to the expected output path provided by UltraPlan prompt metadata using the injected repository analysis template section.

## Quality Bar

Each study should:

- Cite concrete evidence for major findings: file paths AND line numbers, symbols, config keys, tests, docs, or observed behavior.
- Distinguish implemented behavior from inferred intent.
- Capture tradeoffs and failure modes, not just feature presence.
- Call out missing evidence explicitly when a question cannot be answered.
- Keep recommendations specific enough to become engineering work.
- Format every evidence citation as `path/to/file.ts:NN` — not just a filename alone.

## Template Usage

- Use the injected repository analysis template section for the selected source analysis.
- Treat `{{...}}` markers in the injected template as placeholders that must be replaced in the final output.
- Replace template placeholders with concrete prose, tables, or bullet lists as appropriate.
- Do not leave empty sections. If there is no finding, write `No clear evidence found` and explain the search boundary.

## Output Path

Write the final analysis to the exact `Expected output` path in the rendered prompt metadata. That path may be absolute; use it as-is. Do not derive a shorter relative path from the examples or from the current working directory.

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
