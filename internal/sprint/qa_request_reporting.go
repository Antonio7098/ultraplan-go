package sprint

// ActiveQAEvidenceRequests returns unresolved endpoints. Missing links and cycles
// remain visible; malformed history must never hide an unresolved obligation.
func ActiveQAEvidenceRequests(requests []QAArbiterEvidenceRequest) []QAArbiterEvidenceRequest {
	byID := map[string]QAArbiterEvidenceRequest{}
	for _, request := range requests {
		byID[request.ID] = request
	}
	var active []QAArbiterEvidenceRequest
	for _, request := range requests {
		if request.Status == "evidence_recorded" || request.Status == "superseded" {
			continue
		}
		seen := map[string]bool{request.ID: true}
		next := request.SupersededBy
		valid := next != ""
		for next != "" {
			child, ok := byID[next]
			if !ok || seen[next] {
				valid = false
				break
			}
			seen[next] = true
			next = child.SupersededBy
		}
		if !valid {
			active = append(active, request)
		}
	}
	return active
}

func qaRequestBlockers(requests []QAArbiterEvidenceRequest) (active, history []QABlocker) {
	endpoints := map[string]bool{}
	for _, request := range ActiveQAEvidenceRequests(requests) {
		endpoints[request.ID] = true
	}
	for _, request := range requests {
		if request.Status == "evidence_recorded" || request.Status == "superseded" {
			continue
		}
		next := request.NextAction
		if next == "" {
			next = "Return the request to the original investigator and record a valid authored-test run."
		}
		blocker := QABlocker{Category: QAErrorMalformedEvidence, Scope: request.ID, Summary: "Evidence request remains unresolved: " + request.ReasonCode, NextAction: next}
		if endpoints[request.ID] {
			active = append(active, blocker)
		} else {
			history = append(history, blocker)
		}
	}
	return
}
