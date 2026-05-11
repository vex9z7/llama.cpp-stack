package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHFTokenPrefersTokenFile(t *testing.T) {
	t.Setenv("HF_TOKEN", "env-token")
	path := filepath.Join(t.TempDir(), "hf-token")
	if err := os.WriteFile(path, []byte(" file-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HF_TOKEN_FILE", path)

	token, err := hfToken()
	if err != nil {
		t.Fatal(err)
	}
	if token != "file-token" {
		t.Fatalf("token = %q, want file-token", token)
	}
}

func TestHFTokenFallsBackToEnv(t *testing.T) {
	t.Setenv("HF_TOKEN", "env-token")
	t.Setenv("HF_TOKEN_FILE", "")

	token, err := hfToken()
	if err != nil {
		t.Fatal(err)
	}
	if token != "env-token" {
		t.Fatalf("token = %q, want env-token", token)
	}
}
