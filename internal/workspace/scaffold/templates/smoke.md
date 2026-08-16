# Sprint Smoke

Smoke status: `completed`
Verdict: `blocked`

## Smoke Context

Project, sprint, and artifact identity.

## Review Gate

Current review verdict, governed-input fingerprint, and diagnostic override fact.

## Harness And Protocol

Cataloged harness identity and supported protocol-v1 version.

## Smoke Authoring

Authoring runtime/model identity and the bounded harness paths created or
changed before authoritative execution.

## Selected Scope And Rationale

Narrowest sufficient level, suite set, or diagnostic test and its selection reason.

## Preconditions And Environment

Prerequisite status and the environment-name allowlist; never persist values.

## Safe Invocation

Sanitized executable and argv display.

## Run Evidence

Run identity, counts, duration, and optional runtime/model metadata.

### External Evidence Identity And Links

Contained external evidence paths with hashes or fallback identity metadata.

## Findings

Safe diagnostics and selected test findings.

## Open Issues

Relevant open issue identities and links.

## Resolved Issues

Relevant external issue identities and links.

## Mutation And Safety Check

Approved product and harness mutation roots, including confirmation that the
product target and governed sprint inputs were unchanged during authoring.

## Verdict And Next Action

One of `pass`, `pass_with_open_issues`, `fail`, `blocked`, or `not_applicable`, followed by one explicit next action.
