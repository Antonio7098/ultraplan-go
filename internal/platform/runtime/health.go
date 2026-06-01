package runtime

import (
	"context"
	"fmt"

	"github.com/Antonio7098/agentwrap"
)

type HealthRequest struct {
	WorkDir        string
	Provider       string
	Model          string
	Timeout        string
	Checks         []string
	RequiredChecks []string
	Capabilities   []string
}

type HealthReport struct {
	Status       string
	Checks       []HealthCheck
	Capabilities []CapabilityCheck
}

type HealthCheck struct {
	Name    string
	Status  string
	Message string
}

type CapabilityCheck struct {
	Name      string
	Supported bool
	Message   string
}

type Capabilities struct {
	RuntimeKind string
	Features    []CapabilityCheck
}

func (a Adapter) Health(ctx context.Context, req HealthRequest) (HealthReport, error) {
	if a.health == nil {
		return HealthReport{}, fmt.Errorf("runtime does not support health checks")
	}
	checks, err := mapHealthIDs(req.Checks)
	if err != nil {
		return HealthReport{}, err
	}
	required, err := mapHealthIDs(req.RequiredChecks)
	if err != nil {
		return HealthReport{}, err
	}
	report, err := a.health.CheckHealth(ctx, agentwrap.HealthCheckRequest{
		Context:        agentwrap.RuntimeContext{RuntimeKind: "opencode", RuntimeName: "opencode", Provider: agentwrap.ProviderID(req.Provider), Model: agentwrap.ModelID(req.Model)},
		WorkDir:        req.WorkDir,
		Provider:       agentwrap.ProviderID(req.Provider),
		Model:          agentwrap.ModelID(req.Model),
		Checks:         checks,
		RequiredChecks: required,
	})
	if err != nil {
		return HealthReport{}, err
	}
	out := HealthReport{Status: string(report.OverallStatus)}
	for _, result := range report.Results {
		out.Checks = append(out.Checks, HealthCheck{
			Name:    string(result.Check),
			Status:  healthStatus(result.Status),
			Message: result.UserDetail,
		})
	}
	if caps, err := a.Capabilities(ctx); err == nil {
		requiredCaps, capErr := mapCapabilitiesIDs(req.Capabilities)
		if capErr != nil {
			return out, capErr
		}
		for _, required := range requiredCaps {
			supported := false
			message := "required capability is unsupported"
			for _, feature := range caps.Features {
				if feature.Name == string(required) {
					supported = feature.Supported
					message = feature.Message
				}
			}
			out.Capabilities = append(out.Capabilities, CapabilityCheck{Name: string(required), Supported: supported, Message: message})
			if !supported && out.Status == string(agentwrap.HealthReady) {
				out.Status = string(agentwrap.HealthUnsupported)
			}
		}
	}
	if failure := agentwrap.RequiredHealthFailure(report, required); failure != nil {
		return out, failure
	}
	for _, capability := range out.Capabilities {
		if !capability.Supported {
			return out, fmt.Errorf("required runtime capability %q is unsupported", capability.Name)
		}
	}
	return out, nil
}

func mapCapabilities(caps agentwrap.Capabilities) Capabilities {
	out := Capabilities{RuntimeKind: string(caps.RuntimeKind)}
	for name, support := range caps.Features {
		out.Features = append(out.Features, CapabilityCheck{Name: string(name), Supported: support.Supported, Message: support.Detail})
	}
	return out
}

func healthStatus(status agentwrap.HealthStatus) string {
	switch status {
	case agentwrap.HealthReady, agentwrap.HealthSkipped:
		return "ok"
	case agentwrap.HealthDegraded, agentwrap.HealthUnknown, agentwrap.HealthUnsupported:
		return "warn"
	default:
		return "fail"
	}
}
