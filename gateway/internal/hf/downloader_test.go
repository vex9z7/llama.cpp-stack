package hf

import (
	"testing"

	"github.com/vex9z7/llama.cpp-stack/gateway/internal/catalog"
)

func TestChooseFilePrefersMatchingGGUF(t *testing.T) {
	files := []string{
		"README.md",
		"Qwen3-4B-Q4_K_M.gguf",
		"mmproj-Qwen3-4B-Q4_K_M.gguf",
		"Qwen3-4B-Q8_0.gguf",
	}
	got, err := chooseFile(files, catalog.Model{Repo: "Qwen/Qwen3-4B-GGUF", Quant: "Q4_K_M"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "Qwen3-4B-Q4_K_M.gguf" {
		t.Fatalf("chooseFile() = %q", got)
	}
}

func TestChooseFileExact(t *testing.T) {
	got, err := chooseFile([]string{"nested/model.gguf"}, catalog.Model{Repo: "o/r", Quant: "Q4", File: "model.gguf"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "nested/model.gguf" {
		t.Fatalf("chooseFile exact = %q", got)
	}
}

func TestChooseFileAmbiguousRequiresPin(t *testing.T) {
	_, err := chooseFile([]string{"a-Q4_K_M.gguf", "b-Q4_K_M.gguf"}, catalog.Model{Repo: "o/r", Quant: "Q4_K_M"})
	if err == nil {
		t.Fatal("expected ambiguous match error")
	}
	if Code(err) != CodeAmbiguousFiles {
		t.Fatalf("Code(err)=%q", Code(err))
	}
}

func TestChooseFileRejectsSplitGGUF(t *testing.T) {
	_, err := chooseFile([]string{"model-Q4_K_M-00001-of-00002.gguf"}, catalog.Model{Repo: "o/r", Quant: "Q4_K_M"})
	if err == nil {
		t.Fatal("expected split GGUF error")
	}
	if Code(err) != CodeSplitGGUF {
		t.Fatalf("Code(err)=%q", Code(err))
	}
}

func TestChooseExactFileMatchesPathOrBase(t *testing.T) {
	got, err := chooseExactFile([]string{"nested/mmproj-F16.gguf"}, "o/r", "mmproj-F16.gguf")
	if err != nil {
		t.Fatal(err)
	}
	if got != "nested/mmproj-F16.gguf" {
		t.Fatalf("chooseExactFile() = %q", got)
	}
}
