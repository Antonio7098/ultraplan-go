package runtime

import (
	"crypto/sha256"
	"fmt"
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
	prompt := "stable prompt"
	digest := sha256.Sum256([]byte(prompt))
	prefixDigest := fmt.Sprintf("%x", digest[:])
	req := Request{
		Prompt: prompt, Provider: "openai", Model: "gpt", WorkDir: "/workspace", Sandbox: "read_only", Permissions: "restricted",
		Policy: PermissionPolicy{Default: "deny", Tools: map[string]string{"read": "allow"}},
		Cache:  CacheDirective{Key: "foundation", BreakpointBytes: len(prompt), PrefixDigest: prefixDigest, Mode: "stable-prefix"},
	}
	mapped, err := toAgentwrapRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	if mapped.Metadata["prompt_cache_key"] == "" || mapped.Metadata["prompt_cache_key"] == "foundation" || mapped.Metadata["prompt_cache_foundation_key"] != "foundation" || mapped.Metadata["prompt_cache_breakpoint_bytes"] != fmt.Sprint(len(prompt)) || mapped.Metadata["prompt_cache_prefix_sha256"] != prefixDigest {
		t.Fatalf("cache metadata = %+v", mapped.Metadata)
	}
	if mapped.PromptCache.Key != mapped.Metadata["prompt_cache_key"] || mapped.PromptCache.BreakpointBytes != len(prompt) || mapped.PromptCache.PrefixSHA256 != prefixDigest || mapped.PromptCache.Mode != "stable-prefix" {
		t.Fatalf("typed cache directive = %+v", mapped.PromptCache)
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

func TestAgentwrapRequestRejectsStaleCacheBoundary(t *testing.T) {
	_, err := toAgentwrapRequest(Request{
		Prompt: "prompt",
		Cache:  CacheDirective{Key: "foundation", BreakpointBytes: 3, PrefixDigest: "stale", Mode: "stable-prefix"},
	})
	if err == nil {
		t.Fatal("stale cache boundary accepted")
	}
}
