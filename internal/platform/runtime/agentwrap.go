package runtime

import (
	"fmt"

	"github.com/Antonio7098/agentwrap"
)

func mapHealthIDs(values []string) ([]agentwrap.HealthCheckID, error) {
	out := make([]agentwrap.HealthCheckID, 0, len(values))
	for _, value := range values {
		switch value {
		case "":
			continue
		case "runtime_available":
			out = append(out, agentwrap.HealthCheckRuntimeAvailable)
		case "structured_output":
			out = append(out, agentwrap.HealthCheckStructuredOutput)
		case "workdir":
			out = append(out, agentwrap.HealthCheckWorkDir)
		case "config":
			out = append(out, agentwrap.HealthCheckConfig)
		case "provider":
			out = append(out, agentwrap.HealthCheckProvider)
		case "model":
			out = append(out, agentwrap.HealthCheckModel)
		case "authentication":
			out = append(out, agentwrap.HealthCheckAuthentication)
		case "runtime_paths":
			out = append(out, agentwrap.HealthCheckRuntimePaths)
		default:
			return nil, fmt.Errorf("unsupported health check %q", value)
		}
	}
	return out, nil
}

func mapCapabilitiesIDs(values []string) ([]agentwrap.Capability, error) {
	out := make([]agentwrap.Capability, 0, len(values))
	for _, value := range values {
		switch value {
		case "":
			continue
		case "sessions":
			out = append(out, agentwrap.CapabilitySessions)
		case "session_continue":
			out = append(out, agentwrap.CapabilitySessionContinue)
		case "session_fork":
			out = append(out, agentwrap.CapabilitySessionFork)
		case "session_replace":
			out = append(out, agentwrap.CapabilitySessionReplace)
		case "session_release":
			out = append(out, agentwrap.CapabilitySessionRelease)
		case "cancellation":
			out = append(out, agentwrap.CapabilityCancellation)
		case "structured_events":
			out = append(out, agentwrap.CapabilityStructuredEvents)
		case "raw_payloads":
			out = append(out, agentwrap.CapabilityRawPayloads)
		case "artifacts":
			out = append(out, agentwrap.CapabilityArtifacts)
		case "permissions":
			out = append(out, agentwrap.CapabilityPermissions)
		case "usage":
			out = append(out, agentwrap.CapabilityUsage)
		case "validation_events":
			out = append(out, agentwrap.CapabilityValidationEvents)
		default:
			return nil, fmt.Errorf("unsupported capability %q", value)
		}
	}
	return out, nil
}

func mapPermissionPolicy(policy PermissionPolicy) (*agentwrap.PermissionPolicy, error) {
	if policy.Default == "" && len(policy.Tools) == 0 && len(policy.PathRules) == 0 && policy.UnsupportedBehavior == "" {
		return nil, nil
	}
	out := &agentwrap.PermissionPolicy{
		Default:             agentwrap.PermissionAction(policy.Default),
		Tools:               map[agentwrap.PermissionTool]agentwrap.PermissionAction{},
		UnsupportedBehavior: agentwrap.PermissionUnsupportedBehavior(policy.UnsupportedBehavior),
		Metadata:            cloneStringMap(policy.Metadata),
	}
	for tool, action := range policy.Tools {
		out.Tools[agentwrap.PermissionTool(tool)] = agentwrap.PermissionAction(action)
	}
	if len(out.Tools) == 0 {
		out.Tools = nil
	}
	for _, rule := range policy.PathRules {
		out.PathRules = append(out.PathRules, agentwrap.PermissionPathRule{
			Path:   rule.Path,
			Action: agentwrap.PermissionAction(rule.Action),
		})
	}
	if err := agentwrap.ValidatePermissionPolicy(out); err != nil {
		return nil, err
	}
	if len(out.PathRules) > 0 && out.UnsupportedBehavior != agentwrap.PermissionUnsupportedBestEffort {
		return nil, fmt.Errorf("permission path rules are unsupported by the current OpenCode adapter")
	}
	return out, nil
}

func cloneStringMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func cloneAnyMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
