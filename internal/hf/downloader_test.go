package hf

import (
	"testing"

	"github.com/vex9z7/llama.cpp-stack/internal/catalog"
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
