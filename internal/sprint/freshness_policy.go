package sprint

// Snapshot-based freshness is temporarily disabled because unrelated, concurrent,
// or very small edits make completed review and smoke evidence unnecessarily
// brittle. Keep the implementation behind these switches so it can be
// reintroduced once freshness can be attributed to the operation that made the
// relevant change instead of treating every filesystem delta as invalidating.
//
// Artifact existence, format, recorded digest checks, and the smoke harness
// authoring allowlist remain enforced while these switches are disabled.
const (
	strictCompletedReviewSnapshotFreshness = false
	strictCompletedSmokeSnapshotFreshness  = false
	strictSmokeAuthorProtectedSnapshots    = false
)
