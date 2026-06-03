package catalog

import "testing"

func TestDerivedModelIdentityAndPath(t *testing.T) {
	m := Model{Repo: "Qwen/Qwen3-4B-GGUF", Quant: "Q4_K_M", MMProj: "mmproj-F16.gguf"}
	if got := m.Ref(); got != "Qwen/Qwen3-4B-GGUF/Q4_K_M" {
		t.Fatalf("Ref() = %q", got)
	}
	if got := m.StableRelPath(); got != "hf/Qwen/Qwen3-4B-GGUF/Q4_K_M.gguf" {
		t.Fatalf("StableRelPath() = %q", got)
	}
	if got := m.GlobPattern(); got != "*Q4_K_M*.gguf" {
		t.Fatalf("GlobPattern() = %q", got)
	}
	if got := m.MMProjStableRelPath(); got != "hf/Qwen/Qwen3-4B-GGUF/mmproj-F16.gguf" {
		t.Fatalf("MMProjStableRelPath() = %q", got)
	}
}

func TestCatalogLookupByRef(t *testing.T) {
	c := &Catalog{Models: []Model{{Repo: "owner/repo", Quant: "Q8_0", Name: "friendly"}}}
	if _, ok := c.ByRef("owner/repo/Q8_0"); !ok {
		t.Fatal("lookup by derived ref failed")
	}
	if _, ok := c.ByRef("friendly"); !ok {
		t.Fatal("lookup by display name failed")
	}
}
