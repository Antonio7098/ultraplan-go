# Changelog

This file records notable changes validated by the UltraPlan QA dogfood campaigns.

## 2026-09-02

### Changed

- Removed blanket no-tool instructions from QA roles and output-repair prompts. Complete direct-input packets are now an optimization, not a permission restriction.
- QA semantic mappers, investigators, challengers, arbiters, reconcilers, and failed-evidence evaluators now retain bounded read, list, search, and glob permissions under the read-only sandbox. Mutation and shell tools remain denied pending a separate investigator-test design.

### Fixed

- Removed an ignored 39,099,697-byte fixture binary that exceeded the 32 MiB evidence-isolation file limit. The binary and its checksum remain in persistent campaign evidence.

### Verified

- Reran the same QA fixture with the new policy. The completed attempt passed eight of eight shards and produced ten isolated evidence records with assessment `pass`.
- Retained 32 per-call rows with 531,830 input tokens, 73,574 output tokens, 1,015,291 cache-read tokens, and 1,620,695 total reported tokens.
- Verified policy translation from UltraPlan read, list, search, and glob allows to OpenCode native read, list, grep, and glob permissions. All calls reported restricted enforcement and zero unsupported tools.
- The model made zero tool calls. The rerun proves permission configuration and translation, but not a successful end-to-end read invocation.

## 2026-09-01

Context-engineering QA campaign conducted from 2026-08-31 through 2026-09-01. The campaign completed with a `pass_with_findings` verdict.

### Added

- Added schema-1 map upgrades and an arbiter theory budget with a legacy default of 24.
- Added semantic mapping, parallel investigation, strict arbiter reference closure, cross-group reconciliation, isolated evidence checks, and evidence-backed adjudication.
- Added configurable arbiter limits. Campaign runs at limits 1, 2, 4, and 24 kept every group within its configured bound.
- Added bounded read-only discovery tools for QA roles. Direct-input packets can avoid discovery without prohibiting verification.
- Added sequential per-issue and grouped parallel repair execution. Both live fixtures repaired and verified 2 of 2 issues.
- Added controlled coverage for cancellation, stale proposals, journal recovery, target drift, cleanup, smoke gates, replay, and reconnect behavior.
- Added per-call QA and repair telemetry for routes, permissions, tokens, cache reads and writes, tool calls, durations, and runtime events.
- Added a live web ledger with distinct rendering for known zero and unknown values, including a narrow 390-pixel layout.
- Added support for ephemeral loopback listeners with port `0` for IPv4 and IPv6.

### Changed

- QA prompts now treat supplied context as sufficient by default while retaining bounded read, list, search, and glob access under a read-only sandbox.
- QA resume now reuses a retained map, and new-start conflict checks run before a paid mapper call.
- Arbiter contracts now list accepted wire values, require complete theory coverage, preserve valid coverage during repair, and reject Markdown framing or surrounding prose.
- Evidence isolation now accepts contained relative symlinks, copies their link text without following them, and includes link type and target in identity hashes. Escaping links, hard links, and special files still fail closed.
- Repair help now states the conformance-review and containing-smoke admission requirements.
- Repair budget reporting now exposes environment overrides as `environment` instead of the internal `env` label.

### Fixed

- Fixed legacy QA maps being rejected when `arbiter_max_theories` was absent.
- Fixed incomplete arbiter diagnostics that discarded the second failed repair attempt.
- Fixed arbiter outputs referring to block-like identifiers outside their delivered context.
- Fixed arbiter repair turns dropping otherwise valid issue coverage.
- Fixed evidence isolation rejecting safe symlinks that target identity had already accepted.
- Fixed `serve --listen 127.0.0.1:0` failing before it could bind an ephemeral port.

### Verified

- Passed UltraPlan tests, race tests, vet, diff checks, Agentwrap checks, semantic mapping, reconciliation, the four-value arbiter matrix, cache comparison, web-ledger inspection, and controlled recovery lanes.
- Verified identical prompt and prefix construction across same-path cache trials. Provider cache reads increased from 128 to 1,941 tokens in the second mapper trial.
- Completed the live sequential repair campaign in 68.620 seconds with 14,758 reported tokens, down 72.2% from the earlier grouped proof.
- Completed the live grouped parallel repair campaign in 142.278 seconds with 18,141 reported tokens, down 65.8% from the earlier grouped proof. The single grouped worker queue provided no cross-worker latency benefit.

### Known limitations

- Live stale-proposal regeneration after overlapping mutation has not been exercised.
- Live cancellation during proposal or verification has not been injected.
- Live final-smoke failure after an earlier pending success has not been injected.
- Deterministic controlled tests cover these failure paths, but they are not substitutes for live fault-injection runs.

## 2026-08-30

QA and bounded-repair campaign conducted from 2026-08-28 through 2026-08-30, covering Sprint 38 and building on the read-only QA and evidence work from Sprints 36 and 37.

### Added

- Added durable repair campaigns that coordinate ordered issue assignments while keeping one repair run, packet, isolated mutation copy, apply journal, and result per issue.
- Added grouped repair queues with one reusable model session per worker and serial production mutation.
- Added `verified_pending_campaign` for intermediate issues whose scoped gates pass before the campaign's final containing-smoke gate.
- Added deferred containing-smoke records with a reason and next action. The final issue must pass the complete verification ladder before campaign completion.
- Added stable private worker paths that recreate a fresh isolated copy for each queued issue while preserving model-session continuation.
- Added durable campaign projections for worker queues, current items, completed counts, cleanup, mutation, commands, timing, usage, and outcomes.
- Documented the distinction between repair assignments, repair runs, and repair campaigns.

### Changed

- Rebased each subsequent repair packet to the observed current target under the campaign's writer-fenced authority.
- Updated Agentwrap usage projection to retain input, output, cache, timing, and reported-cost fields returned by OpenCode.
- Updated the external Sprint 38 smoke fixture to use a unique valid nonexistent issue ID and accept the intended class of authority rejection.

### Fixed

- Fixed equivalent repair findings being promoted more than once. Deduplication now uses normalized claim, issue class, and location, unions evidence, keeps the strongest severity, and preserves repair eligibility.
- Fixed Git implementation fingerprints being compared with map fingerprints during real-QA evidence identity checks.
- Fixed bounded evidence before-and-after identity checks that could compare a value with itself.
- Fixed grouped queues rerunning broad QA between issues and lacking a truthful intermediate success state.
- Fixed stale fixture identities causing repeated smoke runs to fail for an unrelated retained-state collision.

### Verified

- Completed a real two-issue grouped campaign with one shared model session, fresh isolation and cleanup per issue, serial applies, and a passing final containing-smoke gate.
- Verified 2 of 2 issues in about 110 seconds. The two repair proposals reported 168 input tokens, 67 output tokens, and 52,837 cache-read tokens.
- Passed focused Go tests, race tests, vet, diff checks, the external Sprint 38 smoke fixture, and live dogfood execution.

### Known limitations

- Queues with three or more issues and campaigns with multiple non-empty worker queues were not exercised.
- Cancellation, runtime recovery after production apply, target-drift escalation, failed intermediate gates, failed final smoke, session loss, and runtime-store cleanup still required fault-injection coverage at the end of this campaign.
