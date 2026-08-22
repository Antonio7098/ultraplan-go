package web

import (
	"net/http"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// This matrix freezes the Sprint 30-31 /api/v1 route vocabulary. Changes are
// accepted only with a coordinated browser/docs migration and an explicit
// compatibility rationale in docs/web-compatibility-baseline.md.
func TestAPICompatibilityRouteMethodMatrix(t *testing.T) {
	cases := []struct {
		name, path string
		methods    []string
	}{
		{"api_dashboard", "/api/v1/dashboard", []string{"GET", "HEAD"}},
		{"api_projects", "/api/v1/projects", []string{"GET", "HEAD"}},
		{"api_project", "/api/v1/projects/alpha", []string{"GET", "HEAD"}},
		{"api_sprint", "/api/v1/projects/alpha/sprints/32-hardening", []string{"GET", "HEAD"}},
		{"api_prompt_bundle", "/api/v1/projects/alpha/sprints/32-hardening/prompts/plan", []string{"GET", "HEAD"}},
		{"api_studies", "/api/v1/studies", []string{"GET", "HEAD"}},
		{"api_study", "/api/v1/studies/research", []string{"GET", "HEAD"}},
		{"api_validations", "/api/v1/validations", []string{"GET", "HEAD"}},
		{"api_artifact", "/api/v1/artifacts/opaque_ref", []string{"GET", "HEAD"}},
		{"api_health", "/api/v1/health", []string{"GET", "HEAD"}},
		{"api_operation_prepare", "/api/v1/operations/prepare", []string{"POST"}},
		{"api_operations", "/api/v1/operations", []string{"GET", "HEAD", "POST"}},
		{"api_operation", "/api/v1/operations/op_example", []string{"GET", "DELETE"}},
		{"api_operation_events", "/api/v1/operations/op_example/events", []string{"GET"}},
		{"api_runs", "/api/v1/runs", []string{"GET", "HEAD"}},
		{"api_timeline", "/api/v1/timeline", []string{"GET", "HEAD"}},
		{"api_run", "/api/v1/runs/run_aaaaaaaaaaaaaaaaaaaaaaaaaa", []string{"GET", "HEAD", "DELETE"}},
		{"api_run_events", "/api/v1/runs/run_aaaaaaaaaaaaaaaaaaaaaaaaaa/events", []string{"GET", "HEAD"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			match := matchRoute(tc.path)
			if !match.known || !match.api || match.name != tc.name {
				t.Fatalf("match=%+v", match)
			}
			if got := allowedMethods(match); !reflect.DeepEqual(got, tc.methods) {
				t.Fatalf("methods=%v want=%v", got, tc.methods)
			}
		})
	}
}

func TestAPICompatibilityTransportSchemas(t *testing.T) {
	cases := map[string]struct {
		value any
		want  string
	}{
		"meta":               {responseMeta{}, "api_version:string:api_version|request_id:string:request_id|generated_at:string:generated_at,omitempty|returned_count:*int:returned_count,omitempty|total_count:*int:total_count,omitempty|truncated:*bool:truncated,omitempty"},
		"error":              {errorBody{}, "code:string:code|message:string:message|retryable:bool:retryable,omitempty|details:map[string]interface {}:details,omitempty"},
		"artifact":           {artifactDTO{}, "ref:string:ref|label:string:label,omitempty|display_path:string:display_path|media_type:string:media_type"},
		"finding":            {findingDTO{}, "severity:string:severity|section:string:section,omitempty|problem:string:problem|cause:string:cause,omitempty|suggestion:string:suggestion,omitempty"},
		"stage":              {stageDTO{}, "name:string:name|status:string:status|error:string:error,omitempty|artifact_available:bool:artifact_available,omitempty|artifact_valid:bool:artifact_valid,omitempty|latest_outcome:string:latest_outcome,omitempty|next_action:string:next_action,omitempty"},
		"prompt_input":       {promptInputContractDTO{}, "stage:string:stage|required:[]string:required|optional:[]string:optional,omitempty|forbidden:[]string:forbidden,omitempty"},
		"prompt_block":       {promptBlockDTO{}, "id:string:id|kind:string:kind|mode:string:mode,omitempty|cacheable:bool:cacheable|bytes:int:bytes|sha256:string:sha256"},
		"prompt_bundle":      {promptBundleDTO{}, "stage:string:stage|available:bool:available|scope:string:scope|unavailable_reason:string:unavailable_reason,omitempty|input_contract:web.promptInputContractDTO:input_contract|schema_version:int:schema_version,omitempty|total_bytes:int:total_bytes,omitempty|shared_prefix_bytes:int:shared_prefix_bytes,omitempty|stage_suffix_bytes:int:stage_suffix_bytes,omitempty|shared_prefix_sha256:string:shared_prefix_sha256,omitempty|cache_key:string:cache_key,omitempty|cache_breakpoint_bytes:int:cache_breakpoint_bytes,omitempty|cache_candidate:bool:cache_candidate|cache_transport:string:cache_transport,omitempty|blocks:[]web.promptBlockDTO:blocks"},
		"execute":            {executeDTO{}, "available:bool:available|total:int:total,omitempty|pending:int:pending,omitempty|running:int:running,omitempty|complete:int:complete,omitempty|failed:int:failed,omitempty|cancelled:int:cancelled,omitempty"},
		"reviewer":           {reviewerDTO{}, "id:string:id|name:string:name,omitempty|kind:string:kind,omitempty|path:string:path,omitempty|status:string:status|summary:string:summary,omitempty"},
		"smoke":              {smokeDTO{}, "available:bool:available|status:string:status,omitempty|verdict:string:verdict,omitempty|stale:bool:stale,omitempty|run_id:string:run_id,omitempty"},
		"operation_document": {operationDocument{}, "id:string:id|kind:app.OperationKind:kind|state:string:state|reason:string:reason,omitempty|created_at:time.Time:created_at|started_at:*time.Time:started_at,omitempty|finished_at:*time.Time:finished_at,omitempty|last_event_id:string:last_event_id|durable_status:web.durableStatusDTO:durable_status|result:*web.operationResultDTO:result,omitempty"},
		"active_operation":   {activeOperationDTO{}, "id:string:id|kind:app.OperationKind:kind|state:string:state|project:string:project,omitempty|sprint:string:sprint,omitempty|study:string:study,omitempty|started_at:*time.Time:started_at,omitempty"},
		"operation_result":   {operationResultDTO{}, "state:string:state|subject:string:subject,omitempty|message:string:message,omitempty|content:string:content,omitempty|truncated:bool:truncated,omitempty|findings:[]web.findingDTO:findings,omitempty|error:*web.errorBody:error,omitempty"},
		"operation_scope":    {operationScopeRequest{}, "project:string:project,omitempty|sprint:string:sprint,omitempty|study:string:study,omitempty"},
		"operation_spec":     {operationSpecRequest{}, "kind:string:kind|scope:web.operationScopeRequest:scope|options:web.operationOptionsRequest:options,omitempty"},
		"preparation":        {preparationDTO{}, "preparation_id:string:preparation_id|operation:map[string]interface {}:operation|affected_paths:[]string:affected_paths|mutation_class:string:mutation_class|runtime:map[string]interface {}:runtime,omitempty|harness:map[string]interface {}:harness,omitempty|prerequisites:[]string:prerequisites|input_fingerprint:string:input_fingerprint|expires_at:time.Time:expires_at|confirmation_token:string:confirmation_token"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := jsonSchema(tc.value); got != tc.want {
				t.Fatalf("schema changed\n got: %s\nwant: %s", got, tc.want)
			}
		})
	}
}

func TestAPICompatibilityErrorsAndCachePolicy(t *testing.T) {
	h := testHandler(t, sampleQueries(), nil)
	for _, tc := range []struct {
		method, path, code string
		status             int
	}{
		{http.MethodGet, "/api/v2/projects", "not_found", http.StatusNotFound},
		{http.MethodPost, "/api/v1/projects", "method_not_allowed", http.StatusMethodNotAllowed},
		{http.MethodGet, "/api/v1/projects?unknown=1", "invalid_request", http.StatusBadRequest},
	} {
		res := request(h, tc.method, tc.path, nil)
		if res.Code != tc.status || !strings.Contains(res.Header().Get("Content-Type"), "application/json") ||
			!strings.Contains(res.Body.String(), `"code":"`+tc.code+`"`) || res.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("%s %s status=%d headers=%v body=%s", tc.method, tc.path, res.Code, res.Header(), res.Body.String())
		}
	}
}

func jsonSchema(value any) string {
	typeOf := reflect.TypeOf(value)
	fields := make([]string, 0, typeOf.NumField())
	for i := 0; i < typeOf.NumField(); i++ {
		field := typeOf.Field(i)
		tag := field.Tag.Get("json")
		name := strings.Split(tag, ",")[0]
		fields = append(fields, name+":"+field.Type.String()+":"+tag)
	}
	// Field order is part of the encoded compatibility contract. Sorting a copy
	// is only used to detect accidental duplicate JSON names clearly.
	names := append([]string(nil), fields...)
	sort.Strings(names)
	for i := 1; i < len(names); i++ {
		if strings.Split(names[i-1], ":")[0] == strings.Split(names[i], ":")[0] {
			return "duplicate-json-field:" + names[i]
		}
	}
	return strings.Join(fields, "|")
}
