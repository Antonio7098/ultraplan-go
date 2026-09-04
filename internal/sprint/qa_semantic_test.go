package sprint

import "testing"

func TestSemanticReferencesFallBackFromUnknownModelIDs(t *testing.T) {
	allowed := map[string]bool{"REQ-1": true}
	if got := retainQASemanticReferences([]string{"INVENTED", "REQ-1"}, allowed); len(got) != 1 || got[0] != "REQ-1" {
		t.Fatalf("retained refs = %v", got)
	}
	fallback := []QAShard{{ChangedPaths: []string{"internal/a.go"}, ExpectationRefs: []string{"REQ-1"}}}
	proposal := qaSemanticShardProposal{ChangedPaths: []string{"internal/a.go"}}
	if got := fallbackQASemanticExpectations(fallback, proposal); len(got) != 1 || got[0] != "REQ-1" {
		t.Fatalf("fallback refs = %v", got)
	}
}

func TestSemanticContextPathsDiscardUnknownDirectorySuggestions(t *testing.T) {
	available := map[string]bool{"internal/platform/process/isolation.go": true}
	got := retainQASemanticReferences([]string{"internal/platform/process", "internal/platform/process/isolation.go"}, available)
	if len(got) != 1 || got[0] != "internal/platform/process/isolation.go" {
		t.Fatalf("retained context paths = %v", got)
	}
}
