# Audit templates

Use only the columns that help the current audit. Add exact paths and function names as evidence.

## Model-call inventory

| Call ID | Product stage | Purpose | Executor now | Tools and permissions | Fan-out | Session policy | Output and validator |
|---|---|---|---|---|---:|---|---|
| | | | deterministic / API / agent | | | fresh / continued | |

Include repair, retry, challenger, evaluator, and conflict paths as separate rows.

## Context ledger

| Call ID | Input ID | Producer | Required | Delivery | Bytes or tokens | Mutable | Freshness identity | Trust | Prefix group | Model work if absent |
|---|---|---|---|---|---:|---|---|---|---|---|
| | | | yes / optional / forbidden | inline / excerpt / packet / path / retrieval / live | | | | governed / untrusted | | |

Distinguish content delivered to the model from a path the model must read.

## Reuse groups

| Group | Consumers | Exact ordered blocks | First volatile byte | Cohort settings | Breakpoint | Expected reuse count | Current cache evidence |
|---|---|---|---:|---|---:|---:|---|
| | | | | provider, model, tools, policy, output format, reasoning | | | |

## Executor decision

| Call ID | Needs live exploration | Needs iterative tools | Side effects | Bounded output | Proposed executor | Reason |
|---|---|---|---|---|---|---|
| | yes / no | yes / no | none / bounded / open | yes / no | code / one-off API / agent | |

If only one substep needs exploration, split it from deterministic preparation and validation.

## Finding

```text
Finding:
Evidence:
Current owner:
Repeated or avoidable work:
Proposed owner and artifact:
Affected calls:
Cache effect:
Quality or freshness risk:
Measurement needed:
Priority:
```

## Before-and-after scorecard

| Metric | Baseline | Candidate | Change | Source |
|---|---:|---:|---:|---|
| Successful workflows | | | | |
| Validation pass rate | | | | |
| Prompt bytes | | | | |
| Fresh input tokens | | | | |
| Cached input tokens | | | | |
| Cache-write tokens | | | | |
| Output tokens | | | | |
| Reasoning tokens | | | | |
| Tool calls | | | | |
| Repeated search/read calls | | | | |
| Repair and retry calls | | | | |
| End-to-end duration | | | | |
| Cost per successful workflow | | | | |
| Escaped defects | | | | |

State whether each value is measured, calculated from deterministic fixtures, or estimated.

