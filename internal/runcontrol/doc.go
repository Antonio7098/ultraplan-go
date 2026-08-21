// Package runcontrol owns durable operational run identity and observation.
//
// It deliberately does not own product workflow state, artifacts, runtime
// supervision, or presentation concerns. Product packages decide what work
// means; runcontrol records the accepted execution, fenced owner attempts,
// sanitized ordered events, and the single operational terminal result.
package runcontrol
