# Automated Sprint Review

This is UltraPlan's embedded default for the product-owned `review` stage. A
workspace `prompts/review.md` is an optional override; it is never a required
planning input.

Review the frozen manifest assembled by UltraPlan. Work read-only against the
approved implementation target. Do not write files, mutate Git, install
dependencies, or expand sprint scope.

UltraPlan starts one independent reviewer for every selected contract and one
for the technical handbook. Each reviewer must return the structured result
requested by the runtime prompt, including an explicit applicability decision,
severity, action, and real path/line citations for every applicable finding.

Treat missing coverage, malformed results, unsafe citations, unsupported
permission enforcement, or changed inputs as review failure. UltraPlan owns
aggregation, deterministic verdict calculation, validation, atomic replacement
of sprint-root `review.md`, and review state in `flow-state.json`.
