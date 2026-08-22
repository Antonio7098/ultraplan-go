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
	return sanitizeAnyMap(src, 0)
}

const (
	maxMappedPayloadFields       = 64
	maxMappedPayloadStringBytes  = 8192
	maxMappedTerminalOutputBytes = 96 << 10
	maxMappedPayloadSliceItems   = 16
	maxMappedPayloadDepth        = 3
	maxMappedDiagnosticBytes     = 4096
)

func sanitizeAnyMap(src map[string]any, depth int) map[string]any {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]any, min(len(src), maxMappedPayloadFields+1))
	count := 0
	for k, v := range src {
		if count >= maxMappedPayloadFields {
			dst["_omitted_fields"] = len(src) - count
			break
		}
		dst[k] = sanitizeAnyValue(v, depth+1)
		count++
	}
	return dst
}

func sanitizeAnyValue(value any, depth int) any {
	switch v := value.(type) {
	case nil:
		return nil
	case bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, uintptr, float32, float64:
		return v
	case string:
		return truncatePayloadString(v)
	case []byte:
		if len(v) == 0 {
			return ""
		}
		return fmt.Sprintf("[bytes omitted: %d bytes]", len(v))
	case map[string]any:
		if depth >= maxMappedPayloadDepth {
			return fmt.Sprintf("[map omitted: %d fields]", len(v))
		}
		return sanitizeAnyMap(v, depth)
	case map[string]string:
		if depth >= maxMappedPayloadDepth {
			return fmt.Sprintf("[map omitted: %d fields]", len(v))
		}
		out := make(map[string]any, min(len(v), maxMappedPayloadFields+1))
		count := 0
		for key, item := range v {
			if count >= maxMappedPayloadFields {
				out["_omitted_fields"] = len(v) - count
				break
			}
			out[key] = truncatePayloadString(item)
			count++
		}
		return out
	case []any:
		return sanitizeAnySlice(v, depth)
	case []string:
		out := make([]any, 0, min(len(v), maxMappedPayloadSliceItems+1))
		for i, item := range v {
			if i >= maxMappedPayloadSliceItems {
				out = append(out, fmt.Sprintf("[omitted: %d items]", len(v)-i))
				break
			}
			out = append(out, truncatePayloadString(item))
		}
		return out
	default:
		return fmt.Sprintf("[%T omitted]", value)
	}
}

func sanitizeAnySlice(values []any, depth int) []any {
	if depth >= maxMappedPayloadDepth {
		return []any{fmt.Sprintf("[slice omitted: %d items]", len(values))}
	}
	out := make([]any, 0, min(len(values), maxMappedPayloadSliceItems+1))
	for i, item := range values {
		if i >= maxMappedPayloadSliceItems {
			out = append(out, fmt.Sprintf("[omitted: %d items]", len(values)-i))
			break
		}
		out = append(out, sanitizeAnyValue(item, depth+1))
	}
	return out
}

func truncatePayloadString(value string) string {
	return truncateString(value, maxMappedPayloadStringBytes)
}

func truncateDiagnosticString(value string) string {
	return truncateString(value, maxMappedDiagnosticBytes)
}

func truncateString(value string, limit int) string {
	if limit < 0 || len(value) <= limit {
		return value
	}
	return value[:limit] + fmt.Sprintf("... [truncated %d bytes]", len(value)-limit)
}
