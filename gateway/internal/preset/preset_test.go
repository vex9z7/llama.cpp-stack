package preset

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vex9z7/llama.cpp-stack/gateway/internal/catalog"
)

func TestRenderIncludesAllCatalogModels(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "hf", "owner", "chat"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "hf", "owner", "chat", "Q4.gguf"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	cat := &catalog.Catalog{Models: []catalog.Model{{Repo: "owner/chat", Quant: "Q4"}, {Repo: "owner/missing", Quant: "Q4"}, {Repo: "owner/embed", Quant: "Q4", Kind: "embedding"}}}
	out, err := Render(cat, Config{ModelsDir: dir, Path: filepath.Join(dir, "preset.ini"), CtxSize: 512, Parallel: 1, NGPULayers: 0})
	if err != nil {
		t.Fatal(err)
	}
	if out.IncludedCount != 3 {
		t.Fatalf("included count = %d", out.IncludedCount)
	}
	data, err := os.ReadFile(out.Path)
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	if !strings.Contains(s, "[owner/chat/Q4]") {
		t.Fatalf("missing chat section:\n%s", s)
	}
	if !strings.Contains(s, "[owner/missing/Q4]") {
		t.Fatalf("missing model should still be registered in preset:\n%s", s)
	}
}
