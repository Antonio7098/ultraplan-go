# Meta Study Planning — Generate Research Program from PRD

Read the provided PRD and generate a complete meta study plan.

## Input

**PRD file**: the file at `{prd}`

Review the PRD thoroughly. It defines the product being researched. The meta study should be structured around the key architectural decision areas that the product requires.

## Output

Write all output files relative to `{ultraplan-root}/meta-studies/{meta-name}/`.

### 1. meta-studies/{meta-name}/meta.yml

Write the meta study YAML file. It defines the research program.

The structure must be:

```yaml
name: {meta-name}
description: One-line description of the meta study purpose
goal: >
  Multi-line description of what this meta study is trying to achieve,
  what architectural questions it must answer, and what product decisions
  it will inform.

studies:
  - name: <slug>
    intent: >
      What this individual study researches and why it matters for the product.

documents:
  - slug: <doc-slug>
    title: <Human-readable Title>
    purpose: >
      What this document answers and why it matters. One to three sentences.
    inputs:
      studies: ["study-slug", "*"]
      reports: final | summary | all
    should_contain:
      - <specific thing 1>
      - <specific thing 2>
      - <specific thing 3>
```

### 2. meta-studies/{meta-name}/generated/*.init.yml

For each study in meta.yml, write a study init YAML file to `generated/<study-name>.init.yml`.

Use the structure:

```yaml
name: <study-name>
description: >
  One-line description of what this individual study covers.

repos:
  count: 5
  items:
    - name: <repo-slug>
      url: https://github.com/<org>/<repo>
      description: Why this repo is relevant to the study

dimensions:
  count: 6
  items:
    - number: "01"
      name: <dimension-name>
      title: <Dimension Title>
      purpose: >
        What this dimension analyses and why it matters for this study.
      steps:
        - <analysis step 1>
        - <analysis step 2>
      evidence:
        - <evidence item 1>
        - <evidence item 2>
      questions:
        - <question 1>
        - <question 2>
```

## Repo Verification (CRITICAL)

Before writing any init YAML file, you MUST verify each repo URL actually resolves. Use the Bash tool to check:

```bash
curl -sI https://github.com/<org>/<repo> | head -1
```

A valid repo returns `HTTP/2 200`. If the repo does not exist or has moved, the response will be `HTTP/2 404` or redirect to a generic page.

**Process for each repo:**
1. For each proposed repo, run `curl -sI <url> | head -1`
2. If HTTP/2 200: keep the repo, write it to the init YAML
3. If HTTP/2 404 or any other non-200 response: do NOT include that repo
4. Find a replacement repo that does resolve and serves the same purpose
5. Verify the replacement with curl before including it

**Common issues to avoid:**
- Repos that have been renamed or deleted
- Repos with typos in the org or repo name
- Repos that require authentication
- Subdirectory URLs (e.g., `github.com/org/repo/tree/main/libs`) instead of repo root URLs

Do not write any repo URL that you have not personally verified with curl.

## Planning Rules

1. **Studies are focused research units** — each study should cover one major architectural concern (e.g., observability, performance, agent tooling, workflow patterns, data modelling).
2. **Documents are question-specific syntheses** — each meta document answers a specific architectural question by drawing from multiple studies.
3. **Match document scope to studies** — a document that covers observability should only list observability-study and agent-runtime-study in `inputs.studies`, not all studies.
4. **Be specific in should_contain** — instead of "findings", write "transferable patterns for event-driven observability", "failure modes for async agent runs", "decision rules for log levels".
5. **Name things clearly** — study names use kebab-case slugs. Document slugs use kebab-case. Titles are human-readable.
6. **Dimension count is a target** — if you cannot fill all dimensions from existing knowledge, use fewer and note them as the core set.

## Process

1. Read the PRD carefully. Identify the major architectural concerns the product must address.
2. Define studies that each cover one major concern. Write clear intents for each.
3. Define meta documents that each answer one key architectural question. Be specific about what each should contain.
4. For each study, propose repos. **Immediately verify each URL with `curl -sI` before writing to the init YAML.** Replace any that return 404 with a working alternative.
5. For each study, define dimensions that cover the key analysis angles within that concern.
6. Write all files.

## Output Locations

- meta.yml → `{ultraplan-root}/meta-studies/{meta-name}/meta.yml`
- study init files → `{ultraplan-root}/meta-studies/{meta-name}/generated/<study-name>.init.yml`

Do not write any other files. Do not create directories beyond those specified. Write real content — not placeholder comments or empty structures.

Work thoroughly. A good meta study plan makes the research program clear and the individual studies focused.