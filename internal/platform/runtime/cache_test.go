package runtime

import (
	"path/filepath"
	"testing"

	agentwrapopencode "github.com/Antonio7098/agentwrap/opencode"
)

func TestRuntimeStoreAlsoScopesOpenCodeProcessTemp(t *testing.T) {
	store := filepath.Join(t.TempDir(), ".ultraplan", "runtime", "opencode", "task", "opencode.db")
	mapped, err := toAgentwrapRequest(Request{RuntimeStorePath: store, RuntimeStoreOwner: "task"})
	if err != nil {
		t.Fatal(err)
	}
	if got := mapped.Metadata[agentwrapopencode.MetadataDatabasePath]; got != store {
		t.Fatalf("database path = %q, want %q", got, store)
	}
	wantTemp := filepath.Join(filepath.Dir(store), "tmp")
	if got := mapped.Metadata[agentwrapopencode.MetadataTempRoot]; got != wantTemp {
		t.Fatalf("temp root = %q, want %q", got, wantTemp)
	}
}

func TestAgentwrapRequestReceivesCohortScopedCacheMetadata(t *testing.T) {
	req := Request{
		Prompt: "prompt", Provider: "openai", Model: "gpt", WorkDir: "/workspace", Sandbox: "read_only", Permissions: "restricted",
		Policy: PermissionPolicy{Default: "deny", Tools: map[string]string{"read": "allow"}},
		Cache:  CacheDirective{Key: "foundation", BreakpointBytes: 42, PrefixDigest: "digest", Mode: "stable-prefix"},
	}
	mapped, err := toAgentwrapRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	if mapped.Metadata["prompt_cache_key"] == "" || mapped.Metadata["prompt_cache_key"] == "foundation" || mapped.Metadata["prompt_cache_foundation_key"] != "foundation" || mapped.Metadata["prompt_cache_breakpoint_bytes"] != "42" || mapped.Metadata["prompt_cache_prefix_sha256"] != "digest" {
		t.Fatalf("cache metadata = %+v", mapped.Metadata)
	}
	req.WorkDir = "/different"
	different, err := toAgentwrapRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	if different.Metadata["prompt_cache_key"] == mapped.Metadata["prompt_cache_key"] {
		t.Fatal("different runtime envelopes shared one cache cohort key")
	}
}
