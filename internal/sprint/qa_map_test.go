package sprint

import (
	"bytes"
	"strings"
	"testing"
)

func TestQAMapBuildIsByteStableAndOwnsEveryChangedPathOnce(t *testing.T) {
	input := qaMapInputFixture()
	first, err := BuildQAMap(input)
	if err != nil {
		t.Fatal(err)
	}
	input.ChangedPaths = []string{"internal/web/handlers.go", "internal/sprint/qa.go", "internal/app/usecases.go"}
	second, err := BuildQAMap(input)
	if err != nil {
		t.Fatal(err)
	}
	firstBytes, _ := NormalizedQAMapBytes(first)
	secondBytes, _ := NormalizedQAMapBytes(second)
	if !bytes.Equal(firstBytes, secondBytes) || first.ID != second.ID || first.SemanticAttemptID != second.SemanticAttemptID {
		t.Fatalf("map is not deterministic:\n%s\n%s", firstBytes, secondBytes)
	}
	counts := map[string]int{}
	boundary := 0
	for _, shard := range first.Shards {
		switch shard.Kind {
		case QAShardPrimary:
			for _, path := range shard.ChangedPaths {
				counts[path]++
			}
		case QAShardBoundary:
			boundary++
			if shard.BoundaryReason != "cross-package" || len(shard.OverlapPaths) < 2 {
				t.Fatalf("boundary shard = %+v", shard)
			}
		}
	}
	for _, path := range first.Coverage.ChangedPaths {
		if counts[path] != 1 {
			t.Fatalf("primary ownership for %s = %d", path, counts[path])
		}
	}
	if boundary != 1 {
		t.Fatalf("boundary shards = %d", boundary)
	}
	if len(first.EffectiveSources) != 1 || first.EffectiveSources[0].Field != "qa.model" {
		t.Fatalf("effective source trace = %+v", first.EffectiveSources)
	}
}

func TestQAMapBuildBlocksUnknownPathsWithoutOmission(t *testing.T) {
	input := qaMapInputFixture()
	input.ChangedPaths = []string{"assets/binary.weird"}
	result, err := BuildQAMap(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Shards) != 1 || result.Shards[0].Kind != QAShardPrimary || result.Shards[0].Phase != QAPhaseBlocked || result.Shards[0].Blocker == nil {
		t.Fatalf("unknown path map = %+v", result)
	}
	if len(result.Coverage.BlockedPaths) != 1 || result.Coverage.BlockedPaths[0] != "assets/binary.weird" {
		t.Fatalf("blocked coverage = %+v", result.Coverage)
	}
}

func TestQAMapBuildEnforcesLimitsWithoutOmission(t *testing.T) {
	input := qaMapInputFixture()
	input.Settings.Budgets.ChangedPaths = 2
	if _, err := BuildQAMap(input); err == nil {
		t.Fatal("changed path exhaustion accepted")
	}
	input = qaMapInputFixture()
	input.Settings.Budgets.PrimaryShards = 1
	if _, err := BuildQAMap(input); err == nil {
		t.Fatal("primary shard exhaustion accepted")
	}
}

func TestQAMapBuildFingerprintChangesInvalidateStableIdentity(t *testing.T) {
	base := qaMapInputFixture()
	first, err := BuildQAMap(base)
	if err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*QAMapInput){
		"implementation": func(i *QAMapInput) {
			i.ImplementationFingerprint = strings.Repeat("8", 64)
			i.Target.Fingerprint = i.ImplementationFingerprint
		},
		"review": func(i *QAMapInput) { i.ReviewFingerprint = strings.Repeat("9", 64) },
		"policy": func(i *QAMapInput) { i.PolicyFingerprint = strings.Repeat("a", 64) },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := base
			mutate(&candidate)
			result, err := BuildQAMap(candidate)
			if err != nil {
				t.Fatal(err)
			}
			if result.ID == first.ID || result.SemanticAttemptID == first.SemanticAttemptID {
				t.Fatalf("%s change reused stable identity", name)
			}
		})
	}
}

func TestQAMapBuildCarriesRiskAndAdjacentContext(t *testing.T) {
	input := qaMapInputFixture()
	result, err := BuildQAMap(input)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, shard := range result.Shards {
		if shard.Kind == QAShardPrimary && containsString(shard.ChangedPaths, "internal/web/handlers.go") {
			found = containsString(shard.RiskTags, "public-api") && containsString(shard.ContextPaths, "internal/web/handlers_test.go")
		}
	}
	if !found {
		t.Fatalf("web risk/context missing: %+v", result.Shards)
	}
}

func qaMapInputFixture() QAMapInput {
	settings := QASettings{Runtime: StageRuntime{Model: "openai/qa", Variant: "high"}, Budgets: DefaultQABudgets(), Sources: []QAEffectiveSource{{Field: "qa.model", Source: "workspace"}}}
	paths := []string{"internal/app/usecases.go", "internal/sprint/qa.go", "internal/web/handlers.go"}
	return QAMapInput{
		Project: "alpha", Sprint: "01-test", ChangedPaths: paths,
		ContextPaths:    map[string][]string{"internal/web/handlers.go": {"internal/web/handlers_test.go"}},
		ExpectationRefs: []string{"REQ-WEB", "REQ-MAP"}, RiskTags: qaRiskTags(paths),
		GovernedInputFingerprint: strings.Repeat("1", 64), ImplementationFingerprint: strings.Repeat("2", 64),
		ReviewFingerprint: strings.Repeat("3", 64), PolicyFingerprint: strings.Repeat("4", 64),
		CheckCatalogFingerprint: strings.Repeat("5", 64), Target: QATargetIdentity{Fingerprint: strings.Repeat("2", 64)}, Settings: settings,
	}
}
