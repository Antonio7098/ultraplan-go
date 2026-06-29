# Synthesis Dimension — Combined Report Generation

Read all per-source analysis files across all sources and create a single combined study report for this dimension.

## Inputs Provided By UltraPlan

UltraPlan injects the selected dimension, final report template, per-source report manifest, and expected output path into the rendered prompt. Prompt/template paths in metadata are provenance labels, not instructions to read files.

## Instructions

1. Read the per-source analysis files listed in the injected synthesis inputs.
2. Do NOT access any source code directly — all evidence is already captured in the analysis files.
3. Build a normalized inventory from the per-source reports before writing the final report.
4. Synthesize findings across all sources into a single combined report.
5. Write the report to the expected output path provided by UltraPlan prompt metadata using the injected final report template section.
6. Fill in every template section. Do not leave placeholders behind.

## Synthesis Workflow

### 1. Normalize the Source Reports

For each source, extract:

- Overall rating and short rationale.
- Approach model for this dimension, using the vocabulary from the injected selected dimension section. For project structure this might be an architectural archetype; for another study it might be an error-handling model, testing strategy, configuration model, release pattern, plugin model, UX pattern, or performance strategy.
- Where the studied behavior is implemented.
- Main mechanism used to solve the dimension's problem.
- Supporting mechanisms, abstractions, policies, conventions, or workflows.
- Standout patterns worth copying for this dimension.
- Tradeoffs and failure modes.
- Questions, gaps, or missing evidence.

### 2. Cluster Before Comparing

Group sources by dimension-relevant approach before making broad claims. Use categories that emerge from the per-source reports and the selected dimension.

The final report should explain both:

- What converges across sources despite different implementation choices.
- Why sources diverge based on product shape, maturity, user needs, public API needs, operational constraints, compatibility requirements, performance constraints, or library-vs-application constraints.

Do not treat all differences as quality differences. Some are valid responses to different constraints.

### 3. Extract Patterns, Not Just Summaries

A pattern belongs in the final report if it appears in multiple sources or if one source demonstrates it unusually clearly.

For each pattern, explain:

- What problem it solves.
- Which sources demonstrate it.
- Why it works.
- When to copy it.
- When it is overkill or risky.
- What evidence supports it.

Avoid generic claims like "clean architecture" or "good separation" unless the report names the concrete mechanism.

### 4. Analyze Tradeoffs

For every major design choice, capture both sides:

- Benefit.
- Cost.
- Best-fit context.
- Failure mode.
- Alternative approach seen in another source.

Prefer comparative statements: "Source A uses X because..., while Source B uses Y because..." rather than isolated source recaps.

### 5. Produce Practical Guidance

The final report should include concrete tips for someone applying this dimension's lessons:

- Patterns to copy.
- Patterns to avoid or delay until needed.
- Decision rules for choosing between the main approaches found in the source reports.
- Caution signs that indicate the studied design area is becoming brittle, over-coupled, under-specified, hard to test, hard to operate, or hard to evolve.

### 6. Preserve Evidence Discipline

Use only evidence from the per-source reports. Every major claim must cite at least one source report and at least one code evidence reference from that report where available.

If evidence is missing, say so explicitly. Do not invent line numbers, files, motivations, or enforcement mechanisms.

## Formatting Guidance

- Favor inline Markdown prose and short bullets over tables.
- Use tables only where they materially improve scanning. The rating summary MUST be a table.
- Keep per-source findings brief. The final report is a synthesis, not a concatenation of source reports.
- Prefer sections that answer "what should I learn from this?" over sections that merely list "what each source does."

## Required Rating Summary

Aggregate ratings across sources into `{{rating_summary}}` as a Markdown table with one row per source.

Use this shape unless the selected dimension provides a stronger rating model:

| Source | Score | Approach | Main Strength | Main Concern |
|--------|-------|----------|---------------|--------------|

If the per-source reports include per-dimension ratings, include those dimensions as additional columns. If they only include one overall score, do not invent dimension scores.

## Output

- Combined report: the expected output path provided by UltraPlan prompt metadata.

Work thoroughly. This is a comparative architecture study, not a surface skim.
