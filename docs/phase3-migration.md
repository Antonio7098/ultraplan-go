# Phase 3 Verification Migration

Legacy manually maintained verification files are not authoritative Phase 3 evidence.

1. Back up any manual `review.md` or `deep-smoke.md` that must be retained outside the sprint directory.
2. Remove the legacy sprint-root files.
3. Run `ultraplan sprint <project> <sprint> status` and repair execute prerequisites.
4. Preview with `verify --to smoke --dry-run`.
5. Run `verify --to smoke --yes` to generate current `review.md`, `smoke.md`, and versioned flow state.
6. Validate with `validate review`, `validate smoke`, and `status`.

Do not rename `deep-smoke.md` to `smoke.md`: generated Phase 3 artifacts contain fingerprints, verdicts, evidence identity, and recovery facts that a legacy manual file does not prove. Raw run and issue evidence remains in the cataloged external smoke harness.

Unknown durable-state versions fail closed. Preserve the file for diagnosis, then use the documented recovery path rather than hand-editing a version number.
