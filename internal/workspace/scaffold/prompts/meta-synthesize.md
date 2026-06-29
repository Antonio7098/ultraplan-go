# Meta Synthesis Instructions

You are synthesizing findings from multiple completed architecture studies into a single coherent meta study output.

## Files Referenced

1. `../../prompts/meta-synthesize.md` — These instructions
2. `../../templates/meta-report.md` — Output template
3. Per-study final reports in `studies/{study-name}/reports/final/*.md`
4. Per-study summary.csv for scores

## Core Principles

### Question-Led Synthesis

The meta study exists to answer specific questions. Every section should contribute to answering at least one of those questions. Do not produce a concatenation of study reports — produce a coherent argument.

### Index Before Ingesting

Before reading final reports, understand the inventory:
- How many studies? How many sources each?
- What dimensions does each study cover?
- Which sources are top performers by score?

Use the inventory summary to prioritize which reports to focus on.

### Pattern Over Summary

A meta study should extract patterns, not summarise each study separately. Group findings by theme across studies. Identify what converges despite different implementation choices.

### Preserve Evidence

Every claim must cite a source. Use the format `study-name/dimension:score` or reference specific findings from final reports. Do not invent patterns or tradeoffs.

### Distinguish Divergence from Quality

Some differences are valid responses to different constraints (scale, language, domain). Do not treat all differences as quality differences. Explain *why* studies differ.

## Synthesis Workflow

### 1. Scan the Inventory

Use the inventory summary provided in the prompt to understand the landscape before reading reports.

### 2. Read Selectively

Do not try to read every final report in full. Scan the most relevant ones for:
- Pattern evidence (things that appear in multiple studies)
- Exemplar evidence (best-in-class for a dimension)
- Tradeoff evidence (clear benefit/cost pairs)
- Gap evidence (missing data, weak coverage)

### 3. Cluster by Theme

Group findings into:
- **Patterns**: Recurring solutions to similar problems
- **Tradeoffs**: Recurring benefit/cost pairs with context
- **Exemplars**: Best performers per dimension
- **Gaps**: Missing evidence, incomplete studies
- **Decisions**: Rules that emerge from patterns and tradeoffs

### 4. Structure the Output

Follow the template sections. Each section should be self-contained but link to others. The final output should be a coherent document, not a collection of notes.

## Output Requirements

- Use Markdown prose, not just bullet lists
- Tables only where they improve scanning
- Keep per-study notes brief — the synthesis is the point
- Include an evidence index at the end with specific report references
- Do not leave template placeholders unfilled

## Working with Scores

The `summary.csv` files contain scores across dimensions. Use these to:
- Rank sources within each study
- Identify top exemplars across studies
- Spot gaps (sources with low scores or missing dimensions)

Do not average scores across studies unless the studies are directly comparable. Prefer qualitative synthesis over quantitative aggregation.