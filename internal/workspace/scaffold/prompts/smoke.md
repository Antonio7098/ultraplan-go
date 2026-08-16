# Deep Smoke Authoring

This is UltraPlan's embedded default for the product-owned smoke-authoring
phase. The phase runs after a current sprint review and before the external
smoke suite is selected or executed.

Design and implement deep-smoke coverage for exactly the requested sprint in
the catalogued external smoke harness.

## Purpose

Normal unit and integration tests are deterministic and fake-first. Do not
merely rerun or wrap those checks. Inspect their evidence, then create durable
external probes for the behavior they cannot prove: real binaries and process
lifecycle, real HTTP/browser behavior, real filesystems and persisted state,
real provider/model execution where the product path uses it, cancellation,
recovery, security boundaries, and cross-surface integration.

## Required method

1. Read the sprint requirements, reasoning, plan, execute evidence, review,
   implementation diff/current target, and existing deterministic tests.
2. Build a coverage matrix for every sprint acceptance criterion. Mark what is
   already proven deterministically and give every remaining criterion one or
   more newly authored deep-smoke probes.
3. Add or update one rerunnable sprint-specific harness suite, its fixtures,
   and its protocol discovery mapping.
4. Discovery must enumerate every test identity and its coverage IDs. Declare
   the sprint mapping complete only when the union of those IDs covers every
   required deep-smoke coverage ID.
5. Execute the smallest useful harness self-checks while authoring. Do not run
   the authoritative smoke lane; UltraPlan runs it after validating discovery.
6. Leave concise maintainable tests and deterministic cleanup. Use unique
   temporary paths and ports, disable language test caches where freshness is
   evidence, and never depend on an already-running local service.

## Safety

- Modify only the manifest-declared harness authoring paths.
- Do not modify the product implementation, planning workspace, governed
  sprint inputs, Git state, or prior run/issue evidence.
- Do not weaken an assertion to obtain a pass and do not claim coverage from a
  command that does not exercise the behavior.
- Real provider calls are required when the product path under smoke invokes a
  provider. Do not add irrelevant provider calls to paths that are local-only.
- Never persist credentials, environment values, full provider payloads, or
  secret-bearing command output.

Finish only when protocol discovery exposes a non-empty, traceable, complete
sprint suite that the external runner can execute independently.
